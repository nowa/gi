package gicodingagent

import (
	"testing"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionAutoCompactionResumesAgentQueuedMessages(t *testing.T) {
	session, calls := createAutoCompactionTestSession(t)
	defer session.Dispose()
	continueCalls := 0
	session.AgentContinue = func() error {
		continueCalls++
		return nil
	}
	session.QueueAgentMessage(llm.Message{Role: "custom", Content: []llm.ContentPart{llm.Text("Queued custom")}})

	if session.PendingMessageCount() != 0 || !session.HasAgentQueuedMessages() {
		t.Fatalf("queue state pending=%d agentQueued=%v", session.PendingMessageCount(), session.HasAgentQueuedMessages())
	}
	if err := session.RunAutoCompaction("threshold", false); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 || continueCalls != 1 {
		t.Fatalf("compaction calls=%d continue calls=%d, want 1/1", *calls, continueCalls)
	}
}

func TestAgentSessionAutoCompactionDoesNotRepeatOverflowRecovery(t *testing.T) {
	session, calls := createAutoCompactionTestSession(t)
	defer session.Dispose()
	var events []AgentSessionEvent
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "compaction_end" {
			events = append(events, event)
		}
	})
	overflow := autoCompactionAssistantError("prompt is too long", 1)

	if err := session.CheckCompaction(overflow); err != nil {
		t.Fatal(err)
	}
	if err := session.CheckCompaction(autoCompactionAssistantError("prompt is too long", 2)); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Fatalf("auto compaction calls = %d, want 1", *calls)
	}
	if len(events) != 2 ||
		events[0].Reason != "overflow" ||
		!events[0].WillRetry ||
		events[1].Reason != "overflow" ||
		events[1].ErrorMessage == "" {
		t.Fatalf("events = %#v, want overflow failure", events)
	}
}

func TestAgentSessionAutoCompactionIgnoresStalePreCompactionUsage(t *testing.T) {
	session, calls := createAutoCompactionTestSession(t)
	defer session.Dispose()
	stale := autoCompactionAssistant("large response before compaction", 610_000, 1)
	session.SessionManager.AppendMessage(statsUserMessage("before compaction", 0))
	session.SessionManager.AppendMessage(stale)
	firstKept := session.SessionManager.GetEntries()[0].ID
	session.SessionManager.AppendCompaction("summary", firstKept, 610_000)
	session.SessionManager.AppendMessage(statsUserMessage("session recovery payload", 2))

	if err := session.CheckCompaction(stale, true); err != nil {
		t.Fatal(err)
	}
	if *calls != 0 {
		t.Fatalf("auto compaction calls = %d, want 0", *calls)
	}
}

func TestAgentSessionAutoCompactionUsesLastSuccessfulUsageForError(t *testing.T) {
	session, calls := createAutoCompactionTestSession(t)
	defer session.Dispose()
	successful := autoCompactionAssistant("large successful response", 190_000, 1)
	session.SessionManager.AppendMessage(statsUserMessage("hello", 0))
	session.SessionManager.AppendMessage(successful)

	if err := session.CheckCompaction(autoCompactionAssistantError("529 overloaded", 2)); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Fatalf("auto compaction calls = %d, want 1", *calls)
	}
}

func TestAgentSessionAutoCompactionSkipsErrorWithoutPriorUsage(t *testing.T) {
	session, calls := createAutoCompactionTestSession(t)
	defer session.Dispose()
	session.SessionManager.AppendMessage(statsUserMessage("hello", 0))

	if err := session.CheckCompaction(autoCompactionAssistantError("529 overloaded", 1)); err != nil {
		t.Fatal(err)
	}
	if *calls != 0 {
		t.Fatalf("auto compaction calls = %d, want 0", *calls)
	}
}

func TestAgentSessionAutoCompactionSkipsKeptPreCompactionUsage(t *testing.T) {
	session, calls := createAutoCompactionTestSession(t)
	defer session.Dispose()
	kept := autoCompactionAssistant("kept response from before compaction", 190_000, 1)
	session.SessionManager.AppendMessage(statsUserMessage("before compaction", 0))
	session.SessionManager.AppendMessage(kept)
	firstKept := session.SessionManager.GetEntries()[0].ID
	session.SessionManager.AppendCompaction("summary", firstKept, 190_000)
	session.SessionManager.AppendMessage(statsUserMessage("new prompt", 2))

	if err := session.CheckCompaction(autoCompactionAssistantError("529 overloaded", 3)); err != nil {
		t.Fatal(err)
	}
	if *calls != 0 {
		t.Fatalf("auto compaction calls = %d, want 0", *calls)
	}
}

func createAutoCompactionTestSession(t *testing.T) (*AgentSession, *int) {
	t.Helper()
	calls := 0
	cwd := t.TempDir()
	manager, err := InMemorySessionManager(cwd)
	if err != nil {
		t.Fatal(err)
	}
	settings := agentharness.CompactionSettings{Enabled: true, ReserveTokens: 20_000, KeepRecentTokens: 1}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:                cwd,
		AgentDir:           t.TempDir(),
		Model:              autoCompactionTestModel(),
		SessionManager:     manager,
		CompactionSettings: &settings,
		AutoCompactionRunner: func(string, bool) error {
			calls++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return session, &calls
}

func autoCompactionAssistant(text string, totalTokens int, timestamp int64) llm.Message {
	message := statsAssistantMessage(text, totalTokens, timestamp, autoCompactionTestModel())
	message.Usage.Output = 0
	message.Usage.TotalTokens = totalTokens
	return message
}

func autoCompactionTestModel() llm.Model {
	model := llm.MustGetModel("anthropic", "claude-sonnet-4-5")
	model.ContextWindow = 200_000
	return model
}

func autoCompactionAssistantError(message string, timestamp int64) llm.Message {
	return llm.Message{
		Role:         llm.RoleAssistant,
		StopReason:   llm.StopReasonError,
		ErrorMessage: message,
		Timestamp:    timestamp,
	}
}
