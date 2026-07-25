package gicodingagent

import (
	"reflect"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestBuildSessionContextPiParityCases(t *testing.T) {
	t.Run("empty entries returns empty context", func(t *testing.T) {
		ctx := BuildSessionContext(nil, nil, nil)
		if len(ctx.Messages) != 0 || ctx.ThinkingLevel != "off" || ctx.Model != nil {
			t.Fatalf("context = %#v", ctx)
		}
	})

	t.Run("single user message", func(t *testing.T) {
		ctx := BuildSessionContext([]FileEntry{contextUserEntry("1", nil, "hello")}, nil, nil)
		if len(ctx.Messages) != 1 || contextMessageRole(ctx.Messages[0]) != "user" {
			t.Fatalf("messages = %#v", ctx.Messages)
		}
	})

	t.Run("simple conversation", func(t *testing.T) {
		entries := []FileEntry{
			contextUserEntry("1", nil, "hello"),
			contextAssistantEntry("2", stringPtr("1"), "hi there"),
			contextUserEntry("3", stringPtr("2"), "how are you"),
			contextAssistantEntry("4", stringPtr("3"), "great"),
		}
		ctx := BuildSessionContext(entries, nil, nil)
		if got := contextMessageRoles(ctx.Messages); !reflect.DeepEqual(got, []string{"user", "assistant", "user", "assistant"}) {
			t.Fatalf("roles = %#v", got)
		}
	})

	t.Run("tracks thinking level changes", func(t *testing.T) {
		entries := []FileEntry{
			contextUserEntry("1", nil, "hello"),
			contextThinkingLevelEntry("2", stringPtr("1"), "high"),
			contextAssistantEntry("3", stringPtr("2"), "thinking hard"),
		}
		ctx := BuildSessionContext(entries, nil, nil)
		if ctx.ThinkingLevel != "high" || len(ctx.Messages) != 2 {
			t.Fatalf("context = %#v", ctx)
		}
	})

	t.Run("tracks model from assistant message", func(t *testing.T) {
		entries := []FileEntry{contextUserEntry("1", nil, "hello"), contextAssistantEntry("2", stringPtr("1"), "hi")}
		ctx := BuildSessionContext(entries, nil, nil)
		if ctx.Model == nil || ctx.Model.Provider != "anthropic" || ctx.Model.ModelID != "test-model" {
			t.Fatalf("model = %#v", ctx.Model)
		}
	})

	t.Run("tracks model from model change entry", func(t *testing.T) {
		entries := []FileEntry{
			contextUserEntry("1", nil, "hello"),
			contextModelChangeEntry("2", stringPtr("1"), "openai", "gpt-4"),
			contextAssistantEntry("3", stringPtr("2"), "hi"),
		}
		ctx := BuildSessionContext(entries, nil, nil)
		if ctx.Model == nil || ctx.Model.Provider != "anthropic" || ctx.Model.ModelID != "test-model" {
			t.Fatalf("model = %#v", ctx.Model)
		}
	})

	t.Run("includes summary before kept messages", func(t *testing.T) {
		entries := []FileEntry{
			contextUserEntry("1", nil, "first"),
			contextAssistantEntry("2", stringPtr("1"), "response1"),
			contextUserEntry("3", stringPtr("2"), "second"),
			contextAssistantEntry("4", stringPtr("3"), "response2"),
			contextCompactionEntry("5", stringPtr("4"), "Summary of first two turns", "3"),
			contextUserEntry("6", stringPtr("5"), "third"),
			contextAssistantEntry("7", stringPtr("6"), "response3"),
		}
		ctx := BuildSessionContext(entries, nil, nil)
		if len(ctx.Messages) != 5 || contextMessageRole(ctx.Messages[0]) != "compactionSummary" {
			t.Fatalf("messages = %#v", ctx.Messages)
		}
		summary := contextMessageMap(t, ctx.Messages[0])
		if summary["summary"] != "Summary of first two turns" || summary["tokensBefore"] != 1000 {
			t.Fatalf("summary = %#v", summary)
		}
		if contextMessageText(ctx.Messages[1]) != "second" || contextMessageText(ctx.Messages[4]) != "response3" {
			t.Fatalf("messages = %#v", ctx.Messages)
		}
	})

	t.Run("handles compaction keeping from first message", func(t *testing.T) {
		entries := []FileEntry{
			contextUserEntry("1", nil, "first"),
			contextAssistantEntry("2", stringPtr("1"), "response"),
			contextCompactionEntry("3", stringPtr("2"), "Empty summary", "1"),
			contextUserEntry("4", stringPtr("3"), "second"),
		}
		ctx := BuildSessionContext(entries, nil, nil)
		if len(ctx.Messages) != 4 || contextMessageRole(ctx.Messages[0]) != "compactionSummary" {
			t.Fatalf("messages = %#v", ctx.Messages)
		}
	})

	t.Run("provider projection excludes persisted retry entries", func(t *testing.T) {
		entries := []FileEntry{
			contextUserEntry("1", nil, "first"),
			contextAssistantEntry("2", stringPtr("1"), "retry error"),
			contextCompactionEntry("3", stringPtr("2"), "Summary", "1"),
			contextUserEntry("4", stringPtr("3"), "second"),
		}
		ctx := BuildSessionContextWithOptions(
			entries,
			stringPtr("4"),
			nil,
			SessionContextOptions{
				ExcludeEntryIDs: map[string]struct{}{"2": {}},
			},
		)
		if got := contextMessageRoles(ctx.Messages); !reflect.DeepEqual(
			got,
			[]string{"compactionSummary", "user", "user"},
		) {
			t.Fatalf("roles = %#v", got)
		}
		if contextMessageText(ctx.Messages[1]) != "first" ||
			contextMessageText(ctx.Messages[2]) != "second" {
			t.Fatalf("messages = %#v", ctx.Messages)
		}
	})

	t.Run("multiple compactions uses latest", func(t *testing.T) {
		entries := []FileEntry{
			contextUserEntry("1", nil, "a"),
			contextAssistantEntry("2", stringPtr("1"), "b"),
			contextCompactionEntry("3", stringPtr("2"), "First summary", "1"),
			contextUserEntry("4", stringPtr("3"), "c"),
			contextAssistantEntry("5", stringPtr("4"), "d"),
			contextCompactionEntry("6", stringPtr("5"), "Second summary", "4"),
			contextUserEntry("7", stringPtr("6"), "e"),
		}
		ctx := BuildSessionContext(entries, nil, nil)
		if len(ctx.Messages) != 4 || contextMessageMap(t, ctx.Messages[0])["summary"] != "Second summary" {
			t.Fatalf("messages = %#v", ctx.Messages)
		}
	})

	t.Run("follows path to specified leaf", func(t *testing.T) {
		entries := []FileEntry{
			contextUserEntry("1", nil, "start"),
			contextAssistantEntry("2", stringPtr("1"), "response"),
			contextUserEntry("3", stringPtr("2"), "branch A"),
			contextUserEntry("4", stringPtr("2"), "branch B"),
		}
		ctxA := BuildSessionContext(entries, stringPtr("3"), nil)
		ctxB := BuildSessionContext(entries, stringPtr("4"), nil)
		if contextMessageText(ctxA.Messages[2]) != "branch A" || contextMessageText(ctxB.Messages[2]) != "branch B" {
			t.Fatalf("ctxA=%#v ctxB=%#v", ctxA.Messages, ctxB.Messages)
		}
	})

	t.Run("includes branch summary in path", func(t *testing.T) {
		entries := []FileEntry{
			contextUserEntry("1", nil, "start"),
			contextAssistantEntry("2", stringPtr("1"), "response"),
			contextUserEntry("3", stringPtr("2"), "abandoned path"),
			contextBranchSummaryEntry("4", stringPtr("2"), "Summary of abandoned work", "3"),
			contextUserEntry("5", stringPtr("4"), "new direction"),
		}
		ctx := BuildSessionContext(entries, stringPtr("5"), nil)
		if len(ctx.Messages) != 4 || contextMessageMap(t, ctx.Messages[2])["summary"] != "Summary of abandoned work" {
			t.Fatalf("messages = %#v", ctx.Messages)
		}
	})

	t.Run("complex tree with multiple branches and compaction", func(t *testing.T) {
		entries := []FileEntry{
			contextUserEntry("1", nil, "start"),
			contextAssistantEntry("2", stringPtr("1"), "r1"),
			contextUserEntry("3", stringPtr("2"), "q2"),
			contextAssistantEntry("4", stringPtr("3"), "r2"),
			contextCompactionEntry("5", stringPtr("4"), "Compacted history", "3"),
			contextUserEntry("6", stringPtr("5"), "q3"),
			contextAssistantEntry("7", stringPtr("6"), "r3"),
			contextUserEntry("8", stringPtr("3"), "wrong path"),
			contextAssistantEntry("9", stringPtr("8"), "wrong response"),
			contextBranchSummaryEntry("10", stringPtr("3"), "Tried wrong approach", "9"),
			contextUserEntry("11", stringPtr("10"), "better approach"),
		}
		main := BuildSessionContext(entries, stringPtr("7"), nil)
		branch := BuildSessionContext(entries, stringPtr("11"), nil)
		if got := contextMessageRoles(main.Messages); !reflect.DeepEqual(got, []string{"compactionSummary", "user", "assistant", "user", "assistant"}) {
			t.Fatalf("main roles = %#v", got)
		}
		if got := contextMessageRoles(branch.Messages); !reflect.DeepEqual(got, []string{"user", "assistant", "user", "branchSummary", "user"}) {
			t.Fatalf("branch roles = %#v", got)
		}
	})

	t.Run("uses last entry when leafId not found", func(t *testing.T) {
		entries := []FileEntry{contextUserEntry("1", nil, "hello"), contextAssistantEntry("2", stringPtr("1"), "hi")}
		ctx := BuildSessionContext(entries, stringPtr("nonexistent"), nil)
		if len(ctx.Messages) != 2 {
			t.Fatalf("messages = %#v", ctx.Messages)
		}
	})

	t.Run("handles orphaned entries gracefully", func(t *testing.T) {
		entries := []FileEntry{
			contextUserEntry("1", nil, "hello"),
			contextAssistantEntry("2", stringPtr("missing"), "orphan"),
		}
		ctx := BuildSessionContext(entries, stringPtr("2"), nil)
		if len(ctx.Messages) != 1 || contextMessageText(ctx.Messages[0]) != "orphan" {
			t.Fatalf("messages = %#v", ctx.Messages)
		}
	})
}

func TestBuildContextEntriesReturnsCompactionAwareEntriesIncludingCustomEntries(t *testing.T) {
	entries := []FileEntry{
		contextUserEntry("1", nil, "first"),
		{Type: "custom_message", ID: "2", ParentID: stringPtr("1"), CustomType: "old-state", Content: "old"},
		contextAssistantEntry("3", stringPtr("2"), "response1"),
		{Type: "custom_message", ID: "4", ParentID: stringPtr("3"), CustomType: "kept-card", Content: "kept"},
		contextUserEntry("5", stringPtr("4"), "second"),
		contextCompactionEntry("6", stringPtr("5"), "Summary", "4"),
		{Type: "custom_message", ID: "7", ParentID: stringPtr("6"), CustomType: "after-card", Content: "after"},
		contextAssistantEntry("8", stringPtr("7"), "response2"),
	}

	contextEntries := BuildContextEntries(entries, nil, nil)
	ids := make([]string, 0, len(contextEntries))
	for _, entry := range contextEntries {
		ids = append(ids, entry.ID)
	}
	if want := []string{"6", "4", "5", "7", "8"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("context entry ids = %#v, want %#v", ids, want)
	}
}

func TestSessionEntryToContextMessagesProjectsDisplayAndSummaryEntries(t *testing.T) {
	display := false
	tests := []struct {
		name        string
		entry       FileEntry
		wantRole    string
		wantText    string
		wantDisplay *bool
		wantEmpty   bool
	}{
		{
			name:     "message",
			entry:    contextUserEntry("message", nil, "hello"),
			wantRole: "user",
			wantText: "hello",
		},
		{
			name: "custom message",
			entry: FileEntry{
				Type:       "custom_message",
				ID:         "custom",
				Timestamp:  "2025-01-01T00:00:00Z",
				CustomType: "card",
				Content:    "custom body",
				Display:    false,
			},
			wantRole:    "custom",
			wantText:    "custom body",
			wantDisplay: &display,
		},
		{
			name: "loaded assistant with null content",
			entry: FileEntry{
				Type: "message",
				ID:   "assistant-null",
				Message: map[string]any{
					"role":    "assistant",
					"content": nil,
				},
			},
			wantRole:  "assistant",
			wantEmpty: true,
		},
		{
			name: "loaded tool result with missing content",
			entry: FileEntry{
				Type: "message",
				ID:   "tool-missing",
				Message: map[string]any{
					"role":       "toolResult",
					"toolCallID": "call-1",
				},
			},
			wantRole:  "toolResult",
			wantEmpty: true,
		},
		{
			name: "custom message with null content",
			entry: FileEntry{
				Type:       "custom_message",
				ID:         "custom-null",
				CustomType: "card",
				Content:    nil,
			},
			wantRole:  "custom",
			wantEmpty: true,
		},
		{
			name: "branch summary",
			entry: contextBranchSummaryEntry(
				"branch",
				nil,
				"branch body",
				"source",
			),
			wantRole: "branchSummary",
			wantText: "branch body",
		},
		{
			name: "compaction",
			entry: contextCompactionEntry(
				"compaction",
				nil,
				"compaction body",
				"message",
			),
			wantRole: "compactionSummary",
			wantText: "compaction body",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			messages := SessionEntryToContextMessages(test.entry)
			if len(messages) != 1 {
				t.Fatalf("messages = %#v", messages)
			}
			message := messages[0]
			if message.Role != test.wantRole ||
				interactiveTextFromLLMMessage(message) != test.wantText {
				t.Fatalf("message = %#v", message)
			}
			if test.wantDisplay != nil &&
				(message.Display == nil || *message.Display != *test.wantDisplay) {
				t.Fatalf("display = %#v, want %t", message.Display, *test.wantDisplay)
			}
			if test.wantEmpty && (message.Content == nil || len(message.Content) != 0) {
				t.Fatalf("content = %#v, want non-nil empty content", message.Content)
			}
		})
	}

	if messages := SessionEntryToContextMessages(FileEntry{
		Type: "model_change",
		ID:   "metadata",
	}); len(messages) != 0 {
		t.Fatalf("metadata messages = %#v", messages)
	}
}

