package gicodingagent

import (
	"path/filepath"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func TestCLIMissingSessionCWDPromptContinuesPiStyle(t *testing.T) {
	issue := MissingSessionCwdIssue{
		SessionFile: "/tmp/session.jsonl",
		SessionCwd:  "/missing/project",
		FallbackCwd: "/current/project",
	}
	terminal := gitui.NewVirtualTerminal(120, 28)
	resultCh := make(chan struct {
		result cliMissingCwdPromptResult
		err    error
	}, 1)
	go func() {
		result, err := runCLIMissingSessionCWDPrompt(issue, terminal)
		resultCh <- struct {
			result cliMissingCwdPromptResult
			err    error
		}{result: result, err: err}
	}()

	waitForViewport(t, terminal, "cwd from session file does not exist")
	waitForViewport(t, terminal, "/missing/project")
	waitForViewport(t, terminal, "/current/project")
	terminal.SendInput("\r")

	out := waitForCLIMissingCWDResult(t, resultCh)
	if out.err != nil {
		t.Fatal(out.err)
	}
	if !out.result.Selected || out.result.CWD != issue.FallbackCwd {
		t.Fatalf("result = %#v, want continue with %q", out.result, issue.FallbackCwd)
	}
}

func TestResolveCLIInteractiveMissingSessionCWDUsesOverridePiStyle(t *testing.T) {
	startupCwd := t.TempDir()
	missingCwd := filepath.Join(t.TempDir(), "missing-project")
	agentDir := filepath.Join(t.TempDir(), "agent")
	sessionDir := filepath.Join(t.TempDir(), "sessions")
	manager := makeCLIMissingCWDSession(t, missingCwd, sessionDir)
	terminal := gitui.NewVirtualTerminal(120, 28)
	resultCh := make(chan struct {
		args Args
		host CLIInteractiveRuntimeHost
		err  error
	}, 1)
	go func() {
		args, host, err := resolveCLIInteractiveMissingSessionCWDWithTerminal(Args{
			Offline:    true,
			Model:      "openai/gpt-4o-mini",
			Session:    manager.GetSessionFile(),
			SessionDir: sessionDir,
		}, CLIOptions{CWD: startupCwd, AgentDir: agentDir}, terminal)
		resultCh <- struct {
			args Args
			host CLIInteractiveRuntimeHost
			err  error
		}{args: args, host: host, err: err}
	}()

	waitForViewport(t, terminal, "Session CWD missing")
	terminal.SendInput("\r")
	out := waitForCLIInteractiveMissingCWDResult(t, resultCh)
	if out.err != nil {
		t.Fatal(out.err)
	}
	if out.host != nil {
		t.Fatalf("host = %T, want nil after continue", out.host)
	}
	if out.args.SessionCwdOverride != startupCwd {
		t.Fatalf("SessionCwdOverride = %q, want %q", out.args.SessionCwdOverride, startupCwd)
	}
}

func TestResolveCLIInteractiveMissingSessionCWDCancelExitsPiStyle(t *testing.T) {
	startupCwd := t.TempDir()
	missingCwd := filepath.Join(t.TempDir(), "missing-project")
	agentDir := filepath.Join(t.TempDir(), "agent")
	sessionDir := filepath.Join(t.TempDir(), "sessions")
	manager := makeCLIMissingCWDSession(t, missingCwd, sessionDir)
	terminal := gitui.NewVirtualTerminal(120, 28)
	resultCh := make(chan struct {
		args Args
		host CLIInteractiveRuntimeHost
		err  error
	}, 1)
	go func() {
		args, host, err := resolveCLIInteractiveMissingSessionCWDWithTerminal(Args{
			Offline:    true,
			Model:      "openai/gpt-4o-mini",
			Session:    manager.GetSessionFile(),
			SessionDir: sessionDir,
		}, CLIOptions{CWD: startupCwd, AgentDir: agentDir}, terminal)
		resultCh <- struct {
			args Args
			host CLIInteractiveRuntimeHost
			err  error
		}{args: args, host: host, err: err}
	}()

	waitForViewport(t, terminal, "Session CWD missing")
	terminal.SendInput("\x1b")
	out := waitForCLIInteractiveMissingCWDResult(t, resultCh)
	if out.err != nil {
		t.Fatal(out.err)
	}
	if out.host == nil {
		t.Fatal("host is nil, want no-op host")
	}
	if err := out.host.Run(); err != nil {
		t.Fatal(err)
	}
}

func makeCLIMissingCWDSession(t *testing.T, cwd, sessionDir string) *SessionManager {
	t.Helper()
	manager, err := CreateSessionManager(cwd, sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	manager.AppendMessage(llm.Message{Role: llm.RoleUser, Content: []llm.ContentPart{llm.Text("from missing cwd")}})
	if err := manager.rewriteFile(); err != nil {
		t.Fatal(err)
	}
	return manager
}

func waitForCLIMissingCWDResult(t *testing.T, ch <-chan struct {
	result cliMissingCwdPromptResult
	err    error
}) struct {
	result cliMissingCwdPromptResult
	err    error
} {
	t.Helper()
	select {
	case out := <-ch:
		return out
	case <-time.After(time.Second):
		t.Fatal("missing cwd prompt did not finish")
	}
	return struct {
		result cliMissingCwdPromptResult
		err    error
	}{}
}

func waitForCLIInteractiveMissingCWDResult(t *testing.T, ch <-chan struct {
	args Args
	host CLIInteractiveRuntimeHost
	err  error
}) struct {
	args Args
	host CLIInteractiveRuntimeHost
	err  error
} {
	t.Helper()
	select {
	case out := <-ch:
		return out
	case <-time.After(time.Second):
		t.Fatal("interactive missing cwd prompt did not finish")
	}
	return struct {
		args Args
		host CLIInteractiveRuntimeHost
		err  error
	}{}
}
