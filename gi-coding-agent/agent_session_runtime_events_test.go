package gicodingagent

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionRuntimeEventsForNewAndResume(t *testing.T) {
	var events []ProtocolSessionEvent
	host := createRuntimeEventsHost(t, func(ctx *ProtocolExtensionContext) error {
		for _, eventType := range []string{ProtocolEventSessionBeforeSwitch, ProtocolEventSessionShutdown, ProtocolEventSessionSwitch, ProtocolEventSessionStart} {
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
		{Type: ProtocolEventSessionSwitch, Reason: "new", TargetSessionFile: secondSessionFile, PreviousSessionFile: originalSessionFile},
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
		{Type: ProtocolEventSessionSwitch, Reason: "resume", TargetSessionFile: originalSessionFile, PreviousSessionFile: secondSessionFile},
		{Type: ProtocolEventSessionStart, Reason: "resume", PreviousSessionFile: secondSessionFile},
	})
}

func TestAgentSessionRuntimeReloadEvents(t *testing.T) {
	var events []ProtocolSessionEvent
	host := createRuntimeEventsHost(t, func(ctx *ProtocolExtensionContext) error {
		for _, eventType := range []string{ProtocolEventSessionShutdown, ProtocolEventSessionSwitch, ProtocolEventSessionStart} {
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

	sessionFile := host.Session.SessionManager.GetSessionFile()
	if err := host.Reload(); err != nil {
		t.Fatal(err)
	}
	assertProtocolEvents(t, events, []ProtocolSessionEvent{
		{Type: ProtocolEventSessionShutdown, Reason: "reload", TargetSessionFile: sessionFile},
		{Type: ProtocolEventSessionStart, Reason: "reload", PreviousSessionFile: sessionFile},
	})
}

func TestAgentSessionRuntimeMessageEndCanReplaceAssistantMessage(t *testing.T) {
	host := createRuntimeEventsHost(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.On("message_end", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			if event.Message == nil || event.Message.Role != llm.RoleAssistant {
				return ProtocolEventResult{}, nil
			}
			replacement := *event.Message
			replacement.Usage.Cost.Total = 0.123
			return ProtocolEventResult{Message: &replacement, MessageSet: true}, nil
		})
	})
	var publicAssistantCost float64
	host.Session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "message_end" && event.Message != nil && event.Message.Role == llm.RoleAssistant {
			publicAssistantCost = event.Message.Usage.Cost.Total
		}
	})

	mustPrompt(t, host.Session, "hello")
	messages := host.Session.Messages()
	if len(messages) < 2 || messages[1].Usage.Cost.Total != 0.123 {
		t.Fatalf("messages = %#v", messages)
	}
	entries := host.Session.SessionManager.GetEntries()
	if len(entries) < 2 {
		t.Fatalf("entries = %#v", entries)
	}
	persisted, ok := sessionMessageToLLM(entries[1].Message)
	if !ok || persisted.Usage.Cost.Total != 0.123 {
		t.Fatalf("persisted assistant = %#v ok=%v", persisted, ok)
	}
	if publicAssistantCost != 0.123 {
		t.Fatalf("public assistant cost = %v", publicAssistantCost)
	}
}

func TestAgentSessionToolExecutionUpdateEventsPiParity(t *testing.T) {
	var extensionUpdates []string
	host := createRuntimeEventsHost(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.On(ProtocolEventToolExecutionUpdate, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			extensionUpdates = append(extensionUpdates, sdkContentPartsText(event.Content))
			return ProtocolEventResult{}, nil
		})
	})
	var publicUpdates []string
	host.Session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == ProtocolEventToolExecutionUpdate && event.PartialToolResult != nil {
			publicUpdates = append(publicUpdates, interactiveTextFromLLMMessage(*event.PartialToolResult))
		}
	})
	host.Session.Agent.State.Tools = append(host.Session.Agent.State.Tools, SDKTool{
		Name:          "stream_tool",
		PromptSnippet: "Stream partial tool results",
		ExecuteWithUpdates: func(_ string, _ map[string]any, onUpdate func(SDKToolResult)) (SDKToolResult, error) {
			onUpdate(SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "partial output"}}})
			return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "final output"}}}, nil
		},
	})
	var calls int
	host.Session.Responder = func(_ string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		calls++
		if calls == 1 {
			return llm.Message{
				Role:       llm.RoleAssistant,
				StopReason: "toolUse",
				Content:    []llm.ContentPart{llm.ToolCall("stream-call", "stream_tool", map[string]any{})},
			}, nil
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done")}}, nil
	}

	mustPrompt(t, host.Session, "run streaming tool")
	if !reflect.DeepEqual(extensionUpdates, []string{"partial output"}) {
		t.Fatalf("extension updates = %#v", extensionUpdates)
	}
	if !reflect.DeepEqual(publicUpdates, []string{"partial output"}) {
		t.Fatalf("public updates = %#v", publicUpdates)
	}
	if !sessionHasRole(host.Session, llm.RoleToolResult) {
		t.Fatalf("messages = %#v", host.Session.Messages())
	}
}

