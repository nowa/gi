package gicodingagent

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
)

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
	query := make([]byte, 0, len(params)*16)
	for index, param := range params {
		if index > 0 {
			query = append(query, '&')
		}
		query = append(query, url.QueryEscape(param[0])...)
		query = append(query, '=')
		query = append(query, url.QueryEscape(param[1])...)
	}
	return base + "?" + string(query)
}

func oauthPKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func oauthRandomToken(size int) string {
	if size <= 0 {
		size = 32
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "gi-oauth-token"
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
