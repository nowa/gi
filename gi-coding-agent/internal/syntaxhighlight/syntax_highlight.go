package syntaxhighlight

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
var simpleHighlightTokenPattern = regexp.MustCompile(`\bconst\b|\b\d+\b`)
var decimalTokenPattern = regexp.MustCompile(`^\d+$`)
var javascriptRegexpPattern = regexp.MustCompile(`/([^/\\\n]|\\.)+/[A-Za-z]*`)
var pythonDecoratorPattern = regexp.MustCompile(`(?m)^([ \t]*)(@[A-Za-z_][A-Za-z0-9_]*)`)
var htmlTagNamePattern = regexp.MustCompile(`(?i)</?([A-Za-z][A-Za-z0-9:-]*)`)

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
	language := strings.ToLower(strings.TrimSpace(options.Language))
	if !SupportsLanguage(language) {
		return code
	}
	switch language {
	case "diff", "patch":
		return highlightDiff(code, options.Theme)
	case "py", "python":
		return highlightPython(code, options.Theme)
	case "html", "xml":
		return highlightHTML(code, options.Theme)
	default:
		return highlightJavaScript(code, options.Theme)
	}
}

func highlightJavaScript(code string, theme HighlightTheme) string {
	var output strings.Builder
	last := 0
	for _, match := range javascriptRegexpPattern.FindAllStringIndex(
		code,
		-1,
	) {
		output.WriteString(highlightSimpleTokens(code[last:match[0]], theme))
		token := html.EscapeString(code[match[0]:match[1]])
		if formatter := theme["regexp"]; formatter != nil {
			token = formatter(token)
		}
		output.WriteString(token)
		last = match[1]
	}
	output.WriteString(highlightSimpleTokens(code[last:], theme))
	return output.String()
}

func highlightSimpleTokens(code string, theme HighlightTheme) string {
	escaped := html.EscapeString(code)
	keyword := theme["keyword"]
	number := theme["number"]
	if keyword == nil && number == nil {
		return escaped
	}
	return simpleHighlightTokenPattern.ReplaceAllStringFunc(escaped, func(token string) string {
		if token == "const" && keyword != nil {
			return keyword(token)
		}
		if number != nil && decimalTokenPattern.MatchString(token) {
			return number(token)
		}
		return token
	})
}

func highlightDiff(code string, theme HighlightTheme) string {
	lines := strings.Split(code, "\n")
	for index, line := range lines {
		escaped := html.EscapeString(line)
		var formatter func(string) string
		switch {
		case strings.HasPrefix(line, "+"):
			formatter = theme["addition"]
		case strings.HasPrefix(line, "-"):
			formatter = theme["deletion"]
		}
		if formatter != nil {
			escaped = formatter(escaped)
		}
		lines[index] = escaped
	}
	return strings.Join(lines, "\n")
}

func highlightPython(code string, theme HighlightTheme) string {
	matches := pythonDecoratorPattern.FindAllStringSubmatchIndex(code, -1)
	if len(matches) == 0 {
		return html.EscapeString(code)
	}
	var output strings.Builder
	last := 0
	for _, match := range matches {
		decoratorStart, decoratorEnd := match[4], match[5]
		output.WriteString(html.EscapeString(code[last:decoratorStart]))
		decorator := html.EscapeString(code[decoratorStart:decoratorEnd])
		if formatter := theme["meta"]; formatter != nil {
			decorator = formatter(decorator)
		}
		output.WriteString(decorator)
		last = decoratorEnd
	}
	output.WriteString(html.EscapeString(code[last:]))
	return output.String()
}

func highlightHTML(code string, theme HighlightTheme) string {
	matches := htmlTagNamePattern.FindAllStringSubmatchIndex(code, -1)
	if len(matches) == 0 {
		return html.EscapeString(code)
	}
	var output strings.Builder
	last := 0
	for _, match := range matches {
		nameStart, nameEnd := match[2], match[3]
		output.WriteString(html.EscapeString(code[last:nameStart]))
		name := html.EscapeString(code[nameStart:nameEnd])
		if formatter := theme["name"]; formatter != nil {
			name = formatter(name)
		}
		output.WriteString(name)
		last = nameEnd
	}
	output.WriteString(html.EscapeString(code[last:]))
	return output.String()
}

func SupportsLanguage(language string) bool {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "ts", "tsx", "typescript", "js", "jsx", "javascript",
		"diff", "patch", "py", "python", "html", "xml":
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
