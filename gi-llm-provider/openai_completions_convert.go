package gillmprovider

import (
	"encoding/json"
	"strings"
)

type OpenAICompletionsCompat struct {
	SupportsDeveloperRole                    bool
	SupportsStore                            bool
	SupportsReasoningEffort                  bool
	SupportsUsageInStreaming                 bool
	SupportsStrictMode                       bool
	SupportsOpenAIGrammarTools               bool
	SupportsLongCacheRetention               bool
	MaxTokensField                           string
	ThinkingFormat                           string
	CacheControlFormat                       string
	ZAIToolStream                            bool
	RequiresToolResultName                   bool
	RequiresAssistantAfterToolResult         bool
	RequiresThinkingAsText                   bool
	RequiresReasoningContentOnAssistant      bool
	RequiresReasoningContentOnAssistantTurns bool
	DeferredToolsMode                        string
}

type OpenAICompletionsPayloadOptions struct {
	MaxTokens      int
	Temperature    *float64
	CacheRetention string
	SessionID      string
	Reasoning      string
	ToolChoice     any
	Headers        map[string]string
}

type OpenAICompletionsPayload struct {
	Model                string              `json:"model"`
	Messages             []OpenAIChatMessage `json:"messages"`
	Stream               bool                `json:"stream"`
	PromptCacheKey       string              `json:"prompt_cache_key,omitempty"`
	PromptCacheRetention string              `json:"prompt_cache_retention,omitempty"`
	StreamOptions        map[string]any      `json:"stream_options,omitempty"`
	Store                *bool               `json:"store,omitempty"`
	MaxTokens            int                 `json:"max_tokens,omitempty"`
	MaxCompletionTokens  int                 `json:"max_completion_tokens,omitempty"`
	Temperature          *float64            `json:"temperature,omitempty"`
	Tools                []OpenAIChatTool    `json:"tools,omitempty"`
	ToolChoice           any                 `json:"tool_choice,omitempty"`
	ToolStream           *bool               `json:"tool_stream,omitempty"`
	EnableThinking       *bool               `json:"enable_thinking,omitempty"`
	ChatTemplateKwargs   map[string]any      `json:"chat_template_kwargs,omitempty"`
	Thinking             map[string]any      `json:"thinking,omitempty"`
	Reasoning            map[string]any      `json:"reasoning,omitempty"`
	ReasoningEffort      string              `json:"reasoning_effort,omitempty"`
}

type OpenAIChatTool struct {
	Type         string                    `json:"type"`
	Function     OpenAIChatToolFunction    `json:"function"`
	Custom       *OpenAIChatCustomTool     `json:"custom,omitempty"`
	CacheControl *OpenAICompatCacheControl `json:"cache_control,omitempty"`
}

type OpenAIChatCustomTool struct {
	Name        string                     `json:"name"`
	Description string                     `json:"description,omitempty"`
	Format      OpenAIChatCustomToolFormat `json:"format"`
}

type OpenAIChatCustomToolFormat struct {
	Type    string                      `json:"type"`
	Grammar OpenAIChatCustomToolGrammar `json:"grammar"`
}

type OpenAIChatCustomToolGrammar struct {
	Syntax     GrammarFormat `json:"syntax"`
	Definition string        `json:"definition"`
}

func (tool OpenAIChatTool) MarshalJSON() ([]byte, error) {
	if tool.Type == "custom" || tool.Custom != nil {
		return json.Marshal(struct {
			Type         string                    `json:"type"`
			Custom       *OpenAIChatCustomTool     `json:"custom,omitempty"`
			CacheControl *OpenAICompatCacheControl `json:"cache_control,omitempty"`
		}{
			Type:         tool.Type,
			Custom:       tool.Custom,
			CacheControl: tool.CacheControl,
		})
	}
	type alias OpenAIChatTool
	return json.Marshal(alias(tool))
}

type OpenAIChatToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
	Strict      *bool  `json:"strict,omitempty"`
}

type OpenAICompatCacheControl struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

