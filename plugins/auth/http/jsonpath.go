package http

import (
	"fmt"
	"strconv"
	"strings"
)

func ExtractValue(data map[string]interface{}, path string) interface{} {
	parts := parsePath(path)
	if len(parts) == 0 {
		return nil
	}

	var current interface{} = data

	for _, part := range parts {
		if current == nil {
			return nil
		}

		if part.isIndex {
			arr, ok := current.([]interface{})
			if !ok {
				return nil
			}
			if part.index < 0 || part.index >= len(arr) {
				return nil
			}
			current = arr[part.index]
		} else {
			m, ok := current.(map[string]interface{})
			if !ok {
				return nil
			}
			current, ok = m[part.key]
			if !ok {
				return nil
			}
		}
	}

	return current
}

func ExtractString(data map[string]interface{}, path string) (string, bool) {
	val := ExtractValue(data, path)
	if val == nil {
		return "", false
	}

	switch v := val.(type) {
	case string:
		return v, true
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), true
		}
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case int:
		return strconv.Itoa(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case bool:
		return strconv.FormatBool(v), true
	default:
		return fmt.Sprintf("%v", v), true
	}
}

type pathPart struct {
	key     string
	isIndex bool
	index   int
}

func parsePath(path string) []pathPart {
	if path == "" {
		return nil
	}

	var parts []pathPart
	remaining := path

	for len(remaining) > 0 {
		dotIdx := strings.Index(remaining, ".")
		bracketIdx := strings.Index(remaining, "[")

		if dotIdx < 0 && bracketIdx < 0 {
			if remaining != "" {
				parts = append(parts, pathPart{key: remaining})
			}
			break
		}

		nextSep := dotIdx
		if dotIdx < 0 || (bracketIdx >= 0 && bracketIdx < dotIdx) {
			nextSep = bracketIdx
		}

		if nextSep > 0 {
			parts = append(parts, pathPart{key: remaining[:nextSep]})
		}

		if nextSep == bracketIdx {
			closeBracket := strings.Index(remaining[nextSep:], "]")
			if closeBracket < 0 {
				break
			}
			closeBracket += nextSep

			indexStr := remaining[nextSep+1 : closeBracket]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				break
			}

			parts = append(parts, pathPart{isIndex: true, index: index})
			remaining = remaining[closeBracket+1:]

			if len(remaining) > 0 && remaining[0] == '.' {
				remaining = remaining[1:]
			}
		} else {
			remaining = remaining[nextSep+1:]
		}
	}

	return parts
}
