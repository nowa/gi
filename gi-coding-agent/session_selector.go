package gicodingagent

import "strings"

const sessionSelectorCtrlR = "\x1b[114;5u"

type SessionSelectorOptions struct {
	ShowRenameHint bool
	RenameSession  func(path, name string) error
}

type SessionSelectorComponent struct {
	sessions     []SessionInfo
	options      SessionSelectorOptions
	selected     int
	renameMode   bool
	renameText   string
	renameCursor int
}

func NewSessionSelectorComponent(sessions []SessionInfo, options SessionSelectorOptions) *SessionSelectorComponent {
	return &SessionSelectorComponent{sessions: append([]SessionInfo(nil), sessions...), options: options}
}

func (s *SessionSelectorComponent) Render(_ int) []string {
	title := "Resume Session"
	if s.renameMode {
		title = "Rename Session"
	}
	lines := []string{title}
	if s.options.ShowRenameHint && !s.renameMode {
		lines = append(lines, "ctrl+r rename")
	}
	for index, session := range s.sessions {
		prefix := "  "
		if index == s.selected {
			prefix = "> "
		}
		name := session.Name
		if name == "" {
			name = session.FirstMessage
		}
		lines = append(lines, prefix+name)
	}
	if s.renameMode {
		lines = append(lines, s.renameText)
	}
	return lines
}

func (s *SessionSelectorComponent) HandleInput(input string) {
	if input == sessionSelectorCtrlR && s.options.ShowRenameHint && len(s.sessions) > 0 {
		s.renameMode = true
		s.renameText = s.sessions[s.selected].Name
		s.renameCursor = 0
		return
	}
	if !s.renameMode {
		return
	}
	if input == "\r" || input == "\n" {
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
