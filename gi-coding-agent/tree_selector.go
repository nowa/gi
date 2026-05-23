package gicodingagent

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

type TreeSelectorFilter string

const (
	TreeSelectorDefaultFilter TreeSelectorFilter = "default"
	TreeSelectorNoToolsFilter TreeSelectorFilter = "no-tools"
	TreeSelectorUserFilter    TreeSelectorFilter = "user"
	TreeSelectorLabelFilter   TreeSelectorFilter = "label"
	TreeSelectorAllFilter     TreeSelectorFilter = "all"
)

type TreeSelectorComponent struct {
	focus               gitui.FocusState
	roots               []*SessionTreeNode
	currentLeafID       string
	selectedID          string
	filter              TreeSelectorFilter
	keybindings         KeybindingsConfig
	folded              map[string]bool
	parent              map[string]string
	nodes               map[string]*SessionTreeNode
	activePath          map[string]bool
	list                *TreeSelectorList
	showLabelTimestamps bool
	OnSelect            func(entryID string)
	OnCancel            func()
}

type TreeSelectorOptions struct {
	Keybindings KeybindingsConfig
}

type TreeSelectorList struct {
	selector *TreeSelectorComponent
	flat     []*SessionTreeNode
	selected int
}

func NewTreeSelectorComponent(roots []*SessionTreeNode, currentLeafID string, options ...TreeSelectorOptions) *TreeSelectorComponent {
	keybindings := DefaultProtocolKeybindings()
	if len(options) > 0 && options[0].Keybindings != nil {
		keybindings = cloneKeybindingsConfig(options[0].Keybindings)
	}
	selector := &TreeSelectorComponent{
		roots:         roots,
		currentLeafID: currentLeafID,
		selectedID:    currentLeafID,
		filter:        TreeSelectorDefaultFilter,
		keybindings:   keybindings,
		folded:        map[string]bool{},
		parent:        map[string]string{},
		nodes:         map[string]*SessionTreeNode{},
		activePath:    map[string]bool{},
	}
	selector.list = &TreeSelectorList{selector: selector}
	selector.indexTree()
	selector.rebuild()
	return selector
}

func (s *TreeSelectorComponent) GetTreeList() *TreeSelectorList {
	return s.list
}

func (s *TreeSelectorComponent) SetFilter(filter TreeSelectorFilter) {
	if s == nil {
		return
	}
	s.setFilter(filter)
}

func (s *TreeSelectorComponent) Focused() bool {
	if s == nil {
		return false
	}
	return s.focus.Focused()
}

func (s *TreeSelectorComponent) SetFocused(focused bool) {
	if s != nil {
		s.focus.SetFocused(focused)
	}
}

func (s *TreeSelectorComponent) Invalidate() {}

func (s *TreeSelectorComponent) Render(width int) []string {
	if s == nil {
		return nil
	}
	width = max(24, width)
	lines := []string{
		"",
		treeSelectorBorder(width),
		gitui.TruncateToWidth("  Session Tree", width, "", true),
	}
	for _, hint := range s.footerHints() {
		lines = append(lines, gitui.TruncateToWidth(hint, width, "", true))
	}
	lines = append(lines,
		treeSelectorBorder(width),
		"",
	)
	lines = append(lines, s.list.Render(width)...)
	lines = append(lines, "", treeSelectorBorder(width))
	return lines
}

func (s *TreeSelectorComponent) footerHints() []string {
	keybindings := DefaultProtocolKeybindings()
	if s != nil && s.keybindings != nil {
		keybindings = s.keybindings
	}
	upDown := formatHotkeyKeys(append(gitui.GetKeybindings().GetKeys("tui.select.up"), gitui.GetKeybindings().GetKeys("tui.select.down")...), true)
	confirm := formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.select.confirm"), true)
	cancel := formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.select.cancel"), true)
	fold := formatHotkeyKeys(keybindingValueKeys(keybindings["app.tree.foldOrUp"]), true)
	unfold := formatHotkeyKeys(keybindingValueKeys(keybindings["app.tree.unfoldOrDown"]), true)
	user := formatHotkeyKeys(keybindingValueKeys(keybindings["app.tree.filter.userOnly"]), true)
	defaultFilter := formatHotkeyKeys(keybindingValueKeys(keybindings["app.tree.filter.default"]), true)
	noTools := formatHotkeyKeys(keybindingValueKeys(keybindings["app.tree.filter.noTools"]), true)
	labels := formatHotkeyKeys(keybindingValueKeys(keybindings["app.tree.filter.labeledOnly"]), true)
	all := formatHotkeyKeys(keybindingValueKeys(keybindings["app.tree.filter.all"]), true)
	timestamps := formatHotkeyKeys(keybindingValueKeys(keybindings["app.tree.toggleLabelTimestamp"]), true)
	return []string{
		fmt.Sprintf("  %s move. %s switch. %s/%s fold. %s cancel.",
			firstNonEmptyString(upDown, "Up/Down"),
			firstNonEmptyString(confirm, "Enter"),
			firstNonEmptyString(fold, "Ctrl+Left"),
			firstNonEmptyString(unfold, "Ctrl+Right"),
			firstNonEmptyString(cancel, "Esc"),
		),
		fmt.Sprintf("  %s users. %s default. %s no-tools. %s labels. %s all. %s label time.",
			firstNonEmptyString(user, "Ctrl+U"),
			firstNonEmptyString(defaultFilter, "Ctrl+D"),
			firstNonEmptyString(noTools, "Ctrl+T"),
			firstNonEmptyString(labels, "Ctrl+L"),
			firstNonEmptyString(all, "Ctrl+A"),
			firstNonEmptyString(timestamps, "Shift+T"),
		),
	}
}

