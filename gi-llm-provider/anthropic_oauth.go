package gillmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	anthropicOAuthAuthorizeURL = "https://claude.ai/oauth/authorize"
	anthropicOAuthCallbackPort = "53692"
	anthropicOAuthCallbackPath = "/callback"
	anthropicOAuthRedirectURI  = "http://localhost:53692/callback"
	anthropicOAuthScopes       = "org:create_api_key user:profile " +
		"user:inference user:sessions:claude_code user:mcp_servers " +
		"user:file_upload"
	anthropicOAuthRefreshSkew    = 5 * time.Minute
	defaultAnthropicOAuthTimeout = 30 * time.Second
	maxAnthropicOAuthBodyBytes   = 1 << 20
)

// AnthropicOAuthOptions configures the provider-owned Claude Pro/Max OAuth
// flow. The zero value uses the Pi-compatible endpoints, callback address, and
// request timeout.
type AnthropicOAuthOptions struct {
	Client         HTTPDoer
	AuthContext    AuthContext
	CallbackHost   string
	RequestTimeout time.Duration
}

type anthropicOAuthConfig struct {
	client         HTTPDoer
	authContext    AuthContext
	callbackHost   string
	requestTimeout time.Duration
}

type anthropicOAuthRuntime struct {
	now           func() time.Time
	generatePKCE  func() (PKCE, error)
	startCallback func(
		context.Context,
		oauthLoopbackCallbackOptions,
	) (oauthAuthorizationCodeServer, error)
}

type anthropicOAuthFlow struct {
	PKCE         PKCE
	State        string
	RedirectURI  string
	AuthorizeURL string
}

type anthropicOAuthTokenResponse struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	ExpiresIn    float64 `json:"expires_in"`
}

// NewAnthropicOAuth creates the reusable Claude Pro/Max authorization-code
// flow. Applications may replace it with RegisterOAuthAuthLoader.
func NewAnthropicOAuth(options AnthropicOAuthOptions) *OAuthAuth {
	return newAnthropicOAuth(options, anthropicOAuthRuntime{
		now:           time.Now,
		generatePKCE:  GeneratePKCE,
		startCallback: startOAuthAuthorizationCodeServer,
	})
}

func newAnthropicOAuth(
	options AnthropicOAuthOptions,
	runtime anthropicOAuthRuntime,
) *OAuthAuth {
	if runtime.now == nil {
		runtime.now = time.Now
	}
	if runtime.generatePKCE == nil {
		runtime.generatePKCE = GeneratePKCE
	}
	if runtime.startCallback == nil {
		runtime.startCallback = startOAuthAuthorizationCodeServer
	}
	authContext := options.AuthContext
	if authContext == nil {
		authContext = DefaultProviderAuthContext()
	}
	requestTimeout := options.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = defaultAnthropicOAuthTimeout
	}
	config := anthropicOAuthConfig{
		client:         httpClientOrDefault(options.Client),
		authContext:    authContext,
		callbackHost:   options.CallbackHost,
		requestTimeout: requestTimeout,
	}

	return &OAuthAuth{
		Name: "Anthropic (Claude Pro/Max)",
		Login: func(
			ctx context.Context,
			interaction AuthInteraction,
		) (Credential, error) {
			if interaction == nil {
				return Credential{}, errors.New(
					"Anthropic OAuth auth interaction is required",
				)
			}
			return loginAnthropicOAuth(
				contextOrBackground(ctx),
				config,
				interaction,
				runtime,
			)
		},
		Refresh: func(
			ctx context.Context,
			credential Credential,
		) (Credential, error) {
			if credential.Type != CredentialTypeOAuth ||
				strings.TrimSpace(credential.Refresh) == "" {
				return Credential{}, errors.New(
					"Anthropic OAuth credential has no refresh token",
				)
			}
			refreshed, err := refreshAnthropicOAuthToken(
				contextOrBackground(ctx),
				config,
				credential.Refresh,
				runtime.now,
			)
			if err != nil {
				return Credential{}, err
			}
			return mergeRefreshedOAuthCredential(
				credential,
				refreshed,
			), nil
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
					"Anthropic OAuth credential has no access token",
				)
			}
			return ModelAuth{APIKey: credential.Access}, nil
		},
	}
}

