package gicodingagent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	gitui "github.com/nowa/gi/gi-tui"
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
	Responder              AgentSessionResponder
	ProjectTrustPrompt     ProjectTrustPrompt
	ProjectTrustOverride   *bool
	FirstTimeSetupTerminal gitui.Terminal
}

func RunCLI(options CLIOptions) int {
	timings := newStartupTimingsFromEnv()
	defer timings.Print(nonNilWriter(options.Stderr))
	if code, handled := runPackageSubcommand(options); handled {
		timings.Mark("package command")
		return code
	}
	args := ParseArgs(options.Args)
	if args.ProjectTrustOverride == nil {
		args.ProjectTrustOverride = options.ProjectTrustOverride
	}
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
	if err := maybeRunCLIFirstTimeSetup(options); err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	timings.Mark("first-time setup")
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
	_, _ = fmt.Fprintln(writer, `gi - AI coding assistant with read, bash, edit, write tools

Usage:
  gi [options] [@files...] [messages...]

Commands:
  gi install <source> [-l]     Install package source and add to settings
  gi remove <source> [-l]      Remove package source from settings
  gi uninstall <source> [-l]   Alias for remove
  gi update [source|self|gi]   Update gi and installed packages
  gi list                      List installed packages from settings
  gi config [-l]               Open global or project resource configuration
  gi <command> --help          Show help for install/remove/uninstall/update/list

Options:
  --provider <name>              Provider name (default: google)
  --model <pattern>              Model pattern or ID (supports "provider/id" and optional ":<thinking>")
  --api-key <key>                API key (defaults to env vars)
  --system-prompt <text>         System prompt (default: coding assistant prompt)
  --append-system-prompt <text>  Append text or file contents to the system prompt (can be used multiple times)
  --mode <mode>                  Output mode: text (default), json, or rpc
  --print, -p                    Non-interactive mode: process prompt and exit
  --continue, -c                 Continue previous session
  --resume, -r                   Select a session to resume
  --session <path|id>            Use specific session file or partial UUID
  --fork <path|id>               Fork specific session file or partial UUID into a new session
  --session-dir <dir>            Directory for session storage and lookup
  --no-session                   Don't save session (ephemeral)
  --models <patterns>            Comma-separated model patterns for Ctrl+P cycling
                                 Supports globs (anthropic/*, *sonnet*) and fuzzy matching
  --no-tools, -nt                Disable all tools by default (built-in and extension)
  --no-builtin-tools, -nbt       Disable built-in tools by default but keep extension/custom tools enabled
  --tools, -t <tools>            Comma-separated allowlist of tool names to enable
                                 Applies to built-in, extension, and custom tools
  --thinking <level>             Set thinking level: off, minimal, low, medium, high, xhigh
  --extension, -e <path>         Load an extension descriptor (can be used multiple times)
  --no-extensions, -ne           Disable extension discovery (explicit -e paths still work)
  --skill <path>                 Load a skill file or directory (can be used multiple times)
  --no-skills, -ns               Disable skills discovery and loading
  --prompt-template <path>       Load a prompt template file or directory (can be used multiple times)
  --no-prompt-templates, -np     Disable prompt template discovery and loading
  --theme <path>                 Load a theme file or directory (can be used multiple times)
  --no-themes                    Disable theme discovery and loading
  --no-context-files, -nc        Disable AGENTS.md and CLAUDE.md discovery and loading
  --export <file>                Export session file to HTML and exit
  --list-models [search]         List available models (with optional fuzzy search)
  --verbose                      Force verbose startup (overrides quietStartup setting)
  --approve, -a                  Trust project-local files for this run
  --no-approve, -na              Ignore project-local files for this run
  --offline                      Disable startup network operations (same as GI_OFFLINE=1)
  --help, -h                     Show this help
  --version, -v                  Show version number

Extensions can register additional flags (e.g., --plan from plan-mode extension).`)
	if len(extensionFlags) > 0 {
		_, _ = fmt.Fprintln(writer, "")
		_, _ = fmt.Fprintln(writer, "Extension CLI Flags:")
		for _, flag := range extensionFlags {
			_, _ = fmt.Fprintln(writer, formatCLIExtensionFlagHelp(flag))
		}
	}
	_, _ = fmt.Fprint(writer, `
Examples:
  # Interactive mode
  gi

  # Interactive mode with initial prompt
  gi "List all .go files in gi-coding-agent/"

  # Include files in initial message
  gi @prompt.md @image.png "What color is the sky?"

  # Non-interactive mode (process and exit)
  gi -p "List all .go files in gi-coding-agent/"

  # Multiple messages (interactive)
  gi "Read go.mod" "What packages do we have?"

  # Continue previous session
  gi --continue "What did we discuss?"

  # Use different model
  gi --provider openai --model gpt-4o-mini "Help me refactor this code"

  # Use model with provider prefix (no --provider needed)
  gi --model openai/gpt-4o "Help me refactor this code"

  # Use model with thinking level shorthand
  gi --model sonnet:high "Solve this complex problem"

  # Limit model cycling to specific models
  gi --models claude-sonnet,claude-haiku,gpt-4o

  # Limit to a specific provider with glob pattern
  gi --models "github-copilot/*"

  # Cycle models with fixed thinking levels
  gi --models sonnet:high,haiku:low

  # Start with a specific thinking level
  gi --thinking high "Solve this complex problem"

  # Read-only mode (no file modifications possible)
  gi --tools read,grep,find,ls -p "Review the code in gi-coding-agent/"

  # Export a session file to HTML
  gi --export ~/.gi/agent/sessions/--path--/session.jsonl
  gi --export session.jsonl output.html

Environment Variables:
  ANTHROPIC_API_KEY                - Anthropic Claude API key
  ANTHROPIC_OAUTH_TOKEN            - Anthropic OAuth token (alternative to API key)
  OPENAI_API_KEY                   - OpenAI GPT API key
  AZURE_OPENAI_API_KEY             - Azure OpenAI API key
  AZURE_OPENAI_BASE_URL            - Azure OpenAI/Cognitive Services base URL
  AZURE_OPENAI_RESOURCE_NAME       - Azure OpenAI resource name (alternative to base URL)
  AZURE_OPENAI_API_VERSION         - Azure OpenAI API version
  AZURE_OPENAI_DEPLOYMENT_NAME_MAP - Azure OpenAI model=deployment map (comma-separated)
  COPILOT_GITHUB_TOKEN             - GitHub Copilot token
  DEEPSEEK_API_KEY                 - DeepSeek API key
  GEMINI_API_KEY                   - Google Gemini API key
  GOOGLE_CLOUD_API_KEY             - Google Vertex AI API key
  GROQ_API_KEY                     - Groq API key
  CEREBRAS_API_KEY                 - Cerebras API key
  XAI_API_KEY                      - xAI Grok API key
  FIREWORKS_API_KEY                - Fireworks API key
  TOGETHER_API_KEY                 - Together AI API key
  OPENROUTER_API_KEY               - OpenRouter API key
  AI_GATEWAY_API_KEY               - Vercel AI Gateway API key
  ZAI_API_KEY                      - ZAI API key
  MISTRAL_API_KEY                  - Mistral API key
  MINIMAX_API_KEY                  - MiniMax API key
  MINIMAX_CN_API_KEY               - MiniMax China API key
  MOONSHOT_API_KEY                 - Moonshot AI API key
  HF_TOKEN                         - Hugging Face token
  OPENCODE_API_KEY                 - OpenCode Zen/OpenCode Go API key
  KIMI_API_KEY                     - Kimi For Coding API key
  CLOUDFLARE_API_KEY               - Cloudflare API token (Workers AI and AI Gateway)
  CLOUDFLARE_ACCOUNT_ID            - Cloudflare account id (required for Cloudflare providers)
  CLOUDFLARE_GATEWAY_ID            - Cloudflare AI Gateway slug
  XIAOMI_API_KEY                   - Xiaomi MiMo API key
  XIAOMI_TOKEN_PLAN_CN_API_KEY     - Xiaomi MiMo Token Plan API key (China region)
  XIAOMI_TOKEN_PLAN_AMS_API_KEY    - Xiaomi MiMo Token Plan API key (Amsterdam region)
  XIAOMI_TOKEN_PLAN_SGP_API_KEY    - Xiaomi MiMo Token Plan API key (Singapore region)
  AWS_PROFILE                      - AWS profile for Amazon Bedrock
  AWS_ACCESS_KEY_ID                - AWS access key for Amazon Bedrock
  AWS_SECRET_ACCESS_KEY            - AWS secret key for Amazon Bedrock
  AWS_BEARER_TOKEN_BEDROCK         - Bedrock API key (bearer token)
  AWS_REGION                       - AWS region for Amazon Bedrock
  GI_CODING_AGENT_DIR              - Config directory (default: ~/.gi/agent)
  GI_CODING_AGENT_SESSION_DIR      - Session storage directory (overridden by --session-dir)
  GI_PACKAGE_DIR                   - Override package directory
  GI_OFFLINE                       - Disable startup network operations when set to 1/true/yes
  GI_TELEMETRY                     - Override install telemetry when set to 1/true/yes or 0/false/no
  GI_SHARE_VIEWER_URL              - Base URL for /share command

Built-in Tool Names:
  read   - Read file contents
  bash   - Execute bash commands
  edit   - Edit files with find/replace
  write  - Write files (creates/overwrites)
  grep   - Search file contents (read-only, off by default)
  find   - Find files by glob pattern (read-only, off by default)
  ls     - List directory contents (read-only, off by default)
`)
}

