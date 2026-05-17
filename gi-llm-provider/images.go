package gillmprovider

import (
	"context"
	"fmt"
	"sync"
)

const (
	ImagesStopReasonStop    = "stop"
	ImagesStopReasonError   = "error"
	ImagesStopReasonAborted = "aborted"
)

type ImagesContent struct {
	Type     string
	Text     string
	Data     string
	MIMEType string
}

func ImageText(text string) ImagesContent {
	return ImagesContent{Type: ContentText, Text: text}
}

func ImageData(data, mimeType string) ImagesContent {
	return ImagesContent{Type: ContentImage, Data: data, MIMEType: mimeType}
}

type ImagesContext struct {
	Input []ImagesContent
}

type ImagesModel struct {
	ID       string
	Name     string
	API      string
	Provider string
	BaseURL  string
	Input    []string
	Output   []string
	Cost     ModelCost
	Headers  map[string]string
}

type AssistantImages struct {
	API          string
	Provider     string
	Model        string
	Output       []ImagesContent
	ResponseID   string
	Usage        Usage
	StopReason   string
	ErrorMessage string
	Timestamp    int64
}

type ImagesOptions struct {
	Context         context.Context
	APIKey          string
	Headers         map[string]string
	TimeoutMillis   int
	MaxRetries      int
	MaxRetryDelayMs int
	OnPayload       func(payload any, model ImagesModel) (any, bool, error)
	OnResponse      func(status int, headers map[string]string, model ImagesModel) error
}

type ImagesAPIProvider interface {
	GenerateImages(model ImagesModel, imagesContext ImagesContext, options ImagesOptions) (AssistantImages, error)
}

type ImagesAPIProviderFuncs struct {
	GenerateImagesFunc func(ImagesModel, ImagesContext, ImagesOptions) (AssistantImages, error)
}

func (p ImagesAPIProviderFuncs) GenerateImages(model ImagesModel, imagesContext ImagesContext, options ImagesOptions) (AssistantImages, error) {
	if p.GenerateImagesFunc == nil {
		return AssistantImages{}, fmt.Errorf("provider does not implement image generation")
	}
	return p.GenerateImagesFunc(model, imagesContext, options)
}

var imagesAPIRegistry = struct {
	sync.RWMutex
	providers map[string]ImagesAPIProvider
}{providers: map[string]ImagesAPIProvider{}}

func RegisterImagesAPIProvider(api string, provider ImagesAPIProvider) {
	imagesAPIRegistry.Lock()
	defer imagesAPIRegistry.Unlock()
	imagesAPIRegistry.providers[api] = provider
}

func UnregisterImagesAPIProvider(api string) {
	imagesAPIRegistry.Lock()
	defer imagesAPIRegistry.Unlock()
	delete(imagesAPIRegistry.providers, api)
}

func GetImagesAPIProvider(api string) ImagesAPIProvider {
	imagesAPIRegistry.RLock()
	defer imagesAPIRegistry.RUnlock()
	return imagesAPIRegistry.providers[api]
}

func GenerateImages(model ImagesModel, imagesContext ImagesContext, options ImagesOptions) (AssistantImages, error) {
	provider := GetImagesAPIProvider(model.API)
	if provider == nil {
		return AssistantImages{}, fmt.Errorf("no image API provider registered for api: %s", model.API)
	}
	if options.Context != nil {
		select {
		case <-options.Context.Done():
			return AbortedImages(model, options.Context.Err()), nil
		default:
		}
	}
	return provider.GenerateImages(model, imagesContext, options)
}

func AbortedImages(model ImagesModel, err error) AssistantImages {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return AssistantImages{
		API:          model.API,
		Provider:     model.Provider,
		Model:        model.ID,
		StopReason:   ImagesStopReasonAborted,
		ErrorMessage: message,
		Timestamp:    NowMillis(),
	}
}
