package gicodingagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSystemPromptEmptyToolsShowsNone(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		SelectedTools: []string{},
		ContextFiles:  []SystemPromptContextFile{},
		Skills:        []SystemPromptSkill{},
		CWD:           "/tmp/project",
	})
	if !strings.Contains(prompt, "Available tools:\n(none)") {
		t.Fatalf("prompt should show empty tools list: %q", prompt)
	}
}

func TestBuildSystemPromptEmptyToolsShowsFilePathsGuideline(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		SelectedTools: []string{},
		ContextFiles:  []SystemPromptContextFile{},
		Skills:        []SystemPromptSkill{},
		CWD:           "/tmp/project",
	})
	if !strings.Contains(prompt, "Show file paths clearly") {
		t.Fatalf("prompt should include file path guideline: %q", prompt)
	}
}

func TestBuildSystemPromptIncludesDefaultToolsWhenSnippetsProvided(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		ToolSnippets: map[string]string{
			"read":  "Read file contents",
			"bash":  "Execute bash commands",
			"edit":  "Make surgical edits",
			"write": "Create or overwrite files",
		},
		ContextFiles: []SystemPromptContextFile{},
		Skills:       []SystemPromptSkill{},
		CWD:          "/tmp/project",
	})
	for _, name := range []string{"read", "bash", "edit", "write"} {
		if !strings.Contains(prompt, "- "+name+":") {
			t.Fatalf("prompt should include tool %q: %q", name, prompt)
		}
	}
}

func TestBuildSystemPromptIncludesCustomToolWithPromptSnippet(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		SelectedTools: []string{"read", "dynamic_tool"},
		ToolSnippets:  map[string]string{"dynamic_tool": "Run dynamic test behavior"},
		ContextFiles:  []SystemPromptContextFile{},
		Skills:        []SystemPromptSkill{},
		CWD:           "/tmp/project",
	})
	if !strings.Contains(prompt, "- dynamic_tool: Run dynamic test behavior") {
		t.Fatalf("prompt should include custom tool snippet: %q", prompt)
	}
}

func TestBuildSystemPromptOmitsCustomToolWithoutPromptSnippet(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		SelectedTools: []string{"read", "dynamic_tool"},
		ContextFiles:  []SystemPromptContextFile{},
		Skills:        []SystemPromptSkill{},
		CWD:           "/tmp/project",
	})
	if strings.Contains(prompt, "dynamic_tool") {
		t.Fatalf("prompt should omit custom tool without snippet: %q", prompt)
	}
}

func TestBuildSystemPromptAppendsPromptGuidelines(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		SelectedTools:    []string{"read", "dynamic_tool"},
		PromptGuidelines: []string{"Use dynamic_tool for project summaries."},
		ContextFiles:     []SystemPromptContextFile{},
		Skills:           []SystemPromptSkill{},
		CWD:              "/tmp/project",
	})
	if !strings.Contains(prompt, "- Use dynamic_tool for project summaries.") {
		t.Fatalf("prompt should include custom guideline: %q", prompt)
	}
}

func TestBuildSystemPromptDeduplicatesAndTrimsPromptGuidelines(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		SelectedTools: []string{"read", "dynamic_tool"},
		PromptGuidelines: []string{
			"Use dynamic_tool for summaries.",
			"  Use dynamic_tool for summaries.  ",
			"   ",
		},
		ContextFiles: []SystemPromptContextFile{},
		Skills:       []SystemPromptSkill{},
		CWD:          "/tmp/project",
	})
	if count := strings.Count(prompt, "- Use dynamic_tool for summaries."); count != 1 {
		t.Fatalf("guideline count = %d, want 1: %q", count, prompt)
	}
}

func TestBuildSystemPromptIncludesGiDocumentationPathsWhenProvided(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		CWD:           "/tmp/project",
		SelectedTools: []string{"read"},
		DocumentationPaths: []SystemPromptDocumentationPath{
			{Label: "Main documentation", Path: "/repo/README.md"},
			{Label: "Protocol overview", Path: "/repo/protocol/README.md", Detail: "extension/package host protocol"},
		},
	})
	for _, expected := range []string{
		"Gi documentation (read only when the user asks about Gi itself",
		"- Main documentation: /repo/README.md",
		"- Protocol overview: /repo/protocol/README.md (extension/package host protocol)",
		"read the relevant docs first",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q:\n%s", expected, prompt)
		}
	}
}

func TestDefaultGiDocumentationPathsFindsRepoRoot(t *testing.T) {
	root := t.TempDir()
	writeSystemPromptDocFile(t, filepath.Join(root, "go.mod"), "module github.com/nowa/gi\n")
	writeSystemPromptDocFile(t, filepath.Join(root, "README.md"), "# Gi\n")
	writeSystemPromptDocFile(t, filepath.Join(root, "PI_COMPATIBILITY.md"), "# Compatibility\n")
	writeSystemPromptDocFile(t, filepath.Join(root, "protocol", "README.md"), "# Protocol\n")
	writeSystemPromptDocFile(t, filepath.Join(root, "protocol", "spec", "gi-extension-protocol.md"), "# Spec\n")
	nested := filepath.Join(root, "examples", "project")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	paths := defaultGiDocumentationPaths(nested)
	if len(paths) != 4 {
		t.Fatalf("paths = %#v", paths)
	}
	if paths[0].Label != "Main documentation" || paths[0].Path != filepath.ToSlash(filepath.Join(root, "README.md")) {
		t.Fatalf("main doc path = %#v", paths[0])
	}
}

func writeSystemPromptDocFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
