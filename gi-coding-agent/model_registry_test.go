package gicodingagent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestModelRegistryBaseURLOverridesAndCustomModelMerge(t *testing.T) {
	tempDir := t.TempDir()
	modelsPath := filepath.Join(tempDir, "models.json")
	auth := NewInMemoryAuthStorage(nil)

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"anthropic": map[string]any{"baseUrl": "https://my-proxy.example.com/v1"},
		},
	})
	registry := NewModelRegistry(auth, modelsPath)
	anthropicModels := registryModelsForProvider(registry, "anthropic")
	if len(anthropicModels) <= 1 || !registryHasModelIDContaining(anthropicModels, "claude") {
		t.Fatalf("anthropic models not preserved: %#v", anthropicModels)
	}
	for _, model := range anthropicModels {
		if model.BaseURL != "https://my-proxy.example.com/v1" {
			t.Fatalf("baseURL = %q", model.BaseURL)
		}
	}
	if len(registryModelsForProvider(registry, "google")) == 0 ||
		registryModelsForProvider(registry, "google")[0].BaseURL == "https://my-proxy.example.com/v1" {
		t.Fatalf("google provider should not inherit anthropic override")
	}

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"anthropic": map[string]any{
				"baseUrl": "https://my-proxy.example.com/v1",
				"headers": map[string]string{"X-Custom-Header": "custom-value"},
			},
		},
	})
	registry = NewModelRegistry(auth, modelsPath)
	for _, model := range registryModelsForProvider(registry, "anthropic") {
		authResult := registry.GetAPIKeyAndHeaders(model)
		if !authResult.OK || authResult.Headers["X-Custom-Header"] != "custom-value" {
			t.Fatalf("headers = %#v", authResult)
		}
	}

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"anthropic": map[string]any{"headers": map[string]string{"X-Custom-Header": "custom-value"}},
		},
	})
	registry = NewModelRegistry(auth, modelsPath)
	if registry.GetError() != "" {
		t.Fatalf("headers-only error = %s", registry.GetError())
	}

	writeModelsJSON(t, modelsPath, map[string]ProviderConfigInput{
		"anthropic": registryProviderConfig("https://anthropic-proxy.example.com/v1", []string{"claude-custom"}, "anthropic-messages"),
		"google":    registryProviderConfig("https://google-proxy.example.com/v1", []string{"gemini-custom"}, "google-generative-ai"),
	})
	registry = NewModelRegistry(auth, modelsPath)
	if !registryHasModelID(registryModelsForProvider(registry, "anthropic"), "claude-custom") ||
		!registryHasModelIDContaining(registryModelsForProvider(registry, "anthropic"), "claude") ||
		!registryHasModelID(registryModelsForProvider(registry, "google"), "gemini-custom") {
		t.Fatalf("mixed custom merge failed")
	}

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"anthropic": map[string]any{"baseUrl": "https://first-proxy.example.com/v1"},
		},
	})
	registry = NewModelRegistry(auth, modelsPath)
	if got := registryModelsForProvider(registry, "anthropic")[0].BaseURL; got != "https://first-proxy.example.com/v1" {
		t.Fatalf("first refresh baseURL = %q", got)
	}
	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"anthropic": map[string]any{"baseUrl": "https://second-proxy.example.com/v1"},
		},
	})
	registry.Refresh()
	if got := registryModelsForProvider(registry, "anthropic")[0].BaseURL; got != "https://second-proxy.example.com/v1" {
		t.Fatalf("second refresh baseURL = %q", got)
	}
}

