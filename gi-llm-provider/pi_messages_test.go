package gillmprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestPiMessagesPiContracts(t *testing.T) {
	usage := Usage{
		Input:       10,
		Output:      5,
		TotalTokens: 15,
		Cost: UsageCost{
			Input:  0.1,
			Output: 0.2,
			Total:  0.3,
		},
	}
	llmContext := Context{
		Messages: []Message{UserMessageText("Hello")},
	}

	t.Run("streams text and tool calls and resolves the terminal message", func(t *testing.T) {
		server, recorder := newPiMessagesTestServer(t, piMessagesTestResponse{
			events: []PiMessagesEvent{
				{Type: "start"},
				{Type: "text_start", ContentIndex: 0},
				{Type: "text_delta", ContentIndex: 0, Delta: "Hel"},
				{Type: "text_delta", ContentIndex: 0, Delta: "lo"},
				{Type: "text_end", ContentIndex: 0, Content: "Hello"},
				{
					Type:         "toolcall_start",
					ContentIndex: 1,
					ID:           "call_1",
					ToolName:     "read",
				},
				{
					Type:         "toolcall_delta",
					ContentIndex: 1,
					Delta:        `{"path":`,
				},
				{
					Type:         "toolcall_delta",
					ContentIndex: 1,
					Delta:        `"a.txt"}`,
				},
				{
					Type:         "toolcall_end",
					ContentIndex: 1,
					ToolCall: ToolCall(
						"call_1",
						"read",
						map[string]any{"path": "a.txt"},
					),
				},
				{
					Type:       "done",
					Reason:     StopReasonToolUse,
					Usage:      usage,
					ResponseID: "resp_1",
				},
			},
		})
		model := piMessagesTestModel(server.URL + "/v1")
		provider := NewPiMessagesProvider(server.Client())
		stream, err := provider.Stream(model, llmContext, StreamOptions{
			APIKey:     "test-key",
			SessionID:  "session-1",
			ToolChoice: "auto",
			MaxTokens:  100,
			Headers:    map[string]string{"x-custom": "1"},
		})
		if err != nil {
			t.Fatal(err)
		}
		var events []AssistantMessageEvent
		for event := range stream.Events() {
			events = append(events, event)
		}
		message, err := stream.Result(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if message.StopReason != StopReasonToolUse ||
			!reflect.DeepEqual(message.Usage, usage) ||
			message.ResponseID != "resp_1" ||
			message.Model != "auto" ||
			message.Provider != "radius" {
			t.Fatalf("terminal message = %#v", message)
		}
		wantContent := []ContentPart{
			Text("Hello"),
			ToolCall("call_1", "read", map[string]any{"path": "a.txt"}),
		}
		if !reflect.DeepEqual(message.Content, wantContent) {
			t.Fatalf("content = %#v, want %#v", message.Content, wantContent)
		}
		if countAssistantEvent(events, "text_delta") == 0 ||
			countAssistantEvent(events, "toolcall_end") != 1 {
			t.Fatalf("events = %#v", events)
		}

		requests := recorder.snapshot()
		if len(requests) != 1 {
			t.Fatalf("requests = %d, want 1", len(requests))
		}
		request := requests[0]
		if request.URL != "/v1/messages" ||
			request.Headers.Get("Authorization") != "Bearer test-key" ||
			request.Headers.Get("x-custom") != "1" {
			t.Fatalf("request = %#v", request)
		}
		if request.Body.Model != "auto" ||
			request.Body.Options.MaxTokens != 100 ||
			request.Body.Options.SessionID != "session-1" ||
			request.Body.Options.ToolChoice != "auto" ||
			!reflect.DeepEqual(request.Body.Context, llmContext) {
			t.Fatalf("request body = %#v", request.Body)
		}
	})

	t.Run("appends debug=1 and reports response headers via onResponse", func(t *testing.T) {
		server, recorder := newPiMessagesTestServer(t, piMessagesTestResponse{
			headers: map[string]string{
				"x-pi-gateway-upstream-provider": "anthropic",
			},
			events: []PiMessagesEvent{{
				Type:   "done",
				Reason: StopReasonStop,
				Usage:  usage,
			}},
		})
		model := piMessagesTestModel(server.URL + "/v1")
		var observed map[string]string
		provider := NewPiMessagesProvider(server.Client())
		stream, err := provider.StreamSimple(model, llmContext, SimpleStreamOptions{
			APIKey: "test-key",
			Debug:  true,
			OnResponseStatus: func(
				_ int,
				headers map[string]string,
				_ Model,
			) error {
				observed = headers
				return nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		message, err := stream.Result(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		requests := recorder.snapshot()
		if message.StopReason != StopReasonStop ||
			len(requests) != 1 ||
			requests[0].URL != "/v1/messages?debug=1" ||
			observed["x-pi-gateway-upstream-provider"] != "anthropic" {
			t.Fatalf(
				"message=%#v requests=%#v headers=%#v",
				message,
				requests,
				observed,
			)
		}
	})

	t.Run("surfaces backend error responses with diagnostics", func(t *testing.T) {
		server, _ := newPiMessagesTestServer(t, piMessagesTestResponse{
			status:  http.StatusUnauthorized,
			rawBody: `{"error":{"message":"Token expired","code":"unauthorized"}}`,
		})
		model := piMessagesTestModel(server.URL + "/v1")
		provider := NewPiMessagesProvider(server.Client())
		stream, err := provider.Stream(
			model,
			llmContext,
			StreamOptions{APIKey: "stale"},
		)
		if err != nil {
			t.Fatal(err)
		}
		message, err := stream.Result(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if message.StopReason != StopReasonError ||
			!strings.Contains(message.ErrorMessage, "401") ||
			!strings.Contains(message.ErrorMessage, "Token expired") ||
			!strings.Contains(message.ErrorMessage, "unauthorized") ||
			len(message.Diagnostics) != 1 ||
			message.Diagnostics[0].Type != "pi_messages_response_failure" ||
			message.Diagnostics[0].Details["status"] != http.StatusUnauthorized {
			t.Fatalf("error message = %#v", message)
		}
	})

	t.Run("propagates server-sent error events", func(t *testing.T) {
		server, _ := newPiMessagesTestServer(t, piMessagesTestResponse{
			events: []PiMessagesEvent{
				{Type: "start"},
				{
					Type:         "error",
					Reason:       StopReasonError,
					Usage:        usage,
					ErrorMessage: "Upstream failed",
				},
			},
		})
		provider := NewPiMessagesProvider(server.Client())
		stream, err := provider.Stream(
			piMessagesTestModel(server.URL+"/v1"),
			llmContext,
			StreamOptions{APIKey: "test-key"},
		)
		if err != nil {
			t.Fatal(err)
		}
		message, err := stream.Result(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if message.StopReason != StopReasonError ||
			message.ErrorMessage != "Upstream failed" ||
			!reflect.DeepEqual(message.Usage, usage) {
			t.Fatalf("server error message = %#v", message)
		}
	})

	t.Run("errors when no API key is provided", func(t *testing.T) {
		t.Setenv("RADIUS_API_KEY", "")
		provider := NewPiMessagesProvider(nil)
		stream, err := provider.Stream(
			piMessagesTestModel("http://127.0.0.1:1/v1"),
			llmContext,
			StreamOptions{},
		)
		if err != nil {
			t.Fatal(err)
		}
		message, err := stream.Result(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if message.StopReason != StopReasonError ||
			!strings.Contains(message.ErrorMessage, "No API key provided") {
			t.Fatalf("missing-key message = %#v", message)
		}
	})

	t.Run("errors when the stream ends without a terminal event", func(t *testing.T) {
		server, _ := newPiMessagesTestServer(t, piMessagesTestResponse{
			events: []PiMessagesEvent{
				{Type: "start"},
				{Type: "text_start", ContentIndex: 0},
				{Type: "text_delta", ContentIndex: 0, Delta: "partial"},
			},
		})
		provider := NewPiMessagesProvider(server.Client())
		stream, err := provider.Stream(
			piMessagesTestModel(server.URL+"/v1"),
			llmContext,
			StreamOptions{APIKey: "test-key"},
		)
		if err != nil {
			t.Fatal(err)
		}
		message, err := stream.Result(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if message.StopReason != StopReasonError ||
			!strings.Contains(
				message.ErrorMessage,
				"stream ended without a terminal event",
			) {
			t.Fatalf("unterminated message = %#v", message)
		}
	})

	t.Run("is registered as a builtin api provider", func(t *testing.T) {
		if GetAPIProvider(piMessagesAPI) == nil {
			t.Fatal("pi-messages API provider is not registered")
		}
	})

	t.Run("is a known api usable on models", func(t *testing.T) {
		model := piMessagesTestModel("https://radius.example.test/v1")
		if model.API != "pi-messages" {
			t.Fatalf("model API = %q", model.API)
		}
	})
}

func TestPiMessagesRewriteAndRequestScopedCacheRetention(t *testing.T) {
	t.Setenv("PI_CACHE_RETENTION", "")
	server, recorder := newPiMessagesTestServer(t, piMessagesTestResponse{
		events: []PiMessagesEvent{{
			Type:   "done",
			Reason: StopReasonStop,
			Usage:  EmptyUsage(),
			Rewrite: &PiMessagesRewriteImpact{
				PolicyID:            "policy",
				PolicyVersion:       2,
				Changed:             true,
				TokenCountChange:    -5,
				MessageCountChange:  -1,
				SystemPromptChanged: true,
			},
		}},
	})
	provider := NewPiMessagesProvider(server.Client())
	stream, err := provider.Stream(
		piMessagesTestModel(server.URL+"/v1"),
		Context{},
		StreamOptions{
			APIKey: "key",
			Env:    ProviderEnv{"PI_CACHE_RETENTION": "long"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	message, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(message.Diagnostics) != 1 ||
		message.Diagnostics[0].Type != "pi_messages_rewrite" ||
		message.Diagnostics[0].Details["policyId"] != "policy" {
		t.Fatalf("rewrite diagnostics = %#v", message.Diagnostics)
	}
	requests := recorder.snapshot()
	if len(requests) != 1 ||
		requests[0].Body.Options.CacheRetention != "long" {
		t.Fatalf("requests = %#v", requests)
	}
}

type piMessagesTestResponse struct {
	status  int
	headers map[string]string
	events  []PiMessagesEvent
	rawBody string
}

type piMessagesRecordedRequest struct {
	URL     string
	Headers http.Header
	Body    PiMessagesRequest
}

type piMessagesRequestRecorder struct {
	mu       sync.Mutex
	requests []piMessagesRecordedRequest
}

func (r *piMessagesRequestRecorder) record(request piMessagesRecordedRequest) {
	r.mu.Lock()
	r.requests = append(r.requests, request)
	r.mu.Unlock()
}

func (r *piMessagesRequestRecorder) snapshot() []piMessagesRecordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]piMessagesRecordedRequest, len(r.requests))
	for index, request := range r.requests {
		result[index] = request
		result[index].Headers = request.Headers.Clone()
	}
	return result
}

func newPiMessagesTestServer(
	t *testing.T,
	response piMessagesTestResponse,
) (*httptest.Server, *piMessagesRequestRecorder) {
	t.Helper()
	recorder := &piMessagesRequestRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		var payload PiMessagesRequest
		if len(body) > 0 {
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Error(err)
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		recorder.record(piMessagesRecordedRequest{
			URL:     request.URL.RequestURI(),
			Headers: request.Header.Clone(),
			Body:    payload,
		})
		for name, value := range response.headers {
			writer.Header().Set(name, value)
		}
		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		if status != http.StatusOK {
			writer.Header().Set("content-type", "application/json")
			writer.WriteHeader(status)
			_, _ = io.WriteString(writer, response.rawBody)
			return
		}
		writer.Header().Set("content-type", "text/event-stream")
		writer.WriteHeader(http.StatusOK)
		for _, event := range response.events {
			encoded, err := json.Marshal(event)
			if err != nil {
				t.Error(err)
				return
			}
			_, _ = fmt.Fprintf(writer, "data: %s\n\n", encoded)
		}
	}))
	t.Cleanup(server.Close)
	return server, recorder
}

func piMessagesTestModel(baseURL string) Model {
	return Model{
		ID:            "auto",
		Name:          "Radius Auto",
		API:           piMessagesAPI,
		Provider:      "radius",
		BaseURL:       baseURL,
		Input:         []string{"text"},
		Cost:          ModelCost{Input: 1, Output: 2, CacheRead: 0.1, CacheWrite: 0.2},
		ContextWindow: 128000,
		MaxTokens:     16384,
	}
}

func countAssistantEvent(events []AssistantMessageEvent, eventType string) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}
