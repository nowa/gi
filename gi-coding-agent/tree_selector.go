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
	TreeSelectorUserFilter    TreeSelectorFilter = "user-only"
	TreeSelectorLabelFilter   TreeSelectorFilter = "labeled-only"
	TreeSelectorAllFilter     TreeSelectorFilter = "all"
)

type treeSelectorState struct {
	selectedID          string
	filter              TreeSelectorFilter
	searchQuery         string
	folded              map[string]bool
	showLabelTimestamps bool
}

type treeSelectorGutter struct {
	position int
	show     bool
}

type treeSelectorFlatNode struct {
	node               *SessionTreeNode
	indent             int
	showConnector      bool
	isLast             bool
	gutters            []treeSelectorGutter
	isVirtualRootChild bool
}

type treeSelectorViewportRow struct {
	gutter     string
	body       string
	anchorCol  int
	bodyWidth  int
	isSelected bool
}

type treeSelectorLabelEditor struct {
	entryID string
	input   *gitui.Input
}

// TreeSelectorComponent owns the mutable presentation state for session-tree
// navigation. Session persistence and clipboard I/O stay behind host callbacks.
type TreeSelectorComponent struct {
	focus           gitui.FocusState
	roots           []*SessionTreeNode
	currentLeafID   string
	keybindings     KeybindingsConfig
	maxVisibleLines int
	parent          map[string]string
	nodes           map[string]*SessionTreeNode
	activePath      map[string]bool
	orderedNodes    []*SessionTreeNode
	visibleParent   map[string]string
	visibleChildren map[string][]string
	multipleRoots   bool
	state           treeSelectorState
	list            *TreeSelectorList
	labelEditor     *treeSelectorLabelEditor
	OnSelect        func(entryID string)
	OnCancel        func()
	OnCopy          func(text *string)
	OnLabelChange   func(entryID, label string) error
	OnError         func(error)
}

type TreeSelectorOptions struct {
	Keybindings     KeybindingsConfig
	MaxVisibleLines int
}

// TreeSelectorList exposes the current derived tree projection to callers that
// need selection, search, copy, or label operations.
type TreeSelectorList struct {
	selector *TreeSelectorComponent
	flat     []*treeSelectorFlatNode
	selected int
}

func NewTreeSelectorComponent(roots []*SessionTreeNode, currentLeafID string, options ...TreeSelectorOptions) *TreeSelectorComponent {
	keybindings := DefaultProtocolKeybindings()
	maxVisibleLines := 12
	if len(options) > 0 {
		if options[0].Keybindings != nil {
			keybindings = cloneKeybindingsConfig(options[0].Keybindings)
		}
		if options[0].MaxVisibleLines > 0 {
			maxVisibleLines = max(5, options[0].MaxVisibleLines)
		}
	}
	selector := &TreeSelectorComponent{
		roots:           roots,
		currentLeafID:   currentLeafID,
		keybindings:     keybindings,
		maxVisibleLines: maxVisibleLines,
		parent:          map[string]string{},
		nodes:           map[string]*SessionTreeNode{},
		activePath:      map[string]bool{},
		visibleParent:   map[string]string{},
		visibleChildren: map[string][]string{},
		state: treeSelectorState{
			selectedID: currentLeafID,
			filter:     TreeSelectorDefaultFilter,
			folded:     map[string]bool{},
		},
	}
	selector.list = &TreeSelectorList{selector: selector, selected: -1}
	selector.indexTree()
	selector.rebuild()
	return selector
}

func (s *TreeSelectorComponent) GetTreeList() *TreeSelectorList {
	if s == nil {
		return nil
	}
	return s.list
}

func (s *TreeSelectorComponent) SetFilter(filter TreeSelectorFilter) {
	if s != nil {
		s.setFilter(filter)
	}
}

