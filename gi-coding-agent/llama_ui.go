package gicodingagent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	llama "github.com/nowa/gi/gi-coding-agent/internal/llama"
	gitui "github.com/nowa/gi/gi-tui"
)

const llamaDownloadSelectionValue = "\x00download"

var llamaHuggingFaceModelPattern = regexp.MustCompile(
	`^[^/\s]+/[^:\s]+(?::[^\s:]+)?$`,
)

type llamaManagerScreenKind string

const (
	llamaManagerScreenModels   llamaManagerScreenKind = "models"
	llamaManagerScreenSelect   llamaManagerScreenKind = "select"
	llamaManagerScreenSearch   llamaManagerScreenKind = "search"
	llamaManagerScreenStatus   llamaManagerScreenKind = "status"
	llamaManagerScreenProgress llamaManagerScreenKind = "progress"
)

type llamaManagerViewResult struct {
	action   llamaManagerAction
	value    string
	selected bool
}

type llamaManagerScreen struct {
	id       uint64
	kind     llamaManagerScreenKind
	title    string
	server   string
	message  string
	list     *gitui.SelectList
	result   chan llamaManagerViewResult
	input    *gitui.Input
	query    string
	results  []llama.HuggingFaceModel
	filtered []llama.HuggingFaceModel
	selected int
	status   string
	search   func(context.Context, string) (
		[]llama.HuggingFaceModel,
		error,
	)
	searchCtx context.Context
	progress  llamaProgressState
	stop      chan struct{}
}

// llamaManagerView owns one mutex-protected presentation snapshot. Network
// searches publish only when their screen generation is still current, so a
// late response cannot overwrite a newer dialog or a closed manager.
type llamaManagerView struct {
	focus gitui.FocusState

	mu            sync.RWMutex
	screen        llamaManagerScreen
	nextScreenID  uint64
	searchCache   map[string][]llama.HuggingFaceModel
	searchTimer   *time.Timer
	searchCancel  context.CancelFunc
	requestRender func()
	notify        func(string, string)
	closed        chan struct{}
	closeOnce     sync.Once
}

func newLlamaManagerView(
	requestRender func(),
	notify func(string, string),
) *llamaManagerView {
	return &llamaManagerView{
		searchCache:   map[string][]llama.HuggingFaceModel{},
		requestRender: requestRender,
		notify:        notify,
		closed:        make(chan struct{}),
	}
}

func (v *llamaManagerView) Close() {
	if v == nil {
		return
	}
	v.closeOnce.Do(func() {
		v.mu.Lock()
		timer := v.searchTimer
		cancel := v.searchCancel
		v.searchTimer = nil
		v.searchCancel = nil
		v.mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		if cancel != nil {
			cancel()
		}
		close(v.closed)
	})
}

func (v *llamaManagerView) Focused() bool {
	return v != nil && v.focus.Focused()
}

func (v *llamaManagerView) SetFocused(focused bool) {
	if v == nil {
		return
	}
	v.focus.SetFocused(focused)
	v.mu.RLock()
	input := v.screen.input
	v.mu.RUnlock()
	if input != nil {
		input.SetFocused(focused)
	}
}

func (*llamaManagerView) Invalidate() {}

