package gicodingagent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	CurrentSessionVersion      = 3
	sessionIDValidationMessage = "Session id must be non-empty, contain only alphanumeric characters, '-', '_', and '.', and start and end with an alphanumeric character"
)

var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$`)

// ValidateSessionID applies the portable filename contract used by Pi.
func ValidateSessionID(id string) error {
	if !sessionIDPattern.MatchString(id) {
		return errors.New(sessionIDValidationMessage)
	}
	return nil
}

type SessionHeader struct {
	Type          string `json:"type"`
	Version       int    `json:"version,omitempty"`
	ID            string `json:"id"`
	Timestamp     string `json:"timestamp"`
	CWD           string `json:"cwd"`
	ParentSession string `json:"parentSession,omitempty"`
}

type NewSessionOptions struct {
	// ID is optional. The zero value requests a generated UUIDv7.
	ID            string
	ParentSession string
}

type SessionSummaryOptions struct {
	Details  any
	FromHook bool
	Usage    *llm.Usage
}

// SessionContextOptions controls the projection from the append-only session
// tree into the live provider context. Exclusions do not mutate persisted
// history; they only hide selected entries from the model-facing view.
type SessionContextOptions struct {
	ExcludeEntryIDs map[string]struct{}
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
	Usage         *llm.Usage
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
	if usage := raw["usage"]; usage != nil {
		data, err := json.Marshal(usage)
		if err == nil {
			var decoded llm.Usage
			if json.Unmarshal(data, &decoded) == nil {
				e.Usage = &decoded
			}
		}
	}
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
	if e.Usage != nil {
		values["usage"] = e.Usage
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

func getSessionHeaderCWD(header *SessionHeader) string {
	if header == nil {
		return ""
	}
	return header.CWD
}

func resolveSessionPath(path string) (string, error) {
	return filepath.Abs(ExpandPath(path))
}

func sessionCWDMatches(storedCWD, resolvedCWD string) bool {
	if storedCWD == "" || resolvedCWD == "" {
		return false
	}
	resolvedStoredCWD, err := resolveSessionPath(storedCWD)
	if err != nil {
		return false
	}
	return resolvedStoredCWD == resolvedCWD
}

func FindMostRecentSession(sessionDir string, cwd ...string) string {
	dirEntries, err := os.ReadDir(sessionDir)
	if err != nil {
		return ""
	}
	resolvedCWD := ""
	if len(cwd) > 0 && cwd[0] != "" {
		resolvedCWD, err = resolveSessionPath(cwd[0])
		if err != nil {
			return ""
		}
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
		header := readSessionHeaderForDiscovery(path)
		if header == nil {
			continue
		}
		if resolvedCWD != "" && !sessionCWDMatches(getSessionHeaderCWD(header), resolvedCWD) {
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
	explicitSessionDir := false
	var onProgress SessionListProgress
	for _, arg := range args {
		switch value := arg.(type) {
		case string:
			sessionDir = value
			explicitSessionDir = true
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
	if explicitSessionDir && !sessionDirMatchesDefault(sessionDir, cwd) {
		resolvedCWD, err := resolveSessionPath(cwd)
		if err != nil {
			return nil
		}
		filtered := sessions[:0]
		for _, session := range sessions {
			if sessionCWDMatches(session.CWD, resolvedCWD) {
				filtered = append(filtered, session)
			}
		}
		sessions = filtered
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].Modified.After(sessions[j].Modified)
	})
	return sessions
}

func ListAllSessions(args ...any) []SessionInfo {
	var onProgress SessionListProgress
	root := ""
	for _, arg := range args {
		switch value := arg.(type) {
		case SessionListProgress:
			onProgress = value
		case string:
			root = value
		}
	}
	if root == "" {
		root = defaultSessionRoot()
	}
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

func defaultSessionRoot() string {
	agentDir, err := defaultSessionAgentDir()
	if err != nil {
		return ""
	}
	return filepath.Join(agentDir, "sessions")
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
	return readSessionHeaderForDiscovery(filePath) != nil
}

type SessionManager struct {
	mu                  sync.RWMutex
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
	absPath, err := resolveSessionPath(path)
	if err != nil {
		return nil, err
	}
	var preloadedEntries *[]FileEntry
	cwd := cwdOverride
	if cwd == "" {
		if _, statErr := os.Stat(absPath); statErr == nil {
			header, headerErr := readSessionHeader(absPath)
			if headerErr != nil {
				var limitErr *SessionHeaderScanLimitError
				if !errors.As(headerErr, &limitErr) {
					return nil, headerErr
				}
				entries, loadErr := loadEntriesFromFile(absPath)
				if loadErr != nil {
					return nil, loadErr
				}
				preloadedEntries = &entries
				if len(entries) > 0 && entries[0].Type == "session" {
					header = &SessionHeader{
						Type:          entries[0].Type,
						Version:       entries[0].Version,
						ID:            entries[0].ID,
						Timestamp:     entries[0].Timestamp,
						CWD:           entries[0].CWD,
						ParentSession: entries[0].ParentSession,
					}
				}
			}
			cwd = getSessionHeaderCWD(header)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return nil, statErr
		}
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
	return newSessionManagerWithEntries(cwd, sessionDir, absPath, true, preloadedEntries)
}

func CreateSessionManager(cwd string, sessionDir ...string) (*SessionManager, error) {
	dir, err := resolveNewSessionDir(cwd, sessionDir...)
	if err != nil {
		return nil, err
	}
	return newSessionManager(cwd, dir, "", true)
}

// CreateSessionManagerWithOptions creates a persisted session with explicit
// identity or parent metadata.
func CreateSessionManagerWithOptions(
	cwd, sessionDir string,
	options NewSessionOptions,
) (*SessionManager, error) {
	if options.ID != "" {
		if err := ValidateSessionID(options.ID); err != nil {
			return nil, err
		}
	}
	dir := sessionDir
	var err error
	if dir == "" {
		dir, err = GetDefaultSessionDir(cwd)
	}
	if err != nil {
		return nil, err
	}
	return newSessionManagerWithInitialOptions(cwd, dir, true, options)
}

func resolveNewSessionDir(cwd string, sessionDir ...string) (string, error) {
	if len(sessionDir) > 0 {
		return sessionDir[0], nil
	}
	return GetDefaultSessionDir(cwd)
}

func ContinueRecentSession(cwd string, sessionDir ...string) (*SessionManager, error) {
	dir := ""
	filterCWD := false
	if len(sessionDir) > 0 {
		dir = sessionDir[0]
		filterCWD = !sessionDirMatchesDefault(dir, cwd)
	} else {
		var err error
		dir, err = GetDefaultSessionDir(cwd)
		if err != nil {
			return nil, err
		}
	}
	recent := ""
	if filterCWD {
		recent = FindMostRecentSession(dir, cwd)
	} else {
		recent = FindMostRecentSession(dir)
	}
	if recent != "" {
		return OpenSessionManager(recent, dir)
	}
	return newSessionManager(cwd, dir, "", true)
}

func ForkSessionFrom(sourcePath, targetCwd string, sessionDir ...string) (*SessionManager, error) {
	options := NewSessionOptions{}
	return forkSessionFrom(sourcePath, targetCwd, options, sessionDir...)
}

// ForkSessionFromWithOptions copies a session into a new exclusively-created
// file and optionally assigns a portable custom ID.
func ForkSessionFromWithOptions(
	sourcePath, targetCwd string,
	options NewSessionOptions,
	sessionDir ...string,
) (*SessionManager, error) {
	return forkSessionFrom(sourcePath, targetCwd, options, sessionDir...)
}

func forkSessionFrom(
	sourcePath, targetCwd string,
	options NewSessionOptions,
	sessionDir ...string,
) (*SessionManager, error) {
	return forkSessionFromAt(sourcePath, targetCwd, options, time.Now(), sessionDir...)
}

func forkSessionFromAt(
	sourcePath, targetCwd string,
	options NewSessionOptions,
	now time.Time,
	sessionDir ...string,
) (*SessionManager, error) {
	if options.ID != "" {
		if err := ValidateSessionID(options.ID); err != nil {
			return nil, err
		}
	}
	resolvedSourcePath, err := resolveSessionPath(sourcePath)
	if err != nil {
		return nil, err
	}
	resolvedTargetCWD, err := resolveSessionPath(targetCwd)
	if err != nil {
		return nil, err
	}
	sourceEntries, err := loadEntriesFromFile(resolvedSourcePath)
	if err != nil {
		return nil, err
	}
	if len(sourceEntries) == 0 {
		return nil, errors.New("Cannot fork: source session file is empty or invalid: " + resolvedSourcePath)
	}
	if sourceEntries[0].Type != "session" {
		return nil, errors.New("Cannot fork: source session has no header: " + resolvedSourcePath)
	}
	dir := ""
	if len(sessionDir) > 0 {
		dir, err = resolveSessionPath(sessionDir[0])
		if err != nil {
			return nil, err
		}
	} else {
		dir, err = GetDefaultSessionDir(resolvedTargetCWD)
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	newSessionID := options.ID
	if newSessionID == "" {
		newSessionID = agentharness.UUIDv7()
	}
	timestamp := sessionTimestamp(now)
	fileTimestamp := strings.NewReplacer(":", "-", ".", "-").Replace(timestamp)
	newSessionFile := filepath.Join(dir, fileTimestamp+"_"+newSessionID+".jsonl")
	header := FileEntry{
		Type:          "session",
		Version:       CurrentSessionVersion,
		ID:            newSessionID,
		Timestamp:     timestamp,
		CWD:           resolvedTargetCWD,
		ParentSession: resolvedSourcePath,
	}
	entries := []FileEntry{header}
	for _, entry := range sourceEntries {
		if entry.Type != "session" {
			entries = append(entries, entry)
		}
	}
	if err := writeSessionEntries(
		newSessionFile,
		entries,
		os.O_CREATE|os.O_WRONLY|os.O_EXCL,
	); err != nil {
		return nil, err
	}
	return newSessionManager(resolvedTargetCWD, dir, newSessionFile, true)
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

// InMemorySessionManagerWithOptions creates a non-persisted session with
// explicit identity or parent metadata.
func InMemorySessionManagerWithOptions(
	cwd string,
	options NewSessionOptions,
) (*SessionManager, error) {
	if options.ID != "" {
		if err := ValidateSessionID(options.ID); err != nil {
			return nil, err
		}
	}
	return newSessionManagerWithInitialOptions(cwd, "", false, options)
}

func defaultSessionAgentDir() (string, error) {
	return resolveSessionPath(GetAgentDir())
}

func getDefaultSessionDirPath(cwd, agentDir string) (string, error) {
	resolvedCWD, err := resolveSessionPath(cwd)
	if err != nil {
		return "", err
	}
	resolvedAgentDir, err := resolveSessionPath(agentDir)
	if err != nil {
		return "", err
	}
	safePath := strings.TrimLeft(resolvedCWD, `/\`)
	replacer := strings.NewReplacer("/", "-", `\`, "-", ":", "-")
	return filepath.Join(resolvedAgentDir, "sessions", "--"+replacer.Replace(safePath)+"--"), nil
}

// GetDefaultSessionDirPath computes the session directory without creating it.
func GetDefaultSessionDirPath(cwd string) (string, error) {
	agentDir, err := defaultSessionAgentDir()
	if err != nil {
		return "", err
	}
	return getDefaultSessionDirPath(cwd, agentDir)
}

func GetDefaultSessionDir(cwd string) (string, error) {
	dir, err := GetDefaultSessionDirPath(cwd)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func sessionDirMatchesDefault(sessionDir, cwd string) bool {
	defaultDir, err := GetDefaultSessionDirPath(cwd)
	if err != nil {
		return false
	}
	resolvedDir, err := resolveSessionPath(sessionDir)
	if err != nil {
		return false
	}
	return resolvedDir == defaultDir
}

func newSessionManager(cwd, sessionDir, sessionFile string, persist bool) (*SessionManager, error) {
	return newSessionManagerWithState(cwd, sessionDir, sessionFile, persist, nil, nil)
}

func newSessionManagerWithEntries(
	cwd, sessionDir, sessionFile string,
	persist bool,
	preloadedEntries *[]FileEntry,
) (*SessionManager, error) {
	return newSessionManagerWithState(
		cwd,
		sessionDir,
		sessionFile,
		persist,
		preloadedEntries,
		nil,
	)
}

func newSessionManagerWithInitialOptions(
	cwd, sessionDir string,
	persist bool,
	options NewSessionOptions,
) (*SessionManager, error) {
	return newSessionManagerWithState(
		cwd,
		sessionDir,
		"",
		persist,
		nil,
		&options,
	)
}

func newSessionManagerWithState(
	cwd, sessionDir, sessionFile string,
	persist bool,
	preloadedEntries *[]FileEntry,
	initialOptions *NewSessionOptions,
) (*SessionManager, error) {
	resolvedCWD, err := resolveSessionPath(cwd)
	if err != nil {
		return nil, err
	}
	resolvedSessionDir := ""
	if sessionDir != "" {
		resolvedSessionDir, err = resolveSessionPath(sessionDir)
		if err != nil {
			return nil, err
		}
	}
	sm := &SessionManager{
		cwd:                 resolvedCWD,
		sessionDir:          resolvedSessionDir,
		persist:             persist,
		byID:                map[string]FileEntry{},
		labelsByID:          map[string]string{},
		labelTimestampsByID: map[string]string{},
	}
	if persist && resolvedSessionDir != "" {
		if err := os.MkdirAll(resolvedSessionDir, 0o755); err != nil {
			return nil, err
		}
	}
	if sessionFile != "" {
		sm.mu.Lock()
		err = sm.setSessionFileLocked(sessionFile, preloadedEntries)
		sm.mu.Unlock()
		if err != nil {
			return nil, err
		}
	} else {
		options := NewSessionOptions{}
		if initialOptions != nil {
			options = *initialOptions
		}
		if _, err := sm.newSession(options); err != nil {
			return nil, err
		}
	}
	return sm, nil
}

func (s *SessionManager) SetSessionFile(sessionFile string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setSessionFileLocked(sessionFile, nil)
}

func (s *SessionManager) setSessionFileLocked(
	sessionFile string,
	preloadedEntries *[]FileEntry,
) error {
	absPath, err := resolveSessionPath(sessionFile)
	if err != nil {
		return err
	}
	info, statErr := os.Stat(absPath)
	if statErr == nil {
		var entries []FileEntry
		if preloadedEntries != nil {
			entries = cloneFileEntries(*preloadedEntries)
		} else {
			entries, err = loadEntriesFromFile(absPath)
			if err != nil {
				return err
			}
		}
		if len(entries) == 0 {
			if info.Size() > 0 {
				return fmt.Errorf("Session file is not a valid pi session: %s", absPath)
			}
			sessionID, _, newEntries, newErr := s.newSessionStateLocked(NewSessionOptions{})
			if newErr != nil {
				return newErr
			}
			if s.persist {
				if err := writeSessionEntries(absPath, newEntries, os.O_CREATE|os.O_WRONLY|os.O_TRUNC); err != nil {
					return err
				}
			}
			s.applySessionStateLocked(sessionID, absPath, newEntries, true)
			return nil
		}
		header := entries[0]
		if MigrateSessionEntries(entries) && s.persist {
			if err := writeSessionEntries(absPath, entries, os.O_CREATE|os.O_WRONLY|os.O_TRUNC); err != nil {
				return err
			}
		}
		s.applySessionStateLocked(header.ID, absPath, entries, true)
		return nil
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	sessionID, _, entries, err := s.newSessionStateLocked(NewSessionOptions{})
	if err != nil {
		return err
	}
	s.applySessionStateLocked(sessionID, absPath, entries, false)
	return nil
}

// NewSession replaces the active in-memory state after validating all supplied
// identity metadata. Persisted content is created lazily on the first response.
func (s *SessionManager) NewSession(options ...NewSessionOptions) (string, error) {
	opts := NewSessionOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	return s.newSession(opts)
}

func (s *SessionManager) newSession(options any) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.newSessionLocked(options)
}

func (s *SessionManager) newSessionLocked(options any) (string, error) {
	opts := NewSessionOptions{}
	switch value := options.(type) {
	case string:
		opts.ParentSession = value
	case NewSessionOptions:
		opts = value
	}
	sessionID, sessionFile, entries, err := s.newSessionStateLocked(opts)
	if err != nil {
		return "", err
	}
	s.applySessionStateLocked(sessionID, sessionFile, entries, false)
	return sessionFile, nil
}

func (s *SessionManager) newSessionStateLocked(
	options NewSessionOptions,
) (sessionID, sessionFile string, entries []FileEntry, err error) {
	if options.ID != "" {
		if err := ValidateSessionID(options.ID); err != nil {
			return "", "", nil, err
		}
	}
	sessionID = options.ID
	if sessionID == "" {
		sessionID = agentharness.UUIDv7()
	}
	timestamp := sessionTimestamp(time.Now())
	entries = []FileEntry{{
		Type:          "session",
		Version:       CurrentSessionVersion,
		ID:            sessionID,
		Timestamp:     timestamp,
		CWD:           s.cwd,
		ParentSession: options.ParentSession,
	}}
	if s.persist {
		fileTimestamp := strings.NewReplacer(":", "-", ".", "-").Replace(timestamp)
		sessionFile = filepath.Join(s.sessionDir, fileTimestamp+"_"+sessionID+".jsonl")
	}
	return sessionID, sessionFile, entries, nil
}

func (s *SessionManager) applySessionStateLocked(
	sessionID, sessionFile string,
	entries []FileEntry,
	flushed bool,
) {
	s.sessionID = sessionID
	s.sessionFile = sessionFile
	s.fileEntries = entries
	s.flushed = flushed
	s.buildIndexLocked()
}

func (s *SessionManager) rewriteFile() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rewriteFileLocked()
}

func (s *SessionManager) rewriteFileLocked() error {
	if !s.persist || s.sessionFile == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.sessionFile), 0o755); err != nil {
		return err
	}
	return writeSessionEntries(
		s.sessionFile,
		s.fileEntries,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
	)
}

func (s *SessionManager) appendEntry(entry FileEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry.ParentID = cloneStringPtr(s.leafID)
	if entry.raw != nil {
		if entry.ParentID == nil {
			entry.raw["parentId"] = nil
		} else {
			entry.raw["parentId"] = *entry.ParentID
		}
	}
	s.appendEntryLocked(entry)
}

func (s *SessionManager) appendEntryLocked(entry FileEntry) {
	entry = cloneFileEntry(entry)
	s.fileEntries = append(s.fileEntries, entry)
	s.indexEntry(entry)
	s.leafID = stringPtr(entry.ID)
	s.persistEntryLocked(entry)
}

func (s *SessionManager) persistEntryLocked(entry FileEntry) {
	if !s.persist || s.sessionFile == "" {
		return
	}
	if !s.hasAssistantMessageLocked() {
		s.flushed = false
		return
	}
	if !s.flushed {
		_ = s.rewriteFileLocked()
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasAssistantMessageLocked()
}

func (s *SessionManager) hasAssistantMessageLocked() bool {
	for _, entry := range s.fileEntries {
		if entry.Type == "message" && messageRole(entry.Message) == "assistant" {
			return true
		}
	}
	return false
}

func (s *SessionManager) buildIndex() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buildIndexLocked()
}

func (s *SessionManager) buildIndexLocked() {
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.persist
}

func (s *SessionManager) GetSessionID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionID
}

func (s *SessionManager) GetSessionId() string {
	return s.GetSessionID()
}

func (s *SessionManager) GetSessionFile() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionFile
}

func (s *SessionManager) GetSessionDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionDir
}

// UsesDefaultSessionDir reports whether this manager owns the canonical
// per-working-directory store. It is pure and does not create directories.
func (s *SessionManager) UsesDefaultSessionDir() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	sessionDir := s.sessionDir
	cwd := s.cwd
	s.mu.RUnlock()
	return sessionDirMatchesDefault(sessionDir, cwd)
}

func (s *SessionManager) GetCWD() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cwd
}

func (s *SessionManager) GetCwd() string {
	return s.GetCWD()
}

func (s *SessionManager) GetHeader() *SessionHeader {
	s.mu.RLock()
	defer s.mu.RUnlock()
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := make([]FileEntry, 0, len(s.fileEntries))
	for _, entry := range s.fileEntries {
		if entry.Type == "session" {
			continue
		}
		entries = append(entries, cloneFileEntry(entry))
	}
	return entries
}

func (s *SessionManager) allEntriesSnapshot() []FileEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneFileEntries(s.fileEntries)
}

func (s *SessionManager) forkConfiguration() (cwd, sessionDir, sessionFile string, persist bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cwd, s.sessionDir, s.sessionFile, s.persist
}

func (s *SessionManager) replaceEntries(entries []FileEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fileEntries = cloneFileEntries(entries)
	s.buildIndexLocked()
	s.flushed = false
}

func (s *SessionManager) AppendMessage(message any) string {
	entry := newSessionEntry("message", nil, map[string]any{"message": message})
	entry.Message = message
	s.appendEntry(entry)
	return entry.ID
}

func (s *SessionManager) AppendThinkingLevelChange(thinkingLevel string) string {
	entry := newSessionEntry("thinking_level_change", nil, map[string]any{"thinkingLevel": thinkingLevel})
	entry.ThinkingLevel = thinkingLevel
	s.appendEntry(entry)
	return entry.ID
}

func (s *SessionManager) AppendModelChange(provider, modelID string) string {
	entry := newSessionEntry("model_change", nil, map[string]any{"provider": provider, "modelId": modelID})
	entry.Provider = provider
	entry.ModelID = modelID
	s.appendEntry(entry)
	return entry.ID
}

func (s *SessionManager) AppendCompaction(summary, firstKeptEntryID string, tokensBefore int) string {
	return s.AppendCompactionWithOptions(summary, firstKeptEntryID, tokensBefore, SessionSummaryOptions{})
}

func (s *SessionManager) AppendCompactionWithOptions(
	summary,
	firstKeptEntryID string,
	tokensBefore int,
	options SessionSummaryOptions,
) string {
	values := map[string]any{
		"summary":          summary,
		"firstKeptEntryId": firstKeptEntryID,
		"tokensBefore":     tokensBefore,
	}
	appendSessionSummaryOptions(values, options)
	entry := newSessionEntry("compaction", nil, values)
	entry.Summary = summary
	entry.FirstKeptID = firstKeptEntryID
	entry.TokensBefore = tokensBefore
	entry.Details = options.Details
	entry.FromHook = options.FromHook
	entry.Usage = cloneSessionUsage(options.Usage)
	s.appendEntry(entry)
	return entry.ID
}

func (s *SessionManager) AppendCustomEntry(customType string, data any) string {
	entry := newSessionEntry("custom", nil, map[string]any{"customType": customType, "data": data})
	entry.CustomType = customType
	entry.Data = data
	s.appendEntry(entry)
	return entry.ID
}

func (s *SessionManager) AppendCustomMessageEntry(customType string, content any, display bool, details any) string {
	return s.AppendCustomMessageEntryWithContext(customType, content, display, details, false)
}

func (s *SessionManager) AppendCustomMessageEntryWithContext(customType string, content any, display bool, details any, includeInContext bool) string {
	message := map[string]any{
		"customType": customType,
		"content":    content,
		"display":    display,
		"details":    details,
	}
	if includeInContext {
		message["includeInContext"] = true
	}
	entry := newSessionEntry("custom_message", nil, message)
	entry.CustomType = customType
	entry.Content = content
	entry.Display = display
	entry.Details = details
	s.appendEntry(entry)
	return entry.ID
}

func (s *SessionManager) AppendSessionInfo(name string) string {
	name = strings.TrimSpace(name)
	entry := newSessionEntry("session_info", nil, map[string]any{"name": name})
	entry.Name = name
	s.appendEntry(entry)
	return entry.ID
}

func (s *SessionManager) AppendLabelChange(targetID, label string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[targetID]; !ok {
		return "", errors.New("Entry " + targetID + " not found")
	}
	parentID := cloneStringPtr(s.leafID)
	entry := newSessionEntry("label", parentID, map[string]any{"targetId": targetID, "label": label})
	entry.TargetID = targetID
	entry.Label = label
	s.appendEntryLocked(entry)
	return entry.ID, nil
}

func (s *SessionManager) GetLeafID() *string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneStringPtr(s.leafID)
}

func (s *SessionManager) GetLeafId() *string {
	return s.GetLeafID()
}

func (s *SessionManager) GetLeafEntry() *FileEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.leafID == nil {
		return nil
	}
	entry, ok := s.byID[*s.leafID]
	if !ok {
		return nil
	}
	cloned := cloneFileEntry(entry)
	return &cloned
}

func (s *SessionManager) GetEntry(id string) *FileEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.byID[id]
	if !ok {
		return nil
	}
	cloned := cloneFileEntry(entry)
	return &cloned
}

func (s *SessionManager) GetChildren(parentID string) []FileEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var children []FileEntry
	for _, entry := range s.fileEntries {
		if entry.Type == "session" {
			continue
		}
		if entry.ParentID != nil && *entry.ParentID == parentID {
			children = append(children, cloneFileEntry(entry))
		}
	}
	return children
}

func (s *SessionManager) GetLabel(id string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	label, ok := s.labelsByID[id]
	return label, ok
}

func (s *SessionManager) GetSessionName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.fileEntries) - 1; i >= 0; i-- {
		entry := s.fileEntries[i]
		if entry.Type == "session_info" {
			return strings.TrimSpace(entry.Name)
		}
	}
	return ""
}

func (s *SessionManager) GetBranch(fromID ...string) []FileEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getBranchLocked(fromID...)
}

func (s *SessionManager) getBranchLocked(fromID ...string) []FileEntry {
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
		path = append([]FileEntry{cloneFileEntry(current)}, path...)
		if current.ParentID == nil {
			break
		}
		current, ok = s.byID[*current.ParentID]
	}
	return path
}

func (s *SessionManager) BuildSessionContext() SessionContext {
	return s.BuildSessionContextWithOptions(SessionContextOptions{})
}

// BuildContextEntries returns the active, compaction-aware entry projection.
// The snapshot is captured under one read lock so branch navigation cannot
// produce a path assembled from different session states.
func (s *SessionManager) BuildContextEntries() []FileEntry {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	entries := entriesWithoutHeader(cloneFileEntries(s.fileEntries))
	leafID := cloneStringPtr(s.leafID)
	byID := cloneFileEntryMap(s.byID)
	s.mu.RUnlock()
	if leafID == nil {
		return nil
	}
	return BuildContextEntries(entries, leafID, byID)
}

func (s *SessionManager) BuildSessionContextWithOptions(
	options SessionContextOptions,
) SessionContext {
	s.mu.RLock()
	entries := cloneFileEntries(s.fileEntries)
	leafID := cloneStringPtr(s.leafID)
	byID := cloneFileEntryMap(s.byID)
	s.mu.RUnlock()
	if leafID == nil {
		return SessionContext{ThinkingLevel: "off"}
	}
	return BuildSessionContextWithOptions(
		entriesWithoutHeader(entries),
		leafID,
		byID,
		options,
	)
}

func BuildSessionContext(entries []FileEntry, leafID *string, byID map[string]FileEntry) SessionContext {
	return BuildSessionContextWithOptions(
		entries,
		leafID,
		byID,
		SessionContextOptions{},
	)
}

// BuildContextEntries follows the selected leaf to the root and applies the
// latest compaction. It retains FileEntry identity so presentation layers can
// attach derived state, such as cache-miss notices, without persisting it.
func BuildContextEntries(
	entries []FileEntry,
	leafID *string,
	byID map[string]FileEntry,
) []FileEntry {
	path := buildSessionPath(entriesWithoutHeader(entries), leafID, byID)
	return buildContextEntriesFromPath(path, nil)
}

func buildSessionPath(
	entries []FileEntry,
	leafID *string,
	byID map[string]FileEntry,
) []FileEntry {
	if len(entries) == 0 {
		return nil
	}
	byID = buildEntryIndex(entries, byID)

	var leaf FileEntry
	var ok bool
	if leafID != nil {
		leaf, ok = byID[*leafID]
	}
	if !ok {
		leaf = entries[len(entries)-1]
	}

	path := make([]FileEntry, 0, len(entries))
	current := leaf
	for {
		path = append(path, cloneFileEntry(current))
		if current.ParentID == nil {
			break
		}
		next, found := byID[*current.ParentID]
		if !found {
			break
		}
		current = next
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path
}

func buildEntryIndex(
	entries []FileEntry,
	existing map[string]FileEntry,
) map[string]FileEntry {
	if existing != nil {
		return existing
	}
	index := make(map[string]FileEntry, len(entries))
	for _, entry := range entries {
		if entry.ID != "" {
			index[entry.ID] = entry
		}
	}
	return index
}

func buildContextEntriesFromPath(
	path []FileEntry,
	excludedEntryIDs map[string]struct{},
) []FileEntry {
	compactionIndex := -1
	for index, entry := range path {
		if _, excluded := excludedEntryIDs[entry.ID]; excluded {
			continue
		}
		if entry.Type == "compaction" {
			compactionIndex = index
		}
	}
	if compactionIndex < 0 {
		return cloneFileEntries(path)
	}

	contextEntries := make([]FileEntry, 0, len(path)-compactionIndex+1)
	contextEntries = append(contextEntries, cloneFileEntry(path[compactionIndex]))
	foundFirstKept := false
	firstKeptID := path[compactionIndex].FirstKeptID
	for index := 0; index < compactionIndex; index++ {
		entry := path[index]
		if entry.ID == firstKeptID {
			foundFirstKept = true
		}
		if foundFirstKept {
			contextEntries = append(contextEntries, cloneFileEntry(entry))
		}
	}
	contextEntries = append(contextEntries, cloneFileEntries(path[compactionIndex+1:])...)
	return contextEntries
}

// SessionEntryToContextMessages converts one selected session entry into the
// canonical message representation consumed by the agent and TUI.
func SessionEntryToContextMessages(entry FileEntry) []llm.Message {
	var value any
	switch entry.Type {
	case "message":
		value = entry.Message
	case "custom_message":
		value = map[string]any{
			"role":       "custom",
			"customType": entry.CustomType,
			"content":    entry.Content,
			"display":    entry.Display,
			"timestamp":  sessionEntryTimestampMillis(entry.Timestamp),
			"details":    entry.Details,
		}
	case "branch_summary":
		if entry.Summary == "" {
			return nil
		}
		value = map[string]any{
			"role":      "branchSummary",
			"summary":   entry.Summary,
			"fromId":    entry.FromID,
			"timestamp": sessionEntryTimestampMillis(entry.Timestamp),
		}
	case "compaction":
		value = map[string]any{
			"role":         "compactionSummary",
			"summary":      entry.Summary,
			"tokensBefore": entry.TokensBefore,
			"timestamp":    sessionEntryTimestampMillis(entry.Timestamp),
		}
	default:
		return nil
	}
	message, ok := sessionMessageToLLM(value)
	if !ok {
		return nil
	}
	return []llm.Message{message}
}

func BuildSessionContextWithOptions(
	entries []FileEntry,
	leafID *string,
	byID map[string]FileEntry,
	options SessionContextOptions,
) SessionContext {
	context := SessionContext{ThinkingLevel: "off"}
	path := buildSessionPath(entriesWithoutHeader(entries), leafID, byID)
	if len(path) == 0 {
		return context
	}

	context.ThinkingLevel, context.Model = getSessionContextSettings(
		path,
		options.ExcludeEntryIDs,
	)

	appendEntryMessage := func(entry FileEntry) {
		if _, excluded := options.ExcludeEntryIDs[entry.ID]; excluded {
			return
		}
		switch entry.Type {
		case "message":
			context.Messages = append(context.Messages, entry.Message)
		case "custom_message":
			context.Messages = append(context.Messages, map[string]any{
				"role":       "custom",
				"customType": entry.CustomType,
				"content":    entry.Content,
				"display":    entry.Display,
				"timestamp":  sessionEntryTimestampMillis(entry.Timestamp),
				"details":    entry.Details,
			})
		case "branch_summary":
			if entry.Summary != "" {
				context.Messages = append(context.Messages, map[string]any{
					"role":      "branchSummary",
					"summary":   entry.Summary,
					"fromId":    entry.FromID,
					"timestamp": sessionEntryTimestampMillis(entry.Timestamp),
				})
			}
		case "compaction":
			context.Messages = append(context.Messages, map[string]any{
				"role":         "compactionSummary",
				"summary":      entry.Summary,
				"tokensBefore": entry.TokensBefore,
				"timestamp":    sessionEntryTimestampMillis(entry.Timestamp),
			})
		}
	}

	for _, entry := range buildContextEntriesFromPath(path, options.ExcludeEntryIDs) {
		appendEntryMessage(entry)
	}
	return context
}

func getSessionContextSettings(
	path []FileEntry,
	excludedEntryIDs map[string]struct{},
) (thinkingLevel string, model *SessionModel) {
	thinkingLevel = "off"
	for _, entry := range path {
		if _, excluded := excludedEntryIDs[entry.ID]; excluded {
			continue
		}
		switch entry.Type {
		case "thinking_level_change":
			thinkingLevel = entry.ThinkingLevel
		case "model_change":
			model = &SessionModel{Provider: entry.Provider, ModelID: entry.ModelID}
		case "message":
			message, converted := sessionMessageToLLM(entry.Message)
			if converted && message.Role == llm.RoleAssistant &&
				(message.Provider != "" || message.Model != "") {
				model = &SessionModel{Provider: message.Provider, ModelID: message.Model}
			}
		}
	}
	return thinkingLevel, model
}

func customMessageIncludedInContext(entry FileEntry) bool {
	message, ok := entry.Message.(map[string]any)
	if !ok {
		message = entry.raw
	}
	include, _ := message["includeInContext"].(bool)
	return include
}

func customMessageText(content any) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	case nil:
		return ""
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(jsonStringFallback(typed), "\n", " "), "\t", " "))
		}
		return string(data)
	}
}

func jsonStringFallback(value any) string {
	return strings.TrimSpace(fmt.Sprint(value))
}

func (s *SessionManager) GetTree() []*SessionTreeNode {
	s.mu.RLock()
	entries := entriesWithoutHeader(cloneFileEntries(s.fileEntries))
	labelsByID := cloneSessionStringMap(s.labelsByID)
	labelTimestampsByID := cloneSessionStringMap(s.labelTimestampsByID)
	s.mu.RUnlock()
	nodeMap := map[string]*SessionTreeNode{}
	roots := []*SessionTreeNode{}
	for _, entry := range entries {
		nodeMap[entry.ID] = &SessionTreeNode{
			Entry:          entry,
			Label:          labelsByID[entry.ID],
			LabelTimestamp: labelTimestampsByID[entry.ID],
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[branchFromID]; !ok {
		return errors.New("Entry " + branchFromID + " not found")
	}
	s.leafID = stringPtr(branchFromID)
	return nil
}

func (s *SessionManager) ResetLeaf() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leafID = nil
}

func (s *SessionManager) BranchWithSummary(branchFromID *string, summary string) (string, error) {
	return s.BranchWithSummaryOptions(branchFromID, summary, SessionSummaryOptions{})
}

func (s *SessionManager) BranchWithSummaryOptions(
	branchFromID *string,
	summary string,
	options SessionSummaryOptions,
) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	values := map[string]any{
		"fromId":  fromID,
		"summary": summary,
	}
	appendSessionSummaryOptions(values, options)
	entry := newSessionEntry("branch_summary", parentID, values)
	entry.Summary = summary
	entry.FromID = fromID
	entry.Details = options.Details
	entry.FromHook = options.FromHook
	entry.Usage = cloneSessionUsage(options.Usage)
	s.appendEntryLocked(entry)
	return entry.ID, nil
}

func appendSessionSummaryOptions(values map[string]any, options SessionSummaryOptions) {
	if options.Details != nil {
		values["details"] = options.Details
	}
	if options.FromHook {
		values["fromHook"] = true
	}
	if options.Usage != nil {
		values["usage"] = options.Usage
	}
}

func cloneSessionUsage(usage *llm.Usage) *llm.Usage {
	if usage == nil {
		return nil
	}
	cloned := *usage
	return &cloned
}

func cloneFileEntries(entries []FileEntry) []FileEntry {
	if entries == nil {
		return nil
	}
	cloned := make([]FileEntry, len(entries))
	for index, entry := range entries {
		cloned[index] = cloneFileEntry(entry)
	}
	return cloned
}

func cloneFileEntryMap(entries map[string]FileEntry) map[string]FileEntry {
	if entries == nil {
		return nil
	}
	cloned := make(map[string]FileEntry, len(entries))
	for id, entry := range entries {
		cloned[id] = cloneFileEntry(entry)
	}
	return cloned
}

func entriesWithoutHeader(entries []FileEntry) []FileEntry {
	result := make([]FileEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Type != "session" {
			result = append(result, entry)
		}
	}
	return result
}

func cloneSessionStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneFileEntry(entry FileEntry) FileEntry {
	cloned := entry
	cloned.ParentID = cloneStringPtr(entry.ParentID)
	cloned.Message = cloneSessionValue(entry.Message)
	cloned.Data = cloneSessionValue(entry.Data)
	cloned.Content = cloneSessionValue(entry.Content)
	cloned.Details = cloneSessionValue(entry.Details)
	cloned.Usage = cloneSessionUsage(entry.Usage)
	cloned.raw = cloneSessionMap(entry.raw)
	return cloned
}

func cloneSessionMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = cloneSessionValue(value)
	}
	return cloned
}

func cloneSessionValue(value any) any {
	switch typed := value.(type) {
	case llm.Message:
		return cloneSessionMessage(typed)
	case *llm.Message:
		if typed == nil {
			return (*llm.Message)(nil)
		}
		cloned := cloneSessionMessage(*typed)
		return &cloned
	case map[string]any:
		return cloneSessionMap(typed)
	case map[string]string:
		cloned := make(map[string]string, len(typed))
		for key, item := range typed {
			cloned[key] = item
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneSessionValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	case []llm.ContentPart:
		return cloneSessionContent(typed)
	case []byte:
		return append([]byte(nil), typed...)
	default:
		return typed
	}
}

func cloneSessionMessage(message llm.Message) llm.Message {
	cloned := message
	cloned.Content = cloneSessionContent(message.Content)
	if message.Diagnostics != nil {
		cloned.Diagnostics = append([]llm.AssistantMessageDiagnostic(nil), message.Diagnostics...)
		for index := range cloned.Diagnostics {
			cloned.Diagnostics[index].Details = cloneSessionMap(message.Diagnostics[index].Details)
			if message.Diagnostics[index].Error != nil {
				errorInfo := *message.Diagnostics[index].Error
				cloned.Diagnostics[index].Error = &errorInfo
			}
		}
	}
	cloned.Details = cloneSessionValue(message.Details)
	return cloned
}

func cloneSessionContent(content []llm.ContentPart) []llm.ContentPart {
	if content == nil {
		return nil
	}
	cloned := append([]llm.ContentPart(nil), content...)
	for index := range cloned {
		cloned[index].Arguments = cloneSessionMap(content[index].Arguments)
	}
	return cloned
}

func (s *SessionManager) CreateBranchedSession(leafID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previousSessionFile := s.sessionFile
	path := s.getBranchLocked(leafID)
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
		newSessionFile = filepath.Join(s.sessionDir, fileTimestamp+"_"+newSessionID+".jsonl")
	}
	labels := s.labelEntriesForPath(pathWithoutLabels)
	s.fileEntries = append([]FileEntry{header}, pathWithoutLabels...)
	s.fileEntries = append(s.fileEntries, labels...)
	s.sessionID = newSessionID
	if s.persist {
		s.sessionFile = newSessionFile
	}
	s.buildIndexLocked()
	if s.persist {
		if s.hasAssistantMessageLocked() {
			if err := s.rewriteFileLocked(); err != nil {
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
	if summary, _ := value["summary"].(string); summary != "" {
		return summary
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
	case string:
		if parsed, ok := parseSessionTimeOK(timestamp); ok {
			return parsed.UnixMilli(), true
		}
		return 0, false
	default:
		return 0, false
	}
}

func sessionEntryTimestampMillis(timestamp string) int64 {
	if parsed, ok := parseSessionTimeOK(timestamp); ok {
		return parsed.UnixMilli()
	}
	return 0
}

func getMessageActivityTime(entry FileEntry) (int64, bool) {
	if entry.Type != "message" {
		return 0, false
	}
	message, ok := entry.Message.(map[string]any)
	if !ok {
		return 0, false
	}
	role, _ := message["role"].(string)
	if role != "user" && role != "assistant" {
		return 0, false
	}
	if _, hasContent := message["content"]; !hasContent {
		return 0, false
	}
	switch timestamp := message["timestamp"].(type) {
	case float64:
		return int64(timestamp), true
	case int:
		return int64(timestamp), true
	case int64:
		return timestamp, true
	}
	if parsed, ok := parseSessionTimeOK(entry.Timestamp); ok {
		return parsed.UnixMilli(), true
	}
	return 0, false
}

func getSessionModifiedDate(entries []FileEntry, header FileEntry, statsMtime time.Time) time.Time {
	var lastActivity int64
	for _, entry := range entries {
		if timestamp, ok := getMessageActivityTime(entry); ok && timestamp > lastActivity {
			lastActivity = timestamp
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
