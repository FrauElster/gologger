package gologger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type LokiOption func(loki *LokiNotifier)

func WithLevels(levels []slog.Level) LokiOption {
	return func(l *LokiNotifier) { l.levels = levels }
}
func WithBatchWait(duration time.Duration) LokiOption {
	return func(l *LokiNotifier) { l.batchWait = duration }
}

// WithLoki sets up the logger to send logs to a loki instance
// the context is used to check if the loki instance is reachable AND for the runtime
// if the context is cancelled, the loki will stop sending logs
// lokiHost is the host of the loki instance
// server is the name of the server that sends the logs
// job is the name of the job that sends the logs
// returns an error if the loki instance is not reachable or if the server or job is not set
func WithLoki(ctx context.Context, lokiHost, server, job string, opts ...LokiOption) (Option, error) {
	if lokiHost == "" {
		return nil, fmt.Errorf("lokiHost must be set")
	}
	if server == "" {
		return nil, fmt.Errorf("server must be set")
	}
	if job == "" {
		return nil, fmt.Errorf("job must be set")
	}

	err := waitForLoki(ctx, lokiHost)
	if err != nil {
		return nil, err
	}

	return func(l *logger) {
		labels := map[string]string{"source": server, "job": job}

		loki := LokiNotifier{
			baseLabels: labels,
			lokiHost:   lokiHost,
			levels:     []slog.Level{slog.LevelError, slog.LevelInfo},
			batchWait:  5 * time.Second,
			batch:      make(chan logEntry),
		}

		for _, opt := range opts {
			opt(&loki)
		}

		if sliceContains(loki.levels, slog.LevelDebug) {
			l.onDebug = append(l.onDebug, func(msg string, additionalValues map[string]any) {
				loki.batch <- logEntry{
					Level:            slog.LevelInfo,
					Timestamp:        time.Now(),
					Message:          msg,
					AdditionalValues: formatAdditionalValues(additionalValues),
				}
			})
		}
		if sliceContains(loki.levels, slog.LevelInfo) {
			l.onInfo = append(l.onInfo, func(msg string, additionalValues map[string]any) {
				loki.batch <- logEntry{
					Level:            slog.LevelInfo,
					Timestamp:        time.Now(),
					Message:          msg,
					AdditionalValues: formatAdditionalValues(additionalValues),
				}
			})
		}
		if sliceContains(loki.levels, slog.LevelWarn) {
			l.onWarn = append(l.onWarn, func(msg string, additionalValues map[string]any) {
				loki.batch <- logEntry{
					Level:            slog.LevelWarn,
					Timestamp:        time.Now(),
					Message:          msg,
					AdditionalValues: formatAdditionalValues(additionalValues),
				}
			})
		}
		if sliceContains(loki.levels, slog.LevelError) {
			l.onErr = append(l.onErr, func(msg string, additionalValues map[string]any) {
				loki.batch <- logEntry{
					Level:            slog.LevelError,
					Timestamp:        time.Now(),
					Message:          msg,
					AdditionalValues: formatAdditionalValues(additionalValues),
				}
			})
		}

		go loki.run(ctx)
	}, nil
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        ` json:"values"`
}
type lokiMessage struct {
	Streams []lokiStream `json:"streams"`
}

type logEntry struct {
	Level            slog.Level     `json:"level"`
	Timestamp        time.Time      `json:"-"`
	Message          string         `json:"message"`
	AdditionalValues map[string]any `json:"additionalValues"`
}

func (e logEntry) MarshalJSON() ([]byte, error) {
	// slog.Level warn is "WARN", Loki expects "WARNING"
	level := e.Level.String()
	if level == "WARN" {
		level = "WARNING"
	}

	return json.Marshal(struct {
		Level            string         `json:"level"`
		Message          string         `json:"message"`
		AdditionalValues map[string]any `json:"additionalValues"`
	}{
		Level:            level,
		Message:          e.Message,
		AdditionalValues: e.AdditionalValues,
	})
}

type LokiNotifier struct {
	lokiHost   string
	baseLabels map[string]string
	levels     []slog.Level

	batchWait time.Duration
	batch     chan logEntry
}

func (l *LokiNotifier) run(ctx context.Context) {
	currentBatch := make([]logEntry, 0)
	sendLogs := func() {
		if len(currentBatch) == 0 {
			return
		}

		err := l.send(currentBatch)
		if err != nil {
			// we use std logger here because we don't want to create a loop
			slog.Error("failed to send batch to loki", "lokiHost", l.lokiHost, "err", err)
			for _, entry := range currentBatch {
				if entry.AdditionalValues == nil {
					entry.AdditionalValues = make(map[string]any)
				}
				entry.AdditionalValues["originalTimestamp"] = entry.Timestamp
				slog.Log(context.Background(), entry.Level, entry.Message, mapAdditionalValues(entry.AdditionalValues)...)
			}
		}
		currentBatch = make([]logEntry, 0)
	}

	ticker := time.NewTicker(l.batchWait)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// send remaining logs
			sendLogs()
			return
		case newMessage := <-l.batch:
			currentBatch = append(currentBatch, newMessage)
			continue
		case <-ticker.C:
			sendLogs()
		}
	}
}

func (l *LokiNotifier) send(batch []logEntry) error {
	// first transform our messages into lokiExpected json message format (ref: https://grafana.com/docs/loki/latest/api/#push-log-entries-to-loki)
	values := make([][]string, len(batch))
	for idx, entry := range batch {
		marshelled, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("could not marshal entry: %w", err)
		}
		values[idx] = []string{strconv.FormatInt(entry.Timestamp.UnixNano(), 10), string(marshelled)}
	}
	// copy base labels
	labels := make(map[string]string)
	for k, v := range l.baseLabels {
		labels[k] = v
	}

	content, err := json.Marshal(lokiMessage{Streams: []lokiStream{
		{Stream: labels, Values: values},
	}})
	if err != nil {
		return fmt.Errorf("could not marshal batch: %w", err)
	}

	// compress it
	compressed, err := zip(content)
	if err != nil {
		return fmt.Errorf("could not compress batch: %w", err)
	}

	// prepare request
	req, err := http.NewRequest("POST", joinUrl(l.lokiHost, "/loki/api/v1/push"), compressed)
	if err != nil {
		return fmt.Errorf("could not create request: %w", err)
	}
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")

	// send it to the loki host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not send batch: %w", err)
	}
	defer resp.Body.Close()

	// check for errors on loki
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("got error response from loki: %s - %s", resp.Status, string(body))
	}

	return nil
}

func waitForLoki(ctx context.Context, lokiHost string) error {
	attempts := 0
	for {
		if attempts > 1 {
			return fmt.Errorf("could not connect to loki")
		}
		res, err := http.Get(joinUrl(lokiHost, "/ready"))
		if err != nil {
			attempts += 1
			continue
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			attempts += 1
			time.Sleep(5 * time.Second)
			continue
		}

		return nil
	}
}

func formatDuration(d time.Duration) string {
	// Custom formatting can go here
	// This is a simple example:
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second

	parts := make([]string, 0)
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}

	return strings.Join(parts, ":")
}