func TestModelRegistryCustomModelsCompatAndOverrides(t *testing.T) {
	modelsPath := filepath.Join(t.TempDir(), "models.json")
	auth := NewInMemoryAuthStorage(nil)

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"openrouter": map[string]any{
				"models": []map[string]any{{"id": "fake-provider/fake-model", "name": "Fake model", "reasoning": true, "input": []string{"text"}}},
			},
		},
	})
	registry := NewModelRegistry(auth, modelsPath)
	model := registryMustFind(t, registry, "openrouter", "fake-provider/fake-model")
	if model.API != "openai-completions" || model.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("inherited model defaults = %#v", model)
	}

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"my-custom-provider": map[string]any{
				"models": []map[string]any{{"id": "my-model", "api": "openai-completions", "reasoning": false, "input": []string{"text"}}},
			},
		},
	})
	registry = NewModelRegistry(auth, modelsPath)
	if !strings.Contains(registry.GetError(), "baseUrl") {
		t.Fatalf("missing baseUrl error = %q", registry.GetError())
	}

	writeModelsJSON(t, modelsPath, map[string]ProviderConfigInput{
		"anthropic": registryProviderConfig("https://my-proxy.example.com/v1", []string{"claude-custom"}, "anthropic-messages"),
	})
	registry = NewModelRegistry(auth, modelsPath)
	if !registryHasModelID(registryModelsForProvider(registry, "anthropic"), "claude-custom") ||
		len(registryModelsForProvider(registry, "google")) == 0 ||
		len(registryModelsForProvider(registry, "openai")) == 0 {
		t.Fatalf("built-in merge affected other providers")
	}
	for _, model := range registryModelsForProvider(registry, "anthropic") {
		if model.BaseURL != "https://my-proxy.example.com/v1" {
			t.Fatalf("provider baseURL did not apply to %s: %q", model.ID, model.BaseURL)
		}
	}

	writeModelsJSON(t, modelsPath, map[string]ProviderConfigInput{
		"openrouter": registryProviderConfig("https://my-proxy.example.com/v1", []string{"anthropic/claude-sonnet-4"}, "openai-completions"),
	})
	registry = NewModelRegistry(auth, modelsPath)
	matches := registryModelsWithID(registryModelsForProvider(registry, "openrouter"), "anthropic/claude-sonnet-4")
	if len(matches) != 1 || matches[0].BaseURL != "https://my-proxy.example.com/v1" {
		t.Fatalf("custom model replacement = %#v", matches)
	}

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"demo": map[string]any{
				"baseUrl": "https://example.com/v1",
				"apiKey":  "DEMO_KEY",
				"api":     "openai-completions",
				"compat":  map[string]any{"supportsUsageInStreaming": false, "maxTokensField": "max_tokens"},
				"models": []map[string]any{{
					"id":            "demo-model",
					"reasoning":     false,
					"input":         []string{"text"},
					"contextWindow": 1000,
					"maxTokens":     100,
					"compat":        map[string]any{"supportsUsageInStreaming": true, "maxTokensField": "max_completion_tokens"},
				}},
			},
		},
	})
	registry = NewModelRegistry(auth, modelsPath)
	compat := registryMustFind(t, registry, "demo", "demo-model").Compat
	if compat.SupportsUsageInStreaming == nil || !*compat.SupportsUsageInStreaming || compat.MaxTokensField != "max_completion_tokens" {
		t.Fatalf("model compat override = %#v", compat)
	}

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"openrouter": map[string]any{"compat": map[string]any{"supportsUsageInStreaming": false, "supportsStrictMode": false}},
		},
	})
	registry = NewModelRegistry(auth, modelsPath)
	for _, model := range registryModelsForProvider(registry, "openrouter") {
		if model.Compat.SupportsUsageInStreaming == nil || *model.Compat.SupportsUsageInStreaming ||
			model.Compat.SupportsStrictMode == nil || *model.Compat.SupportsStrictMode {
			t.Fatalf("provider compat not applied to %#v", model.Compat)
		}
	}

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"demo": map[string]any{
				"baseUrl": "https://example.com/v1",
				"apiKey":  "DEMO_KEY",
				"api":     "openai-completions",
				"models": []map[string]any{{
					"id":               "demo-model",
					"reasoning":        true,
					"input":            []string{"text"},
					"contextWindow":    1000,
					"maxTokens":        100,
					"thinkingLevelMap": map[string]any{"minimal": nil, "high": "max"},
					"compat":           map[string]any{"supportsStrictMode": false, "cacheControlFormat": "anthropic"},
				}},
			},
		},
	})
	registry = NewModelRegistry(auth, modelsPath)
	model = registryMustFind(t, registry, "demo", "demo-model")
	if registry.GetError() != "" || model.ThinkingLevelMap["minimal"] != nil || *model.ThinkingLevelMap["high"] != "max" ||
		model.Compat.SupportsStrictMode == nil || *model.Compat.SupportsStrictMode || model.Compat.CacheControlFormat != "anthropic" {
		t.Fatalf("thinking map/compat schema = %#v / %#v / %s", model.ThinkingLevelMap, model.Compat, registry.GetError())
	}

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"demo": map[string]any{
				"baseUrl": "https://example.com",
				"apiKey":  "DEMO_KEY",
				"api":     "anthropic-messages",
				"compat":  map[string]any{"supportsEagerToolInputStreaming": false, "supportsLongCacheRetention": false},
				"models":  []map[string]any{{"id": "demo-model", "reasoning": true, "input": []string{"text"}, "contextWindow": 1000, "maxTokens": 100}},
			},
		},
	})
	registry = NewModelRegistry(auth, modelsPath)
	compat = registryMustFind(t, registry, "demo", "demo-model").Compat
	if compat.SupportsEagerToolInputStreaming == nil || *compat.SupportsEagerToolInputStreaming ||
		compat.SupportsLongCacheRetention == nil || *compat.SupportsLongCacheRetention {
		t.Fatalf("anthropic compat flags = %#v", compat)
	}

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"opencode-go": map[string]any{
				"baseUrl": "https://opencode.ai/zen/go/v1",
				"apiKey":  "TEST_KEY",
				"models": []map[string]any{
					{"id": "minimax-m2.5", "api": "anthropic-messages", "baseUrl": "https://opencode.ai/zen/go", "reasoning": true, "input": []string{"text"}},
					{"id": "glm-5", "api": "openai-completions", "reasoning": true, "input": []string{"text"}},
				},
			},
		},
	})
	registry = NewModelRegistry(auth, modelsPath)
	if registryMustFind(t, registry, "opencode-go", "minimax-m2.5").BaseURL != "https://opencode.ai/zen/go" ||
		registryMustFind(t, registry, "opencode-go", "glm-5").BaseURL != "https://opencode.ai/zen/go/v1" {
		t.Fatalf("model-level baseURL override failed")
	}
}

