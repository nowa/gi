package gicodingagent

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	gitui "github.com/nowa/gi/gi-tui"
)

type AuthSelectorProvider struct {
	ID       string
	Name     string
	AuthType string
}

type AuthStatusResolver func(providerID string) AuthStatus

type OAuthSelector struct {
	Mode           string
	AuthStorage    *AuthStorage
	Providers      []AuthSelectorProvider
	StatusResolver AuthStatusResolver
}

type OAuthSelectorComponent struct {
	focus    gitui.FocusState
	selector OAuthSelector
	query    string
	filtered []AuthSelectorProvider
	selected int
	OnSelect func(providerID string)
	OnCancel func()
}

func IsAPIKeyLoginProvider(providerID string, oauthProviderIDs, builtInProviderIDs map[string]bool) bool {
	if providerID == "github-copilot" && oauthProviderIDs[providerID] {
		return false
	}
	if builtInProviderDisplayNames[providerID] != "" {
		return true
	}
	if builtInProviderIDs[providerID] {
		return false
	}
	return !oauthProviderIDs[providerID]
}

func (s OAuthSelector) Render(width int) []string {
	title := "Select provider to configure:"
	if s.Mode == "logout" {
		title = "Select provider to logout:"
	}
	lines := []string{title, ""}
	if len(s.Providers) == 0 {
		if s.Mode == "logout" {
			return append(lines, "  No providers logged in. Use /login first.")
		}
		return append(lines, "  No providers available")
	}
	for i, provider := range s.Providers {
		prefix := "  "
		if i == 0 {
			prefix = "-> "
		}
		lines = append(lines, prefix+provider.Name+s.statusIndicator(provider))
	}
	return lines
}

func (s OAuthSelector) statusIndicator(provider AuthSelectorProvider) string {
	if s.AuthStorage != nil {
		if credential, ok := s.AuthStorage.Get(provider.ID); ok {
			if credential.Type == provider.AuthType {
				return " ✓ configured"
			}
			if credential.Type == "oauth" {
				return " • subscription configured"
			}
			return " • API key configured"
		}
	}
	if provider.AuthType != "api_key" {
		return " • unconfigured"
	}
	status := AuthStatus{}
	if s.StatusResolver != nil {
		status = s.StatusResolver(provider.ID)
	} else if s.AuthStorage != nil {
		status = s.AuthStorage.GetAuthStatus(provider.ID)
	}
	switch status.Source {
	case "environment":
		label := status.Label
		if label == "" {
			label = "API key"
		}
		return " ✓ env: " + label
	case "runtime":
		return " ✓ runtime API key"
	case "fallback":
		return " ✓ custom API key"
	case "models_json_key":
		return " ✓ key in models.json"
	case "models_json_command":
		return " ✓ command in models.json"
	default:
		return " • unconfigured"
	}
}

func NewOAuthSelectorComponent(selector OAuthSelector) *OAuthSelectorComponent {
	component := &OAuthSelectorComponent{selector: selector}
	component.filter()
	return component
}

func (c *OAuthSelectorComponent) Focused() bool {
	if c == nil {
		return false
	}
	return c.focus.Focused()
}

func (c *OAuthSelectorComponent) SetFocused(focused bool) {
	if c != nil {
		c.focus.SetFocused(focused)
	}
}

func (c *OAuthSelectorComponent) Invalidate() {}

func (c *OAuthSelectorComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	width = max(32, width)
	title := "Select provider to configure:"
	if c.selector.Mode == "logout" {
		title = "Select provider to logout:"
	}
	lines := []string{
		"",
		authSelectorBorder(width),
		gitui.TruncateToWidth("  "+title, width, "", true),
		gitui.TruncateToWidth("  Search: "+c.query, width, "", true),
		gitui.TruncateToWidth(c.footerHint(), width, "", true),
		authSelectorBorder(width),
		"",
	}
	if len(c.filtered) == 0 {
		message := "No matching providers"
		if len(c.selector.Providers) == 0 {
			if c.selector.Mode == "logout" {
				message = "No providers logged in. Use /login first."
			} else {
				message = "No providers available"
			}
		}
		lines = append(lines, gitui.TruncateToWidth("  "+message, width, "", true))
		return append(lines, "", authSelectorBorder(width))
	}
	start := max(0, min(c.selected-4, max(0, len(c.filtered)-8)))
	end := min(len(c.filtered), start+8)
	for index := start; index < end; index++ {
		provider := c.filtered[index]
		prefix := "  "
		if index == c.selected {
			prefix = "> "
		}
		line := prefix + provider.Name + c.selector.statusIndicator(provider)
		lines = append(lines, gitui.TruncateToWidth(line, width, "", true))
	}
	if start > 0 || end < len(c.filtered) {
		lines = append(lines, gitui.TruncateToWidth("  ("+strconv.Itoa(c.selected+1)+"/"+strconv.Itoa(len(c.filtered))+")", width, "", true))
	}
	return append(lines, "", authSelectorBorder(width))
}

