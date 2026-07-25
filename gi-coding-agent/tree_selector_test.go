package gicodingagent

import (
	"errors"
	"strings"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func TestTreeSelectorInitialSelectionUsesNearestVisibleAncestor(t *testing.T) {
	tree := buildTreeSelectorTree([]FileEntry{
		treeUser("user-1", nil, "hello"),
		treeAssistant("asst-1", ptrString("user-1"), "hi"),
		treeUser("user-2", ptrString("asst-1"), "active branch"),
		treeModelChange("model-1", ptrString("user-2")),
		treeUser("user-3", ptrString("asst-1"), "sibling branch"),
	})
	if got := NewTreeSelectorComponent(tree, "model-1").GetTreeList().GetSelectedNode().Entry.ID; got != "user-2" {
		t.Fatalf("selected = %s, want user-2", got)
	}

	tree = buildTreeSelectorTree([]FileEntry{
		treeUser("user-1", nil, "hello"),
		treeAssistant("asst-1", ptrString("user-1"), "hi"),
		treeUser("user-2", ptrString("asst-1"), "active branch"),
		{Type: "thinking_level_change", ID: "thinking-1", ParentID: ptrString("user-2"), ThinkingLevel: "high"},
		treeUser("user-3", ptrString("asst-1"), "sibling branch"),
	})
	if got := NewTreeSelectorComponent(tree, "thinking-1").GetTreeList().GetSelectedNode().Entry.ID; got != "user-2" {
		t.Fatalf("selected = %s, want user-2", got)
	}
}

func TestTreeSelectorFilterSwitchingUsesParentTraversal(t *testing.T) {
	tree := buildTreeSelectorTree([]FileEntry{
		treeUser("user-1", nil, "hello"),
		treeAssistant("asst-1", ptrString("user-1"), "hi"),
		treeUser("user-2", ptrString("asst-1"), "active branch"),
		treeAssistant("asst-2", ptrString("user-2"), "response"),
		treeUser("user-3", ptrString("asst-1"), "sibling branch"),
	})
	selector := NewTreeSelectorComponent(tree, "asst-2")
	list := selector.GetTreeList()
	selector.HandleInput("\x15")
	if got := list.GetSelectedNode().Entry.ID; got != "user-2" {
		t.Fatalf("selected = %s, want user-2", got)
	}
	selector.HandleInput("\x04")
	if got := list.GetSelectedNode().Entry.ID; got != "user-2" {
		t.Fatalf("selected = %s, want user-2", got)
	}
}

func TestTreeSelectorHelpWrapsSemanticItemsWithoutTruncation(t *testing.T) {
	selector := NewTreeSelectorComponent(
		buildTreeSelectorTree([]FileEntry{
			treeUser("user-1", nil, "hello"),
			treeAssistant("asst-1", ptrString("user-1"), "hi"),
		}),
		"asst-1",
	)
	lines := selector.Render(30)
	plain := StripAnsi(strings.Join(lines, "\n"))
	for _, want := range []string{"branch", "copy", "filters", "cycle", "label time"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("narrow help missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "...") {
		t.Fatalf("narrow help should wrap semantic items instead of truncating:\n%s", plain)
	}
	for index, line := range lines {
		if width := gitui.VisibleWidth(line); width > 30 {
			t.Fatalf("line %d width = %d, want <= 30: %q", index, width, line)
		}
	}
	if got := normalizeTreeSelectorHelpKeyText("super+left"); got != "super+←" {
		t.Fatalf("normalized custom key = %q, want super+←", got)
	}
}

func TestTreeSelectorCopiesFullSelectedContent(t *testing.T) {
	message := "  " + strings.Repeat("long message ", 30) + "\nsecond line  "
	selector := NewTreeSelectorComponent(
		buildTreeSelectorTree([]FileEntry{
			treeUser("user-1", nil, "hello"),
			treeAssistant("asst-1", ptrString("user-1"), message),
		}),
		"asst-1",
	)
	var copied *string
	selector.OnCopy = func(text *string) {
		copied = text
	}
	selector.HandleInput("\x18")
	if copied == nil || *copied != message {
		t.Fatalf("copied = %#v, want full untruncated message %q", copied, message)
	}
}

func TestTreeSelectorSearchStateFiltersAndEscapeClearsBeforeCancel(t *testing.T) {
	selector := NewTreeSelectorComponent(
		buildTreeSelectorTree([]FileEntry{
			treeUser("user-1", nil, "alpha branch"),
			treeAssistant("asst-1", ptrString("user-1"), "first reply"),
			treeUser("user-2", ptrString("asst-1"), "beta checkpoint"),
			treeAssistant("asst-2", ptrString("user-2"), "second reply"),
		}),
		"asst-2",
	)
	cancelled := false
	selector.OnCancel = func() { cancelled = true }
	selector.HandleInput("beta checkpoint")

	if got := selector.GetTreeList().GetSearchQuery(); got != "beta checkpoint" {
		t.Fatalf("query = %q, want beta checkpoint", got)
	}
	if got := selector.GetTreeList().GetSelectedNode(); got == nil || got.Entry.ID != "user-2" {
		t.Fatalf("selected after search = %#v, want user-2", got)
	}
	rendered := StripAnsi(strings.Join(selector.Render(80), "\n"))
	if !strings.Contains(rendered, "Type to search: beta checkpoint") || strings.Contains(rendered, "alpha branch") {
		t.Fatalf("search projection mismatch:\n%s", rendered)
	}

	selector.HandleInput("\x1b")
	if cancelled {
		t.Fatal("first escape with an active query should clear search, not cancel")
	}
	if got := selector.GetTreeList().GetSearchQuery(); got != "" {
		t.Fatalf("query after escape = %q, want empty", got)
	}
	selector.HandleInput("\x1b")
	if !cancelled {
		t.Fatal("second escape with an empty query should cancel")
	}
}

func TestTreeSelectorHorizontalViewportKeepsGutterAndSelectedAnchor(t *testing.T) {
	body := strings.Repeat(" ", 30) + tuiThemeAccent("selected anchor content")
	lines := renderTreeSelectorHorizontalViewport([]treeSelectorViewportRow{
		{gutter: "  ", body: strings.Repeat("x", 60), bodyWidth: 60},
		{
			gutter:     tuiThemeAccent("› "),
			body:       body,
			anchorCol:  30,
			bodyWidth:  gitui.VisibleWidth(body),
			isSelected: true,
		},
	}, 20)
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	selected := StripAnsi(lines[1])
	if !strings.HasPrefix(selected, "› ") || !strings.Contains(selected, "selected") {
		t.Fatalf("selected viewport should keep its gutter and anchor context: %q", selected)
	}
	for index, line := range lines {
		if width := gitui.VisibleWidth(line); width > 20 {
			t.Fatalf("line %d width = %d, want <= 20: %q", index, width, line)
		}
	}
}

func TestTreeSelectorLabelEditorPublishesBeforeUpdatingProjection(t *testing.T) {
	tree := buildTreeSelectorTree([]FileEntry{treeUser("user-1", nil, "hello")})
	selector := NewTreeSelectorComponent(tree, "user-1")
	var entryID, label string
	selector.OnLabelChange = func(nextEntryID, nextLabel string) error {
		entryID, label = nextEntryID, nextLabel
		return nil
	}
	selector.HandleInput("L")
	if rendered := StripAnsi(strings.Join(selector.Render(80), "\n")); !strings.Contains(rendered, "Label (empty to remove):") {
		t.Fatalf("label editor not rendered:\n%s", rendered)
	}
	selector.HandleInput("checkpoint")
	selector.HandleInput("\r")
	if entryID != "user-1" || label != "checkpoint" {
		t.Fatalf("label callback = (%q, %q), want (user-1, checkpoint)", entryID, label)
	}
	if tree[0].Label != "checkpoint" || tree[0].LabelTimestamp == "" {
		t.Fatalf("tree label projection = (%q, %q)", tree[0].Label, tree[0].LabelTimestamp)
	}

	wantErr := errors.New("persist label")
	selector.HandleInput("L")
	selector.HandleInput(" replacement")
	selector.OnLabelChange = func(string, string) error { return wantErr }
	var observed error
	selector.OnError = func(err error) { observed = err }
	selector.HandleInput("\r")
	if !errors.Is(observed, wantErr) {
		t.Fatalf("observed error = %v, want %v", observed, wantErr)
	}
	if tree[0].Label != "checkpoint" {
		t.Fatalf("failed persistence changed projection label to %q", tree[0].Label)
	}
}

func TestTreeSelectorLabelTimestamps(t *testing.T) {
	tree := buildTreeSelectorTree([]FileEntry{treeUser("user-1", nil, "hello"), treeAssistant("asst-1", ptrString("user-1"), "hi")})
	tree[0].Label = "checkpoint"
	tree[0].LabelTimestamp = time.Date(2026, 3, 28, 14, 32, 0, 0, time.Local).Format(time.RFC3339)
	selector := NewTreeSelectorComponent(tree, "asst-1")
	render := strings.Join(selector.GetTreeList().Render(200), "\n")
	if !strings.Contains(render, "[checkpoint]") || strings.Contains(render, "3/28 14:32") || strings.Contains(render, "[+label time]") {
		t.Fatalf("render = %q", render)
	}
	selector.HandleInput("T")
	render = strings.Join(selector.GetTreeList().Render(200), "\n")
	if !strings.Contains(render, "3/28 14:32") || !strings.Contains(render, "[+label time]") {
		t.Fatalf("render = %q", render)
	}
}

func TestTreeSelectorLabelTimestampUsesPiRelativeDateFormat(t *testing.T) {
	now := time.Date(2026, time.July, 25, 18, 0, 0, 0, time.Local)
	tests := []struct {
		name  string
		value time.Time
		want  string
	}{
		{name: "today", value: time.Date(2026, time.July, 25, 14, 32, 0, 0, time.Local), want: "14:32"},
		{name: "same year", value: time.Date(2026, time.March, 28, 14, 32, 0, 0, time.Local), want: "3/28 14:32"},
		{name: "prior year", value: time.Date(2025, time.December, 3, 4, 5, 0, 0, time.Local), want: "25/12/3 04:05"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := formatTreeSelectorLabelTimestampAt(test.value.Format(time.RFC3339), now)
			if got != test.want {
				t.Fatalf("timestamp = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTreeSelectorBashEntriesRenderLikePi(t *testing.T) {
	setTUIThemeForTest(t, "dark", nil)
	tree := buildTreeSelectorTree([]FileEntry{
		treeBash("bash-1", nil, "printf gi-bash"),
		treeBash("bash-2", ptrString("bash-1"), "printf hidden-bash"),
	})
	selector := NewTreeSelectorComponent(tree, "bash-2")
	rendered := strings.Join(selector.GetTreeList().Render(120), "\n")
	plain := StripAnsi(rendered)

	for _, want := range []string{
		"  • [bash]: printf gi-bash",
		"› • [bash]: printf hidden-bash",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("render missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "Ran `") {
		t.Fatalf("bash tree rows should show compact Pi labels, got:\n%s", plain)
	}
	if !strings.Contains(rendered, "\x1b[48;2;58;58;74m") {
		t.Fatalf("selected bash row should use Pi selected background:\n%q", rendered)
	}
}

func TestTreeSelectorMessageRowsRenderPiRoleLabels(t *testing.T) {
	setTUIThemeForTest(t, "dark", nil)
	tree := buildTreeSelectorTree([]FileEntry{
		treeUser("user-1", nil, "hello\nthere"),
		treeAssistant("asst-1", ptrString("user-1"), "hi\tthere"),
		{
			Type:     "message",
			ID:       "asst-err",
			ParentID: ptrString("asst-1"),
			Message: llm.Message{
				Role:         llm.RoleAssistant,
				StopReason:   llm.StopReasonError,
				ErrorMessage: "quota\nfailed",
			},
			Timestamp: "asst-err",
		},
	})
	selector := NewTreeSelectorComponent(tree, "asst-err")
	rendered := strings.Join(selector.GetTreeList().Render(160), "\n")
	plain := StripAnsi(rendered)

	for _, want := range []string{
		"  • user: hello there",
		"  • assistant: hi there",
		"› • assistant: quota failed",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("render missing %q:\n%s", want, plain)
		}
	}
	for _, want := range []string{
		tuiThemeAccent("user: "),
		tuiThemeSuccess("assistant: "),
		tuiThemeError("quota failed"),
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("render missing styled token %q:\n%q", want, rendered)
		}
	}
}

func TestTreeSelectorEmptyFilterPreservesSelection(t *testing.T) {
	tree := buildTreeSelectorTree([]FileEntry{
		treeUser("user-1", nil, "hello"),
		treeAssistant("asst-1", ptrString("user-1"), "hi"),
		treeUser("user-2", ptrString("asst-1"), "bye"),
		treeAssistant("asst-2", ptrString("user-2"), "goodbye"),
	})
	selector := NewTreeSelectorComponent(tree, "asst-2")
	list := selector.GetTreeList()
	selector.HandleInput("\x0c")
	if list.GetSelectedNode() != nil {
		t.Fatalf("selected = %#v, want nil", list.GetSelectedNode())
	}
	selector.HandleInput("\x04")
	if got := list.GetSelectedNode().Entry.ID; got != "asst-2" {
		t.Fatalf("selected = %s, want asst-2", got)
	}

	selector.HandleInput("\x0c")
	selector.HandleInput("\x0c")
	if got := list.GetSelectedNode().Entry.ID; got != "asst-2" {
		t.Fatalf("selected = %s, want asst-2", got)
	}
	selector.HandleInput("\x0c")
	selector.HandleInput("\x04")
	if got := list.GetSelectedNode().Entry.ID; got != "asst-2" {
		t.Fatalf("selected = %s, want asst-2", got)
	}
}

func TestTreeSelectorFoldNavigationActiveBranch(t *testing.T) {
	selector := NewTreeSelectorComponent(buildBranchingTreeSelectorTree(), "asst-4a")
	list := selector.GetTreeList()
	selector.HandleInput("\x1b[1;5D")
	assertTreeSelected(t, list, "user-3a")
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x1b[B")
	assertTreeSelected(t, list, "user-3b")
	selector.HandleInput("\x1b[A")
	assertTreeSelected(t, list, "user-3a")
	selector.HandleInput("\x1b[1;5C")
	selector.HandleInput("\x1b[B")
	assertTreeSelected(t, list, "asst-3a")
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x1b[1;5C")
	assertTreeSelected(t, list, "asst-4a")
}

func TestTreeSelectorAltArrowAliases(t *testing.T) {
	selector := NewTreeSelectorComponent(buildBranchingTreeSelectorTree(), "asst-4a")
	list := selector.GetTreeList()
	selector.HandleInput("\x1b[1;3D")
	assertTreeSelected(t, list, "user-3a")
	selector.HandleInput("\x1b[1;3D")
	selector.HandleInput("\x1b[1;3C")
	assertTreeSelected(t, list, "user-3a")
	selector.HandleInput("\x1b[1;3C")
	assertTreeSelected(t, list, "asst-4a")
}

func TestTreeSelectorNestedFoldPreservedOnRootUnfold(t *testing.T) {
	selector := NewTreeSelectorComponent(buildBranchingTreeSelectorTree(), "asst-4a")
	list := selector.GetTreeList()
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x1b[1;5D")
	assertTreeSelected(t, list, "user-1")
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x1b[B")
	assertTreeSelected(t, list, "user-1")
	selector.HandleInput("\x1b[1;5C")
	selector.HandleInput("\x1b[1;5C")
	assertTreeSelected(t, list, "user-3a")
	selector.HandleInput("\x1b[B")
	assertTreeSelected(t, list, "user-3b")
}

func TestTreeSelectorFoldOnNonActiveBranchAndMultipleRoots(t *testing.T) {
	selector := NewTreeSelectorComponent(buildBranchingTreeSelectorTree(), "asst-4a")
	list := selector.GetTreeList()
	for i := 0; i < 20 && list.GetSelectedNode().Entry.ID != "user-3b"; i++ {
		selector.HandleInput("\x1b[B")
	}
	assertTreeSelected(t, list, "user-3b")
	selector.HandleInput("\x1b[1;5C")
	assertTreeSelected(t, list, "user-4b")
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x1b[1;5D")
	assertTreeSelected(t, list, "user-1")

	multi := buildTreeSelectorTree([]FileEntry{
		treeUser("user-1", nil, "first root"),
		treeAssistant("asst-1", ptrString("user-1"), "response 1"),
		treeUser("user-2", nil, "second root"),
		treeAssistant("asst-2", ptrString("user-2"), "response 2"),
	})
	selector = NewTreeSelectorComponent(multi, "asst-1")
	list = selector.GetTreeList()
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x1b[B")
	assertTreeSelected(t, list, "user-2")
	selector.HandleInput("\x1b[1;5C")
	assertTreeSelected(t, list, "asst-2")
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x1b[1;5D")
	assertTreeSelected(t, list, "user-2")
}

func TestTreeSelectorFoldHidesDescendantsThroughFilteredNodes(t *testing.T) {
	tree := buildTreeSelectorTree([]FileEntry{
		treeUser("user-1", nil, "hello"),
		treeToolCallAssistant("tool-asst-1", ptrString("user-1")),
		treeUser("user-2", ptrString("tool-asst-1"), "follow up"),
		treeAssistant("asst-2", ptrString("user-2"), "response"),
	})
	selector := NewTreeSelectorComponent(tree, "asst-2")
	list := selector.GetTreeList()
	selector.HandleInput("\x1b[1;5D")
	assertTreeSelected(t, list, "user-1")
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x1b[B")
	assertTreeSelected(t, list, "user-1")
}

func TestTreeSelectorSearchAndFilterResetFoldState(t *testing.T) {
	selector := NewTreeSelectorComponent(buildBranchingTreeSelectorTree(), "asst-4a")
	list := selector.GetTreeList()
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x1b[B")
	assertTreeSelected(t, list, "user-3b")
	selector.HandleInput("b")
	selector.HandleInput("\x1b")
	moveUntilSelected(selector, list, "user-3a")
	selector.HandleInput("\x1b[B")
	assertTreeSelected(t, list, "asst-3a")

	selector = NewTreeSelectorComponent(buildBranchingTreeSelectorTree(), "asst-4a")
	list = selector.GetTreeList()
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x1b[1;5D")
	selector.HandleInput("\x15")
	selector.HandleInput("\x04")
	moveUntilSelected(selector, list, "user-3a")
	selector.HandleInput("\x1b[B")
	assertTreeSelected(t, list, "asst-3a")
}

func TestTreeSelectorUsesEffectiveKeybindingsPiStyle(t *testing.T) {
	keybindings := mergeKeybindingsConfig(DefaultProtocolKeybindings(), KeybindingsConfig{
		"app.tree.foldOrUp":             "x",
		"app.tree.unfoldOrDown":         "y",
		"app.tree.filter.userOnly":      "r",
		"app.tree.filter.default":       "e",
		"app.tree.filter.noTools":       "n",
		"app.tree.filter.labeledOnly":   "l",
		"app.tree.filter.all":           "q",
		"app.tree.filter.cycleForward":  "o",
		"app.tree.filter.cycleBackward": "p",
		"app.tree.toggleLabelTimestamp": "z",
	})

	selector := NewTreeSelectorComponent(buildBranchingTreeSelectorTree(), "asst-4a", TreeSelectorOptions{Keybindings: keybindings})
	list := selector.GetTreeList()
	selector.HandleInput("x")
	assertTreeSelected(t, list, "user-3a")
	selector.HandleInput("y")
	assertTreeSelected(t, list, "asst-4a")
	render := strings.Join(selector.Render(200), "\n")
	for _, want := range []string{"x/y branch", "filters e/n/r/l/q", "z label time"} {
		if !strings.Contains(render, want) {
			t.Fatalf("render missing %q:\n%s", want, render)
		}
	}

	tree := buildTreeSelectorTree([]FileEntry{
		treeUser("user-1", nil, "hello"),
		treeToolCallAssistant("tool-asst-1", ptrString("user-1")),
		treeUser("user-2", ptrString("tool-asst-1"), "follow up"),
		treeAssistant("asst-2", ptrString("user-2"), "response"),
	})
	tree[0].Label = "checkpoint"
	tree[0].LabelTimestamp = time.Date(2026, 3, 28, 14, 32, 0, 0, time.Local).Format(time.RFC3339)
	selector = NewTreeSelectorComponent(tree, "asst-2", TreeSelectorOptions{Keybindings: keybindings})
	list = selector.GetTreeList()

	selector.HandleInput("q")
	render = strings.Join(list.Render(200), "\n")
	if strings.Contains(StripAnsi(render), "assistant: (no content)") || !strings.Contains(StripAnsi(render), "[all]") {
		t.Fatalf("all filter render = %q, want Pi-style tool-only assistant suppression and all status", render)
	}
	selector.HandleInput("n")
	render = strings.Join(list.Render(200), "\n")
	if strings.Contains(StripAnsi(render), "assistant: (no content)") {
		t.Fatalf("no-tools filter render = %q, want tool-only assistant hidden", render)
	}
	selector.HandleInput("r")
	assertTreeSelected(t, list, "user-2")
	selector.HandleInput("e")
	assertTreeSelected(t, list, "user-2")
	selector.HandleInput("l")
	assertTreeSelected(t, list, "user-1")
	render = strings.Join(list.Render(200), "\n")
	if !strings.Contains(render, "[checkpoint]") || strings.Contains(render, "[+label time]") {
		t.Fatalf("label filter render = %q", render)
	}
	selector.HandleInput("z")
	render = strings.Join(list.Render(200), "\n")
	if !strings.Contains(render, "[+label time]") {
		t.Fatalf("label timestamp render = %q", render)
	}
	selector.HandleInput("o")
	render = strings.Join(list.Render(200), "\n")
	if strings.Contains(StripAnsi(render), "assistant: (no content)") || !strings.Contains(StripAnsi(render), "[all]") {
		t.Fatalf("cycle forward render = %q, want all filter with Pi-style tool-only suppression", render)
	}
	selector.HandleInput("p")
	assertTreeSelected(t, list, "user-1")
}

func buildBranchingTreeSelectorTree() []*SessionTreeNode {
	return buildTreeSelectorTree([]FileEntry{
		treeUser("user-1", nil, "first message"),
		treeAssistant("asst-1", ptrString("user-1"), "response 1"),
		treeUser("user-2", ptrString("asst-1"), "second message"),
		treeAssistant("asst-2", ptrString("user-2"), "response 2"),
		treeUser("user-3a", ptrString("asst-2"), "branch A start"),
		treeAssistant("asst-3a", ptrString("user-3a"), "branch A response"),
		treeUser("user-4a", ptrString("asst-3a"), "branch A deep"),
		treeAssistant("asst-4a", ptrString("user-4a"), "branch A leaf"),
		treeUser("user-3b", ptrString("asst-2"), "branch B start"),
		treeAssistant("asst-3b", ptrString("user-3b"), "branch B response"),
		treeUser("user-4b", ptrString("asst-3b"), "branch B deep"),
	})
}

func buildTreeSelectorTree(entries []FileEntry) []*SessionTreeNode {
	nodes := map[string]*SessionTreeNode{}
	for _, entry := range entries {
		copyEntry := entry
		nodes[entry.ID] = &SessionTreeNode{Entry: copyEntry}
	}
	var roots []*SessionTreeNode
	for _, entry := range entries {
		node := nodes[entry.ID]
		if entry.ParentID == nil {
			roots = append(roots, node)
			continue
		}
		parent := nodes[*entry.ParentID]
		if parent == nil {
			roots = append(roots, node)
			continue
		}
		parent.Children = append(parent.Children, node)
	}
	return roots
}

func treeUser(id string, parent *string, text string) FileEntry {
	return FileEntry{Type: "message", ID: id, ParentID: parent, Message: llm.UserMessageText(text), Timestamp: id}
}

func treeAssistant(id string, parent *string, text string) FileEntry {
	return FileEntry{Type: "message", ID: id, ParentID: parent, Message: llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text(text)}, StopReason: llm.StopReasonStop}, Timestamp: id}
}

func treeToolCallAssistant(id string, parent *string) FileEntry {
	return FileEntry{Type: "message", ID: id, ParentID: parent, Message: llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.ToolCall("tc-"+id, "read", map[string]any{"path": "test.ts"})}, StopReason: "toolUse"}, Timestamp: id}
}

func treeBash(id string, parent *string, command string) FileEntry {
	return FileEntry{
		Type:     "message",
		ID:       id,
		ParentID: parent,
		Message: llm.Message{
			Role:    "bashExecution",
			Content: []llm.ContentPart{llm.Text("Ran `" + command + "`\n\noutput")},
			Details: map[string]any{"command": command},
		},
		Timestamp: id,
	}
}

func treeModelChange(id string, parent *string) FileEntry {
	return FileEntry{Type: "model_change", ID: id, ParentID: parent, Provider: "anthropic", ModelID: "claude-sonnet-4", Timestamp: id}
}

func ptrString(value string) *string { return &value }

func assertTreeSelected(t *testing.T, list *TreeSelectorList, want string) {
	t.Helper()
	if list.GetSelectedNode() == nil || list.GetSelectedNode().Entry.ID != want {
		t.Fatalf("selected = %#v, want %s", list.GetSelectedNode(), want)
	}
}

func moveUntilSelected(selector *TreeSelectorComponent, list *TreeSelectorList, want string) {
	for i := 0; i < 20 && (list.GetSelectedNode() == nil || list.GetSelectedNode().Entry.ID != want); i++ {
		selector.HandleInput("\x1b[B")
	}
}
