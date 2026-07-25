package gicodingagent

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ResourceConfigWriteScope identifies which settings layer the resource
// selector mutates. The project view is an effective projection: it includes
// inherited user resources plus project-local resources and overrides.
type ResourceConfigWriteScope string

const (
	ResourceConfigWriteGlobal  ResourceConfigWriteScope = "global"
	ResourceConfigWriteProject ResourceConfigWriteScope = "project"
)

// ProjectResourceOverrideState is the persisted project-layer decision for one
// resource. Inherit delegates to the user layer; load and unload are explicit
// project overrides.
type ProjectResourceOverrideState string

const (
	ProjectResourceInherit ProjectResourceOverrideState = "inherit"
	ProjectResourceLoad    ProjectResourceOverrideState = "load"
	ProjectResourceUnload  ProjectResourceOverrideState = "unload"
)

// ResourceConfigSnapshot is an immutable pair of resource projections. The
// selector owns presentation state; SettingsManager remains the only owner of
// persisted configuration.
type ResourceConfigSnapshot struct {
	global           []PackageResourceToggleItem
	project          []PackageResourceToggleItem
	inheritedEnabled map[string]bool
}

// ResourceConfigTarget binds a resolved resource to the inherited state from
// the snapshot that displayed it. Project mutations accept this value so a UI
// cannot accidentally calculate path/source encoding from unrelated state.
type ResourceConfigTarget struct {
	item             PackageResourceToggleItem
	inherited        bool
	inheritedEnabled bool
}

func newResourceConfigSnapshot(global, project []PackageResourceToggleItem) ResourceConfigSnapshot {
	snapshot := ResourceConfigSnapshot{
		global:           clonePackageResourceToggleItems(global),
		project:          clonePackageResourceToggleItems(project),
		inheritedEnabled: make(map[string]bool, len(global)),
	}
	for _, item := range snapshot.global {
		snapshot.inheritedEnabled[resourceConfigItemKey(item)] = item.Enabled
	}
	return snapshot
}

// Items returns a copy of the selected projection so UI updates cannot mutate
// the canonical snapshot.
func (s ResourceConfigSnapshot) Items(scope ResourceConfigWriteScope) []PackageResourceToggleItem {
	if scope == ResourceConfigWriteProject {
		return clonePackageResourceToggleItems(s.project)
	}
	return clonePackageResourceToggleItems(s.global)
}

// InheritedEnabled returns the effective user-layer state for item. The
// boolean result distinguishes a matched global resource from the fallback
// used for project-only resources.
func (s ResourceConfigSnapshot) InheritedEnabled(item PackageResourceToggleItem) (bool, bool) {
	enabled, ok := s.inheritedEnabled[resourceConfigItemKey(item)]
	return enabled, ok
}

// IsInherited reports whether item participates in the user-layer projection.
func (s ResourceConfigSnapshot) IsInherited(item PackageResourceToggleItem) bool {
	if resourceConfigItemScope(item) == "user" {
		return true
	}
	_, ok := s.InheritedEnabled(item)
	return ok
}

// Target returns the canonical mutation input for an item in this snapshot.
func (s ResourceConfigSnapshot) Target(item PackageResourceToggleItem) ResourceConfigTarget {
	inheritedEnabled, matched := s.InheritedEnabled(item)
	inherited := resourceConfigItemScope(item) == "user" || matched
	if !matched {
		if resourceConfigItemScope(item) == "user" {
			inheritedEnabled = item.Enabled
		} else {
			inheritedEnabled = true
		}
	}
	return ResourceConfigTarget{
		item:             item,
		inherited:        inherited,
		inheritedEnabled: inheritedEnabled,
	}
}

// Resource returns a copy of the resolved resource represented by target.
func (t ResourceConfigTarget) Resource() PackageResourceToggleItem {
	return t.item
}

// IsInherited reports whether target has a matching user-layer resource.
func (t ResourceConfigTarget) IsInherited() bool {
	return t.inherited
}

// InheritedEnabled returns target's effective user-layer enabled state.
func (t ResourceConfigTarget) InheritedEnabled() bool {
	return t.inheritedEnabled
}

