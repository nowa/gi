package gicodingagent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type modelProviderComposition struct {
	providerID    string
	base          *llm.Provider
	modelsJSON    modelsJSONProviderConfig
	hasModelsJSON bool
	extension     ProviderConfigInput
	hasExtension  bool
}

type composedModelProviderState struct {
	composition modelProviderComposition

	mu                       sync.RWMutex
	modifyMu                 sync.Mutex
	refreshedExtensionModels []ProviderModelDefinition
	hasRefreshedModels       bool
	extensionOAuthCredential *llm.Credential
}

func composeModelProvider(
	input modelProviderComposition,
) (*llm.Provider, error) {
	if strings.TrimSpace(input.providerID) == "" {
		return nil, errors.New("provider ID is required")
	}
	input.modelsJSON = cloneModelsJSONProviderConfig(input.modelsJSON)
	input.extension = cloneRuntimeProviderConfig(input.extension)
	state := &composedModelProviderState{composition: input}
	_, err := state.models()
	if err != nil {
		return nil, err
	}
	auth, err := composeRuntimeProviderAuth(input)
	if err != nil {
		return nil, err
	}
	if auth.APIKey == nil && auth.OAuth == nil {
		return nil, fmt.Errorf(
			"provider %s has no authentication method configured",
			input.providerID,
		)
	}

	baseName := ""
	baseURL := ""
	headers := map[string]string(nil)
	if input.base != nil {
		baseName = input.base.Name
		baseURL = input.base.BaseURL
		headers = cloneStringMap(input.base.Headers)
	}
	name := firstNonEmptyString(
		input.extension.Name,
		input.modelsJSON.Name,
		baseName,
		oauthProviderName(input.extension.OAuth),
		input.providerID,
	)
	baseURL = firstNonEmptyString(
		input.extension.BaseURL,
		input.modelsJSON.BaseURL,
		baseURL,
	)

	provider := &llm.Provider{
		ID:          input.providerID,
		Name:        name,
		BaseURL:     baseURL,
		Headers:     headers,
		Auth:        auth,
		ModelSource: state.models,
	}
	if input.base != nil {
		provider.FilterModelsFunc = input.base.FilterModelsFunc
	}
	if state.refreshable() {
		provider.RefreshModelsFunc = state.refresh
	}
	provider.StreamFunc, provider.StreamSimpleFunc =
		composedProviderStreams(input)
	return provider, nil
}

func validateExtensionProvider(
	input modelProviderComposition,
) error {
	_, err := composeModelProvider(input)
	return err
}

func (s *composedModelProviderState) models() ([]llm.Model, error) {
	baseModels, err := providerModelSnapshot(s.composition.base)
	if err != nil {
		return nil, err
	}
	models, err := applyModelsJSON(
		s.composition.providerID,
		baseModels,
		s.composition.modelsJSON,
		s.composition.hasModelsJSON,
	)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	extension := cloneRuntimeProviderConfig(s.composition.extension)
	if s.hasRefreshedModels {
		extension.Models = presentProviderModelDefinitions(
			s.refreshedExtensionModels,
		)
	}
	credential := cloneCredentialPointerForComposer(
		s.extensionOAuthCredential,
	)
	s.mu.RUnlock()

	models, err = applyExtension(
		s.composition.providerID,
		models,
		extension,
		s.composition.hasExtension,
	)
	if err != nil {
		return nil, err
	}
	if credential != nil &&
		extension.OAuth != nil &&
		extension.OAuth.ModifyModels != nil {
		models = s.modifyModels(
			extension.OAuth.ModifyModels,
			models,
			*credential,
		)
	}
	if s.composition.hasModelsJSON {
		models = applyRuntimeModelOverrides(
			models,
			s.composition.modelsJSON.ModelOverrides,
		)
	}
	return cloneRuntimeModels(models), nil
}

func (s *composedModelProviderState) modifyModels(
	modify func([]llm.Model, AuthCredential) []llm.Model,
	models []llm.Model,
	credential llm.Credential,
) []llm.Model {
	s.modifyMu.Lock()
	defer s.modifyMu.Unlock()
	return cloneRuntimeModels(modify(
		cloneRuntimeModels(models),
		credential.Clone(),
	))
}

