package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	RPCCommandPrompt               = "prompt"
	RPCCommandSteer                = "steer"
	RPCCommandFollowUp             = "follow_up"
	RPCCommandAbort                = "abort"
	RPCCommandNewSession           = "new_session"
	RPCCommandGetState             = "get_state"
	RPCCommandSetModel             = "set_model"
	RPCCommandCycleModel           = "cycle_model"
	RPCCommandSetThinkingLevel     = "set_thinking_level"
	RPCCommandCycleThinkingLevel   = "cycle_thinking_level"
	RPCCommandGetAvailableModels   = "get_available_models"
	RPCCommandSetSteeringMode      = "set_steering_mode"
	RPCCommandSetFollowUpMode      = "set_follow_up_mode"
	RPCCommandCompact              = "compact"
	RPCCommandSetAutoCompaction    = "set_auto_compaction"
	RPCCommandSetAutoRetry         = "set_auto_retry"
	RPCCommandAbortRetry           = "abort_retry"
	RPCCommandBash                 = "bash"
	RPCCommandAbortBash            = "abort_bash"
	RPCCommandGetSessionStats      = "get_session_stats"
	RPCCommandExportHTML           = "export_html"
	RPCCommandSwitchSession        = "switch_session"
	RPCCommandFork                 = "fork"
	RPCCommandGetForkMessages      = "get_fork_messages"
	RPCCommandGetLastAssistantText = "get_last_assistant_text"
	RPCCommandSetSessionName       = "set_session_name"
	RPCCommandGetMessages          = "get_messages"
	RPCCommandGetCommands          = "get_commands"
)

var errNothingToExport = errors.New("Nothing to export yet - start a conversation first")

type RPCSessionHost struct {
	Session               *AgentSession
	Settings              *SettingsManager
	SteeringMode          string
	FollowUpMode          string
	AutoCompactionEnabled bool
	AvailableModels       []llm.Model
	ScopedModels          []RPCScopedModel
	ProviderAuthStatus    AuthStatusResolver
	PromptPreflight       func(RPCCommand) error
	ViewTreeHost          *ViewTreeHost
	TUIEditor             TUIEditorHost
	TUIDialog             TUIDialogHost
	TUITitle              TUITitleHost
	TUIWorking            TUIWorkingHost
	TUIThinkingLabel      TUIThinkingLabelHost
	TUIStatus             TUIStatusHost
	TUITheme              TUIThemeHost
	TUIToolExpansion      TUIToolExpansionHost
	ProcessExecutor       HostProcessExecutor
	PolicyRequester       HostPolicyRequester
	ReloadSession         func() error
	sessionMu             sync.RWMutex
	sessionReplaceHooks   []rpcSessionReplaceHookRegistration
	nextSessionHookID     uint64
	childMu               sync.Mutex
	childAgents           map[string]*AgentSession
}

type rpcSessionReplaceHookRegistration struct {
	id       uint64
	listener func(*AgentSession)
}

type RPCScopedModel struct {
	Model         llm.Model
	ThinkingLevel string
}

type RPCSessionState struct {
	Model                 *llm.Model `json:"model,omitempty"`
	ThinkingLevel         string     `json:"thinkingLevel"`
	IsStreaming           bool       `json:"isStreaming"`
	IsCompacting          bool       `json:"isCompacting"`
	SteeringMode          string     `json:"steeringMode"`
	FollowUpMode          string     `json:"followUpMode"`
	SessionFile           string     `json:"sessionFile,omitempty"`
	SessionID             string     `json:"sessionId"`
	SessionName           string     `json:"sessionName,omitempty"`
	AutoCompactionEnabled bool       `json:"autoCompactionEnabled"`
	MessageCount          int        `json:"messageCount"`
	PendingMessageCount   int        `json:"pendingMessageCount"`
}

type RPCAvailableModelsResult struct {
	Models []llm.Model `json:"models"`
}

type RPCCycleModelResult struct {
	Model         llm.Model `json:"model"`
	ThinkingLevel string    `json:"thinkingLevel"`
	IsScoped      bool      `json:"isScoped"`
}

