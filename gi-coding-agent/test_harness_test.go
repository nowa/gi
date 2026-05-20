package gicodingagent

import (
	"reflect"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestCodingAgentTestHarnessSimpleTextResponse(t *testing.T) {
	harness := mustNewCodingAgentHarness(t, CodingAgentTestHarnessOptions{
		Responses: []CodingAgentTestHarnessResponse{TextHarnessResponse("hello world")},
	})
	defer harness.Cleanup()

	mustPrompt(t, harness.Session, "hi")

	if harness.Faux.CallCount != 1 {
		t.Fatalf("callCount = %d, want 1", harness.Faux.CallCount)
	}
	assistants := codingHarnessMessagesByRole(harness.Messages(), llm.RoleAssistant)
	if len(assistants) != 1 {
		t.Fatalf("assistant messages = %d, want 1", len(assistants))
	}
	if got := assistants[0].Content; !reflect.DeepEqual(got, []llm.ContentPart{llm.Text("hello world")}) {
		t.Fatalf("content = %#v", got)
	}
	if assistants[0].StopReason != llm.StopReasonStop {
		t.Fatalf("stopReason = %q", assistants[0].StopReason)
	}
}

func TestCodingAgentTestHarnessResponseSequence(t *testing.T) {
	harness := mustNewCodingAgentHarness(t, CodingAgentTestHarnessOptions{
		Responses: []CodingAgentTestHarnessResponse{
			TextHarnessResponse("first"),
			TextHarnessResponse("second"),
			TextHarnessResponse("third"),
		},
	})
	defer harness.Cleanup()

	mustPrompt(t, harness.Session, "a")
	mustPrompt(t, harness.Session, "b")
	mustPrompt(t, harness.Session, "c")

	if harness.Faux.CallCount != 3 {
		t.Fatalf("callCount = %d, want 3", harness.Faux.CallCount)
	}
	if got := codingHarnessAssistantTexts(harness.Messages()); !reflect.DeepEqual(got, []string{"first", "second", "third"}) {
		t.Fatalf("assistant texts = %#v", got)
	}
}

func TestCodingAgentTestHarnessToolCallTriggersToolExecution(t *testing.T) {
	toolExecuted := false
	echoTool := SDKTool{
		Name: "echo",
		Execute: func(string, map[string]any) (SDKToolResult, error) {
			toolExecuted = true
			return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "echoed"}}}, nil
		},
	}
	harness := mustNewCodingAgentHarness(t, CodingAgentTestHarnessOptions{
		Responses: []CodingAgentTestHarnessResponse{
			{ToolCalls: []CodingAgentTestHarnessToolCall{{Name: "echo", Arguments: map[string]any{"text": "hi"}}}},
			TextHarnessResponse("done after tool"),
		},
		Tools: []SDKTool{echoTool},
	})
	defer harness.Cleanup()

	mustPrompt(t, harness.Session, "use the tool")

	if !toolExecuted {
		t.Fatal("tool should be executed")
	}
	if harness.Faux.CallCount != 2 {
		t.Fatalf("callCount = %d, want 2", harness.Faux.CallCount)
	}
	if results := codingHarnessMessagesByRole(harness.Messages(), llm.RoleToolResult); len(results) != 1 {
		t.Fatalf("tool results = %d, want 1", len(results))
	}
}

func TestCodingAgentTestHarnessErrorResponse(t *testing.T) {
	harness := mustNewCodingAgentHarness(t, CodingAgentTestHarnessOptions{
		Responses: []CodingAgentTestHarnessResponse{{Error: "something broke"}},
	})
	defer harness.Cleanup()

	mustPrompt(t, harness.Session, "hi")

	assistants := codingHarnessMessagesByRole(harness.Messages(), llm.RoleAssistant)
	if len(assistants) != 1 {
		t.Fatalf("assistant messages = %d, want 1", len(assistants))
	}
	if assistants[0].StopReason != llm.StopReasonError || assistants[0].ErrorMessage != "something broke" {
		t.Fatalf("assistant = %#v", assistants[0])
	}
}

func TestCodingAgentTestHarnessRetriesTransientError(t *testing.T) {
	retry := AgentSessionRetrySettings{Enabled: true, MaxRetries: 3, BaseDelayMs: 1}
	harness := mustNewCodingAgentHarness(t, CodingAgentTestHarnessOptions{
		Responses: []CodingAgentTestHarnessResponse{
			{Error: "overloaded_error"},
			TextHarnessResponse("recovered"),
		},
		RetrySettings: &retry,
	})
	defer harness.Cleanup()

	mustPrompt(t, harness.Session, "hi")

	if harness.Faux.CallCount != 2 {
		t.Fatalf("callCount = %d, want 2", harness.Faux.CallCount)
	}
	if starts := harness.EventsOfType("auto_retry_start"); len(starts) != 1 {
		t.Fatalf("retry starts = %d, want 1", len(starts))
	}
	ends := harness.EventsOfType("auto_retry_end")
	if len(ends) != 1 || !ends[0].Success {
		t.Fatalf("retry ends = %#v", ends)
	}
}

