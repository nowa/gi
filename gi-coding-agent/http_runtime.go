package gicodingagent

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	defaultHTTPIdleTimeoutMS    = 300_000
	disabledProviderTimeoutMS   = math.MaxInt32
	maxHTTPRuntimeTimeoutMillis = int64(math.MaxInt64 / int64(time.Millisecond))
)

type httpIdleTimeoutChoice struct {
	Label     string
	TimeoutMS int
}

var httpIdleTimeoutChoices = []httpIdleTimeoutChoice{
	{Label: "30 sec", TimeoutMS: 30_000},
	{Label: "1 min", TimeoutMS: 60_000},
	{Label: "2 min", TimeoutMS: 120_000},
	{Label: "5 min", TimeoutMS: 300_000},
	{Label: "disabled", TimeoutMS: 0},
}

// ProviderRequestSettings is one immutable projection of mutable application
// settings into provider-facing transport and retry policy.
type ProviderRequestSettings struct {
	Transport               string
	HTTPProxy               string
	HTTP                    llm.HTTPRuntimeConfig
	RequestTimeout          time.Duration
	WebSocketConnectTimeout *time.Duration
	MaxRetries              int
	MaxRetryDelayMS         int
}

type providerRequestSnapshot struct {
	Settings   ProviderRequestSettings
	HTTPClient llm.HTTPDoer
}

// providerRequestRuntime owns the reusable HTTP client derived from settings.
// A settings change constructs the replacement before publishing it, then
// closes idle connections on the retired client.
type providerRequestRuntime struct {
	mu sync.Mutex

	config      llm.HTTPRuntimeConfig
	client      *http.Client
	initialized bool
}

func (r *providerRequestRuntime) snapshot(
	settings *SettingsManager,
) (providerRequestSnapshot, error) {
	requestSettings, err := providerRequestSettings(settings)
	if err != nil {
		return providerRequestSnapshot{}, err
	}
	if err := ApplyHTTPProxySettings(requestSettings.HTTPProxy); err != nil {
		return providerRequestSnapshot{}, err
	}

	r.mu.Lock()
	if r.initialized && r.config == requestSettings.HTTP {
		client := r.client
		r.mu.Unlock()
		return providerRequestSnapshot{
			Settings:   requestSettings,
			HTTPClient: client,
		}, nil
	}
	client, err := llm.NewHTTPClient(requestSettings.HTTP)
	if err != nil {
		r.mu.Unlock()
		return providerRequestSnapshot{}, err
	}
	previous := r.client
	r.config = requestSettings.HTTP
	r.client = client
	r.initialized = true
	r.mu.Unlock()
	if previous != nil {
		previous.CloseIdleConnections()
	}
	return providerRequestSnapshot{
		Settings:   requestSettings,
		HTTPClient: client,
	}, nil
}

func (r *providerRequestRuntime) close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	client := r.client
	r.client = nil
	r.initialized = false
	r.mu.Unlock()
	if client != nil {
		client.CloseIdleConnections()
	}
}

func providerRequestSettings(
	settings *SettingsManager,
) (ProviderRequestSettings, error) {
	if settings == nil {
		timeout := time.Duration(defaultHTTPIdleTimeoutMS) * time.Millisecond
		return ProviderRequestSettings{
			Transport:       "auto",
			HTTP:            llm.HTTPRuntimeConfig{IdleTimeout: timeout},
			RequestTimeout:  timeout,
			MaxRetryDelayMS: defaultProviderMaxRetryDelayMS,
		}, nil
	}

	settings.mu.RLock()
	merged := settings.merged
	httpProxy := strings.TrimSpace(settingsString(settings.global, "httpProxy"))
	settings.mu.RUnlock()

	idleTimeoutMS, configured, err := parseTimeoutSetting(
		merged["httpIdleTimeoutMs"],
		"httpIdleTimeoutMs",
	)
	if err != nil {
		return ProviderRequestSettings{}, err
	}
	if !configured {
		idleTimeoutMS = defaultHTTPIdleTimeoutMS
	}
	webSocketConnectTimeoutMS, webSocketConfigured, err := parseTimeoutSetting(
		merged["websocketConnectTimeoutMs"],
		"websocketConnectTimeoutMs",
	)
	if err != nil {
		return ProviderRequestSettings{}, err
	}

	retrySettings, _ := merged["retry"].(map[string]any)
	providerSettings, _ := retrySettings["provider"].(map[string]any)
	providerTimeoutMS, providerTimeoutConfigured, err := parseTimeoutSetting(
		providerSettings["timeoutMs"],
		"retry.provider.timeoutMs",
	)
	if err != nil {
		return ProviderRequestSettings{}, err
	}

	effectiveTimeoutMS := idleTimeoutMS
	if effectiveTimeoutMS == 0 {
		effectiveTimeoutMS = disabledProviderTimeoutMS
	}
	if providerTimeoutConfigured {
		effectiveTimeoutMS = providerTimeoutMS
	}

	result := ProviderRequestSettings{
		Transport: settingsEnum(
			merged,
			"transport",
			"auto",
			[]string{"sse", "websocket", "websocket-cached", "auto"},
		),
		HTTPProxy: httpProxy,
		HTTP: llm.HTTPRuntimeConfig{
			IdleTimeout: time.Duration(idleTimeoutMS) * time.Millisecond,
		},
		RequestTimeout: time.Duration(effectiveTimeoutMS) * time.Millisecond,
		MaxRetries: settingsValueInt(
			providerSettings["maxRetries"],
			0,
		),
		MaxRetryDelayMS: settingsValueInt(
			providerSettings["maxRetryDelayMs"],
			defaultProviderMaxRetryDelayMS,
		),
	}
	if webSocketConfigured {
		timeout := time.Duration(webSocketConnectTimeoutMS) * time.Millisecond
		result.WebSocketConnectTimeout = &timeout
	}
	return result, nil
}