type RPCSessionStats struct {
	SessionFile       string             `json:"sessionFile,omitempty"`
	SessionID         string             `json:"sessionId"`
	UserMessages      int                `json:"userMessages"`
	AssistantMessages int                `json:"assistantMessages"`
	ToolCalls         int                `json:"toolCalls"`
	ToolResults       int                `json:"toolResults"`
	TotalMessages     int                `json:"totalMessages"`
	Tokens            RPCSessionTokens   `json:"tokens"`
	Cost              float64            `json:"cost"`
	ContextUsage      *AgentContextUsage `json:"contextUsage,omitempty"`
}

type RPCSessionTokens struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cacheRead"`
	CacheWrite int `json:"cacheWrite"`
	Total      int `json:"total"`
}

type RPCExportHTMLResult struct {
	Path string `json:"path"`
}

type RPCLastAssistantTextResult struct {
	Text *string `json:"text"`
}

type RPCForkResult struct {
	Text      string `json:"text"`
	Cancelled bool   `json:"cancelled"`
}

type RPCForkMessagesResult struct {
	Messages []AgentSessionForkMessage `json:"messages"`
}

type RPCMessagesResult struct {
	Messages []llm.Message `json:"messages"`
}

type RPCSlashCommand struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	ArgumentHint string `json:"argumentHint,omitempty"`
	Source       string `json:"source"`
	SourceInfo   any    `json:"sourceInfo"`
}

type RPCCommandsResult struct {
	Commands []RPCSlashCommand `json:"commands"`
}

func NewRPCSessionHost(session *AgentSession) *RPCSessionHost {
	host := &RPCSessionHost{
		Session:               session,
		SteeringMode:          "one-at-a-time",
		FollowUpMode:          "one-at-a-time",
		AutoCompactionEnabled: true,
		ViewTreeHost:          NewViewTreeHost(),
	}
	if session != nil {
		host.Settings = session.SettingsManager
		host.AutoCompactionEnabled = session.CompactionSettings.Enabled
		if session.SteeringMode != "" {
			host.SteeringMode = session.SteeringMode
		}
		if session.FollowUpMode != "" {
			host.FollowUpMode = session.FollowUpMode
		}
		host.ScopedModels = rpcScopedModelsFromSession(session.ScopedModels)
	}
	return host
}

func (h *RPCSessionHost) sessionSnapshot() *AgentSession {
	if h == nil {
		return nil
	}
	h.sessionMu.RLock()
	defer h.sessionMu.RUnlock()
	return h.Session
}

func rpcScopedModelsFromSession(scopedModels []ScopedModel) []RPCScopedModel {
	if len(scopedModels) == 0 {
		return nil
	}
	result := make([]RPCScopedModel, 0, len(scopedModels))
	for _, scoped := range scopedModels {
		result = append(result, RPCScopedModel{
			Model:         scoped.Model,
			ThinkingLevel: string(scoped.ThinkingLevel),
		})
	}
	return result
}

func (h *RPCSessionHost) SubscribeEvents(listener func(AgentSessionEvent)) func() {
	if h == nil || listener == nil {
		return func() {}
	}
	session := h.sessionSnapshot()
	if session == nil {
		return func() {}
	}
	var subscriptionMu sync.Mutex
	active := true
	unsubscribeSession := session.Subscribe(listener)
	unsubscribeReplace := h.OnSessionReplaced(func(session *AgentSession) {
		subscriptionMu.Lock()
		if !active {
			subscriptionMu.Unlock()
			return
		}
		previousUnsubscribe := unsubscribeSession
		unsubscribeSession = session.Subscribe(listener)
		subscriptionMu.Unlock()
		previousUnsubscribe()
	})
	return func() {
		unsubscribeReplace()
		subscriptionMu.Lock()
		active = false
		unsubscribe := unsubscribeSession
		unsubscribeSession = func() {}
		subscriptionMu.Unlock()
		unsubscribe()
	}
}

