package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type AuthCredential = llm.Credential

type AuthStorageData map[string]AuthCredential

type AuthStatus struct {
	Configured bool   `json:"configured"`
	Source     string `json:"source,omitempty"`
	Label      string `json:"label,omitempty"`
}

type AuthStorageOptions struct {
	IncludeFallback    bool
	ExcludeEnvironment bool
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
	mutex := authFileMutex(b.authPath)
	mutex.Lock()
	defer mutex.Unlock()
	if err := b.ensureFile(); err != nil {
		return err
	}

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
	stateMu          sync.RWMutex
	operationsMu     sync.Mutex
	storage          AuthStorageBackend
	data             AuthStorageData
	runtimeOverrides map[string]string
	fallbackResolver func(provider string) string
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

// ReadStoredCredential performs a one-off raw credential read without
// constructing a store or resolving API-key commands and templates.
func ReadStoredCredential(providerID string, authPath ...string) (AuthCredential, bool) {
	path := GetAuthPath()
	if len(authPath) > 0 && strings.TrimSpace(authPath[0]) != "" {
		path = authPath[0]
	}
	content, err := os.ReadFile(ExpandPath(path))
	if err != nil {
		return AuthCredential{}, false
	}
	data, err := parseAuthStorageData(string(content))
	if err != nil {
		return AuthCredential{}, false
	}
	credential, ok := data[providerID]
	return credential.Clone(), ok
}

func (s *AuthStorage) SetRuntimeAPIKey(provider, apiKey string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.runtimeOverrides[provider] = apiKey
}

func (s *AuthStorage) RemoveRuntimeAPIKey(provider string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	delete(s.runtimeOverrides, provider)
}

func (s *AuthStorage) HasRuntimeAPIKey(provider string) bool {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	_, ok := s.runtimeOverrides[provider]
	return ok
}

func (s *AuthStorage) SetFallbackResolver(resolver func(provider string) string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.fallbackResolver = resolver
}

func (s *AuthStorage) Reload() {
	s.operationsMu.Lock()
	defer s.operationsMu.Unlock()

	var content string
	err := s.storage.WithLock(func(current string) (string, bool, error) {
		content = current
		return "", false, nil
	})
	if err != nil {
		s.recordError(err)
		return
	}
	data, err := parseAuthStorageData(content)
	if err != nil {
		s.recordError(err)
		return
	}
	s.stateMu.Lock()
	s.data = data
	s.stateMu.Unlock()
}

func (s *AuthStorage) Get(provider string) (AuthCredential, bool) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	credential, ok := s.data[provider]
	return credential.Clone(), ok
}

func (s *AuthStorage) Set(provider string, credential AuthCredential) {
	_, _, err := s.ModifyCredential(
		context.Background(),
		provider,
		func(context.Context, llm.Credential, bool) (llm.Credential, bool, error) {
			return credential.Clone(), true, nil
		},
	)
	if err == nil {
		return
	}
	s.stateMu.Lock()
	s.data[provider] = credential.Clone()
	s.stateMu.Unlock()
	s.recordError(err)
}

func (s *AuthStorage) Remove(provider string) {
	if err := s.DeleteCredential(context.Background(), provider); err == nil {
		return
	} else {
		s.stateMu.Lock()
		delete(s.data, provider)
		s.stateMu.Unlock()
		s.recordError(err)
	}
}

func (s *AuthStorage) List() []string {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	providers := make([]string, 0, len(s.data))
	for provider := range s.data {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	return providers
}

func (s *AuthStorage) Has(provider string) bool {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	_, ok := s.data[provider]
	return ok
}

func (s *AuthStorage) HasAuth(provider string) bool {
	s.stateMu.RLock()
	if key := s.runtimeOverrides[provider]; key != "" {
		s.stateMu.RUnlock()
		return true
	}
	if _, ok := s.data[provider]; ok {
		s.stateMu.RUnlock()
		return true
	}
	fallbackResolver := s.fallbackResolver
	s.stateMu.RUnlock()
	if getProviderEnvAPIKey(provider) != "" {
		return true
	}
	if fallbackResolver != nil && fallbackResolver(provider) != "" {
		return true
	}
	return false
}

func (s *AuthStorage) GetAuthStatus(provider string) AuthStatus {
	s.stateMu.RLock()
	_, stored := s.data[provider]
	_, runtimeOverride := s.runtimeOverrides[provider]
	fallbackResolver := s.fallbackResolver
	s.stateMu.RUnlock()
	if runtimeOverride {
		return AuthStatus{Configured: true, Source: "runtime", Label: "--api-key"}
	}
	if stored {
		return AuthStatus{Configured: true, Source: "stored"}
	}
	if keys := findProviderEnvKeys(provider); len(keys) > 0 {
		return AuthStatus{Configured: true, Source: "environment", Label: keys[0]}
	}
	if fallbackResolver != nil && fallbackResolver(provider) != "" {
		return AuthStatus{Configured: true, Source: "fallback", Label: "custom provider config"}
	}
	return AuthStatus{Configured: false}
}

func (s *AuthStorage) GetAll() AuthStorageData {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return cloneAuthStorageData(s.data)
}

func (s *AuthStorage) DrainErrors() []error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	drained := append([]error(nil), s.errors...)
	s.errors = nil
	return drained
}

