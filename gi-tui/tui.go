package gitui

import (
	"math"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Component renders terminal lines for a viewport width.
type Component interface {
	Render(width int) []string
	Invalidate()
}

// SizeAwareComponent can opt into terminal-size-aware rendering while keeping
// the simple Component API available for standalone use.
type SizeAwareComponent interface {
	RenderWithSize(width, height int) []string
}

// InputHandler is implemented by components that receive focused keyboard input.
type InputHandler interface {
	HandleInput(data string)
}

// Focusable is implemented by components that need cursor/focus state.
type Focusable interface {
	SetFocused(focused bool)
	Focused() bool
}

func IsFocusable(component Component) bool {
	if component == nil {
		return false
	}
	_, ok := component.(Focusable)
	return ok
}

type KeyReleaseReceiver interface {
	WantsKeyRelease() bool
}

// FocusState can be embedded by focusable components.
type FocusState struct {
	focused atomic.Bool
}

func (f *FocusState) SetFocused(focused bool) { f.focused.Store(focused) }
func (f *FocusState) Focused() bool           { return f.focused.Load() }

const (
	CursorMarker = "\x1b_pi:c\x07"
	segmentReset = "\x1b[0m\x1b]8;;\x07"
)

type Container struct {
	mu       sync.RWMutex
	children []Component
}

func NewContainer() *Container { return &Container{} }

func (c *Container) AddChild(component Component) {
	if component == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.children = append(c.children, component)
}

func (c *Container) InsertChild(index int, component Component) {
	if component == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	index = max(0, min(index, len(c.children)))
	c.children = append(c.children, nil)
	copy(c.children[index+1:], c.children[index:])
	c.children[index] = component
}

func (c *Container) RemoveChild(component Component) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, child := range c.children {
		if child == component {
			c.children = append(c.children[:i], c.children[i+1:]...)
			return
		}
	}
}

func (c *Container) RemoveChildAt(index int) Component {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < 0 || index >= len(c.children) {
		return nil
	}
	removed := c.children[index]
	c.children = append(c.children[:index], c.children[index+1:]...)
	return removed
}

func (c *Container) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.children = nil
}

func (c *Container) SetChildren(children []Component) {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := c.children[:0]
	for _, child := range children {
		if child != nil {
			next = append(next, child)
		}
	}
	c.children = next
}

func (c *Container) ChildCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.children)
}

func (c *Container) Children() []Component {
	return c.snapshotChildren()
}

func (c *Container) snapshotChildren() []Component {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Component, len(c.children))
	copy(out, c.children)
	return out
}

func (c *Container) Invalidate() {
	for _, child := range c.snapshotChildren() {
		child.Invalidate()
	}
}

func (c *Container) Render(width int) []string {
	var lines []string
	for _, child := range c.snapshotChildren() {
		lines = append(lines, child.Render(width)...)
	}
	return lines
}

func (c *Container) RenderWithSize(width, height int) []string {
	var lines []string
	for _, child := range c.snapshotChildren() {
		lines = append(lines, renderComponent(child, width, height)...)
	}
	return lines
}

func renderComponent(component Component, width, height int) []string {
	if sized, ok := component.(SizeAwareComponent); ok {
		return sized.RenderWithSize(width, height)
	}
	return component.Render(width)
}

type InputListenerResult struct {
	Consume bool
	Data    string
	HasData bool
}

type InputListener func(data string) InputListenerResult

func InputListenerData(data string) InputListenerResult {
	return InputListenerResult{Data: data, HasData: true}
}

type inputListenerEntry struct {
	id       int
	listener InputListener
}

type OverlayAnchor string

const (
	OverlayCenter       OverlayAnchor = "center"
	OverlayTopLeft      OverlayAnchor = "top-left"
	OverlayTopRight     OverlayAnchor = "top-right"
	OverlayBottomLeft   OverlayAnchor = "bottom-left"
	OverlayBottomRight  OverlayAnchor = "bottom-right"
	OverlayTopCenter    OverlayAnchor = "top-center"
	OverlayBottomCenter OverlayAnchor = "bottom-center"
	OverlayLeftCenter   OverlayAnchor = "left-center"
	OverlayRightCenter  OverlayAnchor = "right-center"
)

type SizeValue struct {
	Value        int
	Percent      bool
	PercentValue float64
}

