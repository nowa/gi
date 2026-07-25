package gillmprovider

import (
	"context"
	"sync"
)

// ModelsStoreEntry is one provider's persisted dynamic model catalog.
type ModelsStoreEntry struct {
	Models       []Model `json:"models"`
	LastModified *int64  `json:"lastModified,omitempty"`
	CheckedAt    int64   `json:"checkedAt,omitempty"`
}

// ModelsStore persists dynamic model catalogs by provider ID.
type ModelsStore interface {
	ReadModels(ctx context.Context, providerID string) (ModelsStoreEntry, bool, error)
	WriteModels(ctx context.Context, providerID string, entry ModelsStoreEntry) error
	DeleteModels(ctx context.Context, providerID string) error
}

// ProviderModelsStore scopes model persistence to one provider so provider
// implementations cannot inspect or mutate another provider's catalog.
type ProviderModelsStore interface {
	ReadModels(ctx context.Context) (ModelsStoreEntry, bool, error)
	WriteModels(ctx context.Context, entry ModelsStoreEntry) error
	DeleteModels(ctx context.Context) error
}

// InMemoryModelsStore is a concurrent, clone-on-read/write ModelsStore.
type InMemoryModelsStore struct {
	mu      sync.RWMutex
	entries map[string]ModelsStoreEntry
}

// NewInMemoryModelsStore builds a model store from an optional snapshot.
func NewInMemoryModelsStore(initial ...map[string]ModelsStoreEntry) *InMemoryModelsStore {
	store := &InMemoryModelsStore{entries: map[string]ModelsStoreEntry{}}
	if len(initial) > 0 {
		for providerID, entry := range initial[0] {
			store.entries[providerID] = cloneModelsStoreEntry(entry)
		}
	}
	return store
}

func (s *InMemoryModelsStore) ReadModels(
	ctx context.Context,
	providerID string,
) (ModelsStoreEntry, bool, error) {
	if err := contextError(ctx); err != nil {
		return ModelsStoreEntry{}, false, err
	}
	s.mu.RLock()
	entry, ok := s.entries[providerID]
	s.mu.RUnlock()
	if !ok {
		return ModelsStoreEntry{}, false, nil
	}
	return cloneModelsStoreEntry(entry), true, nil
}

func (s *InMemoryModelsStore) WriteModels(
	ctx context.Context,
	providerID string,
	entry ModelsStoreEntry,
) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	s.entries[providerID] = cloneModelsStoreEntry(entry)
	s.mu.Unlock()
	return nil
}

func (s *InMemoryModelsStore) DeleteModels(ctx context.Context, providerID string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.entries, providerID)
	s.mu.Unlock()
	return nil
}

type scopedModelsStore struct {
	store      ModelsStore
	providerID string
}

func (s scopedModelsStore) ReadModels(ctx context.Context) (ModelsStoreEntry, bool, error) {
	return s.store.ReadModels(ctx, s.providerID)
}

func (s scopedModelsStore) WriteModels(ctx context.Context, entry ModelsStoreEntry) error {
	return s.store.WriteModels(ctx, s.providerID, entry)
}

func (s scopedModelsStore) DeleteModels(ctx context.Context) error {
	return s.store.DeleteModels(ctx, s.providerID)
}

func cloneModelsStoreEntry(entry ModelsStoreEntry) ModelsStoreEntry {
	entry.Models = cloneModels(entry.Models)
	if entry.LastModified != nil {
		lastModified := *entry.LastModified
		entry.LastModified = &lastModified
	}
	return entry
}

func cloneModels(models []Model) []Model {
	if models == nil {
		return nil
	}
	cloned := make([]Model, len(models))
	for index, model := range models {
		cloned[index] = cloneModel(model)
	}
	return cloned
}

func cloneModel(model Model) Model {
	cloned := model
	cloned.Headers = cloneStringMap(model.Headers)
	cloned.Input = append([]string(nil), model.Input...)
	cloned.Cost.Tiers = append([]ModelCostTier(nil), model.Cost.Tiers...)
	if model.ThinkingLevelMap != nil {
		cloned.ThinkingLevelMap = make(map[string]*string, len(model.ThinkingLevelMap))
		for level, mapped := range model.ThinkingLevelMap {
			if mapped == nil {
				cloned.ThinkingLevelMap[level] = nil
				continue
			}
			value := *mapped
			cloned.ThinkingLevelMap[level] = &value
		}
	}
	cloned.Compat.OpenRouterRouting = cloneCredentialMetadata(model.Compat.OpenRouterRouting)
	cloned.Compat.VercelGatewayRouting = cloneCredentialMetadata(model.Compat.VercelGatewayRouting)
	cloned.Compat.ChatTemplateKwargs = cloneCredentialMetadata(model.Compat.ChatTemplateKwargs)
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
