package gicodingagent

import (
	"context"
	"fmt"

	gitui "github.com/nowa/gi/gi-tui"
)

type cliProjectTrustPromptResult struct {
	Option   *ProjectTrustOption
	Selected bool
}

type projectTrustInputPrompt func(
	title string,
	placeholder string,
) (value string, submitted bool, err error)

// Project trust has one state owner and a one-way startup flow:
//
//	CLI override ───────────────┐
//	trust.json ─────────────────┤
//	global default ─────────────┼─> ResolveProjectTrusted
//	resource detection + UI ────┘            │
//	                                         v
//	                             SettingsManager.projectTrusted
//	                                  │                 │
//	                                  v                 v
//	                          DefaultResourceLoader  PackageManager
//
// ProjectTrustStore persists decisions but never becomes runtime state.
// Consumers read the resolved state through SettingsManager instead of
// independently re-evaluating trust.
func newCLIRuntimeSettingsManager(args Args, cwd, agentDir string, prompt ProjectTrustPrompt) (*SettingsManager, bool, error) {
	return newCLISettingsManager(args, cwd, agentDir, prompt, false)
}

func newCLICommandSettingsManager(
	args Args,
	cwd string,
	agentDir string,
	prompt ProjectTrustPrompt,
	useSavedProjectTrustOnly bool,
) (*SettingsManager, bool, error) {
	return newCLISettingsManager(args, cwd, agentDir, prompt, useSavedProjectTrustOnly)
}

func newCLISettingsManager(
	args Args,
	cwd string,
	agentDir string,
	prompt ProjectTrustPrompt,
	useSavedProjectTrustOnly bool,
) (*SettingsManager, bool, error) {
	settings := NewSettingsManagerWithOptions(cwd, agentDir, SettingsManagerOptions{ProjectTrusted: false})
	store := NewProjectTrustStore(agentDir)
	if useSavedProjectTrustOnly {
		trusted := false
		if args.ProjectTrustOverride != nil {
			trusted = *args.ProjectTrustOverride
		} else {
			decision, found, err := store.Get(cwd)
			if err != nil {
				return nil, false, err
			}
			trusted = found && decision
		}
		settings.SetProjectTrusted(trusted)
		return settings, trusted, nil
	}
	trusted, err := ResolveProjectTrusted(ResolveProjectTrustOptions{
		CWD:                 cwd,
		TrustStore:          store,
		TrustOverride:       args.ProjectTrustOverride,
		DefaultProjectTrust: settings.GetDefaultProjectTrust(),
		Prompt:              prompt,
	})
	if err != nil {
		return nil, false, err
	}
	settings.SetProjectTrusted(trusted)
	return settings, trusted, nil
}

func cliProjectTrustPrompt(options CLIOptions) ProjectTrustPrompt {
	if options.ProjectTrustPrompt != nil {
		return options.ProjectTrustPrompt
	}
	if isCLIInteractiveStdin(options) {
		return defaultCLIProjectTrustPromptWithAgentDir(options.AgentDir)
	}
	return nil
}

func defaultCLIProjectTrustPrompt(cwd string, options []ProjectTrustOption) (*ProjectTrustOption, error) {
	return defaultCLIProjectTrustPromptWithAgentDir("")(cwd, options)
}

func defaultCLIProjectTrustPromptWithAgentDir(
	agentDir string,
) ProjectTrustPrompt {
	return func(
		cwd string,
		options []ProjectTrustOption,
	) (*ProjectTrustOption, error) {
		settings := NewSettingsManagerWithOptions(
			cwd,
			firstNonEmptyString(agentDir, GetAgentDir(cwd)),
			SettingsManagerOptions{ProjectTrusted: false},
		)
		result, err := runCLIProjectTrustPromptWithSettings(
			cwd,
			options,
			settings,
			gitui.NewProcessTerminal(),
		)
		if err != nil || !result.Selected {
			return nil, err
		}
		return result.Option, nil
	}
}

