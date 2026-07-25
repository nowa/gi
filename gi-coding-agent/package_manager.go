package gicodingagent

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type PackageManagerOptions struct {
	CWD             string
	AgentDir        string
	SettingsManager *SettingsManager
	Operations      PackageManagerOperations
	Progress        func(PackageProgressEvent)
}

type PackageManagerOperations struct {
	RunCommand        func(command string, args []string, options PackageCommandOptions) error
	RunCommandCapture func(command string, args []string, options PackageCommandOptions) (string, error)
}

type PackageCommandOptions struct {
	CWD string
}

type PackageProgressEvent struct {
	Type   string
	Action string
	Source string
	Error  string
}

type ResolveExtensionSourcesOptions struct {
	Temporary bool
}

type VersionReleaseChecker func(currentVersion string, options VersionCheckOptions) (LatestGiRelease, bool)

type SelfUpdateOptions struct {
	PackageName         string
	CurrentVersion      string
	Force               bool
	Environment         InstallEnvironment
	VersionCheck        VersionReleaseChecker
	VersionCheckOptions VersionCheckOptions
}

type SelfUpdateResult struct {
	Updated     bool
	PackageName string
}

type ConfiguredPackage struct {
	Source        string
	Scope         string
	Filtered      bool
	InstalledPath string
}

type PackageUpdate struct {
	Source      string
	DisplayName string
	Type        string
	Scope       string
}

type PackageResourceToggle struct {
	Source       string
	Scope        string
	ResourceType string
	Pattern      string
	Enabled      bool
}

type TopLevelResourceToggle struct {
	Scope        string
	ResourceType string
	Pattern      string
	Enabled      bool
}

type PackageResourceToggleItem struct {
	Source       string
	Scope        string
	ResourceType string
	Pattern      string
	Path         string
	DisplayName  string
	Enabled      bool
	Metadata     ProtocolSourceInfo
}

type PackageUpdateSuggestionError struct {
	Input      string
	Suggestion string
}

func (e PackageUpdateSuggestionError) Error() string {
	return "No matching package found for " + e.Input + ". Did you mean " + e.Suggestion + "?"
}

type PackageSource struct {
	Type   string
	Source string
	Repo   string
	Host   string
	Path   string
	Ref    string
	Pinned bool
}

type DefaultPackageManager struct {
	cwd             string
	agentDir        string
	settingsManager *SettingsManager
	operations      PackageManagerOperations
	progress        func(PackageProgressEvent)
}

func NewDefaultPackageManager(options PackageManagerOptions) *DefaultPackageManager {
	operations := normalizePackageManagerOperations(options.Operations)
	settingsManager := options.SettingsManager
	if settingsManager == nil {
		settingsManager = NewSettingsManager(options.CWD, options.AgentDir)
	}
	return &DefaultPackageManager{
		cwd:             options.CWD,
		agentDir:        options.AgentDir,
		settingsManager: settingsManager,
		operations:      operations,
		progress:        options.Progress,
	}
}

func (m *DefaultPackageManager) ParseSource(source string) PackageSource {
	return ParsePackageSource(source)
}

func (m *DefaultPackageManager) GetPackageIdentity(source string) string {
	return PackageSourceIdentity(ParsePackageSource(source))
}

func (m *DefaultPackageManager) Install(source string, project bool) error {
	source = strings.TrimSpace(source)
	m.emitProgress(PackageProgressEvent{Type: "start", Action: "install", Source: source})
	if err := m.assertProjectTrustedForScope(projectPackageScope(project)); err != nil {
		m.emitProgress(PackageProgressEvent{Type: "error", Action: "install", Source: source, Error: err.Error()})
		return err
	}
	if err := m.installPackageArtifact(source, project); err != nil {
		m.emitProgress(PackageProgressEvent{Type: "error", Action: "install", Source: source, Error: err.Error()})
		return err
	}
	_, err := m.addSourceToSettings(source, project)
	if err != nil {
		m.emitProgress(PackageProgressEvent{Type: "error", Action: "install", Source: source, Error: err.Error()})
		return err
	}
	m.emitProgress(PackageProgressEvent{Type: "done", Action: "install", Source: source})
	return err
}

func (m *DefaultPackageManager) installPackageArtifact(source string, project bool) error {
	if source == "" {
		return fmt.Errorf("missing install source")
	}
	if unsupportedPackageSource(source) {
		return unsupportedPackageSourceError(source)
	}
	parsed := ParsePackageSource(source)
	switch parsed.Type {
	case "official":
		_, err := m.materializeOfficialPackage(parsed.Path)
		return err
	case "git":
		return m.installGitPackage(GitSource{Repo: parsed.Repo, Host: parsed.Host, Path: parsed.Path, Ref: parsed.Ref, Pinned: parsed.Pinned}, project)
	case "local":
		resolved := ResolveToCwd(parsed.Path, m.cwd)
		if _, err := os.Stat(resolved); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("path does not exist: %s", resolved)
			}
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported install source: %s", source)
	}
}

