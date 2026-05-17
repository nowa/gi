package gillmprovider

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

const DefaultAzureOpenAIAPIVersion = "v1"

type AzureOpenAIResponsesOptions struct {
	AzureAPIVersion     string
	AzureResourceName   string
	AzureBaseURL        string
	AzureDeploymentName string
}

type AzureOpenAIConfig struct {
	BaseURL    string
	APIVersion string
}

func NormalizeAzureOpenAIBaseURL(baseURL string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("Invalid Azure OpenAI base URL: %s", baseURL)
	}
	isAzureHost := strings.HasSuffix(parsed.Hostname(), ".openai.azure.com") ||
		strings.HasSuffix(parsed.Hostname(), ".cognitiveservices.azure.com")
	normalizedPath := strings.TrimRight(parsed.Path, "/")
	if isAzureHost && (normalizedPath == "" || normalizedPath == "/" || normalizedPath == "/openai") {
		parsed.Path = "/openai/v1"
		parsed.RawQuery = ""
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func BuildDefaultAzureOpenAIBaseURL(resourceName string) string {
	return "https://" + resourceName + ".openai.azure.com/openai/v1"
}

func ResolveAzureOpenAIConfig(model Model, options AzureOpenAIResponsesOptions) (AzureOpenAIConfig, error) {
	apiVersion := firstNonEmpty(options.AzureAPIVersion, os.Getenv("AZURE_OPENAI_API_VERSION"), DefaultAzureOpenAIAPIVersion)
	baseURL := firstNonEmpty(strings.TrimSpace(options.AzureBaseURL), strings.TrimSpace(os.Getenv("AZURE_OPENAI_BASE_URL")))
	resourceName := firstNonEmpty(options.AzureResourceName, os.Getenv("AZURE_OPENAI_RESOURCE_NAME"))
	if baseURL == "" && resourceName != "" {
		baseURL = BuildDefaultAzureOpenAIBaseURL(resourceName)
	}
	if baseURL == "" {
		baseURL = model.BaseURL
	}
	if baseURL == "" {
		return AzureOpenAIConfig{}, fmt.Errorf("Azure OpenAI base URL is required")
	}
	normalized, err := NormalizeAzureOpenAIBaseURL(baseURL)
	if err != nil {
		return AzureOpenAIConfig{}, err
	}
	return AzureOpenAIConfig{BaseURL: normalized, APIVersion: apiVersion}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
