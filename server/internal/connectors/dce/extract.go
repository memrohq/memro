package dce

import (
	"fmt"
	"strconv"
	"strings"
)

// ExtractValue extracts a value from a nested map or slice using dot notation.
// Supports "$.id", "id", "data.issues", "$.creator.name" and numeric indexes
// ("labels.0.name").
func ExtractValue(data any, path string) (any, bool) {
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "$")
	if path == "" {
		return data, true
	}
	current := data
	for _, part := range strings.Split(path, ".") {
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
		case []any:
			i, err := strconv.Atoi(part)
			if err == nil && i < 0 {
				i += len(val) // "-1" is the last element
			}
			if err != nil || i < 0 || i >= len(val) {
				return nil, false
			}
			current = val[i]
		default:
			return nil, false
		}
	}
	return current, true
}

// ExtractString extracts a value as text. "a[].b" joins b over the array a
// with ", ". Whole numbers print without exponents, so large IDs stay exact.
func ExtractString(data any, path string) string {
	if before, after, ok := strings.Cut(path, "[]"); ok {
		list, ok := ExtractSlice(data, before)
		if !ok {
			return ""
		}
		var parts []string
		for _, el := range list {
			if s := ExtractString(el, strings.TrimPrefix(after, ".")); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	}
	v, ok := ExtractValue(data, path)
	if !ok || v == nil {
		return ""
	}
	return stringOf(v)
}

func stringOf(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(s)
	case fmt.Stringer:
		return s.String()
	case map[string]any, []any:
		return "" // structures are not text; map their fields instead
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
