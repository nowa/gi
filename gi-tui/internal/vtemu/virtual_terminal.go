package vtemu

import (
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// VirtualTerminal is a small deterministic terminal emulator for tests and
// headless consumers. It implements enough VT behavior for TUI viewport,
// cursor movement, line clearing, and input-routing tests without depending on
// a native terminal.
type VirtualTerminal struct {
	mu              sync.Mutex
	columns         int
	rows            int
	lines           []string
	cells           [][]VirtualCell
	wrapped         []bool
	cursorX         int
	cursorY         int
	savedX          int
	savedY          int
	savedOK         bool
	savedStyle      virtualStyle
	savedOriginMode bool
	savedWraparound bool
	savedG0Charset  string
	savedG1Charset  string
	savedUseG1      bool
	scrollTop       int
	scrollBottom    int
	hasScrollRegion bool
	insertMode      bool
	originMode      bool
	wraparoundMode  bool
	pendingWrap     bool
	cursorVisible   bool
	style           virtualStyle
	lastPrintable   rune
	tabStopsCleared bool
	tabStops        map[int]bool
	clearedTabStops map[int]bool
	input           func(string)
	pendingInput    []string
	resize          func()
	stopped         bool
	output          strings.Builder
	title           string
	kitty           bool
	alternateScreen bool
	normalScreen    *virtualScreenState
	g0Charset       string
	g1Charset       string
	useG1Charset    bool
}

type virtualStyle struct {
	bold           bool
	dim            bool
	italic         bool
	underline      bool
	underlineStyle string
	inverse        bool
	strikethrough  bool
	blink          bool
	conceal        bool
	overline       bool
	foreground     VirtualColor
	background     VirtualColor
	underlineColor VirtualColor
}

type virtualScreenState struct {
	lines           []string
	cells           [][]VirtualCell
	wrapped         []bool
	cursorX         int
	cursorY         int
	savedX          int
	savedY          int
	savedOK         bool
	savedStyle      virtualStyle
	savedOriginMode bool
	savedWraparound bool
	savedG0Charset  string
	savedG1Charset  string
	savedUseG1      bool
	scrollTop       int
	scrollBottom    int
	hasScrollRegion bool
	insertMode      bool
	originMode      bool
	wraparoundMode  bool
	pendingWrap     bool
	cursorVisible   bool
	style           virtualStyle
	lastPrintable   rune
	tabStopsCleared bool
	tabStops        map[int]bool
	clearedTabStops map[int]bool
	g0Charset       string
	g1Charset       string
	useG1Charset    bool
}

// VirtualColor is a captured SGR color value for a terminal cell.
// Kind is empty when the color is unset, "ansi" for 16-color SGR indices,
// "index" for 256-color palette indices, and "rgb" for truecolor values.
type VirtualColor struct {
	Kind  string
	Index int
	R     int
	G     int
	B     int
}

// VirtualCell exposes the style state captured for a terminal cell.
type VirtualCell struct {
	Rune           rune
	Bold           bool
	Dim            bool
	Italic         bool
	Underline      bool
	UnderlineStyle string
	Inverse        bool
	Strikethrough  bool
	Blink          bool
	Conceal        bool
	Overline       bool
	Foreground     VirtualColor
	Background     VirtualColor
	UnderlineColor VirtualColor
}

func New(columns, rows int) *VirtualTerminal {
	if columns <= 0 {
		columns = 80
	}
	if rows <= 0 {
		rows = 24
	}
	return &VirtualTerminal{
		columns:         columns,
		rows:            rows,
		lines:           make([]string, rows),
		cells:           make([][]VirtualCell, rows),
		wrapped:         make([]bool, rows),
		tabStops:        map[int]bool{},
		clearedTabStops: map[int]bool{},
		kitty:           true,
		wraparoundMode:  true,
		cursorVisible:   true,
		g0Charset:       "ascii",
		g1Charset:       "ascii",
	}
}

func NewVirtualTerminal(columns, rows int) *VirtualTerminal {
	return New(columns, rows)
}

func (v *VirtualTerminal) Start(onInput func(string), onResize func()) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.input = onInput
	v.resize = onResize
	v.stopped = false
	v.writeLocked("\x1b[?2004h")
}

func (v *VirtualTerminal) Stop() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.writeLocked("\x1b[?2004l")
	v.input = nil
	v.resize = nil
	v.stopped = true
}

func (v *VirtualTerminal) DrainInput(_, _ time.Duration) error { return nil }

func (v *VirtualTerminal) Write(data string) error {
	v.mu.Lock()
	v.writeLocked(data)
	pending := append([]string(nil), v.pendingInput...)
	v.pendingInput = nil
	input := v.input
	v.mu.Unlock()
	if input != nil {
		for _, data := range pending {
			input(data)
		}
	}
	return nil
}

func (v *VirtualTerminal) Columns() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.columns
}

func (v *VirtualTerminal) Rows() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.rows
}

func (v *VirtualTerminal) KittyProtocolActive() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.kitty
}

func (v *VirtualTerminal) MoveBy(lines int) error {
	if lines > 0 {
		return v.Write("\x1b[" + strconv.Itoa(lines) + "B")
	}
	if lines < 0 {
		return v.Write("\x1b[" + strconv.Itoa(-lines) + "A")
	}
	return nil
}

func (v *VirtualTerminal) HideCursor() error      { return v.Write("\x1b[?25l") }
func (v *VirtualTerminal) ShowCursor() error      { return v.Write("\x1b[?25h") }
func (v *VirtualTerminal) ClearLine() error       { return v.Write("\x1b[K") }
func (v *VirtualTerminal) ClearFromCursor() error { return v.Write("\x1b[J") }
func (v *VirtualTerminal) ClearScreen() error     { return v.Write("\x1b[2J\x1b[H") }
func (v *VirtualTerminal) SetTitle(title string) error {
	return v.Write("\x1b]0;" + title + "\x07")
}
func (v *VirtualTerminal) SetProgress(bool) error { return nil }

func (v *VirtualTerminal) SendInput(data string) {
	v.mu.Lock()
	input := v.input
	v.mu.Unlock()
	if input != nil {
		input(data)
	}
}

func (v *VirtualTerminal) Resize(columns, rows int) {
	if columns <= 0 {
		columns = 1
	}
	if rows <= 0 {
		rows = 1
	}
	v.mu.Lock()
	oldStart, _ := v.displayRangeLocked()
	hadScrollRegion := v.hasScrollRegion
	scrollRegionTop := v.scrollTop - oldStart
	scrollRegionBottom := v.scrollBottom - oldStart
	oldColumns := v.columns
	v.columns = columns
	v.rows = rows
	v.ensureWrappedLocked()
	if oldColumns != columns {
		v.reflowSoftWrappedLinesLocked(columns)
	}
	for len(v.lines) < rows {
		v.lines = append(v.lines, "")
		v.cells = append(v.cells, nil)
		v.wrapped = append(v.wrapped, false)
	}
	v.ensureWrappedLocked()
	if hadScrollRegion {
		newStart, _ := v.displayRangeLocked()
		if scrollRegionTop < 0 || scrollRegionBottom >= rows || scrollRegionTop >= scrollRegionBottom {
			v.hasScrollRegion = false
			v.scrollTop = 0
			v.scrollBottom = 0
		} else {
			v.hasScrollRegion = true
			v.scrollTop = newStart + scrollRegionTop
			v.scrollBottom = newStart + scrollRegionBottom
		}
	}
	resize := v.resize
	v.mu.Unlock()
	if resize != nil {
		resize()
	}
}

func (v *VirtualTerminal) GetViewport() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	start := max(0, len(v.lines)-v.rows)
	out := make([]string, 0, v.rows)
	for i := 0; i < v.rows; i++ {
		idx := start + i
		if idx >= 0 && idx < len(v.lines) {
			out = append(out, strings.TrimRight(v.lines[idx], " "))
		} else {
			out = append(out, "")
		}
	}
	return out
}

func (v *VirtualTerminal) Flush() {}

func (v *VirtualTerminal) FlushAndGetViewport() []string {
	v.Flush()
	return v.GetViewport()
}

func (v *VirtualTerminal) WaitForRender() {
	v.Flush()
}

func (v *VirtualTerminal) GetScrollBuffer() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]string, len(v.lines))
	for i, line := range v.lines {
		out[i] = strings.TrimRight(line, " ")
	}
	return out
}

func (v *VirtualTerminal) GetCursorPosition() (x, y int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cursorX, v.viewportCursorYLocked()
}

func (v *VirtualTerminal) CursorVisible() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cursorVisible
}

func (v *VirtualTerminal) WindowTitle() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.title
}

func (v *VirtualTerminal) GetCell(row, col int) (VirtualCell, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if row < 0 || col < 0 || row >= v.rows {
		return VirtualCell{}, false
	}
	start := max(0, len(v.lines)-v.rows)
	y := start + row
	if y < 0 || y >= len(v.cells) {
		return VirtualCell{}, false
	}
	if col >= len(v.cells[y]) {
		return VirtualCell{Rune: ' '}, true
	}
	cell := v.cells[y][col]
	if cell.Rune == 0 {
		cell.Rune = ' '
	}
	return cell, true
}

func (v *VirtualTerminal) Output() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.output.String()
}

func (v *VirtualTerminal) ClearOutput() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.output.Reset()
}

func (v *VirtualTerminal) Clear() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.clearBufferLocked()
}