func (s *AuthStorage) GetAPIKey(provider string) (string, bool) {
	return s.GetAPIKeyWithOptions(provider, AuthStorageOptions{IncludeFallback: true})
}

func (s *AuthStorage) GetAPIKeyWithOptions(provider string, options AuthStorageOptions) (string, bool) {
	s.stateMu.RLock()
	runtimeKey := s.runtimeOverrides[provider]
	credential, stored := s.data[provider]
	credential = credential.Clone()
	fallbackResolver := s.fallbackResolver
	s.stateMu.RUnlock()

	if runtimeKey != "" {
		return runtimeKey, true
	}

	if stored && credential.Type == llm.CredentialTypeAPIKey {
		return ResolveConfigValueWithEnv(credential.Key, credential.Env)
	}
	if stored && credential.Type == llm.CredentialTypeOAuth {
		oauthProvider, registered := GetOAuthProvider(provider)
		if !registered {
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
		if updated, ok := s.Get(provider); ok && updated.Type == llm.CredentialTypeOAuth && nowUnixMilli() < updated.Expires {
			return oauthProvider.APIKey(updated), true
		}
		return "", false
	}

	if !options.ExcludeEnvironment {
		if envKey := getProviderEnvAPIKey(provider); envKey != "" {
			return envKey, true
		}
	}
	if options.IncludeFallback && fallbackResolver != nil {
		if fallback := fallbackResolver(provider); fallback != "" {
			return fallback, true
		}
	}
	return "", false
}

var _ llm.CredentialStore = (*AuthStorage)(nil)

func (s *AuthStorage) ReadCredential(
	ctx context.Context,
	providerID string,
) (llm.Credential, bool, error) {
	if err := authStorageContextError(ctx); err != nil {
		return llm.Credential{}, false, err
	}
	s.stateMu.RLock()
	if apiKey := s.runtimeOverrides[providerID]; apiKey != "" {
		s.stateMu.RUnlock()
		return llm.Credential{
			Type: llm.CredentialTypeAPIKey,
			Key:  apiKey,
		}, true, nil
	}
	credential, ok := s.data[providerID]
	s.stateMu.RUnlock()
	credential = credential.Clone()
	if ok && credential.Type == llm.CredentialTypeAPIKey && credential.Key != "" {
		credential.Key, _ = ResolveConfigValueWithEnv(credential.Key, credential.Env)
	}
	return credential.Clone(), ok, nil
}

func (s *AuthStorage) ListCredentials(ctx context.Context) ([]llm.CredentialInfo, error) {
	if err := authStorageContextError(ctx); err != nil {
		return nil, err
	}
	s.stateMu.RLock()
	entries := make(map[string]llm.CredentialInfo, len(s.data)+len(s.runtimeOverrides))
	for providerID, credential := range s.data {
		entries[providerID] = llm.CredentialInfo{
			ProviderID: providerID,
			Type:       credential.Type,
		}
	}
	for providerID := range s.runtimeOverrides {
		entries[providerID] = llm.CredentialInfo{
			ProviderID: providerID,
			Type:       llm.CredentialTypeAPIKey,
		}
	}
	s.stateMu.RUnlock()
	result := make([]llm.CredentialInfo, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ProviderID < result[j].ProviderID
	})
	return result, nil
}

