package gicodingagent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	envCodingAgentSessionDir       = "GI_CODING_AGENT_SESSION_DIR"
	legacyEnvCodingAgentSessionDir = "PI_CODING_AGENT_SESSION_DIR"
)

type cliResolvedSessionType string

const (
	cliResolvedSessionPath     cliResolvedSessionType = "path"
	cliResolvedSessionLocal    cliResolvedSessionType = "local"
	cliResolvedSessionGlobal   cliResolvedSessionType = "global"
	cliResolvedSessionNotFound cliResolvedSessionType = "not_found"
)

type cliResolvedSession struct {
	Type cliResolvedSessionType
	Path string
	CWD  string
	Arg  string
}

type cliSessionManagerResult struct {
	SessionManager  *SessionManager
	StartupWarnings []string
}

func newCLIPrintModeSessionManager(args Args, cwd, agentDir string, settingsManager *SettingsManager) (cliSessionManagerResult, error) {
	if err := validateCLISessionFlags(args); err != nil {
		return cliSessionManagerResult{}, err
	}
	sessionDir := resolveCLISessionDir(args, cwd, agentDir, settingsManager)
	sessionOptions := NewSessionOptions{ID: args.SessionID}
	if args.NoSession {
		manager, err := InMemorySessionManagerWithOptions(cwd, sessionOptions)
		return cliSessionManagerResult{SessionManager: manager}, err
	}
	if args.Fork != "" {
		if args.SessionID != "" {
			if _, ok := findLocalSessionByExactID(args.SessionID, cwd, sessionDir); ok {
				return cliSessionManagerResult{}, fmt.Errorf(
					"Session already exists with id '%s'",
					args.SessionID,
				)
			}
		}
		resolved := resolveCLISessionPath(args.Fork, cwd, sessionDir)
		switch resolved.Type {
		case cliResolvedSessionPath, cliResolvedSessionLocal, cliResolvedSessionGlobal:
			manager, err := ForkSessionFromWithOptions(
				resolved.Path,
				cwd,
				sessionOptions,
				sessionDir,
			)
			return cliSessionManagerResult{SessionManager: manager}, err
		case cliResolvedSessionNotFound:
			return cliSessionManagerResult{}, fmt.Errorf("No session found matching %q", resolved.Arg)
		}
	}
	if args.Session != "" {
		resolved := resolveCLISessionPath(args.Session, cwd, sessionDir)
		switch resolved.Type {
		case cliResolvedSessionPath, cliResolvedSessionLocal:
			manager, err := OpenSessionManager(resolved.Path, sessionDir, args.SessionCwdOverride)
			return cliSessionManagerResult{SessionManager: manager}, err
		case cliResolvedSessionGlobal:
			return cliSessionManagerResult{}, fmt.Errorf("Session found in different project: %s. Use --fork %s to fork it into the current directory", resolved.CWD, args.Session)
		case cliResolvedSessionNotFound:
			return cliSessionManagerResult{}, fmt.Errorf("No session found matching %q", resolved.Arg)
		}
	}
	if args.Resume {
		return cliSessionManagerResult{}, errors.New("--resume is only supported by interactive mode; use --session or --continue in print/RPC mode")
	}
	if args.Continue {
		var manager *SessionManager
		var err error
		if args.SessionCwdOverride != "" {
			if recent := FindMostRecentSession(sessionDir); recent != "" {
				manager, err = OpenSessionManager(recent, sessionDir, args.SessionCwdOverride)
				return cliSessionManagerResult{SessionManager: manager}, err
			}
		}
		manager, err = ContinueRecentSession(cwd, sessionDir)
		return cliSessionManagerResult{SessionManager: manager}, err
	}
	if args.SessionID != "" {
		if existing, ok := findLocalSessionByExactID(args.SessionID, cwd, sessionDir); ok {
			manager, err := OpenSessionManager(
				existing.Path,
				sessionDir,
				args.SessionCwdOverride,
			)
			return cliSessionManagerResult{SessionManager: manager}, err
		}
	}
	manager, err := CreateSessionManagerWithOptions(cwd, sessionDir, sessionOptions)
	result := cliSessionManagerResult{SessionManager: manager}
	if err == nil && args.SessionID != "" {
		result.StartupWarnings = []string{
			fmt.Sprintf(
				"No project session found with id '%s'; creating a new session with that id.",
				args.SessionID,
			),
		}
	}
	return result, err
}

func validateCLISessionFlags(args Args) error {
	if err := validateCLIForkFlags(args); err != nil {
		return err
	}
	return validateCLISessionIDFlags(args)
}

