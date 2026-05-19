package gitui

import "testing"

func TestKillRingPiSemantics(t *testing.T) {
	ring := NewKillRing()
	ring.Push("", KillRingPushOptions{})
	if ring.Len() != 0 || ring.Length() != 0 {
		t.Fatalf("empty push should not add entries, len=%d", ring.Len())
	}

	ring.Push("one", KillRingPushOptions{})
	ring.Push("two", KillRingPushOptions{})
	if got, ok := ring.Peek(); !ok || got != "two" {
		t.Fatalf("peek = %q ok=%v, want two", got, ok)
	}

	ring.Push("!", KillRingPushOptions{Accumulate: true})
	if got, _ := ring.Peek(); got != "two!" {
		t.Fatalf("append accumulate = %q, want two!", got)
	}

	ring.Push("pre-", KillRingPushOptions{Prepend: true, Accumulate: true})
	if got, _ := ring.Peek(); got != "pre-two!" {
		t.Fatalf("prepend accumulate = %q, want pre-two!", got)
	}

	ring.Rotate()
	if got, _ := ring.Peek(); got != "one" {
		t.Fatalf("rotate should move newest entry to front, newest becomes one; got %q", got)
	}

	ring.Rotate()
	if got, _ := ring.Peek(); got != "pre-two!" {
		t.Fatalf("second rotate should restore newest entry, got %q", got)
	}
}

func TestUndoStackPiSemantics(t *testing.T) {
	stack := NewUndoStack(func(in []string) []string {
		return append([]string(nil), in...)
	})
	state := []string{"a", "b"}
	stack.Push(state)
	state[0] = "mutated"

	if stack.Len() != 1 || stack.Length() != 1 {
		t.Fatalf("len = %d length = %d, want 1", stack.Len(), stack.Length())
	}
	got, ok := stack.Pop()
	if !ok {
		t.Fatalf("pop should return a snapshot")
	}
	if got[0] != "a" || got[1] != "b" {
		t.Fatalf("snapshot = %#v, want detached original", got)
	}
	if _, ok := stack.Pop(); ok {
		t.Fatalf("empty pop should return ok=false")
	}

	stack.Push([]string{"x"})
	stack.Push([]string{"y"})
	stack.Clear()
	if stack.Len() != 0 {
		t.Fatalf("clear len = %d, want 0", stack.Len())
	}
}
