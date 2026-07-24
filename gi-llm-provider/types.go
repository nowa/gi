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
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
	Total      float64 `json:"total"`
}

type Usage struct {
	Input        int       `json:"input"`
	Output       int       `json:"output"`
	CacheRead    int       `json:"cacheRead"`
	CacheWrite   int       `json:"cacheWrite"`
	CacheWrite1h int       `json:"cacheWrite1h,omitempty"`
	Reasoning    *int      `json:"reasoning,omitempty"`
	TotalTokens  int       `json:"totalTokens"`
	Cost         UsageCost `json:"cost"`
}

func EmptyUsage() Usage {
	return Usage{Cost: UsageCost{}}
}

type DiagnosticErrorInfo struct {
	Name    string `json:"name,omitempty"`
	Message string `json:"message"`
	Stack   string `json:"stack,omitempty"`
	Code    any    `json:"code,omitempty"`
}

type AssistantMessageDiagnostic struct {
	Type      string               `json:"type"`
	Timestamp int64                `json:"timestamp"`
	Error     *DiagnosticErrorInfo `json:"error,omitempty"`
	Details   map[string]any       `json:"details,omitempty"`
}

type ContentPart struct {
	Type              string         `json:"type"`
	Text              string         `json:"text,omitempty"`
	TextSignature     string         `json:"textSignature,omitempty"`
	Thinking          string         `json:"thinking,omitempty"`
	ThinkingSignature string         `json:"thinkingSignature,omitempty"`
	Redacted          bool           `json:"redacted,omitempty"`
	Data              string         `json:"data,omitempty"`
	MIMEType          string         `json:"mimeType,omitempty"`
	ID                string         `json:"id,omitempty"`
	Name              string         `json:"name,omitempty"`
	Arguments         map[string]any `json:"arguments,omitempty"`
	ThoughtSignature  string         `json:"thoughtSignature,omitempty"`
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
	Role           string                       `json:"role"`
	Content        []ContentPart                `json:"content,omitempty"`
	Timestamp      int64                        `json:"timestamp,omitempty"`
	API            string                       `json:"api,omitempty"`
	Provider       string                       `json:"provider,omitempty"`
	Model          string                       `json:"model,omitempty"`
	Diagnostics    []AssistantMessageDiagnostic `json:"diagnostics,omitempty"`
	Usage          Usage                        `json:"usage,omitempty"`
	StopReason     string                       `json:"stopReason,omitempty"`
	ErrorMessage   string                       `json:"errorMessage,omitempty"`
	ResponseID     string                       `json:"responseId,omitempty"`
	ToolCallID     string                       `json:"toolCallID,omitempty"`
	ToolName       string                       `json:"toolName,omitempty"`
	CustomType     string                       `json:"customType,omitempty"`
	Display        *bool                        `json:"display,omitempty"`
	Details        any                          `json:"details,omitempty"`
	IsError        bool                         `json:"isError,omitempty"`
	AddedToolNames []string                     `json:"addedToolNames,omitempty"`
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

// ModelCostTier replaces the base rates when total request input exceeds the
// configured threshold.
type ModelCostTier struct {
	InputTokensAbove int     `json:"inputTokensAbove"`
	Input            float64 `json:"input"`
	Output           float64 `json:"output"`
	CacheRead        float64 `json:"cacheRead"`
	CacheWrite       float64 `json:"cacheWrite"`
}

type ModelCost struct {
	Input      float64         `json:"input"`
	Output     float64         `json:"output"`
	CacheRead  float64         `json:"cacheRead"`
	CacheWrite float64         `json:"cacheWrite"`
	Tiers      []ModelCostTier `json:"tiers,omitempty"`
}

type Model struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	API              string             `json:"api"`
	Provider         string             `json:"provider"`
	BaseURL          string             `json:"baseUrl,omitempty"`
	Headers          map[string]string  `json:"headers,omitempty"`
	Compat           ModelCompat        `json:"compat,omitempty"`
	Reasoning        bool               `json:"reasoning"`
	Input            []string           `json:"input,omitempty"`
	Cost             ModelCost          `json:"cost"`
	ContextWindow    int                `json:"contextWindow"`
	MaxTokens        int                `json:"maxTokens"`
	ThinkingLevelMap map[string]*string `json:"thinkingLevelMap,omitempty"`
}

