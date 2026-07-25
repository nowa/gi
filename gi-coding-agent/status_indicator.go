package gicodingagent

import (
	"fmt"
	"strings"
	"sync"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

// StatusIndicatorKind identifies the semantic owner of the shared status slot.
type StatusIndicatorKind string

const (
	// StatusIndicatorKindWorking identifies the normal agent activity indicator.
	StatusIndicatorKindWorking StatusIndicatorKind = "working"
	// StatusIndicatorKindRetry identifies an auto-retry countdown.
	StatusIndicatorKindRetry StatusIndicatorKind = "retry"
	// StatusIndicatorKindCompaction identifies context compaction activity.
	StatusIndicatorKindCompaction StatusIndicatorKind = "compaction"
	// StatusIndicatorKindBranchSummary identifies branch summarization activity.
	StatusIndicatorKindBranchSummary StatusIndicatorKind = "branchSummary"
)

// CompactionStatusReason determines the label used for compaction activity.
type CompactionStatusReason string

const (
	// CompactionStatusReasonManual identifies user-requested compaction.
	CompactionStatusReasonManual CompactionStatusReason = "manual"
	// CompactionStatusReasonThreshold identifies automatic threshold compaction.
	CompactionStatusReasonThreshold CompactionStatusReason = "threshold"
	// CompactionStatusReasonOverflow identifies overflow recovery compaction.
	CompactionStatusReasonOverflow CompactionStatusReason = "overflow"
)

// TUIRenderRequester is the narrow rendering dependency needed by countdowns.
type TUIRenderRequester interface {
	RequestRender(force ...bool)
}

// CountdownTimer owns one countdown goroutine. Dispose waits for it to stop,
// so no callback can run after disposal returns.
type CountdownTimer struct {
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

type transientStatusIndicator interface {
	gitui.Component
	StatusKind() StatusIndicatorKind
	Dispose()
}

type countdownTicker interface {
	Ticks() <-chan time.Time
	Stop()
}

type countdownTickerFactory func(time.Duration) countdownTicker

type realCountdownTicker struct {
	ticker *time.Ticker
}

func (t realCountdownTicker) Ticks() <-chan time.Time {
	return t.ticker.C
}

func (t realCountdownTicker) Stop() {
	t.ticker.Stop()
}

// NewCountdownTimer starts a second-granularity countdown and synchronously
// publishes its initial value.
func NewCountdownTimer(
	timeout time.Duration,
	requester TUIRenderRequester,
	onTick func(int),
	onExpire func(),
) *CountdownTimer {
	return newCountdownTimer(timeout, requester, onTick, onExpire, func(interval time.Duration) countdownTicker {
		return realCountdownTicker{ticker: time.NewTicker(interval)}
	})
}

func newCountdownTimer(
	timeout time.Duration,
	requester TUIRenderRequester,
	onTick func(int),
	onExpire func(),
	newTicker countdownTickerFactory,
) *CountdownTimer {
	timer := &CountdownTimer{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	remainingSeconds := countdownSeconds(timeout)
	if onTick != nil {
		onTick(remainingSeconds)
	}

	ticker := newTicker(time.Second)
	go func() {
		defer close(timer.done)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.Ticks():
				remainingSeconds--
				if onTick != nil {
					onTick(remainingSeconds)
				}
				if requester != nil {
					requester.RequestRender(false)
				}
				if remainingSeconds <= 0 {
					if onExpire != nil {
						onExpire()
					}
					return
				}
			case <-timer.stop:
				return
			}
		}
	}()
	return timer
}

// Dispose stops the countdown and waits until its goroutine exits.
func (t *CountdownTimer) Dispose() {
	if t == nil {
		return
	}
	t.once.Do(func() {
		close(t.stop)
	})
	<-t.done
}

// StatusIndicator is the common state and loader lifecycle shared by every
// transient status shown in the interactive status slot.
type StatusIndicator struct {
	*gitui.Loader
	kind StatusIndicatorKind
}

// NewStatusIndicator constructs the common loader owned by a transient status.
func NewStatusIndicator(
	kind StatusIndicatorKind,
	message string,
	options gitui.LoaderIndicatorOptions,
) *StatusIndicator {
	return &StatusIndicator{
		Loader: gitui.NewLoader(message, options),
		kind:   kind,
	}
}

// StatusKind returns the semantic owner of the shared status slot.
func (s *StatusIndicator) StatusKind() StatusIndicatorKind {
	if s == nil {
		return ""
	}
	return s.kind
}

// Dispose stops the loader animation.
func (s *StatusIndicator) Dispose() {
	if s != nil && s.Loader != nil {
		s.Stop()
	}
}

// WorkingStatusIndicator renders normal agent activity.
type WorkingStatusIndicator struct {
	*StatusIndicator
}

// NewWorkingStatusIndicator constructs an agent activity indicator.
func NewWorkingStatusIndicator(
	ui *gitui.TUI,
	message string,
	indicator *TUIWorkingIndicatorOptions,
) *WorkingStatusIndicator {
	options := gitui.LoaderIndicatorOptions{
		TUI:          ui,
		SpinnerColor: tuiThemeAccent,
		MessageColor: tuiThemeMuted,
	}
	if indicator != nil {
		options.Frames = cloneOptionalStringSlice(indicator.Frames)
		options.IntervalMs = indicator.IntervalMs
	}
	return &WorkingStatusIndicator{
		StatusIndicator: NewStatusIndicator(StatusIndicatorKindWorking, message, options),
	}
}

// RetryStatusIndicator renders retry activity and owns its countdown.
type RetryStatusIndicator struct {
	*StatusIndicator

	mu        sync.Mutex
	countdown *CountdownTimer
}

// NewRetryStatusIndicator constructs a retry indicator with a live countdown.
func NewRetryStatusIndicator(
	ui *gitui.TUI,
	attempt int,
	maxAttempts int,
	delay time.Duration,
) *RetryStatusIndicator {
	var requester TUIRenderRequester
	if ui != nil {
		requester = ui
	}
	return newRetryStatusIndicator(ui, requester, attempt, maxAttempts, delay, func(interval time.Duration) countdownTicker {
		return realCountdownTicker{ticker: time.NewTicker(interval)}
	})
}

func newRetryStatusIndicator(
	ui *gitui.TUI,
	requester TUIRenderRequester,
	attempt int,
	maxAttempts int,
	delay time.Duration,
	newTicker countdownTickerFactory,
) *RetryStatusIndicator {
	message := func(seconds int) string {
		return retryStatusMessage(attempt, maxAttempts, seconds)
	}
	indicator := &RetryStatusIndicator{
		StatusIndicator: NewStatusIndicator(
			StatusIndicatorKindRetry,
			message(countdownSeconds(delay)),
			gitui.LoaderIndicatorOptions{
				TUI:          ui,
				SpinnerColor: tuiThemeWarning,
				MessageColor: tuiThemeMuted,
			},
		),
	}
	timer := newCountdownTimer(
		delay,
		requester,
		func(seconds int) {
			indicator.SetMessage(message(seconds))
		},
		func() {
			indicator.mu.Lock()
			indicator.countdown = nil
			indicator.mu.Unlock()
		},
		newTicker,
	)
	indicator.mu.Lock()
	indicator.countdown = timer
	indicator.mu.Unlock()
	return indicator
}

// Dispose stops both the countdown and loader animation.
func (s *RetryStatusIndicator) Dispose() {
	if s == nil {
		return
	}
	s.mu.Lock()
	countdown := s.countdown
	s.countdown = nil
	s.mu.Unlock()
	if countdown != nil {
		countdown.Dispose()
	}
	s.StatusIndicator.Dispose()
}

// CompactionStatusIndicator renders manual or automatic compaction activity.
type CompactionStatusIndicator struct {
	*StatusIndicator
}

// NewCompactionStatusIndicator constructs a compaction indicator.
func NewCompactionStatusIndicator(
	ui *gitui.TUI,
	reason CompactionStatusReason,
) *CompactionStatusIndicator {
	return &CompactionStatusIndicator{
		StatusIndicator: NewStatusIndicator(
			StatusIndicatorKindCompaction,
			compactionStatusMessage(reason),
			gitui.LoaderIndicatorOptions{
				TUI:          ui,
				SpinnerColor: tuiThemeAccent,
				MessageColor: tuiThemeMuted,
			},
		),
	}
}

func normalizeCompactionStatusReason(reason string) CompactionStatusReason {
	switch normalized := CompactionStatusReason(reason); normalized {
	case CompactionStatusReasonThreshold, CompactionStatusReasonOverflow:
		return normalized
	default:
		return CompactionStatusReasonManual
	}
}

func compactionStatusMessage(reason CompactionStatusReason) string {
	if reason == CompactionStatusReasonManual {
		return "Compacting context... (Esc to cancel)"
	}
	prefix := ""
	if reason == CompactionStatusReasonOverflow {
		prefix = "Context overflow detected, "
	}
	return prefix + "Auto-compacting... (Esc to cancel)"
}

// BranchSummaryStatusIndicator renders branch summarization activity.
type BranchSummaryStatusIndicator struct {
	*StatusIndicator
}

// NewBranchSummaryStatusIndicator constructs a branch summarization indicator.
func NewBranchSummaryStatusIndicator(ui *gitui.TUI) *BranchSummaryStatusIndicator {
	return &BranchSummaryStatusIndicator{
		StatusIndicator: NewStatusIndicator(
			StatusIndicatorKindBranchSummary,
			"Summarizing branch... (Esc to cancel)",
			gitui.LoaderIndicatorOptions{
				TUI:          ui,
				SpinnerColor: tuiThemeAccent,
				MessageColor: tuiThemeMuted,
			},
		),
	}
}

// IdleStatus preserves the two-line status region after a shrinking render.
type IdleStatus struct{}

// Invalidate satisfies gitui.Component; IdleStatus has no cached state.
func (IdleStatus) Invalidate() {}

// Render returns two blank lines matching the height of a status indicator.
func (IdleStatus) Render(width int) []string {
	width = max(width, 0)
	emptyLine := strings.Repeat(" ", width)
	return []string{emptyLine, emptyLine}
}

func retryStatusMessage(attempt, maxAttempts, seconds int) string {
	return fmt.Sprintf(
		"Retrying (%d/%d) in %ds... (Esc to cancel)",
		attempt,
		maxAttempts,
		seconds,
	)
}

func countdownSeconds(delay time.Duration) int {
	if delay <= 0 {
		return 0
	}
	return int((delay-1)/time.Second) + 1
}