func (m *DefaultPackageManager) addSourceToSettings(source string, project bool) (bool, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return false, fmt.Errorf("missing install source")
	}
	if unsupportedPackageSource(source) {
		return false, unsupportedPackageSourceError(source)
	}
	parsed := ParsePackageSource(source)
	if parsed.Type == "official" && !isOfficialPackageName(parsed.Path) {
		return false, fmt.Errorf("unknown official package %q", parsed.Path)
	}
	baseDir := m.settingsBaseDir(project)
	stored := m.packageSettingsValue(source, baseDir)
	packages := m.settingsPackages(project)
	if !packageSettingsContains(packages, stored, source, m.cwd, baseDir) {
		packages = append(packages, stored)
	} else {
		return false, nil
	}
	values := make([]any, len(packages))
	for i, value := range packages {
		values[i] = value
	}
	if project {
		if err := m.settingsManager.SetProjectPackages(values); err != nil {
			return false, err
		}
	} else {
		m.settingsManager.SetPackages(values)
	}
	return true, nil
}

func (m *DefaultPackageManager) Remove(source string, project bool) error {
	source = strings.TrimSpace(source)
	m.emitProgress(PackageProgressEvent{Type: "start", Action: "remove", Source: source})
	if err := m.assertProjectTrustedForScope(projectPackageScope(project)); err != nil {
		m.emitProgress(PackageProgressEvent{Type: "error", Action: "remove", Source: source, Error: err.Error()})
		return err
	}
	if err := m.removePackageArtifact(source, project); err != nil {
		m.emitProgress(PackageProgressEvent{Type: "error", Action: "remove", Source: source, Error: err.Error()})
		return err
	}
	m.emitProgress(PackageProgressEvent{Type: "done", Action: "remove", Source: source})
	return nil
}

func (m *DefaultPackageManager) RemoveAndPersist(source string, project bool) (bool, error) {
	if err := m.Remove(source, project); err != nil {
		return false, err
	}
	return m.removeSourceFromSettings(source, project)
}

func (m *DefaultPackageManager) removePackageArtifact(source string, project bool) error {
	source = strings.TrimSpace(source)
	if source == "" {
		return fmt.Errorf("missing remove source")
	}
	if unsupportedPackageSource(source) {
		return unsupportedPackageSourceError(source)
	}
	parsed := ParsePackageSource(source)
	switch parsed.Type {
	case "git":
		return os.RemoveAll(m.gitPackageInstallPath(GitSource{Host: parsed.Host, Path: parsed.Path}, project))
	case "official":
		return os.RemoveAll(filepath.Join(officialPackageStoreDir(m.agentDir, m.cwd), parsed.Path))
	case "local":
		return nil
	default:
		return fmt.Errorf("unsupported remove source: %s", source)
	}
}

