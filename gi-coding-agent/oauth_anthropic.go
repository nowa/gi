package gicodingagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nowa/gi/gi-coding-agent/internal/oauthflow"
)

const (
	anthropicOAuthTokenURL     = "https://platform.claude.com/v1/oauth/token"
	anthropicOAuthCallbackPort = "53692"
	anthropicOAuthCallbackPath = "/callback"
)

type anthropicOAuthFlow struct {
	Verifier    string
	State       string
	RedirectURI string
	URL         string
}

type anthropicOAuthRuntime struct {
	NewFlow             func() (anthropicOAuthFlow, error)
	StartCallbackServer func(state string) (openAICodexOAuthCallbackServer, error)
	ExchangeCode        func(context.Context, string, string, string) (AuthCredential, error)
	OpenBrowser         func(string) error
}

var defaultAnthropicOAuthRuntime = anthropicOAuthRuntime{
	NewFlow:             newAnthropicOAuthFlow,
	StartCallbackServer: startAnthropicOAuthCallbackServer,
	ExchangeCode:        exchangeAnthropicAuthorizationCode,
	OpenBrowser:         defaultOpenOAuthURL,
}

func newAnthropicOAuthFlow() (anthropicOAuthFlow, error) {
	verifier := oauthRandomToken(32)
	if verifier == "" {
		return anthropicOAuthFlow{}, errors.New("failed to generate PKCE verifier")
	}
	redirectURI := "http://localhost:" + anthropicOAuthCallbackPort + anthropicOAuthCallbackPath
	return anthropicOAuthFlow{
		Verifier:    verifier,
		State:       verifier,
		RedirectURI: redirectURI,
		URL: orderedOAuthURL(anthropicOAuthAuthorize, [][2]string{
			{"code", "true"},
			{"client_id", anthropicOAuthClientID},
			{"response_type", "code"},
			{"redirect_uri", redirectURI},
			{"scope", anthropicOAuthScopes},
			{"code_challenge", oauthPKCEChallenge(verifier)},
			{"code_challenge_method", "S256"},
			{"state", verifier},
		}),
	}, nil
}

func startAnthropicOAuthCallbackServer(state string) (openAICodexOAuthCallbackServer, error) {
	return startAnthropicOAuthCallbackServerOnPort(state, anthropicOAuthCallbackPort)
}

func startAnthropicOAuthCallbackServerOnPort(state, port string) (openAICodexOAuthCallbackServer, error) {
	hosts := oauthCallbackHosts()
	var servers []*http.Server
	var errorsText []string
	local := &localAnthropicOAuthCallbackServer{result: make(chan openAICodexOAuthCallbackResult, 1)}
	handler := local.handler(state)
	for _, host := range hosts {
		listener, err := net.Listen("tcp", net.JoinHostPort(host, port))
		if err != nil {
			errorsText = append(errorsText, err.Error())
			continue
		}
		server := &http.Server{Addr: listener.Addr().String(), Handler: handler}
		servers = append(servers, server)
		go func() {
			if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				local.settle(openAICodexOAuthCallbackResult{Err: serveErr})
			}
		}()
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("could not listen on local OAuth callback port "+port+": %s", strings.Join(errorsText, "; "))
	}
	local.servers = servers
	return local, nil
}

type localAnthropicOAuthCallbackServer struct {
	servers []*http.Server
	result  chan openAICodexOAuthCallbackResult
	once    sync.Once
}

func (s *localAnthropicOAuthCallbackServer) Result() <-chan openAICodexOAuthCallbackResult {
	if s == nil {
		return nil
	}
	return s.result
}

func (s *localAnthropicOAuthCallbackServer) Close() {
	if s == nil {
		return
	}
	for _, server := range s.servers {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = server.Shutdown(ctx)
		cancel()
	}
}

