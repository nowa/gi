package gillmprovider

import (
	"strings"
	"testing"
)

func TestBuildAnthropicPayloadThinkingDisableAndAdaptive(t *testing.T) {
	tests := []struct {
		name         string
		model        Model
		reasoning    string
		wantThinking map[string]any
		wantEffort   string
	}{
		{
			name:         "budget model disables thinking when off",
			model:        MustGetModel("anthropic", "claude-sonnet-4-5"),
			wantThinking: map[string]any{"type": "disabled"},
		},
		{
			name:         "adaptive model disables thinking when off",
			model:        MustGetModel("anthropic", "claude-opus-4-6"),
			wantThinking: map[string]any{"type": "disabled"},
		},
		{
			name:         "opus 4.7 disables thinking when off",
			model:        MustGetModel("anthropic", "claude-opus-4-7"),
			wantThinking: map[string]any{"type": "disabled"},
		},
		{
			name:         "opus 4.7 high uses adaptive thinking",
			model:        MustGetModel("anthropic", "claude-opus-4-7"),
			reasoning:    "high",
			wantThinking: map[string]any{"type": "adaptive", "display": "summarized"},
			wantEffort:   "high",
		},
		{
			name:         "opus 4.7 xhigh maps to xhigh effort",
			model:        MustGetModel("anthropic", "claude-opus-4-7"),
			reasoning:    "xhigh",
			wantThinking: map[string]any{"type": "adaptive", "display": "summarized"},
			wantEffort:   "xhigh",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := BuildAnthropicPayload(tc.model, Context{Messages: []Message{UserMessageText("Hello")}}, AnthropicPayloadOptions{Reasoning: tc.reasoning})
			for key, want := range tc.wantThinking {
				if got := payload.Thinking[key]; got != want {
					t.Fatalf("thinking[%s] = %#v, want %#v; payload=%#v", key, got, want, payload.Thinking)
				}
			}
			if tc.wantEffort != "" {
				if payload.OutputConfig["effort"] != tc.wantEffort {
					t.Fatalf("effort = %#v, want %q", payload.OutputConfig, tc.wantEffort)
				}
			} else if payload.OutputConfig != nil {
				t.Fatalf("output config should be omitted: %#v", payload.OutputConfig)
			}
		})
	}
}

func TestAnthropicClaudeCodeToolNameRoundTrip(t *testing.T) {
	tools := []Tool{
		{Name: "todowrite"},
		{Name: "read"},
		{Name: "find"},
		{Name: "my_custom_tool"},
	}
	tests := []struct {
		name     string
		outbound string
		inbound  string
	}{
		{name: "todo write canonical casing", outbound: "TodoWrite", inbound: "todowrite"},
		{name: "built in read canonical casing", outbound: "Read", inbound: "read"},
		{name: "find is not glob", outbound: "find", inbound: "find"},
		{name: "custom unchanged", outbound: "my_custom_tool", inbound: "my_custom_tool"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := tc.inbound
			if got := ToClaudeCodeToolName(source); got != tc.outbound {
				t.Fatalf("outbound = %q, want %q", got, tc.outbound)
			}
			if got := FromClaudeCodeToolName(tc.outbound, tools); got != tc.inbound {
				t.Fatalf("inbound = %q, want %q", got, tc.inbound)
			}
		})
	}
}

