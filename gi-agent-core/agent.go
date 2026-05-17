package giagentcore

import (
	"context"
	"fmt"
	"sync"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type AgentState struct {
	SystemPrompt     string
	Model            llm.Model
	ThinkingLevel    string
	Tools            []AgentTool
	Messages         []llm.Message
	IsStreaming      bool
	StreamingMessage *llm.Message
	PendingToolCalls map[string]bool
	ErrorMessage     string
}

type AgentOptions struct {
	InitialState     AgentState
	ConvertToLLM     func(messages []llm.Message) ([]llm.Message, error)
	TransformContext func(ctx context.Context, messages []llm.Message) ([]llm.Message, error)
	StreamFn         StreamFn
	GetAPIKey        func(provider string) string
	BeforeToolCall   func(context.Context, BeforeToolCallContext) (BeforeToolCallResult, error)
	AfterToolCall    func(context.Context, AfterToolCallContext) (AfterToolCallResult, error)
	PrepareNextTurn  func(context.Context) (AgentLoopTurnUpdate, bool, error)
	SteeringMode     string
	FollowUpMode     string
	SessionID        string
	ThinkingBudgets  map[string]int
	Transport        string
	MaxRetryDelayMs  int
	ToolExecution    string
}

type AgentOption func(*AgentOptions)

func WithInitialState(state AgentState) AgentOption {
	return func(options *AgentOptions) { options.InitialState = state }
}

func WithStreamFn(streamFn StreamFn) AgentOption {
	return func(options *AgentOptions) { options.StreamFn = streamFn }
}

func WithConvertToLLM(convert func(messages []llm.Message) ([]llm.Message, error)) AgentOption {
	return func(options *AgentOptions) { options.ConvertToLLM = convert }
}

func WithTransformContext(transform func(ctx context.Context, messages []llm.Message) ([]llm.Message, error)) AgentOption {
	return func(options *AgentOptions) { options.TransformContext = transform }
}

func WithToolHooks(
	before func(context.Context, BeforeToolCallContext) (BeforeToolCallResult, error),
	after func(context.Context, AfterToolCallContext) (AfterToolCallResult, error),
) AgentOption {
	return func(options *AgentOptions) {
		options.BeforeToolCall = before
		options.AfterToolCall = after
	}
}

func WithSession(id string) AgentOption {
	return func(options *AgentOptions) { options.SessionID = id }
}

func WithToolExecution(mode string) AgentOption {
	return func(options *AgentOptions) { options.ToolExecution = mode }
}

func New(options ...AgentOption) *Agent {
	var opts AgentOptions
	for _, option := range options {
		option(&opts)
	}
	return NewAgent(opts)
}

type Agent struct {
	state AgentState
	mu    sync.Mutex

	listeners []func(AgentEvent, context.Context) error

	convertToLLM     func(messages []llm.Message) ([]llm.Message, error)
	transformContext func(ctx context.Context, messages []llm.Message) ([]llm.Message, error)
	streamFn         StreamFn
	getAPIKey        func(provider string) string
	beforeToolCall   func(context.Context, BeforeToolCallContext) (BeforeToolCallResult, error)
	afterToolCall    func(context.Context, AfterToolCallContext) (AfterToolCallResult, error)
	prepareNextTurn  func(context.Context) (AgentLoopTurnUpdate, bool, error)

	steeringQueue pendingMessageQueue
	followUpQueue pendingMessageQueue

	sessionID       string
	thinkingBudgets map[string]int
	transport       string
	maxRetryDelayMs int
	toolExecution   string

	activeCancel context.CancelFunc
	activeDone   chan struct{}
	activeCtx    context.Context
}

func NewAgent(options AgentOptions) *Agent {
	state := options.InitialState
	if state.Model.ID == "" {
		state.Model = llm.Model{ID: "unknown", Name: "unknown", API: "unknown", Provider: "unknown", Input: []string{}}
	}
	if state.ThinkingLevel == "" {
		state.ThinkingLevel = "off"
	}
	state.Tools = append([]AgentTool{}, state.Tools...)
	state.Messages = append([]llm.Message{}, state.Messages...)
	state.PendingToolCalls = map[string]bool{}

	steeringMode := options.SteeringMode
	if steeringMode == "" {
		steeringMode = QueueOneAtTime
	}
	followUpMode := options.FollowUpMode
	if followUpMode == "" {
		followUpMode = QueueOneAtTime
	}
	transport := options.Transport
	if transport == "" {
		transport = "auto"
	}
	toolExecution := options.ToolExecution
	if toolExecution == "" {
		toolExecution = ToolExecutionParallel
	}

	return &Agent{
		state:            state,
		convertToLLM:     options.ConvertToLLM,
		transformContext: options.TransformContext,
		streamFn:         options.StreamFn,
		getAPIKey:        options.GetAPIKey,
		beforeToolCall:   options.BeforeToolCall,
		afterToolCall:    options.AfterToolCall,
		prepareNextTurn:  options.PrepareNextTurn,
		steeringQueue:    pendingMessageQueue{mode: steeringMode},
		followUpQueue:    pendingMessageQueue{mode: followUpMode},
		sessionID:        options.SessionID,
		thinkingBudgets:  options.ThinkingBudgets,
		transport:        transport,
		maxRetryDelayMs:  options.MaxRetryDelayMs,
		toolExecution:    toolExecution,
	}
}

func (a *Agent) State() AgentState {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := a.state
	state.Tools = append([]AgentTool{}, state.Tools...)
	state.Messages = append([]llm.Message{}, state.Messages...)
	pending := map[string]bool{}
	for key, value := range state.PendingToolCalls {
		pending[key] = value
	}
	state.PendingToolCalls = pending
	return state
}

func (a *Agent) SetSystemPrompt(prompt string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.SystemPrompt = prompt
}

func (a *Agent) SetModel(model llm.Model) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Model = model
}

