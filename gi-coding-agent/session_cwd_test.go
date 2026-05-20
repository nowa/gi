package gicodingagent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionCwdPiCases(t *testing.T) {
	fallbackCwd := t.TempDir()
	missingCwd := filepath.Join(fallbackCwd, "does-not-exist")
	sessionDir := t.TempDir()
	sessionFile := filepath.Join(sessionDir, "session.jsonl")
	writeSessionCwdFile(t, sessionFile, missingCwd)

	sessionManager, err := OpenSessionManager(sessionFile)
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	issue := GetMissingSessionCwdIssue(sessionManager, fallbackCwd)
	if issue == nil || issue.SessionFile != sessionManager.GetSessionFile() || issue.SessionCwd != missingCwd || issue.FallbackCwd != fallbackCwd {
		t.Fatalf("missing cwd issue = %#v", issue)
	}

	overrideManager, err := OpenSessionManager(sessionFile, "", fallbackCwd)
	if err != nil {
		t.Fatalf("open override session: %v", err)
	}
	if overrideManager.GetCwd() != fallbackCwd {
		t.Fatalf("override cwd = %q", overrideManager.GetCwd())
	}
	if issue := GetMissingSessionCwdIssue(overrideManager, fallbackCwd); issue != nil {
		t.Fatalf("override should avoid missing cwd issue: %#v", issue)
	}

	createRuntimeCalled := false
	_, err = CreateAgentSessionRuntime(func(options AgentSessionRuntimeOptions) (any, error) {
		createRuntimeCalled = true
		return nil, errors.New("should not be called")
	}, AgentSessionRuntimeOptions{
		CWD:            fallbackCwd,
		AgentDir:       fallbackCwd,
		SessionManager: sessionManager,
	})
	var missingErr MissingSessionCwdError
	if !errors.As(err, &missingErr) {
		t.Fatalf("runtime error = %T %v, want MissingSessionCwdError", err, err)
	}
	if createRuntimeCalled {
		t.Fatalf("runtime factory should not be called")
	}
}

func writeSessionCwdFile(t *testing.T, path, cwd string) {
	t.Helper()
	content := `{"type":"session","version":3,"id":"session-id","timestamp":"` + time.Now().UTC().Format(time.RFC3339) + `","cwd":"` + filepath.ToSlash(cwd) + `"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write session file: %v", err)
	}
}