func (s *composedModelProviderState) refreshable() bool {
	if s == nil {
		return false
	}
	return (s.composition.base != nil &&
		s.composition.base.RefreshModelsFunc != nil) ||
		(s.composition.hasExtension &&
			(s.composition.extension.RefreshModels != nil ||
				(s.composition.extension.OAuth != nil &&
					s.composition.extension.OAuth.ModifyModels != nil)))
}

func (s *composedModelProviderState) refresh(
	ctx context.Context,
	input llm.RefreshModelsContext,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if base := s.composition.base; base != nil &&
		base.RefreshModelsFunc != nil {
		if err := base.RefreshModels(ctx, input); err != nil {
			return err
		}
	}
	if refresh := s.composition.extension.RefreshModels; refresh != nil {
		refreshed, err := refresh(ctx, input)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.validateRefreshedModels(refreshed); err != nil {
			return err
		}
		s.mu.Lock()
		s.refreshedExtensionModels = presentProviderModelDefinitions(
			refreshed,
		)
		s.hasRefreshedModels = true
		s.mu.Unlock()
	}
	if oauth := s.composition.extension.OAuth; oauth != nil &&
		oauth.ModifyModels != nil {
		var credential *llm.Credential
		if input.Credential != nil &&
			input.Credential.Type == llm.CredentialTypeOAuth {
			cloned := input.Credential.Clone()
			credential = &cloned
		}
		s.mu.Lock()
		s.extensionOAuthCredential = credential
		s.mu.Unlock()
	}
	return nil
}

func (s *composedModelProviderState) validateRefreshedModels(
	refreshed []ProviderModelDefinition,
) error {
	baseModels, err := providerModelSnapshot(s.composition.base)
	if err != nil {
		return err
	}
	models, err := applyModelsJSON(
		s.composition.providerID,
		baseModels,
		s.composition.modelsJSON,
		s.composition.hasModelsJSON,
	)
	if err != nil {
		return err
	}
	extension := cloneRuntimeProviderConfig(s.composition.extension)
	extension.Models = presentProviderModelDefinitions(refreshed)
	_, err = applyExtension(
		s.composition.providerID,
		models,
		extension,
		true,
	)
	return err
}

func applyModelsJSON(
	providerID string,
	baseModels []llm.Model,
	config modelsJSONProviderConfig,
	configured bool,
) ([]llm.Model, error) {
	models := cloneRuntimeModels(baseModels)
	if !configured {
		return models, nil
	}
	if config.OAuth != "" && config.BaseURL == "" {
		return nil, fmt.Errorf(
			`provider %s: "baseUrl" is required when "oauth" is set`,
			providerID,
		)
	}
	for index := range models {
		if config.BaseURL != "" && config.OAuth != RadiusProviderID {
			models[index].BaseURL = config.BaseURL
		}
		models[index].Compat = mergeCompat(
			models[index].Compat,
			config.Compat,
		)
	}
	for _, definition := range config.Models {
		existing := modelDefinitionDefaults(models, definition.ID)
		model, err := modelFromJSON(
			providerID,
			definition,
			config.API,
			config.BaseURL,
			config.Compat,
			existing,
		)
		if err != nil {
			return nil, err
		}
		models = upsertProviderModel(models, model)
	}
	return models, nil
}

func applyExtension(
	providerID string,
	baseModels []llm.Model,
	config ProviderConfigInput,
	configured bool,
) ([]llm.Model, error) {
	models := cloneRuntimeModels(baseModels)
	if !configured {
		return models, nil
	}
	if config.Models == nil {
		for index := range models {
			if config.BaseURL != "" {
				models[index].BaseURL = config.BaseURL
			}
			models[index].Compat = mergeCompat(
				models[index].Compat,
				config.Compat,
			)
		}
		return models, nil
	}
	replaced := make([]llm.Model, 0, len(config.Models))
	for _, definition := range config.Models {
		defaults := modelDefinitionDefaults(models, definition.ID)
		model, err := modelFromJSON(
			providerID,
			definition,
			config.API,
			config.BaseURL,
			config.Compat,
			defaults,
		)
		if err != nil {
			return nil, err
		}
		replaced = append(replaced, model)
	}
	return replaced, nil
}

