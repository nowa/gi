package gicodingagent

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func (h *CLIInteractiveTUIHost) handleSettingsSlashCommand() error {
	host, err := h.newRPCSessionHost()
	if err != nil {
		return err
	}
	state := host.GetState()
	settings := h.settingsManager()
	if !h.exitAfterInitial {
		return h.handleSettingsSelectDialog(host, state, settings)
	}
	return h.renderSettingsSummary(state, settings)
}

func (h *CLIInteractiveTUIHost) renderSettingsSummary(state RPCSessionState, settings *SettingsManager) error {
	model := ""
	if state.Model != nil {
		model = state.Model.Provider + "/" + state.Model.ID
	}
	rows := []string{
		"| Setting | Value |",
		"|---|---|",
		"| Current model | " + markdownTableValue(model) + " |",
		"| Thinking | " + markdownTableValue(state.ThinkingLevel) + " |",
		"| Queue | " + markdownTableValue("steering="+state.SteeringMode+", follow-up="+state.FollowUpMode) + " |",
		"| Session | " + markdownTableValue(firstNonEmptyString(state.SessionName, state.SessionID)) + " |",
	}
	if settings != nil {
		rows = append(rows,
			"| Default provider | "+markdownTableValue(settings.GetDefaultProvider())+" |",
			"| Default model | "+markdownTableValue(settings.GetDefaultModel())+" |",
			"| Default thinking | "+markdownTableValue(settings.GetDefaultThinkingLevel())+" |",
			"| Theme | "+markdownTableValue(settingsThemeCurrentValue(settings))+" |",
			"| Session dir | "+markdownTableValue(settings.GetSessionDir())+" |",
			"| Image auto resize | "+markdownTableValue(fmt.Sprintf("%t", settings.GetImageAutoResize()))+" |",
			"| Block images | "+markdownTableValue(fmt.Sprintf("%t", settings.GetBlockImages()))+" |",
			"| Install telemetry | "+markdownTableValue(fmt.Sprintf("%t", settings.GetEnableInstallTelemetry()))+" |",
		)
	}
	h.chat.AddChild(newCLIMarkdownWithOptions("**Settings**\n\n"+strings.Join(rows, "\n"), gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}))
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) handleSettingsSelectDialog(host *RPCSessionHost, state RPCSessionState, settings *SettingsManager) error {
	if settings == nil {
		return h.renderSettingsSummary(state, settings)
	}
	if h.ui == nil {
		return errors.New("interactive TUI is not ready")
	}
	resultCh := make(chan struct{}, 1)
	currentTheme := settingsThemeCurrentValue(settings)
	list := gitui.NewSettingsList(settingsListItems(host, state, settings, h.availableThemeNames(currentTheme), settingsListItemsOptions{
		OnThemePreview: h.previewTUITheme,
	}), 10, tuiThemeSettingsList(), gitui.SettingsListOptions{
		EnableSearch: true,
		OnChange: func(id, newValue string) {
			h.applySettingsListChange(host, settings, id, newValue)
		},
		OnCancel: func() {
			select {
			case resultCh <- struct{}{}:
			default:
			}
		},
	})
	restore := h.showEditorReplacement(cliSettingsListComponent{list: list}, list)
	select {
	case <-resultCh:
		restore()
		h.clearTUIThemePreview()
		return nil
	case <-h.done:
		restore()
		h.clearTUIThemePreview()
		return nil
	}
}

type cliSettingsListComponent struct {
	list *gitui.SettingsList
}

func (c cliSettingsListComponent) Invalidate() {
	if c.list != nil {
		c.list.Invalidate()
	}
}

func (c cliSettingsListComponent) Render(width int) []string {
	width = max(1, width)
	lines := []string{tuiThemeBorder(strings.Repeat("─", width))}
	if c.list != nil {
		lines = append(lines, c.list.Render(width)...)
	}
	lines = append(lines, tuiThemeBorder(strings.Repeat("─", width)))
	return lines
}

func (c cliSettingsListComponent) HandleInput(data string) {
	if c.list != nil {
		c.list.HandleInput(data)
	}
}

