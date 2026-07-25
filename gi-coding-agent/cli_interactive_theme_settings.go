package gicodingagent

import (
	"strings"
	"sync"

	gitui "github.com/nowa/gi/gi-tui"
)

const automaticThemeSelectionValue = "/"

type settingsThemeMode uint8

const (
	settingsThemeModeSingle settingsThemeMode = iota
	settingsThemeModeAutomatic
)

// settingsThemeSelection is the complete value state for the theme submenu.
// Rendering and callbacks derive from snapshots of this value, so partially
// configured automatic themes never leak into SettingsManager.
type settingsThemeSelection struct {
	mode          settingsThemeMode
	singleTheme   string
	automatic     AutoThemeSetting
	terminalTheme TerminalTheme
}

func newSettingsThemeSelection(
	currentThemeSetting string,
	terminalTheme TerminalTheme,
	availableThemes []string,
) settingsThemeSelection {
	terminalTheme = normalizeSettingsTerminalTheme(terminalTheme)
	automatic, isAutomatic := ParseAutoThemeSetting(currentThemeSetting)
	if !isAutomatic {
		automatic = defaultAutomaticThemeSetting(currentThemeSetting, availableThemes)
	}

	fixedTheme := ""
	if !isAutomatic && !strings.Contains(currentThemeSetting, "/") {
		fixedTheme = currentThemeSetting
	}
	activeAutomaticTheme := automatic.DarkTheme
	if terminalTheme == TerminalThemeLight {
		activeAutomaticTheme = automatic.LightTheme
	}
	singleTheme := preferredSettingsTheme(availableThemes, fixedTheme, "dark")
	if isAutomatic {
		singleTheme = preferredSettingsTheme(availableThemes, activeAutomaticTheme, "dark")
	}

	mode := settingsThemeModeSingle
	if isAutomatic {
		mode = settingsThemeModeAutomatic
	}
	return settingsThemeSelection{
		mode:          mode,
		singleTheme:   singleTheme,
		automatic:     automatic,
		terminalTheme: terminalTheme,
	}
}

func (s settingsThemeSelection) themeSetting() string {
	if s.mode == settingsThemeModeAutomatic {
		return s.automatic.LightTheme + "/" + s.automatic.DarkTheme
	}
	return s.singleTheme
}

func (s settingsThemeSelection) activeAutomaticTheme() string {
	if s.terminalTheme == TerminalThemeLight {
		return s.automatic.LightTheme
	}
	return s.automatic.DarkTheme
}

func settingsThemeItems(availableThemes []string) []gitui.SelectItem {
	items := make([]gitui.SelectItem, 0, len(availableThemes))
	for _, name := range availableThemes {
		items = append(items, gitui.SelectItem{Value: name, Label: name})
	}
	return items
}

func settingsSingleModeThemeItems(availableThemes []string) []gitui.SelectItem {
	items := []gitui.SelectItem{{
		Value:       automaticThemeSelectionValue,
		Label:       "Automatic",
		Description: "Use separate themes for light and dark terminal appearance",
	}}
	return append(items, settingsThemeItems(availableThemes)...)
}

func preferredSettingsTheme(availableThemes []string, preferred, fallback string) string {
	if preferred != "" && stringSliceContains(availableThemes, preferred) {
		return preferred
	}
	if stringSliceContains(availableThemes, fallback) {
		return fallback
	}
	if len(availableThemes) > 0 {
		return availableThemes[0]
	}
	return fallback
}

func defaultAutomaticThemeSetting(currentThemeSetting string, availableThemes []string) AutoThemeSetting {
	if automatic, ok := ParseAutoThemeSetting(currentThemeSetting); ok {
		return automatic
	}
	fixedTheme := ""
	if !strings.Contains(currentThemeSetting, "/") {
		fixedTheme = currentThemeSetting
	}
	name := preferredSettingsTheme(availableThemes, fixedTheme, "dark")
	return AutoThemeSetting{LightTheme: name, DarkTheme: name}
}

func normalizeSettingsTerminalTheme(terminalTheme TerminalTheme) TerminalTheme {
	if terminalTheme == TerminalThemeLight {
		return TerminalThemeLight
	}
	return TerminalThemeDark
}

func settingsThemeSubmenu(
	availableThemes []string,
	terminalTheme TerminalTheme,
	onPreview func(string),
) func(currentValue string, done func(selectedValue string, changed bool)) gitui.Component {
	themes := append([]string(nil), availableThemes...)
	return func(currentValue string, done func(selectedValue string, changed bool)) gitui.Component {
		return newCLIThemeSubmenu(currentValue, terminalTheme, themes, onPreview, done)
	}
}