func (v *llamaManagerView) ShowModels(
	ctx context.Context,
	serverURL string,
	models []llama.LlamaModelInfo,
) (llamaManagerAction, error) {
	result := make(chan llamaManagerViewResult, 1)
	sorted := sortedLlamaModels(models)
	byID := make(map[string]llama.LlamaModelInfo, len(sorted))
	items := make([]gitui.SelectItem, 0, len(sorted)+1)
	for _, model := range sorted {
		byID[model.ID] = cloneLlamaModelForUI(model)
		items = append(items, gitui.SelectItem{
			Value:       model.ID,
			Label:       model.ID,
			Description: llamaModelDescription(model),
		})
	}
	items = append(items, gitui.SelectItem{
		Value:       llamaDownloadSelectionValue,
		Label:       "Download model…",
		Description: "Hugging Face owner/repository[:quant]",
	})
	list := gitui.NewSelectList(
		items,
		min(len(items), 12),
		llamaSelectListTheme(),
		gitui.SelectListLayoutOptions{
			MinPrimaryColumnWidth: 36,
			MaxPrimaryColumnWidth: 56,
		},
	)
	list.OnSelect = func(item gitui.SelectItem) {
		if item.Value == llamaDownloadSelectionValue {
			sendLlamaViewResult(result, llamaManagerViewResult{
				action: llamaManagerAction{
					Kind: llamaManagerActionDownload,
				},
			})
			return
		}
		model, ok := byID[item.Value]
		if !ok {
			return
		}
		sendLlamaViewResult(result, llamaManagerViewResult{
			action: llamaManagerAction{
				Kind:  llamaManagerActionModel,
				Model: cloneLlamaModelForUI(model),
			},
		})
	}
	list.OnCancel = func() {
		sendLlamaViewResult(result, llamaManagerViewResult{
			action: llamaManagerAction{Kind: llamaManagerActionClose},
		})
	}
	v.replaceScreen(llamaManagerScreen{
		kind:   llamaManagerScreenModels,
		title:  "llama.cpp models",
		server: serverURL,
		list:   list,
		result: result,
	})
	response, err := v.waitResult(ctx, result)
	if err != nil {
		return llamaManagerAction{}, err
	}
	return response.action, nil
}

func (v *llamaManagerView) Select(
	ctx context.Context,
	title string,
	options []string,
) (string, bool, error) {
	if len(options) == 0 {
		return "", false, errors.New(
			"Llama selection requires options",
		)
	}
	result := make(chan llamaManagerViewResult, 1)
	items := make([]gitui.SelectItem, 0, len(options))
	for _, option := range options {
		items = append(items, gitui.SelectItem{
			Value: option,
			Label: option,
		})
	}
	list := gitui.NewSelectList(
		items,
		min(len(items), 12),
		llamaSelectListTheme(),
	)
	list.OnSelect = func(item gitui.SelectItem) {
		sendLlamaViewResult(result, llamaManagerViewResult{
			value:    item.Value,
			selected: true,
		})
	}
	list.OnCancel = func() {
		sendLlamaViewResult(result, llamaManagerViewResult{})
	}
	v.replaceScreen(llamaManagerScreen{
		kind:   llamaManagerScreenSelect,
		title:  title,
		list:   list,
		result: result,
	})
	response, err := v.waitResult(ctx, result)
	return response.value, response.selected, err
}

func (v *llamaManagerView) Confirm(
	ctx context.Context,
	title string,
	message string,
) (bool, error) {
	choice, selected, err := v.Select(
		ctx,
		strings.TrimSpace(title+"\n"+message),
		[]string{"Yes", "No"},
	)
	return selected && choice == "Yes", err
}

func (v *llamaManagerView) ConnectionError(
	ctx context.Context,
	serverURL string,
	message string,
) (bool, error) {
	choice, selected, err := v.Select(
		ctx,
		fmt.Sprintf(
			"llama.cpp unavailable\n%s\n\n%s",
			serverURL,
			message,
		),
		[]string{"Retry", "Close"},
	)
	return selected && choice == "Retry", err
}

func (v *llamaManagerView) SearchModels(
	ctx context.Context,
	search func(context.Context, string) (
		[]llama.HuggingFaceModel,
		error,
	),
) (string, bool, error) {
	if search == nil {
		return "", false, errors.New(
			"Hugging Face search is required",
		)
	}
	result := make(chan llamaManagerViewResult, 1)
	input := gitui.NewInput("owner/repository[:quant]")
	input.OnChange = func(value string) {
		v.scheduleSearchForInput(
			input,
			strings.TrimSpace(value),
		)
	}
	screen := llamaManagerScreen{
		kind:      llamaManagerScreenSearch,
		title:     "Download model",
		result:    result,
		input:     input,
		status:    "Type at least 2 characters",
		search:    search,
		searchCtx: llamaContextOrBackground(ctx),
	}
	v.replaceScreen(screen)
	input.SetFocused(v.Focused())
	response, err := v.waitResult(ctx, result)
	return response.value, response.selected, err
}

func (v *llamaManagerView) ShowStatus(
	title string,
	message string,
) {
	v.replaceScreen(llamaManagerScreen{
		kind:    llamaManagerScreenStatus,
		title:   title,
		message: message,
	})
}

