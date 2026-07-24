package gillmprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ModelsOptions supplies app-owned state boundaries to a Models collection.
type ModelsOptions struct {
	Credentials CredentialStore
	ModelsStore ModelsStore
	AuthContext AuthContext
}

// ModelsRefreshOptions controls dynamic catalog refresh. Offline is false by
// default so the zero value permits network access.
type ModelsRefreshOptions struct {
	Offline bool
	Force   bool
}

// ModelsRefreshResult reports cancellation and provider-specific failures
// without failing the entire concurrent refresh.
type ModelsRefreshResult struct {
	Aborted bool
	Errors  map[string]error
}

// TransformHeadersFunc transforms fully assembled request headers exactly once
// before provider dispatch.
type TransformHeadersFunc func(ctx context.Context, headers map[string]string) (map[string]string, error)

// ModelsStreamOptions wraps provider options with Models-only request
// assembly controls. APIKey shadows StreamOptions.APIKey so an explicit empty
// override remains distinguishable from no override.
type ModelsStreamOptions struct {
	StreamOptions
	APIKey           *string
	TransformHeaders TransformHeadersFunc
}

// Models is an instance-scoped provider collection. It owns provider
// registration and orchestrates credentials, model catalogs, and request
// assembly while APIProvider implementations remain transport-only.
type Models struct {
	mu        sync.RWMutex
	providers map[string]*Provider
	order     []string

	credentials CredentialStore
	modelsStore ModelsStore
	authContext AuthContext
}

// NewModels creates an isolated provider collection.
func NewModels(options ...ModelsOptions) *Models {
	var selected ModelsOptions
	if len(options) > 0 {
		selected = options[0]
	}
	if selected.Credentials == nil {
		selected.Credentials = NewInMemoryCredentialStore()
	}
	if selected.ModelsStore == nil {
		selected.ModelsStore = NewInMemoryModelsStore()
	}
	if selected.AuthContext == nil {
		selected.AuthContext = DefaultProviderAuthContext()
	}
	return &Models{
		providers:   map[string]*Provider{},
		credentials: selected.Credentials,
		modelsStore: selected.ModelsStore,
		authContext: selected.AuthContext,
	}
}

// SetProvider inserts or replaces a provider by ID without changing its
// existing position.
func (m *Models) SetProvider(provider *Provider) error {
	if m == nil {
		return errors.New("models collection is nil")
	}
	if provider == nil || provider.ID == "" {
		return errors.New("provider ID is required")
	}
	if provider.Auth.APIKey == nil && provider.Auth.OAuth == nil {
		return fmt.Errorf("provider %s has no authentication method", provider.ID)
	}
	m.mu.Lock()
	if _, exists := m.providers[provider.ID]; !exists {
		m.order = append(m.order, provider.ID)
	}
	m.providers[provider.ID] = provider
	m.mu.Unlock()
	return nil
}

// DeleteProvider removes a provider from this collection.
func (m *Models) DeleteProvider(providerID string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if _, exists := m.providers[providerID]; exists {
		delete(m.providers, providerID)
		for index, id := range m.order {
			if id == providerID {
				m.order = append(m.order[:index], m.order[index+1:]...)
				break
			}
		}
	}
	m.mu.Unlock()
}

// ClearProviders removes every provider without changing credentials or
// persisted dynamic catalogs.
func (m *Models) ClearProviders() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.providers = map[string]*Provider{}
	m.order = nil
	m.mu.Unlock()
}

// GetProviders returns providers in registration order.
func (m *Models) GetProviders() []*Provider {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	providers := make([]*Provider, 0, len(m.order))
	for _, providerID := range m.order {
		if provider := m.providers[providerID]; provider != nil {
			providers = append(providers, provider)
		}
	}
	m.mu.RUnlock()
	return providers
}

// GetProvider finds one registered provider.
func (m *Models) GetProvider(providerID string) (*Provider, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.RLock()
	provider, ok := m.providers[providerID]
	m.mu.RUnlock()
	return provider, ok
}

