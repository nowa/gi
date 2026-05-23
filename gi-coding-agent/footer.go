package gicodingagent

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	gitui "github.com/nowa/gi/gi-tui"
)

type FooterUsage struct {
	Input      int
	Output     int
	CacheRead  int
	CacheWrite int
	CostTotal  float64
}

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
	Usage                  []FooterUsage
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

	totalInput, totalOutput, totalCacheRead, totalCacheWrite := 0, 0, 0, 0
	totalCost := 0.0
	for _, usage := range state.Usage {
		totalInput += usage.Input
		totalOutput += usage.Output
		totalCacheRead += usage.CacheRead
		totalCacheWrite += usage.CacheWrite
		totalCost += usage.CostTotal
	}

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
	if totalInput > 0 {
		statsParts = append(statsParts, "↑"+formatFooterTokens(totalInput))
	}
	if totalOutput > 0 {
		statsParts = append(statsParts, "↓"+formatFooterTokens(totalOutput))
	}
	if totalCacheRead > 0 {
		statsParts = append(statsParts, "R"+formatFooterTokens(totalCacheRead))
	}
	if totalCacheWrite > 0 {
		statsParts = append(statsParts, "W"+formatFooterTokens(totalCacheWrite))
	}
	if totalCost > 0 || state.UsingOAuth {
		cost := fmt.Sprintf("$%.3f", totalCost)
		if state.UsingOAuth {
			cost += " (sub)"
		}
		statsParts = append(statsParts, cost)
	}

	autoIndicator := ""
	if autoCompactEnabled {
		autoIndicator = " (auto)"
	}
	contextPercentValue := 0.0
	contextPercentDisplay := "?/" + formatFooterTokens(contextWindow) + autoIndicator
	if state.ContextPercent != nil {
		contextPercentValue = *state.ContextPercent
		contextPercentDisplay = fmt.Sprintf("%.1f%%/%s%s", contextPercentValue, formatFooterTokens(contextWindow), autoIndicator)
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
		gitui.TruncateToWidth(pwd, width, "..."),
		gitui.TruncateToWidth(statsLine, width, ""),
	}
	for _, status := range sortedFooterStatuses(state.ExtensionStatuses) {
		lines = append(lines, gitui.TruncateToWidth(sanitizeStatusText(status), width, "..."))
	}
	return lines
}

func cloneFooterState(state FooterState) FooterState {
	state.Usage = append([]FooterUsage(nil), state.Usage...)
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
