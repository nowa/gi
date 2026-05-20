package gicodingagent

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestWithFileMutationQueueSerializesOperationsForSameFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "same.txt")
	var order orderedStrings
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondDone := make(chan struct{})

	go func() {
		err := WithFileMutationQueue(path, func() error {
			order.add("first:start")
			close(firstStarted)
			<-releaseFirst
			order.add("first:end")
			return nil
		})
		if err != nil {
			t.Errorf("first operation: %v", err)
		}
	}()
	<-firstStarted
	go func() {
		err := WithFileMutationQueue(path, func() error {
			order.add("second:start")
			order.add("second:end")
			return nil
		})
		if err != nil {
			t.Errorf("second operation: %v", err)
		}
		close(secondDone)
	}()
	time.Sleep(20 * time.Millisecond)
	if got := order.values(); !reflect.DeepEqual(got, []string{"first:start"}) {
		t.Fatalf("order before release = %#v", got)
	}
	close(releaseFirst)
	<-secondDone

	want := []string{"first:start", "first:end", "second:start", "second:end"}
	if got := order.values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %#v, want %#v", got, want)
	}
}

func TestWithFileMutationQueueAllowsDifferentFilesInParallel(t *testing.T) {
	dir := t.TempDir()
	var order orderedStrings
	aStarted := make(chan struct{})
	bStarted := make(chan struct{})
	releaseA := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)
		errA := make(chan error, 1)
		errB := make(chan error, 1)
		go func() {
			errA <- WithFileMutationQueue(filepath.Join(dir, "a.txt"), func() error {
				order.add("a:start")
				close(aStarted)
				<-releaseA
				order.add("a:end")
				return nil
			})
		}()
		<-aStarted
		go func() {
			errB <- WithFileMutationQueue(filepath.Join(dir, "b.txt"), func() error {
				order.add("b:start")
				close(bStarted)
				order.add("b:end")
				return nil
			})
		}()
		<-bStarted
		close(releaseA)
		if err := <-errA; err != nil {
			t.Errorf("a operation: %v", err)
		}
		if err := <-errB; err != nil {
			t.Errorf("b operation: %v", err)
		}
	}()
	<-done

	got := order.values()
	if indexOf(got, "b:start") > indexOf(got, "a:end") {
		t.Fatalf("different file operation did not run in parallel: %#v", got)
	}
}

func TestWithFileMutationQueueUsesSameQueueForSymlinkAliases(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "target.txt")
	symlinkPath := filepath.Join(dir, "alias.txt")
	if err := os.WriteFile(targetPath, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Fatal(err)
	}

	var order orderedStrings
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondDone := make(chan struct{})
	go func() {
		err := WithFileMutationQueue(targetPath, func() error {
			order.add("target:start")
			close(firstStarted)
			<-releaseFirst
			order.add("target:end")
			return nil
		})
		if err != nil {
			t.Errorf("target operation: %v", err)
		}
	}()
	<-firstStarted
	go func() {
		err := WithFileMutationQueue(symlinkPath, func() error {
			order.add("alias:start")
			order.add("alias:end")
			return nil
		})
		if err != nil {
			t.Errorf("alias operation: %v", err)
		}
		close(secondDone)
	}()
	time.Sleep(20 * time.Millisecond)
	close(releaseFirst)
	<-secondDone

	want := []string{"target:start", "target:end", "alias:start", "alias:end"}
	if got := order.values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %#v, want %#v", got, want)
	}
}

func TestQueuedEditToolPreservesBothParallelEditsOnSameFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "parallel-edit.txt")
	if err := os.WriteFile(filePath, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	editTool := NewEditTool(dir, delayedFileToolOperations(30*time.Millisecond, 30*time.Millisecond))

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, input := range []EditToolInput{
		{Path: filePath, Edits: []Edit{{OldText: "alpha", NewText: "ALPHA"}}},
		{Path: filePath, Edits: []Edit{{OldText: "beta", NewText: "BETA"}}},
	} {
		wg.Add(1)
		go func(input EditToolInput) {
			defer wg.Done()
			_, err := editTool.Execute("call", input)
			errs <- err
		}(input)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(content); got != "ALPHA\nBETA\ngamma\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestQueuedEditAndWriteToolsShareQueue(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "mixed.txt")
	if err := os.WriteFile(filePath, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	readStarted := make(chan struct{})
	editTool := NewEditTool(dir, FileToolOperations{
		Access: statAccess,
		ReadFile: func(path string) ([]byte, error) {
			close(readStarted)
			time.Sleep(30 * time.Millisecond)
			return os.ReadFile(path)
		},
		WriteFile: func(path string, content []byte) error {
			time.Sleep(30 * time.Millisecond)
			return os.WriteFile(path, content, 0o644)
		},
	})
	writeTool := NewWriteTool(dir, FileToolOperations{
		MkdirAll: func(string) error { return nil },
		WriteFile: func(path string, content []byte) error {
			time.Sleep(10 * time.Millisecond)
			return os.WriteFile(path, content, 0o644)
		},
	})

	editErr := make(chan error, 1)
	go func() {
		_, err := editTool.Execute("call-1", EditToolInput{
			Path:  filePath,
			Edits: []Edit{{OldText: "original", NewText: "edited"}},
		})
		editErr <- err
	}()
	<-readStarted
	writeErr := make(chan error, 1)
	go func() {
		_, err := writeTool.Execute("call-2", WriteToolInput{Path: filePath, Content: "replacement\n"})
		writeErr <- err
	}()

	if err := <-editErr; err != nil {
		t.Fatal(err)
	}
	if err := <-writeErr; err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(content); got != "replacement\n" {
		t.Fatalf("content = %q", got)
	}
}

type orderedStrings struct {
	mu    sync.Mutex
	items []string
}

func (o *orderedStrings) add(value string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.items = append(o.items, value)
}

func (o *orderedStrings) values() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.items...)
}

func delayedFileToolOperations(readDelay, writeDelay time.Duration) FileToolOperations {
	return FileToolOperations{
		Access: statAccess,
		ReadFile: func(path string) ([]byte, error) {
			time.Sleep(readDelay)
			return os.ReadFile(path)
		},
		WriteFile: func(path string, content []byte) error {
			time.Sleep(writeDelay)
			return os.WriteFile(path, content, 0o644)
		},
		MkdirAll: func(path string) error {
			return os.MkdirAll(path, 0o755)
		},
	}
}

func statAccess(path string) error {
	_, err := os.Stat(path)
	return err
}

func indexOf(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}