func (v *VirtualTerminal) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.resetStateLocked()
	v.output.Reset()
}

func (v *VirtualTerminal) resetStateLocked() {
	v.lines = make([]string, v.rows)
	v.cells = make([][]VirtualCell, v.rows)
	v.wrapped = make([]bool, v.rows)
	v.cursorX = 0
	v.cursorY = 0
	v.savedX = 0
	v.savedY = 0
	v.savedOK = false
	v.savedStyle = virtualStyle{}
	v.savedOriginMode = false
	v.savedWraparound = true
	v.savedG0Charset = "ascii"
	v.savedG1Charset = "ascii"
	v.savedUseG1 = false
	v.scrollTop = 0
	v.scrollBottom = 0
	v.hasScrollRegion = false
	v.insertMode = false
	v.originMode = false
	v.wraparoundMode = true
	v.pendingWrap = false
	v.cursorVisible = true
	v.style = virtualStyle{}
	v.lastPrintable = 0
	v.tabStopsCleared = false
	v.tabStops = map[int]bool{}
	v.clearedTabStops = map[int]bool{}
	v.title = ""
	v.alternateScreen = false
	v.normalScreen = nil
	v.g0Charset = "ascii"
	v.g1Charset = "ascii"
	v.useG1Charset = false
}

func (v *VirtualTerminal) softResetLocked() {
	displayStart, _ := v.displayRangeLocked()
	v.savedX = 0
	v.savedY = displayStart
	v.savedOK = true
	v.savedStyle = virtualStyle{}
	v.savedOriginMode = false
	v.savedWraparound = true
	v.savedG0Charset = "ascii"
	v.savedG1Charset = "ascii"
	v.savedUseG1 = false
	v.scrollTop = 0
	v.scrollBottom = 0
	v.hasScrollRegion = false
	v.insertMode = false
	v.originMode = false
	v.wraparoundMode = true
	v.pendingWrap = false
	v.cursorVisible = true
	v.style = virtualStyle{}
	v.lastPrintable = 0
	v.g0Charset = "ascii"
	v.g1Charset = "ascii"
	v.useG1Charset = false
}

func (v *VirtualTerminal) snapshotScreenLocked() virtualScreenState {
	return virtualScreenState{
		lines:           append([]string(nil), v.lines...),
		cells:           cloneVirtualCells(v.cells),
		wrapped:         append([]bool(nil), v.wrapped...),
		cursorX:         v.cursorX,
		cursorY:         v.cursorY,
		savedX:          v.savedX,
		savedY:          v.savedY,
		savedOK:         v.savedOK,
		savedStyle:      v.savedStyle,
		savedOriginMode: v.savedOriginMode,
		savedWraparound: v.savedWraparound,
		savedG0Charset:  v.savedG0Charset,
		savedG1Charset:  v.savedG1Charset,
		savedUseG1:      v.savedUseG1,
		scrollTop:       v.scrollTop,
		scrollBottom:    v.scrollBottom,
		hasScrollRegion: v.hasScrollRegion,
		insertMode:      v.insertMode,
		originMode:      v.originMode,
		wraparoundMode:  v.wraparoundMode,
		pendingWrap:     v.pendingWrap,
		cursorVisible:   v.cursorVisible,
		style:           v.style,
		lastPrintable:   v.lastPrintable,
		tabStopsCleared: v.tabStopsCleared,
		tabStops:        cloneBoolMap(v.tabStops),
		clearedTabStops: cloneBoolMap(v.clearedTabStops),
		g0Charset:       v.g0Charset,
		g1Charset:       v.g1Charset,
		useG1Charset:    v.useG1Charset,
	}
}

func (v *VirtualTerminal) restoreScreenLocked(state virtualScreenState) {
	v.lines = append([]string(nil), state.lines...)
	v.cells = cloneVirtualCells(state.cells)
	v.wrapped = append([]bool(nil), state.wrapped...)
	v.ensureWrappedLocked()
	v.cursorX = state.cursorX
	v.cursorY = state.cursorY
	v.savedX = state.savedX
	v.savedY = state.savedY
	v.savedOK = state.savedOK
	v.savedStyle = state.savedStyle
	v.savedOriginMode = state.savedOriginMode
	v.savedWraparound = state.savedWraparound
	v.savedG0Charset = state.savedG0Charset
	v.savedG1Charset = state.savedG1Charset
	v.savedUseG1 = state.savedUseG1
	v.scrollTop = state.scrollTop
	v.scrollBottom = state.scrollBottom
	v.hasScrollRegion = state.hasScrollRegion
	v.insertMode = state.insertMode
	v.originMode = state.originMode
	v.wraparoundMode = state.wraparoundMode
	v.pendingWrap = state.pendingWrap
	v.cursorVisible = state.cursorVisible
	v.style = state.style
	v.lastPrintable = state.lastPrintable
	v.tabStopsCleared = state.tabStopsCleared
	v.tabStops = cloneBoolMap(state.tabStops)
	v.clearedTabStops = cloneBoolMap(state.clearedTabStops)
	v.g0Charset = state.g0Charset
	v.g1Charset = state.g1Charset
	v.useG1Charset = state.useG1Charset
}

func cloneVirtualCells(cells [][]VirtualCell) [][]VirtualCell {
	if cells == nil {
		return nil
	}
	out := make([][]VirtualCell, len(cells))
	for i := range cells {
		out[i] = append([]VirtualCell(nil), cells[i]...)
	}
	return out
}

func cloneBoolMap(m map[int]bool) map[int]bool {
	if m == nil {
		return nil
	}
	out := make(map[int]bool, len(m))
	for key, value := range m {
		out[key] = value
	}
	return out
}

func (v *VirtualTerminal) ensureWrappedLocked() {
	for len(v.wrapped) < len(v.lines) {
		v.wrapped = append(v.wrapped, false)
	}
	if len(v.wrapped) > len(v.lines) {
		v.wrapped = v.wrapped[:len(v.lines)]
	}
}

func (v *VirtualTerminal) setLineWrappedLocked(row int, wrapped bool) {
	if row < 0 {
		return
	}
	v.ensureWrappedLocked()
	for row >= len(v.wrapped) {
		v.wrapped = append(v.wrapped, false)
	}
	v.wrapped[row] = wrapped
}

type virtualStyledCluster struct {
	text  string
	width int
	cells []VirtualCell
}

func (v *VirtualTerminal) reflowSoftWrappedLinesLocked(width int) {
	if width <= 0 {
		width = 1
	}
	v.ensureWrappedLocked()
	oldLines := append([]string(nil), v.lines...)
	oldCells := cloneVirtualCells(v.cells)
	oldWrapped := append([]bool(nil), v.wrapped...)
	oldCursorX, oldCursorY := v.cursorX, v.cursorY
	effectiveLen := len(oldLines)
	for effectiveLen > 1 && effectiveLen-1 > oldCursorY && oldLines[effectiveLen-1] == "" && !oldWrapped[effectiveLen-1] {
		effectiveLen--
	}
	oldLines = oldLines[:effectiveLen]
	oldCells = oldCells[:effectiveLen]
	oldWrapped = oldWrapped[:effectiveLen]

	newLines := make([]string, 0, len(oldLines))
	newCells := make([][]VirtualCell, 0, len(oldCells))
	newWrapped := make([]bool, 0, len(oldWrapped))
	cursorUpdated := false

	for start := 0; start < len(oldLines); {
		end := start + 1
		for end < len(oldLines) && oldWrapped[end] {
			end++
		}

		groupStart := len(newLines)
		groupLines, groupCells := v.reflowLineGroupLocked(oldLines[start:end], oldCells[start:end], width)
		for i := range groupLines {
			newLines = append(newLines, groupLines[i])
			newCells = append(newCells, groupCells[i])
			newWrapped = append(newWrapped, i > 0)
		}

		if oldCursorY >= start && oldCursorY < end {
			offset := oldCursorX
			for i := start; i < oldCursorY; i++ {
				offset += visibleWidthPlain(oldLines[i])
			}
			relativeY, relativeX := virtualCursorForOffset(groupLines, offset)
			v.cursorY = groupStart + relativeY
			v.cursorX = relativeX
			cursorUpdated = true
		}

		start = end
	}

	if len(newLines) == 0 {
		newLines = []string{""}
		newCells = [][]VirtualCell{nil}
		newWrapped = []bool{false}
	}
	for len(newLines) < v.rows {
		newLines = append(newLines, "")
		newCells = append(newCells, nil)
		newWrapped = append(newWrapped, false)
	}
	v.lines = newLines
	v.cells = newCells
	v.wrapped = newWrapped
	if !cursorUpdated {
		v.cursorY = max(0, min(oldCursorY, len(v.lines)-1))
		v.cursorX = min(max(0, oldCursorX), width)
	}
}

