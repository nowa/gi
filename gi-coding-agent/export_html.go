package gicodingagent

import (
	"html"
	"regexp"
	"strings"
)

const ExportHTMLTemplateCSS = `
.output-preview > div:not(.expand-hint),
.output-full > div:not(.expand-hint) {
  white-space: pre-wrap;
}

.ansi-line {
  white-space: pre;
}
`

func AnsiLinesToHTML(lines []string) string {
	var builder strings.Builder
	for _, line := range lines {
		builder.WriteString(`<div class="ansi-line">`)
		builder.WriteString(ansiLineToHTML(line))
		builder.WriteString(`</div>`)
	}
	return builder.String()
}

func RenderCustomToolResultHTML(lines []string) string {
	start := 0
	for start < len(lines) && lines[start] == "" {
		start++
	}
	end := len(lines)
	for end > start && lines[end-1] == "" {
		end--
	}
	return AnsiLinesToHTML(lines[start:end])
}

type ExportHTMLSkillBlock struct {
	Name        string
	Location    string
	Content     string
	UserMessage string
}

type ExportHTMLSidebarEntry struct {
	Role  string
	Label string
}

var exportHTMLSkillBlockPattern = regexp.MustCompile(`(?s)^<skill\s+([^>]*)>\s*\n?(.*?)\n?</skill>\s*(.*)$`)
var exportHTMLSkillAttrPattern = regexp.MustCompile(`([A-Za-z_:][A-Za-z0-9_.:-]*)="([^"]*)"`)

func ParseExportHTMLSkillBlock(text string) (ExportHTMLSkillBlock, bool) {
	match := exportHTMLSkillBlockPattern.FindStringSubmatch(text)
	if match == nil {
		return ExportHTMLSkillBlock{}, false
	}
	attrs := parseExportHTMLSkillAttrs(match[1])
	name := attrs["name"]
	if name == "" {
		return ExportHTMLSkillBlock{}, false
	}
	return ExportHTMLSkillBlock{
		Name:        name,
		Location:    attrs["location"],
		Content:     strings.TrimSpace(match[2]),
		UserMessage: strings.TrimSpace(match[3]),
	}, true
}

func RenderExportHTMLUserMessage(text string) string {
	skillBlock, ok := ParseExportHTMLSkillBlock(text)
	if !ok {
		return `<div class="user-message">` + html.EscapeString(text) + `</div>`
	}
	var builder strings.Builder
	builder.WriteString(`<div class="skill-invocation"><div class="skill-name">`)
	builder.WriteString(html.EscapeString(skillBlock.Name))
	builder.WriteString(`</div><div class="skill-content">`)
	builder.WriteString(renderExportHTMLMarkdown(skillBlock.Content))
	builder.WriteString(`</div></div>`)
	if skillBlock.UserMessage != "" {
		builder.WriteString(`<div class="user-message">`)
		builder.WriteString(html.EscapeString(skillBlock.UserMessage))
		builder.WriteString(`</div>`)
	}
	return builder.String()
}

func ExportHTMLSidebarEntriesForUserMessage(text string) []ExportHTMLSidebarEntry {
	skillBlock, ok := ParseExportHTMLSkillBlock(text)
	if !ok {
		return []ExportHTMLSidebarEntry{{Role: "tree-role-user", Label: text}}
	}
	entries := []ExportHTMLSidebarEntry{{Role: "tree-role-skill", Label: skillBlock.Name}}
	if skillBlock.UserMessage != "" {
		entries = append(entries, ExportHTMLSidebarEntry{Role: "tree-role-user", Label: skillBlock.UserMessage})
	}
	return entries
}

func parseExportHTMLSkillAttrs(raw string) map[string]string {
	attrs := map[string]string{}
	for _, match := range exportHTMLSkillAttrPattern.FindAllStringSubmatch(raw, -1) {
		attrs[match[1]] = html.UnescapeString(match[2])
	}
	return attrs
}

func renderExportHTMLMarkdown(markdown string) string {
	escaped := html.EscapeString(markdown)
	boldPattern := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	return boldPattern.ReplaceAllString(escaped, `<strong>$1</strong>`)
}

func ansiLineToHTML(line string) string {
	const redStart = "\x1b[31m"
	const reset = "\x1b[0m"
	var builder strings.Builder
	red := false
	for len(line) > 0 {
		switch {
		case strings.HasPrefix(line, redStart):
			if red {
				builder.WriteString(`</span>`)
			}
			builder.WriteString(`<span style="color:#800000">`)
			red = true
			line = strings.TrimPrefix(line, redStart)
		case strings.HasPrefix(line, reset):
			if red {
				builder.WriteString(`</span>`)
				red = false
			}
			line = strings.TrimPrefix(line, reset)
		default:
			next := strings.IndexByte(line, '\x1b')
			if next < 0 {
				next = len(line)
			}
			builder.WriteString(html.EscapeString(line[:next]))
			line = line[next:]
		}
	}
	if red {
		builder.WriteString(`</span>`)
	}
	return builder.String()
}
