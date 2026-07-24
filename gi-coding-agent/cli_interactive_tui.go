package gicodingagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

type CLIInteractiveTUIHostOptions struct {
	RuntimeHost         PrintModeRuntimeHost
	InitialMessage      string
	InitialImages       []llm.ContentPart
	Messages            []string
	Terminal            gitui.Terminal
	ViewTreeHost        *ViewTreeHost
	InProcessUI         *InProcessUIRegistry
	ClipboardCopy       func(string) error
	ClipboardImageRead  func() *ClipboardImage
	ClipboardImageDir   string
	ShareCreateGist     func(context.Context, string) (string, error)
	Suspend             InteractiveSuspendOperations
	TmuxOptionReader    TmuxOptionReader
	VerboseStartup      bool
	ExitAfterInitial    bool
	ClearScreenOnStart  bool
	ShowFooter          bool
	Version             string
	PackageName         string
	InstallEnvironment  InstallEnvironment
	VersionCheck        VersionReleaseChecker
	VersionCheckOptions VersionCheckOptions
	PackageUpdateCheck  PackageUpdateChecker
	BashOperations      BashOperations
	ShutdownSignals     <-chan os.Signal
	StartupWarnings     []string
}

type PackageUpdateChecker func() ([]string, error)

type protocolExtensionRuntimeProvider interface {
	ProtocolExtensionRuntime() *ProtocolExtensionRuntime
}

type protocolExtensionProcessProvider interface {
	ProtocolExtensionProcessSpecs() []ProtocolPackageProcessExtension
	NewProtocolExtensionRPCSessionHost(*ViewTreeHost, TUIEditorHost, TUIDialogHost) *RPCSessionHost
}

type agentSessionProvider interface {
	AgentSession() *AgentSession
}

type agentSessionRuntimeHostProvider interface {
	AgentSessionRuntimeHost() *AgentSessionRuntimeHost
}

type interactiveImportRuntimeHost interface {
	ImportFromJsonl(inputPath string, cwdOverride ...string) (InteractiveImportResult, error)
}

type settingsManagerProvider interface {
	SettingsManager() *SettingsManager
}

type modelRegistryProvider interface {
	ModelRegistry() *ModelRegistry
}

type modelRuntimeProvider interface {
	ModelRuntime() *ModelRuntime
}

type availableModelsProvider interface {
	GetAvailable() []llm.Model
}

type startupWarningsProvider interface {
	StartupWarnings() []string
}

type CLIInteractiveTUIHost struct {
	runtimeHost         PrintModeRuntimeHost
	initialMessage      string
	initialImages       []llm.ContentPart
	messages            []string
	terminal            gitui.Terminal
	viewTreeHost        *ViewTreeHost
	inProcessUI         *InProcessUIRegistry
	clipboardCopy       func(string) error
	clipboardImageRead  func() *ClipboardImage
	clipboardImageDir   string
	shareCreateGist     func(context.Context, string) (string, error)
	suspend             InteractiveSuspendOperations
	tmuxOptionReader    TmuxOptionReader
	verboseStartup      bool
	exitAfterInitial    bool
	clearScreenOnStart  bool
	showFooter          bool
	version             string
	packageName         string
	installEnvironment  InstallEnvironment
	versionCheck        VersionReleaseChecker
	versionCheckOptions VersionCheckOptions
	packageUpdateCheck  PackageUpdateChecker
	bashOperations      BashOperations
	shutdownSignals     <-chan os.Signal
	startupWarnings     []string

	ui                      *gitui.TUI
	chat                    *gitui.Container
	pendingMessages         *gitui.Container
	statusContainer         *gitui.Container
	editor                  *gitui.Editor
	editorContainer         *gitui.Container
	startupHeader           *cliStartupHeaderComponent
	footer                  *FooterComponent
	footerDataProvider      *FooterDataProvider
	layout                  *cliInteractiveLayout
	customEditorActive      bool
	autocompleteProvider    gitui.AutocompleteProvider
	lastStatusText          *gitui.Text
	lastStatusSpacer        *gitui.Spacer
	loader                  *gitui.Loader
	compactionLoader        *gitui.Loader
	workingMessage          string
	workingIndicator        *TUIWorkingIndicatorOptions
	workingVisible          bool
	retryLoader             *gitui.Loader
	retryStatus             *gitui.Text
	hiddenThinkingLabel     string
	slots                   map[string]*gitui.Container
	views                   map[string]*ViewTreeComponent
	overlays                map[string]gitui.OverlayHandle
	inProcessSlots          map[string][]gitui.Component
	inProcessMounts         map[string]inProcessMountedComponent
	pendingTools            map[string]*ToolExecutionComponent
	keybindings             KeybindingsConfig
	previousTUIKeybindings  *gitui.KeybindingsManager
	tuiKeybindingsInstalled bool
	compactionQueue         []cliCompactionQueuedMessage
	toolOutputExpanded      bool
	startupResources        []*cliLoadedResourcesComponent
	rendered                int
	renderDeferred          atomic.Bool
	viewTreeTickStarted     atomic.Bool
	done                    chan struct{}
	once                    sync.Once
	uiReady                 chan struct{}
	uiReadyOnce             sync.Once
	mu                      sync.Mutex
	lastClearKeyTime        time.Time
	lastEscapeKeyTime       time.Time
	unwatch                 func()
	unwatchCommands         func()
	unwatchAutocomplete     func()
	unwatchRenderers        func()
	unwatchFooterBranch     func()
	unwatchSession          func()
	sessionWatchMu          sync.Mutex
	sessionWatchClosed      bool
	unwatchInProcess        func()
	unwatchRuntimeSession   func()
	restoreRuntimeRebind    func()
	unwatchProtocolErrors   map[*ProtocolExtensionRuntime]func()
	processSupervisor       *ProtocolExtensionProcessSupervisor
	startupNoticesShown     bool
	anthropicWarningShown   bool
	streamingMessage        *llm.Message
	streamingComponent      *cliAssistantMessageComponent
	activePromptMu          sync.Mutex
	activePromptCount       int
	themePreviewActive      bool
	themePreviewName        string
	deadTerminal            atomic.Bool
}

type inProcessMountedComponent struct {
	slot          string
	version       int
	component     gitui.Component
	dispose       func()
	overlay       *gitui.OverlayOptions
	overlayHandle gitui.OverlayHandle
}

type cliInteractiveLayout struct {
	host *CLIInteractiveTUIHost
}

type cliCompactionQueuedMessage struct {
	Text   string
	Images []llm.ContentPart
	Mode   string
}

type cliStartupHeaderComponent struct {
	version     string
	keybindings KeybindingsConfig
	expanded    atomic.Bool
}

func newCLIStartupHeaderComponent(version string, expanded bool, keybindings ...KeybindingsConfig) *cliStartupHeaderComponent {
	effective := DefaultProtocolKeybindings()
	if len(keybindings) > 0 && keybindings[0] != nil {
		effective = cloneKeybindingsConfig(keybindings[0])
	}
	component := &cliStartupHeaderComponent{version: strings.TrimSpace(version), keybindings: effective}
	component.expanded.Store(expanded)
	return component
}

func (c *cliStartupHeaderComponent) SetExpanded(expanded bool) {
	if c != nil {
		c.expanded.Store(expanded)
	}
}

func (c *cliStartupHeaderComponent) SetKeybindings(keybindings KeybindingsConfig) {
	if c != nil && keybindings != nil {
		c.keybindings = cloneKeybindingsConfig(keybindings)
	}
}

func (c *cliStartupHeaderComponent) Invalidate() {}

func (c *cliStartupHeaderComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	label := tuiThemeBoldAccent("gi")
	if c.version != "" {
		label += tuiThemeDim(" v" + c.version)
	}
	onboarding := tuiThemeDim("Gi can explain its own features and look up its docs. Ask it how to use or extend Gi.")
	var text string
	if c.expanded.Load() {
		text = strings.Join([]string{
			label,
			c.appHint("app.interrupt", "escape", "to interrupt"),
			c.appHint("app.clear", "ctrl+c", "to clear"),
			tuiThemeKeyHint(c.appKey("app.clear", "ctrl+c")+" twice", "to exit"),
			c.appHint("app.exit", "ctrl+d", "to exit (empty)"),
			c.appHint("app.suspend", "ctrl+z", "to suspend"),
			c.tuiHint("tui.editor.deleteToLineEnd", "ctrl+k", "to delete to end"),
			c.appHint("app.thinking.cycle", "shift+tab", "to cycle thinking level"),
			tuiThemeKeyHint(c.appKey("app.model.cycleForward", "ctrl+p")+"/"+c.appKey("app.model.cycleBackward", "shift+ctrl+p"), "to cycle models"),
			c.appHint("app.model.select", "ctrl+l", "to select model"),
			c.appHint("app.tools.expand", "ctrl+o", "to expand tools"),
			c.appHint("app.thinking.toggle", "ctrl+t", "to expand thinking"),
			c.appHint("app.editor.external", "ctrl+g", "for external editor"),
			tuiThemeKeyHint("/", "for commands"),
			tuiThemeKeyHint("!", "to run bash"),
			tuiThemeKeyHint("!!", "to run bash (no context)"),
			c.appHint("app.message.followUp", "option+enter", "to queue follow-up"),
			c.appHint("app.message.dequeue", "option+up", "to edit all queued messages"),
			c.appHint("app.clipboard.pasteImage", "ctrl+v", "to paste image"),
			tuiThemeKeyHint("drop files", "to attach"),
			"",
			onboarding,
		}, "\n")
	} else {
		compact := strings.Join([]string{
			c.appHint("app.interrupt", "escape", "interrupt"),
			tuiThemeKeyHint(c.appKey("app.clear", "ctrl+c")+"/"+c.appKey("app.exit", "ctrl+d"), "clear/exit"),
			tuiThemeKeyHint("/", "commands"),
			tuiThemeKeyHint("!", "bash"),
			c.appHint("app.tools.expand", "ctrl+o", "more"),
		}, tuiThemeMuted(" · "))
		text = label + "\n" +
			compact + "\n" +
			tuiThemeDim("Press "+c.appKey("app.tools.expand", "ctrl+o")+" to show full startup help and loaded resources.") + "\n\n" +
			onboarding
	}
	lines := gitui.NewText(text, 1, 0).Render(width)
	result := make([]string, 0, len(lines)+2)
	result = append(result, "")
	result = append(result, lines...)
	result = append(result, "")
	return result
}

func (c *cliStartupHeaderComponent) appHint(action, fallback, description string) string {
	return tuiThemeKeyHint(c.appKey(action, fallback), description)
}

func (c *cliStartupHeaderComponent) tuiHint(action, fallback, description string) string {
	return tuiThemeKeyHint(c.tuiKey(action, fallback), description)
}

func (c *cliStartupHeaderComponent) appKey(action, fallback string) string {
	keybindings := DefaultProtocolKeybindings()
	if c != nil && c.keybindings != nil {
		keybindings = c.keybindings
	}
	return firstNonEmptyString(formatHotkeyKeys(keybindingValueKeys(keybindings[action]), false), fallback)
}

func (c *cliStartupHeaderComponent) tuiKey(action, fallback string) string {
	return firstNonEmptyString(formatHotkeyKeys(gitui.GetKeybindings().GetKeys(action), false), fallback)
}

type cliInteractiveSuspendUI struct {
	ui *gitui.TUI
}

func (u cliInteractiveSuspendUI) Start() {
	if u.ui != nil {
		u.ui.Start()
	}
}

func (u cliInteractiveSuspendUI) Stop() {
	if u.ui != nil {
		u.ui.Stop()
	}
}

func (u cliInteractiveSuspendUI) RequestRender(force ...bool) {
	if u.ui != nil {
		u.ui.RequestRender(force...)
	}
}

func (u cliInteractiveSuspendUI) Terminal() InteractiveTerminal {
	if u.ui == nil {
		return nil
	}
	return cliInteractiveSuspendTerminal{terminal: u.ui.Terminal()}
}

type cliInteractiveSuspendTerminal struct {
	terminal gitui.Terminal
}

func (t cliInteractiveSuspendTerminal) SetProgress(active bool) {
	if t.terminal != nil {
		_ = t.terminal.SetProgress(active)
	}
}

func (l *cliInteractiveLayout) Invalidate() {
	if l == nil || l.host == nil {
		return
	}
	if l.host.chat != nil {
		l.host.chat.Invalidate()
	}
	for _, slot := range l.host.slots {
		slot.Invalidate()
	}
	if l.host.editorContainer != nil {
		l.host.editorContainer.Invalidate()
	}
	if l.host.statusContainer != nil {
		l.host.statusContainer.Invalidate()
	}
}

func (l *cliInteractiveLayout) Render(width int) []string {
	return l.RenderWithSize(width, 0)
}

func (l *cliInteractiveLayout) RenderWithSize(width, height int) []string {
	if l == nil || l.host == nil {
		return nil
	}
	width = max(20, width)

	var top []string
	top = appendRendered(top, l.host.startupHeader, width, height)
	top = appendRendered(top, l.host.slots["header"], width, height)
	top = appendRendered(top, l.host.chat, width, height)

	bottom := []string{""}
	bottom = appendRendered(bottom, l.host.slots["aboveEditor"], width, height)
	bottom = appendRendered(bottom, l.host.pendingMessages, width, height)
	bottom = appendRendered(bottom, l.host.statusContainer, width, height)
	bottom = appendRendered(bottom, l.host.editorContainer, width, height)
	bottom = appendRendered(bottom, l.host.slots["belowEditor"], width, height)
	if l.host.footer != nil {
		bottom = appendRendered(bottom, l.host.footer, width, height)
	}
	bottom = appendRendered(bottom, l.host.slots["footer"], width, height)

	lines := make([]string, 0, len(top)+len(bottom))
	if height > 0 && len(top)+len(bottom) > height && l.host.shouldReserveBottomRegion() {
		if len(bottom) >= height {
			return bottom[len(bottom)-height:]
		}
		availableTopLines := height - len(bottom)
		if len(top) > availableTopLines {
			top = top[len(top)-availableTopLines:]
		}
	}
	lines = append(lines, top...)
	lines = append(lines, bottom...)
	return lines
}

func (h *CLIInteractiveTUIHost) shouldReserveBottomRegion() bool {
	if h == nil {
		return false
	}
	if h.compactionLoader != nil {
		return true
	}
	session := h.agentSession()
	if session == nil {
		return false
	}
	return session.IsStreaming() || session.IsBashRunning() || session.IsCompacting()
}

func appendRendered(lines []string, component gitui.Component, width, height int) []string {
	if component == nil || isNilComponent(component) {
		return lines
	}
	if sized, ok := component.(gitui.SizeAwareComponent); ok {
		return append(lines, sized.RenderWithSize(width, height)...)
	}
	return append(lines, component.Render(width)...)
}