type OpenAIChatMessage struct {
	Role             string               `json:"role"`
	Content          any                  `json:"content,omitempty"`
	ToolCalls        []OpenAIChatToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string               `json:"tool_call_id,omitempty"`
	Name             string               `json:"name,omitempty"`
	ReasoningContent string               `json:"reasoning_content,omitempty"`
	Reasoning        string               `json:"reasoning,omitempty"`
	ReasoningText    string               `json:"reasoning_text,omitempty"`
	ReasoningDetails []map[string]any     `json:"reasoning_details,omitempty"`
	Tools            []OpenAIChatTool     `json:"tools,omitempty"`
	Extra            map[string]any       `json:"-"`

	includeReasoningContent bool
}

func (m OpenAIChatMessage) MarshalJSON() ([]byte, error) {
	type alias OpenAIChatMessage
	raw, err := json.Marshal(alias(m))
	if err != nil {
		return nil, err
	}
	if !m.includeReasoningContent && len(m.Extra) == 0 {
		return raw, nil
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	if m.includeReasoningContent {
		object["reasoning_content"] = m.ReasoningContent
	}
	for key, value := range m.Extra {
		object[key] = value
	}
	return json.Marshal(object)
}

type OpenAIChatToolCall struct {
	ID       string                     `json:"id"`
	Type     string                     `json:"type"`
	Function OpenAIChatToolCallFunction `json:"function"`
	Custom   *OpenAIChatCustomToolCall  `json:"custom,omitempty"`
}

type OpenAIChatCustomToolCall struct {
	Name  string `json:"name"`
	Input string `json:"input"`
}

func (call OpenAIChatToolCall) MarshalJSON() ([]byte, error) {
	if call.Type == "custom" || call.Custom != nil {
		return json.Marshal(struct {
			ID     string                    `json:"id"`
			Type   string                    `json:"type"`
			Custom *OpenAIChatCustomToolCall `json:"custom,omitempty"`
		}{
			ID:     call.ID,
			Type:   call.Type,
			Custom: call.Custom,
		})
	}
	type alias OpenAIChatToolCall
	return json.Marshal(alias(call))
}

type OpenAIChatToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIChatContentPart struct {
	Type         string                    `json:"type"`
	Text         string                    `json:"text,omitempty"`
	ImageURL     *OpenAIChatImageURL       `json:"image_url,omitempty"`
	CacheControl *OpenAICompatCacheControl `json:"cache_control,omitempty"`
}

type OpenAIChatImageURL struct {
	URL string `json:"url"`
}

// OpenAICompletionsRequestState is resolved once per request and shared by
// payload conversion and stream decoding.
type OpenAICompletionsRequestState struct {
	Compat                     OpenAICompletionsCompat
	ToolPlacement              DeferredToolSet
	GrammarToolInputProperties map[string]string
}

func ResolveOpenAICompletionsRequestState(
	model Model,
	context Context,
) (OpenAICompletionsRequestState, error) {
	return resolveOpenAICompletionsRequestState(context, ResolveOpenAICompletionsCompat(model))
}

func resolveOpenAICompletionsRequestState(
	context Context,
	compat OpenAICompletionsCompat,
) (OpenAICompletionsRequestState, error) {
	grammarToolInputProperties, err := CreateGrammarToolInputProperties(
		context.Tools,
		compat.SupportsOpenAIGrammarTools,
	)
	if err != nil {
		return OpenAICompletionsRequestState{}, err
	}
	return OpenAICompletionsRequestState{
		Compat:                     compat,
		ToolPlacement:              splitKimiDeferredTools(context, compat.DeferredToolsMode == "kimi"),
		GrammarToolInputProperties: grammarToolInputProperties,
	}, nil
}

func ConvertOpenAICompletionsMessages(model Model, context Context, compat OpenAICompletionsCompat) []OpenAIChatMessage {
	messages, _ := ConvertOpenAICompletionsMessagesChecked(model, context, compat)
	return messages
}

func ConvertOpenAICompletionsMessagesChecked(
	model Model,
	context Context,
	compat OpenAICompletionsCompat,
) ([]OpenAIChatMessage, error) {
	state, err := resolveOpenAICompletionsRequestState(context, compat)
	if err != nil {
		return nil, err
	}
	return convertOpenAICompletionsMessages(model, context, state)
}

func convertOpenAICompletionsMessages(
	model Model,
	context Context,
	state OpenAICompletionsRequestState,
) ([]OpenAIChatMessage, error) {
	compat := state.Compat
	var params []OpenAIChatMessage
	transformed := TransformMessages(context.Messages, model, func(id string, target Model, _ Message) string {
		return NormalizeToolCallIDForOpenAICompletions(id, target)
	})

	if context.SystemPrompt != "" {
		role := "system"
		if model.Reasoning && compat.SupportsDeveloperRole {
			role = "developer"
		}
		params = append(params, OpenAIChatMessage{Role: role, Content: SanitizeSurrogates(context.SystemPrompt)})
	}

	lastRole := ""
	for i := 0; i < len(transformed); i++ {
		message := transformed[i]
		if compat.RequiresAssistantAfterToolResult && lastRole == RoleToolResult && message.Role == RoleUser {
			params = append(params, OpenAIChatMessage{Role: RoleAssistant, Content: "I have processed the tool results."})
		}

		switch message.Role {
		case RoleUser:
			content := convertOpenAIChatUserContent(message.Content)
			if len(content) == 0 {
				continue
			}
			params = append(params, OpenAIChatMessage{Role: "user", Content: content})
			lastRole = RoleUser
		case RoleAssistant:
			assistant, err := convertOpenAIChatAssistantMessage(
				message,
				model,
				compat,
				state.GrammarToolInputProperties,
			)
			if err != nil {
				return nil, err
			}
			if assistant == nil {
				continue
			}
			params = append(params, *assistant)
			lastRole = RoleAssistant
		case RoleToolResult:
			imageBlocks := []OpenAIChatContentPart{}
			deferredToolNames := make([]string, 0)
			seenDeferredToolNames := make(map[string]struct{})
			j := i
			for ; j < len(transformed) && transformed[j].Role == RoleToolResult; j++ {
				toolMessage := transformed[j]
				textResult := joinTextContent(toolMessage.Content)
				hasText := textResult != ""
				toolResult := OpenAIChatMessage{
					Role:       "tool",
					Content:    SanitizeSurrogates(selectToolText(hasText, textResult)),
					ToolCallID: toolMessage.ToolCallID,
				}
				if compat.RequiresToolResultName && toolMessage.ToolName != "" {
					toolResult.Name = toolMessage.ToolName
				}
				params = append(params, toolResult)
				if compat.DeferredToolsMode == "kimi" {
					for _, name := range toolMessage.AddedToolNames {
						if _, seen := seenDeferredToolNames[name]; seen {
							continue
						}
						seenDeferredToolNames[name] = struct{}{}
						deferredToolNames = append(deferredToolNames, name)
					}
				}
				if containsString(model.Input, "image") {
					for _, part := range toolMessage.Content {
						if part.Type == ContentImage {
							imageBlocks = append(imageBlocks, OpenAIChatContentPart{
								Type:     "image_url",
								ImageURL: &OpenAIChatImageURL{URL: "data:" + part.MIMEType + ";base64," + part.Data},
							})
						}
					}
				}
			}
			i = j - 1
			if len(imageBlocks) > 0 {
				if compat.RequiresAssistantAfterToolResult {
					params = append(params, OpenAIChatMessage{Role: RoleAssistant, Content: "I have processed the tool results."})
				}
				content := append([]OpenAIChatContentPart{{Type: "text", Text: "Attached image(s) from tool result:"}}, imageBlocks...)
				params = append(params, OpenAIChatMessage{Role: "user", Content: content})
				lastRole = RoleUser
			} else {
				lastRole = RoleToolResult
			}
			if len(deferredToolNames) > 0 {
				deferredTools := getToolsByName(state.ToolPlacement.Deferred, deferredToolNames)
				if len(deferredTools) > 0 {
					convertedTools, err := ConvertOpenAICompletionsToolsChecked(deferredTools, compat)
					if err != nil {
						return nil, err
					}
					params = append(params, OpenAIChatMessage{
						Role:  "system",
						Tools: convertedTools,
					})
				}
			}
		default:
			params = append(params, OpenAIChatMessage{Role: message.Role, Content: joinTextContent(message.Content)})
			lastRole = message.Role
		}
	}
	return params, nil
}

func ShouldSendOpenAICompletionsTools(context Context) bool {
	if len(context.Tools) > 0 {
		return true
	}
	for _, message := range context.Messages {
		if message.Role == RoleAssistant {
			for _, part := range message.Content {
				if part.Type == ContentToolCall {
					return true
				}
			}
		}
		if message.Role == RoleToolResult {
			return true
		}
	}
	return false
}

func BuildOpenAICompletionsPayload(model Model, context Context, options OpenAICompletionsPayloadOptions) OpenAICompletionsPayload {
	payload, _, _ := buildOpenAICompletionsPayload(model, context, options)
	return payload
}

func buildOpenAICompletionsPayload(
	model Model,
	context Context,
	options OpenAICompletionsPayloadOptions,
) (OpenAICompletionsPayload, OpenAICompletionsRequestState, error) {
	state, err := ResolveOpenAICompletionsRequestState(model, context)
	if err != nil {
		return OpenAICompletionsPayload{}, OpenAICompletionsRequestState{}, err
	}
	compat := state.Compat
	cacheRetention := resolveCacheRetention(options.CacheRetention)
	messages, err := convertOpenAICompletionsMessages(model, context, state)
	if err != nil {
		return OpenAICompletionsPayload{}, OpenAICompletionsRequestState{}, err
	}
	payload := OpenAICompletionsPayload{
		Model:    model.ID,
		Messages: messages,
		Stream:   true,
	}

	if compat.SupportsUsageInStreaming {
		payload.StreamOptions = map[string]any{"include_usage": true}
	}
	if compat.SupportsStore {
		payload.Store = ptrBool(false)
	}
	if options.MaxTokens > 0 {
		if compat.MaxTokensField == "max_tokens" {
			payload.MaxTokens = options.MaxTokens
		} else {
			payload.MaxCompletionTokens = options.MaxTokens
		}
	}
	if options.Temperature != nil {
		payload.Temperature = options.Temperature
	}
	if (strings.Contains(model.BaseURL, "api.openai.com") && cacheRetention != "none") ||
		(cacheRetention == "long" && compat.SupportsLongCacheRetention) {
		payload.PromptCacheKey = options.SessionID
	}
	if cacheRetention == "long" && compat.SupportsLongCacheRetention {
		payload.PromptCacheRetention = "24h"
	}
	if len(state.ToolPlacement.Immediate) > 0 {
		payload.Tools, err = ConvertOpenAICompletionsToolsChecked(state.ToolPlacement.Immediate, compat)
		if err != nil {
			return OpenAICompletionsPayload{}, OpenAICompletionsRequestState{}, err
		}
		if compat.ZAIToolStream {
			payload.ToolStream = ptrBool(true)
		}
	} else if ShouldSendOpenAICompletionsTools(context) {
		payload.Tools = []OpenAIChatTool{}
	}
	if cacheControl := openAICompatCacheControl(compat, cacheRetention); cacheControl != nil {
		ApplyOpenAIAnthropicCacheControl(payload.Messages, payload.Tools, cacheControl)
	}
	if options.ToolChoice != nil {
		payload.ToolChoice = options.ToolChoice
	}
	applyOpenAICompletionsReasoning(&payload, model, compat, options.Reasoning)
	return payload, state, nil
}

func BuildOpenAICompletionsPayloadChecked(model Model, context Context, options OpenAICompletionsPayloadOptions) (OpenAICompletionsPayload, error) {
	payload, _, err := buildOpenAICompletionsPayloadChecked(model, context, options)
	return payload, err
}

func buildOpenAICompletionsPayloadChecked(
	model Model,
	context Context,
	options OpenAICompletionsPayloadOptions,
) (OpenAICompletionsPayload, OpenAICompletionsRequestState, error) {
	if err := ValidateThinkingLevelSupported(model, options.Reasoning); err != nil {
		return OpenAICompletionsPayload{}, OpenAICompletionsRequestState{}, err
	}
	return buildOpenAICompletionsPayload(model, context, options)
}

func BuildOpenAICompletionsHeaders(model Model, options OpenAICompletionsPayloadOptions) map[string]string {
	headers := map[string]string{}
	for key, value := range model.Headers {
		headers[key] = value
	}
	cacheRetention := resolveCacheRetention(options.CacheRetention)
	if cacheRetention != "none" && options.SessionID != "" && model.Compat.SendSessionAffinityHeaders != nil && *model.Compat.SendSessionAffinityHeaders {
		headers["session_id"] = options.SessionID
		headers["x-client-request-id"] = options.SessionID
		headers["x-session-affinity"] = options.SessionID
	}
	for key, value := range options.Headers {
		headers[key] = value
	}
	return headers
}

func ConvertOpenAICompletionsTools(tools []Tool, compat OpenAICompletionsCompat) []OpenAIChatTool {
	converted, _ := ConvertOpenAICompletionsToolsChecked(tools, compat)
	return converted
}

func ConvertOpenAICompletionsToolsChecked(
	tools []Tool,
	compat OpenAICompletionsCompat,
) ([]OpenAIChatTool, error) {
	result := make([]OpenAIChatTool, 0, len(tools))
	for _, tool := range tools {
		grammar, ok, err := ResolveGrammarConstrainedSampling(tool, compat.SupportsOpenAIGrammarTools)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, OpenAIChatTool{
				Type: "custom",
				Custom: &OpenAIChatCustomTool{
					Name:        tool.Name,
					Description: tool.Description,
					Format: OpenAIChatCustomToolFormat{
						Type: "grammar",
						Grammar: OpenAIChatCustomToolGrammar{
							Syntax:     grammar.Format,
							Definition: grammar.Definition,
						},
					},
				},
			})
			continue
		}

		strict, err := ResolveJSONSchemaStrictSampling(tool, compat.SupportsStrictMode)
		if err != nil {
			return nil, err
		}
		converted := OpenAIChatTool{
			Type: "function",
			Function: OpenAIChatToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  SchemaToMap(tool.Parameters),
			},
		}
		if compat.SupportsStrictMode {
			converted.Function.Strict = strict
			if converted.Function.Strict == nil {
				converted.Function.Strict = ptrBool(false)
			}
		}
		result = append(result, converted)
	}
	return result, nil
}

