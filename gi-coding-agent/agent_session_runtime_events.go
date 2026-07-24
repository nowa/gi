package gicodingagent

import (
	"context"
	"errors"
	"sync"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	ProtocolEventSessionBeforeSwitch   = "session_before_switch"
	ProtocolEventSessionBeforeFork     = "session_before_fork"
	ProtocolEventSessionShutdown       = "session_shutdown"
	ProtocolEventSessionStart          = "session_start"
	ProtocolEventSessionSwitch         = "session_switch"
	ProtocolEventSessionInfoChanged    = "session_info_changed"
	ProtocolEventSessionBeforeTree     = "session_before_tree"
	ProtocolEventModelSelect           = "model_select"
	ProtocolEventThinkingLevelSelect   = "thinking_level_select"
	ProtocolEventBeforeAgentStart      = "before_agent_start"
	ProtocolEventAgentSettled          = "agent_settled"
	ProtocolEventContext               = "context"
	ProtocolEventToolCall              = "tool_call"
	ProtocolEventToolResult            = "tool_result"
	ProtocolEventToolExecutionUpdate   = "tool_execution_update"
	ProtocolEventMessageStart          = "message_start"
	ProtocolEventMessageUpdate         = "message_update"
	ProtocolEventMessageEnd            = "message_end"
	ProtocolEventUserBash              = "user_bash"
	ProtocolEventResourcesDiscover     = "resources_discover"
	ProtocolEventBeforeProviderRequest = "before_provider_request"
	ProtocolEventAfterProviderResponse = "after_provider_response"
)

type AgentSessionRuntimeHost struct {
	Session                 *AgentSession
	ExtensionRuntime        *ProtocolExtensionRuntime
	BeforeSessionInvalidate func()
	RebindSession           func(*AgentSession) error
	eventListenersMu        sync.Mutex
	eventListeners          []agentSessionRuntimeEventListenerRegistration
	nextEventListenerID     int
}

type AgentSessionRuntimeEventListener func(ProtocolSessionEvent) error

type agentSessionRuntimeEventListenerRegistration struct {
	id       int
	listener AgentSessionRuntimeEventListener
}

type AgentSessionRuntimeSwitchResult struct {
	Cancelled bool `json:"cancelled"`
}

type AgentSessionRuntimeForkOptions struct {
	Position    string
	WithSession func(ProtocolCommandContext) error
}

func NewAgentSessionRuntimeHost(session *AgentSession, extensionRuntime *ProtocolExtensionRuntime) (*AgentSessionRuntimeHost, error) {
	host := &AgentSessionRuntimeHost{Session: session, ExtensionRuntime: extensionRuntime}
	if extensionRuntime != nil {
		extensionRuntime.BindSession(session)
		host.bindCommandContext()
	}
	if _, err := host.emitSessionEvent(ProtocolSessionEvent{Type: ProtocolEventSessionStart, Reason: "startup"}); err != nil {
		return nil, err
	}
	if extensionRuntime != nil {
		extensionRuntime.ApplyToSession(session)
	}
	return host, nil
}

func (h *AgentSessionRuntimeHost) bindCommandContext() {
	if h == nil || h.ExtensionRuntime == nil {
		return
	}
	h.ExtensionRuntime.BindCommandContext(ProtocolCommandContextActions{
		WaitForIdle: func() error {
			if h.Session == nil {
				return nil
			}
			return h.Session.WaitForIdle(context.Background())
		},
		NewSession: func(options ProtocolNewSessionOptions) (ProtocolCommandSwitchResult, error) {
			result, err := h.NewSession(options)
			return ProtocolCommandSwitchResult{Cancelled: result.Cancelled}, err
		},
		Fork: func(entryID string, options ProtocolForkOptions) (ProtocolCommandForkResult, error) {
			result, err := h.Fork(entryID, AgentSessionRuntimeForkOptions{Position: options.Position, WithSession: options.WithSession})
			return ProtocolCommandForkResult{Cancelled: result.Cancelled}, err
		},
		SwitchSession: func(sessionFile string, options ProtocolSwitchSessionOptions) (ProtocolCommandSwitchResult, error) {
			result, err := h.SwitchSession(sessionFile, options)
			return ProtocolCommandSwitchResult{Cancelled: result.Cancelled}, err
		},
		Reload: h.Reload,
	})
}

