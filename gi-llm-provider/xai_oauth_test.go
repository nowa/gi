package gillmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestXAIOAuthDeviceFlowPiContracts(t *testing.T) {
	start := time.Date(2026, 7, 9, 20, 0, 0, 0, time.UTC)

	t.Run("uses the device grant, delays polling, and handles pending and slow_down", func(t *testing.T) {
		clock := newOAuthTestClock(start)
		var pollTimes []time.Time
		tokenReplies := []*http.Response{
			xaiJSONResponse(http.StatusBadRequest, map[string]any{
				"error": "authorization_pending",
			}),
			xaiJSONResponse(http.StatusBadRequest, map[string]any{
				"error":    "slow_down",
				"interval": 10,
			}),
			xaiJSONResponse(http.StatusOK, xaiTokenResponse(nil)),
		}
		client := radiusHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
			switch request.URL.String() {
			case xaiOAuthDeviceCodeURL:
				form := readXAIOAuthForm(t, request)
				if form.Get("client_id") != xaiOAuthClientID ||
					form.Get("scope") != xaiOAuthScope ||
					form.Get("referrer") != "pi" {
					t.Fatalf("device form = %v", form)
				}
				return xaiJSONResponse(
					http.StatusOK,
					xaiDeviceCodeResponse(nil),
				), nil
			case xaiOAuthTokenURL:
				pollTimes = append(pollTimes, clock.now())
				form := readXAIOAuthForm(t, request)
				if form.Get("grant_type") !=
					"urn:ietf:params:oauth:grant-type:device_code" ||
					form.Get("client_id") != xaiOAuthClientID ||
					form.Get("device_code") != "device-code" {
					t.Fatalf("token form = %v", form)
				}
				if len(tokenReplies) == 0 {
					t.Fatal("unexpected token poll")
				}
				response := tokenReplies[0]
				tokenReplies = tokenReplies[1:]
				return response, nil
			default:
				t.Fatalf("unexpected request %s", request.URL)
				return nil, nil
			}
		})
		auth := newTestXAIOAuth(client, clock)
		interaction := &queuedProviderAuthInteraction{}
		credential, err := auth.Login(context.Background(), interaction)
		if err != nil {
			t.Fatal(err)
		}
		wantPollTimes := []time.Time{
			start.Add(5 * time.Second),
			start.Add(10 * time.Second),
			start.Add(20 * time.Second),
		}
		if !equalOAuthPollTimes(pollTimes, wantPollTimes) {
			t.Fatalf("poll times = %v, want %v", pollTimes, wantPollTimes)
		}
		if credential.Type != CredentialTypeOAuth ||
			credential.Access != "access-token" ||
			credential.Refresh != "refresh-token" ||
			credential.Expires != start.
				Add(20*time.Second).
				Add(6*time.Hour).
				Add(-xaiOAuthRefreshSkew).
				UnixMilli() {
			t.Fatalf("credential = %#v", credential)
		}
		if len(interaction.events) != 1 {
			t.Fatalf("events = %#v", interaction.events)
		}
		event := interaction.events[0]
		if event.Type != AuthEventDeviceCode ||
			event.UserCode != "ABCD-1234" ||
			event.VerificationURI != "https://accounts.x.ai/oauth2/device" ||
			event.IntervalSeconds != 5 ||
			event.ExpiresInSeconds != 900 {
			t.Fatalf("event = %#v", event)
		}
	})

	t.Run("falls back to the default poll interval when interval is zero", func(t *testing.T) {
		clock := newOAuthTestClock(start)
		var pollTimes []time.Time
		client := xaiDeviceFlowClient(
			t,
			xaiDeviceCodeResponse(map[string]any{"interval": 0}),
			func(request *http.Request) (*http.Response, error) {
				pollTimes = append(pollTimes, clock.now())
				return xaiJSONResponse(
					http.StatusOK,
					xaiTokenResponse(nil),
				), nil
			},
		)
		_, err := newTestXAIOAuth(client, clock).Login(
			context.Background(),
			&queuedProviderAuthInteraction{},
		)
		if err != nil {
			t.Fatal(err)
		}
		want := []time.Time{start.Add(5 * time.Second)}
		if !equalOAuthPollTimes(pollTimes, want) {
			t.Fatalf("poll times = %v, want %v", pollTimes, want)
		}
	})

	t.Run("prefers verification_uri_complete", func(t *testing.T) {
		clock := newOAuthTestClock(start)
		complete := "https://accounts.x.ai/oauth2/device?user_code=ABCD-1234"
		client := xaiDeviceFlowClient(
			t,
			xaiDeviceCodeResponse(map[string]any{
				"verification_uri_complete": complete,
			}),
			func(*http.Request) (*http.Response, error) {
				return xaiJSONResponse(
					http.StatusOK,
					xaiTokenResponse(nil),
				), nil
			},
		)
		interaction := &queuedProviderAuthInteraction{}
		if _, err := newTestXAIOAuth(client, clock).Login(
			context.Background(),
			interaction,
		); err != nil {
			t.Fatal(err)
		}
		if len(interaction.events) != 1 ||
			interaction.events[0].VerificationURI != complete {
			t.Fatalf("events = %#v", interaction.events)
		}
	})

	t.Run("rejects a non-https verification_uri_complete", func(t *testing.T) {
		client := xaiDeviceFlowClient(
			t,
			xaiDeviceCodeResponse(map[string]any{
				"verification_uri_complete": "http://accounts.x.ai/device",
			}),
			nil,
		)
		_, err := NewXAIOAuth(XAIOAuthOptions{Client: client}).Login(
			context.Background(),
			&queuedProviderAuthInteraction{},
		)
		if err == nil || !strings.Contains(err.Error(), "Untrusted verification URI") {
			t.Fatalf("error = %v", err)
		}
	})

	for _, verificationURI := range []string{
		"http://accounts.x.ai/oauth2/device",
		"file:///etc/passwd",
		"not a url",
	} {
		t.Run(
			"rejects a non-https verification URI "+verificationURI,
			func(t *testing.T) {
				client := xaiDeviceFlowClient(
					t,
					xaiDeviceCodeResponse(map[string]any{
						"verification_uri": verificationURI,
					}),
					nil,
				)
				_, err := NewXAIOAuth(XAIOAuthOptions{
					Client: client,
				}).Login(
					context.Background(),
					&queuedProviderAuthInteraction{},
				)
				if err == nil ||
					!strings.Contains(err.Error(), "Untrusted verification URI") {
					t.Fatalf("error = %v", err)
				}
			},
		)
	}

	for _, denial := range []string{"access_denied", "authorization_denied"} {
		t.Run(
			"fails when device authorization is denied "+denial,
			func(t *testing.T) {
				clock := newOAuthTestClock(start)
				client := xaiDeviceFlowClient(
					t,
					xaiDeviceCodeResponse(map[string]any{"interval": 1}),
					func(*http.Request) (*http.Response, error) {
						return xaiJSONResponse(
							http.StatusBadRequest,
							map[string]any{"error": denial},
						), nil
					},
				)
				_, err := newTestXAIOAuth(client, clock).Login(
					context.Background(),
					&queuedProviderAuthInteraction{},
				)
				if err == nil ||
					!strings.Contains(
						err.Error(),
						"xAI device authorization was denied",
					) {
					t.Fatalf("error = %v", err)
				}
			},
		)
	}

	t.Run("cancels while waiting for the first token poll", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		clock := newOAuthTestClock(start)
		requests := 0
		client := radiusHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
			requests++
			if request.URL.String() != xaiOAuthDeviceCodeURL {
				t.Fatalf("unexpected request %s", request.URL)
			}
			return xaiJSONResponse(
				http.StatusOK,
				xaiDeviceCodeResponse(nil),
			), nil
		})
		_, err := newTestXAIOAuth(client, clock).Login(
			ctx,
			xaiCancelOnDeviceCodeInteraction{cancel: cancel},
		)
		if !errors.Is(err, context.Canceled) ||
			!strings.Contains(err.Error(), "Login cancelled") ||
			requests != 1 {
			t.Fatalf("requests=%d error=%v", requests, err)
		}
	})
}

