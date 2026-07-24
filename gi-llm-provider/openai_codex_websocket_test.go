package gillmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeOpenAICodexWebSocket struct {
	mu       sync.Mutex
	reads    [][]byte
	readErr  error
	writes   [][]byte
	closes   []string
	reusable bool
}

func newFakeOpenAICodexWebSocket(events ...string) *fakeOpenAICodexWebSocket {
	connection := &fakeOpenAICodexWebSocket{reusable: true}
	for _, event := range events {
		connection.reads = append(connection.reads, []byte(event))
	}
	return connection
}

func (c *fakeOpenAICodexWebSocket) Write(ctx context.Context, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.reusable {
		return errors.New("websocket is closed")
	}
	c.writes = append(c.writes, append([]byte(nil), payload...))
	return nil
}

func (c *fakeOpenAICodexWebSocket) Read(
	ctx context.Context,
	_ time.Duration,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.reads) > 0 {
		payload := append([]byte(nil), c.reads[0]...)
		c.reads = c.reads[1:]
		return payload, nil
	}
	if c.readErr != nil {
		c.reusable = false
		return nil, c.readErr
	}
	c.reusable = false
	return nil, io.EOF
}

func (c *fakeOpenAICodexWebSocket) Close(_ int, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reusable = false
	c.closes = append(c.closes, reason)
	return nil
}

func (c *fakeOpenAICodexWebSocket) Reusable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reusable
}

func (c *fakeOpenAICodexWebSocket) writtenFrames(t *testing.T) []map[string]json.RawMessage {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]map[string]json.RawMessage, 0, len(c.writes))
	for _, payload := range c.writes {
		var frame map[string]json.RawMessage
		if err := json.Unmarshal(payload, &frame); err != nil {
			t.Fatalf("decode websocket frame: %v", err)
		}
		result = append(result, frame)
	}
	return result
}

func (c *fakeOpenAICodexWebSocket) closeReasons() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.closes...)
}

type fakeOpenAICodexWebSocketDialer struct {
	mu          sync.Mutex
	connections []OpenAICodexWebSocket
	err         error
	calls       []fakeOpenAICodexWebSocketDialCall
}

type fakeOpenAICodexWebSocketDialCall struct {
	endpoint string
	headers  map[string]string
	timeout  time.Duration
}

func (d *fakeOpenAICodexWebSocketDialer) Dial(
	ctx context.Context,
	endpoint string,
	headers map[string]string,
	timeout time.Duration,
) (OpenAICodexWebSocket, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, fakeOpenAICodexWebSocketDialCall{
		endpoint: endpoint,
		headers:  cloneStringMap(headers),
		timeout:  timeout,
	})
	if d.err != nil {
		return nil, d.err
	}
	index := len(d.calls) - 1
	if index >= len(d.connections) {
		return nil, errors.New("no fake websocket connection configured")
	}
	return d.connections[index], nil
}

func (d *fakeOpenAICodexWebSocketDialer) snapshotCalls() []fakeOpenAICodexWebSocketDialCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]fakeOpenAICodexWebSocketDialCall, len(d.calls))
	copy(result, d.calls)
	return result
}

type openAICodexHTTPDoerFunc func(*http.Request) (*http.Response, error)

