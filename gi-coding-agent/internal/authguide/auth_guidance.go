package authguide

import (
	"strings"
)

const UnknownProvider = "unknown"

type DocumentationPathFunc func(root, name string) string

func ProviderLoginHelp(documentationPath DocumentationPathFunc) string {
	if documentationPath == nil {
		documentationPath = func(_, name string) string { return name }
	}
	return strings.Join([]string{
		"Use /login to log into a provider via OAuth or API key. See:",
		"  " + documentationPath("", "providers.md"),
		"  " + documentationPath("", "models.md"),
	}, "\n")
}

func FormatNoModelsAvailableMessage(documentationPath DocumentationPathFunc) string {
	return "No models available. " + ProviderLoginHelp(documentationPath)
}

func FormatNoModelSelectedMessage(documentationPath DocumentationPathFunc) string {
	return "No model selected.\n\n" + ProviderLoginHelp(documentationPath) + "\n\nThen use /model to select a model."
}

func FormatNoAPIKeyFoundMessage(provider string, documentationPath DocumentationPathFunc) string {
	providerDisplay := provider
	if providerDisplay == "" || providerDisplay == UnknownProvider {
		providerDisplay = "the selected model"
	}
	return "No API key found for " + providerDisplay + ".\n\n" + ProviderLoginHelp(documentationPath)
}
