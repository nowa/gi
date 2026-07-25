package gicodingagent

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

const CapabilityCommandsRegister = "commands.register"
const CapabilityLifecycleEvents = "lifecycle.events"
const CapabilityProvidersRegister = "providers.register"
const CapabilityToolsRegister = "tools.register"
const CapabilityInputEvents = "input.events"
const CapabilityBashIntercept = "bash.intercept"
const CapabilityShortcutsRegister = "shortcuts.register"
const CapabilitySystemPromptModify = "system_prompt.modify"
const CapabilityResourcesDiscover = "resources.discover"
const CapabilityTUIMessageRenderer = "tui.message_renderer"
const CapabilityTUIToolRenderer = "tui.tool_renderer"
const CapabilityTUIAutocomplete = "tui.autocomplete"
const CapabilityTUIWidget = "tui.widget"
const CapabilityTUIHeader = "tui.header"
const CapabilityTUIFooter = "tui.footer"
const CapabilityTUIOverlay = "tui.overlay"
const CapabilityTUIEditor = "tui.editor"
const CapabilityTUITheme = "tui.theme"
const CapabilityTUIToolsExpanded = "tui.tools_expanded"
const CapabilityTUITerminalInput = "tui.terminal_input"

type ProtocolExtensionRuntime struct {
	// registryMu owns extension-contributed registries and their watcher lists.
	// Registration publishes under the write lock; readers consume snapshots.
	// Extension callbacks are always invoked after releasing the lock.
	registryMu sync.RWMutex

	// providerMu owns provider registrations, derived provider maps, pending
	// registrations, and the current model binding. Calls into ModelRuntime,
	// ModelRegistry, or AgentSession always happen after releasing this lock.
	providerMu sync.RWMutex

	capabilities              map[string]bool
	commands                  []ProtocolCommandRegistration
	handlers                  map[string][]protocolEventHandlerRegistration
	inputHandlers             []protocolInputHandlerRegistration
	errorListeners            []ProtocolErrorListener
	providerOverrides         map[string]ProtocolProviderOverride
	providerSources           map[string]ProtocolSourceInfo
	pendingProviders          []protocolProviderRegistration
	providerRegistrations     []protocolProviderRegistration
	modelRegistry             *ModelRegistry
	modelRuntime              *ModelRuntime
	tools                     []SDKTool
	messageRenderers          map[string]ProtocolMessageRenderer
	messageSources            map[string]ProtocolSourceInfo
	messageRegistrations      []ProtocolMessageRendererRegistration
	entryRenderers            map[string]ProtocolEntryRenderer
	entrySources              map[string]ProtocolSourceInfo
	entryRegistrations        []ProtocolEntryRendererRegistration
	messageRenderWatch        []protocolMessageRendererWatcher
	toolRenderers             map[string]ProtocolToolRendererRegistration
	toolRendererRegistrations []ProtocolToolRendererRegistration
	flags                     []ProtocolFlagRegistration
	flagValues                map[string]any
	cliFlagValues             map[string]any
	flagDiagnostics           []ProtocolExtensionFlagDiagnostic
	shortcuts                 []ProtocolShortcutRegistration
	commandWatch              []protocolCommandWatcher
	autocomplete              []ProtocolAutocompleteProviderRegistration
	autocompleteWatch         []protocolAutocompleteWatcher
	viewTreeMounts            []ProtocolViewTreeMountRegistration
	nextCommandWatch          int
	nextMessageRender         int
	nextAutocomplete          int
	boundSession              *AgentSession
	commandContext            ProtocolCommandContextActions
	contextGeneration         int
	abortSignal               *ProtocolAbortSignal
	eventSystemPrompt         string
	hasEventPrompt            bool
	viewTreeHost              *ViewTreeHost
	hostActionHost            *RPCSessionHost
	hostActionSession         *AgentSession
}

type protocolAutocompleteWatcher struct {
	id       int
	callback func()
}

type protocolCommandWatcher struct {
	id       int
	callback func()
}

type protocolMessageRendererWatcher struct {
	id       int
	callback func()
}

type ProtocolExtensionFactory struct {
	Path    string
	Name    string
	Hidden  bool
	Factory func(*ProtocolExtensionContext) error
}

type ProtocolExtensionContext struct {
	runtime *ProtocolExtensionRuntime
	source  ProtocolSourceInfo
}

type ProtocolSourceInfo struct {
	Path   string `json:"path,omitempty"`
	Source string `json:"source,omitempty"`
	Scope  string `json:"scope,omitempty"`
	Origin string `json:"origin,omitempty"`
}

type ProtocolCommandDefinition struct {
	Description        string
	ArgumentHint       string
	Handler            func(args string) error
	HandlerWithContext func(args string, ctx ProtocolCommandContext) error
}

type ProtocolCommandRegistration struct {
	Name               string
	InvocationName     string
	Description        string
	ArgumentHint       string
	SourceInfo         ProtocolSourceInfo
	Handler            func(args string) error
	HandlerWithContext func(args string, ctx ProtocolCommandContext) error
}

type ProtocolProviderOverride struct {
	BaseURL        string
	APIKey         string
	API            string
	Headers        map[string]string
	AuthHeader     *bool
	Compat         llm.ModelCompat
	Models         []ProviderModelDefinition
	ModelOverrides map[string]ModelOverride
	StreamSimple   func(llm.Model, llm.Context, llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error)
}

type ProtocolToolDefinition struct {
	Name               string
	Label              string
	Description        string
	Parameters         llm.Schema
	PromptSnippet      string
	PromptGuidelines   []string
	ExecutionMode      string
	PrepareArguments   func(input map[string]any) map[string]any
	Execute            func(toolCallID string, input map[string]any) (SDKToolResult, error)
	ExecuteWithUpdates func(toolCallID string, input map[string]any, onUpdate func(SDKToolResult)) (SDKToolResult, error)
}

type ProtocolMessageRenderer func(message any, options any) []string

type ProtocolMessageRendererRegistration struct {
	CustomType string
	SourceInfo ProtocolSourceInfo
	Renderer   ProtocolMessageRenderer
}

type ProtocolEntryRenderOptions struct {
	Expanded bool
}

// ProtocolEntryRenderer is a trusted in-process rendering boundary. Portable
// process extensions use ViewTree nodes instead of passing component handles
// across RPC.
type ProtocolEntryRenderer func(
	entry FileEntry,
	options ProtocolEntryRenderOptions,
) gitui.Component

type ProtocolEntryRendererRegistration struct {
	CustomType string
	SourceInfo ProtocolSourceInfo
	Renderer   ProtocolEntryRenderer
}

type ProtocolToolRendererDefinition struct {
	RenderCall   ToolCallRenderer
	RenderResult ToolResultRenderer
}

type ProtocolToolRendererRegistration struct {
	Name         string
	SourceInfo   ProtocolSourceInfo
	RenderCall   ToolCallRenderer
	RenderResult ToolResultRenderer
}

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

