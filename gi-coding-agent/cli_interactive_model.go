package gicodingagent

import (
	"errors"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func (h *CLIInteractiveTUIHost) modelRegistry() *ModelRegistry {
	if h == nil || h.runtimeHost == nil {
		return nil
	}
	provider, ok := h.runtimeHost.(modelRegistryProvider)
	if !ok {
		return nil
	}
	return provider.ModelRegistry()
}

func (h *CLIInteractiveTUIHost) modelRuntime() *ModelRuntime {
	if h == nil || h.runtimeHost == nil {
		return nil
	}
	provider, ok := h.runtimeHost.(modelRuntimeProvider)
	if !ok {
		return nil
	}
	return provider.ModelRuntime()
}

func markdownTableValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "(unset)"
	}
	return strings.ReplaceAll(value, "|", "\\|")
}

func (h *CLIInteractiveTUIHost) handleModelSlashCommand(args string) error {
	host, err := h.newRPCSessionHost()
	if err != nil {
		return err
	}
	args = strings.TrimSpace(args)
	if args == "" {
		if !h.exitAfterInitial {
			return h.handleModelSelectDialog(host, "")
		}
		result, err := host.CycleModel()
		if err != nil {
			return err
		}
		if result == nil {
			h.addStatus("No models available")
			return nil
		}
		h.updateEditorBorderColor()
		h.addStatus(formatModelCycleStatus(result.Model, result.ThinkingLevel))
		h.maybeWarnAboutAnthropicSubscriptionAuth(result.Model)
		h.checkDaxnutsEasterEgg(result.Model)
		return nil
	}
	parsed := ParseModelPattern(args, modelCommandCandidates(host), ModelPatternOptions{StrictInvalidThinkingLevel: true})
	if parsed.Model == nil {
		if !h.exitAfterInitial {
			return h.handleModelSelectDialog(host, args)
		}
		return errors.New("Model not found: " + args)
	}
	model, err := host.SetModel(parsed.Model.Provider, parsed.Model.ID)
	if err != nil {
		return err
	}
	if parsed.ThinkingLevel != "" {
		if err := host.SetThinkingLevel(string(parsed.ThinkingLevel)); err != nil {
			return err
		}
	}
	h.updateEditorBorderColor()
	if strings.TrimSpace(parsed.Warning) != "" {
		h.addStatus("Warning: " + parsed.Warning)
	}
	h.addStatus(formatModelSelectionStatus(model))
	h.maybeWarnAboutAnthropicSubscriptionAuth(model)
	h.checkDaxnutsEasterEgg(model)
	return nil
}

func modelCommandCandidates(host *RPCSessionHost) []llm.Model {
	if host == nil {
		return nil
	}
	if len(host.ScopedModels) == 0 {
		return host.getAvailableModels()
	}
	models := make([]llm.Model, 0, len(host.ScopedModels))
	for _, scoped := range host.ScopedModels {
		models = append(models, scoped.Model)
	}
	return models
}

func (h *CLIInteractiveTUIHost) handleScopedModelsSlashCommand() error {
	host, err := h.newRPCSessionHost()
	if err != nil {
		return err
	}
	session := host.Session
	allModels := host.getAvailableModels()
	var configuredPatterns []string
	if settings := h.settingsManager(); settings != nil {
		configuredPatterns = settings.GetEnabledModels()
	}
	enabledIDs, hasSelectorState := scopedModelsSelectorEnabledIDs(
		session,
		configuredPatterns,
		allModels,
	)
	if !hasSelectorState {
		h.addStatus("No models available")
		return nil
	}
	selector := NewScopedModelsSelectorComponent(ScopedModelsSelectorConfig{
		AllModels:       allModels,
		EnabledModelIDs: enabledIDs,
		Keybindings:     h.effectiveKeybindings(),
	}, ScopedModelsSelectorCallbacks{})
	if h.ui == nil {
		return errors.New("interactive TUI is not ready")
	}
	replacement := &cliEditorReplacementLifecycle{}
	closeSelector := func() {
		if !replacement.close() {
			h.requestRender(false)
		}
	}
	selector.callbacks.OnChange = func(enabled []string) {
		setSessionScopedModelsFromEnabledIDs(session, allModels, enabled)
	}
	selector.callbacks.OnPersist = func(enabled []string) {
		settings := h.settingsManager()
		if settings == nil {
			h.addStatus("Model selection not saved: settings unavailable")
			return
		}
		allIDs := make([]string, 0, len(allModels))
		for _, model := range allModels {
			allIDs = append(allIDs, scopedModelFullID(model))
		}
		if enabled == nil ||
			enabledIDsAreAllAvailable(enabled, allIDs) {
			settings.SetEnabledModels(nil)
		} else {
			settings.SetEnabledModels(enabled)
		}
		h.addStatus("Model selection saved to settings")
		h.requestRender(false)
	}
	selector.callbacks.OnCancel = closeSelector
	replacement.install(h.showEditorReplacement(selector, selector))
	return nil
}

