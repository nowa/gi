package gillmprovider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	defaultCodexBaseURL = "https://chatgpt.com/backend-api"
	codexJWTClaimPath   = "https://api.openai.com/auth"
)

type OpenAICodexResponsesPayloadOptions struct {
	Temperature      *float64
	SessionID        string
	ReasoningEffort  string
	ReasoningSummary string
	ServiceTier      string
	TextVerbosity    string
}

type OpenAICodexResponsesPayload struct {
	Model              string                     `json:"model"`
	Store              bool                       `json:"store"`
	Stream             bool                       `json:"stream"`
	Instructions       string                     `json:"instructions"`
	PreviousResponseID string                     `json:"previous_response_id,omitempty"`
	Input              []OpenAIResponsesInputItem `json:"input,omitempty"`
	Tools              []OpenAIResponsesTool      `json:"tools,omitempty"`
	Temperature        *float64                   `json:"temperature,omitempty"`
	Reasoning          map[string]string          `json:"reasoning,omitempty"`
	ServiceTier        string                     `json:"service_tier,omitempty"`
	Text               map[string]string          `json:"text,omitempty"`
	Include            []string                   `json:"include,omitempty"`
	PromptCacheKey     string                     `json:"prompt_cache_key,omitempty"`
	ToolChoice         string                     `json:"tool_choice,omitempty"`
	ParallelToolCalls  bool                       `json:"parallel_tool_calls"`
}

type OpenAICodexProcessOptions struct {
	ServiceTier                string
	GrammarToolInputProperties map[string]string
}

type OpenAICodexWebSocketDebugStats struct {
	Requests                int    `json:"requests"`
	ConnectionsCreated      int    `json:"connectionsCreated"`
	ConnectionsReused       int    `json:"connectionsReused"`
	CachedContextRequests   int    `json:"cachedContextRequests"`
	StoreTrueRequests       int    `json:"storeTrueRequests"`
	FullContextRequests     int    `json:"fullContextRequests"`
	DeltaRequests           int    `json:"deltaRequests"`
	LastInputItems          int    `json:"lastInputItems"`
	LastDeltaInputItems     *int   `json:"lastDeltaInputItems,omitempty"`
	LastPreviousResponseID  string `json:"lastPreviousResponseId,omitempty"`
	WebSocketFailures       int    `json:"websocketFailures"`
	SSEFallbacks            int    `json:"sseFallbacks"`
	WebSocketFallbackActive bool   `json:"websocketFallbackActive,omitempty"`
	LastWebSocketError      string `json:"lastWebSocketError,omitempty"`
}

func BuildOpenAICodexResponsesPayload(model Model, context Context, options OpenAICodexResponsesPayloadOptions) OpenAICodexResponsesPayload {
	payload, _, _ := buildOpenAICodexResponsesPayload(model, context, options)
	return payload
}

func BuildOpenAICodexResponsesPayloadChecked(
	model Model,
	context Context,
	options OpenAICodexResponsesPayloadOptions,
) (OpenAICodexResponsesPayload, error) {
	payload, _, err := buildOpenAICodexResponsesPayload(model, context, options)
	return payload, err
}