type ProtocolExtensionFlagDiagnostic struct {
	Name    string
	Message string
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

type ProtocolAutocompleteRequest struct {
	Text          string
	Lines         []string
	CursorLine    int
	CursorCol     int
	Force         bool
	Trigger       string
	SlashCommand  string
	ArgumentIndex int
}

type ProtocolAutocompleteRange struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

type ProtocolAutocompleteItem struct {
	ID          string
	Value       string
	Label       string
	Description string
	Kind        string
	Range       *ProtocolAutocompleteRange
	Detail      *ViewTreeNode
}

type ProtocolAutocompleteResult struct {
	Items  []ProtocolAutocompleteItem
	Prefix string
	Start  int
	End    int
}

type ProtocolAutocompleteProviderDefinition struct {
	Description string
	Priority    int
	Handler     func(context.Context, ProtocolAutocompleteRequest) (ProtocolAutocompleteResult, error)
}

type ProtocolAutocompleteProviderRegistration struct {
	ID          string
	Description string
	Priority    int
	SourceInfo  ProtocolSourceInfo
	Handler     func(context.Context, ProtocolAutocompleteRequest) (ProtocolAutocompleteResult, error)
}

type ProtocolViewTreeMountRegistration struct {
	MountID    string
	Slot       string
	View       ViewTreeNode
	Priority   int
	Overlay    *ViewTreeOverlayOptions
	SourceInfo ProtocolSourceInfo
}

type ProtocolProviderRegistration struct {
	Name       string
	Config     ProtocolProviderOverride
	SourceInfo ProtocolSourceInfo
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
	Context               context.Context
	Type                  string
	Reason                string
	WillRetry             bool
	TargetSessionFile     string
	PreviousSessionFile   string
	Prompt                string
	Images                []llm.ContentPart
	SystemPrompt          string
	Messages              []llm.Message
	Message               *llm.Message
	AssistantMessageEvent *llm.AssistantMessageEvent
	EntryID               string
	Position              string
	Role                  string
	Model                 *llm.Model
	PreviousModel         *llm.Model
	SelectSource          string
	ThinkingLevel         string
	PreviousLevel         string
	ToolName              string
	ToolCallID            string
	Input                 map[string]any
	Content               []SDKContentPart
	Details               any
	IsError               bool
	Source                string
	Text                  string
	Command               string
	CWD                   string
	ExcludeFromContext    bool
	Steering              []string
	FollowUp              []string
	Preparation           *agentharness.CompactionPreparation
	BranchEntries         []FileEntry
	CustomInstructions    string
	CompactionEntry       *FileEntry
	FromExtension         bool
	Name                  string
	Payload               any
	Status                int
	Headers               map[string]string
	ProjectTrustContext   *ProtocolProjectTrustContext
}

type ProtocolEventResult struct {
	Cancel            bool
	Block             bool
	Reason            string
	Compaction        *agentharness.CompactionResult
	Messages          []llm.Message
	MessagesSet       bool
	Message           *llm.Message
	MessageSet        bool
	CustomMessages    []ProtocolCustomMessage
	CustomMessagesSet bool
	SystemPrompt      string
	SystemPromptSet   bool
	Content           []SDKContentPart
	ContentSet        bool
	Details           any
	DetailsSet        bool
	IsError           bool
	IsErrorSet        bool
	BashResult        *BashResult
	BashResultSet     bool
	Payload           any
	PayloadSet        bool
	Resources         ResourceExtension
	ResourcesSet      bool
	ProjectTrust      *ProtocolProjectTrustResult
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

type protocolProviderRegistration struct {
	source ProtocolSourceInfo
	name   string
	config ProtocolProviderOverride
}

type ProtocolForkOptions struct {
	Position    string
	WithSession func(ProtocolCommandContext) error
}

type ProtocolCommandForkResult struct {
	Cancelled bool
}

type ProtocolNewSessionOptions struct {
	ParentSession string
	WithSession   func(ProtocolCommandContext) error
}

type ProtocolSwitchSessionOptions struct {
	WithSession func(ProtocolCommandContext) error
}

type ProtocolCommandSwitchResult struct {
	Cancelled bool
}

type ProtocolCommandContextActions struct {
	WaitForIdle   func() error
	Fork          func(entryID string, options ProtocolForkOptions) (ProtocolCommandForkResult, error)
	NewSession    func(options ProtocolNewSessionOptions) (ProtocolCommandSwitchResult, error)
	SwitchSession func(sessionFile string, options ProtocolSwitchSessionOptions) (ProtocolCommandSwitchResult, error)
	Reload        func() error
}

type ProtocolCommandContext struct {
	runtime    *ProtocolExtensionRuntime
	generation int
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
	Stack         string
}

type ProtocolSendUserMessageOptions struct {
	DeliverAs string
}

type ProtocolCustomMessage struct {
	CustomType string
	Content    any
	Display    bool
	Details    any
}

type ProtocolSendCustomMessageOptions struct {
	TriggerTurn bool
	DeliverAs   string
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
		providerSources:   map[string]ProtocolSourceInfo{},
		messageRenderers:  map[string]ProtocolMessageRenderer{},
		messageSources:    map[string]ProtocolSourceInfo{},
		entryRenderers:    map[string]ProtocolEntryRenderer{},
		entrySources:      map[string]ProtocolSourceInfo{},
		toolRenderers:     map[string]ProtocolToolRendererRegistration{},
		flagValues:        map[string]any{},
		cliFlagValues:     map[string]any{},
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
	if session != nil {
		session.ExtensionRuntime = r
	}
	r.boundSession = session
	if r.hostActionSession != session {
		r.hostActionHost = nil
		r.hostActionSession = nil
	}
	r.ApplyToSession(session)
}

func (r *ProtocolExtensionRuntime) BindModelRegistry(registry *ModelRegistry) {
	if r == nil {
		return
	}
	r.providerMu.Lock()
	r.modelRuntime = nil
	r.modelRegistry = registry
	r.providerMu.Unlock()
	r.bindPendingProviderRegistrations()
}

// BindModelRuntime routes provider mutations through the instance runtime
// while retaining the compatibility registry for legacy extension surfaces.
func (r *ProtocolExtensionRuntime) BindModelRuntime(
	runtime *ModelRuntime,
) {
	if r == nil {
		return
	}
	r.providerMu.Lock()
	r.modelRuntime = runtime
	r.modelRegistry = nil
	if runtime != nil {
		r.modelRegistry = runtime.ModelRegistry()
	}
	r.providerMu.Unlock()
	r.bindPendingProviderRegistrations()
}

func (r *ProtocolExtensionRuntime) GetModelRegistry() *ModelRegistry {
	if r == nil {
		return nil
	}
	r.providerMu.RLock()
	registry := r.modelRegistry
	r.providerMu.RUnlock()
	return registry
}

func (r *ProtocolExtensionRuntime) bindPendingProviderRegistrations() {
	r.providerMu.Lock()
	modelRuntime := r.modelRuntime
	modelRegistry := r.modelRegistry
	if modelRuntime == nil && modelRegistry == nil {
		r.providerMu.Unlock()
		return
	}
	pending := append([]protocolProviderRegistration(nil), r.pendingProviders...)
	r.pendingProviders = nil
	r.providerMu.Unlock()
	for _, registration := range pending {
		if err := applyProviderRegistrationTo(
			registration,
			modelRuntime,
			modelRegistry,
		); err != nil {
			r.emitExtensionError(ProtocolExtensionError{
				ExtensionPath: registration.source.Path,
				Event:         "register_provider",
				Error:         err.Error(),
			})
		}
	}
}

func (r *ProtocolExtensionRuntime) BindViewTreeHost(host *ViewTreeHost) {
	if r == nil {
		return
	}
	r.viewTreeHost = host
	if r.hostActionHost != nil && host != nil {
		r.hostActionHost.ViewTreeHost = host
	}
	r.applyViewTreeMounts()
}

func (r *ProtocolExtensionRuntime) BindHostActionHost(host *RPCSessionHost) {
	if r == nil {
		return
	}
	r.hostActionHost = host
	r.hostActionSession = nil
	if host != nil {
		r.hostActionSession = host.Session
		if r.viewTreeHost != nil && host.ViewTreeHost == nil {
			host.ViewTreeHost = r.viewTreeHost
		}
	}
}

func (r *ProtocolExtensionRuntime) BindCommandContext(actions ProtocolCommandContextActions) {
	if r == nil {
		return
	}
	r.commandContext = actions
}

func (r *ProtocolExtensionRuntime) CreateCommandContext() ProtocolCommandContext {
	generation := 0
	if r != nil {
		generation = r.contextGeneration
	}
	return ProtocolCommandContext{runtime: r, generation: generation}
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

func (c *ProtocolExtensionContext) GetFlag(name string) any {
	if c == nil || c.runtime == nil {
		return nil
	}
	name = normalizeProtocolFlagName(name)
	if name == "" {
		return nil
	}
	c.runtime.registryMu.RLock()
	defer c.runtime.registryMu.RUnlock()
	for _, flag := range c.runtime.flags {
		if flag.Name == name && protocolSourceInfoEqual(flag.SourceInfo, c.source) {
			return c.runtime.flagValues[name]
		}
	}
	return nil
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

func (c *ProtocolExtensionContext) IsIdle() bool {
	return c == nil || c.runtime == nil || c.runtime.boundSession == nil || c.runtime.boundSession.IsIdle()
}

func (c ProtocolCommandContext) WaitForIdle() error {
	if err := c.ensureCurrent(); err != nil {
		return err
	}
	if c.runtime.commandContext.WaitForIdle == nil {
		return nil
	}
	return c.runtime.commandContext.WaitForIdle()
}

func (c ProtocolCommandContext) Fork(entryID string, options ...ProtocolForkOptions) (ProtocolCommandForkResult, error) {
	if err := c.ensureCurrent(); err != nil {
		return ProtocolCommandForkResult{}, err
	}
	if c.runtime.commandContext.Fork == nil {
		return ProtocolCommandForkResult{Cancelled: false}, nil
	}
	option := ProtocolForkOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	return c.runtime.commandContext.Fork(entryID, option)
}

func (c ProtocolCommandContext) NewSession(options ...ProtocolNewSessionOptions) (ProtocolCommandSwitchResult, error) {
	if err := c.ensureCurrent(); err != nil {
		return ProtocolCommandSwitchResult{}, err
	}
	if c.runtime.commandContext.NewSession == nil {
		return ProtocolCommandSwitchResult{Cancelled: false}, nil
	}
	option := ProtocolNewSessionOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	return c.runtime.commandContext.NewSession(option)
}

func (c ProtocolCommandContext) SwitchSession(sessionFile string, options ...ProtocolSwitchSessionOptions) (ProtocolCommandSwitchResult, error) {
	if err := c.ensureCurrent(); err != nil {
		return ProtocolCommandSwitchResult{}, err
	}
	if c.runtime.commandContext.SwitchSession == nil {
		return ProtocolCommandSwitchResult{Cancelled: false}, nil
	}
	option := ProtocolSwitchSessionOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	return c.runtime.commandContext.SwitchSession(sessionFile, option)
}

func (c ProtocolCommandContext) Reload() error {
	if err := c.ensureCurrent(); err != nil {
		return err
	}
	if c.runtime.commandContext.Reload == nil {
		return nil
	}
	return c.runtime.commandContext.Reload()
}

func (c ProtocolCommandContext) SendUserMessage(text string, options ...ProtocolSendUserMessageOptions) error {
	if err := c.ensureCurrent(); err != nil {
		return err
	}
	option := ProtocolSendUserMessageOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	return c.runtime.SendUserMessage(text, option)
}

func (c ProtocolCommandContext) ensureCurrent() error {
	if c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension command context is unavailable"}
	}
	if c.generation != c.runtime.contextGeneration {
		return ProtocolRuntimeError{Code: "stale_context", Message: "extension command context has been replaced"}
	}
	return nil
}

func (r *ProtocolExtensionRuntime) InvalidateCommandContexts() {
	if r == nil {
		return
	}
	r.contextGeneration++
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
	c.runtime.registryMu.Lock()
	c.runtime.handlers[eventType] = append(c.runtime.handlers[eventType], protocolEventHandlerRegistration{source: c.source, handler: handler})
	c.runtime.registryMu.Unlock()
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
	c.runtime.registryMu.Lock()
	c.runtime.inputHandlers = append(c.runtime.inputHandlers, protocolInputHandlerRegistration{
		source:  c.source,
		handler: handler,
	})
	c.runtime.registryMu.Unlock()
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
	r.registryMu.RLock()
	handlers := append([]protocolEventHandlerRegistration(nil), r.handlers[currentEvent.Type]...)
	r.registryMu.RUnlock()
	for _, registration := range handlers {
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
		if currentEvent.Type == "tool_call" && combined.Block {
			return combined, nil
		}
		if currentEvent.Type == ProtocolEventUserBash && combined.BashResultSet {
			return combined, nil
		}
	}
	return combined, nil
}

// EmitBeforeProviderHeaders runs after provider, model, and request headers
// have been assembled. Every handler receives the same mutable map, so changes
// flow to the next handler and then to the HTTP transport. A failing extension
// is diagnosed and isolated instead of preventing later handlers from running.
func (r *ProtocolExtensionRuntime) EmitBeforeProviderHeaders(
	ctx context.Context,
	headers map[string]string,
	model *llm.Model,
) map[string]string {
	current := cloneStringMap(headers)
	if current == nil {
		current = map[string]string{}
	}
	if r == nil {
		return current
	}
	r.registryMu.RLock()
	handlers := append(
		[]protocolEventHandlerRegistration(nil),
		r.handlers[ProtocolEventBeforeProviderHeaders]...,
	)
	r.registryMu.RUnlock()
	event := ProtocolSessionEvent{
		Context: ctx,
		Type:    ProtocolEventBeforeProviderHeaders,
		Model:   model,
		Headers: current,
	}
	for _, registration := range handlers {
		if _, err := invokeProtocolEventHandler(registration.handler, event); err != nil {
			r.emitExtensionError(ProtocolExtensionError{
				ExtensionPath: registration.source.Path,
				Event:         ProtocolEventBeforeProviderHeaders,
				Error:         err.Error(),
			})
		}
	}
	return current
}

func invokeProtocolEventHandler(
	handler ProtocolEventHandler,
	event ProtocolSessionEvent,
) (result ProtocolEventResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("extension handler panicked: %v", recovered)
		}
	}()
	return handler(event)
}

func (r *ProtocolExtensionRuntime) applyEventResult(source ProtocolSourceInfo, event *ProtocolSessionEvent, result ProtocolEventResult, combined *ProtocolEventResult) error {
	switch event.Type {
	case "before_agent_start":
		if result.MessagesSet {
			combined.Messages = append(combined.Messages, result.Messages...)
			combined.MessagesSet = true
		}
		if result.CustomMessagesSet {
			combined.CustomMessages = append(combined.CustomMessages, result.CustomMessages...)
			combined.CustomMessagesSet = true
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
	case ProtocolEventMessageEnd:
		if result.MessageSet && result.Message != nil {
			message := *result.Message
			event.Message = &message
			combined.Message = &message
			combined.MessageSet = true
		}
	case "context":
		if result.MessagesSet {
			event.Messages = append([]llm.Message(nil), result.Messages...)
			combined.Messages = append([]llm.Message(nil), result.Messages...)
			combined.MessagesSet = true
		}
	case ProtocolEventBeforeProviderRequest:
		if result.PayloadSet {
			event.Payload = result.Payload
			combined.Payload = result.Payload
			combined.PayloadSet = true
		}
	case "tool_call":
		if result.Block {
			combined.Block = true
			combined.Reason = result.Reason
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
	case ProtocolEventUserBash:
		if result.BashResultSet || result.BashResult != nil {
			var bashResult BashResult
			if result.BashResult != nil {
				bashResult = *result.BashResult
			}
			combined.BashResult = &bashResult
			combined.BashResultSet = true
		}
	case ProtocolEventResourcesDiscover:
		if result.ResourcesSet || len(result.Resources.ExtensionPaths) > 0 || len(result.Resources.SkillPaths) > 0 ||
			len(result.Resources.PromptPaths) > 0 || len(result.Resources.ThemePaths) > 0 {
			resources := resourceExtensionWithSourceDefaults(result.Resources, source)
			combined.Resources.ExtensionPaths = append(combined.Resources.ExtensionPaths, resources.ExtensionPaths...)
			combined.Resources.SkillPaths = append(combined.Resources.SkillPaths, resources.SkillPaths...)
			combined.Resources.PromptPaths = append(combined.Resources.PromptPaths, resources.PromptPaths...)
			combined.Resources.ThemePaths = append(combined.Resources.ThemePaths, resources.ThemePaths...)
			combined.ResourcesSet = true
		}
	}
	return nil
}

func resourceExtensionWithSourceDefaults(resources ResourceExtension, source ProtocolSourceInfo) ResourceExtension {
	for index := range resources.ExtensionPaths {
		resources.ExtensionPaths[index].Metadata = resourceMetadataWithSourceDefaults(resources.ExtensionPaths[index].Path, resources.ExtensionPaths[index].Metadata, source)
	}
	for index := range resources.SkillPaths {
		resources.SkillPaths[index].Metadata = resourceMetadataWithSourceDefaults(resources.SkillPaths[index].Path, resources.SkillPaths[index].Metadata, source)
	}
	for index := range resources.PromptPaths {
		resources.PromptPaths[index].Metadata = resourceMetadataWithSourceDefaults(resources.PromptPaths[index].Path, resources.PromptPaths[index].Metadata, source)
	}
	for index := range resources.ThemePaths {
		resources.ThemePaths[index].Metadata = resourceMetadataWithSourceDefaults(resources.ThemePaths[index].Path, resources.ThemePaths[index].Metadata, source)
	}
	return resources
}

func resourceMetadataWithSourceDefaults(path string, metadata, source ProtocolSourceInfo) ProtocolSourceInfo {
	defaultInfo := protocolDefaultSourceInfo(source)
	if metadata.Path == "" {
		metadata.Path = path
	}
	if metadata.Source == "" {
		metadata.Source = defaultInfo.Source
	}
	if metadata.Scope == "" {
		metadata.Scope = defaultInfo.Scope
	}
	if metadata.Origin == "" {
		metadata.Origin = defaultInfo.Origin
	}
	return metadata
}

func protocolDefaultSourceInfo(source ProtocolSourceInfo) ProtocolSourceInfo {
	info := source
	if info.Source == "" {
		info.Source = "inline"
	}
	if info.Scope == "" {
		info.Scope = "temporary"
	}
	if info.Origin == "" {
		info.Origin = "top-level"
	}
	return info
}

func cloneSDKContentParts(parts []SDKContentPart) []SDKContentPart {
	return append([]SDKContentPart(nil), parts...)
}

func (r *ProtocolExtensionRuntime) EmitInput(text string, images []llm.ContentPart, source string) ProtocolInputResult {
	if r == nil {
		return ProtocolInputResult{Action: "continue"}
	}
	r.registryMu.RLock()
	handlers := append([]protocolInputHandlerRegistration(nil), r.inputHandlers...)
	r.registryMu.RUnlock()
	if len(handlers) == 0 {
		return ProtocolInputResult{Action: "continue"}
	}
	currentText := text
	currentImages := images
	changed := false
	for _, registration := range handlers {
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
	r.registryMu.RLock()
	defer r.registryMu.RUnlock()
	if eventType == "input" {
		return len(r.inputHandlers) > 0
	}
	return len(r.handlers[eventType]) > 0
}

func (r *ProtocolExtensionRuntime) OnError(listener ProtocolErrorListener) func() {
	if r == nil || listener == nil {
		return func() {}
	}
	r.registryMu.Lock()
	r.errorListeners = append(r.errorListeners, listener)
	r.registryMu.Unlock()
	return func() {
		target := reflect.ValueOf(listener).Pointer()
		r.registryMu.Lock()
		defer r.registryMu.Unlock()
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
	r.registryMu.RLock()
	listeners := append([]ProtocolErrorListener(nil), r.errorListeners...)
	r.registryMu.RUnlock()
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

func (c *ProtocolExtensionContext) SendCustomMessage(message ProtocolCustomMessage, options ProtocolSendCustomMessageOptions) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	return c.runtime.SendCustomMessage(message, options)
}

func (c *ProtocolExtensionContext) SetSessionName(name string) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	return c.runtime.SetSessionName(name)
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
		return r.boundSession.QueueExtensionUserMessage(text, "steer")
	case "followUp":
		return r.boundSession.QueueExtensionUserMessage(text, "followUp")
	default:
		return r.boundSession.Prompt(text)
	}
}

func (r *ProtocolExtensionRuntime) SendCustomMessage(message ProtocolCustomMessage, options ProtocolSendCustomMessageOptions) error {
	if r == nil || r.boundSession == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime has no bound session"}
	}
	return r.boundSession.SendCustomMessage(QueuedCustomMessage{
		CustomType: message.CustomType,
		Content:    message.Content,
		Display:    message.Display,
		Details:    message.Details,
	}, options)
}

func (r *ProtocolExtensionRuntime) SetSessionName(name string) error {
	if r == nil || r.boundSession == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime has no bound session"}
	}
	return r.boundSession.SetSessionName(name)
}

func (r *ProtocolExtensionRuntime) AppendCustomEntry(customType string, data any) (string, error) {
	if r == nil || r.boundSession == nil || r.boundSession.SessionManager == nil {
		return "", ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime has no bound session"}
	}
	return r.boundSession.AppendCustomEntry(customType, data)
}

func (r *ProtocolExtensionRuntime) SessionEntries() []FileEntry {
	if r == nil || r.boundSession == nil || r.boundSession.SessionManager == nil {
		return nil
	}
	return r.boundSession.SessionManager.GetEntries()
}

func (r *ProtocolExtensionRuntime) ActiveToolNames() []string {
	if r == nil || r.boundSession == nil {
		return nil
	}
	return r.boundSession.GetActiveToolNames()
}

func (r *ProtocolExtensionRuntime) GetActiveTools() []string {
	return r.ActiveToolNames()
}

func (r *ProtocolExtensionRuntime) LoadFactories(factories []ProtocolExtensionFactory) error {
	for index, input := range factories {
		factory := normalizeProtocolExtensionFactory(input, index)
		if factory.Factory == nil {
			continue
		}
		context := &ProtocolExtensionContext{
			runtime: r,
			source:  protocolInlineFactorySourceInfo(factory),
		}
		if err := factory.Factory(context); err != nil {
			return err
		}
	}
	return nil
}

func (r *ProtocolExtensionRuntime) RemoveSource(source ProtocolSourceInfo) {
	if r == nil || protocolSourceInfoKey(source) == "" {
		return
	}
	r.registryMu.Lock()
	if len(r.handlers) > 0 {
		for eventType, registrations := range r.handlers {
			filtered := registrations[:0]
			for _, registration := range registrations {
				if protocolSourceInfoEqual(registration.source, source) {
					continue
				}
				filtered = append(filtered, registration)
			}
			if len(filtered) == 0 {
				delete(r.handlers, eventType)
			} else {
				r.handlers[eventType] = filtered
			}
		}
	}
	if len(r.inputHandlers) > 0 {
		filtered := r.inputHandlers[:0]
		for _, registration := range r.inputHandlers {
			if protocolSourceInfoEqual(registration.source, source) {
				continue
			}
			filtered = append(filtered, registration)
		}
		r.inputHandlers = filtered
	}
	commandsChanged := false
	if len(r.commands) > 0 {
		filtered := r.commands[:0]
		for _, registration := range r.commands {
			if protocolSourceInfoEqual(registration.SourceInfo, source) {
				commandsChanged = true
				continue
			}
			filtered = append(filtered, registration)
		}
		r.commands = filtered
	}

	sessionChanged := false
	if len(r.tools) > 0 {
		filtered := r.tools[:0]
		for _, tool := range r.tools {
			if protocolSourceInfoEqual(tool.SourceInfo, source) {
				sessionChanged = true
				continue
			}
			filtered = append(filtered, tool)
		}
		r.tools = filtered
	}
	r.registryMu.Unlock()
	affectedProviders := map[string]bool{}
	r.providerMu.Lock()
	if len(r.providerRegistrations) > 0 {
		filtered := r.providerRegistrations[:0]
		for _, registration := range r.providerRegistrations {
			if protocolSourceInfoEqual(registration.source, source) {
				affectedProviders[registration.name] = true
				sessionChanged = true
				continue
			}
			filtered = append(filtered, registration)
		}
		r.providerRegistrations = filtered
		if len(affectedProviders) > 0 {
			filteredPending := r.pendingProviders[:0]
			for _, registration := range r.pendingProviders {
				if protocolSourceInfoEqual(registration.source, source) {
					continue
				}
				filteredPending = append(filteredPending, registration)
			}
			r.pendingProviders = filteredPending
			r.rebuildProviderMapsLocked()
		}
	}
	r.providerMu.Unlock()
	if len(affectedProviders) > 0 {
		r.rebuildProviderState(affectedProviders)
	}

	r.registryMu.Lock()
	messageRenderersChanged := false
	if len(r.messageRegistrations) > 0 {
		filtered := r.messageRegistrations[:0]
		for _, registration := range r.messageRegistrations {
			if protocolSourceInfoEqual(registration.SourceInfo, source) {
				messageRenderersChanged = true
				continue
			}
			filtered = append(filtered, registration)
		}
		r.messageRegistrations = filtered
		if messageRenderersChanged {
			r.rebuildMessageRenderersLocked()
		}
	}
	if len(r.entryRegistrations) > 0 {
		filtered := r.entryRegistrations[:0]
		entryRenderersChanged := false
		for _, registration := range r.entryRegistrations {
			if protocolSourceInfoEqual(registration.SourceInfo, source) {
				entryRenderersChanged = true
				continue
			}
			filtered = append(filtered, registration)
		}
		r.entryRegistrations = filtered
		if entryRenderersChanged {
			messageRenderersChanged = true
			r.rebuildEntryRenderersLocked()
		}
	}

	if len(r.toolRendererRegistrations) > 0 {
		filtered := r.toolRendererRegistrations[:0]
		toolRenderersChanged := false
		for _, renderer := range r.toolRendererRegistrations {
			if protocolSourceInfoEqual(renderer.SourceInfo, source) {
				toolRenderersChanged = true
				continue
			}
			filtered = append(filtered, renderer)
		}
		r.toolRendererRegistrations = filtered
		if toolRenderersChanged {
			r.rebuildToolRenderersLocked()
		}
	}

	if len(r.flags) > 0 {
		visibleBefore := map[string]ProtocolSourceInfo{}
		for _, flag := range r.visibleFlagRegistrationsLocked() {
			visibleBefore[flag.Name] = flag.SourceInfo
		}
		removedVisibleNames := map[string]bool{}
		filtered := r.flags[:0]
		for _, flag := range r.flags {
			if protocolSourceInfoEqual(flag.SourceInfo, source) {
				if protocolSourceInfoEqual(visibleBefore[flag.Name], flag.SourceInfo) {
					removedVisibleNames[flag.Name] = true
				}
				sessionChanged = true
				continue
			}
			filtered = append(filtered, flag)
		}
		r.flags = filtered
		for name := range removedVisibleNames {
			delete(r.flagValues, name)
			r.applyVisibleFlagValueLocked(name)
		}
	}

	if len(r.shortcuts) > 0 {
		filtered := r.shortcuts[:0]
		for _, shortcut := range r.shortcuts {
			if protocolSourceInfoEqual(shortcut.SourceInfo, source) {
				continue
			}
			filtered = append(filtered, shortcut)
		}
		r.shortcuts = filtered
	}

	autocompleteChanged := false
	if len(r.autocomplete) > 0 {
		filtered := r.autocomplete[:0]
		for _, provider := range r.autocomplete {
			if protocolSourceInfoEqual(provider.SourceInfo, source) {
				autocompleteChanged = true
				continue
			}
			filtered = append(filtered, provider)
		}
		r.autocomplete = filtered
	}

	var removedMountIDs []string
	if len(r.viewTreeMounts) > 0 {
		filtered := r.viewTreeMounts[:0]
		for _, mount := range r.viewTreeMounts {
			if protocolSourceInfoEqual(mount.SourceInfo, source) {
				removedMountIDs = append(removedMountIDs, mount.MountID)
				continue
			}
			filtered = append(filtered, mount)
		}
		r.viewTreeMounts = filtered
	}
	r.registryMu.Unlock()

	if r.viewTreeHost != nil {
		for _, mountID := range removedMountIDs {
			r.viewTreeHost.Unmount(mountID)
		}
	}

	if commandsChanged {
		r.notifyCommandsChanged()
	}
	if autocompleteChanged {
		r.notifyAutocompleteProvidersChanged()
	}
	if messageRenderersChanged {
		r.notifyMessageRenderersChanged()
	}
	if sessionChanged {
		r.ApplyToSession(r.boundSession)
	}
}

func cloneProtocolSourceInfoMap(values map[string]ProtocolSourceInfo) map[string]ProtocolSourceInfo {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]ProtocolSourceInfo, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func protocolSourceInfoEqual(left, right ProtocolSourceInfo) bool {
	if left.Path != "" && right.Path != "" {
		return left.Path == right.Path
	}
	return protocolSourceInfoKey(left) != "" && protocolSourceInfoKey(left) == protocolSourceInfoKey(right)
}

func protocolSourceInfoKey(source ProtocolSourceInfo) string {
	if source.Path == "" && source.Source == "" && source.Scope == "" && source.Origin == "" {
		return ""
	}
	return source.Path + "\x00" + source.Source + "\x00" + source.Scope + "\x00" + source.Origin
}

func (c *ProtocolExtensionContext) RegisterCommand(name string, definition ProtocolCommandDefinition) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	if !c.runtime.capabilities[CapabilityCommandsRegister] {
		return ProtocolRuntimeError{Code: "missing_capability", Message: CapabilityCommandsRegister}
	}
	registration := ProtocolCommandRegistration{
		Name:               name,
		Description:        definition.Description,
		ArgumentHint:       definition.ArgumentHint,
		SourceInfo:         c.source,
		Handler:            definition.Handler,
		HandlerWithContext: definition.HandlerWithContext,
	}
	c.runtime.registryMu.Lock()
	for index, existing := range c.runtime.commands {
		if existing.Name == name && protocolSourceInfoEqual(existing.SourceInfo, c.source) {
			c.runtime.commands[index] = registration
			c.runtime.registryMu.Unlock()
			c.runtime.notifyCommandsChanged()
			return nil
		}
	}
	c.runtime.commands = append(c.runtime.commands, registration)
	c.runtime.registryMu.Unlock()
	c.runtime.notifyCommandsChanged()
	return nil
}

