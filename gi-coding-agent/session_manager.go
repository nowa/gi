package gicodingagent

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
)

const CurrentSessionVersion = 3

type SessionHeader struct {
	Type          string `json:"type"`
	Version       int    `json:"version,omitempty"`
	ID            string `json:"id"`
	Timestamp     string `json:"timestamp"`
	CWD           string `json:"cwd"`
	ParentSession string `json:"parentSession,omitempty"`
}

type NewSessionOptions struct {
	ID            string
	ParentSession string
}

type FileEntry struct {
	Type          string
	ID            string
	Version       int
	Timestamp     string
	CWD           string
	ParentSession string
	ParentID      *string
	Message       any
	ThinkingLevel string
	Provider      string
	ModelID       string
	Summary       string
	FirstKeptID   string
	TokensBefore  int
	CustomType    string
	Data          any
	Content       any
	Display       bool
	Details       any
	FromHook      bool
	TargetID      string
	Label         string
	Name          string
	FromID        string
	raw           map[string]any
}

func (e *FileEntry) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.raw = raw
	e.Type, _ = raw["type"].(string)
	e.ID, _ = raw["id"].(string)
	e.Timestamp, _ = raw["timestamp"].(string)
	e.CWD, _ = raw["cwd"].(string)
	e.ParentSession, _ = raw["parentSession"].(string)
	if parentID, ok := raw["parentId"].(string); ok {
		e.ParentID = &parentID
	} else {
		e.ParentID = nil
	}
	if version, ok := raw["version"].(float64); ok {
		e.Version = int(version)
	}
	e.Message = raw["message"]
	e.ThinkingLevel, _ = raw["thinkingLevel"].(string)
	e.Provider, _ = raw["provider"].(string)
	e.ModelID, _ = raw["modelId"].(string)
	e.Summary, _ = raw["summary"].(string)
	e.FirstKeptID, _ = raw["firstKeptEntryId"].(string)
	if tokens, ok := raw["tokensBefore"].(float64); ok {
		e.TokensBefore = int(tokens)
	}
	e.CustomType, _ = raw["customType"].(string)
	e.Data = raw["data"]
	e.Content = raw["content"]
	e.Display, _ = raw["display"].(bool)
	e.Details = raw["details"]
	e.FromHook, _ = raw["fromHook"].(bool)
	e.TargetID, _ = raw["targetId"].(string)
	e.Label, _ = raw["label"].(string)
	e.Name, _ = raw["name"].(string)
	e.FromID, _ = raw["fromId"].(string)
	return nil
}

func (e FileEntry) MarshalJSON() ([]byte, error) {
	if e.raw != nil {
		return json.Marshal(e.raw)
	}
	values := map[string]any{"type": e.Type}
	if e.Version != 0 {
		values["version"] = e.Version
	}
	if e.ID != "" {
		values["id"] = e.ID
	}
	if e.Timestamp != "" {
		values["timestamp"] = e.Timestamp
	}
	if e.CWD != "" {
		values["cwd"] = e.CWD
	}
	if e.ParentSession != "" {
		values["parentSession"] = e.ParentSession
	}
	if e.Type != "" && e.Type != "session" {
		values["parentId"] = nil
	}
	if e.ParentID != nil {
		values["parentId"] = *e.ParentID
	}
	if e.Message != nil {
		values["message"] = e.Message
	}
	if e.ThinkingLevel != "" {
		values["thinkingLevel"] = e.ThinkingLevel
	}
	if e.Provider != "" {
		values["provider"] = e.Provider
	}
	if e.ModelID != "" {
		values["modelId"] = e.ModelID
	}
	if e.Summary != "" {
		values["summary"] = e.Summary
	}
	if e.FirstKeptID != "" {
		values["firstKeptEntryId"] = e.FirstKeptID
	}
	if e.TokensBefore != 0 {
		values["tokensBefore"] = e.TokensBefore
	}
	if e.CustomType != "" {
		values["customType"] = e.CustomType
	}
	if e.Data != nil {
		values["data"] = e.Data
	}
	if e.Content != nil {
		values["content"] = e.Content
	}
	if e.Type == "custom_message" {
		values["display"] = e.Display
	}
	if e.Details != nil {
		values["details"] = e.Details
	}
	if e.FromHook {
		values["fromHook"] = e.FromHook
	}
	if e.TargetID != "" {
		values["targetId"] = e.TargetID
	}
	if e.Type == "label" {
		values["label"] = e.Label
	}
	if e.Name != "" {
		values["name"] = e.Name
	}
	if e.FromID != "" {
		values["fromId"] = e.FromID
	}
	return json.Marshal(values)
}

func (e FileEntry) rawValue(key string) any {
	if e.raw == nil {
		return nil
	}
	return e.raw[key]
}

func LoadEntriesFromFile(filePath string) []FileEntry {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	var entries []FileEntry
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry FileEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return entries
	}
	if entries[0].Type != "session" || entries[0].ID == "" {
		return nil
	}
	return entries
}

