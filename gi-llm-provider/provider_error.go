package gillmprovider

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf16"
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

	bodyNormalized bool
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
	if errors.As(err, &providerErr) && providerErr != nil {
		normalized.StatusCode = providerErr.StatusCode
		if providerErr.bodyNormalized {
			normalized.Body = providerErr.Body
		} else {
			normalized.Body = TruncateProviderErrorText(
				strings.TrimFunc(providerErr.Body, isProviderErrorTrimSpace),
				MaxProviderErrorBodyChars,
			)
		}
		normalized.MessageCarriesBody = normalized.Body == "" ||
			strings.Contains(normalized.Message, normalized.Body)
		if normalized.StatusCode == 0 {
			normalized.StatusCode = providerErrorHTTPStatusCode(err)
		}
		return normalized
	}

	// Smithy and other SDK errors expose status through a method while retaining
	// an unread response stream internally. Read only the status interface:
	// probing or formatting the response body could consume a live stream or
	// leak implementation details into the user-facing error.
	normalized.StatusCode = providerErrorHTTPStatusCode(err)
	if normalized.StatusCode != 0 {
		normalized.MessageCarriesBody = true
	}
	return normalized
}

func providerErrorHTTPStatusCode(err error) int {
	var statusError interface {
		HTTPStatusCode() int
	}
	if errors.As(err, &statusError) && statusError != nil {
		return statusError.HTTPStatusCode()
	}
	return 0
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
			return fmt.Sprintf("%s (%d): %s", label, normalized.StatusCode, normalized.Message)
		}
		return normalized.Message
	}
	if label != "" {
		return fmt.Sprintf("%s (%d): %s", label, normalized.StatusCode, normalized.Body)
	}
	return fmt.Sprintf("%d: %s", normalized.StatusCode, normalized.Body)
}

// TruncateProviderErrorText uses UTF-16 code units because Pi's JavaScript
// String.length and String.slice do. utf16.Decode replaces a split surrogate
// with U+FFFD, keeping the Go result valid UTF-8 at the only boundary where the
// two string models cannot represent exactly the same intermediate value.
func TruncateProviderErrorText(text string, maxChars int) string {
	if maxChars < 0 {
		maxChars = 0
	}
	codeUnits := utf16.Encode([]rune(text))
	if len(codeUnits) <= maxChars {
		return text
	}
	remaining := len(codeUnits) - maxChars
	return fmt.Sprintf("%s... [truncated %d chars]", string(utf16.Decode(codeUnits[:maxChars])), remaining)
}

// readProviderErrorBody applies trim and truncation while reading. It consumes
// the complete body so the suffix reports the exact number of omitted UTF-16
// units, but retains at most MaxProviderErrorBodyChars units in memory.
func readProviderErrorBody(reader io.Reader) (string, error) {
	if reader == nil {
		return "", nil
	}

	input := bufio.NewReader(reader)
	prefix := make([]uint16, 0, MaxProviderErrorBodyChars)
	pendingWhitespace := make([]uint16, 0, MaxProviderErrorBodyChars)
	totalUnits := 0
	pendingUnits := 0
	started := false
	var readErr error

	flushPending := func() {
		if pendingUnits == 0 {
			return
		}
		totalUnits += pendingUnits
		prefix = appendUTF16Prefix(prefix, pendingWhitespace, MaxProviderErrorBodyChars)
		pendingWhitespace = pendingWhitespace[:0]
		pendingUnits = 0
	}

	for {
		value, _, err := input.ReadRune()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				readErr = err
			}
			break
		}
		units, unitCount := utf16UnitsForRune(value)
		if isProviderErrorTrimSpace(value) {
			if !started {
				continue
			}
			pendingUnits += unitCount
			pendingWhitespace = appendUTF16Prefix(
				pendingWhitespace,
				units[:unitCount],
				MaxProviderErrorBodyChars-totalUnits,
			)
			continue
		}

		started = true
		flushPending()
		totalUnits += unitCount
		prefix = appendUTF16Prefix(prefix, units[:unitCount], MaxProviderErrorBodyChars)
	}

	// pendingWhitespace is trailing whitespace and intentionally excluded,
	// matching JavaScript String.trim().
	if totalUnits == 0 {
		return "", readErr
	}
	body := string(utf16.Decode(prefix))
	if totalUnits > MaxProviderErrorBodyChars {
		body = fmt.Sprintf(
			"%s... [truncated %d chars]",
			body,
			totalUnits-MaxProviderErrorBodyChars,
		)
	}
	return body, readErr
}

func utf16UnitsForRune(value rune) ([2]uint16, int) {
	if value > 0xffff {
		high, low := utf16.EncodeRune(value)
		return [2]uint16{uint16(high), uint16(low)}, 2
	}
	return [2]uint16{uint16(value)}, 1
}

func appendUTF16Prefix(target, value []uint16, limit int) []uint16 {
	remaining := limit - len(target)
	if remaining <= 0 {
		return target
	}
	if len(value) > remaining {
		value = value[:remaining]
	}
	return append(target, value...)
}

func isProviderErrorTrimSpace(value rune) bool {
	// ECMAScript trim includes BOM in addition to Unicode White_Space.
	return value == '\uFEFF' || unicode.IsSpace(value)
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
		Body:       body,
		Err:        err,
	}
}

func newNormalizedProviderHTTPError(statusCode int, headers http.Header, body string, err error) *ProviderError {
	providerErr := newProviderHTTPError(statusCode, headers, body, err)
	providerErr.bodyNormalized = true
	return providerErr
}