func (c *ProtocolExtensionContext) RegisterProvider(provider string, override ProtocolProviderOverride) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	if !c.runtime.capabilities[CapabilityProvidersRegister] {
		return ProtocolRuntimeError{Code: "missing_capability", Message: CapabilityProvidersRegister}
	}
	return c.runtime.registerProvider(c.source, provider, override)
}

func (c *ProtocolExtensionContext) UnregisterProvider(provider string) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	if !c.runtime.capabilities[CapabilityProvidersRegister] {
		return ProtocolRuntimeError{Code: "missing_capability", Message: CapabilityProvidersRegister}
	}
	c.runtime.unregisterProvider(provider)
	return nil
}

func (r *ProtocolExtensionRuntime) registerProvider(source ProtocolSourceInfo, provider string, override ProtocolProviderOverride) error {
	if r == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil
	}
	registration := protocolProviderRegistration{
		source: source,
		name:   provider,
		config: cloneProtocolProviderOverride(override),
	}
	r.providerMu.Lock()
	r.providerRegistrations = append(r.providerRegistrations, registration)
	r.rebuildProviderMapsLocked()
	modelRuntime := r.modelRuntime
	modelRegistry := r.modelRegistry
	if modelRuntime == nil && modelRegistry == nil {
		r.pendingProviders = append(r.pendingProviders, registration)
		r.providerMu.Unlock()
		r.ApplyToSession(r.boundSession)
		return nil
	}
	r.providerMu.Unlock()
	if err := applyProviderRegistrationTo(
		registration,
		modelRuntime,
		modelRegistry,
	); err != nil {
		return err
	}
	r.ApplyToSession(r.boundSession)
	return nil
}

