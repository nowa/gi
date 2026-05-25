package gicodingagent

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionSelectorCtrlBackspaceDoesNotDeleteWithSearchQuery(t *testing.T) {
	sessions := []SessionInfo{
		makeSessionSelectorSession("a", ""),
		makeSessionSelectorSession("b", ""),
	}
	confirmationChanges := []string{}
	selector := NewSessionSelectorComponent(sessions, SessionSelectorOptions{
		OnDeleteConfirmationChange: func(path *string) {
			confirmationChanges = append(confirmationChanges, derefSessionSelectorPath(path))
		},
	})

	selector.HandleInput("a")
	selector.HandleInput(sessionSelectorCtrlBackspace)

	if len(confirmationChanges) != 0 {
		t.Fatalf("confirmation changes = %#v, want none", confirmationChanges)
	}
}

func TestSessionSelectorCtrlDConfirmsDeleteWithSearchQuery(t *testing.T) {
	sessions := []SessionInfo{
		makeSessionSelectorSession("a", ""),
		makeSessionSelectorSession("b", ""),
	}
	confirmationChanges := []string{}
	selector := NewSessionSelectorComponent(sessions, SessionSelectorOptions{
		OnDeleteConfirmationChange: func(path *string) {
			confirmationChanges = append(confirmationChanges, derefSessionSelectorPath(path))
		},
	})

	selector.HandleInput("a")
	selector.HandleInput(sessionSelectorCtrlD)

	if got, want := confirmationChanges, []string{sessions[0].Path}; !equalStrings(got, want) {
		t.Fatalf("confirmation changes = %#v, want %#v", got, want)
	}
}

func TestSessionSelectorCtrlBackspaceConfirmsDeleteWhenSearchEmpty(t *testing.T) {
	sessions := []SessionInfo{
		makeSessionSelectorSession("a", ""),
		makeSessionSelectorSession("b", ""),
	}
	confirmationChanges := []string{}
	deletedPath := ""
	selector := NewSessionSelectorComponent(sessions, SessionSelectorOptions{
		OnDeleteConfirmationChange: func(path *string) {
			confirmationChanges = append(confirmationChanges, derefSessionSelectorPath(path))
		},
		DeleteSession: func(path string) error {
			deletedPath = path
			return nil
		},
	})

	selector.HandleInput(sessionSelectorCtrlBackspace)
	if got, want := confirmationChanges, []string{sessions[0].Path}; !equalStrings(got, want) {
		t.Fatalf("confirmation changes after ctrl+backspace = %#v, want %#v", got, want)
	}

	selector.HandleInput(sessionSelectorConfirm)
	if got, want := confirmationChanges, []string{sessions[0].Path, ""}; !equalStrings(got, want) {
		t.Fatalf("confirmation changes after confirm = %#v, want %#v", got, want)
	}
	if deletedPath != sessions[0].Path {
		t.Fatalf("deleted path = %q, want %q", deletedPath, sessions[0].Path)
	}
}

func TestSessionSelectorAllLoadResolveDoesNotSwitchBackToAll(t *testing.T) {
	currentSessions := []SessionInfo{makeSessionSelectorSession("current", "Current")}
	allSessions := []SessionInfo{makeSessionSelectorSession("all", "All")}
	started := make(chan struct{})
	release := make(chan struct{})
	var closeStarted sync.Once
	var allLoadCalls atomic.Int32

	selector := NewLoadingSessionSelectorComponent(
		func(SessionListProgress) ([]SessionInfo, error) { return currentSessions, nil },
		func(SessionListProgress) ([]SessionInfo, error) {
			allLoadCalls.Add(1)
			closeStarted.Do(func() { close(started) })
			<-release
			return allSessions, nil
		},
		SessionSelectorOptions{},
	)

	selector.HandleInput(sessionSelectorTab)
	waitForSessionSelectorSignal(t, started)
	selector.HandleInput(sessionSelectorTab)
	close(release)
	waitForSessionSelectorIdle(t, selector)

	if got := allLoadCalls.Load(); got != 1 {
		t.Fatalf("all load calls = %d, want 1", got)
	}
	output := strings.Join(selector.Render(120), "\n")
	if !strings.Contains(output, "Resume Session (Current Folder)") {
		t.Fatalf("output = %q, want current scope", output)
	}
	if strings.Contains(output, "Resume Session (All)") {
		t.Fatalf("output = %q, should not switch back to all", output)
	}
}

func TestSessionSelectorAllEmptyCancelsPiStyle(t *testing.T) {
	cancelled := make(chan struct{})
	var cancelOnce sync.Once
	selector := NewLoadingSessionSelectorComponent(
		func(SessionListProgress) ([]SessionInfo, error) { return nil, nil },
		func(SessionListProgress) ([]SessionInfo, error) { return nil, nil },
		SessionSelectorOptions{
			OnCancel: func() { cancelOnce.Do(func() { close(cancelled) }) },
		},
	)

	selector.HandleInput(sessionSelectorTab)
	waitForSessionSelectorSignal(t, cancelled)
}

