package gicodingagent

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestProtocolExtensionRunnerRegistryPiParity(t *testing.T) {
	t.Run("handles shortcut conflicts against reserved and non-reserved built-ins", func(t *testing.T) {
		t.Run("warns when extension shortcut conflicts with built-in", func(t *testing.T) {
			runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
			mustLoadProtocolFactories(t, runtime, protocolShortcutFactory("conflict.ts", "ctrl+c", "Conflicts"))
			result := runtime.Shortcuts(DefaultProtocolKeybindings())
			if len(result.Shortcuts) != 0 || !protocolWarningsContain(result.Warnings, "conflicts with built-in") {
				t.Fatalf("result = %#v", result)
			}
		})

		t.Run("allows a shortcut when the reserved set no longer contains the default key", func(t *testing.T) {
			runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
			mustLoadProtocolFactories(t, runtime, protocolShortcutFactory("rebinding.ts", "ctrl+p", "Uses freed default"))
			keybindings := DefaultProtocolKeybindings()
			keybindings["app.model.cycleForward"] = "ctrl+n"
			keybindings["app.message.followUp"] = "ctrl+shift+p"
			result := runtime.Shortcuts(keybindings)
			if _, ok := result.Shortcuts["ctrl+p"]; !ok || protocolWarningsContain(result.Warnings, "conflicts with built-in") {
				t.Fatalf("result = %#v", result)
			}
		})

		t.Run("warns but allows when extension uses non-reserved built-in shortcut", func(t *testing.T) {
			runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
			mustLoadProtocolFactories(t, runtime, protocolShortcutFactory("non-reserved.ts", "ctrl+v", "Overrides non-reserved"))
			result := runtime.Shortcuts(DefaultProtocolKeybindings())
			if _, ok := result.Shortcuts["ctrl+v"]; !ok || !protocolWarningsContain(result.Warnings, "built-in shortcut for app.clipboard.pasteImage") {
				t.Fatalf("result = %#v", result)
			}
		})

		t.Run("blocks shortcuts for reserved actions even when rebound", func(t *testing.T) {
			runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
			mustLoadProtocolFactories(t, runtime, protocolShortcutFactory("rebound-reserved.ts", "ctrl+x", "Conflicts"))
			keybindings := DefaultProtocolKeybindings()
			keybindings["app.interrupt"] = "ctrl+x"
			result := runtime.Shortcuts(keybindings)
			if len(result.Shortcuts) != 0 || !protocolWarningsContain(result.Warnings, "conflicts with built-in") {
				t.Fatalf("result = %#v", result)
			}
		})

		t.Run("blocks shortcuts when reserved key is also bound to non-reserved actions", func(t *testing.T) {
			runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
			mustLoadProtocolFactories(t, runtime, protocolShortcutFactory("shared-reserved.ts", "ctrl+p", "Conflicts"))
			result := runtime.Shortcuts(DefaultProtocolKeybindings())
			if len(result.Shortcuts) != 0 || !protocolWarningsContain(result.Warnings, "conflicts with built-in") {
				t.Fatalf("result = %#v", result)
			}
		})

		t.Run("blocks shortcuts when reserved action has multiple keys", func(t *testing.T) {
			runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
			mustLoadProtocolFactories(t, runtime, protocolShortcutFactory("multi-reserved.ts", "ctrl+y", "Conflicts"))
			keybindings := DefaultProtocolKeybindings()
			keybindings["app.clear"] = []any{"ctrl+x", "ctrl+y"}
			result := runtime.Shortcuts(keybindings)
			if len(result.Shortcuts) != 0 || !protocolWarningsContain(result.Warnings, "conflicts with built-in") {
				t.Fatalf("result = %#v", result)
			}
		})

		t.Run("warns but allows when non-reserved action has multiple keys", func(t *testing.T) {
			runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
			mustLoadProtocolFactories(t, runtime, protocolShortcutFactory("multi-non-reserved.ts", "ctrl+y", "Overrides"))
			keybindings := DefaultProtocolKeybindings()
			keybindings["app.clipboard.pasteImage"] = []any{"ctrl+x", "ctrl+y"}
			result := runtime.Shortcuts(keybindings)
			if _, ok := result.Shortcuts["ctrl+y"]; !ok || !protocolWarningsContain(result.Warnings, "built-in shortcut for app.clipboard.pasteImage") {
				t.Fatalf("result = %#v", result)
			}
		})

		t.Run("warns when two extensions register same shortcut", func(t *testing.T) {
			runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
			mustLoadProtocolFactories(t, runtime,
				protocolShortcutFactory("ext1.ts", "ctrl+shift+x", "First extension"),
				protocolShortcutFactory("ext2.ts", "ctrl+shift+x", "Second extension"),
			)
			result := runtime.Shortcuts(DefaultProtocolKeybindings())
			if got := result.Shortcuts["ctrl+shift+x"].Description; got != "Second extension" || !protocolWarningsContain(result.Warnings, "shortcut conflict") {
				t.Fatalf("result = %#v", result)
			}
		})
	})

	t.Run("collects tools from multiple extensions", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityToolsRegister)
		mustLoadProtocolFactories(t, runtime,
			protocolToolFactory("tool-a.ts", "tool_a", "first"),
			protocolToolFactory("tool-b.ts", "tool_b", "second"),
		)
		tools := runtime.RegisteredTools()
		names := []string{tools[0].Name, tools[1].Name}
		sort.Strings(names)
		if !reflect.DeepEqual(names, []string{"tool_a", "tool_b"}) {
			t.Fatalf("tool names = %#v", names)
		}
	})

	t.Run("keeps first tool when two extensions register the same name", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityToolsRegister)
		mustLoadProtocolFactories(t, runtime,
			protocolToolFactory("a-first.ts", "shared", "first"),
			protocolToolFactory("b-second.ts", "shared", "second"),
		)
		tools := runtime.RegisteredTools()
		if len(tools) != 1 || tools[0].Description != "first" {
			t.Fatalf("registered tools = %#v", tools)
		}
	})

	t.Run("collects commands from multiple extensions", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityCommandsRegister)
		mustLoadProtocolFactories(t, runtime,
			protocolCommandFactory("cmd-a.ts", "cmd-a", "A command"),
			protocolCommandFactory("cmd-b.ts", "cmd-b", "B command"),
		)
		commands := runtime.RegisteredCommands()
		names := []string{commands[0].Name, commands[1].Name}
		invocations := []string{commands[0].InvocationName, commands[1].InvocationName}
		sort.Strings(names)
		sort.Strings(invocations)
		if !reflect.DeepEqual(names, []string{"cmd-a", "cmd-b"}) || !reflect.DeepEqual(invocations, []string{"cmd-a", "cmd-b"}) {
			t.Fatalf("commands = %#v", commands)
		}
	})

	t.Run("gets command by invocation name", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityCommandsRegister)
		mustLoadProtocolFactories(t, runtime, protocolCommandFactory("cmd.ts", "my-cmd", "My command"))
		command := runtime.GetCommand("my-cmd")
		if command == nil || command.Name != "my-cmd" || command.InvocationName != "my-cmd" || command.Description != "My command" {
			t.Fatalf("command = %#v", command)
		}
		if missing := runtime.GetCommand("not-exists"); missing != nil {
			t.Fatalf("missing command = %#v", missing)
		}
	})

	t.Run("suffixes duplicate extension commands in insertion order", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityCommandsRegister)
		mustLoadProtocolFactories(t, runtime,
			protocolCommandFactory("cmd-a.ts", "shared-cmd", "First command"),
			protocolCommandFactory("cmd-b.ts", "shared-cmd", "Second command"),
		)
		commands := runtime.RegisteredCommands()
		if len(commands) != 2 {
			t.Fatalf("commands = %#v", commands)
		}
		if commands[0].InvocationName != "shared-cmd:1" || commands[1].InvocationName != "shared-cmd:2" {
			t.Fatalf("duplicate invocations = %#v", commands)
		}
		if runtime.GetCommand("shared-cmd:1").Description != "First command" || runtime.GetCommand("shared-cmd:2").Description != "Second command" {
			t.Fatalf("duplicate command lookup = %#v", commands)
		}
	})

	t.Run("gets message renderer by type", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		renderer := func(any, any) []string { return []string{"rendered"} }
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "renderer.ts", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.RegisterMessageRenderer("my-type", renderer)
		}})
		if got := runtime.GetMessageRenderer("my-type"); got == nil || !reflect.DeepEqual(got(nil, nil), []string{"rendered"}) {
			t.Fatalf("message renderer = %#v", got)
		}
		if got := runtime.GetMessageRenderer("not-exists"); got != nil {
			t.Fatalf("missing renderer = %#v", got)
		}
	})

	t.Run("collects flags from extensions", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		mustLoadProtocolFactories(t, runtime, protocolFlagFactory("with-flag.ts", "my-flag", "My flag", nil))
		flags := runtime.Flags()
		if len(flags) != 1 || flags[0].Name != "my-flag" || flags[0].Description != "My flag" {
			t.Fatalf("flags = %#v", flags)
		}
	})

	t.Run("keeps first flag when two extensions register the same name", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		mustLoadProtocolFactories(t, runtime,
			protocolFlagFactory("a-first.ts", "shared-flag", "first", true),
			protocolFlagFactory("b-second.ts", "shared-flag", "second", false),
		)
		flags := runtime.Flags()
		if len(flags) != 1 || flags[0].Description != "first" || runtime.FlagValue("shared-flag") != true {
			t.Fatalf("flags = %#v value=%#v", flags, runtime.FlagValue("shared-flag"))
		}
	})

	t.Run("can set flag values", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		mustLoadProtocolFactories(t, runtime, protocolFlagFactory("flag.ts", "test-flag", "Test flag", nil))
		runtime.SetFlagValue("--test-flag", true)
		if got := runtime.FlagValue("test-flag"); got != true {
			t.Fatalf("flag value = %#v", got)
		}
	})

	t.Run("returns true when handlers exist for event type", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
		if runtime.HasHandlers("tool_call") {
			t.Fatal("empty runtime should not have tool_call handlers")
		}
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "handler.ts", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.On("tool_call", func(ProtocolSessionEvent) (ProtocolEventResult, error) {
				return ProtocolEventResult{}, nil
			})
		}})
		if !runtime.HasHandlers("tool_call") || runtime.HasHandlers("agent_end") {
			t.Fatalf("handler presence: tool_call=%v agent_end=%v", runtime.HasHandlers("tool_call"), runtime.HasHandlers("agent_end"))
		}
	})

	t.Run("calls error listeners when handler throws", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "throws.ts", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.On("context", func(ProtocolSessionEvent) (ProtocolEventResult, error) {
				return ProtocolEventResult{}, errors.New("Handler error!")
			})
		}})
		var got []ProtocolExtensionError
		runtime.OnError(func(event ProtocolExtensionError) {
			got = append(got, event)
		})
		if _, err := runtime.EmitSessionEvent(ProtocolSessionEvent{Type: "context"}); err == nil {
			t.Fatal("expected context handler error")
		}
		if len(got) != 1 || got[0].ExtensionPath != "throws.ts" || got[0].Event != "context" || !strings.Contains(got[0].Error, "Handler error!") {
			t.Fatalf("errors = %#v", got)
		}
	})
}

func protocolToolFactory(path, name, description string) ProtocolExtensionFactory {
	return ProtocolExtensionFactory{Path: path, Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.RegisterTool(ProtocolToolDefinition{Name: name, Label: name, Description: description})
	}}
}

func protocolCommandFactory(path, name, description string) ProtocolExtensionFactory {
	return ProtocolExtensionFactory{Path: path, Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.RegisterCommand(name, ProtocolCommandDefinition{Description: description, Handler: func(string) error {
			return nil
		}})
	}}
}

func protocolFlagFactory(path, name, description string, defaultValue any) ProtocolExtensionFactory {
	return ProtocolExtensionFactory{Path: path, Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.RegisterFlag(name, ProtocolFlagDefinition{Description: description, Default: defaultValue})
	}}
}

func protocolShortcutFactory(path, key, description string) ProtocolExtensionFactory {
	return ProtocolExtensionFactory{Path: path, Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.RegisterShortcut(key, ProtocolShortcutDefinition{Description: description})
	}}
}

func protocolWarningsContain(warnings []ProtocolShortcutWarning, text string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning.Message, text) {
			return true
		}
	}
	return false
}
