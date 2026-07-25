package llama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestLlamaNormalizesManagementAndInferenceURLsAndFormatsProgress(
	t *testing.T,
) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "http://127.0.0.1:8080/v1/",
			want:  "http://127.0.0.1:8080",
		},
		{
			input: "https://example.com/prefix/v1",
			want:  "https://example.com/prefix",
		},
	}
	for _, test := range tests {
		got, err := NormalizeLlamaServerURL(test.input)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf(
				"NormalizeLlamaServerURL(%q) = %q, want %q",
				test.input,
				got,
				test.want,
			)
		}
		inference, err := LlamaInferenceURL(test.input)
		if err != nil {
			t.Fatal(err)
		}
		if inference != test.want+"/v1" {
			t.Fatalf("inference URL = %q", inference)
		}
	}
	if _, err := NormalizeLlamaServerURL("file:///tmp/llama"); err == nil ||
		!strings.Contains(err.Error(), "http or https") {
		t.Fatalf("invalid scheme error = %v", err)
	}
	if got := FormatLlamaBytes(512); got != "512 B" {
		t.Fatalf("FormatLlamaBytes(512) = %q", got)
	}
	if got := FormatLlamaBytes(1024); got != "1.00 KiB" {
		t.Fatalf("FormatLlamaBytes(1024) = %q", got)
	}
}