func (r *ProtocolExtensionRuntime) unregisterProvider(provider string) {
	if r == nil {
		return
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return
	}
	r.providerMu.Lock()
	filteredRegistrations := r.providerRegistrations[:0]
	for _, registration := range r.providerRegistrations {
		if registration.name != provider {
			filteredRegistrations = append(filteredRegistrations, registration)
		}
	}
	r.providerRegistrations = filteredRegistrations
	filtered := r.pendingProviders[:0]
	for _, registration := range r.pendingProviders {
		if registration.name != provider {
			filtered = append(filtered, registration)
		}
	}
	r.pendingProviders = filtered
	r.rebuildProviderMapsLocked()
	modelRuntime := r.modelRuntime
	modelRegistry := r.modelRegistry
	r.providerMu.Unlock()
	if modelRuntime != nil {
		modelRuntime.UnregisterProvider(provider)
	} else if modelRegistry != nil {
		modelRegistry.UnregisterProvider(provider)
	}
	r.ApplyToSession(r.boundSession)
}

func (r *ProtocolExtensionRuntime) rebuildProviderState(affected map[string]bool) {
	if r == nil {
		return
	}
	r.providerMu.Lock()
	r.rebuildProviderMapsLocked()
	modelRuntime := r.modelRuntime
	modelRegistry := r.modelRegistry
	registrations := append(
		[]protocolProviderRegistration(nil),
		r.providerRegistrations...,
	)
	r.providerMu.Unlock()
	if modelRuntime == nil && modelRegistry == nil {
		return
	}
	names := sortedBoolMapKeys(affected)
	for _, name := range names {
		if modelRuntime != nil {
			modelRuntime.UnregisterProvider(name)
		} else {
			modelRegistry.UnregisterProvider(name)
		}
		for _, registration := range registrations {
			if registration.name != name {
				continue
			}
			if err := applyProviderRegistrationTo(
				registration,
				modelRuntime,
				modelRegistry,
			); err != nil {
				r.emitExtensionError(ProtocolExtensionError{
					ExtensionPath: registration.source.Path,
					Event:         "register_provider",
					Error:         err.Error(),
				})
			}
		}
	}
}

