package gillmprovider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type simpleOptionsHTTPDoerFunc func(*http.Request) (*http.Response, error)

func (do simpleOptionsHTTPDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return do(request)
}

func TestClampMaxTokensToContext(t *testing.T) {
	contextWithTwoThousandTokens := Context{
		Messages: []Message{UserMessageText(strings.Repeat("x", 8000))},
	}
	tests := []struct {
		name      string
		model     Model
		context   Context
		maxTokens int
		want      int
	}{
		{
			name:      "keeps explicit cap with room",
			model:     Model{ContextWindow: 20_000},
			context:   contextWithTwoThousandTokens,
			maxTokens: 3_000,
			want:      3_000,
		},
		{
			name:      "caps at remaining context",
			model:     Model{ContextWindow: 10_000},
			context:   contextWithTwoThousandTokens,
			maxTokens: 8_000,
			want:      3_904,
		},
		{
			name:      "retains one token when context is exhausted",
			model:     Model{ContextWindow: 4_096},
			context:   contextWithTwoThousandTokens,
			maxTokens: 8_000,
			want:      1,
		},
		{
			name:      "unknown context window still enforces minimum",
			model:     Model{},
			maxTokens: -1,
			want:      1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ClampMaxTokensToContext(test.model, test.context, test.maxTokens); got != test.want {
				t.Fatalf("ClampMaxTokensToContext() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestPrepareSimpleStreamOptionsUsesModelDefaultAndOwnsMutableState(t *testing.T) {
	model := Model{ContextWindow: 10_000, MaxTokens: 8_000}
	contextValue := Context{
		Messages: []Message{UserMessageText(strings.Repeat("x", 8000))},
	}
	options := SimpleStreamOptions{
		Headers:         map[string]string{"x-test": "original"},
		HeaderRemovals:  []string{"x-remove"},
		ThinkingBudgets: map[string]int{"high": 16_384},
		Env:             ProviderEnv{"OPENAI_API_KEY": "original"},
		Metadata:        map[string]any{"nested": map[string]any{"value": "original"}},
	}

	prepared := prepareSimpleStreamOptions(model, contextValue, options)
	if prepared.MaxTokens != 3_904 {
		t.Fatalf("prepared max tokens = %d, want 3904", prepared.MaxTokens)
	}

	options.Headers["x-test"] = "mutated"
	options.HeaderRemovals[0] = "mutated"
	options.ThinkingBudgets["high"] = 1
	options.Env["OPENAI_API_KEY"] = "mutated"
	options.Metadata["nested"].(map[string]any)["value"] = "mutated"

	if prepared.Headers["x-test"] != "original" ||
		prepared.HeaderRemovals[0] != "x-remove" ||
		prepared.ThinkingBudgets["high"] != 16_384 ||
		prepared.Env["OPENAI_API_KEY"] != "original" ||
		prepared.Metadata["nested"].(map[string]any)["value"] != "original" {
		t.Fatalf("prepared options retain caller-owned state: %#v", prepared)
	}
}

func TestAdjustMaxTokensForThinkingStaysWithinContext(t *testing.T) {
	model := Model{ContextWindow: 10_000, MaxTokens: 8_000}
	contextValue := Context{
		Messages: []Message{UserMessageText(strings.Repeat("x", 8000))},
	}
	allocation := AdjustMaxTokensForThinking(3_904, model.MaxTokens, "high", nil)
	if allocation.MaxTokens != 8_000 || allocation.ThinkingBudget != 6_976 {
		t.Fatalf("model allocation = %#v", allocation)
	}
	allocation = clampThinkingTokenAllocationToContext(model, contextValue, allocation)
	if allocation.MaxTokens != 3_904 || allocation.ThinkingBudget != 2_880 {
		t.Fatalf("context allocation = %#v", allocation)
	}

	custom := AdjustMaxTokensForThinking(2_000, 10_000, "max", map[string]int{"high": 3_000})
	if custom.MaxTokens != 5_000 || custom.ThinkingBudget != 3_000 {
		t.Fatalf("custom allocation = %#v", custom)
	}

	defaulted := AdjustMaxTokensForThinking(0, 8_000, "high", nil)
	if defaulted.MaxTokens != 8_000 || defaulted.ThinkingBudget != 6_976 {
		t.Fatalf("default allocation = %#v", defaulted)
	}
}

func TestOpenAICompletionsStreamSimpleClampsMaxTokensToRemainingContext(t *testing.T) {
	model := Model{
		ID:            "gpt-test",
		API:           "openai-completions",
		Provider:      "openai",
		BaseURL:       "https://example.test/v1",
		Input:         []string{"text"},
		ContextWindow: 10_000,
		MaxTokens:     8_000,
	}
	contextValue := Context{
		Messages: []Message{UserMessageText(strings.Repeat("x", 8000))},
	}
	client := simpleOptionsHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"content-type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})
	provider := NewOpenAICompletionsProvider(client)

	tests := []struct {
		name       string
		simple     bool
		maxTokens  int
		wantTokens int
	}{
		{name: "default simple budget", simple: true, wantTokens: 3_904},
		{name: "explicit simple budget", simple: true, maxTokens: 7_000, wantTokens: 3_904},
		{name: "prepared stream budget", maxTokens: 7_000, wantTokens: 7_000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotTokens := 0
			options := StreamOptions{
				APIKey:    "test-key",
				MaxTokens: test.maxTokens,
				OnPayload: func(value any, _ Model) (any, bool, error) {
					payload, ok := value.(OpenAICompletionsPayload)
					if !ok {
						t.Fatalf("payload type = %T", value)
					}
					gotTokens = max(payload.MaxTokens, payload.MaxCompletionTokens)
					return nil, false, nil
				},
			}
			var (
				stream *AssistantMessageEventStream
				err    error
			)
			if test.simple {
				stream, err = provider.StreamSimple(model, contextValue, options)
			} else {
				stream, err = provider.Stream(model, contextValue, options)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := stream.Result(context.Background()); err != nil {
				t.Fatal(err)
			}
			if gotTokens != test.wantTokens {
				t.Fatalf("max tokens = %d, want %d", gotTokens, test.wantTokens)
			}
		})
	}
}

func TestAnthropicStreamSimpleKeepsThinkingBudgetWithinRemainingContext(t *testing.T) {
	model := Model{
		ID:            "claude-sonnet-4-5",
		API:           "anthropic-messages",
		Provider:      "anthropic",
		BaseURL:       "https://example.test",
		Reasoning:     true,
		Input:         []string{"text"},
		ContextWindow: 10_000,
		MaxTokens:     8_000,
	}
	contextValue := Context{
		Messages: []Message{UserMessageText(strings.Repeat("x", 8000))},
	}
	client := simpleOptionsHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"content-type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})
	var payload AnthropicPayload
	stream, err := NewAnthropicMessagesProvider(client).StreamSimple(
		model,
		contextValue,
		SimpleStreamOptions{
			APIKey:    "test-key",
			Reasoning: "high",
			OnPayload: func(value any, _ Model) (any, bool, error) {
				payload = value.(AnthropicPayload)
				return nil, false, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Result(context.Background()); err != nil {
		t.Fatal(err)
	}
	if payload.MaxTokens != 3_904 || payload.Thinking["budget_tokens"] != 2_880 {
		t.Fatalf("payload max=%d thinking=%#v", payload.MaxTokens, payload.Thinking)
	}
}
