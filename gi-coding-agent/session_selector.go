package gicodingagent

import (
	"sort"
	"strings"
	"sync"
)

const (
	sessionSelectorCtrlR         = "\x1b[114;5u"
	sessionSelectorCtrlD         = "\x04"
	sessionSelectorCtrlBackspace = "\x1b[127;5u"
	sessionSelectorTab           = "\t"
	sessionSelectorConfirm       = "\r"
	sessionSelectorConfirmAlt    = "\n"
	sessionSelectorCancel        = "\x1b"
	sessionSelectorCurrentScope  = "current"
	sessionSelectorAllScope      = "all"
)

type SessionSelectorLoader func(SessionListProgress) ([]SessionInfo, error)

type SessionSelectorOptions struct {
	ShowRenameHint             bool
	RenameSession              func(path, name string) error
	DeleteSession              func(path string) error
	OnDeleteConfirmationChange func(path *string)
	OnError                    func(message string)
	CurrentSessionPath         string
}

type SessionSelectorComponent struct {
	mu              sync.Mutex
	sessions        []SessionInfo
	currentSessions []SessionInfo
	allSessions     []SessionInfo
	allLoaded       bool
	allLoading      bool
	allLoadSeq      int
	allLoader       SessionSelectorLoader
	options         SessionSelectorOptions
	selected        int
	scope           string
	sortMode        SessionSelectorSortMode
	nameFilter      SessionSelectorNameFilter
	searchQuery     string
	deleteConfirm   bool
	deletePath      string
	renameMode      bool
	renameText      string
	renameCursor    int
}

func NewSessionSelectorComponent(sessions []SessionInfo, options SessionSelectorOptions) *SessionSelectorComponent {
	cloned := cloneSessionInfos(sessions)
	return &SessionSelectorComponent{
		sessions:        cloned,
		currentSessions: cloneSessionInfos(cloned),
		options:         options,
		scope:           sessionSelectorCurrentScope,
		sortMode:        SessionSelectorSortThreaded,
		nameFilter:      SessionSelectorNameAll,
	}
}

func NewLoadingSessionSelectorComponent(currentLoader, allLoader SessionSelectorLoader, options SessionSelectorOptions) *SessionSelectorComponent {
	selector := &SessionSelectorComponent{
		options:    options,
		scope:      sessionSelectorCurrentScope,
		allLoader:  allLoader,
		sortMode:   SessionSelectorSortThreaded,
		nameFilter: SessionSelectorNameAll,
	}
	if currentLoader == nil {
		return selector
	}
	sessions, err := currentLoader(nil)
	if err != nil {
		if options.OnError != nil {
			options.OnError(err.Error())
		}
		return selector
	}
	selector.sessions = cloneSessionInfos(sessions)
	selector.currentSessions = cloneSessionInfos(sessions)
	return selector
}

func (s *SessionSelectorComponent) GetSessionList() *SessionSelectorComponent {
	return s
}

func (s *SessionSelectorComponent) Render(_ int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.renameMode {
		lines := []string{"Rename Session"}
		lines = append(lines, s.renameText)
		return lines
	}

	title := "Resume Session (Current Folder)"
	if s.scope == sessionSelectorAllScope {
		title = "Resume Session (All)"
	}
	lines := []string{title}
	if s.allLoading && s.scope == sessionSelectorAllScope {
		lines = append(lines, "Loading sessions...")
	}
	if s.options.ShowRenameHint {
		lines = append(lines, "ctrl+r rename")
	}
	if s.deleteConfirm {
		lines = append(lines, "Delete session? enter confirm · esc cancel")
	}

	nodes := s.visibleNodesLocked()
	for index, node := range nodes {
		session := node.session
		prefix := "  "
		if index == s.selected {
			prefix = "> "
		}
		name := session.Name
		if name == "" {
			name = session.FirstMessage
		}
		lines = append(lines, prefix+node.treePrefix()+name)
	}
	return lines
}

