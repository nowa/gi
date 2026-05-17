package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadPromptTemplatesLoadsMarkdownNonRecursively(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "a", "nested"))
	mustMkdir(t, filepath.Join(root, "b"))
	mustWrite(t, filepath.Join(root, "a", "one.md"), "---\ndescription: One template\n---\nHello $1")
	mustWrite(t, filepath.Join(root, "a", "nested", "ignored.md"), "Ignored")
	mustWrite(t, filepath.Join(root, "b", "two.md"), "First line description\nBody")

	result := LoadPromptTemplates(filepath.Join(root, "a"), filepath.Join(root, "b"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	want := []PromptTemplate{
		{Name: "one", Description: "One template", Content: "Hello $1"},
		{Name: "two", Description: "First line description", Content: "First line description\nBody"},
	}
	if !reflect.DeepEqual(result.PromptTemplates, want) {
		t.Fatalf("prompt templates = %#v, want %#v", result.PromptTemplates, want)
	}
}

func TestLoadSourcedPromptTemplatesPreservesSource(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "prompts"))
	path := filepath.Join(root, "prompts")
	mustWrite(t, filepath.Join(path, "example.md"), "---\ndescription: Example\n---\nExample body")

	source := map[string]string{"type": "project"}
	result := LoadSourcedPromptTemplates(map[string]any{path: source})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if len(result.PromptTemplates) != 1 || !reflect.DeepEqual(result.PromptTemplates[0].Source, source) {
		t.Fatalf("sourced templates = %#v", result.PromptTemplates)
	}
	if result.PromptTemplates[0].PromptTemplate != (PromptTemplate{Name: "example", Description: "Example", Content: "Example body"}) {
		t.Fatalf("template = %#v", result.PromptTemplates[0].PromptTemplate)
	}
}

func TestLoadSourcedPromptTemplatesAttachesSourceToDiagnostics(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "broken.md")
	mustWrite(t, path, "---\ndescription: [unterminated\n---\nBody")
	source := map[string]string{"type": "user"}

	result := LoadSourcedPromptTemplates(map[string]any{path: source})
	if len(result.PromptTemplates) != 0 || len(result.Diagnostics) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if result.Diagnostics[0].Code != "parse_failed" || result.Diagnostics[0].Path != path || !reflect.DeepEqual(result.Diagnostics[0].Source, source) {
		t.Fatalf("diagnostic = %#v", result.Diagnostics[0])
	}
}

func TestLoadPromptTemplatesLoadsExplicitAndSymlinkedFiles(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.md")
	link := filepath.Join(root, "link.md")
	mustWrite(t, target, "---\ndescription: Target\n---\nTarget body")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	result := LoadPromptTemplates(target, link)
	want := []PromptTemplate{
		{Name: "target", Description: "Target", Content: "Target body"},
		{Name: "link", Description: "Target", Content: "Target body"},
	}
	if !reflect.DeepEqual(result.PromptTemplates, want) {
		t.Fatalf("prompt templates = %#v, want %#v", result.PromptTemplates, want)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
