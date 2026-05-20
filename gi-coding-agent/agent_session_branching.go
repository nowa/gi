package gicodingagent

import (
	"errors"
	"fmt"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type AgentSessionForkMessage struct {
	EntryID string
	Text    string
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
		return AgentSessionForkResult{}, fmt.Errorf("fork entry %s is not a user message", entryID)
	}
	forkedManager, err := s.SessionManager.ForkBeforeEntry(entryID)
	if err != nil {
		return AgentSessionForkResult{}, err
	}
	forkedSession, err := CreateAgentSession(AgentSessionOptions{
		CWD:            forkedManager.GetCwd(),
		Model:          s.Agent.State.Model,
		SessionManager: forkedManager,
		ResourceLoader: s.ResourceLoader,
	})
	if err != nil {
		return AgentSessionForkResult{}, err
	}
	return AgentSessionForkResult{SelectedText: selectedText, Session: forkedSession}, nil
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
	for _, entry := range branch[:len(branch)-1] {
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
		}
		if timestamp, ok := messageTimestampMillis(typed); ok {
			message.Timestamp = timestamp
		}
		if usage, ok := usageFromSessionMessageValue(typed["usage"]); ok {
			message.Usage = usage
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
				parts = append(parts, llm.Text(stringFromSessionMessageValue(block["text"])))
			case llm.ContentThinking:
				parts = append(parts, llm.Thinking(stringFromSessionMessageValue(block["thinking"])))
			case llm.ContentToolCall:
				parts = append(parts, llm.ToolCall(
					stringFromSessionMessageValue(block["id"]),
					stringFromSessionMessageValue(block["name"]),
					mapFromSessionMessageValue(block["arguments"]),
				))
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
