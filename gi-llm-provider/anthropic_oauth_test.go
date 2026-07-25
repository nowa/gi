package gillmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAnthropicOAuthManualLoginDataFlow(t *testing.T) {
	now := time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)
	var requestBody map[string]any
	client := radiusHTTPDoerFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		if request.URL.String() != AnthropicOAuthTokenURL ||
			request.Method != http.MethodPost ||
			request.Header.Get("content-type") != "application/json" ||
			request.Header.Get("accept") != "application/json" {
			t.Fatalf("request = %#v", request)
		}
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		return anthropicOAuthJSONResponse(
			http.StatusOK,
			`{
				"access_token":"anthropic-access",
				"refresh_token":"anthropic-refresh",
				"expires_in":3600
			}`,
		), nil
	})
	callback := &stubOAuthAuthorizationCodeServer{
		wait: func(ctx context.Context) (
			oauthAuthorizationCode,
			error,
		) {
			<-ctx.Done()
			return oauthAuthorizationCode{}, oauthLoginContextError(ctx)
		},
	}
	var events []AuthEvent
	interaction := oauthProviderTestInteraction{
		prompt: func(
			_ context.Context,
			prompt AuthPrompt,
		) (string, error) {
			if prompt.Type != AuthPromptManualCode ||
				prompt.Placeholder != anthropicOAuthRedirectURI {
				t.Fatalf("prompt = %#v", prompt)
			}
			var authorizeURL string
			for _, event := range events {
				if event.Type == AuthEventURL {
					authorizeURL = event.URL
				}
			}
			parsed, err := url.Parse(authorizeURL)
			if err != nil {
				return "", err
			}
			return anthropicOAuthRedirectURI +
				"?code=manual-code&state=" +
				url.QueryEscape(parsed.Query().Get("state")), nil
		},
		notify: func(event AuthEvent) {
			events = append(events, event)
		},
	}
	auth := newAnthropicOAuth(
		AnthropicOAuthOptions{
			Client:       client,
			CallbackHost: "callback.test",
		},
		anthropicOAuthRuntime{
			now: func() time.Time { return now },
			generatePKCE: func() (PKCE, error) {
				return PKCE{
					Verifier:  "verifier",
					Challenge: "challenge",
				}, nil
			},
			startCallback: func(
				_ context.Context,
				options oauthLoopbackCallbackOptions,
			) (oauthAuthorizationCodeServer, error) {
				if options.Host != "callback.test" ||
					options.Port != anthropicOAuthCallbackPort ||
					options.Path != anthropicOAuthCallbackPath ||
					options.ExpectedState != "verifier" {
					t.Fatalf("callback options = %#v", options)
				}
				return callback, nil
			},
		},
	)

	credential, err := auth.Login(context.Background(), interaction)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Type != CredentialTypeOAuth ||
		credential.Access != "anthropic-access" ||
		credential.Refresh != "anthropic-refresh" ||
		credential.Expires != now.
			Add(time.Hour).
			Add(-anthropicOAuthRefreshSkew).
			UnixMilli() {
		t.Fatalf("credential = %#v", credential)
	}
	for key, want := range map[string]any{
		"grant_type":    "authorization_code",
		"client_id":     AnthropicOAuthClientID,
		"code":          "manual-code",
		"state":         "verifier",
		"redirect_uri":  anthropicOAuthRedirectURI,
		"code_verifier": "verifier",
	} {
		if requestBody[key] != want {
			t.Fatalf("%s = %#v, want %#v", key, requestBody[key], want)
		}
	}
	if len(events) != 2 ||
		events[0].Type != AuthEventURL ||
		events[1].Type != AuthEventProgress {
		t.Fatalf("events = %#v", events)
	}
	authorizeURL, err := url.Parse(events[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"code":                  "true",
		"client_id":             AnthropicOAuthClientID,
		"response_type":         "code",
		"redirect_uri":          anthropicOAuthRedirectURI,
		"scope":                 anthropicOAuthScopes,
		"code_challenge":        "challenge",
		"code_challenge_method": "S256",
		"state":                 "verifier",
	} {
		if got := authorizeURL.Query().Get(key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
	if !callback.closed {
		t.Fatal("callback server was not closed")
	}
}

func TestAnthropicOAuthCallbackWinsAndCancelsManualPrompt(t *testing.T) {
	promptStarted := make(chan struct{})
	promptCancelled := make(chan struct{})
	callback := &stubOAuthAuthorizationCodeServer{
		wait: func(ctx context.Context) (
			oauthAuthorizationCode,
			error,
		) {
			<-promptStarted
			return oauthAuthorizationCode{
				Code:  "callback-code",
				State: "verifier",
			}, nil
		},
	}
	client := radiusHTTPDoerFunc(func(
		*http.Request,
	) (*http.Response, error) {
		return anthropicOAuthJSONResponse(
			http.StatusOK,
			`{
				"access_token":"access",
				"refresh_token":"refresh",
				"expires_in":3600
			}`,
		), nil
	})
	auth := newAnthropicOAuth(
		AnthropicOAuthOptions{Client: client},
		anthropicOAuthRuntime{
			now: func() time.Time { return time.Unix(1000, 0) },
			generatePKCE: func() (PKCE, error) {
				return PKCE{
					Verifier:  "verifier",
					Challenge: "challenge",
				}, nil
			},
			startCallback: func(
				context.Context,
				oauthLoopbackCallbackOptions,
			) (oauthAuthorizationCodeServer, error) {
				return callback, nil
			},
		},
	)
	interaction := oauthProviderTestInteraction{
		prompt: func(
			ctx context.Context,
			_ AuthPrompt,
		) (string, error) {
			close(promptStarted)
			<-ctx.Done()
			close(promptCancelled)
			return "", ctx.Err()
		},
	}

	credential, err := auth.Login(context.Background(), interaction)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Access != "access" {
		t.Fatalf("credential = %#v", credential)
	}
	select {
	case <-promptCancelled:
	case <-time.After(time.Second):
		t.Fatal("manual prompt context was not cancelled")
	}
}

func TestAnthropicOAuthRefreshPreservesCredentialState(t *testing.T) {
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	var body map[string]any
	client := radiusHTTPDoerFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return anthropicOAuthJSONResponse(
			http.StatusOK,
			`{
				"access_token":"new-access",
				"refresh_token":"new-refresh",
				"expires_in":7200
			}`,
		), nil
	})
	auth := newAnthropicOAuth(
		AnthropicOAuthOptions{Client: client},
		anthropicOAuthRuntime{now: func() time.Time { return now }},
	)
	previous := Credential{
		Type:    CredentialTypeOAuth,
		Access:  "old-access",
		Refresh: "old-refresh",
		Env:     ProviderEnv{"tenant": "test"},
		Metadata: map[string]any{
			"preserved": true,
		},
	}

	refreshed, err := auth.Refresh(context.Background(), previous)
	if err != nil {
		t.Fatal(err)
	}
	if body["grant_type"] != "refresh_token" ||
		body["client_id"] != AnthropicOAuthClientID ||
		body["refresh_token"] != "old-refresh" {
		t.Fatalf("refresh body = %#v", body)
	}
	if _, exists := body["scope"]; exists {
		t.Fatalf("refresh body unexpectedly includes scope: %#v", body)
	}
	if refreshed.Access != "new-access" ||
		refreshed.Refresh != "new-refresh" ||
		refreshed.Expires != now.
			Add(2*time.Hour).
			Add(-anthropicOAuthRefreshSkew).
			UnixMilli() ||
		refreshed.Env["tenant"] != "test" ||
		refreshed.Metadata["preserved"] != true {
		t.Fatalf("refreshed = %#v", refreshed)
	}
	modelAuth, err := auth.ToAuth(context.Background(), refreshed)
	if err != nil {
		t.Fatal(err)
	}
	if modelAuth.APIKey != "new-access" {
		t.Fatalf("model auth = %#v", modelAuth)
	}
}

