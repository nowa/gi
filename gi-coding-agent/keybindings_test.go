package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	gitui "github.com/nowa/gi/gi-tui"
)

func TestKeybindingsMigrationRewritesOldKeyNames(t *testing.T) {
	agentDir := createKeybindingsAgentDir(t, map[string]any{
		"cursorUp":    []any{"up", "ctrl+p"},
		"expandTools": "ctrl+x",
	})

	if err := RunMigrations(agentDir); err != nil {
		t.Fatal(err)
	}

	migrated := readKeybindingsJSON(t, agentDir)
	want := map[string]any{
		"tui.editor.cursorUp": []any{"up", "ctrl+p"},
		"app.tools.expand":    "ctrl+x",
	}
	if !reflect.DeepEqual(migrated, want) {
		t.Fatalf("migrated = %#v, want %#v", migrated, want)
	}
}

func TestKeybindingsMigrationKeepsNamespacedValueWhenBothExist(t *testing.T) {
	agentDir := createKeybindingsAgentDir(t, map[string]any{
		"expandTools":      "ctrl+x",
		"app.tools.expand": "ctrl+y",
	})

	if err := RunMigrations(agentDir); err != nil {
		t.Fatal(err)
	}

	migrated := readKeybindingsJSON(t, agentDir)
	want := map[string]any{"app.tools.expand": "ctrl+y"}
	if !reflect.DeepEqual(migrated, want) {
		t.Fatalf("migrated = %#v, want %#v", migrated, want)
	}
}

func TestKeybindingsManagerLoadsOldKeyNamesInMemory(t *testing.T) {
	agentDir := createKeybindingsAgentDir(t, map[string]any{
		"selectConfirm": "enter",
		"interrupt":     "ctrl+x",
	})

	keybindings := NewKeybindingsManager(agentDir)

	want := KeybindingsConfig{
		"tui.select.confirm": "enter",
		"app.interrupt":      "ctrl+x",
	}
	if !reflect.DeepEqual(keybindings.GetUserBindings(), want) {
		t.Fatalf("user bindings = %#v, want %#v", keybindings.GetUserBindings(), want)
	}
	effective := keybindings.GetEffectiveConfig()
	if effective["tui.select.confirm"] != "enter" || effective["app.interrupt"] != "ctrl+x" || effective["app.clear"] != "ctrl+c" {
		t.Fatalf("effective = %#v", effective)
	}
}

func TestDefaultProtocolKeybindingsKeepPiSelectCancelKeys(t *testing.T) {
	manager := gitui.NewKeybindingsManager(tuiKeybindingsFromProtocol(DefaultProtocolKeybindings()))
	if !manager.Matches("\x1b", "tui.select.cancel") || !manager.Matches("\x03", "tui.select.cancel") {
		t.Fatalf("tui.select.cancel keys = %#v, want escape and ctrl+c like Pi", manager.GetKeys("tui.select.cancel"))
	}
}

func TestRunCLIRunsKeybindingsMigrationOnStartupPiStyle(t *testing.T) {
	agentDir := createKeybindingsAgentDir(t, map[string]any{
		"expandTools": "ctrl+x",
		"newSession":  "ctrl+n",
	})

	code := RunCLI(CLIOptions{
		Args:     []string{"--version"},
		CWD:      t.TempDir(),
		AgentDir: agentDir,
	})
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	migrated := readKeybindingsJSON(t, agentDir)
	want := map[string]any{
		"app.tools.expand": "ctrl+x",
		"app.session.new":  "ctrl+n",
	}
	if !reflect.DeepEqual(migrated, want) {
		t.Fatalf("migrated = %#v, want %#v", migrated, want)
	}
}

