package gillmprovider

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const DefaultMaxProviderRetryDelay = 60 * time.Second

var nonRetryableProviderLimitErrorPattern = regexp.MustCompile(`(?i)(GoUsageLimitError|FreeUsageLimitError|Monthly usage limit reached|available balance|insufficient_quota|out of budget|quota exceeded|billing)`)

var retryableProviderErrorPattern = regexp.MustCompile(`(?i)(overloaded|rate.?limit|too many requests|429|500|502|503|504|524|service.?unavailable|server.?error|internal.?error|provider.?returned.?error|network.?error|connection.?error|connection.?refused|connection.?lost|other side closed|fetch failed|getaddrinfo|ENOTFOUND|EAI_AGAIN|upstream.?connect|reset before headers|socket hang up|socket connection was closed|timed? out|timeout|terminated|websocket.?closed|websocket.?error|ended without|stream ended before message_stop|stream ended before a terminal response event|http2 request did not get a response|retry delay|you can retry your request|try your request again|please retry your request|ResourceExhausted)`)

// IsRetryableAssistantError classifies a terminal assistant error using the
// shared Pi-compatible provider and transport patterns. Policy and retry state
// remain owned by the calling runtime.
func IsRetryableAssistantError(message Message) bool {
	if message.StopReason != StopReasonError || message.ErrorMessage == "" {
		return false
	}
	if nonRetryableProviderLimitErrorPattern.MatchString(message.ErrorMessage) {
		return false
	}
	return retryableProviderErrorPattern.MatchString(message.ErrorMessage)
}

// RetryPolicy is the runtime policy for retrying terminal assistant messages.
// It intentionally contains no mutable attempt state and is safe to share.
type RetryPolicy struct {
	Enabled    bool
	MaxRetries int
	BaseDelay  time.Duration
}

// RetryAttempt describes one scheduled retry. Attempt is one-indexed and
// MaxAttempts is the configured number of retries, excluding the initial call.
type RetryAttempt struct {
	Attempt      int
	MaxAttempts  int
	Delay        time.Duration
	ErrorMessage string
}

// RetryResult is emitted exactly once after at least one retry was scheduled.
type RetryResult struct {
	Success    bool
	Attempt    int
	FinalError string
}

// RetryCallbacks lets stateful runtimes project the stateless retry algorithm
// into lifecycle events without moving UI/session state into this package.
type RetryCallbacks struct {
	OnRetryScheduled    func(RetryAttempt)
	OnRetryAttemptStart func(attempt int)
	OnRetryFinished     func(RetryResult)
}

// RetryAssistantCall applies bounded exponential backoff to assistant error
// messages. Go errors from produce remain Go errors and are never reclassified;
// provider adapters should first convert terminal provider failures to Message.
func RetryAssistantCall(
	ctx context.Context,
	policy RetryPolicy,
	callbacks RetryCallbacks,
	produce func(context.Context) (Message, error),
) (Message, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if produce == nil {
		return Message{}, errors.New("assistant call function is required")
	}

	maxRetries := 0
	if policy.Enabled {
		maxRetries = max(policy.MaxRetries, 0)
	}
	attempt := 0
	var lastRetry *RetryAttempt
	for {
		response, err := produce(ctx)
		if err != nil {
			return Message{}, err
		}
		if response.StopReason == StopReasonAborted {
			if lastRetry != nil && callbacks.OnRetryFinished != nil {
				callbacks.OnRetryFinished(RetryResult{Attempt: lastRetry.Attempt})
			}
			return response, nil
		}
		if response.StopReason != StopReasonError {
			if lastRetry != nil && callbacks.OnRetryFinished != nil {
				callbacks.OnRetryFinished(RetryResult{Success: true, Attempt: lastRetry.Attempt})
			}
			return response, nil
		}
		if attempt >= maxRetries || !IsRetryableAssistantError(response) {
			if lastRetry != nil && callbacks.OnRetryFinished != nil {
				callbacks.OnRetryFinished(RetryResult{
					Attempt:    lastRetry.Attempt,
					FinalError: response.ErrorMessage,
				})
			}
			return response, nil
		}

		attempt++
		retry := RetryAttempt{
			Attempt:      attempt,
			MaxAttempts:  maxRetries,
			Delay:        exponentialRetryDelay(policy.BaseDelay, attempt),
			ErrorMessage: retryFirstNonEmpty(response.ErrorMessage, "Unknown error"),
		}
		lastRetry = &retry
		if callbacks.OnRetryScheduled != nil {
			callbacks.OnRetryScheduled(retry)
		}
		if err := providerRetrySleep(ctx, retry.Delay, nil); err != nil {
			if callbacks.OnRetryFinished != nil {
				callbacks.OnRetryFinished(RetryResult{
					Attempt:    retry.Attempt,
					FinalError: retry.ErrorMessage,
				})
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				response.StopReason = StopReasonAborted
				response.ErrorMessage = ""
				return response, nil
			}
			return Message{}, err
		}
		if callbacks.OnRetryAttemptStart != nil {
			callbacks.OnRetryAttemptStart(attempt)
		}
	}
}

