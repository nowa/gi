package gillmprovider

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAnthropicOAuthTokenRequests(t *testing.T) {
	exchange := BuildAnthropicAuthorizationCodeTokenRequest("manual-code", "http://localhost:53692/callback")
	if exchange.URL != AnthropicOAuthTokenURL {
		t.Fatalf("url = %q", exchange.URL)
	}
	if exchange.Body["grant_type"] != "authorization_code" || exchange.Body["code"] != "manual-code" || exchange.Body["redirect_uri"] != "http://localhost:53692/callback" {
		t.Fatalf("exchange body = %#v", exchange.Body)
	}

	refresh := BuildAnthropicRefreshTokenRequest("refresh-token")
	if refresh.Body["grant_type"] != "refresh_token" || refresh.Body["refresh_token"] != "refresh-token" || refresh.Body["client_id"] == "" {
		t.Fatalf("refresh body = %#v", refresh.Body)
	}
	if _, ok := refresh.Body["scope"]; ok {
		t.Fatalf("refresh request must omit scope: %#v", refresh.Body)
	}
}

func TestGitHubCopilotPollScheduleSlowDown(t *testing.T) {
	start := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC).UnixMilli()

	polls, err := GitHubCopilotPollSchedule(start, 5, 900, []GitHubCopilotDevicePollResponse{
		{Error: "authorization_pending"},
		{Error: "slow_down", Interval: 10},
		{AccessToken: "ghu_refresh_token"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{start + 6000, start + 12000, start + 26000}
	if !reflect.DeepEqual(polls, want) {
		t.Fatalf("polls = %#v, want %#v", polls, want)
	}
}

func TestGitHubCopilotPollScheduleFinalPollBeforeSlowDownTimeout(t *testing.T) {
	start := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC).UnixMilli()

	polls, err := GitHubCopilotPollSchedule(start, 5, 25, []GitHubCopilotDevicePollResponse{
		{Error: "slow_down", Interval: 10},
		{Error: "slow_down", Interval: 15},
		{Error: "authorization_pending"},
	})
	if err == nil || !strings.Contains(err.Error(), "Device flow timed out after one or more slow_down responses") {
		t.Fatalf("err = %v", err)
	}
	want := []int64{start + 6000, start + 20000, start + 25000}
	if !reflect.DeepEqual(polls, want) {
		t.Fatalf("polls = %#v, want %#v", polls, want)
	}
}

func TestGitHubCopilotBaseURLFromToken(t *testing.T) {
	token := "tid=test;exp=9999999999;proxy-ep=proxy.individual.githubcopilot.com;"
	if got := GitHubCopilotBaseURL(token, ""); got != "https://api.individual.githubcopilot.com" {
		t.Fatalf("base URL = %q", got)
	}
	if got := GitHubCopilotBaseURL("", "example.com"); got != "https://copilot-api.example.com" {
		t.Fatalf("enterprise base URL = %q", got)
	}
}

func TestOpenAICodexRefreshErrorMessage(t *testing.T) {
	err := OpenAICodexRefreshError(401, "Unauthorized", `{"error":{"message":"Could not validate your token. Please try signing in again.","type":"invalid_request_error"}}`)
	if err == nil || !strings.Contains(err.Error(), "OpenAI Codex token refresh failed (401)") || !strings.Contains(err.Error(), "Could not validate your token") {
		t.Fatalf("err = %v", err)
	}
}
