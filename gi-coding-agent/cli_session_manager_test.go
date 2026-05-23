package gicodingagent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
	if !strings.Contains(html, `data-role="user">hello`) || !strings.Contains(html, `data-role="assistant">done`) {
		t.Fatalf("html = %q", html)
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
