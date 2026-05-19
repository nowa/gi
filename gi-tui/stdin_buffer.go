package gitui

import (
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	bracketedPasteStart = "\x1b[200~"
	bracketedPasteEnd   = "\x1b[201~"
)

type StdinBufferOptions struct {
	Timeout time.Duration
}

type StdinBufferEventMap struct {
	Data  func(string)
	Paste func(string)
}

type stdinBufferEvent struct {
	paste bool
	data  string
}

type StdinBuffer struct {
	mu          sync.Mutex
	buffer      string
	pasteMode   bool
	pasteBuffer string
	timeout     time.Duration
	timer       *time.Timer
	onData      []func(string)
	onPaste     []func(string)
	dedupeRune  rune
	hasDedupe   bool
}

func NewStdinBuffer(options ...StdinBufferOptions) *StdinBuffer {
	timeout := 10 * time.Millisecond
	if len(options) > 0 && options[0].Timeout > 0 {
		timeout = options[0].Timeout
	}
	return &StdinBuffer{timeout: timeout}
}

func (b *StdinBuffer) OnData(fn func(string)) {
	if fn != nil {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.onData = append(b.onData, fn)
	}
}

func (b *StdinBuffer) OnPaste(fn func(string)) {
	if fn != nil {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.onPaste = append(b.onPaste, fn)
	}
}

func (b *StdinBuffer) Process(data string) {
	b.mu.Lock()
	events := make([]stdinBufferEvent, 0)
	b.stopTimerLocked()
	if data == "" && b.buffer == "" && !b.pasteMode {
		b.emitDataLocked("", &events)
		b.mu.Unlock()
		b.deliverEvents(events)
		return
	}
	b.buffer += data

	for {
		if b.pasteMode {
			b.pasteBuffer += b.buffer
			b.buffer = ""
			end := strings.Index(b.pasteBuffer, bracketedPasteEnd)
			if end < 0 {
				b.mu.Unlock()
				b.deliverEvents(events)
				return
			}
			paste := b.pasteBuffer[:end]
			remaining := b.pasteBuffer[end+len(bracketedPasteEnd):]
			b.pasteMode = false
			b.pasteBuffer = ""
			b.clearDedupeLocked()
			b.emitPasteLocked(paste, &events)
			b.buffer = remaining
			continue
		}

		start := strings.Index(b.buffer, bracketedPasteStart)
		if start < 0 {
			break
		}
		if start > 0 {
			before := b.buffer[:start]
			sequences, remainder := extractCompleteInputSequences(before)
			for _, sequence := range sequences {
				b.emitDataWithDedupeLocked(sequence, &events)
			}
			if remainder != "" {
				b.buffer = remainder + b.buffer[start:]
				b.startTimerLocked()
				b.mu.Unlock()
				b.deliverEvents(events)
				return
			}
		}
		b.clearDedupeLocked()
		b.buffer = b.buffer[start+len(bracketedPasteStart):]
		b.pasteMode = true
	}

	sequences, remainder := extractCompleteInputSequences(b.buffer)
	for _, sequence := range sequences {
		b.emitDataWithDedupeLocked(sequence, &events)
	}
	b.buffer = remainder
	if b.buffer != "" {
		b.startTimerLocked()
	}
	b.mu.Unlock()
	b.deliverEvents(events)
}

func (b *StdinBuffer) ProcessBytes(data []byte) {
	if len(data) == 1 && data[0] > 127 {
		b.Process("\x1b" + string([]byte{data[0] - 128}))
		return
	}
	b.Process(string(data))
}

func (b *StdinBuffer) Flush() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopTimerLocked()
	if b.buffer == "" {
		return nil
	}
	out := []string{b.buffer}
	b.buffer = ""
	b.clearDedupeLocked()
	return out
}

func (b *StdinBuffer) GetBuffer() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer
}

