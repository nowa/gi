package gicodingagent

import "github.com/nowa/gi/gi-coding-agent/internal/toolqueue"

func WithFileMutationQueue(filePath string, fn func() error) error {
	return toolqueue.WithFileMutationQueue(filePath, fn)
}
