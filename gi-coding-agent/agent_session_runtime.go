package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

type AgentSessionEvent struct {
	Type                  string                         `json:"type"`
	Source                string                         `json:"source,omitempty"`
	Reason                string                         `json:"reason,omitempty"`
	Result                *agentharness.CompactionResult `json:"result,omitempty"`
	Aborted               bool                           `json:"aborted,omitempty"`
	WillRetry             bool                           `json:"willRetry,omitempty"`
	ErrorMessage          string                         `json:"errorMessage,omitempty"`
	Message               *llm.Message                   `json:"message,omitempty"`
	Attempt               int                            `json:"attempt,omitempty"`
	MaxAttempts           int                            `json:"maxAttempts,omitempty"`
	DelayMs               int                            `json:"delayMs,omitempty"`
	Success               bool                           `json:"success,omitempty"`
	FinalError            string                         `json:"finalError,omitempty"`
	Steering              []string                       `json:"steering,omitempty"`
	FollowUp              []string                       `json:"followUp,omitempty"`
	Messages              []llm.Message                  `json:"messages,omitempty"`
	AssistantMessageEvent *llm.AssistantMessageEvent     `json:"assistantMessageEvent,omitempty"`
	ToolCallID            string                         `json:"toolCallId,omitempty"`
	ToolName              string                         `json:"toolName,omitempty"`
	Args                  map[string]any                 `json:"args,omitempty"`
	ToolResult            *llm.Message                   `json:"toolResult,omitempty"`
	PartialToolResult     *llm.Message                   `json:"partialToolResult,omitempty"`
	Name                  string                         `json:"name,omitempty"`
	Entry                 *FileEntry                     `json:"entry,omitempty"`
}

type AgentSessionEventListener func(AgentSessionEvent)

type AgentSessionResponder func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error)
type AgentSessionStreamResponder func(prompt string, context []llm.Message, model llm.Model) (*llm.AssistantMessageEventStream, error)

type AgentSessionCompactionSummarizer func(preparation agentharness.CompactionPreparation, customInstructions string) (agentharness.CompactionResult, error)

var errCompactionCancelled = errors.New("Compaction cancelled")

func isCompactionCancelledError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, errCompactionCancelled) || strings.Contains(err.Error(), errCompactionCancelled.Error())
}

type AgentSessionRetrySettings struct {
	Enabled     bool
	MaxRetries  int
	BaseDelayMs int
}

func DefaultAgentSessionRetrySettings() AgentSessionRetrySettings {
	return AgentSessionRetrySettings{Enabled: true, MaxRetries: defaultAgentSessionMaxRetries, BaseDelayMs: defaultAgentSessionBaseDelayMS}
}

func (s *AgentSession) Subscribe(listener AgentSessionEventListener) func() {
	if s == nil || listener == nil {
		return func() {}
	}
	s.eventListenersMu.Lock()
	s.nextEventListenerID++
	id := s.nextEventListenerID
	s.eventListeners = append(s.eventListeners, agentSessionEventListenerRegistration{
		id:       id,
		listener: listener,
	})
	s.eventListenersMu.Unlock()
	return func() {
		s.eventListenersMu.Lock()
		defer s.eventListenersMu.Unlock()
		for index, registration := range s.eventListeners {
			if registration.id != id {
				continue
			}
			s.eventListeners = append(s.eventListeners[:index], s.eventListeners[index+1:]...)
			return
		}
	}
}

func (s *AgentSession) emit(event AgentSessionEvent) {
	if s == nil {
		return
	}
	if event.Type == "message_end" &&
		event.Message != nil &&
		s.runMessageCapture {
		s.activeRunMessages = append(
			s.activeRunMessages,
			*event.Message,
		)
	}
	listeners := s.eventListenerSnapshot()
	for _, listener := range listeners {
		listener(event)
	}
}

func (s *AgentSession) eventListenerSnapshot() []AgentSessionEventListener {
	s.eventListenersMu.RLock()
	defer s.eventListenersMu.RUnlock()
	listeners := make([]AgentSessionEventListener, 0, len(s.eventListeners))
	for _, registration := range s.eventListeners {
		if registration.listener != nil {
			listeners = append(listeners, registration.listener)
		}
	}
	return listeners
}

// AppendCustomEntry persists the entry before publishing its presentation
// event. Consumers therefore never observe an entry that is absent from the
// append-only session tree.
func (s *AgentSession) AppendCustomEntry(
	customType string,
	data any,
) (string, error) {
	if s == nil || s.SessionManager == nil {
		return "", errors.New("session manager is required")
	}
	entryID := s.SessionManager.AppendCustomEntry(customType, data)
	entry := s.SessionManager.GetEntry(entryID)
	if entry != nil {
		s.emit(AgentSessionEvent{Type: "entry_appended", Entry: entry})
	}
	return entryID, nil
}

func (s *AgentSession) Prompt(text string) error {
	return s.PromptWithImages(text, nil)
}

type agentSessionPrompt struct {
	text   string
	images []llm.ContentPart
}

func (s *AgentSession) PromptWithImages(text string, images []llm.ContentPart) error {
	prompt, err := s.beginPrompt(text, images)
	if err != nil || prompt == nil {
		return err
	}
	return s.runPreparedPrompt(*prompt)
}

// startPromptWithImages performs all prompt validation and reserves the
// streaming lifecycle before returning. RPC callers can therefore acknowledge
// an accepted prompt without exposing a window where the session still appears
// idle or can be disposed while its goroutine has not started.
func (s *AgentSession) startPromptWithImages(text string, images []llm.ContentPart) error {
	prompt, err := s.beginPrompt(text, images)
	if err != nil || prompt == nil {
		return err
	}
	go func() {
		_ = s.runPreparedPrompt(*prompt)
	}()
	return nil
}

func (s *AgentSession) beginPrompt(text string, images []llm.ContentPart) (*agentSessionPrompt, error) {
	if s == nil || s.SessionManager == nil {
		return nil, errors.New("session manager is required")
	}
	s.SyncRuntimeSettings()
	prompt := strings.TrimSpace(text)
	if prompt == "" {
		return nil, errors.New("prompt is required")
	}
	if strings.HasPrefix(prompt, "/") && s.ExtensionRuntime != nil {
		name, args, ok := parseSlashCommandInvocation(prompt)
		if !ok {
			return nil, errors.New("prompt is required")
		}
		if command := s.ExtensionRuntime.GetCommand(name); command != nil && (command.Handler != nil || command.HandlerWithContext != nil) {
			if command.HandlerWithContext != nil {
				return nil, command.HandlerWithContext(args, s.ExtensionRuntime.CreateCommandContext())
			}
			return nil, command.Handler(args)
		}
	}
	if s.IsStreaming() {
		return nil, errors.New("Agent is already processing. Specify streamingBehavior ('steer' or 'followUp') to queue the message.")
	}
	promptImages := normalizePromptImages(images)
	if s.ExtensionRuntime != nil {
		inputResult := s.ExtensionRuntime.EmitInput(prompt, promptImages, "interactive")
		switch inputResult.Action {
		case "handled":
			return nil, nil
		case "transform":
			prompt = strings.TrimSpace(inputResult.Text)
			if inputResult.ImagesSet {
				promptImages = normalizePromptImages(inputResult.Images)
			}
		}
	}
	expandedPrompt := s.expandPromptCommands(prompt)
	if s.Agent == nil || strings.TrimSpace(s.Agent.State.Model.ID) == "" {
		return nil, errors.New(formatNoModelSelectedMessage())
	}
	if s.Preflight != nil {
		if err := s.Preflight(s.Agent.State.Model); err != nil {
			return nil, err
		}
	}
	if !s.lifecycle.tryStartStreaming() {
		return nil, errors.New("Agent is already processing. Specify streamingBehavior ('steer' or 'followUp') to queue the message.")
	}
	s.lifecycle.resetAbort()
	return &agentSessionPrompt{text: expandedPrompt, images: promptImages}, nil
}

