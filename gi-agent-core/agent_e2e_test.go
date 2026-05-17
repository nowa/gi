package giagentcore

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentIntegrationWithFauxProvider(t *testing.T) {
	t.Run("handles a basic text prompt", func(t *testing.T) {
		faux := llm.RegisterFauxProvider()
		defer faux.Unregister()
		faux.SetResponses([]llm.FauxResponseStep{{Message: llm.FauxAssistantText("4")}})
		agent := newFauxAgent(faux.MustModel(), nil)

		if err := agent.PromptText(context.Background(), "What is 2+2? Answer with just the number."); err != nil {
			t.Fatal(err)
		}

		state := agent.State()
		if state.IsStreaming || len(state.Messages) != 2 || state.Messages[0].Role != llm.RoleUser || state.Messages[1].Role != llm.RoleAssistant {
			t.Fatalf("state = %#v", state)
		}
		if !agentMessageTextContains(state.Messages[1], "4") {
			t.Fatalf("assistant = %#v", state.Messages[1])
		}
	})

	t.Run("executes tools and tracks pending tool calls", func(t *testing.T) {
		faux := llm.RegisterFauxProvider()
		defer faux.Unregister()
		faux.SetResponses([]llm.FauxResponseStep{
			{Message: llm.FauxAssistantMessage([]llm.ContentPart{
				llm.FauxText("Let me calculate that."),
				llm.FauxToolCall("calculate", map[string]any{"expression": "123 * 456"}, "calc-1"),
			}, "toolUse")},
			{Message: llm.FauxAssistantText("The result is 56088.")},
		})
		agent := newFauxAgent(faux.MustModel(), []AgentTool{calculateTestTool()})
		var pendingDuringEvents []eventPendingSnapshot
		agent.Subscribe(func(event AgentEvent, _ context.Context) error {
			if event.Type == "tool_execution_start" || event.Type == "tool_execution_end" {
				pendingDuringEvents = append(pendingDuringEvents, eventPendingSnapshot{Type: event.Type, IDs: sortedPendingIDs(agent.State().PendingToolCalls)})
			}
			return nil
		})

		if err := agent.PromptText(context.Background(), "Calculate 123 * 456 using the calculator tool."); err != nil {
			t.Fatal(err)
		}

		state := agent.State()
		if state.IsStreaming || len(state.Messages) < 4 || len(state.PendingToolCalls) != 0 {
			t.Fatalf("state = %#v", state)
		}
		toolResult := findMessageRole(state.Messages, llm.RoleToolResult)
		if toolResult == nil || !agentMessageTextContains(*toolResult, "123 * 456 = 56088") {
			t.Fatalf("tool result = %#v", toolResult)
		}
		final := state.Messages[len(state.Messages)-1]
		if final.Role != llm.RoleAssistant || !agentMessageTextContains(final, "56088") {
			t.Fatalf("final = %#v", final)
		}
		want := []eventPendingSnapshot{
			{Type: "tool_execution_start", IDs: []string{"calc-1"}},
			{Type: "tool_execution_end", IDs: []string{}},
		}
		if !reflect.DeepEqual(pendingDuringEvents, want) {
			t.Fatalf("pending snapshots = %#v, want %#v", pendingDuringEvents, want)
		}
	})

	t.Run("handles abort during streaming", func(t *testing.T) {
		agent := New(WithInitialState(AgentState{
			SystemPrompt:  "You are a helpful assistant.",
			Model:         testModel(),
			ThinkingLevel: "off",
		}), WithStreamFn(abortableSlowStream()))

		done := make(chan error, 1)
		go func() {
			done <- agent.PromptText(context.Background(), "Count slowly from 1 to 20.")
		}()
		time.Sleep(30 * time.Millisecond)
		agent.Abort()

		if err := <-done; err != nil {
			t.Fatal(err)
		}
		state := agent.State()
		if state.IsStreaming || len(state.Messages) < 2 {
			t.Fatalf("state = %#v", state)
		}
		last := state.Messages[len(state.Messages)-1]
		if last.Role != llm.RoleAssistant || last.StopReason != llm.StopReasonAborted || last.ErrorMessage == "" || state.ErrorMessage != last.ErrorMessage {
			t.Fatalf("last/state = %#v / %#v", last, state)
		}
	})

	t.Run("emits lifecycle updates while streaming", func(t *testing.T) {
		faux := llm.RegisterFauxProvider()
		defer faux.Unregister()
		faux.SetResponses([]llm.FauxResponseStep{{Message: llm.FauxAssistantText("1 2 3 4 5")}})
		agent := newFauxAgent(faux.MustModel(), nil)
		var events []string
		agent.Subscribe(func(event AgentEvent, _ context.Context) error {
			events = append(events, event.Type)
			return nil
		})

		if err := agent.PromptText(context.Background(), "Count from 1 to 5."); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"agent_start", "turn_start", "message_start", "message_update", "message_end", "turn_end", "agent_end"} {
			if !contains(events, want) {
				t.Fatalf("events = %#v, missing %s", events, want)
			}
		}
		if indexOf(events, "agent_start") >= indexOf(events, "message_start") || indexOf(events, "message_start") >= indexOf(events, "message_end") || indexOf(events, "message_end") >= lastIndexOf(events, "agent_end") {
			t.Fatalf("event order = %#v", events)
		}
		if state := agent.State(); state.IsStreaming || len(state.Messages) != 2 {
			t.Fatalf("state = %#v", state)
		}
	})

	t.Run("maintains context across multiple turns", func(t *testing.T) {
		faux := llm.RegisterFauxProvider()
		defer faux.Unregister()
		faux.SetResponses([]llm.FauxResponseStep{
			{Message: llm.FauxAssistantText("Nice to meet you, Alice.")},
			{Factory: func(llmContext llm.Context, _ llm.StreamOptions, _ llm.FauxState, _ llm.Model) (llm.Message, error) {
				if contextContainsText(llmContext.Messages, "Alice") {
					return llm.FauxAssistantText("Your name is Alice."), nil
				}
				return llm.FauxAssistantText("I do not know your name."), nil
			}},
		})
		agent := newFauxAgent(faux.MustModel(), nil)

		if err := agent.PromptText(context.Background(), "My name is Alice."); err != nil {
			t.Fatal(err)
		}
		if len(agent.State().Messages) != 2 {
			t.Fatalf("messages = %#v", agent.State().Messages)
		}
		if err := agent.PromptText(context.Background(), "What is my name?"); err != nil {
			t.Fatal(err)
		}
		state := agent.State()
		if len(state.Messages) != 4 || !strings.Contains(strings.ToLower(messageText(state.Messages[3])), "alice") {
			t.Fatalf("messages = %#v", state.Messages)
		}
	})

	t.Run("preserves thinking content blocks", func(t *testing.T) {
		faux := llm.RegisterFauxProvider(llm.WithFauxModels(llm.FauxModelDefinition{ID: "faux-reasoning", Reasoning: true}))
		defer faux.Unregister()
		faux.SetResponses([]llm.FauxResponseStep{{Message: llm.FauxAssistantMessage([]llm.ContentPart{llm.FauxThinking("step by step"), llm.FauxText("4")}, llm.StopReasonStop)}})
		agent := newFauxAgent(faux.MustModel("faux-reasoning"), nil)
		agent.SetThinkingLevel("low")

		if err := agent.PromptText(context.Background(), "What is 2+2?"); err != nil {
			t.Fatal(err)
		}
		assistant := agent.State().Messages[1]
		want := []llm.ContentPart{llm.Thinking("step by step"), llm.Text("4")}
		if !reflect.DeepEqual(assistant.Content, want) {
			t.Fatalf("content = %#v, want %#v", assistant.Content, want)
		}
	})
}

