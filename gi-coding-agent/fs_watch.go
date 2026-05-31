package gicodingagent

import "github.com/nowa/gi/gi-coding-agent/internal/fswatch"

const FSWatchRetryDelay = fswatch.FSWatchRetryDelay

type FSWatchListener = fswatch.FSWatchListener
type FSWatcher = fswatch.FSWatcher
type FSWatchOptions = fswatch.FSWatchOptions

func CloseWatcher(watcher FSWatcher) {
	fswatch.CloseWatcher(watcher)
}

func WatchWithErrorHandler(path string, listener FSWatchListener, onError func(), options ...FSWatchOptions) FSWatcher {
	return fswatch.WatchWithErrorHandler(path, listener, onError, options...)
}

func WatchPathsWithErrorHandler(paths []string, listener FSWatchListener, onError func(), options ...FSWatchOptions) FSWatcher {
	return fswatch.WatchPathsWithErrorHandler(paths, listener, onError, options...)
}