func (s *AgentSession) runPreparedPrompt(prompt agentSessionPrompt) (returnErr error) {
	defer func() {
		returnErr = errors.Join(returnErr, s.settleAgentRun())
	}()
	s.flushPendingBashMessages()
	content := []llm.ContentPart{llm.Text(prompt.text)}
	content = append(content, prompt.images...)
	if err := s.emitAgentLifecycleEvent(AgentSessionEvent{Type: "agent_start"}); err != nil {
		return err
	}
	if err := s.emitAgentLifecycleEvent(AgentSessionEvent{Type: "turn_start"}); err != nil {
		return err
	}
	userMessage := llm.Message{Role: llm.RoleUser, Content: content, Timestamp: llm.NowMillis()}
	if err := s.emitMessageStart(userMessage); err != nil {
		return err
	}
	var err error
	userMessage, err = s.emitExtensionMessageEnd(userMessage)
	if err != nil {
		return err
	}
	s.SessionManager.AppendMessage(sessionMessageValue(userMessage))
	s.emit(AgentSessionEvent{Type: "message_end", Message: &userMessage})
	for _, custom := range s.queues.takeNextTurn() {
		if err := s.appendCustomMessage(custom, true); err != nil {
			return err
		}
	}
	if err := s.applyBeforeAgentStart(prompt.text, prompt.images); err != nil {
		return err
	}
	return s.runPromptLoop(prompt.text)
}

func parseSlashCommandInvocation(text string) (string, string, bool) {
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	withoutSlash := strings.TrimPrefix(text, "/")
	if strings.TrimSpace(withoutSlash) == "" {
		return "", "", false
	}
	command := withoutSlash
	args := ""
	for index, r := range withoutSlash {
		if unicode.IsSpace(r) {
			command = withoutSlash[:index]
			args = strings.TrimSpace(withoutSlash[index+len(string(r)):])
			break
		}
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return "", "", false
	}
	return command, args, true
}

func (s *AgentSession) expandPromptCommands(prompt string) string {
	expanded := s.expandSkillCommand(prompt)
	if loader, ok := s.ResourceLoader.(AgentSessionPromptResourceLoader); ok {
		expanded = ExpandPromptTemplate(expanded, loader.GetPrompts().Prompts)
	}
	return expanded
}

func (s *AgentSession) expandSkillCommand(prompt string) string {
	if s == nil || s.ResourceLoader == nil || !strings.HasPrefix(prompt, "/skill:") {
		return prompt
	}
	command, args, ok := parseSlashCommandInvocation(prompt)
	if !ok || !strings.HasPrefix(command, "skill:") {
		return prompt
	}
	skillName := strings.TrimSpace(strings.TrimPrefix(command, "skill:"))
	if skillName == "" {
		return prompt
	}
	for _, skill := range s.ResourceLoader.GetSkills().Skills {
		if skill.Name == skillName {
			return agentharness.FormatSkillInvocation(skill, args)
		}
	}
	return prompt
}

func normalizePromptImages(images []llm.ContentPart) []llm.ContentPart {
	if len(images) == 0 {
		return nil
	}
	result := make([]llm.ContentPart, 0, len(images))
	for _, image := range images {
		if image.Type == "" && (image.Data != "" || image.MIMEType != "") {
			image.Type = llm.ContentImage
		}
		if image.Type != llm.ContentImage {
			continue
		}
		result = append(result, image)
	}
	return result
}

func (s *AgentSession) applyBeforeAgentStart(prompt string, images []llm.ContentPart) error {
	if s == nil || s.Agent == nil {
		return nil
	}
	basePrompt := s.BaseSystemPrompt
	if basePrompt == "" {
		basePrompt = s.SystemPrompt
	}
	s.SystemPrompt = basePrompt
	s.Agent.State.SystemPrompt = basePrompt
	if s.ExtensionRuntime == nil || !s.ExtensionRuntime.HasHandlers(ProtocolEventBeforeAgentStart) {
		return nil
	}
	result, err := s.ExtensionRuntime.EmitSessionEvent(ProtocolSessionEvent{
		Type:         ProtocolEventBeforeAgentStart,
		Prompt:       prompt,
		Images:       append([]llm.ContentPart(nil), images...),
		SystemPrompt: basePrompt,
	})
	if err != nil {
		return err
	}
	for _, message := range result.Messages {
		if message.Timestamp == 0 {
			message.Timestamp = llm.NowMillis()
		}
		s.SessionManager.AppendMessage(sessionMessageValue(message))
	}
	for _, custom := range result.CustomMessages {
		if err := s.appendCustomMessage(QueuedCustomMessage{
			CustomType: custom.CustomType,
			Content:    custom.Content,
			Display:    custom.Display,
			Details:    custom.Details,
		}, true); err != nil {
			return err
		}
	}
	if result.SystemPromptSet {
		s.SystemPrompt = result.SystemPrompt
		s.Agent.State.SystemPrompt = result.SystemPrompt
	}
	return nil
}

func (s *AgentSession) providerContextMessages() ([]llm.Message, error) {
	messages := s.baseProviderContextMessages()
	if s == nil || s.ExtensionRuntime == nil || !s.ExtensionRuntime.HasHandlers(ProtocolEventContext) {
		return messages, nil
	}
	result, err := s.ExtensionRuntime.EmitSessionEvent(ProtocolSessionEvent{
		Type:     ProtocolEventContext,
		Messages: append([]llm.Message(nil), messages...),
	})
	if err != nil {
		return nil, err
	}
	if result.MessagesSet {
		return append([]llm.Message(nil), result.Messages...), nil
	}
	return messages, nil
}

func providerContextFromSessionMessages(messages []llm.Message) []llm.Message {
	return agentharness.ConvertToLLM(messages)
}

func providerContextFromSessionValues(messages []any) []llm.Message {
	converted := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		if value, ok := sessionMessageToLLM(message); ok {
			converted = append(converted, value)
		}
	}
	return providerContextFromSessionMessages(converted)
}

