package harness

import (
	"strings"
	"testing"
)

func TestFormatSkillInvocationWithAdditionalInstructions(t *testing.T) {
	skill := Skill{
		Name:        "inspect",
		Description: "Inspect things",
		Content:     "Use inspection tools.",
		FilePath:    "/project/.pi/skills/inspect/SKILL.md",
	}
	got := FormatSkillInvocation(skill, "Check errors.")
	want := "<skill name=\"inspect\" location=\"/project/.pi/skills/inspect/SKILL.md\">\nReferences are relative to /project/.pi/skills/inspect.\n\nUse inspection tools.\n</skill>\n\nCheck errors."
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
