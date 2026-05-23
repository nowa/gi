package gicodingagent

import (
	"os"
	"os/exec"
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

func GetSelfUpdateCommand(packageName string, env InstallEnvironment, _ []string, updatePackageName string) *SelfUpdateCommand {
	if updatePackageName == "" {
		updatePackageName = packageName
	}
	return nil
}

func GetUpdateInstruction(packageName string, env InstallEnvironment) string {
	return GetSelfUpdateUnavailableInstruction(packageName, env, nil, packageName)
}

func GetSelfUpdateUnavailableInstruction(packageName string, env InstallEnvironment, _ []string, updatePackageName string) string {
	if updatePackageName == "" {
		updatePackageName = packageName
	}
	method := DetectInstallMethod(env)
	if method == InstallMethodBunBinary {
		return "Download from: https://github.com/nowa/gi/releases/latest"
	}
	if isNodePackageManagerInstall(method) {
		return "Gi does not support npm, pnpm, yarn, or bun self-updates. Update " + updatePackageName + " using the package manager, wrapper, or source checkout that provides this installation."
	}
	return "Update " + updatePackageName + " using the package manager, wrapper, or source checkout that provides this installation."
}

func isNodePackageManagerInstall(method InstallMethod) bool {
	return method == InstallMethodNPM || method == InstallMethodPNPM || method == InstallMethodYarn || method == InstallMethodBun
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