func (s *AgentSession) runPromptLoop(prompt string) error {
	defer s.lifecycle.setActivity(agentSessionActivityRetrying, false)
	state := agentPromptRunState{prompt: prompt}
	for {
		result, err := s.runAgentPrompt(&state)
		if err != nil {
			return err
		}
		willRetry := s.willRetryAfterAgentRun(state, result)
		if err := s.emitAgentLifecycleEvent(AgentSessionEvent{
			Type:      "agent_end",
			WillRetry: willRetry,
		}); err != nil {
			return err
		}
		continuation, err := s.handlePostAgentRun(&state, result)
		if err != nil {
			return err
		}
		if continuation == agentPromptContinuationNone {
			return nil
		}
		if err := s.beginAgentContinuation(&state, continuation); err != nil {
			return err
		}
	}
}

func (s *AgentSession) respondToPrompt(prompt string, messages []llm.Message, model llm.Model, responder AgentSessionResponder) (llm.Message, error) {
	if s == nil || s.StreamResponder == nil {
		return responder(prompt, messages, model)
	}
	return s.streamAssistantResponse(prompt, messages, model)
}

func (s *AgentSession) streamAssistantResponse(prompt string, messages []llm.Message, model llm.Model) (llm.Message, error) {
	stream, err := s.StreamResponder(prompt, messages, model)
	if err != nil {
		return llm.Message{}, err
	}
	if stream == nil {
		return llm.Message{}, errors.New("stream responder returned nil stream")
	}
	started := false
	var final llm.Message
	for event := range stream.Events() {
		event = s.normalizeAssistantMessageEvent(event)
		switch event.Type {
		case "start":
			started = true
			if err := s.emitMessageStart(event.Partial); err != nil {
				return llm.Message{}, err
			}
		case "text_start", "text_delta", "text_end", "thinking_start", "thinking_delta", "thinking_end", "toolcall_start", "toolcall_delta", "toolcall_end":
			if err := s.emitAssistantMessageEvent(event); err != nil {
				return llm.Message{}, err
			}
			if s.lifecycle.isAbortRequested() {
				return s.abortedAssistantMessage(event.Partial)
			}
		case "done":
			final = event.Message
			if !started {
				if err := s.emitMessageStart(final); err != nil {
					return llm.Message{}, err
				}
			}
			return final, nil
		case "error":
			final = event.Error
			if !started {
				if err := s.emitMessageStart(final); err != nil {
					return llm.Message{}, err
				}
			}
			return final, nil
		}
	}
	final, err = stream.Result(context.Background())
	if err != nil {
		return llm.Message{}, err
	}
	final = s.normalizeAssistantMessage(final)
	if !started {
		if err := s.emitMessageStart(final); err != nil {
			return llm.Message{}, err
		}
	}
	return final, nil
}

func (s *AgentSession) normalizeAssistantMessageEvent(event llm.AssistantMessageEvent) llm.AssistantMessageEvent {
	switch event.Type {
	case "start", "text_start", "text_delta", "text_end", "thinking_start", "thinking_delta", "thinking_end", "toolcall_start", "toolcall_delta", "toolcall_end":
		event.Partial = s.normalizeAssistantMessage(event.Partial)
	case "done":
		event.Message = s.normalizeAssistantMessage(event.Message)
	case "error":
		event.Error = s.normalizeAssistantMessage(event.Error)
	}
	return event
}

func retryDelayMS(baseDelayMS, attempt int) int {
	if baseDelayMS <= 0 || attempt <= 1 {
		return baseDelayMS
	}
	delay := baseDelayMS
	for i := 1; i < attempt; i++ {
		if delay > int(^uint(0)>>1)/2 {
			return int(^uint(0) >> 1)
		}
		delay *= 2
	}
	return delay
}

func (s *AgentSession) prepareRetryDelay() (<-chan struct{}, func()) {
	cancelled := make(chan struct{})
	var once sync.Once
	cleanup := s.lifecycle.startNestedCancellableActivity(
		agentSessionActivityRetrying,
		agentSessionCancellationRetry,
		func() {
			once.Do(func() {
				close(cancelled)
			})
		},
	)
	return cancelled, func() {
		cleanup()
	}
}

func waitForRetryDelay(delayMs int, cancelled <-chan struct{}) bool {
	if delayMs <= 0 {
		select {
		case <-cancelled:
			return false
		default:
			return true
		}
	}
	timer := time.NewTimer(time.Duration(delayMs) * time.Millisecond)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()
	select {
	case <-timer.C:
		return true
	case <-cancelled:
		return false
	}
}

func (s *AgentSession) dequeueQueuedPrompt(kind string) (string, bool, error) {
	var messages []QueuedUserMessage
	switch kind {
	case "steering":
		messages = s.queues.takePrompt(agentSessionSteeringQueue, s.SteeringMode)
	case "follow-up":
		messages = s.queues.takePrompt(agentSessionFollowUpQueue, s.FollowUpMode)
	default:
		return "", false, nil
	}
	if len(messages) == 0 {
		return "", false, nil
	}
	var prompt string
	for _, message := range messages {
		prompt = message.Text
		if err := s.appendQueuedUserMessage(message); err != nil {
			return "", false, err
		}
	}
	s.emitQueueUpdate()
	return prompt, true, nil
}

func (s *AgentSession) appendQueuedUserMessage(message QueuedUserMessage) error {
	if message.Custom != nil {
		return s.appendCustomMessage(*message.Custom, true)
	}
	content := []llm.ContentPart{llm.Text(message.Text)}
	content = append(content, normalizePromptImages(message.Images)...)
	userMessage := llm.Message{Role: llm.RoleUser, Content: content, Timestamp: llm.NowMillis()}
	if err := s.emitMessageStart(userMessage); err != nil {
		return err
	}
	var err error
	userMessage, err = s.emitExtensionMessageEnd(userMessage)
	if err != nil {
		return err
	}
	s.SessionManager.AppendMessage(sessionMessageValue(userMessage))
	s.emit(AgentSessionEvent{Type: "message_end", Message: &userMessage})
	return nil
}

func (s *AgentSession) appendCustomMessage(message QueuedCustomMessage, includeInContext bool) error {
	if s == nil || s.SessionManager == nil {
		return errors.New("session manager is required")
	}
	s.SessionManager.AppendCustomMessageEntryWithContext(message.CustomType, message.Content, message.Display, message.Details, includeInContext)
	customMessage := llm.Message{Role: "custom", CustomType: message.CustomType, Content: []llm.ContentPart{llm.Text(customMessageText(message.Content))}, Timestamp: llm.NowMillis()}
	if err := s.emitMessageStart(customMessage); err != nil {
		return err
	}
	var err error
	customMessage, err = s.emitExtensionMessageEnd(customMessage)
	if err != nil {
		return err
	}
	s.emit(AgentSessionEvent{Type: "message_end", Message: &customMessage})
	return nil
}

