package gicodingagent

import (
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

type AgentSessionEvent struct {
	Type                  string
	Reason                string
	Result                *agentharness.CompactionResult
	Aborted               bool
	WillRetry             bool
	ErrorMessage          string
	Message               *llm.Message
	Attempt               int
	MaxAttempts           int
	DelayMs               int
	Success               bool
	FinalError            string
	Steering              []string
	FollowUp              []string
	AssistantMessageEvent *llm.AssistantMessageEvent
}

type AgentSessionEventListener func(AgentSessionEvent)

type AgentSessionResponder func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error)

type AgentSessionCompactionSummarizer func(preparation agentharness.CompactionPreparation, customInstructions string) (agentharness.CompactionResult, error)

type AgentSessionRetrySettings struct {
	Enabled     bool
	MaxRetries  int
	BaseDelayMs int
}

func DefaultAgentSessionRetrySettings() AgentSessionRetrySettings {
	return AgentSessionRetrySettings{Enabled: false, MaxRetries: 0, BaseDelayMs: 0}
}

func (s *AgentSession) Subscribe(listener AgentSessionEventListener) func() {
	if s == nil || listener == nil {
		return func() {}
	}
	s.eventListeners = append(s.eventListeners, listener)
	return func() {
		target := reflect.ValueOf(listener).Pointer()
		for index, candidate := range s.eventListeners {
			if reflect.ValueOf(candidate).Pointer() != target {
				continue
			}
			s.eventListeners = append(s.eventListeners[:index], s.eventListeners[index+1:]...)
			return
		}
	}
}

func (s *AgentSession) emit(event AgentSessionEvent) {
	if s == nil || len(s.eventListeners) == 0 {
		return
	}
	listeners := append([]AgentSessionEventListener(nil), s.eventListeners...)
	for _, listener := range listeners {
		listener(event)
	}
}

func (s *AgentSession) Prompt(text string) error {
	if s == nil || s.SessionManager == nil {
		return errors.New("session manager is required")
	}
	if s.isStreaming {
		return errors.New("Agent is already processing. Specify streamingBehavior ('steer' or 'followUp') to queue the message.")
	}
	prompt := strings.TrimSpace(text)
	if prompt == "" {
		return errors.New("prompt is required")
	}
	s.isStreaming = true
	defer func() {
		s.isStreaming = false
	}()
	userMessage := llm.UserMessageText(prompt)
	s.SessionManager.AppendMessage(sessionMessageValue(userMessage))
	s.emit(AgentSessionEvent{Type: "message_end", Message: &userMessage})
	s.emit(AgentSessionEvent{Type: "agent_start"})
	err := s.runPromptLoop(prompt)
	s.emit(AgentSessionEvent{Type: "agent_end"})
	return err
}

func (s *AgentSession) runPromptLoop(prompt string) error {
	responder := s.Responder
	if responder == nil {
		responder = DefaultAgentSessionResponder
	}
	attempt := 0
	retried := false
	for {
		assistant, err := responder(prompt, s.Messages(), s.Agent.State.Model)
		if err != nil {
			assistant = llm.Message{Role: llm.RoleAssistant, StopReason: llm.StopReasonError, ErrorMessage: err.Error()}
		}
		assistant = s.normalizeAssistantMessage(assistant)
		s.emitAssistantMessageUpdates(assistant)
		if isRetryableAssistantError(assistant) && s.RetrySettings.Enabled && attempt < s.RetrySettings.MaxRetries {
			attempt++
			retried = true
			s.isRetrying = true
			s.emit(AgentSessionEvent{Type: "auto_retry_start", Attempt: attempt, MaxAttempts: s.RetrySettings.MaxRetries, DelayMs: s.RetrySettings.BaseDelayMs, ErrorMessage: assistant.ErrorMessage})
			continue
		}
		s.SessionManager.AppendMessage(sessionMessageValue(assistant))
		s.emit(AgentSessionEvent{Type: "message_end", Message: &assistant})
		if isRetryableAssistantError(assistant) {
			if retried {
				s.emit(AgentSessionEvent{Type: "auto_retry_end", Success: false, Attempt: attempt, FinalError: assistant.ErrorMessage})
			}
			s.isRetrying = false
			return nil
		}
		if retried {
			s.emit(AgentSessionEvent{Type: "auto_retry_end", Success: true, Attempt: attempt})
			s.isRetrying = false
		}
		if assistant.StopReason != "toolUse" {
			return nil
		}
		if err := s.executeAssistantToolCalls(assistant); err != nil {
			return err
		}
	}
}

func (s *AgentSession) emitAssistantMessageUpdates(message llm.Message) {
	partial := message
	partial.Content = nil
	s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "start", Partial: partial})
	for index, part := range message.Content {
		switch part.Type {
		case llm.ContentThinking:
			partial.Content = append(partial.Content, llm.Thinking(""))
			s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "thinking_start", ContentIndex: index, Partial: partial})
			partial.Content[index].Thinking = part.Thinking
			s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "thinking_delta", ContentIndex: index, Delta: part.Thinking, Partial: partial})
			s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "thinking_end", ContentIndex: index, Content: part.Thinking, Partial: partial})
		case llm.ContentText:
			partial.Content = append(partial.Content, llm.Text(""))
			s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "text_start", ContentIndex: index, Partial: partial})
			partial.Content[index].Text = part.Text
			s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "text_delta", ContentIndex: index, Delta: part.Text, Partial: partial})
			s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "text_end", ContentIndex: index, Content: part.Text, Partial: partial})
		case llm.ContentToolCall:
			partial.Content = append(partial.Content, llm.ToolCall(part.ID, part.Name, map[string]any{}))
			s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "toolcall_start", ContentIndex: index, Partial: partial})
			argsJSON, _ := json.Marshal(part.Arguments)
			s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "toolcall_delta", ContentIndex: index, Delta: string(argsJSON), Partial: partial})
			partial.Content[index] = part
			s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "toolcall_end", ContentIndex: index, ToolCall: part, Partial: partial})
		}
	}
	if message.StopReason == llm.StopReasonError || message.StopReason == llm.StopReasonAborted {
		s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "error", Reason: message.StopReason, Error: message})
		return
	}
	s.emitAssistantMessageEvent(llm.AssistantMessageEvent{Type: "done", Reason: message.StopReason, Message: message})
}

