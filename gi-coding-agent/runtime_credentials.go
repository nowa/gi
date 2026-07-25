package gicodingagent

import (
	"context"
	"sort"
	"sync"

	llm "github.com/nowa/gi/gi-llm-provider"
)

// runtimeCredentialOverlay adds process-local API keys to an app-owned
// CredentialStore. Persistent mutations always delegate to the underlying
// store; deleting a provider clears both layers.
type runtimeCredentialOverlay struct {
	base llm.CredentialStore

	mu        sync.RWMutex
	overrides map[string]string
}

func newRuntimeCredentialOverlay(
	base llm.CredentialStore,
) *runtimeCredentialOverlay {
	if base == nil {
		base = llm.NewInMemoryCredentialStore()
	}
	return &runtimeCredentialOverlay{
		base:      base,
		overrides: map[string]string{},
	}
}

func (s *runtimeCredentialOverlay) SetRuntimeAPIKey(
	providerID string,
	apiKey string,
) {
	s.mu.Lock()
	s.overrides[providerID] = apiKey
	s.mu.Unlock()
}

func (s *runtimeCredentialOverlay) RemoveRuntimeAPIKey(
	providerID string,
) {
	s.mu.Lock()
	delete(s.overrides, providerID)
	s.mu.Unlock()
}

func (s *runtimeCredentialOverlay) HasRuntimeAPIKey(
	providerID string,
) bool {
	s.mu.RLock()
	_, ok := s.overrides[providerID]
	s.mu.RUnlock()
	return ok
}

func (s *runtimeCredentialOverlay) ReadCredential(
	ctx context.Context,
	providerID string,
) (llm.Credential, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return llm.Credential{}, false, err
	}
	s.mu.RLock()
	apiKey, ok := s.overrides[providerID]
	s.mu.RUnlock()
	if ok && apiKey != "" {
		return llm.Credential{
			Type: llm.CredentialTypeAPIKey,
			Key:  apiKey,
		}, true, nil
	}
	return s.base.ReadCredential(ctx, providerID)
}

func (s *runtimeCredentialOverlay) ListCredentials(
	ctx context.Context,
) ([]llm.CredentialInfo, error) {
	stored, err := s.base.ListCredentials(ctx)
	if err != nil {
		return nil, err
	}
	entries := make(
		map[string]llm.CredentialInfo,
		len(stored),
	)
	for _, entry := range stored {
		entries[entry.ProviderID] = entry
	}
	s.mu.RLock()
	for providerID := range s.overrides {
		entries[providerID] = llm.CredentialInfo{
			ProviderID: providerID,
			Type:       llm.CredentialTypeAPIKey,
		}
	}
	s.mu.RUnlock()
	result := make([]llm.CredentialInfo, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ProviderID < result[j].ProviderID
	})
	return result, nil
}

func (s *runtimeCredentialOverlay) ModifyCredential(
	ctx context.Context,
	providerID string,
	modify llm.CredentialModifier,
) (llm.Credential, bool, error) {
	return s.base.ModifyCredential(ctx, providerID, modify)
}

func (s *runtimeCredentialOverlay) DeleteCredential(
	ctx context.Context,
	providerID string,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.RemoveRuntimeAPIKey(providerID)
	return s.base.DeleteCredential(ctx, providerID)
}

var _ llm.CredentialStore = (*runtimeCredentialOverlay)(nil)
