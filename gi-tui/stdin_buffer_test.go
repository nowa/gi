package gitui

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

type stdinBufferRecorder struct {
	mu    sync.Mutex
	data  []string
	paste []string
}

func newRecordedStdinBuffer(timeout time.Duration) (*StdinBuffer, *stdinBufferRecorder) {
	buffer := NewStdinBuffer(StdinBufferOptions{Timeout: timeout})
	recorder := &stdinBufferRecorder{}
	buffer.OnData(func(data string) {
		recorder.recordData(data)
	})
	buffer.OnPaste(func(data string) {
		recorder.recordPaste(data)
	})
	return buffer, recorder
}

func (r *stdinBufferRecorder) recordData(data string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = append(r.data, data)
}

func (r *stdinBufferRecorder) recordPaste(data string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paste = append(r.paste, data)
}

func (r *stdinBufferRecorder) Data() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.data...)
}

func (r *stdinBufferRecorder) Paste() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paste...)
}

func requireStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestStdinBufferPiRegularAndCompleteSequences(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "regular character", input: "a", want: []string{"a"}},
		{name: "regular runes split individually", input: "hello 世界", want: []string{"h", "e", "l", "l", "o", " ", "世", "界"}},
		{name: "SGR mouse", input: "\x1b[<35;20;5m", want: []string{"\x1b[<35;20;5m"}},
		{name: "arrow key", input: "\x1b[A", want: []string{"\x1b[A"}},
		{name: "function key", input: "\x1b[11~", want: []string{"\x1b[11~"}},
		{name: "meta key", input: "\x1ba", want: []string{"\x1ba"}},
		{name: "SS3 sequence", input: "\x1bOA", want: []string{"\x1bOA"}},
		{name: "old-style mouse keeps three payload bytes", input: "\x1b[M abc", want: []string{"\x1b[M ab", "c"}},
		{name: "empty input emits empty event", input: "", want: []string{""}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buffer, recorder := newRecordedStdinBuffer(time.Second)
			buffer.Process(tc.input)
			requireStringSlice(t, recorder.Data(), tc.want)
		})
	}
}

func TestStdinBufferPiPartialAndMixedSequences(t *testing.T) {
	t.Run("buffers incomplete CSI across chunks", func(t *testing.T) {
		buffer, recorder := newRecordedStdinBuffer(time.Second)
		buffer.Process("\x1b[")
		buffer.Process("1;")
		requireStringSlice(t, recorder.Data(), nil)
		if got := buffer.GetBuffer(); got != "\x1b[1;" {
			t.Fatalf("buffer = %q, want partial CSI", got)
		}
		buffer.Process("5H")
		requireStringSlice(t, recorder.Data(), []string{"\x1b[1;5H"})
		if got := buffer.GetBuffer(); got != "" {
			t.Fatalf("buffer after complete sequence = %q, want empty", got)
		}
	})

	t.Run("buffers split mouse sequence across many chunks", func(t *testing.T) {
		buffer, recorder := newRecordedStdinBuffer(time.Second)
		for _, chunk := range []string{"\x1b", "[", "<", "3", "5", ";", "2", "0", ";", "5", "m"} {
			buffer.Process(chunk)
		}
		requireStringSlice(t, recorder.Data(), []string{"\x1b[<35;20;5m"})
	})

	t.Run("mixed text before and after escape sequences", func(t *testing.T) {
		buffer, recorder := newRecordedStdinBuffer(time.Second)
		buffer.Process("abc\x1b[A\x1b[Bd")
		requireStringSlice(t, recorder.Data(), []string{"a", "b", "c", "\x1b[A", "\x1b[B", "d"})
	})

	t.Run("partial with preceding characters waits for completion", func(t *testing.T) {
		buffer, recorder := newRecordedStdinBuffer(time.Second)
		buffer.Process("abc\x1b[<35")
		requireStringSlice(t, recorder.Data(), []string{"a", "b", "c"})
		if got := buffer.GetBuffer(); got != "\x1b[<35" {
			t.Fatalf("buffer = %q, want partial mouse sequence", got)
		}
		buffer.Process(";20;5m")
		requireStringSlice(t, recorder.Data(), []string{"a", "b", "c", "\x1b[<35;20;5m"})
	})

	t.Run("timeout flushes incomplete sequence through callbacks", func(t *testing.T) {
		buffer, recorder := newRecordedStdinBuffer(5 * time.Millisecond)
		buffer.Process("\x1b[<35")
		requireStringSlice(t, recorder.Data(), nil)
		time.Sleep(25 * time.Millisecond)
		requireStringSlice(t, recorder.Data(), []string{"\x1b[<35"})
	})

	t.Run("explicit flush returns incomplete sequence without callback", func(t *testing.T) {
		buffer, recorder := newRecordedStdinBuffer(time.Second)
		buffer.Process("\x1b[<35")
		requireStringSlice(t, buffer.Flush(), []string{"\x1b[<35"})
		requireStringSlice(t, recorder.Data(), nil)
		if got := buffer.GetBuffer(); got != "" {
			t.Fatalf("buffer after flush = %q, want empty", got)
		}
		if got := buffer.Flush(); len(got) != 0 {
			t.Fatalf("empty flush = %#v, want empty", got)
		}
	})
}