// cliThemeSubmenu owns one theme-selection transaction. Only apply publishes
// the selected value to the parent SettingsList; cancel restores the original
// preview and discards the value snapshot.
type cliThemeSubmenu struct {
	mu sync.RWMutex

	content gitui.Component
	input   gitui.InputHandler
	state   settingsThemeSelection

	originalThemeSetting string
	availableThemes      []string
	onPreview            func(string)
	done                 func(string, bool)
}

func newCLIThemeSubmenu(
	currentThemeSetting string,
	terminalTheme TerminalTheme,
	availableThemes []string,
	onPreview func(string),
	done func(string, bool),
) *cliThemeSubmenu {
	component := &cliThemeSubmenu{
		state:                newSettingsThemeSelection(currentThemeSetting, terminalTheme, availableThemes),
		originalThemeSetting: currentThemeSetting,
		availableThemes:      append([]string(nil), availableThemes...),
		onPreview:            onPreview,
		done:                 done,
	}
	if component.state.mode == settingsThemeModeAutomatic {
		component.showAutomaticMenu()
	} else {
		component.showSingleMenu()
	}
	return component
}

func (c *cliThemeSubmenu) Invalidate() {
	if content := c.contentSnapshot(); content != nil {
		content.Invalidate()
	}
}

func (c *cliThemeSubmenu) Render(width int) []string {
	if content := c.contentSnapshot(); content != nil {
		return content.Render(max(1, width))
	}
	return nil
}

func (c *cliThemeSubmenu) HandleInput(data string) {
	c.mu.RLock()
	input := c.input
	c.mu.RUnlock()
	if input != nil {
		input.HandleInput(data)
	}
}

func (c *cliThemeSubmenu) contentSnapshot() gitui.Component {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.content
}

func (c *cliThemeSubmenu) stateSnapshot() settingsThemeSelection {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *cliThemeSubmenu) updateState(update func(*settingsThemeSelection)) settingsThemeSelection {
	c.mu.Lock()
	update(&c.state)
	state := c.state
	c.mu.Unlock()
	return state
}

func (c *cliThemeSubmenu) setContent(content gitui.Component, input gitui.InputHandler) {
	c.mu.Lock()
	c.content = content
	c.input = input
	c.mu.Unlock()
}

func (c *cliThemeSubmenu) showSingleMenu() {
	state := c.updateState(func(state *settingsThemeSelection) {
		state.mode = settingsThemeModeSingle
	})
	menu := newCLIThemeSelectSubmenu(
		"Theme",
		"Select a theme, or choose Automatic to follow terminal appearance.",
		settingsSingleModeThemeItems(c.availableThemes),
		state.singleTheme,
		c.selectSingleTheme,
		c.cancel,
		func(value string) {
			if value == automaticThemeSelectionValue {
				c.preview(c.automaticThemeSetting())
				return
			}
			c.preview(value)
		},
	)
	c.setContent(menu, menu)
}

func (c *cliThemeSubmenu) selectSingleTheme(value string) {
	if value == automaticThemeSelectionValue {
		state := c.updateState(func(state *settingsThemeSelection) {
			state.mode = settingsThemeModeAutomatic
		})
		c.preview(state.themeSetting())
		c.showAutomaticMenu()
		return
	}
	c.updateState(func(state *settingsThemeSelection) {
		state.singleTheme = value
	})
	c.apply(value)
}

