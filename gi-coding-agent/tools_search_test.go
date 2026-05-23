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
	if result.Details == nil || result.Details.MatchLimitReached != 1 {
		t.Fatalf("grep limit details = %#v", result.Details)
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

func TestGrepToolPiPatternOptions(t *testing.T) {
	dir := t.TempDir()
	tool := NewGrepTool(dir)
	writeReadToolFile(t, filepath.Join(dir, "keep.go"), "Alpha\naxb\n")
	writeReadToolFile(t, filepath.Join(dir, "skip.txt"), "Alpha\naxb\n")
	writeReadToolFile(t, filepath.Join(dir, "ignored.go"), "Alpha\n")
	writeReadToolFile(t, filepath.Join(dir, ".gitignore"), "ignored.go\n")

	result, err := tool.Execute("test-call-grep-glob-ignore-case", GrepToolInput{
		Pattern:    "alpha",
		Path:       dir,
		Glob:       "*.go",
		IgnoreCase: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	output := readToolText(result)
	if !strings.Contains(output, "keep.go:1: Alpha") ||
		strings.Contains(output, "skip.txt") ||
		strings.Contains(output, "ignored.go") {
		t.Fatalf("grep glob/ignoreCase/gitignore output = %q", output)
	}

	regexResult, err := tool.Execute("test-call-grep-regex", GrepToolInput{Pattern: "a.b", Path: filepath.Join(dir, "keep.go")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readToolText(regexResult), "keep.go:2: axb") {
		t.Fatalf("grep regex output = %q", readToolText(regexResult))
	}

	literalResult, err := tool.Execute("test-call-grep-literal", GrepToolInput{Pattern: "a.b", Path: filepath.Join(dir, "keep.go"), Literal: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readToolText(literalResult), "No matches found") {
		t.Fatalf("grep literal output = %q", readToolText(literalResult))
	}

	if _, err := tool.Execute("test-call-grep-invalid-regex", GrepToolInput{Pattern: "[", Path: dir}); err == nil || !strings.Contains(err.Error(), "error parsing grep pattern") {
		t.Fatalf("invalid regex err = %v", err)
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

	emptyDir := filepath.Join(dir, "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err = ls.Execute("test-call-ls-empty", LsToolInput{Path: emptyDir})
	if err != nil {
		t.Fatal(err)
	}
	if output := strings.TrimSpace(readToolText(result)); output != "(empty directory)" {
		t.Fatalf("empty ls output = %q", output)
	}

	sortDir := filepath.Join(dir, "sort")
	if err := os.Mkdir(sortDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadToolFile(t, filepath.Join(sortDir, "b.txt"), "")
	writeReadToolFile(t, filepath.Join(sortDir, "A.txt"), "")
	result, err = ls.Execute("test-call-ls-sort", LsToolInput{Path: sortDir})
	if err != nil {
		t.Fatal(err)
	}
	if output := strings.TrimSpace(readToolText(result)); output != "A.txt\nb.txt" {
		t.Fatalf("case-insensitive ls output = %q", output)
	}
}

func TestFindAndLsToolLimitPiParity(t *testing.T) {
	dir := t.TempDir()
	writeReadToolFile(t, filepath.Join(dir, "a.txt"), "")
	writeReadToolFile(t, filepath.Join(dir, "b.txt"), "")
	writeReadToolFile(t, filepath.Join(dir, "c.txt"), "")

	find := NewFindTool(dir)
	findResult, err := find.Execute("test-call-find-limit", FindToolInput{Pattern: "*.txt", Path: dir, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	findOutput := readToolText(findResult)
	if !strings.Contains(findOutput, "a.txt") ||
		!strings.Contains(findOutput, "b.txt") ||
		strings.Contains(findOutput, "c.txt") ||
		!strings.Contains(findOutput, "[2 results limit reached. Use limit=4 for more, or refine pattern]") {
		t.Fatalf("find limit output = %q", findOutput)
	}
	if findResult.Details == nil || findResult.Details.ResultLimitReached != 2 {
		t.Fatalf("find limit details = %#v", findResult.Details)
	}

	ls := NewLsTool(dir)
	lsResult, err := ls.Execute("test-call-ls-limit", LsToolInput{Path: dir, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	lsOutput := readToolText(lsResult)
	if !strings.Contains(lsOutput, "a.txt") ||
		strings.Contains(lsOutput, "b.txt") ||
		!strings.Contains(lsOutput, "[1 entries limit reached. Use limit=2 for more]") {
		t.Fatalf("ls limit output = %q", lsOutput)
	}
	if lsResult.Details == nil || lsResult.Details.EntryLimitReached != 1 {
		t.Fatalf("ls limit details = %#v", lsResult.Details)
	}
}

func TestFindToolPathGlobPiRegressions(t *testing.T) {
	dir := t.TempDir()
	find := NewFindTool(dir)
	writeReadToolFile(t, filepath.Join(dir, "some", "parent", "child", "file.ext"), "")
	writeReadToolFile(t, filepath.Join(dir, "some", "parent", "child", "test.spec.ts"), "")
	writeReadToolFile(t, filepath.Join(dir, "src", "foo", "bar", "example.spec.ts"), "")

	assertFindContains := func(pattern string, wants ...string) {
		t.Helper()
		result, err := find.Execute("test-call-find-path-glob", FindToolInput{Pattern: pattern, Path: dir})
		if err != nil {
			t.Fatalf("find %q err = %v", pattern, err)
		}
		output := readToolText(result)
		for _, want := range wants {
			if !strings.Contains(output, want) {
				t.Fatalf("find %q output = %q, missing %q", pattern, output, want)
			}
		}
	}

	assertFindContains("*.spec.ts", "some/parent/child/test.spec.ts", "src/foo/bar/example.spec.ts")
	assertFindContains("some/parent/child/**", "some/parent/child/file.ext", "some/parent/child/test.spec.ts")
	assertFindContains("**/parent/child/*", "some/parent/child/file.ext", "some/parent/child/test.spec.ts")

	result, err := find.Execute("test-call-find-src-spec", FindToolInput{Pattern: "src/**/*.spec.ts", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	output := strings.TrimSpace(readToolText(result))
	if output != "src/foo/bar/example.spec.ts" {
		t.Fatalf("src glob output = %q", output)
	}
}

func TestFindToolNestedGitignorePiRegression(t *testing.T) {
	dir := t.TempDir()
	find := NewFindTool(dir)
	writeReadToolFile(t, filepath.Join(dir, "a", ".gitignore"), "ignored.txt\n")
	writeReadToolFile(t, filepath.Join(dir, "a", "ignored.txt"), "")
	writeReadToolFile(t, filepath.Join(dir, "a", "kept.txt"), "")
	writeReadToolFile(t, filepath.Join(dir, "b", "ignored.txt"), "")
	writeReadToolFile(t, filepath.Join(dir, "b", "kept.txt"), "")
	writeReadToolFile(t, filepath.Join(dir, "root.txt"), "")

	result, err := find.Execute("test-call-find-nested-ignore", FindToolInput{Pattern: "**/*.txt", Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	output := readToolText(result)
	for _, want := range []string{"a/kept.txt", "b/ignored.txt", "b/kept.txt", "root.txt"} {
		if !strings.Contains(output, want) {
			t.Fatalf("nested ignore output = %q, missing %q", output, want)
		}
	}
	if strings.Contains(output, "a/ignored.txt") {
		t.Fatalf("nested ignore leaked scoped rule output = %q", output)
	}

	deep := t.TempDir()
	find = NewFindTool(deep)
	writeReadToolFile(t, filepath.Join(deep, "a", ".gitignore"), "ignored.txt\n")
	writeReadToolFile(t, filepath.Join(deep, "a", "deep", ".gitignore"), "secret.txt\n")
	writeReadToolFile(t, filepath.Join(deep, "a", "ignored.txt"), "")
	writeReadToolFile(t, filepath.Join(deep, "a", "kept.txt"), "")
	writeReadToolFile(t, filepath.Join(deep, "a", "deep", "ignored.txt"), "")
	writeReadToolFile(t, filepath.Join(deep, "a", "deep", "secret.txt"), "")
	writeReadToolFile(t, filepath.Join(deep, "a", "deep", "kept.txt"), "")
	writeReadToolFile(t, filepath.Join(deep, "b", "ignored.txt"), "")
	writeReadToolFile(t, filepath.Join(deep, "b", "kept.txt"), "")
	writeReadToolFile(t, filepath.Join(deep, "root.txt"), "")
	result, err = find.Execute("test-call-find-deep-ignore", FindToolInput{Pattern: "**/*.txt", Path: deep})
	if err != nil {
		t.Fatal(err)
	}
	output = readToolText(result)
	for _, want := range []string{"a/deep/kept.txt", "a/kept.txt", "b/ignored.txt", "b/kept.txt", "root.txt"} {
		if !strings.Contains(output, want) {
			t.Fatalf("deep ignore output = %q, missing %q", output, want)
		}
	}
	for _, unwanted := range []string{"a/ignored.txt", "a/deep/ignored.txt", "a/deep/secret.txt"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("deep ignore output = %q, contains %q", output, unwanted)
		}
	}
}
