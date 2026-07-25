package gicodingagent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionHeaderDiscoveryMatchesAuthoritativeLoader(t *testing.T) {
	dir := t.TempDir()
	storedCWD := filepath.Join(dir, "stored-project")

	cases := []struct {
		name   string
		prefix string
		id     string
	}{
		{name: "leading blank lines", prefix: "\n  \n", id: "blank-prefix"},
		{name: "leading malformed lines", prefix: "not json\n{broken json\n", id: "malformed-prefix"},
		{name: "multi-buffer header", id: strings.Repeat("a", 8192)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "-")+".jsonl")
			header := fmt.Sprintf(
				`{"type":"session","version":3,"id":%q,"timestamp":"2025-01-01T00:00:00Z","cwd":%q}`,
				tc.id,
				storedCWD,
			)
			if err := os.WriteFile(path, []byte(tc.prefix+header+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			manager, err := OpenSessionManager(path, dir)
			if err != nil {
				t.Fatal(err)
			}
			if manager.GetSessionID() != tc.id {
				t.Fatalf("session id = %q, want %q", manager.GetSessionID(), tc.id)
			}
			if manager.GetCWD() != storedCWD {
				t.Fatalf("cwd = %q, want %q", manager.GetCWD(), storedCWD)
			}
		})
	}
}

