package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
	if effective["tui.select.confirm"] != "enter" || effective["app.interrupt"] != "ctrl+x" {
		t.Fatalf("effective = %#v", effective)
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
