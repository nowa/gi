package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	jsonutil "github.com/nowa/gi/gi-coding-agent/internal/jsonutil"
	llm "github.com/nowa/gi/gi-llm-provider"
)

type ProviderRequestConfig struct {
	APIKey     string
	Headers    map[string]string
	AuthHeader bool
}

type ProviderModelDefinition struct {
	ID               string
	Name             string
	API              string
	BaseURL          string
	Reasoning        bool
	ThinkingLevelMap map[string]*string
	Input            []string
	Cost             llm.ModelCost
	ContextWindow    int
	MaxTokens        int
	Headers          map[string]string
	Compat           llm.ModelCompat
}

type ModelCostOverride struct {
	Input      *float64
	Output     *float64
	CacheRead  *float64
	CacheWrite *float64
	Tiers      *[]llm.ModelCostTier
}

type ModelOverride struct {
	Name             string
	Reasoning        *bool
	ThinkingLevelMap map[string]*string
	Input            []string
	Cost             *ModelCostOverride
	ContextWindow    *int
	MaxTokens        *int
	Headers          map[string]string
	Compat           llm.ModelCompat
}

type ProviderConfigInput struct {
	Name           string
	BaseURL        string
	APIKey         string
	API            string
	Headers        map[string]string
	AuthHeader     *bool
	OAuth          *OAuthProvider                                                                                  `json:"-"`
	StreamSimple   func(llm.Model, llm.Context, llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) `json:"-"`
	RefreshModels  func(context.Context, llm.RefreshModelsContext) ([]ProviderModelDefinition, error)              `json:"-"`
	Compat         llm.ModelCompat
	Models         []ProviderModelDefinition
	ModelOverrides map[string]ModelOverride
}

type ResolvedRequestAuth struct {
	OK             bool
	APIKey         string
	Headers        map[string]string
	HeaderRemovals []string
	BaseURL        string
	Env            llm.ProviderEnv
	Error          string
}

type modelsJSONProviderConfig struct {
	Name           string                    `json:"name,omitempty"`
	BaseURL        string                    `json:"baseUrl,omitempty"`
	APIKey         string                    `json:"apiKey,omitempty"`
	API            string                    `json:"api,omitempty"`
	OAuth          string                    `json:"oauth,omitempty"`
	Headers        map[string]string         `json:"headers,omitempty"`
	AuthHeader     *bool                     `json:"authHeader,omitempty"`
	Compat         llm.ModelCompat           `json:"compat,omitempty"`
	Models         []ProviderModelDefinition `json:"models,omitempty"`
	ModelOverrides map[string]ModelOverride  `json:"modelOverrides,omitempty"`
}

type modelRegistryConfig struct {
	Providers map[string]modelsJSONProviderConfig `json:"providers"`
}

type customModelsResult struct {
	models          []llm.Model
	providers       map[string]modelsJSONProviderConfig
	overrides       map[string]ProviderConfigInput
	modelOverrides  map[string]map[string]ModelOverride
	radiusProviders []radiusRegistryProvider
	errorMessage    string
}

// ModelRegistryOptions supplies app-owned state and explicit startup network
// policy. Network catalog refresh is disabled by the zero value.
type ModelRegistryOptions struct {
	AuthStorage         *AuthStorage
	ModelsJSONPath      string
	ModelsStore         llm.ModelsStore
	RadiusClient        llm.HTTPDoer
	CatalogBaseURL      string
	CatalogClient       llm.HTTPDoer
	CatalogUserAgent    string
	ModelNetworkEnabled *bool
	AllowModelNetwork   bool
	ModelRefreshTimeout time.Duration
}

// ModelRegistryRefreshOptions controls an explicit dynamic catalog refresh.
// Network access is disabled by the zero value.
type ModelRegistryRefreshOptions struct {
	AllowNetwork bool
	Force        bool
	Timeout      time.Duration
}

type apiProviderRestore struct {
	api      string
	previous llm.APIProvider
}

type oauthProviderRestore struct {
	previous OAuthProvider
	had      bool
}

// ModelRegistry is the synchronous coding-agent compatibility facade.
// Mutations are serialized under mu and readers consume detached snapshots;
// the shared llm.Models, credential storage, and ModelsStore boundaries it
// composes are independently safe for concurrent use.
type ModelRegistry struct {
	mu sync.RWMutex

	authStorage              *AuthStorage
	modelsJSONPath           string
	modelsStore              llm.ModelsStore
	radiusClient             llm.HTTPDoer
	catalogBaseURL           string
	catalogClient            llm.HTTPDoer
	catalogUserAgent         string
	initialModelNetwork      bool
	modelNetworkEnabled      bool
	modelRefreshTimeout      time.Duration
	dynamicModels            *llm.Models
	baseModels               []llm.Model
	models                   []llm.Model
	modelsJSONProviders      map[string]modelsJSONProviderConfig
	configuredModelOverrides map[string]map[string]ModelOverride
	providerRequestConfigs   map[string]ProviderRequestConfig
	registeredProviders      map[string]ProviderConfigInput
	registeredOrder          []string
	apiProviderRestores      map[string]apiProviderRestore
	oauthProviderRestores    map[string]oauthProviderRestore
	loadError                string
	modelRuntime             *ModelRuntime
}

var builtInProviderDisplayNames = map[string]string{
	"anthropic":              "Anthropic",
	"amazon-bedrock":         "Amazon Bedrock",
	"azure-openai-responses": "Azure OpenAI Responses",
	"cerebras":               "Cerebras",
	"cloudflare-ai-gateway":  "Cloudflare AI Gateway",
	"cloudflare-workers-ai":  "Cloudflare Workers AI",
	"deepseek":               "DeepSeek",
	"fireworks":              "Fireworks",
	"google":                 "Google Gemini",
	"google-vertex":          "Google Vertex AI",
	"github-copilot":         "GitHub Copilot",
	"groq":                   "Groq",
	"huggingface":            "Hugging Face",
	"kimi-coding":            "Kimi For Coding",
	"mistral":                "Mistral",
	"minimax":                "MiniMax",
	"minimax-cn":             "MiniMax (China)",
	"moonshotai":             "Moonshot AI",
	"moonshotai-cn":          "Moonshot AI (China)",
	"opencode":               "OpenCode Zen",
	"opencode-go":            "OpenCode Go",
	"openai":                 "OpenAI",
	"openrouter":             "OpenRouter",
	RadiusProviderID:         "Radius",
	"together":               "Together AI",
	"vercel-ai-gateway":      "Vercel AI Gateway",
	"xai":                    "xAI",
	"zai":                    "ZAI",
	"xiaomi":                 "Xiaomi MiMo",
	"xiaomi-token-plan-cn":   "Xiaomi MiMo Token Plan (China)",
	"xiaomi-token-plan-ams":  "Xiaomi MiMo Token Plan (Amsterdam)",
	"xiaomi-token-plan-sgp":  "Xiaomi MiMo Token Plan (Singapore)",
}