func (s *AgentSession) emitAssistantMessageUpdates(message llm.Message) (llm.Message, error) {
	partial := message
	partial.Content = nil
	for index, part := range message.Content {
		switch part.Type {
		case llm.ContentThinking:
			partial.Content = append(partial.Content, llm.Thinking(""))
			if err := s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "thinking_start", ContentIndex: index, Partial: partial}); err != nil {
				return llm.Message{}, err
			}
			partial.Content[index].Thinking = part.Thinking
			if err := s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "thinking_delta", ContentIndex: index, Delta: part.Thinking, Partial: partial}); err != nil {
				return llm.Message{}, err
			}
			if err := s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "thinking_end", ContentIndex: index, Content: part.Thinking, Partial: partial}); err != nil {
				return llm.Message{}, err
			}
			if s.lifecycle.isAbortRequested() {
				return s.abortedAssistantMessage(partial)
			}
		case llm.ContentText:
			partial.Content = append(partial.Content, llm.Text(""))
			if err := s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "text_start", ContentIndex: index, Partial: partial}); err != nil {
				return llm.Message{}, err
			}
			partial.Content[index].Text = part.Text
			if err := s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "text_delta", ContentIndex: index, Delta: part.Text, Partial: partial}); err != nil {
				return llm.Message{}, err
			}
			if err := s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "text_end", ContentIndex: index, Content: part.Text, Partial: partial}); err != nil {
				return llm.Message{}, err
			}
			if s.lifecycle.isAbortRequested() {
				return s.abortedAssistantMessage(partial)
			}
		case llm.ContentToolCall:
			partial.Content = append(partial.Content, llm.ToolCall(part.ID, part.Name, map[string]any{}))
			if err := s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "toolcall_start", ContentIndex: index, Partial: partial}); err != nil {
				return llm.Message{}, err
			}
			argsJSON, _ := json.Marshal(part.Arguments)
			if err := s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "toolcall_delta", ContentIndex: index, Delta: string(argsJSON), Partial: partial}); err != nil {
				return llm.Message{}, err
			}
			partial.Content[index] = part
			if err := s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "toolcall_end", ContentIndex: index, ToolCall: part, Partial: partial}); err != nil {
				return llm.Message{}, err
			}
			if s.lifecycle.isAbortRequested() {
				return s.abortedAssistantMessage(partial)
			}
		}
	}
	if message.StopReason == llm.StopReasonError || message.StopReason == llm.StopReasonAborted {
		return message, nil
	}
	return message, nil
}

func (s *AgentSession) abortedAssistantMessage(partial llm.Message) (llm.Message, error) {
	s.lifecycle.resetAbort()
	partial.StopReason = llm.StopReasonAborted
	if partial.ErrorMessage == "" {
		partial.ErrorMessage = "aborted"
	}
	if err := s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "error", Reason: llm.StopReasonAborted, Error: partial}); err != nil {
		return llm.Message{}, err
	}
	return partial, nil
}

func (s *AgentSession) emitAssistantMessageEvent(event llm.AssistantMessageEvent) error {
	message := assistantMessageEventMessage(event)
	if message.Role == "" {
		message.Role = llm.RoleAssistant
	}
	if s != nil && s.ExtensionRuntime != nil {
		eventCopy := event
		messageCopy := message
		if _, err := s.ExtensionRuntime.EmitSessionEvent(ProtocolSessionEvent{
			Type:                  ProtocolEventMessageUpdate,
			Message:               &messageCopy,
			AssistantMessageEvent: &eventCopy,
		}); err != nil {
			return err
		}
	}
	eventCopy := event
	messageCopy := message
	s.emit(AgentSessionEvent{Type: "message_update", Message: &messageCopy, AssistantMessageEvent: &eventCopy})
	return nil
}

func assistantMessageEventMessage(event llm.AssistantMessageEvent) llm.Message {
	switch event.Type {
	case "done":
		return event.Message
	case "error":
		return event.Error
	default:
		return event.Partial
	}
}

func (s *AgentSession) normalizeAssistantMessage(message llm.Message) llm.Message {
	if message.Role == "" {
		message.Role = llm.RoleAssistant
	}
	if message.Timestamp == 0 {
		message.Timestamp = llm.NowMillis()
	}
	if message.Provider == "" {
		message.Provider = s.Agent.State.Model.Provider
	}
	if message.Model == "" {
		message.Model = s.Agent.State.Model.ID
	}
	if message.API == "" {
		message.API = s.Agent.State.Model.API
	}
	if message.StopReason == "" {
		message.StopReason = llm.StopReasonStop
	}
	return message
}

func (s *AgentSession) executeAssistantToolCalls(message llm.Message) error {
	var toolCalls []llm.ContentPart
	for _, part := range message.Content {
		if part.Type != llm.ContentToolCall {
			continue
		}
		toolCalls = append(toolCalls, part)
	}
	blockedResults := map[string]*llm.Message{}
	for _, part := range toolCalls {
		blocked, err := s.emitToolCallEvent(part)
		if err != nil {
			return err
		}
		if blocked != nil {
			blockedResults[part.ID] = blocked
		}
	}
	for _, part := range toolCalls {
		if blocked := blockedResults[part.ID]; blocked != nil {
			s.emit(AgentSessionEvent{Type: "tool_execution_start", ToolCallID: part.ID, ToolName: part.Name, Args: part.Arguments})
			blockedCopy := *blocked
			s.emit(AgentSessionEvent{Type: "tool_execution_end", ToolCallID: part.ID, ToolName: part.Name, Args: part.Arguments, ToolResult: &blockedCopy})
			if err := s.appendToolResultMessage(*blocked); err != nil {
				return err
			}
			continue
		}
		tool := s.sdkTool(part.Name)
		if tool == nil || (tool.Execute == nil && tool.ExecuteWithUpdates == nil) {
			continue
		}
		s.emit(AgentSessionEvent{Type: "tool_execution_start", ToolCallID: part.ID, ToolName: part.Name, Args: part.Arguments})
		result, err := s.executeSDKToolWithUpdates(tool, part)
		isError := err != nil
		toolResult := sdkToolResultMessage(result, part, isError)
		if err != nil {
			toolResult.Content = []llm.ContentPart{llm.Text(err.Error())}
		}
		toolResult, err = s.applyToolResultEvent(toolResult)
		if err != nil {
			return err
		}
		toolResultCopy := toolResult
		s.emit(AgentSessionEvent{Type: "tool_execution_end", ToolCallID: part.ID, ToolName: part.Name, Args: part.Arguments, ToolResult: &toolResultCopy})
		if err := s.appendToolResultMessage(toolResult); err != nil {
			return err
		}
	}
	return nil
}

func (s *AgentSession) executeSDKToolWithUpdates(tool *SDKTool, part llm.ContentPart) (SDKToolResult, error) {
	input := prepareSDKToolArguments(tool, part.Arguments)
	if tool.ExecuteWithUpdates == nil {
		return tool.Execute(part.ID, input)
	}
	return tool.ExecuteWithUpdates(part.ID, input, func(partial SDKToolResult) {
		partialMessage := sdkToolResultMessage(partial, part, false)
		_ = s.emitToolExecutionUpdateEvent(partialMessage)
		s.emit(AgentSessionEvent{
			Type:              ProtocolEventToolExecutionUpdate,
			ToolCallID:        part.ID,
			ToolName:          part.Name,
			Args:              part.Arguments,
			PartialToolResult: &partialMessage,
		})
	})
}

