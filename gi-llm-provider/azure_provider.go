package gillmprovider

import (
	"context"
	"net/url"
	"os"
	"strings"
)

type AzureOpenAIResponsesPayloadOptions struct {
	DeploymentName   string
	MaxTokens        int
	Temperature      *float64
	SessionID        string
	ReasoningEffort  string
	ReasoningSummary string
}

type AzureOpenAIResponsesPayload struct {
	Model           string                     `json:"model"`
	Input           []OpenAIResponsesInputItem `json:"input"`
	Stream          bool                       `json:"stream"`
	PromptCacheKey  string                     `json:"prompt_cache_key,omitempty"`
	MaxOutputTokens int                        `json:"max_output_tokens,omitempty"`
	Temperature     *float64                   `json:"temperature,omitempty"`
	Tools           []OpenAIResponsesTool      `json:"tools,omitempty"`
	Reasoning       map[string]string          `json:"reasoning,omitempty"`
	Include         []string                   `json:"include,omitempty"`
}

type AzureOpenAIResponsesProvider struct {
	Client HTTPDoer
}

func NewAzureOpenAIResponsesProvider(client HTTPDoer) AzureOpenAIResponsesProvider {
	return AzureOpenAIResponsesProvider{Client: httpClientOrDefault(client)}
}

func init() {
	RegisterAPIProvider("azure-openai-responses", NewAzureOpenAIResponsesProvider(nil))
}

func (p AzureOpenAIResponsesProvider) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	return p.StreamSimple(model, llmContext, options)
}

func (p AzureOpenAIResponsesProvider) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	apiKey := apiKeyOrEnv(model.Provider, options.APIKey)
	if apiKey == "" {
		return streamError(model, "missing API key for provider %s", model.Provider), nil
	}
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	azureOptions := AzureOpenAIResponsesOptions{
		AzureAPIVersion:     metadataString(options.Metadata, "azure_api_version"),
		AzureResourceName:   metadataString(options.Metadata, "azure_resource_name"),
		AzureBaseURL:        metadataString(options.Metadata, "azure_base_url"),
		AzureDeploymentName: metadataString(options.Metadata, "azure_deployment_name"),
	}
	config, err := ResolveAzureOpenAIConfig(model, azureOptions)
	if err != nil {
		return streamError(model, "%s", err.Error()), nil
	}
	reasoning := ""
	if options.Reasoning != "" {
		reasoning = ClampThinkingLevel(model, options.Reasoning)
		if reasoning == "off" {
			reasoning = ""
		}
	}
	payload := any(BuildAzureOpenAIResponsesPayload(model, llmContext, AzureOpenAIResponsesPayloadOptions{
		DeploymentName:   ResolveAzureDeploymentName(model, azureOptions.AzureDeploymentName),
		MaxTokens:        options.MaxTokens,
		Temperature:      options.Temperature,
		SessionID:        options.SessionID,
		ReasoningEffort:  reasoning,
		ReasoningSummary: metadataString(options.Metadata, "reasoning_summary"),
	}))
	if options.OnPayload != nil {
		next, replace, err := options.OnPayload(payload, model)
		if err != nil {
			return streamError(model, "%s", err.Error()), nil
		}
		if replace {
			payload = next
		}
	}
	headers := azureOpenAIResponsesHeaders(model, options, apiKey)
	response, err := postSSE(ctx, httpClientOrDefault(p.Client), azureOpenAIResponsesEndpoint(config), headers, payload)
	if err != nil {
		return streamError(model, "request failed: %v", err), nil
	}
	if options.OnResponseStatus != nil {
		if err := options.OnResponseStatus(response.StatusCode, responseHeaders(response.Header), model); err != nil {
			response.Body.Close()
			return streamError(model, "%s", err.Error()), nil
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return responseErrorStream(model, response), nil
	}

	stream := NewAssistantMessageEventStream()
	go streamOpenAIResponsesBody(model, response.Body, stream)
	return stream, nil
}

func BuildAzureOpenAIResponsesPayload(model Model, context Context, options AzureOpenAIResponsesPayloadOptions) AzureOpenAIResponsesPayload {
	deployment := options.DeploymentName
	if deployment == "" {
		deployment = model.ID
	}
	payload := AzureOpenAIResponsesPayload{
		Model:          deployment,
		Input:          ConvertOpenAIResponsesMessages(model, context, ConvertOpenAIResponsesOptions{AllowedToolCallProviders: azureToolCallProviders()}),
		Stream:         true,
		PromptCacheKey: options.SessionID,
	}
	if options.MaxTokens > 0 {
		payload.MaxOutputTokens = options.MaxTokens
	}
	if options.Temperature != nil {
		payload.Temperature = options.Temperature
	}
	if len(context.Tools) > 0 {
		payload.Tools = ConvertOpenAIResponsesTools(context.Tools, false)
	}
	applyAzureOpenAIResponsesReasoning(&payload, model, options)
	return payload
}

func applyAzureOpenAIResponsesReasoning(payload *AzureOpenAIResponsesPayload, model Model, options AzureOpenAIResponsesPayloadOptions) {
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
	if off, ok := model.ThinkingLevelMap["off"]; ok && off == nil {
		return
	}
	effort := "none"
	if off, ok := model.ThinkingLevelMap["off"]; ok && off != nil {
		effort = *off
	}
	payload.Reasoning = map[string]string{"effort": effort}
}

func ResolveAzureDeploymentName(model Model, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if mapped := parseAzureDeploymentNameMap(os.Getenv("AZURE_OPENAI_DEPLOYMENT_NAME_MAP"))[model.ID]; mapped != "" {
		return mapped
	}
	return model.ID
}

func parseAzureDeploymentNameMap(value string) map[string]string {
	result := map[string]string{}
	for _, entry := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		modelID, deployment, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		modelID = strings.TrimSpace(modelID)
		deployment = strings.TrimSpace(deployment)
		if modelID != "" && deployment != "" {
			result[modelID] = deployment
		}
	}
	return result
}

func azureOpenAIResponsesHeaders(model Model, options SimpleStreamOptions, apiKey string) map[string]string {
	headers := map[string]string{}
	for key, value := range model.Headers {
		headers[key] = value
	}
	for key, value := range options.Headers {
		headers[key] = value
	}
	headers["api-key"] = apiKey
	return headers
}

func azureOpenAIResponsesEndpoint(config AzureOpenAIConfig) string {
	parsed, err := url.Parse(config.BaseURL)
	if err != nil {
		return config.BaseURL
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/responses"
	query := parsed.Query()
	if config.APIVersion != "" {
		query.Set("api-version", config.APIVersion)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func azureToolCallProviders() map[string]bool {
	return map[string]bool{
		"openai":                 true,
		"openai-codex":           true,
		"opencode":               true,
		"azure-openai-responses": true,
	}
}
