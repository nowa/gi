package gillmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
)

// CredentialType identifies the persisted credential variant.
type CredentialType string

const (
	// CredentialTypeAPIKey stores an API key or a resolvable key expression.
	CredentialTypeAPIKey CredentialType = "api_key"
	// CredentialTypeOAuth stores a renewable OAuth token set.
	CredentialTypeOAuth CredentialType = "oauth"
)

// ProviderEnv contains provider-scoped configuration resolved alongside a
// credential, such as Cloudflare account and gateway identifiers.
type ProviderEnv map[string]string

// Credential is the canonical persisted auth record. Metadata preserves
// provider-specific OAuth fields without weakening the typed common fields.
type Credential struct {
	Type          CredentialType `json:"type"`
	Key           string         `json:"key,omitempty"`
	Access        string         `json:"access,omitempty"`
	Refresh       string         `json:"refresh,omitempty"`
	Expires       int64          `json:"expires,omitempty"`
	Env           ProviderEnv    `json:"env,omitempty"`
	EnterpriseURL string         `json:"enterpriseUrl,omitempty"`
	Metadata      map[string]any `json:"-"`
}

var credentialJSONFields = map[string]struct{}{
	"type":          {},
	"key":           {},
	"access":        {},
	"refresh":       {},
	"expires":       {},
	"env":           {},
	"enterpriseUrl": {},
}

func (c Credential) MarshalJSON() ([]byte, error) {
	type credentialJSON Credential
	known, err := json.Marshal(credentialJSON(c))
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(known, &object); err != nil {
		return nil, err
	}
	for key, value := range c.Metadata {
		if _, reserved := credentialJSONFields[key]; reserved {
			continue
		}
		object[key] = value
	}
	return json.Marshal(object)
}

func (c *Credential) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("cannot unmarshal credential into nil receiver")
	}
	type credentialJSON Credential
	var known credentialJSON
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	*c = Credential(known)
	for key := range credentialJSONFields {
		delete(object, key)
	}
	if len(object) == 0 {
		c.Metadata = nil
		return nil
	}
	c.Metadata = make(map[string]any, len(object))
	for key, raw := range object {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		c.Metadata[key] = value
	}
	return nil
}

func (c Credential) Clone() Credential {
	cloned := c
	cloned.Env = cloneProviderEnv(c.Env)
	cloned.Metadata = cloneCredentialMetadata(c.Metadata)
	return cloned
}

// CredentialInfo is the non-secret projection returned by credential
// enumeration.
type CredentialInfo struct {
	ProviderID string         `json:"providerId"`
	Type       CredentialType `json:"type"`
}

// CredentialModifier returns write=false to preserve the current credential.
// A successful write always replaces the provider's complete credential. Once
// a modifier returns write=true without an error, the store commits the value
// even if ctx was canceled during the modifier; this prevents losing a
// successfully rotated OAuth refresh token.
type CredentialModifier func(ctx context.Context, current Credential, exists bool) (next Credential, write bool, err error)

// CredentialStore is the app-owned persistence boundary. ModifyCredential is
// the only write path so OAuth refresh and login cannot overwrite each other.
type CredentialStore interface {
	ReadCredential(ctx context.Context, providerID string) (Credential, bool, error)
	ListCredentials(ctx context.Context) ([]CredentialInfo, error)
	ModifyCredential(ctx context.Context, providerID string, modify CredentialModifier) (Credential, bool, error)
	DeleteCredential(ctx context.Context, providerID string) error
}

type credentialGate struct {
	token chan struct{}
}

func newCredentialGate() *credentialGate {
	gate := &credentialGate{token: make(chan struct{}, 1)}
	gate.token <- struct{}{}
	return gate
}

func (g *credentialGate) lock(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-g.token:
		return nil
	}
}

func (g *credentialGate) unlock() {
	g.token <- struct{}{}
}