type cliSettingsListDialog struct {
	title string
	list  *gitui.SettingsList
}

func (c cliSettingsListDialog) Invalidate() {
	if c.list != nil {
		c.list.Invalidate()
	}
}

func (c cliSettingsListDialog) Render(width int) []string {
	width = max(32, width)
	innerWidth := max(1, width-2)
	lines := []string{dialogBorder(width), dialogLine(firstNonEmptyString(c.title, "Settings"), innerWidth)}
	lines = append(lines, dialogLine("", innerWidth))
	if c.list != nil {
		for _, line := range c.list.Render(innerWidth) {
			lines = append(lines, dialogLine(line, innerWidth))
		}
	}
	lines = append(lines, dialogBorder(width))
	return lines
}

func (c cliSettingsListDialog) HandleInput(data string) {
	if c.list != nil {
		c.list.HandleInput(data)
	}
}

type settingsListItemsOptions struct {
	OnThemePreview func(string)
}

var settingsSubmenuSelectListLayout = gitui.SelectListLayoutOptions{
	MinPrimaryColumnWidth: 12,
	MaxPrimaryColumnWidth: 32,
}

type httpIdleTimeoutChoice struct {
	Label     string
	TimeoutMS int
}

var httpIdleTimeoutChoices = []httpIdleTimeoutChoice{
	{Label: "30 sec", TimeoutMS: 30_000},
	{Label: "1 min", TimeoutMS: 60_000},
	{Label: "2 min", TimeoutMS: 120_000},
	{Label: "5 min", TimeoutMS: 300_000},
	{Label: "disabled", TimeoutMS: 0},
}

func formatHTTPIdleTimeoutMS(timeoutMS int) string {
	for _, choice := range httpIdleTimeoutChoices {
		if choice.TimeoutMS == timeoutMS {
			return choice.Label
		}
	}
	return strconv.FormatFloat(float64(timeoutMS)/1000, 'f', -1, 64) + " sec"
}

func httpIdleTimeoutLabels() []string {
	labels := make([]string, 0, len(httpIdleTimeoutChoices))
	for _, choice := range httpIdleTimeoutChoices {
		labels = append(labels, choice.Label)
	}
	return labels
}

func httpIdleTimeoutMSForLabel(label string) (int, bool) {
	for _, choice := range httpIdleTimeoutChoices {
		if choice.Label == label {
			return choice.TimeoutMS, true
		}
	}
	return 0, false
}

func settingsFollowUpDescription() string {
	key := formatHotkeyKeys(keybindingValueKeys(DefaultProtocolKeybindings()["app.message.followUp"]), true)
	if key == "" {
		key = "Option+Enter"
	}
	return key + " queues follow-up messages until agent stops. 'one-at-a-time': deliver one, wait for response. 'all': deliver all at once."
}

