package gillmprovider

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	openAICodexOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	openAICodexOAuthBaseURL  = "https://auth.openai.com"
	openAICodexOAuthAuthURL  = openAICodexOAuthBaseURL +
		"/oauth/authorize"
	openAICodexOAuthTokenURL = openAICodexOAuthBaseURL +
		"/oauth/token"
	openAICodexOAuthRedirectURI  = "http://localhost:1455/auth/callback"
	openAICodexOAuthCallbackPort = "1455"
	openAICodexOAuthCallbackPath = "/auth/callback"
	openAICodexDeviceUserCodeURL = openAICodexOAuthBaseURL +
		"/api/accounts/deviceauth/usercode"
	openAICodexDeviceTokenURL = openAICodexOAuthBaseURL +
		"/api/accounts/deviceauth/token"
	openAICodexDeviceVerificationURI = openAICodexOAuthBaseURL +
		"/codex/device"
	openAICodexDeviceRedirectURI = openAICodexOAuthBaseURL +
		"/deviceauth/callback"
	openAICodexOAuthScope             = "openid profile email offline_access"
	openAICodexBrowserLoginMethod     = "browser"
	openAICodexDeviceCodeLoginMethod  = "device_code"
	openAICodexDeviceCodeTimeout      = 15 * time.Minute
	defaultOpenAICodexOAuthTimeout    = 30 * time.Second
	maxOpenAICodexOAuthBodyBytes      = 1 << 20
	defaultOpenAICodexOAuthOriginator = "gi"
)

// OpenAICodexOAuthOptions configures the provider-owned ChatGPT
// authorization-code and device-code flows.
type OpenAICodexOAuthOptions struct {
	Client         HTTPDoer
	AuthContext    AuthContext
	CallbackHost   string
	Originator     string
	RequestTimeout time.Duration
}

type openAICodexOAuthConfig struct {
	client         HTTPDoer
	authContext    AuthContext
	callbackHost   string
	originator     string
	requestTimeout time.Duration
}

type openAICodexOAuthRuntime struct {
	now           func() time.Time
	generatePKCE  func() (PKCE, error)
	randomState   func() (string, error)
	startCallback func(
		context.Context,
		oauthLoopbackCallbackOptions,
	) (oauthAuthorizationCodeServer, error)
	pollDeviceCode func(
		context.Context,
		OAuthDeviceCodePollOptions[openAICodexDeviceToken],
	) (openAICodexDeviceToken, error)
}

type openAICodexAuthorizationFlow struct {
	PKCE         PKCE
	State        string
	AuthorizeURL string
}

type openAICodexDeviceAuth struct {
	DeviceAuthID    string
	UserCode        string
	IntervalSeconds int
}

type openAICodexDeviceToken struct {
	AuthorizationCode string
	CodeVerifier      string
}

type openAICodexToken struct {
	Access  string
	Refresh string
	Expires int64
}

type openAICodexOAuthHTTPResponse struct {
	StatusCode int
	Status     string
	Body       []byte
}

// NewOpenAICodexOAuth creates the reusable ChatGPT Plus/Pro browser and
// headless device-code login contract.
func NewOpenAICodexOAuth(options OpenAICodexOAuthOptions) *OAuthAuth {
	return newOpenAICodexOAuth(options, openAICodexOAuthRuntime{
		now:          time.Now,
		generatePKCE: GeneratePKCE,
		randomState: func() (string, error) {
			return randomOAuthToken(rand.Reader, 16)
		},
		startCallback:  startOAuthAuthorizationCodeServer,
		pollDeviceCode: PollOAuthDeviceCodeFlow[openAICodexDeviceToken],
	})
}