func MigrateSessionEntries(entries []FileEntry) bool {
	if len(entries) == 0 {
		return false
	}
	header := &entries[0]
	if header.Type != "session" {
		return false
	}
	version := header.Version
	if version == 0 {
		version = 1
	}
	if version >= CurrentSessionVersion {
		return false
	}
	changed := false
	if version < 2 {
		ids := map[string]struct{}{}
		var previousID *string
		for index := range entries {
			entry := &entries[index]
			if entry.Type == "session" {
				entry.Version = 2
				if entry.raw != nil {
					entry.raw["version"] = 2
				}
				changed = true
				continue
			}
			id := generateShortSessionEntryID(ids)
			entry.ID = id
			if entry.raw != nil {
				entry.raw["id"] = id
			}
			if previousID == nil {
				entry.ParentID = nil
				if entry.raw != nil {
					entry.raw["parentId"] = nil
				}
			} else {
				entry.ParentID = cloneStringPtr(previousID)
				if entry.raw != nil {
					entry.raw["parentId"] = *previousID
				}
			}
			previousID = stringPtr(id)
			if entry.Type == "compaction" {
				if firstKeptIndex, ok := numericRawIndex(entry.rawValue("firstKeptEntryIndex")); ok {
					if firstKeptIndex >= 0 && firstKeptIndex < len(entries) && entries[firstKeptIndex].Type != "session" {
						entry.FirstKeptID = entries[firstKeptIndex].ID
						if entry.raw != nil {
							entry.raw["firstKeptEntryId"] = entry.FirstKeptID
						}
					}
					if entry.raw != nil {
						delete(entry.raw, "firstKeptEntryIndex")
					}
				}
			}
			changed = true
		}
		version = 2
	}
	if version < 3 {
		for index := range entries {
			entry := &entries[index]
			if entry.Type == "session" {
				entry.Version = 3
				if entry.raw != nil {
					entry.raw["version"] = 3
				}
				changed = true
				continue
			}
			if entry.Type != "message" {
				continue
			}
			if messageRole(entry.Message) != "hookMessage" {
				continue
			}
			if message, ok := entry.Message.(map[string]any); ok {
				message["role"] = "custom"
			}
			if rawMessage, ok := entry.raw["message"].(map[string]any); ok {
				rawMessage["role"] = "custom"
			}
			changed = true
		}
	}
	if header.Version != CurrentSessionVersion {
		header.Version = CurrentSessionVersion
		if header.raw != nil {
			header.raw["version"] = CurrentSessionVersion
		}
		changed = true
	}
	return changed
}

func FindMostRecentSession(sessionDir string) string {
	dirEntries, err := os.ReadDir(sessionDir)
	if err != nil {
		return ""
	}
	type candidate struct {
		path  string
		mtime time.Time
	}
	var candidates []candidate
	for _, entry := range dirEntries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		path := filepath.Join(sessionDir, entry.Name())
		if !isValidSessionFile(path) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{path: path, mtime: info.ModTime()})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].mtime.After(candidates[j].mtime)
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].path
}

func BuildSessionInfo(filePath string) (*SessionInfo, error) {
	entries := LoadEntriesFromFile(filePath)
	if len(entries) == 0 || entries[0].Type != "session" {
		return nil, errors.New("invalid session file")
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	header := entries[0]
	sessionInfo := &SessionInfo{
		Path:              filePath,
		ID:                header.ID,
		CWD:               header.CWD,
		ParentSessionPath: header.ParentSession,
		Created:           parseSessionTime(header.Timestamp, info.ModTime()),
		Modified:          getSessionModifiedDate(entries, header, info.ModTime()),
		FirstMessage:      "(no messages)",
	}
	var allMessages []string
	for _, entry := range entries {
		if entry.Type == "session_info" {
			sessionInfo.Name = strings.TrimSpace(entry.Name)
			continue
		}
		if entry.Type != "message" {
			continue
		}
		sessionInfo.MessageCount++
		if role := messageRole(entry.Message); role != "user" && role != "assistant" {
			continue
		}
		text := extractMessageText(entry.Message)
		if text == "" {
			continue
		}
		allMessages = append(allMessages, text)
		if sessionInfo.FirstMessage == "(no messages)" && messageRole(entry.Message) == "user" {
			sessionInfo.FirstMessage = text
		}
	}
	sessionInfo.AllMessagesText = strings.Join(allMessages, " ")
	return sessionInfo, nil
}

func ListSessions(cwd string, args ...any) []SessionInfo {
	sessionDir := ""
	var onProgress SessionListProgress
	for _, arg := range args {
		switch value := arg.(type) {
		case string:
			sessionDir = value
		case SessionListProgress:
			onProgress = value
		}
	}
	if sessionDir == "" {
		var err error
		sessionDir, err = GetDefaultSessionDir(cwd)
		if err != nil {
			return nil
		}
	}
	sessions := listSessionsFromDir(sessionDir, onProgress, 0, 0)
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].Modified.After(sessions[j].Modified)
	})
	return sessions
}

