package gologger

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"strings"
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

func mapSlice[T, U any](slice []T, mapper func(T) U) []U {
	result := make([]U, len(slice))
	for idx, item := range slice {
		result[idx] = mapper(item)
	}
	return result
}