func runCLIProjectTrustPrompt(
	cwd string,
	options []ProjectTrustOption,
	terminal gitui.Terminal,
) (cliProjectTrustPromptResult, error) {
	settings := NewSettingsManagerWithOptions(
		cwd,
		GetAgentDir(cwd),
		SettingsManagerOptions{ProjectTrusted: false},
	)
	return runCLIProjectTrustPromptWithSettings(
		cwd,
		options,
		settings,
		terminal,
	)
}

func runCLIProjectTrustPromptWithSettings(
	cwd string,
	options []ProjectTrustOption,
	settings *SettingsManager,
	terminal gitui.Terminal,
) (cliProjectTrustPromptResult, error) {
	selectorOptions := make(
		[]startupSelectorOption[int],
		0,
		len(options),
	)
	for index, option := range options {
		selectorOptions = append(
			selectorOptions,
			startupSelectorOption[int]{
				label: option.Label,
				value: index,
			},
		)
	}
	index, selected, err := showStartupSelector(
		context.Background(),
		settings,
		fmt.Sprintf(
			"Trust project folder?\n%s\n\nThis allows gi to load %s settings and resources, install missing project packages, and execute project extensions.",
			cwd,
			ConfigDirName,
		),
		selectorOptions,
		startupTUIOptions{terminal: terminal},
	)
	if err != nil {
		return cliProjectTrustPromptResult{}, err
	}
	if !selected || index < 0 || index >= len(options) {
		return cliProjectTrustPromptResult{}, nil
	}
	option := options[index]
	return cliProjectTrustPromptResult{
		Option:   &option,
		Selected: true,
	}, nil
}

func projectTrustStartupWarning(cwd string, trusted bool) string {
	if trusted || !HasTrustRequiringProjectResources(cwd) {
		return ""
	}
	return fmt.Sprintf("This project is not trusted. Project %s resources and packages are ignored. Use --approve to trust them for this run.", ConfigDirName)
}

func cliProtocolProjectTrustContext(
	args Args,
	cwd string,
	prompt ProjectTrustPrompt,
	inputPrompt projectTrustInputPrompt,
) ProtocolProjectTrustContext {
	mode := "print"
	switch {
	case args.Mode == ModeJSON:
		mode = "json"
	case args.Mode == ModeRPC:
		mode = "rpc"
	case !args.Print && prompt != nil:
		mode = "interactive"
	}
	context := ProtocolProjectTrustContext{
		CWD:   cwd,
		Mode:  mode,
		HasUI: mode == "interactive" && prompt != nil,
	}
	if !context.HasUI {
		return context
	}
	context.Select = func(_ string, labels []string) (string, error) {
		options := make([]ProjectTrustOption, 0, len(labels))
		for _, label := range labels {
			options = append(options, ProjectTrustOption{Label: label})
		}
		selected, err := prompt(cwd, options)
		if err != nil || selected == nil {
			return "", err
		}
		return selected.Label, nil
	}
	context.Confirm = func(message string) (bool, error) {
		selected, err := context.Select(message, []string{"Yes", "No"})
		return selected == "Yes", err
	}
	inputUnavailable := func() (string, error) {
		return "", ProtocolRuntimeError{
			Code:    "ui_unavailable",
			Message: "project trust text input is not available before TUI startup",
		}
	}
	context.InputWithPlaceholder = func(
		title string,
		placeholder string,
	) (string, error) {
		if inputPrompt == nil {
			return inputUnavailable()
		}
		value, submitted, err := inputPrompt(title, placeholder)
		if err != nil || !submitted {
			return "", err
		}
		return value, nil
	}
	context.Input = func(title string) (string, error) {
		return context.InputWithPlaceholder(title, "")
	}
	context.Notify = func(string) error {
		return nil
	}
	return context
}

func defaultProjectTrustInputPrompt(
	settings *SettingsManager,
) projectTrustInputPrompt {
	return func(
		title string,
		placeholder string,
	) (string, bool, error) {
		return showStartupInput(
			context.Background(),
			settings,
			title,
			placeholder,
			startupTUIOptions{},
		)
	}
}
