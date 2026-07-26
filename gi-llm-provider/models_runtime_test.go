package gillmprovider

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestModelsRuntime(t *testing.T) {
	t.Run("enumerates credential metadata without exposing secrets", func(t *testing.T) {
		store := NewInMemoryCredentialStore(map[string]Credential{
			"api-provider":   {Type: CredentialTypeAPIKey, Key: "secret"},
			"oauth-provider": {Type: CredentialTypeOAuth, Access: "access", Refresh: "refresh"},
		})
		got, err := store.ListCredentials(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		want := []CredentialInfo{
			{ProviderID: "api-provider", Type: CredentialTypeAPIKey},
			{ProviderID: "oauth-provider", Type: CredentialTypeOAuth},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("credential metadata = %#v, want %#v", got, want)
		}
	})

	t.Run("applies request-wide pricing tiers above the configured input threshold", func(t *testing.T) {
		model := runtimeTestModel("openai", "gpt-5.6-sol")
		model.Cost = ModelCost{
			Input:      5,
			Output:     30,
			CacheRead:  0.5,
			CacheWrite: 6.25,
			Tiers: []ModelCostTier{{
				InputTokensAbove: 272000,
				Input:            10,
				Output:           45,
				CacheRead:        1,
				CacheWrite:       12.5,
			}},
		}
		usage := func(cacheWrite int) Usage {
			return Usage{
				Input:       200000,
				Output:      100000,
				CacheRead:   72000,
				CacheWrite:  cacheWrite,
				TotalTokens: 372000 + cacheWrite,
			}
		}
		short := CalculateCost(model, usage(0))
		assertUsageCost(t, short, UsageCost{
			Input:     1,
			Output:    3,
			CacheRead: 0.036,
			Total:     4.036,
		})
		long := CalculateCost(model, usage(1))
		assertUsageCost(t, long, UsageCost{
			Input:      2,
			Output:     4.5,
			CacheRead:  0.072,
			CacheWrite: 0.0000125,
			Total:      6.5720125,
		})
	})

	t.Run("registers, replaces, and deletes providers", func(t *testing.T) {
		models := NewModels()
		first := runtimeTestProvider(runtimeProviderOptions{id: "p1"})
		second := runtimeTestProvider(runtimeProviderOptions{id: "p2"})
		mustSetRuntimeProvider(t, models, first)
		mustSetRuntimeProvider(t, models, second)
		if got := runtimeProviderIDs(models.GetProviders()); !reflect.DeepEqual(got, []string{"p1", "p2"}) {
			t.Fatalf("provider IDs = %#v", got)
		}
		replacement := runtimeTestProvider(runtimeProviderOptions{id: "p1"})
		mustSetRuntimeProvider(t, models, replacement)
		if got, _ := models.GetProvider("p1"); got != replacement || len(models.GetProviders()) != 2 {
			t.Fatalf("replacement failed: got=%p want=%p providers=%d", got, replacement, len(models.GetProviders()))
		}
		models.DeleteProvider("p1")
		if _, exists := models.GetProvider("p1"); exists {
			t.Fatal("deleted provider remained registered")
		}
		models.ClearProviders()
		if len(models.GetProviders()) != 0 {
			t.Fatal("providers remained after clear")
		}
	})

	t.Run("lists and finds models per provider", func(t *testing.T) {
		models := NewModels()
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id:     "p1",
			models: []Model{runtimeTestModel("p1", "m1"), runtimeTestModel("p1", "m2")},
		}))
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id:     "p2",
			models: []Model{runtimeTestModel("p2", "m3")},
		}))
		if got := runtimeModelIDs(models.GetModels()); !reflect.DeepEqual(got, []string{"m1", "m2", "m3"}) {
			t.Fatalf("all model IDs = %#v", got)
		}
		if got := runtimeModelIDs(models.GetModels("p1")); !reflect.DeepEqual(got, []string{"m1", "m2"}) {
			t.Fatalf("p1 model IDs = %#v", got)
		}
		if len(models.GetModels("missing")) != 0 {
			t.Fatal("unknown provider returned models")
		}
		if model, ok := models.GetModel("p2", "m3"); !ok || model.ID != "m3" {
			t.Fatalf("model lookup = %#v, %v", model, ok)
		} else if !HasAPI(model, "test-api") || HasAPI(model, "openai-completions") {
			t.Fatalf("model API check failed for %#v", model)
		}
		if _, ok := models.GetModel("p2", "missing"); ok {
			t.Fatal("missing model reported present")
		}
	})

	t.Run("swallows provider source failures for both all-provider and single-provider listing", func(t *testing.T) {
		models := NewModels()
		broken := runtimeTestProvider(runtimeProviderOptions{id: "broken"})
		broken.ModelSource = func() ([]Model, error) {
			return nil, errors.New("boom")
		}
		mustSetRuntimeProvider(t, models, broken)
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id:     "ok",
			models: []Model{runtimeTestModel("ok", "m1")},
		}))
		if got := runtimeModelIDs(models.GetModels()); !reflect.DeepEqual(got, []string{"m1"}) {
			t.Fatalf("best-effort models = %#v", got)
		}
		if len(models.GetModels("broken")) != 0 {
			t.Fatal("broken provider returned models")
		}
		if _, err := broken.GetModels(); err == nil || err.Error() != "boom" {
			t.Fatalf("direct source error = %v", err)
		}
	})

	t.Run("refresh() updates every configured dynamic provider and reports failures", func(t *testing.T) {
		models := NewModels()
		var (
			mu        sync.Mutex
			list      = []Model{runtimeTestModel("dyn", "before")}
			refreshes int
		)
		dynamic := runtimeTestProvider(runtimeProviderOptions{id: "dyn"})
		dynamic.ModelSource = func() ([]Model, error) {
			mu.Lock()
			defer mu.Unlock()
			return cloneModels(list), nil
		}
		dynamic.RefreshModelsFunc = func(context.Context, RefreshModelsContext) error {
			mu.Lock()
			defer mu.Unlock()
			refreshes++
			list = []Model{runtimeTestModel("dyn", "after")}
			return nil
		}
		mustSetRuntimeProvider(t, models, dynamic)
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{id: "static"}))

		first := models.Refresh(context.Background(), ModelsRefreshOptions{})
		if first.Aborted || len(first.Errors) != 0 {
			t.Fatalf("first refresh = %#v", first)
		}
		if _, ok := models.GetModel("dyn", "after"); !ok {
			t.Fatal("dynamic model was not updated")
		}

		flaky := runtimeTestProvider(runtimeProviderOptions{id: "flaky"})
		flaky.RefreshModelsFunc = func(context.Context, RefreshModelsContext) error {
			return errors.New("fetch failed")
		}
		mustSetRuntimeProvider(t, models, flaky)
		second := models.Refresh(context.Background(), ModelsRefreshOptions{})
		if err := second.Errors["flaky"]; err == nil || err.Error() != "fetch failed" {
			t.Fatalf("flaky error = %v", err)
		}
		mu.Lock()
		gotRefreshes := refreshes
		mu.Unlock()
		if gotRefreshes != 2 {
			t.Fatalf("dynamic refreshes = %d, want 2", gotRefreshes)
		}
	})

	t.Run("persists dynamic catalogs and restores them without network access", func(t *testing.T) {
		credentials := NewInMemoryCredentialStore(map[string]Credential{
			"dynamic": {Type: CredentialTypeAPIKey, Key: "key"},
		})
		modelStore := NewInMemoryModelsStore()
		streams := runtimeAPIProvider(nil)
		onlineProvider, err := CreateProvider(CreateProviderOptions{
			ID:     "dynamic",
			Auth:   ProviderAuth{APIKey: runtimeEnvKeyAuth("")},
			Models: nil,
			FetchModels: func(context.Context, RefreshModelsContext) ([]Model, error) {
				return []Model{runtimeTestModel("dynamic", "fetched")}, nil
			},
			API: streams,
		})
		if err != nil {
			t.Fatal(err)
		}
		online := NewModels(ModelsOptions{Credentials: credentials, ModelsStore: modelStore})
		mustSetRuntimeProvider(t, online, onlineProvider)
		if result := online.Refresh(context.Background(), ModelsRefreshOptions{}); len(result.Errors) != 0 {
			t.Fatalf("online refresh errors = %#v", result.Errors)
		}
		if _, ok := online.GetModel("dynamic", "fetched"); !ok {
			t.Fatal("online catalog missing fetched model")
		}

		var networkCalls atomic.Int32
		offlineProvider, err := CreateProvider(CreateProviderOptions{
			ID:     "dynamic",
			Auth:   ProviderAuth{APIKey: runtimeEnvKeyAuth("")},
			Models: nil,
			FetchModels: func(context.Context, RefreshModelsContext) ([]Model, error) {
				networkCalls.Add(1)
				return nil, errors.New("must not fetch")
			},
			API: streams,
		})
		if err != nil {
			t.Fatal(err)
		}
		offline := NewModels(ModelsOptions{Credentials: credentials, ModelsStore: modelStore})
		mustSetRuntimeProvider(t, offline, offlineProvider)
		if result := offline.Refresh(context.Background(), ModelsRefreshOptions{Offline: true}); len(result.Errors) != 0 {
			t.Fatalf("offline refresh errors = %#v", result.Errors)
		}
		if _, ok := offline.GetModel("dynamic", "fetched"); !ok {
			t.Fatal("offline catalog did not restore stored model")
		}
		if networkCalls.Load() != 0 {
			t.Fatalf("offline refresh made %d network calls", networkCalls.Load())
		}
	})

	t.Run("passes effective API-key credentials and refresh options while skipping unconfigured providers", func(t *testing.T) {
		models := NewModels()
		var (
			effective       *Credential
			force           bool
			unconfiguredRun atomic.Int32
		)
		configured := runtimeTestProvider(runtimeProviderOptions{
			id:   "configured",
			auth: ProviderAuth{APIKey: runtimeEnvKeyAuth("ambient-key")},
		})
		configured.RefreshModelsFunc = func(_ context.Context, input RefreshModelsContext) error {
			effective = cloneCredentialPointer(input.Credential)
			force = input.Force
			return nil
		}
		mustSetRuntimeProvider(t, models, configured)
		unconfigured := runtimeTestProvider(runtimeProviderOptions{
			id:   "unconfigured",
			auth: ProviderAuth{APIKey: runtimeEnvKeyAuth("")},
		})
		unconfigured.RefreshModelsFunc = func(context.Context, RefreshModelsContext) error {
			unconfiguredRun.Add(1)
			return nil
		}
		mustSetRuntimeProvider(t, models, unconfigured)

		result := models.Refresh(context.Background(), ModelsRefreshOptions{Force: true})
		if len(result.Errors) != 0 {
			t.Fatalf("refresh errors = %#v", result.Errors)
		}
		if effective == nil || effective.Type != CredentialTypeAPIKey || effective.Key != "ambient-key" || !force {
			t.Fatalf("effective credential=%#v force=%v", effective, force)
		}
		if unconfiguredRun.Load() != 0 {
			t.Fatalf("unconfigured provider refreshed %d times", unconfiguredRun.Load())
		}
	})

	t.Run("refreshes expired OAuth before refreshing models", func(t *testing.T) {
		credentials := NewInMemoryCredentialStore(map[string]Credential{
			"oauth-dynamic": {
				Type:    CredentialTypeOAuth,
				Access:  "expired",
				Refresh: "refresh",
				Expires: 0,
			},
		})
		models := NewModels(ModelsOptions{Credentials: credentials})
		var effective *Credential
		provider := runtimeTestProvider(runtimeProviderOptions{
			id: "oauth-dynamic",
			auth: ProviderAuth{OAuth: runtimeTestOAuth(func(
				context.Context,
				Credential,
			) (Credential, error) {
				return Credential{
					Type:    CredentialTypeOAuth,
					Access:  "fresh",
					Refresh: "rotated",
					Expires: time.Now().Add(time.Minute).UnixMilli(),
				}, nil
			})},
		})
		provider.RefreshModelsFunc = func(_ context.Context, input RefreshModelsContext) error {
			effective = cloneCredentialPointer(input.Credential)
			return nil
		}
		mustSetRuntimeProvider(t, models, provider)

		if result := models.Refresh(context.Background(), ModelsRefreshOptions{}); len(result.Errors) != 0 {
			t.Fatalf("refresh errors = %#v", result.Errors)
		}
		if effective == nil || effective.Access != "fresh" || effective.Refresh != "rotated" {
			t.Fatalf("refresh credential = %#v", effective)
		}
		persisted, _, err := credentials.ReadCredential(context.Background(), "oauth-dynamic")
		if err != nil || persisted.Access != "fresh" || persisted.Refresh != "rotated" {
			t.Fatalf("persisted credential = %#v err=%v", persisted, err)
		}
	})

	t.Run("returns aborted state without reporting cancellation as a provider error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		models := NewModels()
		provider := runtimeTestProvider(runtimeProviderOptions{id: "dynamic"})
		provider.RefreshModelsFunc = func(context.Context, RefreshModelsContext) error {
			cancel()
			return nil
		}
		mustSetRuntimeProvider(t, models, provider)
		result := models.Refresh(ctx, ModelsRefreshOptions{})
		if !result.Aborted || len(result.Errors) != 0 {
			t.Fatalf("aborted refresh = %#v", result)
		}
	})

	t.Run("resolves auth: stored credential owns the provider, ambient only when nothing stored", func(t *testing.T) {
		credentials := NewInMemoryCredentialStore()
		models := NewModels(ModelsOptions{Credentials: credentials})
		provider := runtimeTestProvider(runtimeProviderOptions{
			id: "p1",
			auth: ProviderAuth{
				APIKey: runtimeEnvKeyAuth("env-key"),
				OAuth:  runtimeTestOAuth(nil),
			},
		})
		mustSetRuntimeProvider(t, models, provider)
		model := runtimeTestModel("p1", "model-a")

		resolution, err := models.GetModelAuth(context.Background(), model, AuthResolutionOverrides{})
		if err != nil || resolution == nil || resolution.Auth.APIKey != "env-key" {
			t.Fatalf("ambient model auth = %#v err=%v", resolution, err)
		}
		explicit := "explicit-key"
		resolution, err = models.GetAuth(
			context.Background(),
			"p1",
			AuthResolutionOverrides{APIKey: &explicit},
		)
		if err != nil || resolution == nil || resolution.Auth.APIKey != explicit {
			t.Fatalf("explicit auth = %#v err=%v", resolution, err)
		}

		writeCredential(t, credentials, "p1", Credential{
			Type:    CredentialTypeOAuth,
			Access:  "oauth-token",
			Refresh: "r",
			Expires: time.Now().Add(time.Minute).UnixMilli(),
		})
		resolution, err = models.GetAuth(context.Background(), "p1", AuthResolutionOverrides{})
		if err != nil || resolution == nil || resolution.Auth.APIKey != "oauth-token" || resolution.Source != "OAuth" {
			t.Fatalf("OAuth auth = %#v err=%v", resolution, err)
		}

		writeCredential(t, credentials, "p1", Credential{Type: CredentialTypeAPIKey, Key: "stored-key"})
		resolution, err = models.GetAuth(context.Background(), "p1", AuthResolutionOverrides{})
		if err != nil || resolution == nil || resolution.Auth.APIKey != "stored-key" || resolution.Source != "stored" {
			t.Fatalf("stored API key auth = %#v err=%v", resolution, err)
		}
	})

	t.Run("checks provider auth without refreshing OAuth and filters available models", func(t *testing.T) {
		credentials := NewInMemoryCredentialStore()
		models := NewModels(ModelsOptions{Credentials: credentials})
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id:   "ambient",
			auth: ProviderAuth{APIKey: runtimeEnvKeyAuth("env-key")},
		}))
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id:   "missing",
			auth: ProviderAuth{APIKey: runtimeEnvKeyAuth("")},
		}))
		var refreshes atomic.Int32
		oauth := runtimeTestProvider(runtimeProviderOptions{
			id: "oauth",
			auth: ProviderAuth{OAuth: runtimeTestOAuth(func(
				_ context.Context,
				credential Credential,
			) (Credential, error) {
				refreshes.Add(1)
				return credential, nil
			})},
		})
		mustSetRuntimeProvider(t, models, oauth)
		writeCredential(t, credentials, "oauth", Credential{
			Type:    CredentialTypeOAuth,
			Access:  "expired",
			Refresh: "refresh",
			Expires: 0,
		})

		check, ok, err := models.CheckAuth(context.Background(), "ambient")
		if err != nil || !ok || check.Source != "env" || check.Type != CredentialTypeAPIKey {
			t.Fatalf("ambient check = %#v ok=%v err=%v", check, ok, err)
		}
		if _, ok, err := models.CheckAuth(context.Background(), "missing"); err != nil || ok {
			t.Fatalf("missing check: ok=%v err=%v", ok, err)
		}
		check, ok, err = models.CheckAuth(context.Background(), "oauth")
		if err != nil || !ok || check.Source != "OAuth" || check.Type != CredentialTypeOAuth {
			t.Fatalf("OAuth check = %#v ok=%v err=%v", check, ok, err)
		}
		if refreshes.Load() != 0 {
			t.Fatalf("auth check refreshed OAuth %d times", refreshes.Load())
		}
		available, err := models.GetAvailable(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got := runtimeProviderIDsFromModels(available); !reflect.DeepEqual(got, []string{"ambient", "oauth"}) {
			t.Fatalf("available providers = %#v", got)
		}
	})

	t.Run("runs provider login and logout through the credential store", func(t *testing.T) {
		credentials := NewInMemoryCredentialStore()
		apiKey := runtimeEnvKeyAuth("")
		apiKey.Login = func(context.Context, AuthInteraction) (Credential, error) {
			return Credential{Type: CredentialTypeAPIKey, Key: "logged-in"}, nil
		}
		models := NewModels(ModelsOptions{Credentials: credentials})
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id:   "p1",
			auth: ProviderAuth{APIKey: apiKey},
		}))

		credential, err := models.Login(
			context.Background(),
			"p1",
			CredentialTypeAPIKey,
			runtimeAuthInteraction{},
		)
		if err != nil || credential.Key != "logged-in" {
			t.Fatalf("login credential = %#v err=%v", credential, err)
		}
		persisted, exists, err := credentials.ReadCredential(context.Background(), "p1")
		if err != nil || !exists || persisted.Key != "logged-in" {
			t.Fatalf("persisted login = %#v exists=%v err=%v", persisted, exists, err)
		}
		if err := models.Logout(context.Background(), "p1"); err != nil {
			t.Fatal(err)
		}
		if _, exists, err := credentials.ReadCredential(context.Background(), "p1"); err != nil || exists {
			t.Fatalf("credential after logout: exists=%v err=%v", exists, err)
		}
	})

	t.Run("a stored credential without a matching handler blocks ambient fallback", func(t *testing.T) {
		credentials := NewInMemoryCredentialStore(map[string]Credential{
			"p1": {Type: CredentialTypeOAuth, Access: "a", Refresh: "r"},
		})
		models := NewModels(ModelsOptions{Credentials: credentials})
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id:   "p1",
			auth: ProviderAuth{APIKey: runtimeEnvKeyAuth("env-key")},
		}))
		result, err := models.GetAuth(context.Background(), "p1", AuthResolutionOverrides{})
		if err != nil || result != nil {
			t.Fatalf("mismatched credential result = %#v err=%v", result, err)
		}
	})

	t.Run("refreshes expired oauth credentials and persists the rotated credential", func(t *testing.T) {
		credentials := NewInMemoryCredentialStore(map[string]Credential{
			"p1": {Type: CredentialTypeOAuth, Access: "old-token", Refresh: "r", Expires: 0},
		})
		models := NewModels(ModelsOptions{Credentials: credentials})
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id: "p1",
			auth: ProviderAuth{OAuth: runtimeTestOAuth(func(
				_ context.Context,
				credential Credential,
			) (Credential, error) {
				credential.Access = "new-token"
				credential.Expires = time.Now().Add(time.Minute).UnixMilli()
				return credential, nil
			})},
		}))
		result, err := models.GetAuth(context.Background(), "p1", AuthResolutionOverrides{})
		if err != nil || result == nil || result.Auth.APIKey != "new-token" {
			t.Fatalf("OAuth auth = %#v err=%v", result, err)
		}
		persisted, _, _ := credentials.ReadCredential(context.Background(), "p1")
		if persisted.Access != "new-token" {
			t.Fatalf("persisted OAuth = %#v", persisted)
		}
	})

	t.Run("rejects with code oauth when refresh fails, preserving the stored credential", func(t *testing.T) {
		credentials := NewInMemoryCredentialStore(map[string]Credential{
			"p1": {Type: CredentialTypeOAuth, Access: "old", Refresh: "r", Expires: 0},
		})
		models := NewModels(ModelsOptions{Credentials: credentials})
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id: "p1",
			auth: ProviderAuth{OAuth: runtimeTestOAuth(func(
				context.Context,
				Credential,
			) (Credential, error) {
				return Credential{}, errors.New("invalid_grant")
			})},
		}))
		_, err := models.GetAuth(context.Background(), "p1", AuthResolutionOverrides{})
		var modelsErr *ModelsError
		if !errors.As(err, &modelsErr) || modelsErr.Code != ModelsErrorOAuth {
			t.Fatalf("error = %v, want code %q", err, ModelsErrorOAuth)
		}
		if got := err.Error(); got != "OAuth refresh failed for p1: invalid_grant" {
			t.Fatalf("error text = %q", got)
		}
		persisted, _, _ := credentials.ReadCredential(context.Background(), "p1")
		if persisted.Access != "old" {
			t.Fatalf("failed refresh changed credential: %#v", persisted)
		}
	})

	t.Run("serializes concurrent OAuth refreshes through store.modify (no double refresh)", func(t *testing.T) {
		credentials := NewInMemoryCredentialStore(map[string]Credential{
			"p1": {Type: CredentialTypeOAuth, Access: "old", Refresh: "r1", Expires: 0},
		})
		var refreshes atomic.Int32
		models := NewModels(ModelsOptions{Credentials: credentials})
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id: "p1",
			auth: ProviderAuth{OAuth: runtimeTestOAuth(func(
				context.Context,
				Credential,
			) (Credential, error) {
				call := refreshes.Add(1)
				time.Sleep(10 * time.Millisecond)
				return Credential{
					Type:    CredentialTypeOAuth,
					Access:  "new-" + strconv.Itoa(int(call)),
					Refresh: "r2",
					Expires: time.Now().Add(time.Minute).UnixMilli(),
				}, nil
			})},
		}))
		results := make(chan *AuthResult, 2)
		errs := make(chan error, 2)
		for range 2 {
			go func() {
				result, err := models.GetAuth(context.Background(), "p1", AuthResolutionOverrides{})
				results <- result
				errs <- err
			}()
		}
		for range 2 {
			if err := <-errs; err != nil {
				t.Fatal(err)
			}
			if result := <-results; result == nil || result.Auth.APIKey != "new-1" {
				t.Fatalf("concurrent result = %#v", result)
			}
		}
		if refreshes.Load() != 1 {
			t.Fatalf("refreshes = %d, want 1", refreshes.Load())
		}
	})

	t.Run("valid oauth tokens resolve without touching modify", func(t *testing.T) {
		base := NewInMemoryCredentialStore(map[string]Credential{
			"p1": {
				Type:    CredentialTypeOAuth,
				Access:  "valid",
				Refresh: "r",
				Expires: time.Now().Add(time.Minute).UnixMilli(),
			},
		})
		counting := &countingCredentialStore{CredentialStore: base}
		models := NewModels(ModelsOptions{Credentials: counting})
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id:   "p1",
			auth: ProviderAuth{OAuth: runtimeTestOAuth(nil)},
		}))
		result, err := models.GetAuth(context.Background(), "p1", AuthResolutionOverrides{})
		if err != nil || result == nil || result.Auth.APIKey != "valid" {
			t.Fatalf("valid OAuth = %#v err=%v", result, err)
		}
		if counting.modifies.Load() != 0 {
			t.Fatalf("modify calls = %d", counting.modifies.Load())
		}
	})

	t.Run("wraps credential store failures in ModelsError", func(t *testing.T) {
		storeErr := errors.New("disk on fire")
		readFailing := credentialStoreFuncs{
			read: func(context.Context, string) (Credential, bool, error) {
				return Credential{}, false, storeErr
			},
		}
		models := NewModels(ModelsOptions{Credentials: readFailing})
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{id: "p1"}))
		_, err := models.GetAuth(context.Background(), "p1", AuthResolutionOverrides{})
		assertModelsError(t, err, ModelsErrorAuth, storeErr)

		modifyFailing := credentialStoreFuncs{
			read: func(context.Context, string) (Credential, bool, error) {
				return Credential{Type: CredentialTypeOAuth, Expires: 0}, true, nil
			},
			modify: func(context.Context, string, CredentialModifier) (Credential, bool, error) {
				return Credential{}, false, storeErr
			},
		}
		models = NewModels(ModelsOptions{Credentials: modifyFailing})
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id:   "p1",
			auth: ProviderAuth{OAuth: runtimeTestOAuth(nil)},
		}))
		_, err = models.GetAuth(context.Background(), "p1", AuthResolutionOverrides{})
		assertModelsError(t, err, ModelsErrorAuth, storeErr)
	})

	t.Run("wraps api-key auth failures in ModelsError", func(t *testing.T) {
		resolverErr := errors.New("nope")
		models := NewModels()
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id: "p1",
			auth: ProviderAuth{APIKey: &APIKeyAuth{
				Name: "Failing",
				Resolve: func(context.Context, APIKeyResolveInput) (*AuthResult, error) {
					return nil, resolverErr
				},
			}},
		}))
		_, err := models.GetAuth(context.Background(), "p1", AuthResolutionOverrides{})
		assertModelsError(t, err, ModelsErrorAuth, resolverErr)
	})

	t.Run("uses explicit request api key and env during provider auth resolution", func(t *testing.T) {
		calls := &runtimeProviderCalls{}
		apiKey := &APIKeyAuth{
			Name: "Scoped",
			Resolve: func(ctx context.Context, input APIKeyResolveInput) (*AuthResult, error) {
				if input.Credential == nil {
					return nil, nil
				}
				account := input.Credential.Env["ACCOUNT_ID"]
				if account == "" {
					account, _, _ = input.Context.Env(ctx, "ACCOUNT_ID")
				}
				if input.Credential.Key == "" || account == "" {
					return nil, nil
				}
				return &AuthResult{
					Auth: ModelAuth{
						APIKey:  input.Credential.Key,
						BaseURL: "https://example.test/" + account,
					},
					Env: ProviderEnv{"ACCOUNT_ID": account},
				}, nil
			},
		}
		models := NewModels()
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id:    "p1",
			auth:  ProviderAuth{APIKey: apiKey},
			calls: calls,
		}))
		explicit := "explicit-key"
		message, err := models.CompleteSimple(
			context.Background(),
			runtimeTestModel("p1", "model-a"),
			runtimeTestContext(),
			ModelsStreamOptions{
				StreamOptions: StreamOptions{Env: ProviderEnv{"ACCOUNT_ID": "acct"}},
				APIKey:        &explicit,
			},
		)
		if err != nil || message.StopReason != StopReasonStop {
			t.Fatalf("completion = %#v err=%v", message, err)
		}
		call := calls.at(0)
		if call.model.BaseURL != "https://example.test/acct" ||
			call.options.APIKey != explicit ||
			call.options.Env["ACCOUNT_ID"] != "acct" {
			t.Fatalf("provider call = %#v", call)
		}
	})

	t.Run("merges resolved auth into stream options; explicit options win per field", func(t *testing.T) {
		calls := &runtimeProviderCalls{}
		models := NewModels()
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id: "p1",
			auth: ProviderAuth{APIKey: &APIKeyAuth{
				Name: "Test",
				Resolve: func(context.Context, APIKeyResolveInput) (*AuthResult, error) {
					return &AuthResult{Auth: ModelAuth{
						APIKey:  "resolved-key",
						Headers: map[string]string{"Authorization": "Bearer resolved-key", "x-a": "auth", "x-b": "auth"},
						BaseURL: "https://auth.test/v1",
					}}, nil
				},
			}},
			calls: calls,
		}))
		explicit := "explicit-key"
		message, err := models.CompleteSimple(
			context.Background(),
			runtimeTestModel("p1", "model-a"),
			runtimeTestContext(),
			ModelsStreamOptions{
				StreamOptions: StreamOptions{
					Headers: map[string]string{"authorization": "Explicit token", "x-b": "explicit"},
				},
				APIKey: &explicit,
			},
		)
		if err != nil || message.StopReason != StopReasonStop {
			t.Fatalf("completion = %#v err=%v", message, err)
		}
		call := calls.at(0)
		wantHeaders := map[string]string{
			"authorization": "Explicit token",
			"x-a":           "auth",
			"x-b":           "explicit",
		}
		if call.options.APIKey != explicit || !reflect.DeepEqual(call.options.Headers, wantHeaders) ||
			call.model.BaseURL != "https://auth.test/v1" {
			t.Fatalf("explicit provider call = %#v", call)
		}

		if _, err := models.CompleteSimple(
			context.Background(),
			runtimeTestModel("p1", "model-a"),
			runtimeTestContext(),
			ModelsStreamOptions{},
		); err != nil {
			t.Fatal(err)
		}
		if call := calls.at(1); call.options.APIKey != "resolved-key" {
			t.Fatalf("resolved provider call = %#v", call)
		}
	})

	t.Run("adds model headers only for model auth and transforms assembled headers once", func(t *testing.T) {
		calls := &runtimeProviderCalls{}
		models := NewModels()
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{
			id:    "p1",
			auth:  ProviderAuth{APIKey: runtimeEnvKeyAuth("key")},
			calls: calls,
		}))
		model := runtimeTestModel("p1", "model-a")
		model.Headers = map[string]string{"x-model": "model", "x-shared": "model"}
		providerAuth, err := models.GetAuth(context.Background(), "p1", AuthResolutionOverrides{})
		if err != nil || providerAuth == nil || providerAuth.Auth.Headers != nil {
			t.Fatalf("provider auth = %#v err=%v", providerAuth, err)
		}
		modelAuth, err := models.GetModelAuth(context.Background(), model, AuthResolutionOverrides{})
		if err != nil || modelAuth == nil || !reflect.DeepEqual(modelAuth.Auth.Headers, model.Headers) {
			t.Fatalf("model auth = %#v err=%v", modelAuth, err)
		}

		var transforms atomic.Int32
		_, err = models.CompleteSimple(
			context.Background(),
			model,
			runtimeTestContext(),
			ModelsStreamOptions{
				StreamOptions: StreamOptions{
					Headers: map[string]string{"x-explicit": "explicit", "X-Shared": "explicit"},
				},
				TransformHeaders: func(_ context.Context, headers map[string]string) (map[string]string, error) {
					transforms.Add(1)
					want := map[string]string{
						"x-model":    "model",
						"x-explicit": "explicit",
						"X-Shared":   "explicit",
					}
					if !reflect.DeepEqual(headers, want) {
						t.Fatalf("assembled headers = %#v, want %#v", headers, want)
					}
					headers["x-transformed"] = "yes"
					return headers, nil
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if transforms.Load() != 1 {
			t.Fatalf("header transforms = %d", transforms.Load())
		}
		want := map[string]string{
			"x-model":       "model",
			"x-explicit":    "explicit",
			"X-Shared":      "explicit",
			"x-transformed": "yes",
		}
		if call := calls.at(0); !reflect.DeepEqual(call.options.Headers, want) {
			t.Fatalf("provider headers = %#v, want %#v", call.options.Headers, want)
		}
	})

	t.Run("produces an error stream for unknown providers instead of throwing", func(t *testing.T) {
		models := NewModels()
		message, err := models.CompleteSimple(
			context.Background(),
			runtimeTestModel("ghost", "model-a"),
			runtimeTestContext(),
			ModelsStreamOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if message.StopReason != StopReasonError ||
			!strings.Contains(message.ErrorMessage, "Unknown provider: ghost") {
			t.Fatalf("unknown provider message = %#v", message)
		}
	})

	t.Run("streams through the provider", func(t *testing.T) {
		models := NewModels()
		mustSetRuntimeProvider(t, models, runtimeTestProvider(runtimeProviderOptions{id: "p1"}))
		stream := models.StreamSimple(
			context.Background(),
			runtimeTestModel("p1", "model-a"),
			runtimeTestContext(),
			ModelsStreamOptions{},
		)
		var events []string
		for event := range stream.Events() {
			events = append(events, event.Type)
		}
		if !reflect.DeepEqual(events, []string{"start", "done"}) {
			t.Fatalf("event types = %#v", events)
		}
		message, err := stream.Result(context.Background())
		if err != nil || message.StopReason != StopReasonStop {
			t.Fatalf("stream result = %#v err=%v", message, err)
		}
	})
}

func TestInMemoryModelsStoreClonesEntries(t *testing.T) {
	store := NewInMemoryModelsStore()
	entry := ModelsStoreEntry{
		Models:       []Model{runtimeTestModel("provider", "model")},
		LastModified: ptrInt64(10),
		CheckedAt:    20,
	}
	entry.Models[0].Headers = map[string]string{"X-Test": "original"}
	if err := store.WriteModels(context.Background(), "provider", entry); err != nil {
		t.Fatal(err)
	}
	entry.Models[0].Headers["X-Test"] = "caller-mutated"

	read, exists, err := store.ReadModels(context.Background(), "provider")
	if err != nil || !exists || read.Models[0].Headers["X-Test"] != "original" {
		t.Fatalf("stored entry = %#v exists=%v err=%v", read, exists, err)
	}
	read.Models[0].Headers["X-Test"] = "read-mutated"
	again, _, _ := store.ReadModels(context.Background(), "provider")
	if again.Models[0].Headers["X-Test"] != "original" {
		t.Fatalf("read shared mutable state: %#v", again)
	}
	if read.LastModified == entry.LastModified ||
		read.LastModified == again.LastModified {
		t.Fatal("last-modified pointer shared mutable state")
	}
}

func ptrInt64(value int64) *int64 {
	return &value
}

type runtimeProviderOptions struct {
	id     string
	models []Model
	auth   ProviderAuth
	calls  *runtimeProviderCalls
}

type runtimeProviderCall struct {
	model   Model
	options StreamOptions
}

type runtimeProviderCalls struct {
	mu    sync.Mutex
	calls []runtimeProviderCall
}

func (c *runtimeProviderCalls) append(call runtimeProviderCall) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.calls = append(c.calls, call)
	c.mu.Unlock()
}

