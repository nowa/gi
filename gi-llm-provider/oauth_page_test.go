package gillmprovider

import (
	"strings"
	"testing"
)

func TestOAuthPageHTMLMatchesPiHelperContract(t *testing.T) {
	page := OAuthPageHTML(OAuthPageOptions{
		Title:       `Provider <ok>`,
		Heading:     `Provider "ok"`,
		Message:     `Close <now> & return`,
		Details:     `bad <script>alert("x")</script>`,
		ProductName: "Gi",
		HistoryPath: "/auth/callback",
	})

	for _, want := range []string{
		`<!doctype html>`,
		`<html lang="en">`,
		`>Gi<`,
		`Provider &lt;ok&gt;`,
		`Provider &#34;ok&#34;`,
		`Close &lt;now&gt; &amp; return`,
		`bad &lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;`,
		`history.replaceState(null,"","/auth/callback")`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("page missing %q:\n%s", want, page)
		}
	}
	for _, forbidden := range []string{
		`<script>alert("x")</script>`,
		`Close <now>`,
	} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("page contains unescaped %q:\n%s", forbidden, page)
		}
	}
}

func TestOAuthPagePiStyleSuccessAndErrorHelpers(t *testing.T) {
	success := OAuthSuccessHTML("You can close this window.")
	if !strings.Contains(success, "Authentication successful") || !strings.Contains(success, "You can close this window.") {
		t.Fatalf("success page = %s", success)
	}

	failed := OAuthErrorHTML("Try again.", "bad state")
	if !strings.Contains(failed, "Authentication failed") ||
		!strings.Contains(failed, "Try again.") ||
		!strings.Contains(failed, "bad state") {
		t.Fatalf("error page = %s", failed)
	}
}
