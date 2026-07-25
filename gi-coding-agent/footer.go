package gicodingagent

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

type FooterState struct {
	CWD                    string
	GitBranch              string
	SessionName            string
	ModelID                string
	Provider               string
	Reasoning              bool
	ThinkingLevel          string
	ContextWindow          int
	ContextPercent         *float64
	AvailableProviderCount int
	UsingOAuth             bool
	Usage                  llm.Usage
	LatestCacheHitRate     *float64
	ExtensionStatuses      map[string]string
}

type FooterComponent struct {
	mu                 sync.RWMutex
	state              FooterState
	autoCompactEnabled bool
}

func NewFooterComponent(state FooterState) *FooterComponent {
	return &FooterComponent{state: cloneFooterState(state), autoCompactEnabled: true}
}

func (f *FooterComponent) SetState(state FooterState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = cloneFooterState(state)
}

func (f *FooterComponent) SetAutoCompactEnabled(enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.autoCompactEnabled = enabled
}

func (f *FooterComponent) Invalidate() {}

func (f *FooterComponent) Render(width int) []string {
	if f == nil || width <= 0 {
		return nil
	}
	f.mu.RLock()
	state := cloneFooterState(f.state)
	autoCompactEnabled := f.autoCompactEnabled
	f.mu.RUnlock()
	contextWindow := state.ContextWindow
	if contextWindow <= 0 {
		contextWindow = 0
	}

	usage := state.Usage

	pwd := state.CWD
	if pwd == "" {
		pwd = "."
	}
	if home := os.Getenv("HOME"); home != "" && strings.HasPrefix(pwd, home) {
		pwd = "~" + strings.TrimPrefix(pwd, home)
	}
	if state.GitBranch != "" {
		pwd += " (" + state.GitBranch + ")"
	}
	if state.SessionName != "" {
		pwd += " • " + state.SessionName
	}

	statsParts := make([]string, 0, 6)
	if usage.Input > 0 {
		statsParts = append(statsParts, "↑"+formatFooterTokens(usage.Input))
	}
	if usage.Output > 0 {
		statsParts = append(statsParts, "↓"+formatFooterTokens(usage.Output))
	}
	if usage.CacheRead > 0 {
		statsParts = append(statsParts, "R"+formatFooterTokens(usage.CacheRead))
	}
	if usage.CacheWrite > 0 {
		statsParts = append(statsParts, "W"+formatFooterTokens(usage.CacheWrite))
	}
	if (usage.CacheRead > 0 || usage.CacheWrite > 0) && state.LatestCacheHitRate != nil {
		statsParts = append(statsParts, fmt.Sprintf("CH%.1f%%", *state.LatestCacheHitRate))
	}
	usingSubscription := state.UsingOAuth || state.Provider == "kimi-coding"
	if usage.Cost.Total > 0 || usingSubscription {
		cost := fmt.Sprintf("$%.3f", usage.Cost.Total)
		if usingSubscription {
			cost += " (sub)"
		}
		statsParts = append(statsParts, cost)
	}

	autoIndicator := ""
	if autoCompactEnabled {
		autoIndicator = " (auto)"
	}
	contextPercentValue := 0.0
	contextPercentDisplay := fmt.Sprintf("%.1f%%/%s%s", contextPercentValue, formatFooterTokens(contextWindow), autoIndicator)
	if state.ContextPercent != nil {
		contextPercentValue = *state.ContextPercent
		contextPercentDisplay = fmt.Sprintf("%.1f%%/%s%s", contextPercentValue, formatFooterTokens(contextWindow), autoIndicator)
	}
	if contextPercentValue > 90 {
		contextPercentDisplay = tuiThemeError(contextPercentDisplay)
	} else if contextPercentValue > 70 {
		contextPercentDisplay = tuiThemeWarning(contextPercentDisplay)
	}
	statsParts = append(statsParts, contextPercentDisplay)

	statsLeft := strings.Join(statsParts, " ")
	statsLeftWidth := gitui.VisibleWidth(statsLeft)
	if statsLeftWidth > width {
		statsLeft = gitui.TruncateToWidth(statsLeft, width, "...")
		statsLeftWidth = gitui.VisibleWidth(statsLeft)
	}

	modelName := state.ModelID
	if modelName == "" {
		modelName = "no-model"
	}
	rightSideWithoutProvider := modelName
	if state.Reasoning {
		thinkingLevel := state.ThinkingLevel
		if thinkingLevel == "" {
			thinkingLevel = "off"
		}
		if thinkingLevel == "off" {
			rightSideWithoutProvider = modelName + " • thinking off"
		} else {
			rightSideWithoutProvider = modelName + " • " + thinkingLevel
		}
	}

	rightSide := rightSideWithoutProvider
	if state.AvailableProviderCount > 1 && state.Provider != "" {
		withProvider := "(" + state.Provider + ") " + rightSideWithoutProvider
		if statsLeftWidth+2+gitui.VisibleWidth(withProvider) <= width {
			rightSide = withProvider
		}
	}

	statsLine := statsLeft
	rightSideWidth := gitui.VisibleWidth(rightSide)
	if statsLeftWidth+2+rightSideWidth <= width {
		statsLine = statsLeft + strings.Repeat(" ", width-statsLeftWidth-rightSideWidth) + rightSide
	} else if availableForRight := width - statsLeftWidth - 2; availableForRight > 0 {
		truncatedRight := gitui.TruncateToWidth(rightSide, availableForRight, "")
		truncatedRightWidth := gitui.VisibleWidth(truncatedRight)
		statsLine = statsLeft + strings.Repeat(" ", max(0, width-statsLeftWidth-truncatedRightWidth)) + truncatedRight
	}

	lines := []string{
		gitui.TruncateToWidth(tuiThemeDim(pwd), width, tuiThemeDim("...")),
		tuiThemeFooterStatsLine(statsLine, statsLeft),
	}
	for _, status := range sortedFooterStatuses(state.ExtensionStatuses) {
		lines = append(lines, gitui.TruncateToWidth(tuiThemeDim(sanitizeStatusText(status)), width, tuiThemeDim("...")))
	}
	return lines
}

