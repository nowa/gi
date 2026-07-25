package gicodingagent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	llama "github.com/nowa/gi/gi-coding-agent/internal/llama"
)

const builtinLlamaCommandDescription = "Manage llama.cpp router models"

var errBuiltinLlamaNotConfigured = errors.New(
	"llama.cpp provider is not configured",
)

type builtinLlamaManagerOptions struct {
	LlamaClient     llama.LlamaClientOptions
	HuggingFace     llama.HuggingFaceClientOptions
	FindToken       func() string
	OperationCancel time.Duration
}

// builtinLlamaManager owns the application-layer data flow between the
// provider runtime, management client, and interactive workflow. Transport
// state remains in internal/llama and presentation state remains in
// llamaManagerView.
type builtinLlamaManager struct {
	runtime         *ModelRuntime
	provider        *llama.LlamaProviderController
	clientOptions   llama.LlamaClientOptions
	huggingFace     llama.HuggingFaceClientOptions
	findToken       func() string
	operationCancel time.Duration
}

func newBuiltinLlamaManager(
	runtime *ModelRuntime,
	options ...builtinLlamaManagerOptions,
) (*builtinLlamaManager, error) {
	if runtime == nil {
		return nil, errors.New("Llama manager requires a model runtime")
	}
	if len(options) > 1 {
		return nil, errors.New(
			"Llama manager accepts at most one options value",
		)
	}
	var selected builtinLlamaManagerOptions
	if len(options) == 1 {
		selected = options[0]
	}
	controller, err := llama.CreateLlamaProvider(
		llama.LlamaProviderOptions{
			HTTPClient:   selected.LlamaClient.HTTPClient,
			Timeout:      selected.LlamaClient.Timeout,
			PollInterval: selected.LlamaClient.PollInterval,
		},
	)
	if err != nil {
		return nil, err
	}
	if err := runtime.RegisterNativeProvider(
		controller.Provider(),
	); err != nil {
		return nil, err
	}
	findToken := selected.FindToken
	if findToken == nil {
		findToken = llama.FindHuggingFaceToken
	}
	cancelTimeout := selected.OperationCancel
	if cancelTimeout <= 0 {
		cancelTimeout = 15 * time.Second
	}
	return &builtinLlamaManager{
		runtime:         runtime,
		provider:        controller,
		clientOptions:   selected.LlamaClient,
		huggingFace:     selected.HuggingFace,
		findToken:       findToken,
		operationCancel: cancelTimeout,
	}, nil
}

func (m *builtinLlamaManager) refreshConfigured(
	ctx context.Context,
	allowNetwork bool,
) error {
	if m == nil || m.runtime == nil || !allowNetwork {
		return nil
	}
	_, configured, err := m.runtime.CheckAuth(
		llamaContextOrBackground(ctx),
		llama.LlamaProviderID,
	)
	if err != nil || !configured {
		return err
	}
	result, err := m.runtime.Refresh(
		ctx,
		ModelRegistryRefreshOptions{AllowNetwork: true},
	)
	if err != nil {
		return err
	}
	return result.Errors[llama.LlamaProviderID]
}

func (m *builtinLlamaManager) configuredClient(
	ctx context.Context,
) (*llama.LlamaClient, error) {
	if m == nil || m.runtime == nil {
		return nil, errors.New("Llama manager is not initialized")
	}
	auth, err := m.runtime.GetProviderAuth(
		llamaContextOrBackground(ctx),
		llama.LlamaProviderID,
	)
	if err != nil {
		return nil, err
	}
	if auth == nil {
		return nil, errBuiltinLlamaNotConfigured
	}
	serverURL := strings.TrimSpace(auth.Env["LLAMA_BASE_URL"])
	if serverURL == "" {
		serverURL = strings.TrimSpace(auth.Auth.BaseURL)
	}
	if serverURL == "" {
		return nil, errBuiltinLlamaNotConfigured
	}
	return llama.NewLlamaClient(
		serverURL,
		auth.Auth.APIKey,
		m.clientOptions,
	)
}

