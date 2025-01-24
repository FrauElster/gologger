package gologger

import (
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type testStruct struct {
	ID   int
	Name string
}

func TestArgumentValidation(t *testing.T) {
	// Reset the default logger before each test
	defaultLogger = &Logger{
		level:     slog.LevelInfo,
		callbacks: make(map[slog.Level][]LogCallback),
		stringers: make(map[reflect.Type]StringConverter),
	}

	tests := []struct {
		name          string
		args          []any
		expectPanic   bool
		panicContains string
	}{
		{
			name:          "odd number of arguments",
			args:          []any{"key1", "value1", "key2"},
			expectPanic:   true,
			panicContains: "invalid number of arguments",
		},
		{
			name:          "non-string key",
			args:          []any{123, "value1"},
			expectPanic:   true,
			panicContains: "invalid key type",
		},
		{
			name:          "empty string key",
			args:          []any{"", "value1"},
			expectPanic:   true,
			panicContains: "empty string key",
		},
		{
			name: "valid key-value pairs",
			args: []any{
				"key1", "value1",
				"key2", 123,
				"key3", true,
			},
			expectPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if tt.expectPanic {
					if r == nil {
						t.Errorf("expected panic containing %q, got none", tt.panicContains)
					} else if !strings.Contains(r.(string), tt.panicContains) {
						t.Errorf("expected panic containing %q, got %q", tt.panicContains, r)
					}
				} else if r != nil {
					t.Errorf("unexpected panic: %v", r)
				}
			}()

			Info("test message", tt.args...)
		})
	}
}

func TestStringConversion(t *testing.T) {
	// Reset the default logger before each test
	defaultLogger = &Logger{
		level:     slog.LevelInfo,
		callbacks: make(map[slog.Level][]LogCallback),
		stringers: make(map[reflect.Type]StringConverter),
	}

	t.Run("time.Time conversion", func(t *testing.T) {
		// Register a custom time formatter
		RegisterStringer(func(tm time.Time) string { return tm.Format("2006-01-02") })

		var captured []any
		RegisterCallback(slog.LevelInfo, func(msg string, args ...any) { captured = args })

		testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		Info("test", "timestamp", testTime)

		if len(captured) != 2 {
			t.Fatalf("expected 2 arguments (key-value pair), got %d", len(captured))
		}
		if captured[1] != "2024-01-01" {
			t.Errorf("expected '2024-01-01', got '%v'", captured[1])
		}
	})

	t.Run("custom type conversion", func(t *testing.T) {
		type UserID int64
		RegisterStringer(func(id UserID) string { return "user-" + strconv.FormatInt(int64(id), 10) })

		var captured []any
		RegisterCallback(slog.LevelInfo, func(msg string, args ...any) { captured = args })

		Info("test", "user_id", UserID(123))

		if len(captured) != 2 {
			t.Fatalf("expected 2 arguments (one key-value pair), got %d", len(captured))
		}
		if captured[1] != "user-123" {
			t.Errorf("expected 'user-123', got '%v'", captured[1])
		}
	})

	t.Run("nil handling", func(t *testing.T) {
		var captured []any
		RegisterCallback(slog.LevelInfo, func(msg string, args ...any) { captured = args })

		// pass a nil pointer
		var ptr *string = nil
		Info("test", "pointer", ptr)
		if len(captured) != 2 {
			t.Fatalf("expected 2 arguments (one key-value pair), got %d", len(captured))
		}
		if captured[1] != nil {
			t.Errorf("expected nil, got '%v' (%T)", captured[1], captured[1])
		}

		// pass it nil directly
		Info("test", "pointer", nil)
		if len(captured) != 2 {
			t.Fatalf("expected 2 arguments (one key-value pair), got %d", len(captured))
		}
		if captured[1] != nil {
			t.Errorf("expected nil, got '%v' (%T)", captured[1], captured[1])
		}
	})

	t.Run("multiple converters", func(t *testing.T) {
		RegisterStringer(func(tm time.Time) string { return tm.Format("2006-01-02") })
		RegisterStringer(func(ts testStruct) string { return ts.Name + "-" + strconv.Itoa(ts.ID) })

		var captured []any
		RegisterCallback(slog.LevelInfo, func(msg string, args ...any) { captured = args })

		testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		testTS := testStruct{ID: 1, Name: "test"}
		Info("test", "timestamp", testTime, "data", testTS)

		if len(captured) != 4 {
			t.Fatalf("expected 4 arguments (two key-value pairs), got %d", len(captured))
		}
		if captured[1] != "2024-01-01" {
			t.Errorf("expected '2024-01-01', got '%v'", captured[1])
		}
		if captured[3] != "test-1" {
			t.Errorf("expected 'test-1', got '%v'", captured[3])
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		RegisterStringer(func(tm time.Time) string { return tm.Format("2006-01-02") })

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
				Info("test", "timestamp", testTime)
			}()
		}
		wg.Wait()
	})

	t.Run("mixed types", func(t *testing.T) {
		RegisterStringer(func(tm time.Time) string { return tm.Format("2006-01-02") })

		var captured []any
		RegisterCallback(slog.LevelInfo, func(msg string, args ...any) { captured = args })

		testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		Info("test", "timestamp", testTime, "message", "plain string", "count", 42, "enabled", true)

		if len(captured) != 8 {
			t.Fatalf("expected 8 arguments (four key-value pairs), got %d", len(captured))
		}
		if captured[1] != "2024-01-01" {
			t.Errorf("expected '2024-01-01', got '%v'", captured[1])
		}
		if captured[3] != "plain string" {
			t.Errorf("expected 'plain string', got '%v'", captured[3])
		}
		if captured[5] != 42 {
			t.Errorf("expected 42, got '%v'", captured[5])
		}
		if captured[7] != true {
			t.Errorf("expected true, got '%v'", captured[7])
		}
	})
}
