package gicodingagent

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	setTUIThemeForTest(t, "dark", nil)
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

func TestModelSelectorRowsPadAfterThemeResetLikePiText(t *testing.T) {
	setTUIThemeForTest(t, "dark", nil)
	models := make([]llm.Model, 0, 11)
	for i := 0; i < 11; i++ {
		models = append(models, llm.Model{Provider: "faux", ID: "model-" + string(rune('a'+i)), Name: "Model"})
	}
	selector := NewInteractiveModelSelectorComponent(ModelSelectorConfig{
		CurrentModel: models[0],
		AllModels:    models,
	}, ModelSelectorCallbacks{})

	const width = 80
	lines := selector.Render(width)
	var lastModelLine string
	for _, line := range lines {
		if strings.Contains(line, "model-j") {
			lastModelLine = line
			break
		}
	}
	if lastModelLine == "" {
		t.Fatalf("rendered selector missing last visible model line:\n%q", strings.Join(lines, "\n"))
	}
	if !strings.Contains(lastModelLine, "[faux]"+tuiResetFG+" ") {
		t.Fatalf("model line should reset foreground before right padding like Pi Text:\n%q", lastModelLine)
	}
	if got := len(StripAnsi(lastModelLine)); got != width {
		t.Fatalf("model line visible width = %d, want %d: %q", got, width, lastModelLine)
	}
}

func TestModelSelectorTreatsTypedNilRuntimeAsStaticCatalog(t *testing.T) {
	model := llm.Model{
		Provider: "openai",
		ID:       "gpt-static",
		Name:     "Static",
	}
	var runtime *ModelRuntime
	selector := NewInteractiveModelSelectorComponent(
		ModelSelectorConfig{
			CurrentModel: model,
			AllModels:    []llm.Model{model},
			Runtime:      runtime,
		},
		ModelSelectorCallbacks{},
	)

	rendered := StripAnsi(strings.Join(selector.Render(100), "\n"))
	if !strings.Contains(rendered, "gpt-static") {
		t.Fatalf("static selector lost its model:\n%s", rendered)
	}
	if strings.Contains(rendered, "Refreshing model catalogs") {
		t.Fatalf("typed nil runtime started a refresh:\n%s", rendered)
	}
	select {
	case <-selector.refreshDone:
	default:
		t.Fatal("typed nil runtime left an active refresh lifecycle")
	}
}

func TestModelSelectorLoadsCachedSnapshotThenPublishesRefresh(t *testing.T) {
	current := llm.Model{
		Provider: "openai",
		ID:       "gpt-current",
		Name:     "Cached current",
	}
	other := llm.Model{
		Provider: "anthropic",
		ID:       "claude-other",
		Name:     "Cached other",
	}
	refreshedCurrent := current
	refreshedCurrent.Name = "Fresh current"
	refreshedOther := other
	refreshedOther.Name = "Fresh other"
	added := llm.Model{
		Provider: "openai",
		ID:       "gpt-added",
		Name:     "Fresh addition",
	}
	runtime := newModelSelectorRuntimeStub(
		[]llm.Model{other, current},
		[]llm.Model{refreshedOther, refreshedCurrent, added},
	)
	var renderRequests atomic.Int32
	selector := NewInteractiveModelSelectorComponent(
		ModelSelectorConfig{
			CurrentModel: current,
			AllModels: []llm.Model{{
				Provider: "stale",
				ID:       "stale",
			}},
			ScopedModels: []ScopedModel{
				{
					Model:         other,
					ThinkingLevel: ThinkingLevel("high"),
				},
				{
					Model:         current,
					ThinkingLevel: ThinkingLevel("xhigh"),
				},
			},
			InitialSearch: "current",
			Runtime:       runtime,
			RefreshOptions: ModelRegistryRefreshOptions{
				AllowNetwork: true,
				Timeout:      time.Second,
			},
			RequestRender: func() {
				renderRequests.Add(1)
			},
		},
		ModelSelectorCallbacks{},
	)
	t.Cleanup(selector.Close)

	waitModelSelectorSignal(t, runtime.refreshStarted, "refresh start")
	rendered := StripAnsi(strings.Join(selector.Render(100), "\n"))
	for _, expected := range []string{
		"gpt-current",
		"Cached current",
		"Refreshing model catalogs…",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf(
				"cached selector missing %q before refresh:\n%s",
				expected,
				rendered,
			)
		}
	}
	if strings.Contains(rendered, "stale") {
		t.Fatalf(
			"selector retained constructor catalog instead of runtime snapshot:\n%s",
			rendered,
		)
	}

	close(runtime.refreshRelease)
	waitModelSelectorSignal(t, selector.refreshDone, "refresh completion")

	rendered = StripAnsi(strings.Join(selector.Render(100), "\n"))
	for _, expected := range []string{
		"Fresh current",
		"Model catalogs refreshed.",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf(
				"refreshed selector missing %q:\n%s",
				expected,
				rendered,
			)
		}
	}
	if strings.Contains(rendered, "Cached current") {
		t.Fatalf("selector retained stale model metadata:\n%s", rendered)
	}

	selector.mu.RLock()
	scoped := append([]ScopedModel(nil), selector.scopedModels...)
	allModels := append([]llm.Model(nil), selector.allModels...)
	selector.mu.RUnlock()
	if got, want := scoped[0].ThinkingLevel, ThinkingLevel("high"); got != want {
		t.Fatalf("first scoped thinking = %q, want %q", got, want)
	}
	if got, want := scoped[1].ThinkingLevel, ThinkingLevel("xhigh"); got != want {
		t.Fatalf("second scoped thinking = %q, want %q", got, want)
	}
	if got, want := scoped[1].Model.Name, "Fresh current"; got != want {
		t.Fatalf("scoped model name = %q, want %q", got, want)
	}
	if len(allModels) != 3 {
		t.Fatalf("all model count = %d, want 3", len(allModels))
	}
	if got := renderRequests.Load(); got != 2 {
		t.Fatalf("render requests = %d, want initial and refreshed", got)
	}
	select {
	case options := <-runtime.refreshOptions:
		if !options.AllowNetwork || options.Timeout != time.Second {
			t.Fatalf("refresh options = %#v", options)
		}
	default:
		t.Fatal("selector did not pass refresh options")
	}
}

