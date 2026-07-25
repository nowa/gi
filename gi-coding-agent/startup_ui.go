package gicodingagent

import (
	"context"
	"errors"
	"os"
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

type firstTimeSetupOutcome struct {
	result    FirstTimeSetupResult
	submitted bool
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
	terminal := options.Terminal
	if terminal == nil {
		terminal = gitui.NewProcessTerminal()
	}
	timeout := options.DetectionTimeout
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}

	ui := gitui.NewTUI(terminal, settings.GetShowHardwareCursor())
	ui.SetClearOnShrink(settings.GetClearOnShrink())
	ui.Start()
	defer ui.Stop()

	detectTheme := options.DetectTheme
	if detectTheme == nil {
		detectTheme = func(
			ctx context.Context,
			ui *gitui.TUI,
			timeout time.Duration,
			environment map[string]string,
		) TerminalTheme {
			return DetectTerminalThemeForAuto(ctx, ui, timeout, environment)
		}
	}
	detectedTheme := detectTheme(ctx, ui, timeout, options.Environment)
	if detectedTheme != TerminalThemeLight {
		detectedTheme = TerminalThemeDark
	}
	if err := tuiSetActiveTheme(string(detectedTheme), nil); err != nil {
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
			if tuiSetActiveTheme(string(theme), nil) == nil {
				ui.RequestRender(false)
			}
		},
		OnSubmit: func(result FirstTimeSetupResult) {
			finish(firstTimeSetupOutcome{result: result, submitted: true})
		},
		OnCancel: func() {
			finish(firstTimeSetupOutcome{})
		},
	})
	ui.AddChild(component)
	ui.SetFocus(component)
	ui.RequestRender(false)

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
