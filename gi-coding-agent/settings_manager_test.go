package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestSettingsManagerPreservesExternalGlobalEdits(t *testing.T) {
	agentDir, projectDir := createSettingsTestDirs(t)
	settingsPath := filepath.Join(agentDir, "settings.json")
	writeSettingsJSON(t, settingsPath, map[string]any{
		"theme":        "dark",
		"defaultModel": "claude-sonnet",
	})

	manager := NewSettingsManager(projectDir, agentDir)
	settings := readSettingsJSON(t, settingsPath)
	settings["enabledModels"] = []any{"claude-opus-4-5", "gpt-5.2-codex"}
	writeSettingsJSON(t, settingsPath, settings)

	manager.SetDefaultThinkingLevel("high")
	if err := manager.Flush(); err != nil {
		t.Fatal(err)
	}

	saved := readSettingsJSON(t, settingsPath)
	if !reflect.DeepEqual(saved["enabledModels"], []any{"claude-opus-4-5", "gpt-5.2-codex"}) {
		t.Fatalf("enabledModels = %#v", saved["enabledModels"])
	}
	if saved["defaultThinkingLevel"] != "high" || saved["theme"] != "dark" || saved["defaultModel"] != "claude-sonnet" {
		t.Fatalf("saved settings = %#v", saved)
	}

	writeSettingsJSON(t, settingsPath, map[string]any{"defaultModel": "claude-sonnet"})
	manager = NewSettingsManager(projectDir, agentDir)
	settings = readSettingsJSON(t, settingsPath)
	settings["shellPath"] = "/bin/zsh"
	settings["extensions"] = []any{"/path/to/extension.ts"}
	writeSettingsJSON(t, settingsPath, settings)
	manager.SetTheme("light")

	saved = readSettingsJSON(t, settingsPath)
	if saved["shellPath"] != "/bin/zsh" || saved["theme"] != "light" {
		t.Fatalf("saved settings = %#v", saved)
	}
	if !reflect.DeepEqual(saved["extensions"], []any{"/path/to/extension.ts"}) {
		t.Fatalf("extensions = %#v", saved["extensions"])
	}
}

func TestSettingsManagerInMemoryChangesWinForSameGlobalKey(t *testing.T) {
	agentDir, projectDir := createSettingsTestDirs(t)
	settingsPath := filepath.Join(agentDir, "settings.json")
	writeSettingsJSON(t, settingsPath, map[string]any{"theme": "dark"})

	manager := NewSettingsManager(projectDir, agentDir)
	settings := readSettingsJSON(t, settingsPath)
	settings["defaultThinkingLevel"] = "low"
	writeSettingsJSON(t, settingsPath, settings)

	manager.SetDefaultThinkingLevel("high")

	saved := readSettingsJSON(t, settingsPath)
	if saved["defaultThinkingLevel"] != "high" {
		t.Fatalf("defaultThinkingLevel = %#v", saved["defaultThinkingLevel"])
	}
}

func TestSettingsManagerPackagesAndExtensions(t *testing.T) {
	agentDir, projectDir := createSettingsTestDirs(t)
	settingsPath := filepath.Join(agentDir, "settings.json")

	writeSettingsJSON(t, settingsPath, map[string]any{
		"extensions": []any{"/local/ext.ts", "./relative/ext.ts"},
	})
	manager := NewSettingsManager(projectDir, agentDir)
	if packages := manager.GetPackages(); len(packages) != 0 {
		t.Fatalf("packages = %#v", packages)
	}
	if got := manager.GetExtensionPaths(); !reflect.DeepEqual(got, []string{"/local/ext.ts", "./relative/ext.ts"}) {
		t.Fatalf("extensions = %#v", got)
	}

	writeSettingsJSON(t, settingsPath, map[string]any{
		"packages": []any{
			"git:github.com/gi-packages/simple-pkg",
			map[string]any{
				"source":     "git:github.com/gi-packages/test-extensions",
				"extensions": []any{"extensions/oracle.gi.json"},
				"skills":     []any{},
			},
		},
	})
	manager = NewSettingsManager(projectDir, agentDir)
	packages := manager.GetPackages()
	if len(packages) != 2 || packages[0] != "git:github.com/gi-packages/simple-pkg" {
		t.Fatalf("packages = %#v", packages)
	}
	object, ok := packages[1].(map[string]any)
	if !ok || object["source"] != "git:github.com/gi-packages/test-extensions" {
		t.Fatalf("package object = %#v", packages[1])
	}
}

