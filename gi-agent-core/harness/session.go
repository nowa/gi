package harness

import (
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type SessionContext struct {
	Messages        []llm.Message
	ThinkingLevel   string
	Model           *SessionModel
	ActiveToolNames []string

	// ModelProvider and ModelID are kept as compatibility mirrors. Model is the
	// canonical representation for new callers.
	ModelProvider string
	ModelID       string
}

type SessionModel struct {
	Provider string
	ModelID  string
}

type ContextEntryTransform func(entries []Entry) []Entry

type CustomEntryContextMessageProjector func(entry Entry, index int, entries []Entry) []llm.Message

type SessionContextBuildOptions struct {
	EntryTransforms []ContextEntryTransform
	EntryProjectors map[string]CustomEntryContextMessageProjector
}

type Session struct {
	storage             SessionStorage
	contextBuildOptions SessionContextBuildOptions
}

type SessionEntryOptions struct {
	Details      any
	FromHook     bool
	Usage        *llm.Usage
	RetainedTail []llm.Message
}

func NewSession(storage SessionStorage, contextBuildOptions ...SessionContextBuildOptions) *Session {
	session := &Session{storage: storage}
	for _, options := range contextBuildOptions {
		session.contextBuildOptions = mergeSessionContextBuildOptions(session.contextBuildOptions, options)
	}
	return session
}

func (s *Session) Storage() SessionStorage   { return s.storage }
func (s *Session) Metadata() SessionMetadata { return s.storage.Metadata() }
func (s *Session) Entries(options ...SessionEntryCursorOptions) []Entry {
	return s.storage.Entries(options...)
}
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
	return s.storage.PathToRootOrCompaction(leaf)
}

func (s *Session) BuildContextEntries(options ...SessionContextBuildOptions) ([]Entry, error) {
	branch, err := s.Branch(nil)
	if err != nil {
		return nil, err
	}
	return BuildContextEntries(branch, s.mergeContextBuildOptions(options...)), nil
}

func (s *Session) BuildContext(options ...SessionContextBuildOptions) (SessionContext, error) {
	branch, err := s.Branch(nil)
	if err != nil {
		return SessionContext{}, err
	}
	return BuildSessionContext(branch, s.mergeContextBuildOptions(options...)), nil
}

func (s *Session) Label(id string) (string, bool) { return s.storage.Label(id) }

func (s *Session) SessionName() (string, bool) {
	return s.storage.SessionName()
}

func (s *Session) Stats() SessionStats { return s.storage.SessionStats() }

func (s *Session) AppendMessage(message llm.Message) (string, error) {
	return s.appendEntry(Entry{Type: "message", ID: s.storage.CreateEntryID(), ParentID: s.currentParentID(), Timestamp: nowISO(), Message: message})
}

func (s *Session) AppendThinkingLevelChange(level string) (string, error) {
	return s.appendEntry(Entry{Type: "thinking_level_change", ID: s.storage.CreateEntryID(), ParentID: s.currentParentID(), Timestamp: nowISO(), ThinkingLevel: level})
}

func (s *Session) AppendModelChange(provider, modelID string) (string, error) {
	return s.appendEntry(Entry{Type: "model_change", ID: s.storage.CreateEntryID(), ParentID: s.currentParentID(), Timestamp: nowISO(), Provider: provider, ModelID: modelID})
}

func (s *Session) AppendActiveToolsChange(activeToolNames []string) (string, error) {
	return s.appendEntry(Entry{
		Type:            "active_tools_change",
		ID:              s.storage.CreateEntryID(),
		ParentID:        s.currentParentID(),
		Timestamp:       nowISO(),
		ActiveToolNames: append([]string{}, activeToolNames...),
	})
}

func (s *Session) AppendCompaction(summary, firstKeptEntryID string, tokensBefore int, details ...any) (string, error) {
	var entryDetails any
	if len(details) > 0 {
		entryDetails = details[0]
	}
	return s.AppendCompactionWithOptions(summary, firstKeptEntryID, tokensBefore, SessionEntryOptions{Details: entryDetails})
}

func (s *Session) AppendCompactionWithOptions(summary, firstKeptEntryID string, tokensBefore int, options SessionEntryOptions) (string, error) {
	return s.appendEntry(Entry{
		Type:             "compaction",
		ID:               s.storage.CreateEntryID(),
		ParentID:         s.currentParentID(),
		Timestamp:        nowISO(),
		Summary:          summary,
		FirstKeptEntryID: firstKeptEntryID,
		TokensBefore:     tokensBefore,
		RetainedTail:     cloneMessages(options.RetainedTail),
		Details:          options.Details,
		FromHook:         options.FromHook,
		Usage:            cloneUsagePointer(options.Usage),
	})
}

