package gicodingagent

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type ProtocolPackageResource struct {
	Path     string
	Enabled  bool
	Metadata ProtocolSourceInfo
}

type ProtocolPackageResources struct {
	Extensions        []ProtocolPackageResource
	ProcessExtensions []ProtocolPackageProcessExtension
	Skills            []ProtocolPackageResource
	Prompts           []ProtocolPackageResource
	Themes            []ProtocolPackageResource
}

type ProtocolPackageProcessExtension struct {
	ID           string
	Path         string
	PackageDir   string
	Command      []string
	Transport    string
	Protocol     string
	Capabilities []string
	Env          map[string]string
	Metadata     ProtocolSourceInfo
}

type ProtocolPackageSourceSpec struct {
	Source   string
	Scope    string
	Autoload *bool
	Filters  ProtocolPackageResourceFilters
}

type ProtocolPackageResourceFilters struct {
	Extensions []string
	Skills     []string
	Prompts    []string
	Themes     []string

	extensionsSet bool
	skillsSet     bool
	promptsSet    bool
	themesSet     bool
}

type protocolPackageManifest struct {
	Extensions        []string
	ProcessExtensions []protocolPackageManifestProcessExtension
	Skills            []string
	Prompts           []string
	Themes            []string
}

type protocolPackageManifestProcessExtension struct {
	ID           string
	Command      []string
	Transport    string
	Protocol     string
	Capabilities []string
	Env          map[string]string
}

func (m *DefaultPackageManager) ResolveProtocolPackageResources(sources []string) (ProtocolPackageResources, error) {
	specs := make([]ProtocolPackageSourceSpec, 0, len(sources))
	for _, source := range sources {
		specs = append(specs, ProtocolPackageSourceSpec{Source: source})
	}
	return m.ResolveProtocolPackageSourceSpecs(specs)
}

func (m *DefaultPackageManager) ResolveProtocolPackageSourceSpecs(specs []ProtocolPackageSourceSpec) (ProtocolPackageResources, error) {
	var result ProtocolPackageResources
	processOverrides := map[string]bool{}
	for _, spec := range specs {
		if err := m.assertProjectTrustedForScope(spec.Scope); err != nil {
			return ProtocolPackageResources{}, err
		}
		resolvedSource := spec.Source
		resolvedScope := spec.Scope
		if base, ok := m.findAutoloadDeltaBase(spec, specs); ok {
			resolvedSource = base.Source
			resolvedScope = base.Scope
		}
		resolved, packageDir, err := m.resolveProtocolPackageSource(resolvedSource, resolvedScope)
		if err != nil {
			return ProtocolPackageResources{}, err
		}
		if packageDir == "" {
			continue
		}
		appendResources := func(target *[]ProtocolPackageResource, resources []ProtocolPackageResource, resourceType string) {
			filters, configured := spec.Filters.forResourceType(resourceType)
			var filtered []ProtocolPackageResource
			if spec.autoloadEnabled() {
				filtered = applyProtocolPackageFiltersConfigured(packageDir, resources, filters, configured)
			} else {
				filtered = applyProtocolPackageDeltaFilters(packageDir, resources, filters)
			}
			*target = append(*target, protocolPackageResourcesWithSpecMetadata(filtered, spec)...)
		}
		appendResources(&result.Extensions, resolved.Extensions, "extensions")
		appendResources(&result.Skills, resolved.Skills, "skills")
		appendResources(&result.Prompts, resolved.Prompts, "prompts")
		appendResources(&result.Themes, resolved.Themes, "themes")

		extensionFilters, extensionsConfigured := spec.Filters.forResourceType("extensions")
		if !spec.autoloadEnabled() {
			for _, process := range resolved.ProcessExtensions {
				enabled, matched := protocolPackageDeltaProcessState(packageDir, process, extensionFilters)
				if !matched {
					continue
				}
				key := protocolPackageProcessKey(process)
				processOverrides[key] = enabled
				if enabled {
					result.ProcessExtensions = append(result.ProcessExtensions, protocolPackageProcessWithSpecMetadata(process, spec))
				}
			}
			continue
		}
		for _, process := range resolved.ProcessExtensions {
			key := protocolPackageProcessKey(process)
			if _, overridden := processOverrides[key]; overridden {
				continue
			}
			if protocolPackageProcessEnabledConfigured(packageDir, process, extensionFilters, extensionsConfigured) {
				result.ProcessExtensions = append(result.ProcessExtensions, protocolPackageProcessWithSpecMetadata(process, spec))
			}
		}
	}
	result.Extensions = dedupeProtocolPackageResources(result.Extensions)
	result.ProcessExtensions = dedupeProtocolPackageProcessExtensions(result.ProcessExtensions)
	result.Skills = dedupeProtocolPackageResources(result.Skills)
	result.Prompts = dedupeProtocolPackageResources(result.Prompts)
	result.Themes = dedupeProtocolPackageResources(result.Themes)
	return result, nil
}