func TestModelRegistryModelOverridesAndRefresh(t *testing.T) {
	modelsPath := filepath.Join(t.TempDir(), "models.json")
	auth := NewInMemoryAuthStorage(nil)

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"openrouter": map[string]any{
				"baseUrl": "https://my-proxy.example.com/v1",
				"models":  []map[string]any{{"id": "custom/openrouter-model", "name": "Custom OpenRouter Model", "reasoning": false, "input": []string{"text"}}},
				"modelOverrides": map[string]any{
					"anthropic/claude-sonnet-4": map[string]any{"name": "Overridden Built-in Sonnet"},
				},
			},
		},
	})
	registry := NewModelRegistry(auth, modelsPath)
	openrouterModels := registryModelsForProvider(registry, "openrouter")
	if !registryHasModelID(openrouterModels, "custom/openrouter-model") ||
		!registryHasModelWithName(openrouterModels, "anthropic/claude-sonnet-4", "Overridden Built-in Sonnet") {
		t.Fatalf("models plus overrides = %#v", openrouterModels)
	}

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"openrouter": map[string]any{
				"modelOverrides": map[string]any{
					"anthropic/claude-sonnet-4": map[string]any{
						"name":    "Custom Sonnet Name",
						"headers": map[string]string{"X-Custom-Model-Header": "value"},
						"compat":  map[string]any{"openRouterRouting": map[string]any{"only": []string{"amazon-bedrock"}}},
						"cost":    map[string]float64{"input": 99},
					},
					"deepseek/deepseek-r1": map[string]any{
						"compat": map[string]any{"openRouterRouting": map[string]any{"only": []string{"anthropic"}}},
					},
				},
			},
		},
	})
	registry = NewModelRegistry(auth, modelsPath)
	sonnet := registryMustFind(t, registry, "openrouter", "anthropic/claude-sonnet-4")
	if sonnet.Name != "Custom Sonnet Name" || sonnet.Cost.Input != 99 ||
		!reflect.DeepEqual(sonnet.Compat.OpenRouterRouting, map[string]any{"only": []any{"amazon-bedrock"}}) {
		t.Fatalf("sonnet override = %#v", sonnet)
	}
	authResult := registry.GetAPIKeyAndHeaders(sonnet)
	if !authResult.OK || authResult.Headers["X-Custom-Model-Header"] != "value" {
		t.Fatalf("model headers = %#v", authResult)
	}
	if registryFind(registry, "openrouter", "nonexistent/model-id") {
		t.Fatalf("nonexistent override created a model")
	}

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"openrouter": map[string]any{"modelOverrides": map[string]any{"anthropic/claude-sonnet-4": map[string]any{"name": "First Name"}}},
		},
	})
	registry = NewModelRegistry(auth, modelsPath)
	if registryMustFind(t, registry, "openrouter", "anthropic/claude-sonnet-4").Name != "First Name" {
		t.Fatalf("first override missing")
	}
	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"openrouter": map[string]any{"modelOverrides": map[string]any{"anthropic/claude-sonnet-4": map[string]any{"name": "Second Name"}}},
		},
	})
	registry.Refresh()
	if registryMustFind(t, registry, "openrouter", "anthropic/claude-sonnet-4").Name != "Second Name" {
		t.Fatalf("refresh override missing")
	}
	writeRawModelsJSON(t, modelsPath, map[string]any{"providers": map[string]any{}})
	registry.Refresh()
	if registryMustFind(t, registry, "openrouter", "anthropic/claude-sonnet-4").Name == "Second Name" {
		t.Fatalf("override should be removed after refresh")
	}
}

