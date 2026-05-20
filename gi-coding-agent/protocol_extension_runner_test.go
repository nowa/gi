package gicodingagent

import (
	"reflect"
	"sort"
	"testing"
)

func TestProtocolExtensionRunnerRegistryPiParity(t *testing.T) {
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
