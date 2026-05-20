package gicodingagent

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEditToolDefinitionPiLegacyInput(t *testing.T) {
	definition := CreateEditToolDefinition(t.TempDir())
	if _, ok := definition.Parameters.Properties["oldText"]; ok {
		t.Fatal("public schema should not expose oldText")
	}
	if _, ok := definition.Parameters.Properties["newText"]; ok {
		t.Fatal("public schema should not expose newText")
	}

	prepared := definition.PrepareArguments(map[string]any{
		"path":    "file.txt",
		"oldText": "before",
		"newText": "after",
	})
	assertPreparedEditArgs(t, prepared, map[string]any{
		"path": "file.txt",
		"edits": []any{
			map[string]any{"oldText": "before", "newText": "after"},
		},
	})

	prepared = definition.PrepareArguments(map[string]any{
		"path": "file.txt",
		"edits": []any{
			map[string]any{"oldText": "a", "newText": "b"},
		},
		"oldText": "c",
		"newText": "d",
	})
	assertPreparedEditArgs(t, prepared, map[string]any{
		"path": "file.txt",
		"edits": []any{
			map[string]any{"oldText": "a", "newText": "b"},
			map[string]any{"oldText": "c", "newText": "d"},
		},
	})

	valid := map[string]any{
		"path": "file.txt",
		"edits": []any{
			map[string]any{"oldText": "a", "newText": "b"},
		},
	}
	if prepared = definition.PrepareArguments(valid); !reflect.DeepEqual(prepared, valid) {
		t.Fatalf("valid input prepared = %#v", prepared)
	}

	for _, input := range []any{nil, "garbage"} {
		if got := definition.PrepareArguments(input); got != input {
			t.Fatalf("PrepareArguments(%#v) = %#v", input, got)
		}
	}
}

func TestEditToolDefinitionPiPreparedArgsExecute(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "legacy.txt")
	if err := os.WriteFile(filePath, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	definition := CreateEditToolDefinition(dir)
	prepared := definition.PrepareArguments(map[string]any{
		"path":    "legacy.txt",
		"oldText": "before",
		"newText": "after",
	})
	result, err := definition.Execute("tool-1", prepared)
	if err != nil {
		t.Fatal(err)
	}
	if got := readToolText(result); got != "Successfully replaced 1 block(s) in legacy.txt." {
		t.Fatalf("result text = %q", got)
	}
	if content, err := os.ReadFile(filePath); err != nil || string(content) != "after\n" {
		t.Fatalf("content = %q err=%v", content, err)
	}
}

func TestEditToolDefinitionPiStringifiedEdits(t *testing.T) {
	definition := CreateEditToolDefinition(t.TempDir())
	prepared := definition.PrepareArguments(map[string]any{
		"path":  "file.txt",
		"edits": `[{"oldText":"a","newText":"b"}]`,
	})
	assertPreparedEditArgs(t, prepared, map[string]any{
		"path": "file.txt",
		"edits": []any{
			map[string]any{"oldText": "a", "newText": "b"},
		},
	})

	invalid := map[string]any{"path": "file.txt", "edits": "not json"}
	if prepared = definition.PrepareArguments(invalid); !reflect.DeepEqual(prepared, invalid) {
		t.Fatalf("invalid json prepared = %#v", prepared)
	}
}

func assertPreparedEditArgs(t *testing.T, got any, want map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prepared = %#v, want %#v", got, want)
	}
}
