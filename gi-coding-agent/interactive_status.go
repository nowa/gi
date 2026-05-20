package gicodingagent

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type InteractiveIndexedTextContainer interface {
	InteractiveContainer
	Len() int
	SetTextAt(index int, text string)
}

type InteractiveStatusChat struct {
	Children []string
}

func (c *InteractiveStatusChat) Clear() {
	c.Children = nil
}

func (c *InteractiveStatusChat) AddText(text string) {
	c.Children = append(c.Children, text)
}

func (c *InteractiveStatusChat) Len() int {
	return len(c.Children)
}

func (c *InteractiveStatusChat) SetTextAt(index int, text string) {
	if index >= 0 && index < len(c.Children) {
		c.Children[index] = text
	}
}

func (c *InteractiveStatusChat) Render() string {
	return strings.Join(c.Children, "\n")
}

type InteractiveExpandable interface {
	SetExpanded(expanded bool)
}

type InteractiveThemeSettings interface {
	GetTheme() string
	SetTheme(theme string)
}

type InteractiveExtensionUIContext struct {
	mode *InteractiveMode
}

type InteractiveThemeResult struct {
	Success bool
}

type AutocompleteProvider interface {
	ShouldTriggerFileCompletion(lines []string, cursorLine, cursorCol int) bool
}

type AutocompleteProviderFactory func(AutocompleteProvider) AutocompleteProvider

type AutocompleteEditor interface {
	SetAutocompleteProvider(AutocompleteProvider)
}

type defaultAutocompleteProvider struct{}

func (defaultAutocompleteProvider) ShouldTriggerFileCompletion([]string, int, int) bool {
	return true
}

type InteractiveSourceInfo struct {
	Path    string
	Source  string
	Scope   string
	Origin  string
	BaseDir string
}

type InteractiveExtensionResource struct {
	Path       string
	SourceInfo *InteractiveSourceInfo
}

type InteractiveSkillResource struct {
	FilePath string
	Name     string
}

type InteractiveResourceDiagnostic struct {
	Type    string
	Message string
}

type InteractiveContextFile struct {
	Path    string
	Content string
}

type InteractiveLoadedResources struct {
	QuietStartup       bool
	Verbose            bool
	ToolOutputExpanded bool
	CWD                string
	ContextFiles       []InteractiveContextFile
	Extensions         []InteractiveExtensionResource
	Skills             []InteractiveSkillResource
	SkillDiagnostics   []InteractiveResourceDiagnostic
}

type InteractiveShowLoadedResourcesOptions struct {
	Force                    bool
	ShowDiagnosticsWhenQuiet bool
}

func (m *InteractiveMode) ShowStatusMessage(message string) {
	if m == nil {
		return
	}
	if chat, ok := m.Chat.(InteractiveIndexedTextContainer); ok {
		if m.lastStatusValid && m.lastStatusIndex == chat.Len()-1 {
			chat.SetTextAt(m.lastStatusIndex, message)
		} else {
			chat.AddText("")
			chat.AddText(message)
			m.lastStatusIndex = chat.Len() - 1
			m.lastStatusValid = true
		}
	} else if m.Chat != nil {
		m.Chat.AddText(message)
	}
	if m.UI != nil {
		m.UI.RequestRender()
	}
}

func (m *InteractiveMode) SetToolsExpanded(expanded bool) {
	if m == nil {
		return
	}
	m.ToolOutputExpanded = expanded
	header := m.CustomHeader
	if header == nil {
		header = m.BuiltInHeader
	}
	if header != nil {
		header.SetExpanded(expanded)
	}
	for _, entry := range m.ChatExpandables {
		if entry != nil {
			entry.SetExpanded(expanded)
		}
	}
	if m.UI != nil {
		m.UI.RequestRender()
	}
}

func (m *InteractiveMode) CreateExtensionUIContext() InteractiveExtensionUIContext {
	return InteractiveExtensionUIContext{mode: m}
}

func (c InteractiveExtensionUIContext) SetTheme(theme string) InteractiveThemeResult {
	if c.mode == nil || !isInteractiveThemeName(theme) {
		return InteractiveThemeResult{Success: false}
	}
	if c.mode.ThemeSettings != nil {
		c.mode.ThemeSettings.SetTheme(theme)
	}
	if c.mode.UI != nil {
		c.mode.UI.RequestRender()
	}
	return InteractiveThemeResult{Success: true}
}

func (c InteractiveExtensionUIContext) AddAutocompleteProvider(wrapper AutocompleteProviderFactory) {
	if c.mode == nil || wrapper == nil {
		return
	}
	c.mode.AutocompleteProviderWrappers = append(c.mode.AutocompleteProviderWrappers, wrapper)
	c.mode.SetupAutocompleteProvider()
}

func isInteractiveThemeName(theme string) bool {
	switch theme {
	case "dark", "light", "system":
		return true
	default:
		return false
	}
}