func (f openAICodexHTTPDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestOpenAICodexResponsesProviderCachedWebSocketLifecycle(t *testing.T) {
	t.Run("opens a fresh cached websocket before the backend connection age limit", func(t *testing.T) {
		sessionID := "aged-ws-session"
		resetOpenAICodexWebSocketTestState(t, sessionID)
		firstConnection := newFakeOpenAICodexWebSocket(codexCompletedEvent("resp_1"))
		secondConnection := newFakeOpenAICodexWebSocket(codexCompletedEvent("resp_2"))
		dialer := &fakeOpenAICodexWebSocketDialer{
			connections: []OpenAICodexWebSocket{firstConnection, secondConnection},
		}
		now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
		provider := newFakeOpenAICodexWebSocketProvider(dialer, &now)
		model := openAICodexWebSocketTestModel()

		first := completeOpenAICodexWebSocketRequest(
			t,
			provider,
			model,
			Context{Messages: []Message{UserMessageText("first")}},
			SimpleStreamOptions{
				APIKey:    mockOpenAICodexToken(t, "acc_test"),
				SessionID: sessionID,
				Transport: "websocket-cached",
			},
		)
		if first.ResponseID != "resp_1" {
			t.Fatalf("first response = %#v", first)
		}

		now = now.Add(56 * time.Minute)
		second := completeOpenAICodexWebSocketRequest(
			t,
			provider,
			model,
			Context{Messages: []Message{UserMessageText("second")}},
			SimpleStreamOptions{
				APIKey:    mockOpenAICodexToken(t, "acc_test"),
				SessionID: sessionID,
				Transport: "websocket-cached",
			},
		)
		if second.ResponseID != "resp_2" {
			t.Fatalf("second response = %#v", second)
		}

		calls := dialer.snapshotCalls()
		if len(calls) != 2 {
			t.Fatalf("websocket dials = %d, want 2", len(calls))
		}
		if calls[0].endpoint != "wss://example.test/backend-api/codex/responses" {
			t.Fatalf("websocket endpoint = %q", calls[0].endpoint)
		}
		if _, ok := lookupHeader(calls[0].headers, "OpenAI-Beta"); ok {
			t.Fatalf("handshake headers must remove OpenAI-Beta: %#v", calls[0].headers)
		}
		if calls[0].headers["session-id"] != sessionID ||
			calls[0].headers["x-client-request-id"] != sessionID {
			t.Fatalf("session headers = %#v", calls[0].headers)
		}
		if reasons := firstConnection.closeReasons(); len(reasons) != 1 || reasons[0] != "connection_age_limit" {
			t.Fatalf("first connection close reasons = %#v", reasons)
		}
		stats, ok := GetOpenAICodexWebSocketDebugStats(sessionID)
		if !ok ||
			stats.Requests != 2 ||
			stats.ConnectionsCreated != 2 ||
			stats.ConnectionsReused != 0 ||
			stats.FullContextRequests != 2 {
			t.Fatalf("websocket stats = %#v ok=%v", stats, ok)
		}
	})

	t.Run("reuses a healthy cached websocket before max age", func(t *testing.T) {
		sessionID := "reused-ws-session"
		resetOpenAICodexWebSocketTestState(t, sessionID)
		connection := newFakeOpenAICodexWebSocket(
			codexCompletedEvent("resp_1"),
			codexCompletedEvent("resp_2"),
		)
		dialer := &fakeOpenAICodexWebSocketDialer{
			connections: []OpenAICodexWebSocket{connection},
		}
		now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
		provider := newFakeOpenAICodexWebSocketProvider(dialer, &now)
		options := SimpleStreamOptions{
			APIKey:    mockOpenAICodexToken(t, "acc_test"),
			SessionID: sessionID,
			Transport: "websocket",
		}

		completeOpenAICodexWebSocketRequest(
			t,
			provider,
			openAICodexWebSocketTestModel(),
			Context{Messages: []Message{UserMessageText("first")}},
			options,
		)
		now = now.Add(time.Minute)
		completeOpenAICodexWebSocketRequest(
			t,
			provider,
			openAICodexWebSocketTestModel(),
			Context{Messages: []Message{UserMessageText("second")}},
			options,
		)

		if calls := dialer.snapshotCalls(); len(calls) != 1 {
			t.Fatalf("websocket dials = %d, want 1", len(calls))
		}
		stats, ok := GetOpenAICodexWebSocketDebugStats(sessionID)
		if !ok ||
			stats.ConnectionsCreated != 1 ||
			stats.ConnectionsReused != 1 ||
			stats.CachedContextRequests != 0 ||
			stats.FullContextRequests != 2 {
			t.Fatalf("websocket stats = %#v ok=%v", stats, ok)
		}
	})

	t.Run("uses a transient websocket while the cached connection is busy", func(t *testing.T) {
		sessionID := "busy-ws-session"
		resetOpenAICodexWebSocketTestState(t, sessionID)
		firstConnection := newFakeOpenAICodexWebSocket()
		secondConnection := newFakeOpenAICodexWebSocket()
		dialer := &fakeOpenAICodexWebSocketDialer{
			connections: []OpenAICodexWebSocket{firstConnection, secondConnection},
		}
		now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
		first, err := acquireOpenAICodexWebSocket(
			context.Background(),
			dialer,
			"wss://example.test/codex/responses",
			nil,
			sessionID,
			now,
			time.Second,
		)
		if err != nil {
			t.Fatal(err)
		}
		second, err := acquireOpenAICodexWebSocket(
			context.Background(),
			dialer,
			"wss://example.test/codex/responses",
			nil,
			sessionID,
			now,
			time.Second,
		)
		if err != nil {
			t.Fatal(err)
		}
		if first.transient || second.connection != secondConnection || !second.transient {
			t.Fatalf("leases = first:%#v second:%#v", first, second)
		}
		second.release(true)
		first.release(true)
		if reasons := secondConnection.closeReasons(); len(reasons) != 1 || reasons[0] != "done" {
			t.Fatalf("transient close reasons = %#v", reasons)
		}
	})

	t.Run("expires an idle cached websocket", func(t *testing.T) {
		sessionID := "idle-ws-session"
		resetOpenAICodexWebSocketTestState(t, sessionID)
		connection := newFakeOpenAICodexWebSocket()
		dialer := &fakeOpenAICodexWebSocketDialer{
			connections: []OpenAICodexWebSocket{connection},
		}
		lease, err := acquireOpenAICodexWebSocket(
			context.Background(),
			dialer,
			"wss://example.test/codex/responses",
			nil,
			sessionID,
			time.Now(),
			time.Second,
		)
		if err != nil {
			t.Fatal(err)
		}
		lease.release(true)
		if lease.session.idleTimer == nil {
			t.Fatal("release did not schedule idle expiry")
		}
		lease.session.idleTimer.Stop()
		expireOpenAICodexWebSocketSession(
			sessionID,
			lease.session,
			lease.session.idleGeneration,
		)

		openAICodexWebSocketState.Lock()
		_, cached := openAICodexWebSocketState.sessions[sessionID]
		openAICodexWebSocketState.Unlock()
		if cached {
			t.Fatal("expired websocket remains cached")
		}
		if reasons := connection.closeReasons(); len(reasons) != 1 || reasons[0] != "idle_timeout" {
			t.Fatalf("idle connection close reasons = %#v", reasons)
		}
	})

	t.Run("ignores a stale idle expiry callback", func(t *testing.T) {
		sessionID := "stale-idle-ws-session"
		resetOpenAICodexWebSocketTestState(t, sessionID)
		connection := newFakeOpenAICodexWebSocket()
		dialer := &fakeOpenAICodexWebSocketDialer{
			connections: []OpenAICodexWebSocket{connection},
		}
		first, err := acquireOpenAICodexWebSocket(
			context.Background(),
			dialer,
			"wss://example.test/codex/responses",
			nil,
			sessionID,
			time.Now(),
			time.Second,
		)
		if err != nil {
			t.Fatal(err)
		}
		first.release(true)
		staleGeneration := first.session.idleGeneration

		second, err := acquireOpenAICodexWebSocket(
			context.Background(),
			dialer,
			"wss://example.test/codex/responses",
			nil,
			sessionID,
			time.Now(),
			time.Second,
		)
		if err != nil {
			t.Fatal(err)
		}
		if !second.reused {
			t.Fatal("second lease did not reuse the cached websocket")
		}
		second.release(true)
		expireOpenAICodexWebSocketSession(sessionID, second.session, staleGeneration)

		openAICodexWebSocketState.Lock()
		cached := openAICodexWebSocketState.sessions[sessionID]
		openAICodexWebSocketState.Unlock()
		if cached != second.session {
			t.Fatal("stale expiry callback removed the current websocket generation")
		}
		if reasons := connection.closeReasons(); len(reasons) != 0 {
			t.Fatalf("stale expiry callback closed the websocket: %#v", reasons)
		}
	})

	t.Run("disables websocket caching when cache retention is none", func(t *testing.T) {
		sessionID := "uncached-ws-session"
		resetOpenAICodexWebSocketTestState(t, sessionID)
		firstConnection := newFakeOpenAICodexWebSocket(codexCompletedEvent("resp_1"))
		secondConnection := newFakeOpenAICodexWebSocket(codexCompletedEvent("resp_2"))
		dialer := &fakeOpenAICodexWebSocketDialer{
			connections: []OpenAICodexWebSocket{firstConnection, secondConnection},
		}
		now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
		provider := newFakeOpenAICodexWebSocketProvider(dialer, &now)
		options := SimpleStreamOptions{
			APIKey:         mockOpenAICodexToken(t, "acc_test"),
			SessionID:      sessionID,
			Transport:      "websocket-cached",
			CacheRetention: "none",
		}

		completeOpenAICodexWebSocketRequest(
			t,
			provider,
			openAICodexWebSocketTestModel(),
			Context{Messages: []Message{UserMessageText("first")}},
			options,
		)
		completeOpenAICodexWebSocketRequest(
			t,
			provider,
			openAICodexWebSocketTestModel(),
			Context{Messages: []Message{UserMessageText("second")}},
			options,
		)

		if calls := dialer.snapshotCalls(); len(calls) != 2 {
			t.Fatalf("websocket dials = %d, want 2", len(calls))
		}
		for _, connection := range []*fakeOpenAICodexWebSocket{firstConnection, secondConnection} {
			frames := connection.writtenFrames(t)
			if len(frames) != 1 {
				t.Fatalf("frames = %d, want 1", len(frames))
			}
			if _, ok := frames[0]["prompt_cache_key"]; ok {
				t.Fatalf("uncached websocket frame has prompt_cache_key: %#v", frames[0])
			}
			if reasons := connection.closeReasons(); len(reasons) != 1 || reasons[0] != "done" {
				t.Fatalf("uncached connection close reasons = %#v", reasons)
			}
		}
		if _, ok := GetOpenAICodexWebSocketDebugStats(sessionID); ok {
			t.Fatal("cache-disabled request must not create session debug state")
		}
	})
}

func TestOpenAICodexResponsesProviderCachedWebSocketContinuation(t *testing.T) {
	sessionID := "continuation-ws-session"
	resetOpenAICodexWebSocketTestState(t, sessionID)
	connection := newFakeOpenAICodexWebSocket(
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","status":"in_progress"}}`,
		`{"type":"response.content_part.added","output_index":0,"part":{"type":"output_text","text":""}}`,
		`{"type":"response.output_text.delta","output_index":0,"delta":"first answer"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","status":"completed","content":[{"type":"output_text","text":"first answer"}]}}`,
		codexCompletedEvent("resp_1"),
		codexCompletedEvent("resp_2"),
	)
	dialer := &fakeOpenAICodexWebSocketDialer{
		connections: []OpenAICodexWebSocket{connection},
	}
	now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	provider := newFakeOpenAICodexWebSocketProvider(dialer, &now)
	model := openAICodexWebSocketTestModel()
	options := SimpleStreamOptions{
		APIKey:    mockOpenAICodexToken(t, "acc_test"),
		SessionID: sessionID,
		Transport: "websocket-cached",
	}
	firstContext := Context{Messages: []Message{UserMessageText("first question")}}
	first := completeOpenAICodexWebSocketRequest(t, provider, model, firstContext, options)
	if first.ResponseID != "resp_1" || len(first.Content) != 1 || first.Content[0].Text != "first answer" {
		t.Fatalf("first response = %#v", first)
	}

	now = now.Add(time.Minute)
	secondContext := Context{Messages: append(
		append([]Message(nil), firstContext.Messages...),
		first,
		UserMessageText("second question"),
	)}
	second := completeOpenAICodexWebSocketRequest(t, provider, model, secondContext, options)
	if second.ResponseID != "resp_2" {
		t.Fatalf("second response = %#v", second)
	}

	frames := connection.writtenFrames(t)
	if len(frames) != 2 {
		t.Fatalf("websocket frames = %d, want 2", len(frames))
	}
	var firstInput, secondInput []json.RawMessage
	if err := json.Unmarshal(frames[0]["input"], &firstInput); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(frames[1]["input"], &secondInput); err != nil {
		t.Fatal(err)
	}
	var previousResponseID string
	if err := json.Unmarshal(frames[1]["previous_response_id"], &previousResponseID); err != nil {
		t.Fatal(err)
	}
	if len(firstInput) != 1 || len(secondInput) != 1 || previousResponseID != "resp_1" {
		t.Fatalf(
			"first input=%d second input=%d previous=%q",
			len(firstInput),
			len(secondInput),
			previousResponseID,
		)
	}
	stats, ok := GetOpenAICodexWebSocketDebugStats(sessionID)
	if !ok ||
		stats.CachedContextRequests != 2 ||
		stats.FullContextRequests != 1 ||
		stats.DeltaRequests != 1 ||
		stats.LastDeltaInputItems == nil ||
		*stats.LastDeltaInputItems != 1 ||
		stats.LastPreviousResponseID != "resp_1" {
		t.Fatalf("websocket stats = %#v ok=%v", stats, ok)
	}
}

func TestOpenAICodexResponsesProviderRetriesWebSocketConnectionLimitBeforeStart(t *testing.T) {
	sessionID := "connection-limit-ws-session"
	resetOpenAICodexWebSocketTestState(t, sessionID)
	firstConnection := newFakeOpenAICodexWebSocket(
		`{"type":"error","error":{"code":"websocket_connection_limit_reached","message":"too many connections"}}`,
	)
	secondConnection := newFakeOpenAICodexWebSocket(codexCompletedEvent("resp_retry"))
	dialer := &fakeOpenAICodexWebSocketDialer{
		connections: []OpenAICodexWebSocket{firstConnection, secondConnection},
	}
	now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	result := completeOpenAICodexWebSocketRequest(
		t,
		newFakeOpenAICodexWebSocketProvider(dialer, &now),
		openAICodexWebSocketTestModel(),
		Context{Messages: []Message{UserMessageText("hello")}},
		SimpleStreamOptions{
			APIKey:    mockOpenAICodexToken(t, "acc_test"),
			SessionID: sessionID,
			Transport: "websocket",
		},
	)
	if result.ResponseID != "resp_retry" || result.StopReason != StopReasonStop {
		t.Fatalf("response = %#v", result)
	}
	if calls := dialer.snapshotCalls(); len(calls) != 2 {
		t.Fatalf("websocket dials = %d, want 2", len(calls))
	}
	stats, ok := GetOpenAICodexWebSocketDebugStats(sessionID)
	if !ok ||
		stats.Requests != 2 ||
		stats.ConnectionsCreated != 2 ||
		stats.WebSocketFailures != 0 ||
		stats.SSEFallbacks != 0 {
		t.Fatalf("stats = %#v ok=%v", stats, ok)
	}
}

func TestOpenAICodexResponsesProviderFallsBackAfterRepeatedWebSocketConnectionLimit(t *testing.T) {
	sessionID := "repeated-connection-limit-ws-session"
	resetOpenAICodexWebSocketTestState(t, sessionID)
	connectionLimitEvent := `{"type":"error","error":{"code":"websocket_connection_limit_reached","message":"too many connections"}}`
	dialer := &fakeOpenAICodexWebSocketDialer{
		connections: []OpenAICodexWebSocket{
			newFakeOpenAICodexWebSocket(connectionLimitEvent),
			newFakeOpenAICodexWebSocket(connectionLimitEvent),
		},
	}
	httpRequests := 0
	client := openAICodexHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		httpRequests++
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"content-type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				"data: " + codexCompletedEvent("resp_fallback") + "\n\n",
			)),
			Request: request,
		}, nil
	})
	now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	provider := newFakeOpenAICodexWebSocketProvider(dialer, &now)
	provider.Client = client
	result := completeOpenAICodexWebSocketRequest(
		t,
		provider,
		openAICodexWebSocketTestModel(),
		Context{Messages: []Message{UserMessageText("hello")}},
		SimpleStreamOptions{
			APIKey:    mockOpenAICodexToken(t, "acc_test"),
			SessionID: sessionID,
			Transport: "websocket",
		},
	)
	if result.ResponseID != "resp_fallback" || httpRequests != 1 || len(result.Diagnostics) != 1 {
		t.Fatalf("result=%#v httpRequests=%d", result, httpRequests)
	}
	stats, ok := GetOpenAICodexWebSocketDebugStats(sessionID)
	if !ok ||
		stats.Requests != 2 ||
		stats.WebSocketFailures != 1 ||
		stats.SSEFallbacks != 1 {
		t.Fatalf("stats = %#v ok=%v", stats, ok)
	}
}

