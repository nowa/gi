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
	"time"

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
	AuthHeader     bool
	OAuth          *OAuthProvider                                                                                  `json:"-"`
	StreamSimple   func(llm.Model, llm.Context, llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) `json:"-"`
	Compat         llm.ModelCompat
	Models         []ProviderModelDefinition
	ModelOverrides map[string]ModelOverride
}

type ResolvedRequestAuth struct {
	OK      bool
	APIKey  string
	Headers map[string]string
	Env     llm.ProviderEnv
	Error   string
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
// Callers serialize registry mutation with reads; the shared llm.Models,
// provider catalog state, credential storage, and ModelsStore boundaries it
// composes are independently safe for concurrent use.
type ModelRegistry struct {
	authStorage              *AuthStorage
	modelsJSONPath           string
	modelsStore              llm.ModelsStore
	radiusClient             llm.HTTPDoer
	dynamicModels            *llm.Models
	baseModels               []llm.Model
	models                   []llm.Model
	configuredModelOverrides map[string]map[string]ModelOverride
	providerRequestConfigs   map[string]ProviderRequestConfig
	modelRequestHeaders      map[string]map[string]string
	registeredProviders      map[string]ProviderConfigInput
	registeredOrder          []string
	apiProviderRestores      map[string]apiProviderRestore
	oauthProviderRestores    map[string]oauthProviderRestore
	loadError                string
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
	registry := &ModelRegistry{
		authStorage:            options.AuthStorage,
		modelsJSONPath:         options.ModelsJSONPath,
		modelsStore:            modelsStore,
		radiusClient:           options.RadiusClient,
		providerRequestConfigs: map[string]ProviderRequestConfig{},
		modelRequestHeaders:    map[string]map[string]string{},
		registeredProviders:    map[string]ProviderConfigInput{},
		apiProviderRestores:    map[string]apiProviderRestore{},
		oauthProviderRestores:  map[string]oauthProviderRestore{},
	}
	refreshCtx, cancel := modelRegistryRefreshContext(
		ctx,
		options.AllowModelNetwork,
		options.ModelRefreshTimeout,
	)
	defer cancel()
	registry.loadModels(refreshCtx, llm.ModelsRefreshOptions{
		Offline: !options.AllowModelNetwork,
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
	r.providerRequestConfigs = map[string]ProviderRequestConfig{}
	r.modelRequestHeaders = map[string]map[string]string{}
	r.loadError = ""
	r.loadModels(context.Background(), llm.ModelsRefreshOptions{Offline: true})
	for _, providerName := range r.registeredOrder {
		config, ok := r.registeredProviders[providerName]
		if ok {
			r.applyProviderConfig(providerName, config)
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
	if r == nil || r.dynamicModels == nil {
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
	for _, providerName := range r.registeredOrder {
		if config, ok := r.registeredProviders[providerName]; ok {
			r.applyProviderConfig(providerName, config)
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
	return r.loadError
}

func (r *ModelRegistry) GetAll() []llm.Model {
	return append([]llm.Model{}, r.models...)
}

func (r *ModelRegistry) GetAvailable() []llm.Model {
	available := make([]llm.Model, 0, len(r.models))
	for _, model := range r.models {
		if r.HasConfiguredAuth(model) {
			available = append(available, model)
		}
	}
	return available
}

func (r *ModelRegistry) Find(provider, modelID string) (llm.Model, bool) {
	for _, model := range r.models {
		if model.Provider == provider && model.ID == modelID {
			return model, true
		}
	}
	return llm.Model{}, false
}

func (r *ModelRegistry) HasConfiguredAuth(model llm.Model) bool {
	if r.hasExplicitAuth(model.Provider) {
		return true
	}
	if config, ok := r.providerRequestConfigs[model.Provider]; ok &&
		config.APIKey != "" {
		return configuredAPIKeyAuthStatus(config.APIKey).Configured
	}
	if r.dynamicModels != nil {
		if _, ok := r.dynamicModels.GetProvider(model.Provider); ok {
			_, configured, err := r.dynamicModels.CheckAuth(
				context.Background(),
				model.Provider,
			)
			if err == nil && configured {
				return true
			}
		}
	}
	if r.authStorage != nil && r.authStorage.HasAuth(model.Provider) {
		return true
	}
	return false
}

func (r *ModelRegistry) GetAPIKeyForProvider(provider string) (string, bool) {
	if r.hasExplicitAuth(provider) {
		if r.dynamicModels != nil {
			if _, ok := r.dynamicModels.GetProvider(provider); ok {
				result, err := r.dynamicModels.GetAuth(
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
		return r.authStorage.GetAPIKeyWithOptions(
			provider,
			AuthStorageOptions{
				ExcludeEnvironment: true,
			},
		)
	}
	if config, ok := r.providerRequestConfigs[provider]; ok &&
		config.APIKey != "" {
		return ResolveConfigValueUncached(config.APIKey)
	}
	if r.dynamicModels != nil {
		if _, ok := r.dynamicModels.GetProvider(provider); ok {
			result, err := r.dynamicModels.GetAuth(
				context.Background(),
				provider,
				llm.AuthResolutionOverrides{},
			)
			if err == nil && result != nil && result.Auth.APIKey != "" {
				return result.Auth.APIKey, true
			}
		}
	}
	if r.authStorage != nil {
		return r.authStorage.GetAPIKeyWithOptions(
			provider,
			AuthStorageOptions{},
		)
	}
	return "", false
}

func (r *ModelRegistry) GetAPIKeyAndHeaders(model llm.Model) ResolvedRequestAuth {
	config := r.providerRequestConfigs[model.Provider]
	apiKey := ""
	var dynamicHeaders map[string]string
	var requestEnv llm.ProviderEnv
	authSelected := r.hasExplicitAuth(model.Provider)
	if authSelected {
		if r.authStorage != nil &&
			!r.authStorage.HasRuntimeAPIKey(model.Provider) {
			if credential, ok := r.authStorage.Get(model.Provider); ok {
				requestEnv = credential.Env
			}
		}
		if r.dynamicModels != nil {
			if _, ok := r.dynamicModels.GetProvider(model.Provider); ok {
				result, err := r.dynamicModels.GetModelAuth(
					context.Background(),
					model,
					llm.AuthResolutionOverrides{},
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
					requestEnv = result.Env
				}
			} else if r.authStorage != nil {
				apiKey, _ = r.authStorage.GetAPIKeyWithOptions(
					model.Provider,
					AuthStorageOptions{
						ExcludeEnvironment: true,
					},
				)
			}
		} else if r.authStorage != nil {
			apiKey, _ = r.authStorage.GetAPIKeyWithOptions(
				model.Provider,
				AuthStorageOptions{
					ExcludeEnvironment: true,
				},
			)
		}
	}
	if !authSelected && config.APIKey != "" {
		value, err := resolveConfigValueOrError(
			config.APIKey,
			fmt.Sprintf(
				`API key for provider "%s"`,
				model.Provider,
			),
		)
		if err != nil {
			return ResolvedRequestAuth{OK: false, Error: err.Error()}
		}
		apiKey = value
		authSelected = true
	}
	if !authSelected && r.dynamicModels != nil {
		if _, ok := r.dynamicModels.GetProvider(model.Provider); ok {
			result, err := r.dynamicModels.GetModelAuth(
				context.Background(),
				model,
				llm.AuthResolutionOverrides{},
			)
			if err != nil {
				return ResolvedRequestAuth{OK: false, Error: err.Error()}
			}
			if result != nil {
				apiKey = result.Auth.APIKey
				dynamicHeaders = result.Auth.Headers
				requestEnv = result.Env
				authSelected = true
			}
		}
	}
	if !authSelected && r.authStorage != nil {
		if key, ok := r.authStorage.GetAPIKeyWithOptions(
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
	modelHeaderConfig := r.modelRequestHeaders[r.modelRequestKey(
		model.Provider,
		model.ID,
	)]
	modelHeaders, err := resolveHeadersOrError(
		modelHeaderConfig,
		fmt.Sprintf(`model "%s/%s"`, model.Provider, model.ID),
		requestEnv,
	)
	if err != nil {
		return ResolvedRequestAuth{OK: false, Error: err.Error()}
	}
	mergedHeaders := mergeStringMaps(
		model.Headers,
		dynamicHeaders,
		headers,
		modelHeaders,
	)
	if config.AuthHeader {
		if apiKey == "" {
			return ResolvedRequestAuth{OK: false, Error: formatNoAPIKeyFoundMessage(model.Provider)}
		}
		if mergedHeaders == nil {
			mergedHeaders = map[string]string{}
		}
		mergedHeaders["Authorization"] = "Bearer " + apiKey
	}
	return ResolvedRequestAuth{
		OK:      true,
		APIKey:  apiKey,
		Headers: emptyMapAsNil(mergedHeaders),
		Env:     cloneResolvedProviderEnv(requestEnv),
	}
}

func providerNeedsExplicitAPIKey(provider string) bool {
	return len(providerEnvKeys(provider)) > 0
}

func (r *ModelRegistry) GetProviderAuthStatus(provider string) AuthStatus {
	storageStatus := AuthStatus{}
	if r.authStorage != nil {
		storageStatus = r.authStorage.GetAuthStatus(provider)
		if storageStatus.Source == "stored" ||
			storageStatus.Source == "runtime" {
			return storageStatus
		}
	}
	if config, ok := r.providerRequestConfigs[provider]; ok &&
		config.APIKey != "" {
		return configuredAPIKeyAuthStatus(config.APIKey)
	}
	if r.dynamicModels != nil {
		if _, ok := r.dynamicModels.GetProvider(provider); ok {
			check, configured, err := r.dynamicModels.CheckAuth(
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
	return r != nil &&
		r.authStorage != nil &&
		(r.authStorage.HasRuntimeAPIKey(provider) ||
			r.authStorage.Has(provider))
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
	if os.Getenv(value) != "" {
		return AuthStatus{
			Configured: true,
			Source:     "environment",
			Label:      value,
		}
	}
	return AuthStatus{Configured: true, Source: "models_json_key"}
}

func (r *ModelRegistry) GetProviderDisplayName(provider string) string {
	if config, ok := r.registeredProviders[provider]; ok {
		if config.Name != "" {
			return config.Name
		}
		if config.OAuth != nil && config.OAuth.Name != "" {
			return config.OAuth.Name
		}
	}
	if r.dynamicModels != nil {
		if dynamic, ok := r.dynamicModels.GetProvider(provider); ok &&
			dynamic.Name != "" {
			return dynamic.Name
		}
	}
	for _, oauthProvider := range GetOAuthProviders() {
		if oauthProvider.ID == provider && oauthProvider.Name != "" {
			return oauthProvider.Name
		}
	}
	if name := builtInProviderDisplayNames[provider]; name != "" {
		return name
	}
	return provider
}

func (r *ModelRegistry) IsUsingOAuth(model llm.Model) bool {
	if r.authStorage == nil {
		return false
	}
	credential, ok := r.authStorage.Get(model.Provider)
	return ok && credential.Type == "oauth"
}

func (r *ModelRegistry) RegisterProvider(providerName string, config ProviderConfigInput) error {
	if err := r.validateProviderConfig(providerName, config); err != nil {
		return err
	}
	r.applyProviderConfig(providerName, config)
	r.applyConfiguredModelOverrides()
	r.upsertRegisteredProvider(providerName, config)
	return nil
}

func (r *ModelRegistry) UnregisterProvider(providerName string) {
	if _, ok := r.registeredProviders[providerName]; !ok {
		return
	}
	delete(r.registeredProviders, providerName)
	r.registeredOrder = removeString(r.registeredOrder, providerName)
	r.restoreAPIProvider(providerName)
	r.restoreOAuthProvider(providerName)
	r.Refresh()
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
			APIKey:  providerConfig.APIKey,
			Headers: providerConfig.Headers,
			AuthHeader: providerConfig.AuthHeader != nil &&
				*providerConfig.AuthHeader,
		})
		if len(providerConfig.ModelOverrides) > 0 {
			result.modelOverrides[providerName] = providerConfig.ModelOverrides
			for modelID, override := range providerConfig.ModelOverrides {
				r.storeModelHeaders(providerName, modelID, override.Headers)
			}
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
			r.storeModelHeaders(providerName, definition.ID, definition.Headers)
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
	if len(config.Models) == 0 {
		return nil
	}
	if config.BaseURL == "" {
		return fmt.Errorf(`Provider %s: "baseUrl" is required when defining models.`, providerName)
	}
	if config.APIKey == "" && config.OAuth == nil {
		return fmt.Errorf(`Provider %s: "apiKey" or "oauth" is required when defining models.`, providerName)
	}
	for _, model := range config.Models {
		if model.API == "" && config.API == "" {
			return fmt.Errorf(`Provider %s, model %s: no "api" specified.`, providerName, model.ID)
		}
	}
	return nil
}

func (r *ModelRegistry) applyProviderConfig(providerName string, config ProviderConfigInput) {
	if config.OAuth != nil {
		r.registerOAuthOverride(providerName, *config.OAuth)
	}
	if config.StreamSimple != nil {
		r.registerAPIOverride(providerName, config.API, config.StreamSimple)
	}
	r.storeProviderRequestConfig(providerName, config)
	if len(config.Models) > 0 {
		r.models = removeModelsForProvider(r.models, providerName)
		for _, definition := range config.Models {
			api := firstNonEmptyString(definition.API, config.API)
			baseURL := firstNonEmptyString(definition.BaseURL, config.BaseURL)
			r.storeModelHeaders(providerName, definition.ID, definition.Headers)
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

func (r *ModelRegistry) storeProviderRequestConfig(providerName string, config ProviderConfigInput) {
	if config.APIKey == "" && len(config.Headers) == 0 && !config.AuthHeader {
		return
	}
	r.providerRequestConfigs[providerName] = ProviderRequestConfig{
		APIKey:     config.APIKey,
		Headers:    cloneStringMap(config.Headers),
		AuthHeader: config.AuthHeader,
	}
}

func (r *ModelRegistry) storeModelHeaders(providerName, modelID string, headers map[string]string) {
	key := r.modelRequestKey(providerName, modelID)
	if len(headers) == 0 {
		delete(r.modelRequestHeaders, key)
		return
	}
	r.modelRequestHeaders[key] = cloneStringMap(headers)
}

func (r *ModelRegistry) modelRequestKey(providerName, modelID string) string {
	return providerName + ":" + modelID
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
		existing.Headers = incoming.Headers
	}
	if incoming.AuthHeader {
		existing.AuthHeader = true
	}
	if incoming.OAuth != nil {
		existing.OAuth = incoming.OAuth
	}
	if incoming.StreamSimple != nil {
		existing.StreamSimple = incoming.StreamSimple
	}
	if hasCompat(incoming.Compat) {
		existing.Compat = mergeCompat(existing.Compat, incoming.Compat)
	}
	if len(incoming.Models) > 0 {
		existing.Models = incoming.Models
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

func stripJSONCommentsAndTrailingCommas(input string) string {
	var out strings.Builder
	inString := false
	escaped := false
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if inString {
			out.WriteByte(ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			out.WriteByte(ch)
			continue
		}
		if ch == '/' && i+1 < len(input) && input[i+1] == '/' {
			for i < len(input) && input[i] != '\n' {
				i++
			}
			if i < len(input) {
				out.WriteByte('\n')
			}
			continue
		}
		out.WriteByte(ch)
	}
	return removeTrailingCommas(out.String())
}

func removeTrailingCommas(input string) string {
	bytes := []byte(input)
	out := make([]byte, 0, len(bytes))
	for i := 0; i < len(bytes); i++ {
		if bytes[i] == '}' || bytes[i] == ']' {
			j := len(out) - 1
			for j >= 0 && (out[j] == ' ' || out[j] == '\t' || out[j] == '\r' || out[j] == '\n') {
				j--
			}
			if j >= 0 && out[j] == ',' {
				out = append(out[:j], out[j+1:]...)
			}
		}
		out = append(out, bytes[i])
	}
	return string(out)
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
