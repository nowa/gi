package gicodingagent

import (
	"math"
	"strconv"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type cacheStatsModelPrices struct {
	model llm.Model
}

func (s cacheStatsModelPrices) GetModel(providerID, modelID string) (llm.Model, bool) {
	if providerID != s.model.Provider || modelID != s.model.ID {
		return llm.Model{}, false
	}
	return s.model, true
}

func TestComputeCacheWasteAccumulatesMissedTokensAndCostAcrossTurns(t *testing.T) {
	turn1, turn2 := cacheStatsHealthyTurns()
	turn3 := cacheStatsAssistant(cacheStatsAssistantOptions{
		cacheWrite: 110_000,
		cost:       llm.UsageCost{CacheWrite: 0.4125},
		timestamp:  120_000,
	})

	totals := computeCacheWaste(
		cacheStatsEntries(turn1, turn2, turn3),
		cacheStatsPrices(),
	)
	if totals.MissedTokens != 105_000 || totals.MissCount != 1 {
		t.Fatalf("cache waste = %#v", totals)
	}
	assertCacheStatsFloatNear(t, totals.MissedCost, 0.36225)
}

func TestComputeCacheWasteCountsNothingForHealthySessions(t *testing.T) {
	turn1, turn2 := cacheStatsHealthyTurns()
	totals := computeCacheWaste(cacheStatsEntries(turn1, turn2), cacheStatsPrices())
	if totals != (CacheWasteTotals{}) {
		t.Fatalf("cache waste = %#v, want zero", totals)
	}
}

func TestComputeCacheWasteSkipsTurnAfterCompactionReset(t *testing.T) {
	turn1, _ := cacheStatsHealthyTurns()
	afterReset := cacheStatsAssistant(cacheStatsAssistantOptions{
		cacheWrite: 20_000,
		cost:       llm.UsageCost{CacheWrite: 0.075},
	})
	entries := []FileEntry{
		cacheStatsEntry("one", turn1),
		{Type: "compaction", ID: "reset"},
		cacheStatsEntry("two", afterReset),
	}

	totals := computeCacheWaste(entries, cacheStatsPrices())
	if totals != (CacheWasteTotals{}) {
		t.Fatalf("cache waste = %#v, want zero", totals)
	}
}

func TestComputeCacheWasteCountsMissesCausedByModelSwitches(t *testing.T) {
	turn1, _ := cacheStatsHealthyTurns()
	otherModel := cacheStatsAssistant(cacheStatsAssistantOptions{
		cacheWrite: 100_000,
		cost:       llm.UsageCost{CacheWrite: 0.375},
		model:      "other-model",
	})

	totals := computeCacheWaste(
		cacheStatsEntries(turn1, otherModel),
		cacheStatsPrices(),
	)
	if totals.MissedTokens != 100_000 || totals.MissCount != 1 {
		t.Fatalf("cache waste = %#v", totals)
	}
}

func TestComputeCacheWasteSkipsProvidersThatReportNoCacheActivity(t *testing.T) {
	first := cacheStatsAssistant(cacheStatsAssistantOptions{input: 100_000})
	second := cacheStatsAssistant(cacheStatsAssistantOptions{input: 110_000})

	totals := computeCacheWaste(
		cacheStatsEntries(first, second),
		cacheStatsPrices(),
	)
	if totals != (CacheWasteTotals{}) {
		t.Fatalf("cache waste = %#v, want zero", totals)
	}
}

func TestCollectCacheMissesMapsCountedMissesToAssistantEntryIDs(t *testing.T) {
	turn1, turn2 := cacheStatsHealthyTurns()
	missTurn := cacheStatsAssistant(cacheStatsAssistantOptions{
		cacheWrite: 110_000,
		cost:       llm.UsageCost{CacheWrite: 0.4125},
		timestamp:  120_000,
	})
	entries := cacheStatsEntries(turn1, turn2, missTurn)

	misses := collectCacheMisses(entries, cacheStatsPrices())
	if len(misses) != 1 || misses["entry-2"].MissedTokens != 105_000 {
		t.Fatalf("cache misses = %#v", misses)
	}
}

func TestDetectCacheMissDetectsJustCompletedMessageWithIdleTime(t *testing.T) {
	turn1, turn2 := cacheStatsHealthyTurns()
	message := cacheStatsAssistant(cacheStatsAssistantOptions{
		cacheWrite: 110_000,
		cost:       llm.UsageCost{CacheWrite: 0.4125},
		timestamp:  600_000,
	})

	miss, ok := detectCacheMiss(
		cacheStatsEntries(turn1, turn2),
		message,
		cacheStatsPrices(),
	)
	if !ok || miss.MissedTokens != 105_000 || miss.Idle != 9*time.Minute {
		t.Fatalf("cache miss = %#v, found=%v", miss, ok)
	}
	assertCacheStatsFloatNear(t, miss.MissedCost, 0.36225)
}

func TestDetectCacheMissFlagsModelSwitches(t *testing.T) {
	turn1, turn2 := cacheStatsHealthyTurns()
	message := cacheStatsAssistant(cacheStatsAssistantOptions{
		cacheWrite: 110_000,
		cost:       llm.UsageCost{CacheWrite: 0.4125},
		model:      "other-model",
		timestamp:  120_000,
	})

	miss, ok := detectCacheMiss(
		cacheStatsEntries(turn1, turn2),
		message,
		cacheStatsPrices(),
	)
	if !ok || miss.MissedTokens != 105_000 || !miss.ModelChanged {
		t.Fatalf("cache miss = %#v, found=%v", miss, ok)
	}
}

func TestDetectCacheMissReturnsFalseForHealthyTurns(t *testing.T) {
	turn1, turn2 := cacheStatsHealthyTurns()
	healthy := cacheStatsAssistant(cacheStatsAssistantOptions{
		cacheRead:  105_000,
		cacheWrite: 2_000,
		cost: llm.UsageCost{
			CacheRead:  0.0315,
			CacheWrite: 0.0075,
		},
		timestamp: 120_000,
	})

	if miss, ok := detectCacheMiss(
		cacheStatsEntries(turn1, turn2),
		healthy,
		cacheStatsPrices(),
	); ok {
		t.Fatalf("cache miss = %#v, want none", miss)
	}
}

func TestDetectCacheMissReturnsFalseForFirstTurnOfSession(t *testing.T) {
	turn1, _ := cacheStatsHealthyTurns()
	if miss, ok := detectCacheMiss(nil, turn1, cacheStatsPrices()); ok {
		t.Fatalf("cache miss = %#v, want none", miss)
	}
}

type cacheStatsAssistantOptions struct {
	input      int
	cacheRead  int
	cacheWrite int
	cost       llm.UsageCost
	model      string
	timestamp  int64
}

func cacheStatsAssistant(options cacheStatsAssistantOptions) llm.Message {
	modelID := options.model
	if modelID == "" {
		modelID = "test-model"
	}
	return llm.Message{
		Role:      llm.RoleAssistant,
		Provider:  "test",
		Model:     modelID,
		Timestamp: options.timestamp,
		Usage: llm.Usage{
			Input:      options.input,
			Output:     10,
			CacheRead:  options.cacheRead,
			CacheWrite: options.cacheWrite,
			Cost:       options.cost,
		},
	}
}

func cacheStatsHealthyTurns() (llm.Message, llm.Message) {
	return cacheStatsAssistant(cacheStatsAssistantOptions{
			cacheWrite: 100_000,
			cost:       llm.UsageCost{CacheWrite: 0.375},
		}),
		cacheStatsAssistant(cacheStatsAssistantOptions{
			cacheRead:  100_000,
			cacheWrite: 5_000,
			cost: llm.UsageCost{
				CacheRead:  0.03,
				CacheWrite: 0.019,
			},
			timestamp: 60_000,
		})
}

func cacheStatsPrices() cacheStatsModelPrices {
	return cacheStatsModelPrices{model: llm.Model{
		Provider: "test",
		ID:       "test-model",
		Cost:     llm.ModelCost{CacheRead: 0.3},
	}}
}

func cacheStatsEntries(messages ...llm.Message) []FileEntry {
	entries := make([]FileEntry, len(messages))
	for index, message := range messages {
		entries[index] = cacheStatsEntry("entry-"+strconv.Itoa(index), message)
	}
	return entries
}

func cacheStatsEntry(id string, message llm.Message) FileEntry {
	return FileEntry{Type: "message", ID: id, Message: message}
}

func assertCacheStatsFloatNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.00001 {
		t.Fatalf("float = %f, want %f", got, want)
	}
}
