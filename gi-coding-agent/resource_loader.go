package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
)

type DefaultResourceLoaderOptions struct {
	CWD                      string
	AgentDir                 string
	SettingsManager          *SettingsManager
	NoExtensions             bool
	NoContextFiles           bool
	NoSkills                 bool
	NoPromptTemplates        bool
	NoThemes                 bool
	AdditionalExtensionPaths []string
	AdditionalSkillPaths     []string
	AdditionalPromptPaths    []string
	AdditionalThemePaths     []string
	SystemPrompt             string
	AppendSystemPrompt       []string
	SkillsOverride           func() ResourceSkillsResult
	SystemPromptOverride     func() string
	ExtensionFactories       []ProtocolExtensionFactory
}

type ResourceExtensionsResult struct {
	Extensions        []ProtocolExtensionSource
	ProcessExtensions []ProtocolPackageProcessExtension
	Errors            []ProtocolExtensionDiscoveryError
	Runtime           *ProtocolExtensionRuntime
}

type ResourceSkillsResult = AgentSessionSkillsResult

type ResourcePromptsResult struct {
	Prompts []PromptTemplate
}

type ResourceTheme struct {
	Name       string
	SourcePath string
}

type ResourceThemesResult struct {
	Themes      []ResourceTheme
	Diagnostics []agentharness.SkillDiagnostic
}

type ResourceContextFile struct {
	Path    string
	Content string
}

type ResourceAgentsFilesResult struct {
	AgentsFiles []ResourceContextFile
}

type ResourceExtensionPath struct {
	Path     string
	Metadata ProtocolSourceInfo
}

type ResourceSkillPath struct {
	Path     string
	Metadata ProtocolSourceInfo
}

type ResourcePromptPath struct {
	Path     string
	Metadata ProtocolSourceInfo
}

type ResourceThemePath struct {
	Path     string
	Metadata ProtocolSourceInfo
}

type ResourceExtension struct {
	ExtensionPaths []ResourceExtensionPath
	SkillPaths     []ResourceSkillPath
	PromptPaths    []ResourcePromptPath
	ThemePaths     []ResourceThemePath
}

type DefaultResourceLoader struct {
	mu                       sync.RWMutex
	cwd                      string
	agentDir                 string
	settingsManager          *SettingsManager
	noExtensions             bool
	noContextFiles           bool
	noSkills                 bool
	noPromptTemplates        bool
	noThemes                 bool
	additionalExtensionPaths []string
	additionalSkillPaths     []string
	additionalPromptPaths    []string
	additionalThemePaths     []string
	systemPromptSource       string
	appendSystemSources      []string
	skillsOverride           func() ResourceSkillsResult
	systemPromptOverride     func() string
	extensionFactories       []ProtocolExtensionFactory

	extensions       ResourceExtensionsResult
	skills           ResourceSkillsResult
	prompts          ResourcePromptsResult
	themes           ResourceThemesResult
	agentsFiles      ResourceAgentsFilesResult
	systemPrompt     string
	appendSystem     string
	extensionSkills  []ResourceSkillPath
	extensionPrompts []ResourcePromptPath
	extensionThemes  []ResourceThemePath
	runtimeSkills    []ResourceSkillPath
	runtimePrompts   []ResourcePromptPath
	runtimeThemes    []ResourceThemePath
	packageResources ProtocolPackageResources
	packageLoadError error
	extendedSkills   []ResourceSkillPath
	extendedPrompts  []ResourcePromptPath
	extendedThemes   []ResourceThemePath
	extendedExtPaths []ResourceExtensionPath
	reloadCount      int
}

func NewDefaultResourceLoader(options DefaultResourceLoaderOptions) *DefaultResourceLoader {
	cwd := options.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	settingsManager := options.SettingsManager
	if settingsManager == nil {
		settingsManager = NewSettingsManager(cwd, options.AgentDir)
	}
	return &DefaultResourceLoader{
		cwd:                      cwd,
		agentDir:                 options.AgentDir,
		settingsManager:          settingsManager,
		noExtensions:             options.NoExtensions,
		noContextFiles:           options.NoContextFiles,
		noSkills:                 options.NoSkills,
		noPromptTemplates:        options.NoPromptTemplates,
		noThemes:                 options.NoThemes,
		additionalExtensionPaths: append([]string(nil), options.AdditionalExtensionPaths...),
		additionalSkillPaths:     append([]string(nil), options.AdditionalSkillPaths...),
		additionalPromptPaths:    append([]string(nil), options.AdditionalPromptPaths...),
		additionalThemePaths:     append([]string(nil), options.AdditionalThemePaths...),
		systemPromptSource:       options.SystemPrompt,
		appendSystemSources:      append([]string(nil), options.AppendSystemPrompt...),
		skillsOverride:           options.SkillsOverride,
		systemPromptOverride:     options.SystemPromptOverride,
		extensionFactories:       append([]ProtocolExtensionFactory(nil), options.ExtensionFactories...),
		extensions:               ResourceExtensionsResult{Runtime: NewDefaultProtocolExtensionRuntime()},
	}
}

