package gillmprovider

import "github.com/nowa/gi/gi-llm-provider/internal/oauthpage"

type OAuthPageOptions = oauthpage.OAuthPageOptions

func OAuthPageHTML(options OAuthPageOptions) string {
	return oauthpage.OAuthPageHTML(options)
}

func OAuthSuccessHTML(message string) string {
	return oauthpage.OAuthSuccessHTML(message)
}

func OAuthErrorHTML(message, details string) string {
	return oauthpage.OAuthErrorHTML(message, details)
}
