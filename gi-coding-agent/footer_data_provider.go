package gicodingagent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultFooterWatchDebounce   = 500 * time.Millisecond
	defaultFooterWatchPoll       = 250 * time.Millisecond
	defaultFooterWatchRetryDelay = 5 * time.Second
)

type FooterGitBranchResolver func(repoDir string) (branch string, ok bool)

type FooterDataProviderOptions struct {
	GitBranchResolver FooterGitBranchResolver
	WatchDebounce     time.Duration
	WatchPollInterval time.Duration
	WatchRetryDelay   time.Duration
	DisableWatchers   bool
}

type FooterDataProvider struct {
	mu                     sync.Mutex
	cwd                    string
	gitPaths               *footerGitPaths
	cachedBranch           string
	cachedBranchSet        bool
	extensionStatuses      map[string]string
	availableProviderCount int
	branchCallbacks        map[int]func()
	nextCallbackID         int
	resolver               FooterGitBranchResolver
	watchDebounce          time.Duration
	watchPollInterval      time.Duration
	watchRetryDelay        time.Duration
	disableWatchers        bool
	refreshTimer           *time.Timer
	refreshInFlight        bool
	refreshPending         bool
	gitWatcherStop         chan struct{}
	gitWatcherActive       bool
	gitWatcherRetryTimer   *time.Timer
	disposed               bool
}

type footerGitPaths struct {
	repoDir      string
	commonGitDir string
	headPath     string
}

type footerPathSnapshot struct {
	exists  bool
	modTime time.Time
	size    int64
	isDir   bool
}

func NewFooterDataProvider(cwd string, options ...FooterDataProviderOptions) *FooterDataProvider {
	opts := FooterDataProviderOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	resolver := opts.GitBranchResolver
	if resolver == nil {
		resolver = resolveFooterBranchWithGit
	}
	watchDebounce := opts.WatchDebounce
	if watchDebounce <= 0 {
		watchDebounce = defaultFooterWatchDebounce
	}
	watchPollInterval := opts.WatchPollInterval
	if watchPollInterval <= 0 {
		watchPollInterval = defaultFooterWatchPoll
	}
	watchRetryDelay := opts.WatchRetryDelay
	if watchRetryDelay <= 0 {
		watchRetryDelay = defaultFooterWatchRetryDelay
	}

	provider := &FooterDataProvider{
		cwd:               cwd,
		gitPaths:          findFooterGitPaths(cwd),
		extensionStatuses: map[string]string{},
		branchCallbacks:   map[int]func(){},
		resolver:          resolver,
		watchDebounce:     watchDebounce,
		watchPollInterval: watchPollInterval,
		watchRetryDelay:   watchRetryDelay,
		disableWatchers:   opts.DisableWatchers,
	}
	provider.setupGitWatcher()
	return provider
}

func (p *FooterDataProvider) GetGitBranch() string {
	p.mu.Lock()
	if p.cachedBranchSet {
		branch := p.cachedBranch
		p.mu.Unlock()
		return branch
	}
	p.mu.Unlock()

	branch := p.resolveGitBranch()

	p.mu.Lock()
	if !p.cachedBranchSet {
		p.cachedBranch = branch
		p.cachedBranchSet = true
	}
	cached := p.cachedBranch
	p.mu.Unlock()
	return cached
}

func (p *FooterDataProvider) GetExtensionStatuses() map[string]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	statuses := make(map[string]string, len(p.extensionStatuses))
	for key, value := range p.extensionStatuses {
		statuses[key] = value
	}
	return statuses
}

func (p *FooterDataProvider) SetExtensionStatus(key string, text *string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if text == nil {
		delete(p.extensionStatuses, key)
		return
	}
	p.extensionStatuses[key] = *text
}

func (p *FooterDataProvider) ClearExtensionStatuses() {
	p.mu.Lock()
	defer p.mu.Unlock()
	clear(p.extensionStatuses)
}

func (p *FooterDataProvider) GetAvailableProviderCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.availableProviderCount
}

func (p *FooterDataProvider) SetAvailableProviderCount(count int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.availableProviderCount = max(0, count)
}

func (p *FooterDataProvider) OnBranchChange(callback func()) func() {
	p.mu.Lock()
	defer p.mu.Unlock()
	id := p.nextCallbackID
	p.nextCallbackID++
	p.branchCallbacks[id] = callback
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		delete(p.branchCallbacks, id)
	}
}

