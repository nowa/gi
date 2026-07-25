package outputguard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestWriterSerializesCompleteConcurrentChunks(t *testing.T) {
	const count = 32
	dst := &oneByteWriter{}
	writer := New(dst, Options{})

	var wait sync.WaitGroup
	wait.Add(count)
	for index := 0; index < count; index++ {
		chunk := fmt.Sprintf("<%02d>", index)
		go func() {
			defer wait.Done()
			if _, err := writer.WriteString(chunk); err != nil {
				t.Errorf("WriteString(%q): %v", chunk, err)
			}
		}()
	}
	wait.Wait()

	output := dst.String()
	if len(output) != count*4 {
		t.Fatalf("output length = %d, want %d", len(output), count*4)
	}
	seen := make(map[string]bool, count)
	for offset := 0; offset < len(output); offset += 4 {
		chunk := output[offset : offset+4]
		if chunk[0] != '<' || chunk[3] != '>' {
			t.Fatalf("interleaved output chunk %q in %q", chunk, output)
		}
		seen[chunk] = true
	}
	if len(seen) != count {
		t.Fatalf("unique output chunks = %d, want %d", len(seen), count)
	}
}

func TestWriterCompletesPartialWrites(t *testing.T) {
	dst := &oneByteWriter{}
	writer := New(dst, Options{})

	n, err := writer.WriteString("complete")
	if err != nil {
		t.Fatal(err)
	}
	if n != len("complete") || dst.String() != "complete" {
		t.Fatalf("write = %d, %q", n, dst.String())
	}
}

func TestWriterRetriesTemporaryErrors(t *testing.T) {
	dst := &temporaryWriter{failures: 2}
	writer := New(dst, Options{RetryDelay: time.Millisecond})

	n, err := writer.WriteString("ready")
	if err != nil {
		t.Fatal(err)
	}
	if n != len("ready") ||
		dst.String() != "ready" ||
		dst.Calls() != 3 {
		t.Fatalf(
			"write = %d, %q, calls=%d",
			n,
			dst.String(),
			dst.Calls(),
		)
	}
}

func TestWriterRetriesNoBufferSpaceError(t *testing.T) {
	dst := &errnoWriter{failures: 1}
	writer := New(dst, Options{RetryDelay: time.Millisecond})

	if _, err := writer.WriteString("ready"); err != nil {
		t.Fatal(err)
	}
	if dst.String() != "ready" || dst.Calls() != 2 {
		t.Fatalf(
			"output = %q, calls = %d",
			dst.String(),
			dst.Calls(),
		)
	}
}

func TestWriterRetainsFirstPermanentError(t *testing.T) {
	writeErr := errors.New("output failed")
	dst := &errorWriter{err: writeErr}
	writer := New(dst, Options{})

	if _, err := writer.WriteString("first"); !errors.Is(
		err,
		writeErr,
	) {
		t.Fatalf("first write error = %v", err)
	}
	if _, err := writer.WriteString("second"); !errors.Is(
		err,
		writeErr,
	) {
		t.Fatalf("second write error = %v", err)
	}
	if !errors.Is(writer.Err(), writeErr) || dst.calls != 1 {
		t.Fatalf("sticky error = %v, calls = %d", writer.Err(), dst.calls)
	}
}

func TestWriterFlushIsBarrierForPriorWrite(t *testing.T) {
	dst := newBarrierWriter()
	writer := New(dst, Options{})
	writeDone := make(chan error, 1)
	go func() {
		_, err := writer.WriteString("record")
		writeDone <- err
	}()
	<-dst.writeStarted

	flushDone := make(chan error, 1)
	go func() {
		flushDone <- writer.Flush()
	}()
	select {
	case err := <-flushDone:
		t.Fatalf("Flush returned before write completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(dst.releaseWrite)
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	if err := <-flushDone; err != nil {
		t.Fatal(err)
	}
	if got := dst.Operations(); fmt.Sprint(got) != "[write flush]" {
		t.Fatalf("operations = %#v", got)
	}
}

func TestWriterCancellationDoesNotPoisonDestination(t *testing.T) {
	dst := &temporaryWriter{failures: 100}
	writer := New(dst, Options{RetryDelay: time.Millisecond})
	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Millisecond,
	)
	defer cancel()

	if _, err := writer.WriteContext(
		ctx,
		[]byte("later"),
	); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled write error = %v", err)
	}
	if writer.Err() != nil {
		t.Fatalf("cancelled write poisoned writer: %v", writer.Err())
	}
	dst.AllowWrites()
	if _, err := writer.WriteString("later"); err != nil {
		t.Fatal(err)
	}
	if dst.String() != "later" {
		t.Fatalf("output = %q", dst.String())
	}
}

type oneByteWriter struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (w *oneByteWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.buffer.WriteByte(data[0]); err != nil {
		return 0, err
	}
	runtime.Gosched()
	return 1, nil
}

func (w *oneByteWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String()
}

type temporaryWriteError struct{}

func (temporaryWriteError) Error() string   { return "temporary pressure" }
func (temporaryWriteError) Temporary() bool { return true }

type temporaryWriter struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	failures int
	calls    int
}

func (w *temporaryWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls++
	if w.failures > 0 {
		w.failures--
		return 0, temporaryWriteError{}
	}
	return w.buffer.Write(data)
}

func (w *temporaryWriter) Calls() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls
}

func (w *temporaryWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String()
}

func (w *temporaryWriter) AllowWrites() {
	w.mu.Lock()
	w.failures = 0
	w.mu.Unlock()
}

type errorWriter struct {
	err   error
	calls int
}

func (w *errorWriter) Write([]byte) (int, error) {
	w.calls++
	return 0, w.err
}

type errnoWriter struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	failures int
	calls    int
}

func (w *errnoWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls++
	if w.failures > 0 {
		w.failures--
		return 0, syscall.ENOBUFS
	}
	return w.buffer.Write(data)
}

func (w *errnoWriter) Calls() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls
}

func (w *errnoWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String()
}

type barrierWriter struct {
	writeStarted chan struct{}
	releaseWrite chan struct{}
	startOnce    sync.Once
	mu           sync.Mutex
	operations   []string
}

func newBarrierWriter() *barrierWriter {
	return &barrierWriter{
		writeStarted: make(chan struct{}),
		releaseWrite: make(chan struct{}),
	}
}

func (w *barrierWriter) Write(data []byte) (int, error) {
	w.startOnce.Do(func() { close(w.writeStarted) })
	<-w.releaseWrite
	w.mu.Lock()
	w.operations = append(w.operations, "write")
	w.mu.Unlock()
	return len(data), nil
}

func (w *barrierWriter) Flush() error {
	w.mu.Lock()
	w.operations = append(w.operations, "flush")
	w.mu.Unlock()
	return nil
}

func (w *barrierWriter) Operations() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.operations...)
}

var _ io.Writer = (*Writer)(nil)