func (v *llamaManagerView) ShowProgress(
	state llamaProgressState,
) <-chan struct{} {
	stop := make(chan struct{}, 1)
	state.Ratio = cloneLlamaRatio(state.Ratio)
	v.replaceScreen(llamaManagerScreen{
		kind:     llamaManagerScreenProgress,
		title:    state.Title,
		progress: state,
		stop:     stop,
	})
	return stop
}

func (v *llamaManagerView) UpdateProgress(
	state llamaProgressState,
) {
	if v == nil {
		return
	}
	state.Ratio = cloneLlamaRatio(state.Ratio)
	v.mu.Lock()
	if v.screen.kind != llamaManagerScreenProgress ||
		v.screen.progress.Model != state.Model ||
		v.screen.progress.Title != state.Title {
		v.mu.Unlock()
		return
	}
	v.screen.progress = state
	v.mu.Unlock()
	v.render()
}

func (v *llamaManagerView) Notify(
	message string,
	level string,
) {
	if v != nil && v.notify != nil {
		v.notify(message, level)
	}
}

func (v *llamaManagerView) replaceScreen(
	screen llamaManagerScreen,
) uint64 {
	if v == nil {
		return 0
	}
	v.mu.Lock()
	timer := v.searchTimer
	cancel := v.searchCancel
	v.searchTimer = nil
	v.searchCancel = nil
	v.nextScreenID++
	screen.id = v.nextScreenID
	if screen.input != nil {
		screen.input.SetFocused(v.Focused())
	}
	v.screen = screen
	v.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
	if cancel != nil {
		cancel()
	}
	v.render()
	return screen.id
}

func (v *llamaManagerView) waitResult(
	ctx context.Context,
	result <-chan llamaManagerViewResult,
) (llamaManagerViewResult, error) {
	ctx = llamaContextOrBackground(ctx)
	select {
	case response := <-result:
		return response, nil
	case <-ctx.Done():
		return llamaManagerViewResult{}, ctx.Err()
	case <-v.closed:
		return llamaManagerViewResult{}, context.Canceled
	}
}

func (v *llamaManagerView) scheduleSearchForInput(
	input *gitui.Input,
	query string,
) {
	if v == nil || input == nil {
		return
	}
	v.mu.RLock()
	screenID := v.screen.id
	active := v.screen.kind == llamaManagerScreenSearch &&
		v.screen.input == input
	v.mu.RUnlock()
	if active {
		v.scheduleSearch(screenID, query)
	}
}

func (v *llamaManagerView) scheduleSearch(
	screenID uint64,
	query string,
) {
	if v == nil {
		return
	}
	v.mu.Lock()
	if v.screen.id != screenID ||
		v.screen.kind != llamaManagerScreenSearch {
		v.mu.Unlock()
		return
	}
	timer := v.searchTimer
	cancel := v.searchCancel
	v.searchTimer = nil
	v.searchCancel = nil
	v.screen.query = query
	v.screen.selected = 0
	cached, cachedOK := v.searchCache[strings.ToLower(query)]
	switch {
	case len(query) < 2:
		v.screen.status = "Type at least 2 characters"
		v.filterSearchResultsLocked()
	case cachedOK:
		results := cloneHuggingFaceModels(cached)
		v.screen.results = results
		if len(results) == 0 {
			v.screen.status = "No GGUF models found"
		} else {
			v.screen.status = ""
		}
		v.filterSearchResultsLocked()
	default:
		v.screen.status = "Searching Hugging Face…"
		v.filterSearchResultsLocked()
		v.searchTimer = time.AfterFunc(
			500*time.Millisecond,
			func() {
				v.runSearch(screenID, query)
			},
		)
	}
	v.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
	if cancel != nil {
		cancel()
	}
	v.render()
}

