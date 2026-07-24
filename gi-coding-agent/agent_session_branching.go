package gicodingagent

import (
	"errors"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type AgentSessionForkMessage struct {
	EntryID string `json:"entryId"`
	Text    string `json:"text"`
}

type AgentSessionForkResult struct {
	Cancelled    bool
	SelectedText string
	Session      *AgentSession
}

func (s *AgentSession) GetUserMessagesForForking() []AgentSessionForkMessage {
	if s == nil || s.SessionManager == nil {
		return nil
	}
	branch := s.SessionManager.GetBranch()
	messages := make([]AgentSessionForkMessage, 0, len(branch))
	for _, entry := range branch {
		if entry.Type != "message" || sessionMessageRole(entry.Message) != llm.RoleUser {
			continue
		}
		text := sessionMessageText(entry.Message)
		if strings.TrimSpace(text) == "" {
			continue
		}
		messages = append(messages, AgentSessionForkMessage{EntryID: entry.ID, Text: text})
	}
	return messages
}

func (s *AgentSession) Fork(entryID string) (AgentSessionForkResult, error) {
	if s == nil || s.SessionManager == nil {
		return AgentSessionForkResult{}, errors.New("session manager is required")
	}
	selectedText := ""
	for _, message := range s.GetUserMessagesForForking() {
		if message.EntryID == entryID {
			selectedText = message.Text
			break
		}
	}
	if selectedText == "" {
		return AgentSessionForkResult{}, errors.New("Invalid entry ID for forking")
	}
	forkedManager, err := s.SessionManager.ForkBeforeEntry(entryID)
	if err != nil {
		return AgentSessionForkResult{}, err
	}
	forkedSession, err := CreateAgentSession(AgentSessionOptions{
		CWD:                  forkedManager.GetCwd(),
		Model:                s.Agent.State.Model,
		ThinkingLevel:        s.Agent.State.ThinkingLevel,
		Preflight:            s.Preflight,
		SettingsManager:      s.SettingsManager,
		SessionManager:       forkedManager,
		ResourceLoader:       s.ResourceLoader,
		CompactionSettings:   &s.CompactionSettings,
		CompactionSummarizer: s.CompactionSummarizer,
		BranchSummarizer:     s.BranchSummarizer,
		RetrySettings:        &s.RetrySettings,
		AutoCompactionRunner: s.AutoCompactionRunner,
		AgentContinue:        s.AgentContinue,
		Responder:            s.Responder,
		StreamResponder:      s.StreamResponder,
		ModelRuntime:         s.ModelRuntime,
		SummaryRuntime:       s.SummaryRuntime,
		ScopedModels:         s.ScopedModels,
		Tools:                s.Tools,
		ToolsSet:             s.ToolsSet,
		NoTools:              s.NoTools,
	})
	if err != nil {
		return AgentSessionForkResult{}, err
	}
	forkedSession.SteeringMode = s.SteeringMode
	forkedSession.FollowUpMode = s.FollowUpMode
	return AgentSessionForkResult{SelectedText: selectedText, Session: forkedSession}, nil
}

func (s *AgentSession) ForkAt(entryID string) (AgentSessionForkResult, error) {
	if s == nil || s.SessionManager == nil {
		return AgentSessionForkResult{}, errors.New("session manager is required")
	}
	forkedManager, err := s.SessionManager.ForkAtEntry(entryID)
	if err != nil {
		return AgentSessionForkResult{}, errors.New("Invalid entry ID for forking")
	}
	forkedSession, err := CreateAgentSession(AgentSessionOptions{
		CWD:                  forkedManager.GetCwd(),
		Model:                s.Agent.State.Model,
		ThinkingLevel:        s.Agent.State.ThinkingLevel,
		Preflight:            s.Preflight,
		SettingsManager:      s.SettingsManager,
		SessionManager:       forkedManager,
		ResourceLoader:       s.ResourceLoader,
		CompactionSettings:   &s.CompactionSettings,
		CompactionSummarizer: s.CompactionSummarizer,
		BranchSummarizer:     s.BranchSummarizer,
		RetrySettings:        &s.RetrySettings,
		AutoCompactionRunner: s.AutoCompactionRunner,
		AgentContinue:        s.AgentContinue,
		Responder:            s.Responder,
		StreamResponder:      s.StreamResponder,
		ModelRuntime:         s.ModelRuntime,
		SummaryRuntime:       s.SummaryRuntime,
		ScopedModels:         s.ScopedModels,
		Tools:                s.Tools,
		ToolsSet:             s.ToolsSet,
		NoTools:              s.NoTools,
	})
	if err != nil {
		return AgentSessionForkResult{}, err
	}
	forkedSession.SteeringMode = s.SteeringMode
	forkedSession.FollowUpMode = s.FollowUpMode
	return AgentSessionForkResult{Session: forkedSession}, nil
}

func (s *AgentSession) Messages() []llm.Message {
	if s == nil || s.SessionManager == nil {
		return nil
	}
	context := s.SessionManager.BuildSessionContext()
	messages := make([]llm.Message, 0, len(context.Messages))
	for _, message := range context.Messages {
		if converted, ok := sessionMessageToLLM(message); ok {
			messages = append(messages, converted)
		}
	}
	return messages
}

func (s *SessionManager) ForkBeforeEntry(entryID string) (*SessionManager, error) {
	return s.forkEntry(entryID, false)
}

func (s *SessionManager) ForkAtEntry(entryID string) (*SessionManager, error) {
	return s.forkEntry(entryID, true)
}

func (s *SessionManager) forkEntry(entryID string, includeEntry bool) (*SessionManager, error) {
	branch := s.GetBranch(entryID)
	if len(branch) == 0 {
		return nil, errors.New("Entry " + entryID + " not found")
	}
	target := branch[len(branch)-1]
	if target.ID != entryID {
		return nil, errors.New("Entry " + entryID + " not found")
	}
	parentSession := ""
	if s.persist {
		parentSession = s.sessionFile
	}
	forked, err := newSessionManager(s.cwd, s.sessionDir, "", s.persist)
	if err != nil {
		return nil, err
	}
	forked.newSession(NewSessionOptions{ParentSession: parentSession})
	header := forked.fileEntries[0]
	entries := []FileEntry{header}
	copyBranch := branch[:len(branch)-1]
	if includeEntry {
		copyBranch = branch
	}
	for _, entry := range copyBranch {
		if entry.Type == "label" {
			continue
		}
		entries = append(entries, entry)
	}
	forked.fileEntries = entries
	forked.buildIndex()
	forked.flushed = false
	return forked, nil
}

func sessionMessageRole(message any) string {
	switch typed := message.(type) {
	case llm.Message:
		return typed.Role
	case map[string]any:
		role, _ := typed["role"].(string)
		return role
	default:
		return ""
	}
}

func sessionMessageText(message any) string {
	switch typed := message.(type) {
	case llm.Message:
		parts := make([]string, 0, len(typed.Content))
		for _, part := range typed.Content {
			if part.Type == llm.ContentText {
				parts = append(parts, part.Text)
			}
		}
		return strings.Join(parts, "")
	case map[string]any:
		return extractMessageText(typed)
	default:
		return ""
	}
}

func sessionMessageToLLM(message any) (llm.Message, bool) {
	switch typed := message.(type) {
	case llm.Message:
		return typed, true
	case map[string]any:
		role, _ := typed["role"].(string)
		if role == "" {
			return llm.Message{}, false
		}
		details := typed["details"]
		if role == "branchSummary" || role == "compactionSummary" {
			detailMap := map[string]any{}
			if existing, ok := details.(map[string]any); ok {
				for key, value := range existing {
					detailMap[key] = value
				}
			}
			if role == "branchSummary" {
				if fromID, ok := typed["fromId"].(string); ok && fromID != "" {
					detailMap["fromId"] = fromID
				}
			}
			if role == "compactionSummary" {
				if tokensBefore, ok := typed["tokensBefore"]; ok {
					detailMap["tokensBefore"] = tokensBefore
				}
			}
			if len(detailMap) > 0 {
				details = detailMap
			}
		}
		message := llm.Message{
			Role:         role,
			Content:      sessionMessageContentToLLM(typed["content"]),
			API:          stringFromSessionMessageValue(typed["api"]),
			Provider:     stringFromSessionMessageValue(typed["provider"]),
			Model:        stringFromSessionMessageValue(typed["model"]),
			StopReason:   stringFromSessionMessageValue(typed["stopReason"]),
			ErrorMessage: stringFromSessionMessageValue(typed["errorMessage"]),
			ToolCallID:   stringFromSessionMessageValue(typed["toolCallID"]),
			ToolName:     stringFromSessionMessageValue(typed["toolName"]),
			CustomType:   stringFromSessionMessageValue(typed["customType"]),
			Details:      details,
		}
		if display, ok := typed["display"].(bool); ok {
			message.Display = &display
		}
		if exclude, ok := typed["excludeFromContext"].(bool); ok && exclude && message.Details == nil {
			message.Details = map[string]any{"excludeFromContext": true}
		}
		if isError, ok := typed["isError"].(bool); ok {
			message.IsError = isError
		}
		if timestamp, ok := messageTimestampMillis(typed); ok {
			message.Timestamp = timestamp
		}
		if usage, ok := usageFromSessionMessageValue(typed["usage"]); ok {
			message.Usage = usage
		}
		if role == "custom" && len(message.Content) == 0 && typed["content"] != nil {
			message.Content = []llm.ContentPart{llm.Text(customMessageText(typed["content"]))}
		}
		if len(message.Content) == 0 {
			message.Content = []llm.ContentPart{llm.Text(extractMessageText(typed))}
		}
		return message, true
	default:
		return llm.Message{}, false
	}
}

func sessionMessageContentToLLM(value any) []llm.ContentPart {
	switch content := value.(type) {
	case []llm.ContentPart:
		return append([]llm.ContentPart(nil), content...)
	case string:
		return []llm.ContentPart{llm.Text(content)}
	case []any:
		parts := make([]llm.ContentPart, 0, len(content))
		for _, item := range content {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			blockType, _ := block["type"].(string)
			switch blockType {
			case llm.ContentText:
				part := llm.Text(stringFromSessionMessageValue(block["text"]))
				part.TextSignature = stringFromSessionMessageValue(block["textSignature"])
				parts = append(parts, part)
			case llm.ContentThinking:
				part := llm.Thinking(stringFromSessionMessageValue(block["thinking"]))
				part.ThinkingSignature = stringFromSessionMessageValue(block["thinkingSignature"])
				if redacted, ok := block["redacted"].(bool); ok {
					part.Redacted = redacted
				}
				parts = append(parts, part)
			case llm.ContentImage:
				parts = append(parts, llm.Image(
					stringFromSessionMessageValue(block["data"]),
					stringFromSessionMessageValue(block["mimeType"]),
				))
			case llm.ContentToolCall:
				part := llm.ToolCall(
					stringFromSessionMessageValue(block["id"]),
					stringFromSessionMessageValue(block["name"]),
					mapFromSessionMessageValue(block["arguments"]),
				)
				part.ThoughtSignature = stringFromSessionMessageValue(block["thoughtSignature"])
				parts = append(parts, part)
			}
		}
		return parts
	default:
		return nil
	}
}

func stringFromSessionMessageValue(value any) string {
	text, _ := value.(string)
	return text
}

func mapFromSessionMessageValue(value any) map[string]any {
	if value == nil {
		return nil
	}
	if result, ok := value.(map[string]any); ok {
		return result
	}
	return nil
}
