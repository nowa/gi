package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const testGeneratedAt = "2026-07-23T10:00:00.000Z"

func TestRunPreservesPublishedOrderAndModelMetadata(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "providers", "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "models.generated.js"), `
import { BETA_MODELS } from "./providers/beta.models.js";
import { ALPHA_MODELS } from "./providers/alpha.models.js";
export const MODELS = {};
`)
	writeTestFile(t, filepath.Join(dataDir, "beta.json"), `{
  "test-api": {
    "model-b2": {
      "id": "model-b2",
      "name": "Beta Two",
      "api": "test-api",
      "provider": "beta",
      "baseUrl": "https://beta.test",
      "reasoning": true,
      "input": ["text", "image"],
      "cost": {
        "input": 1,
        "output": 2,
        "cacheRead": 0.1,
        "cacheWrite": 1.25,
        "tiers": [{
          "inputTokensAbove": 100,
          "input": 2,
          "output": 3,
          "cacheRead": 0.2,
          "cacheWrite": 2.5
        }]
      },
      "contextWindow": 200,
      "maxTokens": 50,
      "headers": {"X-Test": "value"},
      "compat": {
        "supportsToolSearch": true,
        "requiresReasoningContentOnAssistantMessages": true,
        "sessionAffinityFormat": "openai"
      },
      "thinkingLevelMap": {"xhigh": null, "max": "max"}
    },
    "model-b1": {
      "id": "model-b1",
      "name": "Beta One",
      "api": "test-api",
      "provider": "beta",
      "baseUrl": "https://beta.test",
      "reasoning": false,
      "input": ["text"],
      "cost": {"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0},
      "contextWindow": 100,
      "maxTokens": 10
    }
  }
}`)
	writeTestFile(t, filepath.Join(dataDir, "alpha.json"), `{
  "test-api": {
    "model-a": {
      "id": "model-a",
      "name": "Alpha",
      "api": "test-api",
      "provider": "alpha",
      "baseUrl": "https://alpha.test",
      "reasoning": false,
      "input": ["text"],
      "cost": {"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0},
      "contextWindow": 100,
      "maxTokens": 10
    }
  }
}`)
	writeTestManifest(t, dataDir, testGeneratedAt)

	output := filepath.Join(root, "catalog.go")
	opts := options{
		dataDir: dataDir,
		output:  output,
		source:  "@earendil-works/pi-ai@test",
		pkg:     "catalogtest",
	}
	if err := run(opts); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := run(opts); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("generator output is not deterministic")
	}

	generated := string(first)
	assertOrdered(t, generated, `"model-b2"`, `"model-b1"`, `"model-a"`)
	for _, fragment := range []string{
		"Source: @earendil-works/pi-ai@test",
		`const piGeneratedModelDataGeneratedAt = "2026-07-23T10:00:00.000Z"`,
		"Tiers: []ModelCostTier{{InputTokensAbove: 100",
		"SupportsToolSearch: ptrBool(true)",
		"RequiresReasoningContentOnAssistantMessages: ptrBool(true)",
		`SessionAffinityFormat: "openai"`,
		`ThinkingLevelMap: map[string]*string{"max": ptrString("max"), "xhigh": nil}`,
	} {
		if !strings.Contains(generated, fragment) {
			t.Fatalf("generated catalog missing %q:\n%s", fragment, generated)
		}
	}
}

func TestRunRejectsUnknownPublishedFields(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "providers", "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "models.generated.js"), `
import { TEST_MODELS } from "./providers/test.models.js";
`)
	writeTestFile(t, filepath.Join(dataDir, "test.json"), `{
  "test-api": {
    "model": {
      "id": "model",
      "name": "Model",
      "api": "test-api",
      "provider": "test",
      "baseUrl": "https://test.invalid",
      "reasoning": false,
      "input": ["text"],
      "cost": {"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0},
      "contextWindow": 1,
      "maxTokens": 1,
      "newUpstreamField": true
    }
  }
}`)
	writeTestManifest(t, dataDir, testGeneratedAt)
	err := run(options{
		dataDir: dataDir,
		output:  filepath.Join(root, "catalog.go"),
		pkg:     "catalogtest",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown-field failure", err)
	}
}

func TestRunRejectsStaleManifestHash(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "providers", "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "models.generated.js"), `
import { TEST_MODELS } from "./providers/test.models.js";
`)
	modelPath := filepath.Join(dataDir, "test.json")
	writeTestFile(t, modelPath, `{
  "test-api": {
    "model": {
      "id": "model",
      "name": "Model",
      "api": "test-api",
      "provider": "test",
      "baseUrl": "https://test.invalid",
      "reasoning": false,
      "input": ["text"],
      "cost": {"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0},
      "contextWindow": 1,
      "maxTokens": 1
    }
  }
}`)
	writeTestManifest(t, dataDir, testGeneratedAt)
	writeTestFile(t, modelPath, strings.ReplaceAll(
		mustReadTestFile(t, modelPath),
		`"name": "Model"`,
		`"name": "Changed"`,
	))

	err := run(options{
		dataDir: dataDir,
		output:  filepath.Join(root, "catalog.go"),
		pkg:     "catalogtest",
	})
	if err == nil || !strings.Contains(err.Error(), "manifest hash") {
		t.Fatalf("error = %v, want manifest-hash failure", err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustReadTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func writeTestManifest(t *testing.T, dataDir, generatedAt string) {
	t.Helper()
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string]string)
	structure := make(map[string]map[string]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") ||
			entry.Name() == ".manifest.json" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dataDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(content)
		files[entry.Name()] = fmt.Sprintf("%x", sum)

		var groups map[string]map[string]json.RawMessage
		if err := json.Unmarshal(content, &groups); err != nil {
			t.Fatal(err)
		}
		providerID := strings.TrimSuffix(entry.Name(), ".json")
		models := make(map[string]string)
		apis := make([]string, 0, len(groups))
		for api := range groups {
			apis = append(apis, api)
		}
		sort.Strings(apis)
		for _, api := range apis {
			for modelID := range groups[api] {
				models[modelID] = api
			}
		}
		structure[providerID] = models
	}
	normalized, err := json.Marshal(structure)
	if err != nil {
		t.Fatal(err)
	}
	structureSum := sha256.Sum256(normalized)
	manifest := sourceManifest{
		SchemaVersion: sourceManifestSchemaVersion,
		GeneratedAt:   generatedAt,
		StructureHash: fmt.Sprintf("%x", structureSum),
		Files:         files,
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(
		t,
		filepath.Join(dataDir, ".manifest.json"),
		string(encoded)+"\n",
	)
}

func assertOrdered(t *testing.T, value string, fragments ...string) {
	t.Helper()
	position := -1
	for _, fragment := range fragments {
		next := strings.Index(value[position+1:], fragment)
		if next < 0 {
			t.Fatalf("missing ordered fragment %q", fragment)
		}
		position += next + 1
	}
}