func TestAnthropicOAuthRejectsManualStateMismatch(t *testing.T) {
	client := radiusHTTPDoerFunc(func(
		*http.Request,
	) (*http.Response, error) {
		t.Fatal("token endpoint should not be called")
		return nil, nil
	})
	auth := newAnthropicOAuth(
		AnthropicOAuthOptions{Client: client},
		anthropicOAuthRuntime{
			generatePKCE: func() (PKCE, error) {
				return PKCE{
					Verifier:  "expected",
					Challenge: "challenge",
				}, nil
			},
			startCallback: func(
				context.Context,
				oauthLoopbackCallbackOptions,
			) (oauthAuthorizationCodeServer, error) {
				return &stubOAuthAuthorizationCodeServer{
					wait: func(ctx context.Context) (
						oauthAuthorizationCode,
						error,
					) {
						<-ctx.Done()
						return oauthAuthorizationCode{}, ctx.Err()
					},
				}, nil
			},
		},
	)
	_, err := auth.Login(
		context.Background(),
		oauthProviderTestInteraction{
			prompt: func(context.Context, AuthPrompt) (string, error) {
				return "code#wrong", nil
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("error = %v", err)
	}
}

func TestAnthropicOAuthCallbackHandlerRejectsInvalidStateWithoutSettling(
	t *testing.T,
) {
	server := &localOAuthAuthorizationCodeServer{
		result: make(chan oauthAuthorizationCodeResult, 1),
	}
	options := oauthLoopbackCallbackOptions{
		Path:          anthropicOAuthCallbackPath,
		ExpectedState: "expected",
		ProviderName:  "Anthropic",
	}
	request := &http.Request{
		Method: http.MethodGet,
		URL: &url.URL{
			Path:     anthropicOAuthCallbackPath,
			RawQuery: "code=code&state=wrong",
		},
	}
	response := newOAuthResponseRecorder()
	server.handler(options).ServeHTTP(response, request)
	if response.status != http.StatusBadRequest ||
		!strings.Contains(response.body.String(), "State mismatch") {
		t.Fatalf(
			"response = status %d body %q",
			response.status,
			response.body.String(),
		)
	}
	if len(server.result) != 0 {
		t.Fatal("invalid callback settled the authorization flow")
	}

	validRequest := &http.Request{
		Method: http.MethodGet,
		URL: &url.URL{
			Path:     anthropicOAuthCallbackPath,
			RawQuery: "code=valid-code&state=expected",
		},
	}
	validResponse := newOAuthResponseRecorder()
	server.handler(options).ServeHTTP(validResponse, validRequest)
	if validResponse.status != http.StatusOK {
		t.Fatalf("valid callback status = %d", validResponse.status)
	}
	result := <-server.result
	if result.err != nil ||
		result.authorization.Code != "valid-code" ||
		result.authorization.State != "expected" {
		t.Fatalf("valid callback result = %#v", result)
	}
}

type oauthProviderTestInteraction struct {
	prompt func(context.Context, AuthPrompt) (string, error)
	notify func(AuthEvent)
}

func (i oauthProviderTestInteraction) Prompt(
	ctx context.Context,
	prompt AuthPrompt,
) (string, error) {
	if i.prompt == nil {
		return "", errors.New("unexpected auth prompt")
	}
	return i.prompt(ctx, prompt)
}

func (i oauthProviderTestInteraction) Notify(event AuthEvent) {
	if i.notify != nil {
		i.notify(event)
	}
}

type stubOAuthAuthorizationCodeServer struct {
	mu     sync.Mutex
	wait   func(context.Context) (oauthAuthorizationCode, error)
	closed bool
}

func (s *stubOAuthAuthorizationCodeServer) Wait(
	ctx context.Context,
) (oauthAuthorizationCode, error) {
	if s.wait == nil {
		<-ctx.Done()
		return oauthAuthorizationCode{}, ctx.Err()
	}
	return s.wait(ctx)
}

func (s *stubOAuthAuthorizationCodeServer) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}

func anthropicOAuthJSONResponse(
	status int,
	body string,
) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type oauthResponseRecorder struct {
	header http.Header
	status int
	body   strings.Builder
}

func newOAuthResponseRecorder() *oauthResponseRecorder {
	return &oauthResponseRecorder{header: make(http.Header)}
}

func (r *oauthResponseRecorder) Header() http.Header {
	return r.header
}

func (r *oauthResponseRecorder) WriteHeader(status int) {
	r.status = status
}

func (r *oauthResponseRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(data)
}
