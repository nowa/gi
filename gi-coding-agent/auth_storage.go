package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type AuthCredential struct {
	Type    string `json:"type"`
	Key     string `json:"key,omitempty"`
	Access  string `json:"access,omitempty"`
	Refresh string `json:"refresh,omitempty"`
	Expires int64  `json:"expires,omitempty"`
}

type AuthStorageData map[string]AuthCredential

type AuthStatus struct {
	Configured bool   `json:"configured"`
	Source     string `json:"source,omitempty"`
	Label      string `json:"label,omitempty"`
}

type AuthStorageOptions struct {
	IncludeFallback bool
}

type AuthStorageLockFunc func(current string) (next string, write bool, err error)

type AuthStorageBackend interface {
	WithLock(fn AuthStorageLockFunc) error
}

type FileAuthStorageBackend struct {
	authPath string
}

func NewFileAuthStorageBackend(authPath string) *FileAuthStorageBackend {
	return &FileAuthStorageBackend{authPath: authPath}
}

func (b *FileAuthStorageBackend) WithLock(fn AuthStorageLockFunc) error {
	if err := b.ensureFile(); err != nil {
		return err
	}
	mutex := authFileMutex(b.authPath)
	mutex.Lock()
	defer mutex.Unlock()

	content, err := os.ReadFile(b.authPath)
	if err != nil {
		return err
	}
	next, write, err := fn(string(content))
	if err != nil {
		return err
	}
	if !write {
		return nil
	}
	if err := os.WriteFile(b.authPath, []byte(next), 0o600); err != nil {
		return err
	}
	return os.Chmod(b.authPath, 0o600)
}

