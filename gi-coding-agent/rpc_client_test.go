package gicodingagent

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestRPCClientCloneSendsCloneRPCCommand(t *testing.T) {
	var sent []RPCCommand
	client := NewRPCClient(RPCCommandSenderFunc(func(ctx context.Context, command RPCCommand) (RPCResponse, error) {
		sent = append(sent, command)
		return RPCResponse{
			Type:    "response",
			Command: RPCCommandClone,
			Success: true,
			Data:    mustJSONRawMessage(t, RPCCloneResult{Cancelled: false}),
		}, nil
	}))

	result, err := client.Clone(context.Background())
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}

	wantCommand := []RPCCommand{{Type: RPCCommandClone}}
	if !reflect.DeepEqual(sent, wantCommand) {
		t.Fatalf("sent commands = %#v, want %#v", sent, wantCommand)
	}
	wantResult := RPCCloneResult{Cancelled: false}
	if result != wantResult {
		t.Fatalf("Clone() = %#v, want %#v", result, wantResult)
	}
}

func TestRPCClientNoDataMethodsSendPiCommandSurface(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		call func(*RPCClient) error
		want RPCCommand
	}{
		{name: "prompt", call: func(c *RPCClient) error { return c.Prompt(ctx, "hello") }, want: RPCCommand{Type: RPCCommandPrompt, Message: "hello"}},
		{name: "prompt_with_images", call: func(c *RPCClient) error {
			return c.PromptWithImages(ctx, "describe", []llm.ContentPart{llm.Image("img", "image/png")})
		}, want: RPCCommand{Type: RPCCommandPrompt, Message: "describe", Images: []llm.ContentPart{llm.Image("img", "image/png")}}},
		{name: "steer", call: func(c *RPCClient) error { return c.Steer(ctx, "interrupt") }, want: RPCCommand{Type: RPCCommandSteer, Message: "interrupt"}},
		{name: "steer_with_images", call: func(c *RPCClient) error {
			return c.SteerWithImages(ctx, "interrupt", []llm.ContentPart{llm.Image("img", "image/png")})
		}, want: RPCCommand{Type: RPCCommandSteer, Message: "interrupt", Images: []llm.ContentPart{llm.Image("img", "image/png")}}},
		{name: "follow_up", call: func(c *RPCClient) error { return c.FollowUp(ctx, "next") }, want: RPCCommand{Type: RPCCommandFollowUp, Message: "next"}},
		{name: "follow_up_with_images", call: func(c *RPCClient) error {
			return c.FollowUpWithImages(ctx, "next", []llm.ContentPart{llm.Image("img", "image/png")})
		}, want: RPCCommand{Type: RPCCommandFollowUp, Message: "next", Images: []llm.ContentPart{llm.Image("img", "image/png")}}},
		{name: "abort", call: func(c *RPCClient) error { return c.Abort(ctx) }, want: RPCCommand{Type: RPCCommandAbort}},
		{name: "set_thinking_level", call: func(c *RPCClient) error { return c.SetThinkingLevel(ctx, "high") }, want: RPCCommand{Type: RPCCommandSetThinkingLevel, Level: "high"}},
		{name: "set_steering_mode", call: func(c *RPCClient) error { return c.SetSteeringMode(ctx, "one-at-a-time") }, want: RPCCommand{Type: RPCCommandSetSteeringMode, Mode: "one-at-a-time"}},
		{name: "set_follow_up_mode", call: func(c *RPCClient) error { return c.SetFollowUpMode(ctx, "all") }, want: RPCCommand{Type: RPCCommandSetFollowUpMode, Mode: "all"}},
		{name: "set_auto_compaction", call: func(c *RPCClient) error { return c.SetAutoCompaction(ctx, false) }, want: RPCCommand{Type: RPCCommandSetAutoCompaction, Enabled: boolPointer(false)}},
		{name: "set_auto_retry", call: func(c *RPCClient) error { return c.SetAutoRetry(ctx, true) }, want: RPCCommand{Type: RPCCommandSetAutoRetry, Enabled: boolPointer(true)}},
		{name: "abort_retry", call: func(c *RPCClient) error { return c.AbortRetry(ctx) }, want: RPCCommand{Type: RPCCommandAbortRetry}},
		{name: "abort_bash", call: func(c *RPCClient) error { return c.AbortBash(ctx) }, want: RPCCommand{Type: RPCCommandAbortBash}},
		{name: "set_session_name", call: func(c *RPCClient) error { return c.SetSessionName(ctx, "demo") }, want: RPCCommand{Type: RPCCommandSetSessionName, Name: "demo"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent []RPCCommand
			client := NewRPCClient(RPCCommandSenderFunc(func(ctx context.Context, command RPCCommand) (RPCResponse, error) {
				sent = append(sent, command)
				return RPCResponse{Type: "response", Command: command.Type, Success: true}, nil
			}))

			if err := tt.call(client); err != nil {
				t.Fatalf("call error = %v", err)
			}
			if len(sent) != 1 || jsonString(t, sent[0]) != jsonString(t, tt.want) {
				t.Fatalf("sent commands = %#v, want %#v", sent, tt.want)
			}
		})
	}
}

