package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func runSessionSuite(t *testing.T, name string, createStorage func(t *testing.T) SessionStorage, inspect func(t *testing.T)) {
	t.Run(name+"/appends messages and builds context", func(t *testing.T) {
		session := NewSession(createStorage(t))
		if _, err := session.AppendMessage(harnessUserMessage("one")); err != nil {
			t.Fatal(err)
		}
		if _, err := session.AppendMessage(harnessAssistantMessage("two")); err != nil {
			t.Fatal(err)
		}
		context, err := session.BuildContext()
		if err != nil {
			t.Fatal(err)
		}
		if got := messageRoles(context.Messages); !reflect.DeepEqual(got, []string{"user", "assistant"}) {
			t.Fatalf("roles = %#v", got)
		}
	})

	t.Run(name+"/tracks model and thinking level changes", func(t *testing.T) {
		session := NewSession(createStorage(t))
		_, _ = session.AppendMessage(harnessUserMessage("one"))
		_, _ = session.AppendModelChange("openai", "gpt-4.1")
		_, _ = session.AppendThinkingLevelChange("high")
		context, _ := session.BuildContext()
		if context.ThinkingLevel != "high" || context.ModelProvider != "openai" || context.ModelID != "gpt-4.1" {
			t.Fatalf("context = %#v", context)
		}
	})

	t.Run(name+"/supports branching", func(t *testing.T) {
		session := NewSession(createStorage(t))
		user1, _ := session.AppendMessage(harnessUserMessage("one"))
		assistant1, _ := session.AppendMessage(harnessAssistantMessage("two"))
		_, _ = session.AppendMessage(harnessUserMessage("three"))
		_, err := session.MoveTo(&user1, "")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = session.AppendMessage(harnessAssistantMessage("branched"))
		branch, _ := session.Branch(nil)
		ids := entryIDs(branch)
		if !contains(ids, user1) || contains(ids, assistant1) {
			t.Fatalf("branch ids = %#v", ids)
		}
		context, _ := session.BuildContext()
		if got := messageRoles(context.Messages); !reflect.DeepEqual(got, []string{"user", "assistant"}) {
			t.Fatalf("roles = %#v", got)
		}
	})

	t.Run(name+"/supports moving to root", func(t *testing.T) {
		session := NewSession(createStorage(t))
		_, _ = session.AppendMessage(harnessUserMessage("one"))
		_, err := session.MoveTo(nil, "")
		if err != nil {
			t.Fatal(err)
		}
		leaf, _ := session.LeafID()
		if leaf != nil {
			t.Fatalf("leaf = %v, want nil", *leaf)
		}
		context, _ := session.BuildContext()
		if len(context.Messages) != 0 {
			t.Fatalf("context messages = %#v", context.Messages)
		}
	})

	t.Run(name+"/reconstructs compaction summaries", func(t *testing.T) {
		session := NewSession(createStorage(t))
		_, _ = session.AppendMessage(harnessUserMessage("one"))
		_, _ = session.AppendMessage(harnessAssistantMessage("two"))
		user2, _ := session.AppendMessage(harnessUserMessage("three"))
		_, _ = session.AppendMessage(harnessAssistantMessage("four"))
		_, _ = session.AppendCompaction("summary", user2, 1234)
		_, _ = session.AppendMessage(harnessUserMessage("five"))
		context, _ := session.BuildContext()
		if len(context.Messages) != 4 || context.Messages[0].Role != "compactionSummary" {
			t.Fatalf("context = %#v", context.Messages)
		}
	})

	t.Run(name+"/branch summary and custom messages", func(t *testing.T) {
		session := NewSession(createStorage(t))
		user1, _ := session.AppendMessage(harnessUserMessage("one"))
		summaryID, err := session.MoveTo(&user1, "summary text")
		if err != nil || summaryID == nil {
			t.Fatalf("MoveTo summary = %v %v", summaryID, err)
		}
		entry, ok := session.Entry(*summaryID)
		if !ok || entry.Type != "branch_summary" || entry.FromID != user1 {
			t.Fatalf("summary entry = %#v", entry)
		}
		_, _ = session.AppendCustomMessageEntry("custom", "hello", true, map[string]bool{"ok": true})
		context, _ := session.BuildContext()
		if got := messageRoles(context.Messages); !reflect.DeepEqual(got, []string{"user", "branchSummary", "custom"}) {
			t.Fatalf("roles = %#v", got)
		}
	})

	t.Run(name+"/labels and session info", func(t *testing.T) {
		session := NewSession(createStorage(t))
		user1, _ := session.AppendMessage(harnessUserMessage("one"))
		_, _ = session.AppendLabel(user1, "checkpoint")
		_, _ = session.AppendSessionName("name")
		if label, ok := session.Label(user1); !ok || label != "checkpoint" {
			t.Fatalf("label = %q %v", label, ok)
		}
		if name, ok := session.SessionName(); !ok || name != "name" {
			t.Fatalf("session name = %q %v", name, ok)
		}
		context, _ := session.BuildContext()
		if len(context.Messages) != 1 {
			t.Fatalf("context = %#v", context.Messages)
		}
		if _, err := session.AppendLabel("missing", "checkpoint"); err == nil || !strings.Contains(err.Error(), "Entry missing not found") {
			t.Fatalf("AppendLabel missing err = %v", err)
		}
	})

	t.Run(name+"/persists leaf changes and appended entries", func(t *testing.T) {
		storage := createStorage(t)
		session := NewSession(storage)
		user1, _ := session.AppendMessage(harnessUserMessage("one"))
		_, _ = session.AppendMessage(harnessAssistantMessage("two"))
		_, _ = session.AppendLabel(user1, "checkpoint")
		_, _ = session.AppendSessionName("name")
		_, _ = session.MoveTo(&user1, "")
		_, _ = session.AppendMessage(harnessAssistantMessage("branched"))
		session2 := NewSession(storage)
		context, _ := session2.BuildContext()
		if got := messageRoles(context.Messages); !reflect.DeepEqual(got, []string{"user", "assistant"}) {
			t.Fatalf("roles = %#v", got)
		}
		if label, ok := session2.Label(user1); !ok || label != "checkpoint" {
			t.Fatalf("label = %q %v", label, ok)
		}
		if inspect != nil {
			inspect(t)
		}
	})
}

func TestSessionSuites(t *testing.T) {
	runSessionSuite(t, "memory", func(t *testing.T) SessionStorage {
		t.Helper()
		return MustInMemorySessionStorage()
	}, nil)

	var latestDir string
	runSessionSuite(t, "jsonl", func(t *testing.T) SessionStorage {
		t.Helper()
		latestDir = t.TempDir()
		storage, err := CreateJsonlSessionStorage(filepath.Join(latestDir, "session.jsonl"), SessionMetadata{ID: "session-1", CreatedAt: nowISO(), CWD: latestDir})
		if err != nil {
			t.Fatal(err)
		}
		return storage
	}, func(t *testing.T) {
		t.Helper()
		content, err := os.ReadFile(filepath.Join(latestDir, "session.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(content)), "\n")
		if len(lines) <= 1 || !strings.Contains(lines[0], `"version":3`) || !strings.Contains(string(content), `"type":"leaf"`) {
			t.Fatalf("jsonl content = %s", string(content))
		}
	})
}

func messageRoles(messages []llm.Message) []string {
	roles := make([]string, len(messages))
	for i, message := range messages {
		roles[i] = message.Role
	}
	return roles
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
