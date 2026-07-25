package gicodingagent

import (
	"strings"

	"github.com/nowa/gi/gi-coding-agent/internal/syntaxhighlight"
)

type HighlightTheme = syntaxhighlight.HighlightTheme
type HighlightOptions = syntaxhighlight.HighlightOptions

func RenderHighlightedHTML(input string, theme HighlightTheme) string {
	return syntaxhighlight.RenderHighlightedHTML(input, theme)
}

func Highlight(code string, options HighlightOptions) string {
	return syntaxhighlight.Highlight(code, options)
}

func SupportsLanguage(language string) bool {
	return syntaxhighlight.SupportsLanguage(language)
}

func highlightCode(code, language string) []string {
	if !SupportsLanguage(language) {
		lines := strings.Split(code, "\n")
		for index, line := range lines {
			lines[index] = tuiThemeFG("mdCodeBlock", line)
		}
		return lines
	}
	return strings.Split(
		RenderHighlightedHTML(
			Highlight(code, HighlightOptions{
				Language:       language,
				IgnoreIllegals: true,
				Theme:          buildCLIHighlightTheme(),
			}),
			nil,
		),
		"\n",
	)
}

func buildCLIHighlightTheme() HighlightTheme {
	return HighlightTheme{
		"keyword":     func(text string) string { return tuiThemeFG("syntaxKeyword", text) },
		"built_in":    func(text string) string { return tuiThemeFG("syntaxType", text) },
		"literal":     func(text string) string { return tuiThemeFG("syntaxNumber", text) },
		"number":      func(text string) string { return tuiThemeFG("syntaxNumber", text) },
		"regexp":      func(text string) string { return tuiThemeFG("syntaxString", text) },
		"string":      func(text string) string { return tuiThemeFG("syntaxString", text) },
		"comment":     func(text string) string { return tuiThemeFG("syntaxComment", text) },
		"doctag":      func(text string) string { return tuiThemeFG("syntaxComment", text) },
		"meta":        func(text string) string { return tuiThemeFG("muted", text) },
		"function":    func(text string) string { return tuiThemeFG("syntaxFunction", text) },
		"title":       func(text string) string { return tuiThemeFG("syntaxFunction", text) },
		"class":       func(text string) string { return tuiThemeFG("syntaxType", text) },
		"type":        func(text string) string { return tuiThemeFG("syntaxType", text) },
		"tag":         func(text string) string { return tuiThemeFG("syntaxPunctuation", text) },
		"name":        func(text string) string { return tuiThemeFG("syntaxKeyword", text) },
		"attr":        func(text string) string { return tuiThemeFG("syntaxVariable", text) },
		"variable":    func(text string) string { return tuiThemeFG("syntaxVariable", text) },
		"params":      func(text string) string { return tuiThemeFG("syntaxVariable", text) },
		"operator":    func(text string) string { return tuiThemeFG("syntaxOperator", text) },
		"punctuation": func(text string) string { return tuiThemeFG("syntaxPunctuation", text) },
		"emphasis":    tuiThemeItalic,
		"strong":      tuiThemeBold,
		"link": func(text string) string {
			return "\x1b[4m" + text + "\x1b[24m"
		},
		"addition": func(text string) string {
			return tuiThemeFG("toolDiffAdded", text)
		},
		"deletion": func(text string) string {
			return tuiThemeFG("toolDiffRemoved", text)
		},
	}
}