type ModelCompat struct {
	SupportsStore                               *bool          `json:"supportsStore,omitempty"`
	SupportsDeveloperRole                       *bool          `json:"supportsDeveloperRole,omitempty"`
	SupportsReasoningEffort                     *bool          `json:"supportsReasoningEffort,omitempty"`
	SupportsUsageInStreaming                    *bool          `json:"supportsUsageInStreaming,omitempty"`
	SupportsStrictMode                          *bool          `json:"supportsStrictMode,omitempty"`
	SupportsOpenAIGrammarTools                  *bool          `json:"supportsOpenAIGrammarTools,omitempty"`
	SupportsLongCacheRetention                  *bool          `json:"supportsLongCacheRetention,omitempty"`
	SupportsEagerToolInputStreaming             *bool          `json:"supportsEagerToolInputStreaming,omitempty"`
	SupportsCacheControlOnTools                 *bool          `json:"supportsCacheControlOnTools,omitempty"`
	SupportsExplicitPromptCacheMode             *bool          `json:"supportsExplicitPromptCacheMode,omitempty"`
	SupportsTemperature                         *bool          `json:"supportsTemperature,omitempty"`
	SupportsStrictTools                         *bool          `json:"supportsStrictTools,omitempty"`
	SupportsToolReferences                      *bool          `json:"supportsToolReferences,omitempty"`
	SupportsToolSearch                          *bool          `json:"supportsToolSearch,omitempty"`
	ForceAdaptiveThinking                       *bool          `json:"forceAdaptiveThinking,omitempty"`
	AllowEmptySignature                         *bool          `json:"allowEmptySignature,omitempty"`
	SendSessionAffinityHeaders                  *bool          `json:"sendSessionAffinityHeaders,omitempty"`
	SendSessionIDHeader                         *bool          `json:"sendSessionIdHeader,omitempty"`
	RequiresToolResultName                      *bool          `json:"requiresToolResultName,omitempty"`
	RequiresAssistantAfterToolResult            *bool          `json:"requiresAssistantAfterToolResult,omitempty"`
	RequiresThinkingAsText                      *bool          `json:"requiresThinkingAsText,omitempty"`
	RequiresReasoningContentOnAssistantMessages *bool          `json:"requiresReasoningContentOnAssistantMessages,omitempty"`
	RequiresReasoningContentOnAssistantTurns    *bool          `json:"requiresReasoningContentOnAssistantTurns,omitempty"`
	RequiresReasoningContentOnAssistantEvents   *bool          `json:"requiresReasoningContentOnAssistantEvents,omitempty"`
	ZAIToolStream                               *bool          `json:"zaiToolStream,omitempty"`
	OpenRouterRouting                           map[string]any `json:"openRouterRouting,omitempty"`
	VercelGatewayRouting                        map[string]any `json:"vercelGatewayRouting,omitempty"`
	ChatTemplateKwargs                          map[string]any `json:"chatTemplateKwargs,omitempty"`
	MaxTokensField                              string         `json:"maxTokensField,omitempty"`
	ThinkingFormat                              string         `json:"thinkingFormat,omitempty"`
	CacheControlFormat                          string         `json:"cacheControlFormat,omitempty"`
	DeferredToolsMode                           string         `json:"deferredToolsMode,omitempty"`
	SessionAffinityFormat                       string         `json:"sessionAffinityFormat,omitempty"`
}

