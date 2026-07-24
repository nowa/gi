package gillmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxKimiOAuthBodyBytes = 1 << 20

type kimiOAuthHTTPResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	JSON       map[string]any
}

func (r kimiOAuthHTTPResponse) OK() bool {
	return r.StatusCode >= http.StatusOK &&
		r.StatusCode < http.StatusMultipleChoices
}

func resolveKimiOAuthHost(
	ctx context.Context,
	configured string,
	authContext AuthContext,
) (string, error) {
	host := strings.TrimSpace(configured)
	if host == "" {
		if authContext == nil {
			authContext = DefaultProviderAuthContext()
		}
		for _, name := range []string{
			"KIMI_CODE_OAUTH_HOST",
			"KIMI_OAUTH_HOST",
		} {
			value, ok, err := authContext.Env(contextOrBackground(ctx), name)
			if err != nil {
				return "", fmt.Errorf(
					"resolve Kimi Code OAuth host from %s: %w",
					name,
					err,
				)
			}
			if ok && strings.TrimSpace(value) != "" {
				host = strings.TrimSpace(value)
				break
			}
		}
	}
	if host == "" {
		host = defaultKimiOAuthHost
	}
	host = strings.TrimRight(host, "/")
	parsed, err := url.Parse(host)
	if err != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		return "", fmt.Errorf("invalid Kimi Code OAuth host %q", host)
	}
	return parsed.String(), nil
}

func requestKimiDeviceAuthorization(
	ctx context.Context,
	config kimiOAuthConfig,
	host string,
) (kimiDeviceAuthorization, error) {
	response, err := postKimiOAuthForm(
		ctx,
		config,
		host+"/api/oauth/device_authorization",
		url.Values{"client_id": {kimiOAuthClientID}},
	)
	if err != nil {
		return kimiDeviceAuthorization{}, err
	}
	if !response.OK() {
		return kimiDeviceAuthorization{}, kimiOAuthStatusError(
			"Kimi Code device authorization failed with status",
			response,
		)
	}

	deviceCode, deviceOK := kimiOAuthString(
		response.JSON,
		"device_code",
	)
	userCode, userOK := kimiOAuthString(response.JSON, "user_code")
	verificationURI, verificationOK := trustedKimiHTTPURL(
		response.JSON["verification_uri"],
	)
	verificationURIComplete, completeOK := trustedKimiHTTPURL(
		response.JSON["verification_uri_complete"],
	)
	if !deviceOK || !userOK || !verificationOK || !completeOK {
		return kimiDeviceAuthorization{}, fmt.Errorf(
			"Invalid Kimi Code device authorization response: %s",
			kimiOAuthJSONText(response),
		)
	}
	return kimiDeviceAuthorization{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         verificationURI,
		VerificationURIComplete: verificationURIComplete,
		IntervalSeconds: kimiPositiveSeconds(
			response.JSON["interval"],
			defaultKimiPollIntervalSeconds,
		),
		ExpiresInSeconds: kimiPositiveSeconds(
			response.JSON["expires_in"],
			defaultKimiDeviceTimeoutSeconds,
		),
	}, nil
}

func parseKimiTokenResponse(
	body map[string]any,
	operation string,
	now func() time.Time,
) (kimiToken, error) {
	access, accessOK := kimiOAuthString(body, "access_token")
	refresh, refreshOK := kimiOAuthString(body, "refresh_token")
	expiresIn, expiresOK := body["expires_in"].(float64)
	if !accessOK ||
		!refreshOK ||
		!expiresOK ||
		math.IsNaN(expiresIn) ||
		math.IsInf(expiresIn, 0) ||
		expiresIn <= 0 {
		return kimiToken{}, fmt.Errorf(
			"Kimi Code token %s response missing fields: %s",
			operation,
			kimiJSONMapText(body),
		)
	}
	if now == nil {
		now = time.Now
	}
	expires, err := oauthExpiryMillis(now(), expiresIn, 0)
	if err != nil {
		return kimiToken{}, fmt.Errorf(
			"Kimi Code token %s response missing fields: %s",
			operation,
			kimiJSONMapText(body),
		)
	}
	return kimiToken{
		Access:  access,
		Refresh: refresh,
		Expires: expires,
	}, nil
}

