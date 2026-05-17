package giagentcore

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentCreatesDefaultState(t *testing.T) {
	agent := New()
	state := agent.State()
	if state.SystemPrompt != "" || state.Model.ID != "unknown" || state.ThinkingLevel != "off" {
		t.Fatalf("unexpected default state: %#v", state)
	}
	if len(state.Tools) != 0 || len(state.Messages) != 0 || state.IsStreaming || state.StreamingMessage != nil || len(state.PendingToolCalls) != 0 || state.ErrorMessage != "" {
		t.Fatalf("unexpected runtime state: %#v", state)
	}
}

func TestAgentCreatesCustomInitialState(t *testing.T) {
	model := llm.MustGetModel("openai", "gpt-4o-mini")
	agent := New(WithInitialState(AgentState{SystemPrompt: "You are helpful.", Model: model, ThinkingLevel: "low"}))
	state := agent.State()
	if state.SystemPrompt != "You are helpful." || state.Model.ID != model.ID || state.ThinkingLevel != "low" {
		t.Fatalf("state = %#v", state)
	}
}

func TestAgentSubscribeAndStateMutators(t *testing.T) {
	agent := New()
	eventCount := 0
	unsubscribe := agent.Subscribe(func(AgentEvent, context.Context) error {
		eventCount++
		return nil
	})
	agent.SetSystemPrompt("Test prompt")
	if eventCount != 0 {
		t.Fatalf("state mutator emitted %d events, want 0", eventCount)
	}
	unsubscribe()
	agent.SetSystemPrompt("Another prompt")
	if got := agent.State().SystemPrompt; got != "Another prompt" {
		t.Fatalf("SystemPrompt = %q", got)
	}
}

func TestAgentEmitsLifecycleForThrownRunFailures(t *testing.T) {
	agent := New(WithStreamFn(func(llm.Model, llm.Context, llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
		return nil, errors.New("provider exploded")
	}))
	var events []string
	agent.Subscribe(func(event AgentEvent, _ context.Context) error {
		events = append(events, event.Type)
		return nil
	})

	if err := agent.PromptText(context.Background(), "hello"); err != nil {
		t.Fatalf("PromptText() error = %v", err)
	}

	want := []string{"agent_start", "turn_start", "message_start", "message_end", "message_start", "message_end", "turn_end", "agent_end"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
	state := agent.State()
	last := state.Messages[len(state.Messages)-1]
	if last.Role != llm.RoleAssistant || last.StopReason != llm.StopReasonError || last.ErrorMessage != "provider exploded" || state.ErrorMessage != "provider exploded" {
		t.Fatalf("last/state = %#v / %#v", last, state)
	}
}

func TestAgentAwaitsSubscribersBeforePromptResolves(t *testing.T) {
	barrier := make(chan struct{})
	agent := New(WithStreamFn(testStream(testAssistantMessage([]llm.ContentPart{llm.Text("ok")}, llm.StopReasonStop))))
	listenerFinished := false
	agent.Subscribe(func(event AgentEvent, _ context.Context) error {
		if event.Type == "agent_end" {
			<-barrier
			listenerFinished = true
		}
		return nil
	})

	promptDone := make(chan error, 1)
	go func() { promptDone <- agent.PromptText(context.Background(), "hello") }()
	time.Sleep(10 * time.Millisecond)
	select {
	case err := <-promptDone:
		t.Fatalf("prompt resolved before listener barrier: %v", err)
	default:
	}
	if listenerFinished {
		t.Fatal("listener finished before barrier")
	}
	if !agent.State().IsStreaming {
		t.Fatal("agent should remain streaming while listener is blocked")
	}
	close(barrier)
	if err := <-promptDone; err != nil {
		t.Fatalf("PromptText() error = %v", err)
	}
	if !listenerFinished || agent.State().IsStreaming {
		t.Fatalf("listenerFinished=%v isStreaming=%v", listenerFinished, agent.State().IsStreaming)
	}
}

func TestAgentPassesActiveContextToSubscribers(t *testing.T) {
	agent := New(WithStreamFn(func(_ llm.Model, _ llm.Context, options llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
		stream := llm.NewAssistantMessageEventStream()
		go func() {
			stream.Push(llm.AssistantMessageEvent{Type: "start", Partial: testAssistantMessage([]llm.ContentPart{llm.Text("")}, llm.StopReasonStop)})
			<-time.After(5 * time.Millisecond)
			stream.Push(llm.AssistantMessageEvent{Type: "error", Reason: llm.StopReasonAborted, Error: llm.AssistantErrorMessage("Aborted", testModel(), true)})
		}()
		return stream, nil
	}))
	var received context.Context
	agent.Subscribe(func(event AgentEvent, ctx context.Context) error {
		if event.Type == "agent_start" {
			received = ctx
		}
		return nil
	})

	err := agent.PromptText(context.Background(), "hello")
	if err != nil {
		t.Fatalf("PromptText() error = %v", err)
	}
	if received == nil {
		t.Fatal("subscriber did not receive context")
	}
	if received.Err() == nil {
		t.Fatal("context should be canceled after run finishes")
	}
}

func TestAgentStateMutatorsCopyTopLevelSlices(t *testing.T) {
	agent := New()
	model := llm.MustGetModel("google", "gemini-2.5-flash")
	agent.SetSystemPrompt("Custom prompt")
	agent.SetModel(model)
	agent.SetThinkingLevel("high")
	tools := []AgentTool{{Name: "test", Description: "test tool"}}
	agent.SetTools(tools)
	messages := []llm.Message{llm.UserMessageText("Hello")}
	agent.SetMessages(messages)

	tools[0].Name = "mutated"
	messages[0] = llm.UserMessageText("Mutated")
	state := agent.State()
	if state.SystemPrompt != "Custom prompt" || state.Model.ID != model.ID || state.ThinkingLevel != "high" {
		t.Fatalf("state = %#v", state)
	}
	if state.Tools[0].Name != "test" {
		t.Fatalf("tools were not copied: %#v", state.Tools)
	}
	if state.Messages[0].Content[0].Text != "Hello" {
		t.Fatalf("messages were not copied: %#v", state.Messages)
	}
}

func TestAgentQueuesAndAbort(t *testing.T) {
	agent := New()
	message := llm.UserMessageText("queued")
	agent.Steer(message)
	if !agent.HasQueuedMessages() {
		t.Fatal("expected queued steering message")
	}
	agent.ClearAllQueues()
	if agent.HasQueuedMessages() {
		t.Fatal("expected queues cleared")
	}
	agent.Abort()
}

func TestAgentForwardsSessionIDToStreamFnOptions(t *testing.T) {
	var sessionID string
	agent := New(WithSession("session-1"), WithStreamFn(func(_ llm.Model, _ llm.Context, options llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
		sessionID = options.SessionID
		return testStream(testAssistantMessage([]llm.ContentPart{llm.Text("ok")}, llm.StopReasonStop))(llm.Model{}, llm.Context{}, llm.SimpleStreamOptions{})
	}))
	if err := agent.PromptText(context.Background(), "hello"); err != nil {
		t.Fatalf("PromptText() error = %v", err)
	}
	if sessionID != "session-1" {
		t.Fatalf("sessionID = %q, want session-1", sessionID)
	}
}
