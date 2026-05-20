package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
)

type DefaultResourceLoaderOptions struct {
	CWD                      string
	AgentDir                 string
	SettingsManager          *SettingsManager
	NoContextFiles           bool
	NoSkills                 bool
	AdditionalExtensionPaths []string
	AdditionalSkillPaths     []string
	AdditionalPromptPaths    []string
	SkillsOverride           func() ResourceSkillsResult
	SystemPromptOverride     func() string
}

type ResourceExtensionsResult struct {
	Extensions []ProtocolExtensionSource
	Errors     []ProtocolExtensionDiscoveryError
	Runtime    *ProtocolExtensionRuntime
}

type ResourceSkillsResult struct {
	Skills      []agentharness.Skill
	Diagnostics []agentharness.SkillDiagnostic
}

type ResourcePromptsResult struct {
	Prompts []PromptTemplate
}

type ResourceTheme struct {
	Name       string
	SourcePath string
}

type ResourceThemesResult struct {
	Themes []ResourceTheme
}

type ResourceContextFile struct {
	Path string
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

type ResourceExtension struct {
	ExtensionPaths []ResourceExtensionPath
	SkillPaths     []ResourceSkillPath
	PromptPaths    []ResourcePromptPath
}

type DefaultResourceLoader struct {
	cwd                      string
	agentDir                 string
	settingsManager          *SettingsManager
	noContextFiles           bool
	noSkills                 bool
	additionalExtensionPaths []string
	additionalSkillPaths     []string
	additionalPromptPaths    []string
	skillsOverride           func() ResourceSkillsResult
	systemPromptOverride     func() string

	extensions       ResourceExtensionsResult
	skills           ResourceSkillsResult
	prompts          ResourcePromptsResult
	themes           ResourceThemesResult
	agentsFiles      ResourceAgentsFilesResult
	systemPrompt     string
	appendSystem     string
	extensionSkills  []ResourceSkillPath
	extensionPrompts []ResourcePromptPath
	packageResources ProtocolPackageResources
	packageLoadError error
	extendedSkills   []ResourceSkillPath
	extendedPrompts  []ResourcePromptPath
	extendedExtPaths []ResourceExtensionPath
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
		noContextFiles:           options.NoContextFiles,
		noSkills:                 options.NoSkills,
		additionalExtensionPaths: append([]string(nil), options.AdditionalExtensionPaths...),
		additionalSkillPaths:     append([]string(nil), options.AdditionalSkillPaths...),
		additionalPromptPaths:    append([]string(nil), options.AdditionalPromptPaths...),
		skillsOverride:           options.SkillsOverride,
		systemPromptOverride:     options.SystemPromptOverride,
		extensions:               ResourceExtensionsResult{Runtime: NewDefaultProtocolExtensionRuntime()},
	}
}

func (l *DefaultResourceLoader) Reload() {
	l.packageResources, l.packageLoadError = l.loadPackageResources()
	l.extensions = l.loadExtensions()
	l.skills = l.loadSkills()
	l.prompts = ResourcePromptsResult{Prompts: l.loadPrompts()}
	l.themes = ResourceThemesResult{Themes: l.loadThemes()}
	l.agentsFiles = ResourceAgentsFilesResult{AgentsFiles: l.loadAgentsFiles()}
	l.systemPrompt = l.loadSystemPrompt()
	l.appendSystem = l.loadAppendSystemPrompt()
}

func (l *DefaultResourceLoader) ExtendResources(resources ResourceExtension) {
	l.extendedExtPaths = append(l.extendedExtPaths, resources.ExtensionPaths...)
	l.extendedSkills = append(l.extendedSkills, resources.SkillPaths...)
	l.extendedPrompts = append(l.extendedPrompts, resources.PromptPaths...)
	l.extensions = l.loadExtensions()
	l.skills = l.loadSkills()
	l.prompts = ResourcePromptsResult{Prompts: l.loadPrompts()}
}

func (l *DefaultResourceLoader) GetExtensions() ResourceExtensionsResult {
	return l.extensions
}

func (l *DefaultResourceLoader) GetSkills() ResourceSkillsResult {
	return l.skills
}

func (l *DefaultResourceLoader) GetPrompts() ResourcePromptsResult {
	return l.prompts
}

func (l *DefaultResourceLoader) GetThemes() ResourceThemesResult {
	return l.themes
}

