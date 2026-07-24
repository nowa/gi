package gicodingagent

import (
	"math"
	"reflect"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionStatsExposeCurrentContextUsage(t *testing.T) {
	session, sessionManager := createStatsTestSession(t)
	defer session.Dispose()
	model := session.Agent.State.Model

	sessionManager.AppendMessage(statsUserMessage("hello", 1))
	sessionManager.AppendMessage(statsAssistantMessage("hi", 200, 2, model))

	stats := session.GetSessionStats()
	contextUsage := session.GetContextUsage()
	if !reflect.DeepEqual(stats.ContextUsage, contextUsage) {
		t.Fatalf("context usage mismatch: stats=%#v direct=%#v", stats.ContextUsage, contextUsage)
	}
	if stats.ContextUsage == nil || stats.ContextUsage.Tokens == nil {
		t.Fatalf("context usage = %#v, want known tokens", stats.ContextUsage)
	}
	if *stats.ContextUsage.Tokens != 200 {
		t.Fatalf("context tokens = %d, want 200", *stats.ContextUsage.Tokens)
	}
	if stats.ContextUsage.ContextWindow != model.ContextWindow {
		t.Fatalf("context window = %d, want %d", stats.ContextUsage.ContextWindow, model.ContextWindow)
	}
	assertFloatNear(t, *stats.ContextUsage.Percent, (200/float64(model.ContextWindow))*100)
}

func TestAgentSessionStatsReportUnknownCurrentUsageAfterCompaction(t *testing.T) {
	session, sessionManager := createStatsTestSession(t)
	defer session.Dispose()

	sessionManager.AppendMessage(statsUserMessage("first", 1))
	sessionManager.AppendMessage(statsAssistantMessage("response1", 180_000, 2, session.Agent.State.Model))
	keptUserID := sessionManager.AppendMessage(statsUserMessage("second", 3))
	sessionManager.AppendMessage(statsAssistantMessage("response2", 195_000, 4, session.Agent.State.Model))
	sessionManager.AppendCompaction("summary", keptUserID, 195_000)
	sessionManager.AppendMessage(statsUserMessage("third", 5))

	stats := session.GetSessionStats()
	if stats.Tokens.Input != 195_000 {
		t.Fatalf("tokens input = %d, want 195000", stats.Tokens.Input)
	}
	if stats.ContextUsage == nil {
		t.Fatal("context usage should be defined after compaction")
	}
	if stats.ContextUsage.Tokens != nil || stats.ContextUsage.Percent != nil {
		t.Fatalf("context usage = %#v, want unknown tokens and percent", stats.ContextUsage)
	}
}

func TestAgentSessionStatsUsePostCompactionUsageForCurrentContext(t *testing.T) {
	session, sessionManager := createStatsTestSession(t)
	defer session.Dispose()
	model := session.Agent.State.Model

	sessionManager.AppendMessage(statsUserMessage("first", 1))
	sessionManager.AppendMessage(statsAssistantMessage("response1", 180_000, 2, model))
	keptUserID := sessionManager.AppendMessage(statsUserMessage("second", 3))
	sessionManager.AppendMessage(statsAssistantMessage("response2", 195_000, 4, model))
	sessionManager.AppendCompaction("summary", keptUserID, 195_000)
	sessionManager.AppendMessage(statsUserMessage("third", 5))
	sessionManager.AppendMessage(statsAssistantMessage("response3", 25_000, 6, model))

	stats := session.GetSessionStats()
	if stats.Tokens.Input != 220_000 {
		t.Fatalf("tokens input = %d, want 220000", stats.Tokens.Input)
	}
	if stats.ContextUsage == nil || stats.ContextUsage.Tokens == nil {
		t.Fatalf("context usage = %#v, want known post-compaction usage", stats.ContextUsage)
	}
	if *stats.ContextUsage.Tokens != 25_000 {
		t.Fatalf("context tokens = %d, want 25000", *stats.ContextUsage.Tokens)
	}
	assertFloatNear(t, *stats.ContextUsage.Percent, (25_000/float64(model.ContextWindow))*100)
}

func TestAgentSessionStatsIncludeGeneratedSummaryUsagePiStyle(t *testing.T) {
	session, sessionManager := createStatsTestSession(t)
	defer session.Dispose()
	usage := llm.Usage{
		Input:       20,
		Output:      5,
		TotalTokens: 25,
		Cost:        llm.UsageCost{Total: 0.25},
	}
	compactionID := sessionManager.AppendCompactionWithOptions(
		"summary",
		"",
		100,
		SessionSummaryOptions{Usage: &usage},
	)
	if _, err := sessionManager.BranchWithSummaryOptions(
		&compactionID,
		"branch summary",
		SessionSummaryOptions{Usage: &usage},
	); err != nil {
		t.Fatal(err)
	}

	stats := session.GetSessionStats()
	if stats.Tokens.Input != 140 ||
		stats.Tokens.Output != 10 ||
		stats.Tokens.TotalTokens != 150 {
		t.Fatalf("summary usage totals = %#v", stats.Tokens)
	}
}

func createStatsTestSession(t *testing.T) (*AgentSession, *SessionManager) {
	t.Helper()
	cwd := t.TempDir()
	sessionManager, err := InMemorySessionManager(cwd)
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            cwd,
		AgentDir:       t.TempDir(),
		Model:          llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
		SessionManager: sessionManager,
	})
	if err != nil {
		t.Fatal(err)
	}
	return session, sessionManager
}

func statsUserMessage(text string, timestamp int64) llm.Message {
	return llm.Message{
		Role:      llm.RoleUser,
		Content:   []llm.ContentPart{llm.Text(text)},
		Timestamp: timestamp,
	}
}

func statsAssistantMessage(text string, totalTokens int, timestamp int64, model llm.Model) llm.Message {
	return llm.Message{
		Role:       llm.RoleAssistant,
		Content:    []llm.ContentPart{llm.Text(text)},
		API:        model.API,
		Provider:   model.Provider,
		Model:      model.ID,
		Usage:      llm.Usage{Input: totalTokens, TotalTokens: totalTokens},
		StopReason: llm.StopReasonStop,
		Timestamp:  timestamp,
	}
}

func assertFloatNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.0000001 {
		t.Fatalf("float = %f, want %f", got, want)
	}
}
