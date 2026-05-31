package gillmprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveCloudflareBaseURLSubstitutesEnvironment(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "acct")
	t.Setenv("CLOUDFLARE_GATEWAY_ID", "gateway")

	model := Model{
		Provider: "cloudflare-ai-gateway",
		BaseURL:  CloudflareAIGatewayOpenAIBaseURL,
	}
	got, err := ResolveCloudflareBaseURL(model)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://gateway.ai.cloudflare.com/v1/acct/gateway/openai"
	if got != want {
		t.Fatalf("base URL = %q, want %q", got, want)
	}
}

func TestResolveCloudflareBaseURLReportsMissingEnvironment(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "")
	t.Setenv("CLOUDFLARE_GATEWAY_ID", "")

	model := Model{
		Provider: "cloudflare-workers-ai",
		BaseURL:  CloudflareWorkersAIBaseURL,
	}
	_, err := ResolveCloudflareBaseURL(model)
	if err == nil || !strings.Contains(err.Error(), "CLOUDFLARE_ACCOUNT_ID is required for provider cloudflare-workers-ai") {
		t.Fatalf("error = %v", err)
	}
}

func TestOpenAICompletionsProviderResolvesCloudflareBaseURL(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "acct")
	t.Setenv("CLOUDFLARE_GATEWAY_ID", "gateway")

	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"id":"cf-chat","choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	model := Model{
		ID:       "workers-ai/@cf/test",
		Provider: "cloudflare-ai-gateway",
		API:      "openai-completions",
		BaseURL:  server.URL + "/v1/{CLOUDFLARE_ACCOUNT_ID}/{CLOUDFLARE_GATEWAY_ID}/openai",
		Input:    []string{"text"},
	}
	stream, err := NewOpenAICompletionsProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{APIKey: "cf-key"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if requestPath != "/v1/acct/gateway/openai/chat/completions" || result.Content[0].Text != "ok" {
		t.Fatalf("request path/result = %q %#v", requestPath, result)
	}
}

func TestOpenAIResponsesProviderResolvesCloudflareBaseURL(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "acct")
	t.Setenv("CLOUDFLARE_GATEWAY_ID", "gateway")

	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"type":"response.completed","response":{"id":"cf-resp","status":"completed","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)
	}))
	defer server.Close()

	model := Model{
		ID:       "gpt-test",
		Provider: "cloudflare-ai-gateway",
		API:      "openai-responses",
		BaseURL:  server.URL + "/v1/{CLOUDFLARE_ACCOUNT_ID}/{CLOUDFLARE_GATEWAY_ID}/openai",
		Input:    []string{"text"},
	}
	stream, err := NewOpenAIResponsesProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{APIKey: "cf-key"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if requestPath != "/v1/acct/gateway/openai/responses" || result.ResponseID != "cf-resp" {
		t.Fatalf("request path/result = %q %#v", requestPath, result)
	}
}

func TestAnthropicProviderResolvesCloudflareBaseURL(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "acct")
	t.Setenv("CLOUDFLARE_GATEWAY_ID", "gateway")

	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.Header().Set("content-type", "text/event-stream")
		writeNamedSSE(t, w, "message_stop", `{"type":"message_stop"}`)
	}))
	defer server.Close()

	model := Model{
		ID:       "claude-test",
		Provider: "cloudflare-ai-gateway",
		API:      "anthropic-messages",
		BaseURL:  server.URL + "/v1/{CLOUDFLARE_ACCOUNT_ID}/{CLOUDFLARE_GATEWAY_ID}/anthropic",
		Input:    []string{"text"},
	}
	stream, err := NewAnthropicMessagesProvider(server.Client()).StreamSimple(model, Context{Messages: []Message{UserMessageText("hi")}}, SimpleStreamOptions{APIKey: "cf-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Result(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requestPath != "/v1/acct/gateway/anthropic/messages" {
		t.Fatalf("request path = %q", requestPath)
	}
}
