package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	core "github.com/nowa/gi/gi-agent-core"
	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	harnessenv "github.com/nowa/gi/gi-agent-core/harness/env"
	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestReadToolOffsetsLimitsAndTruncation(t *testing.T) {
	ctx := context.Background()
	toolContext := newTestExecutionContext(t)
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("Line %d", i+1)
	}
	mustWriteFile(t, toolContext.Env, "test.txt", strings.Join(lines, "\n"))

	result, err := executeHarnessTool(ctx, CreateReadTool(), map[string]any{
		"path":   "test.txt",
		"offset": 41,
		"limit":  20,
	}, toolContext)
	if err != nil {
		t.Fatal(err)
	}
	output := textOutput(result)
	if strings.Contains(output, "Line 40") ||
		!strings.Contains(output, "Line 41") ||
		!strings.Contains(output, "Line 60") ||
		strings.Contains(output, "Line 61") ||
		!strings.Contains(output, "[40 more lines in file. Use offset=61 to continue.]") {
		t.Fatalf("offset output = %q", output)
	}

	largeLines := make([]string, 2500)
	for i := range largeLines {
		largeLines[i] = fmt.Sprintf("Line %d", i+1)
	}
	mustWriteFile(t, toolContext.Env, "large.txt", strings.Join(largeLines, "\n"))
	result, err = executeHarnessTool(ctx, CreateReadTool(), map[string]any{"path": "large.txt"}, toolContext)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(textOutput(result), "[Showing lines 1-2000 of 2500. Use offset=2001 to continue.]") {
		t.Fatalf("truncation output = %q", textOutput(result))
	}
	details, ok := result.Details.(*ReadToolDetails)
	if !ok || details.Truncation == nil ||
		!details.Truncation.Truncated ||
		details.Truncation.TruncatedBy != "lines" ||
		details.Truncation.TotalLines != 2500 ||
		details.Truncation.OutputLines != 2000 {
		t.Fatalf("truncation details = %#v", result.Details)
	}
}

func TestReadToolTrailingNewlineAndInvalidOffset(t *testing.T) {
	ctx := context.Background()
	toolContext := newTestExecutionContext(t)
	mustWriteFile(t, toolContext.Env, "exact.txt", strings.Repeat("x\n", 2000))

	result, err := executeHarnessTool(ctx, CreateReadTool(), map[string]any{"path": "exact.txt"}, toolContext)
	if err != nil {
		t.Fatal(err)
	}
	if result.Details != nil || strings.Contains(textOutput(result), "Use offset=") {
		t.Fatalf("exact-limit result = %#v", result)
	}

	mustWriteFile(t, toolContext.Env, "short.txt", "one\ntwo\nthree")
	_, err = executeHarnessTool(ctx, CreateReadTool(), map[string]any{"path": "short.txt", "offset": 100}, toolContext)
	if err == nil || !strings.Contains(err.Error(), "Offset 100 is beyond end of file (3 lines total)") {
		t.Fatalf("invalid offset error = %v", err)
	}
}

func TestReadToolDetectsImagesAndUsesProcessor(t *testing.T) {
	ctx := context.Background()
	toolContext := newTestExecutionContext(t)
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGNgYGD4DwABBAEAX+XDSwAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	mustWriteBytes(t, toolContext.Env, "image.txt", png)

	result, err := executeHarnessTool(ctx, CreateReadTool(), map[string]any{"path": "image.txt"}, toolContext)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(textOutput(result), "Read image file [image/png]") || !hasContentType(result, llm.ContentImage) {
		t.Fatalf("PNG result = %#v", result)
	}

	bmp := tinyBMP()
	mustWriteBytes(t, toolContext.Env, "image.bmp", bmp)
	autoResize := false
	var receivedMIME string
	var receivedAutoResize bool
	result, err = executeHarnessTool(ctx, CreateReadTool(ReadToolOptions{
		AutoResizeImages: &autoResize,
		ImageProcessor: func(_ context.Context, content []byte, mimeType string, autoResizeImages bool) (ReadImageProcessorResult, error) {
			if string(content) != string(bmp) {
				t.Fatalf("processor bytes changed")
			}
			receivedMIME = mimeType
			receivedAutoResize = autoResizeImages
			return ReadImageProcessorResult{
				OK:       true,
				Data:     "converted",
				MIMEType: "image/png",
				Hints:    []string{"[Image converted from image/bmp to image/png.]"},
			}, nil
		},
	}), map[string]any{"path": "image.bmp"}, toolContext)
	if err != nil {
		t.Fatal(err)
	}
	if receivedMIME != "image/bmp" || receivedAutoResize {
		t.Fatalf("processor options = %q / %v", receivedMIME, receivedAutoResize)
	}
	if !strings.Contains(textOutput(result), "[Image converted from image/bmp to image/png.]") ||
		len(result.Content) != 2 ||
		result.Content[1].Data != "converted" ||
		result.Content[1].MIMEType != "image/png" {
		t.Fatalf("processed image result = %#v", result)
	}
}

