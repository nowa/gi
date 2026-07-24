package gillmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRadiusProviderPiContracts(t *testing.T) {
	t.Run("restores the legacy credential catalog without network access", func(t *testing.T) {
		credential := radiusTestOAuthCredential(
			"https://radius.example.com/v1",
		)
		modelsStore := NewInMemoryModelsStore()
		models := NewModels(ModelsOptions{
			Credentials: NewInMemoryCredentialStore(map[string]Credential{
				"radius": credential,
			}),
			ModelsStore: modelsStore,
			AuthContext: providerAuthContext(nil),
		})
		provider, err := NewRadiusProvider(RadiusProviderOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if err := models.SetProvider(provider); err != nil {
			t.Fatal(err)
		}

		result := models.Refresh(
			context.Background(),
			ModelsRefreshOptions{Offline: true},
		)
		if result.Aborted || len(result.Errors) != 0 {
			t.Fatalf("refresh result = %#v", result)
		}
		model, ok := models.GetModel("radius", "auto")
		if !ok ||
			model.API != piMessagesAPI ||
			model.BaseURL != "https://radius.example.com/v1" {
			t.Fatalf("restored model = %#v, found=%v", model, ok)
		}
		stored, ok, err := modelsStore.ReadModels(
			context.Background(),
			"radius",
		)
		if err != nil || !ok || len(stored.Models) != 1 {
			t.Fatalf("stored catalog = %#v, found=%v, err=%v", stored, ok, err)
		}
	})

	t.Run("fetches and stores the catalog for configured Radius auth", func(t *testing.T) {
		var authorization string
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			authorization = request.Header.Get("Authorization")
			if request.URL.Path != "/v1/config" {
				t.Errorf("config path = %q", request.URL.Path)
			}
			writer.Header().Set("content-type", "application/json")
			_ = json.NewEncoder(writer).Encode(
				radiusTestConfig(serverURLFromRequest(request) + "/v1"),
			)
		}))
		t.Cleanup(server.Close)

		modelsStore := NewInMemoryModelsStore()
		models := NewModels(ModelsOptions{
			Credentials: NewInMemoryCredentialStore(map[string]Credential{
				"radius": {
					Type:    CredentialTypeOAuth,
					Access:  "access-token",
					Refresh: "refresh-token",
					Expires: time.Now().Add(time.Hour).UnixMilli(),
				},
			}),
			ModelsStore: modelsStore,
			AuthContext: providerAuthContext(nil),
		})
		provider, err := NewRadiusProvider(RadiusProviderOptions{
			Gateway: server.URL,
			Client:  server.Client(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := models.SetProvider(provider); err != nil {
			t.Fatal(err)
		}
		result := models.Refresh(context.Background(), ModelsRefreshOptions{})
		if result.Aborted || len(result.Errors) != 0 {
			t.Fatalf("refresh result = %#v", result)
		}
		if authorization != "Bearer access-token" {
			t.Fatalf("authorization = %q", authorization)
		}
		if _, ok := models.GetModel("radius", "auto"); !ok {
			t.Fatal("refreshed Radius model is missing")
		}
		stored, ok, err := modelsStore.ReadModels(
			context.Background(),
			"radius",
		)
		if err != nil || !ok || len(stored.Models) != 1 ||
			stored.CheckedAt == 0 {
			t.Fatalf("stored catalog = %#v, found=%v, err=%v", stored, ok, err)
		}
	})

	t.Run("does not refresh catalogs over the network by default", func(t *testing.T) {
		var requests atomic.Int32
		client := radiusHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			requests.Add(1)
			return nil, errors.New("unexpected network request")
		})
		models := NewModels(ModelsOptions{
			Credentials: NewInMemoryCredentialStore(map[string]Credential{
				"radius": radiusTestOAuthCredential(
					"https://radius.example.com/v1",
				),
			}),
			ModelsStore: NewInMemoryModelsStore(),
			AuthContext: providerAuthContext(nil),
		})
		provider, err := NewRadiusProvider(RadiusProviderOptions{
			Client: client,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := models.SetProvider(provider); err != nil {
			t.Fatal(err)
		}
		result := models.Refresh(
			context.Background(),
			ModelsRefreshOptions{Offline: true},
		)
		if len(result.Errors) != 0 || requests.Load() != 0 {
			t.Fatalf("refresh=%#v requests=%d", result, requests.Load())
		}
		if _, ok := models.GetModel("radius", "auto"); !ok {
			t.Fatal("offline legacy model is missing")
		}
	})

	t.Run("does not fetch or expose Radius models without configured auth", func(t *testing.T) {
		var requests atomic.Int32
		client := radiusHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			requests.Add(1)
			return nil, errors.New("unexpected network request")
		})
		models := NewModels(ModelsOptions{
			Credentials: NewInMemoryCredentialStore(),
			ModelsStore: NewInMemoryModelsStore(),
			AuthContext: providerAuthContext(nil),
		})
		provider, err := NewRadiusProvider(RadiusProviderOptions{
			Client: client,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := models.SetProvider(provider); err != nil {
			t.Fatal(err)
		}
		result := models.Refresh(context.Background(), ModelsRefreshOptions{})
		if len(result.Errors) != 0 ||
			requests.Load() != 0 ||
			len(models.GetModels("radius")) != 0 {
			t.Fatalf(
				"refresh=%#v requests=%d models=%#v",
				result,
				requests.Load(),
				models.GetModels("radius"),
			)
		}
	})

	t.Run("supports custom Radius gateways from models.json", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			requests.Add(1)
			_ = json.NewEncoder(writer).Encode(
				radiusTestConfig(serverURLFromRequest(request) + "/v1"),
			)
		}))
		t.Cleanup(server.Close)
		provider, err := NewRadiusProvider(RadiusProviderOptions{
			ID:      "radius-dev",
			Name:    "Radius (dev)",
			Gateway: server.URL + "/",
			Client:  server.Client(),
		})
		if err != nil {
			t.Fatal(err)
		}
		store := scopedModelsStore{
			store:      NewInMemoryModelsStore(),
			providerID: "radius-dev",
		}
		if err := provider.RefreshModels(
			context.Background(),
			RefreshModelsContext{
				Credential: &Credential{
					Type:   CredentialTypeOAuth,
					Access: "access-token",
				},
				Store:        store,
				AllowNetwork: true,
			},
		); err != nil {
			t.Fatal(err)
		}
		list, err := provider.GetModels()
		if err != nil {
			t.Fatal(err)
		}
		if provider.Name != "Radius (dev)" ||
			requests.Load() != 1 ||
			len(list) != 1 ||
			list[0].Provider != "radius-dev" ||
			list[0].API != piMessagesAPI {
			t.Fatalf(
				"provider=%#v requests=%d models=%#v",
				provider,
				requests.Load(),
				list,
			)
		}
	})
}

