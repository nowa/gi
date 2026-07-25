package gicodingagent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

const interactiveThemeQueryTimeout = 100 * time.Millisecond

type interactiveThemeSettings interface {
	GetThemeSetting() (string, bool)
	SetTheme(string)
	Flush() error
}

type interactiveThemeResult struct {
	Success bool
	Err     error
}

type interactiveThemeControllerOptions struct {
	UI               *gitui.TUI
	Settings         interactiveThemeSettings
	AvailableThemes  func() []TUIThemeInfo
	ShowError        func(string)
	OnChanged        func()
	Environment      map[string]string
	QueryTimeout     time.Duration
	DetectBackground func(
		context.Context,
		TerminalBackgroundThemeDetector,
		time.Duration,
		map[string]string,
	) TerminalThemeDetection
	DetectAuto func(
		context.Context,
		TerminalAutoThemeDetector,
		time.Duration,
		map[string]string,
	) TerminalTheme
}

// interactiveThemeController is the sole owner of interactive theme state.
// Settings and terminal reports enter as immutable snapshots; serialized
// transitions then update the process-wide palette and publish one UI change.
type interactiveThemeController struct {
	ui               *gitui.TUI
	settings         interactiveThemeSettings
	availableThemes  func() []TUIThemeInfo
	showError        func(string)
	onChanged        func()
	environment      map[string]string
	queryTimeout     time.Duration
	detectBackground func(
		context.Context,
		TerminalBackgroundThemeDetector,
		time.Duration,
		map[string]string,
	) TerminalThemeDetection
	detectAuto func(
		context.Context,
		TerminalAutoThemeDetector,
		time.Duration,
		map[string]string,
	) TerminalTheme

	transitionMu sync.Mutex
	stateMu      sync.RWMutex

	terminalTheme    TerminalTheme
	activeThemeName  string
	autoSyncEnabled  bool
	disposed         bool
	revision         uint64
	unsubscribe      func()
	lifecycleContext context.Context
	cancelLifecycle  context.CancelFunc
}

type interactiveThemeSettingsSnapshot struct {
	value   string
	present bool
}

type interactiveThemeSettingsResolution struct {
	themeName       string
	terminalTheme   TerminalTheme
	updateTerminal  bool
	autoSyncEnabled bool
	showError       bool
	persist         bool
}

type interactiveThemeCommitResult uint8

const (
	interactiveThemeCommitDone interactiveThemeCommitResult = iota
	interactiveThemeCommitStale
	interactiveThemeCommitRetry
)

func newInteractiveThemeController(options interactiveThemeControllerOptions) (*interactiveThemeController, error) {
	if options.UI == nil {
		return nil, errors.New("interactive theme controller requires a TUI")
	}
	if options.Settings == nil {
		return nil, errors.New("interactive theme controller requires settings")
	}
	if options.AvailableThemes == nil {
		options.AvailableThemes = func() []TUIThemeInfo { return nil }
	}
	if options.ShowError == nil {
		options.ShowError = func(string) {}
	}
	if options.OnChanged == nil {
		options.OnChanged = func() {}
	}
	if options.QueryTimeout <= 0 {
		options.QueryTimeout = interactiveThemeQueryTimeout
	}
	if options.DetectBackground == nil {
		options.DetectBackground = DetectTerminalBackgroundTheme
	}
	if options.DetectAuto == nil {
		options.DetectAuto = DetectTerminalThemeForAuto
	}

	controller := &interactiveThemeController{
		ui:               options.UI,
		settings:         options.Settings,
		availableThemes:  options.AvailableThemes,
		showError:        options.ShowError,
		onChanged:        options.OnChanged,
		environment:      cloneThemeEnvironment(options.Environment),
		queryTimeout:     options.QueryTimeout,
		detectBackground: options.DetectBackground,
		detectAuto:       options.DetectAuto,
	}
	controller.lifecycleContext, controller.cancelLifecycle = context.WithCancel(context.Background())
	controller.terminalTheme = DetectTerminalBackgroundFromEnv(controller.environment).Theme
	controller.initializeTheme()
	controller.unsubscribe = controller.ui.OnTerminalColorSchemeChange(controller.applyTerminalTheme)
	return controller, nil
}