func (h *AgentSessionRuntimeHost) NewSession(options ...ProtocolNewSessionOptions) (AgentSessionRuntimeSwitchResult, error) {
	if h == nil || h.Session == nil || h.Session.SessionManager == nil {
		return AgentSessionRuntimeSwitchResult{}, errors.New("session runtime host requires an active session")
	}
	option := ProtocolNewSessionOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	result, err := h.emitSessionEvent(ProtocolSessionEvent{Type: ProtocolEventSessionBeforeSwitch, Reason: "new"})
	if err != nil || result.Cancel {
		return AgentSessionRuntimeSwitchResult{Cancelled: result.Cancel}, err
	}
	oldSession := h.Session
	oldManager := oldSession.SessionManager
	newManager, err := CreateSessionManager(oldManager.GetCWD(), oldManager.GetSessionDir())
	if err != nil {
		return AgentSessionRuntimeSwitchResult{}, err
	}
	newSession, err := cloneAgentSessionWithManager(oldSession, newManager)
	if err != nil {
		return AgentSessionRuntimeSwitchResult{}, err
	}
	if option.ParentSession != "" {
		newManager.NewSession(NewSessionOptions{ParentSession: option.ParentSession})
	}
	if err := h.replaceSession(newSession, "new", newManager.GetSessionFile(), oldManager.GetSessionFile(), option.WithSession); err != nil {
		return AgentSessionRuntimeSwitchResult{}, err
	}
	return AgentSessionRuntimeSwitchResult{}, nil
}

func (h *AgentSessionRuntimeHost) SwitchSession(sessionFile string, options ...ProtocolSwitchSessionOptions) (AgentSessionRuntimeSwitchResult, error) {
	option := ProtocolSwitchSessionOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	result, err := h.emitSessionEvent(ProtocolSessionEvent{Type: ProtocolEventSessionBeforeSwitch, Reason: "resume", TargetSessionFile: sessionFile})
	if err != nil || result.Cancel {
		return AgentSessionRuntimeSwitchResult{Cancelled: result.Cancel}, err
	}
	oldSession := h.Session
	manager, err := OpenSessionManager(sessionFile, oldSession.SessionManager.GetSessionDir())
	if err != nil {
		return AgentSessionRuntimeSwitchResult{}, err
	}
	newSession, err := cloneAgentSessionWithManager(oldSession, manager)
	if err != nil {
		return AgentSessionRuntimeSwitchResult{}, err
	}
	if err := h.replaceSession(newSession, "resume", sessionFile, oldSession.SessionManager.GetSessionFile(), option.WithSession); err != nil {
		return AgentSessionRuntimeSwitchResult{}, err
	}
	return AgentSessionRuntimeSwitchResult{}, nil
}

func (h *AgentSessionRuntimeHost) Reload() error {
	if h == nil || h.Session == nil {
		return errors.New("session runtime host requires an active session")
	}
	sessionFile := ""
	if h.Session.SessionManager != nil {
		sessionFile = h.Session.SessionManager.GetSessionFile()
	}
	if _, err := h.emitSessionEvent(ProtocolSessionEvent{Type: ProtocolEventSessionShutdown, Reason: "reload", TargetSessionFile: sessionFile}); err != nil {
		return err
	}
	if h.ExtensionRuntime != nil {
		h.ExtensionRuntime.BindSession(h.Session)
		h.bindCommandContext()
		h.ExtensionRuntime.ApplyToSession(h.Session)
	}
	h.Session.SyncRuntimeSettings()
	_, err := h.emitSessionEvent(ProtocolSessionEvent{Type: ProtocolEventSessionStart, Reason: "reload", PreviousSessionFile: sessionFile})
	return err
}

func (h *AgentSessionRuntimeHost) SetExtensionRuntime(extensionRuntime *ProtocolExtensionRuntime) {
	if h == nil {
		return
	}
	h.ExtensionRuntime = extensionRuntime
	if h.Session != nil {
		h.Session.ExtensionRuntime = extensionRuntime
	}
}

func (h *AgentSessionRuntimeHost) Fork(entryID string, options ...AgentSessionRuntimeForkOptions) (AgentSessionForkResult, error) {
	position := "before"
	if len(options) > 0 && options[0].Position != "" {
		position = options[0].Position
	}
	result, err := h.emitSessionEvent(ProtocolSessionEvent{Type: ProtocolEventSessionBeforeFork, EntryID: entryID, Position: position})
	if err != nil || result.Cancel {
		return AgentSessionForkResult{Cancelled: result.Cancel}, err
	}
	oldSession := h.Session
	var forkResult AgentSessionForkResult
	if position == "at" {
		forkResult, err = oldSession.ForkAt(entryID)
	} else {
		forkResult, err = oldSession.Fork(entryID)
	}
	if err != nil {
		return AgentSessionForkResult{}, err
	}
	previousSessionFile := oldSession.SessionManager.GetSessionFile()
	if err := h.replaceSession(forkResult.Session, "fork", forkResult.Session.SessionManager.GetSessionFile(), previousSessionFile, optionsWithSession(options)); err != nil {
		return AgentSessionForkResult{}, err
	}
	forkResult.Session = h.Session
	return forkResult, nil
}

func (h *AgentSessionRuntimeHost) SetBeforeSessionInvalidate(callback func()) {
	h.BeforeSessionInvalidate = callback
}

func (h *AgentSessionRuntimeHost) SetRebindSession(callback func(*AgentSession) error) {
	h.RebindSession = callback
}

