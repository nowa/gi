package harness

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type SessionStorage interface {
	Metadata() SessionMetadata
	LeafID() (string, bool, error)
	SetLeafID(*string) error
	CreateEntryID() string
	AppendEntry(Entry) error
	Entry(string) (Entry, bool)
	FindEntries(string) []Entry
	Label(string) (string, bool)
	SessionName() (string, bool)
	SessionStats() SessionStats
	PathToRoot(*string) ([]Entry, error)
	PathToRootOrCompaction(*string) ([]Entry, error)
	Entries(...SessionEntryCursorOptions) []Entry
}

type InMemorySessionStorage struct {
	mu          sync.RWMutex
	metadata    SessionMetadata
	entries     []Entry
	byID        map[string]Entry
	labels      map[string]string
	reservedIDs map[string]struct{}
	leafID      *string
}

func NewInMemorySessionStorage(metadata *SessionMetadata, entries []Entry) (*InMemorySessionStorage, error) {
	if metadata == nil {
		generated := SessionMetadata{ID: UUIDv7(), CreatedAt: nowISO()}
		metadata = &generated
	}
	storage := &InMemorySessionStorage{
		metadata:    cloneSessionMetadata(*metadata),
		entries:     cloneEntries(entries),
		byID:        map[string]Entry{},
		labels:      map[string]string{},
		reservedIDs: map[string]struct{}{},
	}
	for _, entry := range storage.entries {
		storage.byID[entry.ID] = entry
		storage.updateLabel(entry)
		storage.leafID = leafIDAfterEntry(entry)
	}
	if storage.leafID != nil {
		if _, ok := storage.byID[*storage.leafID]; !ok {
			return nil, newSessionError("invalid_session", "Entry %s not found", *storage.leafID)
		}
	}
	return storage, nil
}

func MustInMemorySessionStorage() *InMemorySessionStorage {
	storage, err := NewInMemorySessionStorage(nil, nil)
	if err != nil {
		panic(err)
	}
	return storage
}

func (s *InMemorySessionStorage) Metadata() SessionMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSessionMetadata(s.metadata)
}

func (s *InMemorySessionStorage) LeafID() (string, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.leafID == nil {
		return "", false, nil
	}
	if _, ok := s.byID[*s.leafID]; !ok {
		return "", false, newSessionError("invalid_session", "Entry %s not found", *s.leafID)
	}
	return *s.leafID, true, nil
}

func (s *InMemorySessionStorage) SetLeafID(leafID *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if leafID != nil {
		if _, ok := s.byID[*leafID]; !ok {
			return newSessionError("not_found", "Entry %s not found", *leafID)
		}
	}
	entry := Entry{Type: "leaf", ID: s.createEntryIDLocked(false), ParentID: cloneStringPtr(s.leafID), Timestamp: nowISO(), TargetID: cloneStringPtr(leafID)}
	s.entries = append(s.entries, entry)
	s.byID[entry.ID] = entry
	s.leafID = cloneStringPtr(leafID)
	return nil
}

func (s *InMemorySessionStorage) CreateEntryID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createEntryIDLocked(true)
}

func (s *InMemorySessionStorage) createEntryIDLocked(reserve bool) string {
	for i := 0; i < 100; i++ {
		uuid := UUIDv7()
		id := uuid[len(uuid)-8:]
		_, exists := s.byID[id]
		_, reserved := s.reservedIDs[id]
		if !exists && !reserved {
			if reserve {
				s.reservedIDs[id] = struct{}{}
			}
			return id
		}
	}
	id := UUIDv7()
	if reserve {
		s.reservedIDs[id] = struct{}{}
	}
	return id
}

func (s *InMemorySessionStorage) AppendEntry(entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry = cloneEntry(entry)
	s.entries = append(s.entries, entry)
	s.byID[entry.ID] = entry
	delete(s.reservedIDs, entry.ID)
	s.updateLabel(entry)
	s.leafID = leafIDAfterEntry(entry)
	return nil
}

func (s *InMemorySessionStorage) Entry(id string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.byID[id]
	return cloneEntry(entry), ok
}

func (s *InMemorySessionStorage) FindEntries(entryType string) []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Entry
	for _, entry := range s.entries {
		if entry.Type == entryType {
			result = append(result, cloneEntry(entry))
		}
	}
	return result
}

func (s *InMemorySessionStorage) Label(id string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	label, ok := s.labels[id]
	return label, ok
}

func (s *InMemorySessionStorage) SessionName() (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for index := len(s.entries) - 1; index >= 0; index-- {
		if s.entries[index].Type != "session_info" {
			continue
		}
		name := strings.TrimSpace(s.entries[index].Name)
		return name, name != ""
	}
	return "", false
}

func (s *InMemorySessionStorage) SessionStats() SessionStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return calculateSessionStats(s.entries)
}