// GetModels returns the last-known models for one provider or every provider.
// Provider source errors are isolated and yield no models.
func (m *Models) GetModels(providerID ...string) []Model {
	if len(providerID) > 0 {
		provider, ok := m.GetProvider(providerID[0])
		if !ok {
			return nil
		}
		models, err := provider.GetModels()
		if err != nil {
			return nil
		}
		return models
	}
	var result []Model
	for _, provider := range m.GetProviders() {
		models, err := provider.GetModels()
		if err == nil {
			result = append(result, models...)
		}
	}
	return result
}

// GetModel finds a model within one provider's last-known catalog.
func (m *Models) GetModel(providerID, modelID string) (Model, bool) {
	for _, model := range m.GetModels(providerID) {
		if model.ID == modelID {
			return model, true
		}
	}
	return Model{}, false
}

// Refresh concurrently refreshes configured dynamic providers. Failures are
// returned per provider and do not discard another provider's successful
// result.
func (m *Models) Refresh(ctx context.Context, options ModelsRefreshOptions) ModelsRefreshResult {
	ctx = contextOrBackground(ctx)
	providers := m.GetProviders()
	result := ModelsRefreshResult{Errors: map[string]error{}}
	var (
		wait     sync.WaitGroup
		errorsMu sync.Mutex
	)
	for _, provider := range providers {
		if provider == nil || provider.RefreshModelsFunc == nil {
			continue
		}
		wait.Add(1)
		go func(provider *Provider) {
			defer wait.Done()
			if ctx.Err() != nil {
				return
			}

			var (
				stored       Credential
				storedExists bool
			)
			stored, storedExists, err := m.readCredential(ctx, provider.ID)
			if err == nil {
				var effective *Credential
				effective, err = m.resolveRefreshCredential(
					ctx,
					provider,
					stored,
					storedExists,
					!options.Offline,
				)
				if err == nil && effective == nil {
					return
				}
				if err == nil {
					err = provider.RefreshModels(ctx, RefreshModelsContext{
						Credential:   effective,
						Store:        scopedModelsStore{store: m.modelsStore, providerID: provider.ID},
						AllowNetwork: !options.Offline,
						Force:        options.Force,
					})
				}
			}
			if err == nil {
				return
			}
			if ctx.Err() == nil {
				errorsMu.Lock()
				result.Errors[provider.ID] = err
				errorsMu.Unlock()
			}

			var fallback *Credential
			if storedExists {
				fallback = cloneCredentialPointer(&stored)
			}
			_ = provider.RefreshModels(ctx, RefreshModelsContext{
				Credential:   fallback,
				Store:        scopedModelsStore{store: m.modelsStore, providerID: provider.ID},
				AllowNetwork: false,
			})
		}(provider)
	}
	wait.Wait()
	result.Aborted = ctx.Err() != nil
	return result
}

