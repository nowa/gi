package gicodingagent

import (
	"reflect"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionModelExtensionToolCallBlockPiParity(t *testing.T) {
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), nil)
	session.Agent.State.Tools = append(session.Agent.State.Tools, SDKTool{
		Name: "echo",
		Execute: func(string, map[string]any) (SDKToolResult, error) {
			t.Fatal("blocked tool should not execute")
			return SDKToolResult{}, nil
		},
	})
	callCount := 0
	session.Responder = func(_ string, context []llm.Message, _ llm.Model) (llm.Message, error) {
		callCount++
		if callCount == 1 {
			return llm.Message{
				Role:       llm.RoleAssistant,
				StopReason: "toolUse",
				Content:    []llm.ContentPart{llm.ToolCall("call-1", "echo", map[string]any{"text": "hello"})},
			}, nil
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text(firstToolResultText(context))}}, nil
	}
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "blocker.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.On(ProtocolEventToolCall, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			if event.ToolName != "echo" || event.ToolCallID != "call-1" || event.Input["text"] != "hello" {
				t.Fatalf("tool_call event = %#v", event)
			}
			return ProtocolEventResult{Block: true, Reason: "Blocked by test"}, nil
		})
	}})
	if _, err := NewAgentSessionRuntimeHost(session, runtime); err != nil {
		t.Fatal(err)
	}

	if err := session.Prompt("hi"); err != nil {
		t.Fatal(err)
	}
	messages := session.Messages()
	if !containsAssistantText(messages, "Blocked by test") {
		t.Fatalf("assistant messages = %#v", messages)
	}
	if result := findToolResult(messages); result == nil || !result.IsError {
		t.Fatalf("tool result = %#v", result)
	}
}

func TestAgentSessionModelExtensionToolResultModifyPiParity(t *testing.T) {
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), nil)
	session.Agent.State.Tools = append(session.Agent.State.Tools, SDKTool{
		Name: "echo",
		Execute: func(_ string, input map[string]any) (SDKToolResult, error) {
			return SDKToolResult{
				Content: []SDKContentPart{{Type: "text", Text: stringFromSessionMessageValue(input["text"])}},
				Details: map[string]any{"text": stringFromSessionMessageValue(input["text"])},
			}, nil
		},
	})
	callCount := 0
	session.Responder = func(_ string, context []llm.Message, _ llm.Model) (llm.Message, error) {
		callCount++
		if callCount == 1 {
			return llm.Message{
				Role:       llm.RoleAssistant,
				StopReason: "toolUse",
				Content:    []llm.ContentPart{llm.ToolCall("call-1", "echo", map[string]any{"text": "hello"})},
			}, nil
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text(firstToolResultText(context))}}, nil
	}
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "patcher.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.On(ProtocolEventToolResult, func(ProtocolSessionEvent) (ProtocolEventResult, error) {
			return ProtocolEventResult{
				Content:    []SDKContentPart{{Type: "text", Text: "patched result"}},
				ContentSet: true,
				Details:    map[string]any{"patched": true},
				DetailsSet: true,
			}, nil
		})
	}})
	if _, err := NewAgentSessionRuntimeHost(session, runtime); err != nil {
		t.Fatal(err)
	}

	if err := session.Prompt("hi"); err != nil {
		t.Fatal(err)
	}
	messages := session.Messages()
	if !containsAssistantText(messages, "patched result") {
		t.Fatalf("assistant messages = %#v", messages)
	}
	result := findToolResult(messages)
	if result == nil {
		t.Fatalf("tool result = %#v", result)
	}
	details, _ := result.Details.(map[string]any)
	if details["patched"] != true {
		t.Fatalf("tool result details = %#v", result.Details)
	}
}

