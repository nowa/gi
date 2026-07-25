package gicodingagent

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMaxThinkingLevelIsAcceptedByCLIAndSettings(t *testing.T) {
	if !IsValidThinkingLevel("max") {
		t.Fatal("max thinking level is not valid")
	}
	parsed := ParseArgs([]string{"--thinking", "max"})
	if parsed.Thinking != ThinkingMax || len(parsed.Diagnostics) != 0 {
		t.Fatalf("parsed max thinking = %#v", parsed)
	}

	settings := NewInMemorySettingsManager(nil)
	settings.SetDefaultThinkingLevel("max")
	if err := settings.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := settings.GetDefaultThinkingLevel(); got != "max" {
		t.Fatalf("default thinking level = %q, want max", got)
	}

	model := resolverMockModels[0]
	mappedMax := "max"
	model.ThinkingLevelMap = map[string]*string{"max": &mappedMax}
	if got := clampCLIThinkingLevel(&model, ThinkingMax); got != ThinkingMax {
		t.Fatalf("clamped thinking level = %q, want max", got)
	}
}

func TestMaxThinkingLevelFallsBackToThinkingXHighForLegacyThemes(t *testing.T) {
	themePath := filepath.Join(t.TempDir(), "legacy-theme.json")
	fixture := completeTUIThemeFixture("legacy-theme", map[string]any{
		"thinkingXhigh": "#123456",
	})
	delete(fixture["colors"].(map[string]any), "thinkingMax")
	writeJSON(t, themePath, fixture)

	setTUIThemeForTest(t, "legacy-theme", []TUIThemeInfo{{
		Name: "legacy-theme",
		Path: themePath,
	}})
	maxBorder := tuiThemeThinkingBorder("max")("border")
	xhighBorder := tuiThemeThinkingBorder("xhigh")("border")
	if maxBorder != xhighBorder || !strings.Contains(maxBorder, "border") {
		t.Fatalf("max/xhigh legacy borders = %q / %q", maxBorder, xhighBorder)
	}
}
