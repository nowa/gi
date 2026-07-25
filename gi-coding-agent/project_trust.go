package gicodingagent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type DefaultProjectTrust string

const (
	DefaultProjectTrustAsk    DefaultProjectTrust = "ask"
	DefaultProjectTrustAlways DefaultProjectTrust = "always"
	DefaultProjectTrustNever  DefaultProjectTrust = "never"
)

type ProjectTrustStoreEntry struct {
	Path     string
	Decision bool
}

type ProjectTrustUpdate struct {
	Path     string
	Decision *bool
}

type ProjectTrustOption struct {
	Label     string
	Trusted   bool
	Updates   []ProjectTrustUpdate
	SavedPath string
}

type ProjectTrustPrompt func(cwd string, options []ProjectTrustOption) (*ProjectTrustOption, error)

type ResolveProjectTrustOptions struct {
	CWD                 string
	TrustStore          *ProjectTrustStore
	TrustOverride       *bool
	DefaultProjectTrust DefaultProjectTrust
	Prompt              ProjectTrustPrompt
	ExtensionRuntime    *ProtocolExtensionRuntime
	ExtensionContext    ProtocolProjectTrustContext
	OnExtensionError    func(ProtocolExtensionError)
}

type ProjectTrustStore struct {
	trustPath string
}

var trustRequiringProjectConfigResources = [...]string{
	"settings.json",
	"extensions",
	"skills",
	"prompts",
	"themes",
	"SYSTEM.md",
	"APPEND_SYSTEM.md",
}

var projectTrustProcessLocks sync.Map

const (
	projectTrustLockAttempts   = 10
	projectTrustLockDelay      = 20 * time.Millisecond
	projectTrustLockStaleAfter = 10 * time.Second
)

func NewProjectTrustStore(agentDir string) *ProjectTrustStore {
	return &ProjectTrustStore{trustPath: filepath.Join(resolveAbsolutePath(agentDir), "trust.json")}
}

func (s *ProjectTrustStore) Path() string {
	if s == nil {
		return ""
	}
	return s.trustPath
}

func (s *ProjectTrustStore) Get(cwd string) (decision bool, found bool, err error) {
	entry, err := s.GetEntry(cwd)
	if err != nil || entry == nil {
		return false, false, err
	}
	return entry.Decision, true, nil
}

func (s *ProjectTrustStore) GetEntry(cwd string) (*ProjectTrustStoreEntry, error) {
	if s == nil || strings.TrimSpace(s.trustPath) == "" {
		return nil, errors.New("project trust store path is required")
	}
	var entry *ProjectTrustStoreEntry
	err := withProjectTrustFileLock(s.trustPath, func() error {
		data, err := readProjectTrustFile(s.trustPath)
		if err != nil {
			return err
		}
		entry = findNearestProjectTrustEntry(data, cwd)
		return nil
	})
	return entry, err
}

func (s *ProjectTrustStore) Set(cwd string, decision bool) error {
	value := decision
	return s.SetMany([]ProjectTrustUpdate{{Path: cwd, Decision: &value}})
}

func (s *ProjectTrustStore) Clear(cwd string) error {
	return s.SetMany([]ProjectTrustUpdate{{Path: cwd}})
}

func (s *ProjectTrustStore) SetMany(updates []ProjectTrustUpdate) error {
	if s == nil || strings.TrimSpace(s.trustPath) == "" {
		return errors.New("project trust store path is required")
	}
	return withProjectTrustFileLock(s.trustPath, func() error {
		data, err := readProjectTrustFile(s.trustPath)
		if err != nil {
			return err
		}
		for _, update := range updates {
			key := normalizeProjectTrustPath(update.Path)
			if update.Decision == nil {
				delete(data, key)
				continue
			}
			data[key] = *update.Decision
		}
		return writeProjectTrustFile(s.trustPath, data)
	})
}

func GetProjectTrustParentPath(cwd string) string {
	trustPath := normalizeProjectTrustPath(cwd)
	parent := filepath.Dir(trustPath)
	if parent == trustPath {
		return ""
	}
	return parent
}

func GetProjectTrustOptions(cwd string, includeSessionOnly bool) []ProjectTrustOption {
	trustPath := normalizeProjectTrustPath(cwd)
	trusted := true
	untrusted := false
	options := []ProjectTrustOption{{
		Label:     "Trust",
		Trusted:   true,
		Updates:   []ProjectTrustUpdate{{Path: trustPath, Decision: &trusted}},
		SavedPath: trustPath,
	}}
	if parent := GetProjectTrustParentPath(cwd); parent != "" {
		options = append(options, ProjectTrustOption{
			Label:   fmt.Sprintf("Trust parent folder (%s)", parent),
			Trusted: true,
			Updates: []ProjectTrustUpdate{
				{Path: parent, Decision: &trusted},
				{Path: trustPath},
			},
			SavedPath: parent,
		})
	}
	if includeSessionOnly {
		options = append(options, ProjectTrustOption{Label: "Trust (this session only)", Trusted: true})
	}
	options = append(options, ProjectTrustOption{
		Label:     "Do not trust",
		Trusted:   false,
		Updates:   []ProjectTrustUpdate{{Path: trustPath, Decision: &untrusted}},
		SavedPath: trustPath,
	})
	if includeSessionOnly {
		options = append(options, ProjectTrustOption{Label: "Do not trust (this session only)", Trusted: false})
	}
	return options
}

