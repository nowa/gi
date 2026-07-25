package gicodingagent

import (
	"fmt"
	"sort"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

type ScopedModelsSelectorConfig struct {
	AllModels       []llm.Model
	EnabledModelIDs []string
	Keybindings     KeybindingsConfig
}

type ScopedModelsSelectorCallbacks struct {
	OnChange  func([]string)
	OnPersist func([]string)
	OnCancel  func()
}

type ScopedModelsSelectorComponent struct {
	allModels     []llm.Model
	allIDs        []string
	enabledIDs    []string
	selectedIndex int
	search        string
	callbacks     ScopedModelsSelectorCallbacks
	keybindings   KeybindingsConfig
	focus         gitui.FocusState
	dirty         bool
}

func NewScopedModelsSelectorComponent(config ScopedModelsSelectorConfig, callbacks ScopedModelsSelectorCallbacks) *ScopedModelsSelectorComponent {
	component := &ScopedModelsSelectorComponent{
		allModels:  append([]llm.Model(nil), config.AllModels...),
		enabledIDs: append([]string(nil), config.EnabledModelIDs...),
		callbacks:  callbacks,
		keybindings: func() KeybindingsConfig {
			if config.Keybindings != nil {
				return cloneKeybindingsConfig(config.Keybindings)
			}
			return DefaultProtocolKeybindings()
		}(),
	}
	for _, model := range component.allModels {
		component.allIDs = append(component.allIDs, scopedModelFullID(model))
	}
	return component
}

func (c *ScopedModelsSelectorComponent) HandleInput(data string) {
	if c == nil {
		return
	}
	kb := gitui.GetKeybindings()
	switch {
	case kb.Matches(data, "tui.select.up"):
		c.moveSelection(-1)
	case kb.Matches(data, "tui.select.down"):
		c.moveSelection(1)
	case matchesKeybindingAction(data, c.keybindings, "app.models.reorderUp") || data == "\x1b[1;3A":
		c.moveSelectedEnabled(-1)
	case matchesKeybindingAction(data, c.keybindings, "app.models.reorderDown") || data == "\x1b[1;3B":
		c.moveSelectedEnabled(1)
	case kb.Matches(data, "tui.select.confirm"):
		c.toggleSelected()
	case matchesKeybindingAction(data, c.keybindings, "app.models.enableAll"):
		c.enableModelIDs(c.searchTargetIDs())
		c.dirty = true
		c.notifyChange()
	case matchesKeybindingAction(data, c.keybindings, "app.models.clearAll"):
		c.clearModelIDs(c.searchTargetIDs())
		c.dirty = true
		c.notifyChange()
	case matchesKeybindingAction(data, c.keybindings, "app.models.toggleProvider"):
		c.toggleSelectedProvider()
	case data == "\x03":
		if c.search != "" {
			c.search = ""
			c.selectedIndex = 0
			return
		}
		if c.callbacks.OnCancel != nil {
			c.callbacks.OnCancel()
		}
	case kb.Matches(data, "tui.select.cancel"):
		if c.callbacks.OnCancel != nil {
			c.callbacks.OnCancel()
		}
	case matchesKeybindingAction(data, c.keybindings, "app.models.save"):
		if c.callbacks.OnPersist != nil {
			c.callbacks.OnPersist(append([]string(nil), c.enabledIDs...))
		}
		c.dirty = false
	case isBackspaceInput(data):
		c.search = trimLastRune(c.search)
		c.selectedIndex = 0
	case isPrintableSearchInput(data):
		c.search += data
		c.selectedIndex = 0
	}
}

func (c *ScopedModelsSelectorComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	width = max(24, width)
	items := c.items()
	if c.selectedIndex < 0 {
		c.selectedIndex = 0
	}
	if c.selectedIndex >= len(items) && len(items) > 0 {
		c.selectedIndex = len(items) - 1
	}
	enabledCount := len(c.enabledIDs)
	countText := fmt.Sprintf("%d/%d enabled", enabledCount, len(c.allIDs))
	allEnabled := c.enabledIDs == nil
	if allEnabled {
		countText = "all enabled"
	}
	save := c.modelKeyText("app.models.save", "ctrl+s")
	lines := []string{
		tuiThemeBorder(strings.Repeat("─", width)),
		"",
		tuiThemeBoldAccent("Model Configuration"),
		tuiThemeMuted("Session-only. " + save + " to save to settings."),
		"",
		selectorSearchInputLine(c.search, c.Focused()),
		"",
	}
	if len(items) == 0 {
		if strings.TrimSpace(c.search) == "" {
			lines = append(lines, tuiThemeMuted("  No models available"))
		} else {
			lines = append(lines, tuiThemeMuted("  No matching models"))
		}
	} else {
		const maxVisible = 8
		start := max(0, min(c.selectedIndex-(maxVisible/2), len(items)-maxVisible))
		end := min(len(items), start+maxVisible)
		for index := start; index < end; index++ {
			item := items[index]
			prefix := "  "
			if index == c.selectedIndex {
				prefix = tuiThemeAccent("→ ")
			}
			status := ""
			if !allEnabled {
				status = " [ ]"
				if item.enabled {
					status = " [x]"
				}
			}
			modelID := item.model.ID
			if index == c.selectedIndex {
				modelID = tuiThemeAccent(modelID)
			}
			statusText := status
			if status == " [x]" {
				statusText = tuiThemeSuccess(status)
			} else if status == " [ ]" {
				statusText = tuiThemeDim(status)
			}
			line := prefix + modelID + tuiThemeMuted(" ["+item.model.Provider+"]") + statusText
			lines = append(lines, truncateSelectorLine(line, width))
		}
		if start > 0 || end < len(items) {
			lines = append(lines, tuiThemeMuted(fmt.Sprintf("  (%d/%d)", c.selectedIndex+1, len(items))))
		}
		if selected, ok := c.selectedItem(items); ok && strings.TrimSpace(selected.model.Name) != "" {
			lines = append(lines, "", truncateSelectorLine(tuiThemeMuted("  Model Name: "+selected.model.Name), width))
		}
	}
	lines = append(lines, "")
	footer := c.footerHint()
	status := countText
	if c.dirty {
		status += " (unsaved)"
	}
	lines = append(lines, scopedModelFooterLine(footer, status, width, c.dirty))
	lines = append(lines, tuiThemeBorder(strings.Repeat("─", width)))
	return lines
}

func scopedModelFooterLine(footer, status string, width int, dirty bool) string {
	line := "  " + footer + " · " + status
	if gitui.VisibleWidth(line) > width {
		suffix := " · " + status
		suffixWidth := gitui.VisibleWidth(suffix)
		if suffixWidth >= width {
			line = gitui.TruncateToWidth(line, width, "...")
		} else {
			line = gitui.TruncateToWidth("  "+footer, width-suffixWidth, "...") + suffix
		}
	}
	if !dirty {
		return tuiThemeDim(line)
	}
	prefix, suffix, ok := strings.Cut(line, " (unsaved)")
	if !ok {
		return tuiThemeDim(line)
	}
	return tuiThemeDim(prefix+" ") + tuiThemeWarning("(unsaved)"+suffix)
}

func (c *ScopedModelsSelectorComponent) footerHint() string {
	keybindings := DefaultProtocolKeybindings()
	if c != nil && c.keybindings != nil {
		keybindings = c.keybindings
	}
	confirm := formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.select.confirm"), false)
	all := firstNonEmptyString(formatHotkeyKeys(keybindingValueKeys(keybindings["app.models.enableAll"]), false), "a")
	clear := firstNonEmptyString(formatHotkeyKeys(keybindingValueKeys(keybindings["app.models.clearAll"]), false), "c")
	provider := firstNonEmptyString(formatHotkeyKeys(keybindingValueKeys(keybindings["app.models.toggleProvider"]), false), "p")
	reorderUp := firstNonEmptyString(formatHotkeyKeys(keybindingValueKeys(keybindings["app.models.reorderUp"]), false), "alt+up")
	reorderDown := firstNonEmptyString(formatHotkeyKeys(keybindingValueKeys(keybindings["app.models.reorderDown"]), false), "alt+down")
	save := firstNonEmptyString(formatHotkeyKeys(keybindingValueKeys(keybindings["app.models.save"]), false), "ctrl+s")
	return confirm + " toggle · " + all + " all · " + clear + " clear · " + provider + " provider · " + reorderUp + "/" + reorderDown + " reorder · " + save + " save"
}

func (c *ScopedModelsSelectorComponent) modelKeyText(action, fallback string) string {
	keybindings := DefaultProtocolKeybindings()
	if c != nil && c.keybindings != nil {
		keybindings = c.keybindings
	}
	return firstNonEmptyString(formatHotkeyKeys(keybindingValueKeys(keybindings[action]), false), fallback)
}

func (c *ScopedModelsSelectorComponent) Invalidate() {}

func (c *ScopedModelsSelectorComponent) Focused() bool {
	if c == nil {
		return false
	}
	return c.focus.Focused()
}

func (c *ScopedModelsSelectorComponent) SetFocused(focused bool) {
	if c == nil {
		return
	}
	c.focus.SetFocused(focused)
}

func (c *ScopedModelsSelectorComponent) EnabledModelIDs() []string {
	if c == nil {
		return nil
	}
	return append([]string(nil), c.enabledIDs...)
}

type scopedModelSelectorItem struct {
	id      string
	model   llm.Model
	enabled bool
}

func (c *ScopedModelsSelectorComponent) items() []scopedModelSelectorItem {
	if c == nil {
		return nil
	}
	orderedIDs := c.allIDs
	if c.enabledIDs != nil {
		enabled := map[string]bool{}
		orderedIDs = append([]string(nil), c.enabledIDs...)
		for _, id := range c.enabledIDs {
			enabled[id] = true
		}
		for _, id := range c.allIDs {
			if !enabled[id] {
				orderedIDs = append(orderedIDs, id)
			}
		}
	}
	modelsByID := map[string]llm.Model{}
	for _, model := range c.allModels {
		modelsByID[scopedModelFullID(model)] = model
	}
	items := make([]scopedModelSelectorItem, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		model, ok := modelsByID[id]
		if !ok {
			continue
		}
		items = append(items, scopedModelSelectorItem{
			id:      id,
			model:   model,
			enabled: c.enabledIDs == nil || containsString(c.enabledIDs, id),
		})
	}
	if strings.TrimSpace(c.search) != "" {
		items = gitui.FuzzyFilter(items, c.search, func(item scopedModelSelectorItem) string {
			return modelSearchText(modelSearchItemFromModel(item.model))
		})
	}
	return items
}

func (c *ScopedModelsSelectorComponent) selectedItem(items []scopedModelSelectorItem) (scopedModelSelectorItem, bool) {
	if c == nil || len(items) == 0 || c.selectedIndex < 0 || c.selectedIndex >= len(items) {
		return scopedModelSelectorItem{}, false
	}
	return items[c.selectedIndex], true
}

func (c *ScopedModelsSelectorComponent) moveSelection(delta int) {
	items := c.items()
	if len(items) == 0 {
		return
	}
	c.selectedIndex += delta
	if c.selectedIndex < 0 {
		c.selectedIndex = len(items) - 1
	}
	if c.selectedIndex >= len(items) {
		c.selectedIndex = 0
	}
}

func (c *ScopedModelsSelectorComponent) toggleSelected() {
	items := c.items()
	if len(items) == 0 || c.selectedIndex < 0 || c.selectedIndex >= len(items) {
		return
	}
	id := items[c.selectedIndex].id
	if c.enabledIDs == nil {
		c.enabledIDs = []string{id}
	} else if index := indexOfString(c.enabledIDs, id); index >= 0 {
		c.enabledIDs = append(append([]string(nil), c.enabledIDs[:index]...), c.enabledIDs[index+1:]...)
	} else {
		c.enabledIDs = append(append([]string(nil), c.enabledIDs...), id)
	}
	c.dirty = true
	c.notifyChange()
}

func (c *ScopedModelsSelectorComponent) moveSelectedEnabled(delta int) {
	items := c.items()
	if len(items) == 0 || c.enabledIDs == nil || len(c.enabledIDs) == 0 ||
		c.selectedIndex < 0 || c.selectedIndex >= len(items) {
		return
	}
	selectedID := items[c.selectedIndex].id
	currentIndex := indexOfString(c.enabledIDs, selectedID)
	if currentIndex < 0 {
		return
	}
	newIndex := currentIndex + delta
	if newIndex < 0 || newIndex >= len(c.enabledIDs) {
		return
	}
	c.enabledIDs[currentIndex], c.enabledIDs[newIndex] = c.enabledIDs[newIndex], c.enabledIDs[currentIndex]
	c.selectedIndex += delta
	if c.selectedIndex < 0 {
		c.selectedIndex = 0
	}
	c.dirty = true
	c.notifyChange()
}

func (c *ScopedModelsSelectorComponent) toggleSelectedProvider() {
	items := c.items()
	if len(items) == 0 || c.selectedIndex < 0 || c.selectedIndex >= len(items) {
		return
	}
	provider := items[c.selectedIndex].model.Provider
	var providerIDs []string
	for _, model := range c.allModels {
		if model.Provider == provider {
			providerIDs = append(providerIDs, scopedModelFullID(model))
		}
	}
	allProviderEnabled := true
	for _, id := range providerIDs {
		if c.enabledIDs != nil && !containsString(c.enabledIDs, id) {
			allProviderEnabled = false
			break
		}
	}
	if c.enabledIDs == nil {
		c.enabledIDs = append([]string(nil), c.allIDs...)
	}
	if allProviderEnabled {
		remove := map[string]bool{}
		for _, id := range providerIDs {
			remove[id] = true
		}
		var next []string
		for _, id := range c.enabledIDs {
			if !remove[id] {
				next = append(next, id)
			}
		}
		c.enabledIDs = next
	} else {
		for _, id := range providerIDs {
			if !containsString(c.enabledIDs, id) {
				c.enabledIDs = append(c.enabledIDs, id)
			}
		}
		if len(c.enabledIDs) == len(c.allIDs) {
			c.enabledIDs = nil
		}
	}
	c.dirty = true
	c.notifyChange()
}

func (c *ScopedModelsSelectorComponent) searchTargetIDs() []string {
	if c == nil || strings.TrimSpace(c.search) == "" {
		return nil
	}
	items := c.items()
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.id)
	}
	return ids
}

