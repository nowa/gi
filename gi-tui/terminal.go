package gitui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"
)

const (
	terminalProgressActive = "\x1b]9;4;3\x07"
	terminalProgressClear  = "\x1b]9;4;0;\x07"
)

var kittyKeyboardResponsePattern = regexp.MustCompile(`^\x1b\[\?([0-9]+)u$`)

// Terminal is the output/input boundary used by TUI. Implementations can be a
// real process terminal, an xterm-headless test adapter, or an in-memory fake.
type Terminal interface {
	Start(onInput func(string), onResize func())
	Stop()
	DrainInput(max, idle time.Duration) error
	Write(data string) error
	Columns() int
	Rows() int
	KittyProtocolActive() bool
	MoveBy(lines int) error
	HideCursor() error
	ShowCursor() error
	ClearLine() error
	ClearFromCursor() error
	ClearScreen() error
	SetTitle(title string) error
	SetProgress(active bool) error
}

// ProcessTerminal owns raw terminal setup when stdin is an *os.File, routes
// input through StdinBuffer, and negotiates Kitty keyboard / modifyOtherKeys
// sequences the same way pi-tui does at the process boundary.
type ProcessTerminal struct {
	in                    io.Reader
	out                   io.Writer
	cols                  int
	rows                  int
	dynamicSize           bool
	stopCh                chan struct{}
	once                  sync.Once
	mu                    sync.Mutex
	stdinBuffer           *StdinBuffer
	rawRestore            func() error
	keyboardFallbackTimer *time.Timer
	progressStop          chan struct{}
	resizeStop            func()
	inputHandler          func(string)
	lastInputAt           time.Time
	writeLogPath          string
	kittyProtocolActive   bool
	modifyOtherKeysActive bool
}

func NewProcessTerminal() *ProcessTerminal {
	return &ProcessTerminal{
		in:           os.Stdin,
		out:          os.Stdout,
		cols:         envInt("COLUMNS", 80),
		rows:         envInt("LINES", 24),
		dynamicSize:  true,
		writeLogPath: terminalWriteLogPath(),
	}
}

func NewProcessTerminalWithIO(in io.Reader, out io.Writer, cols, rows int) *ProcessTerminal {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	return &ProcessTerminal{in: in, out: out, cols: cols, rows: rows, writeLogPath: terminalWriteLogPath()}
}

func envInt(name string, fallback int) int {
	if value, err := strconv.Atoi(os.Getenv(name)); err == nil && value > 0 {
		return value
	}
	return fallback
}

func terminalWriteLogPath() string {
	raw := os.Getenv("PI_TUI_WRITE_LOG")
	if raw == "" {
		return ""
	}
	if info, err := os.Stat(raw); err == nil && info.IsDir() {
		name := fmt.Sprintf("tui-%s-%d.log", time.Now().Format("2006-01-02_15-04-05"), os.Getpid())
		return filepath.Join(raw, name)
	}
	return raw
}

func (t *ProcessTerminal) Start(onInput func(string), onResize func()) {
	t.mu.Lock()
	t.once = sync.Once{}
	t.stopCh = make(chan struct{})
	stopCh := t.stopCh
	t.inputHandler = onInput
	t.resizeStop = startProcessResizeWatcher(onResize)
	t.stdinBuffer = NewStdinBuffer(StdinBufferOptions{Timeout: 10 * time.Millisecond})
	buffer := t.stdinBuffer
	if file, ok := t.in.(*os.File); ok {
		if restore, err := enableProcessRawMode(file); err == nil {
			t.rawRestore = restore
		}
	}
	t.mu.Unlock()

	buffer.OnData(func(sequence string) {
		if t.handleKeyboardProtocolResponse(sequence) {
			return
		}
		if handler := t.recordInputAndHandler(); handler != nil {
			handler(sequence)
		}
	})
	buffer.OnPaste(func(content string) {
		if handler := t.recordInputAndHandler(); handler != nil {
			handler(bracketedPasteStart + content + bracketedPasteEnd)
		}
	})

	_ = t.Write("\x1b[?2004h")
	_ = t.Write("\x1b[?u")
	t.scheduleModifyOtherKeysFallback()
	if onResize != nil {
		onResize()
	}
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-stopCh:
				return
			default:
			}
			n, err := t.in.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				buffer.ProcessBytes(buf[:n])
			}
		}
	}()
}

func (t *ProcessTerminal) Stop() {
	t.once.Do(func() {
		t.mu.Lock()
		if t.stopCh != nil {
			close(t.stopCh)
		}
		resizeStop := t.resizeStop
		t.resizeStop = nil
		t.stopKeyboardFallbackLocked()
		t.mu.Unlock()
		if resizeStop != nil {
			resizeStop()
		}
		if t.clearProgressInterval() {
			_ = t.Write(terminalProgressClear)
		}
		_ = t.Write("\x1b[?2004l")
		_ = t.disableKeyboardProtocols()
		t.mu.Lock()
		if t.stdinBuffer != nil {
			t.stdinBuffer.Destroy()
			t.stdinBuffer = nil
		}
		t.inputHandler = nil
		restore := t.rawRestore
		t.rawRestore = nil
		t.mu.Unlock()
		if restore != nil {
			_ = restore()
		}
	})
}

