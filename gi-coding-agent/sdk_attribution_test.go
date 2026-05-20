package gicodingagent

import (
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestSDKAttributionAddsDefaultHeadersForOpenRouterModels(t *testing.T) {
	headers := BuildSDKStreamHeaders(testAttributionModel("openrouter", "https://openrouter.ai/api/v1"), true, nil, nil)
	assertOpenRouterAttributionHeaders(t, headers)
}

func TestSDKAttributionOmitsHeadersWhenTelemetryDisabled(t *testing.T) {
	headers := BuildSDKStreamHeaders(testAttributionModel("openrouter", "https://openrouter.ai/api/v1"), false, nil, nil)
	for _, key := range []string{"HTTP-Referer", "X-OpenRouter-Title", "X-OpenRouter-Categories"} {
		if headers[key] != "" {
			t.Fatalf("%s = %q, want empty", key, headers[key])
		}
	}
}

func TestSDKAttributionAddsHeadersForCustomOpenRouterProvider(t *testing.T) {
	headers := BuildSDKStreamHeaders(testAttributionModel("custom-openrouter", "https://openrouter.ai/api/v1"), true, nil, nil)
	assertOpenRouterAttributionHeaders(t, headers)
}

func TestSDKAttributionProviderAndRequestHeadersOverrideDefaults(t *testing.T) {
	headers := BuildSDKStreamHeaders(
		testAttributionModel("openrouter", "https://openrouter.ai/api/v1"),
		true,
		map[string]string{
			"HTTP-Referer":            "https://provider.example",
			"X-OpenRouter-Categories": "provider-category",
		},
		map[string]string{
			"X-OpenRouter-Title": "request-title",
		},
	)

	if headers["HTTP-Referer"] != "https://provider.example" {
		t.Fatalf("HTTP-Referer = %q", headers["HTTP-Referer"])
	}
	if headers["X-OpenRouter-Title"] != "request-title" {
		t.Fatalf("X-OpenRouter-Title = %q", headers["X-OpenRouter-Title"])
	}
	if headers["X-OpenRouter-Categories"] != "provider-category" {
		t.Fatalf("X-OpenRouter-Categories = %q", headers["X-OpenRouter-Categories"])
	}
}

func testAttributionModel(provider, baseURL string) llm.Model {
	return llm.Model{
		ID:            provider + "-test-model",
		Name:          provider + " Test Model",
		API:           "openai-completions",
		Provider:      provider,
		BaseURL:       baseURL,
		Input:         []string{"text"},
		ContextWindow: 128000,
		MaxTokens:     4096,
	}
}

func assertOpenRouterAttributionHeaders(t *testing.T, headers map[string]string) {
	t.Helper()
	if headers["HTTP-Referer"] != "https://pi.dev" {
		t.Fatalf("HTTP-Referer = %q", headers["HTTP-Referer"])
	}
	if headers["X-OpenRouter-Title"] != "pi" {
		t.Fatalf("X-OpenRouter-Title = %q", headers["X-OpenRouter-Title"])
	}
	if headers["X-OpenRouter-Categories"] != "cli-agent" {
		t.Fatalf("X-OpenRouter-Categories = %q", headers["X-OpenRouter-Categories"])
	}
}
