package gicodingagent

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type manualCountdownTicker struct {
	ticks   chan time.Time
	stopped atomic.Bool
}

func newManualCountdownTicker() *manualCountdownTicker {
	return &manualCountdownTicker{ticks: make(chan time.Time, 4)}
}

func (t *manualCountdownTicker) Ticks() <-chan time.Time {
	return t.ticks
}

func (t *manualCountdownTicker) Stop() {
	t.stopped.Store(true)
}

type countingRenderRequester struct {
	calls  atomic.Int64
	render chan struct{}
}

func newCountingRenderRequester() *countingRenderRequester {
	return &countingRenderRequester{render: make(chan struct{}, 4)}
}

func (r *countingRenderRequester) RequestRender(...bool) {
	r.calls.Add(1)
	select {
	case r.render <- struct{}{}:
	default:
	}
}

func TestStatusIndicatorsPiCases(t *testing.T) {
	t.Run("keeps idle status at the same height as status indicators", func(t *testing.T) {
		lines := (IdleStatus{}).Render(20)
		if len(lines) != 2 || lines[0] != strings.Repeat(" ", 20) || lines[1] != strings.Repeat(" ", 20) {
			t.Fatalf("idle status lines = %#v", lines)
		}
	})

	t.Run("disposes retry countdown updates", func(t *testing.T) {
		ticker := newManualCountdownTicker()
		requester := newCountingRenderRequester()
		indicator := newRetryStatusIndicator(
			nil,
			requester,
			1,
			3,
			2*time.Second,
			func(time.Duration) countdownTicker { return ticker },
		)

		ticker.ticks <- time.Now()
		select {
		case <-requester.render:
		case <-time.After(time.Second):
			t.Fatal("retry countdown did not request a render")
		}
		if rendered := StripAnsi(strings.Join(indicator.Render(80), "\n")); !strings.Contains(rendered, "Retrying (1/3) in 1s") {
			t.Fatalf("retry indicator = %q", rendered)
		}

		callsBeforeDispose := requester.calls.Load()
		indicator.Dispose()
		if !ticker.stopped.Load() {
			t.Fatal("retry countdown ticker was not stopped")
		}
		ticker.ticks <- time.Now()
		time.Sleep(20 * time.Millisecond)
		if calls := requester.calls.Load(); calls != callsBeforeDispose {
			t.Fatalf("render calls after dispose = %d, want %d", calls, callsBeforeDispose)
		}
	})
}

func TestStatusIndicatorKindsAndMessages(t *testing.T) {
	working := NewWorkingStatusIndicator(nil, "Working...", nil)
	defer working.Dispose()
	if working.StatusKind() != StatusIndicatorKindWorking {
		t.Fatalf("working kind = %q", working.StatusKind())
	}

	compaction := NewCompactionStatusIndicator(nil, CompactionStatusReasonOverflow)
	defer compaction.Dispose()
	if rendered := StripAnsi(strings.Join(compaction.Render(100), "\n")); !strings.Contains(rendered, "Context overflow detected, Auto-compacting") {
		t.Fatalf("compaction indicator = %q", rendered)
	}

	branch := NewBranchSummaryStatusIndicator(nil)
	defer branch.Dispose()
	if branch.StatusKind() != StatusIndicatorKindBranchSummary {
		t.Fatalf("branch summary kind = %q", branch.StatusKind())
	}
}
