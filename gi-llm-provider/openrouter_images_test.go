package gillmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestOpenRouterImageModelsRegistry(t *testing.T) {
	model := MustGetImageModel("openrouter", "google/gemini-2.5-flash-image")
	if model.API != "openrouter-images" || model.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("model = %#v", model)
	}
	if !stringSlicesEqual(model.Input, []string{"image", "text"}) || !stringSlicesEqual(model.Output, []string{"image", "text"}) {
		t.Fatalf("modalities = input:%#v output:%#v", model.Input, model.Output)
	}
	if model.Cost.Input != 0.3 || model.Cost.Output != 2.5 || model.Cost.CacheRead != 0.03 {
		t.Fatalf("cost = %#v", model.Cost)
	}

	flux := MustGetImageModel("openrouter", "black-forest-labs/flux.2-pro")
	if !stringSlicesEqual(flux.Output, []string{"image"}) {
		t.Fatalf("flux output = %#v", flux.Output)
	}
}

func TestBuildOpenRouterImagesPayload(t *testing.T) {
	model := MustGetImageModel("openrouter", "google/gemini-3.1-flash-image-preview")
	payload := BuildOpenRouterImagesPayload(model, ImagesContext{Input: []ImagesContent{
		ImageText("Generate \xed\xa0\xbd dog"),
		ImageData("ZmFrZS1wbmc=", "image/png"),
	}})

	if err := ValidateOpenRouterImagesPayload(payload); err != nil {
		t.Fatal(err)
	}
	if payload.Model != model.ID || payload.Stream {
		t.Fatalf("payload = %#v", payload)
	}
	if !reflect.DeepEqual(payload.Modalities, []string{"image", "text"}) {
		t.Fatalf("modalities = %#v", payload.Modalities)
	}
	if len(payload.Messages) != 1 || len(payload.Messages[0].Content) != 2 {
		t.Fatalf("messages = %#v", payload.Messages)
	}
	if got := payload.Messages[0].Content[0]; got.Type != "text" || got.Text != "Generate  dog" {
		t.Fatalf("text part = %#v", got)
	}
	if got := payload.Messages[0].Content[1]; got.Type != "image_url" || got.ImageURL == nil || got.ImageURL.URL != "data:image/png;base64,ZmFrZS1wbmc=" {
		t.Fatalf("image part = %#v", got)
	}
}

func TestBuildOpenRouterImagesPayloadImageOnlyOutput(t *testing.T) {
	model := MustGetImageModel("openrouter", "black-forest-labs/flux.2-pro")
	payload := BuildOpenRouterImagesPayload(model, ImagesContext{Input: []ImagesContent{ImageText("Generate a dog")}})

	if !reflect.DeepEqual(payload.Modalities, []string{"image"}) {
		t.Fatalf("modalities = %#v", payload.Modalities)
	}
}

func TestParseOpenRouterImagesResponse(t *testing.T) {
	model := MustGetImageModel("openrouter", "google/gemini-3.1-flash-image-preview")
	result := ParseOpenRouterImagesResponse(model, OpenRouterImagesResponse{
		ID: "img-1",
		Usage: &OpenRouterImagesUsage{
			PromptTokens:     12,
			CompletionTokens: 34,
			PromptTokensDetails: OpenRouterPromptTokensDetails{
				CachedTokens:     5,
				CacheWriteTokens: 2,
			},
		},
		Choices: []OpenRouterImagesChoice{{
			Message: OpenRouterImagesChoiceMessage{
				Content: "Here is your image.",
				Images: []OpenRouterGeneratedImage{
					{ImageURL: "data:image/png;base64,ZmFrZS1wbmc="},
					{ImageURL: OpenRouterImagesImageURL{URL: "https://example.invalid/remote.png"}},
					{ImageURL: map[string]any{"url": "data:image/webp;base64,d2VicA=="}},
				},
			},
		}},
	})

	if result.StopReason != ImagesStopReasonStop || result.ResponseID != "img-1" {
		t.Fatalf("metadata = %#v", result)
	}
	if len(result.Output) != 3 {
		t.Fatalf("output = %#v", result.Output)
	}
	if result.Output[0].Type != ContentText || result.Output[0].Text != "Here is your image." {
		t.Fatalf("text output = %#v", result.Output[0])
	}
	if result.Output[1].Type != ContentImage || result.Output[1].MIMEType != "image/png" || result.Output[1].Data != "ZmFrZS1wbmc=" {
		t.Fatalf("image output = %#v", result.Output[1])
	}
	if result.Output[2].MIMEType != "image/webp" || result.Output[2].Data != "d2VicA==" {
		t.Fatalf("map image output = %#v", result.Output[2])
	}
	if result.Usage.Input != 7 || result.Usage.CacheRead != 3 || result.Usage.CacheWrite != 2 || result.Usage.Output != 34 || result.Usage.TotalTokens != 46 {
		t.Fatalf("usage = %#v", result.Usage)
	}
	if result.Usage.Cost.Output <= 0 || result.Usage.Cost.Total <= 0 {
		t.Fatalf("cost = %#v", result.Usage.Cost)
	}
}

