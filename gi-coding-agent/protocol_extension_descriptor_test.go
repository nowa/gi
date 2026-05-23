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
		"commands":          []any{map[string]any{"name": "deploy", "description": "Deploy", "argumentHint": "<env>"}},
		"tools":             []any{map[string]any{"name": "tool-a", "description": "Tool A"}},
		"messageRenderers":  []any{map[string]any{"type": "custom-message"}},
		"viewTrees":         []any{map[string]any{"mountId": "deploy.status", "slot": "aboveEditor", "priority": 20, "view": map[string]any{"type": "text", "text": "Deploy ready"}}},
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
	if command := runtime.RegisteredCommands()[0]; command.Description != "Deploy" || command.ArgumentHint != "<env>" {
		t.Fatalf("command metadata = %#v", command)
	}
	if findDynamicSDKTool(runtime.RegisteredTools(), "tool-a") == nil || findDynamicSDKTool(runtime.RegisteredTools(), "tool-b") == nil {
		t.Fatalf("tools = %#v", runtime.RegisteredTools())
	}
	if runtime.GetMessageRenderer("custom-message") == nil {
		t.Fatal("missing message renderer")
	}
	if mounts := runtime.ViewTreeMounts(); len(mounts) != 1 || mounts[0].MountID != "deploy.status" || mounts[0].Priority != 20 {
		t.Fatalf("view tree mounts = %#v", mounts)
	}
	viewTreeHost := NewViewTreeHost()
	runtime.BindViewTreeHost(viewTreeHost)
	if rendered, err := viewTreeHost.RenderMount("deploy.status", 80); err != nil || strings.Join(rendered, "\n") != "Deploy ready" {
		t.Fatalf("descriptor view tree render = %#v err=%v", rendered, err)
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

func TestProtocolExtensionDescriptorResourceThemes(t *testing.T) {
	dir := t.TempDir()
	descriptor := filepath.Join(dir, "resources.gi.json")
	writeJSON(t, descriptor, map[string]any{"gi": map[string]any{
		"extensionProtocol": "descriptor.v1",
		"id":                "resources",
		"resources": map[string]any{
			"skills":  []any{"skills/example"},
			"prompts": []any{"prompts/example.md"},
			"themes":  []any{"themes/example.json"},
		},
	}})

	result := LoadProtocolExtensionDescriptors([]ProtocolExtensionSource{{Path: descriptor, BaseDir: dir}}, NewDefaultProtocolExtensionRuntime())
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %#v", result.Errors)
	}
	wantSkill := filepath.Join(dir, "skills", "example")
	wantPrompt := filepath.Join(dir, "prompts", "example.md")
	wantTheme := filepath.Join(dir, "themes", "example.json")
	if len(result.Resources.SkillPaths) != 1 || result.Resources.SkillPaths[0].Path != wantSkill ||
		len(result.Resources.PromptPaths) != 1 || result.Resources.PromptPaths[0].Path != wantPrompt ||
		len(result.Resources.ThemePaths) != 1 || result.Resources.ThemePaths[0].Path != wantTheme {
		t.Fatalf("resources = %#v", result.Resources)
	}
	if result.Resources.ThemePaths[0].Metadata.Source != "extension:resources" ||
		result.Resources.ThemePaths[0].Metadata.Path != wantTheme {
		t.Fatalf("theme metadata = %#v", result.Resources.ThemePaths[0].Metadata)
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
