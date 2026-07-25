package gicodingagent

import (
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// protocolExtensionLoader owns descriptor caching for one resource loader.
// The cwd and generation token prevent an in-flight read from publishing into
// a newer reload transaction. Parsed descriptors are immutable after decode.
type protocolExtensionLoader struct {
	mu          sync.RWMutex
	cwd         string
	generation  uint64
	descriptors map[string]protocolExtensionDescriptor
}

type protocolExtensionCacheToken struct {
	cwd        string
	generation uint64
}

func newProtocolExtensionLoader() *protocolExtensionLoader {
	return &protocolExtensionLoader{
		descriptors: map[string]protocolExtensionDescriptor{},
	}
}

func (l *protocolExtensionLoader) clearExtensionCache() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.descriptors = map[string]protocolExtensionDescriptor{}
	l.cwd = ""
	l.generation++
	l.mu.Unlock()
}

func (l *protocolExtensionLoader) useExtensionCacheCWD(
	cwd string,
) protocolExtensionCacheToken {
	if l == nil {
		return protocolExtensionCacheToken{}
	}
	resolvedCWD := resolveProtocolExtensionLoadPath(cwd, "")
	l.mu.Lock()
	if l.cwd != "" && l.cwd != resolvedCWD {
		l.descriptors = map[string]protocolExtensionDescriptor{}
		l.generation++
	}
	l.cwd = resolvedCWD
	token := protocolExtensionCacheToken{
		cwd:        resolvedCWD,
		generation: l.generation,
	}
	l.mu.Unlock()
	return token
}

func (l *protocolExtensionLoader) isCurrentCacheToken(
	token protocolExtensionCacheToken,
) bool {
	if l == nil || token.cwd == "" {
		return false
	}
	l.mu.RLock()
	current := l.cwd == token.cwd && l.generation == token.generation
	l.mu.RUnlock()
	return current
}

func (l *protocolExtensionLoader) loadDescriptor(
	path string,
	token *protocolExtensionCacheToken,
) (protocolExtensionDescriptor, error) {
	if l != nil && token != nil && l.isCurrentCacheToken(*token) {
		l.mu.RLock()
		descriptor, ok := l.descriptors[path]
		l.mu.RUnlock()
		if ok {
			return descriptor, nil
		}
	}

	descriptor, err := readProtocolExtensionDescriptor(path)
	if err != nil {
		return protocolExtensionDescriptor{}, err
	}
	if l != nil && token != nil {
		l.mu.Lock()
		if l.cwd == token.cwd && l.generation == token.generation {
			l.descriptors[path] = descriptor
		}
		l.mu.Unlock()
	}
	return descriptor, nil
}

type protocolExtensionConflictIndex struct {
	tools map[string]string
	flags map[string]string
}

func newProtocolExtensionConflictIndex() *protocolExtensionConflictIndex {
	return &protocolExtensionConflictIndex{
		tools: map[string]string{},
		flags: map[string]string{},
	}
}

func (i *protocolExtensionConflictIndex) diagnostics(
	source ProtocolExtensionSource,
	descriptor *protocolExtensionDescriptorGI,
) []ProtocolExtensionDiscoveryError {
	if i == nil || descriptor == nil {
		return nil
	}
	var diagnostics []ProtocolExtensionDiscoveryError
	appendConflict := func(kind, name string, owners map[string]string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if previous := owners[name]; previous != "" && previous != source.Path {
			diagnostics = append(diagnostics, ProtocolExtensionDiscoveryError{
				Path:  source.Path,
				Error: kind + ` "` + name + `" conflicts with ` + previous,
			})
			return
		}
		if owners[name] == "" {
			owners[name] = source.Path
		}
	}
	for _, tool := range descriptor.Tools {
		appendConflict("Tool", tool.Name, i.tools)
	}
	for _, flag := range descriptor.Flags {
		name := normalizeProtocolFlagName(flag.Name)
		if name != "" {
			appendConflict("Flag", "--"+name, i.flags)
		}
	}
	return diagnostics
}

type protocolLoadedExtension struct {
	source     ProtocolExtensionSource
	descriptor protocolExtensionDescriptor
	resources  ResourceExtension
}

// protocolExtensionLoadState is the transaction shared by the pre-trust and
// final load phases. The runtime is mutated only by this transaction; source
// records are immutable snapshots used to rebuild final ordering/resources.
type protocolExtensionLoadState struct {
	loader          *protocolExtensionLoader
	runtime         *ProtocolExtensionRuntime
	cacheToken      *protocolExtensionCacheToken
	loadedByPath    map[string]protocolLoadedExtension
	loadOrder       []string
	failedPaths     map[string]struct{}
	errors          []ProtocolExtensionDiscoveryError
	factoriesLoaded bool
	factorySources  []ProtocolExtensionSource
}