func (c *runtimeProviderCalls) at(index int) runtimeProviderCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[index]
}

func runtimeTestProvider(options runtimeProviderOptions) *Provider {
	if len(options.models) == 0 {
		options.models = []Model{runtimeTestModel(options.id, "model-a")}
	}
	if options.auth.APIKey == nil && options.auth.OAuth == nil {
		options.auth = ProviderAuth{APIKey: runtimeAmbientAuth()}
	}
	return &Provider{
		ID:   options.id,
		Name: options.id,
		Auth: options.auth,
		ModelSource: func() ([]Model, error) {
			return cloneModels(options.models), nil
		},
		StreamFunc: func(
			model Model,
			_ Context,
			streamOptions StreamOptions,
		) (*AssistantMessageEventStream, error) {
			options.calls.append(runtimeProviderCall{
				model:   cloneModel(model),
				options: cloneStreamOptions(streamOptions),
			})
			return runtimeResponseStream(model), nil
		},
		StreamSimpleFunc: func(
			model Model,
			_ Context,
			streamOptions SimpleStreamOptions,
		) (*AssistantMessageEventStream, error) {
			options.calls.append(runtimeProviderCall{
				model:   cloneModel(model),
				options: cloneStreamOptions(streamOptions),
			})
			return runtimeResponseStream(model), nil
		},
	}
}

