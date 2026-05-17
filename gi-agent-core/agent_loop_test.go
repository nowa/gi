package giagentcore

import (
	"context"
	"reflect"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func testModel() llm.Model {
	return llm.Model{ID: "mock", Name: "mock", API: "openai-responses", Provider: "openai", BaseURL: "https://example.invalid", Input: []string{"text"}, ContextWindow: 8192, MaxTokens: 2048}
}

func testAssistantMessage(content []llm.ContentPart, stopReason string) llm.Message {
	return llm.AssistantMessage(content, stopReason, testModel())
}

func testStream(message llm.Message) StreamFn {
	return func(_ llm.Model, _ llm.Context, _ llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
		stream := llm.NewAssistantMessageEventStream()
		go stream.Push(llm.AssistantMessageEvent{Type: "done", Reason: message.StopReason, Message: message})
		return stream, nil
	}
}

func sequenceStream(messages ...llm.Message) StreamFn {
	index := 0
	return func(_ llm.Model, _ llm.Context, _ llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
		if index >= len(messages) {
			index = len(messages) - 1
		}
		message := messages[index]
		index++
		return testStream(message)(llm.Model{}, llm.Context{}, llm.SimpleStreamOptions{})
	}
}

func identityConverter(messages []llm.Message) ([]llm.Message, error) {
	result := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == llm.RoleUser || message.Role == llm.RoleAssistant || message.Role == llm.RoleToolResult {
			result = append(result, message)
		}
	}
	return result, nil
}

func collectAgentStream(t *testing.T, stream *llm.EventStream[AgentEvent, []llm.Message]) ([]AgentEvent, []llm.Message) {
	t.Helper()
	var events []AgentEvent
	for event := range stream.Events() {
		events = append(events, event)
	}
	messages, err := stream.Result(context.Background())
	if err != nil {
		t.Fatalf("stream.Result() error = %v", err)
	}
	return events, messages
}

func TestAgentLoopEmitsEventsWithAgentMessageTypes(t *testing.T) {
	contextSnapshot := AgentContext{SystemPrompt: "You are helpful.", Tools: []AgentTool{}}
	prompt := llm.UserMessageText("Hello")
	config := AgentLoopConfig{Model: testModel(), ConvertToLLM: identityConverter}

	events, messages := collectAgentStream(t, AgentLoop(
		[]llm.Message{prompt},
		contextSnapshot,
		config,
		context.Background(),
		testStream(testAssistantMessage([]llm.ContentPart{llm.Text("Hi there!")}, llm.StopReasonStop)),
	))

	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if messages[0].Role != llm.RoleUser || messages[1].Role != llm.RoleAssistant {
		t.Fatalf("message roles = %q/%q, want user/assistant", messages[0].Role, messages[1].Role)
	}
	gotTypes := eventTypes(events)
	for _, want := range []string{"agent_start", "turn_start", "message_start", "message_end", "turn_end", "agent_end"} {
		if !contains(gotTypes, want) {
			t.Fatalf("event types = %#v, want contains %s", gotTypes, want)
		}
	}
}

func TestAgentLoopAppliesTransformContextBeforeConvertToLLM(t *testing.T) {
	contextSnapshot := AgentContext{
		SystemPrompt: "You are helpful.",
		Messages: []llm.Message{
			llm.UserMessageText("old message 1"),
			testAssistantMessage([]llm.ContentPart{llm.Text("old response 1")}, llm.StopReasonStop),
			llm.UserMessageText("old message 2"),
			testAssistantMessage([]llm.ContentPart{llm.Text("old response 2")}, llm.StopReasonStop),
		},
	}
	prompt := llm.UserMessageText("new message")
	var transformed []llm.Message
	var converted []llm.Message

	config := AgentLoopConfig{
		Model: testModel(),
		TransformContext: func(_ context.Context, messages []llm.Message) ([]llm.Message, error) {
			transformed = append([]llm.Message{}, messages[len(messages)-2:]...)
			return transformed, nil
		},
		ConvertToLLM: func(messages []llm.Message) ([]llm.Message, error) {
			converted = append([]llm.Message{}, messages...)
			return identityConverter(messages)
		},
	}

	_, _ = collectAgentStream(t, AgentLoop([]llm.Message{prompt}, contextSnapshot, config, context.Background(), testStream(testAssistantMessage([]llm.ContentPart{llm.Text("Response")}, llm.StopReasonStop))))
	if len(transformed) != 2 {
		t.Fatalf("transformed len = %d, want 2", len(transformed))
	}
	if len(converted) != 2 {
		t.Fatalf("converted len = %d, want 2", len(converted))
	}
}

