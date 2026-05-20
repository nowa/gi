package gicodingagent

import (
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
