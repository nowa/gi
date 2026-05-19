package gitui

import (
	"reflect"
	"testing"
)

func TestKeybindingsManagerDoesNotEvictDefaultsWhenUserReusesKeys(t *testing.T) {
	defaults := NewKeybindingsManager()
	if got := defaults.Keys("tui.select.up"); !reflect.DeepEqual(got, []string{"up"}) {
		t.Fatalf("default select up keys = %#v, want Pi default [up]", got)
	}
	if got := defaults.Keys("tui.select.down"); !reflect.DeepEqual(got, []string{"down"}) {
		t.Fatalf("default select down keys = %#v, want Pi default [down]", got)
	}
	definition, ok := defaults.Definition("tui.editor.cursorWordLeft")
	if !ok || definition.Description != "Move cursor word left" {
		t.Fatalf("default definition should preserve Pi description, got %#v ok=%v", definition, ok)
	}

	keybindings := NewKeybindingsManager(KeybindingsConfig{
		"tui.input.submit": {"enter", "ctrl+enter"},
	})
	if got := keybindings.Keys("tui.input.submit"); !reflect.DeepEqual(got, []string{"enter", "ctrl+enter"}) {
		t.Fatalf("input submit keys = %#v", got)
	}
	if got := keybindings.Keys("tui.select.confirm"); !reflect.DeepEqual(got, []string{"enter"}) {
		t.Fatalf("select confirm keys = %#v", got)
	}

	keybindings = NewKeybindingsManager(KeybindingsConfig{
		"tui.select.up": {"up", "ctrl+p"},
	})
	if got := keybindings.Keys("tui.select.up"); !reflect.DeepEqual(got, []string{"up", "ctrl+p"}) {
		t.Fatalf("select up keys = %#v", got)
	}
	if got := keybindings.Keys("tui.editor.cursorUp"); !reflect.DeepEqual(got, []string{"up"}) {
		t.Fatalf("editor cursor up keys = %#v", got)
	}
}

func TestKeybindingsManagerReportsOnlyUserBindingConflicts(t *testing.T) {
	keybindings := NewKeybindingsManager(KeybindingsConfig{
		"tui.input.submit":   {"ctrl+x"},
		"tui.select.confirm": {"ctrl+x"},
	})
	conflicts := keybindings.Conflicts()
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %#v", conflicts)
	}
	if conflicts[0].Key != "ctrl+x" || !reflect.DeepEqual(conflicts[0].Actions, []string{"tui.input.submit", "tui.select.confirm"}) {
		t.Fatalf("conflict = %#v", conflicts[0])
	}
	if got := keybindings.Keys("tui.editor.cursorLeft"); !reflect.DeepEqual(got, []string{"left", "ctrl+b"}) {
		t.Fatalf("default cursor left should remain resolved, got %#v", got)
	}
}

func TestKeybindingsManagerPiPublicGetterSurface(t *testing.T) {
	keybindings := NewKeybindingsManagerWithDefinitions(TUI_KEYBINDINGS, KeybindingsConfig{
		"tui.input.submit":   {"ctrl+x"},
		"tui.select.confirm": {"ctrl+x"},
	})

	if got := keybindings.GetKeys("tui.input.submit"); !reflect.DeepEqual(got, []string{"ctrl+x"}) {
		t.Fatalf("GetKeys = %#v, want ctrl+x", got)
	}
	if definition, ok := keybindings.GetDefinition("tui.select.confirm"); !ok || definition.Description != "Confirm selection" {
		t.Fatalf("GetDefinition = %#v ok=%v, want Pi default confirm definition", definition, ok)
	}
	if got := keybindings.GetResolvedBindings()["tui.editor.cursorLeft"]; !reflect.DeepEqual(got, []string{"left", "ctrl+b"}) {
		t.Fatalf("GetResolvedBindings cursorLeft = %#v", got)
	}
	if got := keybindings.GetUserBindings()["tui.select.confirm"]; !reflect.DeepEqual(got, []string{"ctrl+x"}) {
		t.Fatalf("GetUserBindings select.confirm = %#v", got)
	}
	conflicts := keybindings.GetConflicts()
	if len(conflicts) != 1 || conflicts[0].Key != "ctrl+x" {
		t.Fatalf("GetConflicts = %#v, want ctrl+x conflict", conflicts)
	}
	if !reflect.DeepEqual(conflicts[0].Actions, conflicts[0].Keybindings) {
		t.Fatalf("conflict Keybindings alias should mirror Actions: %#v", conflicts[0])
	}
}

func TestKeybindingsManagerNormalizesDuplicateKeys(t *testing.T) {
	keybindings := NewKeybindingsManager(KeybindingsConfig{
		"tui.input.submit": {"enter", "enter", "ctrl+enter"},
	})
	if got := keybindings.Keys("tui.input.submit"); !reflect.DeepEqual(got, []string{"enter", "ctrl+enter"}) {
		t.Fatalf("deduplicated keys = %#v", got)
	}
}
