package gillmprovider

import (
	"fmt"
	"time"
)

type openAICodexExecutionOptions struct {
	transport                string
	sseResponseHeaderTimeout time.Duration
	webSocketConnectTimeout  time.Duration
	webSocketIdleTimeout     time.Duration
	maxRetries               int
	maxRetryDelay            time.Duration
}

func prepareOpenAICodexExecutionOptions(
	options SimpleStreamOptions,
) (openAICodexExecutionOptions, error) {
	responseTimeout := time.Duration(0)
	var err error
	if options.Timeouts.HTTPIdle != nil {
		responseTimeout, err = openAICodexValidatedDuration(
			"timeoutMs",
			*options.Timeouts.HTTPIdle,
		)
	} else {
		responseTimeout, err = openAICodexDurationFromMillis(
			"timeoutMs",
			options.TimeoutMillis,
		)
	}
	if err != nil {
		return openAICodexExecutionOptions{}, err
	}
	connectTimeout := defaultOpenAICodexWebSocketConnectTimeout
	if options.Timeouts.WebSocketConnect != nil {
		connectTimeout, err = openAICodexValidatedDuration(
			"websocketConnectTimeoutMs",
			*options.Timeouts.WebSocketConnect,
		)
	} else if options.WebSocketConnectTimeoutMillis != 0 {
		connectTimeout, err = openAICodexDurationFromMillis(
			"websocketConnectTimeoutMs",
			options.WebSocketConnectTimeoutMillis,
		)
		if err != nil {
			return openAICodexExecutionOptions{}, err
		}
	}

	maxRetryDelay := DefaultMaxProviderRetryDelay
	switch {
	case options.MaxRetryDelayMs > 0:
		maxRetryDelay, err = openAICodexDurationFromMillis(
			"maxRetryDelayMs",
			options.MaxRetryDelayMs,
		)
		if err != nil {
			return openAICodexExecutionOptions{}, err
		}
	case options.MaxRetryDelayMs < 0:
		// A negative value is the Go API's explicit opt-out because an int
		// zero value represents Pi's omitted option and therefore its default.
		maxRetryDelay = 0
	}

	return openAICodexExecutionOptions{
		transport:                normalizeOpenAICodexTransport(options.Transport),
		sseResponseHeaderTimeout: responseTimeout,
		webSocketConnectTimeout:  connectTimeout,
		webSocketIdleTimeout:     responseTimeout,
		maxRetries:               max(options.MaxRetries, 0),
		maxRetryDelay:            maxRetryDelay,
	}, nil
}

func openAICodexValidatedDuration(
	name string,
	value time.Duration,
) (time.Duration, error) {
	if value < 0 {
		return 0, fmt.Errorf("Invalid %s: %s", name, value)
	}
	return value, nil
}

func openAICodexDurationFromMillis(name string, value int) (time.Duration, error) {
	if value < 0 {
		return 0, fmt.Errorf("Invalid %s: %d", name, value)
	}
	const maxDuration = time.Duration(1<<63 - 1)
	if int64(value) > int64(maxDuration/time.Millisecond) {
		return 0, fmt.Errorf("Invalid %s: %d", name, value)
	}
	return time.Duration(value) * time.Millisecond, nil
}
