package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	llama "github.com/nowa/gi/gi-coding-agent/internal/llama"
	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestBuiltinLlamaManagerRegistersNativeProviderAndRefreshesCatalog(
	t *testing.T,
) {
	t.Setenv("LLAMA_BASE_URL", "")
	t.Setenv("LLAMA_API_KEY", "")
	credentials := llm.NewInMemoryCredentialStore(
		map[string]llm.Credential{
			llama.LlamaProviderID: {
				Type: llm.CredentialTypeAPIKey,
				Key:  "secret",
				Env: llm.ProviderEnv{
					"LLAMA_BASE_URL": "http://llama.test",
				},
			},
		},
	)
	registry := NewModelRegistryWithOptions(
		context.Background(),
		ModelRegistryOptions{
			AuthStorage: NewInMemoryAuthStorage(nil),
			ModelNetworkEnabled: modelRuntimeBoolPointer(
				false,
			),
		},
	)
	runtime, err := NewModelRuntime(
		context.Background(),
		ModelRuntimeOptions{
			Registry:    registry,
			Credentials: credentials,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	manager, err := newBuiltinLlamaManager(
		runtime,
		builtinLlamaManagerOptions{
			LlamaClient: llama.LlamaClientOptions{
				HTTPClient: llamaRootHTTPDoerFunc(
					func(request *http.Request) (
						*http.Response,
						error,
					) {
						requests.Add(1)
						if request.URL.Path != "/models" {
							t.Errorf(
								"path = %q",
								request.URL.Path,
							)
						}
						if request.Header.Get("Authorization") !=
							"Bearer secret" {
							t.Errorf(
								"authorization = %q",
								request.Header.Get(
									"Authorization",
								),
							)
						}
						return llamaRootJSONResponse(
							http.StatusOK,
							map[string]any{
								"data": []map[string]any{{
									"id": "local-model",
									"status": map[string]any{
										"value": "loaded",
									},
								}},
							},
						), nil
					},
				),
				Timeout: time.Second,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := runtime.GetRegisteredNativeProvider(
		llama.LlamaProviderID,
	); !ok {
		t.Fatal("llama.cpp native provider was not registered")
	}
	if err := manager.refreshConfigured(
		context.Background(),
		true,
	); err != nil {
		t.Fatal(err)
	}
	if requests.Load() == 0 {
		t.Fatal("configured llama.cpp catalog was not refreshed")
	}
	model, ok := runtime.GetModel(
		llama.LlamaProviderID,
		"local-model",
	)
	if !ok ||
		model.BaseURL != "http://llama.test/v1" {
		t.Fatalf("runtime model = %#v, %v", model, ok)
	}
	auth, err := runtime.GetProviderAuth(
		context.Background(),
		llama.LlamaProviderID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if auth == nil ||
		auth.Auth.APIKey != "secret" ||
		auth.Env["LLAMA_BASE_URL"] != "http://llama.test" {
		t.Fatalf("provider auth = %#v", auth)
	}
	client, err := manager.configuredClient(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if client.ServerURL() != "http://llama.test" {
		t.Fatalf("management URL = %q", client.ServerURL())
	}
	ui := &llamaFakeManagerUI{}
	if err := manager.run(context.Background(), ui); err != nil {
		t.Fatal(err)
	}
	ui.mu.Lock()
	shownServer := ui.server
	shownModels := append(
		[]llama.LlamaModelInfo(nil),
		ui.models...,
	)
	ui.mu.Unlock()
	if shownServer != "http://llama.test" ||
		len(shownModels) != 1 ||
		shownModels[0].ID != "local-model" {
		t.Fatalf(
			"manager view state = server %q, models %#v",
			shownServer,
			shownModels,
		)
	}
	foundCommand := false
	for _, command := range builtinInteractiveSlashCommands() {
		if command.Name == "llama" {
			foundCommand =
				command.Description ==
					builtinLlamaCommandDescription
		}
	}
	if !foundCommand {
		t.Fatal("/llama command was not registered")
	}
}

func TestLlamaManagerViewProjectsModelsAndAcceptsExactDownload(
	t *testing.T,
) {
	view := newLlamaManagerView(nil, nil)
	t.Cleanup(view.Close)
	contextWindow := 32_768
	actionResult := make(chan llamaManagerAction, 1)
	actionError := make(chan error, 1)
	go func() {
		action, err := view.ShowModels(
			context.Background(),
			"http://llama.test",
			[]llama.LlamaModelInfo{
				{
					ID: "sleeping",
					Status: llama.LlamaModelStatus{
						Value: llama.LlamaModelSleeping,
						Args:  []string{"--ctx-size", "4096"},
					},
				},
				{
					ID: "loaded",
					Status: llama.LlamaModelStatus{
						Value: llama.LlamaModelLoaded,
					},
					Meta: llama.LlamaModelMeta{
						ContextWindow: &contextWindow,
					},
				},
			},
		)
		if err != nil {
			actionError <- err
			return
		}
		actionResult <- action
	}()
	waitForLlamaViewScreen(t, view, llamaManagerScreenModels)
	rendered := strings.Join(view.Render(100), "\n")
	for _, expected := range []string{
		"http://llama.test",
		"loaded",
		"33k context",
		"Download model",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf(
				"model view missing %q:\n%s",
				expected,
				rendered,
			)
		}
	}
	view.HandleInput("\r")
	select {
	case err := <-actionError:
		t.Fatal(err)
	case action := <-actionResult:
		if action.Kind != llamaManagerActionModel ||
			action.Model.ID != "loaded" {
			t.Fatalf("model action = %#v", action)
		}
	case <-time.After(time.Second):
		t.Fatal("model selection did not complete")
	}

	searchResult := make(chan string, 1)
	searchError := make(chan error, 1)
	go func() {
		selected, ok, err := view.SearchModels(
			context.Background(),
			func(context.Context, string) (
				[]llama.HuggingFaceModel,
				error,
			) {
				return nil, errors.New(
					"exact input should not require search",
				)
			},
		)
		if err != nil {
			searchError <- err
			return
		}
		if !ok {
			searchError <- errors.New("search was cancelled")
			return
		}
		searchResult <- selected
	}()
	waitForLlamaViewScreen(t, view, llamaManagerScreenSearch)
	view.HandleInput("owner/repository:Q4_K_M")
	view.HandleInput("\r")
	select {
	case err := <-searchError:
		t.Fatal(err)
	case selected := <-searchResult:
		if selected != "owner/repository:Q4_K_M" {
			t.Fatalf("selected model = %q", selected)
		}
	case <-time.After(time.Second):
		t.Fatal("exact Hugging Face selection did not complete")
	}
}

func TestRunLlamaWithProgressSerializesUpdatesAndCancellation(
	t *testing.T,
) {
	t.Run("success", func(t *testing.T) {
		ui := &llamaFakeManagerUI{}
		ratio := 0.5
		result, err := runLlamaWithProgress(
			context.Background(),
			ui,
			llamaProgressOptions[string]{
				Title:          "Loading model",
				Model:          "model",
				InitialMessage: "Starting…",
				Run: func(
					_ context.Context,
					update func(llama.LlamaProgress),
				) (string, error) {
					update(llama.LlamaProgress{
						Message: "Loading text model",
						Ratio:   &ratio,
					})
					return "loaded", nil
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.Cancelled || result.Value != "loaded" {
			t.Fatalf("progress result = %#v", result)
		}
		ui.mu.Lock()
		updates := append([]llamaProgressState(nil), ui.updates...)
		ui.mu.Unlock()
		if len(updates) == 0 ||
			updates[len(updates)-1].Message !=
				"Loading text model" {
			t.Fatalf("progress updates = %#v", updates)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		ui := &llamaFakeManagerUI{
			stopImmediately: true,
			confirm:         true,
		}
		var cancelCalls atomic.Int32
		result, err := runLlamaWithProgress(
			context.Background(),
			ui,
			llamaProgressOptions[string]{
				Title:          "Downloading model",
				Model:          "owner/model",
				InitialMessage: "Starting…",
				Run: func(
					ctx context.Context,
					_ func(llama.LlamaProgress),
				) (string, error) {
					<-ctx.Done()
					return "", ctx.Err()
				},
				Cancel: func(context.Context) error {
					cancelCalls.Add(1)
					return nil
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Cancelled ||
			cancelCalls.Load() != 1 {
			t.Fatalf(
				"cancel result = %#v, calls=%d",
				result,
				cancelCalls.Load(),
			)
		}
	})
}

func TestLlamaManagerSearchRejectsStaleResults(
	t *testing.T,
) {
	view := newLlamaManagerView(nil, nil)
	t.Cleanup(view.Close)
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	result := make(chan bool, 1)
	errs := make(chan error, 1)
	go func() {
		_, selected, err := view.SearchModels(
			context.Background(),
			func(
				_ context.Context,
				query string,
			) ([]llama.HuggingFaceModel, error) {
				switch query {
				case "ab":
					close(firstStarted)
					<-releaseFirst
					return []llama.HuggingFaceModel{{
						ID: "owner/ab-stale",
					}}, nil
				case "abc":
					return []llama.HuggingFaceModel{{
						ID:        "owner/abc-current",
						Downloads: 1200,
					}}, nil
				default:
					return nil, nil
				}
			},
		)
		if err != nil {
			errs <- err
			return
		}
		result <- selected
	}()
	waitForLlamaViewScreen(t, view, llamaManagerScreenSearch)
	view.HandleInput("ab")
	select {
	case <-firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first debounced search did not start")
	}
	view.HandleInput("c")
	close(releaseFirst)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rendered := strings.Join(view.Render(100), "\n")
		if strings.Contains(rendered, "owner/abc-current") {
			if strings.Contains(rendered, "owner/ab-stale") {
				t.Fatalf(
					"stale result replaced current search:\n%s",
					rendered,
				)
			}
			view.HandleInput("\x1b")
			select {
			case err := <-errs:
				t.Fatal(err)
			case selected := <-result:
				if selected {
					t.Fatal("cancelled search reported a selection")
				}
			case <-time.After(time.Second):
				t.Fatal("cancelled search did not complete")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("current search result was not published")
}

func TestLlamaHelpersMatchPiManagerSemantics(t *testing.T) {
	repository, quantization := parseLlamaHuggingFaceModel(
		"owner/repository:Q4_K_M",
	)
	if repository != "owner/repository" ||
		quantization != "Q4_K_M" {
		t.Fatalf(
			"parsed model = %q, %q",
			repository,
			quantization,
		)
	}
	if !llamaModelIsLoaded(llama.LlamaModelInfo{
		Status: llama.LlamaModelStatus{
			Value: llama.LlamaModelSleeping,
		},
	}) {
		t.Fatal("sleeping model should be treated as loaded")
	}
	if message := llamaConnectionErrorMessage(
		errors.New("dial tcp: connection refused"),
	); message != "Could not connect to the server." {
		t.Fatalf("connection message = %q", message)
	}
	if got := compactLlamaCount(1_250_000); got != "1.2M" {
		t.Fatalf("compact count = %q", got)
	}
}

type llamaFakeManagerUI struct {
	mu              sync.Mutex
	server          string
	models          []llama.LlamaModelInfo
	updates         []llamaProgressState
	stopImmediately bool
	confirm         bool
}

func (u *llamaFakeManagerUI) ShowModels(
	_ context.Context,
	server string,
	models []llama.LlamaModelInfo,
) (llamaManagerAction, error) {
	u.mu.Lock()
	u.server = server
	u.models = append([]llama.LlamaModelInfo(nil), models...)
	u.mu.Unlock()
	return llamaManagerAction{Kind: llamaManagerActionClose}, nil
}

func (*llamaFakeManagerUI) Select(
	context.Context,
	string,
	[]string,
) (string, bool, error) {
	return "", false, nil
}

func (u *llamaFakeManagerUI) Confirm(
	context.Context,
	string,
	string,
) (bool, error) {
	return u.confirm, nil
}

func (*llamaFakeManagerUI) ConnectionError(
	context.Context,
	string,
	string,
) (bool, error) {
	return false, nil
}

func (*llamaFakeManagerUI) SearchModels(
	context.Context,
	func(context.Context, string) (
		[]llama.HuggingFaceModel,
		error,
	),
) (string, bool, error) {
	return "", false, nil
}

func (*llamaFakeManagerUI) ShowStatus(string, string) {}

func (u *llamaFakeManagerUI) ShowProgress(
	llamaProgressState,
) <-chan struct{} {
	stop := make(chan struct{}, 1)
	if u.stopImmediately {
		stop <- struct{}{}
	}
	return stop
}

func (u *llamaFakeManagerUI) UpdateProgress(
	state llamaProgressState,
) {
	u.mu.Lock()
	u.updates = append(u.updates, state)
	u.mu.Unlock()
}

func (*llamaFakeManagerUI) Notify(string, string) {}

type llamaRootHTTPDoerFunc func(
	*http.Request,
) (*http.Response, error)

func (f llamaRootHTTPDoerFunc) Do(
	request *http.Request,
) (*http.Response, error) {
	return f(request)
}

func llamaRootJSONResponse(
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

func waitForLlamaViewScreen(
	t *testing.T,
	view *llamaManagerView,
	kind llamaManagerScreenKind,
) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		view.mu.RLock()
		current := view.screen.kind
		view.mu.RUnlock()
		if current == kind {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("Llama view did not show %q", kind)
}
