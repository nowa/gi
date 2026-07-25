package llama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	defaultLlamaRequestTimeout       = 15 * time.Second
	defaultLlamaUnloadPollInterval   = 100 * time.Millisecond
	defaultLlamaLoadPollInterval     = 250 * time.Millisecond
	defaultLlamaDownloadPollInterval = 500 * time.Millisecond
	maxLlamaResponseBytes            = 32 << 20
	maxLlamaSSELineBytes             = 4 << 20
)

// LlamaModelLifecycle is the router-owned lifecycle state for one model.
type LlamaModelLifecycle string

const (
	LlamaModelUnloaded    LlamaModelLifecycle = "unloaded"
	LlamaModelLoading     LlamaModelLifecycle = "loading"
	LlamaModelLoaded      LlamaModelLifecycle = "loaded"
	LlamaModelDownloading LlamaModelLifecycle = "downloading"
	LlamaModelSleeping    LlamaModelLifecycle = "sleeping"
)

// LlamaTransferProgress reports byte progress for one downloaded artifact.
type LlamaTransferProgress struct {
	Done  float64 `json:"done"`
	Total float64 `json:"total"`
}

// LlamaModelStatus is the typed lifecycle projection returned by llama.cpp.
type LlamaModelStatus struct {
	Value    LlamaModelLifecycle              `json:"value"`
	Args     []string                         `json:"args,omitempty"`
	Failed   bool                             `json:"failed,omitempty"`
	ExitCode *int                             `json:"exit_code,omitempty"`
	Progress map[string]LlamaTransferProgress `json:"progress,omitempty"`
}

type LlamaModelArchitecture struct {
	InputModalities  []string `json:"input_modalities,omitempty"`
	OutputModalities []string `json:"output_modalities,omitempty"`
}

type LlamaModelMeta struct {
	ContextWindow      *int   `json:"n_ctx,omitempty"`
	TrainingContext    *int   `json:"n_ctx_train,omitempty"`
	Size               *int64 `json:"size,omitempty"`
	QuantizationFormat string `json:"ftype,omitempty"`
}

// LlamaModelInfo is the canonical management-plane model record.
type LlamaModelInfo struct {
	ID           string                 `json:"id"`
	Aliases      []string               `json:"aliases,omitempty"`
	Status       LlamaModelStatus       `json:"status"`
	Architecture LlamaModelArchitecture `json:"architecture,omitempty"`
	Source       string                 `json:"source,omitempty"`
	Meta         LlamaModelMeta         `json:"meta,omitempty"`
}

