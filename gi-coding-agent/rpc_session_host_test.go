package gicodingagent

import (
	"context"
	"os"
	"path/filepath"
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
				if message.Role == "bashExecution" && strings.Contains(rpcMessageText(message), unique) {
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

func TestRPCSessionHostSessionStats(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	mustRPCPrompt(t, host, "Hello")

	stats := mustRPCHandleData[RPCSessionStats](t, host, RPCCommand{Type: RPCCommandGetSessionStats})
	if stats.SessionFile == "" || stats.SessionID == "" || stats.UserMessages < 1 || stats.AssistantMessages < 1 {
		t.Fatalf("stats = %#v", stats)
	}
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
