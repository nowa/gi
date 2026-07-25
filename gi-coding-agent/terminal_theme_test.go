package gicodingagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

type terminalThemeDetectorStub struct {
	scheme    gitui.TerminalColorScheme
	schemeOK  bool
	schemeErr error
	color     gitui.RGBColor
	colorOK   bool
	colorErr  error
}

func (d terminalThemeDetectorStub) QueryTerminalColorScheme(context.Context) (gitui.TerminalColorScheme, bool, error) {
	return d.scheme, d.schemeOK, d.schemeErr
}

func (d terminalThemeDetectorStub) QueryTerminalBackgroundColor(context.Context) (gitui.RGBColor, bool, error) {
	return d.color, d.colorOK, d.colorErr
}

func TestDetectTerminalBackgroundFromEnvPiCases(t *testing.T) {
	t.Run("uses the COLORFGBG background color index", func(t *testing.T) {
		light := DetectTerminalBackgroundFromEnv(map[string]string{"COLORFGBG": "0;15"})
		if light.Theme != TerminalThemeLight || light.Source != "COLORFGBG" || light.Confidence != "high" {
			t.Fatalf("light detection = %#v", light)
		}
		dark := DetectTerminalBackgroundFromEnv(map[string]string{"COLORFGBG": "15;0"})
		if dark.Theme != TerminalThemeDark || dark.Source != "COLORFGBG" || dark.Confidence != "high" {
			t.Fatalf("dark detection = %#v", dark)
		}
	})

	t.Run("uses the last COLORFGBG field as the background", func(t *testing.T) {
		if got := DetectTerminalBackgroundFromEnv(map[string]string{"COLORFGBG": "0;7;15"}).Theme; got != TerminalThemeLight {
			t.Fatalf("theme = %q, want light", got)
		}
	})

	t.Run("defaults to dark without terminal background hints", func(t *testing.T) {
		detection := DetectTerminalBackgroundFromEnv(map[string]string{})
		if detection.Theme != TerminalThemeDark || detection.Source != "fallback" || detection.Confidence != "low" {
			t.Fatalf("detection = %#v", detection)
		}
	})
}

func TestDetectTerminalBackgroundThemePiCases(t *testing.T) {
	t.Run("uses the queried terminal background before environment hints", func(t *testing.T) {
		detection := DetectTerminalBackgroundTheme(
			context.Background(),
			terminalThemeDetectorStub{color: gitui.RGBColor{R: 250, G: 250, B: 250}, colorOK: true},
			250*time.Millisecond,
			map[string]string{"COLORFGBG": "15;0"},
		)
		if detection.Theme != TerminalThemeLight || detection.Source != "terminal background" || detection.Confidence != "high" {
			t.Fatalf("detection = %#v", detection)
		}
	})

	t.Run("falls back to environment hints when the terminal query returns no color", func(t *testing.T) {
		detection := DetectTerminalBackgroundTheme(
			context.Background(),
			terminalThemeDetectorStub{},
			250*time.Millisecond,
			map[string]string{"COLORFGBG": "15;0"},
		)
		if detection.Theme != TerminalThemeDark || detection.Source != "COLORFGBG" {
			t.Fatalf("detection = %#v", detection)
		}
	})

	t.Run("falls back to environment hints when the terminal query fails", func(t *testing.T) {
		detection := DetectTerminalBackgroundTheme(
			context.Background(),
			terminalThemeDetectorStub{colorErr: errors.New("terminal write failed")},
			250*time.Millisecond,
			map[string]string{"COLORFGBG": "0;15"},
		)
		if detection.Theme != TerminalThemeLight || detection.Source != "COLORFGBG" {
			t.Fatalf("detection = %#v", detection)
		}
	})
}

func TestDetectTerminalThemeForAutoUsesColorSchemeFirst(t *testing.T) {
	detector := terminalThemeDetectorStub{
		scheme:   gitui.TerminalColorSchemeLight,
		schemeOK: true,
		color:    gitui.RGBColor{},
		colorOK:  true,
	}
	if got := DetectTerminalThemeForAuto(context.Background(), detector, time.Second, nil); got != TerminalThemeLight {
		t.Fatalf("theme = %q, want light", got)
	}
}

func TestThemeDetectionFromRGBClassifiesByLuminance(t *testing.T) {
	if got := GetThemeForRGBColor(gitui.RGBColor{R: 8, G: 8, B: 8}); got != TerminalThemeDark {
		t.Fatalf("dark RGB theme = %q", got)
	}
	if got := GetThemeForRGBColor(gitui.RGBColor{R: 250, G: 250, B: 250}); got != TerminalThemeLight {
		t.Fatalf("light RGB theme = %q", got)
	}
}

func TestTUIThemeUsesTerminalCapabilities(t *testing.T) {
	previous := tuiActiveThemeSnapshot()
	t.Cleanup(func() {
		tuiSetActiveThemePalette(previous)
		gitui.ResetCapabilitiesCache()
	})

	t.Run("uses terminal capabilities", func(t *testing.T) {
		gitui.SetCapabilities(gitui.TerminalCapabilities{TrueColor: false})
		if err := tuiSetActiveTheme("dark", nil); err != nil {
			t.Fatal(err)
		}
		if got := tuiThemeAccent("x"); !strings.HasPrefix(got, "\x1b[38;5;") {
			t.Fatalf("256-color accent = %q", got)
		}

		gitui.SetCapabilities(gitui.TerminalCapabilities{TrueColor: true})
		if err := tuiSetActiveTheme("dark", nil); err != nil {
			t.Fatal(err)
		}
		if got := tuiThemeAccent("x"); !strings.HasPrefix(got, "\x1b[38;2;") {
			t.Fatalf("truecolor accent = %q", got)
		}
	})
}
