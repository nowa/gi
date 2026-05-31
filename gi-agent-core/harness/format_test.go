package harness

import (
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestFormatSkillInvocationWithAdditionalInstructions(t *testing.T) {
	skill := Skill{
		Name:        "inspect",
		Description: "Inspect things",
		Content:     "Use inspection tools.",
		FilePath:    "/project/.gi/skills/inspect/SKILL.md",
	}
	got := FormatSkillInvocation(skill, "Check errors.")
	want := "<skill name=\"inspect\" location=\"/project/.gi/skills/inspect/SKILL.md\">\nReferences are relative to /project/.gi/skills/inspect.\n\nUse inspection tools.\n</skill>\n\nCheck errors."
	if got != want {
		t.Fatalf("FormatSkillInvocation() = %q, want %q", got, want)
	}
}

func TestFormatPromptTemplateInvocation(t *testing.T) {
	got := FormatPromptTemplateInvocation(PromptTemplate{Name: "review", Content: "Review $1 with $ARGUMENTS"}, []string{"a.ts", "care"})
	if got != "Review a.ts with a.ts care" {
		t.Fatalf("FormatPromptTemplateInvocation() = %q", got)
	}

	content := "$1 ${@:2} $ARGUMENTS"
	got = FormatPromptTemplateInvocation(PromptTemplate{Name: "one", Content: content}, []string{"hello world", "test"})
	if got != "hello world test hello world test" {
		t.Fatalf("FormatPromptTemplateInvocation() = %q", got)
	}
}

func TestFormatSkillsForSystemPrompt(t *testing.T) {
	visible := Skill{Name: "visible", Description: "Use <this> & that", Content: "visible content", FilePath: "/skills/visible/SKILL.md"}
	second := Skill{Name: "second", Description: "Second skill", Content: "second content", FilePath: "/skills/second/SKILL.md"}
	disabled := Skill{Name: "hidden", Description: "Hidden", Content: "hidden content", FilePath: "/skills/hidden/SKILL.md", DisableModelInvocation: true}
	got := FormatSkillsForSystemPrompt([]Skill{visible, disabled, second})
	if !strings.Contains(got, "<name>visible</name>") || !strings.Contains(got, "<name>second</name>") || strings.Contains(got, "<name>hidden</name>") {
		t.Fatalf("formatted skills = %s", got)
	}
	if !strings.Contains(got, "<description>Use &lt;this&gt; &amp; that</description>") {
		t.Fatalf("formatted skills did not escape XML: %s", got)
	}
	if got := FormatSkillsForSystemPrompt([]Skill{disabled}); got != "" {
		t.Fatalf("disabled-only skills = %q, want empty", got)
	}
}

func TestFormatSkillsForSystemPromptEscapesAllVisibleFields(t *testing.T) {
	got := FormatSkillsForSystemPrompt([]Skill{{
		Name:        "a&b",
		Description: `Quote "double" and 'single'`,
		Content:     "content",
		FilePath:    `/skills/<bad>&"quote"/SKILL.md`,
	}})
	want := "<name>a&amp;b</name>\n    <description>Quote &quot;double&quot; and &apos;single&apos;</description>\n    <location>/skills/&lt;bad&gt;&amp;&quot;quote&quot;/SKILL.md</location>"
	if !strings.Contains(got, want) {
		t.Fatalf("formatted skills = %s, want contains %s", got, want)
	}
}

func TestBashExecutionTextPiStyle(t *testing.T) {
	exitCode := 2
	got := BashExecutionText("go test ./...", BashExecutionTextOptions{
		Output:         "failed\n",
		ExitCode:       &exitCode,
		Truncated:      true,
		FullOutputPath: "/tmp/full.log",
	})
	want := "Ran `go test ./...`\n```\nfailed\n```\n\nCommand exited with code 2\n\n[Output truncated. Full output: /tmp/full.log]"
	if got != want {
		t.Fatalf("bash text = %q, want %q", got, want)
	}

	got = BashExecutionText("sleep 10", BashExecutionTextOptions{Cancelled: true})
	if got != "Ran `sleep 10`\n(no output)\n\n(command cancelled)" {
		t.Fatalf("cancelled bash text = %q", got)
	}
}

func TestConvertToLLMPiSyntheticMessages(t *testing.T) {
	display := true
	messages := []llm.Message{
		{Role: "custom", CustomType: "note", Display: &display, Details: map[string]any{"a": 1}, Content: []llm.ContentPart{llm.Text("custom context")}, Timestamp: 10},
		{Role: "branchSummary", Details: map[string]any{"fromId": "entry-1"}, Content: []llm.ContentPart{llm.Text("branch context")}, Timestamp: 20},
		{Role: "compactionSummary", Details: map[string]any{"tokensBefore": 123}, Content: []llm.ContentPart{llm.Text("compact context")}, Timestamp: 30},
		{Role: "bashExecution", Details: map[string]any{"excludeFromContext": true}, Content: []llm.ContentPart{llm.Text("hidden bash")}, Timestamp: 40},
		{Role: "bashExecution", Content: []llm.ContentPart{llm.Text("bash context")}, Timestamp: 50},
		{Role: "ignored", Content: []llm.ContentPart{llm.Text("ignored")}, Timestamp: 60},
		{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("assistant")}, Timestamp: 70},
	}

	got := ConvertToLLM(messages)
	if len(got) != 5 {
		t.Fatalf("messages = %#v", got)
	}
	for index, message := range got[:4] {
		if message.Role != llm.RoleUser {
			t.Fatalf("message %d role = %q, want user", index, message.Role)
		}
		if message.CustomType != "" || message.Display != nil || message.Details != nil {
			t.Fatalf("message %d retained synthetic fields: %#v", index, message)
		}
	}
	if text := messageTextContent(got[1]); !strings.Contains(text, "summary of a branch") || !strings.Contains(text, "branch context") {
		t.Fatalf("branch summary text = %q", text)
	}
	if text := messageTextContent(got[2]); !strings.Contains(text, "conversation history before this point") || !strings.Contains(text, "compact context") {
		t.Fatalf("compaction summary text = %q", text)
	}
	if got[4].Role != llm.RoleAssistant || messageTextContent(got[4]) != "assistant" {
		t.Fatalf("assistant message = %#v", got[4])
	}
}