func TestAgentSessionMessageEndNormalizesNilContent(t *testing.T) {
	message, err := (&AgentSession{}).emitExtensionMessageEnd(llm.Message{
		Role: llm.RoleAssistant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if message.Content == nil || len(message.Content) != 0 {
		t.Fatalf("content = %#v, want non-nil empty content", message.Content)
	}
}

func contextUserEntry(id string, parentID *string, text string) FileEntry {
	return FileEntry{Type: "message", ID: id, ParentID: parentID, Timestamp: "2025-01-01T00:00:00Z", Message: testUserMessage(text)}
}

func contextAssistantEntry(id string, parentID *string, text string) FileEntry {
	return FileEntry{Type: "message", ID: id, ParentID: parentID, Timestamp: "2025-01-01T00:00:00Z", Message: testAssistantMessage(text)}
}

func contextThinkingLevelEntry(id string, parentID *string, level string) FileEntry {
	return FileEntry{Type: "thinking_level_change", ID: id, ParentID: parentID, Timestamp: "2025-01-01T00:00:00Z", ThinkingLevel: level}
}

func contextModelChangeEntry(id string, parentID *string, provider string, modelID string) FileEntry {
	return FileEntry{Type: "model_change", ID: id, ParentID: parentID, Timestamp: "2025-01-01T00:00:00Z", Provider: provider, ModelID: modelID}
}

func contextCompactionEntry(id string, parentID *string, summary string, firstKeptID string) FileEntry {
	return FileEntry{Type: "compaction", ID: id, ParentID: parentID, Timestamp: "2025-01-01T00:00:00Z", Summary: summary, FirstKeptID: firstKeptID, TokensBefore: 1000}
}

func contextBranchSummaryEntry(id string, parentID *string, summary string, fromID string) FileEntry {
	return FileEntry{Type: "branch_summary", ID: id, ParentID: parentID, Timestamp: "2025-01-01T00:00:00Z", Summary: summary, FromID: fromID}
}

func contextMessageRoles(messages []any) []string {
	roles := make([]string, 0, len(messages))
	for _, message := range messages {
		roles = append(roles, contextMessageRole(message))
	}
	return roles
}

func contextMessageRole(message any) string {
	value, _ := message.(map[string]any)
	role, _ := value["role"].(string)
	return role
}

func contextMessageText(message any) string {
	return extractMessageText(message)
}

func contextMessageMap(t *testing.T, message any) map[string]any {
	t.Helper()
	value, ok := message.(map[string]any)
	if !ok {
		t.Fatalf("message = %#v, want map", message)
	}
	return value
}