func (m *builtinLlamaManager) publishCatalog(
	ctx context.Context,
	client *llama.LlamaClient,
	catalog []llama.LlamaModelInfo,
) error {
	if m == nil || m.provider == nil || m.runtime == nil {
		return errors.New("Llama manager is not initialized")
	}
	if client == nil {
		return errors.New("llama.cpp client is required")
	}
	if err := m.provider.SetCatalog(
		catalog,
		client.ServerURL(),
	); err != nil {
		return err
	}
	result, err := m.runtime.Refresh(
		llamaContextOrBackground(ctx),
		ModelRegistryRefreshOptions{AllowNetwork: false},
	)
	if err != nil {
		return err
	}
	return result.Errors[llama.LlamaProviderID]
}

func (m *builtinLlamaManager) syncCatalog(
	ctx context.Context,
	client *llama.LlamaClient,
) ([]llama.LlamaModelInfo, error) {
	catalog, err := client.List(
		llamaContextOrBackground(ctx),
		llama.LlamaListOptions{},
	)
	if err != nil {
		return nil, err
	}
	if err := m.publishCatalog(ctx, client, catalog); err != nil {
		return nil, err
	}
	return catalog, nil
}

type llamaManagerActionKind string

const (
	llamaManagerActionModel    llamaManagerActionKind = "model"
	llamaManagerActionDownload llamaManagerActionKind = "download"
	llamaManagerActionClose    llamaManagerActionKind = "close"
)

type llamaManagerAction struct {
	Kind  llamaManagerActionKind
	Model llama.LlamaModelInfo
}

type llamaProgressState struct {
	Title   string
	Model   string
	Message string
	Ratio   *float64
	Detail  string
}

type llamaManagerUI interface {
	ShowModels(
		ctx context.Context,
		serverURL string,
		models []llama.LlamaModelInfo,
	) (llamaManagerAction, error)
	Select(
		ctx context.Context,
		title string,
		options []string,
	) (string, bool, error)
	Confirm(
		ctx context.Context,
		title string,
		message string,
	) (bool, error)
	ConnectionError(
		ctx context.Context,
		serverURL string,
		message string,
	) (retry bool, err error)
	SearchModels(
		ctx context.Context,
		search func(context.Context, string) (
			[]llama.HuggingFaceModel,
			error,
		),
	) (string, bool, error)
	ShowStatus(title string, message string)
	ShowProgress(state llamaProgressState) <-chan struct{}
	UpdateProgress(state llamaProgressState)
	Notify(message string, level string)
}

func (m *builtinLlamaManager) run(
	ctx context.Context,
	ui llamaManagerUI,
) error {
	if ui == nil {
		return errors.New("Llama manager UI is required")
	}
	ctx = llamaContextOrBackground(ctx)
	client, err := m.configuredClient(ctx)
	if err != nil {
		return err
	}
	readCatalog := func() ([]llama.LlamaModelInfo, bool, error) {
		for {
			catalog, err := m.syncCatalog(ctx, client)
			if err == nil {
				return catalog, true, nil
			}
			if ctx.Err() != nil {
				return nil, false, ctx.Err()
			}
			retry, dialogErr := ui.ConnectionError(
				ctx,
				client.ServerURL(),
				llamaConnectionErrorMessage(err),
			)
			if dialogErr != nil {
				return nil, false, dialogErr
			}
			if !retry {
				return nil, false, nil
			}
		}
	}

	catalog, ok, err := readCatalog()
	if err != nil || !ok {
		return err
	}
	for {
		action, err := ui.ShowModels(
			ctx,
			client.ServerURL(),
			catalog,
		)
		if err != nil {
			return err
		}
		if action.Kind == llamaManagerActionClose {
			return nil
		}
		var actionErr error
		switch action.Kind {
		case llamaManagerActionDownload:
			actionErr = m.downloadModel(ctx, ui, client)
		case llamaManagerActionModel:
			switch {
			case llamaModelIsLoaded(action.Model):
				actionErr = m.unloadModel(
					ctx,
					ui,
					client,
					action.Model,
				)
			case action.Model.Status.Value == llama.LlamaModelUnloaded:
				actionErr = m.loadModel(
					ctx,
					ui,
					client,
					catalog,
					action.Model,
				)
			default:
				ui.Notify(
					fmt.Sprintf(
						"%s is %s",
						action.Model.ID,
						action.Model.Status.Value,
					),
					"warning",
				)
			}
		}
		refreshed, open, refreshErr := readCatalog()
		if refreshErr != nil {
			return refreshErr
		}
		if !open {
			return nil
		}
		catalog = refreshed
		if actionErr != nil &&
			!isLlamaConnectionError(actionErr) {
			ui.Notify(actionErr.Error(), "error")
		}
	}
}

