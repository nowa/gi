package harness

import (
	"context"
	"fmt"
	"strings"
	"sync"

	core "github.com/nowa/gi/gi-agent-core"
	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	AgentHarnessPhaseIdle          = "idle"
	AgentHarnessPhaseTurn          = "turn"
	AgentHarnessPhaseCompaction    = "compaction"
	AgentHarnessPhaseBranchSummary = "branch_summary"

	harnessSubscriberEventType = "*"
)

type Optional[T any] struct {
	Set   bool
	Value T
}

func Some[T any](value T) Optional[T] {
	return Optional[T]{Set: true, Value: value}
}

type AgentHarnessStreamOptions struct {
	Transport       string
	TimeoutMillis   int
	MaxRetries      int
	MaxRetryDelayMs int
	Headers         map[string]string
	Metadata        map[string]any
	CacheRetention  string
}

type AgentHarnessStreamOptionsPatch struct {
	Transport       Optional[string]
	TimeoutMillis   Optional[int]
	MaxRetries      Optional[int]
	MaxRetryDelayMs Optional[int]
	CacheRetention  Optional[string]
	Headers         map[string]*string
	ClearHeaders    bool
	Metadata        map[string]Optional[any]
	ClearMetadata   bool
}

type AgentHarnessResources struct {
	Skills          []Skill
	PromptTemplates []PromptTemplate
}

func (r AgentHarnessResources) Clone() AgentHarnessResources {
	return AgentHarnessResources{
		Skills:          append([]Skill{}, r.Skills...),
		PromptTemplates: append([]PromptTemplate{}, r.PromptTemplates...),
	}
}

type AgentHarnessAuth struct {
	APIKey  string
	Headers map[string]string
}

type NavigateTreeOptions struct {
	Summarize           bool
	CustomInstructions  string
	ReplaceInstructions bool
	Label               string
}

type NavigateTreeResult struct {
	Cancelled    bool
	EditorText   string
	SummaryEntry *Entry
}

type SystemPromptContext struct {
	Env           any
	Session       *Session
	Model         llm.Model
	ThinkingLevel string
	ActiveTools   []core.AgentTool
	Resources     AgentHarnessResources
}

type AgentHarnessOptions struct {
	Env                 any
	Session             *Session
	Model               llm.Model
	ThinkingLevel       string
	SystemPrompt        string
	BuildSystemPrompt   func(context.Context, SystemPromptContext) (string, error)
	GetAPIKeyAndHeaders func(context.Context, llm.Model) (AgentHarnessAuth, bool, error)
	Resources           AgentHarnessResources
	Tools               []core.AgentTool
	ActiveToolNames     []string
	StreamOptions       AgentHarnessStreamOptions
	SteeringMode        string
	FollowUpMode        string
	ToolExecution       string
}

type AgentHarnessEvent struct {
	Type string

	Message     llm.Message
	Messages    []llm.Message
	ToolResults []llm.Message

	ToolCallID string
	ToolName   string
	Input      map[string]any
	Content    []llm.ContentPart
	Details    any
	IsError    bool

	Steer    []llm.Message
	FollowUp []llm.Message
	NextTurn []llm.Message

	HadPendingMutations bool
	NextTurnCount       int

	Model         llm.Model
	PreviousModel *llm.Model
	Source        string

	Level         string
	PreviousLevel string

	Resources         AgentHarnessResources
	PreviousResources AgentHarnessResources

	Prompt       string
	Images       []llm.ContentPart
	SystemPrompt string

	SessionID     string
	StreamOptions AgentHarnessStreamOptions
	Payload       any

	Status  int
	Headers map[string]string

	ClearedSteer    []llm.Message
	ClearedFollowUp []llm.Message

	Preparation        *CompactionPreparation
	BranchEntries      []Entry
	CustomInstructions string
	Compaction         CompactionResult
	CompactionEntry    Entry
	FromHook           bool
	TargetID           string
	OldLeafID          *string
	NewLeafID          *string
	CommonAncestorID   *string
	BranchSummary      BranchSummaryResult
	SummaryEntry       *Entry

	AgentEvent core.AgentEvent
}

type AgentHarnessHookResult struct {
	Messages    []llm.Message
	HasMessages bool

	SystemPrompt    string
	HasSystemPrompt bool

	StreamOptions *AgentHarnessStreamOptionsPatch

	Payload    any
	HasPayload bool

	Block    bool
	HasBlock bool
	Reason   string

	Content      []llm.ContentPart
	HasContent   bool
	Details      any
	HasDetails   bool
	IsError      bool
	HasIsError   bool
	Terminate    bool
	HasTerminate bool

	Cancel                 bool
	Compaction             CompactionResult
	HasCompaction          bool
	BranchSummary          BranchSummaryResult
	HasBranchSummary       bool
	CustomInstructions     string
	HasCustomInstructions  bool
	ReplaceInstructions    bool
	HasReplaceInstructions bool
}

type AgentHarnessHandler func(context.Context, AgentHarnessEvent) (*AgentHarnessHookResult, error)
type AgentHarnessSubscriber func(context.Context, AgentHarnessEvent) error

type AgentHarnessError struct {
	Code string
	Err  error
}

func (e *AgentHarnessError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *AgentHarnessError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newAgentHarnessError(code, format string, args ...any) *AgentHarnessError {
	return &AgentHarnessError{Code: code, Err: fmt.Errorf(format, args...)}
}

func normalizeHarnessError(err error, fallbackCode string) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*AgentHarnessError); ok {
		return err
	}
	if _, ok := err.(*SessionError); ok {
		return &AgentHarnessError{Code: "session", Err: err}
	}
	return &AgentHarnessError{Code: fallbackCode, Err: err}
}

type handlerEntry struct {
	id int
	fn AgentHarnessHandler
}

type pendingSessionWrite struct {
	kind          string
	message       llm.Message
	provider      string
	modelID       string
	thinkingLevel string
	customType    string
	content       any
	display       bool
	details       any
	targetID      string
	label         string
	name          string
}

type agentHarnessTurnState struct {
	Messages      []llm.Message
	Resources     AgentHarnessResources
	StreamOptions AgentHarnessStreamOptions
	SessionID     string
	SystemPrompt  string
	Model         llm.Model
	ThinkingLevel string
	Tools         []core.AgentTool
	ActiveTools   []core.AgentTool
}