func Cells(value int) SizeValue { return SizeValue{Value: value} }
func Percent(value int) SizeValue {
	return SizeValue{Value: value, Percent: true, PercentValue: float64(value)}
}
func PercentFloat(value float64) SizeValue {
	return SizeValue{Value: int(value), Percent: true, PercentValue: value}
}

type OverlayMargin struct {
	Top, Right, Bottom, Left int
}

type OverlayOptions struct {
	Width        *SizeValue
	MinWidth     int
	MaxHeight    *SizeValue
	Anchor       OverlayAnchor
	OffsetX      int
	OffsetY      int
	Row          *SizeValue
	Col          *SizeValue
	Margin       OverlayMargin
	Visible      func(termWidth, termHeight int) bool
	NonCapturing bool
}

type OverlayHandle interface {
	Hide()
	SetHidden(hidden bool)
	IsHidden() bool
	Focus()
	Unfocus()
	IsFocused() bool
}

type overlayEntry struct {
	component  Component
	options    OverlayOptions
	preFocus   Component
	hidden     bool
	focusOrder int
}

type renderedOverlay struct {
	lines []string
	row   int
	col   int
	width int
}

type overlayHandle struct {
	t     *TUI
	entry *overlayEntry
}

func (h *overlayHandle) Hide() {
	h.t.mu.Lock()
	defer h.t.mu.Unlock()
	h.t.removeOverlayLocked(h.entry)
	h.t.requestRenderLocked(false)
}

func (h *overlayHandle) SetHidden(hidden bool) {
	h.t.mu.Lock()
	defer h.t.mu.Unlock()
	if h.entry.hidden == hidden {
		return
	}
	h.entry.hidden = hidden
	if hidden && h.t.focusedComponent == h.entry.component {
		if top := h.t.topVisibleOverlayLocked(); top != nil {
			h.t.setFocusLocked(top.component)
		} else {
			h.t.setFocusLocked(h.entry.preFocus)
		}
	} else if !hidden && !h.entry.options.NonCapturing && h.t.isOverlayVisibleLocked(h.entry) {
		h.entry.focusOrder = h.t.nextFocusOrder()
		h.t.setFocusLocked(h.entry.component)
	}
	h.t.requestRenderLocked(false)
}

func (h *overlayHandle) IsHidden() bool {
	h.t.mu.Lock()
	defer h.t.mu.Unlock()
	return h.entry.hidden
}

func (h *overlayHandle) Focus() {
	h.t.mu.Lock()
	defer h.t.mu.Unlock()
	if !h.t.containsOverlayLocked(h.entry) || !h.t.isOverlayVisibleLocked(h.entry) {
		return
	}
	h.entry.focusOrder = h.t.nextFocusOrder()
	h.t.setFocusLocked(h.entry.component)
	h.t.requestRenderLocked(false)
}

func (h *overlayHandle) Unfocus() {
	h.t.mu.Lock()
	defer h.t.mu.Unlock()
	if h.t.focusedComponent == h.entry.component {
		if top := h.t.topVisibleOverlayLocked(); top != nil && top != h.entry {
			h.t.setFocusLocked(top.component)
		} else {
			h.t.setFocusLocked(h.entry.preFocus)
		}
		h.t.requestRenderLocked(false)
	}
}

func (h *overlayHandle) IsFocused() bool { return h.t.FocusedComponent() == h.entry.component }

// TUI manages component focus, input routing, overlays, and terminal rendering.
type TUI struct {
	*Container
	terminal          Terminal
	previousLines     []string
	previousWidth     int
	previousHeight    int
	cursorPosition    terminalCursorPosition
	hardwareCursorRow int
	focusedComponent  Component
	listeners         []inputListenerEntry
	nextListenerID    int
	overlays          []*overlayEntry
	focusCounter      int
	stopped           bool
	clearOnShrink     bool
	showCursor        bool
	fullRedraws       int
	onDebug           func()
	mu                sync.Mutex
}

type terminalCursorPosition struct {
	row int
	col int
	ok  bool
}

type renderBuffer struct {
	output   string
	finalRow int
}

func NewTUI(terminal Terminal, showHardwareCursor ...bool) *TUI {
	if terminal == nil {
		terminal = NewProcessTerminal()
	}
	t := &TUI{
		Container:     NewContainer(),
		terminal:      terminal,
		clearOnShrink: os.Getenv("PI_CLEAR_ON_SHRINK") == "1",
		showCursor:    os.Getenv("PI_HARDWARE_CURSOR") == "1",
	}
	if len(showHardwareCursor) > 0 {
		t.showCursor = showHardwareCursor[0]
	}
	return t
}