func (h *RPCSessionHost) OnSessionReplaced(listener func(*AgentSession)) func() {
	if h == nil || listener == nil {
		return func() {}
	}
	h.sessionMu.Lock()
	h.nextSessionHookID++
	id := h.nextSessionHookID
	h.sessionReplaceHooks = append(
		h.sessionReplaceHooks,
		rpcSessionReplaceHookRegistration{id: id, listener: listener},
	)
	h.sessionMu.Unlock()
	return func() {
		h.sessionMu.Lock()
		defer h.sessionMu.Unlock()
		for index, registration := range h.sessionReplaceHooks {
			if registration.id != id {
				continue
			}
			h.sessionReplaceHooks = append(h.sessionReplaceHooks[:index], h.sessionReplaceHooks[index+1:]...)
			return
		}
	}
}

func (h *RPCSessionHost) replaceSession(session *AgentSession) {
	if h == nil || session == nil {
		return
	}
	h.sessionMu.Lock()
	h.Session = session
	hooks := make([]func(*AgentSession), 0, len(h.sessionReplaceHooks))
	for _, registration := range h.sessionReplaceHooks {
		if registration.listener != nil {
			hooks = append(hooks, registration.listener)
		}
	}
	h.sessionMu.Unlock()
	for _, listener := range hooks {
		listener(session)
	}
}

func (h *RPCSessionHost) HandleCommand(ctx context.Context, command RPCCommand) RPCResponse {
	result, err := h.handleCommand(ctx, command)
	if err != nil {
		return rpcErrorResponse(command.Type, err)
	}
	return rpcSuccessResponse(command.Type, result)
}

func (h *RPCSessionHost) handleCommand(ctx context.Context, command RPCCommand) (any, error) {
	session := h.sessionSnapshot()
	if session == nil || session.SessionManager == nil {
		return nil, errors.New("RPC session host requires an active session")
	}
	switch command.Type {
	case RPCCommandPrompt:
		if err := h.runPromptPreflight(command); err != nil {
			return nil, err
		}
		return nil, session.PromptWithImages(command.Message, command.Images)
	case RPCCommandSteer:
		return nil, session.SteerWithImages(command.Message, command.Images)
	case RPCCommandFollowUp:
		return nil, session.FollowUpWithImages(command.Message, command.Images)
	case RPCCommandAbort:
		return nil, session.Abort()
	case RPCCommandNewSession:
		if _, err := session.SessionManager.NewSession(NewSessionOptions{ParentSession: command.ParentSession}); err != nil {
			return nil, err
		}
		session.queues.clearPrompts()
		return RPCCloneResult{Cancelled: false}, nil
	case RPCCommandGetState:
		return h.GetState(), nil
	case RPCCommandSetModel:
		return h.SetModel(command.Provider, command.ModelID)
	case RPCCommandCycleModel:
		return h.CycleModel()
	case RPCCommandSetThinkingLevel:
		return nil, h.SetThinkingLevel(command.Level)
	case RPCCommandCycleThinkingLevel:
		level, err := h.CycleThinkingLevel()
		if err != nil {
			return nil, err
		}
		return map[string]string{"level": level}, nil
	case RPCCommandGetAvailableModels:
		return RPCAvailableModelsResult{Models: h.getAvailableModels()}, nil
	case RPCCommandSetSteeringMode:
		return nil, h.SetSteeringMode(command.Mode)
	case RPCCommandSetFollowUpMode:
		return nil, h.SetFollowUpMode(command.Mode)
	case RPCCommandCompact:
		return session.Compact(command.CustomInstructions)
	case RPCCommandSetAutoCompaction:
		return nil, h.SetAutoCompaction(command.Enabled)
	case RPCCommandSetAutoRetry:
		return nil, h.SetAutoRetry(command.Enabled)
	case RPCCommandAbortRetry:
		session.AbortRetry()
		return nil, nil
	case RPCCommandBash:
		return h.Bash(ctx, command.Command)
	case RPCCommandAbortBash:
		session.AbortBash()
		return nil, nil
	case RPCCommandGetSessionStats:
		return h.GetSessionStats(), nil
	case RPCCommandExportHTML:
		path, err := h.ExportHTML(command.OutputPath)
		if err != nil {
			return nil, err
		}
		return RPCExportHTMLResult{Path: path}, nil
	case RPCCommandSwitchSession:
		return h.SwitchSession(command.SessionPath)
	case RPCCommandFork:
		return h.Fork(command.EntryID)
	case RPCCommandGetLastAssistantText:
		return RPCLastAssistantTextResult{Text: h.GetLastAssistantText()}, nil
	case RPCCommandSetSessionName:
		name := strings.TrimSpace(command.Name)
		if name == "" {
			return nil, errors.New("Session name cannot be empty")
		}
		return nil, session.SetSessionName(name)
	case RPCCommandGetForkMessages:
		return RPCForkMessagesResult{Messages: session.GetUserMessagesForForking()}, nil
	case RPCCommandGetMessages:
		return RPCMessagesResult{Messages: session.Messages()}, nil
	case RPCCommandGetCommands:
		return h.GetCommands(), nil
	case RPCCommandClone:
		return h.Clone()
	default:
		return nil, errors.New("unsupported RPC command: " + command.Type)
	}
}