func (m *DefaultPackageManager) ResolveConfiguredProtocolPackageResources() (ProtocolPackageResources, error) {
	specs := m.configuredProtocolPackageSourceSpecs()
	return m.ResolveProtocolPackageSourceSpecs(specs)
}

func (m *DefaultPackageManager) configuredProtocolPackageSourceSpecs() []ProtocolPackageSourceSpec {
	var candidates []ProtocolPackageSourceSpec
	addSpecs := func(values []any, scope string) {
		for _, value := range values {
			spec, ok := protocolPackageSourceSpecFromSettings(value, scope)
			if !ok {
				continue
			}
			candidates = append(candidates, spec)
		}
	}
	if m.settingsManager.IsProjectTrusted() {
		addSpecs(settingsSlice(m.settingsManager.GetProjectSettings(), "packages"), "project")
	}
	addSpecs(settingsSlice(m.settingsManager.GetGlobalSettings(), "packages"), "user")

	specs := make([]ProtocolPackageSourceSpec, 0, len(candidates))
	index := map[string]int{}
	for _, candidate := range candidates {
		baseDir := m.settingsBaseDir(candidate.Scope == "project")
		key := protocolPackageSettingsIdentity(candidate.Source, baseDir)
		existingIndex, exists := index[key]
		if !exists {
			index[key] = len(specs)
			specs = append(specs, candidate)
			continue
		}
		existing := specs[existingIndex]
		switch {
		case existing.Scope == "project" && candidate.Scope == "user":
			if !existing.autoloadEnabled() {
				specs = append(specs, candidate)
			}
		case candidate.Scope == "project":
			specs[existingIndex] = candidate
		}
	}
	return specs
}

func protocolPackageSourceSpecFromSettings(value any, scope string) (ProtocolPackageSourceSpec, bool) {
	if source, ok := value.(string); ok && strings.TrimSpace(source) != "" {
		return ProtocolPackageSourceSpec{Source: source, Scope: scope}, true
	}
	object, ok := value.(map[string]any)
	if !ok {
		return ProtocolPackageSourceSpec{}, false
	}
	source, _ := object["source"].(string)
	if strings.TrimSpace(source) == "" {
		return ProtocolPackageSourceSpec{}, false
	}
	spec := ProtocolPackageSourceSpec{
		Source: strings.TrimSpace(source),
		Scope:  scope,
		Filters: ProtocolPackageResourceFilters{
			Extensions: settingsStringSlice(object, "extensions"),
			Skills:     settingsStringSlice(object, "skills"),
			Prompts:    settingsStringSlice(object, "prompts"),
			Themes:     settingsStringSlice(object, "themes"),

			extensionsSet: settingsHasSlice(object, "extensions"),
			skillsSet:     settingsHasSlice(object, "skills"),
			promptsSet:    settingsHasSlice(object, "prompts"),
			themesSet:     settingsHasSlice(object, "themes"),
		},
	}
	if autoload, ok := object["autoload"].(bool); ok {
		spec.Autoload = &autoload
	}
	return spec, true
}

func settingsHasSlice(settings map[string]any, key string) bool {
	_, ok := settings[key].([]any)
	return ok
}

func (s ProtocolPackageSourceSpec) autoloadEnabled() bool {
	return s.Autoload == nil || *s.Autoload
}