func (c *ScopedModelsSelectorComponent) enableModelIDs(targetIDs []string) {
	if c == nil {
		return
	}
	if targetIDs == nil {
		c.enabledIDs = nil
		return
	}
	if len(targetIDs) == 0 {
		return
	}
	if c.enabledIDs == nil {
		return
	}
	next := append([]string(nil), c.enabledIDs...)
	for _, id := range targetIDs {
		if !containsString(next, id) {
			next = append(next, id)
		}
	}
	if len(next) == len(c.allIDs) {
		c.enabledIDs = nil
		return
	}
	c.enabledIDs = next
}

func (c *ScopedModelsSelectorComponent) clearModelIDs(targetIDs []string) {
	if c == nil {
		return
	}
	if targetIDs == nil {
		c.enabledIDs = []string{}
		return
	}
	if len(targetIDs) == 0 {
		return
	}
	remove := map[string]bool{}
	for _, id := range targetIDs {
		remove[id] = true
	}
	source := c.enabledIDs
	if source == nil {
		source = c.allIDs
	}
	next := make([]string, 0, len(source))
	for _, id := range source {
		if !remove[id] {
			next = append(next, id)
		}
	}
	c.enabledIDs = next
}

func (c *ScopedModelsSelectorComponent) notifyChange() {
	if c.callbacks.OnChange != nil {
		c.callbacks.OnChange(append([]string(nil), c.enabledIDs...))
	}
}

