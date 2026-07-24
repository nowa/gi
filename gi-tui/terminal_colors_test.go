package gitui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseOSC11BackgroundColorPiMatrix(t *testing.T) {
	tests := []struct {
		name string
		data string
		want RGBColor
	}{
		{
			name: "sixteen bit rgb",
			data: "\x1b]11;rgb:0000/8000/ffff\x07",
			want: RGBColor{R: 0, G: 128, B: 255},
		},
		{
			name: "six digit hex with string terminator",
			data: "\x1b]11;#ffffff\x1b\\",
			want: RGBColor{R: 255, G: 255, B: 255},
		},
		{
			name: "twelve digit hex",
			data: "\x1b]11;#00008000ffff\x07",
			want: RGBColor{R: 0, G: 128, B: 255},
		},
		{
			name: "rgba prefix",
			data: "\x1b]11;rgba:ff/00/80/ff\x07",
			want: RGBColor{R: 255, G: 0, B: 128},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := ParseOSC11BackgroundColor(test.data)
			if !ok || got != test.want {
				t.Fatalf("ParseOSC11BackgroundColor(%q) = %#v, %v, want %#v, true", test.data, got, ok, test.want)
			}
			if !IsOSC11BackgroundColorResponse(test.data) {
				t.Fatalf("IsOSC11BackgroundColorResponse(%q) = false", test.data)
			}
		})
	}
}

func TestParseOSC11BackgroundColorRejectsNonStrictOrInvalidResponses(t *testing.T) {
	nonStrict := []string{
		"x\x1b]11;#ffffff\x07",
		"\x1b]10;#ffffff\x07",
		"\x1b]11;#ffffff\x07x",
		"\x1b]11;#ffffff",
	}
	for _, data := range nonStrict {
		if IsOSC11BackgroundColorResponse(data) {
			t.Fatalf("non-strict response accepted: %q", data)
		}
		if _, ok := ParseOSC11BackgroundColor(data); ok {
			t.Fatalf("non-strict response parsed: %q", data)
		}
	}

	if !IsOSC11BackgroundColorResponse("\x1b]11;not-a-color\x07") {
		t.Fatalf("strict response shape should be recognized before payload parsing")
	}
	if _, ok := ParseOSC11BackgroundColor("\x1b]11;not-a-color\x07"); ok {
		t.Fatalf("invalid strict payload should not parse")
	}
}

func TestParseTerminalColorSchemeReportPiMatrix(t *testing.T) {
	tests := []struct {
		data string
		want TerminalColorScheme
		ok   bool
	}{
		{data: "\x1b[?997;1n", want: TerminalColorSchemeDark, ok: true},
		{data: "\x1b[?997;2n", want: TerminalColorSchemeLight, ok: true},
		{data: "\x1b[?997;3n"},
		{data: "\x1b[?996n"},
		{data: "x\x1b[?997;1n"},
	}
	for _, test := range tests {
		got, ok := ParseTerminalColorSchemeReport(test.data)
		if got != test.want || ok != test.ok {
			t.Fatalf("ParseTerminalColorSchemeReport(%q) = %q, %v, want %q, %v", test.data, got, ok, test.want, test.ok)
		}
	}
}

type terminalColorInputRecorder struct {
	mu     sync.Mutex
	inputs []string
}

func (r *terminalColorInputRecorder) Render(int) []string { return nil }
func (r *terminalColorInputRecorder) Invalidate()         {}
func (r *terminalColorInputRecorder) HandleInput(data string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inputs = append(r.inputs, data)
}
func (r *terminalColorInputRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.inputs...)
}

func TestTUIQueryTerminalBackgroundColorUsesFIFOInputBoundary(t *testing.T) {
	terminal := newFakeTerminal(80, 24)
	ui := NewTUI(terminal)
	recorder := &terminalColorInputRecorder{}
	ui.AddChild(recorder)
	ui.SetFocus(recorder)
	var listenerInputs []string
	ui.AddInputListener(func(data string) InputListenerResult {
		listenerInputs = append(listenerInputs, data)
		return InputListenerResult{}
	})

	type queryResult struct {
		color RGBColor
		ok    bool
		err   error
	}
	result := make(chan queryResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		color, ok, err := ui.QueryTerminalBackgroundColor(ctx)
		result <- queryResult{color: color, ok: ok, err: err}
	}()
	waitFor(t, func() bool { return strings.Contains(terminal.String(), osc11BackgroundColorQuery) })

	ui.HandleInput("\x1b]11;#ffffff\x07")

	got := <-result
	if got.err != nil || !got.ok || got.color != (RGBColor{R: 255, G: 255, B: 255}) {
		t.Fatalf("background query = %#v", got)
	}
	if len(listenerInputs) != 0 || len(recorder.snapshot()) != 0 {
		t.Fatalf("OSC reply leaked into normal input: listeners=%#v component=%#v", listenerInputs, recorder.snapshot())
	}
}

