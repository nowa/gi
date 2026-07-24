package gitui

import (
	"strings"
	"sync"
)

type Input struct {
	FocusState
	mu            sync.Mutex
	text          string
	placeholder   string
	cursor        int
	killRing      []string
	killIndex     int
	undoStack     []inputSnapshot
	lastAction    string
	pasteBuffer   string
	inPaste       bool
	lastKill      bool
	lastYank      bool
	lastYankWidth int
	OnSubmit      func(string)
	OnEscape      func()
	OnChange      func(string)
}

type inputSnapshot struct {
	text   string
	cursor int
}

type inputCallback func()

func NewInput(placeholder ...string) *Input {
	p := ""
	if len(placeholder) > 0 {
		p = placeholder[0]
	}
	return &Input{placeholder: p}
}

func (i *Input) Text() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.text
}
func (i *Input) GetValue() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.text
}
func (i *Input) SetValue(text string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.text = text
	i.cursor = min(i.cursor, len([]rune(text)))
	i.lastAction = ""
	i.lastKill = false
	i.lastYank = false
}
func (i *Input) SetText(text string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.text = text
	i.cursor = len([]rune(text))
	i.lastAction = ""
	i.lastKill = false
	i.lastYank = false
}
func (i *Input) Invalidate() {}
func (i *Input) Render(width int) []string {
	i.mu.Lock()
	value := i.text
	placeholder := i.placeholder
	cursorValue := i.cursor
	focused := i.Focused()
	i.mu.Unlock()

	prompt := "> "
	availableWidth := width - VisibleWidth(prompt)
	if availableWidth <= 0 {
		return []string{prompt}
	}

	cursor := max(0, min(cursorValue, len([]rune(value))))
	displayValue := value
	if displayValue == "" && placeholder != "" {
		displayValue = placeholder
		cursor = 0
	}

	totalWidth := VisibleWidth(displayValue)
	cursorCol := VisibleWidth(string([]rune(value)[:cursor]))
	scrollWidth := availableWidth
	if cursor == len([]rune(value)) {
		scrollWidth = max(1, availableWidth-1)
	}
	startCol := 0
	if totalWidth > availableWidth {
		halfWidth := scrollWidth / 2
		switch {
		case cursorCol < halfWidth:
			startCol = 0
		case cursorCol > totalWidth-halfWidth:
			startCol = max(0, totalWidth-scrollWidth)
		default:
			startCol = max(0, cursorCol-halfWidth)
		}
	}

	visibleText := inputSliceByColumn(displayValue, startCol, scrollWidth)
	beforeCursor := inputSliceByColumn(displayValue, startCol, max(0, cursorCol-startCol))
	atCursor := " "
	afterCursor := ""
	if len(beforeCursor) < len(visibleText) {
		after := visibleText[len(beforeCursor):]
		clusterBytes := firstGraphemeByteLen(after)
		atCursor = after[:clusterBytes]
		afterCursor = after[clusterBytes:]
	}
	marker := ""
	if focused {
		marker = CursorMarker
	}
	line := prompt + beforeCursor + marker + "\x1b[7m" + atCursor + "\x1b[27m" + afterCursor
	return []string{TruncateToWidth(line, width, "", true)}
}

func inputSliceByColumn(text string, startCol, width int) string {
	if width <= 0 || text == "" {
		return ""
	}
	endCol := startCol + width
	col := 0
	var b strings.Builder
	for _, segment := range terminalWidthSegments(text) {
		nextCol := col + segment.width
		if nextCol <= startCol {
			col = nextCol
			continue
		}
		if col >= endCol || col+segment.width > endCol {
			break
		}
		b.WriteString(segment.text)
		col = nextCol
	}
	return b.String()
}

func (i *Input) HandleInput(data string) {
	i.mu.Lock()
	callbacks := i.handleInputLocked(data)
	i.mu.Unlock()
	runInputCallbacks(callbacks)
}

