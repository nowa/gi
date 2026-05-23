package gicodingagent

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionRecordBashResultPersistsImmediatelyWhenIdle(t *testing.T) {
	session := createConcurrentSessionWithResponder(t, func(_ string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		return llm.AssistantMessage([]llm.ContentPart{llm.Text("ok")}, llm.StopReasonStop, llm.MustGetModel("anthropic", "claude-sonnet-4-5")), nil
	})

	session.RecordBashResult("echo hi", BashResult{Output: "hi", ExitCode: 0})

	if session.HasPendingBashMessages() {
		t.Fatal("bash result should not be pending while idle")
	}
	messages := session.Messages()
	if len(messages) == 0 || messages[len(messages)-1].Role != "bashExecution" {
		t.Fatalf("messages = %#v", messages)
	}
	if !entryTypesContain(session.SessionManager.GetEntries(), "message") {
		t.Fatalf("entries = %#v", session.SessionManager.GetEntries())
	}
}

func TestAgentSessionBashResultDefersWhileStreamingAndFlushesBeforeNextPrompt(t *testing.T) {
	releaseTool := make(chan struct{})
	callCount := 0
	session := createConcurrentSessionWithResponder(t, func(_ string, _ []llm.Message, model llm.Model) (llm.Message, error) {
		callCount++
		if callCount == 1 {
			return llm.Message{
				Role:       llm.RoleAssistant,
				Content:    []llm.ContentPart{llm.ToolCall("toolu_wait", "wait", map[string]any{})},
				StopReason: "toolUse",
				Provider:   model.Provider,
				Model:      model.ID,
				API:        model.API,
			}, nil
		}
		return llm.AssistantMessage([]llm.ContentPart{llm.Text("done")}, llm.StopReasonStop, model), nil
	})
	session.Agent.State.Tools = append(session.Agent.State.Tools, SDKTool{
		Name: "wait",
		Execute: func(string, map[string]any) (SDKToolResult, error) {
			<-releaseTool
			return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "released"}}}, nil
		},
	})

	toolStarted := make(chan struct{})
	unsubscribe := session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "tool_execution_start" {
			closeOnce(toolStarted)
		}
	})
	defer unsubscribe()

	firstPrompt := make(chan error, 1)
	go func() {
		firstPrompt <- session.Prompt("start")
	}()
	waitForChannel(t, toolStarted)

	session.RecordBashResult("echo hi", BashResult{Output: "hi", ExitCode: 0})
	if !session.HasPendingBashMessages() {
		t.Fatal("bash result should be pending while prompt is streaming")
	}
	if hasMessageRole(session.Messages(), "bashExecution") {
		t.Fatalf("bashExecution should not be visible before flush: %#v", session.Messages())
	}

	close(releaseTool)
	if err := <-firstPrompt; err != nil {
		t.Fatal(err)
	}
	if !session.HasPendingBashMessages() {
		t.Fatal("bash result should remain pending until the next prompt")
	}

	if err := session.Prompt("next turn"); err != nil {
		t.Fatal(err)
	}
	if session.HasPendingBashMessages() {
		t.Fatal("pending bash result should be flushed")
	}
	if !hasMessageRole(session.Messages(), "bashExecution") {
		t.Fatalf("messages = %#v", session.Messages())
	}
}

func TestAgentSessionExecuteBashRecordsResult(t *testing.T) {
	session := createConcurrentSessionWithResponder(t, nil)

	result, err := session.ExecuteBash("printf 'hello'")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "hello") {
		t.Fatalf("bash result = %#v", result)
	}
	if messages := session.Messages(); len(messages) == 0 || messages[len(messages)-1].Role != "bashExecution" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestAgentSessionAbortBashCancelsRunningCommand(t *testing.T) {
	session := createConcurrentSessionWithResponder(t, nil)
	operations := BashOperations{
		Exec: func(_ string, _ string, options BashExecOptions) (BashOperationResult, error) {
			<-options.Context.Done()
			return BashOperationResult{}, errors.New("aborted")
		},
	}

	done := make(chan struct {
		result BashResult
		err    error
	}, 1)
	go func() {
		result, err := session.ExecuteBash("sleep", AgentSessionBashOptions{Operations: operations})
		done <- struct {
			result BashResult
			err    error
		}{result: result, err: err}
	}()
	waitUntil(t, session.IsBashRunning)
	session.AbortBash()

	completed := <-done
	if completed.err != nil {
		t.Fatalf("ExecuteBash err = %v", completed.err)
	}
	if !completed.result.Cancelled || session.IsBashRunning() {
		t.Fatalf("result = %#v running=%v", completed.result, session.IsBashRunning())
	}
}

func TestAgentSessionDisposeAbortsRunningBashPiStyle(t *testing.T) {
	session := createConcurrentSessionWithResponder(t, nil)
	operations := BashOperations{
		Exec: func(_ string, _ string, options BashExecOptions) (BashOperationResult, error) {
			<-options.Context.Done()
			return BashOperationResult{}, errors.New("aborted")
		},
	}

	done := make(chan struct {
		result BashResult
		err    error
	}, 1)
	go func() {
		result, err := session.ExecuteBash("sleep", AgentSessionBashOptions{Operations: operations})
		done <- struct {
			result BashResult
			err    error
		}{result: result, err: err}
	}()
	waitUntil(t, session.IsBashRunning)
	session.Dispose()

	completed := <-done
	if completed.err != nil {
		t.Fatalf("ExecuteBash err = %v", completed.err)
	}
	if !completed.result.Cancelled || session.IsBashRunning() {
		t.Fatalf("result = %#v running=%v", completed.result, session.IsBashRunning())
	}
}

