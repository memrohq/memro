package dce

import (
	"fmt"
	"strings"
)

// ExtractValue extracts a value from a nested map or slice using dot notation / JSONPath.
// Supports paths like "$.id", "id", "data.issues", "$.creator.name".
func ExtractValue(data any, path string) (any, bool) {
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "$")
	if path == "" {
		return data, true
	}

	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		if part == "" {
			continue
		}
		switch val := current.(type) {
		case map[string]any:
			v, ok := val[part]
			if !ok {
				return nil, false
			}
			current = v
		default:
			return nil, false
		}
	}

	return current, true
}

// ExtractString extracts a string value from data.
func ExtractString(data any, path string) string {
	v, ok := ExtractValue(data, path)
	if !ok || v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case fmt.Stringer:
		return s.String()
	default:
		return fmt.Sprintf("%v", s)
	}
}

// ExtractSlice extracts a slice of items from data (e.g., "data.issues" or "items").
func ExtractSlice(data any, path string) ([]any, bool) {
	v, ok := ExtractValue(data, path)
	if !ok || v == nil {
		return nil, false
	}
	if slice, ok := v.([]any); ok {
		return slice, true
	}
	return nil, false
}
