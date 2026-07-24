package gillmprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// RefreshModelsContext contains the provider-scoped state needed to restore or
// fetch a dynamic model catalog.
type RefreshModelsContext struct {
	Credential   *Credential
	Store        ProviderModelsStore
	AllowNetwork bool
	Force        bool
}

// Provider is the concrete runtime unit owned by a Models collection.
// Callbacks are immutable after registration; dynamic model state belongs to
// the callback implementation or a Provider created with CreateProvider.
type Provider struct {
	ID      string
	Name    string
	BaseURL string
	Headers map[string]string
	Auth    ProviderAuth

	ModelSource       func() ([]Model, error)
	RefreshModelsFunc func(ctx context.Context, input RefreshModelsContext) error
	FilterModelsFunc  func(models []Model, credential *Credential) []Model
	StreamFunc        func(model Model, llmContext Context, options StreamOptions) (*AssistantMessageEventStream, error)
	StreamSimpleFunc  func(model Model, llmContext Context, options SimpleStreamOptions) (*AssistantMessageEventStream, error)

	refreshMu sync.Mutex
	inflight  *providerRefreshCall
}

type providerRefreshCall struct {
	done chan struct{}
	err  error
}

// CreateProviderOptions describes a static catalog with an optional dynamic
// overlay and either one stream implementation or API-specific transports.
type CreateProviderOptions struct {
	ID      string
	Name    string
	BaseURL string
	Headers map[string]string
	Auth    ProviderAuth
	Models  []Model

	FetchModels  func(ctx context.Context, input RefreshModelsContext) ([]Model, error)
	FilterModels func(models []Model, credential *Credential) []Model
	API          APIProvider
	APIs         map[string]APIProvider
}

// CreateProvider constructs a provider with clone-on-read model state,
// provider-scoped persistence, API dispatch, and coalesced refreshes.
func CreateProvider(options CreateProviderOptions) (*Provider, error) {
	if strings.TrimSpace(options.ID) == "" {
		return nil, errors.New("provider ID is required")
	}
	if options.Auth.APIKey == nil && options.Auth.OAuth == nil {
		return nil, fmt.Errorf("provider %s has no authentication method", options.ID)
	}
	if options.API == nil && len(options.APIs) == 0 {
		return nil, fmt.Errorf("provider %s has no API implementation", options.ID)
	}

	state := &createdProviderState{
		providerID: options.ID,
		baseline:   cloneModels(options.Models),
		fetch:      options.FetchModels,
	}
	provider := &Provider{
		ID:               options.ID,
		Name:             options.Name,
		BaseURL:          options.BaseURL,
		Headers:          cloneStringMap(options.Headers),
		Auth:             options.Auth,
		ModelSource:      state.models,
		FilterModelsFunc: options.FilterModels,
	}
	if provider.Name == "" {
		provider.Name = provider.ID
	}
	if options.FetchModels != nil {
		provider.RefreshModelsFunc = state.refresh
	}

	apiByName := make(map[string]APIProvider, len(options.APIs))
	for api, implementation := range options.APIs {
		apiByName[api] = implementation
	}
	streamsFor := func(model Model) (APIProvider, error) {
		implementation := options.API
		if implementation == nil {
			implementation = apiByName[model.API]
		}
		if implementation == nil {
			return nil, &ModelsError{
				Code: ModelsErrorStream,
				Msg:  fmt.Sprintf("Provider %s has no API implementation for %q", options.ID, model.API),
			}
		}
		return implementation, nil
	}
	provider.StreamFunc = func(
		model Model,
		llmContext Context,
		streamOptions StreamOptions,
	) (*AssistantMessageEventStream, error) {
		implementation, err := streamsFor(model)
		if err != nil {
			return nil, err
		}
		return implementation.Stream(model, llmContext, streamOptions)
	}
	provider.StreamSimpleFunc = func(
		model Model,
		llmContext Context,
		streamOptions SimpleStreamOptions,
	) (*AssistantMessageEventStream, error) {
		implementation, err := streamsFor(model)
		if err != nil {
			return nil, err
		}
		return implementation.StreamSimple(model, llmContext, streamOptions)
	}
	return provider, nil
}

