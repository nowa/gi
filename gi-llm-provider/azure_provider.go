package gillmprovider

import (
	"context"
	"net/url"
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
	Store           bool                       `json:"store"`
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
	RegisterBuiltInAPIProvider("azure-openai-responses", NewAzureOpenAIResponsesProvider(nil))
}

func (p AzureOpenAIResponsesProvider) Stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	return p.stream(model, llmContext, options)
}

func (p AzureOpenAIResponsesProvider) StreamSimple(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error) {
	return p.stream(model, llmContext, prepareSimpleStreamOptions(model, llmContext, options))
}

func (p AzureOpenAIResponsesProvider) stream(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error) {
	apiKey := apiKeyOrEnv(model.Provider, options.APIKey, options.Env)
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
		Env:                 options.Env,
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
	azurePayload, sampling, err := buildAzureOpenAIResponsesPayload(model, llmContext, AzureOpenAIResponsesPayloadOptions{
		DeploymentName:   resolveAzureDeploymentName(model, azureOptions.AzureDeploymentName, options.Env),
		MaxTokens:        options.MaxTokens,
		Temperature:      options.Temperature,
		SessionID:        options.SessionID,
		ReasoningEffort:  reasoning,
		ReasoningSummary: metadataString(options.Metadata, "reasoning_summary"),
	})
	if err != nil {
		return streamError(model, "%s", err.Error()), nil
	}
	payload := any(azurePayload)
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
	response, err := postSSEWithRetry(
		ctx,
		httpClientForRequest(p.Client, options),
		azureOpenAIResponsesEndpoint(config),
		headers,
		payload,
		providerRetryOptions(options.MaxRetries, options.MaxRetryDelayMs),
		func(status int, headers map[string]string) error {
			if options.OnResponseStatus == nil {
				return nil
			}
			return options.OnResponseStatus(status, headers, model)
		},
	)
	if err != nil {
		if ctx.Err() != nil {
			return ErrorAssistantStream(AssistantErrorMessage(ctx.Err().Error(), model, true)), nil
		}
		return streamProviderRequestError(model, err, "Azure OpenAI API error"), nil
	}

	stream := NewAssistantMessageEventStream()
	go streamOpenAIResponsesBody(model, response.Body, stream, sampling.GrammarToolInputProperties)
	return stream, nil
}

func BuildAzureOpenAIResponsesPayload(model Model, context Context, options AzureOpenAIResponsesPayloadOptions) AzureOpenAIResponsesPayload {
	payload, _, _ := buildAzureOpenAIResponsesPayload(model, context, options)
	return payload
}

func BuildAzureOpenAIResponsesPayloadChecked(
	model Model,
	context Context,
	options AzureOpenAIResponsesPayloadOptions,
) (AzureOpenAIResponsesPayload, error) {
	payload, _, err := buildAzureOpenAIResponsesPayload(model, context, options)
	return payload, err
}

func buildAzureOpenAIResponsesPayload(
	model Model,
	context Context,
	options AzureOpenAIResponsesPayloadOptions,
) (AzureOpenAIResponsesPayload, OpenAIResponsesSamplingState, error) {
	deployment := options.DeploymentName
	if deployment == "" {
		deployment = model.ID
	}
	sampling, err := ResolveOpenAIResponsesSamplingState(
		model,
		context.Tools,
		OpenAIResponsesSamplingDefaults{
			SupportsStrictMode: true,
			Strict:             OpenAIResponsesStrictDefaultFalse,
		},
	)
	if err != nil {
		return AzureOpenAIResponsesPayload{}, OpenAIResponsesSamplingState{}, err
	}
	input, err := ConvertOpenAIResponsesMessagesChecked(model, context, ConvertOpenAIResponsesOptions{
		AllowedToolCallProviders:   azureToolCallProviders(),
		GrammarToolInputProperties: sampling.GrammarToolInputProperties,
	})
	if err != nil {
		return AzureOpenAIResponsesPayload{}, OpenAIResponsesSamplingState{}, err
	}
	payload := AzureOpenAIResponsesPayload{
		Model:          deployment,
		Input:          input,
		Stream:         true,
		Store:          false,
		PromptCacheKey: options.SessionID,
	}
	if options.MaxTokens > 0 {
		payload.MaxOutputTokens = max(options.MaxTokens, openAIResponsesMinimumOutputTokens)
	}
	if options.Temperature != nil {
		payload.Temperature = options.Temperature
	}
	if len(context.Tools) > 0 {
		payload.Tools, err = ConvertOpenAIResponsesToolsChecked(context.Tools, sampling.ToolOptions)
		if err != nil {
			return AzureOpenAIResponsesPayload{}, OpenAIResponsesSamplingState{}, err
		}
	}
	applyAzureOpenAIResponsesReasoning(&payload, model, options)
	return payload, sampling, nil
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
	return resolveAzureDeploymentName(model, explicit, nil)
}

func resolveAzureDeploymentName(model Model, explicit string, env ProviderEnv) string {
	if explicit != "" {
		return explicit
	}
	if mapped := parseAzureDeploymentNameMap(
		GetProviderEnvValue("AZURE_OPENAI_DEPLOYMENT_NAME_MAP", env),
	)[model.ID]; mapped != "" {
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