func (i *Input) handleInputLocked(data string) []inputCallback {
	var callbacks []inputCallback
	if data == "" {
		return callbacks
	}
	if !i.inPaste {
		if start := strings.Index(data, "\x1b[200~"); start >= 0 {
			if start > 0 {
				callbacks = append(callbacks, i.handleInputLocked(data[:start])...)
			}
			i.inPaste = true
			i.pasteBuffer = ""
			callbacks = append(callbacks, i.handleInputLocked(data[start+len("\x1b[200~"):])...)
			return callbacks
		}
	}
	if i.inPaste {
		i.pasteBuffer += data
		if end := strings.Index(i.pasteBuffer, "\x1b[201~"); end >= 0 {
			pasteContent := i.pasteBuffer[:end]
			remaining := i.pasteBuffer[end+len("\x1b[201~"):]
			i.inPaste = false
			i.pasteBuffer = ""
			callbacks = append(callbacks, i.handlePaste(pasteContent)...)
			if remaining != "" {
				callbacks = append(callbacks, i.handleInputLocked(remaining)...)
			}
		}
		return callbacks
	}

	kb := GetKeybindings()

	switch {
	case kb.Matches(data, "tui.select.cancel"):
		if i.OnEscape != nil {
			onEscape := i.OnEscape
			callbacks = append(callbacks, func() { onEscape() })
		}
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.undo"):
		callbacks = append(callbacks, i.undo()...)
	case kb.Matches(data, "tui.input.submit") || data == "\n":
		if i.OnSubmit != nil {
			onSubmit := i.OnSubmit
			text := i.text
			callbacks = append(callbacks, func() { onSubmit(text) })
		}
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.deleteCharBackward"):
		runes := []rune(i.text)
		if i.cursor > 0 && len(runes) > 0 {
			start := previousGraphemeBoundary(runes, i.cursor)
			i.pushUndo()
			runes = append(runes[:start], runes[i.cursor:]...)
			i.cursor = start
			i.text = string(runes)
			callbacks = append(callbacks, i.changedCallbacks()...)
		}
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.deleteCharForward"):
		runes := []rune(i.text)
		if i.cursor < len(runes) {
			end := nextGraphemeBoundary(runes, i.cursor)
			i.pushUndo()
			runes = append(runes[:i.cursor], runes[end:]...)
			i.text = string(runes)
			callbacks = append(callbacks, i.changedCallbacks()...)
		}
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.deleteWordBackward"):
		callbacks = append(callbacks, i.killWordBackward()...)
	case kb.Matches(data, "tui.editor.deleteWordForward"):
		callbacks = append(callbacks, i.killWordForward()...)
	case kb.Matches(data, "tui.editor.deleteToLineStart"):
		callbacks = append(callbacks, i.killRange(0, i.cursor, true)...)
	case kb.Matches(data, "tui.editor.deleteToLineEnd"):
		callbacks = append(callbacks, i.killRange(i.cursor, len([]rune(i.text)), false)...)
	case kb.Matches(data, "tui.editor.yank"):
		callbacks = append(callbacks, i.yank()...)
	case kb.Matches(data, "tui.editor.yankPop"):
		callbacks = append(callbacks, i.yankPop()...)
	case kb.Matches(data, "tui.editor.cursorLeft"):
		i.cursor = previousGraphemeBoundary([]rune(i.text), i.cursor)
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorRight"):
		i.cursor = nextGraphemeBoundary([]rune(i.text), i.cursor)
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorLineStart"):
		i.cursor = 0
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorLineEnd"):
		i.cursor = len([]rune(i.text))
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorWordLeft"):
		i.cursor = i.wordBackward()
		i.lastAction = ""
		i.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorWordRight"):
		i.cursor = i.wordForward()
		i.lastAction = ""
		i.breakKillAndYank()
	default:
		if isPlainPrintableText(data) {
			i.insertStringWithUndo(data)
			callbacks = append(callbacks, i.changedCallbacks()...)
			i.breakKillAndYank()
			return callbacks
		}
		event := ParseKey(data)
		if event.Rune != 0 && isPlainPrintableRune(event.Rune) && !event.Ctrl && !event.Alt && !event.Super {
			i.insertStringWithUndo(string(event.Rune))
			callbacks = append(callbacks, i.changedCallbacks()...)
			i.breakKillAndYank()
		}
	}
	return callbacks
}

func (i *Input) changed() {
	runInputCallbacks(i.changedCallbacks())
}

func (i *Input) changedCallbacks() []inputCallback {
	if i.OnChange == nil {
		return nil
	}
	onChange := i.OnChange
	text := i.text
	return []inputCallback{func() { onChange(text) }}
}

func runInputCallbacks(callbacks []inputCallback) {
	for _, callback := range callbacks {
		if callback != nil {
			callback()
		}
	}
}

func (i *Input) breakKillAndYank() {
	i.lastKill = false
	i.lastYank = false
}

func (i *Input) insertString(text string) {
	runes := []rune(i.text)
	insert := []rune(text)
	pos := min(i.cursor, len(runes))
	next := make([]rune, 0, len(runes)+len(insert))
	next = append(next, runes[:pos]...)
	next = append(next, insert...)
	next = append(next, runes[pos:]...)
	i.cursor += len(insert)
	i.text = string(next)
}