func indexOfString(values []string, target string) int {
	for index, value := range values {
		if value == target {
			return index
		}
	}
	return -1
}

type ModelSelectorConfig struct {
	CurrentModel  llm.Model
	AllModels     []llm.Model
	ScopedModels  []ScopedModel
	InitialSearch string
	Keybindings   KeybindingsConfig
}

type ModelSelectorCallbacks struct {
	OnSelect func(llm.Model)
	OnCancel func()
}

type ModelSelectorComponent struct {
	currentModel  llm.Model
	allModels     []llm.Model
	scopedModels  []ScopedModel
	scope         string
	search        string
	selectedIndex int
	callbacks     ModelSelectorCallbacks
	keybindings   KeybindingsConfig
	focus         gitui.FocusState
}

func NewModelSelectorComponent(currentModel llm.Model, scopedModels []ScopedModel) *ModelSelectorComponent {
	allModels := make([]llm.Model, 0, len(scopedModels))
	for _, scoped := range scopedModels {
		allModels = append(allModels, scoped.Model)
	}
	return NewInteractiveModelSelectorComponent(ModelSelectorConfig{
		CurrentModel: currentModel,
		AllModels:    allModels,
		ScopedModels: scopedModels,
	}, ModelSelectorCallbacks{})
}

func NewInteractiveModelSelectorComponent(config ModelSelectorConfig, callbacks ModelSelectorCallbacks) *ModelSelectorComponent {
	scope := "all"
	if len(config.ScopedModels) > 0 {
		scope = "scoped"
	}
	component := &ModelSelectorComponent{
		currentModel: config.CurrentModel,
		allModels:    sortModelSelectorModels(config.AllModels, config.CurrentModel),
		scopedModels: append([]ScopedModel(nil), config.ScopedModels...),
		scope:        scope,
		search:       config.InitialSearch,
		callbacks:    callbacks,
		keybindings: func() KeybindingsConfig {
			if config.Keybindings != nil {
				return cloneKeybindingsConfig(config.Keybindings)
			}
			return DefaultProtocolKeybindings()
		}(),
	}
	if len(component.allModels) == 0 && len(component.scopedModels) > 0 {
		for _, scoped := range component.scopedModels {
			component.allModels = append(component.allModels, scoped.Model)
		}
	}
	return component
}

