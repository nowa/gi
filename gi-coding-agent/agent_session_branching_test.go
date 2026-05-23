package gicodingagent

import (
	"errors"
	"os"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionForksFromSingleMessage(t *testing.T) {
	session, sessionManager := createBranchingTestSession(t, false)
	defer session.Dispose()

	userID := sessionManager.AppendMessage(statsUserMessage("Say hello", 1))
	sessionManager.AppendMessage(statsAssistantMessage("Hello!", 100, 2, session.Agent.State.Model))

	userMessages := session.GetUserMessagesForForking()
	if len(userMessages) != 1 {
		t.Fatalf("user messages = %d, want 1", len(userMessages))
	}
	if userMessages[0].EntryID != userID || userMessages[0].Text != "Say hello" {
		t.Fatalf("user message = %#v, want id %q and text Say hello", userMessages[0], userID)
	}

	result, err := session.Fork(userMessages[0].EntryID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Cancelled {
		t.Fatal("fork should not be cancelled")
	}
	if result.SelectedText != "Say hello" {
		t.Fatalf("selected text = %q, want Say hello", result.SelectedText)
	}
	if messages := result.Session.Messages(); len(messages) != 0 {
		t.Fatalf("forked messages = %d, want 0", len(messages))
	}
	sessionFile := result.Session.SessionManager.GetSessionFile()
	if sessionFile == "" {
		t.Fatal("forked persistent session file should be assigned")
	}
	if _, err := os.Stat(sessionFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("forked session file stat err = %v, want not exist", err)
	}
}

func TestSessionMessageValuePreservesContentSignatures(t *testing.T) {
	model := llm.Model{ID: "deepseek-v4-pro", Provider: "deepseek", API: "openai-completions"}
	text := llm.Text("visible")
	text.TextSignature = "text-sig"
	thinking := llm.Thinking("hidden")
	thinking.ThinkingSignature = "reasoning_content"
	thinking.Redacted = true
	toolCall := llm.ToolCall("call_1", "read", map[string]any{"path": "README.md"})
	toolCall.ThoughtSignature = `{"type":"reasoning.encrypted","id":"call_1","data":"opaque"}`
	source := llm.AssistantMessage([]llm.ContentPart{text, thinking, toolCall}, llm.StopReasonToolUse, model)

	roundTrip, ok := sessionMessageToLLM(sessionMessageValue(source))
	if !ok {
		t.Fatal("message did not round trip")
	}
	if len(roundTrip.Content) != 3 {
		t.Fatalf("content = %#v", roundTrip.Content)
	}
	if roundTrip.Content[0].TextSignature != "text-sig" {
		t.Fatalf("text = %#v", roundTrip.Content[0])
	}
	if roundTrip.Content[1].ThinkingSignature != "reasoning_content" || !roundTrip.Content[1].Redacted {
		t.Fatalf("thinking = %#v", roundTrip.Content[1])
	}
	if roundTrip.Content[2].ThoughtSignature != toolCall.ThoughtSignature {
		t.Fatalf("tool call = %#v", roundTrip.Content[2])
	}
}

func TestAgentSessionForksInMemoryNoSession(t *testing.T) {
	session, sessionManager := createBranchingTestSession(t, true)
	defer session.Dispose()
	if sessionManager.GetSessionFile() != "" {
		t.Fatalf("session file = %q, want empty for in-memory session", sessionManager.GetSessionFile())
	}

	sessionManager.AppendMessage(statsUserMessage("Say hi", 1))
	sessionManager.AppendMessage(statsAssistantMessage("Hi!", 100, 2, session.Agent.State.Model))

	userMessages := session.GetUserMessagesForForking()
	if len(userMessages) != 1 {
		t.Fatalf("user messages = %d, want 1", len(userMessages))
	}
	if len(session.Messages()) == 0 {
		t.Fatal("original session should have messages before fork")
	}

	result, err := session.Fork(userMessages[0].EntryID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Cancelled {
		t.Fatal("fork should not be cancelled")
	}
	if result.SelectedText != "Say hi" {
		t.Fatalf("selected text = %q, want Say hi", result.SelectedText)
	}
	if messages := result.Session.Messages(); len(messages) != 0 {
		t.Fatalf("forked messages = %d, want 0", len(messages))
	}
	if sessionFile := result.Session.SessionManager.GetSessionFile(); sessionFile != "" {
		t.Fatalf("forked session file = %q, want empty", sessionFile)
	}
}

func TestAgentSessionForksFromMiddleOfConversation(t *testing.T) {
	session, sessionManager := createBranchingTestSession(t, false)
	defer session.Dispose()

	sessionManager.AppendMessage(statsUserMessage("Say one", 1))
	sessionManager.AppendMessage(statsAssistantMessage("One", 100, 2, session.Agent.State.Model))
	sessionManager.AppendMessage(statsUserMessage("Say two", 3))
	sessionManager.AppendMessage(statsAssistantMessage("Two", 100, 4, session.Agent.State.Model))
	sessionManager.AppendMessage(statsUserMessage("Say three", 5))
	sessionManager.AppendMessage(statsAssistantMessage("Three", 100, 6, session.Agent.State.Model))

	userMessages := session.GetUserMessagesForForking()
	if len(userMessages) != 3 {
		t.Fatalf("user messages = %d, want 3", len(userMessages))
	}

	result, err := session.Fork(userMessages[1].EntryID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Cancelled {
		t.Fatal("fork should not be cancelled")
	}
	if result.SelectedText != "Say two" {
		t.Fatalf("selected text = %q, want Say two", result.SelectedText)
	}

	messages := result.Session.Messages()
	if len(messages) != 2 {
		t.Fatalf("forked messages = %d, want 2", len(messages))
	}
	if messages[0].Role != llm.RoleUser || messages[1].Role != llm.RoleAssistant {
		t.Fatalf("forked roles = %q, %q; want user, assistant", messages[0].Role, messages[1].Role)
	}
}

func TestProviderContextTransformsCodingAgentMessagesLikePi(t *testing.T) {
	messages := []llm.Message{
		{Role: "custom", Content: []llm.ContentPart{llm.Text("custom context")}},
		{Role: "branchSummary", Content: []llm.ContentPart{llm.Text("branch context")}},
		{Role: "compactionSummary", Content: []llm.ContentPart{llm.Text("compact context")}},
		{Role: "bashExecution", Content: []llm.ContentPart{llm.Text("bash context")}},
		{Role: "bashExecution", Content: []llm.ContentPart{llm.Text("hidden bash")}, Details: map[string]any{"excludeFromContext": true}},
	}

	got := providerContextFromSessionMessages(messages)
	if len(got) != 4 {
		t.Fatalf("messages = %#v", got)
	}
	for index, message := range got {
		if message.Role != llm.RoleUser {
			t.Fatalf("message %d role = %q, want user", index, message.Role)
		}
	}
	if text := sessionMessageText(got[1]); !strings.Contains(text, "summary of a branch") || !strings.Contains(text, "branch context") {
		t.Fatalf("branch summary text = %q", text)
	}
	if text := sessionMessageText(got[2]); !strings.Contains(text, "conversation history before this point") || !strings.Contains(text, "compact context") {
		t.Fatalf("compaction summary text = %q", text)
	}
}

func createBranchingTestSession(t *testing.T, noSession bool) (*AgentSession, *SessionManager) {
	t.Helper()
	cwd := t.TempDir()
	var (
		sessionManager *SessionManager
		err            error
	)
	if noSession {
		sessionManager, err = InMemorySessionManager(cwd)
	} else {
		sessionManager, err = CreateSessionManager(cwd, t.TempDir())
	}
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