func (c *cliThemeSubmenu) showAutomaticMenu() {
	state := c.updateState(func(state *settingsThemeSelection) {
		state.mode = settingsThemeModeAutomatic
	})
	content := gitui.NewContainer()
	content.AddChild(gitui.NewText(tuiThemeBoldAccent("Automatic Theme"), 0, 0))
	content.AddChild(gitui.NewSpacer(1))
	content.AddChild(gitui.NewText(
		tuiThemeMuted("Choose themes for terminal light and dark appearance.")+"\n"+
			tuiThemeMuted("Light/dark detection requires terminal support."),
		0,
		0,
	))
	content.AddChild(gitui.NewSpacer(1))

	items := []gitui.SettingItem{
		{
			ID:           "light-theme",
			Label:        "Light theme",
			Description:  "Theme to use in automatic mode when the terminal is light",
			CurrentValue: state.automatic.LightTheme,
			Submenu: func(currentValue string, done func(string, bool)) gitui.Component {
				return c.createThemeSelect(
					"Light Theme",
					"Select the theme to use for light terminal appearance",
					currentValue,
					done,
				)
			},
		},
		{
			ID:           "dark-theme",
			Label:        "Dark theme",
			Description:  "Theme to use in automatic mode when the terminal is dark",
			CurrentValue: state.automatic.DarkTheme,
			Submenu: func(currentValue string, done func(string, bool)) gitui.Component {
				return c.createThemeSelect(
					"Dark Theme",
					"Select the theme to use for dark terminal appearance",
					currentValue,
					done,
				)
			},
		},
		{
			ID:           "apply",
			Label:        "Apply",
			Description:  "Save and go back",
			CurrentValue: "save and go back",
			Values:       []string{"save and go back"},
		},
		{
			ID:           "single-mode",
			Label:        "Change mode",
			Description:  "Switch to one theme for light and dark",
			CurrentValue: "switch to single theme",
			Values:       []string{"switch to single theme"},
		},
	}
	settingsList := gitui.NewSettingsList(
		items,
		min(len(items), 10),
		tuiThemeSettingsList(),
		gitui.SettingsListOptions{
			OnChange: c.changeAutomaticSetting,
			OnCancel: c.cancel,
		},
	)
	content.AddChild(settingsList)
	c.setContent(content, settingsList)
}

func (c *cliThemeSubmenu) createThemeSelect(
	title string,
	description string,
	currentValue string,
	done func(string, bool),
) gitui.Component {
	return newCLIThemeSelectSubmenu(
		title,
		description,
		settingsThemeItems(c.availableThemes),
		currentValue,
		func(value string) {
			done(value, true)
		},
		func() {
			c.preview(c.themeSetting())
			done(currentValue, false)
		},
		c.preview,
	)
}

func (c *cliThemeSubmenu) changeAutomaticSetting(id, value string) {
	switch id {
	case "light-theme":
		state := c.updateState(func(state *settingsThemeSelection) {
			state.automatic.LightTheme = value
		})
		c.preview(state.themeSetting())
	case "dark-theme":
		state := c.updateState(func(state *settingsThemeSelection) {
			state.automatic.DarkTheme = value
		})
		c.preview(state.themeSetting())
	case "apply":
		c.apply(c.automaticThemeSetting())
	case "single-mode":
		state := c.updateState(func(state *settingsThemeSelection) {
			state.mode = settingsThemeModeSingle
			state.singleTheme = state.activeAutomaticTheme()
		})
		c.preview(state.singleTheme)
		c.showSingleMenu()
	}
}

func (c *cliThemeSubmenu) themeSetting() string {
	return c.stateSnapshot().themeSetting()
}

func (c *cliThemeSubmenu) automaticThemeSetting() string {
	state := c.stateSnapshot()
	return state.automatic.LightTheme + "/" + state.automatic.DarkTheme
}

func (c *cliThemeSubmenu) apply(themeSetting string) {
	if c.done != nil {
		c.done(themeSetting, true)
	}
}

func (c *cliThemeSubmenu) cancel() {
	c.preview(c.originalThemeSetting)
	if c.done != nil {
		c.done(c.originalThemeSetting, false)
	}
}

func (c *cliThemeSubmenu) preview(themeSetting string) {
	if c.onPreview != nil {
		c.onPreview(themeSetting)
	}
}

func newCLIThemeSelectSubmenu(
	title string,
	description string,
	items []gitui.SelectItem,
	currentValue string,
	onSelect func(string),
	onCancel func(),
	onSelectionChange func(string),
) *cliSettingsSelectSubmenu {
	list := gitui.NewSelectList(
		items,
		min(len(items), 10),
		tuiThemeSelectList(),
		settingsSubmenuSelectListLayout,
	)
	if index := settingsSelectItemIndex(items, currentValue); index >= 0 {
		list.SetSelectedIndex(index)
	}
	component := &cliSettingsSelectSubmenu{
		title:        title,
		message:      description,
		list:         list,
		currentValue: currentValue,
	}
	list.OnSelect = func(item gitui.SelectItem) {
		if onSelect != nil {
			onSelect(item.Value)
		}
	}
	list.OnCancel = onCancel
	list.OnSelectionChange = func(item gitui.SelectItem) {
		if onSelectionChange != nil {
			onSelectionChange(item.Value)
		}
	}
	return component
}

func (h *CLIInteractiveTUIHost) settingsTerminalTheme() TerminalTheme {
	if h != nil {
		if controller := h.interactiveThemeController(); controller != nil {
			return controller.GetTerminalTheme()
		}
	}
	return DetectTerminalBackgroundFromEnv(nil).Theme
}

func stringSliceContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
