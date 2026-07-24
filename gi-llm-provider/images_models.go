package gillmprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// ImagesModelsOptions is shared with Models so text and image collections can
// use the same application-owned credential and authentication state.
type ImagesModelsOptions = ModelsOptions

// ImagesProvider is the image-generation counterpart of Provider. Its
// callbacks are immutable after registration; refreshed model state is owned
// by the callback implementation or a provider built with
// CreateImagesProvider.
type ImagesProvider struct {
	ID   string
	Name string
	Auth ProviderAuth

	ModelSource        func() ([]ImagesModel, error)
	RefreshModelsFunc  func(ctx context.Context) error
	GenerateImagesFunc func(
		model ImagesModel,
		imagesContext ImagesContext,
		options ImagesOptions,
	) (AssistantImages, error)

	refreshMu sync.Mutex
	inflight  *providerRefreshCall
}

// CreateImagesProviderOptions describes an image provider with a static model
// snapshot, optional dynamic refresh, and one transport implementation.
type CreateImagesProviderOptions struct {
	ID     string
	Name   string
	Auth   ProviderAuth
	Models []ImagesModel

	FetchModels func(ctx context.Context) ([]ImagesModel, error)
	API         ImagesAPIProvider
}

type createdImagesProviderState struct {
	mu     sync.RWMutex
	models []ImagesModel
	fetch  func(ctx context.Context) ([]ImagesModel, error)
}

// CreateImagesProvider constructs an image provider with clone-on-read model
// state and coalesced dynamic refreshes.
func CreateImagesProvider(options CreateImagesProviderOptions) (*ImagesProvider, error) {
	if strings.TrimSpace(options.ID) == "" {
		return nil, errors.New("provider ID is required")
	}
	if options.Auth.APIKey == nil && options.Auth.OAuth == nil {
		return nil, fmt.Errorf("provider %s has no authentication method", options.ID)
	}
	if options.API == nil {
		return nil, fmt.Errorf("provider %s has no image API implementation", options.ID)
	}

	state := &createdImagesProviderState{
		models: cloneImagesModels(options.Models),
		fetch:  options.FetchModels,
	}
	provider := &ImagesProvider{
		ID:          options.ID,
		Name:        options.Name,
		Auth:        options.Auth,
		ModelSource: state.getModels,
		GenerateImagesFunc: func(
			model ImagesModel,
			imagesContext ImagesContext,
			generationOptions ImagesOptions,
		) (AssistantImages, error) {
			return options.API.GenerateImages(model, imagesContext, generationOptions)
		},
	}
	if provider.Name == "" {
		provider.Name = provider.ID
	}
	if options.FetchModels != nil {
		provider.RefreshModelsFunc = state.refresh
	}
	return provider, nil
}

// GetModels returns a detached snapshot of the provider's current catalog.
func (p *ImagesProvider) GetModels() ([]ImagesModel, error) {
	if p == nil || p.ModelSource == nil {
		return nil, nil
	}
	models, err := p.ModelSource()
	if err != nil {
		return nil, err
	}
	return cloneImagesModels(models), nil
}

// RefreshModels updates a dynamic provider's catalog. Concurrent calls share
// one in-flight refresh.
func (p *ImagesProvider) RefreshModels(ctx context.Context) error {
	if p == nil || p.RefreshModelsFunc == nil {
		return nil
	}
	ctx = contextOrBackground(ctx)
	p.refreshMu.Lock()
	if inflight := p.inflight; inflight != nil {
		p.refreshMu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-inflight.done:
			return inflight.err
		}
	}
	call := &providerRefreshCall{done: make(chan struct{})}
	p.inflight = call
	p.refreshMu.Unlock()

	call.err = p.RefreshModelsFunc(ctx)
	close(call.done)

	p.refreshMu.Lock()
	if p.inflight == call {
		p.inflight = nil
	}
	p.refreshMu.Unlock()
	return call.err
}

// GenerateImages dispatches one request to the provider's image transport.
func (p *ImagesProvider) GenerateImages(
	model ImagesModel,
	imagesContext ImagesContext,
	options ImagesOptions,
) (AssistantImages, error) {
	if p == nil || p.GenerateImagesFunc == nil {
		return AssistantImages{}, errors.New("provider does not implement image generation")
	}
	return p.GenerateImagesFunc(
		cloneImagesModel(model),
		cloneImagesContext(imagesContext),
		cloneImagesOptions(options),
	)
}

func (s *createdImagesProviderState) getModels() ([]ImagesModel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneImagesModels(s.models), nil
}

