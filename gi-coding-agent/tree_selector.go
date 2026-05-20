package gicodingagent

import (
	"fmt"
	"time"
	"unicode"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type TreeSelectorFilter string

const (
	TreeSelectorDefaultFilter TreeSelectorFilter = "default"
	TreeSelectorUserFilter    TreeSelectorFilter = "user"
	TreeSelectorLabelFilter   TreeSelectorFilter = "label"
)

type TreeSelectorComponent struct {
	roots               []*SessionTreeNode
	currentLeafID       string
	selectedID          string
	filter              TreeSelectorFilter
	folded              map[string]bool
	parent              map[string]string
	nodes               map[string]*SessionTreeNode
	activePath          map[string]bool
	list                *TreeSelectorList
	showLabelTimestamps bool
}

type TreeSelectorList struct {
	selector *TreeSelectorComponent
	flat     []*SessionTreeNode
	selected int
}

func NewTreeSelectorComponent(roots []*SessionTreeNode, currentLeafID string) *TreeSelectorComponent {
	selector := &TreeSelectorComponent{
		roots:         roots,
		currentLeafID: currentLeafID,
		selectedID:    currentLeafID,
		filter:        TreeSelectorDefaultFilter,
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

func (s *TreeSelectorComponent) HandleInput(input string) {
	switch input {
	case "\x1b[A":
		s.move(-1)
	case "\x1b[B":
		s.move(1)
	case "\x1b[1;5D", "\x1b[1;3D":
		s.foldOrJumpLeft()
	case "\x1b[1;5C", "\x1b[1;3C":
		s.unfoldOrJumpRight()
	case "\x15":
		s.setFilter(TreeSelectorUserFilter)
	case "\x04":
		s.setFilter(TreeSelectorDefaultFilter)
	case "\x0c":
		if s.filter == TreeSelectorLabelFilter {
			s.setFilter(TreeSelectorDefaultFilter)
		} else {
			s.setFilter(TreeSelectorLabelFilter)
		}
	case "T":
		s.showLabelTimestamps = !s.showLabelTimestamps
		s.rebuild()
	case "\x1b":
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
		if width > 0 && len([]rune(text)) > width {
			runes := []rune(text)
			text = string(runes[:width])
		}
		lines = append(lines, text)
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
	default:
		if node.Entry.Type != "message" && node.Entry.Type != "custom_message" && node.Entry.Type != "branch_summary" {
			return false
		}
		if node.Entry.Type == "message" && sessionMessageRole(node.Entry.Message) == llm.RoleAssistant && treeSelectorToolCallOnly(node.Entry.Message) {
			return false
		}
		return true
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
