package gillmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRadiusOAuthBrowserLogin(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	callback := &fakeRadiusOAuthCallbackServer{
		redirectURI: "http://127.0.0.1:9999/oauth/callback",
		code:        "authorization-code",
	}
	var tokenForm url.Values
	var requestPaths []string
	client := radiusHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		requestPaths = append(requestPaths, request.URL.Path)
		switch request.URL.Path {
		case "/v1/oauth":
			return radiusOAuthJSONResponse(
				http.StatusOK,
				radiusOAuthTestConfig(),
			), nil
		case "/v1/oauth/token":
			tokenForm = readRadiusOAuthForm(t, request)
			return radiusOAuthJSONResponse(http.StatusOK, map[string]any{
				"access_token":  "access-token",
				"refresh_token": "refresh-token",
				"expires_in":    3600,
				"scope":         "models:read messages:write",
			}), nil
		default:
			t.Fatalf("unexpected request %s", request.URL)
			return nil, nil
		}
	})
	auth, err := newRadiusOAuth(
		RadiusOAuthOptions{
			Name:    "Radius Dev",
			Gateway: "https://radius.test",
			Client:  client,
		},
		radiusOAuthRuntime{
			now: func() time.Time { return now },
			generatePKCE: func() (PKCE, error) {
				return PKCE{Verifier: "verifier", Challenge: "challenge"}, nil
			},
			randomState: func() (string, error) { return "state", nil },
			startCallback: func(
				context.Context,
				string,
			) (radiusOAuthCallbackServer, error) {
				return callback, nil
			},
			pollDeviceCode: PollOAuthDeviceCodeFlow[Credential],
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	interaction := &queuedProviderAuthInteraction{
		answers: []string{radiusLoginMethodBrowser},
	}
	credential, err := auth.Login(context.Background(), interaction)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Type != CredentialTypeOAuth ||
		credential.Access != "access-token" ||
		credential.Refresh != "refresh-token" ||
		credential.Expires != now.Add(59*time.Minute).UnixMilli() ||
		credential.Metadata["scope"] != "models:read messages:write" {
		t.Fatalf("credential = %#v", credential)
	}
	if callback.closed != 1 {
		t.Fatalf("callback close count = %d", callback.closed)
	}
	if tokenForm.Get("grant_type") != "authorization_code" ||
		tokenForm.Get("client_id") != radiusOAuthClientID ||
		tokenForm.Get("redirect_uri") != callback.redirectURI ||
		tokenForm.Get("code") != "authorization-code" ||
		tokenForm.Get("code_verifier") != "verifier" {
		t.Fatalf("token form = %v", tokenForm)
	}
	if len(interaction.prompts) != 1 ||
		len(interaction.prompts[0].Options) != 2 ||
		interaction.prompts[0].Options[0].ID != radiusLoginMethodBrowser {
		t.Fatalf("prompts = %#v", interaction.prompts)
	}
	if len(interaction.events) != 2 ||
		interaction.events[0].Type != AuthEventProgress ||
		interaction.events[1].Type != AuthEventURL {
		t.Fatalf("events = %#v", interaction.events)
	}
	if !reflect.DeepEqual(requestPaths, []string{
		"/v1/oauth",
		"/v1/oauth/token",
	}) {
		t.Fatalf("browser request paths = %#v", requestPaths)
	}
	authorizeURL, err := url.Parse(interaction.events[1].URL)
	if err != nil {
		t.Fatal(err)
	}
	query := authorizeURL.Query()
	if authorizeURL.String() == "" ||
		query.Get("client_id") != radiusOAuthClientID ||
		query.Get("redirect_uri") != callback.redirectURI ||
		query.Get("scope") != radiusOAuthScope ||
		query.Get("code_challenge") != "challenge" ||
		query.Get("code_challenge_method") != "S256" ||
		query.Get("handoff") != "url" ||
		query.Get("state") != "state" {
		t.Fatalf("authorization URL = %s", authorizeURL)
	}
}

func TestRadiusOAuthDeviceCodeLogin(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	clock := newOAuthTestClock(now)
	tokenPolls := 0
	var deviceForm url.Values
	var requestPaths []string
	client := radiusHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		requestPaths = append(requestPaths, request.URL.Path)
		switch request.URL.Path {
		case "/v1/oauth/device":
			deviceForm = readRadiusOAuthForm(t, request)
			return radiusOAuthJSONResponse(http.StatusOK, map[string]any{
				"device_code":      "device-code",
				"user_code":        "ABCD-1234",
				"verification_uri": "https://radius.test/verify",
				"expires_in":       900,
				"interval":         2,
			}), nil
		case "/v1/oauth/token":
			tokenPolls++
			form := readRadiusOAuthForm(t, request)
			if form.Get("grant_type") != radiusOAuthDeviceGrantType ||
				form.Get("client_id") != radiusOAuthClientID ||
				form.Get("device_code") != "device-code" {
				t.Fatalf("token form = %v", form)
			}
			if tokenPolls == 1 {
				return radiusOAuthJSONResponse(
					http.StatusBadRequest,
					map[string]any{"error": "authorization_pending"},
				), nil
			}
			return radiusOAuthJSONResponse(http.StatusOK, map[string]any{
				"access_token":  "device-access",
				"refresh_token": "device-refresh",
				"expires_in":    3600,
			}), nil
		default:
			t.Fatalf("unexpected request %s", request.URL)
			return nil, nil
		}
	})
	auth, err := newRadiusOAuth(
		RadiusOAuthOptions{
			Gateway: "https://radius.test",
			Client:  client,
		},
		radiusOAuthRuntime{
			now:          clock.now,
			generatePKCE: GeneratePKCE,
			randomState:  func() (string, error) { return "state", nil },
			startCallback: func(
				context.Context,
				string,
			) (radiusOAuthCallbackServer, error) {
				return nil, errors.New("browser callback must not start")
			},
			pollDeviceCode: func(
				ctx context.Context,
				options OAuthDeviceCodePollOptions[Credential],
			) (Credential, error) {
				return pollOAuthDeviceCodeFlow(ctx, options, clock.runtime())
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	interaction := &queuedProviderAuthInteraction{
		answers: []string{radiusLoginMethodDeviceCode},
	}
	credential, err := auth.Login(context.Background(), interaction)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Access != "device-access" ||
		credential.Refresh != "device-refresh" ||
		tokenPolls != 2 {
		t.Fatalf("credential=%#v polls=%d", credential, tokenPolls)
	}
	if deviceForm.Get("client_id") != radiusOAuthClientID ||
		deviceForm.Get("scope") != radiusOAuthScope {
		t.Fatalf("device form = %v", deviceForm)
	}
	if !reflect.DeepEqual(requestPaths, []string{
		"/v1/oauth/device",
		"/v1/oauth/token",
		"/v1/oauth/token",
	}) {
		t.Fatalf("device request paths = %#v", requestPaths)
	}
	if len(interaction.events) != 1 {
		t.Fatalf("events = %#v", interaction.events)
	}
	event := interaction.events[0]
	if event.Type != AuthEventDeviceCode ||
		event.UserCode != "ABCD-1234" ||
		event.VerificationURI != "https://radius.test/verify" ||
		event.IntervalSeconds != 2 ||
		event.ExpiresInSeconds != 900 {
		t.Fatalf("device event = %#v", event)
	}
}

func TestRadiusOAuthRefreshPreservesProviderMetadata(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	var tokenForm url.Values
	var requestPaths []string
	client := radiusHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		requestPaths = append(requestPaths, request.URL.Path)
		switch request.URL.Path {
		case "/v1/oauth/token":
			tokenForm = readRadiusOAuthForm(t, request)
			return radiusOAuthJSONResponse(http.StatusOK, map[string]any{
				"access_token": "new-access",
				"expires_in":   1800,
				"scope":        "new-scope",
			}), nil
		default:
			t.Fatalf("unexpected request %s", request.URL)
			return nil, nil
		}
	})
	auth, err := newRadiusOAuth(
		RadiusOAuthOptions{
			Gateway: "https://radius.test",
			Client:  client,
		},
		radiusOAuthRuntime{now: func() time.Time { return now }},
	)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := auth.Refresh(context.Background(), Credential{
		Type:          CredentialTypeOAuth,
		Access:        "old-access",
		Refresh:       "old-refresh",
		Env:           ProviderEnv{"RADIUS_TENANT": "tenant-a"},
		EnterpriseURL: "https://enterprise.radius.test",
		Metadata: map[string]any{
			"gatewayConfig": map[string]any{"baseUrl": "https://radius.test/v1"},
			"scope":         "old-scope",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if credential.Access != "new-access" ||
		credential.Refresh != "old-refresh" ||
		credential.Expires != now.Add(29*time.Minute).UnixMilli() ||
		credential.Env["RADIUS_TENANT"] != "tenant-a" ||
		credential.EnterpriseURL != "https://enterprise.radius.test" ||
		credential.Metadata["scope"] != "new-scope" ||
		credential.Metadata["gatewayConfig"] == nil {
		t.Fatalf("credential = %#v", credential)
	}
	if tokenForm.Get("grant_type") != "refresh_token" ||
		tokenForm.Get("client_id") != radiusOAuthClientID ||
		tokenForm.Get("refresh_token") != "old-refresh" {
		t.Fatalf("token form = %v", tokenForm)
	}
	if !reflect.DeepEqual(requestPaths, []string{"/v1/oauth/token"}) {
		t.Fatalf("refresh request paths = %#v", requestPaths)
	}
	resolved, err := auth.ToAuth(context.Background(), credential)
	if err != nil || resolved.APIKey != "new-access" {
		t.Fatalf("auth=%#v err=%v", resolved, err)
	}
}

func TestRadiusOAuthResponseErrorsRemainTyped(t *testing.T) {
	client := radiusHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/v1/oauth/token":
			return radiusOAuthJSONResponse(
				http.StatusBadRequest,
				map[string]any{
					"error":             "invalid_grant",
					"error_description": "refresh token expired",
				},
			), nil
		default:
			t.Fatalf("unexpected request %s", request.URL)
			return nil, nil
		}
	})
	auth, err := NewRadiusOAuth(RadiusOAuthOptions{
		Gateway: "https://radius.test",
		Client:  client,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = auth.Refresh(context.Background(), Credential{
		Type:    CredentialTypeOAuth,
		Refresh: "expired",
	})
	var responseError *OAuthResponseError
	if !errors.As(err, &responseError) ||
		responseError.StatusCode != http.StatusBadRequest ||
		responseError.Code != "invalid_grant" ||
		responseError.Description != "refresh token expired" {
		t.Fatalf("error = %#v", err)
	}
}

func TestRadiusProviderUsesBuiltinOAuthWithoutLoader(t *testing.T) {
	client := radiusHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/v1/oauth/device":
			return radiusOAuthJSONResponse(http.StatusOK, map[string]any{
				"device_code":      "device-code",
				"user_code":        "ABCD-1234",
				"verification_uri": "https://radius.test/verify",
				"expires_in":       60,
			}), nil
		case "/v1/oauth/token":
			return radiusOAuthJSONResponse(http.StatusOK, map[string]any{
				"access_token":  "access",
				"refresh_token": "refresh",
				"expires_in":    3600,
			}), nil
		default:
			t.Fatalf("unexpected request %s", request.URL)
			return nil, nil
		}
	})
	provider, err := NewRadiusProvider(RadiusProviderOptions{
		Gateway: "https://radius.test",
		Client:  client,
	})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := provider.Auth.OAuth.Login(
		context.Background(),
		&queuedProviderAuthInteraction{
			answers: []string{radiusLoginMethodDeviceCode},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Access != "access" || credential.Refresh != "refresh" {
		t.Fatalf("credential = %#v", credential)
	}
}

func TestRadiusProviderExplicitOAuthLoaderOverridesBuiltin(t *testing.T) {
	loads := 0
	provider, err := NewRadiusProvider(RadiusProviderOptions{
		Gateway: "https://radius.test",
		OAuthLoader: func(context.Context) (*OAuthAuth, error) {
			loads++
			return &OAuthAuth{
				Name: "Custom Radius",
				Login: func(
					context.Context,
					AuthInteraction,
				) (Credential, error) {
					return Credential{
						Type:   CredentialTypeOAuth,
						Access: "custom-access",
					}, nil
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := provider.Auth.OAuth.Login(
		context.Background(),
		&queuedProviderAuthInteraction{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if loads != 1 || credential.Access != "custom-access" {
		t.Fatalf("loads=%d credential=%#v", loads, credential)
	}
	resolved, err := provider.Auth.OAuth.ToAuth(
		context.Background(),
		credential,
	)
	if err != nil || resolved.APIKey != "custom-access" {
		t.Fatalf("auth=%#v err=%v", resolved, err)
	}
}

func TestRadiusProviderRegisteredOAuthLoaderOverridesBuiltin(t *testing.T) {
	loads := 0
	RegisterOAuthAuthLoader(
		"radius",
		func(context.Context) (*OAuthAuth, error) {
			loads++
			return &OAuthAuth{
				Name: "Registered Radius",
				Login: func(
					context.Context,
					AuthInteraction,
				) (Credential, error) {
					return Credential{
						Type:   CredentialTypeOAuth,
						Access: "registered-access",
					}, nil
				},
			}, nil
		},
	)
	t.Cleanup(func() {
		UnregisterOAuthAuthLoader("radius")
	})

	provider, err := NewRadiusProvider(RadiusProviderOptions{
		Gateway: "https://radius.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := provider.Auth.OAuth.Login(
		context.Background(),
		&queuedProviderAuthInteraction{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if loads != 1 || credential.Access != "registered-access" {
		t.Fatalf("loads=%d credential=%#v", loads, credential)
	}
}

func TestRadiusOAuthExpiryRejectsOverflow(t *testing.T) {
	maxInt64 := int64(^uint64(0) >> 1)
	_, err := radiusOAuthExpiryMillis(
		time.UnixMilli(maxInt64-1000),
		3600,
	)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("error = %v", err)
	}
}

func TestRadiusOAuthCallbackHandler(t *testing.T) {
	local := &localRadiusOAuthCallbackServer{
		redirectURI: "http://127.0.0.1:9999/oauth/callback",
		result:      make(chan radiusOAuthCallbackResult, 1),
	}
	handler := local.handler("expected-state")

	mismatch := httptest.NewRecorder()
	handler.ServeHTTP(
		mismatch,
		httptest.NewRequest(
			http.MethodGet,
			"http://127.0.0.1:9999/oauth/callback?code=bad&state=wrong",
			nil,
		),
	)
	if mismatch.Code != http.StatusBadRequest ||
		!strings.Contains(mismatch.Body.String(), "OAuth state mismatch") {
		t.Fatalf("mismatch response = %d %q", mismatch.Code, mismatch.Body.String())
	}
	select {
	case result := <-local.result:
		t.Fatalf("state mismatch settled callback: %#v", result)
	default:
	}

	success := httptest.NewRecorder()
	handler.ServeHTTP(
		success,
		httptest.NewRequest(
			http.MethodGet,
			"http://127.0.0.1:9999/oauth/callback?code=good&state=expected-state",
			nil,
		),
	)
	if success.Code != http.StatusOK ||
		!strings.Contains(success.Body.String(), "Signed in to Radius") {
		t.Fatalf("success response = %d %q", success.Code, success.Body.String())
	}
	code, err := local.WaitForCode(context.Background())
	if err != nil || code != "good" {
		t.Fatalf("code=%q err=%v", code, err)
	}
}

func TestRadiusOAuthCallbackServerAcceptsBrowserRedirect(t *testing.T) {
	server, err := startRadiusOAuthCallbackServer(
		context.Background(),
		"expected-state",
		"127.0.0.1:0",
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Errorf("close callback server: %v", err)
		}
	})

	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(
		server.RedirectURI() + "?code=browser-code&state=expected-state",
	)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK ||
		!strings.Contains(string(body), "Signed in to Radius") {
		t.Fatalf("callback response = %d %q", response.StatusCode, body)
	}
	code, err := server.WaitForCode(context.Background())
	if err != nil || code != "browser-code" {
		t.Fatalf("code=%q err=%v", code, err)
	}
}

func TestRadiusOAuthConfigValidation(t *testing.T) {
	_, err := NewRadiusOAuth(RadiusOAuthOptions{Gateway: "file:///tmp/radius"})
	if err == nil || !strings.Contains(err.Error(), "invalid Radius gateway URL") {
		t.Fatalf("error = %v", err)
	}

	client := radiusHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		return radiusOAuthJSONResponse(http.StatusOK, map[string]any{}), nil
	})
	auth, err := NewRadiusOAuth(RadiusOAuthOptions{
		Gateway: "https://radius.test",
		Client:  client,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = auth.Login(
		context.Background(),
		&queuedProviderAuthInteraction{answers: []string{radiusLoginMethodBrowser}},
	)
	if err == nil || !strings.Contains(err.Error(), "invalid Radius OAuth config") {
		t.Fatalf("error = %v", err)
	}
}

func radiusOAuthTestConfig() map[string]any {
	return map[string]any{
		"authorizationEndpoint": "https://radius.test/authorize",
	}
}

func radiusOAuthJSONResponse(status int, value any) *http.Response {
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

func readRadiusOAuthForm(t *testing.T, request *http.Request) url.Values {
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

type fakeRadiusOAuthCallbackServer struct {
	redirectURI string
	code        string
	err         error
	closed      int
}

func (s *fakeRadiusOAuthCallbackServer) RedirectURI() string {
	return s.redirectURI
}

func (s *fakeRadiusOAuthCallbackServer) WaitForCode(
	context.Context,
) (string, error) {
	return s.code, s.err
}

func (s *fakeRadiusOAuthCallbackServer) Close() error {
	s.closed++
	return nil
}