func (f ProtocolPackageResourceFilters) forResourceType(resourceType string) ([]string, bool) {
	switch resourceType {
	case "extensions":
		return f.Extensions, f.extensionsSet || f.Extensions != nil
	case "skills":
		return f.Skills, f.skillsSet || f.Skills != nil
	case "prompts":
		return f.Prompts, f.promptsSet || f.Prompts != nil
	case "themes":
		return f.Themes, f.themesSet || f.Themes != nil
	default:
		return nil, false
	}
}

func (m *DefaultPackageManager) findAutoloadDeltaBase(spec ProtocolPackageSourceSpec, specs []ProtocolPackageSourceSpec) (ProtocolPackageSourceSpec, bool) {
	if spec.Scope != "project" || spec.autoloadEnabled() {
		return ProtocolPackageSourceSpec{}, false
	}
	identity := protocolPackageSettingsIdentity(spec.Source, filepath.Join(m.cwd, ConfigDirName))
	for _, candidate := range specs {
		if candidate.Scope != "user" {
			continue
		}
		if protocolPackageSettingsIdentity(candidate.Source, m.agentDir) == identity {
			return candidate, true
		}
	}
	return ProtocolPackageSourceSpec{}, false
}

func protocolPackageResourcesWithSpecMetadata(resources []ProtocolPackageResource, spec ProtocolPackageSourceSpec) []ProtocolPackageResource {
	result := make([]ProtocolPackageResource, len(resources))
	for index, resource := range resources {
		result[index] = resource
		if !spec.autoloadEnabled() {
			result[index].Metadata.Source = spec.Source
			result[index].Metadata.Scope = firstNonEmptyString(spec.Scope, "temporary")
			result[index].Metadata.Origin = "package"
		}
	}
	return result
}

func protocolPackageProcessWithSpecMetadata(process ProtocolPackageProcessExtension, spec ProtocolPackageSourceSpec) ProtocolPackageProcessExtension {
	if !spec.autoloadEnabled() {
		process.Metadata.Source = spec.Source
		process.Metadata.Scope = firstNonEmptyString(spec.Scope, "temporary")
		process.Metadata.Origin = "package"
	}
	return process
}

func protocolPackageSettingsIdentity(source, baseDir string) string {
	parsed := ParsePackageSource(source)
	if parsed.Type == "local" {
		return "local:" + packageLocalIdentity(parsed.Path, baseDir)
	}
	return PackageSourceIdentity(parsed)
}

func (m *DefaultPackageManager) resolveProtocolPackageSource(sourceText, scope string) (ProtocolPackageResources, string, error) {
	sourceText = strings.TrimSpace(sourceText)
	if sourceText == "" {
		return ProtocolPackageResources{}, "", nil
	}
	parsed := ParsePackageSource(sourceText)
	if parsed.Type == "official" {
		packageDir, err := m.materializeOfficialPackage(parsed.Path)
		if err != nil {
			return ProtocolPackageResources{}, "", err
		}
		metadata := ProtocolSourceInfo{
			Source: PackageSourceIdentity(parsed),
			Scope:  firstNonEmptyString(scope, "temporary"),
			Origin: "package",
		}
		return resolveProtocolPackageDir(packageDir, metadata), packageDir, nil
	}
	if parsed.Type == "git" {
		packageDir := m.gitPackageInstallPath(GitSource{Host: parsed.Host, Path: parsed.Path}, scope == "project")
		info, err := os.Stat(packageDir)
		if err != nil {
			if os.IsNotExist(err) {
				return ProtocolPackageResources{}, "", nil
			}
			return ProtocolPackageResources{}, "", err
		}
		if !info.IsDir() {
			return ProtocolPackageResources{}, packageDir, nil
		}
		metadata := ProtocolSourceInfo{
			Source: PackageSourceIdentity(parsed),
			Scope:  firstNonEmptyString(scope, "temporary"),
			Origin: "package",
		}
		return resolveProtocolPackageDir(packageDir, metadata), packageDir, nil
	}
	if parsed.Type == "unsupported" {
		return ProtocolPackageResources{}, "", unsupportedPackageSourceError(sourceText)
	}
	baseDir := m.cwd
	switch scope {
	case "user":
		baseDir = m.agentDir
	case "project":
		baseDir = filepath.Join(m.cwd, ConfigDirName)
	}
	sourcePath := ResolveToCwd(sourceText, baseDir)
	info, err := os.Stat(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ProtocolPackageResources{}, "", nil
		}
		return ProtocolPackageResources{}, "", err
	}
	metadata := ProtocolSourceInfo{
		Source: m.GetPackageIdentity(sourceText),
		Scope:  firstNonEmptyString(scope, "temporary"),
		Origin: "package",
	}
	if !info.IsDir() {
		if !isProtocolExtensionFile(sourcePath) {
			return ProtocolPackageResources{}, filepath.Dir(sourcePath), nil
		}
		return ProtocolPackageResources{Extensions: []ProtocolPackageResource{protocolPackageResource(sourcePath, metadata)}}, filepath.Dir(sourcePath), nil
	}
	return resolveProtocolPackageDir(sourcePath, metadata), sourcePath, nil
}

