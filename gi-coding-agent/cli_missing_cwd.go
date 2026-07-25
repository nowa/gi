package gicodingagent

import (
	"context"
	"strings"

	gitui "github.com/nowa/gi/gi-tui"
)

type cliMissingCwdPromptResult struct {
	CWD      string
	Selected bool
}

func resolveCLIInteractiveMissingSessionCWD(args Args, options CLIOptions) (Args, CLIInteractiveRuntimeHost, error) {
	return resolveCLIInteractiveMissingSessionCWDWithTerminal(args, options, gitui.NewProcessTerminal())
}

func resolveCLIInteractiveMissingSessionCWDWithTerminal(args Args, options CLIOptions, terminal gitui.Terminal) (Args, CLIInteractiveRuntimeHost, error) {
	if args.Fork != "" || args.NoSession {
		return args, nil, nil
	}
	startupCwd, agentDir, err := resolveCLICWDAndAgentDir(options)
	if err != nil {
		return args, nil, err
	}
	settingsManager := NewSettingsManager(startupCwd, agentDir)
	sessionResult, err := newCLIPrintModeSessionManager(args, startupCwd, agentDir, settingsManager)
	if err != nil {
		return args, nil, err
	}
	sessionManager := sessionResult.SessionManager
	issue := GetMissingSessionCwdIssue(sessionManager, startupCwd)
	if issue == nil {
		return args, nil, nil
	}
	result, err := runCLIMissingSessionCWDPromptWithSettings(
		*issue,
		settingsManager,
		terminal,
	)
	if err != nil {
		return args, nil, err
	}
	if !result.Selected || strings.TrimSpace(result.CWD) == "" {
		return args, cliNoopInteractiveHost{}, nil
	}
	args.SessionCwdOverride = result.CWD
	return args, nil, nil
}

func runCLIMissingSessionCWDPrompt(issue MissingSessionCwdIssue, terminal gitui.Terminal) (cliMissingCwdPromptResult, error) {
	return runCLIMissingSessionCWDPromptWithSettings(
		issue,
		NewInMemorySettingsManager(nil),
		terminal,
	)
}

func runCLIMissingSessionCWDPromptWithSettings(
	issue MissingSessionCwdIssue,
	settings *SettingsManager,
	terminal gitui.Terminal,
) (cliMissingCwdPromptResult, error) {
	cwd, selected, err := showStartupSelector(
		context.Background(),
		settings,
		"Session CWD missing\n"+formatMissingSessionCwdPrompt(issue),
		[]startupSelectorOption[string]{
			{label: "Continue", value: issue.FallbackCwd},
			{label: "Cancel"},
		},
		startupTUIOptions{terminal: terminal},
	)
	if err != nil || !selected || strings.TrimSpace(cwd) == "" {
		return cliMissingCwdPromptResult{}, err
	}
	return cliMissingCwdPromptResult{
		CWD:      cwd,
		Selected: true,
	}, nil
}
