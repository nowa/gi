package gicodingagent

import (
	"sync/atomic"
	"testing"
)

func TestAgentSessionLifecycleCancellationUsesLatestRegistration(t *testing.T) {
	var firstCalls atomic.Int32
	var secondCalls atomic.Int32
	lifecycle := agentSessionLifecycle{}
	cleanupFirst := lifecycle.installCancellation(
		agentSessionCancellationRetry,
		func() { firstCalls.Add(1) },
	)
	cleanupSecond := lifecycle.installCancellation(
		agentSessionCancellationRetry,
		func() { secondCalls.Add(1) },
	)

	cleanupFirst()
	if !lifecycle.hasCancellation(agentSessionCancellationRetry) {
		t.Fatal("stale cleanup removed the current cancellation")
	}
	if !lifecycle.cancel(agentSessionCancellationRetry) {
		t.Fatal("current cancellation was not registered")
	}
	if !lifecycle.cancel(agentSessionCancellationRetry) {
		t.Fatal("registered cancellation should remain visible until cleanup")
	}
	if firstCalls.Load() != 0 || secondCalls.Load() != 1 {
		t.Fatalf("first calls=%d second calls=%d, want 0/1", firstCalls.Load(), secondCalls.Load())
	}

	cleanupSecond()
	if lifecycle.hasCancellation(agentSessionCancellationRetry) {
		t.Fatal("current cleanup should remove its cancellation")
	}
}

func TestAgentSessionLifecycleExclusiveActivityHasSingleOwner(t *testing.T) {
	lifecycle := agentSessionLifecycle{}
	cleanupFirst, started := lifecycle.tryStartExclusiveCancellableActivity(
		agentSessionActivityCompacting,
		agentSessionCancellationCompaction,
		func() {},
	)
	if !started || !lifecycle.isActive(agentSessionActivityCompacting) {
		t.Fatal("first exclusive activity should start")
	}
	if _, started := lifecycle.tryStartExclusiveCancellableActivity(
		agentSessionActivityCompacting,
		agentSessionCancellationCompaction,
		func() {},
	); started {
		t.Fatal("second exclusive activity should be rejected")
	}

	cleanupFirst()
	lifecycle.setActivity(agentSessionActivityCompacting, false)
	cleanupSecond, started := lifecycle.tryStartExclusiveCancellableActivity(
		agentSessionActivityCompacting,
		agentSessionCancellationCompaction,
		func() {},
	)
	if !started {
		t.Fatal("exclusive activity should restart after cleanup")
	}
	cleanupSecond()
	lifecycle.setActivity(agentSessionActivityCompacting, false)
}

func TestAgentSessionLifecycleSettlementAdoptsContinuation(t *testing.T) {
	lifecycle := agentSessionLifecycle{}
	if !lifecycle.tryStartStreaming() {
		t.Fatal("initial activity should start")
	}
	lifecycle.mu.Lock()
	idle := lifecycle.idle
	lifecycle.mu.Unlock()

	lifecycle.beginAgentSettlement()
	if !lifecycle.isIdle() {
		t.Fatal("settlement handlers should observe idle state")
	}
	if !lifecycle.tryStartStreaming() {
		t.Fatal("a settlement continuation should adopt the current idle wait")
	}
	lifecycle.beginAgentSettlement()
	lifecycle.finishSettlement()
	select {
	case <-idle:
		t.Fatal("idle signal closed before the outer settlement completed")
	default:
	}
	lifecycle.finishSettlement()
	<-idle
}
