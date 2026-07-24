package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAuthStorageAPIKeyResolution(t *testing.T) {
	t.Run("literal API key is returned directly", func(t *testing.T) {
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "sk-ant-literal-key"},
		})

		if got, ok := storage.GetAPIKey("anthropic"); !ok || got != "sk-ant-literal-key" {
			t.Fatalf("api key = %q, %v", got, ok)
		}
	})

	t.Run("apiKey with ! prefix executes command and uses stdout", func(t *testing.T) {
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "!echo test-api-key-from-command"},
		})

		if got, ok := storage.GetAPIKey("anthropic"); !ok || got != "test-api-key-from-command" {
			t.Fatalf("api key = %q, %v", got, ok)
		}
	})

	t.Run("apiKey with ! prefix trims whitespace from command output", func(t *testing.T) {
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "!echo '  spaced-key  '"},
		})

		if got, ok := storage.GetAPIKey("anthropic"); !ok || got != "spaced-key" {
			t.Fatalf("api key = %q, %v", got, ok)
		}
	})

	t.Run("apiKey with ! prefix handles multiline output (uses trimmed result)", func(t *testing.T) {
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "!printf 'line1\\nline2'"},
		})

		if got, ok := storage.GetAPIKey("anthropic"); !ok || got != "line1\nline2" {
			t.Fatalf("api key = %q, %v", got, ok)
		}
	})

	t.Run("apiKey with ! prefix returns undefined on command failure", func(t *testing.T) {
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "!exit 1"},
		})

		if got, ok := storage.GetAPIKey("anthropic"); ok || got != "" {
			t.Fatalf("api key = %q, %v", got, ok)
		}
	})

	t.Run("apiKey with ! prefix returns undefined on nonexistent command", func(t *testing.T) {
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "!nonexistent-command-12345"},
		})

		if got, ok := storage.GetAPIKey("anthropic"); ok || got != "" {
			t.Fatalf("api key = %q, %v", got, ok)
		}
	})

	t.Run("apiKey with ! prefix returns undefined on empty output", func(t *testing.T) {
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "!printf ''"},
		})

		if got, ok := storage.GetAPIKey("anthropic"); ok || got != "" {
			t.Fatalf("api key = %q, %v", got, ok)
		}
	})

	t.Run("apiKey as environment variable name resolves to env value", func(t *testing.T) {
		t.Setenv("TEST_AUTH_API_KEY_12345", "env-api-key-value")
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "TEST_AUTH_API_KEY_12345"},
		})

		if got, ok := storage.GetAPIKey("anthropic"); !ok || got != "env-api-key-value" {
			t.Fatalf("api key = %q, %v", got, ok)
		}
	})

	t.Run("apiKey as literal value is used directly when not an env var", func(t *testing.T) {
		t.Setenv("literal_api_key_value", "")
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "literal_api_key_value"},
		})

		if got, ok := storage.GetAPIKey("anthropic"); !ok || got != "literal_api_key_value" {
			t.Fatalf("api key = %q, %v", got, ok)
		}
	})

	t.Run("apiKey command can use shell features like pipes", func(t *testing.T) {
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "!echo 'hello world' | tr ' ' '-'"},
		})

		if got, ok := storage.GetAPIKey("anthropic"); !ok || got != "hello-world" {
			t.Fatalf("api key = %q, %v", got, ok)
		}
	})
}

