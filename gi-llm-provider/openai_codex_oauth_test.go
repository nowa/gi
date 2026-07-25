package gillmprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestOpenAICodexOAuthDeviceCodeLoginDataFlow(t *testing.T) {
	now := time.Date(2026, 7, 25, 11, 0, 0, 0, time.UTC)
	accessToken := openAICodexOAuthTestAccessToken(t, "account-123")
	var (
		devicePolls int
		tokenForm   url.Values
	)
	client := radiusHTTPDoerFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		switch request.URL.String() {
		case openAICodexDeviceUserCodeURL:
			var body map[string]string
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if request.Header.Get("content-type") != "application/json" ||
				body["client_id"] != openAICodexOAuthClientID {
				t.Fatalf("device-code request = %#v", body)
			}
			return openAICodexOAuthJSONResponse(
				http.StatusOK,
				`{
					"device_auth_id":"device-auth-id",
					"user_code":"ABCD-1234",
					"interval":"5"
				}`,
			), nil
		case openAICodexDeviceTokenURL:
			devicePolls++
			var body map[string]string
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["device_auth_id"] != "device-auth-id" ||
				body["user_code"] != "ABCD-1234" {
				t.Fatalf("device token request = %#v", body)
			}
			if devicePolls == 1 {
				return openAICodexOAuthJSONResponse(
					http.StatusForbidden,
					`{"error":{"code":"deviceauth_authorization_pending"}}`,
				), nil
			}
			return openAICodexOAuthJSONResponse(
				http.StatusOK,
				`{
					"authorization_code":"authorization-code",
					"code_verifier":"device-verifier"
				}`,
			), nil
		case openAICodexOAuthTokenURL:
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			tokenForm, err = url.ParseQuery(string(body))
			if err != nil {
				t.Fatal(err)
			}
			return openAICodexOAuthJSONResponse(
				http.StatusOK,
				`{
					"access_token":"`+accessToken+`",
					"refresh_token":"refresh-token",
					"expires_in":3600
				}`,
			), nil
		default:
			t.Fatalf("unexpected request %s", request.URL)
			return nil, nil
		}
	})
	var events []AuthEvent
	interaction := oauthProviderTestInteraction{
		prompt: func(
			_ context.Context,
			prompt AuthPrompt,
		) (string, error) {
			if prompt.Type != AuthPromptSelect {
				t.Fatalf("prompt = %#v", prompt)
			}
			if len(prompt.Options) != 2 ||
				prompt.Options[0].ID != openAICodexBrowserLoginMethod ||
				prompt.Options[1].ID != openAICodexDeviceCodeLoginMethod {
				t.Fatalf("options = %#v", prompt.Options)
			}
			return openAICodexDeviceCodeLoginMethod, nil
		},
		notify: func(event AuthEvent) {
			events = append(events, event)
		},
	}
	auth := newOpenAICodexOAuth(
		OpenAICodexOAuthOptions{Client: client},
		openAICodexOAuthRuntime{
			now: func() time.Time { return now },
			pollDeviceCode: func(
				ctx context.Context,
				options OAuthDeviceCodePollOptions[openAICodexDeviceToken],
			) (openAICodexDeviceToken, error) {
				if options.IntervalSeconds != 5 ||
					options.ExpiresInSeconds != 900 ||
					options.WaitBeforeFirstPoll {
					t.Fatalf("poll options = %#v", options)
				}
				first, err := options.Poll(ctx)
				if err != nil {
					return openAICodexDeviceToken{}, err
				}
				if first.Status != OAuthDeviceCodePending {
					t.Fatalf("first poll = %#v", first)
				}
				second, err := options.Poll(ctx)
				if err != nil {
					return openAICodexDeviceToken{}, err
				}
				if second.Status != OAuthDeviceCodeComplete {
					t.Fatalf("second poll = %#v", second)
				}
				return second.Value, nil
			},
		},
	)

	credential, err := auth.Login(context.Background(), interaction)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Type != CredentialTypeOAuth ||
		credential.Access != accessToken ||
		credential.Refresh != "refresh-token" ||
		credential.Expires != now.Add(time.Hour).UnixMilli() ||
		credential.Metadata["accountId"] != "account-123" {
		t.Fatalf("credential = %#v", credential)
	}
	if tokenForm.Get("grant_type") != "authorization_code" ||
		tokenForm.Get("client_id") != openAICodexOAuthClientID ||
		tokenForm.Get("code") != "authorization-code" ||
		tokenForm.Get("code_verifier") != "device-verifier" ||
		tokenForm.Get("redirect_uri") != openAICodexDeviceRedirectURI {
		t.Fatalf("token form = %v", tokenForm)
	}
	if len(events) != 1 ||
		events[0].Type != AuthEventDeviceCode ||
		events[0].UserCode != "ABCD-1234" ||
		events[0].VerificationURI != openAICodexDeviceVerificationURI ||
		events[0].IntervalSeconds != 5 ||
		events[0].ExpiresInSeconds != 900 {
		t.Fatalf("events = %#v", events)
	}
}

