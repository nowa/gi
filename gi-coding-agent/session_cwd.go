package gicodingagent

import sessioncwd "github.com/nowa/gi/gi-coding-agent/internal/sessioncwd"

type MissingSessionCwdIssue = sessioncwd.MissingIssue
type MissingSessionCwdError = sessioncwd.MissingError

func formatMissingSessionCwdPrompt(issue MissingSessionCwdIssue) string {
	return sessioncwd.FormatPrompt(issue)
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
	return sessioncwd.GetIssue(sessionManager, fallbackCwd)
}

func CreateAgentSessionRuntime(factory CreateAgentSessionRuntimeFactory, options AgentSessionRuntimeOptions) (any, error) {
	if issue := GetMissingSessionCwdIssue(options.SessionManager, options.CWD); issue != nil {
		return nil, MissingSessionCwdError{Issue: *issue}
	}
	return factory(options)
}
