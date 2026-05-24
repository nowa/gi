package gicodingagent

import (
	"reflect"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestScopedModelsSelectorReordersEnabledModels(t *testing.T) {
	models := []llm.Model{
		{Provider: "faux", ID: "faux-1", Name: "One", Reasoning: true},
		{Provider: "faux", ID: "faux-2", Name: "Two", Reasoning: true},
		{Provider: "faux", ID: "faux-3", Name: "Three", Reasoning: true},
	}
	orderedIDs := []string{"faux/faux-1", "faux/faux-2", "faux/faux-3"}
	var changes [][]string
	selector := NewScopedModelsSelectorComponent(ScopedModelsSelectorConfig{
		AllModels:       models,
		EnabledModelIDs: orderedIDs,
	}, ScopedModelsSelectorCallbacks{
		OnChange: func(enabled []string) {
			changes = append(changes, enabled)
		},
	})

	selector.HandleInput("\x1b[1;3B")

	want := [][]string{{"faux/faux-2", "faux/faux-1", "faux/faux-3"}}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("changes = %#v, want %#v", changes, want)
	}
}

func TestScopedModelsSelectorTogglesAndPersists(t *testing.T) {
	models := []llm.Model{
		{Provider: "faux", ID: "faux-1", Name: "One", Reasoning: true},
		{Provider: "faux", ID: "faux-2", Name: "Two", Reasoning: true},
	}
	var changes [][]string
	var persisted []string
	selector := NewScopedModelsSelectorComponent(ScopedModelsSelectorConfig{
		AllModels: models,
	}, ScopedModelsSelectorCallbacks{
		OnChange: func(enabled []string) {
			changes = append(changes, enabled)
		},
		OnPersist: func(enabled []string) {
			persisted = enabled
		},
	})

	if enabled := selector.EnabledModelIDs(); enabled != nil {
		t.Fatalf("enabled ids = %#v, want nil for all models enabled", enabled)
	}
	if rendered := StripAnsi(strings.Join(selector.Render(80), "\n")); !strings.Contains(rendered, "all enabled") {
		t.Fatalf("rendered selector missing all-enabled state:\n%s", rendered)
	}

	selector.HandleInput("\r")
	want := [][]string{{"faux/faux-1"}}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("changes = %#v, want %#v", changes, want)
	}
	if rendered := StripAnsi(strings.Join(selector.Render(80), "\n")); !strings.Contains(rendered, "[x]") || !strings.Contains(rendered, "[ ]") {
		t.Fatalf("rendered selector missing enabled/disabled markers:\n%s", rendered)
	}

	selector.HandleInput("\x01")
	if enabled := selector.EnabledModelIDs(); enabled != nil {
		t.Fatalf("enabled ids after all = %#v, want nil", enabled)
	}
	selector.HandleInput("\x13")
	if persisted != nil {
		t.Fatalf("persisted ids = %#v, want nil", persisted)
	}
}

func TestScopedModelsSelectorUsesPiDarkThemeAnsi(t *testing.T) {
	selector := NewScopedModelsSelectorComponent(ScopedModelsSelectorConfig{
		AllModels: []llm.Model{
			{Provider: "anthropic", ID: "claude-3-5-haiku-20241022", Name: "Claude Haiku 3.5"},
		},
	}, ScopedModelsSelectorCallbacks{})
	selector.SetFocused(true)

	rendered := strings.Join(selector.Render(80), "\n")
	for _, expected := range []string{
		"\x1b[38;2;95;135;255m" + strings.Repeat("─", 80),
		"\x1b[1m\x1b[38;2;138;190;183mModel Configuration\x1b[0m",
		"\x1b[38;2;128;128;128m [anthropic]\x1b[39m",
		"> \x1b[7m \x1b[27m",
		"\x1b[38;2;102;102;102m  enter toggle",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered selector missing ANSI %q:\n%q", expected, rendered)
		}
	}
}

