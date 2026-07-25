package gillmprovider

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/smithy-go/auth/bearer"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

type bedrockAWSStream interface {
	Events() <-chan types.ConverseStreamOutput
	Close() error
	Err() error
}

type bedrockAWSConverseStreamResult struct {
	Stream  bedrockAWSStream
	Status  int
	Headers map[string]string
}

type bedrockAWSInvoker interface {
	ConverseStream(
		context.Context,
		*bedrockruntime.ConverseStreamInput,
	) (bedrockAWSConverseStreamResult, error)
}

type bedrockAWSInvokerFactory func(
	context.Context,
	BedrockClientConfig,
	map[string]string,
) (bedrockAWSInvoker, error)

// NewAWSBedrockConverseStreamTransport returns the package's live AWS SDK v2
// transport. The provider still accepts an injected transport for deterministic
// tests and applications that own a custom Bedrock boundary.
func NewAWSBedrockConverseStreamTransport() BedrockConverseStreamTransport {
	return newAWSBedrockConverseStreamTransport(newDefaultBedrockAWSInvoker)
}

func newAWSBedrockConverseStreamTransport(
	factory bedrockAWSInvokerFactory,
) BedrockConverseStreamTransport {
	return func(
		ctx context.Context,
		request BedrockConverseStreamRequest,
	) (<-chan BedrockConverseStreamEvent, error) {
		if factory == nil {
			return nil, errors.New("Bedrock AWS invoker factory is nil")
		}
		input, err := buildAWSBedrockConverseStreamInput(request)
		if err != nil {
			return nil, err
		}
		invoker, err := factory(ctx, request.ClientConfig, request.Headers)
		if err != nil {
			return nil, err
		}
		result, err := invoker.ConverseStream(ctx, input)
		if err != nil {
			return nil, err
		}
		if result.Stream == nil {
			return nil, errors.New("Bedrock ConverseStream returned a nil event stream")
		}
		if request.OnResponse != nil && result.Status != 0 {
			if err := request.OnResponse(
				result.Status,
				cloneStringMap(result.Headers),
			); err != nil {
				result.Stream.Close()
				return nil, err
			}
		}

		events := make(chan BedrockConverseStreamEvent)
		go bridgeAWSBedrockEvents(ctx, result.Stream, events)
		return events, nil
	}
}

type defaultBedrockAWSInvoker struct {
	client   *bedrockruntime.Client
	response *bedrockResponseCapture
}

func newDefaultBedrockAWSInvoker(
	ctx context.Context,
	config BedrockClientConfig,
	headers map[string]string,
) (bedrockAWSInvoker, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	loadOptions := make([]func(*awsconfig.LoadOptions) error, 0, 4)
	if config.Region != "" {
		loadOptions = append(loadOptions, awsconfig.WithRegion(config.Region))
	}
	if config.Profile != "" {
		loadOptions = append(
			loadOptions,
			awsconfig.WithSharedConfigProfile(config.Profile),
		)
	}
	if config.Credentials != nil {
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				config.Credentials.AccessKeyID,
				config.Credentials.SecretAccessKey,
				config.Credentials.SessionToken,
			),
		))
	}

	httpClient, err := newBedrockHTTPClient(config)
	if err != nil {
		return nil, err
	}
	loadOptions = append(loadOptions, awsconfig.WithHTTPClient(httpClient))
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load AWS Bedrock configuration: %w", err)
	}
	if config.BearerToken != "" {
		awsConfig.BearerAuthTokenProvider = bearer.StaticTokenProvider{
			Token: bearer.Token{Value: config.BearerToken},
		}
		awsConfig.AuthSchemePreference = []string{"httpBearerAuth"}
	}

	client := bedrockruntime.NewFromConfig(
		awsConfig,
		func(options *bedrockruntime.Options) {
			if config.Endpoint != "" {
				options.BaseEndpoint = aws.String(config.Endpoint)
			}
			if len(headers) > 0 {
				options.APIOptions = append(
					options.APIOptions,
					addBedrockCustomHeadersMiddleware(headers),
				)
			}
		},
	)
	return &defaultBedrockAWSInvoker{client: client, response: httpClient}, nil
}