func (r *ProtocolExtensionRuntime) rebuildProviderMapsLocked() {
	if r == nil {
		return
	}
	r.providerOverrides = map[string]ProtocolProviderOverride{}
	r.providerSources = map[string]ProtocolSourceInfo{}
	for _, registration := range r.providerRegistrations {
		if registration.name == "" {
			continue
		}
		existing := r.providerOverrides[registration.name]
		r.providerOverrides[registration.name] = mergeProtocolProviderOverride(existing, registration.config)
		r.providerSources[registration.name] = registration.source
	}
}

func mergeProtocolProviderOverride(existing, incoming ProtocolProviderOverride) ProtocolProviderOverride {
	merged := cloneProtocolProviderOverride(existing)
	incoming = cloneProtocolProviderOverride(incoming)
	if incoming.BaseURL != "" {
		merged.BaseURL = incoming.BaseURL
	}
	if incoming.APIKey != "" {
		merged.APIKey = incoming.APIKey
	}
	if incoming.API != "" {
		merged.API = incoming.API
	}
	if incoming.Headers != nil {
		merged.Headers = cloneOptionalStringMap(incoming.Headers)
	}
	if incoming.AuthHeader != nil {
		authHeader := *incoming.AuthHeader
		merged.AuthHeader = &authHeader
	}
	if hasCompat(incoming.Compat) {
		merged.Compat = mergeCompat(merged.Compat, incoming.Compat)
	}
	if incoming.Models != nil {
		merged.Models = cloneProviderModelDefinitions(incoming.Models)
	}
	if len(incoming.ModelOverrides) > 0 {
		merged.ModelOverrides = cloneModelOverrideMap(incoming.ModelOverrides)
	}
	if incoming.StreamSimple != nil {
		merged.StreamSimple = incoming.StreamSimple
	}
	return merged
}

func cloneProtocolProviderOverride(
	override ProtocolProviderOverride,
) ProtocolProviderOverride {
	override.Headers = cloneOptionalStringMap(override.Headers)
	if override.AuthHeader != nil {
		authHeader := *override.AuthHeader
		override.AuthHeader = &authHeader
	}
	override.Models = cloneProviderModelDefinitions(override.Models)
	override.ModelOverrides = cloneModelOverrideMap(
		override.ModelOverrides,
	)
	override.Compat = cloneRuntimeModel(llm.Model{
		Compat: override.Compat,
	}).Compat
	return override
}

func applyProviderRegistrationTo(
	registration protocolProviderRegistration,
	modelRuntime *ModelRuntime,
	modelRegistry *ModelRegistry,
) error {
	config := registration.config.toProviderConfigInput()
	if modelRuntime != nil {
		return modelRuntime.RegisterProvider(
			registration.name,
			config,
		)
	}
	if modelRegistry != nil {
		return modelRegistry.RegisterProvider(
			registration.name,
			config,
		)
	}
	return nil
}

func (o ProtocolProviderOverride) toProviderConfigInput() ProviderConfigInput {
	var authHeader *bool
	if o.AuthHeader != nil {
		value := *o.AuthHeader
		authHeader = &value
	}
	return ProviderConfigInput{
		BaseURL:    o.BaseURL,
		APIKey:     o.APIKey,
		API:        o.API,
		Headers:    cloneOptionalStringMap(o.Headers),
		AuthHeader: authHeader,
		Compat: cloneRuntimeModel(llm.Model{
			Compat: o.Compat,
		}).Compat,
		Models:         cloneProviderModelDefinitions(o.Models),
		ModelOverrides: cloneModelOverrideMap(o.ModelOverrides),
		StreamSimple:   o.StreamSimple,
	}
}

func cloneModelOverrideMap(values map[string]ModelOverride) map[string]ModelOverride {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]ModelOverride, len(values))
	for key, value := range values {
		copy := value
		if value.Reasoning != nil {
			reasoning := *value.Reasoning
			copy.Reasoning = &reasoning
		}
		if value.Cost != nil {
			cost := *value.Cost
			if value.Cost.Tiers != nil {
				tiers := append(
					[]llm.ModelCostTier(nil),
					(*value.Cost.Tiers)...,
				)
				cost.Tiers = &tiers
			}
			copy.Cost = &cost
		}
		copy.ThinkingLevelMap = cloneThinkingLevelMap(value.ThinkingLevelMap)
		if value.Input != nil {
			copy.Input = append([]string{}, value.Input...)
		}
		copy.Headers = cloneStringMap(value.Headers)
		result[key] = copy
	}
	return result
}

func (r *ProtocolExtensionRuntime) PendingProviderRegistrations() []ProtocolProviderRegistration {
	if r == nil {
		return nil
	}
	r.providerMu.RLock()
	defer r.providerMu.RUnlock()
	result := make([]ProtocolProviderRegistration, 0, len(r.pendingProviders))
	for _, registration := range r.pendingProviders {
		result = append(result, ProtocolProviderRegistration{
			Name:       registration.name,
			Config:     cloneProtocolProviderOverride(registration.config),
			SourceInfo: registration.source,
		})
	}
	return result
}

func (c *ProtocolExtensionContext) RegisterTool(definition ProtocolToolDefinition) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	if !c.runtime.capabilities[CapabilityToolsRegister] {
		return ProtocolRuntimeError{Code: "missing_capability", Message: CapabilityToolsRegister}
	}
	sourceInfo := c.source
	if sourceInfo.Source == "" {
		sourceInfo = ProtocolSourceInfo{Path: c.source.Path, Source: "inline", Scope: "temporary", Origin: "top-level"}
	}
	tool := SDKTool{
		Name:               definition.Name,
		Label:              definition.Label,
		Description:        definition.Description,
		Parameters:         definition.Parameters,
		PromptSnippet:      definition.PromptSnippet,
		PromptGuidelines:   append([]string(nil), definition.PromptGuidelines...),
		ExecutionMode:      definition.ExecutionMode,
		PrepareArguments:   definition.PrepareArguments,
		Execute:            definition.Execute,
		ExecuteWithUpdates: definition.ExecuteWithUpdates,
		SourceInfo:         sourceInfo,
	}
	c.runtime.registryMu.Lock()
	replaced := false
	for index, existing := range c.runtime.tools {
		if existing.Name == tool.Name && protocolSourceInfoEqual(existing.SourceInfo, sourceInfo) {
			c.runtime.tools[index] = tool
			replaced = true
			break
		}
	}
	if !replaced {
		c.runtime.tools = append(c.runtime.tools, tool)
	}
	c.runtime.registryMu.Unlock()
	c.runtime.ApplyToSession(c.runtime.boundSession)
	return nil
}