func TestModelRegistryDynamicProviderLifecycle(t *testing.T) {
	modelsPath := filepath.Join(t.TempDir(), "models.json")
	auth := NewInMemoryAuthStorage(nil)
	registry := NewModelRegistry(auth, modelsPath)

	if registry.GetProviderDisplayName("openai") != "OpenAI" ||
		registry.GetProviderDisplayName("github-copilot") != "GitHub Copilot" ||
		registry.GetProviderDisplayName("unknown-provider") != "unknown-provider" {
		t.Fatalf("built-in provider display names failed")
	}

	err := registry.RegisterProvider("named-provider", ProviderConfigInput{
		Name:    "Named Provider",
		BaseURL: "https://provider.test/v1",
		APIKey:  "TEST_KEY",
		API:     "openai-completions",
		Models:  registryModelDefinitions([]string{"demo-model"}, "openai-completions"),
	})
	if err != nil || registry.GetProviderDisplayName("named-provider") != "Named Provider" {
		t.Fatalf("named provider = %v / %s", err, registry.GetProviderDisplayName("named-provider"))
	}
	err = registry.RegisterProvider("oauth-provider", ProviderConfigInput{
		BaseURL: "https://provider.test/v1",
		API:     "openai-completions",
		OAuth:   &OAuthProvider{Name: "OAuth Provider", GetAPIKey: func(c AuthCredential) string { return c.Access }},
		Models:  registryModelDefinitions([]string{"demo-model"}, "openai-completions"),
	})
	if err != nil || registry.GetProviderDisplayName("oauth-provider") != "OAuth Provider" {
		t.Fatalf("oauth provider display = %v / %s", err, registry.GetProviderDisplayName("oauth-provider"))
	}

	if err := registry.RegisterProvider("broken-provider", ProviderConfigInput{
		StreamSimple: func(llm.Model, llm.Context, llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
			return nil, errors.New("should not run")
		},
	}); err == nil || !strings.Contains(err.Error(), `"api" is required when registering streamSimple`) {
		t.Fatalf("expected streamSimple validation error, got %v", err)
	}
	registry.Refresh()

	if err := registry.RegisterProvider("demo-provider", ProviderConfigInput{
		BaseURL: "https://provider.test/v1",
		APIKey:  "TEST_KEY",
		API:     "openai-completions",
		Models:  registryModelDefinitions([]string{"demo-model"}, "openai-completions"),
	}); err != nil {
		t.Fatalf("register demo provider: %v", err)
	}
	if err := registry.RegisterProvider("demo-provider", ProviderConfigInput{
		BaseURL: "https://provider.test/v2",
		APIKey:  "TEST_KEY",
		Models:  registryModelDefinitions([]string{"broken-model"}, ""),
	}); err == nil || !strings.Contains(err.Error(), `no "api" specified`) {
		t.Fatalf("expected broken model validation, got %v", err)
	}
	if !registryFind(registry, "demo-provider", "demo-model") {
		t.Fatalf("failed register removed existing models")
	}
	registry.Refresh()
	if !registryFind(registry, "demo-provider", "demo-model") {
		t.Fatalf("refresh should preserve existing dynamic provider")
	}

	RegisterOAuthProvider(OAuthProvider{ID: "anthropic", Name: "Built-in Anthropic OAuth"})
	t.Cleanup(ResetOAuthProviders)
	err = registry.RegisterProvider("anthropic", ProviderConfigInput{OAuth: &OAuthProvider{Name: "Custom Anthropic OAuth"}})
	if err != nil {
		t.Fatalf("register oauth override: %v", err)
	}
	if provider, _ := GetOAuthProvider("anthropic"); provider.Name != "Custom Anthropic OAuth" {
		t.Fatalf("custom oauth provider = %#v", provider)
	}
	registry.UnregisterProvider("anthropic")
	if provider, _ := GetOAuthProvider("anthropic"); provider.Name == "Custom Anthropic OAuth" {
		t.Fatalf("oauth provider was not restored")
	}
	UnregisterOAuthProvider("anthropic")

	const streamOverrideAPI = "gi-test-stream-override"
	if provider := llm.GetAPIProvider(streamOverrideAPI); provider != nil {
		t.Fatalf("test api provider already registered: %s", streamOverrideAPI)
	}
	err = registry.RegisterProvider("stream-override-provider", ProviderConfigInput{
		API: streamOverrideAPI,
		StreamSimple: func(llm.Model, llm.Context, llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
			return nil, errors.New("custom streamSimple override")
		},
	})
	if err != nil {
		t.Fatalf("stream override register: %v", err)
	}
	_, streamErr := llm.GetAPIProvider(streamOverrideAPI).StreamSimple(llm.Model{ID: "test-openai-model", Provider: "openai", API: streamOverrideAPI}, llm.Context{}, llm.SimpleStreamOptions{})
	if streamErr == nil || streamErr.Error() != "custom streamSimple override" {
		t.Fatalf("stream override err = %v", streamErr)
	}
	registry.UnregisterProvider("stream-override-provider")
	if provider := llm.GetAPIProvider(streamOverrideAPI); provider != nil {
		t.Fatalf("stream override api provider was not removed: %#v", provider)
	}

	registry = NewModelRegistry(auth, modelsPath)
	if err := registry.RegisterProvider("anthropic", ProviderConfigInput{BaseURL: "https://proxy.test/anthropic"}); err != nil {
		t.Fatalf("baseURL override register: %v", err)
	}
	registry.Refresh()
	for _, model := range registryModelsForProvider(registry, "anthropic") {
		if model.BaseURL != "https://proxy.test/anthropic" {
			t.Fatalf("dynamic baseURL not persisted: %#v", model)
		}
	}
	if err := registry.RegisterProvider("anthropic", ProviderConfigInput{BaseURL: "https://custom.test/anthropic", APIKey: "TEST_KEY", API: "anthropic-messages", Models: registryModelDefinitions([]string{"custom-claude"}, "anthropic-messages")}); err != nil {
		t.Fatalf("models override register: %v", err)
	}
	registry.Refresh()
	if ids := registryModelIDs(registryModelsForProvider(registry, "anthropic")); !reflect.DeepEqual(ids, []string{"custom-claude"}) {
		t.Fatalf("models-only dynamic replacement = %#v", ids)
	}
	if err := registry.RegisterProvider("anthropic", ProviderConfigInput{BaseURL: "https://proxy.test/anthropic"}); err != nil {
		t.Fatalf("second baseURL register: %v", err)
	}
	registry.Refresh()
	if registryMustFind(t, registry, "anthropic", "custom-claude").BaseURL != "https://proxy.test/anthropic" {
		t.Fatalf("baseURL-only dynamic update did not keep custom models")
	}

	registry = NewModelRegistry(auth, modelsPath)
	if err := registry.RegisterProvider("custom-provider", ProviderConfigInput{BaseURL: "https://custom.test/v1", APIKey: "TEST_KEY", API: "openai-completions", Models: registryModelDefinitions([]string{"custom-a", "custom-b"}, "openai-completions")}); err != nil {
		t.Fatalf("custom provider register: %v", err)
	}
	if err := registry.RegisterProvider("custom-provider", ProviderConfigInput{Headers: map[string]string{"x-proxy": "enabled"}}); err != nil {
		t.Fatalf("headers register: %v", err)
	}
	registry.Refresh()
	models := registryModelsForProvider(registry, "custom-provider")
	if !reflect.DeepEqual(registryModelIDs(models), []string{"custom-a", "custom-b"}) || models[0].BaseURL != "https://custom.test/v1" {
		t.Fatalf("custom provider refresh = %#v", models)
	}
	if authResult := registry.GetAPIKeyAndHeaders(models[0]); !authResult.OK || authResult.Headers["x-proxy"] != "enabled" {
		t.Fatalf("custom provider headers = %#v", authResult)
	}
}