func postKimiOAuthForm(
	ctx context.Context,
	config kimiOAuthConfig,
	endpoint string,
	values url.Values,
) (kimiOAuthHTTPResponse, error) {
	ctx = contextOrBackground(ctx)
	requestContext, cancel := context.WithTimeout(
		ctx,
		config.requestTimeout,
	)
	defer cancel()

	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodPost,
		endpoint,
		strings.NewReader(values.Encode()),
	)
	if err != nil {
		return kimiOAuthHTTPResponse{}, err
	}
	request.Header.Set(
		"Content-Type",
		"application/x-www-form-urlencoded",
	)
	request.Header.Set("Accept", "application/json")
	response, err := config.client.Do(request)
	if err != nil {
		if contextError(ctx) != nil {
			return kimiOAuthHTTPResponse{}, contextError(ctx)
		}
		if contextError(requestContext) != nil {
			return kimiOAuthHTTPResponse{}, fmt.Errorf(
				"Kimi Code OAuth request timed out: %w",
				contextError(requestContext),
			)
		}
		return kimiOAuthHTTPResponse{}, err
	}
	if response == nil {
		return kimiOAuthHTTPResponse{}, errors.New(
			"Kimi Code OAuth returned an empty HTTP response",
		)
	}
	if response.Body == nil {
		return kimiOAuthHTTPResponse{
			StatusCode: response.StatusCode,
			Header:     response.Header.Clone(),
		}, nil
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(
		response.Body,
		maxKimiOAuthBodyBytes+1,
	))
	if err != nil {
		if contextError(ctx) != nil {
			return kimiOAuthHTTPResponse{}, contextError(ctx)
		}
		return kimiOAuthHTTPResponse{}, err
	}
	if len(body) > maxKimiOAuthBodyBytes {
		return kimiOAuthHTTPResponse{}, fmt.Errorf(
			"Kimi Code OAuth response exceeds %d bytes",
			maxKimiOAuthBodyBytes,
		)
	}

	var object map[string]any
	if len(body) > 0 {
		_ = json.Unmarshal(body, &object)
	}
	return kimiOAuthHTTPResponse{
		StatusCode: response.StatusCode,
		Header:     response.Header.Clone(),
		Body:       body,
		JSON:       object,
	}, nil
}

func trustedKimiHTTPURL(value any) (string, bool) {
	raw, ok := value.(string)
	if !ok || raw == "" {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" {
		return "", false
	}
	return parsed.String(), true
}

func kimiOAuthString(
	body map[string]any,
	field string,
) (string, bool) {
	value, ok := body[field].(string)
	return value, ok && value != ""
}

func kimiPositiveSeconds(value any, fallback int) int {
	number, ok := value.(float64)
	if !ok ||
		math.IsNaN(number) ||
		math.IsInf(number, 0) ||
		number <= 0 {
		return fallback
	}
	seconds := math.Ceil(number)
	maxInt := int(^uint(0) >> 1)
	if seconds >= float64(maxInt) {
		return fallback
	}
	return int(seconds)
}

func kimiOAuthStatusError(
	prefix string,
	response kimiOAuthHTTPResponse,
) error {
	message := fmt.Sprintf("%s %d", prefix, response.StatusCode)
	body := strings.TrimSpace(string(response.Body))
	if body != "" {
		message += ": " + body
	}
	return errors.New(message)
}

func kimiOAuthJSONText(response kimiOAuthHTTPResponse) string {
	return kimiJSONMapText(response.JSON)
}

func kimiJSONMapText(body map[string]any) string {
	if body == nil {
		return "null"
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "null"
	}
	return string(encoded)
}