func TestAuthStorageAPIKeyResolutionCaching(t *testing.T) {
	t.Run("command is only executed once per process", func(t *testing.T) {
		counterFile := writeCounterFile(t)
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: incrementCounterCommand(counterFile, "key-value", false)},
		})

		for i := 0; i < 3; i++ {
			if got, ok := storage.GetAPIKey("anthropic"); !ok || got != "key-value" {
				t.Fatalf("api key = %q, %v", got, ok)
			}
		}
		if got := readCounterFile(t, counterFile); got != 1 {
			t.Fatalf("counter = %d", got)
		}
	})

	t.Run("cache persists across AuthStorage instances", func(t *testing.T) {
		counterFile := writeCounterFile(t)
		authPath := writeTestAuthJSON(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: incrementCounterCommand(counterFile, "key-value", false)},
		})

		storage1 := NewAuthStorage(authPath)
		if got, ok := storage1.GetAPIKey("anthropic"); !ok || got != "key-value" {
			t.Fatalf("storage1 api key = %q, %v", got, ok)
		}
		storage2 := NewAuthStorage(authPath)
		if got, ok := storage2.GetAPIKey("anthropic"); !ok || got != "key-value" {
			t.Fatalf("storage2 api key = %q, %v", got, ok)
		}
		if got := readCounterFile(t, counterFile); got != 1 {
			t.Fatalf("counter = %d", got)
		}
	})

	t.Run("clearConfigValueCache allows command to run again", func(t *testing.T) {
		counterFile := writeCounterFile(t)
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: incrementCounterCommand(counterFile, "key-value", false)},
		})

		storage.GetAPIKey("anthropic")
		ClearConfigValueCache()
		storage.GetAPIKey("anthropic")

		if got := readCounterFile(t, counterFile); got != 2 {
			t.Fatalf("counter = %d", got)
		}
	})

	t.Run("different commands are cached separately", func(t *testing.T) {
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "!echo key-anthropic"},
			"openai":    {Type: "api_key", Key: "!echo key-openai"},
		})

		keyA, okA := storage.GetAPIKey("anthropic")
		keyB, okB := storage.GetAPIKey("openai")
		if !okA || !okB || keyA != "key-anthropic" || keyB != "key-openai" {
			t.Fatalf("keys = %q/%v %q/%v", keyA, okA, keyB, okB)
		}
	})

	t.Run("failed commands are cached until explicitly cleared", func(t *testing.T) {
		counterFile := writeCounterFile(t)
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: incrementCounterCommand(counterFile, "", true)},
		})

		if got, ok := storage.GetAPIKey("anthropic"); ok || got != "" {
			t.Fatalf("first api key = %q, %v", got, ok)
		}
		if got, ok := storage.GetAPIKey("anthropic"); ok || got != "" {
			t.Fatalf("second api key = %q, %v", got, ok)
		}
		if got := readCounterFile(t, counterFile); got != 1 {
			t.Fatalf("counter = %d", got)
		}
	})

	t.Run("environment variables are not cached (changes are picked up)", func(t *testing.T) {
		t.Setenv("TEST_AUTH_KEY_CACHE_TEST_98765", "first-value")
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "TEST_AUTH_KEY_CACHE_TEST_98765"},
		})

		if got, ok := storage.GetAPIKey("anthropic"); !ok || got != "first-value" {
			t.Fatalf("first api key = %q, %v", got, ok)
		}
		t.Setenv("TEST_AUTH_KEY_CACHE_TEST_98765", "second-value")
		if got, ok := storage.GetAPIKey("anthropic"); !ok || got != "second-value" {
			t.Fatalf("second api key = %q, %v", got, ok)
		}
	})
}

func TestAuthStorageOAuthCompromisedLockAllowsRetry(t *testing.T) {
	providerID := "test-oauth-provider-" + strings.ReplaceAll(time.Now().Format(time.RFC3339Nano), ":", "-")
	RegisterOAuthProvider(OAuthProvider{
		ID:   providerID,
		Name: "Test OAuth Provider",
		RefreshToken: func(credentials AuthCredential) (AuthCredential, error) {
			credentials.Access = "refreshed-access-token"
			credentials.Expires = nowUnixMilli() + 60_000
			return credentials, nil
		},
		GetAPIKey: func(credentials AuthCredential) string {
			return "Bearer " + credentials.Access
		},
	})
	t.Cleanup(func() { UnregisterOAuthProvider(providerID) })

	authPath := writeTestAuthJSON(t, AuthStorageData{
		providerID: {
			Type:    "oauth",
			Refresh: "refresh-token",
			Access:  "expired-access-token",
			Expires: nowUnixMilli() - 10_000,
		},
	})
	backend := &compromisingAuthBackend{delegate: NewFileAuthStorageBackend(authPath)}
	storage := NewAuthStorageFromBackend(backend)
	backend.FailNextLock()

	if got, ok := storage.GetAPIKey(providerID); ok || got != "" {
		t.Fatalf("first api key = %q, %v", got, ok)
	}
	if got, ok := storage.GetAPIKey(providerID); !ok || got != "Bearer refreshed-access-token" {
		t.Fatalf("second api key = %q, %v", got, ok)
	}
}

