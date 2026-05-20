package gicodingagent

import (
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionNavigateTreeToUserMessage(t *testing.T) {
	session, manager := createTreeNavigationTestSession(t, nil)
	defer session.Dispose()
	mustPrompt(t, session, "First message")
	mustPrompt(t, session, "Second message")

	tree := manager.GetTree()
	if len(tree) != 1 || tree[0].Entry.Type != "message" {
		t.Fatalf("tree = %#v", tree)
	}
	result, err := session.NavigateTree(tree[0].Entry.ID, AgentSessionNavigateTreeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Cancelled || result.EditorText != "First message" {
		t.Fatalf("result = %#v", result)
	}
	if leaf := manager.GetLeafID(); leaf != nil {
		t.Fatalf("leaf = %v, want nil", *leaf)
	}
}

func TestAgentSessionNavigateTreeToNonUserMessage(t *testing.T) {
	session, manager := createTreeNavigationTestSession(t, nil)
	defer session.Dispose()
	mustPrompt(t, session, "Hello")
	assistant := entriesByRole(manager.GetEntries(), llm.RoleAssistant)[0]

	result, err := session.NavigateTree(assistant.ID, AgentSessionNavigateTreeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Cancelled || result.EditorText != "" {
		t.Fatalf("result = %#v", result)
	}
	if leaf := manager.GetLeafID(); leaf == nil || *leaf != assistant.ID {
		t.Fatalf("leaf = %v, want %s", leaf, assistant.ID)
	}
}

func TestAgentSessionNavigateTreeCreatesBranchSummaryAtRoot(t *testing.T) {
	session, manager := createTreeNavigationTestSession(t, nil)
	defer session.Dispose()
	mustPrompt(t, session, "What is 2+2?")
	mustPrompt(t, session, "What is 3+3?")
	root := manager.GetTree()[0].Entry

	result, err := session.NavigateTree(root.ID, AgentSessionNavigateTreeOptions{Summarize: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Cancelled || result.EditorText != "What is 2+2?" || result.SummaryEntry == nil || result.SummaryEntry.Type != "branch_summary" || result.SummaryEntry.Summary == "" {
		t.Fatalf("result = %#v", result)
	}
	if result.SummaryEntry.ParentID != nil {
		t.Fatalf("summary parent = %v, want nil", *result.SummaryEntry.ParentID)
	}
	if leaf := manager.GetLeafID(); leaf == nil || *leaf != result.SummaryEntry.ID {
		t.Fatalf("leaf = %v, want summary %s", leaf, result.SummaryEntry.ID)
	}
}

func TestAgentSessionNavigateTreeAttachesSummaryToNestedUserParent(t *testing.T) {
	session, manager := createTreeNavigationTestSession(t, nil)
	defer session.Dispose()
	mustPrompt(t, session, "Message one")
	mustPrompt(t, session, "Message two")
	mustPrompt(t, session, "Message three")
	users := entriesByRole(manager.GetEntries(), llm.RoleUser)
	u2 := users[1]
	a1ID := *u2.ParentID

	result, err := session.NavigateTree(u2.ID, AgentSessionNavigateTreeOptions{Summarize: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Cancelled || result.EditorText != "Message two" || result.SummaryEntry == nil || result.SummaryEntry.ParentID == nil || *result.SummaryEntry.ParentID != a1ID {
		t.Fatalf("result = %#v, want parent %s", result, a1ID)
	}
	children := manager.GetChildren(a1ID)
	if len(children) != 2 || !containsEntryType(children, "branch_summary") || !containsEntryType(children, "message") {
		t.Fatalf("children = %#v", children)
	}
}

func TestAgentSessionNavigateTreeAttachesSummaryToAssistant(t *testing.T) {
	session, manager := createTreeNavigationTestSession(t, nil)
	defer session.Dispose()
	mustPrompt(t, session, "Hello")
	mustPrompt(t, session, "Goodbye")
	a1 := entriesByRole(manager.GetEntries(), llm.RoleAssistant)[0]

	result, err := session.NavigateTree(a1.ID, AgentSessionNavigateTreeOptions{Summarize: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Cancelled || result.EditorText != "" || result.SummaryEntry == nil || result.SummaryEntry.ParentID == nil || *result.SummaryEntry.ParentID != a1.ID {
		t.Fatalf("result = %#v, want summary parent %s", result, a1.ID)
	}
	if leaf := manager.GetLeafID(); leaf == nil || *leaf != result.SummaryEntry.ID {
		t.Fatalf("leaf = %v, want summary %s", leaf, result.SummaryEntry.ID)
	}
}

func TestAgentSessionNavigateTreeAbortDuringSummarization(t *testing.T) {
	started := make(chan struct{})
	session, manager := createTreeNavigationTestSession(t, func(_ []FileEntry, _ string, abort <-chan struct{}) (string, error) {
		close(started)
		<-abort
		return "", errAgentSessionBranchSummaryAborted
	})
	defer session.Dispose()
	mustPrompt(t, session, "Tell me about something")
	mustPrompt(t, session, "Continue")
	entriesBefore := len(manager.GetEntries())
	leafBefore := manager.GetLeafID()
	root := manager.GetTree()[0].Entry

	resultCh := make(chan AgentSessionNavigateTreeResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := session.NavigateTree(root.ID, AgentSessionNavigateTreeOptions{Summarize: true})
		resultCh <- result
		errCh <- err
	}()
	<-started
	if !session.IsCompacting() {
		t.Fatal("session should report compaction while branch summary is running")
	}
	session.AbortBranchSummary()
	result := <-resultCh
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if !result.Cancelled || !result.Aborted || result.SummaryEntry != nil {
		t.Fatalf("result = %#v, want aborted cancellation", result)
	}
	if len(manager.GetEntries()) != entriesBefore {
		t.Fatalf("entries changed after abort")
	}
	if leaf := manager.GetLeafID(); (leaf == nil) != (leafBefore == nil) || (leaf != nil && *leaf != *leafBefore) {
		t.Fatalf("leaf = %v, want %v", leaf, leafBefore)
	}
}

func TestAgentSessionNavigateTreeWithoutSummarizeCreatesNoSummary(t *testing.T) {
	session, manager := createTreeNavigationTestSession(t, nil)
	defer session.Dispose()
	mustPrompt(t, session, "First")
	mustPrompt(t, session, "Second")
	entriesBefore := len(manager.GetEntries())

	if _, err := session.NavigateTree(manager.GetTree()[0].Entry.ID, AgentSessionNavigateTreeOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(manager.GetEntries()) != entriesBefore {
		t.Fatalf("entries = %d, want %d", len(manager.GetEntries()), entriesBefore)
	}
	if summaries := filterFileEntriesByType(manager.GetEntries(), "branch_summary"); len(summaries) != 0 {
		t.Fatalf("summaries = %#v, want none", summaries)
	}
}

func TestAgentSessionNavigateTreeSamePositionNoop(t *testing.T) {
	session, manager := createTreeNavigationTestSession(t, nil)
	defer session.Dispose()
	mustPrompt(t, session, "Hello")
	leafBefore := manager.GetLeafID()
	entriesBefore := len(manager.GetEntries())

	result, err := session.NavigateTree(*leafBefore, AgentSessionNavigateTreeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Cancelled || manager.GetLeafID() == nil || *manager.GetLeafID() != *leafBefore || len(manager.GetEntries()) != entriesBefore {
		t.Fatalf("result = %#v leaf=%v entries=%d", result, manager.GetLeafID(), len(manager.GetEntries()))
	}
}

func TestAgentSessionNavigateTreeCustomInstructions(t *testing.T) {
	session, manager := createTreeNavigationTestSession(t, nil)
	defer session.Dispose()
	mustPrompt(t, session, "What is TypeScript?")

	result, err := session.NavigateTree(manager.GetTree()[0].Entry.ID, AgentSessionNavigateTreeOptions{
		Summarize:          true,
		CustomInstructions: "After the summary, you MUST end with exactly: MONKEY MONKEY MONKEY.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SummaryEntry == nil || !strings.Contains(result.SummaryEntry.Summary, "MONKEY MONKEY MONKEY") {
		t.Fatalf("summary = %#v", result.SummaryEntry)
	}
}

func TestAgentSessionNavigateTreeBetweenBranches(t *testing.T) {
	session, manager := createTreeNavigationTestSession(t, nil)
	defer session.Dispose()
	mustPrompt(t, session, "Main branch start")
	mustPrompt(t, session, "Main branch continue")
	a1 := entriesByRole(manager.GetEntries(), llm.RoleAssistant)[0]

	if err := manager.Branch(a1.ID); err != nil {
		t.Fatal(err)
	}
	mustPrompt(t, session, "Branch path")
	u2 := entriesByRole(manager.GetEntries(), llm.RoleUser)[1]

	result, err := session.NavigateTree(u2.ID, AgentSessionNavigateTreeOptions{Summarize: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Cancelled || result.EditorText != "Main branch continue" || result.SummaryEntry == nil || result.SummaryEntry.Summary == "" {
		t.Fatalf("result = %#v", result)
	}
}

func createTreeNavigationTestSession(t *testing.T, summarizer AgentSessionBranchSummarizer) (*AgentSession, *SessionManager) {
	t.Helper()
	cwd := t.TempDir()
	manager, err := InMemorySessionManager(cwd)
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:              cwd,
		AgentDir:         t.TempDir(),
		Model:            llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
		SessionManager:   manager,
		BranchSummarizer: summarizer,
	})
	if err != nil {
		t.Fatal(err)
	}
	return session, manager
}

func entriesByRole(entries []FileEntry, role string) []FileEntry {
	var result []FileEntry
	for _, entry := range entries {
		if entry.Type == "message" && sessionMessageRole(entry.Message) == role {
			result = append(result, entry)
		}
	}
	return result
}

func containsEntryType(entries []FileEntry, entryType string) bool {
	for _, entry := range entries {
		if entry.Type == entryType {
			return true
		}
	}
	return false
}