func (m *DefaultPackageManager) removeSourceFromSettings(source string, project bool) (bool, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return false, fmt.Errorf("missing remove source")
	}
	baseDir := m.settingsBaseDir(project)
	packages := m.settingsPackages(project)
	filtered := make([]string, 0, len(packages))
	removed := false
	for _, existing := range packages {
		if packageSettingsMatch(existing, source, m.cwd, baseDir) {
			removed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if !removed {
		return false, nil
	}
	values := make([]any, len(filtered))
	for i, value := range filtered {
		values[i] = value
	}
	if project {
		if err := m.settingsManager.SetProjectPackages(values); err != nil {
			return false, err
		}
	} else {
		m.settingsManager.SetPackages(values)
	}
	return true, nil
}

func (m *DefaultPackageManager) settingsPackages(project bool) []string {
	if project && !m.settingsManager.IsProjectTrusted() {
		return nil
	}
	settings := m.settingsManager.GetGlobalSettings()
	if project {
		settings = m.settingsManager.GetProjectSettings()
	}
	return settingsPackagesToStrings(settingsSlice(settings, "packages"))
}

func (m *DefaultPackageManager) packageUpdateSuggestion(source string) string {
	source = strings.TrimSpace(source)
	if source == "" || strings.Contains(source, ":") {
		return ""
	}
	for _, existing := range settingsPackagesToStrings(m.settingsManager.GetPackages()) {
		parsed := ParsePackageSource(existing)
		if parsed.Type == "git" && parsed.Host+"/"+parsed.Path == source {
			return existing
		}
	}
	return ""
}

func (m *DefaultPackageManager) ListConfiguredPackages() []ConfiguredPackage {
	if m == nil || m.settingsManager == nil {
		return nil
	}
	var result []ConfiguredPackage
	add := func(values []any, scope string) {
		for _, value := range values {
			spec, ok := protocolPackageSourceSpecFromSettings(value, scope)
			if !ok {
				continue
			}
			result = append(result, ConfiguredPackage{
				Source:        spec.Source,
				Scope:         scope,
				Filtered:      packageSettingHasFilters(value),
				InstalledPath: m.GetInstalledPath(spec.Source, scope),
			})
		}
	}
	add(settingsSlice(m.settingsManager.GetGlobalSettings(), "packages"), "user")
	if m.settingsManager.IsProjectTrusted() {
		add(settingsSlice(m.settingsManager.GetProjectSettings(), "packages"), "project")
	}
	return result
}

func (m *DefaultPackageManager) ListPackageResourceToggles() ([]PackageResourceToggleItem, error) {
	if m == nil || m.settingsManager == nil {
		return nil, nil
	}
	var result []PackageResourceToggleItem
	specs := m.configuredProtocolPackageSourceSpecs()
	for _, spec := range specs {
		resolvedSource := spec.Source
		resolvedScope := spec.Scope
		if base, ok := m.findAutoloadDeltaBase(spec, specs); ok {
			resolvedSource = base.Source
			resolvedScope = base.Scope
		}
		resolved, packageDir, err := m.resolveProtocolPackageSource(resolvedSource, resolvedScope)
		if err != nil {
			return nil, err
		}
		if packageDir == "" {
			continue
		}
		addResources := func(resourceType string, resources []ProtocolPackageResource) {
			filters, configured := spec.Filters.forResourceType(resourceType)
			var filtered []ProtocolPackageResource
			if spec.autoloadEnabled() {
				filtered = applyProtocolPackageFiltersConfigured(packageDir, resources, filters, configured)
			} else {
				filtered = applyProtocolPackageDeltaFilters(packageDir, resources, filters)
			}
			for _, resource := range protocolPackageResourcesWithSpecMetadata(filtered, spec) {
				pattern := packageResourceTogglePattern(packageDir, resource.Path)
				if pattern == "" {
					continue
				}
				result = append(result, PackageResourceToggleItem{
					Source:       spec.Source,
					Scope:        firstNonEmptyString(spec.Scope, "user"),
					ResourceType: resourceType,
					Pattern:      pattern,
					Path:         resource.Path,
					DisplayName:  packageResourceDisplayName(resourceType, resource.Path),
					Enabled:      resource.Enabled,
					Metadata:     resource.Metadata,
				})
			}
		}
		addResources("extensions", resolved.Extensions)
		addResources("skills", resolved.Skills)
		addResources("prompts", resolved.Prompts)
		addResources("themes", resolved.Themes)
		extensionFilters, extensionsConfigured := spec.Filters.forResourceType("extensions")
		for _, process := range resolved.ProcessExtensions {
			enabled := protocolPackageProcessEnabledConfigured(packageDir, process, extensionFilters, extensionsConfigured)
			if !spec.autoloadEnabled() {
				var matched bool
				enabled, matched = protocolPackageDeltaProcessState(packageDir, process, extensionFilters)
				if !matched {
					continue
				}
			}
			pattern := strings.TrimSpace(process.ID)
			if pattern == "" {
				pattern = packageResourceTogglePattern(packageDir, process.Path)
			}
			if pattern == "" {
				continue
			}
			displayName := strings.TrimSpace(process.ID)
			if displayName == "" {
				displayName = packageResourceDisplayName("extensions", process.Path)
			}
			result = append(result, PackageResourceToggleItem{
				Source:       spec.Source,
				Scope:        firstNonEmptyString(spec.Scope, "user"),
				ResourceType: "extensions",
				Pattern:      pattern,
				Path:         process.Path,
				DisplayName:  displayName,
				Enabled:      enabled,
				Metadata:     protocolPackageProcessWithSpecMetadata(process, spec).Metadata,
			})
		}
	}
	sortPackageResourceToggleItems(result)
	return dedupePackageResourceToggleItems(result), nil
}

func (m *DefaultPackageManager) ListResourceToggles() ([]PackageResourceToggleItem, error) {
	packageItems, err := m.ListPackageResourceToggles()
	if err != nil {
		return nil, err
	}
	topLevelItems := m.ListTopLevelResourceToggles()
	result := append([]PackageResourceToggleItem(nil), packageItems...)
	result = append(result, topLevelItems...)
	sortPackageResourceToggleItems(result)
	return result, nil
}

func (m *DefaultPackageManager) ListTopLevelResourceToggles() []PackageResourceToggleItem {
	if m == nil || m.settingsManager == nil {
		return nil
	}
	type scopeConfig struct {
		scope    string
		baseDir  string
		settings map[string]any
	}
	scopes := []scopeConfig{
		{scope: "user", baseDir: m.agentDir, settings: m.settingsManager.GetGlobalSettings()},
		{scope: "project", baseDir: filepath.Join(m.cwd, ConfigDirName), settings: m.settingsManager.GetProjectSettings()},
	}
	var result []PackageResourceToggleItem
	for _, scope := range scopes {
		if scope.scope == "project" && !m.settingsManager.IsProjectTrusted() {
			continue
		}
		if scope.baseDir == "" {
			continue
		}
		for _, resourceType := range []string{"extensions", "skills", "prompts", "themes"} {
			for _, resourcePath := range topLevelResourcePaths(scope.baseDir, resourceType, scope.settings) {
				pattern := packageResourceTogglePattern(scope.baseDir, resourcePath)
				if pattern == "" {
					continue
				}
				enabled := resourceEnabled(resourcePath, settingsStringSlice(m.settingsManager.mergedSnapshot(), resourceType), m.cwd, m.agentDir)
				result = append(result, PackageResourceToggleItem{
					Source:       "auto",
					Scope:        scope.scope,
					ResourceType: resourceType,
					Pattern:      pattern,
					Path:         resourcePath,
					DisplayName:  packageResourceDisplayName(resourceType, resourcePath),
					Enabled:      enabled,
					Metadata: ProtocolSourceInfo{
						Source: "auto",
						Scope:  scope.scope,
						Origin: "top-level",
					},
				})
			}
		}
	}
	sortPackageResourceToggleItems(result)
	return result
}

func topLevelResourcePaths(baseDir, resourceType string, settings map[string]any) []string {
	var paths []string
	paths = append(paths, collectProtocolPackageDir(filepath.Join(baseDir, resourceType), resourceType)...)
	for _, entry := range settingsStringSlice(settings, resourceType) {
		entry = strings.TrimSpace(entry)
		if entry == "" || strings.HasPrefix(entry, "!") || strings.HasPrefix(entry, "-") || strings.HasPrefix(entry, "+") {
			continue
		}
		resolved := ResolveToCwd(entry, baseDir)
		paths = append(paths, collectProtocolPackageEntries(baseDir, []string{resolved}, resourceType)...)
	}
	return dedupeProtocolPackagePaths(paths)
}

func packageResourceTogglePattern(packageDir, resourcePath string) string {
	if packageDir == "" || resourcePath == "" {
		return ""
	}
	rel, err := filepath.Rel(filepath.Clean(packageDir), filepath.Clean(resourcePath))
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return filepath.ToSlash(filepath.Clean(resourcePath))
	}
	return filepath.ToSlash(rel)
}

