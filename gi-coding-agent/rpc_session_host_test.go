package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestRPCSessionHostGetState(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)

	state := mustRPCHandleData[RPCSessionState](t, host, RPCCommand{Type: RPCCommandGetState})
	if state.Model == nil || state.Model.Provider != "anthropic" || state.Model.ID != "claude-sonnet-4-5" {
		t.Fatalf("state model = %#v", state.Model)
	}
	if state.IsStreaming || state.MessageCount != 0 || state.PendingMessageCount != 0 {
		t.Fatalf("state counters = streaming:%v messages:%d pending:%d", state.IsStreaming, state.MessageCount, state.PendingMessageCount)
	}
	if state.SessionID == "" || state.SessionFile == "" {
		t.Fatalf("state session identifiers = %#v", state)
	}
}

func TestRPCSessionHostPromptPersistsMessagesAndEvents(t *testing.T) {
	host, session, manager := createRPCSessionHostForTest(t)
	var events []AgentSessionEvent
	session.Subscribe(func(event AgentSessionEvent) {
		events = append(events, event)
	})

	response := host.HandleCommand(context.Background(), RPCCommand{Type: RPCCommandPrompt, Message: "Reply with hello"})
	if !response.Success {
		t.Fatalf("prompt response = %#v", response)
	}

	messageEndCount := 0
	for _, event := range events {
		if event.Type == "message_end" {
			messageEndCount++
		}
	}
	if messageEndCount < 2 {
		t.Fatalf("message_end events = %d, want user and assistant", messageEndCount)
	}
	entries := LoadEntriesFromFile(manager.GetSessionFile())
	if len(entries) == 0 || entries[0].Type != "session" {
		t.Fatalf("session entries = %#v", entries)
	}
	roles := rolesFromEntries(entries)
	if !containsString(roles, llm.RoleUser) || !containsString(roles, llm.RoleAssistant) {
		t.Fatalf("persisted roles = %#v", roles)
	}
}

func TestRPCSessionHostPromptPersistsImages(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	image := llm.ContentPart{Data: "base64-image", MIMEType: "image/png"}
	response := host.HandleCommand(context.Background(), RPCCommand{Type: RPCCommandPrompt, Message: "Describe", Images: []llm.ContentPart{image}})
	if !response.Success {
		t.Fatalf("prompt response = %#v", response)
	}
	messages := host.Session.Messages()
	if len(messages) < 1 || messages[0].Role != llm.RoleUser || len(messages[0].Content) != 2 {
		t.Fatalf("messages = %#v", messages)
	}
	if messages[0].Content[1].Type != llm.ContentImage || messages[0].Content[1].Data != "base64-image" || messages[0].Content[1].MIMEType != "image/png" {
		t.Fatalf("image content = %#v", messages[0].Content[1])
	}
}

func TestRPCSessionHostManualCompactionPersistsEntry(t *testing.T) {
	host, _, manager := createRPCSessionHostForTest(t)
	mustRPCPrompt(t, host, "Say hello")
	mustRPCPrompt(t, host, "Say goodbye")

	result := mustRPCHandleData[agentharness.CompactionResult](t, host, RPCCommand{Type: RPCCommandCompact})
	if result.Summary == "" || result.TokensBefore <= 0 {
		t.Fatalf("compaction result = %#v", result)
	}
	entries := LoadEntriesFromFile(manager.GetSessionFile())
	if countRPCEntryType(entries, "compaction") != 1 {
		t.Fatalf("compaction entries = %d in %#v", countRPCEntryType(entries, "compaction"), entries)
	}
}

func TestRPCSessionHostBashExecutesAndAddsContext(t *testing.T) {
	host, _, manager := createRPCSessionHostForTest(t)

	result := mustRPCHandleData[BashResult](t, host, RPCCommand{Type: RPCCommandBash, Command: "echo hello"})
	if strings.TrimSpace(result.Output) != "hello" || result.ExitCode != 0 || result.Cancelled {
		t.Fatalf("bash result = %#v", result)
	}

	entries := manager.GetEntries()
	var found bool
	for _, entry := range entries {
		message, ok := entry.Message.(map[string]any)
		if !ok || messageRole(message) != "bashExecution" {
			continue
		}
		output, _ := message["output"].(string)
		found = strings.Contains(output, "hello")
	}
	if !found {
		t.Fatalf("missing bashExecution message in %#v", entries)
	}
}

