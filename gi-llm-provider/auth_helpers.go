package gillmprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// EnvAPIKeyAuth builds the standard provider auth contract: a stored key wins,
// otherwise environment variables are checked in declaration order.
func EnvAPIKeyAuth(name string, envVars ...string) *APIKeyAuth {
	keys := append([]string(nil), envVars...)
	return &APIKeyAuth{
		Name: name,
		Login: func(ctx context.Context, interaction AuthInteraction) (Credential, error) {
			if interaction == nil {
				return Credential{}, errors.New("auth interaction is required")
			}
			key, err := interaction.Prompt(contextOrBackground(ctx), AuthPrompt{
				Type:    AuthPromptSecret,
				Message: "Enter " + name,
			})
			if err != nil {
				return Credential{}, err
			}
			return Credential{Type: CredentialTypeAPIKey, Key: key}, nil
		},
		Resolve: func(ctx context.Context, input APIKeyResolveInput) (*AuthResult, error) {
			if input.Credential != nil && input.Credential.Key != "" {
				return &AuthResult{
					Auth:   ModelAuth{APIKey: input.Credential.Key},
					Env:    cloneProviderEnv(input.Credential.Env),
					Source: "stored credential",
				}, nil
			}
			authContext := input.Context
			if authContext == nil {
				authContext = DefaultProviderAuthContext()
			}
			for _, envVar := range keys {
				value, ok, err := authContext.Env(contextOrBackground(ctx), envVar)
				if err != nil {
					return nil, err
				}
				if ok && strings.TrimSpace(value) != "" {
					return &AuthResult{
						Auth:   ModelAuth{APIKey: value},
						Source: envVar,
					}, nil
				}
			}
			return nil, nil
		},
	}
}

// OAuthAuthLoader resolves one provider's OAuth implementation on demand.
// Applications register interactive implementations without introducing a
// dependency from the reusable provider package back to a CLI or TUI package.
type OAuthAuthLoader func(ctx context.Context) (*OAuthAuth, error)

var oauthAuthLoaders = struct {
	sync.RWMutex
	values map[string]OAuthAuthLoader
}{values: map[string]OAuthAuthLoader{}}

// RegisterOAuthAuthLoader installs or replaces one provider's lazy OAuth
// implementation. Passing nil unregisters the provider.
func RegisterOAuthAuthLoader(providerID string, loader OAuthAuthLoader) {
	oauthAuthLoaders.Lock()
	defer oauthAuthLoaders.Unlock()
	if loader == nil {
		delete(oauthAuthLoaders.values, providerID)
		return
	}
	oauthAuthLoaders.values[providerID] = loader
}

// UnregisterOAuthAuthLoader removes one provider's lazy OAuth implementation.
func UnregisterOAuthAuthLoader(providerID string) {
	RegisterOAuthAuthLoader(providerID, nil)
}

// LazyOAuthAuth exposes stable OAuth metadata while deferring implementation
// loading until login, refresh, or request-auth derivation is actually used.
func LazyOAuthAuth(name, loginLabel string, loader OAuthAuthLoader) *OAuthAuth {
	var (
		loadMu sync.Mutex
		loaded *OAuthAuth
	)
	load := func(ctx context.Context) (*OAuthAuth, error) {
		loadMu.Lock()
		defer loadMu.Unlock()
		if loaded != nil {
			return loaded, nil
		}
		if loader == nil {
			return nil, errors.New("OAuth loader is not configured")
		}
		candidate, err := loader(contextOrBackground(ctx))
		if err != nil {
			return nil, err
		}
		if candidate == nil {
			return nil, errors.New("OAuth loader returned nil")
		}
		loaded = candidate
		return loaded, nil
	}
	return &OAuthAuth{
		Name:       name,
		LoginLabel: loginLabel,
		Login: func(ctx context.Context, interaction AuthInteraction) (Credential, error) {
			auth, err := load(ctx)
			if err != nil {
				return Credential{}, err
			}
			if auth.Login == nil {
				return Credential{}, errors.New("OAuth login is not configured")
			}
			return auth.Login(contextOrBackground(ctx), interaction)
		},
		Refresh: func(ctx context.Context, credential Credential) (Credential, error) {
			auth, err := load(ctx)
			if err != nil {
				return Credential{}, err
			}
			if auth.Refresh == nil {
				return Credential{}, errors.New("OAuth refresh is not configured")
			}
			return auth.Refresh(contextOrBackground(ctx), credential)
		},
		ToAuth: func(ctx context.Context, credential Credential) (ModelAuth, error) {
			auth, err := load(ctx)
			if err != nil {
				return ModelAuth{}, err
			}
			if auth.ToAuth == nil {
				return ModelAuth{}, errors.New("OAuth auth derivation is not configured")
			}
			return auth.ToAuth(contextOrBackground(ctx), credential)
		},
	}
}

func registeredOAuthAuth(providerID, name, loginLabel string) *OAuthAuth {
	return LazyOAuthAuth(name, loginLabel, func(ctx context.Context) (*OAuthAuth, error) {
		loader := getOAuthAuthLoader(providerID)
		if loader == nil {
			return nil, fmt.Errorf("OAuth loader is not registered for provider %s", providerID)
		}
		return loader(contextOrBackground(ctx))
	})
}

func registeredOrBuiltinOAuthAuth(
	providerID string,
	builtin *OAuthAuth,
) *OAuthAuth {
	if builtin == nil {
		return registeredOAuthAuth(providerID, providerID, "")
	}
	return LazyOAuthAuth(
		builtin.Name,
		builtin.LoginLabel,
		func(ctx context.Context) (*OAuthAuth, error) {
			if loader := getOAuthAuthLoader(providerID); loader != nil {
				return loader(contextOrBackground(ctx))
			}
			return builtin, nil
		},
	)
}

func getOAuthAuthLoader(providerID string) OAuthAuthLoader {
	oauthAuthLoaders.RLock()
	loader := oauthAuthLoaders.values[providerID]
	oauthAuthLoaders.RUnlock()
	return loader
}
