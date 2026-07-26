package gillmprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxRadiusOAuthBodyBytes = 1 << 20
	radiusOAuthExpirySkew   = time.Minute
)

type radiusOAuthConfig struct {
	AuthorizationEndpoint       string
	TokenEndpoint               string
	DeviceAuthorizationEndpoint string
	ClientID                    string
	Scope                       string
	DeviceCodeGrantType         string
}

type radiusDeviceAuthorization struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

func loadRadiusOAuthDiscovery(
	ctx context.Context,
	client HTTPDoer,
	gateway string,
) (string, error) {
	endpoint, err := radiusOAuthEndpoint(gateway)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(
		contextOrBackground(ctx),
		http.MethodGet,
		endpoint.String(),
		nil,
	)
	if err != nil {
		return "", err
	}
	request.Header.Set("accept", "application/json")
	response, err := httpClientOrDefault(client).Do(request)
	if err != nil {
		if contextError(ctx) != nil {
			return "", oauthLoginContextError(ctx)
		}
		return "", fmt.Errorf(
			"could not load Radius OAuth config from %s: %w",
			gateway,
			err,
		)
	}
	if response.Body == nil {
		return "", fmt.Errorf(
			"could not load Radius OAuth config from %s: response has no body",
			gateway,
		)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(io.LimitReader(
			response.Body,
			maxOAuthErrorBodyBytes,
		))
		if readErr != nil {
			return "", fmt.Errorf(
				"could not load Radius OAuth config from %s: %d: %w",
				gateway,
				response.StatusCode,
				readErr,
			)
		}
		message := fmt.Sprintf(
			"could not load Radius OAuth config from %s: %d",
			gateway,
			response.StatusCode,
		)
		if detail := truncateOAuthErrorText(string(body)); detail != "" {
			message += " " + detail
		}
		return "", errors.New(message)
	}

	body, err := readBoundedRadiusOAuthBody(response.Body)
	if err != nil {
		return "", fmt.Errorf(
			"could not load Radius OAuth config from %s: %w",
			gateway,
			err,
		)
	}
	var discovery struct {
		AuthorizationEndpoint string `json:"authorizationEndpoint"`
	}
	if err := json.Unmarshal(body, &discovery); err != nil {
		return "", fmt.Errorf(
			"could not load Radius OAuth config from %s: %w",
			gateway,
			err,
		)
	}
	authorizationEndpoint := strings.TrimSpace(
		discovery.AuthorizationEndpoint,
	)
	if _, err := parseRadiusOAuthHTTPURL(
		authorizationEndpoint,
		"authorizationEndpoint",
	); err != nil {
		return "", fmt.Errorf(
			"invalid Radius OAuth config from %s: %w",
			gateway,
			err,
		)
	}
	return authorizationEndpoint, nil
}

func requestRadiusDeviceAuthorization(
	ctx context.Context,
	client HTTPDoer,
	config radiusOAuthConfig,
) (radiusDeviceAuthorization, error) {
	endpoint, err := parseRadiusOAuthHTTPURL(
		config.DeviceAuthorizationEndpoint,
		"deviceAuthorizationEndpoint",
	)
	if err != nil {
		return radiusDeviceAuthorization{}, err
	}
	request, err := http.NewRequestWithContext(
		contextOrBackground(ctx),
		http.MethodPost,
		endpoint.String(),
		strings.NewReader(url.Values{
			"client_id": {config.ClientID},
			"scope":     {config.Scope},
		}.Encode()),
	)
	if err != nil {
		return radiusDeviceAuthorization{}, err
	}
	request.Header.Set("accept", "application/json")
	request.Header.Set(
		"content-type",
		"application/x-www-form-urlencoded",
	)
	response, err := httpClientOrDefault(client).Do(request)
	if err != nil {
		if contextError(ctx) != nil {
			return radiusDeviceAuthorization{}, oauthLoginContextError(ctx)
		}
		return radiusDeviceAuthorization{}, fmt.Errorf(
			"Radius OAuth device authorization failed: %w",
			err,
		)
	}
	if response.Body == nil {
		return radiusDeviceAuthorization{}, errors.New(
			"Radius OAuth device authorization failed: response has no body",
		)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		return radiusDeviceAuthorization{}, readOAuthResponseError(
			response,
			"Radius OAuth device authorization failed",
		)
	}
	body, err := readBoundedRadiusOAuthBody(response.Body)
	if err != nil {
		return radiusDeviceAuthorization{}, fmt.Errorf(
			"Radius OAuth device authorization failed: %w",
			err,
		)
	}
	var device radiusDeviceAuthorization
	if err := json.Unmarshal(body, &device); err != nil {
		return radiusDeviceAuthorization{}, fmt.Errorf(
			"Radius OAuth device authorization failed: %w",
			err,
		)
	}
	if strings.TrimSpace(device.DeviceCode) == "" ||
		strings.TrimSpace(device.UserCode) == "" ||
		strings.TrimSpace(device.VerificationURI) == "" ||
		device.ExpiresIn <= 0 {
		return radiusDeviceAuthorization{}, errors.New(
			"Radius OAuth device authorization response is missing required fields",
		)
	}
	return device, nil
}