func TestSettingsManagerReloadAndErrors(t *testing.T) {
	agentDir, projectDir := createSettingsTestDirs(t)
	globalPath := filepath.Join(agentDir, "settings.json")
	projectPath := filepath.Join(projectDir, ".gi", "settings.json")
	writeSettingsJSON(t, globalPath, map[string]any{
		"theme":      "dark",
		"extensions": []any{"/before.ts"},
	})

	manager := NewSettingsManager(projectDir, agentDir)
	writeSettingsJSON(t, globalPath, map[string]any{
		"theme":        "light",
		"extensions":   []any{"/after.ts"},
		"defaultModel": "claude-sonnet",
	})

	manager.Reload()
	if manager.GetTheme() != "light" || manager.GetDefaultModel() != "claude-sonnet" {
		t.Fatalf("manager after reload: theme=%q model=%q", manager.GetTheme(), manager.GetDefaultModel())
	}
	if got := manager.GetExtensionPaths(); !reflect.DeepEqual(got, []string{"/after.ts"}) {
		t.Fatalf("extensions after reload = %#v", got)
	}

	if err := os.WriteFile(globalPath, []byte("{ invalid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager.Reload()
	if manager.GetTheme() != "light" {
		t.Fatalf("invalid reload should keep previous theme, got %q", manager.GetTheme())
	}

	if err := os.WriteFile(globalPath, []byte("{ invalid global json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPath, []byte("{ invalid project json"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager = NewSettingsManager(projectDir, agentDir)
	errors := manager.DrainErrors()
	if len(errors) != 2 {
		t.Fatalf("errors = %#v", errors)
	}
	scopes := []string{errors[0].Scope, errors[1].Scope}
	sort.Strings(scopes)
	if !reflect.DeepEqual(scopes, []string{"global", "project"}) {
		t.Fatalf("error scopes = %#v", scopes)
	}
	if len(manager.DrainErrors()) != 0 {
		t.Fatalf("errors should drain once")
	}
}

func TestSettingsManagerInMemoryReloadPiRegression(t *testing.T) {
	manager := NewInMemorySettingsManager(map[string]any{
		"defaultThinkingLevel": "high",
		"images":               map[string]any{"autoResize": false},
		"packages":             []any{"git:github.com/gi-packages/test"},
	})

	manager.Reload()
	if manager.GetDefaultThinkingLevel() != "high" || manager.GetImageAutoResize() {
		t.Fatalf("in-memory settings after reload: thinking=%q autoResize=%v", manager.GetDefaultThinkingLevel(), manager.GetImageAutoResize())
	}
	if packages := manager.GetPackages(); !reflect.DeepEqual(packages, []any{"git:github.com/gi-packages/test"}) {
		t.Fatalf("packages after reload = %#v", packages)
	}

	manager.SetTheme("dark")
	if err := manager.Flush(); err != nil {
		t.Fatal(err)
	}
	manager.Reload()
	if manager.GetTheme() != "dark" || manager.GetImageAutoResize() {
		t.Fatalf("in-memory settings after setter+reload: theme=%q autoResize=%v", manager.GetTheme(), manager.GetImageAutoResize())
	}
}

func TestSettingsManagerBranchSummarySkipPromptPiStyle(t *testing.T) {
	manager := NewInMemorySettingsManager(nil)
	if manager.GetBranchSummarySkipPrompt() {
		t.Fatal("branch summary prompt should be enabled by default")
	}

	manager = NewInMemorySettingsManager(map[string]any{
		"branchSummary": map[string]any{"skipPrompt": true},
	})
	if !manager.GetBranchSummarySkipPrompt() {
		t.Fatal("branch summary skipPrompt should be read from nested settings")
	}

	manager.SetBranchSummarySkipPrompt(false)
	if manager.GetBranchSummarySkipPrompt() {
		t.Fatal("branch summary skipPrompt should update through setter")
	}
}

func TestSettingsManagerProjectDirectoryCreation(t *testing.T) {
	agentDir, projectDir := createSettingsTestDirs(t)
	writeSettingsJSON(t, filepath.Join(agentDir, "settings.json"), map[string]any{"theme": "dark"})
	if err := os.RemoveAll(filepath.Join(projectDir, ".gi")); err != nil {
		t.Fatal(err)
	}

	manager := NewSettingsManager(projectDir, agentDir)
	if _, err := os.Stat(filepath.Join(projectDir, ".gi")); !os.IsNotExist(err) {
		t.Fatalf(".gi should not be created by read, err=%v", err)
	}
	if manager.GetTheme() != "dark" {
		t.Fatalf("theme = %q", manager.GetTheme())
	}

	manager.SetProjectPackages([]any{map[string]any{"source": "git:github.com/gi-packages/test-pkg"}})
	if _, err := os.Stat(filepath.Join(projectDir, ".gi", "settings.json")); err != nil {
		t.Fatalf("project settings should be created: %v", err)
	}
}

func TestSettingsManagerShellCommandPrefixAndSessionDir(t *testing.T) {
	agentDir, projectDir := createSettingsTestDirs(t)
	settingsPath := filepath.Join(agentDir, "settings.json")
	writeSettingsJSON(t, settingsPath, map[string]any{
		"shellCommandPrefix": "shopt -s expand_aliases",
	})

	manager := NewSettingsManager(projectDir, agentDir)
	if manager.GetShellCommandPrefix() != "shopt -s expand_aliases" {
		t.Fatalf("shellCommandPrefix = %q", manager.GetShellCommandPrefix())
	}
	manager.SetTheme("light")
	saved := readSettingsJSON(t, settingsPath)
	if saved["shellCommandPrefix"] != "shopt -s expand_aliases" || saved["theme"] != "light" {
		t.Fatalf("saved settings = %#v", saved)
	}

	writeSettingsJSON(t, settingsPath, map[string]any{"theme": "dark"})
	manager = NewSettingsManager(projectDir, agentDir)
	if manager.GetShellCommandPrefix() != "" || manager.GetSessionDir() != "" {
		t.Fatalf("unset shell/session = %q %q", manager.GetShellCommandPrefix(), manager.GetSessionDir())
	}

	writeSettingsJSON(t, settingsPath, map[string]any{"sessionDir": "/tmp/sessions"})
	manager = NewSettingsManager(projectDir, agentDir)
	if manager.GetSessionDir() != "/tmp/sessions" {
		t.Fatalf("global sessionDir = %q", manager.GetSessionDir())
	}

	writeSettingsJSON(t, settingsPath, map[string]any{"sessionDir": "/global/sessions"})
	writeSettingsJSON(t, filepath.Join(projectDir, ".gi", "settings.json"), map[string]any{"sessionDir": "./sessions"})
	manager = NewSettingsManager(projectDir, agentDir)
	if manager.GetSessionDir() != "./sessions" {
		t.Fatalf("project sessionDir = %q", manager.GetSessionDir())
	}

	writeSettingsJSON(t, settingsPath, map[string]any{"sessionDir": "~/sessions"})
	if err := os.Remove(filepath.Join(projectDir, ".gi", "settings.json")); err != nil {
		t.Fatal(err)
	}
	manager = NewSettingsManager(projectDir, agentDir)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "sessions")
	if manager.GetSessionDir() != want {
		t.Fatalf("expanded sessionDir = %q, want %q", manager.GetSessionDir(), want)
	}
}

func TestSettingsManagerPiInteractiveSettingsAccessors(t *testing.T) {
	agentDir, projectDir := createSettingsTestDirs(t)
	settingsPath := filepath.Join(agentDir, "settings.json")
	writeSettingsJSON(t, settingsPath, map[string]any{
		"transport":                 "websocket",
		"httpIdleTimeoutMs":         120000,
		"enableSkillCommands":       false,
		"hideThinkingBlock":         true,
		"collapseChangelog":         true,
		"lastChangelogVersion":      "0.1.0",
		"quietStartup":              true,
		"doubleEscapeAction":        "fork",
		"treeFilterMode":            "user-only",
		"editorPaddingX":            2,
		"autocompleteMaxVisible":    10,
		"showHardwareCursor":        true,
		"enabledModels":             []any{"openai/gpt-4o-mini", "anthropic/claude-sonnet-4-5:high"},
		"terminal":                  map[string]any{"showImages": false, "imageWidthCells": 80, "clearOnShrink": true, "showTerminalProgress": true},
		"warnings":                  map[string]any{"anthropicExtraUsage": false},
		"unknownFutureSetting":      "preserved",
		"unknownFutureNestedObject": map[string]any{"preserved": true},
	})

	manager := NewSettingsManager(projectDir, agentDir)
	if manager.GetTransport() != "websocket" ||
		manager.GetHTTPIdleTimeoutMS() != 120000 ||
		manager.GetEnableSkillCommands() ||
		!manager.GetHideThinkingBlock() ||
		!manager.GetCollapseChangelog() ||
		manager.GetLastChangelogVersion() != "0.1.0" ||
		!manager.GetQuietStartup() ||
		manager.GetDoubleEscapeAction() != "fork" ||
		manager.GetTreeFilterMode() != "user-only" ||
		manager.GetEditorPaddingX() != 2 ||
		manager.GetAutocompleteMaxVisible() != 10 ||
		!manager.GetShowHardwareCursor() ||
		manager.GetShowImages() ||
		manager.GetImageWidthCells() != 80 ||
		!manager.GetClearOnShrink() ||
		!manager.GetShowTerminalProgress() ||
		manager.GetWarnings().AnthropicExtraUsage {
		t.Fatalf("loaded settings not reflected")
	}
	if got := manager.GetEnabledModels(); !reflect.DeepEqual(got, []string{"openai/gpt-4o-mini", "anthropic/claude-sonnet-4-5:high"}) {
		t.Fatalf("enabledModels = %#v", got)
	}

	manager.SetTransport("websocket-cached")
	manager.SetHTTPIdleTimeoutMS(0)
	manager.SetShowImages(true)
	manager.SetImageWidthCells(120)
	manager.SetClearOnShrink(false)
	manager.SetShowTerminalProgress(false)
	manager.SetEnableSkillCommands(true)
	manager.SetHideThinkingBlock(false)
	manager.SetCollapseChangelog(false)
	manager.SetLastChangelogVersion("0.2.0")
	manager.SetQuietStartup(false)
	manager.SetDoubleEscapeAction("none")
	manager.SetTreeFilterMode("all")
	manager.SetEditorPaddingX(3)
	manager.SetAutocompleteMaxVisible(20)
	manager.SetShowHardwareCursor(false)
	manager.SetWarnings(WarningSettings{AnthropicExtraUsage: true})
	manager.SetEnabledModels([]string{"zai/glm-5.1"})

	saved := readSettingsJSON(t, settingsPath)
	terminal, _ := saved["terminal"].(map[string]any)
	warnings, _ := saved["warnings"].(map[string]any)
	if saved["transport"] != "websocket-cached" ||
		saved["httpIdleTimeoutMs"] != float64(0) ||
		saved["enableSkillCommands"] != true ||
		saved["hideThinkingBlock"] != false ||
		saved["collapseChangelog"] != false ||
		saved["lastChangelogVersion"] != "0.2.0" ||
		saved["quietStartup"] != false ||
		saved["doubleEscapeAction"] != "none" ||
		saved["treeFilterMode"] != "all" ||
		saved["editorPaddingX"] != float64(3) ||
		saved["autocompleteMaxVisible"] != float64(20) ||
		saved["showHardwareCursor"] != false ||
		terminal["showImages"] != true ||
		terminal["imageWidthCells"] != float64(120) ||
		terminal["clearOnShrink"] != false ||
		terminal["showTerminalProgress"] != false ||
		warnings["anthropicExtraUsage"] != true {
		t.Fatalf("saved settings = %#v", saved)
	}
	if !reflect.DeepEqual(saved["enabledModels"], []any{"zai/glm-5.1"}) {
		t.Fatalf("saved enabledModels = %#v", saved["enabledModels"])
	}
	if saved["unknownFutureSetting"] != "preserved" {
		t.Fatalf("unknown setting was not preserved: %#v", saved)
	}
}

func TestSettingsManagerPiInteractiveSettingsDefaultsAndValidation(t *testing.T) {
	manager := NewInMemorySettingsManager(map[string]any{
		"transport":              "invalid",
		"httpIdleTimeoutMs":      -1,
		"doubleEscapeAction":     "invalid",
		"treeFilterMode":         "invalid",
		"editorPaddingX":         -2,
		"autocompleteMaxVisible": 0,
		"terminal":               map[string]any{"imageWidthCells": -1},
	})

	if manager.GetTransport() != "auto" ||
		manager.GetHTTPIdleTimeoutMS() != defaultHTTPIdleTimeoutMS ||
		!manager.GetShowImages() ||
		manager.GetImageWidthCells() != 60 ||
		manager.GetClearOnShrink() ||
		manager.GetShowTerminalProgress() ||
		!manager.GetEnableSkillCommands() ||
		manager.GetHideThinkingBlock() ||
		manager.GetCollapseChangelog() ||
		manager.GetQuietStartup() ||
		manager.GetDoubleEscapeAction() != "tree" ||
		manager.GetTreeFilterMode() != "default" ||
		manager.GetEditorPaddingX() != 0 ||
		manager.GetAutocompleteMaxVisible() != 5 ||
		manager.GetShowHardwareCursor() ||
		!manager.GetWarnings().AnthropicExtraUsage {
		t.Fatalf("defaults validation failed")
	}

	manager.SetTransport("invalid")
	manager.SetHTTPIdleTimeoutMS(-1)
	manager.SetDoubleEscapeAction("invalid")
	manager.SetTreeFilterMode("invalid")
	manager.SetEditorPaddingX(-1)
	manager.SetAutocompleteMaxVisible(0)
	manager.SetImageWidthCells(0)
	if manager.GetTransport() != "auto" ||
		manager.GetHTTPIdleTimeoutMS() != defaultHTTPIdleTimeoutMS ||
		manager.GetDoubleEscapeAction() != "tree" ||
		manager.GetTreeFilterMode() != "default" ||
		manager.GetEditorPaddingX() != 0 ||
		manager.GetAutocompleteMaxVisible() != 5 ||
		manager.GetImageWidthCells() != 60 {
		t.Fatalf("setter validation failed")
	}
}

func TestSettingsManagerPreservesExternalArrayEdits(t *testing.T) {
	agentDir, projectDir := createSettingsTestDirs(t)
	settingsPath := filepath.Join(agentDir, "settings.json")

	writeSettingsJSON(t, settingsPath, map[string]any{
		"theme":    "dark",
		"packages": []any{"git:github.com/gi-packages/mcp-adapter"},
	})
	manager := NewSettingsManager(projectDir, agentDir)
	settings := readSettingsJSON(t, settingsPath)
	settings["packages"] = []any{}
	writeSettingsJSON(t, settingsPath, settings)
	manager.SetTheme("light")

	saved := readSettingsJSON(t, settingsPath)
	if !reflect.DeepEqual(saved["packages"], []any{}) || saved["theme"] != "light" {
		t.Fatalf("saved settings = %#v", saved)
	}

	writeSettingsJSON(t, settingsPath, map[string]any{
		"theme":      "dark",
		"extensions": []any{"/old/extension.ts"},
	})
	manager = NewSettingsManager(projectDir, agentDir)
	settings = readSettingsJSON(t, settingsPath)
	settings["extensions"] = []any{"/new/extension.ts"}
	writeSettingsJSON(t, settingsPath, settings)
	manager.SetDefaultThinkingLevel("high")
	saved = readSettingsJSON(t, settingsPath)
	if !reflect.DeepEqual(saved["extensions"], []any{"/new/extension.ts"}) {
		t.Fatalf("extensions = %#v", saved["extensions"])
	}
}

func TestSettingsManagerPreservesExternalProjectEdits(t *testing.T) {
	agentDir, projectDir := createSettingsTestDirs(t)
	projectSettingsPath := filepath.Join(projectDir, ".gi", "settings.json")
	writeSettingsJSON(t, projectSettingsPath, map[string]any{
		"extensions": []any{"./old-extension.ts"},
		"prompts":    []any{"./old-prompt.md"},
	})

	manager := NewSettingsManager(projectDir, agentDir)
	settings := readSettingsJSON(t, projectSettingsPath)
	settings["prompts"] = []any{"./new-prompt.md"}
	writeSettingsJSON(t, projectSettingsPath, settings)
	manager.SetProjectExtensionPaths([]string{"./updated-extension.ts"})

	saved := readSettingsJSON(t, projectSettingsPath)
	if !reflect.DeepEqual(saved["prompts"], []any{"./new-prompt.md"}) {
		t.Fatalf("prompts = %#v", saved["prompts"])
	}
	if !reflect.DeepEqual(saved["extensions"], []any{"./updated-extension.ts"}) {
		t.Fatalf("extensions = %#v", saved["extensions"])
	}

	settings["extensions"] = []any{"./initial-extension.ts"}
	delete(settings, "prompts")
	writeSettingsJSON(t, projectSettingsPath, settings)
	manager = NewSettingsManager(projectDir, agentDir)
	settings = readSettingsJSON(t, projectSettingsPath)
	settings["extensions"] = []any{"./external-extension.ts"}
	writeSettingsJSON(t, projectSettingsPath, settings)
	manager.SetProjectExtensionPaths([]string{"./in-memory-extension.ts"})

	saved = readSettingsJSON(t, projectSettingsPath)
	if !reflect.DeepEqual(saved["extensions"], []any{"./in-memory-extension.ts"}) {
		t.Fatalf("extensions = %#v", saved["extensions"])
	}
}

func createSettingsTestDirs(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	agentDir := filepath.Join(root, "agent")
	projectDir := filepath.Join(root, "project")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, ".gi"), 0o755); err != nil {
		t.Fatal(err)
	}
	return agentDir, projectDir
}

func writeSettingsJSON(t *testing.T, path string, value map[string]any) {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readSettingsJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(content, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
