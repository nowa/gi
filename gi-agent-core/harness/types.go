package harness

import (
	"fmt"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type Skill struct {
	Name                   string
	Description            string
	Content                string
	FilePath               string
	DisableModelInvocation bool
}

type PromptTemplate struct {
	Name        string
	Description string
	Content     string
}

type SessionMetadata struct {
	ID                string `json:"id"`
	CreatedAt         string `json:"createdAt"`
	CWD               string `json:"cwd,omitempty"`
	Path              string `json:"path,omitempty"`
	ParentSessionPath string `json:"parentSessionPath,omitempty"`
}

type Entry struct {
	Type             string      `json:"type"`
	ID               string      `json:"id,omitempty"`
	ParentID         *string     `json:"parentId,omitempty"`
	Timestamp        string      `json:"timestamp,omitempty"`
	Message          llm.Message `json:"message,omitempty"`
	TargetID         *string     `json:"targetId,omitempty"`
	Label            *string     `json:"label,omitempty"`
	ThinkingLevel    string      `json:"thinkingLevel,omitempty"`
	Provider         string      `json:"provider,omitempty"`
	ModelID          string      `json:"modelId,omitempty"`
	Summary          string      `json:"summary,omitempty"`
	FirstKeptEntryID string      `json:"firstKeptEntryId,omitempty"`
	TokensBefore     int         `json:"tokensBefore,omitempty"`
	FromID           string      `json:"fromId,omitempty"`
	CustomType       string      `json:"customType,omitempty"`
	Content          any         `json:"content,omitempty"`
	Display          bool        `json:"display,omitempty"`
	Details          any         `json:"details,omitempty"`
	FromHook         bool        `json:"fromHook,omitempty"`
	Name             string      `json:"name,omitempty"`
}

type SessionError struct {
	Code string
	Err  error
}

func (e *SessionError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *SessionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newSessionError(code, format string, args ...any) *SessionError {
	return &SessionError{Code: code, Err: fmt.Errorf(format, args...)}
}
