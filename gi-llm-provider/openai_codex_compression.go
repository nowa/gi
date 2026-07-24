package gillmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/klauspost/compress/zstd"
)

const openAICodexRequestCompressionZstdLevel = 3

type openAICodexSSERequest struct {
	headers map[string]string
	body    []byte
}

type openAICodexRequestCompressor func([]byte) ([]byte, bool)

var getOpenAICodexZstdEncoder = sync.OnceValues(func() (*zstd.Encoder, error) {
	return zstd.NewWriter(
		nil,
		zstd.WithEncoderLevel(
			zstd.EncoderLevelFromZstd(openAICodexRequestCompressionZstdLevel),
		),
		zstd.WithEncoderConcurrency(1),
		zstd.WithLowerEncoderMem(true),
	)
})

func prepareOpenAICodexSSERequest(
	payload any,
	headers map[string]string,
) (openAICodexSSERequest, error) {
	return prepareOpenAICodexSSERequestWithCompressor(
		payload,
		headers,
		compressOpenAICodexRequestBodyZstd,
	)
}

func prepareOpenAICodexSSERequestWithCompressor(
	payload any,
	headers map[string]string,
	compress openAICodexRequestCompressor,
) (openAICodexSSERequest, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return openAICodexSSERequest{}, err
	}
	requestHeaders := cloneStringMap(headers)
	if requestHeaders == nil {
		requestHeaders = map[string]string{}
	}
	if compress != nil {
		if compressed, ok := compress(body); ok {
			body = compressed
			setHeaderCaseInsensitive(requestHeaders, "content-encoding", "zstd")
		}
	}
	return openAICodexSSERequest{
		headers: requestHeaders,
		body:    body,
	}, nil
}

func compressOpenAICodexRequestBodyZstd(body []byte) ([]byte, bool) {
	encoder, err := getOpenAICodexZstdEncoder()
	if err != nil || encoder == nil {
		return body, false
	}
	return encoder.EncodeAll(body, make([]byte, 0, len(body))), true
}

func postOpenAICodexSSE(
	ctx context.Context,
	client HTTPDoer,
	endpoint string,
	request openAICodexSSERequest,
) (*http.Response, error) {
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(request.body),
	)
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("content-type", "application/json")
	httpRequest.Header.Set("accept", "text/event-stream")
	for key, value := range request.headers {
		if value != "" {
			httpRequest.Header.Set(key, value)
		}
	}
	return client.Do(httpRequest)
}
