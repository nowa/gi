package gicodingagent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type CLIOptions struct {
	Args                   []string
	Stdout                 io.Writer
	Stderr                 io.Writer
	Stdin                  io.Reader
	CWD                    string
	AgentDir               string
	Startup                func(stderr io.Writer) error
	PrintModeHostFactory   func(Args) (PrintModeRuntimeHost, error)
	InteractiveHostFactory func(Args) (CLIInteractiveRuntimeHost, error)
	ConfigHostFactory      func(PackageResourceConfigOptions) (CLIConfigRuntimeHost, error)
	ModelRegistry          *ModelRegistry
	PackageManager         *DefaultPackageManager
	PackageName            string
	Version                string
	InstallEnvironment     InstallEnvironment
	VersionCheck           VersionReleaseChecker
	VersionCheckOptions    VersionCheckOptions
	StartupWarnings        []string
}

func RunCLI(options CLIOptions) int {
	timings := newStartupTimingsFromEnv()
	defer timings.Print(nonNilWriter(options.Stderr))
	if code, handled := runPackageSubcommand(options); handled {
		timings.Mark("package command")
		return code
	}
	args := ParseArgs(options.Args)
	timings.Mark("parse args")
	if reportCLIArgDiagnostics(args.Diagnostics, options.Stderr) {
		return 1
	}
	if options.Startup != nil {
		if err := options.Startup(nonNilWriter(options.Stderr)); err != nil {
			writeCLIError(options.Stderr, err.Error())
			return 1
		}
	}
	timings.Mark("startup hook")
	migrationResult, err := runCLIStartupMigrations(options)
	if err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	timings.Mark("startup migrations")
	options.StartupWarnings = append(options.StartupWarnings, migrationResult.StartupWarnings...)
	if args.Help {
		timings.Mark("dispatch help")
		WriteCLIUsage(cliHelpWriter(args, options), cliHelpExtensionFlags(args, options)...)
		return 0
	}
	if args.Version {
		timings.Mark("dispatch version")
		_, _ = fmt.Fprintln(nonNilWriter(options.Stdout), firstNonEmptyString(options.Version, DefaultCodingAgentVersion))
		return 0
	}
	if args.Export != "" {
		timings.Mark("dispatch export")
		return runCLIExport(args, options)
	}
	if args.ListModels != nil {
		timings.Mark("dispatch list-models")
		return runCLIListModels(args, options)
	}
	if args.Print || args.Mode == ModeJSON {
		timings.Mark("dispatch print")
		return runCLIPrintMode(args, options)
	}
	if args.Mode == ModeRPC {
		timings.Mark("dispatch rpc")
		return runCLIRPCMode(args, options)
	}
	timings.Mark("dispatch interactive")
	return runCLIInteractiveMode(args, options)
}

type CLIStartupMigrationResult struct {
	StartupWarnings []string
}

func runCLIStartupMigrations(options CLIOptions) (CLIStartupMigrationResult, error) {
	cwd, agentDir, err := resolveCLICWDAndAgentDir(options)
	if err != nil {
		return CLIStartupMigrationResult{}, err
	}
	result, err := RunMigrationsWithResult(agentDir, cwd)
	if err != nil {
		return CLIStartupMigrationResult{}, err
	}
	var warnings []string
	if len(result.MigratedAuthProviders) > 0 {
		warnings = append(warnings, "Migrated credentials to auth.json: "+strings.Join(result.MigratedAuthProviders, ", "))
	}
	warnings = append(warnings, result.DeprecationWarnings...)
	return CLIStartupMigrationResult{StartupWarnings: warnings}, nil
}

type CLIInteractiveRuntimeHost interface {
	Run() error
}

type CLIConfigRuntimeHost interface {
	Run() error
}

func runCLIInteractiveMode(args Args, options CLIOptions) int {
	factory := options.InteractiveHostFactory
	if factory == nil {
		factory = func(args Args) (CLIInteractiveRuntimeHost, error) {
			return newDefaultCLIInteractiveHost(args, options)
		}
	}
	host, err := factory(args)
	if err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	if host == nil {
		writeCLIError(options.Stderr, "Interactive mode host is nil.")
		return 1
	}
	if err := host.Run(); err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	return 0
}

func runCLIExport(args Args, options CLIOptions) int {
	cwd, _, err := resolveCLICWDAndAgentDir(options)
	if err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	inputPath := resolveCLIPath(cwd, args.Export)
	outputPath := ""
	if len(args.Messages) > 0 {
		outputPath = resolveCLIPath(cwd, args.Messages[0])
	} else {
		outputPath = filepath.Join(cwd, DefaultSessionExportHTMLName(inputPath))
	}
	result, err := ExportSessionFileToHTML(inputPath, outputPath)
	if err != nil {
		writeCLIError(options.Stderr, "Error: "+err.Error())
		return 1
	}
	_, _ = fmt.Fprintf(nonNilWriter(options.Stdout), "Exported to: %s\n", result)
	return 0
}