func modelFromJSON(
	providerID string,
	definition ProviderModelDefinition,
	providerAPI string,
	providerBaseURL string,
	providerCompat llm.ModelCompat,
	defaults *llm.Model,
) (llm.Model, error) {
	if strings.TrimSpace(definition.ID) == "" {
		return llm.Model{}, fmt.Errorf(
			"provider %s: model missing \"id\"",
			providerID,
		)
	}
	api := firstNonEmptyString(definition.API, providerAPI)
	baseURL := firstNonEmptyString(
		definition.BaseURL,
		providerBaseURL,
	)
	if defaults != nil {
		api = firstNonEmptyString(api, defaults.API)
		baseURL = firstNonEmptyString(baseURL, defaults.BaseURL)
	}
	if api == "" {
		return llm.Model{}, fmt.Errorf(
			`provider %s, model %s: no "api" specified`,
			providerID,
			definition.ID,
		)
	}
	if baseURL == "" {
		return llm.Model{}, fmt.Errorf(
			`provider %s: "baseUrl" is required when defining custom models`,
			providerID,
		)
	}
	if definition.ContextWindow < 0 {
		return llm.Model{}, fmt.Errorf(
			"provider %s, model %s: invalid contextWindow",
			providerID,
			definition.ID,
		)
	}
	if definition.MaxTokens < 0 {
		return llm.Model{}, fmt.Errorf(
			"provider %s, model %s: invalid maxTokens",
			providerID,
			definition.ID,
		)
	}
	return buildModelFromDefinition(
		providerID,
		api,
		baseURL,
		providerCompat,
		definition,
	), nil
}

func modelDefinitionDefaults(
	models []llm.Model,
	modelID string,
) *llm.Model {
	for index := range models {
		if models[index].ID == modelID {
			model := cloneRuntimeModel(models[index])
			return &model
		}
	}
	if len(models) == 0 {
		return nil
	}
	model := cloneRuntimeModel(models[0])
	return &model
}

func upsertProviderModel(
	models []llm.Model,
	model llm.Model,
) []llm.Model {
	for index := range models {
		if models[index].ID == model.ID {
			models[index] = cloneRuntimeModel(model)
			return models
		}
	}
	return append(models, cloneRuntimeModel(model))
}

func providerModelSnapshot(
	provider *llm.Provider,
) ([]llm.Model, error) {
	if provider == nil {
		return nil, nil
	}
	models, err := provider.GetModels()
	if err != nil {
		return nil, err
	}
	return cloneRuntimeModels(models), nil
}

func composedProviderStreams(
	input modelProviderComposition,
) (
	func(llm.Model, llm.Context, llm.StreamOptions) (*llm.AssistantMessageEventStream, error),
	func(llm.Model, llm.Context, llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error),
) {
	baseSupportsAPI := func(api string) bool {
		baseModels, err := providerModelSnapshot(input.base)
		if err != nil {
			return false
		}
		for _, model := range baseModels {
			if model.API == api {
				return true
			}
		}
		return false
	}
	resolveAPI := func(model llm.Model) (llm.APIProvider, error) {
		provider := llm.GetAPIProvider(model.API)
		if provider == nil {
			return nil, fmt.Errorf(
				"no API provider registered for api: %s",
				model.API,
			)
		}
		return provider, nil
	}
	stream := func(
		model llm.Model,
		llmContext llm.Context,
		options llm.StreamOptions,
	) (*llm.AssistantMessageEventStream, error) {
		if input.hasExtension &&
			input.extension.StreamSimple != nil &&
			model.API == input.extension.API {
			return input.extension.StreamSimple(
				model,
				llmContext,
				options,
			)
		}
		if input.base != nil && baseSupportsAPI(model.API) {
			return input.base.Stream(model, llmContext, options)
		}
		provider, err := resolveAPI(model)
		if err != nil {
			return nil, err
		}
		return provider.Stream(model, llmContext, options)
	}
	streamSimple := func(
		model llm.Model,
		llmContext llm.Context,
		options llm.SimpleStreamOptions,
	) (*llm.AssistantMessageEventStream, error) {
		if input.hasExtension &&
			input.extension.StreamSimple != nil &&
			model.API == input.extension.API {
			return input.extension.StreamSimple(
				model,
				llmContext,
				options,
			)
		}
		if input.base != nil && baseSupportsAPI(model.API) {
			return input.base.StreamSimple(model, llmContext, options)
		}
		provider, err := resolveAPI(model)
		if err != nil {
			return nil, err
		}
		return provider.StreamSimple(model, llmContext, options)
	}
	return stream, streamSimple
}