func prepareSDKToolArguments(tool *SDKTool, input map[string]any) map[string]any {
	if tool == nil || tool.PrepareArguments == nil {
		return input
	}
	prepared := tool.PrepareArguments(input)
	if prepared == nil {
		return input
	}
	return prepared
}

func sdkToolResultMessage(result SDKToolResult, part llm.ContentPart, isError bool) llm.Message {
	return llm.Message{
		Role:       llm.RoleToolResult,
		Content:    sdkContentToLLMContent(result.Content),
		ToolCallID: part.ID,
		ToolName:   part.Name,
		Timestamp:  llm.NowMillis(),
		Details:    result.Details,
		IsError:    isError,
	}
}

func (s *AgentSession) emitToolCallEvent(part llm.ContentPart) (*llm.Message, error) {
	if s == nil || s.ExtensionRuntime == nil {
		return nil, nil
	}
	result, err := s.ExtensionRuntime.EmitSessionEvent(ProtocolSessionEvent{
		Type:       ProtocolEventToolCall,
		ToolName:   part.Name,
		ToolCallID: part.ID,
		Input:      part.Arguments,
	})
	if err != nil {
		return nil, err
	}
	if !result.Block {
		return nil, nil
	}
	reason := strings.TrimSpace(result.Reason)
	if reason == "" {
		reason = "Tool execution blocked by extension"
	}
	return &llm.Message{
		Role:       llm.RoleToolResult,
		Content:    []llm.ContentPart{llm.Text(reason)},
		ToolCallID: part.ID,
		ToolName:   part.Name,
		Timestamp:  llm.NowMillis(),
		IsError:    true,
	}, nil
}

func (s *AgentSession) applyToolResultEvent(message llm.Message) (llm.Message, error) {
	if s == nil || s.ExtensionRuntime == nil || !s.ExtensionRuntime.HasHandlers(ProtocolEventToolResult) {
		return message, nil
	}
	result, err := s.ExtensionRuntime.EmitSessionEvent(ProtocolSessionEvent{
		Type:       ProtocolEventToolResult,
		ToolName:   message.ToolName,
		ToolCallID: message.ToolCallID,
		Content:    llmContentToSDKContent(message.Content),
		Details:    message.Details,
		IsError:    message.IsError,
	})
	if err != nil {
		return llm.Message{}, err
	}
	if result.ContentSet {
		message.Content = sdkContentToLLMContent(result.Content)
	}
	if result.DetailsSet {
		message.Details = result.Details
	}
	if result.IsErrorSet {
		message.IsError = result.IsError
	}
	return message, nil
}

func (s *AgentSession) emitToolExecutionUpdateEvent(message llm.Message) error {
	if s == nil || s.ExtensionRuntime == nil || !s.ExtensionRuntime.HasHandlers(ProtocolEventToolExecutionUpdate) {
		return nil
	}
	_, err := s.ExtensionRuntime.EmitSessionEvent(ProtocolSessionEvent{
		Type:       ProtocolEventToolExecutionUpdate,
		ToolName:   message.ToolName,
		ToolCallID: message.ToolCallID,
		Content:    llmContentToSDKContent(message.Content),
		Details:    message.Details,
		IsError:    message.IsError,
	})
	return err
}

func sdkContentToLLMContent(content []SDKContentPart) []llm.ContentPart {
	if len(content) == 0 {
		return []llm.ContentPart{llm.Text("")}
	}
	result := make([]llm.ContentPart, 0, len(content))
	for _, part := range content {
		switch part.Type {
		case llm.ContentText, "":
			result = append(result, llm.Text(part.Text))
		}
	}
	if len(result) == 0 {
		return []llm.ContentPart{llm.Text("")}
	}
	return result
}

func llmContentToSDKContent(content []llm.ContentPart) []SDKContentPart {
	result := make([]SDKContentPart, 0, len(content))
	for _, part := range content {
		if part.Type == llm.ContentText {
			result = append(result, SDKContentPart{Type: "text", Text: part.Text})
		}
	}
	return result
}

func (s *AgentSession) emitExtensionEvent(event ProtocolSessionEvent) error {
	if s == nil || s.ExtensionRuntime == nil {
		return nil
	}
	_, err := s.ExtensionRuntime.EmitSessionEvent(event)
	return err
}

func (s *AgentSession) emitBeforeProviderRequest(ctx context.Context, payload any, model llm.Model) (any, bool, error) {
	if s == nil || s.ExtensionRuntime == nil {
		return nil, false, nil
	}
	result, err := s.ExtensionRuntime.EmitSessionEvent(ProtocolSessionEvent{
		Context: ctx,
		Type:    ProtocolEventBeforeProviderRequest,
		Model:   &model,
		Payload: payload,
	})
	if err != nil {
		return nil, false, err
	}
	if result.PayloadSet {
		return result.Payload, true, nil
	}
	return nil, false, nil
}

func (s *AgentSession) emitBeforeProviderHeaders(
	ctx context.Context,
	headers map[string]string,
	model llm.Model,
) map[string]string {
	if s == nil || s.ExtensionRuntime == nil {
		return cloneStringMap(headers)
	}
	return s.ExtensionRuntime.EmitBeforeProviderHeaders(ctx, headers, &model)
}

func (s *AgentSession) emitAfterProviderResponse(ctx context.Context, status int, headers map[string]string, model llm.Model) error {
	if s == nil || s.ExtensionRuntime == nil {
		return nil
	}
	_, err := s.ExtensionRuntime.EmitSessionEvent(ProtocolSessionEvent{
		Context: ctx,
		Type:    ProtocolEventAfterProviderResponse,
		Model:   &model,
		Status:  status,
		Headers: cloneStringMap(headers),
	})
	return err
}

func (s *AgentSession) emitMessageStart(message llm.Message) error {
	if s == nil {
		return nil
	}
	if message.Role == llm.RoleUser {
		s.overflowRecovered = false
	}
	if s.ExtensionRuntime != nil {
		if _, err := s.ExtensionRuntime.EmitSessionEvent(ProtocolSessionEvent{
			Type:    ProtocolEventMessageStart,
			Role:    message.Role,
			Message: &message,
		}); err != nil {
			return err
		}
	}
	s.emit(AgentSessionEvent{Type: "message_start", Message: &message})
	return nil
}

func (s *AgentSession) emitExtensionMessageEnd(message llm.Message) (llm.Message, error) {
	if s == nil || s.ExtensionRuntime == nil {
		return normalizeSessionMessageContent(message), nil
	}
	result, err := s.ExtensionRuntime.EmitSessionEvent(ProtocolSessionEvent{
		Type:    ProtocolEventMessageEnd,
		Role:    message.Role,
		Message: &message,
	})
	if err != nil {
		return llm.Message{}, err
	}
	if result.MessageSet && result.Message != nil {
		return normalizeSessionMessageContent(*result.Message), nil
	}
	return normalizeSessionMessageContent(message), nil
}