func (v *llamaManagerView) runSearch(
	screenID uint64,
	query string,
) {
	v.mu.Lock()
	if v.screen.id != screenID ||
		v.screen.kind != llamaManagerScreenSearch ||
		v.screen.query != query {
		v.mu.Unlock()
		return
	}
	search := v.screen.search
	baseCtx := v.screen.searchCtx
	requestCtx, cancel := context.WithCancel(
		llamaContextOrBackground(baseCtx),
	)
	v.searchCancel = cancel
	v.searchTimer = nil
	v.mu.Unlock()

	results, err := search(requestCtx, query)
	results = cloneHuggingFaceModels(results)
	v.mu.Lock()
	if err == nil {
		v.searchCache[strings.ToLower(query)] =
			cloneHuggingFaceModels(results)
	}
	if v.screen.id != screenID ||
		v.screen.kind != llamaManagerScreenSearch ||
		v.screen.query != query ||
		requestCtx.Err() != nil {
		v.mu.Unlock()
		cancel()
		return
	}
	v.searchCancel = nil
	v.screen.results = results
	v.screen.selected = 0
	switch {
	case err != nil:
		v.screen.results = nil
		v.screen.status = err.Error()
	case len(results) == 0:
		v.screen.status = "No GGUF models found"
	default:
		v.screen.status = ""
	}
	v.filterSearchResultsLocked()
	v.mu.Unlock()
	cancel()
	v.render()
}

func (v *llamaManagerView) filterSearchResultsLocked() {
	query := v.screen.query
	if query == "" {
		v.screen.filtered = cloneHuggingFaceModels(
			v.screen.results,
		)
	} else {
		matches := gitui.FuzzyFilter(
			v.screen.results,
			query,
			func(model llama.HuggingFaceModel) string {
				return model.ID
			},
		)
		ids := make(map[string]struct{}, len(matches))
		for _, model := range matches {
			ids[model.ID] = struct{}{}
		}
		v.screen.filtered = v.screen.filtered[:0]
		for _, model := range v.screen.results {
			if _, ok := ids[model.ID]; ok {
				v.screen.filtered = append(
					v.screen.filtered,
					model,
				)
			}
		}
	}
	v.screen.selected = min(
		v.screen.selected,
		max(0, len(v.screen.filtered)-1),
	)
}

func (v *llamaManagerView) HandleInput(input string) {
	if v == nil {
		return
	}
	v.mu.RLock()
	screen := v.screen
	v.mu.RUnlock()
	switch screen.kind {
	case llamaManagerScreenModels, llamaManagerScreenSelect:
		if screen.list != nil {
			screen.list.HandleInput(input)
		}
	case llamaManagerScreenSearch:
		v.handleSearchInput(screen.id, input)
	case llamaManagerScreenProgress:
		if gitui.GetKeybindings().Matches(
			input,
			"tui.select.cancel",
		) {
			select {
			case screen.stop <- struct{}{}:
			default:
			}
		}
	}
	v.render()
}

func (v *llamaManagerView) handleSearchInput(
	screenID uint64,
	input string,
) {
	kb := gitui.GetKeybindings()
	v.mu.Lock()
	if v.screen.id != screenID ||
		v.screen.kind != llamaManagerScreenSearch {
		v.mu.Unlock()
		return
	}
	switch {
	case kb.Matches(input, "tui.select.up"):
		if len(v.screen.filtered) > 0 {
			if v.screen.selected == 0 {
				v.screen.selected = len(v.screen.filtered) - 1
			} else {
				v.screen.selected--
			}
		}
		v.mu.Unlock()
		return
	case kb.Matches(input, "tui.select.down"):
		if len(v.screen.filtered) > 0 {
			v.screen.selected =
				(v.screen.selected + 1) %
					len(v.screen.filtered)
		}
		v.mu.Unlock()
		return
	case kb.Matches(input, "tui.select.confirm"):
		query := v.screen.query
		selected := ""
		if llamaHuggingFaceModelPattern.MatchString(query) {
			selected = query
		} else if v.screen.selected >= 0 &&
			v.screen.selected < len(v.screen.filtered) {
			selected = v.screen.filtered[v.screen.selected].ID
		}
		result := v.screen.result
		v.mu.Unlock()
		if selected != "" {
			sendLlamaViewResult(result, llamaManagerViewResult{
				value:    selected,
				selected: true,
			})
		}
		return
	case kb.Matches(input, "tui.select.cancel"):
		result := v.screen.result
		v.mu.Unlock()
		sendLlamaViewResult(result, llamaManagerViewResult{})
		return
	default:
		textInput := v.screen.input
		v.mu.Unlock()
		if textInput != nil {
			textInput.HandleInput(input)
		}
	}
}

