package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

var uuidV7Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestSessionManagerMigrationAndCustomSessionIDsMatchPi(t *testing.T) {
	entries := []FileEntry{
		{Type: "session", ID: "sess-1", Timestamp: "2025-01-01T00:00:00Z", CWD: "/tmp"},
		{Type: "message", Timestamp: "2025-01-01T00:00:01Z", Message: testUserMessage("hi")},
		{Type: "compaction", Timestamp: "2025-01-01T00:00:02Z", Summary: "old", raw: map[string]any{
			"type":                "compaction",
			"timestamp":           "2025-01-01T00:00:02Z",
			"summary":             "old",
			"firstKeptEntryIndex": float64(1),
			"tokensBefore":        float64(99),
		}},
		{Type: "message", Timestamp: "2025-01-01T00:00:03Z", Message: map[string]any{"role": "hookMessage", "content": "from hook", "timestamp": float64(3)}},
	}

	if !MigrateSessionEntries(entries) {
		t.Fatalf("migration should report changed entries")
	}
	if entries[0].Version != CurrentSessionVersion {
		t.Fatalf("header version = %d, want %d", entries[0].Version, CurrentSessionVersion)
	}
	for idx := 1; idx < len(entries); idx++ {
		if len(entries[idx].ID) != 8 {
			t.Fatalf("entry %d id = %q, want 8 chars", idx, entries[idx].ID)
		}
	}
	if entries[1].ParentID != nil {
		t.Fatalf("first migrated entry parent = %v, want nil", entries[1].ParentID)
	}
	if entries[2].ParentID == nil || *entries[2].ParentID != entries[1].ID {
		t.Fatalf("second migrated parent = %v, want %q", entries[2].ParentID, entries[1].ID)
	}
	if entries[2].FirstKeptID != entries[1].ID {
		t.Fatalf("compaction first kept = %q, want %q", entries[2].FirstKeptID, entries[1].ID)
	}
	if role := messageRole(entries[3].Message); role != "custom" {
		t.Fatalf("hook message role = %q, want custom", role)
	}

	already := []FileEntry{
		{Type: "session", ID: "sess-2", Version: 2, Timestamp: "2025-01-01T00:00:00Z", CWD: "/tmp"},
		{Type: "message", ID: "abc12345", Timestamp: "2025-01-01T00:00:01Z", Message: testUserMessage("hi")},
		{Type: "message", ID: "def67890", ParentID: stringPtr("abc12345"), Timestamp: "2025-01-01T00:00:02Z", Message: testAssistantMessage("hello")},
	}
	MigrateSessionEntries(already)
	if already[1].ID != "abc12345" || already[2].ID != "def67890" || already[2].ParentID == nil || *already[2].ParentID != "abc12345" {
		t.Fatalf("v2 ids should be stable after migration: %#v", already)
	}

	session, err := InMemorySessionManager()
	if err != nil {
		t.Fatal(err)
	}
	session.NewSession(NewSessionOptions{ID: "my-custom-id"})
	if session.GetSessionID() != "my-custom-id" || session.GetHeader().ID != "my-custom-id" {
		t.Fatalf("custom session id not preserved: id=%q header=%#v", session.GetSessionID(), session.GetHeader())
	}
	session.NewSession(NewSessionOptions{ParentSession: "parent.jsonl"})
	if !uuidV7Pattern.MatchString(session.GetSessionID()) || session.GetHeader().ParentSession != "parent.jsonl" {
		t.Fatalf("generated session header = %#v", session.GetHeader())
	}
}

