package gicodingagent

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestAgentSessionSubscriptionsAreSafeDuringEmission(t *testing.T) {
	session := &AgentSession{}
	var calls atomic.Int64
	const workers = 32
	const emissions = 200

	var ready sync.WaitGroup
	ready.Add(workers)
	unsubscribes := make([]func(), workers)
	for index := range workers {
		unsubscribes[index] = session.Subscribe(func(AgentSessionEvent) {
			calls.Add(1)
		})
		ready.Done()
	}
	ready.Wait()
	session.emit(AgentSessionEvent{Type: "initial"})
	if got := calls.Load(); got != workers {
		t.Fatalf("initial listener calls = %d, want %d", got, workers)
	}

	var work sync.WaitGroup
	work.Add(workers + 1)
	go func() {
		defer work.Done()
		for range emissions {
			session.emit(AgentSessionEvent{Type: "tick"})
		}
	}()
	for _, unsubscribe := range unsubscribes {
		go func(unsubscribe func()) {
			defer work.Done()
			for range 3 {
				unsubscribe()
			}
		}(unsubscribe)
	}
	work.Wait()

	if listeners := session.eventListenerSnapshot(); len(listeners) != 0 {
		t.Fatalf("listeners after unsubscribe = %d, want 0", len(listeners))
	}
	before := calls.Load()
	session.emit(AgentSessionEvent{Type: "after_unsubscribe"})
	if got := calls.Load(); got != before {
		t.Fatalf("listener calls after unsubscribe = %d, want %d", got, before)
	}
}