func (a *Agent) SetThinkingLevel(level string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.ThinkingLevel = level
}

func (a *Agent) SetTools(tools []AgentTool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Tools = append([]AgentTool{}, tools...)
}

func (a *Agent) SetMessages(messages []llm.Message) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Messages = append([]llm.Message{}, messages...)
}

func (a *Agent) Subscribe(listener func(AgentEvent, context.Context) error) func() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.listeners = append(a.listeners, listener)
	index := len(a.listeners) - 1
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if index >= 0 && index < len(a.listeners) {
			a.listeners[index] = nil
		}
	}
}

func (a *Agent) PromptText(ctx context.Context, text string, images ...llm.ContentPart) error {
	content := []llm.ContentPart{llm.Text(text)}
	content = append(content, images...)
	return a.Prompt(ctx, llm.Message{Role: llm.RoleUser, Content: content, Timestamp: llm.NowMillis()})
}

func (a *Agent) Prompt(ctx context.Context, messages ...llm.Message) error {
	if len(messages) == 0 {
		return nil
	}
	if !a.startRun(ctx) {
		return fmt.Errorf("Agent is already processing a prompt. Use steer() or followUp() to queue messages, or wait for completion")
	}
	defer a.finishRun()
	if err := a.runPromptMessages(messages, false); err != nil {
		return a.handleRunFailure(err)
	}
	return nil
}

func (a *Agent) Continue(ctx context.Context) error {
	if !a.startRun(ctx) {
		return fmt.Errorf("Agent is already processing. Wait for completion before continuing")
	}
	defer a.finishRun()

	state := a.State()
	if len(state.Messages) == 0 {
		return fmt.Errorf("No messages to continue from")
	}
	last := state.Messages[len(state.Messages)-1]
	if last.Role == llm.RoleAssistant {
		queuedSteering := a.steeringQueue.drain()
		if len(queuedSteering) > 0 {
			if err := a.runPromptMessages(queuedSteering, true); err != nil {
				return a.handleRunFailure(err)
			}
			return nil
		}
		queuedFollowUps := a.followUpQueue.drain()
		if len(queuedFollowUps) > 0 {
			if err := a.runPromptMessages(queuedFollowUps, false); err != nil {
				return a.handleRunFailure(err)
			}
			return nil
		}
		return fmt.Errorf("Cannot continue from message role: assistant")
	}
	if err := a.runContinuation(); err != nil {
		return a.handleRunFailure(err)
	}
	return nil
}

func (a *Agent) Steer(message llm.Message) {
	a.steeringQueue.enqueue(message)
}

func (a *Agent) FollowUp(message llm.Message) {
	a.followUpQueue.enqueue(message)
}

func (a *Agent) ClearSteeringQueue() { a.steeringQueue.clear() }
func (a *Agent) ClearFollowUpQueue() { a.followUpQueue.clear() }
func (a *Agent) ClearAllQueues() {
	a.ClearSteeringQueue()
	a.ClearFollowUpQueue()
}

func (a *Agent) HasQueuedMessages() bool {
	return a.steeringQueue.hasItems() || a.followUpQueue.hasItems()
}

func (a *Agent) Abort() {
	a.mu.Lock()
	cancel := a.activeCancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *Agent) WaitForIdle(ctx context.Context) error {
	a.mu.Lock()
	done := a.activeDone
	a.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Agent) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Messages = nil
	a.state.IsStreaming = false
	a.state.StreamingMessage = nil
	a.state.PendingToolCalls = map[string]bool{}
	a.state.ErrorMessage = ""
	a.followUpQueue.clear()
	a.steeringQueue.clear()
}

func (a *Agent) runPromptMessages(messages []llm.Message, skipInitialSteeringPoll bool) error {
	_, err := RunAgentLoop(messages, a.contextSnapshot(), a.loopConfig(skipInitialSteeringPoll), func(event AgentEvent) error {
		return a.processEvent(event)
	}, a.activeCtx, a.streamFn)
	return err
}

func (a *Agent) runContinuation() error {
	_, err := RunAgentLoopContinue(a.contextSnapshot(), a.loopConfig(false), func(event AgentEvent) error {
		return a.processEvent(event)
	}, a.activeCtx, a.streamFn)
	return err
}