func (b *FileAuthStorageBackend) ensureFile() error {
	if err := os.MkdirAll(filepath.Dir(b.authPath), 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(b.authPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.WriteFile(b.authPath, []byte("{}"), 0o600); err != nil {
		return err
	}
	return os.Chmod(b.authPath, 0o600)
}

var authPathLocks sync.Map

func authFileMutex(path string) *sync.Mutex {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	lock, _ := authPathLocks.LoadOrStore(abs, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

type InMemoryAuthStorageBackend struct {
	mu    sync.Mutex
	value string
}

func NewInMemoryAuthStorageBackend(data AuthStorageData) *InMemoryAuthStorageBackend {
	return &InMemoryAuthStorageBackend{value: serializeAuthStorageData(data)}
}

func (b *InMemoryAuthStorageBackend) WithLock(fn AuthStorageLockFunc) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	next, write, err := fn(b.value)
	if err != nil {
		return err
	}
	if write {
		b.value = next
	}
	return nil
}

type AuthStorage struct {
	storage          AuthStorageBackend
	data             AuthStorageData
	runtimeOverrides map[string]string
	fallbackResolver func(provider string) string
	loadErr          error
	errors           []error
}

func NewAuthStorage(authPath string) *AuthStorage {
	return NewAuthStorageFromBackend(NewFileAuthStorageBackend(authPath))
}

func NewAuthStorageFromBackend(storage AuthStorageBackend) *AuthStorage {
	auth := &AuthStorage{
		storage:          storage,
		data:             AuthStorageData{},
		runtimeOverrides: map[string]string{},
	}
	auth.Reload()
	return auth
}

func NewInMemoryAuthStorage(data AuthStorageData) *AuthStorage {
	return NewAuthStorageFromBackend(NewInMemoryAuthStorageBackend(data))
}

func (s *AuthStorage) SetRuntimeAPIKey(provider, apiKey string) {
	s.runtimeOverrides[provider] = apiKey
}

func (s *AuthStorage) RemoveRuntimeAPIKey(provider string) {
	delete(s.runtimeOverrides, provider)
}

func (s *AuthStorage) SetFallbackResolver(resolver func(provider string) string) {
	s.fallbackResolver = resolver
}

func (s *AuthStorage) Reload() {
	var content string
	err := s.storage.WithLock(func(current string) (string, bool, error) {
		content = current
		return "", false, nil
	})
	if err != nil {
		s.loadErr = err
		s.recordError(err)
		return
	}
	data, err := parseAuthStorageData(content)
	if err != nil {
		s.loadErr = err
		s.recordError(err)
		return
	}
	s.data = data
	s.loadErr = nil
}

func (s *AuthStorage) Get(provider string) (AuthCredential, bool) {
	credential, ok := s.data[provider]
	return credential, ok
}

func (s *AuthStorage) Set(provider string, credential AuthCredential) {
	s.data[provider] = credential
	s.persistProviderChange(provider, &credential)
}

func (s *AuthStorage) Remove(provider string) {
	delete(s.data, provider)
	s.persistProviderChange(provider, nil)
}

func (s *AuthStorage) List() []string {
	providers := make([]string, 0, len(s.data))
	for provider := range s.data {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	return providers
}

func (s *AuthStorage) Has(provider string) bool {
	_, ok := s.data[provider]
	return ok
}

func (s *AuthStorage) HasAuth(provider string) bool {
	if key := s.runtimeOverrides[provider]; key != "" {
		return true
	}
	if s.Has(provider) {
		return true
	}
	if getProviderEnvAPIKey(provider) != "" {
		return true
	}
	if s.fallbackResolver != nil && s.fallbackResolver(provider) != "" {
		return true
	}
	return false
}

func (s *AuthStorage) GetAuthStatus(provider string) AuthStatus {
	if s.Has(provider) {
		return AuthStatus{Configured: true, Source: "stored"}
	}
	if _, ok := s.runtimeOverrides[provider]; ok {
		return AuthStatus{Configured: false, Source: "runtime", Label: "--api-key"}
	}
	if keys := findProviderEnvKeys(provider); len(keys) > 0 {
		return AuthStatus{Configured: false, Source: "environment", Label: keys[0]}
	}
	if s.fallbackResolver != nil && s.fallbackResolver(provider) != "" {
		return AuthStatus{Configured: false, Source: "fallback", Label: "custom provider config"}
	}
	return AuthStatus{Configured: false}
}

func (s *AuthStorage) GetAll() AuthStorageData {
	return cloneAuthStorageData(s.data)
}

func (s *AuthStorage) DrainErrors() []error {
	drained := append([]error(nil), s.errors...)
	s.errors = nil
	return drained
}

func (s *AuthStorage) GetAPIKey(provider string) (string, bool) {
	return s.GetAPIKeyWithOptions(provider, AuthStorageOptions{IncludeFallback: true})
}

func (s *AuthStorage) GetAPIKeyWithOptions(provider string, options AuthStorageOptions) (string, bool) {
	if runtimeKey := s.runtimeOverrides[provider]; runtimeKey != "" {
		return runtimeKey, true
	}

	credential, ok := s.data[provider]
	if ok && credential.Type == "api_key" {
		return ResolveConfigValue(credential.Key)
	}
	if ok && credential.Type == "oauth" {
		oauthProvider, ok := GetOAuthProvider(provider)
		if !ok {
			return "", false
		}
		if nowUnixMilli() < credential.Expires {
			return oauthProvider.APIKey(credential), true
		}
		apiKey, ok, err := s.refreshOAuthTokenWithLock(provider, oauthProvider)
		if err == nil {
			return apiKey, ok
		}
		s.recordError(err)
		s.Reload()
		if updated, ok := s.data[provider]; ok && updated.Type == "oauth" && nowUnixMilli() < updated.Expires {
			return oauthProvider.APIKey(updated), true
		}
		return "", false
	}

	if envKey := getProviderEnvAPIKey(provider); envKey != "" {
		return envKey, true
	}
	if options.IncludeFallback && s.fallbackResolver != nil {
		if fallback := s.fallbackResolver(provider); fallback != "" {
			return fallback, true
		}
	}
	return "", false
}

func (s *AuthStorage) persistProviderChange(provider string, credential *AuthCredential) {
	if s.loadErr != nil {
		return
	}
	err := s.storage.WithLock(func(current string) (string, bool, error) {
		currentData, err := parseAuthStorageData(current)
		if err != nil {
			return "", false, err
		}
		if credential == nil {
			delete(currentData, provider)
		} else {
			currentData[provider] = *credential
		}
		return serializeAuthStorageData(currentData), true, nil
	})
	if err != nil {
		s.recordError(err)
	}
}

func (s *AuthStorage) refreshOAuthTokenWithLock(providerID string, provider OAuthProvider) (string, bool, error) {
	var apiKey string
	var ok bool
	err := s.storage.WithLock(func(current string) (string, bool, error) {
		currentData, err := parseAuthStorageData(current)
		if err != nil {
			return "", false, err
		}
		s.data = currentData
		s.loadErr = nil

		credential, exists := currentData[providerID]
		if !exists || credential.Type != "oauth" {
			return "", false, nil
		}
		if nowUnixMilli() < credential.Expires {
			apiKey = provider.APIKey(credential)
			ok = true
			return "", false, nil
		}
		refreshed, err := provider.RefreshToken(credential)
		if err != nil {
			return "", false, err
		}
		refreshed.Type = "oauth"
		currentData[providerID] = refreshed
		s.data = currentData
		apiKey = provider.APIKey(refreshed)
		ok = apiKey != ""
		return serializeAuthStorageData(currentData), true, nil
	})
	return apiKey, ok, err
}

func (s *AuthStorage) recordError(err error) {
	if err != nil {
		s.errors = append(s.errors, err)
	}
}

func parseAuthStorageData(content string) (AuthStorageData, error) {
	if strings.TrimSpace(content) == "" {
		return AuthStorageData{}, nil
	}
	var data AuthStorageData
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		return nil, err
	}
	if data == nil {
		return AuthStorageData{}, nil
	}
	return data, nil
}

func serializeAuthStorageData(data AuthStorageData) string {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(content)
}

func cloneAuthStorageData(data AuthStorageData) AuthStorageData {
	cloned := make(AuthStorageData, len(data))
	for provider, credential := range data {
		cloned[provider] = credential
	}
	return cloned
}

type configValueCacheEntry struct {
	value string
	ok    bool
}

var (
	configValueCacheMu sync.Mutex
	configValueCache   = map[string]configValueCacheEntry{}
)

func ResolveConfigValue(config string) (string, bool) {
	if strings.HasPrefix(config, "!") {
		return resolveCommandConfigValue(config)
	}
	if value := os.Getenv(config); value != "" {
		return value, true
	}
	return config, true
}

func ResolveConfigValueUncached(config string) (string, bool) {
	if strings.HasPrefix(config, "!") {
		return executeConfigCommand(config[1:])
	}
	if value := os.Getenv(config); value != "" {
		return value, true
	}
	return config, true
}

func ClearConfigValueCache() {
	configValueCacheMu.Lock()
	defer configValueCacheMu.Unlock()
	clear(configValueCache)
}

func resolveCommandConfigValue(commandConfig string) (string, bool) {
	configValueCacheMu.Lock()
	entry, ok := configValueCache[commandConfig]
	configValueCacheMu.Unlock()
	if ok {
		return entry.value, entry.ok
	}

	value, resolved := executeConfigCommand(commandConfig[1:])
	configValueCacheMu.Lock()
	configValueCache[commandConfig] = configValueCacheEntry{value: value, ok: resolved}
	configValueCacheMu.Unlock()
	return value, resolved
}

func executeConfigCommand(command string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", false
	}
	return value, true
}

type OAuthProvider struct {
	ID           string
	Name         string
	RefreshToken func(AuthCredential) (AuthCredential, error)
	GetAPIKey    func(AuthCredential) string
}

func (p OAuthProvider) APIKey(credential AuthCredential) string {
	if p.GetAPIKey != nil {
		return p.GetAPIKey(credential)
	}
	return credential.Access
}

var (
	oauthProvidersMu sync.Mutex
	oauthProviders   = map[string]OAuthProvider{}
)

func RegisterOAuthProvider(provider OAuthProvider) {
	oauthProvidersMu.Lock()
	defer oauthProvidersMu.Unlock()
	oauthProviders[provider.ID] = provider
}

func UnregisterOAuthProvider(providerID string) {
	oauthProvidersMu.Lock()
	defer oauthProvidersMu.Unlock()
	delete(oauthProviders, providerID)
}

func GetOAuthProvider(providerID string) (OAuthProvider, bool) {
	oauthProvidersMu.Lock()
	defer oauthProvidersMu.Unlock()
	provider, ok := oauthProviders[providerID]
	return provider, ok
}

func GetOAuthProviders() []OAuthProvider {
	oauthProvidersMu.Lock()
	defer oauthProvidersMu.Unlock()
	providers := make([]OAuthProvider, 0, len(oauthProviders))
	for _, provider := range oauthProviders {
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].ID < providers[j].ID })
	return providers
}

