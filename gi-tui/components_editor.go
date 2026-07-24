package gitui

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

type EditorOptions struct {
	PaddingX               int
	AutocompleteMaxVisible int
	AutocompleteDebounce   time.Duration
	MaxVisibleLines        int
	Borderless             bool
}

type EditorTheme struct {
	Border     func(string) string
	SelectList SelectListTheme
}

var slashCommandSelectListLayout = SelectListLayoutOptions{
	MinPrimaryColumnWidth: 12,
	MaxPrimaryColumnWidth: 32,
}

// EditorComponent is the core contract for editor-like input widgets.
// Optional Pi editor capabilities are modeled as small extension interfaces
// below so custom Go editors can implement only the features they support.
type EditorComponent interface {
	Component
	GetText() string
	SetText(text string)
	HandleInput(data string)
}

type EditorHistoryComponent interface {
	AddToHistory(text string)
}

type EditorTextInserter interface {
	InsertTextAtCursor(text string)
}

type EditorExpandedTextProvider interface {
	GetExpandedText() string
}

type EditorAutocompleteComponent interface {
	SetAutocompleteProvider(provider AutocompleteProvider)
}

type EditorAppearanceComponent interface {
	SetPaddingX(padding int)
	SetAutocompleteMaxVisible(maxVisible int)
	SetBorderColor(fn func(string) string)
}

type EditorSubmitCallbackComponent interface {
	SetOnSubmit(fn func(string))
}

type EditorChangeCallbackComponent interface {
	SetOnChange(fn func(string))
}

type Editor struct {
	FocusState
	mu                          sync.Mutex
	text                        string
	cursor                      int
	theme                       EditorTheme
	options                     EditorOptions
	autocomplete                *AutocompleteSuggestions
	autocompleteList            *SelectList
	autocompleteForce           bool
	autocompleteProvider        AutocompleteProvider
	autocompleteTriggers        []rune
	autocompleteTriggerPattern  *regexp.Regexp
	autocompleteDebouncePattern *regexp.Regexp
	autocompleteTimer           *time.Timer
	autocompleteCancel          context.CancelFunc
	autocompleteToken           int
	autocompleteRequest         int
	history                     []string
	historyIndex                int
	historyDraft                *editorSnapshot
	undoStack                   []editorSnapshot
	lastAction                  string
	pastes                      map[int]string
	pasteCounter                int
	pasteBuffer                 string
	inPaste                     bool
	killRing                    []string
	killIndex                   int
	lastKill                    bool
	lastYank                    bool
	lastYankWidth               int
	jumpDirection               int
	preferredColumn             int
	hasPreferredColumn          bool
	snappedFromLine             int
	snappedFromColumn           int
	hasSnappedFromColumn        bool
	lastLayoutWidth             int
	scrollOffset                int
	OnSubmit                    func(string)
	OnChange                    func(string)
	OnAutocompleteChange        func()
	DisableSubmit               bool
	pendingCallbacks            []func()
}

type editorSnapshot struct {
	text         string
	cursor       int
	pastes       map[int]string
	pasteCounter int
}

type editorAutocompleteFullProvider interface {
	GetSuggestions(lines []string, cursorLine, cursorCol int, force bool) (*AutocompleteSuggestions, error)
	ApplyCompletion(lines []string, cursorLine, cursorCol int, item AutocompleteItem, prefix string) CompletionResult
}

type editorAutocompleteContextProvider interface {
	GetSuggestionsContext(ctx context.Context, lines []string, cursorLine, cursorCol int, force bool) (*AutocompleteSuggestions, error)
	ApplyCompletion(lines []string, cursorLine, cursorCol int, item AutocompleteItem, prefix string) CompletionResult
}

type editorAutocompleteTriggerProvider interface {
	ShouldTriggerFileCompletion(lines []string, cursorLine, cursorCol int) bool
}

const attachmentAutocompleteDebounce = 20 * time.Millisecond

var (
	defaultAutocompleteTriggerCharacters = []rune{'@', '#'}
	pasteMarkerPattern                   = regexp.MustCompile(`\[paste #([0-9]+)( (\+[0-9]+ lines|[0-9]+ chars))?\]`)
	editorCSIuControlPattern             = regexp.MustCompile(`\x1b\[([0-9]+);5u`)
	markdownAutoURIPattern               = regexp.MustCompile(`(?i)<([a-z][a-z0-9+.-]{1,31}:[^<>\x00-\x1f\s]*)>`)
	markdownAutoEmailPattern             = regexp.MustCompile("(?i)<([A-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?(?:\\.[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?)+)>")
	markdownBareURLPattern               = regexp.MustCompile(`(?i)(?:https?://|ftp://|www\.)(?:[A-Z0-9\-]+\.?)+[^\s<]*`)
	markdownBareEmailPattern             = regexp.MustCompile(`(?i)\b[A-Z0-9._+\-]+@[A-Z0-9_\-]+(?:\.[A-Z0-9_\-]*[A-Z0-9])+`)
	markdownBareEmailFullPattern         = regexp.MustCompile(`(?i)^[A-Z0-9._+\-]+@[A-Z0-9_\-]+(?:\.[A-Z0-9_\-]*[A-Z0-9])+$`)
)

func NewEditor(theme EditorTheme, options ...EditorOptions) *Editor {
	opts := EditorOptions{PaddingX: 0, AutocompleteMaxVisible: 5, AutocompleteDebounce: attachmentAutocompleteDebounce}
	if len(options) > 0 {
		opts = options[0]
	}
	opts = normalizeEditorOptions(opts)
	editor := &Editor{theme: theme, options: opts, historyIndex: -1, pastes: map[int]string{}, lastLayoutWidth: 80}
	editor.setAutocompleteTriggerCharacters(nil)
	return editor
}

func escapeCharacterClass(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`^`, `\^`,
		`$`, `\$`,
		`.`, `\.`,
		`*`, `\*`,
		`+`, `\+`,
		`?`, `\?`,
		`(`, `\(`,
		`)`, `\)`,
		`[`, `\[`,
		`]`, `\]`,
		`{`, `\{`,
		`}`, `\}`,
		`|`, `\|`,
		`-`, `\-`,
	)
	return replacer.Replace(value)
}

func buildTriggerPattern(triggerCharacters []rune) *regexp.Regexp {
	var characters strings.Builder
	for _, character := range triggerCharacters {
		characters.WriteString(escapeCharacterClass(string(character)))
	}
	return regexp.MustCompile(`(^|[[:space:]])[` + characters.String() + `][^[:space:]]*$`)
}

func buildDebouncePattern(triggerCharacters []rune) *regexp.Regexp {
	var alternatives []string
	var otherCharacters strings.Builder
	for _, character := range triggerCharacters {
		if character == '@' {
			alternatives = append(alternatives, `@("[^"]*|[^[:space:]]*)`)
			continue
		}
		otherCharacters.WriteString(escapeCharacterClass(string(character)))
	}
	if otherCharacters.Len() > 0 {
		alternatives = append(alternatives, `[`+otherCharacters.String()+`][^[:space:]]*`)
	}
	if len(alternatives) == 0 {
		return regexp.MustCompile(`a^`)
	}
	return regexp.MustCompile(`(^|[ \t])(` + strings.Join(alternatives, "|") + `)$`)
}

func createScrollBorder(direction string, hiddenLineCount, width int) string {
	availableWidth := max(0, width)
	indicator := fmt.Sprintf("─── %s %d more ", direction, hiddenLineCount)
	remaining := availableWidth - VisibleWidth(indicator)
	if remaining >= 0 {
		return indicator + strings.Repeat("─", remaining)
	}
	ellipsis := "..."[:min(3, availableWidth)]
	indicatorWidth := max(0, availableWidth-VisibleWidth(ellipsis))
	return SliceByColumn(indicator, 0, indicatorWidth, true) + ellipsis
}

func normalizeEditorOptions(opts EditorOptions) EditorOptions {
	opts.PaddingX = max(0, opts.PaddingX)
	if opts.AutocompleteMaxVisible == 0 {
		opts.AutocompleteMaxVisible = 5
	} else {
		opts.AutocompleteMaxVisible = max(3, min(20, opts.AutocompleteMaxVisible))
	}
	if opts.AutocompleteDebounce == 0 {
		opts.AutocompleteDebounce = attachmentAutocompleteDebounce
	}
	opts.MaxVisibleLines = max(0, opts.MaxVisibleLines)
	return opts
}

func (e *Editor) GetPaddingX() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.options.PaddingX
}

func (e *Editor) SetPaddingX(padding int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.options.PaddingX = max(0, padding)
}

func (e *Editor) SetBorderColor(fn func(string) string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.theme.Border = fn
}

func (e *Editor) GetAutocompleteMaxVisible() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.options.AutocompleteMaxVisible
}

func (e *Editor) SetAutocompleteMaxVisible(maxVisible int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if maxVisible == 0 {
		e.options.AutocompleteMaxVisible = 5
		return
	}
	e.options.AutocompleteMaxVisible = max(3, min(20, maxVisible))
}

func (e *Editor) GetMaxVisibleLines() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.options.MaxVisibleLines
}

func (e *Editor) SetMaxVisibleLines(lines int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.options.MaxVisibleLines = max(0, lines)
}

func (e *Editor) Text() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.text
}

func (e *Editor) GetText() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.text
}

func (e *Editor) GetLines() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), strings.Split(e.text, "\n")...)
}

func (e *Editor) GetCursor() (line, col int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	lines, cursorLine, cursorCol := e.linesAndCursor()
	if len(lines) == 0 {
		return 0, 0
	}
	return cursorLine, cursorCol
}