func (h *RPCSessionHost) AcceptPrompt(command RPCCommand) error {
	session := h.sessionSnapshot()
	if session == nil {
		return errors.New("RPC session host requires an active session")
	}
	if strings.TrimSpace(command.Message) == "" {
		return errors.New("prompt is required")
	}
	if session.IsStreaming() {
		switch command.StreamingBehavior {
		case "steer":
			return session.SteerWithImages(command.Message, command.Images)
		case "followUp":
			return session.FollowUpWithImages(command.Message, command.Images)
		default:
			return errors.New("Agent is already processing. Specify streamingBehavior ('steer' or 'followUp') to queue the message.")
		}
	}
	if err := h.runPromptPreflight(command); err != nil {
		return err
	}
	go func() {
		_ = session.PromptWithImages(command.Message, command.Images)
	}()
	return nil
}

func (h *RPCSessionHost) runPromptPreflight(command RPCCommand) error {
	if h.PromptPreflight == nil {
		return nil
	}
	return h.PromptPreflight(command)
}

func (h *RPCSessionHost) GetState() RPCSessionState {
	session := h.sessionSnapshot()
	if session == nil || session.SessionManager == nil || session.Agent == nil {
		return RPCSessionState{}
	}
	manager := session.SessionManager
	model := session.Agent.State.Model
	return RPCSessionState{
		Model:                 &model,
		ThinkingLevel:         session.Agent.State.ThinkingLevel,
		IsStreaming:           session.IsStreaming(),
		IsCompacting:          session.IsCompacting(),
		SteeringMode:          h.steeringMode(),
		FollowUpMode:          h.followUpMode(),
		SessionFile:           manager.GetSessionFile(),
		SessionID:             manager.GetSessionID(),
		SessionName:           manager.GetSessionName(),
		AutoCompactionEnabled: h.AutoCompactionEnabled,
		MessageCount:          len(session.Messages()),
		PendingMessageCount:   session.PendingMessageCount(),
	}
}

func (h *RPCSessionHost) SetThinkingLevel(level string) error {
	if !IsValidThinkingLevel(level) {
		return errors.New("invalid thinking level: " + level)
	}
	return h.applyThinkingLevel(level)
}

func (h *RPCSessionHost) SetModel(provider, modelID string) (llm.Model, error) {
	return h.setModel(provider, modelID, "set")
}

func (h *RPCSessionHost) setModel(provider, modelID, source string) (llm.Model, error) {
	for _, model := range h.getAvailableModels() {
		if model.Provider != provider || model.ID != modelID {
			continue
		}
		return h.applyModel(model, source, h.thinkingLevelForModelSwitch(""))
	}
	return llm.Model{}, errors.New("Model not found: " + provider + "/" + modelID)
}

