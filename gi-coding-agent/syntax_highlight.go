package gicodingagent

import "github.com/nowa/gi/gi-coding-agent/internal/syntaxhighlight"

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