func isNilComponent(component gitui.Component) bool {
	value := reflect.ValueOf(component)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func NewCLIInteractiveTUIHost(options CLIInteractiveTUIHostOptions) (*CLIInteractiveTUIHost, error) {
	if options.RuntimeHost == nil {
		return nil, errors.New("interactive TUI runtime host is required")
	}
	terminal := options.Terminal
	if terminal == nil {
		terminal = gitui.NewProcessTerminal()
	}
	viewTreeHost := options.ViewTreeHost
	if viewTreeHost == nil {
		viewTreeHost = NewViewTreeHost()
	}
	startupWarnings := append([]string(nil), options.StartupWarnings...)
	if len(startupWarnings) == 0 {
		startupWarnings = startupWarningsFromRuntimeHost(options.RuntimeHost)
	}
	return &CLIInteractiveTUIHost{
		runtimeHost:         options.RuntimeHost,
		initialMessage:      options.InitialMessage,
		initialImages:       append([]llm.ContentPart(nil), options.InitialImages...),
		messages:            append([]string(nil), options.Messages...),
		terminal:            terminal,
		viewTreeHost:        viewTreeHost,
		inProcessUI:         options.InProcessUI,
		clipboardCopy:       options.ClipboardCopy,
		clipboardImageRead:  options.ClipboardImageRead,
		clipboardImageDir:   options.ClipboardImageDir,
		shareCreateGist:     options.ShareCreateGist,
		suspend:             resolveInteractiveSuspendOperations(options.Suspend),
		tmuxOptionReader:    options.TmuxOptionReader,
		verboseStartup:      options.VerboseStartup,
		exitAfterInitial:    options.ExitAfterInitial,
		clearScreenOnStart:  options.ClearScreenOnStart,
		showFooter:          options.ShowFooter,
		version:             firstNonEmptyString(options.Version, DefaultCodingAgentVersion),
		packageName:         firstNonEmptyString(options.PackageName, DefaultCodingAgentPackageName),
		installEnvironment:  options.InstallEnvironment,
		versionCheck:        options.VersionCheck,
		versionCheckOptions: options.VersionCheckOptions,
		packageUpdateCheck:  options.PackageUpdateCheck,
		bashOperations:      options.BashOperations,
		shutdownSignals:     options.ShutdownSignals,
		startupWarnings:     startupWarnings,
		workingVisible:      true,
		done:                make(chan struct{}),
		uiReady:             make(chan struct{}),
	}, nil
}

func startupWarningsFromRuntimeHost(runtimeHost PrintModeRuntimeHost) []string {
	provider, ok := runtimeHost.(startupWarningsProvider)
	if !ok {
		return nil
	}
	warnings := provider.StartupWarnings()
	if len(warnings) == 0 {
		return nil
	}
	return append([]string(nil), warnings...)
}

func (h *CLIInteractiveTUIHost) Run() error {
	return h.RunContext(context.Background())
}

func (h *CLIInteractiveTUIHost) RunContext(ctx context.Context) (runErr error) {
	if h == nil || h.runtimeHost == nil {
		return errors.New("interactive TUI host is required")
	}
	h.bindRuntimeSessionLifecycle()
	defer func() {
		h.waitForActivePrompts(500 * time.Millisecond)
		h.stopUI()
		h.stopProtocolExtensionProcesses()
		if err := h.runtimeHost.Dispose(); err != nil && runErr == nil {
			runErr = err
		}
	}()
	h.buildUI()
	h.bindProtocolViewTreeHost()
	stopSignalWatcher := h.startShutdownSignalWatcher(ctx)
	defer stopSignalWatcher()
	if h.clearScreenOnStart {
		h.renderDeferred.Store(true)
		h.ui.StartWithOptions(gitui.TUIStartOptions{InitialRender: false})
		h.reportTerminalError(h.terminal.ClearScreen())
	} else {
		h.ui.Start()
	}
	h.updateTerminalTitle()
	h.maybeShowTmuxKeyboardWarning(ctx)
	if err := h.startProtocolExtensionProcesses(ctx, "startup"); err != nil {
		return err
	}
	h.startViewTreeTickLoop(ctx)
	h.refreshViewTreeSlots()
	h.renderExistingMessages(true)
	h.showSessionCompactionNoticeOnStartup()
	h.showModelScopeOnStartup()
	h.showLoadedResourcesOnStartup()
	h.showModelRegistryErrorIfAny()
	h.showStartupWarningsIfNeeded()
	h.showStartupNoticesIfNeeded()
	h.maybeWarnAboutAnthropicSubscriptionAuth(llm.Model{})
	h.startVersionCheck(ctx)
	h.startPackageUpdateCheck(ctx)
	if h.clearScreenOnStart {
		h.renderDeferred.Store(false)
		h.requestRender(true)
	}
	h.markUIReady()

	if h.initialMessage != "" {
		if err := h.submitPrompt(h.initialMessage, h.initialImages); err != nil {
			h.addStatus("Error: " + err.Error())
			if h.exitAfterInitial {
				return err
			}
		}
	}
	for _, message := range h.messages {
		if strings.TrimSpace(message) == "" {
			continue
		}
		if err := h.submitPrompt(message, nil); err != nil {
			h.addStatus("Error: " + err.Error())
			if h.exitAfterInitial {
				return err
			}
		}
	}
	if h.exitAfterInitial {
		h.requestRender(true)
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.done:
		return nil
	}
}

func (h *CLIInteractiveTUIHost) markUIReady() {
	if h == nil || h.uiReady == nil {
		return
	}
	h.uiReadyOnce.Do(func() {
		close(h.uiReady)
	})
}

func (h *CLIInteractiveTUIHost) startShutdownSignalWatcher(ctx context.Context) func() {
	if h == nil {
		return func() {}
	}
	signals := h.shutdownSignals
	cleanup := func() {}
	if signals == nil {
		signals, cleanup = subscribeCLIInteractiveShutdownSignals()
	}
	if signals == nil {
		return cleanup
	}
	watcherCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-watcherCtx.Done():
			return
		case signal, ok := <-signals:
			if !ok {
				return
			}
			if isCLIInteractiveHangupSignal(signal) {
				h.stopForDeadTerminal()
				return
			}
			h.drainTerminalInputOnShutdown()
			h.Stop()
		}
	}()
	return func() {
		cancel()
		cleanup()
		<-done
	}
}

func (h *CLIInteractiveTUIHost) drainTerminalInputOnShutdown() {
	if h == nil || h.terminal == nil {
		return
	}
	_ = h.terminal.DrainInput(time.Second, 50*time.Millisecond)
}

func (h *CLIInteractiveTUIHost) handleTerminalError(err error) {
	if h == nil || !isCLIInteractiveDeadTerminalError(err) {
		return
	}
	h.stopForDeadTerminal()
}

func (h *CLIInteractiveTUIHost) reportTerminalError(err error) {
	if err != nil {
		h.handleTerminalError(err)
	}
}

func (h *CLIInteractiveTUIHost) stopForDeadTerminal() {
	if h == nil {
		return
	}
	h.deadTerminal.Store(true)
	h.Stop()
}

func isCLIInteractiveHangupSignal(signal os.Signal) bool {
	if signal == nil {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(signal.String()))
	return name == "sighup" || name == "hangup" || name == "hup"
}

func (h *CLIInteractiveTUIHost) maybeShowTmuxKeyboardWarning(ctx context.Context) {
	if h == nil || os.Getenv("TMUX") == "" {
		return
	}
	go func() {
		if warning := CheckTmuxKeyboardSetup(ctx, h.tmuxOptionReader); strings.TrimSpace(warning) != "" {
			h.addWarning(warning)
		}
	}()
}

func (h *CLIInteractiveTUIHost) updateTerminalTitle() {
	if h == nil || h.terminal == nil {
		return
	}
	cwd := h.interactiveCWD()
	cwdBase := filepath.Base(cwd)
	if strings.TrimSpace(cwdBase) == "" || cwdBase == "." {
		cwdBase = "Gi"
	}
	title := "Gi - " + cwdBase
	if session := h.agentSession(); session != nil && session.SessionManager != nil {
		if name := strings.TrimSpace(session.SessionManager.GetSessionName()); name != "" {
			title = "Gi - " + name + " - " + cwdBase
		}
	}
	h.reportTerminalError(h.terminal.SetTitle(title))
}

func (h *CLIInteractiveTUIHost) SetTUITitle(title string) error {
	if h == nil || h.terminal == nil {
		return errors.New("interactive TUI title host is not ready")
	}
	if strings.TrimSpace(title) == "" {
		h.updateTerminalTitle()
		return nil
	}
	err := h.terminal.SetTitle(title)
	h.reportTerminalError(err)
	return err
}

func (h *CLIInteractiveTUIHost) SetTUIWorking(update TUIWorkingUpdate) error {
	if h == nil || h.ui == nil {
		return errors.New("interactive TUI working host is not ready")
	}
	streaming := h.agentSessionStreaming()
	h.mu.Lock()
	if update.ResetMessage {
		h.workingMessage = ""
	}
	if update.MessageSet {
		h.workingMessage = update.Message
	}
	if update.ResetIndicator {
		h.workingIndicator = nil
	}
	if update.IndicatorSet {
		indicator := update.Indicator
		indicator.Frames = cloneOptionalStringSlice(update.Indicator.Frames)
		h.workingIndicator = &indicator
	}
	if update.VisibleSet {
		h.workingVisible = update.Visible
	}
	if h.loader != nil {
		if !h.workingVisible {
			h.clearLoaderLocked()
		} else {
			h.loader.SetMessage(h.workingMessageLocked())
			h.loader.SetIndicator(h.workingIndicatorOptionsLocked())
		}
	} else if !h.workingVisible && h.statusContainer != nil {
		h.statusContainer.Clear()
	}
	if h.workingVisible && streaming && h.loader == nil {
		h.showLoaderLocked()
	}
	h.mu.Unlock()
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) SetHiddenThinkingLabel(label string) error {
	if h == nil {
		return errors.New("interactive TUI thinking-label host is not ready")
	}
	h.hiddenThinkingLabel = label
	h.updateAssistantThinkingPresentation()
	return nil
}

func (h *CLIInteractiveTUIHost) SetTUIStatus(key, text string) error {
	if h == nil || strings.TrimSpace(key) == "" {
		return errors.New("interactive TUI status host is not ready")
	}
	if h.footerDataProvider != nil {
		if strings.TrimSpace(text) == "" {
			h.footerDataProvider.SetExtensionStatus(key, nil)
		} else {
			value := text
			h.footerDataProvider.SetExtensionStatus(key, &value)
		}
	}
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) CurrentTUITheme() string {
	if h == nil {
		return ""
	}
	h.mu.Lock()
	if h.themePreviewActive {
		name := h.themePreviewName
		h.mu.Unlock()
		return name
	}
	h.mu.Unlock()
	if settings := h.settingsManager(); settings != nil {
		if name := settings.GetTheme(); name != "" {
			return name
		}
	}
	return tuiActiveThemeSnapshot().name
}

func (h *CLIInteractiveTUIHost) AvailableTUIThemes() []TUIThemeInfo {
	if h == nil {
		return nil
	}
	seen := map[string]bool{}
	var themes []TUIThemeInfo
	add := func(theme TUIThemeInfo) {
		theme.Name = strings.TrimSpace(theme.Name)
		if theme.Name == "" || seen[theme.Name] {
			return
		}
		seen[theme.Name] = true
		themes = append(themes, theme)
	}
	add(TUIThemeInfo{Name: "dark", Builtin: true})
	add(TUIThemeInfo{Name: "light", Builtin: true})
	if session, err := h.currentAgentSession(); err == nil && session != nil && session.ResourceLoader != nil {
		if loader, ok := session.ResourceLoader.(interface{ GetThemes() ResourceThemesResult }); ok {
			for _, theme := range loader.GetThemes().Themes {
				add(TUIThemeInfo{Name: theme.Name, Path: theme.SourcePath})
			}
		}
	}
	if current := h.CurrentTUITheme(); current != "" && current != "system" {
		add(TUIThemeInfo{Name: current})
	}
	return themes
}

func (h *CLIInteractiveTUIHost) SetTUITheme(name string) error {
	if h == nil {
		return errors.New("interactive TUI theme host is not ready")
	}
	settings := h.settingsManager()
	if settings == nil {
		return errors.New("interactive TUI theme host requires settings")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("theme name is required")
	}
	h.clearTUIThemePreview()
	if err := h.applyTUIThemeName(name); err != nil {
		return err
	}
	settings.SetTheme(name)
	if h.viewTreeHost != nil && h.viewTreeHost.HasEventSubscription("theme_change") {
		_ = h.viewTreeHost.DispatchThemeChange(name)
	}
	h.requestRender(true)
	return nil
}

func (h *CLIInteractiveTUIHost) previewTUITheme(name string) {
	if h == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if err := h.applyTUIThemeName(name); err != nil {
		return
	}
	h.mu.Lock()
	h.themePreviewActive = true
	h.themePreviewName = name
	h.mu.Unlock()
	h.requestRender(true)
}

func (h *CLIInteractiveTUIHost) clearTUIThemePreview() {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.themePreviewActive = false
	h.themePreviewName = ""
	h.mu.Unlock()
	_ = h.applyCurrentTUITheme()
}

func (h *CLIInteractiveTUIHost) applyCurrentTUITheme() error {
	name := ""
	if settings := h.settingsManager(); settings != nil {
		name = settings.GetTheme()
	}
	return h.applyTUIThemeName(name)
}

func (h *CLIInteractiveTUIHost) applyTUIThemeName(name string) error {
	if h == nil {
		return errors.New("interactive TUI theme host is not ready")
	}
	return tuiSetActiveTheme(name, h.AvailableTUIThemes())
}

func (h *CLIInteractiveTUIHost) showStartupNoticesIfNeeded() {
	if h == nil || h.chat == nil || h.startupNoticesShown {
		return
	}
	h.startupNoticesShown = true
	changelog := strings.TrimSpace(h.startupChangelogForDisplay())
	if changelog == "" {
		return
	}
	if h.chat.ChildCount() > 0 {
		h.chat.AddChild(gitui.NewSpacer(1))
	}
	settings := h.settingsManager()
	if settings != nil && settings.GetCollapseChangelog() {
		version := displayPackageVersion(firstNonEmptyString(firstChangelogVersion(changelog), h.version))
		h.chat.AddChild(newCLIDynamicBorder())
		h.chat.AddChild(gitui.NewText("Updated to v"+version+". Use "+tuiThemeBold("/changelog")+" to view full changelog.", 1, 0))
		h.chat.AddChild(newCLIDynamicBorder())
		h.requestRender(false)
		return
	}
	h.chat.AddChild(newCLIDynamicBorder())
	h.chat.AddChild(gitui.NewText(tuiThemeBoldAccent("What's New"), 1, 0))
	h.chat.AddChild(gitui.NewSpacer(1))
	h.chat.AddChild(newCLIMarkdownWithOptions(changelog, gitui.MarkdownOptions{PaddingX: 1, PaddingY: 0}))
	h.chat.AddChild(newCLIDynamicBorder())
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) showStartupWarningsIfNeeded() {
	if h == nil || h.chat == nil || len(h.startupWarnings) == 0 {
		return
	}
	for _, warning := range h.startupWarnings {
		h.addWarning(warning)
	}
}