func ResolveOpenAICompletionsCompat(model Model) OpenAICompletionsCompat {
	detected := detectOpenAICompletionsCompat(model)
	compat := model.Compat
	if compat.SupportsStore != nil {
		detected.SupportsStore = *compat.SupportsStore
	}
	if compat.SupportsDeveloperRole != nil {
		detected.SupportsDeveloperRole = *compat.SupportsDeveloperRole
	}
	if compat.SupportsReasoningEffort != nil {
		detected.SupportsReasoningEffort = *compat.SupportsReasoningEffort
	}
	if compat.SupportsUsageInStreaming != nil {
		detected.SupportsUsageInStreaming = *compat.SupportsUsageInStreaming
	}
	if compat.SupportsStrictMode != nil {
		detected.SupportsStrictMode = *compat.SupportsStrictMode
	}
	if compat.SupportsOpenAIGrammarTools != nil {
		detected.SupportsOpenAIGrammarTools = *compat.SupportsOpenAIGrammarTools
	}
	if compat.SupportsLongCacheRetention != nil {
		detected.SupportsLongCacheRetention = *compat.SupportsLongCacheRetention
	}
	if compat.RequiresToolResultName != nil {
		detected.RequiresToolResultName = *compat.RequiresToolResultName
	}
	if compat.RequiresAssistantAfterToolResult != nil {
		detected.RequiresAssistantAfterToolResult = *compat.RequiresAssistantAfterToolResult
	}
	if compat.RequiresThinkingAsText != nil {
		detected.RequiresThinkingAsText = *compat.RequiresThinkingAsText
	}
	if compat.RequiresReasoningContentOnAssistantMessages != nil {
		detected.RequiresReasoningContentOnAssistant = *compat.RequiresReasoningContentOnAssistantMessages
	}
	if compat.RequiresReasoningContentOnAssistantEvents != nil {
		detected.RequiresReasoningContentOnAssistant = *compat.RequiresReasoningContentOnAssistantEvents
	}
	if compat.RequiresReasoningContentOnAssistantTurns != nil {
		detected.RequiresReasoningContentOnAssistantTurns = *compat.RequiresReasoningContentOnAssistantTurns
	}
	if compat.ZAIToolStream != nil {
		detected.ZAIToolStream = *compat.ZAIToolStream
	}
	if compat.MaxTokensField != "" {
		detected.MaxTokensField = compat.MaxTokensField
	}
	if compat.ThinkingFormat != "" {
		detected.ThinkingFormat = compat.ThinkingFormat
	}
	if compat.CacheControlFormat != "" {
		detected.CacheControlFormat = compat.CacheControlFormat
	}
	if compat.DeferredToolsMode != "" {
		detected.DeferredToolsMode = compat.DeferredToolsMode
	}
	return detected
}