func (h *RPCSessionHost) CycleModel() (*RPCCycleModelResult, error) {
	return h.CycleModelDirection("forward")
}

func (h *RPCSessionHost) CycleModelDirection(direction string) (*RPCCycleModelResult, error) {
	if len(h.ScopedModels) > 0 {
		return h.cycleScopedModel(direction)
	}
	models := h.getAvailableModels()
	if len(models) == 0 {
		return nil, nil
	}
	current := h.Session.Agent.State.Model
	index := -1
	for i, model := range models {
		if model.Provider == current.Provider && model.ID == current.ID {
			index = i
			break
		}
	}
	offset := 1
	if direction == "backward" {
		offset = -1
	}
	next := models[(index+offset+len(models))%len(models)]
	if _, err := h.setModel(next.Provider, next.ID, "cycle"); err != nil {
		return nil, err
	}
	return &RPCCycleModelResult{
		Model:         next,
		ThinkingLevel: h.Session.Agent.State.ThinkingLevel,
		IsScoped:      false,
	}, nil
}

func (h *RPCSessionHost) cycleScopedModel(direction string) (*RPCCycleModelResult, error) {
	if len(h.ScopedModels) <= 1 {
		return nil, nil
	}
	current := h.Session.Agent.State.Model
	index := -1
	for i, scoped := range h.ScopedModels {
		if scoped.Model.Provider == current.Provider && scoped.Model.ID == current.ID {
			index = i
			break
		}
	}
	offset := 1
	if direction == "backward" {
		offset = -1
	}
	next := h.ScopedModels[(index+offset+len(h.ScopedModels))%len(h.ScopedModels)]
	thinkingLevel := h.thinkingLevelForModelSwitch(next.ThinkingLevel)
	model, err := h.applyModel(next.Model, "cycle", thinkingLevel)
	if err != nil {
		return nil, err
	}
	return &RPCCycleModelResult{
		Model:         model,
		ThinkingLevel: h.Session.Agent.State.ThinkingLevel,
		IsScoped:      true,
	}, nil
}

func (h *RPCSessionHost) applyModel(model llm.Model, source, thinkingLevel string) (llm.Model, error) {
	if h.Session.Preflight != nil {
		if err := h.Session.Preflight(model); err != nil {
			return llm.Model{}, err
		}
	}
	previousModel := h.Session.Agent.State.Model
	h.Session.Agent.State.Model = model
	h.Session.SessionManager.AppendModelChange(model.Provider, model.ID)
	if err := h.applyThinkingLevel(thinkingLevel); err != nil {
		return llm.Model{}, err
	}
	if err := h.Session.emitExtensionEvent(ProtocolSessionEvent{
		Type:          ProtocolEventModelSelect,
		Model:         &model,
		PreviousModel: &previousModel,
		SelectSource:  source,
	}); err != nil {
		return llm.Model{}, err
	}
	return model, nil
}

func (h *RPCSessionHost) thinkingLevelForModelSwitch(explicitLevel string) string {
	if explicitLevel != "" {
		return explicitLevel
	}
	if h == nil || h.Session == nil || h.Session.Agent == nil {
		return string(DefaultThinkingLevel)
	}
	currentModel := h.Session.Agent.State.Model
	if !currentModel.Reasoning {
		return string(DefaultThinkingLevel)
	}
	if h.Session.Agent.State.ThinkingLevel == "" {
		return string(DefaultThinkingLevel)
	}
	return h.Session.Agent.State.ThinkingLevel
}

func (h *RPCSessionHost) CycleThinkingLevel() (string, error) {
	levels := llm.GetSupportedThinkingLevels(h.Session.Agent.State.Model)
	if len(levels) == 0 {
		levels = []string{string(ThinkingOff)}
	}
	current := h.Session.Agent.State.ThinkingLevel
	index := -1
	for i, level := range levels {
		if level == current {
			index = i
			break
		}
	}
	next := levels[(index+1)%len(levels)]
	if err := h.SetThinkingLevel(next); err != nil {
		return "", err
	}
	return next, nil
}

