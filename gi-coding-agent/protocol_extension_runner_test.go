package gicodingagent

import (
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

	t.Run("gets message renderer by type", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
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

	t.Run("can set flag values", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime()
		mustLoadProtocolFactories(t, runtime, protocolFlagFactory("flag.gi.json", "test-flag", "Test flag", nil))
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
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "handler.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.On("tool_call", func(ProtocolSessionEvent) (ProtocolEventResult, error) {
				return ProtocolEventResult{}, nil
			})
		}})
		if !runtime.HasHandlers("tool_call") || runtime.HasHandlers("agent_end") {
			t.Fatalf("handler presence: tool_call=%v agent_end=%v", runtime.HasHandlers("tool_call"), runtime.HasHandlers("agent_end"))
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
