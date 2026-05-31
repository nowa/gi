package harness

import (
	"context"
	"errors"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestGenerateBranchSummaryErrorsCarryPiStyleCodes(t *testing.T) {
	model := llm.Model{ID: "summary", Name: "summary", Provider: "faux-branch-summary-abort", API: "faux-branch-summary-abort", MaxTokens: 128000, ContextWindow: 200000}
	llm.RegisterAPIProvider("faux-branch-summary-abort", llm.APIProviderFuncs{StreamSimpleFunc: func(llm.Model, llm.Context, llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
		message := llm.AssistantMessage([]llm.ContentPart{llm.Text("partial")}, llm.StopReasonAborted, model)
		message.ErrorMessage = "cancelled"
		return llm.ErrorAssistantStream(message), nil
	}})
	defer llm.UnregisterAPIProvider("faux-branch-summary-abort")

	entries := []Entry{messageEntry("u1", nil, llm.UserMessageText("summarize branch"))}
	_, err := GenerateBranchSummary(context.Background(), entries, model, BranchSummaryOptions{})
	var summaryErr *BranchSummaryError
	if !errors.As(err, &summaryErr) || summaryErr.Code != BranchSummaryErrorAborted {
		t.Fatalf("GenerateBranchSummary err = %#v, want BranchSummaryError aborted", err)
	}
}