func newOpenAICodexOAuth(
	options OpenAICodexOAuthOptions,
	runtime openAICodexOAuthRuntime,
) *OAuthAuth {
	if runtime.now == nil {
		runtime.now = time.Now
	}
	if runtime.generatePKCE == nil {
		runtime.generatePKCE = GeneratePKCE
	}
	if runtime.randomState == nil {
		runtime.randomState = func() (string, error) {
			return randomOAuthToken(rand.Reader, 16)
		}
	}
	if runtime.startCallback == nil {
		runtime.startCallback = startOAuthAuthorizationCodeServer
	}
	if runtime.pollDeviceCode == nil {
		runtime.pollDeviceCode =
			PollOAuthDeviceCodeFlow[openAICodexDeviceToken]
	}
	authContext := options.AuthContext
	if authContext == nil {
		authContext = DefaultProviderAuthContext()
	}
	originator := strings.TrimSpace(options.Originator)
	if originator == "" {
		originator = defaultOpenAICodexOAuthOriginator
	}
	requestTimeout := options.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = defaultOpenAICodexOAuthTimeout
	}
	config := openAICodexOAuthConfig{
		client:         httpClientOrDefault(options.Client),
		authContext:    authContext,
		callbackHost:   options.CallbackHost,
		originator:     originator,
		requestTimeout: requestTimeout,
	}

	return &OAuthAuth{
		Name: "OpenAI (ChatGPT Plus/Pro)",
		Login: func(
			ctx context.Context,
			interaction AuthInteraction,
		) (Credential, error) {
			if interaction == nil {
				return Credential{}, errors.New(
					"OpenAI Codex OAuth auth interaction is required",
				)
			}
			ctx = contextOrBackground(ctx)
			method, err := interaction.Prompt(ctx, AuthPrompt{
				Type:    AuthPromptSelect,
				Message: "Select OpenAI Codex login method:",
				Options: []AuthPromptOption{
					{
						ID:    openAICodexBrowserLoginMethod,
						Label: "Browser login (default)",
					},
					{
						ID:    openAICodexDeviceCodeLoginMethod,
						Label: "Device code login (headless)",
					},
				},
			})
			if err != nil {
				return Credential{}, err
			}
			switch method {
			case openAICodexBrowserLoginMethod:
				return loginOpenAICodexBrowser(
					ctx,
					config,
					interaction,
					runtime,
				)
			case openAICodexDeviceCodeLoginMethod:
				return loginOpenAICodexDeviceCode(
					ctx,
					config,
					interaction,
					runtime,
				)
			default:
				return Credential{}, fmt.Errorf(
					"unknown OpenAI Codex login method: %s",
					method,
				)
			}
		},
		Refresh: func(
			ctx context.Context,
			credential Credential,
		) (Credential, error) {
			if credential.Type != CredentialTypeOAuth ||
				strings.TrimSpace(credential.Refresh) == "" {
				return Credential{}, errors.New(
					"OpenAI Codex OAuth credential has no refresh token",
				)
			}
			refreshed, err := refreshOpenAICodexOAuthToken(
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
					"OpenAI Codex OAuth credential has no access token",
				)
			}
			return ModelAuth{APIKey: credential.Access}, nil
		},
	}
}

func loginOpenAICodexBrowser(
	ctx context.Context,
	config openAICodexOAuthConfig,
	interaction AuthInteraction,
	runtime openAICodexOAuthRuntime,
) (Credential, error) {
	if err := contextError(ctx); err != nil {
		return Credential{}, oauthLoginContextError(ctx)
	}
	flow, err := createOpenAICodexAuthorizationFlow(
		config.originator,
		runtime.generatePKCE,
		runtime.randomState,
	)
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
	callback, callbackErr := runtime.startCallback(
		ctx,
		oauthLoopbackCallbackOptions{
			Host:               callbackHost,
			Port:               openAICodexOAuthCallbackPort,
			Path:               openAICodexOAuthCallbackPath,
			ExpectedState:      flow.State,
			ProviderName:       "OpenAI",
			SuccessMessage:     "OpenAI authentication completed. You can close this window.",
			ValidateStateFirst: true,
		},
	)
	if callback != nil {
		defer callback.Close()
	}
	if callbackErr != nil && contextError(ctx) != nil {
		return Credential{}, oauthLoginContextError(ctx)
	}

	interaction.Notify(AuthEvent{
		Type:         AuthEventURL,
		URL:          flow.AuthorizeURL,
		Instructions: "A browser window should open. Complete login to finish.",
	})
	authorization, err := waitForOAuthAuthorizationCode(
		ctx,
		interaction,
		callback,
		AuthPrompt{
			Type: AuthPromptManualCode,
			Message: "Complete login in your browser, or paste the " +
				"authorization code / redirect URL here:",
			Placeholder: openAICodexOAuthRedirectURI,
		},
		flow.State,
	)
	if err != nil {
		if contextError(ctx) != nil {
			return Credential{}, oauthLoginContextError(ctx)
		}
		return Credential{}, err
	}
	return exchangeOpenAICodexAuthorizationCodeForCredentials(
		ctx,
		config,
		authorization.Code,
		flow.PKCE.Verifier,
		openAICodexOAuthRedirectURI,
		runtime.now,
	)
}