func (c *interactiveThemeController) initializeTheme() {
	snapshot := c.settingsSnapshot()
	themeName := string(c.terminalTheme)
	if snapshot.present {
		if resolved, ok := ResolveThemeSetting(snapshot.value, c.terminalTheme); ok {
			themeName = resolved
		}
	}
	if err := tuiSetActiveTheme(themeName, c.availableThemes()); err != nil {
		_ = tuiSetActiveTheme("dark", nil)
		themeName = "dark"
	} else {
		themeName = tuiActiveThemeSnapshot().name
	}
	c.activeThemeName = themeName
}

// ApplyFromSettings detects terminal state without holding controller locks,
// then commits only if neither settings nor a newer transition superseded the
// snapshot. This keeps terminal query callbacks from deadlocking the owner.
func (c *interactiveThemeController) ApplyFromSettings(ctx context.Context) {
	if c == nil {
		return
	}
	ctx, cancel := c.operationContext(ctx)
	defer cancel()
	for attempts := 0; attempts < 2; attempts++ {
		revision, ok := c.beginRevision()
		if !ok {
			return
		}
		snapshot := c.settingsSnapshot()
		resolution := c.resolveSettings(ctx, snapshot)
		commit, result := c.commitSettings(revision, snapshot, resolution)
		switch commit {
		case interactiveThemeCommitRetry:
			continue
		case interactiveThemeCommitStale:
			return
		}
		if resolution.persist && result.Success {
			c.settings.SetTheme(resolution.themeName)
			if err := c.settings.Flush(); err != nil {
				c.showError("Failed to persist detected terminal theme: " + err.Error())
			}
		}
		return
	}
}

func (c *interactiveThemeController) SetThemeName(themeName string, showError bool) interactiveThemeResult {
	if c == nil {
		return interactiveThemeResult{Err: errors.New("interactive theme controller is not ready")}
	}
	revision, ok := c.beginRevision()
	if !ok {
		return interactiveThemeResult{Err: errors.New("interactive theme controller is disposed")}
	}
	result, committed := c.commitThemeName(revision, themeName, false, showError)
	if !committed {
		return interactiveThemeResult{Err: errors.New("interactive theme transition was superseded")}
	}
	return result
}

func (c *interactiveThemeController) SetThemeInstance(theme tuiThemePalette) interactiveThemeResult {
	if c == nil {
		return interactiveThemeResult{Err: errors.New("interactive theme controller is not ready")}
	}
	revision, ok := c.beginRevision()
	if !ok {
		return interactiveThemeResult{Err: errors.New("interactive theme controller is disposed")}
	}

	c.transitionMu.Lock()
	if !c.revisionIsCurrent(revision) {
		c.transitionMu.Unlock()
		return interactiveThemeResult{Err: errors.New("interactive theme transition was superseded")}
	}
	tuiSetActiveThemePalette(theme)
	c.stateMu.Lock()
	c.autoSyncEnabled = false
	c.activeThemeName = "<in-memory>"
	c.stateMu.Unlock()
	c.transitionMu.Unlock()

	c.syncAutoNotifications()
	c.notifyChanged()
	return interactiveThemeResult{Success: true}
}

// Preview changes only the global palette. The committed theme name and
// automatic-sync state remain untouched so ApplyFromSettings can restore the
// authoritative state when the selector closes.
func (c *interactiveThemeController) Preview(themeSettingOrName string) {
	if c == nil {
		return
	}
	revision, ok := c.beginRevision()
	if !ok {
		return
	}

	c.transitionMu.Lock()
	if !c.revisionIsCurrent(revision) {
		c.transitionMu.Unlock()
		return
	}
	c.stateMu.RLock()
	terminalTheme := c.terminalTheme
	activeThemeName := c.activeThemeName
	c.stateMu.RUnlock()
	themeName, resolved := ResolveThemeSetting(themeSettingOrName, terminalTheme)
	if !resolved {
		themeName = activeThemeName
	}
	if themeName == "" {
		c.transitionMu.Unlock()
		return
	}
	err := tuiSetActiveTheme(themeName, c.availableThemes())
	if err != nil {
		_ = tuiSetActiveTheme("dark", nil)
	}
	c.transitionMu.Unlock()

	if err == nil {
		c.ui.Invalidate()
		c.ui.RequestRender()
	}
}

func (c *interactiveThemeController) DisableAutoSync() {
	if c == nil {
		return
	}
	revision, ok := c.beginRevision()
	if !ok {
		return
	}
	c.transitionMu.Lock()
	if c.revisionIsCurrent(revision) {
		c.stateMu.Lock()
		c.autoSyncEnabled = false
		c.stateMu.Unlock()
	}
	c.transitionMu.Unlock()
	c.syncAutoNotifications()
}