// LoadResourceConfigSnapshot resolves an isolated user-only projection and the
// effective trusted-project projection from the same settings snapshot.
func (m *DefaultPackageManager) LoadResourceConfigSnapshot() (ResourceConfigSnapshot, error) {
	if m == nil || m.settingsManager == nil {
		return ResourceConfigSnapshot{}, nil
	}
	globalSettings := NewInMemorySettingsManager(m.settingsManager.GetGlobalSettings())
	globalSettings.SetProjectTrusted(false)
	globalManager := NewDefaultPackageManager(PackageManagerOptions{
		CWD:             m.cwd,
		AgentDir:        m.agentDir,
		SettingsManager: globalSettings,
	})
	global, err := globalManager.ListResourceToggles()
	if err != nil {
		return ResourceConfigSnapshot{}, err
	}
	project := global
	if m.settingsManager.IsProjectTrusted() {
		project, err = m.ListResourceToggles()
		if err != nil {
			return ResourceConfigSnapshot{}, err
		}
	}
	return newResourceConfigSnapshot(global, project), nil
}

func clonePackageResourceToggleItems(items []PackageResourceToggleItem) []PackageResourceToggleItem {
	return append([]PackageResourceToggleItem(nil), items...)
}

func resourceConfigItemKey(item PackageResourceToggleItem) string {
	return item.ResourceType + "\x00" + CanonicalizePath(item.Path)
}

func resourceConfigItemScope(item PackageResourceToggleItem) string {
	if item.Scope == "project" || item.Metadata.Scope == "project" {
		return "project"
	}
	return "user"
}

// NextProjectResourceOverrideState implements Pi's asymmetric three-state
// cycle. The order always makes the first key press change effective behavior.
func NextProjectResourceOverrideState(current ProjectResourceOverrideState, inheritedEnabled bool) ProjectResourceOverrideState {
	switch current {
	case ProjectResourceInherit:
		if inheritedEnabled {
			return ProjectResourceUnload
		}
		return ProjectResourceLoad
	case ProjectResourceUnload:
		if inheritedEnabled {
			return ProjectResourceLoad
		}
		return ProjectResourceInherit
	default:
		if inheritedEnabled {
			return ProjectResourceInherit
		}
		return ProjectResourceUnload
	}
}

// ProjectResourceOverrideState returns the explicit project-layer state for
// item without folding in inherited enabled state.
func (m *DefaultPackageManager) ProjectResourceOverrideState(item PackageResourceToggleItem) ProjectResourceOverrideState {
	if m == nil || m.settingsManager == nil {
		return ProjectResourceInherit
	}
	if item.Metadata.Origin == "top-level" {
		entries := settingsStringSlice(m.settingsManager.GetProjectSettings(), item.ResourceType)
		return projectOverrideStateFromEntries(entries, m.topLevelOverridePatterns(item, "project"), false)
	}
	_, value, ok := m.findMatchingPackageSetting(item, "project")
	if !ok {
		return ProjectResourceInherit
	}
	object, ok := value.(map[string]any)
	if !ok {
		return ProjectResourceInherit
	}
	if _, configured := object[item.ResourceType]; !configured {
		return ProjectResourceInherit
	}
	autoload, autoloadSet := object["autoload"].(bool)
	emptyArrayIsUnload := !autoloadSet || autoload
	return projectOverrideStateFromEntries(
		settingsStringSlice(object, item.ResourceType),
		map[string]struct{}{normalizeResourcePattern(item.Pattern): {}},
		emptyArrayIsUnload,
	)
}

func projectOverrideStateFromEntries(entries []string, patterns map[string]struct{}, emptyArrayIsUnload bool) ProjectResourceOverrideState {
	if len(entries) == 0 && emptyArrayIsUnload {
		return ProjectResourceUnload
	}
	state := ProjectResourceInherit
	for _, entry := range entries {
		if _, ok := patterns[normalizeResourcePattern(resourcePatternEntryTarget(entry))]; !ok {
			continue
		}
		entry = strings.TrimSpace(entry)
		if strings.HasPrefix(entry, "!") || strings.HasPrefix(entry, "-") {
			state = ProjectResourceUnload
		} else {
			state = ProjectResourceLoad
		}
	}
	return state
}