func loginOpenAICodexDeviceCode(
	ctx context.Context,
	config openAICodexOAuthConfig,
	interaction AuthInteraction,
	runtime openAICodexOAuthRuntime,
) (Credential, error) {
	device, err := startOpenAICodexDeviceAuth(ctx, config)
	if err != nil {
		return Credential{}, err
	}
	interaction.Notify(AuthEvent{
		Type:             AuthEventDeviceCode,
		UserCode:         device.UserCode,
		VerificationURI:  openAICodexDeviceVerificationURI,
		IntervalSeconds:  device.IntervalSeconds,
		ExpiresInSeconds: int(openAICodexDeviceCodeTimeout / time.Second),
	})
	token, err := pollOpenAICodexDeviceAuth(
		ctx,
		config,
		device,
		runtime.pollDeviceCode,
	)
	if err != nil {
		return Credential{}, err
	}
	return exchangeOpenAICodexAuthorizationCodeForCredentials(
		ctx,
		config,
		token.AuthorizationCode,
		token.CodeVerifier,
		openAICodexDeviceRedirectURI,
		runtime.now,
	)
}

func createOpenAICodexAuthorizationFlow(
	originator string,
	generatePKCE func() (PKCE, error),
	randomState func() (string, error),
) (openAICodexAuthorizationFlow, error) {
	pkce, err := generatePKCE()
	if err != nil {
		return openAICodexAuthorizationFlow{}, fmt.Errorf(
			"generate OpenAI Codex OAuth PKCE: %w",
			err,
		)
	}
	if pkce.Verifier == "" || pkce.Challenge == "" {
		return openAICodexAuthorizationFlow{}, errors.New(
			"generate OpenAI Codex OAuth PKCE: empty verifier or challenge",
		)
	}
	state, err := randomState()
	if err != nil {
		return openAICodexAuthorizationFlow{}, fmt.Errorf(
			"generate OpenAI Codex OAuth state: %w",
			err,
		)
	}
	if state == "" {
		return openAICodexAuthorizationFlow{}, errors.New(
			"generate OpenAI Codex OAuth state: empty state",
		)
	}
	authorizeURL, err := url.Parse(openAICodexOAuthAuthURL)
	if err != nil {
		return openAICodexAuthorizationFlow{}, err
	}
	query := authorizeURL.Query()
	query.Set("response_type", "code")
	query.Set("client_id", openAICodexOAuthClientID)
	query.Set("redirect_uri", openAICodexOAuthRedirectURI)
	query.Set("scope", openAICodexOAuthScope)
	query.Set("code_challenge", pkce.Challenge)
	query.Set("code_challenge_method", "S256")
	query.Set("state", state)
	query.Set("id_token_add_organizations", "true")
	query.Set("codex_cli_simplified_flow", "true")
	query.Set("originator", originator)
	authorizeURL.RawQuery = query.Encode()
	return openAICodexAuthorizationFlow{
		PKCE:         pkce,
		State:        state,
		AuthorizeURL: authorizeURL.String(),
	}, nil
}

