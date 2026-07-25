package gicodingagent

import (
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestParseHTTPIdleTimeoutMSPiContract(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int
		ok    bool
	}{
		{name: "nil", value: nil},
		{name: "blank", value: "  "},
		{name: "disabled", value: " DISABLED ", want: 0, ok: true},
		{name: "numeric string floors", value: "1234.9", want: 1234, ok: true},
		{name: "number floors", value: 999.8, want: 999, ok: true},
		{name: "integer", value: 42, want: 42, ok: true},
		{name: "negative", value: -1},
		{name: "not finite", value: math.Inf(1)},
		{
			name:  "duration overflow",
			value: float64(maxHTTPRuntimeTimeoutMillis) + 1,
		},
		{name: "wrong type", value: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := parseHTTPIdleTimeoutMS(test.value)
			if got != test.want || ok != test.ok {
				t.Fatalf(
					"parseHTTPIdleTimeoutMS(%#v) = (%d, %t), want (%d, %t)",
					test.value,
					got,
					ok,
					test.want,
					test.ok,
				)
			}
		})
	}
}

func TestFormatHTTPIdleTimeoutMSPiContract(t *testing.T) {
	tests := map[int]string{
		0:       "disabled",
		30_000:  "30 sec",
		60_000:  "1 min",
		120_000: "2 min",
		300_000: "5 min",
		1_250:   "1.25 sec",
	}
	for timeoutMS, want := range tests {
		if got := formatHTTPIdleTimeoutMS(timeoutMS); got != want {
			t.Fatalf(
				"formatHTTPIdleTimeoutMS(%d) = %q, want %q",
				timeoutMS,
				got,
				want,
			)
		}
	}
}

func TestApplyHTTPProxySettingsPiContract(t *testing.T) {
	t.Run("applies httpProxy to HTTP_PROXY and HTTPS_PROXY", func(t *testing.T) {
		resetHTTPProxyEnv(t)
		if err := ApplyHTTPProxySettings(" http://proxy.example:8080 "); err != nil {
			t.Fatal(err)
		}
		if os.Getenv("HTTP_PROXY") != "http://proxy.example:8080" ||
			os.Getenv("HTTPS_PROXY") != "http://proxy.example:8080" {
			t.Fatalf(
				"proxy env = HTTP_PROXY=%q HTTPS_PROXY=%q",
				os.Getenv("HTTP_PROXY"),
				os.Getenv("HTTPS_PROXY"),
			)
		}
	})

	t.Run("does not override existing proxy env vars", func(t *testing.T) {
		resetHTTPProxyEnv(t)
		if err := os.Setenv("HTTP_PROXY", "http://existing-http.example"); err != nil {
			t.Fatal(err)
		}
		if err := os.Setenv("HTTPS_PROXY", "http://existing-https.example"); err != nil {
			t.Fatal(err)
		}
		if err := ApplyHTTPProxySettings("http://ignored.example"); err != nil {
			t.Fatal(err)
		}
		if os.Getenv("HTTP_PROXY") != "http://existing-http.example" ||
			os.Getenv("HTTPS_PROXY") != "http://existing-https.example" {
			t.Fatalf(
				"existing proxy env changed: HTTP_PROXY=%q HTTPS_PROXY=%q",
				os.Getenv("HTTP_PROXY"),
				os.Getenv("HTTPS_PROXY"),
			)
		}
	})

	t.Run("ignores blank values", func(t *testing.T) {
		resetHTTPProxyEnv(t)
		if err := ApplyHTTPProxySettings(" \t "); err != nil {
			t.Fatal(err)
		}
		if _, ok := os.LookupEnv("HTTP_PROXY"); ok {
			t.Fatal("HTTP_PROXY was set")
		}
		if _, ok := os.LookupEnv("HTTPS_PROXY"); ok {
			t.Fatal("HTTPS_PROXY was set")
		}
	})
}