func resolveProtocolPackageDir(packageDir string, metadata ProtocolSourceInfo) ProtocolPackageResources {
	if manifest, ok := readProtocolPackageManifest(packageDir); ok {
		return resolveProtocolPackageManifest(packageDir, metadata, manifest)
	}
	var result ProtocolPackageResources
	for _, source := range discoverProtocolExtensionsInDir(filepath.Join(packageDir, "extensions")).Extensions {
		result.Extensions = append(result.Extensions, protocolPackageResource(source.Path, metadata))
	}
	for _, path := range collectProtocolSkillFiles(filepath.Join(packageDir, "skills")) {
		result.Skills = append(result.Skills, protocolPackageResource(path, metadata))
	}
	for _, path := range collectProtocolFilesByExt(filepath.Join(packageDir, "prompts"), ".md") {
		result.Prompts = append(result.Prompts, protocolPackageResource(path, metadata))
	}
	for _, path := range collectProtocolFilesByExt(filepath.Join(packageDir, "themes"), ".json") {
		result.Themes = append(result.Themes, protocolPackageResource(path, metadata))
	}
	return result
}

func readProtocolPackageManifest(packageDir string) (protocolPackageManifest, bool) {
	return readProtocolPackageManifestFile(filepath.Join(packageDir, "gi.package.json"))
}

func readProtocolPackageManifestFile(path string) (protocolPackageManifest, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return protocolPackageManifest{}, false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(content, &raw); err != nil {
		return protocolPackageManifest{}, false
	}
	if gi, ok := raw["gi"]; ok {
		var nested map[string]json.RawMessage
		if err := json.Unmarshal(gi, &nested); err == nil {
			raw = nested
		}
	}
	manifest := protocolPackageManifest{
		Skills:  protocolManifestStringList(raw["skills"]),
		Prompts: protocolManifestStringList(raw["prompts"]),
		Themes:  protocolManifestStringList(raw["themes"]),
	}
	manifest.Extensions, manifest.ProcessExtensions = protocolManifestExtensionEntries(raw["extensions"])
	return manifest, len(manifest.Extensions)+len(manifest.ProcessExtensions)+len(manifest.Skills)+len(manifest.Prompts)+len(manifest.Themes) > 0
}

func protocolManifestStringList(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		return values
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil && strings.TrimSpace(value) != "" {
		return []string{value}
	}
	return nil
}

