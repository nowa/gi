package gicodingagent

import (
	"fmt"
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
	FilePath   string
	Name       string
	SourceInfo *InteractiveSourceInfo
}

type InteractivePromptResource struct {
	FilePath   string
	Name       string
	SourceInfo *InteractiveSourceInfo
}

type InteractiveThemeResource struct {
	SourcePath string
	Name       string
	SourceInfo *InteractiveSourceInfo
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
	QuietStartup         bool
	Verbose              bool
	ToolOutputExpanded   bool
	CWD                  string
	ContextFiles         []InteractiveContextFile
	Extensions           []InteractiveExtensionResource
	Skills               []InteractiveSkillResource
	Prompts              []InteractivePromptResource
	Themes               []InteractiveThemeResource
	SkillDiagnostics     []InteractiveResourceDiagnostic
	PromptDiagnostics    []InteractiveResourceDiagnostic
	ExtensionDiagnostics []InteractiveResourceDiagnostic
	ThemeDiagnostics     []InteractiveResourceDiagnostic
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

func (c InteractiveExtensionUIContext) GetToolsExpanded() bool {
	if c.mode == nil {
		return false
	}
	return c.mode.ToolOutputExpanded
}

func (c InteractiveExtensionUIContext) SetToolsExpanded(expanded bool) {
	if c.mode != nil {
		c.mode.SetToolsExpanded(expanded)
	}
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
	case "dark", "light":
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
	output := m.FormatLoadedResources(options)
	if output != "" {
		m.Chat.AddText(output)
	}
}

func (m *InteractiveMode) FormatLoadedResources(options InteractiveShowLoadedResourcesOptions) string {
	if m == nil {
		return ""
	}
	resources := m.LoadedResources
	return formatInteractiveLoadedResources(resources, options, m.ToolOutputExpanded)
}

func formatInteractiveLoadedResources(resources InteractiveLoadedResources, options InteractiveShowLoadedResourcesOptions, toolOutputExpanded bool) string {
	expanded := resources.ToolOutputExpanded || resources.Verbose || toolOutputExpanded
	diagnostics := formatInteractiveResourceDiagnostics(resources, expanded)
	if resources.QuietStartup && !options.Force && !resources.Verbose {
		if options.ShowDiagnosticsWhenQuiet {
			return diagnostics
		}
		return ""
	}
	sections := []string{}
	if len(resources.ContextFiles) > 0 {
		sections = append(sections, formatInteractiveContextFiles(resources.ContextFiles, resources.CWD, expanded))
	}
	if len(resources.Skills) > 0 {
		sections = append(sections, formatInteractiveSkills(resources.Skills, expanded))
	}
	if len(resources.Prompts) > 0 {
		sections = append(sections, formatInteractivePrompts(resources.Prompts, expanded))
	}
	if len(resources.Extensions) > 0 {
		if expanded {
			sections = append(sections, formatInteractiveExtensionsExpanded(resources.Extensions))
		} else {
			sections = append(sections, formatInteractiveExtensionsCompact(resources.Extensions))
		}
	}
	if len(resources.Themes) > 0 {
		sections = append(sections, formatInteractiveThemes(resources.Themes, expanded))
	}
	if diagnostics != "" && (!resources.QuietStartup || options.ShowDiagnosticsWhenQuiet) {
		sections = append(sections, diagnostics)
	}
	if len(sections) > 0 {
		return strings.Join(sections, "\n\n")
	}
	return ""
}

func formatInteractiveDiagnostics(diagnostics []InteractiveResourceDiagnostic) string {
	return formatInteractiveDiagnosticsSection("[Skill conflicts]", diagnostics, false)
}

func formatInteractiveDiagnosticsSection(header string, diagnostics []InteractiveResourceDiagnostic, expanded bool) string {
	if len(diagnostics) == 0 {
		return ""
	}
	lines := []string{header}
	if !expanded && len(diagnostics) > 1 {
		lines = append(lines, fmt.Sprintf("  %d issue(s). Ctrl+O shows details.", len(diagnostics)))
		return strings.Join(lines, "\n")
	}
	for _, diagnostic := range diagnostics {
		message := diagnostic.Message
		if !expanded {
			message = compactInteractiveDiagnosticMessage(message)
		}
		lines = append(lines, "  "+message)
	}
	return strings.Join(lines, "\n")
}

func compactInteractiveDiagnosticMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	if strings.Contains(message, "overridden") {
		message = strings.Join(strings.Fields(message), " ")
	} else {
		lines := strings.FieldsFunc(message, func(r rune) bool { return r == '\n' || r == '\r' })
		if len(lines) > 0 {
			message = strings.TrimSpace(lines[0])
		}
	}
	limit := 60
	if strings.Contains(message, "overridden") {
		limit = 100
	}
	runes := []rune(message)
	if len(runes) <= limit {
		return message
	}
	if strings.Contains(message, "overridden") {
		suffix := " ... overridden"
		maxPrefix := max(0, limit-len([]rune(suffix)))
		return string(runes[:maxPrefix]) + suffix
	}
	return string(runes[:max(0, limit-3)]) + "..."
}

