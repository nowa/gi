package gicodingagent

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionWaitForIdleTracksPromptSettlement(t *testing.T) {
	session, started, release := createBlockingConcurrentSession(t)
	defer session.Dispose()
	if !session.IsIdle() {
		t.Fatal("new session should be idle")
	}
	var extensionSawIdle bool
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{
		Path: "agent-settled.gi.json",
		Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.On(ProtocolEventAgentSettled, func(ProtocolSessionEvent) (ProtocolEventResult, error) {
				extensionSawIdle = ctx.IsIdle()
				return ProtocolEventResult{}, nil
			})
		},
	})
	runtime.BindSession(session)
	settled := make(chan struct{})
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == ProtocolEventAgentSettled {
			close(settled)
		}
	})

	promptErr := make(chan error, 1)
	go func() {
		promptErr <- session.Prompt("First message")
	}()
	<-started
	if session.IsIdle() {
		t.Fatal("session should not be idle while prompt is active")
	}

	waitErr := make(chan error, 1)
	go func() {
		waitErr <- session.WaitForIdle(context.Background())
	}()
	select {
	case err := <-waitErr:
		t.Fatalf("WaitForIdle returned before settlement: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	if err := <-promptErr; err != nil {
		t.Fatal(err)
	}
	if err := <-waitErr; err != nil {
		t.Fatal(err)
	}
	select {
	case <-settled:
	default:
		t.Fatal("WaitForIdle returned before agent_settled was dispatched")
	}
	if !session.IsIdle() {
		t.Fatal("session should be idle after settlement")
	}
	if !extensionSawIdle {
		t.Fatal("agent_settled extension handler should observe an idle session")
	}
}

func TestAgentSessionWaitForIdleHonorsContext(t *testing.T) {
	session, started, release := createBlockingConcurrentSession(t)
	defer session.Dispose()
	promptErr := make(chan error, 1)
	go func() {
		promptErr <- session.Prompt("First message")
	}()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := session.WaitForIdle(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitForIdle error = %v, want context.Canceled", err)
	}
	if session.IsIdle() {
		t.Fatal("canceling a waiter must not cancel the active prompt")
	}

	close(release)
	if err := <-promptErr; err != nil {
		t.Fatal(err)
	}
}

func TestProtocolCommandContextWaitForIdleUsesActiveSession(t *testing.T) {
	session, started, release := createBlockingConcurrentSession(t)
	defer session.Dispose()
	runtime := NewProtocolExtensionRuntime()
	if _, err := NewAgentSessionRuntimeHost(session, runtime); err != nil {
		t.Fatal(err)
	}
	commandContext := runtime.CreateCommandContext()

	promptErr := make(chan error, 1)
	go func() {
		promptErr <- session.Prompt("First message")
	}()
	<-started
	waitErr := make(chan error, 1)
	go func() {
		waitErr <- commandContext.WaitForIdle()
	}()
	select {
	case err := <-waitErr:
		t.Fatalf("command context returned before settlement: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	if err := <-promptErr; err != nil {
		t.Fatal(err)
	}
	if err := <-waitErr; err != nil {
		t.Fatal(err)
	}
}

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

func TestAgentSessionConcurrentPromptsHaveSingleOwner(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	session := createConcurrentSessionWithResponder(t, func(_ string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return retryAssistantText("Done"), nil
	})
	defer session.Dispose()

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, prompt := range []string{"First message", "Second message"} {
		go func() {
			<-start
			results <- session.Prompt(prompt)
		}()
	}
	close(start)
	<-started

	var successes, busyErrors int
	select {
	case err := <-results:
		if err == nil || !strings.Contains(err.Error(), "Agent is already processing") {
			close(release)
			t.Fatalf("concurrent prompt error = %v, want busy error", err)
		}
		busyErrors++
	case <-time.After(time.Second):
		close(release)
		t.Fatal("concurrent prompt did not reject while the first prompt owned the run")
	}
	close(release)
	err := <-results
	switch {
	case err == nil:
		successes++
	case strings.Contains(err.Error(), "Agent is already processing"):
		busyErrors++
	default:
		t.Fatalf("unexpected prompt error: %v", err)
	}
	if successes != 1 || busyErrors != 1 || calls.Load() != 1 {
		t.Fatalf("successes=%d busyErrors=%d responderCalls=%d, want 1/1/1", successes, busyErrors, calls.Load())
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
