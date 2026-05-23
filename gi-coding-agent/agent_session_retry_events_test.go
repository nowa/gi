package gicodingagent

import (
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionRetryEventsPiParityTransientSuccess(t *testing.T) {
	retry := AgentSessionRetrySettings{Enabled: true, MaxRetries: 3, BaseDelayMs: 1}
	session, calls, events := createRetryEventsSession(t, retry, []llm.Message{
		retryAssistantError("overloaded_error"),
		retryAssistantText("recovered"),
	}, nil, nil)
	defer session.Dispose()

	if err := session.Prompt("test"); err != nil {
		t.Fatal(err)
	}
	if got := retryEventLabels(*events); !reflect.DeepEqual(got, []string{"start:1", "end:true"}) {
		t.Fatalf("retry events = %#v", got)
	}
	if *calls != 2 {
		t.Fatalf("calls = %d, want 2", *calls)
	}
	if session.IsRetrying() {
		t.Fatal("session should not be retrying")
	}
}

func TestAgentSessionRetryEventsPiParityMultipleFailuresRecover(t *testing.T) {
	retry := AgentSessionRetrySettings{Enabled: true, MaxRetries: 3, BaseDelayMs: 1}
	session, calls, events := createRetryEventsSession(t, retry, []llm.Message{
		retryAssistantError("overloaded_error"),
		retryAssistantError("overloaded_error"),
		retryAssistantText("success"),
	}, nil, nil)
	defer session.Dispose()

	if err := session.Prompt("test"); err != nil {
		t.Fatal(err)
	}
	if got := retryEventLabels(*events); !reflect.DeepEqual(got, []string{"start:1", "start:2", "end:true"}) {
		t.Fatalf("retry events = %#v", got)
	}
	if *calls != 3 {
		t.Fatalf("calls = %d, want 3", *calls)
	}
}

func TestAgentSessionRetryEventsPiParityExhaustsRetries(t *testing.T) {
	retry := AgentSessionRetrySettings{Enabled: true, MaxRetries: 2, BaseDelayMs: 1}
	session, calls, events := createRetryEventsSession(t, retry, []llm.Message{
		retryAssistantError("overloaded_error"),
		retryAssistantError("overloaded_error"),
		retryAssistantError("overloaded_error"),
	}, nil, nil)
	defer session.Dispose()

	if err := session.Prompt("test"); err != nil {
		t.Fatal(err)
	}
	if got := retryEventLabels(*events); !reflect.DeepEqual(got, []string{"start:1", "start:2", "end:false"}) {
		t.Fatalf("retry events = %#v", got)
	}
	if *calls != 3 {
		t.Fatalf("calls = %d, want 3", *calls)
	}
	if session.IsRetrying() {
		t.Fatal("session should not be retrying")
	}
}

func TestAgentSessionRetryEventsPiParityPromptWaitsForDelayedMessageEnd(t *testing.T) {
	retry := AgentSessionRetrySettings{Enabled: true, MaxRetries: 3, BaseDelayMs: 1}
	session, calls, _ := createRetryEventsSession(t, retry, []llm.Message{
		retryAssistantError("overloaded_error"),
		retryAssistantText("recovered"),
	}, nil, func(ctx *ProtocolExtensionContext) error {
		return ctx.On(ProtocolEventMessageEnd, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			if event.Message != nil && event.Message.Role == llm.RoleAssistant {
				time.Sleep(40 * time.Millisecond)
			}
			return ProtocolEventResult{}, nil
		})
	})
	defer session.Dispose()

	if err := session.Prompt("test"); err != nil {
		t.Fatal(err)
	}
	if *calls != 2 {
		t.Fatalf("calls = %d, want 2", *calls)
	}
	if session.IsRetrying() {
		t.Fatal("session should not be retrying")
	}
}

func TestAgentSessionRetryEventsPiParityDisabledRetry(t *testing.T) {
	retry := AgentSessionRetrySettings{Enabled: false, MaxRetries: 3, BaseDelayMs: 1}
	session, calls, events := createRetryEventsSession(t, retry, []llm.Message{
		retryAssistantError("overloaded_error"),
	}, nil, nil)
	defer session.Dispose()

	if err := session.Prompt("test"); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Fatalf("calls = %d, want 1", *calls)
	}
	if got := eventsOfType(*events, "auto_retry_start"); len(got) != 0 {
		t.Fatalf("retry starts = %#v", got)
	}
}