func (s *AgentSession) appendToolResultMessage(message llm.Message) error {
	if err := s.emitMessageStart(message); err != nil {
		return err
	}
	var err error
	message, err = s.emitExtensionMessageEnd(message)
	if err != nil {
		return err
	}
	s.SessionManager.AppendMessage(sessionMessageValue(message))
	s.emit(AgentSessionEvent{Type: "message_end", Message: &message})
	return nil
}

func (s *AgentSession) sdkTool(name string) *SDKTool {
	if s == nil || s.Agent == nil {
		return nil
	}
	tools := s.GetActiveTools()
	for index := range tools {
		if tools[index].Name == name {
			return &tools[index]
		}
	}
	return nil
}

func (s *AgentSession) IsRetrying() bool {
	if s == nil {
		return false
	}
	return s.lifecycle.isActive(agentSessionActivityRetrying)
}

func (s *AgentSession) IsStreaming() bool {
	if s == nil {
		return false
	}
	return s.lifecycle.isActive(agentSessionActivityStreaming)
}

func (s *AgentSession) IsIdle() bool {
	if s == nil {
		return true
	}
	return s.lifecycle.isIdle()
}

func (s *AgentSession) WaitForIdle(ctx context.Context) error {
	if s == nil {
		return nil
	}
	return s.lifecycle.waitForIdle(ctx)
}

func (s *AgentSession) Abort() error {
	if s == nil {
		return nil
	}
	s.lifecycle.requestAbort()
	s.AbortBranchSummary()
	s.AbortBash()
	s.AbortRetry()
	s.AbortCompaction()
	return nil
}

func (s *AgentSession) SetSessionName(name string) error {
	if s == nil || s.SessionManager == nil {
		return errors.New("session manager is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("Session name cannot be empty")
	}
	s.SessionManager.AppendSessionInfo(name)
	if err := s.emitExtensionEvent(ProtocolSessionEvent{Type: ProtocolEventSessionInfoChanged, Name: name}); err != nil {
		return err
	}
	s.emit(AgentSessionEvent{Type: ProtocolEventSessionInfoChanged, Name: name})
	return nil
}

func (s *AgentSession) AbortRetry() {
	if s == nil {
		return
	}
	if s.lifecycle.cancel(agentSessionCancellationRetry) {
		return
	}
	s.lifecycle.setActivity(agentSessionActivityRetrying, false)
}

func (s *AgentSession) AbortCompaction() {
	if s == nil {
		return
	}
	s.lifecycle.cancel(agentSessionCancellationCompaction)
}

func (s *AgentSession) Steer(text string) error {
	return s.SteerWithImages(text, nil)
}

func (s *AgentSession) SteerWithImages(text string, images []llm.ContentPart) error {
	if s == nil {
		return errors.New("session is required")
	}
	message, err := s.prepareQueuedUserMessage(text, images, "steering")
	if err != nil {
		return err
	}
	s.queues.enqueuePrompt(agentSessionSteeringQueue, message)
	s.emitQueueUpdate()
	return nil
}

func (s *AgentSession) FollowUp(text string) error {
	return s.FollowUpWithImages(text, nil)
}

func (s *AgentSession) FollowUpWithImages(text string, images []llm.ContentPart) error {
	if s == nil {
		return errors.New("session is required")
	}
	message, err := s.prepareQueuedUserMessage(text, images, "follow-up")
	if err != nil {
		return err
	}
	s.queues.enqueuePrompt(agentSessionFollowUpQueue, message)
	s.emitQueueUpdate()
	return nil
}

func (s *AgentSession) QueueExtensionUserMessage(text, deliverAs string) error {
	if s == nil {
		return errors.New("session is required")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("extension message is required")
	}
	message := QueuedUserMessage{Text: text}
	switch deliverAs {
	case "followUp":
		s.queues.enqueuePrompt(agentSessionFollowUpQueue, message)
	default:
		s.queues.enqueuePrompt(agentSessionSteeringQueue, message)
	}
	s.emitQueueUpdate()
	return nil
}

func (s *AgentSession) SendCustomMessage(message QueuedCustomMessage, options ProtocolSendCustomMessageOptions) (returnErr error) {
	if s == nil {
		return errors.New("session is required")
	}
	s.SyncRuntimeSettings()
	if strings.TrimSpace(message.CustomType) == "" {
		return errors.New("custom message type is required")
	}
	text := customMessageText(message.Content)
	if options.DeliverAs == "nextTurn" {
		s.queues.enqueueNextTurn(message)
		return nil
	}
	if s.IsStreaming() {
		queued := QueuedUserMessage{Text: text, Custom: &message}
		switch options.DeliverAs {
		case "followUp":
			s.queues.enqueuePrompt(agentSessionFollowUpQueue, queued)
		default:
			s.queues.enqueuePrompt(agentSessionSteeringQueue, queued)
		}
		s.emitQueueUpdate()
		return nil
	}
	if options.TriggerTurn {
		if !s.lifecycle.tryStartStreaming() {
			return errors.New("Agent is already processing. Specify deliverAs ('steer' or 'followUp') to queue the message.")
		}
		s.lifecycle.resetAbort()
		defer func() {
			returnErr = errors.Join(returnErr, s.settleAgentRun())
		}()
		if err := s.emitAgentLifecycleEvent(AgentSessionEvent{Type: "agent_start"}); err != nil {
			return err
		}
		if err := s.emitAgentLifecycleEvent(AgentSessionEvent{Type: "turn_start"}); err != nil {
			return err
		}
		if err := s.appendCustomMessage(message, true); err != nil {
			return err
		}
		return s.runPromptLoop(text)
	}
	return s.appendCustomMessage(message, false)
}

func (s *AgentSession) prepareQueuedUserMessage(text string, images []llm.ContentPart, label string) (QueuedUserMessage, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return QueuedUserMessage{}, errors.New(label + " message is required")
	}
	if err := s.rejectQueuedExtensionCommand(text); err != nil {
		return QueuedUserMessage{}, err
	}
	return QueuedUserMessage{Text: s.expandPromptCommands(text), Images: normalizePromptImages(images)}, nil
}

func (s *AgentSession) rejectQueuedExtensionCommand(text string) error {
	if s == nil || s.ExtensionRuntime == nil || !strings.HasPrefix(text, "/") {
		return nil
	}
	name, _, ok := parseSlashCommandInvocation(text)
	if !ok {
		return nil
	}
	if command := s.ExtensionRuntime.GetCommand(name); command != nil {
		return errors.New(`Extension command "/` + name + `" cannot be queued. Use prompt() or execute the command when not streaming.`)
	}
	return nil
}

func (s *AgentSession) PendingMessageCount() int {
	if s == nil {
		return 0
	}
	return s.queues.pendingPromptCount()
}