func composeRuntimeProviderAuth(
	input modelProviderComposition,
) (llm.ProviderAuth, error) {
	var inherited llm.ProviderAuth
	if input.base != nil {
		inherited = input.base.Auth
	}
	rawKey, hasRawKey := configuredAPIKey(input)
	rawHeaders := configuredHeaders(input)
	authHeader := configuredProviderAuthHeader(input)

	oauth := composeOAuthAuth(
		input,
		rawHeaders,
		authHeader,
	)

	apiKey := inherited.APIKey
	if apiKey == nil && !hasRawKey && oauth != nil {
		return llm.ProviderAuth{OAuth: oauth}, nil
	}
	apiKey = composeAPIKeyAuth(
		input.providerID,
		apiKey,
		rawKey,
		rawHeaders,
		authHeader,
	)
	return llm.ProviderAuth{
		APIKey: apiKey,
		OAuth:  oauth,
	}, nil
}

func configuredAPIKey(
	input modelProviderComposition,
) (string, bool) {
	if input.hasExtension && input.extension.APIKey != "" {
		return input.extension.APIKey, true
	}
	if input.hasModelsJSON && input.modelsJSON.APIKey != "" {
		return input.modelsJSON.APIKey, true
	}
	return "", false
}

func composeOAuthAuth(
	input modelProviderComposition,
	rawHeaders map[string]string,
	authHeader bool,
) *llm.OAuthAuth {
	var oauth *llm.OAuthAuth
	if input.base != nil {
		oauth = input.base.Auth.OAuth
	}
	if input.hasExtension && input.extension.OAuth != nil {
		oauth = adaptOAuth(*input.extension.OAuth)
	}
	if oauth == nil {
		return nil
	}
	return decorateRuntimeOAuthAuth(
		input.providerID,
		oauth,
		rawHeaders,
		authHeader,
	)
}

