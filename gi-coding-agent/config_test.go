package gicodingagent

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestNormalizeSelfUpdatePackageTargetKeepsIdentityAndInstallSpecSeparate(t *testing.T) {
	t.Run("defaults the install spec to the package identity", func(t *testing.T) {
		got := normalizeSelfUpdatePackageTarget(SelfUpdatePackageTarget{PackageName: " gi "})
		want := SelfUpdatePackageTarget{PackageName: "gi", InstallSpec: "gi"}
		if got != want {
			t.Fatalf("target = %#v, want %#v", got, want)
		}
	})

	t.Run("preserves an exact install spec", func(t *testing.T) {
		target := SelfUpdatePackageTarget{
			PackageName: "gi",
			InstallSpec: "gi@0.82.0",
		}
		if got := normalizeSelfUpdatePackageTarget(target); got != target {
			t.Fatalf("target = %#v, want %#v", got, target)
		}
		instruction := GetSelfUpdateUnavailableInstructionForTarget(
			"legacy-gi",
			InstallEnvironment{ExecPath: "/usr/local/bin/gi"},
			target,
		)
		if !strings.Contains(instruction, "Update gi@0.82.0") {
			t.Fatalf("instruction = %q", instruction)
		}
	})

	t.Run("the public target boundary honors the Go zero value", func(t *testing.T) {
		instruction := GetSelfUpdateUnavailableInstructionForTarget(
			"gi",
			InstallEnvironment{ExecPath: "/usr/local/bin/gi"},
			SelfUpdatePackageTarget{},
		)
		if !strings.Contains(instruction, "Update gi ") {
			t.Fatalf("instruction = %q", instruction)
		}
	})
}

func TestGetPathComparisonCandidatesPreservesLexicalAndRealPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks is not reliable on Windows CI")
	}
	realDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	linkRoot := t.TempDir()
	link := filepath.Join(linkRoot, "package-link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}

	got := getPathComparisonCandidates(link, runtime.GOOS)
	wantLexical, err := filepath.Abs(link)
	if err != nil {
		t.Fatal(err)
	}
	wantReal, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{wantLexical, wantReal} {
		if !configTestContainsString(got, want) {
			t.Fatalf("candidates = %#v, missing %q", got, want)
		}
	}
	if got := getPathComparisonCandidates(filepath.Join(linkRoot, "missing"), runtime.GOOS); len(got) != 0 {
		t.Fatalf("missing candidates = %#v", got)
	}
}

func TestGetPathComparisonCandidatesUsesWindowsCasePolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "MixedCase")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range getPathComparisonCandidates(path, "windows") {
		if candidate != strings.ToLower(candidate) {
			t.Fatalf("candidate = %q, want lowercase", candidate)
		}
	}
}

func TestGetEntrypointPackageDirFindsNearestGoModule(t *testing.T) {
	root := t.TempDir()
	writeConfigTestFile(t, filepath.Join(root, "go.mod"), "module example.com/root\n")
	nestedModule := filepath.Join(root, "tools", "gi")
	writeConfigTestFile(t, filepath.Join(nestedModule, "go.mod"), "module example.com/gi\n")
	entrypoint := filepath.Join(nestedModule, "cmd", "gi", "main")
	writeConfigTestFile(t, entrypoint, "")

	if got := getEntrypointPackageDir(entrypoint); got != nestedModule {
		t.Fatalf("package dir = %q, want %q", got, nestedModule)
	}
	if got := getEntrypointPackageDir(filepath.Join(t.TempDir(), "bin", "gi")); got != "" {
		t.Fatalf("package dir = %q, want empty", got)
	}
}

func TestDetectInstallMethodFollowsEntrypointSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks is not reliable on Windows CI")
	}
	packageDir := filepath.Join(t.TempDir(), "lib", "node_modules", "gi")
	entrypoint := filepath.Join(packageDir, "bin", "gi")
	writeConfigTestFile(t, entrypoint, "")
	linkDir := t.TempDir()
	link := filepath.Join(linkDir, "gi")
	if err := os.Symlink(entrypoint, link); err != nil {
		t.Fatal(err)
	}

	if got := DetectInstallMethod(InstallEnvironment{ExecPath: link}); got != InstallMethodNPM {
		t.Fatalf("method = %q, want %q", got, InstallMethodNPM)
	}
}

