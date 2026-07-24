package gicodingagent

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

type cliEditorReplacementLifecycle struct {
	mu      sync.Mutex
	closed  bool
	restore func()
}

func (l *cliEditorReplacementLifecycle) install(restore func()) {
	if l == nil || restore == nil {
		return
	}
	l.mu.Lock()
	if !l.closed {
		l.restore = restore
		l.mu.Unlock()
		return
	}
	l.mu.Unlock()
	restore()
}

func (l *cliEditorReplacementLifecycle) close() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return false
	}
	l.closed = true
	restore := l.restore
	l.restore = nil
	l.mu.Unlock()
	if restore != nil {
		restore()
		return true
	}
	return false
}

type cliDialogCompletion struct {
	once        sync.Once
	replacement cliEditorReplacementLifecycle
	result      chan TUIDialogResult
}

func newCLIDialogCompletion() *cliDialogCompletion {
	return &cliDialogCompletion{result: make(chan TUIDialogResult, 1)}
}

func (c *cliDialogCompletion) installRestore(restore func()) {
	if c == nil {
		return
	}
	c.replacement.install(restore)
}

func (c *cliDialogCompletion) finish(result TUIDialogResult) {
	if c == nil {
		return
	}
	c.once.Do(func() {
		c.replacement.close()
		c.result <- result
	})
}

func (c *cliDialogCompletion) wait(done <-chan struct{}) TUIDialogResult {
	select {
	case result := <-c.result:
		return result
	case <-done:
		c.finish(TUIDialogResult{Action: "cancelled"})
		return <-c.result
	}
}

func (h *CLIInteractiveTUIHost) RunTUIDialog(request TUIDialogRequest) (TUIDialogResult, error) {
	if h == nil {
		return TUIDialogResult{}, errors.New("interactive TUI dialog host is not ready")
	}
	kind := firstNonEmptyString(strings.TrimSpace(request.Kind), "notify")
	switch kind {
	case "notify", "notification":
		text := firstNonEmptyString(request.Message, request.Title)
		if strings.TrimSpace(text) != "" {
			h.addStatus(formatTUIDialogNotification(text, request.Type))
		}
		return TUIDialogResult{Action: "acknowledged"}, nil
	case "confirm":
		options := []TUIDialogOption{
			{ID: "yes", Label: "Yes", Value: true},
			{ID: "no", Label: "No", Value: false},
		}
		return h.runExtensionOptionDialog(requestDialogTitle(request.Title, request.Message), options, dialogDefaultOptionIndex(options, request.DefaultValue), request.Timeout, func(option TUIDialogOption) TUIDialogResult {
			if option.ID == "yes" {
				return TUIDialogResult{Action: "confirmed", OptionID: "yes", Value: true}
			}
			return TUIDialogResult{Action: "declined", OptionID: "no", Value: false}
		})
	case "select":
		if len(request.Options) == 0 {
			return TUIDialogResult{}, errors.New("select dialog requires options")
		}
		return h.runExtensionOptionDialog(requestDialogTitle(request.Title, request.Message), request.Options, dialogDefaultOptionIndex(request.Options, request.DefaultValue), request.Timeout, func(option TUIDialogOption) TUIDialogResult {
			return TUIDialogResult{Action: "selected", OptionID: option.ID, Value: dialogOptionValue(option)}
		})
	case "input":
		var submitted TUIDialogResult
		component := newCLIInputDialog(request.Title, request.Message, request.Placeholder, dialogStringValue(request.DefaultValue), func(value string) {
			submitted = TUIDialogResult{Action: "submitted", Value: value}
		}, func() {})
		return h.runSelectionDialog(component, func() TUIDialogResult { return submitted }, request.Timeout)
	case "editor", "textarea":
		var submitted TUIDialogResult
		component := newCLIEditorDialog(h.ui, request.Title, request.Message, dialogStringValue(request.DefaultValue), func(value string) {
			submitted = TUIDialogResult{Action: "submitted", Value: value}
		}, func() {})
		return h.runSelectionDialog(component, func() TUIDialogResult { return submitted }, request.Timeout)
	default:
		return TUIDialogResult{}, errors.New("unsupported dialog kind: " + kind)
	}
}

