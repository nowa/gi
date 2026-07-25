package gicodingagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

func TestShouldRunFirstTimeSetupPiCases(t *testing.T) {
	t.Run("returns true when experimental, default agent dir, and no settings.json", func(t *testing.T) {
		settingsPath := firstTimeSetupTestEnvironment(t, true)
		if !ShouldRunFirstTimeSetup(settingsPath) {
			t.Fatal("first-time setup should run")
		}
	})

	t.Run("returns false when experimental features are disabled", func(t *testing.T) {
		settingsPath := firstTimeSetupTestEnvironment(t, false)
		if ShouldRunFirstTimeSetup(settingsPath) {
			t.Fatal("first-time setup should not run")
		}
	})

	t.Run("returns false when a custom agent dir is set", func(t *testing.T) {
		settingsPath := firstTimeSetupTestEnvironment(t, true)
		t.Setenv(EnvCodingAgentDir, t.TempDir())
		if ShouldRunFirstTimeSetup(settingsPath) {
			t.Fatal("first-time setup should not run")
		}
	})

	t.Run("returns false when settings.json already exists", func(t *testing.T) {
		settingsPath := firstTimeSetupTestEnvironment(t, true)
		if err := os.WriteFile(settingsPath, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if ShouldRunFirstTimeSetup(settingsPath) {
			t.Fatal("first-time setup should not run")
		}
	})

	t.Run("returns false for a forked package", func(t *testing.T) {
		settingsPath := firstTimeSetupTestEnvironment(t, true)
		eligibility := discoverFirstTimeSetupEligibility(
			settingsPath,
			DistributionMetadata{
				PackageName:   "example.invalid/gi-fork",
				AppName:       DefaultCodingAgentAppName,
				ConfigDirName: ConfigDirName,
			},
			false,
		)
		if eligibility.ShouldRun() {
			t.Fatal("first-time setup should not run for a fork")
		}
	})
}

func TestFirstTimeSetupComponentOwnsOneStateProjection(t *testing.T) {
	var previews []TerminalTheme
	var submitted FirstTimeSetupResult
	component := NewFirstTimeSetupComponent(FirstTimeSetupOptions{
		DetectedTheme: TerminalThemeDark,
		OnThemePreview: func(theme TerminalTheme) {
			previews = append(previews, theme)
		},
		OnSubmit: func(result FirstTimeSetupResult) {
			submitted = result
		},
	})

	rendered := strings.Join(component.Render(80), "\n")
	if !strings.Contains(rendered, "Welcome to Gi") ||
		!strings.Contains(rendered, "Detected system appearance: dark") {
		t.Fatalf("initial setup render = %q", rendered)
	}

	component.HandleInput("j")
	if state := component.State(); state.Theme != TerminalThemeLight || state.Step != FirstTimeSetupThemeStep {
		t.Fatalf("theme state = %#v", state)
	}
	if len(previews) != 1 || previews[0] != TerminalThemeLight {
		t.Fatalf("theme previews = %#v", previews)
	}

	component.HandleInput("\n")
	component.HandleInput("j")
	if state := component.State(); state.Step != FirstTimeSetupAnalyticsStep || state.ShareAnalytics {
		t.Fatalf("analytics state = %#v", state)
	}
	component.HandleInput("\n")
	if submitted != (FirstTimeSetupResult{Theme: TerminalThemeLight, ShareAnalytics: false}) {
		t.Fatalf("submitted result = %#v", submitted)
	}
}

func TestShowFirstTimeSetupPersistsSubmittedState(t *testing.T) {
	terminal := gitui.NewVirtualTerminal(80, 24)
	settings := NewInMemorySettingsManager(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result := make(chan error, 1)
	go func() {
		result <- ShowFirstTimeSetup(ctx, settings, FirstTimeSetupRunnerOptions{
			Terminal: terminal,
			DetectTheme: func(context.Context, *gitui.TUI, time.Duration, map[string]string) TerminalTheme {
				return TerminalThemeDark
			},
		})
	}()

	waitForFirstTimeSetupViewport(t, terminal, "Welcome to Gi")
	terminal.SendInput("\n")
	waitForFirstTimeSetupViewport(t, terminal, "anonymous usage data")
	terminal.SendInput("j")
	terminal.SendInput("\n")

	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("first-time setup did not finish")
	}
	if settings.GetTheme() != "dark" || settings.GetEnableAnalytics() || settings.GetTrackingID() != "" {
		t.Fatalf(
			"settings after setup: theme=%q analytics=%t tracking=%q",
			settings.GetTheme(),
			settings.GetEnableAnalytics(),
			settings.GetTrackingID(),
		)
	}
}

func firstTimeSetupTestEnvironment(t *testing.T, experimental bool) string {
	t.Helper()
	t.Setenv(EnvExperimental, "")
	if experimental {
		t.Setenv(LegacyEnvExperimental, "1")
	} else {
		t.Setenv(LegacyEnvExperimental, "")
	}
	t.Setenv(EnvCodingAgentDir, "")
	t.Setenv(LegacyEnvCodingAgentDir, "")
	return filepath.Join(t.TempDir(), "settings.json")
}

func waitForFirstTimeSetupViewport(t *testing.T, terminal *gitui.VirtualTerminal, expected string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		terminal.WaitForRender()
		if strings.Contains(strings.Join(terminal.GetViewport(), "\n"), expected) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("first-time setup viewport did not contain %q:\n%s", expected, strings.Join(terminal.GetViewport(), "\n"))
}
