package gicodingagent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
)

type SettingsError struct {
	Scope string
	Err   error
}

type SettingsManager struct {
	cwd            string
	agentDir       string
	globalPath     string
	projectPath    string
	inMemory       bool
	projectTrusted bool

	global  map[string]any
	project map[string]any
	merged  map[string]any

	globalLoadErr  error
	projectLoadErr error
	errors         []SettingsError

	modifiedGlobal  map[string]struct{}
	modifiedProject map[string]struct{}
}

type WarningSettings struct {
	AnthropicExtraUsage bool
}

const defaultHTTPIdleTimeoutMS = 300_000
const defaultAgentSessionMaxRetries = 3
const defaultAgentSessionBaseDelayMS = 2_000
const defaultProviderMaxRetryDelayMS = 60_000

type ProviderRetrySettings struct {
	TimeoutMS       int
	MaxRetries      int
	MaxRetryDelayMS int
}

func NewSettingsManager(cwd, agentDir string) *SettingsManager {
	return NewSettingsManagerWithOptions(cwd, agentDir, SettingsManagerOptions{ProjectTrusted: true})
}

type SettingsManagerOptions struct {
	ProjectTrusted bool
}

func NewSettingsManagerWithOptions(cwd, agentDir string, options SettingsManagerOptions) *SettingsManager {
	manager := &SettingsManager{
		cwd:             cwd,
		agentDir:        agentDir,
		globalPath:      filepath.Join(agentDir, "settings.json"),
		projectPath:     filepath.Join(cwd, ConfigDirName, "settings.json"),
		projectTrusted:  options.ProjectTrusted,
		modifiedGlobal:  map[string]struct{}{},
		modifiedProject: map[string]struct{}{},
	}
	manager.global, manager.globalLoadErr = loadSettingsFile(manager.globalPath)
	manager.project = map[string]any{}
	if manager.projectTrusted {
		manager.project, manager.projectLoadErr = loadSettingsFile(manager.projectPath)
	}
	if manager.globalLoadErr != nil {
		manager.errors = append(manager.errors, SettingsError{Scope: "global", Err: manager.globalLoadErr})
	}
	if manager.projectLoadErr != nil {
		manager.errors = append(manager.errors, SettingsError{Scope: "project", Err: manager.projectLoadErr})
	}
	manager.refreshMerged()
	return manager
}

