package gicodingagent

import (
	"strings"
	"sync/atomic"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	gitui "github.com/nowa/gi/gi-tui"
)

type agentSessionExtensionsResourceLoader interface {
	GetExtensions() ResourceExtensionsResult
}

type agentSessionExtensionFlagResourceLoader interface {
	ApplyExtensionFlagValues(values map[string]any, allowDeferred bool) []ProtocolExtensionDiscoveryError
}

type agentSessionThemesResourceLoader interface {
	GetThemes() ResourceThemesResult
}

type agentSessionAgentsFilesResourceLoader interface {
	GetAgentsFiles() ResourceAgentsFilesResult
}

type cliLoadedResourcesComponent struct {
	resources InteractiveLoadedResources
	options   InteractiveShowLoadedResourcesOptions
	expanded  atomic.Bool
}

func newCLILoadedResourcesComponent(resources InteractiveLoadedResources, options InteractiveShowLoadedResourcesOptions, expanded bool) *cliLoadedResourcesComponent {
	component := &cliLoadedResourcesComponent{resources: resources, options: options}
	component.expanded.Store(expanded)
	return component
}

func (c *cliLoadedResourcesComponent) SetExpanded(expanded bool) {
	if c != nil {
		c.expanded.Store(expanded)
	}
}

func (c *cliLoadedResourcesComponent) Invalidate() {}

func (c *cliLoadedResourcesComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	text := formatInteractiveLoadedResources(c.resources, c.options, c.expanded.Load())
	text = tuiThemeLoadedResources(text)
	return gitui.NewText(text, 0, 0).Render(width)
}

func tuiThemeLoadedResources(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	lines := strings.Split(text, "\n")
	inDiagnosticSection := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"):
			inDiagnosticSection = strings.Contains(trimmed, "conflicts") || strings.Contains(trimmed, "issues")
			if inDiagnosticSection {
				lines[index] = tuiThemeWarning(line)
			} else {
				lines[index] = tuiThemeFG("mdHeading", line)
			}
		case inDiagnosticSection && strings.HasPrefix(line, "  "):
			lines[index] = tuiThemeWarning(line)
		case strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") &&
			(trimmed == "project" || trimmed == "user" || trimmed == "path"):
			lines[index] = strings.TrimSuffix(line, trimmed) + tuiThemeAccent(trimmed)
		case strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") &&
			(strings.HasPrefix(trimmed, "git:") || strings.HasPrefix(trimmed, "official:") || strings.HasPrefix(trimmed, "local:")):
			lines[index] = strings.TrimSuffix(line, trimmed) + tuiThemeFG("mdLink", trimmed)
		case strings.HasPrefix(line, "  "):
			lines[index] = tuiThemeDim(line)
		}
	}
	return strings.Join(lines, "\n")
}

func (h *CLIInteractiveTUIHost) showLoadedResourcesOnStartup() {
	if h == nil || h.chat == nil {
		return
	}
	session, err := h.currentAgentSession()
	if err != nil || session == nil || session.ResourceLoader == nil {
		return
	}
	resources := h.loadedResourcesForSession(session)
	output := formatInteractiveLoadedResources(
		resources,
		InteractiveShowLoadedResourcesOptions{ShowDiagnosticsWhenQuiet: true},
		h.toolOutputExpanded,
	)
	if strings.TrimSpace(output) == "" {
		return
	}
	component := newCLILoadedResourcesComponent(
		resources,
		InteractiveShowLoadedResourcesOptions{ShowDiagnosticsWhenQuiet: true},
		h.toolOutputExpanded,
	)
	h.startupResources = append(h.startupResources, component)
	if len(resources.ContextFiles) > 0 {
		h.chat.AddChild(gitui.NewSpacer(1))
	}
	h.chat.AddChild(component)
	h.chat.AddChild(gitui.NewSpacer(1))
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) showModelScopeOnStartup() {
	if h == nil || h.chat == nil {
		return
	}
	session, err := h.currentAgentSession()
	if err != nil || session == nil || len(session.ScopedModels) == 0 {
		return
	}
	settings := h.settingsManager()
	if !h.verboseStartup && settings != nil && settings.GetQuietStartup() {
		return
	}
	labels := make([]string, 0, len(session.ScopedModels))
	for _, scoped := range session.ScopedModels {
		label := scoped.Model.ID
		if strings.TrimSpace(string(scoped.ThinkingLevel)) != "" {
			label += ":" + string(scoped.ThinkingLevel)
		}
		labels = append(labels, label)
	}
	h.chat.AddChild(gitui.NewText("Model scope: "+strings.Join(labels, ", "), 1, 0))
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) showModelRegistryErrorIfAny() {
	if h == nil || h.chat == nil {
		return
	}
	registry := h.modelRegistry()
	if registry == nil {
		return
	}
	if message := strings.TrimSpace(registry.GetError()); message != "" {
		h.addStatus("models.json error: " + message)
	}
}