func (t *ProcessTerminal) DrainInput(maxDuration, idle time.Duration) error {
	_ = t.disableKeyboardProtocols()
	if maxDuration <= 0 {
		maxDuration = time.Second
	}
	if idle <= 0 {
		idle = 50 * time.Millisecond
	}
	t.mu.Lock()
	previousHandler := t.inputHandler
	t.inputHandler = nil
	t.lastInputAt = time.Now()
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		t.inputHandler = previousHandler
		t.mu.Unlock()
	}()

	deadline := time.Now().Add(maxDuration)
	for {
		now := time.Now()
		if !now.Before(deadline) {
			return nil
		}
		t.mu.Lock()
		lastInputAt := t.lastInputAt
		t.mu.Unlock()
		if now.Sub(lastInputAt) >= idle {
			return nil
		}
		sleep := min(idle-now.Sub(lastInputAt), time.Until(deadline))
		if sleep <= 0 {
			return nil
		}
		time.Sleep(sleep)
	}
}

func (t *ProcessTerminal) Write(data string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, err := io.WriteString(t.out, data)
	if t.writeLogPath != "" {
		if file, logErr := os.OpenFile(t.writeLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); logErr == nil {
			_, _ = io.WriteString(file, data)
			_ = file.Close()
		}
	}
	return err
}

func (t *ProcessTerminal) Columns() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dynamicSize {
		if file, ok := t.out.(*os.File); ok {
			if cols, _, ok := processTerminalSize(file); ok {
				return cols
			}
		}
		return envInt("COLUMNS", 80)
	}
	return t.cols
}

func (t *ProcessTerminal) Rows() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dynamicSize {
		if file, ok := t.out.(*os.File); ok {
			if _, rows, ok := processTerminalSize(file); ok {
				return rows
			}
		}
		return envInt("LINES", 24)
	}
	return t.rows
}

func (t *ProcessTerminal) SetSize(cols, rows int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.dynamicSize = false
	t.cols = cols
	t.rows = rows
}

func (t *ProcessTerminal) KittyProtocolActive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.kittyProtocolActive
}

func (t *ProcessTerminal) recordInputAndHandler() func(string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastInputAt = time.Now()
	return t.inputHandler
}

func (t *ProcessTerminal) handleKeyboardProtocolResponse(sequence string) bool {
	if !kittyKeyboardResponsePattern.MatchString(sequence) {
		return false
	}
	t.mu.Lock()
	if !t.kittyProtocolActive {
		t.kittyProtocolActive = true
		SetKittyProtocolActive(true)
		t.stopKeyboardFallbackLocked()
		t.mu.Unlock()
		_ = t.Write("\x1b[>7u")
		return true
	}
	t.mu.Unlock()
	return false
}

func (t *ProcessTerminal) scheduleModifyOtherKeysFallback() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopKeyboardFallbackLocked()
	t.keyboardFallbackTimer = time.AfterFunc(150*time.Millisecond, func() {
		t.mu.Lock()
		if t.kittyProtocolActive || t.modifyOtherKeysActive {
			t.mu.Unlock()
			return
		}
		t.modifyOtherKeysActive = true
		t.mu.Unlock()
		_ = t.Write("\x1b[>4;2m")
	})
}

func (t *ProcessTerminal) stopKeyboardFallbackLocked() {
	if t.keyboardFallbackTimer != nil {
		t.keyboardFallbackTimer.Stop()
		t.keyboardFallbackTimer = nil
	}
}

func (t *ProcessTerminal) disableKeyboardProtocols() error {
	t.mu.Lock()
	kittyActive := t.kittyProtocolActive
	modifyActive := t.modifyOtherKeysActive
	t.kittyProtocolActive = false
	t.modifyOtherKeysActive = false
	t.stopKeyboardFallbackLocked()
	t.mu.Unlock()
	if kittyActive {
		SetKittyProtocolActive(false)
		if err := t.Write("\x1b[<u"); err != nil {
			return err
		}
	}
	if modifyActive {
		return t.Write("\x1b[>4;0m")
	}
	return nil
}

func (t *ProcessTerminal) MoveBy(lines int) error {
	switch {
	case lines > 0:
		return t.Write(fmt.Sprintf("\x1b[%dB", lines))
	case lines < 0:
		return t.Write(fmt.Sprintf("\x1b[%dA", -lines))
	default:
		return nil
	}
}

func (t *ProcessTerminal) HideCursor() error      { return t.Write("\x1b[?25l") }
func (t *ProcessTerminal) ShowCursor() error      { return t.Write("\x1b[?25h") }
func (t *ProcessTerminal) ClearLine() error       { return t.Write("\x1b[K") }
func (t *ProcessTerminal) ClearFromCursor() error { return t.Write("\x1b[J") }
func (t *ProcessTerminal) ClearScreen() error     { return t.Write("\x1b[2J\x1b[H") }
func (t *ProcessTerminal) SetTitle(title string) error {
	return t.Write("\x1b]0;" + title + "\x07")
}
func (t *ProcessTerminal) SetProgress(active bool) error {
	if active {
		t.mu.Lock()
		if t.progressStop == nil {
			stop := make(chan struct{})
			t.progressStop = stop
			go t.progressKeepalive(stop)
		}
		t.mu.Unlock()
		return t.Write(terminalProgressActive)
	}
	_ = t.clearProgressInterval()
	return t.Write(terminalProgressClear)
}

func (t *ProcessTerminal) clearProgressInterval() bool {
	t.mu.Lock()
	stop := t.progressStop
	t.progressStop = nil
	t.mu.Unlock()
	if stop != nil {
		close(stop)
		return true
	}
	return false
}

func (t *ProcessTerminal) progressKeepalive(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = t.Write(terminalProgressActive)
		case <-stop:
			return
		}
	}
}