type AgentHarness struct {
	Env any

	mu                   sync.Mutex
	session              *Session
	phase                string
	cancel               context.CancelFunc
	done                 chan struct{}
	pendingSessionWrites []pendingSessionWrite
	model                llm.Model
	thinkingLevel        string
	systemPrompt         string
	buildSystemPrompt    func(context.Context, SystemPromptContext) (string, error)
	streamOptions        AgentHarnessStreamOptions
	getAPIKeyAndHeaders  func(context.Context, llm.Model) (AgentHarnessAuth, bool, error)
	resources            AgentHarnessResources
	tools                map[string]core.AgentTool
	activeToolNames      []string
	steerQueue           []llm.Message
	steeringQueueMode    string
	followUpQueue        []llm.Message
	followUpQueueMode    string
	nextTurnQueue        []llm.Message
	toolExecution        string
	handlers             map[string][]handlerEntry
	nextHandlerID        int
}

func NewAgentHarness(options AgentHarnessOptions) (*AgentHarness, error) {
	if options.Session == nil {
		return nil, newAgentHarnessError("invalid_argument", "session is required")
	}
	thinkingLevel := options.ThinkingLevel
	if thinkingLevel == "" {
		thinkingLevel = "off"
	}
	steeringMode := options.SteeringMode
	if steeringMode == "" {
		steeringMode = core.QueueOneAtTime
	}
	followUpMode := options.FollowUpMode
	if followUpMode == "" {
		followUpMode = core.QueueOneAtTime
	}
	toolExecution := options.ToolExecution
	if toolExecution == "" {
		toolExecution = core.ToolExecutionParallel
	}
	tools := make(map[string]core.AgentTool, len(options.Tools))
	activeToolNames := append([]string{}, options.ActiveToolNames...)
	for _, tool := range options.Tools {
		tools[tool.Name] = tool
		if len(options.ActiveToolNames) == 0 {
			activeToolNames = append(activeToolNames, tool.Name)
		}
	}
	if err := validateToolNames(activeToolNames, tools); err != nil {
		return nil, err
	}
	return &AgentHarness{
		Env:                 options.Env,
		session:             options.Session,
		phase:               AgentHarnessPhaseIdle,
		model:               options.Model,
		thinkingLevel:       thinkingLevel,
		systemPrompt:        options.SystemPrompt,
		buildSystemPrompt:   options.BuildSystemPrompt,
		getAPIKeyAndHeaders: options.GetAPIKeyAndHeaders,
		resources:           options.Resources.Clone(),
		streamOptions:       cloneHarnessStreamOptions(options.StreamOptions),
		tools:               tools,
		activeToolNames:     activeToolNames,
		steeringQueueMode:   steeringMode,
		followUpQueueMode:   followUpMode,
		toolExecution:       toolExecution,
		handlers:            map[string][]handlerEntry{},
	}, nil
}

func MustNewAgentHarness(options AgentHarnessOptions) *AgentHarness {
	harness, err := NewAgentHarness(options)
	if err != nil {
		panic(err)
	}
	return harness
}

func (h *AgentHarness) Session() *Session {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.session
}

func (h *AgentHarness) Phase() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.phase
}

func (h *AgentHarness) GetModel() llm.Model {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.model
}

func (h *AgentHarness) GetThinkingLevel() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.thinkingLevel
}

func (h *AgentHarness) GetSteeringMode() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.steeringQueueMode
}

func (h *AgentHarness) SetSteeringMode(mode string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.steeringQueueMode = mode
}

func (h *AgentHarness) GetFollowUpMode() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.followUpQueueMode
}

func (h *AgentHarness) SetFollowUpMode(mode string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.followUpQueueMode = mode
}

func (h *AgentHarness) GetResources() AgentHarnessResources {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.resources.Clone()
}

func (h *AgentHarness) SetResources(ctx context.Context, resources AgentHarnessResources) error {
	previous := h.GetResources()
	h.mu.Lock()
	h.resources = resources.Clone()
	next := h.resources.Clone()
	h.mu.Unlock()
	return h.emitOwn(ctx, AgentHarnessEvent{Type: "resources_update", Resources: next, PreviousResources: previous})
}

func (h *AgentHarness) GetStreamOptions() AgentHarnessStreamOptions {
	h.mu.Lock()
	defer h.mu.Unlock()
	return cloneHarnessStreamOptions(h.streamOptions)
}

func (h *AgentHarness) SetStreamOptions(options AgentHarnessStreamOptions) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.streamOptions = cloneHarnessStreamOptions(options)
}

func (h *AgentHarness) SetModel(ctx context.Context, model llm.Model) error {
	h.mu.Lock()
	phase := h.phase
	previous := h.model
	h.mu.Unlock()
	if phase == AgentHarnessPhaseIdle {
		if _, err := h.session.AppendModelChange(model.Provider, model.ID); err != nil {
			return normalizeHarnessError(err, "session")
		}
	}
	h.mu.Lock()
	if phase != AgentHarnessPhaseIdle {
		h.pendingSessionWrites = append(h.pendingSessionWrites, pendingSessionWrite{kind: "model_change", provider: model.Provider, modelID: model.ID})
	}
	h.model = model
	h.mu.Unlock()
	return h.emitOwn(ctx, AgentHarnessEvent{Type: "model_select", Model: model, PreviousModel: &previous, Source: "set"})
}

func (h *AgentHarness) SetThinkingLevel(ctx context.Context, level string) error {
	h.mu.Lock()
	phase := h.phase
	previous := h.thinkingLevel
	h.mu.Unlock()
	if phase == AgentHarnessPhaseIdle {
		if _, err := h.session.AppendThinkingLevelChange(level); err != nil {
			return normalizeHarnessError(err, "session")
		}
	}
	h.mu.Lock()
	if phase != AgentHarnessPhaseIdle {
		h.pendingSessionWrites = append(h.pendingSessionWrites, pendingSessionWrite{kind: "thinking_level_change", thinkingLevel: level})
	}
	h.thinkingLevel = level
	h.mu.Unlock()
	return h.emitOwn(ctx, AgentHarnessEvent{Type: "thinking_level_select", Level: level, PreviousLevel: previous})
}

