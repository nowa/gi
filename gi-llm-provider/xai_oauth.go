package gillmprovider

import (
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
	xaiOAuthClientID = "b1a00492-073a-47ea-816f-4c329264a828"
	xaiOAuthScope    = "openid profile email offline_access " +
		"grok-cli:access api:access"
	xaiOAuthDeviceCodeURL          = "https://auth.x.ai/oauth2/device/code"
	xaiOAuthTokenURL               = "https://auth.x.ai/oauth2/token"
	xaiOAuthRefreshSkew            = 5 * time.Minute
	xaiDefaultTokenLifetimeSeconds = 3600
	maxXAIOAuthBodyBytes           = 1 << 20
)

// XAIOAuthOptions configures xAI OAuth HTTP execution.
type XAIOAuthOptions struct {
	Client HTTPDoer
}

type xaiOAuthRuntime struct {
	now            func() time.Time
	pollDeviceCode func(
		context.Context,
		OAuthDeviceCodePollOptions[Credential],
	) (Credential, error)
}

type xaiOAuthHTTPResponse struct {
	OK         bool
	StatusCode int
	Body       map[string]any
}

type xaiOAuthFailure struct {
	action   string
	response *OAuthResponseError
}

func (e *xaiOAuthFailure) Error() string {
	if e == nil {
		return ""
	}
	if e.response == nil {
		return "xAI OAuth " + e.action + " failed"
	}
	message := fmt.Sprintf(
		"xAI OAuth %s failed (HTTP %d)",
		e.action,
		e.response.StatusCode,
	)
	detail := e.response.Code
	if e.response.Description != "" {
		if detail != "" {
			detail += ": "
		}
		detail += e.response.Description
	}
	if detail != "" {
		message += ": " + detail
	}
	return message
}

func (e *xaiOAuthFailure) Unwrap() error {
	if e == nil || e.response == nil {
		return nil
	}
	return e.response
}

type xaiDeviceCode struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	IntervalSeconds         int
	ExpiresInSeconds        int
}

// NewXAIOAuth creates the xAI subscription device-code login contract.
func NewXAIOAuth(options XAIOAuthOptions) *OAuthAuth {
	return newXAIOAuth(options, xaiOAuthRuntime{
		now:            time.Now,
		pollDeviceCode: PollOAuthDeviceCodeFlow[Credential],
	})
}

func newXAIOAuth(
	options XAIOAuthOptions,
	runtime xaiOAuthRuntime,
) *OAuthAuth {
	if runtime.now == nil {
		runtime.now = time.Now
	}
	if runtime.pollDeviceCode == nil {
		runtime.pollDeviceCode = PollOAuthDeviceCodeFlow[Credential]
	}
	client := httpClientOrDefault(options.Client)
	return &OAuthAuth{
		Name:       "xAI (Grok/X subscription)",
		LoginLabel: "Sign in with SuperGrok or X Premium",
		Login: func(
			ctx context.Context,
			interaction AuthInteraction,
		) (Credential, error) {
			if interaction == nil {
				return Credential{}, errors.New(
					"xAI OAuth auth interaction is required",
				)
			}
			return loginXAI(
				contextOrBackground(ctx),
				client,
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
					"xAI OAuth credential has no refresh token",
				)
			}
			refreshed, err := refreshXAIToken(
				contextOrBackground(ctx),
				client,
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
					"xAI OAuth credential has no access token",
				)
			}
			return ModelAuth{APIKey: credential.Access}, nil
		},
	}
}

func loginXAI(
	ctx context.Context,
	client HTTPDoer,
	interaction AuthInteraction,
	runtime xaiOAuthRuntime,
) (Credential, error) {
	device, err := requestXAIDeviceCode(ctx, client)
	if err != nil {
		return Credential{}, err
	}
	verificationURI := device.VerificationURIComplete
	if verificationURI == "" {
		verificationURI = device.VerificationURI
	}
	interaction.Notify(AuthEvent{
		Type:             AuthEventDeviceCode,
		UserCode:         device.UserCode,
		VerificationURI:  verificationURI,
		IntervalSeconds:  device.IntervalSeconds,
		ExpiresInSeconds: device.ExpiresInSeconds,
	})
	return pollXAITokens(ctx, client, device, runtime)
}

func requestXAIDeviceCode(
	ctx context.Context,
	client HTTPDoer,
) (xaiDeviceCode, error) {
	response, err := postXAIOAuthForm(
		ctx,
		client,
		xaiOAuthDeviceCodeURL,
		url.Values{
			"client_id": {xaiOAuthClientID},
			"scope":     {xaiOAuthScope},
			"referrer":  {"pi"},
		},
	)
	if err != nil {
		return xaiDeviceCode{}, err
	}
	if !response.OK {
		return xaiDeviceCode{}, xaiOAuthRequestFailure(
			"device authorization",
			response,
		)
	}
	return parseXAIDeviceCode(response.Body)
}

