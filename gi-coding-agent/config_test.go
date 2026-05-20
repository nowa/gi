package gicodingagent

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const testPackageName = "@earendil-works/pi-coding-agent"

func TestDetectInstallMethodAndUpdateInstructions(t *testing.T) {
	t.Run("detects pnpm from Windows .pnpm install paths", func(t *testing.T) {
		env := InstallEnvironment{
			ExecPath: `C:\Users\Admin\Documents\pnpm-repository\global\5\.pnpm\@earendil-works+pi-coding-agent@0.67.68\node_modules\@earendil-works\pi-coding-agent\dist\cli.js`,
		}

		if got := DetectInstallMethod(env); got != InstallMethodPNPM {
			t.Fatalf("method = %q", got)
		}
		if got := GetUpdateInstruction(testPackageName, env); got != "Run: pnpm install -g @earendil-works/pi-coding-agent" {
			t.Fatalf("instruction = %q", got)
		}
	})

	t.Run("does not self-update unknown wrapper installs", func(t *testing.T) {
		env := InstallEnvironment{ExecPath: "/usr/local/bin/node"}

		if got := DetectInstallMethod(env); got != InstallMethodUnknown {
			t.Fatalf("method = %q", got)
		}
		if command := GetSelfUpdateCommand(testPackageName, env, nil, ""); command != nil {
			t.Fatalf("command = %#v", command)
		}
		want := "Update @earendil-works/pi-coding-agent using the package manager, wrapper, or source checkout that provides this installation."
		if got := GetUpdateInstruction(testPackageName, env); got != want {
			t.Fatalf("instruction = %q", got)
		}
	})

	t.Run("self-updates npm installs from custom prefixes", func(t *testing.T) {
		env, prefix := createNPMInstallEnv(t, "pi-prefix-")

		command := GetSelfUpdateCommand(testPackageName, env, nil, "")

		if got := DetectInstallMethod(env); got != InstallMethodNPM {
			t.Fatalf("method = %q", got)
		}
		want := &SelfUpdateCommand{
			Command: "npm",
			Args:    []string{"--prefix", prefix, "install", "-g", testPackageName},
			Display: "npm --prefix " + prefix + " install -g @earendil-works/pi-coding-agent",
		}
		if !reflect.DeepEqual(command, want) {
			t.Fatalf("command = %#v, want %#v", command, want)
		}
	})

	t.Run("self-updates renamed packages from the current install prefix", func(t *testing.T) {
		env, prefix := createNPMInstallEnv(t, "pi-prefix-")

		command := GetSelfUpdateCommand("@mariozechner/pi-coding-agent", env, nil, "@new-scope/pi")

		want := &SelfUpdateCommand{
			Command: "npm",
			Args:    []string{"--prefix", prefix, "install", "-g", "@new-scope/pi"},
			Display: "npm --prefix " + prefix + " uninstall -g @mariozechner/pi-coding-agent && npm --prefix " + prefix + " install -g @new-scope/pi",
			Steps: []SelfUpdateCommandStep{
				{Command: "npm", Args: []string{"--prefix", prefix, "uninstall", "-g", "@mariozechner/pi-coding-agent"}, Display: "npm --prefix " + prefix + " uninstall -g @mariozechner/pi-coding-agent"},
				{Command: "npm", Args: []string{"--prefix", prefix, "install", "-g", "@new-scope/pi"}, Display: "npm --prefix " + prefix + " install -g @new-scope/pi"},
			},
		}
		if !reflect.DeepEqual(command, want) {
			t.Fatalf("command = %#v, want %#v", command, want)
		}
	})

	t.Run("self-update respects configured npmCommand", func(t *testing.T) {
		env, prefix := createNPMInstallEnv(t, "pi-prefix-")
		env.CommandOutput = commandOutputStub(map[string]string{
			commandKey("npm", []string{"--prefix", prefix, "root", "-g"}): filepath.Join(prefix, "lib", "node_modules"),
		})

		command := GetSelfUpdateCommand(testPackageName, env, []string{"npm", "--prefix", prefix}, "")

		want := &SelfUpdateCommand{
			Command: "npm",
			Args:    []string{"--prefix", prefix, "install", "-g", testPackageName},
			Display: "npm --prefix " + prefix + " install -g @earendil-works/pi-coding-agent",
		}
		if !reflect.DeepEqual(command, want) {
			t.Fatalf("command = %#v, want %#v", command, want)
		}
	})

	t.Run("self-update treats empty npmCommand as unset", func(t *testing.T) {
		env, prefix := createNPMInstallEnv(t, "pi-prefix-")

		command := GetSelfUpdateCommand(testPackageName, env, []string{}, "")

		if command == nil || !reflect.DeepEqual(command.Args, []string{"--prefix", prefix, "install", "-g", testPackageName}) {
			t.Fatalf("command = %#v", command)
		}
	})

	t.Run("quotes npm self-update display paths", func(t *testing.T) {
		env, prefix := createNPMInstallEnv(t, "pi prefix ")

		command := GetSelfUpdateCommand(testPackageName, env, nil, "")

		want := `npm --prefix "` + prefix + `" install -g @earendil-works/pi-coding-agent`
		if command == nil || command.Display != want {
			t.Fatalf("display = %#v, want %q", command, want)
		}
	})

	t.Run("does not infer Windows npm custom prefixes from package paths", func(t *testing.T) {
		packageDir := `C:\Users\Admin\npm prefix\node_modules\@earendil-works\pi-coding-agent`
		env := InstallEnvironment{
			PackageDir: packageDir,
			ExecPath:   packageDir + `\dist\cli.js`,
		}

		if got := DetectInstallMethod(env); got != InstallMethodNPM {
			t.Fatalf("method = %q", got)
		}
		if got := GetUpdateInstruction(testPackageName, env); got != "Run: npm install -g @earendil-works/pi-coding-agent" {
			t.Fatalf("instruction = %q", got)
		}
	})
}