func loginAnthropicOAuth(
	ctx context.Context,
	config anthropicOAuthConfig,
	interaction AuthInteraction,
	runtime anthropicOAuthRuntime,
) (Credential, error) {
	if err := contextError(ctx); err != nil {
		return Credential{}, oauthLoginContextError(ctx)
	}
	flow, err := createAnthropicAuthorizationFlow(runtime.generatePKCE)
	if err != nil {
		return Credential{}, err
	}
	callbackHost, err := resolveOAuthCallbackHost(
		ctx,
		config.callbackHost,
		config.authContext,
	)
	if err != nil {
		return Credential{}, err
	}
	callback, err := runtime.startCallback(
		ctx,
		oauthLoopbackCallbackOptions{
			Host:           callbackHost,
			Port:           anthropicOAuthCallbackPort,
			Path:           anthropicOAuthCallbackPath,
			ExpectedState:  flow.State,
			ProviderName:   "Anthropic",
			SuccessMessage: "Anthropic authentication completed. You can close this window.",
		},
	)
	if err != nil {
		if contextError(ctx) != nil {
			return Credential{}, oauthLoginContextError(ctx)
		}
		return Credential{}, fmt.Errorf(
			"start Anthropic OAuth callback server: %w",
			err,
		)
	}
	defer callback.Close()

	interaction.Notify(AuthEvent{
		Type: AuthEventURL,
		URL:  flow.AuthorizeURL,
		Instructions: "Complete login in your browser. If the browser is " +
			"on another machine, paste the final redirect URL here.",
	})
	authorization, err := waitForOAuthAuthorizationCode(
		ctx,
		interaction,
		callback,
		AuthPrompt{
			Type: AuthPromptManualCode,
			Message: "Complete login in your browser, or paste the " +
				"authorization code / redirect URL here:",
			Placeholder: anthropicOAuthRedirectURI,
		},
		flow.State,
	)
	if err != nil {
		if contextError(ctx) != nil {
			return Credential{}, oauthLoginContextError(ctx)
		}
		return Credential{}, err
	}

	interaction.Notify(AuthEvent{
		Type:    AuthEventProgress,
		Message: "Exchanging authorization code for tokens...",
	})
	return exchangeAnthropicAuthorizationCode(
		ctx,
		config,
		authorization.Code,
		authorization.State,
		flow.PKCE.Verifier,
		flow.RedirectURI,
		runtime.now,
	)
}

func createAnthropicAuthorizationFlow(
	generatePKCE func() (PKCE, error),
) (anthropicOAuthFlow, error) {
	pkce, err := generatePKCE()
	if err != nil {
		return anthropicOAuthFlow{}, fmt.Errorf(
			"generate Anthropic OAuth PKCE: %w",
			err,
		)
	}
	if pkce.Verifier == "" || pkce.Challenge == "" {
		return anthropicOAuthFlow{}, errors.New(
			"generate Anthropic OAuth PKCE: empty verifier or challenge",
		)
	}
	authorizeURL, err := url.Parse(anthropicOAuthAuthorizeURL)
	if err != nil {
		return anthropicOAuthFlow{}, err
	}
	query := authorizeURL.Query()
	query.Set("code", "true")
	query.Set("client_id", AnthropicOAuthClientID)
	query.Set("response_type", "code")
	query.Set("redirect_uri", anthropicOAuthRedirectURI)
	query.Set("scope", anthropicOAuthScopes)
	query.Set("code_challenge", pkce.Challenge)
	query.Set("code_challenge_method", "S256")
	query.Set("state", pkce.Verifier)
	authorizeURL.RawQuery = query.Encode()
	return anthropicOAuthFlow{
		PKCE:         pkce,
		State:        pkce.Verifier,
		RedirectURI:  anthropicOAuthRedirectURI,
		AuthorizeURL: authorizeURL.String(),
	}, nil
}

