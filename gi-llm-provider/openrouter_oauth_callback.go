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

type openRouterOAuthExchange func(
	context.Context,
	string,
	string,
) (Credential, error)

type openRouterOAuthCallbackOptions struct {
	Host         string
	Path         string
	Verifier     string
	LoginTimeout time.Duration
	Exchange     openRouterOAuthExchange
}

type openRouterOAuthCallbackResult struct {
	credential Credential
	err        error
}

type localOpenRouterOAuthCallbackServer struct {
	server      *http.Server
	callbackURL string
	options     openRouterOAuthCallbackOptions
	flowContext context.Context
	cancelFlow  context.CancelFunc
	result      chan openRouterOAuthCallbackResult
	done        chan struct{}

	mu        sync.Mutex
	claimed   bool
	settled   bool
	timer     *time.Timer
	closeErr  error
	closeOnce sync.Once
}

func startOpenRouterOAuthCallbackServer(
	ctx context.Context,
	options openRouterOAuthCallbackOptions,
) (openRouterOAuthCallbackServer, error) {
	ctx = contextOrBackground(ctx)
	if contextError(ctx) != nil {
		return nil, oauthLoginContextError(ctx)
	}
	if options.Exchange == nil {
		return nil, errors.New(
			"OpenRouter OAuth token exchange is required",
		)
	}
	if options.Path == "" || !strings.HasPrefix(options.Path, "/") {
		return nil, errors.New("OpenRouter OAuth callback path is invalid")
	}
	if options.LoginTimeout <= 0 {
		options.LoginTimeout = defaultOpenRouterOAuthLoginTimeout
	}
	listener, err := (&net.ListenConfig{}).Listen(
		ctx,
		"tcp",
		net.JoinHostPort(options.Host, "0"),
	)
	if err != nil {
		return nil, err
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		listener.Close()
		return nil, err
	}
	callbackURL := (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(options.Host, port),
		Path:   options.Path,
	}).String()
	flowContext, cancelFlow := context.WithCancel(ctx)
	local := &localOpenRouterOAuthCallbackServer{
		callbackURL: callbackURL,
		options:     options,
		flowContext: flowContext,
		cancelFlow:  cancelFlow,
		result:      make(chan openRouterOAuthCallbackResult, 1),
		done:        make(chan struct{}),
	}
	local.server = &http.Server{
		Handler:           local.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      35 * time.Second,
		IdleTimeout:       15 * time.Second,
	}
	go func() {
		serveErr := local.server.Serve(listener)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			local.finish(openRouterOAuthCallbackResult{err: serveErr})
		}
	}()
	local.setTimer(options.LoginTimeout)
	go func() {
		select {
		case <-ctx.Done():
			local.finish(openRouterOAuthCallbackResult{
				err: errors.New("Login cancelled"),
			})
		case <-local.done:
		}
	}()
	return local, nil
}

func (s *localOpenRouterOAuthCallbackServer) CallbackURL() string {
	return s.callbackURL
}

func (s *localOpenRouterOAuthCallbackServer) Wait(
	ctx context.Context,
) (Credential, error) {
	select {
	case result := <-s.result:
		return result.credential, result.err
	case <-contextOrBackground(ctx).Done():
		s.finish(openRouterOAuthCallbackResult{
			err: errors.New("Login cancelled"),
		})
		result := <-s.result
		return result.credential, result.err
	}
}

func (s *localOpenRouterOAuthCallbackServer) Close() error {
	s.finish(openRouterOAuthCallbackResult{
		err: errors.New("Login cancelled"),
	})
	s.shutdown()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeErr
}