func TestXAIOAuthRefreshPiContracts(t *testing.T) {
	now := time.Date(2026, 7, 9, 20, 0, 0, 0, time.UTC)

	t.Run("refreshes tokens and preserves an unrotated refresh token", func(t *testing.T) {
		requests := 0
		client := radiusHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.String() != xaiOAuthTokenURL {
				t.Fatalf("unexpected request %s", request.URL)
			}
			form := readXAIOAuthForm(t, request)
			if form.Get("grant_type") != "refresh_token" ||
				form.Get("client_id") != xaiOAuthClientID {
				t.Fatalf("refresh form = %v", form)
			}
			requests++
			if requests == 1 {
				if form.Get("refresh_token") != "old-refresh" {
					t.Fatalf("refresh form = %v", form)
				}
				return xaiJSONResponse(
					http.StatusOK,
					xaiTokenResponse(map[string]any{
						"access_token":  "new-access",
						"refresh_token": "new-refresh",
					}),
				), nil
			}
			if form.Get("refresh_token") != "keep-refresh" {
				t.Fatalf("refresh form = %v", form)
			}
			payload := xaiTokenResponse(map[string]any{
				"access_token": "newer-access",
			})
			delete(payload, "refresh_token")
			return xaiJSONResponse(http.StatusOK, payload), nil
		})
		auth := newXAIOAuth(
			XAIOAuthOptions{Client: client},
			xaiOAuthRuntime{now: func() time.Time { return now }},
		)
		rotated, err := auth.Refresh(context.Background(), Credential{
			Type:          CredentialTypeOAuth,
			Access:        "old-access",
			Refresh:       "old-refresh",
			Env:           ProviderEnv{"XAI_TENANT": "tenant-a"},
			EnterpriseURL: "https://enterprise.x.ai",
			Metadata:      map[string]any{"account": "account-a"},
		})
		if err != nil {
			t.Fatal(err)
		}
		preserved, err := auth.Refresh(context.Background(), Credential{
			Type:    CredentialTypeOAuth,
			Access:  "old-access",
			Refresh: "keep-refresh",
		})
		if err != nil {
			t.Fatal(err)
		}
		if rotated.Access != "new-access" ||
			rotated.Refresh != "new-refresh" ||
			rotated.Env["XAI_TENANT"] != "tenant-a" ||
			rotated.EnterpriseURL != "https://enterprise.x.ai" ||
			rotated.Metadata["account"] != "account-a" ||
			preserved.Access != "newer-access" ||
			preserved.Refresh != "keep-refresh" {
			t.Fatalf("rotated=%#v preserved=%#v", rotated, preserved)
		}
		resolved, err := auth.ToAuth(context.Background(), preserved)
		if err != nil || resolved.APIKey != "newer-access" {
			t.Fatalf("auth=%#v err=%v", resolved, err)
		}
	})

	t.Run("assumes a one-hour lifetime when expires_in is missing", func(t *testing.T) {
		payload := xaiTokenResponse(nil)
		delete(payload, "expires_in")
		auth := newXAIOAuth(
			XAIOAuthOptions{Client: xaiStaticTokenClient(t, http.StatusOK, payload)},
			xaiOAuthRuntime{now: func() time.Time { return now }},
		)
		credential, err := auth.Refresh(context.Background(), Credential{
			Type:    CredentialTypeOAuth,
			Refresh: "old-refresh",
		})
		if err != nil {
			t.Fatal(err)
		}
		want := now.Add(time.Hour - xaiOAuthRefreshSkew).UnixMilli()
		if credential.Expires != want {
			t.Fatalf("expires = %d, want %d", credential.Expires, want)
		}
	})

	t.Run("rejects token responses with missing fields", func(t *testing.T) {
		payload := xaiTokenResponse(nil)
		delete(payload, "access_token")
		auth := NewXAIOAuth(XAIOAuthOptions{
			Client: xaiStaticTokenClient(t, http.StatusOK, payload),
		})
		_, err := auth.Refresh(context.Background(), Credential{
			Type:    CredentialTypeOAuth,
			Refresh: "old-refresh",
		})
		if err == nil ||
			!strings.Contains(
				err.Error(),
				"Invalid xAI OAuth response field: access_token",
			) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("surfaces error code and description on refresh failure", func(t *testing.T) {
		auth := NewXAIOAuth(XAIOAuthOptions{
			Client: xaiStaticTokenClient(
				t,
				http.StatusBadRequest,
				map[string]any{
					"error":             "invalid_grant",
					"error_description": "refresh token revoked",
				},
			),
		})
		_, err := auth.Refresh(context.Background(), Credential{
			Type:    CredentialTypeOAuth,
			Refresh: "old-refresh",
		})
		want := "xAI OAuth token refresh failed (HTTP 400): " +
			"invalid_grant: refresh token revoked"
		if err == nil || err.Error() != want {
			t.Fatalf("error = %v, want %q", err, want)
		}
		var responseError *OAuthResponseError
		if !errors.As(err, &responseError) ||
			responseError.StatusCode != http.StatusBadRequest ||
			responseError.Code != "invalid_grant" {
			t.Fatalf("typed error = %#v", responseError)
		}
	})
}

func TestXAIOAuthExpiryRejectsOverflow(t *testing.T) {
	maxInt64 := int64(^uint64(0) >> 1)
	if _, err := xaiOAuthExpiryMillis(
		time.UnixMilli(maxInt64-1),
		3600,
	); err == nil {
		t.Fatal("expected wall-clock overflow error")
	}
	if _, err := xaiOAuthExpiryMillis(
		time.UnixMilli(0),
		float64(1<<63)/1000,
	); err == nil {
		t.Fatal("expected lifetime overflow error")
	}
}

func TestBuiltinXAIProviderUsesDefaultOAuth(t *testing.T) {
	provider, err := NewBuiltinProvider("xai")
	if err != nil {
		t.Fatal(err)
	}
	if provider.Auth.OAuth == nil ||
		provider.Auth.OAuth.Name != "xAI (Grok/X subscription)" ||
		provider.Auth.OAuth.LoginLabel !=
			"Sign in with SuperGrok or X Premium" {
		t.Fatalf("OAuth = %#v", provider.Auth.OAuth)
	}
	resolved, err := provider.Auth.OAuth.ToAuth(
		context.Background(),
		Credential{Type: CredentialTypeOAuth, Access: "built-in-token"},
	)
	if err != nil || resolved.APIKey != "built-in-token" {
		t.Fatalf("auth = %#v, error = %v", resolved, err)
	}
}

func newTestXAIOAuth(
	client HTTPDoer,
	clock *oauthTestClock,
) *OAuthAuth {
	return newXAIOAuth(
		XAIOAuthOptions{Client: client},
		xaiOAuthRuntime{
			now: clock.now,
			pollDeviceCode: func(
				ctx context.Context,
				options OAuthDeviceCodePollOptions[Credential],
			) (Credential, error) {
				return pollOAuthDeviceCodeFlow(ctx, options, clock.runtime())
			},
		},
	)
}

func xaiDeviceFlowClient(
	t *testing.T,
	device map[string]any,
	token func(*http.Request) (*http.Response, error),
) HTTPDoer {
	t.Helper()
	return radiusHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case xaiOAuthDeviceCodeURL:
			return xaiJSONResponse(http.StatusOK, device), nil
		case xaiOAuthTokenURL:
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

func xaiStaticTokenClient(
	t *testing.T,
	status int,
	payload map[string]any,
) HTTPDoer {
	t.Helper()
	return radiusHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != xaiOAuthTokenURL {
			t.Fatalf("unexpected request %s", request.URL)
		}
		return xaiJSONResponse(status, payload), nil
	})
}

