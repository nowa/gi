package gicodingagent

import "github.com/nowa/gi/gi-coding-agent/internal/oauthflow"

const (
	anthropicOAuthClientID   = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	anthropicOAuthAuthorize  = "https://claude.ai/oauth/authorize"
	anthropicOAuthRedirect   = "http://localhost:53692/callback"
	anthropicOAuthScopes     = "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	openAICodexOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	openAICodexOAuthAuthURL  = "https://auth.openai.com/oauth/authorize"
	openAICodexOAuthRedirect = "http://localhost:1455/auth/callback"
	openAICodexOAuthScopes   = "openid profile email offline_access"
)

type oauthLoginPrompt struct {
	URL          string
	Instructions string
	ManualPrompt string
}

func oauthLoginPromptForProvider(providerID string) (oauthLoginPrompt, bool) {
	verifier := oauthRandomToken(32)
	challenge := oauthPKCEChallenge(verifier)
	switch providerID {
	case "anthropic":
		return oauthLoginPrompt{
			URL: orderedOAuthURL(anthropicOAuthAuthorize, [][2]string{
				{"code", "true"},
				{"client_id", anthropicOAuthClientID},
				{"response_type", "code"},
				{"redirect_uri", anthropicOAuthRedirect},
				{"scope", anthropicOAuthScopes},
				{"code_challenge", challenge},
				{"code_challenge_method", "S256"},
				{"state", verifier},
			}),
			Instructions: "Complete login in your browser. If the browser is on another machine, paste the final redirect URL here.",
			ManualPrompt: "Paste redirect URL below, or complete login in browser:",
		}, true
	case "openai-codex":
		return oauthLoginPrompt{
			URL: orderedOAuthURL(openAICodexOAuthAuthURL, [][2]string{
				{"response_type", "code"},
				{"client_id", openAICodexOAuthClientID},
				{"redirect_uri", openAICodexOAuthRedirect},
				{"scope", openAICodexOAuthScopes},
				{"code_challenge", challenge},
				{"code_challenge_method", "S256"},
				{"state", oauthRandomToken(16)},
				{"id_token_add_organizations", "true"},
				{"codex_cli_simplified_flow", "true"},
				{"originator", "gi"},
			}),
			Instructions: "A browser window should open. Complete login to finish.",
			ManualPrompt: "Paste redirect URL below, or complete login in browser:",
		}, true
	default:
		return oauthLoginPrompt{}, false
	}
}

func orderedOAuthURL(base string, params [][2]string) string {
	return oauthflow.OrderedURL(base, params)
}

func oauthPKCEChallenge(verifier string) string {
	return oauthflow.PKCEChallenge(verifier)
}

func oauthRandomToken(size int) string {
	return oauthflow.RandomToken(size)
}
