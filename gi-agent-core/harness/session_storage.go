package harness

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	PathToRoot(*string) ([]Entry, error)
	Entries() []Entry
}

type InMemorySessionStorage struct {
	metadata SessionMetadata
	entries  []Entry
	byID     map[string]Entry
	labels   map[string]string
	leafID   *string
}

func NewInMemorySessionStorage(metadata *SessionMetadata, entries []Entry) (*InMemorySessionStorage, error) {
	if metadata == nil {
		generated := SessionMetadata{ID: UUIDv7(), CreatedAt: nowISO()}
		metadata = &generated
	}
	storage := &InMemorySessionStorage{
		metadata: *metadata,
		entries:  append([]Entry{}, entries...),
		byID:     map[string]Entry{},
		labels:   map[string]string{},
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

func (s *InMemorySessionStorage) Metadata() SessionMetadata { return s.metadata }

func (s *InMemorySessionStorage) LeafID() (string, bool, error) {
	if s.leafID == nil {
		return "", false, nil
	}
	if _, ok := s.byID[*s.leafID]; !ok {
		return "", false, newSessionError("invalid_session", "Entry %s not found", *s.leafID)
	}
	return *s.leafID, true, nil
}

func (s *InMemorySessionStorage) SetLeafID(leafID *string) error {
	if leafID != nil {
		if _, ok := s.byID[*leafID]; !ok {
			return newSessionError("not_found", "Entry %s not found", *leafID)
		}
	}
	entry := Entry{Type: "leaf", ID: s.CreateEntryID(), ParentID: cloneStringPtr(s.leafID), Timestamp: nowISO(), TargetID: cloneStringPtr(leafID)}
	s.entries = append(s.entries, entry)
	s.byID[entry.ID] = entry
	s.leafID = cloneStringPtr(leafID)
	return nil
}

func (s *InMemorySessionStorage) CreateEntryID() string {
	for i := 0; i < 100; i++ {
		id := UUIDv7()[:8]
		if _, ok := s.byID[id]; !ok {
			return id
		}
	}
	return UUIDv7()
}

func (s *InMemorySessionStorage) AppendEntry(entry Entry) error {
	s.entries = append(s.entries, entry)
	s.byID[entry.ID] = entry
	s.updateLabel(entry)
	s.leafID = leafIDAfterEntry(entry)
	return nil
}

func (s *InMemorySessionStorage) Entry(id string) (Entry, bool) {
	entry, ok := s.byID[id]
	return entry, ok
}

func (s *InMemorySessionStorage) FindEntries(entryType string) []Entry {
	var result []Entry
	for _, entry := range s.entries {
		if entry.Type == entryType {
			result = append(result, entry)
		}
	}
	return result
}

func (s *InMemorySessionStorage) Label(id string) (string, bool) {
	label, ok := s.labels[id]
	return label, ok
}

func (s *InMemorySessionStorage) PathToRoot(leafID *string) ([]Entry, error) {
	if leafID == nil {
		return nil, nil
	}
	var path []Entry
	current, ok := s.byID[*leafID]
	if !ok {
		return nil, newSessionError("not_found", "Entry %s not found", *leafID)
	}
	for {
		path = append([]Entry{current}, path...)
		if current.ParentID == nil {
			break
		}
		parent, ok := s.byID[*current.ParentID]
		if !ok {
			return nil, newSessionError("invalid_session", "Entry %s not found", *current.ParentID)
		}
		current = parent
	}
	return path, nil
}

func (s *InMemorySessionStorage) Entries() []Entry {
	return append([]Entry{}, s.entries...)
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
}

type sessionHeader struct {
	Type          string `json:"type"`
	Version       int    `json:"version"`
	ID            string `json:"id"`
	Timestamp     string `json:"timestamp"`
	CWD           string `json:"cwd"`
	ParentSession string `json:"parentSession,omitempty"`
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
	header := sessionHeader{Type: "session", Version: 3, ID: metadata.ID, Timestamp: metadata.CreatedAt, CWD: metadata.CWD, ParentSession: metadata.ParentSessionPath}
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
	metadata := SessionMetadata{ID: header.ID, CreatedAt: header.Timestamp, CWD: header.CWD, Path: filePath, ParentSessionPath: header.ParentSession}
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
	return SessionMetadata{ID: header.ID, CreatedAt: header.Timestamp, CWD: header.CWD, Path: filePath, ParentSessionPath: header.ParentSession}, nil
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
func (s *JsonlSessionStorage) PathToRoot(leafID *string) ([]Entry, error) {
	return s.memory.PathToRoot(leafID)
}
func (s *JsonlSessionStorage) Entries() []Entry { return s.memory.Entries() }

func (s *JsonlSessionStorage) SetLeafID(leafID *string) error {
	before := len(s.memory.entries)
	if err := s.memory.SetLeafID(leafID); err != nil {
		return err
	}
	return s.appendLine(s.memory.entries[before])
}

func (s *JsonlSessionStorage) AppendEntry(entry Entry) error {
	if err := s.memory.AppendEntry(entry); err != nil {
		return err
	}
	return s.appendLine(entry)
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
	_, err = file.Write(append(line, '\n'))
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

func stringPtr(value string) *string { return &value }

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
