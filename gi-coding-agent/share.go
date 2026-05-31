package gicodingagent

import (
	"context"

	share "github.com/nowa/gi/gi-coding-agent/internal/share"
)

func defaultCreateSecretGist(ctx context.Context, htmlPath string) (string, error) {
	return share.CreateSecretGist(ctx, htmlPath)
}

func shareViewerURL(gistID string) string {
	return share.ViewerURL(gistID)
}

func gistIDFromShareOutput(output string) (string, error) {
	return share.GistIDFromOutput(output)
}
