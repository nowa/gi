package gillmprovider

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNormalizeProviderErrorPreservesHTTPMetadata(t *testing.T) {
	t.Parallel()

	providerErr := &ProviderError{
		StatusCode: http.StatusForbidden,
		Headers:    http.Header{"X-Request-Id": []string{"request-1"}},
		Body:       `{"error":"blocked by gateway WAF"}`,
		Err:        errors.New("403 status code (no body)"),
	}
	normalized := NormalizeProviderError(fmt.Errorf("request failed: %w", providerErr))

	if normalized.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", normalized.StatusCode)
	}
	if normalized.Body != providerErr.Body {
		t.Fatalf("body = %q, want %q", normalized.Body, providerErr.Body)
	}
	if normalized.MessageCarriesBody {
		t.Fatal("opaque message should not report that it carries the body")
	}
	if got := FormatProviderError(normalized); got != `HTTP 403: {"error":"blocked by gateway WAF"}` {
		t.Fatalf("formatted = %q", got)
	}
	if got := FormatProviderError(normalized, "OpenAI API error"); got != `OpenAI API error (HTTP 403): {"error":"blocked by gateway WAF"}` {
		t.Fatalf("formatted with prefix = %q", got)
	}
}

func TestNormalizeProviderErrorAvoidsDuplicateBody(t *testing.T) {
	t.Parallel()

	body := `{"error":{"message":"Permission denied"}}`
	normalized := NormalizeProviderError(&ProviderError{
		StatusCode: http.StatusForbidden,
		Body:       body,
		Err:        errors.New("request failed: " + body),
	})

	if !normalized.MessageCarriesBody {
		t.Fatal("message should report that it already carries the body")
	}
	if got := FormatProviderError(normalized, "Google API error"); got != "Google API error (HTTP 403): request failed: "+body {
		t.Fatalf("formatted = %q", got)
	}
}

func TestNormalizeProviderErrorLeavesOrdinaryErrorsAlone(t *testing.T) {
	t.Parallel()

	err := errors.New("plain failure")
	normalized := NormalizeProviderError(err)
	if normalized.StatusCode != 0 || normalized.Body != "" || normalized.MessageCarriesBody {
		t.Fatalf("normalized = %#v", normalized)
	}
	if got := FormatProviderError(normalized); got != err.Error() {
		t.Fatalf("formatted = %q, want %q", got, err)
	}
}

func TestTruncateProviderErrorTextUsesUnicodeCharacters(t *testing.T) {
	t.Parallel()

	input := strings.Repeat("界", MaxProviderErrorBodyChars+2)
	got := TruncateProviderErrorText(input, MaxProviderErrorBodyChars)
	if !utf8.ValidString(got) {
		t.Fatal("truncated provider error is not valid UTF-8")
	}
	if !strings.HasSuffix(got, "... [truncated 2 chars]") {
		t.Fatalf("truncated suffix = %q", got[len(got)-40:])
	}
}
