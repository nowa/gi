package gillmprovider

import "encoding/json"

type OpenAIResponsesCompat struct {
	SendSessionIDHeader        bool
	SupportsLongCacheRetention bool
	SupportsStrictMode         bool
	SupportsOpenAIGrammarTools bool
	SupportsToolSearch         bool
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
	Type         string                     `json:"type"`
	Name         string                     `json:"name"`
	Description  string                     `json:"description,omitempty"`
	Parameters   map[string]any             `json:"parameters,omitempty"`
	Strict       *bool                      `json:"-"`
	StrictNull   bool                       `json:"-"`
	Format       *OpenAIResponsesToolFormat `json:"format,omitempty"`
	DeferLoading bool                       `json:"defer_loading,omitempty"`
}

type OpenAIResponsesToolFormat struct {
	Type       string        `json:"type"`
	Syntax     GrammarFormat `json:"syntax"`
	Definition string        `json:"definition"`
}

type OpenAIResponsesStrictDefault uint8

const (
	OpenAIResponsesStrictDefaultFalse OpenAIResponsesStrictDefault = iota
	OpenAIResponsesStrictDefaultTrue
	OpenAIResponsesStrictDefaultNull
)

type OpenAIResponsesToolOptions struct {
	DefaultStrict              OpenAIResponsesStrictDefault
	SupportsStrictMode         *bool
	SupportsOpenAIGrammarTools bool
	DeferLoading               bool
}

func (tool OpenAIResponsesTool) MarshalJSON() ([]byte, error) {
	type wireTool struct {
		Type         string                     `json:"type"`
		Name         string                     `json:"name"`
		Description  string                     `json:"description,omitempty"`
		Parameters   map[string]any             `json:"parameters,omitempty"`
		Strict       json.RawMessage            `json:"strict,omitempty"`
		Format       *OpenAIResponsesToolFormat `json:"format,omitempty"`
		DeferLoading bool                       `json:"defer_loading,omitempty"`
	}
	var strict json.RawMessage
	switch {
	case tool.StrictNull:
		strict = json.RawMessage("null")
	case tool.Strict != nil:
		raw, err := json.Marshal(*tool.Strict)
		if err != nil {
			return nil, err
		}
		strict = raw
	}
	return json.Marshal(wireTool{
		Type:         tool.Type,
		Name:         tool.Name,
		Description:  tool.Description,
		Parameters:   tool.Parameters,
		Strict:       strict,
		Format:       tool.Format,
		DeferLoading: tool.DeferLoading,
	})
}

func (tool *OpenAIResponsesTool) UnmarshalJSON(data []byte) error {
	type wireTool struct {
		Type         string                     `json:"type"`
		Name         string                     `json:"name"`
		Description  string                     `json:"description"`
		Parameters   map[string]any             `json:"parameters"`
		Strict       json.RawMessage            `json:"strict"`
		Format       *OpenAIResponsesToolFormat `json:"format"`
		DeferLoading bool                       `json:"defer_loading"`
	}
	var wire wireTool
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*tool = OpenAIResponsesTool{
		Type:         wire.Type,
		Name:         wire.Name,
		Description:  wire.Description,
		Parameters:   wire.Parameters,
		Format:       wire.Format,
		DeferLoading: wire.DeferLoading,
	}
	if len(wire.Strict) == 0 {
		return nil
	}
	if string(wire.Strict) == "null" {
		tool.StrictNull = true
		return nil
	}
	var strict bool
	if err := json.Unmarshal(wire.Strict, &strict); err != nil {
		return err
	}
	tool.Strict = ptrBool(strict)
	return nil
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
	if model.Compat.SupportsStrictMode != nil {
		compat.SupportsStrictMode = *model.Compat.SupportsStrictMode
	}
	if model.Compat.SupportsOpenAIGrammarTools != nil {
		compat.SupportsOpenAIGrammarTools = *model.Compat.SupportsOpenAIGrammarTools
	}
	if model.Compat.SupportsToolSearch != nil {
		compat.SupportsToolSearch = *model.Compat.SupportsToolSearch
	}
	return compat
}

func BuildOpenAIResponsesPayload(model Model, context Context, options OpenAIResponsesPayloadOptions) OpenAIResponsesPayload {
	payload, _, _ := buildOpenAIResponsesPayload(model, context, options)
	return payload
}

