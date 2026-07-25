package gicodingagent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	llm "github.com/nowa/gi/gi-llm-provider"
)

// ModelRuntimeOptions supplies the compatibility configuration facade and
// credential boundary used to compose an instance-scoped provider runtime.
// Supplying Registry ignores ModelRegistryOptions; an explicit Credentials
// store still replaces the registry's default AuthStorage for canonical
// runtime operations.
type ModelRuntimeOptions struct {
	Registry    *ModelRegistry
	Credentials llm.CredentialStore
	ModelRegistryOptions
}

type modelRuntimeSnapshot struct {
	all                 []llm.Model
	available           []llm.Model
	configuredProviders map[string]struct{}
	storedProviders     map[string]struct{}
	auth                map[string]llm.AuthCheck
}

type modelRuntimeAvailabilityCall struct {
	done chan struct{}
	err  error
}

type modelRuntimeRefreshCall struct {
	done   chan struct{}
	result llm.ModelsRefreshResult
	err    error
}

// ModelRuntime is the coding-agent ownership boundary for model discovery,
// provider composition, credentials, and request dispatch.
//
// Mutations are serialized under mu and publish immutable snapshots. A
// prepared request captures one provider, model, auth result, and option set
// before invoking extension transforms or transport code, so an in-flight
// request never observes a half-applied provider reload.
type ModelRuntime struct {
	mu sync.RWMutex

	registry            *ModelRegistry
	models              *llm.Models
	credentials         *runtimeCredentialOverlay
	registryCredentials bool
	builtinProviders    map[string]*llm.Provider
	builtinsReady       bool
	nativeProviders     map[string]*llm.Provider
	nativeOrder         []string
	compositionErrors   map[string]string
	availabilityError   string

	snapshot atomic.Pointer[modelRuntimeSnapshot]

	availabilityMu   sync.Mutex
	availabilityCall *modelRuntimeAvailabilityCall

	refreshMu   sync.Mutex
	refreshCall *modelRuntimeRefreshCall
}

// NewModelRuntime constructs a runtime from either an existing compatibility
// registry or the supplied registry options.
func NewModelRuntime(
	ctx context.Context,
	options ModelRuntimeOptions,
) (*ModelRuntime, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	registry := options.Registry
	if registry == nil {
		registryOptions := options.ModelRegistryOptions
		allowModelNetwork := registryOptions.AllowModelNetwork
		registryOptions.AllowModelNetwork = false
		registry = NewModelRegistryWithOptions(
			ctx,
			registryOptions,
		)
		registry.initialModelNetwork = allowModelNetwork &&
			registry.modelNetworkEnabled
	}
	credentialStore := options.Credentials
	registryCredentials := credentialStore == nil &&
		registry.authStorage != nil
	if credentialStore == nil && registry.authStorage != nil {
		credentialStore = registry.authStorage
	}
	runtime := &ModelRuntime{
		registry:            registry,
		credentials:         newRuntimeCredentialOverlay(credentialStore),
		registryCredentials: registryCredentials,
		builtinProviders:    map[string]*llm.Provider{},
		nativeProviders:     map[string]*llm.Provider{},
		compositionErrors:   map[string]string{},
	}
	runtime.mu.Lock()
	err := runtime.ensureBuiltinProvidersLocked()
	runtime.mu.Unlock()
	if err != nil {
		return nil, err
	}
	registry.mu.Lock()
	if registry.modelRuntime != nil {
		registry.mu.Unlock()
		return nil, errors.New(
			"model registry is already bound to a model runtime",
		)
	}
	registry.releaseLegacyProviderOverrides()
	registry.modelRuntime = runtime
	registry.mu.Unlock()
	runtime.mu.Lock()
	err = runtime.rebuildProvidersLocked()
	runtime.mu.Unlock()
	if err != nil {
		return nil, err
	}
	refreshCtx, cancel := modelRegistryRefreshContext(
		ctx,
		registry.initialModelNetwork,
		registry.modelRefreshTimeout,
	)
	runtime.mu.RLock()
	models := runtime.models
	runtime.mu.RUnlock()
	if models != nil {
		_ = models.Refresh(
			refreshCtx,
			llm.ModelsRefreshOptions{
				Offline: !registry.initialModelNetwork,
			},
		)
		runtime.mu.Lock()
		if runtime.models == models {
			runtime.updateModelSnapshotLocked()
		}
		runtime.mu.Unlock()
		registry.publishRuntimeModels(models.GetModels())
	}
	cancel()
	// Availability failures are status, not construction failures. The model
	// catalog remains useful for explicit model selection and diagnostics.
	_ = runtime.RefreshAvailability(ctx)
	return runtime, nil
}

// NewModelRuntimeFromRegistry promotes a loaded compatibility registry into
// the canonical instance request runtime.
func NewModelRuntimeFromRegistry(
	registry *ModelRegistry,
) (*ModelRuntime, error) {
	if registry == nil {
		return nil, errors.New("model registry is required")
	}
	return NewModelRuntime(context.Background(), ModelRuntimeOptions{
		Registry: registry,
	})
}

// ModelRegistry returns the legacy synchronous facade backed by this runtime.
// New request paths should use ModelRuntime directly.
func (r *ModelRuntime) ModelRegistry() *ModelRegistry {
	if r == nil {
		return nil
	}
	return r.registry
}

