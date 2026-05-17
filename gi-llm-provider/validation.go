package gillmprovider

import (
	"fmt"
	"math"
	"strconv"
)

func ValidateToolArguments(tool Tool, toolCall ContentPart) (map[string]any, error) {
	if toolCall.Type != ContentToolCall {
		return nil, fmt.Errorf("validation failed: content is not a tool call")
	}
	value, err := coerceSchema(tool.Parameters, toolCall.Arguments)
	if err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("validation failed: parameters must be object")
	}
	return result, nil
}

func coerceSchema(schema Schema, value any) (any, error) {
	types := schemaTypes(schema.Type)
	if len(types) == 0 {
		return value, nil
	}
	if len(types) > 1 {
		actual := actualSchemaType(value)
		for _, schemaType := range types {
			if schemaType == actual || (schemaType == "number" && actual == "integer") {
				return value, nil
			}
		}
	}

	var lastErr error
	for _, schemaType := range types {
		coerced, err := coerceSingleType(schemaType, schema, value)
		if err == nil {
			return coerced, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func actualSchemaType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case int, int8, int16, int32, int64:
		return "integer"
	case float32, float64:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return ""
	}
}

func schemaTypes(raw any) []string {
	switch typed := raw.(type) {
	case string:
		return []string{typed}
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

func coerceSingleType(schemaType string, schema Schema, value any) (any, error) {
	switch schemaType {
	case "object":
		m, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected object")
		}
		result := make(map[string]any, len(m))
		for key, v := range m {
			result[key] = v
		}
		for _, key := range schema.Required {
			if _, ok := result[key]; !ok {
				return nil, fmt.Errorf("missing required property %q", key)
			}
		}
		for key, propertySchema := range schema.Properties {
			if v, ok := result[key]; ok {
				coerced, err := coerceSchema(propertySchema, v)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", key, err)
				}
				result[key] = coerced
			}
		}
		return result, nil
	case "number":
		return coerceNumber(value, false)
	case "integer":
		return coerceNumber(value, true)
	case "boolean":
		return coerceBoolean(value)
	case "string":
		return coerceString(value)
	case "null":
		return coerceNull(value)
	case "array":
		values, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("expected array")
		}
		if schema.Items == nil {
			return values, nil
		}
		result := make([]any, len(values))
		for i, item := range values {
			coerced, err := coerceSchema(*schema.Items, item)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			result[i] = coerced
		}
		return result, nil
	default:
		return value, nil
	}
}

func coerceNumber(value any, integer bool) (any, error) {
	var number float64
	switch v := value.(type) {
	case int:
		number = float64(v)
	case int64:
		number = float64(v)
	case float64:
		number = v
	case float32:
		number = float64(v)
	case bool:
		if v {
			number = 1
		}
	case nil:
		number = 0
	case string:
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("expected number")
		}
		number = parsed
	default:
		return nil, fmt.Errorf("expected number")
	}
	if integer {
		if math.Trunc(number) != number {
			return nil, fmt.Errorf("expected integer")
		}
		return int(number), nil
	}
	return number, nil
}

func coerceBoolean(value any) (any, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case int:
		if v == 0 {
			return false, nil
		}
		if v == 1 {
			return true, nil
		}
	case float64:
		if v == 0 {
			return false, nil
		}
		if v == 1 {
			return true, nil
		}
	case string:
		if v == "true" {
			return true, nil
		}
		if v == "false" {
			return false, nil
		}
	}
	return nil, fmt.Errorf("expected boolean")
}

func coerceString(value any) (any, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case int:
		return strconv.Itoa(v), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		return fmt.Sprint(v), nil
	}
}

func coerceNull(value any) (any, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case string:
		if v == "" {
			return nil, nil
		}
	case int:
		if v == 0 {
			return nil, nil
		}
	case float64:
		if v == 0 {
			return nil, nil
		}
	case bool:
		if !v {
			return nil, nil
		}
	}
	return nil, fmt.Errorf("expected null")
}
