package gitui

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestProcessTerminalNegotiatesKittyKeyboard(t *testing.T) {
	out := &safeBuffer{}
	terminal := NewProcessTerminalWithIO(strings.NewReader("\x1b[?1u"), out, 80, 24)

	var input []string
	terminal.Start(func(data string) { input = append(input, data) }, func() {})
	waitFor(t, func() bool { return terminal.KittyProtocolActive() })

	if len(input) != 0 {
		t.Fatalf("kitty response should be consumed, got %#v", input)
	}
	if output := out.String(); !strings.Contains(output, "\x1b[?2004h") || !strings.Contains(output, "\x1b[?u") || !strings.Contains(output, "\x1b[>7u") {
		t.Fatalf("startup output missing kitty negotiation sequences: %q", output)
	}

	terminal.Stop()
	if terminal.KittyProtocolActive() {
		t.Fatalf("kitty protocol should be inactive after stop")
	}
	if output := out.String(); !strings.Contains(output, "\x1b[<u") || !strings.Contains(output, "\x1b[?2004l") {
		t.Fatalf("stop output missing keyboard/bracketed-paste cleanup: %q", output)
	}
	output := out.String()
	pasteOff := strings.Index(output, "\x1b[?2004l")
	keyboardOff := strings.Index(output, "\x1b[<u")
	if pasteOff < 0 || keyboardOff < 0 || pasteOff > keyboardOff {
		t.Fatalf("ProcessTerminal.Stop cleanup order should match Pi bracketed-paste-before-keyboard, output=%q", output)
	}
	if strings.Contains(output, "\x1b[?25h") {
		t.Fatalf("ProcessTerminal.Stop should not show cursor directly; TUI.Stop owns cursor restoration, output=%q", output)
	}
}

func TestProcessTerminalOnlyConsumesInitialKittyKeyboardResponseLikePi(t *testing.T) {
	out := &safeBuffer{}
	terminal := NewProcessTerminalWithIO(strings.NewReader("\x1b[?1u\x1b[?1u"), out, 80, 24)

	var inputMu sync.Mutex
	var input []string
	terminal.Start(func(data string) {
		inputMu.Lock()
		defer inputMu.Unlock()
		input = append(input, data)
	}, func() {})
	waitFor(t, func() bool {
		inputMu.Lock()
		defer inputMu.Unlock()
		return len(input) == 1
	})
	terminal.Stop()

	inputMu.Lock()
	gotInput := append([]string(nil), input...)
	inputMu.Unlock()
	if !strings.Contains(out.String(), "\x1b[<u") {
		t.Fatalf("terminal should clean up negotiated Kitty protocol, output=%q", out.String())
	}
	if !equalLines(gotInput, []string{"\x1b[?1u"}) {
		t.Fatalf("only repeated Kitty response should reach input handler, got %#v", gotInput)
	}
	if count := strings.Count(out.String(), "\x1b[>7u"); count != 1 {
		t.Fatalf("Kitty enable sequence count = %d, want 1: %q", count, out.String())
	}
}

func TestProcessTerminalStartStopCanBeReused(t *testing.T) {
	out := &safeBuffer{}
	terminal := NewProcessTerminalWithIO(strings.NewReader(""), out, 80, 24)

	terminal.Start(func(string) {}, func() {})
	terminal.Stop()
	terminal.Start(func(string) {}, func() {})
	terminal.Stop()

	output := out.String()
	if got := strings.Count(output, "\x1b[?2004h"); got != 2 {
		t.Fatalf("bracketed paste enable count = %d, want 2: %q", got, output)
	}
	if got := strings.Count(output, "\x1b[?2004l"); got != 2 {
		t.Fatalf("bracketed paste cleanup count = %d, want 2: %q", got, output)
	}
	if strings.Contains(output, "\x1b[>4;2m") {
		t.Fatalf("stopped reused terminal should cancel modifyOtherKeys fallback timer: %q", output)
	}
}