func settingsListItems(host *RPCSessionHost, state RPCSessionState, settings *SettingsManager, themes []string, options ...settingsListItemsOptions) []gitui.SettingItem {
	thinkingLevels := []string{string(ThinkingOff)}
	if host != nil && host.Session != nil && host.Session.Agent != nil {
		thinkingLevels = llm.GetSupportedThinkingLevels(host.Session.Agent.State.Model)
		if len(thinkingLevels) == 0 {
			thinkingLevels = []string{string(ThinkingOff)}
		}
	}
	opts := settingsListItemsOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	currentTheme := settingsThemeCurrentValue(settings)
	themeSubmenu := settingsSelectSubmenu("Theme", "Select color theme", settingsThemeOptions(themes, currentTheme))
	if opts.OnThemePreview != nil {
		themeSubmenu = settingsSelectSubmenuWithSelectionChange(
			"Theme",
			"Select color theme",
			settingsThemeOptions(themes, currentTheme),
			opts.OnThemePreview,
		)
	}
	items := []gitui.SettingItem{
		{ID: "autocompact", Label: "Auto-compact", Description: "Automatically compact context when it gets too large", CurrentValue: fmt.Sprintf("%t", state.AutoCompactionEnabled), Values: []string{"true", "false"}},
		{ID: "auto-resize-images", Label: "Auto-resize images", Description: "Resize large images to 2000x2000 max for better model compatibility", CurrentValue: fmt.Sprintf("%t", settings.GetImageAutoResize()), Values: []string{"true", "false"}},
		{ID: "block-images", Label: "Block images", Description: "Prevent images from being sent to LLM providers", CurrentValue: fmt.Sprintf("%t", settings.GetBlockImages()), Values: []string{"false", "true"}},
		{ID: "skill-commands", Label: "Skill commands", Description: "Register skills as /skill:name commands", CurrentValue: fmt.Sprintf("%t", settings.GetEnableSkillCommands()), Values: []string{"true", "false"}},
		{ID: "show-hardware-cursor", Label: "Show hardware cursor", Description: "Show the terminal cursor while still positioning it for IME support", CurrentValue: fmt.Sprintf("%t", settings.GetShowHardwareCursor()), Values: []string{"true", "false"}},
		{ID: "editor-padding", Label: "Editor padding", Description: "Horizontal padding for input editor (0-3)", CurrentValue: fmt.Sprintf("%d", settings.GetEditorPaddingX()), Values: []string{"0", "1", "2", "3"}},
		{ID: "autocomplete-max-visible", Label: "Autocomplete max items", Description: "Max visible items in autocomplete dropdown (3-20)", CurrentValue: fmt.Sprintf("%d", settings.GetAutocompleteMaxVisible()), Values: []string{"3", "5", "7", "10", "15", "20"}},
		{ID: "clear-on-shrink", Label: "Clear on shrink", Description: "Clear empty rows when content shrinks (may cause flicker)", CurrentValue: fmt.Sprintf("%t", settings.GetClearOnShrink()), Values: []string{"true", "false"}},
		{ID: "terminal-progress", Label: "Terminal progress", Description: "Show OSC 9;4 progress indicators in the terminal tab bar", CurrentValue: fmt.Sprintf("%t", settings.GetShowTerminalProgress()), Values: []string{"true", "false"}},
		{ID: "steering-mode", Label: "Steering mode", Description: "Enter while streaming queues steering messages. 'one-at-a-time': deliver one, wait for response. 'all': deliver all at once.", CurrentValue: state.SteeringMode, Values: []string{"one-at-a-time", "all"}},
		{ID: "follow-up-mode", Label: "Follow-up mode", Description: settingsFollowUpDescription(), CurrentValue: state.FollowUpMode, Values: []string{"one-at-a-time", "all"}},
		{ID: "transport", Label: "Transport", Description: "Preferred transport for providers that support multiple transports", CurrentValue: settings.GetTransport(), Values: []string{"sse", "websocket", "websocket-cached", "auto"}},
		{ID: "http-idle-timeout", Label: "HTTP idle timeout", Description: "Maximum idle gap while waiting for HTTP headers or body chunks. Disable for local models that pause longer than five minutes.", CurrentValue: formatHTTPIdleTimeoutMS(settings.GetHTTPIdleTimeoutMS()), Values: httpIdleTimeoutLabels()},
		{ID: "hide-thinking", Label: "Hide thinking", Description: "Hide thinking blocks in assistant responses", CurrentValue: fmt.Sprintf("%t", settings.GetHideThinkingBlock()), Values: []string{"true", "false"}},
		{ID: "collapse-changelog", Label: "Collapse changelog", Description: "Show condensed changelog after updates", CurrentValue: fmt.Sprintf("%t", settings.GetCollapseChangelog()), Values: []string{"true", "false"}},
		{ID: "quiet-startup", Label: "Quiet startup", Description: "Disable verbose printing at startup", CurrentValue: fmt.Sprintf("%t", settings.GetQuietStartup()), Values: []string{"true", "false"}},
		{ID: "install-telemetry", Label: "Install telemetry", Description: "Send an anonymous version/update ping after changelog-detected updates", CurrentValue: fmt.Sprintf("%t", settings.GetEnableInstallTelemetry()), Values: []string{"true", "false"}},
		{ID: "double-escape-action", Label: "Double-escape action", Description: "Action when pressing Escape twice with empty editor", CurrentValue: settings.GetDoubleEscapeAction(), Values: []string{"tree", "fork", "none"}},
		{ID: "tree-filter-mode", Label: "Tree filter mode", Description: "Default filter when opening /tree", CurrentValue: settings.GetTreeFilterMode(), Values: []string{"default", "no-tools", "user-only", "labeled-only", "all"}},
		{ID: "warnings", Label: "Warnings", Description: "Enable or disable individual warnings", CurrentValue: "configure", Submenu: settingsWarningsSubmenu(settings)},
		{ID: "thinking", Label: "Thinking level", Description: "Reasoning depth for thinking-capable models", CurrentValue: state.ThinkingLevel, Submenu: settingsSelectSubmenu("Thinking Level", "Select reasoning depth for thinking-capable models", settingsThinkingOptions(thinkingLevels, state.ThinkingLevel))},
		{ID: "theme", Label: "Theme", Description: "Color theme for the interface", CurrentValue: currentTheme, Submenu: themeSubmenu},
	}
	if gitui.GetCapabilities().Images {
		items = insertSettingItems(items, 1,
			gitui.SettingItem{ID: "show-images", Label: "Show images", Description: "Render images inline in terminal", CurrentValue: fmt.Sprintf("%t", settings.GetShowImages()), Values: []string{"true", "false"}},
			gitui.SettingItem{ID: "image-width-cells", Label: "Image width", Description: "Preferred inline image width in terminal cells", CurrentValue: fmt.Sprintf("%d", settings.GetImageWidthCells()), Values: []string{"60", "80", "120"}},
		)
	}
	return items
}