func TestModelSelectorReportsRefreshOutcomes(t *testing.T) {
	model := llm.Model{
		Provider: "openai",
		ID:       "gpt-current",
		Name:     "Current",
	}
	tests := []struct {
		name         string
		result       llm.ModelsRefreshResult
		refreshError error
		runtimeError string
		want         string
	}{
		{
			name: "one provider",
			result: llm.ModelsRefreshResult{Errors: map[string]error{
				"openai": errors.New("offline"),
			}},
			want: "Could not refresh openai; showing cached models.",
		},
		{
			name: "multiple providers",
			result: llm.ModelsRefreshResult{Errors: map[string]error{
				"openai":    errors.New("offline"),
				"anthropic": errors.New("offline"),
			}},
			want: "Could not refresh 2 model catalogs; showing cached models.",
		},
		{
			name:         "runtime diagnostic",
			runtimeError: "models.json: invalid provider",
			want:         "models.json: invalid provider",
		},
		{
			name:         "refresh failure",
			refreshError: errors.New("reload failed"),
			want:         "Could not refresh model catalogs; showing cached models.",
		},
		{
			name: "success",
			want: "Model catalogs refreshed.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := newModelSelectorRuntimeStub(
				[]llm.Model{model},
				[]llm.Model{model},
			)
			runtime.refreshResult = test.result
			runtime.refreshError = test.refreshError
			runtime.runtimeError = test.runtimeError
			close(runtime.refreshRelease)
			selector := NewInteractiveModelSelectorComponent(
				ModelSelectorConfig{
					CurrentModel: model,
					Runtime:      runtime,
				},
				ModelSelectorCallbacks{},
			)
			t.Cleanup(selector.Close)
			waitModelSelectorSignal(
				t,
				selector.refreshDone,
				"refresh completion",
			)

			rendered := StripAnsi(
				strings.Join(selector.Render(100), "\n"),
			)
			if !strings.Contains(rendered, test.want) {
				t.Fatalf(
					"selector missing refresh outcome %q:\n%s",
					test.want,
					rendered,
				)
			}
			if strings.Contains(
				rendered,
				"Refreshing model catalogs…",
			) {
				t.Fatalf(
					"selector retained in-progress status:\n%s",
					rendered,
				)
			}
		})
	}
}

func TestModelSelectorTimesOutAndKeepsCachedModels(t *testing.T) {
	model := llm.Model{
		Provider: "openai",
		ID:       "gpt-cached",
		Name:     "Cached",
	}
	runtime := newModelSelectorRuntimeStub(
		[]llm.Model{model},
		nil,
	)
	selector := NewInteractiveModelSelectorComponent(
		ModelSelectorConfig{
			CurrentModel: model,
			Runtime:      runtime,
			RefreshOptions: ModelRegistryRefreshOptions{
				Timeout: 10 * time.Millisecond,
			},
		},
		ModelSelectorCallbacks{},
	)
	t.Cleanup(selector.Close)
	waitModelSelectorSignal(t, selector.refreshDone, "refresh timeout")

	rendered := StripAnsi(strings.Join(selector.Render(100), "\n"))
	for _, expected := range []string{
		"gpt-cached",
		"Model refresh timed out; showing cached models.",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf(
				"timed-out selector missing %q:\n%s",
				expected,
				rendered,
			)
		}
	}
	waitModelSelectorSignal(
		t,
		runtime.refreshCanceled,
		"refresh cancellation",
	)
}