func xaiDeviceCodeResponse(overrides map[string]any) map[string]any {
	payload := map[string]any{
		"device_code":      "device-code",
		"user_code":        "ABCD-1234",
		"verification_uri": "https://accounts.x.ai/oauth2/device",
		"expires_in":       900,
		"interval":         5,
	}
	for key, value := range overrides {
		payload[key] = value
	}
	return payload
}

func xaiTokenResponse(overrides map[string]any) map[string]any {
	payload := map[string]any{
		"access_token":  "access-token",
		"refresh_token": "refresh-token",
		"expires_in":    21600,
		"token_type":    "Bearer",
	}
	for key, value := range overrides {
		payload[key] = value
	}
	return payload
}

func xaiJSONResponse(status int, value any) *http.Response {
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

func readXAIOAuthForm(t *testing.T, request *http.Request) url.Values {
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

type xaiCancelOnDeviceCodeInteraction struct {
	cancel context.CancelFunc
}

func (i xaiCancelOnDeviceCodeInteraction) Prompt(
	context.Context,
	AuthPrompt,
) (string, error) {
	return "", errors.New("unexpected prompt")
}

func (i xaiCancelOnDeviceCodeInteraction) Notify(event AuthEvent) {
	if event.Type == AuthEventDeviceCode {
		i.cancel()
	}
}