func NewModelRegistry(authStorage *AuthStorage, modelsJSONPath string) *ModelRegistry {
	return NewModelRegistryWithOptions(context.Background(), ModelRegistryOptions{
		AuthStorage:    authStorage,
		ModelsJSONPath: modelsJSONPath,
	})
}

// NewModelRegistryWithOptions builds a registry and restores dynamic catalogs.
// A network refresh occurs only when AllowModelNetwork is explicitly true.
func NewModelRegistryWithOptions(
	ctx context.Context,
	options ModelRegistryOptions,
) *ModelRegistry {
	modelsStore := options.ModelsStore
	if modelsStore == nil {
		if options.ModelsJSONPath == "" {
			modelsStore = llm.NewInMemoryModelsStore()
		} else {
			modelsStore = NewFileModelsStore(filepath.Join(
				filepath.Dir(options.ModelsJSONPath),
				"models-store.json",
			))
		}
	}
	modelNetworkEnabled := !packageManagerOffline()
	if options.ModelNetworkEnabled != nil {
		modelNetworkEnabled = *options.ModelNetworkEnabled
	}
	allowInitialModelNetwork := options.AllowModelNetwork &&
		modelNetworkEnabled
	registry := &ModelRegistry{
		authStorage:            options.AuthStorage,
		modelsJSONPath:         options.ModelsJSONPath,
		modelsStore:            modelsStore,
		radiusClient:           options.RadiusClient,
		catalogBaseURL:         options.CatalogBaseURL,
		catalogClient:          options.CatalogClient,
		catalogUserAgent:       options.CatalogUserAgent,
		initialModelNetwork:    allowInitialModelNetwork,
		modelNetworkEnabled:    modelNetworkEnabled,
		modelRefreshTimeout:    options.ModelRefreshTimeout,
		modelsJSONProviders:    map[string]modelsJSONProviderConfig{},
		providerRequestConfigs: map[string]ProviderRequestConfig{},
		registeredProviders:    map[string]ProviderConfigInput{},
		apiProviderRestores:    map[string]apiProviderRestore{},
		oauthProviderRestores:  map[string]oauthProviderRestore{},
	}
	refreshCtx, cancel := modelRegistryRefreshContext(
		ctx,
		allowInitialModelNetwork,
		options.ModelRefreshTimeout,
	)
	defer cancel()
	registry.loadModels(refreshCtx, llm.ModelsRefreshOptions{
		Offline: !allowInitialModelNetwork,
	})
	return registry
}

func NewInMemoryModelRegistry(authStorage *AuthStorage) *ModelRegistry {
	return NewModelRegistry(authStorage, "")
}

func ClearAPIKeyCache() {
	ClearConfigValueCache()
}

func (r *ModelRegistry) Refresh() {
	if r == nil {
		return
	}
	r.mu.RLock()
	modelRuntime := r.modelRuntime
	r.mu.RUnlock()
	if modelRuntime != nil {
		_, _ = modelRuntime.Refresh(
			context.Background(),
			ModelRegistryRefreshOptions{},
		)
		return
	}
	r.refreshLocal()
}

func (r *ModelRegistry) refreshLocal() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshLocked()
}

func (r *ModelRegistry) refreshLocked() {
	r.providerRequestConfigs = map[string]ProviderRequestConfig{}
	r.loadError = ""
	r.loadModels(context.Background(), llm.ModelsRefreshOptions{Offline: true})
	legacyGlobals := r.modelRuntime == nil
	for _, providerName := range r.registeredOrder {
		config, ok := r.registeredProviders[providerName]
		if ok {
			r.applyProviderConfig(providerName, config, legacyGlobals)
		}
	}
	r.applyConfiguredModelOverrides()
	r.applyOAuthModelOverrides()
}

// RefreshModels refreshes dynamic provider catalogs without reparsing
// models.json. The zero options value only restores persisted state.
func (r *ModelRegistry) RefreshModels(
	ctx context.Context,
	options ModelRegistryRefreshOptions,
) llm.ModelsRefreshResult {
	if r == nil {
		return llm.ModelsRefreshResult{Errors: map[string]error{}}
	}
	r.mu.RLock()
	modelRuntime := r.modelRuntime
	r.mu.RUnlock()
	if modelRuntime != nil {
		result, err := modelRuntime.Refresh(ctx, options)
		if err != nil {
			if result.Errors == nil {
				result.Errors = map[string]error{}
			}
			result.Errors["model-runtime"] = err
		}
		return result
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dynamicModels == nil {
		return llm.ModelsRefreshResult{Errors: map[string]error{}}
	}
	refreshCtx, cancel := modelRegistryRefreshContext(
		ctx,
		options.AllowNetwork,
		options.Timeout,
	)
	defer cancel()
	result := r.dynamicModels.Refresh(
		refreshCtx,
		llm.ModelsRefreshOptions{
			Offline: !options.AllowNetwork,
			Force:   options.Force,
		},
	)
	r.syncDynamicModels()
	legacyGlobals := r.modelRuntime == nil
	for _, providerName := range r.registeredOrder {
		if config, ok := r.registeredProviders[providerName]; ok {
			r.applyProviderConfig(providerName, config, legacyGlobals)
		}
	}
	r.applyConfiguredModelOverrides()
	r.applyOAuthModelOverrides()
	return result
}

func (r *ModelRegistry) applyOAuthModelOverrides() {
	if r == nil || r.authStorage == nil {
		return
	}
	credential, ok := r.authStorage.Get("github-copilot")
	if !ok || credential.Type != "oauth" {
		return
	}
	baseURL := llm.GitHubCopilotBaseURL(credential.Access, credential.EnterpriseURL)
	if strings.TrimSpace(baseURL) == "" {
		return
	}
	for index := range r.models {
		if r.models[index].Provider == "github-copilot" {
			r.models[index].BaseURL = baseURL
		}
	}
}

func (r *ModelRegistry) GetError() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.loadError
}

func (r *ModelRegistry) GetAll() []llm.Model {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]llm.Model{}, r.models...)
}

func (r *ModelRegistry) publishRuntimeModels(models []llm.Model) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.models = cloneRuntimeModels(models)
	r.mu.Unlock()
}

func (r *ModelRegistry) GetAvailable() []llm.Model {
	models := r.GetAll()
	available := make([]llm.Model, 0, len(models))
	for _, model := range models {
		if r.HasConfiguredAuth(model) {
			available = append(available, model)
		}
	}
	return available
}