func (s *InMemorySessionStorage) PathToRoot(leafID *string) ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if leafID == nil {
		return nil, nil
	}
	var path []Entry
	current, ok := s.byID[*leafID]
	if !ok {
		return nil, newSessionError("not_found", "Entry %s not found", *leafID)
	}
	for {
		path = append(path, cloneEntry(current))
		if current.ParentID == nil {
			break
		}
		parent, ok := s.byID[*current.ParentID]
		if !ok {
			return nil, newSessionError("invalid_session", "Entry %s not found", *current.ParentID)
		}
		current = parent
	}
	reverseEntries(path)
	return path, nil
}

func (s *InMemorySessionStorage) PathToRootOrCompaction(leafID *string) ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if leafID == nil {
		return nil, nil
	}
	var path []Entry
	var stopAtEntryID string
	current, ok := s.byID[*leafID]
	if !ok {
		return nil, newSessionError("not_found", "Entry %s not found", *leafID)
	}
	for {
		path = append(path, cloneEntry(current))
		if stopAtEntryID != "" && current.ID == stopAtEntryID {
			break
		}
		if current.Type == "compaction" {
			if current.RetainedTail != nil {
				break
			}
			stopAtEntryID = current.FirstKeptEntryID
		}
		if current.ParentID == nil {
			break
		}
		parent, ok := s.byID[*current.ParentID]
		if !ok {
			return nil, newSessionError("invalid_session", "Entry %s not found", *current.ParentID)
		}
		current = parent
	}
	reverseEntries(path)
	return path, nil
}

func (s *InMemorySessionStorage) Entries(options ...SessionEntryCursorOptions) []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	start, end := sessionEntryRange(len(s.entries), options...)
	return cloneEntries(s.entries[start:end])
}

func (s *InMemorySessionStorage) updateLabel(entry Entry) {
	if entry.Type != "label" || entry.TargetID == nil {
		return
	}
	if entry.Label != nil && strings.TrimSpace(*entry.Label) != "" {
		s.labels[*entry.TargetID] = strings.TrimSpace(*entry.Label)
	} else {
		delete(s.labels, *entry.TargetID)
	}
}

type JsonlSessionStorage struct {
	filePath string
	memory   *InMemorySessionStorage
	mu       sync.Mutex
}