func (r *ModelRuntime) providerIDsLocked() []string {
	seen := map[string]struct{}{}
	var ids []string
	appendID := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if r.registry != nil && r.registry.dynamicModels != nil {
		for _, provider := range r.registry.dynamicModels.GetProviders() {
			if provider != nil {
				appendID(provider.ID)
			}
		}
	}
	for _, providerID := range llm.GetBuiltinProviderIDs() {
		appendID(providerID)
	}
	if r.registry != nil {
		for _, id := range r.registry.compositionProviderIDs() {
			appendID(id)
		}
	}
	for _, id := range r.nativeOrder {
		appendID(id)
	}
	return ids
}

func (r *ModelRuntime) baseProviderLocked(
	providerID string,
) *llm.Provider {
	if provider := r.nativeProviders[providerID]; provider != nil {
		return provider
	}
	if r.registry != nil && r.registry.dynamicModels != nil {
		if provider, ok := r.registry.dynamicModels.GetProvider(providerID); ok {
			return provider
		}
	}
	return r.builtinProviders[providerID]
}

func (r *ModelRuntime) recomposeProviderLocked(
	collection *llm.Models,
	providerID string,
) error {
	base := r.baseProviderLocked(providerID)
	modelsJSON, hasModelsJSON, extension, hasExtension :=
		r.registry.providerCompositionSnapshot(providerID)
	if base == nil && !hasModelsJSON && !hasExtension {
		return nil
	}
	provider, err := composeModelProvider(
		modelProviderComposition{
			providerID:    providerID,
			base:          base,
			modelsJSON:    modelsJSON,
			hasModelsJSON: hasModelsJSON,
			extension:     extension,
			hasExtension:  hasExtension,
		},
	)
	if err != nil {
		if base == nil {
			return err
		}
		if setErr := collection.SetProvider(base); setErr != nil {
			return errors.Join(err, setErr)
		}
		return err
	}
	return collection.SetProvider(provider)
}

func (r *ModelRuntime) ensureBuiltinProvidersLocked() error {
	if r.builtinsReady {
		return nil
	}
	builtins, err := llm.BuiltinProviders()
	if err != nil {
		return err
	}
	providers := make(
		map[string]*llm.Provider,
		len(builtins),
	)
	userAgent := strings.TrimSpace(r.registry.catalogUserAgent)
	if userAgent == "" {
		userAgent = GetGiUserAgent(DefaultCodingAgentVersion)
	}
	for _, provider := range builtins {
		providerID := provider.ID
		if provider.ID != RadiusProviderID {
			provider, err = llm.WithRemoteCatalog(
				provider,
				llm.RemoteCatalogOptions{
					BaseURL:          r.registry.catalogBaseURL,
					Client:           r.registry.catalogClient,
					UserAgent:        userAgent,
					LocalGeneratedAt: llm.GetBuiltinModelDataGeneratedAt(),
				},
			)
			if err != nil {
				return fmt.Errorf(
					"configure remote catalog for %s: %w",
					providerID,
					err,
				)
			}
		}
		providers[provider.ID] = provider
	}
	r.builtinProviders = providers
	r.builtinsReady = true
	return nil
}

func (r *ModelRuntime) rebuildProvidersLocked() error {
	if r == nil || r.registry == nil {
		return errors.New("model runtime requires a registry")
	}
	if err := r.ensureBuiltinProvidersLocked(); err != nil {
		return err
	}
	if r.credentials == nil {
		r.credentials = newRuntimeCredentialOverlay(
			r.registry.authStorage,
		)
	}
	modelsStore := r.registry.modelsStore
	if modelsStore == nil {
		modelsStore = llm.NewInMemoryModelsStore()
	}
	collection := llm.NewModels(llm.ModelsOptions{
		Credentials: r.credentials,
		ModelsStore: modelsStore,
	})
	r.compositionErrors = map[string]string{}
	for _, providerID := range r.providerIDsLocked() {
		if err := r.recomposeProviderLocked(collection, providerID); err != nil {
			r.compositionErrors[providerID] = err.Error()
		}
	}
	r.models = collection
	r.updateModelSnapshotLocked()
	return nil
}

func (r *ModelRuntime) updateModelSnapshotLocked() {
	var all []llm.Model
	if r.models != nil {
		all = r.models.GetModels()
	}
	previous := r.snapshot.Load()
	configured := map[string]struct{}{}
	stored := map[string]struct{}{}
	auth := map[string]llm.AuthCheck{}
	if previous != nil {
		configured = cloneStringSet(previous.configuredProviders)
		stored = cloneStringSet(previous.storedProviders)
		auth = cloneRuntimeAuthChecks(previous.auth)
	}
	r.snapshot.Store(&modelRuntimeSnapshot{
		all:                 cloneRuntimeModels(all),
		available:           availableRuntimeModels(all, configured),
		configuredProviders: configured,
		storedProviders:     stored,
		auth:                auth,
	})
}

