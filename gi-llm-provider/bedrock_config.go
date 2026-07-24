package gillmprovider

import (
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
	Region  string
	Profile string
	Env     ProviderEnv
}

type BedrockClientConfig struct {
	Endpoint string
	Region   string
	Profile  string
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
	return config
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
