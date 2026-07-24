package gillmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultOpenAICodexWebSocketConnectTimeout = 15 * time.Second
	openAICodexWebSocketSessionCacheTTL       = 5 * time.Minute
	openAICodexWebSocketSessionMaxAge         = 55 * time.Minute
	openAICodexWebSocketBeta                  = "responses_websockets=2026-02-06"
)

// OpenAICodexWebSocket is the request-scoped view of a Codex WebSocket
// connection. Implementations must support one active reader and one active
// writer; the session pool guarantees that a cached connection has only one
// borrower at a time.
type OpenAICodexWebSocket interface {
	Write(context.Context, []byte) error
	Read(context.Context, time.Duration) ([]byte, error)
	Close(code int, reason string) error
	Reusable() bool
}

// OpenAICodexWebSocketDialer separates connection establishment from provider
// orchestration so lifecycle and fallback behavior can be tested without a
// network listener.
type OpenAICodexWebSocketDialer interface {
	Dial(
		context.Context,
		string,
		map[string]string,
		time.Duration,
	) (OpenAICodexWebSocket, error)
}

type gorillaOpenAICodexWebSocketDialer struct{}

func (gorillaOpenAICodexWebSocketDialer) Dial(
	ctx context.Context,
	endpoint string,
	headers map[string]string,
	connectTimeout time.Duration,
) (OpenAICodexWebSocket, error) {
	if connectTimeout <= 0 {
		connectTimeout = defaultOpenAICodexWebSocketConnectTimeout
	}
	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: connectTimeout,
	}
	requestHeaders := make(http.Header, len(headers))
	for key, value := range headers {
		if value != "" {
			requestHeaders.Set(key, value)
		}
	}
	connection, response, err := dialer.DialContext(connectCtx, endpoint, requestHeaders)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		if response == nil {
			return nil, fmt.Errorf("open Codex WebSocket: %w", err)
		}
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			detail = response.Status
		}
		return nil, fmt.Errorf("open Codex WebSocket: HTTP %d: %s: %w", response.StatusCode, detail, err)
	}
	return newGorillaOpenAICodexWebSocket(connection), nil
}

type gorillaOpenAICodexWebSocket struct {
	connection *websocket.Conn
	writeMu    sync.Mutex
	reusable   atomic.Bool
}

func newGorillaOpenAICodexWebSocket(connection *websocket.Conn) *gorillaOpenAICodexWebSocket {
	result := &gorillaOpenAICodexWebSocket{connection: connection}
	result.reusable.Store(true)
	return result
}

func (c *gorillaOpenAICodexWebSocket) Write(ctx context.Context, payload []byte) error {
	if ctx == nil {
		ctx = context.Background()
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	clearDeadline, err := armOpenAICodexContextDeadline(
		ctx,
		c.connection.UnderlyingConn().SetWriteDeadline,
		contextDeadline(ctx),
	)
	if err != nil {
		c.reusable.Store(false)
		return err
	}
	err = c.connection.WriteMessage(websocket.TextMessage, payload)
	clearDeadline()
	if err != nil {
		c.reusable.Store(false)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("write Codex WebSocket frame: %w", err)
	}
	return nil
}

func (c *gorillaOpenAICodexWebSocket) Read(
	ctx context.Context,
	idleTimeout time.Duration,
) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	deadline := contextDeadline(ctx)
	if idleTimeout > 0 {
		idleDeadline := time.Now().Add(idleTimeout)
		if deadline.IsZero() || idleDeadline.Before(deadline) {
			deadline = idleDeadline
		}
	}
	clearDeadline, err := armOpenAICodexContextDeadline(
		ctx,
		c.connection.UnderlyingConn().SetReadDeadline,
		deadline,
	)
	if err != nil {
		c.reusable.Store(false)
		return nil, err
	}
	_, payload, err := c.connection.ReadMessage()
	clearDeadline()
	if err == nil {
		return payload, nil
	}

	c.reusable.Store(false)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var networkError net.Error
	if idleTimeout > 0 && errors.As(err, &networkError) && networkError.Timeout() {
		return nil, fmt.Errorf("WebSocket idle timeout after %s", idleTimeout)
	}
	return nil, fmt.Errorf("read Codex WebSocket frame: %w", err)
}