func (e *Editor) GetExpandedText() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.getExpandedTextLocked()
}

func (e *Editor) getExpandedTextLocked() string {
	if len(e.pastes) == 0 || !strings.Contains(e.text, "[paste #") {
		return e.text
	}
	return pasteMarkerPattern.ReplaceAllStringFunc(e.text, func(marker string) string {
		matches := pasteMarkerPattern.FindStringSubmatch(marker)
		if len(matches) < 2 {
			return marker
		}
		id, err := strconv.Atoi(matches[1])
		if err != nil {
			return marker
		}
		if paste, ok := e.pastes[id]; ok {
			return paste
		}
		return marker
	})
}

func (e *Editor) SetText(text string) {
	e.mu.Lock()
	normalized := normalizeEditorText(text)
	if e.text != normalized {
		e.pushUndoSnapshot()
	}
	e.cancelAutocomplete()
	e.setTextInternal(normalized)
	e.exitHistoryBrowsing()
	e.lastAction = ""
	e.resetPreferredColumn()
	e.breakKillAndYank()
	e.changed()
	callbacks := e.takePendingCallbacksLocked()
	e.mu.Unlock()
	runEditorCallbacks(callbacks)
}

func (e *Editor) SetAutocompleteProvider(provider AutocompleteProvider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cancelAutocomplete()
	e.autocompleteProvider = provider
	var triggerCharacters []rune
	if triggered, ok := provider.(AutocompleteTriggerCharactersProvider); ok {
		triggerCharacters = triggered.AutocompleteTriggerCharacters()
	}
	e.setAutocompleteTriggerCharacters(triggerCharacters)
}

func (e *Editor) setAutocompleteTriggerCharacters(triggerCharacters []rune) {
	next := append([]rune(nil), defaultAutocompleteTriggerCharacters...)
	seen := map[rune]struct{}{'@': {}, '#': {}}
	for _, character := range triggerCharacters {
		if character == '/' || unicode.IsSpace(character) {
			continue
		}
		if _, exists := seen[character]; exists {
			continue
		}
		seen[character] = struct{}{}
		next = append(next, character)
	}
	e.autocompleteTriggers = next
	e.autocompleteTriggerPattern = buildTriggerPattern(next)
	e.autocompleteDebouncePattern = buildDebouncePattern(next)
}

func (e *Editor) isAutocompleteTriggerCharacter(character rune) bool {
	for _, trigger := range e.autocompleteTriggers {
		if trigger == character {
			return true
		}
	}
	return false
}

func (e *Editor) SetOnSubmit(fn func(string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.OnSubmit = fn
}

func (e *Editor) SetOnChange(fn func(string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.OnChange = fn
}

func (e *Editor) IsShowingAutocomplete() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.isShowingAutocompleteLocked()
}

func (e *Editor) isShowingAutocompleteLocked() bool {
	return e.autocomplete != nil && e.autocompleteList != nil
}

func (e *Editor) takePendingCallbacksLocked() []func() {
	callbacks := e.pendingCallbacks
	e.pendingCallbacks = nil
	return callbacks
}

func runEditorCallbacks(callbacks []func()) {
	for _, callback := range callbacks {
		if callback != nil {
			callback()
		}
	}
}

func (e *Editor) InsertTextAtCursor(text string) {
	e.mu.Lock()
	if text == "" {
		e.mu.Unlock()
		return
	}
	e.cancelAutocomplete()
	e.pushUndoSnapshot()
	e.insertTextInternal(normalizeEditorText(text))
	e.exitHistoryBrowsing()
	e.lastAction = ""
	e.resetPreferredColumn()
	e.breakKillAndYank()
	callbacks := e.takePendingCallbacksLocked()
	e.mu.Unlock()
	runEditorCallbacks(callbacks)
}

func (e *Editor) PasteToEditor(text string) {
	e.mu.Lock()
	e.handlePaste(text)
	callbacks := e.takePendingCallbacksLocked()
	e.mu.Unlock()
	runEditorCallbacks(callbacks)
}

func (e *Editor) Invalidate() {}
func (e *Editor) Render(width int) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.render(width, 0)
}

func (e *Editor) RenderWithSize(width, height int) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.render(width, height)
}

func (e *Editor) render(width, height int) []string {
	if width <= 0 {
		width = 1
	}
	maxPadding := max(0, (width-1)/2)
	paddingX := min(max(0, e.options.PaddingX), maxPadding)
	contentWidth := max(1, width-paddingX*2)
	layoutWidth := contentWidth
	if paddingX == 0 {
		layoutWidth = max(1, contentWidth-1)
	}
	e.lastLayoutWidth = layoutWidth

	layoutLines := e.layoutEditorLines(layoutWidth)
	cursorLineIndex := 0
	for idx, line := range layoutLines {
		if line.hasCursor {
			cursorLineIndex = idx
			break
		}
	}
	maxVisibleLines := e.maxVisibleLines(height)
	if cursorLineIndex < e.scrollOffset {
		e.scrollOffset = cursorLineIndex
	} else if cursorLineIndex >= e.scrollOffset+maxVisibleLines {
		e.scrollOffset = cursorLineIndex - maxVisibleLines + 1
	}
	e.scrollOffset = max(0, min(e.scrollOffset, max(0, len(layoutLines)-maxVisibleLines)))
	visibleLines := layoutLines[e.scrollOffset:min(len(layoutLines), e.scrollOffset+maxVisibleLines)]

	lines := make([]string, 0, len(visibleLines)+2)
	if !e.options.Borderless {
		lines = append(lines, e.renderEditorTopBorder(width))
	}
	for _, layoutLine := range visibleLines {
		lines = append(lines, e.renderEditorContentLine(layoutLine, width, contentWidth, paddingX))
	}
	if !e.options.Borderless {
		lines = append(lines, e.renderEditorBottomBorder(width, max(0, len(layoutLines)-(e.scrollOffset+len(visibleLines)))))
	}
	if e.isShowingAutocompleteLocked() {
		left := strings.Repeat(" ", paddingX)
		right := left
		for _, line := range e.autocompleteList.Render(contentWidth) {
			lineWidth := VisibleWidth(line)
			lines = append(lines, left+line+strings.Repeat(" ", max(0, contentWidth-lineWidth))+right)
		}
	}
	return lines
}

func (e *Editor) maxVisibleLines(height int) int {
	if e.options.MaxVisibleLines > 0 {
		return e.options.MaxVisibleLines
	}
	if height > 0 {
		return max(5, height*3/10)
	}
	return 5
}

type editorLayoutLine struct {
	text      string
	hasCursor bool
	cursorPos int
}

func (e *Editor) layoutEditorLines(contentWidth int) []editorLayoutLine {
	textLines, cursorLine, cursorCol := e.linesAndCursor()
	if len(textLines) == 0 || (len(textLines) == 1 && textLines[0] == "") {
		return []editorLayoutLine{{text: "", hasCursor: true, cursorPos: 0}}
	}
	layout := make([]editorLayoutLine, 0, len(textLines))
	for lineIndex, line := range textLines {
		chunks := e.wrapEditorLine(line, contentWidth)
		for chunkIndex, chunk := range chunks {
			item := editorLayoutLine{text: chunk.Text}
			if lineIndex == cursorLine {
				cursorByte := runeColToByteIndex(line, cursorCol)
				isLastChunk := chunkIndex == len(chunks)-1
				if cursorByte >= chunk.StartIndex && (cursorByte < chunk.EndIndex || (isLastChunk && cursorByte == chunk.EndIndex)) {
					item.hasCursor = true
					item.cursorPos = max(0, min(len(chunk.Text), cursorByte-chunk.StartIndex))
				}
			}
			layout = append(layout, item)
		}
	}
	if len(layout) == 0 {
		return []editorLayoutLine{{text: "", hasCursor: true, cursorPos: 0}}
	}
	return layout
}

func (e *Editor) renderEditorTopBorder(width int) string {
	horizontal := style(e.theme.Border, "─")
	if e.scrollOffset > 0 {
		return style(e.theme.Border, createScrollBorder("↑", e.scrollOffset, width))
	}
	return strings.Repeat(horizontal, width)
}

func (e *Editor) renderEditorBottomBorder(width, linesBelow int) string {
	if linesBelow > 0 {
		return style(e.theme.Border, createScrollBorder("↓", linesBelow, width))
	}
	return strings.Repeat(style(e.theme.Border, "─"), width)
}

func (e *Editor) renderEditorContentLine(layoutLine editorLayoutLine, width, contentWidth, paddingX int) string {
	display := layoutLine.text
	lineWidth := VisibleWidth(display)
	cursorInPadding := false
	if layoutLine.hasCursor {
		before := display[:layoutLine.cursorPos]
		after := display[layoutLine.cursorPos:]
		marker := ""
		if e.Focused() && !e.isShowingAutocompleteLocked() {
			marker = CursorMarker
		}
		if after != "" {
			size := firstGraphemeByteLen(after)
			cursor := "\x1b[7m" + after[:size] + "\x1b[0m"
			display = before + marker + cursor + after[size:]
		} else {
			display = before + marker + "\x1b[7m \x1b[0m"
			lineWidth++
			if lineWidth > contentWidth && paddingX > 0 {
				cursorInPadding = true
			}
		}
	}
	left := strings.Repeat(" ", paddingX)
	right := left
	if cursorInPadding && len(right) > 0 {
		right = right[:len(right)-1]
	}
	line := left + display + strings.Repeat(" ", max(0, contentWidth-lineWidth)) + right
	return TruncateToWidth(line, width, "", true)
}