func TestRPCSessionHostPromptIncludesBashOutputInContext(t *testing.T) {
	unique := "unique-rpc-context"
	host, _, _ := createRPCSessionHostForTest(t, func(options *AgentSessionOptions) {
		options.Responder = func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error) {
			for _, message := range context {
				if message.Role == llm.RoleUser && strings.Contains(rpcMessageText(message), unique) {
					return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text(unique)}}, nil
				}
			}
			return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("missing")}}, nil
		}
	})

	mustRPCHandleData[BashResult](t, host, RPCCommand{Type: RPCCommandBash, Command: "echo " + unique})
	mustRPCPrompt(t, host, "What was the exact output?")

	text := host.GetLastAssistantText()
	if text == nil || !strings.Contains(*text, unique) {
		t.Fatalf("last assistant text = %#v", text)
	}
}

func TestRPCSessionHostThinkingLevelCommands(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)

	response := host.HandleCommand(context.Background(), RPCCommand{Type: RPCCommandSetThinkingLevel, Level: "high"})
	if !response.Success {
		t.Fatalf("set thinking response = %#v", response)
	}
	state := mustRPCHandleData[RPCSessionState](t, host, RPCCommand{Type: RPCCommandGetState})
	if state.ThinkingLevel != "high" {
		t.Fatalf("thinking level = %q, want high", state.ThinkingLevel)
	}

	cycled := mustRPCHandleData[map[string]string](t, host, RPCCommand{Type: RPCCommandCycleThinkingLevel})
	if cycled["level"] == "" || cycled["level"] == "high" {
		t.Fatalf("cycled level = %#v", cycled)
	}
	state = mustRPCHandleData[RPCSessionState](t, host, RPCCommand{Type: RPCCommandGetState})
	if state.ThinkingLevel != cycled["level"] {
		t.Fatalf("state thinking = %q, want %q", state.ThinkingLevel, cycled["level"])
	}
}

func TestRPCSessionHostGetAvailableModels(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	host.AvailableModels = []llm.Model{llm.MustGetModel("anthropic", "claude-sonnet-4-5")}

	result := mustRPCHandleData[RPCAvailableModelsResult](t, host, RPCCommand{Type: RPCCommandGetAvailableModels})
	if len(result.Models) != 1 {
		t.Fatalf("models = %#v", result.Models)
	}
	model := result.Models[0]
	if model.Provider == "" || model.ID == "" || model.ContextWindow <= 0 {
		t.Fatalf("model fields = %#v", model)
	}
}

func TestRPCSessionHostModelAndQueueModeCommands(t *testing.T) {
	host, _, manager := createRPCSessionHostForTest(t)
	current := llm.MustGetModel("anthropic", "claude-sonnet-4-5")
	next := llm.MustGetModel("openai", "gpt-4o-mini")
	host.AvailableModels = []llm.Model{current, next}

	model := mustRPCHandleData[llm.Model](t, host, RPCCommand{Type: RPCCommandSetModel, Provider: next.Provider, ModelID: next.ID})
	if model.Provider != next.Provider || model.ID != next.ID {
		t.Fatalf("set model result = %#v", model)
	}
	state := mustRPCHandleData[RPCSessionState](t, host, RPCCommand{Type: RPCCommandGetState})
	if state.Model == nil || state.Model.Provider != next.Provider || state.Model.ID != next.ID {
		t.Fatalf("state model = %#v", state.Model)
	}
	if countRPCEntryType(manager.GetEntries(), "model_change") != 1 {
		t.Fatalf("model change entries = %#v", manager.GetEntries())
	}

	cycled := mustRPCHandleData[RPCCycleModelResult](t, host, RPCCommand{Type: RPCCommandCycleModel})
	if cycled.Model.Provider != current.Provider || cycled.Model.ID != current.ID || cycled.IsScoped {
		t.Fatalf("cycle model result = %#v", cycled)
	}

	if response := host.HandleCommand(context.Background(), RPCCommand{Type: RPCCommandSetSteeringMode, Mode: "one-at-a-time"}); !response.Success {
		t.Fatalf("set steering response = %#v", response)
	}
	if response := host.HandleCommand(context.Background(), RPCCommand{Type: RPCCommandSetFollowUpMode, Mode: "one-at-a-time"}); !response.Success {
		t.Fatalf("set follow-up response = %#v", response)
	}
	state = mustRPCHandleData[RPCSessionState](t, host, RPCCommand{Type: RPCCommandGetState})
	if state.SteeringMode != "one-at-a-time" || state.FollowUpMode != "one-at-a-time" {
		t.Fatalf("queue modes = %#v", state)
	}
}

