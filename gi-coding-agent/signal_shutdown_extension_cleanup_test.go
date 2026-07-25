package gicodingagent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

func TestCLIInteractiveSignalShutdownExtensionCleanupPiRegression(t *testing.T) {
	t.Run("re-entrant shutdown is a no-op", func(t *testing.T) {
		runtimeHost := &disposeCountingPrintModeHost{
			PrintModeRuntimeHost: newOfflineInteractiveRuntimeHost(t),
		}
		host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
			RuntimeHost: runtimeHost,
			Terminal:    gitui.NewVirtualTerminal(80, 12),
		})
		if err != nil {
			t.Fatal(err)
		}

		errCh := make(chan error, 1)
		go func() {
			errCh <- host.RunContext(context.Background())
		}()
		waitForHostEditor(t, host)

		var callers sync.WaitGroup
		for range 8 {
			callers.Add(1)
			go func() {
				defer callers.Done()
				host.Stop()
			}()
		}
		callers.Wait()

		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("RunContext returned %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("interactive TUI host did not stop")
		}

		host.Stop()
		if got := runtimeHost.disposeCalls.Load(); got != 1 {
			t.Fatalf("runtime dispose calls = %d, want 1", got)
		}
	})
}

type disposeCountingPrintModeHost struct {
	PrintModeRuntimeHost
	disposeCalls atomic.Int32
}

func (h *disposeCountingPrintModeHost) Dispose() error {
	h.disposeCalls.Add(1)
	return h.PrintModeRuntimeHost.Dispose()
}