func ResetOAuthProviders() {
	oauthProvidersMu.Lock()
	defer oauthProvidersMu.Unlock()
	oauthProviders = map[string]OAuthProvider{}
}

var ErrAuthStorageLockCompromised = errors.New("auth storage lock was compromised")

func nowUnixMilli() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func findProviderEnvKeys(provider string) []string {
	envKeys := providerEnvKeys(provider)
	found := make([]string, 0, len(envKeys))
	for _, key := range envKeys {
		if os.Getenv(key) != "" {
			found = append(found, key)
		}
	}
	if len(found) == 0 {
		return nil
	}
	return found
}

func getProviderEnvAPIKey(provider string) string {
	keys := findProviderEnvKeys(provider)
	if len(keys) > 0 {
		return os.Getenv(keys[0])
	}
	if provider == "amazon-bedrock" {
		if os.Getenv("AWS_PROFILE") != "" ||
			(os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "") ||
			os.Getenv("AWS_BEARER_TOKEN_BEDROCK") != "" ||
			os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "" ||
			os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI") != "" ||
			os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") != "" {
			return "<authenticated>"
		}
	}
	return ""
}

func providerEnvKeys(provider string) []string {
	switch provider {
	case "github-copilot":
		return []string{"COPILOT_GITHUB_TOKEN"}
	case "anthropic":
		return []string{"ANTHROPIC_OAUTH_TOKEN", "ANTHROPIC_API_KEY"}
	case "openai":
		return []string{"OPENAI_API_KEY"}
	case "azure-openai-responses":
		return []string{"AZURE_OPENAI_API_KEY"}
	case "deepseek":
		return []string{"DEEPSEEK_API_KEY"}
	case "google":
		return []string{"GEMINI_API_KEY"}
	case "google-vertex":
		return []string{"GOOGLE_CLOUD_API_KEY"}
	case "groq":
		return []string{"GROQ_API_KEY"}
	case "cerebras":
		return []string{"CEREBRAS_API_KEY"}
	case "xai":
		return []string{"XAI_API_KEY"}
	case "openrouter":
		return []string{"OPENROUTER_API_KEY"}
	case "vercel-ai-gateway":
		return []string{"AI_GATEWAY_API_KEY"}
	case "zai":
		return []string{"ZAI_API_KEY"}
	case "mistral":
		return []string{"MISTRAL_API_KEY"}
	case "minimax":
		return []string{"MINIMAX_API_KEY"}
	case "minimax-cn":
		return []string{"MINIMAX_CN_API_KEY"}
	case "moonshotai", "moonshotai-cn":
		return []string{"MOONSHOT_API_KEY"}
	case "huggingface":
		return []string{"HF_TOKEN"}
	case "fireworks":
		return []string{"FIREWORKS_API_KEY"}
	case "together":
		return []string{"TOGETHER_API_KEY"}
	case "opencode", "opencode-go":
		return []string{"OPENCODE_API_KEY"}
	case "kimi-coding":
		return []string{"KIMI_API_KEY"}
	case "cloudflare-workers-ai", "cloudflare-ai-gateway":
		return []string{"CLOUDFLARE_API_KEY"}
	case "xiaomi":
		return []string{"XIAOMI_API_KEY"}
	case "xiaomi-token-plan-cn":
		return []string{"XIAOMI_TOKEN_PLAN_CN_API_KEY"}
	case "xiaomi-token-plan-ams":
		return []string{"XIAOMI_TOKEN_PLAN_AMS_API_KEY"}
	case "xiaomi-token-plan-sgp":
		return []string{"XIAOMI_TOKEN_PLAN_SGP_API_KEY"}
	default:
		return nil
	}
}
