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

func TestPackageCommandPathsGiProtocolBasics(t *testing.T) {
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

	t.Run("uninstall aliases remove", func(t *testing.T) {
		agentDir, projectDir := createPackageCommandPathDirs(t)
		packageDir := filepath.Join(filepath.Dir(agentDir), "local-package")
		if err := os.MkdirAll(packageDir, 0o755); err != nil {
			t.Fatal(err)
		}

		_, stderr, code := runPackageCommandCLI(t, []string{"install", packageDir}, projectDir, agentDir)
		if code != 0 {
			t.Fatalf("install exit code = %d, stderr = %q", code, stderr)
		}

		stdout, stderr, code := runPackageCommandCLI(t, []string{"uninstall", packageDir}, projectDir, agentDir)
		if code != 0 {
			t.Fatalf("uninstall exit code = %d, stderr = %q", code, stderr)
		}
		if !strings.Contains(stdout, "Removed "+packageDir) {
			t.Fatalf("stdout = %q", stdout)
		}
		if packages := packageCommandSettingsPackages(t, filepath.Join(agentDir, "settings.json")); len(packages) != 0 {
			t.Fatalf("packages = %#v", packages)
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

	t.Run("lists user and project packages with installed paths", func(t *testing.T) {
		agentDir, projectDir := createPackageCommandPathDirs(t)
		settingsPath := filepath.Join(agentDir, "settings.json")
		projectSettingsPath := filepath.Join(projectDir, ConfigDirName, "settings.json")
		writeSettingsJSON(t, settingsPath, map[string]any{"packages": []any{
			"official:gi-tools-ui",
			map[string]any{"source": "git:github.com/gi-packages/formatter", "skills": []any{"SKILL.md"}},
		}})
		writeSettingsJSON(t, projectSettingsPath, map[string]any{"packages": []any{"./local-package"}})

		stdout, stderr, code := runPackageCommandCLI(t, []string{"list"}, projectDir, agentDir)
		if code != 0 {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		for _, expected := range []string{
			"User packages:",
			"official:gi-tools-ui",
			filepath.Join(agentDir, "official-packages", "gi-tools-ui"),
			"git:github.com/gi-packages/formatter (filtered)",
			filepath.Join(agentDir, "git", "github.com", "gi-packages", "formatter"),
			"Project packages:",
			"./local-package",
			filepath.Join(projectDir, ConfigDirName, "local-package"),
		} {
			if !strings.Contains(stdout, expected) {
				t.Fatalf("stdout = %q, want %q", stdout, expected)
			}
		}
	})

	t.Run("list reports empty package settings", func(t *testing.T) {
		stdout, stderr, code := runPackageCommandCLI(t, []string{"list"}, t.TempDir(), t.TempDir())
		if code != 0 || stderr != "" {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if strings.TrimSpace(stdout) != "No packages installed." {
			t.Fatalf("stdout = %q", stdout)
		}
	})

	t.Run("config opens package resource selector host", func(t *testing.T) {
		agentDir, projectDir := createPackageCommandPathDirs(t)
		var got PackageResourceConfigOptions
		var called bool
		stdout, stderr, code := runPackageCommandCLIWithOptions(t, []string{"config"}, projectDir, agentDir, func(options *CLIOptions) {
			options.ConfigHostFactory = func(config PackageResourceConfigOptions) (CLIConfigRuntimeHost, error) {
				called = true
				got = config
				return fakeCLIConfigHost{}, nil
			}
		})
		if code != 0 || strings.TrimSpace(stdout) != "" || strings.TrimSpace(stderr) != "" {
			t.Fatalf("config code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		if !called || got.CWD != projectDir || got.AgentDir != agentDir {
			t.Fatalf("config host called=%v options=%#v", called, got)
		}
	})

	t.Run("shows config subcommand help", func(t *testing.T) {
		stdout, stderr, code := runPackageCommandCLI(t, []string{"config", "--help"}, t.TempDir(), t.TempDir())
		if code != 0 || strings.TrimSpace(stderr) != "" {
			t.Fatalf("config help code=%d stderr=%q", code, stderr)
		}
		if !strings.Contains(stdout, "gi config") || !strings.Contains(stdout, "resource configuration") {
			t.Fatalf("stdout = %q", stdout)
		}
	})

	t.Run("rejects forced self updates for legacy npm-style installs without checking the api", func(t *testing.T) {
		agentDir, projectDir := createPackageCommandPathDirs(t)
		globalPrefix := filepath.Join(filepath.Dir(agentDir), "global-prefix")
		selfPackageDir := packageCommandSelfPackageDir(t, globalPrefix, DefaultCodingAgentPackageName)
		ops := newPackageCommandOps(globalPrefix)
		versionChecked := false

		_, stderr, code := runPackageCommandCLIWithOptions(t, []string{"update", "--self", "--force"}, projectDir, agentDir, func(options *CLIOptions) {
			options.PackageManager = packageCommandManager(projectDir, agentDir, ops.operations())
			options.InstallEnvironment = packageCommandInstallEnvironment(selfPackageDir, globalPrefix)
			options.PackageName = DefaultCodingAgentPackageName
			options.VersionCheck = func(string, VersionCheckOptions) (LatestGiRelease, bool) {
				versionChecked = true
				return LatestGiRelease{}, false
			}
		})
		if code != 1 {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if versionChecked {
			t.Fatal("forced self-update should not check latest version")
		}
		if len(ops.calls) != 0 {
			t.Fatalf("calls = %#v", ops.calls)
		}
		if !strings.Contains(stderr, "does not support npm, pnpm, yarn, or bun self-updates") {
			t.Fatalf("stderr=%q", stderr)
		}
	})

	t.Run("rejects self-update after version check instead of installing npm package names", func(t *testing.T) {
		agentDir, projectDir := createPackageCommandPathDirs(t)
		globalPrefix := filepath.Join(filepath.Dir(agentDir), "global-prefix")
		selfPackageDir := packageCommandSelfPackageDir(t, globalPrefix, "@mariozechner/pi-coding-agent")
		ops := newPackageCommandOps(globalPrefix)
		versionChecks := 0

		_, stderr, code := runPackageCommandCLIWithOptions(t, []string{"update", "--self"}, projectDir, agentDir, func(options *CLIOptions) {
			options.PackageManager = packageCommandManager(projectDir, agentDir, ops.operations())
			options.InstallEnvironment = packageCommandInstallEnvironment(selfPackageDir, globalPrefix)
			options.PackageName = DefaultCodingAgentPackageName
			options.Version = "0.72.0"
			options.VersionCheck = func(string, VersionCheckOptions) (LatestGiRelease, bool) {
				versionChecks++
				return LatestGiRelease{Version: "0.73.0"}, true
			}
		})
		if code != 1 {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if versionChecks != 1 {
			t.Fatalf("version checks = %d", versionChecks)
		}
		if len(ops.calls) != 0 {
			t.Fatalf("calls = %#v", ops.calls)
		}
		if !strings.Contains(stderr, "does not support npm, pnpm, yarn, or bun self-updates") ||
			strings.Contains(stderr, "@new-scope/pi") {
			t.Fatalf("stderr=%q", stderr)
		}
	})

	t.Run("updates a single package with --extension source", func(t *testing.T) {
		agentDir, projectDir := createPackageCommandPathDirs(t)
		source := "git:github.com/gi-packages/formatter"
		stdout, stderr, code := runPackageCommandCLI(t, []string{"update", "--extension", source}, projectDir, agentDir)
		if code != 0 {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if strings.TrimSpace(stdout) != "Updated "+source {
			t.Fatalf("stdout = %q", stdout)
		}
	})

	t.Run("rejects conflicting update extension target forms", func(t *testing.T) {
		_, stderr, code := runPackageCommandCLI(t, []string{"update", "--extension", "official:gi-tools-ui", "official:gi-plan-mode"}, t.TempDir(), t.TempDir())
		if code != 1 {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if !strings.Contains(stderr, "--extension cannot be combined with a positional source") {
			t.Fatalf("stderr = %q", stderr)
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
		Args:     args,
		Stdout:   &stdout,
		Stderr:   &stderr,
		CWD:      projectDir,
		AgentDir: agentDir,
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

type fakeCLIConfigHost struct{}

func (fakeCLIConfigHost) Run() error { return nil }

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