func sortModelSelectorModels(models []llm.Model, current llm.Model) []llm.Model {
	result := append([]llm.Model(nil), models...)
	sort.SliceStable(result, func(i, j int) bool {
		leftCurrent := sameModel(result[i], current)
		rightCurrent := sameModel(result[j], current)
		if leftCurrent != rightCurrent {
			return leftCurrent
		}
		return result[i].Provider < result[j].Provider
	})
	return result
}

func (c *ModelSelectorComponent) HandleInput(data string) {
	if c == nil {
		return
	}
	kb := gitui.GetKeybindings()
	switch {
	case kb.Matches(data, "tui.input.tab"):
		c.toggleScope()
	case kb.Matches(data, "tui.select.up"):
		c.moveSelection(-1)
	case kb.Matches(data, "tui.select.down"):
		c.moveSelection(1)
	case kb.Matches(data, "tui.select.confirm"):
		if selected, ok := c.selectedModel(); ok && c.callbacks.OnSelect != nil {
			c.callbacks.OnSelect(selected)
		}
	case kb.Matches(data, "tui.select.cancel"):
		if c.callbacks.OnCancel != nil {
			c.callbacks.OnCancel()
		}
	case isBackspaceInput(data):
		c.search = trimLastRune(c.search)
		c.selectedIndex = 0
	case isPrintableSearchInput(data):
		c.search += data
		c.selectedIndex = 0
	}
}