func TestRunCLIMigratesCommandsToPromptsOnStartupPiStyle(t *testing.T) {
	cwd := t.TempDir()
	agentDir := t.TempDir()
	writeMigrationFile(t, filepath.Join(agentDir, "commands", "global.md"), "Global command")
	writeMigrationFile(t, filepath.Join(cwd, ConfigDirName, "commands", "project.md"), "Project command")

	code := RunCLI(CLIOptions{
		Args:     []string{"--version"},
		CWD:      cwd,
		AgentDir: agentDir,
	})
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	assertMigrationFile(t, filepath.Join(agentDir, "prompts", "global.md"), "Global command")
	assertMigrationFile(t, filepath.Join(cwd, ConfigDirName, "prompts", "project.md"), "Project command")
	if _, err := os.Stat(filepath.Join(agentDir, "commands")); !os.IsNotExist(err) {
		t.Fatalf("global commands dir still exists or stat failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, ConfigDirName, "commands")); !os.IsNotExist(err) {
		t.Fatalf("project commands dir still exists or stat failed: %v", err)
	}
}

func TestRunMigrationsKeepsCommandsWhenPromptsExistsPiStyle(t *testing.T) {
	agentDir := t.TempDir()
	writeMigrationFile(t, filepath.Join(agentDir, "commands", "old.md"), "Old command")
	writeMigrationFile(t, filepath.Join(agentDir, "prompts", "new.md"), "New prompt")

	if err := RunMigrations(agentDir); err != nil {
		t.Fatal(err)
	}
	assertMigrationFile(t, filepath.Join(agentDir, "commands", "old.md"), "Old command")
	assertMigrationFile(t, filepath.Join(agentDir, "prompts", "new.md"), "New prompt")
}

func TestRunMigrationsMovesRootSessionsToCwdSessionDirPiStyle(t *testing.T) {
	agentDir := t.TempDir()
	cwd := t.TempDir()
	sessionFile := filepath.Join(agentDir, "legacy.jsonl")
	header := FileEntry{
		Type:    "session",
		Version: CurrentSessionVersion,
		ID:      "legacy-session",
		CWD:     cwd,
	}
	line, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionFile, append(line, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	invalid := filepath.Join(agentDir, "invalid.jsonl")
	if err := os.WriteFile(invalid, []byte(`{"type":"message"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RunMigrations(agentDir); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(GetAgentDirSessionDir(cwd, agentDir), "legacy.jsonl")
	assertMigrationFile(t, target, string(line)+"\n")
	if _, err := os.Stat(sessionFile); !os.IsNotExist(err) {
		t.Fatalf("legacy root session still exists or stat failed: %v", err)
	}
	assertMigrationFile(t, invalid, `{"type":"message"}`+"\n")
}

func TestRunMigrationsMovesManagedToolsToBinPiStyle(t *testing.T) {
	agentDir := t.TempDir()
	writeMigrationFile(t, filepath.Join(agentDir, "tools", "fd"), "fd-bin")
	writeMigrationFile(t, filepath.Join(agentDir, "tools", "rg"), "rg-old")
	writeMigrationFile(t, filepath.Join(agentDir, "tools", "custom"), "custom-tool")
	writeMigrationFile(t, filepath.Join(agentDir, "bin", "rg"), "rg-new")

	if err := RunMigrations(agentDir); err != nil {
		t.Fatal(err)
	}
	assertMigrationFile(t, filepath.Join(agentDir, "bin", "fd"), "fd-bin")
	assertMigrationFile(t, filepath.Join(agentDir, "bin", "rg"), "rg-new")
	assertMigrationFile(t, filepath.Join(agentDir, "tools", "custom"), "custom-tool")
	if _, err := os.Stat(filepath.Join(agentDir, "tools", "fd")); !os.IsNotExist(err) {
		t.Fatalf("managed fd still exists or stat failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agentDir, "tools", "rg")); !os.IsNotExist(err) {
		t.Fatalf("managed rg still exists or stat failed: %v", err)
	}
}

func TestRunMigrationsMigratesLegacyAuthToAuthJSONPiStyle(t *testing.T) {
	agentDir := t.TempDir()
	writeJSON(t, filepath.Join(agentDir, "oauth.json"), map[string]any{
		"anthropic": map[string]any{"access": "access-token", "refresh": "refresh-token", "expires": float64(123)},
	})
	writeJSON(t, filepath.Join(agentDir, "settings.json"), map[string]any{
		"theme": "dark",
		"apiKeys": map[string]any{
			"openai":    "openai-key",
			"anthropic": "should-not-overwrite-oauth",
		},
	})

	result, err := RunMigrationsWithResult(agentDir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.MigratedAuthProviders, []string{"anthropic", "openai"}) {
		t.Fatalf("migrated providers = %#v", result.MigratedAuthProviders)
	}
	auth := readAuthStorageData(t, filepath.Join(agentDir, "auth.json"))
	if auth["anthropic"].Type != "oauth" || auth["anthropic"].Access != "access-token" || auth["anthropic"].Refresh != "refresh-token" || auth["anthropic"].Expires != 123 {
		t.Fatalf("anthropic auth = %#v", auth["anthropic"])
	}
	if auth["openai"].Type != "api_key" || auth["openai"].Key != "openai-key" {
		t.Fatalf("openai auth = %#v", auth["openai"])
	}
	mode := fileModePerm(t, filepath.Join(agentDir, "auth.json"))
	if mode != 0o600 {
		t.Fatalf("auth.json mode = %o, want 600", mode)
	}
	if _, err := os.Stat(filepath.Join(agentDir, "oauth.json.migrated")); err != nil {
		t.Fatalf("oauth backup stat = %v", err)
	}
	settings := readMigrationJSON(t, filepath.Join(agentDir, "settings.json"))
	if _, ok := settings["apiKeys"]; ok || settings["theme"] != "dark" {
		t.Fatalf("settings after migration = %#v", settings)
	}

	second, err := RunMigrationsWithResult(agentDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.MigratedAuthProviders) != 0 {
		t.Fatalf("second migration providers = %#v", second.MigratedAuthProviders)
	}
}

func TestRunCLIStartupMigrationsReportsMigratedAuthWarningPiStyle(t *testing.T) {
	agentDir := t.TempDir()
	writeJSON(t, filepath.Join(agentDir, "settings.json"), map[string]any{
		"apiKeys": map[string]any{"openai": "openai-key"},
	})

	result, err := runCLIStartupMigrations(CLIOptions{CWD: t.TempDir(), AgentDir: agentDir})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.StartupWarnings, []string{"Migrated credentials to auth.json: openai"}) {
		t.Fatalf("startup warnings = %#v", result.StartupWarnings)
	}
}

func TestRunMigrationsReportsDeprecatedExtensionDirsPiStyle(t *testing.T) {
	cwd := t.TempDir()
	agentDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(agentDir, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeMigrationFile(t, filepath.Join(agentDir, "tools", "custom-tool"), "custom")
	writeMigrationFile(t, filepath.Join(agentDir, "tools", ".DS_Store"), "hidden")
	if err := os.MkdirAll(filepath.Join(cwd, ConfigDirName, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeMigrationFile(t, filepath.Join(cwd, ConfigDirName, "tools", "fd"), "managed")

	result, err := RunMigrationsWithResult(agentDir, cwd)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Global hooks/ directory found. Hooks have been renamed to extensions.",
		"Global tools/ directory contains custom tools. Custom tools have been merged into extensions.",
		"Project hooks/ directory found. Hooks have been renamed to extensions.",
	}
	if !reflect.DeepEqual(result.DeprecationWarnings, want) {
		t.Fatalf("deprecation warnings = %#v, want %#v", result.DeprecationWarnings, want)
	}

	startupResult, err := runCLIStartupMigrations(CLIOptions{CWD: cwd, AgentDir: agentDir})
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range want {
		if !containsString(startupResult.StartupWarnings, warning) {
			t.Fatalf("startup warnings missing %q: %#v", warning, startupResult.StartupWarnings)
		}
	}
}

func createKeybindingsAgentDir(t *testing.T, config map[string]any) string {
	t.Helper()
	agentDir := t.TempDir()
	content, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "keybindings.json"), append(content, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	return agentDir
}

func writeMigrationFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertMigrationFile(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != want {
		t.Fatalf("%s = %q, want %q", path, string(content), want)
	}
}

func readMigrationJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(content, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func fileModePerm(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func readKeybindingsJSON(t *testing.T, agentDir string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(agentDir, "keybindings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(content, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