func TestRPCSessionHostUsesSessionScopedModels(t *testing.T) {
	current := llm.MustGetModel("anthropic", "claude-sonnet-4-5")
	next := llm.MustGetModel("openai", "gpt-4o-mini")
	host, session, _ := createRPCSessionHostForTest(t, func(options *AgentSessionOptions) {
		options.ScopedModels = []ScopedModel{
			{Model: current, ThinkingLevel: ThinkingHigh},
			{Model: next, ThinkingLevel: ThinkingOff},
		}
	})

	if len(host.ScopedModels) != 2 || len(session.ScopedModels) != 2 {
		t.Fatalf("scoped models host=%#v session=%#v", host.ScopedModels, session.ScopedModels)
	}

	cycled := mustRPCHandleData[RPCCycleModelResult](t, host, RPCCommand{Type: RPCCommandCycleModel})
	if cycled.Model.Provider != next.Provider || cycled.Model.ID != next.ID || !cycled.IsScoped || cycled.ThinkingLevel != string(ThinkingOff) {
		t.Fatalf("first scoped cycle = %#v", cycled)
	}

	cycled = mustRPCHandleData[RPCCycleModelResult](t, host, RPCCommand{Type: RPCCommandCycleModel})
	if cycled.Model.Provider != current.Provider || cycled.Model.ID != current.ID || !cycled.IsScoped || cycled.ThinkingLevel != string(ThinkingHigh) {
		t.Fatalf("second scoped cycle = %#v", cycled)
	}
}

func TestRPCSessionHostModelCommandsClampThinkingAndEmitExtensionEvents(t *testing.T) {
	host, session, manager := createRPCSessionHostForTest(t)
	current := llm.MustGetModel("anthropic", "claude-sonnet-4-5")
	next := llm.MustGetModel("openai", "gpt-4o-mini")
	host.AvailableModels = []llm.Model{current, next}

	var events []ProtocolSessionEvent
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{
		Path: "model-events.gi.json",
		Factory: func(ctx *ProtocolExtensionContext) error {
			if err := ctx.On(ProtocolEventModelSelect, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
				events = append(events, event)
				return ProtocolEventResult{}, nil
			}); err != nil {
				return err
			}
			return ctx.On(ProtocolEventThinkingLevelSelect, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
				events = append(events, event)
				return ProtocolEventResult{}, nil
			})
		},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAgentSessionRuntimeHost(session, runtime); err != nil {
		t.Fatal(err)
	}

	if err := host.SetThinkingLevel("high"); err != nil {
		t.Fatal(err)
	}
	if _, err := host.SetModel(next.Provider, next.ID); err != nil {
		t.Fatal(err)
	}

	state := host.GetState()
	if state.ThinkingLevel != "off" {
		t.Fatalf("thinking level after non-reasoning model = %q, want off", state.ThinkingLevel)
	}
	if countRPCEntryType(manager.GetEntries(), "thinking_level_change") != 2 {
		t.Fatalf("entries = %#v", manager.GetEntries())
	}
	var sawModelSelect, sawClampedThinking bool
	for _, event := range events {
		if event.Type == ProtocolEventModelSelect &&
			event.Model != nil && event.Model.Provider == next.Provider && event.Model.ID == next.ID &&
			event.PreviousModel != nil && event.PreviousModel.Provider == current.Provider &&
			event.SelectSource == "set" {
			sawModelSelect = true
		}
		if event.Type == ProtocolEventThinkingLevelSelect && event.PreviousLevel == "high" && event.ThinkingLevel == "off" {
			sawClampedThinking = true
		}
	}
	if !sawModelSelect || !sawClampedThinking {
		t.Fatalf("events = %#v", events)
	}
}