func (s *SessionSelectorComponent) HandleInput(input string) {
	var (
		deletePath        string
		deleteSession     func(string) error
		confirmPath       *string
		clearConfirm      bool
		errorMessage      string
		onError           func(string)
		shouldEmitConfirm bool
	)

	s.mu.Lock()
	if s.renameMode {
		s.handleRenameInputLocked(input)
		s.mu.Unlock()
		return
	}

	if s.deleteConfirm {
		if input == sessionSelectorConfirm || input == sessionSelectorConfirmAlt {
			deletePath = s.deletePath
			deleteSession = s.options.DeleteSession
			s.deleteConfirm = false
			s.deletePath = ""
			clearConfirm = true
		} else if input == sessionSelectorCancel {
			s.deleteConfirm = false
			s.deletePath = ""
			clearConfirm = true
		}
		s.mu.Unlock()
		if clearConfirm {
			s.emitDeleteConfirmation(nil)
		}
		if deletePath != "" && deleteSession != nil {
			_ = deleteSession(deletePath)
		}
		return
	}

	switch input {
	case sessionSelectorTab:
		s.mu.Unlock()
		s.toggleScope()
		return
	case sessionSelectorCtrlR:
		if s.options.ShowRenameHint && len(s.sessions) > 0 {
			s.renameMode = true
			s.renameText = s.sessions[s.selected].Name
			s.renameCursor = 0
		}
	case sessionSelectorCtrlD:
		confirmPath, errorMessage, onError = s.startDeleteConfirmationLocked()
		shouldEmitConfirm = confirmPath != nil
	case sessionSelectorCtrlBackspace:
		if s.searchQuery == "" {
			confirmPath, errorMessage, onError = s.startDeleteConfirmationLocked()
			shouldEmitConfirm = confirmPath != nil
		} else {
			s.searchQuery = ""
			s.selected = 0
		}
	default:
		if strings.Trim(input, "\r\n") != "" {
			s.searchQuery += input
			s.selected = 0
		}
	}
	s.mu.Unlock()

	if errorMessage != "" && onError != nil {
		onError(errorMessage)
	}
	if shouldEmitConfirm {
		s.emitDeleteConfirmation(confirmPath)
	}
}

func (s *SessionSelectorComponent) IsAllLoading() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.allLoading
}

func (s *SessionSelectorComponent) Scope() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scope
}

func (s *SessionSelectorComponent) SetSortMode(mode SessionSelectorSortMode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sortMode = mode
	s.clampSelectedLocked()
}

func (s *SessionSelectorComponent) SetNameFilter(filter SessionSelectorNameFilter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nameFilter = filter
	s.clampSelectedLocked()
}

func (s *SessionSelectorComponent) toggleScope() {
	var loader SessionSelectorLoader
	var seq int

	s.mu.Lock()
	if s.scope == sessionSelectorCurrentScope {
		s.scope = sessionSelectorAllScope
		if s.allLoaded {
			s.sessions = cloneSessionInfos(s.allSessions)
			s.clampSelectedLocked()
			s.mu.Unlock()
			return
		}
		if s.allLoading {
			s.mu.Unlock()
			return
		}
		loader = s.allLoader
		if loader == nil {
			s.allLoaded = true
			s.allSessions = nil
			s.sessions = nil
			s.selected = 0
			s.mu.Unlock()
			return
		}
		s.allLoading = true
		s.allLoadSeq++
		seq = s.allLoadSeq
		s.mu.Unlock()
		go s.loadAllSessions(seq, loader)
		return
	}

	s.scope = sessionSelectorCurrentScope
	s.sessions = cloneSessionInfos(s.currentSessions)
	s.clampSelectedLocked()
	s.mu.Unlock()
}

func (s *SessionSelectorComponent) loadAllSessions(seq int, loader SessionSelectorLoader) {
	sessions, err := loader(nil)
	var onError func(string)
	var errorMessage string

	s.mu.Lock()
	if seq != s.allLoadSeq {
		s.mu.Unlock()
		return
	}
	s.allLoading = false
	if err != nil {
		if s.scope == sessionSelectorAllScope {
			onError = s.options.OnError
			errorMessage = err.Error()
		}
		s.mu.Unlock()
		if errorMessage != "" && onError != nil {
			onError(errorMessage)
		}
		return
	}
	s.allLoaded = true
	s.allSessions = cloneSessionInfos(sessions)
	if s.scope == sessionSelectorAllScope {
		s.sessions = cloneSessionInfos(sessions)
		s.clampSelectedLocked()
	}
	s.mu.Unlock()
}

func (s *SessionSelectorComponent) handleRenameInputLocked(input string) {
	if input == sessionSelectorConfirm || input == sessionSelectorConfirmAlt {
		if s.options.RenameSession != nil && len(s.sessions) > 0 {
			_ = s.options.RenameSession(s.sessions[s.selected].Path, s.renameText)
		}
		s.renameMode = false
		return
	}
	if input == "\x7f" {
		if s.renameCursor > 0 {
			s.renameText = s.renameText[:s.renameCursor-1] + s.renameText[s.renameCursor:]
			s.renameCursor--
		}
		return
	}
	if strings.Trim(input, "\r\n") == "" {
		return
	}
	s.renameText = s.renameText[:s.renameCursor] + input + s.renameText[s.renameCursor:]
	s.renameCursor += len(input)
}

func (s *SessionSelectorComponent) startDeleteConfirmationLocked() (*string, string, func(string)) {
	nodes := s.visibleNodesLocked()
	if s.selected < 0 || s.selected >= len(nodes) {
		return nil, "", nil
	}
	path := nodes[s.selected].session.Path
	if s.isCurrentSessionPathLocked(path) {
		return nil, "Cannot delete the currently active session", s.options.OnError
	}
	s.deleteConfirm = true
	s.deletePath = path
	return stringPointer(path), "", nil
}