func (s *TreeSelectorComponent) HandleInput(input string) {
	kb := gitui.GetKeybindings()
	switch {
	case kb.Matches(input, "tui.select.up") || input == "k":
		s.move(-1)
	case kb.Matches(input, "tui.select.down") || input == "j":
		s.move(1)
	case matchesKeybindingAction(input, s.keybindings, "app.tree.foldOrUp") || input == "\x1b[1;5D" || input == "\x1b[1;3D":
		s.foldOrJumpLeft()
	case matchesKeybindingAction(input, s.keybindings, "app.tree.unfoldOrDown") || input == "\x1b[1;5C" || input == "\x1b[1;3C":
		s.unfoldOrJumpRight()
	case matchesKeybindingAction(input, s.keybindings, "app.tree.filter.userOnly") || input == "\x15":
		s.setFilter(TreeSelectorUserFilter)
	case matchesKeybindingAction(input, s.keybindings, "app.tree.filter.default") || input == "\x04":
		s.setFilter(TreeSelectorDefaultFilter)
	case matchesKeybindingAction(input, s.keybindings, "app.tree.filter.noTools"):
		s.setFilter(TreeSelectorNoToolsFilter)
	case matchesKeybindingAction(input, s.keybindings, "app.tree.filter.labeledOnly") || input == "\x0c":
		if s.filter == TreeSelectorLabelFilter {
			s.setFilter(TreeSelectorDefaultFilter)
		} else {
			s.setFilter(TreeSelectorLabelFilter)
		}
	case matchesKeybindingAction(input, s.keybindings, "app.tree.filter.all"):
		s.setFilter(TreeSelectorAllFilter)
	case matchesKeybindingAction(input, s.keybindings, "app.tree.filter.cycleForward"):
		s.cycleFilter(1)
	case matchesKeybindingAction(input, s.keybindings, "app.tree.filter.cycleBackward"):
		s.cycleFilter(-1)
	case matchesKeybindingAction(input, s.keybindings, "app.tree.toggleLabelTimestamp") || input == "T":
		s.showLabelTimestamps = !s.showLabelTimestamps
		s.rebuild()
	case kb.Matches(input, "tui.select.confirm"):
		if node := s.list.GetSelectedNode(); node != nil && s.OnSelect != nil {
			s.OnSelect(node.Entry.ID)
		}
	case kb.Matches(input, "tui.select.cancel"):
		if s.OnCancel != nil {
			s.OnCancel()
			return
		}
		s.folded = map[string]bool{}
		s.rebuild()
	default:
		if isTreeSelectorSearchInput(input) {
			s.folded = map[string]bool{}
			s.rebuild()
		}
	}
}

func (l *TreeSelectorList) GetSelectedNode() *SessionTreeNode {
	if l == nil || l.selected < 0 || l.selected >= len(l.flat) {
		return nil
	}
	return l.flat[l.selected]
}

func (l *TreeSelectorList) Render(width int) []string {
	if l == nil {
		return nil
	}
	lines := make([]string, 0, len(l.flat))
	for _, node := range l.flat {
		text := treeSelectorNodeText(node)
		if node.Label != "" {
			text += " [" + node.Label + "]"
			if l.selector.showLabelTimestamps && node.LabelTimestamp != "" {
				text += " " + formatTreeSelectorLabelTimestamp(node.LabelTimestamp) + " [+label time]"
			}
		}
		prefix := "  "
		if l.selected >= 0 && l.selected < len(l.flat) && l.flat[l.selected] == node {
			prefix = "> "
		}
		lines = append(lines, gitui.TruncateToWidth(prefix+text, width, "", true))
	}
	if len(l.flat) == 0 {
		lines = append(lines, gitui.TruncateToWidth("  No entries found", width, "", true))
	}
	if l.selected >= 0 && len(l.flat) > 0 {
		lines = append(lines, gitui.TruncateToWidth("  ("+fmt.Sprintf("%d/%d", l.selected+1, len(l.flat))+")", width, "", true))
	}
	return lines
}

