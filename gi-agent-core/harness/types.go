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
	SourceInfo             any
	DisableModelInvocation bool
}

type PromptTemplate struct {
	Name        string
	Description string
	Content     string
}

type SessionMetadata struct {
	ID                string         `json:"id"`
	CreatedAt         string         `json:"createdAt"`
	CWD               string         `json:"cwd,omitempty"`
	Path              string         `json:"path,omitempty"`
	ParentSessionPath string         `json:"parentSessionPath,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

type Entry struct {
	Type             string        `json:"type"`
	ID               string        `json:"id,omitempty"`
	ParentID         *string       `json:"parentId,omitempty"`
	Timestamp        string        `json:"timestamp,omitempty"`
	Message          llm.Message   `json:"message,omitempty"`
	TargetID         *string       `json:"targetId,omitempty"`
	Label            *string       `json:"label,omitempty"`
	ThinkingLevel    string        `json:"thinkingLevel,omitempty"`
	Provider         string        `json:"provider,omitempty"`
	ModelID          string        `json:"modelId,omitempty"`
	ActiveToolNames  []string      `json:"activeToolNames,omitempty"`
	Summary          string        `json:"summary,omitempty"`
	FirstKeptEntryID string        `json:"firstKeptEntryId,omitempty"`
	TokensBefore     int           `json:"tokensBefore,omitempty"`
	RetainedTail     []llm.Message `json:"retainedTail,omitempty"`
	FromID           string        `json:"fromId,omitempty"`
	CustomType       string        `json:"customType,omitempty"`
	Data             any           `json:"data,omitempty"`
	Content          any           `json:"content,omitempty"`
	Display          bool          `json:"display,omitempty"`
	Details          any           `json:"details,omitempty"`
	FromHook         bool          `json:"fromHook,omitempty"`
	Usage            *llm.Usage    `json:"usage,omitempty"`
	Name             string        `json:"name,omitempty"`
}

type SessionStats struct {
	MessageCount   int     `json:"messageCount"`
	CachedTokens   int     `json:"cachedTokens"`
	UncachedTokens int     `json:"uncachedTokens"`
	TotalTokens    int     `json:"totalTokens"`
	CostTotal      float64 `json:"costTotal"`
}

type SessionEntryCursorOptions struct {
	AfterEntrySeq int
	Limit         int
}

type SessionCreateOptions struct {
	ParentSessionPath string
	Metadata          map[string]any
}

type SessionForkOptions struct {
	ParentSessionPath string
	Metadata          map[string]any
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

const (
	CompactionErrorAborted             = "aborted"
	CompactionErrorSummarizationFailed = "summarization_failed"
	CompactionErrorInvalidSession      = "invalid_session"
	CompactionErrorUnknown             = "unknown"

	BranchSummaryErrorAborted             = "aborted"
	BranchSummaryErrorSummarizationFailed = "summarization_failed"
	BranchSummaryErrorInvalidSession      = "invalid_session"
	BranchSummaryErrorUnknown             = "unknown"
)

type CompactionError struct {
	Code string
	Err  error
}

func (e *CompactionError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *CompactionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newCompactionError(code, format string, args ...any) *CompactionError {
	return &CompactionError{Code: code, Err: fmt.Errorf(format, args...)}
}

type BranchSummaryError struct {
	Code string
	Err  error
}

func (e *BranchSummaryError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *BranchSummaryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newBranchSummaryError(code, format string, args ...any) *BranchSummaryError {
	return &BranchSummaryError{Code: code, Err: fmt.Errorf(format, args...)}
}
