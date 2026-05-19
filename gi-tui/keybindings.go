package gitui

import (
	"sort"
	"sync"
)

type KeybindingDefinition struct {
	Description string
	Default     []string
}

type Keybinding struct {
	ID          string
	Description string
	Keys        []string
}

type KeybindingConflict struct {
	Key         string
	Actions     []string
	Keybindings []string
}

type KeybindingsConfig map[string][]string
type KeybindingDefinitions map[string]KeybindingDefinition
type Keybindings map[string]Keybinding

type KeybindingsManager struct {
	mu           sync.RWMutex
	definitions  KeybindingDefinitions
	userBindings KeybindingsConfig
	bindings     map[string][]string
	conflicts    []KeybindingConflict
}

var defaultKeybindingDefinitions = KeybindingDefinitions{
	"tui.editor.cursorUp":           {Default: []string{"up"}, Description: "Move cursor up"},
	"tui.editor.cursorDown":         {Default: []string{"down"}, Description: "Move cursor down"},
	"tui.editor.cursorLeft":         {Default: []string{"left", "ctrl+b"}, Description: "Move cursor left"},
	"tui.editor.cursorRight":        {Default: []string{"right", "ctrl+f"}, Description: "Move cursor right"},
	"tui.editor.cursorWordLeft":     {Default: []string{"alt+left", "ctrl+left", "alt+b"}, Description: "Move cursor word left"},
	"tui.editor.cursorWordRight":    {Default: []string{"alt+right", "ctrl+right", "alt+f"}, Description: "Move cursor word right"},
	"tui.editor.cursorLineStart":    {Default: []string{"home", "ctrl+a"}, Description: "Move to line start"},
	"tui.editor.cursorLineEnd":      {Default: []string{"end", "ctrl+e"}, Description: "Move to line end"},
	"tui.editor.jumpForward":        {Default: []string{"ctrl+]"}, Description: "Jump forward to character"},
	"tui.editor.jumpBackward":       {Default: []string{"ctrl+alt+]"}, Description: "Jump backward to character"},
	"tui.editor.pageUp":             {Default: []string{"pageUp"}, Description: "Page up"},
	"tui.editor.pageDown":           {Default: []string{"pageDown"}, Description: "Page down"},
	"tui.editor.deleteCharBackward": {Default: []string{"backspace"}, Description: "Delete character backward"},
	"tui.editor.deleteCharForward":  {Default: []string{"delete", "ctrl+d"}, Description: "Delete character forward"},
	"tui.editor.deleteWordBackward": {Default: []string{"ctrl+w", "alt+backspace"}, Description: "Delete word backward"},
	"tui.editor.deleteWordForward":  {Default: []string{"alt+d", "alt+delete"}, Description: "Delete word forward"},
	"tui.editor.deleteToLineStart":  {Default: []string{"ctrl+u"}, Description: "Delete to line start"},
	"tui.editor.deleteToLineEnd":    {Default: []string{"ctrl+k"}, Description: "Delete to line end"},
	"tui.editor.yank":               {Default: []string{"ctrl+y"}, Description: "Yank"},
	"tui.editor.yankPop":            {Default: []string{"alt+y"}, Description: "Yank pop"},
	"tui.editor.undo":               {Default: []string{"ctrl+-"}, Description: "Undo"},
	"tui.input.newLine":             {Default: []string{"shift+enter"}, Description: "Insert newline"},
	"tui.input.submit":              {Default: []string{"enter"}, Description: "Submit input"},
	"tui.input.tab":                 {Default: []string{"tab"}, Description: "Tab / autocomplete"},
	"tui.input.copy":                {Default: []string{"ctrl+c"}, Description: "Copy selection"},
	"tui.select.up":                 {Default: []string{"up"}, Description: "Move selection up"},
	"tui.select.down":               {Default: []string{"down"}, Description: "Move selection down"},
	"tui.select.pageUp":             {Default: []string{"pageUp"}, Description: "Selection page up"},
	"tui.select.pageDown":           {Default: []string{"pageDown"}, Description: "Selection page down"},
	"tui.select.confirm":            {Default: []string{"enter"}, Description: "Confirm selection"},
	"tui.select.cancel":             {Default: []string{"escape", "ctrl+c"}, Description: "Cancel selection"},
}

var TUI_KEYBINDINGS = cloneKeybindingDefinitions(defaultKeybindingDefinitions)

var (
	keybindingsMu sync.RWMutex
	keybindings   = NewKeybindingsManager()
)

func NewKeybindingsManager(config ...KeybindingsConfig) *KeybindingsManager {
	defs := cloneKeybindingDefinitions(defaultKeybindingDefinitions)
	user := KeybindingsConfig{}
	if len(config) > 0 && config[0] != nil {
		user = cloneKeybindingsConfig(config[0])
	}
	manager := &KeybindingsManager{definitions: defs, userBindings: user}
	manager.rebuildLocked()
	return manager
}

