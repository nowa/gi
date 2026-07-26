package gicodingagent

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type InstallMethod string

const (
	DefaultCodingAgentPackageName = "gi"
	DefaultCodingAgentVersion     = "0.82.1"
	DefaultCodingAgentAppName     = "gi"
	DefaultCodingAgentAppTitle    = "Gi"

	EnvCodingAgentDir              = "GI_CODING_AGENT_DIR"
	LegacyEnvCodingAgentDir        = "PI_CODING_AGENT_DIR"
	EnvCodingAgentSessionDir       = "GI_CODING_AGENT_SESSION_DIR"
	LegacyEnvCodingAgentSessionDir = "PI_CODING_AGENT_SESSION_DIR"

	InstallMethodBunBinary InstallMethod = "bun-binary"
	InstallMethodNPM       InstallMethod = "npm"
	InstallMethodPNPM      InstallMethod = "pnpm"
	InstallMethodYarn      InstallMethod = "yarn"
	InstallMethodBun       InstallMethod = "bun"
	InstallMethodUnknown   InstallMethod = "unknown"
)

type SelfUpdateCommandStep struct {
	Command string
	Args    []string
	Display string
}

type SelfUpdateCommand struct {
	Command string
	Args    []string
	Display string
	Steps   []SelfUpdateCommandStep
}

// SelfUpdatePackageTarget separates the package identity used to replace an
// installation from the exact spec a package manager would install.
type SelfUpdatePackageTarget struct {
	PackageName string
	InstallSpec string
}

type InstallEnvironment struct {
	ExecPath      string
	PackageDir    string
	Platform      string
	HomeDir       string
	BunBinary     bool
	BunRuntime    bool
	CommandOutput func(command string, args []string, requireSuccess bool) (string, bool, error)
}

func DefaultInstallEnvironment() InstallEnvironment {
	home, _ := os.UserHomeDir()
	return InstallEnvironment{
		ExecPath:   firstNonEmptyString(os.Args[0], os.Getenv("_")),
		PackageDir: firstNonEmptyString(os.Getenv("GI_PACKAGE_DIR"), os.Getenv("PI_PACKAGE_DIR")),
		Platform:   runtime.GOOS,
		HomeDir:    home,
		CommandOutput: func(command string, args []string, requireSuccess bool) (string, bool, error) {
			output, err := exec.Command(command, args...).Output()
			if err != nil {
				if requireSuccess {
					return "", false, err
				}
				return "", false, nil
			}
			value := strings.TrimSpace(string(output))
			return value, value != "", nil
		},
	}
}

func ExpandTildePath(path string) string {
	return ExpandPath(path)
}