func pollXAITokens(
	ctx context.Context,
	client HTTPDoer,
	device xaiDeviceCode,
	runtime xaiOAuthRuntime,
) (Credential, error) {
	return runtime.pollDeviceCode(
		ctx,
		OAuthDeviceCodePollOptions[Credential]{
			IntervalSeconds:     device.IntervalSeconds,
			ExpiresInSeconds:    device.ExpiresInSeconds,
			WaitBeforeFirstPoll: true,
			Poll: func(pollContext context.Context) (
				OAuthDeviceCodePollResult[Credential],
				error,
			) {
				response, err := postXAIOAuthForm(
					pollContext,
					client,
					xaiOAuthTokenURL,
					url.Values{
						"grant_type": {
							"urn:ietf:params:oauth:grant-type:device_code",
						},
						"client_id":   {xaiOAuthClientID},
						"device_code": {device.DeviceCode},
					},
				)
				if err != nil {
					return OAuthDeviceCodePollResult[Credential]{}, err
				}
				if response.OK {
					credential, parseErr := xaiCredentialsFromTokenResponse(
						response.Body,
						"",
						runtime.now,
					)
					if parseErr != nil {
						return OAuthDeviceCodePollResult[Credential]{}, parseErr
					}
					return OAuthDeviceCodePollResult[Credential]{
						Status: OAuthDeviceCodeComplete,
						Value:  credential,
					}, nil
				}

				code, _ := response.Body["error"].(string)
				switch code {
				case "authorization_pending":
					return OAuthDeviceCodePollResult[Credential]{
						Status: OAuthDeviceCodePending,
					}, nil
				case "slow_down":
					interval, _ := positiveXAIOAuthSeconds(
						response.Body,
						"interval",
						false,
					)
					return OAuthDeviceCodePollResult[Credential]{
						Status:          OAuthDeviceCodeSlowDown,
						IntervalSeconds: interval,
					}, nil
				case "access_denied", "authorization_denied":
					return OAuthDeviceCodePollResult[Credential]{
						Status:  OAuthDeviceCodeFailed,
						Message: "xAI device authorization was denied",
					}, nil
				case "expired_token":
					return OAuthDeviceCodePollResult[Credential]{
						Status:  OAuthDeviceCodeFailed,
						Message: "xAI device code expired",
					}, nil
				default:
					return OAuthDeviceCodePollResult[Credential]{
						Status: OAuthDeviceCodeFailed,
						Message: xaiOAuthRequestFailure(
							"device token polling",
							response,
						).Error(),
					}, nil
				}
			},
		},
	)
}

func refreshXAIToken(
	ctx context.Context,
	client HTTPDoer,
	refreshToken string,
	now func() time.Time,
) (Credential, error) {
	response, err := postXAIOAuthForm(
		ctx,
		client,
		xaiOAuthTokenURL,
		url.Values{
			"grant_type":    {"refresh_token"},
			"client_id":     {xaiOAuthClientID},
			"refresh_token": {refreshToken},
		},
	)
	if err != nil {
		return Credential{}, err
	}
	if !response.OK {
		return Credential{}, xaiOAuthRequestFailure(
			"token refresh",
			response,
		)
	}
	return xaiCredentialsFromTokenResponse(
		response.Body,
		refreshToken,
		now,
	)
}

func postXAIOAuthForm(
	ctx context.Context,
	client HTTPDoer,
	endpoint string,
	values url.Values,
) (xaiOAuthHTTPResponse, error) {
	ctx = contextOrBackground(ctx)
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		strings.NewReader(values.Encode()),
	)
	if err != nil {
		return xaiOAuthHTTPResponse{}, err
	}
	request.Header.Set("accept", "application/json")
	request.Header.Set(
		"content-type",
		"application/x-www-form-urlencoded",
	)
	response, err := httpClientOrDefault(client).Do(request)
	if err != nil {
		if contextError(ctx) != nil {
			return xaiOAuthHTTPResponse{}, oauthLoginContextError(ctx)
		}
		return xaiOAuthHTTPResponse{}, err
	}
	if response.Body == nil {
		return xaiOAuthHTTPResponse{}, fmt.Errorf(
			"xAI OAuth returned invalid JSON (HTTP %d)",
			response.StatusCode,
		)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(
		response.Body,
		maxXAIOAuthBodyBytes+1,
	))
	if err != nil {
		if contextError(ctx) != nil {
			return xaiOAuthHTTPResponse{}, oauthLoginContextError(ctx)
		}
		return xaiOAuthHTTPResponse{}, err
	}
	if len(body) > maxXAIOAuthBodyBytes {
		return xaiOAuthHTTPResponse{}, fmt.Errorf(
			"xAI OAuth returned invalid JSON (HTTP %d): response exceeds %d bytes",
			response.StatusCode,
			maxXAIOAuthBodyBytes,
		)
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		if contextError(ctx) != nil {
			return xaiOAuthHTTPResponse{}, oauthLoginContextError(ctx)
		}
		return xaiOAuthHTTPResponse{}, fmt.Errorf(
			"xAI OAuth returned invalid JSON (HTTP %d)",
			response.StatusCode,
		)
	}
	return xaiOAuthHTTPResponse{
		OK: response.StatusCode >= http.StatusOK &&
			response.StatusCode < http.StatusMultipleChoices,
		StatusCode: response.StatusCode,
		Body:       object,
	}, nil
}

