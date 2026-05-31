package share

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

func CreateSecretGist(ctx context.Context, htmlPath string) (string, error) {
	if strings.TrimSpace(htmlPath) == "" {
		return "", errors.New("share export path is required")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return "", errors.New("GitHub CLI (gh) is not installed. Install it from https://cli.github.com/")
	}
	if _, err := exec.CommandContext(ctx, "gh", "auth", "status").CombinedOutput(); err != nil {
		return "", errors.New("GitHub CLI is not logged in. Run 'gh auth login' first.")
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "gh", "gist", "create", "--public=false", htmlPath)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to create gist: %s", strings.TrimSpace(stderr.String()))
	}
	gistURL := strings.TrimSpace(stdout.String())
	if gistURL == "" {
		return "", errors.New("gh gist create returned an empty URL")
	}
	return gistURL, nil
}

func ViewerURL(gistID string) string {
	base := strings.TrimSpace(firstNonEmptyString(os.Getenv("GI_SHARE_VIEWER_URL"), os.Getenv("PI_SHARE_VIEWER_URL")))
	if base == "" {
		base = "https://gi.dev/session/"
	}
	return strings.TrimRight(base, "/") + "/#" + strings.TrimSpace(gistID)
}

func GistIDFromOutput(output string) (string, error) {
	value := strings.TrimSpace(output)
	if value == "" {
		return "", errors.New("share output does not contain a gist URL")
	}
	lines := strings.Split(value, "\n")
	value = strings.TrimSpace(lines[len(lines)-1])
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		for i := len(parts) - 1; i >= 0; i-- {
			if strings.TrimSpace(parts[i]) != "" {
				return strings.TrimSpace(parts[i]), nil
			}
		}
		return "", errors.New("share output URL does not contain a gist id")
	}
	value = strings.Trim(value, "/")
	if strings.Contains(value, "/") {
		parts := strings.Split(value, "/")
		value = strings.TrimSpace(parts[len(parts)-1])
	}
	if value == "" {
		return "", errors.New("share output does not contain a gist id")
	}
	return value, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
