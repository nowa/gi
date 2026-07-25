package harness

import (
	"context"
	"errors"
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
	if got := CalculateContextTokens(llm.Usage{Input: 10, Output: 5, TotalTokens: 99}); got != 99 {
		t.Fatalf("CalculateContextTokens total = %d", got)
	}
	settings := CompactionSettings{Enabled: true, ReserveTokens: 10000, KeepRecentTokens: 20000}
	if !ShouldCompact(95000, 100000, settings) || ShouldCompact(90000, 100000, settings) || ShouldCompact(89000, 100000, settings) || ShouldCompact(95000, 100000, CompactionSettings{Enabled: false}) {
		t.Fatal("ShouldCompact threshold mismatch")
	}

	assistant := harnessAssistantMessage("assistant")
	assistant.Usage = mockUsage(10, 5, 3, 2)
	if EstimateTokens(llm.UserMessageText("plain user")) <= 0 {
		t.Fatal("expected user estimate")
	}
	if got := EstimateTokens(llm.Message{Role: llm.RoleToolResult, Content: []llm.ContentPart{llm.Image("abc", "image/png")}}); got != 1200 {
		t.Fatalf("image tool result estimate = %d, want 1200", got)
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

func TestEstimateTokensUsesPiContentHeuristic(t *testing.T) {
	if got := EstimateTokens(llm.UserMessageText("1234")); got != 1 {
		t.Fatalf("text estimate = %d, want 1", got)
	}
	if got := EstimateTokens(llm.UserMessageText("🙂🙂")); got != 1 {
		t.Fatalf("UTF-16 estimate = %d, want 1", got)
	}
	assistant := harnessAssistantMessage("")
	assistant.Content = []llm.ContentPart{
		llm.Text("abcd"),
		llm.Thinking("efgh"),
		llm.ToolCall("call", "tool", map[string]any{"x": "y"}),
	}
	wantChars := 4 + 4 + len("tool") + len(`{"x":"y"}`)
	if got, want := EstimateTokens(assistant), (wantChars+3)/4; got != want {
		t.Fatalf("assistant estimate = %d, want %d", got, want)
	}
}

func TestCompactionMessageClassification(t *testing.T) {
	tests := []struct {
		role      string
		cutPoint  bool
		turnStart bool
	}{
		{role: llm.RoleUser, cutPoint: true, turnStart: true},
		{role: llm.RoleAssistant, cutPoint: true},
		{role: "bashExecution", cutPoint: true, turnStart: true},
		{role: "custom", cutPoint: true, turnStart: true},
		{role: "branchSummary", cutPoint: true, turnStart: true},
		{role: "compactionSummary", cutPoint: true, turnStart: true},
		{role: llm.RoleToolResult},
		{role: "unknown"},
	}
	for _, test := range tests {
		t.Run(test.role, func(t *testing.T) {
			message := llm.Message{Role: test.role}
			if got := isCutPointMessage(message); got != test.cutPoint {
				t.Fatalf("isCutPointMessage() = %t, want %t", got, test.cutPoint)
			}
			if got := isTurnStartMessage(message); got != test.turnStart {
				t.Fatalf("isTurnStartMessage() = %t, want %t", got, test.turnStart)
			}
		})
	}
}

func TestCompactionTurnStartEntryUsesContextProjection(t *testing.T) {
	entries := []Entry{
		messageEntry("user", nil, llm.UserMessageText("user")),
		messageEntry("assistant", stringPtr("user"), harnessAssistantMessage("assistant")),
		messageEntry("tool", stringPtr("assistant"), llm.Message{Role: llm.RoleToolResult}),
		messageEntry("bash", stringPtr("tool"), llm.Message{Role: "bashExecution"}),
		messageEntry("custom-role", stringPtr("bash"), llm.Message{Role: "custom"}),
		messageEntry("branch-role", stringPtr("custom-role"), llm.Message{Role: "branchSummary"}),
		messageEntry("compaction-role", stringPtr("branch-role"), llm.Message{Role: "compactionSummary"}),
		{Type: "branch_summary", ID: "branch", Summary: "summary"},
		{Type: "branch_summary", ID: "empty-branch"},
		{Type: "custom_message", ID: "custom", Content: "content"},
		compactionEntry("compaction", stringPtr("custom"), "summary", "user"),
	}
	want := []bool{true, false, false, true, true, true, true, true, false, true, false}
	for index := range entries {
		if got := isTurnStartEntry(entries, index); got != want[index] {
			t.Fatalf("isTurnStartEntry(%d) = %t, want %t", index, got, want[index])
		}
	}
	if messages := compactionContextMessages(entries, len(entries)-1); messages != nil {
		t.Fatalf("compaction context messages = %#v, want nil", messages)
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

	largeBranchSummary := Entry{
		Type:    "branch_summary",
		ID:      "large-branch",
		Summary: strings.Repeat("x", 400),
	}
	cut := FindCutPoint(
		[]Entry{
			messageEntry("small-user", nil, llm.UserMessageText("u")),
			largeBranchSummary,
			messageEntry("small-assistant", stringPtr("large-branch"), harnessAssistantMessage("a")),
		},
		0,
		3,
		50,
	)
	if cut.FirstKeptEntryIndex != 1 || cut.TurnStartIndex != -1 || cut.IsSplitTurn {
		t.Fatalf("branch-summary cut = %#v", cut)
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
	if prep == nil || prep.PreviousSummary != "First summary" || prep.FirstKeptEntryID == "" || prep.TokensBefore == 0 || len(prep.RetainedTail) == 0 {
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
	if !strings.Contains(result, "[Tool result]:") || !strings.Contains(result, "[... 3000 more characters truncated]") || strings.Contains(result, strings.Repeat("x", 3000)) || !strings.Contains(result, strings.Repeat("x", 2000)) {
		t.Fatalf("serialized = %s", result)
	}
}

func TestSerializeConversationPiCompactionSerialization(t *testing.T) {
	shortContent := strings.Repeat("x", 1500)
	if result := SerializeConversation([]llm.Message{{Role: llm.RoleToolResult, Content: []llm.ContentPart{llm.Text(shortContent)}}}); result != "[Tool result]: "+shortContent {
		t.Fatalf("short serialized = %q", result)
	}

	longText := strings.Repeat("y", 5000)
	result := SerializeConversation([]llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentPart{llm.Text(longText)}},
		{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text(longText)}, StopReason: llm.StopReasonStop},
	})
	if strings.Contains(result, "truncated") || !strings.Contains(result, longText) {
		t.Fatalf("user/assistant serialized = %q", result)
	}
}

func TestGenerateSummaryAndCompact(t *testing.T) {
	model := llm.Model{ID: "summary", Name: "summary", Provider: "faux-summary", API: "faux-summary", Reasoning: true, MaxTokens: 128000}
	var seenOptions []llm.SimpleStreamOptions
	var seenPrompts []string
	llm.RegisterAPIProvider("faux-summary", llm.APIProviderFuncs{StreamSimpleFunc: func(_ llm.Model, context llm.Context, options llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
		seenOptions = append(seenOptions, options)
		if len(context.Messages) > 0 && len(context.Messages[0].Content) > 0 {
			seenPrompts = append(seenPrompts, context.Messages[0].Content[0].Text)
		}
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
	if len(seenPrompts) != 1 ||
		!strings.Contains(seenPrompts[0], "<conversation>\n[User]: Summarize this.\n</conversation>") ||
		!strings.Contains(seenPrompts[0], "<previous-summary>\nold summary\n</previous-summary>") ||
		!strings.Contains(seenPrompts[0], "NEW conversation messages to incorporate") ||
		!strings.Contains(seenPrompts[0], "Additional focus: focus") {
		t.Fatalf("summary prompt = %#v", seenPrompts)
	}

	prep := CompactionPreparation{
		FirstKeptEntryID:    "entry-keep",
		MessagesToSummarize: []llm.Message{llm.UserMessageText("history")},
		TurnPrefixMessages:  []llm.Message{llm.UserMessageText("prefix")},
		RetainedTail:        []llm.Message{llm.UserMessageText("tail")},
		IsSplitTurn:         true,
		TokensBefore:        600000,
		FileOps:             FileOps{Read: map[string]bool{"read.ts": true, "write.ts": true}, Written: map[string]bool{"write.ts": true}, Edited: map[string]bool{}},
		Settings:            CompactionSettings{Enabled: true, ReserveTokens: 500000, KeepRecentTokens: 20000},
	}
	result, err := Compact(context.Background(), prep, model, "test-key", "high")
	if err != nil {
		t.Fatal(err)
	}
	if result.FirstKeptEntryID != "entry-keep" || result.Summary == "" ||
		!reflect.DeepEqual(result.RetainedTail, prep.RetainedTail) ||
		!reflect.DeepEqual(result.Details["readFiles"], []string{"read.ts"}) ||
		!reflect.DeepEqual(result.Details["modifiedFiles"], []string{"write.ts"}) {
		t.Fatalf("compact result = %#v", result)
	}
	if !strings.Contains(result.Summary, "<read-files>\nread.ts\n</read-files>") || !strings.Contains(result.Summary, "<modified-files>\nwrite.ts\n</modified-files>") {
		t.Fatalf("compact summary should include Pi XML file operations:\n%s", result.Summary)
	}
	if _, ok := result.Details["writtenFiles"]; ok {
		t.Fatalf("compact details should use Pi readFiles/modifiedFiles shape: %#v", result.Details)
	}
}

func TestCompactionErrorsCarryPiStyleCodes(t *testing.T) {
	model := llm.Model{ID: "summary", Name: "summary", Provider: "faux-compaction-abort", API: "faux-compaction-abort", MaxTokens: 128000}
	llm.RegisterAPIProvider("faux-compaction-abort", llm.APIProviderFuncs{StreamSimpleFunc: func(llm.Model, llm.Context, llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
		message := llm.AssistantMessage([]llm.ContentPart{llm.Text("partial")}, llm.StopReasonAborted, model)
		message.ErrorMessage = "cancelled"
		return llm.ErrorAssistantStream(message), nil
	}})
	defer llm.UnregisterAPIProvider("faux-compaction-abort")

	_, err := GenerateSummary(context.Background(), []llm.Message{llm.UserMessageText("Summarize this.")}, model, 200000, "test-key", "", "", "")
	var compactionErr *CompactionError
	if !errors.As(err, &compactionErr) || compactionErr.Code != CompactionErrorAborted {
		t.Fatalf("GenerateSummary err = %#v, want CompactionError aborted", err)
	}

	_, err = CompactWithOptions(context.Background(), CompactionPreparation{}, model, CompactOptions{})
	compactionErr = nil
	if !errors.As(err, &compactionErr) || compactionErr.Code != CompactionErrorInvalidSession {
		t.Fatalf("CompactWithOptions err = %#v, want CompactionError invalid_session", err)
	}
}