func exchangeAnthropicAuthorizationCode(
	ctx context.Context,
	config anthropicOAuthConfig,
	code string,
	state string,
	verifier string,
	redirectURI string,
	now func() time.Time,
) (Credential, error) {
	return requestAnthropicOAuthToken(
		ctx,
		config,
		"exchange",
		map[string]any{
			"grant_type":    "authorization_code",
			"client_id":     AnthropicOAuthClientID,
			"code":          code,
			"state":         state,
			"redirect_uri":  redirectURI,
			"code_verifier": verifier,
		},
		now,
	)
}

func refreshAnthropicOAuthToken(
	ctx context.Context,
	config anthropicOAuthConfig,
	refreshToken string,
	now func() time.Time,
) (Credential, error) {
	return requestAnthropicOAuthToken(
		ctx,
		config,
		"refresh",
		map[string]any{
			"grant_type":    "refresh_token",
			"client_id":     AnthropicOAuthClientID,
			"refresh_token": refreshToken,
		},
		now,
	)
}

func requestAnthropicOAuthToken(
	ctx context.Context,
	config anthropicOAuthConfig,
	operation string,
	payload map[string]any,
	now func() time.Time,
) (Credential, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Credential{}, fmt.Errorf(
			"marshal Anthropic token %s request: %w",
			operation,
			err,
		)
	}
	requestContext, cancel := context.WithTimeout(
		contextOrBackground(ctx),
		config.requestTimeout,
	)
	defer cancel()
	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodPost,
		AnthropicOAuthTokenURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return Credential{}, err
	}
	request.Header.Set("content-type", "application/json")
	request.Header.Set("accept", "application/json")
	response, err := config.client.Do(request)
	if err != nil {
		if contextError(ctx) != nil {
			return Credential{}, oauthLoginContextError(ctx)
		}
		return Credential{}, fmt.Errorf(
			"Anthropic token %s request failed: %w",
			operation,
			err,
		)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(
		response.Body,
		maxAnthropicOAuthBodyBytes+1,
	))
	if err != nil {
		return Credential{}, fmt.Errorf(
			"read Anthropic token %s response: %w",
			operation,
			err,
		)
	}
	if len(responseBody) > maxAnthropicOAuthBodyBytes {
		return Credential{}, fmt.Errorf(
			"Anthropic token %s response exceeded %d bytes",
			operation,
			maxAnthropicOAuthBodyBytes,
		)
	}
	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		return Credential{}, fmt.Errorf(
			"Anthropic token %s failed (%d): %s",
			operation,
			response.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	var token anthropicOAuthTokenResponse
	if err := json.Unmarshal(responseBody, &token); err != nil {
		return Credential{}, fmt.Errorf(
			"Anthropic token %s returned invalid JSON: %w",
			operation,
			err,
		)
	}
	if token.AccessToken == "" ||
		token.RefreshToken == "" ||
		math.IsNaN(token.ExpiresIn) ||
		math.IsInf(token.ExpiresIn, 0) ||
		token.ExpiresIn <= 0 {
		return Credential{}, fmt.Errorf(
			"Anthropic token %s response missing fields: %s",
			operation,
			strings.TrimSpace(string(responseBody)),
		)
	}
	if now == nil {
		now = time.Now
	}
	expires, err := oauthExpiryMillis(
		now(),
		token.ExpiresIn,
		anthropicOAuthRefreshSkew,
	)
	if err != nil {
		return Credential{}, fmt.Errorf(
			"Anthropic token %s response has invalid expires_in",
			operation,
		)
	}
	return Credential{
		Type:    CredentialTypeOAuth,
		Access:  token.AccessToken,
		Refresh: token.RefreshToken,
		Expires: expires,
	}, nil
}