func formatInteractiveResourceDiagnostics(resources InteractiveLoadedResources, expanded bool) string {
	sections := []string{}
	if formatted := formatInteractiveDiagnosticsSection("[Skill conflicts]", resources.SkillDiagnostics, expanded); formatted != "" {
		sections = append(sections, formatted)
	}
	if formatted := formatInteractiveDiagnosticsSection("[Prompt conflicts]", resources.PromptDiagnostics, expanded); formatted != "" {
		sections = append(sections, formatted)
	}
	if formatted := formatInteractiveDiagnosticsSection("[Extension issues]", resources.ExtensionDiagnostics, expanded); formatted != "" {
		sections = append(sections, formatted)
	}
	if formatted := formatInteractiveDiagnosticsSection("[Theme conflicts]", resources.ThemeDiagnostics, expanded); formatted != "" {
		sections = append(sections, formatted)
	}
	return strings.Join(sections, "\n")
}

func formatInteractiveSkills(skills []InteractiveSkillResource, expanded bool) string {
	lines := []string{"[Skills]"}
	if expanded {
		items := make([]interactiveScopeItem, 0, len(skills))
		for _, skill := range skills {
			if skill.FilePath != "" {
				items = append(items, interactiveScopeItem{Path: skill.FilePath, SourceInfo: skill.SourceInfo})
			}
		}
		lines = append(lines, formatInteractiveScopeGroups(
			buildInteractiveScopeGroups(items),
			func(item interactiveScopeItem) string { return formatInteractiveHomePath(item.Path) },
			func(item interactiveScopeItem) string { return getInteractiveShortPath(item.Path, item.SourceInfo) },
		)...)
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

func formatInteractivePrompts(prompts []InteractivePromptResource, expanded bool) string {
	lines := []string{"[Prompts]"}
	if expanded {
		items := make([]interactiveScopeItem, 0, len(prompts))
		for _, prompt := range prompts {
			if prompt.FilePath != "" {
				label := "/" + strings.TrimPrefix(strings.TrimSpace(prompt.Name), "/")
				if label == "/" {
					label = formatInteractiveHomePath(prompt.FilePath)
				}
				items = append(items, interactiveScopeItem{
					Path:       prompt.FilePath,
					Label:      label,
					SourceInfo: prompt.SourceInfo,
				})
			}
		}
		lines = append(lines, formatInteractiveScopeGroups(
			buildInteractiveScopeGroups(items),
			func(item interactiveScopeItem) string { return item.Label },
			func(item interactiveScopeItem) string { return item.Label },
		)...)
		return strings.Join(lines, "\n")
	}
	names := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		name := strings.TrimSpace(prompt.Name)
		if name != "" {
			names = append(names, "/"+strings.TrimPrefix(name, "/"))
		}
	}
	sort.Strings(names)
	lines = append(lines, "  "+strings.Join(names, ", "))
	return strings.Join(lines, "\n")
}

func formatInteractiveThemes(themes []InteractiveThemeResource, expanded bool) string {
	lines := []string{"[Themes]"}
	if expanded {
		items := make([]interactiveScopeItem, 0, len(themes))
		for _, theme := range themes {
			if theme.SourcePath != "" {
				items = append(items, interactiveScopeItem{Path: theme.SourcePath, SourceInfo: theme.SourceInfo})
			}
		}
		lines = append(lines, formatInteractiveScopeGroups(
			buildInteractiveScopeGroups(items),
			func(item interactiveScopeItem) string { return formatInteractiveHomePath(item.Path) },
			func(item interactiveScopeItem) string { return getInteractiveShortPath(item.Path, item.SourceInfo) },
		)...)
		return strings.Join(lines, "\n")
	}
	values := make([]string, 0, len(themes))
	for _, theme := range themes {
		name := strings.TrimSpace(theme.Name)
		if name != "" {
			values = append(values, name)
		}
	}
	sort.Strings(values)
	lines = append(lines, "  "+strings.Join(values, ", "))
	return strings.Join(lines, "\n")
}

type interactiveScopeItem struct {
	Path       string
	Label      string
	SourceInfo *InteractiveSourceInfo
}

type interactiveScopeGroup struct {
	scope    string
	paths    []interactiveScopeItem
	packages map[string][]interactiveScopeItem
}

func buildInteractiveScopeGroups(items []interactiveScopeItem) []interactiveScopeGroup {
	groups := map[string]*interactiveScopeGroup{
		"project": {scope: "project", packages: map[string][]interactiveScopeItem{}},
		"user":    {scope: "user", packages: map[string][]interactiveScopeItem{}},
		"path":    {scope: "path", packages: map[string][]interactiveScopeItem{}},
	}
	for _, item := range items {
		scope := interactiveScopeGroupKey(item.SourceInfo)
		group := groups[scope]
		if group == nil {
			group = groups["path"]
		}
		if isInteractivePackageSource(item.SourceInfo) {
			source := strings.TrimSpace(item.SourceInfo.Source)
			if source == "" {
				source = "package"
			}
			group.packages[source] = append(group.packages[source], item)
			continue
		}
		group.paths = append(group.paths, item)
	}
	ordered := make([]interactiveScopeGroup, 0, 3)
	for _, scope := range []string{"project", "user", "path"} {
		group := groups[scope]
		if group != nil && (len(group.paths) > 0 || len(group.packages) > 0) {
			ordered = append(ordered, *group)
		}
	}
	return ordered
}

func formatInteractiveScopeGroups(groups []interactiveScopeGroup, formatPath, formatPackagePath func(interactiveScopeItem) string) []string {
	lines := []string{}
	for _, group := range groups {
		lines = append(lines, "  "+group.scope)
		sort.Slice(group.paths, func(i, j int) bool {
			return strings.ToLower(group.paths[i].Path) < strings.ToLower(group.paths[j].Path)
		})
		for _, item := range group.paths {
			lines = append(lines, "    "+formatPath(item))
		}
		sources := make([]string, 0, len(group.packages))
		for source := range group.packages {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		for _, source := range sources {
			lines = append(lines, "    "+source)
			items := group.packages[source]
			sort.Slice(items, func(i, j int) bool {
				return strings.ToLower(items[i].Path) < strings.ToLower(items[j].Path)
			})
			for _, item := range items {
				lines = append(lines, "      "+formatPackagePath(item))
			}
		}
	}
	return lines
}

func interactiveScopeGroupKey(info *InteractiveSourceInfo) string {
	source := "local"
	scope := "project"
	if info != nil {
		if strings.TrimSpace(info.Source) != "" {
			source = strings.TrimSpace(info.Source)
		}
		if strings.TrimSpace(info.Scope) != "" {
			scope = strings.TrimSpace(info.Scope)
		}
	}
	switch {
	case source == "cli", scope == "temporary":
		return "path"
	case scope == "user":
		return "user"
	case scope == "project":
		return "project"
	default:
		return "path"
	}
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
	label := source
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
	if info == nil {
		return false
	}
	source := strings.TrimSpace(info.Source)
	return info.Origin == "package" || strings.HasPrefix(source, "git:")
}

func getInteractiveShortPath(fullPath string, sourceInfo *InteractiveSourceInfo) string {
	if sourceInfo != nil && strings.TrimSpace(sourceInfo.BaseDir) != "" && isInteractivePackageSource(sourceInfo) {
		relative, err := filepath.Rel(sourceInfo.BaseDir, fullPath)
		if err == nil && relative != "." && !strings.HasPrefix(relative, "..") && !filepath.IsAbs(relative) {
			return filepath.ToSlash(relative)
		}
	}
	return formatInteractiveHomePath(fullPath)
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
