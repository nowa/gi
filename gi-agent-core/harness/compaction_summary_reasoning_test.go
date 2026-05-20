package harness

import (
	"context"
	"reflect"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestGenerateSummaryUsesThinkingLevelForReasoningModels(t *testing.T) {
	model := compactionReasoningTestModel("compaction-reasoning-medium", true, 8192)
	seenOptions := registerCompactionSummaryTestProvider(t, model)

	_, err := GenerateSummary(context.Background(), []llm.Message{llm.UserMessageText("Summarize this.")}, model, 2000, "test-key", "", "", "medium")
	if err != nil {
		t.Fatal(err)
	}
	if len(*seenOptions) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(*seenOptions))
	}
	if got := (*seenOptions)[0].Reasoning; got != "medium" {
		t.Fatalf("reasoning = %q, want medium", got)
	}
	if got := (*seenOptions)[0].APIKey; got != "test-key" {
		t.Fatalf("api key = %q, want test-key", got)
	}
}

func TestGenerateSummaryOmitsReasoningWhenThinkingOff(t *testing.T) {
	model := compactionReasoningTestModel("compaction-reasoning-off", true, 8192)
	seenOptions := registerCompactionSummaryTestProvider(t, model)

	_, err := GenerateSummary(context.Background(), []llm.Message{llm.UserMessageText("Summarize this.")}, model, 2000, "test-key", "", "", "off")
	if err != nil {
		t.Fatal(err)
	}
	if len(*seenOptions) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(*seenOptions))
	}
	if got := (*seenOptions)[0].Reasoning; got != "" {
		t.Fatalf("reasoning = %q, want empty", got)
	}
}

func TestGenerateSummaryOmitsReasoningForNonReasoningModels(t *testing.T) {
	model := compactionReasoningTestModel("compaction-non-reasoning", false, 8192)
	seenOptions := registerCompactionSummaryTestProvider(t, model)

	_, err := GenerateSummary(context.Background(), []llm.Message{llm.UserMessageText("Summarize this.")}, model, 2000, "test-key", "", "", "medium")
	if err != nil {
		t.Fatal(err)
	}
	if len(*seenOptions) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(*seenOptions))
	}
	if got := (*seenOptions)[0].Reasoning; got != "" {
		t.Fatalf("reasoning = %q, want empty", got)
	}
}

func TestCompactClampsSummaryMaxTokensToModelOutputCap(t *testing.T) {
	model := compactionReasoningTestModel("compaction-max-token-clamp", false, 128000)
	seenOptions := registerCompactionSummaryTestProvider(t, model)
	preparation := CompactionPreparation{
		FirstKeptEntryID:    "entry-keep",
		MessagesToSummarize: []llm.Message{llm.UserMessageText("history")},
		TurnPrefixMessages:  []llm.Message{llm.UserMessageText("prefix")},
		IsSplitTurn:         true,
		TokensBefore:        600000,
		FileOps:             FileOps{Read: map[string]bool{}, Written: map[string]bool{}, Edited: map[string]bool{}},
		Settings:            CompactionSettings{Enabled: true, ReserveTokens: 500000, KeepRecentTokens: 20000},
	}

	if _, err := Compact(context.Background(), preparation, model, "test-key", "off"); err != nil {
		t.Fatal(err)
	}
	var maxTokens []int
	for _, options := range *seenOptions {
		maxTokens = append(maxTokens, options.MaxTokens)
	}
	if !reflect.DeepEqual(maxTokens, []int{128000, 128000}) {
		t.Fatalf("max tokens = %#v, want [128000 128000]", maxTokens)
	}
}

func compactionReasoningTestModel(api string, reasoning bool, maxTokens int) llm.Model {
	return llm.Model{
		ID:            api + "-model",
		Name:          api + " model",
		API:           api,
		Provider:      "anthropic",
		Reasoning:     reasoning,
		ContextWindow: 200000,
		MaxTokens:     maxTokens,
	}
}

func registerCompactionSummaryTestProvider(t *testing.T, model llm.Model) *[]llm.SimpleStreamOptions {
	t.Helper()
	seenOptions := []llm.SimpleStreamOptions{}
	llm.RegisterAPIProvider(model.API, llm.APIProviderFuncs{
		StreamSimpleFunc: func(_ llm.Model, _ llm.Context, options llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
			seenOptions = append(seenOptions, options)
			return llm.CompletedAssistantStream(llm.AssistantMessage([]llm.ContentPart{llm.Text("## Goal\nTest summary")}, llm.StopReasonStop, model)), nil
		},
	})
	t.Cleanup(func() {
		llm.UnregisterAPIProvider(model.API)
	})
	return &seenOptions
}