// SetSelectedID moves selection to an entry or its nearest visible ancestor.
func (s *TreeSelectorComponent) SetSelectedID(entryID string) {
	if s == nil || strings.TrimSpace(entryID) == "" {
		return
	}
	s.state.selectedID = strings.TrimSpace(entryID)
	s.rebuild()
}

func (s *TreeSelectorComponent) Focused() bool {
	return s != nil && s.focus.Focused()
}

func (s *TreeSelectorComponent) SetFocused(focused bool) {
	if s == nil {
		return
	}
	s.focus.SetFocused(focused)
	if s.labelEditor != nil {
		s.labelEditor.input.SetFocused(focused)
	}
}

func (s *TreeSelectorComponent) Invalidate() {}

func (s *TreeSelectorComponent) HandleInput(input string) {
	if s == nil {
		return
	}
	if s.labelEditor != nil {
		s.handleLabelEditorInput(input)
		return
	}
	kb := gitui.GetKeybindings()
	switch {
	case kb.Matches(input, "tui.select.up"):
		s.move(-1)
	case kb.Matches(input, "tui.select.down"):
		s.move(1)
	case matchesKeybindingAction(input, s.keybindings, "app.tree.foldOrUp"):
		s.foldOrJumpLeft()
	case matchesKeybindingAction(input, s.keybindings, "app.tree.unfoldOrDown"):
		s.unfoldOrJumpRight()
	case kb.Matches(input, "tui.editor.cursorLeft") || kb.Matches(input, "tui.select.pageUp"):
		s.movePage(-1)
	case kb.Matches(input, "tui.editor.cursorRight") || kb.Matches(input, "tui.select.pageDown"):
		s.movePage(1)
	case kb.Matches(input, "tui.select.confirm"):
		if node := s.list.GetSelectedNode(); node != nil && s.OnSelect != nil {
			s.OnSelect(node.Entry.ID)
		}
	case matchesKeybindingAction(input, s.keybindings, "app.message.copy"):
		s.list.CopySelected()
	case kb.Matches(input, "tui.select.cancel"):
		if s.state.searchQuery != "" {
			s.state.searchQuery = ""
			s.clearFolds()
			s.rebuild()
		} else if s.OnCancel != nil {
			s.OnCancel()
		}
	case matchesKeybindingAction(input, s.keybindings, "app.tree.filter.default"):
		s.setFilter(TreeSelectorDefaultFilter)
	case matchesKeybindingAction(input, s.keybindings, "app.tree.filter.noTools"):
		s.toggleFilter(TreeSelectorNoToolsFilter)
	case matchesKeybindingAction(input, s.keybindings, "app.tree.filter.userOnly"):
		s.toggleFilter(TreeSelectorUserFilter)
	case matchesKeybindingAction(input, s.keybindings, "app.tree.filter.labeledOnly"):
		s.toggleFilter(TreeSelectorLabelFilter)
	case matchesKeybindingAction(input, s.keybindings, "app.tree.filter.all"):
		s.toggleFilter(TreeSelectorAllFilter)
	case matchesKeybindingAction(input, s.keybindings, "app.tree.filter.cycleForward"):
		s.cycleFilter(1)
	case matchesKeybindingAction(input, s.keybindings, "app.tree.filter.cycleBackward"):
		s.cycleFilter(-1)
	case kb.Matches(input, "tui.editor.deleteCharBackward"):
		runes := []rune(s.state.searchQuery)
		if len(runes) > 0 {
			s.state.searchQuery = string(runes[:len(runes)-1])
			s.clearFolds()
			s.rebuild()
		}
	case matchesKeybindingAction(input, s.keybindings, "app.tree.editLabel"):
		s.showLabelEditor()
	case matchesKeybindingAction(input, s.keybindings, "app.tree.toggleLabelTimestamp"):
		s.state.showLabelTimestamps = !s.state.showLabelTimestamps
	default:
		if isTreeSelectorSearchInput(input) {
			s.state.searchQuery += input
			s.clearFolds()
			s.rebuild()
		}
	}
}