func requestRadiusOAuthToken(
	ctx context.Context,
	client HTTPDoer,
	config radiusOAuthConfig,
	values url.Values,
	now func() time.Time,
) (Credential, error) {
	endpoint, err := parseRadiusOAuthHTTPURL(
		config.TokenEndpoint,
		"tokenEndpoint",
	)
	if err != nil {
		return Credential{}, err
	}
	request, err := http.NewRequestWithContext(
		contextOrBackground(ctx),
		http.MethodPost,
		endpoint.String(),
		strings.NewReader(values.Encode()),
	)
	if err != nil {
		return Credential{}, err
	}
	request.Header.Set("accept", "application/json")
	request.Header.Set(
		"content-type",
		"application/x-www-form-urlencoded",
	)
	response, err := httpClientOrDefault(client).Do(request)
	if err != nil {
		if contextError(ctx) != nil {
			return Credential{}, oauthLoginContextError(ctx)
		}
		return Credential{}, fmt.Errorf(
			"Radius OAuth token request failed: %w",
			err,
		)
	}
	if response.Body == nil {
		return Credential{}, errors.New(
			"Radius OAuth token request failed: response has no body",
		)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		return Credential{}, readOAuthResponseError(
			response,
			"Radius OAuth token request failed",
		)
	}
	body, err := readBoundedRadiusOAuthBody(response.Body)
	if err != nil {
		return Credential{}, fmt.Errorf(
			"Radius OAuth token request failed: %w",
			err,
		)
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return Credential{}, fmt.Errorf(
			"Radius OAuth token request failed: %w",
			err,
		)
	}
	if strings.TrimSpace(token.AccessToken) == "" || token.ExpiresIn <= 0 {
		return Credential{}, errors.New(
			"Radius OAuth token response is missing required fields",
		)
	}
	if now == nil {
		now = time.Now
	}
	expires, err := radiusOAuthExpiryMillis(now(), token.ExpiresIn)
	if err != nil {
		return Credential{}, err
	}
	credential := Credential{
		Type:    CredentialTypeOAuth,
		Access:  token.AccessToken,
		Refresh: token.RefreshToken,
		Expires: expires,
	}
	if token.Scope != "" {
		credential.Metadata = map[string]any{"scope": token.Scope}
	}
	return credential, nil
}

func radiusOAuthExpiryMillis(
	now time.Time,
	expiresInSeconds int64,
) (int64, error) {
	const (
		maxInt64        = int64(^uint64(0) >> 1)
		minInt64        = -maxInt64 - 1
		millisPerSecond = int64(time.Second / time.Millisecond)
	)
	if expiresInSeconds <= 0 ||
		expiresInSeconds > maxInt64/millisPerSecond {
		return 0, errors.New("Radius OAuth token expiry is out of range")
	}
	delta := expiresInSeconds*millisPerSecond -
		radiusOAuthExpirySkew.Milliseconds()
	nowMillis := now.UnixMilli()
	if (delta > 0 && nowMillis > maxInt64-delta) ||
		(delta < 0 && nowMillis < minInt64-delta) {
		return 0, errors.New("Radius OAuth token expiry is out of range")
	}
	return nowMillis + delta, nil
}

func readBoundedRadiusOAuthBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(
		reader,
		maxRadiusOAuthBodyBytes+1,
	))
	if err != nil {
		return nil, err
	}
	if len(body) > maxRadiusOAuthBodyBytes {
		return nil, fmt.Errorf(
			"response exceeds %d bytes",
			maxRadiusOAuthBodyBytes,
		)
	}
	return body, nil
}

func parseRadiusOAuthHTTPURL(
	value string,
	field string,
) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" {
		return nil, fmt.Errorf(
			"Radius OAuth config has invalid %s %q",
			field,
			value,
		)
	}
	return parsed, nil
}

func radiusOAuthEndpoint(gateway string) (*url.URL, error) {
	return radiusOAuthGatewayEndpoint(gateway, "/v1/oauth")
}

func radiusOAuthGatewayEndpoint(
	gateway string,
	path string,
) (*url.URL, error) {
	base, err := url.Parse(gateway)
	if err != nil {
		return nil, fmt.Errorf("parse Radius gateway %q: %w", gateway, err)
	}
	if (base.Scheme != "http" && base.Scheme != "https") ||
		base.Host == "" {
		return nil, fmt.Errorf("invalid Radius gateway URL %q", gateway)
	}
	return base.ResolveReference(&url.URL{Path: path}), nil
}