func buildOpenAICodexResponsesPayload(
	model Model,
	context Context,
	options OpenAICodexResponsesPayloadOptions,
) (OpenAICodexResponsesPayload, OpenAIResponsesSamplingState, error) {
	verbosity := options.TextVerbosity
	if verbosity == "" {
		verbosity = "low"
	}
	instructions := context.SystemPrompt
	if instructions == "" {
		instructions = "You are a helpful assistant."
	}
	requestState, err := ResolveOpenAIResponsesRequestState(
		model,
		context,
		OpenAIResponsesSamplingDefaults{
			SupportsStrictMode: true,
			Strict:             OpenAIResponsesStrictDefaultNull,
		},
	)
	if err != nil {
		return OpenAICodexResponsesPayload{}, OpenAIResponsesSamplingState{}, err
	}
	input, err := ConvertOpenAIResponsesMessagesChecked(model, context, ConvertOpenAIResponsesOptions{
		IncludeSystemPrompt:        ptrBool(false),
		GrammarToolInputProperties: requestState.Sampling.GrammarToolInputProperties,
		DeferredTools:              requestState.ToolPlacement.Deferred,
		ToolOptions:                requestState.Sampling.ToolOptions,
	})
	if err != nil {
		return OpenAICodexResponsesPayload{}, OpenAIResponsesSamplingState{}, err
	}
	payload := OpenAICodexResponsesPayload{
		Model:             model.ID,
		Store:             false,
		Stream:            true,
		Instructions:      instructions,
		Input:             input,
		Text:              map[string]string{"verbosity": verbosity},
		Include:           []string{"reasoning.encrypted_content"},
		PromptCacheKey:    options.SessionID,
		ToolChoice:        "auto",
		ParallelToolCalls: true,
	}
	if options.Temperature != nil {
		payload.Temperature = options.Temperature
	}
	if options.ServiceTier != "" {
		payload.ServiceTier = options.ServiceTier
	}
	if len(requestState.ToolPlacement.Immediate) > 0 {
		payload.Tools, err = ConvertOpenAIResponsesToolsChecked(
			requestState.ToolPlacement.Immediate,
			requestState.Sampling.ToolOptions,
		)
		if err != nil {
			return OpenAICodexResponsesPayload{}, OpenAIResponsesSamplingState{}, err
		}
	}
	if options.ReasoningEffort != "" {
		effort := mapOpenAICodexReasoningEffort(model, options.ReasoningEffort)
		if effort != "" {
			summary := options.ReasoningSummary
			if summary == "" {
				summary = "auto"
			}
			payload.Reasoning = map[string]string{"effort": effort, "summary": summary}
		}
	}
	return payload, requestState.Sampling, nil
}

func GetOpenAICodexWebSocketDebugStats(sessionID string) (OpenAICodexWebSocketDebugStats, bool) {
	openAICodexWebSocketState.Lock()
	defer openAICodexWebSocketState.Unlock()
	stats, ok := openAICodexWebSocketState.stats[sessionID]
	return stats, ok
}

func ResetOpenAICodexWebSocketDebugStats(sessionIDs ...string) {
	openAICodexWebSocketState.Lock()
	defer openAICodexWebSocketState.Unlock()
	if len(sessionIDs) > 0 && strings.TrimSpace(sessionIDs[0]) != "" {
		sessionID := strings.TrimSpace(sessionIDs[0])
		delete(openAICodexWebSocketState.stats, sessionID)
		delete(openAICodexWebSocketState.sseFallbacks, sessionID)
		return
	}
	openAICodexWebSocketState.stats = map[string]OpenAICodexWebSocketDebugStats{}
	openAICodexWebSocketState.sseFallbacks = map[string]bool{}
}

func isOpenAICodexWebSocketSSEFallbackActive(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	openAICodexWebSocketState.Lock()
	defer openAICodexWebSocketState.Unlock()
	return openAICodexWebSocketState.sseFallbacks[sessionID]
}

func recordOpenAICodexWebSocketSSEFallback(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	openAICodexWebSocketState.Lock()
	defer openAICodexWebSocketState.Unlock()
	stats := openAICodexWebSocketState.stats[sessionID]
	stats.SSEFallbacks++
	stats.WebSocketFallbackActive = openAICodexWebSocketState.sseFallbacks[sessionID]
	openAICodexWebSocketState.stats[sessionID] = stats
}

func recordOpenAICodexWebSocketFailure(sessionID string, err error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	openAICodexWebSocketState.Lock()
	defer openAICodexWebSocketState.Unlock()
	openAICodexWebSocketState.sseFallbacks[sessionID] = true
	stats := openAICodexWebSocketState.stats[sessionID]
	stats.WebSocketFailures++
	stats.WebSocketFallbackActive = true
	if err != nil {
		stats.LastWebSocketError = err.Error()
	}
	openAICodexWebSocketState.stats[sessionID] = stats
}

