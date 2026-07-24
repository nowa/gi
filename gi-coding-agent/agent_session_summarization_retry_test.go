package gicodingagent

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type scriptedAgentSessionSummaryRuntime struct {
	mu        sync.Mutex
	responses []llm.Message
	calls     int
}

func (r *scriptedAgentSessionSummaryRuntime) CompleteSimple(
	_ context.Context,
	_ llm.Model,
	_ llm.Context,
	_ llm.ModelsStreamOptions,
) (llm.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	index := r.calls
	r.calls++
	if index >= len(r.responses) {
		index = len(r.responses) - 1
	}
	return r.responses[index], nil
}

func (r *scriptedAgentSessionSummaryRuntime) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestAgentSessionCompactionRetriesTransientSummaryAndPersistsUsagePiStyle(
	t *testing.T,
) {
	session, manager := createCompactionTestSession(t, false)
	defer session.Dispose()
	mustPrompt(t, session, "one")
	mustPrompt(t, session, "two")

	model := session.Agent.State.Model
	failure := llm.AssistantErrorMessage("terminated", model, false)
	success := llm.AssistantMessage(
		[]llm.ContentPart{llm.Text("recovered summary")},
		llm.StopReasonStop,
		model,
	)
	success.Usage = llm.Usage{
		Input:       12,
		Output:      3,
		TotalTokens: 15,
	}
	runtime := &scriptedAgentSessionSummaryRuntime{
		responses: []llm.Message{failure, success},
	}
	session.SummaryRuntime = runtime
	session.RetrySettings = AgentSessionRetrySettings{
		Enabled:     true,
		MaxRetries:  1,
		BaseDelayMs: 0,
	}
	var events []AgentSessionEvent
	session.Subscribe(func(event AgentSessionEvent) {
		events = append(events, event)
	})

	result, err := session.Compact()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.callCount() < 2 {
		t.Fatalf("summary calls = %d, want retry", runtime.callCount())
	}
	if result.Usage == nil || result.Usage.TotalTokens != 30 {
		t.Fatalf("result usage = %#v", result.Usage)
	}
	var retryEvents []string
	for _, event := range events {
		switch event.Type {
		case "summarization_retry_scheduled":
			retryEvents = append(retryEvents, event.Type)
			if event.Attempt != 1 ||
				event.MaxAttempts != 1 ||
				event.ErrorMessage != "terminated" {
				t.Fatalf("scheduled event = %#v", event)
			}
		case "summarization_retry_attempt_start":
			retryEvents = append(retryEvents, event.Type)
			if event.Source != "compaction" ||
				event.Reason != "manual" {
				t.Fatalf("attempt event = %#v", event)
			}
		case "summarization_retry_finished":
			retryEvents = append(retryEvents, event.Type)
		}
	}
	if !reflect.DeepEqual(retryEvents, []string{
		"summarization_retry_scheduled",
		"summarization_retry_attempt_start",
		"summarization_retry_finished",
	}) {
		t.Fatalf("retry events = %#v", retryEvents)
	}
	compactions := filterFileEntriesByType(
		manager.GetEntries(),
		"compaction",
	)
	if len(compactions) != 1 ||
		compactions[0].Usage == nil ||
		compactions[0].Usage.TotalTokens != 30 {
		t.Fatalf("persisted compaction = %#v", compactions)
	}
	persisted := filterFileEntriesByType(
		LoadEntriesFromFile(manager.GetSessionFile()),
		"compaction",
	)
	if len(persisted) != 1 ||
		persisted[0].Usage == nil ||
		persisted[0].Usage.TotalTokens != 30 {
		t.Fatalf("reloaded compaction = %#v", persisted)
	}
}

func TestAgentSessionAbortCompactionCancelsSummaryRetryBackoffPiStyle(
	t *testing.T,
) {
	session, _ := createCompactionTestSession(t, true)
	defer session.Dispose()
	mustPrompt(t, session, "one")
	mustPrompt(t, session, "two")

	model := session.Agent.State.Model
	runtime := &scriptedAgentSessionSummaryRuntime{
		responses: []llm.Message{
			llm.AssistantErrorMessage("terminated", model, false),
		},
	}
	session.SummaryRuntime = runtime
	session.RetrySettings = AgentSessionRetrySettings{
		Enabled:     true,
		MaxRetries:  5,
		BaseDelayMs: 30_000,
	}
	scheduled := make(chan struct{})
	var once sync.Once
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "summarization_retry_scheduled" {
			once.Do(func() { close(scheduled) })
		}
	})
	errCh := make(chan error, 1)
	go func() {
		_, err := session.Compact()
		errCh <- err
	}()
	select {
	case <-scheduled:
	case <-time.After(time.Second):
		t.Fatal("summary retry was not scheduled")
	}
	session.AbortCompaction()
	select {
	case err := <-errCh:
		if !isCompactionCancelledError(err) {
			t.Fatalf("compact error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("compaction retry did not stop after abort")
	}
	if runtime.callCount() != 1 {
		t.Fatalf("summary calls = %d, want 1", runtime.callCount())
	}
}

func TestAgentSessionBranchSummaryUsesSharedRetryAndPersistsUsagePiStyle(
	t *testing.T,
) {
	session, manager := createTreeNavigationTestSession(t, nil)
	defer session.Dispose()
	mustPrompt(t, session, "first")
	mustPrompt(t, session, "abandoned")
	target := manager.GetTree()[0].Entry

	model := session.Agent.State.Model
	failure := llm.AssistantErrorMessage("terminated", model, false)
	success := llm.AssistantMessage(
		[]llm.ContentPart{llm.Text("recovered branch")},
		llm.StopReasonStop,
		model,
	)
	success.Usage = llm.Usage{Input: 8, Output: 2, TotalTokens: 10}
	runtime := &scriptedAgentSessionSummaryRuntime{
		responses: []llm.Message{failure, success},
	}
	session.SummaryRuntime = runtime
	session.RetrySettings = AgentSessionRetrySettings{
		Enabled:     true,
		MaxRetries:  1,
		BaseDelayMs: 0,
	}
	var events []AgentSessionEvent
	session.Subscribe(func(event AgentSessionEvent) {
		events = append(events, event)
	})

	result, err := session.NavigateTree(
		target.ID,
		AgentSessionNavigateTreeOptions{Summarize: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.SummaryEntry == nil ||
		result.SummaryEntry.Usage == nil ||
		result.SummaryEntry.Usage.TotalTokens != 10 {
		t.Fatalf("summary entry = %#v", result.SummaryEntry)
	}
	if runtime.callCount() != 2 {
		t.Fatalf("summary calls = %d, want 2", runtime.callCount())
	}
	var retryEvents []string
	for _, event := range events {
		switch event.Type {
		case "summarization_retry_scheduled":
			retryEvents = append(retryEvents, event.Type)
		case "summarization_retry_attempt_start":
			retryEvents = append(retryEvents, event.Type)
			if event.Source != "branchSummary" || event.Reason != "" {
				t.Fatalf("attempt event = %#v", event)
			}
		case "summarization_retry_finished":
			retryEvents = append(retryEvents, event.Type)
		}
	}
	if !reflect.DeepEqual(retryEvents, []string{
		"summarization_retry_scheduled",
		"summarization_retry_attempt_start",
		"summarization_retry_finished",
	}) {
		t.Fatalf("retry events = %#v", retryEvents)
	}
}
