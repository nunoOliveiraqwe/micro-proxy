package middleware

import (
	"fmt"
	"strconv"

	"go.uber.org/zap"
)

func ParseStringOpt(opts map[string]interface{}, key string, defaultVal string) (string, error) {
	raw, ok := opts[key]
	if !ok || raw == nil {
		return defaultVal, nil
	}
	switch v := raw.(type) {
	case string:
		return v, nil
	default:
		return "", fmt.Errorf("'%s' option must be a string, got %T", key, raw)
	}
}

func ParseStringRequired(opts map[string]interface{}, key string) (string, error) {
	raw, ok := opts[key]
	if !ok || raw == nil {
		return "", fmt.Errorf("missing required option '%s'", key)
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("'%s' option must be a string, got %T", key, raw)
	}
	if s == "" {
		return "", fmt.Errorf("'%s' option cannot be empty", key)
	}
	return s, nil
}

func ParseBoolOpt(opts map[string]interface{}, key string, defaultVal bool) bool {
	raw, ok := opts[key]
	if !ok || raw == nil {
		return defaultVal
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			zap.S().Warnf("config_parser: invalid '%s' value %q, using default %v", key, v, defaultVal)
			return defaultVal
		}
		return parsed
	default:
		zap.S().Warnf("config_parser: '%s' option has unexpected type %T, using default %v", key, raw, defaultVal)
		return defaultVal
	}
}

func ParseIntOpt(opts map[string]interface{}, key string, defaultValue int) int {
	raw, ok := opts[key]
	if !ok || raw == nil {
		return defaultValue
	}
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return defaultValue
	}
}

func ParseIntOptRequired(opts map[string]interface{}, key string) (int, error) {
	raw, ok := opts[key]
	if !ok || raw == nil {
		return 0, fmt.Errorf("missing required option '%s'", key)
	}
	switch v := raw.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("'%s' option must be a number, got %T", key, raw)
	}
}

func ParseFloatOptRequired(opts map[string]interface{}, key string) (float64, error) {
	raw, ok := opts[key]
	if !ok || raw == nil {
		return 0, fmt.Errorf("missing required option '%s'", key)
	}
	switch v := raw.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("'%s' option must be a number, got %T", key, raw)
	}
}

func ParseStringSliceOpt(opts map[string]interface{}, key string, defaultVal []string) ([]string, error) {
	raw, ok := opts[key]
	if !ok || raw == nil {
		return defaultVal, nil
	}
	slice, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("'%s' option must be an array of strings, got %T", key, raw)
	}
	result := make([]string, 0, len(slice))
	for _, item := range slice {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("each entry in '%s' must be a string, got %T", key, item)
		}
		result = append(result, s)
	}
	return result, nil
}

func ParseRawSliceRequired(opts map[string]interface{}, key string) ([]interface{}, error) {
	raw, ok := opts[key]
	if !ok || raw == nil {
		return nil, fmt.Errorf("missing required option '%s'", key)
	}
	slice, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("'%s' option must be an array of interface, got %T", key, raw)
	}
	return slice, nil
}