func (h *AgentSessionRuntimeHost) OnSessionEvent(listener AgentSessionRuntimeEventListener) func() {
	if h == nil || listener == nil {
		return func() {}
	}
	h.eventListenersMu.Lock()
	h.nextEventListenerID++
	id := h.nextEventListenerID
	h.eventListeners = append(h.eventListeners, agentSessionRuntimeEventListenerRegistration{id: id, listener: listener})
	h.eventListenersMu.Unlock()
	return func() {
		h.eventListenersMu.Lock()
		defer h.eventListenersMu.Unlock()
		for index, registration := range h.eventListeners {
			if registration.id != id {
				continue
			}
			h.eventListeners = append(h.eventListeners[:index], h.eventListeners[index+1:]...)
			return
		}
	}
}

func (h *AgentSessionRuntimeHost) replaceSession(newSession *AgentSession, reason, targetSessionFile, previousSessionFile string, withSession func(ProtocolCommandContext) error) error {
	if _, err := h.emitSessionEvent(ProtocolSessionEvent{Type: ProtocolEventSessionShutdown, Reason: reason, TargetSessionFile: targetSessionFile}); err != nil {
		return err
	}
	if h.ExtensionRuntime != nil {
		h.ExtensionRuntime.InvalidateCommandContexts()
	}
	if h.BeforeSessionInvalidate != nil {
		h.BeforeSessionInvalidate()
	}
	h.Session = newSession
	if h.ExtensionRuntime != nil {
		h.ExtensionRuntime.BindSession(newSession)
		h.bindCommandContext()
	}
	if h.RebindSession != nil {
		if err := h.RebindSession(newSession); err != nil {
			return err
		}
	}
	if _, err := h.emitSessionEvent(ProtocolSessionEvent{
		Type:                ProtocolEventSessionSwitch,
		Reason:              reason,
		TargetSessionFile:   targetSessionFile,
		PreviousSessionFile: previousSessionFile,
	}); err != nil {
		return err
	}
	_, err := h.emitSessionEvent(ProtocolSessionEvent{Type: ProtocolEventSessionStart, Reason: reason, PreviousSessionFile: previousSessionFile})
	if h.ExtensionRuntime != nil {
		h.ExtensionRuntime.ApplyToSession(newSession)
	}
	if err != nil {
		return err
	}
	if withSession != nil {
		if h.ExtensionRuntime == nil {
			return errors.New("extension runtime is required for withSession callback")
		}
		return withSession(h.ExtensionRuntime.CreateCommandContext())
	}
	return err
}

func optionsWithSession(options []AgentSessionRuntimeForkOptions) func(ProtocolCommandContext) error {
	if len(options) == 0 {
		return nil
	}
	return options[0].WithSession
}

func (h *AgentSessionRuntimeHost) emitSessionEvent(event ProtocolSessionEvent) (ProtocolEventResult, error) {
	if h == nil {
		return ProtocolEventResult{}, nil
	}
	var result ProtocolEventResult
	var err error
	if h.ExtensionRuntime != nil {
		result, err = h.ExtensionRuntime.EmitSessionEvent(event)
		if err != nil || result.Cancel {
			return result, err
		}
	}
	for _, listener := range h.sessionEventListeners() {
		if err := listener(event); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (h *AgentSessionRuntimeHost) sessionEventListeners() []AgentSessionRuntimeEventListener {
	if h == nil {
		return nil
	}
	h.eventListenersMu.Lock()
	defer h.eventListenersMu.Unlock()
	listeners := make([]AgentSessionRuntimeEventListener, 0, len(h.eventListeners))
	for _, registration := range h.eventListeners {
		if registration.listener != nil {
			listeners = append(listeners, registration.listener)
		}
	}
	return listeners
}

func cloneAgentSessionWithManager(source *AgentSession, manager *SessionManager) (*AgentSession, error) {
	cloned, err := CreateAgentSession(AgentSessionOptions{
		CWD:                  manager.GetCWD(),
		Model:                source.Agent.State.Model,
		SettingsManager:      source.SettingsManager,
		SessionManager:       manager,
		ResourceLoader:       source.ResourceLoader,
		CompactionSettings:   &source.CompactionSettings,
		CompactionSummarizer: source.CompactionSummarizer,
		BranchSummarizer:     source.BranchSummarizer,
		RetrySettings:        &source.RetrySettings,
		AutoCompactionRunner: source.AutoCompactionRunner,
		AgentContinue:        source.AgentContinue,
		Responder:            source.Responder,
		StreamResponder:      source.StreamResponder,
		ModelRuntime:         source.ModelRuntime,
		SummaryRuntime:       source.SummaryRuntime,
		ScopedModels:         source.ScopedModels,
		Tools:                source.Tools,
		ToolsSet:             source.ToolsSet,
		NoTools:              source.NoTools,
	})
	if err != nil {
		return nil, err
	}
	cloned.SteeringMode = source.SteeringMode
	cloned.FollowUpMode = source.FollowUpMode
	context := manager.BuildSessionContext()
	if context.Model != nil {
		model, _ := llm.GetModel(context.Model.Provider, context.Model.ModelID)
		cloned.Agent.State.Model = model
	}
	if hasSessionThinkingLevel(manager) && context.ThinkingLevel != "" {
		cloned.Agent.State.ThinkingLevel = context.ThinkingLevel
	}
	return cloned, nil
}