func (v *VirtualTerminal) reflowLineGroupLocked(lines []string, cells [][]VirtualCell, width int) ([]string, [][]VirtualCell) {
	clusters := make([]virtualStyledCluster, 0)
	for i, line := range lines {
		var lineCells []VirtualCell
		if i < len(cells) {
			lineCells = cells[i]
		}
		clusters = append(clusters, virtualLineStyledClusters(line, lineCells, v.blankCellLocked())...)
	}
	if len(clusters) == 0 {
		return []string{""}, [][]VirtualCell{nil}
	}

	var current strings.Builder
	currentCells := make([]VirtualCell, 0, width)
	currentWidth := 0
	outLines := make([]string, 0, len(lines))
	outCells := make([][]VirtualCell, 0, len(lines))
	flush := func() {
		outLines = append(outLines, current.String())
		outCells = append(outCells, append([]VirtualCell(nil), currentCells...))
		current.Reset()
		currentCells = currentCells[:0]
		currentWidth = 0
	}

	for _, cluster := range clusters {
		if currentWidth > 0 && currentWidth+cluster.width > width {
			flush()
		}
		current.WriteString(cluster.text)
		currentCells = append(currentCells, cluster.cells...)
		currentWidth += cluster.width
	}
	flush()
	return outLines, outCells
}

func virtualLineStyledClusters(line string, cells []VirtualCell, blank VirtualCell) []virtualStyledCluster {
	if line == "" {
		return nil
	}
	runes := []rune(line)
	offsets := runeByteOffsets(line)
	clusters := make([]virtualStyledCluster, 0, len(runes))
	column := 0
	for _, span := range graphemeSpans(runes) {
		width := graphemeWidth(runes[span.start:span.end])
		if width < 0 {
			width = 0
		}
		cellCount := max(1, width)
		clusterCells := make([]VirtualCell, cellCount)
		for i := range clusterCells {
			if column+i < len(cells) {
				clusterCells[i] = cells[column+i]
			} else {
				clusterCells[i] = blank
			}
		}
		clusters = append(clusters, virtualStyledCluster{
			text:  line[offsets[span.start]:offsets[span.end]],
			width: width,
			cells: clusterCells,
		})
		column += width
	}
	return clusters
}

func virtualCursorForOffset(lines []string, offset int) (int, int) {
	offset = max(0, offset)
	for i, line := range lines {
		width := visibleWidthPlain(line)
		if offset <= width || i == len(lines)-1 {
			return i, offset
		}
		offset -= width
	}
	return 0, 0
}