func TestAgentSessionRetryEventsPiParityNonRetryableError(t *testing.T) {
	retry := AgentSessionRetrySettings{Enabled: true, MaxRetries: 3, BaseDelayMs: 1}
	session, calls, events := createRetryEventsSession(t, retry, []llm.Message{
		retryAssistantError("invalid_api_key"),
	}, nil, nil)
	defer session.Dispose()

	if err := session.Prompt("test"); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Fatalf("calls = %d, want 1", *calls)
	}
	if got := eventsOfType(*events, "auto_retry_start"); len(got) != 0 {
		t.Fatalf("retry starts = %#v", got)
	}
}

func TestAgentSessionRetryEventsPiParityAbortRetryCancelsSleep(t *testing.T) {
	retry := AgentSessionRetrySettings{Enabled: true, MaxRetries: 3, BaseDelayMs: 100}
	session, calls, events := createRetryEventsSession(t, retry, []llm.Message{
		retryAssistantError("overloaded_error"),
	}, nil, nil)
	defer session.Dispose()

	sawRetryStart := make(chan struct{})
	var once sync.Once
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "auto_retry_start" {
			once.Do(func() { close(sawRetryStart) })
		}
	})
	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Prompt("test")
	}()
	<-sawRetryStart
	session.AbortRetry()
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if session.IsRetrying() {
		t.Fatal("session should not be retrying")
	}
	if got := retryFinalErrors(*events); !containsString(got, "Retry cancelled") {
		t.Fatalf("final errors = %#v", got)
	}
	if *calls != 1 {
		t.Fatalf("calls = %d, want 1", *calls)
	}
}

func TestAgentSessionRetryEventsPiParityRecoveryToolLoopCompletes(t *testing.T) {
	retry := AgentSessionRetrySettings{Enabled: true, MaxRetries: 3, BaseDelayMs: 1}
	toolRuns := []string{}
	echoTool := SDKTool{
		Name: "echo",
		Execute: func(_ string, params map[string]any) (SDKToolResult, error) {
			text, _ := params["text"].(string)
			toolRuns = append(toolRuns, text)
			return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "echo:" + text}}, Details: map[string]any{"text": text}}, nil
		},
	}
	toolMessage := retryAssistantText("")
	toolMessage.Content = []llm.ContentPart{llm.ToolCall("call_1", "echo", map[string]any{"text": "hello"})}
	toolMessage.StopReason = llm.StopReasonToolUse
	session, calls, _ := createRetryEventsSession(t, retry, []llm.Message{
		retryAssistantError("overloaded_error"),
		toolMessage,
		retryAssistantText("final answer"),
	}, []SDKTool{echoTool}, nil)
	defer session.Dispose()

	if err := session.Prompt("test"); err != nil {
		t.Fatal(err)
	}
	if *calls != 3 {
		t.Fatalf("calls = %d, want 3", *calls)
	}
	if !reflect.DeepEqual(toolRuns, []string{"hello"}) {
		t.Fatalf("tool runs = %#v", toolRuns)
	}
	if session.IsStreaming() {
		t.Fatal("session should not be streaming")
	}
	if err := session.Prompt("follow-up"); err != nil {
		t.Fatal(err)
	}
	if *calls != 4 {
		t.Fatalf("calls after follow-up = %d, want 4", *calls)
	}
}