func (s *AgentSession) GetSteeringMessages() []string {
	if s == nil {
		return nil
	}
	return s.queues.promptMessages(agentSessionSteeringQueue)
}

func (s *AgentSession) GetFollowUpMessages() []string {
	if s == nil {
		return nil
	}
	return s.queues.promptMessages(agentSessionFollowUpQueue)
}

func (s *AgentSession) GetSteeringQueue() []QueuedUserMessage {
	if s == nil {
		return nil
	}
	return s.queues.promptQueue(agentSessionSteeringQueue)
}

func (s *AgentSession) GetFollowUpQueue() []QueuedUserMessage {
	if s == nil {
		return nil
	}
	return s.queues.promptQueue(agentSessionFollowUpQueue)
}

func (s *AgentSession) ClearQueue() (steering, followUp []string) {
	if s == nil {
		return nil, nil
	}
	steering, followUp = s.queues.clearPrompts()
	s.emitQueueUpdate()
	return steering, followUp
}

func (s *AgentSession) emitQueueUpdate() {
	steering, followUp := s.queues.promptSnapshot()
	s.emit(AgentSessionEvent{
		Type:     "queue_update",
		Steering: steering,
		FollowUp: followUp,
	})
}

func DefaultAgentSessionResponder(prompt string, context []llm.Message, model llm.Model) (llm.Message, error) {
	response := "OK"
	if prompt != "" {
		response = "Response to: " + prompt
	}
	usage := len([]rune(prompt))/4 + len(context)*8 + 32
	return llm.Message{
		Role:       llm.RoleAssistant,
		Content:    []llm.ContentPart{llm.Text(response)},
		API:        model.API,
		Provider:   model.Provider,
		Model:      model.ID,
		Usage:      llm.Usage{Input: usage, Output: 8, TotalTokens: usage + 8},
		StopReason: llm.StopReasonStop,
		Timestamp:  llm.NowMillis(),
	}, nil
}

func (s *AgentSession) Compact(customInstructions ...string) (agentharness.CompactionResult, error) {
	if s == nil || s.SessionManager == nil {
		return agentharness.CompactionResult{}, errors.New("session manager is required")
	}
	s.SyncRuntimeSettings()
	if s.Agent == nil || strings.TrimSpace(s.Agent.State.Model.ID) == "" {
		return agentharness.CompactionResult{}, errors.New("No model selected")
	}
	if s.Preflight != nil {
		if err := s.Preflight(s.Agent.State.Model); err != nil {
			return agentharness.CompactionResult{}, err
		}
	}
	instructions := ""
	if len(customInstructions) > 0 {
		instructions = customInstructions[0]
	}
	ctx, cancel := context.WithCancel(context.Background())
	cleanupCancellation, started := s.lifecycle.tryStartExclusiveCancellableActivity(
		agentSessionActivityCompacting,
		agentSessionCancellationCompaction,
		cancel,
	)
	if !started {
		cancel()
		return agentharness.CompactionResult{}, errors.New("session is busy")
	}
	defer func() {
		cleanupCancellation()
		cancel()
	}()
	s.emit(AgentSessionEvent{Type: "compaction_start", Reason: "manual"})
	result, err := s.compactSession(
		ctx,
		instructions,
		"manual",
		false,
	)
	s.lifecycle.beginActivitySettlement(agentSessionActivityCompacting)
	defer s.lifecycle.finishSettlement()
	if err != nil {
		aborted := isCompactionCancelledError(err)
		errorMessage := "Compaction failed: " + err.Error()
		if aborted {
			errorMessage = errCompactionCancelled.Error()
		}
		s.emit(AgentSessionEvent{
			Type:         "compaction_end",
			Reason:       "manual",
			Aborted:      aborted,
			WillRetry:    false,
			ErrorMessage: errorMessage,
		})
		return agentharness.CompactionResult{}, err
	}
	s.emit(AgentSessionEvent{
		Type:      "compaction_end",
		Reason:    "manual",
		Result:    &result,
		Aborted:   false,
		WillRetry: false,
	})
	return result, nil
}

func (s *AgentSession) compactSession(
	ctx context.Context,
	customInstructions string,
	reason string,
	willRetry bool,
) (agentharness.CompactionResult, error) {
	branch := s.SessionManager.GetBranch()
	if len(branch) == 0 {
		return agentharness.CompactionResult{}, errors.New("Nothing to compact (session too small)")
	}
	if branch[len(branch)-1].Type == "compaction" {
		return agentharness.CompactionResult{}, errors.New("Already compacted")
	}
	preparation, err := agentharness.PrepareCompaction(fileEntriesToHarnessEntries(branch), s.CompactionSettings)
	if err != nil {
		return agentharness.CompactionResult{}, err
	}
	if preparation == nil {
		return agentharness.CompactionResult{}, errors.New("Nothing to compact (session too small)")
	}
	result, fromExtension, err := s.resolveCompactionResult(
		ctx,
		*preparation,
		branch,
		customInstructions,
		reason,
		willRetry,
	)
	if err != nil {
		return agentharness.CompactionResult{}, err
	}
	if strings.TrimSpace(result.Summary) == "" {
		return agentharness.CompactionResult{}, errors.New("compaction summary is empty")
	}
	if result.FirstKeptEntryID == "" {
		result.FirstKeptEntryID = preparation.FirstKeptEntryID
	}
	if result.TokensBefore == 0 {
		result.TokensBefore = preparation.TokensBefore
	}
	entryID := s.SessionManager.AppendCompactionWithOptions(
		result.Summary,
		result.FirstKeptEntryID,
		result.TokensBefore,
		SessionSummaryOptions{
			Details:  result.Details,
			FromHook: fromExtension,
			Usage:    result.Usage,
		},
	)
	if entry := s.SessionManager.GetEntry(entryID); entry != nil {
		_ = s.emitExtensionEvent(ProtocolSessionEvent{
			Type:            "session_compact",
			Reason:          reason,
			WillRetry:       willRetry,
			CompactionEntry: entry,
			FromExtension:   fromExtension,
		})
	}
	result.EstimatedTokensAfter = estimateMessagesTokens(s.Messages())
	return result, nil
}

func (s *AgentSession) resolveCompactionResult(
	ctx context.Context,
	preparation agentharness.CompactionPreparation,
	branch []FileEntry,
	customInstructions string,
	reason string,
	willRetry bool,
) (agentharness.CompactionResult, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.ExtensionRuntime != nil {
		event := ProtocolSessionEvent{
			Context:            ctx,
			Type:               "session_before_compact",
			Reason:             reason,
			WillRetry:          willRetry,
			Preparation:        &preparation,
			BranchEntries:      append([]FileEntry(nil), branch...),
			CustomInstructions: customInstructions,
		}
		result, err := s.ExtensionRuntime.EmitSessionEvent(event)
		if err == nil {
			if result.Cancel || errors.Is(ctx.Err(), context.Canceled) {
				return agentharness.CompactionResult{}, false, errCompactionCancelled
			}
			if result.Compaction != nil {
				return *result.Compaction, true, nil
			}
		}
	}
	summarizer := s.CompactionSummarizer
	if summarizer != nil {
		result, err := summarizer(preparation, customInstructions)
		if errors.Is(ctx.Err(), context.Canceled) {
			return agentharness.CompactionResult{}, false, errCompactionCancelled
		}
		return result, false, err
	}
	result, err := s.generateCompactionSummary(
		ctx,
		preparation,
		customInstructions,
		reason,
	)
	if errors.Is(ctx.Err(), context.Canceled) {
		return agentharness.CompactionResult{}, false, errCompactionCancelled
	}
	return result, false, err
}

