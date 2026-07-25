package tools

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	core "github.com/nowa/gi/gi-agent-core"
	harnessenv "github.com/nowa/gi/gi-agent-core/harness/env"
)

func TestResolveBashTimeoutUsesTypedDurationBoundary(t *testing.T) {
	timeout, err := resolveBashTimeout(0, false)
	if err != nil || timeout != 0 {
		t.Fatalf("unset timeout = %s, %v", timeout, err)
	}
	timeout, err = resolveBashTimeout(0.125, true)
	if err != nil || timeout != 125*time.Millisecond {
		t.Fatalf("fractional timeout = %s, %v", timeout, err)
	}
	for _, value := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := resolveBashTimeout(value, true); err == nil ||
			!strings.Contains(err.Error(), "finite number of seconds") {
			t.Fatalf("timeout %v error = %v", value, err)
		}
	}
	if _, err := resolveBashTimeout(maxBashTimeoutSeconds+1, true); err == nil ||
		!strings.Contains(err.Error(), "maximum is") {
		t.Fatalf("oversized timeout error = %v", err)
	}
}

func TestBashToolCombinesOutputAndReportsFailures(t *testing.T) {
	toolContext := newTestExecutionContext(t)
	tool := CreateBashTool()

	result, err := executeHarnessTool(context.Background(), tool, map[string]any{
		"command": "printf out; printf err >&2",
	}, toolContext)
	if err != nil {
		t.Fatal(err)
	}
	output := textOutput(result)
	if !strings.Contains(output, "out") || !strings.Contains(output, "err") {
		t.Fatalf("combined output = %q", output)
	}

	_, err = executeHarnessTool(context.Background(), tool, map[string]any{
		"command": "printf failed; exit 7",
	}, toolContext)
	if err == nil || !strings.Contains(err.Error(), "failed") || !strings.Contains(err.Error(), "Command exited with code 7") {
		t.Fatalf("nonzero error = %v", err)
	}

	_, err = executeHarnessTool(context.Background(), tool, map[string]any{
		"command": "sleep 2",
		"timeout": 0.01,
	}, toolContext)
	if err == nil || !strings.Contains(err.Error(), "Command timed out after 0.01 seconds") {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestBashToolPreservesTruncatedOutputOnTimeout(t *testing.T) {
	toolContext := newTestExecutionContext(t)
	_, err := executeHarnessTool(context.Background(), CreateBashTool(), map[string]any{
		"command": "i=1; while [ $i -le 3000 ]; do echo line-$i; i=$((i + 1)); done; sleep 2",
		"timeout": 0.05,
	}, toolContext)
	if err == nil {
		t.Fatal("timeout unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "Command timed out after 0.05 seconds") {
		t.Fatalf("timeout error = %v", err)
	}
	fullOutputPath := fullOutputPathFromText(err.Error())
	if fullOutputPath == "" {
		t.Fatalf("full output path missing from %q", err.Error())
	}
	fullOutput, readErr := toolContext.Env.ReadTextFile(context.Background(), fullOutputPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(fullOutput, "line-1\nline-2") || !strings.Contains(fullOutput, "line-2999\nline-3000") {
		t.Fatalf("full timeout output missing bounds: %q", fullOutput)
	}
}

func TestBashToolIgnoresLateOutputCallbacks(t *testing.T) {
	base := harnessenv.MustLocalExecutionEnv(t.TempDir())
	env := &lateOutputEnv{LocalExecutionEnv: base}
	toolContext := NewExecutionToolContext(env)
	var mu sync.Mutex
	var updates []string
	result, err := CreateBashTool().Execute(
		context.Background(),
		"bash-late",
		map[string]any{"command": "late"},
		func(update core.AgentToolResult) {
			mu.Lock()
			updates = append(updates, textOutput(update))
			mu.Unlock()
		},
		toolContext,
	)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if textOutput(result) != "before\n" {
		t.Fatalf("result = %q", textOutput(result))
	}
	mu.Lock()
	defer mu.Unlock()
	for _, update := range updates {
		if strings.Contains(update, "late") {
			t.Fatalf("late output leaked into updates: %#v", updates)
		}
	}
}

func TestBashToolReportsOversizedFinalLine(t *testing.T) {
	toolContext := newTestExecutionContext(t)
	result, err := executeHarnessTool(context.Background(), CreateBashTool(), map[string]any{
		"command": "printf '%060000d' 0",
	}, toolContext)
	if err != nil {
		t.Fatal(err)
	}
	output := textOutput(result)
	if !strings.Contains(output, "Showing last 50.0KB of line 1 (line is 58.6KB). Full output:") {
		t.Fatalf("oversized-line output len=%d tail=%q details=%#v", len(output), output[max(0, len(output)-300):], result.Details)
	}
}

func TestBashToolPrepareReceivesTurnContext(t *testing.T) {
	t.Setenv("GI_BASH_PREPARE_INHERITED", "inherited")
	base := harnessenv.MustLocalExecutionEnv(t.TempDir())
	if err := base.CreateDir(context.Background(), "workspace", harnessenv.CreateDirOptions{}); err != nil {
		t.Fatal(err)
	}
	type preparedContext struct {
		ExecutionToolContext
		Workspace string
	}
	toolContext := preparedContext{
		ExecutionToolContext: NewExecutionToolContext(base),
		Workspace:            base.AbsolutePath("workspace"),
	}
	var received any
	tool := CreateBashTool(BashToolOptions{
		CommandPrefix: "prefix=ready",
		Prepare: func(_ context.Context, execution *BashExecution, contextValue any) error {
			received = contextValue
			context := contextValue.(preparedContext)
			execution.CWD = context.Workspace
			execution.Env = map[string]string{"GI_BASH_PREPARE_EXPLICIT": "explicit"}
			execution.InheritEnv = false
			execution.Command += "\nprintf '%s:%s:%s:%s' \"$prefix\" \"${GI_BASH_PREPARE_INHERITED-}\" \"$GI_BASH_PREPARE_EXPLICIT\" \"$PWD\""
			return nil
		},
	})
	result, err := executeHarnessTool(context.Background(), tool, map[string]any{"command": ":"}, toolContext)
	if err != nil {
		t.Fatal(err)
	}
	if received == nil {
		t.Fatal("prepare did not receive the turn context")
	}
	canonicalWorkspace, err := base.CanonicalPath(context.Background(), toolContext.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("ready::explicit:%s", canonicalWorkspace)
	if textOutput(result) != want {
		t.Fatalf("prepared output = %q, want %q", textOutput(result), want)
	}
}

func TestBashToolSupportsCommandPrefix(t *testing.T) {
	toolContext := newTestExecutionContext(t)
	result, err := executeHarnessTool(context.Background(), CreateBashTool(BashToolOptions{
		CommandPrefix: "value=hello",
	}), map[string]any{"command": "printf \"$value\""}, toolContext)
	if err != nil {
		t.Fatal(err)
	}
	if textOutput(result) != "hello" {
		t.Fatalf("prefix output = %q", textOutput(result))
	}
}

func TestBashToolCoalescesUpdatesAndPersistsFullOutput(t *testing.T) {
	toolContext := newTestExecutionContext(t)
	var mu sync.Mutex
	var updates []core.AgentToolResult
	result, err := CreateBashTool().Execute(
		context.Background(),
		"bash-output",
		map[string]any{"command": "i=1; while [ $i -le 3000 ]; do echo line-$i; i=$((i + 1)); done"},
		func(update core.AgentToolResult) {
			mu.Lock()
			updates = append(updates, update)
			mu.Unlock()
		},
		toolContext,
	)
	if err != nil {
		t.Fatal(err)
	}
	details, ok := result.Details.(*BashToolDetails)
	if !ok || details.Truncation == nil ||
		!details.Truncation.Truncated ||
		details.Truncation.TruncatedBy != "lines" ||
		details.Truncation.TotalLines != 3000 ||
		details.Truncation.OutputLines != 2000 ||
		details.FullOutputPath == "" {
		t.Fatalf("bash details = %#v truncation=%#v", result.Details, details.Truncation)
	}
	if !strings.Contains(textOutput(result), "line-3000") {
		t.Fatalf("tail output missing final line")
	}
	mu.Lock()
	if len(updates) >= 25 {
		t.Fatalf("updates were not coalesced: %d", len(updates))
	}
	finalUpdate := updates[len(updates)-1]
	mu.Unlock()
	if !strings.Contains(textOutput(finalUpdate), "line-3000") {
		t.Fatalf("final update = %#v", finalUpdate)
	}
	updateDetails, ok := finalUpdate.Details.(*BashToolDetails)
	if !ok || updateDetails.Truncation == nil || updateDetails.FullOutputPath != details.FullOutputPath {
		t.Fatalf("final update details = %#v", finalUpdate.Details)
	}
	fullOutput, err := toolContext.Env.ReadTextFile(context.Background(), details.FullOutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fullOutput, "line-1\nline-2") || !strings.Contains(fullOutput, "line-2999\nline-3000") {
		t.Fatalf("full output missing bounds")
	}
}

type lateOutputEnv struct {
	*harnessenv.LocalExecutionEnv
}

func (e *lateOutputEnv) Exec(_ context.Context, _ string, options harnessenv.ExecOptions) (harnessenv.ExecResult, error) {
	if options.OnStdout != nil {
		if err := options.OnStdout("before\n"); err != nil {
			return harnessenv.ExecResult{}, err
		}
		go func() {
			time.Sleep(time.Millisecond)
			_ = options.OnStdout("late\n")
		}()
	}
	return harnessenv.ExecResult{Stdout: "before\n"}, nil
}

func fullOutputPathFromText(text string) string {
	const marker = "Full output: "
	start := strings.Index(text, marker)
	if start < 0 {
		return ""
	}
	path := text[start+len(marker):]
	if end := strings.IndexAny(path, "]\n"); end >= 0 {
		path = path[:end]
	}
	return path
}