func (t *TUI) Terminal() Terminal { return t.terminal }

func (t *TUI) FullRedraws() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.fullRedraws
}

func (t *TUI) GetFullRedraws() int {
	return t.FullRedraws()
}

func (t *TUI) SetFocus(component Component) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.setFocusLocked(component)
}

func (t *TUI) setFocusLocked(component Component) {
	if f, ok := t.focusedComponent.(Focusable); ok {
		f.SetFocused(false)
	}
	t.focusedComponent = component
	if f, ok := component.(Focusable); ok {
		f.SetFocused(true)
	}
}

func (t *TUI) FocusedComponent() Component {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.focusedComponent
}

func (t *TUI) ShowOverlay(component Component, options ...OverlayOptions) OverlayHandle {
	t.mu.Lock()
	defer t.mu.Unlock()
	opts := OverlayOptions{Anchor: OverlayCenter}
	if len(options) > 0 {
		opts = options[0]
		if opts.Anchor == "" {
			opts.Anchor = OverlayCenter
		}
	}
	entry := &overlayEntry{component: component, options: opts, preFocus: t.focusedComponent, focusOrder: t.nextFocusOrder()}
	t.overlays = append(t.overlays, entry)
	if !opts.NonCapturing && t.isOverlayVisibleLocked(entry) {
		t.setFocusLocked(component)
	}
	_ = t.terminal.HideCursor()
	t.requestRenderLocked(false)
	return &overlayHandle{t: t, entry: entry}
}

func (t *TUI) HideOverlay() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.overlays) == 0 {
		return
	}
	entry := t.overlays[len(t.overlays)-1]
	t.removeOverlayLocked(entry)
	t.requestRenderLocked(false)
}

func (t *TUI) HasOverlay() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, entry := range t.overlays {
		if t.isOverlayVisibleLocked(entry) {
			return true
		}
	}
	return false
}

func (t *TUI) removeOverlayLocked(entry *overlayEntry) {
	for i, overlay := range t.overlays {
		if overlay == entry {
			t.overlays = append(t.overlays[:i], t.overlays[i+1:]...)
			break
		}
	}
	if t.focusedComponent == entry.component {
		if top := t.topVisibleOverlayLocked(); top != nil {
			t.setFocusLocked(top.component)
		} else {
			t.setFocusLocked(entry.preFocus)
		}
	}
}

func (t *TUI) containsOverlayLocked(entry *overlayEntry) bool {
	for _, overlay := range t.overlays {
		if overlay == entry {
			return true
		}
	}
	return false
}

func (t *TUI) overlayForComponentLocked(component Component) *overlayEntry {
	if component == nil {
		return nil
	}
	for _, overlay := range t.overlays {
		if overlay.component == component {
			return overlay
		}
	}
	return nil
}

func (t *TUI) topVisibleOverlayLocked() *overlayEntry {
	for i := len(t.overlays) - 1; i >= 0; i-- {
		if !t.overlays[i].options.NonCapturing && t.isOverlayVisibleLocked(t.overlays[i]) {
			return t.overlays[i]
		}
	}
	return nil
}

func (t *TUI) isOverlayVisibleLocked(entry *overlayEntry) bool {
	if entry.hidden {
		return false
	}
	if entry.options.Visible != nil {
		return entry.options.Visible(t.terminal.Columns(), t.terminal.Rows())
	}
	return true
}

func (t *TUI) nextFocusOrder() int {
	t.focusCounter++
	return t.focusCounter
}

func (t *TUI) Start() {
	t.mu.Lock()
	t.stopped = false
	t.mu.Unlock()
	t.terminal.Start(func(data string) { t.HandleInput(data) }, func() { t.RequestRender(false) })
	_ = t.terminal.HideCursor()
	t.queryCellSize()
	t.RequestRender(false)
}

func (t *TUI) queryCellSize() {
	if GetCapabilities().Images {
		_ = t.terminal.Write("\x1b[16t")
	}
}

func (t *TUI) Stop() {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return
	}
	t.stopped = true
	lines := len(t.previousLines)
	hardwareCursorRow := t.hardwareCursorRow
	t.mu.Unlock()
	if lines > 0 {
		_ = t.terminal.MoveBy(lines - hardwareCursorRow)
		_ = t.terminal.Write("\r\n")
	}
	_ = t.terminal.ShowCursor()
	t.terminal.Stop()
}

func (t *TUI) DrainInput(maxDuration, idle time.Duration) error {
	return t.terminal.DrainInput(maxDuration, idle)
}