func TestRadiusConfigValidationAndLoading(t *testing.T) {
	t.Run("normalizes Radius gateway URLs", func(t *testing.T) {
		cases := map[string]string{
			"radius.example.com/":         "https://radius.example.com",
			"http://localhost:8788///":    "http://localhost:8788",
			"HTTPS://radius.example.com/": "HTTPS://radius.example.com",
		}
		for input, want := range cases {
			if got := NormalizeRadiusGatewayURL(input); got != want {
				t.Fatalf(
					"NormalizeRadiusGatewayURL(%q) = %q, want %q",
					input,
					got,
					want,
				)
			}
		}
	})

	t.Run("filters malformed models while preserving a valid config", func(t *testing.T) {
		value := map[string]any{
			"baseUrl": "https://radius.example.com/v1",
			"models": []any{
				map[string]any{
					"id":            "auto",
					"name":          "Radius Auto",
					"reasoning":     false,
					"input":         []any{"text"},
					"cost":          map[string]any{},
					"contextWindow": 128000,
					"maxTokens":     16384,
				},
				map[string]any{"id": "invalid"},
			},
		}
		config, ok := sanitizeRadiusGatewayConfig(value)
		if !ok || len(config.Models) != 1 || config.Models[0].ID != "auto" {
			t.Fatalf("sanitized config = %#v, valid=%v", config, ok)
		}
		models := GetRadiusModelsFromConfig("radius", config)
		if len(models) != 1 ||
			models[0].Provider != "radius" ||
			models[0].API != piMessagesAPI ||
			models[0].BaseURL != config.BaseURL {
			t.Fatalf("Radius models = %#v", models)
		}
	})

	t.Run("reports bounded HTTP errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			_ *http.Request,
		) {
			writer.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(writer, strings.Repeat("x", 600))
		}))
		t.Cleanup(server.Close)
		_, err := LoadRadiusGatewayConfig(
			context.Background(),
			server.Client(),
			server.URL,
			"",
		)
		if err == nil ||
			!strings.Contains(err.Error(), "502") ||
			!strings.Contains(err.Error(), "…") ||
			len([]rune(err.Error())) > 700 {
			t.Fatalf("HTTP error = %v", err)
		}
	})

}