func (s *TreeSelectorComponent) showLabelEditor() {
	node := s.list.GetSelectedNode()
	if node == nil {
		return
	}
	input := gitui.NewInput()
	input.SetText(node.Label)
	input.SetFocused(s.Focused())
	s.labelEditor = &treeSelectorLabelEditor{entryID: node.Entry.ID, input: input}
}

func (s *TreeSelectorComponent) handleLabelEditorInput(input string) {
	if s == nil || s.labelEditor == nil {
		return
	}
	kb := gitui.GetKeybindings()
	switch {
	case kb.Matches(input, "tui.select.confirm"):
		editor := s.labelEditor
		label := strings.TrimSpace(editor.input.GetValue())
		if s.OnLabelChange != nil {
			if err := s.OnLabelChange(editor.entryID, label); err != nil {
				if s.OnError != nil {
					s.OnError(err)
				}
				return
			}
		}
		s.list.UpdateNodeLabel(editor.entryID, label)
		s.hideLabelEditor()
	case kb.Matches(input, "tui.select.cancel"):
		s.hideLabelEditor()
	default:
		s.labelEditor.input.HandleInput(input)
	}
}

func (s *TreeSelectorComponent) hideLabelEditor() {
	if s != nil {
		s.labelEditor = nil
	}
}

func (l *TreeSelectorList) Invalidate() {}

func (l *TreeSelectorList) GetSearchQuery() string {
	if l == nil || l.selector == nil {
		return ""
	}
	return l.selector.state.searchQuery
}

func (l *TreeSelectorList) GetSelectedNode() *SessionTreeNode {
	if l == nil || l.selected < 0 || l.selected >= len(l.flat) {
		return nil
	}
	return l.flat[l.selected].node
}

func (l *TreeSelectorList) CopySelected() {
	if l == nil || l.selector == nil || l.selector.OnCopy == nil {
		return
	}
	text, ok := l.getEntryCopyText(l.GetSelectedNode())
	if !ok {
		l.selector.OnCopy(nil)
		return
	}
	l.selector.OnCopy(&text)
}