func (h *RPCSessionHost) applyThinkingLevel(level string) error {
	model := h.Session.Agent.State.Model
	effective := llm.ClampThinkingLevel(model, level)
	previous := h.Session.Agent.State.ThinkingLevel
	if effective == previous {
		return nil
	}
	h.Session.Agent.State.ThinkingLevel = effective
	h.Session.SessionManager.AppendThinkingLevelChange(effective)
	return h.Session.emitExtensionEvent(ProtocolSessionEvent{
		Type:          ProtocolEventThinkingLevelSelect,
		ThinkingLevel: effective,
		PreviousLevel: previous,
	})
}

func (h *RPCSessionHost) SetSteeringMode(mode string) error {
	if mode != "all" && mode != "one-at-a-time" {
		return errors.New("invalid steering mode: " + mode)
	}
	if h.Settings != nil {
		h.Settings.SetSteeringMode(mode)
	}
	h.SteeringMode = mode
	if h.Session != nil {
		if h.Session.SettingsManager != nil {
			h.Session.SyncRuntimeSettings()
		} else {
			h.Session.SteeringMode = mode
		}
	}
	return nil
}

func (h *RPCSessionHost) SetFollowUpMode(mode string) error {
	if mode != "all" && mode != "one-at-a-time" {
		return errors.New("invalid follow-up mode: " + mode)
	}
	if h.Settings != nil {
		h.Settings.SetFollowUpMode(mode)
	}
	h.FollowUpMode = mode
	if h.Session != nil {
		if h.Session.SettingsManager != nil {
			h.Session.SyncRuntimeSettings()
		} else {
			h.Session.FollowUpMode = mode
		}
	}
	return nil
}

func (h *RPCSessionHost) SetAutoCompaction(enabled *bool) error {
	if enabled == nil {
		return errors.New("auto compaction enabled value is required")
	}
	if h.Settings != nil {
		h.Settings.SetCompactionEnabled(*enabled)
	}
	h.AutoCompactionEnabled = *enabled
	if h.Session.SettingsManager != nil {
		h.Session.SyncRuntimeSettings()
	} else {
		h.Session.CompactionSettings.Enabled = *enabled
	}
	return nil
}

func (h *RPCSessionHost) SetAutoRetry(enabled *bool) error {
	if enabled == nil {
		return errors.New("auto retry enabled value is required")
	}
	if h.Settings != nil {
		h.Settings.SetRetryEnabled(*enabled)
	}
	if h.Session.SettingsManager != nil {
		h.Session.SyncRuntimeSettings()
	} else {
		h.Session.RetrySettings.Enabled = *enabled
		if *enabled && h.Session.RetrySettings.MaxRetries == 0 {
			h.Session.RetrySettings.MaxRetries = 3
		}
	}
	return nil
}

func (h *RPCSessionHost) Bash(ctx context.Context, command string) (BashResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return BashResult{}, errors.New("bash command is required")
	}
	return h.Session.ExecuteBash(command, AgentSessionBashOptions{Context: ctx})
}

func (h *RPCSessionHost) SwitchSession(sessionPath string) (RPCCloneResult, error) {
	sessionPath = strings.TrimSpace(sessionPath)
	if sessionPath == "" {
		return RPCCloneResult{}, errors.New("sessionPath is required")
	}
	manager, err := OpenSessionManager(sessionPath, h.Session.SessionManager.GetSessionDir())
	if err != nil {
		return RPCCloneResult{}, err
	}
	newSession, err := cloneAgentSessionWithManager(h.Session, manager)
	if err != nil {
		return RPCCloneResult{}, err
	}
	h.replaceSession(newSession)
	return RPCCloneResult{Cancelled: false}, nil
}

func (h *RPCSessionHost) Fork(entryID string) (RPCForkResult, error) {
	result, err := h.Session.Fork(entryID)
	if err != nil {
		return RPCForkResult{}, err
	}
	if !result.Cancelled && result.Session != nil {
		h.replaceSession(result.Session)
	}
	return RPCForkResult{Text: result.SelectedText, Cancelled: result.Cancelled}, nil
}

