package gitui

import (
	"context"
	"sync"
	"time"
)

type LoaderIndicatorOptions struct {
	Frames                  []string
	Interval                time.Duration
	IntervalMs              int
	TUI                     *TUI
	SpinnerColor            func(string) string
	MessageColor            func(string) string
	RenderIndicatorVerbatim bool
}

type Loader struct {
	mu                      sync.Mutex
	message                 string
	frames                  []string
	interval                time.Duration
	current                 int
	ui                      *TUI
	spinnerColor            func(string) string
	messageColor            func(string) string
	renderIndicatorVerbatim bool
	stopAnimation           chan struct{}
}

func NewLoader(text string, options ...LoaderIndicatorOptions) *Loader {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	interval := 80 * time.Millisecond
	var opts LoaderIndicatorOptions
	if len(options) > 0 {
		opts = options[0]
		if opts.Frames != nil {
			frames = append([]string(nil), opts.Frames...)
		}
		if opts.Interval > 0 {
			interval = opts.Interval
		} else if opts.IntervalMs > 0 {
			interval = time.Duration(opts.IntervalMs) * time.Millisecond
		}
	}
	loader := &Loader{
		message:                 text,
		frames:                  frames,
		interval:                interval,
		ui:                      opts.TUI,
		spinnerColor:            opts.SpinnerColor,
		messageColor:            opts.MessageColor,
		renderIndicatorVerbatim: opts.RenderIndicatorVerbatim || opts.Frames != nil,
	}
	if opts.TUI != nil {
		loader.Start()
	}
	return loader
}

func (l *Loader) SetText(text string) { l.SetMessage(text) }
func (l *Loader) SetMessage(message string) {
	l.mu.Lock()
	l.message = message
	l.mu.Unlock()
	l.requestRender()
}

func (l *Loader) SetIndicator(options LoaderIndicatorOptions) {
	l.mu.Lock()
	if options.Frames != nil {
		l.frames = append([]string(nil), options.Frames...)
		l.renderIndicatorVerbatim = true
	} else {
		l.frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		l.renderIndicatorVerbatim = false
	}
	if options.Interval > 0 {
		l.interval = options.Interval
	} else if options.IntervalMs > 0 {
		l.interval = time.Duration(options.IntervalMs) * time.Millisecond
	}
	if options.TUI != nil {
		l.ui = options.TUI
	}
	if options.SpinnerColor != nil {
		l.spinnerColor = options.SpinnerColor
	}
	if options.MessageColor != nil {
		l.messageColor = options.MessageColor
	}
	if options.RenderIndicatorVerbatim {
		l.renderIndicatorVerbatim = true
	}
	l.current = 0
	running := l.stopAnimation != nil
	l.mu.Unlock()
	if running {
		l.Start()
	} else {
		l.requestRender()
	}
}

func (l *Loader) SetTUI(ui *TUI) {
	l.mu.Lock()
	l.ui = ui
	l.mu.Unlock()
}

func (l *Loader) Start() {
	l.mu.Lock()
	l.stopAnimationLocked()
	if len(l.frames) <= 1 {
		l.mu.Unlock()
		l.requestRender()
		return
	}
	interval := l.interval
	if interval <= 0 {
		interval = 80 * time.Millisecond
	}
	stop := make(chan struct{})
	l.stopAnimation = stop
	l.mu.Unlock()
	l.requestRender()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				l.mu.Lock()
				if len(l.frames) > 0 {
					l.current = (l.current + 1) % len(l.frames)
				}
				l.mu.Unlock()
				l.requestRender()
			case <-stop:
				return
			}
		}
	}()
}

func (l *Loader) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopAnimationLocked()
}

func (l *Loader) stopAnimationLocked() {
	if l.stopAnimation != nil {
		close(l.stopAnimation)
		l.stopAnimation = nil
	}
}

func (l *Loader) Invalidate() {}
func (l *Loader) Render(width int) []string {
	l.mu.Lock()
	message := l.message
	frames := append([]string(nil), l.frames...)
	current := l.current
	spinnerColor := l.spinnerColor
	messageColor := l.messageColor
	verbatim := l.renderIndicatorVerbatim
	l.mu.Unlock()

	renderedMessage := style(messageColor, message)
	indicator := ""
	if len(frames) > 0 {
		frame := frames[current%len(frames)]
		renderedFrame := frame
		if !verbatim {
			renderedFrame = style(spinnerColor, frame)
		}
		if frame != "" {
			indicator = renderedFrame + " "
		}
	}
	text := NewText(indicator+renderedMessage, 1, 0)
	return append([]string{""}, text.Render(width)...)
}

func (l *Loader) requestRender() {
	l.mu.Lock()
	ui := l.ui
	l.mu.Unlock()
	if ui != nil {
		ui.RequestRender(false)
	}
}

type CancellableLoader struct {
	*Loader
	cancelMu  sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	cancelled bool
	OnCancel  func()
	OnAbort   func()
}

func NewCancellableLoader(text string, options ...LoaderIndicatorOptions) *CancellableLoader {
	ctx, cancel := context.WithCancel(context.Background())
	return &CancellableLoader{Loader: NewLoader(text, options...), ctx: ctx, cancel: cancel}
}

func (l *CancellableLoader) HandleInput(data string) {
	if GetKeybindings().Matches(data, "tui.select.cancel") {
		l.Cancel()
	}
}

func (l *CancellableLoader) Cancel() {
	l.cancelMu.Lock()
	if l.cancelled {
		l.cancelMu.Unlock()
		return
	}
	l.cancelled = true
	cancel := l.cancel
	onCancel := l.OnCancel
	onAbort := l.OnAbort
	l.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if onCancel != nil {
		onCancel()
	}
	if onAbort != nil {
		onAbort()
	}
}

func (l *CancellableLoader) Context() context.Context {
	l.cancelMu.Lock()
	defer l.cancelMu.Unlock()
	if l.ctx == nil {
		l.ctx, l.cancel = context.WithCancel(context.Background())
	}
	return l.ctx
}

func (l *CancellableLoader) Signal() context.Context {
	return l.Context()
}

func (l *CancellableLoader) Cancelled() bool {
	l.cancelMu.Lock()
	defer l.cancelMu.Unlock()
	return l.cancelled
}
func (l *CancellableLoader) Aborted() bool { return l.Cancelled() }
func (l *CancellableLoader) Dispose()      { l.Stop() }