func TestCodingAgentTestHarnessCustomUsageNumbers(t *testing.T) {
	usage := llm.Usage{Input: 100000, Output: 5000}
	harness := mustNewCodingAgentHarness(t, CodingAgentTestHarnessOptions{
		Responses: []CodingAgentTestHarnessResponse{{Text: interactiveStringPtr("big response"), Usage: &usage}},
	})
	defer harness.Cleanup()

	mustPrompt(t, harness.Session, "hi")

	assistants := codingHarnessMessagesByRole(harness.Messages(), llm.RoleAssistant)
	if assistants[0].Usage.Input != 100000 || assistants[0].Usage.Output != 5000 {
		t.Fatalf("usage = %#v", assistants[0].Usage)
	}
}

func TestCodingAgentTestHarnessEventCapture(t *testing.T) {
	harness := mustNewCodingAgentHarness(t, CodingAgentTestHarnessOptions{
		Responses: []CodingAgentTestHarnessResponse{TextHarnessResponse("hello")},
	})
	defer harness.Cleanup()

	mustPrompt(t, harness.Session, "hi")

	if starts := harness.EventsOfType("agent_start"); len(starts) != 1 {
		t.Fatalf("agent starts = %d, want 1", len(starts))
	}
	if ends := harness.EventsOfType("agent_end"); len(ends) != 1 {
		t.Fatalf("agent ends = %d, want 1", len(ends))
	}
	if messageEnds := harness.EventsOfType("message_end"); len(messageEnds) < 2 {
		t.Fatalf("message ends = %d, want >= 2", len(messageEnds))
	}
}

func TestCodingAgentTestHarnessContextCapture(t *testing.T) {
	harness := mustNewCodingAgentHarness(t, CodingAgentTestHarnessOptions{
		Responses: []CodingAgentTestHarnessResponse{TextHarnessResponse("reply")},
	})
	defer harness.Cleanup()

	mustPrompt(t, harness.Session, "my question")

	if len(harness.Faux.Contexts) != 1 {
		t.Fatalf("contexts = %d, want 1", len(harness.Faux.Contexts))
	}
	if !codingHarnessHasUserText(harness.Faux.Contexts[0], "my question") {
		t.Fatalf("context missing user message: %#v", harness.Faux.Contexts[0])
	}
}

func TestCodingAgentTestHarnessWrapsResponses(t *testing.T) {
	harness := mustNewCodingAgentHarness(t, CodingAgentTestHarnessOptions{
		Responses: []CodingAgentTestHarnessResponse{
			TextHarnessResponse("a"),
			TextHarnessResponse("b"),
		},
	})
	defer harness.Cleanup()

	mustPrompt(t, harness.Session, "1")
	mustPrompt(t, harness.Session, "2")
	mustPrompt(t, harness.Session, "3")

	if harness.Faux.CallCount != 3 {
		t.Fatalf("callCount = %d, want 3", harness.Faux.CallCount)
	}
	if got := codingHarnessAssistantTexts(harness.Messages()); !reflect.DeepEqual(got, []string{"a", "b", "a"}) {
		t.Fatalf("assistant texts = %#v", got)
	}
}

func TestCodingAgentTestHarnessStreamsTextDeltas(t *testing.T) {
	harness := mustNewCodingAgentHarness(t, CodingAgentTestHarnessOptions{
		Responses: []CodingAgentTestHarnessResponse{TextHarnessResponse("hello world")},
	})
	defer harness.Cleanup()

	mustPrompt(t, harness.Session, "hi")

	if got := codingHarnessDeltas(harness.EventsOfType("message_update"), "text_delta"); got != "hello world" {
		t.Fatalf("text deltas = %q", got)
	}
}

func TestCodingAgentTestHarnessStreamsThinkingDeltas(t *testing.T) {
	harness := mustNewCodingAgentHarness(t, CodingAgentTestHarnessOptions{
		Responses: []CodingAgentTestHarnessResponse{{Thinking: "let me think about this", Text: interactiveStringPtr("answer")}},
	})
	defer harness.Cleanup()

	mustPrompt(t, harness.Session, "hi")

	updates := harness.EventsOfType("message_update")
	if count := codingHarnessUpdateCount(updates, "thinking_start"); count != 1 {
		t.Fatalf("thinking_start count = %d, want 1", count)
	}
	if count := codingHarnessUpdateCount(updates, "thinking_end"); count != 1 {
		t.Fatalf("thinking_end count = %d, want 1", count)
	}
	if got := codingHarnessDeltas(updates, "thinking_delta"); got != "let me think about this" {
		t.Fatalf("thinking deltas = %q", got)
	}
}