func (m *builtinLlamaManager) loadModel(
	ctx context.Context,
	ui llamaManagerUI,
	client *llama.LlamaClient,
	catalog []llama.LlamaModelInfo,
	target llama.LlamaModelInfo,
) error {
	var loaded []llama.LlamaModelInfo
	for _, model := range catalog {
		if model.ID != target.ID && llamaModelIsLoaded(model) {
			loaded = append(loaded, model)
		}
	}
	replace := false
	if len(loaded) > 0 {
		noun := "models are"
		if len(loaded) == 1 {
			noun = "model is"
		}
		choice, selected, err := ui.Select(
			ctx,
			fmt.Sprintf("%d %s loaded", len(loaded), noun),
			[]string{
				"Unload all and load",
				"Keep loaded and load",
				"Cancel",
			},
		)
		if err != nil || !selected || choice == "Cancel" {
			return err
		}
		replace = choice == "Unload all and load"
	}
	restoreLoaded := func() error {
		ui.Notify("Restoring previously loaded models", "")
		for _, model := range loaded {
			if _, err := client.LoadAndWait(
				ctx,
				model.ID,
				nil,
			); err != nil {
				return err
			}
		}
		_, err := m.syncCatalog(ctx, client)
		return err
	}
	if replace {
		for _, model := range loaded {
			if err := client.UnloadAndWait(ctx, model.ID); err != nil {
				return err
			}
		}
	}

	result, err := runLlamaWithProgress(
		ctx,
		ui,
		llamaProgressOptions[llama.LlamaModelInfo]{
			Title:          "Loading model",
			Model:          target.ID,
			InitialMessage: "Starting…",
			CancelTitle:    "Stop loading?",
			CancelMessage:  target.ID,
			CancelTimeout:  m.operationCancel,
			Run: func(
				operationCtx context.Context,
				update func(llama.LlamaProgress),
			) (llama.LlamaModelInfo, error) {
				return client.LoadAndWait(
					operationCtx,
					target.ID,
					update,
				)
			},
			Cancel: func(cancelCtx context.Context) error {
				return client.Unload(cancelCtx, target.ID)
			},
		},
	)
	if err != nil {
		if replace {
			// Preserve the initiating load failure, matching Pi's recovery
			// contract. Restoration is best-effort on this path.
			_ = restoreLoaded()
		}
		return err
	}
	if result.Cancelled {
		if replace {
			return restoreLoaded()
		}
		return nil
	}
	refreshed, err := m.syncCatalog(ctx, client)
	if err != nil {
		return err
	}
	loadedState := false
	for _, model := range refreshed {
		if model.ID == target.ID &&
			model.Status.Value == llama.LlamaModelLoaded {
			loadedState = true
			break
		}
	}
	if loadedState {
		ui.Notify("Loaded "+target.ID, "")
	} else {
		ui.Notify("Load started for "+target.ID, "")
	}
	return nil
}

func (m *builtinLlamaManager) unloadModel(
	ctx context.Context,
	ui llamaManagerUI,
	client *llama.LlamaClient,
	model llama.LlamaModelInfo,
) error {
	confirmed, err := ui.Confirm(
		ctx,
		"Unload model?",
		model.ID,
	)
	if err != nil || !confirmed {
		return err
	}
	if err := client.UnloadAndWait(ctx, model.ID); err != nil {
		return err
	}
	if _, err := m.syncCatalog(ctx, client); err != nil {
		return err
	}
	ui.Notify("Unloaded "+model.ID, "")
	return nil
}

