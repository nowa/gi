package gicodingagent

import (
	"bytes"
	"os"
	"path/filepath"
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
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunCLI(CLIOptions{
		Args:   args,
		Stdout: &stdout,
		Stderr: &stderr,
		PackageManager: NewDefaultPackageManager(PackageManagerOptions{
			CWD:      projectDir,
			AgentDir: agentDir,
		}),
	})
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
