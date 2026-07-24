package gillmprovider

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIsRetryableAssistantErrorMatchesProviderGuidance(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		message Message
		want    bool
	}{
		{
			name:    "explicit retry guidance",
			message: retryTestAssistantError("You can retry your request"),
			want:    true,
		},
		{
			name:    "network socket drop",
			message: retryTestAssistantError("other side closed"),
			want:    true,
		},
		{
			name:    "responses early eof",
			message: retryTestAssistantError("stream ended before a terminal response event"),
			want:    true,
		},
		{
			name:    "quota wins over status",
			message: retryTestAssistantError("429 quota exceeded"),
			want:    false,
		},
		{
			name:    "provider limit",
			message: retryTestAssistantError("FreeUsageLimitError"),
			want:    false,
		},
		{
			name:    "ordinary provider error",
			message: retryTestAssistantError("invalid_api_key"),
			want:    false,
		},
		{
			name: "success",
			message: Message{
				Role:       RoleAssistant,
				StopReason: StopReasonStop,
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsRetryableAssistantError(tc.message); got != tc.want {
				t.Fatalf("IsRetryableAssistantError(%q) = %t, want %t", tc.message.ErrorMessage, got, tc.want)
			}
		})
	}
}

func TestRetryAssistantCallReturnsSuccessWithoutRetrying(t *testing.T) {
	t.Parallel()

	calls := 0
	result, err := RetryAssistantCall(
		context.Background(),
		RetryPolicy{Enabled: true, MaxRetries: 3},
		RetryCallbacks{},
		func(context.Context) (Message, error) {
			calls++
			return AssistantMessage([]ContentPart{Text("ok")}, StopReasonStop, Model{}), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || result.StopReason != StopReasonStop {
		t.Fatalf("calls=%d result=%#v", calls, result)
	}
}

func TestRetryAssistantCallDoesNotRetryAbortedOrPermanentErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		message Message
	}{
		{name: "does not retry an aborted message", message: Message{Role: RoleAssistant, StopReason: StopReasonAborted}},
		{name: "does not retry a non-retryable error quota billing", message: retryTestAssistantError("insufficient_quota")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			scheduled := 0
			result, err := RetryAssistantCall(
				context.Background(),
				RetryPolicy{Enabled: true, MaxRetries: 3},
				RetryCallbacks{OnRetryScheduled: func(RetryAttempt) { scheduled++ }},
				func(context.Context) (Message, error) {
					calls++
					return tc.message, nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || scheduled != 0 || result.StopReason != tc.message.StopReason {
				t.Fatalf("calls=%d scheduled=%d result=%#v", calls, scheduled, result)
			}
		})
	}
}

func TestRetryAssistantCallExhaustsRetriesAndReportsFinalError(t *testing.T) {
	t.Parallel()

	calls := 0
	var scheduled []RetryAttempt
	var finished []RetryResult
	result, err := RetryAssistantCall(
		context.Background(),
		RetryPolicy{Enabled: true, MaxRetries: 3},
		RetryCallbacks{
			OnRetryScheduled: func(attempt RetryAttempt) { scheduled = append(scheduled, attempt) },
			OnRetryFinished:  func(result RetryResult) { finished = append(finished, result) },
		},
		func(context.Context) (Message, error) {
			calls++
			return retryTestAssistantError("terminated"), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopReasonError || calls != 4 || len(scheduled) != 3 {
		t.Fatalf("result=%#v calls=%d scheduled=%#v", result, calls, scheduled)
	}
	wantFinished := []RetryResult{{Attempt: 3, FinalError: "terminated"}}
	if !reflect.DeepEqual(finished, wantFinished) {
		t.Fatalf("finished = %#v, want %#v", finished, wantFinished)
	}
}

func TestRetryAssistantCallStopsAfterRecovery(t *testing.T) {
	t.Parallel()

	calls := 0
	var finished []RetryResult
	result, err := RetryAssistantCall(
		context.Background(),
		RetryPolicy{Enabled: true, MaxRetries: 3},
		RetryCallbacks{OnRetryFinished: func(result RetryResult) { finished = append(finished, result) }},
		func(context.Context) (Message, error) {
			calls++
			if calls < 3 {
				return retryTestAssistantError("terminated"), nil
			}
			return AssistantMessage([]ContentPart{Text("recovered")}, StopReasonStop, Model{}), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || len(result.Content) != 1 || result.Content[0].Text != "recovered" {
		t.Fatalf("calls=%d result=%#v", calls, result)
	}
	if !reflect.DeepEqual(finished, []RetryResult{{Success: true, Attempt: 2}}) {
		t.Fatalf("finished = %#v", finished)
	}
}

func TestRetryAssistantCallReportsAbortedRetriedCallAsUnsuccessful(t *testing.T) {
	t.Parallel()

	calls := 0
	var finished []RetryResult
	result, err := RetryAssistantCall(
		context.Background(),
		RetryPolicy{Enabled: true, MaxRetries: 3},
		RetryCallbacks{OnRetryFinished: func(result RetryResult) { finished = append(finished, result) }},
		func(context.Context) (Message, error) {
			calls++
			if calls == 1 {
				return retryTestAssistantError("terminated"), nil
			}
			return Message{Role: RoleAssistant, StopReason: StopReasonAborted}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopReasonAborted || calls != 2 {
		t.Fatalf("calls=%d result=%#v", calls, result)
	}
	if !reflect.DeepEqual(finished, []RetryResult{{Attempt: 1}}) {
		t.Fatalf("finished = %#v", finished)
	}
}

func TestRetryAssistantCallDisabledPolicyDoesNotRetry(t *testing.T) {
	t.Parallel()

	calls := 0
	result, err := RetryAssistantCall(
		context.Background(),
		RetryPolicy{Enabled: false, MaxRetries: 3},
		RetryCallbacks{},
		func(context.Context) (Message, error) {
			calls++
			return retryTestAssistantError("terminated"), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopReasonError || calls != 1 {
		t.Fatalf("calls=%d result=%#v", calls, result)
	}
}

func TestRetryAssistantCallEmitsAttemptStartAfterBackoff(t *testing.T) {
	t.Parallel()

	calls := 0
	var events []string
	result, err := RetryAssistantCall(
		context.Background(),
		RetryPolicy{Enabled: true, MaxRetries: 3},
		RetryCallbacks{
			OnRetryScheduled: func(attempt RetryAttempt) {
				events = append(events, "retry:"+strconv.Itoa(attempt.Attempt))
			},
			OnRetryAttemptStart: func(int) {
				events = append(events, "attempt-start")
			},
		},
		func(context.Context) (Message, error) {
			events = append(events, "produce:"+strconv.Itoa(calls))
			calls++
			if calls < 3 {
				return retryTestAssistantError("terminated"), nil
			}
			return AssistantMessage([]ContentPart{Text("recovered")}, StopReasonStop, Model{}), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopReasonStop {
		t.Fatalf("result = %#v", result)
	}
	want := []string{
		"produce:0",
		"retry:1",
		"attempt-start",
		"produce:1",
		"retry:2",
		"attempt-start",
		"produce:2",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestRetryAssistantCallAbortsBackoffAndReturnsAbortedMessage(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	scheduled := make(chan struct{})
	var finished []RetryResult
	resultChannel := make(chan Message, 1)
	errorChannel := make(chan error, 1)
	go func() {
		result, err := RetryAssistantCall(
			ctx,
			RetryPolicy{Enabled: true, MaxRetries: 5, BaseDelay: 10 * time.Second},
			RetryCallbacks{
				OnRetryScheduled: func(RetryAttempt) { close(scheduled) },
				OnRetryFinished:  func(result RetryResult) { finished = append(finished, result) },
			},
			func(context.Context) (Message, error) {
				return retryTestAssistantError("terminated"), nil
			},
		)
		resultChannel <- result
		errorChannel <- err
	}()

	<-scheduled
	cancel()
	result := <-resultChannel
	if err := <-errorChannel; err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopReasonAborted || result.ErrorMessage != "" {
		t.Fatalf("result = %#v", result)
	}
	if !reflect.DeepEqual(finished, []RetryResult{{Attempt: 1, FinalError: "terminated"}}) {
		t.Fatalf("finished = %#v", finished)
	}
}

func TestRetryProviderRequestRetriesRetryableErrors(t *testing.T) {
	t.Parallel()

	attempts := 0
	var delays []time.Duration
	value, err := RetryProviderRequest(context.Background(), ProviderRetryOptions{
		MaxRetries: 1,
		Sleep: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
	}, func(context.Context) (string, error) {
		attempts++
		if attempts == 1 {
			return "", &ProviderError{
				StatusCode: http.StatusTooManyRequests,
				Headers:    http.Header{"Retry-After-Ms": []string{"1000"}},
				Err:        errors.New("provider error: 429"),
			}
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if value != "ok" || attempts != 2 {
		t.Fatalf("value=%q attempts=%d", value, attempts)
	}
	if !reflect.DeepEqual(delays, []time.Duration{time.Second}) {
		t.Fatalf("delays = %v, want [1s]", delays)
	}
}

func TestRetryProviderRequestHonorsNonRetryableHeader(t *testing.T) {
	t.Parallel()

	attempts := 0
	providerErr := &ProviderError{
		StatusCode: http.StatusTooManyRequests,
		Headers:    http.Header{"X-Should-Retry": []string{"false"}},
		Err:        errors.New("do not retry"),
	}
	_, err := RetryProviderRequest(context.Background(), ProviderRetryOptions{MaxRetries: 2}, func(context.Context) (string, error) {
		attempts++
		return "", providerErr
	})
	if !errors.Is(err, providerErr) {
		t.Fatalf("error = %v, want original provider error", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestRetryProviderRequestRejectsServerDelayAboveLimit(t *testing.T) {
	t.Parallel()

	maxDelay := time.Second
	attempts := 0
	_, err := RetryProviderRequest(context.Background(), ProviderRetryOptions{
		MaxRetries:    1,
		MaxRetryDelay: &maxDelay,
	}, func(context.Context) (string, error) {
		attempts++
		return "", &ProviderError{
			StatusCode: http.StatusTooManyRequests,
			Headers:    http.Header{"Retry-After": []string{"277403"}},
			Err:        errors.New("provider error: 429"),
		}
	})
	if err == nil || !strings.Contains(err.Error(), "server requested 277403s retry delay (max: 1s)") {
		t.Fatalf("error = %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestRetryProviderRequestAllowsDisabledDelayLimit(t *testing.T) {
	t.Parallel()

	disabled := time.Duration(0)
	attempts := 0
	var delay time.Duration
	value, err := RetryProviderRequest(context.Background(), ProviderRetryOptions{
		MaxRetries:    1,
		MaxRetryDelay: &disabled,
		Sleep: func(_ context.Context, value time.Duration) error {
			delay = value
			return nil
		},
	}, func(context.Context) (string, error) {
		attempts++
		if attempts == 1 {
			return "", &ProviderError{
				StatusCode: http.StatusTooManyRequests,
				Headers:    http.Header{"Retry-After": []string{"2"}},
				Err:        errors.New("provider error: 429"),
			}
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if value != "ok" || attempts != 2 || delay != 2*time.Second {
		t.Fatalf("value=%q attempts=%d delay=%s", value, attempts, delay)
	}
}

func TestRetryProviderRequestCancelsBackoff(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	_, err := RetryProviderRequest(ctx, ProviderRetryOptions{
		MaxRetries: 2,
		Sleep: func(ctx context.Context, _ time.Duration) error {
			cancel()
			return ctx.Err()
		},
	}, func(context.Context) (string, error) {
		attempts++
		return "", &ProviderError{
			StatusCode: http.StatusTooManyRequests,
			Headers:    http.Header{"Retry-After-Ms": []string{"1000"}},
			Err:        errors.New("provider error: 429"),
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func retryTestAssistantError(message string) Message {
	return Message{
		Role:         RoleAssistant,
		StopReason:   StopReasonError,
		ErrorMessage: message,
	}
}