func TestSelfUpdateCommandsForPackageManagers(t *testing.T) {
	t.Run("self-updates bun global installs from bun pm bin", func(t *testing.T) {
		env := createBunInstallEnv(t)

		command := GetSelfUpdateCommand(testPackageName, env, nil, "")

		if got := DetectInstallMethod(env); got != InstallMethodBun {
			t.Fatalf("method = %q", got)
		}
		want := &SelfUpdateCommand{Command: "bun", Args: []string{"install", "-g", testPackageName}, Display: "bun install -g @earendil-works/pi-coding-agent"}
		if !reflect.DeepEqual(command, want) {
			t.Fatalf("command = %#v, want %#v", command, want)
		}
	})

	t.Run("self-updates renamed pnpm global installs by removing the old package first", func(t *testing.T) {
		env := createPNPMInstallEnv(t)

		command := GetSelfUpdateCommand("@mariozechner/pi-coding-agent", env, nil, "@new-scope/pi")

		if got := DetectInstallMethod(env); got != InstallMethodPNPM {
			t.Fatalf("method = %q", got)
		}
		want := &SelfUpdateCommand{
			Command: "pnpm",
			Args:    []string{"install", "-g", "@new-scope/pi"},
			Display: "pnpm remove -g @mariozechner/pi-coding-agent && pnpm install -g @new-scope/pi",
			Steps: []SelfUpdateCommandStep{
				{Command: "pnpm", Args: []string{"remove", "-g", "@mariozechner/pi-coding-agent"}, Display: "pnpm remove -g @mariozechner/pi-coding-agent"},
				{Command: "pnpm", Args: []string{"install", "-g", "@new-scope/pi"}, Display: "pnpm install -g @new-scope/pi"},
			},
		}
		if !reflect.DeepEqual(command, want) {
			t.Fatalf("command = %#v, want %#v", command, want)
		}
	})

	t.Run("self-updates renamed yarn global installs by removing the old package first", func(t *testing.T) {
		env := createYarnInstallEnv(t)

		command := GetSelfUpdateCommand("@mariozechner/pi-coding-agent", env, nil, "@new-scope/pi")

		if got := DetectInstallMethod(env); got != InstallMethodYarn {
			t.Fatalf("method = %q", got)
		}
		want := &SelfUpdateCommand{
			Command: "yarn",
			Args:    []string{"global", "add", "@new-scope/pi"},
			Display: "yarn global remove @mariozechner/pi-coding-agent && yarn global add @new-scope/pi",
			Steps: []SelfUpdateCommandStep{
				{Command: "yarn", Args: []string{"global", "remove", "@mariozechner/pi-coding-agent"}, Display: "yarn global remove @mariozechner/pi-coding-agent"},
				{Command: "yarn", Args: []string{"global", "add", "@new-scope/pi"}, Display: "yarn global add @new-scope/pi"},
			},
		}
		if !reflect.DeepEqual(command, want) {
			t.Fatalf("command = %#v, want %#v", command, want)
		}
	})

	t.Run("self-updates renamed bun global installs by removing the old package first", func(t *testing.T) {
		env := createBunInstallEnv(t)

		command := GetSelfUpdateCommand("@mariozechner/pi-coding-agent", env, nil, "@new-scope/pi")

		if got := DetectInstallMethod(env); got != InstallMethodBun {
			t.Fatalf("method = %q", got)
		}
		want := &SelfUpdateCommand{
			Command: "bun",
			Args:    []string{"install", "-g", "@new-scope/pi"},
			Display: "bun uninstall -g @mariozechner/pi-coding-agent && bun install -g @new-scope/pi",
			Steps: []SelfUpdateCommandStep{
				{Command: "bun", Args: []string{"uninstall", "-g", "@mariozechner/pi-coding-agent"}, Display: "bun uninstall -g @mariozechner/pi-coding-agent"},
				{Command: "bun", Args: []string{"install", "-g", "@new-scope/pi"}, Display: "bun install -g @new-scope/pi"},
			},
		}
		if !reflect.DeepEqual(command, want) {
			t.Fatalf("command = %#v, want %#v", command, want)
		}
	})
}

