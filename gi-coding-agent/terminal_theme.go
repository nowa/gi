package gicodingagent

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

type TerminalTheme string

const (
	TerminalThemeDark  TerminalTheme = "dark"
	TerminalThemeLight TerminalTheme = "light"
)

type TerminalThemeDetection struct {
	Theme      TerminalTheme
	Source     string
	Detail     string
	Confidence string
}

type TerminalBackgroundThemeDetector interface {
	QueryTerminalBackgroundColor(context.Context) (gitui.RGBColor, bool, error)
}

type TerminalAutoThemeDetector interface {
	TerminalBackgroundThemeDetector
	QueryTerminalColorScheme(context.Context) (gitui.TerminalColorScheme, bool, error)
}

// DetectTerminalBackgroundFromEnv derives the theme from the last valid
// COLORFGBG field. A nil environment reads the process environment; an empty
// map intentionally represents an environment without hints.
func DetectTerminalBackgroundFromEnv(environment map[string]string) TerminalThemeDetection {
	background, ok := colorFGBGBackgroundIndex(environmentValue(environment, "COLORFGBG"))
	if ok {
		return TerminalThemeDetection{
			Theme:      GetThemeForRGBColor(ansi256RGB(background)),
			Source:     "COLORFGBG",
			Detail:     fmt.Sprintf("background color index %d", background),
			Confidence: "high",
		}
	}
	return TerminalThemeDetection{
		Theme:      TerminalThemeDark,
		Source:     "fallback",
		Detail:     "no terminal background hint found",
		Confidence: "low",
	}
}

// DetectTerminalBackgroundTheme prefers a terminal OSC 11 response and falls
// back to the immutable environment snapshot supplied by the caller.
func DetectTerminalBackgroundTheme(
	ctx context.Context,
	detector TerminalBackgroundThemeDetector,
	timeout time.Duration,
	environment map[string]string,
) TerminalThemeDetection {
	if detector != nil {
		queryCtx, cancel := terminalThemeQueryContext(ctx, timeout)
		color, ok, err := detector.QueryTerminalBackgroundColor(queryCtx)
		cancel()
		if err == nil && ok {
			return TerminalThemeDetection{
				Theme:      GetThemeForRGBColor(color),
				Source:     "terminal background",
				Detail:     fmt.Sprintf("OSC 11 background rgb(%d, %d, %d)", color.R, color.G, color.B),
				Confidence: "high",
			}
		}
	}
	return DetectTerminalBackgroundFromEnv(environment)
}

// DetectTerminalThemeForAuto first asks terminals that expose a color-scheme
// capability, then uses OSC 11 and finally COLORFGBG.
func DetectTerminalThemeForAuto(
	ctx context.Context,
	detector TerminalAutoThemeDetector,
	timeout time.Duration,
	environment map[string]string,
) TerminalTheme {
	if detector != nil {
		queryCtx, cancel := terminalThemeQueryContext(ctx, timeout)
		scheme, ok, err := detector.QueryTerminalColorScheme(queryCtx)
		cancel()
		if err == nil && ok {
			switch scheme {
			case gitui.TerminalColorSchemeLight:
				return TerminalThemeLight
			case gitui.TerminalColorSchemeDark:
				return TerminalThemeDark
			}
		}
	}
	return DetectTerminalBackgroundTheme(ctx, detector, timeout, environment).Theme
}

func GetThemeForRGBColor(color gitui.RGBColor) TerminalTheme {
	toLinear := func(channel int) float64 {
		channel = min(max(channel, 0), 255)
		value := float64(channel) / 255
		if value <= 0.03928 {
			return value / 12.92
		}
		return math.Pow((value+0.055)/1.055, 2.4)
	}
	luminance := 0.2126*toLinear(color.R) + 0.7152*toLinear(color.G) + 0.0722*toLinear(color.B)
	if luminance >= 0.5 {
		return TerminalThemeLight
	}
	return TerminalThemeDark
}

func terminalThemeQueryContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func environmentValue(environment map[string]string, key string) string {
	if environment == nil {
		return os.Getenv(key)
	}
	return environment[key]
}

func colorFGBGBackgroundIndex(value string) (int, bool) {
	parts := strings.Split(value, ";")
	for index := len(parts) - 1; index >= 0; index-- {
		background, err := strconv.Atoi(strings.TrimSpace(parts[index]))
		if err == nil && background >= 0 && background <= 255 {
			return background, true
		}
	}
	return 0, false
}

func ansi256RGB(index int) gitui.RGBColor {
	basic := [...]gitui.RGBColor{
		{R: 0, G: 0, B: 0},
		{R: 128, G: 0, B: 0},
		{R: 0, G: 128, B: 0},
		{R: 128, G: 128, B: 0},
		{R: 0, G: 0, B: 128},
		{R: 128, G: 0, B: 128},
		{R: 0, G: 128, B: 128},
		{R: 192, G: 192, B: 192},
		{R: 128, G: 128, B: 128},
		{R: 255, G: 0, B: 0},
		{R: 0, G: 255, B: 0},
		{R: 255, G: 255, B: 0},
		{R: 0, G: 0, B: 255},
		{R: 255, G: 0, B: 255},
		{R: 0, G: 255, B: 255},
		{R: 255, G: 255, B: 255},
	}
	index = min(max(index, 0), 255)
	if index < len(basic) {
		return basic[index]
	}
	if index < 232 {
		cube := index - 16
		component := func(value int) int {
			if value == 0 {
				return 0
			}
			return 55 + value*40
		}
		return gitui.RGBColor{
			R: component(cube / 36),
			G: component((cube % 36) / 6),
			B: component(cube % 6),
		}
	}
	gray := 8 + (index-232)*10
	return gitui.RGBColor{R: gray, G: gray, B: gray}
}
