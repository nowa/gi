package gicodingagent

import (
	"strings"

	gitui "github.com/nowa/gi/gi-tui"
)

type ExtensionSelectorComponent struct {
	focus    gitui.FocusState
	title    string
	options  []string
	selected int
	OnSelect func(option string)
	OnCancel func()
}

func NewExtensionSelectorComponent(title string, options []string) *ExtensionSelectorComponent {
	return &ExtensionSelectorComponent{
		title:   strings.TrimSpace(title),
		options: append([]string(nil), options...),
	}
}

func (c *ExtensionSelectorComponent) Focused() bool {
	if c == nil {
		return false
	}
	return c.focus.Focused()
}

func (c *ExtensionSelectorComponent) SetFocused(focused bool) {
	if c != nil {
		c.focus.SetFocused(focused)
	}
}

func (c *ExtensionSelectorComponent) Invalidate() {}

func (c *ExtensionSelectorComponent) Title() string {
	if c == nil {
		return ""
	}
	return c.title
}

func (c *ExtensionSelectorComponent) SetTitle(title string) {
	if c != nil {
		c.title = strings.TrimSpace(title)
	}
}

func (c *ExtensionSelectorComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	width = max(24, width)
	title := firstNonEmptyString(c.title, "Select")
	lines := []string{
		selectorDynamicBorder(width),
		"",
	}
	lines = append(lines, selectorTextLines(tuiThemeBoldAccent(title), width)...)
	lines = append(lines, "")
	for index, option := range c.options {
		text := "  " + tuiThemeFG("text", option)
		if index == c.selected {
			text = tuiThemeAccent("→ ") + tuiThemeAccent(option)
		}
		lines = append(lines, selectorTextLine(text, width))
	}
	lines = append(lines,
		"",
		selectorTextLine(extensionSelectorFooterHint(), width),
		"",
		selectorDynamicBorder(width),
	)
	return lines
}

func (c *ExtensionSelectorComponent) HandleInput(input string) {
	if c == nil {
		return
	}
	kb := gitui.GetKeybindings()
	switch {
	case kb.Matches(input, "tui.select.up") || input == "k":
		c.move(-1)
	case kb.Matches(input, "tui.select.down") || input == "j":
		c.move(1)
	case kb.Matches(input, "tui.select.confirm") || input == "\n":
		if c.selected >= 0 && c.selected < len(c.options) && c.OnSelect != nil {
			c.OnSelect(c.options[c.selected])
		}
	case kb.Matches(input, "tui.select.cancel") || input == "\x03":
		if c.OnCancel != nil {
			c.OnCancel()
		}
	}
}

func (c *ExtensionSelectorComponent) move(delta int) {
	if len(c.options) == 0 {
		return
	}
	c.selected = max(0, min(len(c.options)-1, c.selected+delta))
}

func extensionSelectorFooterHint() string {
	return tuiThemeKeyHint("↑↓", "navigate") + "  " +
		tuiThemeKeyHint(firstNonEmptyString(formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.select.confirm"), false), "enter"), "select") + "  " +
		tuiThemeKeyHint(selectorCancelKeyHint(), "cancel")
}

func selectorCancelKeyHint() string {
	hint := formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.select.cancel"), false)
	if hint == "" || hint == "escape" {
		return "escape/ctrl+c"
	}
	return hint
}

func selectorDynamicBorder(width int) string {
	return tuiThemeBorder(strings.Repeat("─", max(1, width)))
}

func selectorTextLine(text string, width int) string {
	return gitui.NewTruncatedText(text, 1, 0).Render(width)[0]
}

func selectorTextLines(text string, width int) []string {
	lines := gitui.NewText(text, 1, 0).Render(width)
	if len(lines) == 0 {
		return nil
	}
	return lines
}
