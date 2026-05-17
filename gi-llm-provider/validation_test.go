package gillmprovider

import (
	"reflect"
	"strings"
	"testing"
)

func toolCallWithPlainSchema(schema Schema, value any) (Tool, ContentPart) {
	tool := Tool{
		Name:        "echo",
		Description: "Echo tool",
		Parameters:  Object(map[string]Schema{"value": schema}, "value"),
	}
	toolCall := ToolCall("tool-1", "echo", map[string]any{"value": value})
	return tool, toolCall
}

func TestValidateToolArgumentsCoercesPlainJSONSchemaPrimitiveRules(t *testing.T) {
	tests := []struct {
		name     string
		schema   Schema
		input    any
		expected any
	}{
		{"number string", Number(), "42", float64(42)},
		{"number bool", Number(), true, float64(1)},
		{"number null", Number(), nil, float64(0)},
		{"integer string", Integer(), "42", 42},
		{"boolean true string", Boolean(), "true", true},
		{"boolean false string", Boolean(), "false", false},
		{"boolean one", Boolean(), 1, true},
		{"boolean zero", Boolean(), 0, false},
		{"string null", String(), nil, ""},
		{"string bool", String(), true, "true"},
		{"null empty string", Null(), "", nil},
		{"null zero", Null(), 0, nil},
		{"null false", Null(), false, nil},
		{"union keeps matching string", TypeUnion("number", "string"), "1", "1"},
		{"union coerces when no actual type matches", TypeUnion("boolean", "number"), "1", float64(1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, toolCall := toolCallWithPlainSchema(tt.schema, tt.input)
			got, err := ValidateToolArguments(tool, toolCall)
			if err != nil {
				t.Fatalf("ValidateToolArguments() error = %v", err)
			}
			if !reflect.DeepEqual(got["value"], tt.expected) {
				t.Fatalf("value = %#v (%T), want %#v (%T)", got["value"], got["value"], tt.expected, tt.expected)
			}
		})
	}
}

func TestValidateToolArgumentsRejectsInvalidCoercions(t *testing.T) {
	tests := []struct {
		name   string
		schema Schema
		input  any
	}{
		{"boolean one string", Boolean(), "1"},
		{"boolean zero string", Boolean(), "0"},
		{"null string null", Null(), "null"},
		{"integer decimal string", Integer(), "42.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, toolCall := toolCallWithPlainSchema(tt.schema, tt.input)
			_, err := ValidateToolArguments(tool, toolCall)
			if err == nil {
				t.Fatal("ValidateToolArguments() error = nil, want validation failure")
			}
			if !strings.Contains(strings.ToLower(err.Error()), "validation failed") {
				t.Fatalf("error = %q, want validation failed", err.Error())
			}
		})
	}
}