func TestStdinBufferPiKittySequencesAndPrintableDedupe(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "Kitty press", input: "\x1b[97u", want: []string{"\x1b[97u"}},
		{name: "Kitty release", input: "\x1b[97;1:3u", want: []string{"\x1b[97;1:3u"}},
		{name: "batched press release", input: "\x1b[97u\x1b[97;1:3u", want: []string{"\x1b[97u", "\x1b[97;1:3u"}},
		{name: "Kitty arrow with event type", input: "\x1b[1;1:1A", want: []string{"\x1b[1;1:1A"}},
		{name: "Kitty function key with event type", input: "\x1b[3;1:3~", want: []string{"\x1b[3;1:3~"}},
		{name: "split ESC ESC CSI", input: "\x1b\x1b[27;129:3u", want: []string{"\x1b", "\x1b[27;129:3u"}},
		{name: "plain text mixed with Kitty release", input: "a\x1b[97;1:3u", want: []string{"a", "\x1b[97;1:3u"}},
		{name: "dedupe raw printable after Kitty press", input: "\x1b[224uà", want: []string{"\x1b[224u"}},
		{name: "keep non-matching raw printable after Kitty press", input: "\x1b[97ub", want: []string{"\x1b[97u", "b"}},
		{name: "keep raw printable after modified Kitty press", input: "\x1b[64;3u@", want: []string{"\x1b[64;3u", "@"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buffer, recorder := newRecordedStdinBuffer(time.Second)
			buffer.Process(tc.input)
			requireStringSlice(t, recorder.Data(), tc.want)
		})
	}

	t.Run("dedupe raw printable across chunks", func(t *testing.T) {
		buffer, recorder := newRecordedStdinBuffer(time.Second)
		buffer.Process("\x1b[64u")
		buffer.Process("@")
		requireStringSlice(t, recorder.Data(), []string{"\x1b[64u"})
	})
}

