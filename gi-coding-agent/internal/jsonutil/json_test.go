package jsonutil

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestStripCommentsAndTrailingCommas(t *testing.T) {
	input := `{
		// provider comment
		"url": "https://example.test/a//b",
		"markers": [",}", ",]", "quote: \" // literal",],
		"nested": {
			"enabled": true, // trailing comment
		},
	}`
	stripped := StripCommentsAndTrailingCommas(input)
	var value map[string]any
	if err := json.Unmarshal([]byte(stripped), &value); err != nil {
		t.Fatalf("stripped JSON is invalid: %v\n%s", err, stripped)
	}
	if value["url"] != "https://example.test/a//b" {
		t.Fatalf("URL literal = %#v", value["url"])
	}
	if !reflect.DeepEqual(
		value["markers"],
		[]any{",}", ",]", `quote: " // literal`},
	) {
		t.Fatalf("string literals = %#v", value["markers"])
	}
	nested, ok := value["nested"].(map[string]any)
	if !ok || nested["enabled"] != true {
		t.Fatalf("nested value = %#v", value["nested"])
	}
}

func TestStripCommentsAndTrailingCommasPreservesLayout(t *testing.T) {
	input := "{\r\n  \"value\": 1, // comment\r\n}\n// eof"
	got := StripCommentsAndTrailingCommas(input)
	want := "{\r\n  \"value\": 1 \n}\n"
	if got != want {
		t.Fatalf("stripped JSON = %q, want %q", got, want)
	}
}

func TestStripCommentsAndTrailingCommasLeavesOrdinaryCommas(t *testing.T) {
	input := `{"first": 1, "second": 2}`
	if got := StripCommentsAndTrailingCommas(input); got != input {
		t.Fatalf("ordinary JSON changed to %q", got)
	}
}