func resourcePatternEntryTarget(entry string) string {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return ""
	}
	if strings.ContainsAny(entry[:1], "!+-") {
		return strings.TrimSpace(entry[1:])
	}
	return entry
}

func normalizeResourcePattern(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(pattern)))
}

// SetProjectResourceOverride persists one project-layer transition. target
// carries inherited state from the ResourceConfigSnapshot that displayed it.
func (m *DefaultPackageManager) SetProjectResourceOverride(target ResourceConfigTarget, state ProjectResourceOverrideState) (bool, error) {
	if m == nil || m.settingsManager == nil {
		return false, nil
	}
	item := target.item
	if state != ProjectResourceInherit && state != ProjectResourceLoad && state != ProjectResourceUnload {
		return false, fmt.Errorf("unsupported project resource override state %q", state)
	}
	if !validPackageResourceType(item.ResourceType) {
		return false, fmt.Errorf("unsupported resource type %q", item.ResourceType)
	}
	if err := m.assertProjectTrustedForScope("project"); err != nil {
		return false, err
	}
	if item.Metadata.Origin == "top-level" {
		return m.setProjectTopLevelResourceOverride(item, state, target.inherited)
	}
	return m.setProjectPackageResourceOverride(item, state)
}

func (m *DefaultPackageManager) setProjectTopLevelResourceOverride(item PackageResourceToggleItem, state ProjectResourceOverrideState, inherited bool) (bool, error) {
	current := settingsStringSlice(m.settingsManager.GetProjectSettings(), item.ResourceType)
	pattern := m.resourcePatternForScope(item, "project")
	if inherited {
		pattern = filepath.Clean(item.Path)
	}
	overridePatterns := m.topLevelOverridePatterns(item, "project")
	updated := make([]string, 0, len(current)+2)
	for _, entry := range current {
		target := normalizeResourcePattern(resourcePatternEntryTarget(entry))
		_, isOverrideTarget := overridePatterns[target]
		prefixed := strings.HasPrefix(strings.TrimSpace(entry), "!") ||
			strings.HasPrefix(strings.TrimSpace(entry), "+") ||
			strings.HasPrefix(strings.TrimSpace(entry), "-")
		if prefixed && isOverrideTarget {
			continue
		}
		if state == ProjectResourceInherit && inherited && target == normalizeResourcePattern(pattern) {
			continue
		}
		updated = append(updated, entry)
	}
	if state != ProjectResourceInherit {
		if inherited && !resourcePatternsContain(updated, pattern) {
			updated = append(updated, pattern)
		}
		prefix := "-"
		if state == ProjectResourceLoad {
			prefix = "+"
		}
		updated = append(updated, prefix+pattern)
	}
	if err := m.settingsManager.setProject(item.ResourceType, stringSliceToAny(updated)); err != nil {
		return false, err
	}
	return true, nil
}

func resourcePatternsContain(entries []string, target string) bool {
	target = normalizeResourcePattern(target)
	for _, entry := range entries {
		if normalizeResourcePattern(entry) == target {
			return true
		}
	}
	return false
}

func (m *DefaultPackageManager) topLevelOverridePatterns(item PackageResourceToggleItem, scope string) map[string]struct{} {
	baseDir := m.settingsBaseDir(scope == "project")
	patterns := []string{
		m.resourcePatternForScope(item, scope),
		item.Pattern,
		item.Path,
	}
	if relative, err := filepath.Rel(baseDir, item.Path); err == nil {
		patterns = append(patterns, relative)
	}
	result := make(map[string]struct{}, len(patterns))
	for _, pattern := range patterns {
		if normalized := normalizeResourcePattern(pattern); normalized != "" {
			result[normalized] = struct{}{}
		}
	}
	return result
}

func (m *DefaultPackageManager) resourcePatternForScope(item PackageResourceToggleItem, scope string) string {
	if resourceConfigItemScope(item) != scope {
		return filepath.Clean(item.Path)
	}
	if strings.TrimSpace(item.Pattern) != "" {
		return item.Pattern
	}
	baseDir := m.settingsBaseDir(scope == "project")
	relative, err := filepath.Rel(baseDir, item.Path)
	if err != nil {
		return filepath.Clean(item.Path)
	}
	return relative
}

