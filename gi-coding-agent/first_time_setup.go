package gicodingagent

import (
	"strings"

	gitui "github.com/nowa/gi/gi-tui"
)

type FirstTimeSetupStep string

const (
	FirstTimeSetupThemeStep     FirstTimeSetupStep = "theme"
	FirstTimeSetupAnalyticsStep FirstTimeSetupStep = "analytics"
)

// FirstTimeSetupState is the single mutable state owned by the setup
// component. Rendering and submission are projections of this value.
type FirstTimeSetupState struct {
	Step           FirstTimeSetupStep
	Theme          TerminalTheme
	ShareAnalytics bool
}

type FirstTimeSetupResult struct {
	Theme          TerminalTheme
	ShareAnalytics bool
}

type FirstTimeSetupOptions struct {
	DetectedTheme  TerminalTheme
	OnThemePreview func(TerminalTheme)
	OnSubmit       func(FirstTimeSetupResult)
	OnCancel       func()
}

type FirstTimeSetupComponent struct {
	focus   gitui.FocusState
	state   FirstTimeSetupState
	options FirstTimeSetupOptions
}

var firstTimeSetupThemes = []TerminalTheme{TerminalThemeDark, TerminalThemeLight}

var firstTimeSetupAnalyticsOptions = []struct {
	Value bool
	Label string
}{
	{Value: true, Label: "Share anonymous usage data"},
	{Value: false, Label: "Don't share"},
}

var firstTimeSetupLogo = []string{"██████", "██  ██", "████  ██", "██    ██"}

func NewFirstTimeSetupComponent(options FirstTimeSetupOptions) *FirstTimeSetupComponent {
	detectedTheme := options.DetectedTheme
	if detectedTheme != TerminalThemeLight {
		detectedTheme = TerminalThemeDark
	}
	options.DetectedTheme = detectedTheme
	return &FirstTimeSetupComponent{
		state: FirstTimeSetupState{
			Step:           FirstTimeSetupThemeStep,
			Theme:          detectedTheme,
			ShareAnalytics: true,
		},
		options: options,
	}
}

func (c *FirstTimeSetupComponent) Focused() bool {
	return c != nil && c.focus.Focused()
}

func (c *FirstTimeSetupComponent) SetFocused(focused bool) {
	if c != nil {
		c.focus.SetFocused(focused)
	}
}

func (c *FirstTimeSetupComponent) Invalidate() {}

func (c *FirstTimeSetupComponent) State() FirstTimeSetupState {
	if c == nil {
		return FirstTimeSetupState{}
	}
	return c.state
}

func (c *FirstTimeSetupComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	width = max(32, width)
	lines := []string{
		selectorDynamicBorder(width),
		"",
	}
	lines = append(lines, selectorTextLines(tuiThemeAccent(strings.Join(firstTimeSetupLogo, "\n")), width)...)
	lines = append(lines,
		"",
		selectorTextLine(tuiThemeBoldAccent("Welcome to Gi, the minimal coding agent."), width),
		"",
	)

	switch c.state.Step {
	case FirstTimeSetupAnalyticsStep:
		lines = append(lines,
			selectorTextLine(tuiThemeFG("text", "Opt in to anonymous usage data sharing?"), width),
		)
		lines = append(lines, selectorTextLines(tuiThemeMuted(
			"Opting in stores a tracking identifier in settings.json and enables anonymous\n"+
				"usage analytics. This helps improve Gi and diagnose issues. You can change\n"+
				"the preference at any time in settings.json.",
		), width)...)
		lines = append(lines, "")
		labels := make([]string, len(firstTimeSetupAnalyticsOptions))
		selected := 0
		for index, option := range firstTimeSetupAnalyticsOptions {
			labels[index] = option.Label
			if option.Value == c.state.ShareAnalytics {
				selected = index
			}
		}
		lines = append(lines, c.renderOptionList(labels, selected, width)...)
	default:
		lines = append(lines,
			selectorTextLine(tuiThemeFG("text", "Pick a theme."), width),
			selectorTextLine(tuiThemeMuted("Detected system appearance: "+string(c.options.DetectedTheme)), width),
			"",
		)
		labels := []string{"Dark", "Light"}
		selected := 0
		if c.state.Theme == TerminalThemeLight {
			selected = 1
		}
		lines = append(lines, c.renderOptionList(labels, selected, width)...)
	}

	action := "continue"
	if c.state.Step == FirstTimeSetupAnalyticsStep {
		action = "finish"
	}
	lines = append(lines,
		"",
		selectorTextLine(
			tuiThemeKeyHint("↑↓", "navigate")+"  "+
				tuiThemeKeyHint(firstNonEmptyString(formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.select.confirm"), false), "enter"), action)+"  "+
				tuiThemeKeyHint(selectorCancelKeyHint(), "skip setup"),
			width,
		),
		"",
		selectorDynamicBorder(width),
	)
	return lines
}

func (c *FirstTimeSetupComponent) renderOptionList(labels []string, selected, width int) []string {
	lines := make([]string, 0, len(labels))
	for index, label := range labels {
		text := "  " + tuiThemeFG("text", label)
		if index == selected {
			text = tuiThemeAccent("→ ") + tuiThemeAccent(label)
		}
		lines = append(lines, selectorTextLine(text, width))
	}
	return lines
}

func (c *FirstTimeSetupComponent) HandleInput(data string) {
	if c == nil {
		return
	}
	keybindings := gitui.GetKeybindings()
	switch {
	case keybindings.Matches(data, "tui.select.up") || data == "k":
		c.moveSelection(-1)
	case keybindings.Matches(data, "tui.select.down") || data == "j":
		c.moveSelection(1)
	case keybindings.Matches(data, "tui.select.confirm") || data == "\n":
		if c.state.Step == FirstTimeSetupThemeStep {
			c.state.Step = FirstTimeSetupAnalyticsStep
			return
		}
		if c.options.OnSubmit != nil {
			c.options.OnSubmit(FirstTimeSetupResult{
				Theme:          c.state.Theme,
				ShareAnalytics: c.state.ShareAnalytics,
			})
		}
	case keybindings.Matches(data, "tui.select.cancel") || data == "\x03":
		if c.options.OnCancel != nil {
			c.options.OnCancel()
		}
	}
}

func (c *FirstTimeSetupComponent) moveSelection(delta int) {
	switch c.state.Step {
	case FirstTimeSetupAnalyticsStep:
		index := 0
		if !c.state.ShareAnalytics {
			index = 1
		}
		index = max(0, min(len(firstTimeSetupAnalyticsOptions)-1, index+delta))
		c.state.ShareAnalytics = firstTimeSetupAnalyticsOptions[index].Value
	default:
		index := 0
		if c.state.Theme == TerminalThemeLight {
			index = 1
		}
		index = max(0, min(len(firstTimeSetupThemes)-1, index+delta))
		nextTheme := firstTimeSetupThemes[index]
		if nextTheme == c.state.Theme {
			return
		}
		c.state.Theme = nextTheme
		if c.options.OnThemePreview != nil {
			c.options.OnThemePreview(nextTheme)
		}
	}
}
