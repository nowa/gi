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
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultRemoteCatalogBaseURL is the release catalog service used by Pi.
	DefaultRemoteCatalogBaseURL = "https://pi.dev"

	// RemoteCatalogRefreshInterval is the minimum interval between successful
	// or explicitly unavailable remote catalog checks.
	RemoteCatalogRefreshInterval = 4 * time.Hour

	maxRemoteCatalogBytes = 32 << 20
)

// RemoteCatalogOptions supplies stable dependencies for a provider's remote
// catalog overlay. LocalGeneratedAt decides whether a persisted remote
// catalog is newer than the compiled catalog.
type RemoteCatalogOptions struct {
	BaseURL          string
	Client           HTTPDoer
	UserAgent        string
	LocalGeneratedAt time.Time
	RefreshInterval  time.Duration

	// Now is intended for deterministic hosts and tests. The zero value uses
	// time.Now.
	Now func() time.Time
}

// WithRemoteCatalog returns a provider whose immutable baseline is overlaid
// by a persisted, atomically published remote catalog. It creates fresh
// synchronization state instead of copying Provider's mutexes.
func WithRemoteCatalog(
	provider *Provider,
	options RemoteCatalogOptions,
) (*Provider, error) {
	if provider == nil || strings.TrimSpace(provider.ID) == "" {
		return nil, errors.New("provider ID is required")
	}
	endpoint, err := remoteCatalogEndpoint(options.BaseURL, provider.ID)
	if err != nil {
		return nil, err
	}
	refreshInterval := options.RefreshInterval
	if refreshInterval <= 0 {
		refreshInterval = RemoteCatalogRefreshInterval
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	state := &remoteCatalogState{
		providerID:       provider.ID,
		baseline:         provider.GetModels,
		endpoint:         endpoint,
		client:           httpClientOrDefault(options.Client),
		userAgent:        strings.TrimSpace(options.UserAgent),
		localGeneratedAt: options.LocalGeneratedAt,
		refreshInterval:  refreshInterval,
		now:              now,
	}
	return &Provider{
		ID:                provider.ID,
		Name:              provider.Name,
		BaseURL:           provider.BaseURL,
		Headers:           cloneStringMap(provider.Headers),
		Auth:              provider.Auth,
		ModelSource:       state.models,
		RefreshModelsFunc: state.refresh,
		FilterModelsFunc:  provider.FilterModelsFunc,
		StreamFunc:        provider.StreamFunc,
		StreamSimpleFunc:  provider.StreamSimpleFunc,
	}, nil
}

type remoteCatalogState struct {
	providerID       string
	baseline         func() ([]Model, error)
	endpoint         string
	client           HTTPDoer
	userAgent        string
	localGeneratedAt time.Time
	refreshInterval  time.Duration
	now              func() time.Time

	mu      sync.RWMutex
	dynamic []Model
}

func (s *remoteCatalogState) models() ([]Model, error) {
	baseline, err := s.baseline()
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	dynamic := cloneModels(s.dynamic)
	s.mu.RUnlock()
	return mergeCatalogModels(baseline, dynamic), nil
}

func (s *remoteCatalogState) refresh(
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
		s.publish(remoteCatalogModels(
			stored,
			s.providerID,
			s.localGeneratedAt,
		))
	}
	if !input.AllowNetwork {
		return contextError(ctx)
	}
	if err := contextError(ctx); err != nil {
		return err
	}

	now := s.now()
	if !input.Force &&
		exists &&
		stored.CheckedAt > 0 &&
		stored.LastModified != nil &&
		now.UnixMilli()-stored.CheckedAt <
			s.refreshInterval.Milliseconds() {
		return nil
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		s.endpoint,
		nil,
	)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if s.userAgent != "" {
		request.Header.Set("User-Agent", s.userAgent)
	}
	// A validator is only useful together with its cached representation:
	// accepting a 304 without a body to restore would publish an empty overlay.
	if len(stored.Models) > 0 && stored.ETag != "" {
		request.Header.Set("If-None-Match", stored.ETag)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf(
			"model catalog request failed for %s: %w",
			s.providerID,
			err,
		)
	}
	defer response.Body.Close()
	if err := contextError(ctx); err != nil {
		return err
	}

	checkedAt := s.now().UnixMilli()
	switch response.StatusCode {
	case http.StatusNotModified:
		if exists {
			stored.CheckedAt = checkedAt
			return input.Store.WriteModels(ctx, stored)
		}
	case http.StatusNotFound, http.StatusNotImplemented:
		entry := stored
		if !exists {
			entry.Models = []Model{}
		}
		entry.CheckedAt = checkedAt
		entry.LastModified = int64Pointer(0)
		entry.ETag = ""
		return input.Store.WriteModels(ctx, entry)
	}
	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		entry := stored
		if !exists {
			entry.Models = []Model{}
		}
		entry.CheckedAt = checkedAt
		if err := input.Store.WriteModels(ctx, entry); err != nil {
			return err
		}
		return fmt.Errorf(
			"model catalog request failed for %s: %d",
			s.providerID,
			response.StatusCode,
		)
	}

	body, err := readRemoteCatalogBody(response.Body)
	if err != nil {
		return fmt.Errorf(
			"read model catalog for %s: %w",
			s.providerID,
			err,
		)
	}
	refreshed, err := parseRemoteCatalog(s.providerID, body)
	if err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	lastModified := int64(0)
	if value := response.Header.Get("Last-Modified"); value != "" {
		if parsed, parseErr := http.ParseTime(value); parseErr == nil {
			lastModified = parsed.UnixMilli()
		}
	}
	entry := ModelsStoreEntry{
		Models:       refreshed,
		LastModified: &lastModified,
		CheckedAt:    checkedAt,
		ETag:         response.Header.Get("ETag"),
	}
	dynamic := remoteCatalogModels(
		entry,
		s.providerID,
		s.localGeneratedAt,
	)
	if err := input.Store.WriteModels(ctx, entry); err != nil {
		return err
	}
	s.publish(dynamic)
	return nil
}