type Context struct {
	SystemPrompt string    `json:"systemPrompt,omitempty"`
	Messages     []Message `json:"messages,omitempty"`
	Tools        []Tool    `json:"tools,omitempty"`
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
	ToolChoice       any
	Debug            bool
	ThinkingBudgets  map[string]int
	Headers          map[string]string
	HeaderRemovals   []string
	Env              ProviderEnv
	TimeoutMillis    int
	MaxRetries       int
	MaxRetryDelayMs  int
	Metadata         map[string]any
	OnPayload        func(payload any, model Model) (any, bool, error)
	OnResponseStatus func(status int, headers map[string]string, model Model) error
}

type SimpleStreamOptions = StreamOptions

type AssistantMessageEvent struct {
	Type         string      `json:"type"`
	Partial      Message     `json:"partial,omitempty"`
	Message      Message     `json:"message,omitempty"`
	Error        Message     `json:"error,omitempty"`
	Reason       string      `json:"reason,omitempty"`
	ContentIndex int         `json:"contentIndex,omitempty"`
	Delta        string      `json:"delta,omitempty"`
	Content      string      `json:"content,omitempty"`
	ToolCall     ContentPart `json:"toolCall,omitempty"`
}

type ConstrainedSamplingType string

const (
	ConstrainedSamplingJSONSchema ConstrainedSamplingType = "json_schema"
	ConstrainedSamplingGrammar    ConstrainedSamplingType = "grammar"
)

type ConstrainedSamplingStrictness string

const (
	ConstrainedSamplingPrefer  ConstrainedSamplingStrictness = "prefer"
	ConstrainedSamplingRequire ConstrainedSamplingStrictness = "require"
)

// GrammarVariants carries provider-specific encodings of the same intended
// grammar. A provider selects only variants it explicitly supports.
type GrammarVariants struct {
	OpenAILark  string `json:"openai_lark,omitempty"`
	OpenAIRegex string `json:"openai_regex,omitempty"`
}

// ConstrainedSamplingConfig is the Go representation of Pi's discriminated
// constrained-sampling union. A nil Tool.ConstrainedSampling is equivalent to
// Pi's absent/false value.
type ConstrainedSamplingConfig struct {
	Type     ConstrainedSamplingType       `json:"type"`
	Strict   ConstrainedSamplingStrictness `json:"strict,omitempty"`
	Variants GrammarVariants               `json:"variants,omitempty"`
}

type Tool struct {
	Name                string                     `json:"name"`
	Description         string                     `json:"description,omitempty"`
	Parameters          Schema                     `json:"parameters,omitempty"`
	ConstrainedSampling *ConstrainedSamplingConfig `json:"constrainedSampling,omitempty"`
}

type Schema struct {
	Type        any               `json:"type,omitempty"`
	Description string            `json:"description,omitempty"`
	Default     any               `json:"default,omitempty"`
	Properties  map[string]Schema `json:"properties,omitempty"`
	Required    []string          `json:"required,omitempty"`
	Items       *Schema           `json:"items,omitempty"`
	Enum        []any             `json:"enum,omitempty"`
}

type StringEnumOptions struct {
	Description string
	Default     string
}

func Object(properties map[string]Schema, required ...string) Schema {
	return Schema{Type: "object", Properties: properties, Required: required}
}

func String() Schema  { return Schema{Type: "string"} }
func Number() Schema  { return Schema{Type: "number"} }
func Integer() Schema { return Schema{Type: "integer"} }
func Boolean() Schema { return Schema{Type: "boolean"} }
func Null() Schema    { return Schema{Type: "null"} }

func StringEnum(values ...string) Schema {
	return StringEnumWithOptions(values, StringEnumOptions{})
}

func StringEnumWithOptions(values []string, options StringEnumOptions) Schema {
	enum := make([]any, len(values))
	for i, value := range values {
		enum[i] = value
	}
	schema := Schema{Type: "string", Enum: enum}
	if options.Description != "" {
		schema.Description = options.Description
	}
	if options.Default != "" {
		schema.Default = options.Default
	}
	return schema
}

func TypeUnion(types ...string) Schema {
	values := make([]any, len(types))
	for i, t := range types {
		values[i] = t
	}
	return Schema{Type: values}
}
