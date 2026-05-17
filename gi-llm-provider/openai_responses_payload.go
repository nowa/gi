package gillmprovider

type OpenAIResponsesCompat struct {
	SendSessionIDHeader        bool
	SupportsLongCacheRetention bool
}

type OpenAIResponsesPayloadOptions struct {
	MaxTokens        int
	Temperature      *float64
	CacheRetention   string
	SessionID        string
	ReasoningEffort  string
	ReasoningSummary string
	ServiceTier      string
	Headers          map[string]string
}

type OpenAIResponsesPayload struct {
	Model                string                     `json:"model"`
	Input                []OpenAIResponsesInputItem `json:"input"`
	Stream               bool                       `json:"stream"`
	PromptCacheKey       string                     `json:"prompt_cache_key,omitempty"`
	PromptCacheRetention string                     `json:"prompt_cache_retention,omitempty"`
	Store                bool                       `json:"store"`
	MaxOutputTokens      int                        `json:"max_output_tokens,omitempty"`
	Temperature          *float64                   `json:"temperature,omitempty"`
	ServiceTier          string                     `json:"service_tier,omitempty"`
	Tools                []OpenAIResponsesTool      `json:"tools,omitempty"`
	Reasoning            map[string]string          `json:"reasoning,omitempty"`
	Include              []string                   `json:"include,omitempty"`
}

type OpenAIResponsesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Strict      *bool          `json:"strict,omitempty"`
}

func ResolveOpenAIResponsesCompat(model Model) OpenAIResponsesCompat {
	compat := OpenAIResponsesCompat{
		SendSessionIDHeader:        true,
		SupportsLongCacheRetention: true,
	}
	if model.Compat.SendSessionIDHeader != nil {
		compat.SendSessionIDHeader = *model.Compat.SendSessionIDHeader
	}
	if model.Compat.SupportsLongCacheRetention != nil {
		compat.SupportsLongCacheRetention = *model.Compat.SupportsLongCacheRetention
	}
	return compat
}

func BuildOpenAIResponsesPayload(model Model, context Context, options OpenAIResponsesPayloadOptions) OpenAIResponsesPayload {
	cacheRetention := resolveCacheRetention(options.CacheRetention)
	compat := ResolveOpenAIResponsesCompat(model)
	payload := OpenAIResponsesPayload{
		Model:  model.ID,
		Input:  ConvertOpenAIResponsesMessages(model, context, ConvertOpenAIResponsesOptions{}),
		Stream: true,
		Store:  false,
	}
	if cacheRetention != "none" {
		payload.PromptCacheKey = options.SessionID
	}
	if cacheRetention == "long" && compat.SupportsLongCacheRetention {
		payload.PromptCacheRetention = "24h"
	}
	if options.MaxTokens > 0 {
		payload.MaxOutputTokens = options.MaxTokens
	}
	if options.Temperature != nil {
		payload.Temperature = options.Temperature
	}
	if options.ServiceTier != "" {
		payload.ServiceTier = options.ServiceTier
	}
	if len(context.Tools) > 0 {
		payload.Tools = ConvertOpenAIResponsesTools(context.Tools, true)
	}
	applyOpenAIResponsesReasoning(&payload, model, options)
	return payload
}

func BuildOpenAIResponsesPayloadChecked(model Model, context Context, options OpenAIResponsesPayloadOptions) (OpenAIResponsesPayload, error) {
	if err := ValidateThinkingLevelSupported(model, options.ReasoningEffort); err != nil {
		return OpenAIResponsesPayload{}, err
	}
	return BuildOpenAIResponsesPayload(model, context, options), nil
}

func BuildOpenAIResponsesHeaders(model Model, options OpenAIResponsesPayloadOptions) map[string]string {
	headers := map[string]string{}
	for key, value := range model.Headers {
		headers[key] = value
	}
	cacheRetention := resolveCacheRetention(options.CacheRetention)
	if cacheRetention != "none" && options.SessionID != "" {
		compat := ResolveOpenAIResponsesCompat(model)
		if compat.SendSessionIDHeader {
			headers["session_id"] = options.SessionID
		}
		headers["x-client-request-id"] = options.SessionID
	}
	for key, value := range options.Headers {
		headers[key] = value
	}
	return headers
}

func ConvertOpenAIResponsesTools(tools []Tool, strict bool) []OpenAIResponsesTool {
	result := make([]OpenAIResponsesTool, 0, len(tools))
	for _, tool := range tools {
		result = append(result, OpenAIResponsesTool{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  SchemaToMap(tool.Parameters),
			Strict:      ptrBool(strict),
		})
	}
	return result
}

func OpenAIResponsesServiceTierCostMultiplier(model Model, serviceTier string) float64 {
	switch serviceTier {
	case "flex":
		return 0.5
	case "priority":
		if model.ID == "gpt-5.5" {
			return 2.5
		}
		return 2
	default:
		return 1
	}
}

func ApplyOpenAIResponsesServiceTierPricing(usage *Usage, serviceTier string, model Model) {
	if usage == nil {
		return
	}
	multiplier := OpenAIResponsesServiceTierCostMultiplier(model, serviceTier)
	if multiplier == 1 {
		return
	}
	usage.Cost.Input *= multiplier
	usage.Cost.Output *= multiplier
	usage.Cost.CacheRead *= multiplier
	usage.Cost.CacheWrite *= multiplier
	usage.Cost.Total = usage.Cost.Input + usage.Cost.Output + usage.Cost.CacheRead + usage.Cost.CacheWrite
}

func applyOpenAIResponsesReasoning(payload *OpenAIResponsesPayload, model Model, options OpenAIResponsesPayloadOptions) {
	if !model.Reasoning {
		return
	}
	if options.ReasoningEffort != "" || options.ReasoningSummary != "" {
		effort := options.ReasoningEffort
		if effort == "" {
			effort = "medium"
		}
		effort = mapOpenAIResponsesReasoningEffort(model, effort)
		if effort == "" {
			return
		}
		summary := options.ReasoningSummary
		if summary == "" {
			summary = "auto"
		}
		payload.Reasoning = map[string]string{"effort": effort, "summary": summary}
		payload.Include = []string{"reasoning.encrypted_content"}
		return
	}
	if model.Provider == "github-copilot" {
		return
	}
	if off, ok := model.ThinkingLevelMap["off"]; ok && off == nil {
		return
	}
	effort := "none"
	if off, ok := model.ThinkingLevelMap["off"]; ok && off != nil {
		effort = *off
	}
	payload.Reasoning = map[string]string{"effort": effort}
}

func mapOpenAIResponsesReasoningEffort(model Model, effort string) string {
	if mapped, ok := model.ThinkingLevelMap[effort]; ok {
		if mapped == nil {
			return ""
		}
		return *mapped
	}
	return effort
}
