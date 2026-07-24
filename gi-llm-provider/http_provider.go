package gillmprovider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func httpClientOrDefault(client HTTPDoer) HTTPDoer {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

func apiKeyOrEnv(provider, explicit string, env ProviderEnv) string {
	if explicit != "" {
		return explicit
	}
	return GetEnvAPIKeyWithOverrides(provider, env)
}

func postSSE(ctx context.Context, client HTTPDoer, endpoint string, headers map[string]string, payload any) (*http.Response, error) {
	return postJSONWithAccept(ctx, client, endpoint, headers, payload, "text/event-stream")
}

func postJSONWithAccept(ctx context.Context, client HTTPDoer, endpoint string, headers map[string]string, payload any, accept string) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", accept)
	for key, value := range headers {
		if value != "" {
			req.Header.Set(key, value)
		}
	}
	return client.Do(req)
}

type providerResponseCallback func(status int, headers map[string]string) error

func postSSEWithRetry(
	ctx context.Context,
	client HTTPDoer,
	endpoint string,
	headers map[string]string,
	payload any,
	options ProviderRetryOptions,
	onResponse providerResponseCallback,
) (*http.Response, error) {
	return postJSONWithAcceptAndRetry(ctx, client, endpoint, headers, payload, "text/event-stream", options, onResponse)
}

func postJSONWithRetry(
	ctx context.Context,
	client HTTPDoer,
	endpoint string,
	headers map[string]string,
	payload any,
	options ProviderRetryOptions,
	onResponse providerResponseCallback,
) (*http.Response, error) {
	return postJSONWithAcceptAndRetry(ctx, client, endpoint, headers, payload, "application/json", options, onResponse)
}

func postJSONWithAcceptAndRetry(
	ctx context.Context,
	client HTTPDoer,
	endpoint string,
	headers map[string]string,
	payload any,
	accept string,
	options ProviderRetryOptions,
	onResponse providerResponseCallback,
) (*http.Response, error) {
	return RetryProviderRequest(ctx, options, func(requestContext context.Context) (*http.Response, error) {
		response, err := postJSONWithAccept(requestContext, client, endpoint, headers, payload, accept)
		if err != nil {
			return nil, newProviderTransportError(err)
		}
		headerMap := responseHeaders(response.Header)
		if onResponse != nil {
			if err := onResponse(response.StatusCode, headerMap); err != nil {
				response.Body.Close()
				return nil, err
			}
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return response, nil
		}

		body, readErr := io.ReadAll(io.LimitReader(response.Body, MaxProviderErrorBodyChars+1))
		response.Body.Close()
		if readErr != nil {
			return nil, newProviderHTTPError(response.StatusCode, response.Header, string(body), readErr)
		}
		return nil, newProviderHTTPError(response.StatusCode, response.Header, string(body), errors.New(response.Status))
	})
}

func streamProviderRequestError(model Model, err error, prefix ...string) *AssistantMessageEventStream {
	return streamError(model, "%s", FormatProviderError(NormalizeProviderError(err), prefix...))
}

func providerRetryOptions(maxRetries, maxRetryDelayMillis int) ProviderRetryOptions {
	maxRetryDelay := time.Duration(maxRetryDelayMillis) * time.Millisecond
	return ProviderRetryOptions{
		MaxRetries:    maxRetries,
		MaxRetryDelay: &maxRetryDelay,
	}
}

func streamError(model Model, format string, args ...any) *AssistantMessageEventStream {
	return ErrorAssistantStream(AssistantErrorMessage(fmt.Sprintf(format, args...), model, false))
}

func dispatchSSE(body io.ReadCloser, handle func(data string) error) error {
	return dispatchNamedSSE(body, func(_ string, data string) error {
		return handle(data)
	})
}

func dispatchSSEUntil(body io.ReadCloser, handle func(data string) (bool, error)) error {
	return dispatchNamedSSEUntil(body, func(_ string, data string) (bool, error) {
		return handle(data)
	})
}

func dispatchNamedSSE(body io.ReadCloser, handle func(event, data string) error) error {
	return dispatchNamedSSEUntil(body, func(event, data string) (bool, error) {
		err := handle(event, data)
		return false, err
	})
}

func dispatchNamedSSEUntil(body io.ReadCloser, handle func(event, data string) (bool, error)) error {
	defer body.Close()
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var dataLines []string
	eventName := ""
	flush := func() (bool, error) {
		if len(dataLines) == 0 {
			eventName = ""
			return false, nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = nil
		event := eventName
		eventName = ""
		if strings.TrimSpace(data) == "[DONE]" {
			return false, nil
		}
		return handle(event, data)
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			stop, err := flush()
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	_, err := flush()
	return err
}

func responseHeaders(headers http.Header) map[string]string {
	result := make(map[string]string, len(headers))
	for key, values := range headers {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}

func appendEndpoint(baseURL, defaultBaseURL, path string) string {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, path) {
		return baseURL
	}
	return baseURL + path
}

func responsesEndpoint(baseURL string) string {
	return appendEndpoint(baseURL, "https://api.openai.com/v1", "/responses")
}
