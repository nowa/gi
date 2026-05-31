package gicodingagent

import (
	"errors"

	gitui "github.com/nowa/gi/gi-tui"
)

func (h *CLIInteractiveTUIHost) ReadEditorText() string {
	return h.activeEditorText()
}

func (h *CLIInteractiveTUIHost) SetEditorText(text string) {
	h.setActiveEditorText(text)
}

func (h *CLIInteractiveTUIHost) InsertEditorText(text string) {
	if h == nil {
		return
	}
	editor, ok := h.activeEditorComponent()
	if !ok {
		return
	}
	if inserter, ok := editor.(gitui.EditorTextInserter); ok {
		inserter.InsertTextAtCursor(text)
	} else {
		editor.SetText(editor.GetText() + text)
	}
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) PasteEditorText(text string) {
	if h == nil {
		return
	}
	editor, ok := h.activeEditorComponent()
	if !ok {
		return
	}
	if pasteEditor, ok := editor.(interface{ PasteToEditor(string) }); ok {
		pasteEditor.PasteToEditor(text)
	} else if inserter, ok := editor.(gitui.EditorTextInserter); ok {
		inserter.InsertTextAtCursor(text)
	} else {
		editor.SetText(editor.GetText() + text)
	}
	h.requestRender(false)
}

func (h *CLIInteractiveTUIHost) EditorCursor() (line, col int, ok bool) {
	if h == nil {
		return 0, 0, false
	}
	editor, ok := h.activeEditorComponent()
	if !ok {
		return 0, 0, false
	}
	cursor, ok := editor.(interface{ GetCursor() (int, int) })
	if !ok {
		return 0, 0, false
	}
	line, col = cursor.GetCursor()
	return line, col, true
}

func (h *CLIInteractiveTUIHost) FocusEditor() error {
	if h == nil || h.ui == nil {
		return errors.New("interactive TUI editor is not ready")
	}
	component := h.activeEditorFocusComponent()
	if component == nil {
		return errors.New("interactive TUI editor is not ready")
	}
	h.ui.SetFocus(component)
	h.requestRender(false)
	return nil
}

func (h *CLIInteractiveTUIHost) EditorFocused() bool {
	if h == nil || h.ui == nil {
		return false
	}
	focused := h.ui.FocusedComponent()
	if focused == nil {
		return false
	}
	if h.customEditorActive {
		return h.editorContainerHasChild(focused)
	}
	return focused == h.editor
}

func (h *CLIInteractiveTUIHost) EditorCustomActive() bool {
	return h != nil && h.customEditorActive
}

func (h *CLIInteractiveTUIHost) activeEditorFocusComponent() gitui.Component {
	if h == nil {
		return nil
	}
	if h.customEditorActive && h.editorContainer != nil {
		children := h.editorContainer.Children()
		if len(children) > 0 {
			return children[0]
		}
		return nil
	}
	return h.editor
}

func (h *CLIInteractiveTUIHost) editorContainerHasChild(component gitui.Component) bool {
	if h == nil || h.editorContainer == nil || component == nil {
		return false
	}
	for _, child := range h.editorContainer.Children() {
		if child == component {
			return true
		}
	}
	return false
}

func (h *CLIInteractiveTUIHost) showEditorReplacement(component gitui.Component, focus gitui.Component) func() {
	if h == nil || h.editorContainer == nil || h.ui == nil || component == nil {
		return func() {}
	}
	previousChildren := h.editorContainer.Children()
	previousFocus := h.ui.FocusedComponent()
	h.editorContainer.SetChildren([]gitui.Component{component})
	if focus != nil {
		h.ui.SetFocus(focus)
	} else {
		h.ui.SetFocus(component)
	}
	h.requestRender(false)
	return func() {
		if len(previousChildren) == 0 && h.editor != nil {
			previousChildren = []gitui.Component{h.editor}
		}
		h.editorContainer.SetChildren(previousChildren)
		if previousFocus != nil {
			h.ui.SetFocus(previousFocus)
		} else if h.editor != nil {
			h.ui.SetFocus(h.editor)
		}
		h.requestRender(false)
	}
}

func (h *CLIInteractiveTUIHost) SubmitEditorText() error {
	if h == nil {
		return errors.New("interactive TUI editor is not ready")
	}
	editor, ok := h.activeEditorComponent()
	if !ok {
		return errors.New("interactive TUI editor is not ready")
	}
	editor.HandleInput("\r")
	h.requestRender(false)
	return nil
}