func (c *ModelSelectorComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	width = max(24, width)
	items := c.filteredItems()
	if c.selectedIndex < 0 {
		c.selectedIndex = 0
	}
	if c.selectedIndex >= len(items) && len(items) > 0 {
		c.selectedIndex = len(items) - 1
	}
	lines := []string{tuiThemeBorder(strings.Repeat("─", width)), ""}
	if len(c.scopedModels) > 0 {
		lines = append(lines, c.scopeText(), c.tabScopeHint())
	} else {
		lines = append(lines, tuiThemeWarning("Only showing models from configured providers. Use /login to add providers."))
	}
	lines = append(lines, "", selectorSearchInputLine(c.search, c.Focused()), "")
	if len(items) == 0 {
		lines = append(lines, tuiThemeMuted("  No matching models"))
	} else {
		const maxVisible = 10
		start := max(0, min(c.selectedIndex-(maxVisible/2), len(items)-maxVisible))
		end := min(len(items), start+maxVisible)
		for index := start; index < end; index++ {
			item := items[index]
			prefix := "  "
			if index == c.selectedIndex {
				prefix = tuiThemeAccent("→ ")
			}
			modelID := item.model.ID
			if index == c.selectedIndex {
				modelID = tuiThemeAccent(modelID)
			}
			line := prefix + modelID + " " + tuiThemeMuted("["+item.model.Provider+"]")
			if sameModel(c.currentModel, item.model) {
				line += tuiThemeSuccess(" ✓")
			}
			lines = append(lines, truncateSelectorLine(line, width))
		}
		if start > 0 || end < len(items) {
			lines = append(lines, tuiThemeMuted(fmt.Sprintf("  (%d/%d)", c.selectedIndex+1, len(items))))
		}
		if selected, ok := c.selectedItem(items); ok && strings.TrimSpace(selected.model.Name) != "" {
			lines = append(lines, "", truncateSelectorLine(tuiThemeMuted("  Model Name: "+selected.model.Name), width))
		}
	}
	lines = append(lines, "", tuiThemeBorder(strings.Repeat("─", width)))
	return lines
}

