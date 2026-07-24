package gillmprovider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const maxOAuthErrorBodyBytes = 64 << 10

// OAuthResponseError preserves the HTTP status and OAuth error code so device
// flows can distinguish transient protocol states from terminal failures.
type OAuthResponseError struct {
	StatusCode  int
	Code        string
	Description string
	Operation   string
}

func (e *OAuthResponseError) Error() string {
	if e == nil {
		return ""
	}
	detail := e.Code
	if detail != "" && e.Description != "" {
		detail += ": " + e.Description
	} else if detail == "" {
		detail = e.Description
	}
	if detail == "" {
		detail = strconv.Itoa(e.StatusCode)
	}
	if e.Operation == "" {
		return detail
	}
	return e.Operation + ": " + detail
}

func readOAuthResponseError(
	response *http.Response,
	operation string,
) *OAuthResponseError {
	result := &OAuthResponseError{Operation: operation}
	if response == nil {
		result.Description = "empty HTTP response"
		return result
	}
	result.StatusCode = response.StatusCode
	if response.Body == nil {
		return result
	}
	body, err := io.ReadAll(io.LimitReader(
		response.Body,
		maxOAuthErrorBodyBytes+1,
	))
	if err != nil {
		result.Description = "could not read response body: " + err.Error()
		return result
	}
	if len(body) > maxOAuthErrorBodyBytes {
		body = body[:maxOAuthErrorBodyBytes]
	}

	var payload struct {
		Error            any `json:"error"`
		ErrorDescription any `json:"error_description"`
	}
	if len(body) > 0 && json.Unmarshal(body, &payload) == nil {
		result.Code, _ = payload.Error.(string)
		result.Description, _ = payload.ErrorDescription.(string)
	}
	if result.Code == "" && result.Description == "" {
		result.Description = truncateOAuthErrorText(string(body))
	}
	return result
}

func truncateOAuthErrorText(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= 512 {
		return string(runes)
	}
	return fmt.Sprintf("%s…", string(runes[:512]))
}
