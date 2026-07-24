package gillmprovider

import (
	"strings"
	"testing"
)

func TestGetSupportedThinkingLevels(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		modelID  string
		contains string
		exact    []string
		not      string
	}{
		{name: "opus 4.6 max", provider: "anthropic", modelID: "claude-opus-4-6", contains: "max"},
		{name: "opus 4.7 xhigh", provider: "anthropic", modelID: "claude-opus-4-7", contains: "xhigh"},
		{name: "sonnet no xhigh", provider: "anthropic", modelID: "claude-sonnet-4-5", not: "xhigh"},
		{name: "gpt 5.4 xhigh", provider: "openai-codex", modelID: "gpt-5.4", contains: "xhigh"},
		{name: "gpt 5.5 xhigh", provider: "openai-codex", modelID: "gpt-5.5", contains: "xhigh"},
		{name: "deepseek exact", provider: "deepseek", modelID: "deepseek-v4-flash", exact: []string{"off", "high", "max"}},
		{name: "opencode-go exact", provider: "opencode-go", modelID: "deepseek-v4-flash", exact: []string{"off", "high", "max"}},
		{name: "openrouter deepseek exact", provider: "openrouter", modelID: "deepseek/deepseek-v4-flash", exact: []string{"off", "high", "xhigh"}},
		{name: "openrouter opus max", provider: "openrouter", modelID: "anthropic/claude-opus-4.6", contains: "max"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, ok := GetModel(tt.provider, tt.modelID)
			if !ok {
				t.Fatalf("GetModel(%q, %q) not found", tt.provider, tt.modelID)
			}
			levels := GetSupportedThinkingLevels(model)
			if tt.contains != "" && !contains(levels, tt.contains) {
				t.Fatalf("levels = %#v, want contains %q", levels, tt.contains)
			}
			if tt.not != "" && contains(levels, tt.not) {
				t.Fatalf("levels = %#v, want not contains %q", levels, tt.not)
			}
			if tt.exact != nil && !sameStrings(levels, tt.exact) {
				t.Fatalf("levels = %#v, want %#v", levels, tt.exact)
			}
		})
	}
}

func TestValidateThinkingLevelSupportedForXHigh(t *testing.T) {
	supported := Model{ID: "gpt-5.1-codex-max", Provider: "openai-codex", API: "openai-codex-responses", Reasoning: true, ThinkingLevelMap: map[string]*string{"xhigh": ptrString("xhigh")}}
	if err := ValidateThinkingLevelSupported(supported, "xhigh"); err != nil {
		t.Fatalf("supported xhigh error = %v", err)
	}
	if _, err := BuildOpenAIResponsesPayloadChecked(supported, Context{Messages: []Message{UserMessageText("hi")}}, OpenAIResponsesPayloadOptions{ReasoningEffort: "xhigh"}); err != nil {
		t.Fatalf("checked responses payload error = %v", err)
	}

	unsupported := Model{ID: "gpt-5-mini", Provider: "openai", API: "openai-responses", Reasoning: true, ThinkingLevelMap: map[string]*string{"off": nil}}
	err := ValidateThinkingLevelSupported(unsupported, "xhigh")
	if err == nil || !strings.Contains(err.Error(), "xhigh") {
		t.Fatalf("unsupported xhigh error = %v", err)
	}
	if _, err := BuildOpenAIResponsesPayloadChecked(unsupported, Context{Messages: []Message{UserMessageText("hi")}}, OpenAIResponsesPayloadOptions{ReasoningEffort: "xhigh"}); err == nil || !strings.Contains(err.Error(), "xhigh") {
		t.Fatalf("checked responses payload error = %v", err)
	}
	if _, err := BuildOpenAICompletionsPayloadChecked(unsupported, Context{Messages: []Message{UserMessageText("hi")}}, OpenAICompletionsPayloadOptions{Reasoning: "xhigh"}); err == nil || !strings.Contains(err.Error(), "xhigh") {
		t.Fatalf("checked completions payload error = %v", err)
	}
}

