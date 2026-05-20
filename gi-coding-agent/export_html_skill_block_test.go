package gicodingagent

import (
	"strings"
	"testing"
)

func TestExportHTMLPiSkillBlockStripsWrapper(t *testing.T) {
	message := "<skill name=\"review\" location=\"/skills/review/SKILL.md\">\nUse **care**.\n</skill>\n\nCheck this diff"
	html := RenderExportHTMLUserMessage(message)
	if strings.Contains(html, "<skill") || strings.Contains(html, "</skill>") {
		t.Fatalf("rendered skill wrapper leaked: %s", html)
	}
	if !strings.Contains(html, "Check this diff") {
		t.Fatalf("rendered user message missing: %s", html)
	}
}

func TestExportHTMLPiSkillBlockSiblingRendering(t *testing.T) {
	message := "<skill name=\"review\" location=\"/skills/review/SKILL.md\">\nBody\n</skill>\n\nCheck this diff"
	html := RenderExportHTMLUserMessage(message)
	skillIndex := strings.Index(html, `class="skill-invocation"`)
	userIndex := strings.Index(html, `class="user-message"`)
	if skillIndex < 0 || userIndex < 0 || skillIndex > userIndex {
		t.Fatalf("skill/user blocks not rendered as ordered siblings: %s", html)
	}

	noPrompt := RenderExportHTMLUserMessage("<skill name=\"review\" location=\"/skills/review/SKILL.md\">\nBody\n</skill>")
	if strings.Contains(noPrompt, `class="user-message"`) {
		t.Fatalf("empty user prompt should omit user-message block: %s", noPrompt)
	}
}

func TestExportHTMLPiSkillContentUsesMarkdown(t *testing.T) {
	message := "<skill name=\"review\" location=\"/skills/review/SKILL.md\">\nUse **care**.\n</skill>\n\nCheck this diff"
	html := RenderExportHTMLUserMessage(message)
	if !strings.Contains(html, "<strong>care</strong>") || strings.Contains(html, "**care**") {
		t.Fatalf("skill markdown was not rendered: %s", html)
	}
}

func TestExportHTMLPiSkillSidebarEntries(t *testing.T) {
	message := "<skill name=\"review\" location=\"/skills/review/SKILL.md\">\nBody\n</skill>\n\nCheck this diff"
	entries := ExportHTMLSidebarEntriesForUserMessage(message)
	if len(entries) != 2 {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[0].Role != "tree-role-skill" || entries[0].Label != "review" {
		t.Fatalf("skill entry = %#v", entries[0])
	}
	if entries[1].Role != "tree-role-user" || entries[1].Label != "Check this diff" {
		t.Fatalf("user entry = %#v", entries[1])
	}
}