func (l *TreeSelectorList) UpdateNodeLabel(entryID, label string) {
	if l == nil || l.selector == nil {
		return
	}
	node := l.selector.nodes[entryID]
	if node == nil {
		return
	}
	node.Label = label
	if label == "" {
		node.LabelTimestamp = ""
	} else {
		node.LabelTimestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	l.selector.rebuild()
}

func (s *TreeSelectorComponent) indexTree() {
	var index func(node *SessionTreeNode, parentID string)
	index = func(node *SessionTreeNode, parentID string) {
		if node == nil {
			return
		}
		s.nodes[node.Entry.ID] = node
		if parentID != "" {
			s.parent[node.Entry.ID] = parentID
		}
		for _, child := range node.Children {
			index(child, node.Entry.ID)
		}
	}
	for _, root := range s.roots {
		index(root, "")
	}
	for id := s.currentLeafID; id != ""; id = s.parent[id] {
		s.activePath[id] = true
	}
	var appendOrdered func(nodes []*SessionTreeNode)
	appendOrdered = func(nodes []*SessionTreeNode) {
		for _, active := range []bool{true, false} {
			for _, node := range nodes {
				if node == nil || s.activePath[node.Entry.ID] != active {
					continue
				}
				s.orderedNodes = append(s.orderedNodes, node)
				appendOrdered(node.Children)
			}
		}
	}
	appendOrdered(s.roots)
}

func (s *TreeSelectorComponent) rebuild() {
	if s == nil || s.list == nil {
		return
	}
	flat := make([]*treeSelectorFlatNode, 0, len(s.orderedNodes))
	for _, node := range s.orderedNodes {
		if !s.nodeVisible(node) || s.hiddenByFold(node.Entry.ID) {
			continue
		}
		flat = append(flat, &treeSelectorFlatNode{node: node})
	}
	s.list.flat = flat
	s.recalculateVisualStructure()
	s.list.selected = s.selectedIndex(flat)
	if s.list.selected >= 0 {
		s.state.selectedID = flat[s.list.selected].node.Entry.ID
	}
}

func (s *TreeSelectorComponent) selectedIndex(flat []*treeSelectorFlatNode) int {
	if len(flat) == 0 {
		return -1
	}
	indexByID := make(map[string]int, len(flat))
	for index, node := range flat {
		indexByID[node.node.Entry.ID] = index
	}
	for id := s.state.selectedID; id != ""; id = s.parent[id] {
		if index, ok := indexByID[id]; ok {
			return index
		}
	}
	return len(flat) - 1
}

func (s *TreeSelectorComponent) nodeVisible(node *SessionTreeNode) bool {
	if node == nil {
		return false
	}
	entry := node.Entry
	if entry.Type == "message" &&
		sessionMessageRole(entry.Message) == llm.RoleAssistant &&
		entry.ID != s.currentLeafID &&
		!treeSelectorHasTextContent(entry.Message) {
		stopReason := treeSelectorMessageStopReason(entry.Message)
		if stopReason == "" || stopReason == llm.StopReasonStop || stopReason == llm.StopReasonToolUse {
			return false
		}
	}
	isSettingsEntry := entry.Type == "label" ||
		entry.Type == "custom" ||
		entry.Type == "model_change" ||
		entry.Type == "thinking_level_change" ||
		entry.Type == "session_info"
	switch s.state.filter {
	case TreeSelectorUserFilter:
		if entry.Type != "message" || sessionMessageRole(entry.Message) != llm.RoleUser {
			return false
		}
	case TreeSelectorLabelFilter:
		if node.Label == "" {
			return false
		}
	case TreeSelectorNoToolsFilter:
		if isSettingsEntry || (entry.Type == "message" && sessionMessageRole(entry.Message) == llm.RoleToolResult) {
			return false
		}
	case TreeSelectorAllFilter:
	default:
		if isSettingsEntry {
			return false
		}
	}
	tokens := strings.Fields(strings.ToLower(s.state.searchQuery))
	if len(tokens) == 0 {
		return true
	}
	searchable := strings.ToLower(s.searchableText(node))
	for _, token := range tokens {
		if !strings.Contains(searchable, token) {
			return false
		}
	}
	return true
}

func (s *TreeSelectorComponent) searchableText(node *SessionTreeNode) string {
	if node == nil {
		return ""
	}
	entry := node.Entry
	parts := []string{node.Label}
	switch entry.Type {
	case "message":
		parts = append(parts, sessionMessageRole(entry.Message), treeSelectorMessageContent(entry.Message, 1<<20))
		if sessionMessageRole(entry.Message) == "bashExecution" {
			parts = append(parts, treeSelectorBashCommand(entry.Message))
		}
	case "custom_message":
		parts = append(parts, entry.CustomType, treeSelectorExtractFullContent(entry.Content))
	case "compaction":
		parts = append(parts, "compaction", entry.Summary)
	case "branch_summary":
		parts = append(parts, "branch summary", entry.Summary)
	case "session_info":
		parts = append(parts, "title", entry.Name)
	case "model_change":
		parts = append(parts, "model", entry.ModelID)
	case "thinking_level_change":
		parts = append(parts, "thinking", entry.ThinkingLevel)
	case "custom":
		parts = append(parts, "custom", entry.CustomType)
	case "label":
		parts = append(parts, "label", entry.Label)
	}
	return strings.Join(parts, " ")
}

func (s *TreeSelectorComponent) hiddenByFold(entryID string) bool {
	for parentID := s.parent[entryID]; parentID != ""; parentID = s.parent[parentID] {
		if s.state.folded[parentID] {
			return true
		}
	}
	return false
}

func (s *TreeSelectorComponent) recalculateVisualStructure() {
	s.visibleParent = map[string]string{}
	s.visibleChildren = map[string][]string{"": {}}
	visible := make(map[string]bool, len(s.list.flat))
	byID := make(map[string]*treeSelectorFlatNode, len(s.list.flat))
	for _, node := range s.list.flat {
		id := node.node.Entry.ID
		visible[id] = true
		byID[id] = node
	}
	for _, node := range s.list.flat {
		id := node.node.Entry.ID
		parentID := s.parent[id]
		for parentID != "" && !visible[parentID] {
			parentID = s.parent[parentID]
		}
		s.visibleParent[id] = parentID
		s.visibleChildren[parentID] = append(s.visibleChildren[parentID], id)
	}
	roots := s.visibleChildren[""]
	s.multipleRoots = len(roots) > 1
	type stackItem struct {
		id                 string
		indent             int
		justBranched       bool
		showConnector      bool
		isLast             bool
		gutters            []treeSelectorGutter
		isVirtualRootChild bool
	}
	stack := make([]stackItem, 0, len(s.list.flat))
	rootIndent := 0
	if s.multipleRoots {
		rootIndent = 1
	}
	for index := len(roots) - 1; index >= 0; index-- {
		stack = append(stack, stackItem{
			id:                 roots[index],
			indent:             rootIndent,
			justBranched:       s.multipleRoots,
			showConnector:      s.multipleRoots,
			isLast:             index == len(roots)-1,
			isVirtualRootChild: s.multipleRoots,
		})
	}
	for len(stack) > 0 {
		item := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		node := byID[item.id]
		if node == nil {
			continue
		}
		node.indent = item.indent
		node.showConnector = item.showConnector
		node.isLast = item.isLast
		node.gutters = append([]treeSelectorGutter(nil), item.gutters...)
		node.isVirtualRootChild = item.isVirtualRootChild
		children := s.visibleChildren[item.id]
		multipleChildren := len(children) > 1
		childIndent := item.indent
		if multipleChildren || (item.justBranched && item.indent > 0) {
			childIndent++
		}
		childGutters := item.gutters
		if item.showConnector && !item.isVirtualRootChild {
			displayIndent := item.indent
			if s.multipleRoots {
				displayIndent = max(0, displayIndent-1)
			}
			childGutters = append(append([]treeSelectorGutter(nil), item.gutters...), treeSelectorGutter{
				position: max(0, displayIndent-1),
				show:     !item.isLast,
			})
		}
		for index := len(children) - 1; index >= 0; index-- {
			stack = append(stack, stackItem{
				id:            children[index],
				indent:        childIndent,
				justBranched:  multipleChildren,
				showConnector: multipleChildren,
				isLast:        index == len(children)-1,
				gutters:       childGutters,
			})
		}
	}
}

func (s *TreeSelectorComponent) move(delta int) {
	if len(s.list.flat) == 0 {
		return
	}
	next := (s.list.selected + delta + len(s.list.flat)) % len(s.list.flat)
	s.list.selected = next
	s.state.selectedID = s.list.flat[next].node.Entry.ID
}

func (s *TreeSelectorComponent) movePage(direction int) {
	if len(s.list.flat) == 0 {
		return
	}
	next := s.list.selected + direction*s.maxVisibleLines
	next = max(0, min(next, len(s.list.flat)-1))
	s.list.selected = next
	s.state.selectedID = s.list.flat[next].node.Entry.ID
}

func (s *TreeSelectorComponent) clearFolds() {
	s.state.folded = map[string]bool{}
}

func (s *TreeSelectorComponent) setFilter(filter TreeSelectorFilter) {
	s.state.filter = filter
	s.clearFolds()
	s.rebuild()
}

func (s *TreeSelectorComponent) toggleFilter(filter TreeSelectorFilter) {
	if s.state.filter == filter {
		filter = TreeSelectorDefaultFilter
	}
	s.setFilter(filter)
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
		if s.state.filter == filter {
			index = candidateIndex
			break
		}
	}
	s.setFilter(order[(index+delta+len(order))%len(order)])
}

