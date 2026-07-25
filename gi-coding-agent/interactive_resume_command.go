package gicodingagent

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

type sessionResumeState struct {
	persisted bool
	id        string
	file      string
	dir       string
	cwd       string
}

func (s *SessionManager) resumeState() sessionResumeState {
	if s == nil {
		return sessionResumeState{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sessionResumeState{
		persisted: s.persist,
		id:        s.sessionID,
		file:      s.sessionFile,
		dir:       s.sessionDir,
		cwd:       s.cwd,
	}
}

func quoteIfNeeded(value string) string {
	if value != "" && isSafeCLIArgument(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func isSafeCLIArgument(value string) bool {
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9':
		case strings.ContainsRune("_-./~:@", char):
		default:
			return false
		}
	}
	return true
}

// formatResumeCommand builds a copyable Gi command from one locked session
// identity snapshot. TTY policy belongs to the output host, not session state.
func formatResumeCommand(
	sessionManager *SessionManager,
) (string, bool) {
	state := sessionManager.resumeState()
	if !state.persisted ||
		state.id == "" ||
		state.file == "" {
		return "", false
	}
	if _, err := os.Stat(state.file); err != nil {
		return "", false
	}

	args := []string{DefaultCodingAgentAppName}
	if !sessionDirMatchesDefault(state.dir, state.cwd) {
		args = append(
			args,
			"--session-dir",
			quoteIfNeeded(state.dir),
		)
	}
	args = append(args, "--session", state.id)
	return strings.Join(args, " "), true
}

func resolveInteractiveOutput(
	writer io.Writer,
	isTTYOverride *bool,
) (io.Writer, bool) {
	if writer == nil {
		writer = os.Stdout
	}
	if isTTYOverride != nil {
		return writer, *isTTYOverride
	}
	file, ok := writer.(*os.File)
	return writer, ok && term.IsTerminal(int(file.Fd()))
}

func (h *CLIInteractiveTUIHost) resumeCommandForShutdown() (
	string,
	bool,
) {
	if h == nil ||
		!h.stdoutIsTTY ||
		h.exitAfterInitial ||
		h.resumeCommandSuppressed.Load() {
		return "", false
	}
	select {
	case <-h.done:
	default:
		return "", false
	}
	session := h.agentSession()
	if session == nil {
		return "", false
	}
	return formatResumeCommand(session.SessionManager)
}

func (h *CLIInteractiveTUIHost) writeResumeCommand(
	command string,
) error {
	if h == nil || h.stdout == nil || command == "" {
		return nil
	}
	_, err := fmt.Fprintf(
		h.stdout,
		"%s %s\n",
		tuiThemeDim("To resume this session:"),
		command,
	)
	return err
}
