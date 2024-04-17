package gologger

import (
	"fmt"
	"log/slog"
	"strings"
)

var l *logger

type logger struct {
	level slog.Level

	onDebug []Callback
	onInfo  []Callback
	onWarn  []Callback
	onErr   []Callback
}

type Callback func(msg string, additionalValues map[string]any)

type Option func(*logger)

func Init(level slog.Level, options ...Option) error {
	l = &logger{
		level:   level,
		onDebug: make([]Callback, 0),
		onInfo:  make([]Callback, 0),
		onWarn:  make([]Callback, 0),
		onErr:   make([]Callback, 0),
	}

	for _, option := range options {
		option(l)
	}

	// always enable console logs
	addSlogCallbacks()

	return nil
}

func ApplyOptions(options ...Option) error {
	if l == nil {
		return fmt.Errorf("logger not initialized")
	}

	for _, option := range options {
		option(l)
	}
	return nil
}

func OnDebug(callback Callback) {
	l.onDebug = append(l.onDebug, callback)
}

func OnInfo(callback Callback) {
	l.onInfo = append(l.onInfo, callback)
}

func OnWarn(callback Callback) {
	l.onWarn = append(l.onWarn, callback)
}

func OnErr(callback Callback) {
	l.onErr = append(l.onErr, callback)
}

func Log(level slog.Level, message string, additionaValues map[string]any) {
	if l == nil {
		slog.Error("Logger not initialized")
		return
	}

	if level < l.level {
		return
	}

	switch level {
	case slog.LevelDebug:
		for _, callback := range l.onDebug {
			callback(message, additionaValues)
		}
	case slog.LevelInfo:
		for _, callback := range l.onInfo {
			callback(message, additionaValues)
		}
	case slog.LevelWarn:
		for _, callback := range l.onWarn {
			callback(message, additionaValues)
		}
	case slog.LevelError:
		for _, callback := range l.onErr {
			callback(message, additionaValues)
		}
	}
}

func Debug(message string, additionaValues map[string]any) {
	Log(slog.LevelDebug, message, additionaValues)
}

func Info(message string, additionaValues map[string]any) {
	Log(slog.LevelInfo, message, additionaValues)
}

func Warn(message string, additionaValues map[string]any) {
	Log(slog.LevelWarn, message, additionaValues)
}

func Error(message string, additionaValues map[string]any) {
	Log(slog.LevelError, message, additionaValues)
}

func SetLevel(level slog.Level) {
	if l == nil {
		slog.Error("Logger not initialized")
		return
	}
	l.level = level
}

// ParseLogLevel parses a string into a slog.Level
func ParseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level %q", s)
	}
}

// MustParseLogLevel is like ParseLogLevel but panics if the input is invalid
func MustParseLogLevel(s string) slog.Level {
	level, err := ParseLogLevel(s)
	if err != nil {
		panic(err)
	}
	return level
}

func IsInitialized() bool { return l != nil }
