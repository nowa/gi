package gicodingagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

type DistributionMetadata struct {
	PackageName   string
	AppName       string
	ConfigDirName string
}

type FirstTimeSetupEligibility struct {
	OfficialDistribution  bool
	ExperimentalFeatures  bool
	DefaultAgentDirectory bool
	SettingsMissing       bool
}

func (e FirstTimeSetupEligibility) ShouldRun() bool {
	return e.OfficialDistribution &&
		e.ExperimentalFeatures &&
		e.DefaultAgentDirectory &&
		e.SettingsMissing
}

func IsOfficialDistribution(metadata DistributionMetadata) bool {
	return metadata.PackageName == DefaultCodingAgentPackageName &&
		metadata.AppName == DefaultCodingAgentAppName &&
		metadata.ConfigDirName == ConfigDirName
}

// ShouldRunFirstTimeSetup discovers only the four facts needed by the setup
// policy, then evaluates the immutable result. An optional path makes the
// function deterministic in callers and tests while retaining the default
// settings path used by the CLI.
func ShouldRunFirstTimeSetup(settingsPaths ...string) bool {
	settingsPath := GetSettingsPath()
	if len(settingsPaths) > 0 && strings.TrimSpace(settingsPaths[0]) != "" {
		settingsPath = settingsPaths[0]
	}
	return discoverFirstTimeSetupEligibility(
		settingsPath,
		DistributionMetadata{
			PackageName:   DefaultCodingAgentPackageName,
			AppName:       DefaultCodingAgentAppName,
			ConfigDirName: ConfigDirName,
		},
		hasConfiguredAgentDirectory(),
	).ShouldRun()
}

func discoverFirstTimeSetupEligibility(
	settingsPath string,
	distribution DistributionMetadata,
	agentDirectoryOverridden bool,
) FirstTimeSetupEligibility {
	return FirstTimeSetupEligibility{
		OfficialDistribution:  IsOfficialDistribution(distribution),
		ExperimentalFeatures:  AreExperimentalFeaturesEnabled(),
		DefaultAgentDirectory: !agentDirectoryOverridden,
		SettingsMissing:       !settingsPathOccupied(settingsPath),
	}
}

