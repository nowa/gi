package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadEntriesFromFileMatchesPiSessionHeaderValidation(t *testing.T) {
	dir := t.TempDir()

	if entries := LoadEntriesFromFile(filepath.Join(dir, "missing.jsonl")); len(entries) != 0 {
		t.Fatalf("missing file entries = %#v", entries)
	}

	empty := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if entries := LoadEntriesFromFile(empty); len(entries) != 0 {
		t.Fatalf("empty file entries = %#v", entries)
	}

	noHeader := filepath.Join(dir, "no-header.jsonl")
	if err := os.WriteFile(noHeader, []byte(`{"type":"message","id":"1"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if entries := LoadEntriesFromFile(noHeader); len(entries) != 0 {
		t.Fatalf("no-header entries = %#v", entries)
	}

	malformed := filepath.Join(dir, "malformed.jsonl")
	if err := os.WriteFile(malformed, []byte("not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if entries := LoadEntriesFromFile(malformed); len(entries) != 0 {
		t.Fatalf("malformed entries = %#v", entries)
	}

	valid := filepath.Join(dir, "valid.jsonl")
	if err := os.WriteFile(valid, []byte(
		`{"type":"session","id":"abc","timestamp":"2025-01-01T00:00:00Z","cwd":"/tmp"}`+"\n"+
			`{"type":"message","id":"1","parentId":null,"timestamp":"2025-01-01T00:00:01Z","message":{"role":"user","content":"hi","timestamp":1}}`+"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := LoadEntriesFromFile(valid)
	if len(entries) != 2 || entries[0].Type != "session" || entries[1].Type != "message" {
		t.Fatalf("valid entries = %#v", entries)
	}

	mixed := filepath.Join(dir, "mixed.jsonl")
	if err := os.WriteFile(mixed, []byte(
		`{"type":"session","id":"abc","timestamp":"2025-01-01T00:00:00Z","cwd":"/tmp"}`+"\n"+
			"not valid json\n"+
			`{"type":"message","id":"1","parentId":null,"timestamp":"2025-01-01T00:00:01Z","message":{"role":"user","content":"hi","timestamp":1}}`+"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	entries = LoadEntriesFromFile(mixed)
	if len(entries) != 2 || entries[0].Type != "session" || entries[1].Type != "message" {
		t.Fatalf("mixed entries = %#v", entries)
	}
}

func TestFindMostRecentSessionMatchesPiValidJsonlSelection(t *testing.T) {
	dir := t.TempDir()

	if got := FindMostRecentSession(dir); got != "" {
		t.Fatalf("empty dir most recent = %q", got)
	}
	if got := FindMostRecentSession(filepath.Join(dir, "missing")); got != "" {
		t.Fatalf("missing dir most recent = %q", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindMostRecentSession(dir); got != "" {
		t.Fatalf("non-jsonl most recent = %q", got)
	}

	invalid := filepath.Join(dir, "invalid.jsonl")
	if err := os.WriteFile(invalid, []byte(`{"type":"message"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindMostRecentSession(dir); got != "" {
		t.Fatalf("invalid jsonl most recent = %q", got)
	}

	older := filepath.Join(dir, "older.jsonl")
	newer := filepath.Join(dir, "newer.jsonl")
	if err := os.WriteFile(older, []byte(`{"type":"session","id":"old","timestamp":"2025-01-01T00:00:00Z","cwd":"/tmp"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte(`{"type":"session","id":"new","timestamp":"2025-01-01T00:00:00Z","cwd":"/tmp"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(older, base, base); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, base.Add(time.Second), base.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if got := FindMostRecentSession(dir); got != newer {
		t.Fatalf("most recent = %q, want %q", got, newer)
	}
}

func TestSessionManagerOpenRecoversCorruptedFilesLikePi(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		content string
	}{
		{name: "empty", content: ""},
		{name: "no-header", content: `{"type":"message","id":"abc","parentId":"orphaned","timestamp":"2025-01-01T00:00:00Z","message":{"role":"assistant","content":"test"}}` + "\n"},
		{name: "garbage", content: "garbage content\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".jsonl")
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}

			sm, err := OpenSessionManager(path, dir)
			if err != nil {
				t.Fatal(err)
			}
			if sm.GetSessionID() == "" {
				t.Fatal("session id is empty")
			}
			header := sm.GetHeader()
			if header == nil || header.Type != "session" {
				t.Fatalf("header = %#v", header)
			}
			if sm.GetSessionFile() != path {
				t.Fatalf("session file = %q, want explicit path %q", sm.GetSessionFile(), path)
			}

			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := nonEmptyLines(string(content))
			if len(lines) != 1 {
				t.Fatalf("recovered file lines = %#v", lines)
			}
			var recovered SessionHeader
			if err := json.Unmarshal([]byte(lines[0]), &recovered); err != nil {
				t.Fatal(err)
			}
			if recovered.Type != "session" || recovered.ID != sm.GetSessionID() {
				t.Fatalf("recovered header = %#v, session id = %q", recovered, sm.GetSessionID())
			}

			again, err := OpenSessionManager(path, dir)
			if err != nil {
				t.Fatal(err)
			}
			if again.GetSessionID() != sm.GetSessionID() {
				t.Fatalf("reopened session id = %q, want %q", again.GetSessionID(), sm.GetSessionID())
			}
		})
	}
}

func TestSessionManagerCreateContinueAndDefaultDirMatchPiPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := filepath.Join(string(filepath.Separator), "tmp", "project:one")

	defaultDir, err := GetDefaultSessionDir(cwd)
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(home, ".gi", "agent", "sessions", "--tmp-project-one--")
	if defaultDir != wantDir {
		t.Fatalf("default session dir = %q, want %q", defaultDir, wantDir)
	}

	sessionDir := filepath.Join(home, "sessions")
	created, err := CreateSessionManager(cwd, sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	if !created.IsPersisted() || created.GetSessionID() == "" {
		t.Fatalf("created session persisted/id = %v/%q", created.IsPersisted(), created.GetSessionID())
	}
	if created.GetSessionDir() != sessionDir {
		t.Fatalf("created session dir = %q, want %q", created.GetSessionDir(), sessionDir)
	}
	header := created.GetHeader()
	if header == nil || header.Type != "session" || header.CWD != cwd || header.Version != CurrentSessionVersion {
		t.Fatalf("created header = %#v", header)
	}

	recentFile := filepath.Join(sessionDir, "recent.jsonl")
	if err := os.WriteFile(recentFile, []byte(`{"type":"session","id":"recent","timestamp":"2025-01-01T00:00:00Z","cwd":"/from-file"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	continued, err := ContinueRecentSession(cwd, sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	if continued.GetSessionID() != "recent" || continued.GetSessionFile() != recentFile {
		t.Fatalf("continued session = id %q file %q", continued.GetSessionID(), continued.GetSessionFile())
	}
}

func TestListAllSessionsUsesConfiguredAgentDirPiStyle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	agentDir := filepath.Join(t.TempDir(), "agent")
	t.Setenv("GI_CODING_AGENT_DIR", agentDir)

	writeSessionHeader := func(root, encodedCWD, id, cwd string) {
		t.Helper()
		dir := filepath.Join(root, encodedCWD)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, id+".jsonl")
		header := `{"type":"session","version":3,"id":"` + id + `","timestamp":"2026-01-01T00:00:00Z","cwd":"` + cwd + `"}` + "\n"
		if err := os.WriteFile(path, []byte(header), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeSessionHeader(filepath.Join(agentDir, "sessions"), "--project--", "configured", "/project")
	writeSessionHeader(filepath.Join(home, ConfigDirName, "agent", "sessions"), "--leaked--", "leaked", "/leaked")

	sessions := ListAllSessions()
	if len(sessions) != 1 || sessions[0].ID != "configured" {
		t.Fatalf("sessions from configured agent dir = %#v", sessions)
	}
}

func TestListAllSessionsCanUseExplicitSessionRoot(t *testing.T) {
	firstRoot := filepath.Join(t.TempDir(), "first")
	secondRoot := filepath.Join(t.TempDir(), "second")
	writeSessionHeader := func(root, id string) {
		t.Helper()
		dir := filepath.Join(root, "--project--")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, id+".jsonl")
		header := `{"type":"session","version":3,"id":"` + id + `","timestamp":"2026-01-01T00:00:00Z","cwd":"/project"}` + "\n"
		if err := os.WriteFile(path, []byte(header), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSessionHeader(firstRoot, "first")
	writeSessionHeader(secondRoot, "second")

	sessions := ListAllSessions(firstRoot)
	if len(sessions) != 1 || sessions[0].ID != "first" {
		t.Fatalf("sessions from explicit root = %#v", sessions)
	}
}

func nonEmptyLines(content string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return lines
}
