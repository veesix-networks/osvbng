package conf

import (
	"encoding/json"
	"fmt"
	"strconv"
)

func ParseUint32(v interface{}) (uint32, error) {
	switch val := v.(type) {
	case string:
		i, err := strconv.ParseUint(val, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid uint32 string: %w", err)
		}
		return uint32(i), nil
	case uint32:
		return val, nil
	case json.Number:
		i, err := val.Int64()
		if err != nil {
			return 0, fmt.Errorf("invalid number: %w", err)
		}
		if i < 0 || i > 4294967295 {
			return 0, fmt.Errorf("value out of range for uint32")
		}
		return uint32(i), nil
	case int64:
		if val < 0 || val > 4294967295 {
			return 0, fmt.Errorf("value out of range for uint32")
		}
		return uint32(val), nil
	case int:
		if val < 0 {
			return 0, fmt.Errorf("value out of range for uint32")
		}
		return uint32(val), nil
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
}

func ParseUint16(v interface{}) (uint16, error) {
	switch val := v.(type) {
	case string:
		i, err := strconv.ParseUint(val, 10, 16)
		if err != nil {
			return 0, fmt.Errorf("invalid uint16 string: %w", err)
		}
		return uint16(i), nil
	case uint16:
		return val, nil
	case uint32:
		if val > 65535 {
			return 0, fmt.Errorf("value out of range for uint16")
		}
		return uint16(val), nil
	case json.Number:
		i, err := val.Int64()
		if err != nil {
			return 0, fmt.Errorf("invalid number: %w", err)
		}
		if i < 0 || i > 65535 {
			return 0, fmt.Errorf("value out of range for uint16")
		}
		return uint16(i), nil
	case int64:
		if val < 0 || val > 65535 {
			return 0, fmt.Errorf("value out of range for uint16")
		}
		return uint16(val), nil
	case int:
		if val < 0 || val > 65535 {
			return 0, fmt.Errorf("value out of range for uint16")
		}
		return uint16(val), nil
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
}

func ParseInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case string:
		i, err := strconv.Atoi(val)
		if err != nil {
			return 0, fmt.Errorf("invalid int string: %w", err)
		}
		return i, nil
	case int:
		return val, nil
	case json.Number:
		i, err := val.Int64()
		if err != nil {
			return 0, fmt.Errorf("invalid number: %w", err)
		}
		return int(i), nil
	case int64:
		return int(val), nil
	case uint32:
		return int(val), nil
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
}
