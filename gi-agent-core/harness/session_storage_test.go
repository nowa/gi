package harness

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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
	if got := storage.Metadata(); !reflect.DeepEqual(got, metadata) {
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
	storage, err := CreateJsonlSessionStorage(filePath, SessionMetadata{
		ID:                "session-1",
		CreatedAt:         "2026-01-01T00:00:00.000Z",
		CWD:               dir,
		ParentSessionPath: "/tmp/parent.jsonl",
		Metadata:          map[string]any{"source": "test"},
	})
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
	if metadata.ID != "session-1" || metadata.CWD != dir || metadata.Path != filePath || metadata.ParentSessionPath != "/tmp/parent.jsonl" ||
		!reflect.DeepEqual(metadata.Metadata, map[string]any{"source": "test"}) {
		t.Fatalf("metadata = %#v", metadata)
	}
	loaded, err := OpenJsonlSessionStorage(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := entryIDs(loaded.Entries()); !reflect.DeepEqual(got, []string{"user-1"}) {
		t.Fatalf("loaded entries = %#v", got)
	}
	if !reflect.DeepEqual(loaded.Metadata().Metadata, map[string]any{"source": "test"}) {
		t.Fatalf("loaded header metadata = %#v", loaded.Metadata().Metadata)
	}
}

func TestSessionStorageStatsAndCursor(t *testing.T) {
	assistant := harnessAssistantMessage("answer")
	assistant.Usage = llm.Usage{
		Input:      10,
		Output:     4,
		CacheRead:  3,
		CacheWrite: 2,
		Cost:       llm.UsageCost{Total: 0.5},
	}
	compactionUsage := llm.Usage{
		Input:      5,
		Output:     2,
		CacheRead:  1,
		CacheWrite: 1,
		Cost:       llm.UsageCost{Total: 0.2},
	}
	storage, err := NewInMemorySessionStorage(nil, []Entry{
		{Type: "message", ID: "user", Timestamp: nowISO(), Message: harnessUserMessage("question")},
		{Type: "message", ID: "assistant", ParentID: stringPtr("user"), Timestamp: nowISO(), Message: assistant},
		{Type: "compaction", ID: "compaction", ParentID: stringPtr("assistant"), Timestamp: nowISO(), Usage: &compactionUsage},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got, want := storage.SessionStats(), (SessionStats{
		MessageCount:   2,
		CachedTokens:   4,
		UncachedTokens: 18,
		TotalTokens:    28,
		CostTotal:      0.7,
	}); got != want {
		t.Fatalf("stats = %#v, want %#v", got, want)
	}
	if got, want := entryIDs(storage.Entries(SessionEntryCursorOptions{AfterEntrySeq: 1, Limit: 1})), []string{"assistant"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("cursor entries = %#v, want %#v", got, want)
	}
	if got, want := entryIDs(storage.Entries(SessionEntryCursorOptions{AfterEntrySeq: 1})), []string{"assistant", "compaction"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unbounded cursor entries = %#v, want %#v", got, want)
	}
	if got := storage.Entries(SessionEntryCursorOptions{AfterEntrySeq: 99, Limit: 10}); len(got) != 0 {
		t.Fatalf("out-of-range cursor entries = %#v", got)
	}
}

func TestInMemorySessionStorageOwnsStateAndSupportsConcurrentAccess(t *testing.T) {
	activeToolNames := []string{"read"}
	storage, err := NewInMemorySessionStorage(nil, []Entry{{
		Type:            "active_tools_change",
		ID:              "initial",
		Timestamp:       nowISO(),
		ActiveToolNames: activeToolNames,
	}})
	if err != nil {
		t.Fatal(err)
	}
	activeToolNames[0] = "mutated"
	entry, ok := storage.Entry("initial")
	if !ok || !reflect.DeepEqual(entry.ActiveToolNames, []string{"read"}) {
		t.Fatalf("stored entry = %#v", entry)
	}
	entry.ActiveToolNames[0] = "returned mutation"
	entry, _ = storage.Entry("initial")
	if !reflect.DeepEqual(entry.ActiveToolNames, []string{"read"}) {
		t.Fatalf("entry mutation escaped storage: %#v", entry)
	}

	const appendCount = 64
	var waitGroup sync.WaitGroup
	waitGroup.Add(appendCount)
	for index := 0; index < appendCount; index++ {
		go func() {
			defer waitGroup.Done()
			id := storage.CreateEntryID()
			if err := storage.AppendEntry(Entry{Type: "custom", ID: id, Timestamp: nowISO()}); err != nil {
				t.Errorf("AppendEntry() error = %v", err)
			}
			_ = storage.Entries()
		}()
	}
	waitGroup.Wait()
	entries := storage.Entries()
	if got, want := len(entries), appendCount+1; got != want {
		t.Fatalf("entry count = %d, want %d", got, want)
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if _, exists := seen[entry.ID]; exists {
			t.Fatalf("duplicate entry id %q", entry.ID)
		}
		seen[entry.ID] = struct{}{}
	}
}

func TestJsonlAppendFailureDoesNotAdvanceMemoryState(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "session.jsonl")
	storage, err := CreateJsonlSessionStorage(filePath, SessionMetadata{
		ID:        "session-1",
		CreatedAt: nowISO(),
		CWD:       filepath.Dir(filePath),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filePath); err != nil {
		t.Fatal(err)
	}
	err = storage.AppendEntry(Entry{Type: "custom", ID: storage.CreateEntryID(), Timestamp: nowISO()})
	if err == nil {
		t.Fatal("AppendEntry() error = nil")
	}
	if got := storage.Entries(); len(got) != 0 {
		t.Fatalf("in-memory entries advanced after failed append: %#v", got)
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

func TestSessionStoragePiCaseNames(t *testing.T) {
	t.Run("copies initial entries and persists leaf changes", func(t *testing.T) {
		entry := Entry{Type: "message", ID: "entry-1", Timestamp: "2026-01-01T00:00:00.000Z", Message: harnessUserMessage("one")}
		initial := []Entry{entry}
		storage, err := NewInMemorySessionStorage(nil, initial)
		if err != nil {
			t.Fatal(err)
		}
		initial[0].ID = "mutated"
		if got := entryIDs(storage.Entries()); !reflect.DeepEqual(got, []string{"entry-1"}) {
			t.Fatalf("entries = %#v", got)
		}
		if err := storage.SetLeafID(nil); err != nil {
			t.Fatal(err)
		}
		if last := storage.Entries()[len(storage.Entries())-1]; last.Type != "leaf" || last.TargetID != nil {
			t.Fatalf("last entry = %#v", last)
		}
	})

	t.Run("finds entries by type", func(t *testing.T) {
		storage, err := NewInMemorySessionStorage(nil, []Entry{
			{Type: "message", ID: "message-1", Timestamp: nowISO(), Message: harnessUserMessage("one")},
			{Type: "label", ID: "label-1", Timestamp: nowISO(), TargetID: stringPtr("message-1"), Label: stringPtr("checkpoint")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := entryIDs(storage.FindEntries("message")); !reflect.DeepEqual(got, []string{"message-1"}) {
			t.Fatalf("message entries = %#v", got)
		}
	})

	t.Run("maintains label lookup", func(t *testing.T) {
		storage, err := NewInMemorySessionStorage(nil, []Entry{{Type: "message", ID: "root", Timestamp: nowISO(), Message: harnessUserMessage("root")}})
		if err != nil {
			t.Fatal(err)
		}
		if err := storage.AppendEntry(Entry{Type: "label", ID: "label-1", Timestamp: nowISO(), TargetID: stringPtr("root"), Label: stringPtr("checkpoint")}); err != nil {
			t.Fatal(err)
		}
		if got, ok := storage.Label("root"); !ok || got != "checkpoint" {
			t.Fatalf("label = %q %v", got, ok)
		}
		if err := storage.AppendEntry(Entry{Type: "label", ID: "label-2", Timestamp: nowISO(), TargetID: stringPtr("root")}); err != nil {
			t.Fatal(err)
		}
		if _, ok := storage.Label("root"); ok {
			t.Fatal("label should be removed")
		}
	})

	t.Run("walks paths to root", func(t *testing.T) {
		storage, err := NewInMemorySessionStorage(nil, []Entry{
			{Type: "message", ID: "root", Timestamp: nowISO(), Message: harnessUserMessage("root")},
			{Type: "message", ID: "child", ParentID: stringPtr("root"), Timestamp: nowISO(), Message: harnessAssistantMessage("child")},
		})
		if err != nil {
			t.Fatal(err)
		}
		path, err := storage.PathToRoot(stringPtr("child"))
		if err != nil {
			t.Fatal(err)
		}
		if got := entryIDs(path); !reflect.DeepEqual(got, []string{"root", "child"}) {
			t.Fatalf("path = %#v", got)
		}
	})

	t.Run("writes the header on create", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "session.jsonl")
		if _, err := CreateJsonlSessionStorage(filePath, SessionMetadata{ID: "session-1", CreatedAt: "2026-01-01T00:00:00.000Z", CWD: dir}); err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), `"type":"session"`) || !strings.Contains(string(content), `"version":3`) {
			t.Fatalf("header = %q", string(content))
		}
	})

	t.Run("loads existing entries and reconstructs leaf", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "session.jsonl")
		storage, err := CreateJsonlSessionStorage(filePath, SessionMetadata{ID: "session-1", CreatedAt: "2026-01-01T00:00:00.000Z", CWD: dir})
		if err != nil {
			t.Fatal(err)
		}
		if err := storage.AppendEntry(Entry{Type: "message", ID: "user-1", Timestamp: nowISO(), Message: harnessUserMessage("one")}); err != nil {
			t.Fatal(err)
		}
		if err := storage.SetLeafID(stringPtr("user-1")); err != nil {
			t.Fatal(err)
		}
		loaded, err := OpenJsonlSessionStorage(filePath)
		if err != nil {
			t.Fatal(err)
		}
		leaf, ok, err := loaded.LeafID()
		if err != nil || !ok || leaf != "user-1" {
			t.Fatalf("leaf = %q ok=%v err=%v", leaf, ok, err)
		}
	})
}

func entryIDs(entries []Entry) []string {
	ids := make([]string, len(entries))
	for i, entry := range entries {
		ids[i] = entry.ID
	}
	return ids
}