func (v *VirtualTerminal) writeLocked(data string) {
	v.output.WriteString(data)
	for i := 0; i < len(data); {
		if data[i] == '\x1b' {
			if n := v.consumeEscapeLocked(data[i:]); n > 0 {
				i += n
				continue
			}
		}
		if n := v.consumeC1Locked(data[i:]); n > 0 {
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(data[i:])
		if r == utf8.RuneError && size == 0 {
			return
		}
		if r == '\r' {
			v.pendingWrap = false
			v.cursorX = 0
		} else if r == '\n' || r == '\v' || r == '\f' {
			v.lineFeedLocked(false)
		} else if r == '\b' {
			if v.columns > 0 && v.wraparoundMode && (v.cursorX >= v.columns || v.pendingWrap) {
				v.cursorX = max(0, v.columns-2)
			} else {
				v.cursorX = max(0, v.cursorX-1)
			}
			v.pendingWrap = false
		} else if r == '\t' {
			v.pendingWrap = false
			v.cursorX = v.nextTabStopLocked(v.cursorX)
		} else if r == '\x0e' {
			v.useG1Charset = true
		} else if r == '\x0f' {
			v.useG1Charset = false
		} else if r >= 32 {
			v.putRuneLocked(r)
		}
		i += size
	}
}

func (v *VirtualTerminal) enqueueInputLocked(data string) {
	if data == "" {
		return
	}
	v.pendingInput = append(v.pendingInput, data)
}

func (v *VirtualTerminal) setTabStopLocked(col int) {
	col = max(0, col)
	if v.tabStops == nil {
		v.tabStops = map[int]bool{}
	}
	if v.clearedTabStops != nil {
		delete(v.clearedTabStops, col)
	}
	v.tabStops[col] = true
}

func (v *VirtualTerminal) clearTabStopLocked(col int) {
	col = max(0, col)
	if v.tabStops != nil {
		delete(v.tabStops, col)
	}
	if !v.tabStopsCleared && col%8 == 0 {
		if v.clearedTabStops == nil {
			v.clearedTabStops = map[int]bool{}
		}
		v.clearedTabStops[col] = true
	}
}

func (v *VirtualTerminal) isDefaultTabStopLocked(col int) bool {
	if col <= 0 || v.tabStopsCleared || col%8 != 0 {
		return false
	}
	return v.clearedTabStops == nil || !v.clearedTabStops[col]
}

func (v *VirtualTerminal) isTabStopLocked(col int) bool {
	return (v.tabStops != nil && v.tabStops[col]) || v.isDefaultTabStopLocked(col)
}

func (v *VirtualTerminal) nextTabStopLocked(cursorX int) int {
	cursorX = max(0, cursorX)
	limit := max(0, v.columns-1)
	if cursorX >= limit {
		return cursorX
	}
	for col := cursorX + 1; col <= limit; col++ {
		if v.isTabStopLocked(col) {
			return col
		}
	}
	return limit
}

func (v *VirtualTerminal) previousTabStopLocked(cursorX int) int {
	cursorX = max(0, cursorX)
	for col := cursorX - 1; col > 0; col-- {
		if v.isTabStopLocked(col) {
			return col
		}
	}
	return 0
}

func (v *VirtualTerminal) consumeEscapeLocked(data string) int {
	if len(data) < 2 {
		return 0
	}
	switch data[1] {
	case '[':
		for i := 2; i < len(data); i++ {
			if data[i] >= 0x40 && data[i] <= 0x7e {
				v.handleCSILocked(data[2:i], data[i])
				return i + 1
			}
		}
	case ']':
		if payloadEnd, sequenceEnd, ok := controlStringTerminator(data, 2, true); ok {
			v.handleOSCLocked(data[2:payloadEnd])
			return sequenceEnd
		}
	case '_', 'P', '^', 'X':
		if _, sequenceEnd, ok := controlStringTerminator(data, 2, true); ok {
			return sequenceEnd
		}
	case '7':
		v.saveCursorLocked()
		return 2
	case '8':
		v.restoreCursorLocked()
		return 2
	case 'D':
		v.indexLocked()
		return 2
	case 'E':
		v.cursorX = 0
		v.indexLocked()
		return 2
	case 'H':
		v.setTabStopLocked(v.cursorX)
		return 2
	case '#':
		if len(data) < 3 {
			return 0
		}
		if data[2] == '8' {
			v.screenAlignmentTestLocked()
		}
		return 3
	case '(', ')', '*', '+', '-', '.', '/':
		if len(data) < 3 {
			return 0
		}
		v.designateCharsetLocked(data[1], data[2])
		return 3
	case '%':
		if len(data) < 3 {
			return 0
		}
		return 3
	case 'M':
		v.reverseIndexLocked()
		return 2
	case 'c':
		v.resetStateLocked()
		return 2
	default:
		return 2
	}
	return 0
}

func (v *VirtualTerminal) consumeC1Locked(data string) int {
	code, prefixLen, ok := c1Prefix(data)
	if !ok {
		return 0
	}
	switch code {
	case 0x84:
		v.indexLocked()
		return prefixLen
	case 0x85:
		v.cursorX = 0
		v.indexLocked()
		return prefixLen
	case 0x88:
		v.setTabStopLocked(v.cursorX)
		return prefixLen
	case 0x8d:
		v.reverseIndexLocked()
		return prefixLen
	case 0x90, 0x98, 0x9e, 0x9f:
		if n := c1StringTerminatedLength(data, prefixLen, false); n > 0 {
			return n
		}
	case 0x9b:
		for i := prefixLen; i < len(data); i++ {
			if data[i] >= 0x40 && data[i] <= 0x7e {
				v.handleCSILocked(data[prefixLen:i], data[i])
				return i + 1
			}
		}
	case 0x9c:
		return prefixLen
	case 0x9d:
		if n, payload := c1OSCStringLength(data, prefixLen); n > 0 {
			v.handleOSCLocked(payload)
			return n
		}
	}
	return 0
}

func c1Prefix(data string) (byte, int, bool) {
	if data == "" {
		return 0, 0, false
	}
	if len(data) >= 2 && data[0] == 0xc2 && data[1] >= 0x80 && data[1] <= 0x9f {
		return data[1], 2, true
	}
	if data[0] >= 0x80 && data[0] <= 0x9f {
		return data[0], 1, true
	}
	return 0, 0, false
}

func controlStringTerminator(data string, start int, allowBEL bool) (int, int, bool) {
	payloadEnd := -1
	sequenceEnd := 0
	consider := func(candidatePayloadEnd, candidateSequenceEnd int) {
		if candidateSequenceEnd <= 0 {
			return
		}
		if sequenceEnd == 0 || candidateSequenceEnd < sequenceEnd || (candidateSequenceEnd == sequenceEnd && candidatePayloadEnd < payloadEnd) {
			payloadEnd = candidatePayloadEnd
			sequenceEnd = candidateSequenceEnd
		}
	}
	if idx := strings.Index(data[start:], "\xc2\x9c"); idx >= 0 {
		end := start + idx
		consider(end, end+2)
	}
	if idx := strings.IndexByte(data[start:], 0x9c); idx >= 0 {
		end := start + idx
		payloadEnd := end
		if end > start && data[end-1] == 0xc2 {
			payloadEnd = end - 1
		}
		consider(payloadEnd, end+1)
	}
	if idx := strings.Index(data[start:], "\x1b\\"); idx >= 0 {
		end := start + idx
		consider(end, end+len("\x1b\\"))
	}
	if allowBEL {
		if idx := strings.IndexByte(data[start:], '\x07'); idx >= 0 {
			end := start + idx
			consider(end, end+1)
		}
	}
	return payloadEnd, sequenceEnd, sequenceEnd > 0
}

func c1StringTerminatedLength(data string, start int, allowBEL bool) int {
	if _, sequenceEnd, ok := controlStringTerminator(data, start, allowBEL); ok {
		return sequenceEnd
	}
	return 0
}

func c1OSCStringLength(data string, start int) (int, string) {
	payloadEnd, sequenceEnd, ok := controlStringTerminator(data, start, true)
	if !ok {
		return 0, ""
	}
	return sequenceEnd, data[start:payloadEnd]
}

func (v *VirtualTerminal) handleOSCLocked(payload string) {
	command, value, ok := strings.Cut(payload, ";")
	if !ok {
		return
	}
	switch command {
	case "0", "2":
		v.title = value
	}
}

func (v *VirtualTerminal) handleCSILocked(params string, final byte) {
	switch final {
	case '@':
		v.insertBlankCharactersLocked(csiPositiveParam(params, 1))
	case 'A':
		v.cursorY = v.clampCursorYLocked(v.cursorY - csiPositiveParam(params, 1))
	case '`':
		v.cursorX = v.clampCursorXLocked(csiParam(params, 1) - 1)
	case 'a':
		v.cursorX = v.clampCursorXLocked(v.cursorX + csiPositiveParam(params, 1))
	case 'B':
		v.cursorY = v.clampCursorYLocked(v.cursorY + csiPositiveParam(params, 1))
		v.ensureCursorLineLocked()
	case 'b':
		v.repeatLastPrintableLocked(max(1, csiParam(params, 1)))
	case 'C':
		v.cursorX = v.clampCursorXLocked(v.cursorX + csiPositiveParam(params, 1))
	case 'c':
		v.handleDeviceAttributesLocked(params)
	case 'D':
		v.cursorX = max(0, v.cursorX-csiPositiveParam(params, 1))
	case 'E':
		v.cursorY = v.clampCursorYLocked(v.cursorY + csiPositiveParam(params, 1))
		v.cursorX = 0
		v.ensureCursorLineLocked()
	case 'F':
		v.cursorY = v.clampCursorYLocked(v.cursorY - csiPositiveParam(params, 1))
		v.cursorX = 0
	case 'G':
		v.cursorX = v.clampCursorXLocked(csiParam(params, 1) - 1)
	case 'I':
		for i := 0; i < csiPositiveParam(params, 1); i++ {
			v.cursorX = v.nextTabStopLocked(v.cursorX)
		}
	case 'H', 'f':
		row, col := csiRowCol(params)
		v.cursorY = v.absoluteRowLocked(row)
		v.cursorX = v.clampCursorXLocked(col - 1)
		v.ensureCursorLineLocked()
	case 'h':
		v.setModeLocked(params, true)
	case 'l':
		v.setModeLocked(params, false)
	case 'd':
		v.cursorY = v.absoluteRowLocked(csiParam(params, 1))
		v.ensureCursorLineLocked()
	case 'e':
		v.cursorY = v.clampCursorYLocked(v.cursorY + csiPositiveParam(params, 1))
		v.ensureCursorLineLocked()
	case 'J':
		switch csiParam(params, 0) {
		case 2:
			v.eraseWholeDisplayLocked()
		case 3:
			v.clearScrollbackLocked()
		case 1:
			v.eraseDisplayToCursorLocked()
		default:
			v.eraseDisplayFromCursorLocked()
		}
	case 'K':
		v.ensureCursorLineLocked()
		mode := csiParam(params, 0)
		switch mode {
		case 1:
			v.eraseLineToCursorLocked()
		case 2:
			v.eraseWholeLineLocked()
		default:
			v.eraseLineFromCursorLocked()
		}
	case 'L':
		v.insertLinesLocked(csiPositiveParam(params, 1))
	case 'M':
		v.deleteLinesLocked(csiPositiveParam(params, 1))
	case 'm':
		if !strings.HasPrefix(params, ">") {
			v.handleSGRLocked(params)
		}
	case 'n':
		v.handleDSRLocked(params)
	case 'P':
		v.deleteCharactersLocked(csiPositiveParam(params, 1))
	case 'p':
		if strings.HasSuffix(params, "!") {
			v.softResetLocked()
		}
	case 'r':
		v.setScrollRegionLocked(params)
	case 'S':
		v.scrollUpLocked(csiPositiveParam(params, 1))
	case 'T':
		v.scrollDownLocked(csiPositiveParam(params, 1))
	case 'X':
		v.eraseCharactersLocked(csiPositiveParam(params, 1))
	case 'Z':
		for i := 0; i < csiPositiveParam(params, 1); i++ {
			v.cursorX = v.previousTabStopLocked(v.cursorX)
		}
	case 'g':
		if csiParam(params, 0) == 3 {
			v.tabStopsCleared = true
			v.tabStops = map[int]bool{}
			v.clearedTabStops = map[int]bool{}
		} else {
			v.clearTabStopLocked(v.cursorX)
		}
	case 's':
		v.saveCursorLocked()
	case 'u':
		v.restoreCursorLocked()
	}
}

func (v *VirtualTerminal) handleDSRLocked(params string) {
	private := strings.HasPrefix(params, "?")
	params = strings.TrimPrefix(params, "?")
	switch csiParam(params, 0) {
	case 5:
		if !private {
			v.enqueueInputLocked("\x1b[0n")
		}
	case 6:
		prefix := "\x1b["
		if private {
			prefix = "\x1b[?"
		}
		v.enqueueInputLocked(prefix + strconv.Itoa(v.viewportCursorYLocked()+1) + ";" + strconv.Itoa(v.cursorX+1) + "R")
	}
}

func (v *VirtualTerminal) handleDeviceAttributesLocked(params string) {
	if strings.HasPrefix(params, ">") {
		params = strings.TrimPrefix(params, ">")
		if csiParam(params, 0) == 0 {
			v.enqueueInputLocked("\x1b[>0;276;0c")
		}
		return
	}
	if csiParam(params, 0) == 0 {
		v.enqueueInputLocked("\x1b[?1;2c")
	}
}

func (v *VirtualTerminal) repeatLastPrintableLocked(count int) {
	if v.lastPrintable == 0 || count <= 0 {
		return
	}
	for i := 0; i < count; i++ {
		v.putRuneLocked(v.lastPrintable)
	}
}

func (v *VirtualTerminal) setScrollRegionLocked(params string) {
	params = strings.TrimPrefix(params, "?")
	if params == "" {
		v.hasScrollRegion = false
		v.scrollTop = 0
		v.scrollBottom = 0
		v.cursorX = 0
		v.cursorY = v.homeRowLocked()
		return
	}
	parts := strings.Split(params, ";")
	top := 1
	bottom := v.rows
	if len(parts) > 0 && parts[0] != "" {
		value, err := strconv.Atoi(parts[0])
		if err != nil {
			return
		}
		if value > 0 {
			top = value
		}
	}
	if len(parts) > 1 && parts[1] != "" {
		value, err := strconv.Atoi(parts[1])
		if err != nil {
			return
		}
		if value > 0 {
			bottom = value
		}
	}
	if top < 1 || bottom > v.rows || top >= bottom {
		return
	}
	screenTop, _ := v.displayRangeLocked()
	v.scrollTop = screenTop + top - 1
	v.scrollBottom = screenTop + bottom - 1
	v.hasScrollRegion = !(top == 1 && bottom == v.rows)
	v.cursorX = 0
	v.cursorY = v.homeRowLocked()
}

func (v *VirtualTerminal) setModeLocked(params string, enabled bool) {
	if strings.HasPrefix(params, "?") {
		v.setPrivateModeLocked(strings.TrimPrefix(params, "?"), enabled)
		return
	}
	for _, part := range strings.Split(params, ";") {
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		if value == 4 {
			v.insertMode = enabled
		}
	}
}

func (v *VirtualTerminal) setPrivateModeLocked(params string, enabled bool) {
	for _, part := range strings.Split(params, ";") {
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		if value == 6 {
			v.originMode = enabled
			v.cursorX = 0
			v.cursorY = v.homeRowLocked()
		} else if value == 7 {
			v.wraparoundMode = enabled
			if !enabled {
				v.pendingWrap = false
			}
		} else if value == 25 {
			v.cursorVisible = enabled
		} else if value == 1048 {
			if enabled {
				v.saveCursorLocked()
			} else {
				v.restoreCursorLocked()
			}
		} else if value == 47 || value == 1047 || value == 1049 {
			v.setAlternateScreenLocked(enabled)
		}
	}
}

func (v *VirtualTerminal) setAlternateScreenLocked(enabled bool) {
	if enabled {
		if v.alternateScreen {
			return
		}
		state := v.snapshotScreenLocked()
		v.normalScreen = &state
		v.alternateScreen = true
		v.lines = make([]string, v.rows)
		v.cells = make([][]VirtualCell, v.rows)
		v.wrapped = make([]bool, v.rows)
		v.cursorX = 0
		v.cursorY = 0
		v.savedX = 0
		v.savedY = 0
		v.savedOK = false
		v.savedStyle = virtualStyle{}
		v.savedOriginMode = false
		v.savedWraparound = true
		v.savedG0Charset = "ascii"
		v.savedG1Charset = "ascii"
		v.savedUseG1 = false
		v.scrollTop = 0
		v.scrollBottom = 0
		v.hasScrollRegion = false
		v.insertMode = false
		v.originMode = false
		v.wraparoundMode = true
		v.cursorVisible = true
		v.tabStopsCleared = false
		v.tabStops = map[int]bool{}
		v.clearedTabStops = map[int]bool{}
		v.g0Charset = "ascii"
		v.g1Charset = "ascii"
		v.useG1Charset = false
		return
	}
	if !v.alternateScreen || v.normalScreen == nil {
		return
	}
	state := *v.normalScreen
	v.restoreScreenLocked(state)
	v.alternateScreen = false
	v.normalScreen = nil
}

func (v *VirtualTerminal) homeRowLocked() int {
	if v.originMode && v.hasScrollRegion {
		return v.scrollTop
	}
	start, _ := v.displayRangeLocked()
	return start
}

func (v *VirtualTerminal) absoluteRowLocked(row int) int {
	if row <= 0 {
		row = 1
	}
	if v.originMode && v.hasScrollRegion {
		return max(v.scrollTop, min(v.scrollBottom, v.scrollTop+row-1))
	}
	start, end := v.displayRangeLocked()
	if end <= start {
		return max(0, row-1)
	}
	return max(start, min(end-1, start+row-1))
}

func (v *VirtualTerminal) clampCursorYLocked(row int) int {
	if v.originMode && v.hasScrollRegion {
		return max(v.scrollTop, min(v.scrollBottom, row))
	}
	start, end := v.displayRangeLocked()
	if end <= start {
		return max(0, row)
	}
	return max(start, min(end-1, row))
}

func (v *VirtualTerminal) clampCursorXLocked(col int) int {
	col = max(0, col)
	if v.columns <= 0 {
		return col
	}
	return min(col, v.columns-1)
}

func (v *VirtualTerminal) handleSGRLocked(params string) {
	if params == "" {
		params = "0"
	}
	parts := sgrParams(params)
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		if part == "" {
			part = "0"
		}
		value, err := strconv.Atoi(sgrCode(part))
		if err != nil {
			continue
		}
		switch value {
		case 0:
			v.style = virtualStyle{}
		case 1:
			v.style.bold = true
		case 2:
			v.style.dim = true
		case 21:
			v.style.underline = true
			v.style.underlineStyle = "double"
		case 22:
			v.style.bold = false
			v.style.dim = false
		case 3:
			v.style.italic = true
		case 4:
			v.handleUnderlineSGRLocked(part)
		case 5, 6:
			v.style.blink = true
		case 7:
			v.style.inverse = true
		case 8:
			v.style.conceal = true
		case 9:
			v.style.strikethrough = true
		case 23:
			v.style.italic = false
		case 24:
			v.style.underline = false
			v.style.underlineStyle = ""
		case 25:
			v.style.blink = false
		case 27:
			v.style.inverse = false
		case 28:
			v.style.conceal = false
		case 29:
			v.style.strikethrough = false
		case 30, 31, 32, 33, 34, 35, 36, 37:
			v.style.foreground = VirtualColor{Kind: "ansi", Index: value - 30}
		case 39:
			v.style.foreground = VirtualColor{}
		case 40, 41, 42, 43, 44, 45, 46, 47:
			v.style.background = VirtualColor{Kind: "ansi", Index: value - 40}
		case 49:
			v.style.background = VirtualColor{}
		case 58:
			color, consumed, ok := sgrExtendedColor(parts[i+1:])
			if ok {
				v.style.underlineColor = color
			}
			i += consumed
		case 59:
			v.style.underlineColor = VirtualColor{}
		case 53:
			v.style.overline = true
		case 55:
			v.style.overline = false
		case 90, 91, 92, 93, 94, 95, 96, 97:
			v.style.foreground = VirtualColor{Kind: "ansi", Index: 8 + value - 90}
		case 100, 101, 102, 103, 104, 105, 106, 107:
			v.style.background = VirtualColor{Kind: "ansi", Index: 8 + value - 100}
		case 38, 48:
			color, consumed, ok := sgrExtendedColor(parts[i+1:])
			if ok {
				if value == 38 {
					v.style.foreground = color
				} else if value == 48 {
					v.style.background = color
				}
			}
			i += consumed
		}
	}
}