func TestRPCSessionHostSetModelRunsPreflightBeforePersisting(t *testing.T) {
	host, _, manager := createRPCSessionHostForTest(t, func(options *AgentSessionOptions) {
		options.Preflight = func(model llm.Model) error {
			if model.Provider == "openai" {
				return errors.New("No API key for openai/gpt-4o-mini")
			}
			return nil
		}
	})
	next := llm.MustGetModel("openai", "gpt-4o-mini")
	host.AvailableModels = []llm.Model{llm.MustGetModel("anthropic", "claude-sonnet-4-5"), next}

	_, err := host.SetModel(next.Provider, next.ID)
	if err == nil || !strings.Contains(err.Error(), "No API key for openai/gpt-4o-mini") {
		t.Fatalf("SetModel error = %v", err)
	}
	state := host.GetState()
	if state.Model == nil || state.Model.Provider != "anthropic" {
		t.Fatalf("state model = %#v", state.Model)
	}
	if countRPCEntryType(manager.GetEntries(), "model_change") != 0 {
		t.Fatalf("entries = %#v", manager.GetEntries())
	}
}

func TestRPCSessionHostPromptingAndSessionCommandSurface(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	mustRPCPrompt(t, host, "First")
	mustRPCPrompt(t, host, "Second")

	messages := mustRPCHandleData[RPCMessagesResult](t, host, RPCCommand{Type: RPCCommandGetMessages})
	if len(messages.Messages) != 4 {
		t.Fatalf("messages = %#v", messages.Messages)
	}
	forkMessages := mustRPCHandleData[RPCForkMessagesResult](t, host, RPCCommand{Type: RPCCommandGetForkMessages})
	if len(forkMessages.Messages) != 2 || forkMessages.Messages[0].Text != "First" {
		t.Fatalf("fork messages = %#v", forkMessages.Messages)
	}

	if response := host.HandleCommand(context.Background(), RPCCommand{Type: RPCCommandSteer, Message: "steer"}); !response.Success {
		t.Fatalf("steer response = %#v", response)
	}
	if response := host.HandleCommand(context.Background(), RPCCommand{Type: RPCCommandFollowUp, Message: "follow"}); !response.Success {
		t.Fatalf("follow-up response = %#v", response)
	}
	state := mustRPCHandleData[RPCSessionState](t, host, RPCCommand{Type: RPCCommandGetState})
	if state.PendingMessageCount != 2 {
		t.Fatalf("pending count = %d", state.PendingMessageCount)
	}

	forked := mustRPCHandleData[RPCForkResult](t, host, RPCCommand{Type: RPCCommandFork, EntryID: forkMessages.Messages[0].EntryID})
	if forked.Cancelled || forked.Text != "First" {
		t.Fatalf("fork result = %#v", forked)
	}
	afterFork := mustRPCHandleData[RPCSessionState](t, host, RPCCommand{Type: RPCCommandGetState})
	if afterFork.SessionID == state.SessionID || afterFork.MessageCount != 0 {
		t.Fatalf("after fork state = %#v before = %#v", afterFork, state)
	}
}