func (r *ModelRuntime) runAvailabilityRefresh(
	ctx context.Context,
) error {
	if r == nil {
		return errors.New("model runtime is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.RLock()
	models := r.models
	all := []llm.Model(nil)
	if models != nil {
		all = models.GetModels()
	}
	r.mu.RUnlock()
	if models == nil {
		return errors.New("model runtime is not initialized")
	}

	configured := map[string]struct{}{}
	authChecks := map[string]llm.AuthCheck{}
	var failures []error
	seen := map[string]struct{}{}
	for _, model := range all {
		if _, ok := seen[model.Provider]; ok {
			continue
		}
		seen[model.Provider] = struct{}{}
		check, ok, err := models.CheckAuth(ctx, model.Provider)
		if err != nil {
			failures = append(
				failures,
				fmt.Errorf("%s: %w", model.Provider, err),
			)
			continue
		}
		if ok {
			configured[model.Provider] = struct{}{}
			authChecks[model.Provider] = check
		}
	}
	stored := map[string]struct{}{}
	if r.credentials != nil {
		credentials, err := r.credentials.ListCredentials(ctx)
		if err != nil {
			failures = append(failures, err)
		} else {
			for _, credential := range credentials {
				stored[credential.ProviderID] = struct{}{}
			}
		}
	}

	var refreshErr error
	if len(failures) > 0 {
		refreshErr = errors.Join(failures...)
	}
	r.mu.Lock()
	r.availabilityError = ""
	if refreshErr != nil {
		r.availabilityError = refreshErr.Error()
	}
	r.snapshot.Store(&modelRuntimeSnapshot{
		all:                 cloneRuntimeModels(all),
		available:           availableRuntimeModels(all, configured),
		configuredProviders: configured,
		storedProviders:     stored,
		auth:                authChecks,
	})
	r.mu.Unlock()
	return refreshErr
}

func (r *ModelRuntime) queueAvailabilityRefresh(
	ctx context.Context,
	force bool,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		r.availabilityMu.Lock()
		call := r.availabilityCall
		if call == nil {
			call = &modelRuntimeAvailabilityCall{
				done: make(chan struct{}),
			}
			r.availabilityCall = call
			r.availabilityMu.Unlock()

			call.err = r.runAvailabilityRefresh(ctx)
			close(call.done)

			r.availabilityMu.Lock()
			if r.availabilityCall == call {
				r.availabilityCall = nil
			}
			r.availabilityMu.Unlock()
			return call.err
		}
		r.availabilityMu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-call.done:
			if !force {
				return call.err
			}
			r.availabilityMu.Lock()
			if r.availabilityCall == call {
				r.availabilityCall = nil
			}
			r.availabilityMu.Unlock()
			force = false
		}
	}
}

// RefreshAvailability coalesces concurrent availability readers.
func (r *ModelRuntime) RefreshAvailability(ctx context.Context) error {
	return r.queueAvailabilityRefresh(ctx, false)
}

// ForceRefreshAvailability waits for an older refresh before publishing a
// post-mutation snapshot.
func (r *ModelRuntime) ForceRefreshAvailability(
	ctx context.Context,
) error {
	return r.queueAvailabilityRefresh(ctx, true)
}

// GetProviders returns the current instance provider order.
func (r *ModelRuntime) GetProviders() []*llm.Provider {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	models := r.models
	r.mu.RUnlock()
	if models == nil {
		return nil
	}
	return models.GetProviders()
}

// GetProvider resolves one provider from this runtime only.
func (r *ModelRuntime) GetProvider(
	providerID string,
) (*llm.Provider, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	models := r.models
	r.mu.RUnlock()
	if models == nil {
		return nil, false
	}
	return models.GetProvider(providerID)
}

// GetModels returns a detached current catalog snapshot.
func (r *ModelRuntime) GetModels(
	providerID ...string,
) []llm.Model {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	models := r.models
	r.mu.RUnlock()
	if models == nil {
		return nil
	}
	return models.GetModels(providerID...)
}

// GetAll implements CodingModelRegistry.
func (r *ModelRuntime) GetAll() []llm.Model {
	return r.GetModels()
}

// GetModel finds one model in the current provider snapshot.
func (r *ModelRuntime) GetModel(
	providerID string,
	modelID string,
) (llm.Model, bool) {
	if r == nil {
		return llm.Model{}, false
	}
	r.mu.RLock()
	models := r.models
	r.mu.RUnlock()
	if models == nil {
		return llm.Model{}, false
	}
	return models.GetModel(providerID, modelID)
}

// Find implements CodingModelRegistry.
func (r *ModelRuntime) Find(
	providerID string,
	modelID string,
) (llm.Model, bool) {
	return r.GetModel(providerID, modelID)
}

// CheckAuth reports provider auth without exposing request secrets.
func (r *ModelRuntime) CheckAuth(
	ctx context.Context,
	providerID string,
) (llm.AuthCheck, bool, error) {
	if r == nil {
		return llm.AuthCheck{}, false, errors.New("model runtime is required")
	}
	r.mu.RLock()
	models := r.models
	r.mu.RUnlock()
	if models == nil {
		return llm.AuthCheck{}, false, nil
	}
	return models.CheckAuth(ctx, providerID)
}

// GetAvailable returns the last fully published availability snapshot and
// implements CodingModelRegistry without hidden network access.
func (r *ModelRuntime) GetAvailable() []llm.Model {
	return r.GetAvailableSnapshot()
}

