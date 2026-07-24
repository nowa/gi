package gillmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultRadiusGateway is Pi's hosted Radius endpoint.
	DefaultRadiusGateway = "https://radius.pi.dev"

	maxRadiusConfigBodyBytes = 16 << 20
	maxRadiusErrorBodyBytes  = 16 << 10
)

var radiusGatewayScheme = regexp.MustCompile(
	`(?i)^[a-z][a-z0-9+.-]*://`,
)

// RadiusGatewayModel is one model advertised by a Radius gateway.
type RadiusGatewayModel struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Reasoning        bool               `json:"reasoning"`
	ThinkingLevelMap map[string]*string `json:"thinkingLevelMap,omitempty"`
	Input            []string           `json:"input"`
	Cost             ModelCost          `json:"cost"`
	ContextWindow    int                `json:"contextWindow"`
	MaxTokens        int                `json:"maxTokens"`
}

// RadiusGatewayConfig is the validated dynamic catalog returned by /v1/config.
type RadiusGatewayConfig struct {
	BaseURL string               `json:"baseUrl"`
	Models  []RadiusGatewayModel `json:"models"`
}

// RadiusProviderOptions configures a default or custom Radius gateway.
type RadiusProviderOptions struct {
	ID          string
	Name        string
	Gateway     string
	Client      HTTPDoer
	OAuthLoader OAuthAuthLoader
}

// NormalizeRadiusGatewayURL adds HTTPS when no URI scheme is present and
// removes trailing slashes. Unsupported explicit schemes remain intact so the
// endpoint validator can reject them instead of silently rewriting them.
func NormalizeRadiusGatewayURL(value string) string {
	value = strings.TrimSpace(value)
	if !radiusGatewayScheme.MatchString(value) {
		value = "https://" + value
	}
	return strings.TrimRight(value, "/")
}

// GetRadiusCredentialConfig reads and validates the catalog cached by Pi's
// pre-ModelsStore Radius credential format.
func GetRadiusCredentialConfig(
	credential *Credential,
) (RadiusGatewayConfig, bool) {
	if credential == nil || credential.Type != CredentialTypeOAuth {
		return RadiusGatewayConfig{}, false
	}
	value, ok := credential.Metadata["gatewayConfig"]
	if !ok {
		return RadiusGatewayConfig{}, false
	}
	return sanitizeRadiusGatewayConfig(value)
}

// GetRadiusModelsFromConfig converts gateway rows into detached Gi models.
func GetRadiusModelsFromConfig(
	providerID string,
	config RadiusGatewayConfig,
) []Model {
	models := make([]Model, 0, len(config.Models))
	for _, gatewayModel := range config.Models {
		models = append(models, Model{
			ID:            gatewayModel.ID,
			Name:          gatewayModel.Name,
			API:           piMessagesAPI,
			Provider:      providerID,
			BaseURL:       config.BaseURL,
			Reasoning:     gatewayModel.Reasoning,
			Input:         append([]string(nil), gatewayModel.Input...),
			Cost:          cloneModelCost(gatewayModel.Cost),
			ContextWindow: gatewayModel.ContextWindow,
			MaxTokens:     gatewayModel.MaxTokens,
			ThinkingLevelMap: cloneThinkingLevelMap(
				gatewayModel.ThinkingLevelMap,
			),
		})
	}
	return models
}

// GetRadiusModels returns models embedded in a legacy OAuth credential.
func GetRadiusModels(providerID string, credential *Credential) []Model {
	config, ok := GetRadiusCredentialConfig(credential)
	if !ok {
		return nil
	}
	return GetRadiusModelsFromConfig(providerID, config)
}

// LoadRadiusGatewayConfig loads and validates one gateway catalog.
func LoadRadiusGatewayConfig(
	ctx context.Context,
	client HTTPDoer,
	gateway string,
	apiKey string,
) (RadiusGatewayConfig, error) {
	ctx = contextOrBackground(ctx)
	endpoint, err := radiusConfigEndpoint(gateway)
	if err != nil {
		return RadiusGatewayConfig{}, err
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint.String(),
		nil,
	)
	if err != nil {
		return RadiusGatewayConfig{}, err
	}
	request.Header.Set("accept", "application/json")
	if apiKey != "" {
		request.Header.Set("authorization", "Bearer "+apiKey)
	}
	response, err := httpClientOrDefault(client).Do(request)
	if err != nil {
		return RadiusGatewayConfig{}, err
	}
	if response.Body == nil {
		return RadiusGatewayConfig{}, fmt.Errorf(
			"Radius config response from %s has no body",
			gateway,
		)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(io.LimitReader(
			response.Body,
			maxRadiusErrorBodyBytes,
		))
		if readErr != nil {
			return RadiusGatewayConfig{}, fmt.Errorf(
				"read Radius config error from %s: %w",
				gateway,
				readErr,
			)
		}
		return RadiusGatewayConfig{}, fmt.Errorf(
			"Could not load Radius config from %s: %d: %s",
			gateway,
			response.StatusCode,
			truncateRadiusHTTPBody(string(body)),
		)
	}

	body, err := io.ReadAll(io.LimitReader(
		response.Body,
		maxRadiusConfigBodyBytes+1,
	))
	if err != nil {
		return RadiusGatewayConfig{}, fmt.Errorf(
			"read Radius config from %s: %w",
			gateway,
			err,
		)
	}
	if len(body) > maxRadiusConfigBodyBytes {
		return RadiusGatewayConfig{}, fmt.Errorf(
			"Invalid Radius config from %s: response exceeds %d bytes",
			gateway,
			maxRadiusConfigBodyBytes,
		)
	}
	config, ok := sanitizeRadiusGatewayConfig(json.RawMessage(body))
	if !ok {
		return RadiusGatewayConfig{}, fmt.Errorf(
			"Invalid Radius config from %s",
			gateway,
		)
	}
	return config, nil
}

