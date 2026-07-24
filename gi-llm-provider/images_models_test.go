package gillmprovider

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

func TestImagesModelsRegistersProvidersInOrder(t *testing.T) {
	models := NewImagesModels()
	first := newTestImagesProvider(t, "p1", []ImagesModel{
		testImagesModel("p1", "m1"),
		testImagesModel("p1", "m2"),
	}, nil, nil)
	second := newTestImagesProvider(t, "p2", []ImagesModel{
		testImagesModel("p2", "m3"),
	}, nil, nil)
	if err := models.SetProvider(first); err != nil {
		t.Fatal(err)
	}
	if err := models.SetProvider(second); err != nil {
		t.Fatal(err)
	}

	if got := imageProviderIDs(models.GetProviders()); !slices.Equal(got, []string{"p1", "p2"}) {
		t.Fatalf("provider IDs = %v", got)
	}
	if got := imageModelIDs(models.GetModels()); !slices.Equal(got, []string{"m1", "m2", "m3"}) {
		t.Fatalf("model IDs = %v", got)
	}
	if got := imageModelIDs(models.GetModels("p1")); !slices.Equal(got, []string{"m1", "m2"}) {
		t.Fatalf("p1 model IDs = %v", got)
	}
	if model, ok := models.GetModel("p2", "m3"); !ok || model.ID != "m3" {
		t.Fatalf("GetModel() = %#v, %v", model, ok)
	}
	if _, ok := models.GetModel("p2", "missing"); ok {
		t.Fatal("missing model was found")
	}

	replacement := newTestImagesProvider(t, "p1", []ImagesModel{
		testImagesModel("p1", "replacement"),
	}, nil, nil)
	if err := models.SetProvider(replacement); err != nil {
		t.Fatal(err)
	}
	if got := imageProviderIDs(models.GetProviders()); !slices.Equal(got, []string{"p1", "p2"}) {
		t.Fatalf("provider order after replacement = %v", got)
	}

	models.DeleteProvider("p1")
	if _, ok := models.GetProvider("p1"); ok {
		t.Fatal("deleted provider was found")
	}
	models.ClearProviders()
	if got := models.GetProviders(); len(got) != 0 {
		t.Fatalf("providers after clear = %#v", got)
	}
}

