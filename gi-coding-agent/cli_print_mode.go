package gicodingagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type agentSessionPrintModeHost struct {
	session             *AgentSession
	modelRegistry       *ModelRegistry
	modelRuntime        *ModelRuntime
	processExtensions   []ProtocolPackageProcessExtension
	processSupervisor   *ProtocolExtensionProcessSupervisor
	sessionRuntimeHost  *AgentSessionRuntimeHost
	settingsManager     *SettingsManager
	extensionFlagValues map[string]any
	startupWarnings     []string
}

func (h *agentSessionPrintModeHost) ProtocolExtensionRuntime() *ProtocolExtensionRuntime {
	if h == nil || h.session == nil {
		return nil
	}
	return h.session.ExtensionRuntime
}

func (h *agentSessionPrintModeHost) AgentSession() *AgentSession {
	if h == nil {
		return nil
	}
	return h.session
}

func (h *agentSessionPrintModeHost) AgentSessionRuntimeHost() *AgentSessionRuntimeHost {
	if h == nil {
		return nil
	}
	return h.sessionRuntimeHost
}

func (h *agentSessionPrintModeHost) SettingsManager() *SettingsManager {
	if h == nil {
		return nil
	}
	return h.settingsManager
}

func (h *agentSessionPrintModeHost) ModelRegistry() *ModelRegistry {
	if h == nil {
		return nil
	}
	return h.modelRegistry
}

func (h *agentSessionPrintModeHost) ModelRuntime() *ModelRuntime {
	if h == nil {
		return nil
	}
	return h.modelRuntime
}

func (h *agentSessionPrintModeHost) StartupWarnings() []string {
	if h == nil || len(h.startupWarnings) == 0 {
		return nil
	}
	return append([]string(nil), h.startupWarnings...)
}

func (h *agentSessionPrintModeHost) ProtocolExtensionProcessSpecs() []ProtocolPackageProcessExtension {
	if h == nil {
		return nil
	}
	return append([]ProtocolPackageProcessExtension(nil), h.processExtensions...)
}

func (h *agentSessionPrintModeHost) SetProtocolExtensionProcessSpecs(specs []ProtocolPackageProcessExtension) {
	if h == nil {
		return
	}
	h.processExtensions = append([]ProtocolPackageProcessExtension(nil), specs...)
}

func (h *agentSessionPrintModeHost) ExtensionFlagValues() map[string]any {
	if h == nil || len(h.extensionFlagValues) == 0 {
		return nil
	}
	return cloneMapAny(h.extensionFlagValues)
}

func (h *agentSessionPrintModeHost) NewProtocolExtensionRPCSessionHost(viewTreeHost *ViewTreeHost, editor TUIEditorHost, dialog TUIDialogHost) *RPCSessionHost {
	if h == nil {
		return nil
	}
	host := NewRPCSessionHost(h.session)
	host.Settings = h.settingsManager
	if h.modelRuntime != nil {
		host.ProviderAuthStatus = h.modelRuntime.GetProviderAuthStatus
		host.AvailableModels = h.modelRuntime.GetAvailable()
	} else if h.modelRegistry != nil {
		host.ProviderAuthStatus = h.modelRegistry.GetProviderAuthStatus
	}
	host.ProcessExecutor = LocalHostProcessExecutor{}
	if viewTreeHost != nil {
		host.ViewTreeHost = viewTreeHost
	}
	host.TUIEditor = editor
	host.TUIDialog = dialog
	if h.sessionRuntimeHost != nil {
		host.ReloadSession = h.sessionRuntimeHost.Reload
	}
	host.OnSessionReplaced(func(session *AgentSession) {
		h.session = session
		if h.sessionRuntimeHost != nil {
			h.sessionRuntimeHost.Session = session
			if h.sessionRuntimeHost.ExtensionRuntime != nil {
				h.sessionRuntimeHost.ExtensionRuntime.BindSession(session)
				h.sessionRuntimeHost.ExtensionRuntime.ApplyToSession(session)
			}
		}
	})
	return host
}