func TestProviderRequestSettingsSnapshotAndPrecedence(t *testing.T) {
	settings := NewInMemorySettingsManager(map[string]any{
		"transport":                 "websocket-cached",
		"httpProxy":                 " http://proxy.example:8080 ",
		"httpIdleTimeoutMs":         "disabled",
		"websocketConnectTimeoutMs": 0,
		"retry": map[string]any{
			"provider": map[string]any{
				"timeoutMs":       1250.9,
				"maxRetries":      3,
				"maxRetryDelayMs": 4000,
			},
		},
	})
	snapshot, err := providerRequestSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Transport != "websocket-cached" ||
		snapshot.HTTPProxy != "http://proxy.example:8080" ||
		snapshot.HTTP.IdleTimeout != 0 ||
		snapshot.RequestTimeout != 1250*time.Millisecond ||
		snapshot.WebSocketConnectTimeout == nil ||
		*snapshot.WebSocketConnectTimeout != 0 ||
		snapshot.MaxRetries != 3 ||
		snapshot.MaxRetryDelayMS != 4000 {
		t.Fatalf("provider request settings = %#v", snapshot)
	}

	settings = NewInMemorySettingsManager(map[string]any{
		"httpIdleTimeoutMs": 0,
	})
	snapshot, err = providerRequestSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.HTTP.IdleTimeout != 0 ||
		snapshot.RequestTimeout !=
			time.Duration(disabledProviderTimeoutMS)*time.Millisecond {
		t.Fatalf("disabled timeout settings = %#v", snapshot)
	}
}

