package gicodingagent

import (
	"fmt"
	"os"
)

type MissingSessionCwdIssue struct {
	SessionFile string
	SessionCwd  string
	FallbackCwd string
}

type MissingSessionCwdError struct {
	Issue MissingSessionCwdIssue
}

func (e MissingSessionCwdError) Error() string {
	return fmt.Sprintf("session cwd does not exist: %s", e.Issue.SessionCwd)
}

func formatMissingSessionCwdPrompt(issue MissingSessionCwdIssue) string {
	return fmt.Sprintf("cwd from session file does not exist\n%s\n\ncontinue in current cwd\n%s", issue.SessionCwd, issue.FallbackCwd)
}

type AgentSessionRuntimeOptions struct {
	CWD            string
	AgentDir       string
	SessionManager *SessionManager
}

type CreateAgentSessionRuntimeFactory func(AgentSessionRuntimeOptions) (any, error)

func GetMissingSessionCwdIssue(sessionManager *SessionManager, fallbackCwd string) *MissingSessionCwdIssue {
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
	return &MissingSessionCwdIssue{
		SessionFile: sessionManager.GetSessionFile(),
		SessionCwd:  sessionCwd,
		FallbackCwd: fallbackCwd,
	}
}

func CreateAgentSessionRuntime(factory CreateAgentSessionRuntimeFactory, options AgentSessionRuntimeOptions) (any, error) {
	if issue := GetMissingSessionCwdIssue(options.SessionManager, options.CWD); issue != nil {
		return nil, MissingSessionCwdError{Issue: *issue}
	}
	return factory(options)
}
