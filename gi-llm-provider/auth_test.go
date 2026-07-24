package gillmprovider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolveProviderAuthStoredCredentialOwnsProvider(t *testing.T) {
	store := NewInMemoryCredentialStore()
	authContext := mapAuthContext(map[string]string{"TEST_API_KEY": "ambient-key"})
	auth := ProviderAuth{
		APIKey: &APIKeyAuth{
			Name: "Test API key",
			Resolve: func(ctx context.Context, input APIKeyResolveInput) (*AuthResult, error) {
				if input.Credential != nil && input.Credential.Key != "" {
					return &AuthResult{
						Auth:   ModelAuth{APIKey: input.Credential.Key},
						Source: "stored",
					}, nil
				}
				key, ok, err := input.Context.Env(ctx, "TEST_API_KEY")
				if err != nil || !ok {
					return nil, err
				}
				return &AuthResult{Auth: ModelAuth{APIKey: key}, Source: "env"}, nil
			},
		},
		OAuth: &OAuthAuth{
			Name: "Test OAuth",
			Refresh: func(_ context.Context, credential Credential) (Credential, error) {
				return credential, nil
			},
			ToAuth: func(_ context.Context, credential Credential) (ModelAuth, error) {
				return ModelAuth{APIKey: credential.Access}, nil
			},
		},
	}

	result, err := ResolveProviderAuth(
		context.Background(),
		"provider",
		auth,
		store,
		authContext,
		AuthResolutionOverrides{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Auth.APIKey != "ambient-key" || result.Source != "env" {
		t.Fatalf("ambient result = %#v", result)
	}

	_, _, err = store.ModifyCredential(
		context.Background(),
		"provider",
		func(context.Context, Credential, bool) (Credential, bool, error) {
			return Credential{
				Type:    CredentialTypeOAuth,
				Access:  "oauth-token",
				Refresh: "refresh",
				Expires: time.Now().Add(time.Minute).UnixMilli(),
			}, true, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err = ResolveProviderAuth(
		context.Background(),
		"provider",
		auth,
		store,
		authContext,
		AuthResolutionOverrides{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Auth.APIKey != "oauth-token" || result.Source != "OAuth" {
		t.Fatalf("OAuth result = %#v", result)
	}

	_, _, err = store.ModifyCredential(
		context.Background(),
		"provider",
		func(context.Context, Credential, bool) (Credential, bool, error) {
			return Credential{Type: CredentialTypeAPIKey, Key: "stored-key"}, true, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err = ResolveProviderAuth(
		context.Background(),
		"provider",
		auth,
		store,
		authContext,
		AuthResolutionOverrides{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Auth.APIKey != "stored-key" || result.Source != "stored" {
		t.Fatalf("stored API key result = %#v", result)
	}
}

func TestResolveProviderAuthExplicitAPIKeyPrecedesStoredCredential(t *testing.T) {
	store := NewInMemoryCredentialStore(map[string]Credential{
		"provider": {
			Type:    CredentialTypeOAuth,
			Access:  "stored-oauth",
			Refresh: "refresh",
			Expires: time.Now().Add(time.Minute).UnixMilli(),
		},
	})
	auth := ProviderAuth{
		APIKey: &APIKeyAuth{
			Resolve: func(_ context.Context, input APIKeyResolveInput) (*AuthResult, error) {
				return &AuthResult{Auth: ModelAuth{APIKey: input.Credential.Key}}, nil
			},
		},
		OAuth: &OAuthAuth{
			ToAuth: func(_ context.Context, credential Credential) (ModelAuth, error) {
				return ModelAuth{APIKey: credential.Access}, nil
			},
		},
	}
	explicit := "request-key"

	result, err := ResolveProviderAuth(
		context.Background(),
		"provider",
		auth,
		store,
		nil,
		AuthResolutionOverrides{APIKey: &explicit},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Auth.APIKey != explicit {
		t.Fatalf("explicit result = %#v", result)
	}
}

func TestResolveProviderAuthStoredMismatchedTypeBlocksAmbient(t *testing.T) {
	store := NewInMemoryCredentialStore(map[string]Credential{
		"provider": {
			Type:    CredentialTypeOAuth,
			Access:  "stale",
			Refresh: "refresh",
		},
	})
	auth := ProviderAuth{
		APIKey: &APIKeyAuth{
			Resolve: func(_ context.Context, _ APIKeyResolveInput) (*AuthResult, error) {
				return &AuthResult{Auth: ModelAuth{APIKey: "must-not-be-used"}}, nil
			},
		},
	}

	result, err := ResolveProviderAuth(
		context.Background(),
		"provider",
		auth,
		store,
		nil,
		AuthResolutionOverrides{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatalf("mismatched stored credential fell back to ambient: %#v", result)
	}
}

func TestResolveProviderAuthValidOAuthSkipsModify(t *testing.T) {
	base := NewInMemoryCredentialStore(map[string]Credential{
		"provider": {
			Type:    CredentialTypeOAuth,
			Access:  "valid",
			Refresh: "refresh",
			Expires: time.Now().Add(time.Minute).UnixMilli(),
		},
	})
	store := &countingCredentialStore{CredentialStore: base}
	auth := ProviderAuth{
		OAuth: &OAuthAuth{
			ToAuth: func(_ context.Context, credential Credential) (ModelAuth, error) {
				return ModelAuth{APIKey: credential.Access}, nil
			},
		},
	}

	result, err := ResolveProviderAuth(
		context.Background(),
		"provider",
		auth,
		store,
		nil,
		AuthResolutionOverrides{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Auth.APIKey != "valid" {
		t.Fatalf("OAuth result = %#v", result)
	}
	if got := store.modifies.Load(); got != 0 {
		t.Fatalf("modify calls = %d, want 0", got)
	}
}

func TestResolveProviderAuthSerializesOAuthRefresh(t *testing.T) {
	store := NewInMemoryCredentialStore(map[string]Credential{
		"provider": {
			Type:    CredentialTypeOAuth,
			Access:  "expired",
			Refresh: "old-refresh",
			Expires: 0,
		},
	})
	var refreshes atomic.Int32
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	auth := ProviderAuth{
		OAuth: &OAuthAuth{
			Refresh: func(_ context.Context, _ Credential) (Credential, error) {
				if refreshes.Add(1) == 1 {
					close(refreshStarted)
				}
				<-releaseRefresh
				return Credential{
					Type:    CredentialTypeOAuth,
					Access:  "fresh",
					Refresh: "rotated",
					Expires: time.Now().Add(time.Minute).UnixMilli(),
				}, nil
			},
			ToAuth: func(_ context.Context, credential Credential) (ModelAuth, error) {
				return ModelAuth{APIKey: credential.Access}, nil
			},
		},
	}

	const callers = 16
	start := make(chan struct{})
	results := make(chan *AuthResult, callers)
	errs := make(chan error, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	for range callers {
		go func() {
			ready.Done()
			<-start
			result, err := ResolveProviderAuth(
				context.Background(),
				"provider",
				auth,
				store,
				nil,
				AuthResolutionOverrides{},
			)
			results <- result
			errs <- err
		}()
	}
	ready.Wait()
	close(start)
	<-refreshStarted
	close(releaseRefresh)

	for range callers {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		result := <-results
		if result == nil || result.Auth.APIKey != "fresh" {
			t.Fatalf("concurrent result = %#v", result)
		}
	}
	if got := refreshes.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	persisted, exists, err := store.ReadCredential(context.Background(), "provider")
	if err != nil || !exists {
		t.Fatalf("read persisted credential: exists=%v err=%v", exists, err)
	}
	if persisted.Access != "fresh" || persisted.Refresh != "rotated" {
		t.Fatalf("persisted credential = %#v", persisted)
	}
}

func TestResolveProviderAuthOAuthFailurePreservesCredential(t *testing.T) {
	refreshErr := errors.New("invalid_grant")
	original := Credential{
		Type:    CredentialTypeOAuth,
		Access:  "old",
		Refresh: "refresh",
		Expires: 0,
	}
	store := NewInMemoryCredentialStore(map[string]Credential{"provider": original})
	auth := ProviderAuth{
		OAuth: &OAuthAuth{
			Refresh: func(context.Context, Credential) (Credential, error) {
				return Credential{}, refreshErr
			},
			ToAuth: func(_ context.Context, credential Credential) (ModelAuth, error) {
				return ModelAuth{APIKey: credential.Access}, nil
			},
		},
	}

	_, err := ResolveProviderAuth(
		context.Background(),
		"provider",
		auth,
		store,
		nil,
		AuthResolutionOverrides{},
	)
	var modelsErr *ModelsError
	if !errors.As(err, &modelsErr) || modelsErr.Code != ModelsErrorOAuth {
		t.Fatalf("error = %#v, want OAuth ModelsError", err)
	}
	if !errors.Is(err, refreshErr) {
		t.Fatalf("error does not unwrap refresh failure: %v", err)
	}
	persisted, exists, readErr := store.ReadCredential(context.Background(), "provider")
	if readErr != nil || !exists {
		t.Fatalf("read persisted credential: exists=%v err=%v", exists, readErr)
	}
	if persisted.Type != original.Type ||
		persisted.Access != original.Access ||
		persisted.Refresh != original.Refresh ||
		persisted.Expires != original.Expires {
		t.Fatalf("credential changed on failed refresh: %#v", persisted)
	}
}

func TestResolveProviderAuthWrapsStoreAndResolverFailures(t *testing.T) {
	storeErr := errors.New("disk on fire")
	readFailing := credentialStoreFuncs{
		read: func(context.Context, string) (Credential, bool, error) {
			return Credential{}, false, storeErr
		},
	}
	_, err := ResolveProviderAuth(
		context.Background(),
		"provider",
		ProviderAuth{APIKey: &APIKeyAuth{}},
		readFailing,
		nil,
		AuthResolutionOverrides{},
	)
	assertModelsError(t, err, ModelsErrorAuth, storeErr)

	modifyFailing := credentialStoreFuncs{
		read: func(context.Context, string) (Credential, bool, error) {
			return Credential{Type: CredentialTypeOAuth, Expires: 0}, true, nil
		},
		modify: func(context.Context, string, CredentialModifier) (Credential, bool, error) {
			return Credential{}, false, storeErr
		},
	}
	_, err = ResolveProviderAuth(
		context.Background(),
		"provider",
		ProviderAuth{
			OAuth: &OAuthAuth{
				Refresh: func(_ context.Context, credential Credential) (Credential, error) {
					return credential, nil
				},
				ToAuth: func(context.Context, Credential) (ModelAuth, error) {
					return ModelAuth{}, nil
				},
			},
		},
		modifyFailing,
		nil,
		AuthResolutionOverrides{},
	)
	assertModelsError(t, err, ModelsErrorAuth, storeErr)

	resolverErr := errors.New("command failed")
	_, err = ResolveProviderAuth(
		context.Background(),
		"provider",
		ProviderAuth{
			APIKey: &APIKeyAuth{
				Resolve: func(context.Context, APIKeyResolveInput) (*AuthResult, error) {
					return nil, resolverErr
				},
			},
		},
		NewInMemoryCredentialStore(),
		nil,
		AuthResolutionOverrides{},
	)
	assertModelsError(t, err, ModelsErrorAuth, resolverErr)
}

func TestResolveProviderAuthOverlaysRequestEnvironment(t *testing.T) {
	store := NewInMemoryCredentialStore(map[string]Credential{
		"provider": {
			Type: CredentialTypeAPIKey,
			Key:  "stored-key",
			Env:  ProviderEnv{"ACCOUNT_ID": "stored-account", "REGION": "stored-region"},
		},
	})
	auth := ProviderAuth{
		APIKey: &APIKeyAuth{
			Resolve: func(ctx context.Context, input APIKeyResolveInput) (*AuthResult, error) {
				account := input.Credential.Env["ACCOUNT_ID"]
				region := input.Credential.Env["REGION"]
				ambient, _, err := input.Context.Env(ctx, "AMBIENT")
				if err != nil {
					return nil, err
				}
				return &AuthResult{
					Auth: ModelAuth{
						APIKey:  input.Credential.Key,
						BaseURL: "https://" + account + "." + region,
					},
					Env: ProviderEnv{
						"ACCOUNT_ID": account,
						"REGION":     region,
						"AMBIENT":    ambient,
					},
				}, nil
			},
		},
	}

	result, err := ResolveProviderAuth(
		context.Background(),
		"provider",
		auth,
		store,
		mapAuthContext(map[string]string{"AMBIENT": "process"}),
		AuthResolutionOverrides{
			Env: ProviderEnv{"ACCOUNT_ID": "request-account"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Auth.BaseURL != "https://request-account.stored-region" {
		t.Fatalf("overlaid result = %#v", result)
	}
	if result.Env["AMBIENT"] != "process" {
		t.Fatalf("ambient env = %#v", result.Env)
	}
}

func TestProcessAuthContext(t *testing.T) {
	t.Setenv("GI_AUTH_CONTEXT_TEST", "value")
	authContext := ProcessAuthContext{}
	value, ok, err := authContext.Env(context.Background(), "GI_AUTH_CONTEXT_TEST")
	if err != nil || !ok || value != "value" {
		t.Fatalf("environment result: value=%q ok=%v err=%v", value, ok, err)
	}

	path := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	exists, err := authContext.FileExists(context.Background(), path)
	if err != nil || !exists {
		t.Fatalf("file result: exists=%v err=%v", exists, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := authContext.Env(ctx, "GI_AUTH_CONTEXT_TEST"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled env error = %v", err)
	}
}

func mapAuthContext(values map[string]string) AuthContext {
	return AuthContextFuncs{
		EnvFunc: func(ctx context.Context, name string) (string, bool, error) {
			if err := ctx.Err(); err != nil {
				return "", false, err
			}
			value, ok := values[name]
			return value, ok, nil
		},
		FileExistsFunc: func(ctx context.Context, _ string) (bool, error) {
			return false, ctx.Err()
		},
	}
}

func assertModelsError(t *testing.T, err error, code ModelsErrorCode, cause error) {
	t.Helper()
	var modelsErr *ModelsError
	if !errors.As(err, &modelsErr) {
		t.Fatalf("error = %#v, want ModelsError", err)
	}
	if modelsErr.Code != code {
		t.Fatalf("ModelsError code = %q, want %q", modelsErr.Code, code)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("error %v does not unwrap %v", err, cause)
	}
}

type countingCredentialStore struct {
	CredentialStore
	modifies atomic.Int32
}

func (s *countingCredentialStore) ModifyCredential(
	ctx context.Context,
	providerID string,
	modify CredentialModifier,
) (Credential, bool, error) {
	s.modifies.Add(1)
	return s.CredentialStore.ModifyCredential(ctx, providerID, modify)
}

type credentialStoreFuncs struct {
	read   func(context.Context, string) (Credential, bool, error)
	list   func(context.Context) ([]CredentialInfo, error)
	modify func(context.Context, string, CredentialModifier) (Credential, bool, error)
	delete func(context.Context, string) error
}

func (s credentialStoreFuncs) ReadCredential(
	ctx context.Context,
	providerID string,
) (Credential, bool, error) {
	if s.read == nil {
		return Credential{}, false, nil
	}
	return s.read(ctx, providerID)
}

func (s credentialStoreFuncs) ListCredentials(ctx context.Context) ([]CredentialInfo, error) {
	if s.list == nil {
		return nil, nil
	}
	return s.list(ctx)
}

func (s credentialStoreFuncs) ModifyCredential(
	ctx context.Context,
	providerID string,
	modify CredentialModifier,
) (Credential, bool, error) {
	if s.modify == nil {
		return Credential{}, false, nil
	}
	return s.modify(ctx, providerID, modify)
}

func (s credentialStoreFuncs) DeleteCredential(ctx context.Context, providerID string) error {
	if s.delete == nil {
		return nil
	}
	return s.delete(ctx, providerID)
}
