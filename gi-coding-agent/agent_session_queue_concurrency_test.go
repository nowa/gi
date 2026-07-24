package gicodingagent

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
)

func TestAgentSessionQueuesSerializeConcurrentMutationAndSnapshots(t *testing.T) {
	session := &AgentSession{}
	const writers = 16
	const messagesPerWriter = 50

	start := make(chan struct{})
	var writersDone sync.WaitGroup
	writersDone.Add(writers)
	for writer := range writers {
		go func(writer int) {
			defer writersDone.Done()
			<-start
			for index := range messagesPerWriter {
				message := fmt.Sprintf("%d:%d", writer, index)
				var err error
				if writer%2 == 0 {
					err = session.Steer(message)
				} else {
					err = session.FollowUp(message)
				}
				if err != nil {
					t.Errorf("queue %q: %v", message, err)
					return
				}
			}
		}(writer)
	}

	seen := make(map[string]int, writers*messagesPerWriter)
	var seenMu sync.Mutex
	collect := func(steering, followUp []string) {
		seenMu.Lock()
		defer seenMu.Unlock()
		for _, message := range append(steering, followUp...) {
			seen[message]++
		}
	}

	stopSnapshots := make(chan struct{})
	var snapshotsDone sync.WaitGroup
	snapshotsDone.Add(2)
	go func() {
		defer snapshotsDone.Done()
		for {
			select {
			case <-stopSnapshots:
				return
			default:
				collect(session.ClearQueue())
				runtime.Gosched()
			}
		}
	}()
	go func() {
		defer snapshotsDone.Done()
		for {
			select {
			case <-stopSnapshots:
				return
			default:
				_ = session.PendingMessageCount()
				_ = session.GetSteeringMessages()
				_ = session.GetFollowUpMessages()
				_ = session.GetSteeringQueue()
				_ = session.GetFollowUpQueue()
				runtime.Gosched()
			}
		}
	}()

	close(start)
	writersDone.Wait()
	close(stopSnapshots)
	snapshotsDone.Wait()
	collect(session.ClearQueue())

	expected := writers * messagesPerWriter
	if len(seen) != expected {
		t.Fatalf("unique queued messages = %d, want %d", len(seen), expected)
	}
	for message, count := range seen {
		if count != 1 {
			t.Fatalf("queued message %q collected %d times, want 1", message, count)
		}
	}
}