func (h *agentSessionPrintModeHost) ImportFromJsonl(inputPath string, cwdOverride ...string) (InteractiveImportResult, error) {
	if h == nil || h.session == nil || h.session.SessionManager == nil {
		return InteractiveImportResult{}, errors.New("session host requires an active session")
	}
	path := strings.TrimSpace(inputPath)
	if path == "" {
		return InteractiveImportResult{}, errors.New("input path is required")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(h.session.SessionManager.GetCWD(), path)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return InteractiveImportResult{}, err
	}
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return InteractiveImportResult{}, SessionImportFileNotFoundError{Path: inputPath}
		}
		return InteractiveImportResult{}, err
	}

	override := ""
	if len(cwdOverride) > 0 {
		override = strings.TrimSpace(cwdOverride[0])
	}
	oldSession := h.session
	manager, err := OpenSessionManager(absPath, oldSession.SessionManager.GetSessionDir(), override)
	if err != nil {
		return InteractiveImportResult{}, err
	}
	if issue := GetMissingSessionCwdIssue(manager, oldSession.SessionManager.GetCWD()); issue != nil {
		return InteractiveImportResult{}, MissingSessionCwdError{Issue: *issue}
	}
	newSession, err := cloneAgentSessionWithManager(oldSession, manager)
	if err != nil {
		return InteractiveImportResult{}, err
	}
	if h.sessionRuntimeHost != nil {
		if err := h.sessionRuntimeHost.replaceSession(newSession, "import", manager.GetSessionFile(), oldSession.SessionManager.GetSessionFile(), nil); err != nil {
			return InteractiveImportResult{}, err
		}
	} else {
		h.session = newSession
	}
	return InteractiveImportResult{}, nil
}

func runCLIPrintMode(args Args, options CLIOptions) int {
	promptArgs := args
	stdinContent, err := readCLIPipedStdin(options)
	if err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	initial, err := buildCLIPrintModeInitialMessage(&promptArgs, stdinContent, ProcessFileArgumentsOptions{CWD: options.CWD})
	if err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}

	factory := options.PrintModeHostFactory
	if factory == nil {
		factory = func(args Args) (PrintModeRuntimeHost, error) {
			return newDefaultCLIPrintModeHost(args, options)
		}
	}
	host, err := factory(args)
	if err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	if host == nil {
		writeCLIError(options.Stderr, "print mode host is required")
		return 1
	}

	mode := string(args.Mode)
	if mode == "" {
		mode = "text"
	}
	return RunPrintMode(host, PrintModeOptions{
		Mode:           mode,
		InitialMessage: initial.message,
		InitialImages:  initial.images,
		Messages:       promptArgs.Messages,
		Stdout:         options.Stdout,
		Stderr:         options.Stderr,
	})
}

type cliPrintModeInitialMessage struct {
	message string
	images  []llm.ContentPart
}

func buildCLIPrintModeInitialMessage(args *Args, stdinContent *string, options ...ProcessFileArgumentsOptions) (cliPrintModeInitialMessage, error) {
	var fileText string
	var fileImages []llm.ContentPart
	if args != nil && len(args.FileArgs) > 0 {
		processOptions := ProcessFileArgumentsOptions{}
		if len(options) > 0 {
			processOptions = options[0]
		}
		processed, err := ProcessFileArguments(args.FileArgs, processOptions)
		if err != nil {
			return cliPrintModeInitialMessage{}, err
		}
		fileText = processed.Text
		fileImages = processed.Images
	}
	initial := BuildInitialMessage(InitialMessageInput{
		Parsed:       args,
		FileText:     fileText,
		FileImages:   fileImages,
		StdinContent: stdinContent,
	})
	return cliPrintModeInitialMessage{
		message: initial.InitialMessage,
		images:  initial.InitialImages,
	}, nil
}