func composeAPIKeyAuth(
	providerID string,
	inherited *llm.APIKeyAuth,
	rawKey string,
	rawHeaders map[string]string,
	authHeader bool,
) *llm.APIKeyAuth {
	name := "API key"
	var login func(
		context.Context,
		llm.AuthInteraction,
	) (llm.Credential, error)
	if inherited != nil {
		name = firstNonEmptyString(inherited.Name, name)
		login = inherited.Login
	}
	if login == nil {
		login = func(
			ctx context.Context,
			interaction llm.AuthInteraction,
		) (llm.Credential, error) {
			if interaction == nil {
				return llm.Credential{}, errors.New(
					"auth interaction is required",
				)
			}
			key, err := interaction.Prompt(ctx, llm.AuthPrompt{
				Type:    llm.AuthPromptSecret,
				Message: "Enter API key",
			})
			if err != nil {
				return llm.Credential{}, err
			}
			return llm.Credential{
				Type: llm.CredentialTypeAPIKey,
				Key:  key,
			}, nil
		}
	}
	return &llm.APIKeyAuth{
		Name:  name,
		Login: login,
		Check: func(
			ctx context.Context,
			input llm.APIKeyCheckInput,
		) (*llm.AuthCheck, error) {
			if input.Credential != nil {
				if inherited != nil && inherited.Check != nil {
					return inherited.Check(ctx, input)
				}
				if input.Credential.Key != "" {
					return &llm.AuthCheck{
						Type:   llm.CredentialTypeAPIKey,
						Source: "stored credential",
					}, nil
				}
				if inherited != nil && inherited.Resolve != nil {
					resolved, err := inherited.Resolve(
						ctx,
						llm.APIKeyResolveInput{
							Context:    input.Context,
							Credential: input.Credential,
						},
					)
					if err != nil || resolved == nil {
						return nil, err
					}
					return &llm.AuthCheck{
						Type:   llm.CredentialTypeAPIKey,
						Source: resolved.Source,
					}, nil
				}
			}
			if rawKey != "" {
				configured, err := runtimeConfigValueConfigured(
					ctx,
					input.Context,
					rawKey,
				)
				if err != nil || !configured {
					return nil, err
				}
				return &llm.AuthCheck{
					Type:   llm.CredentialTypeAPIKey,
					Source: "configured API key",
				}, nil
			}
			if inherited != nil && inherited.Check != nil {
				return inherited.Check(ctx, input)
			}
			if inherited == nil || inherited.Resolve == nil {
				return nil, nil
			}
			resolved, err := inherited.Resolve(
				ctx,
				llm.APIKeyResolveInput{Context: input.Context},
			)
			if err != nil || resolved == nil {
				return nil, err
			}
			return &llm.AuthCheck{
				Type:   llm.CredentialTypeAPIKey,
				Source: resolved.Source,
			}, nil
		},
		Resolve: func(
			ctx context.Context,
			input llm.APIKeyResolveInput,
		) (*llm.AuthResult, error) {
			var result *llm.AuthResult
			var err error
			switch {
			case input.Credential != nil:
				if inherited != nil && inherited.Resolve != nil {
					result, err = inherited.Resolve(ctx, input)
				} else {
					result = &llm.AuthResult{
						Auth: llm.ModelAuth{
							APIKey: input.Credential.Key,
						},
						Env: cloneResolvedProviderEnv(
							input.Credential.Env,
						),
						Source: "stored credential",
					}
				}
			case rawKey != "":
				env, envErr := configContextEnv(
					ctx,
					input.Context,
					[]string{rawKey},
					nil,
				)
				if envErr != nil {
					return nil, envErr
				}
				key, resolveErr := ResolveConfigValueOrError(
					rawKey,
					fmt.Sprintf(
						`API key for provider "%s"`,
						providerID,
					),
					env,
				)
				if resolveErr != nil {
					return nil, resolveErr
				}
				if inherited != nil && inherited.Resolve != nil {
					result, err = inherited.Resolve(
						ctx,
						llm.APIKeyResolveInput{
							Context: input.Context,
							Credential: &llm.Credential{
								Type: llm.CredentialTypeAPIKey,
								Key:  key,
							},
						},
					)
				} else {
					result = &llm.AuthResult{
						Auth:   llm.ModelAuth{APIKey: key},
						Env:    env,
						Source: "configured API key",
					}
				}
			case inherited != nil && inherited.Resolve != nil:
				result, err = inherited.Resolve(ctx, input)
			}
			if err != nil || result == nil {
				return result, err
			}
			return decorateRuntimeAuthResult(
				ctx,
				input.Context,
				providerID,
				result,
				rawHeaders,
				authHeader,
				input.Credential,
			)
		},
	}
}

