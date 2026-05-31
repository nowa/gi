package sessioncwd

import (
	"fmt"
	"os"
)

type SessionManager interface {
	GetCwd() string
	GetSessionFile() string
}

type MissingIssue struct {
	SessionFile string
	SessionCwd  string
	FallbackCwd string
}

type MissingError struct {
	Issue MissingIssue
}

func (e MissingError) Error() string {
	return fmt.Sprintf("session cwd does not exist: %s", e.Issue.SessionCwd)
}

func FormatPrompt(issue MissingIssue) string {
	return fmt.Sprintf("cwd from session file does not exist\n%s\n\ncontinue in current cwd\n%s", issue.SessionCwd, issue.FallbackCwd)
}

func GetIssue(sessionManager SessionManager, fallbackCwd string) *MissingIssue {
	if sessionManager == nil {
		return nil
	}
	sessionCwd := sessionManager.GetCwd()
	if sessionCwd == "" || sessionCwd == fallbackCwd {
		return nil
	}
	info, err := os.Stat(sessionCwd)
	if err == nil && info.IsDir() {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil
	}
	return &MissingIssue{
		SessionFile: sessionManager.GetSessionFile(),
		SessionCwd:  sessionCwd,
		FallbackCwd: fallbackCwd,
	}
}