func (c *OAuthSelectorComponent) HandleInput(input string) {
	if c == nil {
		return
	}
	kb := gitui.GetKeybindings()
	switch {
	case kb.Matches(input, "tui.select.up") || input == "k":
		c.move(-1)
	case kb.Matches(input, "tui.select.down") || input == "j":
		c.move(1)
	case kb.Matches(input, "tui.select.confirm") || input == "\n":
		if c.selected >= 0 && c.selected < len(c.filtered) && c.OnSelect != nil {
			c.OnSelect(c.filtered[c.selected].ID)
		}
	case kb.Matches(input, "tui.select.cancel") || input == "\x03":
		if c.OnCancel != nil {
			c.OnCancel()
		}
	case kb.Matches(input, "tui.editor.deleteToLineStart") || input == "\x15":
		c.query = ""
		c.filter()
	case gitui.MatchesKey(input, "backspace") || input == "\b" || input == "\x7f":
		runes := []rune(c.query)
		if len(runes) > 0 {
			c.query = string(runes[:len(runes)-1])
			c.filter()
		}
	default:
		if text := authSelectorSearchText(input); text != "" {
			c.query += text
			c.filter()
		}
	}
}

func (c *OAuthSelectorComponent) footerHint() string {
	upDown := formatHotkeyKeys(append(gitui.GetKeybindings().GetKeys("tui.select.up"), gitui.GetKeybindings().GetKeys("tui.select.down")...), true)
	confirm := formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.select.confirm"), true)
	cancel := formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.select.cancel"), true)
	clear := formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.editor.deleteToLineStart"), true)
	return "  Type search. " +
		firstNonEmptyString(upDown, "Up/Down") + " move. " +
		firstNonEmptyString(confirm, "Enter") + " select. " +
		firstNonEmptyString(clear, "Ctrl+U") + " clear. " +
		firstNonEmptyString(cancel, "Esc") + " cancel."
}

func (c *OAuthSelectorComponent) move(delta int) {
	if len(c.filtered) == 0 {
		return
	}
	c.selected = (c.selected + delta + len(c.filtered)) % len(c.filtered)
}

func (c *OAuthSelectorComponent) filter() {
	if c == nil {
		return
	}
	query := strings.ToLower(strings.TrimSpace(c.query))
	c.filtered = c.filtered[:0]
	for _, provider := range c.selector.Providers {
		haystack := strings.ToLower(provider.Name + " " + provider.ID + " " + provider.AuthType)
		if query == "" || strings.Contains(haystack, query) {
			c.filtered = append(c.filtered, provider)
		}
	}
	if len(c.filtered) == 0 {
		c.selected = 0
		return
	}
	c.selected = max(0, min(c.selected, len(c.filtered)-1))
}

func authSelectorSearchText(input string) string {
	runes := []rune(input)
	if len(runes) == 0 {
		return ""
	}
	for _, r := range runes {
		if !unicode.IsPrint(r) || unicode.IsControl(r) {
			return ""
		}
	}
	return input
}

func loginAuthSelectorProviders(registry *ModelRegistry, authType string) []AuthSelectorProvider {
	if registry == nil {
		return nil
	}
	oauthProviders := GetOAuthProviders()
	oauthProviderIDs := map[string]bool{}
	for _, provider := range oauthProviders {
		oauthProviderIDs[provider.ID] = true
	}
	builtInProviderIDs := map[string]bool{}
	for providerID := range builtInProviderDisplayNames {
		builtInProviderIDs[providerID] = true
	}
	var providers []AuthSelectorProvider
	if authType == "" || authType == "oauth" {
		for _, provider := range oauthProviders {
			providers = append(providers, AuthSelectorProvider{ID: provider.ID, Name: firstNonEmptyString(provider.Name, provider.ID), AuthType: "oauth"})
		}
	}
	if authType == "" || authType == "api_key" {
		modelProviders := map[string]bool{}
		for _, model := range registry.GetAll() {
			if strings.TrimSpace(model.Provider) != "" {
				modelProviders[model.Provider] = true
			}
		}
		for providerID := range modelProviders {
			if !IsAPIKeyLoginProvider(providerID, oauthProviderIDs, builtInProviderIDs) {
				continue
			}
			providers = append(providers, AuthSelectorProvider{ID: providerID, Name: registry.GetProviderDisplayName(providerID), AuthType: "api_key"})
		}
	}
	sortAuthSelectorProviders(providers)
	return providers
}

func logoutAuthSelectorProviders(registry *ModelRegistry) []AuthSelectorProvider {
	if registry == nil || registry.authStorage == nil {
		return nil
	}
	var providers []AuthSelectorProvider
	for _, providerID := range registry.authStorage.List() {
		credential, ok := registry.authStorage.Get(providerID)
		if !ok {
			continue
		}
		providers = append(providers, AuthSelectorProvider{ID: providerID, Name: registry.GetProviderDisplayName(providerID), AuthType: credential.Type})
	}
	sortAuthSelectorProviders(providers)
	return providers
}

func sortAuthSelectorProviders(providers []AuthSelectorProvider) {
	sort.SliceStable(providers, func(i, j int) bool {
		left := strings.ToLower(providers[i].Name)
		right := strings.ToLower(providers[j].Name)
		if left == right {
			if providers[i].AuthType == providers[j].AuthType {
				return providers[i].ID < providers[j].ID
			}
			return providers[i].AuthType < providers[j].AuthType
		}
		return left < right
	})
}

func authSelectorBorder(width int) string {
	return gitui.TruncateToWidth(" "+strings.Repeat("-", max(0, width-1)), width, "", true)
}
