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

type protocolPackageManifest struct {
	Extensions []string
	Skills     []string
	Prompts    []string
	Themes     []string
}

func (m *DefaultPackageManager) ResolveProtocolPackageResources(sources []string) (ProtocolPackageResources, error) {
	var result ProtocolPackageResources
	for _, sourceText := range sources {
		sourceText = strings.TrimSpace(sourceText)
		if sourceText == "" {
			continue
		}
		sourcePath := ResolveToCwd(sourceText, m.cwd)
		info, err := os.Stat(sourcePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return ProtocolPackageResources{}, err
		}
		metadata := ProtocolSourceInfo{
			Source: m.GetPackageIdentity(sourceText),
			Scope:  "temporary",
			Origin: "package",
		}
		if !info.IsDir() {
			if isProtocolExtensionFile(sourcePath) {
				resource := protocolPackageResource(sourcePath, metadata)
				result.Extensions = append(result.Extensions, resource)
			}
			continue
		}
		resolved := resolveProtocolPackageDir(sourcePath, metadata)
		result.Extensions = append(result.Extensions, resolved.Extensions...)
		result.Skills = append(result.Skills, resolved.Skills...)
		result.Prompts = append(result.Prompts, resolved.Prompts...)
		result.Themes = append(result.Themes, resolved.Themes...)
	}
	result.Extensions = dedupeProtocolPackageResources(result.Extensions)
	result.Skills = dedupeProtocolPackageResources(result.Skills)
	result.Prompts = dedupeProtocolPackageResources(result.Prompts)
	result.Themes = dedupeProtocolPackageResources(result.Themes)
	return result, nil
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
