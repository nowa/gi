package gicodingagent

import (
	"strings"
)

const unknownProvider = "unknown"

func providerLoginHelp() string {
	return strings.Join([]string{
		"Use /login to log into a provider via OAuth or API key. See:",
		"  " + giDocumentationFilePath("", "providers.md"),
		"  " + giDocumentationFilePath("", "models.md"),
	}, "\n")
}

func formatNoModelsAvailableMessage() string {
	return "No models available. " + providerLoginHelp()
}

func formatNoModelSelectedMessage() string {
	return "No model selected.\n\n" + providerLoginHelp() + "\n\nThen use /model to select a model."
}

func formatNoAPIKeyFoundMessage(provider string) string {
	providerDisplay := provider
	if providerDisplay == "" || providerDisplay == unknownProvider {
		providerDisplay = "the selected model"
	}
	return "No API key found for " + providerDisplay + ".\n\n" + providerLoginHelp()
}