func TestWriteToolCreatesParents(t *testing.T) {
	ctx := context.Background()
	toolContext := newTestExecutionContext(t)
	result, err := executeHarnessTool(ctx, CreateWriteTool(), map[string]any{
		"path":    "nested/dir/file.txt",
		"content": "hello",
	}, toolContext)
	if err != nil {
		t.Fatal(err)
	}
	if textOutput(result) != "Successfully wrote 5 bytes to nested/dir/file.txt" {
		t.Fatalf("write result = %#v", result)
	}
	content, err := toolContext.Env.ReadTextFile(ctx, "nested/dir/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "hello" {
		t.Fatalf("written content = %q", content)
	}
}

func TestWriteToolHoldsMutationQueueUntilCancelledWriteSettles(t *testing.T) {
	base := harnessenv.MustLocalExecutionEnv(t.TempDir())
	env := &blockingWriteEnv{
		LocalExecutionEnv: base,
		firstStarted:      make(chan struct{}),
		finishFirst:       make(chan struct{}),
	}
	toolContext := NewExecutionToolContext(env)
	tool := CreateWriteTool()
	firstContext, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := executeHarnessTool(firstContext, tool, map[string]any{"path": "file.txt", "content": "first\n"}, toolContext)
		firstDone <- err
	}()
	<-env.firstStarted
	cancelFirst()

	secondDone := make(chan error, 1)
	go func() {
		_, err := executeHarnessTool(context.Background(), tool, map[string]any{"path": "file.txt", "content": "second\n"}, toolContext)
		secondDone <- err
	}()
	time.Sleep(20 * time.Millisecond)
	if env.secondStarted.Load() {
		t.Fatal("second write started before the cancelled write settled")
	}
	close(env.finishFirst)
	if err := <-firstDone; err == nil || err.Error() != "Operation aborted" {
		t.Fatalf("first write error = %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	content, err := env.ReadTextFile(context.Background(), "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "second\n" {
		t.Fatalf("written content = %q", content)
	}
}

func TestEditToolAppliesDisjointEditsAndReturnsDiffs(t *testing.T) {
	ctx := context.Background()
	toolContext := newTestExecutionContext(t)
	original := "alpha\nbeta\ngamma\ndelta\n"
	mustWriteFile(t, toolContext.Env, "edit.txt", original)

	result, err := executeHarnessTool(ctx, CreateEditTool(), editParams("edit.txt",
		Edit{OldText: "alpha\n", NewText: "ALPHA\n"},
		Edit{OldText: "gamma\n", NewText: "GAMMA\n"},
	), toolContext)
	if err != nil {
		t.Fatal(err)
	}
	if textOutput(result) != "Successfully replaced 2 block(s) in edit.txt." {
		t.Fatalf("edit result = %#v", result)
	}
	details, ok := result.Details.(*EditToolDetails)
	if !ok ||
		!strings.Contains(details.Diff, "ALPHA") ||
		!strings.Contains(details.Diff, "GAMMA") ||
		details.FirstChangedLine != 1 {
		t.Fatalf("edit details = %#v", result.Details)
	}
	wantPatch := "--- edit.txt\n" +
		"+++ edit.txt\n" +
		"@@ -1,4 +1,4 @@\n" +
		"-alpha\n" +
		"+ALPHA\n" +
		" beta\n" +
		"-gamma\n" +
		"+GAMMA\n" +
		" delta\n"
	if details.Patch != wantPatch {
		t.Fatalf("patch =\n%s\nwant:\n%s", details.Patch, wantPatch)
	}
	content, err := toolContext.Env.ReadTextFile(ctx, "edit.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "ALPHA\nbeta\nGAMMA\ndelta\n" {
		t.Fatalf("edited content = %q", content)
	}
}

func TestEditToolRejectsOverlapsMissingAndDuplicateTargets(t *testing.T) {
	ctx := context.Background()
	toolContext := newTestExecutionContext(t)
	mustWriteFile(t, toolContext.Env, "edit.txt", "one\ntwo\nthree\n")
	_, err := executeHarnessTool(ctx, CreateEditTool(), editParams("edit.txt",
		Edit{OldText: "one\ntwo\n", NewText: "ONE\nTWO\n"},
		Edit{OldText: "two\nthree\n", NewText: "TWO\nTHREE\n"},
	), toolContext)
	if err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("overlap error = %v", err)
	}
	content, readErr := toolContext.Env.ReadTextFile(ctx, "edit.txt")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if content != "one\ntwo\nthree\n" {
		t.Fatalf("overlap changed file = %q", content)
	}

	mustWriteFile(t, toolContext.Env, "targets.txt", "foo foo foo")
	_, err = executeHarnessTool(ctx, CreateEditTool(), editParams("targets.txt",
		Edit{OldText: "bar", NewText: "baz"},
	), toolContext)
	if err == nil || !strings.Contains(err.Error(), "Could not find the exact text") {
		t.Fatalf("missing-target error = %v", err)
	}
	_, err = executeHarnessTool(ctx, CreateEditTool(), editParams("targets.txt",
		Edit{OldText: "foo", NewText: "bar"},
	), toolContext)
	if err == nil || !strings.Contains(err.Error(), "Found 3 occurrences") {
		t.Fatalf("duplicate-target error = %v", err)
	}
}

