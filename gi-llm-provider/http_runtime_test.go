package gillmprovider

import (
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNewHTTPClientBuildsIsolatedGoTransport(t *testing.T) {
	client, err := NewHTTPClient(HTTPRuntimeConfig{
		IdleTimeout: 1250 * time.Millisecond,
		ProxyURL:    " http://proxy.example:8080 ",
	})
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", client.Transport)
	}
	if transport == http.DefaultTransport {
		t.Fatal("provider transport must not mutate or reuse http.DefaultTransport")
	}
	if transport.ForceAttemptHTTP2 ||
		transport.DialTLS != nil ||
		transport.DialTLSContext != nil ||
		transport.TLSNextProto == nil ||
		transport.ResponseHeaderTimeout != 1250*time.Millisecond {
		t.Fatalf("transport policy = %#v", transport)
	}
	request, err := http.NewRequest(
		http.MethodGet,
		"https://api.example.test/v1",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := transport.Proxy(request)
	if err != nil {
		t.Fatal(err)
	}
	if proxy == nil || proxy.String() != "http://proxy.example:8080" {
		t.Fatalf("proxy = %v", proxy)
	}
}

func TestNewHTTPClientValidatesConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config HTTPRuntimeConfig
	}{
		{
			name:   "negative timeout",
			config: HTTPRuntimeConfig{IdleTimeout: -time.Millisecond},
		},
		{
			name:   "invalid proxy",
			config: HTTPRuntimeConfig{ProxyURL: "not a URL"},
		},
		{
			name:   "unsupported proxy",
			config: HTTPRuntimeConfig{ProxyURL: "socks5://proxy.example:1080"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewHTTPClient(test.config); err == nil {
				t.Fatalf("NewHTTPClient(%#v) succeeded", test.config)
			}
		})
	}
}

func TestNewHTTPClientReadsAmbientProxyWithoutGlobalCache(t *testing.T) {
	for _, key := range []string{
		"http_proxy",
		"HTTP_PROXY",
		"https_proxy",
		"HTTPS_PROXY",
		"all_proxy",
		"ALL_PROXY",
		"no_proxy",
		"NO_PROXY",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("HTTPS_PROXY", "http://first-proxy.example:8080")
	client, err := NewHTTPClient(HTTPRuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	transport := client.Transport.(*http.Transport)
	request, err := http.NewRequest(
		http.MethodGet,
		"https://api.example.test/v1",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := transport.Proxy(request)
	if err != nil {
		t.Fatal(err)
	}
	if proxy == nil ||
		proxy.String() != "http://first-proxy.example:8080" {
		t.Fatalf("first proxy = %v", proxy)
	}

	t.Setenv("HTTPS_PROXY", "http://second-proxy.example:8080")
	proxy, err = transport.Proxy(request)
	if err != nil {
		t.Fatal(err)
	}
	if proxy == nil ||
		proxy.String() != "http://second-proxy.example:8080" {
		t.Fatalf("updated proxy = %v", proxy)
	}
}

func TestHTTPIdleTimeoutConnRefreshesReadDeadline(t *testing.T) {
	connection := &recordingHTTPRuntimeConn{}
	wrapped := &httpIdleTimeoutConn{
		Conn:        connection,
		idleTimeout: 250 * time.Millisecond,
	}
	started := time.Now()
	if _, err := wrapped.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("Read error = %v", err)
	}
	if connection.readDeadline.Before(started.Add(200*time.Millisecond)) ||
		connection.readDeadline.After(time.Now().Add(300*time.Millisecond)) {
		t.Fatalf(
			"read deadline = %s, want approximately 250ms from now",
			connection.readDeadline,
		)
	}
}

func TestHTTPClientForRequestPrefersExplicitDependency(t *testing.T) {
	fallback := httpRuntimeDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("fallback")
	})
	override := httpRuntimeDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("override")
	})
	if _, err := httpClientForRequest(
		fallback,
		StreamOptions{},
	).Do(nil); err == nil || err.Error() != "fallback" {
		t.Fatalf("default client error = %v, want fallback", err)
	}
	if _, err := httpClientForRequest(
		fallback,
		StreamOptions{HTTPClient: override},
	).Do(nil); err == nil || err.Error() != "override" {
		t.Fatalf("request client error = %v, want override", err)
	}
}

func TestCloneStreamOptionsClonesPresenceAwareTimeouts(t *testing.T) {
	idle := 5 * time.Second
	connect := 2 * time.Second
	original := StreamOptions{
		Timeouts: StreamTimeouts{
			HTTPIdle:         &idle,
			WebSocketConnect: &connect,
		},
	}
	cloned := cloneStreamOptions(original)
	idle = time.Second
	connect = time.Second
	if cloned.Timeouts.HTTPIdle == nil ||
		*cloned.Timeouts.HTTPIdle != 5*time.Second ||
		cloned.Timeouts.WebSocketConnect == nil ||
		*cloned.Timeouts.WebSocketConnect != 2*time.Second {
		t.Fatalf("cloned timeouts = %#v", cloned.Timeouts)
	}
}

type httpRuntimeDoerFunc func(*http.Request) (*http.Response, error)

func (f httpRuntimeDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

type recordingHTTPRuntimeConn struct {
	readDeadline time.Time
}

func (c *recordingHTTPRuntimeConn) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (c *recordingHTTPRuntimeConn) Write(buffer []byte) (int, error) {
	return len(buffer), nil
}

func (c *recordingHTTPRuntimeConn) Close() error {
	return nil
}

func (c *recordingHTTPRuntimeConn) LocalAddr() net.Addr {
	return httpRuntimeAddr("local")
}

func (c *recordingHTTPRuntimeConn) RemoteAddr() net.Addr {
	return httpRuntimeAddr("remote")
}

func (c *recordingHTTPRuntimeConn) SetDeadline(time.Time) error {
	return nil
}

func (c *recordingHTTPRuntimeConn) SetReadDeadline(deadline time.Time) error {
	c.readDeadline = deadline
	return nil
}

func (c *recordingHTTPRuntimeConn) SetWriteDeadline(time.Time) error {
	return nil
}

type httpRuntimeAddr string

func (a httpRuntimeAddr) Network() string {
	return string(a)
}

func (a httpRuntimeAddr) String() string {
	return string(a)
}

var _ net.Conn = (*recordingHTTPRuntimeConn)(nil)