func (m *builtinLlamaManager) downloadModel(
	ctx context.Context,
	ui llamaManagerUI,
	client *llama.LlamaClient,
) error {
	huggingFace, err := llama.NewHuggingFaceClient(
		m.findToken(),
		m.huggingFace,
	)
	if err != nil {
		return err
	}
	selected, ok, err := ui.SearchModels(
		ctx,
		huggingFace.Search,
	)
	if err != nil || !ok {
		return err
	}
	repository, quantization := parseLlamaHuggingFaceModel(selected)
	ui.ShowStatus("Loading model details", repository)
	details, err := huggingFace.Details(ctx, repository)
	if err != nil {
		return err
	}
	if details.Gating != llama.HuggingFaceGatingNone {
		approval := "Accept the access terms"
		if details.Gating == llama.HuggingFaceGatingManual {
			approval = "Manual approval is required"
		}
		choice, selected, err := ui.Select(
			ctx,
			fmt.Sprintf(
				"Hugging Face access required\n%s\n\n%s at:\n"+
					"https://huggingface.co/%s\n\n"+
					"The llama.cpp server needs HF_TOKEN with access.",
				details.ID,
				approval,
				details.ID,
			),
			[]string{"Continue", "Back"},
		)
		if err != nil || !selected || choice != "Continue" {
			return err
		}
	}
	if quantization == "" && len(details.Quantizations) > 0 {
		options := make(
			[]string,
			0,
			len(details.Quantizations),
		)
		for _, entry := range details.Quantizations {
			var description []string
			if entry.Size != nil {
				description = append(
					description,
					llama.FormatLlamaBytes(float64(*entry.Size)),
				)
			}
			if entry.Name == "Q4_K_M" {
				description = append(description, "recommended")
			}
			label := entry.Name
			if len(description) > 0 {
				label += " · " + strings.Join(description, " · ")
			}
			options = append(options, label)
		}
		choice, selected, err := ui.Select(
			ctx,
			"Select quantization\n"+details.ID,
			options,
		)
		if err != nil || !selected {
			return err
		}
		for index, option := range options {
			if option == choice {
				quantization = details.Quantizations[index].Name
				break
			}
		}
		if quantization == "" {
			return nil
		}
	}
	model := details.ID
	if quantization != "" {
		model += ":" + quantization
	}
	result, err := runLlamaWithProgress(
		ctx,
		ui,
		llamaProgressOptions[[]llama.LlamaModelInfo]{
			Title:          "Downloading model",
			Model:          model,
			InitialMessage: "Starting…",
			CancelTitle:    "Stop download?",
			CancelMessage:  model,
			CancelTimeout:  m.operationCancel,
			Run: func(
				operationCtx context.Context,
				update func(llama.LlamaProgress),
			) ([]llama.LlamaModelInfo, error) {
				return client.DownloadAndWait(
					operationCtx,
					model,
					update,
				)
			},
			Cancel: func(cancelCtx context.Context) error {
				return client.Unload(cancelCtx, model)
			},
		},
	)
	if err != nil || result.Cancelled {
		return err
	}
	if err := m.publishCatalog(
		ctx,
		client,
		result.Value,
	); err != nil {
		return err
	}
	ui.Notify("Downloaded "+model, "")
	return nil
}

type llamaProgressOptions[T any] struct {
	Title          string
	Model          string
	InitialMessage string
	CancelTitle    string
	CancelMessage  string
	CancelTimeout  time.Duration
	Run            func(
		context.Context,
		func(llama.LlamaProgress),
	) (T, error)
	Cancel func(context.Context) error
}

type llamaProgressResult[T any] struct {
	Value     T
	Cancelled bool
}

type llamaProgressOperationResult[T any] struct {
	value T
	err   error
}

func runLlamaWithProgress[T any](
	ctx context.Context,
	ui llamaManagerUI,
	options llamaProgressOptions[T],
) (llamaProgressResult[T], error) {
	var zero llamaProgressResult[T]
	if ui == nil || options.Run == nil {
		return zero, errors.New("Llama progress operation is required")
	}
	ctx = llamaContextOrBackground(ctx)
	operationCtx, cancelOperation := context.WithCancel(ctx)
	defer cancelOperation()
	updates := make(chan llama.LlamaProgress, 1)
	settled := make(chan llamaProgressOperationResult[T], 1)
	go func() {
		value, err := options.Run(
			operationCtx,
			func(progress llama.LlamaProgress) {
				select {
				case updates <- progress:
				default:
					select {
					case <-updates:
					default:
					}
					select {
					case updates <- progress:
					default:
					}
				}
			},
		)
		settled <- llamaProgressOperationResult[T]{
			value: value,
			err:   err,
		}
	}()

	state := llamaProgressState{
		Title:   options.Title,
		Model:   options.Model,
		Message: options.InitialMessage,
	}
	stop := ui.ShowProgress(state)
	for {
		select {
		case <-ctx.Done():
			cancelOperation()
			return zero, ctx.Err()
		case update := <-updates:
			state.Message = update.Message
			state.Ratio = cloneLlamaRatio(update.Ratio)
			state.Detail = update.Detail
			ui.UpdateProgress(state)
		case result := <-settled:
			select {
			case update := <-updates:
				state.Message = update.Message
				state.Ratio = cloneLlamaRatio(update.Ratio)
				state.Detail = update.Detail
				ui.UpdateProgress(state)
			default:
			}
			if result.err != nil {
				return zero, result.err
			}
			return llamaProgressResult[T]{Value: result.value}, nil
		case <-stop:
			confirmed, err := ui.Confirm(
				ctx,
				options.CancelTitle,
				options.CancelMessage,
			)
			if err != nil {
				cancelOperation()
				return zero, err
			}
			if !confirmed {
				stop = ui.ShowProgress(state)
				continue
			}
			var cancelErr error
			if options.Cancel != nil {
				timeout := options.CancelTimeout
				if timeout <= 0 {
					timeout = 15 * time.Second
				}
				cancelCtx, cancel := context.WithTimeout(ctx, timeout)
				cancelErr = options.Cancel(cancelCtx)
				cancel()
			}
			cancelOperation()
			select {
			case <-settled:
			case <-ctx.Done():
			}
			if cancelErr != nil {
				return zero, cancelErr
			}
			return llamaProgressResult[T]{Cancelled: true}, nil
		}
	}
}