func startOpenAICodexDeviceAuth(
	ctx context.Context,
	config openAICodexOAuthConfig,
) (openAICodexDeviceAuth, error) {
	body, err := json.Marshal(map[string]string{
		"client_id": openAICodexOAuthClientID,
	})
	if err != nil {
		return openAICodexDeviceAuth{}, err
	}
	response, err := doOpenAICodexOAuthRequest(
		ctx,
		config,
		openAICodexDeviceUserCodeURL,
		"application/json",
		body,
	)
	if err != nil {
		return openAICodexDeviceAuth{}, err
	}
	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		if response.StatusCode == http.StatusNotFound {
			return openAICodexDeviceAuth{}, errors.New(
				"OpenAI Codex device code login is not enabled for " +
					"this server. Use browser login or verify the server URL.",
			)
		}
		return openAICodexDeviceAuth{}, fmt.Errorf(
			"OpenAI Codex device code request failed with status %d%s",
			response.StatusCode,
			openAICodexResponseBodySuffix(response.Body),
		)
	}

	var payload map[string]any
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return openAICodexDeviceAuth{}, fmt.Errorf(
			"Invalid OpenAI Codex device code response: %s",
			strings.TrimSpace(string(response.Body)),
		)
	}
	deviceAuthID, deviceOK := payload["device_auth_id"].(string)
	userCode, userOK := payload["user_code"].(string)
	interval, intervalOK := openAICodexIntervalSeconds(
		payload["interval"],
	)
	if !deviceOK ||
		strings.TrimSpace(deviceAuthID) == "" ||
		!userOK ||
		strings.TrimSpace(userCode) == "" ||
		!intervalOK {
		return openAICodexDeviceAuth{}, fmt.Errorf(
			"Invalid OpenAI Codex device code response: %s",
			strings.TrimSpace(string(response.Body)),
		)
	}
	return openAICodexDeviceAuth{
		DeviceAuthID:    deviceAuthID,
		UserCode:        userCode,
		IntervalSeconds: interval,
	}, nil
}

func pollOpenAICodexDeviceAuth(
	ctx context.Context,
	config openAICodexOAuthConfig,
	device openAICodexDeviceAuth,
	poll func(
		context.Context,
		OAuthDeviceCodePollOptions[openAICodexDeviceToken],
	) (openAICodexDeviceToken, error),
) (openAICodexDeviceToken, error) {
	if poll == nil {
		poll = PollOAuthDeviceCodeFlow[openAICodexDeviceToken]
	}
	return poll(ctx, OAuthDeviceCodePollOptions[openAICodexDeviceToken]{
		IntervalSeconds:  device.IntervalSeconds,
		ExpiresInSeconds: int(openAICodexDeviceCodeTimeout / time.Second),
		Poll: func(pollContext context.Context) (
			OAuthDeviceCodePollResult[openAICodexDeviceToken],
			error,
		) {
			body, err := json.Marshal(map[string]string{
				"device_auth_id": device.DeviceAuthID,
				"user_code":      device.UserCode,
			})
			if err != nil {
				return OAuthDeviceCodePollResult[openAICodexDeviceToken]{},
					err
			}
			response, err := doOpenAICodexOAuthRequest(
				pollContext,
				config,
				openAICodexDeviceTokenURL,
				"application/json",
				body,
			)
			if err != nil {
				return OAuthDeviceCodePollResult[openAICodexDeviceToken]{},
					err
			}
			if response.StatusCode >= http.StatusOK &&
				response.StatusCode < http.StatusMultipleChoices {
				var payload struct {
					AuthorizationCode string `json:"authorization_code"`
					CodeVerifier      string `json:"code_verifier"`
				}
				if err := json.Unmarshal(response.Body, &payload); err != nil ||
					payload.AuthorizationCode == "" ||
					payload.CodeVerifier == "" {
					return OAuthDeviceCodePollResult[openAICodexDeviceToken]{
						Status: OAuthDeviceCodeFailed,
						Message: "Invalid OpenAI Codex device auth token " +
							"response: " +
							strings.TrimSpace(string(response.Body)),
					}, nil
				}
				return OAuthDeviceCodePollResult[openAICodexDeviceToken]{
					Status: OAuthDeviceCodeComplete,
					Value: openAICodexDeviceToken{
						AuthorizationCode: payload.AuthorizationCode,
						CodeVerifier:      payload.CodeVerifier,
					},
				}, nil
			}
			if response.StatusCode == http.StatusForbidden ||
				response.StatusCode == http.StatusNotFound {
				return OAuthDeviceCodePollResult[openAICodexDeviceToken]{
					Status: OAuthDeviceCodePending,
				}, nil
			}
			switch openAICodexOAuthErrorCode(response.Body) {
			case "deviceauth_authorization_pending":
				return OAuthDeviceCodePollResult[openAICodexDeviceToken]{
					Status: OAuthDeviceCodePending,
				}, nil
			case "slow_down":
				return OAuthDeviceCodePollResult[openAICodexDeviceToken]{
					Status: OAuthDeviceCodeSlowDown,
				}, nil
			default:
				return OAuthDeviceCodePollResult[openAICodexDeviceToken]{
					Status: OAuthDeviceCodeFailed,
					Message: fmt.Sprintf(
						"OpenAI Codex device auth failed with status %d%s",
						response.StatusCode,
						openAICodexResponseBodySuffix(response.Body),
					),
				}, nil
			}
		},
	})
}

