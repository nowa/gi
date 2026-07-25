package gicodingagent

import (
	"fmt"
	"sync"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestSessionManagerSerializesConcurrentAppendsAndSnapshots(t *testing.T) {
	manager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	const writers = 8
	const entriesPerWriter = 50
	start := make(chan struct{})
	var writes sync.WaitGroup
	writes.Add(writers)
	for writer := range writers {
		go func(writer int) {
			defer writes.Done()
			<-start
			for index := range entriesPerWriter {
				manager.AppendMessage(llm.UserMessageText(fmt.Sprintf("%d/%d", writer, index)))
			}
		}(writer)
	}

	stopReads := make(chan struct{})
	readsDone := make(chan struct{})
	go func() {
		defer close(readsDone)
		<-start
		for {
			select {
			case <-stopReads:
				return
			default:
				_ = manager.GetEntries()
				_ = manager.GetLeafEntry()
				_ = manager.BuildContextEntries()
				_ = manager.BuildSessionContext()
				_ = manager.GetTree()
			}
		}
	}()

	close(start)
	writes.Wait()
	close(stopReads)
	<-readsDone

	entries := manager.GetEntries()
	if got, want := len(entries), writers*entriesPerWriter; got != want {
		t.Fatalf("entries = %d, want %d", got, want)
	}
	leafID := manager.GetLeafID()
	if leafID == nil {
		t.Fatal("leaf id = nil")
	}
	if branch := manager.GetBranch(*leafID); len(branch) != len(entries) {
		t.Fatalf("branch entries = %d, want %d", len(branch), len(entries))
	}
}
