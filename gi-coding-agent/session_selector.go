package gicodingagent

import (
	"os"
	"sort"
	"strings"
	"sync"

	gitui "github.com/nowa/gi/gi-tui"
)

const (
	sessionSelectorCtrlR         = "\x1b[114;5u"
	sessionSelectorCtrlN         = "\x0e"
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
	Keybindings                KeybindingsConfig
	OnDeleteConfirmationChange func(path *string)
	OnError                    func(message string)
	OnSelect                   func(path string)
	OnCancel                   func()
	RequestRender              func()
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
	keybindings     KeybindingsConfig
	searchQuery     string
	showPath        bool
	deleteConfirm   bool
	deletePath      string
	renameMode      bool
	renameText      string
	renameCursor    int
}

type sessionSelectorRenameSubmit struct {
	path    string
	name    string
	rename  func(path, name string) error
	onError func(message string)
}

func NewSessionSelectorComponent(sessions []SessionInfo, options SessionSelectorOptions) *SessionSelectorComponent {
	cloned := cloneSessionInfos(sessions)
	keybindings := DefaultProtocolKeybindings()
	if options.Keybindings != nil {
		keybindings = cloneKeybindingsConfig(options.Keybindings)
	}
	return &SessionSelectorComponent{
		sessions:        cloned,
		currentSessions: cloneSessionInfos(cloned),
		options:         options,
		scope:           sessionSelectorCurrentScope,
		sortMode:        SessionSelectorSortThreaded,
		nameFilter:      SessionSelectorNameAll,
		keybindings:     keybindings,
	}
}

