package gillmprovider

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	radiusLoginMethodBrowser    = "browser"
	radiusLoginMethodDeviceCode = "device-code"
)

// RadiusOAuthOptions configures OAuth discovery and token exchange for one
// default or custom Radius gateway.
type RadiusOAuthOptions struct {
	Name    string
	Gateway string
	Client  HTTPDoer
}

type radiusOAuthRuntime struct {
	now            func() time.Time
	generatePKCE   func() (PKCE, error)
	randomState    func() (string, error)
	startCallback  func(context.Context, string) (radiusOAuthCallbackServer, error)
	pollDeviceCode func(
		context.Context,
		OAuthDeviceCodePollOptions[Credential],
	) (Credential, error)
}

// NewRadiusOAuth creates the complete Radius browser/device-code login,
// refresh, and request-auth contract. Network access remains deferred until a
// login or refresh operation is invoked.
func NewRadiusOAuth(options RadiusOAuthOptions) (*OAuthAuth, error) {
	return newRadiusOAuth(options, radiusOAuthRuntime{
		now:          time.Now,
		generatePKCE: GeneratePKCE,
		randomState: func() (string, error) {
			return randomOAuthToken(rand.Reader, oauthRandomTokenBytes)
		},
		startCallback: func(
			ctx context.Context,
			expectedState string,
		) (radiusOAuthCallbackServer, error) {
			return startRadiusOAuthCallbackServer(
				ctx,
				expectedState,
				radiusOAuthCallbackAddress,
			)
		},
		pollDeviceCode: PollOAuthDeviceCodeFlow[Credential],
	})
}

func newRadiusOAuth(
	options RadiusOAuthOptions,
	runtime radiusOAuthRuntime,
) (*OAuthAuth, error) {
	name := strings.TrimSpace(options.Name)
	if name == "" {
		name = "Radius"
	}
	gateway := strings.TrimSpace(options.Gateway)
	if gateway == "" {
		gateway = DefaultRadiusGateway
	}
	gateway = NormalizeRadiusGatewayURL(gateway)
	if _, err := radiusOAuthEndpoint(gateway); err != nil {
		return nil, err
	}
	runtime = normalizeRadiusOAuthRuntime(runtime)
	client := httpClientOrDefault(options.Client)

	return &OAuthAuth{
		Name: name,
		Login: func(
			ctx context.Context,
			interaction AuthInteraction,
		) (Credential, error) {
			ctx = contextOrBackground(ctx)
			if interaction == nil {
				return Credential{}, errors.New(
					"Radius OAuth auth interaction is required",
				)
			}
			config, err := loadRadiusOAuthConfig(ctx, client, gateway)
			if err != nil {
				return Credential{}, err
			}
			method, err := interaction.Prompt(ctx, AuthPrompt{
				Type:    AuthPromptSelect,
				Message: "Sign in to " + name + ":",
				Options: []AuthPromptOption{
					{
						ID:    radiusLoginMethodBrowser,
						Label: "Sign in with browser (recommended)",
					},
					{
						ID: radiusLoginMethodDeviceCode,
						Label: "Sign in with device code " +
							"(when signing in from another device)",
					},
				},
			})
			if err != nil {
				return Credential{}, err
			}
			switch method {
			case radiusLoginMethodBrowser:
				return loginRadiusWithBrowser(
					ctx,
					client,
					config,
					interaction,
					runtime,
				)
			case radiusLoginMethodDeviceCode:
				return loginRadiusWithDeviceCode(
					ctx,
					client,
					config,
					interaction,
					runtime,
				)
			default:
				return Credential{}, fmt.Errorf(
					"unknown %s sign-in method %q",
					name,
					method,
				)
			}
		},
		Refresh: func(
			ctx context.Context,
			credential Credential,
		) (Credential, error) {
			ctx = contextOrBackground(ctx)
			if credential.Type != CredentialTypeOAuth ||
				strings.TrimSpace(credential.Refresh) == "" {
				return Credential{}, errors.New(
					"Radius OAuth credential has no refresh token",
				)
			}
			config, err := loadRadiusOAuthConfig(ctx, client, gateway)
			if err != nil {
				return Credential{}, err
			}
			refreshed, err := requestRadiusOAuthToken(
				ctx,
				client,
				config,
				url.Values{
					"grant_type":    {"refresh_token"},
					"client_id":     {config.ClientID},
					"refresh_token": {credential.Refresh},
				},
				runtime.now,
			)
			if err != nil {
				return Credential{}, err
			}
			if refreshed.Refresh == "" {
				refreshed.Refresh = credential.Refresh
			}
			refreshed.Env = cloneProviderEnv(credential.Env)
			refreshed.EnterpriseURL = credential.EnterpriseURL
			refreshed.Metadata = mergeCredentialMetadata(
				credential.Metadata,
				refreshed.Metadata,
			)
			return refreshed, nil
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
					"Radius OAuth credential has no access token",
				)
			}
			return ModelAuth{APIKey: credential.Access}, nil
		},
	}, nil
}

func normalizeRadiusOAuthRuntime(
	runtime radiusOAuthRuntime,
) radiusOAuthRuntime {
	if runtime.now == nil {
		runtime.now = time.Now
	}
	if runtime.generatePKCE == nil {
		runtime.generatePKCE = GeneratePKCE
	}
	if runtime.randomState == nil {
		runtime.randomState = func() (string, error) {
			return randomOAuthToken(rand.Reader, oauthRandomTokenBytes)
		}
	}
	if runtime.startCallback == nil {
		runtime.startCallback = func(
			ctx context.Context,
			expectedState string,
		) (radiusOAuthCallbackServer, error) {
			return startRadiusOAuthCallbackServer(
				ctx,
				expectedState,
				radiusOAuthCallbackAddress,
			)
		}
	}
	if runtime.pollDeviceCode == nil {
		runtime.pollDeviceCode = PollOAuthDeviceCodeFlow[Credential]
	}
	return runtime
}