func newDefaultCLIPrintModeHost(args Args, options CLIOptions) (PrintModeRuntimeHost, error) {
	writeAuth := !args.Offline || args.APIKey != ""
	startupCwd, agentDir, err := resolveCLICWDAndAgentDir(options)
	if err != nil {
		return nil, err
	}
	startupSettingsManager := NewSettingsManager(startupCwd, agentDir)

	sessionManager, err := newCLIPrintModeSessionManager(args, startupCwd, agentDir, startupSettingsManager)
	if err != nil {
		return nil, err
	}
	cwd, err := resolveCLIPrintModeRuntimeCWD(sessionManager, startupCwd)
	if err != nil {
		return nil, err
	}
	settingsManager, projectTrusted, err := newCLIRuntimeSettingsManager(args, cwd, agentDir, options.ProjectTrustPrompt)
	if err != nil {
		return nil, err
	}
	registryOptions := options
	registryOptions.CWD = cwd
	registryOptions.AgentDir = agentDir
	modelRegistry, _, _, err := newCLIModelRegistry(registryOptions, writeAuth)
	if err != nil {
		return nil, err
	}
	modelRuntime, err := NewModelRuntimeFromRegistry(modelRegistry)
	if err != nil {
		return nil, err
	}

	resolvedModel, err := resolveCLIPrintModeModelForSession(args, modelRuntime, settingsManager, sessionManager)
	if err != nil {
		return nil, err
	}
	model := resolvedModel.Model
	thinkingLevel := resolvedModel.ThinkingLevel
	if model == nil {
		return nil, errors.New(formatNoModelsAvailableMessage())
	}
	scopedModels := resolveCLIPrintModeModelScope(args, modelRuntime, settingsManager)
	if args.APIKey != "" {
		if err := modelRuntime.SetRuntimeAPIKey(
			context.Background(),
			model.Provider,
			args.APIKey,
		); err != nil {
			return nil, err
		}
	}
	installTelemetryEnabled := IsInstallTelemetryEnabled(settingsManager)
	resourceLoader := NewDefaultResourceLoader(defaultResourceLoaderOptionsFromCLI(args, cwd, agentDir, settingsManager))
	resourceLoader.Reload()
	extensions := resourceLoader.GetExtensions()
	resourceLoader.ApplyExtensionFlagValues(args.UnknownFlags, cliAllowsDeferredExtensionFlags(args, extensions))
	extensions = resourceLoader.GetExtensions()
	if extensions.Runtime != nil {
		extensions.Runtime.BindModelRuntime(modelRuntime)
	}

	host := &agentSessionPrintModeHost{}
	preflight := cliModelRuntimePreflight(modelRuntime, args)
	retrySettings := settingsManager.GetRetrySettings()
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            cwd,
		AgentDir:       agentDir,
		SessionManager: sessionManager,
		Model:          *model,
		ThinkingLevel:  string(thinkingLevel),
		ScopedModels:   scopedModels,
		Preflight:      preflight,
		ResourceLoader: resourceLoader,
		RetrySettings:  &retrySettings,
		ModelRuntime:   modelRuntime,
		Tools:          args.Tools,
		ToolsSet:       args.Tools != nil,
		NoTools:        cliNoToolsMode(args),
	})
	if err != nil {
		return nil, err
	}
	if options.Responder != nil {
		session.Responder = options.Responder
	} else {
		session.Responder = host.providerResponder(modelRegistry, args, installTelemetryEnabled)
		session.StreamResponder = host.providerStreamResponder(modelRegistry, args, installTelemetryEnabled)
	}
	host.session = session
	host.modelRegistry = modelRegistry
	host.modelRuntime = modelRuntime
	host.settingsManager = settingsManager
	host.extensionFlagValues = cloneMapAny(args.UnknownFlags)
	host.processExtensions = extensions.ProcessExtensions
	host.startupWarnings = startupWarningLines(resolvedModel.Warning)
	if warning := projectTrustStartupWarning(cwd, projectTrusted); warning != "" {
		host.startupWarnings = append(host.startupWarnings, warning)
	}
	if extensions.Runtime != nil {
		runtimeHost, err := NewAgentSessionRuntimeHost(session, extensions.Runtime)
		if err != nil {
			return nil, err
		}
		runtimeHost.SetRebindSession(func(session *AgentSession) error {
			host.session = session
			return nil
		})
		host.sessionRuntimeHost = runtimeHost
	}
	return host, nil
}