func (e *Editor) wrapEditorLine(line string, width int) []TextChunk {
	if line == "" {
		return []TextChunk{{Text: "", StartIndex: 0, EndIndex: 0}}
	}
	if VisibleWidth(line) <= width {
		return []TextChunk{{Text: line, StartIndex: 0, EndIndex: len(line)}}
	}
	return wordWrapLineWithSegments(line, width, e.segmentLineForWrap(line))
}

func (e *Editor) HandleInput(data string) {
	e.mu.Lock()
	e.handleInputLocked(data)
	callbacks := e.takePendingCallbacksLocked()
	e.mu.Unlock()
	runEditorCallbacks(callbacks)
}

func (e *Editor) handleInputLocked(data string) {
	if data == "" {
		return
	}
	if !e.inPaste {
		if start := strings.Index(data, "\x1b[200~"); start >= 0 {
			if start > 0 {
				e.handleInputLocked(data[:start])
			}
			e.inPaste = true
			e.pasteBuffer = ""
			e.handleInputLocked(data[start+len("\x1b[200~"):])
			return
		}
	}
	if e.inPaste {
		e.pasteBuffer += data
		if end := strings.Index(e.pasteBuffer, "\x1b[201~"); end >= 0 {
			pasteContent := e.pasteBuffer[:end]
			remaining := e.pasteBuffer[end+len("\x1b[201~"):]
			e.inPaste = false
			e.pasteBuffer = ""
			if pasteContent != "" {
				e.handlePaste(pasteContent)
			}
			if remaining != "" {
				e.handleInputLocked(remaining)
			}
		}
		return
	}

	kb := GetKeybindings()

	if e.jumpDirection != 0 {
		if (e.jumpDirection > 0 && kb.Matches(data, "tui.editor.jumpForward")) || (e.jumpDirection < 0 && kb.Matches(data, "tui.editor.jumpBackward")) {
			e.jumpDirection = 0
			return
		}
		event := ParseKey(data)
		if event.Rune != 0 && !event.Ctrl && !event.Alt && !event.Super {
			e.jumpToRune(event.Rune, e.jumpDirection)
			e.jumpDirection = 0
			e.lastAction = ""
			e.breakKillAndYank()
			return
		}
		e.jumpDirection = 0
	}

	if kb.Matches(data, "tui.input.copy") {
		return
	}

	if kb.Matches(data, "tui.editor.undo") {
		e.undo()
		return
	}

	if e.isShowingAutocompleteLocked() {
		switch {
		case kb.Matches(data, "tui.select.cancel"):
			e.cancelAutocomplete()
			return
		case kb.Matches(data, "tui.select.up"), kb.Matches(data, "tui.select.down"):
			e.autocompleteList.HandleInput(data)
			return
		case kb.Matches(data, "tui.input.tab"):
			e.applySelectedAutocomplete()
			return
		case kb.Matches(data, "tui.select.confirm"):
			autocompletePrefix := ""
			if e.autocomplete != nil {
				autocompletePrefix = e.autocomplete.Prefix
			}
			if strings.HasPrefix(autocompletePrefix, "/") && !e.autocompletePrefixMatchesCurrentCursor() {
				e.cancelAutocomplete()
				break
			}
			if !e.applySelectedAutocompleteWithNotify(!strings.HasPrefix(autocompletePrefix, "/")) {
				return
			}
			if !strings.HasPrefix(autocompletePrefix, "/") {
				return
			}
		}
	}

	switch {
	case kb.Matches(data, "tui.input.tab"):
		e.handleTabCompletion()
	case kb.Matches(data, "tui.editor.deleteToLineEnd"):
		start, end := e.deleteToLineEndRange()
		e.killRange(start, end, false)
	case kb.Matches(data, "tui.editor.deleteToLineStart"):
		start, end := e.deleteToLineStartRange()
		e.killRange(start, end, true)
	case kb.Matches(data, "tui.editor.deleteWordBackward"):
		e.killWordBackward()
	case kb.Matches(data, "tui.editor.deleteWordForward"):
		e.killWordForward()
	case kb.Matches(data, "tui.editor.deleteCharBackward") || MatchesKey(data, "shift+backspace"):
		e.backspace()
		e.exitHistoryBrowsing()
	case kb.Matches(data, "tui.editor.deleteCharForward") || MatchesKey(data, "shift+delete"):
		e.deleteForward()
		e.exitHistoryBrowsing()
	case kb.Matches(data, "tui.editor.yank"):
		e.yank()
	case kb.Matches(data, "tui.editor.yankPop"):
		e.yankPop()
	case kb.Matches(data, "tui.editor.cursorLineStart"):
		e.cursor = e.currentLineStart()
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorLineEnd"):
		e.cursor = e.currentLineEnd()
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorWordLeft"):
		e.cursor = e.wordBackward()
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorWordRight"):
		e.cursor = e.wordForward()
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
	case e.shouldSubmitSlashCommandInput(data, kb):
		e.submitLocked()
	case isEditorNewLineInput(data, kb):
		e.insertRuneWithUndo('\n')
		e.exitHistoryBrowsing()
	case kb.Matches(data, "tui.input.submit"):
		e.submitLocked()
	case kb.Matches(data, "tui.editor.cursorUp"):
		e.handleUp()
	case kb.Matches(data, "tui.editor.cursorDown"):
		e.handleDown()
	case kb.Matches(data, "tui.editor.cursorRight"):
		previous := e.cursor
		e.cursor = e.rightCursor()
		e.lastAction = ""
		if e.cursor == previous && previous == len([]rune(e.text)) {
			e.capturePreferredColumnFromCurrentVisualLine()
		} else {
			e.resetPreferredColumn()
		}
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.cursorLeft"):
		e.cursor = e.leftCursor()
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.pageUp"):
		e.pageScroll(-1)
		e.lastAction = ""
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.pageDown"):
		e.pageScroll(1)
		e.lastAction = ""
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.jumpForward"):
		e.jumpDirection = 1
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
	case kb.Matches(data, "tui.editor.jumpBackward"):
		e.jumpDirection = -1
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
	default:
		if isPlainPrintableText(data) {
			e.insertTextWithTypingUndo(data)
			e.updateAutocompleteAfterTextInput(data)
			return
		}
		event := ParseKey(data)
		if event.Rune != 0 && isPlainPrintableRune(event.Rune) && !event.Ctrl && !event.Alt && !event.Super {
			e.insertRuneWithUndo(event.Rune)
			e.updateAutocompleteAfterRune(event.Rune)
			e.exitHistoryBrowsing()
		}
	}
}

func (e *Editor) shouldSubmitSlashCommandInput(data string, kb *KeybindingsManager) bool {
	if e == nil || kb == nil {
		return false
	}
	if !kb.Matches(data, "tui.input.submit") || kb.Matches(data, "tui.input.newLine") {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(e.getExpandedTextLocked()), "/")
}

func (e *Editor) submitLocked() {
	if e.DisableSubmit {
		return
	}
	if e.replaceBackslashBeforeCursorWithNewline() {
		return
	}
	if e.OnSubmit != nil {
		fn := e.OnSubmit
		text := strings.TrimSpace(e.getExpandedTextLocked())
		e.pendingCallbacks = append(e.pendingCallbacks, func() { fn(text) })
	}
	e.setTextInternal("")
	e.pastes = map[int]string{}
	e.pasteCounter = 0
	e.undoStack = nil
	e.exitHistoryBrowsing()
	e.lastAction = ""
	e.resetPreferredColumn()
	e.breakKillAndYank()
	e.changed()
}

func (e *Editor) AddToHistory(text string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}
	if len(e.history) > 0 && e.history[0] == trimmed {
		return
	}
	e.history = append([]string{trimmed}, e.history...)
	if len(e.history) > 100 {
		e.history = e.history[:100]
	}
}

func (e *Editor) handleUp() {
	if len(e.history) > 0 &&
		e.isOnFirstVisualLine() &&
		(e.text == "" || e.historyIndex >= 0 || e.cursor == e.currentLineStart()) {
		newIndex := e.historyIndex + 1
		if newIndex >= len(e.history) {
			return
		}
		if e.historyIndex == -1 {
			e.pushUndoSnapshot()
			draft := editorSnapshot{
				text:         e.text,
				cursor:       e.cursor,
				pastes:       clonePasteMap(e.pastes),
				pasteCounter: e.pasteCounter,
			}
			e.historyDraft = &draft
		}
		e.historyIndex = newIndex
		e.text = e.history[e.historyIndex]
		e.cursor = 0
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
		e.changed()
		return
	}
	if e.isOnFirstVisualLine() {
		e.cursor = e.currentLineStart()
		e.resetPreferredColumn()
		e.lastAction = ""
		e.breakKillAndYank()
		return
	}
	e.moveVertical(-1)
	e.lastAction = ""
	e.breakKillAndYank()
}