func (c *ProtocolExtensionContext) RegisterMessageRenderer(customType string, renderer ProtocolMessageRenderer) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	if !c.runtime.capabilities[CapabilityTUIMessageRenderer] {
		return ProtocolRuntimeError{Code: "missing_capability", Message: CapabilityTUIMessageRenderer}
	}
	if strings.TrimSpace(customType) == "" || renderer == nil {
		return nil
	}
	registration := ProtocolMessageRendererRegistration{
		CustomType: customType,
		SourceInfo: c.source,
		Renderer:   renderer,
	}
	c.runtime.registryMu.Lock()
	replaced := false
	for index, existing := range c.runtime.messageRegistrations {
		if existing.CustomType == customType && protocolSourceInfoEqual(existing.SourceInfo, c.source) {
			c.runtime.messageRegistrations[index] = registration
			replaced = true
			break
		}
	}
	if !replaced {
		c.runtime.messageRegistrations = append(c.runtime.messageRegistrations, registration)
	}
	c.runtime.rebuildMessageRenderersLocked()
	c.runtime.registryMu.Unlock()
	c.runtime.notifyMessageRenderersChanged()
	return nil
}

func (c *ProtocolExtensionContext) RegisterEntryRenderer(customType string, renderer ProtocolEntryRenderer) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	if !c.runtime.capabilities[CapabilityTUIMessageRenderer] {
		return ProtocolRuntimeError{Code: "missing_capability", Message: CapabilityTUIMessageRenderer}
	}
	customType = strings.TrimSpace(customType)
	if customType == "" || renderer == nil {
		return nil
	}
	registration := ProtocolEntryRendererRegistration{
		CustomType: customType,
		SourceInfo: c.source,
		Renderer:   renderer,
	}
	c.runtime.registryMu.Lock()
	replaced := false
	for index, existing := range c.runtime.entryRegistrations {
		if existing.CustomType == customType && protocolSourceInfoEqual(existing.SourceInfo, c.source) {
			c.runtime.entryRegistrations[index] = registration
			replaced = true
			break
		}
	}
	if !replaced {
		c.runtime.entryRegistrations = append(c.runtime.entryRegistrations, registration)
	}
	c.runtime.rebuildEntryRenderersLocked()
	c.runtime.registryMu.Unlock()
	c.runtime.notifyMessageRenderersChanged()
	return nil
}

func (c *ProtocolExtensionContext) RegisterToolRenderer(toolName string, definition ProtocolToolRendererDefinition) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	if !c.runtime.capabilities[CapabilityTUIToolRenderer] {
		return ProtocolRuntimeError{Code: "missing_capability", Message: CapabilityTUIToolRenderer}
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" || (definition.RenderCall == nil && definition.RenderResult == nil) {
		return nil
	}
	registration := ProtocolToolRendererRegistration{
		Name:         toolName,
		SourceInfo:   c.source,
		RenderCall:   definition.RenderCall,
		RenderResult: definition.RenderResult,
	}
	c.runtime.registryMu.Lock()
	defer c.runtime.registryMu.Unlock()
	for index, existing := range c.runtime.toolRendererRegistrations {
		if existing.Name == toolName && protocolSourceInfoEqual(existing.SourceInfo, c.source) {
			c.runtime.toolRendererRegistrations[index] = registration
			c.runtime.rebuildToolRenderersLocked()
			return nil
		}
	}
	c.runtime.toolRendererRegistrations = append(c.runtime.toolRendererRegistrations, registration)
	c.runtime.rebuildToolRenderersLocked()
	return nil
}

func (c *ProtocolExtensionContext) RegisterFlag(name string, definition ProtocolFlagDefinition) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	name = normalizeProtocolFlagName(name)
	if name == "" {
		return nil
	}
	registration := ProtocolFlagRegistration{
		Name:        name,
		Description: definition.Description,
		Type:        definition.Type,
		Default:     definition.Default,
		SourceInfo:  c.source,
	}
	c.runtime.registryMu.Lock()
	defer c.runtime.registryMu.Unlock()
	for index, existing := range c.runtime.flags {
		if existing.Name == name && protocolSourceInfoEqual(existing.SourceInfo, c.source) {
			c.runtime.flags[index] = registration
			c.runtime.applyVisibleFlagValueLocked(name)
			return nil
		}
	}
	c.runtime.flags = append(c.runtime.flags, registration)
	c.runtime.applyVisibleFlagValueLocked(name)
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
	registration := ProtocolShortcutRegistration{
		Key:         key,
		Description: definition.Description,
		SourceInfo:  c.source,
		Handler:     definition.Handler,
	}
	c.runtime.registryMu.Lock()
	defer c.runtime.registryMu.Unlock()
	for index, existing := range c.runtime.shortcuts {
		if existing.Key == key && protocolSourceInfoEqual(existing.SourceInfo, c.source) {
			c.runtime.shortcuts[index] = registration
			return nil
		}
	}
	c.runtime.shortcuts = append(c.runtime.shortcuts, registration)
	return nil
}

func (c *ProtocolExtensionContext) RegisterAutocompleteProvider(id string, definition ProtocolAutocompleteProviderDefinition) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	if !c.runtime.capabilities[CapabilityTUIAutocomplete] {
		return ProtocolRuntimeError{Code: "missing_capability", Message: CapabilityTUIAutocomplete}
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	registration := ProtocolAutocompleteProviderRegistration{
		ID:          id,
		Description: definition.Description,
		Priority:    definition.Priority,
		SourceInfo:  c.source,
		Handler:     definition.Handler,
	}
	c.runtime.registryMu.Lock()
	replaced := false
	for index, existing := range c.runtime.autocomplete {
		if existing.ID == id && protocolSourceInfoEqual(existing.SourceInfo, c.source) {
			c.runtime.autocomplete[index] = registration
			replaced = true
			break
		}
	}
	if !replaced {
		c.runtime.autocomplete = append(c.runtime.autocomplete, registration)
	}
	c.runtime.registryMu.Unlock()
	c.runtime.notifyAutocompleteProvidersChanged()
	return nil
}

func (c *ProtocolExtensionContext) MountViewTree(mountID, slot string, view ViewTreeNode, options ...ViewTreeMountOptions) error {
	if c == nil || c.runtime == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	return c.runtime.registerViewTreeMount(c.source, mountID, slot, view, options...)
}

func (r *ProtocolExtensionRuntime) registerViewTreeMount(source ProtocolSourceInfo, mountID, slot string, view ViewTreeNode, options ...ViewTreeMountOptions) error {
	if r == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension runtime is unavailable"}
	}
	mountID = strings.TrimSpace(mountID)
	slot = canonicalViewTreeSlot(slot)
	if mountID == "" {
		return ProtocolRuntimeError{Code: "invalid_viewtree_mount", Message: "mountId is required"}
	}
	required := viewTreeSlotCapability(slot)
	if !r.capabilities[required] {
		return ProtocolRuntimeError{Code: "missing_capability", Message: required}
	}
	priority := 0
	var overlay *ViewTreeOverlayOptions
	if len(options) > 0 {
		priority = options[0].Priority
		overlay = options[0].Overlay
	}
	registration := ProtocolViewTreeMountRegistration{
		MountID:    mountID,
		Slot:       slot,
		View:       view,
		Priority:   priority,
		Overlay:    overlay,
		SourceInfo: source,
	}
	r.registryMu.Lock()
	replaced := false
	for index, existing := range r.viewTreeMounts {
		if existing.MountID == mountID {
			r.viewTreeMounts[index] = registration
			replaced = true
			break
		}
	}
	if !replaced {
		r.viewTreeMounts = append(r.viewTreeMounts, registration)
	}
	r.registryMu.Unlock()
	return r.applyViewTreeMount(registration)
}

func (r *ProtocolExtensionRuntime) ViewTreeMounts() []ProtocolViewTreeMountRegistration {
	if r == nil {
		return nil
	}
	r.registryMu.RLock()
	defer r.registryMu.RUnlock()
	return append([]ProtocolViewTreeMountRegistration(nil), r.viewTreeMounts...)
}

func (r *ProtocolExtensionRuntime) applyViewTreeMounts() {
	if r == nil || r.viewTreeHost == nil {
		return
	}
	for _, registration := range r.ViewTreeMounts() {
		_ = r.applyViewTreeMount(registration)
	}
}

func (r *ProtocolExtensionRuntime) applyViewTreeMount(registration ProtocolViewTreeMountRegistration) error {
	if r == nil || r.viewTreeHost == nil {
		return nil
	}
	return r.viewTreeHost.MountWithOptions(
		registration.MountID,
		registration.Slot,
		registration.View,
		ViewTreeMountOptions{Priority: registration.Priority, Overlay: registration.Overlay},
	)
}

