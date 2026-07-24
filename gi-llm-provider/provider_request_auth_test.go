package gillmprovider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestResolveProviderRequestAuth(t *testing.T) {
	t.Run("prefers an explicit API key", func(t *testing.T) {
		auth := resolveProviderRequestAuth(
			"custom-provider",
			"explicit-key",
			ProviderEnv{"CUSTOM_API_KEY": "scoped-key"},
			map[string]string{"Authorization": "Bearer header-token"},
			"authorization",
		)
		if !auth.Configured() || auth.APIKey != "explicit-key" || auth.HeaderOnly {
			t.Fatalf("auth = %#v", auth)
		}
	})

	t.Run("accepts non-blank headers case-insensitively", func(t *testing.T) {
		auth := resolveProviderRequestAuth(
			"custom-provider",
			"",
			nil,
			map[string]string{
				"cf-aig-authorization": "",
				"CF-AIG-Authorization": "Bearer gateway-token",
			},
			"authorization",
			"cf-aig-authorization",
		)
		if !auth.Configured() || auth.APIKey != "" || !auth.HeaderOnly {
			t.Fatalf("auth = %#v", auth)
		}
	})

	t.Run("rejects blank authentication headers", func(t *testing.T) {
		auth := resolveProviderRequestAuth(
			"custom-provider",
			"",
			nil,
			map[string]string{
				"Authorization":        " \t",
				"CF-AIG-Authorization": "",
			},
			"authorization",
			"cf-aig-authorization",
		)
		if auth.Configured() {
			t.Fatalf("auth = %#v", auth)
		}
	})
}

func TestHTTPProvidersAcceptCallerManagedAuthenticationHeaders(t *testing.T) {
	tests := []struct {
		name       string
		model      Model
		headers    map[string]string
		wantHeader string
		stream     func(HTTPDoer, Model, StreamOptions) (*AssistantMessageEventStream, error)
	}{
		{
			name: "Anthropic x-api-key",
			model: Model{
				ID:       "claude-proxy",
				API:      "anthropic-messages",
				Provider: "custom-anthropic",
				BaseURL:  "https://anthropic-proxy.example/v1",
				Input:    []string{"text"},
			},
			headers:    map[string]string{"X-API-Key": "caller-key"},
			wantHeader: "X-API-Key",
			stream: func(client HTTPDoer, model Model, options StreamOptions) (*AssistantMessageEventStream, error) {
				return NewAnthropicMessagesProvider(client).Stream(
					model,
					Context{Messages: []Message{UserMessageText("hello")}},
					options,
				)
			},
		},
		{
			name: "OpenAI Completions authorization",
			model: Model{
				ID:       "chat-proxy",
				API:      "openai-completions",
				Provider: "custom-openai-completions",
				BaseURL:  "https://chat-proxy.example/v1",
				Input:    []string{"text"},
			},
			headers:    map[string]string{"Authorization": "Bearer caller-token"},
			wantHeader: "Authorization",
			stream: func(client HTTPDoer, model Model, options StreamOptions) (*AssistantMessageEventStream, error) {
				return NewOpenAICompletionsProvider(client).Stream(
					model,
					Context{Messages: []Message{UserMessageText("hello")}},
					options,
				)
			},
		},
		{
			name: "OpenAI Responses Cloudflare gateway authorization",
			model: Model{
				ID:       "responses-proxy",
				API:      "openai-responses",
				Provider: "custom-openai-responses",
				BaseURL:  "https://responses-proxy.example/v1",
				Input:    []string{"text"},
			},
			headers:    map[string]string{"CF-AIG-Authorization": "Bearer gateway-token"},
			wantHeader: "CF-AIG-Authorization",
			stream: func(client HTTPDoer, model Model, options StreamOptions) (*AssistantMessageEventStream, error) {
				return NewOpenAIResponsesProvider(client).Stream(
					model,
					Context{Messages: []Message{UserMessageText("hello")}},
					options,
				)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requestCount := 0
			capturedHeader := ""
			client := simpleOptionsHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
				requestCount++
				capturedHeader = request.Header.Get(tc.wantHeader)
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     http.Header{"content-type": []string{"text/event-stream"}},
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    request,
				}, nil
			})

			stream, err := tc.stream(client, tc.model, StreamOptions{
				Headers:   tc.headers,
				MaxTokens: 64,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := stream.Result(context.Background()); err != nil {
				t.Fatal(err)
			}
			if requestCount != 1 || capturedHeader != tc.headers[tc.wantHeader] {
				t.Fatalf("request count=%d header=%q", requestCount, capturedHeader)
			}
		})
	}
}

func TestHTTPProviderAPIKeyOverridesCallerAuthorizationCaseInsensitively(t *testing.T) {
	capturedAuthorization := ""
	client := simpleOptionsHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		capturedAuthorization = request.Header.Get("Authorization")
		if values := request.Header.Values("Authorization"); len(values) != 1 {
			t.Fatalf("Authorization values = %#v", values)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"content-type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})
	model := Model{
		ID:       "chat-proxy",
		API:      "openai-completions",
		Provider: "custom-openai-completions",
		BaseURL:  "https://chat-proxy.example/v1",
		Input:    []string{"text"},
	}
	stream, err := NewOpenAICompletionsProvider(client).Stream(
		model,
		Context{Messages: []Message{UserMessageText("hello")}},
		StreamOptions{
			APIKey:    "explicit-key",
			Headers:   map[string]string{"authorization": "Bearer caller-token"},
			MaxTokens: 64,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Result(context.Background()); err != nil {
		t.Fatal(err)
	}
	if capturedAuthorization != "Bearer explicit-key" {
		t.Fatalf("Authorization = %q", capturedAuthorization)
	}
}
