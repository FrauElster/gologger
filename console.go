package gologger

import "log/slog"

func addSlogCallbacks() {
	OnInfo(func(message string, additionalValues map[string]any) {
		slog.Info(message, mapAdditionalValues(additionalValues)...)
	})
	OnWarn(func(message string, additionalValues map[string]any) {
		slog.Warn(message, mapAdditionalValues(additionalValues)...)
	})
	OnErr(func(message string, additionalValues map[string]any) {
		slog.Error(message, mapAdditionalValues(additionalValues)...)
	})
}

func mapAdditionalValues(values map[string]any) []any {
	result := make([]any, 0)
	for key, value := range values {
		result = append(result, key)
		result = append(result, value)
	}
	return result
}
