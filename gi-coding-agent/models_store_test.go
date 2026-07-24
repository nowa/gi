package gicodingagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestFileModelsStorePiExactCaseNames(t *testing.T) {
	t.Run("persists provider catalogs without replacing unrelated providers", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "models-store.json")
		store := NewFileModelsStore(path)
		ctx := context.Background()

		if err := store.WriteModels(ctx, "one", llm.ModelsStoreEntry{
			Models:    []llm.Model{modelsStoreTestModel("one", "m1")},
			CheckedAt: 100,
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.WriteModels(ctx, "two", llm.ModelsStoreEntry{
			Models:    []llm.Model{modelsStoreTestModel("two", "m2")},
			CheckedAt: 200,
		}); err != nil {
			t.Fatal(err)
		}

		reloaded := NewFileModelsStore(path)
		one, ok, err := reloaded.ReadModels(ctx, "one")
		if err != nil || !ok {
			t.Fatalf("read one = %#v, %v, %v", one, ok, err)
		}
		if len(one.Models) != 1 ||
			one.Models[0].ID != "m1" ||
			one.CheckedAt != 100 {
			t.Fatalf("one = %#v", one)
		}
		two, ok, err := reloaded.ReadModels(ctx, "two")
		if err != nil || !ok {
			t.Fatalf("read two = %#v, %v, %v", two, ok, err)
		}
		if len(two.Models) != 1 || two.Models[0].ID != "m2" {
			t.Fatalf("two = %#v", two)
		}

		if err := reloaded.DeleteModels(ctx, "one"); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := reloaded.ReadModels(ctx, "one"); err != nil || ok {
			t.Fatalf("deleted one still exists: %v, %v", ok, err)
		}
		two, ok, err = reloaded.ReadModels(ctx, "two")
		if err != nil || !ok ||
			len(two.Models) != 1 ||
			two.Models[0].ID != "m2" {
			t.Fatalf("two after delete = %#v, %v, %v", two, ok, err)
		}
	})
}

func TestFileModelsStoreHonorsContextAndPreservesInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models-store.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewFileModelsStore(path)
	if err := store.WriteModels(
		context.Background(),
		"radius",
		llm.ModelsStoreEntry{},
	); err == nil {
		t.Fatal("invalid store was silently replaced")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "{invalid" {
		t.Fatalf("invalid store changed to %q", content)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = store.ReadModels(ctx, "radius")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("read error = %v", err)
	}
}

func modelsStoreTestModel(provider, id string) llm.Model {
	return llm.Model{
		ID:            id,
		Name:          id,
		API:           "openai-completions",
		Provider:      provider,
		BaseURL:       "https://example.test/v1",
		Input:         []string{"text"},
		ContextWindow: 1000,
		MaxTokens:     100,
	}
}
