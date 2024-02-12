package gologger

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"time"
)

type fileLogger struct {
	file   string
	levels []slog.Level
}

type FileLoggerOption func(*fileLogger)

func WithFile(filepath string, opts ...FileLoggerOption) (Option, error) {
	if filepath == "" {
		return nil, fmt.Errorf("no filename specified")
	}

	if !canWriteToFile(filepath) {
		return nil, fmt.Errorf("%s cannot be accessed", filepath)
	}

	return func(l *logger) {
		logger := fileLogger{
			file:   filepath,
			levels: []slog.Level{slog.LevelError, slog.LevelInfo},
		}
		for _, opt := range opts {
			opt(&logger)
		}

		if sliceContains(logger.levels, slog.LevelDebug) {
			OnDebug(func(msg string, additionalValues map[string]any) {
				line := fmt.Sprintf("%s | %s | %s | %s", time.Now(), "DEBUG", msg, formatAdditionalValues(additionalValues))
				appendLineToFile(logger.file, line)
			})
		}
		if sliceContains(logger.levels, slog.LevelInfo) {
			OnInfo(func(msg string, additionalValues map[string]any) {
				line := fmt.Sprintf("%s | %s | %s | %s", time.Now(), "INFO", msg, formatAdditionalValues(additionalValues))
				appendLineToFile(logger.file, line)
			})
		}
		if sliceContains(logger.levels, slog.LevelWarn) {
			OnWarn(func(msg string, additionalValues map[string]any) {
				line := fmt.Sprintf("%s | %s | %s | %s", time.Now(), "WARN", msg, formatAdditionalValues(additionalValues))
				appendLineToFile(logger.file, line)
			})
		}
		if sliceContains(logger.levels, slog.LevelError) {
			OnErr(func(msg string, additionalValues map[string]any) {
				line := fmt.Sprintf("%s | %s | %s | %s", time.Now(), "ERROR", msg, formatAdditionalValues(additionalValues))
				appendLineToFile(logger.file, line)
			})
		}

	}, nil
}

// canWriteToFile checks if a file can be opened for writing.
// If the file does not exist, it checks if it can be created.
func canWriteToFile(filename string) bool {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
	if err != nil {
		return false
	}
	defer file.Close()
	return true
}

func appendLineToFile(filename, line string) error {
	// Open the file with flags to Append, Create if not exists, and Write only mode.
	// 0666 specifies the file permissions when the file is created.
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	if _, err := writer.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("error writing to file: %w", err)
	}

	// Flush any buffered data to the underlying file
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("error flushing data to file: %w", err)
	}

	return nil
}
