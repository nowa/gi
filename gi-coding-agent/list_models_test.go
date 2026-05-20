package gicodingagent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteListModelsPiTableShapeAndFuzzySearch(t *testing.T) {
	registry := NewInMemoryModelRegistry(NewInMemoryAuthStorage(AuthStorageData{
		"custom-provider": {Type: "api_key", Key: "test-key"},
	}))
	if err := registry.RegisterProvider("custom-provider", ProviderConfigInput{
		BaseURL: "https://example.test/v1",
		APIKey:  "TEST_KEY",
		API:     "openai-completions",
		Models: []ProviderModelDefinition{
			{ID: "faux-fast", Input: []string{"text"}, ContextWindow: 128000, MaxTokens: 8192},
			{ID: "faux-thinker", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 1000000},
		},
	}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := WriteListModels(&stdout, &stderr, registry, "think"); err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	if !strings.Contains(output, "provider") ||
		!strings.Contains(output, "model") ||
		!strings.Contains(output, "context") ||
		!strings.Contains(output, "max-out") ||
		!strings.Contains(output, "thinking") ||
		!strings.Contains(output, "images") {
		t.Fatalf("missing table headers:\n%s", output)
	}
	if !strings.Contains(output, "faux-thinker") ||
		!strings.Contains(output, "200K") ||
		!strings.Contains(output, "1M") ||
		!strings.Contains(output, "yes") ||
		strings.Contains(output, "faux-fast") {
		t.Fatalf("filtered output = %q", output)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunCLIListModelsUsesReadOnlyRegistry(t *testing.T) {
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	modelsJSON := `{
		"providers": {
			"custom-provider": {
				"baseUrl": "https://example.test/v1",
				"apiKey": "literal-key",
				"api": "openai-completions",
				"models": [
					{"id": "custom-coder", "contextWindow": 64000, "maxTokens": 4096, "reasoning": true, "input": ["text", "image"]}
				]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(agentDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunCLI(CLIOptions{
		Args:     []string{"--list-models", "coder"},
		Stdout:   &stdout,
		Stderr:   &stderr,
		CWD:      tempDir,
		AgentDir: agentDir,
	})

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "custom-provider") || !strings.Contains(stdout.String(), "custom-coder") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(agentDir, "auth.json")); !os.IsNotExist(err) {
		t.Fatalf("auth.json stat err = %v, want not exist", err)
	}
}