func TestAgentContinueWithFauxProvider(t *testing.T) {
	t.Run("validates continuation preconditions", func(t *testing.T) {
		faux := llm.RegisterFauxProvider()
		defer faux.Unregister()
		agent := newFauxAgent(faux.MustModel(), nil)
		if err := agent.Continue(context.Background()); err == nil || !strings.Contains(err.Error(), "No messages to continue from") {
			t.Fatalf("empty continue error = %v", err)
		}

		agent.SetMessages([]llm.Message{llm.AssistantMessage([]llm.ContentPart{llm.Text("Hello")}, llm.StopReasonStop, faux.MustModel())})
		if err := agent.Continue(context.Background()); err == nil || !strings.Contains(err.Error(), "Cannot continue from message role: assistant") {
			t.Fatalf("assistant continue error = %v", err)
		}
	})

	t.Run("continues from user message", func(t *testing.T) {
		faux := llm.RegisterFauxProvider()
		defer faux.Unregister()
		faux.SetResponses([]llm.FauxResponseStep{{Message: llm.FauxAssistantText("HELLO WORLD")}})
		agent := newFauxAgent(faux.MustModel(), nil)
		agent.SetMessages([]llm.Message{llm.UserMessageText("Say exactly: HELLO WORLD")})

		if err := agent.Continue(context.Background()); err != nil {
			t.Fatal(err)
		}
		state := agent.State()
		if state.IsStreaming || len(state.Messages) != 2 || !strings.Contains(strings.ToUpper(messageText(state.Messages[1])), "HELLO WORLD") {
			t.Fatalf("state = %#v", state)
		}
	})

	t.Run("continues from tool result", func(t *testing.T) {
		faux := llm.RegisterFauxProvider()
		defer faux.Unregister()
		model := faux.MustModel()
		faux.SetResponses([]llm.FauxResponseStep{{Message: llm.FauxAssistantText("The answer is 8.")}})
		agent := newFauxAgent(model, []AgentTool{calculateTestTool()})
		agent.SetMessages([]llm.Message{
			llm.UserMessageText("What is 5 + 3?"),
			llm.AssistantMessage([]llm.ContentPart{
				llm.Text("Let me calculate that."),
				llm.ToolCall("calc-1", "calculate", map[string]any{"expression": "5 + 3"}),
			}, "toolUse", model),
			{Role: llm.RoleToolResult, ToolCallID: "calc-1", ToolName: "calculate", Content: []llm.ContentPart{llm.Text("5 + 3 = 8")}, Timestamp: llm.NowMillis()},
		})

		if err := agent.Continue(context.Background()); err != nil {
			t.Fatal(err)
		}
		state := agent.State()
		last := state.Messages[len(state.Messages)-1]
		if state.IsStreaming || len(state.Messages) < 4 || last.Role != llm.RoleAssistant || !agentMessageTextContains(last, "8") {
			t.Fatalf("state = %#v", state)
		}
	})
}

