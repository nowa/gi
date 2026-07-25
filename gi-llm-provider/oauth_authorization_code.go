package gillmprovider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type oauthAuthorizationCode struct {
	Code  string
	State string
}

type oauthAuthorizationCodeResult struct {
	authorization oauthAuthorizationCode
	err           error
}

type oauthAuthorizationCodeServer interface {
	Wait(context.Context) (oauthAuthorizationCode, error)
	Close() error
}

type oauthLoopbackCallbackOptions struct {
	Host               string
	Port               string
	Path               string
	ExpectedState      string
	ProviderName       string
	SuccessMessage     string
	ValidateStateFirst bool
}

type localOAuthAuthorizationCodeServer struct {
	server     *http.Server
	result     chan oauthAuthorizationCodeResult
	resultOnce sync.Once
	closeOnce  sync.Once
	closeErr   error
}

func startOAuthAuthorizationCodeServer(
	ctx context.Context,
	options oauthLoopbackCallbackOptions,
) (oauthAuthorizationCodeServer, error) {
	ctx = contextOrBackground(ctx)
	if err := contextError(ctx); err != nil {
		return nil, oauthLoginContextError(ctx)
	}
	if strings.TrimSpace(options.Host) == "" {
		return nil, errors.New("OAuth callback host is required")
	}
	if strings.TrimSpace(options.Port) == "" {
		return nil, errors.New("OAuth callback port is required")
	}
	if options.Path == "" || !strings.HasPrefix(options.Path, "/") {
		return nil, errors.New("OAuth callback path is invalid")
	}
	if options.ExpectedState == "" {
		return nil, errors.New("OAuth callback state is required")
	}

	listener, err := (&net.ListenConfig{}).Listen(
		ctx,
		"tcp",
		net.JoinHostPort(options.Host, options.Port),
	)
	if err != nil {
		return nil, err
	}
	local := &localOAuthAuthorizationCodeServer{
		result: make(chan oauthAuthorizationCodeResult, 1),
	}
	local.server = &http.Server{
		Handler:           local.handler(options),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       15 * time.Second,
	}
	go func() {
		serveErr := local.server.Serve(listener)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			local.settle(oauthAuthorizationCodeResult{err: serveErr})
		}
	}()
	return local, nil
}

func (s *localOAuthAuthorizationCodeServer) Wait(
	ctx context.Context,
) (oauthAuthorizationCode, error) {
	select {
	case result := <-s.result:
		return result.authorization, result.err
	case <-contextOrBackground(ctx).Done():
		return oauthAuthorizationCode{}, oauthLoginContextError(ctx)
	}
}

func (s *localOAuthAuthorizationCodeServer) Close() error {
	if s == nil || s.server == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		closeContext, cancel := context.WithTimeout(
			context.Background(),
			time.Second,
		)
		defer cancel()
		s.closeErr = s.server.Shutdown(closeContext)
		if errors.Is(s.closeErr, context.DeadlineExceeded) {
			s.closeErr = s.server.Close()
		}
		if errors.Is(s.closeErr, http.ErrServerClosed) {
			s.closeErr = nil
		}
	})
	return s.closeErr
}

func (s *localOAuthAuthorizationCodeServer) handler(
	options oauthLoopbackCallbackOptions,
) http.Handler {
	return http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet ||
			request.URL == nil ||
			request.URL.Path != options.Path {
			sendOAuthCallbackPage(
				writer,
				http.StatusNotFound,
				options.Path,
				"OAuth callback failed",
				"Callback route not found.",
				"",
			)
			return
		}

		query := request.URL.Query()
		if oauthError := query.Get("error"); oauthError != "" {
			description := query.Get("error_description")
			if description == "" {
				description = "Error: " + oauthError
			}
			sendOAuthCallbackPage(
				writer,
				http.StatusBadRequest,
				options.Path,
				options.ProviderName+" authentication did not complete",
				description,
				"",
			)
			return
		}

		state := query.Get("state")
		if options.ValidateStateFirst && state != options.ExpectedState {
			sendOAuthCallbackPage(
				writer,
				http.StatusBadRequest,
				options.Path,
				"OAuth callback failed",
				"State mismatch.",
				"",
			)
			return
		}

		code := query.Get("code")
		if code == "" || state == "" {
			sendOAuthCallbackPage(
				writer,
				http.StatusBadRequest,
				options.Path,
				"OAuth callback failed",
				"Missing code or state parameter.",
				"",
			)
			return
		}
		if !options.ValidateStateFirst && state != options.ExpectedState {
			sendOAuthCallbackPage(
				writer,
				http.StatusBadRequest,
				options.Path,
				"OAuth callback failed",
				"State mismatch.",
				"",
			)
			return
		}

		message := options.SuccessMessage
		if message == "" {
			message = options.ProviderName +
				" authentication completed. You can close this window."
		}
		sendOAuthCallbackPage(
			writer,
			http.StatusOK,
			options.Path,
			options.ProviderName+" authentication completed",
			message,
			"",
		)
		s.settle(oauthAuthorizationCodeResult{
			authorization: oauthAuthorizationCode{
				Code:  code,
				State: state,
			},
		})
	})
}