func (h *AgentHarness) SetTools(tools []core.AgentTool, activeToolNames ...string) error {
	nextTools := make(map[string]core.AgentTool, len(tools))
	for _, tool := range tools {
		nextTools[tool.Name] = tool
	}
	nextActiveNames := append([]string{}, activeToolNames...)
	if len(nextActiveNames) == 0 {
		h.mu.Lock()
		nextActiveNames = append(nextActiveNames, h.activeToolNames...)
		h.mu.Unlock()
	}
	if err := validateToolNames(nextActiveNames, nextTools); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.tools = nextTools
	h.activeToolNames = nextActiveNames
	return nil
}

func (h *AgentHarness) SetActiveTools(toolNames []string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := validateToolNames(toolNames, h.tools); err != nil {
		return err
	}
	h.activeToolNames = append([]string{}, toolNames...)
	return nil
}

func (h *AgentHarness) Subscribe(listener AgentHarnessSubscriber) func() {
	return h.addHandler(harnessSubscriberEventType, func(ctx context.Context, event AgentHarnessEvent) (*AgentHarnessHookResult, error) {
		return nil, listener(ctx, event)
	})
}

func (h *AgentHarness) On(eventType string, handler AgentHarnessHandler) func() {
	return h.addHandler(eventType, handler)
}

func (h *AgentHarness) Prompt(ctx context.Context, text string, images ...llm.ContentPart) (llm.Message, error) {
	runCtx, finish, err := h.startTurn(ctx)
	if err != nil {
		return llm.Message{}, err
	}
	defer finish()
	turnState, err := h.createTurnState(runCtx)
	if err != nil {
		h.setPhase(AgentHarnessPhaseIdle)
		return llm.Message{}, normalizeHarnessError(err, "session")
	}
	message, err := h.executeTurn(runCtx, turnState, text, images...)
	if err != nil {
		h.setPhase(AgentHarnessPhaseIdle)
		return llm.Message{}, normalizeHarnessError(err, "unknown")
	}
	return message, nil
}

func (h *AgentHarness) Skill(ctx context.Context, name, additionalInstructions string) (llm.Message, error) {
	turnState, err := h.createTurnState(ctx)
	if err != nil {
		return llm.Message{}, normalizeHarnessError(err, "session")
	}
	for _, skill := range turnState.Resources.Skills {
		if skill.Name == name {
			return h.Prompt(ctx, FormatSkillInvocation(skill, additionalInstructions))
		}
	}
	return llm.Message{}, newAgentHarnessError("invalid_argument", "unknown skill: %s", name)
}

func (h *AgentHarness) PromptFromTemplate(ctx context.Context, name string, args []string) (llm.Message, error) {
	turnState, err := h.createTurnState(ctx)
	if err != nil {
		return llm.Message{}, normalizeHarnessError(err, "session")
	}
	for _, template := range turnState.Resources.PromptTemplates {
		if template.Name == name {
			return h.Prompt(ctx, FormatPromptTemplateInvocation(template, args))
		}
	}
	return llm.Message{}, newAgentHarnessError("invalid_argument", "unknown prompt template: %s", name)
}

func (h *AgentHarness) Steer(ctx context.Context, text string, images ...llm.ContentPart) error {
	h.mu.Lock()
	if h.phase == AgentHarnessPhaseIdle {
		h.mu.Unlock()
		return newAgentHarnessError("invalid_state", "cannot steer while idle")
	}
	h.steerQueue = append(h.steerQueue, createHarnessUserMessage(text, images...))
	h.mu.Unlock()
	return h.emitQueueUpdate(ctx)
}

func (h *AgentHarness) FollowUp(ctx context.Context, text string, images ...llm.ContentPart) error {
	h.mu.Lock()
	if h.phase == AgentHarnessPhaseIdle {
		h.mu.Unlock()
		return newAgentHarnessError("invalid_state", "cannot follow up while idle")
	}
	h.followUpQueue = append(h.followUpQueue, createHarnessUserMessage(text, images...))
	h.mu.Unlock()
	return h.emitQueueUpdate(ctx)
}

func (h *AgentHarness) NextTurn(ctx context.Context, text string, images ...llm.ContentPart) error {
	h.mu.Lock()
	h.nextTurnQueue = append(h.nextTurnQueue, createHarnessUserMessage(text, images...))
	h.mu.Unlock()
	return h.emitQueueUpdate(ctx)
}

func (h *AgentHarness) AppendMessage(message llm.Message) error {
	h.mu.Lock()
	phase := h.phase
	if phase != AgentHarnessPhaseIdle {
		h.pendingSessionWrites = append(h.pendingSessionWrites, pendingSessionWrite{kind: "message", message: message})
		h.mu.Unlock()
		return nil
	}
	h.mu.Unlock()
	_, err := h.session.AppendMessage(message)
	return normalizeHarnessError(err, "session")
}

func (h *AgentHarness) Compact(ctx context.Context, customInstructions string) (CompactionResult, error) {
	runCtx, finish, err := h.startExclusivePhase(ctx, AgentHarnessPhaseCompaction, "compact() requires idle harness")
	if err != nil {
		return CompactionResult{}, err
	}
	defer finish()

	h.mu.Lock()
	model := h.model
	thinkingLevel := h.thinkingLevel
	h.mu.Unlock()

	auth, hasAuth, err := h.resolveAuth(runCtx, model)
	if err != nil {
		return CompactionResult{}, normalizeHarnessError(err, "auth")
	}
	if !hasAuth {
		return CompactionResult{}, newAgentHarnessError("auth", "No auth available for compaction")
	}

	branchEntries, err := h.session.Branch(nil)
	if err != nil {
		return CompactionResult{}, normalizeHarnessError(err, "session")
	}
	preparation, err := PrepareCompaction(branchEntries, DefaultCompactionSettings)
	if err != nil {
		return CompactionResult{}, normalizeHarnessError(err, "compaction")
	}
	if preparation == nil {
		return CompactionResult{}, newAgentHarnessError("compaction", "Nothing to compact")
	}

	hookResult, err := h.emitHook(runCtx, AgentHarnessEvent{
		Type:               "session_before_compact",
		Preparation:        preparation,
		BranchEntries:      cloneEntries(branchEntries),
		CustomInstructions: customInstructions,
	})
	if err != nil {
		return CompactionResult{}, err
	}
	if hookResult != nil && hookResult.Cancel {
		return CompactionResult{}, newAgentHarnessError("compaction", "Compaction cancelled")
	}

	fromHook := hookResult != nil && hookResult.HasCompaction
	var result CompactionResult
	if fromHook {
		result = hookResult.Compaction
	} else {
		result, err = CompactWithOptions(runCtx, *preparation, model, CompactOptions{
			APIKey:             auth.APIKey,
			ThinkingLevel:      thinkingLevel,
			CustomInstructions: customInstructions,
		})
		if err != nil {
			return CompactionResult{}, normalizeHarnessError(err, "compaction")
		}
	}

	entryID, err := h.session.AppendCompactionWithOptions(result.Summary, result.FirstKeptEntryID, result.TokensBefore, SessionEntryOptions{Details: result.Details, FromHook: fromHook})
	if err != nil {
		return CompactionResult{}, normalizeHarnessError(err, "session")
	}
	if entry, ok := h.session.Entry(entryID); ok && entry.Type == "compaction" {
		if err := h.emitOwn(runCtx, AgentHarnessEvent{Type: "session_compact", CompactionEntry: entry, FromHook: fromHook}); err != nil {
			return CompactionResult{}, err
		}
	}
	return result, nil
}

