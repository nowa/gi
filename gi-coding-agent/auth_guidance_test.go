package gicodingagent

import (
	"strings"
	"testing"
)

func TestAuthGuidanceMessagesUseGiConfigurationPaths(t *testing.T) {
	help := providerLoginHelp()
	for _, expected := range []string{"--api-key", "~/.gi/agent/auth.json", "~/.gi/agent/models.json", "gi --list-models"} {
		if !strings.Contains(help, expected) {
			t.Fatalf("providerLoginHelp() = %q, want %q", help, expected)
		}
	}
	if strings.Contains(help, "/login") || strings.Contains(help, ".pi") {
		t.Fatalf("providerLoginHelp() = %q, should not mention Pi-only login/config paths", help)
	}

	apiKeyMessage := formatNoAPIKeyFoundMessage("openai")
	if !strings.Contains(apiKeyMessage, "No API key found for openai.") ||
		!strings.Contains(apiKeyMessage, "OPENAI_API_KEY") ||
		!strings.Contains(apiKeyMessage, "--api-key") {
		t.Fatalf("formatNoAPIKeyFoundMessage = %q", apiKeyMessage)
	}

	unknownMessage := formatNoAPIKeyFoundMessage("unknown")
	if !strings.Contains(unknownMessage, "No API key found for the selected model.") {
		t.Fatalf("unknown provider message = %q", unknownMessage)
	}
}