func (c *interactiveThemeController) GetTerminalTheme() TerminalTheme {
	if c == nil {
		return TerminalThemeDark
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.terminalTheme
}

func (c *interactiveThemeController) ActiveThemeName() string {
	if c == nil {
		return ""
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.activeThemeName
}

func (c *interactiveThemeController) AutoSyncEnabled() bool {
	if c == nil {
		return false
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.autoSyncEnabled
}

func (c *interactiveThemeController) Dispose() {
	if c == nil {
		return
	}
	c.transitionMu.Lock()
	c.stateMu.Lock()
	if c.disposed {
		c.stateMu.Unlock()
		c.transitionMu.Unlock()
		return
	}
	c.disposed = true
	c.autoSyncEnabled = false
	c.revision++
	unsubscribe := c.unsubscribe
	c.unsubscribe = nil
	cancelLifecycle := c.cancelLifecycle
	c.cancelLifecycle = nil
	c.stateMu.Unlock()
	c.transitionMu.Unlock()

	if cancelLifecycle != nil {
		cancelLifecycle()
	}
	if unsubscribe != nil {
		unsubscribe()
	}
	c.syncAutoNotifications()
}

func (c *interactiveThemeController) resolveSettings(
	ctx context.Context,
	snapshot interactiveThemeSettingsSnapshot,
) interactiveThemeSettingsResolution {
	if automatic, ok := ParseAutoThemeSetting(snapshot.value); snapshot.present && ok {
		terminalTheme := c.detectAuto(ctx, c.ui, c.queryTimeout, c.environment)
		themeName := automatic.DarkTheme
		if terminalTheme == TerminalThemeLight {
			themeName = automatic.LightTheme
		}
		return interactiveThemeSettingsResolution{
			themeName:       themeName,
			terminalTheme:   terminalTheme,
			updateTerminal:  true,
			autoSyncEnabled: true,
			showError:       true,
		}
	}
	if snapshot.present {
		return interactiveThemeSettingsResolution{
			themeName: snapshot.value,
			showError: true,
		}
	}

	detection := c.detectBackground(ctx, c.ui, c.queryTimeout, c.environment)
	return interactiveThemeSettingsResolution{
		themeName:      string(detection.Theme),
		terminalTheme:  detection.Theme,
		updateTerminal: true,
		persist:        detection.Confidence == "high",
	}
}

func (c *interactiveThemeController) commitSettings(
	revision uint64,
	snapshot interactiveThemeSettingsSnapshot,
	resolution interactiveThemeSettingsResolution,
) (interactiveThemeCommitResult, interactiveThemeResult) {
	c.transitionMu.Lock()
	if !c.revisionIsCurrent(revision) {
		c.transitionMu.Unlock()
		return interactiveThemeCommitStale, interactiveThemeResult{}
	}
	if c.settingsSnapshot() != snapshot {
		c.transitionMu.Unlock()
		return interactiveThemeCommitRetry, interactiveThemeResult{}
	}

	if resolution.updateTerminal {
		c.stateMu.Lock()
		c.terminalTheme = resolution.terminalTheme
		c.autoSyncEnabled = resolution.autoSyncEnabled
		c.stateMu.Unlock()
	} else {
		c.stateMu.Lock()
		c.autoSyncEnabled = false
		c.stateMu.Unlock()
	}
	result := c.applyThemeNameLocked(resolution.themeName)
	c.transitionMu.Unlock()

	c.syncAutoNotifications()
	c.notifyChanged()
	c.reportThemeError(resolution.themeName, resolution.showError, result.Err)
	return interactiveThemeCommitDone, result
}

func (c *interactiveThemeController) commitThemeName(
	revision uint64,
	themeName string,
	autoSyncEnabled bool,
	showError bool,
) (interactiveThemeResult, bool) {
	c.transitionMu.Lock()
	if !c.revisionIsCurrent(revision) {
		c.transitionMu.Unlock()
		return interactiveThemeResult{}, false
	}
	c.stateMu.Lock()
	c.autoSyncEnabled = autoSyncEnabled
	c.stateMu.Unlock()
	result := c.applyThemeNameLocked(themeName)
	c.transitionMu.Unlock()

	c.syncAutoNotifications()
	c.notifyChanged()
	c.reportThemeError(themeName, showError, result.Err)
	return result, true
}

func (c *interactiveThemeController) applyThemeNameLocked(themeName string) interactiveThemeResult {
	err := tuiSetActiveTheme(themeName, c.availableThemes())
	activeThemeName := tuiActiveThemeSnapshot().name
	if err != nil {
		_ = tuiSetActiveTheme("dark", nil)
		activeThemeName = "dark"
	}
	c.stateMu.Lock()
	c.activeThemeName = activeThemeName
	c.stateMu.Unlock()
	return interactiveThemeResult{Success: err == nil, Err: err}
}

func (c *interactiveThemeController) notifyChanged() {
	c.ui.Invalidate()
	c.onChanged()
}

func (c *interactiveThemeController) reportThemeError(themeName string, show bool, err error) {
	if !show || err == nil {
		return
	}
	c.showError(fmt.Sprintf(
		"Failed to load theme %q: %v\nFell back to dark theme.",
		themeName,
		err,
	))
}

func (c *interactiveThemeController) syncAutoNotifications() {
	for {
		c.stateMu.RLock()
		enabled := c.autoSyncEnabled && !c.disposed
		c.stateMu.RUnlock()
		if err := c.ui.SetTerminalColorSchemeNotifications(enabled); err != nil {
			c.showError("Failed to update terminal theme notifications: " + err.Error())
		}
		c.stateMu.RLock()
		stillEnabled := c.autoSyncEnabled && !c.disposed
		c.stateMu.RUnlock()
		if stillEnabled == enabled {
			return
		}
	}
}

func (c *interactiveThemeController) applyTerminalTheme(scheme gitui.TerminalColorScheme) {
	terminalTheme, ok := terminalThemeFromColorScheme(scheme)
	if c == nil || !ok {
		return
	}

	c.transitionMu.Lock()
	c.stateMu.Lock()
	if c.disposed || !c.autoSyncEnabled {
		c.stateMu.Unlock()
		c.transitionMu.Unlock()
		return
	}
	c.revision++
	c.terminalTheme = terminalTheme
	c.stateMu.Unlock()

	snapshot := c.settingsSnapshot()
	automatic, automaticOK := ParseAutoThemeSetting(snapshot.value)
	if !snapshot.present || !automaticOK {
		c.stateMu.Lock()
		c.autoSyncEnabled = false
		c.stateMu.Unlock()
		c.transitionMu.Unlock()
		c.syncAutoNotifications()
		return
	}

	themeName := automatic.DarkTheme
	if terminalTheme == TerminalThemeLight {
		themeName = automatic.LightTheme
	}
	c.stateMu.RLock()
	alreadyActive := themeName == c.activeThemeName
	c.stateMu.RUnlock()
	if alreadyActive {
		c.transitionMu.Unlock()
		return
	}
	result := c.applyThemeNameLocked(themeName)
	c.transitionMu.Unlock()

	c.notifyChanged()
	c.reportThemeError(themeName, false, result.Err)
}

func (c *interactiveThemeController) beginRevision() (uint64, bool) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.disposed {
		return 0, false
	}
	c.revision++
	return c.revision, true
}

func (c *interactiveThemeController) revisionIsCurrent(revision uint64) bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return !c.disposed && c.revision == revision
}

func (c *interactiveThemeController) settingsSnapshot() interactiveThemeSettingsSnapshot {
	value, present := c.settings.GetThemeSetting()
	return interactiveThemeSettingsSnapshot{value: value, present: present}
}

func (c *interactiveThemeController) operationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	operationContext, cancel := context.WithCancel(ctx)
	c.stateMu.RLock()
	lifecycleContext := c.lifecycleContext
	disposed := c.disposed
	c.stateMu.RUnlock()
	if disposed || lifecycleContext == nil {
		cancel()
		return operationContext, func() {}
	}
	stopLifecycleHook := context.AfterFunc(lifecycleContext, cancel)
	return operationContext, func() {
		stopLifecycleHook()
		cancel()
	}
}

func terminalThemeFromColorScheme(scheme gitui.TerminalColorScheme) (TerminalTheme, bool) {
	switch scheme {
	case gitui.TerminalColorSchemeLight:
		return TerminalThemeLight, true
	case gitui.TerminalColorSchemeDark:
		return TerminalThemeDark, true
	default:
		return "", false
	}
}

func cloneThemeEnvironment(environment map[string]string) map[string]string {
	if environment == nil {
		return nil
	}
	cloned := make(map[string]string, len(environment))
	for key, value := range environment {
		cloned[key] = value
	}
	return cloned
}