func settingsThemeCurrentValue(settings *SettingsManager) string {
	if settings == nil {
		return "dark"
	}
	if theme := strings.TrimSpace(settings.GetTheme()); theme != "" {
		return theme
	}
	return "dark"
}

func insertSettingItems(items []gitui.SettingItem, index int, inserted ...gitui.SettingItem) []gitui.SettingItem {
	if len(inserted) == 0 {
		return items
	}
	index = min(max(index, 0), len(items))
	result := make([]gitui.SettingItem, 0, len(items)+len(inserted))
	result = append(result, items[:index]...)
	result = append(result, inserted...)
	result = append(result, items[index:]...)
	return result
}

func settingsSelectSubmenu(title, message string, options []TUIDialogOption) func(currentValue string, done func(selectedValue string, changed bool)) gitui.Component {
	return func(currentValue string, done func(selectedValue string, changed bool)) gitui.Component {
		return newCLISettingsSelectSubmenu(title, message, options, currentValue, done, nil)
	}
}

func settingsSelectSubmenuWithSelectionChange(title, message string, options []TUIDialogOption, onSelectionChange func(string)) func(currentValue string, done func(selectedValue string, changed bool)) gitui.Component {
	return func(currentValue string, done func(selectedValue string, changed bool)) gitui.Component {
		return newCLISettingsSelectSubmenu(title, message, options, currentValue, done, onSelectionChange)
	}
}

type cliSettingsSelectSubmenu struct {
	title             string
	message           string
	list              *gitui.SelectList
	currentValue      string
	done              func(selectedValue string, changed bool)
	onSelectionChange func(string)
}