// InMemoryCredentialStore is deterministic and safe for concurrent use.
// Mutations serialize per provider while unrelated providers can progress.
type InMemoryCredentialStore struct {
	mu          sync.RWMutex
	credentials map[string]Credential
	gatesMu     sync.Mutex
	gates       map[string]*credentialGate
}

// NewInMemoryCredentialStore builds a store from an optional initial snapshot.
func NewInMemoryCredentialStore(initial ...map[string]Credential) *InMemoryCredentialStore {
	store := &InMemoryCredentialStore{
		credentials: map[string]Credential{},
		gates:       map[string]*credentialGate{},
	}
	if len(initial) > 0 {
		for providerID, credential := range initial[0] {
			store.credentials[providerID] = credential.Clone()
		}
	}
	return store
}

func (s *InMemoryCredentialStore) ReadCredential(ctx context.Context, providerID string) (Credential, bool, error) {
	if err := contextError(ctx); err != nil {
		return Credential{}, false, err
	}
	s.mu.RLock()
	credential, ok := s.credentials[providerID]
	s.mu.RUnlock()
	if !ok {
		return Credential{}, false, nil
	}
	return credential.Clone(), true, nil
}

func (s *InMemoryCredentialStore) ListCredentials(ctx context.Context) ([]CredentialInfo, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	result := make([]CredentialInfo, 0, len(s.credentials))
	for providerID, credential := range s.credentials {
		result = append(result, CredentialInfo{ProviderID: providerID, Type: credential.Type})
	}
	s.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool {
		return result[i].ProviderID < result[j].ProviderID
	})
	return result, nil
}

func (s *InMemoryCredentialStore) ModifyCredential(
	ctx context.Context,
	providerID string,
	modify CredentialModifier,
) (Credential, bool, error) {
	if modify == nil {
		return Credential{}, false, errors.New("credential modifier is required")
	}
	ctx = contextOrBackground(ctx)
	gate := s.providerGate(providerID)
	if err := gate.lock(ctx); err != nil {
		return Credential{}, false, err
	}
	defer gate.unlock()
	if err := ctx.Err(); err != nil {
		return Credential{}, false, err
	}

	s.mu.RLock()
	current, exists := s.credentials[providerID]
	s.mu.RUnlock()
	current = current.Clone()
	next, write, err := modify(ctx, current.Clone(), exists)
	if err != nil {
		return Credential{}, false, err
	}
	if !write {
		return current.Clone(), exists, nil
	}
	next = next.Clone()
	s.mu.Lock()
	s.credentials[providerID] = next
	s.mu.Unlock()
	return next.Clone(), true, nil
}

func (s *InMemoryCredentialStore) DeleteCredential(ctx context.Context, providerID string) error {
	ctx = contextOrBackground(ctx)
	gate := s.providerGate(providerID)
	if err := gate.lock(ctx); err != nil {
		return err
	}
	defer gate.unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.credentials, providerID)
	s.mu.Unlock()
	return nil
}

func (s *InMemoryCredentialStore) providerGate(providerID string) *credentialGate {
	s.gatesMu.Lock()
	defer s.gatesMu.Unlock()
	gate := s.gates[providerID]
	if gate == nil {
		gate = newCredentialGate()
		s.gates[providerID] = gate
	}
	return gate
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func cloneProviderEnv(env ProviderEnv) ProviderEnv {
	if env == nil {
		return nil
	}
	cloned := make(ProviderEnv, len(env))
	for key, value := range env {
		cloned[key] = value
	}
	return cloned
}

func cloneCredentialMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = cloneCredentialMetadataValue(value)
	}
	return cloned
}

func cloneCredentialMetadataValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneCredentialMetadata(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneCredentialMetadataValue(item)
		}
		return cloned
	case ProviderEnv:
		return cloneProviderEnv(typed)
	case map[string]string:
		cloned := make(map[string]string, len(typed))
		for key, item := range typed {
			cloned[key] = item
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	case []byte:
		return append([]byte(nil), typed...)
	case json.RawMessage:
		return append(json.RawMessage(nil), typed...)
	default:
		return value
	}
}