func TestRPCSessionHostGetCommandsIncludesPromptTemplates(t *testing.T) {
	host, _, manager := createRPCSessionHostForTest(t)
	promptsDir := filepath.Join(manager.GetCWD(), ConfigDirName, "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(promptsDir, "review.md"), []byte("---\ndescription: Review changes\nargument-hint: <path>\n---\nReview $1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := mustRPCHandleData[RPCCommandsResult](t, host, RPCCommand{Type: RPCCommandGetCommands})
	foundBuiltin := false
	foundPrompt := false
	for _, command := range result.Commands {
		if command.Name == "settings" && command.Source == "builtin" {
			foundBuiltin = true
		}
		if command.Name == "review" && command.Source == "prompt" && command.Description == "Review changes" && command.ArgumentHint == "<path>" {
			foundPrompt = true
		}
	}
	if !foundBuiltin || !foundPrompt {
		t.Fatalf("commands = %#v", result.Commands)
	}
}

func TestRPCSessionHostGetCommandsIncludesExtensionArgumentHints(t *testing.T) {
	host, session, _ := createRPCSessionHostForTest(t)
	session.ExtensionRuntime = NewProtocolExtensionRuntime(CapabilityCommandsRegister)
	session.ExtensionRuntime.BindSession(session)
	ctx := &ProtocolExtensionContext{
		runtime: session.ExtensionRuntime,
		source:  ProtocolSourceInfo{Path: "deploy.gi.json", Source: "local", Scope: "project"},
	}
	if err := ctx.RegisterCommand("deploy", ProtocolCommandDefinition{
		Description:  "Deploy target",
		ArgumentHint: "<env>",
	}); err != nil {
		t.Fatal(err)
	}

	result := host.GetCommands()
	for _, command := range result.Commands {
		if command.Name == "deploy" {
			if command.Source != "extension" || command.Description != "Deploy target" || command.ArgumentHint != "<env>" {
				t.Fatalf("extension command = %#v", command)
			}
			return
		}
	}
	t.Fatalf("deploy command missing: %#v", result.Commands)
}

func TestRPCSessionHostGetCommandsHonorsSkillCommandSettingPiStyle(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t, func(options *AgentSessionOptions) {
		skillDir := filepath.Join(options.CWD, ConfigDirName, "skills", "demo")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := "---\nname: demo\ndescription: Demo skill\n---\nUse demo.\n"
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	})

	result := host.GetCommands()
	if !rpcCommandNamesContain(result.Commands, "skill:demo") {
		t.Fatalf("skill command missing when setting defaults enabled: %#v", result.Commands)
	}

	host.Settings = NewInMemorySettingsManager(map[string]any{"enableSkillCommands": false})
	result = host.GetCommands()
	if rpcCommandNamesContain(result.Commands, "skill:demo") {
		t.Fatalf("skill command should be hidden when disabled: %#v", result.Commands)
	}
}

func rpcCommandNamesContain(commands []RPCSlashCommand, name string) bool {
	for _, command := range commands {
		if command.Name == name {
			return true
		}
	}
	return false
}

func TestRPCSessionHostCloneForksCurrentLeafLikePi(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	mustRPCPrompt(t, host, "First")
	mustRPCPrompt(t, host, "Second")
	before := mustRPCHandleData[RPCSessionState](t, host, RPCCommand{Type: RPCCommandGetState})

	result := mustRPCHandleData[RPCCloneResult](t, host, RPCCommand{Type: RPCCommandClone})
	if result.Cancelled {
		t.Fatalf("clone result = %#v", result)
	}
	after := mustRPCHandleData[RPCSessionState](t, host, RPCCommand{Type: RPCCommandGetState})
	if after.SessionID == before.SessionID || after.MessageCount != before.MessageCount {
		t.Fatalf("after clone = %#v before = %#v", after, before)
	}
	if len(host.Session.Messages()) != 4 {
		t.Fatalf("cloned messages = %#v", host.Session.Messages())
	}
}

func TestRPCSessionHostCloneStartsNewEmptySessionLikePi(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	before := mustRPCHandleData[RPCSessionState](t, host, RPCCommand{Type: RPCCommandGetState})
	result := mustRPCHandleData[RPCCloneResult](t, host, RPCCommand{Type: RPCCommandClone})
	if result.Cancelled {
		t.Fatalf("clone result = %#v", result)
	}
	after := mustRPCHandleData[RPCSessionState](t, host, RPCCommand{Type: RPCCommandGetState})
	if after.SessionID == before.SessionID || after.MessageCount != 0 {
		t.Fatalf("after clone = %#v before = %#v", after, before)
	}
}