func (c *gorillaOpenAICodexWebSocket) Close(code int, reason string) error {
	if !c.reusable.Swap(false) {
		return c.connection.Close()
	}
	message := websocket.FormatCloseMessage(code, reason)
	_ = c.connection.WriteControl(websocket.CloseMessage, message, time.Now().Add(time.Second))
	return c.connection.Close()
}

func (c *gorillaOpenAICodexWebSocket) Reusable() bool {
	return c.reusable.Load()
}

func contextDeadline(ctx context.Context) time.Time {
	if deadline, ok := ctx.Deadline(); ok {
		return deadline
	}
	return time.Time{}
}

func armOpenAICodexContextDeadline(
	ctx context.Context,
	setDeadline func(time.Time) error,
	deadline time.Time,
) (func(), error) {
	if err := setDeadline(deadline); err != nil {
		return nil, err
	}
	cancellationDone := make(chan struct{})
	stopCancellation := context.AfterFunc(ctx, func() {
		defer close(cancellationDone)
		_ = setDeadline(time.Now())
	})
	return func() {
		if !stopCancellation() {
			<-cancellationDone
		}
		_ = setDeadline(time.Time{})
	}, nil
}

type openAICodexWebSocketContinuation struct {
	lastRequestInvariant []byte
	lastRequestInput     []json.RawMessage
	lastResponseID       string
	lastResponseItems    []json.RawMessage
}

type openAICodexWebSocketSession struct {
	connection     OpenAICodexWebSocket
	busy           bool
	createdAt      time.Time
	idleTimer      *time.Timer
	idleGeneration uint64
	continuation   *openAICodexWebSocketContinuation
}

var openAICodexWebSocketState = struct {
	sync.Mutex
	stats        map[string]OpenAICodexWebSocketDebugStats
	sseFallbacks map[string]bool
	sessions     map[string]*openAICodexWebSocketSession
}{
	stats:        map[string]OpenAICodexWebSocketDebugStats{},
	sseFallbacks: map[string]bool{},
	sessions:     map[string]*openAICodexWebSocketSession{},
}

type openAICodexWebSocketLease struct {
	connection OpenAICodexWebSocket
	sessionID  string
	session    *openAICodexWebSocketSession
	reused     bool
	transient  bool
}

