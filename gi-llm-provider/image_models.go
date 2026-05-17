package gillmprovider

import "sync"

var imageModelRegistry = struct {
	sync.RWMutex
	models map[string]map[string]ImagesModel
}{models: map[string]map[string]ImagesModel{}}

func init() {
	RegisterImageModel(ImagesModel{
		ID:       "black-forest-labs/flux.2-pro",
		Name:     "Black Forest Labs: FLUX.2 Pro",
		API:      "openrouter-images",
		Provider: "openrouter",
		BaseURL:  "https://openrouter.ai/api/v1",
		Input:    []string{"text", "image"},
		Output:   []string{"image"},
	})
	RegisterImageModel(ImagesModel{
		ID:       "google/gemini-2.5-flash-image",
		Name:     "Google: Nano Banana (Gemini 2.5 Flash Image)",
		API:      "openrouter-images",
		Provider: "openrouter",
		BaseURL:  "https://openrouter.ai/api/v1",
		Input:    []string{"image", "text"},
		Output:   []string{"image", "text"},
		Cost:     ModelCost{Input: 0.3, Output: 2.5, CacheRead: 0.03, CacheWrite: 0.08333333333333334},
	})
	RegisterImageModel(ImagesModel{
		ID:       "google/gemini-3.1-flash-image-preview",
		Name:     "Google: Nano Banana 2 (Gemini 3.1 Flash Image Preview)",
		API:      "openrouter-images",
		Provider: "openrouter",
		BaseURL:  "https://openrouter.ai/api/v1",
		Input:    []string{"image", "text"},
		Output:   []string{"image", "text"},
		Cost:     ModelCost{Input: 0.5, Output: 3},
	})
}

func RegisterImageModel(model ImagesModel) {
	imageModelRegistry.Lock()
	defer imageModelRegistry.Unlock()
	byProvider := imageModelRegistry.models[model.Provider]
	if byProvider == nil {
		byProvider = map[string]ImagesModel{}
		imageModelRegistry.models[model.Provider] = byProvider
	}
	byProvider[model.ID] = model
}

func GetImageModel(provider, id string) (ImagesModel, bool) {
	imageModelRegistry.RLock()
	defer imageModelRegistry.RUnlock()
	model, ok := imageModelRegistry.models[provider][id]
	return model, ok
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
	providers := make([]string, 0, len(imageModelRegistry.models))
	for provider := range imageModelRegistry.models {
		providers = append(providers, provider)
	}
	return providers
}

func GetImageModels(provider string) []ImagesModel {
	imageModelRegistry.RLock()
	defer imageModelRegistry.RUnlock()
	byProvider := imageModelRegistry.models[provider]
	models := make([]ImagesModel, 0, len(byProvider))
	for _, model := range byProvider {
		models = append(models, model)
	}
	return models
}