func (l *DefaultResourceLoader) GetAgentsFiles() ResourceAgentsFilesResult {
	return l.agentsFiles
}

func (l *DefaultResourceLoader) GetSystemPrompt() string {
	return l.systemPrompt
}

func (l *DefaultResourceLoader) GetAppendSystemPrompt() string {
	return l.appendSystem
}

func (l *DefaultResourceLoader) loadExtensions() ResourceExtensionsResult {
	var combined ProtocolExtensionDiscoveryResult
	for _, source := range l.extendedExtPaths {
		combined.Extensions = append(combined.Extensions, ProtocolExtensionSource{Path: source.Path, BaseDir: filepath.Dir(source.Path), Metadata: source.Metadata})
	}
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
	explicit := LoadProtocolExtensionSources(l.additionalExtensionPaths, l.cwd)
	combined.Extensions = append(combined.Extensions, explicit.Extensions...)
	combined.Errors = append(combined.Errors, explicit.Errors...)
	for _, dir := range []string{filepath.Join(l.cwd, ConfigDirName, "extensions"), filepath.Join(l.agentDir, "extensions")} {
		discovered := discoverProtocolExtensionsInDir(dir)
		combined.Extensions = append(combined.Extensions, discovered.Extensions...)
		combined.Errors = append(combined.Errors, discovered.Errors...)
	}
	filtered := filterProtocolExtensions(combined.Extensions, l.resourceFilters("extensions"), l.cwd, l.agentDir)
	extensions := dedupeProtocolExtensionSources(filtered)
	runtime := NewDefaultProtocolExtensionRuntime()
	loaded := LoadProtocolExtensionDescriptors(extensions, runtime)
	l.extensionSkills = loaded.Resources.SkillPaths
	l.extensionPrompts = loaded.Resources.PromptPaths
	return ResourceExtensionsResult{
		Extensions: extensions,
		Errors:     append(combined.Errors, loaded.Errors...),
		Runtime:    runtime,
	}
}

func (l *DefaultResourceLoader) loadSkills() ResourceSkillsResult {
	if l.skillsOverride != nil {
		return l.skillsOverride()
	}
	var result ResourceSkillsResult
	for _, resource := range l.packageResources.Skills {
		if !resource.Enabled {
			continue
		}
		loaded := loadResourceSkillsWithMetadata(resource.Path, resource.Metadata)
		result.Skills = append(result.Skills, loaded.Skills...)
		result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
	}
	if !l.noSkills {
		for _, dir := range l.settingsResourcePaths("skills") {
			loaded := agentharness.LoadSkills(dir)
			result.Skills = append(result.Skills, loaded.Skills...)
			result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
		}
		for _, dir := range []string{filepath.Join(l.agentDir, "skills")} {
			loaded := agentharness.LoadSkills(dir)
			result.Skills = append(result.Skills, loaded.Skills...)
			result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
		}
		userAgentsDir := userAgentsDirFromAgentDir(l.agentDir)
		if userAgentsDir != "" {
			loaded := agentharness.LoadSkillsWithOptions(agentharness.LoadSkillsOptions{RespectGitignore: true}, filepath.Join(userAgentsDir, "skills"))
			result.Skills = append(result.Skills, loaded.Skills...)
			result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
		}
		for _, dir := range projectAgentsSkillDirs(l.cwd, userAgentsDir) {
			loaded := agentharness.LoadSkillsWithOptions(agentharness.LoadSkillsOptions{RespectGitignore: true}, dir)
			result.Skills = append(result.Skills, loaded.Skills...)
			result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
		}
		loaded := agentharness.LoadSkills(filepath.Join(l.cwd, ConfigDirName, "skills"))
		result.Skills = append(result.Skills, loaded.Skills...)
		result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
	}
	for _, path := range l.additionalSkillPaths {
		loaded := agentharness.LoadSkills(ResolveToCwd(path, l.cwd))
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
	result.Skills = filterSkills(result.Skills, l.resourceFilters("skills"), l.cwd, l.agentDir)
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
		return ""
	}
	return filepath.Join(filepath.Dir(configDir), ".agents")
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
	for _, resource := range l.packageResources.Prompts {
		if resource.Enabled {
			prompts = append(prompts, loadResourcePromptsWithMetadata(resource.Path, resource.Metadata)...)
		}
	}
	for _, path := range l.settingsResourcePaths("prompts") {
		prompts = append(prompts, LoadPromptTemplates(LoadPromptTemplatesOptions{Cwd: l.cwd, PromptPaths: []string{path}})...)
	}
	prompts = append(prompts, loadPromptTemplatesFromDir(filepath.Join(l.agentDir, "prompts"), sourceInfoForPromptPath(filepath.Join(l.agentDir, "prompts"), filepath.Join(l.agentDir, "prompts"), filepath.Join(l.cwd, ConfigDirName, "prompts")))...)
	prompts = append(prompts, loadPromptTemplatesFromDir(filepath.Join(l.cwd, ConfigDirName, "prompts"), sourceInfoForPromptPath(filepath.Join(l.cwd, ConfigDirName, "prompts"), filepath.Join(l.agentDir, "prompts"), filepath.Join(l.cwd, ConfigDirName, "prompts")))...)
	for _, path := range l.additionalPromptPaths {
		prompts = append(prompts, LoadPromptTemplates(LoadPromptTemplatesOptions{Cwd: l.cwd, PromptPaths: []string{path}})...)
	}
	for _, path := range l.extendedPrompts {
		prompts = append(prompts, loadResourcePromptsWithMetadata(ResolveToCwd(path.Path, l.cwd), path.Metadata)...)
	}
	for _, path := range l.extensionPrompts {
		prompts = append(prompts, loadResourcePromptsWithMetadata(ResolveToCwd(path.Path, l.cwd), path.Metadata)...)
	}
	prompts = filterPrompts(prompts, l.resourceFilters("prompts"), l.cwd, l.agentDir)
	return dedupePromptsByName(prompts)
}