func GetPackageDir(envs ...InstallEnvironment) string {
	env := optionalInstallEnvironment(envs...)
	if strings.TrimSpace(env.PackageDir) != "" {
		return expandTildePathWithHome(env.PackageDir, env.HomeDir)
	}
	if env.BunBinary && strings.TrimSpace(env.ExecPath) != "" {
		return filepath.Dir(env.ExecPath)
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

func GetThemesDir(envs ...InstallEnvironment) string {
	return filepath.Join(GetPackageDir(envs...), "gi-coding-agent", "themes")
}

func GetExportTemplateDir(envs ...InstallEnvironment) string {
	return filepath.Join(GetPackageDir(envs...), "gi-coding-agent", "export-html")
}

func GetPackageJSONPath(envs ...InstallEnvironment) string {
	return filepath.Join(GetPackageDir(envs...), "go.mod")
}

func GetPackageJsonPath(envs ...InstallEnvironment) string {
	return GetPackageJSONPath(envs...)
}

func GetReadmePath(envs ...InstallEnvironment) string {
	return filepath.Join(GetPackageDir(envs...), "README.md")
}

func GetDocsPath(envs ...InstallEnvironment) string {
	return filepath.Join(GetPackageDir(envs...), "docs")
}

func GetExamplesPath(envs ...InstallEnvironment) string {
	return filepath.Join(GetPackageDir(envs...), "examples")
}

func GetChangelogPath(envs ...InstallEnvironment) string {
	return filepath.Join(GetPackageDir(envs...), "CHANGELOG.md")
}

func GetInteractiveAssetsDir(envs ...InstallEnvironment) string {
	return filepath.Join(GetPackageDir(envs...), "gi-coding-agent", "assets")
}

func GetBundledInteractiveAssetPath(name string, envs ...InstallEnvironment) string {
	return filepath.Join(GetInteractiveAssetsDir(envs...), name)
}

func GetShareViewerURL(gistID string) string {
	return shareViewerURL(gistID)
}

func GetAgentDir(cwd ...string) string {
	if env := firstNonEmptyString(os.Getenv(EnvCodingAgentDir), os.Getenv(LegacyEnvCodingAgentDir)); env != "" {
		return ExpandPath(env)
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ConfigDirName, "agent")
	}
	if len(cwd) > 0 && strings.TrimSpace(cwd[0]) != "" {
		return filepath.Join(cwd[0], ConfigDirName, "agent")
	}
	return filepath.Join(".", ConfigDirName, "agent")
}

func GetCustomThemesDir(agentDir ...string) string {
	return filepath.Join(optionalAgentDir(agentDir...), "themes")
}

func GetModelsPath(agentDir ...string) string {
	return filepath.Join(optionalAgentDir(agentDir...), "models.json")
}

func GetAuthPath(agentDir ...string) string {
	return filepath.Join(optionalAgentDir(agentDir...), "auth.json")
}

func GetSettingsPath(agentDir ...string) string {
	return filepath.Join(optionalAgentDir(agentDir...), "settings.json")
}

func GetToolsDir(agentDir ...string) string {
	return filepath.Join(optionalAgentDir(agentDir...), "tools")
}

func GetBinDir(agentDir ...string) string {
	return filepath.Join(optionalAgentDir(agentDir...), "bin")
}

func GetPromptsDir(agentDir ...string) string {
	return filepath.Join(optionalAgentDir(agentDir...), "prompts")
}

func GetSessionsDir(agentDir ...string) string {
	return filepath.Join(optionalAgentDir(agentDir...), "sessions")
}

func GetDebugLogPath(agentDir ...string) string {
	return filepath.Join(optionalAgentDir(agentDir...), DefaultCodingAgentAppName+"-debug.log")
}

func DetectInstallMethod(env InstallEnvironment) InstallMethod {
	if env.BunBinary {
		return InstallMethodBunBinary
	}
	resolvedPath := strings.Join(installDetectionPathCandidates(env), "\x00")
	switch {
	case strings.Contains(resolvedPath, "/pnpm/") || strings.Contains(resolvedPath, "/.pnpm/"):
		return InstallMethodPNPM
	case strings.Contains(resolvedPath, "/yarn/") || strings.Contains(resolvedPath, "/.yarn/"):
		return InstallMethodYarn
	case env.BunRuntime || strings.Contains(resolvedPath, "/install/global/node_modules/"):
		return InstallMethodBun
	case strings.Contains(resolvedPath, "/npm/") || strings.Contains(resolvedPath, "/node_modules/"):
		return InstallMethodNPM
	default:
		return InstallMethodUnknown
	}
}

func GetSelfUpdateCommand(packageName string, env InstallEnvironment, _ []string, updatePackageName string) *SelfUpdateCommand {
	target := SelfUpdatePackageTarget{
		PackageName: firstNonEmptyString(strings.TrimSpace(updatePackageName), strings.TrimSpace(packageName)),
	}
	return GetSelfUpdateCommandForTarget(packageName, env, target)
}

// GetSelfUpdateCommandForTarget preserves the typed update target at the
// command-generation boundary. Gi intentionally does not execute Node package
// manager self-updates, so no environment currently produces a command.
func GetSelfUpdateCommandForTarget(packageName string, _ InstallEnvironment, target SelfUpdatePackageTarget) *SelfUpdateCommand {
	_ = resolveSelfUpdatePackageTarget(packageName, target)
	return nil
}

func GetUpdateInstruction(packageName string, env InstallEnvironment) string {
	return GetSelfUpdateUnavailableInstruction(packageName, env, nil, packageName)
}

func GetSelfUpdateUnavailableInstruction(packageName string, env InstallEnvironment, _ []string, updatePackageName string) string {
	target := SelfUpdatePackageTarget{
		PackageName: firstNonEmptyString(strings.TrimSpace(updatePackageName), strings.TrimSpace(packageName)),
	}
	return GetSelfUpdateUnavailableInstructionForTarget(packageName, env, target)
}

// GetSelfUpdateUnavailableInstructionForTarget describes the safe manual
// update path without collapsing an exact install spec into its package name.
func GetSelfUpdateUnavailableInstructionForTarget(packageName string, env InstallEnvironment, target SelfUpdatePackageTarget) string {
	target = resolveSelfUpdatePackageTarget(packageName, target)
	method := DetectInstallMethod(env)
	if method == InstallMethodBunBinary {
		return "Download from: https://github.com/nowa/gi/releases/latest"
	}
	if isNodePackageManagerInstall(method) {
		return "Gi does not support npm, pnpm, yarn, or bun self-updates. Update " + target.InstallSpec + " using the package manager, wrapper, or source checkout that provides this installation."
	}
	return "Update " + target.InstallSpec + " using the package manager, wrapper, or source checkout that provides this installation."
}

func isNodePackageManagerInstall(method InstallMethod) bool {
	return method == InstallMethodNPM || method == InstallMethodPNPM || method == InstallMethodYarn || method == InstallMethodBun
}

func normalizeSelfUpdatePackageTarget(target SelfUpdatePackageTarget) SelfUpdatePackageTarget {
	target.PackageName = strings.TrimSpace(target.PackageName)
	target.InstallSpec = strings.TrimSpace(target.InstallSpec)
	if target.InstallSpec == "" {
		target.InstallSpec = target.PackageName
	}
	return target
}

func resolveSelfUpdatePackageTarget(packageName string, target SelfUpdatePackageTarget) SelfUpdatePackageTarget {
	if strings.TrimSpace(target.PackageName) == "" {
		target.PackageName = packageName
	}
	return normalizeSelfUpdatePackageTarget(target)
}

func installDetectionPathCandidates(env InstallEnvironment) []string {
	paths := []string{env.PackageDir, env.ExecPath}
	if packageDir := getEntrypointPackageDir(env.ExecPath); packageDir != "" {
		paths = append(paths, packageDir)
	}

	candidates := make([]string, 0, len(paths)*3)
	seen := make(map[string]struct{}, len(paths)*3)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		path = strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		candidates = append(candidates, path)
	}
	for _, path := range paths {
		add(path)
		for _, candidate := range getPathComparisonCandidates(path, env.Platform) {
			add(candidate)
		}
	}
	return candidates
}

