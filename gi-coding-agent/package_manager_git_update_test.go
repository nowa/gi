package gicodingagent

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

const packageManagerGitSource = "git:github.com/test/extension"

func TestDefaultPackageManagerGitUpdatePiNormalAndForcePush(t *testing.T) {
	requireGit(t)
	env := newPackageManagerGitTestEnv(t)

	t.Run("skips reset clean and install when already up to date", func(t *testing.T) {
		env.reset(t)
		mkdirAll(t, env.remoteDir)
		initPackageManagerGitRepo(t, env.remoteDir)
		writeFile(t, filepath.Join(env.remoteDir, "package.json"), `{"name":"test-extension","version":"1.0.0"}`)
		createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v1", "Initial commit")
		clonePackageManagerRepo(t, env)
		env.settingsManager.SetPackages([]any{packageManagerGitSource})

		var executed []string
		manager := env.managerWithOperations(PackageManagerOperations{
			RunCommand: func(command string, args []string, options PackageCommandOptions) error {
				executed = append(executed, command+" "+strings.Join(args, " "))
				return runPackageCommand(command, args, options)
			},
		})

		if err := manager.Update(); err != nil {
			t.Fatal(err)
		}
		wantFetch := "git fetch --prune --no-tags origin +refs/heads/main:refs/remotes/origin/main"
		if !containsString(executed, wantFetch) {
			t.Fatalf("commands = %#v, want %q", executed, wantFetch)
		}
		for _, forbidden := range []string{"git fetch --prune origin", "git reset --hard @{upstream}", "git reset --hard origin/HEAD", "git clean -fdx", "npm install"} {
			if containsString(executed, forbidden) {
				t.Fatalf("commands = %#v, should not contain %q", executed, forbidden)
			}
		}
	})

	t.Run("updates to latest commit", func(t *testing.T) {
		env.reset(t)
		env.setupRemoteAndInstall(t, "")
		newCommit := createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v2", "Second commit")
		if err := env.manager.Update(); err != nil {
			t.Fatal(err)
		}
		if got := packageManagerGitHead(t, env.installedDir); got != newCommit {
			t.Fatalf("head = %s, want %s", got, newCommit)
		}
		if got := readFile(t, filepath.Join(env.installedDir, "extension.ts")); got != "// v2" {
			t.Fatalf("content = %q", got)
		}
	})

	t.Run("checks available git package updates without resetting checkout", func(t *testing.T) {
		env.reset(t)
		env.setupRemoteAndInstall(t, "")
		initialHead := packageManagerGitHead(t, env.installedDir)
		createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v2", "Second commit")

		updates, err := env.manager.CheckForAvailableUpdates()
		if err != nil {
			t.Fatal(err)
		}
		if len(updates) != 1 || updates[0].DisplayName != "github.com/test/extension" || updates[0].Type != "git" {
			t.Fatalf("updates = %#v", updates)
		}
		if got := packageManagerGitHead(t, env.installedDir); got != initialHead {
			t.Fatalf("check should not mutate checkout head = %s, want %s", got, initialHead)
		}
	})

	t.Run("handles multiple commits ahead", func(t *testing.T) {
		env.reset(t)
		env.setupRemoteAndInstall(t, "")
		createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v2", "Second commit")
		createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v3", "Third commit")
		latestCommit := createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v4", "Fourth commit")
		if err := env.manager.Update(); err != nil {
			t.Fatal(err)
		}
		if got := packageManagerGitHead(t, env.installedDir); got != latestCommit {
			t.Fatalf("head = %s, want %s", got, latestCommit)
		}
		if got := readFile(t, filepath.Join(env.installedDir, "extension.ts")); got != "// v4" {
			t.Fatalf("content = %q", got)
		}
	})

	t.Run("updates even when checkout has no upstream", func(t *testing.T) {
		env.reset(t)
		env.setupRemoteAndInstall(t, "")
		createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v2", "Second commit")
		latestCommit := createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v3", "Third commit")
		detachedCommit := packageManagerGitHead(t, env.installedDir)
		gitForPackageManager(t, []string{"checkout", detachedCommit}, env.installedDir)

		if err := env.manager.Update(); err != nil {
			t.Fatal(err)
		}
		if got := packageManagerGitHead(t, env.installedDir); got != latestCommit {
			t.Fatalf("head = %s, want %s", got, latestCommit)
		}
	})

	t.Run("recovers when remote history is rewritten", func(t *testing.T) {
		env.reset(t)
		env.setupRemoteAndInstall(t, "")
		initialCommit := packageManagerGitHead(t, env.remoteDir)
		createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v2", "Commit to keep")
		if err := env.manager.Update(); err != nil {
			t.Fatal(err)
		}
		gitForPackageManager(t, []string{"reset", "--hard", initialCommit}, env.remoteDir)
		rewrittenCommit := createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v2-rewritten", "Rewritten commit")

		if err := env.manager.Update(); err != nil {
			t.Fatal(err)
		}
		if got := packageManagerGitHead(t, env.installedDir); got != rewrittenCommit {
			t.Fatalf("head = %s, want %s", got, rewrittenCommit)
		}
		if got := readFile(t, filepath.Join(env.installedDir, "extension.ts")); got != "// v2-rewritten" {
			t.Fatalf("content = %q", got)
		}
	})

	t.Run("recovers when local commit no longer exists in remote", func(t *testing.T) {
		env.reset(t)
		env.setupRemoteAndInstall(t, "")
		createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v2", "Commit A")
		createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v3", "Commit B")
		if err := env.manager.Update(); err != nil {
			t.Fatal(err)
		}
		gitForPackageManager(t, []string{"reset", "--hard", "HEAD~2"}, env.remoteDir)
		newCommit := createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v2-new", "New commit replacing A and B")

		if err := env.manager.Update(); err != nil {
			t.Fatal(err)
		}
		if got := packageManagerGitHead(t, env.installedDir); got != newCommit {
			t.Fatalf("head = %s, want %s", got, newCommit)
		}
		if got := readFile(t, filepath.Join(env.installedDir, "extension.ts")); got != "// v2-new" {
			t.Fatalf("content = %q", got)
		}
	})

	t.Run("handles complete history rewrite", func(t *testing.T) {
		env.reset(t)
		env.setupRemoteAndInstall(t, "")
		createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v2", "v2")
		createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v3", "v3")
		if err := env.manager.Update(); err != nil {
			t.Fatal(err)
		}
		gitForPackageManager(t, []string{"reset", "--hard", "HEAD~2"}, env.remoteDir)
		createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// rewrite-a", "Rewrite A")
		finalCommit := createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// rewrite-b", "Rewrite B")

		if err := env.manager.Update(); err != nil {
			t.Fatal(err)
		}
		if got := packageManagerGitHead(t, env.installedDir); got != finalCommit {
			t.Fatalf("head = %s, want %s", got, finalCommit)
		}
		if got := readFile(t, filepath.Join(env.installedDir, "extension.ts")); got != "// rewrite-b" {
			t.Fatalf("content = %q", got)
		}
	})
}

