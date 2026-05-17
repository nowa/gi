package gillmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestAzureOpenAIResponsesProviderStreamsFromHTTP(t *testing.T) {
	var requestPath string
	var rawQuery string
	var apiKeyHeader string
	var payload AzureOpenAIResponsesPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		rawQuery = r.URL.RawQuery
		apiKeyHeader = r.Header.Get("api-key")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("content-type", "text/event-stream")
		writeSSE(t, w, `{"type":"response.created","response":{"id":"resp_azure","status":"in_progress"}}`)
		writeSSE(t, w, `{"type":"response.output_item.added","item":{"type":"message","id":"msg_1","status":"in_progress"}}`)
		writeSSE(t, w, `{"type":"response.content_part.added","part":{"type":"output_text","text":""}}`)
		writeSSE(t, w, `{"type":"response.output_text.delta","delta":"Hello Azure"}`)
		writeSSE(t, w, `{"type":"response.output_item.done","item":{"type":"message","id":"msg_1","status":"completed","content":[{"type":"output_text","text":"Hello Azure"}]}}`)
		writeSSE(t, w, `{"type":"response.completed","response":{"id":"resp_azure","status":"completed","usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7,"input_tokens_details":{"cached_tokens":1,"cache_write_tokens":0}}}}`)
	}))
	defer server.Close()

	model := Model{
		ID:               "gpt-4o-mini",
		Provider:         "azure-openai-responses",
		API:              "azure-openai-responses",
		BaseURL:          server.URL + "/openai/v1",
		Reasoning:        true,
		Input:            []string{"text", "image"},
		ThinkingLevelMap: map[string]*string{"xhigh": ptrString("high")},
	}
	llmContext := Context{
		Messages: []Message{UserMessageText("hi")},
		Tools: []Tool{{
			Name:        "lookup",
			Description: "Lookup",
			Parameters:  Object(map[string]Schema{"query": String()}, "query"),
		}},
	}
	stream, err := NewAzureOpenAIResponsesProvider(server.Client()).StreamSimple(model, llmContext, SimpleStreamOptions{
		APIKey:    "azure-key",
		MaxTokens: 128,
		SessionID: "session-azure",
		Reasoning: "xhigh",
		Metadata: map[string]any{
			"azure_api_version":     "2025-04-01-preview",
			"azure_deployment_name": "deploy-gpt-4o-mini",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := collectAssistantStreamEvents(stream)
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if requestPath != "/openai/v1/responses" || rawQuery != "api-version=2025-04-01-preview" || apiKeyHeader != "azure-key" {
		t.Fatalf("request path/query/key = %q %q %q", requestPath, rawQuery, apiKeyHeader)
	}
	if payload.Model != "deploy-gpt-4o-mini" || !payload.Stream || payload.PromptCacheKey != "session-azure" || payload.MaxOutputTokens != 128 {
		t.Fatalf("payload metadata = %#v", payload)
	}
	if !reflect.DeepEqual(payload.Reasoning, map[string]string{"effort": "high", "summary": "auto"}) || len(payload.Include) != 1 {
		t.Fatalf("reasoning = %#v include=%#v", payload.Reasoning, payload.Include)
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Strict == nil || *payload.Tools[0].Strict {
		t.Fatalf("tools = %#v", payload.Tools)
	}
	if !containsAssistantEvent(events, "text_delta") || !containsAssistantEvent(events, "done") {
		t.Fatalf("events = %#v", events)
	}
	if result.ResponseID != "resp_azure" || result.Content[0].Text != "Hello Azure" || result.Usage.Input != 3 || result.Usage.CacheRead != 1 || result.Usage.TotalTokens != 7 {
		t.Fatalf("result = %#v", result)
	}
}

func TestAzureOpenAIResponsesProviderUsesDeploymentNameMap(t *testing.T) {
	t.Setenv("AZURE_OPENAI_DEPLOYMENT_NAME_MAP", "gpt-4o-mini=deploy-mini, gpt-5-mini=deploy-five")
	model := Model{ID: "gpt-4o-mini", Provider: "azure-openai-responses", API: "azure-openai-responses"}
	if got := ResolveAzureDeploymentName(model, ""); got != "deploy-mini" {
		t.Fatalf("deployment = %q", got)
	}
	if got := ResolveAzureDeploymentName(model, "explicit-deploy"); got != "explicit-deploy" {
		t.Fatalf("explicit deployment = %q", got)
	}
}

func TestAzureOpenAIResponsesEndpointPreservesProxyQuery(t *testing.T) {
	endpoint := azureOpenAIResponsesEndpoint(AzureOpenAIConfig{BaseURL: "https://proxy.example.com/v1?custom=true", APIVersion: "v1"})
	if endpoint != "https://proxy.example.com/v1/responses?api-version=v1&custom=true" {
		t.Fatalf("endpoint = %q", endpoint)
	}
}
