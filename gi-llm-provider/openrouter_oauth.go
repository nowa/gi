package gillmprovider

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

const (
	openRouterOAuthAuthorizeURL        = "https://openrouter.ai/auth"
	openRouterOAuthTokenURL            = "https://openrouter.ai/api/v1/auth/keys"
	defaultOpenRouterOAuthCallbackHost = "127.0.0.1"
	defaultOpenRouterOAuthLoginTimeout = 5 * time.Minute
	defaultOpenRouterOAuthTokenTimeout = 30 * time.Second
	permanentOAuthCredentialExpires    = int64(1<<53 - 1)
)

// OpenRouterOAuthOptions configures OpenRouter's loopback PKCE flow.
type OpenRouterOAuthOptions struct {
	Client               HTTPDoer
	AuthContext          AuthContext
	CallbackHost         string
	LoginTimeout         time.Duration
	TokenExchangeTimeout time.Duration
}

type openRouterOAuthRuntime struct {
	generatePKCE     func() (PKCE, error)
	randomCallbackID func() (string, error)
	startCallback    func(
		context.Context,
		openRouterOAuthCallbackOptions,
	) (openRouterOAuthCallbackServer, error)
}

type openRouterOAuthConfig struct {
	client               HTTPDoer
	authContext          AuthContext
	callbackHost         string
	loginTimeout         time.Duration
	tokenExchangeTimeout time.Duration
}

type openRouterOAuthCallbackServer interface {
	CallbackURL() string
	Wait(context.Context) (Credential, error)
	Close() error
}

// NewOpenRouterOAuth creates the OpenRouter one-shot loopback PKCE flow.
func NewOpenRouterOAuth(options OpenRouterOAuthOptions) *OAuthAuth {
	return newOpenRouterOAuth(options, openRouterOAuthRuntime{
		generatePKCE: GeneratePKCE,
		randomCallbackID: func() (string, error) {
			return randomOpenRouterCallbackID(rand.Reader)
		},
		startCallback: startOpenRouterOAuthCallbackServer,
	})
}

func newOpenRouterOAuth(
	options OpenRouterOAuthOptions,
	runtime openRouterOAuthRuntime,
) *OAuthAuth {
	if runtime.generatePKCE == nil {
		runtime.generatePKCE = GeneratePKCE
	}
	if runtime.randomCallbackID == nil {
		runtime.randomCallbackID = func() (string, error) {
			return randomOpenRouterCallbackID(rand.Reader)
		}
	}
	if runtime.startCallback == nil {
		runtime.startCallback = startOpenRouterOAuthCallbackServer
	}
	authContext := options.AuthContext
	if authContext == nil {
		authContext = DefaultProviderAuthContext()
	}
	loginTimeout := options.LoginTimeout
	if loginTimeout <= 0 {
		loginTimeout = defaultOpenRouterOAuthLoginTimeout
	}
	tokenTimeout := options.TokenExchangeTimeout
	if tokenTimeout <= 0 {
		tokenTimeout = defaultOpenRouterOAuthTokenTimeout
	}
	config := openRouterOAuthConfig{
		client:               httpClientOrDefault(options.Client),
		authContext:          authContext,
		callbackHost:         options.CallbackHost,
		loginTimeout:         loginTimeout,
		tokenExchangeTimeout: tokenTimeout,
	}

	return &OAuthAuth{
		Name:       "OpenRouter OAuth",
		LoginLabel: "Sign in with OpenRouter",
		Login: func(
			ctx context.Context,
			interaction AuthInteraction,
		) (Credential, error) {
			if interaction == nil {
				return Credential{}, errors.New(
					"OpenRouter OAuth auth interaction is required",
				)
			}
			ctx = contextOrBackground(ctx)
			if contextError(ctx) != nil {
				return Credential{}, oauthLoginContextError(ctx)
			}
			return loginOpenRouter(
				ctx,
				config,
				interaction,
				runtime,
			)
		},
		Refresh: func(
			ctx context.Context,
			credential Credential,
		) (Credential, error) {
			if err := contextError(contextOrBackground(ctx)); err != nil {
				return Credential{}, err
			}
			if credential.Type != CredentialTypeOAuth ||
				strings.TrimSpace(credential.Access) == "" {
				return Credential{}, errors.New(
					"OpenRouter OAuth credential has no API key",
				)
			}
			return credential.Clone(), nil
		},
		ToAuth: func(
			ctx context.Context,
			credential Credential,
		) (ModelAuth, error) {
			if err := contextError(contextOrBackground(ctx)); err != nil {
				return ModelAuth{}, err
			}
			if credential.Type != CredentialTypeOAuth ||
				strings.TrimSpace(credential.Access) == "" {
				return ModelAuth{}, errors.New(
					"OpenRouter OAuth credential has no API key",
				)
			}
			return ModelAuth{APIKey: credential.Access}, nil
		},
	}
}

