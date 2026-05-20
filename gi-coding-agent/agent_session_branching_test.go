package gicodingagent

import (
	"errors"
	"os"
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
