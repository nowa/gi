package gillmprovider

import (
	"fmt"
	"regexp"
)

type OpenRouterImagesPayload struct {
	Model      string                    `json:"model"`
	Messages   []OpenRouterImagesMessage `json:"messages"`
	Stream     bool                      `json:"stream"`
	Modalities []string                  `json:"modalities"`
}

type OpenRouterImagesMessage struct {
	Role    string                        `json:"role"`
	Content []OpenRouterImagesContentPart `json:"content"`
}

type OpenRouterImagesContentPart struct {
	Type     string                    `json:"type"`
	Text     string                    `json:"text,omitempty"`
	ImageURL *OpenRouterImagesImageURL `json:"image_url,omitempty"`
}

type OpenRouterImagesImageURL struct {
	URL string `json:"url"`
}

type OpenRouterImagesResponse struct {
	ID      string                   `json:"id"`
	Usage   *OpenRouterImagesUsage   `json:"usage,omitempty"`
	Choices []OpenRouterImagesChoice `json:"choices"`
}

type OpenRouterImagesUsage struct {
	PromptTokens        int                           `json:"prompt_tokens"`
	CompletionTokens    int                           `json:"completion_tokens"`
	PromptTokensDetails OpenRouterPromptTokensDetails `json:"prompt_tokens_details"`
}

type OpenRouterPromptTokensDetails struct {
	CachedTokens     int `json:"cached_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

type OpenRouterImagesChoice struct {
	Message OpenRouterImagesChoiceMessage `json:"message"`
}

type OpenRouterImagesChoiceMessage struct {
	Content string                     `json:"content,omitempty"`
	Images  []OpenRouterGeneratedImage `json:"images,omitempty"`
}

type OpenRouterGeneratedImage struct {
	ImageURL any `json:"image_url"`
}

func BuildOpenRouterImagesPayload(model ImagesModel, imagesContext ImagesContext) OpenRouterImagesPayload {
	content := make([]OpenRouterImagesContentPart, 0, len(imagesContext.Input))
	for _, input := range imagesContext.Input {
		switch input.Type {
		case ContentText:
			content = append(content, OpenRouterImagesContentPart{Type: "text", Text: SanitizeSurrogates(input.Text)})
		case ContentImage:
			content = append(content, OpenRouterImagesContentPart{
				Type:     "image_url",
				ImageURL: &OpenRouterImagesImageURL{URL: "data:" + input.MIMEType + ";base64," + input.Data},
			})
		}
	}
	modalities := []string{"image"}
	if containsString(model.Output, "text") {
		modalities = []string{"image", "text"}
	}
	return OpenRouterImagesPayload{
		Model:      model.ID,
		Messages:   []OpenRouterImagesMessage{{Role: "user", Content: content}},
		Stream:     false,
		Modalities: modalities,
	}
}

func ParseOpenRouterImagesResponse(model ImagesModel, response OpenRouterImagesResponse) AssistantImages {
	output := AssistantImages{
		API:        model.API,
		Provider:   model.Provider,
		Model:      model.ID,
		ResponseID: response.ID,
		StopReason: ImagesStopReasonStop,
		Timestamp:  NowMillis(),
	}
	if response.Usage != nil {
		output.Usage = ParseOpenRouterImagesUsage(*response.Usage, model)
	}
	if len(response.Choices) == 0 {
		return output
	}
	message := response.Choices[0].Message
	if message.Content != "" {
		output.Output = append(output.Output, ImageText(SanitizeSurrogates(message.Content)))
	}
	for _, image := range message.Images {
		mimeType, data, ok := parseOpenRouterDataImageURL(openRouterGeneratedImageURL(image.ImageURL))
		if !ok {
			continue
		}
		output.Output = append(output.Output, ImageData(data, mimeType))
	}
	return output
}

func ParseOpenRouterImagesUsage(raw OpenRouterImagesUsage, model ImagesModel) Usage {
	promptTokens := raw.PromptTokens
	cacheWriteTokens := raw.PromptTokensDetails.CacheWriteTokens
	reportedCachedTokens := raw.PromptTokensDetails.CachedTokens
	cacheReadTokens := reportedCachedTokens
	if cacheWriteTokens > 0 {
		cacheReadTokens = reportedCachedTokens - cacheWriteTokens
		if cacheReadTokens < 0 {
			cacheReadTokens = 0
		}
	}
	input := promptTokens - cacheReadTokens - cacheWriteTokens
	if input < 0 {
		input = 0
	}
	usage := Usage{
		Input:       input,
		Output:      raw.CompletionTokens,
		CacheRead:   cacheReadTokens,
		CacheWrite:  cacheWriteTokens,
		TotalTokens: input + raw.CompletionTokens + cacheReadTokens + cacheWriteTokens,
	}
	usage.Cost = CalculateCost(Model{Cost: model.Cost}, usage)
	return usage
}

func openRouterGeneratedImageURL(value any) string {
	switch url := value.(type) {
	case string:
		return url
	case map[string]any:
		if raw, ok := url["url"].(string); ok {
			return raw
		}
	case OpenRouterImagesImageURL:
		return url.URL
	case *OpenRouterImagesImageURL:
		if url != nil {
			return url.URL
		}
	}
	return ""
}

var openRouterDataImagePattern = regexp.MustCompile(`^data:([^;]+);base64,(.+)$`)

func parseOpenRouterDataImageURL(value string) (mimeType, data string, ok bool) {
	if value == "" {
		return "", "", false
	}
	matches := openRouterDataImagePattern.FindStringSubmatch(value)
	if len(matches) != 3 {
		return "", "", false
	}
	return matches[1], matches[2], true
}

func ErrorImages(model ImagesModel, err error) AssistantImages {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return AssistantImages{
		API:          model.API,
		Provider:     model.Provider,
		Model:        model.ID,
		StopReason:   ImagesStopReasonError,
		ErrorMessage: message,
		Timestamp:    NowMillis(),
	}
}

func ValidateOpenRouterImagesPayload(payload OpenRouterImagesPayload) error {
	if payload.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(payload.Messages) != 1 || payload.Messages[0].Role != "user" {
		return fmt.Errorf("openrouter image payload requires exactly one user message")
	}
	return nil
}