func TestModelRegistryAPIKeyResolution(t *testing.T) {
	tempDir := t.TempDir()
	modelsPath := filepath.Join(tempDir, "models.json")
	auth := NewInMemoryAuthStorage(nil)
	defer ClearAPIKeyCache()

	writeRawModelsJSON(t, modelsPath, map[string]any{"providers": map[string]any{"custom-provider": registryProviderWithAPIKey("!echo test-api-key-from-command")}})
	registry := NewModelRegistry(auth, modelsPath)
	if key, ok := registry.GetAPIKeyForProvider("custom-provider"); !ok || key != "test-api-key-from-command" {
		t.Fatalf("command api key = %q / %v", key, ok)
	}

	for name, apiKey := range map[string]string{
		"spaced":    "!echo '  spaced-key  '",
		"multiline": "!printf 'line1\\nline2'",
		"pipe":      "!echo 'hello world' | tr ' ' '-'",
		"literal":   "literal_api_key_value",
	} {
		deleteEnv(t, apiKey)
		writeRawModelsJSON(t, modelsPath, map[string]any{"providers": map[string]any{"custom-provider": registryProviderWithAPIKey(apiKey)}})
		registry = NewModelRegistry(auth, modelsPath)
		key, ok := registry.GetAPIKeyForProvider("custom-provider")
		want := map[string]string{"spaced": "spaced-key", "multiline": "line1\nline2", "pipe": "hello-world", "literal": "literal_api_key_value"}[name]
		if !ok || key != want {
			t.Fatalf("%s api key = %q / %v, want %q", name, key, ok, want)
		}
	}

	for _, apiKey := range []string{"!exit 1", "!nonexistent-command-12345", "!printf ''"} {
		writeRawModelsJSON(t, modelsPath, map[string]any{"providers": map[string]any{"custom-provider": registryProviderWithAPIKey(apiKey)}})
		registry = NewModelRegistry(auth, modelsPath)
		if key, ok := registry.GetAPIKeyForProvider("custom-provider"); ok || key != "" {
			t.Fatalf("failed command %q = %q / %v", apiKey, key, ok)
		}
	}

	t.Setenv("TEST_API_KEY_12345", "env-api-key-value")
	writeRawModelsJSON(t, modelsPath, map[string]any{"providers": map[string]any{"custom-provider": registryProviderWithAPIKey("TEST_API_KEY_12345")}})
	registry = NewModelRegistry(auth, modelsPath)
	if key, ok := registry.GetAPIKeyForProvider("custom-provider"); !ok || key != "env-api-key-value" {
		t.Fatalf("env api key = %q / %v", key, ok)
	}

	counterFile := filepath.Join(tempDir, "counter")
	writeTextFile(t, counterFile, "0")
	counterPath := toShellPath(counterFile)
	command := `!sh -c 'count=$(cat "` + counterPath + `"); echo $((count + 1)) > "` + counterPath + `"; echo "key-value"'`
	writeRawModelsJSON(t, modelsPath, map[string]any{"providers": map[string]any{"custom-provider": registryProviderWithAPIKey(command)}})
	registry = NewModelRegistry(auth, modelsPath)
	for i := 0; i < 3; i++ {
		if key, ok := registry.GetAPIKeyForProvider("custom-provider"); !ok || key != "key-value" {
			t.Fatalf("request-time key %d = %q / %v", i, key, ok)
		}
	}
	if readTextFile(t, counterFile) != "3" {
		t.Fatalf("command count = %s", readTextFile(t, counterFile))
	}
	registry2 := NewModelRegistry(auth, modelsPath)
	if _, ok := registry2.GetAPIKeyForProvider("custom-provider"); !ok {
		t.Fatalf("second registry did not resolve command")
	}
	if readTextFile(t, counterFile) != "4" {
		t.Fatalf("second registry count = %s", readTextFile(t, counterFile))
	}

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"provider-a": registryProviderWithAPIKey("!echo key-a"),
			"provider-b": registryProviderWithAPIKey("!echo key-b"),
		},
	})
	registry = NewModelRegistry(auth, modelsPath)
	if key, _ := registry.GetAPIKeyForProvider("provider-a"); key != "key-a" {
		t.Fatalf("provider-a key = %q", key)
	}
	if key, _ := registry.GetAPIKeyForProvider("provider-b"); key != "key-b" {
		t.Fatalf("provider-b key = %q", key)
	}

	counterFile = filepath.Join(tempDir, "fail-counter")
	writeTextFile(t, counterFile, "0")
	counterPath = toShellPath(counterFile)
	command = `!sh -c 'count=$(cat "` + counterPath + `"); echo $((count + 1)) > "` + counterPath + `"; exit 1'`
	writeRawModelsJSON(t, modelsPath, map[string]any{"providers": map[string]any{"custom-provider": registryProviderWithAPIKey(command)}})
	registry = NewModelRegistry(auth, modelsPath)
	registry.GetAPIKeyForProvider("custom-provider")
	registry.GetAPIKeyForProvider("custom-provider")
	if readTextFile(t, counterFile) != "2" {
		t.Fatalf("failed command should be retried, count = %s", readTextFile(t, counterFile))
	}

	t.Setenv("TEST_API_KEY_STATUS_TEST_98765", "status-test-key")
	writeRawModelsJSON(t, modelsPath, map[string]any{"providers": map[string]any{"custom-provider": registryProviderWithAPIKey("TEST_API_KEY_STATUS_TEST_98765")}})
	registry = NewModelRegistry(auth, modelsPath)
	if got := registry.GetProviderAuthStatus("custom-provider"); got != (AuthStatus{Configured: true, Source: "environment", Label: "TEST_API_KEY_STATUS_TEST_98765"}) {
		t.Fatalf("env auth status = %#v", got)
	}

	writeRawModelsJSON(t, modelsPath, map[string]any{"providers": map[string]any{"custom-provider": registryProviderWithAPIKey("literal_api_key_value")}})
	registry = NewModelRegistry(auth, modelsPath)
	if got := registry.GetProviderAuthStatus("custom-provider"); got != (AuthStatus{Configured: true, Source: "models_json_key"}) {
		t.Fatalf("literal auth status = %#v", got)
	}

	statusCounter := filepath.Join(tempDir, "status-counter")
	writeTextFile(t, statusCounter, "0")
	statusCommand := `!sh -c 'echo 1 > "` + toShellPath(statusCounter) + `"; echo key-value'`
	writeRawModelsJSON(t, modelsPath, map[string]any{"providers": map[string]any{"custom-provider": registryProviderWithAPIKey(statusCommand)}})
	registry = NewModelRegistry(auth, modelsPath)
	if got := registry.GetProviderAuthStatus("custom-provider"); got != (AuthStatus{Configured: true, Source: "models_json_command"}) {
		t.Fatalf("command auth status = %#v", got)
	}
	if readTextFile(t, statusCounter) != "0" {
		t.Fatalf("auth status executed command")
	}

	t.Setenv("TEST_API_KEY_CACHE_TEST_98765", "first-value")
	writeRawModelsJSON(t, modelsPath, map[string]any{"providers": map[string]any{"custom-provider": registryProviderWithAPIKey("TEST_API_KEY_CACHE_TEST_98765")}})
	registry = NewModelRegistry(auth, modelsPath)
	if key, _ := registry.GetAPIKeyForProvider("custom-provider"); key != "first-value" {
		t.Fatalf("first env key = %q", key)
	}
	t.Setenv("TEST_API_KEY_CACHE_TEST_98765", "second-value")
	if key, _ := registry.GetAPIKeyForProvider("custom-provider"); key != "second-value" {
		t.Fatalf("second env key = %q", key)
	}

	counterFile = filepath.Join(tempDir, "available-counter")
	writeTextFile(t, counterFile, "0")
	command = `!sh -c 'count=$(cat "` + toShellPath(counterFile) + `"); echo $((count + 1)) > "` + toShellPath(counterFile) + `"; echo "key-value"'`
	writeRawModelsJSON(t, modelsPath, map[string]any{"providers": map[string]any{"custom-provider": registryProviderWithAPIKey(command)}})
	registry = NewModelRegistry(auth, modelsPath)
	if !registryHasProvider(registry.GetAvailable(), "custom-provider") || readTextFile(t, counterFile) != "0" {
		t.Fatalf("getAvailable executed command or missed provider")
	}

	tokenFile := filepath.Join(tempDir, "token")
	writeTextFile(t, tokenFile, "token-1")
	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"custom-provider": mergeRawMaps(registryProviderWithAPIKey(`!sh -c 'cat "`+toShellPath(tokenFile)+`"'`), map[string]any{"authHeader": true}),
		},
	})
	registry = NewModelRegistry(auth, modelsPath)
	model := registryMustFind(t, registry, "custom-provider", "test-model")
	authResult := registry.GetAPIKeyAndHeaders(model)
	if !authResult.OK || authResult.APIKey != "token-1" || authResult.Headers["Authorization"] != "Bearer token-1" {
		t.Fatalf("auth header 1 = %#v", authResult)
	}
	writeTextFile(t, tokenFile, "token-2")
	authResult = registry.GetAPIKeyAndHeaders(model)
	if !authResult.OK || authResult.APIKey != "token-2" || authResult.Headers["Authorization"] != "Bearer token-2" {
		t.Fatalf("auth header 2 = %#v", authResult)
	}

	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"custom-provider": mergeRawMaps(registryProviderWithAPIKey("!exit 1"), map[string]any{"authHeader": true}),
		},
	})
	registry = NewModelRegistry(auth, modelsPath)
	model = registryMustFind(t, registry, "custom-provider", "test-model")
	authResult = registry.GetAPIKeyAndHeaders(model)
	if authResult.OK || !strings.Contains(authResult.Error, `Failed to resolve API key for provider "custom-provider"`) {
		t.Fatalf("failed auth header = %#v", authResult)
	}
}