func TestBuildAnthropicPayloadOAuthToolNamesAndSystemPrompt(t *testing.T) {
	model := MustGetModel("anthropic", "claude-sonnet-4-6")
	context := Context{
		SystemPrompt: "Use tools.",
		Messages:     []Message{UserMessageText("Add todo")},
		Tools: []Tool{{
			Name:        "todowrite",
			Description: "Write todo",
			Parameters:  Object(map[string]Schema{"task": String()}, "task"),
		}},
	}

	payload := BuildAnthropicPayload(model, context, AnthropicPayloadOptions{IsOAuthToken: true, CacheRetention: "none"})

	if len(payload.System) != 2 || payload.System[0].Text == "" || payload.System[1].Text != "Use tools." {
		t.Fatalf("system = %#v", payload.System)
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Name != "TodoWrite" {
		t.Fatalf("tools = %#v", payload.Tools)
	}
}

func TestBuildAnthropicPayloadCacheRetention(t *testing.T) {
	model := MustGetModel("anthropic", "claude-sonnet-4-5")
	context := Context{
		SystemPrompt: "You are helpful.",
		Messages:     []Message{UserMessageText("Hello")},
	}

	payload := BuildAnthropicPayload(model, context, AnthropicPayloadOptions{})
	if payload.System[0].CacheControl == nil || payload.System[0].CacheControl.TTL != "" {
		t.Fatalf("system cache control = %#v", payload.System)
	}
	lastContent := payload.Messages[len(payload.Messages)-1].Content.([]AnthropicContentBlock)
	if lastContent[len(lastContent)-1].CacheControl == nil {
		t.Fatalf("last user cache control = %#v", lastContent)
	}

	payload = BuildAnthropicPayload(model, context, AnthropicPayloadOptions{CacheRetention: "long"})
	if payload.System[0].CacheControl == nil || payload.System[0].CacheControl.TTL != "1h" {
		t.Fatalf("system cache control = %#v", payload.System)
	}

	model.Compat.SupportsLongCacheRetention = ptrBool(false)
	payload = BuildAnthropicPayload(model, context, AnthropicPayloadOptions{CacheRetention: "long"})
	if payload.System[0].CacheControl == nil || payload.System[0].CacheControl.TTL != "" {
		t.Fatalf("system cache control = %#v", payload.System)
	}

	payload = BuildAnthropicPayload(model, context, AnthropicPayloadOptions{CacheRetention: "none"})
	if payload.System[0].CacheControl != nil {
		t.Fatalf("system cache control should be omitted = %#v", payload.System)
	}
}

func TestBuildAnthropicPayloadUsesPICacheRetentionEnv(t *testing.T) {
	t.Setenv("PI_CACHE_RETENTION", "long")
	model := MustGetModel("anthropic", "claude-sonnet-4-5")

	payload := BuildAnthropicPayload(model, Context{SystemPrompt: "sys", Messages: []Message{UserMessageText("hi")}}, AnthropicPayloadOptions{})

	if payload.System[0].CacheControl == nil || payload.System[0].CacheControl.TTL != "1h" {
		t.Fatalf("system cache control = %#v", payload.System)
	}
}

func TestBuildAnthropicHeadersEagerToolInputCompatibility(t *testing.T) {
	model := Model{
		ID:       "claude-opus-4-7",
		Name:     "Claude Opus 4.7",
		API:      "anthropic-messages",
		Provider: "test-anthropic",
		BaseURL:  "http://127.0.0.1:9",
		Input:    []string{"text"},
	}
	context := Context{
		Messages: []Message{UserMessageText("Use tool")},
		Tools: []Tool{{
			Name:        "lookup",
			Description: "Lookup",
			Parameters:  Object(map[string]Schema{"value": String()}, "value"),
		}},
	}

	payload := BuildAnthropicPayload(model, context, AnthropicPayloadOptions{CacheRetention: "none"})
	if len(payload.Tools) != 1 || payload.Tools[0].EagerInputStreaming == nil || !*payload.Tools[0].EagerInputStreaming {
		t.Fatalf("tool eager input = %#v", payload.Tools)
	}
	headers := BuildAnthropicHeaders(model, context, AnthropicPayloadOptions{})
	if _, ok := headers["anthropic-beta"]; ok {
		t.Fatalf("beta header should be omitted: %#v", headers)
	}

	model.Compat.SupportsEagerToolInputStreaming = ptrBool(false)
	payload = BuildAnthropicPayload(model, context, AnthropicPayloadOptions{CacheRetention: "none"})
	if payload.Tools[0].EagerInputStreaming != nil {
		t.Fatalf("eager input should be omitted: %#v", payload.Tools[0])
	}
	headers = BuildAnthropicHeaders(model, context, AnthropicPayloadOptions{})
	if headers["anthropic-beta"] != fineGrainedToolStreamingBeta {
		t.Fatalf("beta header = %#v", headers)
	}

	headers = BuildAnthropicHeaders(model, Context{Messages: []Message{UserMessageText("No tools")}}, AnthropicPayloadOptions{})
	if _, ok := headers["anthropic-beta"]; ok {
		t.Fatalf("beta header should be omitted without tools: %#v", headers)
	}
}

func TestBuildAnthropicHeadersInterleavedThinking(t *testing.T) {
	tests := []struct {
		name      string
		model     Model
		options   AnthropicPayloadOptions
		wantBeta  bool
		forbidAny bool
	}{
		{
			name:     "budget thinking model opts into interleaved beta by default",
			model:    MustGetModel("anthropic", "claude-sonnet-4-5"),
			options:  AnthropicPayloadOptions{Reasoning: "high"},
			wantBeta: true,
		},
		{
			name:      "caller can disable interleaved beta",
			model:     MustGetModel("anthropic", "claude-sonnet-4-5"),
			options:   AnthropicPayloadOptions{Reasoning: "high", InterleavedThink: ptrBool(false)},
			forbidAny: true,
		},
		{
			name:      "adaptive thinking model omits interleaved beta",
			model:     MustGetModel("anthropic", "claude-opus-4-6"),
			options:   AnthropicPayloadOptions{Reasoning: "high"},
			forbidAny: true,
		},
		{
			name:      "disabled thinking omits interleaved beta",
			model:     MustGetModel("anthropic", "claude-sonnet-4-5"),
			options:   AnthropicPayloadOptions{},
			forbidAny: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			headers := BuildAnthropicHeaders(tc.model, Context{Messages: []Message{UserMessageText("hi")}}, tc.options)
			beta := headers["anthropic-beta"]
			if tc.forbidAny && beta != "" {
				t.Fatalf("anthropic-beta = %q, want omitted", beta)
			}
			if strings.Contains(beta, interleavedThinkingBeta) != tc.wantBeta {
				t.Fatalf("anthropic-beta = %q, contains interleaved=%v", beta, strings.Contains(beta, interleavedThinkingBeta))
			}
		})
	}
}

