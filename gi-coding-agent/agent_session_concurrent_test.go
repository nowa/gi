package gicodingagent

import (
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionPromptRejectsWhileStreaming(t *testing.T) {
	session, started, release := createBlockingConcurrentSession(t)
	defer session.Dispose()
	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Prompt("First message")
	}()
	<-started
	if !session.IsStreaming() {
		t.Fatal("session should be streaming")
	}

	err := session.Prompt("Second message")
	if err == nil || !strings.Contains(err.Error(), "Agent is already processing") {
		t.Fatalf("second prompt err = %v", err)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestAgentSessionSteerWhileStreaming(t *testing.T) {
	session, started, release := createBlockingConcurrentSession(t)
	defer session.Dispose()
	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Prompt("First message")
	}()
	<-started

	if err := session.Steer("Steering message"); err != nil {
		t.Fatal(err)
	}
	if session.PendingMessageCount() != 1 {
		t.Fatalf("pending count = %d, want 1", session.PendingMessageCount())
	}
	if got := session.GetSteeringMessages(); len(got) != 1 || got[0] != "Steering message" {
		t.Fatalf("steering messages = %#v", got)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestAgentSessionFollowUpWhileStreaming(t *testing.T) {
	session, started, release := createBlockingConcurrentSession(t)
	defer session.Dispose()
	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Prompt("First message")
	}()
	<-started

	if err := session.FollowUp("Follow-up message"); err != nil {
		t.Fatal(err)
	}
	if session.PendingMessageCount() != 1 {
		t.Fatalf("pending count = %d, want 1", session.PendingMessageCount())
	}
	if got := session.GetFollowUpMessages(); len(got) != 1 || got[0] != "Follow-up message" {
		t.Fatalf("follow-up messages = %#v", got)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestAgentSessionPromptAfterPreviousCompletes(t *testing.T) {
	callCount := 0
	session := createConcurrentSessionWithResponder(t, func(_ string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		callCount++
		return retryAssistantText("Done"), nil
	})
	defer session.Dispose()

	if err := session.Prompt("First message"); err != nil {
		t.Fatal(err)
	}
	if session.IsStreaming() {
		t.Fatal("session should not be streaming after prompt")
	}
	if err := session.Prompt("Second message"); err != nil {
		t.Fatal(err)
	}
	if callCount != 2 {
		t.Fatalf("call count = %d, want 2", callCount)
	}
}

func createBlockingConcurrentSession(t *testing.T) (*AgentSession, chan struct{}, chan struct{}) {
	t.Helper()
	started := make(chan struct{})
	release := make(chan struct{})
	closed := false
	session := createConcurrentSessionWithResponder(t, func(_ string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		if !closed {
			close(started)
			closed = true
		}
		<-release
		return retryAssistantText("Done"), nil
	})
	return session, started, release
}

func createConcurrentSessionWithResponder(t *testing.T, responder AgentSessionResponder) *AgentSession {
	t.Helper()
	cwd := t.TempDir()
	manager, err := InMemorySessionManager(cwd)
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            cwd,
		AgentDir:       t.TempDir(),
		Model:          llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
		SessionManager: manager,
		Responder:      responder,
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}
