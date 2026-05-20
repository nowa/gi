package gicodingagent

import (
	"errors"
	"reflect"
	"sort"
	"strings"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

type AgentSessionEvent struct {
	Type         string
	Reason       string
	Result       *agentharness.CompactionResult
	Aborted      bool
	WillRetry    bool
	ErrorMessage string
	Message      *llm.Message
}

type AgentSessionEventListener func(AgentSessionEvent)

type AgentSessionResponder func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error)

type AgentSessionCompactionSummarizer func(preparation agentharness.CompactionPreparation, customInstructions string) (agentharness.CompactionResult, error)

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
	prompt := strings.TrimSpace(text)
	if prompt == "" {
		return errors.New("prompt is required")
	}
	s.SessionManager.AppendMessage(sessionUserMessageValue(prompt))
	responder := s.Responder
	if responder == nil {
		responder = DefaultAgentSessionResponder
	}
	assistant, err := responder(prompt, s.Messages(), s.Agent.State.Model)
	if err != nil {
		return err
	}
	if assistant.Role == "" {
		assistant.Role = llm.RoleAssistant
	}
	if assistant.Timestamp == 0 {
		assistant.Timestamp = llm.NowMillis()
	}
	if assistant.Provider == "" {
		assistant.Provider = s.Agent.State.Model.Provider
	}
	if assistant.Model == "" {
		assistant.Model = s.Agent.State.Model.ID
	}
	if assistant.API == "" {
		assistant.API = s.Agent.State.Model.API
	}
	if assistant.StopReason == "" {
		assistant.StopReason = llm.StopReasonStop
	}
	s.SessionManager.AppendMessage(sessionMessageValue(assistant))
	s.emit(AgentSessionEvent{Type: "message_end", Message: &assistant})
	return nil
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
	return map[string]any{
		"role":       message.Role,
		"content":    content,
		"timestamp":  message.Timestamp,
		"api":        message.API,
		"provider":   message.Provider,
		"model":      message.Model,
		"usage":      sessionUsageValue(message.Usage),
		"stopReason": message.StopReason,
	}
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
