package gologger

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type FileConfig struct {
	Path       string            // Path to the log file
	TimeFormat string            // Format for timestamps, defaults to time.RFC3339
	FormatJson bool              // Whether to format logs as JSON
	LabelsMap  map[string]string // Labels to be included with every log entry
	MinLevel   *slog.Level       // Minimum log level to write to file
}

type jsonLogEntry struct {
	Time    string            `json:"time"`
	Level   string            `json:"level"`
	Labels  map[string]string `json:"labels,omitempty"`
	Message string            `json:"message"`
	Fields  map[string]any    `json:"fields,omitempty"`
}

var (
	fileWriter *os.File
	fileMu     sync.Mutex
)

// UseFile sets up logging callbacks that write logs to the specified file
func UseFile(cfg FileConfig) error {
	if cfg.Path == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	// Ensure directory exists
	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Open file in append mode, create if not exists
	f, err := os.OpenFile(cfg.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file %s: %w", cfg.Path, err)
	}

	fileWriter = f

	if cfg.TimeFormat == "" {
		cfg.TimeFormat = time.RFC3339
	}

	if cfg.LabelsMap == nil {
		cfg.LabelsMap = make(map[string]string)
	}

	// Helper function to write a log entry to file
	writeToFile := func(level slog.Level, msg string, args ...any) {
		timestamp := time.Now().Format(cfg.TimeFormat)

		var logLine string
		if cfg.FormatJson {
			// Create JSON entry
			entry := jsonLogEntry{
				Time:    timestamp,
				Level:   levelToString(level),
				Message: msg,
			}

			// Add labels if present
			if len(cfg.LabelsMap) > 0 {
				entry.Labels = cfg.LabelsMap
			}

			// Parse args into fields map
			if len(args) > 0 {
				fields := make(map[string]any)
				for i := 0; i < len(args); i += 2 {
					if i+1 < len(args) {
						fields[fmt.Sprint(args[i])] = args[i+1]
					}
				}
				if len(fields) > 0 {
					entry.Fields = fields
				}
			}

			// Marshal to JSON
			jsonData, err := json.Marshal(entry)
			if err != nil {
				slog.Error("Failed to marshal log entry to JSON",
					"error", err,
					"message", msg,
					"level", levelToString(level))
				return
			}
			logLine = string(jsonData) + "\n"
		} else {
			// Format text entry with labels
			var labels string
			if len(cfg.LabelsMap) > 0 {
				labelPairs := make([]string, 0, len(cfg.LabelsMap))
				for k, v := range cfg.LabelsMap {
					labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", k, v))
				}
				labels = fmt.Sprintf("[%s] ", strings.Join(labelPairs, " "))
			}

			// Format fields
			var fields string
			for i := 0; i < len(args); i += 2 {
				if i+1 < len(args) {
					fields += fmt.Sprintf(" %v=%v", args[i], args[i+1])
				}
			}

			logLine = fmt.Sprintf("[%s] %s: %s%s%s\n",
				timestamp,
				levelToString(level),
				labels,
				msg,
				fields,
			)
		}

		// Write to file with mutex lock
		fileMu.Lock()
		if _, err := fileWriter.WriteString(logLine); err != nil {
			// If file writing fails, log to stderr via slog
			slog.Error("Failed to write to log file",
				"error", err,
				"message", msg,
				"level", levelToString(level))
		}
		fileMu.Unlock()
	}

	minLevel := slog.LevelDebug
	if cfg.MinLevel != nil {
		minLevel = *cfg.MinLevel
	}
	levelsToRegister := getLevelsAbove(minLevel)
	for _, level := range levelsToRegister {
		RegisterCallback(level, func(msg string, args ...any) { writeToFile(level, msg, args...) })
	}

	return nil
}

// StopFile closes the file writer
func StopFile() error {
	if fileWriter != nil {
		fileMu.Lock()
		defer fileMu.Unlock()
		return fileWriter.Close()
	}
	return nil
}
