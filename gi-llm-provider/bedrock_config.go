package gillmprovider

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	bedrockInferenceProfileARNPattern = regexp.MustCompile(
		`^arn:aws(?:-[a-z0-9-]+)?:bedrock:([a-z0-9-]+):`,
	)
	standardBedrockEndpointPattern = regexp.MustCompile(
		`^bedrock-runtime(?:-fips)?\.([a-z0-9-]+)\.amazonaws\.com(?:\.cn)?$`,
	)
)

type BedrockClientOptions struct {
	Region      string
	Profile     string
	APIKey      string
	BearerToken string
	Env         ProviderEnv
}

type BedrockCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

type BedrockClientConfig struct {
	Endpoint    string
	Region      string
	Profile     string
	Credentials *BedrockCredentials
	BearerToken string
	SkipAuth    bool
	ProxyURL    string
	ForceHTTP1  bool
}

func ResolveBedrockClientConfig(model Model, options BedrockClientOptions) BedrockClientConfig {
	config := BedrockClientConfig{Profile: GetConfiguredBedrockProfile(options)}
	configuredRegion := GetConfiguredBedrockRegion(options)
	hasAmbientConfiguredProfile := GetProviderEnvValue("AWS_PROFILE", nil) != ""
	endpointRegion := StandardBedrockEndpointRegion(model.BaseURL)
	inferenceProfileRegion := BedrockInferenceProfileRegion(model.ID)
	useExplicitEndpoint := ShouldUseExplicitBedrockEndpoint(
		model.BaseURL,
		configuredRegion,
		hasAmbientConfiguredProfile,
	)
	if useExplicitEndpoint {
		config.Endpoint = model.BaseURL
	}
	switch {
	case inferenceProfileRegion != "":
		config.Region = inferenceProfileRegion
	case configuredRegion != "":
		config.Region = configuredRegion
	case endpointRegion != "" && useExplicitEndpoint:
		config.Region = endpointRegion
	case !hasAmbientConfiguredProfile:
		config.Region = "us-east-1"
	}
	config.SkipAuth = GetProviderEnvValue("AWS_BEDROCK_SKIP_AUTH", options.Env) == "1"
	config.BearerToken = firstNonEmpty(
		options.BearerToken,
		options.APIKey,
		GetProviderEnvValue("AWS_BEARER_TOKEN_BEDROCK", options.Env),
	)
	if config.SkipAuth {
		config.BearerToken = ""
		config.Credentials = &BedrockCredentials{
			AccessKeyID:     "dummy-access-key",
			SecretAccessKey: "dummy-secret-key",
		}
	} else {
		config.Credentials = GetConfiguredBedrockCredentials(options.Env)
	}
	if proxy, err := ResolveHTTPProxyURLForTargetWithEnv(model.BaseURL, options.Env); err == nil && proxy != nil {
		config.ProxyURL = proxy.String()
		config.ForceHTTP1 = true
	}
	if GetProviderEnvValue("AWS_BEDROCK_FORCE_HTTP1", options.Env) == "1" {
		config.ForceHTTP1 = true
	}
	return config
}

// ResolveBedrockClientConfigChecked resolves the complete live-client
// configuration and surfaces proxy validation errors before the AWS SDK loads
// ambient credentials or profiles.
func ResolveBedrockClientConfigChecked(
	model Model,
	options BedrockClientOptions,
) (BedrockClientConfig, error) {
	config := ResolveBedrockClientConfig(model, options)
	proxy, err := ResolveHTTPProxyURLForTargetWithEnv(model.BaseURL, options.Env)
	if err != nil {
		return BedrockClientConfig{}, fmt.Errorf("resolve Bedrock proxy: %w", err)
	}
	config.ProxyURL = ""
	if proxy != nil {
		config.ProxyURL = proxy.String()
		config.ForceHTTP1 = true
	}
	return config, nil
}

func GetConfiguredBedrockCredentials(env ProviderEnv) *BedrockCredentials {
	accessKeyID := strings.TrimSpace(GetProviderEnvValue("AWS_ACCESS_KEY_ID", env))
	secretAccessKey := strings.TrimSpace(GetProviderEnvValue("AWS_SECRET_ACCESS_KEY", env))
	if accessKeyID == "" || secretAccessKey == "" {
		return nil
	}
	return &BedrockCredentials{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		SessionToken:    strings.TrimSpace(GetProviderEnvValue("AWS_SESSION_TOKEN", env)),
	}
}

func GetConfiguredBedrockRegion(options BedrockClientOptions) string {
	return firstNonEmpty(
		options.Region,
		GetProviderEnvValue("AWS_REGION", options.Env),
		GetProviderEnvValue("AWS_DEFAULT_REGION", options.Env),
	)
}

func HasConfiguredBedrockProfile(options BedrockClientOptions) bool {
	return GetConfiguredBedrockProfile(options) != ""
}

func GetConfiguredBedrockProfile(options BedrockClientOptions) string {
	return firstNonEmpty(
		options.Profile,
		GetProviderEnvValue("AWS_PROFILE", options.Env),
	)
}

func ShouldUseExplicitBedrockEndpoint(baseURL, configuredRegion string, hasConfiguredProfile bool) bool {
	endpointRegion := StandardBedrockEndpointRegion(baseURL)
	if endpointRegion == "" {
		return baseURL != ""
	}
	return configuredRegion == "" && !hasConfiguredProfile
}

func BedrockInferenceProfileRegion(modelID string) string {
	match := bedrockInferenceProfileARNPattern.FindStringSubmatch(modelID)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func StandardBedrockEndpointRegion(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	match := standardBedrockEndpointPattern.FindStringSubmatch(
		strings.ToLower(parsed.Hostname()),
	)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}