func runtimeAPIProvider(calls *runtimeProviderCalls) APIProvider {
	return APIProviderFuncs{
		StreamFunc: func(
			model Model,
			_ Context,
			options StreamOptions,
		) (*AssistantMessageEventStream, error) {
			calls.append(runtimeProviderCall{model: cloneModel(model), options: cloneStreamOptions(options)})
			return runtimeResponseStream(model), nil
		},
		StreamSimpleFunc: func(
			model Model,
			_ Context,
			options SimpleStreamOptions,
		) (*AssistantMessageEventStream, error) {
			calls.append(runtimeProviderCall{model: cloneModel(model), options: cloneStreamOptions(options)})
			return runtimeResponseStream(model), nil
		},
	}
}

func runtimeResponseStream(model Model) *AssistantMessageEventStream {
	message := AssistantMessage([]ContentPart{Text("ok")}, StopReasonStop, model)
	stream := NewAssistantMessageEventStream()
	stream.Push(AssistantMessageEvent{Type: "start", Partial: message})
	stream.Push(AssistantMessageEvent{Type: "done", Reason: StopReasonStop, Message: message})
	return stream
}

func runtimeTestModel(providerID, modelID string) Model {
	return Model{
		ID:            modelID,
		Name:          modelID,
		API:           "test-api",
		Provider:      providerID,
		BaseURL:       "https://example.test/v1",
		Input:         []string{"text"},
		Cost:          ModelCost{},
		ContextWindow: 10000,
		MaxTokens:     1000,
	}
}