func loginRadiusWithBrowser(
	ctx context.Context,
	client HTTPDoer,
	config radiusOAuthConfig,
	interaction AuthInteraction,
	runtime radiusOAuthRuntime,
) (Credential, error) {
	authorizationEndpoint, err := parseRadiusOAuthHTTPURL(
		config.AuthorizationEndpoint,
		"authorizationEndpoint",
	)
	if err != nil {
		return Credential{}, err
	}
	pkce, err := runtime.generatePKCE()
	if err != nil {
		return Credential{}, fmt.Errorf("generate Radius OAuth PKCE: %w", err)
	}
	if pkce.Verifier == "" || pkce.Challenge == "" {
		return Credential{}, errors.New(
			"generate Radius OAuth PKCE: empty verifier or challenge",
		)
	}
	state, err := runtime.randomState()
	if err != nil {
		return Credential{}, fmt.Errorf("generate Radius OAuth state: %w", err)
	}
	if state == "" {
		return Credential{}, errors.New("generate Radius OAuth state: empty state")
	}
	callback, err := runtime.startCallback(ctx, state)
	if err != nil {
		return Credential{}, fmt.Errorf(
			"start Radius OAuth callback server: %w",
			err,
		)
	}
	defer callback.Close()

	redirectURI := callback.RedirectURI()
	query := authorizationEndpoint.Query()
	query.Set("response_type", "code")
	query.Set("client_id", config.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("scope", config.Scope)
	query.Set("code_challenge", pkce.Challenge)
	query.Set("code_challenge_method", "S256")
	query.Set("handoff", "url")
	query.Set("state", state)
	authorizationEndpoint.RawQuery = query.Encode()

	interaction.Notify(AuthEvent{
		Type:    AuthEventProgress,
		Message: "Listening for OAuth callback on " + redirectURI,
	})
	interaction.Notify(AuthEvent{
		Type:         AuthEventURL,
		URL:          authorizationEndpoint.String(),
		Instructions: "Continue in your browser.",
	})

	code, err := callback.WaitForCode(ctx)
	if err != nil {
		return Credential{}, err
	}
	if strings.TrimSpace(code) == "" {
		return Credential{}, errors.New("OAuth callback did not complete")
	}
	return requestRadiusOAuthToken(
		ctx,
		client,
		config,
		url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {config.ClientID},
			"redirect_uri":  {redirectURI},
			"code":          {code},
			"code_verifier": {pkce.Verifier},
		},
		runtime.now,
	)
}

func loginRadiusWithDeviceCode(
	ctx context.Context,
	client HTTPDoer,
	config radiusOAuthConfig,
	interaction AuthInteraction,
	runtime radiusOAuthRuntime,
) (Credential, error) {
	if strings.TrimSpace(config.DeviceCodeGrantType) == "" {
		return Credential{}, errors.New(
			"Radius OAuth config is missing deviceCodeGrantType",
		)
	}
	device, err := requestRadiusDeviceAuthorization(ctx, client, config)
	if err != nil {
		return Credential{}, err
	}
	verificationURI := strings.TrimSpace(device.VerificationURI)
	if verificationURI == "" {
		verificationURI = strings.TrimSpace(config.VerificationEndpoint)
	}
	interaction.Notify(AuthEvent{
		Type:             AuthEventDeviceCode,
		UserCode:         device.UserCode,
		VerificationURI:  verificationURI,
		IntervalSeconds:  device.Interval,
		ExpiresInSeconds: device.ExpiresIn,
	})

	return runtime.pollDeviceCode(
		ctx,
		OAuthDeviceCodePollOptions[Credential]{
			IntervalSeconds:  device.Interval,
			ExpiresInSeconds: device.ExpiresIn,
			Poll: func(pollContext context.Context) (
				OAuthDeviceCodePollResult[Credential],
				error,
			) {
				credential, tokenErr := requestRadiusOAuthToken(
					pollContext,
					client,
					config,
					url.Values{
						"grant_type":  {config.DeviceCodeGrantType},
						"client_id":   {config.ClientID},
						"device_code": {device.DeviceCode},
					},
					runtime.now,
				)
				if tokenErr == nil {
					return OAuthDeviceCodePollResult[Credential]{
						Status: OAuthDeviceCodeComplete,
						Value:  credential,
					}, nil
				}
				var responseError *OAuthResponseError
				if !errors.As(tokenErr, &responseError) {
					return OAuthDeviceCodePollResult[Credential]{}, tokenErr
				}
				switch responseError.Code {
				case "authorization_pending":
					return OAuthDeviceCodePollResult[Credential]{
						Status: OAuthDeviceCodePending,
					}, nil
				case "slow_down":
					return OAuthDeviceCodePollResult[Credential]{
						Status: OAuthDeviceCodeSlowDown,
					}, nil
				case "expired_token":
					return OAuthDeviceCodePollResult[Credential]{
						Status:  OAuthDeviceCodeFailed,
						Message: "Device authorization expired.",
					}, nil
				case "access_denied":
					return OAuthDeviceCodePollResult[Credential]{
						Status:  OAuthDeviceCodeFailed,
						Message: "Device authorization was denied.",
					}, nil
				default:
					return OAuthDeviceCodePollResult[Credential]{}, tokenErr
				}
			},
		},
	)
}