// NewRadiusProvider creates an instance-scoped dynamic Radius provider.
func NewRadiusProvider(options RadiusProviderOptions) (*Provider, error) {
	id := options.ID
	if id == "" {
		id = "radius"
	}
	name := options.Name
	if name == "" {
		name = "Radius"
	}
	gateway := options.Gateway
	if gateway == "" {
		gateway = DefaultRadiusGateway
	}
	gateway = NormalizeRadiusGatewayURL(gateway)
	if _, err := radiusConfigEndpoint(gateway); err != nil {
		return nil, err
	}

	client := httpClientOrDefault(options.Client)
	oauth, err := radiusOAuthAuth(
		name,
		gateway,
		client,
		options.OAuthLoader,
	)
	if err != nil {
		return nil, err
	}
	state := &radiusProviderState{
		providerID: id,
		gateway:    gateway,
		client:     client,
	}
	transport := NewPiMessagesProvider(client)
	return &Provider{
		ID:      id,
		Name:    name,
		BaseURL: gateway,
		Auth: ProviderAuth{
			APIKey: EnvAPIKeyAuth("Radius API key", "RADIUS_API_KEY"),
			OAuth:  oauth,
		},
		ModelSource:       state.getModels,
		RefreshModelsFunc: state.refresh,
		StreamFunc:        transport.Stream,
		StreamSimpleFunc:  transport.StreamSimple,
	}, nil
}

func radiusOAuthAuth(
	name string,
	gateway string,
	client HTTPDoer,
	loader OAuthAuthLoader,
) (*OAuthAuth, error) {
	builtin, err := NewRadiusOAuth(RadiusOAuthOptions{
		Name:    name,
		Gateway: gateway,
		Client:  client,
	})
	if err != nil {
		return nil, err
	}
	lazy := registeredOrBuiltinOAuthAuth("radius", builtin)
	if loader != nil {
		lazy = LazyOAuthAuth(name, "", loader)
	}
	return &OAuthAuth{
		Name:       name,
		LoginLabel: lazy.LoginLabel,
		Login:      lazy.Login,
		Refresh:    lazy.Refresh,
		ToAuth: func(ctx context.Context, credential Credential) (ModelAuth, error) {
			if err := contextError(contextOrBackground(ctx)); err != nil {
				return ModelAuth{}, err
			}
			if credential.Type != CredentialTypeOAuth ||
				strings.TrimSpace(credential.Access) == "" {
				return ModelAuth{}, errors.New(
					"Radius OAuth credential has no access token",
				)
			}
			return ModelAuth{APIKey: credential.Access}, nil
		},
	}, nil
}

type radiusProviderState struct {
	providerID string
	gateway    string
	client     HTTPDoer

	modelsMu sync.RWMutex
	models   []Model
}

func (s *radiusProviderState) getModels() ([]Model, error) {
	s.modelsMu.RLock()
	models := cloneModels(s.models)
	s.modelsMu.RUnlock()
	return models, nil
}

func (s *radiusProviderState) refresh(
	ctx context.Context,
	input RefreshModelsContext,
) error {
	ctx = contextOrBackground(ctx)
	if input.Store == nil {
		return errors.New("provider model store is required")
	}

	stored, exists, err := input.Store.ReadModels(ctx)
	if err != nil {
		return err
	}
	if exists {
		s.publish(filterProviderModels(stored.Models, s.providerID))
	} else if legacy := GetRadiusModels(s.providerID, input.Credential); len(legacy) > 0 {
		if err := s.persistAndPublish(ctx, input.Store, legacy); err != nil {
			return err
		}
	}

	if err := contextError(ctx); err != nil {
		return err
	}
	if !input.AllowNetwork {
		return nil
	}
	apiKey := ""
	if input.Credential != nil {
		switch input.Credential.Type {
		case CredentialTypeOAuth:
			apiKey = input.Credential.Access
		case CredentialTypeAPIKey:
			apiKey = input.Credential.Key
		}
	}
	config, err := LoadRadiusGatewayConfig(
		ctx,
		s.client,
		s.gateway,
		apiKey,
	)
	if err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	return s.persistAndPublish(
		ctx,
		input.Store,
		GetRadiusModelsFromConfig(s.providerID, config),
	)
}