func newProtocolExtensionLoadState(
	loader *protocolExtensionLoader,
	runtime *ProtocolExtensionRuntime,
	cwd string,
	useCache bool,
) *protocolExtensionLoadState {
	if loader == nil {
		loader = newProtocolExtensionLoader()
	}
	if runtime == nil {
		runtime = NewDefaultProtocolExtensionRuntime()
	}
	state := &protocolExtensionLoadState{
		loader:       loader,
		runtime:      runtime,
		loadedByPath: map[string]protocolLoadedExtension{},
		failedPaths:  map[string]struct{}{},
	}
	if useCache {
		token := loader.useExtensionCacheCWD(cwd)
		state.cacheToken = &token
	}
	return state
}

func (s *protocolExtensionLoadState) loadExtensionsInternal(
	sources []ProtocolExtensionSource,
	cwd string,
) protocolExtensionDescriptorLoadResult {
	var result protocolExtensionDescriptorLoadResult
	if s == nil {
		return result
	}
	for _, candidate := range sources {
		source := candidate
		resolvedPath := resolveProtocolExtensionLoadPath(source.Path, cwd)
		if resolvedPath == "" {
			continue
		}
		if _, ok := s.loadedByPath[resolvedPath]; ok {
			continue
		}
		if _, failed := s.failedPaths[resolvedPath]; failed {
			continue
		}
		source.Path = resolveProtocolExtensionSourcePath(source.Path, cwd)
		if source.BaseDir == "" {
			source.BaseDir = filepath.Dir(source.Path)
		}
		descriptor, err := s.loader.loadDescriptor(
			resolvedPath,
			s.cacheToken,
		)
		if err != nil {
			diagnostic := ProtocolExtensionDiscoveryError{
				Path:  candidate.Path,
				Error: err.Error(),
			}
			s.failedPaths[resolvedPath] = struct{}{}
			s.errors = append(s.errors, diagnostic)
			result.Errors = append(result.Errors, diagnostic)
			continue
		}
		if descriptor.Gi.InitError != "" {
			diagnostic := ProtocolExtensionDiscoveryError{
				Path:  candidate.Path,
				Error: descriptor.Gi.InitError,
			}
			s.failedPaths[resolvedPath] = struct{}{}
			s.errors = append(s.errors, diagnostic)
			result.Errors = append(result.Errors, diagnostic)
			continue
		}

		sourceInfo := protocolDescriptorSourceInfo(
			source,
			descriptor.Gi.ID,
		)
		source.Metadata = sourceInfo
		context := &ProtocolExtensionContext{
			runtime: s.runtime,
			source:  source.Metadata,
		}
		applyErrors := applyProtocolExtensionDescriptor(
			context,
			descriptor.Gi,
		)
		resources := protocolDescriptorResources(
			source,
			descriptor.Gi.Resources,
			sourceInfo,
		)
		record := protocolLoadedExtension{
			source:     source,
			descriptor: descriptor,
			resources:  resources,
		}
		s.loadedByPath[resolvedPath] = record
		s.loadOrder = append(s.loadOrder, resolvedPath)
		s.errors = append(s.errors, applyErrors...)
		result.Loaded = append(result.Loaded, source)
		result.Errors = append(result.Errors, applyErrors...)
		appendResourceExtension(&result.Resources, resources)
	}
	return result
}

func (s *protocolExtensionLoadState) loadFactories(
	factories []ProtocolExtensionFactory,
) {
	if s == nil || s.factoriesLoaded {
		return
	}
	s.factoriesLoaded = true
	for index, input := range factories {
		factory := normalizeProtocolExtensionFactory(input, index)
		if factory.Factory == nil {
			continue
		}
		sourceInfo := protocolInlineFactorySourceInfo(factory)
		if err := s.runtime.LoadFactories(
			[]ProtocolExtensionFactory{factory},
		); err != nil {
			s.runtime.RemoveSource(sourceInfo)
			s.errors = append(s.errors, ProtocolExtensionDiscoveryError{
				Path:  factory.Path,
				Error: err.Error(),
			})
			continue
		}
		s.factorySources = append(
			s.factorySources,
			ProtocolExtensionSource{
				Path:     factory.Path,
				Metadata: sourceInfo,
				Hidden:   factory.Hidden,
			},
		)
	}
}

func normalizeProtocolExtensionFactory(
	factory ProtocolExtensionFactory,
	index int,
) ProtocolExtensionFactory {
	if strings.TrimSpace(factory.Path) != "" {
		return factory
	}
	name := strings.TrimSpace(factory.Name)
	if name == "" {
		name = strconv.Itoa(index + 1)
	}
	factory.Path = "<inline:" + name + ">"
	return factory
}

func protocolInlineFactorySourceInfo(
	factory ProtocolExtensionFactory,
) ProtocolSourceInfo {
	return ProtocolSourceInfo{
		Path:   factory.Path,
		Source: "inline",
		Scope:  "temporary",
		Origin: "top-level",
	}
}