func llamaModelIsLoaded(model llama.LlamaModelInfo) bool {
	return model.Status.Value == llama.LlamaModelLoaded ||
		model.Status.Value == llama.LlamaModelSleeping
}

func parseLlamaHuggingFaceModel(
	value string,
) (repository string, quantization string) {
	value = strings.TrimSpace(value)
	slash := strings.Index(value, "/")
	if slash < 0 {
		return value, ""
	}
	colon := strings.Index(value[slash+1:], ":")
	if colon < 0 {
		return value, ""
	}
	colon += slash + 1
	return value[:colon], value[colon+1:]
}

func isLlamaConnectionError(err error) bool {
	if err == nil ||
		errors.Is(err, context.Canceled) {
		return false
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return true
	}
	var urlError *url.Error
	if errors.As(err, &urlError) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"connection refused",
		"connection reset",
		"fetch failed",
		"network",
		"no such host",
		"timeout",
		"timed out",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func llamaConnectionErrorMessage(err error) string {
	if isLlamaConnectionError(err) {
		return "Could not connect to the server."
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

func sortedLlamaModels(
	models []llama.LlamaModelInfo,
) []llama.LlamaModelInfo {
	sorted := make([]llama.LlamaModelInfo, len(models))
	for index, model := range models {
		sorted[index] = cloneLlamaModelForUI(model)
	}
	sort.SliceStable(sorted, func(left, right int) bool {
		leftLoaded := sorted[left].Status.Value ==
			llama.LlamaModelLoaded
		rightLoaded := sorted[right].Status.Value ==
			llama.LlamaModelLoaded
		if leftLoaded != rightLoaded {
			return leftLoaded
		}
		return sorted[left].ID < sorted[right].ID
	})
	return sorted
}

func cloneLlamaModelForUI(
	model llama.LlamaModelInfo,
) llama.LlamaModelInfo {
	model.Aliases = append([]string(nil), model.Aliases...)
	model.Status.Args = append([]string(nil), model.Status.Args...)
	if model.Status.ExitCode != nil {
		exitCode := *model.Status.ExitCode
		model.Status.ExitCode = &exitCode
	}
	if model.Status.Progress != nil {
		progress := make(
			map[string]llama.LlamaTransferProgress,
			len(model.Status.Progress),
		)
		for name, value := range model.Status.Progress {
			progress[name] = value
		}
		model.Status.Progress = progress
	}
	model.Architecture.InputModalities = append(
		[]string(nil),
		model.Architecture.InputModalities...,
	)
	model.Architecture.OutputModalities = append(
		[]string(nil),
		model.Architecture.OutputModalities...,
	)
	if model.Meta.ContextWindow != nil {
		value := *model.Meta.ContextWindow
		model.Meta.ContextWindow = &value
	}
	if model.Meta.TrainingContext != nil {
		value := *model.Meta.TrainingContext
		model.Meta.TrainingContext = &value
	}
	if model.Meta.Size != nil {
		value := *model.Meta.Size
		model.Meta.Size = &value
	}
	return model
}

func cloneLlamaRatio(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func llamaContextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
