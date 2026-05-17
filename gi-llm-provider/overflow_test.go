package gillmprovider

import "testing"

func errorMessage(errorMessage string) Message {
	return Message{
		Role:         RoleAssistant,
		API:          "openai-completions",
		Provider:     "ollama",
		Model:        "qwen3.5:35b",
		Usage:        EmptyUsage(),
		StopReason:   StopReasonError,
		ErrorMessage: errorMessage,
		Timestamp:    NowMillis(),
	}
}

func TestIsContextOverflowErrorPatterns(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{"ollama prompt too long", "400 `prompt too long; exceeded max context length by 100918 tokens`", true},
		{"anthropic prompt too long", "prompt is too long: 231000 tokens > 200000 maximum", true},
		{"github copilot exceeds limit", "This model's maximum context length is 128000 tokens. Your input exceeds the limit of 128000", true},
		{"openai maximum context length", "This model's maximum context length is 128000 tokens. However, your messages resulted in 138000 tokens.", true},
		{"openai responses context window", "Input exceeds the context window for this model", true},
		{"google input token count", "input token count (1050000) exceeds the maximum number of tokens allowed", true},
		{"bedrock input too long", "Input is too long for requested model", true},
		{"together context length", "400 The input (516368 tokens) is longer than the model's context length (262144 tokens).", true},
		{"litellm openai wrapped", "Requested token count exceeds the model's maximum context length of 131072 tokens.", true},
		{"generic ollama crash", "500 `model runner crashed unexpectedly`", false},
		{"bedrock throttling too many tokens", "Throttling error: Too many tokens, please wait before trying again.", false},
		{"service unavailable", "Service unavailable: The service is temporarily unavailable.", false},
		{"rate limit", "Rate limit exceeded, please retry after 30 seconds.", false},
		{"http 429", "Too many requests. Please slow down.", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsContextOverflow(errorMessage(tt.message), 200000); got != tt.want {
				t.Fatalf("IsContextOverflow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsContextOverflowLengthStopSignals(t *testing.T) {
	message := Message{
		Role:       RoleAssistant,
		Usage:      Usage{Input: 58, CacheRead: 1048512, Output: 0},
		StopReason: StopReasonLength,
	}
	if !IsContextOverflow(message, 1048576) {
		t.Fatal("expected Xiaomi-style filled-context length stop to be overflow")
	}

	message.Usage = Usage{Input: 1000, CacheRead: 0, Output: 4096}
	if IsContextOverflow(message, 200000) {
		t.Fatal("normal length stop with output should not be overflow")
	}

	message.Usage = Usage{Input: 100, CacheRead: 0, Output: 0}
	if IsContextOverflow(message, 200000) {
		t.Fatal("short length stop should not be overflow")
	}
}
