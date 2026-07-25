package gicodingagent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func TestQuoteIfNeededUsesShellSafeResumeArguments(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "alpha_1-/~:@", want: "alpha_1-/~:@"},
		{value: "", want: "''"},
		{value: "two words", want: "'two words'"},
		{value: "it's here", want: "'it'\\''s here'"},
		{value: "界", want: "'界'"},
	}
	for _, test := range tests {
		if got := quoteIfNeeded(test.value); got != test.want {
			t.Fatalf("quoteIfNeeded(%q) = %q, want %q", test.value, got, test.want)
		}
	}
}

func TestFormatResumeCommandUsesOneSessionIdentitySnapshot(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), "agent")
	t.Setenv(EnvCodingAgentDir, agentDir)
	t.Setenv(LegacyEnvCodingAgentDir, "")
	cwd := t.TempDir()

	defaultManager, err := CreateSessionManager(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if command, ok := formatResumeCommand(defaultManager); ok || command != "" {
		t.Fatalf("unflushed resume command = %q, available=%v", command, ok)
	}
	defaultManager.AppendMessage(sessionMessageValue(
		llm.AssistantMessage(nil, llm.StopReasonStop, llm.Model{}),
	))
	command, ok := formatResumeCommand(defaultManager)
	if !ok ||
		command != DefaultCodingAgentAppName+
			" --session "+defaultManager.GetSessionID() {
		t.Fatalf("default resume command = %q, available=%v", command, ok)
	}

	customDir := filepath.Join(t.TempDir(), "custom sessions")
	customManager, err := CreateSessionManager(cwd, customDir)
	if err != nil {
		t.Fatal(err)
	}
	customManager.AppendMessage(sessionMessageValue(
		llm.AssistantMessage(nil, llm.StopReasonStop, llm.Model{}),
	))
	command, ok = formatResumeCommand(customManager)
	want := DefaultCodingAgentAppName +
		" --session-dir " + quoteIfNeeded(customManager.GetSessionDir()) +
		" --session " + customManager.GetSessionID()
	if !ok || command != want {
		t.Fatalf("custom resume command = %q, %v; want %q", command, ok, want)
	}

	memoryManager, err := InMemorySessionManager(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if command, ok := formatResumeCommand(memoryManager); ok || command != "" {
		t.Fatalf("memory resume command = %q, available=%v", command, ok)
	}

	if err := os.Remove(customManager.GetSessionFile()); err != nil {
		t.Fatal(err)
	}
	if command, ok := formatResumeCommand(customManager); ok || command != "" {
		t.Fatalf("missing-file resume command = %q, available=%v", command, ok)
	}
}

func TestCLIInteractiveTUIHostPrintsResumeCommandAfterNormalStop(
	t *testing.T,
) {
	sessionHost := newPersistedResumeRuntimeHost(t)
	var stdout bytes.Buffer
	stdoutIsTTY := true
	host, err := NewCLIInteractiveTUIHost(
		CLIInteractiveTUIHostOptions{
			RuntimeHost: sessionHost,
			Terminal:    gitui.NewVirtualTerminal(80, 16),
			Stdout:      &stdout,
			StdoutIsTTY: &stdoutIsTTY,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	runResult := make(chan error, 1)
	go func() {
		runResult <- host.RunContext(context.Background())
	}()
	waitForResumeTestHostReady(t, host)
	host.Stop()
	waitForResumeTestHostStop(t, runResult)

	output := StripAnsi(stdout.String())
	command, ok := formatResumeCommand(
		sessionHost.session.SessionManager,
	)
	if !ok {
		t.Fatal("persisted runtime session has no resume command")
	}
	if !strings.Contains(
		output,
		"To resume this session: "+command,
	) {
		t.Fatalf("resume output = %q", output)
	}
}

func TestCLIInteractiveTUIHostSuppressesResumeCommandForSignalStop(
	t *testing.T,
) {
	sessionHost := newPersistedResumeRuntimeHost(t)
	var stdout bytes.Buffer
	stdoutIsTTY := true
	signals := make(chan os.Signal, 1)
	host, err := NewCLIInteractiveTUIHost(
		CLIInteractiveTUIHostOptions{
			RuntimeHost:     sessionHost,
			Terminal:        gitui.NewVirtualTerminal(80, 16),
			Stdout:          &stdout,
			StdoutIsTTY:     &stdoutIsTTY,
			ShutdownSignals: signals,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	runResult := make(chan error, 1)
	go func() {
		runResult <- host.RunContext(context.Background())
	}()
	waitForResumeTestHostReady(t, host)
	signals <- os.Interrupt
	waitForResumeTestHostStop(t, runResult)
	if stdout.Len() != 0 {
		t.Fatalf("signal shutdown resume output = %q", stdout.String())
	}
}

func newPersistedResumeRuntimeHost(
	t *testing.T,
) *agentSessionPrintModeHost {
	t.Helper()
	tempDir := t.TempDir()
	runtimeHost, err := newDefaultCLIPrintModeHost(
		Args{
			Offline: true,
			Model:   "openai/gpt-4o-mini",
		},
		CLIOptions{
			CWD:           tempDir,
			AgentDir:      filepath.Join(tempDir, "agent"),
			ModelRegistry: newTestOpenAIModelRegistry(),
			Responder:     DefaultAgentSessionResponder,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T", runtimeHost)
	}
	sessionHost.session.SessionManager.AppendMessage(
		sessionMessageValue(
			llm.AssistantMessage(nil, llm.StopReasonStop, llm.Model{}),
		),
	)
	return sessionHost
}

func waitForResumeTestHostReady(
	t *testing.T,
	host *CLIInteractiveTUIHost,
) {
	t.Helper()
	select {
	case <-host.uiReady:
	case <-time.After(time.Second):
		t.Fatal("interactive host did not become ready")
	}
}

func waitForResumeTestHostStop(
	t *testing.T,
	runResult <-chan error,
) {
	t.Helper()
	select {
	case err := <-runResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive host did not stop")
	}
}