func ListAllSessions(args ...SessionListProgress) []SessionInfo {
	var onProgress SessionListProgress
	if len(args) > 0 {
		onProgress = args[0]
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	root := filepath.Join(home, ".pi", "agent", "sessions")
	dirEntries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range dirEntries {
		if !entry.IsDir() {
			continue
		}
		childEntries, err := os.ReadDir(filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}
		for _, child := range childEntries {
			if !child.IsDir() && filepath.Ext(child.Name()) == ".jsonl" {
				files = append(files, filepath.Join(root, entry.Name(), child.Name()))
			}
		}
	}
	sessions := buildSessionInfos(files, onProgress, 0, len(files))
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].Modified.After(sessions[j].Modified)
	})
	return sessions
}

func listSessionsFromDir(dir string, onProgress SessionListProgress, progressOffset, progressTotal int) []SessionInfo {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range dirEntries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".jsonl" {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	total := progressTotal
	if total == 0 {
		total = len(files)
	}
	return buildSessionInfos(files, onProgress, progressOffset, total)
}

func buildSessionInfos(files []string, onProgress SessionListProgress, progressOffset, total int) []SessionInfo {
	sessions := make([]SessionInfo, 0, len(files))
	for index, file := range files {
		if info, err := BuildSessionInfo(file); err == nil {
			sessions = append(sessions, *info)
		}
		if onProgress != nil {
			onProgress(progressOffset+index+1, total)
		}
	}
	return sessions
}

func isValidSessionFile(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()
	line, err := bufio.NewReader(file).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	if len(line) == 0 {
		return false
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	var entry FileEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return false
	}
	return entry.Type == "session" && entry.ID != ""
}

type SessionManager struct {
	sessionID           string
	sessionFile         string
	sessionDir          string
	cwd                 string
	persist             bool
	flushed             bool
	fileEntries         []FileEntry
	byID                map[string]FileEntry
	labelsByID          map[string]string
	labelTimestampsByID map[string]string
	leafID              *string
}

type SessionModel struct {
	Provider string
	ModelID  string
}

type SessionContext struct {
	Messages      []any
	ThinkingLevel string
	Model         *SessionModel
}

type SessionTreeNode struct {
	Entry          FileEntry
	Children       []*SessionTreeNode
	Label          string
	LabelTimestamp string
}

type SessionInfo struct {
	Path              string
	ID                string
	CWD               string
	Name              string
	ParentSessionPath string
	Created           time.Time
	Modified          time.Time
	MessageCount      int
	FirstMessage      string
	AllMessagesText   string
}

type SessionListProgress func(loaded int, total int)

func OpenSessionManager(path string, args ...string) (*SessionManager, error) {
	sessionDir := ""
	cwdOverride := ""
	if len(args) > 0 {
		sessionDir = args[0]
	}
	if len(args) > 1 {
		cwdOverride = args[1]
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	entries := LoadEntriesFromFile(absPath)
	cwd := cwdOverride
	if cwd == "" && len(entries) > 0 && entries[0].CWD != "" {
		cwd = entries[0].CWD
	}
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	if sessionDir == "" {
		sessionDir = filepath.Dir(absPath)
	}
	return newSessionManager(cwd, sessionDir, absPath, true)
}

func CreateSessionManager(cwd string, sessionDir ...string) (*SessionManager, error) {
	dir := ""
	if len(sessionDir) > 0 {
		dir = sessionDir[0]
	} else {
		var err error
		dir, err = GetDefaultSessionDir(cwd)
		if err != nil {
			return nil, err
		}
	}
	return newSessionManager(cwd, dir, "", true)
}

func ContinueRecentSession(cwd string, sessionDir ...string) (*SessionManager, error) {
	dir := ""
	if len(sessionDir) > 0 {
		dir = sessionDir[0]
	} else {
		var err error
		dir, err = GetDefaultSessionDir(cwd)
		if err != nil {
			return nil, err
		}
	}
	if recent := FindMostRecentSession(dir); recent != "" {
		return newSessionManager(cwd, dir, recent, true)
	}
	return newSessionManager(cwd, dir, "", true)
}

func ForkSessionFrom(sourcePath, targetCwd string, sessionDir ...string) (*SessionManager, error) {
	sourceEntries := LoadEntriesFromFile(sourcePath)
	if len(sourceEntries) == 0 {
		return nil, errors.New("Cannot fork: source session file is empty or invalid: " + sourcePath)
	}
	if sourceEntries[0].Type != "session" {
		return nil, errors.New("Cannot fork: source session has no header: " + sourcePath)
	}
	dir := ""
	if len(sessionDir) > 0 {
		dir = sessionDir[0]
	} else {
		var err error
		dir, err = GetDefaultSessionDir(targetCwd)
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	newSessionID := agentharness.UUIDv7()
	timestamp := sessionTimestamp(time.Now())
	fileTimestamp := strings.NewReplacer(":", "-", ".", "-").Replace(timestamp)
	newSessionFile := filepath.Join(dir, fileTimestamp+"_"+newSessionID+".jsonl")
	header := FileEntry{
		Type:          "session",
		Version:       CurrentSessionVersion,
		ID:            newSessionID,
		Timestamp:     timestamp,
		CWD:           targetCwd,
		ParentSession: sourcePath,
	}
	entries := []FileEntry{header}
	for _, entry := range sourceEntries {
		if entry.Type != "session" {
			entries = append(entries, entry)
		}
	}
	var builder strings.Builder
	for _, entry := range entries {
		line, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		builder.Write(line)
		builder.WriteByte('\n')
	}
	if err := os.WriteFile(newSessionFile, []byte(builder.String()), 0o644); err != nil {
		return nil, err
	}
	return newSessionManager(targetCwd, dir, newSessionFile, true)
}

func InMemorySessionManager(cwd ...string) (*SessionManager, error) {
	workingDir := ""
	if len(cwd) > 0 {
		workingDir = cwd[0]
	} else {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	return newSessionManager(workingDir, "", "", false)
}

func GetDefaultSessionDir(cwd string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	safePath := strings.TrimLeft(cwd, `/\`)
	replacer := strings.NewReplacer("/", "-", `\`, "-", ":", "-")
	dir := filepath.Join(home, ".pi", "agent", "sessions", "--"+replacer.Replace(safePath)+"--")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func newSessionManager(cwd, sessionDir, sessionFile string, persist bool) (*SessionManager, error) {
	sm := &SessionManager{
		cwd:                 cwd,
		sessionDir:          sessionDir,
		persist:             persist,
		byID:                map[string]FileEntry{},
		labelsByID:          map[string]string{},
		labelTimestampsByID: map[string]string{},
	}
	if persist && sessionDir != "" {
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			return nil, err
		}
	}
	if sessionFile != "" {
		if err := sm.SetSessionFile(sessionFile); err != nil {
			return nil, err
		}
	} else {
		sm.newSession("")
	}
	return sm, nil
}

func (s *SessionManager) SetSessionFile(sessionFile string) error {
	absPath, err := filepath.Abs(sessionFile)
	if err != nil {
		return err
	}
	s.sessionFile = absPath
	if _, err := os.Stat(absPath); err == nil {
		s.fileEntries = LoadEntriesFromFile(absPath)
		if len(s.fileEntries) == 0 {
			explicitPath := s.sessionFile
			s.newSession("")
			s.sessionFile = explicitPath
			if err := s.rewriteFile(); err != nil {
				return err
			}
			s.flushed = true
			return nil
		}
		header := s.fileEntries[0]
		s.sessionID = header.ID
		if s.sessionID == "" {
			s.sessionID = agentharness.UUIDv7()
		}
		if MigrateSessionEntries(s.fileEntries) {
			if err := s.rewriteFile(); err != nil {
				return err
			}
		}
		s.buildIndex()
		s.flushed = true
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	explicitPath := s.sessionFile
	s.newSession("")
	s.sessionFile = explicitPath
	return nil
}

func (s *SessionManager) NewSession(options ...NewSessionOptions) string {
	opts := NewSessionOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	return s.newSession(opts)
}

func (s *SessionManager) newSession(options any) string {
	opts := NewSessionOptions{}
	switch value := options.(type) {
	case string:
		opts.ParentSession = value
	case NewSessionOptions:
		opts = value
	}
	s.sessionID = opts.ID
	if s.sessionID == "" {
		s.sessionID = agentharness.UUIDv7()
	}
	timestamp := sessionTimestamp(time.Now())
	s.fileEntries = []FileEntry{{
		Type:          "session",
		Version:       CurrentSessionVersion,
		ID:            s.sessionID,
		Timestamp:     timestamp,
		CWD:           s.cwd,
		ParentSession: opts.ParentSession,
	}}
	s.buildIndex()
	s.flushed = false
	if s.persist {
		fileTimestamp := strings.NewReplacer(":", "-", ".", "-").Replace(timestamp)
		s.sessionFile = filepath.Join(s.GetSessionDir(), fileTimestamp+"_"+s.sessionID+".jsonl")
	}
	return s.sessionFile
}

func (s *SessionManager) rewriteFile() error {
	if !s.persist || s.sessionFile == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.sessionFile), 0o755); err != nil {
		return err
	}
	var builder strings.Builder
	for _, entry := range s.fileEntries {
		line, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		builder.Write(line)
		builder.WriteByte('\n')
	}
	return os.WriteFile(s.sessionFile, []byte(builder.String()), 0o644)
}

func (s *SessionManager) appendEntry(entry FileEntry) {
	s.fileEntries = append(s.fileEntries, entry)
	s.indexEntry(entry)
	s.leafID = stringPtr(entry.ID)
	s.persistEntry(entry)
}

func (s *SessionManager) persistEntry(entry FileEntry) {
	if !s.persist || s.sessionFile == "" {
		return
	}
	if !s.hasAssistantMessage() {
		s.flushed = false
		return
	}
	if !s.flushed {
		_ = s.rewriteFile()
		s.flushed = true
		return
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	file, err := os.OpenFile(s.sessionFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(line, '\n'))
}

func (s *SessionManager) hasAssistantMessage() bool {
	for _, entry := range s.fileEntries {
		if entry.Type == "message" && messageRole(entry.Message) == "assistant" {
			return true
		}
	}
	return false
}

func (s *SessionManager) buildIndex() {
	if s.byID == nil {
		s.byID = map[string]FileEntry{}
	}
	if s.labelsByID == nil {
		s.labelsByID = map[string]string{}
	}
	if s.labelTimestampsByID == nil {
		s.labelTimestampsByID = map[string]string{}
	}
	clear(s.byID)
	clear(s.labelsByID)
	clear(s.labelTimestampsByID)
	s.leafID = nil
	for _, entry := range s.fileEntries {
		if entry.Type == "session" {
			continue
		}
		s.indexEntry(entry)
		s.leafID = stringPtr(entry.ID)
	}
}

func (s *SessionManager) indexEntry(entry FileEntry) {
	if entry.ID != "" {
		s.byID[entry.ID] = entry
	}
	if entry.Type != "label" || entry.TargetID == "" {
		return
	}
	if entry.Label == "" {
		delete(s.labelsByID, entry.TargetID)
		delete(s.labelTimestampsByID, entry.TargetID)
		return
	}
	s.labelsByID[entry.TargetID] = entry.Label
	s.labelTimestampsByID[entry.TargetID] = entry.Timestamp
}

func (s *SessionManager) IsPersisted() bool {
	return s.persist
}

func (s *SessionManager) GetSessionID() string {
	return s.sessionID
}

func (s *SessionManager) GetSessionId() string {
	return s.GetSessionID()
}

func (s *SessionManager) GetSessionFile() string {
	return s.sessionFile
}

func (s *SessionManager) GetSessionDir() string {
	return s.sessionDir
}

func (s *SessionManager) GetCWD() string {
	return s.cwd
}

func (s *SessionManager) GetCwd() string {
	return s.GetCWD()
}

func (s *SessionManager) GetHeader() *SessionHeader {
	for _, entry := range s.fileEntries {
		if entry.Type != "session" {
			continue
		}
		return &SessionHeader{
			Type:          entry.Type,
			Version:       entry.Version,
			ID:            entry.ID,
			Timestamp:     entry.Timestamp,
			CWD:           entry.CWD,
			ParentSession: entry.ParentSession,
		}
	}
	return nil
}

func (s *SessionManager) GetEntries() []FileEntry {
	entries := make([]FileEntry, 0, len(s.fileEntries))
	for _, entry := range s.fileEntries {
		if entry.Type == "session" {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func (s *SessionManager) AppendMessage(message any) string {
	parentID := cloneStringPtr(s.leafID)
	entry := newSessionEntry("message", parentID, map[string]any{"message": message})
	entry.Message = message
	s.appendEntry(entry)
	return entry.ID
}

func (s *SessionManager) AppendThinkingLevelChange(thinkingLevel string) string {
	parentID := cloneStringPtr(s.leafID)
	entry := newSessionEntry("thinking_level_change", parentID, map[string]any{"thinkingLevel": thinkingLevel})
	entry.ThinkingLevel = thinkingLevel
	s.appendEntry(entry)
	return entry.ID
}

func (s *SessionManager) AppendModelChange(provider, modelID string) string {
	parentID := cloneStringPtr(s.leafID)
	entry := newSessionEntry("model_change", parentID, map[string]any{"provider": provider, "modelId": modelID})
	entry.Provider = provider
	entry.ModelID = modelID
	s.appendEntry(entry)
	return entry.ID
}

func (s *SessionManager) AppendCompaction(summary, firstKeptEntryID string, tokensBefore int) string {
	parentID := cloneStringPtr(s.leafID)
	entry := newSessionEntry("compaction", parentID, map[string]any{
		"summary":          summary,
		"firstKeptEntryId": firstKeptEntryID,
		"tokensBefore":     tokensBefore,
	})
	entry.Summary = summary
	entry.FirstKeptID = firstKeptEntryID
	entry.TokensBefore = tokensBefore
	s.appendEntry(entry)
	return entry.ID
}

func (s *SessionManager) AppendCustomEntry(customType string, data any) string {
	parentID := cloneStringPtr(s.leafID)
	entry := newSessionEntry("custom", parentID, map[string]any{"customType": customType, "data": data})
	entry.CustomType = customType
	entry.Data = data
	s.appendEntry(entry)
	return entry.ID
}

func (s *SessionManager) AppendCustomMessageEntry(customType string, content any, display bool, details any) string {
	parentID := cloneStringPtr(s.leafID)
	entry := newSessionEntry("custom_message", parentID, map[string]any{
		"customType": customType,
		"content":    content,
		"display":    display,
		"details":    details,
	})
	entry.CustomType = customType
	entry.Content = content
	entry.Display = display
	entry.Details = details
	s.appendEntry(entry)
	return entry.ID
}

func (s *SessionManager) AppendSessionInfo(name string) string {
	name = strings.TrimSpace(name)
	parentID := cloneStringPtr(s.leafID)
	entry := newSessionEntry("session_info", parentID, map[string]any{"name": name})
	entry.Name = name
	s.appendEntry(entry)
	return entry.ID
}

func (s *SessionManager) AppendLabelChange(targetID, label string) (string, error) {
	if _, ok := s.byID[targetID]; !ok {
		return "", errors.New("Entry " + targetID + " not found")
	}
	parentID := cloneStringPtr(s.leafID)
	entry := newSessionEntry("label", parentID, map[string]any{"targetId": targetID, "label": label})
	entry.TargetID = targetID
	entry.Label = label
	s.appendEntry(entry)
	return entry.ID, nil
}

func (s *SessionManager) GetLeafID() *string {
	return cloneStringPtr(s.leafID)
}

func (s *SessionManager) GetLeafId() *string {
	return s.GetLeafID()
}

func (s *SessionManager) GetLeafEntry() *FileEntry {
	if s.leafID == nil {
		return nil
	}
	return s.GetEntry(*s.leafID)
}

func (s *SessionManager) GetEntry(id string) *FileEntry {
	entry, ok := s.byID[id]
	if !ok {
		return nil
	}
	return &entry
}

func (s *SessionManager) GetChildren(parentID string) []FileEntry {
	var children []FileEntry
	for _, entry := range s.GetEntries() {
		if entry.ParentID != nil && *entry.ParentID == parentID {
			children = append(children, entry)
		}
	}
	return children
}

func (s *SessionManager) GetLabel(id string) (string, bool) {
	label, ok := s.labelsByID[id]
	return label, ok
}

func (s *SessionManager) GetSessionName() string {
	for i := len(s.fileEntries) - 1; i >= 0; i-- {
		entry := s.fileEntries[i]
		if entry.Type == "session_info" {
			return strings.TrimSpace(entry.Name)
		}
	}
	return ""
}

func (s *SessionManager) GetBranch(fromID ...string) []FileEntry {
	var startID *string
	if len(fromID) > 0 {
		startID = &fromID[0]
	} else {
		startID = s.leafID
	}
	if startID == nil {
		return nil
	}
	var path []FileEntry
	current, ok := s.byID[*startID]
	for ok {
		path = append([]FileEntry{current}, path...)
		if current.ParentID == nil {
			break
		}
		current, ok = s.byID[*current.ParentID]
	}
	return path
}

func (s *SessionManager) BuildSessionContext() SessionContext {
	return BuildSessionContext(s.GetEntries(), s.leafID, s.byID)
}

func BuildSessionContext(entries []FileEntry, leafID *string, byID map[string]FileEntry) SessionContext {
	if byID == nil {
		byID = map[string]FileEntry{}
		for _, entry := range entries {
			if entry.ID != "" {
				byID[entry.ID] = entry
			}
		}
	}
	context := SessionContext{ThinkingLevel: "off"}
	if leafID == nil {
		return context
	}
	var leaf FileEntry
	var ok bool
	leaf, ok = byID[*leafID]
	if !ok && len(entries) > 0 {
		leaf = entries[len(entries)-1]
		ok = true
	}
	if !ok {
		return context
	}
	var path []FileEntry
	current := leaf
	for {
		path = append([]FileEntry{current}, path...)
		if current.ParentID == nil {
			break
		}
		next, ok := byID[*current.ParentID]
		if !ok {
			break
		}
		current = next
	}

	compactionIndex := -1
	for idx, entry := range path {
		switch entry.Type {
		case "thinking_level_change":
			context.ThinkingLevel = entry.ThinkingLevel
		case "model_change":
			context.Model = &SessionModel{Provider: entry.Provider, ModelID: entry.ModelID}
		case "message":
			if messageRole(entry.Message) == "assistant" {
				if provider, modelID := messageProviderModel(entry.Message); provider != "" || modelID != "" {
					context.Model = &SessionModel{Provider: provider, ModelID: modelID}
				}
			}
		case "compaction":
			compactionIndex = idx
		}
	}

	appendEntryMessage := func(entry FileEntry) {
		switch entry.Type {
		case "message":
			context.Messages = append(context.Messages, entry.Message)
		case "custom_message":
			context.Messages = append(context.Messages, map[string]any{
				"role":      entry.CustomType,
				"content":   []any{map[string]any{"type": "text", "text": entry.Content}},
				"timestamp": entry.Timestamp,
				"details":   entry.Details,
			})
		case "branch_summary":
			if entry.Summary != "" {
				context.Messages = append(context.Messages, map[string]any{
					"role":      "branchSummary",
					"content":   []any{map[string]any{"type": "text", "text": entry.Summary}},
					"timestamp": entry.Timestamp,
				})
			}
		}
	}

	if compactionIndex >= 0 {
		compaction := path[compactionIndex]
		context.Messages = append(context.Messages, map[string]any{
			"role":      "compactionSummary",
			"content":   []any{map[string]any{"type": "text", "text": compaction.Summary}},
			"timestamp": compaction.Timestamp,
		})
		foundFirstKept := false
		for idx := 0; idx < compactionIndex; idx++ {
			entry := path[idx]
			if entry.ID == compaction.FirstKeptID {
				foundFirstKept = true
			}
			if foundFirstKept {
				appendEntryMessage(entry)
			}
		}
		for idx := compactionIndex + 1; idx < len(path); idx++ {
			appendEntryMessage(path[idx])
		}
		return context
	}
	for _, entry := range path {
		appendEntryMessage(entry)
	}
	return context
}

func (s *SessionManager) GetTree() []*SessionTreeNode {
	entries := s.GetEntries()
	nodeMap := map[string]*SessionTreeNode{}
	roots := []*SessionTreeNode{}
	for _, entry := range entries {
		nodeMap[entry.ID] = &SessionTreeNode{
			Entry:          entry,
			Label:          s.labelsByID[entry.ID],
			LabelTimestamp: s.labelTimestampsByID[entry.ID],
		}
	}
	for _, entry := range entries {
		node := nodeMap[entry.ID]
		if entry.ParentID == nil || *entry.ParentID == entry.ID {
			roots = append(roots, node)
			continue
		}
		parent := nodeMap[*entry.ParentID]
		if parent == nil {
			roots = append(roots, node)
			continue
		}
		parent.Children = append(parent.Children, node)
	}
	stack := append([]*SessionTreeNode{}, roots...)
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		sort.SliceStable(node.Children, func(i, j int) bool {
			return node.Children[i].Entry.Timestamp < node.Children[j].Entry.Timestamp
		})
		stack = append(stack, node.Children...)
	}
	return roots
}

func (s *SessionManager) Branch(branchFromID string) error {
	if _, ok := s.byID[branchFromID]; !ok {
		return errors.New("Entry " + branchFromID + " not found")
	}
	s.leafID = stringPtr(branchFromID)
	return nil
}

func (s *SessionManager) ResetLeaf() {
	s.leafID = nil
}

func (s *SessionManager) BranchWithSummary(branchFromID *string, summary string) (string, error) {
	if branchFromID != nil {
		if _, ok := s.byID[*branchFromID]; !ok {
			return "", errors.New("Entry " + *branchFromID + " not found")
		}
	}
	s.leafID = cloneStringPtr(branchFromID)
	parentID := cloneStringPtr(branchFromID)
	fromID := "root"
	if branchFromID != nil {
		fromID = *branchFromID
	}
	entry := newSessionEntry("branch_summary", parentID, map[string]any{
		"fromId":  fromID,
		"summary": summary,
	})
	entry.Summary = summary
	s.appendEntry(entry)
	return entry.ID, nil
}

func (s *SessionManager) CreateBranchedSession(leafID string) (string, error) {
	previousSessionFile := s.sessionFile
	path := s.GetBranch(leafID)
	if len(path) == 0 {
		return "", errors.New("Entry " + leafID + " not found")
	}
	pathWithoutLabels := make([]FileEntry, 0, len(path))
	for _, entry := range path {
		if entry.Type != "label" {
			pathWithoutLabels = append(pathWithoutLabels, entry)
		}
	}
	newSessionID := agentharness.UUIDv7()
	timestamp := sessionTimestamp(time.Now())
	header := FileEntry{
		Type:          "session",
		Version:       CurrentSessionVersion,
		ID:            newSessionID,
		Timestamp:     timestamp,
		CWD:           s.cwd,
		ParentSession: "",
	}
	if s.persist {
		header.ParentSession = previousSessionFile
	}
	fileTimestamp := strings.NewReplacer(":", "-", ".", "-").Replace(timestamp)
	newSessionFile := ""
	if s.persist {
		newSessionFile = filepath.Join(s.GetSessionDir(), fileTimestamp+"_"+newSessionID+".jsonl")
	}
	labels := s.labelEntriesForPath(pathWithoutLabels)
	s.fileEntries = append([]FileEntry{header}, pathWithoutLabels...)
	s.fileEntries = append(s.fileEntries, labels...)
	s.sessionID = newSessionID
	if s.persist {
		s.sessionFile = newSessionFile
	}
	s.buildIndex()
	if s.persist {
		if s.hasAssistantMessage() {
			if err := s.rewriteFile(); err != nil {
				return "", err
			}
			s.flushed = true
		} else {
			s.flushed = false
		}
	}
	return newSessionFile, nil
}

func (s *SessionManager) labelEntriesForPath(path []FileEntry) []FileEntry {
	pathIDs := map[string]struct{}{}
	parentID := (*string)(nil)
	if len(path) > 0 {
		parentID = stringPtr(path[len(path)-1].ID)
	}
	for _, entry := range path {
		pathIDs[entry.ID] = struct{}{}
	}
	var labels []FileEntry
	for targetID, label := range s.labelsByID {
		if _, ok := pathIDs[targetID]; !ok {
			continue
		}
		entry := newSessionEntry("label", cloneStringPtr(parentID), map[string]any{
			"targetId": targetID,
			"label":    label,
		})
		if ts := s.labelTimestampsByID[targetID]; ts != "" {
			entry.Timestamp = ts
			entry.raw["timestamp"] = ts
		}
		entry.TargetID = targetID
		entry.Label = label
		labels = append(labels, entry)
		parentID = stringPtr(entry.ID)
	}
	return labels
}

func sessionTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func newSessionEntry(entryType string, parentID *string, fields map[string]any) FileEntry {
	id := newSessionEntryID()
	timestamp := sessionTimestamp(time.Now())
	raw := map[string]any{
		"type":      entryType,
		"id":        id,
		"parentId":  nil,
		"timestamp": timestamp,
	}
	if parentID != nil {
		raw["parentId"] = *parentID
	}
	for key, value := range fields {
		raw[key] = value
	}
	entry := FileEntry{Type: entryType, ID: id, ParentID: cloneStringPtr(parentID), Timestamp: timestamp, raw: raw}
	return entry
}

func newSessionEntryID() string {
	return agentharness.UUIDv7()
}

func generateShortSessionEntryID(existing map[string]struct{}) string {
	for i := 0; i < 100; i++ {
		var bytes [4]byte
		if _, err := rand.Read(bytes[:]); err == nil {
			id := hex.EncodeToString(bytes[:])
			if _, ok := existing[id]; !ok {
				existing[id] = struct{}{}
				return id
			}
		}
	}
	id := agentharness.UUIDv7()
	existing[id] = struct{}{}
	return id
}

func numericRawIndex(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	default:
		return 0, false
	}
}

func stringPtr(value string) *string {
	copy := value
	return &copy
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	return stringPtr(*value)
}

func messageRole(message any) string {
	value, ok := message.(map[string]any)
	if !ok {
		return ""
	}
	role, _ := value["role"].(string)
	return role
}

func messageProviderModel(message any) (string, string) {
	value, ok := message.(map[string]any)
	if !ok {
		return "", ""
	}
	provider, _ := value["provider"].(string)
	modelID, _ := value["model"].(string)
	return provider, modelID
}

func extractMessageText(message any) string {
	value, ok := message.(map[string]any)
	if !ok {
		return ""
	}
	content := value["content"]
	switch typed := content.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, block := range typed {
			blockMap, ok := block.(map[string]any)
			if !ok {
				continue
			}
			blockType, _ := blockMap["type"].(string)
			if blockType != "text" {
				continue
			}
			text, _ := blockMap["text"].(string)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func messageTimestampMillis(message any) (int64, bool) {
	value, ok := message.(map[string]any)
	if !ok {
		return 0, false
	}
	switch timestamp := value["timestamp"].(type) {
	case float64:
		return int64(timestamp), true
	case int:
		return int64(timestamp), true
	case int64:
		return timestamp, true
	default:
		return 0, false
	}
}

func getSessionModifiedDate(entries []FileEntry, header FileEntry, statsMtime time.Time) time.Time {
	var lastActivity int64
	for _, entry := range entries {
		if entry.Type != "message" {
			continue
		}
		role := messageRole(entry.Message)
		if role != "user" && role != "assistant" {
			continue
		}
		if timestamp, ok := messageTimestampMillis(entry.Message); ok && timestamp > lastActivity {
			lastActivity = timestamp
			continue
		}
		if parsed, ok := parseSessionTimeOK(entry.Timestamp); ok {
			ms := parsed.UnixMilli()
			if ms > lastActivity {
				lastActivity = ms
			}
		}
	}
	if lastActivity > 0 {
		return time.UnixMilli(lastActivity)
	}
	return parseSessionTime(header.Timestamp, statsMtime)
}

func parseSessionTime(value string, fallback time.Time) time.Time {
	if parsed, ok := parseSessionTimeOK(value); ok {
		return parsed
	}
	return fallback
}

func parseSessionTimeOK(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, true
	}
	if parsed, err := time.Parse("2006-01-02T15:04:05.000Z", value); err == nil {
		return parsed, true
	}
	return time.Time{}, false
}