func adaptOAuth(
	config OAuthProvider,
) *llm.OAuthAuth {
	var login func(
		context.Context,
		llm.AuthInteraction,
	) (llm.Credential, error)
	if config.Login != nil {
		login = func(
			ctx context.Context,
			interaction llm.AuthInteraction,
		) (llm.Credential, error) {
			credential, err := config.Login(ctx, interaction)
			if err != nil {
				return llm.Credential{}, err
			}
			credential.Type = llm.CredentialTypeOAuth
			return credential, nil
		}
	}
	return &llm.OAuthAuth{
		Name:  config.Name,
		Login: login,
		Refresh: func(
			_ context.Context,
			credential llm.Credential,
		) (llm.Credential, error) {
			if config.RefreshToken == nil {
				return llm.Credential{}, errors.New(
					"OAuth refresh is not configured",
				)
			}
			refreshed, err := config.RefreshToken(credential.Clone())
			if err != nil {
				return llm.Credential{}, err
			}
			refreshed.Type = llm.CredentialTypeOAuth
			return refreshed, nil
		},
		ToAuth: func(
			_ context.Context,
			credential llm.Credential,
		) (llm.ModelAuth, error) {
			return llm.ModelAuth{
				APIKey: config.APIKey(credential.Clone()),
			}, nil
		},
	}
}

func decorateRuntimeOAuthAuth(
	providerID string,
	oauth *llm.OAuthAuth,
	rawHeaders map[string]string,
	authHeader bool,
) *llm.OAuthAuth {
	decorated := *oauth
	baseToAuth := oauth.ToAuth
	decorated.ToAuth = func(
		ctx context.Context,
		credential llm.Credential,
	) (llm.ModelAuth, error) {
		if baseToAuth == nil {
			return llm.ModelAuth{}, errors.New(
				"OAuth request auth is not configured",
			)
		}
		auth, err := baseToAuth(ctx, credential)
		if err != nil {
			return llm.ModelAuth{}, err
		}
		env := cloneResolvedProviderEnv(credential.Env)
		headers, err := resolveHeadersOrError(
			rawHeaders,
			fmt.Sprintf(`provider "%s"`, providerID),
			env,
		)
		if err != nil {
			return llm.ModelAuth{}, err
		}
		return withConfiguredAuth(auth, headers, authHeader)
	}
	return &decorated
}

func decorateRuntimeAuthResult(
	ctx context.Context,
	authContext llm.AuthContext,
	providerID string,
	result *llm.AuthResult,
	rawHeaders map[string]string,
	authHeader bool,
	credential *llm.Credential,
) (*llm.AuthResult, error) {
	explicit := cloneResolvedProviderEnv(result.Env)
	if credential != nil {
		explicit = mergeResolvedProviderEnv(
			credential.Env,
			explicit,
		)
	}
	env, err := configContextEnv(
		ctx,
		authContext,
		mapValues(rawHeaders),
		explicit,
	)
	if err != nil {
		return nil, err
	}
	headers, err := resolveHeadersOrError(
		rawHeaders,
		fmt.Sprintf(`provider "%s"`, providerID),
		env,
	)
	if err != nil {
		return nil, err
	}
	auth, err := withConfiguredAuth(
		result.Auth,
		headers,
		authHeader,
	)
	if err != nil {
		return nil, err
	}
	cloned := *result
	cloned.Auth = auth
	cloned.Env = env
	return &cloned, nil
}

func withConfiguredAuth(
	auth llm.ModelAuth,
	headers map[string]string,
	authHeader bool,
) (llm.ModelAuth, error) {
	auth.Headers = mergeHeadersCaseInsensitive(
		auth.Headers,
		headers,
	)
	auth.HeaderRemovals = clearResolvedHeaderRemovals(
		auth.HeaderRemovals,
		headers,
	)
	if !authHeader {
		return auth, nil
	}
	if auth.APIKey == "" {
		return llm.ModelAuth{}, errors.New(
			"authHeader requires a resolved API key",
		)
	}
	auth.Headers = mergeHeadersCaseInsensitive(
		auth.Headers,
		map[string]string{
			"Authorization": "Bearer " + auth.APIKey,
		},
	)
	auth.HeaderRemovals = clearResolvedHeaderRemovals(
		auth.HeaderRemovals,
		map[string]string{"Authorization": ""},
	)
	return auth, nil
}

func configuredHeaders(
	input modelProviderComposition,
) map[string]string {
	var headers map[string]string
	if input.hasModelsJSON {
		headers = mergeHeadersCaseInsensitive(
			headers,
			input.modelsJSON.Headers,
		)
	}
	if input.hasExtension {
		headers = mergeHeadersCaseInsensitive(
			headers,
			input.extension.Headers,
		)
	}
	return headers
}