// ProviderRetryOptions configures retries around one provider request. A
// non-positive BaseDelay uses 500ms. A nil MaxRetryDelay applies
// DefaultMaxProviderRetryDelay; a pointer to zero explicitly disables the
// server-requested delay cap.
type ProviderRetryOptions struct {
	MaxRetries    int
	MaxRetryDelay *time.Duration
	BaseDelay     time.Duration
	Now           func() time.Time
	Sleep         func(context.Context, time.Duration) error
	Jitter        func() float64
}

// RetryProviderRequest retries only typed ProviderError values. Transport code
// should wrap network failures with newProviderTransportError so ordinary
// application errors fail fast.
func RetryProviderRequest[T any](ctx context.Context, options ProviderRetryOptions, request func(context.Context) (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		ctx = context.Background()
	}
	if request == nil {
		return zero, errors.New("provider request function is required")
	}

	maxRetries := max(options.MaxRetries, 0)
	for retryIndex := 0; ; retryIndex++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		value, err := request(ctx)
		if err == nil {
			return value, nil
		}
		if retryIndex >= maxRetries || !IsRetryableProviderError(err) {
			return zero, err
		}

		delay, delayErr := providerRetryDelay(err, retryIndex, options)
		if delayErr != nil {
			return zero, delayErr
		}
		if err := providerRetrySleep(ctx, delay, options.Sleep); err != nil {
			return zero, err
		}
	}
}

// IsRetryableProviderError mirrors the OpenAI and Anthropic SDK status/header
// policy while using Go's typed error chain.
func IsRetryableProviderError(err error) bool {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return false
	}
	switch strings.ToLower(providerErr.Headers.Get("x-should-retry")) {
	case "true":
		return true
	case "false":
		return false
	}
	if providerErr.StatusCode == 0 {
		return true
	}
	return providerErr.StatusCode == http.StatusRequestTimeout ||
		providerErr.StatusCode == http.StatusConflict ||
		providerErr.StatusCode == http.StatusTooManyRequests ||
		providerErr.StatusCode >= http.StatusInternalServerError
}

func providerRetryDelay(err error, retryIndex int, options ProviderRetryOptions) (time.Duration, error) {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return 0, err
	}

	if value := providerErr.Headers.Get("retry-after-ms"); value != "" {
		if milliseconds, parseErr := strconv.ParseFloat(value, 64); parseErr == nil {
			return validateProviderRetryDelay(
				time.Duration(milliseconds*float64(time.Millisecond)),
				options.MaxRetryDelay,
				providerErr.Error(),
			)
		}
	}
	if value := providerErr.Headers.Get("retry-after"); value != "" {
		if seconds, parseErr := strconv.ParseFloat(value, 64); parseErr == nil {
			return validateProviderRetryDelay(
				time.Duration(seconds*float64(time.Second)),
				options.MaxRetryDelay,
				providerErr.Error(),
			)
		}
		now := time.Now
		if options.Now != nil {
			now = options.Now
		}
		if retryAt, parseErr := http.ParseTime(value); parseErr == nil {
			return validateProviderRetryDelay(retryAt.Sub(now()), options.MaxRetryDelay, providerErr.Error())
		}
	}

	if retryIndex < 0 {
		retryIndex = 0
	}
	baseDelay := 500 * time.Millisecond
	if options.BaseDelay > 0 {
		baseDelay = options.BaseDelay
	}
	exponential := min(
		exponentialRetryDelay(baseDelay, retryIndex+1),
		8*time.Second,
	)
	jitter := rand.Float64
	if options.Jitter != nil {
		jitter = options.Jitter
	}
	factor := 1 - clampFloat64(jitter(), 0, 1)*0.25
	return time.Duration(float64(exponential) * factor), nil
}

func validateProviderRetryDelay(delay time.Duration, configuredMax *time.Duration, providerErrorMessage string) (time.Duration, error) {
	maxDelay := DefaultMaxProviderRetryDelay
	if configuredMax != nil {
		maxDelay = *configuredMax
	}
	if maxDelay > 0 && delay > maxDelay {
		return 0, fmt.Errorf(
			"server requested %ds retry delay (max: %ds). %s",
			int64(math.Ceil(delay.Seconds())),
			int64(math.Ceil(maxDelay.Seconds())),
			providerErrorMessage,
		)
	}
	return max(delay, 0), nil
}

func providerRetrySleep(ctx context.Context, delay time.Duration, sleep func(context.Context, time.Duration) error) error {
	if sleep != nil {
		return sleep(ctx, max(delay, 0))
	}
	timer := time.NewTimer(max(delay, 0))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func clampFloat64(value, minimum, maximum float64) float64 {
	return min(max(value, minimum), maximum)
}

func exponentialRetryDelay(baseDelay time.Duration, attempt int) time.Duration {
	if baseDelay <= 0 || attempt <= 0 {
		return 0
	}
	const maxShift = 30
	multiplier := time.Duration(1 << min(attempt-1, maxShift))
	const maxDuration = time.Duration(1<<63 - 1)
	if baseDelay > maxDuration/multiplier {
		return maxDuration
	}
	return baseDelay * multiplier
}

func retryFirstNonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
