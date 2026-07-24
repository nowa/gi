package gillmprovider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	kimiOAuthClientID                 = "17e5f671-d194-4dfb-9706-5516cb48c098"
	defaultKimiOAuthHost              = "https://auth.kimi.com"
	defaultKimiDeviceTimeoutSeconds   = 15 * 60
	defaultKimiPollIntervalSeconds    = 5
	defaultKimiOAuthRequestTimeout    = 30 * time.Second
	defaultKimiOAuthRefreshMaxRetries = 3
)

// KimiCodingOAuthOptions configures Kimi Code OAuth dependencies. OAuthHost
// takes precedence over KIMI_CODE_OAUTH_HOST and the legacy KIMI_OAUTH_HOST.
type KimiCodingOAuthOptions struct {
	Client         HTTPDoer
	AuthContext    AuthContext
	OAuthHost      string
	RequestTimeout time.Duration
}

type kimiOAuthRuntime struct {
	now            func() time.Time
	pollDeviceCode func(
		context.Context,
		OAuthDeviceCodePollOptions[kimiToken],
	) (kimiToken, error)
	sleep func(context.Context, time.Duration) error
}

type kimiOAuthConfig struct {
	client         HTTPDoer
	authContext    AuthContext
	oauthHost      string
	requestTimeout time.Duration
}

type kimiDeviceAuthorization struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	IntervalSeconds         int
	ExpiresInSeconds        int
}

type kimiToken struct {
	Access  string
	Refresh string
	Expires int64
}

func (t kimiToken) credential() Credential {
	return Credential{
		Type:    CredentialTypeOAuth,
		Access:  t.Access,
		Refresh: t.Refresh,
		Expires: t.Expires,
	}
}

// NewKimiCodingOAuth creates the Kimi Code subscription device-flow
// implementation.
func NewKimiCodingOAuth(options KimiCodingOAuthOptions) *OAuthAuth {
	return newKimiCodingOAuth(options, kimiOAuthRuntime{
		now:            time.Now,
		pollDeviceCode: PollOAuthDeviceCodeFlow[kimiToken],
		sleep:          sleepWithContext,
	})
}

func newKimiCodingOAuth(
	options KimiCodingOAuthOptions,
	runtime kimiOAuthRuntime,
) *OAuthAuth {
	if runtime.now == nil {
		runtime.now = time.Now
	}
	if runtime.pollDeviceCode == nil {
		runtime.pollDeviceCode = PollOAuthDeviceCodeFlow[kimiToken]
	}
	if runtime.sleep == nil {
		runtime.sleep = sleepWithContext
	}
	authContext := options.AuthContext
	if authContext == nil {
		authContext = DefaultProviderAuthContext()
	}
	requestTimeout := options.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = defaultKimiOAuthRequestTimeout
	}
	config := kimiOAuthConfig{
		client:         httpClientOrDefault(options.Client),
		authContext:    authContext,
		oauthHost:      options.OAuthHost,
		requestTimeout: requestTimeout,
	}

	return &OAuthAuth{
		Name:       "Kimi Code (subscription)",
		LoginLabel: "Sign in with Kimi Code",
		Login: func(
			ctx context.Context,
			interaction AuthInteraction,
		) (Credential, error) {
			if interaction == nil {
				return Credential{}, errors.New(
					"Kimi Code OAuth auth interaction is required",
				)
			}
			ctx = contextOrBackground(ctx)
			host, err := resolveKimiOAuthHost(
				ctx,
				config.oauthHost,
				config.authContext,
			)
			if err != nil {
				return Credential{}, err
			}
			credential, err := loginKimiCoding(
				ctx,
				config,
				host,
				interaction,
				runtime,
			)
			if err != nil && contextError(ctx) != nil {
				return Credential{}, oauthLoginContextError(ctx)
			}
			return credential, err
		},
		Refresh: func(
			ctx context.Context,
			credential Credential,
		) (Credential, error) {
			if credential.Type != CredentialTypeOAuth ||
				strings.TrimSpace(credential.Refresh) == "" {
				return Credential{}, errors.New(
					"Kimi Code OAuth credential has no refresh token",
				)
			}
			ctx = contextOrBackground(ctx)
			authContext := overlayProviderAuthContext(
				config.authContext,
				credential.Env,
			)
			host, err := resolveKimiOAuthHost(
				ctx,
				config.oauthHost,
				authContext,
			)
			if err != nil {
				return Credential{}, err
			}
			token, err := refreshKimiToken(
				ctx,
				config,
				host,
				credential.Refresh,
				runtime,
			)
			if err != nil {
				return Credential{}, err
			}
			return mergeRefreshedOAuthCredential(
				credential,
				token.credential(),
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
					"Kimi Code OAuth credential has no access token",
				)
			}
			return ModelAuth{
				Headers: map[string]string{
					"Authorization": "Bearer " + credential.Access,
				},
			}, nil
		},
	}
}

func loginKimiCoding(
	ctx context.Context,
	config kimiOAuthConfig,
	host string,
	interaction AuthInteraction,
	runtime kimiOAuthRuntime,
) (Credential, error) {
	device, err := requestKimiDeviceAuthorization(ctx, config, host)
	if err != nil {
		return Credential{}, err
	}
	interaction.Notify(AuthEvent{
		Type:             AuthEventDeviceCode,
		UserCode:         device.UserCode,
		VerificationURI:  device.VerificationURIComplete,
		IntervalSeconds:  device.IntervalSeconds,
		ExpiresInSeconds: device.ExpiresInSeconds,
	})
	token, err := pollKimiToken(ctx, config, host, device, runtime)
	if err != nil {
		return Credential{}, err
	}
	return token.credential(), nil
}

