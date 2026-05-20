package gicodingagent

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProtocolExtensionDescriptorRuntimeContributions(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.gi.json")
	second := filepath.Join(dir, "second.gi.json")
	writeJSON(t, first, map[string]any{"gi": map[string]any{
		"extensionProtocol": "descriptor.v1",
		"id":                "first",
		"commands":          []any{map[string]any{"name": "deploy", "description": "Deploy"}},
		"tools":             []any{map[string]any{"name": "tool-a", "description": "Tool A"}},
		"messageRenderers":  []any{map[string]any{"type": "custom-message"}},
		"events":            []any{"agent_start", "tool_call"},
		"shortcuts":         []any{map[string]any{"key": "ctrl+t", "description": "Test shortcut"}},
		"flags":             []any{map[string]any{"name": "test-flag", "description": "Test flag", "type": "boolean", "default": true}},
	}})
	writeJSON(t, second, map[string]any{"gi": map[string]any{
		"extensionProtocol": "descriptor.v1",
		"id":                "second",
		"tools":             []any{map[string]any{"name": "tool-b", "description": "Tool B"}},
	}})

	runtime := NewDefaultProtocolExtensionRuntime()
	result := LoadProtocolExtensionDescriptors([]ProtocolExtensionSource{
		{Path: first, BaseDir: dir},
		{Path: second, BaseDir: dir},
	}, runtime)
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if got := runtime.CommandInvocationNames(); len(got) != 1 || got[0] != "deploy" {
		t.Fatalf("commands = %#v", runtime.RegisteredCommands())
	}
	if findDynamicSDKTool(runtime.RegisteredTools(), "tool-a") == nil || findDynamicSDKTool(runtime.RegisteredTools(), "tool-b") == nil {
		t.Fatalf("tools = %#v", runtime.RegisteredTools())
	}
	if runtime.GetMessageRenderer("custom-message") == nil {
		t.Fatal("missing message renderer")
	}
	if !runtime.HasHandlers("agent_start") || !runtime.HasHandlers("tool_call") {
		t.Fatalf("handler presence: agent_start=%v tool_call=%v", runtime.HasHandlers("agent_start"), runtime.HasHandlers("tool_call"))
	}
	if shortcuts := runtime.Shortcuts(KeybindingsConfig{}); len(shortcuts.Shortcuts) != 1 || shortcuts.Shortcuts["ctrl+t"].Description != "Test shortcut" {
		t.Fatalf("shortcuts = %#v", shortcuts)
	}
	if flags := runtime.Flags(); len(flags) != 1 || flags[0].Name != "test-flag" || runtime.FlagValue("test-flag") != true {
		t.Fatalf("flags = %#v value=%#v", runtime.Flags(), runtime.FlagValue("test-flag"))
	}
}

func TestProtocolExtensionDescriptorReportsLoadErrors(t *testing.T) {
	dir := t.TempDir()
	invalid := filepath.Join(dir, "invalid.gi.json")
	missingGI := filepath.Join(dir, "missing-gi.gi.json")
	initError := filepath.Join(dir, "init-error.gi.json")
	writeResourceFile(t, invalid, "{")
	writeJSON(t, missingGI, map[string]any{"notGi": true})
	writeJSON(t, initError, map[string]any{"gi": map[string]any{
		"extensionProtocol": "descriptor.v1",
		"initError":         "Initialization failed!",
	}})

	result := LoadProtocolExtensionDescriptors([]ProtocolExtensionSource{
		{Path: invalid, BaseDir: dir},
		{Path: missingGI, BaseDir: dir},
		{Path: initError, BaseDir: dir},
	}, NewDefaultProtocolExtensionRuntime())

	if len(result.Errors) != 3 {
		t.Fatalf("errors = %#v", result.Errors)
	}
	joined := result.Errors[0].Error + "\n" + result.Errors[1].Error + "\n" + result.Errors[2].Error
	for _, want := range []string{"invalid", "extensionProtocol", "Initialization failed"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("errors = %#v, missing %q", result.Errors, want)
		}
	}
}