func protocolManifestExtensionEntries(raw json.RawMessage) ([]string, []protocolPackageManifestProcessExtension) {
	if len(raw) == 0 {
		return nil, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		var value string
		if err := json.Unmarshal(raw, &value); err == nil && strings.TrimSpace(value) != "" {
			return []string{value}, nil
		}
		return nil, nil
	}
	var descriptorPaths []string
	var processEntries []protocolPackageManifestProcessExtension
	for _, entry := range entries {
		var path string
		if err := json.Unmarshal(entry, &path); err == nil && strings.TrimSpace(path) != "" {
			descriptorPaths = append(descriptorPaths, path)
			continue
		}
		var object struct {
			ID    string `json:"id"`
			Entry struct {
				Kind      string   `json:"kind"`
				Command   []string `json:"command"`
				Transport string   `json:"transport"`
				Protocol  string   `json:"protocol"`
			} `json:"entry"`
			Capabilities []string          `json:"capabilities"`
			Env          map[string]string `json:"env"`
		}
		if err := json.Unmarshal(entry, &object); err != nil {
			continue
		}
		if strings.TrimSpace(object.Entry.Kind) != "process" {
			continue
		}
		processEntries = append(processEntries, protocolPackageManifestProcessExtension{
			ID:           strings.TrimSpace(object.ID),
			Command:      cleanStringSlice(object.Entry.Command),
			Transport:    strings.TrimSpace(object.Entry.Transport),
			Protocol:     strings.TrimSpace(object.Entry.Protocol),
			Capabilities: cleanStringSlice(object.Capabilities),
			Env:          cloneStringMap(object.Env),
		})
	}
	return descriptorPaths, processEntries
}

func resolveProtocolPackageManifest(packageDir string, metadata ProtocolSourceInfo, manifest protocolPackageManifest) ProtocolPackageResources {
	var result ProtocolPackageResources
	for _, path := range resolveProtocolPackageEntries(packageDir, manifest.Extensions, "extensions") {
		result.Extensions = append(result.Extensions, protocolPackageResource(path, metadata))
	}
	for _, extension := range manifest.ProcessExtensions {
		if process := protocolPackageProcessExtension(packageDir, metadata, extension); process.ID != "" {
			result.ProcessExtensions = append(result.ProcessExtensions, process)
		}
	}
	for _, path := range resolveProtocolPackageEntries(packageDir, manifest.Skills, "skills") {
		result.Skills = append(result.Skills, protocolPackageResource(path, metadata))
	}
	for _, path := range resolveProtocolPackageEntries(packageDir, manifest.Prompts, "prompts") {
		result.Prompts = append(result.Prompts, protocolPackageResource(path, metadata))
	}
	for _, path := range resolveProtocolPackageEntries(packageDir, manifest.Themes, "themes") {
		result.Themes = append(result.Themes, protocolPackageResource(path, metadata))
	}
	return result
}

func resolveProtocolPackageEntries(packageDir string, entries []string, kind string) []string {
	var positiveEntries []string
	var excludePatterns []string
	var forceEntries []string
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		switch entry[0] {
		case '!', '-':
			excludePatterns = append(excludePatterns, strings.TrimSpace(entry[1:]))
		case '+':
			forceEntries = append(forceEntries, strings.TrimSpace(entry[1:]))
		default:
			positiveEntries = append(positiveEntries, entry)
		}
	}
	paths := collectProtocolPackageEntries(packageDir, positiveEntries, kind)
	paths = filterProtocolPackagePaths(packageDir, paths, excludePatterns)
	paths = append(paths, collectProtocolPackageEntries(packageDir, forceEntries, kind)...)
	paths = dedupeProtocolPackagePaths(paths)
	sort.Strings(paths)
	return paths
}

func collectProtocolPackageEntries(packageDir string, entries []string, kind string) []string {
	var paths []string
	for _, entry := range entries {
		for _, candidate := range expandProtocolPackageEntry(packageDir, entry) {
			info, err := os.Stat(candidate)
			if err != nil {
				continue
			}
			if info.IsDir() {
				paths = append(paths, collectProtocolPackageDir(candidate, kind)...)
				continue
			}
			if protocolPackageFileMatchesKind(candidate, kind) {
				paths = append(paths, filepath.Clean(candidate))
			}
		}
	}
	return paths
}

func expandProtocolPackageEntry(packageDir, entry string) []string {
	resolved := ResolveToCwd(entry, packageDir)
	if !strings.ContainsAny(entry, "*?[") {
		return []string{resolved}
	}
	matches, err := filepath.Glob(resolved)
	if err != nil {
		return nil
	}
	sort.Strings(matches)
	return matches
}

