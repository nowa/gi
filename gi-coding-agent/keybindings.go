package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type KeybindingsConfig map[string]any

var keybindingNameMigrations = map[string]string{
	"cursorUp":                 "tui.editor.cursorUp",
	"cursorDown":               "tui.editor.cursorDown",
	"cursorLeft":               "tui.editor.cursorLeft",
	"cursorRight":              "tui.editor.cursorRight",
	"cursorWordLeft":           "tui.editor.cursorWordLeft",
	"cursorWordRight":          "tui.editor.cursorWordRight",
	"cursorLineStart":          "tui.editor.cursorLineStart",
	"cursorLineEnd":            "tui.editor.cursorLineEnd",
	"jumpForward":              "tui.editor.jumpForward",
	"jumpBackward":             "tui.editor.jumpBackward",
	"pageUp":                   "tui.editor.pageUp",
	"pageDown":                 "tui.editor.pageDown",
	"deleteCharBackward":       "tui.editor.deleteCharBackward",
	"deleteCharForward":        "tui.editor.deleteCharForward",
	"deleteWordBackward":       "tui.editor.deleteWordBackward",
	"deleteWordForward":        "tui.editor.deleteWordForward",
	"deleteToLineStart":        "tui.editor.deleteToLineStart",
	"deleteToLineEnd":          "tui.editor.deleteToLineEnd",
	"yank":                     "tui.editor.yank",
	"yankPop":                  "tui.editor.yankPop",
	"undo":                     "tui.editor.undo",
	"newLine":                  "tui.input.newLine",
	"submit":                   "tui.input.submit",
	"tab":                      "tui.input.tab",
	"copy":                     "tui.input.copy",
	"selectUp":                 "tui.select.up",
	"selectDown":               "tui.select.down",
	"selectPageUp":             "tui.select.pageUp",
	"selectPageDown":           "tui.select.pageDown",
	"selectConfirm":            "tui.select.confirm",
	"selectCancel":             "tui.select.cancel",
	"interrupt":                "app.interrupt",
	"clear":                    "app.clear",
	"exit":                     "app.exit",
	"suspend":                  "app.suspend",
	"cycleThinkingLevel":       "app.thinking.cycle",
	"cycleModelForward":        "app.model.cycleForward",
	"cycleModelBackward":       "app.model.cycleBackward",
	"selectModel":              "app.model.select",
	"expandTools":              "app.tools.expand",
	"toggleThinking":           "app.thinking.toggle",
	"toggleSessionNamedFilter": "app.session.toggleNamedFilter",
	"externalEditor":           "app.editor.external",
	"followUp":                 "app.message.followUp",
	"dequeue":                  "app.message.dequeue",
	"pasteImage":               "app.clipboard.pasteImage",
	"newSession":               "app.session.new",
	"tree":                     "app.session.tree",
	"fork":                     "app.session.fork",
	"resume":                   "app.session.resume",
	"treeFoldOrUp":             "app.tree.foldOrUp",
	"treeUnfoldOrDown":         "app.tree.unfoldOrDown",
	"treeEditLabel":            "app.tree.editLabel",
	"treeToggleLabelTimestamp": "app.tree.toggleLabelTimestamp",
	"toggleSessionPath":        "app.session.togglePath",
	"toggleSessionSort":        "app.session.toggleSort",
	"renameSession":            "app.session.rename",
	"deleteSession":            "app.session.delete",
	"deleteSessionNoninvasive": "app.session.deleteNoninvasive",
}

func MigrateKeybindingsConfig(rawConfig map[string]any) (map[string]any, bool) {
	config := map[string]any{}
	migrated := false
	for key, value := range rawConfig {
		nextKey := key
		if mapped, ok := keybindingNameMigrations[key]; ok {
			nextKey = mapped
		}
		if nextKey != key {
			migrated = true
		}
		if key != nextKey {
			if _, exists := rawConfig[nextKey]; exists {
				migrated = true
				continue
			}
		}
		config[nextKey] = cloneSettingsValue(value)
	}
	return config, migrated
}

func RunMigrations(agentDir string) error {
	configPath := filepath.Join(agentDir, "keybindings.json")
	rawConfig, err := loadKeybindingsRawConfig(configPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	migratedConfig, migrated := MigrateKeybindingsConfig(rawConfig)
	if !migrated {
		return nil
	}
	content, err := json.MarshalIndent(migratedConfig, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, append(content, '\n'), 0o644)
}

type KeybindingsManager struct {
	configPath   string
	userBindings KeybindingsConfig
}

func NewKeybindingsManager(agentDir string) *KeybindingsManager {
	configPath := filepath.Join(agentDir, "keybindings.json")
	manager := &KeybindingsManager{configPath: configPath, userBindings: KeybindingsConfig{}}
	manager.Reload()
	return manager
}

func (m *KeybindingsManager) Reload() {
	rawConfig, err := loadKeybindingsRawConfig(m.configPath)
	if err != nil {
		m.userBindings = KeybindingsConfig{}
		return
	}
	migratedConfig, _ := MigrateKeybindingsConfig(rawConfig)
	m.userBindings = toKeybindingsConfig(migratedConfig)
}

func (m *KeybindingsManager) GetUserBindings() KeybindingsConfig {
	return cloneKeybindingsConfig(m.userBindings)
}

func (m *KeybindingsManager) GetEffectiveConfig() KeybindingsConfig {
	return cloneKeybindingsConfig(m.userBindings)
}

func loadKeybindingsRawConfig(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return map[string]any{}, nil
	}
	return raw, nil
}

func toKeybindingsConfig(raw map[string]any) KeybindingsConfig {
	config := KeybindingsConfig{}
	for key, value := range raw {
		switch typed := value.(type) {
		case string:
			config[key] = typed
		case []any:
			values := make([]any, 0, len(typed))
			valid := true
			for _, entry := range typed {
				if _, ok := entry.(string); !ok {
					valid = false
					break
				}
				values = append(values, entry)
			}
			if valid {
				config[key] = values
			}
		}
	}
	return config
}

func cloneKeybindingsConfig(input KeybindingsConfig) KeybindingsConfig {
	output := KeybindingsConfig{}
	for key, value := range input {
		output[key] = cloneSettingsValue(value)
	}
	return output
}