func (s *TreeSelectorComponent) foldOrJumpLeft() {
	node := s.list.GetSelectedNode()
	if node == nil {
		return
	}
	id := node.Entry.ID
	if s.isFoldable(id) && !s.state.folded[id] {
		s.state.folded[id] = true
		s.rebuild()
		return
	}
	s.selectVisibleIndex(s.findBranchSegmentStart(-1))
}

func (s *TreeSelectorComponent) unfoldOrJumpRight() {
	node := s.list.GetSelectedNode()
	if node == nil {
		return
	}
	id := node.Entry.ID
	if s.state.folded[id] {
		delete(s.state.folded, id)
		s.rebuild()
		return
	}
	s.selectVisibleIndex(s.findBranchSegmentStart(1))
}

func (s *TreeSelectorComponent) isFoldable(entryID string) bool {
	children := s.visibleChildren[entryID]
	if len(children) == 0 {
		return false
	}
	parentID := s.visibleParent[entryID]
	return parentID == "" || len(s.visibleChildren[parentID]) > 1
}

func (s *TreeSelectorComponent) findBranchSegmentStart(direction int) int {
	if s.list.selected < 0 || s.list.selected >= len(s.list.flat) {
		return s.list.selected
	}
	indexByID := make(map[string]int, len(s.list.flat))
	for index, node := range s.list.flat {
		indexByID[node.node.Entry.ID] = index
	}
	currentID := s.list.flat[s.list.selected].node.Entry.ID
	if direction > 0 {
		for {
			children := s.visibleChildren[currentID]
			if len(children) == 0 {
				return indexByID[currentID]
			}
			if len(children) > 1 {
				return indexByID[children[0]]
			}
			currentID = children[0]
		}
	}
	for {
		parentID := s.visibleParent[currentID]
		if parentID == "" {
			return indexByID[currentID]
		}
		if len(s.visibleChildren[parentID]) > 1 {
			index := indexByID[currentID]
			if index < s.list.selected {
				return index
			}
		}
		currentID = parentID
	}
}

