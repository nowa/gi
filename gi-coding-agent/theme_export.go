package gicodingagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ThemeExportColors struct {
	PageBg *string
	CardBg *string
	InfoBg *string
}

type themeFile struct {
	Name   string         `json:"name"`
	Vars   map[string]any `json:"vars"`
	Colors map[string]any `json:"colors"`
	Export map[string]any `json:"export"`
}

func loadThemeExportFile(themeName, agentDir string) (themeFile, error) {
	if err := validateTUIThemeName(themeName); err != nil {
		return themeFile{}, err
	}
	content, err := os.ReadFile(filepath.Join(agentDir, "themes", themeName+".json"))
	if err != nil {
		return themeFile{}, err
	}
	var theme themeFile
	if err := json.Unmarshal(content, &theme); err != nil {
		return themeFile{}, err
	}
	if err := validateTUIThemeName(theme.Name); err != nil {
		return themeFile{}, err
	}
	return theme, nil
}

func GetThemeExportColors(themeName, agentDir string) (ThemeExportColors, error) {
	theme, err := loadThemeExportFile(themeName, agentDir)
	if err != nil {
		return ThemeExportColors{}, err
	}
	return themeExportColors(theme), nil
}

func themeExportColors(theme themeFile) ThemeExportColors {
	return ThemeExportColors{
		PageBg: resolveThemeExportColor(theme.Export["pageBg"], theme.Vars, map[string]bool{}),
		CardBg: resolveThemeExportColor(theme.Export["cardBg"], theme.Vars, map[string]bool{}),
		InfoBg: resolveThemeExportColor(theme.Export["infoBg"], theme.Vars, map[string]bool{}),
	}
}

func GetResolvedThemeCSSColors(themeName, agentDir string) (map[string]string, error) {
	if colors, ok := builtinThemeCSSColors(themeName); ok {
		return colors, nil
	}
	theme, err := loadThemeExportFile(themeName, agentDir)
	if err != nil {
		return nil, err
	}
	defaultText := "#e5e5e7"
	if themeName == "light" || theme.Name == "light" {
		defaultText = "#000000"
	}
	resolved := make(map[string]string, len(theme.Colors))
	for key, value := range theme.Colors {
		if color, ok := resolveThemeCSSColor(value, theme.Vars, defaultText, map[string]bool{}); ok {
			resolved[key] = color
		}
	}
	if _, ok := resolved["thinkingMax"]; !ok {
		if value, found := tuiThemeColorWithFallback(theme.Colors, "thinkingMax"); found {
			if color, resolvedOK := resolveThemeCSSColor(value, theme.Vars, defaultText, map[string]bool{}); resolvedOK {
				resolved["thinkingMax"] = color
			}
		}
	}
	return resolved, nil
}

func resolveThemeExportColor(value any, vars map[string]any, seen map[string]bool) *string {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil
		}
		if isThemeCSSColorLiteral(typed) {
			return stringPtr(typed)
		}
		if seen[typed] {
			return nil
		}
		seen[typed] = true
		return resolveThemeExportColor(vars[typed], vars, seen)
	case float64:
		return stringPtr(xterm256ColorToHex(int(typed)))
	case int:
		return stringPtr(xterm256ColorToHex(typed))
	default:
		return nil
	}
}

func resolveThemeCSSColor(value any, vars map[string]any, defaultText string, seen map[string]bool) (string, bool) {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return defaultText, true
		}
		if isThemeCSSColorLiteral(typed) {
			return typed, true
		}
		if seen[typed] {
			return "", false
		}
		if vars != nil {
			if next, ok := vars[typed]; ok {
				seen[typed] = true
				return resolveThemeCSSColor(next, vars, defaultText, seen)
			}
		}
		return typed, true
	case float64:
		return xterm256ColorToHex(int(typed)), true
	case int:
		return xterm256ColorToHex(typed), true
	default:
		return "", false
	}
}

func isThemeCSSColorLiteral(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(trimmed, "#") ||
		strings.HasPrefix(lower, "rgb(") ||
		strings.HasPrefix(lower, "rgba(") ||
		strings.HasPrefix(lower, "hsl(") ||
		strings.HasPrefix(lower, "hsla(") ||
		strings.HasPrefix(lower, "oklch(")
}

func builtinThemeCSSColors(themeName string) (map[string]string, bool) {
	name := strings.TrimSpace(themeName)
	if name == "" {
		name = "dark"
	}
	switch name {
	case "dark":
		return builtinThemeCSSColorsFromTUI(tuiDarkThemeFG, tuiDarkThemeBG, "#e5e5e7"), true
	case "light":
		return builtinThemeCSSColorsFromTUI(tuiLightThemeFG, tuiLightThemeBG, "#000000"), true
	default:
		return nil, false
	}
}

func builtinThemeCSSColorsFromTUI(fg, bg map[string]string, defaultText string) map[string]string {
	colors := make(map[string]string, len(fg)+len(bg))
	for key, value := range fg {
		colors[key] = tuiANSIEscapeToHex(value, defaultText)
	}
	for key, value := range bg {
		colors[key] = tuiANSIEscapeToHex(value, defaultText)
	}
	return colors
}

func tuiANSIEscapeToHex(value, defaultText string) string {
	if value == "" {
		return defaultText
	}
	trimmed := strings.TrimSuffix(value, "m")
	for _, prefix := range []string{"\x1b[38;2;", "\x1b[48;2;"} {
		if strings.HasPrefix(trimmed, prefix) {
			parts := strings.Split(strings.TrimPrefix(trimmed, prefix), ";")
			if len(parts) != 3 {
				return defaultText
			}
			r, errR := strconv.Atoi(parts[0])
			g, errG := strconv.Atoi(parts[1])
			b, errB := strconv.Atoi(parts[2])
			if errR != nil || errG != nil || errB != nil {
				return defaultText
			}
			return fmt.Sprintf("#%02x%02x%02x", clampRGB(r), clampRGB(g), clampRGB(b))
		}
	}
	return defaultText
}

func clampRGB(value int) int {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return value
}

func xterm256ColorToHex(index int) string {
	if index < 0 {
		index = 0
	}
	if index > 255 {
		index = 255
	}
	if index < 16 {
		base := []string{
			"#000000", "#800000", "#008000", "#808000", "#000080", "#800080", "#008080", "#c0c0c0",
			"#808080", "#ff0000", "#00ff00", "#ffff00", "#0000ff", "#ff00ff", "#00ffff", "#ffffff",
		}
		return base[index]
	}
	if index >= 232 {
		level := 8 + (index-232)*10
		return fmt.Sprintf("#%02x%02x%02x", level, level, level)
	}
	levels := []int{0, 95, 135, 175, 215, 255}
	n := index - 16
	return fmt.Sprintf("#%02x%02x%02x", levels[n/36], levels[(n/6)%6], levels[n%6])
}
