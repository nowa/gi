package gicodingagent

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestCLIPrintModeSessionFlagsMatchPiSessionManagerSemantics(t *testing.T) {
	t.Run("continue uses most recent file from explicit session dir", func(t *testing.T) {
		cwd := t.TempDir()
		agentDir := filepath.Join(cwd, ".agent")
		sessionDir := filepath.Join(cwd, "sessions")
		older := writeCLITestSessionFile(t, filepath.Join(sessionDir, "older.jsonl"), "old-session", cwd, nil)
		newer := writeCLITestSessionFile(t, filepath.Join(sessionDir, "newer.jsonl"), "new-session", cwd, nil)
		base := time.Now().Add(-time.Hour)
		if err := os.Chtimes(older, base, base); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(newer, base.Add(time.Second), base.Add(time.Second)); err != nil {
			t.Fatal(err)
		}

		host, err := newDefaultCLIPrintModeHost(Args{
			Offline:    true,
			Model:      "openai/gpt-4o-mini",
			Continue:   true,
			SessionDir: sessionDir,
		}, CLIOptions{CWD: cwd, AgentDir: agentDir})
		if err != nil {
			t.Fatal(err)
		}
		session := host.(*agentSessionPrintModeHost).session
		if session.SessionManager.GetSessionID() != "new-session" || session.SessionManager.GetSessionFile() != newer {
			t.Fatalf("session id/file = %q/%q", session.SessionManager.GetSessionID(), session.SessionManager.GetSessionFile())
		}
	})

	t.Run("session id prefix opens local session", func(t *testing.T) {
		cwd := t.TempDir()
		sessionDir := filepath.Join(cwd, "sessions")
		sessionFile := writeCLITestSessionFile(t, filepath.Join(sessionDir, "session.jsonl"), "abcdef123456", cwd, nil)

		host, err := newDefaultCLIPrintModeHost(Args{
			Offline:    true,
			Model:      "openai/gpt-4o-mini",
			Session:    "abcdef",
			SessionDir: sessionDir,
		}, CLIOptions{CWD: cwd, AgentDir: filepath.Join(cwd, ".agent")})
		if err != nil {
			t.Fatal(err)
		}
		session := host.(*agentSessionPrintModeHost).session
		if session.SessionManager.GetSessionID() != "abcdef123456" || session.SessionManager.GetSessionFile() != sessionFile {
			t.Fatalf("session id/file = %q/%q", session.SessionManager.GetSessionID(), session.SessionManager.GetSessionFile())
		}
	})

	t.Run("fork id prefix copies source session into current project", func(t *testing.T) {
		cwd := t.TempDir()
		sessionDir := filepath.Join(cwd, "sessions")
		source := writeCLITestSessionFile(t, filepath.Join(sessionDir, "source.jsonl"), "source-session", cwd, []map[string]any{
			{"type": "message", "id": "m1", "parentId": nil, "timestamp": time.Now().UTC().Format(time.RFC3339), "message": map[string]any{"role": "user", "content": "hello", "timestamp": 1}},
		})

		host, err := newDefaultCLIPrintModeHost(Args{
			Offline:    true,
			Model:      "openai/gpt-4o-mini",
			Fork:       "source",
			SessionDir: sessionDir,
		}, CLIOptions{CWD: cwd, AgentDir: filepath.Join(cwd, ".agent")})
		if err != nil {
			t.Fatal(err)
		}
		session := host.(*agentSessionPrintModeHost).session
		if session.SessionManager.GetSessionID() == "source-session" {
			t.Fatalf("fork reused source id")
		}
		header := session.SessionManager.GetHeader()
		if header == nil || header.ParentSession != source {
			t.Fatalf("fork header = %#v, source = %q", header, source)
		}
		if got := session.SessionManager.BuildSessionContext().Messages; len(got) != 1 || extractMessageText(got[0]) != "hello" {
			t.Fatalf("forked context = %#v", got)
		}
	})
}

