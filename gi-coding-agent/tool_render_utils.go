package gicodingagent

import (
	"net/url"
	"path/filepath"
	"strings"

	gitui "github.com/nowa/gi/gi-tui"
)

type toolPathRenderOptions struct {
	emptyFallback string
}

// linkPath keeps path resolution separate from terminal capability policy.
// Renderers pass styled display text through unchanged when OSC 8 links are
// unavailable.
func linkPath(styledText, rawPath, cwd string) string {
	if !gitui.GetCapabilities().Hyperlinks {
		return styledText
	}
	absolutePath := ResolveToCwd(rawPath, cwd)
	if resolved, err := filepath.Abs(absolutePath); err == nil {
		absolutePath = resolved
	}
	urlPath := filepath.ToSlash(absolutePath)
	if volume := filepath.VolumeName(absolutePath); volume != "" && !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	fileURL := (&url.URL{Scheme: "file", Path: urlPath}).String()
	return gitui.Hyperlink(styledText, fileURL)
}

// renderToolPath centralizes invalid, empty, themed, shortened, and linked path
// rendering so every built-in file tool presents the same state.
func renderToolPath(rawPath *string, cwd string, options ...toolPathRenderOptions) string {
	if rawPath == nil {
		return tuiThemeError("[invalid arg]")
	}
	value := *rawPath
	if value == "" && len(options) > 0 {
		value = options[0].emptyFallback
	}
	if value == "" {
		return tuiThemeToolOutput("...")
	}
	return linkPath(tuiThemeAccent(shortenDisplayPath(value)), value, cwd)
}

// toolPathArgument preserves the protocol's three string states: a missing or
// null value is empty, a string is valid (including ""), and any other value
// is invalid. Keys are checked in null-coalescing order.
func toolPathArgument(args any, typedPath string, keys ...string) *string {
	values, ok := args.(map[string]any)
	if !ok {
		return toolPathStringPointer(typedPath)
	}
	for _, key := range keys {
		value, exists := values[key]
		if !exists || value == nil {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return nil
		}
		return toolPathStringPointer(text)
	}
	return toolPathStringPointer("")
}

func toolPathStringPointer(value string) *string {
	return &value
}
