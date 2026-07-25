package gicodingagent

import (
	"math"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	// CacheTTL is Anthropic's default prompt-cache TTL. It is presentation
	// context for callers; cache accounting itself remains provider-neutral.
	CacheTTL = 5 * time.Minute

	cacheMissNoiseFloorTokens = 1024
)

// CacheMiss describes prompt tokens that were present in the preceding
// request but were billed again instead of being served from cache.
type CacheMiss struct {
	MissedTokens int           `json:"missedTokens"`
	MissedCost   float64       `json:"missedCost"`
	Idle         time.Duration `json:"idle"`
	ModelChanged bool          `json:"modelChanged"`
}

// CacheWasteTotals is the immutable cumulative cache-miss projection for one
// session entry snapshot.
type CacheWasteTotals struct {
	MissedTokens int     `json:"missedTokens"`
	MissedCost   float64 `json:"missedCost"`
	MissCount    int     `json:"missCount"`
}

// ModelPriceSource is the narrow pricing boundary needed for full-miss
// estimates. ModelRuntime satisfies it directly.
type ModelPriceSource interface {
	GetModel(providerID, modelID string) (llm.Model, bool)
}

type previousCacheRequest struct {
	promptTokens  int
	modelKey      string
	timestamp     int64
	reportedCache bool
}

type cacheScanResult struct {
	previous *previousCacheRequest
	totals   CacheWasteTotals
	misses   map[string]CacheMiss
}

func detectCacheMissFromPrevious(
	previous *previousCacheRequest,
	message llm.Message,
	models ModelPriceSource,
) (CacheMiss, bool) {
	usage := message.Usage
	promptTokens := usagePromptTokens(usage)
	if previous == nil ||
		promptTokens <= 0 ||
		(usage.CacheRead+usage.CacheWrite == 0 && !previous.reportedCache) {
		return CacheMiss{}, false
	}

	missedTokens := min(previous.promptTokens, promptTokens) - usage.CacheRead
	if missedTokens <= cacheMissNoiseFloorTokens {
		return CacheMiss{}, false
	}

	paidTokens := usage.Input + usage.CacheWrite
	paidPerToken := 0.0
	if paidTokens > 0 {
		paidPerToken = (usage.Cost.Input + usage.Cost.CacheWrite) / float64(paidTokens)
	}
	readPerToken := 0.0
	if usage.CacheRead > 0 {
		readPerToken = usage.Cost.CacheRead / float64(usage.CacheRead)
	} else if models != nil {
		if model, ok := models.GetModel(message.Provider, message.Model); ok {
			readPerToken = model.Cost.CacheRead / 1_000_000
		}
	}

	idleMillis := message.Timestamp - previous.timestamp
	if idleMillis < 0 {
		idleMillis = 0
	}
	return CacheMiss{
		MissedTokens: missedTokens,
		MissedCost:   float64(missedTokens) * math.Max(0, paidPerToken-readPerToken),
		Idle:         time.Duration(idleMillis) * time.Millisecond,
		ModelChanged: message.Provider+"/"+message.Model != previous.modelKey,
	}, true
}

func asPreviousCacheRequest(
	message llm.Message,
	reportedCache bool,
) *previousCacheRequest {
	promptTokens := usagePromptTokens(message.Usage)
	if promptTokens <= 0 {
		return nil
	}
	return &previousCacheRequest{
		promptTokens: promptTokens,
		modelKey:     message.Provider + "/" + message.Model,
		timestamp:    message.Timestamp,
		reportedCache: reportedCache ||
			message.Usage.CacheRead+message.Usage.CacheWrite > 0,
	}
}

func scanCacheStats(
	entries []FileEntry,
	models ModelPriceSource,
) cacheScanResult {
	result := cacheScanResult{misses: make(map[string]CacheMiss)}
	for _, entry := range entries {
		switch entry.Type {
		case "compaction", "branch_summary":
			result.previous = nil
			continue
		case "message":
		default:
			continue
		}

		message, ok := sessionMessageToLLM(entry.Message)
		if !ok || message.Role != llm.RoleAssistant {
			continue
		}
		if miss, found := detectCacheMissFromPrevious(result.previous, message, models); found {
			result.totals.MissedTokens += miss.MissedTokens
			result.totals.MissedCost += miss.MissedCost
			result.totals.MissCount++
			if entry.ID != "" {
				result.misses[entry.ID] = miss
			}
		}
		reportedCache := false
		if result.previous != nil {
			reportedCache = result.previous.reportedCache
		}
		if previous := asPreviousCacheRequest(message, reportedCache); previous != nil {
			result.previous = previous
		}
	}
	return result
}

func computeCacheWaste(
	entries []FileEntry,
	models ModelPriceSource,
) CacheWasteTotals {
	return scanCacheStats(entries, models).totals
}

// collectCacheMisses keys misses by durable session entry ID. Unlike object
// identity, IDs survive JSON decoding and transcript rebuilds.
func collectCacheMisses(
	entries []FileEntry,
	models ModelPriceSource,
) map[string]CacheMiss {
	return scanCacheStats(entries, models).misses
}

// detectCacheMiss evaluates a just-completed assistant message. The entries
// snapshot must not contain message yet.
func detectCacheMiss(
	entries []FileEntry,
	message llm.Message,
	models ModelPriceSource,
) (CacheMiss, bool) {
	return detectCacheMissFromPrevious(scanCacheStats(entries, models).previous, message, models)
}
