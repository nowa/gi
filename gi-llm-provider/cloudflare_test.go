package gillmprovider

import (
	"context"
	"io"
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

func TestCloudflareProviderStreamsPiCaseNames(t *testing.T) {
	model := Model{
		ID:            "model",
		Name:          "model",
		API:           "openai-completions",
		Provider:      "cloudflare-ai-gateway",
		BaseURL:       CloudflareAIGatewayOpenAIBaseURL,
		Input:         []string{"text"},
		ContextWindow: 1000,
		MaxTokens:     100,
	}
	t.Run("materializes the model endpoint before dispatch", func(t *testing.T) {
		var captured []string
		recorder := APIProviderFuncs{
			StreamFunc: func(
				requestModel Model,
				_ Context,
				_ StreamOptions,
			) (*AssistantMessageEventStream, error) {
				captured = append(captured, requestModel.BaseURL)
				return NewAssistantMessageEventStream(), nil
			},
			StreamSimpleFunc: func(
				requestModel Model,
				_ Context,
				_ SimpleStreamOptions,
			) (*AssistantMessageEventStream, error) {
				captured = append(captured, requestModel.BaseURL)
				return NewAssistantMessageEventStream(), nil
			},
		}
		provider := cloudflareAPIProvider{next: recorder}
		options := StreamOptions{Env: ProviderEnv{
			"CLOUDFLARE_ACCOUNT_ID": "account",
			"CLOUDFLARE_GATEWAY_ID": "gateway",
		}}
		if _, err := provider.Stream(model, Context{}, options); err != nil {
			t.Fatal(err)
		}
		if _, err := provider.StreamSimple(model, Context{}, options); err != nil {
			t.Fatal(err)
		}
		want := []string{
			"https://gateway.ai.cloudflare.com/v1/account/gateway/openai",
			"https://gateway.ai.cloudflare.com/v1/account/gateway/openai",
		}
		if len(captured) != len(want) || captured[0] != want[0] || captured[1] != want[1] {
			t.Fatalf("captured base URLs = %#v, want %#v", captured, want)
		}
	})

	t.Run("keeps placeholders when the provider env does not resolve them", func(t *testing.T) {
		var captured string
		provider := cloudflareAPIProvider{next: APIProviderFuncs{
			StreamSimpleFunc: func(
				requestModel Model,
				_ Context,
				_ SimpleStreamOptions,
			) (*AssistantMessageEventStream, error) {
				captured = requestModel.BaseURL
				return NewAssistantMessageEventStream(), nil
			},
		}}
		if _, err := provider.StreamSimple(model, Context{}, SimpleStreamOptions{}); err != nil {
			t.Fatal(err)
		}
		if captured != model.BaseURL {
			t.Fatalf("captured base URL = %q, want %q", captured, model.BaseURL)
		}
	})
}

func TestCloudflareGatewayHeaderRemovalsReachHTTPTransports(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_KEY", "")
	options := SimpleStreamOptions{
		Headers: map[string]string{
			"cf-aig-authorization": "Bearer gateway-token",
			"Authorization":        "Bearer must-be-removed",
			"x-api-key":            "must-be-removed",
		},
		HeaderRemovals: []string{"authorization", "X-API-KEY"},
	}
	llmContext := Context{Messages: []Message{UserMessageText("hi")}}
	tests := []struct {
		name     string
		api      string
		body     string
		provider APIProvider
	}{
		{
			name: "Anthropic",
			api:  "anthropic-messages",
			body: "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		},
		{
			name: "OpenAI Completions",
			api:  "openai-completions",
			body: "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n",
		},
		{
			name: "OpenAI Responses",
			api:  "openai-responses",
			body: "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &capturingCloudflareHTTPDoer{body: test.body}
			switch test.api {
			case "anthropic-messages":
				test.provider = NewAnthropicMessagesProvider(client)
			case "openai-completions":
				test.provider = NewOpenAICompletionsProvider(client)
			case "openai-responses":
				test.provider = NewOpenAIResponsesProvider(client)
			}
			model := Model{
				ID:            "model",
				Name:          "model",
				API:           test.api,
				Provider:      "cloudflare-ai-gateway",
				BaseURL:       "https://gateway.example.test/v1",
				Input:         []string{"text"},
				ContextWindow: 1000,
				MaxTokens:     100,
			}
			stream, err := test.provider.StreamSimple(model, llmContext, options)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := stream.Result(context.Background()); err != nil {
				t.Fatal(err)
			}
			if client.request == nil {
				t.Fatal("transport did not issue a request")
			}
			headers := client.request.Header
			if got := headers.Get("cf-aig-authorization"); got != "Bearer gateway-token" {
				t.Fatalf("gateway auth header = %q", got)
			}
			if got := headers.Get("Authorization"); got != "" {
				t.Fatalf("Authorization header = %q, want empty", got)
			}
			if got := headers.Get("x-api-key"); got != "" {
				t.Fatalf("x-api-key header = %q, want empty", got)
			}
		})
	}
}

func TestCloudflareGatewayAuthorizationCannotConfigureAnotherProvider(t *testing.T) {
	headers := map[string]string{
		"CF-AIG-Authorization": "Bearer gateway-token",
	}
	if hasCloudflareAIGatewayAuthorization(
		Model{Provider: "openai"},
		headers,
	) {
		t.Fatal("Cloudflare gateway header configured a non-Cloudflare provider")
	}
	if !hasCloudflareAIGatewayAuthorization(
		Model{Provider: "cloudflare-ai-gateway"},
		headers,
	) {
		t.Fatal("Cloudflare gateway header was not recognized case-insensitively")
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

type capturingCloudflareHTTPDoer struct {
	body    string
	request *http.Request
}

func (c *capturingCloudflareHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	c.request = request
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(c.body)),
		Request:    request,
	}, nil
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
