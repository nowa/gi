package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProjectTrustStoreInheritsAndOverridesDecisions(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, "agent")
	parent := filepath.Join(root, "trusted-parent")
	child := filepath.Join(parent, "project")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewProjectTrustStore(agentDir)

	if _, found, err := store.Get(child); err != nil || found {
		t.Fatalf("initial decision = found %t, err %v", found, err)
	}
	if err := store.Set(parent, true); err != nil {
		t.Fatal(err)
	}
	if decision, found, err := store.Get(child); err != nil || !found || !decision {
		t.Fatalf("inherited decision = %t, found %t, err %v", decision, found, err)
	}
	if err := store.Set(child, false); err != nil {
		t.Fatal(err)
	}
	entry, err := store.GetEntry(child)
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil || entry.Path != normalizeProjectTrustPath(child) || entry.Decision {
		t.Fatalf("child entry = %#v", entry)
	}
	if err := store.Clear(child); err != nil {
		t.Fatal(err)
	}
	if decision, found, err := store.Get(child); err != nil || !found || !decision {
		t.Fatalf("decision after clear = %t, found %t, err %v", decision, found, err)
	}
}

func TestProjectTrustStoreCanonicalizesSymlinkPaths(t *testing.T) {
	root := t.TempDir()
	actual := filepath.Join(root, "actual")
	alias := filepath.Join(root, "alias")
	if err := os.MkdirAll(actual, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(actual, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	store := NewProjectTrustStore(filepath.Join(root, "agent"))
	if err := store.Set(alias, true); err != nil {
		t.Fatal(err)
	}
	entry, err := store.GetEntry(actual)
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil || !entry.Decision || entry.Path != normalizeProjectTrustPath(actual) {
		t.Fatalf("canonical entry = %#v", entry)
	}
}

func TestProjectTrustStoreRejectsInvalidData(t *testing.T) {
	root := t.TempDir()
	store := NewProjectTrustStore(filepath.Join(root, "agent"))
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path(), []byte(`{"/tmp/project":"sometimes"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(root); err == nil || !strings.Contains(err.Error(), "must be true, false, or null") {
		t.Fatalf("error = %v", err)
	}
}

func TestProjectTrustStoreWritesDeterministicPrivateJSON(t *testing.T) {
	root := t.TempDir()
	store := NewProjectTrustStore(filepath.Join(root, "agent"))
	for _, path := range []string{
		filepath.Join(root, "z-project"),
		filepath.Join(root, "a-project"),
	} {
		if err := store.Set(path, true); err != nil {
			t.Fatal(err)
		}
	}
	content, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	aPath := normalizeProjectTrustPath(filepath.Join(root, "a-project"))
	zPath := normalizeProjectTrustPath(filepath.Join(root, "z-project"))
	encodedA, err := json.Marshal(aPath)
	if err != nil {
		t.Fatal(err)
	}
	encodedZ, err := json.Marshal(zPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(string(content), string(encodedA)) >= strings.Index(string(content), string(encodedZ)) {
		t.Fatalf("trust store is not sorted:\n%s", content)
	}
	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0o600 {
		t.Fatalf("trust store permissions = %o", got)
	}
}

func TestProjectTrustStoreConcurrentUpdatesPreserveEntries(t *testing.T) {
	root := t.TempDir()
	store := NewProjectTrustStore(filepath.Join(root, "agent"))
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for index := 0; index < 12; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errs <- store.Set(filepath.Join(root, "project", string(rune('a'+index))), index%2 == 0)
		}(index)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < 12; index++ {
		decision, found, err := store.Get(filepath.Join(root, "project", string(rune('a'+index))))
		if err != nil || !found || decision != (index%2 == 0) {
			t.Fatalf("entry %d = %t, found %t, err %v", index, decision, found, err)
		}
	}
}

func TestProjectTrustStoreRecoversAStaleCrossProcessLock(t *testing.T) {
	root := t.TempDir()
	store := NewProjectTrustStore(filepath.Join(root, "agent"))
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0o755); err != nil {
		t.Fatal(err)
	}
	lockPath := store.Path() + ".lock"
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	staleTime := time.Now().Add(-projectTrustLockStaleAfter - time.Second)
	if err := os.Chtimes(lockPath, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(root, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("stale lock still exists: %v", err)
	}
}

func TestHasTrustRequiringProjectResourcesPiMatrix(t *testing.T) {
	home := t.TempDir()
	cwd := filepath.Join(home, "work", "repo", "subdir")
	t.Setenv("HOME", home)
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if HasTrustRequiringProjectResources(cwd) {
		t.Fatal("user ~/.agents/skills must not require project trust")
	}
	if err := os.MkdirAll(filepath.Join(home, "work", "repo", ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !HasTrustRequiringProjectResources(cwd) {
		t.Fatal("ancestor project .agents/skills must require trust")
	}
	if err := os.RemoveAll(filepath.Join(home, "work", "repo", ".agents")); err != nil {
		t.Fatal(err)
	}
	for _, entry := range trustRequiringProjectConfigResources {
		t.Run(entry, func(t *testing.T) {
			project := filepath.Join(t.TempDir(), "project")
			target := filepath.Join(project, ConfigDirName, entry)
			if filepath.Ext(entry) != "" {
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatal(err)
			}
			if !HasTrustRequiringProjectResources(project) {
				t.Fatalf("%s did not require trust", entry)
			}
		})
	}
}

func TestResolveProjectTrustedPrecedenceAndPersistence(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "project")
	agentDir := filepath.Join(root, "agent")
	writeResourceFile(t, filepath.Join(cwd, ConfigDirName, "settings.json"), "{}")
	store := NewProjectTrustStore(agentDir)

	override := true
	trusted, err := ResolveProjectTrusted(ResolveProjectTrustOptions{
		CWD:           cwd,
		TrustStore:    store,
		TrustOverride: &override,
	})
	if err != nil || !trusted {
		t.Fatalf("override = %t, err %v", trusted, err)
	}
	if _, found, err := store.Get(cwd); err != nil || found {
		t.Fatalf("override must not persist: found %t, err %v", found, err)
	}

	if err := store.Set(cwd, false); err != nil {
		t.Fatal(err)
	}
	trusted, err = ResolveProjectTrusted(ResolveProjectTrustOptions{
		CWD:                 cwd,
		TrustStore:          store,
		DefaultProjectTrust: DefaultProjectTrustAlways,
	})
	if err != nil || trusted {
		t.Fatalf("saved decision = %t, err %v", trusted, err)
	}
	if err := store.Clear(cwd); err != nil {
		t.Fatal(err)
	}
	trusted, err = ResolveProjectTrusted(ResolveProjectTrustOptions{
		CWD:                 cwd,
		TrustStore:          store,
		DefaultProjectTrust: DefaultProjectTrustNever,
	})
	if err != nil || trusted {
		t.Fatalf("never decision = %t, err %v", trusted, err)
	}

	prompted := false
	trusted, err = ResolveProjectTrusted(ResolveProjectTrustOptions{
		CWD:        cwd,
		TrustStore: store,
		Prompt: func(_ string, options []ProjectTrustOption) (*ProjectTrustOption, error) {
			prompted = true
			for index := range options {
				if options[index].Label == "Trust" {
					return &options[index], nil
				}
			}
			return nil, nil
		},
	})
	if err != nil || !trusted || !prompted {
		t.Fatalf("prompt decision = %t, prompted %t, err %v", trusted, prompted, err)
	}
	if decision, found, err := store.Get(cwd); err != nil || !found || !decision {
		t.Fatalf("persisted prompt decision = %t, found %t, err %v", decision, found, err)
	}
}

func TestResolveProjectTrustedAutoTrustsProjectsWithoutResources(t *testing.T) {
	cwd := t.TempDir()
	prompted := false
	trusted, err := ResolveProjectTrusted(ResolveProjectTrustOptions{
		CWD:        cwd,
		TrustStore: NewProjectTrustStore(filepath.Join(t.TempDir(), "agent")),
		Prompt: func(_ string, _ []ProjectTrustOption) (*ProjectTrustOption, error) {
			prompted = true
			return nil, nil
		},
	})
	if err != nil || !trusted || prompted {
		t.Fatalf("trusted = %t, prompted = %t, err = %v", trusted, prompted, err)
	}
}