func TestEditToolSerializesCanonicalAndSymlinkPaths(t *testing.T) {
	ctx := context.Background()
	base := harnessenv.MustLocalExecutionEnv(t.TempDir())
	mustWriteFile(t, base, "target.txt", "alpha\nbeta\ngamma\n")
	if err := os.Symlink("target.txt", base.AbsolutePath("link.txt")); err != nil {
		t.Fatal(err)
	}
	env := &slowReadEnv{LocalExecutionEnv: base}
	toolContext := NewExecutionToolContext(env)
	tool := CreateEditTool()
	errors := make(chan error, 2)
	go func() {
		_, err := executeHarnessTool(ctx, tool, editParams("target.txt",
			Edit{OldText: "alpha", NewText: "ALPHA"},
		), toolContext)
		errors <- err
	}()
	go func() {
		_, err := executeHarnessTool(ctx, tool, editParams("link.txt",
			Edit{OldText: "beta", NewText: "BETA"},
		), toolContext)
		errors <- err
	}()
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	content, err := env.ReadTextFile(ctx, "target.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "ALPHA\nBETA\ngamma\n" {
		t.Fatalf("concurrent edit content = %q", content)
	}
}

func TestEditToolPreservesBOMAndCRLF(t *testing.T) {
	ctx := context.Background()
	toolContext := newTestExecutionContext(t)
	mustWriteFile(t, toolContext.Env, "edit.txt", "\ufeffone\r\ntwo\r\n")
	_, err := executeHarnessTool(ctx, CreateEditTool(), editParams("edit.txt",
		Edit{OldText: "two", NewText: "TWO"},
	), toolContext)
	if err != nil {
		t.Fatal(err)
	}
	content, err := toolContext.Env.ReadTextFile(ctx, "edit.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "\ufeffone\r\nTWO\r\n" {
		t.Fatalf("BOM/CRLF content = %q", content)
	}
}

func TestEditToolFuzzyMatchingPreservesUnchangedLines(t *testing.T) {
	ctx := context.Background()
	toolContext := newTestExecutionContext(t)
	mustWriteFile(t, toolContext.Env, "edit.txt", "console.log(\u2018hello\u2019);   \nhello\u00a0world\nunchanged   \n")
	_, err := executeHarnessTool(ctx, CreateEditTool(), editParams("edit.txt",
		Edit{OldText: "console.log('hello');\n", NewText: "console.log('world');\n"},
		Edit{OldText: "hello world\n", NewText: "hello universe\n"},
	), toolContext)
	if err != nil {
		t.Fatal(err)
	}
	content, err := toolContext.Env.ReadTextFile(ctx, "edit.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "console.log('world');\nhello universe\nunchanged   \n" {
		t.Fatalf("fuzzy edit content = %q", content)
	}
}

