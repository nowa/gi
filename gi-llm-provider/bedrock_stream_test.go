package gillmprovider

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestProcessBedrockConverseStreamEventsPiStyle(t *testing.T) {
	model := Model{
		ID:       "bedrock-test",
		Provider: "amazon-bedrock",
		API:      "bedrock-converse-stream",
		Cost:     ModelCost{Input: 1, Output: 2, CacheRead: 0.5, CacheWrite: 1.5},
	}
	output := AssistantMessage(nil, StopReasonStop, model)

	events := ProcessBedrockConverseStreamEvents(model, &output, []BedrockConverseStreamEvent{
		{MessageStart: &BedrockMessageStartEvent{Role: "assistant"}},
		{ContentBlockDelta: &BedrockContentBlockDeltaEvent{ContentBlockIndex: 0, Text: "Hello"}},
		{ContentBlockStop: &BedrockContentBlockStopEvent{ContentBlockIndex: 0}},
		{ContentBlockStart: &BedrockContentBlockStartEvent{ContentBlockIndex: 1, ToolUse: &BedrockToolUseBlock{ToolUseID: "tool-1", Name: "read"}}},
		{ContentBlockDelta: &BedrockContentBlockDeltaEvent{ContentBlockIndex: 1, ToolUseInput: `{"path":"READ`}},
		{ContentBlockDelta: &BedrockContentBlockDeltaEvent{ContentBlockIndex: 1, ToolUseInput: `ME.md"}`}},
		{ContentBlockStop: &BedrockContentBlockStopEvent{ContentBlockIndex: 1}},
		{ContentBlockDelta: &BedrockContentBlockDeltaEvent{ContentBlockIndex: 2, ReasoningContent: &BedrockReasoningContent{Text: "Need file", Signature: "sig_"}}},
		{ContentBlockDelta: &BedrockContentBlockDeltaEvent{ContentBlockIndex: 2, ReasoningContent: &BedrockReasoningContent{Signature: "1"}}},
		{ContentBlockStop: &BedrockContentBlockStopEvent{ContentBlockIndex: 2}},
		{MessageStop: &BedrockMessageStopEvent{StopReason: "tool_use"}},
		{Metadata: &BedrockMetadataEvent{Usage: BedrockUsage{InputTokens: 10, OutputTokens: 5, CacheReadInputTokens: 2, CacheWriteTokens: 1, TotalTokens: 18}}},
	})

	if output.StopReason != StopReasonToolUse {
		t.Fatalf("stop reason = %q", output.StopReason)
	}
	if len(output.Content) != 3 {
		t.Fatalf("content = %#v", output.Content)
	}
	if output.Content[0].Type != ContentText || output.Content[0].Text != "Hello" {
		t.Fatalf("text content = %#v", output.Content[0])
	}
	if output.Content[1].Type != ContentToolCall || output.Content[1].Name != "read" || !reflect.DeepEqual(output.Content[1].Arguments, map[string]any{"path": "README.md"}) {
		t.Fatalf("tool content = %#v", output.Content[1])
	}
	if output.Content[2].Type != ContentThinking || output.Content[2].Thinking != "Need file" || output.Content[2].ThinkingSignature != "sig_1" {
		t.Fatalf("thinking content = %#v", output.Content[2])
	}
	if output.Usage.Input != 10 || output.Usage.Output != 5 || output.Usage.CacheRead != 2 || output.Usage.CacheWrite != 1 || output.Usage.TotalTokens != 18 {
		t.Fatalf("usage = %#v", output.Usage)
	}
	for _, typ := range []string{"start", "text_start", "text_delta", "text_end", "toolcall_start", "toolcall_delta", "toolcall_end", "thinking_start", "thinking_delta", "thinking_end"} {
		if !containsAssistantEvent(events, typ) {
			t.Fatalf("missing event %q in %#v", typ, events)
		}
	}
}