func tuiThemeFooterStatsLine(line, statsLeft string) string {
	if line == "" {
		return line
	}
	if statsLeft == "" || !strings.HasPrefix(line, statsLeft) {
		return tuiThemeDim(line)
	}
	return tuiThemeDim(statsLeft) + tuiThemeDim(line[len(statsLeft):])
}

func cloneFooterState(state FooterState) FooterState {
	if state.LatestCacheHitRate != nil {
		hitRate := *state.LatestCacheHitRate
		state.LatestCacheHitRate = &hitRate
	}
	if state.ExtensionStatuses != nil {
		statuses := make(map[string]string, len(state.ExtensionStatuses))
		for key, value := range state.ExtensionStatuses {
			statuses[key] = value
		}
		state.ExtensionStatuses = statuses
	}
	return state
}

func sortedFooterStatuses(statuses map[string]string) []string {
	if len(statuses) == 0 {
		return nil
	}
	keys := make([]string, 0, len(statuses))
	for key := range statuses {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, statuses[key])
	}
	return values
}

func sanitizeStatusText(text string) string {
	fields := strings.Fields(strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(text))
	return strings.Join(fields, " ")
}

func formatFooterTokens(count int) string {
	if count < 1000 {
		return fmt.Sprintf("%d", count)
	}
	if count < 10000 {
		return fmt.Sprintf("%.1fk", float64(count)/1000)
	}
	if count < 1000000 {
		return fmt.Sprintf("%dk", int(float64(count)/1000+0.5))
	}
	if count < 10000000 {
		return fmt.Sprintf("%.1fM", float64(count)/1000000)
	}
	return fmt.Sprintf("%dM", int(float64(count)/1000000+0.5))
}
