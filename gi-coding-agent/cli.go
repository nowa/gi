package gicodingagent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type CLIOptions struct {
	Args                 []string
	Stdout               io.Writer
	Stderr               io.Writer
	CWD                  string
	AgentDir             string
	Startup              func(stderr io.Writer) error
	PrintModeHostFactory func(Args) (PrintModeRuntimeHost, error)
	PackageManager       *DefaultPackageManager
	PackageName          string
	Version              string
	InstallEnvironment   InstallEnvironment
	VersionCheck         VersionReleaseChecker
	VersionCheckOptions  VersionCheckOptions
}

func RunCLI(options CLIOptions) int {
	if code, handled := runPackageSubcommand(options); handled {
		return code
	}
	args := ParseArgs(options.Args)
	if options.Startup != nil {
		if err := options.Startup(nonNilWriter(options.Stderr)); err != nil {
			writeCLIError(options.Stderr, err.Error())
			return 1
		}
	}
	if args.Help {
		WriteCLIUsage(cliHelpWriter(args, options))
		return 0
	}
	if args.Version {
		_, _ = fmt.Fprintln(nonNilWriter(options.Stdout), "gi")
		return 0
	}
	if args.Print || args.Mode == ModeJSON {
		return runCLIPrintMode(args, options)
	}
	if args.Mode == ModeRPC {
		writeCLIError(options.Stderr, "RPC mode is not wired to the CLI entrypoint yet.")
		return 1
	}
	return 0
}

func WriteCLIUsage(writer io.Writer) {
	if writer == nil {
		return
	}
	_, _ = fmt.Fprintln(writer, "Usage: gi [options] [message]")
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "Options:")
	_, _ = fmt.Fprintln(writer, "  -p, --print        Run in non-interactive print mode")
	_, _ = fmt.Fprintln(writer, "      --mode <mode>  Output mode: text, json, or rpc")
	_, _ = fmt.Fprintln(writer, "  -h, --help         Show help")
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "Commands:")
	_, _ = fmt.Fprintln(writer, "  install <source> [-l]  Install a package or extension source")
	_, _ = fmt.Fprintln(writer, "  remove <source> [-l]   Remove a package or extension source")
	_, _ = fmt.Fprintln(writer, "  update [source]        Update packages or the current CLI")
}

func cliHelpWriter(args Args, options CLIOptions) io.Writer {
	if args.Mode == ModeJSON || args.Print {
		return nonNilWriter(options.Stderr)
	}
	return nonNilWriter(options.Stdout)
}

func writeCLIError(writer io.Writer, message string) {
	if writer == nil || message == "" {
		return
	}
	_, _ = fmt.Fprintln(writer, message)
}

func nonNilWriter(writer io.Writer) io.Writer {
	if writer != nil {
		return writer
	}
	return io.Discard
}

func runPackageSubcommand(options CLIOptions) (int, bool) {
	if len(options.Args) == 0 {
		return 0, false
	}
	command := options.Args[0]
	if command == "update" {
		return runUpdateSubcommand(options)
	}
	if command != "install" && command != "remove" {
		return 0, false
	}

	var project bool
	var source string
	for _, arg := range options.Args[1:] {
		switch {
		case arg == "--help" || arg == "-h":
			writePackageCommandUsage(nonNilWriter(options.Stdout), command)
			return 0, true
		case arg == "-l" || arg == "--local":
			project = true
		case strings.HasPrefix(arg, "-"):
			writer := nonNilWriter(options.Stderr)
			_, _ = fmt.Fprintf(writer, "Unknown option %s for %q.\n", arg, command)
			_, _ = fmt.Fprintf(writer, "Use \"gi --help\" or \"gi %s <source> [-l]\".\n", command)
			return 1, true
		case source == "":
			source = arg
		}
	}
	if source == "" {
		writer := nonNilWriter(options.Stderr)
		_, _ = fmt.Fprintf(writer, "Missing %s source.\n", command)
		writePackageCommandUsage(writer, command)
		return 1, true
	}

	manager := options.PackageManager
	if manager == nil {
		manager = defaultCLIPackageManager()
	}
	var err error
	if command == "install" {
		err = manager.Install(source, project)
	} else {
		err = manager.Remove(source, project)
	}
	if err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1, true
	}
	return 0, true
}

func runUpdateSubcommand(options CLIOptions) (int, bool) {
	var self bool
	var force bool
	var sources []string
	for _, arg := range options.Args[1:] {
		switch {
		case arg == "--help" || arg == "-h":
			writeUpdateCommandUsage(nonNilWriter(options.Stdout))
			return 0, true
		case arg == "--self":
			self = true
		case arg == "--force":
			force = true
		case strings.HasPrefix(arg, "-"):
			writer := nonNilWriter(options.Stderr)
			_, _ = fmt.Fprintf(writer, "Unknown option %s for %q.\n", arg, "update")
			_, _ = fmt.Fprintln(writer, `Use "gi --help" or "gi update [source]".`)
			return 1, true
		default:
			sources = append(sources, arg)
		}
	}

	manager := options.PackageManager
	if manager == nil {
		manager = defaultCLIPackageManager()
	}
	if self {
		environment := options.InstallEnvironment
		if isZeroInstallEnvironment(environment) {
			environment = DefaultInstallEnvironment()
		}
		result, err := manager.RunSelfUpdate(SelfUpdateOptions{
			PackageName:         options.PackageName,
			CurrentVersion:      options.Version,
			Force:               force,
			Environment:         environment,
			VersionCheck:        options.VersionCheck,
			VersionCheckOptions: options.VersionCheckOptions,
		})
		if err != nil {
			writeCLIError(options.Stderr, err.Error())
			return 1, true
		}
		if result.Updated {
			_, _ = fmt.Fprintf(nonNilWriter(options.Stdout), "Updated %s\n", result.PackageName)
		}
		return 0, true
	}
	if err := manager.Update(sources...); err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1, true
	}
	return 0, true
}

func writePackageCommandUsage(writer io.Writer, command string) {
	if writer == nil {
		return
	}
	_, _ = fmt.Fprintln(writer, "Usage:")
	_, _ = fmt.Fprintf(writer, "  gi %s <source> [-l]\n", command)
}

func writeUpdateCommandUsage(writer io.Writer) {
	if writer == nil {
		return
	}
	_, _ = fmt.Fprintln(writer, "Usage:")
	_, _ = fmt.Fprintln(writer, "  gi update [source]")
	_, _ = fmt.Fprintln(writer, "  gi update --self [--force]")
}

func isZeroInstallEnvironment(env InstallEnvironment) bool {
	return env.ExecPath == "" &&
		env.PackageDir == "" &&
		env.Platform == "" &&
		env.HomeDir == "" &&
		!env.BunBinary &&
		!env.BunRuntime &&
		env.CommandOutput == nil
}

func defaultCLIPackageManager() *DefaultPackageManager {
	cwd, err := os.Getwd()
	if err != nil || cwd == "" {
		cwd = "."
	}
	return NewDefaultPackageManager(PackageManagerOptions{CWD: cwd, AgentDir: defaultCLIAgentDir(cwd)})
}

func defaultCLIAgentDir(cwd string) string {
	agentDir := firstNonEmptyString(os.Getenv("GI_CODING_AGENT_DIR"), os.Getenv("PI_CODING_AGENT_DIR"))
	if agentDir != "" {
		return ExpandPath(agentDir)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ConfigDirName, "agent")
	}
	return filepath.Join(cwd, ConfigDirName, "agent")
}
