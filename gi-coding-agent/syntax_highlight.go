package gicodingagent

import (
	"html"
	"regexp"
	"strings"
)

type HighlightTheme map[string]func(string) string

type HighlightOptions struct {
	Language       string
	IgnoreIllegals bool
	Theme          HighlightTheme
}

var spanTagPattern = regexp.MustCompile(`(?is)<span\s+class="([^"]*)">|</span>`)

func RenderHighlightedHTML(input string, theme HighlightTheme) string {
	var output strings.Builder
	stack := []func(string) string{nil}
	last := 0
	matches := spanTagPattern.FindAllStringSubmatchIndex(input, -1)
	for _, match := range matches {
		if match[0] > last {
			writeHighlightedText(&output, input[last:match[0]], stack[len(stack)-1])
		}
		tag := input[match[0]:match[1]]
		if strings.EqualFold(tag, "</span>") {
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		} else {
			classValue := input[match[2]:match[3]]
			stack = append(stack, formatterForClasses(classValue, theme, stack[len(stack)-1]))
		}
		last = match[1]
	}
	if last < len(input) {
		writeHighlightedText(&output, input[last:], stack[len(stack)-1])
	}
	return output.String()
}

func Highlight(code string, options HighlightOptions) string {
	if !SupportsLanguage(options.Language) {
		return code
	}
	escaped := html.EscapeString(code)
	keyword := options.Theme["keyword"]
	number := options.Theme["number"]
	if keyword != nil {
		escaped = regexp.MustCompile(`\bconst\b`).ReplaceAllStringFunc(escaped, keyword)
	}
	if number != nil {
		escaped = regexp.MustCompile(`\b\d+\b`).ReplaceAllStringFunc(escaped, number)
	}
	return escaped
}

func SupportsLanguage(language string) bool {
	switch strings.ToLower(language) {
	case "ts", "tsx", "typescript", "js", "jsx", "javascript":
		return true
	default:
		return false
	}
}

func formatterForClasses(classValue string, theme HighlightTheme, inherited func(string) string) func(string) string {
	for _, className := range strings.Fields(classValue) {
		scope := strings.TrimPrefix(className, "hljs-")
		if formatter := theme[scope]; formatter != nil {
			return formatter
		}
	}
	return inherited
}

func writeHighlightedText(output *strings.Builder, text string, formatter func(string) string) {
	decoded := html.UnescapeString(text)
	if formatter != nil {
		output.WriteString(formatter(decoded))
		return
	}
	output.WriteString(decoded)
}