func defaultResourceLoaderOptionsFromCLI(args Args, cwd, agentDir string, settingsManager *SettingsManager) DefaultResourceLoaderOptions {
	return DefaultResourceLoaderOptions{
		CWD:                      cwd,
		AgentDir:                 agentDir,
		SettingsManager:          settingsManager,
		NoExtensions:             args.NoExtensions,
		NoContextFiles:           args.NoContextFiles,
		NoSkills:                 args.NoSkills,
		NoPromptTemplates:        args.NoPromptTemplates,
		NoThemes:                 args.NoThemes,
		AdditionalExtensionPaths: args.Extensions,
		AdditionalSkillPaths:     args.Skills,
		AdditionalPromptPaths:    args.PromptTemplates,
		AdditionalThemePaths:     args.Themes,
		SystemPrompt:             args.SystemPrompt,
		AppendSystemPrompt:       args.AppendSystemPrompt,
	}
}

func resolveCLIPrintModeRuntimeCWD(sessionManager *SessionManager, fallbackCwd string) (string, error) {
	if issue := GetMissingSessionCwdIssue(sessionManager, fallbackCwd); issue != nil {
		return "", MissingSessionCwdError{Issue: *issue}
	}
	if sessionManager != nil {
		if cwd := strings.TrimSpace(sessionManager.GetCWD()); cwd != "" {
			return cwd, nil
		}
	}
	return fallbackCwd, nil
}

type cliResolvedModelSelection struct {
	Model         *llm.Model
	ThinkingLevel ThinkingLevel
	Warning       string
}

func cliNoToolsMode(args Args) string {
	if args.NoTools {
		return "all"
	}
	if args.NoBuiltinTools {
		return "builtin"
	}
	return ""
}

func cliAllowsDeferredExtensionFlags(args Args, extensions ResourceExtensionsResult) bool {
	return !args.Print && args.Mode != ModeJSON && args.Mode != ModeRPC && len(extensions.ProcessExtensions) > 0
}

func cliModelRuntimePreflight(
	runtime *ModelRuntime,
	args Args,
) AgentSessionPreflight {
	return func(model llm.Model) error {
		var apiKey *string
		if args.APIKey != "" {
			apiKey = &args.APIKey
		}
		auth, err := runtime.GetAuth(
			context.Background(),
			model,
			llm.AuthResolutionOverrides{APIKey: apiKey},
		)
		if err != nil {
			return err
		}
		if auth == nil {
			return errors.New(
				"Provider is not configured: " + model.Provider,
			)
		}
		if args.APIKey == "" &&
			auth.APIKey == "" &&
			providerNeedsExplicitAPIKey(model.Provider) {
			return errors.New(
				formatNoAPIKeyFoundMessage(model.Provider),
			)
		}
		return nil
	}
}

func resolveCLIPrintModeModel(args Args, registry CodingModelRegistry, settingsManagers ...*SettingsManager) (*llm.Model, ThinkingLevel, error) {
	resolved, err := resolveCLIPrintModeModelForSession(args, registry, firstSettingsManager(settingsManagers), nil)
	if err != nil {
		return nil, resolved.ThinkingLevel, err
	}
	return resolved.Model, resolved.ThinkingLevel, nil
}

