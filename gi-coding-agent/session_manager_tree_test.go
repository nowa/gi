package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func testUserMessage(text string) map[string]any {
	return map[string]any{"role": "user", "content": text, "timestamp": float64(1)}
}

func testAssistantMessage(text string) map[string]any {
	return map[string]any{
		"role":      "assistant",
		"content":   []any{map[string]any{"type": "text", "text": text}},
		"api":       "anthropic-messages",
		"provider":  "anthropic",
		"model":     "test-model",
		"usage":     map[string]any{"input": float64(1), "output": float64(1), "totalTokens": float64(2)},
		"timestamp": float64(2),
	}
}

func messageText(message any) string {
	value, _ := message.(map[string]any)
	content := value["content"]
	if text, ok := content.(string); ok {
		return text
	}
	parts, _ := content.([]any)
	if len(parts) == 0 {
		return ""
	}
	part, _ := parts[0].(map[string]any)
	text, _ := part["text"].(string)
	return text
}

func TestSessionManagerTreeAppendAndBranchMatchesPi(t *testing.T) {
	session, err := InMemorySessionManager()
	if err != nil {
		t.Fatal(err)
	}
	if session.GetLeafID() != nil {
		t.Fatalf("new in-memory leaf = %v, want nil", *session.GetLeafID())
	}

	id1 := session.AppendMessage(testUserMessage("first"))
	id2 := session.AppendMessage(testAssistantMessage("second"))
	id3 := session.AppendThinkingLevelChange("high")
	id4 := session.AppendModelChange("openai", "gpt-4")

	entries := session.GetEntries()
	if len(entries) != 4 {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[0].ID != id1 || entries[0].ParentID != nil || entries[0].Type != "message" {
		t.Fatalf("first entry = %#v", entries[0])
	}
	for idx, wantParent := range []string{id1, id2, id3} {
		if entries[idx+1].ParentID == nil || *entries[idx+1].ParentID != wantParent {
			t.Fatalf("entry %d parent = %v, want %q", idx+1, entries[idx+1].ParentID, wantParent)
		}
	}
	if got := session.GetLeafID(); got == nil || *got != id4 {
		t.Fatalf("leaf = %v, want %q", got, id4)
	}

	session.Branch(id2)
	branchID := session.AppendMessage(testUserMessage("branch"))
	if got := session.GetLeafID(); got == nil || *got != branchID {
		t.Fatalf("branch leaf = %v, want %q", got, branchID)
	}
	if entry := session.GetEntry(branchID); entry == nil || entry.ParentID == nil || *entry.ParentID != id2 {
		t.Fatalf("branched entry = %#v", entry)
	}

	path := session.GetBranch()
	var ids []string
	for _, entry := range path {
		ids = append(ids, entry.ID)
	}
	if want := []string{id1, id2, branchID}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("branch path = %#v, want %#v", ids, want)
	}

	tree := session.GetTree()
	if len(tree) != 1 || tree[0].Entry.ID != id1 || len(tree[0].Children) != 1 {
		t.Fatalf("tree root = %#v", tree)
	}
	node2 := tree[0].Children[0]
	if node2.Entry.ID != id2 || len(node2.Children) != 2 {
		t.Fatalf("branched node = %#v", node2)
	}
	childIDs := []string{node2.Children[0].Entry.ID, node2.Children[1].Entry.ID}
	if !sameStringSet(childIDs, []string{id3, branchID}) {
		t.Fatalf("branch child ids = %#v, want %q and %q", childIDs, id3, branchID)
	}

	if err := session.Branch("missing"); err == nil || !strings.Contains(err.Error(), "Entry missing not found") {
		t.Fatalf("missing branch err = %v", err)
	}
}