func TestAgentSessionRuntimeSwitchUsesDestinationCWDAndModelState(t *testing.T) {
	firstCWD := t.TempDir()
	secondCWD := t.TempDir()
	sessionDir := t.TempDir()
	firstManager, err := CreateSessionManager(firstCWD, sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	sourceSession, err := CreateAgentSession(AgentSessionOptions{
		CWD:            firstCWD,
		AgentDir:       filepath.Join(firstCWD, ConfigDirName, "agent"),
		Model:          llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
		SessionManager: firstManager,
	})
	if err != nil {
		t.Fatal(err)
	}
	host, err := NewAgentSessionRuntimeHost(sourceSession, NewProtocolExtensionRuntime(CapabilityLifecycleEvents))
	if err != nil {
		t.Fatal(err)
	}
	secondManager, err := CreateSessionManager(secondCWD, sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	secondModel := llm.MustGetModel("openai", "gpt-5-mini")
	secondManager.AppendModelChange(secondModel.Provider, secondModel.ID)
	secondManager.AppendThinkingLevelChange(string(ThinkingHigh))
	secondManager.AppendMessage(sessionMessageValue(llm.AssistantMessage([]llm.ContentPart{llm.Text("saved")}, llm.StopReasonStop, secondModel)))
	secondFile := secondManager.GetSessionFile()

	result, err := host.SwitchSession(secondFile)
	if err != nil {
		t.Fatal(err)
	}
	if result.Cancelled {
		t.Fatal("switch was cancelled")
	}
	if got := host.Session.SessionManager.GetCWD(); got != secondCWD {
		t.Fatalf("cwd = %q, want %q", got, secondCWD)
	}
	if got := host.Session.Agent.State.Model; got.Provider != secondModel.Provider || got.ID != secondModel.ID {
		t.Fatalf("model = %#v, want %#v", got, secondModel)
	}
	if got := host.Session.Agent.State.ThinkingLevel; got != string(ThinkingHigh) {
		t.Fatalf("thinking = %q, want high", got)
	}
}

func TestAgentSessionRuntimeNewSessionKeepsDefaultThinkingPiStyle(t *testing.T) {
	host := createRuntimeEventsHost(t, func(*ProtocolExtensionContext) error { return nil })
	if got := host.Session.Agent.State.ThinkingLevel; got != string(DefaultThinkingLevel) {
		t.Fatalf("startup thinking = %q, want %q", got, DefaultThinkingLevel)
	}
	if _, err := host.NewSession(); err != nil {
		t.Fatal(err)
	}
	if got := host.Session.Agent.State.ThinkingLevel; got != string(DefaultThinkingLevel) {
		t.Fatalf("/new thinking = %q, want %q", got, DefaultThinkingLevel)
	}
}

func TestAgentSessionRuntimeForkInvalidEntryReturnsError(t *testing.T) {
	host := createRuntimeEventsHost(t, func(*ProtocolExtensionContext) error { return nil })
	if _, err := host.Fork("missing-entry"); err == nil || !strings.Contains(err.Error(), "Invalid entry ID for forking") {
		t.Fatalf("fork error = %v", err)
	}
}

func TestAgentSessionRuntimeForkAtDuplicatesCurrentBranch(t *testing.T) {
	host := createRuntimeEventsHost(t, func(*ProtocolExtensionContext) error { return nil })
	mustPrompt(t, host.Session, "hello")
	mustPrompt(t, host.Session, "again")
	before := sessionMessageTexts(host.Session.Messages())
	leafID := host.Session.SessionManager.GetLeafID()
	if leafID == nil || *leafID == "" {
		t.Fatal("missing leaf id")
	}

	result, err := host.Fork(*leafID, AgentSessionRuntimeForkOptions{Position: "at"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Cancelled || result.SelectedText != "" {
		t.Fatalf("fork result = %#v", result)
	}
	if got := sessionMessageTexts(host.Session.Messages()); !reflect.DeepEqual(got, before) {
		t.Fatalf("messages = %#v, want %#v", got, before)
	}
}

func TestAgentSessionRuntimeForkAtDuplicatesCurrentBranchInMemory(t *testing.T) {
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
	})
	if err != nil {
		t.Fatal(err)
	}
	host, err := NewAgentSessionRuntimeHost(session, NewProtocolExtensionRuntime(CapabilityLifecycleEvents))
	if err != nil {
		t.Fatal(err)
	}
	mustPrompt(t, host.Session, "hello")
	mustPrompt(t, host.Session, "again")
	before := sessionMessageTexts(host.Session.Messages())
	leafID := host.Session.SessionManager.GetLeafID()
	if leafID == nil {
		t.Fatal("missing leaf id")
	}

	result, err := host.Fork(*leafID, AgentSessionRuntimeForkOptions{Position: "at"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Cancelled || host.Session.SessionManager.GetSessionFile() != "" {
		t.Fatalf("fork result = %#v file=%q", result, host.Session.SessionManager.GetSessionFile())
	}
	if got := sessionMessageTexts(host.Session.Messages()); !reflect.DeepEqual(got, before) {
		t.Fatalf("messages = %#v, want %#v", got, before)
	}
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
		for _, eventType := range []string{ProtocolEventSessionShutdown, ProtocolEventSessionSwitch, ProtocolEventSessionStart} {
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
		{Type: ProtocolEventSessionSwitch, Reason: "fork", TargetSessionFile: host.Session.SessionManager.GetSessionFile(), PreviousSessionFile: previousSessionFile},
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

func TestProtocolCommandContextNewSessionWithSessionPiRegression(t *testing.T) {
	var host *AgentSessionRuntimeHost
	var oldSessionFile string
	var replacementSessionFile string
	staleContextThrows := false
	host = createRuntimeCommandHost(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.RegisterCommand("repro", ProtocolCommandDefinition{
			Description: "repro",
			HandlerWithContext: func(_ string, commandCtx ProtocolCommandContext) error {
				oldCtx := commandCtx
				oldSessionFile = host.Session.SessionManager.GetSessionFile()
				_, err := commandCtx.NewSession(ProtocolNewSessionOptions{
					ParentSession: oldSessionFile,
					WithSession: func(replacedCtx ProtocolCommandContext) error {
						replacementSessionFile = host.Session.SessionManager.GetSessionFile()
						if _, err := oldCtx.NewSession(); err != nil {
							staleContextThrows = true
						}
						return replacedCtx.SendUserMessage("Hello from the new session!")
					},
				})
				return err
			},
		})
	}, "hello reply")

	if err := host.Session.Prompt("/repro"); err != nil {
		t.Fatal(err)
	}
	if replacementSessionFile == "" || replacementSessionFile == oldSessionFile {
		t.Fatalf("replacement=%q old=%q", replacementSessionFile, oldSessionFile)
	}
	if !staleContextThrows {
		t.Fatal("old command context should be stale after replacement")
	}
	if got := sessionMessageTexts(host.Session.Messages()); !reflect.DeepEqual(got, []string{
		"user:Hello from the new session!",
		"assistant:hello reply",
	}) {
		t.Fatalf("messages = %#v", got)
	}
}

func TestProtocolCommandContextForkWithSessionPiRegression(t *testing.T) {
	var host *AgentSessionRuntimeHost
	host = createRuntimeCommandHost(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.RegisterCommand("fork-it", ProtocolCommandDefinition{
			Description: "fork-it",
			HandlerWithContext: func(_ string, commandCtx ProtocolCommandContext) error {
				leafID := host.Session.SessionManager.GetLeafID()
				if leafID == nil {
					t.Fatal("missing leaf")
				}
				_, err := commandCtx.Fork(*leafID, ProtocolForkOptions{
					Position: "at",
					WithSession: func(replacedCtx ProtocolCommandContext) error {
						return replacedCtx.SendUserMessage("fork callback message")
					},
				})
				return err
			},
		})
	}, "seed reply", "fork reply")

	if err := host.Session.Prompt("seed"); err != nil {
		t.Fatal(err)
	}
	if err := host.Session.Prompt("/fork-it"); err != nil {
		t.Fatal(err)
	}
	if got := sessionMessageTexts(host.Session.Messages()); !reflect.DeepEqual(got, []string{
		"user:seed",
		"assistant:seed reply",
		"user:fork callback message",
		"assistant:Response to: fork callback message",
	}) {
		t.Fatalf("messages = %#v", got)
	}
}

func TestProtocolCommandContextSwitchSessionWithSessionPiRegression(t *testing.T) {
	var host *AgentSessionRuntimeHost
	targetSessionPath := ""
	host = createRuntimeCommandHost(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.RegisterCommand("switch-it", ProtocolCommandDefinition{
			Description: "switch-it",
			HandlerWithContext: func(_ string, commandCtx ProtocolCommandContext) error {
				_, err := commandCtx.SwitchSession(targetSessionPath, ProtocolSwitchSessionOptions{
					WithSession: func(replacedCtx ProtocolCommandContext) error {
						return replacedCtx.SendUserMessage("switch callback message")
					},
				})
				return err
			},
		})
	}, "root reply", "target reply", "switch reply")

	if err := host.Session.Prompt("root"); err != nil {
		t.Fatal(err)
	}
	originalSessionPath := host.Session.SessionManager.GetSessionFile()
	if _, err := host.NewSession(); err != nil {
		t.Fatal(err)
	}
	if err := host.Session.Prompt("target"); err != nil {
		t.Fatal(err)
	}
	targetSessionPath = host.Session.SessionManager.GetSessionFile()
	if _, err := host.SwitchSession(originalSessionPath); err != nil {
		t.Fatal(err)
	}
	if err := host.Session.Prompt("/switch-it"); err != nil {
		t.Fatal(err)
	}
	if host.Session.SessionManager.GetSessionFile() != targetSessionPath {
		t.Fatalf("session file = %q, want %q", host.Session.SessionManager.GetSessionFile(), targetSessionPath)
	}
	if got := sessionMessageTexts(host.Session.Messages()); !reflect.DeepEqual(got, []string{
		"user:target",
		"assistant:target reply",
		"user:switch callback message",
		"assistant:switch reply",
	}) {
		t.Fatalf("messages = %#v", got)
	}
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

func sdkContentPartsText(parts []SDKContentPart) string {
	var builder strings.Builder
	for _, part := range parts {
		if part.Type == "text" || part.Type == "" {
			builder.WriteString(part.Text)
		}
	}
	return builder.String()
}

func createRuntimeCommandHost(t *testing.T, factory func(*ProtocolExtensionContext) error, responses ...string) *AgentSessionRuntimeHost {
	t.Helper()
	cwd := t.TempDir()
	manager, err := CreateSessionManager(cwd, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	responseIndex := 0
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            cwd,
		AgentDir:       t.TempDir(),
		Model:          llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
		SessionManager: manager,
		Responder: func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
			text := "Response to: " + prompt
			if responseIndex < len(responses) {
				text = responses[responseIndex]
			}
			responseIndex++
			return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text(text)}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityCommandsRegister, CapabilityLifecycleEvents)
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{Path: "runtime-command", Factory: factory}}); err != nil {
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

func sessionMessageTexts(messages []llm.Message) []string {
	result := make([]string, 0, len(messages))
	for _, message := range messages {
		result = append(result, message.Role+":"+rpcMessageText(message))
	}
	return result
}