func (r *ProtocolExtensionRuntime) OnAutocompleteProvidersChanged(callback func()) func() {
	if r == nil || callback == nil {
		return func() {}
	}
	r.registryMu.Lock()
	r.nextAutocomplete++
	id := r.nextAutocomplete
	r.autocompleteWatch = append(r.autocompleteWatch, protocolAutocompleteWatcher{id: id, callback: callback})
	r.registryMu.Unlock()
	return func() {
		r.registryMu.Lock()
		defer r.registryMu.Unlock()
		for index, watcher := range r.autocompleteWatch {
			if watcher.id != id {
				continue
			}
			r.autocompleteWatch = append(r.autocompleteWatch[:index], r.autocompleteWatch[index+1:]...)
			return
		}
	}
}

func (r *ProtocolExtensionRuntime) notifyAutocompleteProvidersChanged() {
	if r == nil {
		return
	}
	r.registryMu.RLock()
	watchers := append([]protocolAutocompleteWatcher(nil), r.autocompleteWatch...)
	r.registryMu.RUnlock()
	for _, watcher := range watchers {
		if watcher.callback != nil {
			watcher.callback()
		}
	}
}

func (r *ProtocolExtensionRuntime) OnCommandsChanged(callback func()) func() {
	if r == nil || callback == nil {
		return func() {}
	}
	r.registryMu.Lock()
	r.nextCommandWatch++
	id := r.nextCommandWatch
	r.commandWatch = append(r.commandWatch, protocolCommandWatcher{id: id, callback: callback})
	r.registryMu.Unlock()
	return func() {
		r.registryMu.Lock()
		defer r.registryMu.Unlock()
		for index, watcher := range r.commandWatch {
			if watcher.id != id {
				continue
			}
			r.commandWatch = append(r.commandWatch[:index], r.commandWatch[index+1:]...)
			return
		}
	}
}

func (r *ProtocolExtensionRuntime) notifyCommandsChanged() {
	if r == nil {
		return
	}
	r.registryMu.RLock()
	watchers := append([]protocolCommandWatcher(nil), r.commandWatch...)
	r.registryMu.RUnlock()
	for _, watcher := range watchers {
		if watcher.callback != nil {
			watcher.callback()
		}
	}
}

func (r *ProtocolExtensionRuntime) OnMessageRenderersChanged(callback func()) func() {
	if r == nil || callback == nil {
		return func() {}
	}
	r.registryMu.Lock()
	r.nextMessageRender++
	id := r.nextMessageRender
	r.messageRenderWatch = append(r.messageRenderWatch, protocolMessageRendererWatcher{id: id, callback: callback})
	r.registryMu.Unlock()
	return func() {
		r.registryMu.Lock()
		defer r.registryMu.Unlock()
		for index, watcher := range r.messageRenderWatch {
			if watcher.id != id {
				continue
			}
			r.messageRenderWatch = append(r.messageRenderWatch[:index], r.messageRenderWatch[index+1:]...)
			return
		}
	}
}

func (r *ProtocolExtensionRuntime) notifyMessageRenderersChanged() {
	if r == nil {
		return
	}
	r.registryMu.RLock()
	watchers := append([]protocolMessageRendererWatcher(nil), r.messageRenderWatch...)
	r.registryMu.RUnlock()
	for _, watcher := range watchers {
		if watcher.callback != nil {
			watcher.callback()
		}
	}
}

func (r *ProtocolExtensionRuntime) RegisteredCommands() []ProtocolCommandRegistration {
	if r == nil {
		return nil
	}
	r.registryMu.RLock()
	commands := append([]ProtocolCommandRegistration(nil), r.commands...)
	r.registryMu.RUnlock()
	counts := map[string]int{}
	for _, command := range commands {
		counts[command.Name]++
	}
	ordinals := map[string]int{}
	takenInvocationNames := map[string]bool{}
	result := make([]ProtocolCommandRegistration, 0, len(commands))
	for _, command := range commands {
		ordinals[command.Name]++
		command.InvocationName = command.Name
		if counts[command.Name] > 1 {
			command.InvocationName = fmt.Sprintf("%s:%d", command.Name, ordinals[command.Name])
		}
		if takenInvocationNames[command.InvocationName] {
			suffix := ordinals[command.Name]
			for {
				suffix++
				command.InvocationName = fmt.Sprintf("%s:%d", command.Name, suffix)
				if !takenInvocationNames[command.InvocationName] {
					break
				}
			}
		}
		takenInvocationNames[command.InvocationName] = true
		result = append(result, command)
	}
	return result
}

func (r *ProtocolExtensionRuntime) AutocompleteProviders() []ProtocolAutocompleteProviderRegistration {
	if r == nil {
		return nil
	}
	r.registryMu.RLock()
	result := append([]ProtocolAutocompleteProviderRegistration(nil), r.autocomplete...)
	r.registryMu.RUnlock()
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Priority > result[j].Priority
	})
	return result
}

func (r *ProtocolExtensionRuntime) SuggestAutocomplete(ctx context.Context, request ProtocolAutocompleteRequest) (ProtocolAutocompleteResult, error) {
	for _, provider := range r.AutocompleteProviders() {
		if provider.Handler == nil {
			continue
		}
		result, err := provider.Handler(ctx, request)
		if err != nil {
			return ProtocolAutocompleteResult{}, err
		}
		if len(result.Items) > 0 {
			return result, nil
		}
	}
	return ProtocolAutocompleteResult{}, nil
}

func (r *ProtocolExtensionRuntime) RegisteredTools() []SDKTool {
	if r == nil {
		return nil
	}
	r.registryMu.RLock()
	tools := append([]SDKTool(nil), r.tools...)
	r.registryMu.RUnlock()
	seen := map[string]bool{}
	result := make([]SDKTool, 0, len(tools))
	for _, tool := range tools {
		if tool.Name == "" || seen[tool.Name] {
			continue
		}
		seen[tool.Name] = true
		result = append(result, tool)
	}
	return result
}

// rebuildMessageRenderersLocked rebuilds the visible renderer projection.
// registryMu must be held for writing.
func (r *ProtocolExtensionRuntime) rebuildMessageRenderersLocked() {
	if r == nil {
		return
	}
	r.messageRenderers = map[string]ProtocolMessageRenderer{}
	r.messageSources = map[string]ProtocolSourceInfo{}
	for _, registration := range r.messageRegistrations {
		if registration.CustomType == "" || registration.Renderer == nil {
			continue
		}
		if _, exists := r.messageRenderers[registration.CustomType]; exists {
			continue
		}
		r.messageRenderers[registration.CustomType] = registration.Renderer
		r.messageSources[registration.CustomType] = registration.SourceInfo
	}
}

func (r *ProtocolExtensionRuntime) GetMessageRenderer(customType string) ProtocolMessageRenderer {
	if r == nil {
		return nil
	}
	r.registryMu.RLock()
	defer r.registryMu.RUnlock()
	return r.messageRenderers[customType]
}

// rebuildEntryRenderersLocked rebuilds the visible renderer projection.
// registryMu must be held for writing. Earlier extensions keep precedence,
// matching message renderers and the extension load order.
func (r *ProtocolExtensionRuntime) rebuildEntryRenderersLocked() {
	if r == nil {
		return
	}
	r.entryRenderers = map[string]ProtocolEntryRenderer{}
	r.entrySources = map[string]ProtocolSourceInfo{}
	for _, registration := range r.entryRegistrations {
		if registration.CustomType == "" || registration.Renderer == nil {
			continue
		}
		if _, exists := r.entryRenderers[registration.CustomType]; exists {
			continue
		}
		r.entryRenderers[registration.CustomType] = registration.Renderer
		r.entrySources[registration.CustomType] = registration.SourceInfo
	}
}

func (r *ProtocolExtensionRuntime) GetEntryRenderer(customType string) ProtocolEntryRenderer {
	if r == nil {
		return nil
	}
	r.registryMu.RLock()
	defer r.registryMu.RUnlock()
	return r.entryRenderers[customType]
}

func (r *ProtocolExtensionRuntime) GetToolRenderer(toolName string) *ProtocolToolRendererRegistration {
	if r == nil {
		return nil
	}
	r.registryMu.RLock()
	defer r.registryMu.RUnlock()
	renderer, ok := r.toolRenderers[toolName]
	if !ok {
		return nil
	}
	copy := renderer
	return &copy
}

// rebuildToolRenderersLocked rebuilds the visible renderer projection.
// registryMu must be held for writing.
func (r *ProtocolExtensionRuntime) rebuildToolRenderersLocked() {
	if r == nil {
		return
	}
	r.toolRenderers = map[string]ProtocolToolRendererRegistration{}
	for _, registration := range r.toolRendererRegistrations {
		if registration.Name == "" || (registration.RenderCall == nil && registration.RenderResult == nil) {
			continue
		}
		if _, exists := r.toolRenderers[registration.Name]; exists {
			continue
		}
		r.toolRenderers[registration.Name] = registration
	}
}

func (r *ProtocolExtensionRuntime) GetRegisteredToolDefinition(toolName string) ToolDefinition {
	renderer := r.GetToolRenderer(toolName)
	if renderer == nil {
		return ToolDefinition{Name: toolName}
	}
	return ToolDefinition{
		Name:         toolName,
		RenderCall:   renderer.RenderCall,
		RenderResult: renderer.RenderResult,
	}
}

func (r *ProtocolExtensionRuntime) Flags() []ProtocolFlagRegistration {
	if r == nil {
		return nil
	}
	r.registryMu.RLock()
	defer r.registryMu.RUnlock()
	return r.visibleFlagRegistrationsLocked()
}

