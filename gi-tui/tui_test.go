package gitui

import (
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeTerminal struct {
	mu      sync.Mutex
	output  strings.Builder
	cols    int
	rows    int
	moves   []int
	input   func(string)
	resize  func()
	stopped bool
}

func newFakeTerminal(cols, rows int) *fakeTerminal {
	return &fakeTerminal{cols: cols, rows: rows}
}

func (f *fakeTerminal) Start(onInput func(string), onResize func()) {
	f.input = onInput
	f.resize = onResize
}
func (f *fakeTerminal) Stop() { f.stopped = true }
func (f *fakeTerminal) DrainInput(_, _ time.Duration) error {
	return nil
}
func (f *fakeTerminal) Write(data string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, _ = f.output.WriteString(data)
	return nil
}
func (f *fakeTerminal) Columns() int              { return f.cols }
func (f *fakeTerminal) Rows() int                 { return f.rows }
func (f *fakeTerminal) KittyProtocolActive() bool { return false }
func (f *fakeTerminal) MoveBy(lines int) error {
	f.moves = append(f.moves, lines)
	return nil
}
func (f *fakeTerminal) HideCursor() error             { return nil }
func (f *fakeTerminal) ShowCursor() error             { return nil }
func (f *fakeTerminal) ClearLine() error              { return nil }
func (f *fakeTerminal) ClearFromCursor() error        { return nil }
func (f *fakeTerminal) ClearScreen() error            { return nil }
func (f *fakeTerminal) SetTitle(title string) error   { return nil }
func (f *fakeTerminal) SetProgress(active bool) error { return nil }
func (f *fakeTerminal) String() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.output.String()
}
func (f *fakeTerminal) ClearOutput() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.output.Reset()
}

func (f *fakeTerminal) Moves() []int {
	out := make([]int, len(f.moves))
	copy(out, f.moves)
	return out
}

type lineComponent struct {
	lines []string
}

func (c *lineComponent) Render(_ int) []string { return append([]string(nil), c.lines...) }
func (c *lineComponent) Invalidate()           {}

type inputRecorderComponent struct {
	inputs []string
}

func (c *inputRecorderComponent) Render(_ int) []string { return []string{""} }
func (c *inputRecorderComponent) Invalidate()           {}
func (c *inputRecorderComponent) HandleInput(data string) {
	c.inputs = append(c.inputs, data)
}

type keyReleaseRecorderComponent struct {
	inputRecorderComponent
	wantsRelease bool
}

func (c *keyReleaseRecorderComponent) WantsKeyRelease() bool { return c.wantsRelease }

type focusableOverlayComponent struct {
	FocusState
	lines  []string
	inputs []string
}

func (c *focusableOverlayComponent) Render(_ int) []string { return append([]string(nil), c.lines...) }
func (c *focusableOverlayComponent) Invalidate()           {}
func (c *focusableOverlayComponent) HandleInput(data string) {
	c.inputs = append(c.inputs, data)
}

type widthRecorderComponent struct {
	lines          []string
	requestedWidth int
}

func (c *widthRecorderComponent) Render(width int) []string {
	c.requestedWidth = width
	return append([]string(nil), c.lines...)
}
func (c *widthRecorderComponent) Invalidate() {}

type sizeRecorderComponent struct {
	width  int
	height int
}

func (c *sizeRecorderComponent) Render(int) []string { return []string{"plain"} }
func (c *sizeRecorderComponent) RenderWithSize(width, height int) []string {
	c.width = width
	c.height = height
	return []string{"sized"}
}
func (c *sizeRecorderComponent) Invalidate() {}

type cellDimensionRecorderComponent struct {
	invalidations int
}

func (c *cellDimensionRecorderComponent) Render(_ int) []string {
	dims := GetCellDimensions()
	return []string{strconv.Itoa(dims.Width) + "x" + strconv.Itoa(dims.Height)}
}
func (c *cellDimensionRecorderComponent) Invalidate() { c.invalidations++ }

type keyTesterComponent struct {
	ui       *TUI
	log      []string
	maxLines int
}

func (c *keyTesterComponent) HandleInput(data string) {
	if MatchesKey(data, "ctrl+c") {
		c.ui.Stop()
		return
	}
	repr := strings.NewReplacer("\x1b", "\\x1b", "\r", "\\r", "\n", "\\n", "\t", "\\t", "\x7f", "\\x7f").Replace(data)
	c.log = append(c.log, `Repr: "`+repr+`"`)
	if c.maxLines <= 0 {
		c.maxLines = 20
	}
	if len(c.log) > c.maxLines {
		c.log = c.log[len(c.log)-c.maxLines:]
	}
}

func (c *keyTesterComponent) Invalidate() {}
func (c *keyTesterComponent) Render(width int) []string {
	lines := []string{
		strings.Repeat("=", width),
		TruncateToWidth("Key Code Tester - Press keys to see their codes (Ctrl+C to exit)", width, "", true),
		strings.Repeat("=", width),
		"",
	}
	for _, entry := range c.log {
		lines = append(lines, TruncateToWidth(entry, width, "", true))
	}
	lines = append(lines,
		strings.Repeat("=", width),
		TruncateToWidth("Test these:", width, "", true),
		TruncateToWidth("  - Shift + Enter", width, "", true),
		TruncateToWidth("  - Option/Alt + Backspace", width, "", true),
		strings.Repeat("=", width),
	)
	return lines
}

func TestTUIRoutesInputToFocusedComponent(t *testing.T) {
	terminal := newFakeTerminal(20, 10)
	ui := NewTUI(terminal)
	input := NewInput()
	ui.AddChild(input)
	ui.SetFocus(input)
	ui.HandleInput("x")
	if input.Text() != "x" {
		t.Fatalf("input text = %q, want x", input.Text())
	}
}

func TestTUIFiltersKeyReleaseUnlessComponentOptsIn(t *testing.T) {
	SetKittyProtocolActive(true)
	defer SetKittyProtocolActive(false)

	terminal := newFakeTerminal(20, 10)
	ui := NewTUI(terminal)
	recorder := &keyReleaseRecorderComponent{}
	ui.SetFocus(recorder)
	ui.HandleInput("\x1b[97;1:3u")
	if len(recorder.inputs) != 0 {
		t.Fatalf("release event should be filtered by default, got %#v", recorder.inputs)
	}

	recorder.wantsRelease = true
	ui.HandleInput("\x1b[97;1:3u")
	if len(recorder.inputs) != 1 || recorder.inputs[0] != "\x1b[97;1:3u" {
		t.Fatalf("opted-in release inputs = %#v", recorder.inputs)
	}
}

func TestTUIInputListenersPreserveOrderAndCanClearData(t *testing.T) {
	terminal := newFakeTerminal(20, 10)
	ui := NewTUI(terminal)
	recorder := &inputRecorderComponent{}
	ui.SetFocus(recorder)
	var seen []string

	removeFirst := ui.AddInputListener(func(data string) InputListenerResult {
		seen = append(seen, "first:"+data)
		return InputListenerData(data + "a")
	})
	ui.AddInputListener(func(data string) InputListenerResult {
		seen = append(seen, "second:"+data)
		return InputListenerData("")
	})

	ui.HandleInput("x")
	if strings.Join(seen, "|") != "first:x|second:xa" {
		t.Fatalf("listener order/data = %#v", seen)
	}
	if len(recorder.inputs) != 0 {
		t.Fatalf("empty listener data should stop forwarding, got %#v", recorder.inputs)
	}

	removeFirst()
	seen = nil
	ui.RemoveInputListener(func(data string) InputListenerResult { return InputListenerResult{} })
	ui.HandleInput("y")
	if strings.Join(seen, "|") != "second:y" {
		t.Fatalf("remaining listener should preserve registration order/removal, got %#v", seen)
	}
}

func TestIsFocusable(t *testing.T) {
	if !IsFocusable(&focusableOverlayComponent{}) {
		t.Fatalf("focusable component should be detected")
	}
	if IsFocusable(&lineComponent{}) || IsFocusable(nil) {
		t.Fatalf("non-focusable component should not be detected")
	}
}

func TestTUIDefaultOptionsFromPiEnvironment(t *testing.T) {
	t.Setenv("PI_HARDWARE_CURSOR", "1")
	t.Setenv("PI_CLEAR_ON_SHRINK", "1")

	ui := NewTUI(newFakeTerminal(20, 5))
	if !ui.ShowHardwareCursor() {
		t.Fatalf("PI_HARDWARE_CURSOR=1 should enable hardware cursor by default")
	}
	if !ui.ClearOnShrink() {
		t.Fatalf("PI_CLEAR_ON_SHRINK=1 should enable clear-on-shrink by default")
	}

	explicit := NewTUI(newFakeTerminal(20, 5), false)
	if explicit.ShowHardwareCursor() {
		t.Fatalf("explicit constructor cursor option should override PI_HARDWARE_CURSOR")
	}
}

func TestContainerSupportsPiStyleOrderedChildMutation(t *testing.T) {
	container := NewContainer()
	welcome := &lineComponent{lines: []string{"welcome"}}
	editor := &lineComponent{lines: []string{"editor"}}
	userMessage := &lineComponent{lines: []string{"user"}}
	botMessage := &lineComponent{lines: []string{"bot"}}

	container.AddChild(welcome)
	container.AddChild(editor)
	container.InsertChild(container.ChildCount()-1, userMessage)
	container.InsertChild(99, botMessage)

	if got, want := container.Render(20), []string{"welcome", "user", "editor", "bot"}; !equalLines(got, want) {
		t.Fatalf("inserted children render order = %#v, want %#v", got, want)
	}

	children := container.Children()
	children[0] = nil
	if got := container.Render(20)[0]; got != "welcome" {
		t.Fatalf("Children should return a defensive copy, first rendered line = %q", got)
	}

	if removed := container.RemoveChildAt(1); removed != userMessage {
		t.Fatalf("RemoveChildAt removed %#v, want user message", removed)
	}
	if removed := container.RemoveChildAt(-1); removed != nil {
		t.Fatalf("out-of-range RemoveChildAt should return nil, got %#v", removed)
	}

	container.SetChildren([]Component{welcome, nil, editor})
	if got, want := container.Render(20), []string{"welcome", "editor"}; !equalLines(got, want) {
		t.Fatalf("SetChildren render order = %#v, want %#v", got, want)
	}
}

func TestContainerConcurrentMutationAndRenderIsSnapshotSafe(t *testing.T) {
	container := NewContainer()
	base := &lineComponent{lines: []string{"base"}}
	alt := &lineComponent{lines: []string{"alt"}}
	container.AddChild(base)

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				switch (worker + i) % 7 {
				case 0:
					container.AddChild(&lineComponent{lines: []string{"child"}})
				case 1:
					container.InsertChild(1, alt)
				case 2:
					container.RemoveChild(alt)
				case 3:
					_ = container.RemoveChildAt(container.ChildCount() - 1)
				case 4:
					container.SetChildren([]Component{base, alt})
				case 5:
					_ = container.Children()
					_ = container.Render(20)
				default:
					container.Invalidate()
					_ = container.RenderWithSize(20, 5)
				}
			}
		}()
	}
	wg.Wait()

	for _, line := range container.Render(20) {
		if VisibleWidth(line) > 20 {
			t.Fatalf("rendered line exceeds width: %q", line)
		}
	}
}

func TestTUIRuntimeStateConcurrentAccessIsSafe(t *testing.T) {
	ui := NewTUI(newFakeTerminal(30, 8))
	ui.AddChild(&lineComponent{lines: []string{"base"}})
	overlay := &lineComponent{lines: []string{"overlay"}}
	handle := ui.ShowOverlay(overlay)

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				switch (worker + i) % 9 {
				case 0:
					handle.SetHidden(i%2 == 0)
				case 1:
					_ = handle.IsHidden()
				case 2:
					handle.Focus()
				case 3:
					handle.Unfocus()
				case 4:
					_ = handle.IsFocused()
				case 5:
					ui.SetClearOnShrink(i%2 == 0)
					_ = ui.ClearOnShrink()
				case 6:
					ui.SetShowHardwareCursor(i%2 == 0)
					_ = ui.ShowHardwareCursor()
				case 7:
					ui.SetOnDebug(func() {})
					ui.HandleInput("\x1b[68;6u")
				default:
					ui.RequestRender(false)
				}
			}
		}()
	}
	wg.Wait()
	handle.Hide()
}

func TestTUIUsesSizeAwareRendering(t *testing.T) {
	terminal := newFakeTerminal(20, 12)
	ui := NewTUI(terminal)
	component := &sizeRecorderComponent{}
	ui.AddChild(component)
	ui.RequestRender(true)

	if component.width != 20 || component.height != 12 {
		t.Fatalf("size-aware render got %dx%d, want 20x12", component.width, component.height)
	}
	if !strings.Contains(terminal.String(), "sized") || strings.Contains(terminal.String(), "plain") {
		t.Fatalf("TUI should use RenderWithSize output, got %q", terminal.String())
	}
}

func TestTUIChatSimpleDemoFlow(t *testing.T) {
	terminal := NewVirtualTerminal(80, 12)
	ui := NewTUI(terminal)
	ui.AddChild(NewText("Welcome to Simple Chat!\n\nType your messages below. Type '/' for commands. Press Ctrl+C to exit.", 1, 1))

	editor := NewEditor(EditorTheme{Border: func(text string) string { return text }, SelectList: SelectListTheme{}}, EditorOptions{PaddingX: 1})
	editor.SetAutocompleteProvider(NewCombinedAutocompleteProviderWithCommandItems(t.TempDir(), []AutocompleteItem{
		{Value: "delete", Description: "Delete the last message"},
		{Value: "clear", Description: "Clear all messages"},
	}))
	ui.AddChild(editor)
	ui.SetFocus(editor)

	var responding bool
	var loader *Loader
	editor.SetOnSubmit(func(value string) {
		if responding {
			return
		}
		switch strings.TrimSpace(value) {
		case "/delete":
			if ui.ChildCount() > 3 {
				ui.RemoveChildAt(ui.ChildCount() - 2)
			}
			ui.RequestRender()
		case "/clear":
			children := ui.Children()
			if len(children) >= 2 {
				ui.SetChildren([]Component{children[0], children[len(children)-1]})
			}
			ui.RequestRender()
		case "":
			return
		default:
			responding = true
			editor.DisableSubmit = true
			ui.InsertChild(ui.ChildCount()-1, NewMarkdownWithOptions(value, MarkdownOptions{PaddingX: 1, PaddingY: 1}))
			loader = NewLoader("Thinking...", LoaderIndicatorOptions{TUI: ui, Frames: []string{"."}, RenderIndicatorVerbatim: true})
			ui.InsertChild(ui.ChildCount()-1, loader)
			ui.RequestRender()
		}
	})

	ui.Start()
	defer ui.Stop()

	editor.SetText("hello world")
	terminal.SendInput("\r")
	viewport := strings.Join(terminal.GetViewport(), "\n")
	if !responding || !editor.DisableSubmit || ui.ChildCount() != 4 {
		t.Fatalf("submit should insert user message and loader, responding=%v disable=%v childCount=%d", responding, editor.DisableSubmit, ui.ChildCount())
	}
	if !strings.Contains(viewport, "hello world") || !strings.Contains(viewport, ". Thinking...") {
		t.Fatalf("chat submit viewport missing user message/loader: %q", viewport)
	}

	editor.SetText("ignored while responding")
	terminal.SendInput("\r")
	if ui.ChildCount() != 4 {
		t.Fatalf("disabled submit should not add children, childCount=%d", ui.ChildCount())
	}

	ui.RemoveChild(loader)
	ui.InsertChild(ui.ChildCount()-1, NewMarkdownWithOptions("I see what you mean.", MarkdownOptions{PaddingX: 1, PaddingY: 1}))
	responding = false
	editor.DisableSubmit = false
	ui.RequestRender(true)
	viewport = strings.Join(terminal.GetViewport(), "\n")
	if strings.Contains(viewport, "Thinking...") || !strings.Contains(viewport, "I see what you mean.") {
		t.Fatalf("chat response viewport = %q", viewport)
	}

	editor.SetText("/delete")
	terminal.SendInput("\r")
	viewport = strings.Join(terminal.GetViewport(), "\n")
	if strings.Contains(viewport, "I see what you mean.") || !strings.Contains(viewport, "hello world") {
		t.Fatalf("/delete should remove latest message only, viewport=%q", viewport)
	}

	editor.SetText("/clear")
	terminal.SendInput("\r")
	viewport = strings.Join(terminal.GetViewport(), "\n")
	if strings.Contains(viewport, "hello world") || !strings.Contains(viewport, "Welcome to Simple Chat!") {
		t.Fatalf("/clear should keep welcome and editor only, viewport=%q", viewport)
	}
}

func TestTUIKeyTesterDemoFlow(t *testing.T) {
	terminal := NewVirtualTerminal(80, 12)
	ui := NewTUI(terminal)
	logger := &keyTesterComponent{ui: ui, maxLines: 2}
	ui.AddChild(logger)
	ui.SetFocus(logger)
	ui.Start()

	terminal.SendInput("a")
	terminal.SendInput("\x1b[A")
	terminal.SendInput("\t")
	ui.RequestRender(true)

	viewport := strings.Join(terminal.GetViewport(), "\n")
	if !strings.Contains(viewport, "Key Code Tester") || !strings.Contains(viewport, `Repr: "\x1b[A"`) || !strings.Contains(viewport, `Repr: "\t"`) {
		t.Fatalf("key tester viewport missing title or latest key logs: %q", viewport)
	}
	if strings.Contains(viewport, `Repr: "a"`) {
		t.Fatalf("key tester should retain only latest maxLines entries, viewport=%q", viewport)
	}

	terminal.ClearOutput()
	terminal.SendInput("\x03")
	if !strings.Contains(terminal.Output(), "\x1b[?2004l") {
		t.Fatalf("ctrl+c should stop TUI and disable bracketed paste, output=%q", terminal.Output())
	}
}

func TestTUIStartInitialRenderDoesNotClearScreen(t *testing.T) {
	terminal := newFakeTerminal(20, 5)
	ui := NewTUI(terminal)
	ui.AddChild(&lineComponent{lines: []string{"alpha", "bravo"}})

	ui.Start()

	output := terminal.String()
	if strings.Contains(output, "\x1b[2J") || strings.Contains(output, "\x1b[3J") {
		t.Fatalf("initial Start render should not clear screen like Pi first render: %q", output)
	}
	if !strings.Contains(output, "alpha") || !strings.Contains(output, "bravo") {
		t.Fatalf("initial Start render missing content: %q", output)
	}
}

func TestTUIRejectsOverwideCustomComponentLinesLikePi(t *testing.T) {
	terminal := newFakeTerminal(10, 4)
	ui := NewTUI(terminal)
	ui.AddChild(&lineComponent{lines: []string{"this line is too wide"}})

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected render to panic for over-wide custom component line")
		}
		message, ok := recovered.(string)
		if !ok {
			t.Fatalf("panic type = %T, want string", recovered)
		}
		if !strings.Contains(message, "rendered line 0 exceeds terminal width") || !strings.Contains(message, "VisibleWidth") {
			t.Fatalf("panic message = %q", message)
		}
		if output := terminal.String(); output != "" {
			t.Fatalf("terminal should not receive partial over-wide render output, got %q", output)
		}
	}()

	ui.RequestRender(false)
}

func TestTUIPositionsHardwareCursorAtVisibleMarker(t *testing.T) {
	terminal := newFakeTerminal(20, 4)
	ui := NewTUI(terminal, true)
	ui.AddChild(&lineComponent{lines: []string{"alpha", "br" + CursorMarker + "avo"}})

	ui.RequestRender(true)
	output := terminal.String()
	if strings.Contains(output, CursorMarker) {
		t.Fatalf("render output should strip cursor marker: %q", output)
	}
	if !strings.Contains(output, "\x1b[2;3H\x1b[?25h") {
		t.Fatalf("hardware cursor should move to marker and show cursor: %q", output)
	}
}

