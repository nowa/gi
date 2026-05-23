package gicodingagent

import (
	"fmt"
	"strings"
)

const unknownProvider = "unknown"

func providerLoginHelp() string {
	return "Configure provider credentials with --api-key, provider environment variables, ~/.gi/agent/auth.json, or ~/.gi/agent/models.json.\nRun gi --list-models to verify available models."
}

func formatNoModelsAvailableMessage() string {
	return "No models available.\n\n" + providerLoginHelp()
}

func formatNoModelSelectedMessage() string {
	return "No model selected.\n\n" + providerLoginHelp()
}

func formatNoAPIKeyFoundMessage(provider string) string {
	providerDisplay := provider
	if providerDisplay == "" || providerDisplay == unknownProvider {
		providerDisplay = "the selected model"
	}
	help := providerLoginHelp()
	if keys := providerEnvKeys(provider); len(keys) > 0 {
		help = fmt.Sprintf("Set %s, pass --api-key, or configure ~/.gi/agent/auth.json or ~/.gi/agent/models.json.\nRun gi --list-models to verify available models.", strings.Join(keys, " or "))
	}
	return fmt.Sprintf("No API key found for %s.\n\n%s", providerDisplay, help)
}
