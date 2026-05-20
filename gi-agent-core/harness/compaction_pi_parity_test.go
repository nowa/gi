package harness

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestPiCompactionGetLastAssistantUsageCases(t *testing.T) {
	builder := newPiCompactionEntryBuilder()
	entries := []Entry{
		builder.message(llm.UserMessageText("Hello")),
		builder.message(piCompactionAssistant("Hi", mockUsage(100, 50, 0, 0))),
		builder.message(llm.UserMessageText("How are you?")),
		builder.message(piCompactionAssistant("Good", mockUsage(200, 100, 0, 0))),
	}
	if usage := GetLastAssistantUsage(entries); usage == nil || usage.Input != 200 {
		t.Fatalf("last usage = %#v, want input 200", usage)
	}

	aborted := piCompactionAssistant("Aborted", mockUsage(300, 150, 0, 0))
	aborted.StopReason = llm.StopReasonAborted
	entries = append(entries, builder.message(llm.UserMessageText("Later")), builder.message(aborted))
	if usage := GetLastAssistantUsage(entries); usage == nil || usage.Input != 200 {
		t.Fatalf("usage after aborted = %#v, want previous input 200", usage)
	}

	if usage := GetLastAssistantUsage([]Entry{builder.message(llm.UserMessageText("Only user"))}); usage != nil {
		t.Fatalf("usage without assistant = %#v, want nil", usage)
	}
}

func TestPiCompactionTokenAndThresholdCases(t *testing.T) {
	if got := CalculateContextTokens(mockUsage(1000, 500, 200, 100)); got != 1800 {
		t.Fatalf("context tokens = %d, want 1800", got)
	}
	if got := CalculateContextTokens(mockUsage(0, 0, 0, 0)); got != 0 {
		t.Fatalf("zero context tokens = %d, want 0", got)
	}
	settings := CompactionSettings{Enabled: true, ReserveTokens: 10000, KeepRecentTokens: 20000}
	if !ShouldCompact(95000, 100000, settings) {
		t.Fatal("expected compaction at threshold")
	}
	if ShouldCompact(89000, 100000, settings) {
		t.Fatal("did not expect compaction below threshold")
	}
	if ShouldCompact(95000, 100000, CompactionSettings{Enabled: false, ReserveTokens: 10000, KeepRecentTokens: 20000}) {
		t.Fatal("disabled compaction should not trigger")
	}
}

func TestPiCompactionFindCutPointCases(t *testing.T) {
	builder := newPiCompactionEntryBuilder()
	var entries []Entry
	for i := 0; i < 10; i++ {
		entries = append(entries,
			builder.message(llm.UserMessageText(fmt.Sprintf("User %d", i))),
			builder.message(piCompactionAssistant(fmt.Sprintf("Assistant %d", i), mockUsage(0, 100, (i+1)*1000, 0))),
		)
	}
	cut := FindCutPoint(entries, 0, len(entries), 2500)
	if entries[cut.FirstKeptEntryIndex].Type != "message" {
		t.Fatalf("cut entry = %#v, want message", entries[cut.FirstKeptEntryIndex])
	}
	role := entries[cut.FirstKeptEntryIndex].Message.Role
	if role != llm.RoleUser && role != llm.RoleAssistant {
		t.Fatalf("cut role = %q, want user or assistant", role)
	}

	noValid := FindCutPoint([]Entry{{Type: "model_change", ID: "model", Provider: "openai", ModelID: "gpt-4"}}, 0, 1, 1000)
	if noValid.FirstKeptEntryIndex != 0 {
		t.Fatalf("no valid cut = %#v, want first index 0", noValid)
	}

	builder = newPiCompactionEntryBuilder()
	fitting := []Entry{
		builder.message(llm.UserMessageText("1")),
		builder.message(piCompactionAssistant("a", mockUsage(0, 50, 500, 0))),
		builder.message(llm.UserMessageText("2")),
		builder.message(piCompactionAssistant("b", mockUsage(0, 50, 1000, 0))),
	}
	if fit := FindCutPoint(fitting, 0, len(fitting), 50000); fit.FirstKeptEntryIndex != 0 {
		t.Fatalf("fit cut = %#v, want first index 0", fit)
	}

	builder = newPiCompactionEntryBuilder()
	splitEntries := []Entry{
		builder.message(llm.UserMessageText("Turn 1")),
		builder.message(piCompactionAssistant("A1", mockUsage(0, 100, 1000, 0))),
		builder.message(llm.UserMessageText("Turn 2")),
		builder.message(piCompactionAssistant("A2-1", mockUsage(0, 100, 5000, 0))),
		builder.message(piCompactionAssistant("A2-2", mockUsage(0, 100, 8000, 0))),
		builder.message(piCompactionAssistant("A2-3", mockUsage(0, 100, 10000, 0))),
	}
	split := FindCutPoint(splitEntries, 0, len(splitEntries), 3000)
	if splitEntries[split.FirstKeptEntryIndex].Message.Role == llm.RoleAssistant {
		if !split.IsSplitTurn || split.TurnStartIndex != 2 {
			t.Fatalf("split cut = %#v, want split turn starting at 2", split)
		}
	}
}

