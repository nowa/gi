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
	"sync"
	"time"
)

const (
	githubCopilotOAuthClientID       = "Iv1.b507a08c87ecfe98"
	githubCopilotDefaultDomain       = "github.com"
	githubCopilotDeviceGrant         = "urn:ietf:params:oauth:grant-type:device_code"
	githubCopilotUserAgent           = "GitHubCopilotChat/0.35.0"
	githubCopilotEditorVersion       = "vscode/1.107.0"
	githubCopilotPluginVersion       = "copilot-chat/0.35.0"
	githubCopilotIntegrationID       = "vscode-chat"
	githubCopilotAPIVersion          = "2026-06-01"
	defaultGitHubCopilotOAuthTimeout = 30 * time.Second
	defaultCopilotCatalogTimeout     = 5 * time.Second
	githubCopilotRefreshSkew         = 5 * time.Minute
	maxGitHubCopilotOAuthBodyBytes   = 1 << 20
	maxGitHubCopilotEnableWorkers    = 6
)

// GitHubCopilotOAuthOptions configures provider-owned GitHub Copilot OAuth
// transport. RequestTimeout covers login, token, and policy requests, while
// ModelCatalogTimeout bounds the account-specific model picker lookup.
type GitHubCopilotOAuthOptions struct {
	Client              HTTPDoer
	RequestTimeout      time.Duration
	ModelCatalogTimeout time.Duration
}

type githubCopilotOAuthConfig struct {
	client              HTTPDoer
	requestTimeout      time.Duration
	modelCatalogTimeout time.Duration
}

type githubCopilotOAuthRuntime struct {
	pollDeviceCode func(
		context.Context,
		OAuthDeviceCodePollOptions[string],
	) (string, error)
	models func() []Model
}

type githubCopilotOAuthEndpoints struct {
	deviceCode   string
	accessToken  string
	copilotToken string
}

type githubCopilotDeviceCode struct {
	DeviceCode       string
	UserCode         string
	VerificationURI  string
	IntervalSeconds  int
	ExpiresInSeconds int
}

type githubCopilotDeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	Interval        *int   `json:"interval"`
	ExpiresIn       int    `json:"expires_in"`
}