func (v *llamaManagerView) Render(width int) []string {
	if v == nil {
		return nil
	}
	width = max(24, width)
	v.mu.RLock()
	screen := v.screen
	screen.filtered = cloneHuggingFaceModels(screen.filtered)
	screen.progress.Ratio = cloneLlamaRatio(screen.progress.Ratio)
	v.mu.RUnlock()

	var body []string
	footer := ""
	switch screen.kind {
	case llamaManagerScreenModels:
		body = append(
			body,
			selectorTextLine(tuiThemeFG("dim", screen.server), width),
			"",
		)
		if screen.list != nil {
			body = append(body, screen.list.Render(width)...)
		}
		footer = llamaSelectFooter("load/unload/download", "close")
	case llamaManagerScreenSelect:
		body = append(body, "")
		if screen.list != nil {
			body = append(body, screen.list.Render(width)...)
		}
		footer = llamaSelectFooter("select", "cancel")
	case llamaManagerScreenSearch:
		body = append(
			body,
			"",
			selectorTextLine(
				tuiThemeFG(
					"dim",
					"Model name or owner/repository[:quant]",
				),
				width,
			),
		)
		if screen.input != nil {
			body = append(body, screen.input.Render(width)...)
		}
		body = append(body, "")
		body = append(
			body,
			renderLlamaSearchResults(screen, width)...,
		)
		footer = llamaSelectFooter("select", "back")
	case llamaManagerScreenStatus:
		body = append(
			body,
			"",
			selectorTextLine(
				tuiThemeFG("dim", screen.message),
				width,
			),
		)
	case llamaManagerScreenProgress:
		body = renderLlamaProgress(screen.progress, width)
		footer = tuiThemeKeyHint(
			selectorCancelKeyHint(),
			"stop",
		)
	default:
		body = append(
			body,
			selectorTextLine(tuiThemeFG("dim", "Loading…"), width),
		)
	}
	return renderLlamaFrame(
		firstNonEmptyString(screen.title, "llama.cpp models"),
		body,
		footer,
		width,
	)
}

func renderLlamaFrame(
	title string,
	body []string,
	footer string,
	width int,
) []string {
	lines := []string{selectorDynamicBorder(width), ""}
	lines = append(
		lines,
		selectorTextLines(tuiThemeBoldAccent(title), width)...,
	)
	lines = append(lines, body...)
	if strings.TrimSpace(footer) != "" {
		lines = append(
			lines,
			"",
			selectorTextLine(tuiThemeFG("dim", footer), width),
		)
	}
	lines = append(lines, "", selectorDynamicBorder(width))
	for index, line := range lines {
		if gitui.VisibleWidth(line) > width {
			lines[index] = gitui.TruncateToWidth(
				line,
				width,
				"",
				true,
			)
		}
	}
	return lines
}

func renderLlamaSearchResults(
	screen llamaManagerScreen,
	width int,
) []string {
	const maxVisible = 10
	start := max(
		0,
		min(
			screen.selected-maxVisible/2,
			len(screen.filtered)-maxVisible,
		),
	)
	end := min(start+maxVisible, len(screen.filtered))
	var lines []string
	for index := start; index < end; index++ {
		model := screen.filtered[index]
		prefix := "  "
		text := model.ID
		detail := compactLlamaCount(model.Downloads) + " downloads"
		if index == screen.selected {
			prefix = "→ "
			text = tuiThemeAccent(
				prefix + text + "  " + detail,
			)
		} else {
			text = prefix + text + tuiThemeFG(
				"muted",
				"  "+detail,
			)
		}
		lines = append(lines, selectorTextLine(text, width))
	}
	if start > 0 || end < len(screen.filtered) {
		lines = append(
			lines,
			selectorTextLine(
				tuiThemeFG(
					"dim",
					fmt.Sprintf(
						"  (%d/%d)",
						screen.selected+1,
						len(screen.filtered),
					),
				),
				width,
			),
		)
	}
	if len(screen.filtered) == 0 ||
		screen.status == "Searching Hugging Face…" {
		lines = append(
			lines,
			selectorTextLine(
				tuiThemeFG("dim", "  "+screen.status),
				width,
			),
		)
	}
	return lines
}

