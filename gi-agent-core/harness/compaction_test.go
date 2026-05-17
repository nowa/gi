package harness

import (
	"context"
	"reflect"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func mockUsage(input, output, cacheRead, cacheWrite int) llm.Usage {
	return llm.Usage{Input: input, Output: output, CacheRead: cacheRead, CacheWrite: cacheWrite, TotalTokens: input + output + cacheRead + cacheWrite}
}

func messageEntry(id string, parent *string, message llm.Message) Entry {
	return Entry{Type: "message", ID: id, ParentID: parent, Timestamp: nowISO(), Message: message}
}

func compactionEntry(id string, parent *string, summary, firstKept string) Entry {
	return Entry{Type: "compaction", ID: id, ParentID: parent, Timestamp: nowISO(), Summary: summary, FirstKeptEntryID: firstKept, TokensBefore: 1234}
}

func TestCompactionTokenCalculations(t *testing.T) {
	if got := CalculateContextTokens(mockUsage(1000, 500, 200, 100)); got != 1800 {
		t.Fatalf("CalculateContextTokens = %d", got)
	}
	settings := CompactionSettings{Enabled: true, ReserveTokens: 10000, KeepRecentTokens: 20000}
	if !ShouldCompact(95000, 100000, settings) || ShouldCompact(89000, 100000, settings) || ShouldCompact(95000, 100000, CompactionSettings{Enabled: false}) {
		t.Fatal("ShouldCompact threshold mismatch")
	}

	assistant := harnessAssistantMessage("assistant")
	assistant.Usage = mockUsage(10, 5, 3, 2)
	if EstimateTokens(llm.UserMessageText("plain user")) <= 0 {
		t.Fatal("expected user estimate")
	}
	if EstimateTokens(llm.Message{Role: llm.RoleToolResult, Content: []llm.ContentPart{llm.Image("abc", "image/png")}}) <= 1000 {
		t.Fatal("expected image tool result estimate > 1000")
	}
	if EstimateTokens(llm.Message{Role: "unknown"}) != 0 {
		t.Fatal("unknown role should estimate to zero")
	}
	entries := []Entry{messageEntry("u1", nil, llm.UserMessageText("user")), messageEntry("a1", stringPtr("u1"), assistant)}
	if got := GetLastAssistantUsage(entries); got == nil || *got != assistant.Usage {
		t.Fatalf("last usage = %#v", got)
	}
	aborted := assistant
	aborted.StopReason = llm.StopReasonAborted
	if got := GetLastAssistantUsage([]Entry{messageEntry("a1", nil, aborted)}); got != nil {
		t.Fatalf("aborted usage = %#v", got)
	}
	estimate := EstimateContextTokens([]llm.Message{assistant, llm.UserMessageText("tail")})
	if estimate.UsageTokens != 20 || estimate.LastUsageIndex == nil || *estimate.LastUsageIndex != 0 {
		t.Fatalf("estimate = %#v", estimate)
	}
}

func TestFindCutPointAndTurnStartEdgeCases(t *testing.T) {
	thinking := Entry{Type: "thinking_level_change", ID: "thinking", ThinkingLevel: "high"}
	modelChange := Entry{Type: "model_change", ID: "model", ParentID: stringPtr("thinking"), Provider: "openai", ModelID: "gpt-4"}
	got := FindCutPoint([]Entry{thinking, modelChange}, 0, 2, 1)
	if got.FirstKeptEntryIndex != 0 || got.TurnStartIndex != -1 || got.IsSplitTurn {
		t.Fatalf("cut = %#v", got)
	}
	branchSummary := Entry{Type: "branch_summary", ID: "branch", ParentID: stringPtr("model"), FromID: "branch", Summary: "branch summary"}
	customMessage := Entry{Type: "custom_message", ID: "custom", ParentID: stringPtr("branch"), CustomType: "note", Content: "custom content", Display: true}
	if FindTurnStartIndex([]Entry{thinking, branchSummary}, 1, 0) != 1 {
		t.Fatal("branch summary should be turn start")
	}
	if FindTurnStartIndex([]Entry{thinking, customMessage}, 1, 0) != 1 {
		t.Fatal("custom message should be turn start")
	}
	bash := messageEntry("bash", stringPtr("custom"), llm.Message{Role: "bashExecution", Content: []llm.ContentPart{llm.Text("go test ./...")}})
	if FindTurnStartIndex([]Entry{thinking, bash}, 1, 0) != 1 {
		t.Fatal("bash execution should be turn start")
	}
	if FindTurnStartIndex([]Entry{thinking, modelChange}, 1, 0) != -1 {
		t.Fatal("model change should not be turn start")
	}

	user := messageEntry("user", nil, llm.UserMessageText("user"))
	compaction := compactionEntry("compact", stringPtr("user"), "summary", "user")
	assistant := messageEntry("assistant", stringPtr("compact"), harnessAssistantMessage("assistant"))
	if cut := FindCutPoint([]Entry{user, compaction, assistant}, 0, 3, 1); cut.FirstKeptEntryIndex != 2 {
		t.Fatalf("cut after compaction = %#v", cut)
	}
}

func TestPrepareCompactionUsesPreviousSummary(t *testing.T) {
	u1 := messageEntry("u1", nil, llm.UserMessageText("user msg 1"))
	a1 := messageEntry("a1", stringPtr("u1"), harnessAssistantMessage("assistant msg 1"))
	u2 := messageEntry("u2", stringPtr("a1"), llm.UserMessageText("user msg 2"))
	a2Msg := harnessAssistantMessage("assistant msg 2")
	a2Msg.Usage = mockUsage(5000, 1000, 0, 0)
	a2 := messageEntry("a2", stringPtr("u2"), a2Msg)
	c1 := compactionEntry("c1", stringPtr("a2"), "First summary", "u2")
	u3 := messageEntry("u3", stringPtr("c1"), llm.UserMessageText("user msg 3"))
	a3Msg := harnessAssistantMessage("assistant msg 3")
	a3Msg.Usage = mockUsage(8000, 2000, 0, 0)
	a3 := messageEntry("a3", stringPtr("u3"), a3Msg)

	prep, err := PrepareCompaction([]Entry{u1, a1, u2, a2, c1, u3, a3}, CompactionSettings{Enabled: true, ReserveTokens: 100, KeepRecentTokens: 1})
	if err != nil {
		t.Fatal(err)
	}
	if prep == nil || prep.PreviousSummary != "First summary" || prep.FirstKeptEntryID == "" || prep.TokensBefore == 0 {
		t.Fatalf("prep = %#v", prep)
	}
}

func TestPrepareCompactionSplitTurnCarriesPriorFileOps(t *testing.T) {
	assistantMessage := harnessAssistantMessage("")
	assistantMessage.Content = []llm.ContentPart{llm.ToolCall("tool-1", "write", map[string]any{"path": "written.ts"})}

	u1 := messageEntry("u1", nil, llm.UserMessageText("user msg 1"))
	a1 := messageEntry("a1", stringPtr("u1"), assistantMessage)
	c1 := compactionEntry("c1", stringPtr("a1"), "First summary", "a1")
	c1.Details = map[string]any{"readFiles": []string{"old-read.ts"}, "modifiedFiles": []string{"old-edit.ts"}}
	u2 := messageEntry("u2", stringPtr("c1"), llm.UserMessageText("large turn"))
	a2 := messageEntry("a2", stringPtr("u2"), harnessAssistantMessage("large assistant message"))

	prep, err := PrepareCompaction([]Entry{u1, a1, c1, u2, a2}, CompactionSettings{Enabled: true, ReserveTokens: 100, KeepRecentTokens: 1})
	if err != nil {
		t.Fatal(err)
	}
	if prep == nil {
		t.Fatal("expected compaction preparation")
	}
	if !prep.IsSplitTurn || prep.PreviousSummary != "First summary" {
		t.Fatalf("prep = %#v", prep)
	}
	if got := messageRoles(prep.TurnPrefixMessages); !reflect.DeepEqual(got, []string{"user"}) {
		t.Fatalf("turn prefix roles = %#v", got)
	}
	if !prep.FileOps.Read["old-read.ts"] || !prep.FileOps.Edited["old-edit.ts"] || !prep.FileOps.Written["written.ts"] {
		t.Fatalf("file ops = %#v", prep.FileOps)
	}
}

func TestPrepareCompactionIncludesCustomAndBranchSummaryEntries(t *testing.T) {
	branchSummary := Entry{Type: "branch_summary", ID: "branch", Summary: "branch summary", FromID: "branch"}
	customMessage := Entry{Type: "custom_message", ID: "custom", ParentID: stringPtr("branch"), CustomType: "custom", Content: "custom content", Display: true}
	user := messageEntry("user", stringPtr("custom"), llm.UserMessageText("keep"))
	assistant := messageEntry("assistant", stringPtr("user"), harnessAssistantMessage("assistant"))

	prep, err := PrepareCompaction([]Entry{branchSummary, customMessage, user, assistant}, CompactionSettings{Enabled: true, ReserveTokens: 100, KeepRecentTokens: 1})
	if err != nil {
		t.Fatal(err)
	}
	if prep == nil {
		t.Fatal("expected compaction preparation")
	}
	if got := messageRoles(prep.MessagesToSummarize); !reflect.DeepEqual(got, []string{"branchSummary", "custom"}) {
		t.Fatalf("summarized roles = %#v", got)
	}
}

func TestPrepareCompactionSkipsEmptyOrAlreadyCompactedBranches(t *testing.T) {
	compaction := compactionEntry("compact", nil, "already compacted", "entry-keep")
	if prep, err := PrepareCompaction([]Entry{compaction}, DefaultCompactionSettings); err != nil || prep != nil {
		t.Fatalf("single compaction prep = %#v err = %v", prep, err)
	}
	if prep, err := PrepareCompaction(nil, DefaultCompactionSettings); err != nil || prep != nil {
		t.Fatalf("empty prep = %#v err = %v", prep, err)
	}
}

func TestSerializeConversationTruncatesToolResults(t *testing.T) {
	longContent := strings.Repeat("x", 5000)
	result := SerializeConversation([]llm.Message{{Role: llm.RoleToolResult, Content: []llm.ContentPart{llm.Text(longContent)}}})
	if !strings.Contains(result, "[Tool result]:") || !strings.Contains(result, "[... 3000 more characters truncated]") {
		t.Fatalf("serialized = %s", result)
	}
}

func TestGenerateSummaryAndCompact(t *testing.T) {
	model := llm.Model{ID: "summary", Name: "summary", Provider: "faux-summary", API: "faux-summary", Reasoning: true, MaxTokens: 128000}
	var seenOptions []llm.SimpleStreamOptions
	llm.RegisterAPIProvider("faux-summary", llm.APIProviderFuncs{StreamSimpleFunc: func(_ llm.Model, _ llm.Context, options llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
		seenOptions = append(seenOptions, options)
		return llm.CompletedAssistantStream(llm.AssistantMessage([]llm.ContentPart{llm.Text("## Goal\nTest summary")}, llm.StopReasonStop, model)), nil
	}})
	defer llm.UnregisterAPIProvider("faux-summary")

	summary, err := GenerateSummary(context.Background(), []llm.Message{llm.UserMessageText("Summarize this.")}, model, 200000, "test-key", "old summary", "focus", "medium")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(summary, "Test summary") || len(seenOptions) != 1 || seenOptions[0].Reasoning != "medium" || seenOptions[0].APIKey != "test-key" || seenOptions[0].MaxTokens != 128000 {
		t.Fatalf("summary/options = %q %#v", summary, seenOptions)
	}

	prep := CompactionPreparation{
		FirstKeptEntryID:    "entry-keep",
		MessagesToSummarize: []llm.Message{llm.UserMessageText("history")},
		TurnPrefixMessages:  []llm.Message{llm.UserMessageText("prefix")},
		IsSplitTurn:         true,
		TokensBefore:        600000,
		FileOps:             FileOps{Read: map[string]bool{"read.ts": true}, Written: map[string]bool{"write.ts": true}, Edited: map[string]bool{}},
		Settings:            CompactionSettings{Enabled: true, ReserveTokens: 500000, KeepRecentTokens: 20000},
	}
	result, err := Compact(context.Background(), prep, model, "test-key", "high")
	if err != nil {
		t.Fatal(err)
	}
	if result.FirstKeptEntryID != "entry-keep" || result.Summary == "" || !reflect.DeepEqual(result.Details["readFiles"], []string{"read.ts"}) {
		t.Fatalf("compact result = %#v", result)
	}
}
