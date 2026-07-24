package gillmprovider

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type openAICodexRetryDoerFunc func(*http.Request) (*http.Response, error)

func (do openAICodexRetryDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return do(request)
}

func TestPrepareOpenAICodexExecutionOptions(t *testing.T) {
	defaults, err := prepareOpenAICodexExecutionOptions(SimpleStreamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.transport != "auto" ||
		defaults.sseResponseHeaderTimeout != 0 ||
		defaults.webSocketConnectTimeout != defaultOpenAICodexWebSocketConnectTimeout ||
		defaults.webSocketIdleTimeout != 0 ||
		defaults.maxRetries != 0 ||
		defaults.maxRetryDelay != DefaultMaxProviderRetryDelay {
		t.Fatalf("default execution options = %#v", defaults)
	}

	configured, err := prepareOpenAICodexExecutionOptions(SimpleStreamOptions{
		Transport:                     " WebSocket-Cached ",
		TimeoutMillis:                 25,
		WebSocketConnectTimeoutMillis: 50,
		MaxRetries:                    2,
		MaxRetryDelayMs:               1500,
	})
	if err != nil {
		t.Fatal(err)
	}
	if configured.transport != "websocket-cached" ||
		configured.sseResponseHeaderTimeout != 25*time.Millisecond ||
		configured.webSocketConnectTimeout != 50*time.Millisecond ||
		configured.webSocketIdleTimeout != 25*time.Millisecond ||
		configured.maxRetries != 2 ||
		configured.maxRetryDelay != 1500*time.Millisecond {
		t.Fatalf("configured execution options = %#v", configured)
	}

	unlimited, err := prepareOpenAICodexExecutionOptions(SimpleStreamOptions{
		MaxRetryDelayMs: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if unlimited.maxRetryDelay != 0 {
		t.Fatalf("unlimited retry delay = %s, want 0", unlimited.maxRetryDelay)
	}
}

func TestOpenAICodexPostWithRetryUsesServerDelay(t *testing.T) {
	requests := 0
	var requestBodies [][]byte
	client := openAICodexRetryDoerFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		requestBodies = append(requestBodies, body)
		if request.Header.Get("content-encoding") != "zstd" {
			return nil, errors.New("Codex SSE request is not zstd encoded")
		}
		if requests == 1 {
			return openAICodexRetryResponse(
				request,
				http.StatusTooManyRequests,
				http.Header{"retry-after-ms": []string{"1500.5"}},
				`{"error":{"message":"rate limited"}}`,
			), nil
		}
		return openAICodexRetryResponse(
			request,
			http.StatusOK,
			http.Header{"content-type": []string{"text/event-stream"}},
			"",
		), nil
	})
	var delays []time.Duration
	provider := NewOpenAICodexResponsesProvider(client)
	provider.Sleep = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}
	options := SimpleStreamOptions{MaxRetries: 1}
	execution, err := prepareOpenAICodexExecutionOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.postWithRetry(
		context.Background(),
		openAICodexRetryModel(),
		options,
		execution,
		nil,
		map[string]any{"input": []any{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if requests != 2 ||
		len(delays) != 1 ||
		delays[0] != 1500*time.Millisecond+500*time.Microsecond {
		t.Fatalf("requests=%d delays=%v", requests, delays)
	}
	if len(requestBodies) != 2 {
		t.Fatalf("captured request bodies = %d, want 2", len(requestBodies))
	}
	if !bytes.Equal(requestBodies[0], requestBodies[1]) {
		t.Fatalf("retry request body lengths = %d and %d", len(requestBodies[0]), len(requestBodies[1]))
	}
}

func TestOpenAICodexPostWithRetryRejectsExcessiveServerDelay(t *testing.T) {
	requests := 0
	client := openAICodexRetryDoerFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return openAICodexRetryResponse(
			request,
			http.StatusServiceUnavailable,
			http.Header{"retry-after": []string{"2"}},
			`{"error":{"message":"retry later"}}`,
		), nil
	})
	provider := NewOpenAICodexResponsesProvider(client)
	options := SimpleStreamOptions{
		MaxRetries:      1,
		MaxRetryDelayMs: 1000,
	}
	execution, err := prepareOpenAICodexExecutionOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.postWithRetry(
		context.Background(),
		openAICodexRetryModel(),
		options,
		execution,
		nil,
		map[string]any{"input": []any{}},
	)
	var exceeded *OpenAICodexRetryDelayExceededError
	if !errors.As(err, &exceeded) ||
		err.Error() != "Server requested 2s retry delay (max: 1s)" {
		t.Fatalf("error = %T %v", err, err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestOpenAICodexPostWithRetrySkipsTerminalRateLimit(t *testing.T) {
	requests := 0
	client := openAICodexRetryDoerFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return openAICodexRetryResponse(
			request,
			http.StatusTooManyRequests,
			nil,
			`{"error":{"code":"insufficient_quota","message":"billing required"}}`,
		), nil
	})
	provider := NewOpenAICodexResponsesProvider(client)
	options := SimpleStreamOptions{MaxRetries: 1}
	execution, err := prepareOpenAICodexExecutionOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.postWithRetry(
		context.Background(),
		openAICodexRetryModel(),
		options,
		execution,
		nil,
		map[string]any{"input": []any{}},
	)
	if err == nil || requests != 1 {
		t.Fatalf("error=%v requests=%d", err, requests)
	}
}

func TestOpenAICodexPostWithRetryDefaultsToNoRetries(t *testing.T) {
	requests := 0
	client := openAICodexRetryDoerFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("network unavailable")
	})
	provider := NewOpenAICodexResponsesProvider(client)
	options := SimpleStreamOptions{}
	execution, err := prepareOpenAICodexExecutionOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.postWithRetry(
		context.Background(),
		openAICodexRetryModel(),
		options,
		execution,
		nil,
		map[string]any{"input": []any{}},
	)
	if err == nil || err.Error() != "network unavailable" || requests != 1 {
		t.Fatalf("error=%v requests=%d", err, requests)
	}
}