func TestTUIIgnoresCursorMarkerOutsideViewport(t *testing.T) {
	terminal := newFakeTerminal(20, 2)
	ui := NewTUI(terminal, true)
	ui.AddChild(&lineComponent{lines: []string{"old" + CursorMarker, "visible 1", "visible 2"}})

	ui.RequestRender(true)
	output := terminal.String()
	if strings.Contains(output, CursorMarker) {
		t.Fatalf("render output should strip offscreen cursor marker: %q", output)
	}
	if strings.Contains(output, "\x1b[1;4H") {
		t.Fatalf("offscreen cursor marker should not position hardware cursor: %q", output)
	}
}

func TestTUIHidesHardwareCursorWhenMarkerDisappears(t *testing.T) {
	terminal := NewVirtualTerminal(20, 4)
	ui := NewTUI(terminal, true)
	component := &lineComponent{lines: []string{"ab" + CursorMarker + "cd"}}
	ui.AddChild(component)

	ui.RequestRender(true)
	terminal.ClearOutput()

	component.lines = []string{"abcd"}
	ui.RequestRender(false)

	if output := terminal.Output(); !strings.Contains(output, "\x1b[?25l") {
		t.Fatalf("marker disappearance should hide hardware cursor: %q", output)
	}
}

func TestTUISetShowHardwareCursorRerendersCursorVisibility(t *testing.T) {
	terminal := NewVirtualTerminal(20, 4)
	ui := NewTUI(terminal, false)
	ui.AddChild(&lineComponent{lines: []string{"ab" + CursorMarker + "cd"}})

	ui.RequestRender(true)
	terminal.ClearOutput()
	ui.SetShowHardwareCursor(true)

	output := terminal.Output()
	if !strings.Contains(output, "\x1b[1;3H") || !strings.Contains(output, "\x1b[?25h") {
		t.Fatalf("enabling hardware cursor should rerender position and show cursor: %q", output)
	}

	terminal.ClearOutput()
	ui.SetShowHardwareCursor(false)
	output = terminal.Output()
	if !strings.Contains(output, "\x1b[?25l") {
		t.Fatalf("disabling hardware cursor should hide cursor: %q", output)
	}
}

func TestTUIStopMovesFromHardwareCursorToContentEnd(t *testing.T) {
	terminal := newFakeTerminal(20, 4)
	ui := NewTUI(terminal, true)
	ui.AddChild(&lineComponent{lines: []string{"alpha", "br" + CursorMarker + "avo", "charlie"}})

	ui.RequestRender(true)
	ui.Stop()

	moves := terminal.Moves()
	if len(moves) != 1 || moves[0] != 2 {
		t.Fatalf("Stop MoveBy calls = %#v, want [2]", moves)
	}
	if !terminal.stopped {
		t.Fatalf("terminal should be stopped")
	}

	terminal = newFakeTerminal(20, 4)
	ui = NewTUI(terminal)
	ui.AddChild(&lineComponent{lines: []string{"alpha", "bravo", "charlie"}})
	ui.RequestRender(true)
	ui.Stop()

	moves = terminal.Moves()
	if len(moves) != 1 || moves[0] != 1 {
		t.Fatalf("Stop without cursor marker MoveBy calls = %#v, want [1]", moves)
	}
}

func TestTUIOverlayRendersAndRestoresFocus(t *testing.T) {
	terminal := newFakeTerminal(20, 10)
	ui := NewTUI(terminal)
	input := NewInput()
	ui.AddChild(NewText("base", 0, 0))
	ui.SetFocus(input)
	handle := ui.ShowOverlay(NewText("menu", 0, 0), OverlayOptions{Width: ptr(6), Anchor: OverlayTopLeft})
	ui.RequestRender(true)
	if !strings.Contains(terminal.String(), "menu") {
		t.Fatalf("overlay output missing menu: %q", terminal.String())
	}
	if !handle.IsFocused() {
		t.Fatalf("overlay should be focused")
	}
	handle.Hide()
	if ui.FocusedComponent() != input {
		t.Fatalf("focus was not restored")
	}
}

func TestTUIOverlayLifecycleUsesDiffRenderWithoutClearing(t *testing.T) {
	SetCapabilities(TerminalCapabilities{})
	defer ResetCapabilitiesCache()

	terminal := newFakeTerminal(20, 6)
	ui := NewTUI(terminal)
	ui.AddChild(&lineComponent{lines: []string{"base"}})
	ui.Start()
	terminal.ClearOutput()

	handle := ui.ShowOverlay(&lineComponent{lines: []string{"overlay"}}, OverlayOptions{Row: ptr(0), Col: ptr(0), Width: ptr(7)})
	output := terminal.String()
	if strings.Contains(output, "\x1b[2J") || strings.Contains(output, "\x1b[3J") {
		t.Fatalf("showOverlay should diff render without clearing like Pi: %q", output)
	}
	if !strings.Contains(output, "overlay") {
		t.Fatalf("showOverlay output missing overlay: %q", output)
	}

	terminal.ClearOutput()
	handle.SetHidden(true)
	output = terminal.String()
	if strings.Contains(output, "\x1b[2J") || strings.Contains(output, "\x1b[3J") {
		t.Fatalf("setHidden should diff render without clearing like Pi: %q", output)
	}

	terminal.ClearOutput()
	handle.SetHidden(false)
	output = terminal.String()
	if strings.Contains(output, "\x1b[2J") || strings.Contains(output, "\x1b[3J") {
		t.Fatalf("unhide overlay should diff render without clearing like Pi: %q", output)
	}

	terminal.ClearOutput()
	handle.Hide()
	output = terminal.String()
	if strings.Contains(output, "\x1b[2J") || strings.Contains(output, "\x1b[3J") {
		t.Fatalf("hide overlay should diff render without clearing like Pi: %q", output)
	}
}

func TestTUIOverlaySuppressesClearOnShrinkFullRedraw(t *testing.T) {
	terminal := newFakeTerminal(20, 6)
	ui := NewTUI(terminal)
	ui.SetClearOnShrink(true)
	component := &lineComponent{lines: []string{"base 0", "base 1", "base 2", "base 3"}}
	ui.AddChild(component)
	ui.ShowOverlay(&lineComponent{lines: []string{"overlay"}}, OverlayOptions{Row: ptr(0), Col: ptr(0), Width: ptr(7)})
	ui.RequestRender(true)
	terminal.ClearOutput()

	component.lines = []string{"base 0"}
	ui.RequestRender(false)

	output := terminal.String()
	if strings.Contains(output, "\x1b[2J") || strings.Contains(output, "\x1b[3J") {
		t.Fatalf("clearOnShrink should not force full redraw while overlays are stacked like Pi: %q", output)
	}
}

func TestTUIOverlayRendersWhenContentShorterThanTerminal(t *testing.T) {
	terminal := NewVirtualTerminal(80, 24)
	ui := NewTUI(terminal)
	ui.AddChild(&lineComponent{lines: []string{"Line 1", "Line 2", "Line 3"}})
	ui.ShowOverlay(&lineComponent{lines: []string{"OVERLAY_TOP", "OVERLAY_MID", "OVERLAY_BOT"}})
	ui.Start()
	defer ui.Stop()

	found := false
	for _, line := range terminal.GetViewport() {
		if strings.Contains(line, "OVERLAY_") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("overlay should be visible in short viewport: %#v", terminal.GetViewport())
	}
}

func TestTUIOverlayCompositesRelativeToVisibleViewport(t *testing.T) {
	terminal := NewVirtualTerminal(20, 5)
	ui := NewTUI(terminal)
	base := &lineComponent{}
	for i := 0; i < 10; i++ {
		base.lines = append(base.lines, "Line "+strconv.Itoa(i))
	}
	ui.AddChild(base)
	ui.ShowOverlay(&lineComponent{lines: []string{"OVERLAY"}}, OverlayOptions{
		Anchor: OverlayTopLeft,
		Width:  ptr(7),
	})

	ui.RequestRender(true)

	viewport := terminal.GetViewport()
	if !strings.HasPrefix(viewport[0], "OVERLAY") {
		t.Fatalf("top-left overlay should be composited onto visible viewport row 0, viewport=%#v", viewport)
	}
	if strings.Contains(strings.Join(viewport[1:], "\n"), "Line 0") {
		t.Fatalf("overlay should not drag scrollback line 0 into the visible viewport: %#v", viewport)
	}
}

func TestTUIOverlayLinesKeepResetAfterSlicing(t *testing.T) {
	terminal := NewVirtualTerminal(20, 6)
	ui := NewTUI(terminal)
	baseLine := "\x1b[3m" + strings.Repeat("X", 20) + "\x1b[23m"
	ui.AddChild(&lineComponent{lines: []string{baseLine, "INPUT"}})
	ui.ShowOverlay(&lineComponent{lines: []string{"OVR"}}, OverlayOptions{Row: ptr(0), Col: ptr(5), Width: ptr(3)})

	ui.mu.Lock()
	lines := ui.renderLocked()
	ui.mu.Unlock()
	if len(lines) < 2 {
		t.Fatalf("rendered lines = %#v", lines)
	}
	if !strings.Contains(lines[0], "OVR") {
		t.Fatalf("overlay line missing OVR: %q", lines[0])
	}
	for idx, line := range lines[:2] {
		if !strings.Contains(line, "\x1b[0m") {
			t.Fatalf("line %d missing reset after overlay slicing: %q", idx, line)
		}
	}
}

func TestVirtualTerminalTracksItalicResetAtLineBoundary(t *testing.T) {
	terminal := NewVirtualTerminal(20, 6)
	ui := NewTUI(terminal)
	baseLine := "\x1b[3m" + strings.Repeat("X", 20) + "\x1b[23m"
	ui.AddChild(&lineComponent{lines: []string{baseLine, "INPUT"}})
	ui.Start()
	defer ui.Stop()

	cell, ok := terminal.GetCell(1, 0)
	if !ok {
		t.Fatalf("missing cell at row 1 col 0")
	}
	if cell.Italic {
		t.Fatalf("italic leaked to next line cell: %#v", cell)
	}
}

func TestVirtualTerminalTracksItalicResetAfterOverlaySlicing(t *testing.T) {
	terminal := NewVirtualTerminal(20, 6)
	ui := NewTUI(terminal)
	baseLine := "\x1b[3m" + strings.Repeat("X", 20) + "\x1b[23m"
	ui.AddChild(&lineComponent{lines: []string{baseLine, "INPUT"}})
	ui.ShowOverlay(&lineComponent{lines: []string{"OVR"}}, OverlayOptions{Row: ptr(0), Col: ptr(5), Width: ptr(3)})
	ui.Start()
	defer ui.Stop()

	cell, ok := terminal.GetCell(1, 0)
	if !ok {
		t.Fatalf("missing cell at row 1 col 0")
	}
	if cell.Italic {
		t.Fatalf("italic leaked after overlay slicing: %#v", cell)
	}
}

func TestVirtualTerminalTracksBasicSGRCellStyles(t *testing.T) {
	terminal := NewVirtualTerminal(40, 4)
	if err := terminal.Write("\x1b[1mB\x1b[22m \x1b[2mD\x1b[22m \x1b[4mU\x1b[24m \x1b[7mR\x1b[27m \x1b[9mS\x1b[29m \x1b[3mI\x1b[23m \x1b[5mK\x1b[25m \x1b[8mC\x1b[28m \x1b[53mO\x1b[55m \x1b[1mQ\x1b[21mP\x1b[24m\x1b[22mZ"); err != nil {
		t.Fatal(err)
	}

	assertStyle := func(col int, name string, check func(VirtualCell) bool) {
		t.Helper()
		cell, ok := terminal.GetCell(0, col)
		if !ok {
			t.Fatalf("missing cell at col %d", col)
		}
		if !check(cell) {
			t.Fatalf("%s style missing at col %d: %#v", name, col, cell)
		}
	}
	assertNoStyle := func(col int) {
		t.Helper()
		cell, ok := terminal.GetCell(0, col)
		if !ok {
			t.Fatalf("missing cell at col %d", col)
		}
		if cell.Bold || cell.Dim || cell.Italic || cell.Underline || cell.UnderlineStyle != "" || cell.Inverse || cell.Strikethrough || cell.Blink || cell.Conceal || cell.Overline {
			t.Fatalf("style leaked to col %d: %#v", col, cell)
		}
	}

	assertStyle(0, "bold", func(cell VirtualCell) bool { return cell.Bold })
	assertNoStyle(1)
	assertStyle(2, "dim", func(cell VirtualCell) bool { return cell.Dim })
	assertNoStyle(3)
	assertStyle(4, "underline", func(cell VirtualCell) bool { return cell.Underline })
	assertNoStyle(5)
	assertStyle(6, "inverse", func(cell VirtualCell) bool { return cell.Inverse })
	assertNoStyle(7)
	assertStyle(8, "strikethrough", func(cell VirtualCell) bool { return cell.Strikethrough })
	assertNoStyle(9)
	assertStyle(10, "italic", func(cell VirtualCell) bool { return cell.Italic })
	assertNoStyle(11)
	assertStyle(12, "blink", func(cell VirtualCell) bool { return cell.Blink })
	assertNoStyle(13)
	assertStyle(14, "conceal", func(cell VirtualCell) bool { return cell.Conceal })
	assertNoStyle(15)
	assertStyle(16, "overline", func(cell VirtualCell) bool { return cell.Overline })
	assertNoStyle(17)
	assertStyle(18, "bold before SGR 21", func(cell VirtualCell) bool { return cell.Bold })
	assertStyle(19, "double underline from SGR 21 without clearing bold", func(cell VirtualCell) bool {
		return cell.Bold && cell.Underline && cell.UnderlineStyle == "double"
	})
	assertNoStyle(20)
}