func TestPromptTemplateAndSystemPromptPiCaseNames(t *testing.T) {
	t.Run("substitutes command arguments", func(t *testing.T) {
		got := FormatPromptTemplateInvocation(PromptTemplate{Name: "review", Content: "Review $1 with ${@:2} / $ARGUMENTS"}, []string{"a.ts", "care"})
		if got != "Review a.ts with care / a.ts care" {
			t.Fatalf("prompt template = %q", got)
		}
	})

	t.Run("formats visible skills in order and skips model-disabled skills", func(t *testing.T) {
		visible := Skill{Name: "visible", Description: "Visible skill", Content: "visible content", FilePath: "/skills/visible/SKILL.md"}
		second := Skill{Name: "second", Description: "Second skill", Content: "second content", FilePath: "/skills/second/SKILL.md"}
		disabled := Skill{Name: "hidden", Description: "Hidden", Content: "hidden content", FilePath: "/skills/hidden/SKILL.md", DisableModelInvocation: true}
		got := FormatSkillsForSystemPrompt([]Skill{visible, disabled, second})
		visibleIndex := strings.Index(got, "<name>visible</name>")
		secondIndex := strings.Index(got, "<name>second</name>")
		if visibleIndex < 0 || secondIndex < 0 || visibleIndex > secondIndex || strings.Contains(got, "<name>hidden</name>") {
			t.Fatalf("formatted skills = %s", got)
		}
	})

	t.Run("returns an empty string when no skills are model-visible", func(t *testing.T) {
		got := FormatSkillsForSystemPrompt([]Skill{{Name: "hidden", DisableModelInvocation: true}})
		if got != "" {
			t.Fatalf("formatted skills = %q", got)
		}
	})

	t.Run("escapes XML in all model-visible skill fields", func(t *testing.T) {
		got := FormatSkillsForSystemPrompt([]Skill{{
			Name:        "a&b",
			Description: `Quote "double" and 'single'`,
			Content:     "content",
			FilePath:    `/skills/<bad>&"quote"/SKILL.md`,
		}})
		for _, want := range []string{"a&amp;b", "&quot;double&quot;", "&apos;single&apos;", "/skills/&lt;bad&gt;&amp;&quot;quote&quot;/SKILL.md"} {
			if !strings.Contains(got, want) {
				t.Fatalf("formatted skills missing %q in %s", want, got)
			}
		}
	})
}
