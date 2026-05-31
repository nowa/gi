package gicodingagent

import authguide "github.com/nowa/gi/gi-coding-agent/internal/authguide"

const unknownProvider = authguide.UnknownProvider

func providerLoginHelp() string {
	return authguide.ProviderLoginHelp(giDocumentationFilePath)
}

func formatNoModelsAvailableMessage() string {
	return authguide.FormatNoModelsAvailableMessage(giDocumentationFilePath)
}

func formatNoModelSelectedMessage() string {
	return authguide.FormatNoModelSelectedMessage(giDocumentationFilePath)
}

func formatNoAPIKeyFoundMessage(provider string) string {
	return authguide.FormatNoAPIKeyFoundMessage(provider, giDocumentationFilePath)
}