func (s *TreeSelectorComponent) indexTree() {
	var walk func(node *SessionTreeNode, parentID string)
	walk = func(node *SessionTreeNode, parentID string) {
		if node == nil {
			return
		}
		s.nodes[node.Entry.ID] = node
		if parentID != "" {
			s.parent[node.Entry.ID] = parentID
		}
		for _, child := range node.Children {
			walk(child, node.Entry.ID)
		}
	}
	for _, root := range s.roots {
		walk(root, "")
	}
	for id := s.currentLeafID; id != ""; id = s.parent[id] {
		s.activePath[id] = true
	}
}

func (s *TreeSelectorComponent) rebuild() {
	var flat []*SessionTreeNode
	var walk func(node *SessionTreeNode, hidden bool)
	walk = func(node *SessionTreeNode, hidden bool) {
		if node == nil || hidden {
			return
		}
		visible := s.nodeVisible(node)
		if visible {
			flat = append(flat, node)
		}
		childHidden := visible && s.folded[node.Entry.ID]
		for _, child := range node.Children {
			walk(child, childHidden)
		}
	}
	for _, root := range s.roots {
		walk(root, false)
	}
	s.list.flat = flat
	s.list.selected = s.selectedIndex(flat)
}

func (s *TreeSelectorComponent) selectedIndex(flat []*SessionTreeNode) int {
	if len(flat) == 0 {
		return -1
	}
	for index, node := range flat {
		if node.Entry.ID == s.selectedID {
			return index
		}
	}
	for id := s.selectedID; id != ""; id = s.parent[id] {
		for index, node := range flat {
			if node.Entry.ID == id {
				s.selectedID = id
				return index
			}
		}
	}
	return 0
}

func (s *TreeSelectorComponent) nodeVisible(node *SessionTreeNode) bool {
	if node == nil {
		return false
	}
	switch s.filter {
	case TreeSelectorUserFilter:
		return node.Entry.Type == "message" && sessionMessageRole(node.Entry.Message) == llm.RoleUser
	case TreeSelectorLabelFilter:
		return node.Label != ""
	case TreeSelectorAllFilter:
		return node.Entry.Type == "message" || node.Entry.Type == "custom_message" || node.Entry.Type == "branch_summary"
	case TreeSelectorDefaultFilter, TreeSelectorNoToolsFilter:
		if node.Entry.Type != "message" && node.Entry.Type != "custom_message" && node.Entry.Type != "branch_summary" {
			return false
		}
		if node.Entry.Type == "message" && sessionMessageRole(node.Entry.Message) == llm.RoleAssistant && treeSelectorToolCallOnly(node.Entry.Message) {
			return false
		}
		return true
	default:
		return node.Entry.Type == "message" || node.Entry.Type == "custom_message" || node.Entry.Type == "branch_summary"
	}
}

func (s *TreeSelectorComponent) move(delta int) {
	if len(s.list.flat) == 0 {
		return
	}
	next := (s.list.selected + delta + len(s.list.flat)) % len(s.list.flat)
	s.list.selected = next
	s.selectedID = s.list.flat[next].Entry.ID
}

func (s *TreeSelectorComponent) setFilter(filter TreeSelectorFilter) {
	s.filter = filter
	s.folded = map[string]bool{}
	s.rebuild()
}

func (s *TreeSelectorComponent) cycleFilter(delta int) {
	order := []TreeSelectorFilter{
		TreeSelectorDefaultFilter,
		TreeSelectorNoToolsFilter,
		TreeSelectorUserFilter,
		TreeSelectorLabelFilter,
		TreeSelectorAllFilter,
	}
	index := 0
	for candidateIndex, filter := range order {
		if s.filter == filter {
			index = candidateIndex
			break
		}
	}
	next := (index + delta + len(order)) % len(order)
	s.setFilter(order[next])
}

func (s *TreeSelectorComponent) foldOrJumpLeft() {
	node := s.list.GetSelectedNode()
	if node == nil {
		return
	}
	segment := s.segmentStart(node.Entry.ID)
	if segment != "" && segment != node.Entry.ID {
		s.selectID(segment)
		return
	}
	if s.hasVisibleDescendant(node.Entry.ID) && !s.folded[node.Entry.ID] {
		s.folded[node.Entry.ID] = true
		s.rebuild()
		return
	}
	parent := s.parent[node.Entry.ID]
	if parent != "" {
		s.selectID(s.segmentStart(parent))
	}
}