func TestSupportedThinkingLevelsPiCaseNames(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		modelID  string
		want     []string
	}{
		{"includes max but not xhigh for Anthropic Opus 4.6 on anthropic-messages API", "anthropic", "claude-opus-4-6", []string{"off", "minimal", "low", "medium", "high", "max"}},
		{"includes xhigh and max for Anthropic Opus 4.8 on anthropic-messages API", "anthropic", "claude-opus-4-8", []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}},
		{"includes max but not xhigh for Anthropic Sonnet 4.6 on anthropic-messages API", "anthropic", "claude-sonnet-4-6", []string{"off", "minimal", "low", "medium", "high", "max"}},
		{"includes xhigh and max for Anthropic Sonnet 5 on anthropic-messages API", "anthropic", "claude-sonnet-5", []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}},
		{"includes xhigh and max but not off for Anthropic Claude Fable 5 on anthropic-messages API", "anthropic", "claude-fable-5", []string{"minimal", "low", "medium", "high", "xhigh", "max"}},
		{"does not include xhigh or max for Claude Sonnet 4.5", "anthropic", "claude-sonnet-4-5", []string{"off", "minimal", "low", "medium", "high"}},
		{"includes only medium/high/xhigh for OpenAI GPT-5.5 Pro", "openai", "gpt-5.5-pro", []string{"medium", "high", "xhigh"}},
		{"includes only medium/high/xhigh for OpenRouter GPT-5.5 Pro", "openrouter", "openai/gpt-5.5-pro", []string{"medium", "high", "xhigh"}},
		{"includes only high/max plus off for DeepSeek V4 Flash on the DeepSeek provider", "deepseek", "deepseek-v4-flash", []string{"off", "high", "max"}},
		{"includes only high/max plus off for DeepSeek V4 Flash on opencode-go", "opencode-go", "deepseek-v4-flash", []string{"off", "high", "max"}},
		{"includes only high plus off for OpenCode Go Kimi K2.6", "opencode-go", "kimi-k2.6", []string{"off", "high"}},
		{"includes only low, high, max for Kimi Coding K3", "kimi-coding", "k3", []string{"low", "high", "max"}},
		{"includes only high for OpenCode Grok Build", "opencode", "grok-build-0.1", []string{"high"}},
		{"includes only high/xhigh plus off for DeepSeek V4 Flash on OpenRouter", "openrouter", "deepseek/deepseek-v4-flash", []string{"off", "high", "xhigh"}},
		{"includes max but not xhigh for OpenRouter Opus 4.6 (openai-completions API)", "openrouter", "anthropic/claude-opus-4.6", []string{"off", "minimal", "low", "medium", "high", "max"}},
		{"includes xhigh and max but not off for Bedrock Claude Fable 5", "amazon-bedrock", "global.anthropic.claude-fable-5", []string{"minimal", "low", "medium", "high", "xhigh", "max"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			assertModelThinkingLevels(t, test.provider, test.modelID, test.want)
		})
	}

	t.Run("includes xhigh for openai-codex %s models", func(t *testing.T) {
		for _, modelID := range []string{"gpt-5.4", "gpt-5.5", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
			if levels := GetSupportedThinkingLevels(MustGetModel("openai-codex", modelID)); !contains(levels, "xhigh") {
				t.Fatalf("%s levels = %#v", modelID, levels)
			}
		}
	})

	t.Run("includes xhigh and max for OpenAI %s models", func(t *testing.T) {
		for _, modelID := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
			assertModelThinkingLevels(
				t,
				"openai",
				modelID,
				[]string{"off", "low", "medium", "high", "xhigh", "max"},
			)
		}
	})

	t.Run("excludes thinking off for Moonshot Kimi K2.7 Code models", func(t *testing.T) {
		for _, provider := range []string{"moonshotai", "moonshotai-cn"} {
			assertModelThinkingLevels(
				t,
				provider,
				"kimi-k2.7-code",
				[]string{"minimal", "low", "medium", "high"},
			)
		}
	})

	t.Run("uses the verified effort options for %s Kimi K3", func(t *testing.T) {
		for _, provider := range []string{"moonshotai", "moonshotai-cn"} {
			assertModelThinkingLevels(t, provider, "kimi-k3", []string{"low", "high", "max"})
		}
	})
}