func TestRadiusRefreshPublishesOnlyAfterPersistenceSucceeds(t *testing.T) {
	config := radiusTestConfig("https://radius.example.com/v1")
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		_ = json.NewEncoder(writer).Encode(config)
	}))
	t.Cleanup(server.Close)
	provider, err := NewRadiusProvider(RadiusProviderOptions{
		Gateway: server.URL,
		Client:  server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &failingRadiusModelsStore{err: errors.New("persist failed")}
	err = provider.RefreshModels(
		context.Background(),
		RefreshModelsContext{
			Credential:   &Credential{Type: CredentialTypeAPIKey, Key: "key"},
			Store:        store,
			AllowNetwork: true,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "persist failed") {
		t.Fatalf("refresh error = %v", err)
	}
	models, err := provider.GetModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 0 {
		t.Fatalf("models published before persistence: %#v", models)
	}
}

func TestRadiusRefreshCoalescesConcurrentNetworkWork(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	client := radiusHTTPDoerFunc(func(
		request *http.Request,
	) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			body, err := json.Marshal(
				radiusTestConfig("https://radius.example.com/v1"),
			)
			if err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(string(body))),
			}, nil
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
	})
	provider, err := NewRadiusProvider(RadiusProviderOptions{
		Client: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := scopedModelsStore{
		store:      NewInMemoryModelsStore(),
		providerID: "radius",
	}
	input := RefreshModelsContext{
		Credential: &Credential{
			Type:   CredentialTypeOAuth,
			Access: "access-token",
		},
		Store:        store,
		AllowNetwork: true,
	}

	firstResult := make(chan error, 1)
	go func() {
		firstResult <- provider.RefreshModels(
			context.Background(),
			input,
		)
	}()
	<-started

	waiterContext, cancel := context.WithCancel(context.Background())
	cancel()
	if err := provider.RefreshModels(waiterContext, input); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf("waiting refresh error = %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("network calls while refresh is in flight = %d, want 1", got)
	}

	close(release)
	if err := <-firstResult; err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("network calls = %d, want 1", got)
	}
}

func radiusTestConfig(baseURL string) RadiusGatewayConfig {
	return RadiusGatewayConfig{
		BaseURL: baseURL,
		Models: []RadiusGatewayModel{{
			ID:            "auto",
			Name:          "Radius Auto",
			Input:         []string{"text"},
			Cost:          ModelCost{Input: 1, Output: 2, CacheRead: 0.1, CacheWrite: 0.2},
			ContextWindow: 128000,
			MaxTokens:     16384,
		}},
	}
}

func radiusTestOAuthCredential(baseURL string) Credential {
	return Credential{
		Type:    CredentialTypeOAuth,
		Access:  "access-token",
		Refresh: "refresh-token",
		Expires: time.Now().Add(time.Hour).UnixMilli(),
		Metadata: map[string]any{
			"gatewayConfig": radiusTestConfig(baseURL),
		},
	}
}

func serverURLFromRequest(request *http.Request) string {
	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + request.Host
}

type failingRadiusModelsStore struct {
	err error
}

type radiusHTTPDoerFunc func(*http.Request) (*http.Response, error)

func (f radiusHTTPDoerFunc) Do(
	request *http.Request,
) (*http.Response, error) {
	return f(request)
}

func (*failingRadiusModelsStore) ReadModels(
	context.Context,
) (ModelsStoreEntry, bool, error) {
	return ModelsStoreEntry{}, false, nil
}

func (s *failingRadiusModelsStore) WriteModels(
	context.Context,
	ModelsStoreEntry,
) error {
	return s.err
}

func (*failingRadiusModelsStore) DeleteModels(context.Context) error {
	return nil
}

func TestRadiusGatewayConfigRoundTrip(t *testing.T) {
	config := radiusTestConfig("https://radius.example.com/v1")
	credential := Credential{
		Type: CredentialTypeOAuth,
		Metadata: map[string]any{
			"gatewayConfig": config,
		},
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Credential
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	got, ok := GetRadiusCredentialConfig(&decoded)
	if !ok || !reflect.DeepEqual(got, config) {
		t.Fatalf("round-trip config = %#v, valid=%v", got, ok)
	}
}
