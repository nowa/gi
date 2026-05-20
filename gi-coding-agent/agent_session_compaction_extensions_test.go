package gicodingagent

import (
	"errors"
	"reflect"
	"testing"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
)

func TestAgentSessionCompactionExtensionEvents(t *testing.T) {
	session := createCompactionExtensionSession(t, func(ctx *ProtocolExtensionContext) error {
		if err := ctx.On("session_before_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			if event.Preparation == nil || event.Preparation.TokensBefore < 0 {
				t.Fatalf("before compact preparation = %#v", event.Preparation)
			}
			if len(event.BranchEntries) == 0 {
				t.Fatal("before compact branch entries should be present")
			}
			return ProtocolEventResult{}, nil
		}); err != nil {
			return err
		}
		return ctx.On("session_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			if event.CompactionEntry == nil || event.CompactionEntry.Type != "compaction" || event.CompactionEntry.Summary == "" || event.FromExtension {
				t.Fatalf("compact event = %#v", event)
			}
			return ProtocolEventResult{}, nil
		})
	})

	mustPrompt(t, session, "What is 2+2?")
	mustPrompt(t, session, "What is 3+3?")
	if _, err := session.Compact(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentSessionCompactionExtensionCanCancel(t *testing.T) {
	session := createCompactionExtensionSession(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.On("session_before_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			return ProtocolEventResult{Cancel: true}, nil
		})
	})
	mustPrompt(t, session, "What is 2+2?")

	_, err := session.Compact()
	if err == nil || err.Error() != "Compaction cancelled" {
		t.Fatalf("compact error = %v", err)
	}
	if entries := filterFileEntriesByType(session.SessionManager.GetEntries(), "compaction"); len(entries) != 0 {
		t.Fatalf("compaction entries = %#v", entries)
	}
}

func TestAgentSessionCompactionExtensionCanProvideCustomResult(t *testing.T) {
	const customSummary = "Custom summary from extension"
	session := createCompactionExtensionSession(t, func(ctx *ProtocolExtensionContext) error {
		if err := ctx.On("session_before_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			return ProtocolEventResult{Compaction: &agentharness.CompactionResult{
				Summary:          customSummary,
				FirstKeptEntryID: event.Preparation.FirstKeptEntryID,
				TokensBefore:     event.Preparation.TokensBefore,
			}}, nil
		}); err != nil {
			return err
		}
		return ctx.On("session_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			if !event.FromExtension || event.CompactionEntry == nil || event.CompactionEntry.Summary != customSummary {
				t.Fatalf("compact event = %#v", event)
			}
			return ProtocolEventResult{}, nil
		})
	})
	mustPrompt(t, session, "What is 2+2?")
	mustPrompt(t, session, "What is 3+3?")

	result, err := session.Compact()
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != customSummary {
		t.Fatalf("summary = %q", result.Summary)
	}
}

func TestAgentSessionCompactionEventAfterEntrySaved(t *testing.T) {
	var session *AgentSession
	session = createCompactionExtensionSession(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.On("session_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			if entries := filterFileEntriesByType(session.SessionManager.GetEntries(), "compaction"); len(entries) != 1 {
				t.Fatalf("compaction entries at event time = %#v", entries)
			}
			return ProtocolEventResult{}, nil
		})
	})
	mustPrompt(t, session, "What is 2+2?")

	if _, err := session.Compact(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentSessionCompactionContinuesWhenBeforeHookErrors(t *testing.T) {
	var compactEvents int
	session := createCompactionExtensionSession(t, func(ctx *ProtocolExtensionContext) error {
		if err := ctx.On("session_before_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			return ProtocolEventResult{}, errors.New("extension intentionally throws")
		}); err != nil {
			return err
		}
		return ctx.On("session_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			compactEvents++
			if event.FromExtension {
				t.Fatalf("compact event = %#v", event)
			}
			return ProtocolEventResult{}, nil
		})
	})
	mustPrompt(t, session, "What is 2+2?")

	result, err := session.Compact()
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary == "" || compactEvents != 1 {
		t.Fatalf("result=%#v compactEvents=%d", result, compactEvents)
	}
}

