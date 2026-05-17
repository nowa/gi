package harness

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func harnessUserMessage(text string) llm.Message {
	return llm.UserMessageText(text)
}

func harnessAssistantMessage(text string) llm.Message {
	return llm.Message{
		Role:       llm.RoleAssistant,
		Content:    []llm.ContentPart{llm.Text(text)},
		API:        "anthropic-messages",
		Provider:   "anthropic",
		Model:      "claude-sonnet-4-5",
		Usage:      llm.EmptyUsage(),
		StopReason: llm.StopReasonStop,
		Timestamp:  llm.NowMillis(),
	}
}

func TestInMemorySessionStorage(t *testing.T) {
	metadata := SessionMetadata{ID: "session-1", CreatedAt: "2026-01-01T00:00:00.000Z"}
	storage, err := NewInMemorySessionStorage(&metadata, nil)
	if err != nil {
		t.Fatalf("NewInMemorySessionStorage() error = %v", err)
	}
	if got := storage.Metadata(); got != metadata {
		t.Fatalf("metadata = %#v", got)
	}

	entry := Entry{Type: "message", ID: "entry-1", Timestamp: "2026-01-01T00:00:00.000Z", Message: harnessUserMessage("one")}
	initial := []Entry{entry}
	storage, err = NewInMemorySessionStorage(nil, initial)
	if err != nil {
		t.Fatalf("NewInMemorySessionStorage(initial) error = %v", err)
	}
	initial = append(initial, Entry{Type: "message", ID: "entry-2"})
	ids := entryIDs(storage.Entries())
	if !reflect.DeepEqual(ids, []string{"entry-1"}) {
		t.Fatalf("entry ids = %#v", ids)
	}
	leaf, ok, err := storage.LeafID()
	if err != nil || !ok || leaf != "entry-1" {
		t.Fatalf("leaf = %q %v %v", leaf, ok, err)
	}
	if err := storage.SetLeafID(nil); err != nil {
		t.Fatalf("SetLeafID(nil) error = %v", err)
	}
	_, ok, err = storage.LeafID()
	if err != nil || ok {
		t.Fatalf("leaf after nil = ok %v err %v", ok, err)
	}
	if last := storage.Entries()[len(storage.Entries())-1]; last.Type != "leaf" || last.TargetID != nil {
		t.Fatalf("last entry = %#v", last)
	}
	if err := storage.SetLeafID(stringPtr("missing")); err == nil || !strings.Contains(err.Error(), "Entry missing not found") {
		t.Fatalf("SetLeafID(missing) error = %v", err)
	}
}

func TestInMemorySessionStorageLabelsAndPath(t *testing.T) {
	root := Entry{Type: "message", ID: "root", Timestamp: "2026-01-01T00:00:00.000Z", Message: harnessUserMessage("root")}
	child := Entry{Type: "message", ID: "child", ParentID: stringPtr("root"), Timestamp: "2026-01-01T00:00:01.000Z", Message: harnessAssistantMessage("child")}
	storage, err := NewInMemorySessionStorage(nil, []Entry{root, child})
	if err != nil {
		t.Fatal(err)
	}
	if got := entryIDs(storage.FindEntries("message")); !reflect.DeepEqual(got, []string{"root", "child"}) {
		t.Fatalf("FindEntries = %#v", got)
	}
	label := "checkpoint"
	if err := storage.AppendEntry(Entry{Type: "label", ID: "label-1", ParentID: stringPtr("child"), Timestamp: nowISO(), TargetID: stringPtr("root"), Label: &label}); err != nil {
		t.Fatal(err)
	}
	if got, ok := storage.Label("root"); !ok || got != "checkpoint" {
		t.Fatalf("label = %q %v", got, ok)
	}
	if err := storage.AppendEntry(Entry{Type: "label", ID: "label-2", ParentID: stringPtr("label-1"), Timestamp: nowISO(), TargetID: stringPtr("root")}); err != nil {
		t.Fatal(err)
	}
	if _, ok := storage.Label("root"); ok {
		t.Fatal("label should be removed")
	}
	path, err := storage.PathToRoot(stringPtr("child"))
	if err != nil {
		t.Fatal(err)
	}
	if got := entryIDs(path); !reflect.DeepEqual(got, []string{"root", "child"}) {
		t.Fatalf("path = %#v", got)
	}
}

func TestJsonlSessionStorage(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "session.jsonl")
	storage, err := CreateJsonlSessionStorage(filePath, SessionMetadata{ID: "session-1", CreatedAt: "2026-01-01T00:00:00.000Z", CWD: dir, ParentSessionPath: "/tmp/parent.jsonl"})
	if err != nil {
		t.Fatalf("CreateJsonlSessionStorage() error = %v", err)
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("stat session: %v", err)
	}
	if len(storage.Entries()) != 0 {
		t.Fatalf("entries = %#v", storage.Entries())
	}
	if err := storage.AppendEntry(Entry{Type: "message", ID: "user-1", Timestamp: "2026-01-01T00:00:00.000Z", Message: harnessUserMessage("one")}); err != nil {
		t.Fatalf("AppendEntry() error = %v", err)
	}
	content, _ := os.ReadFile(filePath)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"type":"session"`) || !strings.Contains(lines[1], `"id":"user-1"`) {
		t.Fatalf("jsonl lines = %#v", lines)
	}
	metadata, err := LoadJsonlSessionMetadata(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ID != "session-1" || metadata.CWD != dir || metadata.Path != filePath || metadata.ParentSessionPath != "/tmp/parent.jsonl" {
		t.Fatalf("metadata = %#v", metadata)
	}
	loaded, err := OpenJsonlSessionStorage(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := entryIDs(loaded.Entries()); !reflect.DeepEqual(got, []string{"user-1"}) {
		t.Fatalf("loaded entries = %#v", got)
	}
}

func TestJsonlSessionStorageRejectsMalformedFiles(t *testing.T) {
	_, err := OpenJsonlSessionStorage(filepath.Join(t.TempDir(), "missing.jsonl"))
	if err == nil {
		t.Fatal("expected missing file error")
	}
	var sessionErr *SessionError
	if !errors.As(err, &sessionErr) || sessionErr.Code != "not_found" {
		t.Fatalf("missing err = %#v", err)
	}

	filePath := filepath.Join(t.TempDir(), "session.jsonl")
	mustWrite(t, filePath, "not json\n")
	_, err = OpenJsonlSessionStorage(filePath)
	if err == nil || !strings.Contains(err.Error(), "first line is not a valid session header") {
		t.Fatalf("malformed header err = %v", err)
	}

	header := `{"type":"session","version":3,"id":"session-1","timestamp":"2026-01-01T00:00:00.000Z","cwd":"/tmp"}`
	mustWrite(t, filePath, header+"\nnot json\n")
	_, err = OpenJsonlSessionStorage(filePath)
	if !errors.As(err, &sessionErr) || sessionErr.Code != "invalid_entry" {
		t.Fatalf("malformed entry err = %#v", err)
	}
}

func entryIDs(entries []Entry) []string {
	ids := make([]string, len(entries))
	for i, entry := range entries {
		ids[i] = entry.ID
	}
	return ids
}
