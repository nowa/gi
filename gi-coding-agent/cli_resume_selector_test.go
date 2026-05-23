package gicodingagent

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func TestCLIResumeSelectorSelectsSessionPiStyle(t *testing.T) {
	cwd := t.TempDir()
	sessionDir := filepath.Join(t.TempDir(), "sessions")
	manager := makeCLIResumeSelectorSession(t, cwd, sessionDir)
	terminal := gitui.NewVirtualTerminal(120, 28)

	resultCh := make(chan struct {
		result cliResumeSelectorResult
		err    error
	}, 1)
	go func() {
		result, err := runCLIResumeSelector(cwd, sessionDir, terminal, DefaultProtocolKeybindings())
		resultCh <- struct {
			result cliResumeSelectorResult
			err    error
		}{result: result, err: err}
	}()

	waitForViewport(t, terminal, "Resume Session (Current Folder)")
	waitForViewport(t, terminal, "Target question")
	terminal.SendInput("\r")

	out := waitForCLIResumeSelectorResult(t, resultCh)
	if out.err != nil {
		t.Fatal(out.err)
	}
	if !out.result.Selected || out.result.Path != manager.GetSessionFile() {
		t.Fatalf("result = %#v, want selected %q", out.result, manager.GetSessionFile())
	}
}

func TestResolveCLIInteractiveResumeConvertsResumeFlagToSessionPiStyle(t *testing.T) {
	cwd := t.TempDir()
	agentDir := filepath.Join(t.TempDir(), "agent")
	sessionDir := filepath.Join(t.TempDir(), "sessions")
	manager := makeCLIResumeSelectorSession(t, cwd, sessionDir)
	terminal := gitui.NewVirtualTerminal(120, 28)

	resultCh := make(chan struct {
		args Args
		host CLIInteractiveRuntimeHost
		err  error
	}, 1)
	go func() {
		args, host, err := resolveCLIInteractiveResumeWithTerminal(Args{
			Resume:     true,
			SessionDir: sessionDir,
		}, CLIOptions{CWD: cwd, AgentDir: agentDir}, terminal)
		resultCh <- struct {
			args Args
			host CLIInteractiveRuntimeHost
			err  error
		}{args: args, host: host, err: err}
	}()

	waitForViewport(t, terminal, "Resume Session (Current Folder)")
	terminal.SendInput("\r")
	out := waitForCLIInteractiveResumeResult(t, resultCh)
	if out.err != nil {
		t.Fatal(out.err)
	}
	if out.host != nil {
		t.Fatalf("host = %T, want nil after selection", out.host)
	}
	if out.args.Resume || out.args.Session != manager.GetSessionFile() {
		t.Fatalf("args = %#v, want Resume false and Session %q", out.args, manager.GetSessionFile())
	}
}

func TestResolveCLIInteractiveResumeCancelReturnsNoopPiStyle(t *testing.T) {
	cwd := t.TempDir()
	agentDir := filepath.Join(t.TempDir(), "agent")
	sessionDir := filepath.Join(t.TempDir(), "sessions")
	makeCLIResumeSelectorSession(t, cwd, sessionDir)
	terminal := gitui.NewVirtualTerminal(120, 28)
	var stdout bytes.Buffer

	resultCh := make(chan struct {
		args Args
		host CLIInteractiveRuntimeHost
		err  error
	}, 1)
	go func() {
		args, host, err := resolveCLIInteractiveResumeWithTerminal(Args{
			Resume:     true,
			SessionDir: sessionDir,
		}, CLIOptions{CWD: cwd, AgentDir: agentDir, Stdout: &stdout}, terminal)
		resultCh <- struct {
			args Args
			host CLIInteractiveRuntimeHost
			err  error
		}{args: args, host: host, err: err}
	}()

	waitForViewport(t, terminal, "Resume Session (Current Folder)")
	terminal.SendInput("\x1b")
	out := waitForCLIInteractiveResumeResult(t, resultCh)
	if out.err != nil {
		t.Fatal(out.err)
	}
	if out.host == nil {
		t.Fatal("host is nil, want no-op host")
	}
	if err := out.host.Run(); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "No session selected\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func makeCLIResumeSelectorSession(t *testing.T, cwd, sessionDir string) *SessionManager {
	t.Helper()
	manager, err := CreateSessionManager(cwd, sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	manager.AppendMessage(llm.Message{Role: llm.RoleUser, Content: []llm.ContentPart{llm.Text("Target question")}})
	manager.AppendMessage(llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("Target answer")}})
	if err := manager.rewriteFile(); err != nil {
		t.Fatal(err)
	}
	return manager
}

func waitForCLIResumeSelectorResult(t *testing.T, ch <-chan struct {
	result cliResumeSelectorResult
	err    error
}) struct {
	result cliResumeSelectorResult
	err    error
} {
	t.Helper()
	select {
	case out := <-ch:
		return out
	case <-time.After(time.Second):
		t.Fatal("resume selector did not finish")
	}
	return struct {
		result cliResumeSelectorResult
		err    error
	}{}
}

func waitForCLIInteractiveResumeResult(t *testing.T, ch <-chan struct {
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
		t.Fatal("interactive resume did not finish")
	}
	return struct {
		args Args
		host CLIInteractiveRuntimeHost
		err  error
	}{}
}
