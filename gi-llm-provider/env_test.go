package gillmprovider

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestEnvironmentAPIKeysDoesNotTreatGenericGitHubTokensAsCopilotCredentials(t *testing.T) {
	t.Setenv("COPILOT_GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "gh-token")
	t.Setenv("GITHUB_TOKEN", "github-token")

	if got := FindEnvKeys("github-copilot"); got != nil {
		t.Fatalf("FindEnvKeys(github-copilot) = %#v, want nil", got)
	}
	if got := GetEnvAPIKey("github-copilot"); got != "" {
		t.Fatalf("GetEnvAPIKey(github-copilot) = %q, want empty", got)
	}
}

func TestEnvironmentAPIKeysResolvesCopilotToken(t *testing.T) {
	t.Setenv("COPILOT_GITHUB_TOKEN", "copilot-token")
	t.Setenv("GH_TOKEN", "gh-token")
	t.Setenv("GITHUB_TOKEN", "github-token")

	got := FindEnvKeys("github-copilot")
	if len(got) != 1 || got[0] != "COPILOT_GITHUB_TOKEN" {
		t.Fatalf("FindEnvKeys(github-copilot) = %#v, want COPILOT_GITHUB_TOKEN", got)
	}
	if got := GetEnvAPIKey("github-copilot"); got != "copilot-token" {
		t.Fatalf("GetEnvAPIKey(github-copilot) = %q, want copilot-token", got)
	}
}

func TestEnvironmentAPIKeysCoverPiV082Providers(t *testing.T) {
	cases := map[string]string{
		"ant-ling":           "ANT_LING_API_KEY",
		"nvidia":             "NVIDIA_API_KEY",
		"qwen-token-plan":    "QWEN_TOKEN_PLAN_API_KEY",
		"qwen-token-plan-cn": "QWEN_TOKEN_PLAN_CN_API_KEY",
		"radius":             "RADIUS_API_KEY",
		"zai-coding-cn":      "ZAI_CODING_CN_API_KEY",
	}
	for providerID, envVar := range cases {
		t.Run(providerID, func(t *testing.T) {
			t.Setenv(envVar, "test-key")
			if got := FindEnvKeys(providerID); len(got) != 1 || got[0] != envVar {
				t.Fatalf("FindEnvKeys(%s) = %#v, want %s", providerID, got, envVar)
			}
			if got := GetEnvAPIKey(providerID); got != "test-key" {
				t.Fatalf("GetEnvAPIKey(%s) = %q, want test-key", providerID, got)
			}
		})
	}
}

func TestEnvironmentAPIKeysSeparatesAnthropicBearerFromAPIKeys(t *testing.T) {
	t.Setenv(AnthropicAuthTokenEnv, "auth-token")
	t.Setenv(AnthropicOAuthTokenEnv, "oauth-token")
	t.Setenv(AnthropicAPIKeyEnv, "api-key")

	if got := FindEnvKeys("anthropic"); !reflect.DeepEqual(got, []string{
		AnthropicAuthTokenEnv,
		AnthropicOAuthTokenEnv,
		AnthropicAPIKeyEnv,
	}) {
		t.Fatalf("FindEnvKeys(anthropic) = %#v", got)
	}
	if got := GetEnvAPIKey("anthropic"); got != "oauth-token" {
		t.Fatalf("GetEnvAPIKey(anthropic) = %q, want OAuth token", got)
	}

	t.Setenv(AnthropicOAuthTokenEnv, "")
	t.Setenv(AnthropicAPIKeyEnv, "")
	if got := FindEnvKeys("anthropic"); !reflect.DeepEqual(
		got,
		[]string{AnthropicAuthTokenEnv},
	) {
		t.Fatalf("bearer-only FindEnvKeys(anthropic) = %#v", got)
	}
	if got := GetEnvAPIKey("anthropic"); got != "" {
		t.Fatalf("bearer-only GetEnvAPIKey(anthropic) = %q", got)
	}

	t.Setenv(AnthropicAuthTokenEnv, "")
	t.Setenv(AnthropicOAuthTokenEnv, "oauth-token")
	if got := GetEnvAPIKey("anthropic"); got != "oauth-token" {
		t.Fatalf("OAuth GetEnvAPIKey(anthropic) = %q", got)
	}

	t.Setenv(AnthropicOAuthTokenEnv, "")
	t.Setenv(AnthropicAPIKeyEnv, "api-key")
	if got := GetEnvAPIKey("anthropic"); got != "api-key" {
		t.Fatalf("API-key GetEnvAPIKey(anthropic) = %q", got)
	}
}

func TestProviderEnvironmentOverridesProcessEnvironment(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "process-key")
	t.Setenv("PI_CACHE_RETENTION", "short")
	t.Setenv("EMPTY_OVERRIDE", "process-fallback")
	env := ProviderEnv{
		"OPENAI_API_KEY":     "scoped-key",
		"PI_CACHE_RETENTION": "long",
	}

	if got := GetProviderEnvValue("OPENAI_API_KEY", env); got != "scoped-key" {
		t.Fatalf("provider env value = %q", got)
	}
	if got := GetEnvAPIKeyWithOverrides("openai", env); got != "scoped-key" {
		t.Fatalf("provider API key = %q", got)
	}
	if got := FindEnvKeysWithOverrides("openai", env); len(got) != 1 || got[0] != "OPENAI_API_KEY" {
		t.Fatalf("provider API key variables = %#v", got)
	}
	if got := apiKeyOrEnv("openai", "explicit-key", env); got != "explicit-key" {
		t.Fatalf("explicit API key = %q", got)
	}
	if got := GetProviderEnvValue("EMPTY_OVERRIDE", ProviderEnv{"EMPTY_OVERRIDE": ""}); got != "process-fallback" {
		t.Fatalf("empty override = %q", got)
	}

	var (
		authorization string
		payload       OpenAICompletionsPayload
	)
	client := simpleOptionsHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		authorization = request.Header.Get("authorization")
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"content-type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})
	model := Model{
		ID:            "gpt-test",
		API:           "openai-completions",
		Provider:      "openai",
		BaseURL:       "https://api.openai.com/v1",
		Input:         []string{"text"},
		ContextWindow: 128_000,
		MaxTokens:     4_096,
	}
	stream, err := NewOpenAICompletionsProvider(client).StreamSimple(
		model,
		Context{Messages: []Message{UserMessageText("hello")}},
		SimpleStreamOptions{
			Env: env,
			OnPayload: func(value any, _ Model) (any, bool, error) {
				payload = value.(OpenAICompletionsPayload)
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
	if authorization != "Bearer scoped-key" || payload.PromptCacheRetention != "24h" {
		t.Fatalf("authorization=%q payload=%#v", authorization, payload)
	}
}

func TestProviderEnvironmentRecognizesScopedBedrockCredentials(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")
	if got := GetEnvAPIKeyWithOverrides(
		"amazon-bedrock",
		ProviderEnv{"AWS_PROFILE": "work"},
	); got != "<authenticated>" {
		t.Fatalf("Bedrock API key = %q", got)
	}
}