func (s *localOpenRouterOAuthCallbackServer) handler() http.Handler {
	return http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet ||
			request.URL.Path != s.options.Path {
			sendOpenRouterOAuthPage(
				writer,
				http.StatusNotFound,
				OAuthErrorHTML("OAuth callback route not found.", ""),
			)
			return
		}
		if s.alreadyUsed() {
			sendOpenRouterOAuthPage(
				writer,
				http.StatusConflict,
				OAuthErrorHTML(
					"This OAuth callback has already been used.",
					"",
				),
			)
			return
		}

		query := request.URL.Query()
		if oauthError := query.Get("error"); oauthError != "" {
			description := query.Get("error_description")
			if description == "" {
				description = oauthError
			}
			sendOpenRouterOAuthPage(
				writer,
				http.StatusBadRequest,
				OAuthErrorHTML(
					"OpenRouter authorization was denied.",
					description,
				),
			)
			s.finish(openRouterOAuthCallbackResult{
				err: fmt.Errorf(
					"OpenRouter authorization failed: %s",
					description,
				),
			})
			return
		}

		code := query.Get("code")
		if code == "" {
			sendOpenRouterOAuthPage(
				writer,
				http.StatusBadRequest,
				OAuthErrorHTML(
					"OpenRouter returned no authorization code.",
					"",
				),
			)
			return
		}
		if !s.claim() {
			sendOpenRouterOAuthPage(
				writer,
				http.StatusConflict,
				OAuthErrorHTML(
					"This OAuth callback has already been used.",
					"",
				),
			)
			return
		}

		credential, err := s.options.Exchange(
			s.flowContext,
			code,
			s.options.Verifier,
		)
		if err != nil {
			sendOpenRouterOAuthPage(
				writer,
				http.StatusBadGateway,
				OAuthErrorHTML(
					"OpenRouter key exchange failed.",
					err.Error(),
				),
			)
			s.finish(openRouterOAuthCallbackResult{err: err})
			return
		}
		sendOpenRouterOAuthPage(
			writer,
			http.StatusOK,
			OAuthSuccessHTML(
				"Signed in to OpenRouter. You may now close this page.",
			),
		)
		s.finish(openRouterOAuthCallbackResult{
			credential: credential,
		})
	})
}

func (s *localOpenRouterOAuthCallbackServer) alreadyUsed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.claimed || s.settled
}

func (s *localOpenRouterOAuthCallbackServer) claim() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed || s.settled {
		return false
	}
	s.claimed = true
	return true
}

func (s *localOpenRouterOAuthCallbackServer) setTimer(
	timeout time.Duration,
) {
	timer := time.AfterFunc(timeout, func() {
		s.finish(openRouterOAuthCallbackResult{
			err: errors.New("OpenRouter OAuth login timed out"),
		})
	})
	s.mu.Lock()
	if s.settled {
		timer.Stop()
	} else {
		s.timer = timer
	}
	s.mu.Unlock()
}

func (s *localOpenRouterOAuthCallbackServer) finish(
	result openRouterOAuthCallbackResult,
) {
	s.mu.Lock()
	if s.settled {
		s.mu.Unlock()
		return
	}
	s.settled = true
	timer := s.timer
	s.mu.Unlock()

	if timer != nil {
		timer.Stop()
	}
	s.cancelFlow()
	s.result <- result
	close(s.done)
	go s.shutdown()
}

func (s *localOpenRouterOAuthCallbackServer) shutdown() {
	s.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			2*time.Second,
		)
		defer cancel()
		err := s.server.Shutdown(ctx)
		if errors.Is(err, context.DeadlineExceeded) {
			err = s.server.Close()
		}
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		s.mu.Lock()
		s.closeErr = err
		s.mu.Unlock()
	})
}

func sendOpenRouterOAuthPage(
	writer http.ResponseWriter,
	status int,
	page string,
) {
	writer.Header().Set("content-type", "text/html; charset=utf-8")
	writer.Header().Set("cache-control", "no-store")
	writer.WriteHeader(status)
	_, _ = io.WriteString(writer, page)
}

func resolveOpenRouterOAuthCallbackHost(
	ctx context.Context,
	configured string,
	authContext AuthContext,
) (string, error) {
	host := strings.TrimSpace(configured)
	if host == "" {
		if authContext == nil {
			authContext = DefaultProviderAuthContext()
		}
		value, ok, err := authContext.Env(
			contextOrBackground(ctx),
			"PI_OAUTH_CALLBACK_HOST",
		)
		if err != nil {
			return "", fmt.Errorf(
				"resolve OpenRouter OAuth callback host: %w",
				err,
			)
		}
		if ok {
			host = strings.TrimSpace(value)
		}
	}
	if host == "" {
		host = defaultOpenRouterOAuthCallbackHost
	}
	if strings.ContainsAny(host, "/?#") ||
		(strings.Contains(host, ":") && net.ParseIP(host) == nil) {
		return "", fmt.Errorf(
			"invalid OpenRouter OAuth callback host %q",
			host,
		)
	}
	return host, nil
}