func TestOpenAICodexOAuthBrowserManualFallback(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	accessToken := openAICodexOAuthTestAccessToken(t, "account-browser")
	var (
		events    []AuthEvent
		tokenForm url.Values
	)
	client := radiusHTTPDoerFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		tokenForm, err = url.ParseQuery(string(body))
		if err != nil {
			t.Fatal(err)
		}
		return openAICodexOAuthJSONResponse(
			http.StatusOK,
			`{
				"access_token":"`+accessToken+`",
				"refresh_token":"refresh-browser",
				"expires_in":1800
			}`,
		), nil
	})
	interaction := oauthProviderTestInteraction{
		prompt: func(
			_ context.Context,
			prompt AuthPrompt,
		) (string, error) {
			switch prompt.Type {
			case AuthPromptSelect:
				return openAICodexBrowserLoginMethod, nil
			case AuthPromptManualCode:
				return openAICodexOAuthRedirectURI +
					"?code=manual-code&state=state", nil
			default:
				t.Fatalf("prompt = %#v", prompt)
				return "", nil
			}
		},
		notify: func(event AuthEvent) {
			events = append(events, event)
		},
	}
	auth := newOpenAICodexOAuth(
		OpenAICodexOAuthOptions{
			Client:     client,
			Originator: "gi-test",
		},
		openAICodexOAuthRuntime{
			now: func() time.Time { return now },
			generatePKCE: func() (PKCE, error) {
				return PKCE{
					Verifier:  "verifier",
					Challenge: "challenge",
				}, nil
			},
			randomState: func() (string, error) {
				return "state", nil
			},
			startCallback: func(
				context.Context,
				oauthLoopbackCallbackOptions,
			) (oauthAuthorizationCodeServer, error) {
				return nil, errors.New("address already in use")
			},
		},
	)

	credential, err := auth.Login(context.Background(), interaction)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Access != accessToken ||
		credential.Refresh != "refresh-browser" ||
		credential.Expires != now.Add(30*time.Minute).UnixMilli() ||
		credential.Metadata["accountId"] != "account-browser" {
		t.Fatalf("credential = %#v", credential)
	}
	if len(events) != 1 || events[0].Type != AuthEventURL {
		t.Fatalf("events = %#v", events)
	}
	authorizeURL, err := url.Parse(events[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"response_type":              "code",
		"client_id":                  openAICodexOAuthClientID,
		"redirect_uri":               openAICodexOAuthRedirectURI,
		"scope":                      openAICodexOAuthScope,
		"code_challenge":             "challenge",
		"code_challenge_method":      "S256",
		"state":                      "state",
		"id_token_add_organizations": "true",
		"codex_cli_simplified_flow":  "true",
		"originator":                 "gi-test",
	} {
		if got := authorizeURL.Query().Get(key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
	if tokenForm.Get("grant_type") != "authorization_code" ||
		tokenForm.Get("code") != "manual-code" ||
		tokenForm.Get("code_verifier") != "verifier" ||
		tokenForm.Get("redirect_uri") != openAICodexOAuthRedirectURI {
		t.Fatalf("token form = %v", tokenForm)
	}
}

func TestOpenAICodexOAuthRefreshRotatesAccountState(t *testing.T) {
	now := time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)
	accessToken := openAICodexOAuthTestAccessToken(t, "new-account")
	var tokenForm url.Values
	client := radiusHTTPDoerFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		tokenForm, err = url.ParseQuery(string(body))
		if err != nil {
			t.Fatal(err)
		}
		return openAICodexOAuthJSONResponse(
			http.StatusOK,
			`{
				"access_token":"`+accessToken+`",
				"refresh_token":"new-refresh",
				"expires_in":3600
			}`,
		), nil
	})
	auth := newOpenAICodexOAuth(
		OpenAICodexOAuthOptions{Client: client},
		openAICodexOAuthRuntime{now: func() time.Time { return now }},
	)
	previous := Credential{
		Type:    CredentialTypeOAuth,
		Access:  "old-access",
		Refresh: "old-refresh",
		Env:     ProviderEnv{"tenant": "test"},
		Metadata: map[string]any{
			"accountId": "old-account",
			"preserved": true,
		},
	}

	refreshed, err := auth.Refresh(context.Background(), previous)
	if err != nil {
		t.Fatal(err)
	}
	if tokenForm.Get("grant_type") != "refresh_token" ||
		tokenForm.Get("refresh_token") != "old-refresh" ||
		tokenForm.Get("client_id") != openAICodexOAuthClientID {
		t.Fatalf("token form = %v", tokenForm)
	}
	if refreshed.Access != accessToken ||
		refreshed.Refresh != "new-refresh" ||
		refreshed.Expires != now.Add(time.Hour).UnixMilli() ||
		refreshed.Env["tenant"] != "test" ||
		refreshed.Metadata["accountId"] != "new-account" ||
		refreshed.Metadata["preserved"] != true {
		t.Fatalf("refreshed = %#v", refreshed)
	}
	modelAuth, err := auth.ToAuth(context.Background(), refreshed)
	if err != nil {
		t.Fatal(err)
	}
	if modelAuth.APIKey != accessToken {
		t.Fatalf("model auth = %#v", modelAuth)
	}
}