func TestOpenAICodexSSEHeaderTimeout(t *testing.T) {
	client := openAICodexRetryDoerFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	provider := NewOpenAICodexResponsesProvider(client)
	options := SimpleStreamOptions{TimeoutMillis: 10}
	execution, err := prepareOpenAICodexExecutionOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.postWithRetry(
		context.Background(),
		openAICodexRetryModel(),
		options,
		execution,
		nil,
		map[string]any{"input": []any{}},
	)
	if err == nil || err.Error() != "Codex SSE response headers timed out after 10ms" {
		t.Fatalf("error = %v", err)
	}
}

func TestOpenAICodexSSEHeaderTimeoutStopsAfterHeaders(t *testing.T) {
	var requestContext context.Context
	client := openAICodexRetryDoerFunc(func(request *http.Request) (*http.Response, error) {
		requestContext = request.Context()
		return openAICodexRetryResponse(
			request,
			http.StatusOK,
			http.Header{"content-type": []string{"text/event-stream"}},
			"",
		), nil
	})
	sseRequest, err := prepareOpenAICodexSSERequest(
		map[string]any{"input": []any{}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := postOpenAICodexSSEWithHeaderTimeout(
		context.Background(),
		client,
		"https://example.test/codex/responses",
		sseRequest,
		10*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-requestContext.Done():
		t.Fatal("request context ended after headers but before body close")
	default:
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-requestContext.Done():
	case <-time.After(time.Second):
		t.Fatal("request context remained active after body close")
	}
}

func openAICodexRetryResponse(
	request *http.Request,
	status int,
	headers http.Header,
	body string,
) *http.Response {
	if headers == nil {
		headers = http.Header{}
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func openAICodexRetryModel() Model {
	return Model{
		ID:       "gpt-5.3-codex",
		Provider: "openai-codex",
		API:      "openai-codex-responses",
		BaseURL:  "https://example.test",
		Input:    []string{"text"},
	}
}
