package gicodingagent

import (
	"context"
	"errors"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	RPCCommandPrompt               = "prompt"
	RPCCommandNewSession           = "new_session"
	RPCCommandGetState             = "get_state"
	RPCCommandSetThinkingLevel     = "set_thinking_level"
	RPCCommandCycleThinkingLevel   = "cycle_thinking_level"
	RPCCommandGetAvailableModels   = "get_available_models"
	RPCCommandCompact              = "compact"
	RPCCommandBash                 = "bash"
	RPCCommandGetSessionStats      = "get_session_stats"
	RPCCommandExportHTML           = "export_html"
	RPCCommandGetLastAssistantText = "get_last_assistant_text"
	RPCCommandSetSessionName       = "set_session_name"
)

type RPCSessionHost struct {
	Session               *AgentSession
	SteeringMode          string
	FollowUpMode          string
	AutoCompactionEnabled bool
	AvailableModels       []llm.Model
	PromptPreflight       func(RPCCommand) error
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

type RPCSessionStats struct {
	SessionFile       string             `json:"sessionFile,omitempty"`
	SessionID         string             `json:"sessionId"`
	UserMessages      int                `json:"userMessages"`
	AssistantMessages int                `json:"assistantMessages"`
	Tokens            llm.Usage          `json:"tokens"`
	ContextUsage      *AgentContextUsage `json:"contextUsage,omitempty"`
}

type RPCExportHTMLResult struct {
	Path string `json:"path"`
}

type RPCLastAssistantTextResult struct {
	Text *string `json:"text"`
}

func NewRPCSessionHost(session *AgentSession) *RPCSessionHost {
	host := &RPCSessionHost{
		Session:               session,
		SteeringMode:          "all",
		FollowUpMode:          "all",
		AutoCompactionEnabled: true,
	}
	if session != nil {
		host.AutoCompactionEnabled = session.CompactionSettings.Enabled
	}
	return host
}

func (h *RPCSessionHost) HandleCommand(ctx context.Context, command RPCCommand) RPCResponse {
	result, err := h.handleCommand(ctx, command)
	if err != nil {
		return rpcErrorResponse(command.Type, err)
	}
	return rpcSuccessResponse(command.Type, result)
}

func (h *RPCSessionHost) handleCommand(ctx context.Context, command RPCCommand) (any, error) {
	if h == nil || h.Session == nil || h.Session.SessionManager == nil {
		return nil, errors.New("RPC session host requires an active session")
	}
	switch command.Type {
	case RPCCommandPrompt:
		if err := h.runPromptPreflight(command); err != nil {
			return nil, err
		}
		return nil, h.Session.Prompt(command.Message)
	case RPCCommandNewSession:
		h.Session.SessionManager.NewSession(NewSessionOptions{ParentSession: command.ParentSession})
		h.Session.steeringMessages = nil
		h.Session.followUpMessages = nil
		return RPCCloneResult{Cancelled: false}, nil
	case RPCCommandGetState:
		return h.GetState(), nil
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
	case RPCCommandCompact:
		return h.Session.Compact(command.CustomInstructions)
	case RPCCommandBash:
		return h.Bash(ctx, command.Command)
	case RPCCommandGetSessionStats:
		return h.GetSessionStats(), nil
	case RPCCommandExportHTML:
		path, err := h.ExportHTML(command.OutputPath)
		if err != nil {
			return nil, err
		}
		return RPCExportHTMLResult{Path: path}, nil
	case RPCCommandGetLastAssistantText:
		return RPCLastAssistantTextResult{Text: h.GetLastAssistantText()}, nil
	case RPCCommandSetSessionName:
		h.Session.SessionManager.AppendSessionInfo(command.Name)
		return nil, nil
	case RPCCommandClone:
		return RPCCloneResult{Cancelled: false}, nil
	default:
		return nil, errors.New("unsupported RPC command: " + command.Type)
	}
}

func (h *RPCSessionHost) AcceptPrompt(command RPCCommand) error {
	if h == nil || h.Session == nil {
		return errors.New("RPC session host requires an active session")
	}
	if strings.TrimSpace(command.Message) == "" {
		return errors.New("prompt is required")
	}
	if h.Session.IsStreaming() {
		switch command.StreamingBehavior {
		case "steer":
			return h.Session.Steer(command.Message)
		case "followUp":
			return h.Session.FollowUp(command.Message)
		default:
			return errors.New("Agent is already processing. Specify streamingBehavior ('steer' or 'followUp') to queue the message.")
		}
	}
	if err := h.runPromptPreflight(command); err != nil {
		return err
	}
	go func() {
		_ = h.Session.Prompt(command.Message)
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
	session := h.Session
	manager := session.SessionManager
	model := session.Agent.State.Model
	return RPCSessionState{
		Model:                 &model,
		ThinkingLevel:         session.Agent.State.ThinkingLevel,
		IsStreaming:           session.IsStreaming(),
		IsCompacting:          session.isCompacting,
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
	h.Session.Agent.State.ThinkingLevel = level
	h.Session.SessionManager.AppendThinkingLevelChange(level)
	return nil
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

func (h *RPCSessionHost) Bash(ctx context.Context, command string) (BashResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return BashResult{}, errors.New("bash command is required")
	}
	result, err := ExecuteBash(command, h.Session.SessionManager.GetCWD(), BashExecutorOptions{Context: ctx})
	h.Session.SessionManager.AppendMessage(map[string]any{
		"role":      "bashExecution",
		"command":   command,
		"output":    result.Output,
		"exitCode":  result.ExitCode,
		"cancelled": result.Cancelled,
		"content":   []any{map[string]any{"type": "text", "text": result.Output}},
		"timestamp": llm.NowMillis(),
	})
	return result, err
}

func (h *RPCSessionHost) GetSessionStats() RPCSessionStats {
	stats := h.Session.GetSessionStats()
	result := RPCSessionStats{
		SessionFile:  h.Session.SessionManager.GetSessionFile(),
		SessionID:    h.Session.SessionManager.GetSessionID(),
		Tokens:       stats.Tokens,
		ContextUsage: stats.ContextUsage,
	}
	for _, message := range h.Session.Messages() {
		switch message.Role {
		case llm.RoleUser:
			result.UserMessages++
		case llm.RoleAssistant:
			result.AssistantMessages++
		}
	}
	return result
}

func (h *RPCSessionHost) ExportHTML(outputPath string) (string, error) {
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
	var builder strings.Builder
	builder.WriteString("<!doctype html><html><body>\n")
	for _, message := range h.Session.Messages() {
		builder.WriteString(`<section data-role="`)
		builder.WriteString(html.EscapeString(message.Role))
		builder.WriteString(`">`)
		builder.WriteString(html.EscapeString(rpcMessageText(message)))
		builder.WriteString("</section>\n")
	}
	builder.WriteString("</body></html>\n")
	return builder.String()
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
