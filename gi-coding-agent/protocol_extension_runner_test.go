package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestProtocolExtensionRunnerRegistryProtocolContracts(t *testing.T) {
	t.Run("handles shortcut conflicts against reserved and non-reserved built-ins", func(t *testing.T) {
		t.Run("warns when extension shortcut conflicts with built-in", func(t *testing.T) {
			runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
			mustLoadProtocolFactories(t, runtime, protocolShortcutFactory("conflict.gi.json", "ctrl+c", "Conflicts"))
			result := runtime.Shortcuts(DefaultProtocolKeybindings())
			if len(result.Shortcuts) != 0 || !protocolWarningsContain(result.Warnings, "conflicts with built-in") {
				t.Fatalf("result = %#v", result)
			}
		})

		t.Run("allows a shortcut when the reserved set no longer contains the default key", func(t *testing.T) {
			runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
			mustLoadProtocolFactories(t, runtime, protocolShortcutFactory("rebinding.gi.json", "ctrl+p", "Uses freed default"))
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
			mustLoadProtocolFactories(t, runtime, protocolShortcutFactory("non-reserved.gi.json", "ctrl+v", "Overrides non-reserved"))
			result := runtime.Shortcuts(DefaultProtocolKeybindings())
			if _, ok := result.Shortcuts["ctrl+v"]; !ok || !protocolWarningsContain(result.Warnings, "built-in shortcut for app.clipboard.pasteImage") {
				t.Fatalf("result = %#v", result)
			}
		})

		t.Run("blocks shortcuts for reserved actions even when rebound", func(t *testing.T) {
			runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
			mustLoadProtocolFactories(t, runtime, protocolShortcutFactory("rebound-reserved.gi.json", "ctrl+x", "Conflicts"))
			keybindings := DefaultProtocolKeybindings()
			keybindings["app.interrupt"] = "ctrl+x"
			result := runtime.Shortcuts(keybindings)
			if len(result.Shortcuts) != 0 || !protocolWarningsContain(result.Warnings, "conflicts with built-in") {
				t.Fatalf("result = %#v", result)
			}
		})

		t.Run("blocks shortcuts when reserved key is also bound to non-reserved actions", func(t *testing.T) {
			runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
			mustLoadProtocolFactories(t, runtime, protocolShortcutFactory("shared-reserved.gi.json", "ctrl+p", "Conflicts"))
			result := runtime.Shortcuts(DefaultProtocolKeybindings())
			if len(result.Shortcuts) != 0 || !protocolWarningsContain(result.Warnings, "conflicts with built-in") {
				t.Fatalf("result = %#v", result)
			}
		})

		t.Run("blocks shortcuts when reserved action has multiple keys", func(t *testing.T) {
			runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
			mustLoadProtocolFactories(t, runtime, protocolShortcutFactory("multi-reserved.gi.json", "ctrl+y", "Conflicts"))
			keybindings := DefaultProtocolKeybindings()
			keybindings["app.clear"] = []any{"ctrl+x", "ctrl+y"}
			result := runtime.Shortcuts(keybindings)
			if len(result.Shortcuts) != 0 || !protocolWarningsContain(result.Warnings, "conflicts with built-in") {
				t.Fatalf("result = %#v", result)
			}
		})

		t.Run("warns but allows when non-reserved action has multiple keys", func(t *testing.T) {
			runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
			mustLoadProtocolFactories(t, runtime, protocolShortcutFactory("multi-non-reserved.gi.json", "ctrl+y", "Overrides"))
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
				protocolShortcutFactory("ext1.gi.json", "ctrl+shift+x", "First extension"),
				protocolShortcutFactory("ext2.gi.json", "ctrl+shift+x", "Second extension"),
			)
			result := runtime.Shortcuts(DefaultProtocolKeybindings())
			if got := result.Shortcuts["ctrl+shift+x"].Description; got != "Second extension" || !protocolWarningsContain(result.Warnings, "shortcut conflict") {
				t.Fatalf("result = %#v", result)
			}
		})

		t.Run("replaces shortcut when same source registers same key again", func(t *testing.T) {
			runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
			mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "shortcut.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				if err := ctx.RegisterShortcut("ctrl+shift+x", ProtocolShortcutDefinition{Description: "First"}); err != nil {
					return err
				}
				return ctx.RegisterShortcut("ctrl+shift+x", ProtocolShortcutDefinition{Description: "Second"})
			}})
			result := runtime.Shortcuts(DefaultProtocolKeybindings())
			if got := result.Shortcuts["ctrl+shift+x"].Description; got != "Second" || protocolWarningsContain(result.Warnings, "shortcut conflict") {
				t.Fatalf("result = %#v", result)
			}
		})
	})

	t.Run("collects tools from multiple extensions", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityToolsRegister)
		mustLoadProtocolFactories(t, runtime,
			protocolToolFactory("tool-a.gi.json", "tool_a", "first"),
			protocolToolFactory("tool-b.gi.json", "tool_b", "second"),
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
			protocolToolFactory("a-first.gi.json", "shared", "first"),
			protocolToolFactory("b-second.gi.json", "shared", "second"),
		)
		tools := runtime.RegisteredTools()
		if len(tools) != 1 || tools[0].Description != "first" {
			t.Fatalf("registered tools = %#v", tools)
		}
	})

	t.Run("replaces tool when the same source registers the same name again", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityToolsRegister)
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "tool.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			if err := ctx.RegisterTool(ProtocolToolDefinition{Name: "shared", Description: "first"}); err != nil {
				return err
			}
			return ctx.RegisterTool(ProtocolToolDefinition{Name: "shared", Description: "second"})
		}})
		tools := runtime.RegisteredTools()
		if len(tools) != 1 || tools[0].Description != "second" {
			t.Fatalf("registered tools = %#v", tools)
		}
	})

	t.Run("collects commands from multiple extensions", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityCommandsRegister)
		mustLoadProtocolFactories(t, runtime,
			protocolCommandFactory("cmd-a.gi.json", "cmd-a", "A command"),
			protocolCommandFactory("cmd-b.gi.json", "cmd-b", "B command"),
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

	t.Run("replaces command when the same source registers the same name again", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityCommandsRegister)
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "cmd.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			if err := ctx.RegisterCommand("shared-cmd", ProtocolCommandDefinition{Description: "first"}); err != nil {
				return err
			}
			return ctx.RegisterCommand("shared-cmd", ProtocolCommandDefinition{Description: "second"})
		}})
		commands := runtime.RegisteredCommands()
		if len(commands) != 1 || commands[0].InvocationName != "shared-cmd" || commands[0].Description != "second" {
			t.Fatalf("commands = %#v", commands)
		}
	})

	t.Run("gets command by invocation name", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityCommandsRegister)
		mustLoadProtocolFactories(t, runtime, protocolCommandFactory("cmd.gi.json", "my-cmd", "My command"))
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
			protocolCommandFactory("cmd-a.gi.json", "shared-cmd", "First command"),
			protocolCommandFactory("cmd-b.gi.json", "shared-cmd", "Second command"),
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

	t.Run("increments duplicate suffixes when generated invocation names collide", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityCommandsRegister)
		mustLoadProtocolFactories(t, runtime,
			protocolCommandFactory("cmd-a.gi.json", "shared-cmd", "First command"),
			protocolCommandFactory("cmd-b.gi.json", "shared-cmd:2", "Explicit second command"),
			protocolCommandFactory("cmd-c.gi.json", "shared-cmd", "Second shared command"),
		)
		commands := runtime.RegisteredCommands()
		if len(commands) != 3 {
			t.Fatalf("commands = %#v", commands)
		}
		got := []string{commands[0].InvocationName, commands[1].InvocationName, commands[2].InvocationName}
		want := []string{"shared-cmd:1", "shared-cmd:2", "shared-cmd:3"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("invocations = %#v, want %#v", got, want)
		}
		if runtime.GetCommand("shared-cmd:3").Description != "Second shared command" {
			t.Fatalf("colliding command lookup = %#v", commands)
		}
	})

	t.Run("gets message renderer by type", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityTUIMessageRenderer)
		renderer := func(any, any) []string { return []string{"rendered"} }
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "renderer.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.RegisterMessageRenderer("my-type", renderer)
		}})
		if got := runtime.GetMessageRenderer("my-type"); got == nil || !reflect.DeepEqual(got(nil, nil), []string{"rendered"}) {
			t.Fatalf("message renderer = %#v", got)
		}
		if got := runtime.GetMessageRenderer("not-exists"); got != nil {
			t.Fatalf("missing renderer = %#v", got)
		}
	})

	t.Run("exposes next duplicate message renderer after first source removal", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityTUIMessageRenderer)
		mustLoadProtocolFactories(t, runtime,
			ProtocolExtensionFactory{Path: "first.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.RegisterMessageRenderer("shared.message", func(any, any) []string { return []string{"first"} })
			}},
			ProtocolExtensionFactory{Path: "second.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.RegisterMessageRenderer("shared.message", func(any, any) []string { return []string{"second"} })
			}},
		)
		if got := runtime.GetMessageRenderer("shared.message")(nil, nil); !reflect.DeepEqual(got, []string{"first"}) {
			t.Fatalf("renderer before remove = %#v", got)
		}

		runtime.RemoveSource(ProtocolSourceInfo{Path: "first.gi.json"})

		renderer := runtime.GetMessageRenderer("shared.message")
		if renderer == nil {
			t.Fatal("missing renderer after first source removal")
		}
		if got := renderer(nil, nil); !reflect.DeepEqual(got, []string{"second"}) {
			t.Fatalf("renderer after remove = %#v", got)
		}
	})

	t.Run("exposes next duplicate tool renderer after first source removal", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityTUIToolRenderer)
		mustLoadProtocolFactories(t, runtime,
			ProtocolExtensionFactory{Path: "first.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.RegisterToolRenderer("shared_tool", ProtocolToolRendererDefinition{
					RenderCall: func(any, ToolRenderContext) []string { return []string{"first"} },
				})
			}},
			ProtocolExtensionFactory{Path: "second.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.RegisterToolRenderer("shared_tool", ProtocolToolRendererDefinition{
					RenderCall: func(any, ToolRenderContext) []string { return []string{"second"} },
				})
			}},
		)
		renderer := runtime.GetToolRenderer("shared_tool")
		if renderer == nil || !reflect.DeepEqual(renderer.RenderCall(nil, ToolRenderContext{}), []string{"first"}) {
			t.Fatalf("renderer before remove = %#v", renderer)
		}

		runtime.RemoveSource(ProtocolSourceInfo{Path: "first.gi.json"})

		renderer = runtime.GetToolRenderer("shared_tool")
		if renderer == nil || !reflect.DeepEqual(renderer.RenderCall(nil, ToolRenderContext{}), []string{"second"}) {
			t.Fatalf("renderer after remove = %#v", renderer)
		}
	})

	t.Run("removes registrations by source", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(
			CapabilityCommandsRegister,
			CapabilityProvidersRegister,
			CapabilityToolsRegister,
			CapabilityShortcutsRegister,
			CapabilityTUIAutocomplete,
			CapabilityTUIMessageRenderer,
			CapabilityTUIToolRenderer,
			CapabilityTUIWidget,
		)
		viewHost := NewViewTreeHost()
		runtime.BindViewTreeHost(viewHost)
		mustLoadProtocolFactories(t, runtime,
			ProtocolExtensionFactory{Path: "owned.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				if err := ctx.RegisterCommand("owned", ProtocolCommandDefinition{Handler: func(string) error { return nil }}); err != nil {
					return err
				}
				if err := ctx.RegisterProvider("owned-provider", protocolProviderConfig("owned-model")); err != nil {
					return err
				}
				if err := ctx.RegisterTool(ProtocolToolDefinition{Name: "owned_tool", Description: "Owned tool"}); err != nil {
					return err
				}
				if err := ctx.RegisterMessageRenderer("owned.message", func(any, any) []string { return []string{"owned"} }); err != nil {
					return err
				}
				if err := ctx.RegisterToolRenderer("owned_tool", ProtocolToolRendererDefinition{RenderCall: func(any, ToolRenderContext) []string { return []string{"owned call"} }}); err != nil {
					return err
				}
				if err := ctx.RegisterFlag("owned-flag", ProtocolFlagDefinition{Default: true}); err != nil {
					return err
				}
				if err := ctx.RegisterShortcut("ctrl+shift+y", ProtocolShortcutDefinition{Description: "Owned shortcut"}); err != nil {
					return err
				}
				if err := ctx.RegisterAutocompleteProvider("owned-ac", ProtocolAutocompleteProviderDefinition{Handler: func(context.Context, ProtocolAutocompleteRequest) (ProtocolAutocompleteResult, error) {
					return ProtocolAutocompleteResult{Items: []ProtocolAutocompleteItem{{ID: "owned", Value: "owned"}}}, nil
				}}); err != nil {
					return err
				}
				return ctx.MountViewTree("owned.widget", "widget.aboveEditor", ViewTreeNode{Type: "text", Text: "Owned widget"})
			}},
			protocolCommandFactory("other.gi.json", "other", "Other command"),
		)
		if runtime.GetCommand("owned") == nil || runtime.GetCommand("other") == nil {
			t.Fatalf("commands before remove = %#v", runtime.CommandInvocationNames())
		}
		if len(runtime.RegisteredTools()) != 1 || runtime.GetMessageRenderer("owned.message") == nil || runtime.GetToolRenderer("owned_tool") == nil ||
			len(runtime.Flags()) != 1 || len(runtime.AutocompleteProviders()) != 1 || len(runtime.PendingProviderRegistrations()) != 1 {
			t.Fatalf("registrations before remove: tools=%#v flags=%#v autocomplete=%#v providers=%#v", runtime.RegisteredTools(), runtime.Flags(), runtime.AutocompleteProviders(), runtime.PendingProviderRegistrations())
		}
		if _, ok := viewHost.Mounted("owned.widget"); !ok {
			t.Fatal("expected owned ViewTree mount before remove")
		}

		runtime.RemoveSource(ProtocolSourceInfo{Path: "owned.gi.json"})

		if runtime.GetCommand("owned") != nil || runtime.GetCommand("other") == nil {
			t.Fatalf("commands after remove = %#v", runtime.CommandInvocationNames())
		}
		if len(runtime.RegisteredTools()) != 0 || runtime.GetMessageRenderer("owned.message") != nil || runtime.GetToolRenderer("owned_tool") != nil ||
			len(runtime.Flags()) != 0 || runtime.FlagValue("owned-flag") != nil || len(runtime.AutocompleteProviders()) != 0 || len(runtime.PendingProviderRegistrations()) != 0 {
			t.Fatalf("registrations after remove: tools=%#v flags=%#v value=%#v autocomplete=%#v providers=%#v", runtime.RegisteredTools(), runtime.Flags(), runtime.FlagValue("owned-flag"), runtime.AutocompleteProviders(), runtime.PendingProviderRegistrations())
		}
		if _, ok := viewHost.Mounted("owned.widget"); ok {
			t.Fatal("owned ViewTree mount still present after source removal")
		}
	})

	t.Run("removes lifecycle and input handlers by source", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents, CapabilityInputEvents)
		ownerEvents := 0
		otherEvents := 0
		ownerInputs := 0
		otherInputs := 0
		mustLoadProtocolFactories(t, runtime,
			ProtocolExtensionFactory{Path: "owned.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				if err := ctx.On("context", func(ProtocolSessionEvent) (ProtocolEventResult, error) {
					ownerEvents++
					return ProtocolEventResult{}, nil
				}); err != nil {
					return err
				}
				return ctx.OnInput(func(ProtocolInputEvent) (ProtocolInputResult, error) {
					ownerInputs++
					return ProtocolInputHandled(), nil
				})
			}},
			ProtocolExtensionFactory{Path: "other.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				if err := ctx.On("context", func(ProtocolSessionEvent) (ProtocolEventResult, error) {
					otherEvents++
					return ProtocolEventResult{}, nil
				}); err != nil {
					return err
				}
				return ctx.OnInput(func(event ProtocolInputEvent) (ProtocolInputResult, error) {
					otherInputs++
					return ProtocolInputTransform("other:" + event.Text), nil
				})
			}},
		)
		if _, err := runtime.EmitSessionEvent(ProtocolSessionEvent{Type: "context"}); err != nil {
			t.Fatal(err)
		}
		if result := runtime.EmitInput("hello", nil, "editor"); result.Action != "handled" {
			t.Fatalf("input before remove = %#v", result)
		}

		runtime.RemoveSource(ProtocolSourceInfo{Path: "owned.gi.json"})

		if _, err := runtime.EmitSessionEvent(ProtocolSessionEvent{Type: "context"}); err != nil {
			t.Fatal(err)
		}
		if result := runtime.EmitInput("hello", nil, "editor"); result.Action != "transform" || result.Text != "other:hello" {
			t.Fatalf("input after remove = %#v", result)
		}
		if ownerEvents != 1 || otherEvents != 2 || ownerInputs != 1 || otherInputs != 1 {
			t.Fatalf("events owner=%d other=%d inputs owner=%d other=%d", ownerEvents, otherEvents, ownerInputs, otherInputs)
		}
	})

	t.Run("requires capability for message renderers", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		err := runtime.LoadFactories([]ProtocolExtensionFactory{{Path: "message-renderer.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.RegisterMessageRenderer("my-type", func(any, any) []string { return nil })
		}}})
		if err == nil || !strings.Contains(err.Error(), CapabilityTUIMessageRenderer) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("collects flags from extensions", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		mustLoadProtocolFactories(t, runtime, protocolFlagFactory("with-flag.gi.json", "my-flag", "My flag", nil))
		flags := runtime.Flags()
		if len(flags) != 1 || flags[0].Name != "my-flag" || flags[0].Description != "My flag" {
			t.Fatalf("flags = %#v", flags)
		}
	})

	t.Run("keeps first flag when two extensions register the same name", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		mustLoadProtocolFactories(t, runtime,
			protocolFlagFactory("a-first.gi.json", "shared-flag", "first", true),
			protocolFlagFactory("b-second.gi.json", "shared-flag", "second", false),
		)
		flags := runtime.Flags()
		if len(flags) != 1 || flags[0].Description != "first" || runtime.FlagValue("shared-flag") != true {
			t.Fatalf("flags = %#v value=%#v", flags, runtime.FlagValue("shared-flag"))
		}
	})

	t.Run("exposes next duplicate flag after first source removal", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		mustLoadProtocolFactories(t, runtime,
			protocolFlagFactory("a-first.gi.json", "shared-flag", "first", true),
			protocolFlagFactory("b-second.gi.json", "shared-flag", "second", false),
		)
		if flags := runtime.Flags(); len(flags) != 1 || flags[0].Description != "first" || runtime.FlagValue("shared-flag") != true {
			t.Fatalf("flags before remove = %#v value=%#v", flags, runtime.FlagValue("shared-flag"))
		}

		runtime.RemoveSource(ProtocolSourceInfo{Path: "a-first.gi.json"})

		flags := runtime.Flags()
		if len(flags) != 1 || flags[0].Description != "second" || runtime.FlagValue("shared-flag") != false {
			t.Fatalf("flags after remove = %#v value=%#v", flags, runtime.FlagValue("shared-flag"))
		}
	})

	t.Run("can set flag values", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		mustLoadProtocolFactories(t, runtime, protocolFlagFactory("flag.gi.json", "test-flag", "Test flag", nil))
		runtime.SetFlagValue("--test-flag", true)
		if got := runtime.FlagValue("test-flag"); got != true {
			t.Fatalf("flag value = %#v", got)
		}
	})

	t.Run("applies cli flag values to registered flags", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		mustLoadProtocolFactories(t, runtime,
			protocolTypedFlagFactory("bool.gi.json", "review-mode", "Review mode", "boolean", false),
			protocolTypedFlagFactory("string.gi.json", "profile", "Profile", "string", "default"),
		)
		diagnostics := runtime.SetCLIFlagValues(map[string]any{
			"review-mode": "ignored",
			"profile":     "fast",
		})
		if len(diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v", diagnostics)
		}
		if got := runtime.FlagValue("review-mode"); got != true {
			t.Fatalf("review-mode = %#v", got)
		}
		if got := runtime.FlagValue("profile"); got != "fast" {
			t.Fatalf("profile = %#v", got)
		}
	})

	t.Run("keeps cli flag values pending until process flags register", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		runtime.SetCLIFlagValues(map[string]any{"late-profile": "deep"})
		mustLoadProtocolFactories(t, runtime, protocolTypedFlagFactory("late.gi.json", "--late-profile", "Late profile", "string", nil))
		if got := runtime.FlagValue("late-profile"); got != "deep" {
			t.Fatalf("late-profile = %#v", got)
		}
	})

	t.Run("diagnoses string flags without values", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		mustLoadProtocolFactories(t, runtime, protocolTypedFlagFactory("string.gi.json", "profile", "Profile", "string", nil))
		diagnostics := runtime.SetCLIFlagValues(map[string]any{"profile": true})
		if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, `--profile`) {
			t.Fatalf("diagnostics = %#v", diagnostics)
		}
	})

	t.Run("extension context reads only flags registered by the same source", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		var ownerCtx *ProtocolExtensionContext
		var otherCtx *ProtocolExtensionContext
		mustLoadProtocolFactories(t, runtime,
			ProtocolExtensionFactory{Path: "owner.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				ownerCtx = ctx
				return ctx.RegisterFlag("review-mode", ProtocolFlagDefinition{Type: "boolean", Default: true})
			}},
			ProtocolExtensionFactory{Path: "other.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				otherCtx = ctx
				return nil
			}},
		)
		if got := ownerCtx.GetFlag("--review-mode"); got != true {
			t.Fatalf("owner flag = %#v", got)
		}
		if got := otherCtx.GetFlag("review-mode"); got != nil {
			t.Fatalf("other flag = %#v", got)
		}
	})

	t.Run("rpc get_flag returns values for the owning process source", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		runtime.SetCLIFlagValues(map[string]any{"profile": "fast"})
		source := ProtocolSourceInfo{Path: "process.gi.json", Source: "rpc", Scope: "project", Origin: "profile-extension"}
		processor := RPCLineProcessor{Runtime: runtime, SourceInfo: source}
		register := processor.handleExtensionRegistration(HostActionRequest{
			ID:       "register_flag",
			Protocol: "gi-ext-rpc@1",
			Method:   "register_flag",
			Params:   mustProtocolTestRawJSON(t, map[string]any{"name": "--profile", "type": "string", "default": "default"}),
		})
		if register.Error != nil {
			t.Fatalf("register response = %#v", register)
		}

		get := processor.handleExtensionRegistration(HostActionRequest{
			ID:       "get_flag",
			Protocol: "gi-ext-rpc@1",
			Method:   "get_flag",
			Params:   mustProtocolTestRawJSON(t, map[string]any{"name": "profile"}),
		})
		result, ok := get.Result.(map[string]any)
		if get.Error != nil || !ok || result["value"] != "fast" || result["set"] != true {
			t.Fatalf("get response = %#v", get)
		}

		other := RPCLineProcessor{Runtime: runtime, SourceInfo: ProtocolSourceInfo{Path: "other-process.gi.json"}}
		get = other.handleExtensionRegistration(HostActionRequest{
			ID:       "get_flag_other",
			Protocol: "gi-ext-rpc@1",
			Method:   "get_flag",
			Params:   mustProtocolTestRawJSON(t, map[string]any{"name": "profile"}),
		})
		result, ok = get.Result.(map[string]any)
		if get.Error != nil || !ok || result["value"] != nil || result["set"] != false {
			t.Fatalf("other get response = %#v", get)
		}
	})

	t.Run("rpc diagnostic errors emit protocol extension errors with stack", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		source := ProtocolSourceInfo{Path: "process.gi.json", Source: "rpc", Scope: "project", Origin: "diagnostic-extension"}
		processor := RPCLineProcessor{Runtime: runtime, SourceInfo: source}
		var got []ProtocolExtensionError
		runtime.OnError(func(event ProtocolExtensionError) {
			got = append(got, event)
		})

		processor.HandleLine(context.Background(), `{"type":"diagnostic","protocol":"gi-ext-rpc@1","severity":"warning","code":"warn","message":"ignore me"}`)
		if len(got) != 0 {
			t.Fatalf("warning diagnostics should not emit extension errors: %#v", got)
		}

		processor.HandleLine(context.Background(), `{"type":"diagnostic","protocol":"gi-ext-rpc@1","severity":"error","code":"handler_error","message":"boom","stack":"Error: boom\n    at run (/pkg/index.js:1:2)"}`)
		if len(got) != 1 || got[0].ExtensionPath != "process.gi.json" || got[0].Event != "handler_error" || got[0].Error != "boom" || !strings.Contains(got[0].Stack, "at run") {
			t.Fatalf("errors = %#v", got)
		}
	})

	t.Run("returns true when handlers exist for event type", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
		if runtime.HasHandlers("tool_call") {
			t.Fatal("empty runtime should not have tool_call handlers")
		}
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "handler.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.On("tool_call", func(ProtocolSessionEvent) (ProtocolEventResult, error) {
				return ProtocolEventResult{}, nil
			})
		}})
		if !runtime.HasHandlers("tool_call") || runtime.HasHandlers("agent_end") {
			t.Fatalf("handler presence: tool_call=%v agent_end=%v", runtime.HasHandlers("tool_call"), runtime.HasHandlers("agent_end"))
		}
	})

	t.Run("allows user bash lifecycle handlers to return replacement results", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
		var got ProtocolSessionEvent
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "bash.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.On(ProtocolEventUserBash, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
				got = event
				return ProtocolEventResult{
					BashResult:    &BashResult{Output: "from extension\n", ExitCode: 0},
					BashResultSet: true,
				}, nil
			})
		}})

		result, err := runtime.EmitSessionEvent(ProtocolSessionEvent{
			Type:               ProtocolEventUserBash,
			Command:            "printf hi",
			CWD:                "/tmp/project",
			ExcludeFromContext: true,
		})
		if err != nil {
			t.Fatalf("EmitSessionEvent error: %v", err)
		}
		if got.Command != "printf hi" || got.CWD != "/tmp/project" || !got.ExcludeFromContext {
			t.Fatalf("event = %#v", got)
		}
		if !result.BashResultSet || result.BashResult == nil || result.BashResult.Output != "from extension\n" || result.BashResult.ExitCode != 0 {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("collects resources_discover results with source metadata", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
		mustLoadProtocolFactories(t, runtime,
			ProtocolExtensionFactory{Path: "dynamic-a.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.On(ProtocolEventResourcesDiscover, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
					if event.Reason != "startup" || event.CWD != "/tmp/project" {
						t.Fatalf("event = %#v", event)
					}
					return ProtocolEventResult{
						ResourcesSet: true,
						Resources: ResourceExtension{
							SkillPaths:  []ResourceSkillPath{{Path: "skills/a"}},
							PromptPaths: []ResourcePromptPath{{Path: "prompts/a.md"}},
						},
					}, nil
				})
			}},
			ProtocolExtensionFactory{Path: "dynamic-b.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.On(ProtocolEventResourcesDiscover, func(ProtocolSessionEvent) (ProtocolEventResult, error) {
					return ProtocolEventResult{
						ResourcesSet: true,
						Resources: ResourceExtension{
							ThemePaths: []ResourceThemePath{{Path: "themes/b.json", Metadata: ProtocolSourceInfo{Source: "extension:b"}}},
						},
					}, nil
				})
			}},
		)

		result, err := runtime.EmitSessionEvent(ProtocolSessionEvent{Type: ProtocolEventResourcesDiscover, Reason: "startup", CWD: "/tmp/project"})
		if err != nil {
			t.Fatalf("EmitSessionEvent error: %v", err)
		}
		if !result.ResourcesSet ||
			len(result.Resources.SkillPaths) != 1 ||
			len(result.Resources.PromptPaths) != 1 ||
			len(result.Resources.ThemePaths) != 1 {
			t.Fatalf("resources = %#v", result.Resources)
		}
		skillMetadata := result.Resources.SkillPaths[0].Metadata
		if skillMetadata.Path != "skills/a" || skillMetadata.Source != "inline" || skillMetadata.Scope != "temporary" || skillMetadata.Origin != "top-level" {
			t.Fatalf("skill metadata = %#v", skillMetadata)
		}
		themeMetadata := result.Resources.ThemePaths[0].Metadata
		if themeMetadata.Path != "themes/b.json" || themeMetadata.Source != "extension:b" || themeMetadata.Scope != "temporary" || themeMetadata.Origin != "top-level" {
			t.Fatalf("theme metadata = %#v", themeMetadata)
		}
	})

	t.Run("exposes current abort signal on extension context", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		done := make(chan struct{})
		runtime.SetAbortSignal(done)

		var signal *ProtocolAbortSignal
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "signal.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			signal = ctx.Signal()
			return nil
		}})
		if signal == nil || signal.Aborted() {
			t.Fatalf("signal = %#v aborted=%v", signal, signal != nil && signal.Aborted())
		}
		close(done)
		if !signal.Aborted() {
			t.Fatal("expected signal to observe abort after host closes it")
		}
	})

	t.Run("chains system prompt updates through context getter", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents, CapabilitySystemPromptModify)
		mustLoadProtocolFactories(t, runtime,
			ProtocolExtensionFactory{Path: "first.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.On("before_agent_start", func(ProtocolSessionEvent) (ProtocolEventResult, error) {
					return ProtocolEventResult{SystemPrompt: ctx.GetSystemPrompt() + "\nfirst", SystemPromptSet: true}, nil
				})
			}},
			ProtocolExtensionFactory{Path: "second.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.On("before_agent_start", func(ProtocolSessionEvent) (ProtocolEventResult, error) {
					return ProtocolEventResult{SystemPrompt: ctx.GetSystemPrompt() + "\nsecond", SystemPromptSet: true}, nil
				})
			}},
		)

		result, err := runtime.EmitSessionEvent(ProtocolSessionEvent{Type: "before_agent_start", Prompt: "hello", SystemPrompt: "base"})
		if err != nil {
			t.Fatalf("EmitSessionEvent error: %v", err)
		}
		if !result.SystemPromptSet || result.SystemPrompt != "base\nfirst\nsecond" {
			t.Fatalf("result = %#v", result)
		}
		if got := runtime.GetSystemPrompt(); got != "" {
			t.Fatalf("runtime prompt leaked after event = %q", got)
		}
	})

	t.Run("rejects system prompt updates without capability", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "prompt.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.On("before_agent_start", func(ProtocolSessionEvent) (ProtocolEventResult, error) {
				return ProtocolEventResult{SystemPrompt: "changed", SystemPromptSet: true}, nil
			})
		}})
		var got []ProtocolExtensionError
		runtime.OnError(func(event ProtocolExtensionError) {
			got = append(got, event)
		})
		if _, err := runtime.EmitSessionEvent(ProtocolSessionEvent{Type: "before_agent_start", SystemPrompt: "base"}); err == nil {
			t.Fatal("expected missing capability error")
		}
		if len(got) != 1 || got[0].ExtensionPath != "prompt.gi.json" || got[0].Event != "before_agent_start" || !strings.Contains(got[0].Error, CapabilitySystemPromptModify) {
			t.Fatalf("errors = %#v", got)
		}
	})

	t.Run("chains tool result content modifications across handlers", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
		mustLoadProtocolFactories(t, runtime,
			ProtocolExtensionFactory{Path: "tool-result-1.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.On("tool_result", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
					content := append(cloneSDKContentParts(event.Content), SDKContentPart{Type: "text", Text: "ext1"})
					return ProtocolEventResult{Content: content, ContentSet: true}, nil
				})
			}},
			ProtocolExtensionFactory{Path: "tool-result-2.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.On("tool_result", func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
					content := append(cloneSDKContentParts(event.Content), SDKContentPart{Type: "text", Text: "ext2"})
					return ProtocolEventResult{Content: content, ContentSet: true}, nil
				})
			}},
		)

		result, err := runtime.EmitSessionEvent(ProtocolSessionEvent{
			Type:       "tool_result",
			ToolName:   "my_tool",
			ToolCallID: "call-1",
			Content:    []SDKContentPart{{Type: "text", Text: "base"}},
			Details:    map[string]any{"initial": true},
		})
		if err != nil {
			t.Fatalf("EmitSessionEvent error: %v", err)
		}
		if !result.ContentSet || len(result.Content) != 3 {
			t.Fatalf("result = %#v", result)
		}
		if got := []string{result.Content[0].Text, result.Content[1].Text, result.Content[2].Text}; !reflect.DeepEqual(got, []string{"base", "ext1", "ext2"}) {
			t.Fatalf("content text = %#v", got)
		}
	})

	t.Run("preserves previous tool result modifications when later handlers return partial patches", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
		details := map[string]any{"source": "ext1"}
		mustLoadProtocolFactories(t, runtime,
			ProtocolExtensionFactory{Path: "tool-result-partial-1.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.On("tool_result", func(ProtocolSessionEvent) (ProtocolEventResult, error) {
					return ProtocolEventResult{
						Content:    []SDKContentPart{{Type: "text", Text: "first"}},
						ContentSet: true,
						Details:    details,
						DetailsSet: true,
					}, nil
				})
			}},
			ProtocolExtensionFactory{Path: "tool-result-partial-2.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.On("tool_result", func(ProtocolSessionEvent) (ProtocolEventResult, error) {
					return ProtocolEventResult{IsError: true, IsErrorSet: true}, nil
				})
			}},
		)

		result, err := runtime.EmitSessionEvent(ProtocolSessionEvent{
			Type:    "tool_result",
			Content: []SDKContentPart{{Type: "text", Text: "base"}},
			Details: map[string]any{"initial": true},
		})
		if err != nil {
			t.Fatalf("EmitSessionEvent error: %v", err)
		}
		if !result.ContentSet || !result.DetailsSet || !result.IsErrorSet || !result.IsError {
			t.Fatalf("result flags = %#v", result)
		}
		if len(result.Content) != 1 || result.Content[0].Text != "first" || !reflect.DeepEqual(result.Details, details) {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("bind model registry ignores invalid queued provider registrations and reports errors", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityProvidersRegister)
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "broken-provider.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.RegisterProvider("broken-provider", ProtocolProviderOverride{
				Models: []ProviderModelDefinition{{ID: "broken-model", Name: "Broken Model"}},
			})
		}})
		if len(runtime.PendingProviderRegistrations()) != 1 {
			t.Fatalf("pending providers = %#v", runtime.PendingProviderRegistrations())
		}

		var got []ProtocolExtensionError
		runtime.OnError(func(event ProtocolExtensionError) {
			got = append(got, event)
		})
		registry := NewInMemoryModelRegistry(nil)
		runtime.BindModelRegistry(registry)

		if len(runtime.PendingProviderRegistrations()) != 0 {
			t.Fatalf("pending providers after bind = %#v", runtime.PendingProviderRegistrations())
		}
		if _, ok := registry.Find("broken-provider", "broken-model"); ok {
			t.Fatal("invalid queued provider should not be registered")
		}
		if len(got) != 1 || got[0].ExtensionPath != "broken-provider.gi.json" || got[0].Event != "register_provider" || !strings.Contains(got[0].Error, `no "api" specified`) {
			t.Fatalf("errors = %#v", got)
		}
	})

	t.Run("pre-bind unregister removes all queued provider registrations", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityProvidersRegister)
		mustLoadProtocolFactories(t, runtime,
			protocolProviderFactory("queued-1.gi.json", "queued-provider", protocolProviderConfig("queued-model-1")),
			protocolProviderFactory("queued-2.gi.json", "queued-provider", protocolProviderConfig("queued-model-2")),
		)
		if len(runtime.PendingProviderRegistrations()) != 2 {
			t.Fatalf("pending providers before unregister = %#v", runtime.PendingProviderRegistrations())
		}
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "unregister.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.UnregisterProvider("queued-provider")
		}})
		if len(runtime.PendingProviderRegistrations()) != 0 {
			t.Fatalf("pending providers after unregister = %#v", runtime.PendingProviderRegistrations())
		}
	})

	t.Run("pre-bind remove source keeps earlier duplicate provider registration", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityProvidersRegister)
		mustLoadProtocolFactories(t, runtime,
			protocolProviderFactory("queued-1.gi.json", "queued-provider", protocolProviderConfig("queued-model-1")),
			protocolProviderFactory("queued-2.gi.json", "queued-provider", protocolProviderConfig("queued-model-2")),
		)
		runtime.RemoveSource(ProtocolSourceInfo{Path: "queued-2.gi.json"})
		pending := runtime.PendingProviderRegistrations()
		if len(pending) != 1 || pending[0].Name != "queued-provider" || len(pending[0].Config.Models) != 1 || pending[0].Config.Models[0].ID != "queued-model-1" {
			t.Fatalf("pending providers after source removal = %#v", pending)
		}

		registry := NewInMemoryModelRegistry(nil)
		runtime.BindModelRegistry(registry)
		if _, ok := registry.Find("queued-provider", "queued-model-1"); !ok {
			t.Fatal("remaining queued provider was not applied after bind")
		}
		if _, ok := registry.Find("queued-provider", "queued-model-2"); ok {
			t.Fatal("removed queued provider should not be applied after bind")
		}
	})

	t.Run("post-bind provider register and unregister take effect immediately", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityProvidersRegister)
		registry := NewInMemoryModelRegistry(nil)
		runtime.BindModelRegistry(registry)

		mustLoadProtocolFactories(t, runtime, protocolProviderFactory("instant.gi.json", "instant-provider", protocolProviderConfig("instant-model")))
		if len(runtime.PendingProviderRegistrations()) != 0 {
			t.Fatalf("pending providers = %#v", runtime.PendingProviderRegistrations())
		}
		if _, ok := registry.Find("instant-provider", "instant-model"); !ok {
			t.Fatal("instant provider model was not registered")
		}

		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "unregister.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.UnregisterProvider("instant-provider")
		}})
		if _, ok := registry.Find("instant-provider", "instant-model"); ok {
			t.Fatal("instant provider model should be unregistered")
		}
	})

	t.Run("post-bind remove source restores earlier duplicate provider registration", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityProvidersRegister)
		registry := NewInMemoryModelRegistry(nil)
		runtime.BindModelRegistry(registry)

		mustLoadProtocolFactories(t, runtime,
			protocolProviderFactory("first.gi.json", "shared-provider", protocolProviderConfig("first-model")),
			protocolProviderFactory("second.gi.json", "shared-provider", protocolProviderConfig("second-model")),
		)
		if _, ok := registry.Find("shared-provider", "second-model"); !ok {
			t.Fatal("second provider model was not active before source removal")
		}
		if _, ok := registry.Find("shared-provider", "first-model"); ok {
			t.Fatal("first provider model should be replaced before source removal")
		}

		runtime.RemoveSource(ProtocolSourceInfo{Path: "second.gi.json"})

		if _, ok := registry.Find("shared-provider", "first-model"); !ok {
			t.Fatal("first provider model was not restored after source removal")
		}
		if _, ok := registry.Find("shared-provider", "second-model"); ok {
			t.Fatal("removed provider model should not remain after source removal")
		}
	})

	t.Run("requires capability for autocomplete providers", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		err := runtime.LoadFactories([]ProtocolExtensionFactory{{Path: "autocomplete.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.RegisterAutocompleteProvider("issues", ProtocolAutocompleteProviderDefinition{
				Handler: func(context.Context, ProtocolAutocompleteRequest) (ProtocolAutocompleteResult, error) {
					return ProtocolAutocompleteResult{}, nil
				},
			})
		}}})
		if err == nil || !strings.Contains(err.Error(), CapabilityTUIAutocomplete) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("orders autocomplete providers and returns first non-empty suggestions", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityTUIAutocomplete)
		var calls []string
		mustLoadProtocolFactories(t, runtime,
			ProtocolExtensionFactory{Path: "low.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.RegisterAutocompleteProvider("low", ProtocolAutocompleteProviderDefinition{
					Description: "Low priority",
					Priority:    10,
					Handler: func(_ context.Context, request ProtocolAutocompleteRequest) (ProtocolAutocompleteResult, error) {
						calls = append(calls, "low:"+request.Text)
						return ProtocolAutocompleteResult{Items: []ProtocolAutocompleteItem{{ID: "low", Value: "low"}}}, nil
					},
				})
			}},
			ProtocolExtensionFactory{Path: "high.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.RegisterAutocompleteProvider("high", ProtocolAutocompleteProviderDefinition{
					Description: "High priority",
					Priority:    90,
					Handler: func(_ context.Context, request ProtocolAutocompleteRequest) (ProtocolAutocompleteResult, error) {
						calls = append(calls, "high:"+request.Text)
						return ProtocolAutocompleteResult{}, nil
					},
				})
			}},
		)

		providers := runtime.AutocompleteProviders()
		if len(providers) != 2 || providers[0].ID != "high" || providers[1].ID != "low" {
			t.Fatalf("providers = %#v", providers)
		}
		result, err := runtime.SuggestAutocomplete(context.Background(), ProtocolAutocompleteRequest{
			Text:       "/pla",
			Lines:      []string{"/pla"},
			CursorLine: 0,
			CursorCol:  4,
			Force:      true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Items) != 1 || result.Items[0].Value != "low" {
			t.Fatalf("result = %#v", result)
		}
		if !reflect.DeepEqual(calls, []string{"high:/pla", "low:/pla"}) {
			t.Fatalf("calls = %#v", calls)
		}
	})

	t.Run("notifies autocomplete provider watchers", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityTUIAutocomplete)
		var changes int
		unwatch := runtime.OnAutocompleteProvidersChanged(func() {
			changes++
		})
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "first.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.RegisterAutocompleteProvider("first", ProtocolAutocompleteProviderDefinition{
				Handler: func(context.Context, ProtocolAutocompleteRequest) (ProtocolAutocompleteResult, error) {
					return ProtocolAutocompleteResult{}, nil
				},
			})
		}})
		if changes != 1 {
			t.Fatalf("changes = %d, want 1", changes)
		}
		unwatch()
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "second.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.RegisterAutocompleteProvider("second", ProtocolAutocompleteProviderDefinition{
				Handler: func(context.Context, ProtocolAutocompleteRequest) (ProtocolAutocompleteResult, error) {
					return ProtocolAutocompleteResult{}, nil
				},
			})
		}})
		if changes != 1 {
			t.Fatalf("changes after unwatch = %d, want 1", changes)
		}
	})

	t.Run("replaces autocomplete provider when same source registers same id again", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityTUIAutocomplete)
		var calls []string
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "autocomplete.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			if err := ctx.RegisterAutocompleteProvider("issues", ProtocolAutocompleteProviderDefinition{
				Description: "First",
				Handler: func(context.Context, ProtocolAutocompleteRequest) (ProtocolAutocompleteResult, error) {
					calls = append(calls, "first")
					return ProtocolAutocompleteResult{Items: []ProtocolAutocompleteItem{{ID: "first", Value: "first"}}}, nil
				},
			}); err != nil {
				return err
			}
			return ctx.RegisterAutocompleteProvider("issues", ProtocolAutocompleteProviderDefinition{
				Description: "Second",
				Handler: func(context.Context, ProtocolAutocompleteRequest) (ProtocolAutocompleteResult, error) {
					calls = append(calls, "second")
					return ProtocolAutocompleteResult{Items: []ProtocolAutocompleteItem{{ID: "second", Value: "second"}}}, nil
				},
			})
		}})
		providers := runtime.AutocompleteProviders()
		if len(providers) != 1 || providers[0].Description != "Second" {
			t.Fatalf("providers = %#v", providers)
		}
		result, err := runtime.SuggestAutocomplete(context.Background(), ProtocolAutocompleteRequest{Text: "x"})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(calls, []string{"second"}) || len(result.Items) != 1 || result.Items[0].Value != "second" {
			t.Fatalf("calls = %#v result = %#v", calls, result)
		}
	})

	t.Run("passes fork options through command context handler", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		type forkCall struct {
			entryID string
			options ProtocolForkOptions
		}
		var calls []forkCall
		runtime.BindCommandContext(ProtocolCommandContextActions{Fork: func(entryID string, options ProtocolForkOptions) (ProtocolCommandForkResult, error) {
			calls = append(calls, forkCall{entryID: entryID, options: options})
			return ProtocolCommandForkResult{}, nil
		}})

		commandContext := runtime.CreateCommandContext()
		if _, err := commandContext.Fork("entry-1"); err != nil {
			t.Fatal(err)
		}
		if _, err := commandContext.Fork("entry-2", ProtocolForkOptions{Position: "at"}); err != nil {
			t.Fatal(err)
		}
		want := []forkCall{{entryID: "entry-1"}, {entryID: "entry-2", options: ProtocolForkOptions{Position: "at"}}}
		if !reflect.DeepEqual(calls, want) {
			t.Fatalf("calls = %#v", calls)
		}
	})

	t.Run("calls error listeners when handler throws", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "throws.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
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
		if len(got) != 1 || got[0].ExtensionPath != "throws.gi.json" || got[0].Event != "context" || !strings.Contains(got[0].Error, "Handler error!") {
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

func protocolTypedFlagFactory(path, name, description, flagType string, defaultValue any) ProtocolExtensionFactory {
	return ProtocolExtensionFactory{Path: path, Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.RegisterFlag(name, ProtocolFlagDefinition{Description: description, Type: flagType, Default: defaultValue})
	}}
}

func protocolProviderFactory(path, name string, config ProtocolProviderOverride) ProtocolExtensionFactory {
	return ProtocolExtensionFactory{Path: path, Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.RegisterProvider(name, config)
	}}
}

