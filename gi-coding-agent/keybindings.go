package gicodingagent

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type KeybindingsConfig map[string]any

func DefaultClipboardPasteImageKey() string {
	if runtime.GOOS == "windows" {
		return "alt+v"
	}
	return "ctrl+v"
}

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

type MigrationResult struct {
	MigratedAuthProviders []string
	DeprecationWarnings   []string
}

func RunMigrations(agentDir string, cwd ...string) error {
	_, err := RunMigrationsWithResult(agentDir, cwd...)
	return err
}

func RunMigrationsWithResult(agentDir string, cwd ...string) (MigrationResult, error) {
	result := MigrationResult{}
	authProviders, err := migrateAuthToAuthJSON(agentDir)
	if err != nil {
		return result, err
	}
	result.MigratedAuthProviders = authProviders
	if err := migrateKeybindingsConfigFile(agentDir); err != nil {
		return result, err
	}
	if err := migrateSessionsFromAgentRoot(agentDir); err != nil {
		return result, err
	}
	if err := migrateManagedToolsToBin(agentDir); err != nil {
		return result, err
	}
	if err := migrateCommandsToPrompts(filepath.Join(agentDir, "commands"), filepath.Join(agentDir, "prompts")); err != nil {
		return result, err
	}
	if len(cwd) > 0 && cwd[0] != "" {
		projectBase := filepath.Join(cwd[0], ConfigDirName)
		if err := migrateCommandsToPrompts(filepath.Join(projectBase, "commands"), filepath.Join(projectBase, "prompts")); err != nil {
			return result, err
		}
		result.DeprecationWarnings = append(result.DeprecationWarnings, deprecatedExtensionDirWarnings(projectBase, "Project")...)
	}
	result.DeprecationWarnings = append(deprecatedExtensionDirWarnings(agentDir, "Global"), result.DeprecationWarnings...)
	return result, nil
}

func migrateAuthToAuthJSON(agentDir string) ([]string, error) {
	authPath := filepath.Join(agentDir, "auth.json")
	if _, err := os.Stat(authPath); err == nil {
		return nil, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	migrated := AuthStorageData{}
	providerSet := map[string]bool{}
	oauthPath := filepath.Join(agentDir, "oauth.json")
	if content, err := os.ReadFile(oauthPath); err == nil {
		var oauth map[string]AuthCredential
		if err := json.Unmarshal(content, &oauth); err == nil {
			for provider, credential := range oauth {
				credential.Type = "oauth"
				migrated[provider] = credential
				providerSet[provider] = true
			}
			_ = os.Rename(oauthPath, oauthPath+".migrated")
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	settingsPath := filepath.Join(agentDir, "settings.json")
	if content, err := os.ReadFile(settingsPath); err == nil {
		var settings map[string]any
		if err := json.Unmarshal(content, &settings); err == nil {
			if apiKeys, ok := settings["apiKeys"].(map[string]any); ok {
				for provider, rawKey := range apiKeys {
					key, ok := rawKey.(string)
					if !ok || migrated[provider].Type != "" {
						continue
					}
					migrated[provider] = AuthCredential{Type: "api_key", Key: key}
					providerSet[provider] = true
				}
				delete(settings, "apiKeys")
				content, err := json.MarshalIndent(settings, "", "  ")
				if err != nil {
					return nil, err
				}
				if err := os.WriteFile(settingsPath, append(content, '\n'), 0o644); err != nil {
					return nil, err
				}
			}
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(migrated) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(authPath), 0o700); err != nil {
		return nil, err
	}
	content := []byte(serializeAuthStorageData(migrated))
	if err := os.WriteFile(authPath, content, 0o600); err != nil {
		return nil, err
	}
	if err := os.Chmod(authPath, 0o600); err != nil {
		return nil, err
	}
	providers := make([]string, 0, len(providerSet))
	for provider := range providerSet {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	return providers, nil
}

func migrateKeybindingsConfigFile(agentDir string) error {
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

func migrateCommandsToPrompts(commandsDir, promptsDir string) error {
	if _, err := os.Stat(commandsDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if _, err := os.Stat(promptsDir); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(commandsDir, promptsDir)
}

func migrateSessionsFromAgentRoot(agentDir string) error {
	entries, err := os.ReadDir(agentDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		source := filepath.Join(agentDir, entry.Name())
		cwd, ok := readMigrationSessionCWD(source)
		if !ok {
			continue
		}
		targetDir := GetAgentDirSessionDir(cwd, agentDir)
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			continue
		}
		target := filepath.Join(targetDir, entry.Name())
		if _, err := os.Stat(target); err == nil {
			continue
		}
		_ = os.Rename(source, target)
	}
	return nil
}

func readMigrationSessionCWD(path string) (string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return "", false
	}
	var header FileEntry
	if err := json.Unmarshal([]byte(scanner.Text()), &header); err != nil {
		return "", false
	}
	if header.Type != "session" || header.CWD == "" {
		return "", false
	}
	return header.CWD, true
}

func migrateManagedToolsToBin(agentDir string) error {
	toolsDir := filepath.Join(agentDir, "tools")
	if _, err := os.Stat(toolsDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	binDir := filepath.Join(agentDir, "bin")
	for _, name := range []string{"fd", "rg", "fd.exe", "rg.exe"} {
		source := filepath.Join(toolsDir, name)
		if _, err := os.Stat(source); err != nil {
			continue
		}
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			continue
		}
		target := filepath.Join(binDir, name)
		if _, err := os.Stat(target); err == nil {
			_ = os.Remove(source)
			continue
		}
		_ = os.Rename(source, target)
	}
	return nil
}

func deprecatedExtensionDirWarnings(baseDir, label string) []string {
	var warnings []string
	if _, err := os.Stat(filepath.Join(baseDir, "hooks")); err == nil {
		warnings = append(warnings, label+" hooks/ directory found. Hooks have been renamed to extensions.")
	}
	toolsDir := filepath.Join(baseDir, "tools")
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return warnings
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		switch strings.ToLower(name) {
		case "fd", "rg", "fd.exe", "rg.exe":
			continue
		default:
			warnings = append(warnings, label+" tools/ directory contains custom tools. Custom tools have been merged into extensions.")
			return warnings
		}
	}
	return warnings
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
	return mergeKeybindingsConfig(DefaultProtocolKeybindings(), m.userBindings)
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

func mergeKeybindingsConfig(base, overrides KeybindingsConfig) KeybindingsConfig {
	merged := cloneKeybindingsConfig(base)
	for key, value := range overrides {
		merged[key] = cloneSettingsValue(value)
	}
	return merged
}
