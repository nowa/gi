package gillmprovider

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOAuthDeviceCodePollingPiContracts(t *testing.T) {
	start := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)

	t.Run("polls immediately and returns the completed value", func(t *testing.T) {
		clock := newOAuthTestClock(start)
		var pollTimes []time.Time
		result, err := pollOAuthDeviceCodeFlow(
			context.Background(),
			OAuthDeviceCodePollOptions[string]{
				IntervalSeconds:  2,
				ExpiresInSeconds: 30,
				Poll: func(context.Context) (
					OAuthDeviceCodePollResult[string],
					error,
				) {
					pollTimes = append(pollTimes, clock.now())
					if len(pollTimes) == 1 {
						return OAuthDeviceCodePollResult[string]{
							Status: OAuthDeviceCodePending,
						}, nil
					}
					return OAuthDeviceCodePollResult[string]{
						Status: OAuthDeviceCodeComplete,
						Value:  "token",
					}, nil
				},
			},
			clock.runtime(),
		)
		if err != nil || result != "token" {
			t.Fatalf("result=%q err=%v", result, err)
		}
		want := []time.Time{start, start.Add(2 * time.Second)}
		if !equalOAuthPollTimes(pollTimes, want) {
			t.Fatalf("poll times = %v, want %v", pollTimes, want)
		}
	})

	t.Run("can wait before the first poll", func(t *testing.T) {
		clock := newOAuthTestClock(start)
		var pollTimes []time.Time
		result, err := pollOAuthDeviceCodeFlow(
			context.Background(),
			OAuthDeviceCodePollOptions[string]{
				IntervalSeconds:     2,
				ExpiresInSeconds:    30,
				WaitBeforeFirstPoll: true,
				Poll: func(context.Context) (
					OAuthDeviceCodePollResult[string],
					error,
				) {
					pollTimes = append(pollTimes, clock.now())
					return OAuthDeviceCodePollResult[string]{
						Status: OAuthDeviceCodeComplete,
						Value:  "token",
					}, nil
				},
			},
			clock.runtime(),
		)
		if err != nil || result != "token" {
			t.Fatalf("result=%q err=%v", result, err)
		}
		want := []time.Time{start.Add(2 * time.Second)}
		if !equalOAuthPollTimes(pollTimes, want) {
			t.Fatalf("poll times = %v, want %v", pollTimes, want)
		}
	})

	t.Run("increases the interval by 5 seconds after slow_down", func(t *testing.T) {
		clock := newOAuthTestClock(start)
		var pollTimes []time.Time
		statuses := []OAuthDeviceCodePollStatus{
			OAuthDeviceCodeSlowDown,
			OAuthDeviceCodeComplete,
		}
		_, err := pollOAuthDeviceCodeFlow(
			context.Background(),
			OAuthDeviceCodePollOptions[string]{
				IntervalSeconds:  2,
				ExpiresInSeconds: 900,
				Poll: func(context.Context) (
					OAuthDeviceCodePollResult[string],
					error,
				) {
					pollTimes = append(pollTimes, clock.now())
					status := statuses[0]
					statuses = statuses[1:]
					return OAuthDeviceCodePollResult[string]{
						Status: status,
						Value:  "token",
					}, nil
				},
			},
			clock.runtime(),
		)
		if err != nil {
			t.Fatal(err)
		}
		want := []time.Time{start, start.Add(7 * time.Second)}
		if !equalOAuthPollTimes(pollTimes, want) {
			t.Fatalf("poll times = %v, want %v", pollTimes, want)
		}
	})

	t.Run("honors a server-provided slow_down interval", func(t *testing.T) {
		clock := newOAuthTestClock(start)
		var pollTimes []time.Time
		calls := 0
		_, err := pollOAuthDeviceCodeFlow(
			context.Background(),
			OAuthDeviceCodePollOptions[string]{
				IntervalSeconds:  2,
				ExpiresInSeconds: 900,
				Poll: func(context.Context) (
					OAuthDeviceCodePollResult[string],
					error,
				) {
					pollTimes = append(pollTimes, clock.now())
					calls++
					if calls == 1 {
						return OAuthDeviceCodePollResult[string]{
							Status:          OAuthDeviceCodeSlowDown,
							IntervalSeconds: 30,
						}, nil
					}
					return OAuthDeviceCodePollResult[string]{
						Status: OAuthDeviceCodeComplete,
						Value:  "token",
					}, nil
				},
			},
			clock.runtime(),
		)
		if err != nil {
			t.Fatal(err)
		}
		want := []time.Time{start, start.Add(30 * time.Second)}
		if !equalOAuthPollTimes(pollTimes, want) {
			t.Fatalf("poll times = %v, want %v", pollTimes, want)
		}
	})

	t.Run("cancels an in-flight wait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		clock := newOAuthTestClock(start)
		clock.sleep = func(context.Context, time.Duration) error {
			cancel()
			return context.Canceled
		}
		_, err := pollOAuthDeviceCodeFlow(
			ctx,
			OAuthDeviceCodePollOptions[string]{
				IntervalSeconds:  5,
				ExpiresInSeconds: 30,
				Poll: func(context.Context) (
					OAuthDeviceCodePollResult[string],
					error,
				) {
					return OAuthDeviceCodePollResult[string]{
						Status: OAuthDeviceCodePending,
					}, nil
				},
			},
			clock.runtime(),
		)
		if !errors.Is(err, context.Canceled) ||
			!strings.Contains(err.Error(), "Login cancelled") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestGeneratePKCEUsesRandomVerifierAndS256Challenge(t *testing.T) {
	pkce, err := generatePKCE(bytes.NewReader(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkce.Verifier) != 43 || len(pkce.Challenge) != 43 {
		t.Fatalf("PKCE lengths = verifier:%d challenge:%d", len(pkce.Verifier), len(pkce.Challenge))
	}
	if pkce.Verifier != "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" {
		t.Fatalf("verifier = %q", pkce.Verifier)
	}
	if pkce.Challenge != "DwBzhbb51LfusnSGBa_hqYSgo7-j8BTQnip4TOnlzRo" {
		t.Fatalf("challenge = %q", pkce.Challenge)
	}
}

func TestOAuthDeviceCodeSecondsDurationDoesNotOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	if duration := oauthDeviceCodeSecondsDuration(maxInt); duration <= 0 {
		t.Fatalf("duration = %s", duration)
	}
}

type oauthTestClock struct {
	current time.Time
	sleep   func(context.Context, time.Duration) error
}

func newOAuthTestClock(start time.Time) *oauthTestClock {
	clock := &oauthTestClock{current: start}
	clock.sleep = func(ctx context.Context, duration time.Duration) error {
		if err := contextError(ctx); err != nil {
			return err
		}
		clock.current = clock.current.Add(duration)
		return nil
	}
	return clock
}

func (c *oauthTestClock) now() time.Time {
	return c.current
}

func (c *oauthTestClock) runtime() oauthDeviceCodeRuntime {
	return oauthDeviceCodeRuntime{now: c.now, sleep: c.sleep}
}

func equalOAuthPollTimes(left, right []time.Time) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !left[index].Equal(right[index]) {
			return false
		}
	}
	return true
}