func TestLlamaProviderPublishesLoadedCatalogAndResolvesAuth(
	t *testing.T,
) {
	var requests atomic.Int32
	client := llamaTestHTTPDoerFunc(
		func(request *http.Request) (*http.Response, error) {
			requests.Add(1)
			if request.Header.Get("Authorization") != "Bearer secret" {
				t.Errorf(
					"authorization = %q",
					request.Header.Get("Authorization"),
				)
			}
			return llamaTestJSONResponse(http.StatusOK, map[string]any{
				"data": []map[string]any{
					{
						"id": "server-loaded",
						"status": map[string]any{
							"value": "loaded",
						},
					},
				},
			}), nil
		},
	)
	controller, err := CreateLlamaProvider(LlamaProviderOptions{
		HTTPClient: client,
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	contextWindow := 65_536
	trainingContext := 131_072
	if err := controller.SetCatalog(
		[]LlamaModelInfo{
			{
				ID: "loaded",
				Status: LlamaModelStatus{
					Value: LlamaModelLoaded,
				},
				Architecture: LlamaModelArchitecture{
					InputModalities: []string{"text", "image"},
				},
				Meta: LlamaModelMeta{
					ContextWindow:   &contextWindow,
					TrainingContext: &trainingContext,
				},
			},
			{
				ID: "unloaded",
				Status: LlamaModelStatus{
					Value: LlamaModelUnloaded,
				},
			},
			{
				ID: "loading",
				Status: LlamaModelStatus{
					Value: LlamaModelLoading,
				},
			},
		},
		"http://llama.test",
	); err != nil {
		t.Fatal(err)
	}
	provider := controller.Provider()
	models, err := provider.GetModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("models = %#v", models)
	}
	model := models[0]
	if model.ID != "loaded" ||
		model.BaseURL != "http://llama.test/v1" ||
		model.ContextWindow != contextWindow ||
		model.MaxTokens != contextWindow ||
		!reflect.DeepEqual(model.Input, []string{"text", "image"}) {
		t.Fatalf("loaded model = %#v", model)
	}
	contextWindow = 1
	model.Input[0] = "mutated"
	if model.Compat.SupportsStore == nil {
		t.Fatal("loaded model has no supports-store policy")
	}
	*model.Compat.SupportsStore = true
	models, err = provider.GetModels()
	if err != nil {
		t.Fatal(err)
	}
	if models[0].ContextWindow != 65_536 ||
		!reflect.DeepEqual(models[0].Input, []string{"text", "image"}) ||
		models[0].Compat.SupportsStore == nil ||
		*models[0].Compat.SupportsStore {
		t.Fatalf("provider catalog aliases caller state: %#v", models[0])
	}
	if err := controller.SetCatalog(
		[]LlamaModelInfo{{
			Status: LlamaModelStatus{Value: LlamaModelLoaded},
		}},
		"http://llama.test",
	); err == nil || !strings.Contains(err.Error(), "model ID") {
		t.Fatalf("missing model ID error = %v", err)
	}

	auth := provider.Auth.APIKey
	emptyContext := llm.AuthContextFuncs{
		EnvFunc: func(
			context.Context,
			string,
		) (string, bool, error) {
			return "", false, nil
		},
	}
	if check, err := auth.Check(
		context.Background(),
		llm.APIKeyCheckInput{Context: emptyContext},
	); err != nil || check != nil {
		t.Fatalf("empty auth check = %#v, %v", check, err)
	}
	if resolved, err := auth.Resolve(
		context.Background(),
		llm.APIKeyResolveInput{Context: emptyContext},
	); err != nil || resolved != nil {
		t.Fatalf("empty auth resolve = %#v, %v", resolved, err)
	}

	interaction := &llamaTestAuthInteraction{
		answers: []string{"http://llama.test", "secret"},
	}
	credential, err := auth.Login(
		context.Background(),
		interaction,
	)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Type != llm.CredentialTypeAPIKey ||
		credential.Key != "secret" ||
		credential.Env["LLAMA_BASE_URL"] != "http://llama.test" {
		t.Fatalf("credential = %#v", credential)
	}
	resolved, err := auth.Resolve(
		context.Background(),
		llm.APIKeyResolveInput{
			Context:    emptyContext,
			Credential: &credential,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if resolved == nil ||
		resolved.Auth.APIKey != "secret" ||
		resolved.Auth.BaseURL != "http://llama.test/v1" ||
		resolved.Source != "stored credential" {
		t.Fatalf("resolved auth = %#v", resolved)
	}
	if err := provider.RefreshModels(
		context.Background(),
		llm.RefreshModelsContext{
			Credential:   &credential,
			AllowNetwork: true,
		},
	); err != nil {
		t.Fatal(err)
	}
	models, err = provider.GetModels()
	if err != nil || len(models) != 1 ||
		models[0].ID != "server-loaded" {
		t.Fatalf("refreshed models = %#v, %v", models, err)
	}
	requestCount := requests.Load()
	if err := provider.RefreshModels(
		context.Background(),
		llm.RefreshModelsContext{
			Credential:   &credential,
			AllowNetwork: false,
		},
	); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != requestCount {
		t.Fatal("offline provider refresh performed a network request")
	}
}

func TestHuggingFaceClientSearchDetailsAndRateLimit(
	t *testing.T,
) {
	clientTransport := llamaTestHTTPDoerFunc(
		func(request *http.Request) (*http.Response, error) {
			if request.Header.Get("Authorization") !=
				"Bearer hf-secret" {
				t.Errorf(
					"authorization = %q",
					request.Header.Get("Authorization"),
				)
			}
			if request.URL.Path == "/api/models" &&
				request.URL.Query().Get("search") == "limited" {
				response := llamaTestJSONResponse(
					http.StatusTooManyRequests,
					map[string]any{"error": "too many"},
				)
				response.Header.Set("RateLimit", "api;t=17")
				return response, nil
			}
			if request.URL.Path == "/api/models" {
				query := request.URL.Query()
				if query.Get("search") != "qwen coder" ||
					query.Get("filter") != "gguf" ||
					query.Get("sort") != "downloads" ||
					query.Get("direction") != "-1" ||
					query.Get("limit") != "20" {
					t.Errorf("search query = %q", request.URL.RawQuery)
				}
				return llamaTestJSONResponse(
					http.StatusOK,
					[]map[string]any{
						{
							"id":        "owner/model-GGUF",
							"downloads": 1200,
						},
					},
				), nil
			}
			if request.URL.Path == "/api/models/owner/model-GGUF" &&
				request.URL.Query().Get("blobs") == "true" {
				return llamaTestJSONResponse(
					http.StatusOK,
					map[string]any{
						"id":    "owner/model-GGUF",
						"gated": "manual",
						"siblings": []map[string]any{
							{
								"rfilename": "model-Q5_K_M.gguf",
								"size":      6000,
							},
							{
								"rfilename": "model-Q4_K_M-00001-of-00002.gguf",
								"size":      2000,
							},
							{
								"rfilename": "model-Q4_K_M-00002-of-00002.gguf",
								"size":      3000,
							},
							{
								"rfilename": "mmproj-F16.gguf",
								"size":      1000,
							},
						},
					},
				), nil
			}
			return llamaTestJSONResponse(
				http.StatusNotFound,
				map[string]any{"error": "not found"},
			), nil
		},
	)
	client, err := NewHuggingFaceClient(
		"hf-secret",
		HuggingFaceClientOptions{
			BaseURL:    "https://huggingface.test",
			HTTPClient: clientTransport,
			Timeout:    time.Second,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	models, err := client.Search(context.Background(), "qwen coder")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(models, []HuggingFaceModel{
		{ID: "owner/model-GGUF", Downloads: 1200},
	}) {
		t.Fatalf("search models = %#v", models)
	}
	details, err := client.Details(
		context.Background(),
		"owner/model-GGUF",
	)
	if err != nil {
		t.Fatal(err)
	}
	if details.ID != "owner/model-GGUF" ||
		details.Gating != HuggingFaceGatingManual ||
		len(details.Quantizations) != 2 ||
		details.Quantizations[0].Name != "Q4_K_M" ||
		details.Quantizations[0].Size == nil ||
		*details.Quantizations[0].Size != 5000 ||
		details.Quantizations[1].Name != "Q5_K_M" ||
		details.Quantizations[1].Size == nil ||
		*details.Quantizations[1].Size != 6000 {
		t.Fatalf("details = %#v", details)
	}
	_, err = client.Search(context.Background(), "limited")
	var httpError *HuggingFaceHTTPError
	if !errors.As(err, &httpError) ||
		httpError.StatusCode != http.StatusTooManyRequests ||
		httpError.Message !=
			"Hugging Face rate limit reached; retry in 17s" {
		t.Fatalf("rate limit error = %T %v", err, err)
	}

	t.Setenv("HF_TOKEN", " hf-secret ")
	if token := FindHuggingFaceToken(); token != "hf-secret" {
		t.Fatalf("Hugging Face token = %q", token)
	}
}

func TestReadHuggingFaceTokenBoundsInput(t *testing.T) {
	path := t.TempDir() + "/token"
	if err := os.WriteFile(
		path,
		[]byte(strings.Repeat("x", maxHuggingFaceTokenBytes+1)),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := readHuggingFaceToken(path); err == nil ||
		!strings.Contains(err.Error(), "size limit") {
		t.Fatalf("oversized token error = %v", err)
	}
}

func TestLlamaClientLoadAndDownloadMergeSSEWithPolling(
	t *testing.T,
) {
	t.Run("load", func(t *testing.T) {
		router := newLlamaTestRouter("load", "test-model")
		client, err := NewLlamaClient(
			"http://llama.test",
			"",
			LlamaClientOptions{
				HTTPClient:   router,
				Timeout:      time.Second,
				PollInterval: 5 * time.Millisecond,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		var progress []LlamaProgress
		model, err := client.LoadAndWait(
			context.Background(),
			"test-model",
			func(update LlamaProgress) {
				progress = append(progress, update)
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if model.Status.Value != LlamaModelLoaded {
			t.Fatalf("loaded model = %#v", model)
		}
		if !llamaProgressContains(
			progress,
			"Loading text model",
			"",
		) {
			t.Fatalf("load progress = %#v", progress)
		}
	})

	t.Run("download", func(t *testing.T) {
		router := newLlamaTestRouter(
			"download",
			"owner/repo:Q4_K_M",
		)
		client, err := NewLlamaClient(
			"http://llama.test",
			"",
			LlamaClientOptions{
				HTTPClient:   router,
				Timeout:      time.Second,
				PollInterval: 5 * time.Millisecond,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		var progress []LlamaProgress
		models, err := client.DownloadAndWait(
			context.Background(),
			"owner/repo:Q4_K_M",
			func(update LlamaProgress) {
				progress = append(progress, update)
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(models) != 1 ||
			models[0].Status.Value != LlamaModelUnloaded {
			t.Fatalf("downloaded catalog = %#v", models)
		}
		if !llamaProgressContains(
			progress,
			"Downloading model",
			"512 B / 1.00 KiB",
		) {
			t.Fatalf("download progress = %#v", progress)
		}
	})
}

func TestLlamaClientHonorsCallerCancellation(t *testing.T) {
	client, err := NewLlamaClient(
		"http://llama.test",
		"",
		LlamaClientOptions{
			HTTPClient: llamaTestHTTPDoerFunc(
				func(request *http.Request) (*http.Response, error) {
					<-request.Context().Done()
					return nil, request.Context().Err()
				},
			),
			Timeout: time.Second,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.List(
		ctx,
		LlamaListOptions{},
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v, want context canceled", err)
	}
}

type llamaTestAuthInteraction struct {
	answers []string
}

func (i *llamaTestAuthInteraction) Prompt(
	_ context.Context,
	_ llm.AuthPrompt,
) (string, error) {
	if len(i.answers) == 0 {
		return "", errors.New("no answer")
	}
	answer := i.answers[0]
	i.answers = i.answers[1:]
	return answer, nil
}

func (*llamaTestAuthInteraction) Notify(llm.AuthEvent) {}

type llamaTestHTTPDoerFunc func(
	*http.Request,
) (*http.Response, error)

func (f llamaTestHTTPDoerFunc) Do(
	request *http.Request,
) (*http.Response, error) {
	return f(request)
}

type llamaTestRouter struct {
	mode  string
	model string

	mu      sync.Mutex
	status  LlamaModelLifecycle
	streams map[*io.PipeWriter]struct{}
	ready   chan struct{}
	once    sync.Once
}

func newLlamaTestRouter(mode, model string) *llamaTestRouter {
	router := &llamaTestRouter{
		mode:    mode,
		model:   model,
		streams: map[*io.PipeWriter]struct{}{},
		ready:   make(chan struct{}),
	}
	if mode == "load" {
		router.status = LlamaModelUnloaded
	}
	return router
}

func (r *llamaTestRouter) Do(
	request *http.Request,
) (*http.Response, error) {
	switch {
	case request.URL.Path == "/models/sse":
		reader, writer := io.Pipe()
		r.mu.Lock()
		r.streams[writer] = struct{}{}
		r.once.Do(func() {
			close(r.ready)
		})
		r.mu.Unlock()
		go func() {
			<-request.Context().Done()
			r.mu.Lock()
			delete(r.streams, writer)
			r.mu.Unlock()
			_ = writer.CloseWithError(request.Context().Err())
		}()
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"},
			},
			Body: reader,
		}, nil
	case request.URL.Path == "/models/load" &&
		request.Method == http.MethodPost:
		r.setStatus(LlamaModelLoading)
		r.waitForStream()
		go r.finishLoad()
		return llamaTestJSONResponse(
			http.StatusOK,
			map[string]any{"success": true},
		), nil
	case request.URL.Path == "/models" &&
		request.Method == http.MethodPost:
		r.setStatus(LlamaModelDownloading)
		r.waitForStream()
		go r.finishDownload()
		return llamaTestJSONResponse(
			http.StatusOK,
			map[string]any{"success": true},
		), nil
	case request.URL.Path == "/models":
		status := r.getStatus()
		data := []map[string]any{}
		if r.mode == "load" || status != "" {
			data = append(data, map[string]any{
				"id": r.model,
				"status": map[string]any{
					"value": status,
				},
			})
		}
		return llamaTestJSONResponse(
			http.StatusOK,
			map[string]any{"data": data},
		), nil
	default:
		return llamaTestJSONResponse(
			http.StatusNotFound,
			map[string]any{"error": map[string]any{
				"message": "not found",
			}},
		), nil
	}
}

func (r *llamaTestRouter) finishLoad() {
	time.Sleep(20 * time.Millisecond)
	r.send(map[string]any{
		"model": r.model,
		"event": "status_change",
		"data": map[string]any{
			"status": "loading",
			"progress": map[string]any{
				"stages": []string{
					"text_model",
					"mmproj_model",
				},
				"current": "text_model",
				"value":   0.5,
			},
		},
	})
	r.setStatus(LlamaModelLoaded)
	r.send(map[string]any{
		"model": r.model,
		"event": "status_change",
		"data": map[string]any{
			"status": "loaded",
		},
	})
}

func (r *llamaTestRouter) finishDownload() {
	time.Sleep(20 * time.Millisecond)
	r.send(map[string]any{
		"model": r.model,
		"event": "download_progress",
		"data": map[string]any{
			"progress": map[string]any{
				"https://example/model.gguf": map[string]any{
					"done":  512,
					"total": 1024,
				},
			},
		},
	})
	r.setStatus(LlamaModelUnloaded)
	r.send(map[string]any{
		"model": r.model,
		"event": "download_finished",
		"data":  map[string]any{},
	})
}

func (r *llamaTestRouter) setStatus(
	status LlamaModelLifecycle,
) {
	r.mu.Lock()
	r.status = status
	r.mu.Unlock()
}

func (r *llamaTestRouter) getStatus() LlamaModelLifecycle {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *llamaTestRouter) waitForStream() {
	select {
	case <-r.ready:
	case <-time.After(time.Second):
	}
}

func (r *llamaTestRouter) send(event any) {
	encoded, err := json.Marshal(event)
	if err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for writer := range r.streams {
		_, _ = fmt.Fprintf(writer, "data: %s\n\n", encoded)
	}
}

func llamaProgressContains(
	progress []LlamaProgress,
	message string,
	detail string,
) bool {
	for _, entry := range progress {
		if entry.Message == message && entry.Detail == detail {
			return true
		}
	}
	return false
}

func llamaTestJSONResponse(
	status int,
	value any,
) *http.Response {
	encoded, _ := json.Marshal(value)
	return &http.Response{
		StatusCode: status,
		Status: fmt.Sprintf(
			"%d %s",
			status,
			http.StatusText(status),
		),
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(string(encoded))),
	}
}