func (t *TUI) AddInputListener(listener InputListener) func() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextListenerID++
	id := t.nextListenerID
	t.listeners = append(t.listeners, inputListenerEntry{id: id, listener: listener})
	return func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		t.removeInputListenerByIDLocked(id)
	}
}

func (t *TUI) RemoveInputListener(listener InputListener) {
	t.mu.Lock()
	defer t.mu.Unlock()
	target := reflect.ValueOf(listener).Pointer()
	for idx, candidate := range t.listeners {
		if reflect.ValueOf(candidate.listener).Pointer() == target {
			t.listeners = append(t.listeners[:idx], t.listeners[idx+1:]...)
			return
		}
	}
}

func (t *TUI) removeInputListenerByIDLocked(id int) {
	for idx, candidate := range t.listeners {
		if candidate.id == id {
			t.listeners = append(t.listeners[:idx], t.listeners[idx+1:]...)
			return
		}
	}
}

func (t *TUI) HandleInput(data string) {
	t.mu.Lock()
	listeners := make([]InputListener, 0, len(t.listeners))
	for _, listener := range t.listeners {
		listeners = append(listeners, listener.listener)
	}
	focused := t.focusedComponent
	t.mu.Unlock()
	for _, listener := range listeners {
		result := listener(data)
		if result.HasData || result.Data != "" {
			data = result.Data
		}
		if result.Consume {
			return
		}
	}
	if data == "" {
		return
	}
	t.mu.Lock()
	onDebug := t.onDebug
	t.mu.Unlock()
	if MatchesKey(data, "shift+ctrl+d") && onDebug != nil {
		onDebug()
		return
	}
	if consumed, updated := consumeCellSizeResponse(data); consumed {
		if updated {
			t.Invalidate()
			t.RequestRender(false)
		}
		return
	}
	t.mu.Lock()
	focused = t.focusedComponent
	if focusedOverlay := t.overlayForComponentLocked(focused); focusedOverlay != nil && !t.isOverlayVisibleLocked(focusedOverlay) {
		if top := t.topVisibleOverlayLocked(); top != nil {
			t.setFocusLocked(top.component)
		} else {
			t.setFocusLocked(focusedOverlay.preFocus)
		}
		focused = t.focusedComponent
	}
	t.mu.Unlock()
	if handler, ok := focused.(InputHandler); ok {
		if IsKeyRelease(data) && !wantsKeyRelease(focused) {
			return
		}
		handler.HandleInput(data)
	}
	t.RequestRender(false)
}

func wantsKeyRelease(component Component) bool {
	if receiver, ok := component.(KeyReleaseReceiver); ok {
		return receiver.WantsKeyRelease()
	}
	return false
}

func consumeCellSizeResponse(data string) (consumed, updated bool) {
	if !strings.HasPrefix(data, "\x1b[6;") || !strings.HasSuffix(data, "t") {
		return false, false
	}
	body := strings.TrimSuffix(strings.TrimPrefix(data, "\x1b["), "t")
	parts := strings.Split(body, ";")
	if len(parts) != 3 || parts[0] != "6" {
		return false, false
	}
	height, errH := strconv.Atoi(parts[1])
	width, errW := strconv.Atoi(parts[2])
	if errH != nil || errW != nil || height <= 0 || width <= 0 {
		return true, false
	}
	SetCellDimensions(CellDimensions{Width: width, Height: height})
	return true, true
}

func (t *TUI) SetOnDebug(fn func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onDebug = fn
}

func (t *TUI) Invalidate() {
	t.Container.Invalidate()
	t.mu.Lock()
	overlays := make([]Component, 0, len(t.overlays))
	for _, overlay := range t.overlays {
		overlays = append(overlays, overlay.component)
	}
	t.mu.Unlock()
	for _, component := range overlays {
		component.Invalidate()
	}
}