func TestSessionSelectorDoesNotStartRedundantAllLoadsWhileLoading(t *testing.T) {
	currentSessions := []SessionInfo{makeSessionSelectorSession("current", "Current")}
	allSessions := []SessionInfo{makeSessionSelectorSession("all", "All")}
	started := make(chan struct{})
	release := make(chan struct{})
	var closeStarted sync.Once
	var allLoadCalls atomic.Int32

	selector := NewLoadingSessionSelectorComponent(
		func(SessionListProgress) ([]SessionInfo, error) { return currentSessions, nil },
		func(SessionListProgress) ([]SessionInfo, error) {
			allLoadCalls.Add(1)
			closeStarted.Do(func() { close(started) })
			<-release
			return allSessions, nil
		},
		SessionSelectorOptions{},
	)

	selector.HandleInput(sessionSelectorTab)
	waitForSessionSelectorSignal(t, started)
	selector.HandleInput(sessionSelectorTab)
	selector.HandleInput(sessionSelectorTab)

	if got := allLoadCalls.Load(); got != 1 {
		t.Fatalf("all load calls while pending = %d, want 1", got)
	}
	close(release)
	waitForSessionSelectorIdle(t, selector)
}

func TestSessionSelectorThreadsSymlinkAliasPaths(t *testing.T) {
	paths := createSymlinkedSessionSelectorPaths(t)
	sessions := []SessionInfo{
		makeSessionSelectorSessionWithPath("parent", paths.parentAliasB, "Parent", ""),
		makeSessionSelectorSessionWithPath("child", paths.childAliasB, "Child", paths.parentAliasA),
	}
	sessions[0].Modified = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions[1].Modified = time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)

	selector := NewSessionSelectorComponent(sessions, SessionSelectorOptions{})
	output := strings.Join(selector.Render(120), "\n")

	if !strings.Contains(output, "Parent") {
		t.Fatalf("output = %q, want parent", output)
	}
	if !strings.Contains(output, "└─ Child") {
		t.Fatalf("output = %q, want threaded child", output)
	}
}

func TestSessionSelectorCurrentSessionActiveAcrossSymlinkAliases(t *testing.T) {
	paths := createSymlinkedSessionSelectorPaths(t)
	sessions := []SessionInfo{
		makeSessionSelectorSessionWithPath("parent", paths.parentAliasB, "Parent", ""),
	}
	confirmationChanges := []string{}
	errorMessage := ""
	selector := NewSessionSelectorComponent(sessions, SessionSelectorOptions{
		CurrentSessionPath: paths.parentAliasA,
		OnDeleteConfirmationChange: func(path *string) {
			confirmationChanges = append(confirmationChanges, derefSessionSelectorPath(path))
		},
		OnError: func(message string) {
			errorMessage = message
		},
	})

	selector.HandleInput(sessionSelectorCtrlD)

	if len(confirmationChanges) != 0 {
		t.Fatalf("confirmation changes = %#v, want none", confirmationChanges)
	}
	if errorMessage != "Cannot delete the currently active session" {
		t.Fatalf("error message = %q", errorMessage)
	}
}

type sessionSelectorSymlinkPaths struct {
	parentAliasA string
	parentAliasB string
	childAliasB  string
}

func createSymlinkedSessionSelectorPaths(t *testing.T) sessionSelectorSymlinkPaths {
	t.Helper()
	baseDir := t.TempDir()
	realDir := filepath.Join(baseDir, "real")
	aliasADir := filepath.Join(baseDir, "alias-a")
	aliasBDir := filepath.Join(baseDir, "alias-b")
	for _, dir := range []string{realDir, aliasADir, aliasBDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	sharedDir := filepath.Join(realDir, "sessions")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatalf("mkdir shared sessions: %v", err)
	}
	aliasASessions := filepath.Join(aliasADir, "sessions")
	aliasBSessions := filepath.Join(aliasBDir, "sessions")
	if err := os.Symlink(sharedDir, aliasASessions); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.Symlink(sharedDir, aliasBSessions); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	parentRealPath := filepath.Join(sharedDir, "parent.jsonl")
	childRealPath := filepath.Join(sharedDir, "child.jsonl")
	if err := os.WriteFile(parentRealPath, []byte("parent\n"), 0o644); err != nil {
		t.Fatalf("write parent: %v", err)
	}
	if err := os.WriteFile(childRealPath, []byte("child\n"), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}
	return sessionSelectorSymlinkPaths{
		parentAliasA: filepath.Join(aliasASessions, "parent.jsonl"),
		parentAliasB: filepath.Join(aliasBSessions, "parent.jsonl"),
		childAliasB:  filepath.Join(aliasBSessions, "child.jsonl"),
	}
}

func makeSessionSelectorSessionWithPath(id, path, name, parentPath string) SessionInfo {
	session := makeSessionSelectorSession(id, name)
	session.Path = path
	session.ParentSessionPath = parentPath
	return session
}

func waitForSessionSelectorSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session selector signal")
	}
}

func waitForSessionSelectorIdle(t *testing.T, selector *SessionSelectorComponent) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !selector.IsAllLoading() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for session selector all loader")
}

func derefSessionSelectorPath(path *string) string {
	if path == nil {
		return ""
	}
	return *path
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}