func TestAuthStoragePersistenceSemantics(t *testing.T) {
	t.Run("set preserves unrelated external edits", func(t *testing.T) {
		authPath := writeTestAuthJSON(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "old-anthropic"},
			"openai":    {Type: "api_key", Key: "openai-key"},
		})
		storage := NewAuthStorage(authPath)

		writeAuthStorageData(t, authPath, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "old-anthropic"},
			"openai":    {Type: "api_key", Key: "openai-key"},
			"google":    {Type: "api_key", Key: "google-key"},
		})
		storage.Set("anthropic", AuthCredential{Type: "api_key", Key: "new-anthropic"})

		updated := readAuthStorageData(t, authPath)
		if updated["anthropic"].Key != "new-anthropic" || updated["openai"].Key != "openai-key" || updated["google"].Key != "google-key" {
			t.Fatalf("updated auth = %#v", updated)
		}
	})

	t.Run("remove preserves unrelated external edits", func(t *testing.T) {
		authPath := writeTestAuthJSON(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "anthropic-key"},
			"openai":    {Type: "api_key", Key: "openai-key"},
		})
		storage := NewAuthStorage(authPath)

		writeAuthStorageData(t, authPath, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "anthropic-key"},
			"openai":    {Type: "api_key", Key: "openai-key"},
			"google":    {Type: "api_key", Key: "google-key"},
		})
		storage.Remove("anthropic")

		updated := readAuthStorageData(t, authPath)
		if _, ok := updated["anthropic"]; ok {
			t.Fatalf("anthropic should be removed: %#v", updated)
		}
		if updated["openai"].Key != "openai-key" || updated["google"].Key != "google-key" {
			t.Fatalf("updated auth = %#v", updated)
		}
	})

	t.Run("does not overwrite malformed auth file after load error", func(t *testing.T) {
		authPath := writeTestAuthJSON(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "anthropic-key"},
		})
		storage := NewAuthStorage(authPath)
		if err := os.WriteFile(authPath, []byte("{invalid-json"), 0o600); err != nil {
			t.Fatal(err)
		}

		storage.Reload()
		storage.Set("openai", AuthCredential{Type: "api_key", Key: "openai-key"})

		raw, err := os.ReadFile(authPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != "{invalid-json" {
			t.Fatalf("raw auth = %q", raw)
		}
	})

	t.Run("reload records parse errors and drainErrors clears buffer", func(t *testing.T) {
		authPath := writeTestAuthJSON(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "anthropic-key"},
		})
		storage := NewAuthStorage(authPath)
		if err := os.WriteFile(authPath, []byte("{invalid-json"), 0o600); err != nil {
			t.Fatal(err)
		}

		storage.Reload()
		if got, ok := storage.Get("anthropic"); !ok || got.Key != "anthropic-key" {
			t.Fatalf("stored credential = %#v, %v", got, ok)
		}
		firstDrain := storage.DrainErrors()
		if len(firstDrain) == 0 {
			t.Fatalf("expected parse errors")
		}
		secondDrain := storage.DrainErrors()
		if len(secondDrain) != 0 {
			t.Fatalf("second drain = %#v", secondDrain)
		}
	})
}

func TestAuthStorageStatusDoesNotExposeSecrets(t *testing.T) {
	storage := NewInMemoryAuthStorage(AuthStorageData{
		"anthropic": {Type: "api_key", Key: "secret-api-key"},
		"openai": {
			Type:    "oauth",
			Access:  "secret-access-token",
			Refresh: "secret-refresh-token",
			Expires: nowUnixMilli() + 1000,
		},
	})

	for _, provider := range []string{"anthropic", "openai"} {
		status := storage.GetAuthStatus(provider)
		if !reflect.DeepEqual(status, AuthStatus{Configured: true, Source: "stored"}) {
			t.Fatalf("%s status = %#v", provider, status)
		}
		encoded, err := json.Marshal(status)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"secret-api-key", "secret-access-token", "secret-refresh-token"} {
			if strings.Contains(string(encoded), secret) {
				t.Fatalf("status exposed secret %q: %s", secret, encoded)
			}
		}
	}
}