func (h *CLIInteractiveTUIHost) loadedResourcesForSession(session *AgentSession) InteractiveLoadedResources {
	resources := InteractiveLoadedResources{
		Verbose:            h != nil && h.verboseStartup,
		CWD:                "",
		ToolOutputExpanded: h != nil && h.toolOutputExpanded,
	}
	if h != nil {
		resources.CWD = h.interactiveCWD()
		if settings := h.settingsManager(); settings != nil {
			resources.QuietStartup = settings.GetQuietStartup()
		}
	}
	if session == nil || session.ResourceLoader == nil {
		return resources
	}

	skills := session.ResourceLoader.GetSkills()
	resources.Skills = interactiveSkillsFromAgentSkills(skills.Skills)
	resources.SkillDiagnostics = interactiveDiagnosticsFromSkillDiagnostics(skills.Diagnostics)

	if loader, ok := session.ResourceLoader.(AgentSessionPromptResourceLoader); ok {
		resources.Prompts = interactivePromptsFromPromptTemplates(loader.GetPrompts().Prompts)
	}
	if loader, ok := session.ResourceLoader.(agentSessionExtensionsResourceLoader); ok {
		extensions := loader.GetExtensions()
		resources.Extensions = interactiveExtensionsFromProtocolSources(extensions.Extensions)
		resources.ExtensionDiagnostics = interactiveDiagnosticsFromExtensionErrors(extensions.Errors)
	}
	resources.ExtensionDiagnostics = append(resources.ExtensionDiagnostics, interactiveBuiltinCommandConflictDiagnostics(session.ExtensionRuntime)...)
	resources.ExtensionDiagnostics = append(resources.ExtensionDiagnostics, interactiveShortcutDiagnostics(session.ExtensionRuntime, h.effectiveKeybindings())...)
	if loader, ok := session.ResourceLoader.(agentSessionThemesResourceLoader); ok {
		themes := loader.GetThemes()
		resources.Themes = interactiveThemesFromResourceThemes(themes.Themes)
		resources.ThemeDiagnostics = interactiveDiagnosticsFromSkillDiagnostics(themes.Diagnostics)
	}
	if loader, ok := session.ResourceLoader.(agentSessionAgentsFilesResourceLoader); ok {
		resources.ContextFiles = interactiveContextFilesFromResources(loader.GetAgentsFiles().AgentsFiles)
	}
	return resources
}

func interactiveSkillsFromAgentSkills(skills []agentharness.Skill) []InteractiveSkillResource {
	result := make([]InteractiveSkillResource, 0, len(skills))
	for _, skill := range skills {
		result = append(result, InteractiveSkillResource{
			FilePath:   skill.FilePath,
			Name:       skill.Name,
			SourceInfo: interactiveSourceInfoFromAny(skill.SourceInfo, ""),
		})
	}
	return result
}

func interactivePromptsFromPromptTemplates(prompts []PromptTemplate) []InteractivePromptResource {
	result := make([]InteractivePromptResource, 0, len(prompts))
	for _, prompt := range prompts {
		result = append(result, InteractivePromptResource{
			FilePath:   prompt.FilePath,
			Name:       prompt.Name,
			SourceInfo: interactiveSourceInfoFromSourceInfo(prompt.SourceInfo, ""),
		})
	}
	return result
}

func interactiveExtensionsFromProtocolSources(extensions []ProtocolExtensionSource) []InteractiveExtensionResource {
	result := make([]InteractiveExtensionResource, 0, len(extensions))
	for _, extension := range extensions {
		result = append(result, InteractiveExtensionResource{
			Path:       extension.Path,
			SourceInfo: interactiveSourceInfoFromProtocol(extension.Metadata, extension.BaseDir),
		})
	}
	return result
}

