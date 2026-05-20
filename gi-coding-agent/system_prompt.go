package gicodingagent

import (
	"fmt"
	"strings"
	"time"
)

type SystemPromptContextFile struct {
	Path    string
	Content string
}

type SystemPromptSkill struct {
	Name        string
	Description string
	Content     string
}

type BuildSystemPromptOptions struct {
	CustomPrompt       string
	SelectedTools      []string
	ToolSnippets       map[string]string
	PromptGuidelines   []string
	AppendSystemPrompt string
	CWD                string
	ContextFiles       []SystemPromptContextFile
	Skills             []SystemPromptSkill
	Now                time.Time
}

func BuildSystemPrompt(options BuildSystemPromptOptions) string {
	cwd := strings.ReplaceAll(options.CWD, `\`, `/`)
	if cwd == "" {
		cwd = "."
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	date := now.Format("2006-01-02")
	appendSection := ""
	if options.AppendSystemPrompt != "" {
		appendSection = "\n\n" + options.AppendSystemPrompt
	}

	if options.CustomPrompt != "" {
		prompt := options.CustomPrompt + appendSection
		prompt += formatProjectContext(options.ContextFiles)
		if systemPromptHasRead(options.SelectedTools) && len(options.Skills) > 0 {
			prompt += formatSystemPromptSkills(options.Skills)
		}
		return prompt + "\nCurrent date: " + date + "\nCurrent working directory: " + cwd
	}

	tools := options.SelectedTools
	if tools == nil {
		tools = []string{"read", "bash", "edit", "write"}
	}
	visibleTools := make([]string, 0, len(tools))
	for _, tool := range tools {
		if snippet := strings.TrimSpace(options.ToolSnippets[tool]); snippet != "" {
			visibleTools = append(visibleTools, fmt.Sprintf("- %s: %s", tool, snippet))
		}
	}
	toolsList := "(none)"
	if len(visibleTools) > 0 {
		toolsList = strings.Join(visibleTools, "\n")
	}

	guidelines := buildSystemPromptGuidelines(tools, options.PromptGuidelines)
	prompt := `You are an expert coding assistant operating inside gi, a coding agent harness. You help users by reading files, executing commands, editing code, and writing new files.

Available tools:
` + toolsList + `

In addition to the tools above, you may have access to other custom tools depending on the project.

Guidelines:
` + strings.Join(guidelines, "\n")

	prompt += appendSection
	prompt += formatProjectContext(options.ContextFiles)
	if containsString(tools, "read") && len(options.Skills) > 0 {
		prompt += formatSystemPromptSkills(options.Skills)
	}
	return prompt + "\nCurrent date: " + date + "\nCurrent working directory: " + cwd
}

func buildSystemPromptGuidelines(tools []string, extra []string) []string {
	var guidelines []string
	seen := map[string]struct{}{}
	add := func(guideline string) {
		normalized := strings.TrimSpace(guideline)
		if normalized == "" {
			return
		}
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		guidelines = append(guidelines, "- "+normalized)
	}
	hasBash := containsString(tools, "bash")
	hasGrep := containsString(tools, "grep")
	hasFind := containsString(tools, "find")
	hasLs := containsString(tools, "ls")
	if hasBash && !hasGrep && !hasFind && !hasLs {
		add("Use bash for file operations like ls, rg, find")
	} else if hasBash && (hasGrep || hasFind || hasLs) {
		add("Prefer grep/find/ls tools over bash for file exploration (faster, respects .gitignore)")
	}
	for _, guideline := range extra {
		add(guideline)
	}
	add("Be concise in your responses")
	add("Show file paths clearly when working with files")
	return guidelines
}

func systemPromptHasRead(selectedTools []string) bool {
	return selectedTools == nil || containsString(selectedTools, "read")
}

func formatProjectContext(files []SystemPromptContextFile) string {
	if len(files) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n# Project Context\n\nProject-specific instructions and guidelines:\n\n")
	for _, file := range files {
		b.WriteString("## ")
		b.WriteString(file.Path)
		b.WriteString("\n\n")
		b.WriteString(file.Content)
		b.WriteString("\n\n")
	}
	return b.String()
}

func formatSystemPromptSkills(skills []SystemPromptSkill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n# Skills\n")
	for _, skill := range skills {
		b.WriteString("\n## ")
		b.WriteString(skill.Name)
		if skill.Description != "" {
			b.WriteString("\n")
			b.WriteString(skill.Description)
		}
		if skill.Content != "" {
			b.WriteString("\n\n")
			b.WriteString(skill.Content)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