func TestVirtualTerminalSaveRestoreCursorRestoresRenditionAndCharset(t *testing.T) {
	terminal := NewVirtualTerminal(20, 4)
	if err := terminal.Write("\x1b[31mA\x1b7\x1b[32mB\x1b8C"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "AC" {
		t.Fatalf("viewport = %q, want AC", got)
	}
	cell, ok := terminal.GetCell(0, 1)
	if !ok {
		t.Fatalf("missing restored cell")
	}
	if cell.Foreground != (VirtualColor{Kind: "ansi", Index: 1}) {
		t.Fatalf("restored cell color = %#v, want red", cell.Foreground)
	}

	terminal.Reset()
	if err := terminal.Write("\x1b(0\x1b7\x1b(Bx\x1b8x"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "│" {
		t.Fatalf("charset restore viewport = %q, want DEC special graphics vertical bar", got)
	}

	terminal.Reset()
	if err := terminal.Write("\x1b[2;4r\x1b[?6h\x1b7\x1b[?6l\x1b8\x1b[1;1HZ"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport(); got[1] != "Z" || got[0] == "Z" {
		t.Fatalf("origin mode was not restored with cursor state: %#v", got)
	}

	terminal.Reset()
	terminal.Resize(3, 4)
	if err := terminal.Write("\x1b[?7l\x1b7\x1b[?7h\x1b8ABCD"); err != nil {
		t.Fatal(err)
	}
	got := terminal.GetViewport()
	if got[0] != "ABD" || got[1] != "" {
		t.Fatalf("wraparound mode was not restored with cursor state: %#v", got[:2])
	}
}

func TestVirtualTerminalSkipsExtendedColorSGRParameters(t *testing.T) {
	terminal := NewVirtualTerminal(20, 4)
	if err := terminal.Write("\x1b[38;2;1;4;9mA\x1b[0m\x1b[48;5;4mB\x1b[0m"); err != nil {
		t.Fatal(err)
	}
	for col := 0; col < 2; col++ {
		cell, ok := terminal.GetCell(0, col)
		if !ok {
			t.Fatalf("missing cell at col %d", col)
		}
		if cell.Underline || cell.Strikethrough {
			t.Fatalf("color parameters should not set text styles at col %d: %#v", col, cell)
		}
	}
}

func TestVirtualTerminalTracksBasicSGRColors(t *testing.T) {
	terminal := NewVirtualTerminal(40, 4)
	if err := terminal.Write("\x1b[31mR\x1b[39mN \x1b[94mB\x1b[39mP \x1b[42mG\x1b[49mX"); err != nil {
		t.Fatal(err)
	}

	assertColor := func(col int, field string, got, want VirtualColor) {
		t.Helper()
		if got != want {
			t.Fatalf("cell %d %s color = %#v, want %#v", col, field, got, want)
		}
	}
	cell, _ := terminal.GetCell(0, 0)
	assertColor(0, "foreground", cell.Foreground, VirtualColor{Kind: "ansi", Index: 1})
	cell, _ = terminal.GetCell(0, 1)
	assertColor(1, "foreground", cell.Foreground, VirtualColor{})
	cell, _ = terminal.GetCell(0, 3)
	assertColor(3, "foreground", cell.Foreground, VirtualColor{Kind: "ansi", Index: 12})
	cell, _ = terminal.GetCell(0, 4)
	assertColor(4, "foreground", cell.Foreground, VirtualColor{})
	cell, _ = terminal.GetCell(0, 6)
	assertColor(6, "background", cell.Background, VirtualColor{Kind: "ansi", Index: 2})
	cell, _ = terminal.GetCell(0, 7)
	assertColor(7, "background", cell.Background, VirtualColor{})
}

func TestVirtualTerminalTracksExtendedSGRColors(t *testing.T) {
	terminal := NewVirtualTerminal(40, 4)
	if err := terminal.Write("\x1b[38;5;196mI\x1b[48;2;1;2;3mB\x1b[58;2;4;5;6mU\x1b[59mN\x1b[0mP"); err != nil {
		t.Fatal(err)
	}

	cell, _ := terminal.GetCell(0, 0)
	if cell.Foreground != (VirtualColor{Kind: "index", Index: 196}) {
		t.Fatalf("indexed foreground = %#v", cell.Foreground)
	}
	cell, _ = terminal.GetCell(0, 1)
	if cell.Background != (VirtualColor{Kind: "rgb", R: 1, G: 2, B: 3}) {
		t.Fatalf("rgb background = %#v", cell.Background)
	}
	cell, _ = terminal.GetCell(0, 2)
	if cell.UnderlineColor != (VirtualColor{Kind: "rgb", R: 4, G: 5, B: 6}) {
		t.Fatalf("rgb underline color = %#v", cell.UnderlineColor)
	}
	cell, _ = terminal.GetCell(0, 3)
	if cell.UnderlineColor != (VirtualColor{}) {
		t.Fatalf("underline color should reset with SGR 59, got %#v", cell.UnderlineColor)
	}
	cell, _ = terminal.GetCell(0, 4)
	if cell.Foreground != (VirtualColor{}) || cell.Background != (VirtualColor{}) || cell.UnderlineColor != (VirtualColor{}) {
		t.Fatalf("SGR 0 should reset all colors, got %#v", cell)
	}
}

func TestVirtualTerminalTracksColonSeparatedExtendedSGRColors(t *testing.T) {
	terminal := NewVirtualTerminal(40, 4)
	if err := terminal.Write("\x1b[38:2::12:34:56mF\x1b[48:5:42mB\x1b[58:2::7:8:9mU"); err != nil {
		t.Fatal(err)
	}

	cell, _ := terminal.GetCell(0, 0)
	if cell.Foreground != (VirtualColor{Kind: "rgb", R: 12, G: 34, B: 56}) {
		t.Fatalf("colon rgb foreground = %#v", cell.Foreground)
	}
	cell, _ = terminal.GetCell(0, 1)
	if cell.Background != (VirtualColor{Kind: "index", Index: 42}) {
		t.Fatalf("colon indexed background = %#v", cell.Background)
	}
	cell, _ = terminal.GetCell(0, 2)
	if cell.UnderlineColor != (VirtualColor{Kind: "rgb", R: 7, G: 8, B: 9}) {
		t.Fatalf("colon rgb underline color = %#v", cell.UnderlineColor)
	}
}

func TestVirtualTerminalEraseAndInsertBlankCellsUseCurrentStyle(t *testing.T) {
	style := "\x1b[4;38;5;196;48;5;22m"
	assertStyledBlank := func(t *testing.T, terminal *VirtualTerminal, col int) {
		t.Helper()
		cell, ok := terminal.GetCell(0, col)
		if !ok {
			t.Fatalf("missing cell at col %d", col)
		}
		if cell.Rune != ' ' || !cell.Underline || cell.Foreground != (VirtualColor{Kind: "index", Index: 196}) || cell.Background != (VirtualColor{Kind: "index", Index: 22}) {
			t.Fatalf("blank cell at col %d did not use current style: %#v", col, cell)
		}
	}

	terminal := NewVirtualTerminal(12, 4)
	if err := terminal.Write(style + "abcdef\x1b[3D\x1b[2@"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "abc  def" {
		t.Fatalf("CSI @ line = %q, want abc two spaces def", got)
	}
	assertStyledBlank(t, terminal, 3)
	assertStyledBlank(t, terminal, 4)

	terminal.Reset()
	if err := terminal.Write(style + "abc\x1b[7G\x1b[2@"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "abc" {
		t.Fatalf("CSI @ past text line = %q, want abc with trailing blanks trimmed", got)
	}
	assertStyledBlank(t, terminal, 6)
	assertStyledBlank(t, terminal, 7)

	terminal.Reset()
	if err := terminal.Write(style + "abcdef\x1b[3D\x1b[2X"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "abc  f" {
		t.Fatalf("CSI X line = %q, want abc two spaces f", got)
	}
	assertStyledBlank(t, terminal, 3)
	assertStyledBlank(t, terminal, 4)

	terminal.Reset()
	if err := terminal.Write(style + "abc\x1b[7G\x1b[2X"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "abc" {
		t.Fatalf("CSI X past text line = %q, want abc with trailing blanks trimmed", got)
	}
	assertStyledBlank(t, terminal, 6)
	assertStyledBlank(t, terminal, 7)

	terminal.Reset()
	if err := terminal.Write(style + "abcdef\x1b[3D\x1b[K"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "abc" {
		t.Fatalf("CSI K line = %q, want abc", got)
	}
	assertStyledBlank(t, terminal, 3)
	assertStyledBlank(t, terminal, 5)

	terminal.Reset()
	if err := terminal.Write(style + "abcdef\x1b[3D\x1b[1K"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "    ef" {
		t.Fatalf("CSI 1K line = %q, want four spaces ef", got)
	}
	assertStyledBlank(t, terminal, 0)
	assertStyledBlank(t, terminal, 3)

	terminal.Reset()
	if err := terminal.Write(style + "abc\x1b[7G\x1b[1K"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "" {
		t.Fatalf("CSI 1K past text line = %q, want empty trimmed viewport line", got)
	}
	assertStyledBlank(t, terminal, 0)
	assertStyledBlank(t, terminal, 6)

	terminal.Reset()
	if err := terminal.Write(style + "abcdef\x1b[2K"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "" {
		t.Fatalf("CSI 2K line = %q, want empty viewport line", got)
	}
	assertStyledBlank(t, terminal, 0)
	assertStyledBlank(t, terminal, 5)

	terminal.Reset()
	if err := terminal.Write(style + "abcdef\x1b[4D\x1b[2P"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "abef" {
		t.Fatalf("CSI P line = %q, want abef", got)
	}
	assertStyledBlank(t, terminal, 4)
	assertStyledBlank(t, terminal, 5)
}

func TestVirtualTerminalEraseDisplayUsesCurrentStyle(t *testing.T) {
	style := "\x1b[4;38;5;196;48;5;22m"
	assertStyledBlank := func(t *testing.T, terminal *VirtualTerminal, row, col int) {
		t.Helper()
		cell, ok := terminal.GetCell(row, col)
		if !ok {
			t.Fatalf("missing cell at row %d col %d", row, col)
		}
		if cell.Rune != ' ' || !cell.Underline || cell.Foreground != (VirtualColor{Kind: "index", Index: 196}) || cell.Background != (VirtualColor{Kind: "index", Index: 22}) {
			t.Fatalf("blank cell at row %d col %d did not use current style: %#v", row, col, cell)
		}
	}

	terminal := NewVirtualTerminal(12, 4)
	if err := terminal.Write("first\r\nsecond\r\nthird\x1b[2;3H" + style + "\x1b[J"); err != nil {
		t.Fatal(err)
	}
	viewport := terminal.GetViewport()
	if viewport[0] != "first" || viewport[1] != "se" || viewport[2] != "" {
		t.Fatalf("CSI J erase-to-end viewport = %#v", viewport[:3])
	}
	assertStyledBlank(t, terminal, 1, 2)
	assertStyledBlank(t, terminal, 2, 0)

	terminal.Reset()
	if err := terminal.Write("first\r\nsecond\r\nthird\x1b[2;4H" + style + "\x1b[1J"); err != nil {
		t.Fatal(err)
	}
	viewport = terminal.GetViewport()
	if viewport[0] != "" || viewport[1] != "    nd" || viewport[2] != "third" {
		t.Fatalf("CSI 1J erase-to-start viewport = %#v", viewport[:3])
	}
	assertStyledBlank(t, terminal, 0, 0)
	assertStyledBlank(t, terminal, 1, 3)

	terminal.Reset()
	if err := terminal.Write("first\r\nsecond\x1b[2;4H" + style + "\x1b[2J"); err != nil {
		t.Fatal(err)
	}
	viewport = terminal.GetViewport()
	if viewport[0] != "" || viewport[1] != "" {
		t.Fatalf("CSI 2J erase display viewport = %#v", viewport[:2])
	}
	assertStyledBlank(t, terminal, 0, 0)
	assertStyledBlank(t, terminal, 1, 5)
	if x, y := terminal.GetCursorPosition(); x != 3 || y != 1 {
		t.Fatalf("CSI 2J should not move cursor, got (%d,%d), want (3,1)", x, y)
	}
}

func TestVirtualTerminalLineInsertDeleteAndScrollBlankLinesUseCurrentStyle(t *testing.T) {
	style := "\x1b[4;38;5;196;48;5;22m"
	assertStyledBlank := func(t *testing.T, terminal *VirtualTerminal, row, col int) {
		t.Helper()
		cell, ok := terminal.GetCell(row, col)
		if !ok {
			t.Fatalf("missing cell at row %d col %d", row, col)
		}
		if cell.Rune != ' ' || !cell.Underline || cell.Foreground != (VirtualColor{Kind: "index", Index: 196}) || cell.Background != (VirtualColor{Kind: "index", Index: 22}) {
			t.Fatalf("blank line cell at row %d col %d did not use current style: %#v", row, col, cell)
		}
	}

	terminal := NewVirtualTerminal(12, 5)
	if err := terminal.Write("one\r\ntwo\r\nthree\x1b[2;1H" + style + "\x1b[L"); err != nil {
		t.Fatal(err)
	}
	viewport := terminal.GetViewport()
	if viewport[0] != "one" || viewport[1] != "" || viewport[2] != "two" || viewport[3] != "three" {
		t.Fatalf("CSI L insert line viewport = %#v", viewport[:4])
	}
	assertStyledBlank(t, terminal, 1, 0)
	assertStyledBlank(t, terminal, 1, 5)

	terminal.Reset()
	if err := terminal.Write("one\r\ntwo\r\nthree\r\nfour\r\nfive\x1b[2;1H" + style + "\x1b[M"); err != nil {
		t.Fatal(err)
	}
	viewport = terminal.GetViewport()
	if viewport[0] != "one" || viewport[1] != "three" || viewport[2] != "four" || viewport[3] != "five" || viewport[4] != "" {
		t.Fatalf("CSI M delete line viewport = %#v", viewport)
	}
	assertStyledBlank(t, terminal, 4, 0)
	assertStyledBlank(t, terminal, 4, 5)

	terminal.Reset()
	if err := terminal.Write("one\r\ntwo\r\nthree" + style + "\x1b[S"); err != nil {
		t.Fatal(err)
	}
	viewport = terminal.GetViewport()
	if viewport[0] != "two" || viewport[1] != "three" || viewport[4] != "" {
		t.Fatalf("CSI S scroll up viewport = %#v", viewport)
	}
	assertStyledBlank(t, terminal, 4, 0)

	terminal.Reset()
	if err := terminal.Write("one\r\ntwo\r\nthree" + style + "\x1b[T"); err != nil {
		t.Fatal(err)
	}
	viewport = terminal.GetViewport()
	if viewport[0] != "" || viewport[1] != "one" || viewport[2] != "two" || viewport[3] != "three" {
		t.Fatalf("CSI T scroll down viewport = %#v", viewport[:4])
	}
	assertStyledBlank(t, terminal, 0, 0)

	terminal.Reset()
	if err := terminal.Write("A\r\nB\r\nC\r\nD\r\nE\x1b[2;4r\x1b[2;1H" + style + "\x1b[S"); err != nil {
		t.Fatal(err)
	}
	viewport = terminal.GetViewport()
	if viewport[0] != "A" || viewport[1] != "C" || viewport[2] != "D" || viewport[3] != "" || viewport[4] != "E" {
		t.Fatalf("scroll-region CSI S viewport = %#v", viewport)
	}
	assertStyledBlank(t, terminal, 3, 0)
}

func TestVirtualTerminalUnderlineColonStyleDoesNotSetDim(t *testing.T) {
	terminal := NewVirtualTerminal(40, 4)
	if err := terminal.Write("\x1b[4:2mD\x1b[4:0mN"); err != nil {
		t.Fatal(err)
	}
	cell, _ := terminal.GetCell(0, 0)
	if !cell.Underline || cell.Dim {
		t.Fatalf("SGR 4:2 should underline without dim, got %#v", cell)
	}
	if cell.UnderlineStyle != "double" {
		t.Fatalf("SGR 4:2 underline style = %q, want double", cell.UnderlineStyle)
	}
	cell, _ = terminal.GetCell(0, 1)
	if cell.Underline || cell.Dim {
		t.Fatalf("SGR 4:0 should reset underline without dim, got %#v", cell)
	}
}

func TestVirtualTerminalTracksUnderlineResetAtLineBoundary(t *testing.T) {
	terminal := NewVirtualTerminal(20, 6)
	ui := NewTUI(terminal)
	baseLine := "\x1b[4m" + strings.Repeat("X", 20) + "\x1b[24m"
	ui.AddChild(&lineComponent{lines: []string{baseLine, "INPUT"}})
	ui.Start()
	defer ui.Stop()

	cell, ok := terminal.GetCell(1, 0)
	if !ok {
		t.Fatalf("missing cell at row 1 col 0")
	}
	if cell.Underline {
		t.Fatalf("underline leaked to next line cell: %#v", cell)
	}
}

func TestVirtualTerminalCommonCSICursorAndEraseModes(t *testing.T) {
	terminal := NewVirtualTerminal(20, 5)
	if err := terminal.Write("abcde\x1b[3DZ\x1b[CY"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "abZdY" {
		t.Fatalf("CSI C/D overwrite line = %q, want abZdY", got)
	}
	if x, y := terminal.GetCursorPosition(); x != 5 || y != 0 {
		t.Fatalf("cursor after CSI C/D = (%d,%d), want (5,0)", x, y)
	}

	terminal.Reset()
	if err := terminal.Write("\x1b[4`X"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "   X" {
		t.Fatalf("CSI ` horizontal position line = %q, want three spaces X", got)
	}

	terminal.Reset()
	if err := terminal.Write("\x1b[2;2H\x1b[3aX"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[1]; got != "    X" {
		t.Fatalf("CSI a horizontal relative position line = %q, want four spaces X", got)
	}

	terminal.Reset()
	if err := terminal.Write("\x1b[2;3H\x1b[2eX"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[3]; got != "  X" {
		t.Fatalf("CSI e vertical relative position line = %q, want two spaces X", got)
	}

	terminal.Reset()
	if err := terminal.Write("abcdef\x1b[3D\x1b[2@"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "abc  def" {
		t.Fatalf("CSI @ insert chars line = %q, want abc two spaces def", got)
	}

	terminal.Reset()
	if err := terminal.Write("abcdef\x1b[4D\x1b[2P"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "abef" {
		t.Fatalf("CSI P delete chars line = %q, want abef", got)
	}

	terminal.Reset()
	if err := terminal.Write("ab\x1b[sXYZ\x1b[u!"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "ab!YZ" {
		t.Fatalf("CSI s/u cursor restore line = %q, want ab!YZ", got)
	}

	terminal.Reset()
	if err := terminal.Write("ab\x1b7XYZ\x1b8!"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "ab!YZ" {
		t.Fatalf("ESC 7/8 cursor restore line = %q, want ab!YZ", got)
	}

	terminal.Reset()
	if err := terminal.Write("one\r\ntwo\r\nthree\x1b[2;1H\x1b[L"); err != nil {
		t.Fatal(err)
	}
	viewport := terminal.GetViewport()
	if viewport[0] != "one" || viewport[1] != "" || viewport[2] != "two" || viewport[3] != "three" {
		t.Fatalf("CSI L insert line viewport = %#v", viewport[:4])
	}

	terminal.Reset()
	if err := terminal.Write("one\r\ntwo\r\nthree\x1b[2;1H\x1b[M"); err != nil {
		t.Fatal(err)
	}
	viewport = terminal.GetViewport()
	if viewport[0] != "one" || viewport[1] != "three" || viewport[2] != "" {
		t.Fatalf("CSI M delete line viewport = %#v", viewport[:3])
	}

	terminal.Reset()
	if err := terminal.Write("one\r\ntwo\r\nthree\x1b[S"); err != nil {
		t.Fatal(err)
	}
	viewport = terminal.GetViewport()
	if viewport[0] != "two" || viewport[1] != "three" || viewport[2] != "" {
		t.Fatalf("CSI S scroll up viewport = %#v", viewport[:3])
	}

	terminal.Reset()
	if err := terminal.Write("one\r\ntwo\r\nthree\x1b[T"); err != nil {
		t.Fatal(err)
	}
	viewport = terminal.GetViewport()
	if viewport[0] != "" || viewport[1] != "one" || viewport[2] != "two" || viewport[3] != "three" {
		t.Fatalf("CSI T scroll down viewport = %#v", viewport[:4])
	}

	terminal.Reset()
	if err := terminal.Write("\x1b[3;4H\x1b[2F\x1b[3E\x1b[2d"); err != nil {
		t.Fatal(err)
	}
	if x, y := terminal.GetCursorPosition(); x != 0 || y != 1 {
		t.Fatalf("cursor after CSI E/F/d = (%d,%d), want (0,1)", x, y)
	}

	terminal.Reset()
	if err := terminal.Write("abcdef\x1b[3D\x1b[X"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "abc ef" {
		t.Fatalf("CSI X line = %q, want abc ef", got)
	}

	terminal.Reset()
	if err := terminal.Write("abcde\x1b[3D\x1b[K"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "ab" {
		t.Fatalf("CSI K erase-to-end line = %q, want ab", got)
	}

	terminal.Reset()
	if err := terminal.Write("abcde\x1b[2D\x1b[1K"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "    e" {
		t.Fatalf("CSI 1K erase-to-start line = %q, want four spaces then e", got)
	}

	terminal.Reset()
	if err := terminal.Write("first\r\nsecond\r\nthird\x1b[2;3H\x1b[J"); err != nil {
		t.Fatal(err)
	}
	viewport = terminal.GetViewport()
	if viewport[0] != "first" || viewport[1] != "se" || viewport[2] != "" {
		t.Fatalf("CSI J erase-to-end viewport = %#v", viewport[:3])
	}

	terminal.Reset()
	if err := terminal.Write("first\r\nsecond\r\nthird\x1b[2;4H\x1b[1J"); err != nil {
		t.Fatal(err)
	}
	viewport = terminal.GetViewport()
	if viewport[0] != "" || viewport[1] != "    nd" || viewport[2] != "third" {
		t.Fatalf("CSI 1J erase-to-start viewport = %#v", viewport[:3])
	}
}

func TestVirtualTerminalCSIColumnOperationsRespectWideCells(t *testing.T) {
	terminal := NewVirtualTerminal(10, 3)
	if err := terminal.Write("界ab\x1b[1;3H\x1b[@"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "界 ab" {
		t.Fatalf("CSI @ after wide cell = %q, want 界 space ab", got)
	}

	terminal.Reset()
	if err := terminal.Write("界abc\x1b[1;3H\x1b[P"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "界bc" {
		t.Fatalf("CSI P after wide cell = %q, want 界bc", got)
	}

	terminal.Reset()
	if err := terminal.Write("界abc\x1b[1;3H\x1b[X"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "界 bc" {
		t.Fatalf("CSI X after wide cell = %q, want 界 space bc", got)
	}

	terminal.Reset()
	if err := terminal.Write("界abc\x1b[1;3H\x1b[K"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "界" {
		t.Fatalf("CSI K after wide cell = %q, want 界", got)
	}

	terminal.Reset()
	if err := terminal.Write("界abc\x1b[1;3H\x1b[1K"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "   bc" {
		t.Fatalf("CSI 1K after wide cell = %q, want three spaces then bc", got)
	}
}

func TestVirtualTerminalClampsExplicitHorizontalCursorMovesLikeXterm(t *testing.T) {
	cases := []struct {
		name string
		data string
		want string
	}{
		{name: "CUF", data: "a\x1b[999CX", want: "a   X"},
		{name: "HPR", data: "a\x1b[999aX", want: "a   X"},
		{name: "CHA", data: "\x1b[999GX", want: "    X"},
		{name: "HPA", data: "\x1b[999`X", want: "    X"},
		{name: "CUP column", data: "\x1b[1;999HX", want: "    X"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			terminal := NewVirtualTerminal(5, 3)
			if err := terminal.Write(tc.data); err != nil {
				t.Fatal(err)
			}
			if got := terminal.GetViewport()[0]; got != tc.want {
				t.Fatalf("%s viewport line = %q, want %q", tc.name, got, tc.want)
			}
			x, y := terminal.GetCursorPosition()
			if x != 5 || y != 0 {
				t.Fatalf("%s cursor = (%d,%d), want after right-margin write at (5,0)", tc.name, x, y)
			}
		})
	}
}

func TestVirtualTerminalDeviceStatusReportsInputResponses(t *testing.T) {
	terminal := NewVirtualTerminal(20, 5)
	var inputs []string
	terminal.Start(func(data string) {
		inputs = append(inputs, data)
	}, func() {})

	if err := terminal.Write("abc\x1b[2;4H\x1b[6n\x1b[5n\x1b[?6n"); err != nil {
		t.Fatal(err)
	}
	want := []string{"\x1b[2;4R", "\x1b[0n", "\x1b[?2;4R"}
	if !equalLines(inputs, want) {
		t.Fatalf("DSR responses = %#v, want %#v", inputs, want)
	}
}

func TestVirtualTerminalDeviceAttributesInputResponses(t *testing.T) {
	terminal := NewVirtualTerminal(20, 5)
	var inputs []string
	terminal.Start(func(data string) {
		inputs = append(inputs, data)
	}, func() {})

	if err := terminal.Write("\x1b[c\x1b[0c\x1b[>c\x1b[>0c\x1b[1c\x1b[>1c"); err != nil {
		t.Fatal(err)
	}
	want := []string{"\x1b[?1;2c", "\x1b[?1;2c", "\x1b[>0;276;0c", "\x1b[>0;276;0c"}
	if !equalLines(inputs, want) {
		t.Fatalf("DA responses = %#v, want %#v", inputs, want)
	}
}

func TestVirtualTerminalCSIRepeatPrecedingCharacter(t *testing.T) {
	terminal := NewVirtualTerminal(20, 4)
	if err := terminal.Write("A\x1b[4b"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "AAAAA" {
		t.Fatalf("CSI b repeated ASCII line = %q, want AAAAA", got)
	}
	if x, y := terminal.GetCursorPosition(); x != 5 || y != 0 {
		t.Fatalf("cursor after ASCII CSI b = (%d,%d), want (5,0)", x, y)
	}

	terminal.Reset()
	if err := terminal.Write("你\x1b[2b"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "你你你" {
		t.Fatalf("CSI b repeated wide-rune line = %q, want three CJK chars", got)
	}
	if x, y := terminal.GetCursorPosition(); x != 6 || y != 0 {
		t.Fatalf("cursor after wide CSI b = (%d,%d), want (6,0)", x, y)
	}
}

func TestVirtualTerminalCSI3JClearsScrollbackOnly(t *testing.T) {
	terminal := NewVirtualTerminal(12, 2)
	if err := terminal.Write("one\r\ntwo\r\nthree"); err != nil {
		t.Fatal(err)
	}
	before := terminal.GetViewport()
	if got, want := before, []string{"two", "three"}; !equalLines(got, want) {
		t.Fatalf("viewport before CSI 3J = %#v, want %#v", got, want)
	}
	if got := terminal.GetScrollBuffer(); len(got) <= 2 {
		t.Fatalf("expected scrollback before CSI 3J, got %#v", got)
	}
	if err := terminal.Write("\x1b[3J"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport(), []string{"two", "three"}; !equalLines(got, want) {
		t.Fatalf("viewport after CSI 3J = %#v, want %#v", got, want)
	}
	if got := terminal.GetScrollBuffer(); !equalLines(got, []string{"two", "three"}) {
		t.Fatalf("scrollback after CSI 3J = %#v, want viewport only", got)
	}
	if x, y := terminal.GetCursorPosition(); x != len("three") || y != 1 {
		t.Fatalf("cursor after CSI 3J = (%d,%d), want (%d,1)", x, y, len("three"))
	}
}

func TestVirtualTerminalTracksCursorVisibilityMode(t *testing.T) {
	terminal := NewVirtualTerminal(8, 4)
	if !terminal.CursorVisible() {
		t.Fatalf("cursor should default visible")
	}
	if err := terminal.HideCursor(); err != nil {
		t.Fatal(err)
	}
	if terminal.CursorVisible() {
		t.Fatalf("HideCursor should update cursor visibility state")
	}
	if err := terminal.ShowCursor(); err != nil {
		t.Fatal(err)
	}
	if !terminal.CursorVisible() {
		t.Fatalf("ShowCursor should update cursor visibility state")
	}
	if err := terminal.Write("\x1b[?25l"); err != nil {
		t.Fatal(err)
	}
	if terminal.CursorVisible() {
		t.Fatalf("CSI ?25l should hide cursor")
	}
	if err := terminal.Write("\x1bc"); err != nil {
		t.Fatal(err)
	}
	if !terminal.CursorVisible() {
		t.Fatalf("RIS should restore cursor visibility")
	}
}

func TestVirtualTerminalConsumesBELTerminatedControlStrings(t *testing.T) {
	terminal := NewVirtualTerminal(20, 4)
	if err := terminal.Write("a\x1b_pi:c\x07b\x1bPignored\x07c\x1b]0;title\x07d\x1b^private\x07e\x1bXsos\x07f"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "abcdef" {
		t.Fatalf("BEL-terminated APC/DCS/OSC/PM/SOS should be consumed, got %q", got)
	}

	terminal.Reset()
	if err := terminal.Write("a\x1b^private\x1b\\b\x1bXsos\x1b\\c"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "abc" {
		t.Fatalf("ST-terminated PM/SOS should be consumed, got %q", got)
	}

	terminal.Reset()
	if err := terminal.Write("a\x1b]0;raw-c1-title\x9cb\x1bPignored\x9cc\x1b_private\xc2\x9cd"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.WindowTitle(); got != "raw-c1-title" {
		t.Fatalf("7-bit OSC with raw C1 ST title = %q, want raw-c1-title", got)
	}
	if got := terminal.GetViewport()[0]; got != "abcd" {
		t.Fatalf("7-bit control strings with C1 ST should be consumed, got %q", got)
	}
}

func TestVirtualTerminalConsumesC1ControlSequences(t *testing.T) {
	terminal := NewVirtualTerminal(20, 4)
	if err := terminal.Write("abc\x9b2DX"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "aXc" {
		t.Fatalf("8-bit CSI cursor movement line = %q, want aXc", got)
	}
	if x, y := terminal.GetCursorPosition(); x != 2 || y != 0 {
		t.Fatalf("cursor after 8-bit CSI write = (%d,%d), want (2,0)", x, y)
	}

	terminal.Reset()
	if err := terminal.Write("a\x9d2;c1-title\x9cb"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.WindowTitle(); got != "c1-title" {
		t.Fatalf("8-bit OSC title = %q, want c1-title", got)
	}
	if got := terminal.GetViewport()[0]; got != "ab" {
		t.Fatalf("8-bit OSC should not render payload, got %q", got)
	}

	terminal.Reset()
	if err := terminal.Write("a\x90dcs\x9cb\x98sos\x9cc\x9eprivate\x9cd\x9fapc\x9ce"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "abcde" {
		t.Fatalf("8-bit string controls should be consumed, got %q", got)
	}

	terminal.Reset()
	if err := terminal.Write("a\xc2\x9b1DX"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "X" {
		t.Fatalf("UTF-8 C1 CSI should be consumed as control, got %q", got)
	}

	terminal.Reset()
	if err := terminal.Write("a\u009d2;utf-title\u009cb"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.WindowTitle(); got != "utf-title" {
		t.Fatalf("UTF-8 C1 OSC title = %q, want utf-title", got)
	}
	if got := terminal.GetViewport()[0]; got != "ab" {
		t.Fatalf("UTF-8 C1 OSC should not render payload, got %q", got)
	}
}

func TestVirtualTerminalCSIInsertMode(t *testing.T) {
	terminal := NewVirtualTerminal(5, 3)
	if err := terminal.Write("abcde\x1b[1;3H\x1b[4hZ"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "abZcd" {
		t.Fatalf("CSI 4h insert-mode line = %q, want abZcd", got)
	}
	if x, y := terminal.GetCursorPosition(); x != 3 || y != 0 {
		t.Fatalf("cursor after CSI 4h insert = (%d,%d), want (3,0)", x, y)
	}

	if err := terminal.Write("\x1b[4lY"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "abZYd" {
		t.Fatalf("CSI 4l replace-mode line = %q, want abZYd", got)
	}
}

func TestVirtualTerminalWritesGraphemeClustersByTerminalColumn(t *testing.T) {
	terminal := NewVirtualTerminal(20, 5)
	if err := terminal.Write("A👍🏽B"); err != nil {
		t.Fatal(err)
	}
	if err := terminal.Write("\x1b[2;1H🇺🇸X"); err != nil {
		t.Fatal(err)
	}
	if err := terminal.Write("\x1b[3;1H👨‍💻Y"); err != nil {
		t.Fatal(err)
	}
	if err := terminal.Write("\x1b[4;1HéZ"); err != nil {
		t.Fatal(err)
	}

	viewport := terminal.GetViewport()
	want := []string{"A👍🏽B", "🇺🇸X", "👨‍💻Y", "éZ"}
	for i, line := range want {
		if viewport[i] != line {
			t.Fatalf("viewport[%d] = %q, want %q (all %#v)", i, viewport[i], line, viewport)
		}
		if got := VisibleWidth(viewport[i]); got != VisibleWidth(line) {
			t.Fatalf("viewport[%d] width = %d, want %d", i, got, VisibleWidth(line))
		}
	}
	x, y := terminal.GetCursorPosition()
	if x != 2 || y != 3 {
		t.Fatalf("cursor = (%d,%d), want (2,3)", x, y)
	}
}

func TestVirtualTerminalOverwritesWideCellsLikeXterm(t *testing.T) {
	terminal := NewVirtualTerminal(8, 4)
	if err := terminal.Write("界B\rA"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "A B" {
		t.Fatalf("narrow over wide-cell head = %q, want A space B", got)
	}

	terminal.Reset()
	if err := terminal.Write("界B\x1b[1;2HX"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != " XB" {
		t.Fatalf("narrow over wide-cell tail = %q, want space X B", got)
	}

	terminal.Reset()
	if err := terminal.Write("abc\r界"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "界c" {
		t.Fatalf("wide over two narrow cells = %q, want 界c", got)
	}

	terminal.Reset()
	if err := terminal.Write("abc\x1b[1;2H界"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "a界" {
		t.Fatalf("wide over two middle narrow cells = %q, want a界", got)
	}
}

func TestVirtualTerminalCombiningEmojiClustersExpandWidthLikeXterm(t *testing.T) {
	terminal := NewVirtualTerminal(6, 2)
	if err := terminal.Write("1\ufe0f\u20e3X"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "1\ufe0f\u20e3X" {
		t.Fatalf("keycap emoji line = %q, want keycap plus trailing X", got)
	}
	if got := VisibleWidth(terminal.GetViewport()[0]); got != 3 {
		t.Fatalf("keycap emoji visible width = %d, want 3", got)
	}
	x, y := terminal.GetCursorPosition()
	if x != 3 || y != 0 {
		t.Fatalf("cursor after keycap emoji = (%d,%d), want (3,0)", x, y)
	}
	cell, ok := terminal.GetCell(0, 2)
	if !ok || cell.Rune != 'X' {
		t.Fatalf("cell after expanded keycap = %#v ok=%v, want X at col 2", cell, ok)
	}
}

func TestVirtualTerminalAutowrapsPrintableWrites(t *testing.T) {
	terminal := NewVirtualTerminal(5, 3)
	if err := terminal.Write("abcdef"); err != nil {
		t.Fatal(err)
	}
	viewport := terminal.GetViewport()
	if got, want := viewport[:3], []string{"abcde", "f", ""}; !equalLines(got, want) {
		t.Fatalf("autowrap viewport = %#v, want %#v", got, want)
	}
	if x, y := terminal.GetCursorPosition(); x != 1 || y != 1 {
		t.Fatalf("cursor after autowrap = (%d,%d), want (1,1)", x, y)
	}
}

func TestVirtualTerminalAutowrapsWideRunesAtRightMargin(t *testing.T) {
	terminal := NewVirtualTerminal(4, 3)
	if err := terminal.Write("abc界z"); err != nil {
		t.Fatal(err)
	}
	viewport := terminal.GetViewport()
	if got, want := viewport[:3], []string{"abc", "界z", ""}; !equalLines(got, want) {
		t.Fatalf("wide autowrap viewport = %#v, want %#v", got, want)
	}
	if x, y := terminal.GetCursorPosition(); x != 3 || y != 1 {
		t.Fatalf("cursor after wide autowrap = (%d,%d), want (3,1)", x, y)
	}
}

func TestVirtualTerminalResizeReflowsSoftWrappedLinesLikeXterm(t *testing.T) {
	terminal := NewVirtualTerminal(5, 4)
	if err := terminal.Write("abcdefghi"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport(), []string{"abcde", "fghi", "", ""}; !equalLines(got, want) {
		t.Fatalf("pre-resize viewport = %#v, want %#v", got, want)
	}

	terminal.Resize(3, 4)

	want := []string{"abc", "def", "ghi", ""}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("narrowed soft-wrap viewport = %#v, want %#v", got, want)
	}
	if x, y := terminal.GetCursorPosition(); x != 3 || y != 2 {
		t.Fatalf("cursor after narrowed soft-wrap resize = (%d,%d), want (3,2)", x, y)
	}

	terminal.Resize(6, 4)

	want = []string{"abcdef", "ghi", "", ""}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("widened soft-wrap viewport = %#v, want %#v", got, want)
	}
	if x, y := terminal.GetCursorPosition(); x != 3 || y != 1 {
		t.Fatalf("cursor after widened soft-wrap resize = (%d,%d), want (3,1)", x, y)
	}
}

func TestVirtualTerminalResizePreservesHardLineBreaksWhenReflowing(t *testing.T) {
	terminal := NewVirtualTerminal(5, 5)
	if err := terminal.Write("abcdef\r\n12"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport(), []string{"abcde", "f", "12", "", ""}; !equalLines(got, want) {
		t.Fatalf("pre-resize hard-break viewport = %#v, want %#v", got, want)
	}

	terminal.Resize(3, 5)

	want := []string{"abc", "def", "12", "", ""}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("hard-break reflow viewport = %#v, want %#v", got, want)
	}
	if x, y := terminal.GetCursorPosition(); x != 2 || y != 2 {
		t.Fatalf("cursor after hard-break resize = (%d,%d), want (2,2)", x, y)
	}
}

func TestVirtualTerminalResizeReflowsWideStyledClustersLikeXterm(t *testing.T) {
	terminal := NewVirtualTerminal(4, 4)
	if err := terminal.Write("\x1b[31m界\x1b[32m好\x1b[34mZ"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport(), []string{"界好", "Z", "", ""}; !equalLines(got, want) {
		t.Fatalf("pre-resize styled wide viewport = %#v, want %#v", got, want)
	}

	terminal.Resize(3, 4)

	want := []string{"界", "好Z", "", ""}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("styled wide reflow viewport = %#v, want %#v", got, want)
	}
	cell, ok := terminal.GetCell(0, 0)
	if !ok || cell.Foreground != (VirtualColor{Kind: "ansi", Index: 1}) {
		t.Fatalf("first wide cell color = %#v ok=%v, want red", cell.Foreground, ok)
	}
	cell, ok = terminal.GetCell(1, 0)
	if !ok || cell.Foreground != (VirtualColor{Kind: "ansi", Index: 2}) {
		t.Fatalf("second wide cell color = %#v ok=%v, want green", cell.Foreground, ok)
	}
	cell, ok = terminal.GetCell(1, 2)
	if !ok || cell.Foreground != (VirtualColor{Kind: "ansi", Index: 4}) {
		t.Fatalf("ASCII tail cell color = %#v ok=%v, want blue", cell.Foreground, ok)
	}
}

func TestVirtualTerminalCursorPositionAndDSRAreViewportRelativeLikeXterm(t *testing.T) {
	terminal := NewVirtualTerminal(12, 2)
	if err := terminal.Write("one\r\ntwo\r\nthree"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport(), []string{"two", "three"}; !equalLines(got, want) {
		t.Fatalf("viewport after scrollback = %#v, want %#v", got, want)
	}
	if x, y := terminal.GetCursorPosition(); x != len("three") || y != 1 {
		t.Fatalf("viewport-relative cursor after scrollback = (%d,%d), want (%d,1)", x, y, len("three"))
	}

	var inputs []string
	terminal.Start(func(data string) {
		inputs = append(inputs, data)
	}, nil)
	if err := terminal.Write("\x1b[6n"); err != nil {
		t.Fatal(err)
	}
	want := []string{"\x1b[2;6R"}
	if !equalLines(inputs, want) {
		t.Fatalf("DSR response after scrollback = %#v, want %#v", inputs, want)
	}
}

func TestVirtualTerminalScreenCursorAddressingIsViewportRelativeLikeXterm(t *testing.T) {
	terminal := NewVirtualTerminal(12, 2)
	if err := terminal.Write("one\r\ntwo\r\nthree\x1b[1;1H!"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport(), []string{"!wo", "three"}; !equalLines(got, want) {
		t.Fatalf("CUP after scrollback viewport = %#v, want %#v", got, want)
	}
	if got := terminal.GetScrollBuffer(); !equalLines(got, []string{"one", "!wo", "three"}) {
		t.Fatalf("CUP should not address hidden scrollback, scroll=%#v", got)
	}

	boundsTerminal := NewVirtualTerminal(5, 3)
	if err := boundsTerminal.Write("\x1b[99;99HZ"); err != nil {
		t.Fatal(err)
	}
	if got, want := boundsTerminal.GetViewport(), []string{"", "", "    Z"}; !equalLines(got[:3], want) {
		t.Fatalf("out-of-range CUP viewport = %#v, want %#v", got[:3], want)
	}
	if got := len(boundsTerminal.GetScrollBuffer()); got != 3 {
		t.Fatalf("out-of-range CUP should not create scrollback, len=%d", got)
	}

	terminal.Reset()
	if err := terminal.Write("one\r\ntwo\r\nthree\x1b[1d!"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport(), []string{"two  !", "three"}; !equalLines(got, want) {
		t.Fatalf("VPA after scrollback viewport = %#v, want %#v", got, want)
	}
}

func TestVirtualTerminalViewportRelativeEditAndScrollAfterScrollback(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		want   []string
		scroll []string
	}{
		{
			name:   "insert line",
			input:  "one\r\ntwo\r\nthree\r\nfour\x1b[2;1H\x1b[L",
			want:   []string{"two", "", "three"},
			scroll: []string{"one", "two", "", "three"},
		},
		{
			name:   "delete line",
			input:  "one\r\ntwo\r\nthree\r\nfour\x1b[2;1H\x1b[M",
			want:   []string{"two", "four", ""},
			scroll: []string{"one", "two", "four", ""},
		},
		{
			name:   "scroll up",
			input:  "one\r\ntwo\r\nthree\r\nfour\x1b[S",
			want:   []string{"three", "four", ""},
			scroll: []string{"one", "three", "four", ""},
		},
		{
			name:   "scroll down",
			input:  "one\r\ntwo\r\nthree\r\nfour\x1b[T",
			want:   []string{"", "two", "three"},
			scroll: []string{"one", "", "two", "three"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			terminal := NewVirtualTerminal(12, 3)
			if err := terminal.Write(tc.input); err != nil {
				t.Fatal(err)
			}
			if got := terminal.GetViewport(); !equalLines(got, tc.want) {
				t.Fatalf("viewport = %#v, want %#v", got, tc.want)
			}
			if got := terminal.GetScrollBuffer(); !equalLines(got, tc.scroll) {
				t.Fatalf("scrollback = %#v, want %#v", got, tc.scroll)
			}
		})
	}
}

func TestVirtualTerminalWraparoundModeCanBeDisabled(t *testing.T) {
	terminal := NewVirtualTerminal(5, 3)
	if err := terminal.Write("\x1b[?7labcdef"); err != nil {
		t.Fatal(err)
	}
	viewport := terminal.GetViewport()
	if got, want := viewport[:3], []string{"abcdf", "", ""}; !equalLines(got, want) {
		t.Fatalf("wraparound disabled viewport = %#v, want %#v", got, want)
	}
	if x, y := terminal.GetCursorPosition(); x != 4 || y != 0 {
		t.Fatalf("cursor with wraparound disabled = (%d,%d), want (4,0)", x, y)
	}

	if err := terminal.Write("\x1b[?7hZ"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport()[:3], []string{"abcdf", "Z", ""}; !equalLines(got, want) {
		t.Fatalf("re-enabled wraparound should honor pending right-margin state = %#v, want %#v", got, want)
	}
	if x, y := terminal.GetCursorPosition(); x != 1 || y != 1 {
		t.Fatalf("cursor after re-enabled wraparound = (%d,%d), want (1,1)", x, y)
	}

	if err := terminal.Write("\x1bcabcdeZ"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport()[:3], []string{"abcde", "Z", ""}; !equalLines(got, want) {
		t.Fatalf("RIS should restore wraparound viewport = %#v, want %#v", got, want)
	}
}

func TestVirtualTerminalBackspaceAndTabControlChars(t *testing.T) {
	terminal := NewVirtualTerminal(20, 5)
	if err := terminal.Write("abc\bZ\rA\tB"); err != nil {
		t.Fatal(err)
	}
	viewport := terminal.GetViewport()
	if got := viewport[0]; got != "AbZ     B" {
		t.Fatalf("BS/TAB line = %q, want AbZ five spaces B", got)
	}
	if x, y := terminal.GetCursorPosition(); x != 9 || y != 0 {
		t.Fatalf("cursor after BS/TAB = (%d,%d), want (9,0)", x, y)
	}

	terminal = NewVirtualTerminal(5, 3)
	if err := terminal.Write("abcde\bX"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport()[0], "abcXe"; got != want {
		t.Fatalf("BS after right-margin pending wrap = %q, want %q", got, want)
	}
	if x, y := terminal.GetCursorPosition(); x != 4 || y != 0 {
		t.Fatalf("cursor after BS at right margin = (%d,%d), want (4,0)", x, y)
	}

	terminal = NewVirtualTerminal(8, 3)
	if err := terminal.Write("ab\tc\bD"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport()[0], "ab    Dc"; got != want {
		t.Fatalf("TAB then BS at right margin = %q, want %q", got, want)
	}
	if x, y := terminal.GetCursorPosition(); x != 7 || y != 0 {
		t.Fatalf("cursor after TAB/BS at right margin = (%d,%d), want (7,0)", x, y)
	}
}

func TestVirtualTerminalLineFeedControlsPreserveColumnLikeXterm(t *testing.T) {
	terminal := NewVirtualTerminal(10, 4)
	if err := terminal.Write("abc\nX"); err != nil {
		t.Fatal(err)
	}
	viewport := terminal.GetViewport()
	if viewport[0] != "abc" || viewport[1] != "   X" {
		t.Fatalf("LF viewport = %#v, want abc then X at preserved column", viewport[:2])
	}
	if x, y := terminal.GetCursorPosition(); x != 4 || y != 1 {
		t.Fatalf("cursor after LF = (%d,%d), want (4,1)", x, y)
	}

	terminal = NewVirtualTerminal(6, 4)
	if err := terminal.Write("111111\n222222"); err != nil {
		t.Fatal(err)
	}
	viewport = terminal.GetViewport()
	want := []string{"111111", "     2", "22222", ""}
	if got := viewport[:4]; !equalLines(got, want) {
		t.Fatalf("LF after right-margin pending wrap viewport = %#v, want %#v", got, want)
	}
	if x, y := terminal.GetCursorPosition(); x != 5 || y != 2 {
		t.Fatalf("LF after right-margin cursor = (%d,%d), want (5,2)", x, y)
	}

	terminal.Reset()
	if err := terminal.Write("abc\vX\fY"); err != nil {
		t.Fatal(err)
	}
	viewport = terminal.GetViewport()
	if viewport[0] != "abc" || viewport[1] != "   X" || viewport[2] != "    Y" {
		t.Fatalf("VT/FF viewport = %#v, want preserved-column line feeds", viewport[:3])
	}
	if x, y := terminal.GetCursorPosition(); x != 5 || y != 2 {
		t.Fatalf("cursor after VT/FF = (%d,%d), want (5,2)", x, y)
	}
}

func TestVirtualTerminalTabStops(t *testing.T) {
	terminal := NewVirtualTerminal(20, 5)
	if err := terminal.Write("abc\x1bH\rX\tY"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "XbcY" {
		t.Fatalf("ESC H tab stop line = %q, want XbcY", got)
	}

	terminal.Reset()
	if err := terminal.Write("\x1b[3gX\tY"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "X                  Y" {
		t.Fatalf("CSI 3g cleared tab stops line = %q, want X spaces Y at right edge", got)
	}

	terminal.Reset()
	if err := terminal.Write("\x1b[9G\x1b[g\x1b[1GX\tY"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "X               Y" {
		t.Fatalf("CSI g cleared current default tab stop line = %q, want Y at next default stop", got)
	}

	terminal.Reset()
	if err := terminal.Write("\x1b[3g\x1b[5G\x1bH\x1b[1GX\tY"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "X   Y" {
		t.Fatalf("ESC H after CSI 3g line = %q, want explicit tab stop after cleared defaults", got)
	}

	terminal.Reset()
	if err := terminal.Write("\x1b[9G\x1b[g\x1bcX\tY"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "X       Y" {
		t.Fatalf("RIS should restore default tab stops, got %q", got)
	}

	terminal.Reset()
	if err := terminal.Write("A\x1b[2IB\x1b[2ZC"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "A       C       B" {
		t.Fatalf("CSI I/Z tab movement line = %q, want C at previous tab stop", got)
	}
	if x, y := terminal.GetCursorPosition(); x != 9 || y != 0 {
		t.Fatalf("cursor after CSI I/Z = (%d,%d), want (9,0)", x, y)
	}
}

func TestVirtualTerminalScrollRegionIndex(t *testing.T) {
	terminal := NewVirtualTerminal(8, 4)
	if err := terminal.Write("\x1b[1;1Haaaa\x1b[2;1Hbbbb\x1b[3;1Hcccc\x1b[4;1Hdddd"); err != nil {
		t.Fatal(err)
	}
	if err := terminal.Write("\x1b[2;3r\x1b[3;1H\n"); err != nil {
		t.Fatal(err)
	}
	want := []string{"aaaa", "cccc", "", "dddd"}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("scroll-region index viewport = %#v, want %#v", got, want)
	}
	if x, y := terminal.GetCursorPosition(); x != 0 || y != 2 {
		t.Fatalf("cursor after scroll-region index = (%d,%d), want (0,2)", x, y)
	}
}

func TestVirtualTerminalScrollRegionReverseIndex(t *testing.T) {
	terminal := NewVirtualTerminal(8, 4)
	if err := terminal.Write("\x1b[1;1Haaaa\x1b[2;1Hbbbb\x1b[3;1Hcccc\x1b[4;1Hdddd"); err != nil {
		t.Fatal(err)
	}
	if err := terminal.Write("\x1b[2;3r\x1b[2;1H\x1bM"); err != nil {
		t.Fatal(err)
	}
	want := []string{"aaaa", "", "bbbb", "dddd"}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("scroll-region reverse-index viewport = %#v, want %#v", got, want)
	}
	if x, y := terminal.GetCursorPosition(); x != 0 || y != 1 {
		t.Fatalf("cursor after scroll-region reverse-index = (%d,%d), want (0,1)", x, y)
	}
}

func TestVirtualTerminalScrollRegionZeroParamsUseDefaultsLikeXterm(t *testing.T) {
	t.Run("zero zero resets to full screen", func(t *testing.T) {
		terminal := NewVirtualTerminal(8, 4)
		if err := terminal.Write("\x1b[2;3r\x1b[0;0rA"); err != nil {
			t.Fatal(err)
		}
		if got, want := terminal.GetViewport(), []string{"A", "", "", ""}; !equalLines(got, want) {
			t.Fatalf("viewport after CSI 0;0r = %#v, want %#v", got, want)
		}
		if x, y := terminal.GetCursorPosition(); x != 1 || y != 0 {
			t.Fatalf("cursor after CSI 0;0r write = (%d,%d), want (1,0)", x, y)
		}
	})

	t.Run("zero top defaults to first row", func(t *testing.T) {
		terminal := NewVirtualTerminal(8, 4)
		if err := terminal.Write("111\r\n222\r\n333\r\n444\x1b[0;3r\x1b[3;1H\x1bD"); err != nil {
			t.Fatal(err)
		}
		if got, want := terminal.GetViewport(), []string{"222", "333", "", "444"}; !equalLines(got, want) {
			t.Fatalf("viewport after CSI 0;3r IND = %#v, want %#v", got, want)
		}
	})

	t.Run("zero bottom defaults to last row", func(t *testing.T) {
		terminal := NewVirtualTerminal(8, 4)
		if err := terminal.Write("111\r\n222\r\n333\r\n444\x1b[2;0r\x1b[4;1H\x1bD"); err != nil {
			t.Fatal(err)
		}
		if got, want := terminal.GetViewport(), []string{"111", "333", "444", ""}; !equalLines(got, want) {
			t.Fatalf("viewport after CSI 2;0r IND = %#v, want %#v", got, want)
		}
	})
}

func TestVirtualTerminalSoftResetMatchesXtermDECSTR(t *testing.T) {
	t.Run("resets modes and style without clearing screen or moving active cursor", func(t *testing.T) {
		terminal := NewVirtualTerminal(8, 4)
		if err := terminal.Write("keep\x1b[3;4H\x1b[31m\x1b[4h\x1b[?7l\x1b[?25l\x1b[!p"); err != nil {
			t.Fatal(err)
		}
		if x, y := terminal.GetCursorPosition(); x != 3 || y != 2 {
			t.Fatalf("cursor after DECSTR = (%d,%d), want (3,2)", x, y)
		}
		if !terminal.CursorVisible() {
			t.Fatalf("DECSTR should restore cursor visibility")
		}
		if got, want := terminal.GetViewport()[0], "keep"; got != want {
			t.Fatalf("DECSTR should not clear existing text, row0 = %q want %q", got, want)
		}
		if err := terminal.Write("Z"); err != nil {
			t.Fatal(err)
		}
		cell, ok := terminal.GetCell(2, 3)
		if !ok || cell.Rune != 'Z' {
			t.Fatalf("cell after DECSTR write = %#v ok=%v, want Z", cell, ok)
		}
		if cell.Foreground.Kind != "" || cell.Bold || cell.Italic || cell.Underline || cell.Strikethrough {
			t.Fatalf("DECSTR should reset SGR style before write, cell = %#v", cell)
		}
	})

	t.Run("resets scroll region origin mode and saved cursor state", func(t *testing.T) {
		terminal := NewVirtualTerminal(8, 4)
		if err := terminal.Write("\x1b[2;3r\x1b[?6h\x1b[H"); err != nil {
			t.Fatal(err)
		}
		if x, y := terminal.GetCursorPosition(); x != 0 || y != 1 {
			t.Fatalf("cursor in origin mode before DECSTR = (%d,%d), want (0,1)", x, y)
		}
		if err := terminal.Write("\x1b[3;5H\x1b7\x1b[4;6H"); err != nil {
			t.Fatal(err)
		}
		beforeX, beforeY := terminal.GetCursorPosition()
		if err := terminal.Write("\x1b[!p"); err != nil {
			t.Fatal(err)
		}
		if x, y := terminal.GetCursorPosition(); x != beforeX || y != beforeY {
			t.Fatalf("active cursor after DECSTR = (%d,%d), want unchanged (%d,%d)", x, y, beforeX, beforeY)
		}
		if err := terminal.Write("\x1b[H"); err != nil {
			t.Fatal(err)
		}
		if x, y := terminal.GetCursorPosition(); x != 0 || y != 0 {
			t.Fatalf("home after DECSTR should use full-screen origin, got (%d,%d)", x, y)
		}
		if err := terminal.Write("\x1b[4;6H\x1b[!p\x1b8"); err != nil {
			t.Fatal(err)
		}
		if x, y := terminal.GetCursorPosition(); x != 0 || y != 0 {
			t.Fatalf("DECRC after DECSTR should restore reset saved cursor, got (%d,%d)", x, y)
		}
	})
}

func TestVirtualTerminalScrollRegionIsViewportRelativeAfterScrollback(t *testing.T) {
	terminal := NewVirtualTerminal(8, 3)
	if err := terminal.Write("one\r\ntwo\r\nthree\r\nfour\x1b[2;3r\x1b[3;1H\x1bD"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport(), []string{"two", "four", ""}; !equalLines(got, want) {
		t.Fatalf("scroll-region IND after scrollback viewport = %#v, want %#v", got, want)
	}
	if got, want := terminal.GetScrollBuffer(), []string{"one", "two", "four", ""}; !equalLines(got, want) {
		t.Fatalf("scroll-region IND should preserve hidden scrollback, got %#v want %#v", got, want)
	}

	terminal.Reset()
	if err := terminal.Write("one\r\ntwo\r\nthree\r\nfour\x1b[2;3r\x1b[2;1H\x1bM"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport(), []string{"two", "", "three"}; !equalLines(got, want) {
		t.Fatalf("scroll-region RI after scrollback viewport = %#v, want %#v", got, want)
	}
	if got, want := terminal.GetScrollBuffer(), []string{"one", "two", "", "three"}; !equalLines(got, want) {
		t.Fatalf("scroll-region RI should preserve hidden scrollback, got %#v want %#v", got, want)
	}
}

func TestVirtualTerminalResizePreservesViewportRelativeScrollRegion(t *testing.T) {
	terminal := NewVirtualTerminal(8, 3)
	if err := terminal.Write("one\r\ntwo\r\nthree\r\nfour\x1b[2;3r"); err != nil {
		t.Fatal(err)
	}
	terminal.Resize(8, 3)
	if err := terminal.Write("\x1b[3;1H\x1bD"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport(), []string{"two", "four", ""}; !equalLines(got, want) {
		t.Fatalf("scroll-region after same-size resize viewport = %#v, want %#v", got, want)
	}
	if got, want := terminal.GetScrollBuffer(), []string{"one", "two", "four", ""}; !equalLines(got, want) {
		t.Fatalf("scroll-region after same-size resize should preserve hidden scrollback, got %#v want %#v", got, want)
	}

	terminal.Reset()
	if err := terminal.Write("one\r\ntwo\r\nthree\r\nfour\x1b[2;3r"); err != nil {
		t.Fatal(err)
	}
	terminal.Resize(8, 4)
	if err := terminal.Write("\x1b[3;1H\x1bD"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport(), []string{"one", "three", "", "four"}; !equalLines(got, want) {
		t.Fatalf("scroll-region after taller resize viewport = %#v, want %#v", got, want)
	}
}

func TestVirtualTerminalOriginModeAddressesWithinScrollRegion(t *testing.T) {
	terminal := NewVirtualTerminal(8, 4)
	if err := terminal.Write("\x1b[1;1H111\x1b[2;1H222\x1b[3;1H333\x1b[4;1H444"); err != nil {
		t.Fatal(err)
	}
	if err := terminal.Write("\x1b[2;4r\x1b[?6h\x1b[1;1HA\x1b[2;1HB"); err != nil {
		t.Fatal(err)
	}
	want := []string{"111", "A22", "B33", "444"}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("origin-mode viewport = %#v, want %#v", got, want)
	}
	if x, y := terminal.GetCursorPosition(); x != 1 || y != 2 {
		t.Fatalf("cursor in origin mode = (%d,%d), want (1,2)", x, y)
	}

	if err := terminal.Write("\x1b[?6l\x1b[1;1HC"); err != nil {
		t.Fatal(err)
	}
	want = []string{"C11", "A22", "B33", "444"}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("origin-mode reset viewport = %#v, want %#v", got, want)
	}
}

func TestVirtualTerminalOriginModeClampsVerticalCursorMovementToScrollRegion(t *testing.T) {
	terminal := NewVirtualTerminal(8, 5)
	if err := terminal.Write("\x1b[1;1H111\x1b[2;1H222\x1b[3;1H333\x1b[4;1H444\x1b[5;1H555"); err != nil {
		t.Fatal(err)
	}
	if err := terminal.Write("\x1b[2;4r\x1b[?6h\x1b[1;1HT\x1b[10AA\x1b[10BB\x1b[10FF\x1b[10EE\x1b[1;1H\x1b[10eV\x1b[99dD"); err != nil {
		t.Fatal(err)
	}
	want := []string{"111", "FA2", "333", "VDB", "555"}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("origin-mode vertical movement viewport = %#v, want %#v", got, want)
	}
	if x, y := terminal.GetCursorPosition(); x != 2 || y != 3 {
		t.Fatalf("origin-mode vertical movement cursor = (%d,%d), want (2,3)", x, y)
	}
}

func TestVirtualTerminalCSIExplicitZeroMovementDefaultsToOneLikeXterm(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
		x     int
		y     int
	}{
		{
			name:  "cursor up zero",
			input: "\x1b[2;2H\x1b[0AZ",
			want:  []string{" Z", "", "", "", ""},
			x:     2,
			y:     0,
		},
		{
			name:  "cursor down zero",
			input: "\x1b[2;2H\x1b[0BZ",
			want:  []string{"", "", " Z", "", ""},
			x:     2,
			y:     2,
		},
		{
			name:  "cursor forward zero",
			input: "\x1b[2;2H\x1b[0CZ",
			want:  []string{"", "  Z", "", "", ""},
			x:     3,
			y:     1,
		},
		{
			name:  "horizontal relative zero",
			input: "\x1b[2;2H\x1b[0aZ",
			want:  []string{"", "  Z", "", "", ""},
			x:     3,
			y:     1,
		},
		{
			name:  "cursor backward zero",
			input: "\x1b[2;2H\x1b[0DZ",
			want:  []string{"", "Z", "", "", ""},
			x:     1,
			y:     1,
		},
		{
			name:  "next line zero",
			input: "\x1b[2;2H\x1b[0EZ",
			want:  []string{"", "", "Z", "", ""},
			x:     1,
			y:     2,
		},
		{
			name:  "vertical relative zero",
			input: "\x1b[2;2H\x1b[0eZ",
			want:  []string{"", "", " Z", "", ""},
			x:     2,
			y:     2,
		},
		{
			name:  "previous line zero",
			input: "\x1b[2;2H\x1b[0FZ",
			want:  []string{"Z", "", "", "", ""},
			x:     1,
			y:     0,
		},
		{
			name:  "forward tab zero",
			input: "\x1b[2;2H\x1b[0IZ",
			want:  []string{"", "        Z", "", "", ""},
			x:     9,
			y:     1,
		},
		{
			name:  "backward tab zero",
			input: "\x1b[2;10H\x1b[0ZZ",
			want:  []string{"", "        Z", "", "", ""},
			x:     9,
			y:     1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			terminal := NewVirtualTerminal(12, 5)
			if err := terminal.Write(tc.input); err != nil {
				t.Fatal(err)
			}
			if got := terminal.GetViewport(); !equalLines(got, tc.want) {
				t.Fatalf("viewport = %#v, want %#v", got, tc.want)
			}
			if x, y := terminal.GetCursorPosition(); x != tc.x || y != tc.y {
				t.Fatalf("cursor = (%d,%d), want (%d,%d)", x, y, tc.x, tc.y)
			}
		})
	}
}

func TestVirtualTerminalCSIExplicitZeroEditCountsDefaultToOneLikeXterm(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "insert blank character zero",
			input: "abcd\x1b[2G\x1b[0@Z",
			want:  []string{"aZbcd", "", ""},
		},
		{
			name:  "delete character zero",
			input: "abcd\x1b[2G\x1b[0P",
			want:  []string{"acd", "", ""},
		},
		{
			name:  "erase character zero",
			input: "abcd\x1b[2G\x1b[0X",
			want:  []string{"a cd", "", ""},
		},
		{
			name:  "insert line zero",
			input: "111\x1b[2;1H222\x1b[3;1H333\x1b[2;1H\x1b[0L",
			want:  []string{"111", "", "222"},
		},
		{
			name:  "delete line zero",
			input: "111\x1b[2;1H222\x1b[3;1H333\x1b[2;1H\x1b[0M",
			want:  []string{"111", "333", ""},
		},
		{
			name:  "scroll up zero",
			input: "111\x1b[2;1H222\x1b[3;1H333\x1b[0S",
			want:  []string{"222", "333", ""},
		},
		{
			name:  "scroll down zero",
			input: "111\x1b[2;1H222\x1b[3;1H333\x1b[0T",
			want:  []string{"", "111", "222"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			terminal := NewVirtualTerminal(8, 3)
			if err := terminal.Write(tc.input); err != nil {
				t.Fatal(err)
			}
			if got := terminal.GetViewport(); !equalLines(got, tc.want) {
				t.Fatalf("viewport = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestVirtualTerminalAlternateScreenBufferRestoresNormalScreen(t *testing.T) {
	terminal := NewVirtualTerminal(8, 4)
	if err := terminal.Write("normal\x1b[2;3H!"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[:2]; !equalLines(got, []string{"normal", "  !"}) {
		t.Fatalf("normal viewport before alternate = %#v", got)
	}
	if x, y := terminal.GetCursorPosition(); x != 3 || y != 1 {
		t.Fatalf("normal cursor before alternate = (%d,%d), want (3,1)", x, y)
	}

	if err := terminal.Write("\x1b[?1049hALT\x1b[2;1HBUF"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[:3]; !equalLines(got, []string{"ALT", "BUF", ""}) {
		t.Fatalf("alternate viewport = %#v", got)
	}

	if err := terminal.Write("\x1b[?1049l"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[:2]; !equalLines(got, []string{"normal", "  !"}) {
		t.Fatalf("restored normal viewport = %#v", got)
	}
	if x, y := terminal.GetCursorPosition(); x != 3 || y != 1 {
		t.Fatalf("restored normal cursor = (%d,%d), want (3,1)", x, y)
	}
}

func TestVirtualTerminalAlternateScreenKeepsCurrentRendition(t *testing.T) {
	terminal := NewVirtualTerminal(8, 4)
	if err := terminal.Write("\x1b[31mN\x1b[?1049hA"); err != nil {
		t.Fatal(err)
	}
	cell, ok := terminal.GetCell(0, 0)
	if !ok || cell.Rune != 'A' || cell.Foreground != (VirtualColor{Kind: "ansi", Index: 1}) {
		t.Fatalf("alternate-screen cell should keep current rendition, got %#v", cell)
	}

	if err := terminal.Write("\x1b[32mB\x1b[?1049lC"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "NC" {
		t.Fatalf("normal screen after alternate = %q, want NC", got)
	}
	cell, ok = terminal.GetCell(0, 1)
	if !ok || cell.Rune != 'C' || cell.Foreground != (VirtualColor{Kind: "ansi", Index: 1}) {
		t.Fatalf("normal-screen restored cursor rendition should be red, got %#v", cell)
	}
}

func TestVirtualTerminalDECPrivateCursorSaveRestore(t *testing.T) {
	terminal := NewVirtualTerminal(10, 4)
	if err := terminal.Write("abc\x1b[?1048h\x1b[3;5HZZ\x1b[?1048lX"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[:3]; !equalLines(got, []string{"abcX", "", "    ZZ"}) {
		t.Fatalf("DEC ?1048 cursor save/restore viewport = %#v", got)
	}
	if x, y := terminal.GetCursorPosition(); x != 4 || y != 0 {
		t.Fatalf("cursor after DEC ?1048 restore and write = (%d,%d), want (4,0)", x, y)
	}
}

func TestVirtualTerminalAlternateScreenLegacyModes(t *testing.T) {
	for _, mode := range []string{"47", "1047"} {
		t.Run(mode, func(t *testing.T) {
			terminal := NewVirtualTerminal(8, 4)
			if err := terminal.Write("main"); err != nil {
				t.Fatal(err)
			}
			if err := terminal.Write("\x1b[?" + mode + "halt\x1b[?" + mode + "l"); err != nil {
				t.Fatal(err)
			}
			if got := terminal.GetViewport()[0]; got != "main" {
				t.Fatalf("mode ?%s restored normal screen = %q, want main", mode, got)
			}
		})
	}
}

func TestVirtualTerminalESCIndexNextLineAndReverseIndex(t *testing.T) {
	terminal := NewVirtualTerminal(20, 5)
	if err := terminal.Write("A\x1bDB\x1bEC\x1bMZ"); err != nil {
		t.Fatal(err)
	}
	viewport := terminal.GetViewport()
	want := []string{"A", " Z", "C"}
	for i, line := range want {
		if viewport[i] != line {
			t.Fatalf("viewport[%d] = %q, want %q (all %#v)", i, viewport[i], line, viewport)
		}
	}
	if x, y := terminal.GetCursorPosition(); x != 2 || y != 1 {
		t.Fatalf("cursor after ESC D/E/M = (%d,%d), want (2,1)", x, y)
	}
}

func TestVirtualTerminalISO2022CharsetAndDECAlignment(t *testing.T) {
	terminal := NewVirtualTerminal(12, 3)
	if err := terminal.Write("a\x1b(Bb\x1b(0lqk\x1b(BX"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "ab┌─┐X" {
		t.Fatalf("G0 charset designation viewport = %q, want ab┌─┐X", got)
	}

	terminal.Reset()
	if err := terminal.Write("\x1b)0\x0elq\x0fA"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "┌─A" {
		t.Fatalf("G1 charset shift viewport = %q, want ┌─A", got)
	}

	terminal.Reset()
	if err := terminal.Write("A\x1b%GB\x1b%@C"); err != nil {
		t.Fatal(err)
	}
	if got := terminal.GetViewport()[0]; got != "ABC" {
		t.Fatalf("ISO-2022 UTF-8 designation should be consumed, got %q", got)
	}

	terminal.Reset()
	if err := terminal.Write("\x1b[31mZ\x1b#8"); err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("E", 12)
	for row, line := range terminal.GetViewport() {
		if line != want {
			t.Fatalf("DECALN row %d = %q, want %q", row, line, want)
		}
	}
	cell, ok := terminal.GetCell(0, 0)
	if !ok || cell.Rune != 'E' || cell.Foreground.Kind != "ansi" || cell.Foreground.Index != 1 {
		t.Fatalf("DECALN should fill styled E cells, got %+v ok=%v", cell, ok)
	}
}

func TestVirtualTerminalRISResetsState(t *testing.T) {
	terminal := NewVirtualTerminal(20, 5)
	if err := terminal.Write("\x1b[31;4mA\x1b[2;4HB\x1bcC"); err != nil {
		t.Fatal(err)
	}
	viewport := terminal.GetViewport()
	if viewport[0] != "C" || viewport[1] != "" {
		t.Fatalf("RIS viewport = %#v, want reset screen with C at origin", viewport[:2])
	}
	if x, y := terminal.GetCursorPosition(); x != 1 || y != 0 {
		t.Fatalf("cursor after RIS = (%d,%d), want (1,0)", x, y)
	}
	cell, ok := terminal.GetCell(0, 0)
	if !ok {
		t.Fatalf("missing cell after RIS")
	}
	if cell.Foreground.Kind != "" || cell.Underline || cell.UnderlineStyle != "" || cell.Bold || cell.Dim || cell.Italic || cell.Inverse || cell.Strikethrough || cell.Blink || cell.Conceal || cell.Overline {
		t.Fatalf("RIS should reset cell style, got %#v", cell)
	}
	if !strings.Contains(terminal.Output(), "\x1bc") {
		t.Fatalf("output log should retain RIS sequence, got %q", terminal.Output())
	}
}

func TestTUIOverlayPiOptionsLayout(t *testing.T) {
	terminal := NewVirtualTerminal(100, 24)
	ui := NewTUI(terminal)
	ui.AddChild(&lineComponent{lines: []string{""}})

	defaultWidth := &widthRecorderComponent{lines: []string{"default"}}
	handle := ui.ShowOverlay(defaultWidth)
	if defaultWidth.requestedWidth != 80 {
		t.Fatalf("default overlay width = %d, want 80", defaultWidth.requestedWidth)
	}
	handle.Hide()

	percentWidth := &widthRecorderComponent{lines: []string{"percent"}}
	ui.ShowOverlay(percentWidth, OverlayOptions{Width: ptrPercent(10), MinWidth: 30})
	if percentWidth.requestedWidth != 30 {
		t.Fatalf("percent/min overlay width = %d, want 30", percentWidth.requestedWidth)
	}
}

func TestTUIOverlayPiDecimalPercentSizeValues(t *testing.T) {
	t.Run("width floors decimal percentage of terminal width", func(t *testing.T) {
		terminal := NewVirtualTerminal(80, 24)
		ui := NewTUI(terminal)
		ui.AddChild(&lineComponent{lines: []string{""}})
		overlay := &widthRecorderComponent{lines: []string{"decimal"}}
		ui.ShowOverlay(overlay, OverlayOptions{Width: ptrPercentFloat(12.5)})
		if overlay.requestedWidth != 10 {
			t.Fatalf("decimal percent width = %d, want 10", overlay.requestedWidth)
		}
	})

	t.Run("maxHeight floors decimal percentage of terminal height", func(t *testing.T) {
		terminal := NewVirtualTerminal(80, 16)
		ui := NewTUI(terminal)
		ui.AddChild(&lineComponent{lines: []string{""}})
		ui.ShowOverlay(&lineComponent{lines: []string{"L1", "L2", "L3", "L4"}}, OverlayOptions{
			Anchor:    OverlayTopLeft,
			MaxHeight: ptrPercentFloat(12.5),
		})
		ui.RequestRender(true)
		content := strings.Join(terminal.GetViewport(), "\n")
		if !strings.Contains(content, "L1") || !strings.Contains(content, "L2") || strings.Contains(content, "L3") {
			t.Fatalf("decimal maxHeight percent clipping mismatch: %#v", terminal.GetViewport())
		}
	})

	t.Run("row and col floor decimal percentage of available movement range", func(t *testing.T) {
		terminal := NewVirtualTerminal(80, 24)
		ui := NewTUI(terminal)
		ui.AddChild(&lineComponent{lines: []string{""}})
		ui.ShowOverlay(&lineComponent{lines: []string{"DEC"}}, OverlayOptions{
			Width: ptr(10),
			Row:   ptrPercentFloat(12.5),
			Col:   ptrPercentFloat(12.5),
		})
		ui.RequestRender(true)
		assertViewportContainsAt(t, terminal.GetViewport(), 2, 8, "DEC")
	})
}

func TestTUIOverlayPiMarginMaxHeightAndWidthClipping(t *testing.T) {
	terminal := NewVirtualTerminal(20, 8)
	ui := NewTUI(terminal)
	ui.AddChild(&lineComponent{lines: []string{strings.Repeat("X", 20)}})
	ui.ShowOverlay(&lineComponent{lines: []string{"A"}}, OverlayOptions{
		Anchor: OverlayTopLeft,
		Width:  ptr(3),
		Margin: OverlayMargin{Top: -5, Left: -10},
	})
	ui.RequestRender(true)
	if got := terminal.GetViewport()[0]; !strings.HasPrefix(got, "A  XXX") {
		t.Fatalf("overlay should clamp negative margin and cover declared width, got %q", got)
	}

	terminal.Reset()
	ui = NewTUI(terminal)
	ui.AddChild(&lineComponent{lines: []string{""}})
	ui.ShowOverlay(&lineComponent{lines: []string{"L1", "L2", "L3", "L4", "L5"}}, OverlayOptions{
		Anchor:    OverlayTopLeft,
		MaxHeight: ptr(3),
	})
	ui.RequestRender(true)
	content := strings.Join(terminal.GetViewport(), "\n")
	if !strings.Contains(content, "L1") || !strings.Contains(content, "L3") || strings.Contains(content, "L4") {
		t.Fatalf("maxHeight clipping mismatch: %q", content)
	}
}

func TestTUIOverlayPiOptionsPositioningMatrix(t *testing.T) {
	t.Run("anchors", func(t *testing.T) {
		cases := []struct {
			name   string
			anchor OverlayAnchor
			row    int
			col    int
		}{
			{name: "top-left", anchor: OverlayTopLeft, row: 0, col: 0},
			{name: "top-center", anchor: OverlayTopCenter, row: 0, col: 35},
			{name: "top-right", anchor: OverlayTopRight, row: 0, col: 70},
			{name: "left-center", anchor: OverlayLeftCenter, row: 11, col: 0},
			{name: "center", anchor: OverlayCenter, row: 11, col: 35},
			{name: "right-center", anchor: OverlayRightCenter, row: 11, col: 70},
			{name: "bottom-left", anchor: OverlayBottomLeft, row: 23, col: 0},
			{name: "bottom-center", anchor: OverlayBottomCenter, row: 23, col: 35},
			{name: "bottom-right", anchor: OverlayBottomRight, row: 23, col: 70},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				terminal := NewVirtualTerminal(80, 24)
				ui := NewTUI(terminal)
				ui.AddChild(&lineComponent{lines: []string{""}})
				ui.ShowOverlay(&lineComponent{lines: []string{"MARK"}}, OverlayOptions{Anchor: tc.anchor, Width: ptr(10)})
				ui.RequestRender(true)
				assertViewportContainsAt(t, terminal.GetViewport(), tc.row, tc.col, "MARK")
			})
		}
	})

	t.Run("margin offset absolute and percent positions", func(t *testing.T) {
		terminal := NewVirtualTerminal(80, 24)
		ui := NewTUI(terminal)
		ui.AddChild(&lineComponent{lines: []string{""}})
		ui.ShowOverlay(&lineComponent{lines: []string{"MARGIN"}}, OverlayOptions{
			Anchor: OverlayTopLeft,
			Width:  ptr(10),
			Margin: OverlayMargin{Top: 2, Left: 3},
		})
		ui.ShowOverlay(&lineComponent{lines: []string{"OFFSET"}}, OverlayOptions{
			Anchor:  OverlayTopLeft,
			Width:   ptr(10),
			OffsetX: 10,
			OffsetY: 5,
		})
		ui.ShowOverlay(&lineComponent{lines: []string{"ABS"}}, OverlayOptions{
			Anchor: OverlayBottomRight,
			Width:  ptr(10),
			Row:    ptr(3),
			Col:    ptr(5),
		})
		ui.ShowOverlay(&lineComponent{lines: []string{"PCT"}}, OverlayOptions{
			Width: ptr(10),
			Row:   ptrPercent(50),
			Col:   ptrPercent(50),
		})
		ui.RequestRender(true)
		viewport := terminal.GetViewport()
		assertViewportContainsAt(t, viewport, 2, 3, "MARGIN")
		assertViewportContainsAt(t, viewport, 5, 10, "OFFSET")
		assertViewportContainsAt(t, viewport, 3, 5, "ABS")
		assertViewportContainsAt(t, viewport, 11, 35, "PCT")
	})

	t.Run("edge clipping and max height percent", func(t *testing.T) {
		terminal := NewVirtualTerminal(80, 10)
		ui := NewTUI(terminal)
		ui.AddChild(&lineComponent{lines: []string{""}})
		ui.ShowOverlay(&lineComponent{lines: []string{"0123456789" + strings.Repeat("X", 50)}}, OverlayOptions{
			Width: ptr(20),
			Col:   ptr(60),
		})
		ui.ShowOverlay(&lineComponent{lines: []string{"L1", "L2", "L3", "L4", "L5", "L6"}}, OverlayOptions{
			Anchor:    OverlayTopLeft,
			MaxHeight: ptrPercent(50),
		})
		ui.RequestRender(true)
		viewport := terminal.GetViewport()
		if strings.Contains(strings.Join(viewport, "\n"), "L6") {
			t.Fatalf("maxHeight percent should clip overlay rows: %#v", viewport)
		}
		if got := VisibleWidth(viewport[5]); got > 80 {
			t.Fatalf("edge overlay exceeded terminal width: width=%d line=%q", got, viewport[5])
		}
	})
}

func TestTUIOverlayPiOptionsANSIAndWideOverflowMatrix(t *testing.T) {
	complex := "\x1b[48;2;40;50;40m \x1b[38;2;128;128;128mStyled\x1b[39m\x1b[49m" +
		"\x1b]8;;http://example.com\x07link\x1b]8;;\x07 " + strings.Repeat("tail ", 20)
	wide := "中文日本語한글テスト漢字"
	cases := []struct {
		name  string
		base  []string
		line  string
		width int
	}{
		{name: "plain overflow", base: []string{""}, line: strings.Repeat("X", 100), width: 20},
		{name: "ansi osc overflow", base: []string{strings.Repeat("B", 80)}, line: complex, width: 60},
		{name: "wide boundary overflow", base: []string{""}, line: wide, width: 15},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			terminal := NewVirtualTerminal(80, 24)
			ui := NewTUI(terminal)
			ui.AddChild(&lineComponent{lines: tc.base})
			ui.ShowOverlay(&lineComponent{lines: []string{tc.line}}, OverlayOptions{Anchor: OverlayCenter, Width: ptr(tc.width)})
			ui.RequestRender(true)
			for row, line := range terminal.GetViewport() {
				if !IsImageLine(line) && VisibleWidth(line) > 80 {
					t.Fatalf("overlay row %d exceeded terminal width: width=%d line=%q", row, VisibleWidth(line), line)
				}
			}
		})
	}
}

func TestTUIOverlayCompositePreservesStyledSuffix(t *testing.T) {
	line := "\x1b[31mabcdef\x1b[0m"
	got := compositeLineAt(line, "XX", 2, 2, 8)
	if stripped := stripANSI(got); stripped != "abXXef  " {
		t.Fatalf("composited line = %q, want abXXef plus padding", stripped)
	}
	if !strings.Contains(got, "\x1b[31mef") {
		t.Fatalf("suffix should inherit base style after overlay reset: %q", got)
	}
	if VisibleWidth(got) != 8 {
		t.Fatalf("composited width = %d, want 8: %q", VisibleWidth(got), got)
	}
}

func TestTUIOverlayPiNonCapturingFocusManagement(t *testing.T) {
	terminal := NewVirtualTerminal(80, 24)
	ui := NewTUI(terminal)
	editor := &focusableOverlayComponent{lines: []string{"EDITOR"}}
	overlay := &focusableOverlayComponent{lines: []string{"OVERLAY"}}
	ui.AddChild(&lineComponent{lines: []string{""}})
	ui.SetFocus(editor)
	ui.Start()
	defer ui.Stop()

	handle := ui.ShowOverlay(overlay, OverlayOptions{NonCapturing: true})
	if !editor.Focused() || overlay.Focused() {
		t.Fatalf("non-capturing overlay should preserve editor focus: editor=%v overlay=%v", editor.Focused(), overlay.Focused())
	}
	handle.Focus()
	if editor.Focused() || !overlay.Focused() || !handle.IsFocused() {
		t.Fatalf("focus() should transfer focus to overlay: editor=%v overlay=%v handle=%v", editor.Focused(), overlay.Focused(), handle.IsFocused())
	}
	handle.Unfocus()
	if !editor.Focused() || overlay.Focused() || handle.IsFocused() {
		t.Fatalf("unfocus() should restore editor focus: editor=%v overlay=%v handle=%v", editor.Focused(), overlay.Focused(), handle.IsFocused())
	}
	handle.SetHidden(true)
	handle.SetHidden(false)
	if !editor.Focused() || overlay.Focused() {
		t.Fatalf("unhiding non-capturing overlay should not auto-focus it: editor=%v overlay=%v", editor.Focused(), overlay.Focused())
	}
}

func TestTUIHideOverlayPiTopmostNonCapturingAndHasOverlay(t *testing.T) {
	terminal := NewVirtualTerminal(80, 24)
	ui := NewTUI(terminal)
	editor := &focusableOverlayComponent{lines: []string{"EDITOR"}}
	capturing := &focusableOverlayComponent{lines: []string{"CAPTURE"}}
	nonCapturing := &focusableOverlayComponent{lines: []string{"NC"}}
	ui.AddChild(&lineComponent{lines: []string{""}})
	ui.SetFocus(editor)
	ui.Start()
	defer ui.Stop()

	ui.ShowOverlay(capturing)
	ui.ShowOverlay(nonCapturing, OverlayOptions{NonCapturing: true})
	if !ui.HasOverlay() || !capturing.Focused() || nonCapturing.Focused() {
		t.Fatalf("overlay setup mismatch: has=%v capturing=%v nonCapturing=%v", ui.HasOverlay(), capturing.Focused(), nonCapturing.Focused())
	}

	ui.HideOverlay()
	if !ui.HasOverlay() || !capturing.Focused() || nonCapturing.Focused() {
		t.Fatalf("HideOverlay should remove topmost non-capturing overlay without changing focus: has=%v capturing=%v nonCapturing=%v", ui.HasOverlay(), capturing.Focused(), nonCapturing.Focused())
	}

	ui.HideOverlay()
	if ui.HasOverlay() || !editor.Focused() || capturing.Focused() {
		t.Fatalf("HideOverlay should remove final overlay and restore pre-focus: has=%v editor=%v capturing=%v", ui.HasOverlay(), editor.Focused(), capturing.Focused())
	}
}

func TestTUIOverlayPiInvisibleFocusedOverlayReroutesInput(t *testing.T) {
	terminal := NewVirtualTerminal(80, 24)
	ui := NewTUI(terminal)
	editor := &focusableOverlayComponent{lines: []string{"EDITOR"}}
	fallback := &focusableOverlayComponent{lines: []string{"FALLBACK"}}
	nonCapturing := &focusableOverlayComponent{lines: []string{"NC"}}
	primary := &focusableOverlayComponent{lines: []string{"PRIMARY"}}
	visible := true
	ui.AddChild(&lineComponent{lines: []string{""}})
	ui.SetFocus(editor)
	ui.Start()
	defer ui.Stop()

	ui.ShowOverlay(fallback)
	ui.ShowOverlay(nonCapturing, OverlayOptions{NonCapturing: true})
	ui.ShowOverlay(primary, OverlayOptions{Visible: func(_, _ int) bool { return visible }})
	if !primary.Focused() {
		t.Fatalf("primary overlay should start focused")
	}
	visible = false
	terminal.SendInput("x")
	if len(primary.inputs) != 0 || len(nonCapturing.inputs) != 0 {
		t.Fatalf("hidden/non-capturing overlays should not receive input: primary=%#v nonCapturing=%#v", primary.inputs, nonCapturing.inputs)
	}
	if !equalLines(fallback.inputs, []string{"x"}) || !fallback.Focused() {
		t.Fatalf("fallback should receive input and focus, inputs=%#v focused=%v", fallback.inputs, fallback.Focused())
	}
}

func TestTUIOverlayPiNonCapturingFocusRestorationMatrix(t *testing.T) {
	t.Run("multiple capturing and non-capturing overlays restore focus through removals", func(t *testing.T) {
		terminal := NewVirtualTerminal(80, 24)
		ui := NewTUI(terminal)
		editor := &focusableOverlayComponent{lines: []string{"EDITOR"}}
		c1 := &focusableOverlayComponent{lines: []string{"C1"}}
		n1 := &focusableOverlayComponent{lines: []string{"N1"}}
		c2 := &focusableOverlayComponent{lines: []string{"C2"}}
		n2 := &focusableOverlayComponent{lines: []string{"N2"}}
		ui.AddChild(&lineComponent{lines: []string{""}})
		ui.SetFocus(editor)
		ui.Start()
		defer ui.Stop()

		c1Handle := ui.ShowOverlay(c1)
		ui.ShowOverlay(n1, OverlayOptions{NonCapturing: true})
		c2Handle := ui.ShowOverlay(c2)
		ui.ShowOverlay(n2, OverlayOptions{NonCapturing: true})
		if !c2.Focused() {
			t.Fatalf("top capturing overlay should be focused")
		}
		c2Handle.Hide()
		if !c1.Focused() || c2.Focused() || n1.Focused() || n2.Focused() {
			t.Fatalf("after hiding C2 focus should restore to C1: c1=%v c2=%v n1=%v n2=%v", c1.Focused(), c2.Focused(), n1.Focused(), n2.Focused())
		}
		c1Handle.Hide()
		if !editor.Focused() || c1.Focused() || n1.Focused() || n2.Focused() {
			t.Fatalf("after hiding C1 focus should restore to editor: editor=%v c1=%v n1=%v n2=%v", editor.Focused(), c1.Focused(), n1.Focused(), n2.Focused())
		}
	})

	t.Run("no-op guards keep focus stable", func(t *testing.T) {
		terminal := NewVirtualTerminal(80, 24)
		ui := NewTUI(terminal)
		editor := &focusableOverlayComponent{lines: []string{"EDITOR"}}
		overlay := &focusableOverlayComponent{lines: []string{"OVERLAY"}}
		ui.AddChild(&lineComponent{lines: []string{""}})
		ui.SetFocus(editor)
		ui.Start()
		defer ui.Stop()

		handle := ui.ShowOverlay(overlay, OverlayOptions{NonCapturing: true})
		handle.SetHidden(true)
		handle.Focus()
		if !editor.Focused() || overlay.Focused() || handle.IsFocused() {
			t.Fatalf("focus on hidden overlay should be no-op: editor=%v overlay=%v handle=%v", editor.Focused(), overlay.Focused(), handle.IsFocused())
		}
		handle.Unfocus()
		if !editor.Focused() || overlay.Focused() {
			t.Fatalf("unfocus when overlay is not focused should be no-op")
		}
		handle.Hide()
		handle.Focus()
		if !editor.Focused() || overlay.Focused() || handle.IsFocused() {
			t.Fatalf("focus after hide should be no-op: editor=%v overlay=%v handle=%v", editor.Focused(), overlay.Focused(), handle.IsFocused())
		}
	})

	t.Run("unfocus with nil preFocus clears focus and does not route input back", func(t *testing.T) {
		terminal := NewVirtualTerminal(80, 24)
		ui := NewTUI(terminal)
		overlay := &focusableOverlayComponent{lines: []string{"OVERLAY"}}
		ui.AddChild(&lineComponent{lines: []string{""}})
		ui.Start()
		defer ui.Stop()

		handle := ui.ShowOverlay(overlay)
		if !overlay.Focused() {
			t.Fatalf("capturing overlay should be focused")
		}
		handle.Unfocus()
		if overlay.Focused() || handle.IsFocused() || ui.FocusedComponent() != nil {
			t.Fatalf("unfocus with nil preFocus should clear focus: overlay=%v handle=%v focused=%T", overlay.Focused(), handle.IsFocused(), ui.FocusedComponent())
		}
		terminal.SendInput("x")
		if len(overlay.inputs) != 0 {
			t.Fatalf("unfocused overlay should not receive input: %#v", overlay.inputs)
		}
	})

	t.Run("toggle focus between non-capturing overlays then unfocus returns to editor", func(t *testing.T) {
		terminal := NewVirtualTerminal(80, 24)
		ui := NewTUI(terminal)
		editor := &focusableOverlayComponent{lines: []string{"EDITOR"}}
		a := &focusableOverlayComponent{lines: []string{"A"}}
		b := &focusableOverlayComponent{lines: []string{"B"}}
		ui.AddChild(&lineComponent{lines: []string{""}})
		ui.SetFocus(editor)
		ui.Start()
		defer ui.Stop()

		aHandle := ui.ShowOverlay(a, OverlayOptions{NonCapturing: true})
		bHandle := ui.ShowOverlay(b, OverlayOptions{NonCapturing: true})
		aHandle.Focus()
		bHandle.Focus()
		aHandle.Focus()
		aHandle.Unfocus()
		if !editor.Focused() || a.Focused() || b.Focused() {
			t.Fatalf("unfocus should return to editor without focus cycle: editor=%v a=%v b=%v", editor.Focused(), a.Focused(), b.Focused())
		}
	})
}

func TestTUIOverlayPiFocusControlsVisualOrder(t *testing.T) {
	terminal := NewVirtualTerminal(20, 6)
	ui := NewTUI(terminal)
	ui.AddChild(&lineComponent{lines: []string{""}})
	ui.Start()
	defer ui.Stop()

	lower := ui.ShowOverlay(&lineComponent{lines: []string{"A"}}, OverlayOptions{Row: ptr(0), Col: ptr(0), Width: ptr(1), NonCapturing: true})
	ui.ShowOverlay(&lineComponent{lines: []string{"B"}}, OverlayOptions{Row: ptr(0), Col: ptr(0), Width: ptr(1), NonCapturing: true})
	if got := terminal.GetViewport()[0]; !strings.HasPrefix(got, "B") {
		t.Fatalf("newer overlay should render on top, got %q", got)
	}
	lower.Focus()
	if got := terminal.GetViewport()[0]; !strings.HasPrefix(got, "A") {
		t.Fatalf("focused lower overlay should render on top, got %q", got)
	}
	lower.Unfocus()
	if got := terminal.GetViewport()[0]; !strings.HasPrefix(got, "A") {
		t.Fatalf("unfocus should not change visual order, got %q", got)
	}
}

func TestTUIOverlayPiFocusOrderMatrix(t *testing.T) {
	t.Run("refocusing an already-focused overlay raises it above newer overlays", func(t *testing.T) {
		terminal := NewVirtualTerminal(20, 6)
		ui := NewTUI(terminal)
		ui.AddChild(&lineComponent{lines: []string{""}})
		ui.Start()
		defer ui.Stop()

		first := ui.ShowOverlay(&lineComponent{lines: []string{"A"}}, OverlayOptions{Row: ptr(0), Col: ptr(0), Width: ptr(1), NonCapturing: true})
		ui.ShowOverlay(&lineComponent{lines: []string{"B"}}, OverlayOptions{Row: ptr(0), Col: ptr(0), Width: ptr(1), NonCapturing: true})
		first.Focus()
		ui.ShowOverlay(&lineComponent{lines: []string{"C"}}, OverlayOptions{Row: ptr(0), Col: ptr(0), Width: ptr(1), NonCapturing: true})
		if got := terminal.GetViewport()[0]; !strings.HasPrefix(got, "C") {
			t.Fatalf("newer overlay should render above focused older overlay, got %q", got)
		}
		first.Focus()
		if got := terminal.GetViewport()[0]; !strings.HasPrefix(got, "A") {
			t.Fatalf("refocusing already-focused overlay should raise it, got %q", got)
		}
		if !first.IsFocused() {
			t.Fatalf("refocused overlay handle should remain focused")
		}
	})

	t.Run("focusing middle overlay preserves remaining overlay order", func(t *testing.T) {
		terminal := NewVirtualTerminal(20, 6)
		ui := NewTUI(terminal)
		ui.AddChild(&lineComponent{lines: []string{""}})
		ui.Start()
		defer ui.Stop()

		ui.ShowOverlay(&lineComponent{lines: []string{"A"}}, OverlayOptions{Row: ptr(0), Col: ptr(0), Width: ptr(1), NonCapturing: true})
		middle := ui.ShowOverlay(&lineComponent{lines: []string{"B"}}, OverlayOptions{Row: ptr(0), Col: ptr(0), Width: ptr(1), NonCapturing: true})
		top := ui.ShowOverlay(&lineComponent{lines: []string{"C"}}, OverlayOptions{Row: ptr(0), Col: ptr(0), Width: ptr(1), NonCapturing: true})
		if got := terminal.GetViewport()[0]; !strings.HasPrefix(got, "C") {
			t.Fatalf("initial overlay order = %q, want C on top", got)
		}
		middle.Focus()
		if got := terminal.GetViewport()[0]; !strings.HasPrefix(got, "B") {
			t.Fatalf("focused middle overlay should render on top, got %q", got)
		}
		middle.Hide()
		if got := terminal.GetViewport()[0]; !strings.HasPrefix(got, "C") {
			t.Fatalf("hiding focused middle overlay should restore previous top, got %q", got)
		}
		top.Hide()
		if got := terminal.GetViewport()[0]; !strings.HasPrefix(got, "A") {
			t.Fatalf("hiding top overlay should reveal bottom overlay, got %q", got)
		}
	})

	t.Run("capturing overlay returns to top after being unhidden", func(t *testing.T) {
		terminal := NewVirtualTerminal(20, 6)
		ui := NewTUI(terminal)
		ui.AddChild(&lineComponent{lines: []string{""}})
		ui.Start()
		defer ui.Stop()

		ui.ShowOverlay(&lineComponent{lines: []string{"A"}}, OverlayOptions{Row: ptr(0), Col: ptr(0), Width: ptr(1), NonCapturing: true})
		capturing := ui.ShowOverlay(&lineComponent{lines: []string{"B"}}, OverlayOptions{Row: ptr(0), Col: ptr(0), Width: ptr(1)})
		if got := terminal.GetViewport()[0]; !strings.HasPrefix(got, "B") {
			t.Fatalf("capturing overlay should render on top, got %q", got)
		}
		capturing.SetHidden(true)
		ui.ShowOverlay(&lineComponent{lines: []string{"C"}}, OverlayOptions{Row: ptr(0), Col: ptr(0), Width: ptr(1), NonCapturing: true})
		if got := terminal.GetViewport()[0]; !strings.HasPrefix(got, "C") {
			t.Fatalf("new non-capturing overlay should render on top while capturing overlay is hidden, got %q", got)
		}
		capturing.SetHidden(false)
		if got := terminal.GetViewport()[0]; !strings.HasPrefix(got, "B") {
			t.Fatalf("unhidden capturing overlay should return to top, got %q", got)
		}
	})
}

func TestTUIInvalidateInvalidatesChildrenAndOverlays(t *testing.T) {
	terminal := newFakeTerminal(20, 5)
	ui := NewTUI(terminal)
	child := &cellDimensionRecorderComponent{}
	overlay := &cellDimensionRecorderComponent{}
	ui.AddChild(child)
	ui.ShowOverlay(overlay)

	child.invalidations = 0
	overlay.invalidations = 0
	ui.Invalidate()

	if child.invalidations != 1 {
		t.Fatalf("child invalidations = %d, want 1", child.invalidations)
	}
	if overlay.invalidations != 1 {
		t.Fatalf("overlay invalidations = %d, want 1", overlay.invalidations)
	}
}

func TestTUIUsesSynchronizedFullAndDiffRendering(t *testing.T) {
	terminal := newFakeTerminal(20, 10)
	ui := NewTUI(terminal)
	component := &lineComponent{lines: []string{"alpha", "bravo"}}
	ui.AddChild(component)
	ui.RequestRender(true)
	first := terminal.String()
	if !strings.Contains(first, "\x1b[?2026h") || !strings.Contains(first, "\x1b[?2026l") {
		t.Fatalf("full render should use synchronized output: %q", first)
	}
	if !strings.Contains(first, "\x1b[2J\x1b[H") {
		t.Fatalf("full render should clear screen: %q", first)
	}
	if ui.FullRedraws() != 1 {
		t.Fatalf("full redraws = %d, want 1", ui.FullRedraws())
	}

	terminal.ClearOutput()
	component.lines = []string{"alpha", "charlie"}
	ui.RequestRender(false)
	diff := terminal.String()
	if strings.Contains(diff, "\x1b[2J") {
		t.Fatalf("diff render should not clear screen: %q", diff)
	}
	if !strings.Contains(diff, "\x1b[2K\r") || !strings.Contains(diff, "charlie") {
		t.Fatalf("diff render missing clear-line/update: %q", diff)
	}
	if ui.FullRedraws() != 1 {
		t.Fatalf("diff render should not increment full redraws: %d", ui.FullRedraws())
	}
}

func TestTUIDiffRenderingOnlyWritesChangedRangeLikePi(t *testing.T) {
	terminal := newFakeTerminal(50, 10)
	ui := NewTUI(terminal)
	component := &lineComponent{lines: []string{"Line 0", "stable tail one", "stable tail two"}}
	ui.AddChild(component)
	ui.RequestRender(true)
	terminal.ClearOutput()

	component.lines = []string{"CHANGED", "stable tail one", "stable tail two"}
	ui.RequestRender(false)
	output := terminal.String()
	if !strings.Contains(output, "CHANGED") {
		t.Fatalf("diff output should include changed line: %q", output)
	}
	if strings.Contains(output, "stable tail one") || strings.Contains(output, "stable tail two") {
		t.Fatalf("diff output should not rewrite unchanged trailing lines: %q", output)
	}
	if strings.Contains(output, "\x1b[2J") {
		t.Fatalf("single-line diff should not force full redraw: %q", output)
	}

	terminal.ClearOutput()
	component.lines = []string{"CHANGED", "changed middle", "stable tail two"}
	ui.RequestRender(false)
	output = terminal.String()
	if !strings.Contains(output, "changed middle") {
		t.Fatalf("middle diff output should include changed line: %q", output)
	}
	if strings.Contains(output, "stable tail two") {
		t.Fatalf("middle diff output should not rewrite unchanged tail: %q", output)
	}
}

func TestTUIDiffRenderingKeepsCursorPositionForSubsequentAppend(t *testing.T) {
	terminal := newFakeTerminal(50, 10)
	ui := NewTUI(terminal)
	component := &lineComponent{lines: []string{"Line 0", "Line 1", "Line 2"}}
	ui.AddChild(component)
	ui.RequestRender(true)
	terminal.ClearOutput()

	component.lines = []string{"CHANGED", "Line 1", "Line 2"}
	ui.RequestRender(false)
	terminal.ClearOutput()

	component.lines = []string{"CHANGED", "Line 1", "Line 2", "appended"}
	ui.RequestRender(false)
	output := terminal.String()
	if !strings.Contains(output, "\r\n\x1b[2Kappended") {
		t.Fatalf("append after narrow diff should scroll from actual cursor row: %q", output)
	}
	if strings.Contains(output, "Line 1") || strings.Contains(output, "Line 2") {
		t.Fatalf("append should not rewrite unchanged existing lines: %q", output)
	}
}

func TestTUIDeletesKittyImagesBeforeRedraw(t *testing.T) {
	terminal := newFakeTerminal(40, 10)
	ui := NewTUI(terminal)
	component := &lineComponent{lines: []string{EncodeKitty([]byte("old"), ImageRenderOptions{ID: 42})}}
	ui.AddChild(component)
	ui.RequestRender(true)
	terminal.ClearOutput()
	component.lines = []string{"plain"}
	ui.RequestRender(true)
	output := terminal.String()
	deleteSeq := DeleteKittyImage(42)
	if !strings.Contains(output, deleteSeq) {
		t.Fatalf("expected old kitty image delete sequence in %q", output)
	}
	if strings.Index(output, deleteSeq) > strings.Index(output, "\x1b[2J") {
		t.Fatalf("image should be deleted before screen clear: %q", output)
	}
}

func TestTUIDeletesChangedKittyImageBeforeMovedPlacement(t *testing.T) {
	terminal := newFakeTerminal(40, 10)
	ui := NewTUI(terminal)
	oldImage := EncodeKitty([]byte("old"), ImageRenderOptions{ID: 42, Width: 2, Height: 2, DisableCursorMovement: true})
	newImage := EncodeKitty([]byte("new"), ImageRenderOptions{ID: 42, Width: 2, Height: 1, DisableCursorMovement: true})
	component := &lineComponent{lines: []string{"top", oldImage}}
	ui.AddChild(component)
	ui.RequestRender(true)
	terminal.ClearOutput()

	component.lines = []string{newImage, ""}
	ui.RequestRender(false)
	output := terminal.String()
	deleteSeq := DeleteKittyImage(42)
	deleteIndex := strings.Index(output, deleteSeq)
	drawIndex := strings.Index(output, newImage)
	if deleteIndex < 0 {
		t.Fatalf("changed old image should be deleted: %q", output)
	}
	if drawIndex < 0 {
		t.Fatalf("new image placement should be drawn: %q", output)
	}
	if deleteIndex > drawIndex {
		t.Fatalf("old image must be deleted before new placement is drawn: %q", output)
	}
}

func TestTUIRedrawsKittyImageLineWhenReservedRowChanges(t *testing.T) {
	terminal := newFakeTerminal(40, 10)
	ui := NewTUI(terminal)
	image := EncodeKitty([]byte("image"), ImageRenderOptions{ID: 88, Width: 2, Height: 2, DisableCursorMovement: true})
	component := &lineComponent{lines: []string{"", image}}
	ui.AddChild(component)
	ui.RequestRender(true)
	terminal.ClearOutput()

	component.lines = []string{"covered", image}
	ui.RequestRender(false)
	output := terminal.String()
	deleteSeq := DeleteKittyImage(88)
	deleteIndex := strings.Index(output, deleteSeq)
	drawIndex := strings.Index(output, image)
	if deleteIndex < 0 {
		t.Fatalf("image should be deleted when a reserved row changes: %q", output)
	}
	if drawIndex < 0 {
		t.Fatalf("unchanged image line should be redrawn after deleting placement: %q", output)
	}
	if deleteIndex > drawIndex {
		t.Fatalf("old placement must be deleted before image line is redrawn: %q", output)
	}
	if strings.Contains(output, "\x1b[2J") {
		t.Fatalf("reserved row changes should not force a full redraw: %q", output)
	}
}

func TestTUIResizeFullRedrawPolicy(t *testing.T) {
	t.Setenv("TERMUX_VERSION", "")
	terminal := newFakeTerminal(40, 10)
	ui := NewTUI(terminal)
	component := &lineComponent{lines: []string{"Line 0", "Line 1"}}
	ui.AddChild(component)
	ui.Start()
	initial := ui.FullRedraws()

	terminal.rows = 15
	terminal.resize()
	if ui.FullRedraws() <= initial {
		t.Fatalf("height resize should trigger full redraw, got %d <= %d", ui.FullRedraws(), initial)
	}
	afterHeight := ui.FullRedraws()

	terminal.cols = 60
	terminal.resize()
	if ui.FullRedraws() <= afterHeight {
		t.Fatalf("width resize should trigger full redraw, got %d <= %d", ui.FullRedraws(), afterHeight)
	}
}

func TestTUIResizeSkipsTermuxHeightFullRedraw(t *testing.T) {
	t.Setenv("TERMUX_VERSION", "1")
	terminal := newFakeTerminal(40, 10)
	ui := NewTUI(terminal)
	component := &lineComponent{lines: []string{"Line 0", "Line 1"}}
	ui.AddChild(component)
	ui.Start()
	initial := ui.FullRedraws()
	terminal.ClearOutput()

	terminal.rows = 15
	terminal.resize()
	if ui.FullRedraws() != initial {
		t.Fatalf("termux height resize should not force full redraw: got %d, want %d", ui.FullRedraws(), initial)
	}
	if output := terminal.String(); strings.Contains(output, "\x1b[2J") {
		t.Fatalf("termux height resize should not clear screen: %q", output)
	}
}

func TestVirtualTerminalViewportForFullRender(t *testing.T) {
	terminal := NewVirtualTerminal(20, 5)
	ui := NewTUI(terminal)
	component := &lineComponent{lines: []string{
		"Line 0", "Line 1", "Line 2", "Line 3", "Line 4", "Line 5", "Line 6",
	}}
	ui.AddChild(component)
	ui.RequestRender(true)

	want := []string{"Line 2", "Line 3", "Line 4", "Line 5", "Line 6"}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("viewport = %#v, want %#v", got, want)
	}
}

func TestVirtualTerminalPiHarnessConvenienceMethods(t *testing.T) {
	terminal := NewVirtualTerminal(5, 2)
	var seen []string
	terminal.Start(func(data string) {
		seen = append(seen, data)
	}, func() {
		seen = append(seen, "resize")
	})
	terminal.SendInput("x")
	terminal.Resize(6, 3)
	terminal.Write("abc\nsecond\nthird")
	terminal.Flush()
	terminal.WaitForRender()

	if !equalLines(seen, []string{"x", "resize"}) {
		t.Fatalf("input/resize callbacks = %#v", seen)
	}
	if got := terminal.FlushAndGetViewport(); !equalLines(got, terminal.GetViewport()) || len(got) != 3 {
		t.Fatalf("flush viewport = %#v", got)
	}
	if scroll := terminal.GetScrollBuffer(); len(scroll) < len(terminal.GetViewport()) {
		t.Fatalf("scroll buffer should include viewport, scroll=%#v viewport=%#v", scroll, terminal.GetViewport())
	}
	x, y := terminal.GetCursorPosition()
	if x < 0 || y < 0 {
		t.Fatalf("cursor position should be non-negative, got %d,%d", x, y)
	}
	terminal.Clear()
	if got := terminal.GetViewport(); len(got) != 3 {
		t.Fatalf("clear should preserve resized viewport height, got %#v", got)
	}
	terminal.Reset()
	if got := terminal.Output(); got != "" {
		t.Fatalf("reset should clear output log, got %q", got)
	}
}

func TestVirtualTerminalClearKeepsCurrentLineLikeXtermHarness(t *testing.T) {
	terminal := NewVirtualTerminal(10, 3)
	if err := terminal.Write("old1\r\nold2\r\nprompt"); err != nil {
		t.Fatal(err)
	}

	terminal.Clear()

	want := []string{"prompt", "", ""}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("clear viewport = %#v, want %#v", got, want)
	}
	if got := terminal.GetScrollBuffer(); !equalLines(got, want) {
		t.Fatalf("clear scroll buffer = %#v, want %#v", got, want)
	}
	x, y := terminal.GetCursorPosition()
	if x != len("prompt") || y != 0 {
		t.Fatalf("clear cursor = %d,%d, want %d,0", x, y, len("prompt"))
	}
}

func TestVirtualTerminalViewportAndScrollBufferTrimTrailingBlanksLikePi(t *testing.T) {
	terminal := NewVirtualTerminal(10, 3)
	if err := terminal.Write("abc   \r\nx  "); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.GetViewport(), []string{"abc", "x", ""}; !equalLines(got, want) {
		t.Fatalf("viewport should trim trailing blanks like Pi xterm harness: %#v, want %#v", got, want)
	}
	if got, want := terminal.GetScrollBuffer(), []string{"abc", "x", ""}; !equalLines(got, want) {
		t.Fatalf("scroll buffer should trim trailing blanks like Pi xterm harness: %#v, want %#v", got, want)
	}
}

func TestVirtualTerminalTUIDifferentialSpinnerPreservesRows(t *testing.T) {
	terminal := NewVirtualTerminal(40, 10)
	ui := NewTUI(terminal)
	component := &lineComponent{lines: []string{"Header", "Working...", "Footer"}}
	ui.AddChild(component)
	ui.RequestRender(true)

	for _, frame := range []string{"|", "/", "-", "\\"} {
		component.lines = []string{"Header", "Working " + frame, "Footer"}
		ui.RequestRender(false)
		viewport := terminal.GetViewport()
		if !strings.Contains(viewport[0], "Header") || !strings.Contains(viewport[1], "Working "+frame) || !strings.Contains(viewport[2], "Footer") {
			t.Fatalf("viewport for frame %q = %#v", frame, viewport[:3])
		}
	}
}

func TestVirtualTerminalTUIDifferentialPiChangedLineMatrix(t *testing.T) {
	t.Run("cursor after shrink with unchanged remaining lines", func(t *testing.T) {
		terminal := NewVirtualTerminal(40, 10)
		ui := NewTUI(terminal)
		component := &lineComponent{lines: []string{"Line 0", "Line 1", "Line 2", "Line 3", "Line 4"}}
		ui.AddChild(component)
		ui.RequestRender(true)

		component.lines = []string{"Line 0", "Line 1", "Line 2"}
		ui.RequestRender(false)
		component.lines = []string{"Line 0", "CHANGED", "Line 2"}
		ui.RequestRender(false)

		viewport := terminal.GetViewport()
		if !strings.Contains(viewport[1], "CHANGED") {
			t.Fatalf("changed row after shrink not rendered at row 1: %#v", viewport[:4])
		}
	})

	t.Run("first line changes while tail stays stable", func(t *testing.T) {
		terminal := NewVirtualTerminal(40, 10)
		ui := NewTUI(terminal)
		component := &lineComponent{lines: []string{"Line 0", "Line 1", "Line 2", "Line 3"}}
		ui.AddChild(component)
		ui.RequestRender(true)

		component.lines = []string{"CHANGED", "Line 1", "Line 2", "Line 3"}
		ui.RequestRender(false)

		viewport := terminal.GetViewport()
		for row, want := range []string{"CHANGED", "Line 1", "Line 2", "Line 3"} {
			if !strings.Contains(viewport[row], want) {
				t.Fatalf("viewport row %d = %q, want %q in %#v", row, viewport[row], want, viewport[:4])
			}
		}
	})

	t.Run("last line changes while prefix stays stable", func(t *testing.T) {
		terminal := NewVirtualTerminal(40, 10)
		ui := NewTUI(terminal)
		component := &lineComponent{lines: []string{"Line 0", "Line 1", "Line 2", "Line 3"}}
		ui.AddChild(component)
		ui.RequestRender(true)

		component.lines = []string{"Line 0", "Line 1", "Line 2", "CHANGED"}
		ui.RequestRender(false)

		viewport := terminal.GetViewport()
		for row, want := range []string{"Line 0", "Line 1", "Line 2", "CHANGED"} {
			if !strings.Contains(viewport[row], want) {
				t.Fatalf("viewport row %d = %q, want %q in %#v", row, viewport[row], want, viewport[:4])
			}
		}
	})

	t.Run("multiple non-adjacent lines change", func(t *testing.T) {
		terminal := NewVirtualTerminal(40, 10)
		ui := NewTUI(terminal)
		component := &lineComponent{lines: []string{"Line 0", "Line 1", "Line 2", "Line 3", "Line 4"}}
		ui.AddChild(component)
		ui.RequestRender(true)

		component.lines = []string{"TOP", "Line 1", "MIDDLE", "Line 3", "BOTTOM"}
		ui.RequestRender(false)

		viewport := terminal.GetViewport()
		for row, want := range []string{"TOP", "Line 1", "MIDDLE", "Line 3", "BOTTOM"} {
			if !strings.Contains(viewport[row], want) {
				t.Fatalf("viewport row %d = %q, want %q in %#v", row, viewport[row], want, viewport[:5])
			}
		}
	})
}

func TestVirtualTerminalTUIShrinkClearsStaleRows(t *testing.T) {
	terminal := NewVirtualTerminal(40, 10)
	ui := NewTUI(terminal)
	ui.SetClearOnShrink(true)
	component := &lineComponent{lines: []string{"Line 0", "Line 1", "Line 2", "Line 3"}}
	ui.AddChild(component)
	ui.RequestRender(true)

	component.lines = []string{"Only line"}
	ui.RequestRender(false)
	viewport := terminal.GetViewport()
	if !strings.Contains(viewport[0], "Only line") {
		t.Fatalf("first viewport row = %q", viewport[0])
	}
	if strings.TrimSpace(viewport[1]) != "" || strings.TrimSpace(viewport[2]) != "" {
		t.Fatalf("stale rows after shrink: %#v", viewport[:4])
	}

	component.lines = nil
	ui.RequestRender(false)
	viewport = terminal.GetViewport()
	if strings.TrimSpace(viewport[0]) != "" || strings.TrimSpace(viewport[1]) != "" {
		t.Fatalf("stale rows after shrink to empty: %#v", viewport[:4])
	}
	redrawsAfterEmpty := ui.FullRedraws()

	component.lines = []string{"New Line 0", "New Line 1"}
	ui.RequestRender(false)
	viewport = terminal.GetViewport()
	if !strings.Contains(viewport[0], "New Line 0") || !strings.Contains(viewport[1], "New Line 1") {
		t.Fatalf("content should recover after empty state: %#v", viewport[:4])
	}
	if strings.TrimSpace(viewport[2]) != "" {
		t.Fatalf("empty-state recovery should not leave stale rows: %#v", viewport[:4])
	}
	if ui.FullRedraws() <= redrawsAfterEmpty {
		t.Fatalf("empty-state recovery should render again: full redraws %d <= %d", ui.FullRedraws(), redrawsAfterEmpty)
	}
}

func TestVirtualTerminalTUIShrinkResetsViewportTop(t *testing.T) {
	terminal := NewVirtualTerminal(20, 5)
	ui := NewTUI(terminal)
	ui.SetClearOnShrink(true)
	component := &lineComponent{}
	ui.AddChild(component)

	component.lines = []string{"Line 0", "Line 1", "Line 2", "Line 3", "Line 4", "Line 5", "Line 6", "Line 7"}
	ui.RequestRender(true)
	component.lines = []string{"Line 0", "Line 1"}
	ui.RequestRender(false)
	component.lines = []string{"Line 0", "Line 1", "Line 2"}
	ui.RequestRender(false)

	want := []string{"Line 0", "Line 1", "Line 2", "", ""}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("viewport = %#v, want %#v", got, want)
	}
}

func TestVirtualTerminalTUIViewportMovedUpForcesFullRedrawWithoutClearOnShrink(t *testing.T) {
	terminal := NewVirtualTerminal(20, 5)
	ui := NewTUI(terminal)
	component := &lineComponent{}
	ui.AddChild(component)

	component.lines = []string{"Line 0", "Line 1", "Line 2", "Line 3", "Line 4", "Line 5", "Line 6", "Line 7", "Line 8", "Line 9", "Line 10", "Line 11"}
	ui.RequestRender(true)
	initialRedraws := ui.FullRedraws()

	component.lines = []string{"Line 0", "Line 1", "Line 2", "Line 3", "Line 4", "Line 5", "Line 6"}
	ui.RequestRender(false)

	if ui.FullRedraws() <= initialRedraws {
		t.Fatalf("viewport-moving shrink should force a full redraw: got %d <= %d", ui.FullRedraws(), initialRedraws)
	}
	want := []string{"Line 2", "Line 3", "Line 4", "Line 5", "Line 6"}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("viewport = %#v, want %#v", got, want)
	}
}

func TestVirtualTerminalTUIAppendAfterViewportResetUsesDiff(t *testing.T) {
	terminal := NewVirtualTerminal(20, 5)
	ui := NewTUI(terminal)
	component := &lineComponent{}
	ui.AddChild(component)

	component.lines = []string{"Line 0", "Line 1", "Line 2", "Line 3", "Line 4", "Line 5", "Line 6", "Line 7"}
	ui.RequestRender(true)
	component.lines = []string{"Line 0", "Line 1"}
	ui.RequestRender(false)
	redrawsAfterShrink := ui.FullRedraws()

	component.lines = []string{"Line 0", "Line 1", "Line 2"}
	ui.RequestRender(false)

	if ui.FullRedraws() != redrawsAfterShrink {
		t.Fatalf("append after viewport reset should use diff path: got %d, want %d", ui.FullRedraws(), redrawsAfterShrink)
	}
	want := []string{"Line 0", "Line 1", "Line 2", "", ""}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("viewport = %#v, want %#v", got, want)
	}
}

func TestTUIAppendPastViewportScrollsFromCurrentCursor(t *testing.T) {
	terminal := NewVirtualTerminal(20, 5)
	ui := NewTUI(terminal)
	component := &lineComponent{}
	ui.AddChild(component)

	component.lines = []string{"Line 0", "Line 1", "Line 2", "Line 3", "Line 4", "Line 5", "Line 6", "Line 7"}
	ui.RequestRender(true)
	terminal.ClearOutput()

	component.lines = append(component.lines, "Line 8")
	ui.RequestRender(false)

	output := terminal.Output()
	if strings.Contains(output, "\x1b[H") {
		t.Fatalf("append after viewport overflow should not home then address an absolute scrollback row: %q", output)
	}
	if !strings.Contains(output, "\r\n\x1b[2KLine 8") {
		t.Fatalf("append after viewport overflow should scroll with CRLF and render appended line: %q", output)
	}
	want := []string{"Line 4", "Line 5", "Line 6", "Line 7", "Line 8"}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("viewport = %#v, want %#v", got, want)
	}
}

func TestTUIChangeAboveViewportForcesFullRedraw(t *testing.T) {
	terminal := newFakeTerminal(20, 5)
	ui := NewTUI(terminal)
	component := &lineComponent{lines: []string{"Line 0", "Line 1", "Line 2", "Line 3", "Line 4", "Line 5", "Line 6", "Line 7"}}
	ui.AddChild(component)
	ui.RequestRender(true)
	initialRedraws := ui.FullRedraws()
	terminal.ClearOutput()

	component.lines[1] = "Changed 1"
	ui.RequestRender(false)

	if ui.FullRedraws() <= initialRedraws {
		t.Fatalf("change above viewport should force a full redraw: got %d <= %d", ui.FullRedraws(), initialRedraws)
	}
	output := terminal.String()
	if !strings.Contains(output, "\x1b[2J\x1b[H\x1b[3J") {
		t.Fatalf("change above viewport should clear and rerender instead of diffing invisible scrollback: %q", output)
	}
}

func TestVirtualTerminalTUIClearsTransientComponentHighWater(t *testing.T) {
	terminal := NewVirtualTerminal(40, 10)
	ui := NewTUI(terminal)
	chat := &lineComponent{}
	editor := &lineComponent{}
	ui.AddChild(chat)
	ui.AddChild(editor)

	longChat := make([]string, 15)
	for i := range longChat {
		longChat[i] = "Chat " + strconv.Itoa(i)
	}
	shortChat := make([]string, 12)
	for i := range shortChat {
		shortChat[i] = "Chat " + strconv.Itoa(i)
	}
	editorLines := []string{"Editor 0", "Editor 1", "Editor 2"}
	selectorLines := make([]string, 8)
	for i := range selectorLines {
		selectorLines[i] = "Selector " + strconv.Itoa(i)
	}

	chat.lines = longChat
	editor.lines = editorLines
	ui.RequestRender(true)

	editor.lines = selectorLines
	ui.RequestRender(false)
	editor.lines = editorLines
	ui.RequestRender(false)

	redrawsBeforeSwitch := ui.FullRedraws()
	chat.lines = shortChat
	ui.RequestRender(false)

	if ui.FullRedraws() <= redrawsBeforeSwitch {
		t.Fatalf("shorter branch should force a full redraw: got %d <= %d", ui.FullRedraws(), redrawsBeforeSwitch)
	}
	want := []string{"Chat 5", "Chat 6", "Chat 7", "Chat 8", "Chat 9", "Chat 10", "Chat 11", "Editor 0", "Editor 1", "Editor 2"}
	if got := terminal.GetViewport(); !equalLines(got, want) {
		t.Fatalf("viewport = %#v, want %#v", got, want)
	}
}

func TestTUIConsumesCellSizeResponsesAndForwardsEscape(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Images: true, Protocol: ImageProtocolKitty})
	defer func() {
		ResetCapabilitiesCache()
		SetCellDimensions(defaultCellDimensions)
	}()
	SetCellDimensions(defaultCellDimensions)

	terminal := NewVirtualTerminal(80, 24)
	ui := NewTUI(terminal)
	recorder := &inputRecorderComponent{}
	ui.SetFocus(recorder)
	ui.Start()

	if !strings.Contains(terminal.Output(), "\x1b[16t") {
		t.Fatalf("expected cell-size query in output: %q", terminal.Output())
	}
	terminal.SendInput("\x1b")
	if !equalLines(recorder.inputs, []string{"\x1b"}) {
		t.Fatalf("bare escape should be forwarded: %#v", recorder.inputs)
	}
	terminal.SendInput("\x1b[6;20;10t")
	if !equalLines(recorder.inputs, []string{"\x1b"}) {
		t.Fatalf("cell-size response should be consumed: %#v", recorder.inputs)
	}
	if got := GetCellDimensions(); got.Width != 10 || got.Height != 20 {
		t.Fatalf("cell dimensions = %#v, want 10x20", got)
	}
	terminal.SendInput("q")
	if !equalLines(recorder.inputs, []string{"\x1b", "q"}) {
		t.Fatalf("later input should be forwarded: %#v", recorder.inputs)
	}
	ui.Stop()
}

func TestTUICellSizeResponseInvalidatesAndRerenders(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Images: true, Protocol: ImageProtocolKitty})
	defer func() {
		ResetCapabilitiesCache()
		SetCellDimensions(defaultCellDimensions)
	}()
	SetCellDimensions(defaultCellDimensions)

	terminal := NewVirtualTerminal(80, 24)
	ui := NewTUI(terminal)
	component := &cellDimensionRecorderComponent{}
	ui.AddChild(component)
	ui.Start()
	terminal.ClearOutput()

	terminal.SendInput("\x1b[6;20;10t")

	if component.invalidations == 0 {
		t.Fatalf("cell-size response should invalidate components")
	}
	if got := GetCellDimensions(); got.Width != 10 || got.Height != 20 {
		t.Fatalf("cell dimensions = %#v, want 10x20", got)
	}
	viewport := terminal.GetViewport()
	if !strings.Contains(strings.Join(viewport, "\n"), "10x20") {
		t.Fatalf("cell-size response should rerender with new dimensions, viewport=%#v output=%q", viewport, terminal.Output())
	}
	ui.Stop()
}

func ptr(v int) *SizeValue {
	s := Cells(v)
	return &s
}

func ptrPercent(v int) *SizeValue {
	s := Percent(v)
	return &s
}

func ptrPercentFloat(v float64) *SizeValue {
	s := PercentFloat(v)
	return &s
}

func assertViewportContainsAt(t *testing.T, viewport []string, row, col int, text string) {
	t.Helper()
	if row < 0 || row >= len(viewport) {
		t.Fatalf("row %d outside viewport length %d", row, len(viewport))
	}
	gotCol := strings.Index(viewport[row], text)
	if gotCol != col {
		t.Fatalf("expected %q at row %d col %d, got col %d in %q", text, row, col, gotCol, viewport[row])
	}
}
