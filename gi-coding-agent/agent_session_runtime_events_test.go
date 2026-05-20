package gicodingagent

import (
	"reflect"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionRuntimeEventsForNewAndResume(t *testing.T) {
	var events []ProtocolSessionEvent
	host := createRuntimeEventsHost(t, func(ctx *ProtocolExtensionContext) error {
		for _, eventType := range []string{ProtocolEventSessionBeforeSwitch, ProtocolEventSessionShutdown, ProtocolEventSessionStart} {
			if err := ctx.On(eventType, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
				events = append(events, event)
				return ProtocolEventResult{}, nil
			}); err != nil {
				return err
			}
		}
		return nil
	})

	assertProtocolEvents(t, events, []ProtocolSessionEvent{{Type: ProtocolEventSessionStart, Reason: "startup"}})
	events = nil

	mustPrompt(t, host.Session, "hello")
	originalSessionFile := host.Session.SessionManager.GetSessionFile()
	result, err := host.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if result.Cancelled {
		t.Fatalf("new session cancelled")
	}
	secondSessionFile := host.Session.SessionManager.GetSessionFile()
	assertProtocolEvents(t, events, []ProtocolSessionEvent{
		{Type: ProtocolEventSessionBeforeSwitch, Reason: "new"},
		{Type: ProtocolEventSessionShutdown, Reason: "new", TargetSessionFile: secondSessionFile},
		{Type: ProtocolEventSessionStart, Reason: "new", PreviousSessionFile: originalSessionFile},
	})

	events = nil
	resumeResult, err := host.SwitchSession(originalSessionFile)
	if err != nil {
		t.Fatal(err)
	}
	if resumeResult.Cancelled {
		t.Fatalf("resume cancelled")
	}
	assertProtocolEvents(t, events, []ProtocolSessionEvent{
		{Type: ProtocolEventSessionBeforeSwitch, Reason: "resume", TargetSessionFile: originalSessionFile},
		{Type: ProtocolEventSessionShutdown, Reason: "resume", TargetSessionFile: originalSessionFile},
		{Type: ProtocolEventSessionStart, Reason: "resume", PreviousSessionFile: secondSessionFile},
	})
}

func TestAgentSessionRuntimeBeforeSwitchCancellation(t *testing.T) {
	var events []ProtocolSessionEvent
	host := createRuntimeEventsHost(t, func(ctx *ProtocolExtensionContext) error {
		if err := ctx.On(ProtocolEventSessionBeforeSwitch, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			events = append(events, event)
			return ProtocolEventResult{Cancel: true}, nil
		}); err != nil {
			return err
		}
		return ctx.On(ProtocolEventSessionStart, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			events = append(events, event)
			return ProtocolEventResult{}, nil
		})
	})

	assertProtocolEvents(t, events, []ProtocolSessionEvent{{Type: ProtocolEventSessionStart, Reason: "startup"}})
	events = nil

	mustPrompt(t, host.Session, "hello")
	originalSessionFile := host.Session.SessionManager.GetSessionFile()
	result, err := host.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Cancelled {
		t.Fatalf("new session result = %#v, want cancelled", result)
	}
	if host.Session.SessionManager.GetSessionFile() != originalSessionFile {
		t.Fatalf("session file changed to %q, want %q", host.Session.SessionManager.GetSessionFile(), originalSessionFile)
	}
	assertProtocolEvents(t, events, []ProtocolSessionEvent{{Type: ProtocolEventSessionBeforeSwitch, Reason: "new"}})
}

func TestAgentSessionRuntimeInvalidatesAfterShutdownBeforeRebind(t *testing.T) {
	var phases []string
	host := createRuntimeEventsHost(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.On(ProtocolEventSessionShutdown, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			phases = append(phases, "session_shutdown")
			return ProtocolEventResult{}, nil
		})
	})
	oldSession := host.Session
	host.SetBeforeSessionInvalidate(func() {
		phases = append(phases, "beforeSessionInvalidate")
		if oldSession.SessionManager.GetCWD() == "" {
			t.Fatal("old session manager should still be readable before invalidation")
		}
	})
	host.SetRebindSession(func(session *AgentSession) error {
		phases = append(phases, "rebindSession")
		return nil
	})

	if _, err := host.NewSession(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(phases, []string{"session_shutdown", "beforeSessionInvalidate", "rebindSession"}) {
		t.Fatalf("phases = %#v", phases)
	}
}