func resolveCLIPrintModeModelForSession(args Args, registry CodingModelRegistry, settingsManager *SettingsManager, sessionManager *SessionManager) (cliResolvedModelSelection, error) {
	if registry == nil {
		return cliResolvedModelSelection{ThinkingLevel: DefaultThinkingLevel}, errors.New("model registry is required")
	}
	if args.Model != "" {
		resolved := ResolveCLIModel(ResolveCLIModelOptions{
			CLIProvider:   args.Provider,
			CLIModel:      args.Model,
			ModelRegistry: registry,
		})
		if resolved.Error != "" {
			return cliResolvedModelSelection{ThinkingLevel: DefaultThinkingLevel, Warning: resolved.Warning}, errors.New(resolved.Error)
		}
		level := firstThinkingLevel(args.Thinking, resolved.ThinkingLevel, DefaultThinkingLevel)
		level = clampCLIThinkingLevel(resolved.Model, level)
		return cliResolvedModelSelection{Model: resolved.Model, ThinkingLevel: level, Warning: resolved.Warning}, nil
	}

	defaultModelID := ""
	defaultProvider := args.Provider
	if args.Provider != "" {
		defaultModelID = DefaultModelPerProvider[args.Provider]
	} else if settingsManager != nil {
		defaultProvider = settingsManager.GetDefaultProvider()
		defaultModelID = settingsManager.GetDefaultModel()
	}
	defaultThinkingLevel := firstThinkingLevel(args.Thinking, settingsThinkingLevel(settingsManager), DefaultThinkingLevel)

	var restoreWarning string
	if restored, ok := restoreCLIPrintModeSessionModel(sessionManager, registry, defaultThinkingLevel); ok {
		return restored, nil
	} else {
		restoreWarning = restored.Warning
	}

	scopedModels := resolveCLIPrintModeModelScope(args, registry, settingsManager)
	if len(scopedModels) > 0 && !args.Continue && !args.Resume {
		model, level := selectCLIStartupScopedModel(scopedModels, registry, settingsManager, defaultThinkingLevel)
		level = clampCLIThinkingLevel(model, level)
		restoreWarning = startupRestoreFallbackWarning(restoreWarning, model)
		return cliResolvedModelSelection{
			Model:         model,
			ThinkingLevel: level,
			Warning:       restoreWarning,
		}, nil
	}

	resolved := FindInitialModel(FindInitialModelOptions{
		CLIProvider:          args.Provider,
		ScopedModels:         scopedModels,
		IsContinuing:         args.Continue || args.Resume,
		DefaultProvider:      defaultProvider,
		DefaultModelID:       defaultModelID,
		DefaultThinkingLevel: defaultThinkingLevel,
		ModelRegistry:        registry,
	})
	if resolved.Error != "" {
		return cliResolvedModelSelection{ThinkingLevel: resolved.ThinkingLevel, Warning: restoreWarning}, errors.New(resolved.Error)
	}
	resolved.ThinkingLevel = clampCLIThinkingLevel(resolved.Model, resolved.ThinkingLevel)
	restoreWarning = startupRestoreFallbackWarning(restoreWarning, resolved.Model)
	return cliResolvedModelSelection{
		Model:         resolved.Model,
		ThinkingLevel: resolved.ThinkingLevel,
		Warning:       combineStartupWarnings(restoreWarning, resolved.FallbackMessage),
	}, nil
}

func selectCLIStartupScopedModel(scopedModels []ScopedModel, registry CodingModelRegistry, settingsManager *SettingsManager, defaultThinkingLevel ThinkingLevel) (*llm.Model, ThinkingLevel) {
	scoped := scopedModels[0]
	if settingsManager != nil && registry != nil {
		defaultProvider := settingsManager.GetDefaultProvider()
		defaultModelID := settingsManager.GetDefaultModel()
		if defaultProvider != "" && defaultModelID != "" {
			if found, ok := registry.Find(defaultProvider, defaultModelID); ok {
				for _, candidate := range scopedModels {
					if sameModel(candidate.Model, found) {
						scoped = candidate
						break
					}
				}
			}
		}
	}
	level := scoped.ThinkingLevel
	if level == "" {
		level = defaultThinkingLevel
	}
	if level == "" {
		level = DefaultThinkingLevel
	}
	return modelPtr(scoped.Model), level
}

func firstSettingsManager(settingsManagers []*SettingsManager) *SettingsManager {
	if len(settingsManagers) == 0 {
		return nil
	}
	return settingsManagers[0]
}

func restoreCLIPrintModeSessionModel(sessionManager *SessionManager, registry CodingModelRegistry, defaultThinkingLevel ThinkingLevel) (cliResolvedModelSelection, bool) {
	if sessionManager == nil || registry == nil {
		return cliResolvedModelSelection{}, false
	}
	context := sessionManager.BuildSessionContext()
	if len(context.Messages) == 0 || context.Model == nil || context.Model.Provider == "" || context.Model.ModelID == "" {
		return cliResolvedModelSelection{}, false
	}
	savedProvider := context.Model.Provider
	savedModelID := context.Model.ModelID
	found, ok := registry.Find(savedProvider, savedModelID)
	if ok && modelConfiguredForSessionRestore(registry, found) {
		level := defaultThinkingLevel
		if hasSessionThinkingLevel(sessionManager) && IsValidThinkingLevel(context.ThinkingLevel) {
			level = ThinkingLevel(context.ThinkingLevel)
		}
		if level == "" {
			level = DefaultThinkingLevel
		}
		level = clampCLIThinkingLevel(&found, level)
		return cliResolvedModelSelection{Model: modelPtr(found), ThinkingLevel: level}, true
	}
	reason := "not found"
	if ok {
		reason = "not configured"
	}
	return cliResolvedModelSelection{
		Warning: "Could not restore model " + savedProvider + "/" + savedModelID + " (" + reason + ").",
	}, false
}