func TestEditToolHoldsMutationQueueUntilCancelledWriteSettles(t *testing.T) {
	base := harnessenv.MustLocalExecutionEnv(t.TempDir())
	mustWriteFile(t, base, "file.txt", "alpha\nbeta\n")
	env := &blockingEditEnv{
		LocalExecutionEnv: base,
		firstStarted:      make(chan struct{}),
		finishFirst:       make(chan struct{}),
	}
	toolContext := NewExecutionToolContext(env)
	tool := CreateEditTool()
	firstContext, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := executeHarnessTool(firstContext, tool, editParams("file.txt",
			Edit{OldText: "alpha", NewText: "ALPHA"},
		), toolContext)
		firstDone <- err
	}()
	<-env.firstStarted
	cancelFirst()

	secondDone := make(chan error, 1)
	go func() {
		_, err := executeHarnessTool(context.Background(), tool, editParams("file.txt",
			Edit{OldText: "beta", NewText: "BETA"},
		), toolContext)
		secondDone <- err
	}()
	time.Sleep(20 * time.Millisecond)
	if env.secondStarted.Load() {
		t.Fatal("second edit started before the cancelled edit settled")
	}
	close(env.finishFirst)
	if err := <-firstDone; err == nil || err.Error() != "Operation aborted" {
		t.Fatalf("first edit error = %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	if !env.firstSettled.Load() {
		t.Fatal("first write had not settled before second edit completed")
	}
	content, err := env.ReadTextFile(context.Background(), "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "ALPHA\nBETA\n" {
		t.Fatalf("edited content = %q", content)
	}
}

type blockingWriteEnv struct {
	*harnessenv.LocalExecutionEnv
	firstStarted  chan struct{}
	finishFirst   chan struct{}
	secondStarted atomic.Bool
}

type slowReadEnv struct {
	*harnessenv.LocalExecutionEnv
}

func (e *slowReadEnv) ReadTextFile(ctx context.Context, path string) (string, error) {
	time.Sleep(20 * time.Millisecond)
	return e.LocalExecutionEnv.ReadTextFile(ctx, path)
}

type blockingEditEnv struct {
	*harnessenv.LocalExecutionEnv
	firstStarted  chan struct{}
	finishFirst   chan struct{}
	firstSettled  atomic.Bool
	secondStarted atomic.Bool
}

func (e *blockingEditEnv) WriteFile(_ context.Context, path string, content []byte) error {
	switch string(content) {
	case "ALPHA\nbeta\n":
		close(e.firstStarted)
		<-e.finishFirst
		err := e.LocalExecutionEnv.WriteFile(context.Background(), path, content)
		e.firstSettled.Store(true)
		return err
	case "ALPHA\nBETA\n", "alpha\nBETA\n":
		e.secondStarted.Store(true)
	}
	return e.LocalExecutionEnv.WriteFile(context.Background(), path, content)
}

func (e *blockingWriteEnv) WriteFile(_ context.Context, path string, content []byte) error {
	switch string(content) {
	case "first\n":
		close(e.firstStarted)
		<-e.finishFirst
	case "second\n":
		e.secondStarted.Store(true)
	}
	return e.LocalExecutionEnv.WriteFile(context.Background(), path, content)
}

func newTestExecutionContext(t *testing.T) ExecutionToolContext {
	t.Helper()
	return NewExecutionToolContext(harnessenv.MustLocalExecutionEnv(t.TempDir()))
}

func executeHarnessTool(ctx context.Context, tool agentharness.AgentHarnessTool, params map[string]any, toolContext any) (core.AgentToolResult, error) {
	return tool.Execute(ctx, "test-call", params, nil, toolContext)
}

func textOutput(result core.AgentToolResult) string {
	var text []string
	for _, part := range result.Content {
		if part.Type == llm.ContentText {
			text = append(text, part.Text)
		}
	}
	return strings.Join(text, "\n")
}

func hasContentType(result core.AgentToolResult, contentType string) bool {
	for _, part := range result.Content {
		if part.Type == contentType {
			return true
		}
	}
	return false
}

func mustWriteFile(t *testing.T, env harnessenv.ExecutionEnv, path, content string) {
	t.Helper()
	mustWriteBytes(t, env, path, []byte(content))
}

func mustWriteBytes(t *testing.T, env harnessenv.ExecutionEnv, path string, content []byte) {
	t.Helper()
	if err := env.WriteFile(context.Background(), path, content); err != nil {
		t.Fatal(err)
	}
}

func tinyBMP() []byte {
	content := make([]byte, 58)
	content[0] = 'B'
	content[1] = 'M'
	content[2] = byte(len(content))
	content[10] = 54
	content[14] = 40
	content[18] = 1
	content[22] = 1
	content[26] = 1
	content[28] = 24
	content[34] = 4
	return content
}

func editParams(path string, edits ...Edit) map[string]any {
	raw := make([]any, 0, len(edits))
	for _, edit := range edits {
		raw = append(raw, map[string]any{
			"oldText": edit.OldText,
			"newText": edit.NewText,
		})
	}
	return map[string]any{"path": path, "edits": raw}
}
