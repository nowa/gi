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
	DefaultCodingAgentVersion     = "0.0.0"

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
		PackageDir: os.Getenv("PI_PACKAGE_DIR"),
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

func DetectInstallMethod(env InstallEnvironment) InstallMethod {
	if env.BunBinary {
		return InstallMethodBunBinary
	}
	resolvedPath := strings.ToLower(strings.ReplaceAll(env.PackageDir+"\x00"+env.ExecPath, "\\", "/"))
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

func GetSelfUpdateCommand(packageName string, env InstallEnvironment, npmCommand []string, updatePackageName string) *SelfUpdateCommand {
	if updatePackageName == "" {
		updatePackageName = packageName
	}
	method := DetectInstallMethod(env)
	command := getSelfUpdateCommandForMethod(method, packageName, updatePackageName, env, npmCommand)
	if command == nil || !isManagedByGlobalPackageManager(method, packageName, env, npmCommand) || !isSelfUpdatePathWritable(env) {
		return nil
	}
	return command
}

func GetUpdateInstruction(packageName string, env InstallEnvironment) string {
	method := DetectInstallMethod(env)
	if command := getSelfUpdateCommandForMethod(method, packageName, packageName, env, nil); command != nil {
		return "Run: " + command.Display
	}
	return GetSelfUpdateUnavailableInstruction(packageName, env, nil, packageName)
}

func GetSelfUpdateUnavailableInstruction(packageName string, env InstallEnvironment, npmCommand []string, updatePackageName string) string {
	if updatePackageName == "" {
		updatePackageName = packageName
	}
	method := DetectInstallMethod(env)
	if method == InstallMethodBunBinary {
		return "Download from: https://github.com/earendil-works/pi-mono/releases/latest"
	}
	command := getSelfUpdateCommandForMethod(method, packageName, updatePackageName, env, npmCommand)
	if command != nil {
		if isManagedByGlobalPackageManager(method, packageName, env, npmCommand) && !isSelfUpdatePathWritable(env) {
			return "This installation is managed by a global " + string(method) + " install, but the install path is not writable. Update it yourself with: " + command.Display
		}
		return "This installation is not managed by a global " + string(method) + " install. Update it with the package manager, wrapper, or source checkout that provides it."
	}
	return "Update " + updatePackageName + " using the package manager, wrapper, or source checkout that provides this installation."
}

func getSelfUpdateCommandForMethod(method InstallMethod, installedPackageName, updatePackageName string, env InstallEnvironment, npmCommand []string) *SelfUpdateCommand {
	switch method {
	case InstallMethodBunBinary:
		return nil
	case InstallMethodPNPM:
		return makeSelfUpdateCommand(
			makeSelfUpdateCommandStep("pnpm", []string{"install", "-g", updatePackageName}),
			removeStepIfRenamed(updatePackageName, installedPackageName, "pnpm", []string{"remove", "-g", installedPackageName}),
		)
	case InstallMethodYarn:
		return makeSelfUpdateCommand(
			makeSelfUpdateCommandStep("yarn", []string{"global", "add", updatePackageName}),
			removeStepIfRenamed(updatePackageName, installedPackageName, "yarn", []string{"global", "remove", installedPackageName}),
		)
	case InstallMethodBun:
		return makeSelfUpdateCommand(
			makeSelfUpdateCommandStep("bun", []string{"install", "-g", updatePackageName}),
			removeStepIfRenamed(updatePackageName, installedPackageName, "bun", []string{"uninstall", "-g", installedPackageName}),
		)
	case InstallMethodNPM:
		command := "npm"
		npmArgs := []string{}
		if len(npmCommand) > 0 {
			command = npmCommand[0]
			npmArgs = append(npmArgs, npmCommand[1:]...)
		} else if inferred, ok := getInferredNPMInstall(env); ok {
			npmArgs = append(npmArgs, "--prefix", inferred.prefix)
		}
		installArgs := append(append([]string{}, npmArgs...), "install", "-g", updatePackageName)
		var uninstallStep *SelfUpdateCommandStep
		if updatePackageName != installedPackageName {
			uninstallArgs := append(append([]string{}, npmArgs...), "uninstall", "-g", installedPackageName)
			step := makeSelfUpdateCommandStep(command, uninstallArgs)
			uninstallStep = &step
		}
		return makeSelfUpdateCommand(makeSelfUpdateCommandStep(command, installArgs), uninstallStep)
	default:
		return nil
	}
}

func removeStepIfRenamed(updatePackageName, installedPackageName, command string, args []string) *SelfUpdateCommandStep {
	if updatePackageName == installedPackageName {
		return nil
	}
	step := makeSelfUpdateCommandStep(command, args)
	return &step
}

func makeSelfUpdateCommand(installStep SelfUpdateCommandStep, uninstallStep *SelfUpdateCommandStep) *SelfUpdateCommand {
	command := &SelfUpdateCommand{Command: installStep.Command, Args: installStep.Args, Display: installStep.Display}
	if uninstallStep != nil {
		command.Display = uninstallStep.Display + " && " + installStep.Display
		command.Steps = []SelfUpdateCommandStep{*uninstallStep, installStep}
	}
	return command
}

func makeSelfUpdateCommandStep(command string, args []string) SelfUpdateCommandStep {
	displayParts := append([]string{command}, args...)
	for i, part := range displayParts {
		if strings.ContainsAny(part, " \t\r\n") {
			displayParts[i] = `"` + part + `"`
		}
	}
	return SelfUpdateCommandStep{Command: command, Args: append([]string(nil), args...), Display: strings.Join(displayParts, " ")}
}

type inferredNPMInstall struct {
	root   string
	prefix string
}

func getInferredNPMInstall(env InstallEnvironment) (inferredNPMInstall, bool) {
	packageDir := env.PackageDir
	if packageDir == "" || strings.Contains(packageDir, "\\") {
		return inferredNPMInstall{}, false
	}
	parent := filepath.Dir(packageDir)
	var root string
	if strings.HasPrefix(filepath.Base(parent), "@") && filepath.Base(filepath.Dir(parent)) == "node_modules" {
		root = filepath.Dir(parent)
	} else if filepath.Base(parent) == "node_modules" {
		root = parent
	}
	if root == "" {
		return inferredNPMInstall{}, false
	}
	rootParent := filepath.Dir(root)
	if filepath.Base(rootParent) == "lib" {
		return inferredNPMInstall{root: root, prefix: filepath.Dir(rootParent)}, true
	}
	return inferredNPMInstall{}, false
}

func isManagedByGlobalPackageManager(method InstallMethod, packageName string, env InstallEnvironment, npmCommand []string) bool {
	packageDir, ok := normalizeExistingInstallPath(env.PackageDir, env.Platform)
	if !ok {
		return false
	}
	for _, root := range getGlobalPackageRoots(method, packageName, env, npmCommand) {
		normalizedRoot, ok := normalizeExistingInstallPath(root, env.Platform)
		if !ok {
			continue
		}
		prefix := normalizedRoot
		if !strings.HasSuffix(prefix, string(filepath.Separator)) {
			prefix += string(filepath.Separator)
		}
		if strings.HasPrefix(packageDir, prefix) {
			return true
		}
	}
	return false
}

func getGlobalPackageRoots(method InstallMethod, _ string, env InstallEnvironment, npmCommand []string) []string {
	switch method {
	case InstallMethodNPM:
		configured := len(npmCommand) > 0
		command := "npm"
		args := []string{}
		if configured {
			command = npmCommand[0]
			args = append(args, npmCommand[1:]...)
		}
		if configured && command == "bun" {
			roots := []string{filepath.Join(env.HomeDir, ".bun", "install", "global", "node_modules")}
			if bunBin, ok := readInstallCommandOutput(env, command, append(args, "pm", "bin", "-g"), true); ok {
				roots = append(roots, filepath.Join(filepath.Dir(bunBin), "install", "global", "node_modules"))
			}
			return roots
		}
		roots := []string{}
		if root, ok := readInstallCommandOutput(env, command, append(args, "root", "-g"), configured); ok {
			roots = append(roots, root)
		}
		if !configured {
			if inferred, ok := getInferredNPMInstall(env); ok {
				roots = append(roots, inferred.root)
			}
		}
		return roots
	case InstallMethodPNPM:
		if root, ok := readInstallCommandOutput(env, "pnpm", []string{"root", "-g"}, false); ok {
			return []string{root, filepath.Dir(root)}
		}
	case InstallMethodYarn:
		if dir, ok := readInstallCommandOutput(env, "yarn", []string{"global", "dir"}, false); ok {
			return []string{dir, filepath.Join(dir, "node_modules")}
		}
	case InstallMethodBun:
		roots := []string{filepath.Join(env.HomeDir, ".bun", "install", "global", "node_modules")}
		if bunBin, ok := readInstallCommandOutput(env, "bun", []string{"pm", "bin", "-g"}, false); ok {
			roots = append(roots, filepath.Join(filepath.Dir(bunBin), "install", "global", "node_modules"))
		}
		return roots
	}
	return nil
}

func readInstallCommandOutput(env InstallEnvironment, command string, args []string, requireSuccess bool) (string, bool) {
	if env.CommandOutput == nil {
		return "", false
	}
	value, ok, err := env.CommandOutput(command, args, requireSuccess)
	if err != nil {
		return "", false
	}
	return value, ok
}

func normalizeExistingInstallPath(path, platform string) (string, bool) {
	if path == "" || strings.Contains(path, "\\") || platform == "windows" {
		return "", false
	}
	resolved := filepath.Clean(path)
	if _, err := os.Stat(resolved); err != nil {
		return "", false
	}
	realPath, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", false
	}
	return realPath, true
}

func isSelfUpdatePathWritable(env InstallEnvironment) bool {
	packageDir := env.PackageDir
	if packageDir == "" || strings.Contains(packageDir, "\\") {
		return false
	}
	return pathModeWritable(packageDir) && pathModeWritable(filepath.Dir(packageDir))
}

func pathModeWritable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().Perm()&0o200 != 0
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