func TestAuthStorageRuntimeOverrides(t *testing.T) {
	t.Run("runtime override takes priority over auth.json", func(t *testing.T) {
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "!echo stored-key"},
		})
		storage.SetRuntimeAPIKey("anthropic", "runtime-key")

		if got, ok := storage.GetAPIKey("anthropic"); !ok || got != "runtime-key" {
			t.Fatalf("api key = %q, %v", got, ok)
		}
	})

	t.Run("removing runtime override falls back to auth.json", func(t *testing.T) {
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: "!echo stored-key"},
		})
		storage.SetRuntimeAPIKey("anthropic", "runtime-key")
		storage.RemoveRuntimeAPIKey("anthropic")

		if got, ok := storage.GetAPIKey("anthropic"); !ok || got != "stored-key" {
			t.Fatalf("api key = %q, %v", got, ok)
		}
	})
}

func TestAuthStorageCredentialStorePreservesCanonicalCredential(t *testing.T) {
	authPath := writeTestAuthJSON(t, AuthStorageData{})
	storage := NewAuthStorage(authPath)
	var credentials llm.CredentialStore = storage

	written, exists, err := credentials.ModifyCredential(
		context.Background(),
		"custom-oauth",
		func(context.Context, llm.Credential, bool) (llm.Credential, bool, error) {
			return llm.Credential{
				Type:    llm.CredentialTypeOAuth,
				Access:  "secret-access",
				Refresh: "secret-refresh",
				Expires: nowUnixMilli() + 60_000,
				Env:     llm.ProviderEnv{"ACCOUNT_ID": "account"},
				Metadata: map[string]any{
					"tenant": "tenant-a",
				},
			}, true, nil
		},
	)
	if err != nil || !exists || written.Access != "secret-access" {
		t.Fatalf("modify result: credential=%#v exists=%v err=%v", written, exists, err)
	}

	reloaded := NewAuthStorage(authPath)
	credential, exists, err := reloaded.ReadCredential(context.Background(), "custom-oauth")
	if err != nil || !exists {
		t.Fatalf("read result: exists=%v err=%v", exists, err)
	}
	if credential.Env["ACCOUNT_ID"] != "account" || credential.Metadata["tenant"] != "tenant-a" {
		t.Fatalf("canonical credential fields = %#v", credential)
	}

	infos, err := reloaded.ListCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []llm.CredentialInfo{{ProviderID: "custom-oauth", Type: llm.CredentialTypeOAuth}}
	if !reflect.DeepEqual(infos, want) {
		t.Fatalf("credential metadata = %#v, want %#v", infos, want)
	}
	encoded, err := json.Marshal(infos)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") {
		t.Fatalf("credential metadata exposed secrets: %s", encoded)
	}
}