func TestOpenAICodexResponsesProviderRetriesMissingWebSocketContinuationWithFullContext(t *testing.T) {
	sessionID := "missing-continuation-ws-session"
	resetOpenAICodexWebSocketTestState(t, sessionID)
	firstConnection := newFakeOpenAICodexWebSocket(
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","status":"in_progress"}}`,
		`{"type":"response.content_part.added","output_index":0,"part":{"type":"output_text","text":""}}`,
		`{"type":"response.output_text.delta","output_index":0,"delta":"first answer"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","status":"completed","content":[{"type":"output_text","text":"first answer"}]}}`,
		codexCompletedEvent("resp_1"),
		`{"type":"error","error":{"code":"previous_response_not_found","message":"continuation expired"}}`,
	)
	secondConnection := newFakeOpenAICodexWebSocket(codexCompletedEvent("resp_2"))
	dialer := &fakeOpenAICodexWebSocketDialer{
		connections: []OpenAICodexWebSocket{firstConnection, secondConnection},
	}
	now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	provider := newFakeOpenAICodexWebSocketProvider(dialer, &now)
	model := openAICodexWebSocketTestModel()
	options := SimpleStreamOptions{
		APIKey:    mockOpenAICodexToken(t, "acc_test"),
		SessionID: sessionID,
		Transport: "websocket-cached",
	}
	firstContext := Context{Messages: []Message{UserMessageText("first question")}}
	first := completeOpenAICodexWebSocketRequest(t, provider, model, firstContext, options)
	now = now.Add(time.Minute)
	secondContext := Context{Messages: append(
		append([]Message(nil), firstContext.Messages...),
		first,
		UserMessageText("second question"),
	)}
	second := completeOpenAICodexWebSocketRequest(t, provider, model, secondContext, options)
	if second.ResponseID != "resp_2" || second.StopReason != StopReasonStop {
		t.Fatalf("second response = %#v", second)
	}

	firstFrames := firstConnection.writtenFrames(t)
	secondFrames := secondConnection.writtenFrames(t)
	if len(firstFrames) != 2 || len(secondFrames) != 1 {
		t.Fatalf("first frames=%d second frames=%d", len(firstFrames), len(secondFrames))
	}
	if _, ok := firstFrames[1]["previous_response_id"]; !ok {
		t.Fatalf("delta frame = %#v", firstFrames[1])
	}
	if _, ok := secondFrames[0]["previous_response_id"]; ok {
		t.Fatalf("full retry frame = %#v", secondFrames[0])
	}
	var retryInput []json.RawMessage
	if err := json.Unmarshal(secondFrames[0]["input"], &retryInput); err != nil {
		t.Fatal(err)
	}
	if len(retryInput) != 3 {
		t.Fatalf("full retry input items = %d, want 3", len(retryInput))
	}
	stats, ok := GetOpenAICodexWebSocketDebugStats(sessionID)
	if !ok ||
		stats.Requests != 3 ||
		stats.ConnectionsCreated != 2 ||
		stats.ConnectionsReused != 1 ||
		stats.FullContextRequests != 2 ||
		stats.DeltaRequests != 1 ||
		stats.WebSocketFailures != 0 {
		t.Fatalf("stats = %#v ok=%v", stats, ok)
	}
}

