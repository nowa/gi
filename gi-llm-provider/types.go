package gillmprovider

import (
	"context"
	"time"
)

const (
	RoleUser       = "user"
	RoleAssistant  = "assistant"
	RoleToolResult = "toolResult"

	ContentText     = "text"
	ContentThinking = "thinking"
	ContentImage    = "image"
	ContentToolCall = "toolCall"

	StopReasonStop    = "stop"
	StopReasonLength  = "length"
	StopReasonError   = "error"
	StopReasonAborted = "aborted"
)

type UsageCost struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
	Total      float64
}

type Usage struct {
	Input       int
	Output      int
	CacheRead   int
	CacheWrite  int
	TotalTokens int
	Cost        UsageCost
}

func EmptyUsage() Usage {
	return Usage{Cost: UsageCost{}}
}

type ContentPart struct {
	Type              string
	Text              string
	TextSignature     string
	Thinking          string
	ThinkingSignature string
	Redacted          bool
	Data              string
	MIMEType          string
	ID                string
	Name              string
	Arguments         map[string]any
	ThoughtSignature  string
}

func Text(text string) ContentPart {
	return ContentPart{Type: ContentText, Text: text}
}

func Thinking(thinking string) ContentPart {
	return ContentPart{Type: ContentThinking, Thinking: thinking}
}

func Image(data, mimeType string) ContentPart {
	return ContentPart{Type: ContentImage, Data: data, MIMEType: mimeType}
}

func ToolCall(id, name string, args map[string]any) ContentPart {
	if args == nil {
		args = map[string]any{}
	}
	return ContentPart{Type: ContentToolCall, ID: id, Name: name, Arguments: args}
}

type Message struct {
	Role         string
	Content      []ContentPart
	Timestamp    int64
	API          string
	Provider     string
	Model        string
	Usage        Usage
	StopReason   string
	ErrorMessage string
	ResponseID   string
	ToolCallID   string
	ToolName     string
	Details      any
	IsError      bool
}

func NowMillis() int64 {
	return time.Now().UnixMilli()
}

func UserMessageText(text string) Message {
	return Message{
		Role:      RoleUser,
		Content:   []ContentPart{Text(text)},
		Timestamp: NowMillis(),
	}
}

func AssistantMessage(content []ContentPart, stopReason string, model Model) Message {
	if stopReason == "" {
		stopReason = StopReasonStop
	}
	return Message{
		Role:       RoleAssistant,
		Content:    content,
		API:        model.API,
		Provider:   model.Provider,
		Model:      model.ID,
		Usage:      EmptyUsage(),
		StopReason: stopReason,
		Timestamp:  NowMillis(),
	}
}

func AssistantErrorMessage(message string, model Model, aborted bool) Message {
	stopReason := StopReasonError
	if aborted {
		stopReason = StopReasonAborted
	}
	return Message{
		Role:         RoleAssistant,
		Content:      []ContentPart{Text("")},
		API:          model.API,
		Provider:     model.Provider,
		Model:        model.ID,
		Usage:        EmptyUsage(),
		StopReason:   stopReason,
		ErrorMessage: message,
		Timestamp:    NowMillis(),
	}
}

type ModelCost struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
}

type Model struct {
	ID               string
	Name             string
	API              string
	Provider         string
	BaseURL          string
	Headers          map[string]string
	Compat           ModelCompat
	Reasoning        bool
	Input            []string
	Cost             ModelCost
	ContextWindow    int
	MaxTokens        int
	ThinkingLevelMap map[string]*string
}

type ModelCompat struct {
	SupportsStore                             *bool
	SupportsDeveloperRole                     *bool
	SupportsReasoningEffort                   *bool
	SupportsUsageInStreaming                  *bool
	SupportsStrictMode                        *bool
	SupportsLongCacheRetention                *bool
	SupportsEagerToolInputStreaming           *bool
	SupportsCacheControlOnTools               *bool
	SendSessionAffinityHeaders                *bool
	SendSessionIDHeader                       *bool
	RequiresToolResultName                    *bool
	RequiresAssistantAfterToolResult          *bool
	RequiresThinkingAsText                    *bool
	RequiresReasoningContentOnAssistantTurns  *bool
	RequiresReasoningContentOnAssistantEvents *bool
	ZAIToolStream                             *bool
	MaxTokensField                            string
	ThinkingFormat                            string
	CacheControlFormat                        string
}

type Context struct {
	SystemPrompt string
	Messages     []Message
	Tools        []Tool
}

type StreamOptions struct {
	Context          context.Context
	Temperature      *float64
	MaxTokens        int
	APIKey           string
	Transport        string
	CacheRetention   string
	SessionID        string
	Reasoning        string
	ThinkingBudgets  map[string]int
	Headers          map[string]string
	TimeoutMillis    int
	MaxRetries       int
	MaxRetryDelayMs  int
	Metadata         map[string]any
	OnPayload        func(payload any, model Model) (any, bool, error)
	OnResponseStatus func(status int, headers map[string]string, model Model) error
}

type SimpleStreamOptions = StreamOptions

type AssistantMessageEvent struct {
	Type    string
	Partial Message
	Message Message
	Error   Message
	Reason  string
}

type Tool struct {
	Name        string
	Description string
	Parameters  Schema
}

type Schema struct {
	Type       any
	Properties map[string]Schema
	Required   []string
	Items      *Schema
	Enum       []any
}

func Object(properties map[string]Schema, required ...string) Schema {
	return Schema{Type: "object", Properties: properties, Required: required}
}

func String() Schema  { return Schema{Type: "string"} }
func Number() Schema  { return Schema{Type: "number"} }
func Integer() Schema { return Schema{Type: "integer"} }
func Boolean() Schema { return Schema{Type: "boolean"} }
func Null() Schema    { return Schema{Type: "null"} }

func TypeUnion(types ...string) Schema {
	values := make([]any, len(types))
	for i, t := range types {
		values[i] = t
	}
	return Schema{Type: values}
}