// LlamaModelEvent is one router SSE event. Data stays raw at the transport
// boundary and is decoded into operation-specific state by wait methods.
type LlamaModelEvent struct {
	Model string          `json:"model"`
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// LlamaProgress is a detached user-facing progress update.
type LlamaProgress struct {
	Message string
	Ratio   *float64
	Detail  string
}

type LlamaClientOptions struct {
	HTTPClient   llm.HTTPDoer
	Timeout      time.Duration
	PollInterval time.Duration
}

type LlamaListOptions struct {
	Reload bool
}

// LlamaHTTPError preserves the management endpoint status and server message.
type LlamaHTTPError struct {
	StatusCode int
	Message    string
}

func (e *LlamaHTTPError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// LlamaClient is a context-aware client for the llama.cpp router management
// API. It owns no model state; callers publish returned catalogs explicitly.
type LlamaClient struct {
	serverURL   string
	apiKey      string
	httpClient  llm.HTTPDoer
	timeout     time.Duration
	pollTimeout time.Duration
}

func NewLlamaClient(
	serverURL string,
	apiKey string,
	options ...LlamaClientOptions,
) (*LlamaClient, error) {
	if len(options) > 1 {
		return nil, errors.New("llama client accepts at most one options value")
	}
	normalized, err := NormalizeLlamaServerURL(serverURL)
	if err != nil {
		return nil, err
	}
	var selected LlamaClientOptions
	if len(options) == 1 {
		selected = options[0]
	}
	if selected.HTTPClient == nil {
		selected.HTTPClient = http.DefaultClient
	}
	if selected.Timeout <= 0 {
		selected.Timeout = defaultLlamaRequestTimeout
	}
	return &LlamaClient{
		serverURL:   normalized,
		apiKey:      strings.TrimSpace(apiKey),
		httpClient:  selected.HTTPClient,
		timeout:     selected.Timeout,
		pollTimeout: selected.PollInterval,
	}, nil
}

func (c *LlamaClient) ServerURL() string {
	if c == nil {
		return ""
	}
	return c.serverURL
}

func NormalizeLlamaServerURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("Server URL must use http or https")
	}
	if parsed.Host == "" {
		return "", errors.New("Server URL requires a host")
	}
	parsed.Fragment = ""
	parsed.RawFragment = ""
	parsed.RawQuery = ""
	path := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(path, "/v1") {
		path = strings.TrimSuffix(path, "/v1")
	}
	parsed.Path = strings.TrimRight(path, "/")
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func LlamaInferenceURL(serverURL string) (string, error) {
	normalized, err := NormalizeLlamaServerURL(serverURL)
	if err != nil {
		return "", err
	}
	return normalized + "/v1", nil
}

func (c *LlamaClient) List(
	ctx context.Context,
	options LlamaListOptions,
) ([]LlamaModelInfo, error) {
	path := "/models"
	if options.Reload {
		path += "?reload=1"
	}
	payload, err := c.request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, errors.New("llama.cpp returned an invalid model catalog")
	}
	rawModels := envelope["data"]
	if len(rawModels) == 0 ||
		len(bytes.TrimSpace(rawModels)) == 0 ||
		bytes.TrimSpace(rawModels)[0] != '[' {
		return nil, errors.New("llama.cpp returned an invalid model catalog")
	}
	var records []json.RawMessage
	if err := json.Unmarshal(rawModels, &records); err != nil {
		return nil, errors.New("llama.cpp returned an invalid model catalog")
	}
	models := make([]LlamaModelInfo, 0, len(records))
	for _, record := range records {
		var model LlamaModelInfo
		if err := json.Unmarshal(record, &model); err != nil ||
			model.ID == "" ||
			model.Status.Value == "" {
			return nil, errors.New(
				"Server is not running in llama.cpp router mode",
			)
		}
		model.Aliases = append([]string(nil), model.Aliases...)
		model.Status.Args = append([]string(nil), model.Status.Args...)
		model.Status.Progress = cloneLlamaTransferProgress(
			model.Status.Progress,
		)
		model.Architecture.InputModalities = append(
			[]string(nil),
			model.Architecture.InputModalities...,
		)
		model.Architecture.OutputModalities = append(
			[]string(nil),
			model.Architecture.OutputModalities...,
		)
		models = append(models, model)
	}
	return models, nil
}

func (c *LlamaClient) Load(ctx context.Context, model string) error {
	_, err := c.request(
		ctx,
		http.MethodPost,
		"/models/load",
		map[string]string{"model": model},
	)
	return err
}

func (c *LlamaClient) Unload(ctx context.Context, model string) error {
	_, err := c.request(
		ctx,
		http.MethodPost,
		"/models/unload",
		map[string]string{"model": model},
	)
	return err
}

func (c *LlamaClient) UnloadAndWait(
	ctx context.Context,
	model string,
) error {
	if err := c.Unload(ctx, model); err != nil {
		return err
	}
	for {
		models, err := c.List(ctx, LlamaListOptions{})
		if err != nil {
			return err
		}
		entry, ok := findLlamaModel(models, model)
		if !ok || entry.Status.Value == LlamaModelUnloaded {
			return nil
		}
		if err := sleepLlamaContext(
			ctx,
			c.pollInterval(defaultLlamaUnloadPollInterval),
		); err != nil {
			return err
		}
	}
}

func (c *LlamaClient) Download(
	ctx context.Context,
	model string,
) error {
	_, err := c.request(
		ctx,
		http.MethodPost,
		"/models",
		map[string]string{"model": model},
	)
	return err
}

