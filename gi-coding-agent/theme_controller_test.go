package gicodingagent

import (
	"context"
	"strings"
	"testing"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

// Controller tests keep palette mutations serial because the palette is
// process-wide by design.
func TestInteractiveThemeControllerAutomaticStateFlow(t *testing.T) {
	restoreThemeAfterTest(t)
	settings := NewInMemorySettingsManager(map[string]any{"theme": "light/dark"})
	ui := gitui.NewTUI(gitui.NewVirtualTerminal(80, 24))
	var changes int
	controller := newInteractiveThemeControllerForTest(t, interactiveThemeControllerOptions{
		UI:       ui,
		Settings: settings,
		Environment: map[string]string{
			"COLORFGBG": "15;0",
		},
		DetectAuto: func(
			context.Context,
			TerminalAutoThemeDetector,
			time.Duration,
			map[string]string,
		) TerminalTheme {
			return TerminalThemeLight
		},
		OnChanged: func() {
			changes++
		},
	})

	if got := controller.ActiveThemeName(); got != "dark" {
		t.Fatalf("initial active theme = %q, want dark from the environment snapshot", got)
	}
	controller.ApplyFromSettings(context.Background())
	if got := controller.ActiveThemeName(); got != "light" {
		t.Fatalf("detected active theme = %q, want light", got)
	}
	if got := controller.GetTerminalTheme(); got != TerminalThemeLight || !controller.AutoSyncEnabled() {
		t.Fatalf("terminal theme = %q, auto sync = %t", got, controller.AutoSyncEnabled())
	}

	ui.HandleInput("\x1b[?997;1n")
	if got := controller.ActiveThemeName(); got != "dark" {
		t.Fatalf("active theme after dark report = %q, want dark", got)
	}
	if changes != 2 {
		t.Fatalf("change notifications = %d, want apply plus terminal transition", changes)
	}

	result := controller.SetThemeName("light", false)
	if !result.Success || controller.AutoSyncEnabled() {
		t.Fatalf("fixed result = %#v, auto sync = %t", result, controller.AutoSyncEnabled())
	}
	ui.HandleInput("\x1b[?997;1n")
	if got := controller.ActiveThemeName(); got != "light" {
		t.Fatalf("disabled auto sync changed active theme to %q", got)
	}
}

func TestInteractiveThemeControllerFallsBackAndReportsErrors(t *testing.T) {
	restoreThemeAfterTest(t)
	settings := NewInMemorySettingsManager(map[string]any{"theme": "dark"})
	ui := gitui.NewTUI(gitui.NewVirtualTerminal(80, 24))
	var errorsSeen []string
	controller := newInteractiveThemeControllerForTest(t, interactiveThemeControllerOptions{
		UI:       ui,
		Settings: settings,
		ShowError: func(message string) {
			errorsSeen = append(errorsSeen, message)
		},
	})

	result := controller.SetThemeName("missing-theme", true)
	if result.Success || result.Err == nil {
		t.Fatalf("missing theme result = %#v", result)
	}
	if got := controller.ActiveThemeName(); got != "dark" {
		t.Fatalf("fallback theme = %q, want dark", got)
	}
	if got := tuiActiveThemeSnapshot().name; got != "dark" {
		t.Fatalf("global fallback theme = %q, want dark", got)
	}
	if len(errorsSeen) != 1 ||
		!strings.Contains(errorsSeen[0], `Failed to load theme "missing-theme": theme not found: missing-theme`) ||
		!strings.Contains(errorsSeen[0], "Fell back to dark theme.") {
		t.Fatalf("errors = %#v", errorsSeen)
	}
}

func TestInteractiveThemeControllerPersistsHighConfidenceDetection(t *testing.T) {
	restoreThemeAfterTest(t)
	settings := NewInMemorySettingsManager(nil)
	controller := newInteractiveThemeControllerForTest(t, interactiveThemeControllerOptions{
		UI:       gitui.NewTUI(gitui.NewVirtualTerminal(80, 24)),
		Settings: settings,
		DetectBackground: func(
			context.Context,
			TerminalBackgroundThemeDetector,
			time.Duration,
			map[string]string,
		) TerminalThemeDetection {
			return TerminalThemeDetection{
				Theme:      TerminalThemeLight,
				Source:     "terminal background",
				Confidence: "high",
			}
		},
	})

	controller.ApplyFromSettings(context.Background())
	if got := controller.ActiveThemeName(); got != "light" {
		t.Fatalf("active theme = %q, want light", got)
	}
	if setting, present := settings.GetThemeSetting(); !present || setting != "light" {
		t.Fatalf("persisted setting = (%q, %t), want (light, true)", setting, present)
	}
}

func TestInteractiveThemeControllerPreviewKeepsCommittedState(t *testing.T) {
	restoreThemeAfterTest(t)
	settings := NewInMemorySettingsManager(map[string]any{"theme": "dark"})
	controller := newInteractiveThemeControllerForTest(t, interactiveThemeControllerOptions{
		UI:       gitui.NewTUI(gitui.NewVirtualTerminal(80, 24)),
		Settings: settings,
	})

	controller.Preview("light")
	if got := tuiActiveThemeSnapshot().name; got != "light" {
		t.Fatalf("preview palette = %q, want light", got)
	}
	if got := controller.ActiveThemeName(); got != "dark" {
		t.Fatalf("preview changed committed theme to %q", got)
	}

	controller.ApplyFromSettings(context.Background())
	if got := tuiActiveThemeSnapshot().name; got != "dark" {
		t.Fatalf("restored palette = %q, want dark", got)
	}
}

func TestInteractiveThemeControllerInMemoryThemeDisablesAutomaticSync(t *testing.T) {
	restoreThemeAfterTest(t)
	settings := NewInMemorySettingsManager(map[string]any{"theme": "light/dark"})
	ui := gitui.NewTUI(gitui.NewVirtualTerminal(80, 24))
	controller := newInteractiveThemeControllerForTest(t, interactiveThemeControllerOptions{
		UI:       ui,
		Settings: settings,
		DetectAuto: func(
			context.Context,
			TerminalAutoThemeDetector,
			time.Duration,
			map[string]string,
		) TerminalTheme {
			return TerminalThemeLight
		},
	})
	controller.ApplyFromSettings(context.Background())
	controller.DisableAutoSync()
	if controller.AutoSyncEnabled() {
		t.Fatal("DisableAutoSync retained automatic synchronization")
	}

	result := controller.SetThemeInstance(tuiBuiltinThemePalette("memory", tuiLightThemeFG, tuiLightThemeBG))
	if !result.Success || controller.ActiveThemeName() != "<in-memory>" {
		t.Fatalf("in-memory theme result = %#v, active = %q", result, controller.ActiveThemeName())
	}
	ui.HandleInput("\x1b[?997;1n")
	if got := controller.ActiveThemeName(); got != "<in-memory>" {
		t.Fatalf("terminal report replaced in-memory theme with %q", got)
	}
}

func TestInteractiveThemeControllerSupersedesInFlightDetection(t *testing.T) {
	restoreThemeAfterTest(t)
	settings := NewInMemorySettingsManager(map[string]any{"theme": "light/dark"})
	started := make(chan struct{})
	release := make(chan struct{})
	controller := newInteractiveThemeControllerForTest(t, interactiveThemeControllerOptions{
		UI:       gitui.NewTUI(gitui.NewVirtualTerminal(80, 24)),
		Settings: settings,
		DetectAuto: func(
			context.Context,
			TerminalAutoThemeDetector,
			time.Duration,
			map[string]string,
		) TerminalTheme {
			close(started)
			<-release
			return TerminalThemeLight
		},
	})

	done := make(chan struct{})
	go func() {
		controller.ApplyFromSettings(context.Background())
		close(done)
	}()
	<-started
	if result := controller.SetThemeName("dark", false); !result.Success {
		t.Fatalf("fixed theme result = %#v", result)
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("in-flight detection did not finish")
	}
	if got := controller.ActiveThemeName(); got != "dark" {
		t.Fatalf("stale detection overwrote active theme with %q", got)
	}
	if controller.AutoSyncEnabled() {
		t.Fatal("stale detection re-enabled automatic synchronization")
	}
}

func TestInteractiveThemeControllerDisposeStopsTerminalUpdates(t *testing.T) {
	restoreThemeAfterTest(t)
	settings := NewInMemorySettingsManager(map[string]any{"theme": "light/dark"})
	ui := gitui.NewTUI(gitui.NewVirtualTerminal(80, 24))
	controller := newInteractiveThemeControllerForTest(t, interactiveThemeControllerOptions{
		UI:       ui,
		Settings: settings,
		DetectAuto: func(
			context.Context,
			TerminalAutoThemeDetector,
			time.Duration,
			map[string]string,
		) TerminalTheme {
			return TerminalThemeLight
		},
	})
	controller.ApplyFromSettings(context.Background())
	controller.Dispose()

	ui.HandleInput("\x1b[?997;1n")
	if got := controller.ActiveThemeName(); got != "light" {
		t.Fatalf("disposed controller changed active theme to %q", got)
	}
	if controller.AutoSyncEnabled() {
		t.Fatal("disposed controller retained automatic synchronization")
	}
}

func newInteractiveThemeControllerForTest(
	t *testing.T,
	options interactiveThemeControllerOptions,
) *interactiveThemeController {
	t.Helper()
	if options.AvailableThemes == nil {
		options.AvailableThemes = func() []TUIThemeInfo {
			return []TUIThemeInfo{
				{Name: "dark", Builtin: true},
				{Name: "light", Builtin: true},
			}
		}
	}
	controller, err := newInteractiveThemeController(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(controller.Dispose)
	return controller
}

func restoreThemeAfterTest(t *testing.T) {
	t.Helper()
	previous := tuiActiveThemeSnapshot()
	t.Cleanup(func() {
		tuiSetActiveThemePalette(previous)
	})
}
