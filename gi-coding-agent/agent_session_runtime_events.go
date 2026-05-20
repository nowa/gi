package gicodingagent

import "errors"

const (
	ProtocolEventSessionBeforeSwitch = "session_before_switch"
	ProtocolEventSessionBeforeFork   = "session_before_fork"
	ProtocolEventSessionShutdown     = "session_shutdown"
	ProtocolEventSessionStart        = "session_start"
)

type AgentSessionRuntimeHost struct {
	Session                 *AgentSession
	ExtensionRuntime        *ProtocolExtensionRuntime
	BeforeSessionInvalidate func()
	RebindSession           func(*AgentSession) error
}

type AgentSessionRuntimeSwitchResult struct {
	Cancelled bool `json:"cancelled"`
}

type AgentSessionRuntimeForkOptions struct {
	Position string
}

func NewAgentSessionRuntimeHost(session *AgentSession, extensionRuntime *ProtocolExtensionRuntime) (*AgentSessionRuntimeHost, error) {
	host := &AgentSessionRuntimeHost{Session: session, ExtensionRuntime: extensionRuntime}
	if _, err := host.emitSessionEvent(ProtocolSessionEvent{Type: ProtocolEventSessionStart, Reason: "startup"}); err != nil {
		return nil, err
	}
	return host, nil
}

func (h *AgentSessionRuntimeHost) NewSession() (AgentSessionRuntimeSwitchResult, error) {
	if h == nil || h.Session == nil || h.Session.SessionManager == nil {
		return AgentSessionRuntimeSwitchResult{}, errors.New("session runtime host requires an active session")
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
	return AgentSessionRuntimeSwitchResult{}, h.replaceSession(newSession, "new", newManager.GetSessionFile(), oldManager.GetSessionFile())
}

func (h *AgentSessionRuntimeHost) SwitchSession(sessionFile string) (AgentSessionRuntimeSwitchResult, error) {
	result, err := h.emitSessionEvent(ProtocolSessionEvent{Type: ProtocolEventSessionBeforeSwitch, Reason: "resume", TargetSessionFile: sessionFile})
	if err != nil || result.Cancel {
		return AgentSessionRuntimeSwitchResult{Cancelled: result.Cancel}, err
	}
	oldSession := h.Session
	manager, err := OpenSessionManager(sessionFile, oldSession.SessionManager.GetSessionDir(), oldSession.SessionManager.GetCWD())
	if err != nil {
		return AgentSessionRuntimeSwitchResult{}, err
	}
	newSession, err := cloneAgentSessionWithManager(oldSession, manager)
	if err != nil {
		return AgentSessionRuntimeSwitchResult{}, err
	}
	return AgentSessionRuntimeSwitchResult{}, h.replaceSession(newSession, "resume", sessionFile, oldSession.SessionManager.GetSessionFile())
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
	forkResult, err := oldSession.Fork(entryID)
	if err != nil {
		return AgentSessionForkResult{}, err
	}
	previousSessionFile := oldSession.SessionManager.GetSessionFile()
	if err := h.replaceSession(forkResult.Session, "fork", forkResult.Session.SessionManager.GetSessionFile(), previousSessionFile); err != nil {
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

func (h *AgentSessionRuntimeHost) replaceSession(newSession *AgentSession, reason, targetSessionFile, previousSessionFile string) error {
	if _, err := h.emitSessionEvent(ProtocolSessionEvent{Type: ProtocolEventSessionShutdown, Reason: reason, TargetSessionFile: targetSessionFile}); err != nil {
		return err
	}
	if h.BeforeSessionInvalidate != nil {
		h.BeforeSessionInvalidate()
	}
	h.Session = newSession
	if h.RebindSession != nil {
		if err := h.RebindSession(newSession); err != nil {
			return err
		}
	}
	_, err := h.emitSessionEvent(ProtocolSessionEvent{Type: ProtocolEventSessionStart, Reason: reason, PreviousSessionFile: previousSessionFile})
	return err
}

func (h *AgentSessionRuntimeHost) emitSessionEvent(event ProtocolSessionEvent) (ProtocolEventResult, error) {
	if h == nil || h.ExtensionRuntime == nil {
		return ProtocolEventResult{}, nil
	}
	return h.ExtensionRuntime.EmitSessionEvent(event)
}

func cloneAgentSessionWithManager(source *AgentSession, manager *SessionManager) (*AgentSession, error) {
	return CreateAgentSession(AgentSessionOptions{
		CWD:                  manager.GetCWD(),
		Model:                source.Agent.State.Model,
		SessionManager:       manager,
		ResourceLoader:       source.ResourceLoader,
		CompactionSettings:   &source.CompactionSettings,
		CompactionSummarizer: source.CompactionSummarizer,
		BranchSummarizer:     source.BranchSummarizer,
		RetrySettings:        &source.RetrySettings,
		AutoCompactionRunner: source.AutoCompactionRunner,
		AgentContinue:        source.AgentContinue,
		Responder:            source.Responder,
	})
}
