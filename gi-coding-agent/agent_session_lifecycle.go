package gicodingagent

import (
	"context"
	"sync"
)

type agentSessionActivity uint8

const (
	agentSessionActivityStreaming agentSessionActivity = iota
	agentSessionActivityRetrying
	agentSessionActivityCompacting
)

type agentSessionCancellation uint8

const (
	agentSessionCancellationRetry agentSessionCancellation = iota
	agentSessionCancellationCompaction
	agentSessionCancellationBash
	agentSessionCancellationBranchSummary
)

type agentSessionCancelRegistration struct {
	id     uint64
	cancel func()
}

// agentSessionLifecycle is the single synchronization boundary for activity
// flags, cancellation handles, abort intent, and idle notification. The zero
// value is ready for use.
type agentSessionLifecycle struct {
	mu sync.Mutex

	streaming      bool
	retrying       bool
	compacting     bool
	settlements    uint
	abortRequested bool
	idle           chan struct{}

	nextCancellationID uint64
	cancellations      map[agentSessionCancellation]agentSessionCancelRegistration
}

func (l *agentSessionLifecycle) tryStartStreaming() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.streaming || l.retrying || l.compacting || (l.idle != nil && l.settlements == 0) {
		return false
	}
	l.startActivityLocked()
	l.streaming = true
	return true
}

// beginAgentSettlement makes the session observably idle while keeping waiters
// blocked until the terminal agent_settled event has been dispatched.
func (l *agentSessionLifecycle) beginAgentSettlement() {
	l.mu.Lock()
	l.streaming = false
	l.retrying = false
	l.settlements++
	l.mu.Unlock()
}

func (l *agentSessionLifecycle) beginActivitySettlement(activity agentSessionActivity) {
	l.mu.Lock()
	l.assignActivityLocked(activity, false)
	l.settlements++
	l.mu.Unlock()
}

func (l *agentSessionLifecycle) finishSettlement() {
	l.mu.Lock()
	if l.settlements > 0 {
		l.settlements--
	}
	l.resolveIdleLocked()
	l.mu.Unlock()
}

func (l *agentSessionLifecycle) setActivity(
	activity agentSessionActivity,
	active bool,
) {
	l.mu.Lock()
	defer l.mu.Unlock()
	current := l.activityLocked(activity)
	if current == active {
		return
	}
	if active {
		l.startActivityLocked()
	}
	l.assignActivityLocked(activity, active)
	if !active {
		l.resolveIdleLocked()
	}
}

func (l *agentSessionLifecycle) isActive(
	activity agentSessionActivity,
) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.activityLocked(activity)
}

func (l *agentSessionLifecycle) isIdle() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return !l.activeLocked()
}

func (l *agentSessionLifecycle) waitForIdle(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	l.mu.Lock()
	if l.idle == nil {
		l.mu.Unlock()
		return nil
	}
	idle := l.idle
	l.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-idle:
		return nil
	}
}

func (l *agentSessionLifecycle) resetAbort() {
	l.mu.Lock()
	l.abortRequested = false
	l.mu.Unlock()
}

func (l *agentSessionLifecycle) requestAbort() {
	l.mu.Lock()
	l.abortRequested = true
	l.mu.Unlock()
}

func (l *agentSessionLifecycle) isAbortRequested() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.abortRequested
}

func (l *agentSessionLifecycle) installCancellation(
	kind agentSessionCancellation,
	cancel func(),
) func() {
	if cancel == nil {
		return func() {}
	}
	l.mu.Lock()
	id := l.registerCancellationLocked(kind, cancel)
	l.mu.Unlock()
	return l.cancellationCleanup(kind, id)
}

func (l *agentSessionLifecycle) startNestedCancellableActivity(
	activity agentSessionActivity,
	kind agentSessionCancellation,
	cancel func(),
) func() {
	l.mu.Lock()
	if !l.activityLocked(activity) {
		l.startActivityLocked()
		l.assignActivityLocked(activity, true)
	}
	id := l.registerCancellationLocked(kind, cancel)
	l.mu.Unlock()
	return l.cancellationCleanup(kind, id)
}

func (l *agentSessionLifecycle) tryStartExclusiveCancellableActivity(
	activity agentSessionActivity,
	kind agentSessionCancellation,
	cancel func(),
) (func(), bool) {
	l.mu.Lock()
	if l.idle != nil {
		l.mu.Unlock()
		return func() {}, false
	}
	l.startActivityLocked()
	l.assignActivityLocked(activity, true)
	id := l.registerCancellationLocked(kind, cancel)
	l.mu.Unlock()
	return l.cancellationCleanup(kind, id), true
}

func (l *agentSessionLifecycle) cancel(kind agentSessionCancellation) bool {
	l.mu.Lock()
	registration, ok := l.cancellations[kind]
	l.mu.Unlock()
	if ok {
		registration.cancel()
	}
	return ok
}

func (l *agentSessionLifecycle) hasCancellation(kind agentSessionCancellation) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.cancellations[kind]
	return ok
}

func (l *agentSessionLifecycle) registerCancellationLocked(
	kind agentSessionCancellation,
	cancel func(),
) uint64 {
	if cancel == nil {
		cancel = func() {}
	}
	l.nextCancellationID++
	id := l.nextCancellationID
	if l.cancellations == nil {
		l.cancellations = make(map[agentSessionCancellation]agentSessionCancelRegistration)
	}
	var once sync.Once
	l.cancellations[kind] = agentSessionCancelRegistration{
		id: id,
		cancel: func() {
			once.Do(cancel)
		},
	}
	return id
}

func (l *agentSessionLifecycle) cancellationCleanup(
	kind agentSessionCancellation,
	id uint64,
) func() {
	return func() {
		l.mu.Lock()
		registration, ok := l.cancellations[kind]
		if ok && registration.id == id {
			delete(l.cancellations, kind)
		}
		l.mu.Unlock()
	}
}

func (l *agentSessionLifecycle) startActivityLocked() {
	if l.idle == nil {
		l.idle = make(chan struct{})
	}
}

func (l *agentSessionLifecycle) activeLocked() bool {
	return l.streaming || l.retrying || l.compacting
}

func (l *agentSessionLifecycle) resolveIdleLocked() {
	if l.activeLocked() || l.settlements > 0 || l.idle == nil {
		return
	}
	close(l.idle)
	l.idle = nil
}

func (l *agentSessionLifecycle) activityLocked(
	activity agentSessionActivity,
) bool {
	switch activity {
	case agentSessionActivityStreaming:
		return l.streaming
	case agentSessionActivityRetrying:
		return l.retrying
	case agentSessionActivityCompacting:
		return l.compacting
	default:
		return false
	}
}

func (l *agentSessionLifecycle) assignActivityLocked(
	activity agentSessionActivity,
	active bool,
) {
	switch activity {
	case agentSessionActivityStreaming:
		l.streaming = active
	case agentSessionActivityRetrying:
		l.retrying = active
	case agentSessionActivityCompacting:
		l.compacting = active
	}
}

func (s *AgentSession) settleAgentRun() error {
	if s == nil {
		return nil
	}
	defer func() {
		s.activeRunMessages = nil
		s.runMessageCapture = false
	}()
	s.lifecycle.beginAgentSettlement()
	defer s.lifecycle.finishSettlement()
	err := s.emitExtensionEvent(ProtocolSessionEvent{Type: ProtocolEventAgentSettled})
	s.emit(AgentSessionEvent{Type: ProtocolEventAgentSettled})
	return err
}