func TestAgentLoopHandlesToolCallsAndResults(t *testing.T) {
	executed := []string{}
	tool := AgentTool{
		Name:        "echo",
		Label:       "Echo",
		Description: "Echo tool",
		Parameters:  llm.Object(map[string]llm.Schema{"value": llm.String()}, "value"),
		Execute: func(_ context.Context, _ string, params map[string]any, _ AgentToolUpdateCallback) (AgentToolResult, error) {
			value := params["value"].(string)
			executed = append(executed, value)
			return AgentToolResult{Content: []llm.ContentPart{llm.Text("echoed: " + value)}, Details: map[string]any{"value": value}}, nil
		},
	}
	contextSnapshot := AgentContext{Tools: []AgentTool{tool}}
	prompt := llm.UserMessageText("use tool")
	assistant := testAssistantMessage([]llm.ContentPart{llm.ToolCall("call-1", "echo", map[string]any{"value": "hello"})}, llm.StopReasonStop)

	events, messages := collectAgentStream(t, AgentLoop([]llm.Message{prompt}, contextSnapshot, AgentLoopConfig{Model: testModel(), ConvertToLLM: identityConverter}, context.Background(), sequenceStream(
		assistant,
		testAssistantMessage([]llm.ContentPart{llm.Text("done")}, llm.StopReasonStop),
	)))

	if !reflect.DeepEqual(executed, []string{"hello"}) {
		t.Fatalf("executed = %#v, want hello", executed)
	}
	if len(messages) != 4 || messages[2].Role != llm.RoleToolResult {
		t.Fatalf("messages = %#v, want third tool result and final assistant", messages)
	}
	if messages[2].ToolCallID != "call-1" || messages[2].Content[0].Text != "echoed: hello" {
		t.Fatalf("tool result = %#v", messages[2])
	}
	if !contains(eventTypes(events), "tool_execution_start") || !contains(eventTypes(events), "tool_execution_end") {
		t.Fatalf("events = %#v, want tool execution lifecycle", eventTypes(events))
	}
}

func TestAgentLoopParallelToolResultsPreserveSourceOrder(t *testing.T) {
	makeTool := func(name string, delay time.Duration) AgentTool {
		return AgentTool{
			Name:        name,
			Label:       name,
			Description: name,
			Parameters:  llm.Object(map[string]llm.Schema{}),
			Execute: func(_ context.Context, toolCallID string, _ map[string]any, _ AgentToolUpdateCallback) (AgentToolResult, error) {
				time.Sleep(delay)
				return AgentToolResult{Content: []llm.ContentPart{llm.Text(toolCallID)}, Details: toolCallID}, nil
			},
		}
	}
	contextSnapshot := AgentContext{Tools: []AgentTool{makeTool("slow", 30*time.Millisecond), makeTool("fast", 1*time.Millisecond)}}
	assistant := testAssistantMessage([]llm.ContentPart{
		llm.ToolCall("slow-call", "slow", map[string]any{}),
		llm.ToolCall("fast-call", "fast", map[string]any{}),
	}, llm.StopReasonStop)

	events, messages := collectAgentStream(t, AgentLoop([]llm.Message{llm.UserMessageText("run")}, contextSnapshot, AgentLoopConfig{Model: testModel(), ConvertToLLM: identityConverter}, context.Background(), sequenceStream(
		assistant,
		testAssistantMessage([]llm.ContentPart{llm.Text("done")}, llm.StopReasonStop),
	)))

	var endOrder []string
	for _, event := range events {
		if event.Type == "tool_execution_end" {
			endOrder = append(endOrder, event.ToolCallID)
		}
	}
	if !reflect.DeepEqual(endOrder, []string{"fast-call", "slow-call"}) {
		t.Fatalf("tool_execution_end order = %#v, want fast then slow", endOrder)
	}
	if messages[len(messages)-3].ToolCallID != "slow-call" || messages[len(messages)-2].ToolCallID != "fast-call" {
		t.Fatalf("tool result source order = %s/%s, want slow/fast", messages[len(messages)-3].ToolCallID, messages[len(messages)-2].ToolCallID)
	}
}

func TestAgentLoopStopsWhenAllToolResultsTerminate(t *testing.T) {
	tool := AgentTool{
		Name:       "stop",
		Parameters: llm.Object(map[string]llm.Schema{}),
		Execute: func(_ context.Context, _ string, _ map[string]any, _ AgentToolUpdateCallback) (AgentToolResult, error) {
			return AgentToolResult{Content: []llm.ContentPart{llm.Text("done")}, Terminate: true}, nil
		},
	}
	calls := 0
	streamFn := func(_ llm.Model, _ llm.Context, _ llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
		calls++
		return testStream(testAssistantMessage([]llm.ContentPart{llm.ToolCall("call-1", "stop", map[string]any{})}, llm.StopReasonStop))(llm.Model{}, llm.Context{}, llm.SimpleStreamOptions{})
	}

	_, messages := collectAgentStream(t, AgentLoop([]llm.Message{llm.UserMessageText("run")}, AgentContext{Tools: []AgentTool{tool}}, AgentLoopConfig{Model: testModel(), ConvertToLLM: identityConverter}, context.Background(), streamFn))
	if calls != 1 {
		t.Fatalf("stream calls = %d, want 1", calls)
	}
	if len(messages) != 3 {
		t.Fatalf("messages len = %d, want 3", len(messages))
	}
}

func eventTypes(events []AgentEvent) []string {
	result := make([]string, len(events))
	for i, event := range events {
		result[i] = event.Type
	}
	return result
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
