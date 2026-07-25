package gicodingagent

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestModelRuntimeComposesRemoteModelsConfigAndExtensionLayers(
	t *testing.T,
) {
	modelsPath := filepath.Join(t.TempDir(), "models.json")
	writeRawModelsJSON(t, modelsPath, map[string]any{
		"providers": map[string]any{
			"openai": map[string]any{
				"apiKey":     "configured-key",
				"authHeader": true,
				"baseUrl":    "https://models-json.test/v1",
				"modelOverrides": map[string]any{
					"remote-model": map[string]any{
						"name": "User Override",
					},
				},
			},
		},
	})

	var (
		callsMu sync.Mutex
		calls   = map[string]int{}
	)
	generatedAt := llm.GetBuiltinModelDataGeneratedAt()
	client := modelRuntimeCatalogDoerFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		parts := strings.Split(request.URL.Path, "/")
		providerID := parts[len(parts)-1]
		callsMu.Lock()
		calls[providerID]++
		callsMu.Unlock()
		if providerID != "openai" {
			return modelRuntimeCatalogResponse(
				http.StatusNotImplemented,
				"",
				nil,
			), nil
		}
		if !strings.HasPrefix(
			request.Header.Get("User-Agent"),
			"gi/",
		) {
			t.Errorf(
				"user agent = %q",
				request.Header.Get("User-Agent"),
			)
		}
		return modelRuntimeCatalogResponse(
			http.StatusOK,
			`{"remote":{"id":"remote-model","name":"Remote","api":"openai-completions","provider":"wrong","baseUrl":"https://remote.test/v1","reasoning":false,"input":["text"],"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0},"contextWindow":1000,"maxTokens":100}}`,
			http.Header{
				"Last-Modified": []string{
					generatedAt.Add(time.Minute).Format(http.TimeFormat),
				},
			},
		), nil
	})

	registry := NewModelRegistryWithOptions(
		context.Background(),
		ModelRegistryOptions{
			AuthStorage:    NewInMemoryAuthStorage(nil),
			ModelsJSONPath: modelsPath,
			CatalogBaseURL: "https://catalog.test",
			CatalogClient:  client,
			ModelNetworkEnabled: modelRuntimeBoolPointer(
				true,
			),
			AllowModelNetwork: true,
		},
	)
	runtime, err := NewModelRuntimeFromRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := runtime.GetModel("openai", "remote-model")
	if !ok {
		t.Fatal("remote model was not published")
	}
	if model.Name != "User Override" ||
		model.BaseURL != "https://models-json.test/v1" {
		t.Fatalf("models.json projection = %#v", model)
	}
	if config := runtime.GetCompatibilityRequestConfig(model); !config.AuthHeader {
		t.Fatalf("models.json request config = %#v", config)
	}
	initialAuth, err := runtime.GetAuth(
		context.Background(),
		model,
		llm.AuthResolutionOverrides{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := initialAuth.Headers["Authorization"]; got !=
		"Bearer configured-key" {
		t.Fatalf("models.json authorization = %q", got)
	}
	callsMu.Lock()
	openAICalls := calls["openai"]
	callsMu.Unlock()
	if openAICalls != 1 {
		t.Fatalf("openai catalog calls = %d, want 1", openAICalls)
	}

	if err := runtime.RegisterProvider(
		"openai",
		ProviderConfigInput{
			BaseURL:    "https://extension.test/v1",
			AuthHeader: modelRuntimeBoolPointer(false),
		},
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		runtime.UnregisterProvider("openai")
	})
	model, ok = runtime.GetModel("openai", "remote-model")
	if !ok ||
		model.Name != "User Override" ||
		model.BaseURL != "https://extension.test/v1" {
		t.Fatalf("extension projection = %#v, %v", model, ok)
	}
	if config := runtime.GetCompatibilityRequestConfig(model); config.AuthHeader {
		t.Fatalf("extension request config = %#v", config)
	}
	extensionAuth, err := runtime.GetAuth(
		context.Background(),
		model,
		llm.AuthResolutionOverrides{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := extensionAuth.Headers["Authorization"]; got != "" {
		t.Fatalf("extension authorization = %q", got)
	}
}

func TestModelRuntimeExtensionRefreshValidatesBeforePublish(
	t *testing.T,
) {
	registry := NewInMemoryModelRegistry(
		NewInMemoryAuthStorage(nil),
	)
	var (
		refreshCalls atomic.Int32
		invalid      atomic.Bool
	)
	if err := registry.RegisterProvider(
		"refresh-provider",
		ProviderConfigInput{
			BaseURL: "https://provider.test/v1",
			APIKey:  "configured-key",
			API:     "openai-completions",
			Models: []ProviderModelDefinition{{
				ID:            "baseline",
				Name:          "Baseline",
				ContextWindow: 1000,
				MaxTokens:     100,
			}},
			RefreshModels: func(
				_ context.Context,
				input llm.RefreshModelsContext,
			) ([]ProviderModelDefinition, error) {
				refreshCalls.Add(1)
				if input.AllowNetwork {
					t.Fatal("startup refresh unexpectedly allowed network")
				}
				if invalid.Load() {
					return []ProviderModelDefinition{{
						ID:            "invalid",
						Name:          "Invalid",
						ContextWindow: -1,
						MaxTokens:     100,
					}}, nil
				}
				return []ProviderModelDefinition{{
					ID:            "refreshed",
					Name:          "Refreshed",
					ContextWindow: 2000,
					MaxTokens:     200,
				}}, nil
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		registry.UnregisterProvider("refresh-provider")
	})

	runtime, err := NewModelRuntimeFromRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := runtime.GetModel(
		"refresh-provider",
		"refreshed",
	); !ok {
		t.Fatal("validated extension refresh was not published")
	}

	invalid.Store(true)
	result, err := runtime.Refresh(
		context.Background(),
		ModelRegistryRefreshOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Errors["refresh-provider"] == nil {
		t.Fatalf("refresh errors = %#v", result.Errors)
	}
	if _, ok := runtime.GetModel(
		"refresh-provider",
		"invalid",
	); ok {
		t.Fatal("invalid extension refresh was published")
	}
	if _, ok := runtime.GetModel(
		"refresh-provider",
		"baseline",
	); !ok {
		t.Fatal("baseline model was not retained after invalid refresh")
	}
	if refreshCalls.Load() < 2 {
		t.Fatalf("refresh calls = %d", refreshCalls.Load())
	}
}

func TestModelRuntimeOAuthModelProjectionUsesRefreshCredential(
	t *testing.T,
) {
	const providerID = "oauth-model-provider"
	auth := NewInMemoryAuthStorage(AuthStorageData{
		providerID: {
			Type:    llm.CredentialTypeOAuth,
			Access:  "oauth-access",
			Refresh: "oauth-refresh",
			Expires: time.Now().Add(time.Hour).UnixMilli(),
		},
	})
	registry := NewInMemoryModelRegistry(auth)
	if err := registry.RegisterProvider(
		providerID,
		ProviderConfigInput{
			BaseURL: "https://provider.test/v1",
			API:     "openai-completions",
			OAuth: &OAuthProvider{
				Name: "Projected OAuth",
				RefreshToken: func(
					credential AuthCredential,
				) (AuthCredential, error) {
					return credential, nil
				},
				GetAPIKey: func(
					credential AuthCredential,
				) string {
					return credential.Access
				},
				ModifyModels: func(
					models []llm.Model,
					credential AuthCredential,
				) []llm.Model {
					if credential.Access != "oauth-access" {
						t.Fatalf(
							"OAuth credential = %#v",
							credential,
						)
					}
					models[0].Name = "OAuth Projected"
					return models
				},
			},
			Models: []ProviderModelDefinition{{
				ID:            "oauth-model",
				Name:          "Original",
				ContextWindow: 1000,
				MaxTokens:     100,
			}},
		},
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		registry.UnregisterProvider(providerID)
	})

	runtime, err := NewModelRuntimeFromRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := runtime.GetModel(providerID, "oauth-model")
	if !ok || model.Name != "OAuth Projected" {
		t.Fatalf("OAuth model projection = %#v, %v", model, ok)
	}
	resolved, err := runtime.GetAuth(
		context.Background(),
		model,
		llm.AuthResolutionOverrides{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if resolved == nil || resolved.APIKey != "oauth-access" {
		t.Fatalf("OAuth request auth = %#v", resolved)
	}
	provider, ok := runtime.GetProvider(providerID)
	if !ok || provider.Auth.APIKey != nil ||
		provider.Auth.OAuth == nil {
		t.Fatalf("OAuth provider auth = %#v, %v", provider, ok)
	}
}

func TestModelRuntimeProviderReregistrationRebuildsRequestProjection(
	t *testing.T,
) {
	const providerID = "replace-provider"
	registry := NewInMemoryModelRegistry(
		NewInMemoryAuthStorage(nil),
	)
	if err := registry.RegisterProvider(
		providerID,
		ProviderConfigInput{
			BaseURL:    "https://provider.test/v1",
			APIKey:     "configured-key",
			API:        "openai-completions",
			Headers:    map[string]string{"X-Old": "stale"},
			AuthHeader: modelRuntimeBoolPointer(true),
			Models: []ProviderModelDefinition{{
				ID:            "replace-model",
				Name:          "Replace Model",
				ContextWindow: 1000,
				MaxTokens:     100,
			}},
		},
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		registry.UnregisterProvider(providerID)
	})
	runtime, err := NewModelRuntimeFromRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := runtime.GetModel(providerID, "replace-model")
	if !ok {
		t.Fatal("registered model is missing")
	}
	initial, err := runtime.GetAuth(
		context.Background(),
		model,
		llm.AuthResolutionOverrides{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Headers["X-Old"] != "stale" ||
		initial.Headers["Authorization"] != "Bearer configured-key" {
		t.Fatalf("initial request projection = %#v", initial)
	}

	if err := runtime.RegisterProvider(
		providerID,
		ProviderConfigInput{
			Headers:    map[string]string{},
			AuthHeader: modelRuntimeBoolPointer(false),
		},
	); err != nil {
		t.Fatal(err)
	}
	effective, ok := runtime.GetRegisteredProviderConfig(providerID)
	if !ok || effective.Headers == nil ||
		len(effective.Headers) != 0 ||
		effective.AuthHeader == nil ||
		*effective.AuthHeader {
		t.Fatalf("effective provider declaration = %#v, %v", effective, ok)
	}
	model, ok = runtime.GetModel(providerID, "replace-model")
	if !ok {
		t.Fatal("re-registered model is missing")
	}
	replaced, err := runtime.GetAuth(
		context.Background(),
		model,
		llm.AuthResolutionOverrides{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Headers["X-Old"] != "" ||
		replaced.Headers["Authorization"] != "" {
		t.Fatalf("replaced request projection = %#v", replaced)
	}
}

func TestModelRuntimeOwnsExtensionProviderRegistrations(
	t *testing.T,
) {
	const (
		providerID = "instance-owned-provider"
		api        = "instance-owned-api"
	)
	t.Cleanup(func() {
		llm.UnregisterAPIProvider(api)
		UnregisterOAuthProvider(providerID)
	})
	registry := NewInMemoryModelRegistry(
		NewInMemoryAuthStorage(nil),
	)
	if err := registry.RegisterProvider(
		providerID,
		ProviderConfigInput{
			BaseURL: "https://provider.test/v1",
			APIKey:  "configured-key",
			API:     api,
			OAuth: &OAuthProvider{
				Name: "Instance OAuth",
			},
			StreamSimple: func(
				model llm.Model,
				_ llm.Context,
				_ llm.SimpleStreamOptions,
			) (*llm.AssistantMessageEventStream, error) {
				return llm.CompletedAssistantStream(
					llm.AssistantMessage(
						[]llm.ContentPart{llm.Text("instance")},
						llm.StopReasonStop,
						model,
					),
				), nil
			},
			Models: []ProviderModelDefinition{{
				ID:            "instance-model",
				Name:          "Instance Model",
				ContextWindow: 1000,
				MaxTokens:     100,
			}},
		},
	); err != nil {
		t.Fatal(err)
	}
	if llm.GetAPIProvider(api) == nil {
		t.Fatal("standalone registry did not install compatibility API")
	}
	if _, ok := GetOAuthProvider(providerID); !ok {
		t.Fatal("standalone registry did not install compatibility OAuth")
	}

	runtime, err := NewModelRuntimeFromRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	if llm.GetAPIProvider(api) != nil {
		t.Fatal("runtime promotion retained process-global API override")
	}
	if _, ok := GetOAuthProvider(providerID); ok {
		t.Fatal("runtime promotion retained process-global OAuth override")
	}
	provider, ok := runtime.GetProvider(providerID)
	if !ok || provider.Auth.OAuth == nil {
		t.Fatalf("instance provider = %#v, %v", provider, ok)
	}
	model, ok := runtime.GetModel(providerID, "instance-model")
	if !ok {
		t.Fatal("instance model is missing")
	}
	stream, err := provider.StreamSimple(
		model,
		llm.Context{},
		llm.SimpleStreamOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	message, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(message.Content) != 1 ||
		message.Content[0].Type != llm.ContentText ||
		message.Content[0].Text != "instance" {
		t.Fatalf("instance stream result = %#v", message)
	}
}

func TestModelRuntimeRuntimeAPIKeyRefreshesExtensionCatalog(
	t *testing.T,
) {
	const providerID = "runtime-key-provider"
	registry := NewInMemoryModelRegistry(
		NewInMemoryAuthStorage(nil),
	)
	runtime, err := NewModelRuntimeFromRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	var refreshCalls atomic.Int32
	if err := runtime.RegisterProvider(
		providerID,
		ProviderConfigInput{
			BaseURL: "https://provider.test/v1",
			API:     "openai-completions",
			RefreshModels: func(
				_ context.Context,
				input llm.RefreshModelsContext,
			) ([]ProviderModelDefinition, error) {
				refreshCalls.Add(1)
				if input.AllowNetwork {
					t.Fatal("runtime API key refresh allowed network")
				}
				if input.Credential == nil ||
					input.Credential.Type !=
						llm.CredentialTypeAPIKey ||
					input.Credential.Key != "runtime-key" {
					t.Fatalf(
						"refresh credential = %#v",
						input.Credential,
					)
				}
				return []ProviderModelDefinition{{
					ID:            "runtime-model",
					Name:          "Runtime Model",
					ContextWindow: 1000,
					MaxTokens:     100,
				}}, nil
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		runtime.UnregisterProvider(providerID)
	})
	provider, ok := runtime.GetProvider(providerID)
	if !ok || provider.Auth.APIKey == nil ||
		provider.Auth.OAuth != nil {
		t.Fatalf("unconfigured provider auth = %#v, %v", provider, ok)
	}
	if _, ok := runtime.GetModel(providerID, "runtime-model"); ok {
		t.Fatal("dynamic model existed before credential refresh")
	}

	if err := runtime.SetRuntimeAPIKey(
		context.Background(),
		providerID,
		"runtime-key",
	); err != nil {
		t.Fatal(err)
	}
	if refreshCalls.Load() != 1 {
		t.Fatalf("refresh calls = %d, want 1", refreshCalls.Load())
	}
	if _, ok := runtime.GetModel(
		providerID,
		"runtime-model",
	); !ok {
		t.Fatal("runtime key refresh did not publish dynamic model")
	}
}

type modelRuntimeCatalogDoerFunc func(
	*http.Request,
) (*http.Response, error)

func (f modelRuntimeCatalogDoerFunc) Do(
	request *http.Request,
) (*http.Response, error) {
	return f(request)
}

func modelRuntimeCatalogResponse(
	status int,
	body string,
	headers http.Header,
) *http.Response {
	if headers == nil {
		headers = http.Header{}
	}
	return &http.Response{
		StatusCode: status,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func modelRuntimeBoolPointer(value bool) *bool {
	return &value
}