func acquireOpenAICodexWebSocket(
	ctx context.Context,
	dialer OpenAICodexWebSocketDialer,
	endpoint string,
	headers map[string]string,
	sessionID string,
	now time.Time,
	connectTimeout time.Duration,
) (*openAICodexWebSocketLease, error) {
	if dialer == nil {
		dialer = gorillaOpenAICodexWebSocketDialer{}
	}
	sessionID = strings.TrimSpace(sessionID)
	handshakeHeaders := openAICodexWebSocketHandshakeHeaders(headers)
	if sessionID == "" {
		connection, err := dialer.Dial(ctx, endpoint, handshakeHeaders, connectTimeout)
		if err != nil {
			return nil, err
		}
		return &openAICodexWebSocketLease{
			connection: connection,
			transient:  true,
		}, nil
	}

	var stale OpenAICodexWebSocket
	staleReason := "done"
	openAICodexWebSocketState.Lock()
	cached := openAICodexWebSocketState.sessions[sessionID]
	if cached != nil {
		if cached.idleTimer != nil {
			cached.idleTimer.Stop()
			cached.idleTimer = nil
			cached.idleGeneration++
		}
		switch {
		case !cached.busy && now.Sub(cached.createdAt) >= openAICodexWebSocketSessionMaxAge:
			delete(openAICodexWebSocketState.sessions, sessionID)
			stale = cached.connection
			staleReason = "connection_age_limit"
		case !cached.busy && cached.connection.Reusable():
			cached.busy = true
			openAICodexWebSocketState.Unlock()
			return &openAICodexWebSocketLease{
				connection: cached.connection,
				sessionID:  sessionID,
				session:    cached,
				reused:     true,
			}, nil
		case cached.busy:
			openAICodexWebSocketState.Unlock()
			connection, err := dialer.Dial(ctx, endpoint, handshakeHeaders, connectTimeout)
			if err != nil {
				return nil, err
			}
			return &openAICodexWebSocketLease{
				connection: connection,
				sessionID:  sessionID,
				transient:  true,
			}, nil
		default:
			delete(openAICodexWebSocketState.sessions, sessionID)
			stale = cached.connection
		}
	}
	openAICodexWebSocketState.Unlock()
	closeOpenAICodexWebSocket(stale, staleReason)

	connection, err := dialer.Dial(ctx, endpoint, handshakeHeaders, connectTimeout)
	if err != nil {
		return nil, err
	}
	session := &openAICodexWebSocketSession{
		connection: connection,
		busy:       true,
		createdAt:  now,
	}

	openAICodexWebSocketState.Lock()
	if openAICodexWebSocketState.sessions[sessionID] == nil {
		openAICodexWebSocketState.sessions[sessionID] = session
		openAICodexWebSocketState.Unlock()
		return &openAICodexWebSocketLease{
			connection: connection,
			sessionID:  sessionID,
			session:    session,
		}, nil
	}
	openAICodexWebSocketState.Unlock()
	return &openAICodexWebSocketLease{
		connection: connection,
		sessionID:  sessionID,
		transient:  true,
	}, nil
}

func (lease *openAICodexWebSocketLease) release(keep bool) {
	if lease == nil || lease.connection == nil {
		return
	}
	if lease.transient || lease.session == nil {
		closeOpenAICodexWebSocket(lease.connection, "done")
		return
	}

	shouldClose := !keep || !lease.connection.Reusable()
	openAICodexWebSocketState.Lock()
	current := openAICodexWebSocketState.sessions[lease.sessionID]
	if current != lease.session {
		openAICodexWebSocketState.Unlock()
		closeOpenAICodexWebSocket(lease.connection, "done")
		return
	}
	if shouldClose {
		delete(openAICodexWebSocketState.sessions, lease.sessionID)
		if lease.session.idleTimer != nil {
			lease.session.idleTimer.Stop()
			lease.session.idleTimer = nil
		}
		openAICodexWebSocketState.Unlock()
		closeOpenAICodexWebSocket(lease.connection, "done")
		return
	}

	lease.session.busy = false
	lease.session.idleGeneration++
	generation := lease.session.idleGeneration
	lease.session.idleTimer = time.AfterFunc(openAICodexWebSocketSessionCacheTTL, func() {
		expireOpenAICodexWebSocketSession(lease.sessionID, lease.session, generation)
	})
	openAICodexWebSocketState.Unlock()
}

func expireOpenAICodexWebSocketSession(
	sessionID string,
	session *openAICodexWebSocketSession,
	generation uint64,
) {
	var connection OpenAICodexWebSocket
	openAICodexWebSocketState.Lock()
	current := openAICodexWebSocketState.sessions[sessionID]
	if current == session && !session.busy && session.idleGeneration == generation {
		delete(openAICodexWebSocketState.sessions, sessionID)
		session.idleTimer = nil
		connection = session.connection
	}
	openAICodexWebSocketState.Unlock()
	closeOpenAICodexWebSocket(connection, "idle_timeout")
}