func loadResourceSkillsWithMetadata(path string, metadata ProtocolSourceInfo) agentharness.SkillResult {
	loadPath := path
	if filepath.Base(path) == "SKILL.md" {
		loadPath = filepath.Dir(path)
	}
	loaded := agentharness.LoadSkills(loadPath)
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

func (l *DefaultResourceLoader) loadThemes() []ResourceTheme {
	var themes []ResourceTheme
	for _, resource := range l.packageResources.Themes {
		if resource.Enabled {
			themes = append(themes, loadThemeFile(resource.Path)...)
		}
	}
	for _, dir := range []string{filepath.Join(l.agentDir, "themes"), filepath.Join(l.cwd, ConfigDirName, "themes")} {
		themes = append(themes, loadThemesFromDir(dir)...)
	}
	themes = filterThemes(themes, l.resourceFilters("themes"), l.cwd, l.agentDir)
	return dedupeThemesByName(themes)
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
	var files []ResourceContextFile
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		path := filepath.Join(l.cwd, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			files = append(files, ResourceContextFile{Path: path})
		}
	}
	return files
}

func (l *DefaultResourceLoader) loadSystemPrompt() string {
	if l.systemPromptOverride != nil {
		return l.systemPromptOverride()
	}
	return strings.TrimSpace(readOptionalFile(filepath.Join(l.cwd, ConfigDirName, "SYSTEM.md")))
}

func (l *DefaultResourceLoader) loadAppendSystemPrompt() string {
	return readOptionalFile(filepath.Join(l.cwd, ConfigDirName, "APPEND_SYSTEM.md"))
}

func (l *DefaultResourceLoader) resourceFilters(key string) []string {
	return settingsStringSlice(l.settingsManager.merged, key)
}

func (l *DefaultResourceLoader) settingsExtensionSources() []ProtocolExtensionSource {
	var sources []ProtocolExtensionSource
	for _, path := range l.settingsResourcePathsByScope("extensions", l.settingsManager.global, l.agentDir) {
		loaded := LoadProtocolExtensionSources([]string{path}, l.agentDir)
		sources = append(sources, loaded.Extensions...)
	}
	projectBase := filepath.Join(l.cwd, ConfigDirName)
	for _, path := range l.settingsResourcePathsByScope("extensions", l.settingsManager.project, projectBase) {
		loaded := LoadProtocolExtensionSources([]string{path}, projectBase)
		sources = append(sources, loaded.Extensions...)
	}
	return sources
}

func (l *DefaultResourceLoader) settingsResourcePaths(key string) []string {
	var paths []string
	paths = append(paths, l.settingsResourcePathsByScope(key, l.settingsManager.global, l.agentDir)...)
	paths = append(paths, l.settingsResourcePathsByScope(key, l.settingsManager.project, filepath.Join(l.cwd, ConfigDirName))...)
	return paths
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