func (h *CLIInteractiveTUIHost) startupChangelogForDisplay() string {
	session := h.agentSession()
	if session != nil && len(session.Messages()) > 0 {
		return ""
	}
	settings := h.settingsManager()
	if settings == nil {
		return ""
	}
	lastVersion := strings.TrimSpace(settings.GetLastChangelogVersion())
	currentVersion := firstNonEmptyString(strings.TrimSpace(h.version), DefaultCodingAgentVersion)
	if lastVersion == "" {
		settings.SetLastChangelogVersion(currentVersion)
		return ""
	}
	changelog := h.loadChangelogMarkdown()
	entries := newChangelogEntries(parseChangelogEntries(changelog), lastVersion)
	if len(entries) == 0 {
		return ""
	}
	settings.SetLastChangelogVersion(currentVersion)
	return changelogEntriesMarkdown(entries)
}

func (h *CLIInteractiveTUIHost) startVersionCheck(ctx context.Context) {
	if h == nil || h.versionCheck == nil {
		return
	}
	go func() {
		release, ok := h.versionCheck(h.version, h.versionCheckOptions)
		if !ok || !IsNewerPackageVersion(release.Version, h.version) {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
			h.showNewVersionNotification(release)
		}
	}()
}

func (h *CLIInteractiveTUIHost) showNewVersionNotification(release LatestGiRelease) {
	if h == nil || h.chat == nil {
		return
	}
	env := h.installEnvironment
	if isZeroInstallEnvironment(env) {
		env = DefaultInstallEnvironment()
	}
	packageName := firstNonEmptyString(h.packageName, DefaultCodingAgentPackageName)
	updatePackageName := firstNonEmptyString(release.PackageName, packageName)
	instruction := GetSelfUpdateUnavailableInstruction(packageName, env, nil, updatePackageName)
	changelogURL := "https://github.com/nowa/gi/releases/latest"
	if h.chat.ChildCount() > 0 {
		h.chat.AddChild(gitui.NewSpacer(1))
	}
	text := fmt.Sprintf("**Update Available**\n\nNew version %s is available.\n%s\n\nChangelog: %s", release.Version, instruction, changelogURL)
	h.chat.AddChild(newCLIMarkdownWithOptions(text, gitui.MarkdownOptions{PaddingX: 1, PaddingY: 0}))
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) startPackageUpdateCheck(ctx context.Context) {
	if h == nil || h.packageUpdateCheck == nil {
		return
	}
	go func() {
		packages, err := h.packageUpdateCheck()
		if err != nil || len(packages) == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
			h.showPackageUpdateNotification(packages)
		}
	}()
}

func (h *CLIInteractiveTUIHost) showPackageUpdateNotification(packages []string) {
	if h == nil || h.chat == nil || len(packages) == 0 {
		return
	}
	if h.chat.ChildCount() > 0 {
		h.chat.AddChild(gitui.NewSpacer(1))
	}
	lines := []string{"**Package Updates Available**", "", "Package updates are available. Run `gi update`.", "", "Packages:"}
	for _, pkg := range packages {
		if strings.TrimSpace(pkg) != "" {
			lines = append(lines, "- "+strings.TrimSpace(pkg))
		}
	}
	h.chat.AddChild(newCLIMarkdownWithOptions(strings.Join(lines, "\n"), gitui.MarkdownOptions{PaddingX: 1, PaddingY: 0}))
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) maybeWarnAboutAnthropicSubscriptionAuth(model llm.Model) {
	if h == nil {
		return
	}
	if strings.TrimSpace(model.Provider) == "" {
		session := h.agentSession()
		if session == nil || session.Agent == nil {
			return
		}
		model = session.Agent.State.Model
	}
	settings := h.settingsManager()
	registry := h.modelRegistry()
	if settings == nil || registry == nil {
		return
	}
	enabled := settings.GetWarnings().AnthropicExtraUsage
	checker := AnthropicSubscriptionWarningChecker{
		Shown:                h.anthropicWarningShown,
		Settings:             AnthropicWarningSettings{AnthropicExtraUsage: &enabled},
		AuthStorage:          registry.authStorage,
		GetAPIKeyForProvider: registry.GetAPIKeyForProvider,
	}
	if checker.MaybeWarn(model.Provider) {
		h.anthropicWarningShown = checker.Shown
		h.addStatus("Warning: " + AnthropicSubscriptionAuthWarning)
		return
	}
	h.anthropicWarningShown = checker.Shown
}

func (h *CLIInteractiveTUIHost) startProtocolExtensionProcesses(ctx context.Context, reason string) error {
	reason = firstNonEmptyString(reason, "startup")
	provider, ok := h.runtimeHost.(protocolExtensionProcessProvider)
	if !ok {
		return nil
	}
	specs := provider.ProtocolExtensionProcessSpecs()
	if len(specs) == 0 {
		return nil
	}
	rpcHost := provider.NewProtocolExtensionRPCSessionHost(h.viewTreeHost, h, h)
	h.configureProtocolExtensionRPCSessionHost(rpcHost)
	supervisor := NewProtocolExtensionProcessSupervisor(rpcHost, specs)
	if err := supervisor.Start(ctx); err != nil {
		return err
	}
	h.processSupervisor = supervisor
	if err := supervisor.EmitEvent(ProtocolEventSessionStart, map[string]any{"reason": reason}); err != nil {
		h.stopProtocolExtensionProcesses()
		return err
	}
	if err := h.extendProtocolProcessResources(ctx, supervisor, reason); err != nil {
		h.stopProtocolExtensionProcesses()
		return err
	}
	return nil
}

func (h *CLIInteractiveTUIHost) extendProtocolProcessResources(ctx context.Context, supervisor *ProtocolExtensionProcessSupervisor, reason string) error {
	if h == nil || supervisor == nil {
		return nil
	}
	session, err := h.currentAgentSession()
	if err != nil || session == nil {
		return err
	}
	cwd := ""
	if session.SessionManager != nil {
		cwd = session.SessionManager.GetCWD()
	}
	resources, err := supervisor.DiscoverResources(ctx, reason, cwd)
	if err != nil {
		h.addStatus("Error: Extension resources discovery failed: " + err.Error())
	}
	if len(resources.SkillPaths) == 0 && len(resources.PromptPaths) == 0 && len(resources.ThemePaths) == 0 {
		return nil
	}
	extender, ok := session.ResourceLoader.(interface {
		ExtendResources(ResourceExtension)
	})
	if !ok {
		return nil
	}
	extender.ExtendResources(resources)
	session.RefreshSystemPrompt()
	h.refreshEditorAutocompleteProvider()
	return nil
}

func (h *CLIInteractiveTUIHost) bindRuntimeSessionLifecycle() {
	if h == nil || h.unwatchRuntimeSession != nil || h.restoreRuntimeRebind != nil {
		return
	}
	runtimeHost := h.agentSessionRuntimeHost()
	if runtimeHost == nil {
		return
	}
	previousRebind := runtimeHost.RebindSession
	wrapper := func(session *AgentSession) error {
		if previousRebind != nil {
			if err := previousRebind(session); err != nil {
				return err
			}
		}
		h.rebindInteractiveSession(session)
		return nil
	}
	wrapperPtr := reflect.ValueOf(wrapper).Pointer()
	runtimeHost.SetRebindSession(wrapper)
	h.restoreRuntimeRebind = func() {
		if runtimeHost.RebindSession != nil && reflect.ValueOf(runtimeHost.RebindSession).Pointer() == wrapperPtr {
			runtimeHost.SetRebindSession(previousRebind)
		}
		h.restoreRuntimeRebind = nil
	}
	h.unwatchRuntimeSession = runtimeHost.OnSessionEvent(func(event ProtocolSessionEvent) error {
		return h.emitRuntimeSessionEventToProcesses(event)
	})
}

func (h *CLIInteractiveTUIHost) rebindInteractiveSession(session *AgentSession) {
	if h == nil || session == nil {
		return
	}
	if h.processSupervisor != nil {
		h.processSupervisor.BindSession(session)
	}
	if h.removeSessionWatcher(false) {
		h.watchAgentSessionQueue()
	}
	h.refreshPendingMessagesDisplay()
	h.refreshFooterState()
	h.updateTerminalTitle()
}

func (h *CLIInteractiveTUIHost) emitRuntimeSessionEventToProcesses(event ProtocolSessionEvent) error {
	if h == nil || h.processSupervisor == nil {
		return nil
	}
	switch event.Type {
	case ProtocolEventSessionSwitch:
		if err := h.processSupervisor.EmitSessionEvent(event); err != nil {
			h.addStatus("Error: Extension lifecycle event failed: " + err.Error())
		}
		return nil
	case ProtocolEventSessionStart:
		if event.Reason == "startup" {
			return nil
		}
		if err := h.processSupervisor.EmitSessionEvent(event); err != nil {
			h.addStatus("Error: Extension lifecycle event failed: " + err.Error())
		}
		return h.extendProtocolProcessResources(context.Background(), h.processSupervisor, event.Reason)
	default:
		return nil
	}
}

func (h *CLIInteractiveTUIHost) stopProtocolExtensionProcesses() {
	if h == nil || h.processSupervisor == nil {
		return
	}
	_ = h.processSupervisor.Stop(context.Background())
	h.processSupervisor = nil
}

func (h *CLIInteractiveTUIHost) startViewTreeTickLoop(ctx context.Context) {
	if h == nil || h.viewTreeHost == nil || !h.viewTreeTickStarted.CompareAndSwap(false, true) {
		return
	}
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		var frame int64
		lastWidth, lastHeight := 0, 0
		for {
			select {
			case <-ticker.C:
				if h.viewTreeHost.HasEventSubscription("tick") {
					frame++
					_ = h.viewTreeHost.DispatchTick(frame)
				}
				width, height := h.terminal.Columns(), h.terminal.Rows()
				if h.viewTreeHost.HasEventSubscription("resize") && (width != lastWidth || height != lastHeight) {
					lastWidth, lastHeight = width, height
					_ = h.viewTreeHost.DispatchResize(width, height)
				}
			case <-ctx.Done():
				return
			case <-h.done:
				return
			}
		}
	}()
}

func (h *CLIInteractiveTUIHost) buildUI() {
	settings := h.settingsManager()
	if err := h.applyCurrentTUITheme(); err != nil {
		_ = tuiSetActiveTheme("dark", nil)
	}
	showHardwareCursor := false
	editorPaddingX := 0
	autocompleteMaxVisible := 5
	clearOnShrink := false
	if settings != nil {
		showHardwareCursor = settings.GetShowHardwareCursor()
		editorPaddingX = settings.GetEditorPaddingX()
		autocompleteMaxVisible = settings.GetAutocompleteMaxVisible()
		clearOnShrink = settings.GetClearOnShrink()
	}
	h.ui = gitui.NewTUI(h.terminal, showHardwareCursor)
	h.ui.SetClearOnShrink(clearOnShrink)
	h.ui.SetTerminalErrorHandler(func(err error) {
		h.handleTerminalError(err)
	})
	h.chat = gitui.NewContainer()
	h.pendingMessages = gitui.NewContainer()
	h.statusContainer = gitui.NewContainer()
	h.editorContainer = gitui.NewContainer()
	h.slots = map[string]*gitui.Container{}
	h.views = map[string]*ViewTreeComponent{}
	h.overlays = map[string]gitui.OverlayHandle{}
	h.inProcessSlots = map[string][]gitui.Component{}
	h.inProcessMounts = map[string]inProcessMountedComponent{}
	h.pendingTools = map[string]*ToolExecutionComponent{}
	h.compactionQueue = nil
	h.editor = gitui.NewEditor(tuiThemeEditor(), gitui.EditorOptions{
		PaddingX:               editorPaddingX,
		AutocompleteMaxVisible: autocompleteMaxVisible,
	})
	h.updateEditorBorderColor()
	h.reloadKeybindings()
	h.startupHeader = newCLIStartupHeaderComponent(firstNonEmptyString(h.version, DefaultCodingAgentVersion), h.toolOutputExpanded, h.effectiveKeybindings())
	h.refreshEditorAutocompleteProvider()
	h.editor.OnAutocompleteChange = func() {
		h.requestRender(false)
	}
	h.watchEditorAutocompleteProviders()
	h.editor.SetOnSubmit(func(text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		h.addEditorHistory(text)
		go func() {
			if err := h.submitPrompt(text, nil); err != nil {
				h.addStatus("Error: " + err.Error())
			}
		}()
	})

	h.ui.AddInputListener(func(data string) gitui.InputListenerResult {
		h.emitPackageTerminalInput(data)
		if h.ui != nil && h.ui.HasOverlay() {
			return gitui.InputListenerResult{}
		}
		keybindings := h.effectiveKeybindings()
		if matchesKeybindingAction(data, keybindings, "app.clear") && h.focusedDefaultEditor() {
			h.handleClearKey()
			return gitui.InputListenerResult{Consume: true}
		}
		if matchesKeybindingAction(data, keybindings, "app.exit") && h.editorIsEmpty() {
			h.Stop()
			return gitui.InputListenerResult{Consume: true}
		}
		if matchesKeybindingAction(data, keybindings, "app.interrupt") && h.focusedDefaultEditor() && h.handleEscapeKey() {
			return gitui.InputListenerResult{Consume: true}
		}
		if h.handleProtocolShortcutKey(data, keybindings) {
			return gitui.InputListenerResult{Consume: true}
		}
		if h.handleAppActionKey(data, keybindings) {
			return gitui.InputListenerResult{Consume: true}
		}
		if h.handleMessageQueueKey(data, keybindings) {
			return gitui.InputListenerResult{Consume: true}
		}
		return gitui.InputListenerResult{}
	})
	h.ui.SetOnDebug(func() {
		if err := h.handleDebugCommand(); err != nil {
			h.addStatus("Error: " + err.Error())
		}
	})
	h.watchAgentSessionQueue()
	h.unwatch = h.viewTreeHost.OnChange(func(ViewTreeChange) {
		h.refreshViewTreeSlots()
		h.requestRender(false)
	})

	h.addSlot("header")
	h.addSlot("aboveEditor")
	h.editorContainer.AddChild(h.editor)
	h.addSlot("belowEditor")
	h.addSlot("footer")
	h.mountInProcessUI()
	h.buildFooter()
	h.layout = &cliInteractiveLayout{host: h}
	h.ui.AddChild(h.layout)
	h.ui.SetFocus(h.editor)
}