// GetAvailableSnapshot returns a detached immutable availability view.
func (r *ModelRuntime) GetAvailableSnapshot() []llm.Model {
	if r == nil {
		return nil
	}
	snapshot := r.snapshot.Load()
	if snapshot == nil {
		return nil
	}
	return cloneRuntimeModels(snapshot.available)
}

// GetError combines configuration, provider-composition, and availability
// diagnostics without discarding a usable catalog.
func (r *ModelRuntime) GetError() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var messages []string
	if r.registry != nil && strings.TrimSpace(r.registry.GetError()) != "" {
		messages = append(messages, r.registry.GetError())
	}
	providerIDs := make([]string, 0, len(r.compositionErrors))
	for providerID := range r.compositionErrors {
		providerIDs = append(providerIDs, providerID)
	}
	sort.Strings(providerIDs)
	for _, providerID := range providerIDs {
		messages = append(
			messages,
			fmt.Sprintf(
				`Provider %q: %s`,
				providerID,
				r.compositionErrors[providerID],
			),
		)
	}
	if r.availabilityError != "" {
		messages = append(
			messages,
			"Availability refresh: "+r.availabilityError,
		)
	}
	return strings.Join(messages, "\n\n")
}

// GetRegisteredProviderConfig returns a detached extension-style provider
// overlay.
func (r *ModelRuntime) GetRegisteredProviderConfig(
	providerID string,
) (ProviderConfigInput, bool) {
	if r == nil {
		return ProviderConfigInput{}, false
	}
	r.mu.RLock()
	config, ok := r.registry.registeredProviders[providerID]
	r.mu.RUnlock()
	return cloneRuntimeProviderConfig(config), ok
}

// GetRegisteredProviderIDs returns config and native registrations in stable
// insertion order.
func (r *ModelRuntime) GetRegisteredProviderIDs() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]struct{}{}
	result := make([]string, 0, len(r.registry.registeredOrder)+len(r.nativeOrder))
	for _, providerID := range append(
		append([]string(nil), r.registry.registeredOrder...),
		r.nativeOrder...,
	) {
		if _, ok := seen[providerID]; ok {
			continue
		}
		seen[providerID] = struct{}{}
		result = append(result, providerID)
	}
	return result
}

// GetRegisteredNativeProvider returns the exact registered provider object.
func (r *ModelRuntime) GetRegisteredNativeProvider(
	providerID string,
) (*llm.Provider, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	provider, ok := r.nativeProviders[providerID]
	r.mu.RUnlock()
	return provider, ok
}

// GetCompatibilityRequestConfig returns a detached legacy request projection
// assembled from the same declarative provider layers as canonical auth.
func (r *ModelRuntime) GetCompatibilityRequestConfig(
	model llm.Model,
) ProviderRequestConfig {
	if r == nil {
		return ProviderRequestConfig{}
	}
	r.mu.RLock()
	registry := r.registry
	r.mu.RUnlock()
	if registry == nil {
		return ProviderRequestConfig{}
	}
	registry.mu.RLock()
	config := registry.providerRequestConfigs[model.Provider]
	config.Headers = cloneStringMap(config.Headers)
	modelsJSON, hasModelsJSON :=
		registry.modelsJSONProviders[model.Provider]
	extension, hasExtension :=
		registry.registeredProviders[model.Provider]
	registry.mu.RUnlock()
	return resolveCompatibilityRequestConfig(
		model,
		config,
		cloneModelsJSONProviderConfig(modelsJSON),
		hasModelsJSON,
		cloneRuntimeProviderConfig(extension),
		hasExtension,
	)
}

// IsUsingOAuth reads the published auth snapshot.
func (r *ModelRuntime) IsUsingOAuth(providerID string) bool {
	if r == nil {
		return false
	}
	snapshot := r.snapshot.Load()
	if snapshot == nil {
		return false
	}
	check, ok := snapshot.auth[providerID]
	return ok && check.Type == llm.CredentialTypeOAuth
}

// HasConfiguredAuth reads the published auth snapshot.
func (r *ModelRuntime) HasConfiguredAuth(providerID string) bool {
	if r == nil {
		return false
	}
	snapshot := r.snapshot.Load()
	if snapshot == nil {
		return false
	}
	_, ok := snapshot.configuredProviders[providerID]
	return ok
}