func TestOpenSessionManagerFallsBackBeyondHeaderScanLimit(t *testing.T) {
	dir := t.TempDir()
	storedCWD := filepath.Join(dir, "stored-project")
	overrideCWD := filepath.Join(dir, "override-project")
	cases := []struct {
		name   string
		id     string
		prefix string
	}{
		{name: "large-header", id: strings.Repeat("a", maxSessionHeaderScanBytes+1)},
		{
			name:   "large-prefix",
			id:     "large-prefix",
			prefix: strings.Repeat("x", maxSessionHeaderScanBytes+1) + "\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".jsonl")
			header := fmt.Sprintf(
				`{"type":"session","version":3,"id":%q,"timestamp":"2025-01-01T00:00:00Z","cwd":%q}`,
				tc.id,
				storedCWD,
			)
			if err := os.WriteFile(path, []byte(tc.prefix+header+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			manager, err := OpenSessionManager(path, dir)
			if err != nil {
				t.Fatal(err)
			}
			if manager.GetSessionID() != tc.id || manager.GetCWD() != storedCWD {
				t.Fatalf("opened session = id %q cwd %q", manager.GetSessionID(), manager.GetCWD())
			}

			overridden, err := OpenSessionManager(path, dir, overrideCWD)
			if err != nil {
				t.Fatal(err)
			}
			if overridden.GetSessionID() != tc.id || overridden.GetCWD() != overrideCWD {
				t.Fatalf("overridden session = id %q cwd %q", overridden.GetSessionID(), overridden.GetCWD())
			}
		})
	}
}

func TestReadSessionHeaderAllowsExactLimitAndRejectsAdditionalInput(t *testing.T) {
	dir := t.TempDir()
	fixedPrefix := `{"type":"session","version":3,"id":"`
	fixedSuffix := `","timestamp":"2025-01-01T00:00:00Z","cwd":"/tmp"}`
	idLength := maxSessionHeaderScanBytes - len(fixedPrefix) - len(fixedSuffix)
	if idLength <= 0 {
		t.Fatal("invalid test fixture size")
	}
	content := fixedPrefix + strings.Repeat("a", idLength) + fixedSuffix
	if len(content) != maxSessionHeaderScanBytes {
		t.Fatalf("fixture length = %d", len(content))
	}

	exact := filepath.Join(dir, "exact.jsonl")
	if err := os.WriteFile(exact, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	header, err := readSessionHeader(exact)
	if err != nil {
		t.Fatal(err)
	}
	if header == nil || len(header.ID) != idLength {
		t.Fatalf("header = %#v", header)
	}

	oversized := filepath.Join(dir, "oversized.jsonl")
	if err := os.WriteFile(oversized, []byte(content+"x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = readSessionHeader(oversized)
	var limitErr *SessionHeaderScanLimitError
	if !errors.As(err, &limitErr) || limitErr.Path != oversized {
		t.Fatalf("readSessionHeader() error = %v", err)
	}
}

func TestSessionDiscoveryFiltersCWDAndSkipsOversizedCorruption(t *testing.T) {
	dir := t.TempDir()
	projectA := filepath.Join(dir, "project-a")
	projectB := filepath.Join(dir, "project-b")
	fileA := filepath.Join(dir, "a.jsonl")
	fileB := filepath.Join(dir, "b.jsonl")
	oversized := filepath.Join(dir, "oversized.jsonl")
	writeTestSessionHeader(t, fileA, "a", projectA)
	writeTestSessionHeader(t, fileB, "b", projectB)
	if err := os.WriteFile(oversized, []byte(strings.Repeat("x", maxSessionHeaderScanBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(fileA, now.Add(-time.Second), now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(fileB, now, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(oversized, now.Add(time.Second), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	if got := FindMostRecentSession(dir); got != fileB {
		t.Fatalf("unfiltered recent = %q, want %q", got, fileB)
	}
	if got := FindMostRecentSession(dir, projectA); got != fileA {
		t.Fatalf("project A recent = %q, want %q", got, fileA)
	}
	if got := FindMostRecentSession(dir, projectB); got != fileB {
		t.Fatalf("project B recent = %q, want %q", got, fileB)
	}
}

func TestSetSessionFileRejectsInvalidInputWithoutChangingState(t *testing.T) {
	cwd := t.TempDir()
	manager, err := CreateSessionManager(cwd, filepath.Join(cwd, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	beforeID := manager.GetSessionID()
	beforeFile := manager.GetSessionFile()
	beforeEntries := manager.GetEntries()

	invalid := filepath.Join(cwd, "invalid.jsonl")
	original := `{"type":"message","id":"orphan"}` + "\n"
	if err := os.WriteFile(invalid, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetSessionFile(invalid); err == nil {
		t.Fatal("SetSessionFile() accepted invalid non-empty file")
	}
	if manager.GetSessionID() != beforeID || manager.GetSessionFile() != beforeFile {
		t.Fatalf(
			"manager changed after rejected file: id/file = %q/%q, want %q/%q",
			manager.GetSessionID(),
			manager.GetSessionFile(),
			beforeID,
			beforeFile,
		)
	}
	if got := manager.GetEntries(); len(got) != len(beforeEntries) {
		t.Fatalf("entries changed after rejected file: %#v", got)
	}
	content, err := os.ReadFile(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("invalid file changed: %q", content)
	}
}

func TestWriteSessionEntriesValidatesBeforeTruncating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	original := `{"type":"session","id":"preserved"}` + "\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []FileEntry{
		{Type: "session", ID: "replacement"},
		{Type: "custom", ID: "invalid", Data: make(chan int)},
	}
	if err := writeSessionEntries(
		path,
		entries,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
	); err == nil {
		t.Fatal("writeSessionEntries() accepted unsupported custom data")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("file was truncated before validation: %q", content)
	}
}

func TestSessionIDValidationAndOptionAwareFactories(t *testing.T) {
	validIDs := []string{"a", "abc-123_def.456", "Z9"}
	for _, id := range validIDs {
		if err := ValidateSessionID(id); err != nil {
			t.Fatalf("ValidateSessionID(%q) = %v", id, err)
		}
	}
	invalidIDs := []string{"", "-abc", "abc-", "_abc", "abc_", ".abc", "abc.", "abc/def", `abc\def`, "abc def"}
	for _, id := range invalidIDs {
		if err := ValidateSessionID(id); err == nil ||
			!strings.Contains(err.Error(), "Session id must be non-empty") {
			t.Fatalf("ValidateSessionID(%q) = %v", id, err)
		}
	}

	manager, err := InMemorySessionManagerWithOptions(
		t.TempDir(),
		NewSessionOptions{ID: "memory-session-id"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if manager.GetSessionID() != "memory-session-id" || manager.GetHeader().ID != "memory-session-id" {
		t.Fatalf("in-memory custom id = %q, header %#v", manager.GetSessionID(), manager.GetHeader())
	}
	beforeID := manager.GetSessionID()
	if _, err := manager.NewSession(NewSessionOptions{ID: "-invalid"}); err == nil {
		t.Fatal("NewSession() accepted invalid id")
	}
	if manager.GetSessionID() != beforeID {
		t.Fatalf("invalid NewSession changed id to %q", manager.GetSessionID())
	}

	cwd := t.TempDir()
	sessionDir := filepath.Join(cwd, "sessions")
	persisted, err := CreateSessionManagerWithOptions(
		cwd,
		sessionDir,
		NewSessionOptions{ID: "created-session-id"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.GetSessionID() != "created-session-id" ||
		!strings.Contains(filepath.Base(persisted.GetSessionFile()), "_created-session-id.jsonl") {
		t.Fatalf("persisted custom id/file = %q/%q", persisted.GetSessionID(), persisted.GetSessionFile())
	}

	source := filepath.Join(cwd, "source.jsonl")
	writeTestSessionHeader(t, source, "source-session-id", cwd)
	forkTime := time.Date(2026, 7, 25, 1, 2, 3, 4_000_000, time.UTC)
	forked, err := forkSessionFromAt(
		source,
		cwd,
		NewSessionOptions{ID: "forked-session-id"},
		forkTime,
		sessionDir,
	)
	if err != nil {
		t.Fatal(err)
	}
	if forked.GetSessionID() != "forked-session-id" ||
		forked.GetHeader().ParentSession != source {
		t.Fatalf("forked session = id %q header %#v", forked.GetSessionID(), forked.GetHeader())
	}
	if _, err := forkSessionFromAt(
		source,
		cwd,
		NewSessionOptions{ID: "forked-session-id"},
		forkTime,
		sessionDir,
	); !errors.Is(err, os.ErrExist) {
		t.Fatalf("duplicate fork error = %v, want os.ErrExist", err)
	}
}

func TestDefaultSessionPathIsPureAndUsesConfiguredAgentDir(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), "agent")
	cwd := filepath.Join(t.TempDir(), "project:one")
	t.Setenv("GI_CODING_AGENT_DIR", agentDir)

	sessionDir, err := GetDefaultSessionDirPath(cwd)
	if err != nil {
		t.Fatal(err)
	}
	safeCWD := strings.TrimLeft(cwd, `/\`)
	safeCWD = strings.NewReplacer("/", "-", `\`, "-", ":", "-").Replace(safeCWD)
	want := filepath.Join(agentDir, "sessions", "--"+safeCWD+"--")
	if sessionDir != want {
		t.Fatalf("default path = %q, want %q", sessionDir, want)
	}
	if _, err := os.Stat(sessionDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pure path lookup created directory or returned unexpected error: %v", err)
	}

	manager, err := CreateSessionManager(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if !manager.UsesDefaultSessionDir() {
		t.Fatalf("manager session dir %q should be default", manager.GetSessionDir())
	}
}

func TestFlatSessionDirectoryScopesListAndContinueByCWD(t *testing.T) {
	dir := t.TempDir()
	projectA := filepath.Join(dir, "project-a")
	projectB := filepath.Join(dir, "project-b")
	fileA := filepath.Join(dir, "a.jsonl")
	fileB := filepath.Join(dir, "b.jsonl")
	writeTestSessionHeader(t, fileA, "a", projectA)
	writeTestSessionHeader(t, fileB, "b", projectB)
	now := time.Now()
	if err := os.Chtimes(fileA, now.Add(-time.Second), now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(fileB, now, now); err != nil {
		t.Fatal(err)
	}

	sessions := ListSessions(projectA, dir)
	if len(sessions) != 1 || sessions[0].Path != fileA {
		t.Fatalf("project A sessions = %#v", sessions)
	}
	continued, err := ContinueRecentSession(projectA, dir)
	if err != nil {
		t.Fatal(err)
	}
	if continued.GetSessionFile() != fileA || continued.GetSessionID() != "a" {
		t.Fatalf("continued session = %q/%q", continued.GetSessionFile(), continued.GetSessionID())
	}
}

func TestSessionInfoActivityUsesOnlyUserAndAssistantContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "activity.jsonl")
	writeJSONL(t, path,
		map[string]any{
			"type": "session", "version": CurrentSessionVersion, "id": "activity",
			"timestamp": "2025-01-01T00:00:00Z", "cwd": "/tmp",
		},
		map[string]any{
			"type": "message", "id": "missing-content", "parentId": nil,
			"timestamp": "2025-01-04T00:00:00Z",
			"message":   map[string]any{"role": "assistant", "timestamp": float64(4_000)},
		},
		map[string]any{
			"type": "message", "id": "user", "parentId": "missing-content",
			"timestamp": "2025-01-02T00:00:00Z",
			"message":   map[string]any{"role": "user", "content": "hello", "timestamp": float64(2_000)},
		},
		map[string]any{
			"type": "message", "id": "custom", "parentId": "user",
			"timestamp": "2025-01-03T00:00:00Z",
			"message":   map[string]any{"role": "custom", "content": "ignored", "timestamp": float64(3_000)},
		},
	)

	info, err := BuildSessionInfo(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Modified.UnixMilli(), int64(2_000); got != want {
		t.Fatalf("modified = %d, want %d", got, want)
	}
}

func writeTestSessionHeader(t *testing.T, path, id, cwd string) {
	t.Helper()
	content := fmt.Sprintf(
		"{\"type\":\"session\",\"version\":3,\"id\":%q,\"timestamp\":\"2025-01-01T00:00:00Z\",\"cwd\":%q}\n",
		id,
		cwd,
	)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