func (s *createdImagesProviderState) refresh(ctx context.Context) error {
	models, err := s.fetch(contextOrBackground(ctx))
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.models = cloneImagesModels(models)
	s.mu.Unlock()
	return nil
}

// ImagesModels is an instance-scoped image-provider collection. It owns
// registration and request assembly while transports remain stateless.
type ImagesModels struct {
	mu        sync.RWMutex
	providers map[string]*ImagesProvider
	order     []string

	credentials CredentialStore
	authContext AuthContext
}

// NewImagesModels creates an isolated image-provider collection.
func NewImagesModels(options ...ImagesModelsOptions) *ImagesModels {
	var selected ImagesModelsOptions
	if len(options) > 0 {
		selected = options[0]
	}
	if selected.Credentials == nil {
		selected.Credentials = NewInMemoryCredentialStore()
	}
	if selected.AuthContext == nil {
		selected.AuthContext = DefaultProviderAuthContext()
	}
	return &ImagesModels{
		providers:   map[string]*ImagesProvider{},
		credentials: selected.Credentials,
		authContext: selected.AuthContext,
	}
}

// SetProvider inserts or replaces a provider by ID without changing its
// existing registration position.
func (m *ImagesModels) SetProvider(provider *ImagesProvider) error {
	if m == nil {
		return errors.New("image models collection is nil")
	}
	if provider == nil || strings.TrimSpace(provider.ID) == "" {
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

// DeleteProvider removes one provider without affecting credentials.
func (m *ImagesModels) DeleteProvider(providerID string) {
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

// ClearProviders removes every provider without affecting credentials.
func (m *ImagesModels) ClearProviders() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.providers = map[string]*ImagesProvider{}
	m.order = nil
	m.mu.Unlock()
}

// GetProviders returns providers in registration order.
func (m *ImagesModels) GetProviders() []*ImagesProvider {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	providers := make([]*ImagesProvider, 0, len(m.order))
	for _, providerID := range m.order {
		if provider := m.providers[providerID]; provider != nil {
			providers = append(providers, provider)
		}
	}
	m.mu.RUnlock()
	return providers
}

// GetProvider finds one registered provider.
func (m *ImagesModels) GetProvider(providerID string) (*ImagesProvider, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.RLock()
	provider, ok := m.providers[providerID]
	m.mu.RUnlock()
	return provider, ok
}

// GetModels returns last-known models for one provider or all providers.
// Provider source failures are isolated and yield no models.
func (m *ImagesModels) GetModels(providerID ...string) []ImagesModel {
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
	var models []ImagesModel
	for _, provider := range m.GetProviders() {
		current, err := provider.GetModels()
		if err == nil {
			models = append(models, current...)
		}
	}
	return models
}

// GetModel finds one model in a provider's last-known catalog.
func (m *ImagesModels) GetModel(providerID, modelID string) (ImagesModel, bool) {
	for _, model := range m.GetModels(providerID) {
		if model.ID == modelID {
			return model, true
		}
	}
	return ImagesModel{}, false
}

// Refresh updates one dynamic provider or all dynamic providers. A targeted
// failure is typed; an all-provider refresh is concurrent and best-effort.
func (m *ImagesModels) Refresh(ctx context.Context, providerID ...string) error {
	ctx = contextOrBackground(ctx)
	if len(providerID) > 0 {
		provider, ok := m.GetProvider(providerID[0])
		if !ok || provider.RefreshModelsFunc == nil {
			return nil
		}
		if err := provider.RefreshModels(ctx); err != nil {
			var modelsError *ModelsError
			if errors.As(err, &modelsError) {
				return err
			}
			return newModelsError(
				ModelsErrorModelSource,
				fmt.Sprintf("Model refresh failed for %s", providerID[0]),
				err,
			)
		}
		return nil
	}

	var wait sync.WaitGroup
	for _, provider := range m.GetProviders() {
		if provider == nil || provider.RefreshModelsFunc == nil {
			continue
		}
		wait.Add(1)
		go func(provider *ImagesProvider) {
			defer wait.Done()
			_ = provider.RefreshModels(ctx)
		}(provider)
	}
	wait.Wait()
	return nil
}

// GetAuth resolves request authentication for one image provider.
func (m *ImagesModels) GetAuth(
	ctx context.Context,
	providerID string,
	overrides AuthResolutionOverrides,
) (*AuthResult, error) {
	provider, ok := m.GetProvider(providerID)
	if !ok {
		return nil, nil
	}
	return ResolveProviderAuth(
		contextOrBackground(ctx),
		provider.ID,
		provider.Auth,
		m.credentials,
		m.authContext,
		overrides,
	)
}

// GetModelAuth resolves request authentication for an image model.
func (m *ImagesModels) GetModelAuth(
	ctx context.Context,
	model ImagesModel,
	overrides AuthResolutionOverrides,
) (*AuthResult, error) {
	return m.GetAuth(ctx, model.Provider, overrides)
}

// GenerateImages resolves provider auth, assembles a detached request, and
// returns failures as terminal image results rather than Go errors.
func (m *ImagesModels) GenerateImages(
	ctx context.Context,
	model ImagesModel,
	imagesContext ImagesContext,
	options ImagesOptions,
) AssistantImages {
	ctx = contextOrBackground(ctx)
	if err := ctx.Err(); err != nil {
		return AbortedImages(model, err)
	}
	provider, ok := m.GetProvider(model.Provider)
	if !ok {
		return ErrorImages(model, newModelsError(
			ModelsErrorProvider,
			fmt.Sprintf("Unknown provider: %s", model.Provider),
			nil,
		))
	}

	requestOptions := cloneImagesOptions(options)
	requestOptions.Context = ctx
	apiKeyOverride := requestOptions.APIKeyOverride
	if apiKeyOverride == nil && requestOptions.APIKey != "" {
		apiKey := requestOptions.APIKey
		apiKeyOverride = &apiKey
	}
	resolution, err := m.GetModelAuth(ctx, model, AuthResolutionOverrides{
		APIKey: apiKeyOverride,
		Env:    requestOptions.Env,
	})
	if err != nil {
		if ctx.Err() != nil {
			return AbortedImages(model, ctx.Err())
		}
		return ErrorImages(model, err)
	}

	requestModel := cloneImagesModel(model)
	if resolution != nil {
		if resolution.Auth.BaseURL != "" {
			requestModel.BaseURL = resolution.Auth.BaseURL
		}
		requestOptions.APIKey = resolution.Auth.APIKey
		if apiKeyOverride != nil {
			requestOptions.APIKey = *apiKeyOverride
		}
		requestOptions.Headers = mergeHeadersCaseInsensitive(
			resolution.Auth.Headers,
			options.Headers,
		)
		requestOptions.HeaderRemovals = clearOverriddenHeaderRemovals(
			resolution.Auth.HeaderRemovals,
			options.Headers,
		)
		requestOptions.HeaderRemovals = appendUniqueHeaderRemovals(
			requestOptions.HeaderRemovals,
			options.HeaderRemovals,
		)
		requestOptions.Env = mergeProviderEnv(resolution.Env, options.Env)
	}
	// Mark the collection-assembled value as authoritative, including an
	// unconfigured empty value, so legacy transports cannot bypass the
	// injected AuthContext by consulting process-global credentials.
	effectiveAPIKey := requestOptions.APIKey
	requestOptions.APIKeyOverride = &effectiveAPIKey

	result, err := provider.GenerateImages(
		requestModel,
		imagesContext,
		requestOptions,
	)
	if err != nil {
		if ctx.Err() != nil {
			return AbortedImages(model, ctx.Err())
		}
		return ErrorImages(model, err)
	}
	return result
}

func cloneImagesModels(models []ImagesModel) []ImagesModel {
	if models == nil {
		return nil
	}
	cloned := make([]ImagesModel, len(models))
	for index, model := range models {
		cloned[index] = cloneImagesModel(model)
	}
	return cloned
}

func cloneImagesModel(model ImagesModel) ImagesModel {
	model.Input = append([]string(nil), model.Input...)
	model.Output = append([]string(nil), model.Output...)
	model.Cost.Tiers = append([]ModelCostTier(nil), model.Cost.Tiers...)
	model.Headers = cloneStringMap(model.Headers)
	return model
}

func cloneImagesContext(imagesContext ImagesContext) ImagesContext {
	imagesContext.Input = append([]ImagesContent(nil), imagesContext.Input...)
	return imagesContext
}

func cloneImagesOptions(options ImagesOptions) ImagesOptions {
	if options.APIKeyOverride != nil {
		apiKey := *options.APIKeyOverride
		options.APIKeyOverride = &apiKey
	}
	options.Headers = cloneStringMap(options.Headers)
	options.HeaderRemovals = append([]string(nil), options.HeaderRemovals...)
	options.Env = cloneProviderEnv(options.Env)
	options.Metadata = cloneCredentialMetadata(options.Metadata)
	return options
}
