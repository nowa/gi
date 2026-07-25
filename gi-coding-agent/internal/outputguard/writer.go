// Package outputguard serializes protocol and print output through one
// stateful writer boundary.
package outputguard

import (
	"context"
	"errors"
	"io"
	"sync"
	"syscall"
	"time"
)

const DefaultRetryDelay = 10 * time.Millisecond

// Options configures Writer retry behavior.
type Options struct {
	RetryDelay time.Duration
}

// Writer owns an output destination, serializes complete logical writes, and
// retains the first permanent output error. Synchronous io.Writer calls provide
// the backpressure boundary; Flush is a barrier for all preceding writes.
type Writer struct {
	mu         sync.Mutex
	dst        io.Writer
	retryDelay time.Duration
	firstErr   error
}

// New returns a serialized writer for dst. A nil destination discards output.
func New(dst io.Writer, options Options) *Writer {
	if dst == nil {
		dst = io.Discard
	}
	retryDelay := options.RetryDelay
	if retryDelay <= 0 {
		retryDelay = DefaultRetryDelay
	}
	return &Writer{
		dst:        dst,
		retryDelay: retryDelay,
	}
}

// Write implements io.Writer. It does not return until the whole logical chunk
// has been accepted or a permanent error occurs.
func (w *Writer) Write(data []byte) (int, error) {
	return w.WriteContext(context.Background(), data)
}

// WriteString writes one logical text chunk without allowing concurrent chunks
// to interleave.
func (w *Writer) WriteString(text string) (int, error) {
	return w.WriteStringContext(context.Background(), text)
}

// WriteContext is Write with cancellation support while retrying transient
// output pressure.
func (w *Writer) WriteContext(
	ctx context.Context,
	data []byte,
) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.firstErr != nil {
		return 0, w.firstErr
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return w.writeAllLocked(ctx, data)
}

// WriteStringContext is WriteString with cancellation support.
func (w *Writer) WriteStringContext(
	ctx context.Context,
	text string,
) (int, error) {
	return w.WriteContext(ctx, []byte(text))
}

// Flush waits for preceding writes, flushes buffered destinations when
// supported, and returns the first permanent output error.
func (w *Writer) Flush() error {
	return w.FlushContext(context.Background())
}

// FlushContext is Flush with cancellation support while retrying a transient
// flusher error.
func (w *Writer) FlushContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.firstErr != nil {
		return w.firstErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	flusher, ok := w.dst.(interface{ Flush() error })
	if !ok {
		return nil
	}
	for {
		err := flusher.Flush()
		if err == nil {
			return nil
		}
		if !isTemporary(err) {
			w.firstErr = err
			return err
		}
		if err := waitRetry(ctx, w.retryDelay); err != nil {
			return err
		}
	}
}

// Err returns the first permanent write or flush error.
func (w *Writer) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.firstErr
}

func (w *Writer) writeAllLocked(
	ctx context.Context,
	data []byte,
) (int, error) {
	total := 0
	for total < len(data) {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := w.dst.Write(data[total:])
		remaining := len(data) - total
		if n < 0 || n > remaining {
			w.firstErr = io.ErrShortWrite
			return total, w.firstErr
		}
		total += n
		if err == nil {
			if n == 0 {
				w.firstErr = io.ErrNoProgress
				return total, w.firstErr
			}
			continue
		}
		if total == len(data) || !isTemporary(err) {
			w.firstErr = err
			return total, err
		}
		if err := waitRetry(ctx, w.retryDelay); err != nil {
			return total, err
		}
	}
	return total, nil
}

func isTemporary(err error) bool {
	if errors.Is(err, syscall.ENOBUFS) ||
		errors.Is(err, syscall.EAGAIN) ||
		errors.Is(err, syscall.EWOULDBLOCK) {
		return true
	}
	type temporary interface {
		Temporary() bool
	}
	var candidate temporary
	return errors.As(err, &candidate) && candidate.Temporary()
}

func waitRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
