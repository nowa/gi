package gillmprovider

import (
	"net/url"
	"os"
	"strings"
)

type BedrockClientOptions struct {
	Region  string
	Profile string
}

type BedrockClientConfig struct {
	Endpoint string
	Region   string
	Profile  string
}

func ResolveBedrockClientConfig(model Model, options BedrockClientOptions) BedrockClientConfig {
	config := BedrockClientConfig{Profile: options.Profile}
	configuredRegion := GetConfiguredBedrockRegion(options)
	hasConfiguredProfile := HasConfiguredBedrockProfile(options)
	endpointRegion := StandardBedrockEndpointRegion(model.BaseURL)
	useExplicitEndpoint := ShouldUseExplicitBedrockEndpoint(model.BaseURL, configuredRegion, hasConfiguredProfile)
	if useExplicitEndpoint {
		config.Endpoint = model.BaseURL
	}
	switch {
	case configuredRegion != "":
		config.Region = configuredRegion
	case endpointRegion != "" && useExplicitEndpoint:
		config.Region = endpointRegion
	case !hasConfiguredProfile:
		config.Region = "us-east-1"
	}
	return config
}

func GetConfiguredBedrockRegion(options BedrockClientOptions) string {
	return firstNonEmpty(options.Region, os.Getenv("AWS_REGION"), os.Getenv("AWS_DEFAULT_REGION"))
}

func HasConfiguredBedrockProfile(options BedrockClientOptions) bool {
	return firstNonEmpty(options.Profile, os.Getenv("AWS_PROFILE")) != ""
}

func ShouldUseExplicitBedrockEndpoint(baseURL, configuredRegion string, hasConfiguredProfile bool) bool {
	endpointRegion := StandardBedrockEndpointRegion(baseURL)
	if endpointRegion == "" {
		return baseURL != ""
	}
	return configuredRegion == "" && !hasConfiguredProfile
}

func StandardBedrockEndpointRegion(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	const prefix = "bedrock-runtime."
	const suffix = ".amazonaws.com"
	if !strings.HasPrefix(host, prefix) || !strings.HasSuffix(host, suffix) {
		return ""
	}
	region := strings.TrimSuffix(strings.TrimPrefix(host, prefix), suffix)
	if region == "" || strings.Contains(region, ".") {
		return ""
	}
	return region
}
