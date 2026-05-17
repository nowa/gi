package harness

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	core "github.com/nowa/gi/gi-agent-core"
	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentHarnessConstructsAndExposesQueueModes(t *testing.T) {
	session := NewSession(MustInMemorySessionStorage())
	model := llm.Model{ID: "model-1", API: "api", Provider: "provider"}
	harness := MustNewAgentHarness(AgentHarnessOptions{
		Session:       session,
		Model:         model,
		ThinkingLevel: "high",
		SystemPrompt:  "You are helpful.",
		SteeringMode:  core.QueueAll,
		FollowUpMode:  core.QueueAll,
	})

	if harness.GetModel().ID != "model-1" || harness.GetThinkingLevel() != "high" {
		t.Fatalf("model/thinking = %#v / %s", harness.GetModel(), harness.GetThinkingLevel())
	}
	if harness.GetSteeringMode() != core.QueueAll || harness.GetFollowUpMode() != core.QueueAll {
		t.Fatalf("queue modes = %s / %s", harness.GetSteeringMode(), harness.GetFollowUpMode())
	}
	harness.SetSteeringMode(core.QueueOneAtTime)
	harness.SetFollowUpMode(core.QueueOneAtTime)
	if harness.GetSteeringMode() != core.QueueOneAtTime || harness.GetFollowUpMode() != core.QueueOneAtTime {
		t.Fatalf("updated queue modes = %s / %s", harness.GetSteeringMode(), harness.GetFollowUpMode())
	}
}

