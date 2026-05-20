package gicodingagent

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestReadToolPiLimitsAndOffsets(t *testing.T) {
	dir := t.TempDir()
	tool := NewReadTool(dir)

	testFile := filepath.Join(dir, "test.txt")
	content := "Hello, world!\nLine 2\nLine 3"
	writeReadToolFile(t, testFile, content)
	result, err := tool.Execute("test-call-1", ReadToolInput{Path: testFile})
	if err != nil {
		t.Fatal(err)
	}
	if got := readToolText(result); got != content {
		t.Fatalf("read text = %q, want %q", got, content)
	}
	if strings.Contains(readToolText(result), "Use offset=") || result.Details != nil {
		t.Fatalf("unexpected truncation: text=%q details=%#v", readToolText(result), result.Details)
	}

	if _, err := tool.Execute("test-call-2", ReadToolInput{Path: filepath.Join(dir, "nonexistent.txt")}); err == nil || !regexp.MustCompile(`ENOENT|not found|no such file|does not exist`).MatchString(err.Error()) {
		t.Fatalf("missing file err = %v", err)
	}

	largeFile := filepath.Join(dir, "large.txt")
	lines := make([]string, 2500)
	for i := range lines {
		lines[i] = "Line " + itoa(i+1)
	}
	writeReadToolFile(t, largeFile, strings.Join(lines, "\n"))
	result, err = tool.Execute("test-call-3", ReadToolInput{Path: largeFile})
	if err != nil {
		t.Fatal(err)
	}
	output := readToolText(result)
	if !strings.Contains(output, "Line 1") || !strings.Contains(output, "Line 2000") || strings.Contains(output, "Line 2001") {
		t.Fatalf("line-limited output missing expected bounds")
	}
	if !strings.Contains(output, "[Showing lines 1-2000 of 2500. Use offset=2001 to continue.]") {
		t.Fatalf("line-limit message missing: %q", output)
	}

	byteFile := filepath.Join(dir, "large-bytes.txt")
	byteLines := make([]string, 500)
	for i := range byteLines {
		byteLines[i] = "Line " + itoa(i+1) + ": " + strings.Repeat("x", 200)
	}
	writeReadToolFile(t, byteFile, strings.Join(byteLines, "\n"))
	result, err = tool.Execute("test-call-4", ReadToolInput{Path: byteFile})
	if err != nil {
		t.Fatal(err)
	}
	output = readToolText(result)
	if !strings.Contains(output, "Line 1:") || !regexp.MustCompile(`\[Showing lines 1-\d+ of 500 \(byte limit\)\. Use offset=\d+ to continue\.\]`).MatchString(output) {
		t.Fatalf("byte-limit output = %q", output)
	}

	offsetFile := filepath.Join(dir, "offset-test.txt")
	hundredLines := make([]string, 100)
	for i := range hundredLines {
		hundredLines[i] = "Line " + itoa(i+1)
	}
	writeReadToolFile(t, offsetFile, strings.Join(hundredLines, "\n"))
	result, err = tool.Execute("test-call-5", ReadToolInput{Path: offsetFile, Offset: 51})
	if err != nil {
		t.Fatal(err)
	}
	output = readToolText(result)
	if strings.Contains(output, "Line 50") || !strings.Contains(output, "Line 51") || !strings.Contains(output, "Line 100") || strings.Contains(output, "Use offset=") {
		t.Fatalf("offset output = %q", output)
	}

	result, err = tool.Execute("test-call-6", ReadToolInput{Path: offsetFile, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	output = readToolText(result)
	if !strings.Contains(output, "Line 1") || !strings.Contains(output, "Line 10") || strings.Contains(output, "Line 11") || !strings.Contains(output, "[90 more lines in file. Use offset=11 to continue.]") {
		t.Fatalf("limit output = %q", output)
	}

	result, err = tool.Execute("test-call-7", ReadToolInput{Path: offsetFile, Offset: 41, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	output = readToolText(result)
	if strings.Contains(output, "Line 40") || !strings.Contains(output, "Line 41") || !strings.Contains(output, "Line 60") || strings.Contains(output, "Line 61") || !strings.Contains(output, "[40 more lines in file. Use offset=61 to continue.]") {
		t.Fatalf("offset+limit output = %q", output)
	}

	shortFile := filepath.Join(dir, "short.txt")
	writeReadToolFile(t, shortFile, "Line 1\nLine 2\nLine 3")
	if _, err := tool.Execute("test-call-8", ReadToolInput{Path: shortFile, Offset: 100}); err == nil || !strings.Contains(err.Error(), "Offset 100 is beyond end of file (3 lines total)") {
		t.Fatalf("offset beyond err = %v", err)
	}

	result, err = tool.Execute("test-call-9", ReadToolInput{Path: largeFile})
	if err != nil {
		t.Fatal(err)
	}
	if result.Details == nil || result.Details.Truncation == nil {
		t.Fatalf("truncation details missing: %#v", result.Details)
	}
	truncation := result.Details.Truncation
	if !truncation.Truncated || truncation.TruncatedBy != "lines" || truncation.TotalLines != 2500 || truncation.OutputLines != 2000 {
		t.Fatalf("truncation = %#v", truncation)
	}
}

func TestReadToolDetectsImageMagic(t *testing.T) {
	dir := t.TempDir()
	tool := NewReadTool(dir)

	imageAsText := filepath.Join(dir, "image.txt")
	if err := os.WriteFile(imageAsText, mustDecodeBase64(t, tinyPNGBase64), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := tool.Execute("test-call-img-1", ReadToolInput{Path: imageAsText})
	if err != nil {
		t.Fatal(err)
	}
	if result.Content[0].Type != llm.ContentText || !strings.Contains(readToolText(result), "Read image file [image/png]") || !hasContentPartType(result.Content, llm.ContentImage) {
		t.Fatalf("image magic result = %#v", result.Content)
	}

	notImage := filepath.Join(dir, "not-an-image.png")
	writeReadToolFile(t, notImage, "definitely not a png")
	result, err = tool.Execute("test-call-img-2", ReadToolInput{Path: notImage})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readToolText(result), "definitely not a png") || hasContentPartType(result.Content, llm.ContentImage) {
		t.Fatalf("fake image result = %#v", result.Content)
	}
}

func readToolText(result FileToolResult) string {
	var parts []string
	for _, part := range result.Content {
		if part.Type == llm.ContentText {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func writeReadToolFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