func (l *DefaultResourceLoader) Reload() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	reason := "startup"
	if l.reloadCount > 0 {
		reason = "reload"
	}
	l.reloadCount++
	if l.settingsManager != nil {
		l.settingsManager.Reload()
	}
	l.packageResources, l.packageLoadError = l.loadPackageResources()
	l.extensions = l.loadExtensions()
	l.discoverRuntimeResources(reason)
	l.skills = l.loadSkills()
	l.prompts = ResourcePromptsResult{Prompts: l.loadPrompts()}
	l.themes = l.loadThemes()
	l.agentsFiles = ResourceAgentsFilesResult{AgentsFiles: l.loadAgentsFiles()}
	l.systemPrompt = l.loadSystemPrompt()
	l.appendSystem = l.loadAppendSystemPrompt()
}

func (l *DefaultResourceLoader) ExtendResources(resources ResourceExtension) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.extendedExtPaths = append(l.extendedExtPaths, resources.ExtensionPaths...)
	l.extendedSkills = append(l.extendedSkills, resources.SkillPaths...)
	l.extendedPrompts = append(l.extendedPrompts, resources.PromptPaths...)
	l.extendedThemes = append(l.extendedThemes, resources.ThemePaths...)
	if len(resources.ExtensionPaths) > 0 {
		l.extensions = l.loadExtensions()
		l.discoverRuntimeResources("reload")
	}
	l.skills = l.loadSkills()
	l.prompts = ResourcePromptsResult{Prompts: l.loadPrompts()}
	l.themes = l.loadThemes()
}