func (s providerRequestSnapshot) apply(
	options *llm.StreamOptions,
) {
	if options == nil {
		return
	}
	options.Transport = s.Settings.Transport
	options.HTTPClient = s.HTTPClient
	timeout := s.Settings.RequestTimeout
	options.Timeouts.HTTPIdle = &timeout
	options.TimeoutMillis = durationMilliseconds(timeout)
	if s.Settings.WebSocketConnectTimeout != nil {
		connectTimeout := *s.Settings.WebSocketConnectTimeout
		options.Timeouts.WebSocketConnect = &connectTimeout
		options.WebSocketConnectTimeoutMillis =
			durationMilliseconds(connectTimeout)
	}
	options.MaxRetries = s.Settings.MaxRetries
	options.MaxRetryDelayMs = s.Settings.MaxRetryDelayMS
}

func durationMilliseconds(value time.Duration) int {
	return int(value / time.Millisecond)
}

func parseTimeoutSetting(
	value any,
	settingName string,
) (int, bool, error) {
	if value == nil {
		return 0, false, nil
	}
	timeoutMS, ok := parseHTTPIdleTimeoutMS(value)
	if !ok || int64(timeoutMS) > maxHTTPRuntimeTimeoutMillis {
		return 0, false, fmt.Errorf(
			"Invalid %s setting: %v",
			settingName,
			value,
		)
	}
	return timeoutMS, true, nil
}

func parseHTTPIdleTimeoutMS(value any) (int, bool) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if strings.EqualFold(trimmed, "disabled") {
			return 0, true
		}
		if trimmed == "" {
			return 0, false
		}
		number, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, false
		}
		return parseHTTPIdleTimeoutMS(number)
	case int:
		if typed < 0 {
			return 0, false
		}
		return typed, true
	case int64:
		if typed < 0 || typed > int64(math.MaxInt) {
			return 0, false
		}
		return int(typed), true
	case float64:
		if math.IsNaN(typed) ||
			math.IsInf(typed, 0) ||
			typed < 0 ||
			typed > float64(maxHTTPRuntimeTimeoutMillis) {
			return 0, false
		}
		return int(math.Floor(typed)), true
	case json.Number:
		number, err := strconv.ParseFloat(string(typed), 64)
		if err != nil {
			return 0, false
		}
		return parseHTTPIdleTimeoutMS(number)
	default:
		return 0, false
	}
}

func formatHTTPIdleTimeoutMS(timeoutMS int) string {
	for _, choice := range httpIdleTimeoutChoices {
		if choice.TimeoutMS == timeoutMS {
			return choice.Label
		}
	}
	return strconv.FormatFloat(
		float64(timeoutMS)/1000,
		'f',
		-1,
		64,
	) + " sec"
}

func httpIdleTimeoutLabels() []string {
	labels := make([]string, 0, len(httpIdleTimeoutChoices))
	for _, choice := range httpIdleTimeoutChoices {
		labels = append(labels, choice.Label)
	}
	return labels
}

func httpIdleTimeoutMSForLabel(label string) (int, bool) {
	for _, choice := range httpIdleTimeoutChoices {
		if choice.Label == label {
			return choice.TimeoutMS, true
		}
	}
	return 0, false
}

// ApplyHTTPProxySettings projects the global httpProxy setting into the
// conventional process environment without replacing caller-owned values.
func ApplyHTTPProxySettings(httpProxy string) error {
	proxy := strings.TrimSpace(httpProxy)
	if proxy == "" {
		return nil
	}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY"} {
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, proxy); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	return nil
}
