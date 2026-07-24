package gillmprovider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenRouterOAuthRunsOneShotLoopbackPKCE(t *testing.T) {
	var exchangeBody map[string]any
	var exchangeCalls atomic.Int32
	client := radiusHTTPDoerFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		exchangeCalls.Add(1)
		if request.URL.String() != openRouterOAuthTokenURL ||
			request.Method != http.MethodPost ||
			request.Header.Get("accept") != "application/json" ||
			request.Header.Get("content-type") != "application/json" {
			t.Fatalf("exchange request = %#v", request)
		}
		if err := json.NewDecoder(request.Body).Decode(&exchangeBody); err != nil {
			t.Fatal(err)
		}
		return openRouterOAuthJSONResponse(
			http.StatusOK,
			map[string]any{"key": "sk-or-test"},
		), nil
	})

	var (
		authorizeURL     *url.URL
		callbackResponse *http.Response
		events           []AuthEvent
	)
	credential, err := NewOpenRouterOAuth(OpenRouterOAuthOptions{
		Client: client,
	}).Login(
		context.Background(),
		openRouterAuthInteraction{
			notify: func(event AuthEvent) {
				events = append(events, event)
				if event.Type != AuthEventURL {
					return
				}
				parsedAuthorizeURL, parseErr := url.Parse(event.URL)
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				authorizeURL = parsedAuthorizeURL
				callbackURL, parseErr := url.Parse(
					authorizeURL.Query().Get("callback_url"),
				)
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				query := callbackURL.Query()
				query.Set("code", "authorization-code")
				callbackURL.RawQuery = query.Encode()
				response, getErr := (&http.Client{
					Timeout: 2 * time.Second,
				}).Get(callbackURL.String())
				if getErr != nil {
					t.Fatal(getErr)
				}
				callbackResponse = response
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Type != CredentialTypeOAuth ||
		credential.Access != "sk-or-test" ||
		credential.Refresh != "" ||
		credential.Expires != permanentOAuthCredentialExpires {
		t.Fatalf("credential = %#v", credential)
	}
	if callbackResponse == nil {
		t.Fatal("callback response was not captured")
	}
	callbackBody := readOpenRouterOAuthResponseBody(t, callbackResponse)
	if callbackResponse.StatusCode != http.StatusOK ||
		!strings.Contains(callbackBody, "Signed in to OpenRouter") {
		t.Fatalf(
			"callback response = %d %q",
			callbackResponse.StatusCode,
			callbackBody,
		)
	}
	if authorizeURL == nil ||
		authorizeURL.Scheme != "https" ||
		authorizeURL.Host != "openrouter.ai" ||
		authorizeURL.Path != "/auth" ||
		authorizeURL.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("authorize URL = %v", authorizeURL)
	}
	callbackURL, err := url.Parse(
		authorizeURL.Query().Get("callback_url"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if callbackURL.Hostname() != defaultOpenRouterOAuthCallbackHost ||
		!regexp.MustCompile(
			`^/oauth/callback/[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
		).MatchString(callbackURL.Path) {
		t.Fatalf("callback URL = %s", callbackURL)
	}
	if exchangeBody["code"] != "authorization-code" ||
		exchangeBody["code_challenge_method"] != "S256" {
		t.Fatalf("exchange body = %#v", exchangeBody)
	}
	verifier, ok := exchangeBody["code_verifier"].(string)
	if !ok || verifier == "" {
		t.Fatalf("exchange verifier = %#v", exchangeBody["code_verifier"])
	}
	digest := sha256.Sum256([]byte(verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(digest[:])
	if authorizeURL.Query().Get("code_challenge") != wantChallenge {
		t.Fatalf(
			"challenge = %q, want %q",
			authorizeURL.Query().Get("code_challenge"),
			wantChallenge,
		)
	}
	if exchangeCalls.Load() != 1 {
		t.Fatalf("exchange calls = %d", exchangeCalls.Load())
	}
	if len(events) != 2 ||
		events[0].Type != AuthEventProgress ||
		events[1].Type != AuthEventURL ||
		events[1].Instructions != "Complete sign-in in your browser." {
		t.Fatalf("events = %#v", events)
	}
}

func TestOpenRouterOAuthReportsExchangeFailureToPageAndLogin(t *testing.T) {
	client := radiusHTTPDoerFunc(func(
		*http.Request,
	) (*http.Response, error) {
		return openRouterOAuthJSONResponse(
			http.StatusForbidden,
			map[string]any{
				"error": map[string]any{"message": "invalid code"},
			},
		), nil
	})
	var callbackResponse *http.Response
	_, err := NewOpenRouterOAuth(OpenRouterOAuthOptions{
		Client: client,
	}).Login(
		context.Background(),
		openRouterAuthInteraction{
			notify: func(event AuthEvent) {
				if event.Type != AuthEventURL {
					return
				}
				callbackURL := openRouterCallbackURLFromEvent(t, event)
				query := callbackURL.Query()
				query.Set("code", "bad-code")
				callbackURL.RawQuery = query.Encode()
				var callbackErr error
				callbackResponse, callbackErr = (&http.Client{
					Timeout: 2 * time.Second,
				}).Get(callbackURL.String())
				if callbackErr != nil {
					t.Fatal(callbackErr)
				}
			},
		},
	)
	want := "OpenRouter OAuth key exchange failed (HTTP 403): invalid code"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
	var responseError *OAuthResponseError
	if !errors.As(err, &responseError) ||
		responseError.StatusCode != http.StatusForbidden ||
		responseError.Description != "invalid code" {
		t.Fatalf("typed error = %#v", responseError)
	}
	if callbackResponse == nil {
		t.Fatal("callback response was not captured")
	}
	body := readOpenRouterOAuthResponseBody(t, callbackResponse)
	if callbackResponse.StatusCode != http.StatusBadGateway ||
		!strings.Contains(body, "OpenRouter key exchange failed") ||
		!strings.Contains(body, "invalid code") {
		t.Fatalf(
			"callback response = %d %q",
			callbackResponse.StatusCode,
			body,
		)
	}
}

func TestOpenRouterOAuthAllowsOnlyOneExchange(t *testing.T) {
	exchangeStarted := make(chan struct{})
	releaseExchange := make(chan struct{})
	var calls atomic.Int32
	client := radiusHTTPDoerFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(exchangeStarted)
		}
		select {
		case <-request.Context().Done():
			return nil, request.Context().Err()
		case <-releaseExchange:
		}
		return openRouterOAuthJSONResponse(
			http.StatusOK,
			map[string]any{"key": "sk-or-test"},
		), nil
	})

	callbackURLs := make(chan *url.URL, 1)
	firstResponse := make(chan *http.Response, 1)
	firstError := make(chan error, 1)
	loginResult := make(chan Credential, 1)
	loginError := make(chan error, 1)
	go func() {
		credential, err := NewOpenRouterOAuth(OpenRouterOAuthOptions{
			Client: client,
		}).Login(
			context.Background(),
			openRouterAuthInteraction{
				notify: func(event AuthEvent) {
					if event.Type != AuthEventURL {
						return
					}
					callbackURL := openRouterCallbackURLFromEvent(t, event)
					query := callbackURL.Query()
					query.Set("code", "authorization-code")
					callbackURL.RawQuery = query.Encode()
					callbackURLs <- callbackURL
					go func() {
						response, getErr := (&http.Client{
							Timeout: 2 * time.Second,
						}).Get(callbackURL.String())
						firstResponse <- response
						firstError <- getErr
					}()
				},
			},
		)
		loginResult <- credential
		loginError <- err
	}()

	callbackURL := <-callbackURLs
	select {
	case <-exchangeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("token exchange did not start")
	}
	second, err := (&http.Client{
		Timeout: 2 * time.Second,
	}).Get(callbackURL.String())
	if err != nil {
		t.Fatal(err)
	}
	secondBody := readOpenRouterOAuthResponseBody(t, second)
	if second.StatusCode != http.StatusConflict ||
		!strings.Contains(secondBody, "already been used") ||
		calls.Load() != 1 {
		t.Fatalf(
			"second callback = %d %q calls=%d",
			second.StatusCode,
			secondBody,
			calls.Load(),
		)
	}
	close(releaseExchange)
	credential := <-loginResult
	if err := <-loginError; err != nil {
		t.Fatal(err)
	}
	if credential.Access != "sk-or-test" {
		t.Fatalf("credential = %#v", credential)
	}
	first := <-firstResponse
	if err := <-firstError; err != nil {
		t.Fatal(err)
	}
	if first == nil || first.StatusCode != http.StatusOK {
		t.Fatalf("first callback = %#v", first)
	}
	first.Body.Close()
}

func TestOpenRouterOAuthRejectsSuccessfulResponseWithoutKey(t *testing.T) {
	client := radiusHTTPDoerFunc(func(
		*http.Request,
	) (*http.Response, error) {
		return openRouterOAuthJSONResponse(
			http.StatusOK,
			map[string]any{"user_id": "user-1"},
		), nil
	})
	var callbackResponse *http.Response
	_, err := NewOpenRouterOAuth(OpenRouterOAuthOptions{
		Client: client,
	}).Login(
		context.Background(),
		openRouterAuthInteraction{
			notify: func(event AuthEvent) {
				if event.Type != AuthEventURL {
					return
				}
				callbackURL := openRouterCallbackURLFromEvent(t, event)
				query := callbackURL.Query()
				query.Set("code", "code-without-key")
				callbackURL.RawQuery = query.Encode()
				var callbackErr error
				callbackResponse, callbackErr = (&http.Client{
					Timeout: 2 * time.Second,
				}).Get(callbackURL.String())
				if callbackErr != nil {
					t.Fatal(callbackErr)
				}
			},
		},
	)
	if err == nil ||
		err.Error() != `OpenRouter OAuth response carries no "key"` {
		t.Fatalf("error = %v", err)
	}
	if callbackResponse == nil ||
		callbackResponse.StatusCode != http.StatusBadGateway {
		t.Fatalf("callback response = %#v", callbackResponse)
	}
	callbackResponse.Body.Close()
}

func TestOpenRouterOAuthCancellationClosesCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var callbackURL *url.URL
	_, err := NewOpenRouterOAuth(OpenRouterOAuthOptions{}).Login(
		ctx,
		openRouterAuthInteraction{
			notify: func(event AuthEvent) {
				if event.Type != AuthEventURL {
					return
				}
				callbackURL = openRouterCallbackURLFromEvent(t, event)
				cancel()
			},
		},
	)
	if !errors.Is(err, context.Canceled) ||
		!strings.Contains(err.Error(), "Login cancelled") ||
		callbackURL == nil {
		t.Fatalf("callback=%v error=%v", callbackURL, err)
	}
	_, getErr := (&http.Client{
		Timeout: 100 * time.Millisecond,
	}).Get(callbackURL.String())
	if getErr == nil {
		t.Fatal("callback server remained reachable after cancellation")
	}
}

func TestOpenRouterOAuthRejectsAlreadyCancelledLogin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	events := 0
	_, err := NewOpenRouterOAuth(OpenRouterOAuthOptions{}).Login(
		ctx,
		openRouterAuthInteraction{
			notify: func(AuthEvent) {
				events++
			},
		},
	)
	if !errors.Is(err, context.Canceled) ||
		!strings.Contains(err.Error(), "Login cancelled") ||
		events != 0 {
		t.Fatalf("events=%d error=%v", events, err)
	}
}

func TestOpenRouterOAuthUsesConfiguredCallbackHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var callbackURL *url.URL
	_, err := NewOpenRouterOAuth(OpenRouterOAuthOptions{
		AuthContext: providerAuthContext(map[string]string{
			"PI_OAUTH_CALLBACK_HOST": "localhost",
		}),
	}).Login(
		ctx,
		openRouterAuthInteraction{
			notify: func(event AuthEvent) {
				if event.Type != AuthEventURL {
					return
				}
				callbackURL = openRouterCallbackURLFromEvent(t, event)
				cancel()
			},
		},
	)
	if !errors.Is(err, context.Canceled) ||
		callbackURL == nil ||
		callbackURL.Hostname() != "localhost" {
		t.Fatalf("callback=%v error=%v", callbackURL, err)
	}
}

func TestOpenRouterOAuthTokenExchangeTimeout(t *testing.T) {
	client := radiusHTTPDoerFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	var callbackResponse *http.Response
	_, err := NewOpenRouterOAuth(OpenRouterOAuthOptions{
		Client:               client,
		TokenExchangeTimeout: 5 * time.Millisecond,
	}).Login(
		context.Background(),
		openRouterAuthInteraction{
			notify: func(event AuthEvent) {
				if event.Type != AuthEventURL {
					return
				}
				callbackURL := openRouterCallbackURLFromEvent(t, event)
				query := callbackURL.Query()
				query.Set("code", "slow-code")
				callbackURL.RawQuery = query.Encode()
				var callbackErr error
				callbackResponse, callbackErr = (&http.Client{
					Timeout: 2 * time.Second,
				}).Get(callbackURL.String())
				if callbackErr != nil {
					t.Fatal(callbackErr)
				}
			},
		},
	)
	if err == nil ||
		err.Error() != "OpenRouter OAuth token exchange timed out" {
		t.Fatalf("error = %v", err)
	}
	if callbackResponse == nil ||
		callbackResponse.StatusCode != http.StatusBadGateway {
		t.Fatalf("callback response = %#v", callbackResponse)
	}
	callbackResponse.Body.Close()
}

func TestBuiltinOpenRouterProviderUsesDefaultOAuth(t *testing.T) {
	provider, err := NewBuiltinProvider("openrouter")
	if err != nil {
		t.Fatal(err)
	}
	if provider.Auth.APIKey == nil ||
		provider.Auth.OAuth == nil ||
		provider.Auth.OAuth.Name != "OpenRouter OAuth" ||
		provider.Auth.OAuth.LoginLabel != "Sign in with OpenRouter" {
		t.Fatalf("auth = %#v", provider.Auth)
	}
	credential := Credential{
		Type:    CredentialTypeOAuth,
		Access:  "sk-or-stored",
		Expires: permanentOAuthCredentialExpires,
		Metadata: map[string]any{
			"account": "account-a",
		},
	}
	refreshed, err := provider.Auth.OAuth.Refresh(
		context.Background(),
		credential,
	)
	if err != nil {
		t.Fatal(err)
	}
	refreshed.Metadata["account"] = "changed"
	if credential.Metadata["account"] != "account-a" {
		t.Fatalf("refresh aliased credential metadata: %#v", credential)
	}
	resolved, err := provider.Auth.OAuth.ToAuth(
		context.Background(),
		credential,
	)
	if err != nil || resolved.APIKey != "sk-or-stored" {
		t.Fatalf("auth = %#v, error = %v", resolved, err)
	}
}

func TestOpenRouterOAuthCallbackIDIsRandomUUID(t *testing.T) {
	id, err := randomOpenRouterCallbackID(
		bytes.NewReader(make([]byte, 16)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if id != "00000000-0000-4000-8000-000000000000" {
		t.Fatalf("callback ID = %q", id)
	}
}

type openRouterAuthInteraction struct {
	notify func(AuthEvent)
}

func (i openRouterAuthInteraction) Prompt(
	context.Context,
	AuthPrompt,
) (string, error) {
	return "", errors.New("OpenRouter login must not prompt for a code")
}

func (i openRouterAuthInteraction) Notify(event AuthEvent) {
	if i.notify != nil {
		i.notify(event)
	}
}

func openRouterCallbackURLFromEvent(
	t *testing.T,
	event AuthEvent,
) *url.URL {
	t.Helper()
	authorizeURL, err := url.Parse(event.URL)
	if err != nil {
		t.Fatal(err)
	}
	callbackURL, err := url.Parse(
		authorizeURL.Query().Get("callback_url"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return callbackURL
}

func openRouterOAuthJSONResponse(
	status int,
	value any,
) *http.Response {
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

func readOpenRouterOAuthResponseBody(
	t *testing.T,
	response *http.Response,
) string {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