func TestPiCompactionBuildSessionContextCases(t *testing.T) {
	builder := newPiCompactionEntryBuilder()
	entries := []Entry{
		builder.message(llm.UserMessageText("1")),
		builder.message(piCompactionAssistant("a", mockUsage(100, 50, 0, 0))),
		builder.message(llm.UserMessageText("2")),
		builder.message(piCompactionAssistant("b", mockUsage(100, 50, 0, 0))),
	}
	context := BuildSessionContext(entries)
	if len(context.Messages) != 4 || context.ThinkingLevel != "off" || context.ModelProvider != "anthropic" || context.ModelID != "claude-sonnet-4-5" {
		t.Fatalf("context = %#v", context)
	}

	builder = newPiCompactionEntryBuilder()
	u1 := builder.message(llm.UserMessageText("1"))
	a1 := builder.message(piCompactionAssistant("a", mockUsage(100, 50, 0, 0)))
	u2 := builder.message(llm.UserMessageText("2"))
	a2 := builder.message(piCompactionAssistant("b", mockUsage(100, 50, 0, 0)))
	compact := builder.compaction("Summary of 1,a,2,b", u2.ID)
	u3 := builder.message(llm.UserMessageText("3"))
	a3 := builder.message(piCompactionAssistant("c", mockUsage(100, 50, 0, 0)))
	context = BuildSessionContext([]Entry{u1, a1, u2, a2, compact, u3, a3})
	if len(context.Messages) != 5 || context.Messages[0].Role != "compactionSummary" || !strings.Contains(piCompactionMessageText(context.Messages[0]), "Summary of 1,a,2,b") {
		t.Fatalf("single compaction context = %#v", context.Messages)
	}

	builder = newPiCompactionEntryBuilder()
	u1 = builder.message(llm.UserMessageText("1"))
	a1 = builder.message(piCompactionAssistant("a", mockUsage(100, 50, 0, 0)))
	compact1 := builder.compaction("First summary", u1.ID)
	u2 = builder.message(llm.UserMessageText("2"))
	b := builder.message(piCompactionAssistant("b", mockUsage(100, 50, 0, 0)))
	u3 = builder.message(llm.UserMessageText("3"))
	c := builder.message(piCompactionAssistant("c", mockUsage(100, 50, 0, 0)))
	compact2 := builder.compaction("Second summary", u3.ID)
	u4 := builder.message(llm.UserMessageText("4"))
	d := builder.message(piCompactionAssistant("d", mockUsage(100, 50, 0, 0)))
	context = BuildSessionContext([]Entry{u1, a1, compact1, u2, b, u3, c, compact2, u4, d})
	if len(context.Messages) != 5 || !strings.Contains(piCompactionMessageText(context.Messages[0]), "Second summary") {
		t.Fatalf("multiple compaction context = %#v", context.Messages)
	}

	builder = newPiCompactionEntryBuilder()
	u1 = builder.message(llm.UserMessageText("1"))
	a1 = builder.message(piCompactionAssistant("a", mockUsage(100, 50, 0, 0)))
	compact1 = builder.compaction("First summary", u1.ID)
	u2 = builder.message(llm.UserMessageText("2"))
	b = builder.message(piCompactionAssistant("b", mockUsage(100, 50, 0, 0)))
	context = BuildSessionContext([]Entry{u1, a1, compact1, u2, b})
	if len(context.Messages) != 5 {
		t.Fatalf("first kept first entry messages = %d, want 5", len(context.Messages))
	}

	builder = newPiCompactionEntryBuilder()
	entries = []Entry{
		builder.message(llm.UserMessageText("1")),
		builder.modelChange("openai", "gpt-4"),
		builder.message(piCompactionAssistant("a", mockUsage(100, 50, 0, 0))),
		builder.thinkingLevel("high"),
	}
	context = BuildSessionContext(entries)
	if context.ModelProvider != "anthropic" || context.ModelID != "claude-sonnet-4-5" || context.ThinkingLevel != "high" {
		t.Fatalf("model/thinking context = %#v", context)
	}
}