func (h *CLIInteractiveTUIHost) emitPackageTerminalInput(data string) {
	if h == nil || h.processSupervisor == nil || data == "" {
		return
	}
	supervisor := h.processSupervisor
	go func() {
		_ = supervisor.EmitTerminalInput(data)
	}()
}

func (h *CLIInteractiveTUIHost) focusedDefaultEditor() bool {
	if h == nil || h.ui == nil {
		return false
	}
	focused := h.ui.FocusedComponent()
	if focused == nil {
		return false
	}
	if focused == h.editor {
		return true
	}
	return h.customEditorActive && h.editorContainerHasChild(focused)
}

func (h *CLIInteractiveTUIHost) activeEditorShowingAutocomplete() bool {
	if h == nil || h.editor == nil {
		return false
	}
	return h.ui != nil && h.ui.FocusedComponent() == h.editor && h.editor.IsShowingAutocomplete()
}

func (h *CLIInteractiveTUIHost) activeEditorComponent() (gitui.EditorComponent, bool) {
	if h == nil {
		return nil, false
	}
	if h.customEditorActive && h.editorContainer != nil {
		children := h.editorContainer.Children()
		if len(children) > 0 {
			return editorComponentFrom(children[0])
		}
		return nil, false
	}
	if h.editor == nil {
		return nil, false
	}
	return h.editor, true
}

func editorComponentFrom(component gitui.Component) (gitui.EditorComponent, bool) {
	if component == nil {
		return nil, false
	}
	if wrapped, ok := component.(*safeInProcessComponent); ok {
		editor, ok := wrapped.component.(gitui.EditorComponent)
		return editor, ok
	}
	editor, ok := component.(gitui.EditorComponent)
	return editor, ok
}

func (h *CLIInteractiveTUIHost) activeEditorText() string {
	editor, ok := h.activeEditorComponent()
	if !ok {
		return ""
	}
	if expanded, ok := editor.(gitui.EditorExpandedTextProvider); ok {
		return expanded.GetExpandedText()
	}
	return editor.GetText()
}

func (h *CLIInteractiveTUIHost) setActiveEditorText(text string) bool {
	editor, ok := h.activeEditorComponent()
	if !ok {
		return false
	}
	editor.SetText(text)
	h.requestRender(false)
	return true
}

func (h *CLIInteractiveTUIHost) handleClipboardImagePaste() {
	if h == nil {
		return
	}
	readImage := h.clipboardImageRead
	if readImage == nil {
		readImage = func() *ClipboardImage {
			return ReadClipboardImage(ClipboardImageOptions{})
		}
	}
	image := readImage()
	if image == nil || len(image.Bytes) == 0 {
		return
	}
	ext := ExtensionForImageMIMEType(image.MIMEType)
	if ext == "" {
		ext = "png"
	}
	file, err := os.CreateTemp(h.clipboardImageDir, "gi-clipboard-*."+ext)
	if err != nil {
		return
	}
	path := file.Name()
	if _, err := file.Write(image.Bytes); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return
	}
	h.InsertEditorText(path)
}

func (h *CLIInteractiveTUIHost) cycleThinkingLevelFromKey() {
	host, err := h.newRPCSessionHost()
	if err != nil {
		h.addStatus("Error: " + err.Error())
		return
	}
	level, err := host.CycleThinkingLevel()
	if err != nil {
		h.addStatus("Error: " + err.Error())
		return
	}
	h.updateEditorBorderColor()
	h.addStatus("Thinking level: " + level)
}

func (h *CLIInteractiveTUIHost) cycleModelFromKey(direction string) {
	host, err := h.newRPCSessionHost()
	if err != nil {
		h.addStatus("Error: " + err.Error())
		return
	}
	result, err := host.CycleModelDirection(direction)
	if err != nil {
		h.addStatus("Error: " + err.Error())
		return
	}
	if result == nil {
		if len(host.ScopedModels) > 0 {
			h.addStatus("Only one model in scope")
		} else {
			h.addStatus("Only one model available")
		}
		return
	}
	h.updateEditorBorderColor()
	h.addStatus(formatModelCycleStatus(result.Model, result.ThinkingLevel))
	h.maybeWarnAboutAnthropicSubscriptionAuth(result.Model)
	h.checkDaxnutsEasterEgg(result.Model)
}

func (h *CLIInteractiveTUIHost) toggleToolOutputExpansion() {
	if h == nil || h.chat == nil {
		return
	}
	h.toolOutputExpanded = !h.toolOutputExpanded
	h.applyToolOutputExpansion()
}

func (h *CLIInteractiveTUIHost) applyToolOutputExpansion() {
	if h == nil || h.chat == nil {
		return
	}
	if h.startupHeader != nil {
		h.startupHeader.SetExpanded(h.toolOutputExpanded)
	}
	for _, component := range h.startupResources {
		if component != nil {
			component.SetExpanded(h.toolOutputExpanded)
		}
	}
	for _, child := range h.chat.Children() {
		if expandable, ok := child.(interface{ SetExpanded(bool) }); ok {
			expandable.SetExpanded(h.toolOutputExpanded)
		}
	}
	for _, tool := range h.pendingTools {
		if tool != nil {
			tool.SetExpanded(h.toolOutputExpanded)
		}
	}
	for _, mounted := range h.inProcessMounts {
		if expandable, ok := mounted.component.(interface{ SetExpanded(bool) }); ok {
			expandable.SetExpanded(h.toolOutputExpanded)
		}
	}
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) TUIToolsExpanded() bool {
	return h != nil && h.toolOutputExpanded
}

func (h *CLIInteractiveTUIHost) SetTUIToolsExpanded(expanded bool) error {
	if h == nil {
		return errors.New("interactive TUI tool expansion host is not ready")
	}
	h.toolOutputExpanded = expanded
	h.applyToolOutputExpansion()
	return nil
}

func (h *CLIInteractiveTUIHost) toggleThinkingBlockVisibility() {
	settings := h.settingsManager()
	if settings == nil {
		h.addStatus("Thinking block settings are unavailable")
		return
	}
	hidden := !settings.GetHideThinkingBlock()
	settings.SetHideThinkingBlock(hidden)
	h.updateAssistantThinkingPresentation()
	if hidden {
		h.addStatus("Thinking blocks: hidden")
	} else {
		h.addStatus("Thinking blocks: visible")
	}
}