func TestReadStoredCredentialReturnsRawCredential(t *testing.T) {
	authPath := writeTestAuthJSON(t, AuthStorageData{
		"provider": {
			Type: llm.CredentialTypeAPIKey,
			Key:  "$READ_STORED_CREDENTIAL_KEY",
			Env:  llm.ProviderEnv{"READ_STORED_CREDENTIAL_KEY": "resolved"},
		},
	})

	credential, exists := ReadStoredCredential("provider", authPath)
	if !exists || credential.Key != "$READ_STORED_CREDENTIAL_KEY" {
		t.Fatalf("raw credential = %#v, exists=%v", credential, exists)
	}
	credential.Env["READ_STORED_CREDENTIAL_KEY"] = "mutated"
	again, exists := ReadStoredCredential("provider", authPath)
	if !exists || again.Env["READ_STORED_CREDENTIAL_KEY"] != "resolved" {
		t.Fatalf("credential read shared mutable state: %#v, exists=%v", again, exists)
	}

	if _, exists := ReadStoredCredential("missing", authPath); exists {
		t.Fatal("missing credential reported present")
	}
	malformed := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(malformed, []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, exists := ReadStoredCredential("provider", malformed); exists {
		t.Fatal("malformed credential file reported a value")
	}
}

func TestAuthStorageCredentialStoreResolvesStoredAPIKey(t *testing.T) {
	t.Setenv("AUTH_STORAGE_SCOPED_KEY", "process-key")
	storage := NewInMemoryAuthStorage(AuthStorageData{
		"template": {
			Type: llm.CredentialTypeAPIKey,
			Key:  "Bearer $AUTH_STORAGE_SCOPED_KEY",
			Env:  llm.ProviderEnv{"AUTH_STORAGE_SCOPED_KEY": "credential-key"},
		},
		"missing": {
			Type: llm.CredentialTypeAPIKey,
			Key:  "$AUTH_STORAGE_MISSING_KEY",
		},
		"oauth": {
			Type:    llm.CredentialTypeOAuth,
			Access:  "$AUTH_STORAGE_SCOPED_KEY",
			Refresh: "refresh-token",
		},
	})

	template, exists, err := storage.ReadCredential(context.Background(), "template")
	if err != nil || !exists {
		t.Fatalf("template read: exists=%v err=%v", exists, err)
	}
	if template.Key != "Bearer credential-key" {
		t.Fatalf("resolved template key = %q", template.Key)
	}
	if template.Env["AUTH_STORAGE_SCOPED_KEY"] != "credential-key" {
		t.Fatalf("credential environment was not preserved: %#v", template.Env)
	}
	raw, exists := storage.Get("template")
	if !exists || raw.Key != "Bearer $AUTH_STORAGE_SCOPED_KEY" {
		t.Fatalf("stored key was mutated: %#v, exists=%v", raw, exists)
	}

	missing, exists, err := storage.ReadCredential(context.Background(), "missing")
	if err != nil || !exists {
		t.Fatalf("missing read: exists=%v err=%v", exists, err)
	}
	if missing.Key != "" {
		t.Fatalf("unresolved key = %q, want empty", missing.Key)
	}

	oauth, exists, err := storage.ReadCredential(context.Background(), "oauth")
	if err != nil || !exists {
		t.Fatalf("OAuth read: exists=%v err=%v", exists, err)
	}
	if oauth.Access != "$AUTH_STORAGE_SCOPED_KEY" {
		t.Fatalf("OAuth credential was unexpectedly resolved: %#v", oauth)
	}
}

func TestAuthStorageCredentialListDoesNotResolveKeys(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test command uses POSIX shell syntax")
	}
	counter := writeCounterFile(t)
	storage := NewInMemoryAuthStorage(AuthStorageData{
		"provider": {
			Type: llm.CredentialTypeAPIKey,
			Key:  incrementCounterCommand(counter, "resolved-key", false),
		},
	})

	infos, err := storage.ListCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := infos, []llm.CredentialInfo{{ProviderID: "provider", Type: llm.CredentialTypeAPIKey}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("credential metadata = %#v, want %#v", got, want)
	}
	if got := readCounterFile(t, counter); got != 0 {
		t.Fatalf("listing executed the key command %d times", got)
	}

	credential, exists, err := storage.ReadCredential(context.Background(), "provider")
	if err != nil || !exists || credential.Key != "resolved-key" {
		t.Fatalf("resolved read: credential=%#v exists=%v err=%v", credential, exists, err)
	}
	if got := readCounterFile(t, counter); got != 1 {
		t.Fatalf("credential read executed the key command %d times", got)
	}
}

func TestAuthStorageCredentialStoreCommitsSuccessfulModifierAfterCancellation(t *testing.T) {
	authPath := writeTestAuthJSON(t, AuthStorageData{
		"provider": {
			Type:    llm.CredentialTypeOAuth,
			Access:  "old",
			Refresh: "old-refresh",
		},
	})
	storage := NewAuthStorage(authPath)
	ctx, cancel := context.WithCancel(context.Background())

	written, exists, err := storage.ModifyCredential(
		ctx,
		"provider",
		func(_ context.Context, current llm.Credential, _ bool) (llm.Credential, bool, error) {
			cancel()
			current.Access = "fresh"
			current.Refresh = "rotated"
			return current, true, nil
		},
	)
	if err != nil || !exists {
		t.Fatalf("modify result: exists=%v err=%v", exists, err)
	}
	if written.Access != "fresh" || written.Refresh != "rotated" {
		t.Fatalf("written credential = %#v", written)
	}
	persisted, exists, err := NewAuthStorage(authPath).ReadCredential(context.Background(), "provider")
	if err != nil || !exists {
		t.Fatalf("persisted credential: exists=%v err=%v", exists, err)
	}
	if persisted.Access != "fresh" || persisted.Refresh != "rotated" {
		t.Fatalf("persisted credential = %#v", persisted)
	}
}

