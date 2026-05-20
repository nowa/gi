package gicodingagent

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

const CapabilityCommandsRegister = "commands.register"
const CapabilityLifecycleEvents = "lifecycle.events"
const CapabilityProvidersRegister = "providers.register"
const CapabilityToolsRegister = "tools.register"
const CapabilityInputEvents = "input.events"
const CapabilityShortcutsRegister = "shortcuts.register"
const CapabilitySystemPromptModify = "system_prompt.modify"

type ProtocolExtensionRuntime struct {
	capabilities      map[string]bool
	commands          []ProtocolCommandRegistration
	handlers          map[string][]protocolEventHandlerRegistration
	inputHandlers     []protocolInputHandlerRegistration
	errorListeners    []ProtocolErrorListener
	providerOverrides map[string]ProtocolProviderOverride
	tools             []SDKTool
	messageRenderers  map[string]ProtocolMessageRenderer
	flags             []ProtocolFlagRegistration
	flagValues        map[string]any
	shortcuts         []ProtocolShortcutRegistration
	boundSession      *AgentSession
	abortSignal       *ProtocolAbortSignal
	eventSystemPrompt string
	hasEventPrompt    bool
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

type ProtocolMessageRenderer func(message any, options any) []string

type ProtocolFlagDefinition struct {
	Description string
	Type        string
	Default     any
}

type ProtocolFlagRegistration struct {
	Name        string
	Description string
	Type        string
	Default     any
	SourceInfo  ProtocolSourceInfo
}

type ProtocolShortcutDefinition struct {
	Description string
	Handler     func() error
}

type ProtocolShortcutRegistration struct {
	Key         string
	Description string
	SourceInfo  ProtocolSourceInfo
	Handler     func() error
}

type ProtocolShortcutWarning struct {
	Key     string
	Message string
}

type ProtocolShortcutsResult struct {
	Shortcuts map[string]ProtocolShortcutRegistration
	Warnings  []ProtocolShortcutWarning
}

type ProtocolSessionEvent struct {
	Type                string
	Reason              string
	TargetSessionFile   string
	PreviousSessionFile string
	Prompt              string
	Images              []llm.ContentPart
	SystemPrompt        string
	EntryID             string
	Position            string
	Role                string
	ToolName            string
	ToolCallID          string
	Input               map[string]any
	Content             []SDKContentPart
	Details             any
	IsError             bool
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
	Cancel          bool
	Compaction      *agentharness.CompactionResult
	Messages        []llm.Message
	MessagesSet     bool
	SystemPrompt    string
	SystemPromptSet bool
	Content         []SDKContentPart
	ContentSet      bool
	Details         any
	DetailsSet      bool
	IsError         bool
	IsErrorSet      bool
}

type ProtocolEventHandler func(ProtocolSessionEvent) (ProtocolEventResult, error)

type ProtocolInputHandler func(ProtocolInputEvent) (ProtocolInputResult, error)

type ProtocolErrorListener func(ProtocolExtensionError)

type protocolInputHandlerRegistration struct {
	source  ProtocolSourceInfo
	handler ProtocolInputHandler
}

type protocolEventHandlerRegistration struct {
	source  ProtocolSourceInfo
	handler ProtocolEventHandler
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

type ProtocolAbortSignal struct {
	done <-chan struct{}
}

func (e ProtocolRuntimeError) Error() string {
	return e.Code + ": " + e.Message
}

func NewProtocolAbortSignal(done <-chan struct{}) *ProtocolAbortSignal {
	if done == nil {
		return nil
	}
	return &ProtocolAbortSignal{done: done}
}

func (s *ProtocolAbortSignal) Done() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.done
}

func (s *ProtocolAbortSignal) Aborted() bool {
	if s == nil || s.done == nil {
		return false
	}
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func NewProtocolExtensionRuntime(capabilities ...string) *ProtocolExtensionRuntime {
	runtime := &ProtocolExtensionRuntime{
		capabilities:      map[string]bool{},
		handlers:          map[string][]protocolEventHandlerRegistration{},
		providerOverrides: map[string]ProtocolProviderOverride{},
		messageRenderers:  map[string]ProtocolMessageRenderer{},
		flagValues:        map[string]any{},
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

func (r *ProtocolExtensionRuntime) SetAbortSignal(done <-chan struct{}) {
	if r == nil {
		return
	}
	r.abortSignal = NewProtocolAbortSignal(done)
}

func (c *ProtocolExtensionContext) Signal() *ProtocolAbortSignal {
	if c == nil || c.runtime == nil {
		return nil
	}
	return c.runtime.abortSignal
}

func (c *ProtocolExtensionContext) GetSystemPrompt() string {
	if c == nil || c.runtime == nil {
		return ""
	}
	return c.runtime.GetSystemPrompt()
}

func (r *ProtocolExtensionRuntime) GetSystemPrompt() string {
	if r == nil {
		return ""
	}
	if r.hasEventPrompt {
		return r.eventSystemPrompt
	}
	if r.boundSession != nil {
		return r.boundSession.SystemPrompt
	}
	return ""
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
	c.runtime.handlers[eventType] = append(c.runtime.handlers[eventType], protocolEventHandlerRegistration{source: c.source, handler: handler})
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
	currentEvent := event
	if currentEvent.Type == "before_agent_start" {
		if currentEvent.SystemPrompt == "" {
			currentEvent.SystemPrompt = r.GetSystemPrompt()
		}
		previousPrompt := r.eventSystemPrompt
		previousHasPrompt := r.hasEventPrompt
		r.eventSystemPrompt = currentEvent.SystemPrompt
		r.hasEventPrompt = true
		defer func() {
			r.eventSystemPrompt = previousPrompt
			r.hasEventPrompt = previousHasPrompt
		}()
	}
	for _, registration := range r.handlers[currentEvent.Type] {
		if currentEvent.Type == "before_agent_start" {
			r.eventSystemPrompt = currentEvent.SystemPrompt
		}
		result, err := registration.handler(currentEvent)
		if err != nil {
			r.emitExtensionError(ProtocolExtensionError{
				ExtensionPath: registration.source.Path,
				Event:         currentEvent.Type,
				Error:         err.Error(),
			})
			return ProtocolEventResult{}, err
		}
		if result.Cancel {
			combined.Cancel = true
			return combined, nil
		}
		if result.Compaction != nil && combined.Compaction == nil {
			combined.Compaction = result.Compaction
		}
		if err := r.applyEventResult(registration.source, &currentEvent, result, &combined); err != nil {
			return ProtocolEventResult{}, err
		}
	}
	return combined, nil
}

func (r *ProtocolExtensionRuntime) applyEventResult(source ProtocolSourceInfo, event *ProtocolSessionEvent, result ProtocolEventResult, combined *ProtocolEventResult) error {
	switch event.Type {
	case "before_agent_start":
		if result.MessagesSet {
			combined.Messages = append(combined.Messages, result.Messages...)
			combined.MessagesSet = true
		}
		if result.SystemPromptSet {
			if !r.capabilities[CapabilitySystemPromptModify] {
				err := ProtocolRuntimeError{Code: "missing_capability", Message: CapabilitySystemPromptModify}
				r.emitExtensionError(ProtocolExtensionError{
					ExtensionPath: source.Path,
					Event:         event.Type,
					Error:         err.Error(),
				})
				return err
			}
			event.SystemPrompt = result.SystemPrompt
			r.eventSystemPrompt = result.SystemPrompt
			combined.SystemPrompt = result.SystemPrompt
			combined.SystemPromptSet = true
		}
	case "tool_result":
		if result.ContentSet {
			event.Content = cloneSDKContentParts(result.Content)
			combined.Content = cloneSDKContentParts(result.Content)
			combined.ContentSet = true
		}
		if result.DetailsSet {
			event.Details = result.Details
			combined.Details = result.Details
			combined.DetailsSet = true
		}
		if result.IsErrorSet {
			event.IsError = result.IsError
			combined.IsError = result.IsError
			combined.IsErrorSet = true
		}
	}
	return nil
}

func cloneSDKContentParts(parts []SDKContentPart) []SDKContentPart {
	return append([]SDKContentPart(nil), parts...)
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

func (c *ProtocolExtensionContext) RegisterMessageRenderer(customType string, renderer ProtocolMessageRenderer) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	if strings.TrimSpace(customType) == "" || renderer == nil {
		return nil
	}
	if _, exists := c.runtime.messageRenderers[customType]; exists {
		return nil
	}
	c.runtime.messageRenderers[customType] = renderer
	return nil
}

func (c *ProtocolExtensionContext) RegisterFlag(name string, definition ProtocolFlagDefinition) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	name = strings.TrimSpace(strings.TrimPrefix(name, "--"))
	if name == "" {
		return nil
	}
	for _, existing := range c.runtime.flags {
		if existing.Name == name {
			return nil
		}
	}
	registration := ProtocolFlagRegistration{
		Name:        name,
		Description: definition.Description,
		Type:        definition.Type,
		Default:     definition.Default,
		SourceInfo:  c.source,
	}
	c.runtime.flags = append(c.runtime.flags, registration)
	if definition.Default != nil {
		c.runtime.flagValues[name] = definition.Default
	}
	return nil
}

func (c *ProtocolExtensionContext) RegisterShortcut(key string, definition ProtocolShortcutDefinition) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	if !c.runtime.capabilities[CapabilityShortcutsRegister] {
		return ProtocolRuntimeError{Code: "missing_capability", Message: CapabilityShortcutsRegister}
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	c.runtime.shortcuts = append(c.runtime.shortcuts, ProtocolShortcutRegistration{
		Key:         key,
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

func (r *ProtocolExtensionRuntime) RegisteredTools() []SDKTool {
	if r == nil {
		return nil
	}
	seen := map[string]bool{}
	result := make([]SDKTool, 0, len(r.tools))
	for _, tool := range r.tools {
		if tool.Name == "" || seen[tool.Name] {
			continue
		}
		seen[tool.Name] = true
		result = append(result, tool)
	}
	return result
}

func (r *ProtocolExtensionRuntime) GetMessageRenderer(customType string) ProtocolMessageRenderer {
	if r == nil {
		return nil
	}
	return r.messageRenderers[customType]
}

func (r *ProtocolExtensionRuntime) Flags() []ProtocolFlagRegistration {
	if r == nil {
		return nil
	}
	return append([]ProtocolFlagRegistration(nil), r.flags...)
}

func (r *ProtocolExtensionRuntime) SetFlagValue(name string, value any) {
	if r == nil {
		return
	}
	name = strings.TrimSpace(strings.TrimPrefix(name, "--"))
	if name == "" {
		return
	}
	r.flagValues[name] = value
}

func (r *ProtocolExtensionRuntime) FlagValue(name string) any {
	if r == nil {
		return nil
	}
	name = strings.TrimSpace(strings.TrimPrefix(name, "--"))
	return r.flagValues[name]
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
	session.DynamicTools = r.RegisteredTools()
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

func (r *ProtocolExtensionRuntime) Shortcuts(keybindings KeybindingsConfig) ProtocolShortcutsResult {
	result := ProtocolShortcutsResult{Shortcuts: map[string]ProtocolShortcutRegistration{}}
	if r == nil {
		return result
	}
	builtIns := protocolKeybindingActions(keybindings)
	reservedKeys := protocolReservedShortcutKeys(keybindings)
	for _, shortcut := range r.shortcuts {
		if _, reserved := reservedKeys[shortcut.Key]; reserved {
			result.Warnings = append(result.Warnings, ProtocolShortcutWarning{
				Key:     shortcut.Key,
				Message: "shortcut " + shortcut.Key + " conflicts with built-in reserved action",
			})
			continue
		}
		if actions := builtIns[shortcut.Key]; len(actions) > 0 {
			result.Warnings = append(result.Warnings, ProtocolShortcutWarning{
				Key:     shortcut.Key,
				Message: "shortcut " + shortcut.Key + " overrides built-in shortcut for " + strings.Join(actions, ", "),
			})
		}
		if _, exists := result.Shortcuts[shortcut.Key]; exists {
			result.Warnings = append(result.Warnings, ProtocolShortcutWarning{
				Key:     shortcut.Key,
				Message: "shortcut conflict for " + shortcut.Key,
			})
		}
		result.Shortcuts[shortcut.Key] = shortcut
	}
	return result
}

var protocolReservedShortcutActions = map[string]bool{
	"app.interrupt":          true,
	"app.clear":              true,
	"app.exit":               true,
	"app.suspend":            true,
	"app.model.cycleForward": true,
}

func DefaultProtocolKeybindings() KeybindingsConfig {
	return KeybindingsConfig{
		"app.interrupt":            "ctrl+c",
		"app.clear":                "ctrl+l",
		"app.model.cycleForward":   "ctrl+p",
		"app.clipboard.pasteImage": "ctrl+v",
		"app.message.followUp":     "ctrl+p",
	}
}

func protocolReservedShortcutKeys(keybindings KeybindingsConfig) map[string]bool {
	keys := map[string]bool{}
	for action, value := range keybindings {
		if !protocolReservedShortcutActions[action] {
			continue
		}
		for _, key := range keybindingValueKeys(value) {
			keys[key] = true
		}
	}
	return keys
}

func protocolKeybindingActions(keybindings KeybindingsConfig) map[string][]string {
	actions := map[string][]string{}
	for action, value := range keybindings {
		for _, key := range keybindingValueKeys(value) {
			actions[key] = append(actions[key], action)
		}
	}
	for key := range actions {
		sort.Strings(actions[key])
	}
	return actions
}

func keybindingValueKeys(value any) []string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	case []string:
		keys := make([]string, 0, len(typed))
		for _, key := range typed {
			if strings.TrimSpace(key) != "" {
				keys = append(keys, key)
			}
		}
		return keys
	case []any:
		keys := make([]string, 0, len(typed))
		for _, value := range typed {
			if key, ok := value.(string); ok && strings.TrimSpace(key) != "" {
				keys = append(keys, key)
			}
		}
		return keys
	default:
		return nil
	}
}