func NewKeybindingsManagerWithDefinitions(definitions KeybindingDefinitions, userBindings ...KeybindingsConfig) *KeybindingsManager {
	defs := cloneKeybindingDefinitions(definitions)
	user := KeybindingsConfig{}
	if len(userBindings) > 0 && userBindings[0] != nil {
		user = cloneKeybindingsConfig(userBindings[0])
	}
	manager := &KeybindingsManager{definitions: defs, userBindings: user}
	manager.rebuildLocked()
	return manager
}

func GetKeybindings() *KeybindingsManager {
	keybindingsMu.RLock()
	defer keybindingsMu.RUnlock()
	return keybindings
}

func SetKeybindings(manager *KeybindingsManager) {
	if manager == nil {
		manager = NewKeybindingsManager()
	}
	keybindingsMu.Lock()
	defer keybindingsMu.Unlock()
	keybindings = manager
}

func (m *KeybindingsManager) Set(action string, keys []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.userBindings == nil {
		m.userBindings = KeybindingsConfig{}
	}
	m.userBindings[action] = normalizeKeyList(keys)
	m.rebuildLocked()
}

func (m *KeybindingsManager) Keys(action string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string(nil), m.bindings[action]...)
}

func (m *KeybindingsManager) GetKeys(action string) []string {
	return m.Keys(action)
}

func (m *KeybindingsManager) Matches(data, action string) bool {
	m.mu.RLock()
	keys := append([]string(nil), m.bindings[action]...)
	m.mu.RUnlock()
	for _, key := range keys {
		if MatchesKey(data, key) {
			return true
		}
	}
	return false
}

func (m *KeybindingsManager) Bindings() Keybindings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(Keybindings, len(m.bindings))
	for action, keys := range m.bindings {
		out[action] = Keybinding{ID: action, Keys: append([]string(nil), keys...)}
	}
	return out
}

func (m *KeybindingsManager) Conflicts() []KeybindingConflict {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]KeybindingConflict, len(m.conflicts))
	for i, conflict := range m.conflicts {
		actions := append([]string(nil), conflict.Actions...)
		out[i] = KeybindingConflict{Key: conflict.Key, Actions: actions, Keybindings: append([]string(nil), actions...)}
	}
	return out
}

func (m *KeybindingsManager) GetConflicts() []KeybindingConflict {
	return m.Conflicts()
}

func (m *KeybindingsManager) UserBindings() KeybindingsConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneKeybindingsConfig(m.userBindings)
}

func (m *KeybindingsManager) GetUserBindings() KeybindingsConfig {
	return m.UserBindings()
}

func (m *KeybindingsManager) SetUserBindings(config KeybindingsConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.userBindings = cloneKeybindingsConfig(config)
	m.rebuildLocked()
}

func (m *KeybindingsManager) Definition(action string) (KeybindingDefinition, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	def, ok := m.definitions[action]
	if !ok {
		return KeybindingDefinition{}, false
	}
	def.Default = append([]string(nil), def.Default...)
	return def, true
}

func (m *KeybindingsManager) GetDefinition(action string) (KeybindingDefinition, bool) {
	return m.Definition(action)
}

func (m *KeybindingsManager) GetResolvedBindings() KeybindingsConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneKeybindingsConfig(m.bindings)
}

func (m *KeybindingsManager) rebuildLocked() {
	m.bindings = make(map[string][]string, len(m.definitions))
	m.conflicts = nil

	claims := map[string][]string{}
	for action, keys := range m.userBindings {
		if _, ok := m.definitions[action]; !ok {
			continue
		}
		for _, key := range normalizeKeyList(keys) {
			if !containsString(claims[key], action) {
				claims[key] = append(claims[key], action)
			}
		}
	}
	for key, actions := range claims {
		if len(actions) > 1 {
			sort.Strings(actions)
			claimants := append([]string(nil), actions...)
			m.conflicts = append(m.conflicts, KeybindingConflict{
				Key:         key,
				Actions:     claimants,
				Keybindings: append([]string(nil), claimants...),
			})
		}
	}
	sort.SliceStable(m.conflicts, func(i, j int) bool { return m.conflicts[i].Key < m.conflicts[j].Key })

	for action, definition := range m.definitions {
		if keys, ok := m.userBindings[action]; ok {
			m.bindings[action] = normalizeKeyList(keys)
			continue
		}
		m.bindings[action] = normalizeKeyList(definition.Default)
	}
}

func normalizeKeyList(keys []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, key := range keys {
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func cloneKeybindingsConfig(config KeybindingsConfig) KeybindingsConfig {
	out := make(KeybindingsConfig, len(config))
	for action, keys := range config {
		out[action] = append([]string(nil), keys...)
	}
	return out
}

func cloneKeybindingDefinitions(definitions KeybindingDefinitions) KeybindingDefinitions {
	out := make(KeybindingDefinitions, len(definitions))
	for action, definition := range definitions {
		out[action] = KeybindingDefinition{
			Description: definition.Description,
			Default:     append([]string(nil), definition.Default...),
		}
	}
	return out
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