func requestDialogTitle(title, message string) string {
	title = strings.TrimSpace(title)
	message = strings.TrimSpace(message)
	switch {
	case title == "":
		return message
	case message == "":
		return title
	default:
		return title + "\n" + message
	}
}

func (h *CLIInteractiveTUIHost) runExtensionOptionDialog(title string, options []TUIDialogOption, defaultIndex int, timeout int, resultFor func(TUIDialogOption) TUIDialogResult) (TUIDialogResult, error) {
	if h == nil || h.ui == nil {
		return TUIDialogResult{}, errors.New("interactive TUI is not ready")
	}
	if len(options) == 0 {
		return TUIDialogResult{}, errors.New("select dialog requires options")
	}
	labels := make([]string, 0, len(options))
	for idx, option := range options {
		labels = append(labels, firstNonEmptyString(option.Label, option.ID, strconv.Itoa(idx+1)))
	}
	selector := NewExtensionSelectorComponent(firstNonEmptyString(title, "Select"), labels)
	selector.selected = max(0, min(defaultIndex, len(options)-1))

	completion := newCLIDialogCompletion()
	finish := func(result TUIDialogResult) {
		completion.finish(result)
	}
	selector.OnSelect = func(_ string) {
		index := max(0, min(selector.selected, len(options)-1))
		if resultFor != nil {
			finish(resultFor(options[index]))
			return
		}
		option := options[index]
		finish(TUIDialogResult{Action: "selected", OptionID: option.ID, Value: dialogOptionValue(option)})
	}
	selector.OnCancel = func() {
		finish(TUIDialogResult{Action: "cancelled"})
	}

	stopTimeout := h.startExtensionSelectorTimeout(selector, timeout, finish)
	defer stopTimeout()
	completion.installRestore(h.showEditorReplacement(selector, selector))
	return completion.wait(h.done), nil
}

func formatTUIDialogNotification(message, noticeType string) string {
	message = strings.TrimSpace(message)
	switch strings.ToLower(strings.TrimSpace(noticeType)) {
	case "error":
		if strings.HasPrefix(message, "Error:") {
			return message
		}
		return "Error: " + message
	case "warning", "warn":
		if strings.HasPrefix(message, "Warning:") {
			return message
		}
		return "Warning: " + message
	default:
		return message
	}
}

func (h *CLIInteractiveTUIHost) runConfirmDialog(component *cliTUIDialogComponent, timeout int) (TUIDialogResult, error) {
	var selected TUIDialogResult
	if component != nil && component.list != nil {
		component.list.OnSelect = func(item gitui.SelectItem) {
			switch item.Value {
			case "yes":
				selected = TUIDialogResult{Action: "confirmed", OptionID: "yes", Value: true}
			default:
				selected = TUIDialogResult{Action: "declined", OptionID: "no", Value: false}
			}
		}
	}
	return h.runSelectionDialog(component, func() TUIDialogResult { return selected }, timeout)
}

