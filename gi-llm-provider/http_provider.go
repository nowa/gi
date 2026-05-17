package gillmprovider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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

func apiKeyOrEnv(provider, explicit string) string {
	if explicit != "" {
		return explicit
	}
	return GetEnvAPIKey(provider)
}

func postSSE(ctx context.Context, client HTTPDoer, endpoint string, headers map[string]string, payload any) (*http.Response, error) {
	return postJSONWithAccept(ctx, client, endpoint, headers, payload, "text/event-stream")
}

func postJSON(ctx context.Context, client HTTPDoer, endpoint string, headers map[string]string, payload any) (*http.Response, error) {
	return postJSONWithAccept(ctx, client, endpoint, headers, payload, "application/json")
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

func streamError(model Model, format string, args ...any) *AssistantMessageEventStream {
	return ErrorAssistantStream(AssistantErrorMessage(fmt.Sprintf(format, args...), model, false))
}

func responseErrorStream(model Model, response *http.Response) *AssistantMessageEventStream {
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = response.Status
	}
	return streamError(model, "provider returned HTTP %d: %s", response.StatusCode, message)
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