func (h *CLIInteractiveTUIHost) openExternalEditor() {
	if h == nil {
		return
	}
	command := externalEditorCommand()
	if command == "" {
		h.addStatus("No editor configured. Set $VISUAL or $EDITOR.")
		return
	}
	name, args, ok := splitExternalEditorCommand(command)
	if !ok {
		h.addStatus("Invalid editor command")
		return
	}
	tmp, err := os.CreateTemp("", "gi-editor-*.md")
	if err != nil {
		h.addStatus("Error: " + err.Error())
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(h.activeEditorText()); err != nil {
		tmp.Close()
		h.addStatus("Error: " + err.Error())
		return
	}
	if err := tmp.Close(); err != nil {
		h.addStatus("Error: " + err.Error())
		return
	}

	if h.ui != nil {
		h.ui.Stop()
	}
	cmd := exec.Command(name, append(args, tmpName)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if h.ui != nil {
		h.ui.Start()
		h.ui.RequestRender(true)
	}
	if err != nil {
		h.addStatus("External editor failed: " + err.Error())
		return
	}
	content, err := os.ReadFile(tmpName)
	if err != nil {
		h.addStatus("Error: " + err.Error())
		return
	}
	h.setActiveEditorText(strings.TrimSuffix(string(content), "\n"))
	h.requestRender(true)
}

func (h *CLIInteractiveTUIHost) watchAgentSessionQueue() {
	session := h.agentSession()
	if session == nil {
		return
	}
	unwatch := session.Subscribe(func(event AgentSessionEvent) {
		switch event.Type {
		case "agent_start":
			h.pendingTools = map[string]*ToolExecutionComponent{}
			h.streamingMessage = nil
			h.streamingComponent = nil
			h.clearRetryStatus()
			h.mu.Lock()
			h.clearLoaderLocked()
			if h.workingVisible {
				h.showLoaderLocked()
			}
			h.mu.Unlock()
			h.setTerminalProgress(true)
			h.requestRender(false)
		case "message_start":
			h.handleLiveMessageStart(event)
		case "message_update":
			h.handleLiveMessageUpdate(event)
		case "message_end":
			h.handleLiveMessageEnd(event)
		case "tool_execution_start":
			h.handleLiveToolExecutionStart(event)
		case ProtocolEventToolExecutionUpdate:
			h.handleLiveToolExecutionUpdate(event)
		case "tool_execution_end":
			h.handleLiveToolExecutionEnd(event)
		case "queue_update":
			h.refreshPendingMessagesDisplay()
			h.requestRender(false)
		case "agent_end":
			h.handleAgentEnd()
		case "turn_end":
			h.setTerminalProgress(false)
			h.clearCompactionLoader()
			h.clearRetryStatus()
			h.refreshPendingMessagesDisplay()
			h.requestRender(false)
		case ProtocolEventSessionInfoChanged:
			h.updateTerminalTitle()
			h.refreshFooterState()
			h.requestRender(false)
		case "thinking_level_changed":
			h.updateEditorBorderColor()
			h.refreshFooterState()
			h.requestRender(false)
		case "compaction_start":
			h.handleCompactionStart(event)
		case "compaction_end":
			h.handleCompactionEnd(event)
		case "auto_retry_start":
			h.handleAutoRetryStart(event)
		case "auto_retry_end":
			h.handleAutoRetryEnd(event)
		}
	})
	h.installSessionWatcher(unwatch)
	h.refreshPendingMessagesDisplay()
}

func (h *CLIInteractiveTUIHost) handleAgentEnd() {
	if h == nil {
		return
	}
	h.setTerminalProgress(false)
	h.clearRetryStatus()
	h.mu.Lock()
	h.clearLoaderLocked()
	if h.editor != nil {
		h.editor.DisableSubmit = false
	}
	h.mu.Unlock()
	if h.streamingComponent != nil && h.chat != nil {
		h.chat.RemoveChild(h.streamingComponent)
	}
	h.streamingComponent = nil
	h.streamingMessage = nil
	h.pendingTools = map[string]*ToolExecutionComponent{}
	h.refreshPendingMessagesDisplay()
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) handleCompactionStart(event AgentSessionEvent) {
	if h == nil {
		return
	}
	h.setTerminalProgress(true)
	h.refreshPendingMessagesDisplay()
	cancelHint := "(Esc to cancel)"
	label := "Compacting context... " + cancelHint
	if event.Reason != "" && event.Reason != "manual" {
		prefix := ""
		if event.Reason == "overflow" {
			prefix = "Context overflow detected, "
		}
		label = prefix + "Auto-compacting... " + cancelHint
	}
	h.showCompactionLoader(label)
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) handleCompactionEnd(event AgentSessionEvent) {
	if h == nil {
		return
	}
	h.setTerminalProgress(false)
	h.clearCompactionLoader()
	h.refreshPendingMessagesDisplay()
	switch {
	case event.Aborted:
		if event.Reason == "manual" {
			h.addStatus("Compaction cancelled")
		} else {
			h.addStatus("Auto-compaction cancelled")
		}
	case event.Result != nil:
		h.resetChatState()
		h.renderExistingMessages(false)
	case strings.TrimSpace(event.ErrorMessage) != "":
		h.addStatus(strings.TrimSpace(event.ErrorMessage))
	}
	h.flushCompactionQueue(event.WillRetry)
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) handleLiveMessageStart(event AgentSessionEvent) {
	if h == nil || event.Message == nil {
		return
	}
	message := *event.Message
	switch message.Role {
	case llm.RoleAssistant:
		component := newCLIAssistantMessageComponent(message, h.hideThinkingBlock(), h.hiddenThinkingLabelValue())
		h.streamingMessage = &message
		h.streamingComponent = component
		if h.chat != nil {
			h.chat.AddChild(component)
		}
	case llm.RoleToolResult:
		// Tool results are shown by the tool_execution_end event and should stay
		// inline with their originating tool call.
	default:
		h.addMessage(message)
	}
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) handleLiveMessageUpdate(event AgentSessionEvent) {
	if h == nil || event.AssistantMessageEvent == nil {
		return
	}
	update := event.AssistantMessageEvent
	message := update.Partial
	switch update.Type {
	case "done":
		message = update.Message
	case "error":
		message = update.Error
	}
	if message.Role == "" {
		message.Role = llm.RoleAssistant
	}
	h.streamingMessage = &message
	h.updateLiveAssistantComponent(message)
	h.updateLiveToolCalls(message)
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) handleLiveMessageEnd(event AgentSessionEvent) {
	if h == nil || event.Message == nil {
		return
	}
	message := *event.Message
	if message.Role == llm.RoleUser {
		h.syncRenderedMessageCount()
		return
	}
	if message.Role == llm.RoleAssistant {
		h.updateLiveAssistantComponent(message)
		h.completeLiveAssistantToolCalls(message)
		h.streamingMessage = nil
		h.streamingComponent = nil
	} else if message.Role == llm.RoleToolResult {
		h.addToolResultMessage(message)
	}
	h.syncRenderedMessageCount()
	h.refreshFooterState()
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) handleLiveToolExecutionStart(event AgentSessionEvent) {
	if h == nil || strings.TrimSpace(event.ToolCallID) == "" {
		return
	}
	component := h.pendingTools[event.ToolCallID]
	if component == nil {
		component = h.newToolExecutionComponent(event.ToolName, event.ToolCallID, event.Args)
		if h.pendingTools == nil {
			h.pendingTools = map[string]*ToolExecutionComponent{}
		}
		h.pendingTools[event.ToolCallID] = component
		if h.chat != nil {
			h.chat.AddChild(component)
		}
	} else {
		component.UpdateArgs(event.Args)
	}
	component.MarkExecutionStarted()
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) handleLiveToolExecutionUpdate(event AgentSessionEvent) {
	if h == nil || event.PartialToolResult == nil || strings.TrimSpace(event.ToolCallID) == "" {
		return
	}
	component := h.pendingTools[event.ToolCallID]
	if component == nil {
		component = h.newToolExecutionComponent(event.ToolName, event.ToolCallID, event.Args)
		if h.pendingTools == nil {
			h.pendingTools = map[string]*ToolExecutionComponent{}
		}
		h.pendingTools[event.ToolCallID] = component
		if h.chat != nil {
			h.chat.AddChild(component)
		}
	}
	component.MarkExecutionStarted()
	component.UpdatePartialResult(fileToolResultFromLLMMessage(*event.PartialToolResult), event.PartialToolResult.IsError)
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) handleLiveToolExecutionEnd(event AgentSessionEvent) {
	if h == nil || event.ToolResult == nil || strings.TrimSpace(event.ToolCallID) == "" {
		return
	}
	component := h.pendingTools[event.ToolCallID]
	if component == nil {
		component = h.newToolExecutionComponent(event.ToolName, event.ToolCallID, event.Args)
		if h.pendingTools == nil {
			h.pendingTools = map[string]*ToolExecutionComponent{}
		}
		h.pendingTools[event.ToolCallID] = component
		if h.chat != nil {
			h.chat.AddChild(component)
		}
	}
	component.MarkExecutionStarted()
	component.UpdateResult(fileToolResultFromLLMMessage(*event.ToolResult), event.ToolResult.IsError)
	delete(h.pendingTools, event.ToolCallID)
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) updateLiveAssistantComponent(message llm.Message) {
	if h == nil || h.streamingComponent == nil {
		return
	}
	if strings.TrimSpace(interactiveAssistantTextFromLLMMessage(message, h.hideThinkingBlock(), h.hiddenThinkingLabelValue())) == "" &&
		strings.TrimSpace(message.ErrorMessage) != "" &&
		message.StopReason == "" {
		message.StopReason = llm.StopReasonError
	}
	h.streamingComponent.SetMessage(message)
}

func (h *CLIInteractiveTUIHost) updateLiveToolCalls(message llm.Message) {
	if h == nil || h.chat == nil {
		return
	}
	for _, part := range message.Content {
		if part.Type != llm.ContentToolCall || strings.TrimSpace(part.ID) == "" {
			continue
		}
		component := h.pendingTools[part.ID]
		if component == nil {
			component = h.newToolExecutionComponent(part.Name, part.ID, part.Arguments)
			if h.pendingTools == nil {
				h.pendingTools = map[string]*ToolExecutionComponent{}
			}
			h.pendingTools[part.ID] = component
			h.chat.AddChild(component)
			continue
		}
		component.UpdateArgs(part.Arguments)
	}
}

func (h *CLIInteractiveTUIHost) completeLiveAssistantToolCalls(message llm.Message) {
	if h == nil {
		return
	}
	for _, part := range message.Content {
		if part.Type != llm.ContentToolCall || strings.TrimSpace(part.ID) == "" {
			continue
		}
		component := h.pendingTools[part.ID]
		if component == nil {
			component = h.newToolExecutionComponent(part.Name, part.ID, part.Arguments)
			if h.pendingTools == nil {
				h.pendingTools = map[string]*ToolExecutionComponent{}
			}
			h.pendingTools[part.ID] = component
			if h.chat != nil {
				h.chat.AddChild(component)
			}
		}
		component.UpdateArgs(part.Arguments)
		component.SetArgsComplete()
		if message.StopReason == "aborted" || message.StopReason == "error" {
			errorMessage := assistantToolErrorMessage(message)
			component.UpdateResult(FileToolResult{Text: errorMessage, Content: []llm.ContentPart{llm.Text(errorMessage)}}, true)
			delete(h.pendingTools, part.ID)
		}
	}
}

func (h *CLIInteractiveTUIHost) syncRenderedMessageCount() {
	if h == nil || h.runtimeHost == nil {
		return
	}
	session := h.runtimeHost.PrintModeSession()
	if session == nil {
		return
	}
	h.rendered = len(session.Messages())
}

func (h *CLIInteractiveTUIHost) liveSessionEventRendering() bool {
	if h == nil {
		return false
	}
	h.sessionWatchMu.Lock()
	watching := h.unwatchSession != nil
	h.sessionWatchMu.Unlock()
	return watching && h.agentSession() != nil
}

func (h *CLIInteractiveTUIHost) installSessionWatcher(unwatch func()) {
	if h == nil || unwatch == nil {
		return
	}
	h.sessionWatchMu.Lock()
	if h.sessionWatchClosed {
		h.sessionWatchMu.Unlock()
		unwatch()
		return
	}
	previous := h.unwatchSession
	h.unwatchSession = unwatch
	h.sessionWatchMu.Unlock()
	if previous != nil {
		previous()
	}
}

func (h *CLIInteractiveTUIHost) removeSessionWatcher(closeWatcher bool) bool {
	if h == nil {
		return false
	}
	h.sessionWatchMu.Lock()
	unwatch := h.unwatchSession
	h.unwatchSession = nil
	if closeWatcher {
		h.sessionWatchClosed = true
	}
	h.sessionWatchMu.Unlock()
	if unwatch != nil {
		unwatch()
		return true
	}
	return false
}

func (h *CLIInteractiveTUIHost) handleAutoRetryStart(event AgentSessionEvent) {
	seconds := (event.DelayMs + 999) / 1000
	if seconds < 0 {
		seconds = 0
	}
	h.clearRetryStatus()
	message := fmt.Sprintf("Retrying (%d/%d) in %ds... (Esc to cancel)", event.Attempt, event.MaxAttempts, seconds)
	h.retryLoader = h.showStatusLoader(message)
	if h.retryLoader == nil {
		h.retryStatus = h.addStatus(message)
	}
}

func (h *CLIInteractiveTUIHost) handleAutoRetryEnd(event AgentSessionEvent) {
	h.clearRetryStatus()
	if event.Success {
		return
	}
	if strings.TrimSpace(event.FinalError) != "" {
		h.addStatus(fmt.Sprintf("Retry failed after %d attempt%s: %s", event.Attempt, pluralSuffix(event.Attempt), event.FinalError))
		return
	}
	h.addStatus("Retry cancelled")
}

func (h *CLIInteractiveTUIHost) clearRetryStatus() {
	if h == nil {
		return
	}
	if h.retryLoader != nil {
		h.retryLoader.Stop()
		if h.statusContainer != nil {
			h.statusContainer.Clear()
		}
		h.retryLoader = nil
		h.requestRender(false)
	}
	if h.retryStatus != nil && h.chat != nil {
		h.chat.RemoveChild(h.retryStatus)
		h.retryStatus = nil
		h.requestRender(false)
	}
}

func (h *CLIInteractiveTUIHost) buildFooter() {
	if h == nil || !h.showFooter {
		return
	}
	cwd := h.interactiveCWD()
	h.footerDataProvider = NewFooterDataProvider(cwd)
	h.footer = NewFooterComponent(FooterState{CWD: cwd})
	h.unwatchFooterBranch = h.footerDataProvider.OnBranchChange(func() {
		h.refreshFooterState()
		h.requestRender(false)
	})
	h.refreshFooterState()
}

func (h *CLIInteractiveTUIHost) addSlot(slot string) {
	if h == nil {
		return
	}
	container := gitui.NewContainer()
	h.slots[slot] = container
	h.refreshViewTreeSlot(slot)
}

func (h *CLIInteractiveTUIHost) mountInProcessUI() {
	if h == nil || h.inProcessUI == nil {
		return
	}
	h.syncInProcessUI()
	h.unwatchInProcess = h.inProcessUI.OnChange(func() {
		h.syncInProcessUI()
		h.requestRender(false)
	})
}

func (h *CLIInteractiveTUIHost) syncInProcessUI() {
	if h == nil || h.inProcessUI == nil {
		return
	}
	if h.inProcessMounts == nil {
		h.inProcessMounts = map[string]inProcessMountedComponent{}
	}
	session, _ := h.currentAgentSession()
	context := InProcessComponentContext{
		Session:     session,
		RuntimeHost: h.runtimeHost,
		ViewTree:    h.viewTreeHost,
	}
	currentEditorText := h.activeEditorText()
	registrations := h.inProcessUI.Slots()
	wanted := make(map[string]InProcessSlotRegistration, len(registrations))
	for _, registration := range registrations {
		wanted[registration.Key] = registration
	}
	for key, mounted := range h.inProcessMounts {
		next, ok := wanted[key]
		if ok && mounted.version == next.Version && mounted.slot == normalizeInProcessSlot(next.Slot) {
			continue
		}
		h.removeInProcessMount(mounted)
		delete(h.inProcessMounts, key)
	}
	for _, registration := range registrations {
		if _, ok := h.inProcessMounts[registration.Key]; ok {
			continue
		}
		component, dispose, err := registration.Factory(context)
		if err != nil {
			h.addStatus("In-process component " + registration.Key + " failed: " + err.Error())
			continue
		}
		if component == nil {
			continue
		}
		slot := normalizeInProcessSlot(registration.Slot)
		if slot == "editor" {
			h.configureInProcessEditorComponent(component, currentEditorText)
		}
		wrapped := &safeInProcessComponent{
			key:       registration.Key,
			component: component,
			onError: func(message string) {
				h.addStatus(message)
			},
		}
		wrapped.SetExpanded(h.toolOutputExpanded)
		mounted := inProcessMountedComponent{
			slot:      slot,
			version:   registration.Version,
			component: wrapped,
			dispose:   dispose,
			overlay:   cloneInProcessOverlayOptions(registration.Overlay),
		}
		if slot == "overlay" && h.ui != nil {
			if mounted.overlay != nil {
				mounted.overlayHandle = h.ui.ShowOverlay(mounted.component, *mounted.overlay)
			} else {
				mounted.overlayHandle = h.ui.ShowOverlay(mounted.component)
			}
		}
		h.inProcessMounts[registration.Key] = mounted
	}
	nextSlots := map[string][]gitui.Component{}
	for _, registration := range registrations {
		mounted, ok := h.inProcessMounts[registration.Key]
		if !ok || mounted.slot == "overlay" {
			continue
		}
		nextSlots[mounted.slot] = append(nextSlots[mounted.slot], mounted.component)
	}
	h.inProcessSlots = nextSlots
	for slot := range h.slots {
		h.refreshViewTreeSlot(slot)
	}
	h.refreshViewTreeEditorSlot()
}

func (h *CLIInteractiveTUIHost) configureInProcessEditorComponent(component gitui.Component, currentText string) {
	if h == nil || component == nil {
		return
	}
	editor, ok := component.(gitui.EditorComponent)
	if !ok {
		return
	}
	editor.SetText(currentText)
	if appearance, ok := component.(gitui.EditorAppearanceComponent); ok {
		settings := h.settingsManager()
		if settings != nil {
			appearance.SetPaddingX(settings.GetEditorPaddingX())
			appearance.SetAutocompleteMaxVisible(settings.GetAutocompleteMaxVisible())
		}
		appearance.SetBorderColor(func(text string) string { return text })
	}
	if autocomplete, ok := component.(gitui.EditorAutocompleteComponent); ok && h.autocompleteProvider != nil {
		autocomplete.SetAutocompleteProvider(h.autocompleteProvider)
	}
	if change, ok := component.(gitui.EditorChangeCallbackComponent); ok {
		change.SetOnChange(func(string) {
			h.requestRender(false)
		})
	}
	if submit, ok := component.(gitui.EditorSubmitCallbackComponent); ok {
		submit.SetOnSubmit(func(text string) {
			text = strings.TrimSpace(text)
			if text == "" {
				return
			}
			if history, ok := component.(gitui.EditorHistoryComponent); ok {
				history.AddToHistory(text)
			}
			editor.SetText("")
			go func() {
				if err := h.submitPrompt(text, nil); err != nil {
					h.addStatus("Error: " + err.Error())
				}
			}()
		})
	}
}

func cloneInProcessOverlayOptions(options *gitui.OverlayOptions) *gitui.OverlayOptions {
	if options == nil {
		return nil
	}
	value := *options
	return &value
}

func (h *CLIInteractiveTUIHost) removeInProcessMount(mounted inProcessMountedComponent) {
	if mounted.overlayHandle != nil {
		mounted.overlayHandle.Hide()
	}
	h.disposeInProcessMount(mounted)
}

func (h *CLIInteractiveTUIHost) disposeInProcessMount(mounted inProcessMountedComponent) {
	if h == nil || mounted.dispose == nil {
		return
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				h.addStatus(fmt.Sprintf("in-process component dispose failed: %v", recovered))
			}
		}()
		mounted.dispose()
	}()
}

func (h *CLIInteractiveTUIHost) refreshViewTreeSlots() {
	if h == nil {
		return
	}
	seen := map[string]struct{}{}
	for slot := range h.slots {
		for _, mount := range h.viewTreeHost.MountsBySlot(slot) {
			seen[mount.MountID] = struct{}{}
		}
		h.refreshViewTreeSlot(slot)
	}
	for _, mount := range h.viewTreeHost.MountsBySlot("editor") {
		seen[mount.MountID] = struct{}{}
	}
	h.refreshViewTreeEditorSlot()
	for _, mount := range h.viewTreeHost.MountsBySlot("overlay") {
		seen[mount.MountID] = struct{}{}
	}
	h.refreshViewTreeOverlays()
	for mountID := range h.views {
		if _, ok := seen[mountID]; !ok {
			delete(h.views, mountID)
		}
	}
}

func (h *CLIInteractiveTUIHost) refreshViewTreeSlot(slot string) {
	if h == nil || h.viewTreeHost == nil {
		return
	}
	container := h.slots[slot]
	if container == nil {
		return
	}
	mounts := h.viewTreeHost.MountsBySlot(slot)
	inProcess := h.inProcessSlots[slot]
	children := make([]gitui.Component, 0, len(inProcess)+len(mounts))
	children = append(children, inProcess...)
	for _, mount := range mounts {
		component := h.views[mount.MountID]
		if component == nil {
			component = NewMountedViewTreeComponent(h.viewTreeHost, mount.MountID)
			h.views[mount.MountID] = component
		}
		children = append(children, component)
	}
	container.SetChildren(children)
}

func (h *CLIInteractiveTUIHost) refreshViewTreeEditorSlot() {
	if h == nil || h.viewTreeHost == nil || h.editorContainer == nil {
		return
	}
	mounts := h.viewTreeHost.MountsBySlot("editor")
	inProcess := h.inProcessSlots["editor"]
	if len(mounts) == 0 && len(inProcess) == 0 {
		wasCustom := h.customEditorActive
		currentText := ""
		copyTextBack := false
		if wasCustom {
			if active, ok := h.activeEditorComponent(); ok && active != h.editor {
				currentText = h.activeEditorText()
				copyTextBack = true
			}
		}
		h.customEditorActive = false
		if copyTextBack && h.editor != nil {
			h.editor.SetText(currentText)
		}
		h.editorContainer.SetChildren([]gitui.Component{h.editor})
		if wasCustom && h.ui != nil {
			h.ui.SetFocus(h.editor)
		}
		return
	}

	children := make([]gitui.Component, 0, len(inProcess)+len(mounts))
	children = append(children, inProcess...)
	for _, mount := range mounts {
		component := h.views[mount.MountID]
		if component == nil {
			component = NewMountedViewTreeComponent(h.viewTreeHost, mount.MountID)
			h.views[mount.MountID] = component
		}
		children = append(children, component)
	}
	wasCustom := h.customEditorActive
	h.customEditorActive = true
	h.editorContainer.SetChildren(children)
	if h.ui != nil && len(children) > 0 && (!wasCustom || h.ui.FocusedComponent() == h.editor) {
		h.ui.SetFocus(children[0])
	}
}