func (p *FooterDataProvider) SetCwd(cwd string) {
	p.mu.Lock()
	if p.cwd == cwd {
		p.mu.Unlock()
		return
	}
	p.cwd = cwd
	p.cachedBranch = ""
	p.cachedBranchSet = false
	p.gitPaths = findFooterGitPaths(cwd)
	p.stopRefreshTimerLocked()
	p.clearGitWatchersLocked()
	p.mu.Unlock()

	p.setupGitWatcher()
	p.notifyBranchChange()
}

func (p *FooterDataProvider) Dispose() {
	p.mu.Lock()
	p.disposed = true
	p.stopRefreshTimerLocked()
	p.clearGitWatchersLocked()
	p.branchCallbacks = map[int]func(){}
	p.mu.Unlock()
}

func (p *FooterDataProvider) resolveGitBranch() string {
	p.mu.Lock()
	gitPaths := p.gitPaths
	resolver := p.resolver
	p.mu.Unlock()
	if gitPaths == nil {
		return ""
	}
	content, err := os.ReadFile(gitPaths.headPath)
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(content))
	const branchPrefix = "ref: refs/heads/"
	if strings.HasPrefix(head, branchPrefix) {
		branch := strings.TrimPrefix(head, branchPrefix)
		if branch == ".invalid" {
			if resolved, ok := resolver(gitPaths.repoDir); ok && strings.TrimSpace(resolved) != "" {
				return strings.TrimSpace(resolved)
			}
			return "detached"
		}
		return branch
	}
	return "detached"
}

func (p *FooterDataProvider) setupGitWatcher() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clearGitWatchersLocked()
	if p.disposed || p.disableWatchers || p.gitPaths == nil {
		return
	}
	paths := p.watchPathsLocked()
	snapshots := make(map[string]footerPathSnapshot, len(paths))
	for _, path := range paths {
		snapshots[path] = snapshotFooterPath(path)
	}
	stop := make(chan struct{})
	p.gitWatcherStop = stop
	p.gitWatcherActive = true
	interval := p.watchPollInterval
	go p.pollGitWatchPaths(stop, interval, snapshots)
}

func (p *FooterDataProvider) pollGitWatchPaths(stop <-chan struct{}, interval time.Duration, snapshots map[string]footerPathSnapshot) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			changed := false
			for path, previous := range snapshots {
				current := snapshotFooterPath(path)
				if current != previous {
					snapshots[path] = current
					changed = true
				}
			}
			if changed {
				p.scheduleRefresh()
			}
		}
	}
}

func (p *FooterDataProvider) watchPathsLocked() []string {
	if p.gitPaths == nil {
		return nil
	}
	paths := []string{p.gitPaths.headPath}
	reftableDir := filepath.Join(p.gitPaths.commonGitDir, "reftable")
	if _, err := os.Stat(reftableDir); err == nil {
		paths = append(paths, reftableDir)
		tablesListPath := filepath.Join(reftableDir, "tables.list")
		if _, err := os.Stat(tablesListPath); err == nil {
			paths = append(paths, tablesListPath)
		}
	}
	return paths
}

func (p *FooterDataProvider) scheduleRefresh() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.disposed || p.refreshTimer != nil {
		return
	}
	if p.refreshInFlight {
		p.refreshPending = true
		return
	}
	p.refreshTimer = time.AfterFunc(p.watchDebounce, func() {
		p.mu.Lock()
		p.refreshTimer = nil
		p.mu.Unlock()
		p.refreshGitBranchAsync()
	})
}

func (p *FooterDataProvider) refreshGitBranchAsync() {
	p.mu.Lock()
	if p.disposed {
		p.mu.Unlock()
		return
	}
	if p.refreshInFlight {
		p.refreshPending = true
		p.mu.Unlock()
		return
	}
	p.refreshInFlight = true
	p.mu.Unlock()

	nextBranch := p.resolveGitBranch()

	var callbacks []func()
	shouldSchedulePending := false
	p.mu.Lock()
	if !p.disposed {
		if p.cachedBranchSet && p.cachedBranch != nextBranch {
			p.cachedBranch = nextBranch
			callbacks = p.branchCallbacksSnapshotLocked()
		} else {
			p.cachedBranch = nextBranch
			p.cachedBranchSet = true
		}
	}
	p.refreshInFlight = false
	if p.refreshPending && !p.disposed {
		p.refreshPending = false
		shouldSchedulePending = true
	}
	p.mu.Unlock()

	for _, callback := range callbacks {
		callback()
	}
	if shouldSchedulePending {
		p.scheduleRefresh()
	}
}