func (h *CLIInteractiveTUIHost) runSelectionDialog(component *cliTUIDialogComponent, selected func() TUIDialogResult, timeoutValues ...int) (TUIDialogResult, error) {
	if component == nil {
		return TUIDialogResult{}, errors.New("interactive TUI dialog component is not ready")
	}
	component.keybindings = h.effectiveKeybindings()
	component.onToggleToolsExpanded = h.toggleToolOutputExpansion
	var finish func(TUIDialogResult)
	originalCancel := component.onCancel
	component.onCancel = func() {
		if originalCancel != nil {
			originalCancel()
		}
		if finish != nil {
			finish(TUIDialogResult{Action: "cancelled"})
		}
	}
	if component.list != nil {
		onSelect := component.list.OnSelect
		component.list.OnSelect = func(item gitui.SelectItem) {
			if onSelect != nil {
				onSelect(item)
			}
			if finish != nil {
				result := selected()
				if result.Action == "" {
					result = TUIDialogResult{Action: "selected", Value: item.Value}
				}
				finish(result)
			}
		}
		component.list.OnCancel = component.onCancel
	}
	if component.input != nil {
		onSubmit := component.input.OnSubmit
		component.input.OnSubmit = func(value string) {
			if onSubmit != nil {
				onSubmit(value)
			}
			if finish != nil {
				result := selected()
				if result.Action == "" {
					result = TUIDialogResult{Action: "submitted", Value: value}
				}
				finish(result)
			}
		}
		component.input.OnEscape = component.onCancel
	}
	if component.editor != nil {
		onSubmit := component.editorSubmit
		component.editor.SetOnSubmit(func(value string) {
			if onSubmit != nil {
				onSubmit(value)
			}
			if finish != nil {
				result := selected()
				if result.Action == "" {
					result = TUIDialogResult{Action: "submitted", Value: value}
				}
				finish(result)
			}
		})
	}
	completion := newCLIDialogCompletion()
	finish = func(result TUIDialogResult) {
		completion.finish(result)
	}
	if h.ui == nil {
		return TUIDialogResult{}, errors.New("interactive TUI is not ready")
	}
	stopTimeout := h.startDialogTimeout(component, timeoutValues, finish)
	defer stopTimeout()
	completion.installRestore(h.showEditorReplacement(component, component))
	return completion.wait(h.done), nil
}

func (h *CLIInteractiveTUIHost) startDialogTimeout(component *cliTUIDialogComponent, timeoutValues []int, finish func(TUIDialogResult)) func() {
	if h == nil || component == nil || finish == nil || len(timeoutValues) == 0 || timeoutValues[0] <= 0 {
		return func() {}
	}
	timeout := time.Duration(timeoutValues[0]) * time.Millisecond
	deadline := time.Now().Add(timeout)
	baseTitle := component.Title()
	done := make(chan struct{})
	var doneOnce sync.Once
	stop := func() {
		doneOnce.Do(func() {
			close(done)
			component.SetTitle(baseTitle)
		})
	}
	updateTitle := func() {
		remaining := int(math.Ceil(time.Until(deadline).Seconds()))
		if remaining < 0 {
			remaining = 0
		}
		component.SetTitle(fmt.Sprintf("%s (%ds)", baseTitle, remaining))
		h.requestRender(false)
	}
	updateTitle()
	timer := time.NewTimer(timeout)
	ticker := time.NewTicker(time.Second)
	go func() {
		defer timer.Stop()
		defer ticker.Stop()
		for {
			select {
			case <-timer.C:
				finish(TUIDialogResult{Action: "cancelled"})
				return
			case <-ticker.C:
				updateTitle()
			case <-done:
				return
			}
		}
	}()
	return stop
}

func (h *CLIInteractiveTUIHost) startExtensionSelectorTimeout(component *ExtensionSelectorComponent, timeout int, finish func(TUIDialogResult)) func() {
	if h == nil || component == nil || finish == nil || timeout <= 0 {
		return func() {}
	}
	timeoutDuration := time.Duration(timeout) * time.Millisecond
	deadline := time.Now().Add(timeoutDuration)
	baseTitle := component.Title()
	done := make(chan struct{})
	var doneOnce sync.Once
	stop := func() {
		doneOnce.Do(func() {
			close(done)
			component.SetTitle(baseTitle)
		})
	}
	updateTitle := func() {
		remaining := int(math.Ceil(time.Until(deadline).Seconds()))
		if remaining < 0 {
			remaining = 0
		}
		component.SetTitle(fmt.Sprintf("%s (%ds)", baseTitle, remaining))
		h.requestRender(false)
	}
	updateTitle()
	timer := time.NewTimer(timeoutDuration)
	ticker := time.NewTicker(time.Second)
	go func() {
		defer timer.Stop()
		defer ticker.Stop()
		for {
			select {
			case <-timer.C:
				stop()
				finish(TUIDialogResult{Action: "cancelled"})
				return
			case <-ticker.C:
				updateTitle()
			case <-done:
				return
			case <-h.done:
				return
			}
		}
	}()
	return stop
}
