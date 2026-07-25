package gicodingagent

import llm "github.com/nowa/gi/gi-llm-provider"

// modelSearchItem is the immutable UI search projection shared by model
// autocomplete and selectors.
type modelSearchItem struct {
	ID       string
	Provider string
	Name     string
}

func modelSearchItemFromModel(model llm.Model) modelSearchItem {
	return modelSearchItem{
		ID:       model.ID,
		Provider: model.Provider,
		Name:     model.Name,
	}
}

func modelSearchText(item modelSearchItem) string {
	text := item.ID + " " +
		item.Provider + " " +
		item.Provider + "/" + item.ID + " " +
		item.Provider + " " +
		item.ID
	return appendModelSearchName(text, item.Name)
}

// modelSelectorSearchText intentionally does not lead with the bare model ID.
// This lets provider-prefixed queries rank their exact provider before proxy
// provider IDs such as openrouter/openai/gpt-5.
func modelSelectorSearchText(item modelSearchItem) string {
	text := item.Provider + " " +
		item.Provider + "/" + item.ID + " " +
		item.Provider + " " +
		item.ID
	return appendModelSearchName(text, item.Name)
}

func appendModelSearchName(text, name string) string {
	if name == "" {
		return text
	}
	return text + " " + name
}