func TestAgentSessionCompactionExtensionOrder(t *testing.T) {
	var order []string
	session := createCompactionExtensionSession(t, nil)
	runtime := session.ExtensionRuntime
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{
		{Path: "extension1", Factory: func(ctx *ProtocolExtensionContext) error {
			if err := ctx.On("session_before_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
				order = append(order, "extension1-before")
				return ProtocolEventResult{}, nil
			}); err != nil {
				return err
			}
			return ctx.On("session_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
				order = append(order, "extension1-after")
				return ProtocolEventResult{}, nil
			})
		}},
		{Path: "extension2", Factory: func(ctx *ProtocolExtensionContext) error {
			if err := ctx.On("session_before_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
				order = append(order, "extension2-before")
				return ProtocolEventResult{}, nil
			}); err != nil {
				return err
			}
			return ctx.On("session_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
				order = append(order, "extension2-after")
				return ProtocolEventResult{}, nil
			})
		}},
	}); err != nil {
		t.Fatal(err)
	}
	mustPrompt(t, session, "What is 2+2?")

	if _, err := session.Compact(); err != nil {
		t.Fatal(err)
	}
	want := []string{"extension1-before", "extension2-before", "extension1-after", "extension2-after"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
}

func TestAgentSessionCompactionBeforeEventData(t *testing.T) {
	var captured ProtocolSessionEvent
	session := createCompactionExtensionSession(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.On("session_before_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			captured = event
			return ProtocolEventResult{}, nil
		})
	})
	mustPrompt(t, session, "What is 2+2?")
	mustPrompt(t, session, "What is 3+3?")

	if _, err := session.Compact(); err != nil {
		t.Fatal(err)
	}
	if captured.Preparation == nil || captured.Preparation.FirstKeptEntryID == "" || captured.Preparation.TokensBefore < 0 {
		t.Fatalf("captured preparation = %#v", captured.Preparation)
	}
	if captured.Preparation.MessagesToSummarize == nil || captured.Preparation.TurnPrefixMessages == nil || len(captured.BranchEntries) == 0 {
		t.Fatalf("captured event = %#v", captured)
	}
}

func TestAgentSessionCompactionUsesExtensionValues(t *testing.T) {
	const customSummary = "Custom summary with modified values"
	session := createCompactionExtensionSession(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.On("session_before_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			return ProtocolEventResult{Compaction: &agentharness.CompactionResult{
				Summary:          customSummary,
				FirstKeptEntryID: event.Preparation.FirstKeptEntryID,
				TokensBefore:     999,
			}}, nil
		})
	})
	mustPrompt(t, session, "What is 2+2?")

	result, err := session.Compact()
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != customSummary || result.TokensBefore != 999 {
		t.Fatalf("result = %#v", result)
	}
}

func TestAgentSessionCompactionExtensionExampleContracts(t *testing.T) {
	session := createCompactionExtensionSession(t, func(ctx *ProtocolExtensionContext) error {
		if err := ctx.On("session_before_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			if event.Preparation == nil || event.Preparation.FirstKeptEntryID == "" || event.BranchEntries == nil {
				t.Fatalf("before compact event = %#v", event)
			}
			summary := "User requests:"
			return ProtocolEventResult{Compaction: &agentharness.CompactionResult{
				Summary:          summary,
				FirstKeptEntryID: event.Preparation.FirstKeptEntryID,
				TokensBefore:     event.Preparation.TokensBefore,
			}}, nil
		}); err != nil {
			return err
		}
		return ctx.On("session_compact", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			if event.CompactionEntry == nil || event.CompactionEntry.Type != "compaction" || event.CompactionEntry.Summary == "" || !event.FromExtension {
				t.Fatalf("compact event = %#v", event)
			}
			return ProtocolEventResult{}, nil
		})
	})
	mustPrompt(t, session, "What is 2+2?")

	if _, err := session.Compact(); err != nil {
		t.Fatal(err)
	}
}

func createCompactionExtensionSession(t *testing.T, factory func(*ProtocolExtensionContext) error) *AgentSession {
	t.Helper()
	session, _ := createCompactionTestSession(t, false)
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	if factory != nil {
		if err := runtime.LoadFactories([]ProtocolExtensionFactory{{Path: "compaction-extension", Factory: factory}}); err != nil {
			t.Fatal(err)
		}
	}
	runtime.BindSession(session)
	return session
}