func (a *Agent) handleRunFailure(err error) error {
	aborted := false
	if a.activeCtx != nil {
		aborted = a.activeCtx.Err() != nil
	}
	model := a.State().Model
	message := llm.AssistantErrorMessage(err.Error(), model, aborted)
	events := []AgentEvent{
		{Type: "message_start", Message: message},
		{Type: "message_end", Message: message},
		{Type: "turn_end", Message: message},
		{Type: "agent_end", Messages: []llm.Message{message}},
	}
	for _, event := range events {
		if processErr := a.processEvent(event); processErr != nil {
			return processErr
		}
	}
	return nil
}

func (a *Agent) contextSnapshot() AgentContext {
	state := a.State()
	return AgentContext{SystemPrompt: state.SystemPrompt, Messages: state.Messages, Tools: state.Tools}
}

func (a *Agent) loopConfig(skipInitialSteeringPoll bool) AgentLoopConfig {
	state := a.State()
	reasoning := ""
	if state.ThinkingLevel != "off" {
		reasoning = state.ThinkingLevel
	}
	skipSteering := skipInitialSteeringPoll
	return AgentLoopConfig{
		SimpleStreamOptions: llm.SimpleStreamOptions{
			Reasoning:       reasoning,
			SessionID:       a.sessionID,
			ThinkingBudgets: a.thinkingBudgets,
			Transport:       a.transport,
			MaxRetryDelayMs: a.maxRetryDelayMs,
		},
		Model:            state.Model,
		ConvertToLLM:     a.convertToLLM,
		TransformContext: a.transformContext,
		GetAPIKey:        a.getAPIKey,
		ToolExecution:    a.toolExecution,
		BeforeToolCall:   a.beforeToolCall,
		AfterToolCall:    a.afterToolCall,
		PrepareNextTurn: func(_ PrepareNextTurnContext) (AgentLoopTurnUpdate, bool, error) {
			if a.prepareNextTurn == nil {
				return AgentLoopTurnUpdate{}, false, nil
			}
			return a.prepareNextTurn(a.activeCtx)
		},
		GetSteeringMessages: func() ([]llm.Message, error) {
			if skipSteering {
				skipSteering = false
				return nil, nil
			}
			return a.steeringQueue.drain(), nil
		},
		GetFollowUpMessages: func() ([]llm.Message, error) {
			return a.followUpQueue.drain(), nil
		},
	}
}

func (a *Agent) startRun(parent context.Context) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeDone != nil {
		return false
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	a.activeCtx = ctx
	a.activeCancel = cancel
	a.activeDone = make(chan struct{})
	a.state.IsStreaming = true
	a.state.StreamingMessage = nil
	a.state.ErrorMessage = ""
	return true
}

func (a *Agent) finishRun() {
	a.mu.Lock()
	a.state.IsStreaming = false
	a.state.StreamingMessage = nil
	a.state.PendingToolCalls = map[string]bool{}
	cancel := a.activeCancel
	done := a.activeDone
	a.activeCancel = nil
	a.activeDone = nil
	a.activeCtx = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		close(done)
	}
}

func (a *Agent) processEvent(event AgentEvent) error {
	a.mu.Lock()
	switch event.Type {
	case "message_start", "message_update":
		msg := event.Message
		a.state.StreamingMessage = &msg
	case "message_end":
		a.state.StreamingMessage = nil
		a.state.Messages = append(a.state.Messages, event.Message)
	case "tool_execution_start":
		pending := copyPending(a.state.PendingToolCalls)
		pending[event.ToolCallID] = true
		a.state.PendingToolCalls = pending
	case "tool_execution_end":
		pending := copyPending(a.state.PendingToolCalls)
		delete(pending, event.ToolCallID)
		a.state.PendingToolCalls = pending
	case "turn_end":
		if event.Message.Role == llm.RoleAssistant && event.Message.ErrorMessage != "" {
			a.state.ErrorMessage = event.Message.ErrorMessage
		}
	case "agent_end":
		a.state.StreamingMessage = nil
	}
	listeners := append([]func(AgentEvent, context.Context) error{}, a.listeners...)
	ctx := a.activeCtx
	a.mu.Unlock()

	if ctx == nil {
		return fmt.Errorf("Agent listener invoked outside active run")
	}
	for _, listener := range listeners {
		if listener == nil {
			continue
		}
		if err := listener(event, ctx); err != nil {
			return err
		}
	}
	return nil
}

func copyPending(input map[string]bool) map[string]bool {
	output := map[string]bool{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

type pendingMessageQueue struct {
	mu       sync.Mutex
	mode     string
	messages []llm.Message
}

func (q *pendingMessageQueue) enqueue(message llm.Message) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.messages = append(q.messages, message)
}

func (q *pendingMessageQueue) hasItems() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.messages) > 0
}

func (q *pendingMessageQueue) drain() []llm.Message {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.messages) == 0 {
		return nil
	}
	if q.mode == QueueAll {
		drained := append([]llm.Message{}, q.messages...)
		q.messages = nil
		return drained
	}
	first := q.messages[0]
	q.messages = q.messages[1:]
	return []llm.Message{first}
}

func (q *pendingMessageQueue) clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.messages = nil
}