func TestAgentSessionRetryEventsPiParityExtensionEventsBeforePublicSubscribers(t *testing.T) {
	order := []string{}
	session, _, _ := createRetryEventsSession(t, AgentSessionRetrySettings{}, []llm.Message{
		retryAssistantText("done"),
	}, nil, func(ctx *ProtocolExtensionContext) error {
		if err := ctx.On(ProtocolEventMessageStart, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			if event.Message != nil {
				order = append(order, "extension:"+event.Type+":"+event.Message.Role)
			}
			return ProtocolEventResult{}, nil
		}); err != nil {
			return err
		}
		return ctx.On(ProtocolEventMessageEnd, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			if event.Message != nil {
				order = append(order, "extension:"+event.Type+":"+event.Message.Role)
			}
			return ProtocolEventResult{}, nil
		})
	})
	defer session.Dispose()
	session.Subscribe(func(event AgentSessionEvent) {
		if (event.Type == "message_start" || event.Type == "message_end") && event.Message != nil {
			order = append(order, "public:"+event.Type+":"+event.Message.Role)
		}
	})

	if err := session.Prompt("hi"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"extension:message_start:user",
		"public:message_start:user",
		"extension:message_end:user",
		"public:message_end:user",
		"extension:message_start:assistant",
		"public:message_start:assistant",
		"extension:message_end:assistant",
		"public:message_end:assistant",
	}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
}

func TestAgentSessionRetryEventsPiParitySinglePromptEventOrder(t *testing.T) {
	session, _, events := createRetryEventsSession(t, AgentSessionRetrySettings{}, []llm.Message{
		retryAssistantText("hello"),
	}, nil, nil)
	defer session.Dispose()

	if err := session.Prompt("hi"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"agent_start",
		"turn_start",
		"message_start:user",
		"message_end:user",
		"message_start:assistant",
		"message_update",
		"message_end:assistant",
		"turn_end",
		"agent_end",
	}
	if got := normalizeRetryEventOrder(*events); !reflect.DeepEqual(got, want) {
		t.Fatalf("event order = %#v, want %#v", got, want)
	}
}

func TestAgentSessionRetryEventsPiParityToolCallEventOrder(t *testing.T) {
	toolRuns := []string{}
	echoTool := SDKTool{
		Name: "echo",
		Execute: func(_ string, params map[string]any) (SDKToolResult, error) {
			text, _ := params["text"].(string)
			toolRuns = append(toolRuns, text)
			return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "echo:" + text}}}, nil
		},
	}
	toolMessage := retryAssistantText("")
	toolMessage.Content = []llm.ContentPart{llm.ToolCall("call_1", "echo", map[string]any{"text": "hello"})}
	toolMessage.StopReason = llm.StopReasonToolUse
	session, _, events := createRetryEventsSession(t, AgentSessionRetrySettings{}, []llm.Message{
		toolMessage,
		retryAssistantText("done"),
	}, []SDKTool{echoTool}, nil)
	defer session.Dispose()

	if err := session.Prompt("hi"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(toolRuns, []string{"hello"}) {
		t.Fatalf("tool runs = %#v", toolRuns)
	}
	want := []string{
		"agent_start",
		"turn_start",
		"message_start:user",
		"message_end:user",
		"message_start:assistant",
		"message_update",
		"message_end:assistant",
		"tool_execution_start:echo",
		"tool_execution_end:echo",
		"message_start:toolResult",
		"message_end:toolResult",
		"turn_end",
		"turn_start",
		"message_start:assistant",
		"message_update",
		"message_end:assistant",
		"turn_end",
		"agent_end",
	}
	if got := normalizeRetryEventOrder(*events); !reflect.DeepEqual(got, want) {
		t.Fatalf("event order = %#v, want %#v", got, want)
	}
}

func TestAgentSessionRetryEventsPiParityMessageUpdateStreamingDeltas(t *testing.T) {
	echoTool := SDKTool{
		Name: "echo",
		Execute: func(_ string, params map[string]any) (SDKToolResult, error) {
			text, _ := params["text"].(string)
			return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "echo:" + text}}}, nil
		},
	}
	toolMessage := retryAssistantText("")
	toolMessage.Content = []llm.ContentPart{
		llm.Thinking("plan"),
		llm.Text("answer"),
		llm.ToolCall("call_1", "echo", map[string]any{"text": "hello"}),
	}
	toolMessage.StopReason = llm.StopReasonToolUse
	session, _, events := createRetryEventsSession(t, AgentSessionRetrySettings{}, []llm.Message{
		toolMessage,
		retryAssistantText("done"),
	}, []SDKTool{echoTool}, nil)
	defer session.Dispose()

	if err := session.Prompt("hi"); err != nil {
		t.Fatal(err)
	}
	updateTypes := []string{}
	for _, event := range eventsOfType(*events, "message_update") {
		if event.AssistantMessageEvent != nil {
			updateTypes = append(updateTypes, event.AssistantMessageEvent.Type)
		}
	}
	for _, want := range []string{"thinking_delta", "text_delta", "toolcall_delta"} {
		if !containsString(updateTypes, want) {
			t.Fatalf("update types = %#v, want %q", updateTypes, want)
		}
	}
}

