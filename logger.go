package gologger

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// LogCallback is the function signature for log event subscribers
type LogCallback func(msg string, args ...any)

// Logger represents the global logger instance
type Logger struct {
	mu        sync.RWMutex
	level     slog.Level
	callbacks map[slog.Level][]LogCallback
}

var (
	defaultLogger = &Logger{level: slog.LevelInfo, callbacks: make(map[slog.Level][]LogCallback)}
	levels        = []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}
)

func ParseLevel(levelStr string) (slog.Level, error) {
	switch strings.ToLower(levelStr) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level %q", levelStr)
	}
}

// Setup configures the default logger with slog handlers based on the given level string
func Setup(levelStr string) error {
	logLvl, err := ParseLevel(levelStr)
	if err != nil {
		return err
	}
	SetLevel(logLvl)

	// Setup default slog handlers
	RegisterCallback(slog.LevelDebug, slog.Debug)
	RegisterCallback(slog.LevelInfo, slog.Info)
	RegisterCallback(slog.LevelWarn, slog.Warn)
	RegisterCallback(slog.LevelError, slog.Error)

	return nil
}

// GetLevel returns the current logging level
func GetLevel() slog.Level {
	defaultLogger.mu.RLock()
	defer defaultLogger.mu.RUnlock()
	return defaultLogger.level
}

func SetLevel(level slog.Level) {
	defaultLogger.mu.Lock()
	defaultLogger.level = level
	defaultLogger.mu.Unlock()
}

// log is a private helper function that handles the common logging logic
func (l *Logger) log(level slog.Level, msg string, args ...any) {
	l.mu.RLock()
	if l.level > level {
		l.mu.RUnlock()
		return
	}
	// Make a copy of callbacks to avoid holding the lock while executing them
	callbacks := make([]LogCallback, len(l.callbacks[level]))
	copy(callbacks, l.callbacks[level])
	l.mu.RUnlock()

	for _, cb := range callbacks {
		cb(msg, args...)
	}
}

// Debug logs a debug message with the given arguments
func Debug(msg string, args ...any) { defaultLogger.log(slog.LevelDebug, msg, args...) }

// Info logs an info message with the given arguments
func Info(msg string, args ...any) { defaultLogger.log(slog.LevelInfo, msg, args...) }

// Warn logs a warning message with the given arguments
func Warn(msg string, args ...any) { defaultLogger.log(slog.LevelWarn, msg, args...) }

// Error logs an error message with the given arguments
func Error(msg string, args ...any) { defaultLogger.log(slog.LevelError, msg, args...) }

// RegisterCallback registers a callback function for the specified level
func RegisterCallback(level slog.Level, cb LogCallback) {
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.callbacks[level] = append(defaultLogger.callbacks[level], cb)
}

// Convenience functions for backward compatibility
func OnDebug(cb LogCallback) {
	RegisterCallback(slog.LevelDebug, cb)
}

func OnInfo(cb LogCallback) {
	RegisterCallback(slog.LevelInfo, cb)
}

func OnWarn(cb LogCallback) {
	RegisterCallback(slog.LevelWarn, cb)
}

func OnError(cb LogCallback) {
	RegisterCallback(slog.LevelError, cb)
}

func getLevelsAbove(level slog.Level) []slog.Level {
	for i, l := range levels {
		if l == level {
			return levels[i:]
		}
	}
	return nil
}