func (v *VirtualTerminal) handleUnderlineSGRLocked(part string) {
	if !strings.Contains(part, ":") {
		v.style.underline = true
		v.style.underlineStyle = "single"
		return
	}
	parts := strings.Split(part, ":")
	if len(parts) < 2 {
		v.style.underline = true
		v.style.underlineStyle = "single"
		return
	}
	switch parts[1] {
	case "0":
		v.style.underline = false
		v.style.underlineStyle = ""
	case "1":
		v.style.underline = true
		v.style.underlineStyle = "single"
	case "2":
		v.style.underline = true
		v.style.underlineStyle = "double"
	case "3":
		v.style.underline = true
		v.style.underlineStyle = "curly"
	case "4":
		v.style.underline = true
		v.style.underlineStyle = "dotted"
	case "5":
		v.style.underline = true
		v.style.underlineStyle = "dashed"
	default:
		v.style.underline = true
		v.style.underlineStyle = "single"
	}
}

func sgrParams(params string) []string {
	if params == "" {
		return []string{"0"}
	}
	fields := strings.Split(params, ";")
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		if isColonExtendedColorField(field) {
			parts = append(parts, strings.Split(field, ":")...)
		} else {
			parts = append(parts, field)
		}
	}
	return parts
}

func isColonExtendedColorField(field string) bool {
	return strings.HasPrefix(field, "38:") || strings.HasPrefix(field, "48:") || strings.HasPrefix(field, "58:")
}

func sgrCode(part string) string {
	if idx := strings.IndexByte(part, ':'); idx >= 0 {
		return part[:idx]
	}
	return part
}

func sgrColorParamCount(parts []string) int {
	_, consumed, _ := sgrExtendedColor(parts)
	return consumed
}

func sgrExtendedColor(parts []string) (VirtualColor, int, bool) {
	if len(parts) == 0 {
		return VirtualColor{}, 0, false
	}
	mode, err := strconv.Atoi(sgrCode(parts[0]))
	if err != nil {
		return VirtualColor{}, 0, false
	}
	switch mode {
	case 2:
		start := 1
		if len(parts) >= 5 && parts[1] == "" {
			start = 2
		}
		if len(parts) < start+3 {
			return VirtualColor{}, len(parts), false
		}
		r, errR := strconv.Atoi(sgrCode(parts[start]))
		g, errG := strconv.Atoi(sgrCode(parts[start+1]))
		b, errB := strconv.Atoi(sgrCode(parts[start+2]))
		if errR != nil || errG != nil || errB != nil {
			return VirtualColor{}, start + 3, false
		}
		return VirtualColor{Kind: "rgb", R: clampColor(r), G: clampColor(g), B: clampColor(b)}, start + 3, true
	case 5:
		if len(parts) < 2 {
			return VirtualColor{}, len(parts), false
		}
		index, err := strconv.Atoi(sgrCode(parts[1]))
		if err != nil {
			return VirtualColor{}, 2, false
		}
		return VirtualColor{Kind: "index", Index: max(0, min(255, index))}, 2, true
	default:
		return VirtualColor{}, 0, false
	}
}

func clampColor(value int) int {
	return max(0, min(255, value))
}

func (v *VirtualTerminal) cellForRune(r rune) VirtualCell {
	return VirtualCell{
		Rune:           r,
		Bold:           v.style.bold,
		Dim:            v.style.dim,
		Italic:         v.style.italic,
		Underline:      v.style.underline,
		UnderlineStyle: v.style.underlineStyle,
		Inverse:        v.style.inverse,
		Strikethrough:  v.style.strikethrough,
		Blink:          v.style.blink,
		Conceal:        v.style.conceal,
		Overline:       v.style.overline,
		Foreground:     v.style.foreground,
		Background:     v.style.background,
		UnderlineColor: v.style.underlineColor,
	}
}

func (v *VirtualTerminal) blankCellLocked() VirtualCell {
	return v.cellForRune(' ')
}