func (e *Editor) handleDown() {
	if e.historyIndex >= 0 && e.isOnLastVisualLine() {
		newIndex := e.historyIndex - 1
		if newIndex < -1 {
			return
		}
		e.historyIndex = newIndex
		if newIndex < 0 {
			draft := e.historyDraft
			e.historyDraft = nil
			if draft == nil {
				e.text = ""
				e.cursor = 0
			} else {
				e.text = draft.text
				e.cursor = min(draft.cursor, len([]rune(draft.text)))
				e.pastes = clonePasteMap(draft.pastes)
				e.pasteCounter = draft.pasteCounter
				e.scrollOffset = 0
			}
		} else {
			e.text = e.history[newIndex]
			e.cursor = len([]rune(e.text))
		}
		e.lastAction = ""
		e.resetPreferredColumn()
		e.breakKillAndYank()
		e.changed()
		return
	}
	if e.isOnLastVisualLine() {
		e.cursor = e.currentLineEnd()
		e.resetPreferredColumn()
		e.lastAction = ""
		e.breakKillAndYank()
		return
	}
	e.moveVertical(1)
	e.lastAction = ""
	e.breakKillAndYank()
}

func (e *Editor) exitHistoryBrowsing() {
	e.historyIndex = -1
	e.historyDraft = nil
}

func (e *Editor) insertRuneWithUndo(r rune) {
	e.exitHistoryBrowsing()
	e.resetPreferredColumn()
	switch {
	case r == '\n':
		e.pushUndoSnapshot()
		e.lastAction = ""
	case unicode.IsSpace(r):
		e.pushUndoSnapshot()
		e.lastAction = "type-word"
	case e.lastAction != "type-word":
		e.pushUndoSnapshot()
		e.lastAction = "type-word"
	default:
		e.lastAction = "type-word"
	}
	e.insertRune(r)
	e.breakKillAndYank()
}

func (e *Editor) insertTextWithTypingUndo(text string) {
	if text == "" {
		return
	}
	e.exitHistoryBrowsing()
	e.resetPreferredColumn()
	if containsWhitespaceRune(text) || e.lastAction != "type-word" {
		e.pushUndoSnapshot()
	}
	e.lastAction = "type-word"
	e.insertTextInternal(text)
	e.breakKillAndYank()
}

func (e *Editor) insertRune(r rune) {
	runes := []rune(e.text)
	pos := min(e.cursor, len(runes))
	runes = append(runes[:pos], append([]rune{r}, runes[pos:]...)...)
	e.cursor++
	e.text = string(runes)
	e.changed()
}

func (e *Editor) replaceBackslashBeforeCursorWithNewline() bool {
	runes := []rune(e.text)
	if e.cursor <= 0 || e.cursor > len(runes) || runes[e.cursor-1] != '\\' {
		return false
	}
	e.cancelAutocomplete()
	e.pushUndoSnapshot()
	e.deleteRuneRange(e.cursor-1, e.cursor)
	e.insertTextInternal("\n")
	e.exitHistoryBrowsing()
	e.lastAction = ""
	e.resetPreferredColumn()
	e.breakKillAndYank()
	return true
}

func (e *Editor) insertTextInternal(text string) {
	if text == "" {
		return
	}
	runes := []rune(e.text)
	insert := []rune(text)
	pos := min(e.cursor, len(runes))
	next := make([]rune, 0, len(runes)+len(insert))
	next = append(next, runes[:pos]...)
	next = append(next, insert...)
	next = append(next, runes[pos:]...)
	e.text = string(next)
	e.cursor = pos + len(insert)
	e.changed()
}

func (e *Editor) setTextInternal(text string) {
	e.text = text
	e.cursor = len([]rune(text))
}

func (e *Editor) backspace() {
	runes := []rune(e.text)
	if e.cursor <= 0 || len(runes) == 0 {
		e.breakKillAndYank()
		e.updateAutocompleteAfterTextDeletion()
		return
	}
	start, end := e.markerSpanBeforeCursor()
	if start < 0 {
		start, end = previousGraphemeBoundary(runes, e.cursor), e.cursor
	}
	e.pushUndoSnapshot()
	e.deleteRuneRange(start, end)
	e.cursor = start
	e.exitHistoryBrowsing()
	e.lastAction = ""
	e.resetPreferredColumn()
	e.breakKillAndYank()
	e.changed()
	e.updateAutocompleteAfterTextDeletion()
}

func (e *Editor) deleteForward() {
	runes := []rune(e.text)
	if e.cursor >= len(runes) {
		e.breakKillAndYank()
		e.updateAutocompleteAfterTextDeletion()
		return
	}
	start, end := e.markerSpanAfterCursor()
	if start < 0 {
		start, end = e.cursor, nextGraphemeBoundary(runes, e.cursor)
	}
	e.pushUndoSnapshot()
	e.deleteRuneRange(start, end)
	e.cursor = start
	e.exitHistoryBrowsing()
	e.lastAction = ""
	e.resetPreferredColumn()
	e.breakKillAndYank()
	e.changed()
	e.updateAutocompleteAfterTextDeletion()
}

func (e *Editor) deleteRuneRange(start, end int) string {
	runes := []rune(e.text)
	start = max(0, min(start, len(runes)))
	end = max(start, min(end, len(runes)))
	deleted := string(runes[start:end])
	next := append([]rune{}, runes[:start]...)
	next = append(next, runes[end:]...)
	e.text = string(next)
	e.cursor = min(start, len(next))
	return deleted
}

func (e *Editor) killRange(start, end int, backward bool) {
	runes := []rune(e.text)
	start = max(0, min(start, len(runes)))
	end = max(start, min(end, len(runes)))
	if start == end {
		e.lastYank = false
		e.updateAutocompleteAfterTextDeletion()
		return
	}
	e.pushUndoSnapshot()
	killed := e.deleteRuneRange(start, end)
	e.cursor = start
	e.recordKill(killed, backward)
	e.exitHistoryBrowsing()
	e.lastAction = ""
	e.resetPreferredColumn()
	e.changed()
	e.updateAutocompleteAfterTextDeletion()
}

func (e *Editor) killWordBackward() {
	start, end := e.wordBackwardDeleteRange()
	e.killRange(start, end, true)
}

func (e *Editor) killWordForward() {
	start, end := e.wordForwardDeleteRange()
	e.killRange(start, end, false)
}

func (e *Editor) deleteToLineStartRange() (int, int) {
	if e.cursor <= 0 {
		return 0, 0
	}
	start := e.currentLineStart()
	if start == e.cursor && e.cursor > 0 {
		start = e.cursor - 1
	}
	return start, e.cursor
}

func (e *Editor) deleteToLineEndRange() (int, int) {
	runes := []rune(e.text)
	if e.cursor >= len(runes) {
		return e.cursor, e.cursor
	}
	end := e.currentLineEnd()
	if end == e.cursor && end < len(runes) && runes[end] == '\n' {
		end++
	}
	return e.cursor, end
}

func (e *Editor) wordBackwardDeleteRange() (int, int) {
	if e.cursor <= 0 {
		return 0, 0
	}
	if start, end := e.markerSpanBeforeCursor(); start >= 0 {
		return start, end
	}
	runes := []rune(e.text)
	end := e.cursor
	if runes[end-1] == '\n' {
		return end - 1, end
	}
	return e.wordBackward(), end
}

func (e *Editor) wordForwardDeleteRange() (int, int) {
	runes := []rune(e.text)
	if e.cursor >= len(runes) {
		return e.cursor, e.cursor
	}
	if _, end := e.markerSpanAfterCursor(); end >= 0 {
		return e.cursor, end
	}
	if runes[e.cursor] == '\n' {
		return e.cursor, e.cursor + 1
	}
	return e.cursor, e.wordForward()
}

func (e *Editor) recordKill(text string, backward bool) {
	if text == "" {
		return
	}
	if e.lastKill && len(e.killRing) > 0 {
		if backward {
			e.killRing[0] = text + e.killRing[0]
		} else {
			e.killRing[0] += text
		}
	} else {
		e.killRing = append([]string{text}, e.killRing...)
	}
	e.killIndex = 0
	e.lastKill = true
	e.lastYank = false
}

func (e *Editor) yank() {
	if len(e.killRing) == 0 {
		e.lastKill = false
		return
	}
	e.exitHistoryBrowsing()
	e.pushUndoSnapshot()
	e.killIndex = 0
	text := e.killRing[e.killIndex]
	e.insertTextInternal(text)
	e.lastYank = true
	e.lastKill = false
	e.lastYankWidth = len([]rune(text))
	e.lastAction = ""
	e.resetPreferredColumn()
}

func (e *Editor) yankPop() {
	if !e.lastYank || len(e.killRing) <= 1 {
		return
	}
	e.exitHistoryBrowsing()
	e.pushUndoSnapshot()
	runes := []rune(e.text)
	start := max(0, e.cursor-e.lastYankWidth)
	next := append([]rune{}, runes[:start]...)
	next = append(next, runes[e.cursor:]...)
	e.text = string(next)
	e.cursor = start
	e.rotateKillRing()
	e.killIndex = 0
	text := e.killRing[0]
	e.insertTextInternal(text)
	e.lastYankWidth = len([]rune(text))
	e.lastYank = true
	e.lastKill = false
	e.lastAction = ""
	e.resetPreferredColumn()
}

func (e *Editor) rotateKillRing() {
	if len(e.killRing) <= 1 {
		return
	}
	first := e.killRing[0]
	copy(e.killRing, e.killRing[1:])
	e.killRing[len(e.killRing)-1] = first
}

func (e *Editor) breakKillAndYank() {
	e.lastKill = false
	e.lastYank = false
}