func (s *AgentSession) emitAssistantMessageEvent(event llm.AssistantMessageEvent) {
	s.emit(AgentSessionEvent{Type: "message_update", AssistantMessageEvent: &event})
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
	for _, part := range message.Content {
		if part.Type != llm.ContentToolCall {
			continue
		}
		tool := s.sdkTool(part.Name)
		if tool == nil || tool.Execute == nil {
			continue
		}
		result, err := tool.Execute(part.ID, part.Arguments)
		text := ""
		for _, content := range result.Content {
			if content.Type == "text" {
				text += content.Text
			}
		}
		toolResult := llm.Message{
			Role:       llm.RoleToolResult,
			Content:    []llm.ContentPart{llm.Text(text)},
			ToolCallID: part.ID,
			ToolName:   part.Name,
			Timestamp:  llm.NowMillis(),
			IsError:    err != nil,
		}
		if err != nil {
			toolResult.Content = []llm.ContentPart{llm.Text(err.Error())}
		}
		s.SessionManager.AppendMessage(sessionMessageValue(toolResult))
	}
	return nil
}

func (s *AgentSession) sdkTool(name string) *SDKTool {
	if s == nil || s.Agent == nil {
		return nil
	}
	for index := range s.Agent.State.Tools {
		if s.Agent.State.Tools[index].Name == name {
			return &s.Agent.State.Tools[index]
		}
	}
	return nil
}

func (s *AgentSession) IsRetrying() bool {
	if s == nil {
		return false
	}
	return s.isRetrying
}

func (s *AgentSession) IsStreaming() bool {
	if s == nil {
		return false
	}
	return s.isStreaming
}

func (s *AgentSession) Steer(text string) error {
	if s == nil {
		return errors.New("session is required")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("steering message is required")
	}
	s.steeringMessages = append(s.steeringMessages, text)
	s.emitQueueUpdate()
	return nil
}

func (s *AgentSession) FollowUp(text string) error {
	if s == nil {
		return errors.New("session is required")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("follow-up message is required")
	}
	s.followUpMessages = append(s.followUpMessages, text)
	s.emitQueueUpdate()
	return nil
}

func (s *AgentSession) PendingMessageCount() int {
	if s == nil {
		return 0
	}
	return len(s.steeringMessages) + len(s.followUpMessages)
}

func (s *AgentSession) GetSteeringMessages() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.steeringMessages...)
}

func (s *AgentSession) GetFollowUpMessages() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.followUpMessages...)
}

func (s *AgentSession) emitQueueUpdate() {
	s.emit(AgentSessionEvent{
		Type:     "queue_update",
		Steering: append([]string(nil), s.steeringMessages...),
		FollowUp: append([]string(nil), s.followUpMessages...),
	})
}

func isRetryableAssistantError(message llm.Message) bool {
	if message.StopReason != llm.StopReasonError {
		return false
	}
	text := strings.ToLower(message.ErrorMessage)
	return strings.Contains(text, "overloaded") || strings.Contains(text, "network_error")
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
	instructions := ""
	if len(customInstructions) > 0 {
		instructions = customInstructions[0]
	}
	s.emit(AgentSessionEvent{Type: "compaction_start", Reason: "manual"})
	result, err := s.compactManual(instructions)
	if err != nil {
		s.emit(AgentSessionEvent{
			Type:         "compaction_end",
			Reason:       "manual",
			Aborted:      false,
			WillRetry:    false,
			ErrorMessage: "Compaction failed: " + err.Error(),
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

func (s *AgentSession) compactManual(customInstructions string) (agentharness.CompactionResult, error) {
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
	summarizer := s.CompactionSummarizer
	if summarizer == nil {
		summarizer = DefaultAgentSessionCompactionSummarizer
	}
	result, err := summarizer(*preparation, customInstructions)
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
	s.SessionManager.AppendCompaction(result.Summary, result.FirstKeptEntryID, result.TokensBefore)
	return result, nil
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
			content = append(content, map[string]any{"type": llm.ContentText, "text": part.Text})
		case llm.ContentThinking:
			content = append(content, map[string]any{"type": llm.ContentThinking, "thinking": part.Thinking})
		case llm.ContentToolCall:
			content = append(content, map[string]any{"type": llm.ContentToolCall, "id": part.ID, "name": part.Name, "arguments": part.Arguments})
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
	if message.ErrorMessage != "" {
		value["errorMessage"] = message.ErrorMessage
	}
	if message.ToolCallID != "" {
		value["toolCallID"] = message.ToolCallID
	}
	if message.ToolName != "" {
		value["toolName"] = message.ToolName
	}
	if message.IsError {
		value["isError"] = message.IsError
	}
	return value
}

func sessionUsageValue(usage llm.Usage) map[string]any {
	return map[string]any{
		"input":       usage.Input,
		"output":      usage.Output,
		"cacheRead":   usage.CacheRead,
		"cacheWrite":  usage.CacheWrite,
		"totalTokens": usage.TotalTokens,
	}
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
