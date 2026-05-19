package gitui

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goldmarkast "github.com/yuin/goldmark/ast"
	extensionast "github.com/yuin/goldmark/extension/ast"
)

func TestTextRendersPaddingAndWrap(t *testing.T) {
	text := NewText("hello world", 1, 1)
	lines := text.Render(8)
	if len(lines) != 4 {
		t.Fatalf("line count = %d, want 4: %#v", len(lines), lines)
	}
	for _, line := range lines {
		if VisibleWidth(line) != 8 {
			t.Fatalf("line %q width = %d, want 8", line, VisibleWidth(line))
		}
	}
}

func TestMutableRenderComponentsConcurrentMutationAndRender(t *testing.T) {
	theme := SelectListTheme{
		SelectedText: func(s string) string { return "<" + s + ">" },
		Description:  func(s string) string { return "[" + s + "]" },
		ScrollInfo:   func(s string) string { return "(" + s + ")" },
		NoMatch:      func(s string) string { return "!" + s },
	}
	text := NewText("hello", 1, 0)
	child := NewText("child", 0, 0)
	box := NewBox(1, 0)
	box.AddChild(child)
	spacer := NewSpacer(1)
	truncated := NewTruncatedText("truncate me", 0, 0)
	input := NewInput("placeholder")
	editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
	markdown := NewMarkdown("**hello** [link](https://example.com)")
	selectList := NewSelectList([]SelectItem{
		{Value: "alpha", Label: "Alpha", Description: "first"},
		{Value: "beta", Label: "Beta", Description: "second"},
		{Value: "gamma", Label: "Gamma", Description: "third"},
	}, 2, theme)
	settings := NewSettingsList([]SettingItem{
		{ID: "mode", Label: "Mode", CurrentValue: "auto", Values: []string{"auto", "manual"}},
		{ID: "theme", Label: "Theme", CurrentValue: "light", Values: []string{"light", "dark"}},
	}, 2, SettingsListTheme{
		Label: func(s string, _ bool) string { return s },
		CurrentValue: func(s string, _ bool) string {
			return s
		},
		Description: func(s string) string { return s },
		Hint:        func(s string) string { return s },
		Cursor:      "→ ",
	}, SettingsListOptions{EnableSearch: true})
	image := NewImage(nil, ImageOptions{Alt: "alt"})
	components := []Component{text, child, box, spacer, truncated, input, editor, markdown, selectList, settings, image}

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 500; i++ {
			for _, component := range components {
				component.Invalidate()
				_ = component.Render(40)
			}
			_ = image.ImageID()
			_, _ = selectList.GetSelectedItem()
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		transient := NewText("transient", 0, 0)
		for i := 0; i < 500; i++ {
			text.SetText(fmt.Sprintf("hello %d", i))
			child.SetText(fmt.Sprintf("child %d", i))
			spacer.SetLines(i % 3)
			truncated.SetText(fmt.Sprintf("truncate %d", i))
			input.SetValue(fmt.Sprintf("input %d", i))
			input.HandleInput("x")
			_ = input.GetValue()
			editor.SetText(fmt.Sprintf("editor %d", i))
			editor.HandleInput("x")
			_ = editor.GetText()
			markdown.SetText(fmt.Sprintf("**item %d**\n\n- alpha\n- beta", i))
			selectList.SetFilter([]string{"", "a", "b"}[i%3])
			selectList.SetSelectedIndex(i)
			selectList.HandleInput("\x1b[B")
			settings.UpdateValue("mode", []string{"auto", "manual"}[i%2])
			settings.HandleInput("\x1b[B")
			settings.HandleInput("m")
			box.SetBackground(func(s string) string { return s })
			box.AddChild(transient)
			box.RemoveChild(transient)
		}
	}()
	close(start)
	wg.Wait()
}

func TestInputCallbacksRunOutsideMutationLock(t *testing.T) {
	input := NewInput()
	changed := make(chan string, 1)
	input.OnChange = func(string) {
		changed <- input.GetValue()
	}
	input.HandleInput("a")
	select {
	case got := <-changed:
		if got != "a" {
			t.Fatalf("OnChange saw value %q, want a", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("OnChange callback deadlocked while reading input value")
	}

	submitted := make(chan string, 1)
	input.OnSubmit = func(string) {
		submitted <- input.GetValue()
	}
	input.HandleInput("\r")
	select {
	case got := <-submitted:
		if got != "a" {
			t.Fatalf("OnSubmit saw value %q, want a", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("OnSubmit callback deadlocked while reading input value")
	}
}

func TestTextAndBoxPiBackgroundSetters(t *testing.T) {
	bg := func(s string) string { return "<" + s + ">" }

	text := NewText("hello", 1, 0)
	text.SetCustomBgFn(bg)
	if got := text.Render(10)[0]; !strings.HasPrefix(got, "<") {
		t.Fatalf("SetCustomBgFn should apply background, got %q", got)
	}

	box := NewBox(1, 0)
	box.AddChild(NewText("hi", 0, 0))
	box.SetBgFn(bg)
	if got := box.Render(10)[0]; !strings.HasPrefix(got, "<") {
		t.Fatalf("SetBgFn should apply background, got %q", got)
	}
}

func TestSpacerPiEmptyLinesAndSetLines(t *testing.T) {
	spacer := NewSpacer(2)
	if lines := spacer.Render(10); len(lines) != 2 || lines[0] != "" || lines[1] != "" {
		t.Fatalf("spacer lines = %#v, want two empty strings", lines)
	}
	spacer.SetLines(0)
	if lines := spacer.Render(10); len(lines) != 0 {
		t.Fatalf("zero spacer lines = %#v, want empty slice", lines)
	}
	spacer.SetLines(3)
	if lines := spacer.Render(0); len(lines) != 3 || strings.Join(lines, "|") != "||" {
		t.Fatalf("set spacer lines = %#v, want three empty strings", lines)
	}
}

func TestTruncatedTextPadsAndTruncates(t *testing.T) {
	text := NewTruncatedText("Hello world", 1, 0)
	lines := text.Render(50)
	if len(lines) != 1 || VisibleWidth(lines[0]) != 50 {
		t.Fatalf("lines = %#v width=%d", lines, VisibleWidth(lines[0]))
	}

	longText := NewTruncatedText("This is a very long piece of text that will definitely exceed the available width", 1, 0)
	lines = longText.Render(30)
	if len(lines) != 1 || VisibleWidth(lines[0]) != 30 || !strings.Contains(lines[0], "...") {
		t.Fatalf("truncated lines = %#v", lines)
	}

	styledText := NewTruncatedText("\x1b[31mThis is a very long red text that will be truncated\x1b[0m", 1, 0)
	lines = styledText.Render(20)
	if len(lines) != 1 || VisibleWidth(lines[0]) != 20 {
		t.Fatalf("styled truncated lines = %#v width=%d", lines, VisibleWidth(lines[0]))
	}
	if !strings.Contains(lines[0], "\x1b[0m...\x1b[0m") {
		t.Fatalf("styled truncation should reset before ellipsis: %#v", lines)
	}
}

func TestTruncatedTextVerticalPaddingANSIAndNewlines(t *testing.T) {
	text := NewTruncatedText("\x1b[31mFirst line\x1b[0m\nSecond line", 0, 2)
	lines := text.Render(40)
	if len(lines) != 5 {
		t.Fatalf("line count = %d, want 5: %#v", len(lines), lines)
	}
	for _, line := range lines {
		if VisibleWidth(line) != 40 {
			t.Fatalf("line %q width=%d, want 40", line, VisibleWidth(line))
		}
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "\x1b[31m") || strings.Contains(joined, "Second line") {
		t.Fatalf("ANSI/newline handling failed: %#v", lines)
	}
}

func TestTruncatedTextPiComponentMatrix(t *testing.T) {
	for _, tc := range []struct {
		name          string
		text          string
		paddingX      int
		width         int
		wantEllipsis  bool
		wantContains  string
		rejectContent []string
	}{
		{
			name:         "fits without ellipsis",
			text:         "Hello world",
			paddingX:     1,
			width:        30,
			wantContains: "Hello world",
		},
		{
			name:         "empty text still pads line",
			text:         "",
			paddingX:     1,
			width:        30,
			wantContains: "",
		},
		{
			name:          "stops at first newline",
			text:          "First line\nSecond line\nThird line",
			paddingX:      1,
			width:         40,
			wantContains:  "First line",
			rejectContent: []string{"Second line", "Third line"},
		},
		{
			name:          "truncates first line before newline",
			text:          "This is a very long first line that needs truncation\nSecond line",
			paddingX:      1,
			width:         25,
			wantEllipsis:  true,
			wantContains:  "This is",
			rejectContent: []string{"Second line"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := NewTruncatedText(tc.text, tc.paddingX, 0).Render(tc.width)
			if len(lines) != 1 {
				t.Fatalf("line count = %d, want 1: %#v", len(lines), lines)
			}
			if got := VisibleWidth(lines[0]); got != tc.width {
				t.Fatalf("visible width = %d, want %d: %q", got, tc.width, lines[0])
			}
			plain := stripANSI(lines[0])
			if tc.wantContains != "" && !strings.Contains(plain, tc.wantContains) {
				t.Fatalf("rendered line missing %q: %q", tc.wantContains, plain)
			}
			if strings.Contains(plain, "...") != tc.wantEllipsis {
				t.Fatalf("ellipsis presence = %v, want %v: %q", strings.Contains(plain, "..."), tc.wantEllipsis, plain)
			}
			for _, rejected := range tc.rejectContent {
				if strings.Contains(plain, rejected) {
					t.Fatalf("rendered line should not include %q: %q", rejected, plain)
				}
			}
		})
	}
}

func TestCancellableLoaderCancelsContextOnce(t *testing.T) {
	loader := NewCancellableLoader("Working")
	var cancelled atomic.Int32
	var aborted atomic.Int32
	loader.OnCancel = func() { cancelled.Add(1) }
	loader.OnAbort = func() { aborted.Add(1) }

	loader.HandleInput("\x1b")
	if !loader.Cancelled() || !loader.Aborted() {
		t.Fatalf("loader should be marked cancelled")
	}
	select {
	case <-loader.Context().Done():
	case <-time.After(time.Second):
		t.Fatalf("loader context was not cancelled")
	}
	if loader.Signal().Err() == nil {
		t.Fatalf("Pi-style Signal should expose the cancelled context")
	}
	loader.HandleInput("\x1b")
	loader.HandleInput("\x03")
	if cancelled.Load() != 1 || aborted.Load() != 1 {
		t.Fatalf("cancel callbacks = onCancel:%d onAbort:%d, want both once", cancelled.Load(), aborted.Load())
	}
	loader.Dispose()
}

func TestCancellableLoaderIgnoresCtrlCLikePi(t *testing.T) {
	previous := GetKeybindings()
	SetKeybindings(NewKeybindingsManager(KeybindingsConfig{
		"tui.select.cancel": []string{"escape"},
	}))
	defer SetKeybindings(previous)

	loader := NewCancellableLoader("Working")
	loader.HandleInput("\x03")
	if loader.Cancelled() || loader.Aborted() || loader.Signal().Err() != nil {
		t.Fatalf("Ctrl+C should not abort CancellableLoader when it is not bound to select cancel")
	}

	loader.HandleInput("\x1b")
	if !loader.Cancelled() || loader.Signal().Err() == nil {
		t.Fatalf("Escape should still abort CancellableLoader")
	}
}

func TestCancellableLoaderConcurrentCancelIsIdempotent(t *testing.T) {
	loader := NewCancellableLoader("Working")
	var cancelled atomic.Int32
	var aborted atomic.Int32
	loader.OnCancel = func() { cancelled.Add(1) }
	loader.OnAbort = func() { aborted.Add(1) }

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			loader.Cancel()
			_ = loader.Cancelled()
			_ = loader.Aborted()
			_ = loader.Signal().Err()
		}()
	}
	wg.Wait()

	if !loader.Cancelled() || loader.Signal().Err() == nil {
		t.Fatalf("loader should remain cancelled after concurrent cancel")
	}
	if cancelled.Load() != 1 || aborted.Load() != 1 {
		t.Fatalf("concurrent cancel callbacks = onCancel:%d onAbort:%d, want both once", cancelled.Load(), aborted.Load())
	}
}

func TestLoaderPiRenderingAndIndicatorOptions(t *testing.T) {
	loader := NewLoader("Loading", LoaderIndicatorOptions{
		SpinnerColor: func(s string) string { return "<" + s + ">" },
		MessageColor: func(s string) string { return "[" + s + "]" },
	})
	lines := loader.Render(20)
	if len(lines) != 2 || lines[0] != "" {
		t.Fatalf("loader should render leading blank line, got %#v", lines)
	}
	if !strings.Contains(lines[1], "<⠋> [Loading]") {
		t.Fatalf("loader styles/default frame missing: %#v", lines)
	}
	if again := loader.Render(20); !strings.Contains(again[1], "<⠋> [Loading]") {
		t.Fatalf("render should not advance frame without Start tick: %#v", again)
	}

	loader.SetIndicator(LoaderIndicatorOptions{Frames: []string{"."}, IntervalMs: 25})
	if loader.interval != 25*time.Millisecond {
		t.Fatalf("Pi-style IntervalMs = %s, want 25ms", loader.interval)
	}

	loader.SetIndicator(LoaderIndicatorOptions{Frames: []string{}})
	lines = loader.Render(20)
	if strings.Contains(lines[1], "⠋") || !strings.Contains(lines[1], "[Loading]") {
		t.Fatalf("empty explicit frames should hide indicator: %#v", lines)
	}

	loader.SetMessage("Done")
	lines = loader.Render(20)
	if !strings.Contains(lines[1], "[Done]") {
		t.Fatalf("SetMessage should update rendered message: %#v", lines)
	}
}

func TestLoaderWithTUIStartsAnimationLikePi(t *testing.T) {
	terminal := NewVirtualTerminal(20, 4)
	ui := NewTUI(terminal)
	loader := NewLoader("Loading", LoaderIndicatorOptions{
		TUI:                     ui,
		Frames:                  []string{"a", "b"},
		IntervalMs:              5,
		RenderIndicatorVerbatim: true,
	})
	defer loader.Stop()

	waitFor(t, func() bool {
		loader.mu.Lock()
		defer loader.mu.Unlock()
		return loader.current > 0
	})

	lines := loader.Render(20)
	if !strings.Contains(lines[1], "b Loading") {
		t.Fatalf("auto-started loader should advance frame, got %#v", lines)
	}
}

func TestSelectListNavigationAndRender(t *testing.T) {
	selected := ""
	list := NewSelectList(
		[]SelectItem{
			{Value: "one", Label: "one", Description: "first item"},
			{Value: "two", Label: "two", Description: "second item"},
		},
		5,
		SelectListTheme{
			SelectedText: func(s string) string { return "[" + s + "]" },
			Description:  func(s string) string { return "<" + s + ">" },
		},
	)
	list.OnSelect = func(item SelectItem) { selected = item.Value }
	list.HandleInput("\x1b[B")
	list.HandleInput("\r")
	if selected != "two" {
		t.Fatalf("selected = %q, want two", selected)
	}
	lines := list.Render(60)
	if !strings.Contains(strings.Join(lines, "\n"), "second item") {
		t.Fatalf("render did not include description: %#v", lines)
	}
}

func TestSelectListPageNavigation(t *testing.T) {
	items := make([]SelectItem, 12)
	for idx := range items {
		value := fmt.Sprintf("item-%02d", idx)
		items[idx] = SelectItem{Value: value, Label: value}
	}
	list := NewSelectList(items, 5, SelectListTheme{})
	var changed []string
	list.OnSelectionChange = func(item SelectItem) {
		changed = append(changed, item.Value)
	}

	list.HandleInput("\x1b[6~")
	item, ok := list.SelectedItem()
	if !ok || item.Value != "item-05" {
		t.Fatalf("page down selected = %#v ok=%v, want item-05", item, ok)
	}
	list.HandleInput("\x1b[6~")
	item, _ = list.SelectedItem()
	if item.Value != "item-10" {
		t.Fatalf("second page down selected = %q, want item-10", item.Value)
	}
	list.HandleInput("\x1b[5~")
	item, _ = list.SelectedItem()
	if item.Value != "item-05" {
		t.Fatalf("page up selected = %q, want item-05", item.Value)
	}
	if len(changed) != 3 {
		t.Fatalf("selection change count = %d, want 3", len(changed))
	}
}

func TestSelectListNormalizesMultilineDescriptions(t *testing.T) {
	list := NewSelectList(
		[]SelectItem{{Value: "test", Label: "test", Description: "Line one\nLine two\r\nLine three"}},
		5,
		selectListIdentityTheme(),
	)

	rendered := list.Render(100)
	if len(rendered) == 0 {
		t.Fatal("expected rendered select list line")
	}
	if strings.Contains(rendered[0], "\n") || strings.Contains(rendered[0], "\r") {
		t.Fatalf("description should render as a single line: %#v", rendered)
	}
	if !strings.Contains(rendered[0], "Line one Line two Line three") {
		t.Fatalf("description was not normalized like pi: %q", rendered[0])
	}
}

func TestSelectListDescriptionAlignmentWithTruncatedPrimary(t *testing.T) {
	list := NewSelectList(
		[]SelectItem{
			{Value: "short", Label: "short", Description: "short description"},
			{Value: "very-long-command-name-that-needs-truncation", Label: "very-long-command-name-that-needs-truncation", Description: "long description"},
		},
		5,
		selectListIdentityTheme(),
	)

	rendered := list.Render(80)
	if got, want := visibleIndexOf(t, rendered[0], "short description"), visibleIndexOf(t, rendered[1], "long description"); got != want {
		t.Fatalf("description columns differ: got %d want %d\n%q\n%q", got, want, rendered[0], rendered[1])
	}
}

func TestSelectListPrimaryColumnMinWidth(t *testing.T) {
	list := NewSelectList(
		[]SelectItem{
			{Value: "a", Label: "a", Description: "first"},
			{Value: "bb", Label: "bb", Description: "second"},
		},
		5,
		selectListIdentityTheme(),
		SelectListLayoutOptions{MinPrimaryColumnWidth: 12, MaxPrimaryColumnWidth: 20},
	)

	rendered := list.Render(80)
	if got := visibleIndexOf(t, rendered[0], "first"); got != 14 {
		t.Fatalf("first description column = %d, want 14: %q", got, rendered[0])
	}
	if got := visibleIndexOf(t, rendered[1], "second"); got != 14 {
		t.Fatalf("second description column = %d, want 14: %q", got, rendered[1])
	}
}

func TestSelectListPrimaryColumnMaxWidth(t *testing.T) {
	list := NewSelectList(
		[]SelectItem{
			{Value: "very-long-command-name-that-needs-truncation", Label: "very-long-command-name-that-needs-truncation", Description: "first"},
			{Value: "short", Label: "short", Description: "second"},
		},
		5,
		selectListIdentityTheme(),
		SelectListLayoutOptions{MinPrimaryColumnWidth: 12, MaxPrimaryColumnWidth: 20},
	)

	rendered := list.Render(80)
	if got := visibleIndexOf(t, rendered[0], "first"); got != 22 {
		t.Fatalf("first description column = %d, want 22: %q", got, rendered[0])
	}
	if got := visibleIndexOf(t, rendered[1], "second"); got != 22 {
		t.Fatalf("second description column = %d, want 22: %q", got, rendered[1])
	}
}

func TestSelectListTruncatePrimaryOverridePreservesDescriptionAlignment(t *testing.T) {
	list := NewSelectList(
		[]SelectItem{
			{Value: "very-long-command-name-that-needs-truncation", Label: "very-long-command-name-that-needs-truncation", Description: "first"},
			{Value: "short", Label: "short", Description: "second"},
		},
		5,
		selectListIdentityTheme(),
		SelectListLayoutOptions{
			MinPrimaryColumnWidth: 12,
			MaxPrimaryColumnWidth: 12,
			TruncatePrimary: func(ctx SelectListTruncatePrimaryContext) string {
				if len(ctx.Text) <= ctx.MaxWidth {
					return ctx.Text
				}
				return ctx.Text[:max(0, ctx.MaxWidth-1)] + "…"
			},
		},
	)

	rendered := list.Render(80)
	if !strings.Contains(rendered[0], "…") {
		t.Fatalf("custom truncation marker missing: %q", rendered[0])
	}
	if got, want := visibleIndexOf(t, rendered[0], "first"), visibleIndexOf(t, rendered[1], "second"); got != want {
		t.Fatalf("description columns differ: got %d want %d\n%q\n%q", got, want, rendered[0], rendered[1])
	}
}

func TestSelectListFilterMatchesValueOnly(t *testing.T) {
	list := NewSelectList(
		[]SelectItem{
			{Value: "run", Label: "Build command"},
			{Value: "fmt", Label: "Run formatter"},
		},
		5,
		selectListIdentityTheme(),
	)

	list.SetFilter("run")
	rendered := strings.Join(list.Render(80), "\n")
	if !strings.Contains(rendered, "Build command") {
		t.Fatalf("value prefix match should remain visible: %q", rendered)
	}
	if strings.Contains(rendered, "Run formatter") {
		t.Fatalf("label-only prefix match should not be visible: %q", rendered)
	}
}

func selectListIdentityTheme() SelectListTheme {
	return SelectListTheme{
		SelectedPrefix: func(text string) string { return text },
		SelectedText:   func(text string) string { return text },
		Description:    func(text string) string { return text },
		ScrollInfo:     func(text string) string { return text },
		NoMatch:        func(text string) string { return text },
	}
}

func visibleIndexOf(t *testing.T, line, text string) int {
	t.Helper()
	index := strings.Index(line, text)
	if index < 0 {
		t.Fatalf("%q not found in %q", text, line)
	}
	return VisibleWidth(line[:index])
}

type settingsSubmenuComponent struct {
	done func(string, bool)
}

func (s *settingsSubmenuComponent) Render(int) []string { return []string{"submenu"} }
func (s *settingsSubmenuComponent) Invalidate()         {}
func (s *settingsSubmenuComponent) HandleInput(data string) {
	if MatchesKey(data, "enter") {
		s.done("opus", true)
	}
}

func TestSettingsListCyclesUpdatesAndCancels(t *testing.T) {
	var changes []string
	cancelled := 0
	list := NewSettingsList(
		[]SettingItem{
			{ID: "theme", Label: "Theme", Description: "Color mode", CurrentValue: "dark", Values: []string{"light", "dark", "system"}},
			{ID: "verbose", Label: "Verbose logging", CurrentValue: "off", Values: []string{"off", "on"}},
		},
		5,
		SettingsListTheme{
			Label:        func(text string, selected bool) string { return text },
			CurrentValue: func(text string, selected bool) string { return "[" + text + "]" },
			Description:  func(text string) string { return "<" + text + ">" },
			Hint:         func(text string) string { return text },
		},
		SettingsListOptions{
			OnChange: func(id string, newValue string) { changes = append(changes, id+"="+newValue) },
			OnCancel: func() { cancelled++ },
		},
	)

	list.HandleInput("\r")
	if len(changes) != 1 || changes[0] != "theme=system" {
		t.Fatalf("changes = %#v, want theme=system", changes)
	}
	list.UpdateValue("theme", "light")
	rendered := strings.Join(list.Render(80), "\n")
	if !strings.Contains(rendered, "[light]") || !strings.Contains(rendered, "<  Color mode>") {
		t.Fatalf("settings render missing updated value/description: %q", rendered)
	}
	list.HandleInput("\x1b")
	if cancelled != 1 {
		t.Fatalf("cancelled = %d, want 1", cancelled)
	}
}

func TestSettingsListSearchAndSubmenu(t *testing.T) {
	var changes []string
	list := NewSettingsList(
		[]SettingItem{
			{ID: "theme", Label: "Theme", CurrentValue: "dark", Values: []string{"dark", "light"}},
			{ID: "model", Label: "Model", CurrentValue: "sonnet", Submenu: func(currentValue string, done func(string, bool)) Component {
				if currentValue != "sonnet" {
					t.Fatalf("submenu current value = %q, want sonnet", currentValue)
				}
				return &settingsSubmenuComponent{done: done}
			}},
		},
		5,
		SettingsListTheme{},
		SettingsListOptions{
			EnableSearch: true,
			OnChange:     func(id string, newValue string) { changes = append(changes, id+"="+newValue) },
		},
	)

	list.HandleInput("m")
	list.HandleInput("o")
	rendered := strings.Join(list.Render(80), "\n")
	if strings.Contains(rendered, "Theme") || !strings.Contains(rendered, "Model") {
		t.Fatalf("search should filter to Model, got %q", rendered)
	}
	list.HandleInput("\r")
	if got := strings.Join(list.Render(80), "\n"); got != "submenu" {
		t.Fatalf("submenu render = %q, want submenu", got)
	}
	list.HandleInput("\r")
	if len(changes) != 1 || changes[0] != "model=opus" {
		t.Fatalf("submenu changes = %#v, want model=opus", changes)
	}
	if rendered = strings.Join(list.Render(80), "\n"); strings.Contains(rendered, "submenu") || !strings.Contains(rendered, "opus") {
		t.Fatalf("settings list should restore after submenu with new value: %q", rendered)
	}
}

func TestSettingsListPiEmptyStateHints(t *testing.T) {
	theme := SettingsListTheme{
		Hint: func(text string) string { return "<" + text + ">" },
	}
	list := NewSettingsList(nil, 5, theme)
	lines := list.Render(80)
	if len(lines) != 1 || lines[0] != "<  No settings available>" {
		t.Fatalf("empty list without search = %#v, want only no-settings line", lines)
	}

	searchable := NewSettingsList(nil, 5, theme, SettingsListOptions{EnableSearch: true})
	rendered := strings.Join(searchable.Render(80), "\n")
	if !strings.Contains(rendered, "<  No settings available>") {
		t.Fatalf("searchable empty list missing no-settings line: %q", rendered)
	}
	if !strings.Contains(rendered, "Type to search") {
		t.Fatalf("searchable empty list should include search hint: %q", rendered)
	}
}

func TestSettingsListTreatsEmptyCurrentValueAsPiValue(t *testing.T) {
	var changes []string
	list := NewSettingsList(
		[]SettingItem{
			{ID: "unset", Label: "Unsettable", Value: "legacy-alias", CurrentValue: "", Values: []string{"", "enabled"}},
			{ID: "alias", Label: "Alias", Value: "legacy", Values: []string{"legacy", "modern"}},
		},
		5,
		SettingsListTheme{},
		SettingsListOptions{OnChange: func(id, newValue string) { changes = append(changes, id+"="+newValue) }},
	)

	rendered := strings.Join(list.Render(40), "\n")
	if strings.Contains(rendered, "legacy-alias") {
		t.Fatalf("explicit empty current value should not fall back to Value alias: %q", rendered)
	}

	list.HandleInput("\r")
	if len(changes) != 1 || changes[0] != "unset=enabled" {
		t.Fatalf("empty current value should cycle to first non-empty choice, changes=%#v", changes)
	}
	if rendered = strings.Join(list.Render(40), "\n"); !strings.Contains(rendered, "enabled") {
		t.Fatalf("updated empty-value setting should render enabled: %q", rendered)
	}

	list.HandleInput("\x1b[B")
	rendered = strings.Join(list.Render(40), "\n")
	if !strings.Contains(rendered, "legacy") {
		t.Fatalf("Value alias should still render when CurrentValue is not set and no empty choice exists: %q", rendered)
	}
	list.HandleInput("\r")
	if len(changes) != 2 || changes[1] != "alias=modern" {
		t.Fatalf("Value alias setting should continue to cycle from alias value, changes=%#v", changes)
	}
}

func TestInputEditingAndSubmit(t *testing.T) {
	input := NewInput()
	submitted := ""
	input.OnSubmit = func(text string) { submitted = text }
	input.HandleInput("a")
	input.HandleInput("b")
	input.HandleInput("\x7f")
	input.HandleInput("\r")
	if submitted != "a" {
		t.Fatalf("submitted = %q, want a", submitted)
	}

	input = NewInput()
	submitted = ""
	input.OnSubmit = func(text string) { submitted = text }
	for _, ch := range []string{"h", "e", "l", "l", "o"} {
		input.HandleInput(ch)
	}
	input.HandleInput("\\")
	input.HandleInput("\r")
	if submitted != "hello\\" {
		t.Fatalf("input should submit literal backslash, got %q", submitted)
	}

	input = NewInput()
	input.HandleInput("\\")
	input.HandleInput("x")
	if input.GetValue() != "\\x" {
		t.Fatalf("input should insert backslash as a regular character, got %q", input.GetValue())
	}
}

func TestInputSetValueClampsCursorAndNarrowPrompt(t *testing.T) {
	input := NewInput()
	input.SetText("abcdef")
	input.HandleInput("\x01")
	input.HandleInput("\x1b[C")
	input.HandleInput("\x1b[C")
	input.SetValue("xyz")
	input.HandleInput("!")
	if input.GetValue() != "xy!z" {
		t.Fatalf("SetValue should preserve clamped cursor, got %q", input.GetValue())
	}
	input.SetValue("a")
	input.HandleInput("!")
	if input.GetValue() != "a!" {
		t.Fatalf("SetValue should clamp cursor at value end, got %q", input.GetValue())
	}

	if got := input.Render(1); len(got) != 1 || got[0] != "> " {
		t.Fatalf("narrow input prompt = %#v, want full prompt", got)
	}

	submitted := ""
	input.OnSubmit = func(text string) { submitted = text }
	input.HandleInput("\n")
	if submitted != "a!" {
		t.Fatalf("linefeed submit = %q, want current value", submitted)
	}
}

func TestInputBatchedPrintableTextLikePi(t *testing.T) {
	input := NewInput()
	input.HandleInput("hello")
	if input.GetValue() != "hello" {
		t.Fatalf("batched input = %q, want hello", input.GetValue())
	}

	input.HandleInput("\x01")
	input.HandleInput("\x1b[C")
	input.HandleInput("XY")
	if input.GetValue() != "hXYello" {
		t.Fatalf("batched middle insert = %q, want hXYello", input.GetValue())
	}
	input.HandleInput("\x1b[45;5u")
	if input.GetValue() != "hello" {
		t.Fatalf("undo batched middle insert = %q, want hello", input.GetValue())
	}

	input = NewInput()
	input.HandleInput("äö😀")
	if input.GetValue() != "äö😀" {
		t.Fatalf("batched unicode input = %q, want äö😀", input.GetValue())
	}
	input.HandleInput("\x01")
	input.HandleInput("\x1b[C")
	input.HandleInput("\x7f")
	if input.GetValue() != "ö😀" {
		t.Fatalf("unicode grapheme delete after batch = %q, want ö😀", input.GetValue())
	}

	input = NewInput()
	input.HandleInput("ok\x1b")
	input.HandleInput("\u0080")
	if input.GetValue() != "" {
		t.Fatalf("batched controls should be ignored, got %q", input.GetValue())
	}
}

func TestInputEscapeCallback(t *testing.T) {
	input := NewInput()
	cancelled := 0
	input.OnEscape = func() { cancelled++ }
	input.HandleInput("\x1b")
	input.HandleInput("\x03")
	if cancelled != 2 {
		t.Fatalf("escape callback count = %d, want 2", cancelled)
	}
}

func TestInputIgnoresUnsupportedModifiedPrintableKeys(t *testing.T) {
	input := NewInput()
	input.HandleInput("\x1b[99;9u")
	if input.GetValue() != "" {
		t.Fatalf("super-modified printable key should not insert text, got %q", input.GetValue())
	}
	input.HandleInput("\x1b[69;2u")
	if input.GetValue() != "E" {
		t.Fatalf("shifted printable key = %q, want E", input.GetValue())
	}
	input.HandleInput("\x1b[99:67:99;2u")
	if input.GetValue() != "EC" {
		t.Fatalf("shifted printable key field = %q, want EC", input.GetValue())
	}
}

func TestInputRenderWideTextKeepsCursorVisible(t *testing.T) {
	input := NewInput()
	input.SetValue("가나다라마바사아자차카타파하")
	input.SetFocused(true)
	input.HandleInput("\x01")
	for range 5 {
		input.HandleInput("\x1b[C")
	}

	lines := input.Render(20)
	if len(lines) != 1 {
		t.Fatalf("line count = %d, want 1", len(lines))
	}
	if VisibleWidth(lines[0]) > 20 {
		t.Fatalf("rendered input overflowed: width=%d line=%q", VisibleWidth(lines[0]), lines[0])
	}
	if !strings.Contains(lines[0], CursorMarker) || !strings.Contains(lines[0], "\x1b[7m") {
		t.Fatalf("focused render should include cursor marker and inverse cursor: %q", lines[0])
	}
}

func TestInputPiWideTextRenderMatrix(t *testing.T) {
	const width = 93
	cases := []string{
		"가나다라마바사아자차카타파하 한글 텍스트가 터미널 너비를 초과하면 크래시가 발생합니다 이것은 재현용 테스트입니다",
		"これはテスト文章です。日本語のテキストが正しく表示されるかどうかを確認するためのサンプルテキストです。あいうえお",
		"这是一段测试文本，用于验证中文字符在终端中的显示宽度是否被正确计算，如果不正确就会导致用户界面崩溃的问题",
		"ＡＢＣＤＥＦＧＨＩＪＫＬＭＮＯＰＱＲＳＴＵＶＷＸＹＺ０１２３４５６７８９ａｂｃｄｅｆｇｈｉｊｋｌｍ",
	}
	cursorPositions := []struct {
		name string
		move func(*Input)
	}{
		{name: "start", move: func(*Input) {}},
		{name: "middle", move: func(input *Input) {
			for range 10 {
				input.HandleInput("\x1b[C")
			}
		}},
		{name: "end", move: func(input *Input) { input.HandleInput("\x05") }},
	}

	for _, text := range cases {
		for _, cursor := range cursorPositions {
			t.Run(cursor.name+"/"+text[:min(len(text), 12)], func(t *testing.T) {
				input := NewInput()
				input.SetValue(text)
				input.SetFocused(true)
				cursor.move(input)
				lines := input.Render(width)
				if len(lines) != 1 {
					t.Fatalf("render line count = %d, want 1", len(lines))
				}
				if got := VisibleWidth(lines[0]); got > width {
					t.Fatalf("rendered line overflowed at %s: width=%d line=%q", cursor.name, got, lines[0])
				}
			})
		}
	}
}

func TestInputGraphemeClusterNavigationDeletionAndRender(t *testing.T) {
	input := NewInput()
	for _, r := range "A👍🏽👨‍💻🇺🇸éZ" {
		input.HandleInput(string(r))
	}
	for _, want := range []string{"A👍🏽👨‍💻🇺🇸é", "A👍🏽👨‍💻🇺🇸", "A👍🏽👨‍💻", "A👍🏽", "A"} {
		input.HandleInput("\x7f")
		if input.Text() != want {
			t.Fatalf("after backspace text = %q, want %q", input.Text(), want)
		}
	}

	input.SetText("A👍🏽B")
	input.HandleInput("\x1b[D")
	input.HandleInput("x")
	if input.Text() != "A👍🏽xB" {
		t.Fatalf("insert after moving left over B = %q", input.Text())
	}

	input.SetText("A👍🏽B")
	input.HandleInput("\x01")
	input.HandleInput("\x1b[C")
	input.HandleInput("\x1b[3~")
	if input.Text() != "AB" {
		t.Fatalf("forward delete should remove full emoji cluster, got %q", input.Text())
	}

	input.SetText("A👍🏽B")
	input.HandleInput("\x01")
	input.HandleInput("\x1b[C")
	input.SetFocused(true)
	line := strings.Join(input.Render(30), "\n")
	if !strings.Contains(line, "\x1b[7m👍🏽\x1b[27m") {
		t.Fatalf("fake cursor should highlight full grapheme cluster: %q", line)
	}
}

func TestInputKillRingReadlineShortcuts(t *testing.T) {
	input := NewInput()
	input.SetValue("foo bar baz")
	input.HandleInput("\x05") // Ctrl+E
	input.HandleInput("\x17") // Ctrl+W deletes baz
	if input.GetValue() != "foo bar " {
		t.Fatalf("after ctrl+w = %q", input.GetValue())
	}
	input.HandleInput("\x01") // Ctrl+A
	input.HandleInput("\x19") // Ctrl+Y
	if input.GetValue() != "bazfoo bar " {
		t.Fatalf("after yank = %q", input.GetValue())
	}

	input.SetValue("one two three")
	input.HandleInput("\x05")
	input.HandleInput("\x17")
	input.HandleInput("\x17")
	input.HandleInput("\x17")
	if input.GetValue() != "" {
		t.Fatalf("after consecutive kills = %q", input.GetValue())
	}
	input.HandleInput("\x19")
	if input.GetValue() != "one two three" {
		t.Fatalf("accumulated yank = %q", input.GetValue())
	}

	input.SetValue("prefix|suffix")
	input.HandleInput("\x01")
	for i := 0; i < len("prefix"); i++ {
		input.HandleInput("\x1b[C")
	}
	input.HandleInput("\x0b") // Ctrl+K deletes from cursor to end.
	if input.GetValue() != "prefix" {
		t.Fatalf("ctrl+k forward kill = %q, want prefix", input.GetValue())
	}
	input.HandleInput("\x19")
	if input.GetValue() != "prefix|suffix" {
		t.Fatalf("yank after ctrl+k forward kill = %q, want prefix|suffix", input.GetValue())
	}
}

func TestInputPiKillRingEdgeCases(t *testing.T) {
	t.Run("ctrl+u saves deleted text", func(t *testing.T) {
		input := NewInput()
		input.SetValue("hello world")
		input.HandleInput("\x01")
		for range 6 {
			input.HandleInput("\x1b[C")
		}
		input.HandleInput("\x15")
		if input.GetValue() != "world" {
			t.Fatalf("after ctrl+u = %q, want world", input.GetValue())
		}
		input.HandleInput("\x19")
		if input.GetValue() != "hello world" {
			t.Fatalf("after yank = %q, want hello world", input.GetValue())
		}
	})

	t.Run("ctrl+k saves deleted text", func(t *testing.T) {
		input := NewInput()
		input.SetValue("hello world")
		input.HandleInput("\x01")
		input.HandleInput("\x0b")
		if input.GetValue() != "" {
			t.Fatalf("after ctrl+k = %q, want empty", input.GetValue())
		}
		input.HandleInput("\x19")
		if input.GetValue() != "hello world" {
			t.Fatalf("after yank = %q, want hello world", input.GetValue())
		}
	})

	t.Run("empty and single-entry yank-pop no-op", func(t *testing.T) {
		input := NewInput()
		input.SetValue("test")
		input.HandleInput("\x05")
		input.HandleInput("\x19")
		if input.GetValue() != "test" {
			t.Fatalf("empty kill-ring yank should be no-op, got %q", input.GetValue())
		}

		input = NewInput()
		input.SetValue("only")
		input.HandleInput("\x05")
		input.HandleInput("\x17")
		input.HandleInput("\x19")
		if input.GetValue() != "only" {
			t.Fatalf("single yank = %q, want only", input.GetValue())
		}
		input.HandleInput("\x1by")
		if input.GetValue() != "only" {
			t.Fatalf("single-entry yank-pop should be no-op, got %q", input.GetValue())
		}
	})

	t.Run("non-delete action breaks kill accumulation", func(t *testing.T) {
		input := NewInput()
		input.SetValue("foo bar baz")
		input.HandleInput("\x05")
		input.HandleInput("\x17")
		if input.GetValue() != "foo bar " {
			t.Fatalf("after first kill = %q, want foo bar ", input.GetValue())
		}
		input.HandleInput("x")
		input.HandleInput("\x17")
		input.HandleInput("\x19")
		if input.GetValue() != "foo bar x" {
			t.Fatalf("most recent separated kill should yank x, got %q", input.GetValue())
		}
		input.HandleInput("\x1by")
		if input.GetValue() != "foo bar baz" {
			t.Fatalf("yank-pop should cycle to previous kill, got %q", input.GetValue())
		}
	})

	t.Run("non-yank action breaks yank-pop chain", func(t *testing.T) {
		input := NewInput()
		input.SetValue("first")
		input.HandleInput("\x05")
		input.HandleInput("\x17")
		input.SetValue("second")
		input.HandleInput("\x05")
		input.HandleInput("\x17")
		input.SetValue("")
		input.HandleInput("\x19")
		if input.GetValue() != "second" {
			t.Fatalf("first yank = %q, want second", input.GetValue())
		}
		input.HandleInput("x")
		input.HandleInput("\x1by")
		if input.GetValue() != "secondx" {
			t.Fatalf("yank-pop after typing should be no-op, got %q", input.GetValue())
		}
	})

	t.Run("middle yank and yank-pop placement", func(t *testing.T) {
		input := NewInput()
		input.SetValue("FIRST")
		input.HandleInput("\x05")
		input.HandleInput("\x17")
		input.SetValue("SECOND")
		input.HandleInput("\x05")
		input.HandleInput("\x17")
		input.SetValue("hello world")
		input.HandleInput("\x01")
		for range 6 {
			input.HandleInput("\x1b[C")
		}
		input.HandleInput("\x19")
		if input.GetValue() != "hello SECONDworld" {
			t.Fatalf("middle yank = %q, want hello SECONDworld", input.GetValue())
		}
		input.HandleInput("\x1by")
		if input.GetValue() != "hello FIRSTworld" {
			t.Fatalf("middle yank-pop = %q, want hello FIRSTworld", input.GetValue())
		}
	})
}

func TestInputYankPopCyclesKillRing(t *testing.T) {
	input := NewInput()
	for _, value := range []string{"first", "second", "third"} {
		input.SetValue(value)
		input.HandleInput("\x05")
		input.HandleInput("\x17")
	}
	input.HandleInput("\x19")
	if input.GetValue() != "third" {
		t.Fatalf("first yank = %q", input.GetValue())
	}
	input.HandleInput("\x1by")
	if input.GetValue() != "second" {
		t.Fatalf("first yank-pop = %q", input.GetValue())
	}
	input.HandleInput("\x1by")
	if input.GetValue() != "first" {
		t.Fatalf("second yank-pop = %q", input.GetValue())
	}

	input.HandleInput("x")
	input.SetValue("")
	input.HandleInput("\x19")
	if input.GetValue() != "first" {
		t.Fatalf("new yank after partial rotation = %q, want first", input.GetValue())
	}
}

func TestInputPiUndoEdgeCases(t *testing.T) {
	t.Run("empty undo no-op", func(t *testing.T) {
		input := NewInput()
		input.HandleInput("\x1b[45;5u")
		if input.GetValue() != "" {
			t.Fatalf("empty undo = %q, want empty", input.GetValue())
		}
	})

	t.Run("spaces undo one at a time", func(t *testing.T) {
		input := NewInput()
		for _, ch := range "hello  " {
			input.HandleInput(string(ch))
		}
		if input.GetValue() != "hello  " {
			t.Fatalf("typed value = %q, want hello two-spaces", input.GetValue())
		}
		input.HandleInput("\x1b[45;5u")
		if input.GetValue() != "hello " {
			t.Fatalf("first space undo = %q, want hello one-space", input.GetValue())
		}
		input.HandleInput("\x1b[45;5u")
		if input.GetValue() != "hello" {
			t.Fatalf("second space undo = %q, want hello", input.GetValue())
		}
		input.HandleInput("\x1b[45;5u")
		if input.GetValue() != "" {
			t.Fatalf("word undo = %q, want empty", input.GetValue())
		}
	})

	t.Run("undo yank", func(t *testing.T) {
		input := NewInput()
		for _, ch := range "hello " {
			input.HandleInput(string(ch))
		}
		input.HandleInput("\x17")
		input.HandleInput("\x19")
		if input.GetValue() != "hello " {
			t.Fatalf("after yank = %q, want hello-space", input.GetValue())
		}
		input.HandleInput("\x1b[45;5u")
		if input.GetValue() != "" {
			t.Fatalf("undo yank = %q, want empty", input.GetValue())
		}
	})
}

func TestInputUndoUsesConfiguredKeybindingLikePi(t *testing.T) {
	previous := GetKeybindings()
	SetKeybindings(NewKeybindingsManager(KeybindingsConfig{
		"tui.editor.undo": []string{"ctrl+z"},
	}))
	defer SetKeybindings(previous)

	input := NewInput()
	input.HandleInput("hello")
	input.HandleInput("\x1b[45;5u")
	if input.GetValue() != "hello" {
		t.Fatalf("default undo key should be replaced by custom binding, got %q", input.GetValue())
	}
	input.HandleInput("\x1a")
	if input.GetValue() != "" {
		t.Fatalf("custom undo key = %q, want empty", input.GetValue())
	}
}

func TestInputUsesResolvedEditingKeybindingsLikePi(t *testing.T) {
	t.Run("submit and cancel overrides", func(t *testing.T) {
		previous := GetKeybindings()
		SetKeybindings(NewKeybindingsManager(KeybindingsConfig{
			"tui.input.submit":  []string{"ctrl+s"},
			"tui.select.cancel": []string{"ctrl+g"},
		}))
		defer SetKeybindings(previous)

		input := NewInput()
		input.SetText("hello")
		submitted := ""
		cancelled := 0
		input.OnSubmit = func(text string) { submitted = text }
		input.OnEscape = func() { cancelled++ }

		input.HandleInput("\r")
		input.HandleInput("\x1b")
		if submitted != "" || cancelled != 0 {
			t.Fatalf("default submit/cancel keys should be replaced, submitted=%q cancelled=%d", submitted, cancelled)
		}
		input.HandleInput("\x13")
		input.HandleInput("\x07")
		if submitted != "hello" || cancelled != 1 {
			t.Fatalf("custom submit/cancel keys failed, submitted=%q cancelled=%d", submitted, cancelled)
		}
	})

	t.Run("cursor and character delete overrides", func(t *testing.T) {
		previous := GetKeybindings()
		SetKeybindings(NewKeybindingsManager(KeybindingsConfig{
			"tui.editor.cursorLineStart":    []string{"ctrl+p"},
			"tui.editor.cursorLineEnd":      []string{"ctrl+o"},
			"tui.editor.cursorLeft":         []string{"ctrl+g"},
			"tui.editor.cursorRight":        []string{"ctrl+t"},
			"tui.editor.deleteCharBackward": []string{"ctrl+x"},
			"tui.editor.deleteCharForward":  []string{"ctrl+r"},
		}))
		defer SetKeybindings(previous)

		input := NewInput()
		input.SetText("abcd")
		input.HandleInput("\x10")
		input.HandleInput(">")
		input.HandleInput("\x0f")
		input.HandleInput("<")
		input.HandleInput("\x07")
		input.HandleInput("!")
		input.HandleInput("\x14")
		input.HandleInput("?")
		if input.GetValue() != ">abcd!<?" {
			t.Fatalf("custom cursor keys produced %q", input.GetValue())
		}

		input.SetText("abcd")
		input.HandleInput("\x10")
		input.HandleInput("\x14")
		input.HandleInput("\x12")
		input.HandleInput("\x18")
		if input.GetValue() != "cd" {
			t.Fatalf("custom delete keys produced %q", input.GetValue())
		}
	})

	t.Run("word delete and kill ring overrides", func(t *testing.T) {
		previous := GetKeybindings()
		SetKeybindings(NewKeybindingsManager(KeybindingsConfig{
			"tui.editor.cursorLineStart":    []string{"ctrl+p"},
			"tui.editor.cursorWordLeft":     []string{"ctrl+l"},
			"tui.editor.deleteWordBackward": []string{"ctrl+v"},
			"tui.editor.deleteWordForward":  []string{"ctrl+n"},
			"tui.editor.deleteToLineEnd":    []string{"ctrl+o"},
			"tui.editor.yank":               []string{"ctrl+r"},
			"tui.editor.yankPop":            []string{"alt+r"},
		}))
		defer SetKeybindings(previous)

		input := NewInput()
		input.SetText("hello world test")
		input.HandleInput("\x0c")
		input.HandleInput("\x16")
		if input.GetValue() != "hello test" {
			t.Fatalf("custom backward word delete = %q", input.GetValue())
		}
		input.HandleInput("\x10")
		input.HandleInput("\x0e")
		if input.GetValue() != " test" {
			t.Fatalf("custom forward word delete = %q", input.GetValue())
		}

		input.SetText("first second")
		input.HandleInput("\x10")
		input.HandleInput("\x0f")
		input.SetText("alpha beta")
		input.HandleInput("\x10")
		input.HandleInput("\x0f")
		input.HandleInput("\x12")
		if input.GetValue() != "alpha beta" {
			t.Fatalf("custom yank = %q", input.GetValue())
		}
		input.HandleInput("\x1br")
		if input.GetValue() != "first second" {
			t.Fatalf("custom yank-pop = %q", input.GetValue())
		}
	})
}

func TestInputKeybindingPriorityMatchesPi(t *testing.T) {
	t.Run("undo beats custom submit on ctrl-minus", func(t *testing.T) {
		previous := GetKeybindings()
		SetKeybindings(NewKeybindingsManager(KeybindingsConfig{
			"tui.input.submit": []string{"ctrl+-"},
		}))
		defer SetKeybindings(previous)

		input := NewInput()
		input.HandleInput("hello")
		submitted := ""
		input.OnSubmit = func(text string) { submitted = text }
		input.HandleInput("\x1b[45;5u")
		if submitted != "" || input.GetValue() != "" {
			t.Fatalf("undo should run before custom submit, submitted=%q value=%q", submitted, input.GetValue())
		}
	})

	t.Run("backspace delete beats custom line start", func(t *testing.T) {
		previous := GetKeybindings()
		SetKeybindings(NewKeybindingsManager(KeybindingsConfig{
			"tui.editor.cursorLineStart": []string{"backspace"},
		}))
		defer SetKeybindings(previous)

		input := NewInput()
		input.SetText("hello")
		input.HandleInput("\x7f")
		if input.GetValue() != "hell" {
			t.Fatalf("backspace should delete before custom line start, got %q", input.GetValue())
		}
	})
}

func TestInputUndoDeletePasteAndForwardWord(t *testing.T) {
	input := NewInput()
	for _, ch := range "hello world" {
		input.HandleInput(string(ch))
	}
	input.HandleInput("\x1b[45;5u")
	if input.GetValue() != "hello" {
		t.Fatalf("first undo = %q, want hello", input.GetValue())
	}
	input.HandleInput("\x1b[45;5u")
	if input.GetValue() != "" {
		t.Fatalf("second undo = %q, want empty", input.GetValue())
	}

	input.SetValue("hello")
	input.HandleInput("\x01")
	input.HandleInput("\x06") // Ctrl+F
	input.HandleInput("\x04") // Ctrl+D
	if input.GetValue() != "hllo" {
		t.Fatalf("forward delete = %q", input.GetValue())
	}
	input.HandleInput("\x1b[45;5u")
	if input.GetValue() != "hello" {
		t.Fatalf("undo forward delete = %q", input.GetValue())
	}

	input.SetValue("hello world test")
	input.HandleInput("\x01")
	input.HandleInput("\x1bd")
	input.HandleInput("\x1bd")
	if input.GetValue() != " test" {
		t.Fatalf("alt+d deletes forward words = %q", input.GetValue())
	}
	input.HandleInput("\x19")
	if input.GetValue() != "hello world test" {
		t.Fatalf("alt+d kill accumulation yank = %q", input.GetValue())
	}

	input.SetValue("hello world")
	input.HandleInput("\x01")
	for range 5 {
		input.HandleInput("\x1b[C")
	}
	input.HandleInput("\x1b[200~beep boop\x1b[201~")
	if input.GetValue() != "hellobeep boop world" {
		t.Fatalf("paste = %q", input.GetValue())
	}
	input.HandleInput("\x1b[45;5u")
	if input.GetValue() != "hello world" {
		t.Fatalf("undo paste = %q", input.GetValue())
	}

	input.SetValue("hello world")
	input.HandleInput("\x01")
	input.HandleInput("\x1bd")
	if input.GetValue() != " world" {
		t.Fatalf("alt+d delete = %q, want leading-space world", input.GetValue())
	}
	input.HandleInput("\x1b[45;5u")
	if input.GetValue() != "hello world" {
		t.Fatalf("undo alt+d = %q", input.GetValue())
	}
}

func TestInputUndoReadlineDeletionShortcutsLikePi(t *testing.T) {
	type step struct {
		key  string
		want string
	}
	cases := []struct {
		name  string
		setup func(*Input)
		steps []step
	}{
		{
			name: "backspace",
			setup: func(input *Input) {
				for _, ch := range "hello" {
					input.HandleInput(string(ch))
				}
			},
			steps: []step{
				{key: "\x7f", want: "hell"},
				{key: "\x1b[45;5u", want: "hello"},
			},
		},
		{
			name: "ctrl+w",
			setup: func(input *Input) {
				for _, ch := range "hello world" {
					input.HandleInput(string(ch))
				}
			},
			steps: []step{
				{key: "\x17", want: "hello "},
				{key: "\x1b[45;5u", want: "hello world"},
			},
		},
		{
			name: "ctrl+k",
			setup: func(input *Input) {
				for _, ch := range "hello world" {
					input.HandleInput(string(ch))
				}
				input.HandleInput("\x01")
				for i := 0; i < 6; i++ {
					input.HandleInput("\x1b[C")
				}
			},
			steps: []step{
				{key: "\x0b", want: "hello "},
				{key: "\x1b[45;5u", want: "hello world"},
			},
		},
		{
			name: "ctrl+u",
			setup: func(input *Input) {
				for _, ch := range "hello world" {
					input.HandleInput(string(ch))
				}
				input.HandleInput("\x01")
				for i := 0; i < 6; i++ {
					input.HandleInput("\x1b[C")
				}
			},
			steps: []step{
				{key: "\x15", want: "world"},
				{key: "\x1b[45;5u", want: "hello world"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := NewInput()
			tc.setup(input)
			for _, step := range tc.steps {
				input.HandleInput(step.key)
				if got := input.GetValue(); got != step.want {
					t.Fatalf("after %q value = %q, want %q", step.key, got, step.want)
				}
			}
		})
	}
}

func TestInputCursorMovementStartsNewUndoUnit(t *testing.T) {
	input := NewInput()
	for _, ch := range "abc" {
		input.HandleInput(string(ch))
	}
	input.HandleInput("\x01")
	input.HandleInput("\x05")
	for _, ch := range "de" {
		input.HandleInput(string(ch))
	}
	if input.GetValue() != "abcde" {
		t.Fatalf("input value = %q, want abcde", input.GetValue())
	}

	input.HandleInput("\x1b[45;5u")
	if input.GetValue() != "abc" {
		t.Fatalf("first undo = %q, want abc", input.GetValue())
	}
	input.HandleInput("\x1b[45;5u")
	if input.GetValue() != "" {
		t.Fatalf("second undo = %q, want empty", input.GetValue())
	}
}

func TestEditorPromptHistoryNavigation(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.AddToHistory("first")
	editor.AddToHistory("second")
	editor.AddToHistory("third")

	editor.HandleInput("\x1b[A")
	if editor.GetText() != "third" {
		t.Fatalf("first up = %q", editor.GetText())
	}
	editor.HandleInput("\x1b[A")
	if editor.GetText() != "second" {
		t.Fatalf("second up = %q", editor.GetText())
	}
	editor.HandleInput("\x1b[A")
	if editor.GetText() != "first" {
		t.Fatalf("third up = %q", editor.GetText())
	}
	editor.HandleInput("\x1b[B")
	if editor.GetText() != "second" {
		t.Fatalf("down = %q", editor.GetText())
	}
	editor.HandleInput("\x1b[B")
	editor.HandleInput("\x1b[B")
	if editor.GetText() != "" {
		t.Fatalf("down past newest should clear editor: %q", editor.GetText())
	}
}

func TestEditorHistoryUsesWrappedVisualLineBoundaries(t *testing.T) {
	editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
	wrapped := strings.Repeat("x", 25)
	editor.AddToHistory("older")
	editor.AddToHistory(wrapped)

	editor.HandleInput("\x1b[A")
	editor.Render(11)
	editor.HandleInput("\x1b[A")
	if editor.GetText() != wrapped {
		t.Fatalf("up inside wrapped history entry changed text to %q", editor.GetText())
	}
	if line, col := editor.GetCursor(); line != 0 || col != 15 {
		t.Fatalf("cursor after wrapped history up = (%d,%d), want (0,15)", line, col)
	}
	editor.HandleInput("\x1b[A")
	if line, col := editor.GetCursor(); line != 0 || col != 5 {
		t.Fatalf("cursor at first wrapped visual line = (%d,%d), want (0,5)", line, col)
	}
	editor.HandleInput("\x1b[A")
	if editor.GetText() != "older" {
		t.Fatalf("up from first visual line = %q, want older", editor.GetText())
	}
}

func TestEditorArrowAtVisualBoundsMovesToLineEdge(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetText("abc")
	editor.HandleInput("\x01")
	editor.HandleInput("\x1b[B")
	if line, col := editor.GetCursor(); line != 0 || col != 3 {
		t.Fatalf("down at last visual line = (%d,%d), want (0,3)", line, col)
	}
	editor.HandleInput("\x1b[A")
	if line, col := editor.GetCursor(); line != 0 || col != 0 {
		t.Fatalf("up at first visual line = (%d,%d), want (0,0)", line, col)
	}
}

func TestEditorPageScrollMovesByVisiblePage(t *testing.T) {
	editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
	editor.SetText(strings.Repeat("x", 60))
	editor.Render(11)

	editor.HandleInput("\x1b[5~")
	if line, col := editor.GetCursor(); line != 0 || col != 9 {
		t.Fatalf("page up cursor = (%d,%d), want (0,9)", line, col)
	}
	editor.HandleInput("\x1b[6~")
	if line, col := editor.GetCursor(); line != 0 || col != 60 {
		t.Fatalf("page down cursor = (%d,%d), want (0,60)", line, col)
	}
}

func TestEditorHistoryExitsOnTypingAndSkipsDuplicates(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.AddToHistory("")
	editor.AddToHistory("same")
	editor.AddToHistory("same")
	editor.HandleInput("\x1b[A")
	editor.HandleInput("x")
	if editor.GetText() != "samex" {
		t.Fatalf("typing should append to history entry: %q", editor.GetText())
	}
	editor.SetText("")
	editor.HandleInput("\x1b[A")
	editor.HandleInput("\x1b[A")
	if editor.GetText() != "same" {
		t.Fatalf("duplicates should be collapsed: %q", editor.GetText())
	}
}

func TestEditorHistoryLimitKeepsMostRecentHundredEntries(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	for i := 0; i < 105; i++ {
		editor.AddToHistory(fmt.Sprintf("prompt %d", i))
	}
	for i := 0; i < 100; i++ {
		editor.HandleInput("\x1b[A")
	}
	if editor.GetText() != "prompt 5" {
		t.Fatalf("oldest retained history = %q, want prompt 5", editor.GetText())
	}
	editor.HandleInput("\x1b[A")
	if editor.GetText() != "prompt 5" {
		t.Fatalf("history should stay at oldest retained entry, got %q", editor.GetText())
	}
}

func TestEditorUndoExitsHistoryBrowsingToPreHistoryState(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.AddToHistory("hello")

	for _, r := range "world" {
		editor.HandleInput(string(r))
	}
	editor.HandleInput("\x17") // Ctrl+W
	if editor.GetText() != "" {
		t.Fatalf("ctrl+w should clear typed word, got %q", editor.GetText())
	}

	editor.HandleInput("\x1b[A")
	if editor.GetText() != "hello" {
		t.Fatalf("history up = %q, want hello", editor.GetText())
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "" {
		t.Fatalf("undo should restore pre-history empty state, got %q", editor.GetText())
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "world" {
		t.Fatalf("second undo should restore pre-delete text, got %q", editor.GetText())
	}
}

func TestEditorUndoSkipsIntermediateHistoryNavigationStates(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.AddToHistory("first")
	editor.AddToHistory("second")
	editor.AddToHistory("third")

	for _, r := range "current" {
		editor.HandleInput(string(r))
	}
	editor.HandleInput("\x17") // Ctrl+W
	if editor.GetText() != "" {
		t.Fatalf("ctrl+w should clear typed word, got %q", editor.GetText())
	}

	for _, want := range []string{"third", "second", "first"} {
		editor.HandleInput("\x1b[A")
		if editor.GetText() != want {
			t.Fatalf("history up = %q, want %q", editor.GetText(), want)
		}
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "" {
		t.Fatalf("undo should restore pre-history empty state, got %q", editor.GetText())
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "current" {
		t.Fatalf("second undo should restore pre-delete text, got %q", editor.GetText())
	}
}

func TestEditorPublicAccessorsReturnCursorAndLines(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	if line, col := editor.GetCursor(); line != 0 || col != 0 {
		t.Fatalf("empty cursor = (%d,%d), want (0,0)", line, col)
	}

	editor.HandleInput("a")
	editor.HandleInput("b")
	editor.HandleInput("c")
	if line, col := editor.GetCursor(); line != 0 || col != 3 {
		t.Fatalf("typed cursor = (%d,%d), want (0,3)", line, col)
	}

	editor.HandleInput("\x1b[D")
	if line, col := editor.GetCursor(); line != 0 || col != 2 {
		t.Fatalf("left cursor = (%d,%d), want (0,2)", line, col)
	}

	editor.SetText("a\nb")
	lines := editor.GetLines()
	if strings.Join(lines, "|") != "a|b" {
		t.Fatalf("lines = %#v, want [a b]", lines)
	}
	lines[0] = "mutated"
	if got := strings.Join(editor.GetLines(), "|"); got != "a|b" {
		t.Fatalf("GetLines should return a defensive slice copy, got %q", got)
	}

	editor.SetText("äö\n😀")
	if line, col := editor.GetCursor(); line != 1 || col != 1 {
		t.Fatalf("unicode cursor = (%d,%d), want rune-based (1,1)", line, col)
	}
}

func TestEditorSetTextNormalizesNotifiesAndIsUndoable(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	var changes []string
	editor.SetOnChange(func(text string) { changes = append(changes, text) })

	editor.SetText("a\r\nb\tc")
	if got := editor.GetText(); got != "a\nb    c" {
		t.Fatalf("SetText should normalize line endings and tabs, got %q", got)
	}
	if line, col := editor.GetCursor(); line != 1 || col != 6 {
		t.Fatalf("SetText cursor = (%d,%d), want (1,6)", line, col)
	}
	if len(changes) != 1 || changes[0] != "a\nb    c" {
		t.Fatalf("SetText OnChange = %#v, want normalized text", changes)
	}

	editor.HandleInput("\x1b[45;5u")
	if got := editor.GetText(); got != "" {
		t.Fatalf("undo after SetText = %q, want empty", got)
	}
	if len(changes) != 2 || changes[1] != "" {
		t.Fatalf("undo OnChange = %#v, want empty notification", changes)
	}
}

func TestEditorInsertTextAtCursorAtomicMultilineAndNormalized(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetText("hello world")
	editor.HandleInput("\x01")
	for range 5 {
		editor.HandleInput("\x1b[C")
	}

	editor.InsertTextAtCursor("line1\r\nline2\rline3\tend")
	if got := editor.GetText(); got != "helloline1\nline2\nline3    end world" {
		t.Fatalf("InsertTextAtCursor normalized text = %q", got)
	}
	if line, col := editor.GetCursor(); line != 2 || col != 12 {
		t.Fatalf("InsertTextAtCursor cursor = (%d,%d), want (2,12)", line, col)
	}

	editor.HandleInput("\x1b[45;5u")
	if got := editor.GetText(); got != "hello world" {
		t.Fatalf("undo InsertTextAtCursor = %q, want hello world", got)
	}
	editor.HandleInput("|")
	if got := editor.GetText(); got != "hello| world" {
		t.Fatalf("cursor after undo InsertTextAtCursor = %q, want hello| world", got)
	}
}

func TestEditorRuntimeOptionsClampAndAffectRender(t *testing.T) {
	editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: -4, AutocompleteMaxVisible: 99})
	if editor.GetPaddingX() != 0 {
		t.Fatalf("initial padding = %d, want 0", editor.GetPaddingX())
	}
	if editor.GetAutocompleteMaxVisible() != 20 {
		t.Fatalf("initial autocomplete max = %d, want 20", editor.GetAutocompleteMaxVisible())
	}

	editor.SetPaddingX(2)
	editor.SetText("hi")
	lines := editor.Render(12)
	if !strings.HasPrefix(stripANSI(lines[1]), "  hi") {
		t.Fatalf("padding should affect render: %#v", stripANSILines(lines))
	}

	editor.SetAutocompleteMaxVisible(1)
	if editor.GetAutocompleteMaxVisible() != 3 {
		t.Fatalf("small autocomplete max = %d, want 3", editor.GetAutocompleteMaxVisible())
	}
	editor.SetAutocompleteMaxVisible(0)
	if editor.GetAutocompleteMaxVisible() != 5 {
		t.Fatalf("zero autocomplete max = %d, want default 5", editor.GetAutocompleteMaxVisible())
	}
	editor.SetMaxVisibleLines(4)
	if editor.GetMaxVisibleLines() != 4 {
		t.Fatalf("max visible lines = %d, want 4", editor.GetMaxVisibleLines())
	}
}

func TestEditorComponentInterfacesAndSetters(t *testing.T) {
	var _ EditorComponent = (*Editor)(nil)
	var _ EditorHistoryComponent = (*Editor)(nil)
	var _ EditorTextInserter = (*Editor)(nil)
	var _ EditorExpandedTextProvider = (*Editor)(nil)
	var _ EditorAutocompleteComponent = (*Editor)(nil)
	var _ EditorAppearanceComponent = (*Editor)(nil)
	var _ EditorSubmitCallbackComponent = (*Editor)(nil)
	var _ EditorChangeCallbackComponent = (*Editor)(nil)

	editor := NewEditor(EditorTheme{})
	changed := ""
	editor.SetOnChange(func(text string) { changed = text })
	editor.InsertTextAtCursor("hello")
	if changed != "hello" {
		t.Fatalf("SetOnChange callback saw %q, want hello", changed)
	}

	submitted := ""
	editor.SetOnSubmit(func(text string) { submitted = text })
	editor.HandleInput("\r")
	if submitted != "hello" {
		t.Fatalf("SetOnSubmit callback saw %q, want hello", submitted)
	}

	editor.SetBorderColor(func(text string) string { return "<" + text + ">" })
	if got := editor.Render(8)[0]; !strings.Contains(got, "<") {
		t.Fatalf("SetBorderColor should affect border render, got %q", got)
	}
}

func TestEditorBackslashEnterNewlineWorkaround(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.HandleInput("\\")
	if editor.GetText() != "\\" {
		t.Fatalf("backslash should be inserted immediately, got %q", editor.GetText())
	}
	editor.HandleInput("\r")
	if editor.GetText() != "\n" {
		t.Fatalf("backslash+enter = %q, want newline", editor.GetText())
	}

	submitted := false
	editor = NewEditor(EditorTheme{})
	editor.OnSubmit = func(string) { submitted = true }
	editor.HandleInput("\\")
	editor.HandleInput("x")
	editor.HandleInput("\r")
	if !submitted {
		t.Fatalf("enter after non-backslash cursor should submit")
	}

	editor = NewEditor(EditorTheme{})
	editor.HandleInput("\\")
	editor.HandleInput("\\")
	editor.HandleInput("\\")
	editor.HandleInput("\r")
	if editor.GetText() != "\\\\\n" {
		t.Fatalf("multiple backslashes should remove only one, got %q", editor.GetText())
	}
}

func TestEditorPiNewlineInputSequences(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{name: "linefeed", data: "\n"},
		{name: "batched linefeed", data: "\nextra"},
		{name: "escape carriage return", data: "\x1b\r"},
		{name: "csi shift enter", data: "\x1b[13;2~"},
		{name: "batched escape carriage return", data: "\x1b[ignored\r"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			editor := NewEditor(EditorTheme{})
			submitted := false
			editor.OnSubmit = func(string) { submitted = true }
			editor.SetText("a")

			editor.HandleInput(tc.data)

			if submitted {
				t.Fatalf("%q should insert a newline, not submit", tc.data)
			}
			if got := editor.GetText(); got != "a\n" {
				t.Fatalf("after %q text = %q, want newline insertion", tc.data, got)
			}
			editor.HandleInput("\x1b[45;5u")
			if got := editor.GetText(); got != "a" {
				t.Fatalf("undo after %q = %q, want original text", tc.data, got)
			}
		})
	}
}

func TestEditorDisableSubmitIgnoresEnter(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.DisableSubmit = true
	submitted := false
	editor.OnSubmit = func(string) { submitted = true }
	editor.SetText("draft")
	editor.HandleInput("\r")
	if submitted {
		t.Fatalf("disabled submit should not call OnSubmit")
	}
	if editor.GetText() != "draft" {
		t.Fatalf("disabled submit should preserve text, got %q", editor.GetText())
	}

	editor.SetText("\\")
	editor.HandleInput("\r")
	if editor.GetText() != "\\" {
		t.Fatalf("disabled submit should not apply backslash newline workaround, got %q", editor.GetText())
	}
}

func TestEditorCSIuPrintableInput(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.HandleInput("\x1b[99;9u")
	if editor.GetText() != "" {
		t.Fatalf("unsupported CSI-u modifiers should not insert text, got %q", editor.GetText())
	}

	editor.HandleInput("\x1b[69;2u")
	editor.HandleInput("\x1b[99:67:99;2u")
	editor.HandleInput("\x1b[27;2;69~")
	if editor.GetText() != "ECE" {
		t.Fatalf("shifted printable CSI-u text = %q, want ECE", editor.GetText())
	}
}

func TestEditorShiftBackspaceAndDeleteAliases(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetText("abc")
	editor.HandleInput("\x1b[127;2u")
	if got := editor.GetText(); got != "ab" {
		t.Fatalf("shift+backspace text = %q, want ab", got)
	}
	editor.HandleInput("\x1b[45;5u")
	if got := editor.GetText(); got != "abc" {
		t.Fatalf("undo shift+backspace = %q, want abc", got)
	}

	editor.HandleInput("\x01")
	editor.HandleInput("\x06")
	editor.HandleInput("\x1b[3;2~")
	if got := editor.GetText(); got != "ac" {
		t.Fatalf("shift+delete text = %q, want ac", got)
	}
	editor.HandleInput("\x1b[45;5u")
	if got := editor.GetText(); got != "abc" {
		t.Fatalf("undo shift+delete = %q, want abc", got)
	}
}

func TestEditorGraphemeClusterNavigationAndDeletion(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	for _, r := range "A👍🏽👨‍💻🇺🇸e\u0301Z" {
		editor.HandleInput(string(r))
	}

	editor.HandleInput("\x7f")
	if editor.GetText() != "A👍🏽👨‍💻🇺🇸e\u0301" {
		t.Fatalf("backspace plain rune = %q", editor.GetText())
	}
	editor.HandleInput("\x7f")
	if editor.GetText() != "A👍🏽👨‍💻🇺🇸" {
		t.Fatalf("backspace combining cluster = %q", editor.GetText())
	}
	editor.HandleInput("\x7f")
	if editor.GetText() != "A👍🏽👨‍💻" {
		t.Fatalf("backspace flag cluster = %q", editor.GetText())
	}
	editor.HandleInput("\x7f")
	if editor.GetText() != "A👍🏽" {
		t.Fatalf("backspace zwj cluster = %q", editor.GetText())
	}
	editor.HandleInput("\x7f")
	if editor.GetText() != "A" {
		t.Fatalf("backspace skin-tone cluster = %q", editor.GetText())
	}

	editor.SetText("A👍🏽👨‍💻B")
	editor.HandleInput("\x1b[D")
	editor.HandleInput("\x1b[D")
	editor.HandleInput("x")
	if editor.GetText() != "A👍🏽x👨‍💻B" {
		t.Fatalf("left over grapheme clusters inserted at wrong position: %q", editor.GetText())
	}

	editor.SetText("A👍🏽B")
	editor.HandleInput("\x01")
	editor.HandleInput("\x1b[C")
	editor.HandleInput("\x1b[3~")
	if editor.GetText() != "AB" {
		t.Fatalf("forward delete grapheme cluster = %q", editor.GetText())
	}
}

func TestEditorUnicodeTextEditingMatchesPi(t *testing.T) {
	t.Run("inserts mixed unicode text literally", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		for _, input := range []string{"H", "e", "l", "l", "o", " ", "ä", "ö", "ü", " ", "😀"} {
			editor.HandleInput(input)
		}
		if got := editor.GetText(); got != "Hello äöü 😀" {
			t.Fatalf("mixed unicode text = %q", got)
		}
	})

	t.Run("backspace deletes unicode graphemes", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		for _, input := range []string{"ä", "ö", "ü"} {
			editor.HandleInput(input)
		}
		editor.HandleInput("\x7f")
		if got := editor.GetText(); got != "äö" {
			t.Fatalf("umlaut backspace = %q, want äö", got)
		}

		editor = NewEditor(EditorTheme{})
		editor.HandleInput("😀")
		editor.HandleInput("👍")
		editor.HandleInput("\x7f")
		if got := editor.GetText(); got != "😀" {
			t.Fatalf("emoji backspace = %q, want 😀", got)
		}
	})

	t.Run("cursor movement inserts around unicode", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		for _, input := range []string{"ä", "ö", "ü"} {
			editor.HandleInput(input)
		}
		editor.HandleInput("\x1b[D")
		editor.HandleInput("\x1b[D")
		editor.HandleInput("x")
		if got := editor.GetText(); got != "äxöü" {
			t.Fatalf("umlaut insertion after cursor movement = %q", got)
		}

		editor = NewEditor(EditorTheme{})
		for _, input := range []string{"😀", "👍", "🎉"} {
			editor.HandleInput(input)
		}
		editor.HandleInput("\x1b[D")
		editor.HandleInput("\x1b[D")
		editor.HandleInput("x")
		if got := editor.GetText(); got != "😀x👍🎉" {
			t.Fatalf("emoji insertion after cursor movement = %q", got)
		}
	})

	t.Run("unicode survives line breaks and set text", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		for _, input := range []string{"ä", "ö", "ü", "\n", "Ä", "Ö", "Ü"} {
			editor.HandleInput(input)
		}
		if got := editor.GetText(); got != "äöü\nÄÖÜ" {
			t.Fatalf("unicode multiline text = %q", got)
		}

		editor.SetText("Hällö Wörld! 😀 äöüÄÖÜß")
		if got := editor.GetText(); got != "Hällö Wörld! 😀 äöüÄÖÜß" {
			t.Fatalf("unicode SetText = %q", got)
		}
	})

	t.Run("readline movement and word navigation remain unicode safe", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.HandleInput("a")
		editor.HandleInput("b")
		editor.HandleInput("\x01")
		editor.HandleInput("x")
		if got := editor.GetText(); got != "xab" {
			t.Fatalf("Ctrl+A insertion = %q, want xab", got)
		}

		editor.SetText("   foo bar")
		editor.HandleInput("\x01")
		editor.HandleInput("\x1b[1;5C")
		if line, col := editor.GetCursor(); line != 0 || col != 6 {
			t.Fatalf("Ctrl+Right over leading whitespace = (%d,%d), want (0,6)", line, col)
		}
	})
}

func TestEditorMultilineVerticalMovement(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetText("line1\nline2")
	editor.HandleInput("\x1b[A")
	editor.HandleInput("X")
	if editor.GetText() != "line1X\nline2" {
		t.Fatalf("up should move cursor within multiline text: %q", editor.GetText())
	}
}

func TestEditorPunctuationAwareWordDeletionAndNavigation(t *testing.T) {
	editor := NewEditor(EditorTheme{})

	editor.SetText("foo bar...")
	editor.HandleInput("\x17")
	if editor.GetText() != "foo bar" {
		t.Fatalf("ctrl+w should delete punctuation run, got %q", editor.GetText())
	}

	editor.SetText("foo 😀😀 bar")
	editor.HandleInput("\x17")
	if editor.GetText() != "foo 😀😀 " {
		t.Fatalf("ctrl+w should delete trailing word, got %q", editor.GetText())
	}
	editor.HandleInput("\x17")
	if editor.GetText() != "foo " {
		t.Fatalf("ctrl+w should delete emoji word run, got %q", editor.GetText())
	}

	editor.SetText("foo bar")
	editor.HandleInput("\x1b\x7f")
	if editor.GetText() != "foo " {
		t.Fatalf("alt+backspace = %q, want foo space", editor.GetText())
	}

	editor.SetText("foo bar... baz")
	editor.HandleInput("\x1b[1;5D")
	if line, col := editor.GetCursor(); line != 0 || col != 11 {
		t.Fatalf("ctrl+left over baz = (%d,%d), want (0,11)", line, col)
	}
	editor.HandleInput("\x1b[1;5D")
	if line, col := editor.GetCursor(); line != 0 || col != 7 {
		t.Fatalf("ctrl+left over punctuation = (%d,%d), want (0,7)", line, col)
	}
	editor.HandleInput("\x1b[1;5D")
	if line, col := editor.GetCursor(); line != 0 || col != 4 {
		t.Fatalf("ctrl+left over word = (%d,%d), want (0,4)", line, col)
	}
	editor.HandleInput("\x1b[1;5C")
	if line, col := editor.GetCursor(); line != 0 || col != 7 {
		t.Fatalf("ctrl+right over word = (%d,%d), want (0,7)", line, col)
	}
	editor.HandleInput("\x1b[1;5C")
	if line, col := editor.GetCursor(); line != 0 || col != 10 {
		t.Fatalf("ctrl+right over punctuation = (%d,%d), want (0,10)", line, col)
	}
	editor.HandleInput("\x1b[1;5C")
	if line, col := editor.GetCursor(); line != 0 || col != 14 {
		t.Fatalf("ctrl+right over baz = (%d,%d), want (0,14)", line, col)
	}

	editor.SetText("foo bar")
	editor.HandleInput("\x01")
	editor.HandleInput("\x1bd")
	if editor.GetText() != " bar" {
		t.Fatalf("alt+d should delete word forward, got %q", editor.GetText())
	}

	editor.SetText("abcd")
	editor.HandleInput("\x01")
	editor.HandleInput("\x06") // Ctrl+F
	editor.HandleInput("\x04") // Ctrl+D
	if editor.GetText() != "acd" {
		t.Fatalf("ctrl+f/ctrl+d editing = %q, want acd", editor.GetText())
	}
	editor.HandleInput("\x02") // Ctrl+B
	if line, col := editor.GetCursor(); line != 0 || col != 0 {
		t.Fatalf("ctrl+b cursor = (%d,%d), want (0,0)", line, col)
	}
}

func TestEditorKillRingYankPopRotationPersists(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	for _, text := range []string{"first", "second", "third"} {
		editor.SetText(text)
		editor.HandleInput("\x17") // Ctrl+W
	}
	editor.SetText("")

	editor.HandleInput("\x19") // Ctrl+Y, most recent
	if editor.GetText() != "third" {
		t.Fatalf("initial yank = %q, want third", editor.GetText())
	}
	editor.HandleInput("\x1by") // Alt+Y
	if editor.GetText() != "second" {
		t.Fatalf("first yank-pop = %q, want second", editor.GetText())
	}
	editor.HandleInput("\x1by")
	if editor.GetText() != "first" {
		t.Fatalf("second yank-pop = %q, want first", editor.GetText())
	}
	editor.HandleInput("\x1by")
	if editor.GetText() != "third" {
		t.Fatalf("third yank-pop = %q, want third", editor.GetText())
	}

	editor.HandleInput("x")
	editor.SetText("")
	editor.HandleInput("\x19")
	if editor.GetText() != "third" {
		t.Fatalf("new yank after full rotation = %q, want third", editor.GetText())
	}

	editor.HandleInput("\x1by")
	if editor.GetText() != "second" {
		t.Fatalf("rotation should persist after yank-pop, got %q", editor.GetText())
	}
	editor.HandleInput("x")
	editor.SetText("")
	editor.HandleInput("\x19")
	if editor.GetText() != "second" {
		t.Fatalf("new yank after partial rotation = %q, want second", editor.GetText())
	}
}

func TestEditorKillRingPiAccumulationAndYankPlacement(t *testing.T) {
	t.Run("ctrl u accumulates multiline deletes", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("line1\nline2\nline3")

		editor.HandleInput("\x15")
		if editor.GetText() != "line1\nline2\n" {
			t.Fatalf("first ctrl+u = %q", editor.GetText())
		}
		editor.HandleInput("\x15")
		if editor.GetText() != "line1\nline2" {
			t.Fatalf("second ctrl+u should delete newline, got %q", editor.GetText())
		}
		editor.HandleInput("\x15")
		editor.HandleInput("\x15")
		editor.HandleInput("\x15")
		if editor.GetText() != "" {
			t.Fatalf("ctrl+u chain should clear text, got %q", editor.GetText())
		}
		editor.HandleInput("\x19")
		if editor.GetText() != "line1\nline2\nline3" {
			t.Fatalf("yank after ctrl+u chain = %q", editor.GetText())
		}
	})

	t.Run("ctrl w coalesces across lines", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("1\n2\n3")
		for i := 0; i < 5; i++ {
			editor.HandleInput("\x17")
		}
		if editor.GetText() != "" {
			t.Fatalf("ctrl+w chain should clear text, got %q", editor.GetText())
		}
		editor.HandleInput("\x19")
		if editor.GetText() != "1\n2\n3" {
			t.Fatalf("yank after ctrl+w chain = %q", editor.GetText())
		}
	})

	t.Run("non delete breaks kill accumulation", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("foo bar baz")
		editor.HandleInput("\x17")
		editor.HandleInput("x")
		editor.HandleInput("\x17")
		editor.HandleInput("\x19")
		if editor.GetText() != "foo bar x" {
			t.Fatalf("yank should restore newest separate kill, got %q", editor.GetText())
		}
		editor.HandleInput("\x1by")
		if editor.GetText() != "foo bar baz" {
			t.Fatalf("yank-pop should restore previous separate kill, got %q", editor.GetText())
		}
	})

	t.Run("yank and yank pop in middle of text", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("FIRST")
		editor.HandleInput("\x17")
		editor.SetText("SECOND")
		editor.HandleInput("\x17")
		editor.SetText("hello world")
		editor.HandleInput("\x01")
		for i := 0; i < 6; i++ {
			editor.HandleInput("\x1b[C")
		}

		editor.HandleInput("\x19")
		if editor.GetText() != "hello SECONDworld" {
			t.Fatalf("middle yank = %q", editor.GetText())
		}
		editor.HandleInput("\x1by")
		if editor.GetText() != "hello FIRSTworld" {
			t.Fatalf("middle yank-pop = %q", editor.GetText())
		}
	})

	t.Run("alt d at line end deletes newline", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("line1\nline2")
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x05")
		editor.HandleInput("\x1bd")
		if editor.GetText() != "line1line2" {
			t.Fatalf("alt+d at line end = %q", editor.GetText())
		}
		editor.HandleInput("\x19")
		if editor.GetText() != "line1\nline2" {
			t.Fatalf("yank after alt+d newline = %q", editor.GetText())
		}
	})

	t.Run("ctrl k at line end deletes newline and coalesces", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("ab\ncd")
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x05")

		editor.HandleInput("\x0b")
		if editor.GetText() != "abcd" {
			t.Fatalf("ctrl+k at line end = %q, want abcd", editor.GetText())
		}
		editor.HandleInput("\x0b")
		if editor.GetText() != "ab" {
			t.Fatalf("second ctrl+k = %q, want ab", editor.GetText())
		}
		editor.HandleInput("\x19")
		if editor.GetText() != "ab\ncd" {
			t.Fatalf("yank after coalesced ctrl+k = %q, want ab\\ncd", editor.GetText())
		}
	})
}

func TestEditorHistoryNavigationBreaksYankPopChain(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.AddToHistory("history")
	for _, text := range []string{"first", "second"} {
		editor.SetText(text)
		editor.HandleInput("\x17") // Ctrl+W
	}
	editor.SetText("")
	editor.HandleInput("\x19") // Ctrl+Y
	if editor.GetText() != "second" {
		t.Fatalf("initial yank = %q, want second", editor.GetText())
	}

	editor.SetText("")
	editor.HandleInput("\x1b[A")
	if editor.GetText() != "history" {
		t.Fatalf("history up = %q, want history", editor.GetText())
	}
	editor.HandleInput("\x1by")
	if editor.GetText() != "history" {
		t.Fatalf("history navigation should break yank-pop chain, got %q", editor.GetText())
	}
}

func TestEditorUsesResolvedKeybindings(t *testing.T) {
	previous := GetKeybindings()
	SetKeybindings(NewKeybindingsManager(KeybindingsConfig{
		"tui.editor.cursorWordLeft":  {"alt+b"},
		"tui.editor.cursorWordRight": {"alt+f"},
		"tui.editor.cursorLineStart": {"home"},
		"tui.editor.cursorLineEnd":   {"end"},
	}))
	defer SetKeybindings(previous)

	editor := NewEditor(EditorTheme{})
	editor.SetText("foo bar")
	editor.HandleInput("\x1b[1;5D")
	if line, col := editor.GetCursor(); line != 0 || col != 7 {
		t.Fatalf("ctrl+left should not match overridden word-left binding, got (%d,%d)", line, col)
	}
	editor.HandleInput("\x1bb")
	if line, col := editor.GetCursor(); line != 0 || col != 4 {
		t.Fatalf("alt+b cursor = (%d,%d), want (0,4)", line, col)
	}
	editor.HandleInput("\x1bf")
	if line, col := editor.GetCursor(); line != 0 || col != 7 {
		t.Fatalf("alt+f cursor = (%d,%d), want (0,7)", line, col)
	}
	editor.HandleInput("\x1b[H")
	if line, col := editor.GetCursor(); line != 0 || col != 0 {
		t.Fatalf("home cursor = (%d,%d), want (0,0)", line, col)
	}
	editor.HandleInput("\x1b[F")
	if line, col := editor.GetCursor(); line != 0 || col != 7 {
		t.Fatalf("end cursor = (%d,%d), want (0,7)", line, col)
	}
}

func TestEditorCharacterJump(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetText("hello world")
	editor.HandleInput("\x01")
	editor.HandleInput("\x1d")
	editor.HandleInput("o")
	if line, col := editor.GetCursor(); line != 0 || col != 4 {
		t.Fatalf("forward jump cursor = (%d,%d), want (0,4)", line, col)
	}
	if editor.GetText() != "hello world" {
		t.Fatalf("jump input should not insert text, got %q", editor.GetText())
	}

	editor.HandleInput("\x1d")
	editor.HandleInput("o")
	if line, col := editor.GetCursor(); line != 0 || col != 7 {
		t.Fatalf("second forward jump cursor = (%d,%d), want (0,7)", line, col)
	}

	editor.HandleInput("\x1b\x1d")
	editor.HandleInput("h")
	if line, col := editor.GetCursor(); line != 0 || col != 0 {
		t.Fatalf("backward jump cursor = (%d,%d), want (0,0)", line, col)
	}

	editor.SetText("one\ntwo\nthree")
	editor.HandleInput("\x01")
	editor.HandleInput("\x1b[A")
	editor.HandleInput("\x1b[A")
	editor.HandleInput("\x1d")
	editor.HandleInput("t")
	if line, col := editor.GetCursor(); line != 1 || col != 0 {
		t.Fatalf("multiline forward jump cursor = (%d,%d), want (1,0)", line, col)
	}

	editor.HandleInput("\x1d")
	editor.HandleInput("\x1d")
	editor.HandleInput("x")
	if editor.GetText() != "one\nxtwo\nthree" {
		t.Fatalf("second ctrl+] should cancel jump mode before typing, got %q", editor.GetText())
	}
}

func TestEditorCharacterJumpPiEdgeCases(t *testing.T) {
	assertCursor := func(t *testing.T, editor *Editor, wantLine, wantCol int) {
		t.Helper()
		if line, col := editor.GetCursor(); line != wantLine || col != wantCol {
			t.Fatalf("cursor = (%d,%d), want (%d,%d)", line, col, wantLine, wantCol)
		}
	}

	t.Run("backward multiline and not found", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("abc\ndef\nghi")
		assertCursor(t, editor, 2, 3)

		editor.HandleInput("\x1b\x1d")
		editor.HandleInput("a")
		assertCursor(t, editor, 0, 0)

		editor.SetText("hello world")
		editor.HandleInput("\x1b\x1d")
		editor.HandleInput("z")
		assertCursor(t, editor, 0, 11)
	})

	t.Run("forward not found and empty text", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("hello world")
		editor.HandleInput("\x01")
		editor.HandleInput("\x1d")
		editor.HandleInput("z")
		assertCursor(t, editor, 0, 0)

		editor.SetText("")
		editor.HandleInput("\x1d")
		editor.HandleInput("x")
		assertCursor(t, editor, 0, 0)
	})

	t.Run("case-sensitive and special characters", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("Hello World")
		editor.HandleInput("\x01")
		editor.HandleInput("\x1d")
		editor.HandleInput("h")
		assertCursor(t, editor, 0, 0)
		editor.HandleInput("\x1d")
		editor.HandleInput("W")
		assertCursor(t, editor, 0, 6)

		editor.SetText("foo(bar) = baz;")
		editor.HandleInput("\x01")
		editor.HandleInput("\x1d")
		editor.HandleInput("(")
		assertCursor(t, editor, 0, 3)
		editor.HandleInput("\x1d")
		editor.HandleInput("=")
		assertCursor(t, editor, 0, 9)
	})

	t.Run("cancel modes insert normally afterward", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("hello world")
		editor.HandleInput("\x01")
		editor.HandleInput("\x1d")
		editor.HandleInput("\x1d")
		editor.HandleInput("o")
		if editor.GetText() != "ohello world" {
			t.Fatalf("typing after forward jump cancel inserted %q", editor.GetText())
		}

		editor.SetText("hello world")
		editor.HandleInput("\x1b\x1d")
		editor.HandleInput("\x1b\x1d")
		editor.HandleInput("o")
		if editor.GetText() != "hello worldo" {
			t.Fatalf("typing after backward jump cancel inserted %q", editor.GetText())
		}

		editor.SetText("hello world")
		editor.HandleInput("\x01")
		editor.HandleInput("\x1d")
		editor.HandleInput("\x1b")
		editor.HandleInput("o")
		if editor.GetText() != "ohello world" {
			t.Fatalf("typing after escape jump cancel inserted %q", editor.GetText())
		}
	})

	t.Run("control key after jump cancel falls through like Pi", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("hello")
		editor.HandleInput("\x1d")
		editor.HandleInput("\x7f")
		if editor.GetText() != "hell" {
			t.Fatalf("backspace after jump cancel = %q, want hell", editor.GetText())
		}

		editor.SetText("hello")
		editor.HandleInput("\x01")
		editor.HandleInput("\x1d")
		editor.HandleInput("\x1b[3~")
		if editor.GetText() != "ello" {
			t.Fatalf("delete after jump cancel = %q, want ello", editor.GetText())
		}
	})

	t.Run("jump resets undo coalescing", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("hello world")
		editor.HandleInput("\x01")
		editor.HandleInput("x")
		editor.HandleInput("\x1d")
		editor.HandleInput("o")
		editor.HandleInput("Y")
		if editor.GetText() != "xhellYo world" {
			t.Fatalf("after jump insert = %q, want xhellYo world", editor.GetText())
		}
		editor.HandleInput("\x1b[45;5u")
		if editor.GetText() != "xhello world" {
			t.Fatalf("undo after jump should only remove post-jump insert, got %q", editor.GetText())
		}
	})
}

func TestEditorInputCopyKeybindingIsNoOpLikePi(t *testing.T) {
	previous := GetKeybindings()
	SetKeybindings(NewKeybindingsManager(KeybindingsConfig{
		"tui.input.copy": []string{"backspace"},
	}))
	defer SetKeybindings(previous)

	editor := NewEditor(EditorTheme{})
	editor.SetText("hello")
	editor.HandleInput("\x7f")
	if editor.GetText() != "hello" {
		t.Fatalf("copy keybinding should be consumed as no-op, got %q", editor.GetText())
	}
}

func TestEditorKeybindingPriorityMatchesPi(t *testing.T) {
	t.Run("delete beats custom newline on backspace", func(t *testing.T) {
		previous := GetKeybindings()
		SetKeybindings(NewKeybindingsManager(KeybindingsConfig{
			"tui.input.newLine": []string{"backspace"},
		}))
		defer SetKeybindings(previous)

		editor := NewEditor(EditorTheme{})
		editor.SetText("hello")
		editor.HandleInput("\x7f")
		if editor.GetText() != "hell" {
			t.Fatalf("backspace should delete before custom newline, got %q", editor.GetText())
		}
	})

	t.Run("delete to line end beats custom submit on ctrl+k", func(t *testing.T) {
		previous := GetKeybindings()
		SetKeybindings(NewKeybindingsManager(KeybindingsConfig{
			"tui.input.submit": []string{"ctrl+k"},
		}))
		defer SetKeybindings(previous)

		editor := NewEditor(EditorTheme{})
		editor.SetText("hello world")
		editor.HandleInput("\x01")
		for range 5 {
			editor.HandleInput("\x1b[C")
		}
		submitted := ""
		editor.OnSubmit = func(text string) { submitted = text }
		editor.HandleInput("\x0b")
		if submitted != "" || editor.GetText() != "hello" {
			t.Fatalf("ctrl+k should delete before custom submit, submitted=%q text=%q", submitted, editor.GetText())
		}
	})
}

func TestEditorStickyColumnForLogicalLineMovement(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetText("1234567890\nabc\n1234567890")

	editor.HandleInput("\x1b[A")
	if line, col := editor.GetCursor(); line != 1 || col != 3 {
		t.Fatalf("up to short line = (%d,%d), want (1,3)", line, col)
	}
	editor.HandleInput("\x1b[A")
	if line, col := editor.GetCursor(); line != 0 || col != 10 {
		t.Fatalf("sticky up to long line = (%d,%d), want (0,10)", line, col)
	}

	editor.SetText("1234567890\nabc\n1234567890")
	editor.HandleInput("\x1b[A")
	editor.HandleInput("\x1b[D")
	editor.HandleInput("\x1b[A")
	if line, col := editor.GetCursor(); line != 0 || col != 2 {
		t.Fatalf("horizontal movement should reset sticky column, got (%d,%d), want (0,2)", line, col)
	}

	editor.SetText("1234567890\nabc\n1234567890")
	editor.HandleInput("\x1b[A")
	editor.HandleInput("x")
	editor.HandleInput("\x1b[A")
	if line, col := editor.GetCursor(); line != 0 || col != 4 {
		t.Fatalf("typing should reset sticky column, got (%d,%d), want (0,4)", line, col)
	}

	editor.SetText("111111111x1111111111\n\n333333333_")
	editor.HandleInput("\x1b[A")
	editor.HandleInput("\x1b[A")
	editor.HandleInput("\x05") // Ctrl+E
	if line, col := editor.GetCursor(); line != 0 || col != 20 {
		t.Fatalf("line-end setup cursor = (%d,%d), want (0,20)", line, col)
	}
	editor.HandleInput("\x1b[B")
	editor.HandleInput("\x1b[B")
	if line, col := editor.GetCursor(); line != 2 || col != 10 {
		t.Fatalf("down to short final line = (%d,%d), want (2,10)", line, col)
	}
	editor.HandleInput("\x1b[C")
	if line, col := editor.GetCursor(); line != 2 || col != 10 {
		t.Fatalf("right at final line end should not move, got (%d,%d)", line, col)
	}
	editor.HandleInput("\x1b[A")
	editor.HandleInput("\x1b[A")
	if line, col := editor.GetCursor(); line != 0 || col != 10 {
		t.Fatalf("right at final line end should set sticky column, got (%d,%d), want (0,10)", line, col)
	}
}

func TestEditorStickyColumnResetsOnEditingAndNavigationLikePi(t *testing.T) {
	assertCursor := func(t *testing.T, editor *Editor, wantLine, wantCol int) {
		t.Helper()
		if line, col := editor.GetCursor(); line != wantLine || col != wantCol {
			t.Fatalf("cursor = (%d,%d), want (%d,%d)", line, col, wantLine, wantCol)
		}
	}
	moveRight := func(editor *Editor, count int) {
		for range count {
			editor.HandleInput("\x1b[C")
		}
	}

	t.Run("backspace", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("1234567890\n\n1234567890")
		editor.HandleInput("\x01")
		moveRight(editor, 8)
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x1b[A")
		assertCursor(t, editor, 0, 8)
		editor.HandleInput("\x7f")
		assertCursor(t, editor, 0, 7)
		editor.HandleInput("\x1b[B")
		editor.HandleInput("\x1b[B")
		assertCursor(t, editor, 2, 7)
	})

	t.Run("ctrl+a", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("1234567890\n\n1234567890")
		editor.HandleInput("\x01")
		moveRight(editor, 8)
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x01")
		assertCursor(t, editor, 1, 0)
		editor.HandleInput("\x1b[A")
		assertCursor(t, editor, 0, 0)
	})

	t.Run("ctrl+e", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("12345\n\n1234567890")
		editor.HandleInput("\x01")
		moveRight(editor, 3)
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x1b[A")
		assertCursor(t, editor, 0, 3)
		editor.HandleInput("\x05")
		assertCursor(t, editor, 0, 5)
		editor.HandleInput("\x1b[B")
		editor.HandleInput("\x1b[B")
		assertCursor(t, editor, 2, 5)
	})

	t.Run("ctrl+left", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("hello world\n\nhello world")
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x1b[A")
		assertCursor(t, editor, 0, 11)
		editor.HandleInput("\x1b[1;5D")
		assertCursor(t, editor, 0, 6)
		editor.HandleInput("\x1b[B")
		editor.HandleInput("\x1b[B")
		assertCursor(t, editor, 2, 6)
	})

	t.Run("ctrl+right", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("hello world\n\nhello world")
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x01")
		assertCursor(t, editor, 0, 0)
		editor.HandleInput("\x1b[B")
		editor.HandleInput("\x1b[B")
		assertCursor(t, editor, 2, 0)
		editor.HandleInput("\x1b[1;5C")
		assertCursor(t, editor, 2, 5)
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x1b[A")
		assertCursor(t, editor, 0, 5)
	})

	t.Run("undo", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("1234567890\n\n1234567890")
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x01")
		moveRight(editor, 8)
		assertCursor(t, editor, 0, 8)
		editor.HandleInput("\x1b[B")
		editor.HandleInput("\x1b[B")
		assertCursor(t, editor, 2, 8)
		editor.HandleInput("X")
		assertCursor(t, editor, 2, 9)
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x1b[A")
		assertCursor(t, editor, 0, 9)
		editor.HandleInput("\x1b[45;5u")
		if editor.GetText() != "1234567890\n\n1234567890" {
			t.Fatalf("undo text = %q", editor.GetText())
		}
		assertCursor(t, editor, 2, 8)
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x1b[A")
		assertCursor(t, editor, 0, 8)
	})

	t.Run("setText", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("1234567890\n\n1234567890")
		editor.HandleInput("\x01")
		moveRight(editor, 8)
		editor.HandleInput("\x1b[A")
		editor.SetText("abcdefghij\n\nabcdefghij")
		assertCursor(t, editor, 2, 10)
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x1b[A")
		assertCursor(t, editor, 0, 10)
	})
}

func TestEditorStickyColumnRewrapsAfterResize(t *testing.T) {
	t.Run("same logical line rewrap uses current visual column", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("12345678901234567890\n\n12345678901234567890")
		editor.HandleInput("\x01")
		for range 15 {
			editor.HandleInput("\x1b[C")
		}
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x1b[A")
		if line, col := editor.GetCursor(); line != 0 || col != 15 {
			t.Fatalf("setup cursor = (%d,%d), want (0,15)", line, col)
		}

		editor.Render(12)
		editor.HandleInput("\x1b[B")
		editor.HandleInput("\x1b[B")
		if line, col := editor.GetCursor(); line != 2 || col != 4 {
			t.Fatalf("resize should rebase sticky to visual col 4, got (%d,%d)", line, col)
		}
	})

	t.Run("short intervening line keeps original preferred column", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("short\n12345678901234567890")
		editor.HandleInput("\x01")
		for range 15 {
			editor.HandleInput("\x1b[C")
		}
		if line, col := editor.GetCursor(); line != 1 || col != 15 {
			t.Fatalf("setup cursor = (%d,%d), want (1,15)", line, col)
		}
		editor.HandleInput("\x1b[A")
		if line, col := editor.GetCursor(); line != 0 || col != 5 {
			t.Fatalf("up to short line = (%d,%d), want (0,5)", line, col)
		}

		editor.Render(10)
		editor.HandleInput("\x1b[B")
		if line, col := editor.GetCursor(); line != 1 || col != 8 {
			t.Fatalf("narrow down should clamp old preferred col, got (%d,%d)", line, col)
		}
		editor.HandleInput("\x1b[A")
		if line, col := editor.GetCursor(); line != 0 || col != 5 {
			t.Fatalf("up to short line after narrow = (%d,%d), want (0,5)", line, col)
		}
		editor.Render(80)
		editor.HandleInput("\x1b[B")
		if line, col := editor.GetCursor(); line != 1 || col != 15 {
			t.Fatalf("wide down should restore old preferred col, got (%d,%d)", line, col)
		}
	})

	t.Run("rewrapped target fits current visual column", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("abcdefghijklmnopqr\n123456789012345678")
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x01")
		for range 18 {
			editor.HandleInput("\x1b[C")
		}
		if line, col := editor.GetCursor(); line != 0 || col != 18 {
			t.Fatalf("setup cursor = (%d,%d), want (0,18)", line, col)
		}

		editor.Render(10)
		editor.HandleInput("\x1b[B")
		if line, col := editor.GetCursor(); line != 1 || col != 8 {
			t.Fatalf("narrow down should clamp to current visual col, got (%d,%d), want (1,8)", line, col)
		}

		editor.Render(80)
		editor.HandleInput("\x1b[A")
		if line, col := editor.GetCursor(); line != 0 || col != 8 {
			t.Fatalf("wide up should keep current visual col, got (%d,%d), want (0,8)", line, col)
		}
		editor.HandleInput("\x1b[B")
		if line, col := editor.GetCursor(); line != 1 || col != 8 {
			t.Fatalf("preferred column should be cleared after rewrap fit, got (%d,%d), want (1,8)", line, col)
		}
	})

	t.Run("rewrapped target shorter than current visual column keeps current preference", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		editor.SetText("abcdefghijklmnopqr\n123456789012345678\nab")
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x1b[A")
		editor.HandleInput("\x01")
		for range 18 {
			editor.HandleInput("\x1b[C")
		}
		if line, col := editor.GetCursor(); line != 0 || col != 18 {
			t.Fatalf("setup cursor = (%d,%d), want (0,18)", line, col)
		}

		editor.Render(10)
		editor.HandleInput("\x1b[B")
		if line, col := editor.GetCursor(); line != 1 || col != 8 {
			t.Fatalf("narrow down should clamp to visual col 8, got (%d,%d)", line, col)
		}

		editor.Render(80)
		editor.HandleInput("\x1b[B")
		if line, col := editor.GetCursor(); line != 2 || col != 2 {
			t.Fatalf("down to short line should clamp to end, got (%d,%d), want (2,2)", line, col)
		}
		editor.HandleInput("\x1b[A")
		if line, col := editor.GetCursor(); line != 1 || col != 8 {
			t.Fatalf("up should restore current visual preference, got (%d,%d), want (1,8)", line, col)
		}
	})
}

func TestEditorUndoTypingCoalescesWordsAndSpaces(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	for _, ch := range "hello world" {
		editor.HandleInput(string(ch))
	}
	if editor.GetText() != "hello world" {
		t.Fatalf("text = %q", editor.GetText())
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "hello" {
		t.Fatalf("first undo = %q, want hello", editor.GetText())
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "" {
		t.Fatalf("second undo = %q, want empty", editor.GetText())
	}

	for _, ch := range "hi  " {
		editor.HandleInput(string(ch))
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "hi " {
		t.Fatalf("space undo = %q, want one trailing space", editor.GetText())
	}
}

func TestEditorUndoNewlineSplitsTypingUnitsLikePi(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	for _, ch := range "hello\nworld" {
		editor.HandleInput(string(ch))
	}
	if got := editor.GetText(); got != "hello\nworld" {
		t.Fatalf("typed text = %q, want hello newline world", got)
	}
	editor.HandleInput("\x1b[45;5u")
	if got := editor.GetText(); got != "hello\n" {
		t.Fatalf("first undo = %q, want hello newline", got)
	}
	editor.HandleInput("\x1b[45;5u")
	if got := editor.GetText(); got != "hello" {
		t.Fatalf("second undo = %q, want hello", got)
	}
	editor.HandleInput("\x1b[45;5u")
	if got := editor.GetText(); got != "" {
		t.Fatalf("third undo = %q, want empty", got)
	}
}

func TestEditorBatchedPrintableTextLikePi(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.HandleInput("hello")
	if editor.GetText() != "hello" {
		t.Fatalf("batched editor input = %q, want hello", editor.GetText())
	}
	editor.HandleInput(" world")
	if editor.GetText() != "hello world" {
		t.Fatalf("batched editor append = %q, want hello world", editor.GetText())
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "hello" {
		t.Fatalf("undo whitespace batch = %q, want hello", editor.GetText())
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "" {
		t.Fatalf("undo word batch = %q, want empty", editor.GetText())
	}

	editor = NewEditor(EditorTheme{})
	editor.HandleInput("hello")
	editor.HandleInput("\x01")
	editor.HandleInput("\x1b[C")
	editor.HandleInput("XY")
	if editor.GetText() != "hXYello" {
		t.Fatalf("batched editor middle insert = %q, want hXYello", editor.GetText())
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "hello" {
		t.Fatalf("undo batched editor middle insert = %q, want hello", editor.GetText())
	}

	editor = NewEditor(EditorTheme{})
	editor.HandleInput("äö😀")
	if editor.GetText() != "äö😀" {
		t.Fatalf("batched editor unicode = %q, want äö😀", editor.GetText())
	}
}

func TestEditorUndoDeleteKillAndYank(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetText("hello world")
	editor.HandleInput("\x7f")
	if editor.GetText() != "hello worl" {
		t.Fatalf("backspace = %q", editor.GetText())
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "hello world" {
		t.Fatalf("undo backspace = %q", editor.GetText())
	}

	editor.HandleInput("\x17")
	if editor.GetText() != "hello " {
		t.Fatalf("ctrl+w = %q", editor.GetText())
	}
	editor.HandleInput("\x19")
	if editor.GetText() != "hello world" {
		t.Fatalf("yank = %q", editor.GetText())
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "hello " {
		t.Fatalf("undo yank = %q", editor.GetText())
	}
}

func TestEditorUndoSubmitMovementAndNoOpDeleteBoundaries(t *testing.T) {
	t.Run("submit clears undo stack", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		submitted := ""
		editor.OnSubmit = func(text string) { submitted = text }
		for _, ch := range "hello" {
			editor.HandleInput(string(ch))
		}
		editor.HandleInput("\r")
		if submitted != "hello" || editor.GetText() != "" {
			t.Fatalf("submit text=%q editor=%q, want hello and empty editor", submitted, editor.GetText())
		}
		editor.HandleInput("\x1b[45;5u")
		if editor.GetText() != "" {
			t.Fatalf("undo after submit should be no-op, got %q", editor.GetText())
		}
	})

	t.Run("cursor movement starts new undo unit", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		for _, ch := range "hello world" {
			editor.HandleInput(string(ch))
		}
		for range 5 {
			editor.HandleInput("\x1b[D")
		}
		for _, ch := range "lol" {
			editor.HandleInput(string(ch))
		}
		if editor.GetText() != "hello lolworld" {
			t.Fatalf("insert after cursor movement = %q", editor.GetText())
		}
		editor.HandleInput("\x1b[45;5u")
		if editor.GetText() != "hello world" {
			t.Fatalf("undo after cursor movement = %q, want hello world", editor.GetText())
		}
		editor.HandleInput("|")
		if editor.GetText() != "hello |world" {
			t.Fatalf("cursor position after undo = %q, want hello |world", editor.GetText())
		}
	})

	t.Run("no-op deletes do not add undo snapshots", func(t *testing.T) {
		editor := NewEditor(EditorTheme{})
		for _, ch := range "hello" {
			editor.HandleInput(string(ch))
		}
		editor.HandleInput("\x17")
		if editor.GetText() != "" {
			t.Fatalf("ctrl+w should delete text, got %q", editor.GetText())
		}
		editor.HandleInput("\x17")
		editor.HandleInput("\x17")
		editor.HandleInput("\x1b[45;5u")
		if editor.GetText() != "hello" {
			t.Fatalf("undo after no-op deletes = %q, want hello", editor.GetText())
		}
	})
}

func TestEditorBracketedPasteAndUndoAreAtomic(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetText("hello world")
	editor.HandleInput("\x01")
	for range 5 {
		editor.HandleInput("\x1b[C")
	}
	editor.HandleInput("\x1b[200~line1\nline2\nline3\x1b[201~")
	if editor.GetText() != "helloline1\nline2\nline3 world" {
		t.Fatalf("paste = %q", editor.GetText())
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "hello world" {
		t.Fatalf("undo paste = %q", editor.GetText())
	}
	editor.HandleInput("|")
	if editor.GetText() != "hello| world" {
		t.Fatalf("cursor after undo = %q", editor.GetText())
	}
}

func TestEditorBracketedPasteDecodesCSIuControls(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.HandleInput("\x1b[200~line1\x1b[106;5uline2\x1b[106;5uline3\x1b[201~")
	if editor.GetText() != "line1\nline2\nline3" {
		t.Fatalf("decoded paste = %q", editor.GetText())
	}
}

func TestEditorPasteToEditorUsesPasteSemantics(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetText("open")
	editor.PasteToEditor("/tmp/file.txt")
	if editor.GetText() != "open /tmp/file.txt" {
		t.Fatalf("programmatic file paste should auto-space after word char, got %q", editor.GetText())
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "open" {
		t.Fatalf("programmatic paste should be undo-atomic, got %q", editor.GetText())
	}

	large := strings.Repeat("x", 1001)
	editor.SetText("")
	editor.PasteToEditor(large)
	if editor.GetText() != "[paste #1 1001 chars]" || editor.GetExpandedText() != large {
		t.Fatalf("large programmatic paste marker=%q expanded len=%d", editor.GetText(), len(editor.GetExpandedText()))
	}
}

func TestEditorLargePasteMarkerExpansionAndSubmit(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	pastedText := strings.Join([]string{
		"line 1",
		"line 2",
		"line 3",
		"line 4",
		"line 5",
		"line 6",
		"line 7",
		"line 8",
		"line 9",
		"line 10",
		"tokens $1 $2 $& $$ $` $' end",
	}, "\n")
	editor.HandleInput("\x1b[200~" + pastedText + "\x1b[201~")
	if editor.GetText() != "[paste #1 +11 lines]" {
		t.Fatalf("marker = %q", editor.GetText())
	}
	if editor.GetExpandedText() != pastedText {
		t.Fatalf("expanded = %q", editor.GetExpandedText())
	}
	submitted := ""
	editor.OnSubmit = func(text string) { submitted = text }
	editor.HandleInput("\r")
	if submitted != pastedText {
		t.Fatalf("submitted = %q", submitted)
	}
	if editor.GetText() != "" {
		t.Fatalf("editor should clear after submit, got %q", editor.GetText())
	}
}

func TestEditorPasteMarkerAtomicNavigationAndDelete(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.HandleInput("A")
	editor.HandleInput("\x1b[200~" + strings.TrimSuffix(strings.Repeat("line\n", 20), "\n") + "\x1b[201~")
	editor.HandleInput("B")
	marker := "[paste #1 +20 lines]"
	if editor.GetText() != "A"+marker+"B" {
		t.Fatalf("marker text = %q", editor.GetText())
	}

	editor.HandleInput("\x01")
	editor.HandleInput("\x1b[C")
	if _, col := editor.GetCursor(); col != 1 {
		t.Fatalf("cursor after A = %d, want 1", col)
	}
	editor.HandleInput("\x1b[C")
	if _, col := editor.GetCursor(); col != 1+len(marker) {
		t.Fatalf("cursor after marker = %d, want %d", col, 1+len(marker))
	}
	editor.HandleInput("\x1b[D")
	if _, col := editor.GetCursor(); col != 1 {
		t.Fatalf("left over marker = %d, want 1", col)
	}

	editor.HandleInput("\x1b[C")
	editor.HandleInput("\x7f")
	if editor.GetText() != "AB" {
		t.Fatalf("backspace marker = %q, want AB", editor.GetText())
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "A"+marker+"B" {
		t.Fatalf("undo marker delete = %q", editor.GetText())
	}

	editor.HandleInput("\x01")
	editor.HandleInput("\x1b[C")
	editor.HandleInput("\x1b[3~")
	if editor.GetText() != "AB" {
		t.Fatalf("forward delete marker = %q, want AB", editor.GetText())
	}
}

func TestEditorPasteMarkerAtomicWordMovementAndManualMarker(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetText("X ")
	editor.HandleInput("\x1b[200~" + strings.TrimSuffix(strings.Repeat("line\n", 20), "\n") + "\x1b[201~")
	editor.HandleInput(" ")
	editor.HandleInput("Y")
	marker := "[paste #1 +20 lines]"

	editor.HandleInput("\x01")
	editor.HandleInput("\x1b[1;5C")
	if _, col := editor.GetCursor(); col != 1 {
		t.Fatalf("ctrl+right over X = %d, want 1", col)
	}
	editor.HandleInput("\x1b[1;5C")
	if _, col := editor.GetCursor(); col != 2+len(marker) {
		t.Fatalf("ctrl+right over marker = %d, want %d", col, 2+len(marker))
	}

	fake := NewEditor(EditorTheme{})
	fake.SetText("[paste #99 +5 lines]")
	fake.HandleInput("\x01")
	fake.HandleInput("\x1b[C")
	if _, col := fake.GetCursor(); col != 1 {
		t.Fatalf("manual marker should move by one rune, col=%d", col)
	}
}

func TestEditorPasteMarkerVerticalMovementSnapsToMarkerStart(t *testing.T) {
	editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
	editor.SetText("12345678901234567890\n\nhello ")
	editor.HandleInput("\x1b[200~" + strings.Repeat("x", 2000) + "\x1b[201~")
	editor.Render(80)

	editor.HandleInput("\x1b[A")
	editor.HandleInput("\x1b[A")
	editor.HandleInput("\x01")
	for range 10 {
		editor.HandleInput("\x1b[C")
	}
	if line, col := editor.GetCursor(); line != 0 || col != 10 {
		t.Fatalf("initial cursor = (%d,%d), want (0,10)", line, col)
	}

	editor.HandleInput("\x1b[B")
	if line, col := editor.GetCursor(); line != 1 || col != 0 {
		t.Fatalf("down to empty line = (%d,%d), want (1,0)", line, col)
	}
	editor.HandleInput("\x1b[B")
	if line, col := editor.GetCursor(); line != 2 || col != 6 {
		t.Fatalf("down into marker = (%d,%d), want (2,6)", line, col)
	}
}

func TestEditorPasteMarkerVerticalMovementPreservesStickyColumn(t *testing.T) {
	editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
	editor.SetText("1234567890123456\n\n")
	editor.HandleInput("\x1b[200~" + strings.Repeat("x", 2000) + "\x1b[201~")
	editor.InsertTextAtCursor("\n\nabcdefghijklmnop")
	editor.Render(30)

	for range 4 {
		editor.HandleInput("\x1b[A")
	}
	editor.HandleInput("\x01")
	for range 10 {
		editor.HandleInput("\x1b[C")
	}
	if line, col := editor.GetCursor(); line != 0 || col != 10 {
		t.Fatalf("initial cursor = (%d,%d), want (0,10)", line, col)
	}

	editor.HandleInput("\x1b[B")
	if line, col := editor.GetCursor(); line != 1 || col != 0 {
		t.Fatalf("down to first empty line = (%d,%d), want (1,0)", line, col)
	}
	editor.HandleInput("\x1b[B")
	if line, col := editor.GetCursor(); line != 2 || col != 0 {
		t.Fatalf("down to marker line = (%d,%d), want (2,0)", line, col)
	}
	editor.HandleInput("\x1b[B")
	if line, col := editor.GetCursor(); line != 3 || col != 0 {
		t.Fatalf("down to second empty line = (%d,%d), want (3,0)", line, col)
	}
	editor.HandleInput("\x1b[B")
	if line, col := editor.GetCursor(); line != 4 || col != 10 {
		t.Fatalf("down to final line = (%d,%d), want (4,10)", line, col)
	}
}

func TestEditorPasteMarkerVerticalMovementThroughWrappedMarker(t *testing.T) {
	editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
	editor.SetText("abcdefgh")
	editor.HandleInput("\x1b[200~" + strings.TrimSuffix(strings.Repeat("line\n", 100), "\n") + "\x1b[201~")
	editor.InsertTextAtCursor("ijklmnopqr\n123456789012345678")
	editor.Render(20)

	marker := pasteMarkerPattern.FindString(editor.GetText())
	if marker == "" {
		t.Fatalf("missing paste marker in %q", editor.GetText())
	}
	markerStart := len([]rune("abcdefgh"))
	markerEnd := markerStart + len([]rune(marker))

	editor.HandleInput("\x1b[A")
	editor.HandleInput("\x01")
	for range 6 {
		editor.HandleInput("\x1b[C")
	}
	if line, col := editor.GetCursor(); line != 0 || col != 6 {
		t.Fatalf("initial cursor = (%d,%d), want (0,6)", line, col)
	}

	editor.HandleInput("\x1b[B")
	if line, col := editor.GetCursor(); line != 0 || col != markerStart {
		t.Fatalf("down to marker start = (%d,%d), want (0,%d)", line, col, markerStart)
	}
	editor.HandleInput("\x1b[B")
	if line, col := editor.GetCursor(); line != 0 || col != markerEnd {
		t.Fatalf("down to marker end = (%d,%d), want (0,%d)", line, col, markerEnd)
	}
	editor.HandleInput("\x1b[A")
	if line, col := editor.GetCursor(); line != 0 || col != markerStart {
		t.Fatalf("up to marker start = (%d,%d), want (0,%d)", line, col, markerStart)
	}
	editor.HandleInput("\x1b[A")
	if line, col := editor.GetCursor(); line != 0 || col != 6 {
		t.Fatalf("up to initial visual line = (%d,%d), want (0,6)", line, col)
	}
}

func TestEditorPasteMarkerVerticalMovementSkipsMarkerContinuationTail(t *testing.T) {
	editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
	editor.SetText("abcdefgh")
	editor.HandleInput("\x1b[200~" + strings.TrimSuffix(strings.Repeat("line\n", 100), "\n") + "\x1b[201~")
	editor.InsertTextAtCursor("ijklmnopqr\n123456789012345678")
	editor.Render(20)

	marker := pasteMarkerPattern.FindString(editor.GetText())
	if marker == "" {
		t.Fatalf("missing paste marker in %q", editor.GetText())
	}
	markerStart := len([]rune("abcdefgh"))

	editor.HandleInput("\x1b[A")
	editor.HandleInput("\x01")
	for range 3 {
		editor.HandleInput("\x1b[C")
	}
	if line, col := editor.GetCursor(); line != 0 || col != 3 {
		t.Fatalf("initial cursor = (%d,%d), want (0,3)", line, col)
	}

	editor.HandleInput("\x1b[B")
	if line, col := editor.GetCursor(); line != 0 || col != markerStart {
		t.Fatalf("down to marker start = (%d,%d), want (0,%d)", line, col, markerStart)
	}
	editor.HandleInput("\x1b[B")
	if line, col := editor.GetCursor(); line != 1 || col != 3 {
		t.Fatalf("down should skip marker tail, got (%d,%d), want (1,3)", line, col)
	}
	editor.HandleInput("\x1b[A")
	if line, col := editor.GetCursor(); line != 0 || col != markerStart {
		t.Fatalf("up to marker start = (%d,%d), want (0,%d)", line, col, markerStart)
	}
	editor.HandleInput("\x1b[A")
	if line, col := editor.GetCursor(); line != 0 || col != 3 {
		t.Fatalf("up to initial visual line = (%d,%d), want (0,3)", line, col)
	}
}

func TestEditorPasteMarkerRenderUsesMarkerAwareVisualChunks(t *testing.T) {
	editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
	editor.SetText("abcdefgh")
	editor.HandleInput("\x1b[200~" + strings.TrimSuffix(strings.Repeat("line\n", 100), "\n") + "\x1b[201~")
	editor.InsertTextAtCursor("ijklmnopqr\n123456789012345678")

	lines := editor.Render(20)
	if len(lines) < 6 {
		t.Fatalf("rendered lines = %#v, want bordered editor lines", lines)
	}
	if stripANSI(lines[0]) != strings.Repeat("─", 20) || stripANSI(lines[len(lines)-1]) != strings.Repeat("─", 20) {
		t.Fatalf("editor should render top/bottom borders: %#v", lines)
	}
	want := []string{
		"abcdefgh",
		"[paste #1 +100 ",
		"lines]ijklmnopqr",
		"123456789012345678",
	}
	for i, wantLine := range want {
		line := stripANSI(lines[i+1])
		if !strings.HasPrefix(line, wantLine) {
			t.Fatalf("line %d = %q, want prefix %q (all lines %#v)", i+1, line, wantLine, lines)
		}
		if VisibleWidth(lines[i+1]) > 20 {
			t.Fatalf("line %d overflows: width=%d line=%q", i+1, VisibleWidth(lines[i+1]), lines[i+1])
		}
	}
}

func TestEditorPasteMarkerRenderKeepsFocusedCursorWithinWidth(t *testing.T) {
	editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
	for range 35 {
		editor.HandleInput("b")
	}
	editor.HandleInput("\x1b[200~" + strings.TrimSuffix(strings.Repeat("line\n", 27), "\n") + "\x1b[201~")
	editor.InsertTextAtCursor("bbbb")
	for range 5 {
		editor.HandleInput("\x1b[D")
	}
	editor.SetFocused(true)

	lines := editor.Render(54)
	for _, line := range lines {
		if VisibleWidth(line) > 54 {
			t.Fatalf("rendered line overflows: width=%d line=%q", VisibleWidth(line), line)
		}
	}
	if !strings.Contains(strings.Join(lines, "\n"), CursorMarker) {
		t.Fatalf("focused marker render should include cursor marker: %#v", lines)
	}
}

func TestEditorRenderUsesPiBordersAndCursor(t *testing.T) {
	editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
	editor.SetText("hello")
	lines := editor.Render(12)
	if len(lines) != 3 {
		t.Fatalf("editor render lines = %#v, want top/content/bottom", lines)
	}
	if stripANSI(lines[0]) != strings.Repeat("─", 12) || stripANSI(lines[2]) != strings.Repeat("─", 12) {
		t.Fatalf("editor borders = %#v", lines)
	}
	content := stripANSI(lines[1])
	if !strings.HasPrefix(content, "hello ") {
		t.Fatalf("editor content should include fake cursor at end: %q", content)
	}
	if VisibleWidth(lines[1]) != 12 {
		t.Fatalf("content width = %d, want 12: %q", VisibleWidth(lines[1]), lines[1])
	}
}

func TestEditorRenderStylesOnlyBorderLikePi(t *testing.T) {
	editor := NewEditor(EditorTheme{
		Border: func(s string) string { return "<border>" + s + "</border>" },
	}, EditorOptions{PaddingX: 0})
	editor.SetText("hello")

	lines := editor.Render(12)
	if !strings.Contains(lines[0], "<border>") || !strings.Contains(lines[len(lines)-1], "<border>") {
		t.Fatalf("editor border should use border theme: %#v", lines)
	}
	if strings.Contains(lines[1], "<border>") {
		t.Fatalf("editor content should not inherit border theme like Pi: %#v", lines)
	}
	if !strings.Contains(stripANSI(lines[1]), "hello") {
		t.Fatalf("editor content missing text: %#v", lines)
	}
}

func TestEditorRenderPiEmptyAndExactFitCases(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
		editor.SetText("")

		lines := editor.Render(40)
		if len(lines) != 3 {
			t.Fatalf("editor render lines = %d, want 3: %#v", len(lines), stripANSILines(lines))
		}
		if stripANSI(lines[0]) != strings.Repeat("─", 40) || stripANSI(lines[2]) != strings.Repeat("─", 40) {
			t.Fatalf("editor borders = %#v", stripANSILines(lines))
		}
		if got := stripANSI(lines[1]); got != strings.Repeat(" ", 40) {
			t.Fatalf("empty content line = %q, want 40 spaces", got)
		}
		if VisibleWidth(lines[1]) != 40 {
			t.Fatalf("empty content width = %d, want 40: %q", VisibleWidth(lines[1]), lines[1])
		}
	})

	t.Run("single word fits exactly with cursor column", func(t *testing.T) {
		editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
		editor.SetText("1234567890")

		lines := editor.Render(11)
		if len(lines) != 3 {
			t.Fatalf("editor render lines = %d, want 3: %#v", len(lines), stripANSILines(lines))
		}
		content := stripANSI(lines[1])
		if !strings.Contains(content, "1234567890") {
			t.Fatalf("content should contain exact-fit word: %q", content)
		}
		if VisibleWidth(lines[1]) != 11 {
			t.Fatalf("content width = %d, want 11: %q", VisibleWidth(lines[1]), lines[1])
		}
	})

	t.Run("multiple spaces are preserved", func(t *testing.T) {
		editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
		editor.SetText("Word1   Word2    Word3")

		lines := editor.Render(40)
		if len(lines) < 3 {
			t.Fatalf("editor render lines = %#v", stripANSILines(lines))
		}
		content := strings.TrimSpace(stripANSI(lines[1]))
		if !strings.Contains(content, "Word1   Word2") {
			t.Fatalf("multiple spaces should be preserved in rendered content: %q", content)
		}
	})
}

func TestEditorRenderScrollIndicatorsLimitVisibleLines(t *testing.T) {
	editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
	editor.SetText("one\ntwo\nthree\nfour\nfive\nsix\nseven\neight")
	lines := editor.Render(18)
	if len(lines) != 7 {
		t.Fatalf("editor render line count = %d, want 7 (top + 5 visible + bottom): %#v", len(lines), lines)
	}
	if !strings.Contains(stripANSI(lines[0]), "↑ 3 more") {
		t.Fatalf("top scroll indicator missing: %#v", lines)
	}
	if strings.Contains(stripANSI(lines[len(lines)-1]), "↓") {
		t.Fatalf("bottom should have no scroll indicator at cursor end: %#v", lines)
	}
	joined := strings.Join(stripANSILines(lines), "\n")
	if strings.Contains(joined, "one") || !strings.Contains(joined, "four") || !strings.Contains(joined, "eight") {
		t.Fatalf("visible window not clamped around cursor: %#v", stripANSILines(lines))
	}
}

func TestEditorRenderWithSizeUsesTerminalRowsForVisibleLines(t *testing.T) {
	editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
	editor.SetText("one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten")
	lines := editor.RenderWithSize(18, 30)
	if len(lines) != 11 {
		t.Fatalf("editor render line count = %d, want 11 (top + 9 visible + bottom): %#v", len(lines), stripANSILines(lines))
	}
	if !strings.Contains(stripANSI(lines[0]), "↑ 1 more") {
		t.Fatalf("top scroll indicator missing for 30-row terminal: %#v", stripANSILines(lines))
	}
	if strings.Contains(stripANSI(lines[len(lines)-1]), "↓") {
		t.Fatalf("bottom should have no scroll indicator at cursor end: %#v", stripANSILines(lines))
	}

	editor.SetMaxVisibleLines(6)
	lines = editor.RenderWithSize(18, 30)
	if len(lines) != 8 {
		t.Fatalf("explicit max visible lines should override terminal rows, got %d lines: %#v", len(lines), stripANSILines(lines))
	}
}

func TestEditorRenderWideUnicodeLinesFitWidth(t *testing.T) {
	for _, tc := range []struct {
		name  string
		text  string
		width int
	}{
		{name: "emoji", text: "Hello ✅ World", width: 20},
		{name: "emoji wrap boundary", text: "0123456789✅", width: 11},
		{name: "cjk", text: "日本語テスト", width: 11},
		{name: "thai am", text: "ำabc", width: 8},
		{name: "lao am", text: "ຳabc", width: 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
			editor.SetText(tc.text)
			lines := editor.Render(tc.width)
			for i := 1; i < len(lines)-1; i++ {
				if got := VisibleWidth(lines[i]); got != tc.width {
					t.Fatalf("content line %d width = %d, want %d: %q", i, got, tc.width, lines[i])
				}
			}
		})
	}

	t.Run("CJK content split matches pi", func(t *testing.T) {
		editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
		editor.SetText("日本語テスト")

		lines := editor.Render(11)
		contentLines := stripANSILines(lines[1 : len(lines)-1])
		for i := range contentLines {
			contentLines[i] = strings.TrimSpace(contentLines[i])
		}
		want := []string{"日本語テス", "ト"}
		if strings.Join(contentLines, "\n") != strings.Join(want, "\n") {
			t.Fatalf("CJK rendered content = %#v, want %#v", contentLines, want)
		}
	})

	t.Run("wide-character cursor keeps content width", func(t *testing.T) {
		editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
		editor.SetText("A✅B")

		lines := editor.Render(20)
		contentLine := lines[1]
		if !strings.Contains(contentLine, "\x1b[7m") {
			t.Fatalf("wide-character line should include reverse-video cursor: %q", contentLine)
		}
		if got := VisibleWidth(contentLine); got != 20 {
			t.Fatalf("wide-character cursor line width = %d, want 20: %q", got, contentLine)
		}
	})
}

func TestEditorRenderCursorWrapsWithPaddingLikePi(t *testing.T) {
	const width = 10
	for _, paddingX := range []int{0, 1} {
		t.Run(fmt.Sprintf("paddingX=%d", paddingX), func(t *testing.T) {
			renderWidth := width + paddingX
			editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: paddingX})
			for _, ch := range "aaaaaaaaa" {
				editor.HandleInput(string(ch))
			}

			lines := editor.Render(renderWidth)
			contentLines := lines[1 : len(lines)-1]
			if len(contentLines) != 1 {
				t.Fatalf("content line count before wrap = %d, want 1: %#v", len(contentLines), stripANSILines(lines))
			}
			if !strings.HasSuffix(contentLines[0], "\x1b[7m \x1b[0m") {
				t.Fatalf("cursor should render at end before wrap: %#v", contentLines[0])
			}
			if got := VisibleWidth(contentLines[0]); got != renderWidth {
				t.Fatalf("content width before wrap = %d, want %d: %q", got, renderWidth, contentLines[0])
			}

			editor.HandleInput("a")
			lines = editor.Render(renderWidth)
			contentLines = lines[1 : len(lines)-1]
			if len(contentLines) != 2 {
				t.Fatalf("content line count after wrap = %d, want 2: %#v", len(contentLines), stripANSILines(lines))
			}
			for i, line := range contentLines {
				if got := VisibleWidth(line); got != renderWidth {
					t.Fatalf("content line %d width after wrap = %d, want %d: %q", i, got, renderWidth, line)
				}
			}
		})
	}
}

func TestEditorRenderLongWordsAndLeadingWhitespaceLikePi(t *testing.T) {
	t.Run("long URLs break without overflowing", func(t *testing.T) {
		editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
		editor.SetText("Check https://example.com/very/long/path/that/exceeds/width here")

		lines := editor.Render(30)
		if len(lines) < 4 {
			t.Fatalf("expected wrapped editor lines, got %#v", stripANSILines(lines))
		}
		for i := 1; i < len(lines)-1; i++ {
			if got := VisibleWidth(lines[i]); got != 30 {
				t.Fatalf("content line %d width = %d, want 30: %q", i, got, lines[i])
			}
		}
		plainLines := stripANSILines(lines[1 : len(lines)-1])
		for i := range plainLines {
			plainLines[i] = strings.TrimRight(plainLines[i], " ")
		}
		plain := strings.Join(plainLines, "")
		if !strings.Contains(plain, "https://example.com/very/long/path/that/exceeds/width") {
			t.Fatalf("wrapped URL content lost: %#v", stripANSILines(lines))
		}
	})

	t.Run("word wrap does not introduce leading whitespace before content", func(t *testing.T) {
		editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
		editor.SetText("Word1 Word2 Word3 Word4 Word5 Word6")

		lines := editor.Render(20)
		if len(lines) < 4 {
			t.Fatalf("expected wrapped editor lines, got %#v", stripANSILines(lines))
		}
		for i, line := range stripANSILines(lines[1 : len(lines)-1]) {
			trimmed := strings.TrimRight(line, " ")
			if trimmed != "" && strings.HasPrefix(trimmed, " ") {
				t.Fatalf("content line %d starts with whitespace: %q", i, line)
			}
		}
	})

	t.Run("multiple spaces are preserved on the same rendered line", func(t *testing.T) {
		editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 0})
		editor.SetText("Word1   Word2    Word3")

		lines := editor.Render(50)
		contentLine := strings.TrimSpace(stripANSI(lines[1]))
		if !strings.Contains(contentLine, "Word1   Word2") || !strings.Contains(contentLine, "Word2    Word3") {
			t.Fatalf("multiple spaces should be preserved, got %q", contentLine)
		}
	})
}

func stripANSILines(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = stripANSI(line)
	}
	return out
}

func TestEditorAutocompleteTabAppliesSingleSuggestionAndUndo(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetText("he")
	editor.SetAutocompleteProvider(AutocompleteProviderFunc(func(text string, cursor int) AutocompleteSuggestions {
		return AutocompleteSuggestions{
			Items:  []AutocompleteItem{{Value: "hello", Label: "hello"}},
			Start:  0,
			End:    cursor,
			Prefix: text[:cursor],
		}
	}))

	editor.HandleInput("\t")
	if editor.GetText() != "hello" {
		t.Fatalf("completion text = %q", editor.GetText())
	}
	if editor.IsShowingAutocomplete() {
		t.Fatalf("single completion should auto-apply without leaving menu open")
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "he" {
		t.Fatalf("undo autocomplete = %q", editor.GetText())
	}
}

func TestEditorAutocompleteCombinedProviderUsesChildProviders(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetText("he")
	changed := make(chan struct{}, 1)
	editor.OnChange = func(string) {
		select {
		case changed <- struct{}{}:
		default:
		}
	}
	editor.SetAutocompleteProvider(NewCombinedAutocompleteProviderWithCommands(t.TempDir(), nil, AutocompleteProviderFunc(func(text string, cursor int) AutocompleteSuggestions {
		if text != "he" || cursor != len([]rune("he")) {
			t.Fatalf("child provider saw text=%q cursor=%d", text, cursor)
		}
		return AutocompleteSuggestions{
			Items:  []AutocompleteItem{{Value: "hello", Label: "hello"}},
			Prefix: "he",
			Start:  0,
			End:    cursor,
		}
	})))

	editor.HandleInput("\t")
	waitForAutocompleteChange(t, changed)
	if got := editor.GetText(); got != "hello" {
		t.Fatalf("combined child provider completion text = %q, want hello", got)
	}
	if editor.IsShowingAutocomplete() {
		t.Fatalf("single child-provider completion should auto-apply without leaving menu open")
	}
}

func TestEditorAutocompleteForceFileSingleSuggestionAppliesWithoutMenu(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetText("Work")
	editor.SetAutocompleteProvider(testFullAutocompleteProvider{get: func(lines []string, _ int, cursorCol int, force bool) (*AutocompleteSuggestions, error) {
		if !force || len(lines) == 0 {
			return nil, nil
		}
		prefix := lines[0][:cursorCol]
		if prefix != "Work" {
			return nil, nil
		}
		return &AutocompleteSuggestions{
			Items:  []AutocompleteItem{{Value: "Workspace/", Label: "Workspace/"}},
			Prefix: prefix,
			Start:  0,
			End:    cursorCol,
		}, nil
	}})

	editor.HandleInput("\t")
	if got := editor.GetText(); got != "Workspace/" {
		t.Fatalf("force single completion text = %q, want Workspace/", got)
	}
	if editor.IsShowingAutocomplete() {
		t.Fatalf("force single completion should not leave menu open")
	}
	editor.HandleInput("\x1b[45;5u")
	if got := editor.GetText(); got != "Work" {
		t.Fatalf("undo force completion = %q, want Work", got)
	}
}

func TestEditorAutocompleteMenuSelectionAndRender(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetText("h")
	editor.SetAutocompleteProvider(AutocompleteProviderFunc(func(text string, cursor int) AutocompleteSuggestions {
		return AutocompleteSuggestions{
			Items: []AutocompleteItem{
				{Value: "hello", Label: "hello", Description: "greeting"},
				{Value: "help", Label: "help", Description: "command"},
			},
			Start:  0,
			End:    cursor,
			Prefix: text[:cursor],
		}
	}))

	editor.HandleInput("\t")
	if !editor.IsShowingAutocomplete() {
		t.Fatalf("multiple completions should show menu")
	}
	if rendered := strings.Join(editor.Render(80), "\n"); !strings.Contains(rendered, "hello") || !strings.Contains(rendered, "help") {
		t.Fatalf("autocomplete render = %q", rendered)
	}
	editor.HandleInput("\x1b[B")
	editor.HandleInput("\t")
	if editor.GetText() != "help" {
		t.Fatalf("selected completion = %q", editor.GetText())
	}
}

func TestEditorAutocompleteRenderScrollWindowBelowBorderLikePi(t *testing.T) {
	editor := NewEditor(EditorTheme{}, EditorOptions{PaddingX: 1, AutocompleteMaxVisible: 3})
	editor.SetText("a")
	editor.SetAutocompleteProvider(AutocompleteProviderFunc(func(text string, cursor int) AutocompleteSuggestions {
		return AutocompleteSuggestions{
			Items: []AutocompleteItem{
				{Value: "alpha", Label: "alpha"},
				{Value: "alps", Label: "alps"},
				{Value: "alpine", Label: "alpine"},
				{Value: "alto", Label: "alto"},
				{Value: "almond", Label: "almond"},
			},
			Start:  0,
			End:    cursor,
			Prefix: text[:cursor],
		}
	}))

	editor.HandleInput("\t")
	lines := stripANSILines(editor.Render(20))
	wantInitial := []string{
		strings.Repeat("─", 20),
		" a                  ",
		strings.Repeat("─", 20),
		" → alpha            ",
		"   alps             ",
		"   alpine           ",
		"   (1/5)            ",
	}
	if strings.Join(lines, "\n") != strings.Join(wantInitial, "\n") {
		t.Fatalf("initial autocomplete render = %#v, want %#v", lines, wantInitial)
	}

	for range 4 {
		editor.HandleInput("\x1b[B")
	}
	lines = stripANSILines(editor.Render(20))
	wantScrolled := []string{
		strings.Repeat("─", 20),
		" a                  ",
		strings.Repeat("─", 20),
		"   alpine           ",
		"   alto             ",
		" → almond           ",
		"   (5/5)            ",
	}
	if strings.Join(lines, "\n") != strings.Join(wantScrolled, "\n") {
		t.Fatalf("scrolled autocomplete render = %#v, want %#v", lines, wantScrolled)
	}
}

func TestEditorRenderSuppressesHardwareCursorMarkerWhileAutocompleteOpen(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetFocused(true)
	editor.SetText("h")
	if rendered := strings.Join(editor.Render(20), "\n"); !strings.Contains(rendered, CursorMarker) {
		t.Fatalf("focused editor without autocomplete should include cursor marker: %#v", rendered)
	}

	editor.SetAutocompleteProvider(AutocompleteProviderFunc(func(text string, cursor int) AutocompleteSuggestions {
		return AutocompleteSuggestions{
			Items: []AutocompleteItem{
				{Value: "hello", Label: "hello"},
				{Value: "help", Label: "help"},
			},
			Start:  0,
			End:    cursor,
			Prefix: text[:cursor],
		}
	}))
	editor.HandleInput("\t")
	if rendered := strings.Join(editor.Render(20), "\n"); strings.Contains(rendered, CursorMarker) {
		t.Fatalf("focused editor should suppress hardware cursor marker while autocomplete is visible: %#v", rendered)
	}
}

func TestEditorAutocompleteKeepsForceFileSuggestionsOpenWhileTyping(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	allFiles := []AutocompleteItem{
		{Value: "readme.md", Label: "readme.md"},
		{Value: "package.json", Label: "package.json"},
		{Value: "src/", Label: "src/"},
		{Value: "dist/", Label: "dist/"},
	}
	editor.SetAutocompleteProvider(testFullAutocompleteProvider{get: func(lines []string, _ int, cursorCol int, force bool) (*AutocompleteSuggestions, error) {
		prefix := ""
		if len(lines) > 0 {
			prefix = string([]rune(lines[0])[:min(cursorCol, len([]rune(lines[0])))])
		}
		if !force && !strings.Contains(prefix, "/") && !strings.HasPrefix(prefix, ".") {
			return nil, nil
		}
		var filtered []AutocompleteItem
		for _, item := range allFiles {
			if strings.HasPrefix(strings.ToLower(item.Value), strings.ToLower(prefix)) {
				filtered = append(filtered, item)
			}
		}
		if len(filtered) == 0 {
			return nil, nil
		}
		return &AutocompleteSuggestions{Items: filtered, Prefix: prefix, Start: 0, End: cursorCol}, nil
	}})

	editor.HandleInput("\t")
	if !editor.IsShowingAutocomplete() {
		t.Fatalf("tab force completion should show all file suggestions")
	}
	editor.HandleInput("r")
	if editor.GetText() != "r" || !editor.IsShowingAutocomplete() {
		t.Fatalf("force completion should stay open after typing r, text=%q showing=%v", editor.GetText(), editor.IsShowingAutocomplete())
	}
	editor.HandleInput("e")
	if editor.GetText() != "re" || !editor.IsShowingAutocomplete() {
		t.Fatalf("force completion should stay open after typing e, text=%q showing=%v", editor.GetText(), editor.IsShowingAutocomplete())
	}
	editor.HandleInput("\t")
	if editor.GetText() != "readme.md" || editor.IsShowingAutocomplete() {
		t.Fatalf("tab should apply narrowed force completion, text=%q showing=%v", editor.GetText(), editor.IsShowingAutocomplete())
	}
}

func TestEditorSlashAutocompleteUsesPiSelectListLayoutAndTheme(t *testing.T) {
	editor := NewEditor(EditorTheme{
		SelectList: SelectListTheme{
			Description: func(text string) string { return "<" + text + ">" },
		},
	})
	editor.SetText("/")
	editor.SetAutocompleteProvider(AutocompleteProviderFunc(func(text string, cursor int) AutocompleteSuggestions {
		return AutocompleteSuggestions{
			Items: []AutocompleteItem{
				{Value: "/a", Label: "/a", Description: "first"},
				{Value: "/bb", Label: "/bb", Description: "second"},
			},
			Start:  0,
			End:    cursor,
			Prefix: text[:cursor],
		}
	}))

	editor.HandleInput("\t")
	if !editor.IsShowingAutocomplete() {
		t.Fatalf("slash autocomplete menu should be visible")
	}
	var firstLine, secondLine string
	for _, line := range editor.Render(80) {
		if strings.Contains(line, "first") {
			firstLine = line
		}
		if strings.Contains(line, "second") {
			secondLine = line
		}
	}
	if firstLine == "" || secondLine == "" {
		t.Fatalf("rendered autocomplete missing items")
	}
	if got := visibleIndexOf(t, firstLine, "first"); got != 14 {
		t.Fatalf("slash autocomplete description column = %d, want 14: %q", got, firstLine)
	}
	if !strings.Contains(secondLine, "<") {
		t.Fatalf("editor select-list theme should style unselected description: %q", secondLine)
	}
}

func TestEditorSlashCommandAutocompleteEnterSubmits(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	changed := make(chan struct{}, 1)
	editor.OnAutocompleteChange = func() { changed <- struct{}{} }
	editor.SetAutocompleteProvider(NewCombinedAutocompleteProviderWithCommands(t.TempDir(), []SlashCommand{
		{Name: "model", Description: "Change model"},
		{Name: "help", Description: "Show help"},
	}))
	submitted := ""
	editor.OnSubmit = func(text string) { submitted = text }

	editor.HandleInput("/")
	waitForAutocompleteChange(t, changed)
	if !editor.IsShowingAutocomplete() {
		t.Fatalf("slash command autocomplete should be showing")
	}
	editor.HandleInput("\r")
	if submitted != "/model" {
		t.Fatalf("submitted slash command = %q, want /model", submitted)
	}
	if editor.GetText() != "" || editor.IsShowingAutocomplete() {
		t.Fatalf("submit should clear editor and autocomplete, text=%q showing=%v", editor.GetText(), editor.IsShowingAutocomplete())
	}
	editor.HandleInput("\x1b[45;5u")
	if editor.GetText() != "" {
		t.Fatalf("undo after slash submit should be no-op, got %q", editor.GetText())
	}
}

type testFullAutocompleteProvider struct {
	get func(lines []string, cursorLine, cursorCol int, force bool) (*AutocompleteSuggestions, error)
}

func (p testFullAutocompleteProvider) Suggestions(string, int) AutocompleteSuggestions {
	return AutocompleteSuggestions{}
}

func (p testFullAutocompleteProvider) GetSuggestions(lines []string, cursorLine, cursorCol int, force bool) (*AutocompleteSuggestions, error) {
	return p.get(lines, cursorLine, cursorCol, force)
}

func (p testFullAutocompleteProvider) ApplyCompletion(lines []string, cursorLine, cursorCol int, item AutocompleteItem, prefix string) CompletionResult {
	next := append([]string(nil), lines...)
	line := next[cursorLine]
	before := line[:max(0, cursorCol-len(prefix))]
	after := line[cursorCol:]
	next[cursorLine] = before + item.Value + after
	return CompletionResult{Lines: next, CursorLine: cursorLine, CursorCol: len(before) + len(item.Value)}
}

type testContextAutocompleteProvider struct {
	get func(ctx context.Context, lines []string, cursorLine, cursorCol int, force bool) (*AutocompleteSuggestions, error)
}

func (p testContextAutocompleteProvider) Suggestions(string, int) AutocompleteSuggestions {
	return AutocompleteSuggestions{}
}

func (p testContextAutocompleteProvider) GetSuggestionsContext(ctx context.Context, lines []string, cursorLine, cursorCol int, force bool) (*AutocompleteSuggestions, error) {
	return p.get(ctx, lines, cursorLine, cursorCol, force)
}

func (p testContextAutocompleteProvider) ApplyCompletion(lines []string, cursorLine, cursorCol int, item AutocompleteItem, prefix string) CompletionResult {
	return testFullAutocompleteProvider{}.ApplyCompletion(lines, cursorLine, cursorCol, item, prefix)
}

func waitForAutocompleteChange(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for autocomplete update")
	}
}

func TestEditorAutocompleteAutoTriggerAndPasteSuppression(t *testing.T) {
	defaultEditor := NewEditor(EditorTheme{})
	if defaultEditor.options.AutocompleteDebounce != 20*time.Millisecond {
		t.Fatalf("default autocomplete debounce = %s, want Pi's 20ms", defaultEditor.options.AutocompleteDebounce)
	}

	editor := NewEditor(EditorTheme{}, EditorOptions{AutocompleteMaxVisible: 5, AutocompleteDebounce: 20 * time.Millisecond})
	changed := make(chan struct{}, 1)
	editor.OnAutocompleteChange = func() { changed <- struct{}{} }
	var calls atomic.Int32
	editor.SetAutocompleteProvider(AutocompleteProviderFunc(func(text string, cursor int) AutocompleteSuggestions {
		calls.Add(1)
		return AutocompleteSuggestions{
			Items:  []AutocompleteItem{{Value: "@file.go", Label: "file.go"}},
			Start:  0,
			End:    cursor,
			Prefix: text[:cursor],
		}
	}))
	editor.HandleInput("@")
	if editor.IsShowingAutocomplete() || calls.Load() != 0 {
		t.Fatalf("@ autocomplete should be debounced, showing=%v calls=%d", editor.IsShowingAutocomplete(), calls.Load())
	}
	waitForAutocompleteChange(t, changed)
	if !editor.IsShowingAutocomplete() || calls.Load() != 1 {
		t.Fatalf("@ should trigger autocomplete after debounce, showing=%v calls=%d", editor.IsShowingAutocomplete(), calls.Load())
	}

	editor = NewEditor(EditorTheme{})
	calls.Store(0)
	editor.SetAutocompleteProvider(AutocompleteProviderFunc(func(text string, cursor int) AutocompleteSuggestions {
		calls.Add(1)
		return AutocompleteSuggestions{Items: []AutocompleteItem{{Value: "@file.go", Label: "file.go"}}, Start: 0, End: cursor}
	}))
	editor.HandleInput("\x1b[200~look at @file.go\x1b[201~")
	if calls.Load() != 0 || editor.IsShowingAutocomplete() {
		t.Fatalf("paste should not trigger autocomplete, calls=%d showing=%v", calls.Load(), editor.IsShowingAutocomplete())
	}

	editor = NewEditor(EditorTheme{}, EditorOptions{AutocompleteMaxVisible: 5, AutocompleteDebounce: 20 * time.Millisecond})
	changed = make(chan struct{}, 1)
	editor.OnAutocompleteChange = func() { changed <- struct{}{} }
	calls.Store(0)
	seenPrefix := ""
	editor.SetAutocompleteProvider(AutocompleteProviderFunc(func(text string, cursor int) AutocompleteSuggestions {
		calls.Add(1)
		seenPrefix = text[:cursor]
		return AutocompleteSuggestions{
			Items:  []AutocompleteItem{{Value: "@main.go", Label: "main.go"}},
			Start:  0,
			End:    cursor,
			Prefix: text[:cursor],
		}
	}))
	editor.HandleInput("@ma")
	if editor.GetText() != "@ma" || calls.Load() != 0 || editor.IsShowingAutocomplete() {
		t.Fatalf("batched @ input should debounce autocomplete, text=%q calls=%d showing=%v", editor.GetText(), calls.Load(), editor.IsShowingAutocomplete())
	}
	waitForAutocompleteChange(t, changed)
	if !editor.IsShowingAutocomplete() || calls.Load() != 1 || seenPrefix != "@ma" {
		t.Fatalf("batched @ autocomplete = showing %v calls %d prefix %q", editor.IsShowingAutocomplete(), calls.Load(), seenPrefix)
	}

	editor = NewEditor(EditorTheme{}, EditorOptions{AutocompleteMaxVisible: 5, AutocompleteDebounce: 20 * time.Millisecond})
	changed = make(chan struct{}, 1)
	editor.OnAutocompleteChange = func() { changed <- struct{}{} }
	calls.Store(0)
	seenPrefix = ""
	editor.SetAutocompleteProvider(AutocompleteProviderFunc(func(text string, cursor int) AutocompleteSuggestions {
		calls.Add(1)
		seenPrefix = text[:cursor]
		return AutocompleteSuggestions{
			Items:  []AutocompleteItem{{Value: "#2983", Label: "#2983"}},
			Start:  0,
			End:    cursor,
			Prefix: text[:cursor],
		}
	}))
	editor.HandleInput("#298")
	if editor.GetText() != "#298" || calls.Load() != 0 || editor.IsShowingAutocomplete() {
		t.Fatalf("batched # input should debounce autocomplete, text=%q calls=%d showing=%v", editor.GetText(), calls.Load(), editor.IsShowingAutocomplete())
	}
	waitForAutocompleteChange(t, changed)
	if !editor.IsShowingAutocomplete() || calls.Load() != 1 || seenPrefix != "#298" {
		t.Fatalf("batched # autocomplete = showing %v calls %d prefix %q", editor.IsShowingAutocomplete(), calls.Load(), seenPrefix)
	}
}

func TestEditorAutocompleteSymbolContextMatchesPi(t *testing.T) {
	cases := []struct {
		before string
		want   bool
	}{
		{before: "@main", want: true},
		{before: "see @main", want: true},
		{before: "see\t#2983", want: true},
		{before: "line\n@main", want: true},
		{before: "see @\"foo bar", want: true},
		{before: "see @\"foo bar\"", want: false},
		{before: "path=@main", want: false},
		{before: "\"@main", want: false},
		{before: "email@example.com", want: false},
		{before: "see ", want: false},
	}
	for _, tc := range cases {
		if got := symbolAutocompleteContext(tc.before); got != tc.want {
			t.Fatalf("symbolAutocompleteContext(%q) = %v, want %v", tc.before, got, tc.want)
		}
	}
}

func TestEditorAutocompleteHidesWhenSlashBackspacedToEmpty(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetAutocompleteProvider(testFullAutocompleteProvider{get: func(lines []string, _ int, cursorCol int, _ bool) (*AutocompleteSuggestions, error) {
		if len(lines) == 0 || cursorCol < 0 {
			return nil, nil
		}
		before := string([]rune(lines[0])[:min(cursorCol, len([]rune(lines[0])))])
		if !strings.HasPrefix(before, "/") {
			return nil, nil
		}
		return &AutocompleteSuggestions{
			Items: []AutocompleteItem{
				{Value: "/model", Label: "model", Description: "Change model"},
				{Value: "/help", Label: "help", Description: "Show help"},
			},
			Prefix: before,
			Start:  0,
			End:    cursorCol,
		}, nil
	}})

	editor.HandleInput("/")
	if editor.GetText() != "/" || !editor.IsShowingAutocomplete() {
		t.Fatalf("slash should show autocomplete, text=%q showing=%v", editor.GetText(), editor.IsShowingAutocomplete())
	}
	editor.HandleInput("\x7f")
	if editor.GetText() != "" || editor.IsShowingAutocomplete() {
		t.Fatalf("backspacing slash should hide autocomplete, text=%q showing=%v", editor.GetText(), editor.IsShowingAutocomplete())
	}
}

func TestEditorAutocompleteClearsOnProviderError(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetAutocompleteProvider(testFullAutocompleteProvider{get: func(lines []string, _ int, cursorCol int, _ bool) (*AutocompleteSuggestions, error) {
		prefix := lines[0][:cursorCol]
		if prefix == "/" {
			return &AutocompleteSuggestions{
				Items:  []AutocompleteItem{{Value: "help", Label: "help"}},
				Prefix: prefix,
				Start:  0,
				End:    cursorCol,
			}, nil
		}
		return nil, fmt.Errorf("autocomplete failed")
	}})

	editor.HandleInput("/")
	if !editor.IsShowingAutocomplete() {
		t.Fatalf("slash should show autocomplete")
	}
	editor.HandleInput("h")
	if editor.GetText() != "/h" || editor.IsShowingAutocomplete() {
		t.Fatalf("provider error should clear autocomplete, text=%q showing=%v", editor.GetText(), editor.IsShowingAutocomplete())
	}
}

func TestEditorAutocompleteDebouncesAndAbortsContextRequests(t *testing.T) {
	editor := NewEditor(EditorTheme{}, EditorOptions{AutocompleteMaxVisible: 5, AutocompleteDebounce: 20 * time.Millisecond})
	changed := make(chan struct{}, 1)
	editor.OnAutocompleteChange = func() { changed <- struct{}{} }
	var calls atomic.Int32
	var aborts atomic.Int32
	editor.SetAutocompleteProvider(testContextAutocompleteProvider{get: func(ctx context.Context, lines []string, _ int, cursorCol int, _ bool) (*AutocompleteSuggestions, error) {
		calls.Add(1)
		select {
		case <-ctx.Done():
			aborts.Add(1)
			return nil, ctx.Err()
		case <-time.After(150 * time.Millisecond):
			prefix := lines[0][:cursorCol]
			return &AutocompleteSuggestions{Items: []AutocompleteItem{{Value: "@main.ts", Label: "main.ts"}}, Prefix: prefix, Start: 0, End: cursorCol}, nil
		}
	}})

	editor.HandleInput("@")
	editor.HandleInput("m")
	editor.HandleInput("a")
	editor.HandleInput("i")
	if calls.Load() != 0 || editor.IsShowingAutocomplete() {
		t.Fatalf("typing should debounce context autocomplete, calls=%d showing=%v", calls.Load(), editor.IsShowingAutocomplete())
	}
	time.Sleep(60 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("debounced context request calls = %d, want 1", calls.Load())
	}
	editor.HandleInput("n")
	deadline := time.After(500 * time.Millisecond)
	for aborts.Load() == 0 {
		select {
		case <-deadline:
			t.Fatalf("expected active autocomplete request to abort")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	waitForAutocompleteChange(t, changed)
	if !editor.IsShowingAutocomplete() {
		t.Fatalf("latest autocomplete request should eventually show suggestions")
	}
}

func TestEditorAutocompleteSelectsBestPrefixMatch(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetAutocompleteProvider(testFullAutocompleteProvider{get: func(lines []string, _ int, cursorCol int, _ bool) (*AutocompleteSuggestions, error) {
		before := lines[0][:cursorCol]
		if !strings.HasPrefix(before, "/argtest ") {
			return nil, nil
		}
		prefix := strings.TrimPrefix(before, "/argtest ")
		return &AutocompleteSuggestions{
			Items: []AutocompleteItem{
				{Value: "one", Label: "one"},
				{Value: "two", Label: "two"},
				{Value: "three", Label: "three"},
			},
			Prefix: prefix,
			Start:  cursorCol - len(prefix),
			End:    cursorCol,
		}, nil
	}})

	for _, r := range "/argtest tw" {
		editor.HandleInput(string(r))
	}
	if !editor.IsShowingAutocomplete() {
		t.Fatalf("autocomplete should be showing")
	}
	editor.HandleInput("\r")
	if got := editor.GetText(); got != "/argtest two" {
		t.Fatalf("best prefix completion = %q, want /argtest two", got)
	}
}

func TestEditorAutocompleteSelectsFirstPrefixMatchWhenMultipleMatch(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetAutocompleteProvider(testFullAutocompleteProvider{get: func(lines []string, _ int, cursorCol int, _ bool) (*AutocompleteSuggestions, error) {
		before := lines[0][:cursorCol]
		if !strings.HasPrefix(before, "/argtest ") {
			return nil, nil
		}
		prefix := strings.TrimPrefix(before, "/argtest ")
		return &AutocompleteSuggestions{
			Items: []AutocompleteItem{
				{Value: "one", Label: "one"},
				{Value: "two", Label: "two"},
				{Value: "three", Label: "three"},
			},
			Prefix: prefix,
			Start:  cursorCol - len(prefix),
			End:    cursorCol,
		}, nil
	}})

	for _, r := range "/argtest t" {
		editor.HandleInput(string(r))
	}
	if !editor.IsShowingAutocomplete() {
		t.Fatalf("autocomplete should be showing")
	}
	editor.HandleInput("\r")
	if got := editor.GetText(); got != "/argtest two" {
		t.Fatalf("first prefix completion = %q, want /argtest two", got)
	}
}

func TestEditorAutocompleteRetainsExactTypedSlashArgument(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	editor.SetAutocompleteProvider(testFullAutocompleteProvider{get: func(lines []string, _ int, cursorCol int, _ bool) (*AutocompleteSuggestions, error) {
		before := lines[0][:cursorCol]
		if !strings.HasPrefix(before, "/model ") {
			return nil, nil
		}
		prefix := strings.TrimPrefix(before, "/model ")
		return &AutocompleteSuggestions{
			Items: []AutocompleteItem{
				{Value: "gpt-4o", Label: "gpt-4o"},
				{Value: "gpt-4o-mini", Label: "gpt-4o-mini"},
				{Value: "claude-sonnet", Label: "claude-sonnet"},
			},
			Prefix: prefix,
			Start:  cursorCol - len(prefix),
			End:    cursorCol,
		}, nil
	}})

	for _, r := range "/model gpt-4o-mini" {
		editor.HandleInput(string(r))
	}
	if !editor.IsShowingAutocomplete() {
		t.Fatalf("autocomplete should be showing for exact slash argument")
	}
	editor.HandleInput("\r")
	if got := editor.GetText(); got != "/model gpt-4o-mini" {
		t.Fatalf("exact slash argument should be retained, got %q", got)
	}
}

func TestEditorAutocompleteCombinedProviderRetainsExactSlashArgument(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	changed := make(chan struct{}, 1)
	editor.OnAutocompleteChange = func() { changed <- struct{}{} }
	provider := NewCombinedAutocompleteProviderWithCommands(t.TempDir(), []SlashCommand{{
		Name: "model",
		GetArgumentCompletions: func(prefix string) []AutocompleteItem {
			all := []AutocompleteItem{
				{Value: "gpt-4o", Label: "gpt-4o"},
				{Value: "gpt-4o-mini", Label: "gpt-4o-mini"},
				{Value: "claude-sonnet", Label: "claude-sonnet"},
			}
			var out []AutocompleteItem
			for _, item := range all {
				if strings.HasPrefix(item.Value, prefix) {
					out = append(out, item)
				}
			}
			return out
		},
	}})
	editor.SetAutocompleteProvider(provider)

	for _, r := range "/model gpt-4o-mini" {
		editor.HandleInput(string(r))
	}
	waitForAutocompleteChange(t, changed)
	if !editor.IsShowingAutocomplete() {
		t.Fatalf("combined provider should show slash argument completions")
	}
	editor.HandleInput("\r")
	if got := editor.GetText(); got != "/model gpt-4o-mini" {
		t.Fatalf("combined provider exact slash argument = %q, want /model gpt-4o-mini", got)
	}
}

func TestEditorAutocompleteAwaitsAsyncSlashArgumentCompletions(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	changed := make(chan struct{}, 1)
	editor.OnAutocompleteChange = func() { changed <- struct{}{} }
	provider := NewCombinedAutocompleteProviderWithCommands(t.TempDir(), []SlashCommand{{
		Name: "load-skills",
		GetArgumentCompletionsContext: func(ctx context.Context, prefix string) ([]AutocompleteItem, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(20 * time.Millisecond):
			}
			if strings.HasPrefix(prefix, "s") {
				return []AutocompleteItem{{Value: "skill-a", Label: "skill-a"}}, nil
			}
			return nil, nil
		},
	}})
	editor.SetAutocompleteProvider(provider)
	editor.SetText("/load-skills ")

	editor.HandleInput("s")
	waitForAutocompleteChange(t, changed)
	if !editor.IsShowingAutocomplete() {
		t.Fatalf("async slash argument completion should show autocomplete")
	}
	editor.HandleInput("\t")
	if got := editor.GetText(); got != "/load-skills skill-a" {
		t.Fatalf("async slash argument completion text = %q", got)
	}
}

func TestEditorAutocompleteSkipsArgumentsWithoutCompleter(t *testing.T) {
	editor := NewEditor(EditorTheme{})
	changed := make(chan struct{}, 4)
	editor.OnAutocompleteChange = func() { changed <- struct{}{} }
	provider := NewCombinedAutocompleteProviderWithCommands(t.TempDir(), []SlashCommand{
		{Name: "help", Description: "Show help"},
		{Name: "model", Description: "Switch model", GetArgumentCompletions: func(string) []AutocompleteItem {
			return []AutocompleteItem{{Value: "claude-opus", Label: "claude-opus"}}
		}},
	})
	editor.SetAutocompleteProvider(provider)

	for _, r := range "/he" {
		editor.HandleInput(string(r))
	}
	waitForAutocompleteChange(t, changed)
	if !editor.IsShowingAutocomplete() {
		t.Fatalf("slash command name completion should show autocomplete")
	}
	editor.HandleInput("\t")
	if got := editor.GetText(); got != "/help " {
		t.Fatalf("slash command completion text = %q, want /help space", got)
	}
	if editor.IsShowingAutocomplete() {
		t.Fatalf("command without argument completer should not keep autocomplete open")
	}

	editor.HandleInput("x")
	select {
	case <-changed:
		if editor.IsShowingAutocomplete() {
			t.Fatalf("typing argument for command without completer should not show autocomplete")
		}
	case <-time.After(40 * time.Millisecond):
	}
	if got := editor.GetText(); got != "/help x" {
		t.Fatalf("typed argument should remain literal, got %q", got)
	}
}

func TestWordWrapLine(t *testing.T) {
	chunks := WordWrapLine("alpha beta gamma", 8)
	if len(chunks) != 3 || chunks[0].Text != "alpha " || chunks[1].Text != "beta " || chunks[2].Text != "gamma" {
		t.Fatalf("chunks = %#v", chunks)
	}
}

func TestWordWrapLinePiBoundaryCases(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		width int
		want  []string
	}{
		{
			name:  "wraps word when it ends exactly at width",
			line:  "hello world test",
			width: 11,
			want:  []string{"hello ", "world test"},
		},
		{
			name:  "keeps whitespace at width boundary",
			line:  "hello world test",
			width: 12,
			want:  []string{"hello world ", "test"},
		},
		{
			name:  "keeps leading space after exact unbreakable word",
			line:  "aaaaaaaaaaaa aaaa",
			width: 12,
			want:  []string{"aaaaaaaaaaaa", " aaaa"},
		},
		{
			name:  "wraps word that fits width but not remaining space",
			line:  "      aaaaaaaaaaaa",
			width: 12,
			want:  []string{"      ", "aaaaaaaaaaaa"},
		},
		{
			name:  "keeps multi-space and following word together when they fit",
			line:  "Lorem ipsum dolor sit amet,    consectetur",
			width: 30,
			want:  []string{"Lorem ipsum dolor sit ", "amet,    consectetur"},
		},
		{
			name:  "keeps multi-space and following word when they fill width exactly",
			line:  "Lorem ipsum dolor sit amet,              consectetur",
			width: 30,
			want:  []string{"Lorem ipsum dolor sit ", "amet,              consectetur"},
		},
		{
			name:  "splits when multi-space group makes next word overflow",
			line:  "Lorem ipsum dolor sit amet,               consectetur",
			width: 30,
			want:  []string{"Lorem ipsum dolor sit ", "amet,               ", "consectetur"},
		},
		{
			name:  "breaks long whitespace at line boundary",
			line:  "Lorem ipsum dolor sit amet,                         consectetur",
			width: 30,
			want:  []string{"Lorem ipsum dolor sit ", "amet,                         ", "consectetur"},
		},
		{
			name:  "preserves overflowed leading whitespace on next chunk",
			line:  "Lorem ipsum dolor sit amet,                          consectetur",
			width: 30,
			want:  []string{"Lorem ipsum dolor sit ", "amet,                         ", " consectetur"},
		},
		{
			name:  "breaks whitespace spanning full lines",
			line:  "Lorem ipsum dolor sit amet,                                     consectetur",
			width: 30,
			want:  []string{"Lorem ipsum dolor sit ", "amet,                         ", "            consectetur"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunks := WordWrapLine(tc.line, tc.width)
			got := make([]string, len(chunks))
			for i, chunk := range chunks {
				got[i] = chunk.Text
				if VisibleWidth(chunk.Text) > tc.width {
					t.Fatalf("chunk %q width=%d exceeds %d", chunk.Text, VisibleWidth(chunk.Text), tc.width)
				}
			}
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Fatalf("chunks = %#v, want %#v", got, tc.want)
			}
			reconstructed := ""
			for _, chunk := range chunks {
				reconstructed += tc.line[chunk.StartIndex:chunk.EndIndex]
			}
			if reconstructed != tc.line {
				t.Fatalf("reconstructed = %q, want original %q", reconstructed, tc.line)
			}
		})
	}
}

func TestWordWrapLineWideCharAfterWrapOpportunity(t *testing.T) {
	line := " " + strings.Repeat("a", 186) + "你"
	chunks := WordWrapLine(line, 187)
	reconstructed := ""
	for _, chunk := range chunks {
		if VisibleWidth(chunk.Text) > 187 {
			t.Fatalf("chunk width=%d exceeds limit: %q", VisibleWidth(chunk.Text), chunk.Text)
		}
		reconstructed += line[chunk.StartIndex:chunk.EndIndex]
	}
	if reconstructed != line {
		t.Fatalf("reconstructed = %q, want original", reconstructed)
	}
}

func TestWordWrapLineSplitsOversizedAtomicSegmentsLikePi(t *testing.T) {
	cases := []struct {
		name         string
		line         string
		segments     []wrapSegment
		wantFirst    string
		wantLast     string
		wantLastHasB bool
	}{
		{
			name: "middle marker",
			line: "A[paste #1 +20 lines]B",
			segments: []wrapSegment{
				{text: "A", index: 0},
				{text: "[paste #1 +20 lines]", index: 1},
				{text: "B", index: len("A[paste #1 +20 lines]")},
			},
			wantFirst:    "A",
			wantLastHasB: true,
		},
		{
			name: "marker at start",
			line: "[paste #1 +20 lines]B",
			segments: []wrapSegment{
				{text: "[paste #1 +20 lines]", index: 0},
				{text: "B", index: len("[paste #1 +20 lines]")},
			},
			wantLastHasB: true,
		},
		{
			name: "marker at end",
			line: "A[paste #1 +20 lines]",
			segments: []wrapSegment{
				{text: "A", index: 0},
				{text: "[paste #1 +20 lines]", index: 1},
			},
			wantFirst: "A",
		},
		{
			name: "consecutive markers",
			line: "[paste #1 +20 lines][paste #2 +30 lines]",
			segments: []wrapSegment{
				{text: "[paste #1 +20 lines]", index: 0},
				{text: "[paste #2 +30 lines]", index: len("[paste #1 +20 lines]")},
			},
		},
		{
			name: "normal wrapping resumes after marker",
			line: "[paste #1 +20 lines] hello world",
			segments: []wrapSegment{
				{text: "[paste #1 +20 lines]", index: 0},
				{text: " ", index: len("[paste #1 +20 lines]")},
				{text: "h", index: len("[paste #1 +20 lines] ")},
				{text: "e", index: len("[paste #1 +20 lines] h")},
				{text: "l", index: len("[paste #1 +20 lines] he")},
				{text: "l", index: len("[paste #1 +20 lines] hel")},
				{text: "o", index: len("[paste #1 +20 lines] hell")},
				{text: " ", index: len("[paste #1 +20 lines] hello")},
				{text: "w", index: len("[paste #1 +20 lines] hello ")},
				{text: "o", index: len("[paste #1 +20 lines] hello w")},
				{text: "r", index: len("[paste #1 +20 lines] hello wo")},
				{text: "l", index: len("[paste #1 +20 lines] hello wor")},
				{text: "d", index: len("[paste #1 +20 lines] hello worl")},
			},
			wantLast: "world",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunks := wordWrapLineWithSegments(tc.line, 10, tc.segments)
			reconstructed := ""
			for _, chunk := range chunks {
				if VisibleWidth(chunk.Text) > 10 {
					t.Fatalf("chunk %q width=%d exceeds limit", chunk.Text, VisibleWidth(chunk.Text))
				}
				reconstructed += tc.line[chunk.StartIndex:chunk.EndIndex]
			}
			if reconstructed != tc.line {
				t.Fatalf("reconstructed = %q, want original %q", reconstructed, tc.line)
			}
			if tc.wantFirst != "" && chunks[0].Text != tc.wantFirst {
				t.Fatalf("first chunk = %q, want %q", chunks[0].Text, tc.wantFirst)
			}
			last := chunks[len(chunks)-1].Text
			if tc.wantLast != "" && last != tc.wantLast {
				t.Fatalf("last chunk = %q, want %q", last, tc.wantLast)
			}
			if tc.wantLastHasB && !strings.Contains(last, "B") {
				t.Fatalf("last chunk should contain B, chunks=%#v", chunks)
			}
		})
	}
}

func TestMarkdownListsWrapAndIndent(t *testing.T) {
	md := NewMarkdown("- parent\n  - alpha beta gamma delta epsilon", MarkdownTheme{})
	lines := md.Render(24)
	if strings.Join(lines, "\n") != "- parent\n    - alpha beta gamma\n      delta epsilon" {
		t.Fatalf("markdown list render = %#v", lines)
	}
}

func TestMarkdownListLazyContinuationMatchesMarked(t *testing.T) {
	md := NewMarkdown("- first\ncontinuation\n  indented continuation", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	want := []string{"- first", "  continuation", "  indented continuation"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unordered lazy continuation = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("3. first\nlazy continuation", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"3. first", "   lazy continuation"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ordered lazy continuation = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("> - quote item\n> lazy continuation", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"│ - quote item", "│   lazy continuation"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("blockquote list lazy continuation = %#v, want %#v", lines, want)
	}
}

func TestMarkdownLooseListContinuationAfterBlankMatchesMarked(t *testing.T) {
	md := NewMarkdown("- first\n\n  second paragraph\n- next", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	want := []string{"- first", "", "  second paragraph", "- next"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("loose unordered list continuation = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("1. first\n\n   second paragraph\n2. next", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"1. first", "", "   second paragraph", "2. next"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("loose ordered list continuation = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("- parent\n  - first\n\n    second paragraph\n  - second", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"- parent", "    - first", "", "      second paragraph", "    - second"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("loose nested list continuation = %#v, want %#v", lines, want)
	}
}

func TestMarkdownListBlankSpacingMatchesPi(t *testing.T) {
	lines := stripANSILines(NewMarkdown("- one\n\n- two", MarkdownTheme{}).Render(80))
	if got, want := strings.Join(lines, "\n"), "- one\n- two"; got != want {
		t.Fatalf("loose list blank line should not render as visible spacer: got %#v want %#v", got, want)
	}

	lines = stripANSILines(NewMarkdown("> - one\n>\n> - two", MarkdownTheme{}).Render(80))
	if got, want := strings.Join(lines, "\n"), "│ - one\n│ - two"; got != want {
		t.Fatalf("blockquote loose list item blank line should not render as visible spacer: got %#v want %#v", got, want)
	}

	md := NewMarkdown("- [top][top]\n  [top]: https://example.com/top\n  - [nested][nested]\n    [nested]: <https://example.com/nested?a=1&amp;b=2>\n      \"Nested title\"\n- [reuse nested][nested]", MarkdownTheme{})
	lines = stripANSILines(md.Render(120))
	joined := strings.Join(lines, "\n")
	want := "[top]: https://example.com/top\n\n    - [nested][nested]"
	if !strings.Contains(joined, want) {
		t.Fatalf("reference-like list continuation should keep Pi spacer before nested list, missing %q in %q", want, joined)
	}
}

func TestMarkdownBlockquoteLooseListContinuationAfterBlankMatchesMarked(t *testing.T) {
	md := NewMarkdown("> - first\n>\n>   second paragraph\n> - next", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	want := []string{"│ - first", "│ ", "│   second paragraph", "│ - next"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("blockquote loose unordered continuation = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("> 1. first\n>\n>    second paragraph\n> 2. next", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"│ 1. first", "│ ", "│    second paragraph", "│ 2. next"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("blockquote loose ordered continuation = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("> - parent\n>   - first\n>\n>     second paragraph\n>   - second", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"│ - parent", "│     - first", "│ ", "│       second paragraph", "│     - second"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("blockquote loose nested continuation = %#v, want %#v", lines, want)
	}
}

func TestMarkdownNestedOrderedAndMixedListsMatchPi(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "deep unordered",
			text: "- Level 1\n  - Level 2\n    - Level 3\n      - Level 4",
			want: []string{"- Level 1", "    - Level 2", "        - Level 3", "            - Level 4"},
		},
		{
			name: "unordered markers normalize to dash",
			text: "* Star\n+ Plus\n- Dash",
			want: []string{"- Star", "- Plus", "- Dash"},
		},
		{
			name: "ordered nested",
			text: "1. First\n   1. Nested first\n   2. Nested second\n2. Second",
			want: []string{"1. First", "    1. Nested first", "    2. Nested second", "2. Second"},
		},
		{
			name: "mixed ordered unordered",
			text: "1. Ordered item\n   - Unordered nested\n   - Another nested\n2. Second ordered\n   - More nested",
			want: []string{"1. Ordered item", "    - Unordered nested", "    - Another nested", "2. Second ordered", "    - More nested"},
		},
		{
			name: "wrapped ordered",
			text: "1. alpha beta gamma delta epsilon",
			want: []string{"1. alpha beta gamma", "   delta epsilon"},
		},
		{
			name: "wrapped multi digit ordered",
			text: "10. alpha beta gamma delta epsilon",
			want: []string{"10. alpha beta gamma", "    delta epsilon"},
		},
		{
			name: "wrapped paren ordered marker normalizes to dot",
			text: "1) alpha beta gamma delta epsilon\n2) second",
			want: []string{"1. alpha beta gamma", "   delta epsilon", "2. second"},
		},
		{
			name: "ordered marker renumbers from list start",
			text: "3. first\n7. second\n9) third",
			want: []string{"3. first", "4. second", "5. third"},
		},
		{
			name: "wrapped nested under ordered parent",
			text: "1. parent\n   - alpha beta gamma delta epsilon",
			want: []string{"1. parent", "    - alpha beta gamma", "      delta epsilon"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			width := 80
			if strings.HasPrefix(tc.name, "wrapped") {
				width = 24
				if tc.name == "wrapped ordered" {
					width = 20
				} else if tc.name == "wrapped multi digit ordered" {
					width = 21
				} else if tc.name == "wrapped paren ordered marker normalizes to dot" {
					width = 20
				}
			}
			md := NewMarkdown(tc.text, MarkdownTheme{})
			lines := stripANSILines(md.Render(width))
			for i, line := range lines {
				lines[i] = strings.TrimRight(line, " ")
			}
			if strings.Join(lines, "\n") != strings.Join(tc.want, "\n") {
				t.Fatalf("markdown nested list lines = %#v, want %#v", lines, tc.want)
			}
		})
	}
}

func TestMarkdownOrderedListInterruptsParagraphOnlyAtOneLikeMarked(t *testing.T) {
	theme := MarkdownTheme{ListBullet: func(s string) string { return "<bullet>" + s + "</bullet>" }}

	md := NewMarkdown("Paragraph\n2. not a list", theme)
	joined := strings.Join(md.Render(80), "\n")
	if strings.Contains(joined, "<bullet>") {
		t.Fatalf("ordered marker starting at 2 should not interrupt paragraph: %q", joined)
	}
	if !strings.Contains(joined, "Paragraph") || !strings.Contains(joined, "2. not a list") {
		t.Fatalf("non-interrupting ordered marker should remain paragraph text: %q", joined)
	}

	md = NewMarkdown("Paragraph\n1. list item", theme)
	joined = strings.Join(md.Render(80), "\n")
	if !strings.Contains(joined, "<bullet>1. </bullet>list item") {
		t.Fatalf("ordered marker starting at 1 should interrupt paragraph: %q", joined)
	}

	md = NewMarkdown("- parent paragraph\n  2. not nested list", theme)
	joined = strings.Join(md.Render(80), "\n")
	if strings.Count(joined, "<bullet>") != 1 || !strings.Contains(joined, "2. not nested list") {
		t.Fatalf("nested ordered marker starting at 2 should stay in list paragraph: %q", joined)
	}

	md = NewMarkdown("> Paragraph\n> 2. not a list", theme)
	joined = strings.Join(md.Render(80), "\n")
	if strings.Contains(joined, "<bullet>") || !strings.Contains(joined, "2. not a list") {
		t.Fatalf("blockquote ordered marker starting at 2 should stay paragraph text: %q", joined)
	}
}

func TestMarkdownListBlockquotesWrapWithPiIndentation(t *testing.T) {
	md := NewMarkdown("- > alpha beta gamma delta epsilon zeta", MarkdownTheme{})
	lines := stripANSILines(md.Render(24))
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
		if VisibleWidth(lines[i]) > 24 {
			t.Fatalf("line exceeds width: %q", lines[i])
		}
	}
	want := []string{"- │ alpha beta gamma", "  │ delta epsilon zeta"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("list blockquote = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("- > first\n  > second", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"- │ first", "  │ second"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("multiline list blockquote = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("> - > first\n>   > second", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"│ - │ first", "│   │ second"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("blockquote list blockquote = %#v, want %#v", lines, want)
	}
}

func TestMarkdownTaskListMarkersRenderLiterally(t *testing.T) {
	md := NewMarkdown("- [ ] beep\n- [x] boop", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	if strings.Join(lines, "\n") != "- [ ] beep\n- [x] boop" {
		t.Fatalf("task list markers = %#v", lines)
	}
}

func TestMarkdownTaskListMarkersUseListBulletStyleLikePi(t *testing.T) {
	theme := MarkdownTheme{ListBullet: func(s string) string { return "<bullet>" + s + "</bullet>" }}
	md := NewMarkdown("- [ ] todo\n- [X] done\n1. [x] ordered", theme)
	joined := strings.Join(md.Render(80), "\n")
	for _, want := range []string{
		"<bullet>- [ ] </bullet>todo",
		"<bullet>- [x] </bullet>done",
		"<bullet>1. [x] </bullet>ordered",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("task marker should be part of styled list marker: missing %q in %q", want, joined)
		}
	}
}

func TestMarkdownNormalizesTabsBeforeParsingLikePi(t *testing.T) {
	md := NewMarkdown("\tcode", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	if got, want := strings.Join(lines, "\n"), "   code"; got != want {
		t.Fatalf("leading tab should normalize to three spaces before parsing, got %q want %q", got, want)
	}
	if strings.Contains(strings.Join(lines, "\n"), "```") {
		t.Fatalf("one leading tab should not become an indented code block: %#v", lines)
	}

	md = NewMarkdown(" \tcode", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want := []string{"```", "  code", "```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("space plus tab should become a four-space indented code block, got %#v want %#v", lines, want)
	}

	md = NewMarkdown("plain\ttext and `code\tspan`", MarkdownTheme{
		Code: func(s string) string { return "<code>" + s + "</code>" },
	})
	joined := strings.Join(md.Render(80), "\n")
	for _, want := range []string{"plain   text", "<code>code   span</code>"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("tab-normalized markdown missing %q in %q", want, joined)
		}
	}

	md = NewMarkdown("<foo\tattr=\"&amp; **raw attr**\">\n**raw body**\n\nafter **bold**", MarkdownTheme{
		Bold: func(s string) string { return "<b>" + s + "</b>" },
	})
	plain := stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, `<foo   attr="&amp; **raw attr**">`) || !strings.Contains(plain, "**raw body**") || !strings.Contains(plain, "after <b>bold</b>") {
		t.Fatalf("tab-normalized custom HTML block should render raw like Pi marked, got %q", plain)
	}
	if strings.Contains(plain, "<b>raw attr</b>") || strings.Contains(plain, "<b>raw body</b>") {
		t.Fatalf("tab-normalized custom HTML block should not parse raw contents, got %q", plain)
	}
}

func TestMarkdownEmptyTextSkipsPaddingLikePi(t *testing.T) {
	md := NewMarkdownWithOptions(" \n\t ", MarkdownOptions{
		PaddingX: 2,
		PaddingY: 1,
		DefaultTextStyle: &DefaultTextStyle{
			BgColor: func(s string) string { return "<bg>" + s + "</bg>" },
		},
	})
	if lines := md.Render(20); len(lines) != 0 {
		t.Fatalf("empty markdown should render no lines even with padding: %#v", lines)
	}
}

func TestMarkdownImageProtocolLinesSkipWrappingPaddingAndBackgroundLikePi(t *testing.T) {
	imageLine := "\x1b_Ga=T,f=100;" + strings.Repeat("A", 80) + "\x1b\\"
	md := NewMarkdownWithOptions(imageLine, MarkdownOptions{
		PaddingX: 2,
		PaddingY: 1,
		DefaultTextStyle: &DefaultTextStyle{
			BgColor: func(s string) string { return "<bg>" + s + "</bg>" },
		},
	})
	lines := md.Render(12)
	if len(lines) != 3 {
		t.Fatalf("markdown image line with vertical padding lines = %#v", lines)
	}
	if lines[1] != imageLine {
		t.Fatalf("image protocol line should bypass wrapping and horizontal padding, got %q want %q", lines[1], imageLine)
	}
	if !IsImageLine(lines[1]) || strings.Contains(lines[1], "<bg>") {
		t.Fatalf("image protocol line should remain raw and unstyled: %q", lines[1])
	}
	if !strings.Contains(lines[0], "<bg>") || !strings.Contains(lines[2], "<bg>") {
		t.Fatalf("vertical padding should still receive background styling: %#v", lines)
	}
}

func TestMarkdownRenderCacheInvalidatesLikePi(t *testing.T) {
	boldCalls := 0
	md := NewMarkdown("**bold**", MarkdownTheme{
		Bold: func(s string) string {
			boldCalls++
			return "<b>" + s + "</b>"
		},
	})

	first := md.Render(80)
	if boldCalls != 1 {
		t.Fatalf("first render bold calls = %d, want 1", boldCalls)
	}
	first[0] = "mutated caller copy"
	second := md.Render(80)
	if boldCalls != 1 {
		t.Fatalf("cached render should not restyle, bold calls = %d", boldCalls)
	}
	if second[0] != "<b>bold</b>" {
		t.Fatalf("cached render should not expose caller mutation, got %#v", second)
	}

	md.Invalidate()
	_ = md.Render(80)
	if boldCalls != 2 {
		t.Fatalf("Invalidate should force rerender, bold calls = %d", boldCalls)
	}

	_ = md.Render(81)
	if boldCalls != 3 {
		t.Fatalf("width change should force rerender, bold calls = %d", boldCalls)
	}

	md.SetText("**new**")
	if got := strings.Join(md.Render(80), "\n"); !strings.Contains(got, "<b>new</b>") {
		t.Fatalf("SetText should clear cached markdown, got %q", got)
	}
	if boldCalls != 4 {
		t.Fatalf("SetText should force rerender, bold calls = %d", boldCalls)
	}
}

func TestMarkdownOrderedListNumberingSurvivesUnindentedCodeBlock(t *testing.T) {
	md := NewMarkdown("1. First item\n\n```typescript\n// code block\n```\n\n2. Second item\n\n```typescript\n// another code block\n```\n\n3. Third item", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	var numbered []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) >= 2 && line[0] >= '0' && line[0] <= '9' && strings.Contains(line, ".") {
			numbered = append(numbered, line)
		}
	}
	if strings.Join(numbered, "\n") != strings.Join([]string{"1. First item", "2. Second item", "3. Third item"}, "\n") {
		t.Fatalf("ordered list numbering around unindented code blocks = %#v", numbered)
	}
	plain := strings.Join(lines, "\n")
	if !strings.Contains(plain, "```") || !strings.Contains(plain, "// code block") || !strings.Contains(plain, "// another code block") {
		t.Fatalf("ordered list numbering should be preserved around code block: %q", plain)
	}
	if strings.Contains(plain, "1. Second item") || strings.Contains(plain, "1. Third item") || strings.Contains(plain, "2. Third item") {
		t.Fatalf("ordered list numbering should not be renumbered: %q", plain)
	}
}

func TestMarkdownBlockquoteAndCodeFence(t *testing.T) {
	md := NewMarkdown("- > alpha beta gamma delta epsilon zeta\n```go\nfmt.Println(\"x\")\n```", MarkdownTheme{})
	lines := md.Render(24)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "- │ alpha beta gamma") || !strings.Contains(joined, "```go") || !strings.Contains(joined, "  fmt.Println") {
		t.Fatalf("markdown quote/code render = %#v", lines)
	}
}

func TestMarkdownCodeBlockHighlightAndIndentOptions(t *testing.T) {
	seenLang := ""
	seenCode := ""
	md := NewMarkdown("```go\nfmt.Println(1)\n```", MarkdownTheme{
		CodeBlockIndent: "    ",
		HighlightCode: func(code, lang string) []string {
			seenCode = code
			seenLang = lang
			return []string{"<" + code + ">"}
		},
	})
	lines := stripANSILines(md.Render(80))
	want := []string{"```go", "    <fmt.Println(1)>", "```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("highlighted code block = %#v, want %#v", lines, want)
	}
	if seenLang != "go" || seenCode != "fmt.Println(1)" {
		t.Fatalf("highlight callback saw lang=%q code=%q", seenLang, seenCode)
	}

	md = NewMarkdown("- ```go\n  fmt.Println(1)\n  ```", MarkdownTheme{CodeBlockIndent: "    "})
	lines = stripANSILines(md.Render(80))
	want = []string{"- ```go", "      fmt.Println(1)", "  ```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("list code block indent = %#v, want %#v", lines, want)
	}
}

func TestMarkdownTopLevelCodeBlockWrapsContinuationLikePi(t *testing.T) {
	md := NewMarkdown("```txt\nalpha beta gamma delta epsilon\n```", MarkdownTheme{})
	lines := stripANSILines(md.Render(20))
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	want := []string{"```txt", "  alpha beta gamma", "delta epsilon", "```"}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("wrapped top-level code block lines = %#v, want %#v", lines, want)
	}
}

func TestMarkdownIndentedFencedCodeStripsOpeningIndentLikeMarked(t *testing.T) {
	md := NewMarkdown("  ```go\n  fmt.Println(1)\n    indented\n  ```", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	want := []string{"```go", "  fmt.Println(1)", "    indented", "```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("indented fence = %#v, want %#v", lines, want)
	}

	md = NewMarkdown(">   ~~~ts\n>   const x = 1\n>     const y = 2\n>   ~~~", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"│ ```ts", "│   const x = 1", "│     const y = 2", "│ ```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("blockquote indented fence = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("-   ```sh\n    echo ok\n      echo nested\n    ```", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"- ```sh", "    echo ok", "      echo nested", "  ```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("list indented fence = %#v, want %#v", lines, want)
	}
}

func TestMarkdownTildeFencesRenderLikePiCodeBlocks(t *testing.T) {
	md := NewMarkdown("~~~go\nfmt.Println(1)\n~~~", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	want := []string{"```go", "  fmt.Println(1)", "```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("tilde code fence = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("> ~~~ts\n> const x = 1\n> ~~~", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"│ ```ts", "│   const x = 1", "│ ```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("blockquote tilde code fence = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("- ~~~sh\n  echo ok\n  ~~~", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"- ```sh", "    echo ok", "  ```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("list tilde code fence = %#v, want %#v", lines, want)
	}
}

func TestMarkdownIndentedCodeBlocksRenderLikePi(t *testing.T) {
	md := NewMarkdown("    fmt.Println(1)\n    fmt.Println(2)\n\ntext", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	want := []string{"```", "  fmt.Println(1)", "  fmt.Println(2)", "```", "", "text"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("indented code block = %#v, want %#v", lines, want)
	}

	md = NewMarkdown(">     quoted code\n>     second line", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"│ ```", "│   quoted code", "│   second line", "│ ```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("blockquote indented code block = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("- item\n    code\n    more", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"- item", "  ```", "    code", "    more", "  ```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("list indented code block = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("> - item\n>     code\n>     more", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"│ - item", "│   ```", "│     code", "│     more", "│   ```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("blockquote list indented code block = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("- parent\n    - nested", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"- parent", "    - nested"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("indented nested list should stay list, got %#v want %#v", lines, want)
	}

	md = NewMarkdown("> - parent\n>     - nested", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"│ - parent", "│     - nested"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("blockquote indented nested list should stay list, got %#v want %#v", lines, want)
	}
}

func TestMarkdownIndentedCodeBlocksPreserveInteriorBlankLinesLikeMarked(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "top level",
			text: "    alpha\n\n    beta\n\ntext",
			want: []string{"```", "  alpha", "", "  beta", "```", "", "text"},
		},
		{
			name: "blockquote",
			text: ">     alpha\n>\n>     beta\n\ntext",
			want: []string{"│ ```", "│   alpha", "│", "│   beta", "│ ```", "", "text"},
		},
		{
			name: "loose list",
			text: "- item\n\n    alpha\n\n    beta\n- next",
			want: []string{"- item", "", "  ```", "    alpha", "", "    beta", "  ```", "- next"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			md := NewMarkdown(tc.text, MarkdownTheme{})
			lines := stripANSILines(md.Render(80))
			for i, line := range lines {
				lines[i] = strings.TrimRight(line, " ")
			}
			if strings.Join(lines, "\n") != strings.Join(tc.want, "\n") {
				t.Fatalf("indented code block lines = %#v, want %#v", lines, tc.want)
			}
		})
	}
}

func TestMarkdownTopLevelIndentedListMarkersRenderAsCodeLikeMarked(t *testing.T) {
	md := NewMarkdown("    - not a list\n    1. also code\n\ntext", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	want := []string{"```", "  - not a list", "  1. also code", "```", "", "text"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("top-level indented list-looking code = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("paragraph\n    - still code", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"paragraph", "```", "  - still code", "```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("indented list-looking code after paragraph = %#v, want %#v", lines, want)
	}
}

func TestMarkdownTopLevelIndentedListMarkersMatchMarked(t *testing.T) {
	md := NewMarkdown("   - alpha beta gamma delta epsilon", MarkdownTheme{})
	lines := stripANSILines(md.Render(20))
	want := []string{"- alpha beta gamma", "  delta epsilon"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("top-level indented unordered list = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("  3. alpha beta gamma delta epsilon", MarkdownTheme{})
	lines = stripANSILines(md.Render(20))
	want = []string{"3. alpha beta gamma", "   delta epsilon"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("top-level indented ordered list = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("   - first\n   - second", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"- first", "- second"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("same-indent top-level list = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("1234567890. alpha beta gamma delta", MarkdownTheme{})
	lines = stripANSILines(md.Render(20))
	want = []string{"1234567890. alpha", "beta gamma delta"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ten-digit ordered marker should stay paragraph = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("-\n+\n*\n1.\n2)", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"- ", "- ", "- ", "1. ", "2. "}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("empty list markers = %#v, want %#v", lines, want)
	}
}

func TestMarkdownListCodeBlockWrapsWithPiIndentation(t *testing.T) {
	md := NewMarkdown("- ```ts\n  alpha beta gamma delta epsilon zeta\n  ```", MarkdownTheme{})
	lines := md.Render(24)
	plain := make([]string, len(lines))
	for i, line := range lines {
		plain[i] = strings.TrimRight(stripANSI(line), " ")
		if VisibleWidth(plain[i]) > 24 {
			t.Fatalf("line exceeds width: %q", plain[i])
		}
	}
	want := []string{"- ```ts", "    alpha beta gamma", "  delta epsilon zeta", "  ```"}
	if strings.Join(plain, "\n") != strings.Join(want, "\n") {
		t.Fatalf("list code block = %#v, want %#v", plain, want)
	}
}

func TestMarkdownLooseListCodeBlocksStayIndentedLikePi(t *testing.T) {
	md := NewMarkdown("- item\n\n    code\n    more", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	want := []string{"- item", "", "  ```", "    code", "    more", "  ```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("loose list indented code = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("- item\n\n  ```go\n  fmt.Println(1)\n  ```", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"- item", "", "  ```go", "    fmt.Println(1)", "  ```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("loose list fenced code = %#v, want %#v", lines, want)
	}

	md = NewMarkdown("- item\n\n```go\nfmt.Println(1)\n```", MarkdownTheme{})
	lines = stripANSILines(md.Render(80))
	want = []string{"- item", "", "```go", "  fmt.Println(1)", "```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unindented fence after list should remain top-level = %#v, want %#v", lines, want)
	}
}

func TestMarkdownBlockquoteLazyContinuationAndExplicitLines(t *testing.T) {
	theme := MarkdownTheme{
		Quote:       func(s string) string { return "\x1b[3m" + s + "\x1b[23m" },
		QuoteBorder: func(s string) string { return s },
	}
	md := NewMarkdown(">Foo\nbar\n\n>baz\n>qux", theme)
	lines := md.Render(80)
	plain := make([]string, len(lines))
	for i, line := range lines {
		plain[i] = stripANSI(line)
	}
	want := []string{"│ Foo", "│ bar", "", "│ baz", "│ qux"}
	if strings.Join(plain, "\n") != strings.Join(want, "\n") {
		t.Fatalf("blockquote lines = %#v, want %#v", plain, want)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "\x1b[3m") {
		t.Fatalf("blockquote should apply quote styling: %#v", lines)
	}
}

func TestMarkdownBlockquoteLazyContinuationStopsBeforeStructuralBlocks(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "list outside quote",
			text: "> quote\n- outside",
			want: []string{"│ quote", "", "- outside"},
		},
		{
			name: "heading outside quote",
			text: "> quote\n# Outside",
			want: []string{"│ quote", "", "Outside"},
		},
		{
			name: "table outside quote",
			text: "> quote\n| A |\n| --- |\n| B |",
			want: []string{"│ quote", "", "┌───┐", "│ A │", "├───┤", "│ B │", "└───┘"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			md := NewMarkdown(tc.text, MarkdownTheme{})
			lines := stripANSILines(md.Render(80))
			for i := range lines {
				lines[i] = strings.TrimRight(lines[i], " ")
			}
			if strings.Join(lines, "\n") != strings.Join(tc.want, "\n") {
				t.Fatalf("blockquote structural boundary = %#v, want %#v", lines, tc.want)
			}
		})
	}
}

func TestMarkdownBlockquoteWrappedLinesKeepBorder(t *testing.T) {
	md := NewMarkdown("> This is a very long blockquote line that should wrap to multiple lines when rendered", MarkdownTheme{})
	lines := md.Render(30)
	var content []string
	for _, line := range lines {
		plain := strings.TrimRight(stripANSI(line), " ")
		if plain != "" {
			content = append(content, plain)
		}
		if VisibleWidth(plain) > 30 {
			t.Fatalf("line exceeds width: %q", plain)
		}
	}
	if len(content) <= 1 {
		t.Fatalf("expected wrapped blockquote lines, got %#v", content)
	}
	for _, line := range content {
		if !strings.HasPrefix(line, "│ ") {
			t.Fatalf("wrapped quote line missing border: %q", line)
		}
	}
	joined := strings.Join(content, " ")
	for _, want := range []string{"very long", "blockquote", "multiple"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("wrapped blockquote lost %q in %#v", want, content)
		}
	}
}

func TestMarkdownBlockquoteDoesNotInheritDefaultTextColor(t *testing.T) {
	md := NewMarkdownWithOptions("> This is styled text that is long enough to wrap", MarkdownOptions{
		Theme: MarkdownTheme{
			Quote: func(s string) string { return "\x1b[3m" + s + "\x1b[23m" },
		},
		DefaultTextStyle: &DefaultTextStyle{
			Color:  func(s string) string { return "\x1b[33m" + s + "\x1b[39m" },
			Italic: true,
		},
	})
	lines := md.Render(25)
	var content []string
	for _, line := range lines {
		plain := strings.TrimRight(stripANSI(line), " ")
		if plain != "" {
			content = append(content, plain)
		}
	}
	if len(content) <= 1 {
		t.Fatalf("expected wrapped blockquote lines, got %#v", content)
	}
	for _, line := range content {
		if !strings.HasPrefix(line, "│ ") {
			t.Fatalf("wrapped blockquote line missing quote border: %q", line)
		}
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "\x1b[3m") {
		t.Fatalf("blockquote should apply quote italic style: %q", joined)
	}
	if strings.Contains(joined, "\x1b[33m") {
		t.Fatalf("blockquote should not inherit default text color: %q", joined)
	}
}

func TestMarkdownBlockquoteCodeFenceUsesPiBlockRendering(t *testing.T) {
	md := NewMarkdown("> ```go\n> fmt.Println(1)\n> ```", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	want := []string{"│ ```go", "│   fmt.Println(1)", "│ ```"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("blockquote code fence = %#v, want %#v", lines, want)
	}
}

func TestMarkdownBlockquoteTableUsesPiBlockRendering(t *testing.T) {
	md := NewMarkdown("> | A | B |\n> | --- | --- |\n> | 1 | 2 |", MarkdownTheme{})
	plain := strings.Join(stripANSILines(md.Render(80)), "\n")
	for _, want := range []string{"│ ┌", "│ │ A", "│ │ 1", "│ └"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("blockquote table missing %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "| --- |") {
		t.Fatalf("blockquote table should be rendered, not left as markdown pipes: %q", plain)
	}
}

func TestMarkdownBlockquoteNestedListUsesPiIndentation(t *testing.T) {
	md := NewMarkdown("> - parent\n>   - child item\n> 3. first\n> 7) second\n> + plus", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	want := []string{"│ - parent", "│     - child item", "│ 3. first", "│ 4. second", "│ - plus"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("blockquote nested list = %#v, want %#v", lines, want)
	}
}

func TestMarkdownNestedBlockquoteUsesPiBorders(t *testing.T) {
	md := NewMarkdown("> outer\n> > inner", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	want := []string{"│ outer", "│ │ inner"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("nested blockquote = %#v, want %#v", lines, want)
	}
}

func TestMarkdownBlockquoteHeadingsAndRuleUsePiBlockRendering(t *testing.T) {
	md := NewMarkdown("> # Title with `code`\n> ---", MarkdownTheme{})
	plain := strings.Join(stripANSILines(md.Render(40)), "\n")
	if strings.Contains(plain, "# Title") {
		t.Fatalf("blockquote h1 should not render ATX marker: %q", plain)
	}
	for _, want := range []string{"│ Title with code", "│ ───"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("blockquote heading/rule missing %q in %q", want, plain)
		}
	}

	setext := NewMarkdown("> Subtitle\n> ---\n> body", MarkdownTheme{})
	setextPlain := strings.Join(stripANSILines(setext.Render(40)), "\n")
	if strings.Contains(setextPlain, "---") || !strings.Contains(setextPlain, "│ Subtitle") || !strings.Contains(setextPlain, "│ body") {
		t.Fatalf("blockquote setext heading render = %q", setextPlain)
	}
}

func TestMarkdownBlockquoteRestoresQuoteStyleAfterInlineResets(t *testing.T) {
	theme := MarkdownTheme{
		Quote: func(s string) string { return "\x1b[3m" + s + "\x1b[23m" },
		Bold:  func(s string) string { return "\x1b[1m" + s + "\x1b[22m" },
		Code:  func(s string) string { return "\x1b[33m" + s + "\x1b[0m" },
	}
	md := NewMarkdown("> Quote with **bold** and `code` after", theme)
	joined := strings.Join(md.Render(80), "\n")

	afterBold := strings.Index(joined, "and")
	if afterBold < 0 {
		t.Fatalf("blockquote output missing text after bold: %q", joined)
	}
	boldPrefix := joined[max(0, afterBold-30):afterBold]
	if !strings.Contains(boldPrefix, "\x1b[3m") {
		t.Fatalf("quote style should be restored after bold reset, prefix=%q output=%q", boldPrefix, joined)
	}

	afterCode := strings.Index(joined, "after")
	if afterCode < 0 {
		t.Fatalf("blockquote output missing text after code: %q", joined)
	}
	codePrefix := joined[max(0, afterCode-30):afterCode]
	if !strings.Contains(codePrefix, "\x1b[3m") {
		t.Fatalf("quote style should be restored after code reset, prefix=%q output=%q", codePrefix, joined)
	}
}

func TestMarkdownPiBlockSpacingNormalization(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "code block without source blanks",
			text: "hello this is text\n```\ncode block\n```\nmore text",
			want: []string{"hello this is text", "", "```", "  code block", "```", "", "more text"},
		},
		{
			name: "code block with source blanks",
			text: "hello this is text\n\n```\ncode block\n```\n\nmore text",
			want: []string{"hello this is text", "", "```", "  code block", "```", "", "more text"},
		},
		{
			name: "heading before paragraph",
			text: "# Hello\nThis is a paragraph",
			want: []string{"Hello", "", "This is a paragraph"},
		},
		{
			name: "divider before paragraph",
			text: "hello world\n\n---\n\nagain, hello world",
			want: []string{"hello world", "", strings.Repeat("─", 80), "", "again, hello world"},
		},
		{
			name: "blockquote before paragraph",
			text: "hello world\n\n> This is a quote\n\nagain, hello world",
			want: []string{"hello world", "", "│ This is a quote", "", "again, hello world"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			md := NewMarkdown(tc.text, MarkdownTheme{})
			lines := md.Render(80)
			plain := make([]string, len(lines))
			for i, line := range lines {
				plain[i] = strings.TrimRight(stripANSI(line), " ")
			}
			if strings.Join(plain, "\n") != strings.Join(tc.want, "\n") {
				t.Fatalf("markdown lines = %#v, want %#v", plain, tc.want)
			}
		})
	}
}

func TestMarkdownHorizontalRulesAllowSpacedMarkersLikeMarked(t *testing.T) {
	for _, text := range []string{"- - -", "* * *", "_ _ _"} {
		md := NewMarkdown(text, MarkdownTheme{})
		lines := stripANSILines(md.Render(20))
		if len(lines) != 1 || strings.TrimRight(lines[0], " ") != strings.Repeat("─", 20) {
			t.Fatalf("spaced horizontal rule %q rendered as %#v", text, lines)
		}
	}

	md := NewMarkdown("- -\n- * -", MarkdownTheme{})
	plain := strings.Join(stripANSILines(md.Render(20)), "\n")
	if strings.Contains(plain, "───") {
		t.Fatalf("invalid spaced marker lines should not render as horizontal rules: %q", plain)
	}
	if !strings.Contains(plain, "- -") || !strings.Contains(plain, "- * -") {
		t.Fatalf("invalid marker lines should remain visible: %q", plain)
	}
}

func TestMarkdownPiBlockSpacingNoTrailingBlank(t *testing.T) {
	cases := []string{
		"# Hello",
		"---",
		"> This is a quote",
		"```js\nconst hello = 'world';\n```",
	}
	for _, text := range cases {
		md := NewMarkdown(text, MarkdownTheme{})
		lines := md.Render(80)
		if len(lines) == 0 {
			t.Fatalf("expected lines for %q", text)
		}
		if strings.TrimRight(stripANSI(lines[len(lines)-1]), " ") == "" {
			t.Fatalf("unexpected trailing blank for %q: %#v", text, lines)
		}
	}
}

func TestMarkdownSetextHeadings(t *testing.T) {
	theme := MarkdownTheme{
		Heading:   func(s string) string { return "<h>" + s + "</h>" },
		Bold:      func(s string) string { return "<b>" + s + "</b>" },
		Underline: func(s string) string { return "<u>" + s + "</u>" },
		HR:        func(s string) string { return "<hr>" + s + "</hr>" },
	}
	md := NewMarkdown("Title with `code`\n===\n\nSubtitle\n---\n\nbody", theme)
	joined := strings.Join(md.Render(120), "\n")
	plain := stripANSI(joined)

	for _, want := range []string{
		"<h><b><u>Title with ",
		"code",
		"</u></b></h>",
		"<h><b>Subtitle</b></h>",
		"body",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("setext heading missing %q in %q", want, joined)
		}
	}
	for _, unwanted := range []string{"===", "<hr>"} {
		if strings.Contains(plain, unwanted) || strings.Contains(joined, unwanted) {
			t.Fatalf("setext heading rendered underline marker %q in %q", unwanted, joined)
		}
	}

	md = NewMarkdown("First line\nsecond line\n---\n\nbody", theme)
	joined = strings.Join(md.Render(120), "\n")
	if !strings.Contains(joined, "<h><b>First line\nsecond line</b></h>") {
		t.Fatalf("multiline setext heading should render as one marked heading token: %q", joined)
	}
	if strings.Contains(joined, "<hr>") || strings.Contains(joined, "---") {
		t.Fatalf("multiline setext heading rendered underline marker: %q", joined)
	}

	quoted := NewMarkdown("> First line\n> second line\n> ---\n> body", MarkdownTheme{})
	quotedPlain := strings.Join(stripANSILines(quoted.Render(80)), "\n")
	if !strings.Contains(quotedPlain, "│ First line\n│ second line") || strings.Contains(quotedPlain, "---") {
		t.Fatalf("blockquote multiline setext heading render = %q", quotedPlain)
	}

	md = NewMarkdown("Allowed\n   ---\n\nNot heading\n    ---\n\nbody", theme)
	joined = strings.Join(md.Render(120), "\n")
	plain = stripANSI(joined)
	if !strings.Contains(joined, "<h><b>Allowed</b></h>") {
		t.Fatalf("three-space setext underline should render heading: %q", joined)
	}
	if strings.Contains(joined, "<h><b>Not heading</b></h>") || !strings.Contains(plain, "Not heading") || !strings.Contains(plain, "---") {
		t.Fatalf("four-space setext underline should remain literal text/code, got %q", joined)
	}
}

func TestMarkdownCombinedListsAndTablesLikePi(t *testing.T) {
	md := NewMarkdown("# Test Document\n\n- Item 1\n  - Nested item\n- Item 2\n\n| Col1 | Col2 |\n| --- | --- |\n| A | B |", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Test Document", "- Item 1", "    - Nested item", "- Item 2", "Col1", "│"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("combined markdown missing %q in %#v", want, lines)
		}
	}
}

func TestMarkdownTable(t *testing.T) {
	md := NewMarkdown("| Name | Age |\n| --- | --- |\n| Alice | 30 |\n| Bob | 25 |", MarkdownTheme{})
	lines := md.Render(80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "│ Name") || !strings.Contains(joined, "Alice") || !strings.Contains(joined, "┼") || !strings.Contains(joined, "┌") || !strings.Contains(joined, "└") {
		t.Fatalf("markdown table render = %#v", lines)
	}
}

func TestMarkdownListContainedTableMatchesMarked(t *testing.T) {
	md := NewMarkdown("- | A | B |\n  | - | - |\n  | one | two |", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"- ┌", "  │ A", "  │ one"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("list-contained table missing %q in %#v", want, lines)
		}
	}
	if strings.Contains(joined, "| A | B |") {
		t.Fatalf("list-contained table should render as table like marked, got %#v", lines)
	}
}

func TestMarkdownPiTableStructureAndWidthMatrix(t *testing.T) {
	t.Run("row dividers between data rows", func(t *testing.T) {
		md := NewMarkdown("| Name | Age |\n| --- | --- |\n| Alice | 30 |\n| Bob | 25 |", MarkdownTheme{})
		lines := stripANSILines(md.Render(80))
		dividerLines := 0
		for _, line := range lines {
			if strings.Contains(line, "┼") {
				dividerLines++
			}
		}
		if dividerLines != 2 {
			t.Fatalf("divider lines = %d, want header + row divider in %#v", dividerLines, lines)
		}
	})

	t.Run("varying column widths keep content", func(t *testing.T) {
		md := NewMarkdown("| Short | Very long column header |\n| --- | --- |\n| A | This is a much longer cell content |\n| B | Short |", MarkdownTheme{})
		plain := strings.Join(stripANSILines(md.Render(80)), "\n")
		for _, want := range []string{"Very long column header", "This is a much longer cell content", "│ A", "│ B"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("varying-width table missing %q in %q", want, plain)
			}
		}
	})

	t.Run("multi-column narrow wrap preserves content", func(t *testing.T) {
		md := NewMarkdown("| Command | Description | Example |\n| --- | --- | --- |\n| npm install | Install all dependencies | npm install |\n| npm run build | Build the project | npm run build |", MarkdownTheme{})
		lines := stripANSILines(md.Render(50))
		for _, line := range lines {
			line = strings.TrimRight(line, " ")
			if VisibleWidth(line) > 50 {
				t.Fatalf("narrow table line exceeds width: %q", line)
			}
		}
		joined := strings.Join(lines, " ")
		for _, want := range []string{"Command", "Description", "npm install", "Install"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("narrow table lost %q in %#v", want, lines)
			}
		}
	})

	t.Run("natural fit keeps table structure", func(t *testing.T) {
		md := NewMarkdown("| A | B |\n| --- | --- |\n| 1 | 2 |", MarkdownTheme{})
		lines := stripANSILines(md.Render(80))
		joined := strings.Join(lines, "\n")
		for _, want := range []string{"│ A", "│ 1", "├", "┼", "└"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("natural-fit table missing %q in %#v", want, lines)
			}
		}
	})
}

func TestMarkdownTableAlignmentMarkersStayLeftAlignedLikePi(t *testing.T) {
	md := NewMarkdown("| Left | Center | Right |\n| :--- | :---: | ---: |\n| A | B | C |\n| Long text | Middle | End |", MarkdownTheme{})
	lines := md.Render(120)
	var row string
	for _, line := range lines {
		if strings.Contains(line, "│ A") && strings.Contains(line, "B") && strings.Contains(line, "C") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("aligned data row not found in %#v", lines)
	}
	segments := strings.Split(row, "│")
	if len(segments) < 5 {
		t.Fatalf("unexpected table row shape: %q", row)
	}
	leftCell := segments[1]
	centerCell := segments[2]
	rightCell := segments[3]
	if !strings.HasPrefix(leftCell, " A") {
		t.Fatalf("left cell should be left aligned: %q", leftCell)
	}
	if !strings.HasPrefix(centerCell, " B") || !strings.HasSuffix(centerCell, "      ") {
		t.Fatalf("Pi keeps center marker cells left aligned: %q", centerCell)
	}
	if !strings.HasPrefix(rightCell, " C") || !strings.HasSuffix(rightCell, "     ") {
		t.Fatalf("Pi keeps right marker cells left aligned: %q", rightCell)
	}
}

func TestMarkdownTableWrapsLongCellContent(t *testing.T) {
	md := NewMarkdown("| Header |\n| --- |\n| This is a very long cell content that should wrap |", MarkdownTheme{})
	lines := md.Render(25)
	plain := make([]string, len(lines))
	for i, line := range lines {
		plain[i] = strings.TrimRight(stripANSI(line), " ")
		if VisibleWidth(plain[i]) > 25 {
			t.Fatalf("line exceeds width: %q", plain[i])
		}
	}
	dataRows := 0
	for _, line := range plain {
		if strings.HasPrefix(line, "│") && !strings.Contains(line, "─") {
			dataRows++
		}
	}
	if dataRows <= 2 {
		t.Fatalf("expected wrapped data rows, got %#v", plain)
	}
	joined := strings.Join(plain, " ")
	for _, want := range []string{"very long", "cell content", "should wrap"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("wrapped table lost %q in %#v", want, plain)
		}
	}
}

func TestMarkdownTableWrapsLongUnbrokenTokenInsideCell(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()
	url := "https://example.com/this/is/a/very/long/url/that/should/wrap"
	md := NewMarkdown("| Value |\n| --- |\n| prefix "+url+" |", MarkdownTheme{})
	lines := md.Render(30)
	plain := make([]string, len(lines))
	for i, line := range lines {
		plain[i] = strings.TrimRight(stripANSI(line), " ")
		if VisibleWidth(plain[i]) > 30 {
			t.Fatalf("line exceeds width: %q", plain[i])
		}
		if strings.HasPrefix(plain[i], "│") && strings.Count(plain[i], "│") != 2 {
			t.Fatalf("wrapped row should keep borders: %q", plain[i])
		}
	}
	extracted := strings.NewReplacer("│", "", "├", "", "┤", "", "┌", "", "┬", "", "┐", "", "└", "", "┴", "", "┘", "", "─", "", " ", "").Replace(strings.Join(plain, ""))
	if !strings.Contains(extracted, "prefix") || !strings.Contains(extracted, url) {
		t.Fatalf("wrapped table lost token, extracted=%q lines=%#v", extracted, plain)
	}
}

func TestMarkdownTableWrapsStyledInlineCodeWithoutBreakingBorders(t *testing.T) {
	md := NewMarkdown("| Code |\n| --- |\n| `averyveryveryverylongidentifier` |", MarkdownTheme{
		Code: func(s string) string { return "\x1b[33m" + s + "\x1b[0m" },
	})
	lines := md.Render(20)
	if !strings.Contains(strings.Join(lines, "\n"), "\x1b[33m") {
		t.Fatalf("inline code should retain style: %#v", lines)
	}
	for _, line := range lines {
		plain := strings.TrimRight(stripANSI(line), " ")
		if VisibleWidth(plain) > 20 {
			t.Fatalf("line exceeds width: %q", plain)
		}
		if strings.HasPrefix(plain, "│") && strings.Count(plain, "│") != 2 {
			t.Fatalf("wrapped row should keep borders: %q", plain)
		}
	}
}

func TestMarkdownTableSplitsPipesInsideCodeAndEscapesLikeMarked(t *testing.T) {
	md := NewMarkdown("| Expr | Meaning |\n| --- | --- |\n| `a | b` | escaped \\| pipe |", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"`a", "b`"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("table should split raw pipes inside table rows like marked, missing %q in %#v", want, lines)
		}
	}
	for _, unwanted := range []string{"a | b", "escaped | pipe"} {
		if strings.Contains(joined, unwanted) {
			t.Fatalf("table should not preserve pipe-containing cell %q as one cell, got %#v", unwanted, lines)
		}
	}
	for _, line := range lines {
		line = strings.TrimRight(line, " ")
		if strings.HasPrefix(line, "│") && strings.Count(line, "│") != 3 {
			t.Fatalf("two-column table row should keep header column count after raw pipe split: %q", line)
		}
	}
}

func TestMarkdownTableFallsBackToRawMarkdownWhenTooNarrow(t *testing.T) {
	md := NewMarkdown("| A | B |\n| --- | --- |\n| 1 | 2 |", MarkdownTheme{})
	lines := stripANSILines(md.Render(5))
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "┌") || strings.Contains(joined, "│") {
		t.Fatalf("too-narrow table should fall back to raw markdown, got %#v", lines)
	}
	for _, line := range lines {
		line = strings.TrimRight(line, " ")
		if VisibleWidth(line) > 5 {
			t.Fatalf("raw fallback line exceeds width: %q", line)
		}
	}
	if !strings.Contains(joined, "| A |") || !strings.Contains(joined, "---") || !strings.Contains(joined, "| 1 |") {
		t.Fatalf("raw fallback should preserve table markdown, got %#v", lines)
	}
}

func TestMarkdownTableSupportsRowsWithoutOuterPipes(t *testing.T) {
	md := NewMarkdown("Name | Age\n--- | ---\nAlice | 30\nBob | 25", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"┌", "│ Name", "│ Alice", "│ Bob", "└"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("outer-pipe-free table missing %q in %#v", want, lines)
		}
	}
	if strings.Contains(joined, "Name | Age") || strings.Contains(joined, "--- | ---") {
		t.Fatalf("outer-pipe-free table should render as table, not raw markdown: %#v", lines)
	}

	paragraph := NewMarkdown("This is not | a table\nbecause this line is not a separator", MarkdownTheme{})
	plain := strings.Join(stripANSILines(paragraph.Render(80)), "\n")
	if strings.Contains(plain, "┌") || !strings.Contains(plain, "This is not | a table") {
		t.Fatalf("non-table paragraph with pipe misrendered: %q", plain)
	}
}

func TestMarkdownTableIndentationAndEmptyRowsMatchGFM(t *testing.T) {
	t.Run("single dash delimiters form marked table", func(t *testing.T) {
		md := NewMarkdown("| A | B |\n| - | - |\n| one | two |", MarkdownTheme{
			Bold: func(s string) string { return "<b>" + s + "</b>" },
		})
		joined := strings.Join(stripANSILines(md.Render(80)), "\n")
		for _, want := range []string{"┌", "│ <b>A", "│ one", "│ two", "└"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("single-dash delimiter table missing %q in %q", want, joined)
			}
		}
		if strings.Contains(joined, "| - | - |") {
			t.Fatalf("single-dash marked table should not remain raw markdown: %q", joined)
		}
	})

	t.Run("up to three spaces still forms table", func(t *testing.T) {
		md := NewMarkdown("   A | B\n   --- | ---\n   1 | 2", MarkdownTheme{})
		joined := strings.Join(stripANSILines(md.Render(80)), "\n")
		for _, want := range []string{"┌", "│ A", "│ 1", "└"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("three-space indented table missing %q in %q", want, joined)
			}
		}
	})

	t.Run("four-space header remains indented code", func(t *testing.T) {
		md := NewMarkdown("    A | B\n    --- | ---\n    1 | 2", MarkdownTheme{})
		joined := strings.Join(stripANSILines(md.Render(80)), "\n")
		if strings.Contains(joined, "┌") || strings.Contains(joined, "│ A") {
			t.Fatalf("four-space indented table should not render as table: %q", joined)
		}
		if !strings.Contains(joined, "```") || !strings.Contains(joined, "A | B") {
			t.Fatalf("four-space table-looking input should remain code-like content: %q", joined)
		}
	})

	t.Run("four-space separator prevents table", func(t *testing.T) {
		md := NewMarkdown("A | B\n    --- | ---\n1 | 2", MarkdownTheme{})
		joined := strings.Join(stripANSILines(md.Render(80)), "\n")
		if strings.Contains(joined, "┌") || strings.Contains(joined, "│ A") {
			t.Fatalf("four-space separator should not render as table: %q", joined)
		}
		if !strings.Contains(joined, "A | B") || !strings.Contains(joined, "--- | ---") {
			t.Fatalf("non-table input should preserve header and separator text: %q", joined)
		}
	})

	t.Run("empty header and data cells remain table cells", func(t *testing.T) {
		md := NewMarkdown("| | |\n| --- | --- |\n| | |\n| 1 | 2 |", MarkdownTheme{})
		lines := stripANSILines(md.Render(80))
		joined := strings.Join(lines, "\n")
		for _, want := range []string{"┌", "│ 1", "│ 2", "└"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("empty-cell table missing %q in %#v", want, lines)
			}
		}
		if strings.Contains(joined, "| | |") || strings.Contains(joined, "| 1 | 2 |") {
			t.Fatalf("empty-cell table rows should be consumed into table rendering: %#v", lines)
		}
	})
}

func TestMarkdownTablePiWidthBoundaries(t *testing.T) {
	t.Run("keeps width at least longest word when possible", func(t *testing.T) {
		longest := "superlongword"
		md := NewMarkdown("| Column One | Column Two |\n| --- | --- |\n| "+longest+" short | otherword |\n| small | tiny |", MarkdownTheme{})
		lines := stripANSILines(md.Render(32))
		var dataLine string
		for _, line := range lines {
			if strings.Contains(line, longest) {
				dataLine = line
				break
			}
		}
		if dataLine == "" {
			t.Fatalf("expected data row containing %q in %#v", longest, lines)
		}
		segments := strings.Split(dataLine, "│")
		if len(segments) < 3 {
			t.Fatalf("expected table borders in %q", dataLine)
		}
		firstColumnWidth := VisibleWidth(segments[1]) - 2
		if firstColumnWidth < len(longest) {
			t.Fatalf("first column width = %d, want at least %d in %q", firstColumnWidth, len(longest), dataLine)
		}
	})

	t.Run("extremely narrow width fits", func(t *testing.T) {
		md := NewMarkdown("| A | B | C |\n| --- | --- | --- |\n| 1 | 2 | 3 |", MarkdownTheme{})
		lines := stripANSILines(md.Render(15))
		if len(lines) == 0 {
			t.Fatalf("expected narrow table output")
		}
		for _, line := range lines {
			line = strings.TrimRight(line, " ")
			if VisibleWidth(line) > 15 {
				t.Fatalf("narrow table line exceeds width: %q", line)
			}
		}
	})

	t.Run("padding participates in width calculation", func(t *testing.T) {
		md := NewMarkdownWithOptions("| Column One | Column Two |\n| --- | --- |\n| Data 1 | Data 2 |", MarkdownOptions{PaddingX: 2})
		lines := stripANSILines(md.Render(40))
		for _, line := range lines {
			line = strings.TrimRight(line, " ")
			if VisibleWidth(line) > 40 {
				t.Fatalf("padded table line exceeds width: %q", line)
			}
			if strings.Contains(line, "│") && !strings.HasPrefix(line, "  ") {
				t.Fatalf("table row should preserve left padding: %q", line)
			}
		}
	})

	t.Run("weighted shrink matches Pi longest-token allocation", func(t *testing.T) {
		md := NewMarkdown("| Wide | Medium | Tiny |\n| --- | --- | --- |\n| averyveryveryveryveryveryveryvery | mediumlengthword | z |", MarkdownTheme{})
		lines := stripANSILines(md.Render(40))
		var row string
		for _, line := range lines {
			if strings.Contains(line, "avery") {
				row = line
				break
			}
		}
		if row == "" {
			t.Fatalf("expected wrapped data row containing long token in %#v", lines)
		}
		segments := strings.Split(row, "│")
		if len(segments) < 5 {
			t.Fatalf("unexpected table row shape: %q", row)
		}
		widths := []int{
			VisibleWidth(segments[1]) - 2,
			VisibleWidth(segments[2]) - 2,
			VisibleWidth(segments[3]) - 2,
		}
		if want := []int{18, 10, 2}; !reflect.DeepEqual(widths, want) {
			t.Fatalf("column widths = %v, want Pi weighted allocation %v in row %q", widths, want, row)
		}
	})

	t.Run("last table has no trailing blank", func(t *testing.T) {
		md := NewMarkdown("| Name |\n| --- |\n| Alice |", MarkdownTheme{})
		lines := stripANSILines(md.Render(80))
		if len(lines) == 0 {
			t.Fatalf("expected table lines")
		}
		if strings.TrimRight(lines[len(lines)-1], " ") == "" {
			t.Fatalf("unexpected trailing blank line: %#v", lines)
		}
	})
}

func TestMarkdownTableRequiresSeparatorColumnCountLikeMarked(t *testing.T) {
	theme := MarkdownTheme{
		Heading: func(s string) string { return "<h>" + s + "</h>" },
		Bold:    func(s string) string { return "<b>" + s + "</b>" },
	}
	tests := []struct {
		name string
		text string
	}{
		{
			name: "too few separator cells",
			text: "A | B\n---\n1 | 2",
		},
		{
			name: "too many separator cells",
			text: "A | B\n--- | --- | ---\n1 | 2 | 3",
		},
		{
			name: "empty separator cell",
			text: "A | B\n--- | \n1 | 2",
		},
		{
			name: "multiple leading alignment colons",
			text: "A | B\n::--- | ---\n1 | 2",
		},
		{
			name: "multiple trailing alignment colons",
			text: "A | B\n---:: | ---\n1 | 2",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			md := NewMarkdown(tc.text, theme)
			joined := strings.Join(stripANSILines(md.Render(120)), "\n")
			if strings.Contains(joined, "┌") || strings.Contains(joined, "│") || strings.Contains(joined, "└") {
				t.Fatalf("mismatched GFM table delimiter should not render as table: %q", joined)
			}
			if !strings.Contains(joined, "A | B") {
				t.Fatalf("non-table markdown should preserve header text, got %q", joined)
			}
		})
	}

	md := NewMarkdown("A | B\n--- | ---\n1 | 2", theme)
	joined := strings.Join(stripANSILines(md.Render(120)), "\n")
	if !strings.Contains(joined, "┌") || !strings.Contains(joined, "│") || !strings.Contains(joined, "1") || !strings.Contains(joined, "2") {
		t.Fatalf("matching GFM table delimiter should render as table, got %q", joined)
	}
}

func TestMarkdownInlineFormattingAndLinks(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()
	theme := MarkdownTheme{
		Bold:          func(s string) string { return "<b>" + s + "</b>" },
		Italic:        func(s string) string { return "<i>" + s + "</i>" },
		Code:          func(s string) string { return "<code>" + s + "</code>" },
		Link:          func(s string) string { return "<link>" + s + "</link>" },
		LinkURL:       func(s string) string { return "<url>" + s + "</url>" },
		Strikethrough: func(s string) string { return "<s>" + s + "</s>" },
	}
	md := NewMarkdown("**bold** *italic* `code` ~~gone~~ [docs](https://example.com)", theme)
	joined := strings.Join(md.Render(120), "\n")
	for _, want := range []string{
		"<b>bold</b>",
		"<i>italic</i>",
		"<code>code</code>",
		"<s>gone</s>",
		"<link>docs</link><url> (https://example.com)</url>",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("inline markdown missing %q in %q", want, joined)
		}
	}
}

func TestMarkdownMultilineParagraphTokensMatchMarked(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()
	theme := MarkdownTheme{
		Bold:          func(s string) string { return "<b>" + s + "</b>" },
		Italic:        func(s string) string { return "<i>" + s + "</i>" },
		Code:          func(s string) string { return "<code>" + s + "</code>" },
		Link:          func(s string) string { return "<link>" + s + "</link>" },
		LinkURL:       func(s string) string { return "<url>" + s + "</url>" },
		Strikethrough: func(s string) string { return "<s>" + s + "</s>" },
	}
	md := NewMarkdown("**bold\nline** and *italic\nline* and __under\nstrong__ and _under\nem_ and ~~gone\nline~~ and `code\nspan`\n[docs\nlabel](https://example.com/docs)\n[ref\nlabel][ref]\n\n[ref]: https://example.com/ref", theme)
	joined := strings.Join(md.Render(240), "\n")
	for _, want := range []string{
		"<b>bold\nline</b>",
		"<i>italic\nline</i>",
		"<b>under\nstrong</b>",
		"<i>under\nem</i>",
		"<s>gone\nline</s>",
		"<code>code span</code>",
		"<link>docs\nlabel</link><url> (https://example.com/docs)</url>",
		"<link>ref\nlabel</link><url> (https://example.com/ref)</url>",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("multiline paragraph markdown missing %q in %q", want, joined)
		}
	}
	if strings.Contains(joined, "[ref]:") {
		t.Fatalf("reference definition should not render as paragraph text: %q", joined)
	}
}

func TestMarkdownListMultilineParagraphTokensMatchMarked(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()
	theme := MarkdownTheme{
		Bold:    func(s string) string { return "<b>" + s + "</b>" },
		Code:    func(s string) string { return "<code>" + s + "</code>" },
		Link:    func(s string) string { return "<link>" + s + "</link>" },
		LinkURL: func(s string) string { return "<url>" + s + "</url>" },
	}
	md := NewMarkdown("- **bold\n  line** and `code\n  span`\n- [docs\n  label](https://example.com/docs)\n\n> - **quote\n>   item**", theme)
	joined := strings.Join(md.Render(240), "\n")
	for _, want := range []string{
		"- <b>bold\n  line</b> and <code>code span</code>",
		"- <link>docs\n  label</link><url> (https://example.com/docs)</url>",
		"│ - <b>quote\n│   item</b>",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("multiline list paragraph markdown missing %q in %q", want, joined)
		}
	}
}

func TestMarkdownNestedInlineFormattingMatchesPiTokens(t *testing.T) {
	theme := MarkdownTheme{
		Bold:          func(s string) string { return "<b>" + s + "</b>" },
		Italic:        func(s string) string { return "<i>" + s + "</i>" },
		Code:          func(s string) string { return "<code>" + s + "</code>" },
		Strikethrough: func(s string) string { return "<s>" + s + "</s>" },
	}
	md := NewMarkdown("**bold _italic_** ~~gone **bold**~~ __strong *em*__ _em `code`_ **asterisk *em*** *asterisk **strong*** **one** **two**", theme)
	joined := strings.Join(md.Render(240), "\n")
	for _, want := range []string{
		"<b>bold <i>italic</i></b>",
		"<s>gone <b>bold</b></s>",
		"<b>strong <i>em</i></b>",
		"<i>em <code>code</code></i>",
		"<b>asterisk <i>em</i></b>",
		"<i>asterisk <b>strong</b></i>",
		"<b>one</b> <b>two</b>",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("nested inline markdown missing %q in %q", want, joined)
		}
	}
}

func TestMarkdownTripleDelimiterStrongEmphasisMatchesMarked(t *testing.T) {
	theme := MarkdownTheme{
		Bold:   func(s string) string { return "<b>" + s + "</b>" },
		Italic: func(s string) string { return "<i>" + s + "</i>" },
	}
	md := NewMarkdown("***asterisk both*** and ___underscore both___", theme)
	joined := strings.Join(stripANSILines(md.Render(80)), "\n")
	for _, want := range []string{
		"<i><b>asterisk both</b></i>",
		"<i><b>underscore both</b></i>",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("triple delimiter markdown missing %q in %q", want, joined)
		}
	}
	for _, unwanted := range []string{"***asterisk", "both***", "___underscore", "both___"} {
		if strings.Contains(joined, unwanted) {
			t.Fatalf("triple delimiter syntax leaked %q in %q", unwanted, joined)
		}
	}
}

func TestMarkdownAsteriskMixedDelimiterRunsMatchMarked(t *testing.T) {
	theme := MarkdownTheme{
		Bold:   func(s string) string { return "<b>" + s + "</b>" },
		Italic: func(s string) string { return "<i>" + s + "</i>" },
	}
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{
			name: "triple opener closes strong then emphasis",
			text: "***foo** bar*",
			want: "<i><b>foo</b> bar</i>",
		},
		{
			name: "triple opener closes emphasis then strong",
			text: "***foo* bar**",
			want: "<b><i>foo</i> bar</b>",
		},
		{
			name: "emphasis wraps inner strong with triple closer",
			text: "*foo **bar***",
			want: "<i>foo <b>bar</b></i>",
		},
		{
			name: "strong wraps inner emphasis with triple closer",
			text: "**foo *bar***",
			want: "<b>foo <i>bar</i></b>",
		},
		{
			name: "inner strong without spaces inside emphasis",
			text: "*foo**bar**baz*",
			want: "<i>foo<b>bar</b>baz</i>",
		},
		{
			name: "inner emphasis without spaces inside strong",
			text: "**foo*bar*baz**",
			want: "<b>foo<i>bar</i>baz</b>",
		},
		{
			name: "emphasis closes at first star of double closer",
			text: "*em**",
			want: "<i>em</i>*",
		},
		{
			name: "second star opens emphasis when strong cannot close",
			text: "**bold*",
			want: "*<i>bold</i>",
		},
		{
			name: "triple opener leaves first star before strong",
			text: "***both**",
			want: "*<b>both</b>",
		},
		{
			name: "escaped star after strong opener leaves marked emphasis split",
			text: `**\**not open?**`,
			want: "***<i>not open?</i>*",
		},
		{
			name: "four asterisks nest strong in strong",
			text: "****four****",
			want: "<b><b>four</b></b>",
		},
		{
			name: "five asterisks wrap nested strong in emphasis",
			text: "*****five*****",
			want: "<i><b><b>five</b></b></i>",
		},
		{
			name: "strong closes before emphasis at triple middle run",
			text: "**foo***bar*",
			want: "<b>foo</b><i>bar</i>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			md := NewMarkdown(tc.text, theme)
			got := strings.Join(stripANSILines(md.Render(200)), "\n")
			if got != tc.want {
				t.Fatalf("rendered markdown = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMarkdownUnderscoreMixedDelimiterRunsMatchMarked(t *testing.T) {
	theme := MarkdownTheme{
		Bold:   func(s string) string { return "<b>" + s + "</b>" },
		Italic: func(s string) string { return "<i>" + s + "</i>" },
	}
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{
			name: "triple opener closes strong then emphasis",
			text: "___foo__ bar_",
			want: "<i><b>foo</b> bar</i>",
		},
		{
			name: "triple opener closes emphasis then strong",
			text: "___foo_ bar__",
			want: "<b><i>foo</i> bar</b>",
		},
		{
			name: "emphasis wraps inner strong with triple closer",
			text: "_foo __bar___",
			want: "<i>foo <b>bar</b></i>",
		},
		{
			name: "strong wraps inner emphasis with triple closer",
			text: "__foo _bar___",
			want: "<b>foo <i>bar</i></b>",
		},
		{
			name: "double opener leaves first underscore before emphasis",
			text: "__foo___bar_",
			want: "_<i>foo___bar</i>",
		},
		{
			name: "double closer leaves trailing underscore after emphasis",
			text: "_foo___bar__",
			want: "<i>foo___bar</i>_",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			md := NewMarkdown(tc.text, theme)
			got := strings.Join(stripANSILines(md.Render(200)), "\n")
			if got != tc.want {
				t.Fatalf("rendered markdown = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMarkdownCodeSpanBacktickRunsMatchMarked(t *testing.T) {
	theme := MarkdownTheme{Code: func(s string) string { return "<code>" + s + "</code>" }}
	md := NewMarkdown("Use ``code ` tick`` and ` single ` and \\`literal\\`", theme)
	plain := strings.Join(md.Render(240), "\n")
	for _, want := range []string{
		"<code>code ` tick</code>",
		"<code>single</code>",
		"`literal`",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("code span render missing %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "\\`literal") {
		t.Fatalf("escaped backtick should not keep escape slash: %q", plain)
	}
}

func TestMarkdownUnderscoreStrongAndEmphasis(t *testing.T) {
	theme := MarkdownTheme{
		Bold:   func(s string) string { return "<b>" + s + "</b>" },
		Italic: func(s string) string { return "<i>" + s + "</i>" },
	}
	md := NewMarkdown("__bold__ _italic_ foo_bar word__no__word __one__ __two__ _a_b_ __ spaced __", theme)
	joined := strings.Join(md.Render(200), "\n")
	for _, want := range []string{
		"<b>bold</b>",
		"<i>italic</i>",
		"<b>one</b> <b>two</b>",
		"<i>a_b</i>",
		"foo_bar",
		"word__no__word",
		"__ spaced __",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("underscore markdown missing %q in %q", want, joined)
		}
	}
	for _, unwanted := range []string{"<i>bar</i>", "<b>no</b>", "<b> spaced </b>"} {
		if strings.Contains(joined, unwanted) {
			t.Fatalf("underscore markdown styled invalid span %q in %q", unwanted, joined)
		}
	}
}

func TestMarkdownDelimiterFlankingMatchesMarked(t *testing.T) {
	theme := MarkdownTheme{
		Bold:   func(s string) string { return "<b>" + s + "</b>" },
		Italic: func(s string) string { return "<i>" + s + "</i>" },
	}
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{
			name: "asterisk opener blocked before punctuation after word",
			text: `a*"foo"*`,
			want: `a*"foo"*`,
		},
		{
			name: "asterisk strong opener blocked before punctuation after word",
			text: `a**"foo"**`,
			want: `a**"foo"**`,
		},
		{
			name: "asterisk opener before punctuation still works at boundary",
			text: `*("foo")*`,
			want: `<i>("foo")</i>`,
		},
		{
			name: "unmatched punctuation-bound delimiters stay literal",
			text: `*(*foo)`,
			want: `*(*foo)`,
		},
		{
			name: "intraword underscores stay literal",
			text: `foo_bar_ __foo__bar`,
			want: `foo_bar_ __foo__bar`,
		},
		{
			name: "underscore can open after punctuation before punctuation",
			text: `foo-_(bar)_ __foo__ bar`,
			want: `foo-<i>(bar)</i> <b>foo</b> bar`,
		},
		{
			name: "unicode symbols count as punctuation for underscores",
			text: `€_price_ and _cost_€`,
			want: `€<i>price</i> and <i>cost</i>€`,
		},
		{
			name: "unicode symbols block asterisk opener after word",
			text: `a*€foo*`,
			want: `a*€foo*`,
		},
		{
			name: "gfm tilde is not punctuation for emphasis",
			text: `a*~foo*`,
			want: `a<i>~foo</i>`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			md := NewMarkdown(tc.text, theme)
			got := strings.Join(stripANSILines(md.Render(200)), "\n")
			if got != tc.want {
				t.Fatalf("rendered markdown = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMarkdownStrikethroughUsesStrictDoubleTilde(t *testing.T) {
	theme := MarkdownTheme{Strikethrough: func(s string) string { return "\x1b[9m" + s + "\x1b[29m" }}
	md := NewMarkdown("Use ~~gone~~ but keep ~literal~ and ~~ spaced ~~", theme)
	lines := md.Render(120)
	joined := strings.Join(lines, "\n")
	plain := stripANSI(joined)

	if !strings.Contains(joined, "\x1b[9mgone\x1b[29m") {
		t.Fatalf("strict strike was not styled: %q", joined)
	}
	if !strings.Contains(plain, "~literal~") {
		t.Fatalf("single-tilde text should remain literal: %q", plain)
	}
	if !strings.Contains(plain, "~~ spaced ~~") {
		t.Fatalf("space-bounded double-tilde text should remain literal: %q", plain)
	}

	md = NewMarkdown(`Use ~~gone\~~~ and ~~\~lead~~`, theme)
	joined = strings.Join(md.Render(120), "\n")
	plain = stripANSI(joined)
	if !strings.Contains(joined, "\x1b[9mgone~\x1b[29m") || !strings.Contains(joined, "\x1b[9m~lead\x1b[29m") {
		t.Fatalf("strict strike should allow escaped boundary tildes like Pi marked tokenizer: raw=%q plain=%q", joined, plain)
	}
	if strings.Contains(plain, `\~`) || strings.Contains(plain, "~~gone") || strings.Contains(plain, "lead~~") {
		t.Fatalf("escaped strict strike boundary syntax leaked: raw=%q plain=%q", joined, plain)
	}

	md = NewMarkdown(`Use ~~trail\ ~~`, theme)
	joined = strings.Join(md.Render(120), "\n")
	plain = stripANSI(joined)
	if !strings.Contains(joined, "\x1b[9mtrail\\ \x1b[29m") {
		t.Fatalf("strict strike should allow escaped trailing whitespace like Pi tokenizer: raw=%q plain=%q", joined, plain)
	}
	if strings.Contains(plain, "~~trail") {
		t.Fatalf("escaped trailing whitespace strike delimiter leaked: raw=%q plain=%q", joined, plain)
	}
}

func TestMarkdownBackslashEscapesInlineMarkup(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()

	theme := MarkdownTheme{
		Bold:          func(s string) string { return "\x1b[1m" + s + "\x1b[22m" },
		Italic:        func(s string) string { return "\x1b[3m" + s + "\x1b[23m" },
		Strikethrough: func(s string) string { return "\x1b[9m" + s + "\x1b[29m" },
		Code:          func(s string) string { return "\x1b[33m" + s + "\x1b[0m" },
	}
	md := NewMarkdown(`\*literal\* \~~no strike~~ \[not link](https://example.com) \!`, theme)
	rendered := strings.Join(md.Render(160), "\n")
	plain := stripANSI(rendered)
	for _, want := range []string{"*literal*", "~~no strike~~", "[not link](https://example.com)", "!"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("escaped markdown missing %q in %q", want, plain)
		}
	}
	if strings.Contains(rendered, "\x1b[3m") || strings.Contains(rendered, "\x1b[9m") || strings.Contains(rendered, "\x1b]8;;") {
		t.Fatalf("escaped inline markup should not be styled as markdown: %q", rendered)
	}

	md = NewMarkdown("`\\*code\\*`", theme)
	if plain := stripANSI(strings.Join(md.Render(80), "\n")); plain != "\\*code\\*" {
		t.Fatalf("code span should preserve backslash escapes, got %q", plain)
	}
}

func TestMarkdownEscapedLinkAndImagePrefixesMatchMarked(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()

	theme := MarkdownTheme{
		Link:      func(s string) string { return "<link>" + s + "</link>" },
		LinkURL:   func(s string) string { return "<url>" + s + "</url>" },
		Underline: func(s string) string { return s },
	}
	md := NewMarkdown(`\[label](https://example.com "Title") \[before][ref] \![alt](https://example.com/img.png) \![reference alt][img] \![shortcut]

[ref]: https://example.com/ref
[img]: https://example.com/reference.png
[shortcut]: https://example.com/shortcut.png`, theme)
	plain := stripANSI(strings.Join(md.Render(400), "\n"))

	for _, want := range []string{
		`[label](<link>https://example.com</link> "Title")`,
		`[before]<link>ref</link><url> (https://example.com/ref)</url>`,
		`!<link>alt</link><url> (https://example.com/img.png)</url>`,
		`!<link>reference alt</link><url> (https://example.com/reference.png)</url>`,
		`!<link>shortcut</link><url> (https://example.com/shortcut.png)</url>`,
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("escaped link/image prefix marked parity missing %q in %q", want, plain)
		}
	}
	for _, unwanted := range []string{`<link>label</link>`, `![alt](`, `![reference alt][img]`, `[ref]:`, `[img]:`, `[shortcut]:`} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("escaped link/image prefix should not render as full literal or full direct link, found %q in %q", unwanted, plain)
		}
	}
}

func TestMarkdownBackslashEscapesAllMarkedPunctuation(t *testing.T) {
	escaped := `\! \" \# \$ \% \& \' \( \) \* \+ \, \- \. \/ \: \; \< \= \> \? \@ \[ \\ \] \^ \_ \` + "`" + ` \{ \| \} \~`
	md := NewMarkdown(escaped+` \a \1`, MarkdownTheme{})
	plain := stripANSI(strings.Join(md.Render(200), "\n"))
	want := `! " # $ % & ' ( ) * + , - . / : ; < = > ? @ [ \ ] ^ _ ` + "`" + ` { | } ~ \a \1`
	if plain != want {
		t.Fatalf("marked punctuation escapes = %q, want %q", plain, want)
	}
}

func TestMarkdownBackslashHardBreakDoesNotRenderMarker(t *testing.T) {
	md := NewMarkdown("alpha\\\nbeta\nspace  \nnext\n\n> quote\\\n> spaced  \n> next\n\n- item\\\n- spaced  \n- next", MarkdownTheme{})
	lines := stripANSILines(md.Render(80))
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	want := []string{
		"alpha",
		"beta",
		"space",
		"next",
		"",
		"│ quote",
		"│ spaced",
		"│ next",
		"",
		"- item",
		"- spaced",
		"- next",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("hard break marker render = %#v, want %#v", lines, want)
	}
}

func TestMarkdownHTMLEntitiesStayRawLikePiMarkedTokens(t *testing.T) {
	theme := MarkdownTheme{Code: func(s string) string { return "<code>" + s + "</code>" }}
	md := NewMarkdown("AT&amp;T &lt;tag&gt; &#35; &#x1F44D; `&amp;` **&lt;bold&gt;**", theme)
	plain := stripANSI(strings.Join(md.Render(240), "\n"))
	for _, want := range []string{"AT&amp;T", "&lt;tag&gt;", "&#35;", "&#x1F44D;", "<code>&amp;</code>", "&lt;bold&gt;"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("raw entity markdown missing %q in %q", want, plain)
		}
	}
	for _, decoded := range []string{"AT&T", "<tag>", "👍", "<bold>"} {
		if strings.Contains(plain, decoded) {
			t.Fatalf("text entities should stay raw like Pi marked tokens, found %q in %q", decoded, plain)
		}
	}
}

func TestMarkdownDefaultTextStyleRestoresAfterInlineCodeAndBold(t *testing.T) {
	theme := MarkdownTheme{
		Code:   func(s string) string { return "\x1b[33m" + s + "\x1b[0m" },
		Bold:   func(s string) string { return "\x1b[1m" + s + "\x1b[22m" },
		Italic: func(s string) string { return "\x1b[3m" + s + "\x1b[23m" },
	}
	md := NewMarkdownWithOptions("This is thinking with `inline code` and **bold text** after", MarkdownOptions{
		Theme:    theme,
		PaddingX: 1,
		DefaultTextStyle: &DefaultTextStyle{
			Color:  func(s string) string { return "\x1b[90m" + s + "\x1b[39m" },
			Italic: true,
		},
	})
	lines := md.Render(80)
	if len(lines) != 1 {
		t.Fatalf("styled markdown lines = %#v", lines)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"\x1b[90m", "\x1b[3m", "\x1b[33m", "\x1b[1m"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("styled markdown missing %q in %q", want, joined)
		}
	}
	if !strings.Contains(joined, "\x1b[0m\x1b[3m\x1b[90m") {
		t.Fatalf("default style should be restored after inline code reset: %q", joined)
	}
	if !strings.HasPrefix(stripANSI(lines[0]), " This is thinking") {
		t.Fatalf("padding should prefix rendered line: %q", stripANSI(lines[0]))
	}
	if VisibleWidth(stripANSI(lines[0])) != 80 {
		t.Fatalf("padded line width = %d, want 80: %q", VisibleWidth(stripANSI(lines[0])), stripANSI(lines[0]))
	}
}

func TestMarkdownDefaultTextStyleDoesNotLeakIntoFollowingTUILine(t *testing.T) {
	theme := MarkdownTheme{
		Code:   func(s string) string { return "\x1b[33m" + s + "\x1b[0m" },
		Bold:   func(s string) string { return "\x1b[1m" + s + "\x1b[22m" },
		Italic: func(s string) string { return "\x1b[3m" + s + "\x1b[23m" },
	}
	terminal := NewVirtualTerminal(80, 6)
	ui := NewTUI(terminal)
	ui.AddChild(NewMarkdownWithOptions("This is thinking with `inline code` and **bold text**", MarkdownOptions{
		Theme:    theme,
		PaddingX: 1,
		DefaultTextStyle: &DefaultTextStyle{
			Color:  func(s string) string { return "\x1b[90m" + s + "\x1b[39m" },
			Italic: true,
		},
	}))
	ui.AddChild(NewText("INPUT", 0, 0))
	ui.Start()
	defer ui.Stop()

	styledCell, ok := terminal.GetCell(0, 1)
	if !ok {
		t.Fatalf("missing styled markdown cell")
	}
	if !styledCell.Italic || styledCell.Foreground != (VirtualColor{Kind: "ansi", Index: 8}) {
		t.Fatalf("markdown default style was not applied before leak check: %#v", styledCell)
	}

	inputCell, ok := terminal.GetCell(1, 0)
	if !ok {
		t.Fatalf("missing following input cell")
	}
	if inputCell.Rune != 'I' {
		t.Fatalf("following line first cell = %q, want I", inputCell.Rune)
	}
	if inputCell.Bold || inputCell.Dim || inputCell.Italic || inputCell.Underline || inputCell.Inverse || inputCell.Strikethrough || inputCell.Blink || inputCell.Conceal || inputCell.Overline || inputCell.Foreground != (VirtualColor{}) || inputCell.Background != (VirtualColor{}) {
		t.Fatalf("markdown default style leaked into following TUI line: %#v", inputCell)
	}
}

func TestMarkdownHeadingStyleRestoresAfterInlineResets(t *testing.T) {
	theme := MarkdownTheme{
		Heading:   func(s string) string { return "\x1b[36m" + s + "\x1b[39m" },
		Bold:      func(s string) string { return "\x1b[1m" + s + "\x1b[22m" },
		Underline: func(s string) string { return "\x1b[4m" + s + "\x1b[24m" },
		Code:      func(s string) string { return "\x1b[33m" + s + "\x1b[0m" },
	}

	h1 := NewMarkdown("# Title with `code` inside", theme)
	h1Joined := strings.Join(h1.Render(80), "\n")
	afterCodeIndex := strings.Index(h1Joined, "inside")
	if afterCodeIndex < 0 {
		t.Fatalf("h1 output missing trailing text: %q", h1Joined)
	}
	h1Prefix := h1Joined[max(0, afterCodeIndex-40):afterCodeIndex]
	for _, want := range []string{"\x1b[36m", "\x1b[1m", "\x1b[4m"} {
		if !strings.Contains(h1Prefix, want) {
			t.Fatalf("h1 should restore %q after inline code, prefix=%q output=%q", want, h1Prefix, h1Joined)
		}
	}

	h2 := NewMarkdown("## Heading with **bold** and more", theme)
	h2Joined := strings.Join(h2.Render(80), "\n")
	afterBoldIndex := strings.Index(h2Joined, "and more")
	if afterBoldIndex < 0 {
		t.Fatalf("h2 output missing trailing text: %q", h2Joined)
	}
	h2Prefix := h2Joined[max(0, afterBoldIndex-40):afterBoldIndex]
	for _, want := range []string{"\x1b[36m", "\x1b[1m"} {
		if !strings.Contains(h2Prefix, want) {
			t.Fatalf("h2 should restore %q after bold reset, prefix=%q output=%q", want, h2Prefix, h2Joined)
		}
	}

	h3 := NewMarkdown("### Why `sourceInfo` should not be optional", theme)
	h3Joined := strings.Join(h3.Render(80), "\n")
	if plain := stripANSI(h3Joined); !strings.Contains(plain, "### Why sourceInfo should not be optional") {
		t.Fatalf("h3 should preserve visible marker and text, plain=%q raw=%q", plain, h3Joined)
	}
	if !strings.Contains(h3Joined, "\x1b[33msourceInfo") {
		t.Fatalf("h3 inline code should use code style: %q", h3Joined)
	}
	afterH3CodeIndex := strings.Index(h3Joined, "should not be optional")
	if afterH3CodeIndex < 0 {
		t.Fatalf("h3 output missing trailing text: %q", h3Joined)
	}
	h3Prefix := h3Joined[max(0, afterH3CodeIndex-40):afterH3CodeIndex]
	for _, want := range []string{"\x1b[36m", "\x1b[1m"} {
		if !strings.Contains(h3Prefix, want) {
			t.Fatalf("h3 should restore %q after inline code, prefix=%q output=%q", want, h3Prefix, h3Joined)
		}
	}
}

func TestMarkdownATXHeadingClosingSequenceIsRemoved(t *testing.T) {
	md := NewMarkdown("# Title ###\n\n> ## Quoted ##\n\n### Keep#literal", MarkdownTheme{})
	plain := strings.Join(stripANSILines(md.Render(80)), "\n")
	for _, want := range []string{"Title", "│ Quoted", "### Keep#literal"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("heading render missing %q in %q", want, plain)
		}
	}
	for _, unwanted := range []string{"Title ###", "Quoted ##"} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("closing heading marker should be removed: %q", plain)
		}
	}

	md = NewMarkdown("#\n\n###\n\n#\tTabbed\n\n####### not heading", MarkdownTheme{})
	plain = strings.Join(stripANSILines(md.Render(80)), "\n")
	for _, want := range []string{"###", "Tabbed", "####### not heading"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("empty/tabbed ATX heading render missing %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "# Tabbed") {
		t.Fatalf("tabbed ATX heading should consume marker: %q", plain)
	}
}

func TestMarkdownH1UnderlineDoesNotLeakIntoPadding(t *testing.T) {
	theme := MarkdownTheme{
		Heading:   func(s string) string { return "\x1b[36m" + s + "\x1b[39m" },
		Bold:      func(s string) string { return "\x1b[1m" + s + "\x1b[22m" },
		Underline: func(s string) string { return "\x1b[4m" + s + "\x1b[24m" },
		Code:      func(s string) string { return "\x1b[33m" + s + "\x1b[0m" },
	}
	md := NewMarkdownWithOptions("# Important distinction from `open()`", MarkdownOptions{
		Theme:    theme,
		PaddingX: 1,
	})
	terminal := NewVirtualTerminal(80, 4)
	ui := NewTUI(terminal)
	ui.AddChild(md)
	ui.Start()
	defer ui.Stop()

	rendered := md.Render(80)
	if len(rendered) == 0 {
		t.Fatalf("missing heading render")
	}
	contentWidth := len(strings.TrimRight(stripANSI(rendered[0]), " "))
	if contentWidth <= 0 {
		t.Fatalf("heading content has zero width: %#v", rendered)
	}
	for col := contentWidth; col < 80; col++ {
		cell, ok := terminal.GetCell(0, col)
		if !ok {
			t.Fatalf("missing cell at col %d", col)
		}
		if cell.Underline {
			t.Fatalf("underline leaked into padding at col %d: %#v", col, cell)
		}
	}
}

func TestMarkdownHyperlinksAndBareURLs(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: true})
	defer ResetCapabilitiesCache()
	md := NewMarkdown("[docs](https://example.com)", MarkdownTheme{})
	joined := strings.Join(md.Render(120), "\n")
	if !strings.Contains(joined, "\x1b]8;;https://example.com\x1b\\docs\x1b]8;;\x1b\\") {
		t.Fatalf("missing OSC 8 hyperlink: %q", joined)
	}
	if strings.Contains(joined, "(https://example.com)") {
		t.Fatalf("hyperlink should not duplicate URL as text: %q", joined)
	}
	md = NewMarkdown("Visit https://example.com, then continue.", MarkdownTheme{})
	joined = strings.Join(md.Render(120), "\n")
	if !strings.Contains(joined, "\x1b]8;;https://example.com\x1b\\https://example.com\x1b]8;;\x1b\\,") {
		t.Fatalf("bare URL hyperlink should exclude trailing comma: %q", joined)
	}
	if strings.Contains(joined, "\x1b]8;;https://example.com,\x1b\\") {
		t.Fatalf("trailing comma should not be part of OSC 8 URL: %q", joined)
	}
	md = NewMarkdown("Visit https://example.com/a_(b)) after.", MarkdownTheme{})
	joined = strings.Join(md.Render(120), "\n")
	if !strings.Contains(joined, "\x1b]8;;https://example.com/a_(b)\x1b\\https://example.com/a_(b)\x1b]8;;\x1b\\)") {
		t.Fatalf("bare URL hyperlink should keep balanced parens and exclude extra closing paren: %q", joined)
	}
	if strings.Contains(joined, "\x1b]8;;https://example.com/a_(b))\x1b\\") {
		t.Fatalf("extra closing paren should not be part of OSC 8 URL: %q", joined)
	}
	md = NewMarkdown("Visit HTTPS://EXAMPLE.COM/Path.", MarkdownTheme{})
	joined = strings.Join(md.Render(120), "\n")
	if !strings.Contains(joined, "\x1b]8;;HTTPS://EXAMPLE.COM/Path\x1b\\HTTPS://EXAMPLE.COM/Path\x1b]8;;\x1b\\.") {
		t.Fatalf("bare URL hyperlink should match uppercase schemes and exclude trailing period: %q", joined)
	}
	md = NewMarkdown("Visit https://example.com?a=1&amp;b=2.", MarkdownTheme{})
	joined = strings.Join(md.Render(120), "\n")
	if !strings.Contains(joined, "\x1b]8;;https://example.com?a=1&amp;b=2\x1b\\https://example.com?a=1&amp;b=2\x1b]8;;\x1b\\.") {
		t.Fatalf("bare URL hyperlink should keep href entities raw and exclude trailing period: %q", joined)
	}
	md = NewMarkdown("Visit https://example.com?a=1&copy; and https://example.org?a=1&b=2&amp;.", MarkdownTheme{})
	joined = strings.Join(md.Render(160), "\n")
	if !strings.Contains(joined, "\x1b]8;;https://example.com?a=1\x1b\\https://example.com?a=1\x1b]8;;\x1b\\") {
		t.Fatalf("named entity suffix should be excluded from marked-style bare URL: %q", joined)
	}
	if !strings.Contains(joined, "\x1b]8;;https://example.org?a=1&b=2\x1b\\https://example.org?a=1&b=2\x1b]8;;\x1b\\") {
		t.Fatalf("trailing amp entity should be excluded after preserving query params: %q", joined)
	}
	if strings.Contains(joined, "\x1b]8;;https://example.com?a=1©") || strings.Contains(joined, "\x1b]8;;https://example.com?a=1&copy;") || strings.Contains(joined, "\x1b]8;;https://example.org?a=1&b=2&") {
		t.Fatalf("bare URL OSC 8 href should not include marked-style trailing entity suffixes: %q", joined)
	}
	md = NewMarkdown(`Quote "https://example.com/path" and emphasize https://example.org/end*`, MarkdownTheme{})
	joined = strings.Join(md.Render(160), "\n")
	if !strings.Contains(joined, "\x1b]8;;https://example.com/path\x1b\\https://example.com/path\x1b]8;;\x1b\\\"") {
		t.Fatalf("bare URL hyperlink should exclude trailing quote: %q", joined)
	}
	if !strings.Contains(joined, "\x1b]8;;https://example.org/end\x1b\\https://example.org/end\x1b]8;;\x1b\\*") {
		t.Fatalf("bare URL hyperlink should exclude trailing emphasis punctuation: %q", joined)
	}
	if strings.Contains(joined, "\x1b]8;;https://example.com/path\"\x1b\\") || strings.Contains(joined, "\x1b]8;;https://example.org/end*\x1b\\") {
		t.Fatalf("bare URL OSC 8 href should not include marked-style trailing punctuation: %q", joined)
	}

	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	md = NewMarkdown("See https://example.com", MarkdownTheme{})
	plain := strings.Join(md.Render(120), "\n")
	if strings.Count(plain, "https://example.com") != 1 {
		t.Fatalf("bare URL should be rendered once, got %q", plain)
	}
}

func TestMarkdownLinkFallbackURLsWhenHyperlinksUnsupported(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()

	md := NewMarkdown("[click here](https://example.com)", MarkdownTheme{})
	plain := stripANSI(strings.Join(md.Render(120), " "))
	if !strings.Contains(plain, "click here") || !strings.Contains(plain, "(https://example.com)") {
		t.Fatalf("direct link fallback should show label and URL, got %q", plain)
	}

	md = NewMarkdown("[Email me](mailto:test@example.com)", MarkdownTheme{})
	plain = stripANSI(strings.Join(md.Render(120), " "))
	if !strings.Contains(plain, "Email me") || !strings.Contains(plain, "(mailto:test@example.com)") {
		t.Fatalf("mailto link fallback should show label and URL, got %q", plain)
	}
	if strings.Count(plain, "test@example.com") != 1 {
		t.Fatalf("mailto fallback should not duplicate the email address, got %q", plain)
	}

	md = NewMarkdown("Contact user@example.com or visit https://example.com", MarkdownTheme{})
	plain = stripANSI(strings.Join(md.Render(120), " "))
	if strings.Count(plain, "user@example.com") != 1 || strings.Contains(plain, "mailto:user@example.com") {
		t.Fatalf("bare email fallback should render once without mailto prefix, got %q", plain)
	}
	if strings.Count(plain, "https://example.com") != 1 {
		t.Fatalf("bare URL fallback should render once, got %q", plain)
	}
}

func TestMarkdownMarkedBareURLSchemesAndWWW(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: true})
	defer ResetCapabilitiesCache()

	md := NewMarkdown("Download ftp://example.com/file, visit www.example.com/a_(b)), and email user@www.example.com.", MarkdownTheme{})
	joined := strings.Join(md.Render(160), "\n")
	if !strings.Contains(joined, "\x1b]8;;ftp://example.com/file\x1b\\ftp://example.com/file\x1b]8;;\x1b\\,") {
		t.Fatalf("FTP bare URL should use OSC 8 and exclude trailing comma: %q", joined)
	}
	if !strings.Contains(joined, "\x1b]8;;http://www.example.com/a_(b)\x1b\\www.example.com/a_(b)\x1b]8;;\x1b\\)") {
		t.Fatalf("www bare URL should use http href and exclude unmatched closing paren: %q", joined)
	}
	if strings.Contains(joined, "http://user@www.example.com") || strings.Contains(joined, "\x1b]8;;http://www.example.com.\x1b\\") {
		t.Fatalf("bare URL matcher should not link inside emails or include trailing punctuation: %q", joined)
	}
	if !strings.Contains(joined, "\x1b]8;;mailto:user@www.example.com\x1b\\user@www.example.com\x1b]8;;\x1b\\") {
		t.Fatalf("email with www host should remain a mailto link: %q", joined)
	}
	md = NewMarkdown("Marked prefixes abchttps://example.com, abcwww.example.com, @www.example.org, and foo@https://example.net.", MarkdownTheme{})
	joined = strings.Join(md.Render(220), "\n")
	for _, want := range []string{
		"\x1b]8;;https://example.com\x1b\\https://example.com\x1b]8;;\x1b\\",
		"\x1b]8;;http://www.example.com\x1b\\www.example.com\x1b]8;;\x1b\\",
		"\x1b]8;;http://www.example.org\x1b\\www.example.org\x1b]8;;\x1b\\",
		"\x1b]8;;https://example.net\x1b\\https://example.net\x1b]8;;\x1b\\.",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("marked-style bare URL should link after ordinary prefixes, missing %q in %q", want, joined)
		}
	}

	md = NewMarkdown("Invalid https://?q=1, https:///path, and www.?q=1 stay literal, but https://- links.", MarkdownTheme{})
	joined = strings.Join(md.Render(180), "\n")
	for _, invalid := range []string{"https://?q=1", "https:///path", "www.?q=1"} {
		if strings.Contains(joined, "\x1b]8;;"+invalid) {
			t.Fatalf("marked-invalid bare URL %q should not become an OSC 8 link: %q", invalid, joined)
		}
	}
	if !strings.Contains(joined, "\x1b]8;;https://-\x1b\\https://-\x1b]8;;\x1b\\") {
		t.Fatalf("marked-valid hyphen host bare URL should still link: %q", joined)
	}

	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	md = NewMarkdown("Visit www.example.com/path and FTP://EXAMPLE.COM/Path.", MarkdownTheme{})
	plain := stripANSI(strings.Join(md.Render(160), " "))
	if !strings.Contains(plain, "www.example.com/path (http://www.example.com/path)") {
		t.Fatalf("www fallback should show marked-style http href: %q", plain)
	}
	if strings.Count(plain, "FTP://EXAMPLE.COM/Path") != 1 {
		t.Fatalf("FTP fallback should render once when label equals href: %q", plain)
	}
}

func TestMarkdownLinksApplyUnderlineInsideLinkStyle(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()
	theme := MarkdownTheme{
		Bold:      func(s string) string { return "<b>" + s + "</b>" },
		Link:      func(s string) string { return "<link>" + s + "</link>" },
		LinkURL:   func(s string) string { return "<url>" + s + "</url>" },
		Underline: func(s string) string { return "<u>" + s + "</u>" },
	}
	md := NewMarkdown("[docs](https://example.com) and [**bold docs**][ref]\n\n[ref]: https://example.com/ref", theme)
	joined := strings.Join(md.Render(240), "\n")
	for _, want := range []string{
		"<link><u>docs</u></link><url> (https://example.com)</url>",
		"<link><u><b>bold docs</b></u></link><url> (https://example.com/ref)</url>",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("link should apply nested inline style and underline: missing %q in %q", want, joined)
		}
	}
}

func TestMarkdownReferenceDefinitionOnlyRendersBlankLineLikePi(t *testing.T) {
	md := NewMarkdown("[ref]: https://example.com/ref", MarkdownTheme{})
	if lines := md.Render(80); len(lines) != 1 || lines[0] != "" {
		t.Fatalf("definition-only markdown should render one blank line, got %#v", lines)
	}

	md = NewMarkdownWithOptions("[ref]: https://example.com/ref", MarkdownOptions{
		PaddingY: 1,
		DefaultTextStyle: &DefaultTextStyle{
			BgColor: func(s string) string { return "<bg>" + s + "</bg>" },
		},
	})
	lines := md.Render(8)
	if len(lines) != 2 || !strings.Contains(lines[0], "<bg>") || !strings.Contains(lines[1], "<bg>") {
		t.Fatalf("definition-only markdown should keep vertical padding only, got %#v", lines)
	}
}

func TestMarkdownTrailingSpaceTokensMatchPi(t *testing.T) {
	lines := NewMarkdown("hello\n\n", MarkdownTheme{}).Render(80)
	if len(lines) != 2 || stripANSI(lines[0]) != "hello" || lines[1] != "" {
		t.Fatalf("explicit trailing markdown blank should render as Pi space token, got %#v", lines)
	}

	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()
	theme := MarkdownTheme{
		Link:    func(s string) string { return "<link>" + s + "</link>" },
		LinkURL: func(s string) string { return "<url>" + s + "</url>" },
	}
	md := NewMarkdown("[ref][r]\n\n[r]: https://example.com \"title\"", theme)
	lines = md.Render(80)
	joined := strings.Join(lines, "\n")
	if len(lines) != 2 || !strings.Contains(joined, "<link>ref</link><url> (https://example.com)</url>") || lines[1] != "" {
		t.Fatalf("hidden reference definition after a blank should preserve Pi trailing blank, got %#v", lines)
	}
}

func TestMarkdownDocumentKeepsGoldmarkASTBehindPiRenderer(t *testing.T) {
	document := parseMarkdownDocument([]string{
		"# Heading",
		"",
		"- item",
		"  [ref]: https://first.example/list",
		"",
		"[ref]: https://second.example/list",
		"",
		"[ref]",
	})
	if document.Root == nil {
		t.Fatal("markdown document should expose a goldmark AST root")
	}
	if document.Context == nil || len(document.Context.References()) == 0 {
		t.Fatal("markdown document should retain goldmark parser context")
	}
	if got := document.LinkDefinitions[normalizeMarkdownReference("ref")]; got != "https://second.example/list" {
		t.Fatalf("Pi-compatible link definitions should ignore list-contained definitions, got %q", got)
	}
}

func TestMarkdownDocumentGoldmarkASTUsesGFM(t *testing.T) {
	document := parseMarkdownDocument([]string{
		"- [x] done",
		"",
		"| A | B |",
		"| --- | --- |",
		"| ~~gone~~ | ok |",
	})
	for _, kind := range []goldmarkast.NodeKind{
		extensionast.KindTaskCheckBox,
		extensionast.KindTable,
		extensionast.KindStrikethrough,
	} {
		if !markdownDocumentHasNodeKind(document.Root, kind) {
			t.Fatalf("goldmark GFM AST should contain %s", kind)
		}
	}
}

func markdownDocumentHasNodeKind(root goldmarkast.Node, kind goldmarkast.NodeKind) bool {
	found := false
	_ = goldmarkast.Walk(root, func(node goldmarkast.Node, entering bool) (goldmarkast.WalkStatus, error) {
		if entering && node.Kind() == kind {
			found = true
			return goldmarkast.WalkStop, nil
		}
		return goldmarkast.WalkContinue, nil
	})
	return found
}

func TestMarkdownReferenceDefinitionsInsideRawBlocksDoNotResolveLikeMarked(t *testing.T) {
	theme := MarkdownTheme{
		Link:    func(s string) string { return "<link>" + s + "</link>" },
		LinkURL: func(s string) string { return "<url>" + s + "</url>" },
	}
	for _, tc := range []struct {
		name string
		text string
	}{
		{
			name: "fenced code",
			text: "```\n[ref]: https://bad.example/fence\n```\n\n[ref]",
		},
		{
			name: "list fenced code",
			text: "- item\n  ```\n  [ref]: https://bad.example/list-fence\n  ```\n\n[ref]",
		},
		{
			name: "nested list fenced code",
			text: "- outer\n  - inner\n    ```\n    [ref]: https://bad.example/nested-list-fence\n    ```\n\n[ref]",
		},
		{
			name: "html block",
			text: "<div>\n[ref]: https://bad.example/html\n</div>\n\n[ref]",
		},
		{
			name: "nested list html block",
			text: "- outer\n  - inner\n    <div>\n    [ref]: https://bad.example/nested-list-html\n    </div>\n\n[ref]",
		},
		{
			name: "blockquote html block",
			text: "> <div>\n> [ref]: https://bad.example/quote-html\n> </div>\n\n[ref]",
		},
		{
			name: "blockquote nested list fenced code",
			text: "> - outer\n>   - inner\n>     ```\n>     [ref]: https://bad.example/quote-nested-list-fence\n>     ```\n\n[ref]",
		},
		{
			name: "blockquote nested list html block",
			text: "> - outer\n>   - inner\n>     <div>\n>     [ref]: https://bad.example/quote-nested-list-html\n>     </div>\n\n[ref]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			md := NewMarkdown(tc.text, theme)
			plain := stripANSI(strings.Join(md.Render(120), "\n"))
			if strings.Contains(plain, "<link>ref</link>") || (strings.Contains(plain, "https://bad.example") && strings.Contains(plain, "<url>")) {
				t.Fatalf("definition inside raw block should not resolve shortcut reference, got %q", plain)
			}
			if !strings.Contains(plain, "[ref]") {
				t.Fatalf("unresolved shortcut reference should stay literal, got %q", plain)
			}
		})
	}
}

func TestMarkdownReferenceDefinitionsFirstWinsLikeMarked(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()
	theme := MarkdownTheme{
		Link:    func(s string) string { return "<link>" + s + "</link>" },
		LinkURL: func(s string) string { return "<url>" + s + "</url>" },
	}
	for _, tc := range []struct {
		name   string
		text   string
		first  string
		second string
	}{
		{
			name:   "top-level normalized duplicates",
			text:   "[ref]: https://first.example/top\n[REF]: https://second.example/top\n\n[ref] [REF]",
			first:  "https://first.example/top",
			second: "https://second.example/top",
		},
		{
			name:   "blockquote before top-level",
			text:   "> [ref]: https://first.example/quote\n\n[ref]: https://second.example/quote\n\n[ref]",
			first:  "https://first.example/quote",
			second: "https://second.example/quote",
		},
		{
			name:   "top-level before blockquote",
			text:   "[ref]: https://first.example/top-quote\n\n> [ref]: https://second.example/top-quote\n\n[ref]",
			first:  "https://first.example/top-quote",
			second: "https://second.example/top-quote",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			md := NewMarkdown(tc.text, theme)
			plain := stripANSI(strings.Join(md.Render(120), "\n"))
			if !strings.Contains(plain, "<url> ("+tc.first+")</url>") {
				t.Fatalf("first reference definition should win, missing %q in %q", tc.first, plain)
			}
			if strings.Contains(plain, tc.second) {
				t.Fatalf("later duplicate reference definition should not override first, got %q", plain)
			}
		})
	}
}

func TestMarkdownMultilineReferenceDefinitionsMatchMarked(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()

	theme := MarkdownTheme{
		Link:    func(s string) string { return "<link>" + s + "</link>" },
		LinkURL: func(s string) string { return "<url>" + s + "</url>" },
	}
	md := NewMarkdown("[multi\nlabel]: https://example.com/multi\n\n[next\nline]:\nhttps://example.com/next\n\"Title\"\n\n- item\n  [list\n  ref]: https://example.com/list\n\n> [quote\n> ref]: https://example.com/quote\n>\n> [quote ref]\n>\n> - item\n>   [quote list\n>   ref]: https://example.com/quote-list\n\n[multi label] [list ref]\n[next line] [quote list ref]", theme)
	plain := stripANSI(strings.Join(md.Render(160), "\n"))
	for _, want := range []string{
		"<link>multi label</link><url> (https://example.com/multi)</url>",
		"<link>next line</link><url> (https://example.com/next)</url>",
		"│ <link>quote ref</link><url> (https://example.com/quote)</url>",
		"[list ref]",
		"[quote list ref]",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("multiline reference definition missing %q in %q", want, plain)
		}
	}
	for _, unwanted := range []string{"[multi", "label]: https://example.com/multi", "[next", "line]:", "\"Title\"", "[quote\n", "ref]: https://example.com/quote"} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("multiline reference definition leaked %q into render: %q", unwanted, plain)
		}
	}
	for _, url := range []string{"https://example.com/multi", "https://example.com/next", "https://example.com/quote"} {
		if count := strings.Count(plain, "("+url+")</url>"); count != 1 {
			t.Fatalf("multiline reference definition URL %q rendered %d times in %q", url, count, plain)
		}
	}
}

func TestMarkdownDirectLinksIgnoreOptionalTitles(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()
	theme := MarkdownTheme{
		Link:    func(s string) string { return "<link>" + s + "</link>" },
		LinkURL: func(s string) string { return "<url>" + s + "</url>" },
	}
	md := NewMarkdown(`[plain](https://example.com "Title") [angle](<https://example.com/space path> 'Title') [angle-escaped](<https://example.com/a\>b>) [paren-title](https://example.com/paren (Paren title)) [escaped-title](https://example.com/e "a\"b") [escaped-paren-title](https://example.com/p (a\)b)) [invalid-title](https://example.com invalid title) [bad-quote](https://example.com/bad "a"b") [bad-paren-title](https://example.com/badp (a)b)) [paren](https://example.com/a_(b)) [escaped](https://example.com/a\)b) [outer [inner]](https://example.com/nested) [quoted](https://example.com/q "Title ) with paren") [single](https://example.com/s 'Title ) single') [query](https://example.com?a=1&amp;b=2)`, theme)
	joined := strings.Join(md.Render(2000), "\n")
	plain := stripANSI(joined)
	for _, want := range []string{
		"<link>plain</link><url> (https://example.com)</url>",
		"<link>angle</link><url> (https://example.com/space path)</url>",
		"<link>angle-escaped</link><url> (https://example.com/a>b)</url>",
		"<link>paren-title</link><url> (https://example.com/paren)</url>",
		"<link>escaped-title</link><url> (https://example.com/e)</url>",
		"<link>escaped-paren-title</link><url> (https://example.com/p)</url>",
		"<link>paren</link><url> (https://example.com/a_(b))</url>",
		"<link>escaped</link><url> (https://example.com/a)b)</url>",
		"<link>outer [inner]</link><url> (https://example.com/nested)</url>",
		"<link>quoted</link><url> (https://example.com/q)</url>",
		"<link>single</link><url> (https://example.com/s)</url>",
		"<link>query</link><url> (https://example.com?a=1&amp;b=2)</url>",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("direct link with title missing %q in %q", want, joined)
		}
	}
	if !strings.Contains(plain, "[invalid-title](") || !strings.Contains(plain, "invalid title)") || strings.Contains(joined, "<link>invalid-title</link><url>") {
		t.Fatalf("invalid unquoted title should not parse as direct link, got %q", joined)
	}
	if !strings.Contains(plain, "[bad-quote](") || !strings.Contains(plain, `[bad-paren-title](`) || strings.Contains(joined, "<link>bad-quote</link><url>") || strings.Contains(joined, "<link>bad-paren-title</link><url>") {
		t.Fatalf("invalid unescaped title delimiters should not parse as direct links, got %q", joined)
	}
	for _, unwanted := range []string{"Title", "Paren title", "<https://", "[plain](", "[paren](", "with paren", "single'", "\\>"} {
		if strings.Contains(joined, unwanted) {
			t.Fatalf("direct link title or markdown syntax leaked %q in %q", unwanted, joined)
		}
	}

	md = NewMarkdown("[empty]() [angle-empty](<>) [empty-ref][empty]\n\n[empty]: <>", theme)
	joined = strings.Join(md.Render(160), "\n")
	for _, want := range []string{
		"<link>empty</link><url> ()</url>",
		"<link>angle-empty</link><url> ()</url>",
		"<link>empty-ref</link><url> ()</url>",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("empty link target missing %q in %q", want, joined)
		}
	}
	if strings.Contains(stripANSI(joined), "[empty]:") || strings.Contains(stripANSI(joined), "[empty]()") {
		t.Fatalf("empty link targets should parse as links, got %q", joined)
	}
}

func TestMarkdownNestedInlineTokensInsideLinksRestoreLikeMarked(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()
	theme := MarkdownTheme{
		Bold:      func(s string) string { return "<b>" + s + "</b>" },
		Code:      func(s string) string { return "<code>" + s + "</code>" },
		Link:      func(s string) string { return "<link>" + s + "</link>" },
		LinkURL:   func(s string) string { return "<url>" + s + "</url>" },
		Underline: func(s string) string { return s },
	}

	md := NewMarkdown("[<span title=\"&amp;\">**bold** `code`</span>](https://example.com/nested)", theme)
	joined := strings.Join(md.Render(200), "\n")
	plain := stripANSI(joined)
	want := `<link><span title="&amp;"><b>bold</b> <code>code</code></span></link><url> (https://example.com/nested)</url>`
	if !strings.Contains(plain, want) {
		t.Fatalf("nested inline tokens inside link label missing %q in %q", want, plain)
	}
	if strings.Contains(plain, "\x00md") || strings.Contains(plain, "&amp;amp;") || strings.Contains(plain, `title="&">`) {
		t.Fatalf("nested inline token stash should be fully restored without decoding HTML attributes, got %q", plain)
	}

	md = NewMarkdown("[![**raw alt**](image.png)](https://example.com/image)", theme)
	joined = strings.Join(md.Render(200), "\n")
	plain = stripANSI(joined)
	want = `<link>**raw alt**</link><url> (https://example.com/image)</url>`
	if !strings.Contains(plain, want) {
		t.Fatalf("image alt inside link label should render as literal link text, missing %q in %q", want, plain)
	}
	if strings.Contains(plain, "\x00md") || strings.Contains(plain, "<b>raw alt</b>") {
		t.Fatalf("image alt inside link label should not leak stash or parse alt markdown, got %q", plain)
	}

	md = NewMarkdown("[<span title=\"&amp;\">**ref** `code`</span>][ref]\n\n[ref]: https://example.com/ref", theme)
	joined = strings.Join(md.Render(200), "\n")
	plain = stripANSI(joined)
	want = `<link><span title="&amp;"><b>ref</b> <code>code</code></span></link><url> (https://example.com/ref)</url>`
	if !strings.Contains(plain, want) {
		t.Fatalf("nested inline tokens inside reference link label missing %q in %q", want, plain)
	}
	if strings.Contains(plain, "\x00md") || strings.Contains(plain, `title="&">`) || strings.Contains(plain, "[ref]:") {
		t.Fatalf("reference link label should restore nested tokens and hide definition, got %q", plain)
	}

	md = NewMarkdown("[![**raw ref alt**][img]][ref]\n\n[img]: image.png\n[ref]: https://example.com/ref-image", theme)
	joined = strings.Join(md.Render(200), "\n")
	plain = stripANSI(joined)
	want = `<link>**raw ref alt**</link><url> (https://example.com/ref-image)</url>`
	if !strings.Contains(plain, want) {
		t.Fatalf("reference image alt inside reference link label missing %q in %q", want, plain)
	}
	if strings.Contains(plain, "\x00md") || strings.Contains(plain, "<b>raw ref alt</b>") || strings.Contains(plain, "[img]:") || strings.Contains(plain, "[ref]:") {
		t.Fatalf("reference image alt inside reference link label should stay literal without leaked definitions, got %q", plain)
	}
}

func TestMarkdownNestedLinksInsideLinkLabelsMatchMarked(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()

	theme := MarkdownTheme{
		Bold:      func(s string) string { return "<b>" + s + "</b>" },
		Link:      func(s string) string { return "<link>" + s + "</link>" },
		LinkURL:   func(s string) string { return "<url>" + s + "</url>" },
		Underline: func(s string) string { return s },
	}
	md := NewMarkdown("[outer [inner](https://inner.example)](https://outer.example) [**bold [ref inner][inner-ref]**][outer-ref]\n\n[inner-ref]: https://inner-ref.example\n[outer-ref]: https://outer-ref.example", theme)
	plain := stripANSI(strings.Join(md.Render(500), "\n"))
	for _, want := range []string{
		"<link>outer <link>inner</link><url> (https://inner.example)</url></link><url> (https://outer.example)</url>",
		"<link><b>bold <link>ref inner</link><url> (https://inner-ref.example)</url></b></link><url> (https://outer-ref.example)</url>",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("nested link token inside link label missing %q in %q", want, plain)
		}
	}
	for _, unwanted := range []string{"[inner](https://inner.example)", "[ref inner][inner-ref]", "[inner-ref]:", "[outer-ref]:"} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("nested link syntax or definitions should not remain literal, found %q in %q", unwanted, plain)
		}
	}

	md = NewMarkdown(`[a [b \] c](https://inner.example)](https://outer.example)`, theme)
	plain = stripANSI(strings.Join(md.Render(500), "\n"))
	want := "<link>a [b ] c](https://inner.example)</link><url> (https://outer.example)</url>"
	if !strings.Contains(plain, want) {
		t.Fatalf("escaped closing bracket inside outer link label should keep inner candidate literal like marked, missing %q in %q", want, plain)
	}
	if strings.Contains(plain, "<link>b ] c</link><url> (https://inner.example)</url>") {
		t.Fatalf("escaped closing bracket label candidate should not become nested link: %q", plain)
	}
}

func TestMarkdownReferenceLinks(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()
	theme := MarkdownTheme{
		Link:    func(s string) string { return "<link>" + s + "</link>" },
		LinkURL: func(s string) string { return "<url>" + s + "</url>" },
	}
	md := NewMarkdown("[docs][ref] and [Guide][] and [Shortcut] and [outer [inner]][nested] and [outer [inner]][] and [outer [shortcut]] and [entity][ent] and [escaped][esc] and [escaped angle][angle-esc] and [escaped label][a*b] and [entity label][a&b] and [invalid][bad-title] and [missing][nope]\n\n[ref]: https://example.com/docs\n[Guide]: <https://example.com/guide> \"Guide title\"\n[Shortcut]: https://example.com/shortcut\n[nested]: https://example.com/nested\n[outer [inner]]: https://example.com/collapsed-nested\n[outer [shortcut]]: https://example.com/shortcut-nested\n[ent]: https://example.com?a=1&amp;b=2\n[esc]: https://example.com/a\\*b\n[angle-esc]: <https://example.com/a\\>b>\n[a\\*b]: https://example.com/escaped-label\n[a&amp;b]: https://example.com/entity-label\n[bad-title]: https://example.com/bad invalid title", theme)
	joined := strings.Join(md.Render(1000), "\n")
	plain := stripANSI(joined)

	for _, want := range []string{
		"<link>docs</link><url> (https://example.com/docs)</url>",
		"<link>Guide</link><url> (https://example.com/guide)</url>",
		"<link>Shortcut</link><url> (https://example.com/shortcut)</url>",
		"<link>outer [inner]</link><url> (https://example.com/nested)</url>",
		"[outer [inner]][]",
		"[outer <link>shortcut</link><url> (https://example.com/shortcut)</url>]",
		"[outer [inner]]: <link>https://example.com/collapsed-nested</link>",
		"[outer <link>shortcut</link><url> (https://example.com/shortcut)</url>]: <link>https://example.com/shortcut-nested</link>",
		"[entity][ent]",
		"[escaped][esc]",
		"[escaped angle][angle-esc]",
		"[ent]: <link>https://example.com?a=1&amp;b=2</link>",
		`[esc]: <link>https://example.com/a\*b</link>`,
		`[angle-esc]: <link>https://example.com/a\</link>b>`,
		"[escaped label][a*b]",
		"[entity label][a&b]",
		"[invalid][bad-title]",
		"[missing][nope]",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("reference link render missing %q in %q", want, joined)
		}
	}
	if !strings.Contains(plain, "[bad-title]:") || !strings.Contains(plain, "invalid title") || strings.Contains(joined, "<link>invalid</link><url>") {
		t.Fatalf("invalid unquoted title should not parse as reference definition, got %q", joined)
	}
	for _, unwanted := range []string{"[ref]:", "[Guide]:", "[Shortcut]:", "[nested]:", "Guide title", "\\>"} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("reference definition leaked %q in %q", unwanted, plain)
		}
	}

	md = NewMarkdown("[docs][multi]\n\n[multi]: https://example.com/multi\n  \"Multiline title\"", theme)
	joined = strings.Join(md.Render(120), "\n")
	plain = stripANSI(joined)
	if !strings.Contains(joined, "<link>docs</link><url> (https://example.com/multi)</url>") {
		t.Fatalf("multiline-title reference link missing in %q", joined)
	}
	for _, unwanted := range []string{"[multi]:", "Multiline title"} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("multiline reference definition leaked %q in %q", unwanted, plain)
		}
	}

	md = NewMarkdown("[valid title][valid-title] [bad quote title][bad-quote-title]\n\n[valid-title]: https://example.com/valid \"a\\\"b\"\n[bad-quote-title]: https://example.com/bad \"a\"b\"", theme)
	joined = strings.Join(md.Render(160), "\n")
	plain = stripANSI(joined)
	if !strings.Contains(joined, "<link>valid title</link><url> (https://example.com/valid)</url>") {
		t.Fatalf("escaped title delimiter reference definition should parse: %q", joined)
	}
	if !strings.Contains(plain, "[bad quote title][bad-quote-title]") || !strings.Contains(plain, "[bad-quote-title]:") || strings.Contains(joined, "<link>bad quote title</link><url>") {
		t.Fatalf("unescaped title delimiter reference definition should stay literal: %q", joined)
	}

	md = NewMarkdown("> quote\n[after-quote]: https://example.com/after\n\n[after quote][after-quote]", theme)
	joined = strings.Join(md.Render(120), "\n")
	plain = stripANSI(joined)
	for _, want := range []string{"│ [after-quote]: <link>https://example.com/after</link>", "[after quote][after-quote]"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("reference definition after lazy blockquote continuation should stay in quote like Pi, missing %q in %q", want, joined)
		}
	}
	for _, unwanted := range []string{"<link>after quote</link><url>"} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("reference definition after blockquote should not resolve outside the quote, found %q in %q", unwanted, plain)
		}
	}

	md = NewMarkdown("> [quote-ref]: https://example.com/quote\n> \"Quote title\"\n> [inside quote][quote-ref]", theme)
	joined = strings.Join(md.Render(120), "\n")
	plain = stripANSI(joined)
	if !strings.Contains(joined, "│ <link>inside quote</link><url> (https://example.com/quote)</url>") {
		t.Fatalf("blockquote reference definition should apply inside quote: %q", joined)
	}
	for _, unwanted := range []string{"[quote-ref]:", "Quote title"} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("blockquote reference definition leaked %q in %q", unwanted, plain)
		}
	}

	md = NewMarkdown("- [top][top]\n  [top]: https://example.com/top\n  - [nested][nested]\n    [nested]: <https://example.com/nested?a=1&amp;b=2>\n      \"Nested title\"\n- [reuse nested][nested]", theme)
	joined = strings.Join(md.Render(180), "\n")
	plain = stripANSI(joined)
	for _, want := range []string{
		"- [top][top]",
		"[top]: <link>https://example.com/top</link>",
		"- [nested][nested]",
		"[nested]: <link>https://example.com/nested?a=1&amp;b=2</link>",
		`"Nested title"`,
		"- [reuse nested][nested]",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("list-contained reference definition should remain visible like Pi, missing %q in %q", want, joined)
		}
	}
	for _, unwanted := range []string{"<link>top</link><url>", "<link>nested</link><url>", "<link>reuse nested</link><url>", "```"} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("list-contained reference links should not resolve like top-level definitions, found %q in %q", unwanted, plain)
		}
	}

	md = NewMarkdown(">   - [quote nested][qref]\n>     [qref]: https://example.com/qref\n>       \"Quote nested title\"\n>   - [quote reuse][qref]", theme)
	joined = strings.Join(md.Render(180), "\n")
	plain = stripANSI(joined)
	for _, want := range []string{
		"│ - [quote nested][qref]",
		"│   [qref]: <link>https://example.com/qref</link>",
		`"Quote nested title"`,
		"│ - [quote reuse][qref]",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("blockquote list-contained reference definition should remain visible like Pi, missing %q in %q", want, joined)
		}
	}
	for _, unwanted := range []string{"<link>quote nested</link><url>", "<link>quote reuse</link><url>", "```"} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("blockquote list-contained reference links should not resolve, found %q in %q", unwanted, plain)
		}
	}

	md = NewMarkdown("    [code-ref]: https://example.com/code\n\n[code-ref]", theme)
	joined = strings.Join(md.Render(120), "\n")
	plain = stripANSI(joined)
	if strings.Contains(joined, "<link>code-ref</link>") {
		t.Fatalf("top-level indented code should not be parsed as a reference definition: %q", joined)
	}
	if !strings.Contains(plain, "[code-ref]: https://example.com/code") || !strings.Contains(plain, "[code-ref]") {
		t.Fatalf("top-level indented code/reference text should remain visible: %q", plain)
	}
}

func TestMarkdownMailtoAndEmailRendering(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()

	md := NewMarkdown("Contact user@example.com for help", MarkdownTheme{})
	plain := stripANSI(strings.Join(md.Render(120), "\n"))
	if strings.Count(plain, "user@example.com") != 1 {
		t.Fatalf("email should render once, got %q", plain)
	}
	if strings.Contains(plain, "mailto:") {
		t.Fatalf("bare email should not show mailto prefix: %q", plain)
	}

	md = NewMarkdown("[Email me](mailto:test@example.com)", MarkdownTheme{})
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, "Email me (mailto:test@example.com)") {
		t.Fatalf("mailto URL should be shown when OSC hyperlinks are unavailable: %q", plain)
	}

	SetCapabilities(TerminalCapabilities{Hyperlinks: true})
	joined := strings.Join(md.Render(120), "\n")
	if !strings.Contains(joined, "\x1b]8;;mailto:test@example.com\x1b\\Email me\x1b]8;;\x1b\\") {
		t.Fatalf("mailto link should use OSC 8 when available: %q", joined)
	}

	md = NewMarkdown("Contact user@example.com for help", MarkdownTheme{})
	joined = strings.Join(md.Render(120), "\n")
	if !strings.Contains(joined, "\x1b]8;;mailto:user@example.com\x1b\\user@example.com\x1b]8;;\x1b\\") {
		t.Fatalf("bare email should use OSC 8 mailto when available: %q", joined)
	}

	md = NewMarkdown("Contact user@example.c, user@example.com-, and user@example.c- today", MarkdownTheme{})
	joined = strings.Join(md.Render(160), "\n")
	if !strings.Contains(joined, "\x1b]8;;mailto:user@example.c\x1b\\user@example.c\x1b]8;;\x1b\\,") {
		t.Fatalf("bare email should allow marked one-character final labels: %q", joined)
	}
	if !strings.Contains(joined, "\x1b]8;;mailto:user@example.co\x1b\\user@example.co\x1b]8;;\x1b\\m-") {
		t.Fatalf("bare email should backtrack before trailing dash/underscore like marked: %q", joined)
	}
	if strings.Contains(joined, "\x1b]8;;mailto:user@example.c-\x1b\\") {
		t.Fatalf("invalid one-character label followed by dash should stay literal: %q", joined)
	}
}

func TestMarkdownAutolinks(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()

	md := NewMarkdown("See <https://example.com/path> and <https://example.com?a=1&amp;b=2> and <mailto:linked@example.com> and <custom+scheme:opaque> and <aa:> and <x:> and <user@example.com> and <a!b@example.com> and <o'hara@example.com>", MarkdownTheme{})
	plain := stripANSI(strings.Join(md.Render(120), " "))
	if strings.Contains(plain, "<https://") || strings.Contains(plain, "<mailto:") || strings.Contains(plain, "<custom+scheme:") || strings.Contains(plain, "<aa:>") || strings.Contains(plain, "<user@") {
		t.Fatalf("autolinks should not keep angle brackets, got %q", plain)
	}
	if !strings.Contains(plain, "<x:>") {
		t.Fatalf("single-character URI scheme should remain literal like marked, got %q", plain)
	}
	if strings.Count(plain, "https://example.com/path") != 1 || strings.Count(plain, "user@example.com") != 1 || strings.Count(plain, "a!b@example.com") != 1 || strings.Count(plain, "o'hara@example.com") != 1 {
		t.Fatalf("autolinks should render visible text once, got %q", plain)
	}
	if strings.Count(plain, "https://example.com?a=1&amp;b=2") != 1 {
		t.Fatalf("URL autolink href entities should stay raw and render once, got %q", plain)
	}
	if strings.Count(plain, "mailto:linked@example.com") != 1 || strings.Count(plain, "custom+scheme:opaque") != 1 || strings.Count(plain, "aa:") != 1 {
		t.Fatalf("generic URI autolinks should render once without duplicate fallback, got %q", plain)
	}

	SetCapabilities(TerminalCapabilities{Hyperlinks: true})
	joined := strings.Join(md.Render(120), "\n")
	if !strings.Contains(joined, "\x1b]8;;https://example.com/path\x1b\\https://example.com/path\x1b]8;;\x1b\\") {
		t.Fatalf("URL autolink should use OSC 8 when available: %q", joined)
	}
	if !strings.Contains(joined, "\x1b]8;;https://example.com?a=1&amp;b=2\x1b\\https://example.com?a=1&amp;b=2\x1b]8;;\x1b\\") {
		t.Fatalf("URL autolink should keep href entities raw for OSC 8 like Pi: %q", joined)
	}
	if !strings.Contains(joined, "\x1b]8;;custom+scheme:opaque\x1b\\custom+scheme:opaque\x1b]8;;\x1b\\") {
		t.Fatalf("generic URI autolink should use OSC 8 when available: %q", joined)
	}
	if !strings.Contains(joined, "\x1b]8;;aa:\x1b\\aa:\x1b]8;;\x1b\\") {
		t.Fatalf("empty-opaque URI autolink should use OSC 8 when available: %q", joined)
	}
	if !strings.Contains(joined, "\x1b]8;;mailto:linked@example.com\x1b\\mailto:linked@example.com\x1b]8;;\x1b\\") {
		t.Fatalf("URI mailto autolink should use OSC 8 with mailto label when available: %q", joined)
	}
	if !strings.Contains(joined, "\x1b]8;;mailto:user@example.com\x1b\\user@example.com\x1b]8;;\x1b\\") {
		t.Fatalf("email autolink should use OSC 8 mailto when available: %q", joined)
	}
	if !strings.Contains(joined, "\x1b]8;;mailto:a!b@example.com\x1b\\a!b@example.com\x1b]8;;\x1b\\") || !strings.Contains(joined, "\x1b]8;;mailto:o'hara@example.com\x1b\\o'hara@example.com\x1b]8;;\x1b\\") {
		t.Fatalf("email autolinks should support marked local-part punctuation: %q", joined)
	}
}

func TestMarkdownURLBackslashEscapesStayRawLikeMarked(t *testing.T) {
	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()
	theme := MarkdownTheme{
		Link:    func(s string) string { return "<link>" + s + "</link>" },
		LinkURL: func(s string) string { return "<url>" + s + "</url>" },
	}
	md := NewMarkdown(`<http://example.com/a\*b> http://example.com/a\*b www.example.com/a\*b`, theme)
	plain := stripANSI(strings.Join(md.Render(200), "\n"))
	for _, want := range []string{
		`<link>http://example.com/a\*b</link>`,
		`<link>www.example.com/a\*b</link><url> (http://www.example.com/a\*b)</url>`,
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("URL backslash escape should stay raw like marked, missing %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "a*b") || strings.Contains(plain, "<<link>") {
		t.Fatalf("URL backslash escape should not be unescaped or parsed as inner bare URL: %q", plain)
	}

	md = NewMarkdown(`\<http://example.com/a>`, theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if plain != `<<link>http://example.com/a></link>` {
		t.Fatalf("escaped autolink opener should fall back to marked bare URL tokenization, got %q", plain)
	}
}

func TestMarkdownHTMLLikeTagsRemainVisible(t *testing.T) {
	md := NewMarkdown("This has <thinking>hidden content</thinking> in plain text", MarkdownTheme{})
	plain := stripANSI(strings.Join(md.Render(120), " "))
	if !strings.Contains(plain, "hidden content") || !strings.Contains(plain, "<thinking>") {
		t.Fatalf("HTML-like text should remain visible, got %q", plain)
	}

	md = NewMarkdown("```html\n<div>Some HTML</div>\n```", MarkdownTheme{})
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, "<div>Some HTML</div>") {
		t.Fatalf("HTML in code fence should remain visible, got %q", plain)
	}

	theme := MarkdownTheme{Bold: func(s string) string { return "<b>" + s + "</b>" }}
	md = NewMarkdown("<div>\n**raw bold** [raw link](https://example.com)\n</div>\n\noutside **bold**", theme)
	rendered := strings.Join(md.Render(120), "\n")
	plain = stripANSI(rendered)
	if !strings.Contains(plain, "**raw bold** [raw link](https://example.com)") {
		t.Fatalf("HTML block contents should render as raw text, got %q", plain)
	}
	if strings.Contains(plain, "<b>raw bold</b>") {
		t.Fatalf("HTML block contents should not be parsed as inline markdown, got %q", plain)
	}
	if !strings.Contains(plain, "outside <b>bold</b>") {
		t.Fatalf("markdown after HTML block should still be parsed, got %q", plain)
	}

	md = NewMarkdown("<thinking data-x=\">\">\n**raw thought** [raw link](https://example.com)\n</thinking>\n\noutside **bold**", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, "**raw thought** [raw link](https://example.com)") || !strings.Contains(plain, "</thinking>") {
		t.Fatalf("custom HTML block contents should render as raw text, got %q", plain)
	}
	if strings.Contains(plain, "<b>raw thought</b>") {
		t.Fatalf("custom HTML block contents should not parse inline markdown, got %q", plain)
	}
	if !strings.Contains(plain, "outside <b>bold</b>") {
		t.Fatalf("markdown after custom HTML block should still be parsed, got %q", plain)
	}

	md = NewMarkdown("<thinking>inline **bold**</thinking> outside **bold**", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, "<thinking>inline <b>bold</b></thinking> outside <b>bold</b>") {
		t.Fatalf("inline custom HTML-like text should still parse surrounding inline markdown, got %q", plain)
	}

	md = NewMarkdown(`<span title="&amp; **raw attr**">inline **bold**</span> &amp; text`, theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, `<span title="&amp; **raw attr**">inline <b>bold</b></span> &amp; text`) {
		t.Fatalf("inline HTML tags should preserve raw attributes while parsing surrounding text, got %q", plain)
	}
	if strings.Contains(plain, `<span title="& <b>raw attr</b>">`) {
		t.Fatalf("inline HTML tag attributes should not decode entities or parse markdown, got %q", plain)
	}

	md = NewMarkdown(`<foo_bar title="&amp; **raw attr**">inline **bold**</foo_bar> and </x:y> **bold**`, theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, `<foo_bar title="&amp; **raw attr**">inline <b>bold</b></foo_bar> and </x:y> <b>bold</b>`) {
		t.Fatalf("marked inline HTML tag-name rules should preserve raw attributes and closing colon tags, got %q", plain)
	}
	if strings.Contains(plain, `<foo_bar title="& <b>raw attr</b>">`) {
		t.Fatalf("underscore HTML tag attributes should not decode entities or parse markdown, got %q", plain)
	}

	md = NewMarkdown(`<span =bad "&amp; **raw attr**"> after **bold** and <!foo**bar**>`, theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, `<span =bad "&amp; <b>raw attr</b>"> after <b>bold</b> and <!foo<b>bar</b>>`) {
		t.Fatalf("invalid inline HTML opening/declaration should fall back to markdown text like marked, got %q", plain)
	}
	if strings.Contains(plain, `**raw attr**`) || strings.Contains(plain, `<!foo**bar**>`) {
		t.Fatalf("invalid inline HTML should not be stashed as raw HTML, got %q", plain)
	}

	md = NewMarkdown(`before <!-- **raw comment** &amp; --> after **bold**`, theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, `before <!-- **raw comment** &amp; --> after <b>bold</b>`) {
		t.Fatalf("inline HTML comments should remain raw while surrounding text parses, got %q", plain)
	}
	if strings.Contains(plain, `<b>raw comment</b>`) || strings.Contains(plain, `<!-- **raw comment** & -->`) {
		t.Fatalf("inline HTML comments should not decode entities or parse markdown, got %q", plain)
	}

	md = NewMarkdown("> <div>\n> **raw quote bold**\n> </div>\n\noutside **bold**", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, "**raw quote bold**") {
		t.Fatalf("blockquote HTML block contents should remain raw, got %q", plain)
	}
	if strings.Contains(plain, "<b>raw quote bold</b>") {
		t.Fatalf("blockquote HTML block contents should not parse inline markdown, got %q", plain)
	}
	if !strings.Contains(plain, "outside <b>bold</b>") {
		t.Fatalf("markdown after blockquote HTML block should still be parsed, got %q", plain)
	}

	markerTheme := MarkdownTheme{
		Bold:   func(s string) string { return "<b>" + s + "</b>" },
		Italic: func(s string) string { return "<i>" + s + "</i>" },
	}
	markerLines := NewMarkdown("> <div>\n> raw **bold**\n> </div>\n\noutside **bold**", markerTheme).Render(120)
	markerWant := []string{
		"│ <i><div>",
		"│ raw **bold**",
		"│ </div></i>",
		"",
		"outside <b>bold</b>",
	}
	if !equalLines(markerLines, markerWant) {
		t.Fatalf("blockquote HTML block marker style = %#v, want %#v", markerLines, markerWant)
	}

	md = NewMarkdown("- <div>\n  **raw list bold** [raw link](https://example.com)\n  </div>\n\noutside **bold**", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	for _, want := range []string{
		"- <div>",
		"  **raw list bold** [raw link](https://example.com)",
		"  </div>",
		"outside <b>bold</b>",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("list HTML block missing %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "<b>raw list bold</b>") {
		t.Fatalf("list HTML block contents should not parse inline markdown, got %q", plain)
	}

	md = NewMarkdown("> - <div>\n>   **raw quote list bold**\n>   </div>\n\noutside **bold**", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	for _, want := range []string{
		"│ - <div>",
		"│   **raw quote list bold**",
		"│   </div>",
		"outside <b>bold</b>",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("blockquote list HTML block missing %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "<b>raw quote list bold</b>") {
		t.Fatalf("blockquote list HTML block contents should not parse inline markdown, got %q", plain)
	}
}

func TestMarkdownCustomHTMLBlocksDoNotInterruptParagraphLikeMarked(t *testing.T) {
	theme := MarkdownTheme{Bold: func(s string) string { return "<b>" + s + "</b>" }}

	md := NewMarkdown("before\n<thinking>\n**inline bold**\n</thinking>\nafter **bold**", theme)
	plain := stripANSI(strings.Join(md.Render(120), "\n"))
	for _, want := range []string{"before", "<thinking>", "<b>inline bold</b>", "</thinking>", "after <b>bold</b>"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("custom HTML tag inside paragraph missing %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "**inline bold**") || strings.Contains(plain, "after **bold**") {
		t.Fatalf("custom HTML tag should not interrupt paragraph inline parsing, got %q", plain)
	}

	md = NewMarkdown("<foo_bar attr:name=\"&amp;\">\n**raw underscore block**\n\nafter **bold**", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, `<foo_bar attr:name="&amp;">`) || !strings.Contains(plain, "**raw underscore block**") || !strings.Contains(plain, "after <b>bold</b>") {
		t.Fatalf("underscore custom HTML block should stay raw like marked, got %q", plain)
	}
	if strings.Contains(plain, `<foo_bar attr:name="&">`) || strings.Contains(plain, "<b>raw underscore block</b>") {
		t.Fatalf("underscore custom HTML block should not decode attributes or parse inline markdown, got %q", plain)
	}

	md = NewMarkdown("<foo =bad \"&amp; **raw attr**\">\n**inline body**\n\nafter **bold**", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, `<foo =bad "&amp; <b>raw attr</b>">`) || !strings.Contains(plain, "<b>inline body</b>") || !strings.Contains(plain, "after <b>bold</b>") {
		t.Fatalf("invalid custom HTML block tag should fall back to paragraph markdown like marked, got %q", plain)
	}
	if strings.Contains(plain, "**inline body**") {
		t.Fatalf("invalid custom HTML block tag should not be rendered as raw HTML block, got %q", plain)
	}

	md = NewMarkdown("<x:y>\n**inline colon opening**\n\nafter **bold**", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, "<x:y>") || !strings.Contains(plain, "<b>inline colon opening</b>") || !strings.Contains(plain, "after <b>bold</b>") {
		t.Fatalf("colon opening tag should remain paragraph inline content like marked, got %q", plain)
	}

	md = NewMarkdown("before\n<div>\n**raw bold**\n</div>\nafter **bold**", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, "**raw bold**") || strings.Contains(plain, "<b>raw bold</b>") {
		t.Fatalf("standard HTML block tag should still interrupt paragraphs as raw HTML, got %q", plain)
	}

	md = NewMarkdown("<div/abc **inline bold**>\nafter **bold**", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, "<div/abc <b>inline bold</b>>") || !strings.Contains(plain, "after <b>bold</b>") {
		t.Fatalf("invalid standard HTML block start should fall back to paragraph markdown like marked, got %q", plain)
	}
	if strings.Contains(plain, "**inline bold**") {
		t.Fatalf("invalid standard HTML block start should not be rendered raw, got %q", plain)
	}
}

func TestMarkdownInterruptingHTMLBlocksStopLazyContinuationLikeMarked(t *testing.T) {
	theme := MarkdownTheme{Bold: func(s string) string { return "<b>" + s + "</b>" }}

	md := NewMarkdown("> quote\n<div>\n**raw bold**\n</div>\n\noutside **bold**", theme)
	plain := stripANSI(strings.Join(md.Render(120), "\n"))
	for _, want := range []string{"│ quote", "<div>", "**raw bold**", "</div>", "outside <b>bold</b>"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("blockquote-interrupting HTML block missing %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "│ <div>") || strings.Contains(plain, "<b>raw bold</b>") {
		t.Fatalf("standard HTML block should stop lazy blockquote continuation, got %q", plain)
	}

	md = NewMarkdown("- item\n<div>\n**raw list bold**\n</div>\n\noutside **bold**", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	for _, want := range []string{"- item", "<div>", "**raw list bold**", "</div>", "outside <b>bold</b>"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("list-interrupting HTML block missing %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "  <div>") || strings.Contains(plain, "<b>raw list bold</b>") {
		t.Fatalf("standard HTML block should stop lazy list continuation, got %q", plain)
	}
}

func TestMarkdownListContinuationHTMLBlocksKeepPiIndentation(t *testing.T) {
	theme := MarkdownTheme{Bold: func(s string) string { return "<b>" + s + "</b>" }}

	md := NewMarkdown("- item\n  <div>\n  **raw continuation**\n  </div>\n- next", theme)
	plain := stripANSI(strings.Join(md.Render(120), "\n"))
	for _, want := range []string{"- item", "  <div>", "  **raw continuation**", "  </div>", "- next"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("list continuation HTML block missing %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "<b>raw continuation</b>") {
		t.Fatalf("list continuation HTML block should remain raw, got %q", plain)
	}

	md = NewMarkdown("> - item\n>   <div>\n>   **raw quote continuation**\n>   </div>\n> - next", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	for _, want := range []string{"│ - item", "│   <div>", "│   **raw quote continuation**", "│   </div>", "│ - next"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("blockquote list continuation HTML block missing %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "<b>raw quote continuation</b>") {
		t.Fatalf("blockquote list continuation HTML block should remain raw, got %q", plain)
	}
}

func TestMarkdownHTMLType1BlocksMatchCommonMark(t *testing.T) {
	theme := MarkdownTheme{Bold: func(s string) string { return "<b>" + s + "</b>" }}

	md := NewMarkdown("<textarea>\n\n**raw textarea**\n\n</textarea>\nafter **bold**", theme)
	plain := stripANSI(strings.Join(md.Render(120), "\n"))
	for _, want := range []string{"<textarea>", "**raw textarea**", "</textarea>", "after <b>bold</b>"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("textarea HTML block missing %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "<b>raw textarea</b>") {
		t.Fatalf("textarea HTML block contents should remain raw through blank lines, got %q", plain)
	}

	md = NewMarkdown("<script\n**raw script**\n</script>\nafter **bold**", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	for _, want := range []string{"<script", "**raw script**", "</script>", "after <b>bold</b>"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("partial script HTML block missing %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "<b>raw script</b>") {
		t.Fatalf("script HTML block started at end-of-line should remain raw, got %q", plain)
	}

	md = NewMarkdown("- item\n  <textarea>\n\n  **raw list textarea**\n\n  </textarea>\n- next", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	for _, want := range []string{"- item", "  <textarea>", "  **raw list textarea**", "  </textarea>", "- next"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("list textarea HTML block missing %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "<b>raw list textarea</b>") {
		t.Fatalf("list textarea HTML block should remain raw through blank lines, got %q", plain)
	}

	md = NewMarkdown("> - item\n>   <textarea>\n>\n>   **raw quote list textarea**\n>\n>   </textarea>\n> - next", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	for _, want := range []string{"│ - item", "│   <textarea>", "│   **raw quote list textarea**", "│   </textarea>", "│ - next"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("blockquote list textarea HTML block missing %q in %q", want, plain)
		}
	}
	if strings.Contains(plain, "<b>raw quote list textarea</b>") {
		t.Fatalf("blockquote list textarea HTML block should remain raw through blank lines, got %q", plain)
	}

	md = NewMarkdown("</script>\n**bold**", theme)
	plain = stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, "</script>") || !strings.Contains(plain, "<b>bold</b>") {
		t.Fatalf("closing type-1 tag alone should not swallow following markdown, got %q", plain)
	}
}

func TestMarkdownHTMLCommentDeclarationAndCDataBlocksMatchMarked(t *testing.T) {
	theme := MarkdownTheme{Bold: func(s string) string { return "<b>" + s + "</b>" }}

	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "comment block",
			text: "<!--\n**raw comment**\n-->\nafter **bold**",
			want: []string{"<!--", "**raw comment**", "-->", "after <b>bold</b>"},
		},
		{
			name: "processing instruction block",
			text: "<?pi\n**raw processing**\n?>\nafter **bold**",
			want: []string{"<?pi", "**raw processing**", "?>", "after <b>bold</b>"},
		},
		{
			name: "declaration block",
			text: "<!DOCTYPE html>\nafter **bold**",
			want: []string{"<!DOCTYPE html>", "after <b>bold</b>"},
		},
		{
			name: "cdata block",
			text: "<![CDATA[\n**raw cdata**\n]]>\nafter **bold**",
			want: []string{"<![CDATA[", "**raw cdata**", "]]>", "after <b>bold</b>"},
		},
		{
			name: "list-contained comment block",
			text: "- item\n  <!--\n  **raw list comment**\n  -->\n- next",
			want: []string{"- item", "  <!--", "  **raw list comment**", "  -->", "- next"},
		},
		{
			name: "blockquote list-contained comment block",
			text: "> - item\n>   <!--\n>   **raw quote list comment**\n>   -->\n> - next",
			want: []string{"│ - item", "│   <!--", "│   **raw quote list comment**", "│   -->", "│ - next"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			md := NewMarkdown(tc.text, theme)
			plain := stripANSI(strings.Join(md.Render(120), "\n"))
			for _, want := range tc.want {
				if !strings.Contains(plain, want) {
					t.Fatalf("HTML raw block missing %q in %q", want, plain)
				}
			}
			if strings.Contains(plain, "<b>raw") {
				t.Fatalf("HTML raw block contents should not parse inline markdown, got %q", plain)
			}
		})
	}

	md := NewMarkdown("<! **raw declaration**>\nafter **bold**", theme)
	plain := stripANSI(strings.Join(md.Render(120), "\n"))
	if !strings.Contains(plain, "<! <b>raw declaration</b>>") || !strings.Contains(plain, "after <b>bold</b>") {
		t.Fatalf("invalid HTML declaration block should fall back to paragraph markdown like marked, got %q", plain)
	}
	if strings.Contains(plain, "**raw declaration**") {
		t.Fatalf("invalid HTML declaration block should not be rendered raw, got %q", plain)
	}
}

func TestMarkdownImageFallbackRendersAltText(t *testing.T) {
	md := NewMarkdown(`Here is ![diagram [alt]](https://example.com/diagram_(v1).png "Diagram ) title") inline`, MarkdownTheme{})
	plain := stripANSI(strings.Join(md.Render(120), " "))
	if !strings.Contains(plain, "Here is diagram [alt] inline") {
		t.Fatalf("markdown image should render alt text, got %q", plain)
	}
	if strings.Contains(plain, "https://example.com") || strings.Contains(plain, "Diagram") || strings.Contains(plain, "![") {
		t.Fatalf("markdown image should not render image URL or delimiters, got %q", plain)
	}

	md = NewMarkdown("Image ![**raw &amp; alt**](https://example.com/raw.png)", MarkdownTheme{
		Bold: func(s string) string { return "<b>" + s + "</b>" },
	})
	plain = stripANSI(strings.Join(md.Render(120), " "))
	if !strings.Contains(plain, "Image **raw &amp; alt**") {
		t.Fatalf("markdown image alt text should render as raw literal text, got %q", plain)
	}
	if strings.Contains(plain, "<b>") {
		t.Fatalf("markdown image alt text should not be parsed as inline markdown: %q", plain)
	}

	md = NewMarkdown(`Image ![\*\*escaped raw alt\*\*](https://example.com/raw.png)`, MarkdownTheme{
		Bold: func(s string) string { return "<b>" + s + "</b>" },
	})
	plain = stripANSI(strings.Join(md.Render(120), " "))
	if !strings.Contains(plain, `Image \*\*escaped raw alt\*\*`) {
		t.Fatalf("markdown image escaped alt text should stay raw like Pi, got %q", plain)
	}
	if strings.Contains(plain, "<b>") || strings.Contains(plain, "Image **escaped raw alt**") {
		t.Fatalf("markdown image escaped alt text should not be unescaped or parsed: %q", plain)
	}

	md = NewMarkdown("See ![reference alt][img] and ![collapsed alt][] and ![shortcut alt] and ![outer [alt]][nested] and ![outer [alt]][] and ![escaped label][img*ref] and ![entity label][img&ref]\n\n[img]: https://example.com/reference.png\n[collapsed alt]: https://example.com/collapsed.png\n[shortcut alt]: https://example.com/shortcut.png\n[nested]: https://example.com/nested.png\n[outer [alt]]: https://example.com/collapsed-nested.png\n[img\\*ref]: https://example.com/escaped.png\n[img&amp;ref]: https://example.com/entity.png", MarkdownTheme{})
	plain = stripANSI(strings.Join(md.Render(120), " "))
	if !strings.Contains(plain, "See reference alt and collapsed alt and shortcut alt and outer [alt] and ![outer [alt]][] and ![escaped label][img*ref] and ![entity label][img&ref]") {
		t.Fatalf("reference images should render alt text, got %q", plain)
	}
	for _, want := range []string{
		"[outer [alt]]: https://example.com/collapsed-nested.png",
		"[img*ref]: https://example.com/escaped.png",
		"[img&amp;ref]: https://example.com/entity.png",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("unrecognized nested/escaped/entity image definitions should render like Pi, missing %q in %q", want, plain)
		}
	}
	for _, unwanted := range []string{"[img]:", "[collapsed alt]:", "[shortcut alt]:", "[nested]:"} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("recognized reference image definitions should not render, found %q in %q", unwanted, plain)
		}
	}

	md = NewMarkdown("Images ![empty]() ![angle empty](<>) ![ref empty][empty]\n\n[empty]: <>", MarkdownTheme{})
	plain = stripANSI(strings.Join(md.Render(120), " "))
	if !strings.Contains(plain, "Images empty angle empty ref empty") {
		t.Fatalf("empty image targets should render alt text, got %q", plain)
	}
	for _, unwanted := range []string{"![empty]()", "![angle empty](<>)", "[empty]:", "<>"} {
		if strings.Contains(plain, unwanted) {
			t.Fatalf("empty image target leaked %q in %q", unwanted, plain)
		}
	}

	md = NewMarkdown("See ![**raw ref &amp; alt**][img]\n\n[img]: https://example.com/reference.png", MarkdownTheme{
		Bold: func(s string) string { return "<b>" + s + "</b>" },
	})
	plain = stripANSI(strings.Join(md.Render(120), " "))
	if !strings.Contains(plain, "See **raw ref &amp; alt**") {
		t.Fatalf("reference image alt text should render as raw literal text, got %q", plain)
	}
	if strings.Contains(plain, "<b>") {
		t.Fatalf("reference image alt text should not be parsed as inline markdown: %q", plain)
	}

	SetCapabilities(TerminalCapabilities{Hyperlinks: false})
	defer ResetCapabilitiesCache()

	md = NewMarkdown("Escaped \\![reference alt][img]\n\n[img]: https://example.com/reference.png", MarkdownTheme{})
	plain = stripANSI(strings.Join(md.Render(120), " "))
	if !strings.Contains(plain, "Escaped !reference alt (https://example.com/reference.png)") {
		t.Fatalf("escaped image marker should leave a link token like marked, got %q", plain)
	}
	if strings.Contains(plain, "![reference alt][img]") || strings.Contains(plain, "[img]:") {
		t.Fatalf("escaped image marker should not render as a full literal image or leak definition, got %q", plain)
	}
}

func TestMatchesKittyBaseLayoutAndModifyOtherKeys(t *testing.T) {
	SetKittyProtocolActive(true)
	if !MatchesKey("\x1b[1089::99;5u", "ctrl+c") {
		t.Fatalf("expected Cyrillic ctrl+c with base key to match ctrl+c")
	}
	if MatchesKey("\x1b[1089::99;5u", "ctrl+shift+c") {
		t.Fatalf("ctrl+c should not match ctrl+shift+c")
	}
	if !MatchesKey("\x1b[107;13u", "ctrl+super+k") {
		t.Fatalf("expected super modifier to be decoded")
	}
	if !MatchesKey("\x1b[57417u", "left") || !MatchesKey("\x1b[57426u", "delete") {
		t.Fatalf("expected keypad navigation to normalize")
	}
	SetKittyProtocolActive(false)
	if !MatchesKey("\x1b[27;5;13~", "ctrl+enter") {
		t.Fatalf("expected modifyOtherKeys ctrl+enter to match")
	}
	if !MatchesKey("\x1b[27;2;9~", "shift+tab") {
		t.Fatalf("expected modifyOtherKeys shift+tab to match")
	}
}

func TestStdinBufferSplitsPaste(t *testing.T) {
	buffer := NewStdinBuffer()
	var data []string
	var paste []string
	buffer.OnData(func(s string) { data = append(data, s) })
	buffer.OnPaste(func(s string) { paste = append(paste, s) })
	buffer.Process("a\x1b[200~hello\nworld\x1b[201~b")
	if strings.Join(data, "") != "ab" {
		t.Fatalf("data = %#v, want a/b", data)
	}
	if len(paste) != 1 || paste[0] != "hello\nworld" {
		t.Fatalf("paste = %#v", paste)
	}
}