func TestScopedModelsSelectorUsesEffectiveModelKeybindingsPiStyle(t *testing.T) {
	models := []llm.Model{
		{Provider: "faux", ID: "faux-1", Name: "One", Reasoning: true},
		{Provider: "faux", ID: "faux-2", Name: "Two", Reasoning: true},
		{Provider: "faux", ID: "faux-3", Name: "Three", Reasoning: true},
	}
	orderedIDs := []string{"faux/faux-1", "faux/faux-2", "faux/faux-3"}
	keybindings := mergeKeybindingsConfig(DefaultProtocolKeybindings(), KeybindingsConfig{
		"app.models.enableAll":      "x",
		"app.models.clearAll":       "z",
		"app.models.toggleProvider": "q",
		"app.models.reorderDown":    "d",
		"app.models.save":           "v",
	})
	var changes [][]string
	var persisted []string
	selector := NewScopedModelsSelectorComponent(ScopedModelsSelectorConfig{
		AllModels:       models,
		EnabledModelIDs: orderedIDs,
		Keybindings:     keybindings,
	}, ScopedModelsSelectorCallbacks{
		OnChange: func(enabled []string) {
			changes = append(changes, enabled)
		},
		OnPersist: func(enabled []string) {
			persisted = enabled
		},
	})

	selector.HandleInput("\x1b[B")
	selector.HandleInput("d")
	wantChanges := [][]string{{"faux/faux-1", "faux/faux-3", "faux/faux-2"}}
	if !reflect.DeepEqual(changes, wantChanges) {
		t.Fatalf("custom reorder changes = %#v, want %#v", changes, wantChanges)
	}

	selector.HandleInput("z")
	if enabled := selector.EnabledModelIDs(); len(enabled) != 0 {
		t.Fatalf("enabled ids after custom clear = %#v, want no enabled models", enabled)
	}
	selector.HandleInput("x")
	if enabled := selector.EnabledModelIDs(); enabled != nil {
		t.Fatalf("enabled ids after custom all = %#v, want nil", enabled)
	}
	selector.HandleInput("v")
	if persisted != nil {
		t.Fatalf("persisted ids = %#v, want nil after custom save", persisted)
	}
	rendered := StripAnsi(strings.Join(selector.Render(100), "\n"))
	for _, expected := range []string{"v to save to settings", "x all", "z clear", "q provider", "d reorder", "v save"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered selector missing %q:\n%s", expected, rendered)
		}
	}
}

func TestScopedModelsSelectorFiltersAndAppliesBulkActionsToSearchResults(t *testing.T) {
	models := []llm.Model{
		{Provider: "openai", ID: "gpt-4o-mini", Name: "GPT 4o Mini"},
		{Provider: "openai", ID: "gpt-5.1", Name: "GPT 5.1"},
		{Provider: "anthropic", ID: "claude-sonnet-4-6", Name: "Claude Sonnet"},
	}
	var changes [][]string
	selector := NewScopedModelsSelectorComponent(ScopedModelsSelectorConfig{
		AllModels: models,
	}, ScopedModelsSelectorCallbacks{
		OnChange: func(enabled []string) {
			changes = append(changes, enabled)
		},
	})

	selector.HandleInput("anth")
	rendered := StripAnsi(strings.Join(selector.Render(100), "\n"))
	if !strings.Contains(rendered, "> anth") ||
		!strings.Contains(rendered, "claude-sonnet-4-6") ||
		strings.Contains(rendered, "gpt-4o-mini") {
		t.Fatalf("filtered render did not show only Anthropic match:\n%s", rendered)
	}

	selector.HandleInput("\x18")
	wantAfterClear := []string{"openai/gpt-4o-mini", "openai/gpt-5.1"}
	if enabled := selector.EnabledModelIDs(); !reflect.DeepEqual(enabled, wantAfterClear) {
		t.Fatalf("enabled ids after filtered clear = %#v, want %#v", enabled, wantAfterClear)
	}

	selector.HandleInput("\x01")
	if enabled := selector.EnabledModelIDs(); enabled != nil {
		t.Fatalf("enabled ids after filtered enable = %#v, want nil/all-enabled", enabled)
	}
	wantChanges := [][]string{wantAfterClear, nil}
	if !reflect.DeepEqual(changes, wantChanges) {
		t.Fatalf("changes = %#v, want %#v", changes, wantChanges)
	}
}