func CloseOpenAICodexWebSocketSessions(sessionIDs ...string) {
	var sessions []*openAICodexWebSocketSession
	openAICodexWebSocketState.Lock()
	if len(sessionIDs) > 0 && strings.TrimSpace(sessionIDs[0]) != "" {
		sessionID := strings.TrimSpace(sessionIDs[0])
		if session := openAICodexWebSocketState.sessions[sessionID]; session != nil {
			sessions = append(sessions, session)
			delete(openAICodexWebSocketState.sessions, sessionID)
		}
	} else {
		sessions = make([]*openAICodexWebSocketSession, 0, len(openAICodexWebSocketState.sessions))
		for _, session := range openAICodexWebSocketState.sessions {
			sessions = append(sessions, session)
		}
		openAICodexWebSocketState.sessions = map[string]*openAICodexWebSocketSession{}
	}
	for _, session := range sessions {
		if session.idleTimer != nil {
			session.idleTimer.Stop()
			session.idleTimer = nil
		}
	}
	openAICodexWebSocketState.Unlock()

	for _, session := range sessions {
		closeOpenAICodexWebSocket(session.connection, "debug_close")
	}
}

func closeOpenAICodexWebSocket(connection OpenAICodexWebSocket, reason string) {
	if connection != nil {
		_ = connection.Close(websocket.CloseNormalClosure, reason)
	}
}

func openAICodexWebSocketHandshakeHeaders(headers map[string]string) map[string]string {
	result := make(map[string]string, len(headers))
	for key, value := range headers {
		if strings.EqualFold(key, "OpenAI-Beta") {
			continue
		}
		result[key] = value
	}
	return result
}

type openAICodexWebSocketRequestBody struct {
	fields             map[string]json.RawMessage
	input              []json.RawMessage
	invariant          []byte
	previousResponseID string
	store              bool
}

type openAICodexWebSocketRequestPlan struct {
	fields             map[string]json.RawMessage
	input              []json.RawMessage
	previousResponseID string
	delta              bool
}

func newOpenAICodexWebSocketRequestBody(payload any) (openAICodexWebSocketRequestBody, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return openAICodexWebSocketRequestBody{}, fmt.Errorf("encode Codex WebSocket request: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return openAICodexWebSocketRequestBody{}, fmt.Errorf("decode Codex WebSocket request object: %w", err)
	}
	if fields == nil {
		return openAICodexWebSocketRequestBody{}, fmt.Errorf("Codex WebSocket request must be a JSON object")
	}

	var input []json.RawMessage
	if value := fields["input"]; len(value) > 0 {
		if err := json.Unmarshal(value, &input); err != nil {
			return openAICodexWebSocketRequestBody{}, fmt.Errorf("decode Codex WebSocket input: %w", err)
		}
	}
	for index := range input {
		input[index], err = canonicalOpenAICodexJSON(input[index])
		if err != nil {
			return openAICodexWebSocketRequestBody{}, fmt.Errorf("canonicalize Codex WebSocket input: %w", err)
		}
	}

	invariantFields := cloneOpenAICodexRawFields(fields)
	delete(invariantFields, "input")
	delete(invariantFields, "previous_response_id")
	invariant, err := json.Marshal(invariantFields)
	if err != nil {
		return openAICodexWebSocketRequestBody{}, err
	}
	var store bool
	_ = json.Unmarshal(fields["store"], &store)
	var previousResponseID string
	_ = json.Unmarshal(fields["previous_response_id"], &previousResponseID)
	return openAICodexWebSocketRequestBody{
		fields:             cloneOpenAICodexRawFields(fields),
		input:              cloneOpenAICodexRawMessages(input),
		invariant:          invariant,
		previousResponseID: previousResponseID,
		store:              store,
	}, nil
}

