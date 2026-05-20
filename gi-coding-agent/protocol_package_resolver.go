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
	Extensions []ProtocolPackageResource
	Skills     []ProtocolPackageResource
	Prompts    []ProtocolPackageResource
	Themes     []ProtocolPackageResource
}

type ProtocolPackageSourceSpec struct {
	Source  string
	Scope   string
	Filters ProtocolPackageResourceFilters
}

type ProtocolPackageResourceFilters struct {
	Extensions []string
	Skills     []string
	Prompts    []string
	Themes     []string
}

type protocolPackageManifest struct {
	Extensions []string
	Skills     []string
	Prompts    []string
	Themes     []string
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
	for _, spec := range specs {
		resolved, packageDir, err := m.resolveProtocolPackageSource(spec.Source, spec.Scope)
		if err != nil {
			return ProtocolPackageResources{}, err
		}
		if packageDir == "" {
			continue
		}
		result.Extensions = append(result.Extensions, applyProtocolPackageFilters(packageDir, resolved.Extensions, spec.Filters.Extensions)...)
		result.Skills = append(result.Skills, applyProtocolPackageFilters(packageDir, resolved.Skills, spec.Filters.Skills)...)
		result.Prompts = append(result.Prompts, applyProtocolPackageFilters(packageDir, resolved.Prompts, spec.Filters.Prompts)...)
		result.Themes = append(result.Themes, applyProtocolPackageFilters(packageDir, resolved.Themes, spec.Filters.Themes)...)
	}
	result.Extensions = dedupeProtocolPackageResources(result.Extensions)
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
	type keyedSpec struct {
		key  string
		spec ProtocolPackageSourceSpec
	}
	var order []keyedSpec
	index := map[string]int{}
	addSpecs := func(values []any, scope, baseDir string) {
		for _, value := range values {
			spec, ok := protocolPackageSourceSpecFromSettings(value, scope)
			if !ok {
				continue
			}
			key := protocolPackageSettingsIdentity(spec.Source, baseDir)
			if existing, ok := index[key]; ok {
				order[existing].spec = spec
				continue
			}
			index[key] = len(order)
			order = append(order, keyedSpec{key: key, spec: spec})
		}
	}
	addSpecs(settingsSlice(m.settingsManager.global, "packages"), "user", m.agentDir)
	addSpecs(settingsSlice(m.settingsManager.project, "packages"), "project", filepath.Join(m.cwd, ConfigDirName))
	specs := make([]ProtocolPackageSourceSpec, 0, len(order))
	for _, item := range order {
		specs = append(specs, item.spec)
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
	return ProtocolPackageSourceSpec{
		Source: strings.TrimSpace(source),
		Scope:  scope,
		Filters: ProtocolPackageResourceFilters{
			Extensions: settingsStringSlice(object, "extensions"),
			Skills:     settingsStringSlice(object, "skills"),
			Prompts:    settingsStringSlice(object, "prompts"),
			Themes:     settingsStringSlice(object, "themes"),
		},
	}, true
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
	sourcePath := ResolveToCwd(sourceText, m.cwd)
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
	if manifest, ok := readProtocolPackageManifestFile(filepath.Join(packageDir, "gi.package.json")); ok {
		return manifest, true
	}
	return readProtocolPackageManifestFile(filepath.Join(packageDir, "package.json"))
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
		Extensions: protocolManifestStringList(raw["extensions"]),
		Skills:     protocolManifestStringList(raw["skills"]),
		Prompts:    protocolManifestStringList(raw["prompts"]),
		Themes:     protocolManifestStringList(raw["themes"]),
	}
	return manifest, len(manifest.Extensions)+len(manifest.Skills)+len(manifest.Prompts)+len(manifest.Themes) > 0
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

func resolveProtocolPackageManifest(packageDir string, metadata ProtocolSourceInfo, manifest protocolPackageManifest) ProtocolPackageResources {
	var result ProtocolPackageResources
	for _, path := range resolveProtocolPackageEntries(packageDir, manifest.Extensions, "extensions") {
		result.Extensions = append(result.Extensions, protocolPackageResource(path, metadata))
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