func (s *AuthStorage) ModifyCredential(
	ctx context.Context,
	providerID string,
	modify llm.CredentialModifier,
) (llm.Credential, bool, error) {
	if modify == nil {
		return llm.Credential{}, false, errors.New("credential modifier is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return llm.Credential{}, false, err
	}
	s.operationsMu.Lock()
	defer s.operationsMu.Unlock()
	if err := ctx.Err(); err != nil {
		return llm.Credential{}, false, err
	}

	var (
		post     llm.Credential
		exists   bool
		snapshot AuthStorageData
	)
	err := s.storage.WithLock(func(current string) (string, bool, error) {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		currentData, err := parseAuthStorageData(current)
		if err != nil {
			return "", false, err
		}
		stored, storedExists := currentData[providerID]
		stored = stored.Clone()
		next, write, err := modify(ctx, stored.Clone(), storedExists)
		if err != nil {
			return "", false, err
		}
		if !write {
			post = stored
			exists = storedExists
			snapshot = cloneAuthStorageData(currentData)
			return "", false, nil
		}
		next = next.Clone()
		currentData[providerID] = next
		serialized, err := marshalAuthStorageData(currentData)
		if err != nil {
			return "", false, err
		}
		post = next
		exists = true
		snapshot = cloneAuthStorageData(currentData)
		return serialized, true, nil
	})
	if err != nil {
		return llm.Credential{}, false, err
	}
	s.replaceCredentialSnapshot(snapshot)
	return post.Clone(), exists, nil
}

func (s *AuthStorage) DeleteCredential(ctx context.Context, providerID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.stateMu.Lock()
	delete(s.runtimeOverrides, providerID)
	s.stateMu.Unlock()
	s.operationsMu.Lock()
	defer s.operationsMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	var snapshot AuthStorageData
	err := s.storage.WithLock(func(current string) (string, bool, error) {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		currentData, err := parseAuthStorageData(current)
		if err != nil {
			return "", false, err
		}
		if _, exists := currentData[providerID]; !exists {
			snapshot = cloneAuthStorageData(currentData)
			return "", false, nil
		}
		delete(currentData, providerID)
		serialized, err := marshalAuthStorageData(currentData)
		if err != nil {
			return "", false, err
		}
		snapshot = cloneAuthStorageData(currentData)
		return serialized, true, nil
	})
	if err != nil {
		return err
	}
	s.replaceCredentialSnapshot(snapshot)
	return nil
}

func (s *AuthStorage) replaceCredentialSnapshot(snapshot AuthStorageData) {
	if snapshot == nil {
		snapshot = AuthStorageData{}
	}
	s.stateMu.Lock()
	s.data = snapshot
	s.stateMu.Unlock()
}

func authStorageContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func (s *AuthStorage) refreshOAuthTokenWithLock(providerID string, provider OAuthProvider) (string, bool, error) {
	credential, exists, err := s.ModifyCredential(
		context.Background(),
		providerID,
		func(_ context.Context, current llm.Credential, exists bool) (llm.Credential, bool, error) {
			if !exists || current.Type != llm.CredentialTypeOAuth {
				return llm.Credential{}, false, nil
			}
			if nowUnixMilli() < current.Expires {
				return llm.Credential{}, false, nil
			}
			refreshed, err := provider.RefreshToken(current)
			if err != nil {
				return llm.Credential{}, false, err
			}
			refreshed.Type = llm.CredentialTypeOAuth
			return refreshed, true, nil
		},
	)
	if err != nil || !exists || credential.Type != llm.CredentialTypeOAuth {
		return "", false, err
	}
	apiKey := provider.APIKey(credential)
	return apiKey, apiKey != "", nil
}

func (s *AuthStorage) recordError(err error) {
	if err == nil {
		return
	}
	s.stateMu.Lock()
	s.errors = append(s.errors, err)
	s.stateMu.Unlock()
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
	content, err := marshalAuthStorageData(data)
	if err != nil {
		return "{}"
	}
	return content
}

func marshalAuthStorageData(data AuthStorageData) (string, error) {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func cloneAuthStorageData(data AuthStorageData) AuthStorageData {
	cloned := make(AuthStorageData, len(data))
	for provider, credential := range data {
		cloned[provider] = credential.Clone()
	}
	return cloned
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
	oauthProviders   = builtInOAuthProviderMap()
)

func builtInOAuthProviderMap() map[string]OAuthProvider {
	providers := map[string]OAuthProvider{}
	builtins, err := llm.BuiltinProviders()
	if err != nil {
		return providers
	}
	for _, provider := range builtins {
		if compatible, ok := providerOwnedOAuthCompatibility(provider); ok {
			providers[compatible.ID] = compatible
		}
	}
	return providers
}

func builtInOAuthProvider(providerID string) (OAuthProvider, bool) {
	provider, err := llm.NewBuiltinProvider(providerID)
	if err != nil {
		return OAuthProvider{}, false
	}
	return providerOwnedOAuthCompatibility(provider)
}

func providerOwnedOAuthCompatibility(
	provider *llm.Provider,
) (OAuthProvider, bool) {
	if provider == nil || provider.Auth.OAuth == nil {
		return OAuthProvider{}, false
	}
	oauth := provider.Auth.OAuth
	return OAuthProvider{
		ID:   provider.ID,
		Name: firstNonEmptyString(provider.Name, oauth.Name, provider.ID),
		RefreshToken: func(
			credential AuthCredential,
		) (AuthCredential, error) {
			if oauth.Refresh == nil {
				return AuthCredential{}, errors.New(
					"OAuth token refresh is not implemented for this provider",
				)
			}
			return oauth.Refresh(context.Background(), credential)
		},
		GetAPIKey: func(credential AuthCredential) string {
			if oauth.ToAuth == nil {
				return ""
			}
			auth, err := oauth.ToAuth(
				context.Background(),
				credential,
			)
			if err != nil {
				return ""
			}
			return auth.APIKey
		},
	}, true
}

func RegisterOAuthProvider(provider OAuthProvider) {
	oauthProvidersMu.Lock()
	defer oauthProvidersMu.Unlock()
	oauthProviders[provider.ID] = provider
}

func UnregisterOAuthProvider(providerID string) {
	oauthProvidersMu.Lock()
	defer oauthProvidersMu.Unlock()
	if provider, ok := builtInOAuthProvider(providerID); ok {
		oauthProviders[providerID] = provider
		return
	}
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
	oauthProviders = builtInOAuthProviderMap()
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
