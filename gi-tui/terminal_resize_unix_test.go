//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package gitui

import (
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestProcessTerminalResizeSignalInvokesHandler(t *testing.T) {
	terminal := NewProcessTerminalWithIO(strings.NewReader(""), &safeBuffer{}, 80, 24)
	var calls atomic.Int32
	resized := make(chan struct{}, 4)

	terminal.Start(func(string) {}, func() {
		calls.Add(1)
		select {
		case resized <- struct{}{}:
		default:
		}
	})
	defer terminal.Stop()

	select {
	case <-resized:
	case <-time.After(time.Second):
		t.Fatalf("initial resize callback was not invoked")
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatalf("send SIGWINCH: %v", err)
	}
	select {
	case <-resized:
	case <-time.After(time.Second):
		t.Fatalf("SIGWINCH did not invoke resize callback")
	}
	if calls.Load() < 2 {
		t.Fatalf("resize calls = %d, want at least initial + SIGWINCH", calls.Load())
	}
}

func TestProcessTerminalStopRemovesResizeSignalWatcher(t *testing.T) {
	terminal := NewProcessTerminalWithIO(strings.NewReader(""), &safeBuffer{}, 80, 24)
	var calls atomic.Int32
	resized := make(chan struct{}, 4)

	terminal.Start(func(string) {}, func() {
		calls.Add(1)
		select {
		case resized <- struct{}{}:
		default:
		}
	})
	select {
	case <-resized:
	case <-time.After(time.Second):
		t.Fatalf("initial resize callback was not invoked")
	}

	terminal.Stop()
	before := calls.Load()
	if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatalf("send SIGWINCH: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if after := calls.Load(); after != before {
		t.Fatalf("resize callback fired after Stop: before=%d after=%d", before, after)
	}
}
