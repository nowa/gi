package giagentcore

import (
	"context"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	ToolExecutionSequential = "sequential"
	ToolExecutionParallel   = "parallel"

	QueueAll       = "all"
	QueueOneAtTime = "one-at-a-time"
)

type AgentMessage = llm.Message
type AgentToolCall = llm.ContentPart

type StreamFn func(model llm.Model, llmContext llm.Context, options llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error)

type AgentToolResult struct {
	Content   []llm.ContentPart
	Details   any
	Terminate bool
}

type AgentToolUpdateCallback func(partialResult AgentToolResult)

type AgentTool struct {
	Name        string
	Label       string
	Description string
	Parameters  llm.Schema

	PrepareArguments func(args any) (map[string]any, error)
	Execute          func(ctx context.Context, toolCallID string, params map[string]any, onUpdate AgentToolUpdateCallback) (AgentToolResult, error)
	ExecutionMode    string
}

func (t AgentTool) AsLLMTool() llm.Tool {
	return llm.Tool{Name: t.Name, Description: t.Description, Parameters: t.Parameters}
}

type BeforeToolCallResult struct {
	Block  bool
	Reason string
}

type AfterToolCallResult struct {
	Content      []llm.ContentPart
	HasContent   bool
	Details      any
	HasDetails   bool
	IsError      bool
	HasIsError   bool
	Terminate    bool
	HasTerminate bool
}

type BeforeToolCallContext struct {
	AssistantMessage llm.Message
	ToolCall         AgentToolCall
	Args             map[string]any
	Context          AgentContext
}

type AfterToolCallContext struct {
	AssistantMessage llm.Message
	ToolCall         AgentToolCall
	Args             map[string]any
	Result           AgentToolResult
	IsError          bool
	Context          AgentContext
}

type ShouldStopAfterTurnContext struct {
	Message     llm.Message
	ToolResults []llm.Message
	Context     AgentContext
	NewMessages []llm.Message
}

type PrepareNextTurnContext = ShouldStopAfterTurnContext

type AgentLoopTurnUpdate struct {
	Context       *AgentContext
	Model         *llm.Model
	ThinkingLevel *string
}

type AgentLoopConfig struct {
	llm.SimpleStreamOptions
	Model llm.Model

	ConvertToLLM     func(messages []llm.Message) ([]llm.Message, error)
	TransformContext func(ctx context.Context, messages []llm.Message) ([]llm.Message, error)
	GetAPIKey        func(provider string) string

	ShouldStopAfterTurn func(ShouldStopAfterTurnContext) (bool, error)
	PrepareNextTurn     func(PrepareNextTurnContext) (AgentLoopTurnUpdate, bool, error)
	GetSteeringMessages func() ([]llm.Message, error)
	GetFollowUpMessages func() ([]llm.Message, error)

	ToolExecution  string
	BeforeToolCall func(context.Context, BeforeToolCallContext) (BeforeToolCallResult, error)
	AfterToolCall  func(context.Context, AfterToolCallContext) (AfterToolCallResult, error)
}

type AgentContext struct {
	SystemPrompt string
	Messages     []llm.Message
	Tools        []AgentTool
}

type AgentEvent struct {
	Type                  string
	Messages              []llm.Message
	Message               llm.Message
	ToolResults           []llm.Message
	AssistantMessageEvent llm.AssistantMessageEvent
	ToolCallID            string
	ToolName              string
	Args                  map[string]any
	PartialResult         any
	Result                any
	IsError               bool
}
