package gicodingagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
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

	t.Run("failed commands are retried", func(t *testing.T) {
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
		if got := readCounterFile(t, counterFile); got != 2 {
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
