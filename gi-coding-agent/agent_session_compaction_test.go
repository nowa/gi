package gicodingagent

import (
	"strings"
	"testing"
	"time"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionCompactManual(t *testing.T) {
	session, _ := createCompactionTestSession(t, false)
	defer session.Dispose()

	mustPrompt(t, session, "What is 2+2? Reply with just the number.")
	mustPrompt(t, session, "What is 3+3? Reply with just the number.")

	result, err := session.Compact()
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary == "" {
		t.Fatal("summary should be non-empty")
	}
	if result.TokensBefore <= 0 {
		t.Fatalf("tokens before = %d, want > 0", result.TokensBefore)
	}
	messages := session.Messages()
	if len(messages) == 0 {
		t.Fatal("messages should remain after compaction")
	}
	if messages[0].Role != "compactionSummary" {
		t.Fatalf("first role = %q, want compactionSummary", messages[0].Role)
	}
}

func TestAgentSessionMaintainsStateAfterCompaction(t *testing.T) {
	session, _ := createCompactionTestSession(t, false)
	defer session.Dispose()

	mustPrompt(t, session, "What is the capital of France? One word answer.")
	mustPrompt(t, session, "What is the capital of Germany? One word answer.")
	if _, err := session.Compact(); err != nil {
		t.Fatal(err)
	}

	mustPrompt(t, session, "What is the capital of Italy? One word answer.")
	messages := session.Messages()
	if len(messages) == 0 {
		t.Fatal("messages should remain after follow-up prompt")
	}
	if countMessagesByRole(messages, llm.RoleAssistant) == 0 {
		t.Fatal("assistant messages should remain after follow-up prompt")
	}
}

func TestAgentSessionPersistsCompactionToSessionFile(t *testing.T) {
	session, sessionManager := createCompactionTestSession(t, false)
	defer session.Dispose()

	mustPrompt(t, session, "Say hello")
	mustPrompt(t, session, "Say goodbye")
	result, err := session.Compact()
	if err != nil {
		t.Fatal(err)
	}

	entries := sessionManager.GetEntries()
	compactions := filterFileEntriesByType(entries, "compaction")
	if len(compactions) != 1 {
		t.Fatalf("compaction entries = %d, want 1", len(compactions))
	}
	compaction := compactions[0]
	if compaction.Summary == "" || compaction.Summary != result.Summary {
		t.Fatalf("compaction summary = %q, want %q", compaction.Summary, result.Summary)
	}
	if compaction.FirstKeptID == "" {
		t.Fatal("compaction first kept entry id should be set")
	}
	if compaction.TokensBefore <= 0 {
		t.Fatalf("tokens before = %d, want > 0", compaction.TokensBefore)
	}

	persisted := LoadEntriesFromFile(sessionManager.GetSessionFile())
	if len(filterFileEntriesByType(persisted, "compaction")) != 1 {
		t.Fatalf("persisted entries should include one compaction, got %#v", persisted)
	}
}

func TestAgentSessionCompactInMemoryNoSession(t *testing.T) {
	session, sessionManager := createCompactionTestSession(t, true)
	defer session.Dispose()

	mustPrompt(t, session, "What is 2+2? Reply with just the number.")
	mustPrompt(t, session, "What is 3+3? Reply with just the number.")
	result, err := session.Compact()
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary == "" {
		t.Fatal("summary should be non-empty")
	}
	if sessionManager.GetSessionFile() != "" {
		t.Fatalf("session file = %q, want empty for in-memory session", sessionManager.GetSessionFile())
	}
	if compactions := filterFileEntriesByType(sessionManager.GetEntries(), "compaction"); len(compactions) != 1 {
		t.Fatalf("compaction entries = %d, want 1", len(compactions))
	}
}

func TestAgentSessionCompactEmitsEvents(t *testing.T) {
	session, _ := createCompactionTestSession(t, false)
	defer session.Dispose()
	var events []AgentSessionEvent
	session.Subscribe(func(event AgentSessionEvent) {
		events = append(events, event)
	})

	mustPrompt(t, session, "Say hello")
	if _, err := session.Compact(); err != nil {
		t.Fatal(err)
	}

	compactionEvents := filterSessionEvents(events, "compaction_start", "compaction_end")
	if len(compactionEvents) != 2 {
		t.Fatalf("compaction events = %d, want 2: %#v", len(compactionEvents), compactionEvents)
	}
	if compactionEvents[0].Type != "compaction_start" || compactionEvents[0].Reason != "manual" {
		t.Fatalf("start event = %#v, want manual compaction_start", compactionEvents[0])
	}
	end := compactionEvents[1]
	if end.Type != "compaction_end" || end.Reason != "manual" || end.Aborted || end.WillRetry || end.Result == nil {
		t.Fatalf("end event = %#v, want successful manual compaction_end", end)
	}
	if len(filterSessionEvents(events, "message_end")) == 0 {
		t.Fatal("message_end should be emitted for prompt response")
	}
}

func TestAgentSessionCompactAbortEmitsPiStyleAbortedEvent(t *testing.T) {
	session, _ := createCompactionTestSession(t, false)
	defer session.Dispose()
	mustPrompt(t, session, "Say hello")
	mustPrompt(t, session, "Say goodbye")

	compactionStarted := make(chan struct{})
	releaseCompaction := make(chan struct{})
	session.CompactionSummarizer = func(preparation agentharness.CompactionPreparation, _ string) (agentharness.CompactionResult, error) {
		close(compactionStarted)
		<-releaseCompaction
		return agentharness.CompactionResult{
			Summary:          "should be discarded",
			FirstKeptEntryID: preparation.FirstKeptEntryID,
			TokensBefore:     preparation.TokensBefore,
		}, nil
	}

	var events []AgentSessionEvent
	session.Subscribe(func(event AgentSessionEvent) {
		events = append(events, event)
	})
	errCh := make(chan error, 1)
	go func() {
		_, err := session.Compact()
		errCh <- err
	}()
	select {
	case <-compactionStarted:
	case <-time.After(time.Second):
		t.Fatal("compaction did not start")
	}
	session.AbortCompaction()
	close(releaseCompaction)
	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "Compaction cancelled") {
			t.Fatalf("compact error = %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("compaction did not finish")
	}

	compactionEvents := filterSessionEvents(events, "compaction_start", "compaction_end")
	if len(compactionEvents) != 2 {
		t.Fatalf("compaction events = %d, want 2: %#v", len(compactionEvents), compactionEvents)
	}
	end := compactionEvents[1]
	if end.Type != "compaction_end" || end.Reason != "manual" || !end.Aborted || end.Result != nil || end.ErrorMessage != "Compaction cancelled" {
		t.Fatalf("end event = %#v, want Pi-style cancelled compaction_end", end)
	}
}

func createCompactionTestSession(t *testing.T, noSession bool) (*AgentSession, *SessionManager) {
	t.Helper()
	cwd := t.TempDir()
	var (
		sessionManager *SessionManager
		err            error
	)
	if noSession {
		sessionManager, err = InMemorySessionManager(cwd)
	} else {
		sessionManager, err = CreateSessionManager(cwd, t.TempDir())
	}
	if err != nil {
		t.Fatal(err)
	}
	settings := agentharness.CompactionSettings{Enabled: true, ReserveTokens: 100, KeepRecentTokens: 1}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:                cwd,
		AgentDir:           t.TempDir(),
		Model:              llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
		SessionManager:     sessionManager,
		CompactionSettings: &settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	return session, sessionManager
}

func mustPrompt(t *testing.T, session *AgentSession, text string) {
	t.Helper()
	if err := session.Prompt(text); err != nil {
		t.Fatal(err)
	}
}

func countMessagesByRole(messages []llm.Message, role string) int {
	count := 0
	for _, message := range messages {
		if message.Role == role {
			count++
		}
	}
	return count
}

func filterFileEntriesByType(entries []FileEntry, entryType string) []FileEntry {
	var filtered []FileEntry
	for _, entry := range entries {
		if entry.Type == entryType {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func filterSessionEvents(events []AgentSessionEvent, eventTypes ...string) []AgentSessionEvent {
	allowed := map[string]bool{}
	for _, eventType := range eventTypes {
		allowed[eventType] = true
	}
	var filtered []AgentSessionEvent
	for _, event := range events {
		if allowed[event.Type] {
			filtered = append(filtered, event)
		}
	}
	return filtered
}