func renderLlamaProgress(
	state llamaProgressState,
	width int,
) []string {
	lines := []string{
		selectorTextLine(tuiThemeFG("text", state.Model), width),
		"",
		selectorTextLine(tuiThemeFG("muted", state.Message), width),
	}
	if state.Ratio != nil {
		const available = 40
		ratio := max(0, min(1, *state.Ratio))
		filled := int(ratio*available + 0.5)
		lines = append(
			lines,
			selectorTextLine(
				tuiThemeAccent(
					strings.Repeat("█", filled)+
						strings.Repeat("─", available-filled)+
						fmt.Sprintf(
							" %d%%",
							int(ratio*100+0.5),
						),
				),
				width,
			),
		)
	}
	if state.Detail != "" {
		lines = append(
			lines,
			selectorTextLine(
				tuiThemeFG("dim", state.Detail),
				width,
			),
		)
	}
	return lines
}

func llamaSelectListTheme() gitui.SelectListTheme {
	return gitui.SelectListTheme{
		SelectedPrefix: tuiThemeAccent,
		SelectedText:   tuiThemeAccent,
		Description: func(text string) string {
			return tuiThemeFG("muted", text)
		},
		ScrollInfo: func(text string) string {
			return tuiThemeFG("dim", text)
		},
		NoMatch: func(text string) string {
			return tuiThemeFG("warning", text)
		},
	}
}

func llamaSelectFooter(confirm string, cancel string) string {
	confirmKey := firstNonEmptyString(
		formatHotkeyKeys(
			gitui.GetKeybindings().GetKeys("tui.select.confirm"),
			false,
		),
		"enter",
	)
	return tuiThemeKeyHint(confirmKey, confirm) + "  " +
		tuiThemeKeyHint(selectorCancelKeyHint(), cancel)
}

func llamaContextLabel(model llama.LlamaModelInfo) string {
	contextWindow := 0
	if model.Meta.ContextWindow != nil {
		contextWindow = *model.Meta.ContextWindow
	} else if model.Meta.TrainingContext != nil {
		contextWindow = *model.Meta.TrainingContext
	}
	if contextWindow <= 0 {
		for index := 0; index+1 < len(model.Status.Args); index++ {
			switch model.Status.Args[index] {
			case "--ctx-size", "-c", "-ctx":
				value, err := strconv.Atoi(model.Status.Args[index+1])
				if err == nil && value > 0 {
					contextWindow = value
				}
			}
			if contextWindow > 0 {
				break
			}
		}
	}
	if contextWindow <= 0 {
		return ""
	}
	if contextWindow >= 1000 {
		return fmt.Sprintf("%dk", (contextWindow+500)/1000)
	}
	return strconv.Itoa(contextWindow)
}

func llamaModelDescription(model llama.LlamaModelInfo) string {
	var details []string
	loaded := llamaModelIsLoaded(model)
	if loaded {
		details = append(details, "loaded")
	} else if model.Status.Value != llama.LlamaModelUnloaded {
		details = append(
			details,
			string(model.Status.Value),
		)
	}
	if loaded {
		if contextLabel := llamaContextLabel(model); contextLabel != "" {
			details = append(
				details,
				contextLabel+" context",
			)
		}
	}
	return strings.Join(details, " · ")
}

func compactLlamaCount(value int64) string {
	switch {
	case value >= 10_000_000:
		return fmt.Sprintf("%.0fM", float64(value)/1_000_000)
	case value >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(value)/1_000_000)
	case value >= 100_000:
		return fmt.Sprintf("%.0fk", float64(value)/1_000)
	case value >= 1_000:
		return fmt.Sprintf("%.1fk", float64(value)/1_000)
	default:
		return strconv.FormatInt(value, 10)
	}
}

func cloneHuggingFaceModels(
	models []llama.HuggingFaceModel,
) []llama.HuggingFaceModel {
	return append([]llama.HuggingFaceModel(nil), models...)
}

func sendLlamaViewResult(
	result chan<- llamaManagerViewResult,
	value llamaManagerViewResult,
) {
	select {
	case result <- value:
	default:
	}
}

func (v *llamaManagerView) render() {
	if v != nil && v.requestRender != nil {
		v.requestRender()
	}
}
