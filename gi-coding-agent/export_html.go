package gicodingagent

import (
	"errors"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

func DefaultSessionExportHTMLName(inputPath string) string {
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	if strings.TrimSpace(base) == "" {
		base = "session"
	}
	return "gi-session-" + base + ".html"
}

func ExportSessionFileToHTML(inputPath, outputPath string) (string, error) {
	inputPath = strings.TrimSpace(inputPath)
	if inputPath == "" {
		return "", errors.New("session file path is required")
	}
	if _, err := os.Stat(inputPath); err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("File not found: " + inputPath)
		}
		return "", err
	}
	sessionManager, err := OpenSessionManager(inputPath)
	if err != nil {
		return "", err
	}
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		outputPath = DefaultSessionExportHTMLName(inputPath)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(outputPath, []byte(RenderSessionManagerHTML(sessionManager)), 0o644); err != nil {
		return "", err
	}
	return outputPath, nil
}

func RenderSessionManagerHTML(sessionManager *SessionManager) string {
	var builder strings.Builder
	builder.WriteString("<!doctype html><html><body>\n")
	if sessionManager != nil {
		for _, message := range sessionManager.BuildSessionContext().Messages {
			builder.WriteString(`<section data-role="`)
			builder.WriteString(html.EscapeString(messageRole(message)))
			builder.WriteString(`">`)
			builder.WriteString(html.EscapeString(extractMessageText(message)))
			builder.WriteString("</section>\n")
		}
	}
	builder.WriteString("</body></html>\n")
	return builder.String()
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

func RenderExportHTMLMarkdownLink(href, text string) string {
	if !isSafeExportHTMLURL(href) {
		return html.EscapeString(text)
	}
	return `<a href="` + html.EscapeString(href) + `">` + html.EscapeString(text) + `</a>`
}

func RenderExportHTMLMarkdownImage(src, alt string) string {
	if !isSafeExportHTMLURL(src) {
		return ""
	}
	return `<img src="` + html.EscapeString(src) + `" alt="` + html.EscapeString(alt) + `">`
}

func RenderExportHTMLInlineImage(mimeType, data string) string {
	return `<img src="data:` + html.EscapeString(mimeType) + `;base64,` + html.EscapeString(data) + `">`
}

func RenderExportHTMLSessionEntryAttrs(entryID string) string {
	escaped := html.EscapeString(entryID)
	return `id="entry-` + escaped + `" data-entry-id="` + escaped + `"`
}

func RenderExportHTMLTreeMetadata(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if values[key] == "" {
			continue
		}
		parts = append(parts, "["+html.EscapeString(key)+": "+html.EscapeString(values[key])+"]")
	}
	return strings.Join(parts, " ")
}

func RenderExportHTMLHeaderModels(models []string) string {
	if len(models) == 0 {
		return "unknown"
	}
	return html.EscapeString(strings.Join(models, ", "))
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

func isSafeExportHTMLURL(rawURL string) bool {
	normalized := strings.ToLower(strings.TrimSpace(rawURL))
	normalized = strings.ReplaceAll(normalized, "\u0000", "")
	return !strings.HasPrefix(normalized, "javascript:") && !strings.HasPrefix(normalized, "vbscript:")
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