func applyProtocolPackageFilters(packageDir string, resources []ProtocolPackageResource, filters []string) []ProtocolPackageResource {
	if len(filters) == 0 {
		return resources
	}
	hasPositive := false
	for _, filter := range filters {
		filter = strings.TrimSpace(filter)
		if filter != "" && filter[0] != '!' && filter[0] != '-' && filter[0] != '+' {
			hasPositive = true
			break
		}
	}
	result := make([]ProtocolPackageResource, len(resources))
	copy(result, resources)
	if hasPositive {
		for i := range result {
			result[i].Enabled = false
		}
	}
	for _, filter := range filters {
		filter = strings.TrimSpace(filter)
		if filter == "" {
			continue
		}
		enabled := true
		patternText := filter
		switch filter[0] {
		case '!', '-':
			enabled = false
			patternText = strings.TrimSpace(filter[1:])
		case '+':
			patternText = strings.TrimSpace(filter[1:])
		}
		for i := range result {
			if protocolPackagePatternMatches(packageDir, result[i].Path, patternText) {
				result[i].Enabled = enabled
			}
		}
	}
	return result
}

func applyProtocolPackageFiltersConfigured(packageDir string, resources []ProtocolPackageResource, filters []string, configured bool) []ProtocolPackageResource {
	if configured && len(filters) == 0 {
		result := make([]ProtocolPackageResource, len(resources))
		copy(result, resources)
		for index := range result {
			result[index].Enabled = false
		}
		return result
	}
	return applyProtocolPackageFilters(packageDir, resources, filters)
}

func applyProtocolPackageDeltaFilters(packageDir string, resources []ProtocolPackageResource, filters []string) []ProtocolPackageResource {
	if len(filters) == 0 {
		return nil
	}
	enabledByPath := map[string]bool{}
	for _, filter := range filters {
		filter = strings.TrimSpace(filter)
		if filter == "" {
			continue
		}
		patternText := filter
		enabled := true
		exact := false
		switch filter[0] {
		case '-', '!':
			enabled = false
			exact = filter[0] == '-'
			patternText = strings.TrimSpace(filter[1:])
		case '+':
			exact = true
			patternText = strings.TrimSpace(filter[1:])
		}
		for _, resource := range resources {
			matched := protocolPackagePatternMatches(packageDir, resource.Path, patternText)
			if exact {
				matched = protocolPackageExactPatternMatches(packageDir, resource.Path, patternText)
			}
			if matched {
				enabledByPath[filepath.Clean(resource.Path)] = enabled
			}
		}
	}
	result := make([]ProtocolPackageResource, 0, len(enabledByPath))
	for _, resource := range resources {
		enabled, ok := enabledByPath[filepath.Clean(resource.Path)]
		if !ok {
			continue
		}
		resource.Enabled = enabled
		result = append(result, resource)
	}
	return result
}

func applyProtocolPackageProcessFilters(packageDir string, resources []ProtocolPackageProcessExtension, filters []string) []ProtocolPackageProcessExtension {
	if len(filters) == 0 {
		return resources
	}
	filtered := make([]ProtocolPackageProcessExtension, 0, len(resources))
	for _, resource := range resources {
		if protocolPackageProcessEnabled(packageDir, resource, filters) {
			filtered = append(filtered, resource)
		}
	}
	return filtered
}

func protocolPackageProcessEnabledConfigured(packageDir string, resource ProtocolPackageProcessExtension, filters []string, configured bool) bool {
	if configured && len(filters) == 0 {
		return false
	}
	return protocolPackageProcessEnabled(packageDir, resource, filters)
}

func protocolPackageProcessEnabled(packageDir string, resource ProtocolPackageProcessExtension, filters []string) bool {
	hasPositive := false
	for _, filter := range filters {
		filter = strings.TrimSpace(filter)
		if filter != "" && filter[0] != '!' && filter[0] != '-' && filter[0] != '+' {
			hasPositive = true
			break
		}
	}
	enabled := !hasPositive
	for _, filter := range filters {
		filter = strings.TrimSpace(filter)
		if filter == "" {
			continue
		}
		nextEnabled := true
		patternText := filter
		switch filter[0] {
		case '-', '!':
			nextEnabled = false
			patternText = strings.TrimSpace(filter[1:])
		case '+':
			patternText = strings.TrimSpace(filter[1:])
		}
		if protocolPackagePatternMatches(packageDir, resource.Path, patternText) || patternText == resource.ID {
			enabled = nextEnabled
		}
	}
	return enabled
}