func BuildOpenAICodexSSEHeaders(modelHeaders, optionHeaders map[string]string, token, sessionID string) (map[string]string, error) {
	accountID, err := ExtractOpenAICodexAccountID(token)
	if err != nil {
		return nil, err
	}
	headers := buildOpenAICodexBaseHeaders(modelHeaders, optionHeaders, accountID, token)
	setHeaderCaseInsensitive(headers, "OpenAI-Beta", "responses=experimental")
	setHeaderCaseInsensitive(headers, "accept", "text/event-stream")
	setHeaderCaseInsensitive(headers, "content-type", "application/json")
	if sessionID != "" {
		setHeaderCaseInsensitive(headers, "session-id", sessionID)
		setHeaderCaseInsensitive(headers, "x-client-request-id", sessionID)
	}
	return headers, nil
}

func BuildOpenAICodexWebSocketHeaders(modelHeaders, optionHeaders map[string]string, token, requestID string) (map[string]string, error) {
	accountID, err := ExtractOpenAICodexAccountID(token)
	if err != nil {
		return nil, err
	}
	headers := buildOpenAICodexBaseHeaders(modelHeaders, optionHeaders, accountID, token)
	removeHeaderCaseInsensitive(headers, "accept")
	removeHeaderCaseInsensitive(headers, "content-type")
	setHeaderCaseInsensitive(headers, "OpenAI-Beta", openAICodexWebSocketBeta)
	setHeaderCaseInsensitive(headers, "x-client-request-id", requestID)
	setHeaderCaseInsensitive(headers, "session-id", requestID)
	return headers, nil
}

func ExtractOpenAICodexAccountID(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("failed to extract accountId from token")
	}
	payload, err := decodeJWTPayload(parts[1])
	if err != nil {
		return "", fmt.Errorf("failed to extract accountId from token")
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("failed to extract accountId from token")
	}
	auth, ok := claims[codexJWTClaimPath].(map[string]any)
	if !ok {
		return "", fmt.Errorf("failed to extract accountId from token")
	}
	accountID, ok := auth["chatgpt_account_id"].(string)
	if !ok || accountID == "" {
		return "", fmt.Errorf("failed to extract accountId from token")
	}
	return accountID, nil
}

func ResolveOpenAICodexURL(baseURL string) string {
	raw := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if raw == "" {
		raw = defaultCodexBaseURL
	}
	if strings.HasSuffix(raw, "/codex/responses") {
		return raw
	}
	if strings.HasSuffix(raw, "/codex") {
		return raw + "/responses"
	}
	return raw + "/codex/responses"
}

func ResolveOpenAICodexWebSocketURL(baseURL string) string {
	url := ResolveOpenAICodexURL(baseURL)
	switch {
	case strings.HasPrefix(url, "https://"):
		return "wss://" + strings.TrimPrefix(url, "https://")
	case strings.HasPrefix(url, "http://"):
		return "ws://" + strings.TrimPrefix(url, "http://")
	default:
		return url
	}
}

func ProcessOpenAICodexStreamEvents(model Model, output *Message, events []OpenAIResponsesStreamEvent, options OpenAICodexProcessOptions) []AssistantMessageEvent {
	processor := NewOpenAIResponsesStreamProcessorWithOptions(
		model,
		output,
		OpenAIResponsesStreamProcessorOptions{
			GrammarToolInputProperties: options.GrammarToolInputProperties,
		},
	)
	var emitted []AssistantMessageEvent
	for _, event := range events {
		normalized, ok := normalizeOpenAICodexEvent(event)
		if !ok {
			continue
		}
		events := processor.Process(normalized)
		if normalized.Type == "response.completed" && normalized.Response != nil {
			tier := ResolveOpenAICodexServiceTier(normalized.Response.ServiceTier, options.ServiceTier)
			ApplyOpenAICodexServiceTierPricing(&output.Usage, tier, model)
			for i := range events {
				events[i].Reason = output.StopReason
				events[i].Message = *output
				events[i].Partial = *output
			}
		}
		emitted = append(emitted, events...)
	}
	return emitted
}

func ResolveOpenAICodexServiceTier(responseServiceTier, requestServiceTier string) string {
	if responseServiceTier == "default" && (requestServiceTier == "flex" || requestServiceTier == "priority") {
		return requestServiceTier
	}
	if responseServiceTier != "" {
		return responseServiceTier
	}
	return requestServiceTier
}

