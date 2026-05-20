package gicodingagent

import (
	"os"
	"path/filepath"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	FauxProvider = "faux"
	FauxModelID  = "faux-1"
	FauxAPI      = "anthropic-messages"
)

var FauxModel = llm.Model{
	ID:            FauxModelID,
	Name:          "Faux Model",
	API:           FauxAPI,
	Provider:      FauxProvider,
	BaseURL:       "http://localhost:0",
	Reasoning:     false,
	Input:         []string{"text", "image"},
	ContextWindow: 128000,
	MaxTokens:     16384,
}

type CodingAgentTestHarnessResponse struct {
	Text       *string
	Thinking   string
	ToolCalls  []CodingAgentTestHarnessToolCall
	StopReason string
	Error      string
	Usage      *llm.Usage
	Model      *CodingAgentTestHarnessModelOverride
}

type CodingAgentTestHarnessToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

type CodingAgentTestHarnessModelOverride struct {
	Provider string
	ID       string
}

type CodingAgentTestHarnessOptions struct {
	Responses     []CodingAgentTestHarnessResponse
	Model         llm.Model
	RetrySettings *AgentSessionRetrySettings
	Tools         []SDKTool
}

type CodingAgentFauxState struct {
	CallCount int
	Contexts  [][]llm.Message
}

type CodingAgentTestHarness struct {
	Session        *AgentSession
	SessionManager *SessionManager
	Faux           *CodingAgentFauxState
	Events         []AgentSessionEvent
	TempDir        string
}

func NewCodingAgentTestHarness(options CodingAgentTestHarnessOptions) (*CodingAgentTestHarness, error) {
	tempDir, err := os.MkdirTemp("", "gi-harness-*")
	if err != nil {
		return nil, err
	}
	sessionManager, err := InMemorySessionManager(tempDir)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, err
	}
	state := &CodingAgentFauxState{}
	responses := options.Responses
	if len(responses) == 0 {
		responses = []CodingAgentTestHarnessResponse{TextHarnessResponse("ok")}
	}
	model := options.Model
	if model.ID == "" {
		model = FauxModel
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            tempDir,
		AgentDir:       filepath.Join(tempDir, ".pi", "agent"),
		Model:          model,
		SessionManager: sessionManager,
		RetrySettings:  options.RetrySettings,
		Responder:      makeCodingAgentHarnessResponder(responses, state, model),
	})
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, err
	}
	if options.Tools != nil {
		session.Agent.State.Tools = append([]SDKTool(nil), options.Tools...)
	}
	harness := &CodingAgentTestHarness{
		Session:        session,
		SessionManager: sessionManager,
		Faux:           state,
		TempDir:        tempDir,
	}
	session.Subscribe(func(event AgentSessionEvent) {
		harness.Events = append(harness.Events, event)
	})
	return harness, nil
}

func TextHarnessResponse(text string) CodingAgentTestHarnessResponse {
	return CodingAgentTestHarnessResponse{Text: &text}
}

func (h *CodingAgentTestHarness) Cleanup() {
	if h == nil {
		return
	}
	if h.Session != nil {
		h.Session.Dispose()
	}
	if h.TempDir != "" {
		_ = os.RemoveAll(h.TempDir)
	}
}

func (h *CodingAgentTestHarness) EventsOfType(eventType string) []AgentSessionEvent {
	if h == nil {
		return nil
	}
	events := []AgentSessionEvent{}
	for _, event := range h.Events {
		if event.Type == eventType {
			events = append(events, event)
		}
	}
	return events
}

func (h *CodingAgentTestHarness) Messages() []llm.Message {
	if h == nil || h.Session == nil {
		return nil
	}
	return h.Session.Messages()
}

func makeCodingAgentHarnessResponder(responses []CodingAgentTestHarnessResponse, state *CodingAgentFauxState, defaultModel llm.Model) AgentSessionResponder {
	return func(_ string, context []llm.Message, model llm.Model) (llm.Message, error) {
		index := state.CallCount % len(responses)
		state.CallCount++
		state.Contexts = append(state.Contexts, append([]llm.Message(nil), context...))
		response := responses[index]
		if model.ID == "" {
			model = defaultModel
		}
		return response.toMessage(model), nil
	}
}

func (r CodingAgentTestHarnessResponse) toMessage(model llm.Model) llm.Message {
	content := []llm.ContentPart{}
	if r.Thinking != "" {
		content = append(content, llm.Thinking(r.Thinking))
	}
	if r.Text != nil {
		content = append(content, llm.Text(*r.Text))
	}
	for _, toolCall := range r.ToolCalls {
		id := toolCall.ID
		if id == "" {
			id = "faux_tc_1"
		}
		content = append(content, llm.ToolCall(id, toolCall.Name, toolCall.Arguments))
	}
	stopReason := r.StopReason
	if stopReason == "" {
		switch {
		case r.Error != "":
			stopReason = llm.StopReasonError
		case len(r.ToolCalls) > 0:
			stopReason = "toolUse"
		default:
			stopReason = llm.StopReasonStop
		}
	}
	if len(content) == 0 && r.Error == "" {
		content = append(content, llm.Text(""))
	}
	usage := llm.Usage{Input: 100, Output: 50, TotalTokens: 150}
	if r.Usage != nil {
		usage = *r.Usage
		if usage.TotalTokens == 0 {
			usage.TotalTokens = usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
		}
	}
	provider := model.Provider
	modelID := model.ID
	if r.Model != nil {
		if r.Model.Provider != "" {
			provider = r.Model.Provider
		}
		if r.Model.ID != "" {
			modelID = r.Model.ID
		}
	}
	return llm.Message{
		Role:         llm.RoleAssistant,
		Content:      content,
		API:          model.API,
		Provider:     provider,
		Model:        modelID,
		Usage:        usage,
		StopReason:   stopReason,
		ErrorMessage: r.Error,
		Timestamp:    llm.NowMillis(),
	}
}
