package harness

import (
	"fmt"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type SessionContext struct {
	Messages      []llm.Message
	ThinkingLevel string
	ModelProvider string
	ModelID       string
}

type Session struct {
	storage SessionStorage
}

type SessionEntryOptions struct {
	Details  any
	FromHook bool
}

func NewSession(storage SessionStorage) *Session {
	return &Session{storage: storage}
}

func (s *Session) Storage() SessionStorage       { return s.storage }
func (s *Session) Metadata() SessionMetadata     { return s.storage.Metadata() }
func (s *Session) Entries() []Entry              { return s.storage.Entries() }
func (s *Session) Entry(id string) (Entry, bool) { return s.storage.Entry(id) }
func (s *Session) LeafID() (*string, error) {
	id, ok, err := s.storage.LeafID()
	if err != nil || !ok {
		return nil, err
	}
	return &id, nil
}

func (s *Session) Branch(fromID *string) ([]Entry, error) {
	leaf := fromID
	if leaf == nil {
		current, err := s.LeafID()
		if err != nil {
			return nil, err
		}
		leaf = current
	}
	return s.storage.PathToRoot(leaf)
}

func (s *Session) BuildContext() (SessionContext, error) {
	branch, err := s.Branch(nil)
	if err != nil {
		return SessionContext{}, err
	}
	return BuildSessionContext(branch), nil
}

func (s *Session) Label(id string) (string, bool) { return s.storage.Label(id) }

func (s *Session) SessionName() (string, bool) {
	entries := s.storage.FindEntries("session_info")
	if len(entries) == 0 {
		return "", false
	}
	name := strings.TrimSpace(entries[len(entries)-1].Name)
	return name, name != ""
}

func (s *Session) AppendMessage(message llm.Message) (string, error) {
	return s.appendEntry(Entry{Type: "message", ID: s.storage.CreateEntryID(), ParentID: s.currentParentID(), Timestamp: nowISO(), Message: message})
}

func (s *Session) AppendThinkingLevelChange(level string) (string, error) {
	return s.appendEntry(Entry{Type: "thinking_level_change", ID: s.storage.CreateEntryID(), ParentID: s.currentParentID(), Timestamp: nowISO(), ThinkingLevel: level})
}

func (s *Session) AppendModelChange(provider, modelID string) (string, error) {
	return s.appendEntry(Entry{Type: "model_change", ID: s.storage.CreateEntryID(), ParentID: s.currentParentID(), Timestamp: nowISO(), Provider: provider, ModelID: modelID})
}

func (s *Session) AppendCompaction(summary, firstKeptEntryID string, tokensBefore int, details ...any) (string, error) {
	var entryDetails any
	if len(details) > 0 {
		entryDetails = details[0]
	}
	return s.AppendCompactionWithOptions(summary, firstKeptEntryID, tokensBefore, SessionEntryOptions{Details: entryDetails})
}

func (s *Session) AppendCompactionWithOptions(summary, firstKeptEntryID string, tokensBefore int, options SessionEntryOptions) (string, error) {
	return s.appendEntry(Entry{Type: "compaction", ID: s.storage.CreateEntryID(), ParentID: s.currentParentID(), Timestamp: nowISO(), Summary: summary, FirstKeptEntryID: firstKeptEntryID, TokensBefore: tokensBefore, Details: options.Details, FromHook: options.FromHook})
}

func (s *Session) AppendCustomMessageEntry(customType string, content any, display bool, details any) (string, error) {
	return s.appendEntry(Entry{Type: "custom_message", ID: s.storage.CreateEntryID(), ParentID: s.currentParentID(), Timestamp: nowISO(), CustomType: customType, Content: content, Display: display, Details: details})
}

func (s *Session) AppendLabel(targetID, label string) (string, error) {
	if _, ok := s.storage.Entry(targetID); !ok {
		return "", newSessionError("not_found", "Entry %s not found", targetID)
	}
	var labelPtr *string
	if label != "" {
		labelPtr = &label
	}
	return s.appendEntry(Entry{Type: "label", ID: s.storage.CreateEntryID(), ParentID: s.currentParentID(), Timestamp: nowISO(), TargetID: &targetID, Label: labelPtr})
}

func (s *Session) AppendSessionName(name string) (string, error) {
	return s.appendEntry(Entry{Type: "session_info", ID: s.storage.CreateEntryID(), ParentID: s.currentParentID(), Timestamp: nowISO(), Name: strings.TrimSpace(name)})
}

func (s *Session) MoveTo(entryID *string, summary string, details ...any) (*string, error) {
	var entryDetails any
	if len(details) > 0 {
		entryDetails = details[0]
	}
	return s.MoveToWithOptions(entryID, summary, SessionEntryOptions{Details: entryDetails})
}

func (s *Session) MoveToWithOptions(entryID *string, summary string, options SessionEntryOptions) (*string, error) {
	if entryID != nil {
		if _, ok := s.storage.Entry(*entryID); !ok {
			return nil, newSessionError("not_found", "Entry %s not found", *entryID)
		}
	}
	if err := s.storage.SetLeafID(entryID); err != nil {
		return nil, err
	}
	if summary == "" {
		return nil, nil
	}
	fromID := "root"
	if entryID != nil {
		fromID = *entryID
	}
	id, err := s.appendEntry(Entry{Type: "branch_summary", ID: s.storage.CreateEntryID(), ParentID: cloneStringPtr(entryID), Timestamp: nowISO(), FromID: fromID, Summary: summary, Details: options.Details, FromHook: options.FromHook})
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (s *Session) appendEntry(entry Entry) (string, error) {
	if err := s.storage.AppendEntry(entry); err != nil {
		return "", err
	}
	return entry.ID, nil
}

func (s *Session) currentParentID() *string {
	id, ok, _ := s.storage.LeafID()
	if !ok {
		return nil
	}
	return &id
}

func BuildSessionContext(pathEntries []Entry) SessionContext {
	context := SessionContext{ThinkingLevel: "off"}
	var compaction *Entry
	for i := range pathEntries {
		entry := pathEntries[i]
		switch entry.Type {
		case "thinking_level_change":
			context.ThinkingLevel = entry.ThinkingLevel
		case "model_change":
			context.ModelProvider = entry.Provider
			context.ModelID = entry.ModelID
		case "message":
			if entry.Message.Role == llm.RoleAssistant {
				context.ModelProvider = entry.Message.Provider
				context.ModelID = entry.Message.Model
			}
		case "compaction":
			compaction = &pathEntries[i]
		}
	}

	appendMessage := func(entry Entry) {
		switch entry.Type {
		case "message":
			context.Messages = append(context.Messages, entry.Message)
		case "custom_message":
			context.Messages = append(context.Messages, llm.Message{Role: entry.CustomType, Content: []llm.ContentPart{llm.Text(fmt.Sprint(entry.Content))}, Timestamp: llm.NowMillis(), Details: entry.Details})
		case "branch_summary":
			if entry.Summary != "" {
				context.Messages = append(context.Messages, llm.Message{Role: "branchSummary", Content: []llm.ContentPart{llm.Text(entry.Summary)}, Timestamp: llm.NowMillis()})
			}
		}
	}

	if compaction != nil {
		context.Messages = append(context.Messages, llm.Message{Role: "compactionSummary", Content: []llm.ContentPart{llm.Text(compaction.Summary)}, Timestamp: llm.NowMillis()})
		compactionIndex := -1
		for i, entry := range pathEntries {
			if entry.ID == compaction.ID {
				compactionIndex = i
				break
			}
		}
		foundFirstKept := false
		for i := 0; i < compactionIndex; i++ {
			entry := pathEntries[i]
			if entry.ID == compaction.FirstKeptEntryID {
				foundFirstKept = true
			}
			if foundFirstKept {
				appendMessage(entry)
			}
		}
		for i := compactionIndex + 1; i < len(pathEntries); i++ {
			appendMessage(pathEntries[i])
		}
		return context
	}

	for _, entry := range pathEntries {
		appendMessage(entry)
	}
	return context
}
