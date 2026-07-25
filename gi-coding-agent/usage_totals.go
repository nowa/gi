package gicodingagent

import (
	"sort"

	llm "github.com/nowa/gi/gi-llm-provider"
)

// UsageCostBreakdownEntry attributes billed usage to a concrete model or to
// application-owned work such as tools and summaries.
type UsageCostBreakdownEntry struct {
	Key    string  `json:"key"`
	Cost   float64 `json:"cost"`
	Tokens int     `json:"tokens"`
}

type sessionStatsSnapshot struct {
	entries     []FileEntry
	branch      []FileEntry
	sessionFile string
	sessionID   string
}

// sessionStatsSnapshot captures every input needed by the derived statistics
// pipeline under one read lock. Consumers never observe totals from one tree
// revision and context usage from another.
func (s *SessionManager) sessionStatsSnapshot() sessionStatsSnapshot {
	if s == nil {
		return sessionStatsSnapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := make([]FileEntry, 0, len(s.fileEntries))
	for _, entry := range s.fileEntries {
		if entry.Type != "session" {
			entries = append(entries, cloneFileEntry(entry))
		}
	}
	return sessionStatsSnapshot{
		entries:     entries,
		branch:      s.getBranchLocked(),
		sessionFile: s.sessionFile,
		sessionID:   s.sessionID,
	}
}

// aggregateAgentSessionStats derives session-wide billed usage and message
// counts from the append-only entry log. Context-window state is intentionally
// computed from the active branch by AgentSession.GetSessionStats.
func aggregateAgentSessionStats(entries []FileEntry) AgentSessionStats {
	stats := AgentSessionStats{Tokens: llm.EmptyUsage()}
	for _, entry := range entries {
		switch entry.Type {
		case "compaction", "branch_summary":
			if entry.Usage != nil {
				addUsage(&stats.Tokens, *entry.Usage)
			}
		case "message":
			message, ok := sessionMessageToLLM(entry.Message)
			if !ok {
				continue
			}
			stats.TotalMessages++
			switch message.Role {
			case llm.RoleUser:
				stats.UserMessages++
			case llm.RoleAssistant:
				stats.AssistantMessages++
				for _, part := range message.Content {
					if part.Type == llm.ContentToolCall {
						stats.ToolCalls++
					}
				}
				addUsage(&stats.Tokens, message.Usage)
				stats.LatestCacheHitRate = nil
				if promptTokens := usagePromptTokens(message.Usage); promptTokens > 0 {
					hitRate := float64(message.Usage.CacheRead) / float64(promptTokens) * 100
					stats.LatestCacheHitRate = &hitRate
				}
			case llm.RoleToolResult:
				stats.ToolResults++
				addUsage(&stats.Tokens, message.Usage)
			}
		}
	}
	return stats
}

func usageCostBreakdown(entries []FileEntry) []UsageCostBreakdownEntry {
	totalsByKey := make(map[string]llm.Usage)
	for _, entry := range entries {
		var (
			key   string
			usage llm.Usage
			ok    bool
		)
		switch entry.Type {
		case "compaction", "branch_summary":
			if entry.Usage != nil {
				key, usage, ok = "Tools/summaries", *entry.Usage, true
			}
		case "message":
			message, converted := sessionMessageToLLM(entry.Message)
			if !converted {
				continue
			}
			switch message.Role {
			case llm.RoleAssistant:
				modelID := message.Model
				if message.ResponseModel != "" {
					modelID = message.ResponseModel
				}
				if message.Provider != "" && modelID != "" {
					key, usage, ok = message.Provider+"/"+modelID, message.Usage, true
				}
			case llm.RoleToolResult:
				key, usage, ok = "Tools/summaries", message.Usage, true
			}
		}
		if !ok || !hasUsage(usage) {
			continue
		}
		total := totalsByKey[key]
		addUsage(&total, usage)
		totalsByKey[key] = total
	}

	breakdown := make([]UsageCostBreakdownEntry, 0, len(totalsByKey))
	for key, total := range totalsByKey {
		tokens := usageBillableTokens(total)
		if total.Cost.Total == 0 && tokens == 0 {
			continue
		}
		breakdown = append(breakdown, UsageCostBreakdownEntry{
			Key:    key,
			Cost:   total.Cost.Total,
			Tokens: tokens,
		})
	}
	sort.Slice(breakdown, func(i, j int) bool {
		if breakdown[i].Cost != breakdown[j].Cost {
			return breakdown[i].Cost > breakdown[j].Cost
		}
		return breakdown[i].Key < breakdown[j].Key
	})
	return breakdown
}

func addUsage(total *llm.Usage, usage llm.Usage) {
	if total == nil {
		return
	}
	total.Input += usage.Input
	total.Output += usage.Output
	total.CacheRead += usage.CacheRead
	total.CacheWrite += usage.CacheWrite
	total.CacheWrite1h += usage.CacheWrite1h
	total.TotalTokens += usageTokenTotal(usage)
	total.Cost.Input += usage.Cost.Input
	total.Cost.Output += usage.Cost.Output
	total.Cost.CacheRead += usage.Cost.CacheRead
	total.Cost.CacheWrite += usage.Cost.CacheWrite
	total.Cost.Total += usage.Cost.Total
	if usage.Reasoning != nil {
		if total.Reasoning == nil {
			total.Reasoning = new(int)
		}
		*total.Reasoning += *usage.Reasoning
	}
}

func hasUsage(usage llm.Usage) bool {
	return usageTokenTotal(usage) != 0 ||
		usage.CacheWrite1h != 0 ||
		usage.Reasoning != nil ||
		usage.Cost.Input != 0 ||
		usage.Cost.Output != 0 ||
		usage.Cost.CacheRead != 0 ||
		usage.Cost.CacheWrite != 0 ||
		usage.Cost.Total != 0
}

func usagePromptTokens(usage llm.Usage) int {
	return usage.Input + usage.CacheRead + usage.CacheWrite
}

func usageBillableTokens(usage llm.Usage) int {
	return usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
}
