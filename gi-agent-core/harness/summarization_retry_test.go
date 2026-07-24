package harness

import (
	"context"
	"math"
	"reflect"
	"strconv"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type scriptedSummaryRuntime struct {
	responses []llm.Message
	calls     int
	options   []llm.ModelsStreamOptions
}

func (r *scriptedSummaryRuntime) CompleteSimple(
	_ context.Context,
	_ llm.Model,
	_ llm.Context,
	options llm.ModelsStreamOptions,
) (llm.Message, error) {
	r.options = append(r.options, options)
	index := r.calls
	r.calls++
	if index >= len(r.responses) {
		index = len(r.responses) - 1
	}
	return r.responses[index], nil
}

func TestCompleteSimpleWithRetriesUsesOneIsolatedRequestSnapshotPiStyle(
	t *testing.T,
) {
	model := llm.Model{
		ID:       "summary",
		Provider: "test",
		API:      "test",
	}
	failure := llm.AssistantErrorMessage("terminated", model, false)
	success := llm.AssistantMessage(
		[]llm.ContentPart{llm.Text("recovered")},
		llm.StopReasonStop,
		model,
	)
	runtime := &scriptedSummaryRuntime{
		responses: []llm.Message{failure, failure, success},
	}
	var events []string
	result, err := CompleteSimpleWithRetries(
		context.Background(),
		runtime,
		model,
		llm.Context{Messages: []llm.Message{llm.UserMessageText("summarize")}},
		llm.ModelsStreamOptions{},
		llm.RetryPolicy{Enabled: true, MaxRetries: 3},
		llm.RetryCallbacks{
			OnRetryScheduled: func(attempt llm.RetryAttempt) {
				events = append(
					events,
					"scheduled:"+strconv.Itoa(attempt.Attempt),
				)
			},
			OnRetryAttemptStart: func(attempt int) {
				events = append(
					events,
					"start:"+strconv.Itoa(attempt),
				)
			},
			OnRetryFinished: func(result llm.RetryResult) {
				if result.Success {
					events = append(events, "finished:success")
				}
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != llm.StopReasonStop || runtime.calls != 3 {
		t.Fatalf("result = %#v, calls = %d", result, runtime.calls)
	}
	if !reflect.DeepEqual(events, []string{
		"scheduled:1",
		"start:1",
		"scheduled:2",
		"start:2",
		"finished:success",
	}) {
		t.Fatalf("events = %#v", events)
	}
	var sessionID string
	for index, options := range runtime.options {
		if options.CacheRetention != "none" {
			t.Fatalf(
				"attempt %d cache retention = %q",
				index,
				options.CacheRetention,
			)
		}
		if options.SessionID == "" {
			t.Fatalf("attempt %d has no isolated session ID", index)
		}
		if sessionID == "" {
			sessionID = options.SessionID
		} else if options.SessionID != sessionID {
			t.Fatalf(
				"attempt %d session ID = %q, want %q",
				index,
				options.SessionID,
				sessionID,
			)
		}
	}
}

func TestCompactWithOptionsRetriesAndCombinesSummaryUsagePiStyle(
	t *testing.T,
) {
	model := llm.Model{
		ID:        "summary",
		Provider:  "test",
		API:       "test",
		MaxTokens: 100_000,
	}
	failure := llm.AssistantErrorMessage("terminated", model, false)
	history := llm.AssistantMessage(
		[]llm.ContentPart{llm.Text("history summary")},
		llm.StopReasonStop,
		model,
	)
	history.Usage = llm.Usage{
		Input:       10,
		Output:      2,
		TotalTokens: 12,
		Cost:        llm.UsageCost{Total: 0.2},
	}
	prefix := llm.AssistantMessage(
		[]llm.ContentPart{llm.Text("prefix summary")},
		llm.StopReasonStop,
		model,
	)
	prefix.Usage = llm.Usage{
		Input:       4,
		Output:      1,
		TotalTokens: 5,
		Cost:        llm.UsageCost{Total: 0.1},
	}
	runtime := &scriptedSummaryRuntime{
		responses: []llm.Message{failure, history, prefix},
	}
	var scheduled int
	result, err := CompactWithOptions(
		context.Background(),
		CompactionPreparation{
			FirstKeptEntryID: "kept",
			MessagesToSummarize: []llm.Message{
				llm.UserMessageText("history"),
			},
			TurnPrefixMessages: []llm.Message{
				llm.UserMessageText("prefix"),
			},
			IsSplitTurn:  true,
			TokensBefore: 100,
			FileOps: FileOps{
				Read:    map[string]bool{},
				Written: map[string]bool{},
				Edited:  map[string]bool{},
			},
			Settings: CompactionSettings{ReserveTokens: 1_000},
		},
		model,
		CompactOptions{
			Runtime: runtime,
			Retry: llm.RetryPolicy{
				Enabled:    true,
				MaxRetries: 1,
			},
			RetryCallbacks: llm.RetryCallbacks{
				OnRetryScheduled: func(llm.RetryAttempt) {
					scheduled++
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.calls != 3 || scheduled != 1 {
		t.Fatalf(
			"calls = %d, scheduled = %d",
			runtime.calls,
			scheduled,
		)
	}
	if result.Usage == nil ||
		result.Usage.Input != 14 ||
		result.Usage.Output != 3 ||
		result.Usage.TotalTokens != 17 ||
		math.Abs(result.Usage.Cost.Total-0.3) > 1e-9 {
		t.Fatalf("combined usage = %#v", result.Usage)
	}
}

func TestAgentHarnessSummaryRuntimeOwnsAuthRetryEventsAndUsagePiStyle(
	t *testing.T,
) {
	model := llm.Model{
		ID:            "summary",
		Provider:      "test",
		API:           "test",
		ContextWindow: 200_000,
		MaxTokens:     100_000,
	}
	failure := llm.AssistantErrorMessage("terminated", model, false)
	success := llm.AssistantMessage(
		[]llm.ContentPart{llm.Text("recovered summary")},
		llm.StopReasonStop,
		model,
	)
	success.Usage = llm.Usage{Input: 7, Output: 2, TotalTokens: 9}
	runtime := &scriptedSummaryRuntime{
		responses: []llm.Message{failure, success},
	}
	session := NewSession(MustInMemorySessionStorage())
	_, _ = session.AppendMessage(llm.UserMessageText("one"))
	first := llm.AssistantMessage(
		[]llm.ContentPart{llm.Text("two")},
		llm.StopReasonStop,
		model,
	)
	first.Usage = llm.Usage{Input: 50_000, TotalTokens: 50_000}
	_, _ = session.AppendMessage(first)
	_, _ = session.AppendMessage(llm.UserMessageText("three"))
	second := llm.AssistantMessage(
		[]llm.ContentPart{llm.Text("four")},
		llm.StopReasonStop,
		model,
	)
	second.Usage = llm.Usage{Input: 50_000, TotalTokens: 50_000}
	_, _ = session.AppendMessage(second)
	harness := MustNewAgentHarness(AgentHarnessOptions{
		Session: session,
		Models:  runtime,
		Model:   model,
		Retry: llm.RetryPolicy{
			Enabled:    true,
			MaxRetries: 1,
		},
	})
	var events []string
	harness.Subscribe(func(_ context.Context, event AgentHarnessEvent) error {
		switch event.Type {
		case "retry_scheduled",
			"retry_attempt_start",
			"retry_finished":
			events = append(events, event.Type+":"+event.Operation)
		}
		return nil
	})

	result, err := harness.Compact(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.calls < 2 {
		t.Fatalf("summary calls = %d, want retry", runtime.calls)
	}
	if !reflect.DeepEqual(events, []string{
		"retry_scheduled:compaction",
		"retry_attempt_start:compaction",
		"retry_finished:compaction",
	}) {
		t.Fatalf("retry events = %#v", events)
	}
	if result.Usage == nil || result.Usage.TotalTokens == 0 {
		t.Fatalf("result usage = %#v", result.Usage)
	}
	entries := session.Entries()
	last := entries[len(entries)-1]
	if last.Type != "compaction" ||
		last.Usage == nil ||
		last.Usage.TotalTokens != result.Usage.TotalTokens {
		t.Fatalf("persisted compaction = %#v", last)
	}
}