func TestProcessTerminalDimensionsFromEnvironment(t *testing.T) {
	t.Setenv("COLUMNS", "123")
	t.Setenv("LINES", "45")
	terminal := NewProcessTerminal()
	if terminal.Columns() != 123 || terminal.Rows() != 45 {
		t.Fatalf("size = %dx%d, want 123x45", terminal.Columns(), terminal.Rows())
	}

	t.Setenv("COLUMNS", "132")
	t.Setenv("LINES", "54")
	if terminal.Columns() != 132 || terminal.Rows() != 54 {
		t.Fatalf("dynamic size = %dx%d, want 132x54", terminal.Columns(), terminal.Rows())
	}

	terminal.SetSize(80, 24)
	t.Setenv("COLUMNS", "140")
	t.Setenv("LINES", "60")
	if terminal.Columns() != 80 || terminal.Rows() != 24 {
		t.Fatalf("explicit SetSize should pin dimensions, got %dx%d", terminal.Columns(), terminal.Rows())
	}
}

func TestProcessTerminalNativeTermFallsBackForNonTTYFiles(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	restore, err := enableProcessRawMode(file)
	if err != nil {
		t.Fatalf("non-TTY raw mode should be a no-op, got error: %v", err)
	}
	if restore != nil {
		t.Fatalf("non-TTY raw mode should not return a restore callback")
	}
	if cols, rows, ok := processTerminalSize(file); ok || cols != 0 || rows != 0 {
		t.Fatalf("non-TTY size = %dx%d ok=%v, want 0x0 false", cols, rows, ok)
	}
}

func TestProcessTerminalConcurrentSizeAccessIsSafe(t *testing.T) {
	terminal := NewProcessTerminalWithIO(strings.NewReader(""), &safeBuffer{}, 80, 24)

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				terminal.SetSize(40+worker+i%10, 10+worker+i%5)
				if terminal.Columns() <= 0 || terminal.Rows() <= 0 {
					t.Errorf("terminal dimensions should remain positive, got %dx%d", terminal.Columns(), terminal.Rows())
				}
			}
		}()
	}
	wg.Wait()
}

