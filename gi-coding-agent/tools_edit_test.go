package gicodingagent

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestEditToolPiExactMatching(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditTool(dir)

	testFile := filepath.Join(dir, "edit-test.txt")
	writeEditToolFile(t, testFile, "Hello, world!")
	result, err := tool.Execute("test-call-5", EditToolInput{
		Path:  testFile,
		Edits: []Edit{{OldText: "world", NewText: "testing"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readToolText(result), "Successfully replaced") || result.Details == nil || !strings.Contains(result.Details.Diff, "testing") {
		t.Fatalf("edit result = %#v", result)
	}
	if content := readEditToolFile(t, testFile); content != "Hello, testing!" {
		t.Fatalf("content = %q", content)
	}

	writeEditToolFile(t, testFile, "Hello, world!")
	_, err = tool.Execute("test-call-6", EditToolInput{
		Path:  testFile,
		Edits: []Edit{{OldText: "nonexistent", NewText: "testing"}},
	})
	if err == nil || !strings.Contains(err.Error(), "Could not find the exact text") {
		t.Fatalf("not found err = %v", err)
	}

	missingFile := filepath.Join(dir, "missing.txt")
	_, err = tool.Execute("test-call-6b", EditToolInput{
		Path:  missingFile,
		Edits: []Edit{{OldText: "hello", NewText: "world"}},
	})
	if err == nil || err.Error() != "Could not edit file: "+missingFile+". Error code: ENOENT." {
		t.Fatalf("missing err = %v", err)
	}

	writeEditToolFile(t, testFile, "foo foo foo")
	_, err = tool.Execute("test-call-7", EditToolInput{
		Path:  testFile,
		Edits: []Edit{{OldText: "foo", NewText: "bar"}},
	})
	if err == nil || !strings.Contains(err.Error(), "Found 3 occurrences") {
		t.Fatalf("duplicate err = %v", err)
	}
}

func TestEditToolPiMultiEditAtomicity(t *testing.T) {
	dir := t.TempDir()
	tool := NewEditTool(dir)

	multiFile := filepath.Join(dir, "edit-multi.txt")
	writeEditToolFile(t, multiFile, "alpha\nbeta\ngamma\ndelta\n")
	result, err := tool.Execute("test-call-8", EditToolInput{
		Path: multiFile,
		Edits: []Edit{
			{OldText: "alpha\n", NewText: "ALPHA\n"},
			{OldText: "gamma\n", NewText: "GAMMA\n"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readToolText(result), "Successfully replaced 2 block(s)") {
		t.Fatalf("multi edit output = %q", readToolText(result))
	}
	if content := readEditToolFile(t, multiFile); content != "ALPHA\nbeta\nGAMMA\ndelta\n" {
		t.Fatalf("multi content = %q", content)
	}
	if result.Details == nil || !strings.Contains(result.Details.Diff, "ALPHA") || !strings.Contains(result.Details.Diff, "GAMMA") {
		t.Fatalf("multi diff = %#v", result.Details)
	}

	gapFile := filepath.Join(dir, "edit-multi-large-gap.txt")
	lines := make([]string, 600)
	for i := range lines {
		lines[i] = "line " + leftPad3(i+1)
	}
	writeEditToolFile(t, gapFile, strings.Join(lines, "\n")+"\n")
	result, err = tool.Execute("test-call-8b", EditToolInput{
		Path: gapFile,
		Edits: []Edit{
			{OldText: "line 100\n", NewText: "LINE 100\n"},
			{OldText: "line 300\n", NewText: "LINE 300\n"},
			{OldText: "line 500\n", NewText: "LINE 500\n"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	diff := ""
	if result.Details != nil {
		diff = result.Details.Diff
	}
	if !strings.Contains(diff, "LINE 100") || !strings.Contains(diff, "LINE 300") || !strings.Contains(diff, "LINE 500") || !strings.Contains(diff, "...") || strings.Contains(diff, "line 250") || len(strings.Split(diff, "\n")) >= 50 {
		t.Fatalf("collapsed diff = %q", diff)
	}

	originalFile := filepath.Join(dir, "edit-multi-original.txt")
	writeEditToolFile(t, originalFile, "foo\nbar\nbaz\n")
	_, err = tool.Execute("test-call-9", EditToolInput{
		Path: originalFile,
		Edits: []Edit{
			{OldText: "foo\n", NewText: "foo bar\n"},
			{OldText: "bar\n", NewText: "BAR\n"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if content := readEditToolFile(t, originalFile); content != "foo bar\nBAR\nbaz\n" {
		t.Fatalf("original matching content = %q", content)
	}

	overlapFile := filepath.Join(dir, "edit-overlap.txt")
	writeEditToolFile(t, overlapFile, "one\ntwo\nthree\n")
	_, err = tool.Execute("test-call-12", EditToolInput{
		Path: overlapFile,
		Edits: []Edit{
			{OldText: "one\ntwo\n", NewText: "ONE\nTWO\n"},
			{OldText: "two\nthree\n", NewText: "TWO\nTHREE\n"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("overlap err = %v", err)
	}

	noPartialFile := filepath.Join(dir, "edit-no-partial.txt")
	originalContent := "alpha\nbeta\ngamma\n"
	writeEditToolFile(t, noPartialFile, originalContent)
	_, err = tool.Execute("test-call-13", EditToolInput{
		Path: noPartialFile,
		Edits: []Edit{
			{OldText: "alpha\n", NewText: "ALPHA\n"},
			{OldText: "missing\n", NewText: "MISSING\n"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "Could not find") {
		t.Fatalf("no partial err = %v", err)
	}
	if content := readEditToolFile(t, noPartialFile); content != originalContent {
		t.Fatalf("partial content = %q", content)
	}
}

func writeEditToolFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readEditToolFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func leftPad3(value int) string {
	text := strconv.Itoa(value)
	for len(text) < 3 {
		text = "0" + text
	}
	return text
}
