package gicodingagent

import (
	"fmt"
	"reflect"
	"sort"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

const CapabilityCommandsRegister = "commands.register"
const CapabilityLifecycleEvents = "lifecycle.events"
const CapabilityProvidersRegister = "providers.register"
const CapabilityToolsRegister = "tools.register"
const CapabilityInputEvents = "input.events"

type ProtocolExtensionRuntime struct {
	capabilities      map[string]bool
	commands          []ProtocolCommandRegistration
	handlers          map[string][]ProtocolEventHandler
	inputHandlers     []protocolInputHandlerRegistration
	errorListeners    []ProtocolErrorListener
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
	Role                string
	ToolCallID          string
	Source              string
	Text                string
	Steering            []string
	FollowUp            []string
	Preparation         *agentharness.CompactionPreparation
	BranchEntries       []FileEntry
	CompactionEntry     *FileEntry
	FromExtension       bool
}

type ProtocolEventResult struct {
	Cancel     bool
	Compaction *agentharness.CompactionResult
}

type ProtocolEventHandler func(ProtocolSessionEvent) (ProtocolEventResult, error)

type ProtocolInputHandler func(ProtocolInputEvent) (ProtocolInputResult, error)

type ProtocolErrorListener func(ProtocolExtensionError)

type protocolInputHandlerRegistration struct {
	source  ProtocolSourceInfo
	handler ProtocolInputHandler
}

type ProtocolInputEvent struct {
	Type   string
	Text   string
	Images []llm.ContentPart
	Source string
}

type ProtocolInputResult struct {
	Action    string
	Text      string
	Images    []llm.ContentPart
	ImagesSet bool
}

type ProtocolExtensionError struct {
	ExtensionPath string
	Event         string
	Error         string
}

type ProtocolSendUserMessageOptions struct {
	DeliverAs string
}

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

func (c *ProtocolExtensionContext) OnInput(handler ProtocolInputHandler) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	if !c.runtime.capabilities[CapabilityInputEvents] {
		return ProtocolRuntimeError{Code: "missing_capability", Message: CapabilityInputEvents}
	}
	if handler == nil {
		return nil
	}
	c.runtime.inputHandlers = append(c.runtime.inputHandlers, protocolInputHandlerRegistration{
		source:  c.source,
		handler: handler,
	})
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
		if result.Compaction != nil && combined.Compaction == nil {
			combined.Compaction = result.Compaction
		}
	}
	return combined, nil
}

func (r *ProtocolExtensionRuntime) EmitInput(text string, images []llm.ContentPart, source string) ProtocolInputResult {
	if r == nil || len(r.inputHandlers) == 0 {
		return ProtocolInputResult{Action: "continue"}
	}
	currentText := text
	currentImages := images
	changed := false
	for _, registration := range r.inputHandlers {
		result, err := registration.handler(ProtocolInputEvent{
			Type:   "input",
			Text:   currentText,
			Images: currentImages,
			Source: source,
		})
		if err != nil {
			r.emitExtensionError(ProtocolExtensionError{
				ExtensionPath: registration.source.Path,
				Event:         "input",
				Error:         err.Error(),
			})
			continue
		}
		switch result.Action {
		case "handled":
			return ProtocolInputResult{Action: "handled"}
		case "transform":
			currentText = result.Text
			if result.ImagesSet {
				currentImages = append([]llm.ContentPart(nil), result.Images...)
			}
			changed = true
		}
	}
	if changed {
		return ProtocolInputResult{Action: "transform", Text: currentText, Images: currentImages, ImagesSet: true}
	}
	return ProtocolInputResult{Action: "continue"}
}

func (r *ProtocolExtensionRuntime) HasHandlers(eventType string) bool {
	if r == nil {
		return false
	}
	if eventType == "input" {
		return len(r.inputHandlers) > 0
	}
	return len(r.handlers[eventType]) > 0
}

func (r *ProtocolExtensionRuntime) OnError(listener ProtocolErrorListener) func() {
	if r == nil || listener == nil {
		return func() {}
	}
	r.errorListeners = append(r.errorListeners, listener)
	return func() {
		target := reflect.ValueOf(listener).Pointer()
		for index, candidate := range r.errorListeners {
			if reflect.ValueOf(candidate).Pointer() != target {
				continue
			}
			r.errorListeners = append(r.errorListeners[:index], r.errorListeners[index+1:]...)
			return
		}
	}
}

func (r *ProtocolExtensionRuntime) emitExtensionError(event ProtocolExtensionError) {
	if r == nil {
		return
	}
	listeners := append([]ProtocolErrorListener(nil), r.errorListeners...)
	for _, listener := range listeners {
		listener(event)
	}
}

func ProtocolInputContinue() ProtocolInputResult {
	return ProtocolInputResult{Action: "continue"}
}

func ProtocolInputHandled() ProtocolInputResult {
	return ProtocolInputResult{Action: "handled"}
}

func ProtocolInputTransform(text string) ProtocolInputResult {
	return ProtocolInputResult{Action: "transform", Text: text}
}

func ProtocolInputTransformWithImages(text string, images []llm.ContentPart) ProtocolInputResult {
	return ProtocolInputResult{Action: "transform", Text: text, Images: append([]llm.ContentPart(nil), images...), ImagesSet: true}
}

func (c *ProtocolExtensionContext) SendUserMessage(text string, options ProtocolSendUserMessageOptions) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	return c.runtime.SendUserMessage(text, options)
}

func (r *ProtocolExtensionRuntime) SendUserMessage(text string, options ProtocolSendUserMessageOptions) error {
	if r == nil || r.boundSession == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime has no bound session"}
	}
	if _, err := r.EmitSessionEvent(ProtocolSessionEvent{Type: "input", Source: "extension", Text: text}); err != nil {
		return err
	}
	switch options.DeliverAs {
	case "steer":
		return r.boundSession.Steer(text)
	case "followUp":
		return r.boundSession.FollowUp(text)
	default:
		return r.boundSession.Prompt(text)
	}
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