func OpenAICodexServiceTierCostMultiplier(model Model, serviceTier string) float64 {
	switch serviceTier {
	case "flex":
		return 0.5
	case "priority":
		if model.ID == "gpt-5.5" {
			return 2.5
		}
		return 2
	default:
		return 1
	}
}

func ApplyOpenAICodexServiceTierPricing(usage *Usage, serviceTier string, model Model) {
	if usage == nil {
		return
	}
	multiplier := OpenAICodexServiceTierCostMultiplier(model, serviceTier)
	if multiplier == 1 {
		return
	}
	usage.Cost.Input *= multiplier
	usage.Cost.Output *= multiplier
	usage.Cost.CacheRead *= multiplier
	usage.Cost.CacheWrite *= multiplier
	usage.Cost.Total = usage.Cost.Input + usage.Cost.Output + usage.Cost.CacheRead + usage.Cost.CacheWrite
}

func OpenAICodexRetryDelay(status int, headers map[string]string, attempt int, now time.Time) time.Duration {
	if value, ok := lookupHeader(headers, "retry-after-ms"); ok {
		if millis, err := parseNonNegativeInt(value); err == nil {
			return time.Duration(millis) * time.Millisecond
		}
	}
	if value, ok := lookupHeader(headers, "retry-after"); ok {
		if seconds, err := parseNonNegativeInt(value); err == nil {
			return time.Duration(seconds) * time.Second
		}
		if date, err := time.Parse(time.RFC1123, value); err == nil {
			if date.Before(now) {
				return 0
			}
			return date.Sub(now)
		}
	}
	if attempt < 0 {
		attempt = 0
	}
	return time.Duration(1<<attempt) * time.Second
}

func IsOpenAICodexRetryable(status int, body string) bool {
	switch status {
	case 429, 500, 502, 503, 504:
		return true
	}
	lower := strings.ToLower(body)
	return strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "ratelimit") ||
		strings.Contains(lower, "overloaded") ||
		strings.Contains(lower, "service unavailable") ||
		strings.Contains(lower, "upstream connect") ||
		strings.Contains(lower, "connection refused")
}

func buildOpenAICodexBaseHeaders(modelHeaders, optionHeaders map[string]string, accountID, token string) map[string]string {
	headers := mergeHeadersCaseInsensitive(modelHeaders, optionHeaders)
	if headers == nil {
		headers = map[string]string{}
	}
	setHeaderCaseInsensitive(headers, "Authorization", "Bearer "+token)
	setHeaderCaseInsensitive(headers, "chatgpt-account-id", accountID)
	setHeaderCaseInsensitive(headers, "originator", "pi")
	setHeaderCaseInsensitive(headers, "User-Agent", "pi (go)")
	return headers
}

func decodeJWTPayload(value string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	return nil, fmt.Errorf("invalid jwt payload")
}

func mapOpenAICodexReasoningEffort(model Model, effort string) string {
	if effort == "none" {
		if mapped, ok := model.ThinkingLevelMap["off"]; ok && mapped != nil {
			return *mapped
		}
		return "none"
	}
	if effort == "off" {
		return ""
	}
	if mapped, ok := model.ThinkingLevelMap[effort]; ok && mapped != nil {
		return *mapped
	}
	return effort
}

func normalizeOpenAICodexEvent(event OpenAIResponsesStreamEvent) (OpenAIResponsesStreamEvent, bool) {
	switch event.Type {
	case "response.done", "response.completed", "response.incomplete":
		if event.Response == nil {
			event.Response = &OpenAIResponsesResponseEvent{}
		}
		if event.Type == "response.incomplete" && event.Response.Status == "" {
			event.Response.Status = "incomplete"
		}
		event.Type = "response.completed"
		return event, true
	case "response.failed":
		if event.Response == nil {
			event.Response = &OpenAIResponsesResponseEvent{Status: "failed"}
		}
		event.Type = "response.completed"
		return event, true
	case "error":
		return event, false
	default:
		return event, true
	}
}

func lookupHeader(headers map[string]string, name string) (string, bool) {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value, true
		}
	}
	return "", false
}

func parseNonNegativeInt(value string) (int, error) {
	result, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	if result < 0 {
		return 0, nil
	}
	return result, nil
}
