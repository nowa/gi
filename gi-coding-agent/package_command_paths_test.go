package gicodingagent

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPackageCommandPathsPiBasics(t *testing.T) {
	t.Run("persists global relative local package paths relative to settings.json", func(t *testing.T) {
		agentDir, projectDir := createPackageCommandPathDirs(t)
		relativePkgDir := filepath.Join(projectDir, "packages", "local-package")
		if err := os.MkdirAll(relativePkgDir, 0o755); err != nil {
			t.Fatal(err)
		}

		_, stderr, code := runPackageCommandCLI(t, []string{"install", "./packages/local-package"}, projectDir, agentDir)
		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr)
		}

		settingsPath := filepath.Join(agentDir, "settings.json")
		packages := packageCommandSettingsPackages(t, settingsPath)
		if len(packages) != 1 {
			t.Fatalf("packages = %#v", packages)
		}
		resolvedFromSettings := realPackageCommandPath(t, filepath.Join(agentDir, packages[0]))
		if want := realPackageCommandPath(t, relativePkgDir); resolvedFromSettings != want {
			t.Fatalf("stored path resolves to %q, want %q", resolvedFromSettings, want)
		}
	})

	t.Run("removes local packages using a path with a trailing slash", func(t *testing.T) {
		agentDir, projectDir := createPackageCommandPathDirs(t)
		packageDir := filepath.Join(filepath.Dir(agentDir), "local-package")
		if err := os.MkdirAll(packageDir, 0o755); err != nil {
			t.Fatal(err)
		}

		_, stderr, code := runPackageCommandCLI(t, []string{"install", packageDir + string(filepath.Separator)}, projectDir, agentDir)
		if code != 0 {
			t.Fatalf("install exit code = %d, stderr = %q", code, stderr)
		}
		settingsPath := filepath.Join(agentDir, "settings.json")
		if packages := packageCommandSettingsPackages(t, settingsPath); len(packages) != 1 {
			t.Fatalf("installed packages = %#v", packages)
		}

		_, stderr, code = runPackageCommandCLI(t, []string{"remove", packageDir + string(filepath.Separator)}, projectDir, agentDir)
		if code != 0 {
			t.Fatalf("remove exit code = %d, stderr = %q", code, stderr)
		}
		if packages := packageCommandSettingsPackages(t, settingsPath); len(packages) != 0 {
			t.Fatalf("removed packages = %#v", packages)
		}
	})

	t.Run("shows install subcommand help", func(t *testing.T) {
		stdout, stderr, code := runPackageCommandCLI(t, []string{"install", "--help"}, t.TempDir(), t.TempDir())
		if code != 0 {
			t.Fatalf("exit code = %d", code)
		}
		if !strings.Contains(stdout, "Usage:") || !strings.Contains(stdout, "gi install <source> [-l]") {
			t.Fatalf("stdout = %q", stdout)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("shows a friendly error for unknown install options", func(t *testing.T) {
		_, stderr, code := runPackageCommandCLI(t, []string{"install", "--unknown"}, t.TempDir(), t.TempDir())
		if code != 1 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr)
		}
		for _, expected := range []string{
			`Unknown option --unknown for "install".`,
			`Use "gi --help" or "gi install <source> [-l]".`,
		} {
			if !strings.Contains(stderr, expected) {
				t.Fatalf("stderr = %q, want %q", stderr, expected)
			}
		}
	})

	t.Run("shows a friendly error for missing install source", func(t *testing.T) {
		_, stderr, code := runPackageCommandCLI(t, []string{"install"}, t.TempDir(), t.TempDir())
		if code != 1 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr)
		}
		for _, expected := range []string{"Missing install source.", "Usage:", "gi install <source> [-l]"} {
			if !strings.Contains(stderr, expected) {
				t.Fatalf("stderr = %q, want %q", stderr, expected)
			}
		}
		if strings.Contains(stderr, "at ") {
			t.Fatalf("stderr contains stack-looking text: %q", stderr)
		}
	})

	t.Run("uses global npmCommand and current package name for forced self updates without checking the api", func(t *testing.T) {
		agentDir, projectDir := createPackageCommandPathDirs(t)
		globalPrefix := filepath.Join(filepath.Dir(agentDir), "global-prefix")
		projectPrefix := filepath.Join(filepath.Dir(agentDir), "project-prefix")
		selfPackageDir := packageCommandSelfPackageDir(t, globalPrefix, DefaultCodingAgentPackageName)
		writeSettingsJSON(t, filepath.Join(agentDir, "settings.json"), map[string]any{
			"npmCommand": []any{"fake-npm", "--prefix", globalPrefix},
		})
		writeSettingsJSON(t, filepath.Join(projectDir, ConfigDirName, "settings.json"), map[string]any{
			"npmCommand": []any{"fake-npm", "--prefix", projectPrefix},
		})
		ops := newPackageCommandOps(globalPrefix)
		versionChecked := false

		_, stderr, code := runPackageCommandCLIWithOptions(t, []string{"update", "--self", "--force"}, projectDir, agentDir, func(options *CLIOptions) {
			options.PackageManager = packageCommandManager(projectDir, agentDir, ops.operations())
			options.InstallEnvironment = packageCommandInstallEnvironment(selfPackageDir, globalPrefix)
			options.PackageName = DefaultCodingAgentPackageName
			options.VersionCheck = func(string, VersionCheckOptions) (LatestPiRelease, bool) {
				versionChecked = true
				return LatestPiRelease{}, false
			}
		})
		if code != 0 || stderr != "" {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if versionChecked {
			t.Fatal("forced self-update should not check latest version")
		}
		if len(ops.calls) != 1 {
			t.Fatalf("calls = %#v", ops.calls)
		}
		args := ops.calls[0].Args
		if !containsString(args, globalPrefix) || !containsString(args, DefaultCodingAgentPackageName) || containsString(args, projectPrefix) {
			t.Fatalf("args = %#v", args)
		}
	})

	t.Run("uses the current package name when the update check omits packageName", func(t *testing.T) {
		agentDir, projectDir := createPackageCommandPathDirs(t)
		globalPrefix := filepath.Join(filepath.Dir(agentDir), "global-prefix")
		selfPackageDir := packageCommandSelfPackageDir(t, globalPrefix, "@mariozechner/pi-coding-agent")
		writeSettingsJSON(t, filepath.Join(agentDir, "settings.json"), map[string]any{
			"npmCommand": []any{"fake-npm", "--prefix", globalPrefix},
		})
		ops := newPackageCommandOps(globalPrefix)
		versionChecks := 0

		_, stderr, code := runPackageCommandCLIWithOptions(t, []string{"update", "--self"}, projectDir, agentDir, func(options *CLIOptions) {
			options.PackageManager = packageCommandManager(projectDir, agentDir, ops.operations())
			options.InstallEnvironment = packageCommandInstallEnvironment(selfPackageDir, globalPrefix)
			options.PackageName = DefaultCodingAgentPackageName
			options.Version = "0.72.0"
			options.VersionCheck = func(string, VersionCheckOptions) (LatestPiRelease, bool) {
				versionChecks++
				return LatestPiRelease{Version: "0.73.0"}, true
			}
		})
		if code != 0 || stderr != "" {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if versionChecks != 1 {
			t.Fatalf("version checks = %d", versionChecks)
		}
		if len(ops.calls) != 1 || !containsString(ops.calls[0].Args, DefaultCodingAgentPackageName) {
			t.Fatalf("calls = %#v", ops.calls)
		}
	})

	t.Run("installs the active package name from the update check during self-update", func(t *testing.T) {
		agentDir, projectDir := createPackageCommandPathDirs(t)
		globalPrefix := filepath.Join(filepath.Dir(agentDir), "global-prefix")
		selfPackageDir := packageCommandSelfPackageDir(t, globalPrefix, "@mariozechner/pi-coding-agent")
		activePackageName := "@new-scope/pi"
		writeSettingsJSON(t, filepath.Join(agentDir, "settings.json"), map[string]any{
			"npmCommand": []any{"fake-npm", "--prefix", globalPrefix},
		})
		ops := newPackageCommandOps(globalPrefix)

		_, stderr, code := runPackageCommandCLIWithOptions(t, []string{"update", "--self"}, projectDir, agentDir, func(options *CLIOptions) {
			options.PackageManager = packageCommandManager(projectDir, agentDir, ops.operations())
			options.InstallEnvironment = packageCommandInstallEnvironment(selfPackageDir, globalPrefix)
			options.PackageName = DefaultCodingAgentPackageName
			options.Version = "0.72.0"
			options.VersionCheck = func(string, VersionCheckOptions) (LatestPiRelease, bool) {
				return LatestPiRelease{PackageName: activePackageName, Version: "0.73.0"}, true
			}
		})
		if code != 0 || stderr != "" {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		want := []packageCommandCall{
			{Command: "fake-npm", Args: []string{"--prefix", globalPrefix, "uninstall", "-g", DefaultCodingAgentPackageName}},
			{Command: "fake-npm", Args: []string{"--prefix", globalPrefix, "install", "-g", activePackageName}},
		}
		if !reflect.DeepEqual(ops.calls, want) {
			t.Fatalf("calls = %#v, want %#v", ops.calls, want)
		}
	})

	t.Run("fails self-update when renamed npm package installation fails", func(t *testing.T) {
		agentDir, projectDir := createPackageCommandPathDirs(t)
		globalPrefix := filepath.Join(filepath.Dir(agentDir), "global-prefix")
		selfPackageDir := packageCommandSelfPackageDir(t, globalPrefix, "@mariozechner/pi-coding-agent")
		activePackageName := "@new-scope/pi"
		writeSettingsJSON(t, filepath.Join(agentDir, "settings.json"), map[string]any{
			"npmCommand": []any{"fake-npm", "--prefix", globalPrefix},
		})
		ops := newPackageCommandOps(globalPrefix)
		ops.failInstall = true

		stdout, stderr, code := runPackageCommandCLIWithOptions(t, []string{"update", "--self"}, projectDir, agentDir, func(options *CLIOptions) {
			options.PackageManager = packageCommandManager(projectDir, agentDir, ops.operations())
			options.InstallEnvironment = packageCommandInstallEnvironment(selfPackageDir, globalPrefix)
			options.PackageName = DefaultCodingAgentPackageName
			options.Version = "0.72.0"
			options.VersionCheck = func(string, VersionCheckOptions) (LatestPiRelease, bool) {
				return LatestPiRelease{PackageName: activePackageName, Version: "0.73.0"}, true
			}
		})
		if code != 1 {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if strings.Contains(stdout, "Updated") || !strings.Contains(stderr, "exited with code 23") {
			t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
		}
		want := []packageCommandCall{
			{Command: "fake-npm", Args: []string{"--prefix", globalPrefix, "uninstall", "-g", DefaultCodingAgentPackageName}},
			{Command: "fake-npm", Args: []string{"--prefix", globalPrefix, "install", "-g", activePackageName}},
		}
		if !reflect.DeepEqual(ops.calls, want) {
			t.Fatalf("calls = %#v, want %#v", ops.calls, want)
		}
	})

	t.Run("suggests the configured source when update input omits the git prefix", func(t *testing.T) {
		agentDir, projectDir := createPackageCommandPathDirs(t)
		settingsPath := filepath.Join(agentDir, "settings.json")
		writeSettingsJSON(t, settingsPath, map[string]any{"packages": []any{"git:github.com/gi-packages/formatter"}})

		stdout, stderr, code := runPackageCommandCLI(t, []string{"update", "github.com/gi-packages/formatter"}, projectDir, agentDir)
		if code != 1 {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if !strings.Contains(stderr, "Did you mean git:github.com/gi-packages/formatter?") || strings.Contains(stdout, "Updated github.com/gi-packages/formatter") {
			t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
		}
		if packages := packageCommandSettingsPackages(t, settingsPath); !reflect.DeepEqual(packages, []string{"git:github.com/gi-packages/formatter"}) {
			t.Fatalf("packages = %#v", packages)
		}
	})
}

func createPackageCommandPathDirs(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	agentDir := filepath.Join(root, "agent")
	projectDir := filepath.Join(root, "project")
	for _, dir := range []string{agentDir, projectDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return agentDir, projectDir
}

func runPackageCommandCLI(t *testing.T, args []string, projectDir, agentDir string) (string, string, int) {
	return runPackageCommandCLIWithOptions(t, args, projectDir, agentDir, nil)
}

func runPackageCommandCLIWithOptions(t *testing.T, args []string, projectDir, agentDir string, configure func(*CLIOptions)) (string, string, int) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	options := CLIOptions{
		Args:   args,
		Stdout: &stdout,
		Stderr: &stderr,
		PackageManager: NewDefaultPackageManager(PackageManagerOptions{
			CWD:      projectDir,
			AgentDir: agentDir,
		}),
	}
	if configure != nil {
		configure(&options)
	}
	code := RunCLI(options)
	return stdout.String(), stderr.String(), code
}

func packageCommandSettingsPackages(t *testing.T, settingsPath string) []string {
	t.Helper()
	settings := readSettingsJSON(t, settingsPath)
	values, _ := settings["packages"].([]any)
	return settingsPackagesToStrings(values)
}

func realPackageCommandPath(t *testing.T, path string) string {
	t.Helper()
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(realPath)
}

func packageCommandManager(projectDir, agentDir string, operations PackageManagerOperations) *DefaultPackageManager {
	return NewDefaultPackageManager(PackageManagerOptions{CWD: projectDir, AgentDir: agentDir, Operations: operations})
}

func packageCommandSelfPackageDir(t *testing.T, prefix, packageName string) string {
	t.Helper()
	scope, name, ok := strings.Cut(strings.TrimPrefix(packageName, "@"), "/")
	if !ok {
		scope = ""
		name = packageName
	}
	elements := []string{prefix, "lib", "node_modules"}
	if scope != "" {
		elements = append(elements, "@"+scope, name)
	} else {
		elements = append(elements, name)
	}
	packageDir := filepath.Join(elements...)
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return packageDir
}

func packageCommandInstallEnvironment(selfPackageDir, globalPrefix string) InstallEnvironment {
	root := filepath.Join(globalPrefix, "lib", "node_modules")
	return InstallEnvironment{
		PackageDir: selfPackageDir,
		ExecPath:   filepath.Join(selfPackageDir, "dist", "cli.js"),
		HomeDir:    filepath.Dir(globalPrefix),
		CommandOutput: func(command string, args []string, requireSuccess bool) (string, bool, error) {
			if command == "fake-npm" && containsString(args, "root") && containsString(args, "-g") {
				return root, true, nil
			}
			if requireSuccess {
				return "", false, errors.New("unexpected package command")
			}
			return "", false, nil
		},
	}
}

type packageCommandCall struct {
	Command string
	Args    []string
}

type packageCommandOps struct {
	globalPrefix string
	failInstall  bool
	calls        []packageCommandCall
}

func newPackageCommandOps(globalPrefix string) *packageCommandOps {
	return &packageCommandOps{globalPrefix: globalPrefix}
}

func (o *packageCommandOps) operations() PackageManagerOperations {
	return PackageManagerOperations{
		RunCommand: func(command string, args []string, _ PackageCommandOptions) error {
			copied := append([]string(nil), args...)
			o.calls = append(o.calls, packageCommandCall{Command: command, Args: copied})
			if o.failInstall && containsString(args, "install") {
				return errors.New("exited with code 23")
			}
			return nil
		},
		RunCommandCapture: func(string, []string, PackageCommandOptions) (string, error) {
			return "", nil
		},
	}
}
