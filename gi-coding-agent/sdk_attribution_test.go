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

func TestSDKAttributionPreservesLegacyOpenRouterSubstringMatching(
	t *testing.T,
) {
	headers := BuildSDKStreamHeaders(
		testAttributionModel(
			"custom-openrouter",
			"not-a-url-openrouter.ai",
		),
		true,
		nil,
		nil,
	)
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

func TestSDKAttributionAddsNVIDIANIMHeaders(t *testing.T) {
	tests := []llm.Model{
		testAttributionModel(
			"custom-nim",
			"https://integrate.api.nvidia.com/v1",
		),
		testAttributionModel(
			"nvidia",
			"https://example.test/v1",
		),
	}
	for _, model := range tests {
		t.Run(model.Provider, func(t *testing.T) {
			headers := BuildSDKStreamHeaders(model, true, nil, nil)
			if headers["X-BILLING-INVOKE-ORIGIN"] != "Gi" {
				t.Fatalf(
					"X-BILLING-INVOKE-ORIGIN = %q",
					headers["X-BILLING-INVOKE-ORIGIN"],
				)
			}
		})
	}
}

func TestSDKAttributionNVIDIANIMPolicy(t *testing.T) {
	t.Run("disabled with telemetry", func(t *testing.T) {
		headers := BuildSDKStreamHeaders(
			testAttributionModel(
				"nvidia",
				"https://integrate.api.nvidia.com/v1",
			),
			false,
			nil,
			nil,
		)
		if headers["X-BILLING-INVOKE-ORIGIN"] != "" {
			t.Fatalf("headers = %#v", headers)
		}
	})

	t.Run("request overrides provider and default", func(t *testing.T) {
		headers := BuildSDKStreamHeaders(
			testAttributionModel(
				"nvidia",
				"https://integrate.api.nvidia.com/v1",
			),
			true,
			map[string]string{
				"X-BILLING-INVOKE-ORIGIN": "Provider",
			},
			map[string]string{
				"X-BILLING-INVOKE-ORIGIN": "Request",
			},
		)
		if headers["X-BILLING-INVOKE-ORIGIN"] != "Request" {
			t.Fatalf("headers = %#v", headers)
		}
	})

	t.Run("does not infer from routed model id", func(t *testing.T) {
		model := testAttributionModel(
			"openrouter",
			"https://openrouter.ai/api/v1",
		)
		model.ID = "nvidia/nemotron-3-super-120b-a12b"
		headers := BuildSDKStreamHeaders(model, true, nil, nil)
		if headers["X-BILLING-INVOKE-ORIGIN"] != "" {
			t.Fatalf("headers = %#v", headers)
		}
		assertOpenRouterAttributionHeaders(t, headers)
	})

	t.Run("requires exact endpoint host", func(t *testing.T) {
		headers := BuildSDKStreamHeaders(
			testAttributionModel(
				"custom-nim",
				"https://integrate.api.nvidia.com.example/v1",
			),
			true,
			nil,
			nil,
		)
		if headers != nil {
			t.Fatalf("headers = %#v, want nil", headers)
		}
	})
}

func TestSDKAttributionAddsOpenCodeSessionHeaders(t *testing.T) {
	headers := MergeProviderAttributionHeaders(
		testAttributionModel(
			"opencode",
			"https://opencode.ai/zen/v1",
		),
		ProviderAttributionContext{
			InstallTelemetryEnabled: true,
			SessionID:               "opencode-session",
		},
	)
	if headers["x-opencode-session"] != "opencode-session" ||
		headers["x-opencode-client"] != "gi" {
		t.Fatalf("headers = %#v", headers)
	}
}

func TestSDKAttributionConfiguredHeadersOverrideOpenCodeSession(
	t *testing.T,
) {
	headers := MergeProviderAttributionHeaders(
		testAttributionModel(
			"custom-opencode",
			"https://opencode.ai/zen/v1",
		),
		ProviderAttributionContext{
			InstallTelemetryEnabled: true,
			SessionID:               "opencode-session",
		},
		map[string]string{
			"x-opencode-session": "configured-session",
			"x-opencode-client":  "configured-client",
		},
	)
	if headers["x-opencode-session"] != "configured-session" ||
		headers["x-opencode-client"] != "configured-client" {
		t.Fatalf("headers = %#v", headers)
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
	if headers["HTTP-Referer"] != "https://github.com/nowa/gi" {
		t.Fatalf("HTTP-Referer = %q", headers["HTTP-Referer"])
	}
	if headers["X-OpenRouter-Title"] != "gi" {
		t.Fatalf("X-OpenRouter-Title = %q", headers["X-OpenRouter-Title"])
	}
	if headers["X-OpenRouter-Categories"] != "cli-agent" {
		t.Fatalf("X-OpenRouter-Categories = %q", headers["X-OpenRouter-Categories"])
	}
}
