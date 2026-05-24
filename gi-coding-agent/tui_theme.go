package gicodingagent

import gitui "github.com/nowa/gi/gi-tui"

const (
	tuiResetAll = "\x1b[0m"
	tuiResetFG  = "\x1b[39m"
)

var tuiDarkThemeFG = map[string]string{
	"accent":             "\x1b[38;2;138;190;183m",
	"border":             "\x1b[38;2;95;135;255m",
	"borderAccent":       "\x1b[38;2;0;215;255m",
	"borderMuted":        "\x1b[38;2;80;80;80m",
	"success":            "\x1b[38;2;181;189;104m",
	"error":              "\x1b[38;2;204;102;102m",
	"warning":            "\x1b[38;2;255;255;0m",
	"muted":              "\x1b[38;2;128;128;128m",
	"dim":                "\x1b[38;2;102;102;102m",
	"text":               "\x1b[38;2;212;212;212m",
	"thinkingText":       "\x1b[38;2;128;128;128m",
	"customMessageLabel": "\x1b[38;2;149;117;205m",
	"mdHeading":          "\x1b[38;2;240;198;116m",
	"mdLink":             "\x1b[38;2;129;162;190m",
	"mdLinkUrl":          "\x1b[38;2;102;102;102m",
	"mdCode":             "\x1b[38;2;138;190;183m",
	"mdCodeBlock":        "\x1b[38;2;181;189;104m",
	"mdCodeBlockBorder":  "\x1b[38;2;128;128;128m",
	"mdQuote":            "\x1b[38;2;128;128;128m",
	"mdQuoteBorder":      "\x1b[38;2;128;128;128m",
	"mdHr":               "\x1b[38;2;128;128;128m",
	"mdListBullet":       "\x1b[38;2;138;190;183m",
}

func tuiThemeFG(color, text string) string {
	if text == "" {
		return text
	}
	prefix, ok := tuiDarkThemeFG[color]
	if !ok {
		return text
	}
	return prefix + text + tuiResetFG
}

func tuiThemeBold(text string) string {
	if text == "" {
		return text
	}
	return "\x1b[1m" + text + tuiResetAll
}

func tuiThemeBoldAccent(text string) string {
	if text == "" {
		return text
	}
	return "\x1b[1m" + tuiDarkThemeFG["accent"] + text + tuiResetAll
}

func tuiThemeDim(text string) string {
	return tuiThemeFG("dim", text)
}

func tuiThemeMuted(text string) string {
	return tuiThemeFG("muted", text)
}

func tuiThemeAccent(text string) string {
	return tuiThemeFG("accent", text)
}

func tuiThemeWarning(text string) string {
	return tuiThemeFG("warning", text)
}

func tuiThemeError(text string) string {
	return tuiThemeFG("error", text)
}

func tuiThemeSuccess(text string) string {
	return tuiThemeFG("success", text)
}

func tuiThemeBorder(text string) string {
	return tuiThemeFG("border", text)
}

func tuiThemeBorderMuted(text string) string {
	return tuiThemeFG("borderMuted", text)
}

func tuiThemeKeyHint(key, description string) string {
	if description == "" {
		return tuiThemeDim(key)
	}
	return tuiThemeDim(key) + tuiThemeMuted(" "+description)
}

func tuiThemeSelectList() gitui.SelectListTheme {
	return gitui.SelectListTheme{
		SelectedPrefix: tuiThemeAccent,
		SelectedText:   tuiThemeAccent,
		Description:    tuiThemeMuted,
		ScrollInfo:     tuiThemeMuted,
		NoMatch:        tuiThemeMuted,
	}
}

func tuiThemeEditor() gitui.EditorTheme {
	return gitui.EditorTheme{
		Border:     tuiThemeBorderMuted,
		SelectList: tuiThemeSelectList(),
	}
}

func tuiThemeSettingsList() gitui.SettingsListTheme {
	return gitui.SettingsListTheme{
		Label: func(text string, selected bool) string {
			if selected {
				return tuiThemeAccent(text)
			}
			return text
		},
		CurrentValue: func(text string, selected bool) string {
			if selected {
				return tuiThemeAccent(text)
			}
			return tuiThemeMuted(text)
		},
		Description: tuiThemeDim,
		Hint:        tuiThemeDim,
		Selected:    tuiThemeAccent,
		Value:       tuiThemeMuted,
		Cursor:      tuiThemeAccent("→ "),
	}
}

func tuiThemeMarkdown() gitui.MarkdownTheme {
	return gitui.MarkdownTheme{
		Heading:         func(text string) string { return tuiThemeFG("mdHeading", text) },
		Link:            func(text string) string { return tuiThemeFG("mdLink", text) },
		LinkURL:         func(text string) string { return tuiThemeFG("mdLinkUrl", text) },
		Code:            func(text string) string { return tuiThemeFG("mdCode", text) },
		CodeBlock:       func(text string) string { return tuiThemeFG("mdCodeBlock", text) },
		CodeBlockBorder: func(text string) string { return tuiThemeFG("mdCodeBlockBorder", text) },
		Quote:           func(text string) string { return tuiThemeFG("mdQuote", text) },
		QuoteBorder:     func(text string) string { return tuiThemeFG("mdQuoteBorder", text) },
		HR:              func(text string) string { return tuiThemeFG("mdHr", text) },
		ListBullet:      func(text string) string { return tuiThemeFG("mdListBullet", text) },
		Bold:            tuiThemeBold,
		Italic:          func(text string) string { return "\x1b[3m" + text + "\x1b[23m" },
		Underline:       func(text string) string { return "\x1b[4m" + text + "\x1b[24m" },
		Strikethrough:   func(text string) string { return "\x1b[9m" + text + "\x1b[29m" },
	}
}
