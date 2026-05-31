package oauthflow

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func OrderedURL(base string, params [][2]string) string {
	query := make([]byte, 0, len(params)*16)
	for index, param := range params {
		if index > 0 {
			query = append(query, '&')
		}
		query = append(query, url.QueryEscape(param[0])...)
		query = append(query, '=')
		query = append(query, url.QueryEscape(param[1])...)
	}
	return base + "?" + string(query)
}

func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func RandomToken(size int) string {
	if size <= 0 {
		size = 32
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "gi-oauth-token"
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func ParseAuthorizationInput(input string) (code string, state string) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return parsed.Query().Get("code"), parsed.Query().Get("state")
	}
	if strings.Contains(value, "#") {
		code, state, _ := strings.Cut(value, "#")
		return code, state
	}
	if strings.Contains(value, "code=") {
		params, err := url.ParseQuery(value)
		if err == nil {
			return params.Get("code"), params.Get("state")
		}
	}
	return value, ""
}

func CallbackHosts() []string {
	if host := strings.TrimSpace(os.Getenv("GI_OAUTH_CALLBACK_HOST")); host != "" {
		return []string{host}
	}
	if host := strings.TrimSpace(os.Getenv("PI_OAUTH_CALLBACK_HOST")); host != "" {
		return []string{host}
	}
	return []string{"127.0.0.1", "::1"}
}

func OpenBrowser(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}
