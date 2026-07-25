package gicodingagent

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestConfigValueMigrationLeavesUppercaseAuthJSONAPIKeyValuesUnchanged(
	t *testing.T,
) {
	agentDir := t.TempDir()
	authPath := filepath.Join(agentDir, "auth.json")
	data := AuthStorageData{
		"anthropic": {
			Type: llm.CredentialTypeAPIKey,
			Key:  "ANTHROPIC_API_KEY",
		},
		"openai": {
			Type: llm.CredentialTypeAPIKey,
			Key:  "$OPENAI_API_KEY",
		},
		"opencode": {
			Type: llm.CredentialTypeAPIKey,
			Key:  "public",
		},
		"github": {
			Type:    llm.CredentialTypeOAuth,
			Access:  "ACCESS_TOKEN",
			Refresh: "REFRESH_TOKEN",
			Expires: 1,
		},
	}
	content, err := marshalAuthStorageData(data)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(content), '\n')
	if err := os.WriteFile(authPath, before, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := RunMigrationsWithResult(agentDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.MigratedAuthProviders) != 0 ||
		len(result.DeprecationWarnings) != 0 {
		t.Fatalf("migration result = %#v", result)
	}
	after, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("auth.json changed:\n%s", after)
	}

	t.Setenv("ANTHROPIC_API_KEY", "env-anthropic")
	t.Setenv("OPENAI_API_KEY", "env-openai")
	storage := NewAuthStorage(authPath)
	if key, ok := storage.GetAPIKey("anthropic"); !ok ||
		key != "ANTHROPIC_API_KEY" {
		t.Fatalf("uppercase literal key = %q, configured=%v", key, ok)
	}
	if key, ok := storage.GetAPIKey("openai"); !ok ||
		key != "env-openai" {
		t.Fatalf("explicit environment key = %q, configured=%v", key, ok)
	}
	if key, ok := storage.GetAPIKey("opencode"); !ok || key != "public" {
		t.Fatalf("lowercase literal key = %q, configured=%v", key, ok)
	}
	github, ok := storage.Get("github")
	if !ok ||
		github.Access != "ACCESS_TOKEN" ||
		github.Refresh != "REFRESH_TOKEN" {
		t.Fatalf("OAuth credential = %#v, exists=%v", github, ok)
	}
}

func TestConfigValueMigrationLeavesUppercaseModelsJSONAPIKeyAndHeaderValuesUnchanged(
	t *testing.T,
) {
	agentDir := t.TempDir()
	modelsPath := filepath.Join(agentDir, "models.json")
	for _, name := range []string{
		"CUSTOM_API_KEY",
		"HEADER_API_KEY",
		"MODEL_API_KEY",
		"OVERRIDE_API_KEY",
	} {
		t.Setenv(name, "env-"+name)
	}
	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"custom-provider": map[string]any{
				"baseUrl": "https://example.com/v1",
				"apiKey":  "CUSTOM_API_KEY",
				"api":     "openai-completions",
				"headers": map[string]string{
					"x-api-key": "HEADER_API_KEY",
					"x-literal": "literal",
				},
				"models": []map[string]any{
					{
						"id": "model-a",
						"headers": map[string]string{
							"x-model-key": "MODEL_API_KEY",
						},
					},
				},
				"modelOverrides": map[string]any{
					"model-b": map[string]any{
						"headers": map[string]string{
							"x-override-key": "OVERRIDE_API_KEY",
						},
					},
				},
			},
		},
	})
	before, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}

	result, err := RunMigrationsWithResult(agentDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.MigratedAuthProviders) != 0 ||
		len(result.DeprecationWarnings) != 0 {
		t.Fatalf("migration result = %#v", result)
	}
	after, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("models.json changed:\n%s", after)
	}

	registry := NewModelRegistry(
		NewInMemoryAuthStorage(nil),
		modelsPath,
	)
	model := registryMustFind(
		t,
		registry,
		"custom-provider",
		"model-a",
	)
	if key, ok := registry.GetAPIKeyForProvider("custom-provider"); !ok ||
		key != "CUSTOM_API_KEY" {
		t.Fatalf("provider key = %q, configured=%v", key, ok)
	}
	auth := registry.GetAPIKeyAndHeaders(model)
	if !auth.OK ||
		auth.APIKey != "CUSTOM_API_KEY" ||
		auth.Headers["x-api-key"] != "HEADER_API_KEY" ||
		auth.Headers["x-literal"] != "literal" ||
		auth.Headers["x-model-key"] != "MODEL_API_KEY" {
		t.Fatalf("request auth = %#v", auth)
	}
}

func TestUppercaseHeaderRegressionKeepsUppercaseHeaderStringsAsLiteralsDuringStartupMigrations(
	t *testing.T,
) {
	agentDir := t.TempDir()
	modelsPath := filepath.Join(agentDir, "models.json")
	t.Setenv("CUSTOM_API_KEY", "env-CUSTOM_API_KEY")
	t.Setenv("BEARER", "env-BEARER")
	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"my-provider": map[string]any{
				"baseUrl": "https://example.com/v1",
				"apiKey":  "CUSTOM_API_KEY",
				"api":     "openai-completions",
				"headers": map[string]string{
					"Authorization": "BEARER",
				},
				"models": []map[string]any{{"id": "my-model"}},
			},
		},
	})
	before, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := RunMigrations(agentDir); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("models.json changed:\n%s", after)
	}

	registry := NewModelRegistry(
		NewInMemoryAuthStorage(nil),
		modelsPath,
	)
	model := registryMustFind(t, registry, "my-provider", "my-model")
	auth := registry.GetAPIKeyAndHeaders(model)
	if !auth.OK ||
		auth.APIKey != "CUSTOM_API_KEY" ||
		auth.Headers["Authorization"] != "BEARER" {
		t.Fatalf("request auth = %#v", auth)
	}
}