func TestRPCSessionHostEventSubscriptionRebindsAfterFork(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	mustRPCPrompt(t, host, "First")
	forkMessages := mustRPCHandleData[RPCForkMessagesResult](t, host, RPCCommand{Type: RPCCommandGetForkMessages})
	if len(forkMessages.Messages) == 0 {
		t.Fatal("expected forkable user message")
	}

	var events []AgentSessionEvent
	unsubscribe := host.SubscribeEvents(func(event AgentSessionEvent) {
		if event.Type == "message_end" {
			events = append(events, event)
		}
	})
	defer unsubscribe()

	forked := mustRPCHandleData[RPCForkResult](t, host, RPCCommand{Type: RPCCommandFork, EntryID: forkMessages.Messages[0].EntryID})
	if forked.Cancelled {
		t.Fatalf("fork result = %#v", forked)
	}
	mustRPCPrompt(t, host, "After fork")

	var roles []string
	for _, event := range events {
		if event.Message != nil {
			roles = append(roles, event.Message.Role)
		}
	}
	if !containsString(roles, llm.RoleUser) || !containsString(roles, llm.RoleAssistant) {
		t.Fatalf("rebound events roles = %#v events=%#v", roles, events)
	}
}

func TestRPCSessionHostTogglesAndNoopAbortCommands(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	disabled := false
	enabled := true

	if response := host.HandleCommand(context.Background(), RPCCommand{Type: RPCCommandSetAutoCompaction, Enabled: &disabled}); !response.Success {
		t.Fatalf("set auto compaction response = %#v", response)
	}
	state := mustRPCHandleData[RPCSessionState](t, host, RPCCommand{Type: RPCCommandGetState})
	if state.AutoCompactionEnabled || host.Session.CompactionSettings.Enabled {
		t.Fatalf("auto compaction state = %#v settings=%v", state, host.Session.CompactionSettings.Enabled)
	}

	if response := host.HandleCommand(context.Background(), RPCCommand{Type: RPCCommandSetAutoRetry, Enabled: &enabled}); !response.Success {
		t.Fatalf("set auto retry response = %#v", response)
	}
	if !host.Session.RetrySettings.Enabled || host.Session.RetrySettings.MaxRetries == 0 {
		t.Fatalf("retry settings = %#v", host.Session.RetrySettings)
	}
	for _, command := range []string{RPCCommandAbort, RPCCommandAbortRetry, RPCCommandAbortBash} {
		if response := host.HandleCommand(context.Background(), RPCCommand{Type: command}); !response.Success {
			t.Fatalf("%s response = %#v", command, response)
		}
	}
}

func TestRPCSessionHostSessionStats(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	mustRPCPrompt(t, host, "Hello")

	stats := mustRPCHandleData[RPCSessionStats](t, host, RPCCommand{Type: RPCCommandGetSessionStats})
	if stats.SessionFile == "" || stats.SessionID == "" || stats.UserMessages < 1 || stats.AssistantMessages < 1 {
		t.Fatalf("stats = %#v", stats)
	}
	if stats.TotalMessages < stats.UserMessages+stats.AssistantMessages || stats.Tokens.Total <= 0 {
		t.Fatalf("stats totals = %#v", stats)
	}
}