func pollKimiToken(
	ctx context.Context,
	config kimiOAuthConfig,
	host string,
	device kimiDeviceAuthorization,
	runtime kimiOAuthRuntime,
) (kimiToken, error) {
	return runtime.pollDeviceCode(
		ctx,
		OAuthDeviceCodePollOptions[kimiToken]{
			IntervalSeconds:     device.IntervalSeconds,
			ExpiresInSeconds:    device.ExpiresInSeconds,
			WaitBeforeFirstPoll: true,
			Poll: func(pollContext context.Context) (
				OAuthDeviceCodePollResult[kimiToken],
				error,
			) {
				response, err := postKimiOAuthForm(
					pollContext,
					config,
					host+"/api/oauth/token",
					url.Values{
						"client_id":   {kimiOAuthClientID},
						"device_code": {device.DeviceCode},
						"grant_type": {
							"urn:ietf:params:oauth:grant-type:device_code",
						},
					},
				)
				if err != nil {
					return OAuthDeviceCodePollResult[kimiToken]{}, err
				}
				if response.StatusCode >= http.StatusInternalServerError {
					return OAuthDeviceCodePollResult[kimiToken]{
						Status: OAuthDeviceCodeFailed,
						Message: kimiOAuthStatusError(
							"Kimi Code device token request failed with status",
							response,
						).Error(),
					}, nil
				}
				if response.OK() {
					if _, ok := response.JSON["access_token"].(string); ok {
						token, parseErr := parseKimiTokenResponse(
							response.JSON,
							"poll",
							runtime.now,
						)
						if parseErr != nil {
							return OAuthDeviceCodePollResult[kimiToken]{
								Status:  OAuthDeviceCodeFailed,
								Message: parseErr.Error(),
							}, nil
						}
						return OAuthDeviceCodePollResult[kimiToken]{
							Status: OAuthDeviceCodeComplete,
							Value:  token,
						}, nil
					}
				}

				code, _ := response.JSON["error"].(string)
				description, _ := response.JSON["error_description"].(string)
				switch code {
				case "authorization_pending":
					return OAuthDeviceCodePollResult[kimiToken]{
						Status: OAuthDeviceCodePending,
					}, nil
				case "slow_down":
					return OAuthDeviceCodePollResult[kimiToken]{
						Status: OAuthDeviceCodeSlowDown,
						IntervalSeconds: kimiPositiveSeconds(
							response.JSON["interval"],
							0,
						),
					}, nil
				case "expired_token":
					return OAuthDeviceCodePollResult[kimiToken]{
						Status: OAuthDeviceCodeFailed,
						Message: "Kimi Code device authorization expired. " +
							"Please restart login.",
					}, nil
				case "access_denied":
					return OAuthDeviceCodePollResult[kimiToken]{
						Status:  OAuthDeviceCodeFailed,
						Message: "Kimi Code login was denied.",
					}, nil
				default:
					message := fmt.Sprintf(
						"Kimi Code device token request failed (status %d)",
						response.StatusCode,
					)
					if code != "" {
						message += ": " + code
						if description != "" {
							message += ": " + description
						}
					}
					return OAuthDeviceCodePollResult[kimiToken]{
						Status:  OAuthDeviceCodeFailed,
						Message: message,
					}, nil
				}
			},
		},
	)
}

func refreshKimiToken(
	ctx context.Context,
	config kimiOAuthConfig,
	host string,
	refreshToken string,
	runtime kimiOAuthRuntime,
) (kimiToken, error) {
	attempts := 0
	token, err := RetryProviderRequest(
		ctx,
		ProviderRetryOptions{
			MaxRetries: defaultKimiOAuthRefreshMaxRetries,
			BaseDelay:  time.Second,
			Sleep:      runtime.sleep,
			Jitter:     func() float64 { return 0 },
		},
		func(requestContext context.Context) (kimiToken, error) {
			attempts++
			response, requestErr := postKimiOAuthForm(
				requestContext,
				config,
				host+"/api/oauth/token",
				url.Values{
					"client_id":     {kimiOAuthClientID},
					"grant_type":    {"refresh_token"},
					"refresh_token": {refreshToken},
				},
			)
			if requestErr != nil {
				if contextError(ctx) != nil {
					return kimiToken{}, contextError(ctx)
				}
				return kimiToken{}, newProviderTransportError(requestErr)
			}
			if response.OK() {
				return parseKimiTokenResponse(
					response.JSON,
					"refresh",
					runtime.now,
				)
			}

			code, _ := response.JSON["error"].(string)
			description, _ := response.JSON["error_description"].(string)
			if response.StatusCode == http.StatusUnauthorized ||
				response.StatusCode == http.StatusForbidden ||
				code == "invalid_grant" {
				message := fmt.Sprintf(
					"Kimi Code token refresh unauthorized (status %d)",
					response.StatusCode,
				)
				if description != "" {
					message += ": " + description
				}
				return kimiToken{}, errors.New(message)
			}

			message := fmt.Sprintf(
				"Kimi Code token refresh failed with status %d",
				response.StatusCode,
			)
			retryable := response.StatusCode == http.StatusTooManyRequests ||
				response.StatusCode >= http.StatusInternalServerError
			if !retryable ||
				attempts > defaultKimiOAuthRefreshMaxRetries {
				if body := kimiOAuthJSONText(response); body != "" {
					message += ": " + body
				}
			}
			if retryable {
				return kimiToken{}, newProviderHTTPError(
					response.StatusCode,
					response.Header,
					string(response.Body),
					errors.New(message),
				)
			}
			return kimiToken{}, errors.New(message)
		},
	)
	if err != nil && contextError(ctx) != nil {
		return kimiToken{}, fmt.Errorf(
			"Kimi Code token refresh aborted: %w",
			contextError(ctx),
		)
	}
	return token, err
}