func (s *TreeSelectorComponent) selectVisibleIndex(index int) {
	if index < 0 || index >= len(s.list.flat) {
		return
	}
	s.list.selected = index
	s.state.selectedID = s.list.flat[index].node.Entry.ID
}

func isTreeSelectorSearchInput(input string) bool {
	if input == "" {
		return false
	}
	for _, char := range input {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
}

func treeSelectorHasTextContent(message any) bool {
	switch typed := message.(type) {
	case llm.Message:
		for _, part := range typed.Content {
			if part.Type == llm.ContentText && strings.TrimSpace(part.Text) != "" {
				return true
			}
		}
	case map[string]any:
		switch content := typed["content"].(type) {
		case string:
			return strings.TrimSpace(content) != ""
		case []llm.ContentPart:
			for _, part := range content {
				if part.Type == llm.ContentText && strings.TrimSpace(part.Text) != "" {
					return true
				}
			}
		case []any:
			for _, item := range content {
				block, ok := item.(map[string]any)
				if !ok || block["type"] != llm.ContentText {
					continue
				}
				text, _ := block["text"].(string)
				if strings.TrimSpace(text) != "" {
					return true
				}
			}
		}
	}
	return false
}

func treeSelectorExtractFullContent(content any) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []llm.ContentPart:
		var result strings.Builder
		for _, part := range typed {
			if part.Type == llm.ContentText {
				result.WriteString(part.Text)
			}
		}
		return result.String()
	case []any:
		var result strings.Builder
		for _, item := range typed {
			block, ok := item.(map[string]any)
			if !ok || block["type"] != llm.ContentText {
				continue
			}
			text, _ := block["text"].(string)
			result.WriteString(text)
		}
		return result.String()
	default:
		return ""
	}
}

