package gillmprovider

import "sync"

var imageModelRegistry = struct {
	sync.RWMutex
	models        map[string]map[string]ImagesModel
	providerOrder []string
	modelOrder    map[string][]string
}{
	models:     map[string]map[string]ImagesModel{},
	modelOrder: map[string][]string{},
}

// builtinImageModelCatalog is immutable release input. The public registry is
// a compatibility surface and must not be able to contaminate fresh built-in
// provider instances.
var builtinImageModelCatalog = []ImagesModel{
	{
		ID:       "black-forest-labs/flux.2-pro",
		Name:     "Black Forest Labs: FLUX.2 Pro",
		API:      "openrouter-images",
		Provider: "openrouter",
		BaseURL:  "https://openrouter.ai/api/v1",
		Input:    []string{"text", "image"},
		Output:   []string{"image"},
	},
	{
		ID:       "google/gemini-2.5-flash-image",
		Name:     "Google: Nano Banana (Gemini 2.5 Flash Image)",
		API:      "openrouter-images",
		Provider: "openrouter",
		BaseURL:  "https://openrouter.ai/api/v1",
		Input:    []string{"image", "text"},
		Output:   []string{"image", "text"},
		Cost:     ModelCost{Input: 0.3, Output: 2.5, CacheRead: 0.03, CacheWrite: 0.08333333333333334},
	},
	{
		ID:       "google/gemini-3.1-flash-image-preview",
		Name:     "Google: Nano Banana 2 (Gemini 3.1 Flash Image Preview)",
		API:      "openrouter-images",
		Provider: "openrouter",
		BaseURL:  "https://openrouter.ai/api/v1",
		Input:    []string{"image", "text"},
		Output:   []string{"image", "text"},
		Cost:     ModelCost{Input: 0.5, Output: 3},
	},
}

func init() {
	for _, model := range builtinImageModelCatalog {
		RegisterImageModel(model)
	}
}

func RegisterImageModel(model ImagesModel) {
	imageModelRegistry.Lock()
	defer imageModelRegistry.Unlock()
	byProvider := imageModelRegistry.models[model.Provider]
	if byProvider == nil {
		byProvider = map[string]ImagesModel{}
		imageModelRegistry.models[model.Provider] = byProvider
		imageModelRegistry.providerOrder = append(
			imageModelRegistry.providerOrder,
			model.Provider,
		)
	}
	if _, exists := byProvider[model.ID]; !exists {
		imageModelRegistry.modelOrder[model.Provider] = append(
			imageModelRegistry.modelOrder[model.Provider],
			model.ID,
		)
	}
	byProvider[model.ID] = cloneImagesModel(model)
}

func GetImageModel(provider, id string) (ImagesModel, bool) {
	imageModelRegistry.RLock()
	defer imageModelRegistry.RUnlock()
	model, ok := imageModelRegistry.models[provider][id]
	return cloneImagesModel(model), ok
}

func MustGetImageModel(provider, id string) ImagesModel {
	model, ok := GetImageModel(provider, id)
	if !ok {
		panic("unknown image model: " + provider + "/" + id)
	}
	return model
}

func GetImageProviders() []string {
	imageModelRegistry.RLock()
	defer imageModelRegistry.RUnlock()
	return append([]string(nil), imageModelRegistry.providerOrder...)
}

func GetImageModels(provider string) []ImagesModel {
	imageModelRegistry.RLock()
	defer imageModelRegistry.RUnlock()
	byProvider := imageModelRegistry.models[provider]
	models := make([]ImagesModel, 0, len(byProvider))
	for _, modelID := range imageModelRegistry.modelOrder[provider] {
		models = append(models, cloneImagesModel(byProvider[modelID]))
	}
	return models
}

func getBuiltinImageModels(provider string) []ImagesModel {
	models := make([]ImagesModel, 0, len(builtinImageModelCatalog))
	for _, model := range builtinImageModelCatalog {
		if model.Provider == provider {
			models = append(models, cloneImagesModel(model))
		}
	}
	return models
}
