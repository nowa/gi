package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	githubCopilotOAuthClientID  = "Iv1.b507a08c87ecfe98"
	githubCopilotDefaultDomain  = "github.com"
	githubCopilotDeviceGrant    = "urn:ietf:params:oauth:grant-type:device_code"
	githubCopilotUserAgent      = "GitHubCopilotChat/0.35.0"
	githubCopilotEditorVersion  = "vscode/1.107.0"
	githubCopilotPluginVersion  = "copilot-chat/0.35.0"
	githubCopilotIntegrationID  = "vscode-chat"
	githubCopilotDefaultTimeout = 30 * time.Second
)

type githubCopilotDeviceCode struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	Interval        int
	ExpiresIn       int
}

type githubCopilotOAuthRuntime struct {
	StartDeviceFlow func(context.Context, string) (githubCopilotDeviceCode, error)
	PollAccessToken func(context.Context, string, githubCopilotDeviceCode) (string, error)
	RefreshToken    func(context.Context, string, string) (AuthCredential, error)
	EnableModels    func(context.Context, AuthCredential) error
	OpenBrowser     func(string) error
}

var defaultGitHubCopilotOAuthRuntime = githubCopilotOAuthRuntime{
	StartDeviceFlow: startGitHubCopilotDeviceFlow,
	PollAccessToken: pollGitHubCopilotAccessToken,
	RefreshToken:    refreshGitHubCopilotOAuthTokenWithContext,
	EnableModels:    enableGitHubCopilotModels,
	OpenBrowser:     defaultOpenOAuthURL,
}

var githubCopilotHTTPClient = &http.Client{Timeout: githubCopilotDefaultTimeout}

func normalizeGitHubCopilotDomain(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", true
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		parsed, err = url.Parse("https://" + trimmed)
	}
	if err != nil || parsed.Host == "" {
		return "", false
	}
	return parsed.Hostname(), parsed.Hostname() != ""
}

func githubCopilotOAuthURLs(domain string) (deviceCodeURL, accessTokenURL, copilotTokenURL string) {
	if strings.TrimSpace(domain) == "" {
		domain = githubCopilotDefaultDomain
	}
	return "https://" + domain + "/login/device/code",
		"https://" + domain + "/login/oauth/access_token",
		"https://api." + domain + "/copilot_internal/v2/token"
}