func newCLISettingsSelectSubmenu(title, message string, options []TUIDialogOption, currentValue string, done func(selectedValue string, changed bool), onSelectionChange func(string)) *cliSettingsSelectSubmenu {
	items := make([]gitui.SelectItem, 0, len(options))
	for _, option := range options {
		value := dialogStringValue(dialogOptionValue(option))
		label := strings.TrimSpace(option.Label)
		if label == "" {
			label = firstNonEmptyString(option.ID, value)
		}
		items = append(items, gitui.SelectItem{Value: value, Label: label, Description: option.Description})
	}
	list := gitui.NewSelectList(items, min(len(items), 10), tuiThemeSelectList(), settingsSubmenuSelectListLayout)
	if index := settingsSelectItemIndex(items, currentValue); index >= 0 {
		list.SetSelectedIndex(index)
	}
	component := &cliSettingsSelectSubmenu{
		title:             title,
		message:           message,
		list:              list,
		currentValue:      currentValue,
		done:              done,
		onSelectionChange: onSelectionChange,
	}
	list.OnSelect = func(item gitui.SelectItem) {
		if component.done != nil {
			component.done(item.Value, true)
		}
	}
	list.OnCancel = func() {
		if component.onSelectionChange != nil {
			component.onSelectionChange(component.currentValue)
		}
		if component.done != nil {
			component.done(component.currentValue, false)
		}
	}
	list.OnSelectionChange = func(item gitui.SelectItem) {
		if component.onSelectionChange != nil {
			component.onSelectionChange(item.Value)
		}
	}
	return component
}

func settingsSelectItemIndex(items []gitui.SelectItem, currentValue string) int {
	for idx, item := range items {
		if item.Value == currentValue {
			return idx
		}
	}
	return -1
}

func (c *cliSettingsSelectSubmenu) Invalidate() {
	if c != nil && c.list != nil {
		c.list.Invalidate()
	}
}

func (c *cliSettingsSelectSubmenu) Render(width int) []string {
	if c == nil {
		return nil
	}
	width = max(1, width)
	lines := []string{tuiThemeBoldAccent(c.title)}
	if strings.TrimSpace(c.message) != "" {
		lines = append(lines, "", tuiThemeMuted(c.message))
	}
	lines = append(lines, "")
	if c.list != nil {
		lines = append(lines, c.list.Render(width)...)
	}
	lines = append(lines, "", tuiThemeDim("  Enter to select · Esc to go back"))
	return lines
}

func (c *cliSettingsSelectSubmenu) HandleInput(data string) {
	if c != nil && c.list != nil {
		c.list.HandleInput(data)
	}
}

func settingsThinkingOptions(levels []string, _ string) []TUIDialogOption {
	options := make([]TUIDialogOption, 0, len(levels))
	for _, level := range levels {
		options = append(options, TUIDialogOption{ID: level, Label: level, Description: thinkingLevelDescription(level), Value: level})
	}
	return options
}

func settingsThemeOptions(themes []string, _ string) []TUIDialogOption {
	options := make([]TUIDialogOption, 0, len(themes))
	for _, themeName := range themes {
		options = append(options, TUIDialogOption{ID: themeName, Label: themeName, Value: themeName})
	}
	return options
}

func settingsWarningsSubmenu(settings *SettingsManager) func(currentValue string, done func(selectedValue string, changed bool)) gitui.Component {
	return func(currentValue string, done func(selectedValue string, changed bool)) gitui.Component {
		currentWarnings := settings.GetWarnings()
		items := []gitui.SettingItem{
			{
				ID:           "warning-anthropic-extra-usage",
				Label:        "Anthropic extra usage",
				Description:  "Warn when Anthropic subscription auth may use paid extra usage",
				CurrentValue: fmt.Sprintf("%t", currentWarnings.AnthropicExtraUsage),
				Values:       []string{"true", "false"},
			},
		}
		return gitui.NewSettingsList(items, min(len(items), 10), tuiThemeSettingsList(), gitui.SettingsListOptions{
			OnChange: func(id, newValue string) {
				if id == "warning-anthropic-extra-usage" {
					settings.SetWarningAnthropicExtraUsage(newValue == "true")
				}
			},
			OnCancel: func() {
				done(currentValue, false)
			},
		})
	}
}

func thinkingLevelDescription(level string) string {
	switch level {
	case "off":
		return "No reasoning"
	case "minimal":
		return "Very brief reasoning (~1k tokens)"
	case "low":
		return "Light reasoning (~2k tokens)"
	case "medium":
		return "Moderate reasoning (~8k tokens)"
	case "high":
		return "Deep reasoning (~16k tokens)"
	case "xhigh":
		return "Maximum reasoning (~32k tokens)"
	default:
		return ""
	}
}

