package gicodingagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPackageName = "gi"

func TestDetectInstallMethodKeepsLegacyNodeInstallSignals(t *testing.T) {
	t.Run("detects pnpm from Windows .pnpm install paths without self-update support", func(t *testing.T) {
		env := InstallEnvironment{
			ExecPath: `C:\Users\Admin\Documents\pnpm-repository\global\5\.pnpm\gi@0.67.68\node_modules\gi\dist\cli.js`,
		}

		if got := DetectInstallMethod(env); got != InstallMethodPNPM {
			t.Fatalf("method = %q", got)
		}
		assertNodeSelfUpdateUnsupported(t, env)
	})

	t.Run("detects npm global paths without generating npm commands", func(t *testing.T) {
		env, _ := createNPMInstallEnv(t, "gi-prefix-")
		if got := DetectInstallMethod(env); got != InstallMethodNPM {
			t.Fatalf("method = %q", got)
		}
		assertNodeSelfUpdateUnsupported(t, env)
	})

	t.Run("detects bun global paths without generating bun commands", func(t *testing.T) {
		env := createBunInstallEnv(t)
		if got := DetectInstallMethod(env); got != InstallMethodBun {
			t.Fatalf("method = %q", got)
		}
		assertNodeSelfUpdateUnsupported(t, env)
	})

	t.Run("does not self-update unknown wrapper installs", func(t *testing.T) {
		env := InstallEnvironment{ExecPath: "/usr/local/bin/gi"}

		if got := DetectInstallMethod(env); got != InstallMethodUnknown {
			t.Fatalf("method = %q", got)
		}
		if command := GetSelfUpdateCommand(testPackageName, env, nil, ""); command != nil {
			t.Fatalf("command = %#v", command)
		}
		want := "Update gi using the package manager, wrapper, or source checkout that provides this installation."
		if got := GetUpdateInstruction(testPackageName, env); got != want {
			t.Fatalf("instruction = %q", got)
		}
	})
}

func TestSelfUpdateCommandsDoNotUseNodePackageManagers(t *testing.T) {
	cases := []struct {
		name string
		env  InstallEnvironment
		want InstallMethod
	}{
		{name: "npm", env: mustNPMInstallEnv(t), want: InstallMethodNPM},
		{name: "pnpm", env: createPNPMInstallEnv(t), want: InstallMethodPNPM},
		{name: "yarn", env: createYarnInstallEnv(t), want: InstallMethodYarn},
		{name: "bun", env: createBunInstallEnv(t), want: InstallMethodBun},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetectInstallMethod(tc.env); got != tc.want {
				t.Fatalf("method = %q, want %q", got, tc.want)
			}
			assertNodeSelfUpdateUnsupported(t, tc.env)
		})
	}
}

func TestBunBinaryUpdateInstructionUsesGiReleases(t *testing.T) {
	env := InstallEnvironment{BunBinary: true}
	if command := GetSelfUpdateCommand(testPackageName, env, nil, ""); command != nil {
		t.Fatalf("command = %#v", command)
	}
	if got := GetUpdateInstruction(testPackageName, env); got != "Download from: https://github.com/nowa/gi/releases/latest" {
		t.Fatalf("instruction = %q", got)
	}
}

func TestDefaultInstallEnvironmentPrefersGiPackageDir(t *testing.T) {
	t.Setenv("GI_PACKAGE_DIR", "/gi/package")
	t.Setenv("PI_PACKAGE_DIR", "/pi/package")
	if got := DefaultInstallEnvironment().PackageDir; got != "/gi/package" {
		t.Fatalf("package dir = %q", got)
	}
}

func assertNodeSelfUpdateUnsupported(t *testing.T, env InstallEnvironment) {
	t.Helper()
	if command := GetSelfUpdateCommand(testPackageName, env, []string{"fake-npm"}, ""); command != nil {
		t.Fatalf("command = %#v", command)
	}
	instruction := GetUpdateInstruction(testPackageName, env)
	for _, expected := range []string{"does not support npm, pnpm, yarn, or bun self-updates", "Update gi"} {
		if !strings.Contains(instruction, expected) {
			t.Fatalf("instruction = %q, want %q", instruction, expected)
		}
	}
}

func mustNPMInstallEnv(t *testing.T) InstallEnvironment {
	t.Helper()
	env, _ := createNPMInstallEnv(t, "gi-prefix-")
	return env
}

func createNPMInstallEnv(t *testing.T, pattern string) (InstallEnvironment, string) {
	t.Helper()
	prefix, err := os.MkdirTemp(os.TempDir(), pattern)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(prefix) })
	root := filepath.Join(prefix, "lib", "node_modules")
	packageDir := filepath.Join(root, "gi")
	if err := os.MkdirAll(packageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return InstallEnvironment{
		PackageDir: packageDir,
		ExecPath:   filepath.Join(packageDir, "dist", "cli.js"),
		HomeDir:    filepath.Dir(prefix),
	}, prefix
}

func createPNPMInstallEnv(t *testing.T) InstallEnvironment {
	t.Helper()
	temp := t.TempDir()
	root := filepath.Join(temp, "pnpm", "global", "5", "node_modules")
	packageDir := filepath.Join(root, "gi")
	if err := os.MkdirAll(packageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return InstallEnvironment{
		PackageDir: packageDir,
		ExecPath:   filepath.Join(root, ".pnpm", "gi@0.0.0", "node_modules", "gi", "dist", "cli.js"),
		HomeDir:    t.TempDir(),
		CommandOutput: commandOutputStub(map[string]string{
			commandKey("pnpm", []string{"root", "-g"}): root,
		}),
	}
}

func createYarnInstallEnv(t *testing.T) InstallEnvironment {
	t.Helper()
	temp := t.TempDir()
	globalDir := filepath.Join(temp, "yarn", "global")
	packageDir := filepath.Join(globalDir, "node_modules", "gi")
	if err := os.MkdirAll(packageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return InstallEnvironment{
		PackageDir: packageDir,
		ExecPath:   filepath.Join(globalDir, ".yarn", "gi", "dist", "cli.js"),
		HomeDir:    t.TempDir(),
		CommandOutput: commandOutputStub(map[string]string{
			commandKey("yarn", []string{"global", "dir"}): globalDir,
		}),
	}
}

func createBunInstallEnv(t *testing.T) InstallEnvironment {
	t.Helper()
	temp := t.TempDir()
	prefix := filepath.Join(temp, ".bun")
	bunBin := filepath.Join(prefix, "bin")
	root := filepath.Join(prefix, "install", "global", "node_modules")
	packageDir := filepath.Join(root, "gi")
	if err := os.MkdirAll(packageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bunBin, 0o700); err != nil {
		t.Fatal(err)
	}
	return InstallEnvironment{
		PackageDir: packageDir,
		ExecPath:   filepath.Join(packageDir, "dist", "cli.js"),
		HomeDir:    temp,
		CommandOutput: commandOutputStub(map[string]string{
			commandKey("bun", []string{"pm", "bin", "-g"}): bunBin,
		}),
	}
}

func commandOutputStub(outputs map[string]string) func(string, []string, bool) (string, bool, error) {
	return func(command string, args []string, requireSuccess bool) (string, bool, error) {
		value, ok := outputs[commandKey(command, args)]
		if !ok {
			return "", false, nil
		}
		return value, value != "", nil
	}
}

func commandKey(command string, args []string) string {
	return command + "\x00" + strings.Join(args, "\x00")
}