func (l *DefaultResourceLoader) GetExtensions() ResourceExtensionsResult {
	if l == nil {
		return ResourceExtensionsResult{}
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return cloneResourceExtensionsResult(l.extensions)
}

func (l *DefaultResourceLoader) ApplyExtensionFlagValues(values map[string]any, allowDeferred bool) []ProtocolExtensionDiscoveryError {
	if l == nil || len(values) == 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.extensions.Runtime == nil {
		return nil
	}
	var diagnostics []ProtocolExtensionFlagDiagnostic
	diagnostics = append(diagnostics, l.extensions.Runtime.SetCLIFlagValues(values)...)
	if !allowDeferred {
		diagnostics = append(diagnostics, l.extensions.Runtime.UnknownCLIFlagDiagnostics()...)
	}
	if len(diagnostics) == 0 {
		return nil
	}
	errors := make([]ProtocolExtensionDiscoveryError, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		errors = append(errors, ProtocolExtensionDiscoveryError{
			Path:  "extension flags",
			Error: diagnostic.Message,
		})
	}
	l.extensions.Errors = append(l.extensions.Errors, errors...)
	return errors
}

func (l *DefaultResourceLoader) GetSkills() ResourceSkillsResult {
	if l == nil {
		return ResourceSkillsResult{}
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return cloneResourceSkillsResult(l.skills)
}

func (l *DefaultResourceLoader) GetPrompts() ResourcePromptsResult {
	if l == nil {
		return ResourcePromptsResult{}
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return ResourcePromptsResult{
		Prompts: append([]PromptTemplate(nil), l.prompts.Prompts...),
	}
}

func (l *DefaultResourceLoader) GetThemes() ResourceThemesResult {
	if l == nil {
		return ResourceThemesResult{}
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return ResourceThemesResult{
		Themes: append([]ResourceTheme(nil), l.themes.Themes...),
		Diagnostics: append(
			[]agentharness.SkillDiagnostic(nil),
			l.themes.Diagnostics...,
		),
	}
}

func (l *DefaultResourceLoader) GetAgentsFiles() ResourceAgentsFilesResult {
	if l == nil {
		return ResourceAgentsFilesResult{}
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return ResourceAgentsFilesResult{
		AgentsFiles: append(
			[]ResourceContextFile(nil),
			l.agentsFiles.AgentsFiles...,
		),
	}
}

func (l *DefaultResourceLoader) GetSystemPrompt() string {
	if l == nil {
		return ""
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.systemPrompt
}

func (l *DefaultResourceLoader) GetAppendSystemPrompt() string {
	if l == nil {
		return ""
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.appendSystem
}

func cloneResourceExtensionsResult(
	result ResourceExtensionsResult,
) ResourceExtensionsResult {
	cloned := ResourceExtensionsResult{
		Extensions: append(
			[]ProtocolExtensionSource(nil),
			result.Extensions...,
		),
		ProcessExtensions: make(
			[]ProtocolPackageProcessExtension,
			len(result.ProcessExtensions),
		),
		Errors:  append([]ProtocolExtensionDiscoveryError(nil), result.Errors...),
		Runtime: result.Runtime,
	}
	for index, extension := range result.ProcessExtensions {
		cloned.ProcessExtensions[index] = extension
		cloned.ProcessExtensions[index].Command = append(
			[]string(nil),
			extension.Command...,
		)
		cloned.ProcessExtensions[index].Capabilities = append(
			[]string(nil),
			extension.Capabilities...,
		)
		cloned.ProcessExtensions[index].Env = cloneStringMap(extension.Env)
	}
	return cloned
}

func cloneResourceSkillsResult(result ResourceSkillsResult) ResourceSkillsResult {
	return ResourceSkillsResult{
		Skills: append([]agentharness.Skill(nil), result.Skills...),
		Diagnostics: append(
			[]agentharness.SkillDiagnostic(nil),
			result.Diagnostics...,
		),
	}
}

func (l *DefaultResourceLoader) loadExtensions() ResourceExtensionsResult {
	var combined ProtocolExtensionDiscoveryResult
	for _, source := range l.extendedExtPaths {
		combined.Extensions = append(combined.Extensions, ProtocolExtensionSource{Path: source.Path, BaseDir: filepath.Dir(source.Path), Metadata: source.Metadata})
	}
	if !l.noExtensions {
		for _, resource := range l.packageResources.Extensions {
			if !resource.Enabled {
				continue
			}
			combined.Extensions = append(combined.Extensions, ProtocolExtensionSource{Path: resource.Path, BaseDir: filepath.Dir(resource.Path), Metadata: resource.Metadata})
		}
		if l.packageLoadError != nil {
			combined.Errors = append(combined.Errors, ProtocolExtensionDiscoveryError{Path: "packages", Error: l.packageLoadError.Error()})
		}
		for _, source := range l.settingsExtensionSources() {
			combined.Extensions = append(combined.Extensions, source)
		}
	}
	explicit := LoadProtocolExtensionSources(l.additionalExtensionPaths, l.cwd)
	combined.Extensions = append(combined.Extensions, explicit.Extensions...)
	combined.Errors = append(combined.Errors, explicit.Errors...)
	if !l.noExtensions {
		var dirs []string
		if l.projectTrusted() {
			dirs = append(dirs, filepath.Join(l.cwd, ConfigDirName, "extensions"))
		}
		dirs = append(dirs, filepath.Join(l.agentDir, "extensions"))
		for _, dir := range dirs {
			discovered := discoverProtocolExtensionsInDir(dir)
			combined.Extensions = append(combined.Extensions, discovered.Extensions...)
			combined.Errors = append(combined.Errors, discovered.Errors...)
		}
	}
	filtered := filterProtocolExtensions(combined.Extensions, l.resourceFilters("extensions"), l.cwd, l.agentDir)
	extensions := dedupeProtocolExtensionSources(filtered)
	runtime := NewDefaultProtocolExtensionRuntime()
	loaded := LoadProtocolExtensionDescriptors(extensions, runtime)
	if len(l.extensionFactories) > 0 {
		if err := runtime.LoadFactories(l.extensionFactories); err != nil {
			loaded.Errors = append(loaded.Errors, ProtocolExtensionDiscoveryError{Path: "extension factories", Error: err.Error()})
		}
	}
	l.extensionSkills = loaded.Resources.SkillPaths
	l.extensionPrompts = loaded.Resources.PromptPaths
	l.extensionThemes = loaded.Resources.ThemePaths
	return ResourceExtensionsResult{
		Extensions:        extensions,
		ProcessExtensions: append([]ProtocolPackageProcessExtension(nil), l.packageResources.ProcessExtensions...),
		Errors:            append(combined.Errors, loaded.Errors...),
		Runtime:           runtime,
	}
}

func (l *DefaultResourceLoader) discoverRuntimeResources(reason string) {
	l.runtimeSkills = nil
	l.runtimePrompts = nil
	l.runtimeThemes = nil
	runtime := l.extensions.Runtime
	if runtime == nil || !runtime.HasHandlers(ProtocolEventResourcesDiscover) {
		return
	}
	result, err := runtime.EmitSessionEvent(ProtocolSessionEvent{Type: ProtocolEventResourcesDiscover, Reason: reason, CWD: l.cwd})
	if err != nil {
		l.extensions.Errors = append(l.extensions.Errors, ProtocolExtensionDiscoveryError{Path: "resources_discover", Error: err.Error()})
		return
	}
	if !result.ResourcesSet {
		return
	}
	l.runtimeSkills = append(l.runtimeSkills, result.Resources.SkillPaths...)
	l.runtimePrompts = append(l.runtimePrompts, result.Resources.PromptPaths...)
	l.runtimeThemes = append(l.runtimeThemes, result.Resources.ThemePaths...)
}

func (l *DefaultResourceLoader) loadSkills() ResourceSkillsResult {
	if l.skillsOverride != nil {
		return l.skillsOverride()
	}
	var result ResourceSkillsResult
	if !l.noSkills {
		for _, resource := range l.packageResources.Skills {
			if !resource.Enabled {
				continue
			}
			loaded := loadResourceSkillsWithMetadata(resource.Path, resource.Metadata)
			result.Skills = append(result.Skills, loaded.Skills...)
			result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
		}
	}
	if !l.noSkills {
		for _, dir := range l.settingsResourcePaths("skills") {
			loaded := loadResourceSkillsWithMetadata(dir, skillSourceMetadata(dir, "temporary"))
			result.Skills = append(result.Skills, loaded.Skills...)
			result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
		}
		for _, dir := range []string{filepath.Join(l.agentDir, "skills")} {
			loaded := loadResourceSkillsWithMetadata(dir, skillSourceMetadata(dir, "user"))
			result.Skills = append(result.Skills, loaded.Skills...)
			result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
		}
		userAgentsDir := userAgentsDirFromAgentDir(l.agentDir)
		if userAgentsDir != "" {
			dir := filepath.Join(userAgentsDir, "skills")
			loaded := loadResourceSkillsWithMetadataOptions(dir, skillSourceMetadata(dir, "user"), agentharness.LoadSkillsOptions{RespectGitignore: true})
			result.Skills = append(result.Skills, loaded.Skills...)
			result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
		}
		if l.projectTrusted() {
			for _, dir := range projectAgentsSkillDirs(l.cwd, userAgentsDir) {
				loaded := loadResourceSkillsWithMetadataOptions(dir, skillSourceMetadata(dir, "project"), agentharness.LoadSkillsOptions{RespectGitignore: true})
				result.Skills = append(result.Skills, loaded.Skills...)
				result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
			}
			loaded := loadResourceSkillsWithMetadata(filepath.Join(l.cwd, ConfigDirName, "skills"), skillSourceMetadata(filepath.Join(l.cwd, ConfigDirName, "skills"), "project"))
			result.Skills = append(result.Skills, loaded.Skills...)
			result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
		}
	}
	for _, path := range l.additionalSkillPaths {
		resolved := ResolveToCwd(path, l.cwd)
		loaded := loadResourceSkillsWithMetadata(resolved, skillSourceMetadata(resolved, "temporary"))
		result.Skills = append(result.Skills, loaded.Skills...)
		result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
	}
	for _, path := range l.extendedSkills {
		loaded := loadResourceSkillsWithMetadata(ResolveToCwd(path.Path, l.cwd), path.Metadata)
		result.Skills = append(result.Skills, loaded.Skills...)
		result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
	}
	for _, path := range l.extensionSkills {
		loaded := loadResourceSkillsWithMetadata(ResolveToCwd(path.Path, l.cwd), path.Metadata)
		result.Skills = append(result.Skills, loaded.Skills...)
		result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
	}
	for _, path := range l.runtimeSkills {
		loaded := loadResourceSkillsWithMetadata(ResolveToCwd(path.Path, l.cwd), path.Metadata)
		result.Skills = append(result.Skills, loaded.Skills...)
		result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
	}
	result.Skills = filterSkills(result.Skills, l.resourceFilters("skills"), l.cwd, l.agentDir)
	result.Diagnostics = append(result.Diagnostics, skillCollisionDiagnostics(result.Skills)...)
	result.Skills = dedupeSkillsByName(result.Skills)
	return result
}

func projectAgentsSkillDirs(cwd, userAgentsDir string) []string {
	var dirs []string
	for _, dir := range projectAgentsDirs(cwd, userAgentsDir) {
		dirs = append(dirs, filepath.Join(dir, "skills"))
	}
	return dirs
}

func userAgentsDirFromAgentDir(agentDir string) string {
	if agentDir == "" {
		return ""
	}
	agentDir = filepath.Clean(agentDir)
	configDir := filepath.Dir(agentDir)
	if filepath.Base(agentDir) != "agent" || filepath.Base(configDir) != ConfigDirName {
		if cliAgentDirOverridden(agentDir) {
			if home, err := os.UserHomeDir(); err == nil && home != "" {
				return filepath.Join(home, ".agents")
			}
		}
		return ""
	}
	return filepath.Join(filepath.Dir(configDir), ".agents")
}

func cliAgentDirOverridden(agentDir string) bool {
	for _, envName := range []string{"GI_CODING_AGENT_DIR", "PI_CODING_AGENT_DIR"} {
		value := strings.TrimSpace(os.Getenv(envName))
		if value == "" {
			continue
		}
		if filepath.Clean(ExpandPath(value)) == agentDir {
			return true
		}
	}
	return false
}

func projectAgentsDirs(cwd, userAgentsDir string) []string {
	var scanned []string
	current := filepath.Clean(cwd)
	for {
		agentsDir := filepath.Join(current, ".agents")
		if userAgentsDir == "" || filepath.Clean(agentsDir) != filepath.Clean(userAgentsDir) {
			scanned = append(scanned, agentsDir)
		}
		if hasGitDir(current) || isFilesystemRoot(current) {
			break
		}
		current = filepath.Dir(current)
	}
	for i, j := 0, len(scanned)-1; i < j; i, j = i+1, j-1 {
		scanned[i], scanned[j] = scanned[j], scanned[i]
	}
	return scanned
}

func hasGitDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

func isFilesystemRoot(path string) bool {
	parent := filepath.Dir(path)
	return parent == path
}

func (l *DefaultResourceLoader) loadPrompts() []PromptTemplate {
	var prompts []PromptTemplate
	if !l.noPromptTemplates {
		for _, resource := range l.packageResources.Prompts {
			if resource.Enabled {
				prompts = append(prompts, loadResourcePromptsWithMetadata(resource.Path, resource.Metadata)...)
			}
		}
		for _, path := range l.settingsResourcePaths("prompts") {
			prompts = append(prompts, LoadPromptTemplates(LoadPromptTemplatesOptions{Cwd: l.cwd, PromptPaths: []string{path}})...)
		}
		prompts = append(prompts, loadPromptTemplatesFromDir(filepath.Join(l.agentDir, "prompts"), sourceInfoForPromptPath(filepath.Join(l.agentDir, "prompts"), filepath.Join(l.agentDir, "prompts"), filepath.Join(l.cwd, ConfigDirName, "prompts")))...)
		if l.projectTrusted() {
			prompts = append(prompts, loadPromptTemplatesFromDir(filepath.Join(l.cwd, ConfigDirName, "prompts"), sourceInfoForPromptPath(filepath.Join(l.cwd, ConfigDirName, "prompts"), filepath.Join(l.agentDir, "prompts"), filepath.Join(l.cwd, ConfigDirName, "prompts")))...)
		}
	}
	for _, path := range l.additionalPromptPaths {
		prompts = append(prompts, LoadPromptTemplates(LoadPromptTemplatesOptions{Cwd: l.cwd, PromptPaths: []string{path}})...)
	}
	for _, path := range l.extendedPrompts {
		prompts = append(prompts, loadResourcePromptsWithMetadata(ResolveToCwd(path.Path, l.cwd), path.Metadata)...)
	}
	for _, path := range l.extensionPrompts {
		prompts = append(prompts, loadResourcePromptsWithMetadata(ResolveToCwd(path.Path, l.cwd), path.Metadata)...)
	}
	for _, path := range l.runtimePrompts {
		prompts = append(prompts, loadResourcePromptsWithMetadata(ResolveToCwd(path.Path, l.cwd), path.Metadata)...)
	}
	prompts = filterPrompts(prompts, l.resourceFilters("prompts"), l.cwd, l.agentDir)
	return dedupePromptsByName(prompts)
}

func loadResourceSkillsWithMetadata(path string, metadata ProtocolSourceInfo) agentharness.SkillResult {
	return loadResourceSkillsWithMetadataOptions(path, metadata, agentharness.LoadSkillsOptions{IncludeRootMarkdownFiles: true})
}

func loadResourceSkillsWithMetadataOptions(path string, metadata ProtocolSourceInfo, options agentharness.LoadSkillsOptions) agentharness.SkillResult {
	loadPath := path
	if filepath.Base(path) == "SKILL.md" {
		loadPath = filepath.Dir(path)
	}
	loaded := agentharness.LoadSkillsWithOptions(options, loadPath)
	for index := range loaded.Skills {
		info := metadata
		info.Path = loaded.Skills[index].FilePath
		loaded.Skills[index].SourceInfo = info
	}
	for index := range loaded.Diagnostics {
		loaded.Diagnostics[index].Source = metadata
	}
	return loaded
}

func skillSourceMetadata(path, scope string) ProtocolSourceInfo {
	return ProtocolSourceInfo{Path: path, Source: "local", Scope: scope, Origin: "top-level"}
}

func loadResourcePromptsWithMetadata(path string, metadata ProtocolSourceInfo) []PromptTemplate {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	sourceInfo := sourceInfoFromProtocolMetadata(metadata)
	if info.IsDir() {
		return loadPromptTemplatesFromDir(path, func(filePath string) SourceInfo {
			fileInfo := sourceInfo
			fileInfo.Path = filePath
			return fileInfo
		})
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	sourceInfo.Path = path
	template, ok := loadPromptTemplateFromFile(path, sourceInfo)
	if !ok {
		return nil
	}
	return []PromptTemplate{template}
}

func sourceInfoFromProtocolMetadata(metadata ProtocolSourceInfo) SourceInfo {
	return SourceInfo{
		Path:   metadata.Path,
		Source: metadata.Source,
		Scope:  metadata.Scope,
		Origin: metadata.Origin,
	}
}

func (l *DefaultResourceLoader) loadThemes() ResourceThemesResult {
	var themes []ResourceTheme
	var diagnostics []agentharness.SkillDiagnostic
	if !l.noThemes {
		for _, resource := range l.packageResources.Themes {
			if resource.Enabled {
				themes = append(themes, loadThemeFile(resource.Path)...)
			}
		}
		dirs := []string{filepath.Join(l.agentDir, "themes")}
		if l.projectTrusted() {
			dirs = append(dirs, filepath.Join(l.cwd, ConfigDirName, "themes"))
		}
		for _, dir := range dirs {
			themes = append(themes, loadThemesFromDir(dir)...)
		}
	}
	for _, path := range l.additionalThemePaths {
		resolved := ResolveToCwd(path, l.cwd)
		info, err := os.Stat(resolved)
		if err != nil {
			diagnostics = append(diagnostics, agentharness.SkillDiagnostic{
				Type:    "warning",
				Message: "theme path does not exist",
				Path:    resolved,
			})
		} else if info.IsDir() {
			themes = append(themes, loadThemesFromDir(resolved)...)
		} else if info.Mode().IsRegular() && strings.HasSuffix(resolved, ".json") {
			themes = append(themes, loadThemeFile(resolved)...)
		} else {
			diagnostics = append(diagnostics, agentharness.SkillDiagnostic{
				Type:    "warning",
				Message: "theme path is not a json file",
				Path:    resolved,
			})
		}
	}
	for _, path := range l.extendedThemes {
		themes = append(themes, loadThemeResourcePath(path, l.cwd)...)
	}
	for _, path := range l.extensionThemes {
		themes = append(themes, loadThemeResourcePath(path, l.cwd)...)
	}
	for _, path := range l.runtimeThemes {
		themes = append(themes, loadThemeResourcePath(path, l.cwd)...)
	}
	themes = filterThemes(themes, l.resourceFilters("themes"), l.cwd, l.agentDir)
	return ResourceThemesResult{Themes: dedupeThemesByName(themes), Diagnostics: diagnostics}
}

func loadThemeResourcePath(path ResourceThemePath, cwd string) []ResourceTheme {
	resolved := ResolveToCwd(path.Path, cwd)
	if info, err := os.Stat(resolved); err == nil && info.IsDir() {
		return loadThemesFromDir(resolved)
	}
	return loadThemeFile(resolved)
}

func (l *DefaultResourceLoader) loadPackageResources() (ProtocolPackageResources, error) {
	manager := NewDefaultPackageManager(PackageManagerOptions{
		CWD:             l.cwd,
		AgentDir:        l.agentDir,
		SettingsManager: l.settingsManager,
	})
	return manager.ResolveConfiguredProtocolPackageResources()
}

func (l *DefaultResourceLoader) loadAgentsFiles() []ResourceContextFile {
	if l.noContextFiles {
		return nil
	}
	return loadProjectContextResourceFiles(l.cwd, l.agentDir)
}

func loadProjectContextResourceFiles(cwd, agentDir string) []ResourceContextFile {
	var files []ResourceContextFile
	seen := map[string]struct{}{}
	add := func(file *ResourceContextFile) {
		if file == nil {
			return
		}
		clean := filepath.Clean(file.Path)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		files = append(files, *file)
	}
	add(loadContextFileFromDir(agentDir))

	var ancestors []ResourceContextFile
	current := filepath.Clean(cwd)
	for {
		if file := loadContextFileFromDir(current); file != nil {
			ancestors = append([]ResourceContextFile{*file}, ancestors...)
		}
		if isFilesystemRoot(current) {
			break
		}
		next := filepath.Dir(current)
		if next == current {
			break
		}
		current = next
	}
	for i := range ancestors {
		add(&ancestors[i])
	}
	return files
}

func loadContextFileFromDir(dir string) *ResourceContextFile {
	if dir == "" {
		return nil
	}
	for _, name := range []string{"AGENTS.md", "AGENTS.MD", "CLAUDE.md", "CLAUDE.MD"} {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		return &ResourceContextFile{Path: path, Content: string(content)}
	}
	return nil
}

func (l *DefaultResourceLoader) loadSystemPrompt() string {
	if l.systemPromptOverride != nil {
		return l.systemPromptOverride()
	}
	if strings.TrimSpace(l.systemPromptSource) != "" {
		return resolvePromptInput(l.systemPromptSource)
	}
	if l.projectTrusted() {
		if projectPrompt := strings.TrimSpace(readOptionalFile(filepath.Join(l.cwd, ConfigDirName, "SYSTEM.md"))); projectPrompt != "" {
			return projectPrompt
		}
	}
	return strings.TrimSpace(readOptionalFile(filepath.Join(l.agentDir, "SYSTEM.md")))
}

func (l *DefaultResourceLoader) loadAppendSystemPrompt() string {
	if len(l.appendSystemSources) > 0 {
		var parts []string
		for _, source := range l.appendSystemSources {
			if resolved := strings.TrimSpace(resolvePromptInput(source)); resolved != "" {
				parts = append(parts, resolved)
			}
		}
		return strings.Join(parts, "\n\n")
	}
	if l.projectTrusted() {
		if projectPrompt := readOptionalFile(filepath.Join(l.cwd, ConfigDirName, "APPEND_SYSTEM.md")); projectPrompt != "" {
			return projectPrompt
		}
	}
	return readOptionalFile(filepath.Join(l.agentDir, "APPEND_SYSTEM.md"))
}

func (l *DefaultResourceLoader) resourceFilters(key string) []string {
	return settingsStringSlice(l.settingsManager.mergedSnapshot(), key)
}

func (l *DefaultResourceLoader) settingsExtensionSources() []ProtocolExtensionSource {
	var sources []ProtocolExtensionSource
	for _, path := range l.settingsResourcePathsByScope("extensions", l.settingsManager.GetGlobalSettings(), l.agentDir) {
		loaded := LoadProtocolExtensionSources([]string{path}, l.agentDir)
		sources = append(sources, loaded.Extensions...)
	}
	projectBase := filepath.Join(l.cwd, ConfigDirName)
	if l.projectTrusted() {
		for _, path := range l.settingsResourcePathsByScope("extensions", l.settingsManager.GetProjectSettings(), projectBase) {
			loaded := LoadProtocolExtensionSources([]string{path}, projectBase)
			sources = append(sources, loaded.Extensions...)
		}
	}
	return sources
}

func (l *DefaultResourceLoader) settingsResourcePaths(key string) []string {
	var paths []string
	paths = append(paths, l.settingsResourcePathsByScope(key, l.settingsManager.GetGlobalSettings(), l.agentDir)...)
	if l.projectTrusted() {
		paths = append(paths, l.settingsResourcePathsByScope(key, l.settingsManager.GetProjectSettings(), filepath.Join(l.cwd, ConfigDirName))...)
	}
	return paths
}

func (l *DefaultResourceLoader) projectTrusted() bool {
	return l == nil || l.settingsManager == nil || l.settingsManager.IsProjectTrusted()
}

func (l *DefaultResourceLoader) settingsResourcePathsByScope(key string, settings map[string]any, baseDir string) []string {
	var paths []string
	for _, entry := range settingsStringSlice(settings, key) {
		entry = strings.TrimSpace(entry)
		if entry == "" || strings.HasPrefix(entry, "!") || strings.HasPrefix(entry, "-") || strings.HasPrefix(entry, "+") {
			continue
		}
		paths = append(paths, ResolveToCwd(entry, baseDir))
	}
	return paths
}

func filterProtocolExtensions(extensions []ProtocolExtensionSource, filters []string, cwd, agentDir string) []ProtocolExtensionSource {
	var result []ProtocolExtensionSource
	for _, extension := range extensions {
		if resourceEnabled(extension.Path, filters, cwd, agentDir) {
			result = append(result, extension)
		}
	}
	return result
}

func filterSkills(skills []agentharness.Skill, filters []string, cwd, agentDir string) []agentharness.Skill {
	var result []agentharness.Skill
	for _, skill := range skills {
		if resourceEnabled(skill.FilePath, filters, cwd, agentDir) {
			result = append(result, skill)
		}
	}
	return result
}

func filterPrompts(prompts []PromptTemplate, filters []string, cwd, agentDir string) []PromptTemplate {
	var result []PromptTemplate
	for _, prompt := range prompts {
		if resourceEnabled(prompt.FilePath, filters, cwd, agentDir) {
			result = append(result, prompt)
		}
	}
	return result
}

func filterThemes(themes []ResourceTheme, filters []string, cwd, agentDir string) []ResourceTheme {
	var result []ResourceTheme
	for _, theme := range themes {
		if resourceEnabled(theme.SourcePath, filters, cwd, agentDir) {
			result = append(result, theme)
		}
	}
	return result
}

func resourceEnabled(path string, filters []string, cwd, agentDir string) bool {
	enabled := true
	for _, filter := range filters {
		filter = strings.TrimSpace(filter)
		if filter == "" {
			continue
		}
		action := filter[0]
		if action != '-' && action != '!' && action != '+' {
			continue
		}
		pattern := strings.TrimSpace(filter[1:])
		for _, base := range []string{cwd, filepath.Join(cwd, ConfigDirName), agentDir} {
			if resourceMatchesFilter(path, pattern, base) {
				enabled = action == '+'
				break
			}
		}
	}
	return enabled
}

func resourceMatchesFilter(path, pattern, base string) bool {
	if base == "" || pattern == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(path))
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return false
	}
	rel = filepath.ToSlash(rel)
	pattern = filepath.ToSlash(filepath.Clean(pattern))
	return rel == pattern || strings.HasPrefix(rel, strings.TrimSuffix(pattern, "/")+"/") || protocolPackagePatternMatches(base, path, pattern)
}

func dedupeSkillsByName(skills []agentharness.Skill) []agentharness.Skill {
	byName := map[string]agentharness.Skill{}
	var order []string
	for _, skill := range skills {
		if _, ok := byName[skill.Name]; !ok {
			order = append(order, skill.Name)
		}
		byName[skill.Name] = skill
	}
	result := make([]agentharness.Skill, 0, len(order))
	for _, name := range order {
		result = append(result, byName[name])
	}
	return result
}

func skillCollisionDiagnostics(skills []agentharness.Skill) []agentharness.SkillDiagnostic {
	lastByName := map[string]agentharness.Skill{}
	countByName := map[string]int{}
	for _, skill := range skills {
		if skill.Name == "" {
			continue
		}
		countByName[skill.Name]++
		lastByName[skill.Name] = skill
	}
	var diagnostics []agentharness.SkillDiagnostic
	reportedLoser := map[string]bool{}
	for _, skill := range skills {
		if countByName[skill.Name] <= 1 {
			continue
		}
		winner := lastByName[skill.Name]
		if skill.FilePath == winner.FilePath {
			continue
		}
		key := skill.Name + "\x00" + skill.FilePath
		if reportedLoser[key] {
			continue
		}
		reportedLoser[key] = true
		diagnostics = append(diagnostics, agentharness.SkillDiagnostic{
			Type:    "collision",
			Code:    "skill_collision",
			Message: `skill "` + skill.Name + `" from ` + skill.FilePath + ` was overridden by ` + winner.FilePath,
			Path:    skill.FilePath,
			Source:  skill.SourceInfo,
		})
	}
	return diagnostics
}

func dedupePromptsByName(prompts []PromptTemplate) []PromptTemplate {
	byName := map[string]PromptTemplate{}
	var order []string
	for _, prompt := range prompts {
		if _, ok := byName[prompt.Name]; !ok {
			order = append(order, prompt.Name)
		}
		byName[prompt.Name] = prompt
	}
	result := make([]PromptTemplate, 0, len(order))
	for _, name := range order {
		result = append(result, byName[name])
	}
	return result
}

func dedupeThemesByName(themes []ResourceTheme) []ResourceTheme {
	byName := map[string]ResourceTheme{}
	var order []string
	for _, theme := range themes {
		if theme.Name == "" {
			continue
		}
		if _, ok := byName[theme.Name]; !ok {
			order = append(order, theme.Name)
		}
		byName[theme.Name] = theme
	}
	result := make([]ResourceTheme, 0, len(order))
	for _, name := range order {
		result = append(result, byName[name])
	}
	return result
}

func loadThemesFromDir(dir string) []ResourceTheme {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var themes []ResourceTheme
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var value struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(content, &value); err != nil || strings.TrimSpace(value.Name) == "" {
			continue
		}
		themes = append(themes, ResourceTheme{Name: value.Name, SourcePath: path})
	}
	return themes
}

func loadThemeFile(path string) []ResourceTheme {
	if !strings.HasSuffix(path, ".json") {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var value struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(content, &value); err != nil || strings.TrimSpace(value.Name) == "" {
		return nil
	}
	return []ResourceTheme{{Name: value.Name, SourcePath: path}}
}

func readOptionalFile(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}

func resolvePromptInput(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if content, err := os.ReadFile(input); err == nil {
		return string(content)
	}
	return input
}