func (r *ModelRegistry) Find(provider, modelID string) (llm.Model, bool) {
	if r == nil {
		return llm.Model{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, model := range r.models {
		if model.Provider == provider && model.ID == modelID {
			return model, true
		}
	}
	return llm.Model{}, false
}

func (r *ModelRegistry) HasConfiguredAuth(model llm.Model) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	authStorage := r.authStorage
	config, hasConfig := r.providerRequestConfigs[model.Provider]
	dynamicModels := r.dynamicModels
	r.mu.RUnlock()
	if hasExplicitAuthStorage(authStorage, model.Provider) {
		return true
	}
	if hasConfig && config.APIKey != "" {
		return configuredAPIKeyAuthStatus(config.APIKey).Configured
	}
	if dynamicModels != nil {
		if _, ok := dynamicModels.GetProvider(model.Provider); ok {
			_, configured, err := dynamicModels.CheckAuth(
				context.Background(),
				model.Provider,
			)
			if err == nil && configured {
				return true
			}
		}
	}
	if authStorage != nil && authStorage.HasAuth(model.Provider) {
		return true
	}
	return false
}

func (r *ModelRegistry) GetAPIKeyForProvider(provider string) (string, bool) {
	if r == nil {
		return "", false
	}
	r.mu.RLock()
	authStorage := r.authStorage
	config, hasConfig := r.providerRequestConfigs[provider]
	dynamicModels := r.dynamicModels
	r.mu.RUnlock()
	if hasExplicitAuthStorage(authStorage, provider) {
		if dynamicModels != nil {
			if _, ok := dynamicModels.GetProvider(provider); ok {
				result, err := dynamicModels.GetAuth(
					context.Background(),
					provider,
					llm.AuthResolutionOverrides{},
				)
				if err == nil &&
					result != nil &&
					result.Auth.APIKey != "" {
					return result.Auth.APIKey, true
				}
				return "", false
			}
		}
		return authStorage.GetAPIKeyWithOptions(
			provider,
			AuthStorageOptions{
				ExcludeEnvironment: true,
			},
		)
	}
	if hasConfig && config.APIKey != "" {
		return ResolveConfigValueUncached(config.APIKey)
	}
	if dynamicModels != nil {
		if _, ok := dynamicModels.GetProvider(provider); ok {
			result, err := dynamicModels.GetAuth(
				context.Background(),
				provider,
				llm.AuthResolutionOverrides{},
			)
			if err == nil && result != nil && result.Auth.APIKey != "" {
				return result.Auth.APIKey, true
			}
		}
	}
	if authStorage != nil {
		return authStorage.GetAPIKeyWithOptions(
			provider,
			AuthStorageOptions{},
		)
	}
	return "", false
}

func (r *ModelRegistry) GetAPIKeyAndHeaders(model llm.Model) ResolvedRequestAuth {
	return r.GetAPIKeyAndHeadersWithOverrides(
		context.Background(),
		model,
		llm.AuthResolutionOverrides{},
	)
}

// GetAPIKeyAndHeadersWithOverrides resolves one immutable request-auth
// snapshot. Request-scoped key and environment overrides are applied without
// mutating credential storage or the model registry.
func (r *ModelRegistry) GetAPIKeyAndHeadersWithOverrides(
	ctx context.Context,
	model llm.Model,
	overrides llm.AuthResolutionOverrides,
) ResolvedRequestAuth {
	if r == nil {
		return ResolvedRequestAuth{Error: "model registry is required"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ResolvedRequestAuth{Error: err.Error()}
	}
	r.mu.RLock()
	config := r.providerRequestConfigs[model.Provider]
	config.Headers = cloneStringMap(config.Headers)
	modelsJSON, hasModelsJSON := r.modelsJSONProviders[model.Provider]
	extension, hasExtension := r.registeredProviders[model.Provider]
	authStorage := r.authStorage
	dynamicModels := r.dynamicModels
	r.mu.RUnlock()
	apiKey := ""
	var dynamicHeaders map[string]string
	var headerRemovals []string
	baseURL := ""
	requestEnv := cloneResolvedProviderEnv(overrides.Env)
	authSelected := overrides.APIKey != nil || hasExplicitAuthStorage(authStorage, model.Provider)
	if overrides.APIKey != nil {
		apiKey = *overrides.APIKey
	}
	if authSelected {
		if authStorage != nil &&
			!authStorage.HasRuntimeAPIKey(model.Provider) {
			if credential, ok := authStorage.Get(model.Provider); ok {
				requestEnv = mergeResolvedProviderEnv(
					credential.Env,
					overrides.Env,
				)
			}
		}
		if dynamicModels != nil {
			if _, ok := dynamicModels.GetProvider(model.Provider); ok {
				result, err := dynamicModels.GetModelAuth(
					ctx,
					model,
					overrides,
				)
				if err != nil {
					return ResolvedRequestAuth{
						OK:    false,
						Error: err.Error(),
					}
				}
				if result != nil {
					apiKey = result.Auth.APIKey
					dynamicHeaders = result.Auth.Headers
					headerRemovals = append(
						[]string(nil),
						result.Auth.HeaderRemovals...,
					)
					baseURL = result.Auth.BaseURL
					requestEnv = mergeResolvedProviderEnv(
						result.Env,
						overrides.Env,
					)
				}
			} else if authStorage != nil &&
				overrides.APIKey == nil {
				apiKey, _ = authStorage.GetAPIKeyWithOptions(
					model.Provider,
					AuthStorageOptions{
						ExcludeEnvironment: true,
					},
				)
			}
		} else if authStorage != nil &&
			overrides.APIKey == nil {
			apiKey, _ = authStorage.GetAPIKeyWithOptions(
				model.Provider,
				AuthStorageOptions{
					ExcludeEnvironment: true,
				},
			)
		}
	}
	if !authSelected && config.APIKey != "" {
		value, err := ResolveConfigValueOrError(
			config.APIKey,
			fmt.Sprintf(
				`API key for provider "%s"`,
				model.Provider,
			),
			requestEnv,
		)
		if err != nil {
			return ResolvedRequestAuth{OK: false, Error: err.Error()}
		}
		apiKey = value
		authSelected = true
	}
	if !authSelected && dynamicModels != nil {
		if _, ok := dynamicModels.GetProvider(model.Provider); ok {
			result, err := dynamicModels.GetModelAuth(
				ctx,
				model,
				overrides,
			)
			if err != nil {
				return ResolvedRequestAuth{OK: false, Error: err.Error()}
			}
			if result != nil {
				apiKey = result.Auth.APIKey
				dynamicHeaders = result.Auth.Headers
				headerRemovals = append(
					[]string(nil),
					result.Auth.HeaderRemovals...,
				)
				baseURL = result.Auth.BaseURL
				requestEnv = mergeResolvedProviderEnv(
					result.Env,
					overrides.Env,
				)
				authSelected = true
			}
		}
	}
	if !authSelected && authStorage != nil {
		if key, ok := authStorage.GetAPIKeyWithOptions(
			model.Provider,
			AuthStorageOptions{},
		); ok {
			apiKey = key
		}
	}

	headers, err := resolveHeadersOrError(
		config.Headers,
		fmt.Sprintf(`provider "%s"`, model.Provider),
		requestEnv,
	)
	if err != nil {
		return ResolvedRequestAuth{OK: false, Error: err.Error()}
	}
	modelHeaders, err := resolveConfiguredModelHeaders(
		model,
		modelsJSON,
		hasModelsJSON,
		extension,
		hasExtension,
		requestEnv,
	)
	if err != nil {
		return ResolvedRequestAuth{OK: false, Error: err.Error()}
	}
	mergedHeaders := mergeHeadersCaseInsensitive(
		model.Headers,
		dynamicHeaders,
		headers,
		modelHeaders,
	)
	headerRemovals = clearResolvedHeaderRemovals(
		headerRemovals,
		mergedHeaders,
	)
	if config.AuthHeader {
		if apiKey == "" {
			return ResolvedRequestAuth{OK: false, Error: formatNoAPIKeyFoundMessage(model.Provider)}
		}
		mergedHeaders = mergeHeadersCaseInsensitive(
			mergedHeaders,
			map[string]string{"Authorization": "Bearer " + apiKey},
		)
		headerRemovals = clearResolvedHeaderRemovals(
			headerRemovals,
			map[string]string{"Authorization": ""},
		)
	}
	return ResolvedRequestAuth{
		OK:             true,
		APIKey:         apiKey,
		Headers:        emptyMapAsNil(mergedHeaders),
		HeaderRemovals: headerRemovals,
		BaseURL:        baseURL,
		Env:            cloneResolvedProviderEnv(requestEnv),
	}
}

func providerNeedsExplicitAPIKey(provider string) bool {
	return len(providerEnvKeys(provider)) > 0
}

func (r *ModelRegistry) GetProviderAuthStatus(provider string) AuthStatus {
	if r == nil {
		return AuthStatus{Configured: false}
	}
	r.mu.RLock()
	authStorage := r.authStorage
	modelsJSON, hasModelsJSON := r.modelsJSONProviders[provider]
	extension, hasExtension := r.registeredProviders[provider]
	dynamicModels := r.dynamicModels
	r.mu.RUnlock()
	storageStatus := AuthStatus{}
	if authStorage != nil {
		storageStatus = authStorage.GetAuthStatus(provider)
		if storageStatus.Source == "stored" ||
			storageStatus.Source == "runtime" {
			return storageStatus
		}
	}
	if configured := configuredRequestAuthStatus(
		modelsJSON,
		hasModelsJSON,
		extension,
		hasExtension,
	); configured != nil {
		return *configured
	}
	if dynamicModels != nil {
		if _, ok := dynamicModels.GetProvider(provider); ok {
			check, configured, err := dynamicModels.CheckAuth(
				context.Background(),
				provider,
			)
			if err == nil && configured {
				return AuthStatus{
					Configured: true,
					Source:     "environment",
					Label:      check.Source,
				}
			}
		}
	}
	if storageStatus.Configured {
		return storageStatus
	}
	return AuthStatus{Configured: false}
}

func (r *ModelRegistry) hasExplicitAuth(provider string) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	authStorage := r.authStorage
	r.mu.RUnlock()
	return hasExplicitAuthStorage(authStorage, provider)
}

func hasExplicitAuthStorage(authStorage *AuthStorage, provider string) bool {
	return authStorage != nil &&
		(authStorage.HasRuntimeAPIKey(provider) ||
			authStorage.Has(provider))
}

func configuredAPIKeyAuthStatus(value string) AuthStatus {
	if value == "" {
		return AuthStatus{Configured: false}
	}
	if IsCommandConfigValue(value) {
		return AuthStatus{
			Configured: true,
			Source:     "models_json_command",
		}
	}
	if names := ConfigValueEnvVarNames(value); len(names) > 0 {
		if !IsConfigValueConfigured(value, nil) {
			return AuthStatus{Configured: false}
		}
		return AuthStatus{
			Configured: true,
			Source:     "environment",
			Label:      strings.Join(names, ", "),
		}
	}
	return AuthStatus{Configured: true, Source: "models_json_key"}
}

func (r *ModelRegistry) GetProviderDisplayName(provider string) string {
	if r == nil {
		return provider
	}
	r.mu.RLock()
	modelRuntime := r.modelRuntime
	config, hasConfig := r.registeredProviders[provider]
	config = cloneRuntimeProviderConfig(config)
	modelsJSON, hasModelsJSON := r.modelsJSONProviders[provider]
	dynamicModels := r.dynamicModels
	r.mu.RUnlock()
	if modelRuntime != nil {
		if runtimeProvider, ok := modelRuntime.GetProvider(provider); ok {
			return runtimeProvider.Name
		}
	}
	if hasConfig {
		if config.Name != "" {
			return config.Name
		}
	}
	if hasModelsJSON && modelsJSON.Name != "" {
		return modelsJSON.Name
	}
	if dynamicModels != nil {
		if dynamic, ok := dynamicModels.GetProvider(provider); ok &&
			dynamic.Name != "" {
			return dynamic.Name
		}
	}
	if name := builtInProviderDisplayNames[provider]; name != "" {
		return name
	}
	if hasConfig && config.OAuth != nil && config.OAuth.Name != "" {
		return config.OAuth.Name
	}
	for _, oauthProvider := range GetOAuthProviders() {
		if oauthProvider.ID == provider && oauthProvider.Name != "" {
			return oauthProvider.Name
		}
	}
	return provider
}

// GetRegisteredProviderConfig returns a detached compatibility provider
// registration.
func (r *ModelRegistry) GetRegisteredProviderConfig(
	provider string,
) (ProviderConfigInput, bool) {
	if r == nil {
		return ProviderConfigInput{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	config, ok := r.registeredProviders[provider]
	return cloneRuntimeProviderConfig(config), ok
}

// GetRegisteredProviderIDs returns configured and native registrations. Native
// providers are owned by ModelRuntime.
func (r *ModelRegistry) GetRegisteredProviderIDs() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	modelRuntime := r.modelRuntime
	registeredOrder := append([]string(nil), r.registeredOrder...)
	r.mu.RUnlock()
	if modelRuntime != nil {
		return modelRuntime.GetRegisteredProviderIDs()
	}
	return registeredOrder
}

// GetRegisteredNativeProvider projects the native provider owned by the bound
// ModelRuntime.
func (r *ModelRegistry) GetRegisteredNativeProvider(
	provider string,
) (*llm.Provider, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	modelRuntime := r.modelRuntime
	r.mu.RUnlock()
	if modelRuntime == nil {
		return nil, false
	}
	return modelRuntime.GetRegisteredNativeProvider(provider)
}

func (r *ModelRegistry) IsUsingOAuth(model llm.Model) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	authStorage := r.authStorage
	r.mu.RUnlock()
	if authStorage == nil {
		return false
	}
	credential, ok := authStorage.Get(model.Provider)
	return ok && credential.Type == "oauth"
}

func (r *ModelRegistry) RegisterProvider(providerName string, config ProviderConfigInput) error {
	if r == nil {
		return errors.New("model registry is required")
	}
	r.mu.RLock()
	modelRuntime := r.modelRuntime
	r.mu.RUnlock()
	if modelRuntime != nil {
		return modelRuntime.RegisterProvider(providerName, config)
	}
	return r.registerProviderLocal(providerName, config)
}

func (r *ModelRegistry) registerProviderLocal(
	providerName string,
	config ProviderConfigInput,
) error {
	if err := r.validateProviderConfig(providerName, config); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upsertRegisteredProvider(providerName, config)
	effective := r.registeredProviders[providerName]
	r.resetProviderRequestConfig(providerName)
	r.applyProviderConfig(
		providerName,
		effective,
		r.modelRuntime == nil,
	)
	r.applyConfiguredModelOverrides()
	return nil
}

func (r *ModelRegistry) UnregisterProvider(providerName string) {
	if r == nil {
		return
	}
	r.mu.RLock()
	modelRuntime := r.modelRuntime
	r.mu.RUnlock()
	if modelRuntime != nil {
		modelRuntime.UnregisterProvider(providerName)
		return
	}
	r.unregisterProviderLocal(providerName)
}

func (r *ModelRegistry) unregisterProviderLocal(
	providerName string,
) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.registeredProviders[providerName]; !ok {
		return
	}
	delete(r.registeredProviders, providerName)
	r.registeredOrder = removeString(r.registeredOrder, providerName)
	r.restoreAPIProvider(providerName)
	r.restoreOAuthProvider(providerName)
	r.refreshLocked()
}