func (s *TreeSelectorComponent) unfoldOrJumpRight() {
	node := s.list.GetSelectedNode()
	if node == nil {
		return
	}
	if s.folded[node.Entry.ID] {
		delete(s.folded, node.Entry.ID)
		s.rebuild()
		return
	}
	s.selectID(s.deepestSegmentLeaf(node.Entry.ID))
}

func (s *TreeSelectorComponent) selectID(id string) {
	if id == "" {
		return
	}
	s.selectedID = id
	s.rebuild()
}

func (s *TreeSelectorComponent) segmentStart(id string) string {
	chain := s.ancestorChain(id)
	if len(chain) == 0 {
		return id
	}
	segment := chain[0]
	for index := 1; index < len(chain); index++ {
		parentID := chain[index-1]
		if len(s.visibleChildSegments(parentID)) > 1 {
			segment = chain[index]
		}
	}
	for !s.nodeVisible(s.nodes[segment]) && segment != "" {
		children := s.visibleChildSegments(segment)
		if len(children) == 0 {
			break
		}
		segment = children[0].Entry.ID
	}
	return segment
}

func (s *TreeSelectorComponent) deepestSegmentLeaf(id string) string {
	current := id
	for {
		if s.folded[current] {
			return current
		}
		children := s.visibleChildSegments(current)
		if len(children) == 0 {
			return current
		}
		next := children[0]
		for _, child := range children {
			if s.subtreeContainsActivePath(child) {
				next = child
				break
			}
		}
		current = next.Entry.ID
	}
}

func (s *TreeSelectorComponent) visibleChildSegments(id string) []*SessionTreeNode {
	node := s.nodes[id]
	if node == nil {
		return nil
	}
	var result []*SessionTreeNode
	var collect func(child *SessionTreeNode)
	collect = func(child *SessionTreeNode) {
		if child == nil {
			return
		}
		if s.nodeVisible(child) {
			result = append(result, child)
			return
		}
		for _, grandchild := range child.Children {
			collect(grandchild)
		}
	}
	for _, child := range node.Children {
		collect(child)
	}
	return result
}

func (s *TreeSelectorComponent) hasVisibleDescendant(id string) bool {
	return len(s.visibleChildSegments(id)) > 0
}

func (s *TreeSelectorComponent) subtreeContainsActivePath(node *SessionTreeNode) bool {
	if node == nil {
		return false
	}
	if s.activePath[node.Entry.ID] {
		return true
	}
	for _, child := range node.Children {
		if s.subtreeContainsActivePath(child) {
			return true
		}
	}
	return false
}

func (s *TreeSelectorComponent) ancestorChain(id string) []string {
	if _, ok := s.nodes[id]; !ok {
		return nil
	}
	var reverse []string
	for current := id; current != ""; current = s.parent[current] {
		reverse = append(reverse, current)
	}
	chain := make([]string, len(reverse))
	for index := range reverse {
		chain[len(reverse)-1-index] = reverse[index]
	}
	return chain
}

func isTreeSelectorSearchInput(input string) bool {
	runes := []rune(input)
	return len(runes) == 1 && unicode.IsPrint(runes[0]) && !unicode.IsControl(runes[0])
}

func treeSelectorToolCallOnly(message any) bool {
	switch typed := message.(type) {
	case llm.Message:
		return len(typed.Content) > 0 && allTreeSelectorContentPartsAreToolCalls(typed.Content)
	case map[string]any:
		content, _ := typed["content"].([]any)
		if len(content) == 0 {
			return false
		}
		for _, item := range content {
			part, ok := item.(map[string]any)
			if !ok || part["type"] != llm.ContentToolCall {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func allTreeSelectorContentPartsAreToolCalls(parts []llm.ContentPart) bool {
	for _, part := range parts {
		if part.Type != llm.ContentToolCall {
			return false
		}
	}
	return true
}

func treeSelectorNodeText(node *SessionTreeNode) string {
	if node == nil {
		return ""
	}
	if node.Entry.Type == "branch_summary" {
		return node.Entry.Summary
	}
	text := sessionMessageText(node.Entry.Message)
	if text == "" {
		text = node.Entry.ID
	}
	return text
}

func formatTreeSelectorLabelTimestamp(value string) string {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fmt.Sprintf("%s", value)
	}
	return parsed.Format("1/2 15:04")
}

func treeSelectorBorder(width int) string {
	return gitui.TruncateToWidth(" "+strings.Repeat("-", max(0, width-1)), width, "", true)
}