func (h *CLIInteractiveTUIHost) refreshViewTreeOverlays() {
	if h == nil || h.ui == nil || h.viewTreeHost == nil {
		return
	}
	if h.overlays == nil {
		h.overlays = map[string]gitui.OverlayHandle{}
	}
	active := map[string]struct{}{}
	mounts := h.viewTreeHost.MountsBySlot("overlay")
	for _, mount := range mounts {
		active[mount.MountID] = struct{}{}
		if _, ok := h.overlays[mount.MountID]; ok {
			continue
		}
		component := h.views[mount.MountID]
		if component == nil {
			component = NewMountedViewTreeComponent(h.viewTreeHost, mount.MountID)
			h.views[mount.MountID] = component
		}
		h.overlays[mount.MountID] = h.ui.ShowOverlay(component, viewTreeOverlayOptionsToTUI(mount.Overlay))
	}
	for mountID, handle := range h.overlays {
		if _, ok := active[mountID]; ok {
			continue
		}
		handle.Hide()
		delete(h.overlays, mountID)
	}
}

func viewTreeOverlayOptionsToTUI(options *ViewTreeOverlayOptions) gitui.OverlayOptions {
	width := gitui.Cells(64)
	result := gitui.OverlayOptions{Width: &width, MinWidth: 30, Anchor: gitui.OverlayCenter}
	if options == nil {
		return result
	}
	if options.Width.Set {
		result.Width = viewTreeSizeValueToTUI(options.Width)
	}
	if options.MinWidth > 0 {
		result.MinWidth = options.MinWidth
	}
	if options.MaxHeight.Set {
		result.MaxHeight = viewTreeSizeValueToTUI(options.MaxHeight)
	}
	if strings.TrimSpace(options.Anchor) != "" {
		result.Anchor = gitui.OverlayAnchor(strings.TrimSpace(options.Anchor))
	}
	result.OffsetX = options.OffsetX
	result.OffsetY = options.OffsetY
	if options.Row.Set {
		result.Row = viewTreeSizeValueToTUI(options.Row)
	}
	if options.Col.Set {
		result.Col = viewTreeSizeValueToTUI(options.Col)
	}
	if options.Margin.Set {
		result.Margin = gitui.OverlayMargin{Top: options.Margin.Top, Right: options.Margin.Right, Bottom: options.Margin.Bottom, Left: options.Margin.Left}
	}
	result.NonCapturing = options.NonCapturing
	return result
}

func viewTreeSizeValueToTUI(value ViewTreeSizeValue) *gitui.SizeValue {
	if value.Percent {
		size := gitui.PercentFloat(value.PercentValue)
		return &size
	}
	size := gitui.Cells(value.Value)
	return &size
}

func (h *CLIInteractiveTUIHost) renderExistingMessages(populateHistory bool) {
	session := h.runtimeHost.PrintModeSession()
	if session == nil {
		return
	}
	messages := session.Messages()
	for _, message := range messages {
		h.addMessage(message)
		if populateHistory && message.Role == llm.RoleUser {
			h.addEditorHistory(interactiveTextFromLLMMessage(message))
		}
	}
	h.rendered = len(messages)
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) addEditorHistory(text string) {
	if h == nil || strings.TrimSpace(text) == "" {
		return
	}
	editor, ok := h.activeEditorComponent()
	if !ok {
		return
	}
	if history, ok := editor.(gitui.EditorHistoryComponent); ok {
		history.AddToHistory(text)
	}
}

func (h *CLIInteractiveTUIHost) showSessionCompactionNoticeOnStartup() {
	session, err := h.currentAgentSession()
	if err != nil || session == nil || session.SessionManager == nil {
		return
	}
	compactionCount := 0
	for _, entry := range session.SessionManager.GetEntries() {
		if entry.Type == "compaction" {
			compactionCount++
		}
	}
	if compactionCount == 0 {
		return
	}
	times := fmt.Sprintf("%d times", compactionCount)
	if compactionCount == 1 {
		times = "1 time"
	}
	h.addStatus("Session compacted " + times)
}

func (h *CLIInteractiveTUIHost) rerenderSessionMessages() {
	if h == nil || h.chat == nil {
		return
	}
	h.chat.Clear()
	h.pendingTools = map[string]*ToolExecutionComponent{}
	h.rendered = 0
	h.renderMessagesFrom(0)
}

func (h *CLIInteractiveTUIHost) submitPrompt(message string, images []llm.ContentPart) error {
	if handled, err := h.handleInlineCommand(message); handled || err != nil {
		return err
	}
	if handled, err := h.handleBuiltinSlashCommand(message); handled || err != nil {
		return err
	}
	if h.agentSessionCompacting() {
		return h.queuePromptDuringCompaction(message, images, "steering")
	}
	if h.agentSessionStreaming() {
		return h.queuePromptDuringStreaming(message, images, "steering")
	}
	h.mu.Lock()
	if h.editor != nil {
		h.editor.DisableSubmit = true
	}
	h.showLoaderLocked()
	start := h.rendered
	h.mu.Unlock()
	h.requestRender(false)

	session := h.runtimeHost.PrintModeSession()
	if session == nil {
		h.clearLoader()
		return errors.New("interactive TUI session is required")
	}
	finishPrompt := h.beginActivePrompt()
	if finishPrompt == nil {
		h.clearLoader()
		return nil
	}
	defer finishPrompt()
	if err := session.Prompt(message, PrintModePromptOptions{Images: images}); err != nil {
		h.clearLoader()
		return err
	}
	if err := session.WaitForIdle(); err != nil {
		h.clearLoader()
		return err
	}
	h.clearLoader()
	if h.liveSessionEventRendering() {
		h.syncRenderedMessageCount()
		h.requestRender(false)
		return nil
	}
	h.renderMessagesFrom(start)
	return nil
}

func (h *CLIInteractiveTUIHost) submitFollowUpPrompt(message string, images []llm.ContentPart) error {
	if h.agentSessionCompacting() {
		return h.queuePromptDuringCompaction(message, images, "follow-up")
	}
	if !h.agentSessionStreaming() {
		return h.submitPrompt(message, images)
	}
	return h.queuePromptDuringStreaming(message, images, "follow-up")
}

func (h *CLIInteractiveTUIHost) queuePromptDuringStreaming(message string, images []llm.ContentPart, kind string) error {
	session := h.agentSession()
	if session == nil {
		return errors.New("interactive TUI agent session is required for queued messages")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return nil
	}
	if h.isExtensionCommand(message) {
		if len(images) == 0 {
			return session.Prompt(message)
		}
		return session.PromptWithImages(message, images)
	}
	var err error
	if kind == "follow-up" {
		err = session.FollowUpWithImages(message, images)
	} else {
		err = session.SteerWithImages(message, images)
	}
	if err != nil {
		return err
	}
	h.refreshPendingMessagesDisplay()
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) queuePromptDuringCompaction(message string, images []llm.ContentPart, kind string) error {
	session := h.agentSession()
	if session == nil {
		return errors.New("interactive TUI agent session is required for queued messages")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return nil
	}
	if h.isExtensionCommand(message) {
		if len(images) == 0 {
			return session.Prompt(message)
		}
		return session.PromptWithImages(message, images)
	}
	if kind == "" {
		kind = "steering"
	}
	h.compactionQueue = append(h.compactionQueue, cliCompactionQueuedMessage{
		Text:   message,
		Images: normalizePromptImages(images),
		Mode:   kind,
	})
	h.refreshPendingMessagesDisplay()
	h.addStatus("Queued message for after compaction")
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) flushCompactionQueue(willRetry bool) {
	session := h.agentSession()
	if h == nil || session == nil {
		return
	}
	queued := h.takeCompactionQueue()
	if len(queued) == 0 {
		return
	}
	h.refreshPendingMessagesDisplay()
	if willRetry {
		if err := h.queueCompactionMessagesForActiveTurn(session, queued); err != nil {
			h.restoreCompactionQueue(session, queued, err)
		}
		return
	}
	firstPromptIndex := -1
	for index, message := range queued {
		if !h.isExtensionCommand(message.Text) {
			firstPromptIndex = index
			break
		}
	}
	if firstPromptIndex == -1 {
		for _, message := range queued {
			if err := h.submitCompactionQueuedExtensionCommand(session, message); err != nil {
				h.restoreCompactionQueue(session, queued, err)
				return
			}
		}
		return
	}
	for _, message := range queued[:firstPromptIndex] {
		if err := h.submitCompactionQueuedExtensionCommand(session, message); err != nil {
			h.restoreCompactionQueue(session, queued, err)
			return
		}
	}
	firstPrompt := queued[firstPromptIndex]
	rest := queued[firstPromptIndex+1:]
	go func() {
		if err := session.PromptWithImages(firstPrompt.Text, firstPrompt.Images); err != nil {
			h.restoreCompactionQueue(session, queued, err)
			return
		}
		if h.liveSessionEventRendering() {
			h.syncRenderedMessageCount()
			h.requestRender(false)
			return
		}
		h.renderMessagesFrom(h.rendered)
	}()
	if err := h.queueCompactionMessagesForActiveTurn(session, rest); err != nil {
		h.restoreCompactionQueue(session, queued, err)
		return
	}
	h.refreshPendingMessagesDisplay()
}

func (h *CLIInteractiveTUIHost) takeCompactionQueue() []cliCompactionQueuedMessage {
	if h == nil || len(h.compactionQueue) == 0 {
		return nil
	}
	queued := append([]cliCompactionQueuedMessage(nil), h.compactionQueue...)
	h.compactionQueue = nil
	return queued
}

func (h *CLIInteractiveTUIHost) restoreCompactionQueue(session *AgentSession, queued []cliCompactionQueuedMessage, err error) {
	if session != nil {
		session.ClearQueue()
	}
	if h != nil {
		h.compactionQueue = append(append([]cliCompactionQueuedMessage(nil), queued...), h.compactionQueue...)
		h.refreshPendingMessagesDisplay()
		if err != nil {
			h.addStatus("Failed to send queued messages: " + err.Error())
		}
	}
}

func (h *CLIInteractiveTUIHost) queueCompactionMessagesForActiveTurn(session *AgentSession, queued []cliCompactionQueuedMessage) error {
	for _, message := range queued {
		if h.isExtensionCommand(message.Text) {
			if err := h.submitCompactionQueuedExtensionCommand(session, message); err != nil {
				return err
			}
			continue
		}
		if message.Mode == "follow-up" {
			if err := session.FollowUpWithImages(message.Text, message.Images); err != nil {
				return err
			}
			continue
		}
		if err := session.SteerWithImages(message.Text, message.Images); err != nil {
			return err
		}
	}
	return nil
}

func (h *CLIInteractiveTUIHost) submitCompactionQueuedExtensionCommand(session *AgentSession, message cliCompactionQueuedMessage) error {
	if len(message.Images) == 0 {
		return session.Prompt(message.Text)
	}
	return session.PromptWithImages(message.Text, message.Images)
}

func (h *CLIInteractiveTUIHost) isExtensionCommand(text string) bool {
	session := h.agentSession()
	if session == nil || session.ExtensionRuntime == nil || !strings.HasPrefix(strings.TrimSpace(text), "/") {
		return false
	}
	name, _, ok := parseSlashCommandInvocation(strings.TrimSpace(text))
	if !ok {
		return false
	}
	return session.ExtensionRuntime.GetCommand(name) != nil
}

func (h *CLIInteractiveTUIHost) renderMessagesFrom(start int) {
	if h == nil || h.runtimeHost == nil {
		return
	}
	session := h.runtimeHost.PrintModeSession()
	if session == nil {
		return
	}
	messages := session.Messages()
	if start < 0 || start > len(messages) {
		start = 0
	}
	for _, message := range messages[start:] {
		h.addMessage(message)
	}
	h.rendered = len(messages)
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) addMessage(message llm.Message) {
	switch message.Role {
	case "custom":
		if !customMessageShouldDisplay(message) {
			return
		}
		if h.addRenderedCustomMessage(message) {
			return
		}
		h.addFallbackCustomMessage(message)
		return
	case "branchSummary":
		h.addBranchSummaryMessage(message)
		return
	case "compactionSummary":
		h.addCompactionSummaryMessage(message)
		return
	case llm.RoleAssistant:
		h.addAssistantMessage(message)
		return
	case llm.RoleToolResult:
		if h.addToolResultMessage(message) {
			return
		}
	case llm.RoleUser:
		if h.addSkillInvocationMessage(message) {
			return
		}
		h.addUserMessage(message)
		return
	}
	text := interactiveTextFromLLMMessage(message)
	if text == "" && message.ErrorMessage != "" {
		text = message.ErrorMessage
	}
	if strings.TrimSpace(text) == "" {
		return
	}
	prefix := ""
	switch message.Role {
	case llm.RoleToolResult:
		prefix = "Tool: "
	default:
		prefix = string(message.Role) + ": "
	}
	h.chat.AddChild(newCLIMarkdownWithOptions(prefix+text, gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}))
}

func (h *CLIInteractiveTUIHost) addUserMessage(message llm.Message) {
	if h == nil || h.chat == nil {
		return
	}
	text := interactiveTextFromLLMMessage(message)
	if strings.TrimSpace(text) == "" {
		return
	}
	h.addSpacerBeforeUserMessage()
	h.chat.AddChild(newCLIUserMessageComponent(text))
}

func (h *CLIInteractiveTUIHost) addSpacerBeforeUserMessage() {
	if h == nil || h.chat == nil || h.chat.ChildCount() == 0 {
		return
	}
	h.chat.AddChild(gitui.NewSpacer(1))
}

func (h *CLIInteractiveTUIHost) addBranchSummaryMessage(message llm.Message) {
	if h == nil || h.chat == nil {
		return
	}
	summary := strings.TrimSpace(interactiveTextFromLLMMessage(message))
	if summary == "" {
		return
	}
	h.chat.AddChild(newCLICollapsibleMarkdownMessage(cliCollapsibleMarkdownMessageOptions{
		Label:     "branch",
		Title:     "Branch Summary",
		Body:      summary,
		Collapsed: "Branch summary (" + h.toolExpandHint() + " to expand)",
		Expanded:  h.toolOutputExpanded,
	}))
}

func (h *CLIInteractiveTUIHost) addCompactionSummaryMessage(message llm.Message) {
	if h == nil || h.chat == nil {
		return
	}
	summary := strings.TrimSpace(interactiveTextFromLLMMessage(message))
	if summary == "" {
		return
	}
	tokensBefore := sessionSummaryTokensBefore(message)
	tokenText := "unknown"
	if tokensBefore > 0 {
		tokenText = formatIntWithCommas(tokensBefore)
	}
	h.chat.AddChild(newCLICollapsibleMarkdownMessage(cliCollapsibleMarkdownMessageOptions{
		Label:     "compaction",
		Title:     "Compacted from " + tokenText + " tokens",
		Body:      summary,
		Collapsed: "Compacted from " + tokenText + " tokens (" + h.toolExpandHint() + " to expand)",
		Expanded:  h.toolOutputExpanded,
	}))
}