func (b *StdinBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopTimerLocked()
	b.buffer = ""
	b.pasteMode = false
	b.pasteBuffer = ""
	b.clearDedupeLocked()
}

func (b *StdinBuffer) Destroy() { b.Clear() }

func (b *StdinBuffer) startTimerLocked() {
	if b.timeout <= 0 {
		return
	}
	b.timer = time.AfterFunc(b.timeout, func() {
		flushed := b.Flush()
		if len(flushed) == 0 {
			return
		}
		events := make([]stdinBufferEvent, 0, len(flushed))
		for _, sequence := range flushed {
			events = append(events, stdinBufferEvent{data: sequence})
		}
		b.deliverEvents(events)
	})
}

func (b *StdinBuffer) stopTimerLocked() {
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
}

func (b *StdinBuffer) emitDataLocked(data string, events *[]stdinBufferEvent) {
	*events = append(*events, stdinBufferEvent{data: data})
}

func (b *StdinBuffer) emitDataWithDedupeLocked(data string, events *[]stdinBufferEvent) {
	if b.hasDedupe {
		if !strings.HasPrefix(data, "\x1b") {
			r, size := utf8.DecodeRuneInString(data)
			if r == b.dedupeRune && size == len(data) {
				b.clearDedupeLocked()
				return
			}
		}
		b.clearDedupeLocked()
	}
	b.emitDataLocked(data, events)
	if strings.HasPrefix(data, "\x1b[") {
		if r, ok := DecodePrintableKey(data); ok {
			b.dedupeRune = r
			b.hasDedupe = true
		}
	}
}

func (b *StdinBuffer) clearDedupeLocked() {
	b.hasDedupe = false
	b.dedupeRune = 0
}

func (b *StdinBuffer) emitPasteLocked(data string, events *[]stdinBufferEvent) {
	*events = append(*events, stdinBufferEvent{paste: true, data: data})
}

func (b *StdinBuffer) deliverEvents(events []stdinBufferEvent) {
	for _, event := range events {
		b.mu.Lock()
		var handlers []func(string)
		if event.paste {
			handlers = append(handlers, b.onPaste...)
		} else {
			handlers = append(handlers, b.onData...)
		}
		b.mu.Unlock()
		for _, fn := range handlers {
			fn(event.data)
		}
	}
}

func nextInputSequence(data string) (string, string) {
	if data == "" {
		return "", ""
	}
	if data[0] != '\x1b' {
		r, size := []rune(data)[0], 0
		size = len(string(r))
		return data[:size], data[size:]
	}
	if len(data) >= 3 && data[1] == '[' {
		for i := 2; i < len(data); i++ {
			if data[i] >= 0x40 && data[i] <= 0x7e {
				return data[:i+1], data[i+1:]
			}
		}
	}
	return data[:1], data[1:]
}

func extractCompleteInputSequences(buffer string) ([]string, string) {
	var sequences []string
	for len(buffer) > 0 {
		if status, end := completeC1InputSequence(buffer); status != "not-c1" {
			switch status {
			case "complete":
				sequences = append(sequences, buffer[:end])
				buffer = buffer[end:]
				continue
			case "incomplete":
				return sequences, buffer
			}
		}
		if buffer[0] != '\x1b' {
			r, size := utf8.DecodeRuneInString(buffer)
			if r == utf8.RuneError && size == 0 {
				break
			}
			if size <= 0 {
				size = 1
			}
			sequences = append(sequences, buffer[:size])
			buffer = buffer[size:]
			continue
		}
		status, end := completeEscapeSequence(buffer)
		switch status {
		case "complete":
			if end == 2 && strings.HasPrefix(buffer, "\x1b\x1b") && len(buffer) > 2 && startsEscapeBody(buffer[2]) {
				sequences = append(sequences, "\x1b")
				buffer = buffer[1:]
				continue
			}
			sequences = append(sequences, buffer[:end])
			buffer = buffer[end:]
		case "incomplete":
			return sequences, buffer
		default:
			sequences = append(sequences, buffer[:1])
			buffer = buffer[1:]
		}
	}
	return sequences, ""
}