func (s *localAnthropicOAuthCallbackServer) handler(expectedState string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.URL == nil || r.URL.Path != anthropicOAuthCallbackPath {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, oauthResultHTMLForPath("OAuth callback failed", "Callback route not found.", anthropicOAuthCallbackPath))
			return
		}
		if errorText := r.URL.Query().Get("error"); errorText != "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, oauthResultHTMLForPath("Anthropic authentication did not complete", "Error: "+errorText, anthropicOAuthCallbackPath))
			s.settle(openAICodexOAuthCallbackResult{Err: fmt.Errorf("anthropic OAuth error: %s", errorText)})
			return
		}
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		if code == "" || state == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, oauthResultHTMLForPath("OAuth callback failed", "Missing code or state parameter.", anthropicOAuthCallbackPath))
			s.settle(openAICodexOAuthCallbackResult{Err: errors.New("missing code or state parameter")})
			return
		}
		if state != expectedState {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, oauthResultHTMLForPath("OAuth callback failed", "State mismatch.", anthropicOAuthCallbackPath))
			s.settle(openAICodexOAuthCallbackResult{Err: errors.New("OAuth state mismatch")})
			return
		}
		_, _ = io.WriteString(w, oauthResultHTMLForPath("Anthropic authentication completed", "You can close this window.", anthropicOAuthCallbackPath))
		s.settle(openAICodexOAuthCallbackResult{Code: code})
	})
}

func (s *localAnthropicOAuthCallbackServer) settle(result openAICodexOAuthCallbackResult) {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.result <- result
	})
}

func exchangeAnthropicAuthorizationCode(ctx context.Context, code, verifier, redirectURI string) (AuthCredential, error) {
	return exchangeAnthropicOAuthToken(ctx, http.DefaultClient, anthropicOAuthTokenURL, "exchange", map[string]any{
		"grant_type":    "authorization_code",
		"client_id":     anthropicOAuthClientID,
		"code":          code,
		"state":         verifier,
		"redirect_uri":  redirectURI,
		"code_verifier": verifier,
	})
}

func refreshAnthropicOAuthToken(credential AuthCredential) (AuthCredential, error) {
	if strings.TrimSpace(credential.Refresh) == "" {
		return AuthCredential{}, errors.New("Anthropic refresh token is missing")
	}
	return exchangeAnthropicOAuthToken(context.Background(), http.DefaultClient, anthropicOAuthTokenURL, "refresh", map[string]any{
		"grant_type":    "refresh_token",
		"client_id":     anthropicOAuthClientID,
		"refresh_token": credential.Refresh,
	})
}

func exchangeAnthropicOAuthToken(ctx context.Context, client *http.Client, tokenURL, operation string, payload map[string]any) (AuthCredential, error) {
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	body, err := json.Marshal(payload)
	if err != nil {
		return AuthCredential{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(body))
	if err != nil {
		return AuthCredential{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return AuthCredential{}, fmt.Errorf("Anthropic token %s request failed: %w", operation, err)
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return AuthCredential{}, fmt.Errorf("Anthropic token %s failed (%d): %s", operation, response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return AuthCredential{}, fmt.Errorf("Anthropic token %s returned invalid JSON: %w", operation, err)
	}
	if parsed.AccessToken == "" || parsed.RefreshToken == "" || parsed.ExpiresIn <= 0 {
		return AuthCredential{}, fmt.Errorf("Anthropic token %s response missing fields: %s", operation, strings.TrimSpace(string(responseBody)))
	}
	return AuthCredential{
		Type:    "oauth",
		Access:  parsed.AccessToken,
		Refresh: parsed.RefreshToken,
		Expires: nowUnixMilli() + parsed.ExpiresIn*1000 - 5*60*1000,
	}, nil
}

func parseOAuthAuthorizationInput(input string) (code string, state string) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return parsed.Query().Get("code"), parsed.Query().Get("state")
	}
	if strings.Contains(value, "#") {
		code, state, _ := strings.Cut(value, "#")
		return code, state
	}
	if strings.Contains(value, "code=") {
		params, err := url.ParseQuery(value)
		if err == nil {
			return params.Get("code"), params.Get("state")
		}
	}
	return value, ""
}

func oauthCallbackHosts() []string {
	return oauthflow.CallbackHosts()
}