func (h *AgentHarness) NavigateTree(ctx context.Context, targetID string, options NavigateTreeOptions) (NavigateTreeResult, error) {
	runCtx, finish, err := h.startExclusivePhase(ctx, AgentHarnessPhaseBranchSummary, "navigateTree() requires idle harness")
	if err != nil {
		return NavigateTreeResult{}, err
	}
	defer finish()

	oldLeafID, err := h.session.LeafID()
	if err != nil {
		return NavigateTreeResult{}, normalizeHarnessError(err, "session")
	}
	if oldLeafID != nil && *oldLeafID == targetID {
		return NavigateTreeResult{}, nil
	}
	targetEntry, ok := h.session.Entry(targetID)
	if !ok {
		return NavigateTreeResult{}, newAgentHarnessError("invalid_argument", "Entry %s not found", targetID)
	}
	collectResult, err := CollectEntriesForBranchSummary(h.session, oldLeafID, targetID)
	if err != nil {
		return NavigateTreeResult{}, normalizeHarnessError(err, "branch_summary")
	}

	hookResult, err := h.emitHook(runCtx, AgentHarnessEvent{
		Type:               "session_before_tree",
		TargetID:           targetID,
		OldLeafID:          cloneStringPtr(oldLeafID),
		CommonAncestorID:   cloneStringPtr(collectResult.CommonAncestorID),
		BranchEntries:      cloneEntries(collectResult.Entries),
		CustomInstructions: options.CustomInstructions,
	})
	if err != nil {
		return NavigateTreeResult{}, err
	}
	if hookResult != nil && hookResult.Cancel {
		return NavigateTreeResult{Cancelled: true}, nil
	}

	var summaryText string
	var summaryDetails map[string]any
	fromHook := hookResult != nil && hookResult.HasBranchSummary
	customInstructions := options.CustomInstructions
	replaceInstructions := options.ReplaceInstructions
	if hookResult != nil {
		if hookResult.HasCustomInstructions {
			customInstructions = hookResult.CustomInstructions
		}
		if hookResult.HasReplaceInstructions {
			replaceInstructions = hookResult.ReplaceInstructions
		}
	}
	if fromHook {
		summaryText = hookResult.BranchSummary.Summary
		summaryDetails = map[string]any{
			"readFiles":     append([]string{}, hookResult.BranchSummary.ReadFiles...),
			"modifiedFiles": append([]string{}, hookResult.BranchSummary.ModifiedFiles...),
		}
	} else if options.Summarize && len(collectResult.Entries) > 0 {
		h.mu.Lock()
		model := h.model
		h.mu.Unlock()
		auth, hasAuth, err := h.resolveAuth(runCtx, model)
		if err != nil {
			return NavigateTreeResult{}, normalizeHarnessError(err, "auth")
		}
		if !hasAuth {
			return NavigateTreeResult{}, newAgentHarnessError("auth", "No auth available for branch summary")
		}
		summary, err := GenerateBranchSummary(runCtx, collectResult.Entries, model, BranchSummaryOptions{
			APIKey:              auth.APIKey,
			CustomInstructions:  customInstructions,
			ReplaceInstructions: replaceInstructions,
		})
		if err != nil {
			if runCtx.Err() != nil {
				return NavigateTreeResult{Cancelled: true}, nil
			}
			return NavigateTreeResult{}, normalizeHarnessError(err, "branch_summary")
		}
		summaryText = summary.Summary
		summaryDetails = map[string]any{
			"readFiles":     append([]string{}, summary.ReadFiles...),
			"modifiedFiles": append([]string{}, summary.ModifiedFiles...),
		}
	}

	newLeafID := &targetID
	editorText := ""
	if targetEntry.Type == "message" && targetEntry.Message.Role == llm.RoleUser {
		newLeafID = cloneStringPtr(targetEntry.ParentID)
		editorText = textFromEntry(targetEntry)
	} else if targetEntry.Type == "custom_message" {
		newLeafID = cloneStringPtr(targetEntry.ParentID)
		editorText = textFromEntry(targetEntry)
	}

	summaryID, err := h.session.MoveToWithOptions(newLeafID, summaryText, SessionEntryOptions{Details: summaryDetails, FromHook: fromHook})
	if err != nil {
		return NavigateTreeResult{}, normalizeHarnessError(err, "session")
	}
	var summaryEntry *Entry
	if summaryID != nil {
		if entry, ok := h.session.Entry(*summaryID); ok && entry.Type == "branch_summary" {
			summaryEntry = &entry
		}
	}
	currentLeafID, err := h.session.LeafID()
	if err != nil {
		return NavigateTreeResult{}, normalizeHarnessError(err, "session")
	}
	if err := h.emitOwn(runCtx, AgentHarnessEvent{
		Type:         "session_tree",
		TargetID:     targetID,
		OldLeafID:    cloneStringPtr(oldLeafID),
		NewLeafID:    cloneStringPtr(currentLeafID),
		SummaryEntry: summaryEntry,
		FromHook:     fromHook,
	}); err != nil {
		return NavigateTreeResult{}, err
	}
	return NavigateTreeResult{EditorText: editorText, SummaryEntry: summaryEntry}, nil
}

