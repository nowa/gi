package gicodingagent

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionCompactionSuitePiParityExtensionSummary(t *testing.T) {
	session := createCompactionExtensionSession(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.On("session_before_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			return ProtocolEventResult{Compaction: &agentharness.CompactionResult{
				Summary:          "summary from extension",
				FirstKeptEntryID: event.Preparation.FirstKeptEntryID,
				TokensBefore:     event.Preparation.TokensBefore,
				Details:          map[string]any{"source": "extension"},
			}}, nil
		})
	})
	defer session.Dispose()
	mustPrompt(t, session, "one")
	mustPrompt(t, session, "two")

	result, err := session.Compact()
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != "summary from extension" {
		t.Fatalf("summary = %q", result.Summary)
	}
	if entries := filterFileEntriesByType(session.SessionManager.GetEntries(), "compaction"); len(entries) != 1 {
		t.Fatalf("compaction entries = %#v", entries)
	}
	if messages := session.Messages(); len(messages) == 0 || messages[0].Role != "compactionSummary" {
		t.Fatalf("messages after compaction = %#v", messages)
	}
}

func TestAgentSessionCompactionSuitePiParityRequiresModel(t *testing.T) {
	session, _ := createCompactionTestSession(t, false)
	defer session.Dispose()
	mustPrompt(t, session, "one")
	session.Agent.State.Model = llm.Model{}

	_, err := session.Compact()
	if err == nil || !strings.Contains(err.Error(), "No model selected") {
		t.Fatalf("compact error = %v", err)
	}
}

func TestAgentSessionCompactionSuitePiParityRequiresAuth(t *testing.T) {
	session, _ := createCompactionTestSession(t, false)
	defer session.Dispose()
	mustPrompt(t, session, "one")
	session.Preflight = func(model llm.Model) error {
		return errors.New("No API key found for " + model.Provider + ".")
	}

	_, err := session.Compact()
	if err == nil || !strings.Contains(err.Error(), "No API key found for "+session.Agent.State.Model.Provider+".") {
		t.Fatalf("compact error = %v", err)
	}
}

func TestAgentSessionCompactionSuitePiParityAbortManualCompaction(t *testing.T) {
	started := make(chan struct{})
	session := createCompactionExtensionSession(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.On("session_before_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			close(started)
			<-event.Context.Done()
			return ProtocolEventResult{Cancel: true}, nil
		})
	})
	defer session.Dispose()
	mustPrompt(t, session, "one")
	mustPrompt(t, session, "two")

	errCh := make(chan error, 1)
	go func() {
		_, err := session.Compact()
		errCh <- err
	}()
	<-started
	session.AbortCompaction()
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "Compaction cancelled") {
		t.Fatalf("compact error = %v", err)
	}
}

func TestAgentSessionCompactionSuitePiParityResumesAgentQueuedMessages(t *testing.T) {
	session, _ := createCompactionTestSession(t, false)
	defer session.Dispose()
	mustPrompt(t, session, "first")
	mustPrompt(t, session, "second")
	session.QueueAgentMessage(llm.Message{Role: "custom", Content: []llm.ContentPart{llm.Text("queued custom")}})
	continueCalls := 0
	session.AgentContinue = func() error {
		continueCalls++
		return nil
	}

	if err := session.RunAutoCompaction("threshold", false); err != nil {
		t.Fatal(err)
	}
	if continueCalls != 1 {
		t.Fatalf("continue calls = %d, want 1", continueCalls)
	}
}

func TestAgentSessionCompactionSuitePiParityDoesNotRepeatOverflowRecovery(t *testing.T) {
	session, calls := createAutoCompactionTestSession(t)
	defer session.Dispose()
	var compactionErrors []string
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "compaction_end" && event.ErrorMessage != "" {
			compactionErrors = append(compactionErrors, event.ErrorMessage)
		}
	})

	if err := session.CheckCompaction(autoCompactionAssistantError("prompt is too long", 1)); err != nil {
		t.Fatal(err)
	}
	if err := session.CheckCompaction(autoCompactionAssistantError("prompt is too long", 2)); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Fatalf("auto compaction calls = %d, want 1", *calls)
	}
	if !containsString(compactionErrors, "Context overflow recovery failed after one compact-and-retry attempt. Try reducing context or switching to a larger-context model.") {
		t.Fatalf("compaction errors = %#v", compactionErrors)
	}
}

func TestAgentSessionCompactionSuitePiParityIgnoresStalePreCompactionUsage(t *testing.T) {
	session, calls := createAutoCompactionTestSession(t)
	defer session.Dispose()
	stale := autoCompactionAssistant("large response before compaction", 610_000, 1)
	session.SessionManager.AppendMessage(statsUserMessage("before compaction", 0))
	session.SessionManager.AppendMessage(sessionMessageValue(stale))
	firstKept := session.SessionManager.GetEntries()[0].ID
	session.SessionManager.AppendCompaction("summary", firstKept, 610_000)
	session.SessionManager.AppendMessage(statsUserMessage("after compaction", 2))

	if err := session.CheckCompaction(stale, true); err != nil {
		t.Fatal(err)
	}
	if *calls != 0 {
		t.Fatalf("auto compaction calls = %d, want 0", *calls)
	}
}

func TestAgentSessionCompactionSuitePiParityThresholdForErrorUsesLastSuccessfulUsage(t *testing.T) {
	session, calls := createAutoCompactionTestSession(t)
	defer session.Dispose()
	successful := autoCompactionAssistant("large successful response", 190_000, 1)
	session.SessionManager.AppendMessage(statsUserMessage("hello", 0))
	session.SessionManager.AppendMessage(sessionMessageValue(successful))

	if err := session.CheckCompaction(autoCompactionAssistantError("529 overloaded", 2)); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Fatalf("auto compaction calls = %d, want 1", *calls)
	}
}

func TestAgentSessionCompactionSuitePiParitySkipsErrorWithoutPriorUsage(t *testing.T) {
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

func TestAgentSessionCompactionSuitePiParitySkipsKeptPreCompactionUsage(t *testing.T) {
	session, calls := createAutoCompactionTestSession(t)
	defer session.Dispose()
	kept := autoCompactionAssistant("kept response from before compaction", 190_000, 1)
	session.SessionManager.AppendMessage(statsUserMessage("before compaction", 0))
	session.SessionManager.AppendMessage(sessionMessageValue(kept))
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

func TestAgentSessionCompactionSuitePiParityThresholdDisabledOrBelowThreshold(t *testing.T) {
	belowSession, belowCalls := createAutoCompactionTestSession(t)
	defer belowSession.Dispose()
	if err := belowSession.CheckCompaction(autoCompactionAssistant("small", 1_000, 1)); err != nil {
		t.Fatal(err)
	}

	disabledSession, disabledCalls := createAutoCompactionTestSession(t)
	defer disabledSession.Dispose()
	disabledSession.CompactionSettings.Enabled = false
	if err := disabledSession.CheckCompaction(autoCompactionAssistant("large", 1_000_000, 1)); err != nil {
		t.Fatal(err)
	}

	if got := []int{*belowCalls, *disabledCalls}; !reflect.DeepEqual(got, []int{0, 0}) {
		t.Fatalf("auto compaction calls = %#v, want [0 0]", got)
	}
}
