package oauthflow

import (
	"strings"
	"testing"
)

func TestOrderedURLPreservesOrderAndEscapesValues(t *testing.T) {
	got := OrderedURL("https://example.test/auth", [][2]string{
		{"response_type", "code"},
		{"scope", "openid profile"},
		{"redirect_uri", "http://localhost:1455/auth/callback"},
	})
	want := "https://example.test/auth?response_type=code&scope=openid+profile&redirect_uri=http%3A%2F%2Flocalhost%3A1455%2Fauth%2Fcallback"
	if got != want {
		t.Fatalf("OrderedURL() = %q, want %q", got, want)
	}
}

func TestPKCEChallengeMatchesRFCExample(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const want = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := PKCEChallenge(verifier); got != want {
		t.Fatalf("PKCEChallenge() = %q, want %q", got, want)
	}
}

func TestParseAuthorizationInput(t *testing.T) {
	for _, tc := range []struct {
		name      string
		input     string
		wantCode  string
		wantState string
	}{
		{name: "redirect URL", input: "http://localhost:1455/auth/callback?code=abc&state=def", wantCode: "abc", wantState: "def"},
		{name: "query string", input: "code=abc&state=def", wantCode: "abc", wantState: "def"},
		{name: "manual code state", input: "abc#def", wantCode: "abc", wantState: "def"},
		{name: "manual code", input: "abc", wantCode: "abc"},
		{name: "blank", input: "  "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, state := ParseAuthorizationInput(tc.input)
			if code != tc.wantCode || state != tc.wantState {
				t.Fatalf("ParseAuthorizationInput(%q) = %q, %q; want %q, %q", tc.input, code, state, tc.wantCode, tc.wantState)
			}
		})
	}
}

func TestCallbackHostsPrefersGiThenLegacyPi(t *testing.T) {
	t.Setenv("GI_OAUTH_CALLBACK_HOST", "localhost")
	t.Setenv("PI_OAUTH_CALLBACK_HOST", "legacy.localhost")
	if got := strings.Join(CallbackHosts(), ","); got != "localhost" {
		t.Fatalf("CallbackHosts() = %q, want GI host", got)
	}

	t.Setenv("GI_OAUTH_CALLBACK_HOST", "")
	if got := strings.Join(CallbackHosts(), ","); got != "legacy.localhost" {
		t.Fatalf("CallbackHosts() = %q, want legacy PI host", got)
	}
}
