package gicodingagent

import (
	"errors"
	"io"
	"os"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type basicCLIInteractiveHost struct {
	runtimeHost PrintModeRuntimeHost
	initial     cliPrintModeInitialMessage
	messages    []string
	stdout      io.Writer
}

func newDefaultCLIInteractiveHost(args Args, options CLIOptions) (CLIInteractiveRuntimeHost, error) {
	interactiveTTY := isCLIInteractiveStdin(options)
	runtimeOptions := options
	if interactiveTTY && runtimeOptions.ProjectTrustPrompt == nil {
		runtimeOptions.ProjectTrustPrompt =
			defaultCLIProjectTrustPromptWithAgentDir(options.AgentDir)
	}
	promptArgs := args
	stdinContent, err := readCLIPipedStdin(options)
	if err != nil {
		return nil, err
	}
	initial, err := buildCLIPrintModeInitialMessage(&promptArgs, stdinContent, ProcessFileArgumentsOptions{CWD: options.CWD})
	if err != nil {
		return nil, err
	}
	if initial.message == "" && len(promptArgs.Messages) == 0 {
		if !interactiveTTY {
			return nil, errors.New("Interactive input is required until the full TUI host is wired. Pass a message, pipe stdin, or use -p for print mode.")
		}
	}
	if interactiveTTY {
		var resumeHost CLIInteractiveRuntimeHost
		args, resumeHost, err = resolveCLIInteractiveResume(args, options)
		if err != nil {
			return nil, err
		}
		if resumeHost != nil {
			return resumeHost, nil
		}
		var missingCwdHost CLIInteractiveRuntimeHost
		args, missingCwdHost, err = resolveCLIInteractiveMissingSessionCWD(args, options)
		if err != nil {
			return nil, err
		}
		if missingCwdHost != nil {
			return missingCwdHost, nil
		}
	}

	runtimeHost, err := newDefaultCLIPrintModeHost(args, runtimeOptions)
	if err != nil {
		return nil, err
	}
	if interactiveTTY {
		versionCheck := options.VersionCheck
		if versionCheck == nil && !args.Offline {
			versionCheck = GetLatestGiRelease
		}
		var packageUpdateCheck PackageUpdateChecker
		if !args.Offline {
			packageUpdateCheck = defaultPackageUpdateChecker(runtimeHost)
		}
		return NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
			RuntimeHost:         runtimeHost,
			InitialMessage:      initial.message,
			InitialImages:       initial.images,
			Messages:            promptArgs.Messages,
			Stdout:              options.Stdout,
			VerboseStartup:      args.Verbose,
			ClearScreenOnStart:  true,
			ShowFooter:          true,
			Version:             firstNonEmptyString(options.Version, DefaultCodingAgentVersion),
			PackageName:         firstNonEmptyString(options.PackageName, DefaultCodingAgentPackageName),
			InstallEnvironment:  options.InstallEnvironment,
			VersionCheck:        versionCheck,
			VersionCheckOptions: options.VersionCheckOptions,
			PackageUpdateCheck:  packageUpdateCheck,
			StartupWarnings:     options.StartupWarnings,
		})
	}
	return &basicCLIInteractiveHost{
		runtimeHost: runtimeHost,
		initial:     initial,
		messages:    append([]string(nil), promptArgs.Messages...),
		stdout:      nonNilWriter(options.Stdout),
	}, nil
}

func defaultPackageUpdateChecker(runtimeHost PrintModeRuntimeHost) PackageUpdateChecker {
	provider, ok := runtimeHost.(interface {
		AgentSession() *AgentSession
		SettingsManager() *SettingsManager
	})
	if !ok {
		return nil
	}
	session := provider.AgentSession()
	settings := provider.SettingsManager()
	if session == nil || session.SessionManager == nil || settings == nil {
		return nil
	}
	cwd := session.SessionManager.GetCWD()
	agentDir := settings.agentDir
	if cwd == "" || agentDir == "" {
		return nil
	}
	return func() ([]string, error) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings})
		updates, err := manager.CheckForAvailableUpdates()
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(updates))
		for _, update := range updates {
			if update.DisplayName != "" {
				names = append(names, update.DisplayName)
			}
		}
		return names, nil
	}
}

func isCLIInteractiveStdin(options CLIOptions) bool {
	if options.Stdin != nil {
		return false
	}
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (h *basicCLIInteractiveHost) Run() (runErr error) {
	if h == nil || h.runtimeHost == nil {
		return errors.New("interactive mode host is required")
	}
	defer func() {
		if err := h.runtimeHost.Dispose(); err != nil && runErr == nil {
			runErr = err
		}
	}()

	session := h.runtimeHost.PrintModeSession()
	if session == nil {
		return errors.New("interactive mode session is required")
	}
	if h.initial.message != "" {
		if err := session.Prompt(h.initial.message, PrintModePromptOptions{Images: h.initial.images}); err != nil {
			return err
		}
	} else {
		for _, message := range h.messages {
			if err := session.Prompt(message, PrintModePromptOptions{}); err != nil {
				return err
			}
		}
	}
	if err := session.WaitForIdle(); err != nil {
		return err
	}

	last := lastPrintModeAssistantMessage(session.Messages())
	if last.StopReason == llm.StopReasonError {
		if last.ErrorMessage != "" {
			return errors.New(last.ErrorMessage)
		}
		return errors.New("assistant response failed")
	}
	return writePrintModeOutput(h.stdout, "text", last)
}
