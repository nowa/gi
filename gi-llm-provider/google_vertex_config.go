package gillmprovider

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	GCPVertexCredentialsMarker = "gcp-vertex-credentials"
	// GoogleCloudPlatformScope is the OAuth scope required by Vertex AI.
	GoogleCloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"
)

var (
	googleVertexPlaceholderAPIKeyPattern = regexp.MustCompile(`^<[^>]+>$`)
	googleVertexAPIVersionPattern        = regexp.MustCompile(`^v\d+(?:beta\d*)?$`)
)

type GoogleVertexOptions struct {
	APIKey   string
	Project  string
	Location string
	Env      ProviderEnv
}

type GoogleVertexClientConfig struct {
	VertexAI          bool
	APIKey            string
	Project           string
	Location          string
	APIVersion        string
	GoogleAuthOptions *GoogleAuthOptions
	HTTPOptions       *GoogleVertexHTTPOptions
}

// GoogleAuthOptions captures the request-scoped ADC configuration that Pi
// passes to the Google client. Keeping it separate from the client config
// avoids mutating process-wide environment variables.
type GoogleAuthOptions struct {
	KeyFilename string
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
		config.Project = firstNonEmpty(
			options.Project,
			GetProviderEnvValue("GOOGLE_CLOUD_PROJECT", options.Env),
			GetProviderEnvValue("GCLOUD_PROJECT", options.Env),
		)
		config.Location = firstNonEmpty(
			options.Location,
			GetProviderEnvValue("GOOGLE_CLOUD_LOCATION", options.Env),
		)
		config.GoogleAuthOptions = BuildGoogleAuthOptions(options.Env)
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

// ValidateGoogleVertexClientConfig checks the mutually exclusive Express and
// ADC configuration requirements without performing authentication or I/O.
func ValidateGoogleVertexClientConfig(config GoogleVertexClientConfig) error {
	if config.APIKey != "" {
		return nil
	}
	if config.Project == "" {
		return fmt.Errorf(
			"Vertex AI requires a project ID. Set GOOGLE_CLOUD_PROJECT/GCLOUD_PROJECT or pass project in options.",
		)
	}
	if config.Location == "" {
		return fmt.Errorf(
			"Vertex AI requires a location. Set GOOGLE_CLOUD_LOCATION or pass location in options.",
		)
	}
	return nil
}

// BuildGoogleAuthOptions projects request-scoped provider environment into the
// explicit credential-file option used by an ADC token provider.
func BuildGoogleAuthOptions(env ProviderEnv) *GoogleAuthOptions {
	keyFilename := strings.TrimSpace(GetProviderEnvValue("GOOGLE_APPLICATION_CREDENTIALS", env))
	if keyFilename == "" {
		return nil
	}
	return &GoogleAuthOptions{KeyFilename: keyFilename}
}

func ResolveGoogleVertexAPIKey(options GoogleVertexOptions) string {
	apiKey := strings.TrimSpace(firstNonEmpty(
		options.APIKey,
		GetProviderEnvValue("GOOGLE_CLOUD_API_KEY", options.Env),
	))
	if apiKey == "" || apiKey == GCPVertexCredentialsMarker || IsPlaceholderAPIKey(apiKey) {
		return ""
	}
	return apiKey
}

func IsPlaceholderAPIKey(apiKey string) bool {
	return googleVertexPlaceholderAPIKeyPattern.MatchString(apiKey)
}

func ResolveGoogleVertexCustomBaseURL(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" || strings.Contains(trimmed, "{location}") {
		return ""
	}
	return trimmed
}

func GoogleVertexBaseURLIncludesAPIVersion(baseURL string) bool {
	if parsed, err := url.Parse(baseURL); err == nil {
		for _, part := range strings.Split(parsed.Path, "/") {
			if googleVertexAPIVersionPattern.MatchString(part) {
				return true
			}
		}
	}
	return false
}

// GoogleVertexStreamEndpoint builds the REST endpoint selected by the Google
// Gen AI SDK: Express mode uses the global collection, while ADC requests use
// a project/location resource path unless a collection-scoped custom base URL
// is configured.
func GoogleVertexStreamEndpoint(model Model, config GoogleVertexClientConfig) (string, error) {
	if strings.TrimSpace(model.ID) == "" {
		return "", fmt.Errorf("Google Vertex model ID is required")
	}
	if err := ValidateGoogleVertexClientConfig(config); err != nil {
		return "", err
	}
	baseURL := ""
	customCollection := false
	if config.HTTPOptions != nil {
		baseURL = strings.TrimSpace(config.HTTPOptions.BaseURL)
		customCollection = config.HTTPOptions.BaseURLResourceScope == "COLLECTION"
	}
	if baseURL == "" {
		baseURL = defaultGoogleVertexBaseURL(config)
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse Google Vertex base URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid Google Vertex base URL %q", baseURL)
	}

	resource := googleVertexModelResource(model.ID)
	pathParts := []string{strings.TrimRight(parsed.EscapedPath(), "/")}
	if !GoogleVertexBaseURLIncludesAPIVersion(baseURL) {
		pathParts = append(pathParts, config.APIVersion)
	}
	if config.APIKey == "" && !customCollection && !strings.HasPrefix(resource, "projects/") {
		pathParts = append(
			pathParts,
			"projects",
			url.PathEscape(config.Project),
			"locations",
			url.PathEscape(config.Location),
		)
	}
	pathParts = append(pathParts, resource+":streamGenerateContent")
	rawPath := joinGoogleVertexPath(pathParts...)
	decodedPath, err := url.PathUnescape(rawPath)
	if err != nil {
		return "", fmt.Errorf("decode Google Vertex request path: %w", err)
	}
	parsed.Path = decodedPath
	parsed.RawPath = rawPath

	query := parsed.Query()
	query.Set("alt", "sse")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func defaultGoogleVertexBaseURL(config GoogleVertexClientConfig) string {
	if config.APIKey != "" || config.Location == "global" {
		return "https://aiplatform.googleapis.com"
	}
	if config.Location == "us" || config.Location == "eu" {
		return "https://aiplatform." + config.Location + ".rep.googleapis.com"
	}
	return "https://" + config.Location + "-aiplatform.googleapis.com"
}

func googleVertexModelResource(modelID string) string {
	modelID = strings.Trim(strings.TrimSpace(modelID), "/")
	switch {
	case strings.HasPrefix(modelID, "publishers/"),
		strings.HasPrefix(modelID, "projects/"),
		strings.HasPrefix(modelID, "models/"):
		return escapeGoogleVertexResource(modelID)
	case strings.Contains(modelID, "/"):
		parts := strings.SplitN(modelID, "/", 2)
		return "publishers/" + url.PathEscape(parts[0]) + "/models/" + url.PathEscape(parts[1])
	default:
		return "publishers/google/models/" + url.PathEscape(modelID)
	}
}

func escapeGoogleVertexResource(resource string) string {
	parts := strings.Split(resource, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func joinGoogleVertexPath(parts ...string) string {
	var nonEmpty []string
	for _, part := range parts {
		if trimmed := strings.Trim(part, "/"); trimmed != "" {
			nonEmpty = append(nonEmpty, trimmed)
		}
	}
	return "/" + strings.Join(nonEmpty, "/")
}
