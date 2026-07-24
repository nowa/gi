package tools

import (
	"fmt"
	"math"
)

func requiredString(params map[string]any, name string) (string, error) {
	value, ok := params[name].(string)
	if !ok {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func optionalNumber(params map[string]any, name string) (float64, bool, error) {
	value, exists := params[name]
	if !exists || value == nil {
		return 0, false, nil
	}
	switch number := value.(type) {
	case float64:
		return number, true, nil
	case float32:
		return float64(number), true, nil
	case int:
		return float64(number), true, nil
	case int64:
		return float64(number), true, nil
	case int32:
		return float64(number), true, nil
	default:
		return 0, false, fmt.Errorf("%s must be a number", name)
	}
}

func optionalNonNegativeInt(params map[string]any, name string) (int, error) {
	number, exists, err := optionalNumber(params, name)
	if err != nil || !exists {
		return 0, err
	}
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("%s must be a finite number", name)
	}
	if number <= 0 {
		return 0, nil
	}
	return int(number), nil
}