func buildOpenAICodexWebSocketRequestPlan(
	session *openAICodexWebSocketSession,
	full openAICodexWebSocketRequestBody,
	useCachedContext bool,
) openAICodexWebSocketRequestPlan {
	plan := openAICodexWebSocketRequestPlan{
		fields:             cloneOpenAICodexRawFields(full.fields),
		input:              cloneOpenAICodexRawMessages(full.input),
		previousResponseID: full.previousResponseID,
		delta:              full.previousResponseID != "",
	}
	if !useCachedContext || session == nil || session.continuation == nil {
		return plan
	}

	continuation := session.continuation
	if !bytes.Equal(full.invariant, continuation.lastRequestInvariant) ||
		strings.TrimSpace(continuation.lastResponseID) == "" {
		session.continuation = nil
		return plan
	}
	baseline := make([]json.RawMessage, 0, len(continuation.lastRequestInput)+len(continuation.lastResponseItems))
	baseline = append(baseline, continuation.lastRequestInput...)
	baseline = append(baseline, continuation.lastResponseItems...)
	if len(full.input) < len(baseline) {
		session.continuation = nil
		return plan
	}
	for index := range baseline {
		if !bytes.Equal(full.input[index], baseline[index]) {
			session.continuation = nil
			return plan
		}
	}

	plan.input = cloneOpenAICodexRawMessages(full.input[len(baseline):])
	plan.previousResponseID = continuation.lastResponseID
	plan.delta = true
	return plan
}

func (plan openAICodexWebSocketRequestPlan) frame() ([]byte, error) {
	fields := cloneOpenAICodexRawFields(plan.fields)
	fields["type"] = json.RawMessage(`"response.create"`)
	input, err := json.Marshal(plan.input)
	if err != nil {
		return nil, err
	}
	fields["input"] = input
	if plan.previousResponseID != "" {
		previousResponseID, err := json.Marshal(plan.previousResponseID)
		if err != nil {
			return nil, err
		}
		fields["previous_response_id"] = previousResponseID
	} else {
		delete(fields, "previous_response_id")
	}
	return json.Marshal(fields)
}

func updateOpenAICodexWebSocketContinuation(
	session *openAICodexWebSocketSession,
	full openAICodexWebSocketRequestBody,
	responseID string,
	responseItems []json.RawMessage,
) {
	if session == nil || strings.TrimSpace(responseID) == "" {
		return
	}
	session.continuation = &openAICodexWebSocketContinuation{
		lastRequestInvariant: append([]byte(nil), full.invariant...),
		lastRequestInput:     cloneOpenAICodexRawMessages(full.input),
		lastResponseID:       responseID,
		lastResponseItems:    cloneOpenAICodexRawMessages(responseItems),
	}
}

func clearOpenAICodexWebSocketContinuation(session *openAICodexWebSocketSession) {
	if session != nil {
		session.continuation = nil
	}
}

func recordOpenAICodexWebSocketRequest(
	sessionID string,
	reused bool,
	useCachedContext bool,
	store bool,
	plan openAICodexWebSocketRequestPlan,
) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	openAICodexWebSocketState.Lock()
	defer openAICodexWebSocketState.Unlock()

	stats := openAICodexWebSocketState.stats[sessionID]
	stats.Requests++
	if reused {
		stats.ConnectionsReused++
	} else {
		stats.ConnectionsCreated++
	}
	if useCachedContext {
		stats.CachedContextRequests++
	}
	if store {
		stats.StoreTrueRequests++
	}
	stats.LastInputItems = len(plan.input)
	if plan.delta {
		stats.DeltaRequests++
		value := len(plan.input)
		stats.LastDeltaInputItems = &value
		stats.LastPreviousResponseID = plan.previousResponseID
	} else {
		stats.FullContextRequests++
		stats.LastDeltaInputItems = nil
		stats.LastPreviousResponseID = ""
	}
	openAICodexWebSocketState.stats[sessionID] = stats
}

func cloneOpenAICodexRawFields(fields map[string]json.RawMessage) map[string]json.RawMessage {
	result := make(map[string]json.RawMessage, len(fields))
	for key, value := range fields {
		result[key] = append(json.RawMessage(nil), value...)
	}
	return result
}

func cloneOpenAICodexRawMessages(messages []json.RawMessage) []json.RawMessage {
	result := make([]json.RawMessage, len(messages))
	for index, message := range messages {
		result[index] = append(json.RawMessage(nil), message...)
	}
	return result
}

func canonicalOpenAICodexJSON(raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}