func (c *LlamaClient) Watch(
	ctx context.Context,
	onEvent func(LlamaModelEvent),
) error {
	if c == nil {
		return errors.New("llama client is required")
	}
	ctx = llamaContext(ctx)
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.serverURL+"/models/sse",
		nil,
	)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		defer response.Body.Close()
		payload, readErr := readLlamaResponseBody(response.Body)
		if readErr != nil {
			return readErr
		}
		return &LlamaHTTPError{
			StatusCode: response.StatusCode,
			Message: fmt.Sprintf(
				"llama.cpp SSE returned HTTP %d: %s",
				response.StatusCode,
				llamaErrorMessage(payload, http.StatusText(response.StatusCode)),
			),
		}
	}
	defer response.Body.Close()
	return readLlamaEventStream(response.Body, onEvent)
}

func (c *LlamaClient) LoadAndWait(
	ctx context.Context,
	model string,
	onProgress func(LlamaProgress),
) (LlamaModelInfo, error) {
	ctx = llamaContext(ctx)
	watchCtx, stopWatcher := context.WithCancel(ctx)
	defer stopWatcher()
	state := &llamaLoadWatchState{model: model}
	go func() {
		_ = c.Watch(watchCtx, state.record)
	}()

	if err := c.Load(ctx, model); err != nil {
		return LlamaModelInfo{}, err
	}
	emitLlamaProgress(onProgress, LlamaProgress{
		Message: "Loading model",
	})
	for {
		models, err := c.List(ctx, LlamaListOptions{})
		if err != nil {
			return LlamaModelInfo{}, err
		}
		eventLoaded, eventError, progress := state.snapshot()
		for _, update := range progress {
			emitLlamaProgress(onProgress, update)
		}
		entry, exists := findLlamaModel(models, model)
		if exists && entry.Status.Value == LlamaModelLoaded {
			return entry, nil
		}
		if eventLoaded && !exists {
			return LlamaModelInfo{
				ID: model,
				Status: LlamaModelStatus{
					Value: LlamaModelLoaded,
				},
			}, nil
		}
		if (exists && entry.Status.Failed) || eventError != "" {
			if exists && entry.Status.ExitCode != nil {
				return LlamaModelInfo{}, fmt.Errorf(
					"Model exited with code %d",
					*entry.Status.ExitCode,
				)
			}
			if eventError == "" {
				eventError = "Model failed to load"
			}
			return LlamaModelInfo{}, errors.New(eventError)
		}
		if err := sleepLlamaContext(
			ctx,
			c.pollInterval(defaultLlamaLoadPollInterval),
		); err != nil {
			return LlamaModelInfo{}, err
		}
	}
}

func (c *LlamaClient) DownloadAndWait(
	ctx context.Context,
	model string,
	onProgress func(LlamaProgress),
) ([]LlamaModelInfo, error) {
	ctx = llamaContext(ctx)
	watchCtx, stopWatcher := context.WithCancel(ctx)
	defer stopWatcher()
	state := &llamaDownloadWatchState{model: model}
	go func() {
		_ = c.Watch(watchCtx, state.record)
	}()

	if err := c.Download(ctx, model); err != nil {
		return nil, err
	}
	emitLlamaProgress(onProgress, LlamaProgress{
		Message: "Downloading model",
	})
	sawDownloading := false
	polls := 0
	for {
		finished, failure, eventSawDownloading, progress :=
			state.snapshot()
		sawDownloading = sawDownloading || eventSawDownloading
		for _, update := range progress {
			emitLlamaProgress(onProgress, update)
		}
		if failure != "" {
			return nil, errors.New(failure)
		}
		models, err := c.List(ctx, LlamaListOptions{})
		if err != nil {
			return nil, err
		}
		polls++
		finishedAfterList, failureAfterList, sawAfterList, progressAfterList :=
			state.snapshot()
		finished = finished || finishedAfterList
		sawDownloading = sawDownloading || sawAfterList
		for _, update := range progressAfterList {
			emitLlamaProgress(onProgress, update)
		}
		if failureAfterList != "" {
			return nil, errors.New(failureAfterList)
		}
		entry, exists := findLlamaModel(models, model)
		if exists && entry.Status.Value == LlamaModelDownloading {
			sawDownloading = true
			if update, ok := parseLlamaTransferProgress(
				entry.Status.Progress,
			); ok {
				emitLlamaProgress(onProgress, update)
			}
		} else if finished || exists && (sawDownloading || polls >= 2) {
			return c.List(ctx, LlamaListOptions{Reload: true})
		}
		if err := sleepLlamaContext(
			ctx,
			c.pollInterval(defaultLlamaDownloadPollInterval),
		); err != nil {
			return nil, err
		}
	}
}