func validateCLIForkFlags(args Args) error {
	if args.Fork == "" {
		return nil
	}
	var conflicts []string
	if args.Session != "" {
		conflicts = append(conflicts, "--session")
	}
	if args.Continue {
		conflicts = append(conflicts, "--continue")
	}
	if args.Resume {
		conflicts = append(conflicts, "--resume")
	}
	if args.NoSession {
		conflicts = append(conflicts, "--no-session")
	}
	if len(conflicts) == 0 {
		return nil
	}
	return errors.New("--fork cannot be combined with " + strings.Join(conflicts, ", "))
}

func validateCLISessionIDFlags(args Args) error {
	if !hasCLISessionID(args) {
		return nil
	}
	var conflicts []string
	if args.Session != "" {
		conflicts = append(conflicts, "--session")
	}
	if args.Continue {
		conflicts = append(conflicts, "--continue")
	}
	if args.Resume {
		conflicts = append(conflicts, "--resume")
	}
	if len(conflicts) > 0 {
		return errors.New("--session-id cannot be combined with " + strings.Join(conflicts, ", "))
	}
	return ValidateSessionID(args.SessionID)
}

func hasCLISessionID(args Args) bool {
	return args.SessionIDSet || args.SessionID != ""
}

func resolveCLISessionDir(args Args, cwd, agentDir string, settingsManager *SettingsManager) string {
	if args.SessionDir != "" {
		return ExpandPath(args.SessionDir)
	}
	if env := firstNonEmptyString(os.Getenv(envCodingAgentSessionDir), os.Getenv(legacyEnvCodingAgentSessionDir)); env != "" {
		return ExpandPath(env)
	}
	if settingsManager != nil {
		if sessionDir := settingsManager.GetSessionDir(); sessionDir != "" {
			return sessionDir
		}
	}
	return GetAgentDirSessionDir(cwd, agentDir)
}

func resolveCLISessionPath(sessionArg, cwd, sessionDir string) cliResolvedSession {
	if looksLikeSessionPath(sessionArg) {
		return cliResolvedSession{Type: cliResolvedSessionPath, Path: resolveCLIPath(cwd, sessionArg), Arg: sessionArg}
	}
	localSessions := ListSessions(cwd, sessionDir)
	if session, ok := findSessionInfoByExactID(localSessions, sessionArg); ok {
		return cliResolvedSession{Type: cliResolvedSessionLocal, Path: session.Path, CWD: session.CWD, Arg: sessionArg}
	}
	for _, session := range localSessions {
		if strings.HasPrefix(session.ID, sessionArg) {
			return cliResolvedSession{Type: cliResolvedSessionLocal, Path: session.Path, CWD: session.CWD, Arg: sessionArg}
		}
	}
	allSessions := ListAllSessions(filepath.Dir(sessionDir))
	if session, ok := findSessionInfoByExactID(allSessions, sessionArg); ok {
		return cliResolvedSession{Type: cliResolvedSessionGlobal, Path: session.Path, CWD: session.CWD, Arg: sessionArg}
	}
	for _, session := range allSessions {
		if strings.HasPrefix(session.ID, sessionArg) {
			return cliResolvedSession{Type: cliResolvedSessionGlobal, Path: session.Path, CWD: session.CWD, Arg: sessionArg}
		}
	}
	return cliResolvedSession{Type: cliResolvedSessionNotFound, Arg: sessionArg}
}

func findLocalSessionByExactID(sessionID, cwd, sessionDir string) (cliResolvedSession, bool) {
	session, ok := findSessionInfoByExactID(ListSessions(cwd, sessionDir), sessionID)
	if !ok {
		return cliResolvedSession{}, false
	}
	return cliResolvedSession{
		Type: cliResolvedSessionLocal,
		Path: session.Path,
		CWD:  session.CWD,
		Arg:  sessionID,
	}, true
}

func findSessionInfoByExactID(sessions []SessionInfo, sessionID string) (SessionInfo, bool) {
	for _, session := range sessions {
		if session.ID == sessionID {
			return session, true
		}
	}
	return SessionInfo{}, false
}

func looksLikeSessionPath(value string) bool {
	return strings.Contains(value, "/") || strings.Contains(value, `\`) || strings.HasSuffix(value, ".jsonl")
}

func resolveCLIPath(cwd, value string) string {
	path := ExpandPath(value)
	if filepath.IsAbs(path) || cwd == "" {
		return path
	}
	return filepath.Join(cwd, path)
}