func (s *localOAuthAuthorizationCodeServer) settle(
	result oauthAuthorizationCodeResult,
) {
	s.resultOnce.Do(func() {
		s.result <- result
	})
}

func sendOAuthCallbackPage(
	writer http.ResponseWriter,
	status int,
	historyPath string,
	title string,
	message string,
	details string,
) {
	writer.Header().Set("content-type", "text/html; charset=utf-8")
	writer.Header().Set("cache-control", "no-store")
	writer.Header().Set("referrer-policy", "no-referrer")
	writer.WriteHeader(status)
	_, _ = io.WriteString(writer, OAuthPageHTML(OAuthPageOptions{
		Title:       title,
		Heading:     title,
		Message:     message,
		Details:     details,
		ProductName: "Gi",
		HistoryPath: historyPath,
	}))
}

func parseOAuthAuthorizationInput(
	input string,
) oauthAuthorizationCode {
	value := strings.TrimSpace(input)
	if value == "" {
		return oauthAuthorizationCode{}
	}
	if parsed, err := url.Parse(value); err == nil &&
		parsed.Scheme != "" &&
		parsed.Host != "" {
		return oauthAuthorizationCode{
			Code:  parsed.Query().Get("code"),
			State: parsed.Query().Get("state"),
		}
	}
	if strings.Contains(value, "#") {
		code, state, _ := strings.Cut(value, "#")
		return oauthAuthorizationCode{Code: code, State: state}
	}
	if strings.Contains(value, "code=") {
		if query, err := url.ParseQuery(value); err == nil {
			return oauthAuthorizationCode{
				Code:  query.Get("code"),
				State: query.Get("state"),
			}
		}
	}
	return oauthAuthorizationCode{Code: value}
}

type oauthManualCodeResult struct {
	authorization oauthAuthorizationCode
	err           error
}

func waitForOAuthAuthorizationCode(
	ctx context.Context,
	interaction AuthInteraction,
	callback oauthAuthorizationCodeServer,
	prompt AuthPrompt,
	expectedState string,
) (oauthAuthorizationCode, error) {
	ctx = contextOrBackground(ctx)
	promptContext, cancelPrompt := context.WithCancel(ctx)
	defer cancelPrompt()

	manualResults := make(chan oauthManualCodeResult, 1)
	go func() {
		input, err := interaction.Prompt(promptContext, prompt)
		result := oauthManualCodeResult{err: err}
		if err == nil {
			result.authorization = parseOAuthAuthorizationInput(input)
		}
		select {
		case manualResults <- result:
		case <-promptContext.Done():
		}
	}()

	var callbackResults <-chan oauthAuthorizationCodeResult
	if callback != nil {
		results := make(chan oauthAuthorizationCodeResult, 1)
		callbackResults = results
		go func() {
			authorization, err := callback.Wait(promptContext)
			select {
			case results <- oauthAuthorizationCodeResult{
				authorization: authorization,
				err:           err,
			}:
			case <-promptContext.Done():
			}
		}()
	}

	select {
	case result := <-manualResults:
		if result.err != nil {
			return oauthAuthorizationCode{}, result.err
		}
		if result.authorization.State != "" &&
			result.authorization.State != expectedState {
			return oauthAuthorizationCode{}, errors.New(
				"OAuth state mismatch",
			)
		}
		if result.authorization.Code == "" {
			return oauthAuthorizationCode{}, errors.New(
				"missing authorization code",
			)
		}
		if result.authorization.State == "" {
			result.authorization.State = expectedState
		}
		return result.authorization, nil
	case result := <-callbackResults:
		return result.authorization, result.err
	case <-ctx.Done():
		return oauthAuthorizationCode{}, oauthLoginContextError(ctx)
	}
}

func resolveOAuthCallbackHost(
	ctx context.Context,
	configured string,
	authContext AuthContext,
) (string, error) {
	host := strings.TrimSpace(configured)
	if host != "" {
		return host, nil
	}
	if authContext == nil {
		authContext = DefaultProviderAuthContext()
	}
	for _, name := range []string{
		"GI_OAUTH_CALLBACK_HOST",
		"PI_OAUTH_CALLBACK_HOST",
	} {
		value, ok, err := authContext.Env(
			contextOrBackground(ctx),
			name,
		)
		if err != nil {
			return "", fmt.Errorf(
				"resolve OAuth callback host from %s: %w",
				name,
				err,
			)
		}
		if ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), nil
		}
	}
	return "127.0.0.1", nil
}