func (m *InteractiveMode) SetupAutocompleteProvider() {
	if m == nil {
		return
	}
	var provider AutocompleteProvider
	if m.CreateBaseAutocompleteProvider != nil {
		provider = m.CreateBaseAutocompleteProvider()
	}
	if provider == nil {
		provider = defaultAutocompleteProvider{}
	}
	for _, wrapper := range m.AutocompleteProviderWrappers {
		provider = wrapper(provider)
	}
	if m.DefaultAutocompleteEditor != nil {
		m.DefaultAutocompleteEditor.SetAutocompleteProvider(provider)
	}
	if m.AutocompleteEditor != nil {
		m.AutocompleteEditor.SetAutocompleteProvider(provider)
	}
}

func (m *InteractiveMode) ShowLoadedResources(options InteractiveShowLoadedResourcesOptions) {
	if m == nil || m.Chat == nil {
		return
	}
	resources := m.LoadedResources
	diagnostics := formatInteractiveDiagnostics(resources.SkillDiagnostics)
	if resources.QuietStartup && !options.Force && !resources.Verbose {
		if options.ShowDiagnosticsWhenQuiet && diagnostics != "" {
			m.Chat.AddText(diagnostics)
		}
		return
	}
	expanded := resources.ToolOutputExpanded || resources.Verbose || m.ToolOutputExpanded
	sections := []string{}
	if diagnostics != "" && (!resources.QuietStartup || options.ShowDiagnosticsWhenQuiet) {
		sections = append(sections, diagnostics)
	}
	if len(resources.Skills) > 0 {
		sections = append(sections, formatInteractiveSkills(resources.Skills, expanded))
	}
	if len(resources.Extensions) > 0 {
		if expanded {
			sections = append(sections, formatInteractiveExtensionsExpanded(resources.Extensions))
		} else {
			sections = append(sections, formatInteractiveExtensionsCompact(resources.Extensions))
		}
	}
	if len(resources.ContextFiles) > 0 {
		sections = append(sections, formatInteractiveContextFiles(resources.ContextFiles, resources.CWD, expanded))
	}
	if len(sections) > 0 {
		m.Chat.AddText(strings.Join(sections, "\n"))
	}
}

func formatInteractiveDiagnostics(diagnostics []InteractiveResourceDiagnostic) string {
	if len(diagnostics) == 0 {
		return ""
	}
	lines := []string{"[Skill conflicts]"}
	for _, diagnostic := range diagnostics {
		lines = append(lines, "  "+diagnostic.Message)
	}
	return strings.Join(lines, "\n")
}

func formatInteractiveSkills(skills []InteractiveSkillResource, expanded bool) string {
	lines := []string{"[Skills]"}
	if expanded {
		lines = append(lines, "  resource-list")
		return strings.Join(lines, "\n")
	}
	names := make([]string, 0, len(skills))
	for _, skill := range skills {
		names = append(names, skill.Name)
	}
	sort.Strings(names)
	lines = append(lines, "  "+strings.Join(names, ", "))
	return strings.Join(lines, "\n")
}

func formatInteractiveExtensionsCompact(extensions []InteractiveExtensionResource) string {
	labels := compactInteractiveExtensionLabels(extensions)
	return "[Extensions]\n  " + strings.Join(labels, ", ")
}

