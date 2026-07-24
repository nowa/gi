package gicodingagent

import (
	"runtime"
	"strings"
	"sync"

	gitui "github.com/nowa/gi/gi-tui"
)

type LoginDialogComponent struct {
	mu           sync.RWMutex
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
	c.mu.Lock()
	defer c.mu.Unlock()
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
	c.mu.Lock()
	c.authURL = strings.TrimSpace(url)
	c.instructions = strings.TrimSpace(instructions)
	c.manualPrompt = strings.TrimSpace(manualPrompt)
	c.infoLines = nil
	c.inputVisible = c.manualPrompt != ""
	input := c.input
	c.mu.Unlock()
	if input != nil {
		input.SetText("")
	}
}

func (c *LoginDialogComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	titleValue := c.title
	message := c.message
	input := c.input
	inputVisible := c.inputVisible
	infoLines := append([]string(nil), c.infoLines...)
	authURL := c.authURL
	instructions := c.instructions
	manualPrompt := c.manualPrompt
	c.mu.RUnlock()

	width = max(24, width)
	title := firstNonEmptyString(titleValue, "Login")
	lines := []string{
		selectorDynamicBorder(width),
		selectorTextLine(tuiThemeBoldAccent(title), width),
	}
	if authURL != "" {
		lines = append(lines, "")
		lines = append(lines, selectorTextLines(tuiThemeAccent(terminalHyperlink(authURL, authURL)), width)...)
		lines = append(lines, selectorTextLines(tuiThemeDim(terminalHyperlink(authURL, oauthClickHint())), width)...)
		if instructions != "" {
			lines = append(lines, "")
			lines = append(lines, selectorTextLines(tuiThemeWarning(instructions), width)...)
		}
		if manualPrompt != "" {
			lines = append(lines, "")
			lines = append(lines, selectorTextLines(tuiThemeDim(manualPrompt), width)...)
		}
		if inputVisible && input != nil {
			lines = append(lines, input.Render(width)...)
		}
		lines = append(lines,
			selectorTextLine("("+tuiThemeKeyHint(selectorCancelKeyHint(), "to cancel")+")", width),
			selectorDynamicBorder(width),
		)
		return lines
	}
	if len(infoLines) > 0 {
		lines = append(lines, "")
		for _, line := range infoLines {
			lines = append(lines, selectorTextLines(line, width)...)
		}
		lines = append(lines,
			"",
			selectorTextLine("("+tuiThemeKeyHint(selectorCancelKeyHint(), "to close")+")", width),
			selectorDynamicBorder(width),
		)
		return lines
	}
	if message != "" {
		lines = append(lines, "", selectorTextLine(tuiThemeFG("text", message), width))
	}
	if inputVisible && input != nil {
		lines = append(lines, input.Render(width)...)
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
	c.mu.RLock()
	onCancel := c.OnCancel
	inputVisible := c.inputVisible
	inputComponent := c.input
	c.mu.RUnlock()
	if gitui.GetKeybindings().Matches(input, "tui.select.cancel") || input == "\x03" {
		if onCancel != nil {
			onCancel()
		}
		return
	}
	if inputVisible && inputComponent != nil {
		inputComponent.HandleInput(input)
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
