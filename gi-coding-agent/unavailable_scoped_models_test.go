package gicodingagent

import (
	"reflect"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestScopedModelsSelectorShowsAndRemovesEnabledModelWithoutCatalogEntry(
	t *testing.T,
) {
	available := llm.Model{
		Provider: "test-provider",
		ID:       "available",
		Name:     "Available",
	}
	availableID := scopedModelFullID(available)
	unavailableID := "test-provider/unavailable"
	var changes [][]string
	var persisted [][]string
	selector := NewScopedModelsSelectorComponent(
		ScopedModelsSelectorConfig{
			AllModels: []llm.Model{available},
			EnabledModelIDs: []string{
				unavailableID,
				availableID,
			},
		},
		ScopedModelsSelectorCallbacks{
			OnChange: func(enabled []string) {
				changes = append(
					changes,
					cloneOptionalStringSlice(enabled),
				)
			},
			OnPersist: func(enabled []string) {
				persisted = append(
					persisted,
					cloneOptionalStringSlice(enabled),
				)
			},
		},
	)

	rendered := StripAnsi(strings.Join(selector.Render(100), "\n"))
	if !strings.Contains(
		rendered,
		unavailableID+" [unavailable] ✗",
	) || !strings.Contains(rendered, "1 unavailable") {
		t.Fatalf("unavailable model render = %q", rendered)
	}

	selector.HandleInput("\r")
	if !reflect.DeepEqual(changes, [][]string{{availableID}}) {
		t.Fatalf("changes = %#v", changes)
	}
	selector.HandleInput("\x13")
	if !reflect.DeepEqual(persisted, [][]string{{availableID}}) {
		t.Fatalf("persisted = %#v", persisted)
	}
}

func TestScopedModelsSelectorCarriesUnmatchedSettingsAndUnavailableSessionScope(
	t *testing.T,
) {
	unavailableIDs := []string{
		"test-provider/unavailable-one",
		"test-provider/unavailable-two",
	}
	enabled, ok := scopedModelsSelectorEnabledIDs(
		nil,
		unavailableIDs,
		nil,
	)
	if !ok || !reflect.DeepEqual(enabled, unavailableIDs) {
		t.Fatalf("settings state = %#v, %t", enabled, ok)
	}
	selector := NewScopedModelsSelectorComponent(
		ScopedModelsSelectorConfig{EnabledModelIDs: enabled},
		ScopedModelsSelectorCallbacks{},
	)
	rendered := StripAnsi(strings.Join(selector.Render(100), "\n"))
	for _, id := range unavailableIDs {
		if !strings.Contains(rendered, id+" [unavailable] ✗") {
			t.Fatalf("selector missing %q:\n%s", id, rendered)
		}
	}

	unavailable := llm.Model{
		Provider: "session-provider",
		ID:       "unavailable",
		Name:     "Unavailable",
	}
	session := &AgentSession{
		ScopedModels: []ScopedModel{{Model: unavailable}},
	}
	enabled, ok = scopedModelsSelectorEnabledIDs(session, nil, nil)
	if !ok || !reflect.DeepEqual(
		enabled,
		[]string{"session-provider/unavailable"},
	) {
		t.Fatalf("session state = %#v, %t", enabled, ok)
	}
	enabled, ok = scopedModelsSelectorEnabledIDs(
		session,
		[]string{"configured-provider/unavailable"},
		nil,
	)
	if !ok || !reflect.DeepEqual(enabled, []string{
		"session-provider/unavailable",
		"configured-provider/unavailable",
	}) {
		t.Fatalf(
			"combined session and settings state = %#v, %t",
			enabled,
			ok,
		)
	}
}

func TestScopedModelsSelectorDoesNotClearPartialScopeWhenEnabledModelIsUnavailable(
	t *testing.T,
) {
	models := []llm.Model{
		{Provider: "test-provider", ID: "one", Name: "One"},
		{Provider: "test-provider", ID: "two", Name: "Two"},
		{Provider: "test-provider", ID: "three", Name: "Three"},
	}
	session := &AgentSession{}
	setSessionScopedModelsFromEnabledIDs(
		session,
		models,
		[]string{
			"test-provider/two",
			"test-provider/one",
			"test-provider/unavailable",
		},
	)
	if len(session.ScopedModels) != 2 ||
		session.ScopedModels[0].Model.ID != "two" ||
		session.ScopedModels[1].Model.ID != "one" {
		t.Fatalf("session scope = %#v", session.ScopedModels)
	}
}
