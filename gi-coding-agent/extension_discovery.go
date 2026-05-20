package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ProtocolExtensionSource struct {
	Path    string
	BaseDir string
}

type ProtocolExtensionDiscoveryError struct {
	Path  string
	Error string
}

type ProtocolExtensionDiscoveryResult struct {
	Extensions []ProtocolExtensionSource
	Errors     []ProtocolExtensionDiscoveryError
}

func DiscoverProtocolExtensions(explicitPaths []string, cwd, agentDir string) ProtocolExtensionDiscoveryResult {
	result := LoadProtocolExtensionSources(explicitPaths, cwd)
	for _, dir := range defaultProtocolExtensionDirs(cwd, agentDir) {
		discovered := discoverProtocolExtensionsInDir(dir)
		result.Extensions = append(result.Extensions, discovered.Extensions...)
		result.Errors = append(result.Errors, discovered.Errors...)
	}
	result.Extensions = dedupeProtocolExtensionSources(result.Extensions)
	return result
}

func LoadProtocolExtensionSources(paths []string, cwd string) ProtocolExtensionDiscoveryResult {
	var result ProtocolExtensionDiscoveryResult
	for _, rawPath := range paths {
		for _, source := range resolveProtocolExtensionPath(rawPath, cwd) {
			result.Extensions = append(result.Extensions, source)
		}
	}
	result.Extensions = dedupeProtocolExtensionSources(result.Extensions)
	return result
}

func defaultProtocolExtensionDirs(cwd, agentDir string) []string {
	var dirs []string
	if agentDir != "" {
		dirs = append(dirs, filepath.Join(agentDir, "extensions"))
	}
	if cwd != "" {
		projectDir := filepath.Join(cwd, ".pi", "extensions")
		if filepath.Clean(projectDir) != filepath.Clean(filepath.Join(agentDir, "extensions")) {
			dirs = append(dirs, projectDir)
		}
	}
	return dirs
}

func discoverProtocolExtensionsInDir(dir string) ProtocolExtensionDiscoveryResult {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return ProtocolExtensionDiscoveryResult{}
		}
		return ProtocolExtensionDiscoveryResult{Errors: []ProtocolExtensionDiscoveryError{{Path: dir, Error: err.Error()}}}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var result ProtocolExtensionDiscoveryResult
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" {
			continue
		}
		fullPath := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			result.Errors = append(result.Errors, ProtocolExtensionDiscoveryError{Path: fullPath, Error: err.Error()})
			continue
		}
		if info.Mode().IsRegular() && isProtocolExtensionFile(fullPath) {
			result.Extensions = append(result.Extensions, ProtocolExtensionSource{Path: fullPath, BaseDir: dir})
			continue
		}
		if info.IsDir() {
			result.Extensions = append(result.Extensions, resolveProtocolExtensionDir(fullPath)...)
		}
	}
	return result
}

func resolveProtocolExtensionPath(rawPath, cwd string) []ProtocolExtensionSource {
	path := ResolveToCwd(rawPath, cwd)
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if info.IsDir() {
		return resolveProtocolExtensionDir(path)
	}
	if info.Mode().IsRegular() && isProtocolExtensionFile(path) {
		return []ProtocolExtensionSource{{Path: filepath.Clean(path), BaseDir: filepath.Dir(path)}}
	}
	return nil
}

func resolveProtocolExtensionDir(dir string) []ProtocolExtensionSource {
	if manifestSources := protocolExtensionManifestSources(dir); len(manifestSources) > 0 {
		return manifestSources
	}
	for _, name := range []string{"index.gi.json"} {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return []ProtocolExtensionSource{{Path: filepath.Clean(path), BaseDir: dir}}
		}
	}
	return nil
}

func protocolExtensionManifestSources(dir string) []ProtocolExtensionSource {
	content, err := os.ReadFile(filepath.Join(dir, "gi.package.json"))
	if err != nil {
		return nil
	}
	var manifest struct {
		Gi *struct {
			Extensions []string `json:"extensions"`
		} `json:"gi"`
	}
	if err := json.Unmarshal(content, &manifest); err != nil || manifest.Gi == nil {
		return nil
	}
	var sources []ProtocolExtensionSource
	for _, entry := range manifest.Gi.Extensions {
		path := ResolveToCwd(entry, dir)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || !isProtocolExtensionFile(path) {
			continue
		}
		sources = append(sources, ProtocolExtensionSource{Path: filepath.Clean(path), BaseDir: dir})
	}
	return sources
}

func isProtocolExtensionFile(path string) bool {
	return strings.HasSuffix(strings.ToLower(filepath.Base(path)), ".gi.json")
}

func dedupeProtocolExtensionSources(sources []ProtocolExtensionSource) []ProtocolExtensionSource {
	seen := map[string]struct{}{}
	result := make([]ProtocolExtensionSource, 0, len(sources))
	for _, source := range sources {
		clean := filepath.Clean(source.Path)
		realPath, err := filepath.EvalSymlinks(clean)
		key := clean
		if err == nil {
			key = realPath
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		source.Path = clean
		result = append(result, source)
	}
	return result
}
