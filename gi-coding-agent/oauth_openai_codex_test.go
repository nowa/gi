package gicodingagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestOpenAICodexOAuthFlowMatchesPiCallbackContract(t *testing.T) {
	flow, err := newOpenAICodexOAuthFlow()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(flow.URL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.String() == "" || parsed.Scheme != "https" || parsed.Host != "auth.openai.com" || parsed.Path != "/oauth/authorize" {
		t.Fatalf("auth URL = %q", flow.URL)
	}
	for key, want := range map[string]string{
		"response_type":              "code",
		"client_id":                  openAICodexOAuthClientID,
		"redirect_uri":               openAICodexOAuthRedirect,
		"scope":                      openAICodexOAuthScopes,
		"code_challenge_method":      "S256",
		"state":                      flow.State,
		"id_token_add_organizations": "true",
		"codex_cli_simplified_flow":  "true",
		"originator":                 "gi",
	} {
		if got := query.Get(key); got != want {
			t.Fatalf("%s = %q, want %q in %s", key, got, want, flow.URL)
		}
	}
	if flow.Verifier == "" || query.Get("code_challenge") != oauthPKCEChallenge(flow.Verifier) {
		t.Fatalf("PKCE challenge did not match verifier")
	}
}

func TestOpenAICodexOAuthCallbackServerAcceptsBrowserRedirect(t *testing.T) {
	t.Setenv("GI_OAUTH_CALLBACK_HOST", "127.0.0.1")
	server, err := startOpenAICodexOAuthCallbackServerOnPort("state-ok", "0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	local, ok := server.(*localOpenAICodexOAuthCallbackServer)
	if !ok || len(local.servers) != 1 {
		t.Fatalf("server = %#v", server)
	}
	addr := local.servers[0].Addr
	if addr == "" {
		t.Fatal("test callback server did not record address")
	}
	response, err := http.Get("http://" + addr + "/auth/callback?code=code-ok&state=state-ok")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if !strings.Contains(string(body), ">Gi<") || !strings.Contains(string(body), "OpenAI authentication completed") {
		t.Fatalf("callback body missing Gi branding or success text: %s", body)
	}
	select {
	case result := <-server.Result():
		if result.Err != nil || result.Code != "code-ok" {
			t.Fatalf("callback result = %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("callback result timed out")
	}
}

func TestOAuthResultHTMLUsesSharedProviderPageAndCallbackPath(t *testing.T) {
	body := oauthResultHTMLForPath("Anthropic authentication completed", "You can close this window.", "/callback")
	for _, want := range []string{
		">Gi<",
		"Anthropic authentication completed",
		"You can close this window.",
		`history.replaceState(null,"","/callback")`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("callback body missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `history.replaceState(null,"","/auth/callback")`) {
		t.Fatalf("callback body used OpenAI callback path for Anthropic page: %s", body)
	}
}

func TestOpenAICodexTokenExchangeBuildsStoredCredential(t *testing.T) {
	accessToken := testOpenAICodexAccessToken(t, "acc_test")
	var body string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		body = r.PostForm.Encode()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  accessToken,
			"refresh_token": "refresh-token",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	before := nowUnixMilli()
	credential, err := exchangeOpenAICodexToken(context.Background(), tokenServer.Client(), tokenServer.URL, "exchange", url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {openAICodexOAuthClientID},
		"code":          {"code-ok"},
		"code_verifier": {"verifier-ok"},
		"redirect_uri":  {openAICodexOAuthRedirect},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"grant_type=authorization_code",
		"client_id=" + url.QueryEscape(openAICodexOAuthClientID),
		"code=code-ok",
		"code_verifier=verifier-ok",
		"redirect_uri=" + url.QueryEscape(openAICodexOAuthRedirect),
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("request body missing %q: %s", expected, body)
		}
	}
	if credential.Type != "oauth" || credential.Access != accessToken || credential.Refresh != "refresh-token" || credential.Expires < before+3599_000 {
		t.Fatalf("credential = %#v", credential)
	}
}

func TestAnthropicOAuthFlowMatchesPiCallbackContract(t *testing.T) {
	flow, err := newAnthropicOAuthFlow()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(flow.URL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Scheme != "https" || parsed.Host != "claude.ai" || parsed.Path != "/oauth/authorize" {
		t.Fatalf("auth URL = %q", flow.URL)
	}
	for key, want := range map[string]string{
		"code":                  "true",
		"client_id":             anthropicOAuthClientID,
		"response_type":         "code",
		"redirect_uri":          anthropicOAuthRedirect,
		"scope":                 anthropicOAuthScopes,
		"code_challenge_method": "S256",
		"state":                 flow.Verifier,
	} {
		if got := query.Get(key); got != want {
			t.Fatalf("%s = %q, want %q in %s", key, got, want, flow.URL)
		}
	}
	if flow.RedirectURI != anthropicOAuthRedirect || flow.State != flow.Verifier || query.Get("code_challenge") != oauthPKCEChallenge(flow.Verifier) {
		t.Fatalf("flow = %#v challenge=%q", flow, query.Get("code_challenge"))
	}
}

func TestAnthropicOAuthCallbackHandlerAcceptsBrowserRedirect(t *testing.T) {
	server := &localAnthropicOAuthCallbackServer{result: make(chan openAICodexOAuthCallbackResult, 1)}
	request := httptest.NewRequest(http.MethodGet, "http://localhost/callback?code=code-ok&state=verifier-ok", nil)
	response := httptest.NewRecorder()

	server.handler("verifier-ok").ServeHTTP(response, request)

	body := response.Body.String()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, body)
	}
	if !strings.Contains(body, ">Gi<") || !strings.Contains(body, "Anthropic authentication completed") || !strings.Contains(body, `history.replaceState(null,"","/callback")`) {
		t.Fatalf("callback body missing Gi branding, success text, or callback path: %s", body)
	}
	select {
	case result := <-server.Result():
		if result.Err != nil || result.Code != "code-ok" {
			t.Fatalf("callback result = %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("callback result timed out")
	}
}

func TestAnthropicTokenExchangeBuildsStoredCredential(t *testing.T) {
	var body map[string]any
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("content-type = %q", contentType)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "anthropic-access",
			"refresh_token": "anthropic-refresh",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	before := nowUnixMilli()
	credential, err := exchangeAnthropicOAuthToken(context.Background(), tokenServer.Client(), tokenServer.URL, "exchange", map[string]any{
		"grant_type":    "authorization_code",
		"client_id":     anthropicOAuthClientID,
		"code":          "code-ok",
		"state":         "verifier-ok",
		"redirect_uri":  anthropicOAuthRedirect,
		"code_verifier": "verifier-ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"grant_type":    "authorization_code",
		"client_id":     anthropicOAuthClientID,
		"code":          "code-ok",
		"state":         "verifier-ok",
		"redirect_uri":  anthropicOAuthRedirect,
		"code_verifier": "verifier-ok",
	} {
		if got := body[key]; got != want {
			t.Fatalf("%s = %#v, want %#v in %#v", key, got, want, body)
		}
	}
	if credential.Type != "oauth" || credential.Access != "anthropic-access" || credential.Refresh != "anthropic-refresh" || credential.Expires < before+3299_000 {
		t.Fatalf("credential = %#v", credential)
	}
}

func TestGitHubCopilotOAuthContractsMatchPi(t *testing.T) {
	for input, want := range map[string]string{
		"":                          "",
		"github.com":                "github.com",
		"https://company.ghe.com/x": "company.ghe.com",
		"company.ghe.com":           "company.ghe.com",
	} {
		got, ok := normalizeGitHubCopilotDomain(input)
		if !ok || got != want {
			t.Fatalf("normalizeGitHubCopilotDomain(%q) = %q, %v; want %q, true", input, got, ok, want)
		}
	}
	if _, ok := normalizeGitHubCopilotDomain("://bad domain"); ok {
		t.Fatal("invalid enterprise domain accepted")
	}

	deviceURL, accessURL, tokenURL := githubCopilotOAuthURLs("github.com")
	if deviceURL != "https://github.com/login/device/code" ||
		accessURL != "https://github.com/login/oauth/access_token" ||
		tokenURL != "https://api.github.com/copilot_internal/v2/token" {
		t.Fatalf("github URLs = %q %q %q", deviceURL, accessURL, tokenURL)
	}

	request, err := newGitHubCopilotFormRequest(context.Background(), deviceURL, url.Values{"client_id": {githubCopilotOAuthClientID}, "scope": {"read:user"}})
	if err != nil {
		t.Fatal(err)
	}
	if request.Header.Get("Accept") != "application/json" ||
		request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" ||
		request.Header.Get("User-Agent") != githubCopilotUserAgent {
		t.Fatalf("headers = %#v", request.Header)
	}
}

func testOpenAICodexAccessToken(t *testing.T, accountID string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payloadBytes, err := json.Marshal(map[string]any{
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": accountID},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return header + "." + payload + "."
}
