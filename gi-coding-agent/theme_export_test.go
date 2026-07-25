package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestThemeExportColorsResolveVariableReferences(t *testing.T) {
	agentDir := writeThemeFixture(t, "custom-export-vars", map[string]any{
		"name": "custom-export-vars",
		"vars": map[string]any{
			"pageBgVar":   "#112233",
			"pageBgAlias": "pageBgVar",
			"infoBgVar":   "#445566",
			"cardBgVar":   "#223344",
		},
		"colors": map[string]any{},
		"export": map[string]any{
			"pageBg": "pageBgAlias",
			"cardBg": "cardBgVar",
			"infoBg": "infoBgVar",
		},
	})

	colors, err := GetThemeExportColors("custom-export-vars", agentDir)
	if err != nil {
		t.Fatal(err)
	}
	assertStringPtr(t, colors.PageBg, "#112233")
	assertStringPtr(t, colors.CardBg, "#223344")
	assertStringPtr(t, colors.InfoBg, "#445566")
}

func TestThemeExportColorsResolveRecursiveVarsAndANSI256(t *testing.T) {
	agentDir := writeThemeFixture(t, "custom-export-recursive", map[string]any{
		"name": "custom-export-recursive",
		"vars": map[string]any{
			"deepPageBg":  "#abcdef",
			"pageBgAlias": "deepPageBg",
			"cardBgAnsi":  24,
		},
		"colors": map[string]any{},
		"export": map[string]any{
			"pageBg": "pageBgAlias",
			"cardBg": "cardBgAnsi",
			"infoBg": "",
		},
	})

	colors, err := GetThemeExportColors("custom-export-recursive", agentDir)
	if err != nil {
		t.Fatal(err)
	}
	assertStringPtr(t, colors.PageBg, "#abcdef")
	assertStringPtr(t, colors.CardBg, "#005f87")
	if colors.InfoBg != nil {
		t.Fatalf("InfoBg = %q, want nil", *colors.InfoBg)
	}
}

func TestResolvedThemeCSSColorsFallsBackFromThinkingMaxToXhigh(t *testing.T) {
	agentDir := writeThemeFixture(t, "legacy-thinking", map[string]any{
		"name": "legacy-thinking",
		"colors": map[string]any{
			"thinkingXhigh": "#123456",
		},
	})

	colors, err := GetResolvedThemeCSSColors("legacy-thinking", agentDir)
	if err != nil {
		t.Fatal(err)
	}
	if colors["thinkingXhigh"] != "#123456" || colors["thinkingMax"] != "#123456" {
		t.Fatalf("thinking colors = %#v", colors)
	}
}

func TestThemeExportRejectsSlashNameReservedForAutomaticSettings(t *testing.T) {
	if _, err := GetResolvedThemeCSSColors("../paper", t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "reserved for automatic light/dark theme settings") {
		t.Fatalf("theme export path error = %v", err)
	}

	agentDir := writeThemeFixture(t, "invalid-name", map[string]any{
		"name":   "paper/night",
		"colors": map[string]any{},
	})

	if _, err := GetResolvedThemeCSSColors("invalid-name", agentDir); err == nil ||
		!strings.Contains(err.Error(), "reserved for automatic light/dark theme settings") {
		t.Fatalf("theme export error = %v", err)
	}
}

func writeThemeFixture(t *testing.T, name string, theme map[string]any) string {
	t.Helper()
	agentDir := t.TempDir()
	themesDir := filepath.Join(agentDir, "themes")
	if err := os.MkdirAll(themesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	content, err := json.MarshalIndent(theme, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themesDir, name+".json"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	return agentDir
}

func assertStringPtr(t *testing.T, got *string, want string) {
	t.Helper()
	if got == nil || *got != want {
		if got == nil {
			t.Fatalf("got nil, want %q", want)
		}
		t.Fatalf("got %q, want %q", *got, want)
	}
}