func TestStdinBufferPiC1AndControlStringSequences(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "raw C1 CSI", input: "\x9bA", want: []string{"\x9bA"}},
		{name: "UTF-8 C1 CSI", input: "\xc2\x9bA", want: []string{"\xc2\x9bA"}},
		{name: "OSC BEL terminated", input: "\x1b]0;title\x07x", want: []string{"\x1b]0;title\x07", "x"}},
		{name: "OSC ST terminated", input: "\x1b]0;title\x1b\\x", want: []string{"\x1b]0;title\x1b\\", "x"}},
		{name: "DCS ST terminated", input: "\x1bPpayload\x1b\\", want: []string{"\x1bPpayload\x1b\\"}},
		{name: "APC ST terminated", input: "\x1b_payload\x1b\\", want: []string{"\x1b_payload\x1b\\"}},
		{name: "PM ST terminated", input: "\x1b^payload\x1b\\", want: []string{"\x1b^payload\x1b\\"}},
		{name: "SOS ST terminated", input: "\x1bXpayload\x1b\\", want: []string{"\x1bXpayload\x1b\\"}},
		{name: "raw C1 OSC ST terminated", input: "\x9d0;title\x9c", want: []string{"\x9d0;title\x9c"}},
		{name: "UTF-8 C1 OSC ST terminated", input: "\xc2\x9d0;title\xc2\x9c", want: []string{"\xc2\x9d0;title\xc2\x9c"}},
		{name: "raw C1 DCS ST terminated", input: "\x90payload\x9c", want: []string{"\x90payload\x9c"}},
		{name: "UTF-8 C1 DCS ST terminated", input: "\xc2\x90payload\xc2\x9c", want: []string{"\xc2\x90payload\xc2\x9c"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buffer, recorder := newRecordedStdinBuffer(time.Second)
			buffer.Process(tc.input)
			requireStringSlice(t, recorder.Data(), tc.want)
		})
	}

	t.Run("DCS ignores BEL and waits for ST", func(t *testing.T) {
		buffer, recorder := newRecordedStdinBuffer(time.Second)
		buffer.Process("\x1bPpayload\x07")
		requireStringSlice(t, recorder.Data(), nil)
		if got := buffer.GetBuffer(); got != "\x1bPpayload\x07" {
			t.Fatalf("buffer = %q, want incomplete DCS", got)
		}
		buffer.Process("\x1b\\")
		requireStringSlice(t, recorder.Data(), []string{"\x1bPpayload\x07\x1b\\"})
	})

	t.Run("single high-bit byte converts to legacy ESC meta sequence", func(t *testing.T) {
		buffer, recorder := newRecordedStdinBuffer(time.Second)
		buffer.ProcessBytes([]byte{0xe1})
		requireStringSlice(t, recorder.Data(), []string{"\x1ba"})
	})
}

func TestStdinBufferPiPasteClearAndDestroy(t *testing.T) {
	t.Run("bracketed paste emits paste only", func(t *testing.T) {
		buffer, recorder := newRecordedStdinBuffer(time.Second)
		buffer.Process("\x1b[200~Hello 世界 🎉\nline2\x1b[201~")
		requireStringSlice(t, recorder.Data(), nil)
		requireStringSlice(t, recorder.Paste(), []string{"Hello 世界 🎉\nline2"})
	})

	t.Run("bracketed paste chunks and surrounding data", func(t *testing.T) {
		buffer, recorder := newRecordedStdinBuffer(time.Second)
		buffer.Process("a")
		buffer.Process("\x1b[200~hello ")
		buffer.Process("world\x1b[201~")
		buffer.Process("b")
		requireStringSlice(t, recorder.Data(), []string{"a", "b"})
		requireStringSlice(t, recorder.Paste(), []string{"hello world"})
	})

	t.Run("clear drops buffered content without emitting", func(t *testing.T) {
		buffer, recorder := newRecordedStdinBuffer(time.Second)
		buffer.Process("\x1b[<35")
		buffer.Clear()
		if got := buffer.GetBuffer(); got != "" {
			t.Fatalf("buffer after clear = %q, want empty", got)
		}
		requireStringSlice(t, recorder.Data(), nil)
	})

	t.Run("destroy clears pending timeout", func(t *testing.T) {
		buffer, recorder := newRecordedStdinBuffer(5 * time.Millisecond)
		buffer.Process("\x1b[<35")
		buffer.Destroy()
		time.Sleep(25 * time.Millisecond)
		requireStringSlice(t, recorder.Data(), nil)
	})
}