func (s *protocolExtensionLoadState) retainSources(
	sources []ProtocolExtensionSource,
	cwd string,
) {
	if s == nil {
		return
	}
	retained := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		if path := resolveProtocolExtensionLoadPath(source.Path, cwd); path != "" {
			retained[path] = struct{}{}
		}
	}
	order := s.loadOrder[:0]
	for _, path := range s.loadOrder {
		record, ok := s.loadedByPath[path]
		if !ok {
			continue
		}
		if _, keep := retained[path]; !keep {
			s.runtime.RemoveSource(record.source.Metadata)
			delete(s.loadedByPath, path)
			continue
		}
		order = append(order, path)
	}
	s.loadOrder = order
}

func (s *protocolExtensionLoadState) conflictDiagnostics(
	sources []ProtocolExtensionSource,
	cwd string,
) []ProtocolExtensionDiscoveryError {
	if s == nil {
		return nil
	}
	conflicts := newProtocolExtensionConflictIndex()
	var diagnostics []ProtocolExtensionDiscoveryError
	for _, source := range sources {
		path := resolveProtocolExtensionLoadPath(source.Path, cwd)
		record, ok := s.loadedByPath[path]
		if !ok {
			continue
		}
		diagnostics = append(
			diagnostics,
			conflicts.diagnostics(
				record.source,
				record.descriptor.Gi,
			)...,
		)
	}
	return diagnostics
}

func (s *protocolExtensionLoadState) orderedSources(
	sources []ProtocolExtensionSource,
	cwd string,
) []ProtocolExtensionSource {
	if s == nil {
		return nil
	}
	ordered := make(
		[]ProtocolExtensionSource,
		0,
		len(sources)+len(s.factorySources),
	)
	for _, source := range sources {
		path := resolveProtocolExtensionLoadPath(source.Path, cwd)
		record, ok := s.loadedByPath[path]
		if !ok {
			continue
		}
		ordered = append(ordered, record.source)
	}
	ordered = append(ordered, s.factorySources...)
	return ordered
}

func (s *protocolExtensionLoadState) orderedResources(
	sources []ProtocolExtensionSource,
	cwd string,
) ResourceExtension {
	var resources ResourceExtension
	if s == nil {
		return resources
	}
	for _, source := range sources {
		path := resolveProtocolExtensionLoadPath(source.Path, cwd)
		record, ok := s.loadedByPath[path]
		if !ok {
			continue
		}
		appendResourceExtension(&resources, record.resources)
	}
	return resources
}

func (s *protocolExtensionLoadState) diagnostics() []ProtocolExtensionDiscoveryError {
	if s == nil {
		return nil
	}
	return append([]ProtocolExtensionDiscoveryError(nil), s.errors...)
}

func (s *protocolExtensionLoadState) orderedSourceInfo(
	sources []ProtocolExtensionSource,
	cwd string,
) []ProtocolSourceInfo {
	if s == nil {
		return nil
	}
	ordered := make([]ProtocolSourceInfo, 0, len(sources)+len(s.factorySources))
	for _, source := range sources {
		path := resolveProtocolExtensionLoadPath(source.Path, cwd)
		record, ok := s.loadedByPath[path]
		if ok {
			ordered = append(ordered, record.source.Metadata)
		}
	}
	for _, source := range s.factorySources {
		ordered = append(ordered, source.Metadata)
	}
	return ordered
}

func protocolDescriptorResources(
	source ProtocolExtensionSource,
	resources protocolResourceDescriptor,
	metadata ProtocolSourceInfo,
) ResourceExtension {
	return ResourceExtension{
		SkillPaths: protocolDescriptorResourcePaths(
			source,
			resources.Skills,
			metadata,
		),
		PromptPaths: protocolDescriptorPromptPaths(
			source,
			resources.Prompts,
			metadata,
		),
		ThemePaths: protocolDescriptorThemePaths(
			source,
			resources.Themes,
			metadata,
		),
	}
}

func appendResourceExtension(target *ResourceExtension, source ResourceExtension) {
	if target == nil {
		return
	}
	target.ExtensionPaths = append(
		target.ExtensionPaths,
		source.ExtensionPaths...,
	)
	target.SkillPaths = append(target.SkillPaths, source.SkillPaths...)
	target.PromptPaths = append(target.PromptPaths, source.PromptPaths...)
	target.ThemePaths = append(target.ThemePaths, source.ThemePaths...)
}

func resolveProtocolExtensionLoadPath(path, cwd string) string {
	resolved := resolveProtocolExtensionSourcePath(path, cwd)
	if resolved == "" {
		return ""
	}
	if realPath, err := filepath.EvalSymlinks(resolved); err == nil {
		return filepath.Clean(realPath)
	}
	return resolved
}

func resolveProtocolExtensionSourcePath(path, cwd string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	resolved := ResolveToCwd(path, cwd)
	if absolute, err := filepath.Abs(resolved); err == nil {
		resolved = absolute
	}
	return filepath.Clean(resolved)
}