func (e *Editor) handlePaste(pastedText string) {
	e.cancelAutocomplete()
	decoded := decodeCSIuControls(pastedText)
	filtered := filterEditorPaste(normalizeEditorText(decoded))
	if filtered == "" {
		return
	}
	if strings.ContainsAny(filtered[:1], "/~.") {
		runes := []rune(e.text)
		if e.cursor > 0 && e.cursor <= len(runes) && isWordRune(runes[e.cursor-1]) {
			filtered = " " + filtered
		}
	}

	e.pushUndoSnapshot()
	e.exitHistoryBrowsing()
	e.lastAction = ""
	e.resetPreferredColumn()
	e.breakKillAndYank()

	pastedLines := strings.Split(filtered, "\n")
	totalChars := len([]rune(filtered))
	if len(pastedLines) > 10 || totalChars > 1000 {
		e.pasteCounter++
		if e.pastes == nil {
			e.pastes = map[int]string{}
		}
		pasteID := e.pasteCounter
		e.pastes[pasteID] = filtered
		marker := fmt.Sprintf("[paste #%d %d chars]", pasteID, totalChars)
		if len(pastedLines) > 10 {
			marker = fmt.Sprintf("[paste #%d +%d lines]", pasteID, len(pastedLines))
		}
		e.insertTextInternal(marker)
		return
	}
	e.insertTextInternal(filtered)
}

func isEditorNewLineInput(data string, kb *KeybindingsManager) bool {
	if kb.Matches(data, "tui.input.newLine") {
		return true
	}
	if data == "\n" || data == "\x1b\r" || data == "\x1b[13;2~" {
		return true
	}
	return (len(data) > 1 && data[0] == '\n') || (len(data) > 1 && strings.Contains(data, "\x1b") && strings.Contains(data, "\r"))
}

func (e *Editor) handleTabCompletion() {
	if e.autocompleteProvider == nil {
		return
	}
	e.requestAutocomplete(true, true)
	if e.isShowingAutocompleteLocked() && len(e.autocomplete.Items) == 1 {
		e.applySelectedAutocomplete()
	}
}

func (e *Editor) updateAutocompleteAfterRune(r rune) {
	if e.autocompleteProvider == nil {
		return
	}
	if e.isShowingAutocompleteLocked() {
		e.requestAutocomplete(e.autocompleteForce)
		return
	}
	if r == '/' && e.isAtStartOfMessage() {
		e.requestAutocomplete(false)
		return
	}
	if e.isAutocompleteTriggerCharacter(r) {
		if e.symbolCompletionContext() {
			e.requestAutocomplete(false)
		}
		return
	}
	if isAutocompleteContinuationRune(r) && (e.inSlashCommandContext() || e.symbolCompletionContext()) {
		e.requestAutocomplete(false)
	}
}

func (e *Editor) updateAutocompleteAfterTextInput(text string) {
	runes := []rune(text)
	if len(runes) == 0 {
		return
	}
	e.updateAutocompleteAfterRune(runes[len(runes)-1])
}

func (e *Editor) updateAutocompleteAfterTextDeletion() {
	if e.autocompleteProvider == nil {
		return
	}
	if e.inSlashCommandContext() || e.symbolCompletionContext() || (e.isShowingAutocompleteLocked() && e.autocompleteForce) {
		e.requestAutocomplete(e.autocompleteForce)
		return
	}
	wasShowing := e.isShowingAutocompleteLocked()
	e.cancelAutocomplete()
	if wasShowing {
		e.notifyAutocompleteChanged()
	}
}

func (e *Editor) requestAutocomplete(force bool, explicitTab ...bool) {
	if e.autocompleteProvider == nil {
		return
	}
	lines, cursorLine, cursorCol := e.linesAndCursor()
	if force {
		if provider, ok := e.autocompleteProvider.(editorAutocompleteTriggerProvider); ok && !provider.ShouldTriggerFileCompletion(lines, cursorLine, cursorCol) {
			return
		}
	}
	tab := len(explicitTab) > 0 && explicitTab[0]
	e.cancelAutocompleteRequest()
	e.autocompleteToken++
	token := e.autocompleteToken
	if e.autocompleteDebounce(force, tab) > 0 {
		e.autocompleteTimer = time.AfterFunc(e.autocompleteDebounce(force, tab), func() {
			e.mu.Lock()
			e.startAutocompleteRequest(token, force, tab)
			callbacks := e.takePendingCallbacksLocked()
			e.mu.Unlock()
			runEditorCallbacks(callbacks)
		})
		return
	}
	e.startAutocompleteRequest(token, force, tab)
}

func (e *Editor) autocompleteDebounce(force, explicitTab bool) time.Duration {
	if force || explicitTab {
		return 0
	}
	lines, cursorLine, cursorCol := e.linesAndCursor()
	if cursorLine < 0 || cursorLine >= len(lines) {
		return 0
	}
	line := lines[cursorLine]
	if cursorCol < 0 {
		cursorCol = 0
	}
	if cursorCol > len([]rune(line)) {
		cursorCol = len([]rune(line))
	}
	before := string([]rune(line)[:cursorCol])
	if e.autocompleteDebouncePattern != nil && e.autocompleteDebouncePattern.MatchString(before) {
		return e.options.AutocompleteDebounce
	}
	return 0
}

func (e *Editor) startAutocompleteRequest(token int, force, explicitTab bool) {
	if token != e.autocompleteToken || e.autocompleteProvider == nil {
		return
	}
	lines, cursorLine, cursorCol := e.linesAndCursor()
	snapshotText := e.text
	snapshotLine := cursorLine
	snapshotCol := cursorCol
	e.autocompleteRequest++
	requestID := e.autocompleteRequest
	ctx, cancel := context.WithCancel(context.Background())
	e.autocompleteCancel = cancel
	if provider, ok := e.autocompleteProvider.(editorAutocompleteContextProvider); ok {
		linesCopy := append([]string(nil), lines...)
		go func() {
			suggestions, err := provider.GetSuggestionsContext(ctx, linesCopy, cursorLine, cursorCol, force)
			e.mu.Lock()
			e.finishAutocompleteRequest(requestID, ctx, snapshotText, snapshotLine, snapshotCol, force, explicitTab, suggestions, err)
			callbacks := e.takePendingCallbacksLocked()
			e.mu.Unlock()
			runEditorCallbacks(callbacks)
		}()
		return
	}
	var suggestions *AutocompleteSuggestions
	if provider, ok := e.autocompleteProvider.(editorAutocompleteFullProvider); ok {
		got, err := provider.GetSuggestions(lines, cursorLine, cursorCol, force)
		if err == nil {
			suggestions = got
		}
	} else {
		got := e.autocompleteProvider.Suggestions(e.text, e.cursor)
		if len(got.Items) > 0 {
			suggestions = &got
		}
	}
	e.finishAutocompleteRequest(requestID, ctx, snapshotText, snapshotLine, snapshotCol, force, explicitTab, suggestions, nil)
}

func (e *Editor) finishAutocompleteRequest(requestID int, ctx context.Context, snapshotText string, snapshotLine, snapshotCol int, force, explicitTab bool, suggestions *AutocompleteSuggestions, err error) {
	if ctx.Err() != nil || requestID != e.autocompleteRequest || e.text != snapshotText {
		return
	}
	_, currentLine, currentCol := e.linesAndCursor()
	if currentLine != snapshotLine || currentCol != snapshotCol {
		return
	}
	e.autocompleteCancel = nil
	if err != nil || suggestions == nil || len(suggestions.Items) == 0 {
		e.clearAutocompleteUI()
		e.notifyAutocompleteChanged()
		return
	}
	if force && explicitTab && len(suggestions.Items) == 1 {
		e.autocomplete = suggestions
		e.autocompleteForce = force
		e.autocompleteList = e.newAutocompleteList(suggestions.Prefix, []SelectItem{{Value: suggestions.Items[0].Value, Label: suggestions.Items[0].Label, Description: suggestions.Items[0].Description}})
		e.applySelectedAutocomplete()
		return
	}
	e.applyAutocompleteSuggestions(suggestions, force)
	e.notifyAutocompleteChanged()
}

func (e *Editor) applyAutocompleteSuggestions(suggestions *AutocompleteSuggestions, force bool) {
	e.autocomplete = suggestions
	e.autocompleteForce = force
	items := make([]SelectItem, 0, len(suggestions.Items))
	for _, item := range suggestions.Items {
		items = append(items, SelectItem{Value: item.Value, Label: item.Label, Description: item.Description})
	}
	e.autocompleteList = e.newAutocompleteList(suggestions.Prefix, items)
	if index := bestAutocompleteMatchIndex(suggestions.Items, suggestions.Prefix); index >= 0 {
		e.autocompleteList.SetSelectedIndex(index)
	}
}

func (e *Editor) newAutocompleteList(prefix string, items []SelectItem) *SelectList {
	if strings.HasPrefix(prefix, "/") {
		return NewSelectList(items, e.options.AutocompleteMaxVisible, e.theme.SelectList, slashCommandSelectListLayout)
	}
	return NewSelectList(items, e.options.AutocompleteMaxVisible, e.theme.SelectList)
}

func (e *Editor) autocompletePrefixMatchesCurrentCursor() bool {
	if e == nil || e.autocomplete == nil {
		return false
	}
	lines, cursorLine, cursorCol := e.linesAndCursor()
	if cursorLine < 0 || cursorLine >= len(lines) {
		return false
	}
	lineRunes := []rune(lines[cursorLine])
	start := max(0, min(e.autocomplete.Start, len(lineRunes)))
	end := max(start, min(e.autocomplete.End, len(lineRunes)))
	if end != cursorCol {
		return false
	}
	return string(lineRunes[start:end]) == e.autocomplete.Prefix
}

func (e *Editor) applySelectedAutocomplete() {
	e.applySelectedAutocompleteWithNotify(true)
}