func TestPiCompactionPrepareRepeatedCompactions(t *testing.T) {
	builder := newPiCompactionEntryBuilder()
	u1 := builder.message(llm.UserMessageText("user msg 1 (summarized by compaction1)"))
	a1 := builder.message(piCompactionAssistant("assistant msg 1", mockUsage(100, 50, 0, 0)))
	u2 := builder.message(llm.UserMessageText("user msg 2 - kept by compaction1"))
	a2 := builder.message(piCompactionAssistant("assistant msg 2", mockUsage(100, 50, 0, 0)))
	u3 := builder.message(llm.UserMessageText("user msg 3 - kept by compaction1"))
	a3 := builder.message(piCompactionAssistant("assistant msg 3", mockUsage(5000, 1000, 0, 0)))
	compaction1 := builder.compaction("First summary", u2.ID)
	u4 := builder.message(llm.UserMessageText("user msg 4 (new after compaction1)"))
	a4 := builder.message(piCompactionAssistant("assistant msg 4", mockUsage(8000, 2000, 0, 0)))
	pathEntries := []Entry{u1, a1, u2, a2, u3, a3, compaction1, u4, a4}
	contextBefore := BuildSessionContext(pathEntries)
	preparation, err := PrepareCompaction(pathEntries, DefaultCompactionSettings)
	if err != nil {
		t.Fatal(err)
	}
	if preparation == nil || preparation.FirstKeptEntryID != u2.ID || preparation.PreviousSummary != "First summary" {
		t.Fatalf("preparation = %#v", preparation)
	}
	if strings.Contains(piCompactionMessagesText(preparation.MessagesToSummarize), "First summary") {
		t.Fatalf("messages to summarize should not include previous summary: %#v", preparation.MessagesToSummarize)
	}
	if preparation.TokensBefore != EstimateContextTokens(contextBefore.Messages).Tokens {
		t.Fatalf("tokens before = %d, want %d", preparation.TokensBefore, EstimateContextTokens(contextBefore.Messages).Tokens)
	}
	compaction2 := Entry{Type: "compaction", ID: "compaction2-id", ParentID: stringPtr(a4.ID), Timestamp: nowISO(), Summary: "Second summary", FirstKeptEntryID: preparation.FirstKeptEntryID, TokensBefore: preparation.TokensBefore}
	contextAfterText := piCompactionMessagesText(BuildSessionContext(append(pathEntries, compaction2)).Messages)
	if !strings.Contains(contextAfterText, "user msg 2 - kept by compaction1") || !strings.Contains(contextAfterText, "user msg 3 - kept by compaction1") {
		t.Fatalf("context after text = %q", contextAfterText)
	}

	builder = newPiCompactionEntryBuilder()
	u1 = builder.message(llm.UserMessageText(strings.Repeat("user msg 1 (summarized by compaction1)", 4)))
	a1 = builder.message(piCompactionAssistant(strings.Repeat("assistant msg 1", 4), mockUsage(100, 50, 0, 0)))
	u2 = builder.message(llm.UserMessageText(strings.Repeat("user msg 2 - kept by compaction1 ", 12)))
	a2 = builder.message(piCompactionAssistant(strings.Repeat("assistant msg 2 ", 12), mockUsage(100, 50, 0, 0)))
	u3 = builder.message(llm.UserMessageText(strings.Repeat("user msg 3 - kept by compaction1 ", 12)))
	a3 = builder.message(piCompactionAssistant(strings.Repeat("assistant msg 3 ", 12), mockUsage(5000, 1000, 0, 0)))
	compaction1 = builder.compaction("First summary", u2.ID)
	u4 = builder.message(llm.UserMessageText(strings.Repeat("user msg 4 (new after compaction1) ", 12)))
	a4 = builder.message(piCompactionAssistant(strings.Repeat("assistant msg 4 ", 12), mockUsage(8000, 2000, 0, 0)))
	preparation, err = PrepareCompaction([]Entry{u1, a1, u2, a2, u3, a3, compaction1, u4, a4}, CompactionSettings{Enabled: true, ReserveTokens: DefaultCompactionSettings.ReserveTokens, KeepRecentTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	if preparation == nil {
		t.Fatal("expected preparation")
	}
	summarizedText := piCompactionMessagesText(preparation.MessagesToSummarize)
	if !strings.Contains(summarizedText, "user msg 2 - kept by compaction1") || !strings.Contains(summarizedText, "user msg 3 - kept by compaction1") || strings.Contains(summarizedText, "First summary") || preparation.PreviousSummary != "First summary" {
		t.Fatalf("re-summary prep = %#v text=%q", preparation, summarizedText)
	}
}

func TestPiCompactionLargeSessionCases(t *testing.T) {
	entries := piCompactionLargeEntries(70)
	filePath := filepath.Join(t.TempDir(), "large-session.jsonl")
	storage, err := CreateJsonlSessionStorage(filePath, SessionMetadata{ID: "large-session", CreatedAt: nowISO(), CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if err := storage.AppendEntry(entry); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := OpenJsonlSessionStorage(filePath)
	if err != nil {
		t.Fatal(err)
	}
	loadedEntries := loaded.Entries()
	if len(loadedEntries) <= 100 {
		t.Fatalf("loaded entries = %d, want > 100", len(loadedEntries))
	}
	messageCount := 0
	for _, entry := range loadedEntries {
		if entry.Type == "message" {
			messageCount++
		}
	}
	if messageCount <= 100 {
		t.Fatalf("message count = %d, want > 100", messageCount)
	}

	cut := FindCutPoint(loadedEntries, 0, len(loadedEntries), DefaultCompactionSettings.KeepRecentTokens)
	if loadedEntries[cut.FirstKeptEntryIndex].Type != "message" {
		t.Fatalf("large cut entry = %#v, want message", loadedEntries[cut.FirstKeptEntryIndex])
	}
	role := loadedEntries[cut.FirstKeptEntryIndex].Message.Role
	if role != llm.RoleUser && role != llm.RoleAssistant {
		t.Fatalf("large cut role = %q, want user or assistant", role)
	}

	context := BuildSessionContext(loadedEntries)
	if len(context.Messages) <= 100 || context.ModelProvider == "" || context.ModelID == "" {
		t.Fatalf("large context = %#v", context)
	}
}

func TestPiCompactionLargeSessionWithFauxProvider(t *testing.T) {
	entries := piCompactionLargeEntries(70)
	model := compactionReasoningTestModel("compaction-large-faux", false, 8192)
	registerCompactionSummaryTestProvider(t, model)

	preparation, err := PrepareCompaction(entries, DefaultCompactionSettings)
	if err != nil {
		t.Fatal(err)
	}
	if preparation == nil {
		t.Fatal("expected preparation")
	}
	result, err := Compact(context.Background(), *preparation, model, "test-key", "off")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Summary) == 0 || result.FirstKeptEntryID == "" || result.TokensBefore <= 0 {
		t.Fatalf("compaction result = %#v", result)
	}

	loaded := BuildSessionContext(entries)
	parentID := entries[len(entries)-1].ID
	compactionEntry := Entry{Type: "compaction", ID: "compaction-test-id", ParentID: &parentID, Timestamp: nowISO(), Summary: result.Summary, FirstKeptEntryID: result.FirstKeptEntryID, TokensBefore: result.TokensBefore}
	reloaded := BuildSessionContext(append(entries, compactionEntry))
	if len(reloaded.Messages) >= len(loaded.Messages) || reloaded.Messages[0].Role != "compactionSummary" || !strings.Contains(piCompactionMessageText(reloaded.Messages[0]), result.Summary) {
		t.Fatalf("reloaded messages = %d original = %d first = %#v", len(reloaded.Messages), len(loaded.Messages), reloaded.Messages[0])
	}
}

type piCompactionEntryBuilder struct {
	counter int
	lastID  *string
}

func newPiCompactionEntryBuilder() *piCompactionEntryBuilder {
	return &piCompactionEntryBuilder{}
}

func (b *piCompactionEntryBuilder) message(message llm.Message) Entry {
	id := fmt.Sprintf("pi-id-%d", b.counter)
	b.counter++
	entry := messageEntry(id, cloneStringPtr(b.lastID), message)
	b.lastID = &entry.ID
	return entry
}

func (b *piCompactionEntryBuilder) compaction(summary, firstKept string) Entry {
	id := fmt.Sprintf("pi-id-%d", b.counter)
	b.counter++
	entry := compactionEntry(id, cloneStringPtr(b.lastID), summary, firstKept)
	b.lastID = &entry.ID
	return entry
}

func (b *piCompactionEntryBuilder) modelChange(provider, modelID string) Entry {
	id := fmt.Sprintf("pi-id-%d", b.counter)
	b.counter++
	entry := Entry{Type: "model_change", ID: id, ParentID: cloneStringPtr(b.lastID), Timestamp: nowISO(), Provider: provider, ModelID: modelID}
	b.lastID = &entry.ID
	return entry
}

func (b *piCompactionEntryBuilder) thinkingLevel(level string) Entry {
	id := fmt.Sprintf("pi-id-%d", b.counter)
	b.counter++
	entry := Entry{Type: "thinking_level_change", ID: id, ParentID: cloneStringPtr(b.lastID), Timestamp: nowISO(), ThinkingLevel: level}
	b.lastID = &entry.ID
	return entry
}

func piCompactionAssistant(text string, usage llm.Usage) llm.Message {
	message := harnessAssistantMessage(text)
	message.Usage = usage
	return message
}

func piCompactionMessageText(message llm.Message) string {
	parts := make([]string, 0, len(message.Content))
	for _, part := range message.Content {
		if part.Type == llm.ContentText {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, " ")
}

func piCompactionMessagesText(messages []llm.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		parts = append(parts, piCompactionMessageText(message))
	}
	return strings.Join(parts, "\n")
}

func piCompactionLargeEntries(turns int) []Entry {
	builder := newPiCompactionEntryBuilder()
	entries := make([]Entry, 0, turns*2)
	for i := 0; i < turns; i++ {
		entries = append(entries,
			builder.message(llm.UserMessageText(fmt.Sprintf("Large user message %03d", i))),
			builder.message(piCompactionAssistant(fmt.Sprintf("Large assistant message %03d", i), mockUsage(0, 100, (i+1)*750, 0))),
		)
	}
	return entries
}
