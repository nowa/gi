package gicodingagent

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestFSWatchPiStyleHelpers(t *testing.T) {
	t.Run("safe close ignores nil and repeated close", func(t *testing.T) {
		CloseWatcher(nil)
		path := filepath.Join(t.TempDir(), "watched.txt")
		writeFooterFile(t, path, "one")
		watcher := WatchWithErrorHandler(path, nil, nil, FSWatchOptions{PollInterval: 5 * time.Millisecond})
		if watcher == nil {
			t.Fatal("watcher should be created")
		}
		CloseWatcher(watcher)
		CloseWatcher(watcher)
	})

	t.Run("calls error handler when watch cannot start", func(t *testing.T) {
		var errors int32
		watcher := WatchWithErrorHandler(filepath.Join(t.TempDir(), "missing.txt"), nil, func() {
			atomic.AddInt32(&errors, 1)
		}, FSWatchOptions{PollInterval: 5 * time.Millisecond})
		if watcher != nil {
			t.Fatalf("watcher = %#v, want nil", watcher)
		}
		if got := atomic.LoadInt32(&errors); got != 1 {
			t.Fatalf("errors = %d, want 1", got)
		}
	})

	t.Run("notifies listener on file changes until closed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "watched.txt")
		writeFooterFile(t, path, "one")
		var changes int32
		watcher := WatchWithErrorHandler(path, func(changedPath string) {
			if changedPath != path {
				t.Errorf("changed path = %q, want %q", changedPath, path)
			}
			atomic.AddInt32(&changes, 1)
		}, nil, FSWatchOptions{PollInterval: 5 * time.Millisecond})
		if watcher == nil {
			t.Fatal("watcher should be created")
		}

		writeFooterFile(t, path, "two")
		waitForFooterCondition(t, func() bool { return atomic.LoadInt32(&changes) > 0 })
		CloseWatcher(watcher)

		before := atomic.LoadInt32(&changes)
		writeFooterFile(t, path, "three")
		time.Sleep(30 * time.Millisecond)
		if got := atomic.LoadInt32(&changes); got != before {
			t.Fatalf("changes after close = %d, want %d", got, before)
		}
	})

	t.Run("notifies listener for any watched path", func(t *testing.T) {
		dir := t.TempDir()
		first := filepath.Join(dir, "one.txt")
		second := filepath.Join(dir, "two.txt")
		writeFooterFile(t, first, "one")
		writeFooterFile(t, second, "two")
		var sawSecond atomic.Bool
		watcher := WatchPathsWithErrorHandler([]string{first, second}, func(changedPath string) {
			if changedPath == second {
				sawSecond.Store(true)
			}
		}, nil, FSWatchOptions{PollInterval: 5 * time.Millisecond})
		if watcher == nil {
			t.Fatal("watcher should be created")
		}
		defer CloseWatcher(watcher)

		if err := os.WriteFile(second, []byte("updated"), 0o600); err != nil {
			t.Fatal(err)
		}
		waitForFooterCondition(t, sawSecond.Load)
	})
}
