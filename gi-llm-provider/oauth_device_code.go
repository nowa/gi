package gillmprovider

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	defaultOAuthDeviceCodePollInterval = 5 * time.Second
	minOAuthDeviceCodePollInterval     = time.Second
	oauthDeviceCodeSlowDownIncrement   = 5 * time.Second
)

// OAuthDeviceCodePollStatus identifies the outcome of one device-token poll.
type OAuthDeviceCodePollStatus string

const (
	OAuthDeviceCodePending  OAuthDeviceCodePollStatus = "pending"
	OAuthDeviceCodeSlowDown OAuthDeviceCodePollStatus = "slow_down"
	OAuthDeviceCodeFailed   OAuthDeviceCodePollStatus = "failed"
	OAuthDeviceCodeComplete OAuthDeviceCodePollStatus = "complete"
)

// OAuthDeviceCodePollResult is the tagged result returned by one token poll.
// IntervalSeconds is meaningful only for OAuthDeviceCodeSlowDown.
type OAuthDeviceCodePollResult[T any] struct {
	Status          OAuthDeviceCodePollStatus
	Value           T
	Message         string
	IntervalSeconds int
}

// OAuthDeviceCodePollOptions configures an RFC 8628 polling loop.
type OAuthDeviceCodePollOptions[T any] struct {
	IntervalSeconds     int
	ExpiresInSeconds    int
	WaitBeforeFirstPoll bool
	Poll                func(context.Context) (OAuthDeviceCodePollResult[T], error)
}

type oauthDeviceCodeRuntime struct {
	now   func() time.Time
	sleep func(context.Context, time.Duration) error
}

// PollOAuthDeviceCodeFlow polls immediately by default, observes RFC 8628
// slow_down responses, and stops promptly when ctx is cancelled.
func PollOAuthDeviceCodeFlow[T any](
	ctx context.Context,
	options OAuthDeviceCodePollOptions[T],
) (T, error) {
	return pollOAuthDeviceCodeFlow(ctx, options, oauthDeviceCodeRuntime{
		now:   time.Now,
		sleep: sleepWithContext,
	})
}

func pollOAuthDeviceCodeFlow[T any](
	ctx context.Context,
	options OAuthDeviceCodePollOptions[T],
	runtime oauthDeviceCodeRuntime,
) (T, error) {
	var zero T
	ctx = contextOrBackground(ctx)
	if options.Poll == nil {
		return zero, errors.New("OAuth device-code poll function is required")
	}
	if runtime.now == nil {
		runtime.now = time.Now
	}
	if runtime.sleep == nil {
		runtime.sleep = sleepWithContext
	}

	interval := defaultOAuthDeviceCodePollInterval
	if options.IntervalSeconds > 0 {
		interval = oauthDeviceCodeSecondsDuration(options.IntervalSeconds)
	}
	if interval < minOAuthDeviceCodePollInterval {
		interval = minOAuthDeviceCodePollInterval
	}

	var deadline time.Time
	if options.ExpiresInSeconds > 0 {
		deadline = runtime.now().Add(
			oauthDeviceCodeSecondsDuration(options.ExpiresInSeconds),
		)
	}

	slowDownResponses := 0
	if options.WaitBeforeFirstPoll {
		if err := sleepForOAuthPoll(ctx, runtime, interval, deadline); err != nil {
			return zero, err
		}
	}

	for deadline.IsZero() || runtime.now().Before(deadline) {
		if err := oauthLoginContextError(ctx); err != nil {
			return zero, err
		}
		result, err := options.Poll(ctx)
		if err != nil {
			return zero, err
		}
		switch result.Status {
		case OAuthDeviceCodeComplete:
			return result.Value, nil
		case OAuthDeviceCodeFailed:
			if result.Message == "" {
				return zero, errors.New("Device authorization failed")
			}
			return zero, errors.New(result.Message)
		case OAuthDeviceCodeSlowDown:
			slowDownResponses++
			if result.IntervalSeconds > 0 {
				interval = oauthDeviceCodeSecondsDuration(
					result.IntervalSeconds,
				)
				if interval < minOAuthDeviceCodePollInterval {
					interval = minOAuthDeviceCodePollInterval
				}
			} else {
				interval += oauthDeviceCodeSlowDownIncrement
			}
		case OAuthDeviceCodePending:
		default:
			return zero, fmt.Errorf(
				"unknown OAuth device-code poll status %q",
				result.Status,
			)
		}

		if err := sleepForOAuthPoll(ctx, runtime, interval, deadline); err != nil {
			return zero, err
		}
	}

	if slowDownResponses > 0 {
		return zero, errors.New(
			"Device flow timed out after one or more slow_down responses. " +
				"This is often caused by clock drift in WSL or VM environments. " +
				"Please sync or restart the VM clock and try again.",
		)
	}
	return zero, errors.New("Device flow timed out")
}

func oauthDeviceCodeSecondsDuration(seconds int) time.Duration {
	const maxDuration = time.Duration(1<<63 - 1)
	if int64(seconds) > int64(maxDuration/time.Second) {
		return maxDuration
	}
	return time.Duration(seconds) * time.Second
}

func sleepForOAuthPoll(
	ctx context.Context,
	runtime oauthDeviceCodeRuntime,
	interval time.Duration,
	deadline time.Time,
) error {
	if !deadline.IsZero() {
		remaining := deadline.Sub(runtime.now())
		if remaining <= 0 {
			return nil
		}
		if interval > remaining {
			interval = remaining
		}
	}
	if err := runtime.sleep(ctx, interval); err != nil {
		if contextError(ctx) != nil {
			return oauthLoginContextError(ctx)
		}
		return err
	}
	return nil
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func oauthLoginContextError(ctx context.Context) error {
	if err := contextError(ctx); err != nil {
		return fmt.Errorf("Login cancelled: %w", err)
	}
	return nil
}