func TestBedrockConverseStreamProviderBuildsRequestAndStreams(t *testing.T) {
	model := Model{
		ID:        "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/my-profile",
		Name:      "Claude Opus 4.6",
		Provider:  "amazon-bedrock",
		API:       "bedrock-converse-stream",
		BaseURL:   "https://bedrock-runtime.us-east-1.amazonaws.com",
		Reasoning: true,
		MaxTokens: 32000,
	}
	var captured BedrockConverseStreamRequest
	provider := NewBedrockConverseStreamProvider(func(_ context.Context, request BedrockConverseStreamRequest) (<-chan BedrockConverseStreamEvent, error) {
		captured = request
		events := make(chan BedrockConverseStreamEvent, 4)
		events <- BedrockConverseStreamEvent{MessageStart: &BedrockMessageStartEvent{Role: "assistant"}}
		events <- BedrockConverseStreamEvent{ContentBlockDelta: &BedrockContentBlockDeltaEvent{ContentBlockIndex: 0, Text: "OK"}}
		events <- BedrockConverseStreamEvent{ContentBlockStop: &BedrockContentBlockStopEvent{ContentBlockIndex: 0}}
		events <- BedrockConverseStreamEvent{MessageStop: &BedrockMessageStopEvent{StopReason: "end_turn"}}
		close(events)
		return events, nil
	})

	stream, err := provider.StreamSimple(model, Context{
		SystemPrompt: "You are helpful.",
		Messages:     []Message{UserMessageText("hi")},
		Tools: []Tool{{
			Name:        "read_file",
			Description: "Read a file",
			Parameters:  Schema{Type: "object"},
		}},
	}, SimpleStreamOptions{
		Reasoning:   "high",
		MaxTokens:   2048,
		Temperature: ptrFloat64(0.2),
		Metadata: map[string]any{
			"region":           "us-west-2",
			"profile":          "dev",
			"thinking_display": "omitted",
			"request_metadata": map[string]any{"owner": "gi"},
			"tool_choice":      "any",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	message, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(message.Content) != 1 || message.Content[0].Text != "OK" {
		t.Fatalf("message = %#v", message)
	}
	if captured.ClientConfig.Region != "us-west-2" || captured.ClientConfig.Profile != "dev" || captured.ClientConfig.Endpoint != "" {
		t.Fatalf("client config = %#v", captured.ClientConfig)
	}
	if captured.MaxTokens != 2048 || captured.Temperature == nil || *captured.Temperature != 0.2 {
		t.Fatalf("request options = %#v", captured)
	}
	if captured.RequestMetadata["owner"] != "gi" {
		t.Fatalf("request metadata = %#v", captured.RequestMetadata)
	}
	if captured.Payload.ToolConfig == nil || len(captured.Payload.ToolConfig.Tools) != 1 {
		t.Fatalf("tool config = %#v", captured.Payload.ToolConfig)
	}
	if !reflect.DeepEqual(captured.Payload.ToolConfig.ToolChoice, map[string]any{"any": map[string]any{}}) {
		t.Fatalf("tool choice = %#v", captured.Payload.ToolConfig.ToolChoice)
	}
	thinking, ok := captured.Payload.AdditionalModelRequestFields["thinking"].(map[string]any)
	if !ok || thinking["type"] != "adaptive" || thinking["display"] != "omitted" {
		t.Fatalf("additional fields = %#v", captured.Payload.AdditionalModelRequestFields)
	}
}

func TestBedrockStreamSimpleKeepsThinkingBudgetWithinRemainingContext(t *testing.T) {
	model := Model{
		ID:            "anthropic.claude-sonnet-4-5",
		Name:          "Claude Sonnet 4.5",
		Provider:      "amazon-bedrock",
		API:           "bedrock-converse-stream",
		Reasoning:     true,
		ContextWindow: 10_000,
		MaxTokens:     8_000,
	}
	contextValue := Context{
		Messages: []Message{UserMessageText(strings.Repeat("x", 8000))},
	}
	var captured BedrockConverseStreamRequest
	provider := NewBedrockConverseStreamProvider(func(
		_ context.Context,
		request BedrockConverseStreamRequest,
	) (<-chan BedrockConverseStreamEvent, error) {
		captured = request
		events := make(chan BedrockConverseStreamEvent)
		close(events)
		return events, nil
	})

	stream, err := provider.StreamSimple(
		model,
		contextValue,
		SimpleStreamOptions{Reasoning: "high"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Result(context.Background()); err != nil {
		t.Fatal(err)
	}
	thinking, ok := captured.Payload.AdditionalModelRequestFields["thinking"].(map[string]any)
	if !ok || captured.MaxTokens != 3_904 || thinking["budget_tokens"] != 2_880 {
		t.Fatalf("request max=%d additional=%#v", captured.MaxTokens, captured.Payload.AdditionalModelRequestFields)
	}
}

func TestBedrockConverseStreamProviderMissingTransportReturnsAssistantError(t *testing.T) {
	model := Model{ID: "bedrock-test", Provider: "amazon-bedrock", API: "bedrock-converse-stream"}
	stream, err := NewBedrockConverseStreamProvider(nil).StreamSimple(model, Context{}, SimpleStreamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	message, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if message.StopReason != StopReasonError || !strings.Contains(message.ErrorMessage, "transport is not configured") {
		t.Fatalf("message = %#v", message)
	}
}

func ptrFloat64(value float64) *float64 {
	return &value
}
