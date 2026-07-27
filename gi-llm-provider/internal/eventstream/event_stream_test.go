package eventstream

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestEventStreamPushDoesNotWaitForConsumer(t *testing.T) {
	t.Parallel()

	const terminal = 10_000
	stream := NewEventStream(
		func(event int) bool { return event == terminal },
		func(event int) int { return event },
	)
	produced := make(chan struct{})
	go func() {
		defer close(produced)
		for event := 0; event <= terminal; event++ {
			stream.Push(event)
		}
	}()

	select {
	case <-produced:
	case <-time.After(time.Second):
		t.Fatal("Push blocked while no consumer was reading")
	}

	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result != terminal {
		t.Fatalf("Result() = %d, want %d", result, terminal)
	}

	count := 0
	for event := range stream.Events() {
		if event != count {
			t.Fatalf("event %d = %d", count, event)
		}
		count++
	}
	if count != terminal+1 {
		t.Fatalf("received %d events, want %d", count, terminal+1)
	}
}

func TestEventStreamEndDrainsQueuedEventsAndIgnoresLaterPushes(t *testing.T) {
	t.Parallel()

	stream := NewEventStream(
		func(event int) bool { return event < 0 },
		func(event int) string { return "terminal" },
	)
	stream.Push(1)
	stream.Push(2)
	stream.End("ended")
	stream.Push(3)
	stream.End("replaced")

	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result != "ended" {
		t.Fatalf("Result() = %q, want ended", result)
	}

	var events []int
	for event := range stream.Events() {
		events = append(events, event)
	}
	if len(events) != 2 || events[0] != 1 || events[1] != 2 {
		t.Fatalf("events = %#v, want [1 2]", events)
	}
}

func TestEventStreamConcurrentCompletionIsSafe(t *testing.T) {
	t.Parallel()

	for iteration := 0; iteration < 100; iteration++ {
		stream := NewEventStream(
			func(event int) bool { return event == -1 },
			func(event int) int { return event },
		)

		var producers sync.WaitGroup
		for producer := 0; producer < 8; producer++ {
			producers.Add(1)
			go func(value int) {
				defer producers.Done()
				stream.Push(value)
			}(producer)
		}
		producers.Add(2)
		go func() {
			defer producers.Done()
			stream.Push(-1)
		}()
		go func() {
			defer producers.Done()
			stream.End(42)
		}()
		producers.Wait()

		result, err := stream.Result(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if result != -1 && result != 42 {
			t.Fatalf("Result() = %d, want terminal event or End result", result)
		}
		for range stream.Events() {
		}
	}
}

func TestEventStreamResultHonorsContext(t *testing.T) {
	t.Parallel()

	stream := NewEventStream(
		func(int) bool { return false },
		func(event int) int { return event },
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := stream.Result(ctx); err != context.Canceled {
		t.Fatalf("Result() error = %v, want context.Canceled", err)
	}
	stream.End(0)
	for range stream.Events() {
	}
}