func (h *AgentHarness) Abort(ctx context.Context) (clearedSteer, clearedFollowUp []llm.Message, err error) {
	h.mu.Lock()
	clearedSteer = cloneMessages(h.steerQueue)
	clearedFollowUp = cloneMessages(h.followUpQueue)
	h.steerQueue = nil
	h.followUpQueue = nil
	cancel := h.cancel
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	var firstErr error
	if err := h.emitQueueUpdate(ctx); err != nil {
		firstErr = err
	}
	if err := h.WaitForIdle(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := h.emitOwn(ctx, AgentHarnessEvent{Type: "abort", ClearedSteer: clearedSteer, ClearedFollowUp: clearedFollowUp}); err != nil && firstErr == nil {
		firstErr = err
	}
	return clearedSteer, clearedFollowUp, firstErr
}

func (h *AgentHarness) WaitForIdle(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	h.mu.Lock()
	done := h.done
	h.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *AgentHarness) addHandler(eventType string, handler AgentHarnessHandler) func() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextHandlerID++
	id := h.nextHandlerID
	h.handlers[eventType] = append(h.handlers[eventType], handlerEntry{id: id, fn: handler})
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		entries := h.handlers[eventType]
		for i, entry := range entries {
			if entry.id == id {
				h.handlers[eventType] = append(entries[:i], entries[i+1:]...)
				return
			}
		}
	}
}

func (h *AgentHarness) startTurn(parent context.Context) (context.Context, func(), error) {
	return h.startExclusivePhase(parent, AgentHarnessPhaseTurn, "AgentHarness is busy")
}