func TestRPCResultJSONShapesUsePiLowerCamelCase(t *testing.T) {
	compaction := mustMarshalJSON(t, agentharness.CompactionResult{
		Summary:          "summary",
		FirstKeptEntryID: "entry_1",
		TokensBefore:     42,
		Details:          map[string]any{"source": "test"},
	})
	assertJSONContains(t, compaction, `"firstKeptEntryId":"entry_1"`, `"tokensBefore":42`)
	assertJSONNotContains(t, compaction, "FirstKeptEntryID", "TokensBefore")

	bash := mustMarshalJSON(t, BashResult{
		Output:         "ok",
		ExitCode:       7,
		Cancelled:      true,
		Truncated:      true,
		FullOutputPath: "/tmp/gi-output",
		TotalLines:     10,
		OutputLines:    2,
	})
	assertJSONContains(t, bash, `"exitCode":7`, `"fullOutputPath":"/tmp/gi-output"`, `"totalLines":10`, `"outputLines":2`)
	assertJSONNotContains(t, bash, "ExitCode", "FullOutputPath", "TotalLines", "OutputLines")

	contextUsage := mustMarshalJSON(t, AgentContextUsage{ContextWindow: 100})
	assertJSONContains(t, contextUsage, `"tokens":null`, `"contextWindow":100`, `"percent":null`)
	assertJSONNotContains(t, contextUsage, "ContextWindow")

	stats := mustMarshalJSON(t, RPCSessionStats{
		SessionID:         "session",
		UserMessages:      1,
		AssistantMessages: 2,
		ToolCalls:         3,
		ToolResults:       4,
		TotalMessages:     5,
		Tokens:            RPCSessionTokens{Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4, Total: 10},
		Cost:              0.5,
		ContextUsage:      &AgentContextUsage{ContextWindow: 100},
	})
	assertJSONContains(t, stats, `"sessionId":"session"`, `"toolCalls":3`, `"toolResults":4`, `"totalMessages":5`, `"total":10`, `"cost":0.5`)
	assertJSONNotContains(t, stats, "SessionID", "ToolCalls", "TotalTokens")
}

func TestRPCSessionHostNewSessionClearsMessages(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	mustRPCPrompt(t, host, "Hello")
	before := mustRPCHandleData[RPCSessionState](t, host, RPCCommand{Type: RPCCommandGetState})
	if before.MessageCount == 0 {
		t.Fatalf("message count before new session = %d", before.MessageCount)
	}

	result := mustRPCHandleData[RPCCloneResult](t, host, RPCCommand{Type: RPCCommandNewSession})
	if result.Cancelled {
		t.Fatalf("new session cancelled = %#v", result)
	}
	after := mustRPCHandleData[RPCSessionState](t, host, RPCCommand{Type: RPCCommandGetState})
	if after.MessageCount != 0 {
		t.Fatalf("message count after new session = %d", after.MessageCount)
	}
}

func TestRPCSessionHostExportHTMLCreatesFile(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	mustRPCPrompt(t, host, "Hello")

	outputPath := filepath.Join(t.TempDir(), "session.html")
	result := mustRPCHandleData[RPCExportHTMLResult](t, host, RPCCommand{Type: RPCCommandExportHTML, OutputPath: outputPath})
	if !strings.HasSuffix(result.Path, ".html") {
		t.Fatalf("export path = %q", result.Path)
	}
	if _, err := os.Stat(result.Path); err != nil {
		t.Fatalf("exported html missing: %v", err)
	}
}

func TestRPCSessionHostExportHTMLEmptySessionMatchesPiError(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)

	outputPath := filepath.Join(t.TempDir(), "empty.html")
	_, err := host.ExportHTML(outputPath)
	if !errors.Is(err, errNothingToExport) {
		t.Fatalf("ExportHTML empty error = %v, want %v", err, errNothingToExport)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatalf("empty export file stat err = %v, want not exist", statErr)
	}
}

func TestRPCSessionHostLastAssistantText(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	initial := mustRPCHandleData[RPCLastAssistantTextResult](t, host, RPCCommand{Type: RPCCommandGetLastAssistantText})
	if initial.Text != nil {
		t.Fatalf("initial last assistant text = %#v", initial.Text)
	}

	mustRPCPrompt(t, host, "test123")
	after := mustRPCHandleData[RPCLastAssistantTextResult](t, host, RPCCommand{Type: RPCCommandGetLastAssistantText})
	if after.Text == nil || !strings.Contains(*after.Text, "test123") {
		t.Fatalf("last assistant text = %#v", after.Text)
	}
}

