package gillmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
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
	apiKey := apiKeyOrEnv(model.Provider, options.APIKey)
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
	response, err := postJSON(ctx, httpClientOrDefault(p.Client), openRouterImagesEndpoint(model.BaseURL), openRouterImagesHeaders(model, options, apiKey), payload)
	if err != nil {
		if ctx.Err() != nil {
			return AbortedImages(model, ctx.Err()), nil
		}
		return ErrorImages(model, fmt.Errorf("request failed: %w", err)), nil
	}
	defer response.Body.Close()
	if options.OnResponse != nil {
		if err := options.OnResponse(response.StatusCode, responseHeaders(response.Header), model); err != nil {
			return ErrorImages(model, err), nil
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = response.Status
		}
		return ErrorImages(model, fmt.Errorf("provider returned HTTP %d: %s", response.StatusCode, message)), nil
	}
	var parsed OpenRouterImagesResponse
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return ErrorImages(model, fmt.Errorf("decode OpenRouter images response: %w", err)), nil
	}
	return ParseOpenRouterImagesResponse(model, parsed), nil
}

func openRouterImagesHeaders(model ImagesModel, options ImagesOptions, apiKey string) map[string]string {
	headers := map[string]string{}
	for key, value := range model.Headers {
		headers[key] = value
	}
	for key, value := range options.Headers {
		headers[key] = value
	}
	headers["Authorization"] = "Bearer " + apiKey
	return headers
}

func openRouterImagesEndpoint(baseURL string) string {
	return appendEndpoint(baseURL, "https://openrouter.ai/api/v1", "/chat/completions")
}