func TestAuthStorageCredentialStoreRuntimeOverlay(t *testing.T) {
	storage := NewInMemoryAuthStorage(AuthStorageData{
		"provider": {
			Type:    llm.CredentialTypeOAuth,
			Access:  "stored-oauth",
			Refresh: "stored-refresh",
		},
		"stored-only": {
			Type: llm.CredentialTypeAPIKey,
			Key:  "stored-key",
		},
	})
	storage.SetRuntimeAPIKey("provider", "runtime-key")
	storage.SetRuntimeAPIKey("runtime-only", "runtime-only-key")
	if !storage.HasRuntimeAPIKey("provider") || storage.HasRuntimeAPIKey("missing") {
		t.Fatal("runtime API key presence did not match overrides")
	}

	credential, exists, err := storage.ReadCredential(context.Background(), "provider")
	if err != nil || !exists {
		t.Fatalf("read overlay: exists=%v err=%v", exists, err)
	}
	if credential.Type != llm.CredentialTypeAPIKey || credential.Key != "runtime-key" {
		t.Fatalf("runtime overlay = %#v", credential)
	}
	infos, err := storage.ListCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []llm.CredentialInfo{
		{ProviderID: "provider", Type: llm.CredentialTypeAPIKey},
		{ProviderID: "runtime-only", Type: llm.CredentialTypeAPIKey},
		{ProviderID: "stored-only", Type: llm.CredentialTypeAPIKey},
	}
	if !reflect.DeepEqual(infos, want) {
		t.Fatalf("overlay metadata = %#v, want %#v", infos, want)
	}

	_, _, err = storage.ModifyCredential(
		context.Background(),
		"provider",
		func(_ context.Context, current llm.Credential, exists bool) (llm.Credential, bool, error) {
			if !exists || current.Type != llm.CredentialTypeOAuth {
				t.Fatalf("modifier saw overlay instead of stored credential: %#v", current)
			}
			current.Access = "updated-oauth"
			return current, true, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	storage.RemoveRuntimeAPIKey("provider")
	if storage.HasRuntimeAPIKey("provider") {
		t.Fatal("runtime API key remained after removal")
	}
	credential, exists, err = storage.ReadCredential(context.Background(), "provider")
	if err != nil || !exists {
		t.Fatalf("read stored credential: exists=%v err=%v", exists, err)
	}
	if credential.Type != llm.CredentialTypeOAuth || credential.Access != "updated-oauth" {
		t.Fatalf("stored credential after overlay removal = %#v", credential)
	}

	storage.SetRuntimeAPIKey("provider", "runtime-key-again")
	if err := storage.DeleteCredential(context.Background(), "provider"); err != nil {
		t.Fatal(err)
	}
	if storage.HasRuntimeAPIKey("provider") {
		t.Fatal("delete did not clear provider runtime API key")
	}
	if _, exists, err := storage.ReadCredential(context.Background(), "provider"); err != nil || exists {
		t.Fatalf("provider credential after delete: exists=%v err=%v", exists, err)
	}
	if _, exists := storage.Get("provider"); exists {
		t.Fatal("delete did not clear persisted provider credential")
	}
	if storedOnly, exists := storage.Get("stored-only"); !exists || storedOnly.Key != "stored-key" {
		t.Fatalf("delete changed unrelated credential: %#v, exists=%v", storedOnly, exists)
	}

	if err := storage.DeleteCredential(context.Background(), "runtime-only"); err != nil {
		t.Fatal(err)
	}
	if storage.HasRuntimeAPIKey("runtime-only") {
		t.Fatal("delete did not clear runtime API key")
	}
	if _, exists, err := storage.ReadCredential(context.Background(), "runtime-only"); err != nil || exists {
		t.Fatalf("runtime credential after delete: exists=%v err=%v", exists, err)
	}
}

func TestAuthStorageCredentialStoreSerializesRefreshAcrossInstances(t *testing.T) {
	authPath := writeTestAuthJSON(t, AuthStorageData{
		"provider": {
			Type:    llm.CredentialTypeOAuth,
			Access:  "expired",
			Refresh: "old-refresh",
			Expires: 0,
		},
	})
	first := NewAuthStorage(authPath)
	second := NewAuthStorage(authPath)

	var refreshes atomic.Int32
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	auth := llm.ProviderAuth{
		OAuth: &llm.OAuthAuth{
			Refresh: func(_ context.Context, _ llm.Credential) (llm.Credential, error) {
				if refreshes.Add(1) == 1 {
					close(refreshStarted)
				}
				<-releaseRefresh
				return llm.Credential{
					Type:    llm.CredentialTypeOAuth,
					Access:  "fresh",
					Refresh: "rotated",
					Expires: nowUnixMilli() + 60_000,
				}, nil
			},
			ToAuth: func(_ context.Context, credential llm.Credential) (llm.ModelAuth, error) {
				return llm.ModelAuth{APIKey: credential.Access}, nil
			},
		},
	}

	type resolution struct {
		result *llm.AuthResult
		err    error
	}
	results := make(chan resolution, 2)
	resolve := func(storage *AuthStorage) {
		result, err := llm.ResolveProviderAuth(
			context.Background(),
			"provider",
			auth,
			storage,
			nil,
			llm.AuthResolutionOverrides{},
		)
		results <- resolution{result: result, err: err}
	}
	go resolve(first)
	go resolve(second)
	<-refreshStarted
	close(releaseRefresh)

	for range 2 {
		resolved := <-results
		if resolved.err != nil {
			t.Fatal(resolved.err)
		}
		if resolved.result == nil || resolved.result.Auth.APIKey != "fresh" {
			t.Fatalf("resolution = %#v", resolved.result)
		}
	}
	if got := refreshes.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	persisted, exists, err := NewAuthStorage(authPath).ReadCredential(context.Background(), "provider")
	if err != nil || !exists {
		t.Fatalf("persisted credential: exists=%v err=%v", exists, err)
	}
	if persisted.Access != "fresh" || persisted.Refresh != "rotated" {
		t.Fatalf("persisted credential = %#v", persisted)
	}
}

type compromisingAuthBackend struct {
	delegate *FileAuthStorageBackend
	mu       sync.Mutex
	failNext bool
}

func (b *compromisingAuthBackend) FailNextLock() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failNext = true
}

func (b *compromisingAuthBackend) WithLock(fn AuthStorageLockFunc) error {
	b.mu.Lock()
	if b.failNext {
		b.failNext = false
		b.mu.Unlock()
		return ErrAuthStorageLockCompromised
	}
	b.mu.Unlock()
	return b.delegate.WithLock(fn)
}

func newTestAuthStorage(t *testing.T, data AuthStorageData) *AuthStorage {
	t.Helper()
	return NewAuthStorage(writeTestAuthJSON(t, data))
}

func writeTestAuthJSON(t *testing.T, data AuthStorageData) string {
	t.Helper()
	ClearConfigValueCache()
	t.Cleanup(ClearConfigValueCache)
	authPath := filepath.Join(t.TempDir(), "auth.json")
	writeAuthStorageData(t, authPath, data)
	return authPath
}

func writeAuthStorageData(t *testing.T, path string, data AuthStorageData) {
	t.Helper()
	content, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readAuthStorageData(t *testing.T, path string) AuthStorageData {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var data AuthStorageData
	if err := json.Unmarshal(content, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func writeCounterFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "counter")
	if err := os.WriteFile(path, []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readCounterFile(t *testing.T, path string) int {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(content)), "%d", &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func incrementCounterCommand(counterFile, output string, fail bool) string {
	command := "count=$(cat " + shellQuote(counterFile) + "); echo $((count + 1)) > " + shellQuote(counterFile)
	if output != "" {
		command += "; echo " + shellQuote(output)
	}
	if fail {
		command += "; exit 1"
	}
	return "!" + command
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func TestAuthStorageCompromisingBackendReturnsExpectedError(t *testing.T) {
	backend := &compromisingAuthBackend{delegate: NewFileAuthStorageBackend(writeTestAuthJSON(t, AuthStorageData{}))}
	backend.FailNextLock()
	err := backend.WithLock(func(current string) (string, bool, error) {
		return "", false, nil
	})
	if !errors.Is(err, ErrAuthStorageLockCompromised) {
		t.Fatalf("error = %v", err)
	}
}