func TestModelRegistryAcceptsAuthOnlyProviderConfiguration(t *testing.T) {
	modelsPath := filepath.Join(t.TempDir(), "models.json")

	t.Run("models.json auth overrides ambient provider auth", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "ambient-key")
		t.Setenv("MODEL_REGISTRY_TENANT", "process-tenant")
		writeRawModelsJSON(t, modelsPath, map[string]any{
			"providers": map[string]any{
				"openai": map[string]any{
					"apiKey": "configured-key",
					"headers": map[string]any{
						"X-Tenant": "$MODEL_REGISTRY_TENANT",
					},
				},
			},
		})
		registry := NewModelRegistry(NewInMemoryAuthStorage(nil), modelsPath)
		if registry.GetError() != "" {
			t.Fatalf("load error = %q", registry.GetError())
		}
		if got := registry.GetProviderAuthStatus("openai"); got !=
			(AuthStatus{Configured: true, Source: "models_json_key"}) {
			t.Fatalf("auth status = %#v", got)
		}
		if key, ok := registry.GetAPIKeyForProvider("openai"); !ok ||
			key != "configured-key" {
			t.Fatalf("provider key = %q, configured=%v", key, ok)
		}
		requestAuth := registry.GetAPIKeyAndHeaders(
			llm.Model{Provider: "openai", ID: "test"},
		)
		if !requestAuth.OK ||
			requestAuth.APIKey != "configured-key" ||
			requestAuth.Headers["X-Tenant"] != "process-tenant" {
			t.Fatalf("request auth = %#v", requestAuth)
		}

		storedRegistry := NewModelRegistry(
			NewInMemoryAuthStorage(AuthStorageData{
				"openai": {
					Type: llm.CredentialTypeAPIKey,
					Key:  "stored-key",
					Env: llm.ProviderEnv{
						"MODEL_REGISTRY_TENANT": "credential-tenant",
					},
				},
			}),
			modelsPath,
		)
		if key, ok := storedRegistry.GetAPIKeyForProvider("openai"); !ok ||
			key != "stored-key" {
			t.Fatalf("stored provider key = %q, configured=%v", key, ok)
		}
		storedRequestAuth := storedRegistry.GetAPIKeyAndHeaders(
			llm.Model{Provider: "openai", ID: "test"},
		)
		if !storedRequestAuth.OK ||
			storedRequestAuth.Headers["X-Tenant"] !=
				"credential-tenant" ||
			storedRequestAuth.Env["MODEL_REGISTRY_TENANT"] !=
				"credential-tenant" {
			t.Fatalf("stored request auth = %#v", storedRequestAuth)
		}
	})

	t.Run("explicit false authHeader remains valid configuration", func(t *testing.T) {
		writeRawModelsJSON(t, modelsPath, map[string]any{
			"providers": map[string]any{
				"openai": map[string]any{"authHeader": false},
			},
		})
		registry := NewModelRegistry(NewInMemoryAuthStorage(nil), modelsPath)
		if registry.GetError() != "" {
			t.Fatalf("load error = %q", registry.GetError())
		}
	})
}