func TestCodingAgentTestHarnessStreamsToolCallDeltas(t *testing.T) {
	echoTool := SDKTool{
		Name: "echo",
		Execute: func(string, map[string]any) (SDKToolResult, error) {
			return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "echoed"}}}, nil
		},
	}
	harness := mustNewCodingAgentHarness(t, CodingAgentTestHarnessOptions{
		Responses: []CodingAgentTestHarnessResponse{
			{ToolCalls: []CodingAgentTestHarnessToolCall{{Name: "echo", Arguments: map[string]any{"text": "hi"}}}},
			TextHarnessResponse("done"),
		},
		Tools: []SDKTool{echoTool},
	})
	defer harness.Cleanup()

	mustPrompt(t, harness.Session, "use tool")

	updates := harness.EventsOfType("message_update")
	if count := codingHarnessUpdateCount(updates, "toolcall_start"); count != 1 {
		t.Fatalf("toolcall_start count = %d, want 1", count)
	}
	if count := codingHarnessUpdateCount(updates, "toolcall_end"); count != 1 {
		t.Fatalf("toolcall_end count = %d, want 1", count)
	}
	if got := codingHarnessDeltas(updates, "toolcall_delta"); got == "" {
		t.Fatal("toolcall_delta should be emitted")
	}
}

func TestCodingAgentTestHarnessStreamsThinkingTextToolCallInOrder(t *testing.T) {
	echoTool := SDKTool{
		Name: "echo",
		Execute: func(string, map[string]any) (SDKToolResult, error) {
			return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "echoed"}}}, nil
		},
	}
	harness := mustNewCodingAgentHarness(t, CodingAgentTestHarnessOptions{
		Responses: []CodingAgentTestHarnessResponse{
			{
				Thinking:  "hmm",
				Text:      interactiveStringPtr("I will call a tool"),
				ToolCalls: []CodingAgentTestHarnessToolCall{{Name: "echo", Arguments: map[string]any{"text": "x"}}},
			},
			TextHarnessResponse("final"),
		},
		Tools: []SDKTool{echoTool},
	})
	defer harness.Cleanup()

	mustPrompt(t, harness.Session, "do it")

	types := codingHarnessUpdateTypes(harness.EventsOfType("message_update"))
	firstThinking := codingHarnessIndex(types, "thinking_start")
	firstText := codingHarnessIndex(types, "text_start")
	firstToolCall := codingHarnessIndex(types, "toolcall_start")
	if firstThinking < 0 || firstText < 0 || firstToolCall < 0 {
		t.Fatalf("missing stream types: %#v", types)
	}
	if !(firstThinking < firstText && firstText < firstToolCall) {
		t.Fatalf("stream type order = %#v", types)
	}
}

func TestCodingAgentTestHarnessSessionPersistence(t *testing.T) {
	harness := mustNewCodingAgentHarness(t, CodingAgentTestHarnessOptions{
		Responses: []CodingAgentTestHarnessResponse{TextHarnessResponse("persisted")},
	})
	defer harness.Cleanup()

	mustPrompt(t, harness.Session, "hi")

	messageEntries := filterFileEntriesByType(harness.SessionManager.GetEntries(), "message")
	if len(messageEntries) < 2 {
		t.Fatalf("message entries = %d, want >= 2", len(messageEntries))
	}
}

func mustNewCodingAgentHarness(t *testing.T, options CodingAgentTestHarnessOptions) *CodingAgentTestHarness {
	t.Helper()
	harness, err := NewCodingAgentTestHarness(options)
	if err != nil {
		t.Fatal(err)
	}
	return harness
}

func codingHarnessMessagesByRole(messages []llm.Message, role string) []llm.Message {
	filtered := []llm.Message{}
	for _, message := range messages {
		if message.Role == role {
			filtered = append(filtered, message)
		}
	}
	return filtered
}

func codingHarnessAssistantTexts(messages []llm.Message) []string {
	texts := []string{}
	for _, message := range codingHarnessMessagesByRole(messages, llm.RoleAssistant) {
		for _, part := range message.Content {
			if part.Type == llm.ContentText {
				texts = append(texts, part.Text)
				break
			}
		}
	}
	return texts
}

func codingHarnessHasUserText(messages []llm.Message, text string) bool {
	for _, message := range messages {
		if message.Role != llm.RoleUser {
			continue
		}
		for _, part := range message.Content {
			if part.Type == llm.ContentText && part.Text == text {
				return true
			}
		}
	}
	return false
}

func codingHarnessDeltas(events []AgentSessionEvent, eventType string) string {
	result := ""
	for _, event := range events {
		if event.AssistantMessageEvent != nil && event.AssistantMessageEvent.Type == eventType {
			result += event.AssistantMessageEvent.Delta
		}
	}
	return result
}

func codingHarnessUpdateCount(events []AgentSessionEvent, eventType string) int {
	count := 0
	for _, event := range events {
		if event.AssistantMessageEvent != nil && event.AssistantMessageEvent.Type == eventType {
			count++
		}
	}
	return count
}

func codingHarnessUpdateTypes(events []AgentSessionEvent) []string {
	types := []string{}
	for _, event := range events {
		if event.AssistantMessageEvent != nil {
			types = append(types, event.AssistantMessageEvent.Type)
		}
	}
	return types
}

func codingHarnessIndex(values []string, needle string) int {
	for index, value := range values {
		if value == needle {
			return index
		}
	}
	return -1
}