func TestAgentSessionModelExtensionContextModifyPiParity(t *testing.T) {
	var providerUserText string
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), func(_ string, context []llm.Message, _ llm.Model) (llm.Message, error) {
		for _, message := range context {
			if message.Role == llm.RoleUser {
				providerUserText = rpcMessageText(message)
				break
			}
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done")}}, nil
	})
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "context.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.On(ProtocolEventContext, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			messages := append([]llm.Message(nil), event.Messages...)
			for index := range messages {
				if messages[index].Role == llm.RoleUser {
					messages[index].Content = []llm.ContentPart{llm.Text("rewritten")}
				}
			}
			return ProtocolEventResult{Messages: messages, MessagesSet: true}, nil
		})
	}})
	if _, err := NewAgentSessionRuntimeHost(session, runtime); err != nil {
		t.Fatal(err)
	}

	if err := session.Prompt("original"); err != nil {
		t.Fatal(err)
	}
	if providerUserText != "rewritten" {
		t.Fatalf("provider user text = %q", providerUserText)
	}
	assertFirstUserMessageText(t, session, "original")
}

func TestAgentSessionModelExtensionBeforeAgentStartPiParity(t *testing.T) {
	var session *AgentSession
	var providerSystemPrompt string
	var providerUserTexts []string
	session = newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), func(_ string, context []llm.Message, _ llm.Model) (llm.Message, error) {
		providerSystemPrompt = session.SystemPrompt
		providerUserTexts = userTexts(context)
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done")}}, nil
	})
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents, CapabilitySystemPromptModify)
	mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "before-start.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.On(ProtocolEventBeforeAgentStart, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			return ProtocolEventResult{
				CustomMessages: []ProtocolCustomMessage{{
					CustomType: "before-start",
					Content:    "injected",
					Display:    true,
					Details:    map[string]any{"injected": true},
				}},
				CustomMessagesSet: true,
				SystemPrompt:      event.SystemPrompt + "\n\nextra instructions",
				SystemPromptSet:   true,
			}, nil
		})
	}})
	if _, err := NewAgentSessionRuntimeHost(session, runtime); err != nil {
		t.Fatal(err)
	}

	if err := session.Prompt("hello"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(providerSystemPrompt, "extra instructions") {
		t.Fatalf("provider system prompt = %q", providerSystemPrompt)
	}
	if !reflect.DeepEqual(providerUserTexts, []string{"hello", "injected"}) {
		t.Fatalf("provider user texts = %#v", providerUserTexts)
	}
	if !hasCustomMessageEntry(session.SessionManager.GetEntries(), "before-start", "injected") {
		t.Fatalf("entries = %#v", session.SessionManager.GetEntries())
	}
}

func TestRPCSessionHostScopedModelCyclingPreservesThinkingPreferencePiParity(t *testing.T) {
	session := newPromptExpansionSessionWithResponder(t, t.TempDir(), t.TempDir(), nil)
	modelOne := llm.Model{Provider: "faux", ID: "faux-1", Reasoning: true}
	modelTwo := llm.Model{Provider: "faux", ID: "faux-2", Reasoning: false}
	session.Agent.State.Model = modelOne
	session.Agent.State.ThinkingLevel = string(ThinkingHigh)
	host := &RPCSessionHost{
		Session: session,
		ScopedModels: []RPCScopedModel{
			{Model: modelOne, ThinkingLevel: string(ThinkingHigh)},
			{Model: modelTwo},
		},
	}

	first, err := host.CycleModel()
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || first.Model.ID != "faux-2" || first.ThinkingLevel != string(ThinkingOff) || !first.IsScoped {
		t.Fatalf("first cycle = %#v", first)
	}
	second, err := host.CycleModel()
	if err != nil {
		t.Fatal(err)
	}
	if second == nil || second.Model.ID != "faux-1" || second.ThinkingLevel != string(ThinkingHigh) || !second.IsScoped {
		t.Fatalf("second cycle = %#v", second)
	}
}

func firstToolResultText(messages []llm.Message) string {
	if message := findToolResult(messages); message != nil {
		return rpcMessageText(*message)
	}
	return ""
}

func findToolResult(messages []llm.Message) *llm.Message {
	for index := range messages {
		if messages[index].Role == llm.RoleToolResult {
			return &messages[index]
		}
	}
	return nil
}

func containsAssistantText(messages []llm.Message, text string) bool {
	for _, message := range messages {
		if message.Role == llm.RoleAssistant && strings.Contains(rpcMessageText(message), text) {
			return true
		}
	}
	return false
}