func (h *CLIInteractiveTUIHost) addSkillInvocationMessage(message llm.Message) bool {
	if h == nil || h.chat == nil {
		return false
	}
	text := strings.TrimSpace(interactiveTextFromLLMMessage(message))
	skillBlock, ok := ParseExportHTMLSkillBlock(text)
	if !ok {
		return false
	}
	h.addSpacerBeforeUserMessage()
	h.chat.AddChild(newCLICollapsibleMarkdownMessage(cliCollapsibleMarkdownMessageOptions{
		Label:     "skill",
		Title:     skillBlock.Name,
		Body:      skillBlock.Content,
		Collapsed: skillBlock.Name + " (" + h.toolExpandHint() + " to expand)",
		Expanded:  h.toolOutputExpanded,
	}))
	if skillBlock.UserMessage != "" {
		h.chat.AddChild(newCLIUserMessageComponent(skillBlock.UserMessage))
	}
	return true
}

func (h *CLIInteractiveTUIHost) toolExpandHint() string {
	hint := ""
	if h != nil {
		hint = formatHotkeyKeys(keybindingValueKeys(h.effectiveKeybindings()["app.tools.expand"]), true)
	}
	if hint == "" {
		return "Ctrl+O"
	}
	return hint
}

func sessionSummaryTokensBefore(message llm.Message) int {
	details, ok := message.Details.(map[string]any)
	if !ok {
		return 0
	}
	return settingsValueInt(details["tokensBefore"], 0)
}

func formatIntWithCommas(value int) string {
	text := strconv.Itoa(value)
	if len(text) <= 3 {
		return text
	}
	var parts []string
	for len(text) > 3 {
		parts = append([]string{text[len(text)-3:]}, parts...)
		text = text[:len(text)-3]
	}
	parts = append([]string{text}, parts...)
	return strings.Join(parts, ",")
}

func customMessageShouldDisplay(message llm.Message) bool {
	if message.Display == nil {
		return true
	}
	return *message.Display
}

func (h *CLIInteractiveTUIHost) addRenderedCustomMessage(message llm.Message) bool {
	if message.Role != "custom" || strings.TrimSpace(message.CustomType) == "" || h == nil || h.chat == nil {
		return false
	}
	runtime := h.protocolRuntime()
	if runtime == nil {
		return false
	}
	renderer := runtime.GetMessageRenderer(message.CustomType)
	if renderer == nil {
		return false
	}
	h.chat.AddChild(cliRenderedLinesComponent{render: func(width int) []string {
		return renderer(message, map[string]any{"width": width, "expanded": h.toolOutputExpanded})
	}})
	return true
}

func (h *CLIInteractiveTUIHost) addFallbackCustomMessage(message llm.Message) {
	if h == nil || h.chat == nil {
		return
	}
	text := interactiveTextFromLLMMessage(message)
	if strings.TrimSpace(text) == "" {
		return
	}
	label := strings.TrimSpace(message.CustomType)
	if label == "" {
		label = "custom"
	}
	h.chat.AddChild(newCLIMarkdownWithOptions("["+label+"] "+text, gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}))
}

func (h *CLIInteractiveTUIHost) addAssistantMessage(message llm.Message) {
	component := newCLIAssistantMessageComponent(message, h.hideThinkingBlock(), h.hiddenThinkingLabelValue())
	if len(component.Render(80)) > 0 {
		h.chat.AddChild(component)
	}
	for _, part := range message.Content {
		if part.Type != llm.ContentToolCall {
			continue
		}
		component := h.newToolExecutionComponent(part.Name, part.ID, part.Arguments)
		component.SetArgsComplete()
		if message.StopReason == "aborted" || message.StopReason == "error" {
			errorMessage := assistantToolErrorMessage(message)
			component.UpdateResult(FileToolResult{Text: errorMessage, Content: []llm.ContentPart{llm.Text(errorMessage)}}, true)
		} else {
			if h.pendingTools == nil {
				h.pendingTools = map[string]*ToolExecutionComponent{}
			}
			h.pendingTools[part.ID] = component
		}
		h.chat.AddChild(component)
	}
}

func (h *CLIInteractiveTUIHost) hideThinkingBlock() bool {
	if settings := h.settingsManager(); settings != nil {
		return settings.GetHideThinkingBlock()
	}
	return false
}

func (h *CLIInteractiveTUIHost) hiddenThinkingLabelValue() string {
	if h != nil && strings.TrimSpace(h.hiddenThinkingLabel) != "" {
		return h.hiddenThinkingLabel
	}
	return "Thinking..."
}

func (h *CLIInteractiveTUIHost) updateAssistantThinkingPresentation() {
	if h == nil {
		return
	}
	hide := h.hideThinkingBlock()
	label := h.hiddenThinkingLabelValue()
	if h.chat != nil {
		for _, child := range h.chat.Children() {
			if component, ok := child.(*cliAssistantMessageComponent); ok {
				component.SetHideThinkingBlock(hide)
				component.SetHiddenThinkingLabel(label)
			}
		}
	}
	if h.streamingComponent != nil {
		h.streamingComponent.SetHideThinkingBlock(hide)
		h.streamingComponent.SetHiddenThinkingLabel(label)
	}
	h.requestRender(false)
}

func interactiveAssistantTextFromLLMMessage(message llm.Message, hideThinking bool, hiddenThinkingLabel string) string {
	parts := make([]string, 0, len(message.Content))
	if strings.TrimSpace(hiddenThinkingLabel) == "" {
		hiddenThinkingLabel = "Thinking..."
	}
	hasToolCalls := false
	for _, part := range message.Content {
		switch part.Type {
		case llm.ContentText:
			if strings.TrimSpace(part.Text) != "" {
				parts = append(parts, strings.TrimSpace(part.Text))
			}
		case llm.ContentThinking:
			if strings.TrimSpace(part.Thinking) == "" {
				continue
			}
			if hideThinking {
				parts = append(parts, hiddenThinkingLabel)
			} else {
				parts = append(parts, strings.TrimSpace(part.Thinking))
			}
		case llm.ContentToolCall:
			hasToolCalls = true
		}
	}
	if !hasToolCalls {
		if status := assistantMessageStatusText(message); status != "" {
			parts = append(parts, status)
		}
	}
	return strings.Join(parts, "\n\n")
}

func assistantMessageStatusText(message llm.Message) string {
	switch message.StopReason {
	case llm.StopReasonAborted:
		if errorMessage := strings.TrimSpace(message.ErrorMessage); errorMessage != "" && errorMessage != "Request was aborted" {
			return errorMessage
		}
		return "Operation aborted"
	case llm.StopReasonError:
		errorMessage := strings.TrimSpace(message.ErrorMessage)
		if errorMessage == "" {
			errorMessage = "Unknown error"
		}
		return "Error: " + errorMessage
	default:
		return ""
	}
}

func assistantToolErrorMessage(message llm.Message) string {
	if message.StopReason == llm.StopReasonAborted {
		if errorMessage := strings.TrimSpace(message.ErrorMessage); errorMessage != "" && errorMessage != "Request was aborted" {
			return errorMessage
		}
		return "Operation aborted"
	}
	if strings.TrimSpace(message.ErrorMessage) != "" {
		return message.ErrorMessage
	}
	return "Error"
}

func (h *CLIInteractiveTUIHost) addToolResultMessage(message llm.Message) bool {
	if h == nil || h.chat == nil || strings.TrimSpace(message.ToolCallID) == "" {
		return false
	}
	component := h.pendingTools[message.ToolCallID]
	if component == nil {
		return false
	}
	component.UpdateResult(fileToolResultFromLLMMessage(message), message.IsError)
	delete(h.pendingTools, message.ToolCallID)
	h.requestRender(false)
	return true
}

func (h *CLIInteractiveTUIHost) newToolExecutionComponent(name, callID string, args any) *ToolExecutionComponent {
	definition := ToolDefinition{Name: name}
	if runtime := h.protocolRuntime(); runtime != nil {
		definition = runtime.GetRegisteredToolDefinition(name)
	}
	options := ToolExecutionOptions{}
	if settings := h.settingsManager(); settings != nil {
		showImages := settings.GetShowImages()
		options.ShowImages = &showImages
		options.ImageWidthCells = settings.GetImageWidthCells()
	}
	component := NewToolExecutionComponent(name, callID, args, definition, h.interactiveCWD(), options)
	component.SetExpanded(h.toolOutputExpanded)
	return component
}

func (h *CLIInteractiveTUIHost) protocolRuntime() *ProtocolExtensionRuntime {
	if h == nil || h.runtimeHost == nil {
		return nil
	}
	provider, ok := h.runtimeHost.(protocolExtensionRuntimeProvider)
	if !ok {
		return nil
	}
	return provider.ProtocolExtensionRuntime()
}

func (h *CLIInteractiveTUIHost) interactiveCWD() string {
	if h == nil || h.runtimeHost == nil {
		return ""
	}
	if provider, ok := h.runtimeHost.(interface{ PrintModeCWD() string }); ok {
		return provider.PrintModeCWD()
	}
	return ""
}

func (h *CLIInteractiveTUIHost) requestRender(force bool) {
	if h != nil && h.renderDeferred.Load() && !force {
		return
	}
	h.refreshFooterState()
	if h != nil && h.ui != nil {
		h.ui.RequestRender(force)
	}
}

func (h *CLIInteractiveTUIHost) refreshFooterState() {
	if h == nil || h.footer == nil {
		return
	}
	cwd := h.interactiveCWD()
	if h.footerDataProvider != nil {
		h.footerDataProvider.SetCwd(cwd)
	}
	state := FooterState{CWD: cwd}
	if h.footerDataProvider != nil {
		state.GitBranch = h.footerDataProvider.GetGitBranch()
		state.ExtensionStatuses = h.footerDataProvider.GetExtensionStatuses()
	}
	session := h.footerAgentSession()
	if session != nil && session.Agent != nil {
		model := session.Agent.State.Model
		state.ModelID = model.ID
		state.Provider = model.Provider
		state.Reasoning = model.Reasoning
		state.ThinkingLevel = session.Agent.State.ThinkingLevel
		state.ContextWindow = model.ContextWindow
		stats := session.GetSessionStats()
		state.Usage = []FooterUsage{{
			Input:      stats.Tokens.Input,
			Output:     stats.Tokens.Output,
			CacheRead:  stats.Tokens.CacheRead,
			CacheWrite: stats.Tokens.CacheWrite,
			CostTotal:  stats.Tokens.Cost.Total,
		}}
		if stats.ContextUsage != nil {
			state.ContextWindow = stats.ContextUsage.ContextWindow
			state.ContextPercent = stats.ContextUsage.Percent
		}
		h.footer.SetAutoCompactEnabled(session.CompactionSettings.Enabled)
		if session.SessionManager != nil {
			state.SessionName = session.SessionManager.GetSessionName()
		}
	}
	if runtime := h.modelRuntime(); runtime != nil {
		state.AvailableProviderCount = footerAvailableProviderCount(
			runtime,
		)
		if session != nil && session.Agent != nil {
			state.UsingOAuth = runtime.IsUsingOAuth(
				session.Agent.State.Model.Provider,
			)
		}
	} else if registry := h.modelRegistry(); registry != nil {
		state.AvailableProviderCount = footerAvailableProviderCount(
			registry,
		)
		if session != nil && session.Agent != nil {
			state.UsingOAuth = registry.IsUsingOAuth(
				session.Agent.State.Model,
			)
		}
	}
	h.footer.SetState(state)
}

func (h *CLIInteractiveTUIHost) refreshPendingMessagesDisplay() {
	if h == nil || h.pendingMessages == nil {
		return
	}
	h.pendingMessages.Clear()
	session := h.agentSession()
	if session == nil {
		return
	}
	steering := session.GetSteeringMessages()
	followUp := session.GetFollowUpMessages()
	for _, message := range h.compactionQueue {
		if message.Mode == "follow-up" {
			followUp = append(followUp, message.Text)
		} else {
			steering = append(steering, message.Text)
		}
	}
	if len(steering) == 0 && len(followUp) == 0 {
		return
	}
	h.pendingMessages.AddChild(cliPendingMessagesComponent{
		Steering: append([]string(nil), steering...),
		FollowUp: append([]string(nil), followUp...),
	})
}

