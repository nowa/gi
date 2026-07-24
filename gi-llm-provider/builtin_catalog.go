package gillmprovider

// compiledBuiltinCatalog is captured immediately after generated model
// registration. Runtime compatibility registrations must not mutate the
// release-pinned catalog used to construct built-in providers.
var compiledBuiltinCatalog struct {
	providerOrder []string
	modelOrder    map[string][]string
	models        map[string]map[string]Model
}

func captureBuiltinCatalog() {
	modelRegistryMu.RLock()
	defer modelRegistryMu.RUnlock()

	compiledBuiltinCatalog.providerOrder = append(
		[]string(nil),
		modelProviderOrder...,
	)
	compiledBuiltinCatalog.modelOrder = make(
		map[string][]string,
		len(modelOrder),
	)
	compiledBuiltinCatalog.models = make(
		map[string]map[string]Model,
		len(modelRegistry),
	)
	for providerID, order := range modelOrder {
		compiledBuiltinCatalog.modelOrder[providerID] = append(
			[]string(nil),
			order...,
		)
	}
	for providerID, models := range modelRegistry {
		cloned := make(map[string]Model, len(models))
		for modelID, model := range models {
			cloned[modelID] = cloneModel(model)
		}
		compiledBuiltinCatalog.models[providerID] = cloned
	}
}

func builtinProviderIDs() []string {
	return append([]string(nil), compiledBuiltinCatalog.providerOrder...)
}

func builtinModel(providerID, modelID string) (Model, bool) {
	model, ok := compiledBuiltinCatalog.models[providerID][modelID]
	if !ok {
		return Model{}, false
	}
	return cloneModel(model), true
}

func builtinModels(providerID string) []Model {
	order := compiledBuiltinCatalog.modelOrder[providerID]
	models := compiledBuiltinCatalog.models[providerID]
	result := make([]Model, 0, len(order))
	for _, modelID := range order {
		if model, ok := models[modelID]; ok {
			result = append(result, cloneModel(model))
		}
	}
	return result
}