func NewInMemorySettingsManager(settings map[string]any) *SettingsManager {
	manager := &SettingsManager{
		inMemory:        true,
		projectTrusted:  true,
		global:          migrateSettings(cloneSettingsMap(settings)),
		project:         map[string]any{},
		modifiedGlobal:  map[string]struct{}{},
		modifiedProject: map[string]struct{}{},
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
	if skillsSettings, ok := settings["skills"].(map[string]any); ok {
		if value, exists := skillsSettings["enableSkillCommands"]; exists {
			if _, topLevelExists := settings["enableSkillCommands"]; !topLevelExists {
				settings["enableSkillCommands"] = cloneSettingsValue(value)
			}
		}
		if customDirectories, ok := skillsSettings["customDirectories"].([]any); ok && len(customDirectories) > 0 {
			settings["skills"] = cloneSettingsValue(customDirectories)
		} else {
			delete(settings, "skills")
		}
	}
	if retrySettings, ok := settings["retry"].(map[string]any); ok {
		if maxDelayMS := retrySettings["maxDelayMs"]; settingsValueIsNumber(maxDelayMS) {
			providerSettings, _ := retrySettings["provider"].(map[string]any)
			if providerSettings == nil {
				providerSettings = map[string]any{}
			} else {
				providerSettings = cloneSettingsMap(providerSettings)
			}
			if providerSettings["maxRetryDelayMs"] == nil {
				providerSettings["maxRetryDelayMs"] = cloneSettingsValue(maxDelayMS)
				retrySettings["provider"] = providerSettings
			}
		}
		delete(retrySettings, "maxDelayMs")
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
	if s.inMemory {
		clear(s.modifiedGlobal)
		clear(s.modifiedProject)
		s.refreshMerged()
		return
	}
	if settings, err := loadSettingsFile(s.globalPath); err != nil {
		s.globalLoadErr = err
		s.errors = append(s.errors, SettingsError{Scope: "global", Err: err})
	} else {
		s.global = settings
		s.globalLoadErr = nil
	}
	if s.projectTrusted {
		if settings, err := loadSettingsFile(s.projectPath); err != nil {
			s.projectLoadErr = err
			s.errors = append(s.errors, SettingsError{Scope: "project", Err: err})
		} else {
			s.project = settings
			s.projectLoadErr = nil
		}
	} else {
		s.project = map[string]any{}
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

func (s *SettingsManager) GetGlobalSettings() map[string]any {
	return cloneSettingsMap(s.global)
}

func (s *SettingsManager) GetProjectSettings() map[string]any {
	return cloneSettingsMap(s.project)
}

func (s *SettingsManager) IsProjectTrusted() bool {
	return s != nil && s.projectTrusted
}

func (s *SettingsManager) SetProjectTrusted(trusted bool) {
	if s == nil || s.projectTrusted == trusted {
		return
	}
	s.projectTrusted = trusted
	clear(s.modifiedProject)
	if !trusted {
		s.project = map[string]any{}
		s.projectLoadErr = nil
		s.refreshMerged()
		return
	}
	if s.inMemory {
		s.project = map[string]any{}
		s.projectLoadErr = nil
		s.refreshMerged()
		return
	}
	settings, err := loadSettingsFile(s.projectPath)
	if err != nil {
		s.project = map[string]any{}
		s.projectLoadErr = err
		s.errors = append(s.errors, SettingsError{Scope: "project", Err: err})
	} else {
		s.project = settings
		s.projectLoadErr = nil
	}
	s.refreshMerged()
}

func (s *SettingsManager) GetDefaultProjectTrust() DefaultProjectTrust {
	return normalizeDefaultProjectTrust(DefaultProjectTrust(settingsString(s.global, "defaultProjectTrust")))
}

func (s *SettingsManager) SetDefaultProjectTrust(defaultProjectTrust DefaultProjectTrust) {
	s.setGlobal("defaultProjectTrust", string(normalizeDefaultProjectTrust(defaultProjectTrust)))
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

func (s *SettingsManager) SetDefaultModel(model string) {
	s.setGlobal("defaultModel", model)
}

func (s *SettingsManager) GetDefaultProvider() string {
	return settingsString(s.merged, "defaultProvider")
}

func (s *SettingsManager) SetDefaultProvider(provider string) {
	s.setGlobal("defaultProvider", provider)
}

func (s *SettingsManager) GetDefaultThinkingLevel() string {
	return settingsString(s.merged, "defaultThinkingLevel")
}

func (s *SettingsManager) SetDefaultThinkingLevel(level string) {
	s.setGlobal("defaultThinkingLevel", level)
}

func (s *SettingsManager) GetTransport() string {
	return settingsEnum(s.merged, "transport", "auto", []string{"sse", "websocket", "websocket-cached", "auto"})
}

func (s *SettingsManager) SetTransport(transport string) {
	s.setGlobal("transport", normalizeSettingsEnum(transport, "auto", []string{"sse", "websocket", "websocket-cached", "auto"}))
}

func (s *SettingsManager) GetHTTPIdleTimeoutMS() int {
	timeout := settingsInt(s.merged, "httpIdleTimeoutMs", defaultHTTPIdleTimeoutMS)
	if timeout < 0 {
		return defaultHTTPIdleTimeoutMS
	}
	return timeout
}

func (s *SettingsManager) SetHTTPIdleTimeoutMS(timeoutMS int) {
	if timeoutMS < 0 {
		timeoutMS = defaultHTTPIdleTimeoutMS
	}
	s.setGlobal("httpIdleTimeoutMs", timeoutMS)
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

func (s *SettingsManager) GetImageAutoResize() bool {
	return settingsNestedBool(s.merged, "images", "autoResize", true)
}

func (s *SettingsManager) SetImageAutoResize(enabled bool) {
	s.setGlobalNested("images", "autoResize", enabled)
}

func (s *SettingsManager) GetBlockImages() bool {
	return settingsNestedBool(s.merged, "images", "blockImages", false)
}

func (s *SettingsManager) SetBlockImages(blocked bool) {
	s.setGlobalNested("images", "blockImages", blocked)
}

func (s *SettingsManager) GetShowImages() bool {
	return settingsNestedBool(s.merged, "terminal", "showImages", true)
}

func (s *SettingsManager) SetShowImages(enabled bool) {
	s.setGlobalNested("terminal", "showImages", enabled)
}

func (s *SettingsManager) GetImageWidthCells() int {
	width := settingsNestedInt(s.merged, "terminal", "imageWidthCells", 60)
	if width <= 0 {
		return 60
	}
	return width
}

func (s *SettingsManager) SetImageWidthCells(width int) {
	if width <= 0 {
		width = 60
	}
	s.setGlobalNested("terminal", "imageWidthCells", width)
}

func (s *SettingsManager) GetClearOnShrink() bool {
	return settingsNestedBool(s.merged, "terminal", "clearOnShrink", false)
}

func (s *SettingsManager) SetClearOnShrink(enabled bool) {
	s.setGlobalNested("terminal", "clearOnShrink", enabled)
}

func (s *SettingsManager) GetShowTerminalProgress() bool {
	return settingsNestedBool(s.merged, "terminal", "showTerminalProgress", false)
}

func (s *SettingsManager) SetShowTerminalProgress(enabled bool) {
	s.setGlobalNested("terminal", "showTerminalProgress", enabled)
}

func (s *SettingsManager) GetEnableInstallTelemetry() bool {
	return settingsBool(s.merged, "enableInstallTelemetry", true)
}

func (s *SettingsManager) SetEnableInstallTelemetry(enabled bool) {
	s.setGlobal("enableInstallTelemetry", enabled)
}

func (s *SettingsManager) GetEnableSkillCommands() bool {
	return settingsBool(s.merged, "enableSkillCommands", true)
}

func (s *SettingsManager) SetEnableSkillCommands(enabled bool) {
	s.setGlobal("enableSkillCommands", enabled)
}

func (s *SettingsManager) GetHideThinkingBlock() bool {
	return settingsBool(s.merged, "hideThinkingBlock", false)
}

func (s *SettingsManager) SetHideThinkingBlock(hidden bool) {
	s.setGlobal("hideThinkingBlock", hidden)
}

func (s *SettingsManager) GetCollapseChangelog() bool {
	return settingsBool(s.merged, "collapseChangelog", false)
}

func (s *SettingsManager) SetCollapseChangelog(collapsed bool) {
	s.setGlobal("collapseChangelog", collapsed)
}

func (s *SettingsManager) GetLastChangelogVersion() string {
	return settingsString(s.merged, "lastChangelogVersion")
}

func (s *SettingsManager) SetLastChangelogVersion(version string) {
	s.setGlobal("lastChangelogVersion", version)
}

func (s *SettingsManager) GetQuietStartup() bool {
	return settingsBool(s.merged, "quietStartup", false)
}

func (s *SettingsManager) SetQuietStartup(enabled bool) {
	s.setGlobal("quietStartup", enabled)
}

func (s *SettingsManager) GetDoubleEscapeAction() string {
	return settingsEnum(s.merged, "doubleEscapeAction", "tree", []string{"tree", "fork", "none"})
}

func (s *SettingsManager) SetDoubleEscapeAction(action string) {
	s.setGlobal("doubleEscapeAction", normalizeSettingsEnum(action, "tree", []string{"tree", "fork", "none"}))
}

func (s *SettingsManager) GetTreeFilterMode() string {
	return settingsEnum(s.merged, "treeFilterMode", "default", []string{"default", "no-tools", "user-only", "labeled-only", "all"})
}

func (s *SettingsManager) SetTreeFilterMode(mode string) {
	s.setGlobal("treeFilterMode", normalizeSettingsEnum(mode, "default", []string{"default", "no-tools", "user-only", "labeled-only", "all"}))
}

func (s *SettingsManager) GetBranchSummarySkipPrompt() bool {
	return settingsNestedBool(s.merged, "branchSummary", "skipPrompt", false)
}

func (s *SettingsManager) SetBranchSummarySkipPrompt(skip bool) {
	s.setGlobalNested("branchSummary", "skipPrompt", skip)
}

func (s *SettingsManager) GetSteeringMode() string {
	return settingsEnum(
		s.merged,
		"steeringMode",
		"one-at-a-time",
		[]string{"one-at-a-time", "all"},
	)
}

func (s *SettingsManager) SetSteeringMode(mode string) {
	s.setGlobal(
		"steeringMode",
		normalizeSettingsEnum(
			mode,
			"one-at-a-time",
			[]string{"one-at-a-time", "all"},
		),
	)
}

func (s *SettingsManager) GetFollowUpMode() string {
	return settingsEnum(
		s.merged,
		"followUpMode",
		"one-at-a-time",
		[]string{"one-at-a-time", "all"},
	)
}

func (s *SettingsManager) SetFollowUpMode(mode string) {
	s.setGlobal(
		"followUpMode",
		normalizeSettingsEnum(
			mode,
			"one-at-a-time",
			[]string{"one-at-a-time", "all"},
		),
	)
}

func (s *SettingsManager) GetCompactionEnabled() bool {
	return settingsNestedBool(
		s.merged,
		"compaction",
		"enabled",
		agentharness.DefaultCompactionSettings.Enabled,
	)
}

func (s *SettingsManager) SetCompactionEnabled(enabled bool) {
	s.setGlobalNested("compaction", "enabled", enabled)
}

func (s *SettingsManager) GetCompactionReserveTokens() int {
	return settingsNestedInt(
		s.merged,
		"compaction",
		"reserveTokens",
		agentharness.DefaultCompactionSettings.ReserveTokens,
	)
}

func (s *SettingsManager) GetCompactionKeepRecentTokens() int {
	return settingsNestedInt(
		s.merged,
		"compaction",
		"keepRecentTokens",
		agentharness.DefaultCompactionSettings.KeepRecentTokens,
	)
}

func (s *SettingsManager) GetCompactionSettings() agentharness.CompactionSettings {
	return agentharness.CompactionSettings{
		Enabled:          s.GetCompactionEnabled(),
		ReserveTokens:    s.GetCompactionReserveTokens(),
		KeepRecentTokens: s.GetCompactionKeepRecentTokens(),
	}
}

func (s *SettingsManager) GetRetryEnabled() bool {
	return settingsNestedBool(s.merged, "retry", "enabled", true)
}

func (s *SettingsManager) SetRetryEnabled(enabled bool) {
	s.setGlobalNested("retry", "enabled", enabled)
}

func (s *SettingsManager) GetRetrySettings() AgentSessionRetrySettings {
	return AgentSessionRetrySettings{
		Enabled:     s.GetRetryEnabled(),
		MaxRetries:  settingsNestedInt(s.merged, "retry", "maxRetries", defaultAgentSessionMaxRetries),
		BaseDelayMs: settingsNestedInt(s.merged, "retry", "baseDelayMs", defaultAgentSessionBaseDelayMS),
	}
}

func (s *SettingsManager) GetProviderRetrySettings() ProviderRetrySettings {
	retrySettings, _ := s.merged["retry"].(map[string]any)
	providerSettings, _ := retrySettings["provider"].(map[string]any)
	return ProviderRetrySettings{
		TimeoutMS:       settingsValueInt(providerSettings["timeoutMs"], 0),
		MaxRetries:      settingsValueInt(providerSettings["maxRetries"], 0),
		MaxRetryDelayMS: settingsValueInt(providerSettings["maxRetryDelayMs"], defaultProviderMaxRetryDelayMS),
	}
}

func (s *SettingsManager) GetEditorPaddingX() int {
	padding := settingsInt(s.merged, "editorPaddingX", 0)
	if padding < 0 {
		return 0
	}
	return padding
}

func (s *SettingsManager) SetEditorPaddingX(padding int) {
	if padding < 0 {
		padding = 0
	}
	s.setGlobal("editorPaddingX", padding)
}

func (s *SettingsManager) GetAutocompleteMaxVisible() int {
	visible := settingsInt(s.merged, "autocompleteMaxVisible", 5)
	if visible <= 0 {
		return 5
	}
	return visible
}

func (s *SettingsManager) SetAutocompleteMaxVisible(visible int) {
	if visible <= 0 {
		visible = 5
	}
	s.setGlobal("autocompleteMaxVisible", visible)
}

func (s *SettingsManager) GetShowHardwareCursor() bool {
	return settingsBool(s.merged, "showHardwareCursor", false)
}

func (s *SettingsManager) SetShowHardwareCursor(enabled bool) {
	s.setGlobal("showHardwareCursor", enabled)
}

func (s *SettingsManager) GetWarnings() WarningSettings {
	return WarningSettings{
		AnthropicExtraUsage: settingsNestedBool(s.merged, "warnings", "anthropicExtraUsage", true),
	}
}

func (s *SettingsManager) SetWarnings(warnings WarningSettings) {
	s.setGlobal("warnings", map[string]any{"anthropicExtraUsage": warnings.AnthropicExtraUsage})
}

func (s *SettingsManager) SetWarningAnthropicExtraUsage(enabled bool) {
	s.setGlobalNested("warnings", "anthropicExtraUsage", enabled)
}

func (s *SettingsManager) GetEnabledModels() []string {
	return settingsStringSlice(s.merged, "enabledModels")
}

func (s *SettingsManager) SetEnabledModels(models []string) {
	values := make([]any, len(models))
	for i, model := range models {
		values[i] = model
	}
	s.setGlobal("enabledModels", values)
}

func (s *SettingsManager) GetPackages() []any {
	return settingsSlice(s.merged, "packages")
}

func (s *SettingsManager) SetPackages(packages []any) {
	s.setGlobal("packages", cloneSettingsValue(packages))
}

func (s *SettingsManager) SetProjectPackages(packages []any) error {
	return s.setProject("packages", cloneSettingsValue(packages))
}

func (s *SettingsManager) GetExtensionPaths() []string {
	return settingsStringSlice(s.merged, "extensions")
}

func (s *SettingsManager) SetProjectExtensionPaths(paths []string) error {
	return s.setProject("extensions", settingsStringsValue(paths))
}

func (s *SettingsManager) SetProjectSkillPaths(paths []string) error {
	return s.setProject("skills", settingsStringsValue(paths))
}

func (s *SettingsManager) SetProjectPromptTemplatePaths(paths []string) error {
	return s.setProject("prompts", settingsStringsValue(paths))
}

func (s *SettingsManager) SetProjectThemePaths(paths []string) error {
	return s.setProject("themes", settingsStringsValue(paths))
}

func (s *SettingsManager) setGlobal(key string, value any) {
	s.global[key] = cloneSettingsValue(value)
	s.modifiedGlobal[key] = struct{}{}
	s.refreshMerged()
	s.saveGlobal()
}

func (s *SettingsManager) setGlobalNested(key, nestedKey string, value any) {
	nested, _ := s.global[key].(map[string]any)
	if nested == nil {
		nested = map[string]any{}
		s.global[key] = nested
	}
	nested[nestedKey] = cloneSettingsValue(value)
	s.modifiedGlobal[key] = struct{}{}
	s.refreshMerged()
	s.saveGlobal()
}

func (s *SettingsManager) setProject(key string, value any) error {
	if !s.projectTrusted {
		return errors.New("Project is not trusted; refusing to write project settings")
	}
	s.project[key] = cloneSettingsValue(value)
	s.modifiedProject[key] = struct{}{}
	s.refreshMerged()
	return s.saveProject()
}

func settingsStringsValue(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func (s *SettingsManager) saveGlobal() {
	if s.inMemory {
		clear(s.modifiedGlobal)
		return
	}
	if s.globalLoadErr != nil {
		return
	}
	if err := saveModifiedSettings(s.globalPath, s.global, s.modifiedGlobal); err != nil {
		s.errors = append(s.errors, SettingsError{Scope: "global", Err: err})
		return
	}
	clear(s.modifiedGlobal)
}

func (s *SettingsManager) saveProject() error {
	if s.inMemory {
		clear(s.modifiedProject)
		return nil
	}
	if s.projectLoadErr != nil {
		return s.projectLoadErr
	}
	if err := saveModifiedSettings(s.projectPath, s.project, s.modifiedProject); err != nil {
		s.errors = append(s.errors, SettingsError{Scope: "project", Err: err})
		return err
	}
	clear(s.modifiedProject)
	return nil
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

func settingsEnum(settings map[string]any, key, defaultValue string, allowed []string) string {
	return normalizeSettingsEnum(settingsString(settings, key), defaultValue, allowed)
}

func normalizeSettingsEnum(value, defaultValue string, allowed []string) string {
	for _, allowedValue := range allowed {
		if value == allowedValue {
			return value
		}
	}
	return defaultValue
}

func settingsNestedBool(settings map[string]any, key, nestedKey string, defaultValue bool) bool {
	nested, ok := settings[key].(map[string]any)
	if !ok {
		return defaultValue
	}
	value, ok := nested[nestedKey].(bool)
	if !ok {
		return defaultValue
	}
	return value
}

func settingsBool(settings map[string]any, key string, defaultValue bool) bool {
	value, ok := settings[key].(bool)
	if !ok {
		return defaultValue
	}
	return value
}

func settingsInt(settings map[string]any, key string, defaultValue int) int {
	return settingsValueInt(settings[key], defaultValue)
}

func settingsNestedInt(settings map[string]any, key, nestedKey string, defaultValue int) int {
	nested, ok := settings[key].(map[string]any)
	if !ok {
		return defaultValue
	}
	return settingsValueInt(nested[nestedKey], defaultValue)
}

func settingsValueInt(value any, defaultValue int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		if typed == float64(int(typed)) {
			return int(typed)
		}
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed)
		}
	}
	return defaultValue
}

func settingsValueIsNumber(value any) bool {
	switch value.(type) {
	case int, int64, float64, json.Number:
		return true
	default:
		return false
	}
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