func settingsPathOccupied(path string) bool {
	if strings.TrimSpace(path) == "" {
		return true
	}
	_, err := os.Stat(path)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

func hasConfiguredAgentDirectory() bool {
	return os.Getenv(EnvCodingAgentDir) != "" || os.Getenv(LegacyEnvCodingAgentDir) != ""
}

type FirstTimeSetupRunnerOptions struct {
	Terminal         gitui.Terminal
	DetectionTimeout time.Duration
	Environment      map[string]string
	DetectTheme      func(context.Context, *gitui.TUI, time.Duration, map[string]string) TerminalTheme
}

type startupTUIOptions struct {
	terminal         gitui.Terminal
	detectionTimeout time.Duration
	environment      map[string]string
	detectTheme      func(context.Context, *gitui.TUI, time.Duration, map[string]string) TerminalTheme
}

type startupUISettings struct {
	cwd                string
	agentDir           string
	global             map[string]any
	themeSetting       string
	themeSettingExists bool
	showHardwareCursor bool
	clearOnShrink      bool
}

type startupTUIRuntime struct {
	ui              *gitui.TUI
	terminal        gitui.Terminal
	settings        startupUISettings
	availableThemes []TUIThemeInfo
	options         startupTUIOptions

	detectionCancel context.CancelFunc
	detectionDone   <-chan struct{}
	closeOnce       sync.Once
	closeErr        error
}

type startupSelectorOption[T any] struct {
	label string
	value T
}

type startupSelectorOutcome[T any] struct {
	value    T
	selected bool
}

type startupInputOutcome struct {
	value     string
	submitted bool
}

type firstTimeSetupOutcome struct {
	result    FirstTimeSetupResult
	submitted bool
}

func snapshotStartupUISettings(
	settings *SettingsManager,
) (startupUISettings, error) {
	if settings == nil {
		return startupUISettings{}, errors.New("startup UI requires settings")
	}
	settings.mu.RLock()
	defer settings.mu.RUnlock()

	cwd := strings.TrimSpace(settings.cwd)
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	agentDir := strings.TrimSpace(settings.agentDir)
	if agentDir == "" && !settings.inMemory {
		agentDir = GetAgentDir(cwd)
	}
	themeSetting, themeSettingExists := settings.merged["theme"].(string)
	return startupUISettings{
		cwd:                cwd,
		agentDir:           agentDir,
		global:             cloneSettingsMap(settings.global),
		themeSetting:       themeSetting,
		themeSettingExists: themeSettingExists,
		showHardwareCursor: settingsBool(settings.merged, "showHardwareCursor", false),
		clearOnShrink: settingsNestedBool(
			settings.merged,
			"terminal",
			"clearOnShrink",
			false,
		),
	}, nil
}

// loadStartupThemes resolves only global theme resources. Project settings and
// resources are deliberately excluded because startup prompts can run before
// the project-trust decision exists.
func loadStartupThemes(settings *SettingsManager) []TUIThemeInfo {
	state, err := snapshotStartupUISettings(settings)
	if err != nil {
		return nil
	}
	return loadStartupThemesFromSnapshot(state)
}

func loadStartupThemesFromSnapshot(
	state startupUISettings,
) []TUIThemeInfo {
	if state.agentDir == "" {
		return nil
	}
	globalSettings := NewInMemorySettingsManager(state.global)
	globalSettings.SetProjectTrusted(false)
	manager := NewDefaultPackageManager(PackageManagerOptions{
		CWD:             state.cwd,
		AgentDir:        state.agentDir,
		SettingsManager: globalSettings,
	})

	var paths []string
	if resources, err := manager.ResolveConfiguredProtocolPackageResources(); err == nil {
		for _, resource := range resources.Themes {
			if resource.Enabled {
				paths = append(paths, resource.Path)
			}
		}
	}

	filters := settingsStringSlice(state.global, "themes")
	explicitPaths := resolveProtocolPackageEntries(
		state.agentDir,
		filters,
		"themes",
	)
	autoPaths := collectProtocolPackageDir(
		filepath.Join(state.agentDir, "themes"),
		"themes",
	)
	localPaths := dedupeProtocolPackagePaths(
		append(explicitPaths, autoPaths...),
	)
	localResources := make([]ProtocolPackageResource, 0, len(localPaths))
	for _, path := range localPaths {
		localResources = append(
			localResources,
			protocolPackageResource(path, ProtocolSourceInfo{
				Source: "local",
				Scope:  "user",
				Origin: "top-level",
			}),
		)
	}
	for _, resource := range applyProtocolPackageFilters(
		state.agentDir,
		localResources,
		filters,
	) {
		if resource.Enabled {
			paths = append(paths, resource.Path)
		}
	}

	seen := make(map[string]struct{}, len(paths))
	themes := make([]TUIThemeInfo, 0, len(paths))
	for _, path := range dedupeProtocolPackagePaths(paths) {
		for _, theme := range loadThemeFile(path) {
			if _, exists := seen[theme.Name]; exists {
				continue
			}
			if _, err := tuiLoadThemePaletteFromPath(
				theme.Name,
				theme.SourcePath,
			); err != nil {
				continue
			}
			seen[theme.Name] = struct{}{}
			themes = append(themes, TUIThemeInfo{
				Name: theme.Name,
				Path: theme.SourcePath,
			})
		}
	}
	return themes
}

func newStartupTUIRuntime(
	settings *SettingsManager,
	options startupTUIOptions,
) (*startupTUIRuntime, error) {
	state, err := snapshotStartupUISettings(settings)
	if err != nil {
		return nil, err
	}
	if options.terminal == nil {
		options.terminal = gitui.NewProcessTerminal()
	}
	if options.detectionTimeout <= 0 {
		options.detectionTimeout = 100 * time.Millisecond
	}
	if options.detectTheme == nil {
		options.detectTheme = func(
			ctx context.Context,
			ui *gitui.TUI,
			timeout time.Duration,
			environment map[string]string,
		) TerminalTheme {
			return DetectTerminalThemeForAuto(
				ctx,
				ui,
				timeout,
				environment,
			)
		}
	}

	availableThemes := loadStartupThemesFromSnapshot(state)
	initialTerminalTheme := DetectTerminalBackgroundFromEnv(
		options.environment,
	).Theme
	initialTheme := string(initialTerminalTheme)
	if resolved, ok := ResolveThemeSetting(
		state.themeSetting,
		initialTerminalTheme,
	); ok && strings.TrimSpace(resolved) != "" {
		initialTheme = resolved
	}
	if err := tuiSetActiveTheme(initialTheme, availableThemes); err != nil {
		_ = tuiSetActiveTheme("dark", availableThemes)
	}

	ui := gitui.NewTUI(
		options.terminal,
		state.showHardwareCursor,
	)
	ui.SetClearOnShrink(state.clearOnShrink)
	return &startupTUIRuntime{
		ui:              ui,
		terminal:        options.terminal,
		settings:        state,
		availableThemes: availableThemes,
		options:         options,
	}, nil
}

func (r *startupTUIRuntime) startWithAutoTheme(
	ctx context.Context,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	r.ui.Start()
	if r.settings.themeSettingExists &&
		r.settings.themeSetting != "" {
		if _, automatic := ParseAutoThemeSetting(
			r.settings.themeSetting,
		); !automatic {
			return
		}
	}

	detectionCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	r.detectionCancel = cancel
	r.detectionDone = done
	go func() {
		defer close(done)
		detectedTheme := r.detectTerminalTheme(detectionCtx)
		select {
		case <-detectionCtx.Done():
			return
		default:
		}
		themeName := string(detectedTheme)
		if resolved, ok := ResolveThemeSetting(
			r.settings.themeSetting,
			detectedTheme,
		); ok && strings.TrimSpace(resolved) != "" {
			themeName = resolved
		}
		if tuiSetActiveTheme(
			themeName,
			r.availableThemes,
		) == nil {
			r.ui.Invalidate()
			r.ui.RequestRender(false)
		}
	}()
}

func (r *startupTUIRuntime) detectTerminalTheme(
	ctx context.Context,
) TerminalTheme {
	detected := r.options.detectTheme(
		ctx,
		r.ui,
		r.options.detectionTimeout,
		r.options.environment,
	)
	if detected == TerminalThemeLight {
		return detected
	}
	return TerminalThemeDark
}

func (r *startupTUIRuntime) close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		if r.detectionCancel != nil {
			r.detectionCancel()
		}
		if r.detectionDone != nil {
			<-r.detectionDone
		}
		r.closeErr = r.terminal.ClearScreen()
		r.ui.Stop()
	})
	return r.closeErr
}