func (i *defaultBedrockAWSInvoker) ConverseStream(
	ctx context.Context,
	input *bedrockruntime.ConverseStreamInput,
) (bedrockAWSConverseStreamResult, error) {
	output, err := i.client.ConverseStream(ctx, input)
	if err != nil {
		return bedrockAWSConverseStreamResult{}, err
	}
	status, headers := i.response.snapshot()
	if requestID, ok := awsmiddleware.GetRequestIDMetadata(output.ResultMetadata); ok &&
		requestID != "" {
		if headers == nil {
			headers = map[string]string{}
		}
		if _, exists := headers["x-amzn-requestid"]; !exists {
			headers["x-amzn-requestid"] = requestID
		}
	}
	return bedrockAWSConverseStreamResult{
		Stream:  output.GetStream(),
		Status:  status,
		Headers: headers,
	}, nil
}

type bedrockResponseCapture struct {
	client HTTPDoer
	mu     sync.Mutex
	status int
	header http.Header
}

func newBedrockHTTPClient(
	config BedrockClientConfig,
) (*bedrockResponseCapture, error) {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("default HTTP transport is not *http.Transport")
	}
	cloned := transport.Clone()
	if config.ProxyURL != "" {
		proxyURL, err := url.Parse(config.ProxyURL)
		if err != nil || proxyURL.Scheme == "" || proxyURL.Host == "" {
			return nil, fmt.Errorf("invalid Bedrock proxy URL %q", config.ProxyURL)
		}
		if proxyURL.Scheme != "http" && proxyURL.Scheme != "https" {
			return nil, fmt.Errorf(
				"%s Got %s:",
				UnsupportedProxyProtocolMessage,
				proxyURL.Scheme,
			)
		}
		cloned.Proxy = http.ProxyURL(proxyURL)
	}
	if config.ForceHTTP1 {
		cloned.ForceAttemptHTTP2 = false
		cloned.TLSNextProto = map[string]func(
			string,
			*tls.Conn,
		) http.RoundTripper{}
	}
	capture := &bedrockResponseCapture{
		client: &http.Client{Transport: cloned},
	}
	return capture, nil
}

func (c *bedrockResponseCapture) Do(
	request *http.Request,
) (*http.Response, error) {
	response, err := c.client.Do(request)
	if response != nil {
		c.mu.Lock()
		c.status = response.StatusCode
		c.header = response.Header.Clone()
		c.mu.Unlock()
	}
	return response, err
}

func (c *bedrockResponseCapture) snapshot() (int, map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.header == nil {
		return c.status, nil
	}
	return c.status, responseHeaders(c.header)
}

func addBedrockCustomHeadersMiddleware(
	headers map[string]string,
) func(*middleware.Stack) error {
	copied := cloneStringMap(headers)
	return func(stack *middleware.Stack) error {
		return stack.Build.Add(
			middleware.BuildMiddlewareFunc(
				"gi-bedrock-custom-headers",
				func(
					ctx context.Context,
					input middleware.BuildInput,
					next middleware.BuildHandler,
				) (middleware.BuildOutput, middleware.Metadata, error) {
					if request, ok := input.Request.(*smithyhttp.Request); ok {
						applyBedrockCustomHeaders(request.Header, copied)
					}
					return next.HandleBuild(ctx, input)
				},
			),
			middleware.After,
		)
	}
}

func applyBedrockCustomHeaders(header http.Header, headers map[string]string) {
	for key, value := range headers {
		if !IsReservedBedrockHeader(key) {
			header.Set(key, value)
		}
	}
}

// IsReservedBedrockHeader reports headers owned by HTTP routing or AWS
// authentication. Caller values for these fields are never installed.
func IsReservedBedrockHeader(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	return lower == "authorization" ||
		lower == "host" ||
		strings.HasPrefix(lower, "x-amz-")
}

func bridgeAWSBedrockEvents(
	ctx context.Context,
	stream bedrockAWSStream,
	output chan<- BedrockConverseStreamEvent,
) {
	defer close(output)
	defer stream.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-stream.Events():
			if !ok {
				if err := stream.Err(); err != nil {
					sendBedrockEvent(ctx, output, BedrockConverseStreamEvent{Error: err})
				}
				return
			}
			converted, ok := convertAWSBedrockStreamEvent(event)
			if !ok {
				continue
			}
			if !sendBedrockEvent(ctx, output, converted) {
				return
			}
		}
	}
}