func TestSessionManagerCustomEntriesAndContextMatchPi(t *testing.T) {
	session, err := InMemorySessionManager()
	if err != nil {
		t.Fatal(err)
	}
	msgID := session.AppendMessage(testUserMessage("hello"))
	customID := session.AppendCustomEntry("my_data", map[string]any{"foo": "bar"})
	assistantID := session.AppendMessage(testAssistantMessage("hi"))

	entries := session.GetEntries()
	if len(entries) != 3 {
		t.Fatalf("entries = %#v", entries)
	}
	custom := session.GetEntry(customID)
	if custom == nil || custom.Type != "custom" || custom.CustomType != "my_data" || custom.ParentID == nil || *custom.ParentID != msgID {
		t.Fatalf("custom entry = %#v", custom)
	}
	if !reflect.DeepEqual(custom.Data, map[string]any{"foo": "bar"}) {
		t.Fatalf("custom data = %#v", custom.Data)
	}

	path := session.GetBranch()
	gotPath := []string{path[0].ID, path[1].ID, path[2].ID}
	if want := []string{msgID, customID, assistantID}; !reflect.DeepEqual(gotPath, want) {
		t.Fatalf("path = %#v, want %#v", gotPath, want)
	}

	ctx := session.BuildSessionContext()
	if len(ctx.Messages) != 2 {
		t.Fatalf("context messages = %#v, want only message entries", ctx.Messages)
	}
	if messageText(ctx.Messages[0]) != "hello" || messageText(ctx.Messages[1]) != "hi" {
		t.Fatalf("context message texts = %#v", ctx.Messages)
	}
	if ctx.Model == nil || ctx.Model.Provider != "anthropic" || ctx.Model.ModelID != "test-model" {
		t.Fatalf("context model = %#v", ctx.Model)
	}
}

func TestSessionManagerBranchSummaryContextAndForkPersistenceMatchPi(t *testing.T) {
	session, err := InMemorySessionManager()
	if err != nil {
		t.Fatal(err)
	}
	rootID := session.AppendMessage(testUserMessage("one"))
	session.AppendMessage(testAssistantMessage("two"))
	session.AppendMessage(testUserMessage("three"))
	summaryID, err := session.BranchWithSummary(&rootID, "Summary of abandoned work")
	if err != nil {
		t.Fatal(err)
	}
	if got := session.GetLeafID(); got == nil || *got != summaryID {
		t.Fatalf("summary leaf = %v, want %q", got, summaryID)
	}
	ctx := session.BuildSessionContext()
	if len(ctx.Messages) != 2 || messageText(ctx.Messages[0]) != "one" || messageText(ctx.Messages[1]) != "Summary of abandoned work" {
		t.Fatalf("branch summary context = %#v", ctx.Messages)
	}

	dir := t.TempDir()
	persisted, err := CreateSessionManager(dir, dir)
	if err != nil {
		t.Fatal(err)
	}
	firstID := persisted.AppendMessage(testUserMessage("first question"))
	persisted.AppendMessage(testAssistantMessage("first answer"))
	persisted.AppendMessage(testUserMessage("second question"))

	newFile, err := persisted.CreateBranchedSession(firstID)
	if err != nil {
		t.Fatal(err)
	}
	if newFile == "" {
		t.Fatalf("persisted branch file should be returned")
	}
	if _, err := os.Stat(newFile); !os.IsNotExist(err) {
		t.Fatalf("branch from user-only path should defer file creation, stat err=%v", err)
	}
	persisted.AppendCustomEntry("preset-state", map[string]any{"name": "plan"})
	persisted.AppendMessage(testAssistantMessage("new answer"))
	records := readSessionJSONL(t, newFile)
	if countEntryType(records, "session") != 1 {
		t.Fatalf("deferred branch file should have one header, records=%#v", records)
	}
	if duplicateIDs(records) {
		t.Fatalf("deferred branch file has duplicate ids: %#v", records)
	}

	persistedWithAssistant, err := CreateSessionManager(dir, dir)
	if err != nil {
		t.Fatal(err)
	}
	persistedWithAssistant.AppendMessage(testUserMessage("first question"))
	secondID := persistedWithAssistant.AppendMessage(testAssistantMessage("first answer"))
	persistedWithAssistant.AppendMessage(testUserMessage("second question"))
	withAssistant, err := persistedWithAssistant.CreateBranchedSession(secondID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(withAssistant); err != nil {
		t.Fatalf("branch including assistant should write immediately: %v", err)
	}
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := map[string]int{}
	for _, value := range a {
		counts[value]++
	}
	for _, value := range b {
		counts[value]--
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func readSessionJSONL(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("bad json line %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func countEntryType(records []map[string]any, entryType string) int {
	count := 0
	for _, record := range records {
		if record["type"] == entryType {
			count++
		}
	}
	return count
}

func duplicateIDs(records []map[string]any) bool {
	seen := map[string]struct{}{}
	for _, record := range records {
		if record["type"] == "session" {
			continue
		}
		id, _ := record["id"].(string)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			return true
		}
		seen[id] = struct{}{}
	}
	return false
}
