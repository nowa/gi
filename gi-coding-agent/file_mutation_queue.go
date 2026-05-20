package gicodingagent

import (
	"path/filepath"
	"sync"
)

var globalFileMutationQueues = struct {
	sync.Mutex
	queues map[string]*fileMutationQueue
}{queues: map[string]*fileMutationQueue{}}

type fileMutationQueue struct {
	mu   sync.Mutex
	refs int
}

func WithFileMutationQueue(filePath string, fn func() error) error {
	key := fileMutationQueueKey(filePath)
	queue := acquireFileMutationQueue(key)
	queue.mu.Lock()
	defer releaseFileMutationQueue(key, queue)
	return fn()
}

func fileMutationQueueKey(filePath string) string {
	resolvedPath, err := filepath.Abs(filePath)
	if err != nil {
		resolvedPath = filepath.Clean(filePath)
	}
	if realPath, err := filepath.EvalSymlinks(resolvedPath); err == nil {
		return realPath
	}
	return resolvedPath
}

func acquireFileMutationQueue(key string) *fileMutationQueue {
	globalFileMutationQueues.Lock()
	defer globalFileMutationQueues.Unlock()
	queue := globalFileMutationQueues.queues[key]
	if queue == nil {
		queue = &fileMutationQueue{}
		globalFileMutationQueues.queues[key] = queue
	}
	queue.refs++
	return queue
}

func releaseFileMutationQueue(key string, queue *fileMutationQueue) {
	queue.mu.Unlock()
	globalFileMutationQueues.Lock()
	defer globalFileMutationQueues.Unlock()
	queue.refs--
	if queue.refs == 0 && globalFileMutationQueues.queues[key] == queue {
		delete(globalFileMutationQueues.queues, key)
	}
}
