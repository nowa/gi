package gillmprovider

// NewOpenRouterImagesRuntimeProvider constructs the built-in OpenRouter image
// provider with the same API-key and OAuth contracts as the text provider.
func NewOpenRouterImagesRuntimeProvider() (*ImagesProvider, error) {
	return CreateImagesProvider(CreateImagesProviderOptions{
		ID:   "openrouter",
		Name: "OpenRouter",
		Auth: ProviderAuth{
			APIKey: EnvAPIKeyAuth("OpenRouter API key", "OPENROUTER_API_KEY"),
			OAuth: registeredOrBuiltinOAuthAuth(
				"openrouter",
				NewOpenRouterOAuth(OpenRouterOAuthOptions{}),
			),
		},
		Models: getBuiltinImageModels("openrouter"),
		API:    NewOpenRouterImagesProvider(nil),
	})
}

// BuiltinImagesProviders constructs every built-in image provider in Pi
// declaration order.
func BuiltinImagesProviders() ([]*ImagesProvider, error) {
	openRouter, err := NewOpenRouterImagesRuntimeProvider()
	if err != nil {
		return nil, err
	}
	return []*ImagesProvider{openRouter}, nil
}

// BuiltinImagesModels creates an isolated ImagesModels collection with every
// built-in image provider registered.
func BuiltinImagesModels(options ...ImagesModelsOptions) (*ImagesModels, error) {
	models := NewImagesModels(options...)
	providers, err := BuiltinImagesProviders()
	if err != nil {
		return nil, err
	}
	for _, provider := range providers {
		if err := models.SetProvider(provider); err != nil {
			return nil, err
		}
	}
	return models, nil
}
