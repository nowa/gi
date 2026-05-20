package gicodingagent

import (
	"fmt"
	"sort"
)

const CapabilityCommandsRegister = "commands.register"
const CapabilityLifecycleEvents = "lifecycle.events"
const CapabilityProvidersRegister = "providers.register"
const CapabilityToolsRegister = "tools.register"

type ProtocolExtensionRuntime struct {
	capabilities      map[string]bool
	commands          []ProtocolCommandRegistration
	handlers          map[string][]ProtocolEventHandler
	providerOverrides map[string]ProtocolProviderOverride
	tools             []SDKTool
	boundSession      *AgentSession
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
	Path   string
	Source string
	Scope  string
	Origin string
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

type ProtocolProviderOverride struct {
	BaseURL string
}

type ProtocolToolDefinition struct {
	Name             string
	Label            string
	Description      string
	PromptSnippet    string
	PromptGuidelines []string
	Execute          func(toolCallID string, input map[string]any) (SDKToolResult, error)
}

type ProtocolSessionEvent struct {
	Type                string
	Reason              string
	TargetSessionFile   string
	PreviousSessionFile string
	EntryID             string
	Position            string
}

type ProtocolEventResult struct {
	Cancel bool
}

type ProtocolEventHandler func(ProtocolSessionEvent) (ProtocolEventResult, error)

type ProtocolRuntimeError struct {
	Code    string
	Message string
}

func (e ProtocolRuntimeError) Error() string {
	return e.Code + ": " + e.Message
}

func NewProtocolExtensionRuntime(capabilities ...string) *ProtocolExtensionRuntime {
	runtime := &ProtocolExtensionRuntime{
		capabilities:      map[string]bool{},
		handlers:          map[string][]ProtocolEventHandler{},
		providerOverrides: map[string]ProtocolProviderOverride{},
	}
	for _, capability := range capabilities {
		runtime.capabilities[capability] = true
	}
	return runtime
}

func (r *ProtocolExtensionRuntime) BindSession(session *AgentSession) {
	if r == nil {
		return
	}
	r.boundSession = session
	r.ApplyToSession(session)
}

func (c *ProtocolExtensionContext) On(eventType string, handler ProtocolEventHandler) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	if !c.runtime.capabilities[CapabilityLifecycleEvents] {
		return ProtocolRuntimeError{Code: "missing_capability", Message: CapabilityLifecycleEvents}
	}
	if handler == nil {
		return nil
	}
	c.runtime.handlers[eventType] = append(c.runtime.handlers[eventType], handler)
	return nil
}

func (r *ProtocolExtensionRuntime) EmitSessionEvent(event ProtocolSessionEvent) (ProtocolEventResult, error) {
	if r == nil {
		return ProtocolEventResult{}, nil
	}
	var combined ProtocolEventResult
	for _, handler := range r.handlers[event.Type] {
		result, err := handler(event)
		if err != nil {
			return ProtocolEventResult{}, err
		}
		if result.Cancel {
			combined.Cancel = true
			return combined, nil
		}
	}
	return combined, nil
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

func (c *ProtocolExtensionContext) RegisterProvider(provider string, override ProtocolProviderOverride) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	if !c.runtime.capabilities[CapabilityProvidersRegister] {
		return ProtocolRuntimeError{Code: "missing_capability", Message: CapabilityProvidersRegister}
	}
	c.runtime.providerOverrides[provider] = override
	c.runtime.ApplyToSession(c.runtime.boundSession)
	return nil
}

func (c *ProtocolExtensionContext) RegisterTool(definition ProtocolToolDefinition) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	if !c.runtime.capabilities[CapabilityToolsRegister] {
		return ProtocolRuntimeError{Code: "missing_capability", Message: CapabilityToolsRegister}
	}
	tool := SDKTool{
		Name:             definition.Name,
		Label:            definition.Label,
		Description:      definition.Description,
		PromptSnippet:    definition.PromptSnippet,
		PromptGuidelines: append([]string(nil), definition.PromptGuidelines...),
		Execute:          definition.Execute,
		SourceInfo:       ProtocolSourceInfo{Path: c.source.Path, Source: "inline", Scope: "temporary", Origin: "top-level"},
	}
	c.runtime.tools = append(c.runtime.tools, tool)
	c.runtime.ApplyToSession(c.runtime.boundSession)
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

func (r *ProtocolExtensionRuntime) ApplyToSession(session *AgentSession) {
	if r == nil || session == nil || session.Agent == nil {
		return
	}
	if override, ok := r.providerOverrides[session.Agent.State.Model.Provider]; ok {
		if override.BaseURL != "" {
			session.Agent.State.Model.BaseURL = override.BaseURL
		}
	}
	session.ExtensionRuntime = r
	session.DynamicTools = append([]SDKTool(nil), r.tools...)
	session.RefreshSystemPrompt()
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