// GetModels returns a detached snapshot of the provider's current catalog.
func (p *Provider) GetModels() ([]Model, error) {
	if p == nil || p.ModelSource == nil {
		return nil, nil
	}
	models, err := p.ModelSource()
	if err != nil {
		return nil, err
	}
	return cloneModels(models), nil
}

// RefreshModels restores or updates the provider's dynamic catalog. Concurrent
// calls share one in-flight refresh.
func (p *Provider) RefreshModels(ctx context.Context, input RefreshModelsContext) error {
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

	call.err = p.RefreshModelsFunc(ctx, cloneRefreshModelsContext(input))
	close(call.done)

	p.refreshMu.Lock()
	if p.inflight == call {
		p.inflight = nil
	}
	p.refreshMu.Unlock()
	return call.err
}

// FilterModels applies provider policy to a detached model snapshot.
func (p *Provider) FilterModels(models []Model, credential *Credential) []Model {
	models = cloneModels(models)
	if p == nil || p.FilterModelsFunc == nil {
		return models
	}
	return cloneModels(p.FilterModelsFunc(models, cloneCredentialPointer(credential)))
}

// Stream dispatches a request through this provider.
func (p *Provider) Stream(
	model Model,
	llmContext Context,
	options StreamOptions,
) (*AssistantMessageEventStream, error) {
	if p == nil || p.StreamFunc == nil {
		return nil, errors.New("provider does not implement stream")
	}
	return p.StreamFunc(cloneModel(model), llmContext, options)
}

// StreamSimple dispatches a request through the provider's simple stream
// implementation, falling back to StreamFunc when necessary.
func (p *Provider) StreamSimple(
	model Model,
	llmContext Context,
	options SimpleStreamOptions,
) (*AssistantMessageEventStream, error) {
	if p == nil {
		return nil, errors.New("provider is required")
	}
	if p.StreamSimpleFunc != nil {
		return p.StreamSimpleFunc(cloneModel(model), llmContext, options)
	}
	if p.StreamFunc != nil {
		return p.StreamFunc(cloneModel(model), llmContext, options)
	}
	return nil, errors.New("provider does not implement stream")
}

type createdProviderState struct {
	providerID string
	baseline   []Model
	fetch      func(ctx context.Context, input RefreshModelsContext) ([]Model, error)

	mu      sync.RWMutex
	dynamic []Model
}

func (s *createdProviderState) models() ([]Model, error) {
	s.mu.RLock()
	dynamic := cloneModels(s.dynamic)
	s.mu.RUnlock()
	merged := cloneModels(s.baseline)
	positions := make(map[string]int, len(merged))
	for index, model := range merged {
		positions[model.ID] = index
	}
	for _, model := range dynamic {
		if index, ok := positions[model.ID]; ok {
			merged[index] = cloneModel(model)
			continue
		}
		positions[model.ID] = len(merged)
		merged = append(merged, cloneModel(model))
	}
	return merged, nil
}

func (s *createdProviderState) refresh(ctx context.Context, input RefreshModelsContext) error {
	if input.Store == nil {
		return errors.New("provider model store is required")
	}
	stored, exists, err := input.Store.ReadModels(ctx)
	if err != nil {
		return err
	}
	if exists {
		restored := make([]Model, 0, len(stored.Models))
		for _, model := range stored.Models {
			if model.Provider == s.providerID {
				restored = append(restored, cloneModel(model))
			}
		}
		s.mu.Lock()
		s.dynamic = restored
		s.mu.Unlock()
	}
	if !input.AllowNetwork || contextError(ctx) != nil {
		return contextError(ctx)
	}

	refreshed, err := s.fetch(ctx, cloneRefreshModelsContext(input))
	if err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	refreshed = cloneModels(refreshed)
	s.mu.Lock()
	s.dynamic = refreshed
	s.mu.Unlock()
	return input.Store.WriteModels(ctx, ModelsStoreEntry{
		Models:    refreshed,
		CheckedAt: time.Now().UnixMilli(),
	})
}

func cloneRefreshModelsContext(input RefreshModelsContext) RefreshModelsContext {
	input.Credential = cloneCredentialPointer(input.Credential)
	return input
}

func cloneCredentialPointer(credential *Credential) *Credential {
	if credential == nil {
		return nil
	}
	cloned := credential.Clone()
	return &cloned
}
