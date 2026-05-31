package fswatch

import (
	"os"
	"strings"
	"sync"
	"time"
)

const FSWatchRetryDelay = 5 * time.Second

const defaultFSWatchPollInterval = 250 * time.Millisecond

type FSWatchListener func(path string)

type FSWatcher interface {
	Close() error
}

type FSWatchOptions struct {
	PollInterval time.Duration
}

type fsWatchSnapshot struct {
	exists  bool
	modTime time.Time
	size    int64
	isDir   bool
}

type pollingFSWatcher struct {
	done chan struct{}
	once sync.Once
}

func CloseWatcher(watcher FSWatcher) {
	if watcher == nil {
		return
	}
	_ = watcher.Close()
}

func WatchWithErrorHandler(path string, listener FSWatchListener, onError func(), options ...FSWatchOptions) FSWatcher {
	return WatchPathsWithErrorHandler([]string{path}, listener, onError, options...)
}

func WatchPathsWithErrorHandler(paths []string, listener FSWatchListener, onError func(), options ...FSWatchOptions) FSWatcher {
	cleanPaths := make([]string, 0, len(paths))
	for _, path := range paths {
		if trimmed := strings.TrimSpace(path); trimmed != "" {
			cleanPaths = append(cleanPaths, trimmed)
		}
	}
	if len(cleanPaths) == 0 {
		callFSWatchError(onError)
		return nil
	}

	snapshots := make(map[string]fsWatchSnapshot, len(cleanPaths))
	for _, path := range cleanPaths {
		if _, err := os.Stat(path); err != nil {
			callFSWatchError(onError)
			return nil
		}
		snapshots[path] = snapshotFSWatchPath(path)
	}

	pollInterval := defaultFSWatchPollInterval
	if len(options) > 0 && options[0].PollInterval > 0 {
		pollInterval = options[0].PollInterval
	}
	watcher := &pollingFSWatcher{done: make(chan struct{})}
	go watcher.poll(cleanPaths, snapshots, pollInterval, listener)
	return watcher
}

func (w *pollingFSWatcher) Close() error {
	if w == nil {
		return nil
	}
	w.once.Do(func() {
		close(w.done)
	})
	return nil
}

func (w *pollingFSWatcher) poll(paths []string, snapshots map[string]fsWatchSnapshot, interval time.Duration, listener FSWatchListener) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			for _, path := range paths {
				previous := snapshots[path]
				current := snapshotFSWatchPath(path)
				if current != previous {
					snapshots[path] = current
					if listener != nil {
						listener(path)
					}
				}
			}
		}
	}
}

func snapshotFSWatchPath(path string) fsWatchSnapshot {
	stat, err := os.Stat(path)
	if err != nil {
		return fsWatchSnapshot{}
	}
	return fsWatchSnapshot{
		exists:  true,
		modTime: stat.ModTime(),
		size:    stat.Size(),
		isDir:   stat.IsDir(),
	}
}

func callFSWatchError(onError func()) {
	if onError != nil {
		onError()
	}
}
