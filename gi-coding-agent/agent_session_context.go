package gicodingagent

import (
	"sync"

	llm "github.com/nowa/gi/gi-llm-provider"
)

// agentSessionProviderContextState owns the difference between durable session
// history and the live context sent to providers. Retry and overflow errors
// remain auditable in the session file while being excluded from subsequent
// model calls for the lifetime of this AgentSession.
type agentSessionProviderContextState struct {
	mu               sync.RWMutex
	excludedEntryIDs map[string]struct{}
}

func (s *agentSessionProviderContextState) exclude(entryID string) {
	if s == nil || entryID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.excludedEntryIDs == nil {
		s.excludedEntryIDs = make(map[string]struct{})
	}
	s.excludedEntryIDs[entryID] = struct{}{}
}

func (s *agentSessionProviderContextState) options() SessionContextOptions {
	if s == nil {
		return SessionContextOptions{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.excludedEntryIDs) == 0 {
		return SessionContextOptions{}
	}
	excluded := make(map[string]struct{}, len(s.excludedEntryIDs))
	for entryID := range s.excludedEntryIDs {
		excluded[entryID] = struct{}{}
	}
	return SessionContextOptions{ExcludeEntryIDs: excluded}
}

func (s *AgentSession) baseProviderContextMessages() []llm.Message {
	if s == nil || s.SessionManager == nil {
		return nil
	}
	sessionContext := s.SessionManager.BuildSessionContextWithOptions(
		s.providerContext.options(),
	)
	return providerContextFromSessionValues(sessionContext.Messages)
}