func completeC1InputSequence(data string) (string, int) {
	code, prefixLen, ok := c1Prefix(data)
	if !ok {
		return "not-c1", 0
	}
	switch code {
	case 0x8e, 0x8f:
		if len(data) >= prefixLen+1 {
			return "complete", prefixLen + 1
		}
		return "incomplete", 0
	case 0x90, 0x98, 0x9e, 0x9f:
		return completeC1StringTerminatedSequence(data, prefixLen, false)
	case 0x9b:
		for i := prefixLen; i < len(data); i++ {
			if data[i] >= 0x40 && data[i] <= 0x7e {
				payload := data[prefixLen : i+1]
				if strings.HasPrefix(payload, "<") {
					if isCompleteSGRMousePayload(payload) {
						return "complete", i + 1
					}
					return "incomplete", 0
				}
				return "complete", i + 1
			}
		}
		return "incomplete", 0
	case 0x9c:
		return "complete", prefixLen
	case 0x9d:
		return completeC1StringTerminatedSequence(data, prefixLen, true)
	case 0x84, 0x85, 0x88, 0x8d:
		return "complete", prefixLen
	default:
		return "complete", prefixLen
	}
}

func completeC1StringTerminatedSequence(data string, start int, allowBEL bool) (string, int) {
	if _, sequenceEnd, ok := controlStringTerminator(data, start, allowBEL); ok {
		return "complete", sequenceEnd
	}
	return "incomplete", 0
}

func completeEscapeSequence(data string) (string, int) {
	if data == "" || data[0] != '\x1b' {
		return "not-escape", 0
	}
	if len(data) == 1 {
		return "incomplete", 0
	}
	switch data[1] {
	case '[':
		if strings.HasPrefix(data, "\x1b[M") {
			if len(data) >= 6 {
				return "complete", 6
			}
			return "incomplete", 0
		}
		return completeCSISequence(data)
	case ']':
		return completeStringTerminatedSequence(data, "\x1b]", 2, true)
	case 'P', '_', '^', 'X':
		return completeStringTerminatedSequence(data, data[:2], 2, false)
	case 'O':
		if len(data) >= 3 {
			return "complete", 3
		}
		return "incomplete", 0
	default:
		return "complete", 2
	}
}

func completeCSISequence(data string) (string, int) {
	if len(data) < 3 {
		return "incomplete", 0
	}
	for i := 2; i < len(data); i++ {
		if data[i] >= 0x40 && data[i] <= 0x7e {
			payload := data[2 : i+1]
			if strings.HasPrefix(payload, "<") {
				if isCompleteSGRMousePayload(payload) {
					return "complete", i + 1
				}
				return "incomplete", 0
			}
			return "complete", i + 1
		}
	}
	return "incomplete", 0
}

func completeStringTerminatedSequence(data, prefix string, minLen int, allowBEL bool) (string, int) {
	if !strings.HasPrefix(data, prefix) || len(data) < minLen {
		return "complete", minLen
	}
	if idx := strings.Index(data, "\x1b\\"); idx >= 0 {
		return "complete", idx + len("\x1b\\")
	}
	if allowBEL {
		if idx := strings.IndexByte(data, '\x07'); idx >= 0 {
			return "complete", idx + 1
		}
	}
	return "incomplete", 0
}

func isCompleteSGRMousePayload(payload string) bool {
	if len(payload) < 5 || payload[0] != '<' {
		return false
	}
	final := payload[len(payload)-1]
	if final != 'M' && final != 'm' {
		return false
	}
	parts := strings.Split(payload[1:len(payload)-1], ";")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func startsEscapeBody(b byte) bool {
	switch b {
	case '[', ']', 'O', 'P', '_', '^', 'X':
		return true
	default:
		return false
	}
}