func showStartupSelector[T any](
	ctx context.Context,
	settings *SettingsManager,
	title string,
	options []startupSelectorOption[T],
	runnerOptions startupTUIOptions,
) (
	value T,
	selected bool,
	err error,
) {
	runtime, err := newStartupTUIRuntime(settings, runnerOptions)
	if err != nil {
		return value, false, err
	}
	defer func() {
		if closeErr := runtime.close(); err == nil {
			err = closeErr
		}
	}()

	labels := make([]string, 0, len(options))
	indexByLabel := make(map[string]int, len(options))
	for index, option := range options {
		labels = append(labels, option.label)
		if _, exists := indexByLabel[option.label]; !exists {
			indexByLabel[option.label] = index
		}
	}
	outcomes := make(chan startupSelectorOutcome[T], 1)
	var finishOnce sync.Once
	finish := func(outcome startupSelectorOutcome[T]) {
		finishOnce.Do(func() {
			outcomes <- outcome
		})
	}
	selector := NewExtensionSelectorComponent(title, labels)
	selector.OnSelect = func(label string) {
		index, ok := indexByLabel[label]
		if !ok {
			finish(startupSelectorOutcome[T]{})
			return
		}
		finish(startupSelectorOutcome[T]{
			value:    options[index].value,
			selected: true,
		})
	}
	selector.OnCancel = func() {
		finish(startupSelectorOutcome[T]{})
	}
	runtime.ui.AddChild(selector)
	runtime.ui.SetFocus(selector)
	runtime.startWithAutoTheme(ctx)

	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return value, false, ctx.Err()
	case outcome := <-outcomes:
		return outcome.value, outcome.selected, nil
	}
}