func buildOpenAIResponsesPayload(
	model Model,
	context Context,
	options OpenAIResponsesPayloadOptions,
) (OpenAIResponsesPayload, OpenAIResponsesSamplingState, error) {
	cacheRetention := resolveCacheRetention(options.CacheRetention)
	compat := ResolveOpenAIResponsesCompat(model)
	requestState, err := ResolveOpenAIResponsesRequestState(
		model,
		context,
		OpenAIResponsesSamplingDefaults{
			Strict: OpenAIResponsesStrictDefaultFalse,
		},
	)
	if err != nil {
		return OpenAIResponsesPayload{}, OpenAIResponsesSamplingState{}, err
	}
	input, err := ConvertOpenAIResponsesMessagesChecked(model, context, ConvertOpenAIResponsesOptions{
		GrammarToolInputProperties: requestState.Sampling.GrammarToolInputProperties,
		DeferredTools:              requestState.ToolPlacement.Deferred,
		ToolOptions:                requestState.Sampling.ToolOptions,
	})
	if err != nil {
		return OpenAIResponsesPayload{}, OpenAIResponsesSamplingState{}, err
	}
	payload := OpenAIResponsesPayload{
		Model:  model.ID,
		Input:  input,
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
	if len(requestState.ToolPlacement.Immediate) > 0 {
		payload.Tools, err = ConvertOpenAIResponsesToolsChecked(
			requestState.ToolPlacement.Immediate,
			requestState.Sampling.ToolOptions,
		)
		if err != nil {
			return OpenAIResponsesPayload{}, OpenAIResponsesSamplingState{}, err
		}
	}
	applyOpenAIResponsesReasoning(&payload, model, options)
	return payload, requestState.Sampling, nil
}

func BuildOpenAIResponsesPayloadChecked(model Model, context Context, options OpenAIResponsesPayloadOptions) (OpenAIResponsesPayload, error) {
	payload, _, err := buildOpenAIResponsesPayloadChecked(model, context, options)
	return payload, err
}

func buildOpenAIResponsesPayloadChecked(
	model Model,
	context Context,
	options OpenAIResponsesPayloadOptions,
) (OpenAIResponsesPayload, OpenAIResponsesSamplingState, error) {
	if err := ValidateThinkingLevelSupported(model, options.ReasoningEffort); err != nil {
		return OpenAIResponsesPayload{}, OpenAIResponsesSamplingState{}, err
	}
	return buildOpenAIResponsesPayload(model, context, options)
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

// ConvertOpenAIResponsesTools preserves the original convenience API. New
// request paths should use ConvertOpenAIResponsesToolsChecked so constrained
// sampling validation errors reach the caller.
func ConvertOpenAIResponsesTools(tools []Tool, strict bool) []OpenAIResponsesTool {
	defaultStrict := OpenAIResponsesStrictDefaultFalse
	if strict {
		defaultStrict = OpenAIResponsesStrictDefaultTrue
	}
	converted, _ := ConvertOpenAIResponsesToolsChecked(tools, OpenAIResponsesToolOptions{
		DefaultStrict:      defaultStrict,
		SupportsStrictMode: ptrBool(true),
	})
	return converted
}

func ConvertOpenAIResponsesToolsChecked(
	tools []Tool,
	options OpenAIResponsesToolOptions,
) ([]OpenAIResponsesTool, error) {
	supportsStrictMode := true
	if options.SupportsStrictMode != nil {
		supportsStrictMode = *options.SupportsStrictMode
	}
	result := make([]OpenAIResponsesTool, 0, len(tools))
	for _, tool := range tools {
		grammar, ok, err := ResolveGrammarConstrainedSampling(tool, options.SupportsOpenAIGrammarTools)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, OpenAIResponsesTool{
				Type:         "custom",
				Name:         tool.Name,
				Description:  tool.Description,
				Format:       &OpenAIResponsesToolFormat{Type: "grammar", Syntax: grammar.Format, Definition: grammar.Definition},
				DeferLoading: options.DeferLoading,
			})
			continue
		}

		strict, err := ResolveJSONSchemaStrictSampling(tool, supportsStrictMode)
		if err != nil {
			return nil, err
		}
		converted := OpenAIResponsesTool{
			Type:         "function",
			Name:         tool.Name,
			Description:  tool.Description,
			Parameters:   SchemaToMap(tool.Parameters),
			DeferLoading: options.DeferLoading,
		}
		if supportsStrictMode {
			converted.Strict = strict
			if converted.Strict == nil {
				switch options.DefaultStrict {
				case OpenAIResponsesStrictDefaultNull:
					converted.StrictNull = true
				case OpenAIResponsesStrictDefaultTrue:
					converted.Strict = ptrBool(true)
				case OpenAIResponsesStrictDefaultFalse:
					converted.Strict = ptrBool(false)
				}
			}
		}
		result = append(result, converted)
	}
	return result, nil
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
