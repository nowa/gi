package gicodingagent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestCodingAgentPiRadiusExactCaseNames(t *testing.T) {
	t.Run("restores the legacy credential catalog without network access", func(t *testing.T) {
		auth := NewInMemoryAuthStorage(AuthStorageData{
			"radius": codingRadiusOAuthCredential(
				"https://radius.example.com/v1",
			),
		})
		registry := NewModelRegistryWithOptions(
			context.Background(),
			ModelRegistryOptions{
				AuthStorage: auth,
				ModelsStore: llm.NewInMemoryModelsStore(),
			},
		)

		model := registryMustFind(t, registry, "radius", "auto")
		if model.API != "pi-messages" ||
			model.BaseURL != "https://radius.example.com/v1" {
			t.Fatalf("model = %#v", model)
		}
		if registry.GetProviderDisplayName("radius") != "Radius" {
			t.Fatalf(
				"display name = %q",
				registry.GetProviderDisplayName("radius"),
			)
		}
		if !registry.HasConfiguredAuth(model) {
			t.Fatal("Radius auth is not configured")
		}
	})

	t.Run("fetches and stores the catalog for configured Radius auth", func(t *testing.T) {
		var authorization string
		client := codingRadiusDoerFunc(func(
			request *http.Request,
		) (*http.Response, error) {
			authorization = request.Header.Get("authorization")
			return codingRadiusConfigResponse(
				"https://radius.example.com/v1",
			), nil
		})
		store := llm.NewInMemoryModelsStore()
		auth := NewInMemoryAuthStorage(AuthStorageData{
			"radius": codingRadiusBareOAuthCredential(),
		})
		registry := NewModelRegistryWithOptions(
			context.Background(),
			ModelRegistryOptions{
				AuthStorage:       auth,
				ModelsStore:       store,
				RadiusClient:      client,
				AllowModelNetwork: true,
			},
		)

		registryMustFind(t, registry, "radius", "auto")
		stored, ok, err := store.ReadModels(
			context.Background(),
			"radius",
		)
		if err != nil || !ok || len(stored.Models) != 1 {
			t.Fatalf("stored = %#v, %v, %v", stored, ok, err)
		}
		if authorization != "Bearer access-token" {
			t.Fatalf("authorization = %q", authorization)
		}
	})

	t.Run("does not refresh catalogs over the network by default", func(t *testing.T) {
		var calls atomic.Int32
		client := codingRadiusDoerFunc(func(
			*http.Request,
		) (*http.Response, error) {
			calls.Add(1)
			return codingRadiusConfigResponse(
				"https://radius.example.com/v1",
			), nil
		})
		registry := NewModelRegistryWithOptions(
			context.Background(),
			ModelRegistryOptions{
				AuthStorage: NewInMemoryAuthStorage(AuthStorageData{
					"radius": codingRadiusOAuthCredential(
						"https://radius.example.com/v1",
					),
				}),
				ModelsStore:  llm.NewInMemoryModelsStore(),
				RadiusClient: client,
			},
		)

		registryMustFind(t, registry, "radius", "auto")
		if calls.Load() != 0 {
			t.Fatalf("network calls = %d", calls.Load())
		}
	})

	t.Run("does not fetch or expose Radius models without configured auth", func(t *testing.T) {
		var calls atomic.Int32
		client := codingRadiusDoerFunc(func(
			*http.Request,
		) (*http.Response, error) {
			calls.Add(1)
			return codingRadiusConfigResponse(
				"https://radius.example.com/v1",
			), nil
		})
		registry := NewModelRegistryWithOptions(
			context.Background(),
			ModelRegistryOptions{
				AuthStorage:       NewInMemoryAuthStorage(nil),
				ModelsStore:       llm.NewInMemoryModelsStore(),
				RadiusClient:      client,
				AllowModelNetwork: true,
			},
		)

		if models := registryModelsForProvider(registry, "radius"); len(models) != 0 {
			t.Fatalf("models = %#v", models)
		}
		if calls.Load() != 0 {
			t.Fatalf("network calls = %d", calls.Load())
		}
	})

	t.Run("supports custom Radius gateways from models.json", func(t *testing.T) {
		modelsPath := writeCodingRadiusModelsJSON(t, map[string]any{
			"radius-dev": map[string]any{
				"name":    "Radius (dev)",
				"baseUrl": "http://localhost:8788",
				"oauth":   "radius",
			},
		})
		client := codingRadiusDoerFunc(func(
			request *http.Request,
		) (*http.Response, error) {
			if request.URL.String() !=
				"http://localhost:8788/v1/config" {
				t.Fatalf("request URL = %q", request.URL)
			}
			return codingRadiusConfigResponse(
				"http://localhost:8788/v1",
			), nil
		})
		registry := NewModelRegistryWithOptions(
			context.Background(),
			ModelRegistryOptions{
				AuthStorage: NewInMemoryAuthStorage(AuthStorageData{
					"radius-dev": codingRadiusBareOAuthCredential(),
				}),
				ModelsJSONPath:    modelsPath,
				ModelsStore:       llm.NewInMemoryModelsStore(),
				RadiusClient:      client,
				AllowModelNetwork: true,
			},
		)

		model := registryMustFind(t, registry, "radius-dev", "auto")
		if model.API != "pi-messages" ||
			model.BaseURL != "http://localhost:8788/v1" {
			t.Fatalf("model = %#v", model)
		}
		if got := registry.GetProviderDisplayName("radius-dev"); got !=
			"Radius (dev)" {
			t.Fatalf("display name = %q", got)
		}
		auth := registry.GetAPIKeyAndHeaders(model)
		if !auth.OK || auth.APIKey != "access-token" {
			t.Fatalf("request auth = %#v", auth)
		}
	})

	t.Run("keeps models.json overrides above a refreshed Radius catalog", func(t *testing.T) {
		modelsPath := writeCodingRadiusModelsJSON(t, map[string]any{
			"radius-dev": map[string]any{
				"name":    "Radius (dev)",
				"baseUrl": "http://localhost:8788",
				"oauth":   "radius",
				"models": []any{map[string]any{
					"id":        "auto",
					"name":      "Configured Base",
					"maxTokens": 4096,
				}},
				"modelOverrides": map[string]any{
					"auto": map[string]any{
						"name":      "Configured Auto",
						"maxTokens": 2048,
					},
				},
			},
		})
		client := codingRadiusDoerFunc(func(
			*http.Request,
		) (*http.Response, error) {
			return codingRadiusConfigResponse(
				"http://localhost:8788/v1",
			), nil
		})
		registry := NewModelRegistryWithOptions(
			context.Background(),
			ModelRegistryOptions{
				AuthStorage: NewInMemoryAuthStorage(AuthStorageData{
					"radius-dev": codingRadiusBareOAuthCredential(),
				}),
				ModelsJSONPath:    modelsPath,
				ModelsStore:       llm.NewInMemoryModelsStore(),
				RadiusClient:      client,
				AllowModelNetwork: true,
			},
		)

		model := registryMustFind(t, registry, "radius-dev", "auto")
		if model.Name != "Configured Auto" ||
			model.MaxTokens != 2048 ||
			model.API != "pi-messages" ||
			model.BaseURL != "http://localhost:8788" {
			t.Fatalf("configured model = %#v", model)
		}
	})

	t.Run("requires baseUrl for custom Radius gateways", func(t *testing.T) {
		modelsPath := writeCodingRadiusModelsJSON(t, map[string]any{
			"radius-dev": map[string]any{"oauth": "radius"},
		})
		registry := NewModelRegistryWithOptions(
			context.Background(),
			ModelRegistryOptions{
				AuthStorage:    NewInMemoryAuthStorage(nil),
				ModelsJSONPath: modelsPath,
				ModelsStore:    llm.NewInMemoryModelsStore(),
			},
		)
		if !strings.Contains(
			registry.GetError(),
			`"baseUrl" is required when "oauth" is set`,
		) {
			t.Fatalf("error = %q", registry.GetError())
		}
	})
}