func showStartupInput(
	ctx context.Context,
	settings *SettingsManager,
	title string,
	placeholder string,
	runnerOptions startupTUIOptions,
) (
	value string,
	submitted bool,
	err error,
) {
	runtime, err := newStartupTUIRuntime(settings, runnerOptions)
	if err != nil {
		return "", false, err
	}
	defer func() {
		if closeErr := runtime.close(); err == nil {
			err = closeErr
		}
	}()

	outcomes := make(chan startupInputOutcome, 1)
	var finishOnce sync.Once
	finish := func(outcome startupInputOutcome) {
		finishOnce.Do(func() {
			outcomes <- outcome
		})
	}
	input := newCLIInputDialog(
		title,
		"",
		placeholder,
		"",
		func(value string) {
			finish(startupInputOutcome{
				value:     value,
				submitted: true,
			})
		},
		func() {
			finish(startupInputOutcome{})
		},
	)
	runtime.ui.AddChild(input)
	runtime.ui.SetFocus(input)
	runtime.startWithAutoTheme(ctx)

	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return "", false, ctx.Err()
	case outcome := <-outcomes:
		return outcome.value, outcome.submitted, nil
	}
}

// ShowFirstTimeSetup owns the complete startup interaction lifecycle. The
// component emits one result, and only this coordinator mutates settings.
func ShowFirstTimeSetup(
	ctx context.Context,
	settings *SettingsManager,
	options FirstTimeSetupRunnerOptions,
) error {
	if settings == nil {
		return errors.New("first-time setup requires settings")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runtime, err := newStartupTUIRuntime(settings, startupTUIOptions{
		terminal:         options.Terminal,
		detectionTimeout: options.DetectionTimeout,
		environment:      options.Environment,
		detectTheme:      options.DetectTheme,
	})
	if err != nil {
		return err
	}
	runtime.ui.Start()
	defer runtime.close()

	detectedTheme := runtime.detectTerminalTheme(ctx)
	if err := tuiSetActiveTheme(
		string(detectedTheme),
		runtime.availableThemes,
	); err != nil {
		return err
	}

	outcomes := make(chan firstTimeSetupOutcome, 1)
	var finishOnce sync.Once
	finish := func(outcome firstTimeSetupOutcome) {
		finishOnce.Do(func() {
			outcomes <- outcome
		})
	}
	component := NewFirstTimeSetupComponent(FirstTimeSetupOptions{
		DetectedTheme: detectedTheme,
		OnThemePreview: func(theme TerminalTheme) {
			if tuiSetActiveTheme(
				string(theme),
				runtime.availableThemes,
			) == nil {
				runtime.ui.RequestRender(false)
			}
		},
		OnSubmit: func(result FirstTimeSetupResult) {
			finish(firstTimeSetupOutcome{result: result, submitted: true})
		},
		OnCancel: func() {
			finish(firstTimeSetupOutcome{})
		},
	})
	runtime.ui.AddChild(component)
	runtime.ui.SetFocus(component)
	runtime.ui.RequestRender(false)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case outcome := <-outcomes:
		if !outcome.submitted {
			return nil
		}
		return settings.ApplyFirstTimeSetup(outcome.result)
	}
}

func maybeRunCLIFirstTimeSetup(options CLIOptions) error {
	if !isCLIInteractiveStdin(options) {
		return nil
	}
	cwd, agentDir, err := resolveCLICWDAndAgentDir(options)
	if err != nil {
		return err
	}
	distribution := DistributionMetadata{
		PackageName:   firstNonEmptyString(options.PackageName, DefaultCodingAgentPackageName),
		AppName:       DefaultCodingAgentAppName,
		ConfigDirName: ConfigDirName,
	}
	eligibility := discoverFirstTimeSetupEligibility(
		GetSettingsPath(agentDir),
		distribution,
		strings.TrimSpace(options.AgentDir) != "" || hasConfiguredAgentDirectory(),
	)
	if !eligibility.ShouldRun() {
		return nil
	}
	settings := NewSettingsManagerWithOptions(cwd, agentDir, SettingsManagerOptions{ProjectTrusted: false})
	return ShowFirstTimeSetup(context.Background(), settings, FirstTimeSetupRunnerOptions{
		Terminal: options.FirstTimeSetupTerminal,
	})
}