func TestProcessTerminalClearLineMatchesPi(t *testing.T) {
	out := &safeBuffer{}
	terminal := NewProcessTerminalWithIO(strings.NewReader(""), out, 80, 24)

	if err := terminal.ClearLine(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "\x1b[K" {
		t.Fatalf("ClearLine output = %q, want Pi erase-to-end sequence", got)
	}
}

func TestProcessTerminalWriteLogPath(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "writes.log")
	t.Setenv("PI_TUI_WRITE_LOG", logPath)
	out := &safeBuffer{}
	terminal := NewProcessTerminalWithIO(strings.NewReader(""), out, 80, 24)

	if err := terminal.Write("abc"); err != nil {
		t.Fatal(err)
	}
	if err := terminal.Write("def"); err != nil {
		t.Fatal(err)
	}

	if got := out.String(); got != "abcdef" {
		t.Fatalf("terminal output = %q, want abcdef", got)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "abcdef" {
		t.Fatalf("write log = %q, want abcdef", string(data))
	}

	logDir := t.TempDir()
	t.Setenv("PI_TUI_WRITE_LOG", logDir)
	terminal = NewProcessTerminalWithIO(strings.NewReader(""), &safeBuffer{}, 80, 24)
	if err := terminal.Write("xyz"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "tui-") || !strings.HasSuffix(entries[0].Name(), ".log") {
		t.Fatalf("directory log entries = %#v, want one tui-*.log", entries)
	}
	data, err = os.ReadFile(filepath.Join(logDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "xyz" {
		t.Fatalf("directory write log = %q, want xyz", string(data))
	}
}

func TestProcessTerminalFallsBackToModifyOtherKeys(t *testing.T) {
	out := &safeBuffer{}
	terminal := NewProcessTerminalWithIO(strings.NewReader(""), out, 80, 24)

	terminal.Start(func(string) {}, func() {})
	waitFor(t, func() bool { return strings.Contains(out.String(), "\x1b[>4;2m") })
	terminal.Stop()

	if output := out.String(); !strings.Contains(output, "\x1b[>4;2m") || !strings.Contains(output, "\x1b[>4;0m") {
		t.Fatalf("modifyOtherKeys fallback/cleanup missing: %q", output)
	}
}

func TestProcessTerminalDrainInputSuppressesLateInput(t *testing.T) {
	reader, writer := io.Pipe()
	out := &safeBuffer{}
	terminal := NewProcessTerminalWithIO(reader, out, 80, 24)
	defer func() {
		_ = writer.Close()
		_ = reader.Close()
		terminal.Stop()
	}()

	var mu sync.Mutex
	var input []string
	terminal.Start(func(data string) {
		mu.Lock()
		defer mu.Unlock()
		input = append(input, data)
	}, func() {})

	done := make(chan error, 1)
	go func() {
		done <- terminal.DrainInput(120*time.Millisecond, 25*time.Millisecond)
	}()
	waitFor(t, func() bool {
		terminal.mu.Lock()
		defer terminal.mu.Unlock()
		return terminal.inputHandler == nil
	})

	if _, err := io.WriteString(writer, "x"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatalf("DrainInput did not finish")
	}

	mu.Lock()
	gotDuringDrain := append([]string(nil), input...)
	mu.Unlock()
	if len(gotDuringDrain) != 0 {
		t.Fatalf("DrainInput should suppress input while draining, got %#v", gotDuringDrain)
	}

	if _, err := io.WriteString(writer, "y"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(input) == 1 && input[0] == "y"
	})
}

func TestProcessTerminalProgressSequencesAndCleanup(t *testing.T) {
	out := &safeBuffer{}
	terminal := NewProcessTerminalWithIO(strings.NewReader(""), out, 80, 24)

	if err := terminal.SetProgress(true); err != nil {
		t.Fatal(err)
	}
	if err := terminal.SetProgress(true); err != nil {
		t.Fatal(err)
	}
	if err := terminal.SetProgress(false); err != nil {
		t.Fatal(err)
	}

	output := out.String()
	if strings.Count(output, terminalProgressActive) != 2 {
		t.Fatalf("active progress writes = %q, want two direct active writes", output)
	}
	if !strings.Contains(output, terminalProgressClear) {
		t.Fatalf("clear progress sequence missing: %q", output)
	}
}

func TestProcessTerminalStopOnlyClearsActiveProgress(t *testing.T) {
	out := &safeBuffer{}
	terminal := NewProcessTerminalWithIO(strings.NewReader(""), out, 80, 24)
	terminal.Stop()
	if output := out.String(); strings.Contains(output, terminalProgressClear) {
		t.Fatalf("stop without active progress should not write progress clear: %q", output)
	}

	out = &safeBuffer{}
	terminal = NewProcessTerminalWithIO(strings.NewReader(""), out, 80, 24)
	if err := terminal.SetProgress(true); err != nil {
		t.Fatal(err)
	}
	terminal.Stop()
	if output := out.String(); !strings.Contains(output, terminalProgressClear) {
		t.Fatalf("stop with active progress should write progress clear: %q", output)
	}
}

func TestVirtualTerminalCursorVisibilityWritesPiSequences(t *testing.T) {
	terminal := NewVirtualTerminal(80, 24)
	if err := terminal.HideCursor(); err != nil {
		t.Fatal(err)
	}
	if err := terminal.ShowCursor(); err != nil {
		t.Fatal(err)
	}
	output := terminal.Output()
	if !strings.Contains(output, "\x1b[?25l") || !strings.Contains(output, "\x1b[?25h") {
		t.Fatalf("cursor visibility output = %q", output)
	}
}

func TestVirtualTerminalTracksWindowTitleOSC(t *testing.T) {
	terminal := NewVirtualTerminal(80, 24)
	if err := terminal.SetTitle("initial"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.WindowTitle(); got != "initial" {
		t.Fatalf("SetTitle window title = %q, want initial", got)
	}
	if err := terminal.Write("a\x1b]2;from-st\x1b\\b\x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\c"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.WindowTitle(); got != "from-st" {
		t.Fatalf("OSC 2 title = %q, want from-st", got)
	}
	if got := terminal.GetViewport()[0]; got != "ablinkc" {
		t.Fatalf("OSC title/hyperlink should not render visible text, got %q", got)
	}
	terminal.Reset()
	if got := terminal.WindowTitle(); got != "" {
		t.Fatalf("Reset title = %q, want empty", got)
	}
}

func TestStdinBufferPassesThroughRegularAndCompleteSequences(t *testing.T) {
	buffer := NewStdinBuffer(StdinBufferOptions{Timeout: time.Hour})
	var data []string
	buffer.OnData(func(s string) { data = append(data, s) })

	buffer.Process("abc")
	if !equalLines(data, []string{"a", "b", "c"}) {
		t.Fatalf("regular characters = %#v, want split characters", data)
	}

	data = nil
	buffer.Process("hello 世界")
	if !equalLines(data, []string{"h", "e", "l", "l", "o", " ", "世", "界"}) {
		t.Fatalf("unicode characters = %#v, want rune-sized events", data)
	}

	for _, seq := range []string{"\x1b[<35;20;5m", "\x1b[A", "\x1b[11~", "\x1ba", "\x1bOA", "\x1b[" + strings.Repeat("1;", 50) + "H"} {
		data = nil
		buffer.Process(seq)
		if !equalLines(data, []string{seq}) {
			t.Fatalf("complete sequence %q emitted %#v", seq, data)
		}
	}

	for _, seq := range []string{"\x9bA", "\x9b<35;20;5m", "\x8fA", "\xc2\x9bA"} {
		data = nil
		buffer.Process(seq)
		if !equalLines(data, []string{seq}) {
			t.Fatalf("complete C1 sequence %q emitted %#v", seq, data)
		}
	}
}

func TestStdinBufferBuffersPartialEscapeSequences(t *testing.T) {
	buffer := NewStdinBuffer(StdinBufferOptions{Timeout: 50 * time.Millisecond})
	var data []string
	buffer.OnData(func(s string) { data = append(data, s) })

	buffer.Process("\x1b[")
	if len(data) != 0 {
		t.Fatalf("partial CSI should be buffered, got %#v", data)
	}
	buffer.Process("A")
	if len(data) != 1 || data[0] != "\x1b[A" {
		t.Fatalf("data = %#v, want complete up-arrow sequence", data)
	}
}

func TestStdinBufferPiMixedKittyMouseAndPasteMatrix(t *testing.T) {
	t.Run("mixed plain and escape content", func(t *testing.T) {
		buffer := NewStdinBuffer(StdinBufferOptions{Timeout: time.Hour})
		var data []string
		buffer.OnData(func(s string) { data = append(data, s) })

		buffer.Process("abc\x1b[A")
		if !equalLines(data, []string{"a", "b", "c", "\x1b[A"}) {
			t.Fatalf("plain before escape = %#v", data)
		}

		data = nil
		buffer.Process("\x1b[Aabc")
		if !equalLines(data, []string{"\x1b[A", "a", "b", "c"}) {
			t.Fatalf("escape before plain = %#v", data)
		}

		data = nil
		buffer.Process("\x1b[A\x1b[B\x1b[C")
		if !equalLines(data, []string{"\x1b[A", "\x1b[B", "\x1b[C"}) {
			t.Fatalf("multiple escape sequences = %#v", data)
		}

		data = nil
		buffer.Process("\x1b\x1b[27;129:3u")
		if !equalLines(data, []string{"\x1b", "\x1b[27;129:3u"}) {
			t.Fatalf("ESC+ESC+CSI should split into Escape key and Kitty release: %#v", data)
		}

		data = nil
		buffer.Process("\x1b\x1b[27;1:3u")
		if !equalLines(data, []string{"\x1b", "\x1b[27;1:3u"}) {
			t.Fatalf("ESC+ESC+CSI without modifier should split into Escape key and Kitty release: %#v", data)
		}

		data = nil
		buffer.Process("\x1b\x1b")
		if !equalLines(data, []string{"\x1b\x1b"}) {
			t.Fatalf("bare ESC+ESC should remain one sequence: %#v", data)
		}

		data = nil
		buffer.Process("abc\x1b[<35")
		if !equalLines(data, []string{"a", "b", "c"}) || buffer.GetBuffer() != "\x1b[<35" {
			t.Fatalf("partial mouse with prefix data=%#v buffer=%q", data, buffer.GetBuffer())
		}
		buffer.Process(";20;5m")
		if !equalLines(data, []string{"a", "b", "c", "\x1b[<35;20;5m"}) {
			t.Fatalf("completed partial mouse = %#v", data)
		}
	})

	t.Run("batched kitty and split mouse events", func(t *testing.T) {
		buffer := NewStdinBuffer(StdinBufferOptions{Timeout: time.Hour})
		var data []string
		buffer.OnData(func(s string) { data = append(data, s) })

		buffer.Process("\x1b[97u\x1b[97;1:3u\x1b[98u\x1b[98;1:3u")
		if !equalLines(data, []string{"\x1b[97u", "\x1b[97;1:3u", "\x1b[98u", "\x1b[98;1:3u"}) {
			t.Fatalf("batched kitty events = %#v", data)
		}

		data = nil
		buffer.Process("\x1b[104u\x1b[104;1:3u\x1b[105u\x1b[105;1:3u")
		if !equalLines(data, []string{"\x1b[104u", "\x1b[104;1:3u", "\x1b[105u", "\x1b[105;1:3u"}) {
			t.Fatalf("rapid kitty typing = %#v", data)
		}

		data = nil
		buffer.Process("\x1b[1;1:1A")
		if !equalLines(data, []string{"\x1b[1;1:1A"}) {
			t.Fatalf("Kitty arrow event type = %#v", data)
		}

		data = nil
		buffer.Process("\x1b[3;1:3~")
		if !equalLines(data, []string{"\x1b[3;1:3~"}) {
			t.Fatalf("Kitty functional release = %#v", data)
		}

		data = nil
		buffer.Process("\x1b[<3")
		buffer.Process("5;1")
		buffer.Process("5;")
		buffer.Process("10m")
		if !equalLines(data, []string{"\x1b[<35;15;10m"}) {
			t.Fatalf("split mouse event = %#v", data)
		}

		data = nil
		buffer.Process("\x1b[M")
		if got := buffer.GetBuffer(); got != "\x1b[M" {
			t.Fatalf("old mouse partial buffer = %q", got)
		}
		buffer.Process(" a")
		if got := buffer.GetBuffer(); got != "\x1b[M a" {
			t.Fatalf("old mouse partial buffer after two bytes = %q", got)
		}
		buffer.Process("b")
		if !equalLines(data, []string{"\x1b[M ab"}) {
			t.Fatalf("old mouse completed = %#v", data)
		}
	})

	t.Run("chunked paste with surrounding input and unicode", func(t *testing.T) {
		buffer := NewStdinBuffer(StdinBufferOptions{Timeout: time.Hour})
		var data []string
		var paste []string
		buffer.OnData(func(s string) { data = append(data, s) })
		buffer.OnPaste(func(s string) { paste = append(paste, s) })

		buffer.Process("a")
		buffer.Process(bracketedPasteStart)
		buffer.Process("hello ")
		buffer.Process("world" + bracketedPasteEnd)
		buffer.Process("b")
		if !equalLines(data, []string{"a", "b"}) {
			t.Fatalf("data around chunked paste = %#v", data)
		}
		if !equalLines(paste, []string{"hello world"}) {
			t.Fatalf("chunked paste = %#v", paste)
		}

		data = nil
		paste = nil
		buffer.Process(bracketedPasteStart + "line1\nline2\nline3" + bracketedPasteEnd)
		buffer.Process(bracketedPasteStart + "Hello 世界 🎉" + bracketedPasteEnd)
		if len(data) != 0 {
			t.Fatalf("paste should not emit data events, got %#v", data)
		}
		if !equalLines(paste, []string{"line1\nline2\nline3", "Hello 世界 🎉"}) {
			t.Fatalf("unicode/newline paste = %#v", paste)
		}
	})
}

func TestStdinBufferPiMouseAndKittyDedup(t *testing.T) {
	buffer := NewStdinBuffer(StdinBufferOptions{Timeout: time.Hour})
	var data []string
	buffer.OnData(func(s string) { data = append(data, s) })

	buffer.Process("")
	if len(data) != 1 || data[0] != "" {
		t.Fatalf("empty input data = %#v, want empty event", data)
	}

	data = nil
	buffer.Process("\x1b[M abc")
	if strings.Join(data, "|") != "\x1b[M ab|c" {
		t.Fatalf("old mouse sequence split = %#v", data)
	}

	data = nil
	buffer.Process("\x1b[224uà")
	if len(data) != 1 || data[0] != "\x1b[224u" {
		t.Fatalf("kitty duplicate raw char should be dropped: %#v", data)
	}

	data = nil
	buffer.Process("\x1b[97ub")
	if strings.Join(data, "|") != "\x1b[97u|b" {
		t.Fatalf("non-matching raw char should be kept: %#v", data)
	}

	data = nil
	buffer.Process("\x1b[64;3u@")
	if strings.Join(data, "|") != "\x1b[64;3u|@" {
		t.Fatalf("modified kitty raw char should be kept: %#v", data)
	}
}

func TestStdinBufferProcessBytesHighBitMetaCompatibility(t *testing.T) {
	buffer := NewStdinBuffer(StdinBufferOptions{Timeout: time.Hour})
	var data []string
	buffer.OnData(func(s string) { data = append(data, s) })

	buffer.ProcessBytes([]byte{0xe1})
	if len(data) != 1 || data[0] != "\x1ba" {
		t.Fatalf("single high-bit byte should convert to ESC+a, got %#v", data)
	}

	data = nil
	buffer.ProcessBytes([]byte("\x1b[A"))
	if len(data) != 1 || data[0] != "\x1b[A" {
		t.Fatalf("multi-byte input should process normally, got %#v", data)
	}
}

func TestStdinBufferPiSequenceCompletenessEdgeCases(t *testing.T) {
	buffer := NewStdinBuffer(StdinBufferOptions{Timeout: time.Hour})
	var data []string
	buffer.OnData(func(s string) { data = append(data, s) })

	buffer.Process("\x1b[")
	buffer.Process("")
	if len(data) != 0 {
		t.Fatalf("empty input while an escape is buffered should not emit an empty data event: %#v", data)
	}
	if got := buffer.GetBuffer(); got != "\x1b[" {
		t.Fatalf("buffer after empty input = %q, want pending CSI", got)
	}
	buffer.Clear()

	buffer.Process("\x1b[<1A")
	if len(data) != 0 {
		t.Fatalf("malformed SGR mouse prefix should remain buffered until timeout/flush: %#v", data)
	}
	if got := buffer.GetBuffer(); got != "\x1b[<1A" {
		t.Fatalf("buffered malformed SGR mouse sequence = %q", got)
	}
	if flushed := buffer.Flush(); len(flushed) != 1 || flushed[0] != "\x1b[<1A" {
		t.Fatalf("flushed malformed SGR mouse sequence = %#v", flushed)
	}

	buffer.Process("\x1b]0;title\x07")
	if len(data) != 1 || data[0] != "\x1b]0;title\x07" {
		t.Fatalf("OSC BEL sequence should complete immediately, got %#v", data)
	}
	data = nil

	buffer.Process("\x1bPpayload\x07")
	if len(data) != 0 {
		t.Fatalf("DCS BEL should not terminate the sequence like OSC: %#v", data)
	}
	buffer.Process("\x1b\\")
	if len(data) != 1 || data[0] != "\x1bPpayload\x07\x1b\\" {
		t.Fatalf("DCS should complete only at ST, got %#v", data)
	}
	data = nil

	buffer.Process("\x1b_Gi=1\x07")
	if len(data) != 0 {
		t.Fatalf("APC BEL should not terminate the sequence like OSC: %#v", data)
	}
	buffer.Process("\x1b\\")
	if len(data) != 1 || data[0] != "\x1b_Gi=1\x07\x1b\\" {
		t.Fatalf("APC should complete only at ST, got %#v", data)
	}
	data = nil

	buffer.Process("\x1b^private\x07")
	if len(data) != 0 {
		t.Fatalf("PM BEL should not terminate the sequence like OSC: %#v", data)
	}
	buffer.Process("\x1b\\")
	if len(data) != 1 || data[0] != "\x1b^private\x07\x1b\\" {
		t.Fatalf("PM should complete only at ST, got %#v", data)
	}
	data = nil

	buffer.Process("\x1bXsos\x07")
	if len(data) != 0 {
		t.Fatalf("SOS BEL should not terminate the sequence like OSC: %#v", data)
	}
	buffer.Process("\x1b\\")
	if len(data) != 1 || data[0] != "\x1bXsos\x07\x1b\\" {
		t.Fatalf("SOS should complete only at ST, got %#v", data)
	}
	data = nil

	buffer.Process("\x9d0;c1-title\x07")
	if len(data) != 1 || data[0] != "\x9d0;c1-title\x07" {
		t.Fatalf("8-bit OSC BEL sequence should complete immediately, got %#v", data)
	}
	data = nil

	buffer.Process("\x90payload\x07")
	if len(data) != 0 {
		t.Fatalf("8-bit DCS BEL should not terminate the sequence like OSC: %#v", data)
	}
	buffer.Process("\x9c")
	if len(data) != 1 || data[0] != "\x90payload\x07\x9c" {
		t.Fatalf("8-bit DCS should complete only at ST, got %#v", data)
	}
	data = nil

	buffer.Process("\x9eprivate\x07")
	if len(data) != 0 {
		t.Fatalf("8-bit PM BEL should not terminate the sequence like OSC: %#v", data)
	}
	buffer.Process("\x1b\\")
	if len(data) != 1 || data[0] != "\x9eprivate\x07\x1b\\" {
		t.Fatalf("8-bit PM should complete at ESC ST, got %#v", data)
	}
	data = nil

	buffer.Process("\u009d0;utf-title\u009c")
	if len(data) != 1 || data[0] != "\u009d0;utf-title\u009c" {
		t.Fatalf("UTF-8 C1 OSC should complete at UTF-8 C1 ST, got %#v", data)
	}
}

func TestStdinBufferTimeoutFlushesIncompleteSequence(t *testing.T) {
	buffer := NewStdinBuffer(StdinBufferOptions{Timeout: 5 * time.Millisecond})
	var mu sync.Mutex
	var data []string
	buffer.OnData(func(s string) {
		mu.Lock()
		defer mu.Unlock()
		data = append(data, s)
	})

	buffer.Process("\x1b[")
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(data) == 1
	})
	mu.Lock()
	gotData := append([]string(nil), data...)
	mu.Unlock()
	if gotData[0] != "\x1b[" {
		t.Fatalf("timeout data = %#v, want pending CSI", gotData)
	}
	if got := buffer.GetBuffer(); got != "" {
		t.Fatalf("buffer after timeout = %q, want empty", got)
	}
}

func TestStdinBufferClearAndDestroyDiscardPendingInput(t *testing.T) {
	buffer := NewStdinBuffer(StdinBufferOptions{Timeout: 20 * time.Millisecond})
	var data []string
	buffer.OnData(func(s string) { data = append(data, s) })

	buffer.Process("\x1b[")
	buffer.Clear()
	time.Sleep(30 * time.Millisecond)
	if len(data) != 0 {
		t.Fatalf("clear should discard pending input without emitting, got %#v", data)
	}

	buffer.Process("\x1b]")
	buffer.Destroy()
	time.Sleep(30 * time.Millisecond)
	if len(data) != 0 {
		t.Fatalf("destroy should discard pending input without emitting, got %#v", data)
	}
	if got := buffer.GetBuffer(); got != "" {
		t.Fatalf("buffer after destroy = %q, want empty", got)
	}
}

func TestStdinBufferExplicitFlushPiEdgeCases(t *testing.T) {
	buffer := NewStdinBuffer(StdinBufferOptions{Timeout: time.Hour})
	var data []string
	buffer.OnData(func(s string) { data = append(data, s) })

	if flushed := buffer.Flush(); len(flushed) != 0 {
		t.Fatalf("empty flush = %#v, want nil/empty", flushed)
	}
	buffer.Process("\x1b")
	if len(data) != 0 {
		t.Fatalf("lone escape should buffer before flush, got %#v", data)
	}
	if flushed := buffer.Flush(); !equalLines(flushed, []string{"\x1b"}) {
		t.Fatalf("flush lone escape = %#v", flushed)
	}
	if got := buffer.GetBuffer(); got != "" {
		t.Fatalf("buffer after flush = %q, want empty", got)
	}

	buffer.Process("\x1b[")
	if flushed := buffer.Flush(); !equalLines(flushed, []string{"\x1b["}) {
		t.Fatalf("flush incomplete CSI = %#v", flushed)
	}
}

func TestStdinBufferPasteClearsKittyPrintableDedupe(t *testing.T) {
	buffer := NewStdinBuffer(StdinBufferOptions{Timeout: time.Hour})
	var data []string
	var paste []string
	buffer.OnData(func(s string) { data = append(data, s) })
	buffer.OnPaste(func(s string) { paste = append(paste, s) })

	buffer.Process("\x1b[64u" + bracketedPasteStart + "payload" + bracketedPasteEnd + "@")

	if !equalLines(data, []string{"\x1b[64u", "@"}) {
		t.Fatalf("data events = %#v, want Kitty printable plus raw @ after paste", data)
	}
	if !equalLines(paste, []string{"payload"}) {
		t.Fatalf("paste events = %#v, want payload", paste)
	}

	data = nil
	paste = nil
	buffer.Process("\x1b[65u")
	buffer.Process(bracketedPasteStart)
	buffer.Process("payload")
	buffer.Process(bracketedPasteEnd + "A")

	if !equalLines(data, []string{"\x1b[65u", "A"}) {
		t.Fatalf("chunked data events = %#v, want Kitty printable plus raw A after paste", data)
	}
	if !equalLines(paste, []string{"payload"}) {
		t.Fatalf("chunked paste events = %#v, want payload", paste)
	}
}

func TestStdinBufferKittyPrintableDedupeResetsAfterOtherEvents(t *testing.T) {
	buffer := NewStdinBuffer(StdinBufferOptions{Timeout: time.Hour})
	var data []string
	buffer.OnData(func(s string) { data = append(data, s) })

	buffer.Process("\x1b[64u\x1b[A@")
	if !equalLines(data, []string{"\x1b[64u", "\x1b[A", "@"}) {
		t.Fatalf("escape event should clear stale Kitty printable dedupe, data=%#v", data)
	}

	data = nil
	buffer.Process("\x1b[65u")
	buffer.Process("\x1b[")
	if flushed := buffer.Flush(); len(flushed) != 1 || flushed[0] != "\x1b[" {
		t.Fatalf("flush = %#v, want pending CSI", flushed)
	}
	buffer.Process("A")
	if !equalLines(data, []string{"\x1b[65u", "A"}) {
		t.Fatalf("flush should clear stale Kitty printable dedupe, data=%#v", data)
	}
}

func TestStdinBufferCallbacksRunOutsideInternalLock(t *testing.T) {
	buffer := NewStdinBuffer(StdinBufferOptions{Timeout: time.Hour})
	done := make(chan string, 1)
	buffer.OnData(func(sequence string) {
		if sequence != "x" {
			done <- "unexpected data sequence: " + sequence
			return
		}
		if got := buffer.GetBuffer(); got != "" {
			done <- "buffer should be readable and empty inside data callback, got " + got
			return
		}
		if flushed := buffer.Flush(); len(flushed) != 0 {
			done <- "flush inside data callback should be empty"
			return
		}
		done <- ""
	})
	go buffer.Process("x")
	select {
	case err := <-done:
		if err != "" {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatalf("data callback appears to be blocked by StdinBuffer internal lock")
	}

	pasteBuffer := NewStdinBuffer(StdinBufferOptions{Timeout: time.Hour})
	pasteDone := make(chan string, 1)
	pasteBuffer.OnPaste(func(content string) {
		if content != "payload" {
			pasteDone <- "unexpected paste content: " + content
			return
		}
		if got := pasteBuffer.GetBuffer(); got != "" {
			pasteDone <- "buffer should be readable and empty inside paste callback, got " + got
			return
		}
		pasteDone <- ""
	})
	go pasteBuffer.Process(bracketedPasteStart + "payload" + bracketedPasteEnd)
	select {
	case err := <-pasteDone:
		if err != "" {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatalf("paste callback appears to be blocked by StdinBuffer internal lock")
	}
}

func waitFor(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met before timeout")
}