func cliHelpExtensionFlags(args Args, options CLIOptions) []ProtocolFlagRegistration {
	cwd, agentDir, err := resolveCLICWDAndAgentDir(options)
	if err != nil {
		return nil
	}
	settingsManager, _, err := newCLIRuntimeSettingsManager(args, cwd, agentDir, nil)
	if err != nil {
		return nil
	}
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
	options.Args, options.ProjectTrustOverride = extractPackageProjectTrustFlags(options.Args, options.ProjectTrustOverride)
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
		var err error
		manager, err = defaultCLIPackageManager(options, false)
		if err != nil {
			writeCLIError(options.Stderr, err.Error())
			return 1, true
		}
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
	local := false
	for _, arg := range options.Args[1:] {
		switch arg {
		case "--help", "-h":
			writeConfigCommandUsage(nonNilWriter(options.Stdout))
			return 0
		case "--local", "-l":
			local = true
		default:
			writer := nonNilWriter(options.Stderr)
			if strings.HasPrefix(arg, "-") {
				_, _ = fmt.Fprintf(writer, "Unknown option %s for %q.\n", arg, "config")
				_, _ = fmt.Fprintln(writer, `Use "gi --help" or "gi config".`)
			} else {
				_, _ = fmt.Fprintf(writer, "Unexpected argument %s.\n", arg)
				writeConfigCommandUsage(writer)
			}
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
	settings, _, err := newCLIRuntimeSettingsManager(
		Args{ProjectTrustOverride: options.ProjectTrustOverride},
		cwd,
		agentDir,
		cliProjectTrustPrompt(options),
	)
	if err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	if local && !settings.IsProjectTrusted() {
		writeCLIError(options.Stderr, "Project is not trusted. Use --approve to modify local resource config.")
		return 1
	}
	writeScope := ResourceConfigWriteGlobal
	if local {
		writeScope = ResourceConfigWriteProject
	}
	host, err := factory(PackageResourceConfigOptions{
		CWD:             cwd,
		AgentDir:        agentDir,
		SettingsManager: settings,
		WriteScope:      writeScope,
	})
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
		var err error
		manager, err = defaultCLIPackageManager(options, true)
		if err != nil {
			writeCLIError(options.Stderr, err.Error())
			return 1, true
		}
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
		var err error
		manager, err = defaultCLIPackageManager(options, false)
		if err != nil {
			writeCLIError(options.Stderr, err.Error())
			return 1
		}
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
	switch command {
	case "remove":
		_, _ = fmt.Fprint(writer, `Usage:
  gi remove <source> [-l]

Remove a package and its source from settings.
Alias: gi uninstall <source> [-l]

Options:
  -l, --local    Remove from project settings (.gi/settings.json)

Examples:
  gi remove official:gi-tools-ui
  gi uninstall official:gi-tools-ui
`)
	default:
		_, _ = fmt.Fprint(writer, `Usage:
  gi install <source> [-l]

Install a package and add it to settings.

Options:
  -l, --local    Install project-locally (.gi/settings.json)

Examples:
  gi install official:gi-tools-ui
  gi install git:github.com/user/repo
  gi install git:git@github.com:user/repo
  gi install https://github.com/user/repo
  gi install ssh://git@github.com/user/repo
  gi install ./local/path
`)
	}
}

func writeUpdateCommandUsage(writer io.Writer) {
	if writer == nil {
		return
	}
	_, _ = fmt.Fprint(writer, `Usage:
  gi update [source|self|gi] [--self] [--extensions] [--extension <source>] [--force]

Update gi and installed packages.

Options:
  --self                  Update gi only
  --extensions            Update installed packages only
  --extension <source>    Update one package only
  --force                 Reinstall gi even if the current version is latest

Short forms:
  gi update                Update gi and all packages
  gi update <source>       Update one package
  gi update gi             Update gi only (self works as alias to gi)
`)
}

func writeListPackagesUsage(writer io.Writer) {
	if writer == nil {
		return
	}
	_, _ = fmt.Fprint(writer, `Usage:
  gi list

List installed packages from user and project settings.
`)
}

func writeConfigCommandUsage(writer io.Writer) {
	if writer == nil {
		return
	}
	_, _ = fmt.Fprintln(writer, "Usage:")
	_, _ = fmt.Fprintln(writer, "  gi config [-l]")
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "Open resource configuration.")
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "Options:")
	_, _ = fmt.Fprintln(writer, "  -l, --local  Start in project-local write mode")
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

func defaultCLIPackageManager(options CLIOptions, useSavedProjectTrustOnly bool) (*DefaultPackageManager, error) {
	cwd, agentDir, err := resolveCLICWDAndAgentDir(options)
	if err != nil {
		return nil, err
	}
	settings, _, err := newCLICommandSettingsManager(
		Args{ProjectTrustOverride: options.ProjectTrustOverride},
		cwd,
		agentDir,
		cliProjectTrustPrompt(options),
		useSavedProjectTrustOnly,
	)
	if err != nil {
		return nil, err
	}
	return NewDefaultPackageManager(PackageManagerOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings}), nil
}

func extractPackageProjectTrustFlags(args []string, initial *bool) ([]string, *bool) {
	if len(args) == 0 {
		return nil, initial
	}
	filtered := []string{args[0]}
	override := initial
	for _, arg := range args[1:] {
		switch arg {
		case "--approve", "-a":
			value := true
			override = &value
		case "--no-approve", "-na":
			value := false
			override = &value
		default:
			filtered = append(filtered, arg)
		}
	}
	return filtered, override
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