type githubCopilotDeviceTokenResponse struct {
	AccessToken      string `json:"access_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Interval         int    `json:"interval"`
}

type githubCopilotAccessTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

type githubCopilotModelRecord struct {
	ID                 string
	ModelPickerEnabled bool
	PolicyState        string
	ToolCalls          *bool
}

// NewGitHubCopilotOAuth creates the reusable GitHub Copilot device-flow
// contract. Applications may still replace it with RegisterOAuthAuthLoader.
func NewGitHubCopilotOAuth(options GitHubCopilotOAuthOptions) *OAuthAuth {
	return newGitHubCopilotOAuth(options, githubCopilotOAuthRuntime{
		pollDeviceCode: PollOAuthDeviceCodeFlow[string],
		models: func() []Model {
			return GetModels("github-copilot")
		},
	})
}

func newGitHubCopilotOAuth(
	options GitHubCopilotOAuthOptions,
	runtime githubCopilotOAuthRuntime,
) *OAuthAuth {
	if runtime.pollDeviceCode == nil {
		runtime.pollDeviceCode = PollOAuthDeviceCodeFlow[string]
	}
	if runtime.models == nil {
		runtime.models = func() []Model {
			return GetModels("github-copilot")
		}
	}
	requestTimeout := options.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = defaultGitHubCopilotOAuthTimeout
	}
	modelCatalogTimeout := options.ModelCatalogTimeout
	if modelCatalogTimeout <= 0 {
		modelCatalogTimeout = defaultCopilotCatalogTimeout
	}
	config := githubCopilotOAuthConfig{
		client:              httpClientOrDefault(options.Client),
		requestTimeout:      requestTimeout,
		modelCatalogTimeout: modelCatalogTimeout,
	}

	return &OAuthAuth{
		Name: "GitHub Copilot",
		Login: func(
			ctx context.Context,
			interaction AuthInteraction,
		) (Credential, error) {
			if interaction == nil {
				return Credential{}, errors.New(
					"GitHub Copilot OAuth auth interaction is required",
				)
			}
			ctx = contextOrBackground(ctx)
			credential, err := loginGitHubCopilot(
				ctx,
				config,
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
					"GitHub Copilot OAuth credential has no refresh token",
				)
			}
			enterpriseDomain := githubCopilotEnterpriseDomain(credential)
			refreshed, err := refreshGitHubCopilotToken(
				contextOrBackground(ctx),
				config,
				credential.Refresh,
				enterpriseDomain,
			)
			if err != nil {
				return Credential{}, err
			}
			merged := mergeRefreshedOAuthCredential(credential, refreshed)
			// Unlike opaque provider metadata, the enterprise domain is an
			// endpoint selector and must stay normalized after refresh.
			merged.EnterpriseURL = refreshed.EnterpriseURL
			return merged, nil
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
					"GitHub Copilot OAuth credential has no access token",
				)
			}
			return ModelAuth{
				APIKey: credential.Access,
				BaseURL: GitHubCopilotBaseURL(
					credential.Access,
					githubCopilotEnterpriseDomain(credential),
				),
			}, nil
		},
	}
}

func loginGitHubCopilot(
	ctx context.Context,
	config githubCopilotOAuthConfig,
	interaction AuthInteraction,
	runtime githubCopilotOAuthRuntime,
) (Credential, error) {
	input, err := interaction.Prompt(ctx, AuthPrompt{
		Type:        AuthPromptText,
		Message:     "GitHub Enterprise URL/domain (blank for github.com)",
		Placeholder: "company.ghe.com",
	})
	if err != nil {
		return Credential{}, err
	}
	if err := oauthLoginContextError(ctx); err != nil {
		return Credential{}, err
	}

	enterpriseDomain, err := normalizeGitHubCopilotDomain(input)
	if err != nil {
		return Credential{}, err
	}
	domain := enterpriseDomain
	if domain == "" {
		domain = githubCopilotDefaultDomain
	}
	device, err := requestGitHubCopilotDeviceCode(ctx, config, domain)
	if err != nil {
		return Credential{}, err
	}
	interaction.Notify(AuthEvent{
		Type:             AuthEventDeviceCode,
		UserCode:         device.UserCode,
		VerificationURI:  device.VerificationURI,
		IntervalSeconds:  device.IntervalSeconds,
		ExpiresInSeconds: device.ExpiresInSeconds,
	})

	refreshToken, err := pollGitHubCopilotAccessToken(
		ctx,
		config,
		domain,
		device,
		runtime.pollDeviceCode,
	)
	if err != nil {
		return Credential{}, err
	}
	credential, err := refreshGitHubCopilotAccessToken(
		ctx,
		config,
		refreshToken,
		enterpriseDomain,
	)
	if err != nil {
		return Credential{}, err
	}

	interaction.Notify(AuthEvent{
		Type:    AuthEventProgress,
		Message: "Enabling models...",
	})
	enableAllGitHubCopilotModels(
		ctx,
		config,
		credential,
		runtime.models(),
	)
	modelIDs, err := fetchAvailableGitHubCopilotModelIDs(
		ctx,
		config,
		credential.Access,
		enterpriseDomain,
	)
	if err != nil {
		return Credential{}, err
	}
	credential.Metadata = map[string]any{
		"availableModelIds": modelIDs,
	}
	return credential, nil
}

func normalizeGitHubCopilotDomain(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", nil
	}
	candidate := trimmed
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Hostname() == "" {
		return "", errors.New("Invalid GitHub Enterprise URL/domain")
	}
	return strings.ToLower(parsed.Hostname()), nil
}

func githubCopilotEnterpriseDomain(credential Credential) string {
	domain, err := normalizeGitHubCopilotDomain(credential.EnterpriseURL)
	if err != nil {
		return ""
	}
	return domain
}

func githubCopilotOAuthURLs(domain string) githubCopilotOAuthEndpoints {
	if strings.TrimSpace(domain) == "" {
		domain = githubCopilotDefaultDomain
	}
	return githubCopilotOAuthEndpoints{
		deviceCode:  "https://" + domain + "/login/device/code",
		accessToken: "https://" + domain + "/login/oauth/access_token",
		copilotToken: "https://api." + domain +
			"/copilot_internal/v2/token",
	}
}

func requestGitHubCopilotDeviceCode(
	ctx context.Context,
	config githubCopilotOAuthConfig,
	domain string,
) (githubCopilotDeviceCode, error) {
	endpoints := githubCopilotOAuthURLs(domain)
	body, err := doGitHubCopilotJSON(
		ctx,
		config,
		config.requestTimeout,
		"device authorization request",
		http.MethodPost,
		endpoints.deviceCode,
		strings.NewReader(url.Values{
			"client_id": {githubCopilotOAuthClientID},
			"scope":     {"read:user"},
		}.Encode()),
		map[string]string{
			"Accept":       "application/json",
			"Content-Type": "application/x-www-form-urlencoded",
			"User-Agent":   githubCopilotUserAgent,
		},
	)
	if err != nil {
		return githubCopilotDeviceCode{}, err
	}
	var response githubCopilotDeviceCodeResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return githubCopilotDeviceCode{}, fmt.Errorf(
			"GitHub Copilot device authorization returned invalid JSON: %w",
			err,
		)
	}
	if response.DeviceCode == "" ||
		response.UserCode == "" ||
		response.VerificationURI == "" ||
		response.ExpiresIn <= 0 {
		return githubCopilotDeviceCode{}, errors.New(
			"GitHub Copilot device authorization response has invalid fields",
		)
	}
	verificationURI, err := normalizeGitHubCopilotVerificationURI(
		response.VerificationURI,
	)
	if err != nil {
		return githubCopilotDeviceCode{}, err
	}
	interval := 0
	if response.Interval != nil {
		interval = *response.Interval
	}
	return githubCopilotDeviceCode{
		DeviceCode:       response.DeviceCode,
		UserCode:         response.UserCode,
		VerificationURI:  verificationURI,
		IntervalSeconds:  interval,
		ExpiresInSeconds: response.ExpiresIn,
	}, nil
}

func normalizeGitHubCopilotVerificationURI(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil ||
		parsed.Host == "" ||
		(parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", errors.New(
			"Untrusted verification_uri in device code response",
		)
	}
	return parsed.String(), nil
}

func pollGitHubCopilotAccessToken(
	ctx context.Context,
	config githubCopilotOAuthConfig,
	domain string,
	device githubCopilotDeviceCode,
	poll func(
		context.Context,
		OAuthDeviceCodePollOptions[string],
	) (string, error),
) (string, error) {
	endpoints := githubCopilotOAuthURLs(domain)
	return poll(ctx, OAuthDeviceCodePollOptions[string]{
		IntervalSeconds:     device.IntervalSeconds,
		ExpiresInSeconds:    device.ExpiresInSeconds,
		WaitBeforeFirstPoll: true,
		Poll: func(pollContext context.Context) (
			OAuthDeviceCodePollResult[string],
			error,
		) {
			body, err := doGitHubCopilotJSON(
				pollContext,
				config,
				config.requestTimeout,
				"device token request",
				http.MethodPost,
				endpoints.accessToken,
				strings.NewReader(url.Values{
					"client_id":   {githubCopilotOAuthClientID},
					"device_code": {device.DeviceCode},
					"grant_type":  {githubCopilotDeviceGrant},
				}.Encode()),
				map[string]string{
					"Accept":       "application/json",
					"Content-Type": "application/x-www-form-urlencoded",
					"User-Agent":   githubCopilotUserAgent,
				},
			)
			if err != nil {
				return OAuthDeviceCodePollResult[string]{}, err
			}
			var response githubCopilotDeviceTokenResponse
			if err := json.Unmarshal(body, &response); err != nil {
				return OAuthDeviceCodePollResult[string]{}, fmt.Errorf(
					"GitHub Copilot device token response is invalid JSON: %w",
					err,
				)
			}
			if response.AccessToken != "" {
				return OAuthDeviceCodePollResult[string]{
					Status: OAuthDeviceCodeComplete,
					Value:  response.AccessToken,
				}, nil
			}
			switch response.Error {
			case "authorization_pending":
				return OAuthDeviceCodePollResult[string]{
					Status: OAuthDeviceCodePending,
				}, nil
			case "slow_down":
				return OAuthDeviceCodePollResult[string]{
					Status:          OAuthDeviceCodeSlowDown,
					IntervalSeconds: response.Interval,
				}, nil
			case "":
				return OAuthDeviceCodePollResult[string]{
					Status:  OAuthDeviceCodeFailed,
					Message: "Invalid device token response",
				}, nil
			default:
				message := "Device flow failed: " + response.Error
				if response.ErrorDescription != "" {
					message += ": " + response.ErrorDescription
				}
				return OAuthDeviceCodePollResult[string]{
					Status:  OAuthDeviceCodeFailed,
					Message: message,
				}, nil
			}
		},
	})
}

func refreshGitHubCopilotToken(
	ctx context.Context,
	config githubCopilotOAuthConfig,
	refreshToken string,
	enterpriseDomain string,
) (Credential, error) {
	credential, err := refreshGitHubCopilotAccessToken(
		ctx,
		config,
		refreshToken,
		enterpriseDomain,
	)
	if err != nil {
		return Credential{}, err
	}
	modelIDs, err := fetchAvailableGitHubCopilotModelIDs(
		ctx,
		config,
		credential.Access,
		enterpriseDomain,
	)
	if err != nil {
		return Credential{}, err
	}
	credential.Metadata = map[string]any{
		"availableModelIds": modelIDs,
	}
	return credential, nil
}

func refreshGitHubCopilotAccessToken(
	ctx context.Context,
	config githubCopilotOAuthConfig,
	refreshToken string,
	enterpriseDomain string,
) (Credential, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return Credential{}, errors.New(
			"GitHub Copilot OAuth refresh token is missing",
		)
	}
	domain := enterpriseDomain
	if domain == "" {
		domain = githubCopilotDefaultDomain
	}
	endpoints := githubCopilotOAuthURLs(domain)
	body, err := doGitHubCopilotJSON(
		ctx,
		config,
		config.requestTimeout,
		"token request",
		http.MethodGet,
		endpoints.copilotToken,
		nil,
		githubCopilotTokenHeaders(refreshToken),
	)
	if err != nil {
		return Credential{}, err
	}
	var response githubCopilotAccessTokenResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return Credential{}, fmt.Errorf(
			"GitHub Copilot token response is invalid JSON: %w",
			err,
		)
	}
	if response.Token == "" || response.ExpiresAt <= 0 {
		return Credential{}, errors.New(
			"GitHub Copilot token response has invalid fields",
		)
	}
	refreshSkewMillis := githubCopilotRefreshSkew.Milliseconds()
	if response.ExpiresAt >
		math.MaxInt64/int64(time.Second/time.Millisecond) {
		return Credential{}, errors.New(
			"GitHub Copilot token expiry is out of range",
		)
	}
	return Credential{
		Type:          CredentialTypeOAuth,
		Access:        response.Token,
		Refresh:       refreshToken,
		Expires:       response.ExpiresAt*1000 - refreshSkewMillis,
		EnterpriseURL: enterpriseDomain,
	}, nil
}

func fetchAvailableGitHubCopilotModelIDs(
	ctx context.Context,
	config githubCopilotOAuthConfig,
	copilotToken string,
	enterpriseDomain string,
) ([]string, error) {
	baseURL := GitHubCopilotBaseURL(copilotToken, enterpriseDomain)
	headers := githubCopilotTokenHeaders(copilotToken)
	headers["X-GitHub-Api-Version"] = githubCopilotAPIVersion
	body, err := doGitHubCopilotJSON(
		ctx,
		config,
		config.modelCatalogTimeout,
		"models request",
		http.MethodGet,
		strings.TrimRight(baseURL, "/")+"/models",
		nil,
		headers,
	)
	if err != nil {
		return nil, err
	}
	return parseAvailableGitHubCopilotModelIDs(body)
}

func parseAvailableGitHubCopilotModelIDs(raw []byte) ([]string, error) {
	var response struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("Invalid Copilot models response: %w", err)
	}
	if len(response.Data) == 0 || string(response.Data) == "null" {
		return nil, errors.New("Invalid Copilot models response")
	}
	var rawItems []json.RawMessage
	if err := json.Unmarshal(response.Data, &rawItems); err != nil {
		return nil, errors.New("Invalid Copilot models response")
	}
	ids := make([]string, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, ok := parseGitHubCopilotModelRecord(rawItem)
		if !ok {
			continue
		}
		if item.ID != "" && isSelectableGitHubCopilotModel(item) {
			ids = append(ids, item.ID)
		}
	}
	return ids, nil
}

func parseGitHubCopilotModelRecord(
	raw json.RawMessage,
) (githubCopilotModelRecord, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return githubCopilotModelRecord{}, false
	}
	var item githubCopilotModelRecord
	_ = json.Unmarshal(fields["id"], &item.ID)
	_ = json.Unmarshal(
		fields["model_picker_enabled"],
		&item.ModelPickerEnabled,
	)

	var policy map[string]json.RawMessage
	if json.Unmarshal(fields["policy"], &policy) == nil && policy != nil {
		_ = json.Unmarshal(policy["state"], &item.PolicyState)
	}
	var capabilities map[string]json.RawMessage
	if json.Unmarshal(fields["capabilities"], &capabilities) == nil &&
		capabilities != nil {
		var supports map[string]json.RawMessage
		if json.Unmarshal(capabilities["supports"], &supports) == nil &&
			supports != nil {
			var toolCalls bool
			if json.Unmarshal(supports["tool_calls"], &toolCalls) == nil {
				item.ToolCalls = &toolCalls
			}
		}
	}
	return item, true
}

func isSelectableGitHubCopilotModel(item githubCopilotModelRecord) bool {
	if !item.ModelPickerEnabled {
		return false
	}
	if item.PolicyState == "disabled" {
		return false
	}
	return item.ToolCalls == nil || *item.ToolCalls
}

func enableAllGitHubCopilotModels(
	ctx context.Context,
	config githubCopilotOAuthConfig,
	credential Credential,
	models []Model,
) {
	if len(models) == 0 || contextError(ctx) != nil {
		return
	}
	workerCount := min(maxGitHubCopilotEnableWorkers, len(models))
	jobs := make(chan string)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for modelID := range jobs {
				enableGitHubCopilotModel(
					ctx,
					config,
					credential.Access,
					credential.EnterpriseURL,
					modelID,
				)
			}
		}()
	}
	for _, model := range models {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		select {
		case jobs <- model.ID:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return
		}
	}
	close(jobs)
	workers.Wait()
}

func enableGitHubCopilotModel(
	ctx context.Context,
	config githubCopilotOAuthConfig,
	copilotToken string,
	enterpriseDomain string,
	modelID string,
) bool {
	ctx, cancel := context.WithTimeout(
		contextOrBackground(ctx),
		config.requestTimeout,
	)
	defer cancel()
	baseURL := GitHubCopilotBaseURL(copilotToken, enterpriseDomain)
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(baseURL, "/")+
			"/models/"+url.PathEscape(modelID)+"/policy",
		strings.NewReader(`{"state":"enabled"}`),
	)
	if err != nil {
		return false
	}
	for key, value := range githubCopilotCommonHeaders() {
		request.Header.Set(key, value)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+copilotToken)
	request.Header.Set("openai-intent", "chat-policy")
	request.Header.Set("x-interaction-type", "chat-policy")
	response, err := config.client.Do(request)
	if err != nil || response == nil {
		return false
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
	return response.StatusCode >= 200 && response.StatusCode < 300
}

func doGitHubCopilotJSON(
	ctx context.Context,
	config githubCopilotOAuthConfig,
	timeout time.Duration,
	action string,
	method string,
	endpoint string,
	body io.Reader,
	headers map[string]string,
) ([]byte, error) {
	ctx, cancel := context.WithTimeout(contextOrBackground(ctx), timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("GitHub Copilot %s is invalid: %w", action, err)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := config.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("GitHub Copilot %s failed: %w", action, err)
	}
	if response == nil {
		return nil, fmt.Errorf(
			"GitHub Copilot %s failed: empty HTTP response",
			action,
		)
	}
	if response.Body == nil {
		return nil, fmt.Errorf(
			"GitHub Copilot %s failed: empty response body",
			action,
		)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(
		response.Body,
		maxGitHubCopilotOAuthBodyBytes+1,
	))
	if err != nil {
		return nil, fmt.Errorf(
			"GitHub Copilot %s response could not be read: %w",
			action,
			err,
		)
	}
	if len(responseBody) > maxGitHubCopilotOAuthBodyBytes {
		return nil, fmt.Errorf(
			"GitHub Copilot %s response exceeds %d bytes",
			action,
			maxGitHubCopilotOAuthBodyBytes,
		)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(responseBody))
		if message == "" {
			message = response.Status
		}
		return nil, fmt.Errorf(
			"GitHub Copilot %s failed (HTTP %d): %s",
			action,
			response.StatusCode,
			message,
		)
	}
	return responseBody, nil
}

func githubCopilotTokenHeaders(token string) map[string]string {
	headers := githubCopilotCommonHeaders()
	headers["Accept"] = "application/json"
	headers["Authorization"] = "Bearer " + token
	return headers
}

func githubCopilotCommonHeaders() map[string]string {
	return map[string]string{
		"User-Agent":             githubCopilotUserAgent,
		"Editor-Version":         githubCopilotEditorVersion,
		"Editor-Plugin-Version":  githubCopilotPluginVersion,
		"Copilot-Integration-Id": githubCopilotIntegrationID,
	}
}