func startupRestoreFallbackWarning(warning string, model *llm.Model) string {
	warning = strings.TrimSpace(warning)
	if warning == "" || model == nil {
		return warning
	}
	return strings.TrimSuffix(warning, ".") + ". Using " + model.Provider + "/" + model.ID + "."
}

func modelConfiguredForSessionRestore(registry CodingModelRegistry, model llm.Model) bool {
	if configured, ok := registry.(interface{ HasConfiguredAuth(llm.Model) bool }); ok {
		return configured.HasConfiguredAuth(model)
	}
	return true
}

func hasSessionThinkingLevel(sessionManager *SessionManager) bool {
	if sessionManager == nil {
		return false
	}
	for _, entry := range sessionManager.GetBranch() {
		if entry.Type == "thinking_level_change" && IsValidThinkingLevel(entry.ThinkingLevel) {
			return true
		}
	}
	return false
}

func combineStartupWarnings(warnings ...string) string {
	return strings.Join(startupWarningLines(strings.Join(warnings, "\n")), "\n")
}

func startupWarningLines(warning string) []string {
	if strings.TrimSpace(warning) == "" {
		return nil
	}
	rawLines := strings.Split(warning, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func resolveCLIPrintModeModelScope(args Args, registry CodingModelRegistry, settingsManager *SettingsManager) []ScopedModel {
	patterns := args.Models
	if len(patterns) == 0 && settingsManager != nil {
		patterns = settingsManager.GetEnabledModels()
	}
	return ResolveModelScope(patterns, registry)
}

func settingsThinkingLevel(settingsManager *SettingsManager) ThinkingLevel {
	if settingsManager == nil {
		return ""
	}
	level := ThinkingLevel(settingsManager.GetDefaultThinkingLevel())
	if !IsValidThinkingLevel(string(level)) {
		return ""
	}
	return level
}

func firstThinkingLevel(levels ...ThinkingLevel) ThinkingLevel {
	for _, level := range levels {
		if level != "" {
			return level
		}
	}
	return ""
}

func clampCLIThinkingLevel(model *llm.Model, level ThinkingLevel) ThinkingLevel {
	if model == nil {
		return ThinkingOff
	}
	if level == "" {
		return ""
	}
	return ThinkingLevel(llm.ClampThinkingLevel(*model, string(level)))
}

func cliReasoningOption(model llm.Model, level ThinkingLevel) string {
	if level == "" {
		return ""
	}
	clamped := llm.ClampThinkingLevel(model, string(level))
	if clamped == string(ThinkingOff) {
		return ""
	}
	return clamped
}

func (h *agentSessionPrintModeHost) PrintModeSession() PrintModeSession {
	return h
}

func (h *agentSessionPrintModeHost) Dispose() error {
	if h != nil && h.processSupervisor != nil {
		if err := h.processSupervisor.Stop(context.Background()); err != nil {
			return err
		}
		h.processSupervisor = nil
	}
	if h != nil && h.session != nil {
		h.session.Dispose()
	}
	return nil
}

func (h *agentSessionPrintModeHost) Prompt(message string, options PrintModePromptOptions) error {
	if h == nil || h.session == nil {
		return errors.New("session is required")
	}
	if len(options.Images) == 0 {
		return h.session.Prompt(message)
	}
	// AgentSession currently stores text user turns. Preserve the visible text
	// path until the session message model grows first-class image turns.
	return h.session.Prompt(message)
}

func (h *agentSessionPrintModeHost) WaitForIdle() error {
	if h == nil || h.session == nil {
		return nil
	}
	return h.session.WaitForIdle(context.Background())
}

func (h *agentSessionPrintModeHost) Messages() []llm.Message {
	if h == nil || h.session == nil {
		return nil
	}
	return h.session.Messages()
}

func (h *agentSessionPrintModeHost) PrintModeCWD() string {
	if h == nil || h.session == nil || h.session.SessionManager == nil {
		return ""
	}
	return h.session.SessionManager.GetCWD()
}

func (h *agentSessionPrintModeHost) providerResponder(registry *ModelRegistry, args Args, installTelemetryEnabled bool) AgentSessionResponder {
	return func(_ string, messages []llm.Message, model llm.Model) (llm.Message, error) {
		ctx := context.Background()
		runtime, err := h.modelRuntimeForRegistry(registry)
		if err != nil {
			return llm.Message{}, err
		}
		options := h.modelRuntimeStreamOptions(
			ctx,
			model,
			args,
			installTelemetryEnabled,
		)
		return runtime.CompleteSimple(ctx, model, llm.Context{
			SystemPrompt: h.session.SystemPrompt,
			Messages:     messages,
			Tools:        h.session.GetActiveLLMTools(),
		}, options)
	}
}

func (h *agentSessionPrintModeHost) providerStreamResponder(registry *ModelRegistry, args Args, installTelemetryEnabled bool) AgentSessionStreamResponder {
	return func(_ string, messages []llm.Message, model llm.Model) (*llm.AssistantMessageEventStream, error) {
		ctx := context.Background()
		runtime, err := h.modelRuntimeForRegistry(registry)
		if err != nil {
			return nil, err
		}
		options := h.modelRuntimeStreamOptions(
			ctx,
			model,
			args,
			installTelemetryEnabled,
		)
		return runtime.StreamSimple(ctx, model, llm.Context{
			SystemPrompt: h.session.SystemPrompt,
			Messages:     messages,
			Tools:        h.session.GetActiveLLMTools(),
		}, options)
	}
}

func (h *agentSessionPrintModeHost) modelRuntimeForRegistry(
	registry *ModelRegistry,
) (*ModelRuntime, error) {
	if h.modelRuntime != nil &&
		h.modelRuntime.ModelRegistry() == registry {
		return h.modelRuntime, nil
	}
	runtime, err := NewModelRuntimeFromRegistry(registry)
	if err != nil {
		return nil, err
	}
	h.modelRuntime = runtime
	return runtime, nil
}

func (h *agentSessionPrintModeHost) modelRuntimeStreamOptions(
	ctx context.Context,
	model llm.Model,
	args Args,
	installTelemetryEnabled bool,
) llm.ModelsStreamOptions {
	providerRetrySettings := ProviderRetrySettings{
		MaxRetryDelayMS: defaultProviderMaxRetryDelayMS,
	}
	if h.settingsManager != nil {
		providerRetrySettings =
			h.settingsManager.GetProviderRetrySettings()
	}
	options := llm.ModelsStreamOptions{
		StreamOptions: llm.StreamOptions{
			Context: ctx,
			Reasoning: cliReasoningOption(
				model,
				ThinkingLevel(
					h.session.Agent.State.ThinkingLevel,
				),
			),
			Transport:       settingsTransport(h.settingsManager),
			TimeoutMillis:   providerRetrySettings.TimeoutMS,
			MaxRetries:      providerRetrySettings.MaxRetries,
			MaxRetryDelayMs: providerRetrySettings.MaxRetryDelayMS,
			OnPayload: func(
				payload any,
				model llm.Model,
			) (any, bool, error) {
				if h.session == nil {
					return nil, false, nil
				}
				return h.session.emitBeforeProviderRequest(
					ctx,
					payload,
					model,
				)
			},
			OnResponseStatus: func(
				status int,
				headers map[string]string,
				model llm.Model,
			) error {
				if h.session == nil {
					return nil
				}
				return h.session.emitAfterProviderResponse(
					ctx,
					status,
					headers,
					model,
				)
			},
		},
		TransformHeaders: func(
			_ context.Context,
			headers map[string]string,
		) (map[string]string, error) {
			return BuildSDKStreamHeaders(
				model,
				installTelemetryEnabled,
				headers,
				nil,
			), nil
		},
	}
	if args.APIKey != "" {
		apiKey := args.APIKey
		options.APIKey = &apiKey
	}
	return options
}

func settingsTransport(settings *SettingsManager) string {
	if settings == nil {
		return ""
	}
	return settings.GetTransport()
}