func TestTUIBackgroundQueryLeavesUnrelatedInputAndConsumesLateReply(t *testing.T) {
	terminal := newFakeTerminal(80, 24)
	ui := NewTUI(terminal)
	recorder := &terminalColorInputRecorder{}
	ui.AddChild(recorder)
	ui.SetFocus(recorder)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	result := make(chan error, 1)
	go func() {
		_, _, err := ui.QueryTerminalBackgroundColor(ctx)
		result <- err
	}()
	waitFor(t, func() bool { return strings.Contains(terminal.String(), osc11BackgroundColorQuery) })

	ui.HandleInput("x")
	if got := recorder.snapshot(); len(got) != 1 || got[0] != "x" {
		t.Fatalf("unrelated input = %#v, want x", got)
	}
	if err := <-result; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed-out query error = %v", err)
	}
	cancel()

	ui.HandleInput("\x1b]11;#ffffff\x07")
	if got := recorder.snapshot(); len(got) != 1 {
		t.Fatalf("late OSC reply leaked into component input: %#v", got)
	}
}

func TestTUIBackgroundQueryReturnsInvalidStrictPayloadAsUnavailable(t *testing.T) {
	terminal := newFakeTerminal(80, 24)
	ui := NewTUI(terminal)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	result := make(chan terminalBackgroundColorResult, 1)
	go func() {
		color, ok, _ := ui.QueryTerminalBackgroundColor(ctx)
		result <- terminalBackgroundColorResult{color: color, ok: ok}
	}()
	waitFor(t, func() bool { return strings.Contains(terminal.String(), osc11BackgroundColorQuery) })
	ui.HandleInput("\x1b]11;not-a-color\x07")

	got := <-result
	if got.ok {
		t.Fatalf("invalid strict response should resolve as unavailable: %#v", got)
	}
}

func TestTUIQueryTerminalColorSchemeAndNotifications(t *testing.T) {
	terminal := newFakeTerminal(80, 24)
	ui := NewTUI(terminal)
	ui.Start()
	terminal.ClearOutput()

	if err := ui.SetTerminalColorSchemeNotifications(true); err != nil {
		t.Fatal(err)
	}
	if err := ui.SetTerminalColorSchemeNotifications(true); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(terminal.String(), terminalColorNotificationsOn); got != 1 {
		t.Fatalf("notification enable writes = %d, want 1: %q", got, terminal.String())
	}

	type schemeResult struct {
		scheme TerminalColorScheme
		ok     bool
		err    error
	}
	result := make(chan schemeResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		scheme, ok, err := ui.QueryTerminalColorScheme(ctx)
		result <- schemeResult{scheme: scheme, ok: ok, err: err}
	}()
	waitFor(t, func() bool { return strings.Contains(terminal.String(), terminalColorSchemeQuery) })
	ui.HandleInput("\x1b[?997;2n")

	got := <-result
	if got.err != nil || !got.ok || got.scheme != TerminalColorSchemeLight {
		t.Fatalf("scheme query = %#v", got)
	}

	ui.StopWithoutRender()
	if got := strings.Count(terminal.String(), terminalColorNotificationsOff); got != 1 {
		t.Fatalf("notification disable writes = %d, want 1: %q", got, terminal.String())
	}
}

func TestTUITerminalColorSchemeListenerCanUnsubscribe(t *testing.T) {
	ui := NewTUI(newFakeTerminal(80, 24))
	var schemes []TerminalColorScheme
	unsubscribe := ui.OnTerminalColorSchemeChange(func(scheme TerminalColorScheme) {
		schemes = append(schemes, scheme)
	})
	ui.HandleInput("\x1b[?997;1n")
	unsubscribe()
	unsubscribe()
	ui.HandleInput("\x1b[?997;2n")

	if len(schemes) != 1 || schemes[0] != TerminalColorSchemeDark {
		t.Fatalf("listener schemes = %#v, want [dark]", schemes)
	}
}
