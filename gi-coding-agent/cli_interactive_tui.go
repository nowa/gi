package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
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
	workingMessage          string
	workingIndicator        *TUIWorkingIndicatorOptions
	workingVisible          bool
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

type cliReloadBoxComponent struct {
	message string
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

func (c *cliReloadBoxComponent) Invalidate() {}

func (c *cliReloadBoxComponent) Render(width int) []string {
	width = max(20, width)
	border := tuiThemeBorder(strings.Repeat("─", width))
	message := tuiThemeMuted(" " + firstNonEmptyString(strings.TrimSpace(c.message), "Reloading..."))
	return []string{
		border,
		"",
		gitui.TruncateToWidth(message, width, "", true),
		"",
		border,
	}
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
	bottom = appendRendered(bottom, l.host.editorContainer, width, height)
	bottom = appendRendered(bottom, l.host.slots["belowEditor"], width, height)
	if l.host.footer != nil {
		bottom = appendRendered(bottom, l.host.footer, width, height)
	}
	bottom = appendRendered(bottom, l.host.slots["footer"], width, height)

	lines := make([]string, 0, len(top)+len(bottom))
	lines = append(lines, top...)
	lines = append(lines, bottom...)
	return lines
}

func appendRendered(lines []string, component gitui.Component, width, height int) []string {
	if component == nil {
		return lines
	}
	if sized, ok := component.(gitui.SizeAwareComponent); ok {
		return append(lines, sized.RenderWithSize(width, height)...)
	}
	return append(lines, component.Render(width)...)
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
	h.renderExistingMessages()
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
			h.loader.Stop()
			if h.chat != nil {
				h.chat.RemoveChild(h.loader)
			}
			h.loader = nil
		} else {
			h.loader.SetMessage(h.workingMessageLocked())
			h.loader.SetIndicator(h.workingIndicatorOptionsLocked())
		}
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
	h.rerenderSessionMessages()
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
	add(TUIThemeInfo{Name: "system", Builtin: true})
	if session, err := h.currentAgentSession(); err == nil && session != nil && session.ResourceLoader != nil {
		if loader, ok := session.ResourceLoader.(interface{ GetThemes() ResourceThemesResult }); ok {
			for _, theme := range loader.GetThemes().Themes {
				add(TUIThemeInfo{Name: theme.Name, Path: theme.SourcePath})
			}
		}
	}
	if current := h.CurrentTUITheme(); current != "" {
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
	if h.unwatchSession != nil {
		h.unwatchSession()
		h.unwatchSession = nil
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

func (h *CLIInteractiveTUIHost) handleProtocolShortcutKey(data string, keybindings KeybindingsConfig) bool {
	if h == nil || data == "" {
		return false
	}
	session := h.agentSession()
	if session == nil || session.ExtensionRuntime == nil {
		return false
	}
	shortcuts := session.ExtensionRuntime.Shortcuts(keybindings).Shortcuts
	if len(shortcuts) == 0 {
		return false
	}
	for key, shortcut := range shortcuts {
		if !gitui.MatchesKey(data, key) {
			continue
		}
		if shortcut.Handler != nil {
			go func(registration ProtocolShortcutRegistration) {
				if err := registration.Handler(); err != nil {
					h.addStatus("Error: Shortcut handler error: " + err.Error())
				}
			}(shortcut)
		}
		return true
	}
	return false
}

func (h *CLIInteractiveTUIHost) handleClearKey() {
	if h == nil {
		return
	}
	now := time.Now()
	if !h.lastClearKeyTime.IsZero() && now.Sub(h.lastClearKeyTime) < 500*time.Millisecond {
		h.Stop()
		return
	}
	h.lastClearKeyTime = now
	h.setActiveEditorText("")
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) handleEscapeKey() bool {
	if h == nil {
		return false
	}
	if h.activeEditorShowingAutocomplete() {
		return false
	}
	session := h.agentSession()
	if session != nil && session.IsBashRunning() {
		session.AbortBash()
		h.addStatus("Bash command cancelled")
		return true
	}
	if session != nil && session.IsRetrying() {
		session.AbortRetry()
		h.addStatus("Retry cancelled")
		return true
	}
	if session != nil && session.IsBranchSummaryRunning() {
		session.AbortBranchSummary()
		return true
	}
	if session != nil && session.IsCompacting() {
		session.AbortCompaction()
		h.addStatus("Compaction cancelled")
		return true
	}
	if session != nil && session.IsStreaming() {
		h.restoreQueuedMessagesToEditor(false)
		_ = session.Abort()
		return true
	}
	if strings.HasPrefix(strings.TrimSpace(h.activeEditorText()), "!") {
		h.setActiveEditorText("")
		h.requestRender(false)
		return true
	}
	if !h.editorIsEmpty() {
		return false
	}
	action := "tree"
	if settings := h.settingsManager(); settings != nil {
		action = settings.GetDoubleEscapeAction()
	}
	if action == "none" {
		return true
	}
	now := time.Now()
	if h.lastEscapeKeyTime.IsZero() || now.Sub(h.lastEscapeKeyTime) >= 500*time.Millisecond {
		h.lastEscapeKeyTime = now
		return true
	}
	h.lastEscapeKeyTime = time.Time{}
	go h.runDoubleEscapeAction(action)
	return true
}

func (h *CLIInteractiveTUIHost) editorIsEmpty() bool {
	if h == nil {
		return true
	}
	return strings.TrimSpace(h.activeEditorText()) == ""
}

func (h *CLIInteractiveTUIHost) runDoubleEscapeAction(action string) {
	var err error
	switch action {
	case "fork":
		err = h.handleForkSlashCommand("")
	default:
		err = h.handleTreeSlashCommand("")
	}
	if err != nil {
		h.addStatus("Error: " + err.Error())
	}
}

func (h *CLIInteractiveTUIHost) handleAppActionKey(data string, keybindings KeybindingsConfig) bool {
	if h == nil {
		return false
	}
	if h.focusedDefaultEditor() && matchesKeybindingAction(data, keybindings, "app.clipboard.pasteImage") {
		go h.handleClipboardImagePaste()
		return true
	}
	if h.activeEditorShowingAutocomplete() {
		return false
	}
	switch {
	case matchesKeybindingAction(data, keybindings, "app.thinking.cycle"):
		go h.cycleThinkingLevelFromKey()
		return true
	case matchesKeybindingAction(data, keybindings, "app.model.cycleForward"):
		go h.cycleModelFromKey("forward")
		return true
	case matchesKeybindingAction(data, keybindings, "app.model.cycleBackward"):
		go h.cycleModelFromKey("backward")
		return true
	case matchesKeybindingAction(data, keybindings, "app.model.select"):
		go func() {
			if err := h.handleModelSlashCommand(""); err != nil {
				h.addStatus("Error: " + err.Error())
			}
		}()
		return true
	case matchesKeybindingAction(data, keybindings, "app.tools.expand"):
		h.toggleToolOutputExpansion()
		return true
	case matchesKeybindingAction(data, keybindings, "app.thinking.toggle"):
		go h.toggleThinkingBlockVisibility()
		return true
	case matchesKeybindingAction(data, keybindings, "app.editor.external"):
		if !h.focusedDefaultEditor() {
			return false
		}
		go h.openExternalEditor()
		return true
	case matchesKeybindingAction(data, keybindings, "app.suspend"):
		go h.handleSuspend()
		return true
	case matchesKeybindingAction(data, keybindings, "app.session.new"):
		go h.runSessionActionKey("new")
		return true
	case matchesKeybindingAction(data, keybindings, "app.session.tree"):
		go h.runSessionActionKey("tree")
		return true
	case matchesKeybindingAction(data, keybindings, "app.session.fork"):
		go h.runSessionActionKey("fork")
		return true
	case matchesKeybindingAction(data, keybindings, "app.session.resume"):
		go h.runSessionActionKey("resume")
		return true
	default:
		return false
	}
}

func (h *CLIInteractiveTUIHost) runSessionActionKey(action string) {
	var err error
	switch action {
	case "new":
		err = h.handleNewSlashCommand()
	case "tree":
		err = h.handleTreeSlashCommand("")
	case "fork":
		err = h.handleForkSlashCommand("")
	case "resume":
		err = h.handleResumeSlashCommand("/resume")
	default:
		return
	}
	if err != nil {
		h.addStatus("Error: " + err.Error())
	}
}

func (h *CLIInteractiveTUIHost) handleSuspend() {
	if h == nil || h.ui == nil {
		return
	}
	mode := &InteractiveMode{
		UI: cliInteractiveSuspendUI{ui: h.ui},
		ShowStatus: func(text string) {
			h.addStatus(text)
		},
		Suspend: h.suspend,
	}
	if err := mode.HandleCtrlZ(); err != nil {
		h.addStatus("Error: " + err.Error())
	}
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

func isShiftTabKey(data string) bool {
	event := gitui.ParseKey(data)
	return event.Shift && (event.Key == gitui.KeyTab || event.Key == gitui.KeyBacktab)
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
	h.addStatus("Thinking: " + level)
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
		h.addStatus("Only one model available")
		return
	}
	h.updateEditorBorderColor()
	h.addStatus("Model: " + result.Model.Provider + "/" + result.Model.ID + " (thinking: " + result.ThinkingLevel + ")")
	h.maybeWarnAboutAnthropicSubscriptionAuth(result.Model)
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
	h.rerenderSessionMessages()
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
	h.unwatchSession = session.Subscribe(func(event AgentSessionEvent) {
		switch event.Type {
		case "agent_start":
			h.pendingTools = map[string]*ToolExecutionComponent{}
			h.streamingMessage = nil
			h.streamingComponent = nil
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
			h.refreshPendingMessagesDisplay()
			h.requestRender(false)
		case ProtocolEventSessionInfoChanged:
			h.updateTerminalTitle()
			h.refreshFooterState()
			h.requestRender(false)
		case "thinking_level_changed":
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
	h.refreshPendingMessagesDisplay()
}

func (h *CLIInteractiveTUIHost) handleAgentEnd() {
	if h == nil {
		return
	}
	h.setTerminalProgress(false)
	h.clearRetryStatus()
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
	h.addStatus(label)
}

func (h *CLIInteractiveTUIHost) handleCompactionEnd(event AgentSessionEvent) {
	if h == nil {
		return
	}
	h.setTerminalProgress(false)
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
		h.renderExistingMessages()
	case strings.TrimSpace(event.ErrorMessage) != "":
		h.addStatus(strings.TrimSpace(event.ErrorMessage))
	}
	h.requestRender(false)
	go h.flushCompactionQueue(event.WillRetry)
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
	return h != nil && h.unwatchSession != nil && h.agentSession() != nil
}

func (h *CLIInteractiveTUIHost) handleAutoRetryStart(event AgentSessionEvent) {
	seconds := (event.DelayMs + 999) / 1000
	if seconds < 0 {
		seconds = 0
	}
	h.clearRetryStatus()
	h.retryStatus = h.addStatus(fmt.Sprintf("Retrying (%d/%d) in %ds... (Esc to cancel)", event.Attempt, event.MaxAttempts, seconds))
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
	if h == nil || h.retryStatus == nil {
		return
	}
	if h.chat != nil {
		h.chat.RemoveChild(h.retryStatus)
	}
	h.retryStatus = nil
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) handleMessageQueueKey(data string, keybindings KeybindingsConfig) bool {
	if h == nil {
		return false
	}
	if h.activeEditorShowingAutocomplete() {
		return false
	}
	switch {
	case matchesKeybindingAction(data, keybindings, "app.message.followUp"):
		go h.submitEditorTextAs("follow-up")
		return true
	case matchesKeybindingAction(data, keybindings, "app.message.dequeue"):
		h.restoreQueuedMessagesToEditor(true)
		return true
	case matchesKeybindingAction(data, keybindings, "tui.input.submit") && h.agentSessionStreaming():
		go h.submitEditorTextAs("steering")
		return true
	default:
		return false
	}
}

func (h *CLIInteractiveTUIHost) submitEditorTextAs(kind string) {
	if h == nil {
		return
	}
	editor, ok := h.activeEditorComponent()
	if !ok {
		return
	}
	text := strings.TrimSpace(h.activeEditorText())
	if text == "" {
		return
	}
	if history, ok := editor.(gitui.EditorHistoryComponent); ok {
		history.AddToHistory(text)
	}
	editor.SetText("")
	h.requestRender(false)
	var err error
	if kind == "follow-up" {
		err = h.submitFollowUpPrompt(text, nil)
	} else {
		err = h.submitPrompt(text, nil)
	}
	if err != nil {
		h.addStatus("Error: " + err.Error())
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
	h.autocompleteProvider = provider
	h.editor.SetAutocompleteProvider(provider)
	if active, ok := h.activeEditorComponent(); ok && active != h.editor {
		if editor, ok := active.(gitui.EditorAutocompleteComponent); ok {
			editor.SetAutocompleteProvider(provider)
		}
	}
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
	filter := strings.TrimSpace(prefix)
	if filter != "" {
		models = gitui.FuzzyFilter(models, filter, func(model llm.Model) string {
			return strings.Join([]string{model.ID, model.Provider}, " ")
		})
	}
	items := make([]gitui.AutocompleteItem, 0, len(models))
	for _, model := range models {
		value := scopedModelFullID(model)
		description := strings.TrimSpace(model.Provider)
		items = append(items, gitui.AutocompleteItem{
			Value:       value,
			Label:       model.ID,
			Description: description,
		})
	}
	return items
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

func (h *CLIInteractiveTUIHost) renderExistingMessages() {
	session := h.runtimeHost.PrintModeSession()
	if session == nil {
		return
	}
	messages := session.Messages()
	for _, message := range messages {
		h.addMessage(message)
	}
	h.rendered = len(messages)
	h.requestRender(false)
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
	h.chat.AddChild(newCLIUserMessageComponent(text))
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

type cliRenderedLinesComponent struct {
	render func(width int) []string
}

func (c cliRenderedLinesComponent) Render(width int) []string {
	if c.render == nil {
		return nil
	}
	return normalizeRenderedLines(c.render(width))
}

func (c cliRenderedLinesComponent) Invalidate() {}

func (h *CLIInteractiveTUIHost) addStatus(text string) *gitui.Text {
	if strings.TrimSpace(text) == "" || h.chat == nil {
		return nil
	}
	if statusTextCoalescible(text) {
		children := h.chat.Children()
		if len(children) > 1 && h.lastStatusText != nil && h.lastStatusSpacer != nil &&
			children[len(children)-1] == h.lastStatusText &&
			children[len(children)-2] == h.lastStatusSpacer {
			h.lastStatusText.SetText(tuiThemeStatusText(text))
			h.requestRender(false)
			return h.lastStatusText
		}
	}
	spacer := gitui.NewSpacer(1)
	status := gitui.NewText(tuiThemeStatusText(text), 1, 0)
	h.chat.AddChild(spacer)
	h.chat.AddChild(status)
	if statusTextNeedsTrailingSpacer(text) {
		h.chat.AddChild(gitui.NewSpacer(1))
	}
	if statusTextCoalescible(text) {
		h.lastStatusSpacer = spacer
		h.lastStatusText = status
	} else {
		h.lastStatusSpacer = nil
		h.lastStatusText = nil
	}
	h.requestRender(false)
	return status
}

func (h *CLIInteractiveTUIHost) addWarning(text string) *gitui.Text {
	if h == nil || h.chat == nil {
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if !strings.HasPrefix(text, "Warning:") {
		text = "Warning: " + text
	}
	h.chat.AddChild(gitui.NewSpacer(1))
	warning := gitui.NewText(tuiThemeWarning(text), 1, 0)
	h.chat.AddChild(warning)
	h.lastStatusSpacer = nil
	h.lastStatusText = nil
	h.requestRender(false)
	return warning
}

func tuiThemeStatusText(text string) string {
	switch {
	case strings.HasPrefix(text, "Error:"), strings.HasPrefix(text, "Failed "):
		return tuiThemeError(text)
	case strings.HasPrefix(text, "Warning:"), strings.HasPrefix(text, "models.json error:"):
		return tuiThemeWarning(text)
	default:
		return tuiThemeDim(text)
	}
}

func statusTextCoalescible(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	for _, prefix := range []string{"Error:", "Warning:", "models.json error:", "Failed "} {
		if strings.HasPrefix(text, prefix) {
			return false
		}
	}
	return true
}

func statusTextNeedsTrailingSpacer(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasPrefix(text, "Error:") || strings.HasPrefix(text, "Failed ")
}

func (h *CLIInteractiveTUIHost) showExtensionError(event ProtocolExtensionError) {
	if h == nil || h.chat == nil {
		return
	}
	path := strings.TrimSpace(event.ExtensionPath)
	if path == "" {
		path = "extension"
	}
	message := strings.TrimSpace(event.Error)
	if message == "" {
		message = "Unknown error"
	}
	h.chat.AddChild(gitui.NewText(`Extension "`+path+`" error: `+message, 1, 0))
	if stackLines := extensionErrorStackLines(event.Stack); len(stackLines) > 0 {
		h.chat.AddChild(gitui.NewText(strings.Join(stackLines, "\n"), 1, 0))
	}
	h.lastStatusSpacer = nil
	h.lastStatusText = nil
	h.requestRender(false)
}

func extensionErrorStackLines(stack string) []string {
	lines := strings.Split(strings.TrimSpace(stack), "\n")
	if len(lines) <= 1 {
		return nil
	}
	result := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		result = append(result, "  "+line)
	}
	return result
}

func (h *CLIInteractiveTUIHost) showLoaderLocked() {
	if h.ui == nil || h.loader != nil || !h.workingVisible {
		return
	}
	options := h.workingIndicatorOptionsLocked()
	options.TUI = h.ui
	h.loader = gitui.NewLoader(h.workingMessageLocked(), options)
	h.chat.AddChild(h.loader)
}

func (h *CLIInteractiveTUIHost) workingMessageLocked() string {
	if strings.TrimSpace(h.workingMessage) != "" {
		return h.workingMessage
	}
	return "Thinking..."
}

func (h *CLIInteractiveTUIHost) workingIndicatorOptionsLocked() gitui.LoaderIndicatorOptions {
	if h.workingIndicator == nil {
		return gitui.LoaderIndicatorOptions{}
	}
	return gitui.LoaderIndicatorOptions{
		Frames:     cloneOptionalStringSlice(h.workingIndicator.Frames),
		IntervalMs: h.workingIndicator.IntervalMs,
	}
}

func cloneOptionalStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}

func (h *CLIInteractiveTUIHost) clearLoader() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.loader != nil && h.chat != nil {
		h.loader.Stop()
		h.chat.RemoveChild(h.loader)
		h.loader = nil
	}
	if h.editor != nil {
		h.editor.DisableSubmit = false
	}
	h.requestRender(false)
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
	if registryProvider, ok := h.runtimeHost.(modelRegistryProvider); ok {
		registry := registryProvider.ModelRegistry()
		state.AvailableProviderCount = footerAvailableProviderCount(registry)
		if session != nil && session.Agent != nil && registry != nil {
			state.UsingOAuth = registry.IsUsingOAuth(session.Agent.State.Model)
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

func footerAvailableProviderCount(registry *ModelRegistry) int {
	if registry == nil {
		return 0
	}
	seen := map[string]struct{}{}
	for _, model := range registry.GetAvailable() {
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

func (h *CLIInteractiveTUIHost) handleSettingsSlashCommand() error {
	host, err := h.newRPCSessionHost()
	if err != nil {
		return err
	}
	state := host.GetState()
	settings := h.settingsManager()
	if !h.exitAfterInitial {
		return h.handleSettingsSelectDialog(host, state, settings)
	}
	return h.renderSettingsSummary(state, settings)
}

func (h *CLIInteractiveTUIHost) renderSettingsSummary(state RPCSessionState, settings *SettingsManager) error {
	model := ""
	if state.Model != nil {
		model = state.Model.Provider + "/" + state.Model.ID
	}
	rows := []string{
		"| Setting | Value |",
		"|---|---|",
		"| Current model | " + markdownTableValue(model) + " |",
		"| Thinking | " + markdownTableValue(state.ThinkingLevel) + " |",
		"| Queue | " + markdownTableValue("steering="+state.SteeringMode+", follow-up="+state.FollowUpMode) + " |",
		"| Session | " + markdownTableValue(firstNonEmptyString(state.SessionName, state.SessionID)) + " |",
	}
	if settings != nil {
		rows = append(rows,
			"| Default provider | "+markdownTableValue(settings.GetDefaultProvider())+" |",
			"| Default model | "+markdownTableValue(settings.GetDefaultModel())+" |",
			"| Default thinking | "+markdownTableValue(settings.GetDefaultThinkingLevel())+" |",
			"| Theme | "+markdownTableValue(settings.GetTheme())+" |",
			"| Session dir | "+markdownTableValue(settings.GetSessionDir())+" |",
			"| Image auto resize | "+markdownTableValue(fmt.Sprintf("%t", settings.GetImageAutoResize()))+" |",
			"| Block images | "+markdownTableValue(fmt.Sprintf("%t", settings.GetBlockImages()))+" |",
			"| Install telemetry | "+markdownTableValue(fmt.Sprintf("%t", settings.GetEnableInstallTelemetry()))+" |",
		)
	}
	h.chat.AddChild(newCLIMarkdownWithOptions("**Settings**\n\n"+strings.Join(rows, "\n"), gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}))
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) handleSettingsSelectDialog(host *RPCSessionHost, state RPCSessionState, settings *SettingsManager) error {
	if settings == nil {
		return h.renderSettingsSummary(state, settings)
	}
	if h.ui == nil {
		return errors.New("interactive TUI is not ready")
	}
	resultCh := make(chan struct{}, 1)
	list := gitui.NewSettingsList(settingsListItems(host, state, settings, h.availableThemeNames(settings.GetTheme()), settingsListItemsOptions{
		OnThemePreview: h.previewTUITheme,
	}), 10, tuiThemeSettingsList(), gitui.SettingsListOptions{
		EnableSearch: true,
		OnChange: func(id, newValue string) {
			h.applySettingsListChange(host, settings, id, newValue)
		},
		OnCancel: func() {
			select {
			case resultCh <- struct{}{}:
			default:
			}
		},
	})
	restore := h.showEditorReplacement(cliSettingsListComponent{list: list}, list)
	select {
	case <-resultCh:
		restore()
		h.clearTUIThemePreview()
		return nil
	case <-h.done:
		restore()
		h.clearTUIThemePreview()
		return nil
	}
}

func (h *CLIInteractiveTUIHost) showEditorReplacement(component gitui.Component, focus gitui.Component) func() {
	if h == nil || h.editorContainer == nil || h.ui == nil || component == nil {
		return func() {}
	}
	previousChildren := h.editorContainer.Children()
	previousFocus := h.ui.FocusedComponent()
	h.editorContainer.SetChildren([]gitui.Component{component})
	if focus != nil {
		h.ui.SetFocus(focus)
	} else {
		h.ui.SetFocus(component)
	}
	h.requestRender(false)
	return func() {
		if len(previousChildren) == 0 && h.editor != nil {
			previousChildren = []gitui.Component{h.editor}
		}
		h.editorContainer.SetChildren(previousChildren)
		if previousFocus != nil {
			h.ui.SetFocus(previousFocus)
		} else if h.editor != nil {
			h.ui.SetFocus(h.editor)
		}
		h.requestRender(false)
	}
}

type cliSettingsListComponent struct {
	list *gitui.SettingsList
}

func (c cliSettingsListComponent) Invalidate() {
	if c.list != nil {
		c.list.Invalidate()
	}
}

func (c cliSettingsListComponent) Render(width int) []string {
	width = max(1, width)
	lines := []string{tuiThemeBorder(strings.Repeat("─", width))}
	if c.list != nil {
		lines = append(lines, c.list.Render(width)...)
	}
	lines = append(lines, tuiThemeBorder(strings.Repeat("─", width)))
	return lines
}

func (c cliSettingsListComponent) HandleInput(data string) {
	if c.list != nil {
		c.list.HandleInput(data)
	}
}

type cliSettingsListDialog struct {
	title string
	list  *gitui.SettingsList
}

func (c cliSettingsListDialog) Invalidate() {
	if c.list != nil {
		c.list.Invalidate()
	}
}

func (c cliSettingsListDialog) Render(width int) []string {
	width = max(32, width)
	innerWidth := max(1, width-2)
	lines := []string{dialogBorder(width), dialogLine(firstNonEmptyString(c.title, "Settings"), innerWidth)}
	lines = append(lines, dialogLine("", innerWidth))
	if c.list != nil {
		for _, line := range c.list.Render(innerWidth) {
			lines = append(lines, dialogLine(line, innerWidth))
		}
	}
	lines = append(lines, dialogBorder(width))
	return lines
}

func (c cliSettingsListDialog) HandleInput(data string) {
	if c.list != nil {
		c.list.HandleInput(data)
	}
}

type settingsListItemsOptions struct {
	OnThemePreview func(string)
}

type httpIdleTimeoutChoice struct {
	Label     string
	TimeoutMS int
}

var httpIdleTimeoutChoices = []httpIdleTimeoutChoice{
	{Label: "30 sec", TimeoutMS: 30_000},
	{Label: "1 min", TimeoutMS: 60_000},
	{Label: "2 min", TimeoutMS: 120_000},
	{Label: "5 min", TimeoutMS: 300_000},
	{Label: "disabled", TimeoutMS: 0},
}

func formatHTTPIdleTimeoutMS(timeoutMS int) string {
	for _, choice := range httpIdleTimeoutChoices {
		if choice.TimeoutMS == timeoutMS {
			return choice.Label
		}
	}
	return strconv.FormatFloat(float64(timeoutMS)/1000, 'f', -1, 64) + " sec"
}

func httpIdleTimeoutLabels() []string {
	labels := make([]string, 0, len(httpIdleTimeoutChoices))
	for _, choice := range httpIdleTimeoutChoices {
		labels = append(labels, choice.Label)
	}
	return labels
}

func httpIdleTimeoutMSForLabel(label string) (int, bool) {
	for _, choice := range httpIdleTimeoutChoices {
		if choice.Label == label {
			return choice.TimeoutMS, true
		}
	}
	return 0, false
}

func settingsFollowUpDescription() string {
	key := formatHotkeyKeys(keybindingValueKeys(DefaultProtocolKeybindings()["app.message.followUp"]), true)
	if key == "" {
		key = "Option+Enter"
	}
	return key + " queues follow-up messages until agent stops. 'one-at-a-time': deliver one, wait for response. 'all': deliver all at once."
}

func settingsListItems(host *RPCSessionHost, state RPCSessionState, settings *SettingsManager, themes []string, options ...settingsListItemsOptions) []gitui.SettingItem {
	thinkingLevels := []string{string(ThinkingOff)}
	if host != nil && host.Session != nil && host.Session.Agent != nil {
		thinkingLevels = llm.GetSupportedThinkingLevels(host.Session.Agent.State.Model)
		if len(thinkingLevels) == 0 {
			thinkingLevels = []string{string(ThinkingOff)}
		}
	}
	opts := settingsListItemsOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	themeSubmenu := settingsSelectSubmenu("Theme", "Select color theme", settingsThemeOptions(themes, settings.GetTheme()))
	if opts.OnThemePreview != nil {
		themeSubmenu = settingsSelectSubmenuWithSelectionChange(
			"Theme",
			"Select color theme",
			settingsThemeOptions(themes, settings.GetTheme()),
			opts.OnThemePreview,
		)
	}
	items := []gitui.SettingItem{
		{ID: "autocompact", Label: "Auto-compact", Description: "Automatically compact context when it gets too large", CurrentValue: fmt.Sprintf("%t", state.AutoCompactionEnabled), Values: []string{"true", "false"}},
		{ID: "auto-resize-images", Label: "Auto-resize images", Description: "Resize large images to 2000x2000 max for better model compatibility", CurrentValue: fmt.Sprintf("%t", settings.GetImageAutoResize()), Values: []string{"true", "false"}},
		{ID: "block-images", Label: "Block images", Description: "Prevent images from being sent to LLM providers", CurrentValue: fmt.Sprintf("%t", settings.GetBlockImages()), Values: []string{"false", "true"}},
		{ID: "skill-commands", Label: "Skill commands", Description: "Register skills as /skill:name commands", CurrentValue: fmt.Sprintf("%t", settings.GetEnableSkillCommands()), Values: []string{"true", "false"}},
		{ID: "show-hardware-cursor", Label: "Show hardware cursor", Description: "Show the terminal cursor while still positioning it for IME support", CurrentValue: fmt.Sprintf("%t", settings.GetShowHardwareCursor()), Values: []string{"true", "false"}},
		{ID: "editor-padding", Label: "Editor padding", Description: "Horizontal padding for input editor (0-3)", CurrentValue: fmt.Sprintf("%d", settings.GetEditorPaddingX()), Values: []string{"0", "1", "2", "3"}},
		{ID: "autocomplete-max-visible", Label: "Autocomplete max items", Description: "Max visible items in autocomplete dropdown (3-20)", CurrentValue: fmt.Sprintf("%d", settings.GetAutocompleteMaxVisible()), Values: []string{"3", "5", "7", "10", "15", "20"}},
		{ID: "clear-on-shrink", Label: "Clear on shrink", Description: "Clear empty rows when content shrinks (may cause flicker)", CurrentValue: fmt.Sprintf("%t", settings.GetClearOnShrink()), Values: []string{"true", "false"}},
		{ID: "terminal-progress", Label: "Terminal progress", Description: "Show OSC 9;4 progress indicators in the terminal tab bar", CurrentValue: fmt.Sprintf("%t", settings.GetShowTerminalProgress()), Values: []string{"true", "false"}},
		{ID: "steering-mode", Label: "Steering mode", Description: "Enter while streaming queues steering messages. 'one-at-a-time': deliver one, wait for response. 'all': deliver all at once.", CurrentValue: state.SteeringMode, Values: []string{"one-at-a-time", "all"}},
		{ID: "follow-up-mode", Label: "Follow-up mode", Description: settingsFollowUpDescription(), CurrentValue: state.FollowUpMode, Values: []string{"one-at-a-time", "all"}},
		{ID: "transport", Label: "Transport", Description: "Preferred transport for providers that support multiple transports", CurrentValue: settings.GetTransport(), Values: []string{"sse", "websocket", "websocket-cached", "auto"}},
		{ID: "http-idle-timeout", Label: "HTTP idle timeout", Description: "Maximum idle gap while waiting for HTTP headers or body chunks. Disable for local models that pause longer than five minutes.", CurrentValue: formatHTTPIdleTimeoutMS(settings.GetHTTPIdleTimeoutMS()), Values: httpIdleTimeoutLabels()},
		{ID: "hide-thinking", Label: "Hide thinking", Description: "Hide thinking blocks in assistant responses", CurrentValue: fmt.Sprintf("%t", settings.GetHideThinkingBlock()), Values: []string{"true", "false"}},
		{ID: "collapse-changelog", Label: "Collapse changelog", Description: "Show condensed changelog after updates", CurrentValue: fmt.Sprintf("%t", settings.GetCollapseChangelog()), Values: []string{"true", "false"}},
		{ID: "quiet-startup", Label: "Quiet startup", Description: "Disable verbose printing at startup", CurrentValue: fmt.Sprintf("%t", settings.GetQuietStartup()), Values: []string{"true", "false"}},
		{ID: "install-telemetry", Label: "Install telemetry", Description: "Send an anonymous version/update ping after changelog-detected updates", CurrentValue: fmt.Sprintf("%t", settings.GetEnableInstallTelemetry()), Values: []string{"true", "false"}},
		{ID: "double-escape-action", Label: "Double-escape action", Description: "Action when pressing Escape twice with empty editor", CurrentValue: settings.GetDoubleEscapeAction(), Values: []string{"tree", "fork", "none"}},
		{ID: "tree-filter-mode", Label: "Tree filter mode", Description: "Default filter when opening /tree", CurrentValue: settings.GetTreeFilterMode(), Values: []string{"default", "no-tools", "user-only", "labeled-only", "all"}},
		{ID: "warnings", Label: "Warnings", Description: "Enable or disable individual warnings", CurrentValue: "configure", Submenu: settingsWarningsSubmenu(settings)},
		{ID: "thinking", Label: "Thinking level", Description: "Reasoning depth for thinking-capable models", CurrentValue: state.ThinkingLevel, Submenu: settingsSelectSubmenu("Thinking Level", "Select reasoning depth for thinking-capable models", settingsThinkingOptions(thinkingLevels, state.ThinkingLevel))},
		{ID: "theme", Label: "Theme", Description: "Color theme for the interface", CurrentValue: settings.GetTheme(), Submenu: themeSubmenu},
	}
	if gitui.GetCapabilities().Images {
		items = insertSettingItems(items, 1,
			gitui.SettingItem{ID: "show-images", Label: "Show images", Description: "Render images inline in terminal", CurrentValue: fmt.Sprintf("%t", settings.GetShowImages()), Values: []string{"true", "false"}},
			gitui.SettingItem{ID: "image-width-cells", Label: "Image width", Description: "Preferred inline image width in terminal cells", CurrentValue: fmt.Sprintf("%d", settings.GetImageWidthCells()), Values: []string{"60", "80", "120"}},
		)
	}
	return items
}

func insertSettingItems(items []gitui.SettingItem, index int, inserted ...gitui.SettingItem) []gitui.SettingItem {
	if len(inserted) == 0 {
		return items
	}
	index = min(max(index, 0), len(items))
	result := make([]gitui.SettingItem, 0, len(items)+len(inserted))
	result = append(result, items[:index]...)
	result = append(result, inserted...)
	result = append(result, items[index:]...)
	return result
}

func settingsSelectSubmenu(title, message string, options []TUIDialogOption) func(currentValue string, done func(selectedValue string, changed bool)) gitui.Component {
	return func(currentValue string, done func(selectedValue string, changed bool)) gitui.Component {
		return newCLISelectDialog(title, message, options, dialogDefaultOptionIndex(options, currentValue), func(option TUIDialogOption) {
			done(dialogStringValue(dialogOptionValue(option)), true)
		}, func() {
			done(currentValue, false)
		})
	}
}

func settingsSelectSubmenuWithSelectionChange(title, message string, options []TUIDialogOption, onSelectionChange func(string)) func(currentValue string, done func(selectedValue string, changed bool)) gitui.Component {
	return func(currentValue string, done func(selectedValue string, changed bool)) gitui.Component {
		return newCLISelectDialogWithOptions(title, message, options, dialogDefaultOptionIndex(options, currentValue), func(option TUIDialogOption) {
			done(dialogStringValue(dialogOptionValue(option)), true)
		}, func() {
			if onSelectionChange != nil {
				onSelectionChange(currentValue)
			}
			done(currentValue, false)
		}, cliSelectDialogOptions{
			OnSelectionChange: func(option TUIDialogOption) {
				if onSelectionChange != nil {
					onSelectionChange(dialogStringValue(dialogOptionValue(option)))
				}
			},
		})
	}
}

func settingsThinkingOptions(levels []string, current string) []TUIDialogOption {
	options := make([]TUIDialogOption, 0, len(levels))
	for _, level := range levels {
		description := thinkingLevelDescription(level)
		if level == current {
			description = strings.TrimSpace(description + " (current)")
		}
		options = append(options, TUIDialogOption{ID: level, Label: level, Description: description, Value: level})
	}
	return options
}

func settingsThemeOptions(themes []string, current string) []TUIDialogOption {
	options := make([]TUIDialogOption, 0, len(themes))
	for _, themeName := range themes {
		description := ""
		if themeName == current {
			description = "(current)"
		}
		options = append(options, TUIDialogOption{ID: themeName, Label: themeName, Description: description, Value: themeName})
	}
	return options
}

func settingsWarningsSubmenu(settings *SettingsManager) func(currentValue string, done func(selectedValue string, changed bool)) gitui.Component {
	return func(currentValue string, done func(selectedValue string, changed bool)) gitui.Component {
		currentWarnings := settings.GetWarnings()
		items := []gitui.SettingItem{
			{
				ID:           "warning-anthropic-extra-usage",
				Label:        "Anthropic extra usage",
				Description:  "Warn when Anthropic subscription auth may use paid extra usage",
				CurrentValue: fmt.Sprintf("%t", currentWarnings.AnthropicExtraUsage),
				Values:       []string{"true", "false"},
			},
		}
		return gitui.NewSettingsList(items, min(len(items), 10), tuiThemeSettingsList(), gitui.SettingsListOptions{
			OnChange: func(id, newValue string) {
				if id == "warning-anthropic-extra-usage" {
					settings.SetWarningAnthropicExtraUsage(newValue == "true")
				}
			},
			OnCancel: func() {
				done(currentValue, false)
			},
		})
	}
}

func thinkingLevelDescription(level string) string {
	switch level {
	case "off":
		return "No reasoning"
	case "minimal":
		return "Very brief reasoning (~1k tokens)"
	case "low":
		return "Light reasoning (~2k tokens)"
	case "medium":
		return "Moderate reasoning (~8k tokens)"
	case "high":
		return "Deep reasoning (~16k tokens)"
	case "xhigh":
		return "Maximum reasoning (~32k tokens)"
	default:
		return ""
	}
}

func (h *CLIInteractiveTUIHost) applySettingsListChange(host *RPCSessionHost, settings *SettingsManager, id, newValue string) {
	switch id {
	case "autocompact":
		enabled := newValue == "true"
		if err := host.SetAutoCompaction(&enabled); err != nil {
			h.addStatus("Error: " + err.Error())
		}
	case "show-images":
		settings.SetShowImages(newValue == "true")
		h.applyToolImageSettings(settings)
	case "image-width-cells":
		if width, err := strconv.Atoi(newValue); err == nil {
			settings.SetImageWidthCells(width)
			h.applyToolImageSettings(settings)
		}
	case "auto-resize-images", "image-auto-resize":
		settings.SetImageAutoResize(newValue == "true")
	case "block-images":
		settings.SetBlockImages(newValue == "true")
	case "skill-commands":
		settings.SetEnableSkillCommands(newValue == "true")
		h.refreshEditorAutocompleteProvider()
	case "show-hardware-cursor":
		settings.SetShowHardwareCursor(newValue == "true")
		if h.ui != nil {
			h.ui.SetShowHardwareCursor(settings.GetShowHardwareCursor())
		}
	case "editor-padding":
		if padding, err := strconv.Atoi(newValue); err == nil {
			settings.SetEditorPaddingX(padding)
			if h.editor != nil {
				h.editor.SetPaddingX(settings.GetEditorPaddingX())
			}
		}
	case "autocomplete-max-visible":
		if visible, err := strconv.Atoi(newValue); err == nil {
			settings.SetAutocompleteMaxVisible(visible)
			if h.editor != nil {
				h.editor.SetAutocompleteMaxVisible(settings.GetAutocompleteMaxVisible())
			}
		}
	case "clear-on-shrink":
		settings.SetClearOnShrink(newValue == "true")
		if h.ui != nil {
			h.ui.SetClearOnShrink(settings.GetClearOnShrink())
		}
	case "terminal-progress":
		settings.SetShowTerminalProgress(newValue == "true")
	case "thinking":
		if err := host.SetThinkingLevel(newValue); err != nil {
			h.addStatus("Error: " + err.Error())
		}
		h.updateEditorBorderColor()
	case "theme":
		h.clearTUIThemePreview()
		settings.SetTheme(newValue)
		h.requestRender(true)
	case "steering-mode":
		if err := host.SetSteeringMode(newValue); err != nil {
			h.addStatus("Error: " + err.Error())
		}
	case "follow-up-mode":
		if err := host.SetFollowUpMode(newValue); err != nil {
			h.addStatus("Error: " + err.Error())
		}
	case "transport":
		settings.SetTransport(newValue)
	case "http-idle-timeout":
		if timeoutMS, ok := httpIdleTimeoutMSForLabel(newValue); ok {
			settings.SetHTTPIdleTimeoutMS(timeoutMS)
		}
	case "hide-thinking":
		settings.SetHideThinkingBlock(newValue == "true")
		h.rerenderSessionMessages()
	case "collapse-changelog":
		settings.SetCollapseChangelog(newValue == "true")
	case "quiet-startup":
		settings.SetQuietStartup(newValue == "true")
	case "install-telemetry":
		settings.SetEnableInstallTelemetry(newValue == "true")
	case "double-escape-action":
		settings.SetDoubleEscapeAction(newValue)
	case "tree-filter-mode":
		settings.SetTreeFilterMode(newValue)
	case "warning-anthropic-extra-usage":
		settings.SetWarningAnthropicExtraUsage(newValue == "true")
	}
}

func (h *CLIInteractiveTUIHost) applyToolImageSettings(settings *SettingsManager) {
	if h == nil || settings == nil {
		return
	}
	apply := func(component *ToolExecutionComponent) {
		if component == nil {
			return
		}
		component.SetShowImages(settings.GetShowImages())
		component.SetImageWidthCells(settings.GetImageWidthCells())
	}
	for _, component := range h.pendingTools {
		apply(component)
	}
	if h.chat != nil {
		for _, child := range h.chat.Children() {
			if component, ok := child.(*ToolExecutionComponent); ok {
				apply(component)
			}
		}
	}
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) packageResourceManager() (*DefaultPackageManager, error) {
	settings := h.settingsManager()
	if settings == nil {
		return nil, errors.New("package resource selector requires settings")
	}
	return NewDefaultPackageManager(PackageManagerOptions{
		CWD:             h.interactiveCWD(),
		AgentDir:        settings.agentDir,
		SettingsManager: settings,
	}), nil
}

func packageResourceSettingItems(resources []PackageResourceToggleItem) ([]gitui.SettingItem, map[string]resourceToggleSelection) {
	items := make([]gitui.SettingItem, 0, len(resources))
	toggles := make(map[string]resourceToggleSelection, len(resources))
	for _, resource := range resources {
		id := packageResourceSettingID(resource)
		state := "disabled"
		if resource.Enabled {
			state = "enabled"
		}
		items = append(items, gitui.SettingItem{
			ID:           id,
			Label:        packageResourceLabel(resource),
			Description:  packageResourceDescription(resource),
			CurrentValue: state,
			Values:       []string{"enabled", "disabled"},
		})
		if resource.Metadata.Origin == "top-level" {
			toggles[id] = resourceToggleSelection{TopLevel: TopLevelResourceToggle{
				Scope:        resource.Scope,
				ResourceType: resource.ResourceType,
				Pattern:      resource.Pattern,
				Enabled:      resource.Enabled,
			}}
		} else {
			toggles[id] = resourceToggleSelection{Package: PackageResourceToggle{
				Source:       resource.Source,
				Scope:        resource.Scope,
				ResourceType: resource.ResourceType,
				Pattern:      resource.Pattern,
				Enabled:      resource.Enabled,
			}}
		}
	}
	return items, toggles
}

type resourceToggleSelection struct {
	Package  PackageResourceToggle
	TopLevel TopLevelResourceToggle
}

func applyResourceToggle(manager *DefaultPackageManager, toggle resourceToggleSelection, enabled bool) (bool, error) {
	if toggle.TopLevel.Pattern != "" {
		toggle.TopLevel.Enabled = enabled
		return manager.SetTopLevelResourceEnabled(toggle.TopLevel)
	}
	toggle.Package.Enabled = enabled
	return manager.SetPackageResourceEnabled(toggle.Package)
}

func packageResourceSettingID(resource PackageResourceToggleItem) string {
	return strings.Join([]string{resource.Scope, resource.Source, resource.ResourceType, resource.Pattern}, "\x1f")
}

func packageResourceLabel(resource PackageResourceToggleItem) string {
	label := strings.TrimSpace(resource.DisplayName)
	if label == "" {
		label = strings.TrimSpace(resource.Pattern)
	}
	return strings.TrimSpace(packageResourceTypeLabel(resource.ResourceType) + " " + label)
}

func packageResourceTypeLabel(resourceType string) string {
	switch resourceType {
	case "extensions":
		return "Extension"
	case "skills":
		return "Skill"
	case "prompts":
		return "Prompt"
	case "themes":
		return "Theme"
	default:
		return strings.TrimSpace(resourceType)
	}
}

func packageResourceDescription(resource PackageResourceToggleItem) string {
	parts := []string{
		"Source: " + packageResourceSourceLabel(resource),
		"Scope: " + firstNonEmptyString(resource.Scope, "user"),
		"Pattern: " + resource.Pattern,
	}
	if resource.Path != "" {
		parts = append(parts, "Path: "+resource.Path)
	}
	return strings.Join(parts, " · ")
}

func packageResourceSourceLabel(resource PackageResourceToggleItem) string {
	if resource.Metadata.Origin == "top-level" {
		if resource.Scope == "project" {
			return "Project .gi"
		}
		return "User agent"
	}
	return resource.Source
}

func (h *CLIInteractiveTUIHost) availableThemeNames(current string) []string {
	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, theme := range h.AvailableTUIThemes() {
		add(theme.Name)
	}
	add(current)
	return names
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

func (h *CLIInteractiveTUIHost) reloadKeybindings() {
	if h == nil {
		return
	}
	keybindings := DefaultProtocolKeybindings()
	if settings := h.settingsManager(); settings != nil && settings.agentDir != "" {
		keybindings = NewKeybindingsManager(settings.agentDir).GetEffectiveConfig()
	}
	h.keybindings = keybindings
	if h.startupHeader != nil {
		h.startupHeader.SetKeybindings(keybindings)
	}
	if !h.tuiKeybindingsInstalled {
		h.previousTUIKeybindings = gitui.GetKeybindings()
		h.tuiKeybindingsInstalled = true
	}
	gitui.SetKeybindings(gitui.NewKeybindingsManager(tuiKeybindingsFromProtocol(keybindings)))
}

func (h *CLIInteractiveTUIHost) effectiveKeybindings() KeybindingsConfig {
	if h != nil && h.keybindings != nil {
		return h.keybindings
	}
	return DefaultProtocolKeybindings()
}

func tuiKeybindingsFromProtocol(keybindings KeybindingsConfig) gitui.KeybindingsConfig {
	result := gitui.KeybindingsConfig{}
	for action, value := range keybindings {
		if !strings.HasPrefix(action, "tui.") {
			continue
		}
		keys := keybindingValueKeys(value)
		if len(keys) > 0 {
			result[action] = keys
		}
	}
	return result
}

func matchesKeybindingAction(data string, keybindings KeybindingsConfig, action string) bool {
	if data == "" {
		return false
	}
	if keybindings == nil {
		keybindings = DefaultProtocolKeybindings()
	}
	for _, key := range keybindingValueKeys(keybindings[action]) {
		if strings.EqualFold(key, "shift+tab") && isShiftTabKey(data) {
			return true
		}
		if gitui.MatchesKey(data, key) {
			return true
		}
	}
	return false
}

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
		h.addStatus("Model: " + result.Model.Provider + "/" + result.Model.ID + " (thinking: " + result.ThinkingLevel + ")")
		h.maybeWarnAboutAnthropicSubscriptionAuth(result.Model)
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
	h.addStatus("Model: " + model.Provider + "/" + model.ID + " (thinking: " + host.Session.Agent.State.ThinkingLevel + ")")
	h.maybeWarnAboutAnthropicSubscriptionAuth(model)
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
	if len(allModels) == 0 {
		h.addStatus("No models available")
		return nil
	}
	enabledIDs := enabledModelIDsForScopedSelector(session, h.settingsManager(), allModels)
	selector := NewScopedModelsSelectorComponent(ScopedModelsSelectorConfig{
		AllModels:       allModels,
		EnabledModelIDs: enabledIDs,
		Keybindings:     h.effectiveKeybindings(),
	}, ScopedModelsSelectorCallbacks{})
	if h.ui == nil {
		return errors.New("interactive TUI is not ready")
	}
	var restore func()
	closeSelector := func() {
		if restore != nil {
			restore()
			restore = nil
			return
		}
		h.requestRender(false)
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
		if len(enabled) == 0 || len(enabled) == len(allModels) {
			settings.SetEnabledModels(nil)
		} else {
			settings.SetEnabledModels(enabled)
		}
		h.addStatus("Model selection saved to settings")
		h.requestRender(false)
	}
	selector.callbacks.OnCancel = closeSelector
	restore = h.showEditorReplacement(selector, selector)
	return nil
}

func enabledModelIDsForScopedSelector(session *AgentSession, settings *SettingsManager, allModels []llm.Model) []string {
	if session != nil && len(session.ScopedModels) > 0 {
		ids := make([]string, 0, len(session.ScopedModels))
		for _, scoped := range session.ScopedModels {
			ids = append(ids, scopedModelFullID(scoped.Model))
		}
		return ids
	}
	if settings == nil {
		return nil
	}
	patterns := settings.GetEnabledModels()
	if len(patterns) == 0 {
		return nil
	}
	scopedModels := ResolveModelScope(patterns, cliModelSliceRegistry(allModels))
	ids := make([]string, 0, len(scopedModels))
	for _, scoped := range scopedModels {
		ids = append(ids, scopedModelFullID(scoped.Model))
	}
	return ids
}

func setSessionScopedModelsFromEnabledIDs(session *AgentSession, allModels []llm.Model, enabled []string) {
	if session == nil {
		return
	}
	if len(enabled) > 0 && len(enabled) < len(allModels) {
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
	if len(allModels) == 0 {
		h.addStatus("No models available")
		return nil
	}
	var restore func()
	closeSelector := func() {
		if restore != nil {
			restore()
			restore = nil
			return
		}
		h.requestRender(false)
	}
	selector := NewInteractiveModelSelectorComponent(ModelSelectorConfig{
		CurrentModel:  host.Session.Agent.State.Model,
		AllModels:     allModels,
		ScopedModels:  scopedModelsFromRPC(host.ScopedModels),
		InitialSearch: search,
		Keybindings:   h.effectiveKeybindings(),
	}, ModelSelectorCallbacks{})
	selector.callbacks.OnSelect = func(model llm.Model) {
		closeSelector()
		if err := h.applyModelSelection(host, model.Provider, model.ID); err != nil {
			h.addStatus("Model selection failed: " + err.Error())
		}
	}
	selector.callbacks.OnCancel = func() {
		closeSelector()
	}
	restore = h.showEditorReplacement(selector, selector)
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
	h.addStatus("Model: " + model.Provider + "/" + model.ID + " (thinking: " + host.Session.Agent.State.ThinkingLevel + ")")
	h.maybeWarnAboutAnthropicSubscriptionAuth(model)
	return nil
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

func (h *CLIInteractiveTUIHost) handleNameSlashCommand(args string) error {
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	name := strings.TrimSpace(args)
	if name == "" {
		current := session.SessionManager.GetSessionName()
		if current == "" {
			h.addWarning("Usage: /name <name>")
		} else {
			h.addStatus("Session name: " + current)
		}
		return nil
	}
	if err := session.SetSessionName(name); err != nil {
		return err
	}
	h.updateTerminalTitle()
	h.addStatus("Session name set: " + name)
	return nil
}

func (h *CLIInteractiveTUIHost) handleSessionSlashCommand() error {
	host, err := h.newRPCSessionHost()
	if err != nil {
		return err
	}
	state := host.GetState()
	stats := host.GetSessionStats()
	h.chat.AddChild(gitui.NewSpacer(1))
	h.chat.AddChild(gitui.NewText(renderInteractiveSessionInfo(state, stats), 1, 0))
	h.requestRender(false)
	return nil
}

func renderInteractiveSessionInfo(state RPCSessionState, stats RPCSessionStats) string {
	label := func(text string) string {
		return tuiThemeDim(text + ":")
	}
	lines := []string{tuiThemeBold("Session Info"), ""}
	if state.SessionName != "" {
		lines = append(lines, label("Name")+" "+state.SessionName)
	}
	sessionFile := firstNonEmptyString(stats.SessionFile, state.SessionFile, "In-memory")
	lines = append(lines,
		label("File")+" "+sessionFile,
		label("ID")+" "+firstNonEmptyString(stats.SessionID, state.SessionID),
	)
	lines = append(lines,
		"",
		tuiThemeBold("Messages"),
		label("User")+" "+formatSessionInfoInt(stats.UserMessages),
		label("Assistant")+" "+formatSessionInfoInt(stats.AssistantMessages),
		label("Tool Calls")+" "+formatSessionInfoInt(stats.ToolCalls),
		label("Tool Results")+" "+formatSessionInfoInt(stats.ToolResults),
		label("Total")+" "+formatSessionInfoInt(stats.TotalMessages),
	)
	lines = append(lines,
		"",
		tuiThemeBold("Tokens"),
		label("Input")+" "+formatSessionInfoInt(stats.Tokens.Input),
		label("Output")+" "+formatSessionInfoInt(stats.Tokens.Output),
	)
	if stats.Tokens.CacheRead > 0 {
		lines = append(lines, label("Cache Read")+" "+formatSessionInfoInt(stats.Tokens.CacheRead))
	}
	if stats.Tokens.CacheWrite > 0 {
		lines = append(lines, label("Cache Write")+" "+formatSessionInfoInt(stats.Tokens.CacheWrite))
	}
	lines = append(lines, label("Total")+" "+formatSessionInfoInt(stats.Tokens.Total))
	if stats.Cost > 0 {
		lines = append(lines, "", tuiThemeBold("Cost"), label("Total")+" "+fmt.Sprintf("%.4f", stats.Cost))
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func formatSessionInfoInt(value int) string {
	text := strconv.Itoa(value)
	if len(text) <= 3 {
		return text
	}
	var builder strings.Builder
	prefix := len(text) % 3
	if prefix == 0 {
		prefix = 3
	}
	builder.WriteString(text[:prefix])
	for index := prefix; index < len(text); index += 3 {
		builder.WriteByte(',')
		builder.WriteString(text[index : index+3])
	}
	return builder.String()
}

func (h *CLIInteractiveTUIHost) handleLoginSlashCommand(args string) error {
	provider := strings.TrimSpace(args)
	authType := "api_key"
	if provider == "" && !h.exitAfterInitial {
		registry := h.modelRegistry()
		if registry != nil {
			selected, selectedAuthType, handled, err := h.selectLoginProvider(registry)
			if err != nil {
				return err
			}
			if !handled {
				return nil
			}
			provider = selected
			authType = selectedAuthType
		}
	}
	if provider != "" && !h.exitAfterInitial {
		return h.runInteractiveLogin(provider, authType)
	}
	message := providerLoginHelp()
	if provider != "" {
		message = formatNoAPIKeyFoundMessage(provider)
	} else if !h.exitAfterInitial {
		h.addStatus("No API key providers available. Configure ~/.gi/agent/models.json or provider environment variables.")
	}
	h.chat.AddChild(newCLIMarkdownWithOptions("**Login**\n\n"+message, gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}))
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) selectLoginProvider(registry *ModelRegistry) (providerID string, authType string, handled bool, err error) {
	for {
		selectedAuthType, cancelled, err := h.selectLoginAuthType()
		if err != nil {
			return "", "", false, err
		}
		if cancelled {
			return "", "", false, nil
		}
		providers := loginAuthSelectorProviders(registry, selectedAuthType)
		if len(providers) == 0 {
			if selectedAuthType == "oauth" {
				h.addStatus("No subscription providers available.")
			} else {
				h.addStatus("No API key providers available.")
			}
			return "", "", false, nil
		}
		selected, providerCancelled, err := h.selectAuthProvider("login", registry, providers)
		if err != nil {
			return "", "", false, err
		}
		if providerCancelled {
			continue
		}
		return selected, selectedAuthType, true, nil
	}
}

func (h *CLIInteractiveTUIHost) selectLoginAuthType() (string, bool, error) {
	if h == nil || h.ui == nil {
		return "", true, errors.New("interactive TUI is not ready")
	}
	const subscriptionLabel = "Use a subscription"
	const apiKeyLabel = "Use an API key"
	selector := NewExtensionSelectorComponent("Select authentication method:", []string{subscriptionLabel, apiKeyLabel})
	resultCh := make(chan TUIDialogResult, 1)
	var closeOnce sync.Once
	var restore func()
	finish := func(result TUIDialogResult) {
		closeOnce.Do(func() {
			if restore != nil {
				restore()
				restore = nil
			}
			resultCh <- result
		})
	}
	selector.OnSelect = func(option string) {
		value := "api_key"
		if option == subscriptionLabel {
			value = "oauth"
		}
		finish(TUIDialogResult{Action: "selected", Value: value})
	}
	selector.OnCancel = func() {
		finish(TUIDialogResult{Action: "cancelled"})
	}
	restore = h.showEditorReplacement(selector, selector)
	select {
	case result := <-resultCh:
		if result.Action != "selected" {
			return "", true, nil
		}
		return dialogStringValue(result.Value), false, nil
	case <-h.done:
		if restore != nil {
			restore()
		}
		return "", true, nil
	}
}

func (h *CLIInteractiveTUIHost) runInteractiveLogin(providerID, authType string) error {
	registry := h.modelRegistry()
	if registry == nil || registry.authStorage == nil {
		h.chat.AddChild(newCLIMarkdownWithOptions("**Login**\n\n"+formatNoAPIKeyFoundMessage(providerID), gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}))
		h.requestRender(false)
		return nil
	}
	providerName := registry.GetProviderDisplayName(providerID)
	if authType == "oauth" {
		return h.showOAuthLoginDialog(providerID, providerName)
	}
	if providerID == "amazon-bedrock" {
		h.addBedrockSetupInfo(providerID, providerName)
		return nil
	}
	apiKey, cancelled, err := h.promptForAPIKey(providerName)
	if err != nil {
		return err
	}
	if cancelled {
		return nil
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		h.addStatus("Failed to save API key for " + providerName + ": API key cannot be empty.")
		return nil
	}
	registry.authStorage.Set(providerID, AuthCredential{Type: "api_key", Key: apiKey})
	registry.Refresh()
	h.addStatus("Saved API key for " + providerName + ". Credentials saved to ~/.gi/agent/auth.json")
	return nil
}

func (h *CLIInteractiveTUIHost) showOAuthLoginDialog(providerID, providerName string) error {
	if h == nil || h.ui == nil {
		return errors.New("interactive TUI is not ready")
	}
	prompt, ok := oauthLoginPromptForProvider(providerID)
	if !ok {
		h.addStatus("Subscription login is not implemented yet for " + providerName)
		return nil
	}
	dialog := NewLoginDialogComponent("Login to "+providerName, "")
	resultCh := make(chan TUIDialogResult, 1)
	var closeOnce sync.Once
	var restore func()
	finish := func(result TUIDialogResult) {
		closeOnce.Do(func() {
			if restore != nil {
				restore()
				restore = nil
			}
			resultCh <- result
		})
	}
	dialog.OnSubmit = func(value string) {
		finish(TUIDialogResult{Action: "submitted", Value: value})
	}
	dialog.OnCancel = func() {
		finish(TUIDialogResult{Action: "cancelled"})
	}
	dialog.ShowAuth(prompt.URL, prompt.Instructions, prompt.ManualPrompt)
	restore = h.showEditorReplacement(dialog, dialog)
	select {
	case result := <-resultCh:
		if result.Action == "submitted" {
			h.addStatus("Subscription login token exchange is not implemented yet for " + providerName + ".")
		}
		return nil
	case <-h.done:
		if restore != nil {
			restore()
		}
		return nil
	}
}

func (h *CLIInteractiveTUIHost) promptForAPIKey(providerName string) (string, bool, error) {
	if h == nil || h.ui == nil {
		return "", true, errors.New("interactive TUI is not ready")
	}
	dialog := NewLoginDialogComponent("Login to "+providerName, "Enter API key:")
	resultCh := make(chan TUIDialogResult, 1)
	var closeOnce sync.Once
	var restore func()
	finish := func(result TUIDialogResult) {
		closeOnce.Do(func() {
			if restore != nil {
				restore()
				restore = nil
			}
			resultCh <- result
		})
	}
	dialog.OnSubmit = func(value string) {
		finish(TUIDialogResult{Action: "submitted", Value: value})
	}
	dialog.OnCancel = func() {
		finish(TUIDialogResult{Action: "cancelled"})
	}
	restore = h.showEditorReplacement(dialog, dialog)
	select {
	case result := <-resultCh:
		if result.Action != "submitted" {
			return "", true, nil
		}
		return dialogStringValue(result.Value), false, nil
	case <-h.done:
		if restore != nil {
			restore()
		}
		return "", true, nil
	}
}

func (h *CLIInteractiveTUIHost) addBedrockSetupInfo(providerID, providerName string) {
	title := "Amazon Bedrock setup"
	if providerName == "" {
		providerName = providerID
	}
	dialog := NewLoginDialogComponent(title, "")
	dialog.ShowInfo([]string{
		tuiThemeFG("text", "Amazon Bedrock uses AWS credentials instead of a single API key."),
		tuiThemeFG("text", "Configure an AWS profile, IAM keys, bearer token, or role-based credentials."),
		tuiThemeMuted("See:"),
		tuiThemeAccent("  " + giProvidersDocumentationPath(h.interactiveCWD())),
	})
	var restore func()
	dialog.OnCancel = func() {
		if restore != nil {
			restore()
			restore = nil
		}
	}
	restore = h.showEditorReplacement(dialog, dialog)
}

func (h *CLIInteractiveTUIHost) handleLogoutSlashCommand(args string) error {
	registry := h.modelRegistry()
	if registry == nil || registry.authStorage == nil {
		h.addStatus("No local auth storage is configured")
		return nil
	}
	provider := strings.TrimSpace(args)
	if provider != "" {
		if !registry.authStorage.Has(provider) {
			h.addStatus("No stored credential for " + provider + ". Environment variables and models.json config are unchanged.")
			return nil
		}
		registry.authStorage.Remove(provider)
		registry.Refresh()
		h.addStatus("Removed stored credential for " + provider + ". Environment variables and models.json config are unchanged.")
		return nil
	}
	providers := registry.authStorage.List()
	if len(providers) == 0 {
		h.addStatus("No stored credentials to remove. /logout only removes credentials saved by /login; environment variables and models.json config are unchanged.")
		return nil
	}
	if h.exitAfterInitial {
		h.addStatus("Usage: /logout <provider>. Stored providers: " + strings.Join(providers, ", "))
		return nil
	}
	selected, cancelled, err := h.selectAuthProvider("logout", registry, logoutAuthSelectorProviders(registry))
	if err != nil {
		return err
	}
	if cancelled {
		h.addStatus("Logout cancelled")
		return nil
	}
	provider = selected
	if provider == "" {
		return errors.New("invalid provider selection")
	}
	registry.authStorage.Remove(provider)
	registry.Refresh()
	h.addStatus("Removed stored credential for " + provider + ". Environment variables and models.json config are unchanged.")
	return nil
}

func (h *CLIInteractiveTUIHost) selectAuthProvider(mode string, registry *ModelRegistry, providers []AuthSelectorProvider) (string, bool, error) {
	if h == nil || h.ui == nil {
		return "", true, errors.New("interactive TUI is not ready")
	}
	if len(providers) == 0 {
		return "", true, nil
	}
	var authStorage *AuthStorage
	var resolver AuthStatusResolver
	if registry != nil {
		authStorage = registry.authStorage
		resolver = registry.GetProviderAuthStatus
	}
	selector := NewOAuthSelectorComponent(OAuthSelector{
		Mode:           mode,
		AuthStorage:    authStorage,
		Providers:      providers,
		StatusResolver: resolver,
	})
	resultCh := make(chan TUIDialogResult, 1)
	var closeOnce sync.Once
	var restore func()
	finish := func(result TUIDialogResult) {
		closeOnce.Do(func() {
			if restore != nil {
				restore()
				restore = nil
			}
			resultCh <- result
		})
	}
	selector.OnSelect = func(providerID string) {
		finish(TUIDialogResult{Action: "selected", Value: providerID})
	}
	selector.OnCancel = func() {
		finish(TUIDialogResult{Action: "cancelled"})
	}
	restore = h.showEditorReplacement(selector, selector)
	select {
	case result := <-resultCh:
		if result.Action != "selected" {
			return "", true, nil
		}
		return dialogStringValue(result.Value), false, nil
	case <-h.done:
		if restore != nil {
			restore()
		}
		return "", true, nil
	}
}

func (h *CLIInteractiveTUIHost) handleHotkeysSlashCommand() error {
	hotkeys := strings.TrimSpace(h.hotkeysMarkdown())
	h.chat.AddChild(gitui.NewSpacer(1))
	h.chat.AddChild(newCLIDynamicBorder())
	h.chat.AddChild(gitui.NewText(tuiThemeBoldAccent("Keyboard Shortcuts"), 1, 0))
	h.chat.AddChild(gitui.NewSpacer(1))
	h.chat.AddChild(newCLIMarkdownWithOptions(hotkeys, gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}))
	h.chat.AddChild(newCLIDynamicBorder())
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) hotkeysMarkdown() string {
	keybindings := h.effectiveKeybindings()
	tuiKeys := func(action string) string {
		return formatHotkeyKeys(gitui.GetKeybindings().GetKeys(action), true)
	}
	appKeys := func(action string) string {
		return formatHotkeyKeys(keybindingValueKeys(keybindings[action]), true)
	}
	lines := []string{
		"**Navigation**",
		"",
		"| Key | Action |",
		"|-----|--------|",
		"| " + hotkeyRef(tuiKeys("tui.editor.cursorUp")) + " / " + hotkeyRef(tuiKeys("tui.editor.cursorDown")) + " / " + hotkeyRef(tuiKeys("tui.editor.cursorLeft")) + " / " + hotkeyRef(tuiKeys("tui.editor.cursorRight")) + " | Move cursor / browse history (Up when empty) |",
		"| " + hotkeyRef(tuiKeys("tui.editor.cursorWordLeft")) + " / " + hotkeyRef(tuiKeys("tui.editor.cursorWordRight")) + " | Move by word |",
		"| " + hotkeyRef(tuiKeys("tui.editor.cursorLineStart")) + " | Start of line |",
		"| " + hotkeyRef(tuiKeys("tui.editor.cursorLineEnd")) + " | End of line |",
		"| " + hotkeyRef(tuiKeys("tui.editor.jumpForward")) + " | Jump forward to character |",
		"| " + hotkeyRef(tuiKeys("tui.editor.jumpBackward")) + " | Jump backward to character |",
		"| " + hotkeyRef(tuiKeys("tui.editor.pageUp")) + " / " + hotkeyRef(tuiKeys("tui.editor.pageDown")) + " | Scroll by page |",
		"",
		"**Editing**",
		"",
		"| Key | Action |",
		"|-----|--------|",
		"| " + hotkeyRef(tuiKeys("tui.input.submit")) + " | Send message |",
		"| " + hotkeyRef(tuiKeys("tui.input.newLine")) + " | New line" + windowsTerminalNewLineNote() + " |",
		"| " + hotkeyRef(tuiKeys("tui.editor.deleteWordBackward")) + " | Delete word backwards |",
		"| " + hotkeyRef(tuiKeys("tui.editor.deleteWordForward")) + " | Delete word forwards |",
		"| " + hotkeyRef(tuiKeys("tui.editor.deleteToLineStart")) + " | Delete to start of line |",
		"| " + hotkeyRef(tuiKeys("tui.editor.deleteToLineEnd")) + " | Delete to end of line |",
		"| " + hotkeyRef(tuiKeys("tui.editor.yank")) + " | Paste the most-recently-deleted text |",
		"| " + hotkeyRef(tuiKeys("tui.editor.yankPop")) + " | Cycle through the deleted text after pasting |",
		"| " + hotkeyRef(tuiKeys("tui.editor.undo")) + " | Undo |",
		"",
		"**Other**",
		"",
		"| Key | Action |",
		"|-----|--------|",
		"| " + hotkeyRef(tuiKeys("tui.input.tab")) + " | Path completion / accept autocomplete |",
		"| " + hotkeyRef(appKeys("app.interrupt")) + " | Cancel autocomplete / abort streaming |",
		"| " + hotkeyRef(appKeys("app.clear")) + " | Clear editor (first) / exit (second) |",
		"| " + hotkeyRef(appKeys("app.exit")) + " | Exit (when editor is empty) |",
		"| " + hotkeyRef(appKeys("app.suspend")) + " | Suspend to background |",
		"| " + hotkeyRef(appKeys("app.thinking.cycle")) + " | Cycle thinking level |",
		"| " + hotkeyRef(appKeys("app.model.cycleForward")) + " / " + hotkeyRef(appKeys("app.model.cycleBackward")) + " | Cycle models |",
		"| " + hotkeyRef(appKeys("app.model.select")) + " | Open model selector |",
		"| " + hotkeyRef(appKeys("app.tools.expand")) + " | Toggle tool output expansion |",
		"| " + hotkeyRef(appKeys("app.thinking.toggle")) + " | Toggle thinking block visibility |",
		"| " + hotkeyRef(appKeys("app.editor.external")) + " | Edit message in external editor |",
		"| " + hotkeyRef(appKeys("app.message.followUp")) + " | Queue follow-up message |",
		"| " + hotkeyRef(appKeys("app.message.dequeue")) + " | Restore queued messages |",
		"| " + hotkeyRef(appKeys("app.clipboard.pasteImage")) + " | Paste image from clipboard |",
		"| `/` | Slash commands |",
		"| `!` | Run bash command |",
		"| `!!` | Run bash command (excluded from context) |",
	}
	if runtime := h.protocolRuntime(); runtime != nil {
		shortcuts := runtime.Shortcuts(keybindings).Shortcuts
		if len(shortcuts) > 0 {
			lines = append(lines, "", "**Extensions**", "", "| Key | Action |", "|-----|--------|")
			keys := make([]string, 0, len(shortcuts))
			for key := range shortcuts {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				shortcut := shortcuts[key]
				description := firstNonEmptyString(shortcut.Description, shortcut.SourceInfo.Path, "Extension shortcut")
				lines = append(lines, "| "+hotkeyRef(formatHotkeyText(key, true))+" | "+markdownTableValue(description)+" |")
			}
		}
	}
	return strings.Join(lines, "\n")
}

func windowsTerminalNewLineNote() string {
	if runtime.GOOS == "windows" {
		return " (Ctrl+Enter on Windows Terminal)"
	}
	return ""
}

func hotkeyRef(display string) string {
	display = strings.TrimSpace(display)
	if display == "" {
		return markdownTableValue("")
	}
	return "`" + markdownTableValue(display) + "`"
}

func formatHotkeyKeys(keys []string, capitalize bool) string {
	if len(keys) == 0 {
		return ""
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if strings.TrimSpace(key) != "" {
			parts = append(parts, formatHotkeyText(key, capitalize))
		}
	}
	return strings.Join(parts, "/")
}

func formatHotkeyText(key string, capitalize bool) string {
	groups := strings.Split(key, "/")
	for groupIndex, group := range groups {
		parts := strings.Split(group, "+")
		for partIndex, part := range parts {
			part = strings.TrimSpace(part)
			if runtime.GOOS == "darwin" && strings.EqualFold(part, "alt") {
				part = "option"
			}
			if capitalize && part != "" {
				part = strings.ToUpper(part[:1]) + part[1:]
			}
			parts[partIndex] = part
		}
		groups[groupIndex] = strings.Join(parts, "+")
	}
	return strings.Join(groups, "/")
}

func (h *CLIInteractiveTUIHost) handleChangelogSlashCommand() error {
	changelog := h.loadChangelogMarkdown()
	if strings.TrimSpace(changelog) == "" {
		changelog = "No changelog entries found."
	}
	h.chat.AddChild(gitui.NewSpacer(1))
	h.chat.AddChild(newCLIDynamicBorder())
	h.chat.AddChild(gitui.NewText(tuiThemeBoldAccent("What's New"), 1, 0))
	h.chat.AddChild(gitui.NewSpacer(1))
	h.chat.AddChild(newCLIMarkdownWithOptions(changelog, gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}))
	h.chat.AddChild(newCLIDynamicBorder())
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) handleDebugCommand() error {
	if h == nil {
		return errors.New("interactive TUI host is not ready")
	}
	width := 80
	height := 24
	if h.terminal != nil {
		width = h.terminal.Columns()
		height = h.terminal.Rows()
	}
	lines := []string(nil)
	if h.layout != nil {
		lines = h.layout.RenderWithSize(width, height)
	} else if h.ui != nil {
		lines = h.ui.Render(width)
	}
	debugPath := h.debugLogPath()
	if err := os.MkdirAll(filepath.Dir(debugPath), 0o755); err != nil {
		return err
	}
	var builder strings.Builder
	builder.WriteString("Debug output at ")
	builder.WriteString(time.Now().UTC().Format(time.RFC3339Nano))
	builder.WriteString("\n")
	builder.WriteString(fmt.Sprintf("Terminal: %dx%d\n", width, height))
	builder.WriteString(fmt.Sprintf("Total lines: %d\n\n", len(lines)))
	builder.WriteString("=== All rendered lines with visible widths ===\n")
	for index, line := range lines {
		encoded, _ := json.Marshal(line)
		builder.WriteString(fmt.Sprintf("[%d] (w=%d) %s\n", index, gitui.VisibleWidth(line), string(encoded)))
	}
	builder.WriteString("\n=== Agent messages (JSONL) ===\n")
	if session := h.agentSession(); session != nil {
		for _, message := range session.Messages() {
			encoded, err := json.Marshal(message)
			if err != nil {
				continue
			}
			builder.Write(encoded)
			builder.WriteString("\n")
		}
	}
	if err := os.WriteFile(debugPath, []byte(builder.String()), 0o644); err != nil {
		return err
	}
	if h.chat != nil {
		h.chat.AddChild(gitui.NewSpacer(1))
		h.chat.AddChild(gitui.NewText(tuiThemeAccent("✓ Debug log written")+"\n"+tuiThemeMuted(debugPath), 1, 1))
		h.requestRender(false)
	}
	return nil
}

func (h *CLIInteractiveTUIHost) handleArminSaysHiCommand() error {
	h.chat.AddChild(gitui.NewSpacer(1))
	h.chat.AddChild(newCLIArminComponent())
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) handleDementedDelvesCommand() error {
	h.chat.AddChild(gitui.NewSpacer(1))
	h.chat.AddChild(newCLIEarendilAnnouncementComponent())
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) debugLogPath() string {
	if settings := h.settingsManager(); settings != nil && settings.agentDir != "" {
		return filepath.Join(settings.agentDir, firstNonEmptyString(h.packageName, DefaultCodingAgentPackageName)+"-debug.log")
	}
	cwd := "."
	if h != nil {
		cwd = firstNonEmptyString(h.interactiveCWD(), ".")
	}
	return filepath.Join(cwd, ConfigDirName, "agent", firstNonEmptyString(h.packageName, DefaultCodingAgentPackageName)+"-debug.log")
}

func (h *CLIInteractiveTUIHost) loadChangelogMarkdown() string {
	cwd := h.interactiveCWD()
	if strings.TrimSpace(cwd) == "" {
		return ""
	}
	for _, name := range []string{"CHANGELOG.md", "CHANGELOG"} {
		content, err := os.ReadFile(filepath.Join(cwd, name))
		if err == nil {
			return string(content)
		}
	}
	return ""
}

func (h *CLIInteractiveTUIHost) handleNewSlashCommand() error {
	if runtimeHost := h.agentSessionRuntimeHost(); runtimeHost != nil {
		result, err := runtimeHost.NewSession()
		if err != nil {
			return err
		}
		if result.Cancelled {
			h.addStatus("New session cancelled")
			return nil
		}
	} else {
		host, err := h.newRPCSessionHost()
		if err != nil {
			return err
		}
		if _, err := host.handleCommand(context.Background(), RPCCommand{Type: RPCCommandNewSession}); err != nil {
			return err
		}
	}
	h.resetChatState()
	h.addSuccessStatus("✓ New session started")
	return nil
}

func (h *CLIInteractiveTUIHost) addSuccessStatus(text string) *gitui.Text {
	if strings.TrimSpace(text) == "" || h.chat == nil {
		return nil
	}
	h.chat.AddChild(gitui.NewSpacer(1))
	status := gitui.NewText(tuiThemeAccent(text), 1, 1)
	h.chat.AddChild(status)
	h.lastStatusSpacer = nil
	h.lastStatusText = nil
	h.requestRender(false)
	return status
}

func (h *CLIInteractiveTUIHost) handleExportSlashCommand(path string) error {
	host, err := h.newRPCSessionHost()
	if err != nil {
		return err
	}
	var exported string
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(path)), ".jsonl") {
		exported, err = host.ExportJSONL(path)
	} else {
		exported, err = host.ExportHTML(path)
	}
	if err != nil {
		return fmt.Errorf("Failed to export session: %w", err)
	}
	h.addStatus("Session exported to: " + exported)
	return nil
}

func (h *CLIInteractiveTUIHost) handleShareSlashCommand() error {
	host, err := h.newRPCSessionHost()
	if err != nil {
		return err
	}
	file, err := os.CreateTemp("", "gi-session-*.html")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	defer os.Remove(tempPath)
	if _, err := host.ExportHTML(tempPath); err != nil {
		return fmt.Errorf("Failed to export session: %w", err)
	}
	createGist := h.shareCreateGist
	if createGist == nil {
		createGist = defaultCreateSecretGist
	}
	ctx := context.Background()
	restoreShareLoader := h.showShareLoader(&ctx)
	defer restoreShareLoader()
	gistURL, err := createGist(ctx, tempPath)
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return err
	}
	gistID, err := gistIDFromShareOutput(gistURL)
	if err != nil {
		return err
	}
	h.addStatus("Share URL: " + shareViewerURL(gistID) + "\nGist: " + strings.TrimSpace(gistURL))
	return nil
}

func (h *CLIInteractiveTUIHost) showShareLoader(ctx *context.Context) func() {
	if h == nil || h.exitAfterInitial || h.ui == nil || h.editorContainer == nil || ctx == nil {
		return func() {}
	}
	loader := gitui.NewCancellableLoader("Creating gist...", gitui.LoaderIndicatorOptions{TUI: h.ui})
	*ctx = loader.Context()
	var restoreOnce sync.Once
	restore := func() {
		restoreOnce.Do(func() {
			loader.Dispose()
			h.editorContainer.SetChildren([]gitui.Component{h.editor})
			if h.ui != nil && h.editor != nil {
				h.ui.SetFocus(h.editor)
			}
			h.requestRender(true)
		})
	}
	loader.OnAbort = func() {
		restore()
		h.addStatus("Share cancelled")
	}
	h.editorContainer.SetChildren([]gitui.Component{loader})
	h.ui.SetFocus(loader)
	h.requestRender(false)
	return restore
}

func (h *CLIInteractiveTUIHost) handleImportSlashCommand(text string) error {
	path, ok := GetPathCommandArgument(strings.TrimSpace(text), "/import")
	if !ok {
		h.addStatus("Usage: /import <path.jsonl>")
		return nil
	}
	if !h.exitAfterInitial {
		result, err := h.RunTUIDialog(TUIDialogRequest{
			Kind:    "confirm",
			Title:   "Import session",
			Message: "Replace current session with " + path + "?",
		})
		if err != nil {
			return err
		}
		if result.Action != "confirmed" {
			h.addStatus("Import cancelled")
			return nil
		}
	}
	return h.importSessionFromJSONL(path)
}

func (h *CLIInteractiveTUIHost) importSessionFromJSONL(path string, cwdOverride ...string) error {
	importer, ok := h.runtimeHost.(interactiveImportRuntimeHost)
	if !ok {
		return errors.New("interactive TUI import requires an import-capable runtime host")
	}
	result, err := importer.ImportFromJsonl(path, cwdOverride...)
	if err == nil {
		if result.Cancelled {
			h.addStatus("Import cancelled")
			return nil
		}
		h.resetChatState()
		h.renderExistingMessages()
		h.addStatus("Session imported from: " + path)
		return nil
	}
	var missingCwd MissingSessionCwdError
	if errors.As(err, &missingCwd) && !h.exitAfterInitial {
		result, dialogErr := h.RunTUIDialog(TUIDialogRequest{
			Kind:         "input",
			Title:        "Session CWD missing",
			Message:      "Current session cwd does not exist: " + missingCwd.Issue.SessionCwd,
			DefaultValue: missingCwd.Issue.FallbackCwd,
		})
		if dialogErr != nil {
			return dialogErr
		}
		if result.Action != "submitted" || strings.TrimSpace(dialogStringValue(result.Value)) == "" {
			h.addStatus("Import cancelled")
			return nil
		}
		return h.importSessionFromJSONL(path, dialogStringValue(result.Value))
	}
	var notFound SessionImportFileNotFoundError
	if errors.As(err, &notFound) {
		h.addStatus("Failed to import session: " + notFound.Error())
		return nil
	}
	return err
}

func (h *CLIInteractiveTUIHost) handleResumeSlashCommand(text string) error {
	path, ok := GetPathCommandArgument(strings.TrimSpace(text), "/resume")
	if !ok {
		return h.handleResumeSessionSelector()
	}
	return h.resumeSessionPath(path)
}

func (h *CLIInteractiveTUIHost) handleResumeSessionSelector() error {
	if h == nil {
		return errors.New("interactive TUI host is not ready")
	}
	if h.exitAfterInitial {
		h.addStatus("Usage: /resume <path.jsonl>")
		return nil
	}
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	if session == nil || session.SessionManager == nil {
		return errors.New("resume requires an active session")
	}
	manager := session.SessionManager
	if h.ui == nil {
		if len(ListSessions(manager.GetCWD(), manager.GetSessionDir())) == 0 {
			h.addStatus("No sessions to resume")
			return nil
		}
		options := sessionResumeDialogOptions(manager)
		result, err := h.RunTUIDialog(TUIDialogRequest{Kind: "select", Title: "Resume Session", Options: options})
		if err != nil {
			return err
		}
		if result.Action != "selected" {
			h.addStatus("Resume cancelled")
			return nil
		}
		return h.resumeSessionPath(dialogStringValue(result.Value))
	}

	resultCh := make(chan TUIDialogResult, 1)
	var closeOnce sync.Once
	var restore func()
	finish := func(result TUIDialogResult) {
		closeOnce.Do(func() {
			if restore != nil {
				restore()
				restore = nil
			}
			resultCh <- result
		})
	}
	selector := NewLoadingSessionSelectorComponent(
		func(progress SessionListProgress) ([]SessionInfo, error) {
			return ListSessions(manager.GetCWD(), manager.GetSessionDir(), progress), nil
		},
		func(progress SessionListProgress) ([]SessionInfo, error) {
			return ListAllSessions(progress), nil
		},
		SessionSelectorOptions{
			ShowRenameHint:     true,
			CurrentSessionPath: manager.GetSessionFile(),
			Keybindings:        h.effectiveKeybindings(),
			RequestRender:      func() { h.requestRender(false) },
			OnSelect: func(path string) {
				finish(TUIDialogResult{Action: "selected", Value: path})
			},
			OnCancel: func() {
				finish(TUIDialogResult{Action: "cancelled"})
			},
			OnError: func(message string) {
				h.addStatus(message)
			},
			RenameSession: func(path, name string) error {
				name = strings.TrimSpace(name)
				if name == "" {
					return nil
				}
				resumeManager, err := OpenSessionManager(path, manager.GetSessionDir())
				if err != nil {
					return err
				}
				resumeManager.AppendSessionInfo(name)
				return nil
			},
			DeleteSession: func(path string) error {
				if err := os.Remove(path); err != nil {
					h.addStatus("Delete session failed: " + err.Error())
					return err
				}
				h.requestRender(false)
				return nil
			},
		},
	)
	restore = h.showEditorReplacement(selector, selector)
	select {
	case result := <-resultCh:
		if result.Action != "selected" {
			return nil
		}
		path := dialogStringValue(result.Value)
		if strings.TrimSpace(path) == "" {
			return nil
		}
		return h.resumeSessionPath(path)
	case <-h.done:
		if restore != nil {
			restore()
		}
		return nil
	}
}

func sessionResumeDialogOptions(manager *SessionManager) []TUIDialogOption {
	if manager == nil {
		return nil
	}
	sessions := ListSessions(manager.GetCWD(), manager.GetSessionDir())
	options := make([]TUIDialogOption, 0, len(sessions))
	for _, session := range sessions {
		if strings.TrimSpace(session.Path) == "" {
			continue
		}
		options = append(options, TUIDialogOption{
			ID:          session.Path,
			Label:       sessionResumeDialogLabel(session),
			Description: sessionResumeDialogDescription(session),
			Value:       session.Path,
		})
	}
	return options
}

func sessionResumeDialogLabel(session SessionInfo) string {
	label := firstNonEmptyString(strings.TrimSpace(session.Name), strings.TrimSpace(session.FirstMessage), strings.TrimSpace(session.ID), filepath.Base(session.Path))
	if label == "" {
		label = "Session"
	}
	return truncateDialogLabel(label)
}

func sessionResumeDialogDescription(session SessionInfo) string {
	parts := []string{}
	if session.MessageCount > 0 {
		parts = append(parts, fmt.Sprintf("%d messages", session.MessageCount))
	}
	if session.CWD != "" {
		parts = append(parts, filepath.Base(session.CWD))
	}
	if len(parts) == 0 {
		return filepath.Base(session.Path)
	}
	return strings.Join(parts, " · ")
}

func (h *CLIInteractiveTUIHost) resumeSessionPath(path string) error {
	if runtimeHost := h.agentSessionRuntimeHost(); runtimeHost != nil {
		result, err := runtimeHost.SwitchSession(path)
		if err != nil {
			return err
		}
		if result.Cancelled {
			h.addStatus("Resume cancelled")
			return nil
		}
	} else {
		host, err := h.newRPCSessionHost()
		if err != nil {
			return err
		}
		if _, err := host.SwitchSession(path); err != nil {
			return err
		}
	}
	h.resetChatState()
	h.renderExistingMessages()
	h.addStatus("Session resumed from: " + path)
	return nil
}

func (h *CLIInteractiveTUIHost) handleCopySlashCommand() error {
	host, err := h.newRPCSessionHost()
	if err != nil {
		return err
	}
	text := host.GetLastAssistantText()
	if text == nil || strings.TrimSpace(*text) == "" {
		h.addStatus("Error: No agent messages to copy yet.")
		return nil
	}
	copyFn := h.clipboardCopy
	if copyFn == nil {
		copyFn = func(text string) error {
			return CopyToClipboard(text, ClipboardCopyOptions{})
		}
	}
	if err := copyFn(*text); err != nil {
		h.addStatus("Error: " + err.Error())
		return nil
	}
	h.addStatus("Copied last agent message to clipboard")
	return nil
}

func (h *CLIInteractiveTUIHost) handleCompactSlashCommand(args string) error {
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	messageCount := 0
	for _, entry := range session.SessionManager.GetEntries() {
		if entry.Type == "message" {
			messageCount++
		}
	}
	if messageCount < 2 {
		h.addWarning("Nothing to compact (no messages yet)")
		return nil
	}
	result, err := session.Compact(strings.TrimSpace(args))
	if err != nil {
		if isCompactionCancelledError(err) {
			return nil
		}
		return err
	}
	h.resetChatState()
	h.renderExistingMessages()
	h.addStatus("Compacted: " + fmt.Sprintf("%d", result.TokensBefore) + " tokens before")
	return nil
}

func (h *CLIInteractiveTUIHost) handleCloneSlashCommand() error {
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	leafID := session.SessionManager.GetLeafID()
	if leafID == nil || strings.TrimSpace(*leafID) == "" {
		return errors.New("Entry " + cloneEmptySessionEntryID(session.SessionManager) + " not found")
	}
	if runtimeHost := h.agentSessionRuntimeHost(); runtimeHost != nil {
		result, err := runtimeHost.Fork(*leafID, AgentSessionRuntimeForkOptions{Position: "at"})
		if err != nil {
			return err
		}
		if result.Cancelled {
			h.addStatus("Clone cancelled")
			return nil
		}
	} else {
		result, err := session.ForkAt(*leafID)
		if err != nil {
			return err
		}
		if result.Session == nil {
			return errors.New("clone did not produce a session")
		}
		if owner, ok := h.runtimeHost.(*agentSessionPrintModeHost); ok {
			owner.session = result.Session
		} else {
			return errors.New("clone requires a replaceable agent session host")
		}
	}
	h.resetChatState()
	h.renderExistingMessages()
	h.addStatus("Cloned to new session")
	return nil
}

func cloneEmptySessionEntryID(manager *SessionManager) string {
	if manager == nil {
		return "unknown"
	}
	id := strings.ReplaceAll(strings.TrimSpace(manager.GetSessionID()), "-", "")
	if id == "" {
		return "unknown"
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func (h *CLIInteractiveTUIHost) handleForkSlashCommand(args string) error {
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	entryID := strings.TrimSpace(args)
	if entryID == "" {
		if h.exitAfterInitial {
			h.addStatus("Usage: /fork <entry-id>")
			return nil
		}
		messages := session.GetUserMessagesForForking()
		if len(messages) == 0 {
			h.addStatus("No messages to fork from")
			return nil
		}
		selectedID, cancelled, err := h.selectForkUserMessage(messages)
		if err != nil {
			return err
		}
		if cancelled {
			h.addStatus("Fork cancelled")
			return nil
		}
		entryID = selectedID
	}
	var result AgentSessionForkResult
	if runtimeHost := h.agentSessionRuntimeHost(); runtimeHost != nil {
		result, err = runtimeHost.Fork(entryID, AgentSessionRuntimeForkOptions{Position: "before"})
	} else {
		result, err = session.Fork(entryID)
		if err == nil && result.Session != nil {
			if owner, ok := h.runtimeHost.(*agentSessionPrintModeHost); ok {
				owner.session = result.Session
			}
		}
	}
	if err != nil {
		return err
	}
	if result.Cancelled {
		h.addStatus("Fork cancelled")
		return nil
	}
	h.resetChatState()
	h.renderExistingMessages()
	if strings.TrimSpace(result.SelectedText) != "" && h.editor != nil {
		h.editor.SetText(result.SelectedText)
	}
	h.addStatus("Forked to new session")
	return nil
}

func (h *CLIInteractiveTUIHost) selectForkUserMessage(messages []AgentSessionForkMessage) (string, bool, error) {
	if h == nil || h.ui == nil {
		return "", true, errors.New("interactive TUI is not ready")
	}
	selector := NewUserMessageSelectorComponent(messages, "")
	resultCh := make(chan TUIDialogResult, 1)
	var closeOnce sync.Once
	var restore func()
	finish := func(result TUIDialogResult) {
		closeOnce.Do(func() {
			if restore != nil {
				restore()
				restore = nil
			}
			resultCh <- result
		})
	}
	selector.OnSelect = func(entryID string) {
		finish(TUIDialogResult{Action: "selected", Value: entryID})
	}
	selector.OnCancel = func() {
		finish(TUIDialogResult{Action: "cancelled"})
	}
	restore = h.showEditorReplacement(selector, selector)
	select {
	case result := <-resultCh:
		if result.Action != "selected" {
			return "", true, nil
		}
		return dialogStringValue(result.Value), false, nil
	case <-h.done:
		if restore != nil {
			restore()
		}
		return "", true, nil
	}
}

func (h *CLIInteractiveTUIHost) handleTreeSlashCommand(args string) error {
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	entryID := strings.TrimSpace(args)
	initialSelectedID := entryID
	for {
		selectedFromTree := false
		if entryID == "" {
			if h.exitAfterInitial {
				h.addStatus("Usage: /tree <entry-id>")
				return nil
			}
			selectedID, cancelled, err := h.selectTreeEntry(session, initialSelectedID)
			if err != nil {
				return err
			}
			if cancelled {
				return nil
			}
			entryID = selectedID
			selectedFromTree = true
		}
		if h.treeEntryIsCurrentLeaf(session, entryID) {
			h.addStatus("Already at this point")
			return nil
		}
		options, cancelled, err := h.selectTreeNavigationOptions()
		if err != nil {
			return err
		}
		if cancelled {
			if selectedFromTree && !h.exitAfterInitial {
				initialSelectedID = entryID
				entryID = ""
				continue
			}
			h.addStatus("Tree switch cancelled")
			return nil
		}
		if options.Summarize {
			h.addStatus("Summarizing branch... (Esc to cancel)")
		}
		result, err := session.NavigateTree(entryID, options)
		if err != nil {
			return err
		}
		if result.Aborted {
			h.addStatus("Branch summarization cancelled")
			if selectedFromTree && !h.exitAfterInitial {
				initialSelectedID = entryID
				entryID = ""
				continue
			}
			return nil
		}
		if result.Cancelled {
			h.addStatus("Navigation cancelled")
			return nil
		}
		h.resetChatState()
		h.renderExistingMessages()
		if strings.TrimSpace(result.EditorText) != "" && h.editor != nil {
			h.editor.SetText(result.EditorText)
		}
		h.addStatus("Navigated to selected point")
		return nil
	}
}

func (h *CLIInteractiveTUIHost) treeEntryIsCurrentLeaf(session *AgentSession, entryID string) bool {
	if session == nil || session.SessionManager == nil || strings.TrimSpace(entryID) == "" {
		return false
	}
	leafID := session.SessionManager.GetLeafID()
	return leafID != nil && *leafID == entryID
}

func (h *CLIInteractiveTUIHost) selectTreeNavigationOptions() (AgentSessionNavigateTreeOptions, bool, error) {
	if h == nil || h.exitAfterInitial {
		return AgentSessionNavigateTreeOptions{}, false, nil
	}
	if settings := h.settingsManager(); settings != nil && settings.GetBranchSummarySkipPrompt() {
		return AgentSessionNavigateTreeOptions{}, false, nil
	}
	for {
		result, err := h.RunTUIDialog(TUIDialogRequest{
			Kind:    "select",
			Title:   "Summarize branch?",
			Message: "Choose whether to summarize the branch before navigating.",
			Options: []TUIDialogOption{
				{ID: "none", Label: "No summary", Value: "none"},
				{ID: "summary", Label: "Summarize", Value: "summary"},
				{ID: "custom", Label: "Summarize with custom prompt", Value: "custom"},
			},
			DefaultValue: "none",
		})
		if err != nil {
			return AgentSessionNavigateTreeOptions{}, false, err
		}
		if result.Action != "selected" {
			return AgentSessionNavigateTreeOptions{}, true, nil
		}
		switch firstNonEmptyString(result.OptionID, dialogStringValue(result.Value)) {
		case "none":
			return AgentSessionNavigateTreeOptions{}, false, nil
		case "summary":
			return AgentSessionNavigateTreeOptions{Summarize: true}, false, nil
		case "custom":
			editorResult, err := h.RunTUIDialog(TUIDialogRequest{
				Kind:  "editor",
				Title: "Custom summarization instructions",
			})
			if err != nil {
				return AgentSessionNavigateTreeOptions{}, false, err
			}
			if editorResult.Action != "submitted" {
				continue
			}
			return AgentSessionNavigateTreeOptions{
				Summarize:          true,
				CustomInstructions: dialogStringValue(editorResult.Value),
			}, false, nil
		default:
			return AgentSessionNavigateTreeOptions{}, false, nil
		}
	}
}

func (h *CLIInteractiveTUIHost) selectTreeEntry(session *AgentSession, initialSelectedID ...string) (string, bool, error) {
	if h == nil || h.ui == nil {
		return "", true, errors.New("interactive TUI is not ready")
	}
	if session == nil || session.SessionManager == nil {
		return "", true, errors.New("interactive TUI session tree is not ready")
	}
	roots := session.SessionManager.GetTree()
	currentLeafID := ""
	if leafID := session.SessionManager.GetLeafID(); leafID != nil {
		currentLeafID = *leafID
	}
	selector := NewTreeSelectorComponent(roots, currentLeafID, TreeSelectorOptions{Keybindings: h.effectiveKeybindings()})
	if len(initialSelectedID) > 0 && strings.TrimSpace(initialSelectedID[0]) != "" {
		selector.selectedID = strings.TrimSpace(initialSelectedID[0])
		selector.rebuild()
	}
	selector.SetFilter(h.treeSelectorInitialFilter())
	if len(roots) > 0 && (selector.GetTreeList() == nil || selector.GetTreeList().GetSelectedNode() == nil) {
		h.addStatus("No tree entries to switch to")
		return "", true, nil
	}
	resultCh := make(chan TUIDialogResult, 1)
	var closeOnce sync.Once
	var restore func()
	finish := func(result TUIDialogResult) {
		closeOnce.Do(func() {
			if restore != nil {
				restore()
				restore = nil
			}
			resultCh <- result
		})
	}
	selector.OnSelect = func(entryID string) {
		finish(TUIDialogResult{Action: "selected", Value: entryID})
	}
	selector.OnCancel = func() {
		finish(TUIDialogResult{Action: "cancelled"})
	}
	restore = h.showEditorReplacement(selector, selector)
	select {
	case result := <-resultCh:
		if result.Action != "selected" {
			return "", true, nil
		}
		return dialogStringValue(result.Value), false, nil
	case <-h.done:
		if restore != nil {
			restore()
		}
		return "", true, nil
	}
}

func (h *CLIInteractiveTUIHost) treeSelectorInitialFilter() TreeSelectorFilter {
	settings := h.settingsManager()
	if settings == nil {
		return TreeSelectorDefaultFilter
	}
	switch settings.GetTreeFilterMode() {
	case "no-tools":
		return TreeSelectorNoToolsFilter
	case "user-only":
		return TreeSelectorUserFilter
	case "labeled-only":
		return TreeSelectorLabelFilter
	case "all":
		return TreeSelectorAllFilter
	default:
		return TreeSelectorDefaultFilter
	}
}

func treeDialogOptions(session *AgentSession) []TUIDialogOption {
	if session == nil || session.SessionManager == nil {
		return nil
	}
	var options []TUIDialogOption
	var walk func(nodes []*SessionTreeNode, depth int)
	walk = func(nodes []*SessionTreeNode, depth int) {
		for _, node := range nodes {
			if node == nil {
				continue
			}
			label := treeDialogLabel(node.Entry)
			if label != "" {
				options = append(options, TUIDialogOption{
					ID:          node.Entry.ID,
					Label:       strings.Repeat("  ", depth) + label,
					Description: node.Entry.ID,
					Value:       node.Entry.ID,
				})
			}
			walk(node.Children, depth+1)
		}
	}
	walk(session.SessionManager.GetTree(), 0)
	return options
}

func treeDialogLabel(entry FileEntry) string {
	switch entry.Type {
	case "message":
		text := strings.TrimSpace(sessionMessageText(entry.Message))
		role := string(sessionMessageRole(entry.Message))
		if text == "" {
			return ""
		}
		return role + ": " + truncateDialogLabel(text)
	case "custom_message":
		return "custom: " + firstNonEmptyString(entry.CustomType, entry.ID)
	case "branch_summary":
		return "summary: " + truncateDialogLabel(entry.Summary)
	default:
		return ""
	}
}

func truncateDialogLabel(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= 72 {
		return text
	}
	return string(runes[:69]) + "..."
}

func (h *CLIInteractiveTUIHost) showReloadingEditor() func() {
	if h == nil || h.editorContainer == nil || h.ui == nil {
		return func() {}
	}
	previousChildren := h.editorContainer.Children()
	previousFocus := h.ui.FocusedComponent()
	reloadBox := &cliReloadBoxComponent{message: "Reloading keybindings, extensions, skills, prompts, themes..."}
	h.editorContainer.SetChildren([]gitui.Component{reloadBox})
	h.ui.SetFocus(reloadBox)
	h.requestRender(true)
	return func() {
		if len(previousChildren) == 0 && h.editor != nil {
			previousChildren = []gitui.Component{h.editor}
		}
		h.editorContainer.SetChildren(previousChildren)
		if previousFocus != nil {
			h.ui.SetFocus(previousFocus)
		} else if h.editor != nil {
			h.ui.SetFocus(h.editor)
		}
		h.requestRender(false)
	}
}

func (h *CLIInteractiveTUIHost) handleReloadSlashCommand() error {
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	if session.IsStreaming() {
		h.addStatus("Warning: Wait for the current response to finish before reloading.")
		return nil
	}
	if session.IsCompacting() {
		h.addStatus("Warning: Wait for compaction to finish before reloading.")
		return nil
	}
	restoreReloadBox := h.showReloadingEditor()
	reloadCompleted := false
	defer func() {
		if !reloadCompleted {
			restoreReloadBox()
		}
	}()
	h.stopProtocolExtensionProcesses()
	var extensions ResourceExtensionsResult
	if loader, ok := session.ResourceLoader.(interface{ Reload() }); ok {
		loader.Reload()
	}
	if loader, ok := session.ResourceLoader.(agentSessionExtensionsResourceLoader); ok {
		extensions = loader.GetExtensions()
		if flagLoader, ok := session.ResourceLoader.(agentSessionExtensionFlagResourceLoader); ok {
			if provider, ok := h.runtimeHost.(interface{ ExtensionFlagValues() map[string]any }); ok {
				flagLoader.ApplyExtensionFlagValues(provider.ExtensionFlagValues(), len(extensions.ProcessExtensions) > 0)
				extensions = loader.GetExtensions()
			}
		}
		if extensions.Runtime != nil {
			if registry := h.modelRegistry(); registry != nil {
				extensions.Runtime.BindModelRegistry(registry)
			}
			h.bindProtocolRuntimeHosts(extensions.Runtime)
		}
		if host, ok := h.runtimeHost.(interface {
			SetProtocolExtensionProcessSpecs([]ProtocolPackageProcessExtension)
		}); ok {
			host.SetProtocolExtensionProcessSpecs(extensions.ProcessExtensions)
		}
	}
	if runtimeHost := h.agentSessionRuntimeHost(); runtimeHost != nil {
		if extensions.Runtime != nil {
			runtimeHost.SetExtensionRuntime(extensions.Runtime)
		}
		if err := runtimeHost.Reload(); err != nil {
			return err
		}
	} else if extensions.Runtime != nil {
		session.ExtensionRuntime = extensions.Runtime
		extensions.Runtime.BindSession(session)
		h.bindProtocolRuntimeHosts(extensions.Runtime)
		extensions.Runtime.ApplyToSession(session)
	} else if session.ExtensionRuntime != nil {
		session.ExtensionRuntime.BindSession(session)
		h.bindProtocolRuntimeHosts(session.ExtensionRuntime)
		session.ExtensionRuntime.ApplyToSession(session)
	}
	h.reloadKeybindings()
	h.applyReloadedInteractiveSettings()
	h.refreshEditorAutocompleteProvider()
	if err := h.startProtocolExtensionProcesses(context.Background(), "reload"); err != nil {
		return err
	}
	restoreReloadBox()
	reloadCompleted = true
	h.refreshViewTreeSlots()
	h.rerenderSessionMessages()
	h.showLoadedResourcesOnStartup()
	h.showModelRegistryErrorIfAny()
	h.addStatus("Reloaded keybindings, extensions, skills, prompts, themes")
	return nil
}

func (h *CLIInteractiveTUIHost) bindProtocolViewTreeHost() {
	if h == nil || h.viewTreeHost == nil {
		return
	}
	if provider, ok := h.runtimeHost.(protocolExtensionRuntimeProvider); ok {
		if runtime := provider.ProtocolExtensionRuntime(); runtime != nil {
			h.bindProtocolRuntimeHosts(runtime)
		}
	}
	if runtimeHost := h.agentSessionRuntimeHost(); runtimeHost != nil && runtimeHost.ExtensionRuntime != nil {
		h.bindProtocolRuntimeHosts(runtimeHost.ExtensionRuntime)
	}
}

func (h *CLIInteractiveTUIHost) bindProtocolRuntimeHosts(runtime *ProtocolExtensionRuntime) {
	if h == nil || runtime == nil {
		return
	}
	runtime.BindViewTreeHost(h.viewTreeHost)
	h.watchProtocolRuntimeErrors(runtime)
	if provider, ok := h.runtimeHost.(protocolExtensionProcessProvider); ok {
		runtime.BindHostActionHost(h.configureProtocolExtensionRPCSessionHost(provider.NewProtocolExtensionRPCSessionHost(h.viewTreeHost, h, h)))
	}
}

func (h *CLIInteractiveTUIHost) watchProtocolRuntimeErrors(runtime *ProtocolExtensionRuntime) {
	if h == nil || runtime == nil {
		return
	}
	if h.unwatchProtocolErrors == nil {
		h.unwatchProtocolErrors = map[*ProtocolExtensionRuntime]func(){}
	}
	if h.unwatchProtocolErrors[runtime] != nil {
		return
	}
	h.unwatchProtocolErrors[runtime] = runtime.OnError(func(event ProtocolExtensionError) {
		h.showExtensionError(event)
	})
}

func (h *CLIInteractiveTUIHost) configureProtocolExtensionRPCSessionHost(rpcHost *RPCSessionHost) *RPCSessionHost {
	if h == nil || rpcHost == nil {
		return rpcHost
	}
	rpcHost.TUITitle = h
	rpcHost.TUIWorking = h
	rpcHost.TUIThinkingLabel = h
	rpcHost.TUIStatus = h
	rpcHost.TUITheme = h
	rpcHost.TUIToolExpansion = h
	if rpcHost.ProcessExecutor == nil {
		rpcHost.ProcessExecutor = LocalHostProcessExecutor{}
	}
	return rpcHost
}

func (h *CLIInteractiveTUIHost) applyReloadedInteractiveSettings() {
	if h == nil {
		return
	}
	settings := h.settingsManager()
	if settings == nil {
		return
	}
	if h.editor != nil {
		h.editor.SetPaddingX(settings.GetEditorPaddingX())
		h.editor.SetAutocompleteMaxVisible(settings.GetAutocompleteMaxVisible())
	}
	if active, ok := h.activeEditorComponent(); ok && active != h.editor {
		if appearance, ok := active.(gitui.EditorAppearanceComponent); ok {
			appearance.SetPaddingX(settings.GetEditorPaddingX())
			appearance.SetAutocompleteMaxVisible(settings.GetAutocompleteMaxVisible())
		}
	}
	if h.ui != nil {
		h.ui.SetShowHardwareCursor(settings.GetShowHardwareCursor())
		h.ui.SetClearOnShrink(settings.GetClearOnShrink())
	}
	if err := h.applyCurrentTUITheme(); err == nil {
		h.updateEditorBorderColor()
	}
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
	if registry := h.modelRegistry(); registry != nil {
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

func (h *CLIInteractiveTUIHost) ReadEditorText() string {
	return h.activeEditorText()
}

func (h *CLIInteractiveTUIHost) SetEditorText(text string) {
	h.setActiveEditorText(text)
}

func (h *CLIInteractiveTUIHost) InsertEditorText(text string) {
	if h == nil {
		return
	}
	editor, ok := h.activeEditorComponent()
	if !ok {
		return
	}
	if inserter, ok := editor.(gitui.EditorTextInserter); ok {
		inserter.InsertTextAtCursor(text)
	} else {
		editor.SetText(editor.GetText() + text)
	}
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) PasteEditorText(text string) {
	if h == nil {
		return
	}
	editor, ok := h.activeEditorComponent()
	if !ok {
		return
	}
	if pasteEditor, ok := editor.(interface{ PasteToEditor(string) }); ok {
		pasteEditor.PasteToEditor(text)
	} else if inserter, ok := editor.(gitui.EditorTextInserter); ok {
		inserter.InsertTextAtCursor(text)
	} else {
		editor.SetText(editor.GetText() + text)
	}
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) EditorCursor() (line, col int, ok bool) {
	if h == nil {
		return 0, 0, false
	}
	editor, ok := h.activeEditorComponent()
	if !ok {
		return 0, 0, false
	}
	cursor, ok := editor.(interface{ GetCursor() (int, int) })
	if !ok {
		return 0, 0, false
	}
	line, col = cursor.GetCursor()
	return line, col, true
}

func (h *CLIInteractiveTUIHost) FocusEditor() error {
	if h == nil || h.ui == nil {
		return errors.New("interactive TUI editor is not ready")
	}
	component := h.activeEditorFocusComponent()
	if component == nil {
		return errors.New("interactive TUI editor is not ready")
	}
	h.ui.SetFocus(component)
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) EditorFocused() bool {
	if h == nil || h.ui == nil {
		return false
	}
	focused := h.ui.FocusedComponent()
	if focused == nil {
		return false
	}
	if h.customEditorActive {
		return h.editorContainerHasChild(focused)
	}
	return focused == h.editor
}

func (h *CLIInteractiveTUIHost) EditorCustomActive() bool {
	return h != nil && h.customEditorActive
}

func (h *CLIInteractiveTUIHost) activeEditorFocusComponent() gitui.Component {
	if h == nil {
		return nil
	}
	if h.customEditorActive && h.editorContainer != nil {
		children := h.editorContainer.Children()
		if len(children) > 0 {
			return children[0]
		}
		return nil
	}
	return h.editor
}

func (h *CLIInteractiveTUIHost) editorContainerHasChild(component gitui.Component) bool {
	if h == nil || h.editorContainer == nil || component == nil {
		return false
	}
	for _, child := range h.editorContainer.Children() {
		if child == component {
			return true
		}
	}
	return false
}

func (h *CLIInteractiveTUIHost) SubmitEditorText() error {
	if h == nil {
		return errors.New("interactive TUI editor is not ready")
	}
	editor, ok := h.activeEditorComponent()
	if !ok {
		return errors.New("interactive TUI editor is not ready")
	}
	editor.HandleInput("\r")
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) RunTUIDialog(request TUIDialogRequest) (TUIDialogResult, error) {
	if h == nil {
		return TUIDialogResult{}, errors.New("interactive TUI dialog host is not ready")
	}
	kind := firstNonEmptyString(strings.TrimSpace(request.Kind), "notify")
	switch kind {
	case "notify", "notification":
		text := firstNonEmptyString(request.Message, request.Title)
		if strings.TrimSpace(text) != "" {
			h.addStatus(formatTUIDialogNotification(text, request.Type))
		}
		return TUIDialogResult{Action: "acknowledged"}, nil
	case "confirm":
		options := []TUIDialogOption{
			{ID: "yes", Label: "Yes", Value: true},
			{ID: "no", Label: "No", Value: false},
		}
		return h.runExtensionOptionDialog(requestDialogTitle(request.Title, request.Message), options, dialogDefaultOptionIndex(options, request.DefaultValue), request.Timeout, func(option TUIDialogOption) TUIDialogResult {
			if option.ID == "yes" {
				return TUIDialogResult{Action: "confirmed", OptionID: "yes", Value: true}
			}
			return TUIDialogResult{Action: "declined", OptionID: "no", Value: false}
		})
	case "select":
		if len(request.Options) == 0 {
			return TUIDialogResult{}, errors.New("select dialog requires options")
		}
		return h.runExtensionOptionDialog(requestDialogTitle(request.Title, request.Message), request.Options, dialogDefaultOptionIndex(request.Options, request.DefaultValue), request.Timeout, func(option TUIDialogOption) TUIDialogResult {
			return TUIDialogResult{Action: "selected", OptionID: option.ID, Value: dialogOptionValue(option)}
		})
	case "input":
		var submitted TUIDialogResult
		component := newCLIInputDialog(request.Title, request.Message, request.Placeholder, dialogStringValue(request.DefaultValue), func(value string) {
			submitted = TUIDialogResult{Action: "submitted", Value: value}
		}, func() {})
		return h.runSelectionDialog(component, func() TUIDialogResult { return submitted }, request.Timeout)
	case "editor", "textarea":
		var submitted TUIDialogResult
		component := newCLIEditorDialog(h.ui, request.Title, request.Message, dialogStringValue(request.DefaultValue), func(value string) {
			submitted = TUIDialogResult{Action: "submitted", Value: value}
		}, func() {})
		return h.runSelectionDialog(component, func() TUIDialogResult { return submitted }, request.Timeout)
	default:
		return TUIDialogResult{}, errors.New("unsupported dialog kind: " + kind)
	}
}

func requestDialogTitle(title, message string) string {
	title = strings.TrimSpace(title)
	message = strings.TrimSpace(message)
	switch {
	case title == "":
		return message
	case message == "":
		return title
	default:
		return title + "\n" + message
	}
}

func (h *CLIInteractiveTUIHost) runExtensionOptionDialog(title string, options []TUIDialogOption, defaultIndex int, timeout int, resultFor func(TUIDialogOption) TUIDialogResult) (TUIDialogResult, error) {
	if h == nil || h.ui == nil {
		return TUIDialogResult{}, errors.New("interactive TUI is not ready")
	}
	if len(options) == 0 {
		return TUIDialogResult{}, errors.New("select dialog requires options")
	}
	labels := make([]string, 0, len(options))
	for idx, option := range options {
		labels = append(labels, firstNonEmptyString(option.Label, option.ID, strconv.Itoa(idx+1)))
	}
	selector := NewExtensionSelectorComponent(firstNonEmptyString(title, "Select"), labels)
	selector.selected = max(0, min(defaultIndex, len(options)-1))

	resultCh := make(chan TUIDialogResult, 1)
	var closeOnce sync.Once
	var restore func()
	finish := func(result TUIDialogResult) {
		closeOnce.Do(func() {
			if restore != nil {
				restore()
				restore = nil
			}
			resultCh <- result
		})
	}
	selector.OnSelect = func(_ string) {
		index := max(0, min(selector.selected, len(options)-1))
		if resultFor != nil {
			finish(resultFor(options[index]))
			return
		}
		option := options[index]
		finish(TUIDialogResult{Action: "selected", OptionID: option.ID, Value: dialogOptionValue(option)})
	}
	selector.OnCancel = func() {
		finish(TUIDialogResult{Action: "cancelled"})
	}

	stopTimeout := h.startExtensionSelectorTimeout(selector, timeout, finish)
	defer stopTimeout()
	restore = h.showEditorReplacement(selector, selector)
	select {
	case result := <-resultCh:
		return result, nil
	case <-h.done:
		if restore != nil {
			restore()
		}
		return TUIDialogResult{Action: "cancelled"}, nil
	}
}

func formatTUIDialogNotification(message, noticeType string) string {
	message = strings.TrimSpace(message)
	switch strings.ToLower(strings.TrimSpace(noticeType)) {
	case "error":
		if strings.HasPrefix(message, "Error:") {
			return message
		}
		return "Error: " + message
	case "warning", "warn":
		if strings.HasPrefix(message, "Warning:") {
			return message
		}
		return "Warning: " + message
	default:
		return message
	}
}

func (h *CLIInteractiveTUIHost) runConfirmDialog(component *cliTUIDialogComponent, timeout int) (TUIDialogResult, error) {
	var selected TUIDialogResult
	if component != nil && component.list != nil {
		component.list.OnSelect = func(item gitui.SelectItem) {
			switch item.Value {
			case "yes":
				selected = TUIDialogResult{Action: "confirmed", OptionID: "yes", Value: true}
			default:
				selected = TUIDialogResult{Action: "declined", OptionID: "no", Value: false}
			}
		}
	}
	return h.runSelectionDialog(component, func() TUIDialogResult { return selected }, timeout)
}

func (h *CLIInteractiveTUIHost) runSelectionDialog(component *cliTUIDialogComponent, selected func() TUIDialogResult, timeoutValues ...int) (TUIDialogResult, error) {
	if component == nil {
		return TUIDialogResult{}, errors.New("interactive TUI dialog component is not ready")
	}
	component.keybindings = h.effectiveKeybindings()
	component.onToggleToolsExpanded = h.toggleToolOutputExpansion
	var finish func(TUIDialogResult)
	originalCancel := component.onCancel
	component.onCancel = func() {
		if originalCancel != nil {
			originalCancel()
		}
		if finish != nil {
			finish(TUIDialogResult{Action: "cancelled"})
		}
	}
	if component.list != nil {
		onSelect := component.list.OnSelect
		component.list.OnSelect = func(item gitui.SelectItem) {
			if onSelect != nil {
				onSelect(item)
			}
			if finish != nil {
				result := selected()
				if result.Action == "" {
					result = TUIDialogResult{Action: "selected", Value: item.Value}
				}
				finish(result)
			}
		}
		component.list.OnCancel = component.onCancel
	}
	if component.input != nil {
		onSubmit := component.input.OnSubmit
		component.input.OnSubmit = func(value string) {
			if onSubmit != nil {
				onSubmit(value)
			}
			if finish != nil {
				result := selected()
				if result.Action == "" {
					result = TUIDialogResult{Action: "submitted", Value: value}
				}
				finish(result)
			}
		}
		component.input.OnEscape = component.onCancel
	}
	if component.editor != nil {
		onSubmit := component.editorSubmit
		component.editor.SetOnSubmit(func(value string) {
			if onSubmit != nil {
				onSubmit(value)
			}
			if finish != nil {
				result := selected()
				if result.Action == "" {
					result = TUIDialogResult{Action: "submitted", Value: value}
				}
				finish(result)
			}
		})
	}
	resultCh := make(chan TUIDialogResult, 1)
	var closeOnce sync.Once
	var restore func()
	finish = func(result TUIDialogResult) {
		closeOnce.Do(func() {
			if restore != nil {
				restore()
				restore = nil
			}
			resultCh <- result
		})
	}
	if h.ui == nil {
		return TUIDialogResult{}, errors.New("interactive TUI is not ready")
	}
	stopTimeout := h.startDialogTimeout(component, timeoutValues, finish)
	defer stopTimeout()
	restore = h.showEditorReplacement(component, component)
	select {
	case result := <-resultCh:
		return result, nil
	case <-h.done:
		if restore != nil {
			restore()
		}
		return TUIDialogResult{Action: "cancelled"}, nil
	}
}

func (h *CLIInteractiveTUIHost) startDialogTimeout(component *cliTUIDialogComponent, timeoutValues []int, finish func(TUIDialogResult)) func() {
	if h == nil || component == nil || finish == nil || len(timeoutValues) == 0 || timeoutValues[0] <= 0 {
		return func() {}
	}
	timeout := time.Duration(timeoutValues[0]) * time.Millisecond
	deadline := time.Now().Add(timeout)
	baseTitle := component.Title()
	done := make(chan struct{})
	var doneOnce sync.Once
	stop := func() {
		doneOnce.Do(func() {
			close(done)
			component.SetTitle(baseTitle)
		})
	}
	updateTitle := func() {
		remaining := int(math.Ceil(time.Until(deadline).Seconds()))
		if remaining < 0 {
			remaining = 0
		}
		component.SetTitle(fmt.Sprintf("%s (%ds)", baseTitle, remaining))
		h.requestRender(false)
	}
	updateTitle()
	timer := time.NewTimer(timeout)
	ticker := time.NewTicker(time.Second)
	go func() {
		defer timer.Stop()
		defer ticker.Stop()
		for {
			select {
			case <-timer.C:
				finish(TUIDialogResult{Action: "cancelled"})
				return
			case <-ticker.C:
				updateTitle()
			case <-done:
				return
			}
		}
	}()
	return stop
}

func (h *CLIInteractiveTUIHost) startExtensionSelectorTimeout(component *ExtensionSelectorComponent, timeout int, finish func(TUIDialogResult)) func() {
	if h == nil || component == nil || finish == nil || timeout <= 0 {
		return func() {}
	}
	timeoutDuration := time.Duration(timeout) * time.Millisecond
	deadline := time.Now().Add(timeoutDuration)
	baseTitle := component.Title()
	done := make(chan struct{})
	var doneOnce sync.Once
	stop := func() {
		doneOnce.Do(func() {
			close(done)
			component.SetTitle(baseTitle)
		})
	}
	updateTitle := func() {
		remaining := int(math.Ceil(time.Until(deadline).Seconds()))
		if remaining < 0 {
			remaining = 0
		}
		component.SetTitle(fmt.Sprintf("%s (%ds)", baseTitle, remaining))
		h.requestRender(false)
	}
	updateTitle()
	timer := time.NewTimer(timeoutDuration)
	ticker := time.NewTicker(time.Second)
	go func() {
		defer timer.Stop()
		defer ticker.Stop()
		for {
			select {
			case <-timer.C:
				stop()
				finish(TUIDialogResult{Action: "cancelled"})
				return
			case <-ticker.C:
				updateTitle()
			case <-done:
				return
			case <-h.done:
				return
			}
		}
	}()
	return stop
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
	if h.unwatchSession != nil {
		h.unwatchSession()
		h.unwatchSession = nil
	}
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