func packageResourceDisplayName(resourceType, resourcePath string) string {
	fileName := filepath.Base(resourcePath)
	parent := filepath.Base(filepath.Dir(resourcePath))
	switch resourceType {
	case "skills":
		if fileName == "SKILL.md" && parent != "." {
			return parent
		}
	case "extensions":
		if parent != "" && parent != "." && parent != "extensions" {
			return filepath.ToSlash(filepath.Join(parent, fileName))
		}
	}
	return fileName
}

func sortPackageResourceToggleItems(items []PackageResourceToggleItem) {
	typeOrder := map[string]int{"extensions": 0, "skills": 1, "prompts": 2, "themes": 3}
	originOrder := map[string]int{"package": 0, "top-level": 1}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if originOrder[left.Metadata.Origin] != originOrder[right.Metadata.Origin] {
			return originOrder[left.Metadata.Origin] < originOrder[right.Metadata.Origin]
		}
		if left.Scope != right.Scope {
			return left.Scope < right.Scope
		}
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		if typeOrder[left.ResourceType] != typeOrder[right.ResourceType] {
			return typeOrder[left.ResourceType] < typeOrder[right.ResourceType]
		}
		if left.DisplayName != right.DisplayName {
			return left.DisplayName < right.DisplayName
		}
		return left.Pattern < right.Pattern
	})
}