func (i *Input) insertStringWithUndo(text string) {
	if text == "" {
		return
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return
	}
	if containsWhitespaceRune(text) || i.lastAction != "type-word" {
		i.pushUndo()
	}
	i.lastAction = "type-word"
	i.insertString(text)
}

func (i *Input) killRange(start, end int, backward bool) []inputCallback {
	runes := []rune(i.text)
	start = max(0, min(start, len(runes)))
	end = max(start, min(end, len(runes)))
	if start == end {
		i.lastYank = false
		return nil
	}
	i.pushUndo()
	killed := string(runes[start:end])
	next := append([]rune{}, runes[:start]...)
	next = append(next, runes[end:]...)
	i.text = string(next)
	i.cursor = start
	i.recordKill(killed, backward)
	return i.changedCallbacks()
}

func (i *Input) killWordBackward() []inputCallback {
	start, end := i.wordBackwardDeleteRange()
	return i.killRange(start, end, true)
}

func (i *Input) killWordForward() []inputCallback {
	start, end := i.wordForwardDeleteRange()
	return i.killRange(start, end, false)
}

func (i *Input) recordKill(text string, backward bool) {
	if text == "" {
		return
	}
	if i.lastKill && len(i.killRing) > 0 {
		if backward {
			i.killRing[0] = text + i.killRing[0]
		} else {
			i.killRing[0] += text
		}
	} else {
		i.killRing = append([]string{text}, i.killRing...)
	}
	i.killIndex = 0
	i.lastKill = true
	i.lastYank = false
}

func (i *Input) yank() []inputCallback {
	if len(i.killRing) == 0 {
		i.lastKill = false
		return nil
	}
	i.pushUndo()
	i.killIndex = 0
	text := i.killRing[i.killIndex]
	i.insertString(text)
	i.lastYank = true
	i.lastKill = false
	i.lastYankWidth = len([]rune(text))
	return i.changedCallbacks()
}

func (i *Input) yankPop() []inputCallback {
	if !i.lastYank || len(i.killRing) <= 1 {
		return nil
	}
	i.pushUndo()
	runes := []rune(i.text)
	start := max(0, i.cursor-i.lastYankWidth)
	runes = append(runes[:start], runes[i.cursor:]...)
	i.text = string(runes)
	i.cursor = start
	i.rotateKillRing()
	i.killIndex = 0
	text := i.killRing[0]
	i.insertString(text)
	i.lastYankWidth = len([]rune(text))
	i.lastYank = true
	i.lastKill = false
	return i.changedCallbacks()
}

func (i *Input) rotateKillRing() {
	if len(i.killRing) <= 1 {
		return
	}
	first := i.killRing[0]
	copy(i.killRing, i.killRing[1:])
	i.killRing[len(i.killRing)-1] = first
}

func (i *Input) handlePaste(pastedText string) []inputCallback {
	clean := normalizeEditorText(pastedText)
	clean = strings.ReplaceAll(clean, "\n", "")
	if clean == "" {
		return nil
	}
	i.pushUndo()
	i.lastAction = ""
	i.breakKillAndYank()
	i.insertString(clean)
	return i.changedCallbacks()
}

func (i *Input) pushUndo() {
	snapshot := inputSnapshot{text: i.text, cursor: i.cursor}
	if len(i.undoStack) > 0 {
		last := i.undoStack[len(i.undoStack)-1]
		if last.text == snapshot.text && last.cursor == snapshot.cursor {
			return
		}
	}
	i.undoStack = append(i.undoStack, snapshot)
	if len(i.undoStack) > 200 {
		i.undoStack = i.undoStack[len(i.undoStack)-200:]
	}
}

func (i *Input) undo() []inputCallback {
	if len(i.undoStack) == 0 {
		return nil
	}
	snapshot := i.undoStack[len(i.undoStack)-1]
	i.undoStack = i.undoStack[:len(i.undoStack)-1]
	i.text = snapshot.text
	i.cursor = min(snapshot.cursor, len([]rune(i.text)))
	i.lastAction = ""
	i.breakKillAndYank()
	return i.changedCallbacks()
}

func (i *Input) wordBackwardDeleteRange() (int, int) {
	if i.cursor <= 0 {
		return 0, 0
	}
	end := min(i.cursor, len([]rune(i.text)))
	return FindWordBackward(i.text, end), end
}

func (i *Input) wordForwardDeleteRange() (int, int) {
	runes := []rune(i.text)
	if i.cursor >= len(runes) {
		return i.cursor, i.cursor
	}
	return i.cursor, i.wordForward()
}

func (i *Input) wordBackward() int {
	start, _ := i.wordBackwardDeleteRange()
	return start
}

func (i *Input) wordForward() int {
	return FindWordForward(i.text, i.cursor)
}