func (h *CLIInteractiveTUIHost) restoreQueuedMessagesToEditor(showStatus bool) int {
	session := h.agentSession()
	if session == nil || h == nil || h.editor == nil {
		return 0
	}
	steering, followUp := session.ClearQueue()
	for _, message := range h.takeCompactionQueue() {
		if message.Mode == "follow-up" {
			followUp = append(followUp, message.Text)
		} else {
			steering = append(steering, message.Text)
		}
	}
	queued := append(append([]string(nil), steering...), followUp...)
	if len(queued) == 0 {
		if showStatus {
			h.addStatus("No queued messages to restore")
		}
		h.refreshPendingMessagesDisplay()
		return 0
	}
	current := strings.TrimSpace(h.editor.GetText())
	parts := append([]string{}, queued...)
	if current != "" {
		parts = append(parts, current)
	}
	h.editor.SetText(strings.Join(parts, "\n\n"))
	h.refreshPendingMessagesDisplay()
	if showStatus {
		h.addStatus(fmt.Sprintf("Restored %d queued message%s to editor", len(queued), pluralSuffix(len(queued))))
	}
	h.requestRender(false)
	return len(queued)
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

type cliPendingMessagesComponent struct {
	Steering []string
	FollowUp []string
}

func (c cliPendingMessagesComponent) Invalidate() {}

func (c cliPendingMessagesComponent) Render(width int) []string {
	var lines []string
	lines = append(lines, "")
	for _, message := range c.Steering {
		lines = append(lines, truncateSelectorLine("Steering: "+message, width))
	}
	for _, message := range c.FollowUp {
		lines = append(lines, truncateSelectorLine("Follow-up: "+message, width))
	}
	lines = append(lines, truncateSelectorLine("-> Alt+Up to edit all queued messages", width))
	return lines
}

func (h *CLIInteractiveTUIHost) agentSessionStreaming() bool {
	session := h.agentSession()
	return session != nil && session.IsStreaming()
}

func (h *CLIInteractiveTUIHost) agentSessionCompacting() bool {
	session := h.agentSession()
	return session != nil && session.IsCompacting()
}

func (h *CLIInteractiveTUIHost) agentSession() *AgentSession {
	if h == nil || h.runtimeHost == nil {
		return nil
	}
	if provider, ok := h.runtimeHost.(agentSessionProvider); ok {
		return provider.AgentSession()
	}
	printSession := h.runtimeHost.PrintModeSession()
	if provider, ok := printSession.(agentSessionProvider); ok {
		return provider.AgentSession()
	}
	return nil
}

func (h *CLIInteractiveTUIHost) updateEditorBorderColor() {
	if h == nil || h.editor == nil {
		return
	}
	level := "off"
	if session := h.agentSession(); session != nil && session.Agent != nil {
		level = firstNonEmptyString(session.Agent.State.ThinkingLevel, "off")
	}
	h.editor.SetBorderColor(tuiThemeThinkingBorder(level))
}

func (h *CLIInteractiveTUIHost) footerAgentSession() *AgentSession {
	return h.agentSession()
}

func footerAvailableProviderCount(
	models availableModelsProvider,
) int {
	if models == nil {
		return 0
	}
	seen := map[string]struct{}{}
	for _, model := range models.GetAvailable() {
		if strings.TrimSpace(model.Provider) != "" {
			seen[model.Provider] = struct{}{}
		}
	}
	return len(seen)
}

func (h *CLIInteractiveTUIHost) handleBuiltinSlashCommand(text string) (bool, error) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return false, nil
	}
	name, args, ok := parseSlashCommandInvocation(text)
	if !ok {
		return false, nil
	}
	hasArgs := strings.TrimSpace(args) != ""
	switch name {
	case "settings":
		if hasArgs {
			return false, nil
		}
		return true, h.handleSettingsSlashCommand()
	case "trust":
		if hasArgs {
			return false, nil
		}
		return true, h.handleTrustSlashCommand()
	case "model":
		return true, h.handleModelSlashCommand(args)
	case "scoped-models":
		if hasArgs {
			return false, nil
		}
		return true, h.handleScopedModelsSlashCommand()
	case "name":
		return true, h.handleNameSlashCommand(args)
	case "session":
		if hasArgs {
			return false, nil
		}
		return true, h.handleSessionSlashCommand()
	case "hotkeys":
		if hasArgs {
			return false, nil
		}
		return true, h.handleHotkeysSlashCommand()
	case "changelog":
		if hasArgs {
			return false, nil
		}
		return true, h.handleChangelogSlashCommand()
	case "export":
		path := ""
		if parsed, ok := GetPathCommandArgument(text, "/export"); ok {
			path = parsed
		}
		return true, h.handleExportSlashCommand(path)
	case "share":
		if hasArgs {
			return false, nil
		}
		return true, h.handleShareSlashCommand()
	case "import":
		return true, h.handleImportSlashCommand(text)
	case "resume":
		if hasArgs {
			return false, nil
		}
		return true, h.handleResumeSlashCommand(text)
	case "copy":
		if hasArgs {
			return false, nil
		}
		return true, h.handleCopySlashCommand()
	case "new":
		if hasArgs {
			return false, nil
		}
		return true, h.handleNewSlashCommand()
	case "clone":
		if hasArgs {
			return false, nil
		}
		return true, h.handleCloneSlashCommand()
	case "fork":
		if hasArgs {
			return false, nil
		}
		return true, h.handleForkSlashCommand(args)
	case "tree":
		if hasArgs {
			return false, nil
		}
		return true, h.handleTreeSlashCommand(args)
	case "login":
		if hasArgs {
			return false, nil
		}
		return true, h.handleLoginSlashCommand(args)
	case "logout":
		if hasArgs {
			return false, nil
		}
		return true, h.handleLogoutSlashCommand(args)
	case "compact":
		return true, h.handleCompactSlashCommand(args)
	case "reload":
		if hasArgs {
			return false, nil
		}
		return true, h.handleReloadSlashCommand()
	case "debug":
		if hasArgs {
			return false, nil
		}
		return true, h.handleDebugCommand()
	case "arminsayshi":
		if hasArgs {
			return false, nil
		}
		return true, h.handleArminSaysHiCommand()
	case "dementedelves":
		if hasArgs {
			return false, nil
		}
		return true, h.handleDementedDelvesCommand()
	case "quit":
		if hasArgs {
			return false, nil
		}
		h.Stop()
		return true, nil
	default:
		return false, nil
	}
}

func (h *CLIInteractiveTUIHost) handleInlineCommand(text string) (bool, error) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "!") {
		return false, nil
	}
	excluded := strings.HasPrefix(text, "!!")
	command := strings.TrimSpace(strings.TrimPrefix(text, "!"))
	if excluded {
		command = strings.TrimSpace(strings.TrimPrefix(text, "!!"))
	}
	if command == "" {
		return false, nil
	}
	if session := h.agentSession(); session != nil && session.IsBashRunning() {
		h.addStatus("A bash command is already running. Press Esc to cancel it first.")
		if h.editor != nil {
			h.editor.SetText(text)
		}
		h.requestRender(false)
		return true, nil
	}
	return true, h.handleBashCommand(command, excluded)
}

func (h *CLIInteractiveTUIHost) settingsManager() *SettingsManager {
	if h == nil || h.runtimeHost == nil {
		return nil
	}
	provider, ok := h.runtimeHost.(settingsManagerProvider)
	if !ok {
		return nil
	}
	return provider.SettingsManager()
}

func (h *CLIInteractiveTUIHost) handleBashCommand(command string, excludeFromContext bool) error {
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	if session.IsBashRunning() {
		h.addStatus("A bash command is already running. Press Esc to cancel it first.")
		return nil
	}
	if result, handled, err := h.emitUserBashInterception(session, command, excludeFromContext); err != nil || handled {
		if err != nil {
			return err
		}
		h.renderBashResult(session, command, result, excludeFromContext)
		return nil
	}
	component := NewBashExecutionComponent(command, BashExecutionOptions{ExcludeFromContext: excludeFromContext})
	if session.IsStreaming() && h.pendingMessages != nil {
		h.pendingMessages.AddChild(component)
	} else {
		h.chat.AddChild(component)
	}
	h.requestRender(false)
	result, err := session.ExecuteBash(command, AgentSessionBashOptions{
		ExcludeFromContext: excludeFromContext,
		Operations:         h.bashOperations,
		OnChunk: func(chunk string) {
			component.AppendOutput(chunk)
			h.requestRender(false)
		},
	})
	component.SetComplete(result.ExitCode, result.Cancelled, BashExecutionCompleteOptions{
		Truncated:      result.Truncated,
		FullOutputPath: result.FullOutputPath,
	})
	h.rendered = len(session.Messages())
	h.requestRender(false)
	return err
}

func (h *CLIInteractiveTUIHost) emitUserBashInterception(session *AgentSession, command string, excludeFromContext bool) (BashResult, bool, error) {
	if h == nil || session == nil {
		return BashResult{}, false, nil
	}
	cwd := ""
	if session.SessionManager != nil {
		cwd = session.SessionManager.GetCWD()
	}
	event := ProtocolSessionEvent{
		Type:               ProtocolEventUserBash,
		Command:            command,
		CWD:                cwd,
		ExcludeFromContext: excludeFromContext,
	}
	if runtime := h.protocolRuntime(); runtime != nil && runtime.HasHandlers(ProtocolEventUserBash) {
		result, err := runtime.EmitSessionEvent(event)
		if err != nil {
			return BashResult{}, false, err
		}
		if result.BashResultSet {
			if result.BashResult == nil {
				return BashResult{}, true, nil
			}
			return *result.BashResult, true, nil
		}
	}
	if h.processSupervisor == nil {
		return BashResult{}, false, nil
	}
	result, handled, err := h.processSupervisor.EmitUserBash(context.Background(), map[string]any{
		"command":            command,
		"cwd":                cwd,
		"excludeFromContext": excludeFromContext,
	})
	if err != nil || !handled {
		return BashResult{}, handled, err
	}
	if result == nil {
		return BashResult{}, true, nil
	}
	return *result, true, nil
}

func (h *CLIInteractiveTUIHost) renderBashResult(session *AgentSession, command string, result BashResult, excludeFromContext bool) {
	component := NewBashExecutionComponent(command, BashExecutionOptions{ExcludeFromContext: excludeFromContext})
	if session.IsStreaming() && h.pendingMessages != nil {
		h.pendingMessages.AddChild(component)
	} else {
		h.chat.AddChild(component)
	}
	if result.Output != "" {
		component.AppendOutput(result.Output)
	}
	component.SetComplete(result.ExitCode, result.Cancelled, BashExecutionCompleteOptions{
		Truncated:      result.Truncated,
		FullOutputPath: result.FullOutputPath,
	})
	session.RecordBashResult(command, result, AgentSessionBashRecordOptions{ExcludeFromContext: excludeFromContext})
	h.rendered = len(session.Messages())
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) resetChatState() {
	if h == nil {
		return
	}
	if h.chat != nil {
		h.chat.Clear()
	}
	h.lastStatusSpacer = nil
	h.lastStatusText = nil
	h.pendingTools = map[string]*ToolExecutionComponent{}
	h.rendered = 0
	h.updateTerminalTitle()
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) currentAgentSession() (*AgentSession, error) {
	if h == nil || h.runtimeHost == nil {
		return nil, errors.New("interactive TUI session is required")
	}
	session := h.runtimeHost.PrintModeSession()
	if session == nil {
		return nil, errors.New("interactive TUI session is required")
	}
	provider, ok := session.(agentSessionProvider)
	if !ok || provider.AgentSession() == nil {
		return nil, errors.New("interactive TUI slash commands require an agent session")
	}
	return provider.AgentSession(), nil
}

func (h *CLIInteractiveTUIHost) agentSessionRuntimeHost() *AgentSessionRuntimeHost {
	if h == nil || h.runtimeHost == nil {
		return nil
	}
	provider, ok := h.runtimeHost.(agentSessionRuntimeHostProvider)
	if !ok {
		return nil
	}
	return provider.AgentSessionRuntimeHost()
}

func (h *CLIInteractiveTUIHost) newRPCSessionHost() (*RPCSessionHost, error) {
	session, err := h.currentAgentSession()
	if err != nil {
		return nil, err
	}
	host := NewRPCSessionHost(session)
	host.Settings = h.settingsManager()
	host.ViewTreeHost = h.viewTreeHost
	host.TUIEditor = h
	host.TUIDialog = h
	host.TUITitle = h
	host.TUIWorking = h
	host.TUIThinkingLabel = h
	host.TUIStatus = h
	host.TUITheme = h
	host.TUIToolExpansion = h
	if runtime := h.modelRuntime(); runtime != nil {
		host.ProviderAuthStatus = runtime.GetProviderAuthStatus
		host.AvailableModels = runtime.GetAvailable()
	} else if registry := h.modelRegistry(); registry != nil {
		host.ProviderAuthStatus = registry.GetProviderAuthStatus
		host.AvailableModels = registry.GetAvailable()
	}
	if owner, ok := h.runtimeHost.(*agentSessionPrintModeHost); ok {
		host.OnSessionReplaced(func(session *AgentSession) {
			owner.session = session
		})
	}
	return host, nil
}

func (h *CLIInteractiveTUIHost) Stop() {
	if session := h.agentSession(); session != nil && session.IsStreaming() {
		_ = session.Abort()
	}
	h.once.Do(func() {
		close(h.done)
	})
}

func (h *CLIInteractiveTUIHost) waitForActivePrompts(timeout time.Duration) {
	if h == nil {
		return
	}
	deadline := time.Now().Add(timeout)
	for {
		h.activePromptMu.Lock()
		count := h.activePromptCount
		h.activePromptMu.Unlock()
		if count == 0 {
			return
		}
		if timeout > 0 && !time.Now().Before(deadline) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (h *CLIInteractiveTUIHost) beginActivePrompt() func() {
	if h == nil {
		return nil
	}
	h.activePromptMu.Lock()
	defer h.activePromptMu.Unlock()
	select {
	case <-h.done:
		return nil
	default:
		h.activePromptCount++
		return func() {
			h.activePromptMu.Lock()
			if h.activePromptCount > 0 {
				h.activePromptCount--
			}
			h.activePromptMu.Unlock()
		}
	}
}

func (h *CLIInteractiveTUIHost) stopUI() {
	if h == nil {
		return
	}
	if h.unwatch != nil {
		h.unwatch()
		h.unwatch = nil
	}
	if h.unwatchCommands != nil {
		h.unwatchCommands()
		h.unwatchCommands = nil
	}
	if h.unwatchAutocomplete != nil {
		h.unwatchAutocomplete()
		h.unwatchAutocomplete = nil
	}
	if h.unwatchRenderers != nil {
		h.unwatchRenderers()
		h.unwatchRenderers = nil
	}
	if h.unwatchFooterBranch != nil {
		h.unwatchFooterBranch()
		h.unwatchFooterBranch = nil
	}
	h.removeSessionWatcher(true)
	if h.unwatchRuntimeSession != nil {
		h.unwatchRuntimeSession()
		h.unwatchRuntimeSession = nil
	}
	if h.restoreRuntimeRebind != nil {
		h.restoreRuntimeRebind()
	}
	if h.unwatchInProcess != nil {
		h.unwatchInProcess()
		h.unwatchInProcess = nil
	}
	if len(h.unwatchProtocolErrors) > 0 {
		for runtime, unwatch := range h.unwatchProtocolErrors {
			if unwatch != nil {
				unwatch()
			}
			delete(h.unwatchProtocolErrors, runtime)
		}
	}
	if h.footerDataProvider != nil {
		h.footerDataProvider.Dispose()
		h.footerDataProvider = nil
	}
	for key, mounted := range h.inProcessMounts {
		h.removeInProcessMount(mounted)
		delete(h.inProcessMounts, key)
	}
	h.inProcessSlots = nil
	if !h.deadTerminal.Load() {
		h.setTerminalProgress(false)
	}
	if h.ui != nil {
		if h.deadTerminal.Load() {
			h.ui.StopWithoutRender()
		} else {
			h.drainTerminalInputOnShutdown()
			h.ui.Stop()
		}
	}
	if h.tuiKeybindingsInstalled {
		if h.previousTUIKeybindings != nil {
			gitui.SetKeybindings(h.previousTUIKeybindings)
		} else {
			gitui.SetKeybindings(gitui.NewKeybindingsManager())
		}
		h.tuiKeybindingsInstalled = false
		h.previousTUIKeybindings = nil
	}
}

func (h *CLIInteractiveTUIHost) setTerminalProgress(active bool) {
	if h == nil || h.terminal == nil || !h.terminalProgressEnabled() {
		return
	}
	h.reportTerminalError(h.terminal.SetProgress(active))
}

func (h *CLIInteractiveTUIHost) terminalProgressEnabled() bool {
	provider, ok := h.runtimeHost.(settingsManagerProvider)
	if !ok {
		return false
	}
	settings := provider.SettingsManager()
	return settings != nil && settings.GetShowTerminalProgress()
}