func (c *ModelSelectorComponent) scopeText() string {
	all := tuiThemeMuted("all")
	scoped := tuiThemeMuted("scoped")
	if c != nil && c.scope == "all" {
		all = tuiThemeAccent("all")
	}
	if c != nil && c.scope == "scoped" {
		scoped = tuiThemeAccent("scoped")
	}
	return tuiThemeMuted("Scope: ") + all + tuiThemeMuted(" | ") + scoped
}

func (c *ModelSelectorComponent) tabScopeHint() string {
	tab := firstNonEmptyString(formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.input.tab"), true), "Tab")
	return tuiThemeKeyHint(tab, "scope") + tuiThemeMuted(" (all/scoped)")
}

func (c *ModelSelectorComponent) Focused() bool {
	if c == nil {
		return false
	}
	return c.focus.Focused()
}

func (c *ModelSelectorComponent) SetFocused(focused bool) {
	if c == nil {
		return
	}
	c.focus.SetFocused(focused)
}

func (c *ModelSelectorComponent) Invalidate() {}

type modelSelectorItem struct {
	model llm.Model
}

func (c *ModelSelectorComponent) filteredItems() []modelSelectorItem {
	if c == nil {
		return nil
	}
	var models []llm.Model
	if c.scope == "scoped" && len(c.scopedModels) > 0 {
		for _, scoped := range c.scopedModels {
			models = append(models, scoped.Model)
		}
	} else {
		models = append(models, c.allModels...)
	}
	items := make([]modelSelectorItem, 0, len(models))
	for _, model := range models {
		items = append(items, modelSelectorItem{model: model})
	}
	if strings.TrimSpace(c.search) != "" {
		items = gitui.FuzzyFilter(items, c.search, func(item modelSelectorItem) string {
			return modelSelectorSearchText(modelSearchItemFromModel(item.model))
		})
	}
	return items
}

func (c *ModelSelectorComponent) selectedModel() (llm.Model, bool) {
	item, ok := c.selectedItem(c.filteredItems())
	return item.model, ok
}

func (c *ModelSelectorComponent) selectedItem(items []modelSelectorItem) (modelSelectorItem, bool) {
	if c == nil || len(items) == 0 || c.selectedIndex < 0 || c.selectedIndex >= len(items) {
		return modelSelectorItem{}, false
	}
	return items[c.selectedIndex], true
}

func (c *ModelSelectorComponent) moveSelection(delta int) {
	items := c.filteredItems()
	if len(items) == 0 {
		return
	}
	c.selectedIndex += delta
	if c.selectedIndex < 0 {
		c.selectedIndex = len(items) - 1
	}
	if c.selectedIndex >= len(items) {
		c.selectedIndex = 0
	}
}

func (c *ModelSelectorComponent) toggleScope() {
	if c == nil || len(c.scopedModels) == 0 {
		return
	}
	if c.scope == "scoped" {
		c.scope = "all"
	} else {
		c.scope = "scoped"
	}
	c.selectedIndex = 0
}

func scopedModelFullID(model llm.Model) string {
	if model.Provider == "" {
		return model.ID
	}
	return model.Provider + "/" + model.ID
}

func sameModel(a, b llm.Model) bool {
	return a.Provider == b.Provider && a.ID == b.ID
}

func truncateSelectorLine(line string, width int) string {
	line = strings.TrimRight(line, " \t\r\n")
	if width <= 0 {
		return line
	}
	if gitui.VisibleWidth(line) <= width {
		return line + strings.Repeat(" ", max(0, width-gitui.VisibleWidth(line)))
	}
	return gitui.TruncateToWidth(line, width, "...", true)
}

func selectorSearchInputLine(value string, focused bool) string {
	line := "> " + value
	if !focused {
		return line
	}
	return line + "\x1b[7m \x1b[27m"
}

func isBackspaceInput(data string) bool {
	return data == "\b" || data == "\x7f"
}

func trimLastRune(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	return string(runes[:len(runes)-1])
}

func isPrintableSearchInput(data string) bool {
	if data == "" || strings.HasPrefix(data, "\x1b") {
		return false
	}
	for _, r := range data {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
