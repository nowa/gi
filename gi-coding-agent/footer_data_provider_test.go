package gicodingagent

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestFooterDataProviderUsesHEADDirectlyInRegularRepoFromNestedDirectory(t *testing.T) {
	repoDir := createPlainFooterRepo(t)
	nestedDir := filepath.Join(repoDir, "src", "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var resolverCalls int32
	provider := NewFooterDataProvider(nestedDir, FooterDataProviderOptions{
		GitBranchResolver: func(string) (string, bool) {
			atomic.AddInt32(&resolverCalls, 1)
			return "unexpected", true
		},
		DisableWatchers: true,
	})
	defer provider.Dispose()

	if got := provider.GetGitBranch(); got != "main" {
		t.Fatalf("branch = %q, want main", got)
	}
	if got := atomic.LoadInt32(&resolverCalls); got != 0 {
		t.Fatalf("resolver calls = %d, want 0", got)
	}
}

func TestFooterDataProviderResolvesInvalidReftableHeadWithGit(t *testing.T) {
	repoDir := createPlainFooterReftableRepo(t)
	var resolvedRepo string
	provider := NewFooterDataProvider(repoDir, FooterDataProviderOptions{
		GitBranchResolver: func(repoDir string) (string, bool) {
			resolvedRepo = repoDir
			return "main", true
		},
		DisableWatchers: true,
	})
	defer provider.Dispose()

	if got := provider.GetGitBranch(); got != "main" {
		t.Fatalf("branch = %q, want main", got)
	}
	if resolvedRepo != repoDir {
		t.Fatalf("resolver repo = %q, want %q", resolvedRepo, repoDir)
	}
}

func TestFooterDataProviderResolvesInvalidReftableWorktreeHeadWithGit(t *testing.T) {
	worktreeDir, _ := createFooterReftableWorktree(t)
	provider := NewFooterDataProvider(worktreeDir, FooterDataProviderOptions{
		GitBranchResolver: func(repoDir string) (string, bool) {
			if repoDir != worktreeDir {
				t.Fatalf("resolver repo = %q, want %q", repoDir, worktreeDir)
			}
			return "main", true
		},
		DisableWatchers: true,
	})
	defer provider.Dispose()

	if got := provider.GetGitBranch(); got != "main" {
		t.Fatalf("branch = %q, want main", got)
	}
}

func TestFooterDataProviderTreatsUnresolvedInvalidReftableHeadAsDetached(t *testing.T) {
	repoDir := createPlainFooterReftableRepo(t)
	provider := NewFooterDataProvider(repoDir, FooterDataProviderOptions{
		GitBranchResolver: func(string) (string, bool) {
			return "", false
		},
		DisableWatchers: true,
	})
	defer provider.Dispose()

	if got := provider.GetGitBranch(); got != "detached" {
		t.Fatalf("branch = %q, want detached", got)
	}
}

func TestFooterDataProviderDoesNotNotifyWhenReftableUpdateKeepsSameBranch(t *testing.T) {
	worktreeDir, reftableDir := createFooterReftableWorktree(t)
	var resolverCalls int32
	provider := NewFooterDataProvider(worktreeDir, footerProviderWatchOptions(func(string) (string, bool) {
		atomic.AddInt32(&resolverCalls, 1)
		return "main", true
	}))
	defer provider.Dispose()
	if got := provider.GetGitBranch(); got != "main" {
		t.Fatalf("branch = %q, want main", got)
	}
	atomic.StoreInt32(&resolverCalls, 0)
	var callbackCalls int32
	provider.OnBranchChange(func() { atomic.AddInt32(&callbackCalls, 1) })

	writeFooterFile(t, filepath.Join(reftableDir, "tables.list"), "1\n")
	waitForFooterCondition(t, func() bool { return atomic.LoadInt32(&resolverCalls) == 1 })
	time.Sleep(50 * time.Millisecond)

	if got := provider.GetGitBranch(); got != "main" {
		t.Fatalf("branch = %q, want main", got)
	}
	if got := atomic.LoadInt32(&callbackCalls); got != 0 {
		t.Fatalf("callback calls = %d, want 0", got)
	}
}

func TestFooterDataProviderDebouncesRapidReftableUpdatesIntoSingleRefresh(t *testing.T) {
	worktreeDir, reftableDir := createFooterReftableWorktree(t)
	var resolverCalls int32
	provider := NewFooterDataProvider(worktreeDir, footerProviderWatchOptions(func(string) (string, bool) {
		atomic.AddInt32(&resolverCalls, 1)
		return "main", true
	}))
	defer provider.Dispose()
	if got := provider.GetGitBranch(); got != "main" {
		t.Fatalf("branch = %q, want main", got)
	}
	atomic.StoreInt32(&resolverCalls, 0)

	writeFooterFile(t, filepath.Join(reftableDir, "tables.list"), "1\n")
	writeFooterFile(t, filepath.Join(reftableDir, "tables.list"), "22\n")
	writeFooterFile(t, filepath.Join(reftableDir, "tables.list"), "333\n")
	waitForFooterCondition(t, func() bool { return atomic.LoadInt32(&resolverCalls) == 1 })
	time.Sleep(80 * time.Millisecond)

	if got := atomic.LoadInt32(&resolverCalls); got != 1 {
		t.Fatalf("resolver calls = %d, want 1", got)
	}
}

func TestFooterDataProviderUpdatesCachedBranchWhenReftableDirectoryChanges(t *testing.T) {
	worktreeDir, reftableDir := createFooterReftableWorktree(t)
	var branch atomic.Value
	branch.Store("main")
	var resolverCalls int32
	provider := NewFooterDataProvider(worktreeDir, footerProviderWatchOptions(func(string) (string, bool) {
		atomic.AddInt32(&resolverCalls, 1)
		return branch.Load().(string), true
	}))
	defer provider.Dispose()
	if got := provider.GetGitBranch(); got != "main" {
		t.Fatalf("branch = %q, want main", got)
	}
	atomic.StoreInt32(&resolverCalls, 0)
	var callbackCalls int32
	provider.OnBranchChange(func() { atomic.AddInt32(&callbackCalls, 1) })

	branch.Store("foo")
	writeFooterFile(t, filepath.Join(reftableDir, "tables.list"), "1\n")
	waitForFooterCondition(t, func() bool { return provider.GetGitBranch() == "foo" })

	if got := atomic.LoadInt32(&resolverCalls); got != 1 {
		t.Fatalf("resolver calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&callbackCalls); got != 1 {
		t.Fatalf("callback calls = %d, want 1", got)
	}
}

func TestFooterDataProviderRetriesGitWatchersAfterAsyncError(t *testing.T) {
	repoDir := createPlainFooterRepo(t)
	provider := NewFooterDataProvider(repoDir, FooterDataProviderOptions{
		WatchRetryDelay: 30 * time.Millisecond,
	})
	defer provider.Dispose()
	if !provider.hasGitWatcher() {
		t.Fatal("git watcher should be active")
	}

	provider.handleGitWatcherError()
	if provider.hasGitWatcher() {
		t.Fatal("git watcher should be stopped after error")
	}
	time.Sleep(20 * time.Millisecond)
	if provider.hasGitWatcher() {
		t.Fatal("git watcher retried before retry delay")
	}
	waitForFooterCondition(t, provider.hasGitWatcher)
}

func footerProviderWatchOptions(resolver FooterGitBranchResolver) FooterDataProviderOptions {
	return FooterDataProviderOptions{
		GitBranchResolver: resolver,
		WatchDebounce:     20 * time.Millisecond,
		WatchPollInterval: 5 * time.Millisecond,
		WatchRetryDelay:   30 * time.Millisecond,
	}
}

func createPlainFooterRepo(t *testing.T) string {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFooterFile(t, filepath.Join(repoDir, ".git", "HEAD"), "ref: refs/heads/main\n")
	return repoDir
}

func createPlainFooterReftableRepo(t *testing.T) string {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git", "reftable"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFooterFile(t, filepath.Join(repoDir, ".git", "HEAD"), "ref: refs/heads/.invalid\n")
	return repoDir
}

func createFooterReftableWorktree(t *testing.T) (worktreeDir string, reftableDir string) {
	t.Helper()
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "repo")
	commonGitDir := filepath.Join(repoDir, ".git")
	gitDir := filepath.Join(commonGitDir, "worktrees", "src")
	worktreeDir = filepath.Join(tempDir, "worktree")
	reftableDir = filepath.Join(commonGitDir, "reftable")
	for _, dir := range []string{gitDir, reftableDir, worktreeDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFooterFile(t, filepath.Join(worktreeDir, ".git"), "gitdir: "+gitDir+"\n")
	writeFooterFile(t, filepath.Join(gitDir, "HEAD"), "ref: refs/heads/.invalid\n")
	writeFooterFile(t, filepath.Join(gitDir, "commondir"), "../..\n")
	writeFooterFile(t, filepath.Join(reftableDir, "tables.list"), "0\n")
	return worktreeDir, reftableDir
}

func writeFooterFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func waitForFooterCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for condition")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