func sendBedrockEvent(
	ctx context.Context,
	output chan<- BedrockConverseStreamEvent,
	event BedrockConverseStreamEvent,
) bool {
	select {
	case <-ctx.Done():
		return false
	case output <- event:
		return true
	}
}

func convertAWSBedrockStreamEvent(
	event types.ConverseStreamOutput,
) (BedrockConverseStreamEvent, bool) {
	switch value := event.(type) {
	case *types.ConverseStreamOutputMemberMessageStart:
		return BedrockConverseStreamEvent{
			MessageStart: &BedrockMessageStartEvent{Role: string(value.Value.Role)},
		}, true
	case *types.ConverseStreamOutputMemberContentBlockStart:
		start := &BedrockContentBlockStartEvent{
			ContentBlockIndex: awsInt32(value.Value.ContentBlockIndex),
		}
		if tool, ok := value.Value.Start.(*types.ContentBlockStartMemberToolUse); ok {
			start.ToolUse = &BedrockToolUseBlock{
				ToolUseID: aws.ToString(tool.Value.ToolUseId),
				Name:      aws.ToString(tool.Value.Name),
				Input:     map[string]any{},
			}
		}
		return BedrockConverseStreamEvent{ContentBlockStart: start}, true
	case *types.ConverseStreamOutputMemberContentBlockDelta:
		delta := &BedrockContentBlockDeltaEvent{
			ContentBlockIndex: awsInt32(value.Value.ContentBlockIndex),
		}
		switch member := value.Value.Delta.(type) {
		case *types.ContentBlockDeltaMemberText:
			delta.Text = member.Value
		case *types.ContentBlockDeltaMemberToolUse:
			delta.ToolUseInput = aws.ToString(member.Value.Input)
		case *types.ContentBlockDeltaMemberReasoningContent:
			reasoning := &BedrockReasoningContent{}
			switch item := member.Value.(type) {
			case *types.ReasoningContentBlockDeltaMemberText:
				reasoning.Text = item.Value
			case *types.ReasoningContentBlockDeltaMemberSignature:
				reasoning.Signature = item.Value
			default:
				return BedrockConverseStreamEvent{}, false
			}
			delta.ReasoningContent = reasoning
		default:
			return BedrockConverseStreamEvent{}, false
		}
		return BedrockConverseStreamEvent{ContentBlockDelta: delta}, true
	case *types.ConverseStreamOutputMemberContentBlockStop:
		return BedrockConverseStreamEvent{
			ContentBlockStop: &BedrockContentBlockStopEvent{
				ContentBlockIndex: awsInt32(value.Value.ContentBlockIndex),
			},
		}, true
	case *types.ConverseStreamOutputMemberMessageStop:
		return BedrockConverseStreamEvent{
			MessageStop: &BedrockMessageStopEvent{
				StopReason: string(value.Value.StopReason),
			},
		}, true
	case *types.ConverseStreamOutputMemberMetadata:
		usage := BedrockUsage{}
		if value.Value.Usage != nil {
			usage = BedrockUsage{
				InputTokens:  awsInt32(value.Value.Usage.InputTokens),
				OutputTokens: awsInt32(value.Value.Usage.OutputTokens),
				CacheReadInputTokens: awsInt32(
					value.Value.Usage.CacheReadInputTokens,
				),
				CacheWriteTokens: awsInt32(
					value.Value.Usage.CacheWriteInputTokens,
				),
				TotalTokens: awsInt32(value.Value.Usage.TotalTokens),
			}
		}
		return BedrockConverseStreamEvent{
			Metadata: &BedrockMetadataEvent{Usage: usage},
		}, true
	default:
		return BedrockConverseStreamEvent{}, false
	}
}

func awsInt32(value *int32) int {
	if value == nil {
		return 0
	}
	return int(*value)
}

var _ bedrockAWSInvoker = (*defaultBedrockAWSInvoker)(nil)
var _ HTTPDoer = (*bedrockResponseCapture)(nil)