func TestSessionManagerOpenMigratesAndRewritesLegacyFilesMatchPi(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.jsonl")
	writeJSONL(t, path,
		map[string]any{"type": "session", "id": "sess", "timestamp": "2025-01-01T00:00:00Z", "cwd": dir},
		map[string]any{"type": "message", "timestamp": "2025-01-01T00:00:01Z", "message": testUserMessage("hi")},
		map[string]any{"type": "message", "timestamp": "2025-01-01T00:00:02Z", "message": map[string]any{"role": "hookMessage", "content": "legacy", "timestamp": float64(2)}},
	)

	session, err := OpenSessionManager(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	entries := session.GetEntries()
	if len(entries) != 2 {
		t.Fatalf("loaded entries = %#v", entries)
	}
	if entries[0].ID == "" || entries[1].ParentID == nil || *entries[1].ParentID != entries[0].ID {
		t.Fatalf("migrated parent chain = %#v", entries)
	}
	if role := messageRole(entries[1].Message); role != "custom" {
		t.Fatalf("migrated message role = %q, want custom", role)
	}
	records := readSessionJSONL(t, path)
	if int(records[0]["version"].(float64)) != CurrentSessionVersion {
		t.Fatalf("rewritten header = %#v", records[0])
	}
	if records[1]["id"] == "" || records[2]["parentId"] != records[1]["id"] {
		t.Fatalf("rewritten records missing migrated ids: %#v", records)
	}
}

func TestSessionManagerLabelsResetLeafAndBranchPreservationMatchPi(t *testing.T) {
	session, err := InMemorySessionManager()
	if err != nil {
		t.Fatal(err)
	}
	msg1 := session.AppendMessage(testUserMessage("hello"))
	msg2 := session.AppendMessage(testAssistantMessage("hi"))
	msg3 := session.AppendMessage(testUserMessage("off path"))

	if _, ok := session.GetLabel(msg1); ok {
		t.Fatalf("new entry should not have label")
	}
	label1, err := session.AppendLabelChange(msg1, "first")
	if err != nil {
		t.Fatal(err)
	}
	session.AppendLabelChange(msg1, "second")
	label2, err := session.AppendLabelChange(msg2, "response")
	if err != nil {
		t.Fatal(err)
	}
	session.AppendLabelChange(msg3, "discard")
	if got, _ := session.GetLabel(msg1); got != "second" {
		t.Fatalf("last label = %q, want second", got)
	}
	session.AppendLabelChange(msg1, "")
	if _, ok := session.GetLabel(msg1); ok {
		t.Fatalf("empty label should clear label")
	}
	session.AppendLabelChange(msg1, "important")

	tree := session.GetTree()
	msg1Node := tree[0]
	if msg1Node.Entry.ID != msg1 || msg1Node.Label != "important" {
		t.Fatalf("root label node = %#v", msg1Node)
	}
	msg2Node := msg1Node.Children[0]
	if msg2Node.Entry.ID != msg2 || msg2Node.Label != "response" {
		t.Fatalf("child label node = %#v", msg2Node)
	}
	entries := session.GetEntries()
	timestampByID := map[string]string{}
	for _, entry := range entries {
		timestampByID[entry.ID] = entry.Timestamp
	}
	if msg2Node.LabelTimestamp != timestampByID[label2] {
		t.Fatalf("label timestamp = %q, want %q", msg2Node.LabelTimestamp, timestampByID[label2])
	}
	if timestampByID[label1] == "" {
		t.Fatalf("first label entry should remain append-only")
	}

	session.ResetLeaf()
	if ctx := session.BuildSessionContext(); len(ctx.Messages) != 0 {
		t.Fatalf("reset leaf context = %#v, want empty", ctx.Messages)
	}

	if _, err := session.CreateBranchedSession(msg2); err != nil {
		t.Fatal(err)
	}
	if got, _ := session.GetLabel(msg1); got != "important" {
		t.Fatalf("path label msg1 = %q", got)
	}
	if got, _ := session.GetLabel(msg2); got != "response" {
		t.Fatalf("path label msg2 = %q", got)
	}
	if _, ok := session.GetLabel(msg3); ok {
		t.Fatalf("off-path label should not be preserved")
	}
}

func TestSessionManagerListAndForkFromMatchPi(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "older.jsonl")
	newer := filepath.Join(dir, "newer.jsonl")
	invalid := filepath.Join(dir, "invalid.jsonl")
	writeJSONL(t, older,
		map[string]any{"type": "session", "version": CurrentSessionVersion, "id": "older", "timestamp": "2025-01-01T00:00:00Z", "cwd": "/older"},
		map[string]any{"type": "session_info", "id": "older-name", "parentId": nil, "timestamp": "2025-01-01T00:00:01Z", "name": " Old name "},
		map[string]any{"type": "message", "id": "older-1", "parentId": "older-name", "timestamp": "2025-01-01T00:00:02Z", "message": map[string]any{"role": "user", "content": "old question", "timestamp": float64(1000)}},
	)
	writeJSONL(t, newer,
		map[string]any{"type": "session", "version": CurrentSessionVersion, "id": "newer", "timestamp": "2025-01-02T00:00:00Z", "cwd": "/newer", "parentSession": "parent.jsonl"},
		map[string]any{"type": "message", "id": "newer-1", "parentId": nil, "timestamp": "2025-01-02T00:00:01Z", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "new answer"}}, "provider": "anthropic", "model": "claude", "timestamp": float64(5000)}},
		map[string]any{"type": "message", "id": "newer-2", "parentId": "newer-1", "timestamp": "2025-01-02T00:00:02Z", "message": map[string]any{"role": "user", "content": "new question", "timestamp": float64(6000)}},
	)
	if err := os.WriteFile(invalid, []byte(`{"type":"message"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var progress [][2]int
	sessions := ListSessions("/ignored", dir, SessionListProgress(func(loaded, total int) {
		progress = append(progress, [2]int{loaded, total})
	}))
	if len(sessions) != 2 {
		t.Fatalf("sessions = %#v", sessions)
	}
	if sessions[0].ID != "newer" || sessions[1].ID != "older" {
		t.Fatalf("session order = %#v", []string{sessions[0].ID, sessions[1].ID})
	}
	if sessions[0].MessageCount != 2 || sessions[0].FirstMessage != "new question" || sessions[0].AllMessagesText != "new answer new question" {
		t.Fatalf("newer session info = %#v", sessions[0])
	}
	if sessions[0].ParentSessionPath != "parent.jsonl" || sessions[1].Name != "Old name" {
		t.Fatalf("session metadata = %#v %#v", sessions[0], sessions[1])
	}
	if len(progress) != 3 || progress[len(progress)-1] != [2]int{3, 3} {
		t.Fatalf("progress = %#v, want each jsonl file including invalid", progress)
	}

	forked, err := ForkSessionFrom(newer, dir, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !uuidV7Pattern.MatchString(forked.GetSessionID()) {
		t.Fatalf("forked id = %q, want uuidv7", forked.GetSessionID())
	}
	if forked.GetHeader().ParentSession != newer || forked.GetHeader().CWD != dir {
		t.Fatalf("forked header = %#v", forked.GetHeader())
	}
	gotEntries := forked.GetEntries()
	wantIDs := []string{"newer-1", "newer-2"}
	gotIDs := []string{gotEntries[0].ID, gotEntries[1].ID}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("forked entries = %#v, want %#v", gotIDs, wantIDs)
	}
}

func writeJSONL(t *testing.T, path string, records ...map[string]any) {
	t.Helper()
	var builder strings.Builder
	for _, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		builder.Write(line)
		builder.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
}
