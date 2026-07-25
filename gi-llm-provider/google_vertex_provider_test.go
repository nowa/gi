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
)

func TestGoogleVertexProviderStreamsWithExpressAPIKey(t *testing.T) {
	var captured *http.Request
	var payload GooglePayload
	tokenCalls := 0
	payloadCalls := 0
	statusCalls := 0
	provider := NewGoogleVertexAPIProvider(
		simpleOptionsHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
			captured = request
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			return googleVertexTestResponse(request), nil
		}),
		GoogleVertexTokenProviderFunc(func(context.Context, GoogleVertexTokenRequest) (string, error) {
			tokenCalls++
			return "", errors.New("ADC must not be used in Express mode")
		}),
	)
	model := MustGetModel("google-vertex", "gemini-3-flash-preview")
	model.Headers = map[string]string{
		"X-Model-Header": "model",
		"X-Override":     "model",
		"X-Remove":       "drop",
	}
	stream, err := provider.Stream(model, Context{
		SystemPrompt: "Be concise.",
		Messages:     []Message{UserMessageText("hello")},
	}, StreamOptions{
		APIKey:         "vertex-api-key",
		Headers:        map[string]string{"x-override": "request"},
		HeaderRemovals: []string{"x-remove"},
		OnPayload: func(raw any, hookModel Model) (any, bool, error) {
			payloadCalls++
			if hookModel.ID != model.ID {
				t.Fatalf("payload hook model = %#v", hookModel)
			}
			return raw, false, nil
		},
		OnResponseStatus: func(status int, _ map[string]string, hookModel Model) error {
			statusCalls++
			if status != http.StatusOK || hookModel.ID != model.ID {
				t.Fatalf("status hook: status=%d model=%#v", status, hookModel)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if tokenCalls != 0 {
		t.Fatalf("ADC token calls = %d, want 0", tokenCalls)
	}
	if payloadCalls != 1 || statusCalls != 1 {
		t.Fatalf("payload calls=%d status calls=%d", payloadCalls, statusCalls)
	}
	if captured == nil {
		t.Fatal("HTTP client was not called")
	}
	if got := captured.URL.String(); got != "https://aiplatform.googleapis.com/v1/publishers/google/models/gemini-3-flash-preview:streamGenerateContent?alt=sse" {
		t.Fatalf("endpoint = %q", got)
	}
	if got := captured.Header.Get("x-goog-api-key"); got != "vertex-api-key" {
		t.Fatalf("x-goog-api-key = %q", got)
	}
	if got := captured.Header.Get("X-Override"); got != "request" {
		t.Fatalf("overridden header = %q", got)
	}
	if captured.Header.Get("X-Model-Header") != "model" || captured.Header.Get("X-Remove") != "" {
		t.Fatalf("headers = %#v", captured.Header)
	}
	if payload.SystemInstruction == nil || len(payload.Contents) != 1 {
		t.Fatalf("payload = %#v", payload)
	}
	if result.ResponseID != "vertex-response" || result.Content[0].Text != "ok" {
		t.Fatalf("result = %#v", result)
	}
}

func TestGoogleVertexProviderStreamsWithRequestScopedADC(t *testing.T) {
	var captured *http.Request
	var tokenRequest GoogleVertexTokenRequest
	tokenCalls := 0
	provider := NewGoogleVertexAPIProvider(
		simpleOptionsHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
			captured = request
			return googleVertexTestResponse(request), nil
		}),
		GoogleVertexTokenProviderFunc(func(_ context.Context, request GoogleVertexTokenRequest) (string, error) {
			tokenCalls++
			tokenRequest = request
			return "adc-token", nil
		}),
	)
	model := MustGetModel("google-vertex", "gemini-3-flash-preview")
	stream, err := provider.Stream(model, Context{
		Messages: []Message{UserMessageText("hello")},
	}, StreamOptions{
		Project:  "scoped-project",
		Location: "us-central1",
		Env: ProviderEnv{
			"GOOGLE_APPLICATION_CREDENTIALS": "/credentials/request.json",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Result(context.Background()); err != nil {
		t.Fatal(err)
	}

	if tokenCalls != 1 ||
		tokenRequest.Project != "scoped-project" ||
		tokenRequest.Location != "us-central1" ||
		tokenRequest.AuthOptions == nil ||
		tokenRequest.AuthOptions.KeyFilename != "/credentials/request.json" {
		t.Fatalf("token calls=%d request=%#v", tokenCalls, tokenRequest)
	}
	if captured == nil {
		t.Fatal("HTTP client was not called")
	}
	wantEndpoint := "https://us-central1-aiplatform.googleapis.com/v1/projects/scoped-project/locations/us-central1/publishers/google/models/gemini-3-flash-preview:streamGenerateContent?alt=sse"
	if got := captured.URL.String(); got != wantEndpoint {
		t.Fatalf("endpoint = %q, want %q", got, wantEndpoint)
	}
	if got := captured.Header.Get("Authorization"); got != "Bearer adc-token" {
		t.Fatalf("Authorization = %q", got)
	}
	if captured.Header.Get("x-goog-api-key") != "" {
		t.Fatalf("unexpected API-key header: %#v", captured.Header)
	}
}

func TestGoogleVertexProviderValidatesADCBeforeResolvingToken(t *testing.T) {
	tests := []struct {
		name    string
		options StreamOptions
		want    string
	}{
		{
			name:    "project",
			options: StreamOptions{Location: "us-central1"},
			want:    "Vertex AI requires a project ID",
		},
		{
			name:    "location",
			options: StreamOptions{Project: "project"},
			want:    "Vertex AI requires a location",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tokenCalls := 0
			httpCalls := 0
			provider := NewGoogleVertexAPIProvider(
				simpleOptionsHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
					httpCalls++
					return nil, errors.New("unexpected HTTP request")
				}),
				GoogleVertexTokenProviderFunc(func(context.Context, GoogleVertexTokenRequest) (string, error) {
					tokenCalls++
					return "token", nil
				}),
			)
			model := MustGetModel("google-vertex", "gemini-3-flash-preview")
			stream, err := provider.Stream(model, Context{}, tc.options)
			if err != nil {
				t.Fatal(err)
			}
			result, err := stream.Result(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if tokenCalls != 0 || httpCalls != 0 {
				t.Fatalf("token calls=%d HTTP calls=%d", tokenCalls, httpCalls)
			}
			if result.StopReason != StopReasonError || !strings.Contains(result.ErrorMessage, tc.want) {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestGoogleVertexProviderReportsTokenFailureBeforeHTTP(t *testing.T) {
	httpCalls := 0
	provider := NewGoogleVertexAPIProvider(
		simpleOptionsHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			httpCalls++
			return nil, errors.New("unexpected HTTP request")
		}),
		GoogleVertexTokenProviderFunc(func(context.Context, GoogleVertexTokenRequest) (string, error) {
			return "", errors.New("ADC unavailable")
		}),
	)
	model := MustGetModel("google-vertex", "gemini-3-flash-preview")
	stream, err := provider.Stream(model, Context{}, StreamOptions{
		Project:  "project",
		Location: "us-central1",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if httpCalls != 0 {
		t.Fatalf("HTTP calls = %d", httpCalls)
	}
	if result.StopReason != StopReasonError || !strings.Contains(result.ErrorMessage, "ADC unavailable") {
		t.Fatalf("result = %#v", result)
	}
}

func TestGoogleVertexBuiltInADCFailureIsExplicit(t *testing.T) {
	httpCalls := 0
	provider := NewGoogleVertexAPIProvider(
		simpleOptionsHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			httpCalls++
			return nil, errors.New("unexpected HTTP request")
		}),
		nil,
	)
	model := MustGetModel("google-vertex", "gemini-3-flash-preview")
	stream, err := provider.Stream(model, Context{}, StreamOptions{
		Project:  "project",
		Location: "us-central1",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if httpCalls != 0 {
		t.Fatalf("HTTP calls = %d", httpCalls)
	}
	if result.StopReason != StopReasonError ||
		!strings.Contains(result.ErrorMessage, "GoogleVertexTokenProvider") {
		t.Fatalf("result = %#v", result)
	}
}

func TestGoogleVertexProviderRejectsStrictSamplingBeforeAuth(t *testing.T) {
	tokenCalls := 0
	httpCalls := 0
	provider := NewGoogleVertexAPIProvider(
		simpleOptionsHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			httpCalls++
			return nil, errors.New("unexpected HTTP request")
		}),
		GoogleVertexTokenProviderFunc(func(context.Context, GoogleVertexTokenRequest) (string, error) {
			tokenCalls++
			return "token", nil
		}),
	)
	model := Model{
		ID:       "gemini-2.5-pro",
		Provider: "google-vertex",
		API:      "google-vertex",
	}
	stream, err := provider.Stream(model, Context{Tools: []Tool{{
		Name: "lookup",
		ConstrainedSampling: &ConstrainedSamplingConfig{
			Type:   ConstrainedSamplingJSONSchema,
			Strict: ConstrainedSamplingRequire,
		},
	}}}, StreamOptions{Project: "project", Location: "us-central1"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tokenCalls != 0 || httpCalls != 0 {
		t.Fatalf("token calls=%d HTTP calls=%d", tokenCalls, httpCalls)
	}
	if result.StopReason != StopReasonError ||
		!strings.Contains(result.ErrorMessage, "requires JSON-schema constrained sampling") {
		t.Fatalf("result = %#v", result)
	}
}

func TestGoogleVertexStreamEndpointMatchesSDKRouting(t *testing.T) {
	model := Model{ID: "partner/model with space"}
	tests := []struct {
		name   string
		config GoogleVertexClientConfig
		want   string
	}{
		{
			name: "Express mode",
			config: GoogleVertexClientConfig{
				APIKey:     "key",
				APIVersion: "v1",
			},
			want: "https://aiplatform.googleapis.com/v1/publishers/partner/models/model%20with%20space:streamGenerateContent?alt=sse",
		},
		{
			name: "global ADC",
			config: GoogleVertexClientConfig{
				Project:    "project",
				Location:   "global",
				APIVersion: "v1",
			},
			want: "https://aiplatform.googleapis.com/v1/projects/project/locations/global/publishers/partner/models/model%20with%20space:streamGenerateContent?alt=sse",
		},
		{
			name: "multi-region ADC",
			config: GoogleVertexClientConfig{
				Project:    "project",
				Location:   "eu",
				APIVersion: "v1",
			},
			want: "https://aiplatform.eu.rep.googleapis.com/v1/projects/project/locations/eu/publishers/partner/models/model%20with%20space:streamGenerateContent?alt=sse",
		},
		{
			name: "versioned custom collection",
			config: GoogleVertexClientConfig{
				Project:    "ignored-by-collection-scope",
				Location:   "us-central1",
				APIVersion: "v1",
				HTTPOptions: &GoogleVertexHTTPOptions{
					BaseURL:              "https://proxy.example.com/custom/v1?tenant=test",
					BaseURLResourceScope: "COLLECTION",
				},
			},
			want: "https://proxy.example.com/custom/v1/publishers/partner/models/model%20with%20space:streamGenerateContent?alt=sse&tenant=test",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GoogleVertexStreamEndpoint(model, tc.config)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("endpoint = %q, want %q", got, tc.want)
			}
			if _, err := url.ParseRequestURI(got); err != nil {
				t.Fatalf("invalid endpoint %q: %v", got, err)
			}
		})
	}
}

func TestBuildGoogleAuthOptionsUsesScopedCredentialsFile(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/credentials/process.json")
	options := BuildGoogleAuthOptions(ProviderEnv{
		"GOOGLE_APPLICATION_CREDENTIALS": "/credentials/request.json",
	})
	if options == nil || options.KeyFilename != "/credentials/request.json" {
		t.Fatalf("auth options = %#v", options)
	}
}

func TestGoogleVertexProviderHonorsCallerManagedAuthHeaders(t *testing.T) {
	tokenCalls := 0
	var authorization string
	provider := NewGoogleVertexAPIProvider(
		simpleOptionsHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
			authorization = request.Header.Get("Authorization")
			return googleVertexTestResponse(request), nil
		}),
		GoogleVertexTokenProviderFunc(func(context.Context, GoogleVertexTokenRequest) (string, error) {
			tokenCalls++
			return "ADC should not replace caller auth", nil
		}),
	)
	model := MustGetModel("google-vertex", "gemini-3-flash-preview")
	stream, err := provider.Stream(model, Context{}, StreamOptions{
		Project:  "project",
		Location: "us-central1",
		Headers:  map[string]string{"authorization": "Bearer caller-token"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Result(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tokenCalls != 0 || authorization != "Bearer caller-token" {
		t.Fatalf("token calls=%d Authorization=%q", tokenCalls, authorization)
	}
}

func googleVertexTestResponse(request *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"content-type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"responseId\":\"vertex-response\",\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"ok\"}]},\"finishReason\":\"STOP\"}]}\n\n",
		)),
		Request: request,
	}
}