func TestAgentSessionPersistsCustomUserAssistantAndToolMessagesInOrder(t *testing.T) {
	callCount := 0
	session := createConcurrentSessionWithResponder(t, func(_ string, _ []llm.Message, model llm.Model) (llm.Message, error) {
		callCount++
		if callCount == 1 {
			return llm.Message{
				Role:       llm.RoleAssistant,
				Content:    []llm.ContentPart{llm.ToolCall("toolu_echo", "echo", map[string]any{"text": "hello"})},
				StopReason: "toolUse",
				Provider:   model.Provider,
				Model:      model.ID,
				API:        model.API,
			}, nil
		}
		return llm.AssistantMessage([]llm.ContentPart{llm.Text("done")}, llm.StopReasonStop, model), nil
	})
	session.Agent.State.Tools = append(session.Agent.State.Tools, SDKTool{
		Name: "echo",
		Execute: func(toolCallID string, input map[string]any) (SDKToolResult, error) {
			text, _ := input["text"].(string)
			return SDKToolResult{
				Content: []SDKContentPart{{Type: "text", Text: "echo:" + text}},
				Details: map[string]any{
					"text":       text,
					"toolCallID": toolCallID,
				},
			}, nil
		},
	})

	if err := session.SendCustomMessage(QueuedCustomMessage{CustomType: "note", Content: "hello", Display: true, Details: map[string]any{"a": 1}}, ProtocolSendCustomMessageOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt("start"); err != nil {
		t.Fatal(err)
	}

	if got, want := entryTypes(session.SessionManager.GetEntries()), []string{"custom_message", "message", "message", "message", "message"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("entry types = %#v, want %#v", got, want)
	}
	if got, want := messageRolesFromMessages(session.Messages()), []string{"custom", "user", "assistant", "toolResult", "assistant"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("message roles = %#v, want %#v", got, want)
	}
}

func TestAgentSessionRecordBashResultDoesNotEmitMessageEnd(t *testing.T) {
	session := createConcurrentSessionWithResponder(t, nil)
	var roles []string
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "message_end" && event.Message != nil {
			roles = append(roles, event.Message.Role)
		}
	})

	session.RecordBashResult("echo hi", BashResult{Output: "hi", ExitCode: 0})

	if len(roles) != 0 {
		t.Fatalf("message_end roles = %#v", roles)
	}
}

func TestAgentSessionPersistsAbortedAssistantMessages(t *testing.T) {
	session := createConcurrentSessionWithResponder(t, func(_ string, _ []llm.Message, model llm.Model) (llm.Message, error) {
		return llm.AssistantMessage([]llm.ContentPart{llm.Text(strings.Repeat("x", 20_000))}, llm.StopReasonStop, model), nil
	})
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "message_update" {
			_ = session.Abort()
		}
	})

	if err := session.Prompt("hi"); err != nil {
		t.Fatal(err)
	}

	entries := session.SessionManager.GetEntries()
	last := entries[len(entries)-1]
	if last.Type != "message" {
		t.Fatalf("last entry = %#v", last)
	}
	message, ok := sessionMessageToLLM(last.Message)
	if !ok || message.Role != llm.RoleAssistant || message.StopReason != llm.StopReasonAborted {
		t.Fatalf("last message = %#v ok=%v", message, ok)
	}
}

func TestAgentSessionExecuteBashRecordsCustomOperationsOutput(t *testing.T) {
	session := createConcurrentSessionWithResponder(t, nil)
	operations := BashOperations{
		Exec: func(_ string, _ string, options BashExecOptions) (BashOperationResult, error) {
			options.OnData([]byte("hello from custom ops"))
			return BashOperationResult{ExitCode: 0}, nil
		},
	}

	result, err := session.ExecuteBash("custom", AgentSessionBashOptions{Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "hello from custom ops") {
		t.Fatalf("bash result = %#v", result)
	}
	if messages := session.Messages(); len(messages) == 0 || messages[len(messages)-1].Role != "bashExecution" {
		t.Fatalf("messages = %#v", messages)
	}
}

func entryTypes(entries []FileEntry) []string {
	types := make([]string, 0, len(entries))
	for _, entry := range entries {
		types = append(types, entry.Type)
	}
	return types
}

func entryTypesContain(entries []FileEntry, want string) bool {
	for _, entry := range entries {
		if entry.Type == want {
			return true
		}
	}
	return false
}

func hasMessageRole(messages []llm.Message, role string) bool {
	for _, message := range messages {
		if message.Role == role {
			return true
		}
	}
	return false
}

func messageRolesFromMessages(messages []llm.Message) []string {
	roles := make([]string, 0, len(messages))
	for _, message := range messages {
		roles = append(roles, message.Role)
	}
	return roles
}

func waitForChannel(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel")
	}
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func closeOnce(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}
