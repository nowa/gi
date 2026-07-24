package gillmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestKimiCodingOAuthLogsInWithDeviceAuthorizationFlow(t *testing.T) {
	start := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	clock := newOAuthTestClock(start)
	var pollTimes []time.Time
	tokenReplies := []*http.Response{
		kimiJSONResponse(http.StatusBadRequest, map[string]any{
			"error": "authorization_pending",
		}),
		kimiJSONResponse(http.StatusOK, map[string]any{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"expires_in":    3600,
		}),
	}
	client := radiusHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case defaultKimiOAuthHost + "/api/oauth/device_authorization":
			if request.Method != http.MethodPost ||
				request.Header.Get("Content-Type") !=
					"application/x-www-form-urlencoded" ||
				request.Header.Get("Accept") != "application/json" {
				t.Fatalf("device request = %#v", request)
			}
			form := readKimiOAuthForm(t, request)
			if form.Get("client_id") != kimiOAuthClientID {
				t.Fatalf("device form = %v", form)
			}
			return kimiDeviceAuthorizationResponse(nil), nil
		case defaultKimiOAuthHost + "/api/oauth/token":
			pollTimes = append(pollTimes, clock.now())
			form := readKimiOAuthForm(t, request)
			if form.Get("grant_type") !=
				"urn:ietf:params:oauth:grant-type:device_code" ||
				form.Get("client_id") != kimiOAuthClientID ||
				form.Get("device_code") != "device-code-123" {
				t.Fatalf("token form = %v", form)
			}
			if len(tokenReplies) == 0 {
				t.Fatal("unexpected extra token poll")
			}
			response := tokenReplies[0]
			tokenReplies = tokenReplies[1:]
			return response, nil
		default:
			t.Fatalf("unexpected request %s", request.URL)
			return nil, nil
		}
	})
	interaction := &queuedProviderAuthInteraction{}
	credential, err := newTestKimiCodingOAuth(
		KimiCodingOAuthOptions{Client: client},
		clock,
	).Login(context.Background(), interaction)
	if err != nil {
		t.Fatal(err)
	}
	wantPollTimes := []time.Time{
		start.Add(5 * time.Second),
		start.Add(10 * time.Second),
	}
	if !equalOAuthPollTimes(pollTimes, wantPollTimes) {
		t.Fatalf("poll times = %v, want %v", pollTimes, wantPollTimes)
	}
	if credential.Type != CredentialTypeOAuth ||
		credential.Access != "access-token" ||
		credential.Refresh != "refresh-token" ||
		credential.Expires != start.Add(10*time.Second).Add(time.Hour).UnixMilli() {
		t.Fatalf("credential = %#v", credential)
	}
	if len(interaction.events) != 1 {
		t.Fatalf("events = %#v", interaction.events)
	}
	event := interaction.events[0]
	if event.Type != AuthEventDeviceCode ||
		event.UserCode != "ABCD-1234" ||
		event.VerificationURI !=
			"https://www.kimi.com/code?user_code=ABCD-1234" ||
		event.IntervalSeconds != 5 ||
		event.ExpiresInSeconds != 600 {
		t.Fatalf("event = %#v", event)
	}
}