func exchangeOpenAICodexAuthorizationCodeForCredentials(
	ctx context.Context,
	config openAICodexOAuthConfig,
	code string,
	verifier string,
	redirectURI string,
	now func() time.Time,
) (Credential, error) {
	token, err := requestOpenAICodexToken(
		ctx,
		config,
		"exchange",
		url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {openAICodexOAuthClientID},
			"code":          {code},
			"code_verifier": {verifier},
			"redirect_uri":  {redirectURI},
		},
		now,
	)
	if err != nil {
		return Credential{}, err
	}
	return openAICodexCredentialsFromToken(token)
}

func refreshOpenAICodexOAuthToken(
	ctx context.Context,
	config openAICodexOAuthConfig,
	refreshToken string,
	now func() time.Time,
) (Credential, error) {
	token, err := requestOpenAICodexToken(
		ctx,
		config,
		"refresh",
		url.Values{
			"grant_type":    {"refresh_token"},
			"refresh_token": {refreshToken},
			"client_id":     {openAICodexOAuthClientID},
		},
		now,
	)
	if err != nil {
		return Credential{}, err
	}
	return openAICodexCredentialsFromToken(token)
}

func requestOpenAICodexToken(
	ctx context.Context,
	config openAICodexOAuthConfig,
	operation string,
	values url.Values,
	now func() time.Time,
) (openAICodexToken, error) {
	response, err := doOpenAICodexOAuthRequest(
		ctx,
		config,
		openAICodexOAuthTokenURL,
		"application/x-www-form-urlencoded",
		[]byte(values.Encode()),
	)
	if err != nil {
		return openAICodexToken{}, fmt.Errorf(
			"OpenAI Codex token %s error: %w",
			operation,
			err,
		)
	}
	return readOpenAICodexTokenResponse(
		response,
		operation,
		now,
	)
}

func readOpenAICodexTokenResponse(
	response openAICodexOAuthHTTPResponse,
	operation string,
	now func() time.Time,
) (openAICodexToken, error) {
	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		return openAICodexToken{}, openAICodexTokenResponseError(
			operation,
			response.StatusCode,
			response.Status,
			string(response.Body),
		)
	}
	var payload struct {
		AccessToken  string  `json:"access_token"`
		RefreshToken string  `json:"refresh_token"`
		ExpiresIn    float64 `json:"expires_in"`
	}
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return openAICodexToken{}, fmt.Errorf(
			"OpenAI Codex token %s response missing fields: %s",
			operation,
			strings.TrimSpace(string(response.Body)),
		)
	}
	if payload.AccessToken == "" ||
		payload.RefreshToken == "" ||
		math.IsNaN(payload.ExpiresIn) ||
		math.IsInf(payload.ExpiresIn, 0) ||
		payload.ExpiresIn <= 0 {
		return openAICodexToken{}, fmt.Errorf(
			"OpenAI Codex token %s response missing fields: %s",
			operation,
			strings.TrimSpace(string(response.Body)),
		)
	}
	if now == nil {
		now = time.Now
	}
	expires, err := oauthExpiryMillis(now(), payload.ExpiresIn, 0)
	if err != nil {
		return openAICodexToken{}, fmt.Errorf(
			"OpenAI Codex token %s response missing fields: %s",
			operation,
			strings.TrimSpace(string(response.Body)),
		)
	}
	return openAICodexToken{
		Access:  payload.AccessToken,
		Refresh: payload.RefreshToken,
		Expires: expires,
	}, nil
}