func (m *DefaultPackageManager) setProjectPackageResourceOverride(item PackageResourceToggleItem, state ProjectResourceOverrideState) (bool, error) {
	packages := settingsSlice(m.settingsManager.GetProjectSettings(), "packages")
	index, value, found := m.findMatchingPackageSettingIn(packages, item, "project")
	if !found {
		if state == ProjectResourceInherit {
			return false, nil
		}
		value = m.createPackageOverrideSetting(item)
		packages = append(packages, value)
		index = len(packages) - 1
	}
	object, source := packageSettingObject(value, item.Source)
	pattern := normalizeResourcePattern(item.Pattern)
	current := settingsStringSlice(object, item.ResourceType)
	updated := make([]string, 0, len(current)+1)
	for _, entry := range current {
		if normalizeResourcePattern(resourcePatternEntryTarget(entry)) != pattern {
			updated = append(updated, entry)
		}
	}
	if state != ProjectResourceInherit {
		prefix := "-"
		if state == ProjectResourceLoad {
			prefix = "+"
		}
		updated = append(updated, prefix+item.Pattern)
	}
	if len(updated) == 0 {
		delete(object, item.ResourceType)
	} else {
		object[item.ResourceType] = stringSliceToAny(updated)
	}
	if !packageSettingObjectHasResourceKeys(object) {
		autoload, autoloadSet := object["autoload"].(bool)
		if autoloadSet && !autoload {
			packages = append(packages[:index], packages[index+1:]...)
		} else {
			packages[index] = source
		}
	} else {
		packages[index] = object
	}
	if err := m.settingsManager.SetProjectPackages(packages); err != nil {
		return false, err
	}
	return true, nil
}

func packageSettingObjectHasResourceKeys(object map[string]any) bool {
	for _, resourceType := range []string{"extensions", "skills", "prompts", "themes"} {
		if _, ok := object[resourceType]; ok {
			return true
		}
	}
	return false
}

func (m *DefaultPackageManager) createPackageOverrideSetting(item PackageResourceToggleItem) map[string]any {
	source := item.Source
	if ParsePackageSource(source).Type == "local" {
		sourcePath := ResolveToCwd(source, m.settingsBaseDir(resourceConfigItemScope(item) == "project"))
		if relative, err := filepath.Rel(m.settingsBaseDir(true), sourcePath); err == nil {
			source = relative
			if source == "" {
				source = "."
			}
		} else {
			source = sourcePath
		}
	}
	return map[string]any{"source": source, "autoload": false}
}

func (m *DefaultPackageManager) findMatchingPackageSetting(item PackageResourceToggleItem, scope string) (int, any, bool) {
	settings := m.settingsManager.GetGlobalSettings()
	if scope == "project" {
		settings = m.settingsManager.GetProjectSettings()
	}
	return m.findMatchingPackageSettingIn(settingsSlice(settings, "packages"), item, scope)
}

func (m *DefaultPackageManager) findMatchingPackageSettingIn(packages []any, item PackageResourceToggleItem, scope string) (int, any, bool) {
	for index, value := range packages {
		spec, ok := protocolPackageSourceSpecFromSettings(value, scope)
		if !ok {
			continue
		}
		if m.packageSourceStringsMatch(item.Source, resourceConfigItemScope(item), spec.Source, scope) {
			return index, value, true
		}
	}
	return -1, nil, false
}

func (m *DefaultPackageManager) packageSourceStringsMatch(leftSource, leftScope, rightSource, rightScope string) bool {
	leftSource = strings.TrimSpace(leftSource)
	rightSource = strings.TrimSpace(rightSource)
	if leftSource == rightSource {
		return true
	}
	if ParsePackageSource(leftSource).Type != "local" || ParsePackageSource(rightSource).Type != "local" {
		return false
	}
	left := packageLocalIdentity(leftSource, m.settingsBaseDir(leftScope == "project"))
	right := packageLocalIdentity(rightSource, m.settingsBaseDir(rightScope == "project"))
	return left == right
}