func xaiOAuthRequestFailure(
	action string,
	response xaiOAuthHTTPResponse,
) error {
	code, _ := response.Body["error"].(string)
	description, _ := response.Body["error_description"].(string)
	return &xaiOAuthFailure{
		action: action,
		response: &OAuthResponseError{
			StatusCode:  response.StatusCode,
			Code:        code,
			Description: description,
			Operation:   "xAI OAuth " + action,
		},
	}
}

func parseXAIDeviceCode(body map[string]any) (xaiDeviceCode, error) {
	deviceCode, err := requiredXAIOAuthString(body, "device_code")
	if err != nil {
		return xaiDeviceCode{}, err
	}
	userCode, err := requiredXAIOAuthString(body, "user_code")
	if err != nil {
		return xaiDeviceCode{}, err
	}
	verificationURIValue, err := requiredXAIOAuthString(
		body,
		"verification_uri",
	)
	if err != nil {
		return xaiDeviceCode{}, err
	}
	verificationURI, err := validateXAIVerificationURI(
		verificationURIValue,
	)
	if err != nil {
		return xaiDeviceCode{}, err
	}
	expires, err := positiveXAIOAuthSeconds(body, "expires_in", true)
	if err != nil {
		return xaiDeviceCode{}, err
	}
	interval, _ := positiveXAIOAuthSeconds(body, "interval", false)

	verificationURIComplete := ""
	if raw, ok := body["verification_uri_complete"].(string); ok && raw != "" {
		verificationURIComplete, err = validateXAIVerificationURI(raw)
		if err != nil {
			return xaiDeviceCode{}, err
		}
	}
	return xaiDeviceCode{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         verificationURI,
		VerificationURIComplete: verificationURIComplete,
		IntervalSeconds:         interval,
		ExpiresInSeconds:        expires,
	}, nil
}

func xaiCredentialsFromTokenResponse(
	body map[string]any,
	previousRefreshToken string,
	now func() time.Time,
) (Credential, error) {
	access, err := requiredXAIOAuthString(body, "access_token")
	if err != nil {
		return Credential{}, err
	}
	refresh := previousRefreshToken
	if _, exists := body["refresh_token"]; exists || refresh == "" {
		refresh, err = requiredXAIOAuthString(body, "refresh_token")
		if err != nil {
			return Credential{}, err
		}
	}
	expiresIn := float64(xaiDefaultTokenLifetimeSeconds)
	if _, exists := body["expires_in"]; exists {
		expiresIn, err = positiveXAIOAuthNumber(body, "expires_in", true)
		if err != nil {
			return Credential{}, err
		}
	}
	if now == nil {
		now = time.Now
	}
	expires, err := xaiOAuthExpiryMillis(now(), expiresIn)
	if err != nil {
		return Credential{}, err
	}
	return Credential{
		Type:    CredentialTypeOAuth,
		Access:  access,
		Refresh: refresh,
		Expires: expires,
	}, nil
}

func requiredXAIOAuthString(
	body map[string]any,
	field string,
) (string, error) {
	value, ok := body[field].(string)
	if !ok || value == "" {
		return "", fmt.Errorf(
			"Invalid xAI OAuth response field: %s",
			field,
		)
	}
	return value, nil
}

func positiveXAIOAuthNumber(
	body map[string]any,
	field string,
	required bool,
) (float64, error) {
	value, ok := body[field].(float64)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		if !required {
			return 0, nil
		}
		return 0, fmt.Errorf(
			"Invalid xAI OAuth response field: %s",
			field,
		)
	}
	return value, nil
}

func positiveXAIOAuthSeconds(
	body map[string]any,
	field string,
	required bool,
) (int, error) {
	value, err := positiveXAIOAuthNumber(body, field, required)
	if err != nil || value == 0 {
		return 0, err
	}
	seconds := math.Ceil(value)
	maxInt := int(^uint(0) >> 1)
	if seconds >= float64(maxInt) {
		if !required {
			return 0, nil
		}
		return 0, fmt.Errorf(
			"Invalid xAI OAuth response field: %s",
			field,
		)
	}
	return int(seconds), nil
}

func validateXAIVerificationURI(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New(
			"Untrusted verification URI in xAI OAuth response",
		)
	}
	return parsed.String(), nil
}

func xaiOAuthExpiryMillis(
	now time.Time,
	expiresInSeconds float64,
) (int64, error) {
	expires, err := oauthExpiryMillis(
		now,
		expiresInSeconds,
		xaiOAuthRefreshSkew,
	)
	if err != nil {
		return 0, errors.New(
			"Invalid xAI OAuth response field: expires_in",
		)
	}
	return expires, nil
}
