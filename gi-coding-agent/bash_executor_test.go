package gicodingagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestExecuteBashResolvesAfterShellExitWithInheritedStdio(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test command uses POSIX sh")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "grandchild.pid")
	command := "sleep 60 & echo $! > " + bashExecutorShellQuote(pidFile) + "; echo child-exiting"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	defer cleanupBashGrandchild(t, pidFile)

	result, err := ExecuteBash(command, dir, BashExecutorOptions{Context: ctx, ExitStdioGrace: 25 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "child-exiting") {
		t.Fatalf("output = %q, want child-exiting", result.Output)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Cancelled {
		t.Fatal("command should not be marked cancelled")
	}
}

func TestSDKBashToolResolvesAfterShellExitWithInheritedStdio(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test command uses POSIX sh")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "tool-grandchild.pid")
	command := "sleep 60 & echo $! > " + bashExecutorShellQuote(pidFile) + "; echo child-exiting"
	defer cleanupBashGrandchild(t, pidFile)

	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            dir,
		AgentDir:       dir,
		SessionManager: mustInMemorySessionManager(t, dir),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()
	bashTool, ok := findSDKTool(session.Agent.State.Tools, "bash")
	if !ok {
		t.Fatal("bash tool not found")
	}

	done := make(chan struct{})
	var result SDKToolResult
	var executeErr error
	go func() {
		defer close(done)
		result, executeErr = bashTool.Execute("test-call", map[string]any{"command": command})
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bash tool did not resolve after shell exit")
	}
	if executeErr != nil {
		t.Fatal(executeErr)
	}
	if output := sdkToolText(result); !strings.Contains(output, "child-exiting") {
		t.Fatalf("output = %q, want child-exiting", output)
	}
}

func bashExecutorShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func cleanupBashGrandchild(t *testing.T, pidFile string) {
	t.Helper()
	content, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil || pid <= 0 {
		return
	}
	_ = exec.Command("kill", strconv.Itoa(pid)).Run()
}

func mustInMemorySessionManager(t *testing.T, cwd string) *SessionManager {
	t.Helper()
	sessionManager, err := InMemorySessionManager(cwd)
	if err != nil {
		t.Fatal(err)
	}
	return sessionManager
}