func NewLoadingSessionSelectorComponent(currentLoader, allLoader SessionSelectorLoader, options SessionSelectorOptions) *SessionSelectorComponent {
	keybindings := DefaultProtocolKeybindings()
	if options.Keybindings != nil {
		keybindings = cloneKeybindingsConfig(options.Keybindings)
	}
	selector := &SessionSelectorComponent{
		options:     options,
		scope:       sessionSelectorCurrentScope,
		allLoader:   allLoader,
		sortMode:    SessionSelectorSortThreaded,
		nameFilter:  SessionSelectorNameAll,
		keybindings: keybindings,
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

func (s *SessionSelectorComponent) Invalidate() {}

func (s *SessionSelectorComponent) Render(width int) []string {
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
	border := tuiThemeAccent(strings.Repeat("─", max(1, width)))
	lines := []string{"", border, "", s.headerLineLocked(title, width)}
	lines = append(lines, s.hintLinesLocked()...)
	if s.deleteConfirm {
		lines = append(lines, tuiThemeError(gitui.TruncateToWidth("Delete session? "+s.tuiKeyText("tui.select.confirm", "Enter")+" confirm · "+s.tuiKeyText("tui.select.cancel", "Esc")+" cancel", width, "")))
	}
	lines = append(lines, "")
	lines = append(lines, s.searchLineLocked())
	lines = append(lines, "")

	nodes := s.visibleNodesLocked()
	if len(nodes) == 0 {
		lines = append(lines, tuiThemeMuted(gitui.TruncateToWidth(s.emptyMessageLocked(), width, "")))
		lines = append(lines, "", border)
		return lines
	}
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
		if s.showPath && strings.TrimSpace(session.Path) != "" {
			name += " [" + shortenSessionSelectorPath(session.Path) + "]"
		}
		lines = append(lines, prefix+node.treePrefix()+name)
	}
	lines = append(lines, "", border)
	return lines
}

func (s *SessionSelectorComponent) headerLineLocked(title string, width int) string {
	left := tuiThemeBold(title)
	sortLabel := sessionSelectorSortLabel(s.sortMode)
	nameLabel := "All"
	if s.nameFilter == SessionSelectorNameNamed {
		nameLabel = "Named"
	}
	var scopeText string
	if s.allLoading && s.scope == sessionSelectorAllScope {
		scopeText = tuiThemeMuted("○ Current Folder | ") + tuiThemeAccent("Loading ...")
	} else if s.scope == sessionSelectorCurrentScope {
		scopeText = tuiThemeAccent("◉ Current Folder") + tuiThemeMuted(" | ○ All")
	} else {
		scopeText = tuiThemeMuted("○ Current Folder | ") + tuiThemeAccent("◉ All")
	}
	right := scopeText + "  " + tuiThemeMuted("Name: ") + tuiThemeAccent(nameLabel) + "  " + tuiThemeMuted("Sort: ") + tuiThemeAccent(sortLabel)
	right = gitui.TruncateToWidth(right, width, "")
	availableLeft := max(0, width-gitui.VisibleWidth(right)-1)
	left = gitui.TruncateToWidth(left, availableLeft, "")
	spacing := max(0, width-gitui.VisibleWidth(left)-gitui.VisibleWidth(right))
	return left + strings.Repeat(" ", spacing) + right
}

func sessionSelectorSortLabel(mode SessionSelectorSortMode) string {
	switch mode {
	case SessionSelectorSortThreaded:
		return "Threaded"
	case SessionSelectorSortRecent:
		return "Recent"
	case SessionSelectorSortRelevance:
		return "Fuzzy"
	default:
		return string(mode)
	}
}

func (s *SessionSelectorComponent) searchLineLocked() string {
	return "> " + s.searchQuery + "\x1b[7m \x1b[0m"
}

func (s *SessionSelectorComponent) emptyMessageLocked() string {
	if s.nameFilter == SessionSelectorNameNamed {
		toggleKey := s.appKeyText("app.session.toggleNamedFilter", "ctrl+n")
		if s.scope == sessionSelectorAllScope {
			return "  No named sessions found. Press " + toggleKey + " to show all."
		}
		return "  No named sessions in current folder. Press " + toggleKey + " to show all, or Tab to view all."
	}
	if s.scope == sessionSelectorAllScope {
		return "  No sessions found"
	}
	return "  No sessions in current folder. Press Tab to view all."
}

func (s *SessionSelectorComponent) hintLinesLocked() []string {
	pathState := "off"
	if s.showPath {
		pathState = "on"
	}
	separator := tuiThemeMuted(" · ")
	line1 := tuiThemeKeyHint(s.tuiKeyText("tui.input.tab", "tab"), "scope") + separator + tuiThemeMuted(`re:<pattern> regex · "phrase" exact`)
	parts := []string{
		tuiThemeKeyHint(s.appKeyText("app.session.toggleSort", "ctrl+s"), "sort"),
		tuiThemeKeyHint(s.appKeyText("app.session.toggleNamedFilter", "ctrl+n"), "named"),
		tuiThemeKeyHint(s.appKeyText("app.session.delete", "ctrl+d"), "delete"),
		tuiThemeKeyHint(s.appKeyText("app.session.togglePath", "ctrl+p"), "path ("+pathState+")"),
	}
	if s.options.ShowRenameHint {
		parts = append(parts, tuiThemeKeyHint(s.appKeyText("app.session.rename", "ctrl+r"), "rename"))
	}
	return []string{line1, strings.Join(parts, separator)}
}

func (s *SessionSelectorComponent) tuiKeyText(action, fallback string) string {
	keys := gitui.GetKeybindings().GetKeys(action)
	if text := formatHotkeyKeys(keys, false); text != "" {
		return text
	}
	return fallback
}

func (s *SessionSelectorComponent) appKeyText(action, fallback string) string {
	keybindings := s.keybindings
	if keybindings == nil {
		keybindings = DefaultProtocolKeybindings()
	}
	if text := formatHotkeyKeys(keybindingValueKeys(keybindings[action]), false); text != "" {
		return text
	}
	return fallback
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
		submit := s.handleRenameInputLocked(input)
		s.mu.Unlock()
		s.finishRenameSubmit(submit)
		return
	}

	if s.deleteConfirm {
		kb := gitui.GetKeybindings()
		if kb.Matches(input, "tui.select.confirm") || input == sessionSelectorConfirmAlt {
			deletePath = s.deletePath
			deleteSession = s.options.DeleteSession
			s.deleteConfirm = false
			s.deletePath = ""
			clearConfirm = true
		} else if kb.Matches(input, "tui.select.cancel") {
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

	kb := gitui.GetKeybindings()
	switch {
	case kb.Matches(input, "tui.select.up") || input == "k":
		s.moveSelectedLocked(-1)
	case kb.Matches(input, "tui.select.down") || input == "j":
		s.moveSelectedLocked(1)
	case kb.Matches(input, "tui.select.pageUp") || input == "\x1b[5~":
		s.moveSelectedLocked(-8)
	case kb.Matches(input, "tui.select.pageDown") || input == "\x1b[6~":
		s.moveSelectedLocked(8)
	case kb.Matches(input, "tui.select.confirm") || input == sessionSelectorConfirmAlt:
		path := s.selectedSessionPathLocked()
		s.mu.Unlock()
		if path != "" && s.options.OnSelect != nil {
			s.options.OnSelect(path)
		}
		return
	case kb.Matches(input, "tui.select.cancel"):
		onCancel := s.options.OnCancel
		s.mu.Unlock()
		if onCancel != nil {
			onCancel()
		}
		return
	case kb.Matches(input, "tui.input.tab") || input == sessionSelectorTab:
		s.mu.Unlock()
		s.toggleScope()
		return
	case matchesKeybindingAction(input, s.keybindings, "app.session.toggleSort"):
		s.toggleSortModeLocked()
		s.clampSelectedLocked()
	case matchesKeybindingAction(input, s.keybindings, "app.session.togglePath"):
		s.showPath = !s.showPath
	case matchesKeybindingAction(input, s.keybindings, "app.session.rename") || input == sessionSelectorCtrlR:
		if s.options.ShowRenameHint && len(s.sessions) > 0 {
			nodes := s.visibleNodesLocked()
			if s.selected >= 0 && s.selected < len(nodes) {
				s.renameMode = true
				s.renameText = nodes[s.selected].session.Name
				s.renameCursor = 0
			}
		}
	case matchesKeybindingAction(input, s.keybindings, "app.session.toggleNamedFilter") || input == sessionSelectorCtrlN:
		if s.nameFilter == SessionSelectorNameNamed {
			s.nameFilter = SessionSelectorNameAll
		} else {
			s.nameFilter = SessionSelectorNameNamed
		}
		s.selected = 0
	case matchesKeybindingAction(input, s.keybindings, "app.session.delete") || input == sessionSelectorCtrlD:
		confirmPath, errorMessage, onError = s.startDeleteConfirmationLocked()
		shouldEmitConfirm = confirmPath != nil
	case matchesKeybindingAction(input, s.keybindings, "app.session.deleteNoninvasive") || input == sessionSelectorCtrlBackspace:
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

func (s *SessionSelectorComponent) toggleSortModeLocked() {
	switch s.sortMode {
	case SessionSelectorSortThreaded:
		s.sortMode = SessionSelectorSortRecent
	case SessionSelectorSortRecent:
		s.sortMode = SessionSelectorSortRelevance
	default:
		s.sortMode = SessionSelectorSortThreaded
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
	requestRender := s.options.RequestRender
	s.mu.Unlock()
	if requestRender != nil {
		requestRender()
	}
}

func (s *SessionSelectorComponent) selectedSessionPathLocked() string {
	nodes := s.visibleNodesLocked()
	if s.selected < 0 || s.selected >= len(nodes) {
		return ""
	}
	return nodes[s.selected].session.Path
}

func (s *SessionSelectorComponent) moveSelectedLocked(delta int) {
	nodes := s.visibleNodesLocked()
	if len(nodes) == 0 {
		s.selected = 0
		return
	}
	s.selected = max(0, min(s.selected+delta, len(nodes)-1))
}

func (s *SessionSelectorComponent) handleRenameInputLocked(input string) *sessionSelectorRenameSubmit {
	kb := gitui.GetKeybindings()
	if kb.Matches(input, "tui.select.cancel") {
		s.renameMode = false
		return nil
	}
	if kb.Matches(input, "tui.select.confirm") || input == sessionSelectorConfirmAlt {
		submit := &sessionSelectorRenameSubmit{
			path:    s.selectedSessionPathLocked(),
			name:    strings.TrimSpace(s.renameText),
			rename:  s.options.RenameSession,
			onError: s.options.OnError,
		}
		s.renameMode = false
		return submit
	}
	if input == "\x7f" {
		if s.renameCursor > 0 {
			s.renameText = s.renameText[:s.renameCursor-1] + s.renameText[s.renameCursor:]
			s.renameCursor--
		}
		return nil
	}
	if strings.Trim(input, "\r\n") == "" {
		return nil
	}
	s.renameText = s.renameText[:s.renameCursor] + input + s.renameText[s.renameCursor:]
	s.renameCursor += len(input)
	return nil
}

func (s *SessionSelectorComponent) finishRenameSubmit(submit *sessionSelectorRenameSubmit) {
	if submit == nil || submit.rename == nil || submit.path == "" {
		return
	}
	if err := submit.rename(submit.path, submit.name); err != nil {
		if submit.onError != nil {
			submit.onError(err.Error())
		}
		return
	}
	s.mu.Lock()
	s.updateSessionNameLocked(submit.path, submit.name)
	s.mu.Unlock()
}

func (s *SessionSelectorComponent) updateSessionNameLocked(path, name string) {
	update := func(sessions []SessionInfo) {
		for index := range sessions {
			if sameSessionSelectorPath(sessions[index].Path, path) {
				sessions[index].Name = name
			}
		}
	}
	update(s.sessions)
	update(s.currentSessions)
	update(s.allSessions)
}

func sameSessionSelectorPath(a, b string) bool {
	return CanonicalizePath(a) == CanonicalizePath(b)
}

func shortenSessionSelectorPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+string(os.PathSeparator)) {
			return "~" + strings.TrimPrefix(path, home)
		}
	}
	return path
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