func (h *CLIInteractiveTUIHost) applySettingsListChange(host *RPCSessionHost, settings *SettingsManager, id, newValue string) {
	switch id {
	case "autocompact":
		enabled := newValue == "true"
		if err := host.SetAutoCompaction(&enabled); err != nil {
			h.addStatus("Error: " + err.Error())
		}
	case "show-images":
		settings.SetShowImages(newValue == "true")
		h.applyToolImageSettings(settings)
	case "image-width-cells":
		if width, err := strconv.Atoi(newValue); err == nil {
			settings.SetImageWidthCells(width)
			h.applyToolImageSettings(settings)
		}
	case "auto-resize-images", "image-auto-resize":
		settings.SetImageAutoResize(newValue == "true")
	case "block-images":
		settings.SetBlockImages(newValue == "true")
	case "skill-commands":
		settings.SetEnableSkillCommands(newValue == "true")
		h.refreshEditorAutocompleteProvider()
	case "show-hardware-cursor":
		settings.SetShowHardwareCursor(newValue == "true")
		if h.ui != nil {
			h.ui.SetShowHardwareCursor(settings.GetShowHardwareCursor())
		}
	case "editor-padding":
		if padding, err := strconv.Atoi(newValue); err == nil {
			settings.SetEditorPaddingX(padding)
			if h.editor != nil {
				h.editor.SetPaddingX(settings.GetEditorPaddingX())
			}
		}
	case "autocomplete-max-visible":
		if visible, err := strconv.Atoi(newValue); err == nil {
			settings.SetAutocompleteMaxVisible(visible)
			if h.editor != nil {
				h.editor.SetAutocompleteMaxVisible(settings.GetAutocompleteMaxVisible())
			}
		}
	case "clear-on-shrink":
		settings.SetClearOnShrink(newValue == "true")
		if h.ui != nil {
			h.ui.SetClearOnShrink(settings.GetClearOnShrink())
		}
	case "terminal-progress":
		settings.SetShowTerminalProgress(newValue == "true")
	case "thinking":
		if err := host.SetThinkingLevel(newValue); err != nil {
			h.addStatus("Error: " + err.Error())
		}
		h.updateEditorBorderColor()
	case "theme":
		h.clearTUIThemePreview()
		settings.SetTheme(newValue)
		h.requestRender(true)
	case "steering-mode":
		if err := host.SetSteeringMode(newValue); err != nil {
			h.addStatus("Error: " + err.Error())
		}
	case "follow-up-mode":
		if err := host.SetFollowUpMode(newValue); err != nil {
			h.addStatus("Error: " + err.Error())
		}
	case "transport":
		settings.SetTransport(newValue)
	case "http-idle-timeout":
		if timeoutMS, ok := httpIdleTimeoutMSForLabel(newValue); ok {
			settings.SetHTTPIdleTimeoutMS(timeoutMS)
		}
	case "hide-thinking":
		settings.SetHideThinkingBlock(newValue == "true")
		h.updateAssistantThinkingPresentation()
	case "collapse-changelog":
		settings.SetCollapseChangelog(newValue == "true")
	case "quiet-startup":
		settings.SetQuietStartup(newValue == "true")
	case "install-telemetry":
		settings.SetEnableInstallTelemetry(newValue == "true")
	case "double-escape-action":
		settings.SetDoubleEscapeAction(newValue)
	case "tree-filter-mode":
		settings.SetTreeFilterMode(newValue)
	case "warning-anthropic-extra-usage":
		settings.SetWarningAnthropicExtraUsage(newValue == "true")
	}
}

