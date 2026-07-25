package gillmprovider

import "strings"

const ambientAuthMarker = "<authenticated>"

// hasExplicitAPIKey distinguishes a real request override from an omitted or
// whitespace-only value.
func hasExplicitAPIKey(apiKey string) bool {
	return strings.TrimSpace(apiKey) != ""
}

// withEnvAPIKey preserves the legacy package-level Stream contract for custom
// API providers. Built-in providers still resolve environment values at their
// own typed request boundary, so applying this projection is idempotent.
func withEnvAPIKey(model Model, options StreamOptions) StreamOptions {
	if hasExplicitAPIKey(options.APIKey) {
		return options
	}
	apiKey := GetEnvAPIKeyWithOverrides(model.Provider, options.Env)
	if apiKey == "" || apiKey == ambientAuthMarker {
		return options
	}
	options.APIKey = apiKey
	return options
}

// hasResolvedCloudflareAuth reports whether a compatibility request already
// carries direct or AI Gateway authentication.
func hasResolvedCloudflareAuth(options StreamOptions) bool {
	return hasExplicitAPIKey(options.APIKey) ||
		hasNonBlankHeaderCaseInsensitive(
			options.Headers,
			"cf-aig-authorization",
		)
}