func TestKimiCodingOAuthDeviceFlowTerminalStates(t *testing.T) {
	start := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name    string
		code    string
		message string
	}{
		{
			name:    "fails when the device code expires",
			code:    "expired_token",
			message: "expired",
		},
		{
			name:    "fails when the user denies the login",
			code:    "access_denied",
			message: "denied",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			clock := newOAuthTestClock(start)
			client := kimiDeviceFlowClient(
				t,
				kimiDeviceAuthorizationPayload(nil),
				func(*http.Request) (*http.Response, error) {
					return kimiJSONResponse(
						http.StatusBadRequest,
						map[string]any{"error": test.code},
					), nil
				},
			)
			_, err := newTestKimiCodingOAuth(
				KimiCodingOAuthOptions{Client: client},
				clock,
			).Login(
				context.Background(),
				&queuedProviderAuthInteraction{},
			)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestKimiCodingOAuthHonorsHostOverrides(t *testing.T) {
	start := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name string
		env  map[string]string
		host string
	}{
		{
			name: "honors the KIMI_CODE_OAUTH_HOST override",
			env: map[string]string{
				"KIMI_CODE_OAUTH_HOST": "https://auth.example.com/",
				"KIMI_OAUTH_HOST":      "https://legacy.example.com",
			},
			host: "https://auth.example.com",
		},
		{
			name: "falls back to the legacy KIMI_OAUTH_HOST override",
			env: map[string]string{
				"KIMI_OAUTH_HOST": "https://legacy.example.com/",
			},
			host: "https://legacy.example.com",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			clock := newOAuthTestClock(start)
			var urls []string
			client := radiusHTTPDoerFunc(func(
				request *http.Request,
			) (*http.Response, error) {
				urls = append(urls, request.URL.String())
				switch request.URL.String() {
				case test.host + "/api/oauth/device_authorization":
					return kimiDeviceAuthorizationResponse(
						map[string]any{"interval": 1},
					), nil
				case test.host + "/api/oauth/token":
					return kimiJSONResponse(http.StatusOK, map[string]any{
						"access_token":  "a",
						"refresh_token": "r",
						"expires_in":    60,
					}), nil
				default:
					t.Fatalf("unexpected request %s", request.URL)
					return nil, nil
				}
			})
			credential, err := newTestKimiCodingOAuth(
				KimiCodingOAuthOptions{
					Client:      client,
					AuthContext: providerAuthContext(test.env),
				},
				clock,
			).Login(
				context.Background(),
				&queuedProviderAuthInteraction{},
			)
			if err != nil {
				t.Fatal(err)
			}
			if credential.Access != "a" || credential.Refresh != "r" {
				t.Fatalf("credential = %#v", credential)
			}
			want := []string{
				test.host + "/api/oauth/device_authorization",
				test.host + "/api/oauth/token",
			}
			if !reflect.DeepEqual(urls, want) {
				t.Fatalf("URLs = %v, want %v", urls, want)
			}
		})
	}
}