func protocolProviderConfig(modelID string) ProtocolProviderOverride {
	return ProtocolProviderOverride{
		BaseURL: "https://provider.test/v1",
		APIKey:  "TEST_KEY",
		API:     "openai-completions",
		Models: []ProviderModelDefinition{{
			ID:            modelID,
			Name:          "Test " + modelID,
			ContextWindow: 128000,
			MaxTokens:     4096,
		}},
	}
}

func TestProtocolProviderOverridePreservesExplicitEmptyState(
	t *testing.T,
) {
	enabled := true
	disabled := false
	merged := mergeProtocolProviderOverride(
		ProtocolProviderOverride{
			Headers:    map[string]string{"X-Old": "stale"},
			AuthHeader: &enabled,
			Models: []ProviderModelDefinition{{
				ID: "old-model",
			}},
		},
		ProtocolProviderOverride{
			Headers:    map[string]string{},
			AuthHeader: &disabled,
			Models:     []ProviderModelDefinition{},
		},
	)
	if merged.Headers == nil || len(merged.Headers) != 0 {
		t.Fatalf("headers = %#v, want explicit empty", merged.Headers)
	}
	if merged.AuthHeader == nil || *merged.AuthHeader {
		t.Fatalf("authHeader = %#v, want explicit false", merged.AuthHeader)
	}
	if merged.Models == nil || len(merged.Models) != 0 {
		t.Fatalf("models = %#v, want explicit empty", merged.Models)
	}

	config := merged.toProviderConfigInput()
	if config.Models == nil || len(config.Models) != 0 {
		t.Fatalf("provider models = %#v, want explicit empty", config.Models)
	}
}

func protocolShortcutFactory(path, key, description string) ProtocolExtensionFactory {
	return ProtocolExtensionFactory{Path: path, Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.RegisterShortcut(key, ProtocolShortcutDefinition{Description: description})
	}}
}

func mustProtocolTestRawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func protocolWarningsContain(warnings []ProtocolShortcutWarning, text string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning.Message, text) {
			return true
		}
	}
	return false
}
