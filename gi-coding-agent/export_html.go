package gicodingagent

import (
	"html"
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
