package gicodingagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

type HostActionRequest struct {
	Type     string          `json:"type"`
	Protocol string          `json:"protocol,omitempty"`
	ID       string          `json:"id"`
	Method   string          `json:"method"`
	Params   json.RawMessage `json:"params"`
}

type HostActionResponse struct {
	Type     string           `json:"type"`
	Protocol string           `json:"protocol,omitempty"`
	ID       string           `json:"id"`
	Result   any              `json:"result,omitempty"`
	Error    *HostActionError `json:"error,omitempty"`
}

type HostActionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type TUIEditorHost interface {
	ReadEditorText() string
	SetEditorText(text string)
	InsertEditorText(text string)
	SubmitEditorText() error
}

type TUIEditorPasteHost interface {
	PasteEditorText(text string)
}

type TUIEditorCursorHost interface {
	EditorCursor() (line, col int, ok bool)
}

type TUIEditorFocusHost interface {
	FocusEditor() error
	EditorFocused() bool
}

type TUIEditorCustomStateHost interface {
	EditorCustomActive() bool
}

type TUIDialogHost interface {
	RunTUIDialog(request TUIDialogRequest) (TUIDialogResult, error)
}

type TUITitleHost interface {
	SetTUITitle(title string) error
}

type TUIWorkingHost interface {
	SetTUIWorking(update TUIWorkingUpdate) error
}

type TUIThinkingLabelHost interface {
	SetHiddenThinkingLabel(label string) error
}

type TUIStatusHost interface {
	SetTUIStatus(key, text string) error
}

type TUIThemeHost interface {
	CurrentTUITheme() string
	AvailableTUIThemes() []TUIThemeInfo
	SetTUITheme(name string) error
}

type TUIToolExpansionHost interface {
	TUIToolsExpanded() bool
	SetTUIToolsExpanded(expanded bool) error
}

type TUIThemeInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Builtin bool   `json:"builtin,omitempty"`
}

type TUIWorkingUpdate struct {
	Message        string
	MessageSet     bool
	ResetMessage   bool
	Visible        bool
	VisibleSet     bool
	Indicator      TUIWorkingIndicatorOptions
	IndicatorSet   bool
	ResetIndicator bool
}

type TUIWorkingIndicatorOptions struct {
	Frames     []string `json:"frames,omitempty"`
	IntervalMs int      `json:"intervalMs,omitempty"`
}

type TUIDialogRequest struct {
	Kind         string            `json:"kind"`
	Type         string            `json:"type,omitempty"`
	Title        string            `json:"title,omitempty"`
	Message      string            `json:"message,omitempty"`
	Placeholder  string            `json:"placeholder,omitempty"`
	Options      []TUIDialogOption `json:"options,omitempty"`
	DefaultValue any               `json:"defaultValue,omitempty"`
	Timeout      int               `json:"timeout,omitempty"`
}

type TUIDialogOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Value       any    `json:"value,omitempty"`
}

type TUIDialogResult struct {
	Action   string `json:"action"`
	OptionID string `json:"optionId,omitempty"`
	Value    any    `json:"value,omitempty"`
}

type HostProcessExecutor interface {
	ExecuteHostProcess(command []string, cwd string) (HostProcessResult, error)
}

type HostProcessExecutorWithOptions interface {
	ExecuteHostProcessWithOptions(command []string, cwd string, options HostProcessOptions) (HostProcessResult, error)
}

type HostProcessOptions struct {
	Timeout time.Duration
	Context context.Context
}

const (
	hostProcessForceKillDelay = 5 * time.Second
	hostProcessStdioGrace     = 100 * time.Millisecond
)

type HostProcessResult struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	Killed   bool   `json:"killed,omitempty"`
}

type LocalHostProcessExecutor struct{}

type HostPolicyRequester interface {
	RequestHostPolicyGrant(request HostPolicyRequest) (HostPolicyDecision, error)
}

type HostPolicyRequesterFunc func(HostPolicyRequest) (HostPolicyDecision, error)

func (f HostPolicyRequesterFunc) RequestHostPolicyGrant(request HostPolicyRequest) (HostPolicyDecision, error) {
	return f(request)
}

