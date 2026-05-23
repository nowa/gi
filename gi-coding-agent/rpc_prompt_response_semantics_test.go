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

func TestRPCLineProcessorParseErrorMatchesPiRPC(t *testing.T) {
	var output []string
	processor := &RPCLineProcessor{WriteLine: func(line string) { output = append(output, line) }}

	processor.HandleLine(context.Background(), `{"id":`)

	if len(output) != 1 {
		t.Fatalf("output = %#v, want one parse response", output)
	}
	var response RPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output[0])), &response); err != nil {
		t.Fatal(err)
	}
	if response.Type != "response" || response.Command != "parse" || response.Success || !strings.Contains(response.Error, "Failed to parse command") {
		t.Fatalf("parse response = %#v", response)
	}
}

func TestRPCLineProcessorIgnoresExtensionUIResponses(t *testing.T) {
	var output []string
	processor := &RPCLineProcessor{WriteLine: func(line string) { output = append(output, line) }}

	processor.HandleLine(context.Background(), `{"type":"extension_ui_response","id":"ui-1","value":"ok"}`)

	if len(output) != 0 {
		t.Fatalf("output = %#v, want no response", output)
	}
}

func TestRPCLineProcessorRegisterToolPreservesSchemaAndInvokeContext(t *testing.T) {
	host, session, manager := createRPCSessionHostForTest(t)
	runtime := NewProtocolExtensionRuntime(CapabilityToolsRegister)
	runtime.BindSession(session)

	var invokeParams map[string]any
	var processor *RPCLineProcessor
	processor = &RPCLineProcessor{
		Host:                host,
		Runtime:             runtime,
		SourceInfo:          ProtocolSourceInfo{Path: "gi.package.json#toolbox", Source: "local:test", Scope: "temporary", Origin: "package"},
		AllowedCapabilities: []string{CapabilityToolsRegister},
		EnforceCapabilities: true,
		WriteLine: func(line string) {
			var envelope struct {
				Type   string          `json:"type"`
				ID     string          `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := json.Unmarshal([]byte(line), &envelope); err != nil || envelope.Type != "event" || envelope.Method != "tool.invoke" {
				return
			}
			if err := json.Unmarshal(envelope.Params, &invokeParams); err != nil {
				t.Fatal(err)
			}
			response, err := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "ok"}},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			processor.HandleLine(context.Background(), string(response))
		},
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"register_schema_tool","method":"register_tool","params":{"name":"schema_tool","description":"Schema tool","parameters":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}}}`)
	tool := waitForProtocolTool(t, runtime, "schema_tool")
	if tool.Parameters.Properties["query"].Type != "string" || len(tool.Parameters.Required) != 1 || tool.Parameters.Required[0] != "query" {
		t.Fatalf("tool parameters = %#v", tool.Parameters)
	}

	result, err := tool.Execute("tool-call-1", map[string]any{"query": "docs"})
	if err != nil {
		t.Fatal(err)
	}
	if sdkToolText(result) != "ok" {
		t.Fatalf("result = %#v", result)
	}
	if invokeParams["toolName"] != "schema_tool" || invokeParams["name"] != "schema_tool" {
		t.Fatalf("invoke params missing tool name aliases: %#v", invokeParams)
	}
	contextValue, ok := invokeParams["context"].(map[string]any)
	if !ok || contextValue["cwd"] != manager.GetCWD() || contextValue["sessionId"] != manager.GetSessionID() {
		t.Fatalf("invoke context = %#v", invokeParams["context"])
	}
	source, ok := contextValue["source"].(map[string]any)
	if !ok || source["source"] != "local:test" || source["origin"] != "package" {
		t.Fatalf("invoke source = %#v", contextValue["source"])
	}
}

func TestRPCLineProcessorWritesSessionEventsAsJSONL(t *testing.T) {
	var output []string
	processor := &RPCLineProcessor{WriteLine: func(line string) { output = append(output, line) }}
	message := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done")}}

	processor.WriteEvent(AgentSessionEvent{Type: "message_end", Message: &message})

	if len(output) != 1 {
		t.Fatalf("output = %#v, want one event", output)
	}
	if !strings.Contains(output[0], `"type":"message_end"`) || !strings.Contains(output[0], `"role":"assistant"`) {
		t.Fatalf("event output = %q", output[0])
	}
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