func protocolPackageDeltaProcessState(packageDir string, resource ProtocolPackageProcessExtension, filters []string) (bool, bool) {
	var enabled bool
	matched := false
	for _, filter := range filters {
		filter = strings.TrimSpace(filter)
		if filter == "" {
			continue
		}
		patternText := filter
		nextEnabled := true
		exact := false
		switch filter[0] {
		case '-', '!':
			nextEnabled = false
			exact = filter[0] == '-'
			patternText = strings.TrimSpace(filter[1:])
		case '+':
			exact = true
			patternText = strings.TrimSpace(filter[1:])
		}
		pathMatched := protocolPackagePatternMatches(packageDir, resource.Path, patternText)
		if exact {
			pathMatched = protocolPackageExactPatternMatches(packageDir, resource.Path, patternText)
		}
		if pathMatched || patternText == resource.ID {
			enabled = nextEnabled
			matched = true
		}
	}
	return enabled, matched
}

func protocolPackageProcessKey(resource ProtocolPackageProcessExtension) string {
	return filepath.Clean(resource.PackageDir) + "\x00" + resource.ID
}

func filterProtocolPackagePaths(packageDir string, paths []string, excludePatterns []string) []string {
	if len(excludePatterns) == 0 {
		return paths
	}
	var result []string
	for _, candidate := range paths {
		excluded := false
		for _, pattern := range excludePatterns {
			if protocolPackagePatternMatches(packageDir, candidate, pattern) {
				excluded = true
				break
			}
		}
		if !excluded {
			result = append(result, candidate)
		}
	}
	return result
}

func protocolPackagePatternMatches(packageDir, candidate, patternText string) bool {
	patternText = strings.TrimPrefix(strings.TrimSpace(filepath.ToSlash(patternText)), "./")
	if patternText == "" {
		return false
	}
	rel, err := filepath.Rel(packageDir, candidate)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == patternText || strings.HasPrefix(rel, strings.TrimSuffix(patternText, "/")+"/") {
		return true
	}
	if patternText == "**" || patternText == "**/*" {
		return true
	}
	if strings.HasPrefix(patternText, "**/") {
		suffix := strings.TrimPrefix(patternText, "**/")
		if ok, _ := path.Match(suffix, path.Base(rel)); ok {
			return true
		}
		return rel == suffix || strings.HasSuffix(rel, "/"+suffix) || strings.Contains(rel, "/"+strings.TrimSuffix(suffix, "/")+"/")
	}
	if ok, _ := path.Match(patternText, rel); ok {
		return true
	}
	if ok, _ := path.Match(patternText, path.Base(rel)); ok {
		return true
	}
	return false
}

func protocolPackageExactPatternMatches(packageDir, candidate, patternText string) bool {
	patternText = strings.TrimPrefix(strings.TrimSpace(filepath.ToSlash(patternText)), "./")
	if patternText == "" {
		return false
	}
	candidate = filepath.Clean(candidate)
	rel, err := filepath.Rel(filepath.Clean(packageDir), candidate)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if patternText == rel || filepath.Clean(patternText) == candidate {
		return true
	}
	if filepath.Base(candidate) != "SKILL.md" {
		return false
	}
	parent := filepath.Dir(candidate)
	parentRel, err := filepath.Rel(filepath.Clean(packageDir), parent)
	if err != nil {
		return false
	}
	return patternText == filepath.ToSlash(parentRel) || filepath.Clean(patternText) == parent
}