func (e *Editor) applySelectedAutocompleteWithNotify(notify bool) bool {
	if !e.isShowingAutocompleteLocked() {
		return false
	}
	selected, ok := e.autocompleteList.SelectedItem()
	if !ok {
		e.cancelAutocomplete()
		return false
	}
	item := AutocompleteItem{Value: selected.Value, Label: selected.Label, Description: selected.Description}
	e.pushUndoSnapshot()
	if provider, ok := e.autocompleteProvider.(editorAutocompleteContextProvider); ok {
		lines, cursorLine, cursorCol := e.linesAndCursor()
		result := provider.ApplyCompletion(lines, cursorLine, cursorCol, item, e.autocomplete.Prefix)
		e.text = strings.Join(result.Lines, "\n")
		e.cursor = cursorFromLineCol(result.Lines, result.CursorLine, result.CursorCol)
	} else if provider, ok := e.autocompleteProvider.(editorAutocompleteFullProvider); ok {
		lines, cursorLine, cursorCol := e.linesAndCursor()
		result := provider.ApplyCompletion(lines, cursorLine, cursorCol, item, e.autocomplete.Prefix)
		e.text = strings.Join(result.Lines, "\n")
		e.cursor = cursorFromLineCol(result.Lines, result.CursorLine, result.CursorCol)
	} else {
		e.applyFlatCompletion(item)
	}
	e.exitHistoryBrowsing()
	e.lastAction = ""
	e.resetPreferredColumn()
	e.cancelAutocomplete()
	e.breakKillAndYank()
	if notify {
		e.changed()
	}
	return true
}

func (e *Editor) applyFlatCompletion(item AutocompleteItem) {
	runes := []rune(e.text)
	start := max(0, min(e.autocomplete.Start, len(runes)))
	end := max(start, min(e.autocomplete.End, len(runes)))
	value := []rune(item.Value)
	next := append([]rune{}, runes[:start]...)
	next = append(next, value...)
	next = append(next, runes[end:]...)
	e.text = string(next)
	e.cursor = start + len(value)
}

func (e *Editor) cancelAutocomplete() {
	e.cancelAutocompleteRequest()
	e.clearAutocompleteUI()
}

func (e *Editor) cancelAutocompleteRequest() {
	e.autocompleteToken++
	if e.autocompleteTimer != nil {
		e.autocompleteTimer.Stop()
		e.autocompleteTimer = nil
	}
	if e.autocompleteCancel != nil {
		e.autocompleteCancel()
		e.autocompleteCancel = nil
	}
}

func (e *Editor) clearAutocompleteUI() {
	e.autocomplete = nil
	e.autocompleteList = nil
	e.autocompleteForce = false
}

func (e *Editor) notifyAutocompleteChanged() {
	if e.OnAutocompleteChange != nil {
		fn := e.OnAutocompleteChange
		e.pendingCallbacks = append(e.pendingCallbacks, fn)
	}
}

func bestAutocompleteMatchIndex(items []AutocompleteItem, prefix string) int {
	if prefix == "" {
		return -1
	}
	firstPrefix := -1
	for i, item := range items {
		if item.Value == prefix {
			return i
		}
		if firstPrefix < 0 && strings.HasPrefix(item.Value, prefix) {
			firstPrefix = i
		}
	}
	return firstPrefix
}

func symbolAutocompleteContext(before string) bool {
	lineStart := strings.LastIndexByte(before, '\n') + 1
	line := before[lineStart:]
	for start := 0; start < len(line); start++ {
		if start > 0 && line[start-1] != ' ' && line[start-1] != '\t' {
			continue
		}
		if symbolAutocompleteTokenMatchesPi(line[start:]) {
			return true
		}
	}
	return false
}

func symbolAutocompleteTokenMatchesPi(token string) bool {
	if strings.HasPrefix(token, "@\"") {
		return !strings.Contains(token[2:], "\"")
	}
	if strings.HasPrefix(token, "@") || strings.HasPrefix(token, "#") {
		return !strings.ContainsAny(token, " \t")
	}
	return false
}

func (e *Editor) linesAndCursor() ([]string, int, int) {
	lines := strings.Split(e.text, "\n")
	runes := []rune(e.text)
	cursor := max(0, min(e.cursor, len(runes)))
	before := string(runes[:cursor])
	cursorLine := strings.Count(before, "\n")
	lastNewline := strings.LastIndex(before, "\n")
	cursorCol := len([]rune(before))
	if lastNewline >= 0 {
		cursorCol = len([]rune(before[lastNewline+1:]))
	}
	return lines, cursorLine, cursorCol
}

func cursorFromLineCol(lines []string, cursorLine, cursorCol int) int {
	if len(lines) == 0 {
		return 0
	}
	cursorLine = max(0, min(cursorLine, len(lines)-1))
	cursor := 0
	for i := 0; i < cursorLine; i++ {
		cursor += len([]rune(lines[i])) + 1
	}
	lineRunes := []rune(lines[cursorLine])
	cursorCol = max(0, min(cursorCol, len(lineRunes)))
	cursor += cursorCol
	return cursor
}

func (e *Editor) isAtStartOfMessage() bool {
	before := string([]rune(e.text)[:max(0, min(e.cursor, len([]rune(e.text))))])
	lineStart := strings.LastIndex(before, "\n") + 1
	return strings.TrimSpace(before[lineStart:]) == "/"
}

func (e *Editor) inSlashCommandContext() bool {
	before := string([]rune(e.text)[:max(0, min(e.cursor, len([]rune(e.text))))])
	lineStart := strings.LastIndex(before, "\n") + 1
	line := before[lineStart:]
	return strings.HasPrefix(line, "/") && !strings.Contains(line, "\n")
}

func (e *Editor) symbolCompletionContext() bool {
	before := string([]rune(e.text)[:max(0, min(e.cursor, len([]rune(e.text))))])
	lineStart := strings.LastIndex(before, "\n") + 1
	line := before[lineStart:]
	if e.autocompleteTriggerPattern == nil {
		return false
	}
	return e.autocompleteTriggerPattern.MatchString(line)
}

func isAutocompleteContinuationRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' || r == '/'
}

func containsWhitespaceRune(text string) bool {
	return strings.IndexFunc(text, unicode.IsSpace) >= 0
}

func decodeCSIuControls(text string) string {
	return editorCSIuControlPattern.ReplaceAllStringFunc(text, func(seq string) string {
		matches := editorCSIuControlPattern.FindStringSubmatch(seq)
		if len(matches) != 2 {
			return seq
		}
		code, err := strconv.Atoi(matches[1])
		if err != nil {
			return seq
		}
		switch {
		case code >= 'a' && code <= 'z':
			return string(rune(code - 'a' + 1))
		case code >= 'A' && code <= 'Z':
			return string(rune(code - 'A' + 1))
		default:
			return seq
		}
	})
}