func TestOpenAICodexDevicePollStatusContracts(t *testing.T) {
	device := openAICodexDeviceAuth{
		DeviceAuthID:    "device",
		UserCode:        "code",
		IntervalSeconds: 1,
	}
	responses := []*http.Response{
		openAICodexOAuthJSONResponse(
			http.StatusBadRequest,
			`{"error":{"code":"deviceauth_authorization_pending"}}`,
		),
		openAICodexOAuthJSONResponse(
			http.StatusTooManyRequests,
			`{"error":"slow_down"}`,
		),
		openAICodexOAuthJSONResponse(
			http.StatusInternalServerError,
			`{"error":"server_error","error_description":"later"}`,
		),
	}
	client := radiusHTTPDoerFunc(func(
		*http.Request,
	) (*http.Response, error) {
		response := responses[0]
		responses = responses[1:]
		return response, nil
	})
	config := openAICodexOAuthConfig{
		client:         client,
		requestTimeout: time.Second,
	}
	_, err := pollOpenAICodexDeviceAuth(
		context.Background(),
		config,
		device,
		func(
			ctx context.Context,
			options OAuthDeviceCodePollOptions[openAICodexDeviceToken],
		) (openAICodexDeviceToken, error) {
			first, err := options.Poll(ctx)
			if err != nil {
				return openAICodexDeviceToken{}, err
			}
			second, err := options.Poll(ctx)
			if err != nil {
				return openAICodexDeviceToken{}, err
			}
			third, err := options.Poll(ctx)
			if err != nil {
				return openAICodexDeviceToken{}, err
			}
			if first.Status != OAuthDeviceCodePending ||
				second.Status != OAuthDeviceCodeSlowDown ||
				third.Status != OAuthDeviceCodeFailed {
				t.Fatalf(
					"poll statuses = %q %q %q",
					first.Status,
					second.Status,
					third.Status,
				)
			}
			return openAICodexDeviceToken{}, errors.New(third.Message)
		},
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			`OpenAI Codex device auth failed with status 500: `+
				`{"error":"server_error","error_description":"later"}`,
		) {
		t.Fatalf("error = %v", err)
	}
}