func TestScopedModelsSelectorSearchEditingAndNoMatch(t *testing.T) {
	models := []llm.Model{
		{Provider: "openai", ID: "gpt-4o-mini", Name: "GPT 4o Mini"},
	}
	var cancelled bool
	selector := NewScopedModelsSelectorComponent(ScopedModelsSelectorConfig{
		AllModels: models,
	}, ScopedModelsSelectorCallbacks{
		OnCancel: func() {
			cancelled = true
		},
	})

	selector.HandleInput("zzz")
	if rendered := StripAnsi(strings.Join(selector.Render(80), "\n")); !strings.Contains(rendered, "No matching models") {
		t.Fatalf("rendered selector missing no-match state:\n%s", rendered)
	}

	selector.HandleInput("\x03")
	if cancelled {
		t.Fatal("ctrl+c with search text should clear search before cancelling")
	}
	if rendered := StripAnsi(strings.Join(selector.Render(80), "\n")); !strings.Contains(rendered, "gpt-4o-mini") {
		t.Fatalf("rendered selector did not restore list after ctrl+c cleared search:\n%s", rendered)
	}

	selector.HandleInput("\x03")
	if !cancelled {
		t.Fatal("ctrl+c with empty search should cancel selector")
	}
}

func TestModelSelectorSupportsScopedAllScopeToggleAndSearch(t *testing.T) {
	modelOne := llm.Model{Provider: "faux", ID: "faux-1", Name: "One", Reasoning: true}
	modelTwo := llm.Model{Provider: "faux", ID: "faux-2", Name: "Two", Reasoning: true}
	modelThree := llm.Model{Provider: "faux", ID: "faux-3", Name: "Three", Reasoning: true}
	var selected llm.Model
	selector := NewInteractiveModelSelectorComponent(ModelSelectorConfig{
		CurrentModel:  modelTwo,
		AllModels:     []llm.Model{modelOne, modelTwo, modelThree},
		ScopedModels:  []ScopedModel{{Model: modelTwo}},
		InitialSearch: "faux-1",
	}, ModelSelectorCallbacks{
		OnSelect: func(model llm.Model) {
			selected = model
		},
	})

	if rendered := StripAnsi(strings.Join(selector.Render(120), "\n")); !strings.Contains(rendered, "Scope: all | scoped") ||
		!strings.Contains(rendered, "No matching models") {
		t.Fatalf("scoped render should not include all-scope match yet:\n%s", rendered)
	}

	selector.HandleInput("\t")
	rendered := StripAnsi(strings.Join(selector.Render(120), "\n"))
	if !strings.Contains(rendered, "Scope: all | scoped") || !strings.Contains(rendered, "faux-1") {
		t.Fatalf("all-scope render missing filtered all-model match:\n%s", rendered)
	}
	selector.HandleInput("\r")
	if !sameModel(selected, modelOne) {
		t.Fatalf("selected model = %#v, want %#v", selected, modelOne)
	}
}

func TestModelSelectorPreservesScopedModelOrder(t *testing.T) {
	modelOne := llm.Model{Provider: "faux", ID: "faux-1", Name: "One", Reasoning: true}
	modelTwo := llm.Model{Provider: "faux", ID: "faux-2", Name: "Two", Reasoning: true}
	modelThree := llm.Model{Provider: "faux", ID: "faux-3", Name: "Three", Reasoning: true}
	selector := NewModelSelectorComponent(modelOne, []ScopedModel{
		{Model: modelTwo},
		{Model: modelOne},
		{Model: modelThree},
	})

	var orderedIDs []string
	for _, line := range selector.Render(120) {
		if !strings.Contains(line, "[faux]") {
			continue
		}
		line = StripAnsi(line)
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "→"))
		modelID, _, _ := strings.Cut(line, " [")
		orderedIDs = append(orderedIDs, strings.TrimSpace(modelID))
	}

	want := []string{"faux-2", "faux-1", "faux-3"}
	if !reflect.DeepEqual(orderedIDs, want) {
		t.Fatalf("ordered ids = %#v, want %#v", orderedIDs, want)
	}
}
