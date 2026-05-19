package gitui_test

import (
	"context"
	"testing"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

type apiParityComponent struct {
	focused bool
}

func (c *apiParityComponent) Render(width int) []string {
	return []string{gitui.TruncateToWidth("component", width, "")}
}

func (c *apiParityComponent) Invalidate()        {}
func (c *apiParityComponent) HandleInput(string) {}
func (c *apiParityComponent) SetFocused(focused bool) {
	c.focused = focused
}
func (c *apiParityComponent) Focused() bool { return c.focused }

func TestPiTUIIndexPublicSurfaceCompiles(t *testing.T) {
	style := func(text string) string { return text }
	component := &apiParityComponent{}

	var _ gitui.Component = component
	var _ gitui.Focusable = component
	if !gitui.IsFocusable(component) {
		t.Fatalf("component should satisfy Focusable")
	}

	text := gitui.NewText("hello", 1, 0, style)
	text.SetText("updated")
	text.SetCustomBackground(style)
	var _ gitui.Component = text

	spacer := gitui.NewSpacer(1)
	spacer.SetLines(2)
	var _ gitui.Component = spacer

	box := gitui.NewBox(1, 1, style)
	box.AddChild(text)
	box.RemoveChild(text)
	box.AddChild(spacer)
	var _ gitui.Component = box

	truncated := gitui.NewTruncatedText("abcdef", 1, 0, gitui.TruncatedTextOptions{Ellipsis: "…", Style: style})
	truncated.SetText("ghijkl")
	var _ gitui.Component = truncated

	loader := gitui.NewLoader("loading", gitui.LoaderIndicatorOptions{
		Frames:                  []string{"-"},
		Interval:                time.Millisecond,
		IntervalMs:              1,
		SpinnerColor:            style,
		MessageColor:            style,
		RenderIndicatorVerbatim: true,
	})
	loader.SetText("done")
	loader.SetIndicator(gitui.LoaderIndicatorOptions{Frames: []string{"."}})
	var _ gitui.Component = loader

	cancellable := gitui.NewCancellableLoader("working")
	abortCalled := false
	cancellable.OnAbort = func() { abortCalled = true }
	cancellable.Cancel()
	if !cancellable.Cancelled() || !cancellable.Aborted() {
		t.Fatalf("cancellable loader should report cancellation")
	}
	_ = cancellable.Signal()
	if !abortCalled {
		t.Fatalf("cancellable loader should expose Pi-style OnAbort callback")
	}
	var _ gitui.Component = cancellable

	input := gitui.NewInput("placeholder")
	input.SetValue("abc")
	input.SetFocused(true)
	var _ gitui.Component = input
	var _ gitui.Focusable = input

	selectList := gitui.NewSelectList([]gitui.SelectItem{{Value: "one", Description: "first"}}, 5, gitui.SelectListTheme{
		SelectedPrefix: style,
		SelectedText:   style,
		Description:    style,
		ScrollInfo:     style,
		NoMatch:        style,
	}, gitui.SelectListLayoutOptions{
		MinPrimaryColumnWidth: 4,
		MaxPrimaryColumnWidth: 12,
		TruncatePrimary: func(ctx gitui.SelectListTruncatePrimaryContext) string {
			return gitui.TruncateToWidth(ctx.Text, ctx.MaxWidth, "")
		},
	})
	selectList.SetFilter("o")
	if _, ok := selectList.SelectedItem(); !ok {
		t.Fatalf("select list should expose selected item")
	}
	if _, ok := selectList.GetSelectedItem(); !ok {
		t.Fatalf("select list should expose Pi-style selected item getter")
	}
	var _ gitui.Component = selectList

	settings := gitui.NewSettingsList([]gitui.SettingItem{{
		ID:           "theme",
		Label:        "Theme",
		Description:  "Theme mode",
		CurrentValue: "system",
		Values:       []string{"system", "light", "dark"},
		Submenu:      func(string, func(string, bool)) gitui.Component { return component },
	}}, 5, gitui.SettingsListTheme{
		Label:        func(text string, selected bool) string { return text },
		CurrentValue: func(text string, selected bool) string { return text },
		Description:  style,
		Hint:         style,
		Selected:     style,
		Value:        style,
		Cursor:       ">",
	}, gitui.SettingsListOptions{
		EnableSearch: true,
		OnChange:     func(string, string) {},
		OnCancel:     func() {},
	})
	settings.UpdateValue("theme", "dark")
	var _ gitui.Component = settings

	markdown := gitui.NewMarkdownWithOptions("**hello**", gitui.MarkdownOptions{
		Theme: gitui.MarkdownTheme{
			Text:            style,
			Heading:         style,
			Link:            style,
			LinkURL:         style,
			Code:            style,
			CodeBlock:       style,
			CodeBlockBorder: style,
			Quote:           style,
			QuoteBorder:     style,
			HR:              style,
			ListBullet:      style,
			Bold:            style,
			Italic:          style,
			Strikethrough:   style,
			Underline:       style,
			HighlightCode: func(code, lang string) []string {
				return []string{lang + ":" + code}
			},
			CodeBlockIndent: "  ",
		},
		PaddingX: 1,
		DefaultTextStyle: &gitui.DefaultTextStyle{
			Color:     style,
			BgColor:   style,
			Bold:      true,
			Italic:    true,
			Underline: true,
		},
	})
	markdown.SetText("updated")
	var _ gitui.Component = markdown

	editor := gitui.NewEditor(gitui.EditorTheme{Border: style, SelectList: gitui.SelectListTheme{}}, gitui.EditorOptions{
		PaddingX:               1,
		AutocompleteMaxVisible: 5,
		AutocompleteDebounce:   time.Millisecond,
		MaxVisibleLines:        4,
	})
	editor.SetOnSubmit(func(string) {})
	editor.SetOnChange(func(string) {})
	editor.SetAutocompleteProvider(gitui.NewCombinedAutocompleteProvider())
	editor.SetText("hello")
	editor.InsertTextAtCursor(" world")
	editor.PasteToEditor("paste")
	var _ gitui.Component = editor
	var _ gitui.Focusable = editor
	var _ gitui.EditorComponent = editor

	killRing := gitui.NewKillRing()
	killRing.Push("cut", gitui.KillRingPushOptions{})
	killRing.Rotate()
	if _, ok := killRing.Peek(); !ok || killRing.Len() != 1 || killRing.Length() != 1 {
		t.Fatalf("kill ring should expose Pi-compatible helpers")
	}
	undoStack := gitui.NewUndoStack(func(in []string) []string { return append([]string(nil), in...) })
	undoStack.Push([]string{"state"})
	if _, ok := undoStack.Pop(); !ok || undoStack.Len() != 0 || undoStack.Length() != 0 {
		t.Fatalf("undo stack should expose Pi-compatible helpers")
	}

	image := gitui.NewImage([]byte("data"), gitui.ImageOptions{
		MimeType:       "image/png",
		Alt:            "alt",
		Filename:       "image.png",
		MaxWidth:       10,
		MaxHeight:      4,
		MaxWidthCells:  10,
		MaxHeightCells: 4,
		ImageId:        2,
		Dimensions:     &gitui.ImageDimensions{Width: 10, Height: 10},
	}, gitui.ImageTheme{Fallback: style, FallbackColor: style})
	_ = image.GetImageID()
	_ = image.GetImageId()
	var _ gitui.Component = image

	provider := gitui.NewCombinedAutocompleteProviderWithCommands(".", []gitui.SlashCommand{{
		Name:                   "run",
		Description:            "Run command",
		ArgumentHint:           "<arg>",
		GetArgumentCompletions: func(string) []gitui.AutocompleteItem { return []gitui.AutocompleteItem{{Value: "target"}} },
		GetArgumentCompletionsContext: func(context.Context, string) ([]gitui.AutocompleteItem, error) {
			return []gitui.AutocompleteItem{{Value: "ctx"}}, nil
		},
	}}, gitui.AutocompleteProviderFunc(func(text string, cursor int) gitui.AutocompleteSuggestions {
		return gitui.AutocompleteSuggestions{Start: cursor, End: cursor, Prefix: "", Items: []gitui.AutocompleteItem{{Value: text}}}
	}))
	provider.SetCommandItems([]gitui.AutocompleteItem{{Value: "help"}})
	provider.SetBasePath(".")
	if _, err := provider.GetSuggestionsContext(context.Background(), []string{"/help "}, 0, len("/help "), true); err != nil {
		t.Fatalf("autocomplete provider should be callable: %v", err)
	}
	var _ gitui.AutocompleteProvider = provider

	filtered := gitui.FuzzyFilter([]string{"alpha", "beta"}, "al", func(value string) string { return value })
	if len(filtered) != 1 || !gitui.FuzzyMatchText("al", "alpha").Matches {
		t.Fatalf("fuzzy helpers should be callable")
	}

	kb := gitui.NewKeybindingsManager(gitui.KeybindingsConfig{"tui.select.up": {"ctrl+p"}})
	gitui.SetKeybindings(kb)
	if !gitui.GetKeybindings().Matches("\x10", "tui.select.up") {
		t.Fatalf("keybindings should be configurable")
	}
	if keys := kb.GetKeys("tui.select.up"); len(keys) != 1 || keys[0] != "ctrl+p" {
		t.Fatalf("Pi-style GetKeys = %#v, want ctrl+p", keys)
	}
	if _, ok := kb.GetDefinition("tui.select.up"); !ok {
		t.Fatalf("Pi-style GetDefinition should find default action")
	}
	if bindings := kb.GetResolvedBindings(); len(bindings["tui.select.up"]) != 1 {
		t.Fatalf("Pi-style GetResolvedBindings should expose resolved action keys")
	}
	conflicting := gitui.NewKeybindingsManager(gitui.KeybindingsConfig{
		"tui.input.submit":   {"ctrl+x"},
		"tui.select.confirm": {"ctrl+x"},
	})
	if conflicts := conflicting.GetConflicts(); len(conflicts) != 1 || len(conflicts[0].Keybindings) != 2 {
		t.Fatalf("Pi-style GetConflicts = %#v, want one conflict with keybindings", conflicts)
	}
	if userBindings := conflicting.GetUserBindings(); len(userBindings["tui.input.submit"]) != 1 {
		t.Fatalf("Pi-style GetUserBindings should expose configured keys")
	}
	defaults := gitui.NewKeybindingsManagerWithDefinitions(gitui.TUI_KEYBINDINGS)
	if keys := defaults.GetKeys("tui.select.confirm"); len(keys) != 1 || keys[0] != "enter" {
		t.Fatalf("Pi-style TUI_KEYBINDINGS default confirm = %#v", keys)
	}
	gitui.SetKeybindings(gitui.NewKeybindingsManager())

	gitui.SetKittyProtocolActive(true)
	if !gitui.IsKittyProtocolActive() || !gitui.IsKeyRelease("\x1b[65;1:3u") {
		t.Fatalf("key helpers should expose kitty state and event parsing")
	}
	if gitui.Key.Ctrl("c") != "ctrl+c" || gitui.Key.CtrlAlt(gitui.Key.RightBracket) != "ctrl+alt+]" {
		t.Fatalf("Pi-style Key helper should build key IDs")
	}
	if event := gitui.ParseKey("\x1b[A"); event.Key != gitui.KeyUp {
		t.Fatalf("parse key = %#v, want up", event)
	}
	if r, ok := gitui.DecodePrintableKey("\x1b[65;2u"); !ok || r != 'A' {
		t.Fatalf("decode printable = %q %v, want A true", r, ok)
	}

	stdin := gitui.NewStdinBuffer(gitui.StdinBufferOptions{Timeout: time.Millisecond})
	stdin.OnData(func(string) {})
	stdin.OnPaste(func(string) {})
	stdin.Process("x")
	stdin.ProcessBytes([]byte{'y'})
	stdin.Clear()
	stdin.Destroy()

	terminal := gitui.NewVirtualTerminal(40, 8)
	var _ gitui.Terminal = terminal
	terminal.Flush()
	_ = terminal.FlushAndGetViewport()
	terminal.WaitForRender()
	terminal.SendInput("prestart")
	terminal.Resize(40, 8)
	_ = terminal.GetViewport()
	_ = terminal.GetScrollBuffer()
	_, _ = terminal.GetCursorPosition()
	ui := gitui.NewTUI(terminal, true)
	if !ui.GetShowHardwareCursor() {
		t.Fatalf("Pi-style hardware cursor getter should reflect constructor option")
	}
	ui.SetClearOnShrink(true)
	if !ui.GetClearOnShrink() {
		t.Fatalf("Pi-style clear-on-shrink getter should reflect setter")
	}
	_ = ui.GetFullRedraws()
	ui.AddChild(component)
	ui.InsertChild(0, gitui.NewText("welcome", 0, 0))
	children := ui.Children()
	if len(children) < 2 {
		t.Fatalf("children should expose container ordering")
	}
	ui.SetChildren(children)
	if removed := ui.RemoveChildAt(ui.ChildCount() - 1); removed == nil {
		t.Fatalf("RemoveChildAt should return removed child")
	}
	ui.AddChild(component)
	ui.SetFocus(component)
	removeListener := ui.AddInputListener(func(data string) gitui.InputListenerResult {
		return gitui.InputListenerData(data)
	})
	removeListener()
	handle := ui.ShowOverlay(gitui.NewText("overlay", 0, 0), gitui.OverlayOptions{
		Width:        ptrSize(gitui.PercentFloat(12.5)),
		MaxHeight:    ptrSize(gitui.Percent(50)),
		Anchor:       gitui.OverlayCenter,
		Margin:       gitui.OverlayMargin{Top: 1, Right: 1, Bottom: 1, Left: 1},
		Visible:      func(int, int) bool { return true },
		NonCapturing: true,
	})
	handle.SetHidden(true)
	handle.Unfocus()
	handle.Hide()
	ui.Start()
	ui.RequestRender()
	ui.RequestRender(true)
	ui.Stop()

	gitui.SetCapabilities(gitui.TerminalCapabilities{Images: true, Protocol: gitui.ImageProtocolKitty, TrueColor: true, Hyperlinks: true})
	defer gitui.ResetCapabilitiesCache()
	gitui.SetCellDimensions(gitui.CellDimensions{Width: 9, Height: 18})
	gitui.SetCellDimensions(gitui.CellDimensions{WidthPx: 9, HeightPx: 18})
	_ = gitui.GetCellDimensions()
	_ = gitui.GetCapabilities()
	_ = gitui.DetectCapabilities()
	_ = gitui.AllocateImageID()
	_ = gitui.CalculateImageRows(gitui.ImageDimensions{Width: 100, Height: 50}, 10)
	_ = gitui.CalculateImageRows(gitui.ImageDimensions{WidthPx: 100, HeightPx: 50}, 10)
	_ = gitui.CalculateImageRows(gitui.ImageDimensions{WidthPx: 100, HeightPx: 50}, 10, gitui.CellDimensions{WidthPx: 9, HeightPx: 18})
	_ = gitui.CalculateImageCellSize(gitui.ImageDimensions{Width: 100, Height: 50}, 10)
	_ = gitui.CalculateImageCellSize(gitui.ImageDimensions{Width: 100, Height: 50}, 10, 3, gitui.CellDimensions{WidthPx: 9, HeightPx: 18})
	moveCursor := false
	_ = gitui.EncodeKitty([]byte("data"), gitui.ImageRenderOptions{ID: 1, Width: 2, Height: 1})
	_ = gitui.EncodeKitty([]byte("data"), gitui.ImageRenderOptions{ImageID: 1, ImageId: 2, MaxWidthCells: 2, MaxHeightCells: 1, MoveCursor: &moveCursor})
	_ = gitui.EncodeITerm2([]byte("data"), gitui.ImageRenderOptions{Alt: "alt"})
	_ = gitui.RenderImage([]byte("data"), gitui.ImageRenderOptions{Protocol: gitui.ImageProtocolNone})
	_ = gitui.RenderImageWithDimensions([]byte("data"), gitui.ImageDimensions{WidthPx: 100, HeightPx: 50}, gitui.ImageRenderOptions{MaxWidthCells: 10})
	_ = gitui.DeleteKittyImage(1)
	_ = gitui.DeleteAllKittyImages()
	_ = gitui.Hyperlink("text", "https://example.com")
	_ = gitui.ImageFallback("alt", 10)
	_ = gitui.ImageFallback("image/png", gitui.ImageDimensions{WidthPx: 1, HeightPx: 1}, "a.png")
	_ = gitui.ImageFallbackDescription("image/png", &gitui.ImageDimensions{Width: 1, Height: 1}, "a.png")

	_ = gitui.VisibleWidth("wide")
	_ = gitui.WrapTextWithANSI("hello world", 5)
	_ = gitui.TruncateToWidth("hello", 3, "…")
	_ = gitui.NormalizeTerminalOutput("ำ")
	_, _ = gitui.ExtractAnsiCode("\x1b[31mred", 0)
	_ = gitui.IsWhitespaceChar(' ')
	_ = gitui.IsPunctuationChar('!')
	_ = gitui.ApplyBackgroundToLine("x", 3, func(text string) string { return text })
	_ = gitui.SliceByColumn("\x1b[31mhello", 0, 3, true)
	_ = gitui.SliceWithWidth("hello", 1, 3)
	_ = gitui.ExtractSegments("\x1b[31mabcdef", 2, 4, 2, true)
}

func ptrSize(value gitui.SizeValue) *gitui.SizeValue {
	return &value
}
