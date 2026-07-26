package gillmprovider

import (
	"context"
	"errors"
	"strings"
)

// anthropicAPIKeyAuth resolves Anthropic's three ambient credential forms into
// the package-wide ModelAuth shape. Bearer tokens stay in Headers so transport
// adapters cannot accidentally apply Claude OAuth request shaping to them.
func anthropicAPIKeyAuth() *APIKeyAuth {
	return &APIKeyAuth{
		Name: "Anthropic API key",
		Login: func(ctx context.Context, interaction AuthInteraction) (Credential, error) {
			if interaction == nil {
				return Credential{}, errors.New("auth interaction is required")
			}
			key, err := interaction.Prompt(contextOrBackground(ctx), AuthPrompt{
				Type:    AuthPromptSecret,
				Message: "Enter Anthropic API key",
			})
			if err != nil {
				return Credential{}, err
			}
			return Credential{Type: CredentialTypeAPIKey, Key: key}, nil
		},
		Resolve: func(ctx context.Context, input APIKeyResolveInput) (*AuthResult, error) {
			if input.Credential != nil && strings.TrimSpace(input.Credential.Key) != "" {
				return &AuthResult{
					Auth:   ModelAuth{APIKey: input.Credential.Key},
					Env:    cloneProviderEnv(input.Credential.Env),
					Source: "stored credential",
				}, nil
			}

			authContext := authContextOrDefault(input.Context)
			authToken, err := authEnv(ctx, authContext, AnthropicAuthTokenEnv)
			if err != nil {
				return nil, err
			}
			if authToken != "" {
				return &AuthResult{
					Auth: ModelAuth{Headers: map[string]string{
						"Authorization": "Bearer " + authToken,
					}},
					Source: AnthropicAuthTokenEnv,
				}, nil
			}

			for _, name := range []string{
				AnthropicOAuthTokenEnv,
				AnthropicAPIKeyEnv,
			} {
				value, err := authEnv(ctx, authContext, name)
				if err != nil {
					return nil, err
				}
				if value != "" {
					return &AuthResult{
						Auth:   ModelAuth{APIKey: value},
						Source: name,
					}, nil
				}
			}
			return nil, nil
		},
	}
}