func (v *VirtualTerminal) putRuneLocked(r rune) {
	r = v.mapCharsetRuneLocked(r)
	v.ensureCursorLineLocked()
	line := v.lines[v.cursorY]
	cells := v.cells[v.cursorY]
	lineWidth := visibleWidthPlain(line)
	if v.cursorX == lineWidth && virtualRuneExtendsPreviousCluster(line, r) {
		nextLine := appendRuneToLastVirtualCluster(line, r)
		nextWidth := visibleWidthPlain(nextLine)
		if delta := nextWidth - lineWidth; delta > 0 {
			tail := v.blankCellLocked()
			if lineWidth > 0 {
				for len(cells) < lineWidth {
					cells = append(cells, v.blankCellLocked())
				}
				tail = cells[lineWidth-1]
				tail.Rune = ' '
			}
			for len(cells) < nextWidth {
				cells = append(cells, tail)
			}
			for col := lineWidth; col < nextWidth && col < len(cells); col++ {
				cells[col] = tail
			}
			v.cursorX += delta
			if v.columns > 0 && v.cursorX > v.columns {
				v.cursorX = v.columns
			}
		}
		v.lines[v.cursorY] = nextLine
		if v.columns > 0 && visibleWidthPlain(v.lines[v.cursorY]) > v.columns {
			v.lines[v.cursorY], _ = truncateFragmentToWidth(v.lines[v.cursorY], v.columns)
		}
		if v.columns > 0 && len(cells) > v.columns {
			cells = cells[:v.columns]
		}
		v.cells[v.cursorY] = cells
		return
	}
	width := graphemeWidth([]rune{r})
	if v.columns > 0 {
		if v.wraparoundMode && (v.cursorX >= v.columns || (v.pendingWrap && v.cursorX >= v.columns-1) || (v.cursorX > 0 && v.cursorX+width > v.columns)) {
			v.pendingWrap = false
			v.lineFeedLocked(true)
			v.setLineWrappedLocked(v.cursorY, true)
			line = v.lines[v.cursorY]
			cells = v.cells[v.cursorY]
			lineWidth = visibleWidthPlain(line)
		} else if !v.wraparoundMode && v.cursorX >= v.columns {
			v.cursorX = max(0, v.columns-1)
		}
	}
	for len(cells) < v.cursorX {
		cells = append(cells, v.blankCellLocked())
	}
	if v.cursorX > lineWidth {
		gap := strings.Repeat(" ", v.cursorX-lineWidth)
		line += gap
		for len(cells) < v.cursorX {
			cells = append(cells, v.blankCellLocked())
		}
	}
	cell := v.cellForRune(r)
	start := virtualByteIndexAtColumnFloor(line, v.cursorX)
	startCol := visibleWidthPlain(line[:start])
	end := virtualByteIndexAtColumnCeil(line, v.cursorX+width)
	endCol := visibleWidthPlain(line[:end])
	if v.insertMode {
		end = start
		endCol = startCol
	}
	beforePad := strings.Repeat(" ", max(0, v.cursorX-startCol))
	afterPad := ""
	if !v.insertMode {
		afterPad = strings.Repeat(" ", max(0, endCol-(v.cursorX+width)))
	}
	line = line[:start] + beforePad + string(r) + afterPad + line[end:]
	for len(cells) <= v.cursorX {
		cells = append(cells, v.blankCellLocked())
	}
	if v.insertMode {
		inserted := make([]VirtualCell, max(1, width))
		for i := range inserted {
			if i == 0 {
				inserted[i] = cell
			} else {
				inserted[i] = v.cellForRune(' ')
			}
		}
		cells = append(cells[:v.cursorX], append(inserted, cells[v.cursorX:]...)...)
	} else {
		coveredEnd := max(endCol, v.cursorX+width)
		for len(cells) < coveredEnd {
			cells = append(cells, v.blankCellLocked())
		}
		for col := startCol; col < coveredEnd; col++ {
			cells[col] = v.blankCellLocked()
		}
		cells[v.cursorX] = cell
		for i := 1; i < width; i++ {
			if v.cursorX+i < len(cells) {
				cells[v.cursorX+i] = v.cellForRune(' ')
			} else {
				cells = append(cells, v.cellForRune(' '))
			}
		}
	}
	if v.columns > 0 && visibleWidthPlain(line) > v.columns {
		line, _ = truncateFragmentToWidth(line, v.columns)
	}
	if v.columns > 0 && len(cells) > v.columns {
		cells = cells[:v.columns]
	}
	v.cursorX += width
	if v.columns > 0 && v.cursorX >= v.columns {
		v.pendingWrap = width > 0
		if !v.wraparoundMode {
			v.cursorX = v.columns - 1
		}
	} else if width > 0 {
		v.pendingWrap = false
	}
	v.lines[v.cursorY] = line
	v.cells[v.cursorY] = cells
	if width > 0 {
		v.lastPrintable = r
	}
}

func (v *VirtualTerminal) designateCharsetLocked(target byte, final byte) {
	charset := "ascii"
	switch final {
	case '0', '2':
		charset = "dec-special"
	}
	switch target {
	case '(':
		v.g0Charset = charset
	case ')':
		v.g1Charset = charset
	}
}

func (v *VirtualTerminal) mapCharsetRuneLocked(r rune) rune {
	charset := v.g0Charset
	if v.useG1Charset {
		charset = v.g1Charset
	}
	if charset != "dec-special" {
		return r
	}
	if mapped, ok := decSpecialGraphicsRunes[r]; ok {
		return mapped
	}
	return r
}

var decSpecialGraphicsRunes = map[rune]rune{
	'`': '◆',
	'a': '▒',
	'b': '␉',
	'c': '␌',
	'd': '␍',
	'e': '␊',
	'f': '°',
	'g': '±',
	'h': '␤',
	'i': '␋',
	'j': '┘',
	'k': '┐',
	'l': '┌',
	'm': '└',
	'n': '┼',
	'o': '⎺',
	'p': '⎻',
	'q': '─',
	'r': '⎼',
	's': '⎽',
	't': '├',
	'u': '┤',
	'v': '┴',
	'w': '┬',
	'x': '│',
	'y': '≤',
	'z': '≥',
	'{': 'π',
	'|': '≠',
	'}': '£',
	'~': '·',
}

func virtualRuneExtendsPreviousCluster(line string, r rune) bool {
	runes := []rune(line)
	if len(runes) == 0 {
		return false
	}
	if r == '\u200d' || isGraphemeExtend(r) || runes[len(runes)-1] == '\u200d' {
		return true
	}
	if !isRegionalIndicator(r) {
		return false
	}
	spans := graphemeSpans(runes)
	if len(spans) == 0 {
		return false
	}
	last := spans[len(spans)-1]
	cluster := runes[last.start:last.end]
	return len(cluster) == 1 && isRegionalIndicator(cluster[0])
}

func appendRuneToLastVirtualCluster(line string, r rune) string {
	runes := []rune(line)
	spans := graphemeSpans(runes)
	if len(spans) == 0 {
		return line + string(r)
	}
	offsets := runeByteOffsets(line)
	last := spans[len(spans)-1]
	end := offsets[last.end]
	return line[:end] + string(r) + line[end:]
}

func virtualByteRangeAtColumn(line string, col int) (int, int) {
	if line == "" {
		return 0, 0
	}
	runes := []rune(line)
	offsets := runeByteOffsets(line)
	current := 0
	for _, span := range graphemeSpans(runes) {
		width := graphemeWidth(runes[span.start:span.end])
		if col <= current || col < current+width {
			return offsets[span.start], offsets[span.end]
		}
		current += width
	}
	return len(line), len(line)
}

func virtualByteIndexAtColumnFloor(line string, col int) int {
	if line == "" || col <= 0 {
		return 0
	}
	runes := []rune(line)
	offsets := runeByteOffsets(line)
	current := 0
	for _, span := range graphemeSpans(runes) {
		width := graphemeWidth(runes[span.start:span.end])
		if col <= current || col < current+width {
			return offsets[span.start]
		}
		current += width
	}
	return len(line)
}

func virtualByteIndexAtColumnCeil(line string, col int) int {
	if line == "" || col <= 0 {
		return 0
	}
	runes := []rune(line)
	offsets := runeByteOffsets(line)
	current := 0
	for _, span := range graphemeSpans(runes) {
		width := graphemeWidth(runes[span.start:span.end])
		if col <= current {
			return offsets[span.start]
		}
		if col < current+width {
			return offsets[span.end]
		}
		current += width
	}
	return len(line)
}

func (v *VirtualTerminal) lineFeedLocked(resetColumn bool) {
	v.pendingWrap = false
	if v.hasScrollRegion && v.cursorY == v.scrollBottom {
		v.scrollUpRegionLocked(v.scrollTop, v.scrollBottom, 1)
		if resetColumn {
			v.cursorX = 0
		} else if v.columns > 0 && v.cursorX >= v.columns {
			v.cursorX = v.columns - 1
		}
		v.setLineWrappedLocked(v.cursorY, false)
		return
	}
	v.cursorY++
	if resetColumn {
		v.cursorX = 0
	} else if v.columns > 0 && v.cursorX >= v.columns {
		v.cursorX = v.columns - 1
	}
	v.ensureCursorLineLocked()
	v.setLineWrappedLocked(v.cursorY, false)
}

func (v *VirtualTerminal) indexLocked() {
	if v.hasScrollRegion && v.cursorY == v.scrollBottom {
		v.scrollUpRegionLocked(v.scrollTop, v.scrollBottom, 1)
		v.setLineWrappedLocked(v.cursorY, false)
		return
	}
	v.cursorY++
	v.ensureCursorLineLocked()
	v.setLineWrappedLocked(v.cursorY, false)
}

func (v *VirtualTerminal) reverseIndexLocked() {
	if v.hasScrollRegion && v.cursorY == v.scrollTop {
		v.scrollDownRegionLocked(v.scrollTop, v.scrollBottom, 1)
		return
	}
	if v.cursorY > 0 {
		v.cursorY--
		return
	}
	v.scrollDownLocked(1)
}

func (v *VirtualTerminal) ensureCursorLineLocked() {
	for v.cursorY >= len(v.lines) {
		v.lines = append(v.lines, "")
		v.cells = append(v.cells, nil)
		v.wrapped = append(v.wrapped, false)
	}
	v.ensureWrappedLocked()
}

func (v *VirtualTerminal) eraseLineFromCursorLocked() {
	v.ensureCursorLineLocked()
	line := v.lines[v.cursorY]
	targetCells := v.eraseLineCellWidthLocked(v.cursorX)
	start := virtualByteIndexAtColumnFloor(line, v.cursorX)
	if start < len(line) {
		v.lines[v.cursorY] = strings.TrimRight(line[:start], " ")
	}
	cells := v.cells[v.cursorY]
	for len(cells) < targetCells {
		cells = append(cells, v.blankCellLocked())
	}
	for i := v.cursorX; i < len(cells); i++ {
		cells[i] = v.blankCellLocked()
	}
	v.cells[v.cursorY] = cells
}