func (h *AgentHarness) startExclusivePhase(parent context.Context, phase, busyMessage string) (context.Context, func(), error) {
	if parent == nil {
		parent = context.Background()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.phase != AgentHarnessPhaseIdle {
		return nil, nil, newAgentHarnessError("busy", "%s", busyMessage)
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	h.phase = phase
	h.cancel = cancel
	h.done = done
	finish := func() {
		h.mu.Lock()
		cancel := h.cancel
		h.cancel = nil
		h.done = nil
		if h.phase == phase {
			h.phase = AgentHarnessPhaseIdle
		}
		h.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		close(done)
	}
	return ctx, finish, nil
}

func (h *AgentHarness) setPhase(phase string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.phase = phase
}

func (h *AgentHarness) createTurnState(ctx context.Context) (agentHarnessTurnState, error) {
	sessionContext, err := h.session.BuildContext()
	if err != nil {
		return agentHarnessTurnState{}, err
	}
	metadata := h.session.Metadata()

	h.mu.Lock()
	resources := h.resources.Clone()
	streamOptions := cloneHarnessStreamOptions(h.streamOptions)
	model := h.model
	thinkingLevel := h.thinkingLevel
	systemPrompt := h.systemPrompt
	buildSystemPrompt := h.buildSystemPrompt
	activeTools := h.activeToolsLocked()
	tools := h.toolsLocked()
	env := h.Env
	session := h.session
	h.mu.Unlock()

	resolvedSystemPrompt := "You are a helpful assistant."
	if buildSystemPrompt != nil {
		resolved, err := buildSystemPrompt(ctx, SystemPromptContext{
			Env:           env,
			Session:       session,
			Model:         model,
			ThinkingLevel: thinkingLevel,
			ActiveTools:   activeTools,
			Resources:     resources.Clone(),
		})
		if err != nil {
			return agentHarnessTurnState{}, err
		}
		resolvedSystemPrompt = resolved
	} else if systemPrompt != "" {
		resolvedSystemPrompt = systemPrompt
	}
	return agentHarnessTurnState{
		Messages:      cloneMessages(sessionContext.Messages),
		Resources:     resources,
		StreamOptions: streamOptions,
		SessionID:     metadata.ID,
		SystemPrompt:  resolvedSystemPrompt,
		Model:         model,
		ThinkingLevel: thinkingLevel,
		Tools:         tools,
		ActiveTools:   activeTools,
	}, nil
}

func (h *AgentHarness) executeTurn(ctx context.Context, turnState agentHarnessTurnState, text string, images ...llm.ContentPart) (llm.Message, error) {
	activeTurnState := turnState
	messages := []llm.Message{createHarnessUserMessage(text, images...)}
	queuedMessages, err := h.drainNextTurnQueue(ctx)
	if err != nil {
		return llm.Message{}, err
	}
	if len(queuedMessages) > 0 {
		messages = append(queuedMessages, messages[0])
	}

	beforeResult, err := h.emitHook(ctx, AgentHarnessEvent{
		Type:         "before_agent_start",
		Prompt:       text,
		Images:       append([]llm.ContentPart{}, images...),
		SystemPrompt: turnState.SystemPrompt,
		Resources:    turnState.Resources.Clone(),
	})
	if err != nil {
		return h.emitRunFailure(ctx, activeTurnState.Model, err)
	}
	systemPrompt := turnState.SystemPrompt
	if beforeResult != nil {
		if beforeResult.HasMessages {
			messages = append(messages, beforeResult.Messages...)
		}
		if beforeResult.HasSystemPrompt {
			systemPrompt = beforeResult.SystemPrompt
		}
	}

	getTurnState := func() agentHarnessTurnState {
		return activeTurnState
	}
	setTurnState := func(next agentHarnessTurnState) {
		activeTurnState = next
	}
	newMessages, err := core.RunAgentLoop(
		messages,
		h.createContext(turnState, systemPrompt),
		h.createLoopConfig(ctx, getTurnState, setTurnState),
		func(event core.AgentEvent) error { return h.handleAgentEvent(ctx, event) },
		ctx,
		h.createStreamFn(ctx, getTurnState),
	)
	if err != nil {
		return h.emitRunFailure(ctx, activeTurnState.Model, err)
	}
	if err := h.flushPendingSessionWrites(); err != nil {
		return llm.Message{}, err
	}
	for i := len(newMessages) - 1; i >= 0; i-- {
		if newMessages[i].Role == llm.RoleAssistant {
			return newMessages[i], nil
		}
	}
	return llm.Message{}, newAgentHarnessError("invalid_state", "AgentHarness prompt completed without an assistant message")
}

func (h *AgentHarness) createContext(turnState agentHarnessTurnState, systemPrompt string) core.AgentContext {
	return core.AgentContext{
		SystemPrompt: systemPrompt,
		Messages:     cloneMessages(turnState.Messages),
		Tools:        append([]core.AgentTool{}, turnState.ActiveTools...),
	}
}

func (h *AgentHarness) createLoopConfig(ctx context.Context, getTurnState func() agentHarnessTurnState, setTurnState func(agentHarnessTurnState)) core.AgentLoopConfig {
	turnState := getTurnState()
	reasoning := ""
	if turnState.ThinkingLevel != "off" {
		reasoning = turnState.ThinkingLevel
	}
	return core.AgentLoopConfig{
		Model:         turnState.Model,
		ToolExecution: h.toolExecution,
		SimpleStreamOptions: llm.SimpleStreamOptions{
			Context:   ctx,
			Reasoning: reasoning,
			SessionID: turnState.SessionID,
		},
		TransformContext: func(ctx context.Context, messages []llm.Message) ([]llm.Message, error) {
			result, err := h.emitHook(ctx, AgentHarnessEvent{Type: "context", Messages: cloneMessages(messages)})
			if err != nil {
				return nil, err
			}
			if result != nil && result.HasMessages {
				return cloneMessages(result.Messages), nil
			}
			return messages, nil
		},
		BeforeToolCall: func(ctx context.Context, toolContext core.BeforeToolCallContext) (core.BeforeToolCallResult, error) {
			result, err := h.emitHook(ctx, AgentHarnessEvent{
				Type:       "tool_call",
				ToolCallID: toolContext.ToolCall.ID,
				ToolName:   toolContext.ToolCall.Name,
				Input:      cloneAnyMap(toolContext.Args),
			})
			if err != nil || result == nil {
				return core.BeforeToolCallResult{}, err
			}
			return core.BeforeToolCallResult{Block: result.Block, Reason: result.Reason}, nil
		},
		AfterToolCall: func(ctx context.Context, toolContext core.AfterToolCallContext) (core.AfterToolCallResult, error) {
			result, err := h.emitHook(ctx, AgentHarnessEvent{
				Type:       "tool_result",
				ToolCallID: toolContext.ToolCall.ID,
				ToolName:   toolContext.ToolCall.Name,
				Input:      cloneAnyMap(toolContext.Args),
				Content:    append([]llm.ContentPart{}, toolContext.Result.Content...),
				Details:    toolContext.Result.Details,
				IsError:    toolContext.IsError,
			})
			if err != nil || result == nil {
				return core.AfterToolCallResult{}, err
			}
			return core.AfterToolCallResult{
				Content:      append([]llm.ContentPart{}, result.Content...),
				HasContent:   result.HasContent,
				Details:      result.Details,
				HasDetails:   result.HasDetails,
				IsError:      result.IsError,
				HasIsError:   result.HasIsError,
				Terminate:    result.Terminate,
				HasTerminate: result.HasTerminate,
			}, nil
		},
		PrepareNextTurn: func(core.PrepareNextTurnContext) (core.AgentLoopTurnUpdate, bool, error) {
			if err := h.flushPendingSessionWrites(); err != nil {
				return core.AgentLoopTurnUpdate{}, false, err
			}
			nextTurnState, err := h.createTurnState(ctx)
			if err != nil {
				return core.AgentLoopTurnUpdate{}, false, err
			}
			setTurnState(nextTurnState)
			contextValue := h.createContext(nextTurnState, nextTurnState.SystemPrompt)
			return core.AgentLoopTurnUpdate{
				Context:       &contextValue,
				Model:         &nextTurnState.Model,
				ThinkingLevel: &nextTurnState.ThinkingLevel,
			}, true, nil
		},
		GetSteeringMessages: func() ([]llm.Message, error) {
			return h.drainQueuedMessages(ctx, "steer")
		},
		GetFollowUpMessages: func() ([]llm.Message, error) {
			return h.drainQueuedMessages(ctx, "follow_up")
		},
	}
}

func (h *AgentHarness) createStreamFn(ctx context.Context, getTurnState func() agentHarnessTurnState) core.StreamFn {
	return func(model llm.Model, llmContext llm.Context, streamOptions llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
		turnState := getTurnState()
		auth, hasAuth, err := h.resolveAuth(ctx, model)
		if err != nil {
			return nil, err
		}
		snapshotOptions := cloneHarnessStreamOptions(turnState.StreamOptions)
		if hasAuth {
			snapshotOptions.Headers = mergeHeaders(snapshotOptions.Headers, auth.Headers)
		}
		requestOptions, err := h.emitBeforeProviderRequest(ctx, model, turnState.SessionID, snapshotOptions)
		if err != nil {
			return nil, err
		}
		options := streamOptions
		options.Context = ctx
		options.SessionID = turnState.SessionID
		options.APIKey = auth.APIKey
		options.Transport = requestOptions.Transport
		options.TimeoutMillis = requestOptions.TimeoutMillis
		options.MaxRetries = requestOptions.MaxRetries
		options.MaxRetryDelayMs = requestOptions.MaxRetryDelayMs
		options.Headers = cloneStringMap(requestOptions.Headers)
		options.Metadata = cloneAnyMap(requestOptions.Metadata)
		options.CacheRetention = requestOptions.CacheRetention
		options.OnPayload = func(payload any, payloadModel llm.Model) (any, bool, error) {
			return h.emitBeforeProviderPayload(ctx, payloadModel, payload)
		}
		options.OnResponseStatus = func(status int, headers map[string]string, responseModel llm.Model) error {
			return h.emitOwn(ctx, AgentHarnessEvent{Type: "after_provider_response", Status: status, Headers: cloneStringMap(headers), Model: responseModel})
		}
		return llm.StreamSimple(model, llmContext, options)
	}
}

func (h *AgentHarness) resolveAuth(ctx context.Context, model llm.Model) (AgentHarnessAuth, bool, error) {
	h.mu.Lock()
	getAuth := h.getAPIKeyAndHeaders
	h.mu.Unlock()
	if getAuth == nil {
		return AgentHarnessAuth{}, false, nil
	}
	auth, ok, err := getAuth(ctx, model)
	if err != nil {
		return AgentHarnessAuth{}, false, err
	}
	auth.Headers = cloneStringMap(auth.Headers)
	return auth, ok, nil
}

func (h *AgentHarness) handleAgentEvent(ctx context.Context, event core.AgentEvent) error {
	harnessEvent := agentEventToHarnessEvent(event)
	if event.Type == "message_end" {
		if _, err := h.session.AppendMessage(event.Message); err != nil {
			return normalizeHarnessError(err, "session")
		}
		return h.emitAny(ctx, harnessEvent)
	}
	if event.Type == "turn_end" {
		eventErr := h.emitAny(ctx, harnessEvent)
		h.mu.Lock()
		hadPendingMutations := len(h.pendingSessionWrites) > 0
		h.mu.Unlock()
		if err := h.flushPendingSessionWrites(); err != nil {
			return err
		}
		if eventErr != nil {
			return eventErr
		}
		return h.emitOwn(ctx, AgentHarnessEvent{Type: "save_point", HadPendingMutations: hadPendingMutations})
	}
	if event.Type == "agent_end" {
		if err := h.flushPendingSessionWrites(); err != nil {
			return err
		}
		h.setPhase(AgentHarnessPhaseIdle)
		if err := h.emitAny(ctx, harnessEvent); err != nil {
			return err
		}
		h.mu.Lock()
		nextTurnCount := len(h.nextTurnQueue)
		h.mu.Unlock()
		return h.emitOwn(ctx, AgentHarnessEvent{Type: "settled", NextTurnCount: nextTurnCount})
	}
	return h.emitAny(ctx, harnessEvent)
}

func (h *AgentHarness) emitRunFailure(ctx context.Context, model llm.Model, cause error) (llm.Message, error) {
	message := llm.AssistantErrorMessage(cause.Error(), model, ctx.Err() != nil)
	events := []core.AgentEvent{
		{Type: "message_start", Message: message},
		{Type: "message_end", Message: message},
		{Type: "turn_end", Message: message},
		{Type: "agent_end", Messages: []llm.Message{message}},
	}
	for _, event := range events {
		if err := h.handleAgentEvent(ctx, event); err != nil {
			return llm.Message{}, newAgentHarnessError("unknown", "agent run failed and failure reporting failed: %v; reporting error: %v", cause, err)
		}
	}
	return message, nil
}

func (h *AgentHarness) emitOwn(ctx context.Context, event AgentHarnessEvent) error {
	return h.emitHandlers(ctx, harnessSubscriberEventType, event)
}

func (h *AgentHarness) emitAny(ctx context.Context, event AgentHarnessEvent) error {
	return h.emitHandlers(ctx, harnessSubscriberEventType, event)
}

func (h *AgentHarness) emitHook(ctx context.Context, event AgentHarnessEvent) (*AgentHarnessHookResult, error) {
	handlers := h.handlersSnapshot(event.Type)
	var last *AgentHarnessHookResult
	for _, handler := range handlers {
		result, err := handler(ctx, event)
		if err != nil {
			return nil, normalizeHarnessError(err, "hook")
		}
		if result != nil {
			last = result
		}
	}
	return last, nil
}

func (h *AgentHarness) emitHandlers(ctx context.Context, eventType string, event AgentHarnessEvent) error {
	for _, handler := range h.handlersSnapshot(eventType) {
		if _, err := handler(ctx, event); err != nil {
			return normalizeHarnessError(err, "hook")
		}
	}
	return nil
}

func (h *AgentHarness) handlersSnapshot(eventType string) []AgentHarnessHandler {
	h.mu.Lock()
	defer h.mu.Unlock()
	entries := h.handlers[eventType]
	handlers := make([]AgentHarnessHandler, 0, len(entries))
	for _, entry := range entries {
		handlers = append(handlers, entry.fn)
	}
	return handlers
}

func (h *AgentHarness) emitBeforeProviderRequest(ctx context.Context, model llm.Model, sessionID string, options AgentHarnessStreamOptions) (AgentHarnessStreamOptions, error) {
	current := cloneHarnessStreamOptions(options)
	for _, handler := range h.handlersSnapshot("before_provider_request") {
		result, err := handler(ctx, AgentHarnessEvent{
			Type:          "before_provider_request",
			Model:         model,
			SessionID:     sessionID,
			StreamOptions: cloneHarnessStreamOptions(current),
		})
		if err != nil {
			return AgentHarnessStreamOptions{}, normalizeHarnessError(err, "hook")
		}
		if result != nil && result.StreamOptions != nil {
			current = ApplyStreamOptionsPatch(current, *result.StreamOptions)
		}
	}
	return current, nil
}

func (h *AgentHarness) emitBeforeProviderPayload(ctx context.Context, model llm.Model, payload any) (any, bool, error) {
	current := payload
	changed := false
	for _, handler := range h.handlersSnapshot("before_provider_payload") {
		result, err := handler(ctx, AgentHarnessEvent{Type: "before_provider_payload", Model: model, Payload: current})
		if err != nil {
			return nil, false, normalizeHarnessError(err, "hook")
		}
		if result != nil && result.HasPayload {
			current = result.Payload
			changed = true
		}
	}
	return current, changed, nil
}

func (h *AgentHarness) emitQueueUpdate(ctx context.Context) error {
	h.mu.Lock()
	event := AgentHarnessEvent{
		Type:     "queue_update",
		Steer:    cloneMessages(h.steerQueue),
		FollowUp: cloneMessages(h.followUpQueue),
		NextTurn: cloneMessages(h.nextTurnQueue),
	}
	h.mu.Unlock()
	return h.emitOwn(ctx, event)
}

func (h *AgentHarness) drainQueuedMessages(ctx context.Context, queueName string) ([]llm.Message, error) {
	h.mu.Lock()
	var messages []llm.Message
	switch queueName {
	case "steer":
		messages, h.steerQueue = drainHarnessQueue(h.steerQueue, h.steeringQueueMode)
	case "follow_up":
		messages, h.followUpQueue = drainHarnessQueue(h.followUpQueue, h.followUpQueueMode)
	}
	h.mu.Unlock()
	if len(messages) == 0 {
		return nil, nil
	}
	if err := h.emitQueueUpdate(ctx); err != nil {
		h.mu.Lock()
		switch queueName {
		case "steer":
			h.steerQueue = append(cloneMessages(messages), h.steerQueue...)
		case "follow_up":
			h.followUpQueue = append(cloneMessages(messages), h.followUpQueue...)
		}
		h.mu.Unlock()
		return nil, err
	}
	return messages, nil
}

func (h *AgentHarness) drainNextTurnQueue(ctx context.Context) ([]llm.Message, error) {
	h.mu.Lock()
	messages := cloneMessages(h.nextTurnQueue)
	h.nextTurnQueue = nil
	h.mu.Unlock()
	if len(messages) == 0 {
		return nil, nil
	}
	if err := h.emitQueueUpdate(ctx); err != nil {
		h.mu.Lock()
		h.nextTurnQueue = append(messages, h.nextTurnQueue...)
		h.mu.Unlock()
		return nil, err
	}
	return messages, nil
}

func (h *AgentHarness) flushPendingSessionWrites() error {
	for {
		h.mu.Lock()
		if len(h.pendingSessionWrites) == 0 {
			h.mu.Unlock()
			return nil
		}
		write := h.pendingSessionWrites[0]
		h.mu.Unlock()

		var err error
		switch write.kind {
		case "message":
			_, err = h.session.AppendMessage(write.message)
		case "model_change":
			_, err = h.session.AppendModelChange(write.provider, write.modelID)
		case "thinking_level_change":
			_, err = h.session.AppendThinkingLevelChange(write.thinkingLevel)
		case "custom_message":
			_, err = h.session.AppendCustomMessageEntry(write.customType, write.content, write.display, write.details)
		case "label":
			_, err = h.session.AppendLabel(write.targetID, write.label)
		case "session_info":
			_, err = h.session.AppendSessionName(write.name)
		}
		if err != nil {
			return normalizeHarnessError(err, "session")
		}

		h.mu.Lock()
		if len(h.pendingSessionWrites) > 0 {
			h.pendingSessionWrites = h.pendingSessionWrites[1:]
		}
		h.mu.Unlock()
	}
}

func (h *AgentHarness) activeToolsLocked() []core.AgentTool {
	activeTools := make([]core.AgentTool, 0, len(h.activeToolNames))
	for _, name := range h.activeToolNames {
		if tool, ok := h.tools[name]; ok {
			activeTools = append(activeTools, tool)
		}
	}
	return activeTools
}

func (h *AgentHarness) toolsLocked() []core.AgentTool {
	tools := make([]core.AgentTool, 0, len(h.tools))
	for _, tool := range h.tools {
		tools = append(tools, tool)
	}
	return tools
}

func ApplyStreamOptionsPatch(base AgentHarnessStreamOptions, patch AgentHarnessStreamOptionsPatch) AgentHarnessStreamOptions {
	result := cloneHarnessStreamOptions(base)
	if patch.Transport.Set {
		result.Transport = patch.Transport.Value
	}
	if patch.TimeoutMillis.Set {
		result.TimeoutMillis = patch.TimeoutMillis.Value
	}
	if patch.MaxRetries.Set {
		result.MaxRetries = patch.MaxRetries.Value
	}
	if patch.MaxRetryDelayMs.Set {
		result.MaxRetryDelayMs = patch.MaxRetryDelayMs.Value
	}
	if patch.CacheRetention.Set {
		result.CacheRetention = patch.CacheRetention.Value
	}
	if patch.ClearHeaders {
		result.Headers = nil
	} else if patch.Headers != nil {
		headers := cloneStringMap(result.Headers)
		if headers == nil {
			headers = map[string]string{}
		}
		for key, value := range patch.Headers {
			if value == nil {
				delete(headers, key)
				continue
			}
			headers[key] = *value
		}
		if len(headers) == 0 {
			result.Headers = nil
		} else {
			result.Headers = headers
		}
	}
	if patch.ClearMetadata {
		result.Metadata = nil
	} else if patch.Metadata != nil {
		metadata := cloneAnyMap(result.Metadata)
		if metadata == nil {
			metadata = map[string]any{}
		}
		for key, value := range patch.Metadata {
			if !value.Set {
				delete(metadata, key)
				continue
			}
			metadata[key] = value.Value
		}
		if len(metadata) == 0 {
			result.Metadata = nil
		} else {
			result.Metadata = metadata
		}
	}
	return result
}

func createHarnessUserMessage(text string, images ...llm.ContentPart) llm.Message {
	content := []llm.ContentPart{llm.Text(text)}
	content = append(content, images...)
	return llm.Message{Role: llm.RoleUser, Content: content, Timestamp: llm.NowMillis()}
}

func textFromEntry(entry Entry) string {
	if entry.Type == "custom_message" {
		return fmt.Sprint(entry.Content)
	}
	var out strings.Builder
	for _, part := range entry.Message.Content {
		if part.Type == llm.ContentText {
			out.WriteString(part.Text)
		}
	}
	return out.String()
}

func drainHarnessQueue(queue []llm.Message, mode string) ([]llm.Message, []llm.Message) {
	if len(queue) == 0 {
		return nil, queue
	}
	if mode == core.QueueAll {
		return cloneMessages(queue), nil
	}
	return cloneMessages(queue[:1]), cloneMessages(queue[1:])
}

func agentEventToHarnessEvent(event core.AgentEvent) AgentHarnessEvent {
	return AgentHarnessEvent{
		Type:        event.Type,
		Message:     event.Message,
		Messages:    cloneMessages(event.Messages),
		ToolResults: cloneMessages(event.ToolResults),
		ToolCallID:  event.ToolCallID,
		ToolName:    event.ToolName,
		Input:       cloneAnyMap(event.Args),
		IsError:     event.IsError,
		AgentEvent:  event,
	}
}

func validateToolNames(names []string, tools map[string]core.AgentTool) error {
	var missing []string
	for _, name := range names {
		if _, ok := tools[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return newAgentHarnessError("invalid_argument", "unknown tool(s): %v", missing)
	}
	return nil
}

func cloneHarnessStreamOptions(options AgentHarnessStreamOptions) AgentHarnessStreamOptions {
	options.Headers = cloneStringMap(options.Headers)
	options.Metadata = cloneAnyMap(options.Metadata)
	return options
}

func mergeHeaders(headers ...map[string]string) map[string]string {
	merged := map[string]string{}
	hasHeaders := false
	for _, entry := range headers {
		for key, value := range entry {
			merged[key] = value
			hasHeaders = true
		}
	}
	if !hasHeaders {
		return nil
	}
	return merged
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	clone := make(map[string]any, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneMessages(messages []llm.Message) []llm.Message {
	if messages == nil {
		return nil
	}
	return append([]llm.Message{}, messages...)
}

func cloneEntries(entries []Entry) []Entry {
	if entries == nil {
		return nil
	}
	return append([]Entry{}, entries...)
}
