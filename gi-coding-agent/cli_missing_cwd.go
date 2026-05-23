package gicodingagent

import (
	"strings"
	"sync"

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
	sessionManager, err := newCLIPrintModeSessionManager(args, startupCwd, agentDir, settingsManager)
	if err != nil {
		return args, nil, err
	}
	issue := GetMissingSessionCwdIssue(sessionManager, startupCwd)
	if issue == nil {
		return args, nil, nil
	}
	result, err := runCLIMissingSessionCWDPrompt(*issue, terminal)
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
	if terminal == nil {
		terminal = gitui.NewProcessTerminal()
	}
	resultCh := make(chan cliMissingCwdPromptResult, 1)
	var finishOnce sync.Once
	finish := func(result cliMissingCwdPromptResult) {
		finishOnce.Do(func() {
			resultCh <- result
		})
	}

	component := newCLISelectDialog(
		"Session CWD missing",
		formatMissingSessionCwdPrompt(issue),
		[]TUIDialogOption{
			{ID: "continue", Label: "Continue", Description: issue.FallbackCwd, Value: issue.FallbackCwd},
			{ID: "cancel", Label: "Cancel", Value: ""},
		},
		0,
		func(option TUIDialogOption) {
			if option.ID != "continue" {
				finish(cliMissingCwdPromptResult{})
				return
			}
			finish(cliMissingCwdPromptResult{CWD: dialogStringValue(option.Value), Selected: true})
		},
		func() {
			finish(cliMissingCwdPromptResult{})
		},
	)

	ui := gitui.NewTUI(terminal)
	ui.AddChild(component)
	ui.SetFocus(component)
	_ = terminal.ClearScreen()
	ui.Start()
	defer ui.Stop()
	return <-resultCh, nil
}