func (c *LlamaClient) request(
	ctx context.Context,
	method string,
	path string,
	body any,
) ([]byte, error) {
	if c == nil {
		return nil, errors.New("llama client is required")
	}
	ctx = llamaContext(ctx)
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(
		requestCtx,
		method,
		c.serverURL+path,
		reader,
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	payload, err := readLlamaResponseBody(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		return nil, &LlamaHTTPError{
			StatusCode: response.StatusCode,
			Message: llamaErrorMessage(
				payload,
				fmt.Sprintf(
					"llama.cpp returned HTTP %d",
					response.StatusCode,
				),
			),
		}
	}
	return payload, nil
}

func readLlamaResponseBody(reader io.Reader) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(
		reader,
		maxLlamaResponseBytes+1,
	))
	if err != nil {
		return nil, err
	}
	if len(payload) > maxLlamaResponseBytes {
		return nil, errors.New("llama.cpp response exceeds size limit")
	}
	return payload, nil
}

func llamaErrorMessage(payload []byte, fallback string) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(payload, &envelope) == nil &&
		envelope.Error.Message != "" {
		return envelope.Error.Message
	}
	return fallback
}

func readLlamaEventStream(
	reader io.Reader,
	onEvent func(LlamaModelEvent),
) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLlamaSSELineBytes)
	var dataLines []string
	flush := func() {
		if len(dataLines) == 0 {
			return
		}
		data := strings.Join(dataLines, "\n")
		dataLines = nil
		var event LlamaModelEvent
		if json.Unmarshal([]byte(data), &event) != nil ||
			event.Model == "" ||
			event.Event == "" {
			return
		}
		if onEvent != nil {
			onEvent(event)
		}
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(
				dataLines,
				strings.TrimLeft(strings.TrimPrefix(line, "data:"), " \t"),
			)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	flush()
	return nil
}

type llamaLoadWatchState struct {
	model string

	mu       sync.Mutex
	loaded   bool
	failure  string
	progress []LlamaProgress
}

func (s *llamaLoadWatchState) record(event LlamaModelEvent) {
	if s == nil ||
		event.Model != s.model ||
		event.Event != "model_status" &&
			event.Event != "status_change" {
		return
	}
	var data struct {
		Status   LlamaModelLifecycle `json:"status"`
		Progress *struct {
			Stages  []string `json:"stages"`
			Current string   `json:"current"`
			Stage   string   `json:"stage"`
			Value   *float64 `json:"value"`
		} `json:"progress"`
	}
	if json.Unmarshal(event.Data, &data) != nil {
		return
	}
	update, hasProgress := parseLlamaLoadProgress(data.Progress)
	s.mu.Lock()
	switch data.Status {
	case LlamaModelLoaded:
		s.loaded = true
	case LlamaModelUnloaded:
		s.failure = "Model failed to load"
	}
	if hasProgress {
		s.progress = append(s.progress, update)
	}
	s.mu.Unlock()
}

func (s *llamaLoadWatchState) snapshot() (
	bool,
	string,
	[]LlamaProgress,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	progress := append([]LlamaProgress(nil), s.progress...)
	s.progress = nil
	return s.loaded, s.failure, progress
}

type llamaDownloadWatchState struct {
	model string

	mu             sync.Mutex
	finished       bool
	failure        string
	sawDownloading bool
	progress       []LlamaProgress
}

func (s *llamaDownloadWatchState) record(event LlamaModelEvent) {
	if s == nil || event.Model != s.model {
		return
	}
	var update LlamaProgress
	var hasProgress bool
	switch event.Event {
	case "download_finished":
		s.mu.Lock()
		s.finished = true
		s.mu.Unlock()
		return
	case "download_failed":
		s.mu.Lock()
		s.failure = llamaErrorMessage(event.Data, "Download failed")
		s.mu.Unlock()
		return
	case "download_progress":
		update, hasProgress = parseLlamaDownloadProgress(event.Data)
	default:
		return
	}
	s.mu.Lock()
	s.sawDownloading = true
	if hasProgress {
		s.progress = append(s.progress, update)
	}
	s.mu.Unlock()
}