func ResolveProjectTrusted(options ResolveProjectTrustOptions) (bool, error) {
	if options.TrustOverride != nil {
		return *options.TrustOverride, nil
	}
	if !HasTrustRequiringProjectResources(options.CWD) {
		return true, nil
	}
	if options.ExtensionRuntime != nil {
		context := options.ExtensionContext
		if strings.TrimSpace(context.CWD) == "" {
			context.CWD = options.CWD
		}
		result, extensionErrors := options.ExtensionRuntime.EmitProjectTrustEvent(
			context,
		)
		for _, extensionError := range extensionErrors {
			if options.OnExtensionError != nil {
				options.OnExtensionError(extensionError)
			}
		}
		if result != nil {
			trusted := result.Trusted == ProtocolProjectTrustYes
			if result.Remember && options.TrustStore != nil {
				if err := options.TrustStore.Set(options.CWD, trusted); err != nil {
					return false, err
				}
			}
			return trusted, nil
		}
	}
	if options.TrustStore != nil {
		decision, found, err := options.TrustStore.Get(options.CWD)
		if err != nil {
			return false, err
		}
		if found {
			return decision, nil
		}
	}
	switch normalizeDefaultProjectTrust(options.DefaultProjectTrust) {
	case DefaultProjectTrustAlways:
		return true, nil
	case DefaultProjectTrustNever:
		return false, nil
	}
	if options.Prompt == nil {
		return false, nil
	}
	selected, err := options.Prompt(options.CWD, GetProjectTrustOptions(options.CWD, true))
	if err != nil {
		return false, err
	}
	if selected == nil {
		return false, nil
	}
	if len(selected.Updates) > 0 && options.TrustStore != nil {
		if err := options.TrustStore.SetMany(selected.Updates); err != nil {
			return false, err
		}
	}
	return selected.Trusted, nil
}

func HasTrustRequiringProjectResources(cwd string) bool {
	current := normalizeProjectTrustPath(cwd)
	configDir := filepath.Join(current, ConfigDirName)
	for _, entry := range trustRequiringProjectConfigResources {
		if _, err := os.Stat(filepath.Join(configDir, entry)); err == nil {
			return true
		}
	}

	home, _ := os.UserHomeDir()
	userAgentsSkillsDir := normalizeProjectTrustPath(filepath.Join(home, ".agents", "skills"))
	for {
		agentsSkillsDir := normalizeProjectTrustPath(filepath.Join(current, ".agents", "skills"))
		if agentsSkillsDir != userAgentsSkillsDir {
			if _, err := os.Stat(agentsSkillsDir); err == nil {
				return true
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

func normalizeDefaultProjectTrust(value DefaultProjectTrust) DefaultProjectTrust {
	switch value {
	case DefaultProjectTrustAlways, DefaultProjectTrustNever:
		return value
	default:
		return DefaultProjectTrustAsk
	}
}

func normalizeProjectTrustPath(path string) string {
	resolved := resolveAbsolutePath(path)
	if canonical, err := filepath.EvalSymlinks(resolved); err == nil {
		return filepath.Clean(canonical)
	}
	return filepath.Clean(resolved)
}

func resolveAbsolutePath(path string) string {
	path = ExpandPath(strings.TrimSpace(path))
	if path == "" {
		path = "."
	}
	if absolute, err := filepath.Abs(path); err == nil {
		return filepath.Clean(absolute)
	}
	return filepath.Clean(path)
}

func findNearestProjectTrustEntry(data map[string]bool, cwd string) *ProjectTrustStoreEntry {
	current := normalizeProjectTrustPath(cwd)
	for {
		if value, ok := data[current]; ok {
			return &ProjectTrustStoreEntry{Path: current, Decision: value}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}

func readProjectTrustFile(path string) (map[string]bool, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read trust store %s: %w", path, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("failed to read trust store %s: %w", path, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("invalid trust store %s: expected an object", path)
	}
	data := make(map[string]bool, len(raw))
	for key, encoded := range raw {
		if string(encoded) == "null" {
			continue
		}
		var value bool
		if err := json.Unmarshal(encoded, &value); err != nil {
			return nil, fmt.Errorf("invalid trust store %s: value for %q must be true, false, or null", path, key)
		}
		data[key] = value
	}
	return data, nil
}

func writeProjectTrustFile(path string, data map[string]bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var content bytes.Buffer
	if len(keys) == 0 {
		content.WriteString("{}\n")
	} else {
		content.WriteString("{\n")
		for index, key := range keys {
			encodedKey, err := json.Marshal(key)
			if err != nil {
				return err
			}
			content.WriteString("  ")
			content.Write(encodedKey)
			content.WriteString(": ")
			if data[key] {
				content.WriteString("true")
			} else {
				content.WriteString("false")
			}
			if index < len(keys)-1 {
				content.WriteByte(',')
			}
			content.WriteByte('\n')
		}
		content.WriteString("}\n")
	}

	if err := os.WriteFile(path, content.Bytes(), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func withProjectTrustFileLock(path string, fn func() error) error {
	processLock, _ := projectTrustProcessLocks.LoadOrStore(path, &sync.Mutex{})
	mutex := processLock.(*sync.Mutex)
	mutex.Lock()
	defer mutex.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	lockPath := path + ".lock"
	var lastErr error
	for attempt := 0; attempt < projectTrustLockAttempts; attempt++ {
		err := os.Mkdir(lockPath, 0o700)
		if err == nil {
			defer os.Remove(lockPath)
			return fn()
		}
		if !os.IsExist(err) {
			return err
		}
		if info, statErr := os.Stat(lockPath); statErr == nil &&
			time.Since(info.ModTime()) > projectTrustLockStaleAfter {
			if removeErr := os.Remove(lockPath); removeErr == nil || os.IsNotExist(removeErr) {
				continue
			}
		}
		lastErr = err
		time.Sleep(projectTrustLockDelay)
	}
	return fmt.Errorf("failed to acquire trust store lock %s: %w", lockPath, lastErr)
}
