package gicodingagent

import (
	"strings"
	"testing"
)

func TestAuthGuidanceMessagesUseGiConfigurationPaths(t *testing.T) {
	help := providerLoginHelp()
	for _, expected := range []string{"/login", "docs/providers.md", "docs/models.md"} {
		if !strings.Contains(help, expected) {
			t.Fatalf("providerLoginHelp() = %q, want %q", help, expected)
		}
	}
	if strings.Contains(help, ".pi") {
		t.Fatalf("providerLoginHelp() = %q, should not mention Pi config paths", help)
	}

	apiKeyMessage := formatNoAPIKeyFoundMessage("openai")
	if !strings.Contains(apiKeyMessage, "No API key found for openai.") ||
		!strings.Contains(apiKeyMessage, "Use /login to log into a provider via OAuth or API key. See:") ||
		!strings.Contains(apiKeyMessage, "docs/models.md") {
		t.Fatalf("formatNoAPIKeyFoundMessage = %q", apiKeyMessage)
	}

	unknownMessage := formatNoAPIKeyFoundMessage("unknown")
	if !strings.Contains(unknownMessage, "No API key found for the selected model.") {
		t.Fatalf("unknown provider message = %q", unknownMessage)
	}
}