type sessionHeader struct {
	Type          string         `json:"type"`
	Version       int            `json:"version"`
	ID            string         `json:"id"`
	Timestamp     string         `json:"timestamp"`
	CWD           string         `json:"cwd"`
	ParentSession string         `json:"parentSession,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

func CreateJsonlSessionStorage(filePath string, metadata SessionMetadata) (*JsonlSessionStorage, error) {
	if metadata.ID == "" {
		metadata.ID = UUIDv7()
	}
	if metadata.CreatedAt == "" {
		metadata.CreatedAt = nowISO()
	}
	metadata.Path = filePath
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, err
	}
	header := sessionHeader{
		Type:          "session",
		Version:       3,
		ID:            metadata.ID,
		Timestamp:     metadata.CreatedAt,
		CWD:           metadata.CWD,
		ParentSession: metadata.ParentSessionPath,
		Metadata:      cloneSessionMetadataMap(metadata.Metadata),
	}
	line, _ := json.Marshal(header)
	if err := os.WriteFile(filePath, append(line, '\n'), 0o644); err != nil {
		return nil, err
	}
	memory, err := NewInMemorySessionStorage(&metadata, nil)
	if err != nil {
		return nil, err
	}
	return &JsonlSessionStorage{filePath: filePath, memory: memory}, nil
}

func OpenJsonlSessionStorage(filePath string) (*JsonlSessionStorage, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, newSessionError("not_found", "Session not found: %s", filePath)
		}
		return nil, err
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return nil, invalidSession(filePath, "missing session header")
	}
	header, err := parseSessionHeader(lines[0], filePath)
	if err != nil {
		return nil, err
	}
	metadata := sessionMetadataFromHeader(header, filePath)
	var entries []Entry
	for i, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, newSessionError("invalid_entry", "Invalid JSONL session file %s: line %d is not valid JSON", filePath, i+2)
		}
		if entry.Type == "" || entry.ID == "" {
			return nil, newSessionError("invalid_entry", "Invalid JSONL session file %s: line %d is missing entry fields", filePath, i+2)
		}
		entries = append(entries, entry)
	}
	memory, err := NewInMemorySessionStorage(&metadata, entries)
	if err != nil {
		return nil, err
	}
	return &JsonlSessionStorage{filePath: filePath, memory: memory}, nil
}

func LoadJsonlSessionMetadata(filePath string) (SessionMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return SessionMetadata{}, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return SessionMetadata{}, invalidSession(filePath, "missing session header")
	}
	header, err := parseSessionHeader(scanner.Text(), filePath)
	if err != nil {
		return SessionMetadata{}, err
	}
	return sessionMetadataFromHeader(header, filePath), nil
}

func (s *JsonlSessionStorage) Metadata() SessionMetadata { return s.memory.Metadata() }
func (s *JsonlSessionStorage) LeafID() (string, bool, error) {
	return s.memory.LeafID()
}
func (s *JsonlSessionStorage) CreateEntryID() string { return s.memory.CreateEntryID() }
func (s *JsonlSessionStorage) Entry(id string) (Entry, bool) {
	return s.memory.Entry(id)
}
func (s *JsonlSessionStorage) FindEntries(entryType string) []Entry {
	return s.memory.FindEntries(entryType)
}
func (s *JsonlSessionStorage) Label(id string) (string, bool) { return s.memory.Label(id) }
func (s *JsonlSessionStorage) SessionName() (string, bool) {
	return s.memory.SessionName()
}
func (s *JsonlSessionStorage) SessionStats() SessionStats {
	return s.memory.SessionStats()
}
func (s *JsonlSessionStorage) PathToRoot(leafID *string) ([]Entry, error) {
	return s.memory.PathToRoot(leafID)
}
func (s *JsonlSessionStorage) PathToRootOrCompaction(leafID *string) ([]Entry, error) {
	return s.memory.PathToRootOrCompaction(leafID)
}
func (s *JsonlSessionStorage) Entries(options ...SessionEntryCursorOptions) []Entry {
	return s.memory.Entries(options...)
}

func (s *JsonlSessionStorage) SetLeafID(leafID *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if leafID != nil {
		if _, ok := s.memory.Entry(*leafID); !ok {
			return newSessionError("not_found", "Entry %s not found", *leafID)
		}
	}
	parentID, hasParent, err := s.memory.LeafID()
	if err != nil {
		return err
	}
	entry := Entry{
		Type:      "leaf",
		ID:        s.memory.CreateEntryID(),
		Timestamp: nowISO(),
		TargetID:  cloneStringPtr(leafID),
	}
	if hasParent {
		entry.ParentID = &parentID
	}
	if err := s.appendLine(entry); err != nil {
		return err
	}
	return s.memory.AppendEntry(entry)
}

func (s *JsonlSessionStorage) AppendEntry(entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry = cloneEntry(entry)
	if err := s.appendLine(entry); err != nil {
		return err
	}
	return s.memory.AppendEntry(entry)
}

func (s *JsonlSessionStorage) appendLine(entry Entry) error {
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	payload := append(line, '\n')
	written, err := file.Write(payload)
	if err == nil && written != len(payload) {
		return io.ErrShortWrite
	}
	return err
}

func parseSessionHeader(line, filePath string) (sessionHeader, error) {
	var header sessionHeader
	if err := json.Unmarshal([]byte(line), &header); err != nil {
		return sessionHeader{}, invalidSession(filePath, "first line is not a valid session header")
	}
	if header.Type != "session" || header.Version != 3 || header.ID == "" || header.Timestamp == "" || header.CWD == "" {
		return sessionHeader{}, invalidSession(filePath, "first line is not a valid session header")
	}
	return header, nil
}

func invalidSession(filePath, message string) error {
	return newSessionError("invalid_session", "Invalid JSONL session file %s: %s", filePath, message)
}

func leafIDAfterEntry(entry Entry) *string {
	if entry.Type == "leaf" {
		return cloneStringPtr(entry.TargetID)
	}
	return &entry.ID
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func calculateSessionStats(entries []Entry) SessionStats {
	var stats SessionStats
	for _, entry := range entries {
		var usage *llm.Usage
		switch entry.Type {
		case "message":
			stats.MessageCount++
			if entry.Message.Role == llm.RoleAssistant {
				usage = &entry.Message.Usage
			}
		case "compaction", "branch_summary":
			usage = entry.Usage
		}
		if usage == nil {
			continue
		}
		stats.CachedTokens += usage.CacheRead
		stats.UncachedTokens += usage.Input + usage.CacheWrite
		stats.TotalTokens += usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
		stats.CostTotal += usage.Cost.Total
	}
	return stats
}

func sessionEntryRange(entryCount int, options ...SessionEntryCursorOptions) (int, int) {
	if len(options) == 0 {
		return 0, entryCount
	}
	start := options[len(options)-1].AfterEntrySeq
	if start < 0 {
		start = 0
	}
	if start > entryCount {
		start = entryCount
	}
	limit := options[len(options)-1].Limit
	if limit <= 0 {
		return start, entryCount
	}
	end := start + limit
	if end > entryCount {
		end = entryCount
	}
	return start, end
}

func sessionMetadataFromHeader(header sessionHeader, filePath string) SessionMetadata {
	return SessionMetadata{
		ID:                header.ID,
		CreatedAt:         header.Timestamp,
		CWD:               header.CWD,
		Path:              filePath,
		ParentSessionPath: header.ParentSession,
		Metadata:          cloneSessionMetadataMap(header.Metadata),
	}
}

func cloneSessionMetadata(metadata SessionMetadata) SessionMetadata {
	metadata.Metadata = cloneSessionMetadataMap(metadata.Metadata)
	return metadata
}

func cloneSessionMetadataMap(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func reverseEntries(entries []Entry) {
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
}

func stringPtr(value string) *string { return &value }

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