func (v *VirtualTerminal) eraseLineToCursorLocked() {
	v.ensureCursorLineLocked()
	line := v.lines[v.cursorY]
	cells := v.cells[v.cursorY]
	end := virtualByteIndexAtColumnCeil(line, v.cursorX+1)
	replacementWidth := max(visibleWidthPlain(line[:end]), v.cursorX+1)
	targetCells := min(v.eraseLineCellWidthLocked(v.cursorX), replacementWidth)
	for len(cells) < targetCells {
		cells = append(cells, v.blankCellLocked())
	}
	for i := 0; i < targetCells; i++ {
		cells[i] = v.blankCellLocked()
	}
	v.lines[v.cursorY] = strings.TrimRight(strings.Repeat(" ", replacementWidth)+line[end:], " ")
	v.cells[v.cursorY] = cells
}

func (v *VirtualTerminal) eraseWholeLineLocked() {
	v.ensureCursorLineLocked()
	v.eraseWholeLineAtLocked(v.cursorY)
}

func (v *VirtualTerminal) eraseDisplayFromCursorLocked() {
	v.ensureCursorLineLocked()
	_, end := v.displayRangeLocked()
	v.eraseLineFromCursorLocked()
	for row := v.cursorY + 1; row < end; row++ {
		v.eraseWholeLineAtLocked(row)
	}
}

func (v *VirtualTerminal) eraseDisplayToCursorLocked() {
	v.ensureCursorLineLocked()
	start, _ := v.displayRangeLocked()
	for row := start; row < v.cursorY && row < len(v.lines); row++ {
		v.eraseWholeLineAtLocked(row)
	}
	v.eraseLineToCursorLocked()
}

func (v *VirtualTerminal) eraseWholeDisplayLocked() {
	v.ensureCursorLineLocked()
	start, end := v.displayRangeLocked()
	for row := start; row < end; row++ {
		v.eraseWholeLineAtLocked(row)
	}
}

func (v *VirtualTerminal) eraseWholeLineAtLocked(row int) {
	if row < 0 {
		return
	}
	for row >= len(v.lines) {
		v.lines = append(v.lines, "")
		v.cells = append(v.cells, nil)
		v.wrapped = append(v.wrapped, false)
	}
	v.lines[row] = ""
	v.cells[row] = v.blankLineCellsLocked()
	v.setLineWrappedLocked(row, false)
}

func (v *VirtualTerminal) blankLinesLocked(count int) ([]string, [][]VirtualCell) {
	lines := make([]string, count)
	cells := make([][]VirtualCell, count)
	for i := range cells {
		cells[i] = v.blankLineCellsLocked()
	}
	return lines, cells
}

func (v *VirtualTerminal) blankLineCellsLocked() []VirtualCell {
	width := v.columns
	if width <= 0 {
		width = 0
	}
	cells := make([]VirtualCell, width)
	for i := range cells {
		cells[i] = v.blankCellLocked()
	}
	return cells
}

func (v *VirtualTerminal) displayRangeLocked() (int, int) {
	if v.rows <= 0 {
		return 0, len(v.lines)
	}
	start := max(0, len(v.lines)-v.rows)
	end := min(len(v.lines), start+v.rows)
	return start, end
}

func (v *VirtualTerminal) viewportCursorYLocked() int {
	start, end := v.displayRangeLocked()
	if end <= start {
		return 0
	}
	return max(0, min(end-start-1, v.cursorY-start))
}

func (v *VirtualTerminal) eraseLineCellWidthLocked(fromCol int) int {
	if v.columns > 0 {
		return v.columns
	}
	return max(len(v.cells[v.cursorY]), fromCol+1)
}

func (v *VirtualTerminal) insertBlankCharactersLocked(count int) {
	if count <= 0 {
		count = 1
	}
	v.ensureCursorLineLocked()
	line := v.lines[v.cursorY]
	cells := v.cells[v.cursorY]
	lineWidth := visibleWidthPlain(line)
	if lineWidth < v.cursorX {
		line += strings.Repeat(" ", v.cursorX-lineWidth)
	}
	for len(cells) < v.cursorX {
		cells = append(cells, v.blankCellLocked())
	}
	start := virtualByteIndexAtColumnFloor(line, v.cursorX)
	blanks := strings.Repeat(" ", count)
	blankCells := make([]VirtualCell, count)
	for i := range blankCells {
		blankCells[i] = v.blankCellLocked()
	}
	line = line[:start] + blanks + line[start:]
	cells = append(cells[:v.cursorX], append(blankCells, cells[v.cursorX:]...)...)
	if v.columns > 0 && visibleWidthPlain(line) > v.columns {
		line, _ = truncateFragmentToWidth(line, v.columns)
	}
	if len(cells) > v.columns {
		cells = cells[:v.columns]
	}
	v.lines[v.cursorY] = line
	v.cells[v.cursorY] = cells
}

func (v *VirtualTerminal) deleteCharactersLocked(count int) {
	if count <= 0 {
		count = 1
	}
	v.ensureCursorLineLocked()
	line := v.lines[v.cursorY]
	cells := v.cells[v.cursorY]
	fillWidth := v.columns
	if fillWidth <= 0 {
		fillWidth = len(cells)
	}
	start := virtualByteIndexAtColumnFloor(line, v.cursorX)
	end := virtualByteIndexAtColumnCeil(line, v.cursorX+count)
	removedWidth := visibleWidthPlain(line[start:end])
	line = line[:start] + line[end:]
	if v.cursorX < len(cells) {
		end := min(len(cells), v.cursorX+max(count, removedWidth))
		cells = append(cells[:v.cursorX], cells[end:]...)
	}
	for len(cells) < fillWidth {
		cells = append(cells, v.blankCellLocked())
	}
	if fillWidth > 0 && len(cells) > fillWidth {
		cells = cells[:fillWidth]
	}
	v.lines[v.cursorY] = line
	v.cells[v.cursorY] = cells
}

func (v *VirtualTerminal) insertLinesLocked(count int) {
	if count <= 0 {
		count = 1
	}
	v.ensureCursorLineLocked()
	for len(v.lines) < v.rows {
		v.lines = append(v.lines, "")
		v.cells = append(v.cells, nil)
		v.wrapped = append(v.wrapped, false)
	}
	_, displayEnd := v.displayRangeLocked()
	limit := displayEnd - 1
	if v.hasScrollRegion {
		if v.cursorY < v.scrollTop || v.cursorY > v.scrollBottom {
			return
		}
		limit = v.scrollBottom
	}
	count = min(count, max(0, limit-v.cursorY+1))
	if count <= 0 {
		return
	}
	blankLines, blankCells := v.blankLinesLocked(count)
	v.lines = append(v.lines[:v.cursorY], append(blankLines, v.lines[v.cursorY:]...)...)
	v.cells = append(v.cells[:v.cursorY], append(blankCells, v.cells[v.cursorY:]...)...)
	v.wrapped = append(v.wrapped[:v.cursorY], append(make([]bool, count), v.wrapped[v.cursorY:]...)...)
	removeAt := limit + 1
	if removeAt+count <= len(v.lines) {
		v.lines = append(v.lines[:removeAt], v.lines[removeAt+count:]...)
		v.cells = append(v.cells[:removeAt], v.cells[removeAt+count:]...)
		v.wrapped = append(v.wrapped[:removeAt], v.wrapped[removeAt+count:]...)
	}
	v.ensureWrappedLocked()
}

func (v *VirtualTerminal) deleteLinesLocked(count int) {
	if count <= 0 {
		count = 1
	}
	v.ensureCursorLineLocked()
	for len(v.lines) < v.rows {
		v.lines = append(v.lines, "")
		v.cells = append(v.cells, nil)
		v.wrapped = append(v.wrapped, false)
	}
	_, displayEnd := v.displayRangeLocked()
	limit := displayEnd - 1
	if v.hasScrollRegion {
		if v.cursorY < v.scrollTop || v.cursorY > v.scrollBottom {
			return
		}
		limit = v.scrollBottom
	}
	count = min(count, max(0, limit-v.cursorY+1))
	if count <= 0 {
		return
	}
	end := min(limit+1, v.cursorY+count)
	v.lines = append(v.lines[:v.cursorY], v.lines[end:]...)
	v.cells = append(v.cells[:v.cursorY], v.cells[end:]...)
	v.wrapped = append(v.wrapped[:v.cursorY], v.wrapped[end:]...)
	insertAt := max(v.cursorY, limit-count+1)
	for i := 0; i < count; i++ {
		if insertAt <= len(v.lines) {
			v.lines = append(v.lines[:insertAt], append([]string{""}, v.lines[insertAt:]...)...)
			v.cells = append(v.cells[:insertAt], append([][]VirtualCell{v.blankLineCellsLocked()}, v.cells[insertAt:]...)...)
			v.wrapped = append(v.wrapped[:insertAt], append([]bool{false}, v.wrapped[insertAt:]...)...)
		}
	}
	if !v.hasScrollRegion {
		for len(v.lines) < displayEnd {
			v.lines = append(v.lines, "")
			v.cells = append(v.cells, v.blankLineCellsLocked())
			v.wrapped = append(v.wrapped, false)
		}
	}
	v.ensureWrappedLocked()
}