func codingRadiusOAuthCredential(baseURL string) AuthCredential {
	credential := codingRadiusBareOAuthCredential()
	credential.Metadata = map[string]any{
		"gatewayConfig": codingRadiusGatewayConfig(baseURL),
	}
	return credential
}

func codingRadiusBareOAuthCredential() AuthCredential {
	return AuthCredential{
		Type:    llm.CredentialTypeOAuth,
		Access:  "access-token",
		Refresh: "refresh-token",
		Expires: time.Now().Add(time.Hour).UnixMilli(),
	}
}

func codingRadiusGatewayConfig(baseURL string) llm.RadiusGatewayConfig {
	return llm.RadiusGatewayConfig{
		BaseURL: baseURL,
		Models: []llm.RadiusGatewayModel{{
			ID:    "auto",
			Name:  "Radius Auto",
			Input: []string{"text"},
			Cost: llm.ModelCost{
				Input:      1,
				Output:     2,
				CacheRead:  0.1,
				CacheWrite: 0.2,
			},
			ContextWindow: 128000,
			MaxTokens:     16384,
		}},
	}
}

func codingRadiusConfigResponse(baseURL string) *http.Response {
	body := `{"baseUrl":` + mustJSONQuote(baseURL) + `,"models":[{` +
		`"id":"auto","name":"Radius Auto","reasoning":false,` +
		`"input":["text"],"cost":{"input":1,"output":2,` +
		`"cacheRead":0.1,"cacheWrite":0.2},` +
		`"contextWindow":128000,"maxTokens":16384}]}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func writeCodingRadiusModelsJSON(
	t *testing.T,
	providers map[string]any,
) string {
	t.Helper()
	path := t.TempDir() + "/models.json"
	writeRawModelsJSON(t, path, map[string]any{"providers": providers})
	return path
}

func mustJSONQuote(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
	)
	return `"` + replacer.Replace(value) + `"`
}

type codingRadiusDoerFunc func(*http.Request) (*http.Response, error)

func (f codingRadiusDoerFunc) Do(
	request *http.Request,
) (*http.Response, error) {
	return f(request)
}