func TestOpenAICodexTokenResponseValidationAndRefreshError(t *testing.T) {
	_, err := readOpenAICodexTokenResponse(
		openAICodexOAuthHTTPResponse{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       []byte(`{"access_token":"only-access"}`),
		},
		"exchange",
		time.Now,
	)
	if err == nil || !strings.Contains(err.Error(), "missing fields") {
		t.Fatalf("missing-fields error = %v", err)
	}

	_, err = readOpenAICodexTokenResponse(
		openAICodexOAuthHTTPResponse{
			StatusCode: http.StatusUnauthorized,
			Status:     "401 Unauthorized",
			Body: []byte(`{
				"error":{"message":"Could not validate your token."}
			}`),
		},
		"refresh",
		time.Now,
	)
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"OpenAI Codex token refresh failed (401): "+
				"Could not validate your token.",
		) {
		t.Fatalf("refresh error = %v", err)
	}
}

func TestOpenAICodexOAuthCallbackValidatesStateBeforeCodeWithoutSettling(
	t *testing.T,
) {
	server := &localOAuthAuthorizationCodeServer{
		result: make(chan oauthAuthorizationCodeResult, 1),
	}
	options := oauthLoopbackCallbackOptions{
		Path:               openAICodexOAuthCallbackPath,
		ExpectedState:      "expected",
		ProviderName:       "OpenAI",
		ValidateStateFirst: true,
	}
	request := &http.Request{
		Method: http.MethodGet,
		URL: &url.URL{
			Path: openAICodexOAuthCallbackPath,
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
			Path: openAICodexOAuthCallbackPath,
			RawQuery: url.Values{
				"code":  {"valid-code"},
				"state": {"expected"},
			}.Encode(),
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

func TestOpenAICodexOAuthLoginCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := radiusHTTPDoerFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		cancel()
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	auth := newOpenAICodexOAuth(
		OpenAICodexOAuthOptions{Client: client},
		openAICodexOAuthRuntime{
			pollDeviceCode: PollOAuthDeviceCodeFlow[openAICodexDeviceToken],
		},
	)
	_, err := auth.Login(
		ctx,
		oauthProviderTestInteraction{
			prompt: func(
				context.Context,
				AuthPrompt,
			) (string, error) {
				return openAICodexDeviceCodeLoginMethod, nil
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "Login cancelled") {
		t.Fatalf("error = %v", err)
	}
}

func TestAuthorizationCodeOAuthBuiltinsAndLoaderOverrides(t *testing.T) {
	for _, providerID := range []string{"anthropic", "openai-codex"} {
		t.Run(providerID, func(t *testing.T) {
			previous := getOAuthAuthLoader(providerID)
			UnregisterOAuthAuthLoader(providerID)
			t.Cleanup(func() {
				RegisterOAuthAuthLoader(providerID, previous)
			})

			provider, err := NewBuiltinProvider(providerID)
			if err != nil {
				t.Fatal(err)
			}
			if provider.Auth.OAuth == nil {
				t.Fatal("builtin OAuth auth is missing")
			}
			result, err := provider.Auth.OAuth.ToAuth(
				context.Background(),
				Credential{
					Type:   CredentialTypeOAuth,
					Access: "builtin-access",
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.APIKey != "builtin-access" {
				t.Fatalf("builtin model auth = %#v", result)
			}

			RegisterOAuthAuthLoader(
				providerID,
				func(context.Context) (*OAuthAuth, error) {
					return &OAuthAuth{
						ToAuth: func(
							context.Context,
							Credential,
						) (ModelAuth, error) {
							return ModelAuth{
								APIKey: "override-access",
							}, nil
						},
					}, nil
				},
			)
			overridden, err := NewBuiltinProvider(providerID)
			if err != nil {
				t.Fatal(err)
			}
			result, err = overridden.Auth.OAuth.ToAuth(
				context.Background(),
				Credential{Type: CredentialTypeOAuth},
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.APIKey != "override-access" {
				t.Fatalf("overridden model auth = %#v", result)
			}
		})
	}
}

func openAICodexOAuthTestAccessToken(
	t *testing.T,
	accountID string,
) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": "none"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		codexJWTClaimPath: map[string]string{
			"chatgpt_account_id": accountID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload) +
		".signature"
}

func openAICodexOAuthJSONResponse(
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