func configuredProviderAuthHeader(
	input modelProviderComposition,
) bool {
	authHeader := false
	if input.hasModelsJSON && input.modelsJSON.AuthHeader != nil {
		authHeader = *input.modelsJSON.AuthHeader
	}
	if input.hasExtension && input.extension.AuthHeader != nil {
		authHeader = *input.extension.AuthHeader
	}
	return authHeader
}

func rawModelHeaders(
	modelID string,
	config modelsJSONProviderConfig,
	hasConfig bool,
	extension ProviderConfigInput,
	hasExtension bool,
) map[string]string {
	var headers map[string]string
	if hasConfig {
		if override, ok := config.ModelOverrides[modelID]; ok {
			headers = mergeHeadersCaseInsensitive(
				headers,
				override.Headers,
			)
		}
		for _, definition := range config.Models {
			if definition.ID == modelID {
				headers = mergeHeadersCaseInsensitive(
					headers,
					definition.Headers,
				)
				break
			}
		}
	}
	if hasExtension {
		for _, definition := range extension.Models {
			if definition.ID == modelID {
				headers = mergeHeadersCaseInsensitive(
					headers,
					definition.Headers,
				)
				break
			}
		}
	}
	return headers
}

func resolveConfiguredModelHeaders(
	model llm.Model,
	config modelsJSONProviderConfig,
	hasConfig bool,
	extension ProviderConfigInput,
	hasExtension bool,
	env llm.ProviderEnv,
) (map[string]string, error) {
	return resolveHeadersOrError(
		rawModelHeaders(
			model.ID,
			config,
			hasConfig,
			extension,
			hasExtension,
		),
		fmt.Sprintf(`model "%s/%s"`, model.Provider, model.ID),
		env,
	)
}

func resolveCompatibilityRequestConfig(
	model llm.Model,
	config ProviderRequestConfig,
	modelsJSON modelsJSONProviderConfig,
	hasModelsJSON bool,
	extension ProviderConfigInput,
	hasExtension bool,
) ProviderRequestConfig {
	config.Headers = mergeHeadersCaseInsensitive(
		model.Headers,
		config.Headers,
		rawModelHeaders(
			model.ID,
			modelsJSON,
			hasModelsJSON,
			extension,
			hasExtension,
		),
	)
	return config
}

func configContextEnv(
	ctx context.Context,
	authContext llm.AuthContext,
	values []string,
	explicit llm.ProviderEnv,
) (llm.ProviderEnv, error) {
	env := cloneResolvedProviderEnv(explicit)
	if env == nil {
		env = llm.ProviderEnv{}
	}
	if authContext == nil {
		authContext = llm.DefaultProviderAuthContext()
	}
	for _, value := range values {
		for _, name := range ConfigValueEnvVarNames(value) {
			if _, ok := env[name]; ok {
				continue
			}
			resolved, ok, err := authContext.Env(ctx, name)
			if err != nil {
				return nil, err
			}
			if ok {
				env[name] = resolved
			}
		}
	}
	if len(env) == 0 {
		return nil, nil
	}
	return env, nil
}

func runtimeConfigValueConfigured(
	ctx context.Context,
	authContext llm.AuthContext,
	value string,
) (bool, error) {
	if IsCommandConfigValue(value) {
		return true, nil
	}
	env, err := configContextEnv(
		ctx,
		authContext,
		[]string{value},
		nil,
	)
	if err != nil {
		return false, err
	}
	return IsConfigValueConfigured(value, env), nil
}

func configuredRequestAuthStatus(
	config modelsJSONProviderConfig,
	hasConfig bool,
	extension ProviderConfigInput,
	hasExtension bool,
) *AuthStatus {
	value := ""
	source := "models_json_key"
	if hasConfig {
		value = config.APIKey
	}
	if hasExtension && extension.APIKey != "" {
		value = extension.APIKey
		source = "fallback"
	}
	if value == "" {
		return nil
	}
	status := configuredAPIKeyAuthStatus(value)
	if status.Configured &&
		status.Source == "models_json_key" {
		status.Source = source
	}
	return &status
}

