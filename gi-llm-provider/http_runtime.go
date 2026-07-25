package gillmprovider

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPRuntimeConfig describes the stable HTTP transport dependencies for a
// group of provider requests. IdleTimeout applies to each socket read,
// including response headers and streaming body chunks. Zero disables the
// timeout.
type HTTPRuntimeConfig struct {
	IdleTimeout time.Duration
	ProxyURL    string
}

// NewHTTPClient constructs an independently owned provider client. It avoids
// mutating http.DefaultTransport, disables HTTP/2 to keep per-connection idle
// deadlines isolated, and uses an explicit proxy when configured.
func NewHTTPClient(config HTTPRuntimeConfig) (*http.Client, error) {
	if config.IdleTimeout < 0 {
		return nil, fmt.Errorf(
			"invalid HTTP idle timeout: %s",
			config.IdleTimeout,
		)
	}
	proxyURL, err := parseHTTPRuntimeProxyURL(config.ProxyURL)
	if err != nil {
		return nil, err
	}

	transport := cloneDefaultHTTPTransport()
	transport.ForceAttemptHTTP2 = false
	transport.DialTLS = nil
	transport.DialTLSContext = nil
	transport.TLSNextProto = map[string]func(
		string,
		*tls.Conn,
	) http.RoundTripper{}
	if proxyURL != nil {
		transport.Proxy = http.ProxyURL(proxyURL)
	} else {
		transport.Proxy = func(
			request *http.Request,
		) (*url.URL, error) {
			return ResolveHTTPProxyURLForTarget(
				request.URL.String(),
			)
		}
	}
	if config.IdleTimeout > 0 {
		baseDial := transport.DialContext
		transport.DialContext = func(
			ctx context.Context,
			network string,
			address string,
		) (net.Conn, error) {
			connection, err := baseDial(ctx, network, address)
			if err != nil {
				return nil, err
			}
			return &httpIdleTimeoutConn{
				Conn:        connection,
				idleTimeout: config.IdleTimeout,
			}, nil
		}
		transport.ResponseHeaderTimeout = config.IdleTimeout
	} else {
		transport.ResponseHeaderTimeout = 0
	}
	return &http.Client{Transport: transport}, nil
}

func cloneDefaultHTTPTransport() *http.Transport {
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		cloned := transport.Clone()
		if cloned.DialContext != nil {
			return cloned
		}
	}
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

func parseHTTPRuntimeProxyURL(value string) (*url.URL, error) {
	proxy := strings.TrimSpace(value)
	if proxy == "" {
		return nil, nil
	}
	parsed, err := url.Parse(proxy)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid proxy URL %q", proxy)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed, nil
	default:
		return nil, fmt.Errorf(
			"%s Got %s:",
			UnsupportedProxyProtocolMessage,
			parsed.Scheme,
		)
	}
}

type httpIdleTimeoutConn struct {
	net.Conn
	idleTimeout time.Duration
}

func (c *httpIdleTimeoutConn) Read(buffer []byte) (int, error) {
	if c.idleTimeout > 0 {
		if err := c.SetReadDeadline(time.Now().Add(c.idleTimeout)); err != nil {
			return 0, err
		}
	}
	return c.Conn.Read(buffer)
}

func httpClientForRequest(
	fallback HTTPDoer,
	options StreamOptions,
) HTTPDoer {
	if options.HTTPClient != nil {
		return options.HTTPClient
	}
	return httpClientOrDefault(fallback)
}