func ApplyOpenAIAnthropicCacheControl(messages []OpenAIChatMessage, tools []OpenAIChatTool, cacheControl *OpenAICompatCacheControl) {
	if cacheControl == nil {
		return
	}
	for i := range messages {
		if messages[i].Role == "system" || messages[i].Role == "developer" {
			if addOpenAITextCacheControl(&messages[i], cacheControl) {
				break
			}
		}
	}
	if len(tools) > 0 {
		tools[len(tools)-1].CacheControl = cacheControl
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser || messages[i].Role == RoleAssistant {
			if addOpenAITextCacheControl(&messages[i], cacheControl) {
				break
			}
		}
	}
}

func detectOpenAICompletionsCompat(model Model) OpenAICompletionsCompat {
	provider := model.Provider
	baseURL := model.BaseURL
	isZai := provider == "zai" || strings.Contains(baseURL, "api.z.ai")
	isTogether := provider == "together" || strings.Contains(baseURL, "api.together.ai") || strings.Contains(baseURL, "api.together.xyz")
	isMoonshot := provider == "moonshotai" || provider == "moonshotai-cn" || strings.Contains(baseURL, "api.moonshot.")
	isCloudflareWorkersAI := provider == "cloudflare-workers-ai" || strings.Contains(baseURL, "api.cloudflare.com")
	isCloudflareAIGateway := provider == "cloudflare-ai-gateway" || strings.Contains(baseURL, "gateway.ai.cloudflare.com")
	isDeepSeek := provider == "deepseek" || strings.Contains(baseURL, "deepseek.com")
	isGrok := provider == "xai" || strings.Contains(baseURL, "api.x.ai")
	isNonStandard := provider == "cerebras" ||
		strings.Contains(baseURL, "cerebras.ai") ||
		provider == "xai" ||
		strings.Contains(baseURL, "api.x.ai") ||
		isTogether ||
		strings.Contains(baseURL, "chutes.ai") ||
		isDeepSeek ||
		isZai ||
		isMoonshot ||
		provider == "opencode" ||
		strings.Contains(baseURL, "opencode.ai") ||
		isCloudflareWorkersAI ||
		isCloudflareAIGateway
	useMaxTokens := strings.Contains(baseURL, "chutes.ai") || isMoonshot || isCloudflareAIGateway || isTogether || isDeepSeek || isGrok
	thinkingFormat := "openai"
	switch {
	case isDeepSeek:
		thinkingFormat = "deepseek"
	case isGrok:
		thinkingFormat = "xai"
	case isZai:
		thinkingFormat = "zai"
	case isTogether:
		thinkingFormat = "together"
	case provider == "openrouter" || strings.Contains(baseURL, "openrouter.ai"):
		thinkingFormat = "openrouter"
	}
	cacheControlFormat := ""
	if provider == "openrouter" && strings.HasPrefix(model.ID, "anthropic/") {
		cacheControlFormat = "anthropic"
	}
	maxTokensField := "max_completion_tokens"
	if useMaxTokens {
		maxTokensField = "max_tokens"
	}
	return OpenAICompletionsCompat{
		SupportsStore:                       !isNonStandard,
		SupportsDeveloperRole:               !isNonStandard,
		SupportsReasoningEffort:             !isZai && !isMoonshot && !isTogether && !isCloudflareAIGateway,
		SupportsUsageInStreaming:            true,
		MaxTokensField:                      maxTokensField,
		RequiresReasoningContentOnAssistant: isDeepSeek,
		ThinkingFormat:                      thinkingFormat,
		SupportsStrictMode:                  !(isDeepSeek || isGrok || isMoonshot || isTogether || isCloudflareAIGateway),
		CacheControlFormat:                  cacheControlFormat,
		SupportsLongCacheRetention:          !(isDeepSeek || isGrok || isTogether || isCloudflareWorkersAI || isCloudflareAIGateway),
	}
}