func TestDefaultPackageManagerGitUpdatePiPinnedTemporaryAndScope(t *testing.T) {
	requireGit(t)
	env := newPackageManagerGitTestEnv(t)

	t.Run("does not update pinned git sources", func(t *testing.T) {
		env.reset(t)
		mkdirAll(t, env.remoteDir)
		initPackageManagerGitRepo(t, env.remoteDir)
		initialCommit := createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v1", "Initial commit")
		clonePackageManagerRepo(t, env)
		gitForPackageManager(t, []string{"checkout", initialCommit}, env.installedDir)
		env.settingsManager.SetPackages([]any{packageManagerGitSource + "@" + initialCommit})
		createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v2", "Second commit")

		if err := env.manager.Update(); err != nil {
			t.Fatal(err)
		}
		if got := packageManagerGitHead(t, env.installedDir); got != initialCommit {
			t.Fatalf("head = %s, want %s", got, initialCommit)
		}
		if got := readFile(t, filepath.Join(env.installedDir, "extension.ts")); got != "// v1" {
			t.Fatalf("content = %q", got)
		}
	})

	t.Run("refreshes cached temporary git sources when resolving", func(t *testing.T) {
		env.reset(t)
		cachedDir := packageManagerTemporaryGitDir(env.agentDir, "github.com", "test/extension")
		extensionFile := filepath.Join(cachedDir, "pi-extensions", "session-breakdown.ts")
		removeAll(t, cachedDir)
		mkdirAll(t, filepath.Join(cachedDir, "pi-extensions"))
		writeFile(t, filepath.Join(cachedDir, "package.json"), `{"pi":{"extensions":["./pi-extensions"]}}`)
		writeFile(t, extensionFile, "// stale")
		t.Cleanup(func() { _ = os.RemoveAll(cachedDir) })

		var executed []string
		manager := env.managerWithOperations(PackageManagerOperations{
			RunCommand: func(command string, args []string, _ PackageCommandOptions) error {
				executed = append(executed, command+" "+strings.Join(args, " "))
				if command == "git" && len(args) > 0 && args[0] == "reset" {
					writeFile(t, extensionFile, "// fresh")
				}
				return nil
			},
			RunCommandCapture: func(_ string, args []string, _ PackageCommandOptions) (string, error) {
				if reflect.DeepEqual(args, []string{"rev-parse", "HEAD"}) {
					return "local-head", nil
				}
				if reflect.DeepEqual(args, []string{"rev-parse", "origin/main^{commit}"}) {
					return "remote-head", nil
				}
				return "", nil
			},
		})

		if _, err := manager.ResolveExtensionSources([]string{packageManagerGitSource}, ResolveExtensionSourcesOptions{Temporary: true}); err != nil {
			t.Fatal(err)
		}
		wantFetch := "git fetch --prune --no-tags origin +refs/heads/main:refs/remotes/origin/main"
		if !containsString(executed, wantFetch) {
			t.Fatalf("commands = %#v, want %q", executed, wantFetch)
		}
		if got := readFile(t, extensionFile); got != "// fresh" {
			t.Fatalf("extension = %q", got)
		}
	})

	t.Run("does not refresh pinned temporary git sources", func(t *testing.T) {
		env.reset(t)
		cachedDir := packageManagerTemporaryGitDir(env.agentDir, "github.com", "test/extension")
		extensionFile := filepath.Join(cachedDir, "pi-extensions", "session-breakdown.ts")
		removeAll(t, cachedDir)
		mkdirAll(t, filepath.Join(cachedDir, "pi-extensions"))
		writeFile(t, filepath.Join(cachedDir, "package.json"), `{"pi":{"extensions":["./pi-extensions"]}}`)
		writeFile(t, extensionFile, "// pinned")
		t.Cleanup(func() { _ = os.RemoveAll(cachedDir) })

		var executed []string
		manager := env.managerWithOperations(PackageManagerOperations{
			RunCommand: func(command string, args []string, _ PackageCommandOptions) error {
				executed = append(executed, command+" "+strings.Join(args, " "))
				return nil
			},
		})
		if _, err := manager.ResolveExtensionSources([]string{packageManagerGitSource + "@main"}, ResolveExtensionSourcesOptions{Temporary: true}); err != nil {
			t.Fatal(err)
		}
		if len(executed) != 0 {
			t.Fatalf("commands = %#v, want none", executed)
		}
		if got := readFile(t, extensionFile); got != "// pinned" {
			t.Fatalf("extension = %q", got)
		}
	})

	t.Run("skips refreshing temporary git sources when offline", func(t *testing.T) {
		env.reset(t)
		t.Setenv("GI_OFFLINE", "1")
		cachedDir := packageManagerTemporaryGitDir(env.agentDir, "github.com", "test/extension")
		removeAll(t, cachedDir)
		mkdirAll(t, filepath.Join(cachedDir, "extensions"))
		writeFile(t, filepath.Join(cachedDir, "extensions", "index.gi.json"), `{"gi":{"extensionProtocol":"jsonl-rpc.v1"}}`)
		t.Cleanup(func() { _ = os.RemoveAll(cachedDir) })

		manager := env.managerWithOperations(PackageManagerOperations{
			RunCommand: func(command string, args []string, _ PackageCommandOptions) error {
				t.Fatalf("unexpected command while offline: %s %v", command, args)
				return nil
			},
		})
		resolved, err := manager.ResolveExtensionSources([]string{packageManagerGitSource}, ResolveExtensionSourcesOptions{Temporary: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(resolved) != 1 || resolved[0] != cachedDir {
			t.Fatalf("resolved = %#v, want %q", resolved, cachedDir)
		}
	})

	t.Run("skips missing package sources when offline", func(t *testing.T) {
		env.reset(t)
		t.Setenv("GI_OFFLINE", "1")
		cachedDir := packageManagerTemporaryGitDir(env.agentDir, "github.com", "test/extension")
		removeAll(t, cachedDir)
		t.Cleanup(func() { _ = os.RemoveAll(cachedDir) })

		manager := env.managerWithOperations(PackageManagerOperations{
			RunCommand: func(command string, args []string, _ PackageCommandOptions) error {
				t.Fatalf("unexpected command while offline: %s %v", command, args)
				return nil
			},
		})
		resolved, err := manager.ResolveExtensionSources([]string{packageManagerGitSource}, ResolveExtensionSourcesOptions{Temporary: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(resolved) != 0 {
			t.Fatalf("resolved = %#v, want empty", resolved)
		}
	})

	t.Run("passes git commands as argv with cwd containing spaces", func(t *testing.T) {
		env.reset(t)
		packageDir := filepath.Join(env.tempDir, "repo with spaces")
		mkdirAll(t, packageDir)
		type call struct {
			command string
			args    []string
			cwd     string
		}
		var calls []call
		manager := env.managerWithOperations(PackageManagerOperations{
			RunCommand: func(command string, args []string, options PackageCommandOptions) error {
				calls = append(calls, call{command: command, args: append([]string(nil), args...), cwd: options.CWD})
				return nil
			},
			RunCommandCapture: func(command string, args []string, options PackageCommandOptions) (string, error) {
				calls = append(calls, call{command: command, args: append([]string(nil), args...), cwd: options.CWD})
				if reflect.DeepEqual(args, []string{"rev-parse", "HEAD"}) {
					return "local-head", nil
				}
				if reflect.DeepEqual(args, []string{"rev-parse", "origin/main^{commit}"}) {
					return "remote-head", nil
				}
				return "", nil
			},
		})

		if err := manager.refreshGitPackage(packageDir); err != nil {
			t.Fatal(err)
		}
		if len(calls) == 0 {
			t.Fatal("expected git calls")
		}
		for _, got := range calls {
			if got.command != "git" || got.cwd != packageDir {
				t.Fatalf("call = %#v, want command git with cwd %q", got, packageDir)
			}
			for _, arg := range got.args {
				if strings.Contains(arg, packageDir) {
					t.Fatalf("package dir should be passed via cwd, not shell args: %#v", got)
				}
			}
		}
	})

	t.Run("does not install locally when source is only registered globally", func(t *testing.T) {
		env.reset(t)
		env.setupRemoteAndInstall(t, "")
		createPackageManagerCommit(t, env.remoteDir, "extension.ts", "// v2", "Second commit")
		projectGitDir := filepath.Join(env.tempDir, ".gi", "git", "github.com", "test", "extension")
		if _, err := os.Stat(projectGitDir); !os.IsNotExist(err) {
			t.Fatalf("project git dir exists before update: %v", err)
		}
		if err := env.manager.Update(packageManagerGitSource); err != nil {
			t.Fatal(err)
		}
		if got := readFile(t, filepath.Join(env.installedDir, "extension.ts")); got != "// v2" {
			t.Fatalf("global content = %q", got)
		}
		if _, err := os.Stat(projectGitDir); !os.IsNotExist(err) {
			t.Fatalf("project git dir exists after update: %v", err)
		}
	})
}

func TestDefaultPackageManagerReconcilesExistingPinnedGitRef(t *testing.T) {
	agentDir := t.TempDir()
	source := GitSource{
		Repo:   "https://github.com/test/extension",
		Host:   "github.com",
		Path:   "test/extension",
		Ref:    "v2",
		Pinned: true,
	}
	targetDir := filepath.Join(agentDir, "git", "github.com", "test", "extension")
	mkdirAll(t, targetDir)

	type commandCall struct {
		args []string
		cwd  string
	}
	var commands []commandCall
	manager := NewDefaultPackageManager(PackageManagerOptions{
		CWD:      t.TempDir(),
		AgentDir: agentDir,
		Operations: PackageManagerOperations{
			RunCommand: func(command string, args []string, options PackageCommandOptions) error {
				if command != "git" {
					t.Fatalf("command = %q", command)
				}
				commands = append(commands, commandCall{args: append([]string(nil), args...), cwd: options.CWD})
				return nil
			},
			RunCommandCapture: func(command string, args []string, options PackageCommandOptions) (string, error) {
				if command != "git" || options.CWD != targetDir {
					t.Fatalf("capture = %q %#v cwd=%q", command, args, options.CWD)
				}
				switch {
				case reflect.DeepEqual(args, []string{"rev-parse", "HEAD"}):
					return "old-head", nil
				case reflect.DeepEqual(args, []string{"rev-parse", "FETCH_HEAD^{commit}"}):
					return "new-head", nil
				default:
					t.Fatalf("unexpected capture args: %#v", args)
					return "", nil
				}
			},
		},
	})

	if err := manager.installGitPackage(source, false); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"fetch", "origin", "v2"},
		{"reset", "--hard", "FETCH_HEAD^{commit}"},
		{"clean", "-fdx"},
	}
	if len(commands) != len(want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
	for index, args := range want {
		if !reflect.DeepEqual(commands[index].args, args) || commands[index].cwd != targetDir {
			t.Fatalf("command[%d] = %#v, want args=%#v cwd=%q", index, commands[index], args, targetDir)
		}
	}
}

func TestPackageInstallLocatorContainsManagedPaths(t *testing.T) {
	agentDir := t.TempDir()
	tempFolder := filepath.Join(agentDir, "tmp", "extensions")
	if err := os.MkdirAll(tempFolder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tempFolder, 0o755); err != nil {
		t.Fatal(err)
	}
	gotTempFolder, err := GetExtensionTempFolder(agentDir)
	if err != nil {
		t.Fatal(err)
	}
	if gotTempFolder != tempFolder {
		t.Fatalf("temp folder = %q, want %q", gotTempFolder, tempFolder)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(tempFolder)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("temp folder mode = %#o, want 0700", got)
		}
	}

	locator := packageInstallLocator{cwd: t.TempDir(), agentDir: agentDir}
	unsafeSource := GitSource{Host: "evil.example", Path: "../../victim/repo"}
	for _, scope := range []packageInstallScope{
		packageInstallScopeUser,
		packageInstallScopeProject,
		packageInstallScopeTemporary,
	} {
		if path, err := locator.gitInstallPath(unsafeSource, scope); err == nil ||
			!strings.Contains(err.Error(), "outside package install root") {
			t.Fatalf("scope %d path = %q, err = %v", scope, path, err)
		}
	}
	if runtime.GOOS != "windows" {
		outside := t.TempDir()
		symlink := filepath.Join(agentDir, "git", "linked-host")
		if err := os.MkdirAll(filepath.Dir(symlink), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, symlink); err != nil {
			t.Fatal(err)
		}
		if path, err := resolveManagedPath(filepath.Join(agentDir, "git"), "linked-host", "package"); err == nil ||
			!strings.Contains(err.Error(), "outside package install root") {
			t.Fatalf("symlink path = %q, err = %v", path, err)
		}
	}

	validSource := GitSource{Host: "github.com", Path: "test/extension"}
	temporaryPath, err := locator.gitInstallPath(validSource, packageInstallScopeTemporary)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(tempFolder, temporaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("temporary path = %q, want under %q", temporaryPath, tempFolder)
	}
}

type packageManagerGitTestEnv struct {
	tempDir         string
	remoteDir       string
	agentDir        string
	installedDir    string
	settingsManager *SettingsManager
	manager         *DefaultPackageManager
}

func newPackageManagerGitTestEnv(t *testing.T) *packageManagerGitTestEnv {
	t.Helper()
	env := &packageManagerGitTestEnv{tempDir: t.TempDir()}
	env.reset(t)
	return env
}

func (e *packageManagerGitTestEnv) reset(t *testing.T) {
	t.Helper()
	removeAll(t, e.tempDir)
	mkdirAll(t, e.tempDir)
	e.remoteDir = filepath.Join(e.tempDir, "remote")
	e.agentDir = filepath.Join(e.tempDir, "agent")
	e.installedDir = filepath.Join(e.agentDir, "git", "github.com", "test", "extension")
	mkdirAll(t, e.agentDir)
	e.settingsManager = NewInMemorySettingsManager(nil)
	e.manager = e.managerWithOperations(PackageManagerOperations{})
}

func (e *packageManagerGitTestEnv) managerWithOperations(operations PackageManagerOperations) *DefaultPackageManager {
	return NewDefaultPackageManager(PackageManagerOptions{
		CWD:             e.tempDir,
		AgentDir:        e.agentDir,
		SettingsManager: e.settingsManager,
		Operations:      operations,
	})
}

func (e *packageManagerGitTestEnv) setupRemoteAndInstall(t *testing.T, sourceOverride string) {
	t.Helper()
	mkdirAll(t, e.remoteDir)
	initPackageManagerGitRepo(t, e.remoteDir)
	createPackageManagerCommit(t, e.remoteDir, "extension.ts", "// v1", "Initial commit")
	clonePackageManagerRepo(t, e)
	source := packageManagerGitSource
	if sourceOverride != "" {
		source = sourceOverride
	}
	e.settingsManager.SetPackages([]any{source})
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required")
	}
}

func initPackageManagerGitRepo(t *testing.T, repoDir string) {
	t.Helper()
	gitForPackageManager(t, []string{"init", "--initial-branch=main"}, repoDir)
	gitForPackageManager(t, []string{"config", "--local", "user.email", "test@test.com"}, repoDir)
	gitForPackageManager(t, []string{"config", "--local", "user.name", "Test"}, repoDir)
}

func createPackageManagerCommit(t *testing.T, repoDir, filename, content, message string) string {
	t.Helper()
	writeFile(t, filepath.Join(repoDir, filename), content)
	gitForPackageManager(t, []string{"add", filename}, repoDir)
	gitForPackageManager(t, []string{"commit", "-m", message}, repoDir)
	return packageManagerGitHead(t, repoDir)
}

func clonePackageManagerRepo(t *testing.T, env *packageManagerGitTestEnv) {
	t.Helper()
	mkdirAll(t, filepath.Join(env.agentDir, "git", "github.com", "test"))
	gitForPackageManager(t, []string{"clone", env.remoteDir, env.installedDir}, env.tempDir)
	gitForPackageManager(t, []string{"config", "--local", "user.email", "test@test.com"}, env.installedDir)
	gitForPackageManager(t, []string{"config", "--local", "user.name", "Test"}, env.installedDir)
}

func packageManagerGitHead(t *testing.T, repoDir string) string {
	t.Helper()
	return gitForPackageManager(t, []string{"rev-parse", "HEAD"}, repoDir)
}

func gitForPackageManager(t *testing.T, args []string, cwd string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed in %s: %v\n%s", strings.Join(args, " "), cwd, err, string(output))
	}
	return strings.TrimSpace(string(output))
}

func packageManagerTemporaryGitDir(agentDir, host, path string) string {
	cacheKey := "git-" + host + "-" + path
	sum := sha256.Sum256([]byte(cacheKey))
	hash := hex.EncodeToString(sum[:])[:8]
	parts := []string{agentDir, "tmp", "extensions", "git-" + host, hash}
	parts = append(parts, strings.Split(path, "/")...)
	return filepath.Join(parts...)
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func removeAll(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestPackageManagerGitTestHelpersUseSameTemporaryPath(t *testing.T) {
	source, ok := ParseGitURL(packageManagerGitSource)
	if !ok {
		t.Fatal("source did not parse")
	}
	agentDir := t.TempDir()
	got, err := (packageInstallLocator{agentDir: agentDir}).temporaryGitInstallPath(source)
	if err != nil {
		t.Fatal(err)
	}
	if want := packageManagerTemporaryGitDir(agentDir, "github.com", "test/extension"); got != want {
		t.Fatalf("temporary path = %q, want %q", got, want)
	}
}