func (s *radiusProviderState) persistAndPublish(
	ctx context.Context,
	store ProviderModelsStore,
	models []Model,
) error {
	models = cloneModels(models)
	if err := store.WriteModels(ctx, ModelsStoreEntry{
		Models:    models,
		CheckedAt: time.Now().UnixMilli(),
	}); err != nil {
		return err
	}
	s.publish(models)
	return nil
}

func (s *radiusProviderState) publish(models []Model) {
	s.modelsMu.Lock()
	s.models = cloneModels(models)
	s.modelsMu.Unlock()
}

func filterProviderModels(models []Model, providerID string) []Model {
	filtered := make([]Model, 0, len(models))
	for _, model := range models {
		if model.Provider == providerID {
			filtered = append(filtered, cloneModel(model))
		}
	}
	return filtered
}

func radiusConfigEndpoint(gateway string) (*url.URL, error) {
	base, err := url.Parse(gateway)
	if err != nil {
		return nil, fmt.Errorf("parse Radius gateway %q: %w", gateway, err)
	}
	if base.Scheme != "http" && base.Scheme != "https" || base.Host == "" {
		return nil, fmt.Errorf("invalid Radius gateway URL %q", gateway)
	}
	return base.ResolveReference(&url.URL{Path: "/v1/config"}), nil
}

func sanitizeRadiusGatewayConfig(value any) (RadiusGatewayConfig, bool) {
	raw, err := json.Marshal(value)
	if err != nil {
		return RadiusGatewayConfig{}, false
	}
	var object struct {
		BaseURL *string           `json:"baseUrl"`
		Models  []json.RawMessage `json:"models"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&object); err != nil ||
		object.BaseURL == nil ||
		object.Models == nil {
		return RadiusGatewayConfig{}, false
	}
	config := RadiusGatewayConfig{
		BaseURL: *object.BaseURL,
		Models:  make([]RadiusGatewayModel, 0, len(object.Models)),
	}
	for _, rawModel := range object.Models {
		model, ok := sanitizeRadiusGatewayModel(rawModel)
		if ok {
			config.Models = append(config.Models, model)
		}
	}
	return config, true
}

func sanitizeRadiusGatewayModel(
	raw json.RawMessage,
) (RadiusGatewayModel, bool) {
	var candidate struct {
		ID               *string            `json:"id"`
		Name             *string            `json:"name"`
		Reasoning        *bool              `json:"reasoning"`
		ThinkingLevelMap map[string]*string `json:"thinkingLevelMap"`
		Input            *[]string          `json:"input"`
		Cost             *ModelCost         `json:"cost"`
		ContextWindow    *int               `json:"contextWindow"`
		MaxTokens        *int               `json:"maxTokens"`
	}
	if err := json.Unmarshal(raw, &candidate); err != nil ||
		candidate.ID == nil ||
		candidate.Name == nil ||
		candidate.Reasoning == nil ||
		candidate.Input == nil ||
		candidate.Cost == nil ||
		candidate.ContextWindow == nil ||
		candidate.MaxTokens == nil {
		return RadiusGatewayModel{}, false
	}
	return RadiusGatewayModel{
		ID:               *candidate.ID,
		Name:             *candidate.Name,
		Reasoning:        *candidate.Reasoning,
		ThinkingLevelMap: cloneThinkingLevelMap(candidate.ThinkingLevelMap),
		Input:            append([]string(nil), (*candidate.Input)...),
		Cost:             cloneModelCost(*candidate.Cost),
		ContextWindow:    *candidate.ContextWindow,
		MaxTokens:        *candidate.MaxTokens,
	}, true
}

func cloneModelCost(cost ModelCost) ModelCost {
	cost.Tiers = append([]ModelCostTier(nil), cost.Tiers...)
	return cost
}

func cloneThinkingLevelMap(values map[string]*string) map[string]*string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]*string, len(values))
	for key, value := range values {
		if value == nil {
			cloned[key] = nil
			continue
		}
		mapped := *value
		cloned[key] = &mapped
	}
	return cloned
}

func truncateRadiusHTTPBody(body string) string {
	trimmed := strings.TrimSpace(body)
	runes := []rune(trimmed)
	if len(runes) <= 512 {
		return trimmed
	}
	return string(runes[:512]) + "…"
}
