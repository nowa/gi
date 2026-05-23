package gicodingagent

import (
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionRetrySucceedsAfterTransientError(t *testing.T) {
	session, calls := createRetryTestSession(t, 1, 3)
	defer session.Dispose()
	var events []string
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "auto_retry_start" {
			events = append(events, "start:"+string(rune('0'+event.Attempt)))
		}
		if event.Type == "auto_retry_end" {
			events = append(events, "end:success="+boolString(event.Success))
		}
	})

	if err := session.Prompt("Test"); err != nil {
		t.Fatal(err)
	}
	if *calls != 2 {
		t.Fatalf("calls = %d, want 2", *calls)
	}
	if got := joinEventStrings(events); got != "start:1,end:success=true" {
		t.Fatalf("events = %q", got)
	}
	if session.IsRetrying() {
		t.Fatal("session should not be retrying")
	}
}

func TestAgentSessionRetryExhaustsMaxRetries(t *testing.T) {
	session, calls := createRetryTestSession(t, 99, 2)
	defer session.Dispose()
	var events []string
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "auto_retry_start" {
			events = append(events, "start:"+string(rune('0'+event.Attempt)))
		}
		if event.Type == "auto_retry_end" {
			events = append(events, "end:success="+boolString(event.Success))
		}
	})

	if err := session.Prompt("Test"); err != nil {
		t.Fatal(err)
	}
	if *calls != 3 {
		t.Fatalf("calls = %d, want 3", *calls)
	}
	if got := joinEventStrings(events); got != "start:1,start:2,end:success=false" {
		t.Fatalf("events = %q", got)
	}
	if session.IsRetrying() {
		t.Fatal("session should not be retrying")
	}
}

func TestAgentSessionPromptWaitsForRetryCompletionWithDelayedMessageEnd(t *testing.T) {
	session, calls := createRetryTestSession(t, 1, 3)
	defer session.Dispose()
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "message_end" && event.Message != nil && event.Message.Role == llm.RoleAssistant {
			time.Sleep(40 * time.Millisecond)
		}
	})

	if err := session.Prompt("Test"); err != nil {
		t.Fatal(err)
	}
	if *calls != 2 {
		t.Fatalf("calls = %d, want 2", *calls)
	}
	if session.IsRetrying() {
		t.Fatal("session should not be retrying")
	}
}

func TestAgentSessionRetryProviderNetworkError(t *testing.T) {
	callCount := 0
	session := createRetryTestSessionWithResponder(t, 3, func(_ string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		callCount++
		if callCount == 1 {
			return retryAssistantError("Provider finish_reason: network_error"), nil
		}
		return retryAssistantText("Recovered after retry"), nil
	})
	defer session.Dispose()
	var events []string
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "auto_retry_start" {
			events = append(events, "start:"+string(rune('0'+event.Attempt)))
		}
		if event.Type == "auto_retry_end" {
			events = append(events, "end:success="+boolString(event.Success))
		}
	})

	if err := session.Prompt("Test"); err != nil {
		t.Fatal(err)
	}
	if callCount != 2 {
		t.Fatalf("calls = %d, want 2", callCount)
	}
	if got := joinEventStrings(events); got != "start:1,end:success=true" {
		t.Fatalf("events = %q", got)
	}
}

func TestAgentSessionRetryNetworkConnectionLostPiRegression(t *testing.T) {
	callCount := 0
	session := createRetryTestSessionWithResponder(t, 3, func(_ string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		callCount++
		if callCount == 1 {
			return retryAssistantError("Network connection lost."), nil
		}
		return retryAssistantText("recovered after reconnect"), nil
	})
	defer session.Dispose()
	var retryErrors []string
	var retryEnds []bool
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "auto_retry_start" {
			retryErrors = append(retryErrors, event.ErrorMessage)
		}
		if event.Type == "auto_retry_end" {
			retryEnds = append(retryEnds, event.Success)
		}
	})

	if err := session.Prompt("test"); err != nil {
		t.Fatal(err)
	}
	if callCount != 2 {
		t.Fatalf("calls = %d, want 2", callCount)
	}
	if len(retryErrors) != 1 || retryErrors[0] != "Network connection lost." {
		t.Fatalf("retry errors = %#v", retryErrors)
	}
	if len(retryEnds) != 1 || !retryEnds[0] {
		t.Fatalf("retry end events = %#v", retryEnds)
	}
}

func TestAgentSessionPromptWaitsForRetryToolLoop(t *testing.T) {
	callCount := 0
	toolExecuted := false
	session := createRetryTestSessionWithResponder(t, 3, func(_ string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		callCount++
		switch callCount {
		case 1:
			return retryAssistantError("overloaded_error"), nil
		case 2:
			message := retryAssistantText("Looking that up now.")
			message.StopReason = "toolUse"
			message.Content = []llm.ContentPart{
				llm.Text("Looking that up now."),
				llm.ToolCall("call_1", "echo", map[string]any{"text": "hello"}),
			}
			return message, nil
		default:
			return retryAssistantText("Final answer."), nil
		}
	})
	session.Agent.State.Tools = []SDKTool{{
		Name: "echo",
		Execute: func(_ string, _ map[string]any) (SDKToolResult, error) {
			toolExecuted = true
			return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "echoed"}}}, nil
		},
	}}
	defer session.Dispose()

	if err := session.Prompt("Test"); err != nil {
		t.Fatal(err)
	}
	if callCount != 3 {
		t.Fatalf("calls = %d, want 3", callCount)
	}
	if !toolExecuted {
		t.Fatal("tool should execute before prompt returns")
	}
	if session.IsRetrying() {
		t.Fatal("session should not be retrying")
	}
	if err := session.Prompt("Follow-up"); err != nil {
		t.Fatal(err)
	}
	if callCount != 4 {
		t.Fatalf("calls after follow-up = %d, want 4", callCount)
	}
}

func createRetryTestSession(t *testing.T, failCount, maxRetries int) (*AgentSession, *int) {
	t.Helper()
	callCount := 0
	session := createRetryTestSessionWithResponder(t, maxRetries, func(_ string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		callCount++
		if callCount <= failCount {
			return retryAssistantError("overloaded_error"), nil
		}
		return retryAssistantText("Success"), nil
	})
	return session, &callCount
}

func createRetryTestSessionWithResponder(t *testing.T, maxRetries int, responder AgentSessionResponder) *AgentSession {
	t.Helper()
	cwd := t.TempDir()
	manager, err := InMemorySessionManager(cwd)
	if err != nil {
		t.Fatal(err)
	}
	retry := AgentSessionRetrySettings{Enabled: true, MaxRetries: maxRetries, BaseDelayMs: 1}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            cwd,
		AgentDir:       t.TempDir(),
		Model:          llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
		SessionManager: manager,
		RetrySettings:  &retry,
		Responder:      responder,
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func retryAssistantText(text string) llm.Message {
	return llm.Message{
		Role:       llm.RoleAssistant,
		Content:    []llm.ContentPart{llm.Text(text)},
		StopReason: llm.StopReasonStop,
		Timestamp:  llm.NowMillis(),
	}
}

func retryAssistantError(message string) llm.Message {
	return llm.Message{
		Role:         llm.RoleAssistant,
		Content:      []llm.ContentPart{},
		StopReason:   llm.StopReasonError,
		ErrorMessage: message,
		Timestamp:    llm.NowMillis(),
	}
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func joinEventStrings(events []string) string {
	result := ""
	for index, event := range events {
		if index > 0 {
			result += ","
		}
		result += event
	}
	return result
}