func (r *ProtocolExtensionRuntime) SetCLIFlagValues(values map[string]any) []ProtocolExtensionFlagDiagnostic {
	if r == nil || len(values) == 0 {
		return nil
	}
	r.registryMu.Lock()
	defer r.registryMu.Unlock()
	if r.cliFlagValues == nil {
		r.cliFlagValues = map[string]any{}
	}
	keys := sortedAnyMapKeys(values)
	for _, key := range keys {
		name := normalizeProtocolFlagName(key)
		if name == "" {
			continue
		}
		r.cliFlagValues[name] = values[key]
	}
	before := len(r.flagDiagnostics)
	for _, flag := range r.visibleFlagRegistrationsLocked() {
		r.applyCLIFlagValueToRegistrationLocked(flag)
	}
	return append([]ProtocolExtensionFlagDiagnostic(nil), r.flagDiagnostics[before:]...)
}

func (r *ProtocolExtensionRuntime) FlagDiagnostics() []ProtocolExtensionFlagDiagnostic {
	if r == nil {
		return nil
	}
	r.registryMu.RLock()
	defer r.registryMu.RUnlock()
	if len(r.flagDiagnostics) == 0 {
		return nil
	}
	return append([]ProtocolExtensionFlagDiagnostic(nil), r.flagDiagnostics...)
}

func (r *ProtocolExtensionRuntime) UnknownCLIFlagDiagnostics() []ProtocolExtensionFlagDiagnostic {
	if r == nil {
		return nil
	}
	r.registryMu.RLock()
	defer r.registryMu.RUnlock()
	if len(r.cliFlagValues) == 0 {
		return nil
	}
	registered := map[string]bool{}
	for _, flag := range r.visibleFlagRegistrationsLocked() {
		registered[flag.Name] = true
	}
	names := make([]string, 0, len(r.cliFlagValues))
	for name := range r.cliFlagValues {
		if !registered[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	diagnostics := make([]ProtocolExtensionFlagDiagnostic, 0, len(names))
	for _, name := range names {
		diagnostics = append(diagnostics, ProtocolExtensionFlagDiagnostic{
			Name:    name,
			Message: "Unknown option: --" + name,
		})
	}
	return diagnostics
}

func (r *ProtocolExtensionRuntime) applyCLIFlagValueToRegistrationLocked(flag ProtocolFlagRegistration) {
	if r == nil || len(r.cliFlagValues) == 0 {
		return
	}
	name := normalizeProtocolFlagName(flag.Name)
	if name == "" {
		return
	}
	value, ok := r.cliFlagValues[name]
	if !ok {
		return
	}
	switch strings.ToLower(strings.TrimSpace(flag.Type)) {
	case "boolean", "bool":
		r.flagValues[name] = true
	case "", "string":
		text, ok := value.(string)
		if !ok {
			r.appendFlagDiagnosticLocked(ProtocolExtensionFlagDiagnostic{
				Name:    name,
				Message: `Extension flag "--` + name + `" requires a value`,
			})
			return
		}
		r.flagValues[name] = text
	default:
		r.flagValues[name] = value
	}
}

func (r *ProtocolExtensionRuntime) applyVisibleFlagValueLocked(name string) {
	if r == nil {
		return
	}
	name = normalizeProtocolFlagName(name)
	if name == "" {
		return
	}
	flag, ok := r.visibleFlagRegistrationLocked(name)
	if !ok {
		delete(r.flagValues, name)
		return
	}
	if flag.Default != nil {
		if _, exists := r.flagValues[name]; !exists {
			r.flagValues[name] = flag.Default
		}
	}
	r.applyCLIFlagValueToRegistrationLocked(flag)
}

func (r *ProtocolExtensionRuntime) visibleFlagRegistrationLocked(name string) (ProtocolFlagRegistration, bool) {
	if r == nil {
		return ProtocolFlagRegistration{}, false
	}
	name = normalizeProtocolFlagName(name)
	if name == "" {
		return ProtocolFlagRegistration{}, false
	}
	for _, flag := range r.flags {
		if flag.Name == name {
			return flag, true
		}
	}
	return ProtocolFlagRegistration{}, false
}

func (r *ProtocolExtensionRuntime) visibleFlagRegistrationsLocked() []ProtocolFlagRegistration {
	if r == nil {
		return nil
	}
	seen := map[string]bool{}
	result := make([]ProtocolFlagRegistration, 0, len(r.flags))
	for _, flag := range r.flags {
		if flag.Name == "" || seen[flag.Name] {
			continue
		}
		seen[flag.Name] = true
		result = append(result, flag)
	}
	return result
}

func (r *ProtocolExtensionRuntime) appendFlagDiagnosticLocked(diagnostic ProtocolExtensionFlagDiagnostic) {
	if r == nil || strings.TrimSpace(diagnostic.Message) == "" {
		return
	}
	for _, existing := range r.flagDiagnostics {
		if existing.Name == diagnostic.Name && existing.Message == diagnostic.Message {
			return
		}
	}
	r.flagDiagnostics = append(r.flagDiagnostics, diagnostic)
}

func (r *ProtocolExtensionRuntime) SetFlagValue(name string, value any) {
	if r == nil {
		return
	}
	name = normalizeProtocolFlagName(name)
	if name == "" {
		return
	}
	r.registryMu.Lock()
	defer r.registryMu.Unlock()
	r.flagValues[name] = value
}

func (r *ProtocolExtensionRuntime) FlagValue(name string) any {
	if r == nil {
		return nil
	}
	name = normalizeProtocolFlagName(name)
	r.registryMu.RLock()
	defer r.registryMu.RUnlock()
	return r.flagValues[name]
}

func normalizeProtocolFlagName(name string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "--"))
}

func sortedAnyMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedBoolMapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (r *ProtocolExtensionRuntime) ApplyToSession(session *AgentSession) {
	if r == nil || session == nil || session.Agent == nil {
		return
	}
	r.providerMu.RLock()
	override, ok := r.providerOverrides[session.Agent.State.Model.Provider]
	r.providerMu.RUnlock()
	if ok {
		if override.BaseURL != "" {
			session.Agent.State.Model.BaseURL = override.BaseURL
		}
	}
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
	r.registryMu.RLock()
	shortcuts := append([]ProtocolShortcutRegistration(nil), r.shortcuts...)
	r.registryMu.RUnlock()
	for _, shortcut := range shortcuts {
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
	"app.interrupt":              true,
	"app.clear":                  true,
	"app.exit":                   true,
	"app.suspend":                true,
	"app.thinking.cycle":         true,
	"app.model.cycleForward":     true,
	"app.model.cycleBackward":    true,
	"app.model.select":           true,
	"app.tools.expand":           true,
	"app.thinking.toggle":        true,
	"app.editor.external":        true,
	"app.message.copy":           true,
	"app.message.followUp":       true,
	"tui.input.submit":           true,
	"tui.select.confirm":         true,
	"tui.select.cancel":          true,
	"tui.input.copy":             true,
	"tui.editor.deleteToLineEnd": true,
}

func DefaultProtocolKeybindings() KeybindingsConfig {
	return KeybindingsConfig{
		"app.interrupt":                 "escape",
		"app.clear":                     "ctrl+c",
		"app.exit":                      "ctrl+d",
		"app.suspend":                   "ctrl+z",
		"app.thinking.cycle":            "shift+tab",
		"app.model.cycleForward":        "ctrl+p",
		"app.model.cycleBackward":       "shift+ctrl+p",
		"app.model.select":              "ctrl+l",
		"app.tools.expand":              "ctrl+o",
		"app.thinking.toggle":           "ctrl+t",
		"app.session.toggleNamedFilter": "ctrl+n",
		"app.editor.external":           "ctrl+g",
		"app.message.copy":              "ctrl+x",
		"app.message.followUp":          "alt+enter",
		"app.message.dequeue":           "alt+up",
		"app.clipboard.pasteImage":      DefaultClipboardPasteImageKey(),
		"app.session.new":               []any{},
		"app.session.tree":              []any{},
		"app.session.fork":              []any{},
		"app.session.resume":            []any{},
		"app.tree.foldOrUp":             []any{"ctrl+left", "alt+left"},
		"app.tree.unfoldOrDown":         []any{"ctrl+right", "alt+right"},
		"app.tree.editLabel":            "shift+l",
		"app.tree.toggleLabelTimestamp": "shift+t",
		"app.session.togglePath":        "ctrl+p",
		"app.session.toggleSort":        "ctrl+s",
		"app.session.rename":            "ctrl+r",
		"app.session.delete":            "ctrl+d",
		"app.session.deleteNoninvasive": "ctrl+backspace",
		"app.models.save":               "ctrl+s",
		"app.models.enableAll":          "ctrl+a",
		"app.models.clearAll":           "ctrl+x",
		"app.models.toggleProvider":     "ctrl+p",
		"app.models.reorderUp":          "alt+up",
		"app.models.reorderDown":        "alt+down",
		"app.tree.filter.default":       "ctrl+d",
		"app.tree.filter.noTools":       "ctrl+t",
		"app.tree.filter.userOnly":      "ctrl+u",
		"app.tree.filter.labeledOnly":   "ctrl+l",
		"app.tree.filter.all":           "ctrl+a",
		"app.tree.filter.cycleForward":  "ctrl+o",
		"app.tree.filter.cycleBackward": "shift+ctrl+o",
		"tui.input.submit":              "enter",
		"tui.select.confirm":            "enter",
		"tui.select.cancel":             []any{"escape", "ctrl+c"},
		"tui.input.copy":                "ctrl+c",
		"tui.editor.deleteToLineEnd":    "ctrl+k",
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
