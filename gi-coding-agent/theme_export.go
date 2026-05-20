package gicodingagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func GetThemeExportColors(themeName, agentDir string) (ThemeExportColors, error) {
	content, err := os.ReadFile(filepath.Join(agentDir, "themes", themeName+".json"))
	if err != nil {
		return ThemeExportColors{}, err
	}
	var theme themeFile
	if err := json.Unmarshal(content, &theme); err != nil {
		return ThemeExportColors{}, err
	}
	return ThemeExportColors{
		PageBg: resolveThemeExportColor(theme.Export["pageBg"], theme.Vars, map[string]bool{}),
		CardBg: resolveThemeExportColor(theme.Export["cardBg"], theme.Vars, map[string]bool{}),
		InfoBg: resolveThemeExportColor(theme.Export["infoBg"], theme.Vars, map[string]bool{}),
	}, nil
}

func resolveThemeExportColor(value any, vars map[string]any, seen map[string]bool) *string {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil
		}
		if strings.HasPrefix(typed, "#") {
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
