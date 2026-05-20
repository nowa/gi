package gicodingagent

import (
	"encoding/base64"
	"os"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type ProcessFileArgumentsOptions struct {
	AutoResizeImages bool
}

type ProcessedFileArguments struct {
	Text   string
	Images []llm.ContentPart
}

func ProcessFileArguments(paths []string, options ...ProcessFileArgumentsOptions) (ProcessedFileArguments, error) {
	var result ProcessedFileArguments
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return ProcessedFileArguments{}, err
		}
		if mimeType := imageMIMETypeForPath(path); mimeType != "" {
			result.Images = append(result.Images, llm.Image(base64.StdEncoding.EncodeToString(content), mimeType))
			continue
		}
		if result.Text != "" && !strings.HasSuffix(result.Text, "\n") {
			result.Text += "\n"
		}
		result.Text += string(content)
	}
	return result, nil
}
