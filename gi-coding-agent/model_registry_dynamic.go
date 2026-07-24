package gicodingagent

import (
	"fmt"
	"sort"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

// RadiusProviderID is the built-in Radius provider identifier used by
// models.json OAuth composition.
const RadiusProviderID = "radius"

type radiusRegistryProvider struct {
	id      string
	name    string
	gateway string
}

func (r *ModelRegistry) configureDynamicModels(
	configured []radiusRegistryProvider,
) error {
	r.dynamicModels = nil
	var credentials llm.CredentialStore = llm.NewInMemoryCredentialStore()
	if r.authStorage != nil {
		credentials = r.authStorage
	}
	models := llm.NewModels(llm.ModelsOptions{
		Credentials: credentials,
		ModelsStore: r.modelsStore,
	})

	defaultRadius, err := llm.NewRadiusProvider(llm.RadiusProviderOptions{
		Client: r.radiusClient,
	})
	if err != nil {
		return err
	}
	if err := models.SetProvider(defaultRadius); err != nil {
		return err
	}

	sort.Slice(configured, func(i, j int) bool {
		return configured[i].id < configured[j].id
	})
	for _, config := range configured {
		provider, err := llm.NewRadiusProvider(llm.RadiusProviderOptions{
			ID:      config.id,
			Name:    config.name,
			Gateway: config.gateway,
			Client:  r.radiusClient,
		})
		if err != nil {
			return fmt.Errorf(
				"Provider %s: configure Radius gateway: %w",
				config.id,
				err,
			)
		}
		if err := models.SetProvider(provider); err != nil {
			return err
		}
	}
	r.dynamicModels = models
	return nil
}

func (r *ModelRegistry) syncDynamicModels() {
	if r == nil {
		return
	}
	r.models = append([]llm.Model(nil), r.baseModels...)
	if r.dynamicModels != nil {
		r.models = mergeMissingDynamicModels(
			r.models,
			r.dynamicModels.GetModels(),
		)
	}
	r.applyConfiguredModelOverrides()
}

func mergeMissingDynamicModels(
	base []llm.Model,
	dynamic []llm.Model,
) []llm.Model {
	type modelIdentity struct {
		provider string
		id       string
	}
	merged := append([]llm.Model(nil), base...)
	seen := make(map[modelIdentity]struct{}, len(merged))
	for _, model := range merged {
		seen[modelIdentity{provider: model.Provider, id: model.ID}] = struct{}{}
	}
	for _, model := range dynamic {
		key := modelIdentity{provider: model.Provider, id: model.ID}
		if _, exists := seen[key]; exists {
			continue
		}
		merged = append(merged, model)
		seen[key] = struct{}{}
	}
	return merged
}

func radiusGatewayFromBaseURL(baseURL string) string {
	gateway := strings.TrimRight(baseURL, "/")
	return strings.TrimSuffix(gateway, "/v1")
}