func TestProviderRequestSettingsRejectInvalidTimeouts(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]any
		want     string
	}{
		{
			name:     "HTTP idle",
			settings: map[string]any{"httpIdleTimeoutMs": -1},
			want:     "Invalid httpIdleTimeoutMs setting",
		},
		{
			name:     "WebSocket connect",
			settings: map[string]any{"websocketConnectTimeoutMs": "later"},
			want:     "Invalid websocketConnectTimeoutMs setting",
		},
		{
			name: "provider request",
			settings: map[string]any{
				"retry": map[string]any{
					"provider": map[string]any{"timeoutMs": -1},
				},
			},
			want: "Invalid retry.provider.timeoutMs setting",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := providerRequestSettings(
				NewInMemorySettingsManager(test.settings),
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSettingsManagerGoTimeoutAccessors(t *testing.T) {
	settings := NewInMemorySettingsManager(map[string]any{
		"httpIdleTimeoutMs":         "1250.9",
		"websocketConnectTimeoutMs": "disabled",
	})
	idleTimeout, err := settings.HTTPIdleTimeout()
	if err != nil {
		t.Fatal(err)
	}
	connectTimeout, err := settings.WebSocketConnectTimeout()
	if err != nil {
		t.Fatal(err)
	}
	if idleTimeout != 1250*time.Millisecond ||
		connectTimeout == nil ||
		*connectTimeout != 0 {
		t.Fatalf(
			"idle=%s websocket=%v",
			idleTimeout,
			connectTimeout,
		)
	}

	if err := settings.SetHTTPIdleTimeoutMS(30_000); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetHTTPIdleTimeoutMS(-1); err == nil {
		t.Fatal("negative timeout setter succeeded")
	}
	idleTimeout, err = settings.HTTPIdleTimeout()
	if err != nil {
		t.Fatal(err)
	}
	if idleTimeout != 30*time.Second {
		t.Fatalf(
			"invalid setter changed timeout to %s",
			idleTimeout,
		)
	}
}

func TestProviderRequestRuntimeReusesAndReplacesHTTPClient(t *testing.T) {
	resetHTTPProxyEnv(t)
	settings := NewInMemorySettingsManager(nil)
	var runtime providerRequestRuntime
	t.Cleanup(runtime.close)

	first, err := runtime.snapshot(settings)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtime.snapshot(settings)
	if err != nil {
		t.Fatal(err)
	}
	if first.HTTPClient != second.HTTPClient {
		t.Fatal("unchanged settings should reuse the HTTP client")
	}

	if err := settings.SetHTTPIdleTimeoutMS(120_000); err != nil {
		t.Fatal(err)
	}
	third, err := runtime.snapshot(settings)
	if err != nil {
		t.Fatal(err)
	}
	if third.HTTPClient == first.HTTPClient {
		t.Fatal("changed transport settings should replace the HTTP client")
	}
	if third.Settings.HTTP.IdleTimeout != 2*time.Minute {
		t.Fatalf(
			"updated idle timeout = %s",
			third.Settings.HTTP.IdleTimeout,
		)
	}
}

func TestSDKStreamOptionsForwardsWebSocketConnectTimeoutFromSettings(
	t *testing.T,
) {
	t.Run("forwards websocketConnectTimeoutMs from settings", func(t *testing.T) {
		settings := NewInMemorySettingsManager(map[string]any{
			"websocketConnectTimeoutMs": 0,
		})
		var runtime providerRequestRuntime
		t.Cleanup(runtime.close)
		snapshot, err := runtime.snapshot(settings)
		if err != nil {
			t.Fatal(err)
		}
		var options llm.StreamOptions
		snapshot.apply(&options)
		if options.Timeouts.WebSocketConnect == nil ||
			*options.Timeouts.WebSocketConnect != 0 {
			t.Fatalf(
				"WebSocket timeout = %#v, want explicit zero",
				options.Timeouts.WebSocketConnect,
			)
		}
		if options.TimeoutMillis != defaultHTTPIdleTimeoutMS ||
			options.HTTPClient == nil {
			t.Fatalf("stream options = %#v", options)
		}
	})
}

func TestProviderRequestRuntimeConcurrentSnapshots(t *testing.T) {
	settings := NewInMemorySettingsManager(nil)
	var runtime providerRequestRuntime
	t.Cleanup(runtime.close)

	const iterations = 40
	var wait sync.WaitGroup
	errorsFound := make(chan error, iterations*3)
	wait.Add(1)
	go func() {
		defer wait.Done()
		for index := 0; index < iterations; index++ {
			timeoutMS := 30_000
			if index%2 == 0 {
				timeoutMS = 120_000
			}
			if err := settings.SetHTTPIdleTimeoutMS(timeoutMS); err != nil {
				errorsFound <- err
			}
		}
	}()
	for range 3 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range iterations {
				snapshot, err := runtime.snapshot(settings)
				if err != nil {
					errorsFound <- err
					continue
				}
				if snapshot.HTTPClient == nil {
					errorsFound <- errMissingProviderHTTPClient
				}
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
}

func TestModelRuntimeCloneDetachesProviderTimeoutPointers(t *testing.T) {
	idle := 5 * time.Second
	connect := 2 * time.Second
	cloned := cloneRuntimeStreamOptions(llm.StreamOptions{
		Timeouts: llm.StreamTimeouts{
			HTTPIdle:         &idle,
			WebSocketConnect: &connect,
		},
	})
	idle = time.Second
	connect = time.Second
	if cloned.Timeouts.HTTPIdle == nil ||
		*cloned.Timeouts.HTTPIdle != 5*time.Second ||
		cloned.Timeouts.WebSocketConnect == nil ||
		*cloned.Timeouts.WebSocketConnect != 2*time.Second {
		t.Fatalf("cloned timeouts = %#v", cloned.Timeouts)
	}
}

var errMissingProviderHTTPClient = errors.New(
	"provider request snapshot has no HTTP client",
)

func resetHTTPProxyEnv(t *testing.T) {
	t.Helper()
	keys := []string{"HTTP_PROXY", "HTTPS_PROXY"}
	type previousValue struct {
		value  string
		exists bool
	}
	previous := make(map[string]previousValue, len(keys))
	for _, key := range keys {
		value, exists := os.LookupEnv(key)
		previous[key] = previousValue{value: value, exists: exists}
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, key := range keys {
			value := previous[key]
			if value.exists {
				_ = os.Setenv(key, value.value)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	})
}