func (s *remoteCatalogState) publish(models []Model) {
	s.mu.Lock()
	s.dynamic = cloneModels(models)
	s.mu.Unlock()
}

func mergeCatalogModels(
	baseline []Model,
	dynamic []Model,
) []Model {
	merged := cloneModels(baseline)
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
	return merged
}

func remoteCatalogModels(
	entry ModelsStoreEntry,
	providerID string,
	localGeneratedAt time.Time,
) []Model {
	if !localGeneratedAt.IsZero() &&
		(entry.LastModified == nil ||
			*entry.LastModified <= localGeneratedAt.UnixMilli()) {
		return nil
	}
	models := make([]Model, 0, len(entry.Models))
	for _, model := range entry.Models {
		if model.Provider == providerID {
			models = append(models, cloneModel(model))
		}
	}
	return models
}

func parseRemoteCatalog(
	providerID string,
	raw []byte,
) ([]Model, error) {
	entries, err := remoteCatalogEntries(raw)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid model catalog for provider %q: %w",
			providerID,
			err,
		)
	}
	models := make([]Model, 0, len(entries))
	for _, entry := range entries {
		var identity struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(entry, &identity); err != nil ||
			identity.ID == "" {
			continue
		}
		var model Model
		if err := json.Unmarshal(entry, &model); err != nil {
			return nil, fmt.Errorf(
				"invalid model %q for provider %q: %w",
				identity.ID,
				providerID,
				err,
			)
		}
		model.Provider = providerID
		models = append(models, model)
	}
	return models, nil
}

func remoteCatalogEntries(raw []byte) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	var array []json.RawMessage
	if len(trimmed) > 0 && trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &array); err != nil {
			return nil, err
		}
		return array, nil
	}

	var object map[string]json.RawMessage
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("expected an array or object")
	}
	if err := json.Unmarshal(trimmed, &object); err != nil ||
		object == nil {
		return nil, errors.New("expected an array or object")
	}
	if models, ok := object["models"]; ok {
		models = bytes.TrimSpace(models)
		if len(models) == 0 || models[0] != '[' {
			return nil, errors.New(`"models" must be an array`)
		}
		if err := json.Unmarshal(models, &array); err != nil {
			return nil, err
		}
		return array, nil
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entries := make([]json.RawMessage, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, object[key])
	}
	return entries, nil
}

func readRemoteCatalogBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(
		reader,
		maxRemoteCatalogBytes+1,
	))
	if err != nil {
		return nil, err
	}
	if len(body) > maxRemoteCatalogBytes {
		return nil, fmt.Errorf(
			"response exceeds %d bytes",
			maxRemoteCatalogBytes,
		)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("empty response")
	}
	return body, nil
}

func remoteCatalogEndpoint(
	baseURL string,
	providerID string,
) (string, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultRemoteCatalogBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid model catalog base URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" {
		return "", fmt.Errorf(
			"invalid model catalog base URL %q",
			baseURL,
		)
	}
	escapedID := url.PathEscape(providerID)
	parsed.Path = "/api/models/providers/" + providerID
	parsed.RawPath = "/api/models/providers/" + escapedID
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func int64Pointer(value int64) *int64 {
	return &value
}
