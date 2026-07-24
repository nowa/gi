package tools

import (
	"context"
	"errors"
	"path/filepath"
	"sync"

	harnessenv "github.com/nowa/gi/gi-agent-core/harness/env"
)

var defaultFileMutationQueue = NewFileMutationQueue()

type FileMutationQueue struct {
	mu     sync.Mutex
	queues map[string]*fileMutation
}

type fileMutation struct {
	mu   sync.Mutex
	refs int
}

func NewFileMutationQueue() *FileMutationQueue {
	return &FileMutationQueue{queues: make(map[string]*fileMutation)}
}

// With serializes mutations that resolve to the same canonical path. The lock
// remains held until fn returns, even when ctx is cancelled while fn is
// settling.
func (q *FileMutationQueue) With(ctx context.Context, env harnessenv.ExecutionEnv, path string, fn func() error) error {
	if q == nil {
		q = defaultFileMutationQueue
	}
	key, err := mutationQueueKey(env, path)
	if err != nil {
		return err
	}
	mutation := q.acquire(key)
	mutation.mu.Lock()
	defer q.release(key, mutation)

	if err := contextError(ctx); err != nil {
		return err
	}
	return fn()
}

func mutationQueueKey(env harnessenv.ExecutionEnv, path string) (string, error) {
	absolutePath := env.AbsolutePath(path)
	canonicalPath, err := env.CanonicalPath(context.Background(), absolutePath)
	if err == nil {
		return canonicalPath, nil
	}
	var fileError *harnessenv.FileError
	if errors.As(err, &fileError) &&
		(fileError.Code == harnessenv.FileErrorNotFound || fileError.Code == harnessenv.FileErrorNotSupported) {
		return filepath.Clean(absolutePath), nil
	}
	return "", err
}

func (q *FileMutationQueue) acquire(key string) *fileMutation {
	q.mu.Lock()
	defer q.mu.Unlock()
	mutation := q.queues[key]
	if mutation == nil {
		mutation = &fileMutation{}
		q.queues[key] = mutation
	}
	mutation.refs++
	return mutation
}

func (q *FileMutationQueue) release(key string, mutation *fileMutation) {
	mutation.mu.Unlock()
	q.mu.Lock()
	defer q.mu.Unlock()
	mutation.refs--
	if mutation.refs == 0 && q.queues[key] == mutation {
		delete(q.queues, key)
	}
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return errors.New("Operation aborted")
	}
	return nil
}