func collectProtocolPackageDir(dir, kind string) []string {
	switch kind {
	case "extensions":
		sources := discoverProtocolExtensionsInDir(dir).Extensions
		paths := make([]string, 0, len(sources))
		for _, source := range sources {
			paths = append(paths, filepath.Clean(source.Path))
		}
		sort.Strings(paths)
		return paths
	case "skills":
		return collectProtocolSkillFiles(dir)
	case "prompts":
		return collectProtocolFilesByExt(dir, ".md")
	case "themes":
		return collectProtocolFilesByExt(dir, ".json")
	default:
		return nil
	}
}

func dedupeProtocolPackagePaths(paths []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(paths))
	for _, rawPath := range paths {
		clean := filepath.Clean(rawPath)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	return result
}

func collectProtocolSkillFiles(root string) []string {
	var paths []string
	info, err := os.Stat(root)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		if filepath.Base(root) == "SKILL.md" {
			return []string{filepath.Clean(root)}
		}
		return nil
	}
	if _, err := os.Stat(filepath.Join(root, "SKILL.md")); err == nil {
		return []string{filepath.Join(root, "SKILL.md")}
	}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != root {
				if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err == nil {
					paths = append(paths, filepath.Join(path, "SKILL.md"))
					return filepath.SkipDir
				}
			}
			return nil
		}
		if filepath.Base(path) == "SKILL.md" {
			paths = append(paths, filepath.Clean(path))
		}
		return nil
	})
	sort.Strings(paths)
	return paths
}

func collectProtocolFilesByExt(root, ext string) []string {
	var paths []string
	info, err := os.Stat(root)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		if strings.EqualFold(filepath.Ext(root), ext) {
			return []string{filepath.Clean(root)}
		}
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			continue
		}
		paths = append(paths, filepath.Join(root, entry.Name()))
	}
	sort.Strings(paths)
	return paths
}

func protocolPackageFileMatchesKind(path, kind string) bool {
	switch kind {
	case "extensions":
		return isProtocolExtensionFile(path)
	case "skills":
		return filepath.Base(path) == "SKILL.md"
	case "prompts":
		return strings.EqualFold(filepath.Ext(path), ".md")
	case "themes":
		return strings.EqualFold(filepath.Ext(path), ".json")
	default:
		return false
	}
}

func protocolPackageResource(path string, metadata ProtocolSourceInfo) ProtocolPackageResource {
	metadata.Path = filepath.Clean(path)
	return ProtocolPackageResource{Path: filepath.Clean(path), Enabled: true, Metadata: metadata}
}

func protocolPackageProcessExtension(packageDir string, metadata ProtocolSourceInfo, extension protocolPackageManifestProcessExtension) ProtocolPackageProcessExtension {
	id := strings.TrimSpace(extension.ID)
	if id == "" || len(extension.Command) == 0 {
		return ProtocolPackageProcessExtension{}
	}
	processPath := filepath.Join(packageDir, "gi.package.json") + "#" + id
	metadata.Path = processPath
	return ProtocolPackageProcessExtension{
		ID:           id,
		Path:         processPath,
		PackageDir:   filepath.Clean(packageDir),
		Command:      append([]string(nil), extension.Command...),
		Transport:    firstNonEmptyString(extension.Transport, "stdio-ndjson"),
		Protocol:     firstNonEmptyString(extension.Protocol, "gi-ext-rpc@1"),
		Capabilities: append([]string(nil), extension.Capabilities...),
		Env:          cloneStringMap(extension.Env),
		Metadata:     metadata,
	}
}

func dedupeProtocolPackageResources(resources []ProtocolPackageResource) []ProtocolPackageResource {
	seen := map[string]struct{}{}
	var result []ProtocolPackageResource
	for _, resource := range resources {
		key := filepath.Clean(resource.Path)
		if realPath, err := filepath.EvalSymlinks(key); err == nil {
			key = realPath
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, resource)
	}
	return result
}

func dedupeProtocolPackageProcessExtensions(resources []ProtocolPackageProcessExtension) []ProtocolPackageProcessExtension {
	seen := map[string]struct{}{}
	result := make([]ProtocolPackageProcessExtension, 0, len(resources))
	for _, resource := range resources {
		key := filepath.Clean(resource.PackageDir) + "\x00" + resource.ID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, resource)
	}
	return result
}

func cleanStringSlice(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}