func (s *llamaDownloadWatchState) snapshot() (
	bool,
	string,
	bool,
	[]LlamaProgress,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	progress := append([]LlamaProgress(nil), s.progress...)
	s.progress = nil
	return s.finished, s.failure, s.sawDownloading, progress
}

func parseLlamaLoadProgress(progress *struct {
	Stages  []string `json:"stages"`
	Current string   `json:"current"`
	Stage   string   `json:"stage"`
	Value   *float64 `json:"value"`
}) (LlamaProgress, bool) {
	if progress == nil {
		return LlamaProgress{}, false
	}
	stage := progress.Current
	if stage == "" {
		stage = progress.Stage
	}
	var ratio *float64
	if progress.Value != nil {
		value := max(0, min(1, *progress.Value))
		ratio = &value
	}
	if stage != "" && len(progress.Stages) > 0 {
		for index, candidate := range progress.Stages {
			if candidate != stage {
				continue
			}
			stageRatio := 0.0
			if ratio != nil {
				stageRatio = *ratio
			}
			value := (float64(index) + stageRatio) /
				float64(len(progress.Stages))
			ratio = &value
			break
		}
	}
	message := "Loading model"
	if stage != "" {
		message = "Loading " + strings.ReplaceAll(stage, "_", " ")
	}
	return LlamaProgress{Message: message, Ratio: ratio}, true
}

func parseLlamaDownloadProgress(
	raw json.RawMessage,
) (LlamaProgress, bool) {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(raw, &envelope) != nil {
		return LlamaProgress{}, false
	}
	filesRaw := raw
	if nested := envelope["progress"]; len(nested) > 0 {
		filesRaw = nested
	}
	var files map[string]struct {
		Done  *float64 `json:"done"`
		Total *float64 `json:"total"`
	}
	if json.Unmarshal(filesRaw, &files) != nil {
		return LlamaProgress{}, false
	}
	transfers := make(map[string]LlamaTransferProgress, len(files))
	for name, file := range files {
		if file.Done == nil || file.Total == nil {
			continue
		}
		transfers[name] = LlamaTransferProgress{
			Done:  *file.Done,
			Total: *file.Total,
		}
	}
	return parseLlamaTransferProgress(transfers)
}

func parseLlamaTransferProgress(
	files map[string]LlamaTransferProgress,
) (LlamaProgress, bool) {
	var done, total float64
	for _, file := range files {
		done += file.Done
		total += file.Total
	}
	if total <= 0 {
		return LlamaProgress{}, false
	}
	ratio := done / total
	return LlamaProgress{
		Message: "Downloading model",
		Ratio:   &ratio,
		Detail: FormatLlamaBytes(done) +
			" / " + FormatLlamaBytes(total),
	}, true
}

func FormatLlamaBytes(bytes float64) string {
	if bytes < 1024 {
		return strconv.FormatFloat(bytes, 'f', -1, 64) + " B"
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	value := bytes / 1024
	unit := units[0]
	for index := 1; index < len(units) && value >= 1024; index++ {
		value /= 1024
		unit = units[index]
	}
	precision := 2
	if value >= 10 {
		precision = 1
	}
	return strconv.FormatFloat(value, 'f', precision, 64) + " " + unit
}

func cloneLlamaTransferProgress(
	progress map[string]LlamaTransferProgress,
) map[string]LlamaTransferProgress {
	if progress == nil {
		return nil
	}
	cloned := make(map[string]LlamaTransferProgress, len(progress))
	for name, value := range progress {
		cloned[name] = value
	}
	return cloned
}

func findLlamaModel(
	models []LlamaModelInfo,
	id string,
) (LlamaModelInfo, bool) {
	for _, model := range models {
		if model.ID == id {
			return model, true
		}
	}
	return LlamaModelInfo{}, false
}

func emitLlamaProgress(
	onProgress func(LlamaProgress),
	progress LlamaProgress,
) {
	if onProgress != nil {
		onProgress(progress)
	}
}

func (c *LlamaClient) pollInterval(fallback time.Duration) time.Duration {
	if c != nil && c.pollTimeout > 0 {
		return c.pollTimeout
	}
	return fallback
}

func sleepLlamaContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func llamaContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
