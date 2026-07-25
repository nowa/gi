package gicodingagent

import (
	"context"
	"sort"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func (h *CLIInteractiveTUIHost) refreshEditorAutocompleteProvider() {
	if h == nil || h.editor == nil || h.runtimeHost == nil {
		return
	}
	commands := h.autocompleteSlashCommands()
	var providers []gitui.AutocompleteProvider
	providerHost, ok := h.runtimeHost.(protocolExtensionRuntimeProvider)
	if ok {
		runtime := providerHost.ProtocolExtensionRuntime()
		if runtime != nil && len(runtime.AutocompleteProviders()) > 0 {
			providers = append(providers, protocolAutocompleteTUIProvider{runtime: runtime})
		}
	}
	provider := gitui.NewCombinedAutocompleteProviderWithCommands(h.interactiveCWD(), commands, providers...)
	h.setAutocompleteProvider(provider)
	h.editor.SetAutocompleteProvider(provider)
	if active, ok := h.activeEditorComponent(); ok && active != h.editor {
		if editor, ok := active.(gitui.EditorAutocompleteComponent); ok {
			editor.SetAutocompleteProvider(provider)
		}
	}
}

func (h *CLIInteractiveTUIHost) setAutocompleteProvider(provider gitui.AutocompleteProvider) {
	if h == nil {
		return
	}
	h.autocompleteMu.Lock()
	h.autocompleteProvider = provider
	h.autocompleteMu.Unlock()
}

func (h *CLIInteractiveTUIHost) autocompleteProviderSnapshot() gitui.AutocompleteProvider {
	if h == nil {
		return nil
	}
	h.autocompleteMu.RLock()
	provider := h.autocompleteProvider
	h.autocompleteMu.RUnlock()
	return provider
}

func (h *CLIInteractiveTUIHost) autocompleteSlashCommands() []gitui.SlashCommand {
	seen := map[string]bool{}
	var commands []gitui.SlashCommand
	add := func(command gitui.SlashCommand) {
		if command.Name == "" || seen[command.Name] {
			return
		}
		seen[command.Name] = true
		commands = append(commands, command)
	}
	for _, command := range builtinInteractiveSlashCommands() {
		slash := gitui.SlashCommand{Name: command.Name, Description: command.Description, ArgumentHint: command.ArgumentHint}
		if command.Name == "model" {
			slash.GetArgumentCompletions = h.modelArgumentCompletions
		}
		if command.Name == "login" {
			slash.GetArgumentCompletions = h.loginArgumentCompletions
		}
		add(slash)
	}
	if host, err := h.newRPCSessionHost(); err == nil && host != nil {
		for _, command := range host.GetCommands().Commands {
			add(gitui.SlashCommand{
				Name:         command.Name,
				Description:  autocompleteDescriptionWithSource(command.Description, command.SourceInfo),
				ArgumentHint: command.ArgumentHint,
			})
		}
	}
	return commands
}

func (h *CLIInteractiveTUIHost) modelArgumentCompletions(prefix string) []gitui.AutocompleteItem {
	host, err := h.newRPCSessionHost()
	if err != nil || host == nil {
		return nil
	}
	models := host.getAvailableModels()
	if len(host.ScopedModels) > 0 {
		models = models[:0]
		for _, scopedModel := range host.ScopedModels {
			models = append(models, scopedModel.Model)
		}
	}
	return createFuzzyAutocompleteItems(
		models,
		prefix,
		func(model llm.Model) string {
			return modelSearchText(modelSearchItemFromModel(model))
		},
		func(model llm.Model) gitui.AutocompleteItem {
			value := scopedModelFullID(model)
			return gitui.AutocompleteItem{
				Value:       value,
				Label:       model.ID,
				Description: strings.TrimSpace(model.Provider),
			}
		},
	)
}

type loginProviderCompletionOption struct {
	id        string
	name      string
	authTypes []string
}

func createFuzzyAutocompleteItems[T any](
	items []T,
	prefix string,
	searchText func(T) string,
	toAutocompleteItem func(T) gitui.AutocompleteItem,
) []gitui.AutocompleteItem {
	filtered := gitui.FuzzyFilter(items, prefix, searchText)
	if len(filtered) == 0 {
		return nil
	}
	completions := make([]gitui.AutocompleteItem, 0, len(filtered))
	for _, item := range filtered {
		completions = append(completions, toAutocompleteItem(item))
	}
	return completions
}

func getLoginProviderCompletionOptions(
	providers []AuthSelectorProvider,
) []loginProviderCompletionOption {
	options := make([]loginProviderCompletionOption, 0, len(providers))
	indexByID := make(map[string]int, len(providers))
	for _, provider := range providers {
		if index, ok := indexByID[provider.ID]; ok {
			existing := &options[index]
			if !containsString(existing.authTypes, provider.AuthType) {
				existing.authTypes = append(
					existing.authTypes,
					provider.AuthType,
				)
				sort.SliceStable(
					existing.authTypes,
					func(i, j int) bool {
						return loginAuthTypeRank(existing.authTypes[i]) <
							loginAuthTypeRank(existing.authTypes[j])
					},
				)
			}
			continue
		}
		indexByID[provider.ID] = len(options)
		options = append(options, loginProviderCompletionOption{
			id:        provider.ID,
			name:      provider.Name,
			authTypes: []string{provider.AuthType},
		})
	}
	sort.SliceStable(options, func(i, j int) bool {
		left := strings.ToLower(options[i].name)
		right := strings.ToLower(options[j].name)
		if left == right {
			return options[i].id < options[j].id
		}
		return left < right
	})
	return options
}

func loginAuthTypeRank(authType string) int {
	switch authType {
	case string(llm.CredentialTypeOAuth):
		return 0
	case string(llm.CredentialTypeAPIKey):
		return 1
	default:
		return 2
	}
}

func getLoginProviderSearchText(
	provider loginProviderCompletionOption,
) string {
	authTypes := make([]string, 0, len(provider.authTypes)*2)
	for _, authType := range provider.authTypes {
		authTypes = append(
			authTypes,
			authType,
			formatAuthSelectorProviderType(authType),
		)
	}
	return strings.Join(
		[]string{
			provider.id,
			provider.name,
			strings.Join(authTypes, " "),
		},
		" ",
	)
}

func formatLoginProviderCompletionDescription(
	provider loginProviderCompletionOption,
) string {
	authTypes := make([]string, 0, len(provider.authTypes))
	for _, authType := range provider.authTypes {
		authTypes = append(
			authTypes,
			formatAuthSelectorProviderType(authType),
		)
	}
	description := strings.Join(authTypes, "/")
	if provider.name == provider.id {
		return description
	}
	return provider.name + " · " + description
}

func (h *CLIInteractiveTUIHost) loginArgumentCompletions(
	prefix string,
) []gitui.AutocompleteItem {
	registry := h.modelRegistry()
	if registry == nil {
		return nil
	}
	providers := getLoginProviderCompletionOptions(
		loginAuthSelectorProviders(registry, ""),
	)
	return createFuzzyAutocompleteItems(
		providers,
		prefix,
		getLoginProviderSearchText,
		func(provider loginProviderCompletionOption) gitui.AutocompleteItem {
			return gitui.AutocompleteItem{
				Value:       provider.id,
				Label:       provider.id,
				Description: formatLoginProviderCompletionDescription(provider),
			}
		},
	)
}

func autocompleteDescriptionWithSource(description string, sourceInfo any) string {
	tag := autocompleteSourceTag(sourceInfo)
	if tag == "" {
		return description
	}
	if description == "" {
		return "[" + tag + "]"
	}
	return "[" + tag + "] " + description
}

func autocompleteSourceTag(sourceInfo any) string {
	source, scope := sourceInfoFields(sourceInfo)
	if source == "" && scope == "" {
		return ""
	}
	scopePrefix := "t"
	switch scope {
	case "user":
		scopePrefix = "u"
	case "project":
		scopePrefix = "p"
	}
	switch {
	case source == "", source == "auto", source == "local", source == "cli", source == "inline":
		return scopePrefix
	case strings.HasPrefix(source, "git:"):
		if gitSource, ok := ParseGitURL(source); ok {
			ref := ""
			if gitSource.Ref != "" {
				ref = "@" + gitSource.Ref
			}
			return scopePrefix + ":git:" + gitSource.Host + "/" + gitSource.Path + ref
		}
		return scopePrefix + ":" + source
	case strings.HasPrefix(source, "official:"):
		return scopePrefix + ":" + source
	default:
		return scopePrefix
	}
}

func sourceInfoFields(sourceInfo any) (source, scope string) {
	switch info := sourceInfo.(type) {
	case SourceInfo:
		return strings.TrimSpace(info.Source), strings.TrimSpace(info.Scope)
	case *SourceInfo:
		if info == nil {
			return "", ""
		}
		return strings.TrimSpace(info.Source), strings.TrimSpace(info.Scope)
	case ProtocolSourceInfo:
		return strings.TrimSpace(info.Source), strings.TrimSpace(info.Scope)
	case *ProtocolSourceInfo:
		if info == nil {
			return "", ""
		}
		return strings.TrimSpace(info.Source), strings.TrimSpace(info.Scope)
	case map[string]any:
		source, _ := info["source"].(string)
		scope, _ := info["scope"].(string)
		return strings.TrimSpace(source), strings.TrimSpace(scope)
	default:
		return "", ""
	}
}

type interactiveSlashCommand struct {
	Name         string
	Description  string
	ArgumentHint string
}

func builtinInteractiveSlashCommands() []interactiveSlashCommand {
	return []interactiveSlashCommand{
		{Name: "settings", Description: "Open settings menu"},
		{Name: "trust", Description: "Manage project trust"},
		{Name: "model", Description: "Select model (opens selector UI)"},
		{Name: "scoped-models", Description: "Enable/disable models for Ctrl+P cycling"},
		{Name: "export", Description: "Export session (HTML default, or specify path: .html/.jsonl)"},
		{Name: "import", Description: "Import and resume a session from a JSONL file"},
		{Name: "share", Description: "Share session as a secret GitHub gist"},
		{Name: "copy", Description: "Copy last agent message to clipboard"},
		{Name: "name", Description: "Set session display name"},
		{Name: "session", Description: "Show session info and stats"},
		{Name: "changelog", Description: "Show changelog entries"},
		{Name: "hotkeys", Description: "Show all keyboard shortcuts"},
		{Name: "fork", Description: "Create a new fork from a previous user message"},
		{Name: "clone", Description: "Duplicate the current session at the current position"},
		{Name: "tree", Description: "Navigate session tree (switch branches)"},
		{Name: "login", Description: "Configure provider authentication"},
		{Name: "logout", Description: "Remove provider authentication"},
		{Name: "llama", Description: builtinLlamaCommandDescription},
		{Name: "new", Description: "Start a new session"},
		{Name: "compact", Description: "Manually compact the session context"},
		{Name: "resume", Description: "Resume a different session"},
		{Name: "reload", Description: "Reload keybindings, extensions, skills, prompts, and themes"},
		{Name: "quit", Description: "Quit Gi"},
	}
}

func (h *CLIInteractiveTUIHost) watchEditorAutocompleteProviders() {
	if h == nil || h.runtimeHost == nil {
		return
	}
	providerHost, ok := h.runtimeHost.(protocolExtensionRuntimeProvider)
	if !ok {
		return
	}
	runtime := providerHost.ProtocolExtensionRuntime()
	if runtime == nil {
		return
	}
	h.unwatchCommands = runtime.OnCommandsChanged(func() {
		h.refreshEditorAutocompleteProvider()
		h.requestRender(false)
	})
	h.unwatchRenderers = runtime.OnMessageRenderersChanged(func() {
		go h.rerenderSessionMessages()
	})
	h.unwatchAutocomplete = runtime.OnAutocompleteProvidersChanged(func() {
		h.refreshEditorAutocompleteProvider()
		h.requestRender(false)
	})
}

type protocolAutocompleteTUIProvider struct {
	runtime *ProtocolExtensionRuntime
}

func (p protocolAutocompleteTUIProvider) Suggestions(text string, cursor int) gitui.AutocompleteSuggestions {
	lines, cursorLine, cursorCol := protocolAutocompleteTextCursor(text, cursor)
	result, err := p.suggest(context.Background(), lines, cursorLine, cursorCol, false)
	if err != nil || len(result.Items) == 0 {
		return gitui.AutocompleteSuggestions{Start: cursor, End: cursor}
	}
	return protocolAutocompleteToTUI(result)
}

func (p protocolAutocompleteTUIProvider) GetSuggestionsContext(ctx context.Context, lines []string, cursorLine, cursorCol int, force bool) (*gitui.AutocompleteSuggestions, error) {
	result, err := p.suggest(ctx, lines, cursorLine, cursorCol, force)
	if err != nil || len(result.Items) == 0 {
		return nil, err
	}
	converted := protocolAutocompleteToTUI(result)
	return &converted, nil
}

func (p protocolAutocompleteTUIProvider) ApplyCompletion(lines []string, cursorLine, cursorCol int, item gitui.AutocompleteItem, prefix string) gitui.CompletionResult {
	return protocolAutocompleteApplyCompletion(lines, cursorLine, cursorCol, item, prefix)
}

func (p protocolAutocompleteTUIProvider) suggest(ctx context.Context, lines []string, cursorLine, cursorCol int, force bool) (ProtocolAutocompleteResult, error) {
	if p.runtime == nil {
		return ProtocolAutocompleteResult{}, nil
	}
	lineCopy := append([]string(nil), lines...)
	slashCommand, argumentIndex := protocolAutocompleteSlashArgumentContext(lineCopy, cursorLine, cursorCol)
	return p.runtime.SuggestAutocomplete(ctx, ProtocolAutocompleteRequest{
		Text:          strings.Join(lineCopy, "\n"),
		Lines:         lineCopy,
		CursorLine:    cursorLine,
		CursorCol:     cursorCol,
		Force:         force,
		SlashCommand:  slashCommand,
		ArgumentIndex: argumentIndex,
	})
}

func protocolAutocompleteSlashArgumentContext(lines []string, cursorLine, cursorCol int) (string, int) {
	if cursorLine < 0 || cursorLine >= len(lines) {
		return "", 0
	}
	line := lines[cursorLine]
	if cursorCol < 0 {
		cursorCol = 0
	}
	lineRunes := []rune(line)
	if cursorCol > len(lineRunes) {
		cursorCol = len(lineRunes)
	}
	beforeCursor := string(lineRunes[:cursorCol])
	if !strings.HasPrefix(beforeCursor, "/") {
		return "", 0
	}
	space := strings.IndexAny(beforeCursor, " \t")
	if space <= 1 {
		return "", 0
	}
	commandName := strings.TrimSpace(beforeCursor[1:space])
	if commandName == "" {
		return "", 0
	}
	argumentText := beforeCursor[space+1:]
	fields := strings.Fields(argumentText)
	argumentIndex := len(fields) - 1
	if strings.TrimSpace(argumentText) == "" {
		argumentIndex = 0
	} else if strings.HasSuffix(argumentText, " ") || strings.HasSuffix(argumentText, "\t") {
		argumentIndex = len(fields)
	}
	if argumentIndex < 0 {
		argumentIndex = 0
	}
	return commandName, argumentIndex
}

func protocolAutocompleteToTUI(result ProtocolAutocompleteResult) gitui.AutocompleteSuggestions {
	items := make([]gitui.AutocompleteItem, 0, len(result.Items))
	for _, item := range result.Items {
		value := firstNonEmptyString(item.Value, item.Label, item.ID)
		if value == "" {
			continue
		}
		items = append(items, gitui.AutocompleteItem{
			Value:       value,
			Label:       firstNonEmptyString(item.Label, value),
			Description: item.Description,
		})
	}
	return gitui.AutocompleteSuggestions{
		Items:  items,
		Prefix: result.Prefix,
		Start:  result.Start,
		End:    result.End,
	}
}

func protocolAutocompleteTextCursor(text string, cursor int) ([]string, int, int) {
	runes := []rune(text)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	beforeLines := strings.Split(string(runes[:cursor]), "\n")
	cursorLine := len(beforeLines) - 1
	cursorCol := len([]rune(beforeLines[cursorLine]))
	return strings.Split(text, "\n"), cursorLine, cursorCol
}

func protocolAutocompleteApplyCompletion(lines []string, cursorLine, cursorCol int, item gitui.AutocompleteItem, prefix string) gitui.CompletionResult {
	nextLines := append([]string(nil), lines...)
	if cursorLine < 0 || cursorLine >= len(nextLines) {
		return gitui.CompletionResult{Lines: nextLines, CursorLine: cursorLine, CursorCol: cursorCol}
	}
	lineRunes := []rune(nextLines[cursorLine])
	if cursorCol < 0 {
		cursorCol = 0
	}
	if cursorCol > len(lineRunes) {
		cursorCol = len(lineRunes)
	}
	start := cursorCol - len([]rune(prefix))
	if start < 0 {
		start = cursorCol
	}
	replacement := []rune(item.Value)
	updated := make([]rune, 0, len(lineRunes)-cursorCol+start+len(replacement))
	updated = append(updated, lineRunes[:start]...)
	updated = append(updated, replacement...)
	updated = append(updated, lineRunes[cursorCol:]...)
	nextLines[cursorLine] = string(updated)
	return gitui.CompletionResult{
		Lines:      nextLines,
		CursorLine: cursorLine,
		CursorCol:  start + len(replacement),
	}
}
