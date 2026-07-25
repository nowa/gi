package gicodingagent

import (
	"context"
	"strings"
	"testing"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

func TestSettingsThemeSelectionMatchesPiStateRules(t *testing.T) {
	tests := []struct {
		name          string
		current       string
		terminal      TerminalTheme
		themes        []string
		wantMode      settingsThemeMode
		wantSetting   string
		wantSingle    string
		wantAutomatic string
	}{
		{
			name:          "preserves automatic pair for a light terminal",
			current:       "paper/night",
			terminal:      TerminalThemeLight,
			themes:        []string{"dark", "light", "paper", "night"},
			wantMode:      settingsThemeModeAutomatic,
			wantSetting:   "paper/night",
			wantSingle:    "paper",
			wantAutomatic: "paper",
		},
		{
			name:          "selects dark automatic branch for single mode",
			current:       "paper/night",
			terminal:      TerminalThemeDark,
			themes:        []string{"dark", "light", "paper", "night"},
			wantMode:      settingsThemeModeAutomatic,
			wantSetting:   "paper/night",
			wantSingle:    "night",
			wantAutomatic: "night",
		},
		{
			name:          "seeds automatic mode from fixed theme",
			current:       "focus",
			terminal:      TerminalThemeLight,
			themes:        []string{"dark", "focus"},
			wantMode:      settingsThemeModeSingle,
			wantSetting:   "focus",
			wantSingle:    "focus",
			wantAutomatic: "focus",
		},
		{
			name:          "rejects malformed automatic setting",
			current:       "paper/night/other",
			terminal:      TerminalThemeLight,
			themes:        []string{"dark", "light"},
			wantMode:      settingsThemeModeSingle,
			wantSetting:   "dark",
			wantSingle:    "dark",
			wantAutomatic: "dark",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := newSettingsThemeSelection(test.current, test.terminal, test.themes)
			if state.mode != test.wantMode ||
				state.themeSetting() != test.wantSetting ||
				state.singleTheme != test.wantSingle ||
				state.activeAutomaticTheme() != test.wantAutomatic {
				t.Fatalf("theme state = %#v", state)
			}
		})
	}
}

func TestCLIThemeSubmenuAppliesFixedTheme(t *testing.T) {
	var previews []string
	var selected string
	var changed bool
	component := newCLIThemeSubmenu(
		"dark",
		TerminalThemeDark,
		[]string{"dark", "light"},
		func(value string) {
			previews = append(previews, value)
		},
		func(value string, didChange bool) {
			selected = value
			changed = didChange
		},
	)

	rendered := strings.Join(component.Render(80), "\n")
	if !strings.Contains(rendered, "→ dark") ||
		!strings.Contains(rendered, "Automatic") ||
		!strings.Contains(rendered, "follow terminal appearance") {
		t.Fatalf("single theme menu = %q", rendered)
	}

	component.HandleInput("\x1b[B")
	component.HandleInput("\r")
	if selected != "light" || !changed {
		t.Fatalf("selected = %q, changed = %t", selected, changed)
	}
	if len(previews) != 1 || previews[0] != "light" {
		t.Fatalf("previews = %#v", previews)
	}
}

func TestCLIThemeSubmenuConfiguresAndAppliesAutomaticPair(t *testing.T) {
	var previews []string
	var selected string
	var changed bool
	component := newCLIThemeSubmenu(
		"dark",
		TerminalThemeLight,
		[]string{"dark", "light"},
		func(value string) {
			previews = append(previews, value)
		},
		func(value string, didChange bool) {
			selected = value
			changed = didChange
		},
	)

	component.HandleInput("\x1b[A")
	component.HandleInput("\r")
	rendered := strings.Join(component.Render(80), "\n")
	for _, expected := range []string{
		"Automatic Theme",
		"Choose themes for terminal light and dark appearance.",
		"Light/dark detection requires terminal support.",
		"Light theme",
		"Dark theme",
		"Change mode",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("automatic theme menu missing %q: %q", expected, rendered)
		}
	}

	component.HandleInput("\r")
	if rendered = strings.Join(component.Render(80), "\n"); !strings.Contains(rendered, "Light Theme") ||
		!strings.Contains(rendered, "→ dark") {
		t.Fatalf("light theme selector = %q", rendered)
	}
	component.HandleInput("\x1b[B")
	component.HandleInput("\r")

	component.HandleInput("\x1b[B")
	component.HandleInput("\x1b[B")
	component.HandleInput("\r")
	if selected != "light/dark" || !changed {
		t.Fatalf("selected = %q, changed = %t", selected, changed)
	}
	if !stringSliceContains(previews, "dark/dark") ||
		!stringSliceContains(previews, "light") ||
		!stringSliceContains(previews, "light/dark") {
		t.Fatalf("previews = %#v", previews)
	}
}