func TestRPCClientDataMethodsDecodePiResponses(t *testing.T) {
	ctx := context.Background()
	model := llm.Model{Provider: "openai", ID: "gpt-4o-mini", ContextWindow: 128000}
	lastText := "done"
	leafID := "entry-2"
	entry := FileEntry{Type: "message", ID: leafID, Message: llm.UserMessageText("hello")}
	tree := []*SessionTreeNode{{Entry: entry, Children: []*SessionTreeNode{}}}
	client := NewRPCClient(RPCCommandSenderFunc(func(ctx context.Context, command RPCCommand) (RPCResponse, error) {
		var data any
		switch command.Type {
		case RPCCommandNewSession, RPCCommandSwitchSession, RPCCommandClone:
			data = RPCCloneResult{Cancelled: false}
		case RPCCommandGetState:
			data = RPCSessionState{ThinkingLevel: "medium", SessionID: "session-id"}
		case RPCCommandSetModel:
			data = model
		case RPCCommandCycleModel:
			data = RPCCycleModelResult{Model: model, ThinkingLevel: "low", IsScoped: false}
		case RPCCommandGetAvailableModels:
			data = RPCAvailableModelsResult{Models: []llm.Model{model}}
		case RPCCommandCycleThinkingLevel:
			data = RPCThinkingLevelResult{Level: "high"}
		case RPCCommandGetAvailableThinkingLevels:
			data = RPCThinkingLevelsResult{Levels: []string{"off", "low", "high"}}
		case RPCCommandCompact:
			data = agentharness.CompactionResult{Summary: "summary", TokensBefore: 42}
		case RPCCommandBash:
			data = BashResult{Output: "ok", ExitCode: 0}
		case RPCCommandGetSessionStats:
			data = RPCSessionStats{SessionID: "session-id", UserMessages: 1}
		case RPCCommandExportHTML:
			data = RPCExportHTMLResult{Path: "session.html"}
		case RPCCommandFork:
			data = RPCForkResult{Text: "hello", Cancelled: false}
		case RPCCommandGetForkMessages:
			data = RPCForkMessagesResult{Messages: []AgentSessionForkMessage{{EntryID: "entry-1", Text: "hello"}}}
		case RPCCommandGetEntries:
			if command.Since == nil || *command.Since != "entry-1" {
				t.Fatalf("get entries command = %#v", command)
			}
			data = RPCEntriesResult{Entries: []FileEntry{entry}, LeafID: &leafID}
		case RPCCommandGetTree:
			data = RPCTreeResult{Tree: tree, LeafID: &leafID}
		case RPCCommandGetLastAssistantText:
			data = RPCLastAssistantTextResult{Text: &lastText}
		case RPCCommandGetMessages:
			data = RPCMessagesResult{Messages: []llm.Message{llm.UserMessageText("hello")}}
		case RPCCommandGetCommands:
			data = RPCCommandsResult{Commands: []RPCSlashCommand{{Name: "skill:demo", Source: "skill"}}}
		default:
			t.Fatalf("unexpected command: %#v", command)
		}
		return RPCResponse{Type: "response", Command: command.Type, Success: true, Data: mustJSONRawMessage(t, data)}, nil
	}))

	if result, err := client.NewSession(ctx, "parent"); err != nil || result.Cancelled {
		t.Fatalf("NewSession = %#v err=%v", result, err)
	}
	if result, err := client.GetState(ctx); err != nil || result.SessionID != "session-id" {
		t.Fatalf("GetState = %#v err=%v", result, err)
	}
	if result, err := client.SetModel(ctx, "openai", "gpt-4o-mini"); err != nil || result.ID != model.ID {
		t.Fatalf("SetModel = %#v err=%v", result, err)
	}
	if result, err := client.CycleModel(ctx); err != nil || result == nil || result.Model.ID != model.ID {
		t.Fatalf("CycleModel = %#v err=%v", result, err)
	}
	if result, err := client.GetAvailableModels(ctx); err != nil || len(result) != 1 {
		t.Fatalf("GetAvailableModels = %#v err=%v", result, err)
	}
	if result, err := client.CycleThinkingLevel(ctx); err != nil || result == nil || result.Level != "high" {
		t.Fatalf("CycleThinkingLevel = %#v err=%v", result, err)
	}
	if result, err := client.GetAvailableThinkingLevels(ctx); err != nil || !reflect.DeepEqual(result, []string{"off", "low", "high"}) {
		t.Fatalf("GetAvailableThinkingLevels = %#v err=%v", result, err)
	}
	if result, err := client.Compact(ctx, "short"); err != nil || result.Summary != "summary" {
		t.Fatalf("Compact = %#v err=%v", result, err)
	}
	if result, err := client.Bash(ctx, "echo ok"); err != nil || result.Output != "ok" {
		t.Fatalf("Bash = %#v err=%v", result, err)
	}
	if result, err := client.GetSessionStats(ctx); err != nil || result.SessionID != "session-id" {
		t.Fatalf("GetSessionStats = %#v err=%v", result, err)
	}
	if result, err := client.ExportHTML(ctx, "session.html"); err != nil || result.Path != "session.html" {
		t.Fatalf("ExportHTML = %#v err=%v", result, err)
	}
	if result, err := client.SwitchSession(ctx, "session.jsonl"); err != nil || result.Cancelled {
		t.Fatalf("SwitchSession = %#v err=%v", result, err)
	}
	if result, err := client.Fork(ctx, "entry-1"); err != nil || result.Text != "hello" {
		t.Fatalf("Fork = %#v err=%v", result, err)
	}
	if result, err := client.GetForkMessages(ctx); err != nil || len(result) != 1 {
		t.Fatalf("GetForkMessages = %#v err=%v", result, err)
	}
	if result, err := client.GetEntries(ctx, "entry-1"); err != nil ||
		len(result.Entries) != 1 ||
		result.LeafID == nil ||
		*result.LeafID != leafID {
		t.Fatalf("GetEntries = %#v err=%v", result, err)
	}
	if result, err := client.GetTree(ctx); err != nil ||
		len(result.Tree) != 1 ||
		result.LeafID == nil ||
		*result.LeafID != leafID {
		t.Fatalf("GetTree = %#v err=%v", result, err)
	}
	if result, err := client.GetLastAssistantText(ctx); err != nil || result == nil || *result != "done" {
		t.Fatalf("GetLastAssistantText = %#v err=%v", result, err)
	}
	if result, err := client.GetMessages(ctx); err != nil || len(result) != 1 {
		t.Fatalf("GetMessages = %#v err=%v", result, err)
	}
	if result, err := client.GetCommands(ctx); err != nil || len(result) != 1 {
		t.Fatalf("GetCommands = %#v err=%v", result, err)
	}
	if result, err := client.Clone(ctx); err != nil || result.Cancelled {
		t.Fatalf("Clone = %#v err=%v", result, err)
	}
	if _, err := client.GetEntries(ctx, "one", "two"); err == nil {
		t.Fatal("GetEntries accepted more than one since entry ID")
	}
}

func mustJSONRawMessage(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%#v) error = %v", value, err)
	}
	return data
}

func boolPointer(value bool) *bool {
	return &value
}

func jsonString(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
