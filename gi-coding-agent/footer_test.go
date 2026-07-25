package gicodingagent

import (
	"path/filepath"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func TestFormatCWDForFooterOnlyShortensHomeAndDescendants(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "me")
	nested := filepath.Join(home, "projects", "gi")
	sibling := filepath.Join(root, "me-other")

	if got := FormatCWDForFooter(home, home); got != "~" {
		t.Fatalf("home path = %q", got)
	}
	if got, want := FormatCWDForFooter(nested, home),
		"~"+string(filepath.Separator)+filepath.Join("projects", "gi"); got != want {
		t.Fatalf("nested path = %q, want %q", got, want)
	}
	if got := FormatCWDForFooter(sibling, home); got != sibling {
		t.Fatalf("sibling prefix path = %q, want %q", got, sibling)
	}
	if got := FormatCWDForFooter(nested, ""); got != nested {
		t.Fatalf("path without home = %q, want %q", got, nested)
	}
}

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
		Usage: llm.Usage{
			Input:  12345,
			Output: 6789,
			Cost:   llm.UsageCost{Total: 1.234},
		},
	})

	for i, line := range footer.Render(width) {
		if got := gitui.VisibleWidth(line); got > width {
			t.Fatalf("line %d visible width = %d, want <= %d: %q", i, got, width, line)
		}
	}
}

func TestFooterComponentUsesPiDarkThemeDimAnsi(t *testing.T) {
	setTUIThemeForTest(t, "dark", nil)
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

func TestFooterIncludesSummaryAndToolResultUsageInTotalCost(t *testing.T) {
	model := llm.Model{Provider: "test", ID: "model"}
	assistant := llm.AssistantMessage(nil, llm.StopReasonStop, model)
	assistant.Usage = llm.Usage{
		Input: 100,
		Cost:  llm.UsageCost{Total: 0.5},
	}
	toolResult := llm.Message{
		Role:  llm.RoleToolResult,
		Usage: llm.Usage{Input: 15, Cost: llm.UsageCost{Total: 0.375}},
	}
	branchUsage := llm.Usage{Input: 20, Cost: llm.UsageCost{Total: 0.25}}
	compactionUsage := llm.Usage{Input: 5, Cost: llm.UsageCost{Total: 0.125}}
	stats := aggregateAgentSessionStats([]FileEntry{
		{Type: "message", Message: assistant},
		{Type: "message", Message: toolResult},
		{Type: "branch_summary", Usage: &branchUsage},
		{Type: "compaction", Usage: &compactionUsage},
	})
	footer := NewFooterComponent(FooterState{
		CWD:           "/tmp/project",
		ModelID:       model.ID,
		Provider:      model.Provider,
		ContextWindow: 200_000,
		Usage:         stats.Tokens,
	})

	rendered := StripAnsi(strings.Join(footer.Render(120), "\n"))
	if !strings.Contains(rendered, "$1.250") {
		t.Fatalf("footer = %q, want total cost", rendered)
	}
}

func TestFooterShowsLatestCacheHitRateWhenCacheUsageIsPresent(t *testing.T) {
	hitRate := 25.0
	footer := NewFooterComponent(FooterState{
		CWD:                "/tmp/project",
		ModelID:            "model",
		Provider:           "test",
		ContextWindow:      200_000,
		LatestCacheHitRate: &hitRate,
		Usage: llm.Usage{
			Input:      100,
			Output:     10,
			CacheRead:  50,
			CacheWrite: 50,
		},
	})

	rendered := StripAnsi(strings.Join(footer.Render(120), "\n"))
	if !strings.Contains(rendered, "CH25.0%") {
		t.Fatalf("footer = %q, want cache hit rate", rendered)
	}
}

func TestFooterMarksKimiCodingCostsAsSubscriptionEstimates(t *testing.T) {
	footer := NewFooterComponent(FooterState{
		CWD:           "/tmp/project",
		ModelID:       "kimi-k2.5",
		Provider:      "kimi-coding",
		ContextWindow: 200_000,
		Usage: llm.Usage{
			Input: 100,
			Cost:  llm.UsageCost{Total: 1.234},
		},
	})

	rendered := StripAnsi(strings.Join(footer.Render(120), "\n"))
	if !strings.Contains(rendered, "$1.234 (sub)") {
		t.Fatalf("footer = %q, want subscription estimate", rendered)
	}
}