func TestAgentSessionRuntimeForkEventsAndCancellation(t *testing.T) {
	var events []ProtocolSessionEvent
	cancelNextFork := false
	host := createRuntimeEventsHost(t, func(ctx *ProtocolExtensionContext) error {
		if err := ctx.On(ProtocolEventSessionBeforeFork, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			events = append(events, event)
			if cancelNextFork {
				cancelNextFork = false
				return ProtocolEventResult{Cancel: true}, nil
			}
			return ProtocolEventResult{}, nil
		}); err != nil {
			return err
		}
		for _, eventType := range []string{ProtocolEventSessionShutdown, ProtocolEventSessionStart} {
			if err := ctx.On(eventType, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
				events = append(events, event)
				return ProtocolEventResult{}, nil
			}); err != nil {
				return err
			}
		}
		return nil
	})

	assertProtocolEvents(t, events, []ProtocolSessionEvent{{Type: ProtocolEventSessionStart, Reason: "startup"}})
	events = nil

	mustPrompt(t, host.Session, "hello")
	userMessage := host.Session.GetUserMessagesForForking()[0]
	previousSessionFile := host.Session.SessionManager.GetSessionFile()

	successResult, err := host.Fork(userMessage.EntryID)
	if err != nil {
		t.Fatal(err)
	}
	if successResult.Cancelled || successResult.SelectedText != "hello" {
		t.Fatalf("fork result = %#v", successResult)
	}
	assertProtocolEvents(t, events, []ProtocolSessionEvent{
		{Type: ProtocolEventSessionBeforeFork, EntryID: userMessage.EntryID, Position: "before"},
		{Type: ProtocolEventSessionShutdown, Reason: "fork", TargetSessionFile: host.Session.SessionManager.GetSessionFile()},
		{Type: ProtocolEventSessionStart, Reason: "fork", PreviousSessionFile: previousSessionFile},
	})

	events = nil
	cancelNextFork = true
	cancelResult, err := host.Fork(userMessage.EntryID)
	if err != nil {
		t.Fatal(err)
	}
	if !cancelResult.Cancelled {
		t.Fatalf("fork cancel result = %#v", cancelResult)
	}
	assertProtocolEvents(t, events, []ProtocolSessionEvent{{Type: ProtocolEventSessionBeforeFork, EntryID: userMessage.EntryID, Position: "before"}})

	events = nil
	cancelNextFork = true
	cancelAtResult, err := host.Fork("missing-entry", AgentSessionRuntimeForkOptions{Position: "at"})
	if err != nil {
		t.Fatal(err)
	}
	if !cancelAtResult.Cancelled {
		t.Fatalf("fork at cancel result = %#v", cancelAtResult)
	}
	assertProtocolEvents(t, events, []ProtocolSessionEvent{{Type: ProtocolEventSessionBeforeFork, EntryID: "missing-entry", Position: "at"}})
}

func createRuntimeEventsHost(t *testing.T, factory func(*ProtocolExtensionContext) error) *AgentSessionRuntimeHost {
	t.Helper()
	cwd := t.TempDir()
	manager, err := CreateSessionManager(cwd, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            cwd,
		AgentDir:       t.TempDir(),
		Model:          llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
		SessionManager: manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{Path: "runtime-events", Factory: factory}}); err != nil {
		t.Fatal(err)
	}
	host, err := NewAgentSessionRuntimeHost(session, runtime)
	if err != nil {
		t.Fatal(err)
	}
	return host
}

func assertProtocolEvents(t *testing.T, got, want []ProtocolSessionEvent) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}