func (t *TUI) RequestRender(force ...bool) {
	forceRender := false
	if len(force) > 0 {
		forceRender = force[0]
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.requestRenderLocked(forceRender)
}

func (t *TUI) requestRenderLocked(force bool) {
	if t.stopped {
		return
	}
	lines := t.renderLocked()
	width := t.terminal.Columns()
	height := t.terminal.Rows()
	widthChanged := t.previousWidth != 0 && t.previousWidth != width
	heightChanged := t.previousHeight != 0 && t.previousHeight != height && os.Getenv("TERMUX_VERSION") == ""
	shrunk := t.clearOnShrink && len(t.overlays) == 0 && len(lines) < len(t.previousLines)
	previousViewportTop := viewportTopForLineCount(len(t.previousLines), t.previousHeight)
	newViewportTop := viewportTopForLineCount(len(lines), height)
	viewportMovedUp := len(t.previousLines) > 0 && len(lines) < len(t.previousLines) && newViewportTop < previousViewportTop
	firstChanged, _ := changedRange(t.previousLines, lines)
	changedAboveViewport := firstChanged >= 0 && firstChanged < previousViewportTop
	firstRender := len(t.previousLines) == 0
	full := force || firstRender || widthChanged || heightChanged || shrunk || viewportMovedUp || changedAboveViewport
	if !full && equalLines(lines, t.previousLines) {
		output := t.hardwareCursorBuffer(len(lines), height)
		if output != "" {
			_ = t.terminal.Write(output)
			if t.cursorPosition.ok {
				t.hardwareCursorRow = terminalCursorContentRow(lines, height, t.cursorPosition)
			}
		}
		return
	}
	result := renderBuffer{finalRow: t.hardwareCursorRow}
	if full {
		clear := force || widthChanged || heightChanged || shrunk || viewportMovedUp || changedAboveViewport
		result = t.fullRenderBuffer(lines, clear)
		t.fullRedraws++
	} else {
		result = t.diffRenderBuffer(lines)
	}
	output := result.output + t.hardwareCursorBuffer(len(lines), height)
	_ = t.terminal.Write(output)
	if t.cursorPosition.ok {
		t.hardwareCursorRow = terminalCursorContentRow(lines, height, t.cursorPosition)
	} else {
		t.hardwareCursorRow = result.finalRow
	}
	t.previousLines = lines
	t.previousWidth = width
	t.previousHeight = height
}

func terminalCursorContentRow(lines []string, height int, position terminalCursorPosition) int {
	if len(lines) == 0 {
		return 0
	}
	if position.ok {
		return viewportTopForLineCount(len(lines), height) + position.row
	}
	return len(lines) - 1
}

func (t *TUI) fullRenderBuffer(lines []string, clear bool) renderBuffer {
	var b strings.Builder
	b.WriteString("\x1b[?2026h")
	if clear {
		b.WriteString(deleteImageIDsFromLines(t.previousLines))
		b.WriteString("\x1b[2J\x1b[H\x1b[3J")
	}
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\r\n")
		}
		b.WriteString(line)
	}
	b.WriteString("\x1b[?2026l")
	return renderBuffer{output: b.String(), finalRow: max(0, len(lines)-1)}
}

func viewportTopForLineCount(lineCount, height int) int {
	if height <= 0 {
		return 0
	}
	return max(0, lineCount-height)
}

func (t *TUI) diffRenderBuffer(lines []string) renderBuffer {
	if isPureAppend(t.previousLines, lines) {
		return t.appendRenderBuffer(lines)
	}
	first, last := changedRange(t.previousLines, lines)
	if first < 0 {
		return renderBuffer{finalRow: t.hardwareCursorRow}
	}
	last = max(last, lastKittyImageLineFrom(t.previousLines, first))
	var b strings.Builder
	b.WriteString("\x1b[?2026h")
	if first < len(t.previousLines) {
		b.WriteString(deleteImageIDsFromLines(t.previousLines[first:min(last+1, len(t.previousLines))]))
	}
	b.WriteString("\x1b[H")
	moveToLine(&b, first)
	renderEnd := min(last, len(lines)-1)
	finalRow := max(0, renderEnd)
	if first < len(lines) {
		for i := first; i <= renderEnd; i++ {
			if i > first {
				b.WriteString("\r\n")
			}
			b.WriteString("\x1b[2K\r")
			b.WriteString(lines[i])
		}
	}
	if len(t.previousLines) > len(lines) {
		if first >= len(lines) {
			for i := first; i <= min(last, len(t.previousLines)-1); i++ {
				if i > first {
					b.WriteString("\r\n")
				}
				b.WriteString("\x1b[2K\r")
			}
		} else {
			for i := len(lines); i <= min(last, len(t.previousLines)-1); i++ {
				b.WriteString("\r\n\x1b[2K")
			}
		}
		extraLines := max(0, min(last, len(t.previousLines)-1)-max(first, len(lines))+1)
		if extraLines > 0 {
			b.WriteString("\x1b[")
			b.WriteString(strconv.Itoa(extraLines))
			b.WriteString("A")
			finalRow = max(0, len(lines)-1)
		}
	}
	b.WriteString("\x1b[?2026l")
	return renderBuffer{output: b.String(), finalRow: finalRow}
}

