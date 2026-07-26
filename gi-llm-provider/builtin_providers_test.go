package gillmprovider

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBuiltinProviderPiContracts(t *testing.T) {
	t.Run("builtinModels registers every builtin provider with models", func(t *testing.T) {
		models, err := BuiltinModels()
		if err != nil {
			t.Fatal(err)
		}
		providers := models.GetProviders()
		builtins, err := BuiltinProviders()
		if err != nil {
			t.Fatal(err)
		}
		if len(providers) != len(builtins) {
			t.Fatalf("provider count = %d, want %d", len(providers), len(builtins))
		}
		if _, ok := models.GetProvider("anthropic"); !ok {
			t.Fatal("anthropic provider is not registered")
		}

		anthropic, ok := models.GetModel("anthropic", "claude-haiku-4-5")
		if !ok || anthropic.API != "anthropic-messages" {
			t.Fatalf("anthropic model = %#v, found=%v", anthropic, ok)
		}
		if got := len(models.GetModels()); got <= 500 {
			t.Fatalf("model count = %d, want more than 500", got)
		}

		for _, provider := range providers {
			list := models.GetModels(provider.ID)
			if provider.ID == "radius" {
				if len(list) != 0 {
					t.Fatalf("radius has %d static models, want 0", len(list))
				}
				continue
			}
			if len(list) == 0 {
				t.Fatalf("provider %s has no static models", provider.ID)
			}
			for _, model := range list {
				if model.Provider != provider.ID {
					t.Fatalf("provider %s returned model owned by %s", provider.ID, model.Provider)
				}
			}
		}
	})

	t.Run("keeps the compiled builtin catalog isolated from compatibility registrations", func(t *testing.T) {
		builtin := requireBuiltinModel(t, "openai", "gpt-4o")
		compatibility, ok := GetModel("openai", "gpt-4o")
		if !ok {
			t.Fatal("compatibility model is missing")
		}
		t.Cleanup(func() { RegisterModel(compatibility) })

		replacement := cloneModel(compatibility)
		replacement.Name = "Runtime override"
		RegisterModel(replacement)
		if got, _ := GetModel("openai", "gpt-4o"); got.Name != "Runtime override" {
			t.Fatalf("compatibility registration = %#v", got)
		}
		if got := requireBuiltinModel(t, "openai", "gpt-4o"); got.Name != builtin.Name {
			t.Fatalf("compiled catalog was mutated: %#v", got)
		}

		builtin.Input[0] = "caller mutation"
		if got := requireBuiltinModel(t, "openai", "gpt-4o"); got.Input[0] == "caller mutation" {
			t.Fatal("compiled catalog returned shared mutable state")
		}
		wantGeneratedAt := time.Date(2026, time.July, 25, 12, 44, 19, 521000000, time.UTC)
		if got := GetBuiltinModelDataGeneratedAt(); !got.Equal(wantGeneratedAt) {
			t.Fatalf("catalog generated at = %s, want %s", got, wantGeneratedAt)
		}
	})

	t.Run("stores native constrained-sampling capabilities in model metadata", func(t *testing.T) {
		gpt4o := requireBuiltinModel(t, "openai", "gpt-4o")
		if !boolValue(gpt4o.Compat.SupportsStrictMode) {
			t.Fatal("gpt-4o strict mode is not enabled")
		}
		if gpt4o.Compat.SupportsOpenAIGrammarTools != nil {
			t.Fatal("gpt-4o unexpectedly declares native grammar tools")
		}
		gpt54 := requireBuiltinModel(t, "openai", "gpt-5.4")
		if !boolValue(gpt54.Compat.SupportsStrictMode) ||
			!boolValue(gpt54.Compat.SupportsOpenAIGrammarTools) {
			t.Fatalf("gpt-5.4 compatibility = %#v", gpt54.Compat)
		}
		haiku := requireBuiltinModel(t, "anthropic", "claude-haiku-4-5")
		if !boolValue(haiku.Compat.SupportsStrictTools) {
			t.Fatalf("claude-haiku-4-5 compatibility = %#v", haiku.Compat)
		}
	})

	t.Run("uses official Kimi K3 pricing for Moonshot providers", func(t *testing.T) {
		want := ModelCost{Input: 3, Output: 15, CacheRead: 0.3, CacheWrite: 0}
		for _, providerID := range []string{"moonshotai", "moonshotai-cn"} {
			model := requireBuiltinModel(t, providerID, "kimi-k3")
			if !reflect.DeepEqual(model.Cost, want) {
				t.Fatalf("%s kimi-k3 cost = %#v, want %#v", providerID, model.Cost, want)
			}
		}
	})

	t.Run("uses API-equivalent implied pricing for Kimi Coding subscription models", func(t *testing.T) {
		cases := map[string]ModelCost{
			"k3":                        {Input: 3, Output: 15, CacheRead: 0.3, CacheWrite: 0},
			"kimi-for-coding-highspeed": {Input: 1.9, Output: 8, CacheRead: 0.38, CacheWrite: 0},
		}
		for modelID, want := range cases {
			model := requireBuiltinModel(t, "kimi-coding", modelID)
			if !reflect.DeepEqual(model.Cost, want) {
				t.Fatalf("%s cost = %#v, want %#v", modelID, model.Cost, want)
			}
		}
	})

	t.Run("resolves Anthropic bearer auth from env with auth token precedence", func(t *testing.T) {
		models := NewModels(ModelsOptions{AuthContext: providerAuthContext(map[string]string{
			AnthropicAuthTokenEnv:  "auth-token",
			AnthropicOAuthTokenEnv: "oauth-token",
			AnthropicAPIKeyEnv:     "key",
		})})
		provider, err := NewAnthropicProvider()
		if err != nil {
			t.Fatal(err)
		}
		mustSetBuiltinProvider(t, models, provider)

		result, err := models.GetAuth(context.Background(), "anthropic", AuthResolutionOverrides{})
		if err != nil {
			t.Fatal(err)
		}
		if result == nil ||
			!reflect.DeepEqual(result.Auth, ModelAuth{Headers: map[string]string{
				"Authorization": "Bearer auth-token",
			}}) ||
			result.Source != AnthropicAuthTokenEnv {
			t.Fatalf("anthropic bearer auth = %#v", result)
		}
	})

	t.Run("preserves Anthropic OAuth token precedence over API key", func(t *testing.T) {
		models := NewModels(ModelsOptions{AuthContext: providerAuthContext(map[string]string{
			AnthropicAPIKeyEnv:     "key",
			AnthropicOAuthTokenEnv: "oauth-token",
		})})
		provider, err := NewAnthropicProvider()
		if err != nil {
			t.Fatal(err)
		}
		mustSetBuiltinProvider(t, models, provider)

		result, err := models.GetAuth(context.Background(), "anthropic", AuthResolutionOverrides{})
		if err != nil {
			t.Fatal(err)
		}
		if result == nil || result.Auth.APIKey != "oauth-token" ||
			result.Source != "ANTHROPIC_OAUTH_TOKEN" {
			t.Fatalf("anthropic auth = %#v", result)
		}
	})

	t.Run("runs provider-owned Bedrock bearer token and AWS profile login flows", func(t *testing.T) {
		provider, err := NewAmazonBedrockProvider()
		if err != nil {
			t.Fatal(err)
		}
		auth := provider.Auth.APIKey
		if auth == nil || auth.Login == nil || auth.Resolve == nil {
			t.Fatal("Bedrock API-key auth is incomplete")
		}

		bearer := &queuedProviderAuthInteraction{answers: []string{"bearer-token", "bedrock-token"}}
		credential, err := auth.Login(context.Background(), bearer)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(credential, Credential{
			Type: CredentialTypeAPIKey,
			Key:  "bedrock-token",
		}) {
			t.Fatalf("bearer credential = %#v", credential)
		}

		profile := &queuedProviderAuthInteraction{answers: []string{"aws-profile", "work"}}
		credential, err = auth.Login(context.Background(), profile)
		if err != nil {
			t.Fatal(err)
		}
		wantCredential := Credential{
			Type: CredentialTypeAPIKey,
			Env:  ProviderEnv{"AWS_PROFILE": "work"},
		}
		if !reflect.DeepEqual(credential, wantCredential) {
			t.Fatalf("profile credential = %#v, want %#v", credential, wantCredential)
		}
		if len(profile.events) != 1 || len(profile.events[0].Links) != 1 ||
			profile.events[0].Links[0].Label != "AWS credential provider chain" {
			t.Fatalf("profile auth events = %#v", profile.events)
		}

		result, err := auth.Resolve(context.Background(), APIKeyResolveInput{
			Context:    providerAuthContext(nil),
			Credential: &credential,
		})
		if err != nil {
			t.Fatal(err)
		}
		if result == nil || !reflect.DeepEqual(result.Auth, ModelAuth{}) ||
			!reflect.DeepEqual(result.Env, ProviderEnv{"AWS_PROFILE": "work"}) {
			t.Fatalf("profile auth resolution = %#v", result)
		}
	})

	t.Run("reports bedrock as configured from ambient AWS credentials without an api key", func(t *testing.T) {
		models := NewModels(ModelsOptions{
			AuthContext: providerAuthContext(map[string]string{"AWS_PROFILE": "dev"}),
		})
		provider, err := NewAmazonBedrockProvider()
		if err != nil {
			t.Fatal(err)
		}
		mustSetBuiltinProvider(t, models, provider)

		result, err := models.GetAuth(context.Background(), "amazon-bedrock", AuthResolutionOverrides{})
		if err != nil {
			t.Fatal(err)
		}
		if result == nil || !reflect.DeepEqual(result.Auth, ModelAuth{}) ||
			result.Source != "AWS_PROFILE" {
			t.Fatalf("ambient Bedrock auth = %#v", result)
		}

		unconfigured := NewModels(ModelsOptions{AuthContext: providerAuthContext(nil)})
		provider, err = NewAmazonBedrockProvider()
		if err != nil {
			t.Fatal(err)
		}
		mustSetBuiltinProvider(t, unconfigured, provider)
		result, err = unconfigured.GetAuth(
			context.Background(),
			"amazon-bedrock",
			AuthResolutionOverrides{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if result != nil {
			t.Fatalf("unconfigured Bedrock auth = %#v, want nil", result)
		}
	})

	t.Run("requires Cloudflare Workers AI account config and returns scoped env", func(t *testing.T) {
		missing := NewModels(ModelsOptions{AuthContext: providerAuthContext(map[string]string{
			"CLOUDFLARE_API_KEY": "cf-key",
		})})
		provider, err := NewCloudflareWorkersAIProvider()
		if err != nil {
			t.Fatal(err)
		}
		mustSetBuiltinProvider(t, missing, provider)
		result, err := missing.GetAuth(
			context.Background(),
			"cloudflare-workers-ai",
			AuthResolutionOverrides{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if result != nil {
			t.Fatalf("partial Cloudflare auth = %#v, want nil", result)
		}

		configured := NewModels(ModelsOptions{AuthContext: providerAuthContext(map[string]string{
			"CLOUDFLARE_API_KEY":    "cf-key",
			"CLOUDFLARE_ACCOUNT_ID": "account-id",
		})})
		provider, err = NewCloudflareWorkersAIProvider()
		if err != nil {
			t.Fatal(err)
		}
		mustSetBuiltinProvider(t, configured, provider)
		result, err = configured.GetAuth(
			context.Background(),
			"cloudflare-workers-ai",
			AuthResolutionOverrides{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if result == nil || result.Auth.APIKey != "cf-key" ||
			!reflect.DeepEqual(result.Env, ProviderEnv{"CLOUDFLARE_ACCOUNT_ID": "account-id"}) {
			t.Fatalf("Cloudflare Workers AI auth = %#v", result)
		}
	})

	t.Run("requires Cloudflare AI Gateway account and gateway config and returns scoped env headers", func(t *testing.T) {
		missing := NewModels(ModelsOptions{AuthContext: providerAuthContext(map[string]string{
			"CLOUDFLARE_API_KEY":    "cf-key",
			"CLOUDFLARE_ACCOUNT_ID": "account-id",
		})})
		provider, err := NewCloudflareAIGatewayProvider()
		if err != nil {
			t.Fatal(err)
		}
		mustSetBuiltinProvider(t, missing, provider)
		result, err := missing.GetAuth(
			context.Background(),
			"cloudflare-ai-gateway",
			AuthResolutionOverrides{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if result != nil {
			t.Fatalf("partial Cloudflare Gateway auth = %#v, want nil", result)
		}

		configured := NewModels(ModelsOptions{AuthContext: providerAuthContext(map[string]string{
			"CLOUDFLARE_API_KEY":    "cf-key",
			"CLOUDFLARE_ACCOUNT_ID": "account-id",
			"CLOUDFLARE_GATEWAY_ID": "gateway-id",
		})})
		provider, err = NewCloudflareAIGatewayProvider()
		if err != nil {
			t.Fatal(err)
		}
		mustSetBuiltinProvider(t, configured, provider)
		result, err = configured.GetAuth(
			context.Background(),
			"cloudflare-ai-gateway",
			AuthResolutionOverrides{},
		)
		if err != nil {
			t.Fatal(err)
		}
		wantAuth := ModelAuth{
			Headers:        map[string]string{"cf-aig-authorization": "Bearer cf-key"},
			HeaderRemovals: []string{"Authorization", "x-api-key"},
		}
		wantEnv := ProviderEnv{
			"CLOUDFLARE_ACCOUNT_ID": "account-id",
			"CLOUDFLARE_GATEWAY_ID": "gateway-id",
		}
		if result == nil || !reflect.DeepEqual(result.Auth, wantAuth) ||
			!reflect.DeepEqual(result.Env, wantEnv) {
			t.Fatalf("Cloudflare Gateway auth = %#v", result)
		}
	})

	t.Run("runs provider-owned Vertex API key and ADC login flows", func(t *testing.T) {
		provider, err := NewGoogleVertexProvider()
		if err != nil {
			t.Fatal(err)
		}
		auth := provider.Auth.APIKey
		if auth == nil || auth.Login == nil || auth.Resolve == nil {
			t.Fatal("Vertex API-key auth is incomplete")
		}

		key := &queuedProviderAuthInteraction{answers: []string{"api-key", "vertex-key"}}
		credential, err := auth.Login(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(credential, Credential{
			Type: CredentialTypeAPIKey,
			Key:  "vertex-key",
		}) {
			t.Fatalf("Vertex key credential = %#v", credential)
		}

		adc := &queuedProviderAuthInteraction{
			answers: []string{"adc", "project-id", "us-central1"},
		}
		credential, err = auth.Login(context.Background(), adc)
		if err != nil {
			t.Fatal(err)
		}
		want := Credential{
			Type: CredentialTypeAPIKey,
			Env: ProviderEnv{
				"GOOGLE_CLOUD_PROJECT":  "project-id",
				"GOOGLE_CLOUD_LOCATION": "us-central1",
			},
		}
		if !reflect.DeepEqual(credential, want) {
			t.Fatalf("Vertex ADC credential = %#v, want %#v", credential, want)
		}
		if len(adc.events) != 1 || len(adc.events[0].Links) != 1 ||
			adc.events[0].Links[0].Label != "Application Default Credentials" {
			t.Fatalf("Vertex auth events = %#v", adc.events)
		}

		result, err := auth.Resolve(context.Background(), APIKeyResolveInput{
			Context:    providerAuthContext(nil, vertexADCPath),
			Credential: &credential,
		})
		if err != nil {
			t.Fatal(err)
		}
		if result == nil || !reflect.DeepEqual(result.Auth, ModelAuth{}) ||
			!reflect.DeepEqual(result.Env, want.Env) {
			t.Fatalf("Vertex ADC auth = %#v", result)
		}
	})

	t.Run("resolves vertex via ADC file plus project and location", func(t *testing.T) {
		configured := NewModels(ModelsOptions{AuthContext: providerAuthContext(
			map[string]string{
				"GOOGLE_CLOUD_PROJECT":  "proj",
				"GOOGLE_CLOUD_LOCATION": "us-central1",
			},
			vertexADCPath,
		)})
		provider, err := NewGoogleVertexProvider()
		if err != nil {
			t.Fatal(err)
		}
		mustSetBuiltinProvider(t, configured, provider)
		result, err := configured.GetAuth(
			context.Background(),
			"google-vertex",
			AuthResolutionOverrides{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if result == nil || !reflect.DeepEqual(result.Auth, ModelAuth{}) ||
			!strings.Contains(result.Source, "application default") {
			t.Fatalf("Vertex ADC auth = %#v", result)
		}

		partial := NewModels(ModelsOptions{AuthContext: providerAuthContext(
			map[string]string{"GOOGLE_CLOUD_PROJECT": "proj"},
			vertexADCPath,
		)})
		provider, err = NewGoogleVertexProvider()
		if err != nil {
			t.Fatal(err)
		}
		mustSetBuiltinProvider(t, partial, provider)
		result, err = partial.GetAuth(
			context.Background(),
			"google-vertex",
			AuthResolutionOverrides{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if result != nil {
			t.Fatalf("partial Vertex ADC auth = %#v, want nil", result)
		}

		keyed := NewModels(ModelsOptions{AuthContext: providerAuthContext(map[string]string{
			"GOOGLE_CLOUD_API_KEY": "vertex-key",
		})})
		provider, err = NewGoogleVertexProvider()
		if err != nil {
			t.Fatal(err)
		}
		mustSetBuiltinProvider(t, keyed, provider)
		result, err = keyed.GetAuth(
			context.Background(),
			"google-vertex",
			AuthResolutionOverrides{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if result == nil || result.Auth.APIKey != "vertex-key" {
			t.Fatalf("Vertex explicit key auth = %#v", result)
		}
	})

	t.Run("prefers the stored credential key and falls back through env vars in order", func(t *testing.T) {
		auth := EnvAPIKeyAuth("Test key", "FIRST_KEY", "SECOND_KEY")
		stored, err := auth.Resolve(context.Background(), APIKeyResolveInput{
			Context: providerAuthContext(map[string]string{"FIRST_KEY": "env"}),
			Credential: &Credential{
				Type: CredentialTypeAPIKey,
				Key:  "stored",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if stored == nil || stored.Auth.APIKey != "stored" ||
			stored.Source != "stored credential" {
			t.Fatalf("stored auth = %#v", stored)
		}

		second, err := auth.Resolve(context.Background(), APIKeyResolveInput{
			Context: providerAuthContext(map[string]string{"SECOND_KEY": "second"}),
		})
		if err != nil {
			t.Fatal(err)
		}
		if second == nil || second.Auth.APIKey != "second" || second.Source != "SECOND_KEY" {
			t.Fatalf("second env auth = %#v", second)
		}

		missing, err := auth.Resolve(context.Background(), APIKeyResolveInput{
			Context: providerAuthContext(nil),
		})
		if err != nil {
			t.Fatal(err)
		}
		if missing != nil {
			t.Fatalf("missing auth = %#v, want nil", missing)
		}
	})

	t.Run("login prompts for a secret and returns an api-key credential", func(t *testing.T) {
		auth := EnvAPIKeyAuth("Test key", "TEST_KEY")
		interaction := &queuedProviderAuthInteraction{answers: []string{"entered-key"}}
		credential, err := auth.Login(context.Background(), interaction)
		if err != nil {
			t.Fatal(err)
		}
		if len(interaction.prompts) != 1 ||
			interaction.prompts[0].Type != AuthPromptSecret {
			t.Fatalf("auth prompts = %#v", interaction.prompts)
		}
		want := Credential{Type: CredentialTypeAPIKey, Key: "entered-key"}
		if !reflect.DeepEqual(credential, want) {
			t.Fatalf("credential = %#v, want %#v", credential, want)
		}
	})

	t.Run("dispatches on model.api for mixed-API providers", func(t *testing.T) {
		var calls []string
		provider, err := CreateProvider(CreateProviderOptions{
			ID:     "mixed",
			Auth:   ProviderAuth{APIKey: ambientAPIKeyAuth()},
			Models: []Model{providerTestModel("mixed", "api-a", "model-a"), providerTestModel("mixed", "api-b", "model-b")},
			APIs: map[string]APIProvider{
				"api-a": recordingProviderAPI("a", &calls, nil),
				"api-b": recordingProviderAPI("b", &calls, nil),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		models := NewModels()
		mustSetBuiltinProvider(t, models, provider)
		_, err = models.CompleteSimple(
			context.Background(),
			providerTestModel("mixed", "api-a", "model-a"),
			providerTestContext(),
			ModelsStreamOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}
		_, err = models.CompleteSimple(
			context.Background(),
			providerTestModel("mixed", "api-b", "model-b"),
			providerTestContext(),
			ModelsStreamOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(calls, []string{"a:model-a", "b:model-b"}) {
			t.Fatalf("API calls = %#v", calls)
		}
	})

	t.Run("merges provider-resolved env into stream options", func(t *testing.T) {
		var captured StreamOptions
		model := providerTestModel("env-provider", "api-a", "model-a")
		provider, err := CreateProvider(CreateProviderOptions{
			ID:     "env-provider",
			Models: []Model{model},
			Auth: ProviderAuth{APIKey: &APIKeyAuth{
				Name: "Test",
				Resolve: func(context.Context, APIKeyResolveInput) (*AuthResult, error) {
					return &AuthResult{
						Auth: ModelAuth{APIKey: "provider-key"},
						Env: ProviderEnv{
							"PROVIDER_ONLY": "provider",
							"SHARED":        "provider",
						},
					}, nil
				},
			}},
			API: recordingProviderAPI("a", nil, &captured),
		})
		if err != nil {
			t.Fatal(err)
		}
		models := NewModels()
		mustSetBuiltinProvider(t, models, provider)
		_, err = models.CompleteSimple(
			context.Background(),
			model,
			providerTestContext(),
			ModelsStreamOptions{StreamOptions: StreamOptions{
				APIKey: "request-key",
				Env: ProviderEnv{
					"REQUEST_ONLY": "request",
					"SHARED":       "request",
				},
			}},
		)
		if err != nil {
			t.Fatal(err)
		}
		wantEnv := ProviderEnv{
			"PROVIDER_ONLY": "provider",
			"REQUEST_ONLY":  "request",
			"SHARED":        "request",
		}
		if captured.APIKey != "request-key" || !reflect.DeepEqual(captured.Env, wantEnv) {
			t.Fatalf("stream options = %#v", captured)
		}
	})

	t.Run("produces a stream error for a model whose api has no implementation", func(t *testing.T) {
		model := providerTestModel("mixed", "api-a", "model-a")
		provider, err := CreateProvider(CreateProviderOptions{
			ID:     "mixed",
			Auth:   ProviderAuth{APIKey: ambientAPIKeyAuth()},
			Models: []Model{model},
			APIs: map[string]APIProvider{
				"api-a": recordingProviderAPI("a", nil, nil),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		models := NewModels()
		mustSetBuiltinProvider(t, models, provider)
		message, err := models.CompleteSimple(
			context.Background(),
			providerTestModel("mixed", "api-ghost", "model-x"),
			providerTestContext(),
			ModelsStreamOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if message.StopReason != StopReasonError ||
			!strings.Contains(message.ErrorMessage, "no API implementation") {
			t.Fatalf("unsupported API result = %#v", message)
		}
	})

	t.Run("supports dynamic providers: empty until refreshed, in-flight refreshes deduped", func(t *testing.T) {
		var fetches atomic.Int32
		firstFetchStarted := make(chan struct{})
		releaseFirstFetch := make(chan struct{})
		provider, err := CreateProvider(CreateProviderOptions{
			ID:     "dynamic",
			Auth:   ProviderAuth{APIKey: ambientAPIKeyAuth()},
			Models: nil,
			FetchModels: func(ctx context.Context, _ RefreshModelsContext) ([]Model, error) {
				if fetches.Add(1) == 1 {
					close(firstFetchStarted)
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-releaseFirstFetch:
					}
				}
				return []Model{providerTestModel("dynamic", "api-a", "listed")}, nil
			},
			API: recordingProviderAPI("a", nil, nil),
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := provider.GetModels; got == nil {
			t.Fatal("dynamic provider has no model source")
		}
		initial, err := provider.GetModels()
		if err != nil || len(initial) != 0 {
			t.Fatalf("initial dynamic models = %#v, err=%v", initial, err)
		}

		store := scopedModelsStore{store: NewInMemoryModelsStore(), providerID: "dynamic"}
		refresh := func(ctx context.Context) error {
			return provider.RefreshModels(ctx, RefreshModelsContext{
				Credential:   &Credential{Type: CredentialTypeAPIKey},
				Store:        store,
				AllowNetwork: true,
			})
		}
		firstDone := make(chan error, 1)
		go func() { firstDone <- refresh(context.Background()) }()
		<-firstFetchStarted
		secondContext, cancelSecond := context.WithCancel(context.Background())
		secondDone := make(chan error, 1)
		go func() { secondDone <- refresh(secondContext) }()
		cancelSecond()
		if err := <-secondDone; !errors.Is(err, context.Canceled) {
			t.Fatalf("coalesced canceled refresh error = %v", err)
		}
		close(releaseFirstFetch)
		if err := <-firstDone; err != nil {
			t.Fatal(err)
		}
		if got := fetches.Load(); got != 1 {
			t.Fatalf("coalesced fetch count = %d, want 1", got)
		}
		refreshed, err := provider.GetModels()
		if err != nil || len(refreshed) != 1 || refreshed[0].ID != "listed" {
			t.Fatalf("refreshed models = %#v, err=%v", refreshed, err)
		}

		if err := refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
		if got := fetches.Load(); got != 2 {
			t.Fatalf("later fetch count = %d, want 2", got)
		}
	})

	t.Run("streams queued responses through a Models collection", func(t *testing.T) {
		faux, err := NewFauxProvider()
		if err != nil {
			t.Fatal(err)
		}
		models := NewModels()
		mustSetBuiltinProvider(t, models, faux.Provider)
		faux.SetResponses([]FauxResponseStep{{Message: FauxAssistantText("hello from faux")}})
		model, ok := faux.GetModel()
		if !ok {
			t.Fatal("faux model is missing")
		}
		result, err := models.CompleteSimple(
			context.Background(),
			model,
			providerTestContext(),
			ModelsStreamOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.StopReason != StopReasonStop ||
			len(result.Content) != 1 ||
			result.Content[0].Type != "text" ||
			result.Content[0].Text != "hello from faux" {
			t.Fatalf("faux result = %#v", result)
		}
		if got := faux.State().CallCount; got != 1 {
			t.Fatalf("faux call count = %d, want 1", got)
		}
	})

	t.Run("materializes Cloudflare endpoints from request-scoped auth env", func(t *testing.T) {
		var (
			capturedModel   Model
			capturedOptions StreamOptions
		)
		api := APIProviderFuncs{
			StreamSimpleFunc: func(
				model Model,
				_ Context,
				options SimpleStreamOptions,
			) (*AssistantMessageEventStream, error) {
				capturedModel = cloneModel(model)
				capturedOptions = cloneStreamOptions(options)
				return providerTestResponseStream(model), nil
			},
		}
		model := providerTestModel("cloudflare-ai-gateway", "openai-completions", "model-a")
		model.BaseURL = "https://gateway.ai.cloudflare.com/v1/{CLOUDFLARE_ACCOUNT_ID}/{CLOUDFLARE_GATEWAY_ID}/compat"
		provider, err := CreateProvider(CreateProviderOptions{
			ID:     "cloudflare-ai-gateway",
			Auth:   ProviderAuth{APIKey: cloudflareAIGatewayAuth()},
			Models: []Model{model},
			API:    cloudflareAPIProvider{next: api},
		})
		if err != nil {
			t.Fatal(err)
		}
		models := NewModels(ModelsOptions{AuthContext: providerAuthContext(map[string]string{
			"CLOUDFLARE_API_KEY":    "cf-key",
			"CLOUDFLARE_ACCOUNT_ID": "account-id",
			"CLOUDFLARE_GATEWAY_ID": "gateway-id",
		})})
		mustSetBuiltinProvider(t, models, provider)
		_, err = models.CompleteSimple(
			context.Background(),
			model,
			providerTestContext(),
			ModelsStreamOptions{StreamOptions: StreamOptions{
				Headers: map[string]string{"authorization": "Bearer request-token"},
			}},
		)
		if err != nil {
			t.Fatal(err)
		}
		wantBaseURL := "https://gateway.ai.cloudflare.com/v1/account-id/gateway-id/compat"
		if capturedModel.BaseURL != wantBaseURL {
			t.Fatalf("Cloudflare base URL = %q, want %q", capturedModel.BaseURL, wantBaseURL)
		}
		if got := capturedOptions.Headers["cf-aig-authorization"]; got != "Bearer cf-key" {
			t.Fatalf("Cloudflare auth header = %q", got)
		}
		if got := capturedOptions.Headers["authorization"]; got != "Bearer request-token" {
			t.Fatalf("request authorization header = %q", got)
		}
		if !reflect.DeepEqual(capturedOptions.HeaderRemovals, []string{"x-api-key"}) {
			t.Fatalf("header removals = %#v", capturedOptions.HeaderRemovals)
		}
	})

	t.Run("applies nullable-header semantics case-insensitively", func(t *testing.T) {
		headers := map[string]string{
			"Authorization":        "Bearer default",
			"X-API-Key":            "default-key",
			"cf-aig-authorization": "Bearer gateway",
		}
		got := applyHeaderRemovals(headers, []string{"authorization", "x-api-key"})
		want := map[string]string{"cf-aig-authorization": "Bearer gateway"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("headers after removals = %#v, want %#v", got, want)
		}
	})
}

func TestGitHubCopilotFiltersModelsToAuthenticatedAccountPickerCatalog(t *testing.T) {
	provider, err := NewBuiltinProvider("github-copilot")
	if err != nil {
		t.Fatal(err)
	}
	models := []Model{
		{ID: "gpt-4.1", Provider: "github-copilot"},
		{ID: "claude-opus-4.7", Provider: "github-copilot"},
		{ID: "gpt-5.4-nano", Provider: "github-copilot"},
	}
	credential := Credential{
		Type: CredentialTypeOAuth,
		Metadata: map[string]any{
			"availableModelIds": []any{"gpt-4.1"},
		},
	}

	filtered := provider.FilterModels(models, &credential)
	if len(filtered) != 1 || filtered[0].ID != "gpt-4.1" {
		t.Fatalf("filtered models = %#v", filtered)
	}
	if len(models) != 3 {
		t.Fatalf("provider policy mutated its input: %#v", models)
	}
}

func TestLazyOAuthAuthLoadsOnce(t *testing.T) {
	const providerID = "lazy-oauth-test"
	t.Cleanup(func() { UnregisterOAuthAuthLoader(providerID) })
	var loads atomic.Int32
	RegisterOAuthAuthLoader(providerID, func(context.Context) (*OAuthAuth, error) {
		loads.Add(1)
		return &OAuthAuth{
			Login: func(context.Context, AuthInteraction) (Credential, error) {
				return Credential{Type: CredentialTypeOAuth, Access: "access"}, nil
			},
			Refresh: func(_ context.Context, credential Credential) (Credential, error) {
				credential.Access = "refreshed"
				return credential, nil
			},
			ToAuth: func(context.Context, Credential) (ModelAuth, error) {
				return ModelAuth{APIKey: "derived"}, nil
			},
		}, nil
	})
	auth := registeredOAuthAuth(providerID, "Lazy OAuth", "Sign in")

	credential, err := auth.Login(context.Background(), &queuedProviderAuthInteraction{})
	if err != nil {
		t.Fatal(err)
	}
	credential, err = auth.Refresh(context.Background(), credential)
	if err != nil {
		t.Fatal(err)
	}
	modelAuth, err := auth.ToAuth(context.Background(), credential)
	if err != nil {
		t.Fatal(err)
	}
	if loads.Load() != 1 || credential.Access != "refreshed" || modelAuth.APIKey != "derived" {
		t.Fatalf("loads=%d credential=%#v auth=%#v", loads.Load(), credential, modelAuth)
	}
}

func TestLazyOAuthAuthReportsMissingLoader(t *testing.T) {
	auth := LazyOAuthAuth("Missing", "", nil)
	_, err := auth.ToAuth(context.Background(), Credential{Type: CredentialTypeOAuth})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("missing loader error = %v", err)
	}
}

func TestRegisteredLazyOAuthAuthRetriesAfterLateRegistration(t *testing.T) {
	const providerID = "late-oauth-test"
	UnregisterOAuthAuthLoader(providerID)
	t.Cleanup(func() { UnregisterOAuthAuthLoader(providerID) })

	auth := registeredOAuthAuth(providerID, "Late OAuth", "")
	if _, err := auth.ToAuth(
		context.Background(),
		Credential{Type: CredentialTypeOAuth},
	); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("initial missing loader error = %v", err)
	}

	var loads atomic.Int32
	RegisterOAuthAuthLoader(providerID, func(context.Context) (*OAuthAuth, error) {
		loads.Add(1)
		return &OAuthAuth{
			ToAuth: func(context.Context, Credential) (ModelAuth, error) {
				return ModelAuth{APIKey: "late-key"}, nil
			},
		}, nil
	})
	result, err := auth.ToAuth(
		context.Background(),
		Credential{Type: CredentialTypeOAuth},
	)
	if err != nil {
		t.Fatal(err)
	}
	if loads.Load() != 1 || result.APIKey != "late-key" {
		t.Fatalf("loads=%d auth=%#v", loads.Load(), result)
	}
}

type queuedProviderAuthInteraction struct {
	mu      sync.Mutex
	answers []string
	prompts []AuthPrompt
	events  []AuthEvent
}

func (i *queuedProviderAuthInteraction) Prompt(
	_ context.Context,
	prompt AuthPrompt,
) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.prompts = append(i.prompts, prompt)
	if len(i.answers) == 0 {
		return "", errors.New("no auth prompt answer queued")
	}
	answer := i.answers[0]
	i.answers = i.answers[1:]
	return answer, nil
}

func (i *queuedProviderAuthInteraction) Notify(event AuthEvent) {
	i.mu.Lock()
	i.events = append(i.events, event)
	i.mu.Unlock()
}

func providerAuthContext(env map[string]string, files ...string) AuthContext {
	fileSet := make(map[string]struct{}, len(files))
	for _, file := range files {
		fileSet[file] = struct{}{}
	}
	return AuthContextFuncs{
		EnvFunc: func(ctx context.Context, name string) (string, bool, error) {
			if err := contextError(ctx); err != nil {
				return "", false, err
			}
			value, ok := env[name]
			return value, ok && strings.TrimSpace(value) != "", nil
		},
		FileExistsFunc: func(ctx context.Context, path string) (bool, error) {
			if err := contextError(ctx); err != nil {
				return false, err
			}
			_, ok := fileSet[path]
			return ok, nil
		},
	}
}

func requireBuiltinModel(t *testing.T, providerID, modelID string) Model {
	t.Helper()
	model, ok := GetBuiltinModel(providerID, modelID)
	if !ok {
		t.Fatalf("built-in model %s/%s is missing", providerID, modelID)
	}
	return model
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func mustSetBuiltinProvider(t *testing.T, models *Models, provider *Provider) {
	t.Helper()
	if err := models.SetProvider(provider); err != nil {
		t.Fatal(err)
	}
}

func providerTestModel(providerID, api, modelID string) Model {
	return Model{
		ID:            modelID,
		Name:          modelID,
		API:           api,
		Provider:      providerID,
		BaseURL:       "https://example.test/v1",
		Input:         []string{"text"},
		ContextWindow: 10000,
		MaxTokens:     1000,
	}
}

func providerTestContext() Context {
	return Context{Messages: []Message{UserMessageText("hi")}}
}

func recordingProviderAPI(
	label string,
	calls *[]string,
	captured *StreamOptions,
) APIProvider {
	record := func(model Model, options StreamOptions) *AssistantMessageEventStream {
		if calls != nil {
			*calls = append(*calls, label+":"+model.ID)
		}
		if captured != nil {
			*captured = cloneStreamOptions(options)
		}
		return providerTestResponseStream(model)
	}
	return APIProviderFuncs{
		StreamFunc: func(
			model Model,
			_ Context,
			options StreamOptions,
		) (*AssistantMessageEventStream, error) {
			return record(model, options), nil
		},
		StreamSimpleFunc: func(
			model Model,
			_ Context,
			options SimpleStreamOptions,
		) (*AssistantMessageEventStream, error) {
			return record(model, options), nil
		},
	}
}

func providerTestResponseStream(model Model) *AssistantMessageEventStream {
	return CompletedAssistantStream(
		AssistantMessage([]ContentPart{Text("ok")}, StopReasonStop, model),
	)
}