func WriteCLIUsage(writer io.Writer, extensionFlags ...ProtocolFlagRegistration) {
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
	_, _ = fmt.Fprintln(writer, "  uninstall <source> [-l]")
	_, _ = fmt.Fprintln(writer, "  config                 Open resource configuration")
	_, _ = fmt.Fprintln(writer, "  list                   List configured packages")
	_, _ = fmt.Fprintln(writer, "  update [source]        Update packages or the current CLI")
	if len(extensionFlags) > 0 {
		_, _ = fmt.Fprintln(writer, "")
		_, _ = fmt.Fprintln(writer, "Extension CLI Flags:")
		for _, flag := range extensionFlags {
			_, _ = fmt.Fprintln(writer, formatCLIExtensionFlagHelp(flag))
		}
	}
}

func cliHelpExtensionFlags(args Args, options CLIOptions) []ProtocolFlagRegistration {
	cwd, agentDir, err := resolveCLICWDAndAgentDir(options)
	if err != nil {
		return nil
	}
	settingsManager := NewSettingsManager(cwd, agentDir)
	loader := NewDefaultResourceLoader(defaultResourceLoaderOptionsFromCLI(args, cwd, agentDir, settingsManager))
	loader.Reload()
	extensions := loader.GetExtensions()
	if extensions.Runtime == nil {
		return nil
	}
	return extensions.Runtime.Flags()
}

