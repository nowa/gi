package gillmprovider

import (
	"strings"
	"testing"
)

func TestCrossProviderHandoffConvertersAcceptMixedHistory(t *testing.T) {
	contextValue := mixedProviderHandoffContext()

	anthropic := MustGetModel("anthropic", "claude-sonnet-4-5")
	anthropicMessages := ConvertAnthropicMessages(anthropic, contextValue, false, nil)
	assertAnthropicToolIDsPortable(t, anthropicMessages)

	openAIChat := MustGetModel("openai", "gpt-4o-mini")
	openAIChat.API = "openai-completions"
	chatMessages := ConvertOpenAICompletionsMessages(openAIChat, contextValue, ResolveOpenAICompletionsCompat(openAIChat))
	if len(chatMessages) == 0 || !chatMessagesContainToolCallID(chatMessages, "call_foreign") {
		t.Fatalf("openai chat messages = %#v", chatMessages)
	}

	responsesModel := MustGetModel("openai-codex", "gpt-5.4")
	responsesItems := ConvertOpenAIResponsesMessages(responsesModel, contextValue, ConvertOpenAIResponsesOptions{})
	if len(responsesItems) == 0 || !responsesItemsContainFunctionCall(responsesItems, "call_foreign") {
		t.Fatalf("responses items = %#v", responsesItems)
	}

	google := Model{ID: "gemini-3-flash-preview", Provider: "google", API: "google-generative-ai", Reasoning: true, Input: []string{"text", "image"}}
	googleContents := ConvertGoogleMessages(google, contextValue)
	if len(googleContents) == 0 || !googleContentsContainFunctionResponse(googleContents) {
		t.Fatalf("google contents = %#v", googleContents)
	}

	mistral := MustGetModel("mistral", "mistral-medium-3.5")
	mistralMessages := ConvertMistralMessages(mistral, contextValue)
	if len(mistralMessages) == 0 || !mistralMessagesContainToolResult(mistralMessages, "call_foreign") {
		t.Fatalf("mistral messages = %#v", mistralMessages)
	}
}

func mixedProviderHandoffContext() Context {
	copilotSource := Model{ID: "gpt-5.1-codex", Provider: "github-copilot", API: "openai-responses", Reasoning: true}
	googleSource := Model{ID: "gemini-3-flash-preview", Provider: "google", API: "google-generative-ai", Reasoning: true}
	assistant := AssistantMessage([]ContentPart{
		Thinking("Need to double the number."),
		ToolCall("call_foreign|item/with+foreign==", "double_number", map[string]any{"value": 21}),
	}, "toolUse", copilotSource)
	final := AssistantMessage([]ContentPart{
		Thinking("Tool result was processed."),
		Text("The doubled value is 42."),
	}, StopReasonStop, googleSource)
	return Context{
		SystemPrompt: "You are a helpful assistant.",
		Messages: []Message{
			UserMessageText("Please double 21."),
			assistant,
			{Role: RoleToolResult, ToolCallID: "call_foreign|item/with+foreign==", ToolName: "double_number", Content: []ContentPart{Text("42")}, Timestamp: NowMillis()},
			final,
			UserMessageText("Now say hi."),
		},
		Tools: []Tool{{
			Name:        "double_number",
			Description: "Doubles a number.",
			Parameters:  Object(map[string]Schema{"value": Number()}, "value"),
		}},
	}
}

func assertAnthropicToolIDsPortable(t *testing.T, messages []AnthropicMessage) {
	t.Helper()
	found := false
	for _, message := range messages {
		blocks, ok := message.Content.([]AnthropicContentBlock)
		if !ok {
			continue
		}
		for _, block := range blocks {
			switch block.Type {
			case "tool_use":
				found = true
				if strings.Contains(block.ID, "|") || strings.Contains(block.ID, "+") || strings.Contains(block.ID, "/") {
					t.Fatalf("non-portable anthropic tool id = %q", block.ID)
				}
			case "tool_result":
				if strings.Contains(block.ToolUseID, "|") || strings.Contains(block.ToolUseID, "+") || strings.Contains(block.ToolUseID, "/") {
					t.Fatalf("non-portable anthropic tool result id = %q", block.ToolUseID)
				}
			}
		}
	}
	if !found {
		t.Fatalf("anthropic messages missing tool call: %#v", messages)
	}
}

func chatMessagesContainToolCallID(messages []OpenAIChatMessage, id string) bool {
	for _, message := range messages {
		for _, toolCall := range message.ToolCalls {
			if toolCall.ID == id {
				return true
			}
		}
	}
	return false
}

func responsesItemsContainFunctionCall(items []OpenAIResponsesInputItem, callID string) bool {
	for _, item := range items {
		if item.Type == "function_call" && item.CallID == callID && strings.HasPrefix(item.ID, "fc_") {
			return true
		}
	}
	return false
}

func googleContentsContainFunctionResponse(contents []GoogleContent) bool {
	for _, content := range contents {
		for _, part := range content.Parts {
			if part.FunctionResponse != nil {
				return true
			}
		}
	}
	return false
}

func mistralMessagesContainToolResult(messages []MistralMessage, id string) bool {
	for _, message := range messages {
		if message.Role == "tool" && message.ToolCallID == id {
			return true
		}
	}
	return false
}
