package gicodingagent

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