func (h *RPCSessionHost) Clone() (RPCCloneResult, error) {
	leafID := h.Session.SessionManager.GetLeafID()
	if leafID == nil || strings.TrimSpace(*leafID) == "" {
		manager, err := CreateSessionManager(h.Session.SessionManager.GetCWD(), h.Session.SessionManager.GetSessionDir())
		if err != nil {
			return RPCCloneResult{}, err
		}
		newSession, err := cloneAgentSessionWithManager(h.Session, manager)
		if err != nil {
			return RPCCloneResult{}, err
		}
		h.replaceSession(newSession)
		return RPCCloneResult{Cancelled: false}, nil
	}
	forkedManager, err := h.Session.SessionManager.ForkAtEntry(*leafID)
	if err != nil {
		return RPCCloneResult{}, err
	}
	newSession, err := cloneAgentSessionWithManager(h.Session, forkedManager)
	if err != nil {
		return RPCCloneResult{}, err
	}
	h.replaceSession(newSession)
	return RPCCloneResult{Cancelled: false}, nil
}

func (h *RPCSessionHost) GetSessionStats() RPCSessionStats {
	return rpcSessionStatsFromAgentStats(h.Session.sessionStats(false))
}

func rpcSessionStatsFromAgentStats(stats AgentSessionStats) RPCSessionStats {
	return RPCSessionStats{
		SessionFile:       stats.SessionFile,
		SessionID:         stats.SessionID,
		UserMessages:      stats.UserMessages,
		AssistantMessages: stats.AssistantMessages,
		ToolCalls:         stats.ToolCalls,
		ToolResults:       stats.ToolResults,
		TotalMessages:     stats.TotalMessages,
		Tokens:            rpcSessionTokens(stats.Tokens),
		Cost:              stats.Tokens.Cost.Total,
		ContextUsage:      stats.ContextUsage,
	}
}

func rpcSessionTokens(usage llm.Usage) RPCSessionTokens {
	return RPCSessionTokens{
		Input:      usage.Input,
		Output:     usage.Output,
		CacheRead:  usage.CacheRead,
		CacheWrite: usage.CacheWrite,
		Total:      usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite,
	}
}