func openAICompatCacheControl(compat OpenAICompletionsCompat, cacheRetention string) *OpenAICompatCacheControl {
	if compat.CacheControlFormat != "anthropic" || cacheRetention == "none" {
		return nil
	}
	result := &OpenAICompatCacheControl{Type: "ephemeral"}
	if cacheRetention == "long" && compat.SupportsLongCacheRetention {
		result.TTL = "1h"
	}
	return result
}

func addOpenAITextCacheControl(message *OpenAIChatMessage, cacheControl *OpenAICompatCacheControl) bool {
	switch content := message.Content.(type) {
	case string:
		if content == "" {
			return false
		}
		message.Content = []OpenAIChatContentPart{{Type: "text", Text: content, CacheControl: cacheControl}}
		return true
	case []OpenAIChatContentPart:
		for i := len(content) - 1; i >= 0; i-- {
			if content[i].Type == "text" {
				content[i].CacheControl = cacheControl
				message.Content = content
				return true
			}
		}
	}
	return false
}

func applyOpenAICompletionsReasoning(payload *OpenAICompletionsPayload, model Model, compat OpenAICompletionsCompat, level string) {
	if !model.Reasoning {
		return
	}
	if level != "" {
		level = ClampThinkingLevel(model, level)
		if level == "off" && compat.ThinkingFormat != "xai" {
			level = ""
		}
	}
	mapped := mapThinkingLevel(model, level)
	switch compat.ThinkingFormat {
	case "zai", "qwen":
		payload.EnableThinking = ptrBool(level != "")
	case "qwen-chat-template":
		payload.ChatTemplateKwargs = map[string]any{"enable_thinking": level != "", "preserve_thinking": true}
	case "deepseek":
		state := "disabled"
		if level != "" {
			state = "enabled"
			payload.ReasoningEffort = mapped
		}
		payload.Thinking = map[string]any{"type": state}
	case "xai":
		if level == "off" && mapped == "off" {
			mapped = "none"
		}
		if level != "" && compat.SupportsReasoningEffort {
			payload.ReasoningEffort = mapped
		}
	case "openrouter":
		if level != "" {
			payload.Reasoning = map[string]any{"effort": mapped}
		}
	case "together":
		payload.Reasoning = map[string]any{"enabled": level != ""}
		if level != "" && compat.SupportsReasoningEffort {
			payload.ReasoningEffort = mapped
		}
	default:
		if level != "" && compat.SupportsReasoningEffort {
			payload.ReasoningEffort = mapped
		}
	}
}