func (s *Session) AppendCustomEntry(customType string, data any) (string, error) {
	return s.appendEntry(Entry{
		Type:       "custom",
		ID:         s.storage.CreateEntryID(),
		ParentID:   s.currentParentID(),
		Timestamp:  nowISO(),
		CustomType: customType,
		Data:       data,
	})
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
	sanitizedName := strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(name)
	return s.appendEntry(Entry{Type: "session_info", ID: s.storage.CreateEntryID(), ParentID: s.currentParentID(), Timestamp: nowISO(), Name: strings.TrimSpace(sanitizedName)})
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
	id, err := s.appendEntry(Entry{Type: "branch_summary", ID: s.storage.CreateEntryID(), ParentID: cloneStringPtr(entryID), Timestamp: nowISO(), FromID: fromID, Summary: summary, Details: options.Details, FromHook: options.FromHook, Usage: cloneUsagePointer(options.Usage)})
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func cloneUsagePointer(usage *llm.Usage) *llm.Usage {
	if usage == nil {
		return nil
	}
	cloned := *usage
	return &cloned
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

type sessionContextState struct {
	thinkingLevel   string
	model           *SessionModel
	activeToolNames []string
}

func deriveSessionContextState(pathEntries []Entry) sessionContextState {
	state := sessionContextState{thinkingLevel: "off"}
	for _, entry := range pathEntries {
		switch entry.Type {
		case "thinking_level_change":
			state.thinkingLevel = entry.ThinkingLevel
		case "model_change":
			state.model = &SessionModel{Provider: entry.Provider, ModelID: entry.ModelID}
		case "message":
			if entry.Message.Role == llm.RoleAssistant {
				state.model = &SessionModel{Provider: entry.Message.Provider, ModelID: entry.Message.Model}
			}
		case "active_tools_change":
			state.activeToolNames = append([]string{}, entry.ActiveToolNames...)
		}
	}
	return state
}

func DefaultContextEntryTransform(pathEntries []Entry) []Entry {
	compactionIndex := -1
	for index, entry := range pathEntries {
		if entry.Type == "compaction" {
			compactionIndex = index
		}
	}
	if compactionIndex == -1 {
		return cloneEntries(pathEntries)
	}

	compaction := pathEntries[compactionIndex]
	entries := []Entry{cloneEntry(compaction)}
	if compaction.RetainedTail != nil {
		entries = append(entries, cloneEntries(pathEntries[compactionIndex+1:])...)
		return entries
	}
	if compaction.FirstKeptEntryID != "" {
		foundFirstKept := false
		for _, entry := range pathEntries[:compactionIndex] {
			if entry.ID == compaction.FirstKeptEntryID {
				foundFirstKept = true
			}
			if foundFirstKept {
				entries = append(entries, cloneEntry(entry))
			}
		}
	}
	entries = append(entries, cloneEntries(pathEntries[compactionIndex+1:])...)
	return entries
}

func BuildContextEntries(pathEntries []Entry, options ...SessionContextBuildOptions) []Entry {
	mergedOptions := mergeSessionContextBuildOptions(SessionContextBuildOptions{}, options...)
	entries := DefaultContextEntryTransform(pathEntries)
	for _, transform := range mergedOptions.EntryTransforms {
		if transform == nil {
			continue
		}
		entries = cloneEntries(transform(cloneEntries(entries)))
	}
	return entries
}

func SessionEntryToContextMessages(entry Entry, index int, entries []Entry, options ...SessionContextBuildOptions) []llm.Message {
	switch entry.Type {
	case "message":
		return []llm.Message{cloneMessage(entry.Message)}
	case "custom_message":
		return []llm.Message{customMessageFromEntry(entry)}
	case "compaction":
		messages := []llm.Message{compactionSummaryMessageFromEntry(entry)}
		messages = append(messages, cloneMessages(entry.RetainedTail)...)
		return messages
	case "branch_summary":
		if entry.Summary != "" {
			return []llm.Message{branchSummaryMessageFromEntry(entry)}
		}
	case "custom":
		mergedOptions := mergeSessionContextBuildOptions(SessionContextBuildOptions{}, options...)
		if projector := mergedOptions.EntryProjectors[entry.CustomType]; projector != nil {
			return cloneMessages(projector(entry, index, cloneEntries(entries)))
		}
	}
	return nil
}

func BuildSessionContext(pathEntries []Entry, options ...SessionContextBuildOptions) SessionContext {
	mergedOptions := mergeSessionContextBuildOptions(SessionContextBuildOptions{}, options...)
	state := deriveSessionContextState(pathEntries)
	context := SessionContext{
		ThinkingLevel:   state.thinkingLevel,
		Model:           cloneSessionModel(state.model),
		ActiveToolNames: cloneStrings(state.activeToolNames),
	}
	if state.model != nil {
		context.ModelProvider = state.model.Provider
		context.ModelID = state.model.ModelID
	}
	contextEntries := BuildContextEntries(pathEntries, mergedOptions)
	for index, entry := range contextEntries {
		context.Messages = append(
			context.Messages,
			SessionEntryToContextMessages(entry, index, contextEntries, mergedOptions)...,
		)
	}
	return context
}

func (s *Session) mergeContextBuildOptions(options ...SessionContextBuildOptions) SessionContextBuildOptions {
	return mergeSessionContextBuildOptions(s.contextBuildOptions, options...)
}

func mergeSessionContextBuildOptions(base SessionContextBuildOptions, options ...SessionContextBuildOptions) SessionContextBuildOptions {
	merged := SessionContextBuildOptions{
		EntryTransforms: append([]ContextEntryTransform{}, base.EntryTransforms...),
		EntryProjectors: make(map[string]CustomEntryContextMessageProjector, len(base.EntryProjectors)),
	}
	for customType, projector := range base.EntryProjectors {
		merged.EntryProjectors[customType] = projector
	}
	for _, option := range options {
		merged.EntryTransforms = append(merged.EntryTransforms, option.EntryTransforms...)
		for customType, projector := range option.EntryProjectors {
			merged.EntryProjectors[customType] = projector
		}
	}
	if len(merged.EntryProjectors) == 0 {
		merged.EntryProjectors = nil
	}
	return merged
}

func cloneSessionModel(model *SessionModel) *SessionModel {
	if model == nil {
		return nil
	}
	cloned := *model
	return &cloned
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}