func TestAgentSessionRetryEventsPiParityAgentEndForErrorResponses(t *testing.T) {
	session, _, events := createRetryEventsSession(t, AgentSessionRetrySettings{}, []llm.Message{
		retryAssistantError("broken"),
	}, nil, nil)
	defer session.Dispose()

	if err := session.Prompt("hi"); err != nil {
		t.Fatal(err)
	}
	if last := (*events)[len(*events)-1]; last.Type != "agent_end" {
		t.Fatalf("last event = %q, want agent_end", last.Type)
	}
}

func TestAgentSessionRetryEventsPiParityAgentEndForAbortedRuns(t *testing.T) {
	session, _, events := createRetryEventsSession(t, AgentSessionRetrySettings{}, []llm.Message{
		retryAssistantText("abort me"),
	}, nil, nil)
	defer session.Dispose()
	var once sync.Once
	session.Subscribe(func(event AgentSessionEvent) {
		if event.Type == "message_update" {
			once.Do(func() {
				_ = session.Abort()
			})
		}
	})

	if err := session.Prompt("hi"); err != nil {
		t.Fatal(err)
	}
	if last := (*events)[len(*events)-1]; last.Type != "agent_end" {
		t.Fatalf("last event = %q, want agent_end", last.Type)
	}
	messages := session.Messages()
	lastMessage := messages[len(messages)-1]
	if lastMessage.Role != llm.RoleAssistant || lastMessage.StopReason != llm.StopReasonAborted {
		t.Fatalf("last message = %#v", lastMessage)
	}
}

func createRetryEventsSession(t *testing.T, retry AgentSessionRetrySettings, responses []llm.Message, tools []SDKTool, factory func(*ProtocolExtensionContext) error) (*AgentSession, *int, *[]AgentSessionEvent) {
	t.Helper()
	calls := 0
	manager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            manager.GetCWD(),
		AgentDir:       t.TempDir(),
		Model:          llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
		SessionManager: manager,
		RetrySettings:  &retry,
		CustomTools:    tools,
		Responder: func(_ string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
			calls++
			if calls <= len(responses) {
				return responses[calls-1], nil
			}
			return retryAssistantText("done"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if factory != nil {
		runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "retry-events.gi.json", Factory: factory})
		runtime.BindSession(session)
	}
	events := []AgentSessionEvent{}
	session.Subscribe(func(event AgentSessionEvent) {
		events = append(events, event)
	})
	return session, &calls, &events
}

func retryEventLabels(events []AgentSessionEvent) []string {
	var labels []string
	for _, event := range events {
		switch event.Type {
		case "auto_retry_start":
			labels = append(labels, "start:"+strconv.Itoa(event.Attempt))
		case "auto_retry_end":
			labels = append(labels, "end:"+boolString(event.Success))
		}
	}
	return labels
}

func retryFinalErrors(events []AgentSessionEvent) []string {
	var errors []string
	for _, event := range events {
		if event.Type == "auto_retry_end" && event.FinalError != "" {
			errors = append(errors, event.FinalError)
		}
	}
	return errors
}

func eventsOfType(events []AgentSessionEvent, eventType string) []AgentSessionEvent {
	var result []AgentSessionEvent
	for _, event := range events {
		if event.Type == eventType {
			result = append(result, event)
		}
	}
	return result
}

func normalizeRetryEventOrder(events []AgentSessionEvent) []string {
	var normalized []string
	for _, event := range events {
		label := event.Type
		switch event.Type {
		case "message_start", "message_end":
			if event.Message != nil {
				label = event.Type + ":" + event.Message.Role
			}
		case "tool_execution_start", "tool_execution_end":
			label = event.Type + ":" + event.ToolName
		}
		if label == "message_update" && len(normalized) > 0 && normalized[len(normalized)-1] == "message_update" {
			continue
		}
		normalized = append(normalized, label)
	}
	return normalized
}
