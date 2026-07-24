package gillmprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOpenAICompletionsProviderRetriesTransientHTTPFailure(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	var responses atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("retry-after-ms", "0")
			http.Error(w, "temporarily overloaded", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"id":"retried","choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	model := Model{
		ID:       "test-model",
		Provider: "openai",
		API:      "openai-completions",
		BaseURL:  server.URL,
		Input:    []string{"text"},
	}
	stream, err := NewOpenAICompletionsProvider(server.Client()).StreamSimple(model, Context{}, SimpleStreamOptions{
		APIKey:           "test-key",
		MaxRetries:       1,
		MaxRetryDelayMs:  1000,
		OnResponseStatus: func(int, map[string]string, Model) error { responses.Add(1); return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 || responses.Load() != 2 {
		t.Fatalf("attempts=%d response callbacks=%d, want 2 each", attempts.Load(), responses.Load())
	}
	if result.StopReason != StopReasonStop || len(result.Content) != 1 || result.Content[0].Text != "ok" {
		t.Fatalf("result = %#v", result)
	}
}

func TestOpenAICompletionsProviderHonorsRetryHeaderAndSurfacesBody(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.Header().Set("x-should-retry", "false")
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"account limit reached"}`))
	}))
	defer server.Close()

	model := Model{
		ID:       "test-model",
		Provider: "openai",
		API:      "openai-completions",
		BaseURL:  server.URL,
		Input:    []string{"text"},
	}
	stream, err := NewOpenAICompletionsProvider(server.Client()).StreamSimple(model, Context{}, SimpleStreamOptions{
		APIKey:          "test-key",
		MaxRetries:      2,
		MaxRetryDelayMs: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", attempts.Load())
	}
	if result.StopReason != StopReasonError ||
		!strings.Contains(result.ErrorMessage, "429") ||
		!strings.Contains(result.ErrorMessage, "account limit reached") {
		t.Fatalf("result = %#v", result)
	}
}
