package gicodingagent

import "github.com/nowa/gi/gi-coding-agent/internal/diffrender"

func RenderDiff(diffText string) string {
	return diffrender.Render(diffText, diffrender.Theme{
		Context: func(text string) string { return tuiThemeFG("toolDiffContext", text) },
		Removed: func(text string) string { return tuiThemeFG("toolDiffRemoved", text) },
		Added:   func(text string) string { return tuiThemeFG("toolDiffAdded", text) },
		Inverse: tuiThemeInverse,
	})
}

func tuiThemeInverse(text string) string {
	if text == "" {
		return text
	}
	return "\x1b[7m" + text + "\x1b[27m"
}
