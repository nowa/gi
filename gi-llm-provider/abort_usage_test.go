package gillmprovider

import "testing"

func TestExpectedAbortUsage(t *testing.T) {
	tests := []struct {
		name  string
		model Model
		want  AbortUsageExpectation
	}{
		{name: "openai completions reports usage only in final chunk", model: Model{Provider: "openai", API: "openai-completions"}, want: AbortUsageFinalOnly},
		{name: "openai responses reports usage only in final chunk", model: Model{Provider: "openai", API: "openai-responses"}, want: AbortUsageFinalOnly},
		{name: "codex responses reports usage only in final chunk", model: Model{Provider: "openai-codex", API: "openai-codex-responses"}, want: AbortUsageFinalOnly},
		{name: "bedrock reports usage only in final chunk", model: Model{Provider: "amazon-bedrock", API: "bedrock-converse-stream"}, want: AbortUsageFinalOnly},
		{name: "minimax omits usage on abort", model: Model{Provider: "minimax", API: "openai-completions"}, want: AbortUsageFinalOnly},
		{name: "kimi coding reports input before output", model: Model{Provider: "kimi-coding", API: "anthropic-messages"}, want: AbortUsageInputOnly},
		{name: "anthropic reports usage incrementally", model: Model{Provider: "anthropic", API: "anthropic-messages"}, want: AbortUsageIncremental},
		{name: "google reports usage incrementally", model: Model{Provider: "google", API: "google-generative-ai"}, want: AbortUsageIncremental},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExpectedAbortUsage(tc.model); got != tc.want {
				t.Fatalf("ExpectedAbortUsage() = %q, want %q", got, tc.want)
			}
		})
	}
}