func (l *TreeSelectorList) getEntryCopyText(node *SessionTreeNode) (string, bool) {
	if node == nil {
		return "", false
	}
	entry := node.Entry
	var text string
	switch entry.Type {
	case "message":
		if sessionMessageRole(entry.Message) == "bashExecution" {
			text = treeSelectorBashCommand(entry.Message)
			break
		}
		switch message := entry.Message.(type) {
		case llm.Message:
			text = treeSelectorExtractFullContent(message.Content)
			if text == "" && message.Role == llm.RoleAssistant {
				text = message.ErrorMessage
			}
		case map[string]any:
			text = treeSelectorExtractFullContent(message["content"])
			if text == "" && sessionMessageRole(message) == llm.RoleAssistant {
				text, _ = message["errorMessage"].(string)
			}
		}
	case "custom_message":
		text = treeSelectorExtractFullContent(entry.Content)
	case "compaction", "branch_summary":
		text = entry.Summary
	}
	return text, strings.TrimSpace(text) != ""
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
	entry := node.Entry
	switch entry.Type {
	case "message":
		return treeSelectorMessageDisplayText(entry.Message)
	case "custom_message":
		return tuiThemeFG("customMessageLabel", "["+entry.CustomType+"]: ") + treeSelectorNormalizeText(treeSelectorContentText(entry.Content, 200))
	case "compaction":
		tokens := (entry.TokensBefore + 500) / 1000
		return tuiThemeFG("borderAccent", fmt.Sprintf("[compaction: %dk tokens]", tokens))
	case "branch_summary":
		return tuiThemeWarning("[branch summary]: ") + treeSelectorNormalizeText(entry.Summary)
	case "model_change":
		return tuiThemeDim("[model: " + entry.ModelID + "]")
	case "thinking_level_change":
		return tuiThemeDim("[thinking: " + entry.ThinkingLevel + "]")
	case "custom":
		return tuiThemeDim("[custom: " + entry.CustomType + "]")
	case "label":
		return tuiThemeDim("[label: " + firstNonEmptyString(entry.Label, "(cleared)") + "]")
	case "session_info":
		if strings.TrimSpace(entry.Name) == "" {
			return tuiThemeDim("[title: ") + tuiThemeItalic(tuiThemeDim("empty")) + tuiThemeDim("]")
		}
		return tuiThemeDim("[title: ") + tuiThemeDim(entry.Name) + tuiThemeDim("]")
	default:
		return ""
	}
}

func treeSelectorMessageDisplayText(message any) string {
	role := sessionMessageRole(message)
	switch role {
	case llm.RoleUser:
		return tuiThemeAccent("user: ") + treeSelectorNormalizeText(treeSelectorMessageContent(message, 200))
	case llm.RoleAssistant:
		prefix := tuiThemeSuccess("assistant: ")
		text := treeSelectorNormalizeText(treeSelectorMessageContent(message, 200))
		if text != "" {
			return prefix + text
		}
		if treeSelectorMessageStopReason(message) == llm.StopReasonAborted {
			return prefix + tuiThemeMuted("(aborted)")
		}
		if errMsg := treeSelectorNormalizeText(treeSelectorMessageError(message)); errMsg != "" {
			return prefix + tuiThemeError(treeSelectorTruncateText(errMsg, 80))
		}
		return prefix + tuiThemeMuted("(no content)")
	case llm.RoleToolResult:
		return tuiThemeMuted("[" + firstNonEmptyString(treeSelectorMessageToolName(message), "tool") + "]")
	case "bashExecution":
		return tuiThemeDim("[bash]: " + treeSelectorNormalizeText(treeSelectorBashCommand(message)))
	case "":
		return ""
	default:
		return tuiThemeDim("[" + role + "]")
	}
}