func scopedModelsSelectorEnabledIDs(
	session *AgentSession,
	configuredPatterns []string,
	allModels []llm.Model,
) ([]string, bool) {
	if len(allModels) == 0 &&
		len(configuredPatterns) == 0 &&
		(session == nil || len(session.ScopedModels) == 0) {
		return nil, false
	}
	var configuredScope ResolveModelScopeResult
	if len(configuredPatterns) > 0 {
		configuredScope = ResolveModelScopeWithDiagnostics(
			configuredPatterns,
			cliModelSliceRegistry(allModels),
		)
	}
	var ids []string
	if session != nil && len(session.ScopedModels) > 0 {
		ids = make([]string, 0, len(session.ScopedModels))
		for _, scoped := range session.ScopedModels {
			ids = append(ids, scopedModelFullID(scoped.Model))
		}
	} else if len(configuredPatterns) > 0 {
		ids = make([]string, 0, len(configuredScope.ScopedModels))
		for _, scoped := range configuredScope.ScopedModels {
			ids = append(ids, scopedModelFullID(scoped.Model))
		}
	}
	for _, diagnostic := range configuredScope.Diagnostics {
		if diagnostic.Code != ModelScopeDiagnosticNoMatch ||
			containsString(ids, diagnostic.Pattern) {
			continue
		}
		ids = append(ids, diagnostic.Pattern)
	}
	return ids, true
}

func setSessionScopedModelsFromEnabledIDs(session *AgentSession, allModels []llm.Model, enabled []string) {
	if session == nil {
		return
	}
	allIDs := make([]string, 0, len(allModels))
	hasEnabledAvailableModel := false
	for _, model := range allModels {
		id := scopedModelFullID(model)
		allIDs = append(allIDs, id)
		if containsString(enabled, id) {
			hasEnabledAvailableModel = true
		}
	}
	allAvailableModelsEnabled := enabled != nil
	for _, id := range allIDs {
		if !containsString(enabled, id) {
			allAvailableModelsEnabled = false
			break
		}
	}
	if enabled != nil &&
		hasEnabledAvailableModel &&
		!allAvailableModelsEnabled {
		session.SetScopedModels(ResolveModelScope(enabled, cliModelSliceRegistry(allModels)))
		return
	}
	session.SetScopedModels(nil)
}

type cliModelSliceRegistry []llm.Model

func (r cliModelSliceRegistry) GetAll() []llm.Model {
	return append([]llm.Model(nil), r...)
}

func (r cliModelSliceRegistry) GetAvailable() []llm.Model {
	return append([]llm.Model(nil), r...)
}

func (r cliModelSliceRegistry) Find(provider, modelID string) (llm.Model, bool) {
	for _, model := range r {
		if model.Provider == provider && model.ID == modelID {
			return model, true
		}
	}
	return llm.Model{}, false
}

func (h *CLIInteractiveTUIHost) handleModelSelectDialog(host *RPCSessionHost, search string) error {
	if h.ui != nil {
		return h.showModelSelector(host, search)
	}
	options, defaultValue := modelSelectDialogOptions(host, search)
	if len(options) == 0 {
		if strings.TrimSpace(search) == "" {
			h.addStatus("No models available")
		} else {
			h.addStatus("No models match: " + search)
		}
		return nil
	}
	result, err := h.RunTUIDialog(TUIDialogRequest{
		Kind:         "select",
		Title:        "Select model",
		Message:      modelSelectDialogMessage(search),
		Options:      options,
		DefaultValue: defaultValue,
	})
	if err != nil {
		return err
	}
	if result.Action != "selected" {
		h.addStatus("Model selection cancelled")
		return nil
	}
	provider, modelID, ok := splitModelReference(dialogStringValue(result.Value))
	if !ok {
		return errors.New("invalid model selection")
	}
	return h.applyModelSelection(host, provider, modelID)
}

func (h *CLIInteractiveTUIHost) showModelSelector(host *RPCSessionHost, search string) error {
	if host == nil || host.Session == nil || host.Session.Agent == nil {
		return errors.New("model selector requires a session host")
	}
	allModels := host.getAvailableModels()
	runtime := h.modelRuntime()
	if len(allModels) == 0 && runtime == nil {
		h.addStatus("No models available")
		return nil
	}
	replacement := &cliEditorReplacementLifecycle{}
	closeSelector := func() {
		if !replacement.close() {
			h.requestRender(false)
		}
	}
	callbacks := ModelSelectorCallbacks{
		OnSelect: func(model llm.Model) {
			closeSelector()
			if err := h.applyModelSelection(
				host,
				model.Provider,
				model.ID,
			); err != nil {
				h.addStatus(
					"Model selection failed: " + err.Error(),
				)
			}
		},
		OnCancel: closeSelector,
	}
	var selectorRuntime ModelSelectorRuntime
	refreshOptions := ModelRegistryRefreshOptions{}
	if runtime != nil {
		selectorRuntime = runtime
		refreshOptions = runtime.defaultRefreshOptions()
		refreshOptions.Timeout = modelSelectorRefreshTimeout
	}
	selector := NewInteractiveModelSelectorComponent(ModelSelectorConfig{
		CurrentModel:   host.Session.Agent.State.Model,
		AllModels:      allModels,
		ScopedModels:   scopedModelsFromRPC(host.ScopedModels),
		InitialSearch:  search,
		Keybindings:    h.effectiveKeybindings(),
		Runtime:        selectorRuntime,
		RefreshOptions: refreshOptions,
		RequestRender: func() {
			h.requestRender(false)
		},
	}, callbacks)
	replacement.install(h.showEditorReplacement(selector, selector))
	return nil
}

