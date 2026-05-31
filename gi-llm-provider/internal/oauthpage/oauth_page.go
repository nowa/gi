package oauthpage

import (
	"encoding/json"
	"html"
	"strings"
)

const oauthPageLogoSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 800" aria-hidden="true"><path fill="#fff" fill-rule="evenodd" d="M165.29 165.29 H517.36 V400 H400 V517.36 H282.65 V634.72 H165.29 Z M282.65 282.65 V400 H400 V282.65 Z"/><path fill="#fff" d="M517.36 400 H634.72 V634.72 H517.36 Z"/></svg>`

// OAuthPageOptions describes a local OAuth callback result page.
type OAuthPageOptions struct {
	Title       string
	Heading     string
	Message     string
	Details     string
	ProductName string
	HistoryPath string
}

// OAuthPageHTML renders the shared OAuth callback page used by provider flows.
func OAuthPageHTML(options OAuthPageOptions) string {
	title := firstNonEmptyOAuthPageString(options.Title, "Authentication")
	heading := firstNonEmptyOAuthPageString(options.Heading, title)
	message := firstNonEmptyOAuthPageString(options.Message, "You can close this window.")
	product := firstNonEmptyOAuthPageString(options.ProductName, "Gi")
	details := strings.TrimSpace(options.Details)

	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>` + html.EscapeString(title) + `</title>
  <style>
    :root {
      --text: #fafafa;
      --text-dim: #a1a1aa;
      --page-bg: #09090b;
      --accent: #7dd3fc;
      --font-sans: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Noto Sans", sans-serif, "Apple Color Emoji", "Segoe UI Emoji";
      --font-mono: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
    }
    * { box-sizing: border-box; }
    html { color-scheme: dark; }
    body {
      margin: 0;
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 24px;
      background: var(--page-bg);
      color: var(--text);
      font-family: var(--font-sans);
      text-align: center;
    }
    main {
      width: 100%;
      max-width: 560px;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
    }
    .brand {
      color: var(--accent);
      font-size: 14px;
      font-weight: 700;
      letter-spacing: .08em;
      margin-bottom: 16px;
    }
    .logo {
      width: 72px;
      height: 72px;
      display: block;
      margin-bottom: 24px;
    }
    h1 {
      margin: 0 0 10px;
      font-size: 28px;
      line-height: 1.15;
      font-weight: 650;
      color: var(--text);
    }
    p {
      margin: 0;
      line-height: 1.7;
      color: var(--text-dim);
      font-size: 15px;
    }
    .details {
      margin-top: 16px;
      font-family: var(--font-mono);
      font-size: 13px;
      color: var(--text-dim);
      white-space: pre-wrap;
      word-break: break-word;
    }
  </style>
</head>
<body>
  <main>
    <div class="brand">` + html.EscapeString(product) + `</div>
    <div class="logo">` + oauthPageLogoSVG + `</div>
    <h1>` + html.EscapeString(heading) + `</h1>
    <p>` + html.EscapeString(message) + `</p>
    ` + oauthPageDetailsHTML(details) + `
  </main>` + oauthPageHistoryScript(options.HistoryPath) + `
</body>
</html>`
}

// OAuthSuccessHTML mirrors Pi's success helper with Gi's branded page shell.
func OAuthSuccessHTML(message string) string {
	return OAuthPageHTML(OAuthPageOptions{
		Title:   "Authentication successful",
		Heading: "Authentication successful",
		Message: message,
	})
}

// OAuthErrorHTML mirrors Pi's error helper with Gi's branded page shell.
func OAuthErrorHTML(message, details string) string {
	return OAuthPageHTML(OAuthPageOptions{
		Title:   "Authentication failed",
		Heading: "Authentication failed",
		Message: message,
		Details: details,
	})
}

func oauthPageDetailsHTML(details string) string {
	if details == "" {
		return ""
	}
	return `<div class="details">` + html.EscapeString(details) + `</div>`
}

func oauthPageHistoryScript(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return ""
	}
	encoded, err := json.Marshal(path)
	if err != nil {
		return ""
	}
	return `<script>try{history.replaceState(null,"",` + string(encoded) + `)}catch(e){}</script>`
}

func firstNonEmptyOAuthPageString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
