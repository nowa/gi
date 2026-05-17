package gillmprovider

import (
	"strings"
	"testing"
)

func TestCopilotClaudeAnthropicHeadersAndPayload(t *testing.T) {
	model := MustGetModel("github-copilot", "claude-sonnet-4.6")
	if model.API != "anthropic-messages" {
		t.Fatalf("model = %#v", model)
	}
	contextValue := Context{
		SystemPrompt: "You are a helpful assistant.",
		Messages:     []Message{UserMessageText("Hello")},
	}

	headers := BuildAnthropicRequestHeaders(model, contextValue, AnthropicPayloadOptions{})

	if !strings.Contains(headers["User-Agent"], "GitHubCopilotChat") {
		t.Fatalf("headers = %#v", headers)
	}
	if headers["Copilot-Integration-Id"] != "vscode-chat" || headers["X-Initiator"] != "user" || headers["Openai-Intent"] != "conversation-edits" {
		t.Fatalf("headers = %#v", headers)
	}
	if strings.Contains(headers["anthropic-beta"], fineGrainedToolStreamingBeta) {
		t.Fatalf("fine grained beta should be omitted without tools: %#v", headers)
	}

	payload := BuildAnthropicPayload(model, contextValue, AnthropicPayloadOptions{})
	if payload.Model != "claude-sonnet-4.6" || !payload.Stream || payload.MaxTokens <= 0 || len(payload.Messages) != 1 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCopilotDynamicHeadersAgentAndVision(t *testing.T) {
	model := MustGetModel("github-copilot", "claude-sonnet-4.6")
	assistant := AssistantMessage([]ContentPart{Text("ok")}, StopReasonStop, model)
	headers := BuildCopilotDynamicHeaders([]Message{UserMessageText("hi"), assistant})
	if headers["X-Initiator"] != "agent" {
		t.Fatalf("headers = %#v", headers)
	}

	headers = BuildCopilotDynamicHeaders([]Message{{Role: RoleUser, Content: []ContentPart{Text("see"), Image("abc", "image/png")}}})
	if headers["Copilot-Vision-Request"] != "true" {
		t.Fatalf("vision headers = %#v", headers)
	}
}

func TestCopilotAdaptiveThinkingOmitsInterleavedBeta(t *testing.T) {
	model := MustGetModel("github-copilot", "claude-sonnet-4.6")
	headers := BuildAnthropicRequestHeaders(model, Context{Messages: []Message{UserMessageText("hi")}}, AnthropicPayloadOptions{
		Reasoning:        "high",
		InterleavedThink: ptrBool(true),
	})
	if strings.Contains(headers["anthropic-beta"], interleavedThinkingBeta) {
		t.Fatalf("headers = %#v", headers)
	}
}