func (p *FooterDataProvider) handleGitWatcherError() {
	p.mu.Lock()
	p.clearGitWatchersLocked()
	if p.disposed || p.gitWatcherRetryTimer != nil {
		p.mu.Unlock()
		return
	}
	delay := p.watchRetryDelay
	p.gitWatcherRetryTimer = time.AfterFunc(delay, func() {
		p.mu.Lock()
		p.gitWatcherRetryTimer = nil
		p.mu.Unlock()
		p.setupGitWatcher()
	})
	p.mu.Unlock()
}

func (p *FooterDataProvider) hasGitWatcher() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.gitWatcherActive
}

func (p *FooterDataProvider) notifyBranchChange() {
	p.mu.Lock()
	callbacks := p.branchCallbacksSnapshotLocked()
	p.mu.Unlock()
	for _, callback := range callbacks {
		callback()
	}
}

func (p *FooterDataProvider) branchCallbacksSnapshotLocked() []func() {
	callbacks := make([]func(), 0, len(p.branchCallbacks))
	for _, callback := range p.branchCallbacks {
		callbacks = append(callbacks, callback)
	}
	return callbacks
}

func (p *FooterDataProvider) stopRefreshTimerLocked() {
	if p.refreshTimer != nil {
		p.refreshTimer.Stop()
		p.refreshTimer = nil
	}
}

func (p *FooterDataProvider) clearGitWatchersLocked() {
	if p.gitWatcherStop != nil {
		close(p.gitWatcherStop)
		p.gitWatcherStop = nil
	}
	p.gitWatcherActive = false
	if p.gitWatcherRetryTimer != nil {
		p.gitWatcherRetryTimer.Stop()
		p.gitWatcherRetryTimer = nil
	}
}

func snapshotFooterPath(path string) footerPathSnapshot {
	stat, err := os.Stat(path)
	if err != nil {
		return footerPathSnapshot{}
	}
	return footerPathSnapshot{
		exists:  true,
		modTime: stat.ModTime(),
		size:    stat.Size(),
		isDir:   stat.IsDir(),
	}
}

func findFooterGitPaths(cwd string) *footerGitPaths {
	dir, err := filepath.Abs(cwd)
	if err != nil {
		dir = filepath.Clean(cwd)
	}
	for {
		gitPath := filepath.Join(dir, ".git")
		stat, err := os.Stat(gitPath)
		if err == nil {
			if stat.IsDir() {
				headPath := filepath.Join(gitPath, "HEAD")
				if _, err := os.Stat(headPath); err != nil {
					return nil
				}
				return &footerGitPaths{repoDir: dir, commonGitDir: gitPath, headPath: headPath}
			}
			content, err := os.ReadFile(gitPath)
			if err != nil {
				return nil
			}
			line := strings.TrimSpace(string(content))
			if strings.HasPrefix(line, "gitdir: ") {
				gitDir := resolveFooterPath(dir, strings.TrimSpace(strings.TrimPrefix(line, "gitdir: ")))
				headPath := filepath.Join(gitDir, "HEAD")
				if _, err := os.Stat(headPath); err != nil {
					return nil
				}
				commonGitDir := gitDir
				commonDirPath := filepath.Join(gitDir, "commondir")
				if commonDirBytes, err := os.ReadFile(commonDirPath); err == nil {
					commonGitDir = resolveFooterPath(gitDir, strings.TrimSpace(string(commonDirBytes)))
				}
				return &footerGitPaths{repoDir: dir, commonGitDir: commonGitDir, headPath: headPath}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

func resolveFooterPath(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(base, path))
}

func resolveFooterBranchWithGit(repoDir string) (string, bool) {
	cmd := exec.Command("git", "--no-optional-locks", "symbolic-ref", "--quiet", "--short", "HEAD")
	cmd.Dir = repoDir
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	branch := strings.TrimSpace(string(output))
	return branch, branch != ""
}