func scopedModelsFromRPC(scopedModels []RPCScopedModel) []ScopedModel {
	if len(scopedModels) == 0 {
		return nil
	}
	result := make([]ScopedModel, 0, len(scopedModels))
	for _, scoped := range scopedModels {
		result = append(result, ScopedModel{
			Model:         scoped.Model,
			ThinkingLevel: ThinkingLevel(scoped.ThinkingLevel),
		})
	}
	return result
}

func (h *CLIInteractiveTUIHost) applyModelSelection(host *RPCSessionHost, provider, modelID string) error {
	model, err := selectModelFromDialog(host, provider, modelID)
	if err != nil {
		return err
	}
	if settings := h.settingsManager(); settings != nil {
		settings.SetDefaultProvider(model.Provider)
		settings.SetDefaultModel(model.ID)
	}
	h.updateEditorBorderColor()
	h.addStatus(formatModelSelectionStatus(model))
	h.maybeWarnAboutAnthropicSubscriptionAuth(model)
	h.checkDaxnutsEasterEgg(model)
	return nil
}

func formatModelSelectionStatus(model llm.Model) string {
	return "Model: " + model.ID
}

func (h *CLIInteractiveTUIHost) checkDaxnutsEasterEgg(model llm.Model) {
	if h == nil || h.chat == nil {
		return
	}
	if model.Provider == "opencode" && strings.Contains(strings.ToLower(model.ID), "kimi-k2.5") {
		h.chat.AddChild(gitui.NewSpacer(1))
		h.chat.AddChild(newCLIDaxnutsComponent(h.ui))
		h.requestRender(false)
	}
}

func formatModelCycleStatus(model llm.Model, thinkingLevel string) string {
	name := strings.TrimSpace(model.Name)
	if name == "" {
		name = model.ID
	}
	suffix := ""
	if model.Reasoning && thinkingLevel != "" && thinkingLevel != "off" {
		suffix = " (thinking: " + thinkingLevel + ")"
	}
	return "Switched to " + name + suffix
}

func modelSelectDialogOptions(host *RPCSessionHost, search string) ([]TUIDialogOption, string) {
	if host == nil || host.Session == nil || host.Session.Agent == nil {
		return nil, ""
	}
	currentID := scopedModelFullID(host.Session.Agent.State.Model)
	filter := strings.ToLower(strings.TrimSpace(search))
	models := host.getAvailableModels()
	scoped := map[string]RPCScopedModel{}
	for _, scopedModel := range host.ScopedModels {
		id := scopedModelFullID(scopedModel.Model)
		scoped[id] = scopedModel
	}
	if len(host.ScopedModels) > 0 {
		models = models[:0]
		for _, scopedModel := range host.ScopedModels {
			models = append(models, scopedModel.Model)
		}
	}
	options := make([]TUIDialogOption, 0, len(models))
	for _, model := range models {
		id := scopedModelFullID(model)
		if filter != "" && !modelMatchesDialogSearch(model, filter) {
			continue
		}
		label := model.ID
		if id == currentID {
			label += " *"
		}
		description := model.Provider
		if scopedModel, ok := scoped[id]; ok && strings.TrimSpace(scopedModel.ThinkingLevel) != "" {
			description += " | thinking: " + scopedModel.ThinkingLevel
		}
		if strings.TrimSpace(model.Name) != "" && model.Name != model.ID {
			description += " | " + model.Name
		}
		options = append(options, TUIDialogOption{
			ID:          id,
			Label:       label,
			Description: description,
			Value:       id,
		})
	}
	return options, currentID
}

func modelMatchesDialogSearch(model llm.Model, filter string) bool {
	haystack := strings.ToLower(strings.Join([]string{model.Provider, model.ID, model.Name, scopedModelFullID(model)}, "\x00"))
	return strings.Contains(haystack, filter)
}

func modelSelectDialogMessage(search string) string {
	search = strings.TrimSpace(search)
	if search == "" {
		return "Choose a model for this session."
	}
	return "Showing models matching: " + search
}

func splitModelReference(value string) (string, string, bool) {
	parts := strings.SplitN(strings.TrimSpace(value), "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func selectModelFromDialog(host *RPCSessionHost, provider, modelID string) (llm.Model, error) {
	if host == nil {
		return llm.Model{}, errors.New("model selector requires a session host")
	}
	for _, scoped := range host.ScopedModels {
		if scoped.Model.Provider == provider && scoped.Model.ID == modelID {
			return host.applyModel(scoped.Model, "select", host.thinkingLevelForModelSwitch(scoped.ThinkingLevel))
		}
	}
	return host.SetModel(provider, modelID)
}