func runtimeTestContext() Context {
	return Context{Messages: []Message{UserMessageText("hi")}}
}

func runtimeAmbientAuth() *APIKeyAuth {
	return &APIKeyAuth{
		Name: "Ambient",
		Resolve: func(context.Context, APIKeyResolveInput) (*AuthResult, error) {
			return &AuthResult{Auth: ModelAuth{}}, nil
		},
	}
}

func runtimeEnvKeyAuth(ambient string) *APIKeyAuth {
	return &APIKeyAuth{
		Name: "Test API key",
		Resolve: func(_ context.Context, input APIKeyResolveInput) (*AuthResult, error) {
			if input.Credential != nil && input.Credential.Key != "" {
				return &AuthResult{
					Auth:   ModelAuth{APIKey: input.Credential.Key},
					Source: "stored",
				}, nil
			}
			if ambient == "" {
				return nil, nil
			}
			return &AuthResult{Auth: ModelAuth{APIKey: ambient}, Source: "env"}, nil
		},
	}
}

func runtimeTestOAuth(
	refresh func(context.Context, Credential) (Credential, error),
) *OAuthAuth {
	if refresh == nil {
		refresh = func(_ context.Context, credential Credential) (Credential, error) {
			return credential, nil
		}
	}
	return &OAuthAuth{
		Name: "Test OAuth",
		Login: func(context.Context, AuthInteraction) (Credential, error) {
			return Credential{}, errors.New("not used")
		},
		Refresh: refresh,
		ToAuth: func(_ context.Context, credential Credential) (ModelAuth, error) {
			return ModelAuth{APIKey: credential.Access}, nil
		},
	}
}