type eventPendingSnapshot struct {
	Type string
	IDs  []string
}

func newFauxAgent(model llm.Model, tools []AgentTool) *Agent {
	return New(WithInitialState(AgentState{
		SystemPrompt:  "You are a helpful assistant.",
		Model:         model,
		ThinkingLevel: "off",
		Tools:         tools,
	}))
}

func calculateTestTool() AgentTool {
	return AgentTool{
		Name:        "calculate",
		Description: "Evaluate a simple arithmetic expression.",
		Parameters:  llm.Object(map[string]llm.Schema{"expression": llm.String()}, "expression"),
		Execute: func(_ context.Context, _ string, params map[string]any, _ AgentToolUpdateCallback) (AgentToolResult, error) {
			expression, _ := params["expression"].(string)
			switch expression {
			case "123 * 456":
				return AgentToolResult{Content: []llm.ContentPart{llm.Text("123 * 456 = 56088")}}, nil
			case "5 + 3":
				return AgentToolResult{Content: []llm.ContentPart{llm.Text("5 + 3 = 8")}}, nil
			default:
				return AgentToolResult{Content: []llm.ContentPart{llm.Text("unsupported expression: " + expression)}}, nil
			}
		},
	}
}

func abortableSlowStream() StreamFn {
	return func(model llm.Model, _ llm.Context, options llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
		stream := llm.NewAssistantMessageEventStream()
		go func() {
			partial := llm.AssistantMessage(nil, llm.StopReasonStop, model)
			stream.Push(llm.AssistantMessageEvent{Type: "start", Partial: partial})
			select {
			case <-options.Context.Done():
				stream.Push(llm.AssistantMessageEvent{Type: "error", Reason: llm.StopReasonAborted, Error: llm.AssistantErrorMessage(options.Context.Err().Error(), model, true)})
			case <-time.After(250 * time.Millisecond):
				stream.Push(llm.AssistantMessageEvent{Type: "done", Reason: llm.StopReasonStop, Message: llm.AssistantMessage([]llm.ContentPart{llm.Text("finished")}, llm.StopReasonStop, model)})
			}
		}()
		return stream, nil
	}
}

func sortedPendingIDs(pending map[string]bool) []string {
	ids := make([]string, 0, len(pending))
	for id, ok := range pending {
		if ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func findMessageRole(messages []llm.Message, role string) *llm.Message {
	for i := range messages {
		if messages[i].Role == role {
			return &messages[i]
		}
	}
	return nil
}

func agentMessageTextContains(message llm.Message, want string) bool {
	return strings.Contains(messageText(message), want)
}

func messageText(message llm.Message) string {
	var parts []string
	for _, part := range message.Content {
		if part.Type == llm.ContentText {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func contextContainsText(messages []llm.Message, want string) bool {
	for _, message := range messages {
		if strings.Contains(messageText(message), want) {
			return true
		}
	}
	return false
}

func indexOf(values []string, needle string) int {
	for i, value := range values {
		if value == needle {
			return i
		}
	}
	return -1
}

func lastIndexOf(values []string, needle string) int {
	for i := len(values) - 1; i >= 0; i-- {
		if values[i] == needle {
			return i
		}
	}
	return -1
}
