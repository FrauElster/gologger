package gologger

import (
	"bytes"
	"compress/gzip"
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

func WithLoki(lokiHost, server, job string, opts ...LokiOption) (Option, error) {
	if lokiHost == "" {
		return nil, fmt.Errorf("lokiHost must be set")
	}
	if server == "" {
		return nil, fmt.Errorf("server must be set")
	}
	if job == "" {
		return nil, fmt.Errorf("job must be set")
	}

	err := waitForLoki(context.Background(), lokiHost)
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
			OnDebug(func(msg string, additionalValues map[string]any) {
				loki.batch <- logEntry{
					Level:            slog.LevelInfo,
					Timestamp:        time.Now(),
					Message:          msg,
					AdditionalValues: formatAdditionalValues(additionalValues),
				}
			})
		}
		if sliceContains(loki.levels, slog.LevelInfo) {
			OnInfo(func(msg string, additionalValues map[string]any) {
				loki.batch <- logEntry{
					Level:            slog.LevelInfo,
					Timestamp:        time.Now(),
					Message:          msg,
					AdditionalValues: formatAdditionalValues(additionalValues),
				}
			})
		}
		if sliceContains(loki.levels, slog.LevelWarn) {
			OnWarn(func(msg string, additionalValues map[string]any) {
				loki.batch <- logEntry{
					Level:            slog.LevelWarn,
					Timestamp:        time.Now(),
					Message:          msg,
					AdditionalValues: formatAdditionalValues(additionalValues),
				}
			})
		}
		if sliceContains(loki.levels, slog.LevelError) {
			OnErr(func(msg string, additionalValues map[string]any) {
				loki.batch <- logEntry{
					Level:            slog.LevelError,
					Timestamp:        time.Now(),
					Message:          msg,
					AdditionalValues: formatAdditionalValues(additionalValues),
				}
			})
		}

		go loki.run()
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

type LokiNotifier struct {
	lokiHost   string
	baseLabels map[string]string
	levels     []slog.Level

	batchWait time.Duration
	batch     chan logEntry
}

func (l *LokiNotifier) run() {
	ticker := time.NewTicker(l.batchWait)

	currentBatch := make([]logEntry, 0)
	for {
		select {
		case newMessage := <-l.batch:
			currentBatch = append(currentBatch, newMessage)
			continue
		case <-ticker.C:
			if len(currentBatch) == 0 {
				continue
			}

			err := l.send(currentBatch)
			if err != nil {
				slog.Error("failed to send batch to loki", "lokiHost", l.lokiHost, "err", err)
				for _, entry := range currentBatch {
					if entry.AdditionalValues == nil {
						entry.AdditionalValues = make(map[string]any)
					}
					entry.AdditionalValues["originalTimestamp"] = entry.Timestamp
					slog.Log(context.Background(), entry.Level, entry.Message, mapAdditionalValues(entry.AdditionalValues)...)
				}
			}
			clear(currentBatch)
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
	client := &http.Client{}
	resp, err := client.Do(req)
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
		if attempts > 5 {
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

func zip(data []byte) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	gz := gzip.NewWriter(buf)

	_, err := gz.Write(data)
	if err != nil {
		return buf, fmt.Errorf("could not compress data: %w", err)
	}

	err = gz.Close()
	if err != nil {
		return buf, fmt.Errorf("could not close compression writer: %w", err)
	}

	return buf, nil
}

func joinUrl(elements ...string) string {
	for idx, element := range elements {
		if idx > 0 {
			element = strings.TrimPrefix(element, "/")
		}
		if idx < len(elements)-1 {
			element = strings.TrimSuffix(element, "/")
		}
		elements[idx] = element
	}
	return strings.Join(elements, "/")
}
