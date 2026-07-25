package gicodingagent

import (
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestModelSearchTextPiContract(t *testing.T) {
	item := modelSearchItem{
		ID:       "gpt-5",
		Provider: "openai",
		Name:     "GPT 5",
	}
	if got, want := modelSearchText(item), "gpt-5 openai openai/gpt-5 openai gpt-5 GPT 5"; got != want {
		t.Fatalf("model search text = %q, want %q", got, want)
	}
	if got, want := modelSelectorSearchText(item), "openai openai/gpt-5 openai gpt-5 GPT 5"; got != want {
		t.Fatalf("model selector search text = %q, want %q", got, want)
	}

	item.Name = ""
	if got, want := modelSearchText(item), "gpt-5 openai openai/gpt-5 openai gpt-5"; got != want {
		t.Fatalf("unnamed model search text = %q, want %q", got, want)
	}
}

func TestModelSelectorSearchRanksExactProviderBeforeProxyIDPiStyle(t *testing.T) {
	proxy := llm.Model{
		Provider: "openrouter",
		ID:       "openai/gpt-5",
		Name:     "GPT 5 through OpenRouter",
	}
	direct := llm.Model{
		Provider: "openai",
		ID:       "gpt-5",
		Name:     "GPT 5",
	}
	selector := NewInteractiveModelSelectorComponent(ModelSelectorConfig{
		CurrentModel:  proxy,
		AllModels:     []llm.Model{proxy, direct},
		InitialSearch: "openai/gpt-5",
	}, ModelSelectorCallbacks{})

	items := selector.filteredItems()
	if len(items) != 2 {
		t.Fatalf("filtered models = %#v, want direct and proxy models", items)
	}
	if got := items[0].model; !sameModel(got, direct) {
		t.Fatalf("first model = %s/%s, want exact provider %s/%s", got.Provider, got.ID, direct.Provider, direct.ID)
	}
}
