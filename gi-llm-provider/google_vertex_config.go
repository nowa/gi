package gillmprovider

import (
	"os"
	"regexp"
	"strings"
)

const GCPVertexCredentialsMarker = "gcp-vertex-credentials"

type GoogleVertexOptions struct {
	APIKey   string
	Project  string
	Location string
}

type GoogleVertexClientConfig struct {
	VertexAI    bool
	APIKey      string
	Project     string
	Location    string
	APIVersion  string
	HTTPOptions *GoogleVertexHTTPOptions
}

type GoogleVertexHTTPOptions struct {
	BaseURL              string
	BaseURLResourceScope string
	APIVersion           string
}

func ResolveGoogleVertexClientConfig(model Model, options GoogleVertexOptions) GoogleVertexClientConfig {
	apiKey := ResolveGoogleVertexAPIKey(options)
	config := GoogleVertexClientConfig{VertexAI: true, APIVersion: "v1"}
	if apiKey != "" {
		config.APIKey = apiKey
	} else {
		config.Project = firstNonEmpty(options.Project, os.Getenv("GOOGLE_CLOUD_PROJECT"), os.Getenv("GCLOUD_PROJECT"))
		config.Location = firstNonEmpty(options.Location, os.Getenv("GOOGLE_CLOUD_LOCATION"))
	}
	if baseURL := ResolveGoogleVertexCustomBaseURL(model.BaseURL); baseURL != "" {
		config.HTTPOptions = &GoogleVertexHTTPOptions{
			BaseURL:              baseURL,
			BaseURLResourceScope: "COLLECTION",
		}
		if GoogleVertexBaseURLIncludesAPIVersion(baseURL) {
			config.HTTPOptions.APIVersion = ""
		}
	}
	return config
}

func ResolveGoogleVertexAPIKey(options GoogleVertexOptions) string {
	apiKey := strings.TrimSpace(firstNonEmpty(options.APIKey, os.Getenv("GOOGLE_CLOUD_API_KEY")))
	if apiKey == "" || apiKey == GCPVertexCredentialsMarker || IsPlaceholderAPIKey(apiKey) {
		return ""
	}
	return apiKey
}

func IsPlaceholderAPIKey(apiKey string) bool {
	return regexp.MustCompile(`^<[^>]+>$`).MatchString(apiKey)
}

func ResolveGoogleVertexCustomBaseURL(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" || strings.Contains(trimmed, "{location}") {
		return ""
	}
	return trimmed
}

func GoogleVertexBaseURLIncludesAPIVersion(baseURL string) bool {
	return regexp.MustCompile(`(?:^|/)v\d+(?:beta\d*)?(?:/|$)`).MatchString(baseURL)
}