func (r *ModelRegistry) loadModels(
	ctx context.Context,
	refreshOptions llm.ModelsRefreshOptions,
) {
	result := r.loadCustomModels()
	if result.errorMessage != "" {
		r.loadError = result.errorMessage
	}
	r.configuredModelOverrides = result.modelOverrides
	r.modelsJSONProviders = cloneModelsJSONProviderConfigs(
		result.providers,
	)
	builtIns := r.loadBuiltInModels(result.overrides)
	r.baseModels = mergeCustomModels(builtIns, result.models)
	if err := r.configureDynamicModels(result.radiusProviders); err != nil {
		r.appendLoadError(err.Error())
	}
	if r.dynamicModels != nil {
		_ = r.dynamicModels.Refresh(ctx, refreshOptions)
	}
	r.syncDynamicModels()
	r.applyOAuthModelOverrides()
}

func (r *ModelRegistry) loadCustomModels() customModelsResult {
	result := customModelsResult{
		providers:       map[string]modelsJSONProviderConfig{},
		overrides:       map[string]ProviderConfigInput{},
		modelOverrides:  map[string]map[string]ModelOverride{},
		radiusProviders: []radiusRegistryProvider{},
	}
	if r.modelsJSONPath == "" {
		return result
	}
	content, err := os.ReadFile(r.modelsJSONPath)
	if errors.Is(err, os.ErrNotExist) {
		return result
	}
	if err != nil {
		result.errorMessage = fmt.Sprintf("Failed to load models.json: %s\n\nFile: %s", err, r.modelsJSONPath)
		return result
	}

	var config modelRegistryConfig
	if err := json.Unmarshal([]byte(stripJSONCommentsAndTrailingCommas(string(content))), &config); err != nil {
		result.errorMessage = fmt.Sprintf("Failed to parse models.json: %s\n\nFile: %s", err, r.modelsJSONPath)
		return result
	}
	if config.Providers == nil {
		config.Providers = map[string]modelsJSONProviderConfig{}
	}
	if err := r.validateConfig(config); err != nil {
		result.errorMessage = fmt.Sprintf("Failed to load models.json: %s\n\nFile: %s", err, r.modelsJSONPath)
		return result
	}
	result.providers = cloneModelsJSONProviderConfigs(config.Providers)

	for _, providerName := range sortedModelsJSONProviderNames(
		config.Providers,
	) {
		providerConfig := config.Providers[providerName]
		if providerConfig.OAuth == RadiusProviderID {
			result.radiusProviders = append(
				result.radiusProviders,
				radiusRegistryProvider{
					id:      providerName,
					name:    firstNonEmptyString(providerConfig.Name, providerName),
					gateway: radiusGatewayFromBaseURL(providerConfig.BaseURL),
				},
			)
		}
		if providerConfig.BaseURL != "" || hasCompat(providerConfig.Compat) {
			result.overrides[providerName] = ProviderConfigInput{BaseURL: providerConfig.BaseURL, Compat: providerConfig.Compat}
		}
		r.storeProviderRequestConfig(providerName, ProviderConfigInput{
			APIKey:     providerConfig.APIKey,
			Headers:    providerConfig.Headers,
			AuthHeader: providerConfig.AuthHeader,
		})
		if len(providerConfig.ModelOverrides) > 0 {
			result.modelOverrides[providerName] = providerConfig.ModelOverrides
		}
	}
	result.models = r.parseModels(config)
	return result
}