// GetAuth resolves one request-scoped auth snapshot.
func (r *ModelRuntime) GetAuth(
	ctx context.Context,
	model llm.Model,
	overrides llm.AuthResolutionOverrides,
) (*ResolvedRequestAuth, error) {
	if r == nil {
		return nil, errors.New("model runtime is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.RLock()
	models := r.models
	registry := r.registry
	if models == nil {
		r.mu.RUnlock()
		return nil, errors.New("model runtime is not initialized")
	}
	if _, ok := models.GetProvider(model.Provider); !ok {
		r.mu.RUnlock()
		return nil, &llm.ModelsError{
			Code: llm.ModelsErrorProvider,
			Msg:  fmt.Sprintf("Unknown provider: %s", model.Provider),
		}
	}
	var (
		modelsJSON    modelsJSONProviderConfig
		hasModelsJSON bool
		extension     ProviderConfigInput
		hasExtension  bool
	)
	if registry != nil {
		registry.mu.RLock()
		modelsJSON, hasModelsJSON =
			registry.modelsJSONProviders[model.Provider]
		extension, hasExtension =
			registry.registeredProviders[model.Provider]
		modelsJSON = cloneModelsJSONProviderConfig(modelsJSON)
		extension = cloneRuntimeProviderConfig(extension)
		registry.mu.RUnlock()
	}
	r.mu.RUnlock()
	resolution, err := models.GetModelAuth(ctx, model, overrides)
	if err != nil {
		return nil, err
	}
	if resolution == nil {
		return nil, nil
	}
	requestEnv := mergeResolvedProviderEnv(
		resolution.Env,
		overrides.Env,
	)
	modelHeaders, err := resolveConfiguredModelHeaders(
		model,
		modelsJSON,
		hasModelsJSON,
		extension,
		hasExtension,
		requestEnv,
	)
	if err != nil {
		return nil, err
	}
	return &ResolvedRequestAuth{
		OK:             true,
		APIKey:         resolution.Auth.APIKey,
		Headers:        mergeHeadersCaseInsensitive(resolution.Auth.Headers, modelHeaders),
		HeaderRemovals: clearResolvedHeaderRemovals(resolution.Auth.HeaderRemovals, modelHeaders),
		BaseURL:        resolution.Auth.BaseURL,
		Env:            requestEnv,
	}, nil
}

// SetRuntimeAPIKey installs a process-local key and publishes the resulting
// model availability.
func (r *ModelRuntime) SetRuntimeAPIKey(
	ctx context.Context,
	providerID string,
	apiKey string,
) error {
	if r == nil {
		return errors.New("model runtime is required")
	}
	r.mu.Lock()
	if r.credentials == nil {
		r.mu.Unlock()
		return errors.New("runtime credential storage is unavailable")
	}
	r.credentials.SetRuntimeAPIKey(providerID, apiKey)
	r.mu.Unlock()
	_, err := r.Refresh(ctx, ModelRegistryRefreshOptions{
		Timeout: r.registry.modelRefreshTimeout,
	})
	return err
}

// RemoveRuntimeAPIKey removes only the process-local key.
func (r *ModelRuntime) RemoveRuntimeAPIKey(
	ctx context.Context,
	providerID string,
) error {
	if r == nil {
		return errors.New("model runtime is required")
	}
	r.mu.Lock()
	if r.credentials == nil {
		r.mu.Unlock()
		return errors.New("runtime credential storage is unavailable")
	}
	r.credentials.RemoveRuntimeAPIKey(providerID)
	r.mu.Unlock()
	_, err := r.Refresh(ctx, ModelRegistryRefreshOptions{
		AllowNetwork: r.registry.modelNetworkEnabled,
		Timeout:      r.registry.modelRefreshTimeout,
	})
	return err
}

// ListCredentials returns metadata only; request secrets remain inside the
// credential boundary.
func (r *ModelRuntime) ListCredentials(
	ctx context.Context,
) ([]llm.CredentialInfo, error) {
	if r == nil {
		return nil, errors.New("model runtime is required")
	}
	r.mu.RLock()
	storage := r.credentials
	r.mu.RUnlock()
	if storage == nil {
		return nil, nil
	}
	return storage.ListCredentials(ctx)
}

// GetProviderAuthStatus returns a synchronous status projection suitable for
// UI and RPC responses.
func (r *ModelRuntime) GetProviderAuthStatus(
	providerID string,
) AuthStatus {
	if r == nil {
		return AuthStatus{}
	}
	r.mu.RLock()
	registry := r.registry
	if r.credentials != nil &&
		r.credentials.HasRuntimeAPIKey(providerID) {
		r.mu.RUnlock()
		return AuthStatus{
			Configured: true,
			Source:     "runtime",
		}
	}
	if r.registryCredentials &&
		registry != nil &&
		registry.authStorage != nil &&
		registry.authStorage.HasRuntimeAPIKey(providerID) {
		r.mu.RUnlock()
		return AuthStatus{
			Configured: true,
			Source:     "runtime",
		}
	}
	r.mu.RUnlock()
	snapshot := r.snapshot.Load()
	if snapshot != nil {
		if _, ok := snapshot.storedProviders[providerID]; ok {
			return AuthStatus{
				Configured: true,
				Source:     "stored",
			}
		}
	}
	if registry != nil {
		if configured := registry.configuredProviderRequestAuthStatus(
			providerID,
		); configured != nil {
			return *configured
		}
	}
	if snapshot != nil {
		if check, ok := snapshot.auth[providerID]; ok {
			return AuthStatus{
				Configured: true,
				Source:     "environment",
				Label:      check.Source,
			}
		}
	}
	return AuthStatus{}
}

type preparedModelRuntimeRequest struct {
	provider *llm.Provider
	model    llm.Model
	options  llm.StreamOptions
}

func (r *ModelRuntime) prepareRequest(
	ctx context.Context,
	model llm.Model,
	options llm.ModelsStreamOptions,
) (preparedModelRuntimeRequest, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	apiKeyOverride := options.APIKey
	if apiKeyOverride == nil && options.StreamOptions.APIKey != "" {
		apiKey := options.StreamOptions.APIKey
		apiKeyOverride = &apiKey
	}
	resolution, err := r.GetAuth(ctx, model, llm.AuthResolutionOverrides{
		APIKey: apiKeyOverride,
		Env:    options.StreamOptions.Env,
	})
	if err != nil {
		return preparedModelRuntimeRequest{}, err
	}
	if resolution == nil {
		return preparedModelRuntimeRequest{}, &llm.ModelsError{
			Code: llm.ModelsErrorAuth,
			Msg:  fmt.Sprintf("Provider is not configured: %s", model.Provider),
		}
	}

	r.mu.RLock()
	provider, ok := r.models.GetProvider(model.Provider)
	r.mu.RUnlock()
	if !ok {
		return preparedModelRuntimeRequest{}, &llm.ModelsError{
			Code: llm.ModelsErrorProvider,
			Msg:  fmt.Sprintf("Unknown provider: %s", model.Provider),
		}
	}

	requestOptions := cloneRuntimeStreamOptions(options.StreamOptions)
	requestOptions.Context = ctx
	requestOptions.APIKey = resolution.APIKey
	if apiKeyOverride != nil {
		requestOptions.APIKey = *apiKeyOverride
	}
	requestOptions.Headers = mergeHeadersCaseInsensitive(
		resolution.Headers,
		options.StreamOptions.Headers,
	)
	requestOptions.HeaderRemovals = clearResolvedHeaderRemovals(
		resolution.HeaderRemovals,
		options.StreamOptions.Headers,
	)
	requestOptions.HeaderRemovals = appendRuntimeHeaderRemovals(
		requestOptions.HeaderRemovals,
		options.StreamOptions.HeaderRemovals,
	)
	if options.TransformHeaders != nil {
		transformed, err := options.TransformHeaders(
			ctx,
			cloneStringMap(requestOptions.Headers),
		)
		if err != nil {
			return preparedModelRuntimeRequest{}, err
		}
		requestOptions.Headers = cloneStringMap(transformed)
		requestOptions.HeaderRemovals = clearResolvedHeaderRemovals(
			requestOptions.HeaderRemovals,
			requestOptions.Headers,
		)
	}
	requestOptions.Env = mergeResolvedProviderEnv(
		resolution.Env,
		options.StreamOptions.Env,
	)
	requestModel := cloneRuntimeModel(model)
	if resolution.BaseURL != "" {
		requestModel.BaseURL = resolution.BaseURL
	}
	return preparedModelRuntimeRequest{
		provider: provider,
		model:    requestModel,
		options:  requestOptions,
	}, nil
}

// Stream prepares and dispatches one provider-native stream.
func (r *ModelRuntime) Stream(
	ctx context.Context,
	model llm.Model,
	llmContext llm.Context,
	options llm.ModelsStreamOptions,
) (*llm.AssistantMessageEventStream, error) {
	prepared, err := r.prepareRequest(ctx, model, options)
	if err != nil {
		return nil, err
	}
	return prepared.provider.Stream(
		prepared.model,
		llmContext,
		prepared.options,
	)
}

// Complete collects Stream's terminal assistant message.
func (r *ModelRuntime) Complete(
	ctx context.Context,
	model llm.Model,
	llmContext llm.Context,
	options llm.ModelsStreamOptions,
) (llm.Message, error) {
	stream, err := r.Stream(ctx, model, llmContext, options)
	return completeRuntimeStream(ctx, model, stream, err)
}

// StreamSimple prepares and dispatches one simple provider stream.
func (r *ModelRuntime) StreamSimple(
	ctx context.Context,
	model llm.Model,
	llmContext llm.Context,
	options llm.ModelsStreamOptions,
) (*llm.AssistantMessageEventStream, error) {
	prepared, err := r.prepareRequest(ctx, model, options)
	if err != nil {
		return nil, err
	}
	return prepared.provider.StreamSimple(
		prepared.model,
		llmContext,
		prepared.options,
	)
}

// CompleteSimple collects StreamSimple's terminal assistant message.
func (r *ModelRuntime) CompleteSimple(
	ctx context.Context,
	model llm.Model,
	llmContext llm.Context,
	options llm.ModelsStreamOptions,
) (llm.Message, error) {
	stream, err := r.StreamSimple(ctx, model, llmContext, options)
	return completeRuntimeStream(ctx, model, stream, err)
}

// Login runs a provider-owned login flow and refreshes dependent model state.
func (r *ModelRuntime) Login(
	ctx context.Context,
	providerID string,
	credentialType llm.CredentialType,
	interaction llm.AuthInteraction,
) (llm.Credential, error) {
	if r == nil {
		return llm.Credential{}, errors.New("model runtime is required")
	}
	r.mu.RLock()
	models := r.models
	r.mu.RUnlock()
	if models == nil {
		return llm.Credential{}, errors.New("model runtime is not initialized")
	}
	credential, err := models.Login(
		ctx,
		providerID,
		credentialType,
		interaction,
	)
	if err != nil {
		return llm.Credential{}, err
	}
	_, err = r.Refresh(ctx, ModelRegistryRefreshOptions{
		AllowNetwork: r.registry.modelNetworkEnabled,
		Timeout:      r.registry.modelRefreshTimeout,
	})
	if err != nil {
		return llm.Credential{}, err
	}
	return credential, nil
}

// Logout removes one provider credential and rebuilds credential-dependent
// model projections.
func (r *ModelRuntime) Logout(
	ctx context.Context,
	providerID string,
) error {
	if r == nil {
		return errors.New("model runtime is required")
	}
	r.mu.RLock()
	models := r.models
	r.mu.RUnlock()
	if models == nil {
		return errors.New("model runtime is not initialized")
	}
	if err := models.Logout(ctx, providerID); err != nil {
		return err
	}
	_, err := r.Refresh(ctx, ModelRegistryRefreshOptions{
		AllowNetwork: r.registry.modelNetworkEnabled,
		Timeout:      r.registry.modelRefreshTimeout,
	})
	return err
}

// Refresh reparses configuration, refreshes dynamic catalogs according to the
// explicit network policy, and coalesces concurrent callers.
func (r *ModelRuntime) Refresh(
	ctx context.Context,
	options ModelRegistryRefreshOptions,
) (llm.ModelsRefreshResult, error) {
	if r == nil {
		return llm.ModelsRefreshResult{}, errors.New(
			"model runtime is required",
		)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.refreshMu.Lock()
	if call := r.refreshCall; call != nil {
		r.refreshMu.Unlock()
		select {
		case <-ctx.Done():
			return llm.ModelsRefreshResult{}, ctx.Err()
		case <-call.done:
			return cloneRuntimeRefreshResult(call.result), call.err
		}
	}
	call := &modelRuntimeRefreshCall{done: make(chan struct{})}
	r.refreshCall = call
	r.refreshMu.Unlock()

	r.mu.Lock()
	r.registry.refreshLocal()
	call.err = r.rebuildProvidersLocked()
	models := r.models
	r.mu.Unlock()
	if call.err == nil && models != nil {
		refreshCtx, cancel := modelRegistryRefreshContext(
			ctx,
			options.AllowNetwork,
			options.Timeout,
		)
		call.result = models.Refresh(
			refreshCtx,
			llm.ModelsRefreshOptions{
				Offline: !options.AllowNetwork,
				Force:   options.Force,
			},
		)
		cancel()
		publish := false
		r.mu.Lock()
		if r.models == models {
			r.updateModelSnapshotLocked()
			publish = true
		}
		r.mu.Unlock()
		if publish {
			r.registry.publishRuntimeModels(models.GetModels())
		}
	}
	// Availability failures are recorded by ForceRefreshAvailability and do
	// not roll back or hide a successfully refreshed model catalog.
	_ = r.ForceRefreshAvailability(ctx)
	close(call.done)

	r.refreshMu.Lock()
	if r.refreshCall == call {
		r.refreshCall = nil
	}
	r.refreshMu.Unlock()
	return cloneRuntimeRefreshResult(call.result), call.err
}

// RegisterNativeProvider installs a provider with its native auth and
// transport implementation.
func (r *ModelRuntime) RegisterNativeProvider(
	provider *llm.Provider,
) error {
	if r == nil {
		return errors.New("model runtime is required")
	}
	if provider == nil || strings.TrimSpace(provider.ID) == "" {
		return errors.New("provider ID is required")
	}
	r.mu.Lock()
	if _, exists := r.registry.GetRegisteredProviderConfig(
		provider.ID,
	); exists {
		r.registry.unregisterProviderLocal(provider.ID)
	}
	if _, exists := r.nativeProviders[provider.ID]; !exists {
		r.nativeOrder = append(r.nativeOrder, provider.ID)
	}
	r.nativeProviders[provider.ID] = provider
	err := r.rebuildProvidersLocked()
	models := r.models
	r.mu.Unlock()
	if err != nil {
		return err
	}
	if models != nil {
		r.registry.publishRuntimeModels(models.GetModels())
	}
	_ = r.ForceRefreshAvailability(context.Background())
	return nil
}

// RegisterProvider composes an extension-style provider overlay.
func (r *ModelRuntime) RegisterProvider(
	providerID string,
	config ProviderConfigInput,
) error {
	if r == nil {
		return errors.New("model runtime is required")
	}
	r.mu.Lock()
	if err := r.registry.registerProviderLocal(providerID, config); err != nil {
		r.mu.Unlock()
		return err
	}
	delete(r.nativeProviders, providerID)
	r.nativeOrder = removeString(r.nativeOrder, providerID)
	err := r.rebuildProvidersLocked()
	models := r.models
	r.mu.Unlock()
	if err != nil {
		return err
	}
	if models != nil {
		r.registry.publishRuntimeModels(models.GetModels())
	}
	_ = r.ForceRefreshAvailability(context.Background())
	return nil
}

// UnregisterProvider removes config and native registrations, then restores
// the built-in provider with the same ID when present.
func (r *ModelRuntime) UnregisterProvider(providerID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.registry.unregisterProviderLocal(providerID)
	delete(r.nativeProviders, providerID)
	r.nativeOrder = removeString(r.nativeOrder, providerID)
	_ = r.rebuildProvidersLocked()
	models := r.models
	r.mu.Unlock()
	if models != nil {
		r.registry.publishRuntimeModels(models.GetModels())
	}
	_ = r.ForceRefreshAvailability(context.Background())
}

func applyRuntimeModelOverrides(
	models []llm.Model,
	overrides map[string]ModelOverride,
) []llm.Model {
	result := cloneRuntimeModels(models)
	for index := range result {
		if override, ok := overrides[result[index].ID]; ok {
			result[index] = applyModelOverride(result[index], override)
		}
	}
	return result
}

func completeRuntimeStream(
	ctx context.Context,
	model llm.Model,
	stream *llm.AssistantMessageEventStream,
	err error,
) (llm.Message, error) {
	if err != nil {
		return llm.Message{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	message, err := stream.Result(ctx)
	if err != nil && ctx.Err() != nil {
		return llm.AssistantErrorMessage(
			ctx.Err().Error(),
			model,
			true,
		), nil
	}
	return message, err
}

func availableRuntimeModels(
	all []llm.Model,
	configured map[string]struct{},
) []llm.Model {
	available := make([]llm.Model, 0, len(all))
	for _, model := range all {
		if _, ok := configured[model.Provider]; ok {
			available = append(available, cloneRuntimeModel(model))
		}
	}
	return available
}

func cloneRuntimeModels(models []llm.Model) []llm.Model {
	if len(models) == 0 {
		return nil
	}
	cloned := make([]llm.Model, len(models))
	for index, model := range models {
		cloned[index] = cloneRuntimeModel(model)
	}
	return cloned
}

func cloneRuntimeModel(model llm.Model) llm.Model {
	model.Headers = cloneStringMap(model.Headers)
	model.Input = append([]string(nil), model.Input...)
	model.Cost.Tiers = append(
		[]llm.ModelCostTier(nil),
		model.Cost.Tiers...,
	)
	model.ThinkingLevelMap = cloneThinkingLevelMap(
		model.ThinkingLevelMap,
	)
	model.Compat.OpenRouterRouting = cloneRuntimeAnyMap(
		model.Compat.OpenRouterRouting,
	)
	model.Compat.VercelGatewayRouting = cloneRuntimeAnyMap(
		model.Compat.VercelGatewayRouting,
	)
	model.Compat.ChatTemplateKwargs = cloneRuntimeAnyMap(
		model.Compat.ChatTemplateKwargs,
	)
	return model
}

func cloneRuntimeAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneRuntimeProviderConfig(
	config ProviderConfigInput,
) ProviderConfigInput {
	config.Headers = cloneOptionalStringMap(config.Headers)
	config.Models = cloneProviderModelDefinitions(config.Models)
	if config.AuthHeader != nil {
		authHeader := *config.AuthHeader
		config.AuthHeader = &authHeader
	}
	if config.OAuth != nil {
		oauth := *config.OAuth
		config.OAuth = &oauth
	}
	config.ModelOverrides = cloneModelOverrideMap(
		config.ModelOverrides,
	)
	return config
}

func cloneRuntimeStreamOptions(
	options llm.StreamOptions,
) llm.StreamOptions {
	options.Headers = cloneStringMap(options.Headers)
	options.HeaderRemovals = append(
		[]string(nil),
		options.HeaderRemovals...,
	)
	options.Env = cloneResolvedProviderEnv(options.Env)
	options.ThinkingBudgets = cloneRuntimeIntMap(
		options.ThinkingBudgets,
	)
	options.Metadata = cloneRuntimeAnyMap(options.Metadata)
	options.Timeouts = cloneRuntimeStreamTimeouts(options.Timeouts)
	return options
}

func cloneRuntimeStreamTimeouts(
	timeouts llm.StreamTimeouts,
) llm.StreamTimeouts {
	if timeouts.HTTPIdle != nil {
		timeout := *timeouts.HTTPIdle
		timeouts.HTTPIdle = &timeout
	}
	if timeouts.WebSocketConnect != nil {
		timeout := *timeouts.WebSocketConnect
		timeouts.WebSocketConnect = &timeout
	}
	return timeouts
}

func cloneRuntimeIntMap(values map[string]int) map[string]int {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]int, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func appendRuntimeHeaderRemovals(
	base []string,
	additions []string,
) []string {
	result := append([]string(nil), base...)
	for _, addition := range additions {
		found := false
		for _, existing := range result {
			if strings.EqualFold(existing, addition) {
				found = true
				break
			}
		}
		if !found {
			result = append(result, addition)
		}
	}
	return result
}

func cloneStringSet(
	values map[string]struct{},
) map[string]struct{} {
	cloned := make(map[string]struct{}, len(values))
	for value := range values {
		cloned[value] = struct{}{}
	}
	return cloned
}

func cloneRuntimeAuthChecks(
	values map[string]llm.AuthCheck,
) map[string]llm.AuthCheck {
	cloned := make(map[string]llm.AuthCheck, len(values))
	for providerID, check := range values {
		cloned[providerID] = check
	}
	return cloned
}

func cloneRuntimeRefreshResult(
	result llm.ModelsRefreshResult,
) llm.ModelsRefreshResult {
	cloned := llm.ModelsRefreshResult{
		Aborted: result.Aborted,
		Errors:  make(map[string]error, len(result.Errors)),
	}
	for providerID, err := range result.Errors {
		cloned.Errors[providerID] = err
	}
	return cloned
}
