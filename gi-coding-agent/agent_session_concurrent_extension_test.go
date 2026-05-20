package gicodingagent

import (
	"reflect"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionQueuesExtensionOriginSteeringMessagesWhileStreaming(t *testing.T) {
	session, started, release := createBlockingConcurrentSession(t)
	defer session.Dispose()
	var extensionContext *ProtocolExtensionContext
	var lastInputSource string
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{Path: "input-extension", Factory: func(ctx *ProtocolExtensionContext) error {
		extensionContext = ctx
		return ctx.On("input", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			lastInputSource = event.Source
			return ProtocolEventResult{}, nil
		})
	}}}); err != nil {
		t.Fatal(err)
	}
	runtime.BindSession(session)
	var queueEvents []ProtocolSessionEvent
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "queue_update" {
			queueEvents = append(queueEvents, ProtocolSessionEvent{Steering: event.Steering, FollowUp: event.FollowUp})
		}
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Prompt("First message")
	}()
	<-started

	if err := extensionContext.SendUserMessage("Steer from extension", ProtocolSendUserMessageOptions{DeliverAs: "steer"}); err != nil {
		t.Fatal(err)
	}
	if session.PendingMessageCount() != 1 {
		t.Fatalf("pending count = %d", session.PendingMessageCount())
	}
	if got := session.GetSteeringMessages(); len(got) != 1 || got[0] != "Steer from extension" {
		t.Fatalf("steering messages = %#v", got)
	}
	if lastInputSource != "extension" {
		t.Fatalf("input source = %q", lastInputSource)
	}
	if len(queueEvents) != 1 || !containsString(queueEvents[0].Steering, "Steer from extension") {
		t.Fatalf("queue events = %#v", queueEvents)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestAgentSessionToolCallHooksSeeAssistantPersistedBeforeToolResults(t *testing.T) {
	session := createToolOrderingSession(t, false)
	defer session.Dispose()
	var snapshots [][]string
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{Path: "tool-call-extension", Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.On("tool_call", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			snapshots = append(snapshots, messageRoles(session.SessionManager.GetEntries()))
			return ProtocolEventResult{}, nil
		})
	}}}); err != nil {
		t.Fatal(err)
	}
	runtime.BindSession(session)

	if err := session.Prompt("hi"); err != nil {
		t.Fatal(err)
	}

	want := [][]string{{"user", "assistant"}, {"user", "assistant"}}
	if !reflect.DeepEqual(snapshots, want) {
		t.Fatalf("snapshots = %#v, want %#v", snapshots, want)
	}
}

func TestAgentSessionMessageEndHooksDoNotReorderPersistedToolLoopMessages(t *testing.T) {
	session := createToolOrderingSession(t, true)
	defer session.Dispose()
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{Path: "message-end-extension", Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.On("message_end", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			if event.Role == llm.RoleAssistant {
				time.Sleep(40 * time.Millisecond)
			}
			return ProtocolEventResult{}, nil
		})
	}}}); err != nil {
		t.Fatal(err)
	}
	runtime.BindSession(session)

	if err := session.Prompt("hi"); err != nil {
		t.Fatal(err)
	}

	want := []string{"user", "assistant", "toolResult", "assistant"}
	if got := messageRoles(session.SessionManager.GetEntries()); !reflect.DeepEqual(got, want) {
		t.Fatalf("message roles = %#v, want %#v", got, want)
	}
}

func createToolOrderingSession(t *testing.T, includeTextBeforeTool bool) *AgentSession {
	t.Helper()
	callCount := 0
	session := createConcurrentSessionWithResponder(t, func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error) {
		callCount++
		if callCount > 1 {
			return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done")}, StopReason: llm.StopReasonStop}, nil
		}
		content := []llm.ContentPart{}
		if includeTextBeforeTool {
			content = append(content, llm.Text("calling tool"))
			content = append(content, llm.ToolCall("toolu_1", "dummy", map[string]any{"q": "x"}))
		} else {
			content = append(content,
				llm.ToolCall("toolu_1", "dummy", map[string]any{"q": "x"}),
				llm.ToolCall("toolu_2", "dummy", map[string]any{"q": "y"}),
			)
		}
		return llm.Message{Role: llm.RoleAssistant, Content: content, StopReason: "toolUse"}, nil
	})
	session.Agent.State.Tools = append(session.Agent.State.Tools, SDKTool{
		Name: "dummy",
		Execute: func(toolCallID string, input map[string]any) (SDKToolResult, error) {
			return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "result"}}}, nil
		},
	})
	return session
}

func messageRoles(entries []FileEntry) []string {
	roles := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type == "message" {
			roles = append(roles, sessionMessageRole(entry.Message))
		}
	}
	return roles
}