func TestImagesModelsResolvesAndMergesRequestAuth(t *testing.T) {
	var capturedModel ImagesModel
	var capturedOptions ImagesOptions
	provider, err := CreateImagesProvider(CreateImagesProviderOptions{
		ID: "p1",
		Auth: testImagesAPIKeyAuth(
			"TEST_IMAGES_KEY",
			ModelAuth{
				Headers: map[string]string{
					"X-Provider": "provider",
					"X-Shared":   "provider",
				},
				HeaderRemovals: []string{"X-Removed", "X-Request"},
				BaseURL:        "https://resolved.example/v1",
			},
			ProviderEnv{
				"PROVIDER_ONLY": "provider",
				"SHARED":        "provider",
			},
		),
		Models: []ImagesModel{testImagesModel("p1", "model-a")},
		API: ImagesAPIProviderFuncs{GenerateImagesFunc: func(
			model ImagesModel,
			_ ImagesContext,
			options ImagesOptions,
		) (AssistantImages, error) {
			capturedModel = model
			capturedOptions = options
			return testImagesResult(model), nil
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	models := NewImagesModels(ImagesModelsOptions{
		AuthContext: testImagesAuthContext(map[string]string{
			"TEST_IMAGES_KEY": "environment-key",
		}),
	})
	if err := models.SetProvider(provider); err != nil {
		t.Fatal(err)
	}
	model := models.GetModels("p1")[0]

	auth, err := models.GetModelAuth(
		context.Background(),
		model,
		AuthResolutionOverrides{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if auth == nil || auth.Auth.APIKey != "environment-key" {
		t.Fatalf("resolved auth = %#v", auth)
	}
	explicit := "explicit-auth"
	auth, err = models.GetAuth(
		context.Background(),
		"p1",
		AuthResolutionOverrides{APIKey: &explicit},
	)
	if err != nil {
		t.Fatal(err)
	}
	if auth == nil || auth.Auth.APIKey != explicit {
		t.Fatalf("explicit auth = %#v", auth)
	}

	result := models.GenerateImages(
		context.Background(),
		model,
		ImagesContext{Input: []ImagesContent{ImageText("a red circle")}},
		ImagesOptions{
			APIKey: "request-key",
			Headers: map[string]string{
				"x-shared":  "request",
				"X-Request": "request",
			},
			HeaderRemovals: []string{"X-Provider"},
			Env: ProviderEnv{
				"REQUEST_ONLY": "request",
				"SHARED":       "request",
			},
		},
	)
	if result.StopReason != ImagesStopReasonStop {
		t.Fatalf("result = %#v", result)
	}
	if capturedModel.BaseURL != "https://resolved.example/v1" {
		t.Fatalf("request base URL = %q", capturedModel.BaseURL)
	}
	if capturedOptions.APIKey != "request-key" {
		t.Fatalf("request API key = %q", capturedOptions.APIKey)
	}
	wantHeaders := map[string]string{
		"X-Provider": "provider",
		"x-shared":   "request",
		"X-Request":  "request",
	}
	if !maps.Equal(capturedOptions.Headers, wantHeaders) {
		t.Fatalf("request headers = %#v, want %#v", capturedOptions.Headers, wantHeaders)
	}
	wantEnv := ProviderEnv{
		"PROVIDER_ONLY": "provider",
		"REQUEST_ONLY":  "request",
		"SHARED":        "request",
	}
	if !maps.Equal(capturedOptions.Env, wantEnv) {
		t.Fatalf("request env = %#v, want %#v", capturedOptions.Env, wantEnv)
	}
	if !slices.Equal(
		capturedOptions.HeaderRemovals,
		[]string{"X-Removed", "X-Provider"},
	) {
		t.Fatalf("request header removals = %#v", capturedOptions.HeaderRemovals)
	}

	explicitEmpty := ""
	models.GenerateImages(
		context.Background(),
		model,
		ImagesContext{},
		ImagesOptions{APIKeyOverride: &explicitEmpty},
	)
	if capturedOptions.APIKey != "" {
		t.Fatalf("explicit empty API key = %q", capturedOptions.APIKey)
	}
}

func TestImagesModelsUnknownAndUnconfiguredProviders(t *testing.T) {
	models := NewImagesModels(ImagesModelsOptions{
		AuthContext: testImagesAuthContext(nil),
	})
	unknown := models.GenerateImages(
		context.Background(),
		testImagesModel("ghost", "missing"),
		ImagesContext{},
		ImagesOptions{},
	)
	if unknown.StopReason != ImagesStopReasonError ||
		unknown.ErrorMessage != "Unknown provider: ghost" {
		t.Fatalf("unknown provider result = %#v", unknown)
	}

	var called atomic.Bool
	provider := newTestImagesProvider(
		t,
		"p1",
		[]ImagesModel{testImagesModel("p1", "model-a")},
		&APIKeyAuth{
			Name: "missing key",
			Resolve: func(
				context.Context,
				APIKeyResolveInput,
			) (*AuthResult, error) {
				return nil, nil
			},
		},
		func(model ImagesModel, _ ImagesContext, options ImagesOptions) (AssistantImages, error) {
			called.Store(true)
			if options.APIKey != "" {
				t.Fatalf("unconfigured API key = %q", options.APIKey)
			}
			if options.APIKeyOverride == nil || *options.APIKeyOverride != "" {
				t.Fatalf("assembled API key override = %#v", options.APIKeyOverride)
			}
			return testImagesResult(model), nil
		},
	)
	if err := models.SetProvider(provider); err != nil {
		t.Fatal(err)
	}
	model := models.GetModels("p1")[0]
	if auth, err := models.GetModelAuth(
		context.Background(),
		model,
		AuthResolutionOverrides{},
	); err != nil || auth != nil {
		t.Fatalf("unconfigured auth = %#v, %v", auth, err)
	}
	result := models.GenerateImages(
		context.Background(),
		model,
		ImagesContext{},
		ImagesOptions{},
	)
	if !called.Load() || result.StopReason != ImagesStopReasonStop {
		t.Fatalf("unconfigured dispatch = %v, %#v", called.Load(), result)
	}
}

func TestImagesProviderRefreshCoalescesAndPreservesLastSnapshot(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var fetches atomic.Int32
	var failRefresh atomic.Bool
	refreshError := errors.New("fetch failed")
	provider, err := CreateImagesProvider(CreateImagesProviderOptions{
		ID:     "dynamic",
		Auth:   testImagesAPIKeyAuth("", ModelAuth{}, nil),
		Models: nil,
		FetchModels: func(ctx context.Context) ([]ImagesModel, error) {
			if fetches.Add(1) == 1 {
				close(started)
			}
			if failRefresh.Load() {
				return nil, refreshError
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-release:
				return []ImagesModel{testImagesModel("dynamic", "listed")}, nil
			}
		},
		API: ImagesAPIProviderFuncs{GenerateImagesFunc: func(
			model ImagesModel,
			_ ImagesContext,
			_ ImagesOptions,
		) (AssistantImages, error) {
			return testImagesResult(model), nil
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	models := NewImagesModels()
	if err := models.SetProvider(provider); err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- models.Refresh(context.Background(), "dynamic")
	}()
	<-started
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- models.Refresh(context.Background(), "dynamic")
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("coalesced refresh returned before fetch completed: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	if fetches.Load() != 1 {
		t.Fatalf("fetches = %d, want 1", fetches.Load())
	}
	if model, ok := models.GetModel("dynamic", "listed"); !ok || model.ID != "listed" {
		t.Fatalf("refreshed model = %#v, %v", model, ok)
	}

	failRefresh.Store(true)
	err = models.Refresh(context.Background(), "dynamic")
	var modelsError *ModelsError
	if !errors.As(err, &modelsError) ||
		modelsError.Code != ModelsErrorModelSource ||
		!errors.Is(err, refreshError) {
		t.Fatalf("targeted refresh error = %#v", err)
	}
	if model, ok := models.GetModel("dynamic", "listed"); !ok || model.ID != "listed" {
		t.Fatalf("model after failed refresh = %#v, %v", model, ok)
	}
	if err := models.Refresh(context.Background()); err != nil {
		t.Fatalf("best-effort refresh error = %v", err)
	}
}

func TestBuiltinImagesModelsSharesOpenRouterCredentialsWithTextModels(t *testing.T) {
	store := NewInMemoryCredentialStore(map[string]Credential{
		"openrouter": {
			Type:    CredentialTypeOAuth,
			Access:  "shared-oauth-key",
			Expires: permanentOAuthCredentialExpires,
		},
	})
	options := ModelsOptions{
		Credentials: store,
		AuthContext: testImagesAuthContext(map[string]string{
			"OPENROUTER_API_KEY": "ambient-key",
		}),
	}
	textModels, err := BuiltinModels(options)
	if err != nil {
		t.Fatal(err)
	}
	imageModels, err := BuiltinImagesModels(options)
	if err != nil {
		t.Fatal(err)
	}

	if got := imageProviderIDs(imageModels.GetProviders()); !slices.Equal(got, []string{"openrouter"}) {
		t.Fatalf("image providers = %v", got)
	}
	catalog := imageModels.GetModels("openrouter")
	if got := imageModelIDs(catalog); !slices.Equal(got, []string{
		"black-forest-labs/flux.2-pro",
		"google/gemini-2.5-flash-image",
		"google/gemini-3.1-flash-image-preview",
	}) {
		t.Fatalf("image catalog = %v", got)
	}
	for _, model := range catalog {
		if model.API != "openrouter-images" {
			t.Fatalf("image model API = %q", model.API)
		}
	}

	textAuth, err := textModels.GetAuth(
		context.Background(),
		"openrouter",
		AuthResolutionOverrides{},
	)
	if err != nil {
		t.Fatal(err)
	}
	imageAuth, err := imageModels.GetAuth(
		context.Background(),
		"openrouter",
		AuthResolutionOverrides{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if textAuth == nil || imageAuth == nil ||
		textAuth.Auth.APIKey != "shared-oauth-key" ||
		imageAuth.Auth.APIKey != textAuth.Auth.APIKey ||
		textAuth.Source != "OAuth" ||
		imageAuth.Source != "OAuth" {
		t.Fatalf("shared auth = text %#v, image %#v", textAuth, imageAuth)
	}
}

func newTestImagesProvider(
	t *testing.T,
	id string,
	models []ImagesModel,
	apiKeyAuth *APIKeyAuth,
	generate func(ImagesModel, ImagesContext, ImagesOptions) (AssistantImages, error),
) *ImagesProvider {
	t.Helper()
	if apiKeyAuth == nil {
		auth := testImagesAPIKeyAuth("", ModelAuth{}, nil)
		apiKeyAuth = auth.APIKey
	}
	if generate == nil {
		generate = func(
			model ImagesModel,
			_ ImagesContext,
			_ ImagesOptions,
		) (AssistantImages, error) {
			return testImagesResult(model), nil
		}
	}
	provider, err := CreateImagesProvider(CreateImagesProviderOptions{
		ID:     id,
		Auth:   ProviderAuth{APIKey: apiKeyAuth},
		Models: models,
		API:    ImagesAPIProviderFuncs{GenerateImagesFunc: generate},
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func testImagesAPIKeyAuth(
	envName string,
	modelAuth ModelAuth,
	env ProviderEnv,
) ProviderAuth {
	return ProviderAuth{APIKey: &APIKeyAuth{
		Name: "Test image key",
		Resolve: func(
			ctx context.Context,
			input APIKeyResolveInput,
		) (*AuthResult, error) {
			key := ""
			source := ""
			if input.Credential != nil {
				key = input.Credential.Key
				source = "stored"
			} else if envName != "" {
				var ok bool
				var err error
				key, ok, err = input.Context.Env(ctx, envName)
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, nil
				}
				source = envName
			}
			auth := cloneModelAuth(modelAuth)
			auth.APIKey = key
			return &AuthResult{
				Auth:   auth,
				Env:    cloneProviderEnv(env),
				Source: source,
			}, nil
		},
	}}
}

func testImagesAuthContext(env map[string]string) AuthContext {
	return AuthContextFuncs{
		EnvFunc: func(
			ctx context.Context,
			name string,
		) (string, bool, error) {
			if err := ctx.Err(); err != nil {
				return "", false, err
			}
			value, ok := env[name]
			return value, ok, nil
		},
		FileExistsFunc: func(context.Context, string) (bool, error) {
			return false, nil
		},
	}
}

func testImagesModel(provider, id string) ImagesModel {
	return ImagesModel{
		ID:       id,
		Name:     id,
		API:      "test-images",
		Provider: provider,
		BaseURL:  "https://example.test/v1",
		Input:    []string{"text"},
		Output:   []string{"image"},
	}
}

func testImagesResult(model ImagesModel) AssistantImages {
	return AssistantImages{
		API:        model.API,
		Provider:   model.Provider,
		Model:      model.ID,
		Output:     []ImagesContent{ImageData("aGk=", "image/png")},
		StopReason: ImagesStopReasonStop,
		Timestamp:  NowMillis(),
	}
}

func imageProviderIDs(providers []*ImagesProvider) []string {
	ids := make([]string, 0, len(providers))
	for _, provider := range providers {
		ids = append(ids, provider.ID)
	}
	return ids
}

func imageModelIDs(models []ImagesModel) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}
