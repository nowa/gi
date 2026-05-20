package gicodingagent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRPCJSONLSerializesStrictRecordsWithoutEscapingUnicodeSeparators(t *testing.T) {
	line, err := SerializeJSONLine(map[string]string{"text": "a\u2028b\u2029c"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "a\u2028b\u2029c") {
		t.Fatalf("line = %q, want literal unicode separators", line)
	}
	if !strings.HasSuffix(line, "\n") {
		t.Fatalf("line should end with LF: %q", line)
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, map[string]string{"text": "a\u2028b\u2029c"}) {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestRPCJSONLPreservesUnicodeSeparatorsAndSplitsOnLF(t *testing.T) {
	line, err := SerializeJSONLine(map[string]string{"text": "a\u2028b\u2029c"})
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{}
	if err := AttachJSONLLineReader(strings.NewReader(line), func(line string) {
		lines = append(lines, line)
	}); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("lines = %#v, want 1 line", lines)
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["text"] != "a\u2028b\u2029c" {
		t.Fatalf("decoded text = %q", decoded["text"])
	}
}

func TestRPCJSONLHandlesCRLFDelimitedInput(t *testing.T) {
	lines := []string{}
	err := AttachJSONLLineReader(strings.NewReader("{\"a\":1}\r\n{\"b\":2}\r\n"), func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lines, []string{`{"a":1}`, `{"b":2}`}) {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestRPCJSONLEmitsFinalLineWithoutTrailingLF(t *testing.T) {
	lines := []string{}
	err := AttachJSONLLineReader(strings.NewReader(`{"a":1}`), func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lines, []string{`{"a":1}`}) {
		t.Fatalf("lines = %#v", lines)
	}
}
