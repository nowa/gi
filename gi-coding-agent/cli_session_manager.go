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

func newCLIPrintModeSessionManager(args Args, cwd, agentDir string, settingsManager *SettingsManager) (*SessionManager, error) {
	if err := validateCLISessionFlags(args); err != nil {
		return nil, err
	}
	sessionDir := resolveCLISessionDir(args, cwd, agentDir, settingsManager)
	if args.NoSession {
		return InMemorySessionManager(cwd)
	}
	if args.Fork != "" {
		resolved := resolveCLISessionPath(args.Fork, cwd, sessionDir)
		switch resolved.Type {
		case cliResolvedSessionPath, cliResolvedSessionLocal, cliResolvedSessionGlobal:
			return ForkSessionFrom(resolved.Path, cwd, sessionDir)
		case cliResolvedSessionNotFound:
			return nil, fmt.Errorf("No session found matching %q", resolved.Arg)
		}
	}
	if args.Session != "" {
		resolved := resolveCLISessionPath(args.Session, cwd, sessionDir)
		switch resolved.Type {
		case cliResolvedSessionPath, cliResolvedSessionLocal:
			return OpenSessionManager(resolved.Path, sessionDir, args.SessionCwdOverride)
		case cliResolvedSessionGlobal:
			return nil, fmt.Errorf("Session found in different project: %s. Use --fork %s to fork it into the current directory", resolved.CWD, args.Session)
		case cliResolvedSessionNotFound:
			return nil, fmt.Errorf("No session found matching %q", resolved.Arg)
		}
	}
	if args.Resume {
		return nil, errors.New("--resume is only supported by interactive mode; use --session or --continue in print/RPC mode")
	}
	if args.Continue {
		if args.SessionCwdOverride != "" {
			if recent := FindMostRecentSession(sessionDir); recent != "" {
				return OpenSessionManager(recent, sessionDir, args.SessionCwdOverride)
			}
		}
		return ContinueRecentSession(cwd, sessionDir)
	}
	return CreateSessionManager(cwd, sessionDir)
}

func validateCLISessionFlags(args Args) error {
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
	for _, session := range ListSessions(cwd, sessionDir) {
		if strings.HasPrefix(session.ID, sessionArg) {
			return cliResolvedSession{Type: cliResolvedSessionLocal, Path: session.Path, CWD: session.CWD, Arg: sessionArg}
		}
	}
	for _, session := range ListAllSessions(filepath.Dir(sessionDir)) {
		if strings.HasPrefix(session.ID, sessionArg) {
			return cliResolvedSession{Type: cliResolvedSessionGlobal, Path: session.Path, CWD: session.CWD, Arg: sessionArg}
		}
	}
	return cliResolvedSession{Type: cliResolvedSessionNotFound, Arg: sessionArg}
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