func (t *TUI) appendRenderBuffer(lines []string) renderBuffer {
	first := len(t.previousLines)
	if first == 0 || first >= len(lines) {
		return renderBuffer{finalRow: t.hardwareCursorRow}
	}
	targetRow := first - 1
	lineDelta := targetRow - t.hardwareCursorRow
	var b strings.Builder
	b.WriteString("\x1b[?2026h")
	if lineDelta > 0 {
		b.WriteString("\x1b[")
		b.WriteString(strconv.Itoa(lineDelta))
		b.WriteString("B")
	} else if lineDelta < 0 {
		b.WriteString("\x1b[")
		b.WriteString(strconv.Itoa(-lineDelta))
		b.WriteString("A")
	}
	b.WriteString("\r\n")
	for i := first; i < len(lines); i++ {
		if i > first {
			b.WriteString("\r\n")
		}
		b.WriteString("\x1b[2K")
		b.WriteString(lines[i])
	}
	b.WriteString("\x1b[?2026l")
	return renderBuffer{output: b.String(), finalRow: len(lines) - 1}
}

func moveToLine(b *strings.Builder, row int) {
	if row <= 0 {
		return
	}
	b.WriteString("\x1b[")
	b.WriteString(strconv.Itoa(row))
	b.WriteString("B")
}

func isPureAppend(oldLines, newLines []string) bool {
	if len(newLines) <= len(oldLines) {
		return false
	}
	for i := range oldLines {
		if oldLines[i] != newLines[i] {
			return false
		}
	}
	return true
}

func changedRange(oldLines, newLines []string) (int, int) {
	maxLine := max(len(oldLines), len(newLines))
	first, last := -1, -1
	for i := 0; i < maxLine; i++ {
		var oldLine, newLine string
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}
		if oldLine != newLine {
			if first == -1 {
				first = i
			}
			last = i
		}
	}
	return first, last
}

func lastKittyImageLineFrom(lines []string, first int) int {
	last := -1
	for idx := max(0, first); idx < len(lines); idx++ {
		if len(extractKittyImageIDs(lines[idx])) > 0 {
			last = idx
		}
	}
	return last
}

func deleteImageIDsFromLines(lines []string) string {
	seen := map[uint32]bool{}
	var b strings.Builder
	for _, line := range lines {
		for _, id := range extractKittyImageIDs(line) {
			if !seen[id] {
				b.WriteString(DeleteKittyImage(id))
				seen[id] = true
			}
		}
	}
	return b.String()
}

func extractKittyImageIDs(line string) []uint32 {
	const prefix = "\x1b_G"
	var ids []uint32
	for {
		start := strings.Index(line, prefix)
		if start < 0 {
			return ids
		}
		line = line[start+len(prefix):]
		end := strings.Index(line, ";")
		if end < 0 {
			return ids
		}
		for _, param := range strings.Split(line[:end], ",") {
			if !strings.HasPrefix(param, "i=") {
				continue
			}
			raw := strings.TrimPrefix(param, "i=")
			parsed, err := strconv.ParseUint(raw, 10, 32)
			if err == nil && parsed > 0 {
				ids = append(ids, uint32(parsed))
			}
		}
		line = line[end+1:]
	}
}

