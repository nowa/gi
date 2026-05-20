package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestRPCPromptResponseSemanticsPreflightFailureOnce(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	host.PromptPreflight = func(command RPCCommand) error {
		return errors.New("No API key found for fake-provider.\n\nUse /login to log into a provider via OAuth or API key. See:")
	}
	var output []string
	processor := &RPCLineProcessor{Host: host, WriteLine: func(line string) { output = append(output, line) }}

	processor.HandleLine(context.Background(), `{"id":"b1","type":"prompt","message":"Hello"}`)

	responses := promptResponsesByID(t, output, "b1")
	if len(responses) != 1 {
		t.Fatalf("responses = %#v, want one failure", responses)
	}
	if responses[0].Success || !strings.Contains(responses[0].Error, "No API key found for fake-provider") {
		t.Fatalf("failure response = %#v", responses[0])
	}
}

func TestRPCPromptResponseSemanticsSuccessOnce(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	var output []string
	processor := &RPCLineProcessor{Host: host, WriteLine: func(line string) { output = append(output, line) }}

	processor.HandleLine(context.Background(), `{"id":"b2","type":"prompt","message":"Hello"}`)

	responses := promptResponsesByID(t, output, "b2")
	if len(responses) != 1 {
		t.Fatalf("responses = %#v, want one success", responses)
	}
	if !responses[0].Success {
		t.Fatalf("success response = %#v", responses[0])
	}
}

func TestRPCPromptResponseSemanticsQueuedDuringStreamingSuccessOnce(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	host, _, _ := createRPCSessionHostForTest(t, func(options *AgentSessionOptions) {
		var first bool
		options.Responder = func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error) {
			if !first {
				first = true
				close(started)
				<-release
			}
			return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done")}}, nil
		}
	})
	var output []string
	processor := &RPCLineProcessor{Host: host, WriteLine: func(line string) { output = append(output, line) }}

	processor.HandleLine(context.Background(), `{"id":"b3-start","type":"prompt","message":"Start"}`)
	<-started
	if responses := promptResponsesByID(t, output, "b3-start"); len(responses) != 1 || !responses[0].Success {
		t.Fatalf("start responses = %#v", responses)
	}

	output = nil
	processor.HandleLine(context.Background(), `{"id":"b3","type":"prompt","message":"Queue this","streamingBehavior":"followUp"}`)

	responses := promptResponsesByID(t, output, "b3")
	if len(responses) != 1 {
		t.Fatalf("queued responses = %#v, want one success", responses)
	}
	if !responses[0].Success {
		t.Fatalf("queued response = %#v", responses[0])
	}
	close(release)
}

func promptResponsesByID(t *testing.T, lines []string, id string) []RPCResponse {
	t.Helper()
	var responses []RPCResponse
	for _, chunk := range lines {
		for _, line := range strings.Split(chunk, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var response RPCResponse
			if err := json.Unmarshal([]byte(line), &response); err != nil {
				t.Fatalf("json.Unmarshal(%q) error = %v", line, err)
			}
			if response.ID == id && response.Type == "response" && response.Command == RPCCommandPrompt {
				responses = append(responses, response)
			}
		}
	}
	return responses
}