func formatCLIExtensionFlagHelp(flag ProtocolFlagRegistration) string {
	name := strings.TrimSpace(flag.Name)
	if name == "" {
		return ""
	}
	value := ""
	if flag.Type == "string" {
		value = " <value>"
	}
	label := "  --" + name + value
	description := strings.TrimSpace(flag.Description)
	if description == "" {
		description = strings.TrimSpace(flag.SourceInfo.Path)
	}
	if description == "" {
		return label
	}
	if len(label) < 30 {
		label += strings.Repeat(" ", 30-len(label))
	} else {
		label += "  "
	}
	return label + description
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

func reportCLIArgDiagnostics(diagnostics []Diagnostic, writer io.Writer) bool {
	hasError := false
	for _, diagnostic := range diagnostics {
		switch diagnostic.Type {
		case "error":
			hasError = true
			writeCLIError(writer, "Error: "+diagnostic.Message)
		default:
			writeCLIError(writer, "Warning: "+diagnostic.Message)
		}
	}
	return hasError
}

func nonNilWriter(writer io.Writer) io.Writer {
	if writer != nil {
		return writer
	}
	return io.Discard
}

func nonNilReader(reader io.Reader) io.Reader {
	if reader != nil {
		return reader
	}
	return os.Stdin
}

func readCLIPipedStdin(options CLIOptions) (*string, error) {
	var reader io.Reader
	if options.Stdin != nil {
		reader = options.Stdin
	} else {
		info, err := os.Stdin.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice != 0 {
			return nil, nil
		}
		reader = os.Stdin
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil, nil
	}
	return &text, nil
}

func runCLIRPCMode(args Args, options CLIOptions) int {
	if len(args.FileArgs) > 0 {
		writeCLIError(options.Stderr, "Error: @file arguments are not supported in RPC mode")
		return 1
	}
	host, err := newDefaultCLIRPCSessionHost(args, options)
	if err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	processor := RPCLineProcessor{
		Host: host,
		WriteLine: func(line string) {
			_, _ = io.WriteString(nonNilWriter(options.Stdout), line)
		},
	}
	unsubscribe := host.SubscribeEvents(func(event AgentSessionEvent) {
		processor.WriteEvent(event)
	})
	defer unsubscribe()
	if err := AttachJSONLLineReader(nonNilReader(options.Stdin), func(line string) {
		processor.HandleLine(context.Background(), line)
	}); err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	return 0
}

func newDefaultCLIRPCSessionHost(args Args, options CLIOptions) (*RPCSessionHost, error) {
	host, err := newDefaultCLIPrintModeHost(args, options)
	if err != nil {
		return nil, err
	}
	printHost, ok := host.(*agentSessionPrintModeHost)
	if !ok || printHost.session == nil {
		return nil, fmt.Errorf("RPC mode requires an agent session host")
	}
	rpcHost := NewRPCSessionHost(printHost.session)
	rpcHost.Settings = printHost.settingsManager
	return rpcHost, nil
}

func runPackageSubcommand(options CLIOptions) (int, bool) {
	if len(options.Args) == 0 {
		return 0, false
	}
	command := options.Args[0]
	if command == "uninstall" {
		command = "remove"
	}
	if command == "update" {
		return runUpdateSubcommand(options)
	}
	if command == "config" {
		return runConfigSubcommand(options), true
	}
	if command == "list" {
		return runListPackagesSubcommand(options), true
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
	if command == "install" {
		if err := manager.Install(source, project); err != nil {
			writeCLIError(options.Stderr, err.Error())
			return 1, true
		}
		_, _ = fmt.Fprintf(nonNilWriter(options.Stdout), "Installed %s\n", source)
		return 0, true
	}

	removed, err := manager.RemoveAndPersist(source, project)
	if err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1, true
	}
	if !removed {
		writeCLIError(options.Stderr, "No matching package found for "+source)
		return 1, true
	}
	_, _ = fmt.Fprintf(nonNilWriter(options.Stdout), "Removed %s\n", source)
	return 0, true
}

func runConfigSubcommand(options CLIOptions) int {
	for _, arg := range options.Args[1:] {
		switch arg {
		case "--help", "-h":
			writeConfigCommandUsage(nonNilWriter(options.Stdout))
			return 0
		default:
			writer := nonNilWriter(options.Stderr)
			_, _ = fmt.Fprintf(writer, "Unknown argument %s for %q.\n", arg, "config")
			_, _ = fmt.Fprintln(writer, `Use "gi --help" or "gi config".`)
			return 1
		}
	}
	cwd, agentDir, err := resolveCLICWDAndAgentDir(options)
	if err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	factory := options.ConfigHostFactory
	if factory == nil {
		factory = newDefaultCLIConfigHost
	}
	host, err := factory(PackageResourceConfigOptions{CWD: cwd, AgentDir: agentDir})
	if err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	if host == nil {
		writeCLIError(options.Stderr, "Config host is nil.")
		return 1
	}
	if err := host.Run(); err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	return 0
}

type cliUpdateTarget struct {
	updateSelf     bool
	updatePackages bool
	sources        []string
}

type cliUpdateParseResult struct {
	target             cliUpdateTarget
	force              bool
	invalidOption      string
	invalidArgument    string
	missingOptionValue string
	conflict           string
}

func runUpdateSubcommand(options CLIOptions) (int, bool) {
	if packageCommandHasHelp(options.Args[1:]) {
		writeUpdateCommandUsage(nonNilWriter(options.Stdout))
		return 0, true
	}
	parsed := parseCLIUpdateArgs(options.Args[1:])
	if parsed.invalidOption != "" {
		writer := nonNilWriter(options.Stderr)
		_, _ = fmt.Fprintf(writer, "Unknown option %s for %q.\n", parsed.invalidOption, "update")
		_, _ = fmt.Fprintln(writer, `Use "gi --help" or "gi update [source]".`)
		return 1, true
	}
	if parsed.missingOptionValue != "" {
		writer := nonNilWriter(options.Stderr)
		_, _ = fmt.Fprintf(writer, "Missing value for %s.\n", parsed.missingOptionValue)
		writeUpdateCommandUsage(writer)
		return 1, true
	}
	if parsed.invalidArgument != "" {
		writer := nonNilWriter(options.Stderr)
		_, _ = fmt.Fprintf(writer, "Unexpected argument %s.\n", parsed.invalidArgument)
		writeUpdateCommandUsage(writer)
		return 1, true
	}
	if parsed.conflict != "" {
		writer := nonNilWriter(options.Stderr)
		_, _ = fmt.Fprintln(writer, parsed.conflict)
		writeUpdateCommandUsage(writer)
		return 1, true
	}

	manager := options.PackageManager
	if manager == nil {
		manager = defaultCLIPackageManager()
	}
	if parsed.target.updatePackages {
		if err := manager.Update(parsed.target.sources...); err != nil {
			writeCLIError(options.Stderr, err.Error())
			return 1, true
		}
		if len(parsed.target.sources) == 1 {
			_, _ = fmt.Fprintf(nonNilWriter(options.Stdout), "Updated %s\n", parsed.target.sources[0])
		} else {
			_, _ = fmt.Fprintln(nonNilWriter(options.Stdout), "Updated packages")
		}
	}
	if parsed.target.updateSelf {
		environment := options.InstallEnvironment
		if isZeroInstallEnvironment(environment) {
			environment = DefaultInstallEnvironment()
		}
		result, err := manager.RunSelfUpdate(SelfUpdateOptions{
			PackageName:         options.PackageName,
			CurrentVersion:      options.Version,
			Force:               parsed.force,
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
	return 0, true
}

func parseCLIUpdateArgs(args []string) cliUpdateParseResult {
	var result cliUpdateParseResult
	var selfFlag bool
	var extensionsFlag bool
	var extensionSource string
	var source string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--self":
			selfFlag = true
		case arg == "--extensions":
			extensionsFlag = true
		case arg == "--force":
			result.force = true
		case arg == "--extension":
			if extensionSource != "" {
				result.conflict = firstNonEmptyString(result.conflict, "--extension can only be provided once")
				continue
			}
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				result.missingOptionValue = firstNonEmptyString(result.missingOptionValue, arg)
				continue
			}
			index++
			extensionSource = args[index]
		case strings.HasPrefix(arg, "-"):
			result.invalidOption = firstNonEmptyString(result.invalidOption, arg)
		default:
			if source == "" {
				source = arg
			} else {
				result.invalidArgument = firstNonEmptyString(result.invalidArgument, arg)
			}
		}
	}

	if extensionSource != "" {
		if selfFlag || extensionsFlag {
			result.conflict = firstNonEmptyString(result.conflict, "--extension cannot be combined with --self or --extensions")
		}
		if source != "" {
			result.conflict = firstNonEmptyString(result.conflict, "--extension cannot be combined with a positional source")
		}
		result.target = cliUpdateTarget{updatePackages: true, sources: []string{extensionSource}}
		return result
	}

	if source != "" {
		if source == "self" || source == "gi" {
			result.target.updateSelf = true
			result.target.updatePackages = extensionsFlag
			return result
		}
		if selfFlag || extensionsFlag {
			result.conflict = firstNonEmptyString(result.conflict, "positional update targets cannot be combined with --self or --extensions")
		}
		result.target = cliUpdateTarget{updatePackages: true, sources: []string{source}}
		return result
	}

	result.target.updateSelf = selfFlag
	result.target.updatePackages = !selfFlag || extensionsFlag
	return result
}

func runListPackagesSubcommand(options CLIOptions) int {
	args := options.Args[1:]
	if packageCommandHasHelp(args) {
		writeListPackagesUsage(nonNilWriter(options.Stdout))
		return 0
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			writer := nonNilWriter(options.Stderr)
			_, _ = fmt.Fprintf(writer, "Unknown option %s for %q.\n", arg, "list")
			_, _ = fmt.Fprintln(writer, `Use "gi --help" or "gi list".`)
			return 1
		}
		writer := nonNilWriter(options.Stderr)
		_, _ = fmt.Fprintf(writer, "Unexpected argument %s.\n", arg)
		writeListPackagesUsage(writer)
		return 1
	}
	manager := options.PackageManager
	if manager == nil {
		manager = defaultCLIPackageManager()
	}
	writeConfiguredPackages(nonNilWriter(options.Stdout), manager.ListConfiguredPackages())
	return 0
}

func packageCommandHasHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func writeConfiguredPackages(writer io.Writer, packages []ConfiguredPackage) {
	if writer == nil {
		return
	}
	if len(packages) == 0 {
		_, _ = fmt.Fprintln(writer, "No packages installed.")
		return
	}
	writePackageScope := func(title, scope string) {
		wroteTitle := false
		for _, pkg := range packages {
			if pkg.Scope != scope {
				continue
			}
			if !wroteTitle {
				_, _ = fmt.Fprintln(writer, title+":")
				wroteTitle = true
			}
			display := pkg.Source
			if pkg.Filtered {
				display += " (filtered)"
			}
			_, _ = fmt.Fprintln(writer, "  "+display)
			if pkg.InstalledPath != "" {
				_, _ = fmt.Fprintln(writer, "    "+pkg.InstalledPath)
			}
		}
	}
	writePackageScope("User packages", "user")
	hasUser := false
	for _, pkg := range packages {
		if pkg.Scope == "user" {
			hasUser = true
			break
		}
	}
	if hasUser {
		for _, pkg := range packages {
			if pkg.Scope == "project" {
				_, _ = fmt.Fprintln(writer)
				break
			}
		}
	}
	writePackageScope("Project packages", "project")
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
	_, _ = fmt.Fprintln(writer, "  gi update --extensions")
	_, _ = fmt.Fprintln(writer, "  gi update --extension <source>")
	_, _ = fmt.Fprintln(writer, "  gi update --self [--force]")
}

func writeListPackagesUsage(writer io.Writer) {
	if writer == nil {
		return
	}
	_, _ = fmt.Fprintln(writer, "Usage:")
	_, _ = fmt.Fprintln(writer, "  gi list")
}

func writeConfigCommandUsage(writer io.Writer) {
	if writer == nil {
		return
	}
	_, _ = fmt.Fprintln(writer, "Usage:")
	_, _ = fmt.Fprintln(writer, "  gi config")
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "Open resource configuration.")
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
