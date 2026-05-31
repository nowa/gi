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
		{name: "opus 4.6 xhigh", provider: "anthropic", modelID: "claude-opus-4-6", contains: "xhigh"},
		{name: "opus 4.7 xhigh", provider: "anthropic", modelID: "claude-opus-4-7", contains: "xhigh"},
		{name: "sonnet no xhigh", provider: "anthropic", modelID: "claude-sonnet-4-5", not: "xhigh"},
		{name: "gpt 5.4 xhigh", provider: "openai-codex", modelID: "gpt-5.4", contains: "xhigh"},
		{name: "gpt 5.5 xhigh", provider: "openai-codex", modelID: "gpt-5.5", contains: "xhigh"},
		{name: "deepseek exact", provider: "deepseek", modelID: "deepseek-v4-flash", exact: []string{"off", "high", "xhigh"}},
		{name: "opencode-go exact", provider: "opencode-go", modelID: "deepseek-v4-flash", exact: []string{"off", "high", "xhigh"}},
		{name: "openrouter deepseek exact", provider: "openrouter", modelID: "deepseek/deepseek-v4-flash", exact: []string{"off", "high", "xhigh"}},
		{name: "openrouter opus xhigh", provider: "openrouter", modelID: "anthropic/claude-opus-4.6", contains: "xhigh"},
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
	t.Run("includes xhigh for Anthropic Opus 4.6 on anthropic-messages API", func(t *testing.T) {
		model := MustGetModel("anthropic", "claude-opus-4-6")
		if !contains(GetSupportedThinkingLevels(model), "xhigh") {
			t.Fatalf("levels = %#v", GetSupportedThinkingLevels(model))
		}
	})

	t.Run("includes xhigh for Anthropic Opus 4.7 on anthropic-messages API", func(t *testing.T) {
		model := MustGetModel("anthropic", "claude-opus-4-7")
		if !contains(GetSupportedThinkingLevels(model), "xhigh") {
			t.Fatalf("levels = %#v", GetSupportedThinkingLevels(model))
		}
	})

	t.Run("does not include xhigh for non-Opus Anthropic models", func(t *testing.T) {
		model := MustGetModel("anthropic", "claude-sonnet-4-5")
		if contains(GetSupportedThinkingLevels(model), "xhigh") {
			t.Fatalf("levels = %#v", GetSupportedThinkingLevels(model))
		}
	})

	t.Run("includes xhigh for %s models", func(t *testing.T) {
		for _, model := range []Model{
			MustGetModel("openai-codex", "gpt-5.4"),
			MustGetModel("openai-codex", "gpt-5.5"),
		} {
			if !contains(GetSupportedThinkingLevels(model), "xhigh") {
				t.Fatalf("%s levels = %#v", model.ID, GetSupportedThinkingLevels(model))
			}
		}
	})

	t.Run("includes xhigh for OpenRouter Opus 4.6 (openai-completions API)", func(t *testing.T) {
		model := MustGetModel("openrouter", "anthropic/claude-opus-4.6")
		if model.API != "openai-completions" || !contains(GetSupportedThinkingLevels(model), "xhigh") {
			t.Fatalf("model = %#v levels=%#v", model, GetSupportedThinkingLevels(model))
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