func (v *VirtualTerminal) scrollUpLocked(count int) {
	if count <= 0 {
		count = 1
	}
	if v.hasScrollRegion {
		v.scrollUpRegionLocked(v.scrollTop, v.scrollBottom, count)
		return
	}
	for len(v.lines) < v.rows {
		v.lines = append(v.lines, "")
		v.cells = append(v.cells, nil)
		v.wrapped = append(v.wrapped, false)
	}
	start, end := v.displayRangeLocked()
	if end <= start {
		return
	}
	v.scrollUpRegionLocked(start, end-1, count)
}

func (v *VirtualTerminal) scrollDownLocked(count int) {
	if count <= 0 {
		count = 1
	}
	if v.hasScrollRegion {
		v.scrollDownRegionLocked(v.scrollTop, v.scrollBottom, count)
		return
	}
	for len(v.lines) < v.rows {
		v.lines = append(v.lines, "")
		v.cells = append(v.cells, nil)
		v.wrapped = append(v.wrapped, false)
	}
	start, end := v.displayRangeLocked()
	if end <= start {
		return
	}
	v.scrollDownRegionLocked(start, end-1, count)
}

func (v *VirtualTerminal) scrollUpRegionLocked(top, bottom, count int) {
	for len(v.lines) < v.rows {
		v.lines = append(v.lines, "")
		v.cells = append(v.cells, nil)
		v.wrapped = append(v.wrapped, false)
	}
	v.ensureWrappedLocked()
	limit := max(0, len(v.lines)-1)
	top = max(0, min(top, limit))
	bottom = max(top, min(bottom, limit))
	count = min(max(1, count), bottom-top+1)
	copy(v.lines[top:bottom-count+1], v.lines[top+count:bottom+1])
	copy(v.cells[top:bottom-count+1], v.cells[top+count:bottom+1])
	copy(v.wrapped[top:bottom-count+1], v.wrapped[top+count:bottom+1])
	for i := bottom - count + 1; i <= bottom; i++ {
		v.eraseWholeLineAtLocked(i)
		v.setLineWrappedLocked(i, false)
	}
}

func (v *VirtualTerminal) scrollDownRegionLocked(top, bottom, count int) {
	for len(v.lines) < v.rows {
		v.lines = append(v.lines, "")
		v.cells = append(v.cells, nil)
		v.wrapped = append(v.wrapped, false)
	}
	v.ensureWrappedLocked()
	limit := max(0, len(v.lines)-1)
	top = max(0, min(top, limit))
	bottom = max(top, min(bottom, limit))
	count = min(max(1, count), bottom-top+1)
	copy(v.lines[top+count:bottom+1], v.lines[top:bottom-count+1])
	copy(v.cells[top+count:bottom+1], v.cells[top:bottom-count+1])
	copy(v.wrapped[top+count:bottom+1], v.wrapped[top:bottom-count+1])
	for i := top; i < top+count; i++ {
		v.eraseWholeLineAtLocked(i)
		v.setLineWrappedLocked(i, false)
	}
}

func (v *VirtualTerminal) eraseCharactersLocked(count int) {
	if count <= 0 {
		count = 1
	}
	v.ensureCursorLineLocked()
	line := v.lines[v.cursorY]
	cells := v.cells[v.cursorY]
	start := virtualByteIndexAtColumnFloor(line, v.cursorX)
	end := virtualByteIndexAtColumnCeil(line, v.cursorX+count)
	replacementWidth := visibleWidthPlain(line[start:end])
	endCells := v.cursorX + max(count, replacementWidth)
	if v.columns > 0 {
		endCells = min(endCells, v.columns)
	}
	for len(cells) < endCells {
		cells = append(cells, v.blankCellLocked())
	}
	for i := v.cursorX; i < endCells; i++ {
		cells[i] = v.blankCellLocked()
	}
	v.lines[v.cursorY] = line[:start] + strings.Repeat(" ", replacementWidth) + line[end:]
	v.cells[v.cursorY] = cells
}

func (v *VirtualTerminal) clearScreenLocked() {
	v.lines = make([]string, v.rows)
	v.cells = make([][]VirtualCell, v.rows)
	v.wrapped = make([]bool, v.rows)
	v.cursorX = 0
	v.cursorY = 0
	v.savedX = 0
	v.savedY = 0
	v.savedOK = false
	v.savedStyle = virtualStyle{}
	v.savedOriginMode = false
	v.savedWraparound = true
	v.savedG0Charset = "ascii"
	v.savedG1Charset = "ascii"
	v.savedUseG1 = false
}

func (v *VirtualTerminal) clearBufferLocked() {
	v.ensureCursorLineLocked()
	line := ""
	var cells []VirtualCell
	wrapped := false
	if v.cursorY >= 0 && v.cursorY < len(v.lines) {
		line = v.lines[v.cursorY]
	}
	if v.cursorY >= 0 && v.cursorY < len(v.cells) {
		cells = append([]VirtualCell(nil), v.cells[v.cursorY]...)
	}
	if v.cursorY >= 0 && v.cursorY < len(v.wrapped) {
		wrapped = v.wrapped[v.cursorY]
	}
	rows := max(1, v.rows)
	v.lines = make([]string, rows)
	v.cells = make([][]VirtualCell, rows)
	v.wrapped = make([]bool, rows)
	v.lines[0] = line
	v.cells[0] = cells
	v.wrapped[0] = wrapped
	v.cursorY = 0
	v.cursorX = v.clampCursorXLocked(v.cursorX)
	if v.savedOK {
		v.savedY = 0
		v.savedX = v.clampCursorXLocked(v.savedX)
	}
	if v.hasScrollRegion {
		v.scrollTop = max(0, min(v.scrollTop, rows-1))
		v.scrollBottom = max(v.scrollTop, min(v.scrollBottom, rows-1))
	}
}

func (v *VirtualTerminal) screenAlignmentTestLocked() {
	rows := max(1, v.rows)
	cols := max(1, v.columns)
	v.lines = make([]string, rows)
	v.cells = make([][]VirtualCell, rows)
	v.wrapped = make([]bool, rows)
	line := strings.Repeat("E", cols)
	for row := 0; row < rows; row++ {
		v.lines[row] = line
		v.cells[row] = make([]VirtualCell, cols)
		for col := range v.cells[row] {
			v.cells[row][col] = v.cellForRune('E')
		}
	}
	v.cursorX = 0
	v.cursorY = 0
	v.lastPrintable = 'E'
}

func (v *VirtualTerminal) clearScrollbackLocked() {
	extra := len(v.lines) - v.rows
	if extra <= 0 {
		return
	}
	v.lines = append([]string(nil), v.lines[extra:]...)
	v.cells = cloneVirtualCells(v.cells[extra:])
	v.wrapped = append([]bool(nil), v.wrapped[extra:]...)
	v.ensureWrappedLocked()
	v.cursorY = max(0, v.cursorY-extra)
	v.savedY = max(0, v.savedY-extra)
	if v.hasScrollRegion {
		v.scrollTop = max(0, v.scrollTop-extra)
		v.scrollBottom = max(v.scrollTop, v.scrollBottom-extra)
		if v.scrollBottom >= v.rows || v.scrollTop >= v.scrollBottom {
			v.hasScrollRegion = false
			v.scrollTop = 0
			v.scrollBottom = 0
		}
	}
}

func (v *VirtualTerminal) saveCursorLocked() {
	v.savedX = v.cursorX
	v.savedY = v.cursorY
	v.savedStyle = v.style
	v.savedOriginMode = v.originMode
	v.savedWraparound = v.wraparoundMode
	v.savedG0Charset = v.g0Charset
	v.savedG1Charset = v.g1Charset
	v.savedUseG1 = v.useG1Charset
	v.savedOK = true
}

func (v *VirtualTerminal) restoreCursorLocked() {
	if !v.savedOK {
		return
	}
	v.style = v.savedStyle
	v.originMode = v.savedOriginMode
	v.wraparoundMode = v.savedWraparound
	v.g0Charset = v.savedG0Charset
	v.g1Charset = v.savedG1Charset
	v.useG1Charset = v.savedUseG1
	v.cursorX = v.clampCursorXLocked(v.savedX)
	v.cursorY = max(0, v.savedY)
	v.ensureCursorLineLocked()
}

func csiParam(params string, fallback int) int {
	params = strings.TrimPrefix(params, "?")
	if params == "" {
		return fallback
	}
	parts := strings.Split(params, ";")
	if len(parts) == 0 || parts[0] == "" {
		return fallback
	}
	value, err := strconv.Atoi(parts[0])
	if err != nil {
		return fallback
	}
	return value
}

func csiPositiveParam(params string, fallback int) int {
	value := csiParam(params, fallback)
	if value <= 0 {
		return fallback
	}
	return value
}

func csiRowCol(params string) (int, int) {
	if params == "" {
		return 1, 1
	}
	parts := strings.Split(params, ";")
	row, col := 1, 1
	if len(parts) > 0 && parts[0] != "" {
		if value, err := strconv.Atoi(parts[0]); err == nil {
			row = value
		}
	}
	if len(parts) > 1 && parts[1] != "" {
		if value, err := strconv.Atoi(parts[1]); err == nil {
			col = value
		}
	}
	return row, col
}