func (h *CLIInteractiveTUIHost) applyToolImageSettings(settings *SettingsManager) {
	if h == nil || settings == nil {
		return
	}
	apply := func(component *ToolExecutionComponent) {
		if component == nil {
			return
		}
		component.SetShowImages(settings.GetShowImages())
		component.SetImageWidthCells(settings.GetImageWidthCells())
	}
	for _, component := range h.pendingTools {
		apply(component)
	}
	if h.chat != nil {
		for _, child := range h.chat.Children() {
			if component, ok := child.(*ToolExecutionComponent); ok {
				apply(component)
			}
		}
	}
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) packageResourceManager() (*DefaultPackageManager, error) {
	settings := h.settingsManager()
	if settings == nil {
		return nil, errors.New("package resource selector requires settings")
	}
	return NewDefaultPackageManager(PackageManagerOptions{
		CWD:             h.interactiveCWD(),
		AgentDir:        settings.agentDir,
		SettingsManager: settings,
	}), nil
}

func packageResourceSettingItems(resources []PackageResourceToggleItem) ([]gitui.SettingItem, map[string]resourceToggleSelection) {
	items := make([]gitui.SettingItem, 0, len(resources))
	toggles := make(map[string]resourceToggleSelection, len(resources))
	for _, resource := range resources {
		id := packageResourceSettingID(resource)
		state := "disabled"
		if resource.Enabled {
			state = "enabled"
		}
		items = append(items, gitui.SettingItem{
			ID:           id,
			Label:        packageResourceLabel(resource),
			Description:  packageResourceDescription(resource),
			CurrentValue: state,
			Values:       []string{"enabled", "disabled"},
		})
		if resource.Metadata.Origin == "top-level" {
			toggles[id] = resourceToggleSelection{TopLevel: TopLevelResourceToggle{
				Scope:        resource.Scope,
				ResourceType: resource.ResourceType,
				Pattern:      resource.Pattern,
				Enabled:      resource.Enabled,
			}}
		} else {
			toggles[id] = resourceToggleSelection{Package: PackageResourceToggle{
				Source:       resource.Source,
				Scope:        resource.Scope,
				ResourceType: resource.ResourceType,
				Pattern:      resource.Pattern,
				Enabled:      resource.Enabled,
			}}
		}
	}
	return items, toggles
}

type resourceToggleSelection struct {
	Package  PackageResourceToggle
	TopLevel TopLevelResourceToggle
}

func applyResourceToggle(manager *DefaultPackageManager, toggle resourceToggleSelection, enabled bool) (bool, error) {
	if toggle.TopLevel.Pattern != "" {
		toggle.TopLevel.Enabled = enabled
		return manager.SetTopLevelResourceEnabled(toggle.TopLevel)
	}
	toggle.Package.Enabled = enabled
	return manager.SetPackageResourceEnabled(toggle.Package)
}

func packageResourceSettingID(resource PackageResourceToggleItem) string {
	return strings.Join([]string{resource.Scope, resource.Source, resource.ResourceType, resource.Pattern}, "\x1f")
}

func packageResourceLabel(resource PackageResourceToggleItem) string {
	label := strings.TrimSpace(resource.DisplayName)
	if label == "" {
		label = strings.TrimSpace(resource.Pattern)
	}
	return strings.TrimSpace(packageResourceTypeLabel(resource.ResourceType) + " " + label)
}

func packageResourceTypeLabel(resourceType string) string {
	switch resourceType {
	case "extensions":
		return "Extension"
	case "skills":
		return "Skill"
	case "prompts":
		return "Prompt"
	case "themes":
		return "Theme"
	default:
		return strings.TrimSpace(resourceType)
	}
}

func packageResourceDescription(resource PackageResourceToggleItem) string {
	parts := []string{
		"Source: " + packageResourceSourceLabel(resource),
		"Scope: " + firstNonEmptyString(resource.Scope, "user"),
		"Pattern: " + resource.Pattern,
	}
	if resource.Path != "" {
		parts = append(parts, "Path: "+resource.Path)
	}
	return strings.Join(parts, " · ")
}

func packageResourceSourceLabel(resource PackageResourceToggleItem) string {
	if resource.Metadata.Origin == "top-level" {
		if resource.Scope == "project" {
			return "Project .gi"
		}
		return "User agent"
	}
	return resource.Source
}

func (h *CLIInteractiveTUIHost) availableThemeNames(current string) []string {
	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, theme := range h.AvailableTUIThemes() {
		add(theme.Name)
	}
	add(current)
	return names
}