func TestKimiCodingOAuthRefreshesAndBuildsBearerAuth(t *testing.T) {
	now := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	clock := newOAuthTestClock(now)
	client := radiusHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != defaultKimiOAuthHost+"/api/oauth/token" {
			t.Fatalf("unexpected request %s", request.URL)
		}
		form := readKimiOAuthForm(t, request)
		if form.Get("grant_type") != "refresh_token" ||
			form.Get("refresh_token") != "old-refresh" ||
			form.Get("client_id") != kimiOAuthClientID {
			t.Fatalf("refresh form = %v", form)
		}
		return kimiJSONResponse(http.StatusOK, map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"expires_in":    3600,
		}), nil
	})
	auth := newTestKimiCodingOAuth(
		KimiCodingOAuthOptions{Client: client},
		clock,
	)
	credential, err := auth.Refresh(context.Background(), Credential{
		Type:          CredentialTypeOAuth,
		Access:        "old-access",
		Refresh:       "old-refresh",
		Env:           ProviderEnv{"KIMI_SCOPE": "team-a"},
		EnterpriseURL: "https://enterprise.kimi.test",
		Metadata:      map[string]any{"account": "account-a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if credential.Type != CredentialTypeOAuth ||
		credential.Access != "new-access" ||
		credential.Refresh != "new-refresh" ||
		credential.Expires != now.Add(time.Hour).UnixMilli() ||
		credential.Env["KIMI_SCOPE"] != "team-a" ||
		credential.EnterpriseURL != "https://enterprise.kimi.test" ||
		credential.Metadata["account"] != "account-a" {
		t.Fatalf("credential = %#v", credential)
	}
	resolved, err := auth.ToAuth(context.Background(), credential)
	if err != nil ||
		resolved.Headers["Authorization"] != "Bearer new-access" {
		t.Fatalf("auth = %#v, error = %v", resolved, err)
	}
}

func TestKimiCodingOAuthRefreshRetryContracts(t *testing.T) {
	start := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)

	t.Run("retries refresh on 429", func(t *testing.T) {
		clock := newOAuthTestClock(start)
		var delays []time.Duration
		sleep := clock.sleep
		clock.sleep = func(
			ctx context.Context,
			delay time.Duration,
		) error {
			delays = append(delays, delay)
			return sleep(ctx, delay)
		}
		calls := 0
		client := radiusHTTPDoerFunc(func(
			*http.Request,
		) (*http.Response, error) {
			calls++
			if calls == 1 {
				return kimiJSONResponse(
					http.StatusTooManyRequests,
					map[string]any{"error": "temporarily_unavailable"},
				), nil
			}
			return kimiJSONResponse(http.StatusOK, map[string]any{
				"access_token":  "a",
				"refresh_token": "r",
				"expires_in":    60,
			}), nil
		})
		credential, err := newTestKimiCodingOAuth(
			KimiCodingOAuthOptions{Client: client},
			clock,
		).Refresh(context.Background(), Credential{
			Type:    CredentialTypeOAuth,
			Refresh: "old",
		})
		if err != nil {
			t.Fatal(err)
		}
		if credential.Access != "a" ||
			calls != 2 ||
			!reflect.DeepEqual(delays, []time.Duration{time.Second}) {
			t.Fatalf(
				"credential=%#v calls=%d delays=%v",
				credential,
				calls,
				delays,
			)
		}
	})

	t.Run("fails unauthorized on invalid_grant without retry", func(t *testing.T) {
		clock := newOAuthTestClock(start)
		calls := 0
		client := radiusHTTPDoerFunc(func(
			*http.Request,
		) (*http.Response, error) {
			calls++
			return kimiJSONResponse(
				http.StatusBadRequest,
				map[string]any{"error": "invalid_grant"},
			), nil
		})
		_, err := newTestKimiCodingOAuth(
			KimiCodingOAuthOptions{Client: client},
			clock,
		).Refresh(context.Background(), Credential{
			Type:    CredentialTypeOAuth,
			Refresh: "old",
		})
		if err == nil ||
			!strings.Contains(err.Error(), "unauthorized") ||
			calls != 1 {
			t.Fatalf("calls=%d error=%v", calls, err)
		}
	})

	t.Run("retries transport failures with one two four second backoff", func(t *testing.T) {
		clock := newOAuthTestClock(start)
		var delays []time.Duration
		sleep := clock.sleep
		clock.sleep = func(
			ctx context.Context,
			delay time.Duration,
		) error {
			delays = append(delays, delay)
			return sleep(ctx, delay)
		}
		calls := 0
		client := radiusHTTPDoerFunc(func(
			*http.Request,
		) (*http.Response, error) {
			calls++
			return nil, errors.New("network unavailable")
		})
		_, err := newTestKimiCodingOAuth(
			KimiCodingOAuthOptions{Client: client},
			clock,
		).Refresh(context.Background(), Credential{
			Type:    CredentialTypeOAuth,
			Refresh: "old",
		})
		wantDelays := []time.Duration{
			time.Second,
			2 * time.Second,
			4 * time.Second,
		}
		if err == nil ||
			!strings.Contains(err.Error(), "network unavailable") ||
			calls != 4 ||
			!reflect.DeepEqual(delays, wantDelays) {
			t.Fatalf(
				"calls=%d delays=%v error=%v",
				calls,
				delays,
				err,
			)
		}
	})
}

func TestKimiCodingOAuthValidatesDeviceURLsAndDefaults(t *testing.T) {
	start := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	t.Run("uses interval and expiry defaults", func(t *testing.T) {
		clock := newOAuthTestClock(start)
		client := kimiDeviceFlowClient(
			t,
			kimiDeviceAuthorizationPayload(map[string]any{
				"interval":   0,
				"expires_in": "invalid",
			}),
			func(*http.Request) (*http.Response, error) {
				return kimiJSONResponse(http.StatusOK, map[string]any{
					"access_token":  "a",
					"refresh_token": "r",
					"expires_in":    60,
				}), nil
			},
		)
		interaction := &queuedProviderAuthInteraction{}
		if _, err := newTestKimiCodingOAuth(
			KimiCodingOAuthOptions{Client: client},
			clock,
		).Login(context.Background(), interaction); err != nil {
			t.Fatal(err)
		}
		if len(interaction.events) != 1 ||
			interaction.events[0].IntervalSeconds !=
				defaultKimiPollIntervalSeconds ||
			interaction.events[0].ExpiresInSeconds !=
				defaultKimiDeviceTimeoutSeconds {
			t.Fatalf("events = %#v", interaction.events)
		}
	})

	for _, raw := range []string{
		"file:///etc/passwd",
		"javascript:alert(1)",
		"not a URL",
	} {
		t.Run("rejects untrusted device URL "+raw, func(t *testing.T) {
			client := kimiDeviceFlowClient(
				t,
				kimiDeviceAuthorizationPayload(map[string]any{
					"verification_uri_complete": raw,
				}),
				nil,
			)
			_, err := NewKimiCodingOAuth(KimiCodingOAuthOptions{
				Client: client,
			}).Login(
				context.Background(),
				&queuedProviderAuthInteraction{},
			)
			if err == nil ||
				!strings.Contains(
					err.Error(),
					"Invalid Kimi Code device authorization response",
				) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestBuiltinKimiCodingProviderUsesDefaultOAuth(t *testing.T) {
	provider, err := NewBuiltinProvider("kimi-coding")
	if err != nil {
		t.Fatal(err)
	}
	if provider.Auth.OAuth == nil ||
		provider.Auth.OAuth.Name != "Kimi Code (subscription)" ||
		provider.Auth.OAuth.LoginLabel != "Sign in with Kimi Code" {
		t.Fatalf("OAuth = %#v", provider.Auth.OAuth)
	}
	resolved, err := provider.Auth.OAuth.ToAuth(
		context.Background(),
		Credential{Type: CredentialTypeOAuth, Access: "built-in-token"},
	)
	if err != nil ||
		resolved.Headers["Authorization"] != "Bearer built-in-token" {
		t.Fatalf("auth = %#v, error = %v", resolved, err)
	}
}

func TestKimiCodingOAuthRefreshCancellationIsPrompt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	clock := newOAuthTestClock(time.Now())
	calls := 0
	client := radiusHTTPDoerFunc(func(
		*http.Request,
	) (*http.Response, error) {
		calls++
		cancel()
		return nil, context.Canceled
	})
	_, err := newTestKimiCodingOAuth(
		KimiCodingOAuthOptions{Client: client},
		clock,
	).Refresh(ctx, Credential{
		Type:    CredentialTypeOAuth,
		Refresh: "old",
	})
	if !errors.Is(err, context.Canceled) ||
		!strings.Contains(err.Error(), "refresh aborted") ||
		calls != 1 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func newTestKimiCodingOAuth(
	options KimiCodingOAuthOptions,
	clock *oauthTestClock,
) *OAuthAuth {
	return newKimiCodingOAuth(options, kimiOAuthRuntime{
		now: clock.now,
		pollDeviceCode: func(
			ctx context.Context,
			options OAuthDeviceCodePollOptions[kimiToken],
		) (kimiToken, error) {
			return pollOAuthDeviceCodeFlow(
				ctx,
				options,
				clock.runtime(),
			)
		},
		sleep: clock.sleep,
	})
}

func kimiDeviceFlowClient(
	t *testing.T,
	device map[string]any,
	token func(*http.Request) (*http.Response, error),
) HTTPDoer {
	t.Helper()
	return radiusHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case defaultKimiOAuthHost + "/api/oauth/device_authorization":
			return kimiJSONResponse(http.StatusOK, device), nil
		case defaultKimiOAuthHost + "/api/oauth/token":
			if token == nil {
				t.Fatal("unexpected token request")
				return nil, nil
			}
			return token(request)
		default:
			t.Fatalf("unexpected request %s", request.URL)
			return nil, nil
		}
	})
}

func kimiDeviceAuthorizationResponse(
	overrides map[string]any,
) *http.Response {
	return kimiJSONResponse(
		http.StatusOK,
		kimiDeviceAuthorizationPayload(overrides),
	)
}

func kimiDeviceAuthorizationPayload(
	overrides map[string]any,
) map[string]any {
	payload := map[string]any{
		"user_code":                 "ABCD-1234",
		"device_code":               "device-code-123",
		"verification_uri":          "https://www.kimi.com/code",
		"verification_uri_complete": "https://www.kimi.com/code?user_code=ABCD-1234",
		"interval":                  5,
		"expires_in":                600,
	}
	for key, value := range overrides {
		payload[key] = value
	}
	return payload
}

func kimiJSONResponse(status int, value any) *http.Response {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}
}

func readKimiOAuthForm(t *testing.T, request *http.Request) url.Values {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatal(err)
	}
	return values
}