func (t *TUI) renderLocked() []string {
	width := max(1, t.terminal.Columns())
	height := max(1, t.terminal.Rows())
	lines := t.Container.RenderWithSize(width, height)
	overlays := make([]*overlayEntry, 0, len(t.overlays))
	for _, overlay := range t.overlays {
		if !t.isOverlayVisibleLocked(overlay) {
			continue
		}
		overlays = append(overlays, overlay)
	}
	sort.SliceStable(overlays, func(i, j int) bool {
		return overlays[i].focusOrder < overlays[j].focusOrder
	})
	rendered := make([]renderedOverlay, 0, len(overlays))
	workingHeight := max(len(lines), height)
	for _, overlay := range overlays {
		overlayWidth := resolveOverlayWidth(overlay.options, width)
		overlayLines := renderComponent(overlay.component, overlayWidth, height)
		if maxHeight, ok := resolveOverlayMaxHeight(overlay.options, height); ok && len(overlayLines) > maxHeight {
			overlayLines = overlayLines[:maxHeight]
		}
		row, col := resolveOverlayPosition(overlay.options, width, height, overlayWidth, len(overlayLines))
		rendered = append(rendered, renderedOverlay{lines: overlayLines, row: row, col: col, width: overlayWidth})
		workingHeight = max(workingHeight, row+len(overlayLines))
	}
	if len(rendered) > 0 {
		for len(lines) < workingHeight {
			lines = append(lines, "")
		}
		viewportStart := max(0, workingHeight-height)
		for _, overlay := range rendered {
			lines = renderOverlay(lines, overlay.lines, width, viewportStart+overlay.row, overlay.col, overlay.width)
		}
	}
	t.cursorPosition = extractTerminalCursorPosition(lines, width, height)
	for i := range lines {
		lines[i] = strings.ReplaceAll(lines[i], CursorMarker, "")
		if VisibleWidth(lines[i]) > width {
			if !IsImageLine(lines[i]) {
				panic(renderedLineWidthError(i, lines[i], width))
			}
		}
		if !IsImageLine(lines[i]) {
			lines[i] = NormalizeTerminalOutput(lines[i]) + segmentReset
		}
	}
	return lines
}

func renderedLineWidthError(index int, line string, width int) string {
	return "rendered line " + strconv.Itoa(index) + " exceeds terminal width (" + strconv.Itoa(VisibleWidth(line)) + " > " + strconv.Itoa(width) + "). Use VisibleWidth and TruncateToWidth before returning custom component lines."
}

func extractTerminalCursorPosition(lines []string, width, height int) terminalCursorPosition {
	viewportTop := max(0, len(lines)-max(1, height))
	position := terminalCursorPosition{}
	for row := len(lines) - 1; row >= 0; row-- {
		markerIndex := strings.Index(lines[row], CursorMarker)
		if markerIndex < 0 {
			continue
		}
		if row >= viewportTop && !position.ok {
			position = terminalCursorPosition{
				row: row - viewportTop,
				col: min(max(0, width-1), VisibleWidth(lines[row][:markerIndex])),
				ok:  true,
			}
		}
	}
	return position
}

func (t *TUI) hardwareCursorBuffer(lineCount, height int) string {
	if !t.cursorPosition.ok {
		return "\x1b[?25l"
	}
	row := max(0, min(t.cursorPosition.row, max(0, min(lineCount, height)-1)))
	col := max(0, t.cursorPosition.col)
	var b strings.Builder
	b.WriteString("\x1b[")
	b.WriteString(strconv.Itoa(row + 1))
	b.WriteString(";")
	b.WriteString(strconv.Itoa(col + 1))
	b.WriteString("H")
	if t.showCursor {
		b.WriteString("\x1b[?25h")
	} else {
		b.WriteString("\x1b[?25l")
	}
	return b.String()
}

func renderOverlay(base, overlay []string, termWidth, row, col, overlayWidth int) []string {
	if len(overlay) == 0 {
		return base
	}
	out := append([]string(nil), base...)
	for len(out) < row {
		out = append(out, "")
	}
	for i, line := range overlay {
		target := row + i
		for len(out) <= target {
			out = append(out, "")
		}
		if VisibleWidth(line) > overlayWidth {
			line = SliceByColumn(line, 0, overlayWidth, true)
		}
		out[target] = compositeLineAt(out[target], line, col, overlayWidth, termWidth)
	}
	return out
}

func compositeLineAt(baseLine, overlayLine string, startCol, overlayWidth, totalWidth int) string {
	if IsImageLine(baseLine) {
		return baseLine
	}
	afterStart := startCol + overlayWidth
	base := ExtractSegments(baseLine, startCol, afterStart, max(0, totalWidth-afterStart), true)
	overlay := SliceWithWidth(overlayLine, 0, overlayWidth, true)
	beforePad := max(0, startCol-base.BeforeWidth)
	overlayPad := max(0, overlayWidth-overlay.Width)
	actualBeforeWidth := max(startCol, base.BeforeWidth)
	actualOverlayWidth := max(overlayWidth, overlay.Width)
	afterTarget := max(0, totalWidth-actualBeforeWidth-actualOverlayWidth)
	afterPad := max(0, afterTarget-base.AfterWidth)
	result := base.Before +
		strings.Repeat(" ", beforePad) +
		segmentReset +
		overlay.Text +
		strings.Repeat(" ", overlayPad) +
		segmentReset +
		base.After +
		strings.Repeat(" ", afterPad)
	if VisibleWidth(result) <= totalWidth {
		return result
	}
	return SliceByColumn(result, 0, totalWidth, true)
}

