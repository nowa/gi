package gicodingagent

import (
	"fmt"
	"sort"
)

const CapabilityCommandsRegister = "commands.register"

type ProtocolExtensionRuntime struct {
	capabilities map[string]bool
	commands     []ProtocolCommandRegistration
}

type ProtocolExtensionFactory struct {
	Path    string
	Factory func(*ProtocolExtensionContext) error
}

type ProtocolExtensionContext struct {
	runtime *ProtocolExtensionRuntime
	source  ProtocolSourceInfo
}

type ProtocolSourceInfo struct {
	Path string
}

type ProtocolCommandDefinition struct {
	Description string
	Handler     func(args string) error
}

type ProtocolCommandRegistration struct {
	Name           string
	InvocationName string
	Description    string
	SourceInfo     ProtocolSourceInfo
	Handler        func(args string) error
}

type ProtocolRuntimeError struct {
	Code    string
	Message string
}

func (e ProtocolRuntimeError) Error() string {
	return e.Code + ": " + e.Message
}

func NewProtocolExtensionRuntime(capabilities ...string) *ProtocolExtensionRuntime {
	runtime := &ProtocolExtensionRuntime{capabilities: map[string]bool{}}
	for _, capability := range capabilities {
		runtime.capabilities[capability] = true
	}
	return runtime
}

func (r *ProtocolExtensionRuntime) LoadFactories(factories []ProtocolExtensionFactory) error {
	for _, factory := range factories {
		if factory.Factory == nil {
			continue
		}
		context := &ProtocolExtensionContext{
			runtime: r,
			source:  ProtocolSourceInfo{Path: factory.Path},
		}
		if err := factory.Factory(context); err != nil {
			return err
		}
	}
	return nil
}

func (c *ProtocolExtensionContext) RegisterCommand(name string, definition ProtocolCommandDefinition) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	if !c.runtime.capabilities[CapabilityCommandsRegister] {
		return ProtocolRuntimeError{Code: "missing_capability", Message: CapabilityCommandsRegister}
	}
	c.runtime.commands = append(c.runtime.commands, ProtocolCommandRegistration{
		Name:        name,
		Description: definition.Description,
		SourceInfo:  c.source,
		Handler:     definition.Handler,
	})
	return nil
}

func (r *ProtocolExtensionRuntime) RegisteredCommands() []ProtocolCommandRegistration {
	if r == nil {
		return nil
	}
	counts := map[string]int{}
	for _, command := range r.commands {
		counts[command.Name]++
	}
	ordinals := map[string]int{}
	result := make([]ProtocolCommandRegistration, 0, len(r.commands))
	for _, command := range r.commands {
		ordinals[command.Name]++
		command.InvocationName = command.Name
		if counts[command.Name] > 1 {
			command.InvocationName = fmt.Sprintf("%s:%d", command.Name, ordinals[command.Name])
		}
		result = append(result, command)
	}
	return result
}

func (r *ProtocolExtensionRuntime) GetCommand(invocationName string) *ProtocolCommandRegistration {
	for _, command := range r.RegisteredCommands() {
		if command.InvocationName == invocationName {
			copy := command
			return &copy
		}
	}
	return nil
}

func (r *ProtocolExtensionRuntime) CommandInvocationNames() []string {
	commands := r.RegisteredCommands()
	names := make([]string, 0, len(commands))
	for _, command := range commands {
		names = append(names, command.InvocationName)
	}
	sort.Strings(names)
	return names
}