func (h *RPCSessionHost) ExportHTML(outputPath string) (string, error) {
	if h == nil || h.Session == nil || h.Session.SessionManager == nil {
		return "", errors.New("RPC session host requires an active session")
	}
	if !h.Session.SessionManager.hasAssistantMessage() {
		return "", errNothingToExport
	}
	path := strings.TrimSpace(outputPath)
	if path == "" {
		if sessionFile := h.Session.SessionManager.GetSessionFile(); sessionFile != "" {
			path = strings.TrimSuffix(sessionFile, filepath.Ext(sessionFile)) + ".html"
		} else {
			path = filepath.Join(os.TempDir(), h.Session.SessionManager.GetSessionID()+".html")
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	htmlText := h.renderHTML()
	if err := os.WriteFile(path, []byte(htmlText), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (h *RPCSessionHost) ExportJSONL(outputPath string) (string, error) {
	if h == nil || h.Session == nil || h.Session.SessionManager == nil {
		return "", errors.New("RPC session host requires an active session")
	}
	path := strings.TrimSpace(outputPath)
	if path == "" {
		if sessionFile := h.Session.SessionManager.GetSessionFile(); sessionFile != "" {
			path = sessionFile
		} else {
			path = filepath.Join(os.TempDir(), h.Session.SessionManager.GetSessionID()+".jsonl")
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	var builder strings.Builder
	for _, entry := range h.Session.SessionManager.allEntriesSnapshot() {
		line, err := json.Marshal(entry)
		if err != nil {
			return "", err
		}
		builder.Write(line)
		builder.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (h *RPCSessionHost) GetLastAssistantText() *string {
	messages := h.Session.Messages()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != llm.RoleAssistant {
			continue
		}
		text := rpcMessageText(messages[i])
		return &text
	}
	return nil
}

func (h *RPCSessionHost) GetCommands() RPCCommandsResult {
	var commands []RPCSlashCommand
	for _, command := range builtinInteractiveSlashCommands() {
		commands = append(commands, RPCSlashCommand{
			Name:        command.Name,
			Description: command.Description,
			Source:      "builtin",
		})
	}
	if h == nil || h.Session == nil {
		return RPCCommandsResult{Commands: commands}
	}
	if h.Session.ExtensionRuntime != nil {
		for _, command := range h.Session.ExtensionRuntime.RegisteredCommands() {
			commands = append(commands, RPCSlashCommand{
				Name:         command.InvocationName,
				Description:  command.Description,
				ArgumentHint: command.ArgumentHint,
				Source:       "extension",
				SourceInfo:   command.SourceInfo,
			})
		}
	}
	if loader, ok := h.Session.ResourceLoader.(AgentSessionPromptResourceLoader); ok {
		for _, template := range loader.GetPrompts().Prompts {
			commands = append(commands, RPCSlashCommand{
				Name:         template.Name,
				Description:  template.Description,
				ArgumentHint: template.ArgumentHint,
				Source:       "prompt",
				SourceInfo:   template.SourceInfo,
			})
		}
	}
	if h.Session.ResourceLoader != nil && h.skillCommandsEnabled() {
		for _, skill := range h.Session.ResourceLoader.GetSkills().Skills {
			commands = append(commands, RPCSlashCommand{
				Name:        "skill:" + skill.Name,
				Description: skill.Description,
				Source:      "skill",
				SourceInfo:  skill.SourceInfo,
			})
		}
	}
	return RPCCommandsResult{Commands: commands}
}

func (h *RPCSessionHost) skillCommandsEnabled() bool {
	if h == nil || h.Settings == nil {
		return true
	}
	return h.Settings.GetEnableSkillCommands()
}

func (h *RPCSessionHost) getAvailableModels() []llm.Model {
	if len(h.AvailableModels) > 0 {
		return append([]llm.Model(nil), h.AvailableModels...)
	}
	var models []llm.Model
	for _, provider := range llm.GetProviders() {
		models = append(models, llm.GetModels(provider)...)
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider == models[j].Provider {
			return models[i].ID < models[j].ID
		}
		return models[i].Provider < models[j].Provider
	})
	return models
}

func (h *RPCSessionHost) steeringMode() string {
	if h.SteeringMode != "" {
		return h.SteeringMode
	}
	return "all"
}

func (h *RPCSessionHost) followUpMode() string {
	if h.FollowUpMode != "" {
		return h.FollowUpMode
	}
	return "all"
}

func (h *RPCSessionHost) renderHTML() string {
	options := ExportHTMLOptions{}
	if h != nil && h.Settings != nil {
		options.ThemeName = h.Settings.GetTheme()
		options.AgentDir = h.Settings.agentDir
	}
	if h != nil && h.Session != nil && h.Session.ExtensionRuntime != nil {
		runtime := h.Session.ExtensionRuntime
		options.ToolRenderer = CreateToolHTMLRenderer(ExportHTMLToolRendererDeps{
			CWD: h.Session.SessionManager.GetCWD(),
			GetToolDefinition: func(name string) (ToolDefinition, bool) {
				renderer := runtime.GetToolRenderer(name)
				if renderer == nil {
					return ToolDefinition{}, false
				}
				return ToolDefinition{
					Name:         name,
					RenderCall:   renderer.RenderCall,
					RenderResult: renderer.RenderResult,
				}, true
			},
		})
	}
	return RenderSessionManagerHTMLWithOptions(h.Session.SessionManager, options)
}

func rpcMessageText(message llm.Message) string {
	parts := make([]string, 0, len(message.Content))
	for _, part := range message.Content {
		if part.Type == llm.ContentText {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "")
}