func TestOpenRouterImagesProviderPostsAndParsesHTTP(t *testing.T) {
	var requestPath string
	var authHeader string
	var refererHeader string
	var payload OpenRouterImagesPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		authHeader = r.Header.Get("Authorization")
		refererHeader = r.Header.Get("HTTP-Referer")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("content-type", "application/json")
		if err := json.NewEncoder(w).Encode(OpenRouterImagesResponse{
			ID: "img-live",
			Usage: &OpenRouterImagesUsage{
				PromptTokens:     12,
				CompletionTokens: 34,
				PromptTokensDetails: OpenRouterPromptTokensDetails{
					CachedTokens: 1,
				},
			},
			Choices: []OpenRouterImagesChoice{{
				Message: OpenRouterImagesChoiceMessage{
					Content: "Here is your image.",
					Images:  []OpenRouterGeneratedImage{{ImageURL: "data:image/png;base64,ZmFrZS1wbmc="}},
				},
			}},
		}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	model := MustGetImageModel("openrouter", "google/gemini-3.1-flash-image-preview")
	model.BaseURL = server.URL + "/api/v1"
	model.Headers = map[string]string{"HTTP-Referer": "https://example.com"}
	result, err := NewOpenRouterImagesProvider(server.Client()).GenerateImages(model, ImagesContext{
		Input: []ImagesContent{ImageText("Generate a dog")},
	}, ImagesOptions{APIKey: "openrouter-key"})
	if err != nil {
		t.Fatal(err)
	}

	if requestPath != "/api/v1/chat/completions" || authHeader != "Bearer openrouter-key" || refererHeader != "https://example.com" {
		t.Fatalf("request path/auth/referer = %q %q %q", requestPath, authHeader, refererHeader)
	}
	if payload.Model != model.ID || payload.Stream || !reflect.DeepEqual(payload.Modalities, []string{"image", "text"}) {
		t.Fatalf("payload = %#v", payload)
	}
	if result.StopReason != ImagesStopReasonStop || result.ResponseID != "img-live" {
		t.Fatalf("result metadata = %#v", result)
	}
	if len(result.Output) != 2 || result.Output[0].Text != "Here is your image." || result.Output[1].MIMEType != "image/png" {
		t.Fatalf("output = %#v", result.Output)
	}
	if result.Usage.Input != 11 || result.Usage.CacheRead != 1 || result.Usage.Output != 34 {
		t.Fatalf("usage = %#v", result.Usage)
	}
}

func TestOpenRouterImagesProviderHandlesHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	model := MustGetImageModel("openrouter", "black-forest-labs/flux.2-pro")
	model.BaseURL = server.URL
	result, err := NewOpenRouterImagesProvider(server.Client()).GenerateImages(model, ImagesContext{
		Input: []ImagesContent{ImageText("Generate a dog")},
	}, ImagesOptions{APIKey: "openrouter-key"})
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != ImagesStopReasonError || !strings.Contains(result.ErrorMessage, "HTTP 400") {
		t.Fatalf("result = %#v", result)
	}
}

func TestGenerateImagesHandlesImmediateContextCancel(t *testing.T) {
	model := MustGetImageModel("openrouter", "black-forest-labs/flux.2-pro")
	called := false
	RegisterImagesAPIProvider("test-openrouter-images", ImagesAPIProviderFuncs{
		GenerateImagesFunc: func(ImagesModel, ImagesContext, ImagesOptions) (AssistantImages, error) {
			called = true
			return AssistantImages{}, nil
		},
	})
	defer UnregisterImagesAPIProvider("test-openrouter-images")
	model.API = "test-openrouter-images"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := GenerateImages(model, ImagesContext{Input: []ImagesContent{ImageText("hi")}}, ImagesOptions{Context: ctx})

	if err != nil {
		t.Fatalf("GenerateImages() error = %v", err)
	}
	if called {
		t.Fatal("provider should not be called after context cancellation")
	}
	if result.StopReason != ImagesStopReasonAborted || result.ErrorMessage == "" {
		t.Fatalf("result = %#v", result)
	}
}
