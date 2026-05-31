package gicodingagent

import (
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
	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	openAICodexOAuthTokenURL = "https://auth.openai.com/oauth/token"
)

type openAICodexOAuthFlow struct {
	Verifier string
	State    string
	URL      string
}

type openAICodexOAuthCallbackResult struct {
	Code string
	Err  error
}

type openAICodexOAuthCallbackServer interface {
	Result() <-chan openAICodexOAuthCallbackResult
	Close()
}

type openAICodexOAuthRuntime struct {
	NewFlow             func() (openAICodexOAuthFlow, error)
	StartCallbackServer func(state string) (openAICodexOAuthCallbackServer, error)
	ExchangeCode        func(context.Context, string, string) (AuthCredential, error)
	OpenBrowser         func(string) error
}

var defaultOpenAICodexOAuthRuntime = openAICodexOAuthRuntime{
	NewFlow:             newOpenAICodexOAuthFlow,
	StartCallbackServer: startOpenAICodexOAuthCallbackServer,
	ExchangeCode:        exchangeOpenAICodexAuthorizationCode,
	OpenBrowser:         defaultOpenOAuthURL,
}

func newOpenAICodexOAuthFlow() (openAICodexOAuthFlow, error) {
	verifier := oauthRandomToken(32)
	if verifier == "" {
		return openAICodexOAuthFlow{}, errors.New("failed to generate PKCE verifier")
	}
	state := oauthRandomToken(16)
	if state == "" {
		return openAICodexOAuthFlow{}, errors.New("failed to generate OAuth state")
	}
	return openAICodexOAuthFlow{
		Verifier: verifier,
		State:    state,
		URL: orderedOAuthURL(openAICodexOAuthAuthURL, [][2]string{
			{"response_type", "code"},
			{"client_id", openAICodexOAuthClientID},
			{"redirect_uri", openAICodexOAuthRedirect},
			{"scope", openAICodexOAuthScopes},
			{"code_challenge", oauthPKCEChallenge(verifier)},
			{"code_challenge_method", "S256"},
			{"state", state},
			{"id_token_add_organizations", "true"},
			{"codex_cli_simplified_flow", "true"},
			{"originator", "gi"},
		}),
	}, nil
}

func parseOpenAICodexAuthorizationInput(input string) (code string, state string) {
	return oauthflow.ParseAuthorizationInput(input)
}

type localOpenAICodexOAuthCallbackServer struct {
	servers []*http.Server
	result  chan openAICodexOAuthCallbackResult
	once    sync.Once
}

func startOpenAICodexOAuthCallbackServer(state string) (openAICodexOAuthCallbackServer, error) {
	return startOpenAICodexOAuthCallbackServerOnPort(state, "1455")
}

func startOpenAICodexOAuthCallbackServerOnPort(state, port string) (openAICodexOAuthCallbackServer, error) {
	hosts := openAICodexOAuthCallbackHosts()
	var servers []*http.Server
	var errorsText []string
	local := &localOpenAICodexOAuthCallbackServer{result: make(chan openAICodexOAuthCallbackResult, 1)}
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
		return nil, fmt.Errorf("could not listen on local OAuth callback port 1455: %s", strings.Join(errorsText, "; "))
	}
	local.servers = servers
	return local, nil
}

func openAICodexOAuthCallbackHosts() []string {
	return oauthflow.CallbackHosts()
}

func (s *localOpenAICodexOAuthCallbackServer) Result() <-chan openAICodexOAuthCallbackResult {
	if s == nil {
		return nil
	}
	return s.result
}

func (s *localOpenAICodexOAuthCallbackServer) Close() {
	if s == nil {
		return
	}
	for _, server := range s.servers {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = server.Shutdown(ctx)
		cancel()
	}
}

func (s *localOpenAICodexOAuthCallbackServer) handler(expectedState string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.URL == nil || r.URL.Path != "/auth/callback" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, oauthResultHTML("OAuth callback failed", "Callback route not found."))
			return
		}
		if r.URL.Query().Get("state") != expectedState {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, oauthResultHTML("OAuth callback failed", "State mismatch."))
			s.settle(openAICodexOAuthCallbackResult{Err: errors.New("state mismatch")})
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, oauthResultHTML("OAuth callback failed", "Missing authorization code."))
			s.settle(openAICodexOAuthCallbackResult{Err: errors.New("missing authorization code")})
			return
		}
		_, _ = io.WriteString(w, oauthResultHTML("OpenAI authentication completed", "You can close this window."))
		s.settle(openAICodexOAuthCallbackResult{Code: code})
	})
}

func (s *localOpenAICodexOAuthCallbackServer) settle(result openAICodexOAuthCallbackResult) {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.result <- result
	})
}

func oauthResultHTML(title, message string) string {
	return oauthResultHTMLForPath(title, message, "/auth/callback")
}

func oauthResultHTMLForPath(title, message, historyPath string) string {
	return llm.OAuthPageHTML(llm.OAuthPageOptions{
		Title:       title,
		Heading:     title,
		Message:     message,
		ProductName: "Gi",
		HistoryPath: historyPath,
	})
}

func exchangeOpenAICodexAuthorizationCode(ctx context.Context, code, verifier string) (AuthCredential, error) {
	return exchangeOpenAICodexToken(ctx, http.DefaultClient, openAICodexOAuthTokenURL, "exchange", url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {openAICodexOAuthClientID},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {openAICodexOAuthRedirect},
	})
}

func refreshOpenAICodexOAuthToken(credential AuthCredential) (AuthCredential, error) {
	if strings.TrimSpace(credential.Refresh) == "" {
		return AuthCredential{}, errors.New("OpenAI Codex refresh token is missing")
	}
	return exchangeOpenAICodexToken(context.Background(), http.DefaultClient, openAICodexOAuthTokenURL, "refresh", url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {openAICodexOAuthClientID},
		"refresh_token": {credential.Refresh},
	})
}

func exchangeOpenAICodexToken(ctx context.Context, client *http.Client, tokenURL, operation string, values url.Values) (AuthCredential, error) {
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return AuthCredential{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return AuthCredential{}, fmt.Errorf("OpenAI Codex token request failed: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return AuthCredential{}, openAICodexTokenHTTPError(operation, response.StatusCode, response.Status, string(body))
	}
	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return AuthCredential{}, fmt.Errorf("OpenAI Codex token response parse failed: %w", err)
	}
	if parsed.AccessToken == "" || parsed.RefreshToken == "" || parsed.ExpiresIn <= 0 {
		return AuthCredential{}, fmt.Errorf("OpenAI Codex token exchange response missing fields: %s", strings.TrimSpace(string(body)))
	}
	if _, err := llm.ExtractOpenAICodexAccountID(parsed.AccessToken); err != nil {
		return AuthCredential{}, fmt.Errorf("OpenAI Codex token response missing account id: %w", err)
	}
	return AuthCredential{
		Type:    "oauth",
		Access:  parsed.AccessToken,
		Refresh: parsed.RefreshToken,
		Expires: nowUnixMilli() + parsed.ExpiresIn*1000,
	}, nil
}

func openAICodexTokenHTTPError(operation string, status int, statusText, body string) error {
	message := statusText
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err == nil && parsed.Error.Message != "" {
		message = parsed.Error.Message
	} else if strings.TrimSpace(body) != "" {
		message = strings.TrimSpace(body)
	}
	if operation == "" {
		operation = "token request"
	}
	return fmt.Errorf("OpenAI Codex token %s failed (%d): %s", operation, status, message)
}

func defaultOpenOAuthURL(rawURL string) error {
	return oauthflow.OpenBrowser(rawURL)
}