func TestDefaultInstallEnvironmentPrefersGiPackageDir(t *testing.T) {
	t.Setenv("GI_PACKAGE_DIR", "/gi/package")
	t.Setenv("PI_PACKAGE_DIR", "/pi/package")
	if got := DefaultInstallEnvironment().PackageDir; got != "/gi/package" {
		t.Fatalf("package dir = %q", got)
	}
}

func TestConfigPathHelpersUseGiNamesPiStyle(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), "agent")
	t.Setenv(EnvCodingAgentDir, agentDir)
	t.Setenv(LegacyEnvCodingAgentDir, filepath.Join(t.TempDir(), "pi-agent"))

	if got := GetAgentDir(); got != agentDir {
		t.Fatalf("agent dir = %q, want %q", got, agentDir)
	}
	assertPath(t, GetCustomThemesDir(), agentDir, "themes")
	assertPath(t, GetModelsPath(), agentDir, "models.json")
	assertPath(t, GetAuthPath(), agentDir, "auth.json")
	assertPath(t, GetSettingsPath(), agentDir, "settings.json")
	assertPath(t, GetToolsDir(), agentDir, "tools")
	assertPath(t, GetBinDir(), agentDir, "bin")
	assertPath(t, GetPromptsDir(), agentDir, "prompts")
	assertPath(t, GetSessionsDir(), agentDir, "sessions")
	assertPath(t, GetDebugLogPath(), agentDir, "gi-debug.log")
}

func TestConfigPathHelpersKeepLegacyPiFallback(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), "legacy-agent")
	t.Setenv(EnvCodingAgentDir, "")
	t.Setenv(LegacyEnvCodingAgentDir, agentDir)

	if got := GetAgentDir(); got != agentDir {
		t.Fatalf("agent dir = %q, want legacy %q", got, agentDir)
	}
}

func TestPackageAssetPathHelpersUseGoPackageLayout(t *testing.T) {
	packageDir := filepath.Join(t.TempDir(), "gi")
	env := InstallEnvironment{PackageDir: packageDir}

	assertPath(t, GetPackageDir(env), packageDir)
	assertPath(t, GetThemesDir(env), packageDir, "gi-coding-agent", "themes")
	assertPath(t, GetExportTemplateDir(env), packageDir, "gi-coding-agent", "export-html")
	assertPath(t, GetPackageJSONPath(env), packageDir, "go.mod")
	assertPath(t, GetPackageJsonPath(env), packageDir, "go.mod")
	assertPath(t, GetReadmePath(env), packageDir, "README.md")
	assertPath(t, GetDocsPath(env), packageDir, "docs")
	assertPath(t, GetExamplesPath(env), packageDir, "examples")
	assertPath(t, GetChangelogPath(env), packageDir, "CHANGELOG.md")
	assertPath(t, GetInteractiveAssetsDir(env), packageDir, "gi-coding-agent", "assets")
	assertPath(t, GetBundledInteractiveAssetPath("clankolas.png", env), packageDir, "gi-coding-agent", "assets", "clankolas.png")
}

func TestPackageDirExpandsTildeWithInstallEnvironmentHome(t *testing.T) {
	home := t.TempDir()
	env := InstallEnvironment{PackageDir: "~/pkg", HomeDir: home}

	assertPath(t, GetPackageDir(env), home, "pkg")
}

func TestShareViewerURLUsesGiEnvPiStyle(t *testing.T) {
	t.Setenv("GI_SHARE_VIEWER_URL", "https://share.example/session/")
	t.Setenv("PI_SHARE_VIEWER_URL", "https://pi.example/session/")

	if got := GetShareViewerURL("abc123"); got != "https://share.example/session/#abc123" {
		t.Fatalf("share url = %q", got)
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
		ExecPath:   filepath.Join(root, ".pnpm", "gi@"+DefaultCodingAgentVersion, "node_modules", "gi", "dist", "cli.js"),
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

func assertPath(t *testing.T, got string, parts ...string) {
	t.Helper()
	want := filepath.Join(parts...)
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func configTestContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func writeConfigTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