func TestConvertAnthropicMessagesPreservesImageToolResults(t *testing.T) {
	model := MustGetModel("anthropic", "claude-haiku-4-5")
	assistant := AssistantMessage([]ContentPart{
		ToolCall("call_img", "get_circle", nil),
		ToolCall("call_mix", "get_circle_with_description", nil),
	}, "toolUse", model)
	context := Context{Messages: []Message{
		UserMessageText("inspect generated images"),
		assistant,
		{Role: RoleToolResult, ToolCallID: "call_img", ToolName: "get_circle", Content: []ContentPart{Image("red-circle", "image/png")}},
		{Role: RoleToolResult, ToolCallID: "call_mix", ToolName: "get_circle_with_description", Content: []ContentPart{Text("diameter: 100px"), Image("red-circle", "image/png")}},
	}}

	messages := ConvertAnthropicMessages(model, context, false, nil)

	if len(messages) != 3 || messages[2].Role != "user" {
		t.Fatalf("messages = %#v", messages)
	}
	toolResults, ok := messages[2].Content.([]AnthropicContentBlock)
	if !ok || len(toolResults) != 2 {
		t.Fatalf("tool results = %#v", messages[2].Content)
	}
	imageOnly, ok := toolResults[0].Content.([]AnthropicContentBlock)
	if !ok || len(imageOnly) != 1 || imageOnly[0].Source == nil || imageOnly[0].Source.MediaType != "image/png" {
		t.Fatalf("image-only tool result = %#v", toolResults[0].Content)
	}
	mixed, ok := toolResults[1].Content.([]AnthropicContentBlock)
	if !ok || len(mixed) != 2 || mixed[0].Text != "diameter: 100px" || mixed[1].Source == nil {
		t.Fatalf("mixed tool result = %#v", toolResults[1].Content)
	}
}