func (r *ModelRegistry) appendLoadError(message string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	if r.loadError == "" {
		r.loadError = message
		return
	}
	r.loadError += "\n\n" + message
}

func modelRegistryRefreshContext(
	ctx context.Context,
	allowNetwork bool,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !allowNetwork {
		return ctx, func() {}
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

func (r *ModelRegistry) validateConfig(config modelRegistryConfig) error {
	builtIns := builtInProviderSet()
	for _, providerName := range sortedModelsJSONProviderNames(
		config.Providers,
	) {
		providerConfig := config.Providers[providerName]
		isBuiltIn := builtIns[providerName]
		if providerConfig.OAuth != "" &&
			providerConfig.OAuth != RadiusProviderID {
			return fmt.Errorf(
				`Provider %s: unsupported "oauth" value %q.`,
				providerName,
				providerConfig.OAuth,
			)
		}
		if providerConfig.OAuth != "" && providerConfig.BaseURL == "" {
			return fmt.Errorf(
				`Provider %s: "baseUrl" is required when "oauth" is set.`,
				providerName,
			)
		}
		models := providerConfig.Models
		if len(models) == 0 {
			if providerConfig.BaseURL == "" &&
				len(providerConfig.Headers) == 0 &&
				!hasCompat(providerConfig.Compat) &&
				len(providerConfig.ModelOverrides) == 0 &&
				providerConfig.APIKey == "" &&
				providerConfig.OAuth == "" &&
				providerConfig.AuthHeader == nil {
				return fmt.Errorf(`Provider %s: must specify "baseUrl", "headers", "compat", "modelOverrides", or "models".`, providerName)
			}
			continue
		}
		if !isBuiltIn {
			if providerConfig.BaseURL == "" {
				return fmt.Errorf(`Provider %s: "baseUrl" is required when defining custom models.`, providerName)
			}
			if providerConfig.APIKey == "" && providerConfig.OAuth == "" {
				return fmt.Errorf(`Provider %s: "apiKey" is required when defining custom models.`, providerName)
			}
		}
		for _, model := range models {
			if model.ID == "" {
				return fmt.Errorf("Provider %s: model missing \"id\"", providerName)
			}
			if !isBuiltIn &&
				providerConfig.API == "" &&
				model.API == "" &&
				providerConfig.OAuth != RadiusProviderID {
				return fmt.Errorf(`Provider %s, model %s: no "api" specified. Set at provider or model level.`, providerName, model.ID)
			}
			if model.ContextWindow < 0 {
				return fmt.Errorf("Provider %s, model %s: invalid contextWindow", providerName, model.ID)
			}
			if model.MaxTokens < 0 {
				return fmt.Errorf("Provider %s, model %s: invalid maxTokens", providerName, model.ID)
			}
		}
	}
	return nil
}

func (r *ModelRegistry) parseModels(config modelRegistryConfig) []llm.Model {
	builtIns := builtInProviderSet()
	models := []llm.Model{}
	for _, providerName := range sortedModelsJSONProviderNames(
		config.Providers,
	) {
		providerConfig := config.Providers[providerName]
		if len(providerConfig.Models) == 0 {
			continue
		}
		defaults, hasDefaults := builtInDefaults(providerName)
		for _, definition := range providerConfig.Models {
			api := firstNonEmptyString(definition.API, providerConfig.API)
			baseURL := firstNonEmptyString(definition.BaseURL, providerConfig.BaseURL)
			if api == "" && providerConfig.OAuth == RadiusProviderID {
				api = "pi-messages"
			}
			if api == "" && builtIns[providerName] && hasDefaults {
				api = defaults.API
			}
			if baseURL == "" && builtIns[providerName] && hasDefaults {
				baseURL = defaults.BaseURL
			}
			if api == "" || baseURL == "" {
				continue
			}
			models = append(models, buildModelFromDefinition(providerName, api, baseURL, providerConfig.Compat, definition))
		}
	}
	return models
}

func sortedModelsJSONProviderNames(
	providers map[string]modelsJSONProviderConfig,
) []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *ModelRegistry) loadBuiltInModels(overrides map[string]ProviderConfigInput) []llm.Model {
	providers := llm.GetProviders()
	sort.Strings(providers)
	models := make([]llm.Model, 0)
	for _, provider := range providers {
		providerModels := llm.GetModels(provider)
		override := overrides[provider]
		for _, model := range providerModels {
			if override.BaseURL != "" {
				model.BaseURL = override.BaseURL
			}
			model.Compat = mergeCompat(model.Compat, override.Compat)
			models = append(models, model)
		}
	}
	return models
}

func (r *ModelRegistry) applyConfiguredModelOverrides() {
	if r == nil || len(r.configuredModelOverrides) == 0 {
		return
	}
	for index, model := range r.models {
		providerOverrides := r.configuredModelOverrides[model.Provider]
		if override, ok := providerOverrides[model.ID]; ok {
			r.models[index] = applyModelOverride(model, override)
		}
	}
}

func (r *ModelRegistry) validateProviderConfig(providerName string, config ProviderConfigInput) error {
	if config.StreamSimple != nil && config.API == "" {
		return fmt.Errorf(`Provider %s: "api" is required when registering streamSimple.`, providerName)
	}
	if config.Models == nil {
		return nil
	}
	var base *llm.Provider
	if r != nil {
		r.mu.RLock()
		dynamicModels := r.dynamicModels
		r.mu.RUnlock()
		if dynamicModels != nil {
			base, _ = dynamicModels.GetProvider(providerName)
		}
	}
	if base == nil {
		base, _ = llm.NewBuiltinProvider(providerName)
	}
	modelsJSON, hasModelsJSON, _, _ :=
		r.providerCompositionSnapshot(providerName)
	input := modelProviderComposition{
		providerID:    providerName,
		base:          base,
		modelsJSON:    modelsJSON,
		hasModelsJSON: hasModelsJSON,
		extension:     config,
		hasExtension:  true,
	}
	return validateExtensionProvider(input)
}

func (r *ModelRegistry) applyProviderConfig(
	providerName string,
	config ProviderConfigInput,
	legacyGlobals bool,
) {
	if legacyGlobals && config.OAuth != nil {
		r.registerOAuthOverride(providerName, *config.OAuth)
	}
	if legacyGlobals && config.StreamSimple != nil {
		r.registerAPIOverride(providerName, config.API, config.StreamSimple)
	}
	r.storeProviderRequestConfig(providerName, config)
	if config.Models != nil {
		r.models = removeModelsForProvider(r.models, providerName)
		for _, definition := range config.Models {
			api := firstNonEmptyString(definition.API, config.API)
			baseURL := firstNonEmptyString(definition.BaseURL, config.BaseURL)
			r.models = append(r.models, buildModelFromDefinition(providerName, api, baseURL, config.Compat, definition))
		}
		return
	}
	if config.BaseURL != "" || hasCompat(config.Compat) {
		for i := range r.models {
			if r.models[i].Provider != providerName {
				continue
			}
			if config.BaseURL != "" {
				r.models[i].BaseURL = config.BaseURL
			}
			r.models[i].Compat = mergeCompat(r.models[i].Compat, config.Compat)
		}
	}
}

func (r *ModelRegistry) upsertRegisteredProvider(providerName string, config ProviderConfigInput) {
	config = cloneRuntimeProviderConfig(config)
	existing, ok := r.registeredProviders[providerName]
	if !ok {
		r.registeredProviders[providerName] = config
		r.registeredOrder = append(r.registeredOrder, providerName)
		return
	}
	r.registeredProviders[providerName] = mergeProviderConfig(existing, config)
}

func (r *ModelRegistry) registerAPIOverride(providerName, api string, streamSimple func(llm.Model, llm.Context, llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error)) {
	if _, ok := r.apiProviderRestores[providerName]; !ok {
		r.apiProviderRestores[providerName] = apiProviderRestore{api: api, previous: llm.GetAPIProvider(api)}
	}
	llm.RegisterAPIProvider(api, llm.APIProviderFuncs{StreamSimpleFunc: streamSimple})
}

func (r *ModelRegistry) restoreAPIProvider(providerName string) {
	restore, ok := r.apiProviderRestores[providerName]
	if !ok {
		return
	}
	if restore.previous != nil {
		llm.RegisterAPIProvider(restore.api, restore.previous)
	} else {
		llm.UnregisterAPIProvider(restore.api)
	}
	delete(r.apiProviderRestores, providerName)
}

func (r *ModelRegistry) registerOAuthOverride(providerName string, provider OAuthProvider) {
	if _, ok := r.oauthProviderRestores[providerName]; !ok {
		previous, had := GetOAuthProvider(providerName)
		r.oauthProviderRestores[providerName] = oauthProviderRestore{previous: previous, had: had}
	}
	provider.ID = providerName
	RegisterOAuthProvider(provider)
}

func (r *ModelRegistry) restoreOAuthProvider(providerName string) {
	restore, ok := r.oauthProviderRestores[providerName]
	if !ok {
		return
	}
	if restore.had {
		RegisterOAuthProvider(restore.previous)
	} else {
		UnregisterOAuthProvider(providerName)
	}
	delete(r.oauthProviderRestores, providerName)
}

func (r *ModelRegistry) releaseLegacyProviderOverrides() {
	for index := len(r.registeredOrder) - 1; index >= 0; index-- {
		providerName := r.registeredOrder[index]
		r.restoreAPIProvider(providerName)
		r.restoreOAuthProvider(providerName)
	}
}

func (r *ModelRegistry) storeProviderRequestConfig(providerName string, config ProviderConfigInput) {
	if config.APIKey == "" && len(config.Headers) == 0 && config.AuthHeader == nil {
		return
	}
	stored := r.providerRequestConfigs[providerName]
	if config.APIKey != "" {
		stored.APIKey = config.APIKey
	}
	stored.Headers = mergeHeadersCaseInsensitive(
		stored.Headers,
		config.Headers,
	)
	if config.AuthHeader != nil {
		stored.AuthHeader = *config.AuthHeader
	}
	r.providerRequestConfigs[providerName] = stored
}

func (r *ModelRegistry) resetProviderRequestConfig(providerName string) {
	delete(r.providerRequestConfigs, providerName)
	config, ok := r.modelsJSONProviders[providerName]
	if !ok {
		return
	}
	r.storeProviderRequestConfig(providerName, ProviderConfigInput{
		APIKey:     config.APIKey,
		Headers:    cloneOptionalStringMap(config.Headers),
		AuthHeader: config.AuthHeader,
	})
}

func buildModelFromDefinition(providerName, api, baseURL string, providerCompat llm.ModelCompat, definition ProviderModelDefinition) llm.Model {
	name := definition.Name
	if name == "" {
		name = definition.ID
	}
	input := append([]string{}, definition.Input...)
	if len(input) == 0 {
		input = []string{"text"}
	}
	contextWindow := definition.ContextWindow
	if contextWindow == 0 {
		contextWindow = 128000
	}
	maxTokens := definition.MaxTokens
	if maxTokens == 0 {
		maxTokens = 16384
	}
	return llm.Model{
		ID:               definition.ID,
		Name:             name,
		API:              api,
		Provider:         providerName,
		BaseURL:          baseURL,
		Reasoning:        definition.Reasoning,
		ThinkingLevelMap: cloneThinkingLevelMap(definition.ThinkingLevelMap),
		Input:            input,
		Cost:             definition.Cost,
		ContextWindow:    contextWindow,
		MaxTokens:        maxTokens,
		Compat:           mergeCompat(providerCompat, definition.Compat),
	}
}

func applyModelOverride(model llm.Model, override ModelOverride) llm.Model {
	if override.Name != "" {
		model.Name = override.Name
	}
	if override.Reasoning != nil {
		model.Reasoning = *override.Reasoning
	}
	if override.ThinkingLevelMap != nil {
		model.ThinkingLevelMap = mergeThinkingLevelMap(model.ThinkingLevelMap, override.ThinkingLevelMap)
	}
	if override.Input != nil {
		model.Input = append([]string{}, override.Input...)
	}
	if override.ContextWindow != nil {
		model.ContextWindow = *override.ContextWindow
	}
	if override.MaxTokens != nil {
		model.MaxTokens = *override.MaxTokens
	}
	if override.Cost != nil {
		if override.Cost.Input != nil {
			model.Cost.Input = *override.Cost.Input
		}
		if override.Cost.Output != nil {
			model.Cost.Output = *override.Cost.Output
		}
		if override.Cost.CacheRead != nil {
			model.Cost.CacheRead = *override.Cost.CacheRead
		}
		if override.Cost.CacheWrite != nil {
			model.Cost.CacheWrite = *override.Cost.CacheWrite
		}
		if override.Cost.Tiers != nil {
			model.Cost.Tiers = append(
				[]llm.ModelCostTier(nil),
				(*override.Cost.Tiers)...,
			)
		}
	}
	model.Compat = mergeCompat(model.Compat, override.Compat)
	return model
}

func mergeCustomModels(builtIns, customModels []llm.Model) []llm.Model {
	merged := append([]llm.Model{}, builtIns...)
	for _, custom := range customModels {
		replaced := false
		for i, model := range merged {
			if model.Provider == custom.Provider && model.ID == custom.ID {
				merged[i] = custom
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, custom)
		}
	}
	return merged
}

func mergeProviderConfig(existing, incoming ProviderConfigInput) ProviderConfigInput {
	existing = cloneRuntimeProviderConfig(existing)
	incoming = cloneRuntimeProviderConfig(incoming)
	if incoming.Name != "" {
		existing.Name = incoming.Name
	}
	if incoming.BaseURL != "" {
		existing.BaseURL = incoming.BaseURL
	}
	if incoming.APIKey != "" {
		existing.APIKey = incoming.APIKey
	}
	if incoming.API != "" {
		existing.API = incoming.API
	}
	if incoming.Headers != nil {
		existing.Headers = cloneOptionalStringMap(incoming.Headers)
	}
	if incoming.AuthHeader != nil {
		authHeader := *incoming.AuthHeader
		existing.AuthHeader = &authHeader
	}
	if incoming.OAuth != nil {
		oauth := *incoming.OAuth
		existing.OAuth = &oauth
	}
	if incoming.StreamSimple != nil {
		existing.StreamSimple = incoming.StreamSimple
	}
	if incoming.RefreshModels != nil {
		existing.RefreshModels = incoming.RefreshModels
	}
	if hasCompat(incoming.Compat) {
		existing.Compat = mergeCompat(existing.Compat, incoming.Compat)
	}
	if incoming.Models != nil {
		existing.Models = cloneProviderModelDefinitions(incoming.Models)
	}
	if len(incoming.ModelOverrides) > 0 {
		if existing.ModelOverrides == nil {
			existing.ModelOverrides = map[string]ModelOverride{}
		}
		for modelID, override := range incoming.ModelOverrides {
			existing.ModelOverrides[modelID] = override
		}
	}
	return existing
}

func mergeCompat(base, override llm.ModelCompat) llm.ModelCompat {
	if !hasCompat(override) {
		return base
	}
	merged := base
	if override.SupportsStore != nil {
		merged.SupportsStore = override.SupportsStore
	}
	if override.SupportsDeveloperRole != nil {
		merged.SupportsDeveloperRole = override.SupportsDeveloperRole
	}
	if override.SupportsReasoningEffort != nil {
		merged.SupportsReasoningEffort = override.SupportsReasoningEffort
	}
	if override.SupportsUsageInStreaming != nil {
		merged.SupportsUsageInStreaming = override.SupportsUsageInStreaming
	}
	if override.SupportsStrictMode != nil {
		merged.SupportsStrictMode = override.SupportsStrictMode
	}
	if override.SupportsLongCacheRetention != nil {
		merged.SupportsLongCacheRetention = override.SupportsLongCacheRetention
	}
	if override.SupportsEagerToolInputStreaming != nil {
		merged.SupportsEagerToolInputStreaming = override.SupportsEagerToolInputStreaming
	}
	if override.SupportsCacheControlOnTools != nil {
		merged.SupportsCacheControlOnTools = override.SupportsCacheControlOnTools
	}
	if override.SendSessionAffinityHeaders != nil {
		merged.SendSessionAffinityHeaders = override.SendSessionAffinityHeaders
	}
	if override.SendSessionIDHeader != nil {
		merged.SendSessionIDHeader = override.SendSessionIDHeader
	}
	if override.RequiresToolResultName != nil {
		merged.RequiresToolResultName = override.RequiresToolResultName
	}
	if override.RequiresAssistantAfterToolResult != nil {
		merged.RequiresAssistantAfterToolResult = override.RequiresAssistantAfterToolResult
	}
	if override.RequiresThinkingAsText != nil {
		merged.RequiresThinkingAsText = override.RequiresThinkingAsText
	}
	if override.RequiresReasoningContentOnAssistantTurns != nil {
		merged.RequiresReasoningContentOnAssistantTurns = override.RequiresReasoningContentOnAssistantTurns
	}
	if override.RequiresReasoningContentOnAssistantEvents != nil {
		merged.RequiresReasoningContentOnAssistantEvents = override.RequiresReasoningContentOnAssistantEvents
	}
	if override.ZAIToolStream != nil {
		merged.ZAIToolStream = override.ZAIToolStream
	}
	if len(override.OpenRouterRouting) > 0 {
		merged.OpenRouterRouting = mergeAnyMaps(merged.OpenRouterRouting, override.OpenRouterRouting)
	}
	if len(override.VercelGatewayRouting) > 0 {
		merged.VercelGatewayRouting = mergeAnyMaps(merged.VercelGatewayRouting, override.VercelGatewayRouting)
	}
	if override.MaxTokensField != "" {
		merged.MaxTokensField = override.MaxTokensField
	}
	if override.ThinkingFormat != "" {
		merged.ThinkingFormat = override.ThinkingFormat
	}
	if override.CacheControlFormat != "" {
		merged.CacheControlFormat = override.CacheControlFormat
	}
	return merged
}

func hasCompat(compat llm.ModelCompat) bool {
	return compat.SupportsStore != nil ||
		compat.SupportsDeveloperRole != nil ||
		compat.SupportsReasoningEffort != nil ||
		compat.SupportsUsageInStreaming != nil ||
		compat.SupportsStrictMode != nil ||
		compat.SupportsLongCacheRetention != nil ||
		compat.SupportsEagerToolInputStreaming != nil ||
		compat.SupportsCacheControlOnTools != nil ||
		compat.SendSessionAffinityHeaders != nil ||
		compat.SendSessionIDHeader != nil ||
		compat.RequiresToolResultName != nil ||
		compat.RequiresAssistantAfterToolResult != nil ||
		compat.RequiresThinkingAsText != nil ||
		compat.RequiresReasoningContentOnAssistantTurns != nil ||
		compat.RequiresReasoningContentOnAssistantEvents != nil ||
		compat.ZAIToolStream != nil ||
		len(compat.OpenRouterRouting) > 0 ||
		len(compat.VercelGatewayRouting) > 0 ||
		compat.MaxTokensField != "" ||
		compat.ThinkingFormat != "" ||
		compat.CacheControlFormat != ""
}

func builtInDefaults(provider string) (llm.Model, bool) {
	models := llm.GetModels(provider)
	if len(models) == 0 {
		return llm.Model{}, false
	}
	defaults := models[0]
	for _, model := range models {
		if model.BaseURL != "" {
			defaults.BaseURL = model.BaseURL
			defaults.API = model.API
			break
		}
	}
	return defaults, true
}

func builtInProviderSet() map[string]bool {
	set := map[string]bool{}
	for _, provider := range llm.GetBuiltinProviderIDs() {
		set[provider] = true
	}
	return set
}

func removeModelsForProvider(models []llm.Model, provider string) []llm.Model {
	filtered := models[:0]
	for _, model := range models {
		if model.Provider != provider {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func resolveConfigValueOrError(config, description string) (string, error) {
	return ResolveConfigValueOrError(config, description, nil)
}

func resolveHeadersOrError(
	headers map[string]string,
	description string,
	env llm.ProviderEnv,
) (map[string]string, error) {
	return ResolveConfigHeadersOrError(headers, description, env)
}

func cloneResolvedProviderEnv(env llm.ProviderEnv) llm.ProviderEnv {
	if env == nil {
		return nil
	}
	cloned := make(llm.ProviderEnv, len(env))
	for name, value := range env {
		cloned[name] = value
	}
	return cloned
}

func mergeResolvedProviderEnv(
	base llm.ProviderEnv,
	override llm.ProviderEnv,
) llm.ProviderEnv {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := cloneResolvedProviderEnv(base)
	if merged == nil {
		merged = llm.ProviderEnv{}
	}
	for name, value := range override {
		merged[name] = value
	}
	return merged
}

func stripJSONCommentsAndTrailingCommas(input string) string {
	return jsonutil.StripCommentsAndTrailingCommas(input)
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneOptionalStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func mergeStringMaps(maps ...map[string]string) map[string]string {
	var merged map[string]string
	for _, values := range maps {
		for key, value := range values {
			if merged == nil {
				merged = map[string]string{}
			}
			merged[key] = value
		}
	}
	return merged
}

func mergeHeadersCaseInsensitive(
	maps ...map[string]string,
) map[string]string {
	var merged map[string]string
	for _, values := range maps {
		for name, value := range values {
			if merged == nil {
				merged = map[string]string{}
			}
			for existing := range merged {
				if strings.EqualFold(existing, name) {
					delete(merged, existing)
				}
			}
			merged[name] = value
		}
	}
	return emptyMapAsNil(merged)
}

func clearResolvedHeaderRemovals(
	removals []string,
	headers map[string]string,
) []string {
	if len(removals) == 0 || len(headers) == 0 {
		return append([]string(nil), removals...)
	}
	filtered := make([]string, 0, len(removals))
	for _, removal := range removals {
		overridden := false
		for name := range headers {
			if strings.EqualFold(name, removal) {
				overridden = true
				break
			}
		}
		if !overridden {
			filtered = append(filtered, removal)
		}
	}
	return filtered
}

func emptyMapAsNil(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	return values
}

func mergeAnyMaps(base, override map[string]any) map[string]any {
	merged := map[string]any{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func cloneThinkingLevelMap(values map[string]*string) map[string]*string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]*string, len(values))
	for key, value := range values {
		if value == nil {
			cloned[key] = nil
		} else {
			copyValue := *value
			cloned[key] = &copyValue
		}
	}
	return cloned
}

func mergeThinkingLevelMap(base, override map[string]*string) map[string]*string {
	merged := cloneThinkingLevelMap(base)
	if merged == nil {
		merged = map[string]*string{}
	}
	for key, value := range override {
		if value == nil {
			merged[key] = nil
		} else {
			copyValue := *value
			merged[key] = &copyValue
		}
	}
	return merged
}

func removeString(values []string, target string) []string {
	filtered := values[:0]
	for _, value := range values {
		if value != target {
			filtered = append(filtered, value)
		}
	}
	return filtered
}
