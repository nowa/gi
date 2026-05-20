package gicodingagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteToolPiBasics(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteTool(dir)

	testFile := filepath.Join(dir, "write-test.txt")
	result, err := tool.Execute("test-call-3", WriteToolInput{Path: testFile, Content: "Test content"})
	if err != nil {
		t.Fatal(err)
	}
	output := readToolText(result)
	if !strings.Contains(output, "Successfully wrote") || !strings.Contains(output, testFile) {
		t.Fatalf("write output = %q", output)
	}
	if content, err := os.ReadFile(testFile); err != nil || string(content) != "Test content" {
		t.Fatalf("written content = %q err=%v", content, err)
	}

	nestedFile := filepath.Join(dir, "nested", "dir", "test.txt")
	result, err = tool.Execute("test-call-4", WriteToolInput{Path: nestedFile, Content: "Nested content"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readToolText(result), "Successfully wrote") {
		t.Fatalf("nested write output = %q", readToolText(result))
	}
	if content, err := os.ReadFile(nestedFile); err != nil || string(content) != "Nested content" {
		t.Fatalf("nested content = %q err=%v", content, err)
	}
}

func TestEditToolRejectsEmptyEdits(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "edit-empty-edits.txt")
	if err := os.WriteFile(testFile, []byte("hello\nworld\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := NewEditTool(dir)
	_, err := tool.Execute("test-call-11", EditToolInput{Path: testFile, Edits: []Edit{}})
	if err == nil || !strings.Contains(err.Error(), "edits must contain at least one replacement") {
		t.Fatalf("empty edits err = %v", err)
	}
}
