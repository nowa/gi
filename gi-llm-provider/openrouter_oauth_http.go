package gillmprovider

import (
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

const maxOpenRouterOAuthBodyBytes = 1 << 20

type openRouterOAuthExchangeBody struct {
	Code                string `json:"code"`
	CodeVerifier        string `json:"code_verifier"`
	CodeChallengeMethod string `json:"code_challenge_method"`
}

type openRouterOAuthFailure struct {
	response *OAuthResponseError
}

func (e *openRouterOAuthFailure) Error() string {
	if e == nil || e.response == nil {
		return "OpenRouter OAuth key exchange failed"
	}
	message := fmt.Sprintf(
		"OpenRouter OAuth key exchange failed (HTTP %d)",
		e.response.StatusCode,
	)
	if detail := openRouterOAuthErrorDetail(e.response); detail != "" {
		message += ": " + detail
	}
	return message
}

func (e *openRouterOAuthFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.response
}

func exchangeOpenRouterAuthorizationCode(
	ctx context.Context,
	client HTTPDoer,
	code string,
	verifier string,
	timeout time.Duration,
) (Credential, error) {
	ctx = contextOrBackground(ctx)
	if contextError(ctx) != nil {
		return Credential{}, oauthLoginContextError(ctx)
	}
	if timeout <= 0 {
		timeout = defaultOpenRouterOAuthTokenTimeout
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload, err := json.Marshal(openRouterOAuthExchangeBody{
		Code:                code,
		CodeVerifier:        verifier,
		CodeChallengeMethod: "S256",
	})
	if err != nil {
		return Credential{}, err
	}
	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodPost,
		openRouterOAuthTokenURL,
		bytes.NewReader(payload),
	)
	if err != nil {
		return Credential{}, err
	}
	request.Header.Set("accept", "application/json")
	request.Header.Set("content-type", "application/json")
	response, err := httpClientOrDefault(client).Do(request)
	if err != nil {
		switch {
		case contextError(ctx) != nil:
			return Credential{}, oauthLoginContextError(ctx)
		case contextError(requestContext) != nil:
			return Credential{}, errors.New(
				"OpenRouter OAuth token exchange timed out",
			)
		default:
			return Credential{}, err
		}
	}
	if response == nil {
		return Credential{}, errors.New(
			"OpenRouter OAuth returned an empty HTTP response",
		)
	}
	ok := response.StatusCode >= http.StatusOK &&
		response.StatusCode < http.StatusMultipleChoices
	if response.Body == nil {
		if !ok {
			return Credential{}, newOpenRouterOAuthFailure(
				response.StatusCode,
				map[string]any{},
			)
		}
		return Credential{}, errors.New(
			"OpenRouter OAuth returned invalid JSON",
		)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(
		response.Body,
		maxOpenRouterOAuthBodyBytes+1,
	))
	if err != nil {
		switch {
		case contextError(ctx) != nil:
			return Credential{}, oauthLoginContextError(ctx)
		case contextError(requestContext) != nil:
			return Credential{}, errors.New(
				"OpenRouter OAuth token exchange timed out",
			)
		default:
			return Credential{}, err
		}
	}
	if len(body) > maxOpenRouterOAuthBodyBytes {
		return Credential{}, fmt.Errorf(
			"OpenRouter OAuth response exceeds %d bytes",
			maxOpenRouterOAuthBodyBytes,
		)
	}

	var object map[string]any
	parseErr := json.Unmarshal(body, &object)
	if parseErr != nil || object == nil {
		if ok {
			return Credential{}, errors.New(
				"OpenRouter OAuth returned invalid JSON",
			)
		}
		object = map[string]any{}
	}
	if !ok {
		return Credential{}, newOpenRouterOAuthFailure(
			response.StatusCode,
			object,
		)
	}
	key, _ := object["key"].(string)
	if key == "" {
		return Credential{}, errors.New(
			`OpenRouter OAuth response carries no "key"`,
		)
	}
	return Credential{
		Type:    CredentialTypeOAuth,
		Access:  key,
		Refresh: "",
		Expires: permanentOAuthCredentialExpires,
	}, nil
}

func newOpenRouterOAuthFailure(
	statusCode int,
	body map[string]any,
) error {
	code, _ := body["error"].(string)
	description := openRouterOAuthBodyErrorDetail(body)
	if description == code {
		description = ""
	}
	return &openRouterOAuthFailure{
		response: &OAuthResponseError{
			StatusCode:  statusCode,
			Code:        code,
			Description: description,
			Operation:   "OpenRouter OAuth key exchange",
		},
	}
}

func openRouterOAuthBodyErrorDetail(body map[string]any) string {
	for _, field := range []string{
		"error_description",
		"message",
		"error",
	} {
		if value, ok := body[field].(string); ok {
			return value
		}
	}
	if nested, ok := body["error"].(map[string]any); ok {
		if message, ok := nested["message"].(string); ok {
			return message
		}
	}
	return ""
}

func openRouterOAuthErrorDetail(response *OAuthResponseError) string {
	if response == nil {
		return ""
	}
	if strings.TrimSpace(response.Description) != "" {
		return response.Description
	}
	return response.Code
}
