package gillmprovider

import (
	"context"
	"encoding/json"
	"fmt"
)

type OpenRouterImagesProvider struct {
	Client HTTPDoer
}

func NewOpenRouterImagesProvider(client HTTPDoer) OpenRouterImagesProvider {
	return OpenRouterImagesProvider{Client: httpClientOrDefault(client)}
}

func init() {
	RegisterImagesAPIProvider("openrouter-images", NewOpenRouterImagesProvider(nil))
}

func (p OpenRouterImagesProvider) GenerateImages(model ImagesModel, imagesContext ImagesContext, options ImagesOptions) (AssistantImages, error) {
	apiKey := options.APIKey
	if options.APIKeyOverride == nil {
		apiKey = apiKeyOrEnv(model.Provider, apiKey, options.Env)
	} else {
		apiKey = *options.APIKeyOverride
	}
	if apiKey == "" {
		return ErrorImages(model, fmt.Errorf("missing API key for provider %s", model.Provider)), nil
	}
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	payload := any(BuildOpenRouterImagesPayload(model, imagesContext))
	if options.OnPayload != nil {
		next, replace, err := options.OnPayload(payload, model)
		if err != nil {
			return ErrorImages(model, err), nil
		}
		if replace {
			payload = next
		}
	}
	response, err := postJSONWithRetry(
		ctx,
		httpClientOrDefault(p.Client),
		openRouterImagesEndpoint(model.BaseURL),
		openRouterImagesHeaders(model, options, apiKey),
		payload,
		providerRetryOptions(options.MaxRetries, options.MaxRetryDelayMs),
		func(status int, headers map[string]string) error {
			if options.OnResponse == nil {
				return nil
			}
			return options.OnResponse(status, headers, model)
		},
	)
	if err != nil {
		if ctx.Err() != nil {
			return AbortedImages(model, ctx.Err()), nil
		}
		return ErrorImages(model, fmt.Errorf("%s", FormatProviderError(NormalizeProviderError(err)))), nil
	}
	defer response.Body.Close()
	var parsed OpenRouterImagesResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return ErrorImages(model, fmt.Errorf("decode OpenRouter images response: %w", err)), nil
	}
	return ParseOpenRouterImagesResponse(model, parsed), nil
}

func openRouterImagesHeaders(model ImagesModel, options ImagesOptions, apiKey string) map[string]string {
	headers := mergeHeadersCaseInsensitive(model.Headers, options.Headers)
	if headers == nil {
		headers = map[string]string{}
	}
	removeHeaderCaseInsensitive(headers, "Authorization")
	headers["Authorization"] = "Bearer " + apiKey
	return applyHeaderRemovals(headers, options.HeaderRemovals)
}

func openRouterImagesEndpoint(baseURL string) string {
	return appendEndpoint(baseURL, "https://openrouter.ai/api/v1", "/chat/completions")
}
