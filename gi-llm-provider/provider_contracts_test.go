package gillmprovider

import "testing"

func TestProviderConvertersHandleEmptyMessages(t *testing.T) {
	openAICompletions := Model{ID: "gpt-4o-mini", Provider: "openai", API: "openai-completions", Input: []string{"text"}}
	openAIResponses := Model{ID: "gpt-5-mini", Provider: "openai", API: "openai-responses", Input: []string{"text"}}
	anthropic := Model{ID: "claude-haiku-4-5", Provider: "anthropic", API: "anthropic-messages", Input: []string{"text"}}
	google := Model{ID: "gemini-2.5-flash", Provider: "google", API: "google-generative-ai", Input: []string{"text"}}
	contextValue := Context{Messages: []Message{
		{Role: RoleUser, Content: nil, Timestamp: NowMillis()},
		{Role: RoleUser, Content: []ContentPart{Text("")}, Timestamp: NowMillis()},
		AssistantMessage(nil, StopReasonStop, openAICompletions),
		UserMessageText("next"),
	}}

	t.Run("openai-completions", func(t *testing.T) {
		messages := ConvertOpenAICompletionsMessages(openAICompletions, contextValue, OpenAICompletionsCompat{})
		if len(messages) != 1 || messages[0].Role != "user" {
			t.Fatalf("messages = %#v", messages)
		}
	})
	t.Run("openai-responses", func(t *testing.T) {
		items := ConvertOpenAIResponsesMessages(openAIResponses, contextValue, ConvertOpenAIResponsesOptions{})
		if len(items) != 1 || items[0].Role != "user" {
			t.Fatalf("items = %#v", items)
		}
	})
	t.Run("anthropic", func(t *testing.T) {
		messages := ConvertAnthropicMessages(anthropic, contextValue, false, nil)
		if len(messages) != 1 || messages[0].Role != "user" {
			t.Fatalf("messages = %#v", messages)
		}
	})
	t.Run("google", func(t *testing.T) {
		messages := ConvertGoogleMessages(google, contextValue)
		if len(messages) != 1 || messages[0].Role != "user" {
			t.Fatalf("messages = %#v", messages)
		}
	})
}

func TestProviderConvertersInsertSyntheticToolResultsForOrphanedCalls(t *testing.T) {
	model := Model{ID: "gpt-4o-mini", Provider: "openai", API: "openai-completions", Input: []string{"text"}}
	contextValue := Context{Messages: []Message{
		UserMessageText("calculate"),
		AssistantMessage([]ContentPart{ToolCall("call-1", "calculate", map[string]any{"expression": "25 * 18"})}, StopReasonToolUse, model),
		UserMessageText("never mind"),
	}}

	messages := ConvertOpenAICompletionsMessages(model, contextValue, OpenAICompletionsCompat{})

	if len(messages) != 4 {
		t.Fatalf("messages = %#v", messages)
	}
	toolResult := messages[2]
	if toolResult.Role != "tool" || toolResult.ToolCallID != "call-1" || toolResult.Content != "No result provided" {
		t.Fatalf("synthetic result = %#v", toolResult)
	}
	if messages[3].Role != "user" {
		t.Fatalf("follow-up user missing: %#v", messages)
	}
}

func TestUsageTotalTokensEqualsComponentsAcrossProviders(t *testing.T) {
	model := Model{ID: "metered", Cost: ModelCost{Input: 1, Output: 2, CacheRead: 0.5, CacheWrite: 0.25}}
	cases := []struct {
		name  string
		usage Usage
	}{
		{
			name: "anthropic",
			usage: usageFromAnthropicRaw(AnthropicRawUsage{
				InputTokens:              10,
				OutputTokens:             4,
				CacheReadInputTokens:     3,
				CacheCreationInputTokens: 2,
			}, model),
		},
		{
			name: "openai-completions",
			usage: ParseOpenAIChatUsage(OpenAIChatUsage{
				PromptTokens:     15,
				CompletionTokens: 7,
				PromptTokensDetails: OpenAIChatPromptTokenDetails{
					CachedTokens:     4,
					CacheWriteTokens: 3,
				},
			}, model),
		},
		{
			name: "openai-responses",
			usage: ParseOpenAIResponsesUsage(OpenAIResponsesUsage{
				InputTokens:  20,
				OutputTokens: 8,
				InputTokensDetails: OpenAIResponsesInputTokenDetails{
					CachedTokens: 5,
				},
			}, model),
		},
		{
			name: "openrouter-images",
			usage: ParseOpenRouterImagesUsage(OpenRouterImagesUsage{
				PromptTokens:     12,
				CompletionTokens: 6,
				PromptTokensDetails: OpenRouterPromptTokensDetails{
					CachedTokens:     4,
					CacheWriteTokens: 1,
				},
			}, ImagesModel{Cost: model.Cost}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			computed := tc.usage.Input + tc.usage.Output + tc.usage.CacheRead + tc.usage.CacheWrite
			if tc.usage.TotalTokens != computed {
				t.Fatalf("usage = %#v computed = %d", tc.usage, computed)
			}
			if tc.usage.Cost.Total <= 0 {
				t.Fatalf("cost = %#v", tc.usage.Cost)
			}
		})
	}
}
