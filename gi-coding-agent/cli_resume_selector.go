package gicodingagent

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	gitui "github.com/nowa/gi/gi-tui"
)

type cliNoSessionSelectedHost struct {
	stdout io.Writer
}

func (h cliNoSessionSelectedHost) Run() error {
	_, _ = fmt.Fprintln(nonNilWriter(h.stdout), "No session selected")
	return nil
}

type cliNoopInteractiveHost struct{}

func (cliNoopInteractiveHost) Run() error { return nil }

type cliResumeSelectorResult struct {
	Path     string
	Selected bool
}

func resolveCLIInteractiveResume(args Args, options CLIOptions) (Args, CLIInteractiveRuntimeHost, error) {
	return resolveCLIInteractiveResumeWithTerminal(args, options, gitui.NewProcessTerminal())
}

func resolveCLIInteractiveResumeWithTerminal(args Args, options CLIOptions, terminal gitui.Terminal) (Args, CLIInteractiveRuntimeHost, error) {
	if !args.Resume {
		return args, nil, nil
	}
	cwd, agentDir, err := resolveCLICWDAndAgentDir(options)
	if err != nil {
		return args, nil, err
	}
	settingsManager := NewSettingsManager(cwd, agentDir)
	sessionDir := resolveCLISessionDir(args, cwd, agentDir, settingsManager)
	result, err := runCLIResumeSelector(cwd, sessionDir, terminal, DefaultProtocolKeybindings())
	if err != nil {
		return args, nil, err
	}
	if !result.Selected || strings.TrimSpace(result.Path) == "" {
		return args, cliNoSessionSelectedHost{stdout: options.Stdout}, nil
	}
	args.Resume = false
	args.Session = result.Path
	return args, nil, nil
}

func runCLIResumeSelector(cwd, sessionDir string, terminal gitui.Terminal, keybindings KeybindingsConfig) (cliResumeSelectorResult, error) {
	if terminal == nil {
		terminal = gitui.NewProcessTerminal()
	}
	if keybindings == nil {
		keybindings = DefaultProtocolKeybindings()
	}

	resultCh := make(chan cliResumeSelectorResult, 1)
	var finishOnce sync.Once
	finish := func(result cliResumeSelectorResult) {
		finishOnce.Do(func() {
			resultCh <- result
		})
	}

	var ui *gitui.TUI
	selector := NewLoadingSessionSelectorComponent(
		func(progress SessionListProgress) ([]SessionInfo, error) {
			return ListSessions(cwd, sessionDir, progress), nil
		},
		func(progress SessionListProgress) ([]SessionInfo, error) {
			return ListAllSessions(filepath.Dir(sessionDir), progress), nil
		},
		SessionSelectorOptions{
			ShowRenameHint: true,
			Keybindings:    keybindings,
			RequestRender: func() {
				if ui != nil {
					ui.RequestRender(false)
				}
			},
			OnSelect: func(path string) {
				finish(cliResumeSelectorResult{Path: path, Selected: true})
			},
			OnCancel: func() {
				finish(cliResumeSelectorResult{})
			},
		},
	)

	ui = gitui.NewTUI(terminal)
	ui.AddChild(selector)
	ui.SetFocus(selector)
	_ = terminal.ClearScreen()
	ui.Start()
	defer ui.Stop()
	return <-resultCh, nil
}