func TestSelfUpdateUnavailableWhenInstallPathIsNotWritable(t *testing.T) {
	env, _ := createNPMInstallEnv(t, "pi-prefix-")
	if err := os.Chmod(env.PackageDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(env.PackageDir, 0o700) })

	if command := GetSelfUpdateCommand(testPackageName, env, nil, ""); command != nil {
		t.Fatalf("command = %#v", command)
	}
	if got := GetSelfUpdateUnavailableInstruction(testPackageName, env, nil, ""); !strings.Contains(got, "the install path is not writable") {
		t.Fatalf("instruction = %q", got)
	}
}

func createNPMInstallEnv(t *testing.T, pattern string) (InstallEnvironment, string) {
	t.Helper()
	prefix, err := os.MkdirTemp(os.TempDir(), pattern)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(prefix) })
	root := filepath.Join(prefix, "lib", "node_modules")
	packageDir := filepath.Join(root, "@earendil-works", "pi-coding-agent")
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
	packageDir := filepath.Join(root, "@mariozechner", "pi-coding-agent")
	if err := os.MkdirAll(packageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return InstallEnvironment{
		PackageDir: packageDir,
		ExecPath: filepath.Join(root, ".pnpm", "@mariozechner+pi-coding-agent@0.0.0", "node_modules",
			"@mariozechner", "pi-coding-agent", "dist", "cli.js"),
		HomeDir: t.TempDir(),
		CommandOutput: commandOutputStub(map[string]string{
			commandKey("pnpm", []string{"root", "-g"}): root,
		}),
	}
}

func createYarnInstallEnv(t *testing.T) InstallEnvironment {
	t.Helper()
	temp := t.TempDir()
	globalDir := filepath.Join(temp, "yarn", "global")
	packageDir := filepath.Join(globalDir, "node_modules", "@mariozechner", "pi-coding-agent")
	if err := os.MkdirAll(packageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return InstallEnvironment{
		PackageDir: packageDir,
		ExecPath:   filepath.Join(globalDir, ".yarn", "@mariozechner", "pi-coding-agent", "dist", "cli.js"),
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
	packageDir := filepath.Join(root, "@earendil-works", "pi-coding-agent")
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