func openAICodexCredentialsFromToken(
	token openAICodexToken,
) (Credential, error) {
	accountID, err := ExtractOpenAICodexAccountID(token.Access)
	if err != nil {
		return Credential{}, errors.New(
			"failed to extract accountId from token",
		)
	}
	return Credential{
		Type:    CredentialTypeOAuth,
		Access:  token.Access,
		Refresh: token.Refresh,
		Expires: token.Expires,
		Metadata: map[string]any{
			"accountId": accountID,
		},
	}, nil
}

func doOpenAICodexOAuthRequest(
	ctx context.Context,
	config openAICodexOAuthConfig,
	endpoint string,
	contentType string,
	body []byte,
) (openAICodexOAuthHTTPResponse, error) {
	requestContext, cancel := context.WithTimeout(
		contextOrBackground(ctx),
		config.requestTimeout,
	)
	defer cancel()
	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodPost,
		endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return openAICodexOAuthHTTPResponse{}, err
	}
	request.Header.Set("content-type", contentType)
	response, err := config.client.Do(request)
	if err != nil {
		if contextError(ctx) != nil {
			return openAICodexOAuthHTTPResponse{},
				oauthLoginContextError(ctx)
		}
		return openAICodexOAuthHTTPResponse{}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(
		response.Body,
		maxOpenAICodexOAuthBodyBytes+1,
	))
	if err != nil {
		return openAICodexOAuthHTTPResponse{}, err
	}
	if len(responseBody) > maxOpenAICodexOAuthBodyBytes {
		return openAICodexOAuthHTTPResponse{}, fmt.Errorf(
			"OpenAI Codex OAuth response exceeded %d bytes",
			maxOpenAICodexOAuthBodyBytes,
		)
	}
	return openAICodexOAuthHTTPResponse{
		StatusCode: response.StatusCode,
		Status:     response.Status,
		Body:       responseBody,
	}, nil
}

func openAICodexTokenResponseError(
	operation string,
	status int,
	statusText string,
	body string,
) error {
	if operation == "refresh" {
		return OpenAICodexRefreshError(
			status,
			statusText,
			body,
		)
	}
	message := statusText
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err == nil &&
		parsed.Error.Message != "" {
		message = parsed.Error.Message
	} else if strings.TrimSpace(body) != "" {
		message = strings.TrimSpace(body)
	}
	return fmt.Errorf(
		"OpenAI Codex token %s failed (%d): %s",
		operation,
		status,
		message,
	)
}

func openAICodexOAuthErrorCode(body []byte) string {
	var payload struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &payload) != nil || len(payload.Error) == 0 {
		return ""
	}
	var direct string
	if json.Unmarshal(payload.Error, &direct) == nil {
		return direct
	}
	var nested struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(payload.Error, &nested) == nil {
		return nested.Code
	}
	return ""
}

func openAICodexIntervalSeconds(value any) (int, bool) {
	var seconds float64
	switch candidate := value.(type) {
	case float64:
		seconds = candidate
	case string:
		parsed, err := strconv.ParseFloat(
			strings.TrimSpace(candidate),
			64,
		)
		if err != nil {
			return 0, false
		}
		seconds = parsed
	default:
		return 0, false
	}
	if math.IsNaN(seconds) ||
		math.IsInf(seconds, 0) ||
		seconds < 0 {
		return 0, false
	}
	seconds = math.Ceil(seconds)
	maxInt := int(^uint(0) >> 1)
	if seconds > float64(maxInt) {
		return 0, false
	}
	return int(seconds), true
}

func openAICodexResponseBodySuffix(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	return ": " + text
}