func dedupePackageResourceToggleItems(items []PackageResourceToggleItem) []PackageResourceToggleItem {
	seen := map[string]struct{}{}
	result := make([]PackageResourceToggleItem, 0, len(items))
	for _, item := range items {
		key := item.ResourceType + "\x00" + CanonicalizePath(item.Path)
		if item.Path == "" {
			key = item.ResourceType + "\x00" + item.Source + "\x00" + item.Pattern
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func packageSettingHasFilters(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"extensions", "skills", "prompts", "themes"} {
		if len(settingsStringSlice(object, key)) > 0 {
			return true
		}
	}
	return false
}

func (m *DefaultPackageManager) GetInstalledPath(source, scope string) string {
	parsed := ParsePackageSource(source)
	switch parsed.Type {
	case "git":
		return m.gitPackageInstallPath(GitSource{Host: parsed.Host, Path: parsed.Path}, scope == "project")
	case "official":
		return filepath.Join(officialPackageStoreDir(m.agentDir, m.cwd), parsed.Path)
	case "local":
		baseDir := m.agentDir
		if scope == "project" {
			baseDir = filepath.Join(m.cwd, ConfigDirName)
		}
		return ResolveToCwd(parsed.Path, baseDir)
	default:
		return ""
	}
}

func (m *DefaultPackageManager) SetPackageResourceEnabled(toggle PackageResourceToggle) (bool, error) {
	if m == nil || m.settingsManager == nil {
		return false, nil
	}
	resourceType := strings.TrimSpace(toggle.ResourceType)
	if !validPackageResourceType(resourceType) {
		return false, fmt.Errorf("unsupported package resource type %q", toggle.ResourceType)
	}
	pattern := strings.TrimSpace(toggle.Pattern)
	if pattern == "" {
		return false, fmt.Errorf("missing package resource pattern")
	}
	project := toggle.Scope == "project"
	scope := "user"
	baseDir := m.agentDir
	if project {
		scope = "project"
		baseDir = filepath.Join(m.cwd, ConfigDirName)
	}
	values := settingsSlice(m.settingsManager.GetGlobalSettings(), "packages")
	if project {
		values = settingsSlice(m.settingsManager.GetProjectSettings(), "packages")
	}
	for index, value := range values {
		spec, ok := protocolPackageSourceSpecFromSettings(value, scope)
		if !ok || !packageConfigSourceMatches(spec.Source, toggle.Source, baseDir) {
			continue
		}
		object, originalSource := packageSettingObject(value, spec.Source)
		filters := settingsStringSlice(object, resourceType)
		object[resourceType] = stringSliceToAny(updatePackageResourceFilters(filters, pattern, toggle.Enabled))
		if len(settingsStringSlice(object, resourceType)) == 0 {
			delete(object, resourceType)
		}
		if !packageSettingObjectHasFilters(object) {
			values[index] = originalSource
		} else {
			values[index] = object
		}
		if project {
			if err := m.settingsManager.SetProjectPackages(values); err != nil {
				return false, err
			}
		} else {
			m.settingsManager.SetPackages(values)
		}
		return true, nil
	}
	return false, nil
}

func (m *DefaultPackageManager) SetTopLevelResourceEnabled(toggle TopLevelResourceToggle) (bool, error) {
	if m == nil || m.settingsManager == nil {
		return false, nil
	}
	resourceType := strings.TrimSpace(toggle.ResourceType)
	if !validPackageResourceType(resourceType) {
		return false, fmt.Errorf("unsupported resource type %q", toggle.ResourceType)
	}
	pattern := strings.TrimSpace(toggle.Pattern)
	if pattern == "" {
		return false, fmt.Errorf("missing resource pattern")
	}
	project := toggle.Scope == "project"
	values := settingsStringSlice(m.settingsManager.GetGlobalSettings(), resourceType)
	if project {
		values = settingsStringSlice(m.settingsManager.GetProjectSettings(), resourceType)
	}
	values = updatePackageResourceFilters(values, pattern, toggle.Enabled)
	if project {
		if err := m.settingsManager.setProject(resourceType, stringSliceToAny(values)); err != nil {
			return false, err
		}
	} else {
		m.settingsManager.setGlobal(resourceType, stringSliceToAny(values))
	}
	return true, nil
}

func projectPackageScope(project bool) string {
	if project {
		return "project"
	}
	return "user"
}

func (m *DefaultPackageManager) assertProjectTrustedForScope(scope string) error {
	if scope == "project" && (m == nil || m.settingsManager == nil || !m.settingsManager.IsProjectTrusted()) {
		return errors.New("Project is not trusted; refusing to access project package storage")
	}
	return nil
}

func validPackageResourceType(resourceType string) bool {
	switch resourceType {
	case "extensions", "skills", "prompts", "themes":
		return true
	default:
		return false
	}
}

func packageConfigSourceMatches(configuredSource, targetSource, baseDir string) bool {
	configuredSource = strings.TrimSpace(configuredSource)
	targetSource = strings.TrimSpace(targetSource)
	return configuredSource == targetSource || protocolPackageSettingsIdentity(configuredSource, baseDir) == targetSource
}

func packageSettingObject(value any, source string) (map[string]any, string) {
	if object, ok := value.(map[string]any); ok {
		cloned := cloneSettingsMap(object)
		if objectSource, ok := cloned["source"].(string); ok && strings.TrimSpace(objectSource) != "" {
			return cloned, strings.TrimSpace(objectSource)
		}
		cloned["source"] = source
		return cloned, source
	}
	return map[string]any{"source": source}, source
}

func updatePackageResourceFilters(filters []string, pattern string, enabled bool) []string {
	updated := make([]string, 0, len(filters)+1)
	for _, filter := range filters {
		if packageResourceFilterPattern(filter) == pattern {
			continue
		}
		updated = append(updated, filter)
	}
	prefix := "-"
	if enabled {
		prefix = "+"
	}
	return append(updated, prefix+pattern)
}

func packageResourceFilterPattern(filter string) string {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return ""
	}
	if strings.ContainsAny(filter[:1], "!+-") {
		return strings.TrimSpace(filter[1:])
	}
	return filter
}

func stringSliceToAny(values []string) []any {
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = value
	}
	return out
}

func packageSettingObjectHasFilters(object map[string]any) bool {
	for _, key := range []string{"extensions", "skills", "prompts", "themes"} {
		if len(settingsStringSlice(object, key)) > 0 {
			return true
		}
	}
	return false
}