func (m *Models) resolveRefreshCredential(
	ctx context.Context,
	provider *Provider,
	stored Credential,
	storedExists bool,
	allowNetwork bool,
) (*Credential, error) {
	if storedExists && stored.Type == CredentialTypeOAuth {
		if provider.Auth.OAuth == nil {
			return nil, nil
		}
		if !allowNetwork || time.Now().UnixMilli() < stored.Expires {
			return cloneCredentialPointer(&stored), nil
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		post, exists, err := m.credentials.ModifyCredential(
			ctx,
			provider.ID,
			func(ctx context.Context, current Credential, exists bool) (Credential, bool, error) {
				if !exists || current.Type != CredentialTypeOAuth {
					return Credential{}, false, nil
				}
				if time.Now().UnixMilli() < current.Expires {
					return Credential{}, false, nil
				}
				if provider.Auth.OAuth.Refresh == nil {
					return Credential{}, false, errors.New("OAuth refresh is not configured")
				}
				refreshed, err := provider.Auth.OAuth.Refresh(ctx, current.Clone())
				if err != nil {
					return Credential{}, false, err
				}
				if refreshed.Type != CredentialTypeOAuth {
					return Credential{}, false, fmt.Errorf(
						"OAuth refresh returned credential type %q",
						refreshed.Type,
					)
				}
				return refreshed, true, nil
			},
		)
		if err != nil {
			return nil, err
		}
		if !exists || post.Type != CredentialTypeOAuth {
			return nil, nil
		}
		return cloneCredentialPointer(&post), nil
	}

	apiKey := provider.Auth.APIKey
	if apiKey == nil || apiKey.Resolve == nil {
		return nil, nil
	}
	var credential *Credential
	if storedExists && stored.Type == CredentialTypeAPIKey {
		credential = cloneCredentialPointer(&stored)
	}
	resolution, err := apiKey.Resolve(ctx, APIKeyResolveInput{
		Context:    m.authContext,
		Credential: credential,
	})
	if err != nil {
		return nil, newModelsError(
			ModelsErrorAuth,
			fmt.Sprintf("API key auth failed for provider %s", provider.ID),
			err,
		)
	}
	if resolution == nil {
		return nil, nil
	}
	return &Credential{
		Type: CredentialTypeAPIKey,
		Key:  resolution.Auth.APIKey,
		Env:  cloneProviderEnv(resolution.Env),
	}, nil
}

// CheckAuth reports provider auth availability without refreshing OAuth.
func (m *Models) CheckAuth(ctx context.Context, providerID string) (AuthCheck, bool, error) {
	ctx = contextOrBackground(ctx)
	provider, ok := m.GetProvider(providerID)
	if !ok {
		return AuthCheck{}, false, nil
	}
	credential, exists, err := m.readCredential(ctx, providerID)
	if err != nil {
		return AuthCheck{}, false, err
	}
	return m.checkProviderAuth(ctx, provider, credential, exists)
}

func (m *Models) checkProviderAuth(
	ctx context.Context,
	provider *Provider,
	credential Credential,
	exists bool,
) (AuthCheck, bool, error) {
	if exists && credential.Type == CredentialTypeOAuth {
		if provider.Auth.OAuth == nil {
			return AuthCheck{}, false, nil
		}
		return AuthCheck{Source: "OAuth", Type: CredentialTypeOAuth}, true, nil
	}
	apiKey := provider.Auth.APIKey
	if apiKey == nil {
		return AuthCheck{}, false, nil
	}
	if apiKey.Check != nil {
		var stored *Credential
		if exists && credential.Type == CredentialTypeAPIKey {
			stored = cloneCredentialPointer(&credential)
		}
		check, err := apiKey.Check(ctx, APIKeyCheckInput{
			Context:    m.authContext,
			Credential: stored,
		})
		if err != nil {
			return AuthCheck{}, false, newModelsError(
				ModelsErrorAuth,
				fmt.Sprintf("API key auth check failed for provider %s", provider.ID),
				err,
			)
		}
		if check == nil {
			return AuthCheck{}, false, nil
		}
		return *check, true, nil
	}

	resolution, err := ResolveProviderAuth(
		ctx,
		provider.ID,
		provider.Auth,
		m.credentials,
		m.authContext,
		AuthResolutionOverrides{},
	)
	if err != nil || resolution == nil {
		return AuthCheck{}, false, err
	}
	return AuthCheck{
		Source: resolution.Source,
		Type:   CredentialTypeAPIKey,
	}, true, nil
}

// GetAvailable returns models whose providers have complete auth. Provider
// checks run concurrently while output remains in provider registration order.
func (m *Models) GetAvailable(ctx context.Context, providerID ...string) ([]Model, error) {
	ctx = contextOrBackground(ctx)
	var providers []*Provider
	if len(providerID) > 0 {
		if provider, ok := m.GetProvider(providerID[0]); ok {
			providers = []*Provider{provider}
		}
	} else {
		providers = m.GetProviders()
	}
	type providerModelsResult struct {
		models []Model
		err    error
	}
	results := make([]providerModelsResult, len(providers))
	var wait sync.WaitGroup
	for index, provider := range providers {
		wait.Add(1)
		go func(index int, provider *Provider) {
			defer wait.Done()
			credential, exists, err := m.readCredential(ctx, provider.ID)
			if err != nil {
				results[index].err = err
				return
			}
			_, configured, err := m.checkProviderAuth(ctx, provider, credential, exists)
			if err != nil || !configured {
				results[index].err = err
				return
			}
			models, err := provider.GetModels()
			if err != nil {
				results[index].err = newModelsError(
					ModelsErrorModelSource,
					fmt.Sprintf("Model source failed for %s", provider.ID),
					err,
				)
				return
			}
			var stored *Credential
			if exists {
				stored = &credential
			}
			results[index].models = provider.FilterModels(models, stored)
		}(index, provider)
	}
	wait.Wait()

	var models []Model
	for _, result := range results {
		if result.err != nil {
			return nil, result.err
		}
		models = append(models, result.models...)
	}
	return models, nil
}

// GetAuth resolves request auth for one provider.
func (m *Models) GetAuth(
	ctx context.Context,
	providerID string,
	overrides AuthResolutionOverrides,
) (*AuthResult, error) {
	ctx = contextOrBackground(ctx)
	provider, ok := m.GetProvider(providerID)
	if !ok {
		return nil, nil
	}
	return ResolveProviderAuth(
		ctx,
		provider.ID,
		provider.Auth,
		m.credentials,
		m.authContext,
		overrides,
	)
}

// GetModelAuth resolves provider auth and overlays static model headers.
func (m *Models) GetModelAuth(
	ctx context.Context,
	model Model,
	overrides AuthResolutionOverrides,
) (*AuthResult, error) {
	result, err := m.GetAuth(ctx, model.Provider, overrides)
	if err != nil || result == nil || len(model.Headers) == 0 {
		return result, err
	}
	cloned := cloneAuthResult(*result)
	cloned.Auth.Headers = mergeHeadersCaseInsensitive(cloned.Auth.Headers, model.Headers)
	cloned.Auth.HeaderRemovals = clearOverriddenHeaderRemovals(
		cloned.Auth.HeaderRemovals,
		model.Headers,
	)
	return &cloned, nil
}

// Login runs a provider-owned login flow and persists its credential through
// the canonical serialized mutation path.
func (m *Models) Login(
	ctx context.Context,
	providerID string,
	credentialType CredentialType,
	interaction AuthInteraction,
) (Credential, error) {
	ctx = contextOrBackground(ctx)
	provider, ok := m.GetProvider(providerID)
	if !ok {
		return Credential{}, newModelsError(
			ModelsErrorProvider,
			fmt.Sprintf("Unknown provider: %s", providerID),
			nil,
		)
	}
	var login func(context.Context, AuthInteraction) (Credential, error)
	switch credentialType {
	case CredentialTypeAPIKey:
		if provider.Auth.APIKey != nil {
			login = provider.Auth.APIKey.Login
		}
	case CredentialTypeOAuth:
		if provider.Auth.OAuth != nil {
			login = provider.Auth.OAuth.Login
		}
	default:
		return Credential{}, newModelsError(
			ModelsErrorAuth,
			fmt.Sprintf("Unsupported credential type: %s", credentialType),
			nil,
		)
	}
	if login == nil {
		return Credential{}, newModelsError(
			ModelsErrorAuth,
			fmt.Sprintf("%s does not support %s login", provider.Name, credentialType),
			nil,
		)
	}
	credential, err := login(contextOrBackground(ctx), interaction)
	if err != nil {
		return Credential{}, err
	}
	if credential.Type != credentialType {
		return Credential{}, newModelsError(
			ModelsErrorAuth,
			fmt.Sprintf(
				"%s login returned credential type %q, want %q",
				provider.Name,
				credential.Type,
				credentialType,
			),
			nil,
		)
	}
	persisted, _, err := m.credentials.ModifyCredential(
		ctx,
		providerID,
		func(context.Context, Credential, bool) (Credential, bool, error) {
			return credential.Clone(), true, nil
		},
	)
	if err != nil {
		return Credential{}, newModelsError(
			ModelsErrorAuth,
			fmt.Sprintf("Credential store modify failed for %s", providerID),
			err,
		)
	}
	return persisted, nil
}

// Logout removes one provider's stored credential.
func (m *Models) Logout(ctx context.Context, providerID string) error {
	ctx = contextOrBackground(ctx)
	if err := m.credentials.DeleteCredential(ctx, providerID); err != nil {
		return newModelsError(
			ModelsErrorAuth,
			fmt.Sprintf("Credential store delete failed for %s", providerID),
			err,
		)
	}
	return nil
}

// Stream resolves auth asynchronously and forwards the provider's native event
// stream. Setup failures become terminal stream errors.
func (m *Models) Stream(
	ctx context.Context,
	model Model,
	llmContext Context,
	options ModelsStreamOptions,
) *AssistantMessageEventStream {
	ctx = contextOrBackground(ctx)
	output := NewAssistantMessageEventStream()
	go func() {
		provider, requestModel, requestOptions, err := m.applyAuth(ctx, model, options)
		if err != nil {
			pushModelsStreamError(output, model, err, ctx.Err() != nil)
			return
		}
		stream, err := provider.Stream(requestModel, llmContext, requestOptions)
		forwardAssistantStream(ctx, output, stream, requestModel, err)
	}()
	return output
}

// Complete collects Stream's terminal assistant message.
func (m *Models) Complete(
	ctx context.Context,
	model Model,
	llmContext Context,
	options ModelsStreamOptions,
) (Message, error) {
	ctx = contextOrBackground(ctx)
	message, err := m.Stream(ctx, model, llmContext, options).Result(ctx)
	if err != nil && ctx.Err() != nil {
		return AssistantErrorMessage(ctx.Err().Error(), model, true), nil
	}
	return message, err
}

// StreamSimple is the simple-options form of Stream.
func (m *Models) StreamSimple(
	ctx context.Context,
	model Model,
	llmContext Context,
	options ModelsStreamOptions,
) *AssistantMessageEventStream {
	ctx = contextOrBackground(ctx)
	output := NewAssistantMessageEventStream()
	go func() {
		provider, requestModel, requestOptions, err := m.applyAuth(ctx, model, options)
		if err != nil {
			pushModelsStreamError(output, model, err, ctx.Err() != nil)
			return
		}
		stream, err := provider.StreamSimple(requestModel, llmContext, requestOptions)
		forwardAssistantStream(ctx, output, stream, requestModel, err)
	}()
	return output
}

// CompleteSimple collects StreamSimple's terminal assistant message.
func (m *Models) CompleteSimple(
	ctx context.Context,
	model Model,
	llmContext Context,
	options ModelsStreamOptions,
) (Message, error) {
	ctx = contextOrBackground(ctx)
	message, err := m.StreamSimple(ctx, model, llmContext, options).Result(ctx)
	if err != nil && ctx.Err() != nil {
		return AssistantErrorMessage(ctx.Err().Error(), model, true), nil
	}
	return message, err
}

func (m *Models) applyAuth(
	ctx context.Context,
	model Model,
	options ModelsStreamOptions,
) (*Provider, Model, StreamOptions, error) {
	provider, ok := m.GetProvider(model.Provider)
	if !ok {
		return nil, Model{}, StreamOptions{}, newModelsError(
			ModelsErrorProvider,
			fmt.Sprintf("Unknown provider: %s", model.Provider),
			nil,
		)
	}
	apiKeyOverride := options.APIKey
	if apiKeyOverride == nil && options.StreamOptions.APIKey != "" {
		apiKey := options.StreamOptions.APIKey
		apiKeyOverride = &apiKey
	}
	resolution, err := m.GetModelAuth(ctx, model, AuthResolutionOverrides{
		APIKey: apiKeyOverride,
		Env:    options.StreamOptions.Env,
	})
	if err != nil {
		return nil, Model{}, StreamOptions{}, err
	}
	if resolution == nil {
		return nil, Model{}, StreamOptions{}, newModelsError(
			ModelsErrorAuth,
			fmt.Sprintf("Provider is not configured: %s", model.Provider),
			nil,
		)
	}

	requestOptions := cloneStreamOptions(options.StreamOptions)
	requestOptions.Context = ctx
	requestOptions.APIKey = resolution.Auth.APIKey
	if apiKeyOverride != nil {
		requestOptions.APIKey = *apiKeyOverride
	}
	requestOptions.Headers = mergeHeadersCaseInsensitive(
		resolution.Auth.Headers,
		options.StreamOptions.Headers,
	)
	requestOptions.HeaderRemovals = clearOverriddenHeaderRemovals(
		resolution.Auth.HeaderRemovals,
		options.StreamOptions.Headers,
	)
	requestOptions.HeaderRemovals = appendUniqueHeaderRemovals(
		requestOptions.HeaderRemovals,
		options.StreamOptions.HeaderRemovals,
	)
	if options.TransformHeaders != nil {
		requestOptions.Headers, err = options.TransformHeaders(
			ctx,
			cloneStringMap(requestOptions.Headers),
		)
		if err != nil {
			return nil, Model{}, StreamOptions{}, err
		}
		requestOptions.Headers = cloneStringMap(requestOptions.Headers)
		requestOptions.HeaderRemovals = clearOverriddenHeaderRemovals(
			requestOptions.HeaderRemovals,
			requestOptions.Headers,
		)
	}
	requestOptions.Env = mergeProviderEnv(resolution.Env, options.StreamOptions.Env)

	requestModel := cloneModel(model)
	if resolution.Auth.BaseURL != "" {
		requestModel.BaseURL = resolution.Auth.BaseURL
	}
	return provider, requestModel, requestOptions, nil
}

func (m *Models) readCredential(
	ctx context.Context,
	providerID string,
) (Credential, bool, error) {
	credential, exists, err := m.credentials.ReadCredential(
		contextOrBackground(ctx),
		providerID,
	)
	if err != nil {
		return Credential{}, false, newModelsError(
			ModelsErrorAuth,
			fmt.Sprintf("Credential store read failed for %s", providerID),
			err,
		)
	}
	return credential, exists, nil
}

func mergeHeadersCaseInsensitive(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := cloneStringMap(base)
	if merged == nil {
		merged = map[string]string{}
	}
	for name, value := range override {
		for existing := range merged {
			if strings.EqualFold(existing, name) {
				delete(merged, existing)
			}
		}
		merged[name] = value
	}
	return merged
}

func cloneStreamOptions(options StreamOptions) StreamOptions {
	options.ThinkingBudgets = cloneIntMap(options.ThinkingBudgets)
	options.Headers = cloneStringMap(options.Headers)
	options.HeaderRemovals = append([]string(nil), options.HeaderRemovals...)
	options.Env = cloneProviderEnv(options.Env)
	options.Metadata = cloneCredentialMetadata(options.Metadata)
	return options
}

func cloneIntMap(values map[string]int) map[string]int {
	if values == nil {
		return nil
	}
	cloned := make(map[string]int, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func forwardAssistantStream(
	ctx context.Context,
	output *AssistantMessageEventStream,
	input *AssistantMessageEventStream,
	model Model,
	setupErr error,
) {
	if setupErr != nil {
		pushModelsStreamError(output, model, setupErr, ctx.Err() != nil)
		return
	}
	if input == nil {
		pushModelsStreamError(output, model, errors.New("provider returned a nil stream"), false)
		return
	}
	for {
		select {
		case <-ctx.Done():
			pushModelsStreamError(output, model, ctx.Err(), true)
			return
		case event, ok := <-input.Events():
			if !ok {
				result, err := input.Result(ctx)
				if err != nil {
					pushModelsStreamError(output, model, err, ctx.Err() != nil)
					return
				}
				output.End(result)
				return
			}
			output.Push(event)
			if event.Type == "done" || event.Type == "error" {
				return
			}
		}
	}
}

func pushModelsStreamError(
	stream *AssistantMessageEventStream,
	model Model,
	err error,
	aborted bool,
) {
	message := AssistantErrorMessage(err.Error(), model, aborted)
	stream.Push(AssistantMessageEvent{
		Type:   "error",
		Reason: message.StopReason,
		Error:  message,
	})
}
