package gologger

import (
	"fmt"
	"time"
)

func sliceContains[I comparable](slice []I, item I) bool {
	for _, i := range slice {
		if i == item {
			return true
		}
	}
	return false
}

func formatAdditionalValues(additionalValues map[string]any) map[string]any {
	for key, value := range additionalValues {
		switch v := value.(type) {
		case error:
			additionalValues[key] = v.Error()
		case time.Duration:
			additionalValues[key] = formatDuration(v)
		case fmt.Stringer:
			additionalValues[key] = v.String()
		}
	}

	return additionalValues
}