func resolveOverlayWidth(opts OverlayOptions, termWidth int) int {
	margin := normalizeOverlayMargin(opts.Margin)
	available := max(1, termWidth-margin.Left-margin.Right)
	width := min(80, available)
	if opts.Width != nil {
		width = resolveSize(*opts.Width, termWidth)
	}
	if opts.MinWidth > 0 {
		width = max(width, opts.MinWidth)
	}
	return max(1, min(width, available))
}

func resolveOverlayMaxHeight(opts OverlayOptions, termHeight int) (int, bool) {
	if opts.MaxHeight == nil {
		return 0, false
	}
	margin := normalizeOverlayMargin(opts.Margin)
	available := max(1, termHeight-margin.Top-margin.Bottom)
	height := resolveSize(*opts.MaxHeight, termHeight)
	return max(1, min(height, available)), true
}

func resolveOverlayPosition(opts OverlayOptions, termWidth, termHeight, width, height int) (int, int) {
	margin := normalizeOverlayMargin(opts.Margin)
	availableWidth := max(1, termWidth-margin.Left-margin.Right)
	availableHeight := max(1, termHeight-margin.Top-margin.Bottom)
	maxRow := max(0, availableHeight-height)
	maxCol := max(0, availableWidth-width)
	row := margin.Top + maxRow/2
	col := margin.Left + maxCol/2
	switch opts.Anchor {
	case OverlayTopLeft:
		row, col = margin.Top, margin.Left
	case OverlayTopRight:
		row, col = margin.Top, margin.Left+maxCol
	case OverlayBottomLeft:
		row, col = margin.Top+maxRow, margin.Left
	case OverlayBottomRight:
		row, col = margin.Top+maxRow, margin.Left+maxCol
	case OverlayTopCenter:
		row = margin.Top
	case OverlayBottomCenter:
		row = margin.Top + maxRow
	case OverlayLeftCenter:
		col = margin.Left
	case OverlayRightCenter:
		col = margin.Left + maxCol
	}
	if opts.Row != nil {
		if opts.Row.Percent {
			row = margin.Top + percentSize(*opts.Row, maxRow)
		} else {
			row = opts.Row.Value
		}
	}
	if opts.Col != nil {
		if opts.Col.Percent {
			col = margin.Left + percentSize(*opts.Col, maxCol)
		} else {
			col = opts.Col.Value
		}
	}
	row += opts.OffsetY
	col += opts.OffsetX
	row = max(margin.Top, min(row, margin.Top+maxRow))
	col = max(margin.Left, min(col, margin.Left+maxCol))
	return row, col
}

func normalizeOverlayMargin(m OverlayMargin) OverlayMargin {
	return OverlayMargin{
		Top:    max(0, m.Top),
		Right:  max(0, m.Right),
		Bottom: max(0, m.Bottom),
		Left:   max(0, m.Left),
	}
}

func resolveSize(value SizeValue, reference int) int {
	if value.Percent {
		return percentSize(value, reference)
	}
	return value.Value
}

func percentSize(value SizeValue, reference int) int {
	percent := value.PercentValue
	if percent == 0 && value.Value != 0 {
		percent = float64(value.Value)
	}
	return int(math.Floor(float64(reference) * percent / 100))
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sliceByWidth(s string, start, width int) string {
	if width <= 0 {
		return ""
	}
	current := 0
	var b strings.Builder
	for _, r := range stripANSI(s) {
		w := runeWidth(r)
		if current+w > start && current < start+width {
			b.WriteRune(r)
		}
		current += w
	}
	return b.String()
}

func (t *TUI) SetClearOnShrink(enabled bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.clearOnShrink = enabled
}
func (t *TUI) ClearOnShrink() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.clearOnShrink
}
func (t *TUI) GetClearOnShrink() bool { return t.ClearOnShrink() }
func (t *TUI) SetShowHardwareCursor(enabled bool) {
	t.mu.Lock()
	if t.showCursor == enabled {
		t.mu.Unlock()
		return
	}
	t.showCursor = enabled
	terminal := t.terminal
	t.mu.Unlock()
	if !enabled {
		_ = terminal.HideCursor()
	}
	t.RequestRender(false)
}
func (t *TUI) ShowHardwareCursor() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.showCursor
}
func (t *TUI) GetShowHardwareCursor() bool {
	return t.ShowHardwareCursor()
}

var _ Component = (*Container)(nil)
