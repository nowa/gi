package gillmprovider

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

type fakeBedrockAWSStream struct {
	events chan types.ConverseStreamOutput
	err    error
	closed bool
}

func (s *fakeBedrockAWSStream) Events() <-chan types.ConverseStreamOutput {
	return s.events
}

func (s *fakeBedrockAWSStream) Close() error {
	s.closed = true
	return nil
}

func (s *fakeBedrockAWSStream) Err() error {
	return s.err
}

type fakeBedrockAWSInvoker struct {
	input  *bedrockruntime.ConverseStreamInput
	result bedrockAWSConverseStreamResult
	err    error
}

func (i *fakeBedrockAWSInvoker) ConverseStream(
	_ context.Context,
	input *bedrockruntime.ConverseStreamInput,
) (bedrockAWSConverseStreamResult, error) {
	i.input = input
	return i.result, i.err
}

func TestAWSBedrockTransportConvertsRequestResponseAndEvents(t *testing.T) {
	index := int32(0)
	inputTokens := int32(12)
	outputTokens := int32(4)
	totalTokens := int32(16)
	events := make(chan types.ConverseStreamOutput, 6)
	events <- &types.ConverseStreamOutputMemberMessageStart{
		Value: types.MessageStartEvent{Role: types.ConversationRoleAssistant},
	}
	events <- &types.ConverseStreamOutputMemberContentBlockDelta{
		Value: types.ContentBlockDeltaEvent{
			ContentBlockIndex: &index,
			Delta: &types.ContentBlockDeltaMemberText{
				Value: "hello",
			},
		},
	}
	events <- &types.ConverseStreamOutputMemberContentBlockStop{
		Value: types.ContentBlockStopEvent{ContentBlockIndex: &index},
	}
	events <- &types.ConverseStreamOutputMemberMessageStop{
		Value: types.MessageStopEvent{StopReason: types.StopReasonEndTurn},
	}
	events <- &types.ConverseStreamOutputMemberMetadata{
		Value: types.ConverseStreamMetadataEvent{
			Usage: &types.TokenUsage{
				InputTokens:  &inputTokens,
				OutputTokens: &outputTokens,
				TotalTokens:  &totalTokens,
			},
		},
	}
	close(events)
	sdkStream := &fakeBedrockAWSStream{events: events}
	invoker := &fakeBedrockAWSInvoker{
		result: bedrockAWSConverseStreamResult{
			Stream:  sdkStream,
			Status:  http.StatusOK,
			Headers: map[string]string{"x-amzn-requestid": "request-1"},
		},
	}
	var factoryConfig BedrockClientConfig
	var factoryHeaders map[string]string
	transport := newAWSBedrockConverseStreamTransport(func(
		_ context.Context,
		config BedrockClientConfig,
		headers map[string]string,
	) (bedrockAWSInvoker, error) {
		factoryConfig = config
		factoryHeaders = cloneStringMap(headers)
		return invoker, nil
	})
	statusCalls := 0
	model := Model{
		ID:       "anthropic.claude-sonnet-5",
		Provider: "amazon-bedrock",
		API:      "bedrock-converse-stream",
		Compat:   ModelCompat{SupportsStrictMode: ptrBool(true)},
	}
	payload, err := BuildBedrockPayloadChecked(model, Context{
		SystemPrompt: "system",
		Messages:     []Message{UserMessageText("hi")},
		Tools: []Tool{{
			Name:       "lookup",
			Parameters: Object(map[string]Schema{}),
			ConstrainedSampling: &ConstrainedSamplingConfig{
				Type: ConstrainedSamplingJSONSchema,
			},
		}},
	}, BedrockPayloadOptions{ToolChoice: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	request := BedrockConverseStreamRequest{
		Model:        model,
		Payload:      payload,
		ClientConfig: BedrockClientConfig{Region: "us-west-2"},
		MaxTokens:    4096,
		Temperature:  ptrFloat64(0.25),
		Headers: map[string]string{
			"x-caller": "gi",
		},
		RequestMetadata: map[string]string{"owner": "tests"},
		OnResponse: func(status int, headers map[string]string) error {
			statusCalls++
			if status != http.StatusOK ||
				headers["x-amzn-requestid"] != "request-1" {
				t.Fatalf("response = %d %#v", status, headers)
			}
			return nil
		},
	}

	stream, err := transport(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var converted []BedrockConverseStreamEvent
	for event := range stream {
		converted = append(converted, event)
	}

	if factoryConfig.Region != "us-west-2" ||
		!reflect.DeepEqual(factoryHeaders, request.Headers) {
		t.Fatalf("factory config=%#v headers=%#v", factoryConfig, factoryHeaders)
	}
	if statusCalls != 1 || !sdkStream.closed {
		t.Fatalf("status calls=%d stream closed=%v", statusCalls, sdkStream.closed)
	}
	if invoker.input == nil ||
		aws.ToString(invoker.input.ModelId) != model.ID ||
		invoker.input.InferenceConfig == nil ||
		aws.ToInt32(invoker.input.InferenceConfig.MaxTokens) != 4096 ||
		aws.ToFloat32(invoker.input.InferenceConfig.Temperature) != 0.25 ||
		invoker.input.RequestMetadata["owner"] != "tests" ||
		len(invoker.input.Messages) != 1 ||
		len(invoker.input.System) == 0 ||
		invoker.input.ToolConfig == nil {
		t.Fatalf("SDK input = %#v", invoker.input)
	}
	tool := invoker.input.ToolConfig.Tools[0].(*types.ToolMemberToolSpec)
	if tool.Value.Strict == nil || !*tool.Value.Strict {
		t.Fatalf("SDK tool = %#v", tool.Value)
	}
	if len(converted) != 5 ||
		converted[0].MessageStart == nil ||
		converted[1].ContentBlockDelta == nil ||
		converted[1].ContentBlockDelta.Text != "hello" ||
		converted[4].Metadata == nil ||
		converted[4].Metadata.Usage.TotalTokens != 16 {
		t.Fatalf("events = %#v", converted)
	}
}

func TestAWSBedrockTransportSurfacesStreamAndResponseHookErrors(t *testing.T) {
	t.Run("stream", func(t *testing.T) {
		streamErr := errors.New("event stream failed")
		events := make(chan types.ConverseStreamOutput)
		close(events)
		sdkStream := &fakeBedrockAWSStream{events: events, err: streamErr}
		invoker := &fakeBedrockAWSInvoker{
			result: bedrockAWSConverseStreamResult{Stream: sdkStream},
		}
		transport := newAWSBedrockConverseStreamTransport(func(
			context.Context,
			BedrockClientConfig,
			map[string]string,
		) (bedrockAWSInvoker, error) {
			return invoker, nil
		})
		output, err := transport(context.Background(), BedrockConverseStreamRequest{
			Model:   Model{ID: "model"},
			Payload: BedrockPayload{},
		})
		if err != nil {
			t.Fatal(err)
		}
		event := <-output
		if !errors.Is(event.Error, streamErr) {
			t.Fatalf("event = %#v", event)
		}
	})

	t.Run("response hook", func(t *testing.T) {
		events := make(chan types.ConverseStreamOutput)
		sdkStream := &fakeBedrockAWSStream{events: events}
		invoker := &fakeBedrockAWSInvoker{
			result: bedrockAWSConverseStreamResult{
				Stream: sdkStream,
				Status: http.StatusOK,
			},
		}
		transport := newAWSBedrockConverseStreamTransport(func(
			context.Context,
			BedrockClientConfig,
			map[string]string,
		) (bedrockAWSInvoker, error) {
			return invoker, nil
		})
		hookErr := errors.New("reject response")
		_, err := transport(context.Background(), BedrockConverseStreamRequest{
			Model:      Model{ID: "model"},
			Payload:    BedrockPayload{},
			OnResponse: func(int, map[string]string) error { return hookErr },
		})
		if !errors.Is(err, hookErr) || !sdkStream.closed {
			t.Fatalf("error=%v closed=%v", err, sdkStream.closed)
		}
	})
}

func TestApplyBedrockCustomHeadersSkipsAuthenticationFields(t *testing.T) {
	headers := http.Header{
		"Authorization": []string{"owned"},
		"Host":          []string{"bedrock.example"},
		"X-Amz-Date":    []string{"signed"},
	}
	applyBedrockCustomHeaders(headers, map[string]string{
		"authorization": "caller",
		"host":          "caller.example",
		"x-amz-date":    "caller-date",
		"x-caller":      "gi",
	})
	if headers.Get("Authorization") != "owned" ||
		headers.Get("Host") != "bedrock.example" ||
		headers.Get("X-Amz-Date") != "signed" ||
		headers.Get("X-Caller") != "gi" {
		t.Fatalf("headers = %#v", headers)
	}
}

func TestDefaultBedrockAWSInvokerAppliesResolvedClientState(t *testing.T) {
	invoker, err := newDefaultBedrockAWSInvoker(
		context.Background(),
		BedrockClientConfig{
			Region:      "us-west-2",
			Endpoint:    "https://bedrock.example.com",
			BearerToken: "bearer-token",
			Credentials: &BedrockCredentials{
				AccessKeyID:     "access-key",
				SecretAccessKey: "secret-key",
				SessionToken:    "session-token",
			},
			ForceHTTP1: true,
		},
		map[string]string{"x-caller": "gi"},
	)
	if err != nil {
		t.Fatal(err)
	}
	resolved := invoker.(*defaultBedrockAWSInvoker)
	options := resolved.client.Options()
	if options.Region != "us-west-2" ||
		options.BaseEndpoint == nil ||
		*options.BaseEndpoint != "https://bedrock.example.com" ||
		options.BearerAuthTokenProvider == nil ||
		!reflect.DeepEqual(options.AuthSchemePreference, []string{"httpBearerAuth"}) {
		t.Fatalf("options = %#v", options)
	}
	credential, err := options.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if credential.AccessKeyID != "access-key" ||
		credential.SecretAccessKey != "secret-key" ||
		credential.SessionToken != "session-token" {
		t.Fatalf("credential = %#v", credential)
	}
	token, err := options.BearerAuthTokenProvider.RetrieveBearerToken(
		context.Background(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if token.Value != "bearer-token" {
		t.Fatalf("bearer token = %#v", token)
	}
}