func TestModelSelectorSelectionClosesRefreshBeforeCallback(t *testing.T) {
	model := llm.Model{
		Provider: "openai",
		ID:       "gpt-current",
		Name:     "Current",
	}
	runtime := newModelSelectorRuntimeStub(
		[]llm.Model{model},
		nil,
	)
	var selected llm.Model
	selector := NewInteractiveModelSelectorComponent(
		ModelSelectorConfig{
			CurrentModel: model,
			Runtime:      runtime,
		},
		ModelSelectorCallbacks{
			OnSelect: func(model llm.Model) {
				selected = model
				select {
				case refreshContext := <-runtime.refreshContexts:
					select {
					case <-refreshContext.Done():
					default:
						t.Fatal(
							"refresh context was not cancelled before selection callback",
						)
					}
				default:
					t.Fatal("refresh did not receive a context")
				}
			},
		},
	)
	waitModelSelectorSignal(t, runtime.refreshStarted, "refresh start")

	selector.HandleInput("\r")
	selector.Close()
	waitModelSelectorSignal(t, selector.refreshDone, "refresh completion")
	if !sameModel(selected, model) {
		t.Fatalf("selected model = %#v, want %#v", selected, model)
	}
	selector.mu.RLock()
	closed := selector.closed
	selector.mu.RUnlock()
	if !closed {
		t.Fatal("selector remained open after selection")
	}
}

type modelSelectorRuntimeStub struct {
	mu sync.RWMutex

	snapshot      []llm.Model
	nextSnapshot  []llm.Model
	runtimeError  string
	refreshResult llm.ModelsRefreshResult
	refreshError  error

	refreshStarted  chan struct{}
	refreshRelease  chan struct{}
	refreshCanceled chan struct{}
	refreshOptions  chan ModelRegistryRefreshOptions
	refreshContexts chan context.Context
	startOnce       sync.Once
	cancelOnce      sync.Once
}

func newModelSelectorRuntimeStub(
	snapshot []llm.Model,
	nextSnapshot []llm.Model,
) *modelSelectorRuntimeStub {
	return &modelSelectorRuntimeStub{
		snapshot:        append([]llm.Model(nil), snapshot...),
		nextSnapshot:    append([]llm.Model(nil), nextSnapshot...),
		refreshStarted:  make(chan struct{}),
		refreshRelease:  make(chan struct{}),
		refreshCanceled: make(chan struct{}),
		refreshOptions:  make(chan ModelRegistryRefreshOptions, 1),
		refreshContexts: make(chan context.Context, 1),
		refreshResult:   llm.ModelsRefreshResult{Errors: map[string]error{}},
	}
}

func (r *modelSelectorRuntimeStub) GetAvailableSnapshot() []llm.Model {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]llm.Model(nil), r.snapshot...)
}

func (r *modelSelectorRuntimeStub) GetModel(
	providerID string,
	modelID string,
) (llm.Model, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, model := range r.snapshot {
		if model.Provider == providerID && model.ID == modelID {
			return model, true
		}
	}
	return llm.Model{}, false
}

func (r *modelSelectorRuntimeStub) GetError() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.runtimeError
}

func (r *modelSelectorRuntimeStub) Refresh(
	ctx context.Context,
	options ModelRegistryRefreshOptions,
) (llm.ModelsRefreshResult, error) {
	r.refreshOptions <- options
	r.refreshContexts <- ctx
	r.startOnce.Do(func() {
		close(r.refreshStarted)
	})
	select {
	case <-ctx.Done():
		r.cancelOnce.Do(func() {
			close(r.refreshCanceled)
		})
		result := r.refreshResult
		result.Aborted = true
		return result, nil
	case <-r.refreshRelease:
		r.mu.Lock()
		r.snapshot = append([]llm.Model(nil), r.nextSnapshot...)
		result := r.refreshResult
		err := r.refreshError
		r.mu.Unlock()
		return result, err
	}
}

func waitModelSelectorSignal(
	t *testing.T,
	signal <-chan struct{},
	description string,
) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}
