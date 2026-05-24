package gicodingagent

import (
	"runtime"
	"strings"

	gitui "github.com/nowa/gi/gi-tui"
)

type LoginDialogComponent struct {
	focus        gitui.FocusState
	title        string
	message      string
	input        *gitui.Input
	inputVisible bool
	infoLines    []string
	authURL      string
	instructions string
	manualPrompt string
	OnSubmit     func(value string)
	OnCancel     func()
}

func NewLoginDialogComponent(title, message string) *LoginDialogComponent {
	input := gitui.NewInput()
	component := &LoginDialogComponent{
		title:   strings.TrimSpace(title),
		message: strings.TrimSpace(message),
		input:   input,
	}
	component.inputVisible = true
	input.OnSubmit = func(value string) {
		if component.OnSubmit != nil {
			component.OnSubmit(value)
		}
	}
	input.OnEscape = func() {
		if component.OnCancel != nil {
			component.OnCancel()
		}
	}
	return component
}

func (c *LoginDialogComponent) Focused() bool {
	if c == nil {
		return false
	}
	return c.focus.Focused()
}

func (c *LoginDialogComponent) SetFocused(focused bool) {
	if c == nil {
		return
	}
	c.focus.SetFocused(focused)
	if c.input != nil {
		c.input.SetFocused(focused)
	}
}

func (c *LoginDialogComponent) Invalidate() {
	if c != nil && c.input != nil {
		c.input.Invalidate()
	}
}

func (c *LoginDialogComponent) ShowInfo(lines []string) {
	if c == nil {
		return
	}
	c.inputVisible = false
	c.infoLines = append([]string(nil), lines...)
	c.authURL = ""
	c.instructions = ""
	c.manualPrompt = ""
}

func (c *LoginDialogComponent) ShowAuth(url, instructions, manualPrompt string) {
	if c == nil {
		return
	}
	c.authURL = strings.TrimSpace(url)
	c.instructions = strings.TrimSpace(instructions)
	c.manualPrompt = strings.TrimSpace(manualPrompt)
	c.infoLines = nil
	c.inputVisible = true
	if c.input != nil {
		c.input.SetText("")
	}
}

func (c *LoginDialogComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	width = max(24, width)
	title := firstNonEmptyString(c.title, "Login")
	lines := []string{
		selectorDynamicBorder(width),
		selectorTextLine(tuiThemeBoldAccent(title), width),
	}
	if c.authURL != "" {
		lines = append(lines, "")
		lines = append(lines, selectorTextLines(tuiThemeAccent(terminalHyperlink(c.authURL, c.authURL)), width)...)
		lines = append(lines, selectorTextLines(tuiThemeDim(terminalHyperlink(c.authURL, oauthClickHint())), width)...)
		if c.instructions != "" {
			lines = append(lines, "")
			lines = append(lines, selectorTextLines(tuiThemeWarning(c.instructions), width)...)
		}
		if c.manualPrompt != "" {
			lines = append(lines, "")
			lines = append(lines, selectorTextLines(tuiThemeDim(c.manualPrompt), width)...)
		}
		if c.inputVisible && c.input != nil {
			lines = append(lines, c.input.Render(width)...)
		}
		lines = append(lines,
			selectorTextLine("("+tuiThemeKeyHint(selectorCancelKeyHint(), "to cancel")+")", width),
			selectorDynamicBorder(width),
		)
		return lines
	}
	if len(c.infoLines) > 0 {
		lines = append(lines, "")
		for _, line := range c.infoLines {
			lines = append(lines, selectorTextLines(line, width)...)
		}
		lines = append(lines,
			"",
			selectorTextLine("("+tuiThemeKeyHint(selectorCancelKeyHint(), "to close")+")", width),
			selectorDynamicBorder(width),
		)
		return lines
	}
	if c.message != "" {
		lines = append(lines, "", selectorTextLine(tuiThemeFG("text", c.message), width))
	}
	if c.inputVisible && c.input != nil {
		lines = append(lines, c.input.Render(width)...)
	}
	lines = append(lines,
		selectorTextLine("("+tuiThemeKeyHint(selectorCancelKeyHint(), "to cancel,")+" "+tuiThemeKeyHint(firstNonEmptyString(formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.select.confirm"), false), "enter"), "to submit")+")", width),
		selectorDynamicBorder(width),
	)
	return lines
}

func (c *LoginDialogComponent) HandleInput(input string) {
	if c == nil {
		return
	}
	if gitui.GetKeybindings().Matches(input, "tui.select.cancel") || input == "\x03" {
		if c.OnCancel != nil {
			c.OnCancel()
		}
		return
	}
	if c.inputVisible && c.input != nil {
		c.input.HandleInput(input)
	}
}

func terminalHyperlink(url, label string) string {
	if strings.TrimSpace(url) == "" {
		return label
	}
	return "\x1b]8;;" + url + "\x07" + label + "\x1b]8;;\x07"
}

func oauthClickHint() string {
	if runtime.GOOS == "darwin" {
		return "Cmd+click to open"
	}
	return "Ctrl+click to open"
}
