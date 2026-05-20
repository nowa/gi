package gicodingagent

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBashToolPiBasics(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test commands use POSIX shell syntax")
	}
	dir := t.TempDir()
	tool := NewBashTool(dir)

	result, err := tool.Execute("test-call-8", BashToolInput{Command: "echo 'test output'"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readToolText(result), "test output") {
		t.Fatalf("bash output = %q", readToolText(result))
	}
	if _, err := tool.Execute("test-call-9", BashToolInput{Command: "exit 1"}); err == nil || !strings.Contains(err.Error(), "code 1") {
		t.Fatalf("exit err = %v", err)
	}
	if _, err := tool.Execute("test-call-10", BashToolInput{Command: "sleep 5", Timeout: 1}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "timed out") {
		t.Fatalf("timeout err = %v", err)
	}

	badCwd := filepath.Join(dir, "missing")
	if _, err := NewBashTool(badCwd).Execute("test-call-11", BashToolInput{Command: "echo test"}); err == nil || !strings.Contains(err.Error(), "Working directory does not exist") {
		t.Fatalf("bad cwd err = %v", err)
	}
}

func TestBashToolCommandPrefixPiBasics(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test commands use POSIX shell syntax")
	}
	dir := t.TempDir()

	withEnvPrefix := NewBashTool(dir, BashToolOptions{CommandPrefix: "export TEST_VAR=hello"})
	result, err := withEnvPrefix.Execute("test-prefix-1", BashToolInput{Command: "echo $TEST_VAR"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(readToolText(result)) != "hello" {
		t.Fatalf("prefix env output = %q", readToolText(result))
	}

	withOutputPrefix := NewBashTool(dir, BashToolOptions{CommandPrefix: "echo prefix-output"})
	result, err = withOutputPrefix.Execute("test-prefix-2", BashToolInput{Command: "echo command-output"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(readToolText(result)) != "prefix-output\ncommand-output" {
		t.Fatalf("prefix output = %q", readToolText(result))
	}

	withoutPrefix := NewBashTool(dir)
	result, err = withoutPrefix.Execute("test-prefix-3", BashToolInput{Command: "echo no-prefix"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(readToolText(result)) != "no-prefix" {
		t.Fatalf("no prefix output = %q", readToolText(result))
	}
}
