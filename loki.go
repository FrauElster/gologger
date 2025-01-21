package gologger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type LokiConfig struct {
	URL       string            // Loki server URL
	BatchWait time.Duration     // Maximum amount of time to wait before sending a batch
	Labels    map[string]string // Default labels to add to all logs
	Tenant    string            // Optional tenant ID for multi-tenancy
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"` // [timestamp, message]
}

type lokiBatch struct {
	Streams []lokiStream `json:"streams"`
}

type buffer struct {
	entries []logEntry
	mu      sync.Mutex
}

type logEntry struct {
	timestamp time.Time
	level     slog.Level
	msg       string
	args      []any
}

var (
	logBuffer *buffer
	ticker    *time.Ticker
	done      chan bool
)

// UseLoki sets up logging callbacks that send logs to a Loki instance
func UseLoki(cfg LokiConfig) error {
	if cfg.URL == "" {
		return fmt.Errorf("Loki URL cannot be empty")
	}

	// Set default values if not provided
	if cfg.BatchWait == 0 {
		cfg.BatchWait = time.Second
	}
	if cfg.Labels == nil {
		cfg.Labels = make(map[string]string)
	}

	// Ensure we have some basic labels
	if _, ok := cfg.Labels["source"]; !ok {
		cfg.Labels["source"] = "application"
	}

	// Initialize buffer and control channels
	logBuffer = &buffer{
		entries: make([]logEntry, 0),
	}
	done = make(chan bool)
	ticker = time.NewTicker(cfg.BatchWait)

	// Start batch processing
	go processBatches(cfg)

	// Register callbacks for all levels
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		level := level // Create new variable for closure
		RegisterCallback(level, func(msg string, args ...any) {
			logBuffer.mu.Lock()
			logBuffer.entries = append(logBuffer.entries, logEntry{
				timestamp: time.Now(),
				level:     level,
				msg:       msg,
				args:      args,
			})
			logBuffer.mu.Unlock()
		})
	}

	return nil
}

// StopLoki gracefully shuts down the Loki integration
func StopLoki() {
	if ticker != nil {
		ticker.Stop()
		done <- true
	}
}

func processBatches(cfg LokiConfig) {
	client := &http.Client{Timeout: 5 * time.Second}

	for {
		select {
		case <-ticker.C:
			sendBatch(client, cfg)
		case <-done:
			// Send any remaining logs before shutting down
			sendBatch(client, cfg)
			return
		}
	}
}

func sendBatch(client *http.Client, cfg LokiConfig) {
	logBuffer.mu.Lock()
	if len(logBuffer.entries) == 0 {
		logBuffer.mu.Unlock()
		return
	}

	// Take current entries and reset the buffer
	entries := logBuffer.entries
	logBuffer.entries = make([]logEntry, 0)
	logBuffer.mu.Unlock()

	// Group entries by level
	streamsByLevel := make(map[slog.Level][][2]string)
	for _, entry := range entries {
		// Format message with args
		var fields string
		for i := 0; i < len(entry.args); i += 2 {
			if i+1 < len(entry.args) {
				fields += fmt.Sprintf(" %v=%v", entry.args[i], entry.args[i+1])
			}
		}
		message := fmt.Sprintf("%s%s", entry.msg, fields)

		// Create timestamp in nanosecond precision
		timestamp := fmt.Sprintf("%d", entry.timestamp.UnixNano())

		streamsByLevel[entry.level] = append(streamsByLevel[entry.level], [2]string{timestamp, message})
	}

	// Create batch payload
	batch := lokiBatch{
		Streams: make([]lokiStream, 0, len(streamsByLevel)),
	}

	// Create a stream for each level
	for level, values := range streamsByLevel {
		labels := make(map[string]string)
		for k, v := range cfg.Labels {
			labels[k] = v
		}
		labels["level"] = levelToString(level)

		batch.Streams = append(batch.Streams, lokiStream{
			Stream: labels,
			Values: values,
		})
	}

	// Send to Loki
	payload, err := json.Marshal(batch)
	if err != nil {
		slog.Error("Failed to marshal Loki batch", "error", err)
		return
	}

	req, err := http.NewRequest("POST", cfg.URL+"/loki/api/v1/push", bytes.NewBuffer(payload))
	if err != nil {
		slog.Error("Failed to create Loki request", "error", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if cfg.Tenant != "" {
		req.Header.Set("X-Scope-OrgID", cfg.Tenant)
	}

	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Failed to send logs to Loki", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		slog.Error("Unexpected response from Loki",
			"statusCode", resp.StatusCode,
			"status", resp.Status)
	}
}

func levelToString(level slog.Level) string {
	switch level {
	case slog.LevelDebug:
		return "debug"
	case slog.LevelInfo:
		return "info"
	case slog.LevelWarn:
		return "warn"
	case slog.LevelError:
		return "error"
	default:
		return "unknown"
	}
}