func mapValues(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func cloneProviderModelDefinitions(
	definitions []ProviderModelDefinition,
) []ProviderModelDefinition {
	if definitions == nil {
		return nil
	}
	cloned := make([]ProviderModelDefinition, len(definitions))
	for index, definition := range definitions {
		cloned[index] = definition
		cloned[index].Headers = cloneStringMap(definition.Headers)
		cloned[index].Input = append(
			[]string(nil),
			definition.Input...,
		)
		cloned[index].ThinkingLevelMap = cloneThinkingLevelMap(
			definition.ThinkingLevelMap,
		)
		cloned[index].Cost.Tiers = append(
			[]llm.ModelCostTier(nil),
			definition.Cost.Tiers...,
		)
		cloned[index].Compat = cloneRuntimeModel(llm.Model{
			Compat: definition.Compat,
		}).Compat
	}
	return cloned
}

func presentProviderModelDefinitions(
	definitions []ProviderModelDefinition,
) []ProviderModelDefinition {
	cloned := cloneProviderModelDefinitions(definitions)
	if cloned == nil {
		return []ProviderModelDefinition{}
	}
	return cloned
}

func cloneModelsJSONProviderConfigs(
	configs map[string]modelsJSONProviderConfig,
) map[string]modelsJSONProviderConfig {
	cloned := make(
		map[string]modelsJSONProviderConfig,
		len(configs),
	)
	for providerID, config := range configs {
		cloned[providerID] = cloneModelsJSONProviderConfig(config)
	}
	return cloned
}

func cloneModelsJSONProviderConfig(
	config modelsJSONProviderConfig,
) modelsJSONProviderConfig {
	config.Headers = cloneOptionalStringMap(config.Headers)
	config.Models = cloneProviderModelDefinitions(config.Models)
	config.ModelOverrides = cloneModelOverrideMap(
		config.ModelOverrides,
	)
	config.Compat = cloneRuntimeModel(llm.Model{
		Compat: config.Compat,
	}).Compat
	if config.AuthHeader != nil {
		authHeader := *config.AuthHeader
		config.AuthHeader = &authHeader
	}
	return config
}

func cloneCredentialPointerForComposer(
	credential *llm.Credential,
) *llm.Credential {
	if credential == nil {
		return nil
	}
	cloned := credential.Clone()
	return &cloned
}

func oauthProviderName(provider *OAuthProvider) string {
	if provider == nil {
		return ""
	}
	return provider.Name
}

func (r *ModelRegistry) providerCompositionSnapshot(
	providerID string,
) (
	modelsJSONProviderConfig,
	bool,
	ProviderConfigInput,
	bool,
) {
	if r == nil {
		return modelsJSONProviderConfig{},
			false,
			ProviderConfigInput{},
			false
	}
	r.mu.RLock()
	modelsJSON, hasModelsJSON := r.modelsJSONProviders[providerID]
	extension, hasExtension := r.registeredProviders[providerID]
	r.mu.RUnlock()
	return cloneModelsJSONProviderConfig(modelsJSON),
		hasModelsJSON,
		cloneRuntimeProviderConfig(extension),
		hasExtension
}

func (r *ModelRegistry) compositionProviderIDs() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	seen := make(
		map[string]struct{},
		len(r.modelsJSONProviders)+len(r.registeredOrder),
	)
	ids := make(
		[]string,
		0,
		len(r.modelsJSONProviders)+len(r.registeredOrder),
	)
	for providerID := range r.modelsJSONProviders {
		seen[providerID] = struct{}{}
		ids = append(ids, providerID)
	}
	sort.Strings(ids)
	for _, providerID := range r.registeredOrder {
		if _, ok := seen[providerID]; ok {
			continue
		}
		seen[providerID] = struct{}{}
		ids = append(ids, providerID)
	}
	r.mu.RUnlock()
	return ids
}
