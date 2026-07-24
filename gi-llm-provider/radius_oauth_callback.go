package gillmprovider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	radiusOAuthCallbackAddress = "127.0.0.1:1456"
	radiusOAuthCallbackPath    = "/oauth/callback"
)

type radiusOAuthCallbackServer interface {
	RedirectURI() string
	WaitForCode(context.Context) (string, error)
	Close() error
}

type radiusOAuthCallbackResult struct {
	code string
	err  error
}

type localRadiusOAuthCallbackServer struct {
	server      *http.Server
	redirectURI string
	result      chan radiusOAuthCallbackResult
	resultOnce  sync.Once
	closeOnce   sync.Once
	closeErr    error
}

func startRadiusOAuthCallbackServer(
	ctx context.Context,
	expectedState string,
	address string,
) (radiusOAuthCallbackServer, error) {
	listener, err := (&net.ListenConfig{}).Listen(
		contextOrBackground(ctx),
		"tcp",
		address,
	)
	if err != nil {
		return nil, err
	}
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		listener.Close()
		return nil, err
	}
	if host == "" || host == "::" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	redirect := (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, port),
		Path:   radiusOAuthCallbackPath,
	}).String()
	local := &localRadiusOAuthCallbackServer{
		redirectURI: redirect,
		result:      make(chan radiusOAuthCallbackResult, 1),
	}
	local.server = &http.Server{
		Handler:           local.handler(expectedState),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       15 * time.Second,
	}
	go func() {
		err := local.server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			local.settle(radiusOAuthCallbackResult{err: err})
		}
	}()
	return local, nil
}

func (s *localRadiusOAuthCallbackServer) RedirectURI() string {
	return s.redirectURI
}

func (s *localRadiusOAuthCallbackServer) WaitForCode(
	ctx context.Context,
) (string, error) {
	select {
	case <-contextOrBackground(ctx).Done():
		return "", oauthLoginContextError(ctx)
	case result := <-s.result:
		return result.code, result.err
	}
}

func (s *localRadiusOAuthCallbackServer) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.server.Close()
		s.settle(radiusOAuthCallbackResult{
			err: errors.New("OAuth callback did not complete"),
		})
	})
	return s.closeErr
}

func (s *localRadiusOAuthCallbackServer) handler(
	expectedState string,
) http.Handler {
	return http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path != radiusOAuthCallbackPath {
			sendRadiusOAuthPage(
				writer,
				http.StatusNotFound,
				OAuthErrorHTML("Callback route not found.", ""),
			)
			return
		}
		if request.URL.Query().Get("state") != expectedState {
			sendRadiusOAuthPage(
				writer,
				http.StatusBadRequest,
				OAuthErrorHTML("OAuth state mismatch.", ""),
			)
			return
		}
		if oauthError := request.URL.Query().Get("error"); oauthError != "" {
			description := request.URL.Query().Get("error_description")
			if description == "" {
				description = oauthError
			}
			sendRadiusOAuthPage(
				writer,
				http.StatusBadRequest,
				OAuthErrorHTML(description, ""),
			)
			s.settle(radiusOAuthCallbackResult{
				err: fmt.Errorf("Radius OAuth callback failed: %s", description),
			})
			return
		}
		code := request.URL.Query().Get("code")
		if code == "" {
			sendRadiusOAuthPage(
				writer,
				http.StatusBadRequest,
				OAuthErrorHTML("Missing authorization code.", ""),
			)
			return
		}
		sendRadiusOAuthPage(
			writer,
			http.StatusOK,
			OAuthSuccessHTML(
				"Signed in to Radius. You may now close this page.",
			),
		)
		s.settle(radiusOAuthCallbackResult{code: code})
	})
}

func (s *localRadiusOAuthCallbackServer) settle(
	result radiusOAuthCallbackResult,
) {
	s.resultOnce.Do(func() {
		s.result <- result
	})
}

func sendRadiusOAuthPage(
	writer http.ResponseWriter,
	status int,
	page string,
) {
	writer.Header().Set("content-type", "text/html; charset=utf-8")
	writer.WriteHeader(status)
	_, _ = io.WriteString(writer, page)
}
