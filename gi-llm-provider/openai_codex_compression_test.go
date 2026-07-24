package gillmprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestPrepareOpenAICodexSSERequestClonesHeadersAndCompresses(t *testing.T) {
	headers := map[string]string{"Content-Encoding": "identity"}
	payload := map[string]any{
		"input": []map[string]any{{
			"role":    "user",
			"content": strings.Repeat("compress me ", 100),
		}},
	}
	request, err := prepareOpenAICodexSSERequest(payload, headers)
	if err != nil {
		t.Fatal(err)
	}
	if headers["Content-Encoding"] != "identity" {
		t.Fatalf("input headers mutated: %#v", headers)
	}
	if value, ok := lookupHeader(request.headers, "content-encoding"); !ok || value != "zstd" {
		t.Fatalf("request headers = %#v", request.headers)
	}
	decoded := decodeOpenAICodexZstdRequest(t, request.body)
	want, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, want) {
		t.Fatalf("decoded request = %s, want %s", decoded, want)
	}
}

func TestPrepareOpenAICodexSSERequestFallsBackToJSON(t *testing.T) {
	payload := map[string]any{"input": []string{"hello"}}
	request, err := prepareOpenAICodexSSERequestWithCompressor(
		payload,
		nil,
		func(body []byte) ([]byte, bool) {
			return body, false
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lookupHeader(request.headers, "content-encoding"); ok {
		t.Fatalf("fallback headers = %#v", request.headers)
	}
	want, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(request.body, want) {
		t.Fatalf("fallback request = %s, want %s", request.body, want)
	}
}

func TestOpenAICodexResponsesProviderZstdCompressesSSERequestBodies(t *testing.T) {
	for _, text := range []string{
		"hi",
		strings.Repeat("compress me ", 400),
	} {
		t.Run(openAICodexCompressionTestCaseLabel(text), func(t *testing.T) {
			var (
				capturedBody     []byte
				capturedEncoding string
				expectedBody     []byte
			)
			client := openAICodexRetryDoerFunc(func(request *http.Request) (*http.Response, error) {
				var err error
				capturedBody, err = io.ReadAll(request.Body)
				if err != nil {
					return nil, err
				}
				capturedEncoding = request.Header.Get("content-encoding")
				return openAICodexRetryResponse(
					request,
					http.StatusOK,
					http.Header{"content-type": []string{"text/event-stream"}},
					"data: "+codexCompletedEvent("resp_zstd")+"\n\n",
				), nil
			})
			provider := NewOpenAICodexResponsesProvider(client)
			stream, err := provider.StreamSimple(
				openAICodexWebSocketTestModel(),
				Context{Messages: []Message{UserMessageText(text)}},
				SimpleStreamOptions{
					APIKey:    mockOpenAICodexToken(t, "acc_test"),
					Transport: "sse",
					OnPayload: func(payload any, _ Model) (any, bool, error) {
						encoded, encodeErr := json.Marshal(payload)
						expectedBody = encoded
						return nil, false, encodeErr
					},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			result, err := stream.Result(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if result.ResponseID != "resp_zstd" ||
				capturedEncoding != "zstd" ||
				len(capturedBody) == 0 ||
				bytes.Equal(capturedBody, expectedBody) {
				t.Fatalf(
					"result=%#v encoding=%q compressed=%d original=%d",
					result,
					capturedEncoding,
					len(capturedBody),
					len(expectedBody),
				)
			}
			if decoded := decodeOpenAICodexZstdRequest(t, capturedBody); !bytes.Equal(decoded, expectedBody) {
				t.Fatalf("decoded request = %s, want %s", decoded, expectedBody)
			}
		})
	}
}

func decodeOpenAICodexZstdRequest(t *testing.T, body []byte) []byte {
	t.Helper()
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	decoded, err := decoder.DecodeAll(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func openAICodexCompressionTestCaseLabel(text string) string {
	if len(text) < 100 {
		return "small body"
	}
	return "large body"
}
