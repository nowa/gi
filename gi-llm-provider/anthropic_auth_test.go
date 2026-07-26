package gillmprovider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAnthropicAmbientAuthReachesRequestShape(t *testing.T) {
	t.Run("uses bearer authorization without OAuth shaping", func(t *testing.T) {
		var captured *http.Request
		var payload AnthropicPayload
		models, model := newAnthropicAuthTestModels(
			t,
			map[string]string{
				AnthropicAuthTokenEnv:  "auth-token",
				AnthropicOAuthTokenEnv: "oauth-token",
				AnthropicAPIKeyEnv:     "api-key",
			},
			func(request *http.Request) {
				captured = request.Clone(request.Context())
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Errorf("decode payload: %v", err)
				}
			},
		)

		if _, err := models.CompleteSimple(
			context.Background(),
			model,
			Context{
				SystemPrompt: "System prompt.",
				Messages:     []Message{UserMessageText("Hello")},
			},
			ModelsStreamOptions{},
		); err != nil {
			t.Fatal(err)
		}

		if captured == nil {
			t.Fatal("Anthropic request was not captured")
		}
		if got := captured.Header.Get("Authorization"); got != "Bearer auth-token" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := captured.Header.Get("x-api-key"); got != "" {
			t.Fatalf("x-api-key = %q", got)
		}
		if got := captured.Header.Get("anthropic-beta"); strings.Contains(
			got,
			"oauth-2025-04-20",
		) {
			t.Fatalf("anthropic-beta = %q", got)
		}
		if len(payload.System) != 1 ||
			payload.System[0].Text != "System prompt." {
			t.Fatalf("system = %#v", payload.System)
		}
	})

	t.Run("preserves OAuth shaping for OAuth token", func(t *testing.T) {
		var captured *http.Request
		var payload AnthropicPayload
		models, model := newAnthropicAuthTestModels(
			t,
			map[string]string{
				AnthropicOAuthTokenEnv: "sk-ant-oat-test",
				AnthropicAPIKeyEnv:     "api-key",
			},
			func(request *http.Request) {
				captured = request.Clone(request.Context())
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Errorf("decode payload: %v", err)
				}
			},
		)

		if _, err := models.CompleteSimple(
			context.Background(),
			model,
			Context{
				SystemPrompt: "System prompt.",
				Messages:     []Message{UserMessageText("Hello")},
			},
			ModelsStreamOptions{},
		); err != nil {
			t.Fatal(err)
		}

		if captured == nil {
			t.Fatal("Anthropic request was not captured")
		}
		if got := captured.Header.Get("Authorization"); got != "Bearer sk-ant-oat-test" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := captured.Header.Get("anthropic-beta"); !strings.Contains(
			got,
			"oauth-2025-04-20",
		) {
			t.Fatalf("anthropic-beta = %q", got)
		}
		if len(payload.System) != 2 ||
			payload.System[0].Text !=
				"You are Claude Code, Anthropic's official CLI for Claude." ||
			payload.System[1].Text != "System prompt." {
			t.Fatalf("system = %#v", payload.System)
		}
	})

	t.Run("explicit request header overrides ambient bearer", func(t *testing.T) {
		var captured *http.Request
		models, model := newAnthropicAuthTestModels(
			t,
			map[string]string{AnthropicAuthTokenEnv: "auth-token"},
			func(request *http.Request) {
				captured = request.Clone(request.Context())
			},
		)

		if _, err := models.CompleteSimple(
			context.Background(),
			model,
			Context{Messages: []Message{UserMessageText("Hello")}},
			ModelsStreamOptions{StreamOptions: StreamOptions{
				Headers: map[string]string{
					"authorization": "Bearer explicit-token",
				},
			}},
		); err != nil {
			t.Fatal(err)
		}

		if captured == nil {
			t.Fatal("Anthropic request was not captured")
		}
		if got := captured.Header.Get("Authorization"); got !=
			"Bearer explicit-token" {
			t.Fatalf("Authorization = %q", got)
		}
	})
}

func newAnthropicAuthTestModels(
	t *testing.T,
	env map[string]string,
	capture func(*http.Request),
) (*Models, Model) {
	t.Helper()
	model := MustGetModel("anthropic", "claude-haiku-4-5")
	api := NewAnthropicMessagesProvider(anthropicAuthHTTPDoerFunc(
		func(request *http.Request) (*http.Response, error) {
			if capture != nil {
				capture(request)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream"},
				},
				Body: io.NopCloser(strings.NewReader(
					"event: message_stop\n" +
						"data: {\"type\":\"message_stop\"}\n\n",
				)),
				Request: request,
			}, nil
		},
	))
	provider, err := CreateProvider(CreateProviderOptions{
		ID:      "anthropic",
		Name:    "Anthropic",
		BaseURL: model.BaseURL,
		Auth: ProviderAuth{
			APIKey: anthropicAPIKeyAuth(),
		},
		Models: []Model{model},
		API:    api,
	})
	if err != nil {
		t.Fatal(err)
	}
	models := NewModels(ModelsOptions{
		AuthContext: providerAuthContext(env),
	})
	if err := models.SetProvider(provider); err != nil {
		t.Fatal(err)
	}
	return models, model
}

type anthropicAuthHTTPDoerFunc func(*http.Request) (*http.Response, error)

func (f anthropicAuthHTTPDoerFunc) Do(
	request *http.Request,
) (*http.Response, error) {
	return f(request)
}
