package gicodingagent

import (
	"strings"
	"testing"

	gitui "github.com/nowa/gi/gi-tui"
)

func TestFooterComponentKeepsAllLinesWithinWidthForWideSessionNames(t *testing.T) {
	width := 93
	percent := 12.3
	footer := NewFooterComponent(FooterState{
		CWD:            "/tmp/project",
		GitBranch:      "main",
		SessionName:    strings.Repeat("한글", 30),
		ModelID:        "test-model",
		Provider:       "test",
		ContextWindow:  200000,
		ContextPercent: &percent,
	})

	for i, line := range footer.Render(width) {
		if got := gitui.VisibleWidth(line); got > width {
			t.Fatalf("line %d visible width = %d, want <= %d: %q", i, got, width, line)
		}
	}
}

func TestFooterComponentKeepsStatsLineWithinWidthForWideModelAndProviderNames(t *testing.T) {
	width := 60
	percent := 12.3
	footer := NewFooterComponent(FooterState{
		CWD:                    "/tmp/project",
		GitBranch:              "main",
		ModelID:                strings.Repeat("模", 30),
		Provider:               "공급자",
		Reasoning:              true,
		ThinkingLevel:          "high",
		ContextWindow:          200000,
		ContextPercent:         &percent,
		AvailableProviderCount: 2,
		Usage: []FooterUsage{{
			Input:     12345,
			Output:    6789,
			CostTotal: 1.234,
		}},
	})

	for i, line := range footer.Render(width) {
		if got := gitui.VisibleWidth(line); got > width {
			t.Fatalf("line %d visible width = %d, want <= %d: %q", i, got, width, line)
		}
	}
}

func TestFooterComponentUsesPiDarkThemeDimAnsi(t *testing.T) {
	percent := 0.0
	footer := NewFooterComponent(FooterState{
		CWD:                    "/tmp/project",
		GitBranch:              "main",
		ModelID:                "gpt-4o-mini",
		Provider:               "openai",
		ContextWindow:          128000,
		ContextPercent:         &percent,
		AvailableProviderCount: 2,
	})

	lines := footer.Render(80)
	if len(lines) < 2 {
		t.Fatalf("footer lines = %#v", lines)
	}
	for _, line := range lines[:2] {
		if !strings.HasPrefix(line, "\x1b[38;2;102;102;102m") {
			t.Fatalf("footer line missing Pi dim color: %q", line)
		}
	}
}
