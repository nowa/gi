package gillmprovider

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	AnthropicOAuthTokenURL = "https://platform.claude.com/v1/oauth/token"
	AnthropicOAuthClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
)

type OAuthCredentials struct {
	Access  string
	Refresh string
	Expires int64
}

type OAuthTokenRequest struct {
	URL  string
	Body map[string]string
}

func BuildAnthropicAuthorizationCodeTokenRequest(code, redirectURI string) OAuthTokenRequest {
	return OAuthTokenRequest{
		URL: AnthropicOAuthTokenURL,
		Body: map[string]string{
			"grant_type":   "authorization_code",
			"client_id":    AnthropicOAuthClientID,
			"code":         code,
			"redirect_uri": redirectURI,
		},
	}
}

func BuildAnthropicRefreshTokenRequest(refreshToken string) OAuthTokenRequest {
	return OAuthTokenRequest{
		URL: AnthropicOAuthTokenURL,
		Body: map[string]string{
			"grant_type":    "refresh_token",
			"client_id":     AnthropicOAuthClientID,
			"refresh_token": refreshToken,
		},
	}
}

type GitHubCopilotDevicePollResponse struct {
	AccessToken string
	Error       string
	Interval    int
}

func GitHubCopilotPollSchedule(startMillis int64, intervalSeconds, expiresSeconds int, responses []GitHubCopilotDevicePollResponse) ([]int64, error) {
	deadline := startMillis + int64(expiresSeconds)*1000
	now := startMillis
	intervalMs := intervalSeconds * 1000
	if intervalMs < 1000 {
		intervalMs = 1000
	}
	multiplier := 1.2
	slowDownResponses := 0
	pollTimes := make([]int64, 0, len(responses))

	for i := 0; now < deadline && i < len(responses); i++ {
		remaining := deadline - now
		waitMs := int64(float64(intervalMs)*multiplier + 0.999999)
		if waitMs > remaining {
			waitMs = remaining
		}
		now += waitMs
		pollTimes = append(pollTimes, now)

		response := responses[i]
		if response.AccessToken != "" {
			return pollTimes, nil
		}
		switch response.Error {
		case "", "authorization_pending":
			continue
		case "slow_down":
			slowDownResponses++
			if response.Interval > 0 {
				intervalMs = response.Interval * 1000
			} else {
				intervalMs += 5000
				if intervalMs < 1000 {
					intervalMs = 1000
				}
			}
			multiplier = 1.4
		default:
			return pollTimes, fmt.Errorf("Device flow failed: %s", response.Error)
		}
	}
	if slowDownResponses > 0 {
		return pollTimes, fmt.Errorf("Device flow timed out after one or more slow_down responses")
	}
	return pollTimes, fmt.Errorf("Device flow timed out")
}

func GitHubCopilotBaseURL(token, enterpriseDomain string) string {
	if token != "" {
		for _, part := range strings.Split(token, ";") {
			key, value, ok := strings.Cut(part, "=")
			if ok && key == "proxy-ep" && value != "" {
				return "https://" + strings.Replace(value, "proxy.", "api.", 1)
			}
		}
	}
	if enterpriseDomain != "" {
		return "https://copilot-api." + enterpriseDomain
	}
	return "https://api.githubcopilot.com"
}

func OpenAICodexRefreshError(status int, statusText, body string) error {
	message := statusText
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err == nil && parsed.Error.Message != "" {
		message = parsed.Error.Message
	} else if strings.TrimSpace(body) != "" {
		message = strings.TrimSpace(body)
	}
	return fmt.Errorf("OpenAI Codex token refresh failed (%d): %s", status, message)
}