func estimateMessagesTokens(messages []llm.Message) int {
	total := 0
	for _, message := range messages {
		total += agentharness.EstimateTokens(message)
	}
	return total
}

func DefaultAgentSessionCompactionSummarizer(preparation agentharness.CompactionPreparation, customInstructions string) (agentharness.CompactionResult, error) {
	sections := []string{}
	if strings.TrimSpace(preparation.PreviousSummary) != "" {
		sections = append(sections, "Previous summary:\n"+strings.TrimSpace(preparation.PreviousSummary))
	}
	if strings.TrimSpace(customInstructions) != "" {
		sections = append(sections, "Focus:\n"+strings.TrimSpace(customInstructions))
	}
	if len(preparation.MessagesToSummarize) > 0 {
		sections = append(sections, "Conversation:\n"+agentharness.SerializeConversation(preparation.MessagesToSummarize))
	} else {
		sections = append(sections, "No prior history.")
	}
	if len(preparation.TurnPrefixMessages) > 0 {
		sections = append(sections, "Turn context:\n"+agentharness.SerializeConversation(preparation.TurnPrefixMessages))
	}
	return agentharness.CompactionResult{
		Summary:          strings.Join(sections, "\n\n"),
		FirstKeptEntryID: preparation.FirstKeptEntryID,
		TokensBefore:     preparation.TokensBefore,
		Details: map[string]any{
			"readFiles":     sortedMapKeys(preparation.FileOps.Read),
			"writtenFiles":  sortedMapKeys(preparation.FileOps.Written),
			"modifiedFiles": sortedMapKeys(preparation.FileOps.Edited),
		},
	}, nil
}

func fileEntriesToHarnessEntries(entries []FileEntry) []agentharness.Entry {
	converted := make([]agentharness.Entry, 0, len(entries))
	for _, entry := range entries {
		harnessEntry := agentharness.Entry{
			Type:             entry.Type,
			ID:               entry.ID,
			ParentID:         cloneStringPtr(entry.ParentID),
			Timestamp:        entry.Timestamp,
			ThinkingLevel:    entry.ThinkingLevel,
			Provider:         entry.Provider,
			ModelID:          entry.ModelID,
			Summary:          entry.Summary,
			FirstKeptEntryID: entry.FirstKeptID,
			TokensBefore:     entry.TokensBefore,
			FromID:           entry.FromID,
			CustomType:       entry.CustomType,
			Content:          entry.Content,
			Display:          entry.Display,
			Details:          entry.Details,
			FromHook:         entry.FromHook,
			Usage:            cloneSessionUsage(entry.Usage),
			Name:             entry.Name,
		}
		if entry.TargetID != "" {
			harnessEntry.TargetID = stringPtr(entry.TargetID)
		}
		if entry.Label != "" {
			harnessEntry.Label = stringPtr(entry.Label)
		}
		if entry.Type == "message" {
			message, ok := sessionMessageToLLM(entry.Message)
			if !ok {
				continue
			}
			harnessEntry.Message = message
		}
		converted = append(converted, harnessEntry)
	}
	return converted
}

func sessionUserMessageValue(text string) map[string]any {
	return map[string]any{
		"role":      llm.RoleUser,
		"content":   []any{map[string]any{"type": llm.ContentText, "text": text}},
		"timestamp": llm.NowMillis(),
	}
}

func sessionMessageValue(message llm.Message) map[string]any {
	content := make([]any, 0, len(message.Content))
	for _, part := range message.Content {
		switch part.Type {
		case llm.ContentText:
			block := map[string]any{"type": llm.ContentText, "text": part.Text}
			if part.TextSignature != "" {
				block["textSignature"] = part.TextSignature
			}
			content = append(content, block)
		case llm.ContentThinking:
			block := map[string]any{"type": llm.ContentThinking, "thinking": part.Thinking}
			if part.ThinkingSignature != "" {
				block["thinkingSignature"] = part.ThinkingSignature
			}
			if part.Redacted {
				block["redacted"] = true
			}
			content = append(content, block)
		case llm.ContentImage:
			content = append(content, map[string]any{"type": llm.ContentImage, "data": part.Data, "mimeType": part.MIMEType})
		case llm.ContentToolCall:
			block := map[string]any{"type": llm.ContentToolCall, "id": part.ID, "name": part.Name, "arguments": part.Arguments}
			if part.ThoughtSignature != "" {
				block["thoughtSignature"] = part.ThoughtSignature
			}
			content = append(content, block)
		}
	}
	if message.Timestamp == 0 {
		message.Timestamp = llm.NowMillis()
	}
	value := map[string]any{
		"role":       message.Role,
		"content":    content,
		"timestamp":  message.Timestamp,
		"api":        message.API,
		"provider":   message.Provider,
		"model":      message.Model,
		"usage":      sessionUsageValue(message.Usage),
		"stopReason": message.StopReason,
	}
	if message.ResponseModel != "" {
		value["responseModel"] = message.ResponseModel
	}
	if message.ErrorMessage != "" {
		value["errorMessage"] = message.ErrorMessage
	}
	if message.ToolCallID != "" {
		value["toolCallID"] = message.ToolCallID
	}
	if message.ToolName != "" {
		value["toolName"] = message.ToolName
	}
	if message.CustomType != "" {
		value["customType"] = message.CustomType
	}
	if message.Display != nil {
		value["display"] = *message.Display
	}
	if message.Details != nil {
		value["details"] = message.Details
	}
	if message.IsError {
		value["isError"] = message.IsError
	}
	return value
}

func sessionUsageValue(usage llm.Usage) map[string]any {
	value := map[string]any{
		"input":       usage.Input,
		"output":      usage.Output,
		"cacheRead":   usage.CacheRead,
		"cacheWrite":  usage.CacheWrite,
		"totalTokens": usage.TotalTokens,
		"cost": map[string]any{
			"input":      usage.Cost.Input,
			"output":     usage.Cost.Output,
			"cacheRead":  usage.Cost.CacheRead,
			"cacheWrite": usage.Cost.CacheWrite,
			"total":      usage.Cost.Total,
		},
	}
	if usage.CacheWrite1h != 0 {
		value["cacheWrite1h"] = usage.CacheWrite1h
	}
	if usage.Reasoning != nil {
		value["reasoning"] = *usage.Reasoning
	}
	return value
}

func sortedMapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key, ok := range values {
		if ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}