func TestAgentHarnessStreamOptionsHooksAndPayloadHooks(t *testing.T) {
	ctx := context.Background()
	registration := llm.RegisterFauxProvider()
	defer registration.Unregister()

	var capturedOptions llm.StreamOptions
	var finalPayload any
	registration.SetResponses([]llm.FauxResponseStep{{
		Factory: func(_ llm.Context, options llm.StreamOptions, _ llm.FauxState, model llm.Model) (llm.Message, error) {
			capturedOptions = options
			payload, changed, err := options.OnPayload(map[string]any{"steps": []string{"provider"}}, model)
			if err != nil {
				return llm.Message{}, err
			}
			if changed {
				finalPayload = payload
			}
			return llm.FauxAssistantText("ok"), nil
		},
	}})

	storage, err := NewInMemorySessionStorage(&SessionMetadata{ID: "session-1", CreatedAt: "now"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	harness := MustNewAgentHarness(AgentHarnessOptions{
		Session: NewSession(storage),
		Model:   registration.MustModel(),
		StreamOptions: AgentHarnessStreamOptions{
			TimeoutMillis:   1000,
			MaxRetries:      2,
			MaxRetryDelayMs: 3000,
			Headers:         map[string]string{"x-base": "base", "remove": "base"},
			Metadata:        map[string]any{"base": true, "remove": true},
			CacheRetention:  "none",
		},
		GetAPIKeyAndHeaders: func(context.Context, llm.Model) (AgentHarnessAuth, bool, error) {
			return AgentHarnessAuth{APIKey: "secret", Headers: map[string]string{"x-auth": "auth"}}, true, nil
		},
	})

	harness.On("before_provider_request", func(_ context.Context, event AgentHarnessEvent) (*AgentHarnessHookResult, error) {
		if !reflect.DeepEqual(event.StreamOptions.Headers, map[string]string{"x-base": "base", "remove": "base", "x-auth": "auth"}) {
			t.Fatalf("first hook headers = %#v", event.StreamOptions.Headers)
		}
		return &AgentHarnessHookResult{StreamOptions: &AgentHarnessStreamOptionsPatch{
			Headers:  map[string]*string{"x-hook": testStringPtr("hook"), "remove": nil},
			Metadata: map[string]Optional[any]{"hook": Some[any](true), "remove": {}},
		}}, nil
	})
	harness.On("before_provider_payload", func(_ context.Context, event AgentHarnessEvent) (*AgentHarnessHookResult, error) {
		return &AgentHarnessHookResult{Payload: map[string]any{"steps": []string{"provider", "first"}}, HasPayload: true}, nil
	})
	harness.On("before_provider_payload", func(_ context.Context, event AgentHarnessEvent) (*AgentHarnessHookResult, error) {
		want := map[string]any{"steps": []string{"provider", "first"}}
		if !reflect.DeepEqual(event.Payload, want) {
			t.Fatalf("second payload hook saw %#v", event.Payload)
		}
		return &AgentHarnessHookResult{Payload: map[string]any{"steps": []string{"provider", "first", "second"}}, HasPayload: true}, nil
	})

	if _, err := harness.Prompt(ctx, "hello"); err != nil {
		t.Fatal(err)
	}

	if capturedOptions.APIKey != "secret" || capturedOptions.SessionID != "session-1" || capturedOptions.TimeoutMillis != 1000 || capturedOptions.MaxRetries != 2 || capturedOptions.MaxRetryDelayMs != 3000 || capturedOptions.CacheRetention != "none" {
		t.Fatalf("captured options = %#v", capturedOptions)
	}
	if !reflect.DeepEqual(capturedOptions.Headers, map[string]string{"x-base": "base", "x-auth": "auth", "x-hook": "hook"}) {
		t.Fatalf("captured headers = %#v", capturedOptions.Headers)
	}
	if !reflect.DeepEqual(capturedOptions.Metadata, map[string]any{"base": true, "hook": true}) {
		t.Fatalf("captured metadata = %#v", capturedOptions.Metadata)
	}
	if !reflect.DeepEqual(finalPayload, map[string]any{"steps": []string{"provider", "first", "second"}}) {
		t.Fatalf("final payload = %#v", finalPayload)
	}
}

func TestAgentHarnessDrainsQueuedMessagesOneAtATime(t *testing.T) {
	ctx := context.Background()
	registration := llm.RegisterFauxProvider()
	defer registration.Unregister()

	var userCounts []int
	registration.SetResponses([]llm.FauxResponseStep{
		{Factory: func(context llm.Context, _ llm.StreamOptions, _ llm.FauxState, _ llm.Model) (llm.Message, error) {
			userCounts = append(userCounts, countUserMessages(context.Messages))
			return llm.FauxAssistantText("first"), nil
		}},
		{Factory: func(context llm.Context, _ llm.StreamOptions, _ llm.FauxState, _ llm.Model) (llm.Message, error) {
			userCounts = append(userCounts, countUserMessages(context.Messages))
			return llm.FauxAssistantText("second"), nil
		}},
		{Factory: func(context llm.Context, _ llm.StreamOptions, _ llm.FauxState, _ llm.Model) (llm.Message, error) {
			userCounts = append(userCounts, countUserMessages(context.Messages))
			return llm.FauxAssistantText("third"), nil
		}},
	})

	harness := MustNewAgentHarness(AgentHarnessOptions{
		Session:      NewSession(MustInMemorySessionStorage()),
		Model:        registration.MustModel(),
		SteeringMode: core.QueueOneAtTime,
	})
	var queueLengths []int
	queued := false
	harness.Subscribe(func(ctx context.Context, event AgentHarnessEvent) error {
		if event.Type == "queue_update" {
			queueLengths = append(queueLengths, len(event.Steer))
		}
		if event.Type == "message_start" && event.Message.Role == llm.RoleAssistant && !queued {
			queued = true
			if err := harness.Steer(ctx, "one"); err != nil {
				return err
			}
			return harness.Steer(ctx, "two")
		}
		return nil
	})

	if _, err := harness.Prompt(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(userCounts, []int{1, 2, 3}) {
		t.Fatalf("user counts = %#v", userCounts)
	}
	if !reflect.DeepEqual(queueLengths, []int{1, 2, 1, 0}) {
		t.Fatalf("queue lengths = %#v", queueLengths)
	}
}

func TestAgentHarnessAbortClearsSteerAndFollowUpButPreservesNextTurn(t *testing.T) {
	ctx := context.Background()
	registration := llm.RegisterFauxProvider()
	defer registration.Unregister()

	firstStarted := make(chan context.Context, 1)
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseFirst) })
	}
	defer release()

	var requestMu sync.Mutex
	var secondRequestText []string
	registration.SetResponses([]llm.FauxResponseStep{
		{Factory: func(_ llm.Context, options llm.StreamOptions, _ llm.FauxState, _ llm.Model) (llm.Message, error) {
			firstStarted <- options.Context
			<-releaseFirst
			return llm.FauxAssistantText("first finished"), nil
		}},
		{Factory: func(context llm.Context, _ llm.StreamOptions, _ llm.FauxState, _ llm.Model) (llm.Message, error) {
			requestMu.Lock()
			secondRequestText = textFromUserMessages(context.Messages)
			requestMu.Unlock()
			return llm.FauxAssistantText("second finished"), nil
		}},
	})

	harness := MustNewAgentHarness(AgentHarnessOptions{
		Session: NewSession(MustInMemorySessionStorage()),
		Model:   registration.MustModel(),
	})

	type queueLengths struct {
		steer    int
		followUp int
		nextTurn int
	}
	var queueMu sync.Mutex
	var queueUpdates []queueLengths
	harness.Subscribe(func(_ context.Context, event AgentHarnessEvent) error {
		if event.Type != "queue_update" {
			return nil
		}
		queueMu.Lock()
		queueUpdates = append(queueUpdates, queueLengths{
			steer:    len(event.Steer),
			followUp: len(event.FollowUp),
			nextTurn: len(event.NextTurn),
		})
		queueMu.Unlock()
		return nil
	})

	type promptResult struct {
		message llm.Message
		err     error
	}
	promptDone := make(chan promptResult, 1)
	go func() {
		message, err := harness.Prompt(ctx, "first")
		promptDone <- promptResult{message: message, err: err}
	}()

	var runCtx context.Context
	select {
	case runCtx = <-firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first provider request")
	}

	if err := harness.Steer(ctx, "steer"); err != nil {
		t.Fatal(err)
	}
	if err := harness.FollowUp(ctx, "follow"); err != nil {
		t.Fatal(err)
	}
	if err := harness.NextTurn(ctx, "next"); err != nil {
		t.Fatal(err)
	}

	type abortResult struct {
		clearedSteer    []llm.Message
		clearedFollowUp []llm.Message
		err             error
	}
	abortDone := make(chan abortResult, 1)
	go func() {
		clearedSteer, clearedFollowUp, err := harness.Abort(ctx)
		abortDone <- abortResult{clearedSteer: clearedSteer, clearedFollowUp: clearedFollowUp, err: err}
	}()

	select {
	case <-runCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for abort to cancel the active request context")
	}

	release()

	var abort abortResult
	select {
	case abort = <-abortDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for abort to settle")
	}
	if abort.err != nil {
		t.Fatal(abort.err)
	}
	if !reflect.DeepEqual(textFromUserMessages(abort.clearedSteer), []string{"steer"}) {
		t.Fatalf("cleared steer = %#v", textFromUserMessages(abort.clearedSteer))
	}
	if !reflect.DeepEqual(textFromUserMessages(abort.clearedFollowUp), []string{"follow"}) {
		t.Fatalf("cleared follow-up = %#v", textFromUserMessages(abort.clearedFollowUp))
	}

	select {
	case result := <-promptDone:
		if result.err != nil {
			t.Fatal(result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first prompt to finish")
	}

	if _, err := harness.Prompt(ctx, "second"); err != nil {
		t.Fatal(err)
	}

	requestMu.Lock()
	gotRequestText := append([]string{}, secondRequestText...)
	requestMu.Unlock()
	if !reflect.DeepEqual(gotRequestText, []string{"first", "next", "second"}) {
		t.Fatalf("second request text = %#v", gotRequestText)
	}

	queueMu.Lock()
	gotQueueUpdates := append([]queueLengths{}, queueUpdates...)
	queueMu.Unlock()
	sawAbortQueueState := false
	for _, update := range gotQueueUpdates {
		if update == (queueLengths{steer: 0, followUp: 0, nextTurn: 1}) {
			sawAbortQueueState = true
			break
		}
	}
	if !sawAbortQueueState {
		t.Fatalf("queue updates did not preserve next-turn during abort: %#v", gotQueueUpdates)
	}
}

func TestAgentHarnessDrainsFollowUpMessagesOneAtATimeAfterStop(t *testing.T) {
	ctx := context.Background()
	registration := llm.RegisterFauxProvider()
	defer registration.Unregister()

	var userCounts []int
	registration.SetResponses([]llm.FauxResponseStep{
		{Factory: func(context llm.Context, _ llm.StreamOptions, _ llm.FauxState, _ llm.Model) (llm.Message, error) {
			userCounts = append(userCounts, countUserMessages(context.Messages))
			return llm.FauxAssistantText("first"), nil
		}},
		{Factory: func(context llm.Context, _ llm.StreamOptions, _ llm.FauxState, _ llm.Model) (llm.Message, error) {
			userCounts = append(userCounts, countUserMessages(context.Messages))
			return llm.FauxAssistantText("second"), nil
		}},
		{Factory: func(context llm.Context, _ llm.StreamOptions, _ llm.FauxState, _ llm.Model) (llm.Message, error) {
			userCounts = append(userCounts, countUserMessages(context.Messages))
			return llm.FauxAssistantText("third"), nil
		}},
	})

	harness := MustNewAgentHarness(AgentHarnessOptions{
		Session:      NewSession(MustInMemorySessionStorage()),
		Model:        registration.MustModel(),
		FollowUpMode: core.QueueOneAtTime,
	})
	var queueLengths []int
	queued := false
	harness.Subscribe(func(ctx context.Context, event AgentHarnessEvent) error {
		if event.Type == "queue_update" {
			queueLengths = append(queueLengths, len(event.FollowUp))
		}
		if event.Type == "message_start" && event.Message.Role == llm.RoleAssistant && !queued {
			queued = true
			if err := harness.FollowUp(ctx, "one"); err != nil {
				return err
			}
			return harness.FollowUp(ctx, "two")
		}
		return nil
	})

	if _, err := harness.Prompt(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(userCounts, []int{1, 2, 3}) {
		t.Fatalf("user counts = %#v", userCounts)
	}
	if !reflect.DeepEqual(queueLengths, []int{1, 2, 1, 0}) {
		t.Fatalf("queue lengths = %#v", queueLengths)
	}
}

func TestAgentHarnessBeforeAgentStartMessagesArePersisted(t *testing.T) {
	ctx := context.Background()
	registration := llm.RegisterFauxProvider()
	defer registration.Unregister()

	var requestText []string
	registration.SetResponses([]llm.FauxResponseStep{{
		Factory: func(context llm.Context, _ llm.StreamOptions, _ llm.FauxState, _ llm.Model) (llm.Message, error) {
			requestText = textFromUserMessages(context.Messages)
			return llm.FauxAssistantText("ok"), nil
		},
	}})

	session := NewSession(MustInMemorySessionStorage())
	harness := MustNewAgentHarness(AgentHarnessOptions{Session: session, Model: registration.MustModel()})
	harness.On("before_agent_start", func(context.Context, AgentHarnessEvent) (*AgentHarnessHookResult, error) {
		return &AgentHarnessHookResult{Messages: []llm.Message{llm.UserMessageText("hook")}, HasMessages: true}, nil
	})

	if _, err := harness.Prompt(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(requestText, []string{"hello", "hook"}) {
		t.Fatalf("request text = %#v", requestText)
	}
	if got := textFromSessionUserMessages(session); !reflect.DeepEqual(got, []string{"hello", "hook"}) {
		t.Fatalf("persisted text = %#v", got)
	}
}

func TestAgentHarnessRefreshesRuntimeStateAtSavePoints(t *testing.T) {
	ctx := context.Background()
	registration := llm.RegisterFauxProvider(llm.WithFauxModels(
		llm.FauxModelDefinition{ID: "first", Reasoning: true},
		llm.FauxModelDefinition{ID: "second", Reasoning: true},
	))
	defer registration.Unregister()
	secondModel := registration.MustModel("second")

	var captured []struct {
		modelID      string
		reasoning    string
		systemPrompt string
		tools        []string
		timeout      int
	}
	registration.SetResponses([]llm.FauxResponseStep{
		{Factory: func(context llm.Context, options llm.StreamOptions, _ llm.FauxState, model llm.Model) (llm.Message, error) {
			captured = append(captured, struct {
				modelID      string
				reasoning    string
				systemPrompt string
				tools        []string
				timeout      int
			}{model.ID, options.Reasoning, context.SystemPrompt, toolNames(context.Tools), options.TimeoutMillis})
			return llm.FauxAssistantMessage([]llm.ContentPart{llm.FauxToolCall("calculate", map[string]any{"expression": "1 + 1"}, "call-1")}, "toolUse"), nil
		}},
		{Factory: func(context llm.Context, options llm.StreamOptions, _ llm.FauxState, model llm.Model) (llm.Message, error) {
			captured = append(captured, struct {
				modelID      string
				reasoning    string
				systemPrompt string
				tools        []string
				timeout      int
			}{model.ID, options.Reasoning, context.SystemPrompt, toolNames(context.Tools), options.TimeoutMillis})
			return llm.FauxAssistantText("done"), nil
		}},
	})

	harness := MustNewAgentHarness(AgentHarnessOptions{
		Session:       NewSession(MustInMemorySessionStorage()),
		Model:         registration.MustModel("first"),
		ThinkingLevel: "off",
		Resources: AgentHarnessResources{Skills: []Skill{
			{Name: "prompt", Description: "prompt", Content: "first prompt", FilePath: "/skills/prompt/SKILL.md"},
		}},
		BuildSystemPrompt: func(_ context.Context, context SystemPromptContext) (string, error) {
			if len(context.Resources.Skills) == 0 {
				return "missing prompt", nil
			}
			return context.Resources.Skills[0].Content, nil
		},
		Tools:         []core.AgentTool{calculateTool()},
		StreamOptions: AgentHarnessStreamOptions{TimeoutMillis: 1000},
	})
	harness.Subscribe(func(ctx context.Context, event AgentHarnessEvent) error {
		if event.Type != "tool_execution_start" {
			return nil
		}
		if err := harness.SetModel(ctx, secondModel); err != nil {
			return err
		}
		if err := harness.SetThinkingLevel(ctx, "high"); err != nil {
			return err
		}
		if err := harness.SetResources(ctx, AgentHarnessResources{Skills: []Skill{
			{Name: "prompt", Description: "prompt", Content: "second prompt", FilePath: "/skills/prompt/SKILL.md"},
		}}); err != nil {
			return err
		}
		harness.SetStreamOptions(AgentHarnessStreamOptions{TimeoutMillis: 2000})
		return harness.SetTools([]core.AgentTool{calculateTool(), timeTool()}, "time")
	})

	if _, err := harness.Prompt(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		modelID      string
		reasoning    string
		systemPrompt string
		tools        []string
		timeout      int
	}{
		{"first", "", "first prompt", []string{"calculate"}, 1000},
		{"second", "high", "second prompt", []string{"time"}, 2000},
	}
	if !reflect.DeepEqual(captured, want) {
		t.Fatalf("captured = %#v", captured)
	}
}

func TestAgentHarnessHookFailurePersistsAssistantError(t *testing.T) {
	ctx := context.Background()
	registration := llm.RegisterFauxProvider()
	defer registration.Unregister()

	session := NewSession(MustInMemorySessionStorage())
	harness := MustNewAgentHarness(AgentHarnessOptions{Session: session, Model: registration.MustModel()})
	harness.On("context", func(context.Context, AgentHarnessEvent) (*AgentHarnessHookResult, error) {
		return nil, errString("context exploded")
	})

	response, err := harness.Prompt(ctx, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if response.StopReason != llm.StopReasonError || response.ErrorMessage != "context exploded" {
		t.Fatalf("response = %#v", response)
	}
	messages := sessionMessages(session)
	if len(messages) != 2 || messages[0].Role != llm.RoleUser || messages[1].StopReason != llm.StopReasonError {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestAgentHarnessCompactAppendsCompactionEntry(t *testing.T) {
	ctx := context.Background()
	registration := llm.RegisterFauxProvider()
	defer registration.Unregister()

	var summaryPrompts []string
	registration.SetResponses([]llm.FauxResponseStep{
		{Factory: func(context llm.Context, _ llm.StreamOptions, _ llm.FauxState, _ llm.Model) (llm.Message, error) {
			summaryPrompts = append(summaryPrompts, textFromUserMessages(context.Messages)...)
			return llm.FauxAssistantText("history summary"), nil
		}},
		{Factory: func(context llm.Context, _ llm.StreamOptions, _ llm.FauxState, _ llm.Model) (llm.Message, error) {
			summaryPrompts = append(summaryPrompts, textFromUserMessages(context.Messages)...)
			return llm.FauxAssistantText("prefix summary"), nil
		}},
	})

	session := NewSession(MustInMemorySessionStorage())
	_, _ = session.AppendMessage(llm.UserMessageText("one"))
	firstAssistant := harnessAssistantMessage("two")
	firstAssistant.Usage = mockUsage(50_000, 1_000, 0, 0)
	_, _ = session.AppendMessage(firstAssistant)
	_, _ = session.AppendMessage(llm.UserMessageText("three"))
	secondAssistant := harnessAssistantMessage("four")
	secondAssistant.Usage = mockUsage(50_000, 1_000, 0, 0)
	_, _ = session.AppendMessage(secondAssistant)

	harness := MustNewAgentHarness(AgentHarnessOptions{
		Session: session,
		Model:   registration.MustModel(),
		GetAPIKeyAndHeaders: func(context.Context, llm.Model) (AgentHarnessAuth, bool, error) {
			return AgentHarnessAuth{APIKey: "test-key"}, true, nil
		},
	})
	var compactEvent Entry
	harness.Subscribe(func(_ context.Context, event AgentHarnessEvent) error {
		if event.Type == "session_compact" {
			compactEvent = event.CompactionEntry
		}
		return nil
	})

	result, err := harness.Compact(ctx, "focus")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Summary, "history summary") || !strings.Contains(result.Summary, "prefix summary") {
		t.Fatalf("summary = %q", result.Summary)
	}
	if len(summaryPrompts) != 2 || !strings.Contains(summaryPrompts[0], "Additional focus: focus") || !strings.Contains(summaryPrompts[1], "prefix of a turn") {
		t.Fatalf("summary prompts = %#v", summaryPrompts)
	}
	entries := session.Entries()
	last := entries[len(entries)-1]
	if last.Type != "compaction" || last.Summary != result.Summary || last.FirstKeptEntryID != result.FirstKeptEntryID {
		t.Fatalf("last entry = %#v result = %#v", last, result)
	}
	if compactEvent.ID != last.ID {
		t.Fatalf("compact event = %#v, last = %#v", compactEvent, last)
	}
}

func TestAgentHarnessNavigateTreeMovesToUserParentAndReturnsEditorText(t *testing.T) {
	ctx := context.Background()
	registration := llm.RegisterFauxProvider()
	defer registration.Unregister()

	session := NewSession(MustInMemorySessionStorage())
	_, _ = session.AppendMessage(llm.UserMessageText("one"))
	a1, _ := session.AppendMessage(harnessAssistantMessage("two"))
	u2, _ := session.AppendMessage(llm.UserMessageText("three"))
	a2, _ := session.AppendMessage(harnessAssistantMessage("four"))

	harness := MustNewAgentHarness(AgentHarnessOptions{Session: session, Model: registration.MustModel()})
	var treeEvent AgentHarnessEvent
	harness.Subscribe(func(_ context.Context, event AgentHarnessEvent) error {
		if event.Type == "session_tree" {
			treeEvent = event
		}
		return nil
	})

	result, err := harness.NavigateTree(ctx, u2, NavigateTreeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.EditorText != "three" || result.SummaryEntry != nil {
		t.Fatalf("result = %#v", result)
	}
	leaf, err := session.LeafID()
	if err != nil {
		t.Fatal(err)
	}
	if leaf == nil || *leaf != a1 {
		t.Fatalf("leaf = %v, want %s", leaf, a1)
	}
	if treeEvent.OldLeafID == nil || *treeEvent.OldLeafID != a2 || treeEvent.NewLeafID == nil || *treeEvent.NewLeafID != a1 {
		t.Fatalf("tree event = %#v", treeEvent)
	}
}

func TestAgentHarnessNavigateTreeCanGenerateBranchSummary(t *testing.T) {
	ctx := context.Background()
	registration := llm.RegisterFauxProvider()
	defer registration.Unregister()

	var summaryPrompt string
	registration.SetResponses([]llm.FauxResponseStep{{
		Factory: func(context llm.Context, _ llm.StreamOptions, _ llm.FauxState, _ llm.Model) (llm.Message, error) {
			text := textFromUserMessages(context.Messages)
			if len(text) > 0 {
				summaryPrompt = text[0]
			}
			return llm.FauxAssistantText("branch body"), nil
		},
	}})

	session := NewSession(MustInMemorySessionStorage())
	u1, _ := session.AppendMessage(llm.UserMessageText("one"))
	_, _ = session.AppendMessage(harnessAssistantMessage("two"))
	_, _ = session.AppendMessage(llm.UserMessageText("three"))
	_, _ = session.AppendMessage(harnessAssistantMessage("four"))

	harness := MustNewAgentHarness(AgentHarnessOptions{
		Session: session,
		Model:   registration.MustModel(),
		GetAPIKeyAndHeaders: func(context.Context, llm.Model) (AgentHarnessAuth, bool, error) {
			return AgentHarnessAuth{APIKey: "test-key"}, true, nil
		},
	})

	result, err := harness.NavigateTree(ctx, u1, NavigateTreeOptions{Summarize: true, CustomInstructions: "focus"})
	if err != nil {
		t.Fatal(err)
	}
	if result.EditorText != "one" || result.SummaryEntry == nil {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.SummaryEntry.Summary, "branch body") || !strings.Contains(result.SummaryEntry.Summary, "different conversation branch") {
		t.Fatalf("summary entry = %#v", result.SummaryEntry)
	}
	if !strings.Contains(summaryPrompt, "Additional focus: focus") {
		t.Fatalf("summary prompt = %q", summaryPrompt)
	}
}

func TestAgentHarnessNavigateTreeHookCanProvideSummary(t *testing.T) {
	ctx := context.Background()
	registration := llm.RegisterFauxProvider()
	defer registration.Unregister()

	session := NewSession(MustInMemorySessionStorage())
	u1, _ := session.AppendMessage(llm.UserMessageText("one"))
	_, _ = session.AppendMessage(harnessAssistantMessage("two"))
	_, _ = session.AppendMessage(llm.UserMessageText("three"))
	_, _ = session.AppendMessage(harnessAssistantMessage("four"))

	harness := MustNewAgentHarness(AgentHarnessOptions{Session: session, Model: registration.MustModel()})
	harness.On("session_before_tree", func(context.Context, AgentHarnessEvent) (*AgentHarnessHookResult, error) {
		return &AgentHarnessHookResult{
			BranchSummary:    BranchSummaryResult{Summary: "hook summary", ReadFiles: []string{"read.go"}, ModifiedFiles: []string{"edit.go"}},
			HasBranchSummary: true,
		}, nil
	})

	result, err := harness.NavigateTree(ctx, u1, NavigateTreeOptions{Summarize: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.SummaryEntry == nil || result.SummaryEntry.Summary != "hook summary" || !result.SummaryEntry.FromHook {
		t.Fatalf("summary entry = %#v", result.SummaryEntry)
	}
	details, ok := result.SummaryEntry.Details.(map[string]any)
	if !ok || !reflect.DeepEqual(details["readFiles"], []string{"read.go"}) || !reflect.DeepEqual(details["modifiedFiles"], []string{"edit.go"}) {
		t.Fatalf("summary details = %#v", result.SummaryEntry.Details)
	}
	if registration.PendingResponseCount() != 0 {
		t.Fatalf("hook-provided summary should not call provider; pending responses = %d", registration.PendingResponseCount())
	}
}

func TestAgentHarnessNavigateTreeHookOverridesSummaryInstructions(t *testing.T) {
	ctx := context.Background()
	registration := llm.RegisterFauxProvider()
	defer registration.Unregister()

	var summaryPrompt string
	registration.SetResponses([]llm.FauxResponseStep{{
		Factory: func(context llm.Context, _ llm.StreamOptions, _ llm.FauxState, _ llm.Model) (llm.Message, error) {
			text := textFromUserMessages(context.Messages)
			if len(text) > 0 {
				summaryPrompt = text[0]
			}
			return llm.FauxAssistantText("branch body"), nil
		},
	}})

	session := NewSession(MustInMemorySessionStorage())
	u1, _ := session.AppendMessage(llm.UserMessageText("one"))
	_, _ = session.AppendMessage(harnessAssistantMessage("two"))
	_, _ = session.AppendMessage(llm.UserMessageText("three"))
	_, _ = session.AppendMessage(harnessAssistantMessage("four"))

	harness := MustNewAgentHarness(AgentHarnessOptions{
		Session: session,
		Model:   registration.MustModel(),
		GetAPIKeyAndHeaders: func(context.Context, llm.Model) (AgentHarnessAuth, bool, error) {
			return AgentHarnessAuth{APIKey: "test-key"}, true, nil
		},
	})
	harness.On("session_before_tree", func(context.Context, AgentHarnessEvent) (*AgentHarnessHookResult, error) {
		return &AgentHarnessHookResult{
			CustomInstructions:     "hook-only instructions",
			HasCustomInstructions:  true,
			ReplaceInstructions:    true,
			HasReplaceInstructions: true,
		}, nil
	})

	if _, err := harness.NavigateTree(ctx, u1, NavigateTreeOptions{Summarize: true, CustomInstructions: "caller focus"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(summaryPrompt, "hook-only instructions") || strings.Contains(summaryPrompt, "caller focus") || strings.Contains(summaryPrompt, "Use this exact shape") {
		t.Fatalf("summary prompt = %q", summaryPrompt)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func testStringPtr(value string) *string { return &value }

func calculateTool() core.AgentTool {
	return core.AgentTool{
		Name:        "calculate",
		Description: "Calculate an expression.",
		Parameters:  llm.Object(map[string]llm.Schema{"expression": llm.String()}, "expression"),
		Execute: func(_ context.Context, _ string, params map[string]any, _ core.AgentToolUpdateCallback) (core.AgentToolResult, error) {
			return core.AgentToolResult{Content: []llm.ContentPart{llm.Text("2")}, Details: params}, nil
		},
	}
}

func timeTool() core.AgentTool {
	return core.AgentTool{
		Name:        "time",
		Description: "Get the time.",
		Parameters:  llm.Object(map[string]llm.Schema{}),
		Execute: func(context.Context, string, map[string]any, core.AgentToolUpdateCallback) (core.AgentToolResult, error) {
			return core.AgentToolResult{Content: []llm.ContentPart{llm.Text("now")}}, nil
		},
	}
}

func countUserMessages(messages []llm.Message) int {
	count := 0
	for _, message := range messages {
		if message.Role == llm.RoleUser {
			count++
		}
	}
	return count
}

func textFromUserMessages(messages []llm.Message) []string {
	var text []string
	for _, message := range messages {
		if message.Role != llm.RoleUser {
			continue
		}
		for _, part := range message.Content {
			if part.Type == llm.ContentText {
				text = append(text, part.Text)
			}
		}
	}
	return text
}

func textFromSessionUserMessages(session *Session) []string {
	return textFromUserMessages(sessionMessages(session))
}

func sessionMessages(session *Session) []llm.Message {
	var messages []llm.Message
	for _, entry := range session.Entries() {
		if entry.Type == "message" {
			messages = append(messages, entry.Message)
		}
	}
	return messages
}

func toolNames(tools []llm.Tool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}