func TestCLIPrintModeSessionIDUsesOneValidatedSelectionFlow(t *testing.T) {
	t.Run("creates a missing exact id with a startup warning", func(t *testing.T) {
		cwd := t.TempDir()
		sessionDir := filepath.Join(cwd, "sessions")
		result, err := newCLIPrintModeSessionManager(
			Args{SessionID: "planned-session", SessionDir: sessionDir},
			cwd,
			filepath.Join(cwd, ".agent"),
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.SessionManager.GetSessionID() != "planned-session" {
			t.Fatalf("session id = %q", result.SessionManager.GetSessionID())
		}
		wantWarning := "No project session found with id 'planned-session'; creating a new session with that id."
		if len(result.StartupWarnings) != 1 || result.StartupWarnings[0] != wantWarning {
			t.Fatalf("startup warnings = %#v", result.StartupWarnings)
		}
	})

	t.Run("opens an existing exact id without warning", func(t *testing.T) {
		cwd := t.TempDir()
		sessionDir := filepath.Join(cwd, "sessions")
		sessionFile := writeCLITestSessionFile(
			t,
			filepath.Join(sessionDir, "existing.jsonl"),
			"existing-session",
			cwd,
			nil,
		)
		result, err := newCLIPrintModeSessionManager(
			Args{SessionID: "existing-session", SessionDir: sessionDir},
			cwd,
			filepath.Join(cwd, ".agent"),
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.SessionManager.GetSessionFile() != sessionFile ||
			result.SessionManager.GetSessionID() != "existing-session" {
			t.Fatalf(
				"opened session = %q/%q",
				result.SessionManager.GetSessionID(),
				result.SessionManager.GetSessionFile(),
			)
		}
		if len(result.StartupWarnings) != 0 {
			t.Fatalf("startup warnings = %#v", result.StartupWarnings)
		}
	})

	t.Run("assigns the exact id to ephemeral and forked sessions", func(t *testing.T) {
		cwd := t.TempDir()
		sessionDir := filepath.Join(cwd, "sessions")
		ephemeral, err := newCLIPrintModeSessionManager(
			Args{
				NoSession:    true,
				SessionID:    "ephemeral-session",
				SessionIDSet: true,
				SessionDir:   sessionDir,
			},
			cwd,
			filepath.Join(cwd, ".agent"),
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if ephemeral.SessionManager.GetSessionID() != "ephemeral-session" ||
			ephemeral.SessionManager.IsPersisted() {
			t.Fatalf(
				"ephemeral session = id %q persisted %v",
				ephemeral.SessionManager.GetSessionID(),
				ephemeral.SessionManager.IsPersisted(),
			)
		}

		source := writeCLITestSessionFile(
			t,
			filepath.Join(sessionDir, "source.jsonl"),
			"source-session",
			cwd,
			nil,
		)
		forked, err := newCLIPrintModeSessionManager(
			Args{
				Fork:       "source-session",
				SessionID:  "forked-session",
				SessionDir: sessionDir,
			},
			cwd,
			filepath.Join(cwd, ".agent"),
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if forked.SessionManager.GetSessionID() != "forked-session" {
			t.Fatalf("forked session id = %q", forked.SessionManager.GetSessionID())
		}
		header := forked.SessionManager.GetHeader()
		if header == nil || header.ParentSession != source {
			t.Fatalf("forked header = %#v", header)
		}
	})

	t.Run("rejects a fork target id that already exists locally", func(t *testing.T) {
		cwd := t.TempDir()
		sessionDir := filepath.Join(cwd, "sessions")
		writeCLITestSessionFile(
			t,
			filepath.Join(sessionDir, "source.jsonl"),
			"source-session",
			cwd,
			nil,
		)
		writeCLITestSessionFile(
			t,
			filepath.Join(sessionDir, "target.jsonl"),
			"target-session",
			cwd,
			nil,
		)
		_, err := newCLIPrintModeSessionManager(
			Args{
				Fork:       "source-session",
				SessionID:  "target-session",
				SessionDir: sessionDir,
			},
			cwd,
			filepath.Join(cwd, ".agent"),
			nil,
		)
		if err == nil || err.Error() != "Session already exists with id 'target-session'" {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestCLISessionIDValidationMatchesPiFlagContract(t *testing.T) {
	tests := []struct {
		name    string
		args    Args
		wantErr string
	}{
		{
			name:    "session conflict",
			args:    Args{SessionID: "custom", Session: "existing"},
			wantErr: "--session-id cannot be combined with --session",
		},
		{
			name:    "continue conflict",
			args:    Args{SessionID: "custom", Continue: true},
			wantErr: "--session-id cannot be combined with --continue",
		},
		{
			name:    "resume conflict",
			args:    Args{SessionID: "custom", Resume: true},
			wantErr: "--session-id cannot be combined with --resume",
		},
		{
			name:    "invalid id",
			args:    Args{SessionID: "-bad"},
			wantErr: sessionIDValidationMessage,
		},
		{
			name:    "explicit empty id",
			args:    Args{SessionIDSet: true},
			wantErr: sessionIDValidationMessage,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCLISessionFlags(tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}

	if err := validateCLISessionFlags(Args{
		NoSession: true,
		SessionID: "ephemeral",
	}); err != nil {
		t.Fatalf("--no-session with --session-id = %v", err)
	}
	if err := validateCLISessionFlags(Args{
		Fork:      "source",
		SessionID: "target",
	}); err != nil {
		t.Fatalf("--fork with --session-id = %v", err)
	}
}

func TestResolveCLISessionPathPrefersExactIDBeforeNewerPrefix(t *testing.T) {
	cwd := t.TempDir()
	sessionDir := filepath.Join(cwd, "sessions")
	exact := writeCLITestSessionFile(
		t,
		filepath.Join(sessionDir, "exact.jsonl"),
		"abc",
		cwd,
		nil,
	)
	prefix := writeCLITestSessionFile(
		t,
		filepath.Join(sessionDir, "prefix.jsonl"),
		"abcdef",
		cwd,
		nil,
	)
	base := time.Now().Add(-time.Hour)
	if err := os.Chtimes(exact, base, base); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(prefix, base.Add(time.Second), base.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	resolved := resolveCLISessionPath("abc", cwd, sessionDir)
	if resolved.Type != cliResolvedSessionLocal || resolved.Path != exact {
		t.Fatalf("resolved = %#v, want exact path %q", resolved, exact)
	}
}

func TestCLIPrintModeForkRejectsPiConflictingFlags(t *testing.T) {
	_, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		Model:     "openai/gpt-4o-mini",
		Fork:      "abc",
		NoSession: true,
	}, CLIOptions{CWD: t.TempDir(), AgentDir: filepath.Join(t.TempDir(), ".agent")})
	if err == nil || !strings.Contains(err.Error(), "--fork cannot be combined with --no-session") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunCLIExportSessionFileMatchesPiStandaloneExport(t *testing.T) {
	cwd := t.TempDir()
	sessionFile := writeCLITestSessionFile(t, filepath.Join(cwd, "session.jsonl"), "export-session", cwd, []map[string]any{
		{"type": "message", "id": "u1", "parentId": nil, "timestamp": time.Now().UTC().Format(time.RFC3339), "message": map[string]any{"role": "user", "content": "hello", "timestamp": 1}},
		{"type": "message", "id": "a1", "parentId": "u1", "timestamp": time.Now().UTC().Format(time.RFC3339), "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "done"}}, "timestamp": 2}},
	})
	outputPath := filepath.Join(cwd, "out.html")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := RunCLI(CLIOptions{
		Args:   []string{"--export", filepath.Base(sessionFile), filepath.Base(outputPath)},
		CWD:    cwd,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Exported to: "+outputPath) || stderr.String() != "" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	html := string(content)
	if !strings.Contains(html, `id="session-data"`) || !strings.Contains(html, `decodeSessionData`) {
		t.Fatalf("html = %q", html)
	}
	match := regexp.MustCompile(`<script id="session-data" type="application/json">([^<]+)</script>`).FindStringSubmatch(html)
	if match == nil {
		t.Fatalf("session data script missing in html = %q", html)
	}
	payload, err := base64.StdEncoding.DecodeString(match[1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"role":"user"`) ||
		!strings.Contains(string(payload), `"hello"`) ||
		!strings.Contains(string(payload), `"role":"assistant"`) ||
		!strings.Contains(string(payload), `"done"`) {
		t.Fatalf("decoded session data = %s", payload)
	}
}

func writeCLITestSessionFile(t *testing.T, path, id, cwd string, entries []map[string]any) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	header := map[string]any{
		"type":      "session",
		"version":   CurrentSessionVersion,
		"id":        id,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"cwd":       cwd,
	}
	all := append([]map[string]any{header}, entries...)
	var builder strings.Builder
	for _, entry := range all {
		line, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		builder.Write(line)
		builder.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