func normalizeExistingPathForComparison(path string, resolveSymlinks bool, platform string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	if _, err := os.Stat(absolutePath); err != nil {
		return ""
	}
	normalizedPath := filepath.Clean(absolutePath)
	if resolveSymlinks {
		normalizedPath, err = filepath.EvalSymlinks(normalizedPath)
		if err != nil {
			return ""
		}
		normalizedPath = filepath.Clean(normalizedPath)
	}
	if isWindowsInstallPlatform(platform) {
		normalizedPath = strings.ToLower(normalizedPath)
	}
	return normalizedPath
}

func getPathComparisonCandidates(path string, platform string) []string {
	candidates := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	for _, resolveSymlinks := range []bool{false, true} {
		candidate := normalizeExistingPathForComparison(path, resolveSymlinks, platform)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func getEntrypointPackageDir(entrypoint string) string {
	entrypoint = strings.TrimSpace(entrypoint)
	if entrypoint == "" {
		return ""
	}
	absoluteEntrypoint, err := filepath.Abs(entrypoint)
	if err != nil {
		return ""
	}
	for dir := filepath.Dir(absoluteEntrypoint); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		info, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil && !info.IsDir() {
			return dir
		}
	}
	return ""
}

func isWindowsInstallPlatform(platform string) bool {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform == "" {
		platform = runtime.GOOS
	}
	return platform == "windows" || platform == "win32"
}

func optionalInstallEnvironment(envs ...InstallEnvironment) InstallEnvironment {
	if len(envs) > 0 {
		return envs[0]
	}
	return DefaultInstallEnvironment()
}

func optionalAgentDir(agentDir ...string) string {
	if len(agentDir) > 0 && strings.TrimSpace(agentDir[0]) != "" {
		return ExpandPath(agentDir[0])
	}
	return GetAgentDir()
}

func expandTildePathWithHome(path, home string) string {
	path = normalizeUserPathText(path)
	if path == "~" {
		if home != "" {
			return home
		}
		return ExpandPath(path)
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home != "" {
			return filepath.Join(home, path[2:])
		}
		return ExpandPath(path)
	}
	return path
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