type HostPolicyRequest struct {
	Capability   string   `json:"capability,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	Persistent   bool     `json:"persistent,omitempty"`
}

type HostPolicyDecision struct {
	Granted             bool     `json:"granted"`
	GrantedCapabilities []string `json:"grantedCapabilities,omitempty"`
	DeniedCapabilities  []string `json:"deniedCapabilities,omitempty"`
	Reason              string   `json:"reason,omitempty"`
	Persistent          bool     `json:"persistent,omitempty"`
}

type hostTUIMountParams struct {
	MountID        string                  `json:"mountId"`
	Slot           string                  `json:"slot"`
	View           ViewTreeNode            `json:"view"`
	Priority       int                     `json:"priority,omitempty"`
	Overlay        *ViewTreeOverlayOptions `json:"overlay,omitempty"`
	OverlayOptions *ViewTreeOverlayOptions `json:"overlayOptions,omitempty"`
}

type hostTUIPatchParams struct {
	MountID string                   `json:"mountId"`
	Ops     []ViewTreePatchOperation `json:"ops"`
}

type hostTUIUnmountParams struct {
	MountID string `json:"mountId"`
}

type hostTUIStatusParams struct {
	Key      string `json:"key"`
	Text     string `json:"text"`
	Priority int    `json:"priority,omitempty"`
}

type hostTUITitleParams struct {
	Title string `json:"title"`
}

type hostTUIWorkingParams struct {
	Message        *string                     `json:"message,omitempty"`
	ResetMessage   bool                        `json:"resetMessage,omitempty"`
	Visible        *bool                       `json:"visible,omitempty"`
	Indicator      *TUIWorkingIndicatorOptions `json:"indicator,omitempty"`
	ResetIndicator bool                        `json:"resetIndicator,omitempty"`
}

type hostTUIThinkingLabelParams struct {
	Label string `json:"label,omitempty"`
	Reset bool   `json:"reset,omitempty"`
}

type hostTUIThemeParams struct {
	Action string `json:"action,omitempty"`
	Name   string `json:"name,omitempty"`
}

type hostTUIToolsExpandedParams struct {
	Expanded *bool `json:"expanded,omitempty"`
}

type hostTUIEditorParams struct {
	Action  string `json:"action"`
	Text    string `json:"text,omitempty"`
	Force   bool   `json:"force,omitempty"`
	Trigger string `json:"trigger,omitempty"`
}

type hostToolsSetActiveParams struct {
	Mode      string   `json:"mode"`
	ToolNames []string `json:"toolNames"`
}

type hostToolInfo struct {
	Name          string             `json:"name"`
	Label         string             `json:"label,omitempty"`
	Description   string             `json:"description,omitempty"`
	Parameters    llm.Schema         `json:"parameters,omitempty"`
	PromptSnippet string             `json:"promptSnippet,omitempty"`
	ExecutionMode string             `json:"executionMode,omitempty"`
	SourceInfo    ProtocolSourceInfo `json:"sourceInfo"`
	Active        bool               `json:"active"`
}

type hostSessionAppendCustomParams struct {
	Owner string `json:"owner"`
	Type  string `json:"type"`
	Data  any    `json:"data"`
}

type hostSessionEntriesParams struct {
	Scope  string `json:"scope,omitempty"`
	Branch bool   `json:"branch,omitempty"`
}

type hostSessionSetLabelParams struct {
	EntryID string `json:"entryId"`
	Label   string `json:"label"`
}

type hostSessionSetNameParams struct {
	Name string `json:"name"`
}

type hostModelSelectParams struct {
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

type hostThinkingSetParams struct {
	Level string `json:"level"`
}

type hostAgentSendUserMessageParams struct {
	Text      string `json:"text"`
	Message   string `json:"message,omitempty"`
	DeliverAs string `json:"deliverAs,omitempty"`
}

type hostAgentRunParams struct {
	Prompt        string   `json:"prompt"`
	Task          string   `json:"task,omitempty"`
	Message       string   `json:"message,omitempty"`
	Name          string   `json:"name,omitempty"`
	CWD           string   `json:"cwd,omitempty"`
	Tools         []string `json:"tools,omitempty"`
	NoTools       string   `json:"noTools,omitempty"`
	ParentSession string   `json:"parentSession,omitempty"`
}

type hostAgentAbortParams struct {
	Target      string `json:"target,omitempty"`
	SessionID   string `json:"sessionId,omitempty"`
	SessionFile string `json:"sessionFile,omitempty"`
}

type hostSessionActionParams struct {
	Action             string `json:"action"`
	EntryID            string `json:"entryId,omitempty"`
	TargetID           string `json:"targetId,omitempty"`
	SessionFile        string `json:"sessionFile,omitempty"`
	ParentSession      string `json:"parentSession,omitempty"`
	Summarize          bool   `json:"summarize,omitempty"`
	CustomInstructions string `json:"customInstructions,omitempty"`
}

type hostFSReadParams struct {
	Path string `json:"path"`
}

type hostFSWriteParams struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type hostProcessExecParams struct {
	Command       []string `json:"command"`
	CWD           string   `json:"cwd,omitempty"`
	TimeoutMillis int      `json:"timeoutMillis,omitempty"`
}

type hostPolicyRequestParams = HostPolicyRequest

type hostTUIDialogParams = TUIDialogRequest

func (h *RPCSessionHost) HandleHostAction(request HostActionRequest) HostActionResponse {
	return h.HandleHostActionContext(context.Background(), request)
}

func (h *RPCSessionHost) HandleHostActionContext(ctx context.Context, request HostActionRequest) HostActionResponse {
	if h == nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "internal_error", "RPC session host is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if request.Protocol == "" {
		request.Protocol = "gi-ext-rpc@1"
	}
	switch request.Method {
	case "host.tools.list":
		if h.Session == nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active session")
		}
		active := stringSet(h.Session.GetActiveToolNames())
		tools := make([]hostToolInfo, 0, len(hostActionRegisteredTools(h.Session)))
		for _, tool := range hostActionRegisteredTools(h.Session) {
			tools = append(tools, hostToolInfo{
				Name:          tool.Name,
				Label:         tool.Label,
				Description:   tool.Description,
				Parameters:    tool.Parameters,
				PromptSnippet: tool.PromptSnippet,
				ExecutionMode: tool.ExecutionMode,
				SourceInfo:    tool.SourceInfo,
				Active:        active[tool.Name],
			})
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{
			"tools":           tools,
			"activeToolNames": h.Session.GetActiveToolNames(),
		})
	case "host.tools.set_active":
		var params hostToolsSetActiveParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		activeNames, err := h.setActiveTools(params.Mode, params.ToolNames)
		if err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"activeToolNames": activeNames})
	case "host.session.append_custom":
		var params hostSessionAppendCustomParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		entryID, err := h.appendCustomSessionEntry(params)
		if err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"entryId": entryID, "type": strings.TrimSpace(params.Type)})
	case "host.commands.list":
		if h.Session == nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active session")
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, h.GetCommands())
	case "host.session.entries":
		var params hostSessionEntriesParams
		if len(request.Params) > 0 {
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
			}
		}
		if h.Session == nil || h.Session.SessionManager == nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active session")
		}
		entries := h.Session.SessionManager.GetEntries()
		if params.Branch || strings.TrimSpace(params.Scope) == "branch" {
			entries = h.Session.SessionManager.GetBranch()
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"entries": entries})
	case "host.session.set_label":
		var params hostSessionSetLabelParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		if h.Session == nil || h.Session.SessionManager == nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active session")
		}
		entryID, err := h.Session.SessionManager.AppendLabelChange(params.EntryID, params.Label)
		if err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"entryId": entryID, "targetEntryId": params.EntryID, "label": params.Label})
	case "host.session.set_name":
		var params hostSessionSetNameParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		if h.Session == nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active session")
		}
		if err := h.Session.SetSessionName(params.Name); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"name": params.Name})
	case "host.model.list":
		if h.Session == nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active session")
		}
		models := h.getAvailableModels()
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{
			"models": models,
			"current": map[string]string{
				"provider": h.Session.Agent.State.Model.Provider,
				"modelId":  h.Session.Agent.State.Model.ID,
			},
			"auth": h.modelAuthStatuses(models),
		})
	case "host.model.select":
		var params hostModelSelectParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		model, err := h.SetModel(params.Provider, params.ModelID)
		if err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"model": model, "thinkingLevel": h.Session.Agent.State.ThinkingLevel})
	case "host.thinking.get":
		if h.Session == nil || h.Session.Agent == nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active session")
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"thinkingLevel": h.Session.Agent.State.ThinkingLevel})
	case "host.thinking.set":
		var params hostThinkingSetParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		if err := h.SetThinkingLevel(params.Level); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"thinkingLevel": h.Session.Agent.State.ThinkingLevel})
	case "host.agent.send_user_message":
		var params hostAgentSendUserMessageParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		delivered, err := h.sendUserMessageHostAction(params)
		if err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"deliveredAs": delivered})
	case "host.agent.run", "host.agent.spawn":
		var params hostAgentRunParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		result, err := h.runChildAgentHostAction(params, request.Method == "host.agent.spawn")
		if err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, result)
	case "host.agent.abort":
		var params hostAgentAbortParams
		if len(request.Params) > 0 {
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
			}
		}
		if shouldAbortChildAgents(params) {
			aborted, err := h.abortChildAgentHostAction(params)
			if err != nil {
				return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
			}
			return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"aborted": aborted, "target": "children"})
		}
		if h.Session == nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active session")
		}
		if err := h.Session.Abort(); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "internal_error", err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"aborted": true})
	case "host.session.action":
		var params hostSessionActionParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		result, err := h.sessionActionHostAction(params)
		if err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, result)
	case "host.fs.read":
		var params hostFSReadParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		result, err := h.readHostActionFile(params.Path)
		if err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, result)
	case "host.fs.write":
		var params hostFSWriteParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		result, err := h.writeHostActionFile(params.Path, params.Content)
		if err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, result)
	case "host.process.exec":
		var params hostProcessExecParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		result, err := h.execHostActionProcess(params.Command, params.CWD, HostProcessOptions{
			Timeout: time.Duration(params.TimeoutMillis) * time.Millisecond,
			Context: ctx,
		})
		if err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, result)
	case "host.policy.request":
		var params hostPolicyRequestParams
		if len(request.Params) > 0 {
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
			}
		}
		result, err := h.requestHostPolicyGrant(HostPolicyRequest(params))
		if err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, result)
	case "host.tui.dialog":
		var params hostTUIDialogParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		if h.TUIDialog == nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active TUI dialog host")
		}
		result, err := h.TUIDialog.RunTUIDialog(TUIDialogRequest(params))
		if err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, result)
	case "host.tui.mount":
		var params hostTUIMountParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		if h.ViewTreeHost == nil {
			h.ViewTreeHost = NewViewTreeHost()
		}
		overlay := params.OverlayOptions
		if overlay == nil {
			overlay = params.Overlay
		}
		if err := h.ViewTreeHost.MountWithOptions(params.MountID, params.Slot, params.View, ViewTreeMountOptions{Priority: params.Priority, Overlay: overlay}); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"mounted": true, "mountId": params.MountID})
	case "host.tui.patch":
		var params hostTUIPatchParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		if h.ViewTreeHost == nil {
			h.ViewTreeHost = NewViewTreeHost()
		}
		if err := h.ViewTreeHost.Patch(params.MountID, params.Ops); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"patched": true, "mountId": params.MountID})
	case "host.tui.unmount":
		var params hostTUIUnmountParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		unmounted := false
		if h.ViewTreeHost != nil {
			unmounted = h.ViewTreeHost.Unmount(params.MountID)
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"unmounted": unmounted, "mountId": params.MountID})
	case "host.tui.status":
		var params hostTUIStatusParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		if h.ViewTreeHost == nil {
			h.ViewTreeHost = NewViewTreeHost()
		}
		if err := h.ViewTreeHost.SetStatus(params.Key, params.Text, params.Priority); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		if h.TUIStatus != nil {
			if err := h.TUIStatus.SetTUIStatus(params.Key, params.Text); err != nil {
				return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
			}
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"updated": true, "key": params.Key})
	case "host.tui.title":
		var params hostTUITitleParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		if h.TUITitle == nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active TUI title host")
		}
		if err := h.TUITitle.SetTUITitle(params.Title); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"updated": true, "title": params.Title})
	case "host.tui.working":
		var params hostTUIWorkingParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		if h.TUIWorking == nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active TUI working host")
		}
		update := TUIWorkingUpdate{
			ResetMessage:   params.ResetMessage,
			ResetIndicator: params.ResetIndicator,
		}
		if params.Message != nil {
			update.Message = *params.Message
			update.MessageSet = true
		}
		if params.Visible != nil {
			update.Visible = *params.Visible
			update.VisibleSet = true
		}
		if params.Indicator != nil {
			update.Indicator = *params.Indicator
			update.IndicatorSet = true
		}
		if err := h.TUIWorking.SetTUIWorking(update); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"updated": true})
	case "host.tui.thinking_label":
		var params hostTUIThinkingLabelParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		if h.TUIThinkingLabel == nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active TUI thinking-label host")
		}
		label := params.Label
		if params.Reset {
			label = ""
		}
		if err := h.TUIThinkingLabel.SetHiddenThinkingLabel(label); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"updated": true, "label": label})
	case "host.tui.theme":
		var params hostTUIThemeParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		if h.TUITheme == nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active TUI theme host")
		}
		action := strings.TrimSpace(params.Action)
		if action == "" {
			action = "current"
		}
		switch action {
		case "current":
			return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{
				"current": h.TUITheme.CurrentTUITheme(),
				"themes":  h.TUITheme.AvailableTUIThemes(),
			})
		case "list", "getAll", "get_all":
			return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{
				"themes": h.TUITheme.AvailableTUIThemes(),
			})
		case "get":
			theme, ok := findTUIThemeInfo(h.TUITheme.AvailableTUIThemes(), params.Name)
			if !ok {
				return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"theme": nil})
			}
			return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"theme": theme})
		case "set":
			name := strings.TrimSpace(params.Name)
			if name == "" {
				return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{
					"success": false,
					"error":   "theme name is required",
				})
			}
			if _, ok := findTUIThemeInfo(h.TUITheme.AvailableTUIThemes(), name); !ok {
				return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{
					"success": false,
					"theme":   name,
					"error":   "unknown theme: " + name,
				})
			}
			if err := h.TUITheme.SetTUITheme(name); err != nil {
				return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
			}
			return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{
				"success": true,
				"theme":   name,
				"current": h.TUITheme.CurrentTUITheme(),
			})
		default:
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "unsupported theme action: "+action)
		}
	case "host.tui.tools_expanded":
		var params hostTUIToolsExpandedParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		if h.TUIToolExpansion == nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active TUI tool expansion host")
		}
		if params.Expanded != nil {
			if err := h.TUIToolExpansion.SetTUIToolsExpanded(*params.Expanded); err != nil {
				return hostActionErrorResponse(request.ID, request.Protocol, request.Method, hostActionErrorCode(err.Error()), err.Error())
			}
		}
		return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"expanded": h.TUIToolExpansion.TUIToolsExpanded()})
	case "host.tui.editor":
		var params hostTUIEditorParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
		}
		if h.TUIEditor == nil {
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active TUI editor")
		}
		action := strings.TrimSpace(params.Action)
		if action == "" {
			action = "read"
		}
		switch action {
		case "read":
			return hostActionSuccessResponse(request.ID, request.Protocol, tuiEditorState(h.TUIEditor))
		case "cursor":
			result := tuiEditorState(h.TUIEditor)
			if _, ok := result["cursor"]; !ok {
				result["cursor"] = nil
			}
			return hostActionSuccessResponse(request.ID, request.Protocol, result)
		case "autocomplete_context", "autocompleteContext":
			result := tuiEditorState(h.TUIEditor)
			if _, ok := result["cursor"]; !ok {
				result["cursor"] = nil
			}
			text, _ := result["text"].(string)
			result["lines"] = strings.Split(text, "\n")
			result["force"] = params.Force
			if params.Trigger != "" {
				result["trigger"] = params.Trigger
			}
			return hostActionSuccessResponse(request.ID, request.Protocol, result)
		case "focus":
			focusHost, ok := h.TUIEditor.(TUIEditorFocusHost)
			if !ok {
				return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "unsupported_method", "active TUI editor does not support focus")
			}
			if err := focusHost.FocusEditor(); err != nil {
				return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
			}
			return hostActionSuccessResponse(request.ID, request.Protocol, tuiEditorState(h.TUIEditor))
		case "set":
			h.TUIEditor.SetEditorText(params.Text)
			result := tuiEditorState(h.TUIEditor)
			result["updated"] = true
			return hostActionSuccessResponse(request.ID, request.Protocol, result)
		case "insert":
			h.TUIEditor.InsertEditorText(params.Text)
			result := tuiEditorState(h.TUIEditor)
			result["updated"] = true
			return hostActionSuccessResponse(request.ID, request.Protocol, result)
		case "paste":
			pasteHost, ok := h.TUIEditor.(TUIEditorPasteHost)
			if ok {
				pasteHost.PasteEditorText(params.Text)
			} else {
				h.TUIEditor.InsertEditorText(params.Text)
			}
			result := tuiEditorState(h.TUIEditor)
			result["updated"] = true
			result["pasteSemantics"] = ok
			return hostActionSuccessResponse(request.ID, request.Protocol, result)
		case "submit":
			if err := h.TUIEditor.SubmitEditorText(); err != nil {
				return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", err.Error())
			}
			return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"submitted": true})
		default:
			return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "unsupported editor action: "+action)
		}
	default:
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "unsupported_method", "unsupported host action: "+request.Method)
	}
}

func findTUIThemeInfo(themes []TUIThemeInfo, name string) (TUIThemeInfo, bool) {
	name = strings.TrimSpace(name)
	for _, theme := range themes {
		if theme.Name == name {
			return theme, true
		}
	}
	return TUIThemeInfo{}, false
}

func tuiEditorState(host TUIEditorHost) map[string]any {
	result := map[string]any{"text": host.ReadEditorText()}
	if cursorHost, ok := host.(TUIEditorCursorHost); ok {
		line, col, available := cursorHost.EditorCursor()
		if available {
			result["cursor"] = map[string]int{"line": line, "col": col}
		}
	}
	if focusHost, ok := host.(TUIEditorFocusHost); ok {
		result["focused"] = focusHost.EditorFocused()
	}
	if customHost, ok := host.(TUIEditorCustomStateHost); ok {
		result["customEditorActive"] = customHost.EditorCustomActive()
	}
	return result
}

func (LocalHostProcessExecutor) ExecuteHostProcess(command []string, cwd string) (HostProcessResult, error) {
	return LocalHostProcessExecutor{}.ExecuteHostProcessWithOptions(command, cwd, HostProcessOptions{})
}

func (LocalHostProcessExecutor) ExecuteHostProcessWithOptions(command []string, cwd string, options HostProcessOptions) (HostProcessResult, error) {
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return HostProcessResult{}, errors.New("process command is required")
	}
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return HostProcessResult{ExitCode: -1, Killed: true}, nil
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = cwd
	configureHostProcessCommand(cmd)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return HostProcessResult{}, err
	}
	defer stdoutReader.Close()
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		return HostProcessResult{}, err
	}
	defer stderrReader.Close()
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	if err := cmd.Start(); err != nil {
		_ = stdoutWriter.Close()
		_ = stderrWriter.Close()
		return HostProcessResult{}, err
	}
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	readDone := make(chan struct{}, 2)
	go readHostProcessPipe(stdoutReader, &stdout, readDone)
	go readHostProcessPipe(stderrReader, &stderr, readDone)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	killed := false
	var timeoutC <-chan time.Time
	var timer *time.Timer
	if options.Timeout > 0 {
		timer = time.NewTimer(options.Timeout)
		defer timer.Stop()
		timeoutC = timer.C
	}
	select {
	case err = <-done:
	case <-timeoutC:
		killed = true
		err = waitAfterHostProcessCancel(done, cmd.Process)
	case <-ctx.Done():
		killed = true
		err = waitAfterHostProcessCancel(done, cmd.Process)
	}
	waitForHostProcessPipesOrClose(stdoutReader, stderrReader, readDone, hostProcessStdioGrace)

	result := HostProcessResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if killed {
		result.ExitCode = -1
		result.Killed = true
		return result, nil
	}
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
			return result, nil
		}
		return HostProcessResult{}, err
	}
	return result, nil
}

func readHostProcessPipe(pipe *os.File, output *bytes.Buffer, done chan<- struct{}) {
	defer func() { done <- struct{}{} }()
	buffer := make([]byte, 4096)
	for {
		n, err := pipe.Read(buffer)
		if n > 0 {
			output.Write(buffer[:n])
		}
		if err != nil {
			return
		}
	}
}

func waitForHostProcessPipesOrClose(stdout, stderr *os.File, readDone <-chan struct{}, grace time.Duration) {
	remaining := 2
	timer := time.NewTimer(grace)
	defer timer.Stop()
	for remaining > 0 {
		select {
		case <-readDone:
			remaining--
		case <-timer.C:
			_ = stdout.Close()
			_ = stderr.Close()
			for remaining > 0 {
				<-readDone
				remaining--
			}
		}
	}
}

func waitAfterHostProcessCancel(done <-chan error, process *os.Process) error {
	_ = terminateHostProcess(process)
	forceTimer := time.NewTimer(hostProcessForceKillDelay)
	defer forceTimer.Stop()
	select {
	case err := <-done:
		return err
	case <-forceTimer.C:
		_ = killHostProcess(process)
		return <-done
	}
}

func (h *RPCSessionHost) execHostActionProcess(command []string, cwd string, options HostProcessOptions) (HostProcessResult, error) {
	if h == nil || h.Session == nil || h.Session.SessionManager == nil {
		return HostProcessResult{}, errors.New("no active session")
	}
	if h.ProcessExecutor == nil {
		return HostProcessResult{}, errors.New("policy denied: process execution is not enabled")
	}
	resolvedCWD := h.Session.SessionManager.GetCWD()
	if strings.TrimSpace(cwd) != "" {
		var err error
		resolvedCWD, err = h.resolveHostActionPath(cwd)
		if err != nil {
			return HostProcessResult{}, err
		}
	}
	if executor, ok := h.ProcessExecutor.(HostProcessExecutorWithOptions); ok {
		return executor.ExecuteHostProcessWithOptions(command, resolvedCWD, options)
	}
	return h.ProcessExecutor.ExecuteHostProcess(command, resolvedCWD)
}

func (h *RPCSessionHost) modelAuthStatuses(models []llm.Model) map[string]AuthStatus {
	auth := map[string]AuthStatus{}
	seen := map[string]bool{}
	for _, model := range models {
		provider := strings.TrimSpace(model.Provider)
		if provider == "" || seen[provider] {
			continue
		}
		seen[provider] = true
		if h != nil && h.ProviderAuthStatus != nil {
			auth[provider] = h.ProviderAuthStatus(provider)
			continue
		}
		auth[provider] = AuthStatus{Configured: false}
	}
	return auth
}

func (h *RPCSessionHost) requestHostPolicyGrant(request HostPolicyRequest) (HostPolicyDecision, error) {
	request.Capabilities = hostPolicyRequestedCapabilities(request)
	request.Capability = ""
	if len(request.Capabilities) == 0 {
		return HostPolicyDecision{}, errors.New("policy denied: at least one capability is required")
	}
	for _, capability := range request.Capabilities {
		if !isSupportedExtensionCapability(capability) {
			return HostPolicyDecision{}, errors.New("unsupported capability: " + capability)
		}
	}
	if h == nil || h.PolicyRequester == nil {
		return HostPolicyDecision{
			Granted:            false,
			DeniedCapabilities: append([]string(nil), request.Capabilities...),
			Reason:             "runtime policy grants are not configured",
			Persistent:         request.Persistent,
		}, nil
	}
	decision, err := h.PolicyRequester.RequestHostPolicyGrant(request)
	if err != nil {
		return HostPolicyDecision{}, err
	}
	return normalizeHostPolicyDecision(request, decision), nil
}

func hostPolicyRequestedCapabilities(request HostPolicyRequest) []string {
	seen := map[string]bool{}
	var capabilities []string
	for _, capability := range append([]string{request.Capability}, request.Capabilities...) {
		capability = strings.TrimSpace(capability)
		if capability == "" || seen[capability] {
			continue
		}
		seen[capability] = true
		capabilities = append(capabilities, capability)
	}
	return capabilities
}

func normalizeHostPolicyDecision(request HostPolicyRequest, decision HostPolicyDecision) HostPolicyDecision {
	requested := stringSet(request.Capabilities)
	granted := cleanSupportedCapabilities(decision.GrantedCapabilities)
	if decision.Granted && len(granted) == 0 {
		granted = append([]string(nil), request.Capabilities...)
	}
	var denied []string
	for _, capability := range request.Capabilities {
		if !capabilityGrantCoversRequest(capability, granted) {
			denied = append(denied, capability)
		}
	}
	for _, capability := range cleanSupportedCapabilities(decision.DeniedCapabilities) {
		if requested[capability] && !capabilityGrantCoversRequest(capability, granted) && !containsString(denied, capability) {
			denied = append(denied, capability)
		}
	}
	decision.GrantedCapabilities = granted
	decision.DeniedCapabilities = denied
	decision.Granted = len(granted) > 0 && len(denied) == 0
	if !decision.Persistent {
		decision.Persistent = request.Persistent
	}
	return decision
}

func capabilityGrantCoversRequest(requested string, granted []string) bool {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return true
	}
	for _, grant := range granted {
		grant = strings.TrimSpace(grant)
		if grant == requested {
			return true
		}
		if strings.HasSuffix(grant, ":") && strings.HasPrefix(requested, grant) {
			return true
		}
	}
	return false
}

func cleanSupportedCapabilities(capabilities []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, capability := range capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" || seen[capability] || !isSupportedExtensionCapability(capability) {
			continue
		}
		seen[capability] = true
		result = append(result, capability)
	}
	return result
}

func (h *RPCSessionHost) readHostActionFile(path string) (map[string]any, error) {
	absolutePath, err := h.resolveHostActionPath(path)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(absolutePath)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": absolutePath, "content": string(content), "bytes": len(content)}, nil
}

func (h *RPCSessionHost) writeHostActionFile(path, content string) (map[string]any, error) {
	absolutePath, err := h.resolveHostActionPath(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(absolutePath, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{"path": absolutePath, "bytes": len(content)}, nil
}

func (h *RPCSessionHost) resolveHostActionPath(path string) (string, error) {
	if h == nil || h.Session == nil || h.Session.SessionManager == nil {
		return "", errors.New("no active session")
	}
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is required")
	}
	cwd := h.Session.SessionManager.GetCWD()
	absolutePath := ResolveToCwd(path, cwd)
	relative, err := filepath.Rel(cwd, absolutePath)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("policy denied: path is outside session cwd")
	}
	return absolutePath, nil
}

func (h *RPCSessionHost) sendUserMessageHostAction(params hostAgentSendUserMessageParams) (string, error) {
	if h == nil || h.Session == nil {
		return "", errors.New("no active session")
	}
	text := firstNonEmptyString(params.Text, params.Message)
	switch strings.TrimSpace(params.DeliverAs) {
	case "", "prompt":
		return "prompt", h.Session.Prompt(text)
	case "steer":
		return "steer", h.Session.QueueExtensionUserMessage(text, "steer")
	case "followUp":
		return "followUp", h.Session.QueueExtensionUserMessage(text, "followUp")
	default:
		return "", errors.New("unsupported delivery mode: " + params.DeliverAs)
	}
}

func (h *RPCSessionHost) runChildAgentHostAction(params hostAgentRunParams, persisted bool) (map[string]any, error) {
	if h == nil || h.Session == nil || h.Session.SessionManager == nil || h.Session.Agent == nil {
		return nil, errors.New("no active session")
	}
	prompt := strings.TrimSpace(firstNonEmptyString(params.Prompt, params.Task, params.Message))
	if prompt == "" {
		return nil, errors.New("prompt is required")
	}
	cwd, err := h.resolveHostActionCWD(params.CWD)
	if err != nil {
		return nil, err
	}
	var manager *SessionManager
	if persisted {
		parentSessionDir := h.Session.SessionManager.GetSessionDir()
		if strings.TrimSpace(parentSessionDir) == "" {
			manager, err = InMemorySessionManager(cwd)
		} else {
			manager, err = CreateSessionManager(cwd, parentSessionDir)
		}
		if err != nil {
			return nil, err
		}
		if manager.IsPersisted() {
			parentSession := firstNonEmptyString(params.ParentSession, h.Session.SessionManager.GetSessionFile())
			if _, err := manager.NewSession(NewSessionOptions{ParentSession: parentSession}); err != nil {
				return nil, err
			}
		}
	} else {
		manager, err = InMemorySessionManager(cwd)
		if err != nil {
			return nil, err
		}
	}
	child, err := h.newChildAgentSession(manager, params)
	if err != nil {
		return nil, err
	}
	childKeys := h.registerChildAgent(manager.GetSessionID(), manager.GetSessionFile(), child)
	defer h.unregisterChildAgent(childKeys)
	if strings.TrimSpace(params.Name) != "" {
		_ = child.SetSessionName(params.Name)
	}
	statusKey := "agent:" + manager.GetSessionID()
	statusLabel := firstNonEmptyString(strings.TrimSpace(params.Name), prompt)
	progressEvents := make([]map[string]any, 0, 8)
	h.setChildAgentStatus(statusKey, statusLabel, "running")
	unsubscribe := child.Subscribe(func(event AgentSessionEvent) {
		if progress := childAgentProgressEvent(event); progress != nil {
			progressEvents = append(progressEvents, progress)
		}
		if status := childAgentStatusFromEvent(event); status != "" {
			h.setChildAgentStatus(statusKey, statusLabel, status)
		}
	})
	defer unsubscribe()
	if err := child.Prompt(prompt); err != nil {
		return nil, err
	}
	lastAssistantText := lastAssistantTextFromSession(child)
	aborted := lastAssistantAbortedFromSession(child)
	if aborted {
		h.setChildAgentStatus(statusKey, statusLabel, "aborted")
	} else {
		h.setChildAgentStatus(statusKey, statusLabel, "done")
	}
	return map[string]any{
		"sessionId":         manager.GetSessionID(),
		"sessionFile":       manager.GetSessionFile(),
		"persisted":         manager.IsPersisted(),
		"parentSessionFile": h.Session.SessionManager.GetSessionFile(),
		"lastAssistantText": lastAssistantText,
		"aborted":           aborted,
		"progressEvents":    progressEvents,
		"entries":           manager.GetEntries(),
		"stats":             child.GetSessionStats(),
	}, nil
}

func (h *RPCSessionHost) setChildAgentStatus(key, label, status string) {
	if h == nil || h.ViewTreeHost == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(status) == "" {
		return
	}
	text := "agent " + status
	if strings.TrimSpace(label) != "" {
		text = "agent " + status + ": " + gitui.TruncateToWidth(label, 40, "...")
	}
	_ = h.ViewTreeHost.SetStatus(key, text, 60)
}

func childAgentProgressEvent(event AgentSessionEvent) map[string]any {
	if event.Type == "" {
		return nil
	}
	progress := map[string]any{"type": event.Type}
	if event.ToolName != "" {
		progress["toolName"] = event.ToolName
	}
	if event.ToolCallID != "" {
		progress["toolCallId"] = event.ToolCallID
	}
	if event.ErrorMessage != "" {
		progress["errorMessage"] = event.ErrorMessage
	}
	if event.AssistantMessageEvent != nil {
		progress["messageEvent"] = event.AssistantMessageEvent.Type
	}
	return progress
}

func childAgentStatusFromEvent(event AgentSessionEvent) string {
	switch event.Type {
	case "agent_start", "turn_start":
		return "running"
	case "message_update":
		return "responding"
	case "tool_execution_start":
		if event.ToolName != "" {
			return "tool " + event.ToolName
		}
		return "tool"
	case "agent_end":
		return "done"
	default:
		return ""
	}
}

func shouldAbortChildAgents(params hostAgentAbortParams) bool {
	target := strings.TrimSpace(params.Target)
	return target == "child" || target == "children" || strings.TrimSpace(params.SessionID) != "" || strings.TrimSpace(params.SessionFile) != ""
}

func (h *RPCSessionHost) registerChildAgent(sessionID, sessionFile string, child *AgentSession) []string {
	if h == nil || child == nil {
		return nil
	}
	keys := make([]string, 0, 2)
	for _, key := range []string{strings.TrimSpace(sessionID), strings.TrimSpace(sessionFile)} {
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil
	}
	h.childMu.Lock()
	defer h.childMu.Unlock()
	if h.childAgents == nil {
		h.childAgents = map[string]*AgentSession{}
	}
	for _, key := range keys {
		h.childAgents[key] = child
	}
	return keys
}

func (h *RPCSessionHost) unregisterChildAgent(keys []string) {
	if h == nil || len(keys) == 0 {
		return
	}
	h.childMu.Lock()
	defer h.childMu.Unlock()
	for _, key := range keys {
		delete(h.childAgents, key)
	}
}

func (h *RPCSessionHost) abortChildAgentHostAction(params hostAgentAbortParams) (bool, error) {
	if h == nil {
		return false, errors.New("no active session")
	}
	keys := []string{strings.TrimSpace(params.SessionID), strings.TrimSpace(params.SessionFile)}
	h.childMu.Lock()
	var children []*AgentSession
	if keys[0] != "" || keys[1] != "" {
		for _, key := range keys {
			if key == "" {
				continue
			}
			if child := h.childAgents[key]; child != nil {
				children = append(children, child)
				break
			}
		}
	} else {
		seen := map[*AgentSession]struct{}{}
		for _, child := range h.childAgents {
			if child == nil {
				continue
			}
			if _, ok := seen[child]; ok {
				continue
			}
			seen[child] = struct{}{}
			children = append(children, child)
		}
	}
	h.childMu.Unlock()
	if len(children) == 0 {
		if keys[0] != "" || keys[1] != "" {
			return false, errors.New("no matching child agent")
		}
		return false, nil
	}
	for _, child := range children {
		if err := child.Abort(); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (h *RPCSessionHost) newChildAgentSession(manager *SessionManager, params hostAgentRunParams) (*AgentSession, error) {
	compactionSettings := h.Session.CompactionSettings
	retrySettings := h.Session.RetrySettings
	tools := append([]string(nil), h.Session.Tools...)
	toolsSet := h.Session.ToolsSet
	if params.Tools != nil {
		tools = append([]string(nil), params.Tools...)
		toolsSet = true
	}
	noTools := firstNonEmptyString(params.NoTools, h.Session.NoTools)
	return CreateAgentSession(AgentSessionOptions{
		CWD:                  manager.GetCWD(),
		SettingsManager:      h.Session.SettingsManager,
		SessionManager:       manager,
		ResourceLoader:       h.Session.ResourceLoader,
		Model:                h.Session.Agent.State.Model,
		ThinkingLevel:        h.Session.Agent.State.ThinkingLevel,
		Preflight:            h.Session.Preflight,
		CompactionSettings:   &compactionSettings,
		CompactionSummarizer: h.Session.CompactionSummarizer,
		BranchSummarizer:     h.Session.BranchSummarizer,
		RetrySettings:        &retrySettings,
		Responder:            h.Session.Responder,
		StreamResponder:      h.Session.StreamResponder,
		ModelRuntime:         h.Session.ModelRuntime,
		SummaryRuntime:       h.Session.SummaryRuntime,
		CustomTools:          append([]SDKTool(nil), h.Session.DynamicTools...),
		Tools:                tools,
		ToolsSet:             toolsSet,
		NoTools:              noTools,
	})
}

func (h *RPCSessionHost) resolveHostActionCWD(path string) (string, error) {
	if h == nil || h.Session == nil || h.Session.SessionManager == nil {
		return "", errors.New("no active session")
	}
	parentCWD := h.Session.SessionManager.GetCWD()
	if strings.TrimSpace(path) == "" {
		return parentCWD, nil
	}
	absolutePath := ResolveToCwd(path, parentCWD)
	relative, err := filepath.Rel(parentCWD, absolutePath)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("policy denied: child agent cwd is outside session cwd")
	}
	return absolutePath, nil
}

func lastAssistantTextFromSession(session *AgentSession) *string {
	if session == nil {
		return nil
	}
	messages := session.Messages()
	for idx := len(messages) - 1; idx >= 0; idx-- {
		if messages[idx].Role != llm.RoleAssistant {
			continue
		}
		text := rpcMessageText(messages[idx])
		return &text
	}
	return nil
}

func lastAssistantAbortedFromSession(session *AgentSession) bool {
	if session == nil {
		return false
	}
	messages := session.Messages()
	for idx := len(messages) - 1; idx >= 0; idx-- {
		if messages[idx].Role != llm.RoleAssistant {
			continue
		}
		return messages[idx].StopReason == llm.StopReasonAborted
	}
	return false
}

func (h *RPCSessionHost) sessionActionHostAction(params hostSessionActionParams) (map[string]any, error) {
	if h == nil || h.Session == nil || h.Session.SessionManager == nil {
		return nil, errors.New("no active session")
	}
	switch strings.TrimSpace(params.Action) {
	case "new", "clear":
		if _, err := h.Session.SessionManager.NewSession(NewSessionOptions{ParentSession: params.ParentSession}); err != nil {
			return nil, err
		}
		h.Session.queues.clearPrompts()
		return map[string]any{"action": params.Action, "cancelled": false, "sessionFile": h.Session.SessionManager.GetSessionFile()}, nil
	case "fork":
		result, err := h.Fork(params.EntryID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"action": "fork", "cancelled": result.Cancelled, "text": result.Text, "sessionFile": h.Session.SessionManager.GetSessionFile()}, nil
	case "switch":
		result, err := h.SwitchSession(params.SessionFile)
		if err != nil {
			return nil, err
		}
		return map[string]any{"action": "switch", "cancelled": result.Cancelled, "sessionFile": h.Session.SessionManager.GetSessionFile()}, nil
	case "reload":
		if h.ReloadSession == nil {
			return nil, errors.New("reload is unavailable")
		}
		if err := h.ReloadSession(); err != nil {
			return nil, err
		}
		return map[string]any{"action": "reload", "cancelled": false, "reloaded": true, "sessionFile": h.Session.SessionManager.GetSessionFile()}, nil
	case "navigate_tree":
		targetID := firstNonEmptyString(params.TargetID, params.EntryID)
		if strings.TrimSpace(targetID) == "" {
			return nil, errors.New("navigate_tree requires targetId")
		}
		result, err := h.Session.NavigateTree(targetID, AgentSessionNavigateTreeOptions{
			Summarize:          params.Summarize,
			CustomInstructions: params.CustomInstructions,
		})
		if err != nil {
			return nil, err
		}
		response := map[string]any{
			"action":     "navigate_tree",
			"cancelled":  result.Cancelled,
			"aborted":    result.Aborted,
			"editorText": result.EditorText,
		}
		if result.SummaryEntry != nil {
			response["summaryEntryId"] = result.SummaryEntry.ID
		}
		if leaf := h.Session.SessionManager.GetLeafID(); leaf != nil {
			response["leafId"] = *leaf
		}
		return response, nil
	default:
		return nil, errors.New("unsupported session action: " + params.Action)
	}
}

func (h *RPCSessionHost) setActiveTools(mode string, toolNames []string) ([]string, error) {
	if h == nil || h.Session == nil {
		return nil, errors.New("no active session")
	}
	known := map[string]bool{}
	for _, tool := range hostActionRegisteredTools(h.Session) {
		known[tool.Name] = true
	}
	requested := cleanToolNames(toolNames)
	for _, name := range requested {
		if !known[name] {
			return nil, errors.New("unknown tool: " + name)
		}
	}
	current := h.Session.GetActiveToolNames()
	nextSet := map[string]bool{}
	switch firstNonEmptyString(strings.TrimSpace(mode), "replace") {
	case "replace":
		nextSet = stringSet(requested)
	case "add":
		nextSet = stringSet(current)
		for _, name := range requested {
			nextSet[name] = true
		}
	case "remove":
		nextSet = stringSet(current)
		for _, name := range requested {
			delete(nextSet, name)
		}
	default:
		return nil, errors.New("unsupported tools.set_active mode: " + mode)
	}
	h.Session.Tools = toolNamesInRegistryOrder(hostActionRegisteredTools(h.Session), nextSet)
	h.Session.ToolsSet = true
	h.Session.NoTools = ""
	h.Session.RefreshSystemPrompt()
	return h.Session.GetActiveToolNames(), nil
}

func (h *RPCSessionHost) appendCustomSessionEntry(params hostSessionAppendCustomParams) (string, error) {
	if h == nil || h.Session == nil || h.Session.SessionManager == nil {
		return "", errors.New("no active session")
	}
	owner := strings.TrimSpace(params.Owner)
	customType := strings.TrimSpace(params.Type)
	if owner == "" {
		return "", errors.New("custom entry owner is required")
	}
	if customType == "" {
		return "", errors.New("custom entry type is required")
	}
	return h.Session.AppendCustomEntry(customType, map[string]any{
		"owner": owner,
		"data":  params.Data,
	})
}

func hostActionRegisteredTools(session *AgentSession) []SDKTool {
	if session == nil || session.Agent == nil {
		return nil
	}
	tools := append([]SDKTool(nil), session.Agent.State.Tools...)
	tools = append(tools, session.DynamicTools...)
	return tools
}

func cleanToolNames(toolNames []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, name := range toolNames {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, name)
	}
	return result
}

func toolNamesInRegistryOrder(tools []SDKTool, allowed map[string]bool) []string {
	var result []string
	for _, tool := range tools {
		if allowed[tool.Name] {
			result = append(result, tool.Name)
		}
	}
	return result
}

func stringSet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}

func hostActionSuccessResponse(id, protocol string, result any) HostActionResponse {
	return HostActionResponse{Type: "response", Protocol: firstNonEmptyString(protocol, "gi-ext-rpc@1"), ID: id, Result: result}
}

func hostActionErrorResponse(id, protocol, _ string, code, message string) HostActionResponse {
	return HostActionResponse{
		Type:     "response",
		Protocol: firstNonEmptyString(protocol, "gi-ext-rpc@1"),
		ID:       id,
		Error:    &HostActionError{Code: code, Message: message},
	}
}

func hostActionErrorCode(message string) string {
	if strings.Contains(message, "stale viewtree mount") {
		return "stale_context"
	}
	if strings.Contains(message, "no active session") {
		return "stale_context"
	}
	if strings.Contains(message, "no active TUI") {
		return "stale_context"
	}
	if strings.Contains(message, "policy denied") {
		return "policy_denied"
	}
	if strings.Contains(message, "timeout") {
		return "timeout"
	}
	return "invalid_params"
}