func loginOpenRouter(
	ctx context.Context,
	config openRouterOAuthConfig,
	interaction AuthInteraction,
	runtime openRouterOAuthRuntime,
) (Credential, error) {
	pkce, err := runtime.generatePKCE()
	if err != nil {
		return Credential{}, fmt.Errorf(
			"generate OpenRouter OAuth PKCE: %w",
			err,
		)
	}
	if pkce.Verifier == "" || pkce.Challenge == "" {
		return Credential{}, errors.New(
			"generate OpenRouter OAuth PKCE: empty verifier or challenge",
		)
	}
	callbackID, err := runtime.randomCallbackID()
	if err != nil {
		return Credential{}, fmt.Errorf(
			"generate OpenRouter OAuth callback ID: %w",
			err,
		)
	}
	if callbackID == "" {
		return Credential{}, errors.New(
			"generate OpenRouter OAuth callback ID: empty ID",
		)
	}
	callbackHost, err := resolveOpenRouterOAuthCallbackHost(
		ctx,
		config.callbackHost,
		config.authContext,
	)
	if err != nil {
		return Credential{}, err
	}
	callbackPath := "/oauth/callback/" + callbackID
	callback, err := runtime.startCallback(
		ctx,
		openRouterOAuthCallbackOptions{
			Host:         callbackHost,
			Path:         callbackPath,
			Verifier:     pkce.Verifier,
			LoginTimeout: config.loginTimeout,
			Exchange: func(
				exchangeContext context.Context,
				code string,
				verifier string,
			) (Credential, error) {
				return exchangeOpenRouterAuthorizationCode(
					exchangeContext,
					config.client,
					code,
					verifier,
					config.tokenExchangeTimeout,
				)
			},
		},
	)
	if err != nil {
		if contextError(ctx) != nil {
			return Credential{}, oauthLoginContextError(ctx)
		}
		return Credential{}, fmt.Errorf(
			"start OpenRouter OAuth callback server: %w",
			err,
		)
	}
	defer callback.Close()

	authorizeURL, err := url.Parse(openRouterOAuthAuthorizeURL)
	if err != nil {
		return Credential{}, err
	}
	query := authorizeURL.Query()
	query.Set("callback_url", callback.CallbackURL())
	query.Set("code_challenge", pkce.Challenge)
	query.Set("code_challenge_method", "S256")
	authorizeURL.RawQuery = query.Encode()

	interaction.Notify(AuthEvent{
		Type: AuthEventProgress,
		Message: "Listening for OpenRouter OAuth callback on " +
			callback.CallbackURL(),
	})
	interaction.Notify(AuthEvent{
		Type:         AuthEventURL,
		URL:          authorizeURL.String(),
		Instructions: "Complete sign-in in your browser.",
	})

	credential, err := callback.Wait(ctx)
	if err != nil {
		if contextError(ctx) != nil {
			return Credential{}, oauthLoginContextError(ctx)
		}
		return Credential{}, err
	}
	return credential, nil
}

func randomOpenRouterCallbackID(reader io.Reader) (string, error) {
	if reader == nil {
		return "", errors.New("OpenRouter OAuth random source is required")
	}
	var value [16]byte
	if _, err := io.ReadFull(reader, value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		value[0:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:16],
	), nil
}