func ParsePackageSource(source string) PackageSource {
	trimmed := strings.TrimSpace(source)
	if strings.HasPrefix(trimmed, "npm:") {
		return PackageSource{Type: "unsupported", Source: trimmed, Path: strings.TrimSpace(strings.TrimPrefix(trimmed, "npm:"))}
	}
	if strings.HasPrefix(trimmed, "official:") {
		return PackageSource{Type: "official", Source: trimmed, Path: strings.TrimSpace(strings.TrimPrefix(trimmed, "official:"))}
	}
	if gitSource, ok := ParseGitURL(trimmed); ok {
		return PackageSource{
			Type:   "git",
			Source: trimmed,
			Repo:   gitSource.Repo,
			Host:   gitSource.Host,
			Path:   gitSource.Path,
			Ref:    gitSource.Ref,
			Pinned: gitSource.Pinned,
		}
	}
	return PackageSource{Type: "local", Source: trimmed, Path: trimmed}
}

func PackageSourceIdentity(source PackageSource) string {
	if source.Type == "git" {
		return "git:" + source.Host + "/" + source.Path
	}
	if source.Type == "official" {
		return "official:" + source.Path
	}
	if source.Type == "unsupported" {
		return "unsupported:" + source.Source
	}
	return "local:" + filepath.Clean(source.Path)
}

func (m *DefaultPackageManager) Update(sources ...string) error {
	m.emitProgress(PackageProgressEvent{Type: "start", Action: "update"})
	configured := m.ListConfiguredPackages()
	type updateTarget struct {
		source string
		scope  string
	}
	var targets []updateTarget
	if len(sources) == 0 {
		for _, pkg := range configured {
			targets = append(targets, updateTarget{source: pkg.Source, scope: pkg.Scope})
		}
	} else {
		for _, sourceText := range sources {
			if unsupportedPackageSource(sourceText) {
				err := unsupportedPackageSourceError(sourceText)
				m.emitProgress(PackageProgressEvent{Type: "error", Action: "update", Source: strings.TrimSpace(sourceText), Error: err.Error()})
				return err
			}
			if suggestion := m.packageUpdateSuggestion(sourceText); suggestion != "" {
				err := PackageUpdateSuggestionError{Input: sourceText, Suggestion: suggestion}
				m.emitProgress(PackageProgressEvent{Type: "error", Action: "update", Source: strings.TrimSpace(sourceText), Error: err.Error()})
				return err
			}
			identity := PackageSourceIdentity(ParsePackageSource(sourceText))
			for _, pkg := range configured {
				if PackageSourceIdentity(ParsePackageSource(pkg.Source)) == identity {
					targets = append(targets, updateTarget{source: pkg.Source, scope: pkg.Scope})
				}
			}
		}
	}
	for _, target := range targets {
		sourceText := target.source
		if unsupportedPackageSource(sourceText) {
			err := unsupportedPackageSourceError(sourceText)
			m.emitProgress(PackageProgressEvent{Type: "error", Action: "update", Source: strings.TrimSpace(sourceText), Error: err.Error()})
			return err
		}
	}
	for _, target := range targets {
		sourceText := strings.TrimSpace(target.source)
		source, ok := ParseGitURL(sourceText)
		if !ok || source.Pinned {
			continue
		}
		installedDir := m.gitPackageInstallPath(source, target.scope == "project")
		if _, err := os.Stat(installedDir); err != nil {
			if os.IsNotExist(err) {
				if err := m.installGitPackage(source, target.scope == "project"); err != nil {
					m.emitProgress(PackageProgressEvent{Type: "error", Action: "update", Source: sourceText, Error: err.Error()})
					return err
				}
				continue
			}
			m.emitProgress(PackageProgressEvent{Type: "error", Action: "update", Source: sourceText, Error: err.Error()})
			return err
		}
		if err := m.refreshGitPackage(installedDir); err != nil {
			m.emitProgress(PackageProgressEvent{Type: "error", Action: "update", Source: sourceText, Error: err.Error()})
			return err
		}
	}
	m.emitProgress(PackageProgressEvent{Type: "done", Action: "update"})
	return nil
}

func (m *DefaultPackageManager) CheckForAvailableUpdates() ([]PackageUpdate, error) {
	if m == nil || packageManagerOffline() {
		return nil, nil
	}
	configured := m.ListConfiguredPackages()
	updates := make([]PackageUpdate, 0, len(configured))
	seen := map[string]struct{}{}
	for _, pkg := range configured {
		parsed := ParsePackageSource(pkg.Source)
		identity := PackageSourceIdentity(parsed)
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}
		if parsed.Type != "git" || parsed.Pinned {
			continue
		}
		if _, err := os.Stat(pkg.InstalledPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		hasUpdate, err := m.gitPackageHasAvailableUpdate(pkg.InstalledPath)
		if err != nil || !hasUpdate {
			continue
		}
		updates = append(updates, PackageUpdate{
			Source:      pkg.Source,
			DisplayName: parsed.Host + "/" + parsed.Path,
			Type:        "git",
			Scope:       pkg.Scope,
		})
	}
	return updates, nil
}

func (m *DefaultPackageManager) SetProgressCallback(callback func(PackageProgressEvent)) {
	if m == nil {
		return
	}
	m.progress = callback
}

func (m *DefaultPackageManager) emitProgress(event PackageProgressEvent) {
	if m == nil || m.progress == nil {
		return
	}
	m.progress(event)
}

