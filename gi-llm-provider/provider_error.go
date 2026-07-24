package gillmprovider

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

// MaxProviderErrorBodyChars bounds provider response bodies exposed to callers.
const MaxProviderErrorBodyChars = 4000

// ProviderError is the package-wide transport error contract. Provider
// adapters should preserve HTTP metadata here instead of flattening it into a
// string, so retry policy and user-facing formatting consume the same data.
type ProviderError struct {
	StatusCode int
	Headers    http.Header
	Body       string
	Err        error
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	if e.StatusCode > 0 {
		return http.StatusText(e.StatusCode)
	}
	return "provider request failed"
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NormalizedProviderError is the stable representation used by provider
// adapters when rendering failures.
type NormalizedProviderError struct {
	StatusCode         int
	Body               string
	Message            string
	MessageCarriesBody bool
}

// NormalizeProviderError extracts the typed provider metadata while preserving
// ordinary Go error wrapping.
func NormalizeProviderError(err error) NormalizedProviderError {
	if err == nil {
		return NormalizedProviderError{}
	}

	normalized := NormalizedProviderError{Message: err.Error()}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return normalized
	}

	normalized.StatusCode = providerErr.StatusCode
	normalized.Body = TruncateProviderErrorText(strings.TrimSpace(providerErr.Body), MaxProviderErrorBodyChars)
	normalized.MessageCarriesBody = normalized.Body == "" || strings.Contains(normalized.Message, normalized.Body)
	return normalized
}

// FormatProviderError composes a provider error without dropping a response
// body that was retained separately from the Go error message.
func FormatProviderError(normalized NormalizedProviderError, prefix ...string) string {
	label := ""
	if len(prefix) > 0 {
		label = strings.TrimSpace(prefix[0])
	}
	if normalized.MessageCarriesBody || normalized.StatusCode == 0 || normalized.Body == "" {
		if label != "" && normalized.StatusCode != 0 {
			return fmt.Sprintf("%s (HTTP %d): %s", label, normalized.StatusCode, normalized.Message)
		}
		return normalized.Message
	}
	if label != "" {
		return fmt.Sprintf("%s (HTTP %d): %s", label, normalized.StatusCode, normalized.Body)
	}
	return fmt.Sprintf("HTTP %d: %s", normalized.StatusCode, normalized.Body)
}

// TruncateProviderErrorText truncates by Unicode code point so the result
// remains valid UTF-8 even when a provider body contains non-ASCII text.
func TruncateProviderErrorText(text string, maxChars int) string {
	if maxChars < 0 {
		maxChars = 0
	}
	if utf8.RuneCountInString(text) <= maxChars {
		return text
	}
	runes := []rune(text)
	remaining := len(runes) - maxChars
	return fmt.Sprintf("%s... [truncated %d chars]", string(runes[:maxChars]), remaining)
}

func newProviderTransportError(err error) *ProviderError {
	if err == nil {
		err = errors.New("provider request failed")
	}
	return &ProviderError{Err: err}
}

func newProviderHTTPError(statusCode int, headers http.Header, body string, err error) *ProviderError {
	if headers != nil {
		headers = headers.Clone()
	}
	if err == nil {
		status := http.StatusText(statusCode)
		if status == "" {
			status = "provider request failed"
		}
		err = errors.New(status)
	}
	return &ProviderError{
		StatusCode: statusCode,
		Headers:    headers,
		Body:       TruncateProviderErrorText(strings.TrimSpace(body), MaxProviderErrorBodyChars),
		Err:        err,
	}
}