func treeSelectorMessageContent(message any, maxLen int) string {
	switch typed := message.(type) {
	case llm.Message:
		return treeSelectorContentText(typed.Content, maxLen)
	case map[string]any:
		return treeSelectorContentText(typed["content"], maxLen)
	default:
		return ""
	}
}

func treeSelectorContentText(content any, maxLen int) string {
	switch typed := content.(type) {
	case string:
		return treeSelectorTruncateText(typed, maxLen)
	case []llm.ContentPart:
		var builder strings.Builder
		for _, part := range typed {
			if part.Type != llm.ContentText {
				continue
			}
			builder.WriteString(part.Text)
			if treeSelectorTextLen(builder.String()) >= maxLen {
				return treeSelectorTruncateText(builder.String(), maxLen)
			}
		}
		return builder.String()
	case []any:
		var builder strings.Builder
		for _, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			blockType, _ := block["type"].(string)
			if blockType != llm.ContentText {
				continue
			}
			text, _ := block["text"].(string)
			builder.WriteString(text)
			if treeSelectorTextLen(builder.String()) >= maxLen {
				return treeSelectorTruncateText(builder.String(), maxLen)
			}
		}
		return builder.String()
	default:
		return ""
	}
}

func treeSelectorNormalizeText(text string) string {
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\t", " ")
	return strings.TrimSpace(text)
}

func treeSelectorTruncateText(text string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if treeSelectorTextLen(text) <= maxLen {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen])
}

func treeSelectorTextLen(text string) int {
	return len([]rune(text))
}

func treeSelectorMessageStopReason(message any) string {
	switch typed := message.(type) {
	case llm.Message:
		return typed.StopReason
	case map[string]any:
		stopReason, _ := typed["stopReason"].(string)
		return stopReason
	default:
		return ""
	}
}

func treeSelectorMessageError(message any) string {
	switch typed := message.(type) {
	case llm.Message:
		return typed.ErrorMessage
	case map[string]any:
		errorMessage, _ := typed["errorMessage"].(string)
		return errorMessage
	default:
		return ""
	}
}

func treeSelectorMessageToolName(message any) string {
	switch typed := message.(type) {
	case llm.Message:
		return typed.ToolName
	case map[string]any:
		toolName, _ := typed["toolName"].(string)
		return toolName
	default:
		return ""
	}
}

func treeSelectorBashCommand(message any) string {
	switch typed := message.(type) {
	case map[string]any:
		if command, _ := typed["command"].(string); strings.TrimSpace(command) != "" {
			return strings.TrimSpace(command)
		}
	case llm.Message:
		if details, ok := typed.Details.(map[string]any); ok {
			if command, _ := details["command"].(string); strings.TrimSpace(command) != "" {
				return strings.TrimSpace(command)
			}
		}
	}
	return extractBashCommandFromExecutionText(sessionMessageText(message))
}

func extractBashCommandFromExecutionText(text string) string {
	text = strings.TrimSpace(text)
	const prefix = "Ran `"
	if !strings.HasPrefix(text, prefix) {
		return text
	}
	rest := strings.TrimPrefix(text, prefix)
	if end := strings.Index(rest, "`"); end >= 0 {
		return strings.TrimSpace(rest[:end])
	}
	return strings.TrimSpace(rest)
}

func formatTreeSelectorLabelTimestamp(value string) string {
	return formatTreeSelectorLabelTimestampAt(value, time.Now())
}

func formatTreeSelectorLabelTimestampAt(value string, now time.Time) string {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	parsed = parsed.In(time.Local)
	now = now.In(time.Local)
	if parsed.Year() == now.Year() && parsed.YearDay() == now.YearDay() {
		return parsed.Format("15:04")
	}
	if parsed.Year() == now.Year() {
		return parsed.Format("1/2 15:04")
	}
	return parsed.Format("06/1/2 15:04")
}

func treeSelectorBorder(width int) string {
	return tuiThemeBorder(strings.Repeat("─", max(1, width)))
}