func TestMaxThinkingLevelPiCaseNames(t *testing.T) {
	t.Run("is opt-in for ordinary reasoning models", func(t *testing.T) {
		model := Model{
			ID:               "ordinary-reasoning",
			Provider:         "test",
			API:              "openai-completions",
			Reasoning:        true,
			ContextWindow:    128000,
			MaxTokens:        4096,
			ThinkingLevelMap: nil,
		}
		if levels := GetSupportedThinkingLevels(model); !sameStrings(
			levels,
			[]string{"off", "minimal", "low", "medium", "high"},
		) {
			t.Fatalf("levels = %#v", levels)
		}
		if got := ClampThinkingLevel(model, "max"); got != "high" {
			t.Fatalf("clamped max = %q", got)
		}
	})

	t.Run("exposes xhigh and max for openai-codex/%s", func(t *testing.T) {
		for _, modelID := range []string{"gpt-5.6-luna", "gpt-5.6-sol", "gpt-5.6-terra"} {
			model := MustGetModel("openai-codex", modelID)
			if mapped := model.ThinkingLevelMap["xhigh"]; mapped == nil || *mapped != "xhigh" {
				t.Fatalf("%s xhigh mapping = %#v", modelID, mapped)
			}
			if mapped := model.ThinkingLevelMap["max"]; mapped == nil || *mapped != "max" {
				t.Fatalf("%s max mapping = %#v", modelID, mapped)
			}
		}
	})

	t.Run("supports a hole between high and max", func(t *testing.T) {
		model := Model{
			ID:        "high-and-max",
			Provider:  "test",
			API:       "openai-completions",
			Reasoning: true,
			ThinkingLevelMap: map[string]*string{
				"xhigh": nil,
				"max":   ptrString("max"),
			},
		}
		if levels := GetSupportedThinkingLevels(model); !sameStrings(
			levels,
			[]string{"off", "minimal", "low", "medium", "high", "max"},
		) {
			t.Fatalf("levels = %#v", levels)
		}
		if got := ClampThinkingLevel(model, "xhigh"); got != "max" {
			t.Fatalf("clamped xhigh = %q", got)
		}
	})

	t.Run("sends max to the Codex Responses API", func(t *testing.T) {
		model := MustGetModel("openai-codex", "gpt-5.6-sol")
		payload := BuildOpenAICodexResponsesPayload(
			model,
			Context{Messages: []Message{UserMessageText("Hello")}},
			OpenAICodexResponsesPayloadOptions{ReasoningEffort: "max"},
		)
		if payload.Reasoning["effort"] != "max" || payload.Reasoning["summary"] != "auto" {
			t.Fatalf("reasoning = %#v", payload.Reasoning)
		}
	})
}

func TestXHighPiCaseNames(t *testing.T) {
	t.Run("should work with openai-responses", func(t *testing.T) {
		model := Model{ID: "gpt-5.1-codex-max", Provider: "openai-codex", API: "openai-codex-responses", Reasoning: true, ThinkingLevelMap: map[string]*string{"xhigh": ptrString("xhigh")}}
		if _, err := BuildOpenAIResponsesPayloadChecked(model, Context{Messages: []Message{UserMessageText("hi")}}, OpenAIResponsesPayloadOptions{ReasoningEffort: "xhigh"}); err != nil {
			t.Fatalf("checked responses payload error = %v", err)
		}
	})

	t.Run("should error with openai-responses when using xhigh", func(t *testing.T) {
		model := Model{ID: "gpt-5-mini", Provider: "openai", API: "openai-responses", Reasoning: true, ThinkingLevelMap: map[string]*string{"off": nil}}
		if _, err := BuildOpenAIResponsesPayloadChecked(model, Context{Messages: []Message{UserMessageText("hi")}}, OpenAIResponsesPayloadOptions{ReasoningEffort: "xhigh"}); err == nil || !strings.Contains(err.Error(), "xhigh") {
			t.Fatalf("checked responses payload error = %v", err)
		}
	})

	t.Run("should error with openai-completions when using xhigh", func(t *testing.T) {
		model := Model{ID: "gpt-5-mini", Provider: "openai", API: "openai-completions", Reasoning: true, ThinkingLevelMap: map[string]*string{"off": nil}}
		if _, err := BuildOpenAICompletionsPayloadChecked(model, Context{Messages: []Message{UserMessageText("hi")}}, OpenAICompletionsPayloadOptions{Reasoning: "xhigh"}); err == nil || !strings.Contains(err.Error(), "xhigh") {
			t.Fatalf("checked completions payload error = %v", err)
		}
	})
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func assertModelThinkingLevels(t *testing.T, provider, modelID string, want []string) {
	t.Helper()
	model, ok := GetModel(provider, modelID)
	if !ok {
		t.Fatalf("GetModel(%q, %q) not found", provider, modelID)
	}
	if got := GetSupportedThinkingLevels(model); !sameStrings(got, want) {
		t.Fatalf("%s/%s levels = %#v, want %#v", provider, modelID, got, want)
	}
}