func interactiveBuiltinCommandConflictDiagnostics(runtime *ProtocolExtensionRuntime) []InteractiveResourceDiagnostic {
	if runtime == nil {
		return nil
	}
	builtinNames := map[string]bool{}
	for _, command := range builtinInteractiveSlashCommands() {
		builtinNames[command.Name] = true
	}
	var diagnostics []InteractiveResourceDiagnostic
	for _, command := range runtime.RegisteredCommands() {
		if !builtinNames[command.Name] {
			continue
		}
		message := "Extension command '/" + command.Name + "' conflicts with built-in interactive command. Skipping in autocomplete."
		if command.InvocationName != "" && command.InvocationName != command.Name {
			message = "Extension command '/" + command.Name + "' conflicts with built-in interactive command. Available as '/" + command.InvocationName + "'."
		}
		diagnostics = append(diagnostics, InteractiveResourceDiagnostic{Type: "warning", Message: message})
	}
	return diagnostics
}

func interactiveShortcutDiagnostics(runtime *ProtocolExtensionRuntime, keybindings ...KeybindingsConfig) []InteractiveResourceDiagnostic {
	if runtime == nil {
		return nil
	}
	effective := DefaultProtocolKeybindings()
	if len(keybindings) > 0 && keybindings[0] != nil {
		effective = keybindings[0]
	}
	shortcuts := runtime.Shortcuts(effective)
	diagnostics := make([]InteractiveResourceDiagnostic, 0, len(shortcuts.Warnings))
	for _, warning := range shortcuts.Warnings {
		if strings.TrimSpace(warning.Message) == "" {
			continue
		}
		diagnostics = append(diagnostics, InteractiveResourceDiagnostic{Type: "warning", Message: warning.Message})
	}
	return diagnostics
}

func interactiveThemesFromResourceThemes(themes []ResourceTheme) []InteractiveThemeResource {
	result := make([]InteractiveThemeResource, 0, len(themes))
	for _, theme := range themes {
		result = append(result, InteractiveThemeResource{Name: theme.Name, SourcePath: theme.SourcePath})
	}
	return result
}

func interactiveContextFilesFromResources(files []ResourceContextFile) []InteractiveContextFile {
	result := make([]InteractiveContextFile, 0, len(files))
	for _, file := range files {
		result = append(result, InteractiveContextFile{Path: file.Path})
	}
	return result
}

func interactiveDiagnosticsFromSkillDiagnostics(diagnostics []agentharness.SkillDiagnostic) []InteractiveResourceDiagnostic {
	result := make([]InteractiveResourceDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		result = append(result, InteractiveResourceDiagnostic{Type: diagnostic.Type, Message: diagnostic.Message, Path: diagnostic.Path})
	}
	return result
}

func interactiveDiagnosticsFromExtensionErrors(errors []ProtocolExtensionDiscoveryError) []InteractiveResourceDiagnostic {
	result := make([]InteractiveResourceDiagnostic, 0, len(errors))
	for _, err := range errors {
		message := strings.TrimSpace(err.Error)
		if err.Path != "" {
			message = err.Path + ": " + message
		}
		result = append(result, InteractiveResourceDiagnostic{Type: "error", Message: message})
	}
	return result
}

func interactiveSourceInfoFromAny(value any, baseDir string) *InteractiveSourceInfo {
	switch source := value.(type) {
	case ProtocolSourceInfo:
		return interactiveSourceInfoFromProtocol(source, baseDir)
	case *ProtocolSourceInfo:
		if source == nil {
			return nil
		}
		return interactiveSourceInfoFromProtocol(*source, baseDir)
	case SourceInfo:
		return interactiveSourceInfoFromSourceInfo(source, baseDir)
	case *SourceInfo:
		if source == nil {
			return nil
		}
		return interactiveSourceInfoFromSourceInfo(*source, baseDir)
	default:
		return nil
	}
}

func interactiveSourceInfoFromProtocol(source ProtocolSourceInfo, baseDir string) *InteractiveSourceInfo {
	if source.Path == "" && source.Source == "" && source.Scope == "" && source.Origin == "" && baseDir == "" {
		return nil
	}
	return &InteractiveSourceInfo{
		Path:    source.Path,
		Source:  source.Source,
		Scope:   source.Scope,
		Origin:  source.Origin,
		BaseDir: baseDir,
	}
}

func interactiveSourceInfoFromSourceInfo(source SourceInfo, baseDir string) *InteractiveSourceInfo {
	if source.Path == "" && source.Source == "" && source.Scope == "" && source.Origin == "" && baseDir == "" {
		return nil
	}
	return &InteractiveSourceInfo{
		Path:    source.Path,
		Source:  source.Source,
		Scope:   source.Scope,
		Origin:  source.Origin,
		BaseDir: baseDir,
	}
}