func unsupportedPackageSource(source string) bool {
	return ParsePackageSource(source).Type == "unsupported"
}

func unsupportedPackageSourceError(source string) error {
	return fmt.Errorf("unsupported package source %q: Gi packages support local paths, git URLs, and official:<name> sources with gi.package.json; npm packages are not supported", strings.TrimSpace(source))
}

func (m *DefaultPackageManager) RunSelfUpdate(options SelfUpdateOptions) (SelfUpdateResult, error) {
	packageName := firstNonEmptyString(options.PackageName, DefaultCodingAgentPackageName)
	currentVersion := firstNonEmptyString(options.CurrentVersion, DefaultCodingAgentVersion)
	updatePackageName := packageName
	if !options.Force {
		checker := options.VersionCheck
		if checker == nil {
			checker = GetLatestGiRelease
		}
		release, ok := checker(currentVersion, options.VersionCheckOptions)
		if !ok || !IsNewerPackageVersion(release.Version, currentVersion) {
			return SelfUpdateResult{}, nil
		}
		if release.PackageName != "" {
			updatePackageName = release.PackageName
		}
	}

	command := GetSelfUpdateCommand(packageName, options.Environment, nil, updatePackageName)
	if command == nil {
		return SelfUpdateResult{}, fmt.Errorf("%s", GetSelfUpdateUnavailableInstruction(packageName, options.Environment, nil, updatePackageName))
	}
	steps := command.Steps
	if len(steps) == 0 {
		steps = []SelfUpdateCommandStep{{Command: command.Command, Args: command.Args, Display: command.Display}}
	}
	for _, step := range steps {
		if err := m.operations.RunCommand(step.Command, step.Args, PackageCommandOptions{}); err != nil {
			return SelfUpdateResult{}, err
		}
	}
	return SelfUpdateResult{Updated: true, PackageName: updatePackageName}, nil
}

func (m *DefaultPackageManager) ResolveExtensionSources(sources []string, options ResolveExtensionSourcesOptions) ([]string, error) {
	resolved := make([]string, 0, len(sources))
	for _, sourceText := range sources {
		source, ok := ParseGitURL(sourceText)
		if !ok {
			continue
		}
		packageDir := gitPackageInstallPath(m.agentDir, source)
		if options.Temporary {
			packageDir = temporaryGitPackagePath(source)
		}
		offline := packageManagerOffline()
		if options.Temporary && !source.Pinned && !offline {
			if _, err := os.Stat(packageDir); err == nil {
				if err := m.refreshGitPackage(packageDir); err != nil {
					return nil, err
				}
			} else if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
		} else if offline {
			if _, err := os.Stat(packageDir); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
		}
		resolved = append(resolved, packageDir)
	}
	return resolved, nil
}

func (m *DefaultPackageManager) installGitPackage(source GitSource, project bool) error {
	targetDir := m.gitPackageInstallPath(source, project)
	if _, err := os.Stat(targetDir); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := ensurePackageStoreGitIgnore(m.gitPackageInstallRoot(project)); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		return err
	}
	if err := m.operations.RunCommand("git", []string{"clone", source.Repo, targetDir}, PackageCommandOptions{}); err != nil {
		return err
	}
	if source.Ref != "" {
		if err := m.operations.RunCommand("git", []string{"checkout", source.Ref}, PackageCommandOptions{CWD: targetDir}); err != nil {
			return err
		}
	}
	return nil
}

func packageManagerOffline() bool {
	value := strings.TrimSpace(os.Getenv("GI_OFFLINE"))
	if value == "" {
		value = strings.TrimSpace(os.Getenv("PI_OFFLINE"))
	}
	return value == "1" || strings.EqualFold(value, "true")
}

func (m *DefaultPackageManager) refreshGitPackage(packageDir string) error {
	branch := "main"
	fetchArgs := []string{"fetch", "--prune", "--no-tags", "origin", "+refs/heads/" + branch + ":refs/remotes/origin/" + branch}
	if err := m.operations.RunCommand("git", fetchArgs, PackageCommandOptions{CWD: packageDir}); err != nil {
		return err
	}

	localHead, _ := m.operations.RunCommandCapture("git", []string{"rev-parse", "HEAD"}, PackageCommandOptions{CWD: packageDir})
	remoteHead, _ := m.operations.RunCommandCapture("git", []string{"rev-parse", "origin/" + branch}, PackageCommandOptions{CWD: packageDir})
	if strings.TrimSpace(localHead) != "" && strings.TrimSpace(localHead) == strings.TrimSpace(remoteHead) {
		return nil
	}

	if err := m.operations.RunCommand("git", []string{"reset", "--hard", "origin/" + branch}, PackageCommandOptions{CWD: packageDir}); err != nil {
		return err
	}
	return m.operations.RunCommand("git", []string{"clean", "-fdx"}, PackageCommandOptions{CWD: packageDir})
}