func TestOpenAICodexResponsesProviderFallsBackOnlyBeforeWebSocketStreamStart(t *testing.T) {
	sessionID := "fallback-ws-session"
	resetOpenAICodexWebSocketTestState(t, sessionID)
	dialer := &fakeOpenAICodexWebSocketDialer{err: errors.New("dial unavailable")}
	httpRequests := 0
	client := openAICodexHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		httpRequests++
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"content-type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				"data: " + codexCompletedEvent("resp_fallback") + "\n\n",
			)),
			Request: request,
		}, nil
	})
	provider := NewOpenAICodexResponsesProvider(client)
	provider.WebSocketDialer = dialer
	stream, err := provider.StreamSimple(
		openAICodexWebSocketTestModel(),
		Context{Messages: []Message{UserMessageText("hello")}},
		SimpleStreamOptions{
			APIKey:    mockOpenAICodexToken(t, "acc_test"),
			SessionID: sessionID,
			Transport: "websocket",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.ResponseID != "resp_fallback" || result.StopReason != StopReasonStop {
		t.Fatalf("fallback response = %#v", result)
	}
	if httpRequests != 1 || len(dialer.snapshotCalls()) != 1 {
		t.Fatalf("http requests=%d websocket dials=%d", httpRequests, len(dialer.snapshotCalls()))
	}
	if len(result.Diagnostics) != 1 ||
		result.Diagnostics[0].Type != "provider_transport_failure" ||
		result.Diagnostics[0].Details["configuredTransport"] != "websocket" ||
		result.Diagnostics[0].Details["fallbackTransport"] != "sse" ||
		result.Diagnostics[0].Details["eventsEmitted"] != false {
		t.Fatalf("fallback diagnostics = %#v", result.Diagnostics)
	}
	stats, ok := GetOpenAICodexWebSocketDebugStats(sessionID)
	if !ok ||
		stats.WebSocketFailures != 1 ||
		stats.SSEFallbacks != 1 ||
		!stats.WebSocketFallbackActive {
		t.Fatalf("fallback stats = %#v ok=%v", stats, ok)
	}

	secondStream, err := provider.StreamSimple(
		openAICodexWebSocketTestModel(),
		Context{Messages: []Message{UserMessageText("again")}},
		SimpleStreamOptions{
			APIKey:    mockOpenAICodexToken(t, "acc_test"),
			SessionID: sessionID,
			Transport: "auto",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondStream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.ResponseID != "resp_fallback" || len(second.Diagnostics) != 0 {
		t.Fatalf("second fallback response = %#v", second)
	}
	if httpRequests != 2 || len(dialer.snapshotCalls()) != 1 {
		t.Fatalf("second http requests=%d websocket dials=%d", httpRequests, len(dialer.snapshotCalls()))
	}
	stats, _ = GetOpenAICodexWebSocketDebugStats(sessionID)
	if stats.WebSocketFailures != 1 || stats.SSEFallbacks != 2 {
		t.Fatalf("second fallback stats = %#v", stats)
	}
}

func TestOpenAICodexResponsesProviderDoesNotFallBackAfterWebSocketStreamStart(t *testing.T) {
	sessionID := "started-ws-session"
	resetOpenAICodexWebSocketTestState(t, sessionID)
	connection := newFakeOpenAICodexWebSocket(
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","status":"in_progress"}}`,
	)
	connection.readErr = errors.New("connection lost")
	dialer := &fakeOpenAICodexWebSocketDialer{
		connections: []OpenAICodexWebSocket{connection},
	}
	httpRequests := 0
	provider := NewOpenAICodexResponsesProvider(openAICodexHTTPDoerFunc(
		func(*http.Request) (*http.Response, error) {
			httpRequests++
			return nil, errors.New("unexpected SSE fallback")
		},
	))
	provider.WebSocketDialer = dialer

	stream, err := provider.StreamSimple(
		openAICodexWebSocketTestModel(),
		Context{Messages: []Message{UserMessageText("hello")}},
		SimpleStreamOptions{
			APIKey:    mockOpenAICodexToken(t, "acc_test"),
			SessionID: sessionID,
			Transport: "websocket",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	events := collectAssistantStreamEvents(stream)
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopReasonError ||
		result.ErrorMessage != "connection lost" ||
		httpRequests != 0 ||
		!containsAssistantEvent(events, "start") ||
		!containsAssistantEvent(events, "error") {
		t.Fatalf("result=%#v events=%#v httpRequests=%d", result, events, httpRequests)
	}
	if len(result.Diagnostics) != 1 ||
		result.Diagnostics[0].Details["eventsEmitted"] != true ||
		result.Diagnostics[0].Details["fallbackTransport"] != nil {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	stats, ok := GetOpenAICodexWebSocketDebugStats(sessionID)
	if !ok ||
		stats.WebSocketFailures != 1 ||
		stats.SSEFallbacks != 0 ||
		!stats.WebSocketFallbackActive {
		t.Fatalf("stats = %#v ok=%v", stats, ok)
	}
}

func TestOpenAICodexResponsesProviderDoesNotFallBackForWebSocketAPIOrProtocolErrors(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		want      string
		wantExact bool
	}{
		{
			name:      "api error",
			payload:   `{"type":"error","error":{"code":"invalid_request","message":"bad request"}}`,
			want:      "Codex error: bad request",
			wantExact: true,
		},
		{
			name:      "response failed",
			payload:   `{"type":"response.failed","response":{"error":{"code":"invalid_request","message":"backend failed"}}}`,
			want:      "backend failed",
			wantExact: true,
		},
		{
			name:    "protocol error",
			payload: `{`,
			want:    "invalid Codex WebSocket JSON",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sessionID := "non-transport-" + strings.ReplaceAll(test.name, " ", "-")
			resetOpenAICodexWebSocketTestState(t, sessionID)
			dialer := &fakeOpenAICodexWebSocketDialer{
				connections: []OpenAICodexWebSocket{
					newFakeOpenAICodexWebSocket(test.payload),
				},
			}
			httpRequests := 0
			provider := NewOpenAICodexResponsesProvider(openAICodexHTTPDoerFunc(
				func(*http.Request) (*http.Response, error) {
					httpRequests++
					return nil, errors.New("unexpected SSE fallback")
				},
			))
			provider.WebSocketDialer = dialer
			result := completeOpenAICodexWebSocketRequest(
				t,
				provider,
				openAICodexWebSocketTestModel(),
				Context{Messages: []Message{UserMessageText("hello")}},
				SimpleStreamOptions{
					APIKey:    mockOpenAICodexToken(t, "acc_test"),
					SessionID: sessionID,
					Transport: "websocket",
				},
			)
			messageMatches := strings.Contains(result.ErrorMessage, test.want)
			if test.wantExact {
				messageMatches = result.ErrorMessage == test.want
			}
			if result.StopReason != StopReasonError ||
				!messageMatches ||
				len(result.Diagnostics) != 0 ||
				httpRequests != 0 {
				t.Fatalf("result=%#v httpRequests=%d", result, httpRequests)
			}
			stats, ok := GetOpenAICodexWebSocketDebugStats(sessionID)
			if !ok ||
				stats.Requests != 1 ||
				stats.WebSocketFailures != 0 ||
				stats.SSEFallbacks != 0 ||
				stats.WebSocketFallbackActive {
				t.Fatalf("stats = %#v ok=%v", stats, ok)
			}
		})
	}
}

func TestCloseOpenAICodexWebSocketSessionsClosesCachedConnection(t *testing.T) {
	sessionID := "close-ws-session"
	resetOpenAICodexWebSocketTestState(t, sessionID)
	connection := newFakeOpenAICodexWebSocket()
	dialer := &fakeOpenAICodexWebSocketDialer{
		connections: []OpenAICodexWebSocket{connection},
	}
	lease, err := acquireOpenAICodexWebSocket(
		context.Background(),
		dialer,
		"wss://example.test/codex/responses",
		nil,
		sessionID,
		time.Now(),
		time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	lease.release(true)

	CloseOpenAICodexWebSocketSessions(sessionID)
	if reasons := connection.closeReasons(); len(reasons) != 1 || reasons[0] != "debug_close" {
		t.Fatalf("cached connection close reasons = %#v", reasons)
	}
}

func TestClampOpenAIPromptCacheKeyUsesUnicodeCharacters(t *testing.T) {
	if got := ClampOpenAIPromptCacheKey(strings.Repeat("a", 64)); len([]rune(got)) != 64 {
		t.Fatalf("64-character key length = %d", len([]rune(got)))
	}
	key := strings.Repeat("界", 65)
	got := ClampOpenAIPromptCacheKey(key)
	if len([]rune(got)) != OpenAIPromptCacheKeyMaxLength ||
		got != strings.Repeat("界", OpenAIPromptCacheKeyMaxLength) {
		t.Fatalf("clamped key = %q", got)
	}
}

func TestArmOpenAICodexContextDeadlineClearsCancellationDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var (
		mu        sync.Mutex
		deadlines []time.Time
	)
	initial := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	clearDeadline, err := armOpenAICodexContextDeadline(
		ctx,
		func(deadline time.Time) error {
			mu.Lock()
			deadlines = append(deadlines, deadline)
			mu.Unlock()
			return nil
		},
		initial,
	)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	clearDeadline()

	mu.Lock()
	defer mu.Unlock()
	if len(deadlines) < 3 ||
		!deadlines[0].Equal(initial) ||
		deadlines[1].IsZero() ||
		!deadlines[len(deadlines)-1].IsZero() {
		t.Fatalf("deadline transitions = %#v", deadlines)
	}
}

func TestOpenAICodexResponsesProviderRejectsInvalidTimeoutsBeforeTransport(t *testing.T) {
	tests := []struct {
		name    string
		options SimpleStreamOptions
		want    string
	}{
		{
			name: "response timeout",
			options: SimpleStreamOptions{
				TimeoutMillis: -1,
			},
			want: "Invalid timeoutMs: -1",
		},
		{
			name: "websocket connect timeout",
			options: SimpleStreamOptions{
				WebSocketConnectTimeoutMillis: -1,
			},
			want: "Invalid websocketConnectTimeoutMs: -1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			httpRequests := 0
			provider := NewOpenAICodexResponsesProvider(openAICodexHTTPDoerFunc(
				func(*http.Request) (*http.Response, error) {
					httpRequests++
					return nil, errors.New("unexpected HTTP request")
				},
			))
			dialer := &fakeOpenAICodexWebSocketDialer{
				err: errors.New("unexpected WebSocket dial"),
			}
			provider.WebSocketDialer = dialer
			test.options.APIKey = mockOpenAICodexToken(t, "acc_test")

			result := completeOpenAICodexWebSocketRequest(
				t,
				provider,
				openAICodexWebSocketTestModel(),
				Context{Messages: []Message{UserMessageText("hello")}},
				test.options,
			)
			if result.StopReason != StopReasonError ||
				!strings.Contains(result.ErrorMessage, test.want) {
				t.Fatalf("result = %#v", result)
			}
			if httpRequests != 0 {
				t.Fatalf("HTTP requests = %d, want 0", httpRequests)
			}
			if calls := dialer.snapshotCalls(); len(calls) != 0 {
				t.Fatalf("WebSocket dials = %d, want 0", len(calls))
			}
		})
	}
}

func TestOpenAICodexPreparedRequestSeparatesSessionOwnershipFromWireKey(t *testing.T) {
	sessionID := strings.Repeat("界", 65)
	provider := NewOpenAICodexResponsesProvider(nil)
	request, err := provider.buildRequest(
		openAICodexWebSocketTestModel(),
		Context{Messages: []Message{UserMessageText("hello")}},
		SimpleStreamOptions{
			SessionID:      sessionID,
			HeaderRemovals: []string{"authorization", "openai-beta"},
		},
		mockOpenAICodexToken(t, "acc_test"),
	)
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := request.payload.(OpenAICodexResponsesPayload)
	if !ok {
		t.Fatalf("payload type = %T", request.payload)
	}
	if request.cacheSessionID != sessionID ||
		len([]rune(request.codexSessionID)) != OpenAIPromptCacheKeyMaxLength ||
		payload.PromptCacheKey != request.codexSessionID {
		t.Fatalf(
			"cache session=%q wire session=%q prompt key=%q",
			request.cacheSessionID,
			request.codexSessionID,
			payload.PromptCacheKey,
		)
	}
	for _, headers := range []map[string]string{request.sseHeaders, request.webSocketHeaders} {
		if _, ok := lookupHeader(headers, "authorization"); ok {
			t.Fatalf("authorization removal not applied: %#v", headers)
		}
		if _, ok := lookupHeader(headers, "openai-beta"); ok {
			t.Fatalf("OpenAI-Beta removal not applied: %#v", headers)
		}
	}
}

func TestOpenAICodexWebSocketRequestPlanPreservesExplicitPreviousResponse(t *testing.T) {
	body, err := newOpenAICodexWebSocketRequestBody(map[string]any{
		"model":                "gpt-5.3-codex",
		"store":                false,
		"stream":               true,
		"input":                []map[string]any{{"role": "user", "content": "hello"}},
		"previous_response_id": "resp_explicit",
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := buildOpenAICodexWebSocketRequestPlan(nil, body, false)
	frame, err := plan.frame()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(frame, &decoded); err != nil {
		t.Fatal(err)
	}
	var previousResponseID string
	if err := json.Unmarshal(decoded["previous_response_id"], &previousResponseID); err != nil {
		t.Fatal(err)
	}
	if previousResponseID != "resp_explicit" || !plan.delta {
		t.Fatalf("previous response=%q delta=%v frame=%s", previousResponseID, plan.delta, frame)
	}
}

func newFakeOpenAICodexWebSocketProvider(
	dialer OpenAICodexWebSocketDialer,
	now *time.Time,
) OpenAICodexResponsesProvider {
	provider := NewOpenAICodexResponsesProvider(nil)
	provider.WebSocketDialer = dialer
	provider.Now = func() time.Time {
		return *now
	}
	return provider
}

func openAICodexWebSocketTestModel() Model {
	return Model{
		ID:       "gpt-5.3-codex",
		Provider: "openai-codex",
		API:      "openai-codex-responses",
		BaseURL:  "https://example.test/backend-api",
		Input:    []string{"text"},
	}
}

func completeOpenAICodexWebSocketRequest(
	t *testing.T,
	provider OpenAICodexResponsesProvider,
	model Model,
	contextValue Context,
	options SimpleStreamOptions,
) Message {
	t.Helper()
	stream, err := provider.StreamSimple(model, contextValue, options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func resetOpenAICodexWebSocketTestState(t *testing.T, sessionID string) {
	t.Helper()
	CloseOpenAICodexWebSocketSessions(sessionID)
	ResetOpenAICodexWebSocketDebugStats(sessionID)
	t.Cleanup(func() {
		CloseOpenAICodexWebSocketSessions(sessionID)
		ResetOpenAICodexWebSocketDebugStats(sessionID)
	})
}

func codexCompletedEvent(responseID string) string {
	return `{"type":"response.completed","response":{"id":"` +
		responseID +
		`","status":"completed"}}`
}