func registryProviderConfig(baseURL string, ids []string, api string) ProviderConfigInput {
	return ProviderConfigInput{
		BaseURL: baseURL,
		APIKey:  "TEST_KEY",
		API:     api,
		Models:  registryModelDefinitions(ids, api),
	}
}

func registryModelDefinitions(ids []string, api string) []ProviderModelDefinition {
	models := make([]ProviderModelDefinition, 0, len(ids))
	for _, id := range ids {
		models = append(models, ProviderModelDefinition{
			ID:            id,
			Name:          id,
			API:           api,
			Reasoning:     false,
			Input:         []string{"text"},
			Cost:          llm.ModelCost{},
			ContextWindow: 100000,
			MaxTokens:     8000,
		})
	}
	return models
}

func registryProviderWithAPIKey(apiKey string) map[string]any {
	return map[string]any{
		"baseUrl": "https://example.com/v1",
		"apiKey":  apiKey,
		"api":     "anthropic-messages",
		"models": []map[string]any{{
			"id":            "test-model",
			"name":          "Test Model",
			"reasoning":     false,
			"input":         []string{"text"},
			"contextWindow": 100000,
			"maxTokens":     8000,
		}},
	}
}

func writeModelsJSON(t *testing.T, path string, providers map[string]ProviderConfigInput) {
	t.Helper()
	writeRawModelsJSON(t, path, map[string]any{"providers": providers})
}