func (s *SessionSelectorComponent) emitDeleteConfirmation(path *string) {
	if s.options.OnDeleteConfirmationChange != nil {
		s.options.OnDeleteConfirmationChange(path)
	}
}

func (s *SessionSelectorComponent) isCurrentSessionPathLocked(path string) bool {
	if s.options.CurrentSessionPath == "" {
		return false
	}
	current := CanonicalizePath(s.options.CurrentSessionPath)
	candidate := CanonicalizePath(path)
	return candidate == current
}

func (s *SessionSelectorComponent) visibleNodesLocked() []sessionSelectorFlatNode {
	sessions := s.filteredSessionsLocked()
	if strings.TrimSpace(s.searchQuery) != "" || s.sortMode != SessionSelectorSortThreaded {
		nodes := make([]sessionSelectorFlatNode, 0, len(sessions))
		for _, session := range sessions {
			nodes = append(nodes, sessionSelectorFlatNode{session: session, isLast: true})
		}
		return nodes
	}
	return flattenSessionSelectorTree(buildSessionSelectorTree(sessions))
}

func (s *SessionSelectorComponent) filteredSessionsLocked() []SessionInfo {
	return FilterAndSortSessions(s.sessions, s.searchQuery, s.sortMode, s.nameFilter)
}

func (s *SessionSelectorComponent) clampSelectedLocked() {
	nodes := s.visibleNodesLocked()
	if len(nodes) == 0 {
		s.selected = 0
		return
	}
	if s.selected >= len(nodes) {
		s.selected = len(nodes) - 1
	}
	if s.selected < 0 {
		s.selected = 0
	}
}

type sessionSelectorTreeNode struct {
	session  SessionInfo
	children []*sessionSelectorTreeNode
}

type sessionSelectorFlatNode struct {
	session           SessionInfo
	depth             int
	isLast            bool
	ancestorContinues []bool
}

func (n sessionSelectorFlatNode) treePrefix() string {
	if n.depth == 0 {
		return ""
	}
	parts := make([]string, 0, len(n.ancestorContinues)+1)
	for _, continues := range n.ancestorContinues {
		if continues {
			parts = append(parts, "│  ")
		} else {
			parts = append(parts, "   ")
		}
	}
	if n.isLast {
		parts = append(parts, "└─ ")
	} else {
		parts = append(parts, "├─ ")
	}
	return strings.Join(parts, "")
}

func buildSessionSelectorTree(sessions []SessionInfo) []*sessionSelectorTreeNode {
	byPath := map[string]*sessionSelectorTreeNode{}
	for _, session := range sessions {
		byPath[CanonicalizePath(session.Path)] = &sessionSelectorTreeNode{session: session}
	}

	roots := []*sessionSelectorTreeNode{}
	for _, session := range sessions {
		node := byPath[CanonicalizePath(session.Path)]
		parentPath := ""
		if session.ParentSessionPath != "" {
			parentPath = CanonicalizePath(session.ParentSessionPath)
		}
		if parentPath != "" {
			if parent := byPath[parentPath]; parent != nil {
				parent.children = append(parent.children, node)
				continue
			}
		}
		roots = append(roots, node)
	}

	var sortNodes func([]*sessionSelectorTreeNode)
	sortNodes = func(nodes []*sessionSelectorTreeNode) {
		sort.SliceStable(nodes, func(i, j int) bool {
			return nodes[i].session.Modified.After(nodes[j].session.Modified)
		})
		for _, node := range nodes {
			sortNodes(node.children)
		}
	}
	sortNodes(roots)
	return roots
}

func flattenSessionSelectorTree(roots []*sessionSelectorTreeNode) []sessionSelectorFlatNode {
	nodes := []sessionSelectorFlatNode{}
	var walk func(node *sessionSelectorTreeNode, depth int, ancestors []bool, isLast bool)
	walk = func(node *sessionSelectorTreeNode, depth int, ancestors []bool, isLast bool) {
		nodes = append(nodes, sessionSelectorFlatNode{
			session:           node.session,
			depth:             depth,
			isLast:            isLast,
			ancestorContinues: append([]bool(nil), ancestors...),
		})
		for index, child := range node.children {
			childIsLast := index == len(node.children)-1
			continues := false
			if depth > 0 {
				continues = !isLast
			}
			walk(child, depth+1, append(append([]bool(nil), ancestors...), continues), childIsLast)
		}
	}
	for index, root := range roots {
		walk(root, 0, nil, index == len(roots)-1)
	}
	return nodes
}

func cloneSessionInfos(sessions []SessionInfo) []SessionInfo {
	if len(sessions) == 0 {
		return nil
	}
	return append([]SessionInfo(nil), sessions...)
}

func stringPointer(value string) *string {
	copyValue := value
	return &copyValue
}
