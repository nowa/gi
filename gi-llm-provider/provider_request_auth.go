package gillmprovider

// providerRequestAuth is the resolved authentication state for one transport
// request. HeaderOnly keeps caller-managed proxy authentication distinct from
// API keys so adapters never need a fake SDK key such as Pi's "unused" marker.
type providerRequestAuth struct {
	APIKey     string
	HeaderOnly bool
}

func (auth providerRequestAuth) Configured() bool {
	return auth.APIKey != "" || auth.HeaderOnly
}

func resolveProviderRequestAuth(
	provider string,
	explicitAPIKey string,
	env ProviderEnv,
	headers map[string]string,
	acceptedHeaderNames ...string,
) providerRequestAuth {
	if explicitAPIKey != "" {
		return providerRequestAuth{APIKey: explicitAPIKey}
	}
	for _, name := range acceptedHeaderNames {
		if hasNonBlankHeaderCaseInsensitive(headers, name) {
			return providerRequestAuth{HeaderOnly: true}
		}
	}
	if apiKey := apiKeyOrEnv(provider, "", env); apiKey != "" {
		return providerRequestAuth{APIKey: apiKey}
	}
	return providerRequestAuth{}
}
