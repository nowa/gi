package gillmprovider

import (
	"context"
	"errors"
	"strings"
)

const googleVertexAPI = "google-vertex"

// GoogleVertexTokenRequest is the immutable authentication projection of one
// resolved Vertex request.
type GoogleVertexTokenRequest struct {
	Project     string
	Location    string
	AuthOptions *GoogleAuthOptions
}

// GoogleVertexTokenProvider resolves an ADC access token for one request.
// Implementations must be safe for concurrent use.
type GoogleVertexTokenProvider interface {
	AccessToken(context.Context, GoogleVertexTokenRequest) (string, error)
}

// GoogleVertexTokenProviderFunc adapts a function to GoogleVertexTokenProvider.
type GoogleVertexTokenProviderFunc func(context.Context, GoogleVertexTokenRequest) (string, error)

func (f GoogleVertexTokenProviderFunc) AccessToken(
	ctx context.Context,
	request GoogleVertexTokenRequest,
) (string, error) {
	if f == nil {
		return "", errors.New("Google Vertex token provider is nil")
	}
	return f(ctx, request)
}

// GoogleVertexAPIProvider owns only stable dependencies. Authentication,
// endpoint selection, headers, and payload are resolved into request-local
// values before the HTTP call begins.
type GoogleVertexAPIProvider struct {
	Client        HTTPDoer
	TokenProvider GoogleVertexTokenProvider
}

type googleVertexRequest struct {
	endpoint string
	headers  map[string]string
}

// NewGoogleVertexAPIProvider creates a Vertex API provider with injectable HTTP
// and ADC token boundaries. Nil dependencies select the package defaults.
func NewGoogleVertexAPIProvider(
	client HTTPDoer,
	tokenProvider GoogleVertexTokenProvider,
) GoogleVertexAPIProvider {
	if tokenProvider == nil {
		tokenProvider = newDefaultGoogleVertexTokenProvider()
	}
	return GoogleVertexAPIProvider{
		Client:        httpClientOrDefault(client),
		TokenProvider: tokenProvider,
	}
}

func init() {
	RegisterBuiltInAPIProvider(googleVertexAPI, NewGoogleVertexAPIProvider(nil, nil))
}

// Stream starts a Vertex AI stream using fully specified options.
func (p GoogleVertexAPIProvider) Stream(
	model Model,
	llmContext Context,
	options StreamOptions,
) (*AssistantMessageEventStream, error) {
	return p.stream(model, llmContext, options)
}

// StreamSimple applies the shared simple-option policy before starting Vertex.
func (p GoogleVertexAPIProvider) StreamSimple(
	model Model,
	llmContext Context,
	options SimpleStreamOptions,
) (*AssistantMessageEventStream, error) {
	return p.stream(model, llmContext, prepareSimpleStreamOptions(model, llmContext, options))
}

func (p GoogleVertexAPIProvider) stream(
	model Model,
	llmContext Context,
	options StreamOptions,
) (*AssistantMessageEventStream, error) {
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}

	clientConfig := ResolveGoogleVertexClientConfig(model, GoogleVertexOptions{
		APIKey:   options.APIKey,
		Project:  options.Project,
		Location: options.Location,
		Env:      options.Env,
	})
	if err := ValidateGoogleVertexClientConfig(clientConfig); err != nil {
		return streamProviderRequestError(model, err), nil
	}
	builtPayload, err := BuildGooglePayloadChecked(model, llmContext, GooglePayloadOptions{
		MaxTokens:       options.MaxTokens,
		Temperature:     options.Temperature,
		Reasoning:       options.Reasoning,
		ThinkingBudgets: options.ThinkingBudgets,
		ToolChoice:      options.ToolChoice,
	})
	if err != nil {
		return streamError(model, "%s", err.Error()), nil
	}
	payload := any(builtPayload)
	if options.OnPayload != nil {
		next, replace, err := options.OnPayload(payload, model)
		if err != nil {
			return streamError(model, "%s", err.Error()), nil
		}
		if replace {
			payload = next
		}
	}

	request, err := p.resolveRequest(ctx, model, clientConfig, options)
	if err != nil {
		return streamProviderRequestError(model, err), nil
	}
	response, err := postSSEWithRetry(
		ctx,
		httpClientOrDefault(p.Client),
		request.endpoint,
		request.headers,
		payload,
		providerRetryOptions(options.MaxRetries, options.MaxRetryDelayMs),
		func(status int, headers map[string]string) error {
			if options.OnResponseStatus == nil {
				return nil
			}
			return options.OnResponseStatus(status, headers, model)
		},
	)
	if err != nil {
		if ctx.Err() != nil {
			return ErrorAssistantStream(AssistantErrorMessage(ctx.Err().Error(), model, true)), nil
		}
		return streamProviderRequestError(model, err), nil
	}

	stream := NewAssistantMessageEventStream()
	go streamGoogleBody(model, response.Body, stream)
	return stream, nil
}

func (p GoogleVertexAPIProvider) resolveRequest(
	ctx context.Context,
	model Model,
	config GoogleVertexClientConfig,
	options StreamOptions,
) (googleVertexRequest, error) {
	endpoint, err := GoogleVertexStreamEndpoint(model, config)
	if err != nil {
		return googleVertexRequest{}, err
	}
	headers := mergeHeadersCaseInsensitive(model.Headers, options.Headers)
	if headers == nil {
		headers = map[string]string{}
	}
	if config.APIKey != "" {
		if !hasHeaderCaseInsensitive(headers, "x-goog-api-key") {
			setHeaderCaseInsensitive(headers, "x-goog-api-key", config.APIKey)
		}
	} else if !hasHeaderCaseInsensitive(headers, "Authorization") {
		tokenProvider := p.TokenProvider
		if tokenProvider == nil {
			tokenProvider = newDefaultGoogleVertexTokenProvider()
		}
		accessToken, err := tokenProvider.AccessToken(ctx, GoogleVertexTokenRequest{
			Project:     config.Project,
			Location:    config.Location,
			AuthOptions: config.GoogleAuthOptions,
		})
		if err != nil {
			return googleVertexRequest{}, err
		}
		accessToken = strings.TrimSpace(accessToken)
		if accessToken == "" {
			return googleVertexRequest{}, errors.New("Google application default credentials returned an empty access token")
		}
		setHeaderCaseInsensitive(headers, "Authorization", "Bearer "+accessToken)
	}
	headers = applyHeaderRemovals(headers, options.HeaderRemovals)
	return googleVertexRequest{endpoint: endpoint, headers: headers}, nil
}

var _ APIProvider = GoogleVertexAPIProvider{}
var _ GoogleVertexTokenProvider = GoogleVertexTokenProviderFunc(nil)