func startGitHubCopilotDeviceFlow(ctx context.Context, domain string) (githubCopilotDeviceCode, error) {
	deviceCodeURL, _, _ := githubCopilotOAuthURLs(domain)
	values := url.Values{
		"client_id": {githubCopilotOAuthClientID},
		"scope":     {"read:user"},
	}
	request, err := newGitHubCopilotFormRequest(ctx, deviceCodeURL, values)
	if err != nil {
		return githubCopilotDeviceCode{}, err
	}
	response, err := githubCopilotHTTPClient.Do(request)
	if err != nil {
		return githubCopilotDeviceCode{}, fmt.Errorf("GitHub Copilot device flow request failed: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return githubCopilotDeviceCode{}, fmt.Errorf("GitHub Copilot device flow failed (%d): %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		Interval        int    `json:"interval"`
		ExpiresIn       int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return githubCopilotDeviceCode{}, fmt.Errorf("GitHub Copilot device flow returned invalid JSON: %w", err)
	}
	if parsed.DeviceCode == "" || parsed.UserCode == "" || parsed.VerificationURI == "" || parsed.Interval <= 0 || parsed.ExpiresIn <= 0 {
		return githubCopilotDeviceCode{}, fmt.Errorf("GitHub Copilot device flow response missing fields: %s", strings.TrimSpace(string(body)))
	}
	return githubCopilotDeviceCode(parsed), nil
}

func pollGitHubCopilotAccessToken(ctx context.Context, domain string, device githubCopilotDeviceCode) (string, error) {
	_, accessTokenURL, _ := githubCopilotOAuthURLs(domain)
	deadline := time.Now().Add(time.Duration(device.ExpiresIn) * time.Second)
	interval := time.Duration(max(1, device.Interval)) * time.Second
	multiplier := 1.2
	slowDownResponses := 0
	for time.Now().Before(deadline) {
		wait := time.Duration(float64(interval) * multiplier)
		if remaining := time.Until(deadline); wait > remaining {
			wait = remaining
		}
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return "", errors.New("Login cancelled")
			}
		}
		values := url.Values{
			"client_id":   {githubCopilotOAuthClientID},
			"device_code": {device.DeviceCode},
			"grant_type":  {githubCopilotDeviceGrant},
		}
		request, err := newGitHubCopilotFormRequest(ctx, accessTokenURL, values)
		if err != nil {
			return "", err
		}
		response, err := githubCopilotHTTPClient.Do(request)
		if err != nil {
			return "", fmt.Errorf("GitHub Copilot access token polling failed: %w", err)
		}
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		var parsed struct {
			AccessToken      string `json:"access_token"`
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
			Interval         int    `json:"interval"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return "", fmt.Errorf("GitHub Copilot access token response invalid JSON: %w", err)
		}
		if parsed.AccessToken != "" {
			return parsed.AccessToken, nil
		}
		switch parsed.Error {
		case "", "authorization_pending":
			continue
		case "slow_down":
			slowDownResponses++
			if parsed.Interval > 0 {
				interval = time.Duration(parsed.Interval) * time.Second
			} else {
				interval += 5 * time.Second
			}
			multiplier = 1.4
		default:
			if parsed.ErrorDescription != "" {
				return "", fmt.Errorf("Device flow failed: %s: %s", parsed.Error, parsed.ErrorDescription)
			}
			return "", fmt.Errorf("Device flow failed: %s", parsed.Error)
		}
	}
	if slowDownResponses > 0 {
		return "", errors.New("Device flow timed out after one or more slow_down responses")
	}
	return "", errors.New("Device flow timed out")
}

func refreshGitHubCopilotOAuthToken(credential AuthCredential) (AuthCredential, error) {
	return refreshGitHubCopilotOAuthTokenWithContext(context.Background(), credential.Refresh, credential.EnterpriseURL)
}

func refreshGitHubCopilotOAuthTokenWithContext(ctx context.Context, refreshToken, enterpriseDomain string) (AuthCredential, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return AuthCredential{}, errors.New("GitHub Copilot refresh token is missing")
	}
	domain := firstNonEmptyString(strings.TrimSpace(enterpriseDomain), githubCopilotDefaultDomain)
	_, _, copilotTokenURL := githubCopilotOAuthURLs(domain)
	ctx, cancel := context.WithTimeout(ctx, githubCopilotDefaultTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, copilotTokenURL, nil)
	if err != nil {
		return AuthCredential{}, err
	}
	for key, value := range githubCopilotTokenHeaders(refreshToken) {
		request.Header.Set(key, value)
	}
	response, err := githubCopilotHTTPClient.Do(request)
	if err != nil {
		return AuthCredential{}, fmt.Errorf("GitHub Copilot token request failed: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return AuthCredential{}, fmt.Errorf("GitHub Copilot token request failed (%d): %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return AuthCredential{}, fmt.Errorf("GitHub Copilot token response invalid JSON: %w", err)
	}
	if parsed.Token == "" || parsed.ExpiresAt <= 0 {
		return AuthCredential{}, fmt.Errorf("GitHub Copilot token response missing fields: %s", strings.TrimSpace(string(body)))
	}
	return AuthCredential{
		Type:          "oauth",
		Access:        parsed.Token,
		Refresh:       refreshToken,
		Expires:       parsed.ExpiresAt*1000 - 5*60*1000,
		EnterpriseURL: strings.TrimSpace(enterpriseDomain),
	}, nil
}

func enableGitHubCopilotModels(ctx context.Context, credential AuthCredential) error {
	baseURL := llm.GitHubCopilotBaseURL(credential.Access, credential.EnterpriseURL)
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	client := githubCopilotHTTPClient
	for _, model := range llm.GetModels("github-copilot") {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/models/"+url.PathEscape(model.ID)+"/policy", strings.NewReader(`{"state":"enabled"}`))
		if err != nil {
			continue
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+credential.Access)
		for key, value := range githubCopilotCommonHeaders() {
			request.Header.Set(key, value)
		}
		request.Header.Set("openai-intent", "chat-policy")
		request.Header.Set("x-interaction-type", "chat-policy")
		response, err := client.Do(request)
		if err == nil && response != nil {
			_ = response.Body.Close()
		}
	}
	return nil
}

func newGitHubCopilotFormRequest(ctx context.Context, endpoint string, values url.Values) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", githubCopilotUserAgent)
	return request, nil
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