func filterEditorPaste(text string) string {
	var b strings.Builder
	for _, r := range text {
		if r == '\n' || r >= 32 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normalizeEditorText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\t", "    ")
	return text
}

func (e *Editor) pushUndoSnapshot() {
	snapshot := editorSnapshot{
		text:         e.text,
		cursor:       e.cursor,
		pastes:       clonePasteMap(e.pastes),
		pasteCounter: e.pasteCounter,
	}
	if len(e.undoStack) > 0 {
		last := e.undoStack[len(e.undoStack)-1]
		if last.text == snapshot.text && last.cursor == snapshot.cursor {
			return
		}
	}
	e.undoStack = append(e.undoStack, snapshot)
	if len(e.undoStack) > 200 {
		e.undoStack = e.undoStack[len(e.undoStack)-200:]
	}
}

func (e *Editor) undo() {
	if len(e.undoStack) == 0 {
		return
	}
	snapshot := e.undoStack[len(e.undoStack)-1]
	e.undoStack = e.undoStack[:len(e.undoStack)-1]
	e.text = snapshot.text
	e.cursor = min(snapshot.cursor, len([]rune(e.text)))
	e.pastes = clonePasteMap(snapshot.pastes)
	e.pasteCounter = snapshot.pasteCounter
	e.exitHistoryBrowsing()
	e.lastAction = ""
	e.resetPreferredColumn()
	e.breakKillAndYank()
	e.changed()
}

func clonePasteMap(in map[int]string) map[int]string {
	out := make(map[int]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (e *Editor) leftCursor() int {
	if start, end := e.markerSpanBeforeCursor(); start >= 0 && e.cursor <= end {
		return start
	}
	return previousGraphemeBoundary([]rune(e.text), e.cursor)
}

func (e *Editor) rightCursor() int {
	if start, end := e.markerSpanAfterCursor(); start >= 0 && e.cursor >= start {
		return end
	}
	return nextGraphemeBoundary([]rune(e.text), e.cursor)
}

func (e *Editor) wordBackward() int {
	if start, _ := e.markerSpanBeforeCursor(); start >= 0 {
		return start
	}
	runes := []rune(e.text)
	pos := min(e.cursor, len(runes))
	lineStart := pos
	for lineStart > 0 && runes[lineStart-1] != '\n' {
		lineStart--
	}
	if pos == lineStart {
		if pos > 0 {
			return pos - 1
		}
		return 0
	}
	line := string(runes[lineStart:pos])
	return lineStart + FindWordBackward(line, len([]rune(line)), e.wordNavigationOptions())
}

func (e *Editor) wordForward() int {
	if _, end := e.markerSpanAfterCursor(); end >= 0 {
		return end
	}
	runes := []rune(e.text)
	pos := min(e.cursor, len(runes))
	lineStart := pos
	for lineStart > 0 && runes[lineStart-1] != '\n' {
		lineStart--
	}
	lineEnd := pos
	for lineEnd < len(runes) && runes[lineEnd] != '\n' {
		lineEnd++
	}
	if pos == lineEnd {
		if lineEnd < len(runes) {
			return lineEnd + 1
		}
		return lineEnd
	}
	line := string(runes[lineStart:lineEnd])
	return lineStart + FindWordForward(line, pos-lineStart, e.wordNavigationOptions())
}

func (e *Editor) wordNavigationOptions() WordNavigationOptions {
	return WordNavigationOptions{
		Segment:         e.wordSegments,
		IsAtomicSegment: isPasteMarkerSegment,
	}
}

func (e *Editor) wordSegments(text string) []WordSegment {
	if len(e.pastes) == 0 || !strings.Contains(text, "[paste #") {
		return defaultWordSegments(text)
	}
	matches := pasteMarkerPattern.FindAllStringSubmatchIndex(text, -1)
	segments := make([]WordSegment, 0, len(matches)*2+1)
	cursor := 0
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		id, err := strconv.Atoi(text[match[2]:match[3]])
		if err != nil {
			continue
		}
		if _, ok := e.pastes[id]; !ok {
			continue
		}
		segments = append(segments, defaultWordSegments(text[cursor:match[0]])...)
		segments = append(segments, WordSegment{Text: text[match[0]:match[1]], WordLike: true})
		cursor = match[1]
	}
	segments = append(segments, defaultWordSegments(text[cursor:])...)
	return segments
}

func isPasteMarkerSegment(segment string) bool {
	match := pasteMarkerPattern.FindStringIndex(segment)
	return match != nil && match[0] == 0 && match[1] == len(segment)
}

func (e *Editor) markerSpanBeforeCursor() (int, int) {
	return e.markerSpanBeforePosition(e.cursor)
}

func (e *Editor) markerSpanAfterCursor() (int, int) {
	return e.markerSpanAtPosition(e.cursor)
}

func (e *Editor) markerSpanBeforePosition(pos int) (int, int) {
	for _, span := range e.validPasteMarkerSpans() {
		if pos > span.start && pos <= span.end {
			return span.start, span.end
		}
	}
	return -1, -1
}

func (e *Editor) markerSpanAtPosition(pos int) (int, int) {
	for _, span := range e.validPasteMarkerSpans() {
		if pos >= span.start && pos < span.end {
			return span.start, span.end
		}
	}
	return -1, -1
}

type editorMarkerSpan struct {
	start int
	end   int
}

func (e *Editor) validPasteMarkerSpans() []editorMarkerSpan {
	if len(e.pastes) == 0 || !strings.Contains(e.text, "[paste #") {
		return nil
	}
	matches := pasteMarkerPattern.FindAllStringSubmatchIndex(e.text, -1)
	spans := make([]editorMarkerSpan, 0, len(matches))
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		id, err := strconv.Atoi(e.text[match[2]:match[3]])
		if err != nil {
			continue
		}
		if _, ok := e.pastes[id]; !ok {
			continue
		}
		start := len([]rune(e.text[:match[0]]))
		end := len([]rune(e.text[:match[1]]))
		spans = append(spans, editorMarkerSpan{start: start, end: end})
	}
	return spans
}

func isHorizontalWhitespace(r rune) bool {
	return r == ' ' || r == '\t'
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isEditorPunctuationRune(r rune) bool {
	return strings.ContainsRune(`(){}[]<>.,;:'"!?+-=*/\|&%^$#@~_`, r)
}

func (e *Editor) changed() {
	if e.OnChange != nil {
		fn := e.OnChange
		text := e.text
		e.pendingCallbacks = append(e.pendingCallbacks, func() { fn(text) })
	}
}

func (e *Editor) currentLineStart() int {
	runes := []rune(e.text)
	pos := min(e.cursor, len(runes))
	for pos > 0 && runes[pos-1] != '\n' {
		pos--
	}
	return pos
}

func (e *Editor) currentLineEnd() int {
	runes := []rune(e.text)
	pos := min(e.cursor, len(runes))
	for pos < len(runes) && runes[pos] != '\n' {
		pos++
	}
	return pos
}

func (e *Editor) currentLineCol() int {
	return e.cursor - e.currentLineStart()
}

func (e *Editor) resetPreferredColumn() {
	e.resetPreferredColumnOnly()
	e.clearSnappedFromColumn()
}

func (e *Editor) resetPreferredColumnOnly() {
	e.hasPreferredColumn = false
	e.preferredColumn = 0
}

func (e *Editor) clearSnappedFromColumn() {
	e.hasSnappedFromColumn = false
	e.snappedFromLine = 0
	e.snappedFromColumn = 0
}

func (e *Editor) capturePreferredColumnFromCurrentVisualLine() {
	lines := strings.Split(e.text, "\n")
	visualLines := e.editorVisualLines(max(1, e.lastLayoutWidth), lines)
	if len(visualLines) == 0 {
		e.resetPreferredColumn()
		return
	}
	_, cursorLine, cursorCol := e.linesAndCursor()
	current := findEditorVisualLine(visualLines, cursorLine, cursorCol)
	currentVL := visualLines[current]
	e.preferredColumn = max(0, cursorCol-currentVL.startCol)
	e.hasPreferredColumn = true
}

func (e *Editor) isOnFirstLine() bool { return e.currentLineStart() == 0 }

func (e *Editor) isOnLastLine() bool { return e.currentLineEnd() == len([]rune(e.text)) }

func (e *Editor) isOnFirstVisualLine() bool {
	visualLines, current := e.currentEditorVisualLine()
	return len(visualLines) == 0 || current == 0
}

func (e *Editor) isOnLastVisualLine() bool {
	visualLines, current := e.currentEditorVisualLine()
	return len(visualLines) == 0 || current == len(visualLines)-1
}

func (e *Editor) currentEditorVisualLine() ([]editorVisualLine, int) {
	lines := strings.Split(e.text, "\n")
	visualLines := e.editorVisualLines(max(1, e.lastLayoutWidth), lines)
	if len(visualLines) == 0 {
		return visualLines, 0
	}
	_, cursorLine, cursorCol := e.linesAndCursor()
	return visualLines, findEditorVisualLine(visualLines, cursorLine, cursorCol)
}

func (e *Editor) pageScroll(direction int) {
	if direction == 0 || e.text == "" {
		return
	}
	lines := strings.Split(e.text, "\n")
	visualLines := e.editorVisualLines(max(1, e.lastLayoutWidth), lines)
	if len(visualLines) == 0 {
		return
	}
	_, cursorLine, cursorCol := e.linesAndCursor()
	current := findEditorVisualLine(visualLines, cursorLine, cursorCol)
	pageSize := e.editorPageSize()
	target := current + direction*pageSize
	if target < 0 {
		target = 0
	}
	if target >= len(visualLines) {
		target = len(visualLines) - 1
	}
	e.moveToEditorVisualLine(lines, visualLines, current, target)
}

func (e *Editor) editorPageSize() int {
	return 5
}

func (e *Editor) moveVertical(delta int) {
	if delta == 0 || e.text == "" {
		return
	}
	lines := strings.Split(e.text, "\n")
	visualLines := e.editorVisualLines(max(1, e.lastLayoutWidth), lines)
	if len(visualLines) == 0 {
		return
	}
	_, cursorLine, cursorCol := e.linesAndCursor()
	current := findEditorVisualLine(visualLines, cursorLine, cursorCol)
	target := current + delta
	if target < 0 || target >= len(visualLines) {
		return
	}
	e.moveToEditorVisualLine(lines, visualLines, current, target)
}

type editorVisualLine struct {
	logicalLine int
	startCol    int
	length      int
}

func (e *Editor) editorVisualLines(width int, lines []string) []editorVisualLine {
	width = max(1, width)
	visualLines := make([]editorVisualLine, 0, len(lines))
	for lineIndex, line := range lines {
		lineLen := len([]rune(line))
		if line == "" {
			visualLines = append(visualLines, editorVisualLine{logicalLine: lineIndex})
			continue
		}
		if VisibleWidth(line) <= width {
			visualLines = append(visualLines, editorVisualLine{logicalLine: lineIndex, length: lineLen})
			continue
		}
		chunks := wordWrapLineWithSegments(line, width, e.segmentLineForWrap(line))
		for _, chunk := range chunks {
			startCol := byteIndexToRuneCol(line, chunk.StartIndex)
			endCol := byteIndexToRuneCol(line, chunk.EndIndex)
			visualLines = append(visualLines, editorVisualLine{
				logicalLine: lineIndex,
				startCol:    startCol,
				length:      max(0, endCol-startCol),
			})
		}
	}
	if len(visualLines) == 0 {
		return []editorVisualLine{{}}
	}
	return visualLines
}

func findEditorVisualLine(visualLines []editorVisualLine, line, col int) int {
	lastForLine := -1
	for i, vl := range visualLines {
		if vl.logicalLine != line {
			continue
		}
		lastForLine = i
		offset := col - vl.startCol
		isLastSegment := i == len(visualLines)-1 || visualLines[i+1].logicalLine != line
		if offset >= 0 && (offset < vl.length || (isLastSegment && offset == vl.length)) {
			return i
		}
	}
	if lastForLine >= 0 {
		return lastForLine
	}
	return max(0, min(len(visualLines)-1, line))
}

func (e *Editor) moveToEditorVisualLine(lines []string, visualLines []editorVisualLine, current, target int) {
	if current < 0 || current >= len(visualLines) || target < 0 || target >= len(visualLines) {
		return
	}
	currentVL := visualLines[current]
	targetVL := visualLines[target]

	currentLineCol := e.currentLineCol()
	currentVisualCol := max(0, currentLineCol-currentVL.startCol)
	if e.hasSnappedFromColumn && e.snappedFromLine == currentVL.logicalLine {
		snappedLine := findEditorVisualLine(visualLines, currentVL.logicalLine, e.snappedFromColumn)
		if snappedLine >= 0 && snappedLine < len(visualLines) {
			currentVisualCol = max(0, e.snappedFromColumn-visualLines[snappedLine].startCol)
		}
	}
	sourceMaxCol := currentVL.length
	if current+1 < len(visualLines) && visualLines[current+1].logicalLine == currentVL.logicalLine {
		sourceMaxCol = max(0, currentVL.length-1)
	}
	targetMaxCol := targetVL.length
	if target+1 < len(visualLines) && visualLines[target+1].logicalLine == targetVL.logicalLine {
		targetMaxCol = max(0, targetVL.length-1)
	}
	moveCol := e.computeVerticalMoveColumn(currentVisualCol, sourceMaxCol, targetMaxCol)
	targetLine := lines[targetVL.logicalLine]
	targetCol := min(targetVL.startCol+moveCol, len([]rune(targetLine)))
	targetCursor := cursorFromLineCol(lines, targetVL.logicalLine, targetCol)

	if markerStart, markerEnd := e.markerSpanAtPosition(targetCursor); markerStart >= 0 {
		lineStart := cursorFromLineCol(lines, targetVL.logicalLine, 0)
		markerStartCol := markerStart - lineStart
		markerEndCol := markerEnd - lineStart
		isContinuation := markerStartCol < targetVL.startCol
		isMovingDown := target > current
		if isContinuation && isMovingDown {
			next := target + 1
			for next < len(visualLines) &&
				visualLines[next].logicalLine == targetVL.logicalLine &&
				visualLines[next].startCol < markerEndCol {
				next++
			}
			if next < len(visualLines) {
				e.moveToEditorVisualLine(lines, visualLines, current, next)
				return
			}
		}
		e.snappedFromLine = targetVL.logicalLine
		e.snappedFromColumn = targetCol
		e.hasSnappedFromColumn = true
		e.cursor = markerStart
		return
	}

	e.clearSnappedFromColumn()
	e.cursor = targetCursor
}

func (e *Editor) computeVerticalMoveColumn(currentVisualCol, sourceMaxVisualCol, targetMaxVisualCol int) int {
	currentVisualCol = max(0, currentVisualCol)
	sourceMaxVisualCol = max(0, sourceMaxVisualCol)
	targetMaxVisualCol = max(0, targetMaxVisualCol)

	cursorInMiddle := currentVisualCol < sourceMaxVisualCol
	targetTooShort := targetMaxVisualCol < currentVisualCol

	if !e.hasPreferredColumn || cursorInMiddle {
		if targetTooShort {
			e.preferredColumn = currentVisualCol
			e.hasPreferredColumn = true
			return targetMaxVisualCol
		}
		e.resetPreferredColumnOnly()
		return currentVisualCol
	}

	if targetTooShort || targetMaxVisualCol < e.preferredColumn {
		return targetMaxVisualCol
	}

	moveCol := e.preferredColumn
	e.resetPreferredColumnOnly()
	return moveCol
}

func (e *Editor) segmentLineForWrap(line string) []wrapSegment {
	if len(e.pastes) == 0 || !strings.Contains(line, "[paste #") {
		return segmentTextForWrap(line)
	}
	matches := pasteMarkerPattern.FindAllStringSubmatchIndex(line, -1)
	type byteSpan struct {
		start int
		end   int
	}
	markers := make([]byteSpan, 0, len(matches))
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		id, err := strconv.Atoi(line[match[2]:match[3]])
		if err != nil {
			continue
		}
		if _, ok := e.pastes[id]; !ok {
			continue
		}
		markers = append(markers, byteSpan{start: match[0], end: match[1]})
	}
	if len(markers) == 0 {
		return segmentTextForWrap(line)
	}
	segments := make([]wrapSegment, 0, len(line))
	markerIndex := 0
	for idx, r := range line {
		for markerIndex < len(markers) && idx >= markers[markerIndex].end {
			markerIndex++
		}
		if markerIndex < len(markers) && idx >= markers[markerIndex].start && idx < markers[markerIndex].end {
			if idx == markers[markerIndex].start {
				segments = append(segments, wrapSegment{text: line[markers[markerIndex].start:markers[markerIndex].end], index: markers[markerIndex].start})
			}
			continue
		}
		segments = append(segments, wrapSegment{text: string(r), index: idx})
	}
	return segments
}

func byteIndexToRuneCol(text string, byteIndex int) int {
	byteIndex = max(0, min(byteIndex, len(text)))
	return len([]rune(text[:byteIndex]))
}

func runeColToByteIndex(text string, col int) int {
	if col <= 0 {
		return 0
	}
	runeCol := 0
	for idx := range text {
		if runeCol == col {
			return idx
		}
		runeCol++
	}
	return len(text)
}

func (e *Editor) jumpToRune(target rune, direction int) {
	runes := []rune(e.text)
	if len(runes) == 0 || target == 0 {
		return
	}
	if direction >= 0 {
		for pos := min(e.cursor+1, len(runes)); pos < len(runes); pos++ {
			if runes[pos] == target {
				e.cursor = pos
				return
			}
		}
		return
	}
	for pos := min(e.cursor-1, len(runes)-1); pos >= 0; pos-- {
		if runes[pos] == target {
			e.cursor = pos
			return
		}
	}
}

type TextChunk struct {
	Text       string
	StartIndex int
	EndIndex   int
}

func WordWrapLine(line string, maxWidth int) []TextChunk {
	return wordWrapLineWithSegments(line, maxWidth, segmentTextForWrap(line))
}

func wordWrapLineWithSegments(line string, maxWidth int, segments []wrapSegment) []TextChunk {
	if line == "" || maxWidth <= 0 {
		return []TextChunk{{Text: "", StartIndex: 0, EndIndex: 0}}
	}
	if VisibleWidth(line) <= maxWidth {
		return []TextChunk{{Text: line, StartIndex: 0, EndIndex: len(line)}}
	}
	chunks := make([]TextChunk, 0, len(segments))
	currentWidth := 0
	chunkStart := 0
	wrapOppIndex := -1
	wrapOppWidth := 0

	for i, seg := range segments {
		segWidth := VisibleWidth(seg.text)
		isWhitespace := segmentIsWhitespace(seg.text)

		if currentWidth+segWidth > maxWidth {
			if wrapOppIndex >= 0 && currentWidth-wrapOppWidth+segWidth <= maxWidth {
				chunks = append(chunks, TextChunk{
					Text:       line[chunkStart:wrapOppIndex],
					StartIndex: chunkStart,
					EndIndex:   wrapOppIndex,
				})
				chunkStart = wrapOppIndex
				currentWidth -= wrapOppWidth
			} else if chunkStart < seg.index {
				chunks = append(chunks, TextChunk{
					Text:       line[chunkStart:seg.index],
					StartIndex: chunkStart,
					EndIndex:   seg.index,
				})
				chunkStart = seg.index
				currentWidth = 0
			}
			wrapOppIndex = -1
		}

		if segWidth > maxWidth {
			subChunks := splitOversizedWrapSegment(seg.text, maxWidth)
			for _, sub := range subChunks[:max(0, len(subChunks)-1)] {
				chunks = append(chunks, TextChunk{
					Text:       sub.Text,
					StartIndex: seg.index + sub.StartIndex,
					EndIndex:   seg.index + sub.EndIndex,
				})
			}
			last := subChunks[len(subChunks)-1]
			chunkStart = seg.index + last.StartIndex
			currentWidth = VisibleWidth(last.Text)
			wrapOppIndex = -1
			continue
		}

		currentWidth += segWidth
		if isWhitespace && i+1 < len(segments) && !segmentIsWhitespace(segments[i+1].text) {
			wrapOppIndex = segments[i+1].index
			wrapOppWidth = currentWidth
		}
	}
	chunks = append(chunks, TextChunk{Text: line[chunkStart:], StartIndex: chunkStart, EndIndex: len(line)})
	return chunks
}

type wrapSegment struct {
	text  string
	index int
}

func segmentTextForWrap(text string) []wrapSegment {
	segments := make([]wrapSegment, 0, len(text))
	for idx, r := range text {
		segments = append(segments, wrapSegment{text: string(r), index: idx})
	}
	return segments
}

func segmentIsWhitespace(segment string) bool {
	if segment == "" {
		return false
	}
	for _, r := range segment {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func splitOversizedWrapSegment(text string, maxWidth int) []TextChunk {
	if text == "" {
		return []TextChunk{{Text: "", StartIndex: 0, EndIndex: 0}}
	}
	if len([]rune(text)) > 1 {
		return wordWrapLineWithSegments(text, maxWidth, segmentTextForWrap(text))
	}
	var chunks []TextChunk
	start := 0
	width := 0
	for idx, r := range text {
		rw := runeWidth(r)
		if width > 0 && width+rw > maxWidth {
			chunks = append(chunks, TextChunk{Text: text[start:idx], StartIndex: start, EndIndex: idx})
			start = idx
			width = 0
		}
		width += rw
	}
	chunks = append(chunks, TextChunk{Text: text[start:], StartIndex: start, EndIndex: len(text)})
	return chunks
}