func formatInteractiveExtensionsExpanded(extensions []InteractiveExtensionResource) string {
	groups := map[string]*interactiveExpandedScope{}
	for _, extension := range extensions {
		scope := interactiveScopeLabel(extension.SourceInfo)
		group := groups[scope]
		if group == nil {
			group = &interactiveExpandedScope{packages: map[string][]string{}}
			groups[scope] = group
		}
		if extension.SourceInfo != nil && extension.SourceInfo.Origin == "package" {
			group.packages[extension.SourceInfo.Source] = append(group.packages[extension.SourceInfo.Source], expandedInteractiveExtensionPath(extension))
			continue
		}
		group.locals = append(group.locals, expandedInteractiveExtensionPath(extension))
	}
	lines := []string{"[Extensions]"}
	for _, scope := range []string{"project", "user", "path"} {
		group := groups[scope]
		if group == nil || (len(group.locals) == 0 && len(group.packages) == 0) {
			continue
		}
		sort.Strings(group.locals)
		lines = append(lines, "  "+scope)
		for _, item := range group.locals {
			lines = append(lines, "    "+item)
		}
		sources := make([]string, 0, len(group.packages))
		for source := range group.packages {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		for _, source := range sources {
			lines = append(lines, "    "+source)
			children := group.packages[source]
			sort.Strings(children)
			for _, child := range children {
				lines = append(lines, "      "+child)
			}
		}
	}
	return strings.Join(lines, "\n")
}

type interactiveExpandedScope struct {
	locals   []string
	packages map[string][]string
}

func compactInteractiveExtensionLabels(extensions []InteractiveExtensionResource) []string {
	labels := make([]string, len(extensions))
	local := []compactExtensionSegments{}
	for index, extension := range extensions {
		if isInteractivePackageSource(extension.SourceInfo) {
			labels[index] = compactInteractivePackageLabel(extension)
			continue
		}
		segments := compactInteractivePathSegments(extension.Path)
		local = append(local, compactExtensionSegments{index: index, segments: segments})
	}
	for _, item := range local {
		labels[item.index] = compactInteractiveUniqueLabel(item, local)
	}
	sort.Slice(labels, func(i, j int) bool {
		return strings.ToLower(labels[i]) < strings.ToLower(labels[j])
	})
	return labels
}

type compactExtensionSegments struct {
	index    int
	segments []string
}

func compactInteractiveUniqueLabel(target compactExtensionSegments, all []compactExtensionSegments) string {
	for width := 1; width <= len(target.segments); width++ {
		label := strings.Join(target.segments[len(target.segments)-width:], "/")
		unique := true
		for _, candidate := range all {
			if candidate.index == target.index || len(candidate.segments) < width {
				continue
			}
			if strings.Join(candidate.segments[len(candidate.segments)-width:], "/") == label {
				unique = false
				break
			}
		}
		if unique {
			return label
		}
	}
	return strings.Join(target.segments, "/")
}

func compactInteractivePathSegments(path string) []string {
	normalized := normalizeInteractivePath(path)
	segments := strings.Split(strings.Trim(normalized, "/"), "/")
	if len(segments) == 0 {
		return []string{normalized}
	}
	last := segments[len(segments)-1]
	if last == "index.gi.json" {
		segments = segments[:len(segments)-1]
	}
	if len(segments) == 0 {
		return []string{last}
	}
	return segments
}

func compactInteractivePackageLabel(extension InteractiveExtensionResource) string {
	source := extension.SourceInfo.Source
	label := strings.TrimPrefix(source, "npm:")
	if strings.HasPrefix(source, "git:") {
		label = strings.TrimPrefix(source, "git:")
		label = strings.TrimPrefix(label, "github.com/")
	}
	relative := packageRelativeExtensionPath(extension)
	if relative != "" && relative != "extensions" {
		label += ":" + strings.TrimPrefix(relative, "extensions/")
	}
	return label
}

func expandedInteractiveExtensionPath(extension InteractiveExtensionResource) string {
	info := extension.SourceInfo
	if info != nil && info.Origin == "package" {
		return packageRelativeExtensionPath(extension)
	}
	normalized := normalizeInteractivePath(extension.Path)
	if strings.HasSuffix(normalized, "/index.gi.json") {
		return strings.TrimSuffix(normalized, "/index.gi.json")
	}
	return normalized
}

func packageRelativeExtensionPath(extension InteractiveExtensionResource) string {
	if extension.SourceInfo == nil || extension.SourceInfo.BaseDir == "" {
		return ""
	}
	relative := strings.TrimPrefix(normalizeInteractivePath(extension.Path), strings.TrimRight(normalizeInteractivePath(extension.SourceInfo.BaseDir), "/")+"/")
	relative = strings.TrimSuffix(relative, "/index.gi.json")
	return relative
}

func interactiveScopeLabel(info *InteractiveSourceInfo) string {
	if info == nil || info.Scope == "" {
		return "path"
	}
	if info.Scope == "temporary" {
		return "path"
	}
	return info.Scope
}

func isInteractivePackageSource(info *InteractiveSourceInfo) bool {
	return info != nil && info.Origin == "package"
}

func formatInteractiveContextFiles(files []InteractiveContextFile, cwd string, expanded bool) string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		if expanded {
			paths = append(paths, formatInteractiveHomePath(file.Path))
		} else {
			paths = append(paths, formatInteractiveCompactContextPath(file.Path, cwd))
		}
	}
	return "[Context]\n  " + strings.Join(paths, ", ")
}

func formatInteractiveCompactContextPath(path, cwd string) string {
	normalized := normalizeInteractivePath(path)
	cwd = strings.TrimRight(normalizeInteractivePath(cwd), "/")
	if cwd != "" && strings.HasPrefix(normalized, cwd+"/") {
		return strings.TrimPrefix(normalized, cwd+"/")
	}
	return formatInteractiveHomePath(normalized)
}

func formatInteractiveHomePath(path string) string {
	normalized := normalizeInteractivePath(path)
	home, err := osUserHomeDir()
	if err == nil && home != "" {
		home = strings.TrimRight(normalizeInteractivePath(home), "/")
		if normalized == home {
			return "~"
		}
		if strings.HasPrefix(normalized, home+"/") {
			return "~/" + strings.TrimPrefix(normalized, home+"/")
		}
	}
	return normalized
}

var osUserHomeDir = func() (string, error) {
	return os.UserHomeDir()
}

func normalizeInteractivePath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}