func TestCLIThemeSubmenuSwitchesToActiveAutomaticThemeAndCancels(t *testing.T) {
	var previews []string
	var selected string
	changed := true
	component := newCLIThemeSubmenu(
		"light/dark",
		TerminalThemeLight,
		[]string{"dark", "light"},
		func(value string) {
			previews = append(previews, value)
		},
		func(value string, didChange bool) {
			selected = value
			changed = didChange
		},
	)

	component.HandleInput("\x1b[B")
	component.HandleInput("\x1b[B")
	component.HandleInput("\x1b[B")
	component.HandleInput("\r")
	rendered := strings.Join(component.Render(80), "\n")
	if !strings.Contains(rendered, "→ light") ||
		strings.Contains(rendered, "Automatic Theme") {
		t.Fatalf("single theme menu after mode change = %q", rendered)
	}

	component.HandleInput("\x1b")
	if selected != "light/dark" || changed {
		t.Fatalf("cancel selected = %q, changed = %t", selected, changed)
	}
	if len(previews) < 2 ||
		previews[len(previews)-2] != "light" ||
		previews[len(previews)-1] != "light/dark" {
		t.Fatalf("previews = %#v", previews)
	}
}

func TestAvailableThemeNamesExpandsAutomaticSetting(t *testing.T) {
	host := &CLIInteractiveTUIHost{}
	names := host.availableThemeNames("paper/night")
	for _, name := range []string{"dark", "light", "paper", "night"} {
		if !stringSliceContains(names, name) {
			t.Fatalf("available themes missing %q: %#v", name, names)
		}
	}
	if stringSliceContains(names, "paper/night") {
		t.Fatalf("automatic setting leaked into theme names: %#v", names)
	}
}

func TestSettingsThemeChangeFlowsThroughController(t *testing.T) {
	previousTheme := tuiActiveThemeSnapshot()
	t.Cleanup(func() {
		tuiSetActiveThemePalette(previousTheme)
	})

	settings := NewInMemorySettingsManager(map[string]any{"theme": "dark"})
	ui := gitui.NewTUI(gitui.NewVirtualTerminal(80, 24))
	controller, err := newInteractiveThemeController(interactiveThemeControllerOptions{
		UI:              ui,
		Settings:        settings,
		AvailableThemes: func() []TUIThemeInfo { return nil },
		DetectAuto: func(
			context.Context,
			TerminalAutoThemeDetector,
			time.Duration,
			map[string]string,
		) TerminalTheme {
			return TerminalThemeLight
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(controller.Dispose)
	host := &CLIInteractiveTUIHost{
		ui:              ui,
		themeController: controller,
	}

	host.applySettingsListChange(nil, settings, "theme", "light/dark")

	setting, present := settings.GetThemeSetting()
	if !present || setting != "light/dark" || settings.GetTheme() != "" {
		t.Fatalf("settings = (%q, %t), fixed = %q", setting, present, settings.GetTheme())
	}
	if controller.ActiveThemeName() != "light" ||
		controller.GetTerminalTheme() != TerminalThemeLight ||
		!controller.AutoSyncEnabled() {
		t.Fatalf(
			"controller active=%q terminal=%q auto=%t",
			controller.ActiveThemeName(),
			controller.GetTerminalTheme(),
			controller.AutoSyncEnabled(),
		)
	}
}
