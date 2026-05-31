package gillmprovider

import (
	"context"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"testing"
)

func TestLazyProviderModuleLoadingPiParity(t *testing.T) {
	t.Run("does not load provider SDKs when importing the root barrel", func(t *testing.T) {
		RegisterBuiltInAPIProviders()
		for _, api := range []string{
			"anthropic-messages",
			"openai-completions",
			"openai-responses",
			"openai-codex-responses",
			"azure-openai-responses",
			"google-generative-ai",
			"mistral-conversations",
			"bedrock-converse-stream",
		} {
			if GetAPIProvider(api) == nil {
				t.Fatalf("built-in provider %q missing", api)
			}
		}
		assertNoExternalProviderSDKDependencies(t)
	})

	t.Run("loads only the Anthropic SDK when calling the root lazy wrapper", func(t *testing.T) {
		client := &lazyModuleLoadHTTPDoer{}
		provider := NewAnthropicMessagesProvider(client)
		message := completeAnthropicLazyModuleProbe(t, provider)
		if text := strings.TrimSpace(messageTextForLazyModuleProbe(message)); text != "Hello" {
			t.Fatalf("message text = %q, want Hello", text)
		}
		if client.calls != 1 || !strings.Contains(client.lastURL, "/v1/messages") {
			t.Fatalf("anthropic client calls = %d url = %q", client.calls, client.lastURL)
		}
		assertNoExternalProviderSDKDependencies(t)
	})

	t.Run("loads only the Anthropic SDK when dispatching through streamSimple", func(t *testing.T) {
		client := &lazyModuleLoadHTTPDoer{}
		RegisterAPIProviderWithSource("anthropic-messages", NewAnthropicMessagesProvider(client), "lazy-module-load-test")
		t.Cleanup(ResetAPIProviders)

		message, err := CompleteSimple(context.Background(), lazyModuleLoadAnthropicModel(), lazyModuleLoadContext(), SimpleStreamOptions{
			APIKey: "test-key",
		})
		if err != nil {
			t.Fatal(err)
		}
		if text := strings.TrimSpace(messageTextForLazyModuleProbe(message)); text != "Hello" {
			t.Fatalf("message text = %q, want Hello", text)
		}
		if client.calls != 1 || !strings.Contains(client.lastURL, "/v1/messages") {
			t.Fatalf("anthropic dispatch calls = %d url = %q", client.calls, client.lastURL)
		}
		assertNoExternalProviderSDKDependencies(t)
	})
}

func completeAnthropicLazyModuleProbe(t *testing.T, provider AnthropicMessagesProvider) Message {
	t.Helper()
	stream, err := provider.StreamSimple(lazyModuleLoadAnthropicModel(), lazyModuleLoadContext(), SimpleStreamOptions{
		APIKey: "test-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	message, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return message
}

func lazyModuleLoadAnthropicModel() Model {
	return Model{
		ID:       "claude-sonnet-4-6",
		Name:     "Claude Sonnet 4",
		API:      "anthropic-messages",
		Provider: "anthropic",
		BaseURL:  "https://api.anthropic.com",
	}
}

func lazyModuleLoadContext() Context {
	return Context{Messages: []Message{
		UserMessageText("hi"),
	}}
}

type lazyModuleLoadHTTPDoer struct {
	calls   int
	lastURL string
}

func (c *lazyModuleLoadHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	c.calls++
	c.lastURL = request.URL.String()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(lazyModuleLoadAnthropicSSE())),
		Request:    request,
	}, nil
}

func lazyModuleLoadAnthropicSSE() string {
	return strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_lazy","usage":{"input_tokens":1,"output_tokens":0}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":1,"output_tokens":1}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
		``,
	}, "\n")
}

func messageTextForLazyModuleProbe(message Message) string {
	var builder strings.Builder
	for _, part := range message.Content {
		if part.Type == ContentText {
			builder.WriteString(part.Text)
		}
	}
	return builder.String()
}

func assertNoExternalProviderSDKDependencies(t *testing.T) {
	t.Helper()
	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("build info unavailable")
	}
	forbidden := []string{
		"github.com/anthropics/anthropic-sdk-go",
		"github.com/openai/openai-go",
		"github.com/google/generative-ai-go",
		"github.com/mistralai/client-go",
		"github.com/aws/aws-sdk-go-v2",
	}
	for _, dep := range info.Deps {
		for _, forbiddenPath := range forbidden {
			if strings.HasPrefix(dep.Path, forbiddenPath) {
				t.Fatalf("external provider SDK dependency %q should not be linked in Gi provider core", dep.Path)
			}
		}
	}
}