func writeRawModelsJSON(t *testing.T, path string, value any) {
	t.Helper()
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal models json: %v", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write models json: %v", err)
	}
}

func registryModelsForProvider(registry *ModelRegistry, provider string) []llm.Model {
	models := registry.GetAll()
	result := make([]llm.Model, 0)
	for _, model := range models {
		if model.Provider == provider {
			result = append(result, model)
		}
	}
	return result
}

func registryModelsWithID(models []llm.Model, id string) []llm.Model {
	result := []llm.Model{}
	for _, model := range models {
		if model.ID == id {
			result = append(result, model)
		}
	}
	return result
}

func registryHasModelID(models []llm.Model, id string) bool {
	return len(registryModelsWithID(models, id)) > 0
}

func registryHasModelWithName(models []llm.Model, id, name string) bool {
	for _, model := range models {
		if model.ID == id && model.Name == name {
			return true
		}
	}
	return false
}

func registryHasModelIDContaining(models []llm.Model, needle string) bool {
	for _, model := range models {
		if strings.Contains(model.ID, needle) {
			return true
		}
	}
	return false
}

func registryHasProvider(models []llm.Model, provider string) bool {
	for _, model := range models {
		if model.Provider == provider {
			return true
		}
	}
	return false
}

func registryModelIDs(models []llm.Model) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}

func registryMustFind(t *testing.T, registry *ModelRegistry, provider, modelID string) llm.Model {
	t.Helper()
	model, ok := registry.Find(provider, modelID)
	if !ok {
		t.Fatalf("missing model %s/%s", provider, modelID)
	}
	return model
}

func registryFind(registry *ModelRegistry, provider, modelID string) bool {
	_, ok := registry.Find(provider, modelID)
	return ok
}

func writeTextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write text file: %v", err)
	}
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read text file: %v", err)
	}
	return strings.TrimSpace(string(content))
}

func toShellPath(path string) string {
	return strings.ReplaceAll(strings.ReplaceAll(path, `\`, `/`), `"`, `\"`)
}

func deleteEnv(t *testing.T, key string) {
	t.Helper()
	if strings.HasPrefix(key, "!") {
		return
	}
	t.Setenv(key, "")
}

func mergeRawMaps(base map[string]any, extra map[string]any) map[string]any {
	merged := map[string]any{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return merged
}
