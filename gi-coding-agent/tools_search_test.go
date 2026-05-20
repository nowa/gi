package gicodingagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepToolPiBasics(t *testing.T) {
	dir := t.TempDir()
	tool := NewGrepTool(dir)

	testFile := filepath.Join(dir, "example.txt")
	writeReadToolFile(t, testFile, "first line\nmatch line\nlast line")
	result, err := tool.Execute("test-call-11", GrepToolInput{Pattern: "match", Path: testFile})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readToolText(result), "example.txt:2: match line") {
		t.Fatalf("grep output = %q", readToolText(result))
	}

	contextFile := filepath.Join(dir, "context.txt")
	writeReadToolFile(t, contextFile, strings.Join([]string{"before", "match one", "after", "middle", "match two", "after two"}, "\n"))
	result, err = tool.Execute("test-call-12", GrepToolInput{Pattern: "match", Path: contextFile, Limit: 1, Context: 1})
	if err != nil {
		t.Fatal(err)
	}
	output := readToolText(result)
	for _, want := range []string{"context.txt-1- before", "context.txt:2: match one", "context.txt-3- after", "[1 matches limit reached. Use limit=2 for more, or refine pattern]"} {
		if !strings.Contains(output, want) {
			t.Fatalf("grep context output = %q, missing %q", output, want)
		}
	}
	if strings.Contains(output, "match two") {
		t.Fatalf("grep output included second match: %q", output)
	}

	payload := filepath.Join(dir, "payload.sh")
	marker := filepath.Join(dir, "grep-injection-marker")
	target := filepath.Join(dir, "target.txt")
	writeReadToolFile(t, payload, "#!/bin/sh\necho executed > "+marker+"\ncat \"$1\"\n")
	if err := os.Chmod(payload, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadToolFile(t, target, "target\n")
	result, err = tool.Execute("test-call-grep-injection", GrepToolInput{Pattern: "--pre=" + payload, Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readToolText(result), "No matches found") {
		t.Fatalf("flag-like grep output = %q", readToolText(result))
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("grep executed payload, marker err=%v", err)
	}
}

func TestFindAndLsToolPiBasics(t *testing.T) {
	dir := t.TempDir()
	find := NewFindTool(dir)
	ls := NewLsTool(dir)

	hiddenDir := filepath.Join(dir, ".secret")
	if err := os.Mkdir(hiddenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadToolFile(t, filepath.Join(hiddenDir, "hidden.txt"), "hidden")
	writeReadToolFile(t, filepath.Join(dir, "visible.txt"), "visible")
	result, err := find.Execute("test-call-13", FindToolInput{Pattern: "**/*.txt", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	output := readToolText(result)
	if !strings.Contains(output, "visible.txt") || !strings.Contains(output, ".secret/hidden.txt") {
		t.Fatalf("find hidden output = %q", output)
	}

	writeReadToolFile(t, filepath.Join(dir, ".gitignore"), "ignored.txt\n")
	writeReadToolFile(t, filepath.Join(dir, "ignored.txt"), "ignored")
	writeReadToolFile(t, filepath.Join(dir, "kept.txt"), "kept")
	result, err = find.Execute("test-call-14", FindToolInput{Pattern: "**/*.txt", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	output = readToolText(result)
	if !strings.Contains(output, "kept.txt") || strings.Contains(output, "ignored.txt") {
		t.Fatalf("find gitignore output = %q", output)
	}

	if _, err := find.Execute("test-call-15", FindToolInput{Pattern: "[", Path: dir}); err == nil || !strings.Contains(err.Error(), "error parsing glob") {
		t.Fatalf("bad glob err = %v", err)
	}

	result, err = find.Execute("test-call-find-flag-pattern", FindToolInput{Pattern: "--help", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readToolText(result), "No files found matching pattern") {
		t.Fatalf("flag-like find output = %q", readToolText(result))
	}

	writeReadToolFile(t, filepath.Join(dir, ".hidden-file"), "secret")
	if err := os.Mkdir(filepath.Join(dir, ".hidden-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err = ls.Execute("test-call-15", LsToolInput{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	output = readToolText(result)
	if !strings.Contains(output, ".hidden-file") || !strings.Contains(output, ".hidden-dir/") {
		t.Fatalf("ls output = %q", output)
	}
}