func (m *DefaultPackageManager) gitPackageHasAvailableUpdate(packageDir string) (bool, error) {
	localHead, err := m.operations.RunCommandCapture("git", []string{"rev-parse", "HEAD"}, PackageCommandOptions{CWD: packageDir})
	if err != nil {
		return false, err
	}
	remoteHead, err := m.gitPackageRemoteHead(packageDir)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(localHead) != "" && strings.TrimSpace(remoteHead) != "" && strings.TrimSpace(localHead) != strings.TrimSpace(remoteHead), nil
}

func (m *DefaultPackageManager) gitPackageRemoteHead(packageDir string) (string, error) {
	output, err := m.operations.RunCommandCapture("git", []string{"ls-remote", "origin", "HEAD"}, PackageCommandOptions{CWD: packageDir})
	if err != nil {
		return "", err
	}
	return parseGitRemoteHead(output), nil
}

func parseGitRemoteHead(output string) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "HEAD" {
			return fields[0]
		}
	}
	return ""
}

func normalizePackageManagerOperations(operations PackageManagerOperations) PackageManagerOperations {
	if operations.RunCommand == nil {
		operations.RunCommand = runPackageCommand
	}
	if operations.RunCommandCapture == nil {
		operations.RunCommandCapture = runPackageCommandCapture
	}
	return operations
}

func runPackageCommand(command string, args []string, options PackageCommandOptions) error {
	cmd := exec.Command(command, args...)
	cmd.Dir = options.CWD
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Command failed: %s %s\n%s", command, strings.Join(args, " "), string(output))
	}
	return nil
}

func runPackageCommandCapture(command string, args []string, options PackageCommandOptions) (string, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = options.CWD
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Command failed: %s %s\n%s", command, strings.Join(args, " "), string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func ensurePackageStoreGitIgnore(dir string) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte("*\n!.gitignore\n"), 0o644)
}

func settingsPackagesToStrings(values []any) []string {
	sources := make([]string, 0, len(values))
	for _, value := range values {
		if source, ok := value.(string); ok && strings.TrimSpace(source) != "" {
			sources = append(sources, source)
		}
	}
	return sources
}

func (m *DefaultPackageManager) settingsBaseDir(project bool) string {
	if project {
		return filepath.Join(m.cwd, ConfigDirName)
	}
	return m.agentDir
}

func (m *DefaultPackageManager) packageSettingsValue(source, baseDir string) string {
	parsed := ParsePackageSource(source)
	if parsed.Type != "local" {
		return source
	}
	absolute := ResolveToCwd(parsed.Path, m.cwd)
	if relative, err := filepath.Rel(baseDir, absolute); err == nil {
		cleaned := filepath.Clean(relative)
		if cleaned != "." && !strings.HasPrefix(cleaned, ".") {
			return "." + string(filepath.Separator) + cleaned
		}
		return cleaned
	}
	return absolute
}

func packageSettingsContains(existing []string, stored, source, cwd, baseDir string) bool {
	for _, value := range existing {
		if packageSettingsMatch(value, source, cwd, baseDir) || packageSettingsMatch(value, stored, cwd, baseDir) {
			return true
		}
	}
	return false
}

func packageSettingsMatch(existing, source, cwd, baseDir string) bool {
	existingParsed := ParsePackageSource(existing)
	sourceParsed := ParsePackageSource(source)
	if existingParsed.Type != sourceParsed.Type {
		return false
	}
	if existingParsed.Type == "local" {
		return packageLocalIdentity(existingParsed.Path, baseDir) == packageLocalIdentity(sourceParsed.Path, cwd)
	}
	return PackageSourceIdentity(existingParsed) == PackageSourceIdentity(sourceParsed)
}

func packageLocalIdentity(path, baseDir string) string {
	absolute := ResolveToCwd(strings.TrimRight(path, `/\`), baseDir)
	if realPath, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = realPath
	}
	return filepath.Clean(absolute)
}

func gitPackageInstallPath(agentDir string, source GitSource) string {
	elements := []string{agentDir, "git", source.Host}
	elements = append(elements, strings.Split(source.Path, "/")...)
	return filepath.Join(elements...)
}

func (m *DefaultPackageManager) gitPackageInstallPath(source GitSource, project bool) string {
	return gitPackageInstallPath(m.packageInstallBaseDir(project), source)
}

func (m *DefaultPackageManager) gitPackageInstallRoot(project bool) string {
	return filepath.Join(m.packageInstallBaseDir(project), "git")
}

func (m *DefaultPackageManager) packageInstallBaseDir(project bool) string {
	if project {
		return filepath.Join(m.cwd, ConfigDirName)
	}
	return m.agentDir
}

func temporaryGitPackagePath(source GitSource) string {
	cacheKey := "git-" + source.Host + "-" + source.Path
	sum := sha256.Sum256([]byte(cacheKey))
	hash := hex.EncodeToString(sum[:])[:8]
	elements := []string{os.TempDir(), "pi-extensions", "git-" + source.Host, hash}
	elements = append(elements, strings.Split(source.Path, "/")...)
	return filepath.Join(elements...)
}