func mapThinkingLevel(model Model, level string) string {
	if level == "" {
		return ""
	}
	if mapped, ok := model.ThinkingLevelMap[level]; ok {
		if mapped == nil {
			return ""
		}
		return *mapped
	}
	return level
}

func resolveCacheRetention(value string) string {
	return resolveCacheRetentionWithEnv(value, nil)
}

func resolveCacheRetentionWithEnv(value string, env ProviderEnv) string {
	switch value {
	case "none", "short", "long":
		return value
	default:
		if GetProviderEnvValue("PI_CACHE_RETENTION", env) == "long" {
			return "long"
		}
		return "short"
	}
}

func convertOpenAIChatUserContent(content []ContentPart) []OpenAIChatContentPart {
	parts := make([]OpenAIChatContentPart, 0, len(content))
	for _, part := range content {
		switch part.Type {
		case ContentText:
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			parts = append(parts, OpenAIChatContentPart{Type: "text", Text: SanitizeSurrogates(part.Text)})
		case ContentImage:
			parts = append(parts, OpenAIChatContentPart{Type: "image_url", ImageURL: &OpenAIChatImageURL{URL: "data:" + part.MIMEType + ";base64," + part.Data}})
		}
	}
	return parts
}

func convertOpenAIChatAssistantMessage(
	message Message,
	model Model,
	compat OpenAICompletionsCompat,
	grammarToolInputProperties map[string]string,
) (*OpenAIChatMessage, error) {
	assistant := OpenAIChatMessage{Role: RoleAssistant}
	if compat.RequiresAssistantAfterToolResult {
		assistant.Content = ""
	}
	textParts := []string{}
	thinkingParts := []ContentPart{}
	for _, part := range message.Content {
		switch part.Type {
		case ContentText:
			if strings.TrimSpace(part.Text) != "" {
				textParts = append(textParts, SanitizeSurrogates(part.Text))
			}
		case ContentThinking:
			if strings.TrimSpace(part.Thinking) != "" {
				thinkingParts = append(thinkingParts, part)
			}
		case ContentToolCall:
			if inputProperty, custom := grammarToolInputProperties[part.Name]; custom {
				input, err := GetGrammarToolInput(part.Name, part.Arguments, inputProperty)
				if err != nil {
					return nil, err
				}
				assistant.ToolCalls = append(assistant.ToolCalls, OpenAIChatToolCall{
					ID:   part.ID,
					Type: "custom",
					Custom: &OpenAIChatCustomToolCall{
						Name:  part.Name,
						Input: SanitizeSurrogates(input),
					},
				})
			} else {
				arguments, _ := json.Marshal(part.Arguments)
				assistant.ToolCalls = append(assistant.ToolCalls, OpenAIChatToolCall{
					ID:   part.ID,
					Type: "function",
					Function: OpenAIChatToolCallFunction{
						Name:      part.Name,
						Arguments: string(arguments),
					},
				})
			}
			if part.ThoughtSignature != "" {
				var detail map[string]any
				if json.Unmarshal([]byte(part.ThoughtSignature), &detail) == nil {
					assistant.ReasoningDetails = append(assistant.ReasoningDetails, detail)
				}
			}
		}
	}
	if len(thinkingParts) > 0 {
		if compat.RequiresThinkingAsText {
			var thinking []string
			for _, part := range thinkingParts {
				thinking = append(thinking, SanitizeSurrogates(part.Thinking))
			}
			content := []OpenAIChatContentPart{{Type: "text", Text: strings.Join(thinking, "\n\n")}}
			for _, text := range textParts {
				content = append(content, OpenAIChatContentPart{Type: "text", Text: text})
			}
			assistant.Content = content
		} else if len(textParts) > 0 {
			assistant.Content = strings.Join(textParts, "")
		}
		if !compat.RequiresThinkingAsText {
			setOpenAIChatAssistantThinkingReplayField(&assistant, thinkingParts)
		}
	} else if len(textParts) > 0 {
		assistant.Content = strings.Join(textParts, "")
	}
	if (compat.RequiresReasoningContentOnAssistant || compat.RequiresReasoningContentOnAssistantTurns) && model.Reasoning && !assistant.includeReasoningContent {
		assistant.ReasoningContent = ""
		assistant.includeReasoningContent = true
	}
	hasStringContent := false
	switch content := assistant.Content.(type) {
	case string:
		hasStringContent = content != ""
	case []OpenAIChatContentPart:
		hasStringContent = len(content) > 0
	default:
		hasStringContent = content != nil
	}
	if !hasStringContent && len(assistant.ToolCalls) == 0 {
		return nil, nil
	}
	return &assistant, nil
}

func setOpenAIChatAssistantThinkingReplayField(assistant *OpenAIChatMessage, parts []ContentPart) {
	if assistant == nil || len(parts) == 0 {
		return
	}
	signature := parts[0].ThinkingSignature
	if strings.TrimSpace(signature) == "" {
		return
	}
	thinking := make([]string, 0, len(parts))
	for _, part := range parts {
		thinking = append(thinking, SanitizeSurrogates(part.Thinking))
	}
	value := strings.Join(thinking, "\n")
	switch signature {
	case "reasoning_content":
		assistant.ReasoningContent = value
		assistant.includeReasoningContent = true
	case "reasoning":
		assistant.Reasoning = value
	case "reasoning_text":
		assistant.ReasoningText = value
	default:
		if assistant.Extra == nil {
			assistant.Extra = map[string]any{}
		}
		assistant.Extra[signature] = value
	}
}

func joinTextContent(content []ContentPart) string {
	return ExtractTextContent(content)
}

func selectToolText(hasText bool, text string) string {
	if hasText {
		return text
	}
	return "(see attached image)"
}