func TestRPCSessionHostSessionNamePersists(t *testing.T) {
	host, _, manager := createRPCSessionHostForTest(t)
	mustRPCPrompt(t, host, "Hello")

	response := host.HandleCommand(context.Background(), RPCCommand{Type: RPCCommandSetSessionName, Name: "my-test-session"})
	if !response.Success {
		t.Fatalf("set session name response = %#v", response)
	}
	state := mustRPCHandleData[RPCSessionState](t, host, RPCCommand{Type: RPCCommandGetState})
	if state.SessionName != "my-test-session" {
		t.Fatalf("session name = %q", state.SessionName)
	}
	entries := LoadEntriesFromFile(manager.GetSessionFile())
	if countRPCEntryType(entries, "session_info") != 1 {
		t.Fatalf("session_info entries = %d in %#v", countRPCEntryType(entries, "session_info"), entries)
	}
}

func TestAgentSessionSetSessionNameEmitsPiRegressionEvent(t *testing.T) {
	_, session, manager := createRPCSessionHostForTest(t)
	var names []string
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == ProtocolEventSessionInfoChanged {
			names = append(names, event.Name)
		}
	})

	if err := session.SetSessionName("hello world"); err != nil {
		t.Fatal(err)
	}
	if manager.GetSessionName() != "hello world" {
		t.Fatalf("session name = %q", manager.GetSessionName())
	}
	if !reflect.DeepEqual(names, []string{"hello world"}) {
		t.Fatalf("session name events = %#v", names)
	}
}

func TestProtocolExtensionSetSessionNameEmitsPiRegressionEvent(t *testing.T) {
	session := createDynamicSessionForTest(t, nil, nil)
	var names []string
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == ProtocolEventSessionInfoChanged {
			names = append(names, event.Name)
		}
	})
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{Path: "session-name", Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.On(ProtocolEventSessionStart, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			return ProtocolEventResult{}, ctx.SetSessionName("from extension")
		})
	}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAgentSessionRuntimeHost(session, runtime); err != nil {
		t.Fatal(err)
	}
	if session.SessionManager.GetSessionName() != "from extension" {
		t.Fatalf("session name = %q", session.SessionManager.GetSessionName())
	}
	if !reflect.DeepEqual(names, []string{"from extension"}) {
		t.Fatalf("session name events = %#v", names)
	}
}

func createRPCSessionHostForTest(t *testing.T, configure ...func(*AgentSessionOptions)) (*RPCSessionHost, *AgentSession, *SessionManager) {
	t.Helper()
	cwd := t.TempDir()
	manager, err := CreateSessionManager(cwd, filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	options := AgentSessionOptions{
		CWD:            cwd,
		AgentDir:       t.TempDir(),
		Model:          llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
		SessionManager: manager,
	}
	for _, apply := range configure {
		apply(&options)
	}
	session, err := CreateAgentSession(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Dispose)
	return NewRPCSessionHost(session), session, manager
}

func mustRPCHandleData[T any](t *testing.T, host *RPCSessionHost, command RPCCommand) T {
	t.Helper()
	response := host.HandleCommand(context.Background(), command)
	if !response.Success {
		t.Fatalf("HandleCommand(%s) = %#v", command.Type, response)
	}
	data, err := rpcResponseData[T](response)
	if err != nil {
		t.Fatalf("response data for %s error = %v", command.Type, err)
	}
	return data
}

func mustRPCPrompt(t *testing.T, host *RPCSessionHost, message string) {
	t.Helper()
	response := host.HandleCommand(context.Background(), RPCCommand{Type: RPCCommandPrompt, Message: message})
	if !response.Success {
		t.Fatalf("prompt response = %#v", response)
	}
}

func mustMarshalJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(data)
}

func assertJSONContains(t *testing.T, data string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if !strings.Contains(data, needle) {
			t.Fatalf("json %s missing %s", data, needle)
		}
	}
}

func assertJSONNotContains(t *testing.T, data string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if strings.Contains(data, needle) {
			t.Fatalf("json %s unexpectedly contains %s", data, needle)
		}
	}
}

func rolesFromEntries(entries []FileEntry) []string {
	roles := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type == "message" {
			roles = append(roles, sessionMessageRole(entry.Message))
		}
	}
	return roles
}

func countRPCEntryType(entries []FileEntry, entryType string) int {
	count := 0
	for _, entry := range entries {
		if entry.Type == entryType {
			count++
		}
	}
	return count
}