type runtimeAuthInteraction struct{}

func (runtimeAuthInteraction) Prompt(context.Context, AuthPrompt) (string, error) {
	return "unused", nil
}

func (runtimeAuthInteraction) Notify(AuthEvent) {}

func mustSetRuntimeProvider(t *testing.T, models *Models, provider *Provider) {
	t.Helper()
	if err := models.SetProvider(provider); err != nil {
		t.Fatal(err)
	}
}

func writeCredential(
	t *testing.T,
	store CredentialStore,
	providerID string,
	credential Credential,
) {
	t.Helper()
	if _, _, err := store.ModifyCredential(
		context.Background(),
		providerID,
		func(context.Context, Credential, bool) (Credential, bool, error) {
			return credential, true, nil
		},
	); err != nil {
		t.Fatal(err)
	}
}

func runtimeProviderIDs(providers []*Provider) []string {
	ids := make([]string, len(providers))
	for index, provider := range providers {
		ids[index] = provider.ID
	}
	return ids
}

func runtimeModelIDs(models []Model) []string {
	ids := make([]string, len(models))
	for index, model := range models {
		ids[index] = model.ID
	}
	return ids
}

func runtimeProviderIDsFromModels(models []Model) []string {
	ids := make([]string, len(models))
	for index, model := range models {
		ids[index] = model.Provider
	}
	return ids
}

func assertUsageCost(t *testing.T, got, want UsageCost) {
	t.Helper()
	for name, values := range map[string][2]float64{
		"input":       {got.Input, want.Input},
		"output":      {got.Output, want.Output},
		"cache read":  {got.CacheRead, want.CacheRead},
		"cache write": {got.CacheWrite, want.CacheWrite},
		"total":       {got.Total, want.Total},
	} {
		if math.Abs(values[0]-values[1]) > 1e-12 {
			t.Fatalf("%s cost = %.12f, want %.12f", name, values[0], values[1])
		}
	}
}
