package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type SettingsError struct {
	Scope string
	Err   error
}

type SettingsManager struct {
	cwd         string
	agentDir    string
	globalPath  string
	projectPath string

	global  map[string]any
	project map[string]any
	merged  map[string]any

	globalLoadErr  error
	projectLoadErr error
	errors         []SettingsError

	modifiedGlobal  map[string]struct{}
	modifiedProject map[string]struct{}
}

func NewSettingsManager(cwd, agentDir string) *SettingsManager {
	manager := &SettingsManager{
		cwd:             cwd,
		agentDir:        agentDir,
		globalPath:      filepath.Join(agentDir, "settings.json"),
		projectPath:     filepath.Join(cwd, ".pi", "settings.json"),
		modifiedGlobal:  map[string]struct{}{},
		modifiedProject: map[string]struct{}{},
	}
	manager.global, manager.globalLoadErr = loadSettingsFile(manager.globalPath)
	manager.project, manager.projectLoadErr = loadSettingsFile(manager.projectPath)
	if manager.globalLoadErr != nil {
		manager.errors = append(manager.errors, SettingsError{Scope: "global", Err: manager.globalLoadErr})
	}
	if manager.projectLoadErr != nil {
		manager.errors = append(manager.errors, SettingsError{Scope: "project", Err: manager.projectLoadErr})
	}
	manager.refreshMerged()
	return manager
}

func loadSettingsFile(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return map[string]any{}, err
	}
	var settings map[string]any
	if err := json.Unmarshal(content, &settings); err != nil {
		return map[string]any{}, err
	}
	if settings == nil {
		settings = map[string]any{}
	}
	return migrateSettings(settings), nil
}

func migrateSettings(settings map[string]any) map[string]any {
	if value, ok := settings["queueMode"]; ok {
		if _, exists := settings["steeringMode"]; !exists {
			settings["steeringMode"] = value
		}
		delete(settings, "queueMode")
	}
	if value, ok := settings["websockets"].(bool); ok {
		if _, exists := settings["transport"]; !exists {
			if value {
				settings["transport"] = "websocket"
			} else {
				settings["transport"] = "sse"
			}
		}
		delete(settings, "websockets")
	}
	return settings
}

func (s *SettingsManager) refreshMerged() {
	s.merged = mergeSettings(s.global, s.project)
}

func mergeSettings(base, overrides map[string]any) map[string]any {
	result := cloneSettingsMap(base)
	for key, overrideValue := range overrides {
		if overrideMap, ok := overrideValue.(map[string]any); ok {
			if baseMap, ok := result[key].(map[string]any); ok {
				result[key] = mergeSettings(baseMap, overrideMap)
				continue
			}
		}
		result[key] = cloneSettingsValue(overrideValue)
	}
	return result
}

func cloneSettingsMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = cloneSettingsValue(value)
	}
	return output
}

func cloneSettingsValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneSettingsMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneSettingsValue(item)
		}
		return out
	default:
		return typed
	}
}

func (s *SettingsManager) Reload() {
	if settings, err := loadSettingsFile(s.globalPath); err != nil {
		s.globalLoadErr = err
		s.errors = append(s.errors, SettingsError{Scope: "global", Err: err})
	} else {
		s.global = settings
		s.globalLoadErr = nil
	}
	if settings, err := loadSettingsFile(s.projectPath); err != nil {
		s.projectLoadErr = err
		s.errors = append(s.errors, SettingsError{Scope: "project", Err: err})
	} else {
		s.project = settings
		s.projectLoadErr = nil
	}
	clear(s.modifiedGlobal)
	clear(s.modifiedProject)
	s.refreshMerged()
}

func (s *SettingsManager) Flush() error {
	return nil
}

func (s *SettingsManager) DrainErrors() []SettingsError {
	drained := append([]SettingsError(nil), s.errors...)
	s.errors = nil
	return drained
}

func (s *SettingsManager) GetTheme() string {
	return settingsString(s.merged, "theme")
}

func (s *SettingsManager) SetTheme(theme string) {
	s.setGlobal("theme", theme)
}

func (s *SettingsManager) GetDefaultModel() string {
	return settingsString(s.merged, "defaultModel")
}

func (s *SettingsManager) GetDefaultThinkingLevel() string {
	return settingsString(s.merged, "defaultThinkingLevel")
}

func (s *SettingsManager) SetDefaultThinkingLevel(level string) {
	s.setGlobal("defaultThinkingLevel", level)
}

func (s *SettingsManager) GetShellCommandPrefix() string {
	return settingsString(s.merged, "shellCommandPrefix")
}

func (s *SettingsManager) GetSessionDir() string {
	sessionDir := settingsString(s.merged, "sessionDir")
	if sessionDir == "" {
		return ""
	}
	return ExpandPath(sessionDir)
}

func (s *SettingsManager) GetPackages() []any {
	return settingsSlice(s.merged, "packages")
}

func (s *SettingsManager) SetProjectPackages(packages []any) {
	s.setProject("packages", cloneSettingsValue(packages))
}

func (s *SettingsManager) GetExtensionPaths() []string {
	return settingsStringSlice(s.merged, "extensions")
}

func (s *SettingsManager) SetProjectExtensionPaths(paths []string) {
	values := make([]any, len(paths))
	for i, path := range paths {
		values[i] = path
	}
	s.setProject("extensions", values)
}

func (s *SettingsManager) setGlobal(key string, value any) {
	s.global[key] = cloneSettingsValue(value)
	s.modifiedGlobal[key] = struct{}{}
	s.refreshMerged()
	s.saveGlobal()
}

func (s *SettingsManager) setProject(key string, value any) {
	s.project[key] = cloneSettingsValue(value)
	s.modifiedProject[key] = struct{}{}
	s.refreshMerged()
	s.saveProject()
}

func (s *SettingsManager) saveGlobal() {
	if s.globalLoadErr != nil {
		return
	}
	if err := saveModifiedSettings(s.globalPath, s.global, s.modifiedGlobal); err != nil {
		s.errors = append(s.errors, SettingsError{Scope: "global", Err: err})
		return
	}
	clear(s.modifiedGlobal)
}

func (s *SettingsManager) saveProject() {
	if s.projectLoadErr != nil {
		return
	}
	if err := saveModifiedSettings(s.projectPath, s.project, s.modifiedProject); err != nil {
		s.errors = append(s.errors, SettingsError{Scope: "project", Err: err})
		return
	}
	clear(s.modifiedProject)
}

func saveModifiedSettings(path string, inMemory map[string]any, modified map[string]struct{}) error {
	current, err := loadSettingsFile(path)
	if os.IsNotExist(err) {
		current = map[string]any{}
	} else if err != nil {
		return err
	}
	for key := range modified {
		current[key] = cloneSettingsValue(inMemory[key])
	}
	content, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(content, '\n'), 0o644)
}

func settingsString(settings map[string]any, key string) string {
	value, _ := settings[key].(string)
	return value
}

func settingsSlice(settings map[string]any, key string) []any {
	values, ok := settings[key].([]any)
	if !ok {
		return nil
	}
	return cloneSettingsValue(values).([]any)
}

func settingsStringSlice(settings map[string]any, key string) []string {
	values := settingsSlice(settings, key)
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if stringValue, ok := value.(string); ok {
			result = append(result, stringValue)
		}
	}
	return result
}
