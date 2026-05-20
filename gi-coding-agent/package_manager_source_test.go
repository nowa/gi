package gicodingagent

import "testing"

func TestPackageManagerGitSourceParsingPiSSHMatrix(t *testing.T) {
	manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})

	assertGitSource(t, manager.ParseSource("https://github.com/user/repo"), "github.com", "user/repo", "https://github.com/user/repo", "", false)
	assertGitSource(t, manager.ParseSource("ssh://git@github.com/user/repo"), "github.com", "user/repo", "ssh://git@github.com/user/repo", "", false)
	assertGitSource(t, manager.ParseSource("git:git@github.com:user/repo"), "github.com", "user/repo", "git@github.com:user/repo", "", false)
	assertGitSource(t, manager.ParseSource("git:github.com/user/repo"), "github.com", "user/repo", "https://github.com/user/repo", "", false)
	assertGitSource(t, manager.ParseSource("git:git@github.com:user/repo@v1.0.0"), "github.com", "user/repo", "git@github.com:user/repo", "v1.0.0", true)

	if source := manager.ParseSource("git@github.com:user/repo"); source.Type != "local" {
		t.Fatalf("git@ without prefix type = %q, want local", source.Type)
	}
	if source := manager.ParseSource("github.com/user/repo"); source.Type != "local" {
		t.Fatalf("host/path without prefix type = %q, want local", source.Type)
	}

	prefixed := manager.GetPackageIdentity("git:git@github.com:user/repo")
	https := manager.GetPackageIdentity("https://github.com/user/repo")
	ssh := manager.GetPackageIdentity("ssh://git@github.com/user/repo")
	if prefixed != "git:github.com/user/repo" || prefixed != https || prefixed != ssh {
		t.Fatalf("identities = %q %q %q", prefixed, https, ssh)
	}
}

func TestPackageManagerSourceParsingPiDocsAndHTTPSMatrix(t *testing.T) {
	manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})

	for _, source := range []string{"npm:@scope/pkg@1.2.3", "npm:pkg"} {
		if parsed := manager.ParseSource(source); parsed.Type != "npm" {
			t.Fatalf("ParseSource(%q).Type = %q, want npm", source, parsed.Type)
		}
	}
	for _, source := range []string{
		"git:github.com/user/repo@v1",
		"https://github.com/user/repo@v1",
		"git:git@github.com:user/repo@v1",
		"ssh://git@github.com/user/repo@v1",
	} {
		if parsed := manager.ParseSource(source); parsed.Type != "git" {
			t.Fatalf("ParseSource(%q).Type = %q, want git", source, parsed.Type)
		}
	}
	for _, source := range []string{"/absolute/path/to/package", "./relative/path/to/package", "../relative/path/to/package"} {
		if parsed := manager.ParseSource(source); parsed.Type != "local" {
			t.Fatalf("ParseSource(%q).Type = %q, want local", source, parsed.Type)
		}
	}

	dotSlash := manager.ParseSource("./packages/agent-timers")
	if dotSlash.Type != "local" || dotSlash.Path != "./packages/agent-timers" {
		t.Fatalf("dot slash source = %#v", dotSlash)
	}
	dotDotSlash := manager.ParseSource("../packages/agent-timers")
	if dotDotSlash.Type != "local" || dotDotSlash.Path != "../packages/agent-timers" {
		t.Fatalf("dot dot slash source = %#v", dotDotSlash)
	}

	assertGitSource(t, manager.ParseSource("https://github.com/user/repo"), "github.com", "user/repo", "https://github.com/user/repo", "", false)
	assertGitSource(t, manager.ParseSource("git:https://github.com/user/repo"), "github.com", "user/repo", "https://github.com/user/repo", "", false)
	assertGitSource(t, manager.ParseSource("https://github.com/user/repo@v1.2.3"), "github.com", "user/repo", "https://github.com/user/repo", "v1.2.3", true)
	assertGitSource(t, manager.ParseSource("git:github.com/user/repo"), "github.com", "user/repo", "https://github.com/user/repo", "", false)
	if parsed := manager.ParseSource("github.com/user/repo"); parsed.Type != "local" {
		t.Fatalf("host/path without prefix = %#v", parsed)
	}
	assertGitSource(t, manager.ParseSource("https://github.com/user/repo.git"), "github.com", "user/repo", "https://github.com/user/repo.git", "", false)
	assertGitSource(t, manager.ParseSource("https://gitlab.com/user/repo"), "gitlab.com", "user/repo", "https://gitlab.com/user/repo", "", false)
	assertGitSource(t, manager.ParseSource("https://bitbucket.org/user/repo"), "bitbucket.org", "user/repo", "https://bitbucket.org/user/repo", "", false)
	assertGitSource(t, manager.ParseSource("https://codeberg.org/user/repo"), "codeberg.org", "user/repo", "https://codeberg.org/user/repo", "", false)

	identities := []string{
		manager.GetPackageIdentity("https://github.com/user/repo"),
		manager.GetPackageIdentity("https://github.com/user/repo@v1.0.0"),
		manager.GetPackageIdentity("git:github.com/user/repo"),
		manager.GetPackageIdentity("https://github.com/user/repo.git"),
	}
	for _, identity := range identities {
		if identity != "git:github.com/user/repo" {
			t.Fatalf("identity = %q, want git:github.com/user/repo", identity)
		}
	}
	if manager.GetPackageIdentity("git:github.com/user/repo") != manager.GetPackageIdentity("https://github.com/user/repo.git") {
		t.Fatal("supported git URL formats should deduplicate to the same identity")
	}

	mainRef := manager.ParseSource("https://github.com/user/repo@main")
	if mainRef.Ref != "main" || !mainRef.Pinned {
		t.Fatalf("main ref = %#v", mainRef)
	}
	branchRef := manager.ParseSource("https://github.com/user/repo@feature/branch")
	if branchRef.Ref != "feature/branch" {
		t.Fatalf("branch ref = %#v", branchRef)
	}
}

func TestPackageManagerPackageIdentityDedupePiParity(t *testing.T) {
	manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})

	t.Run("dedupes SSH and HTTPS URLs for same repo", func(t *testing.T) {
		httpsIdentity := manager.GetPackageIdentity("https://github.com/user/repo")
		sshIdentity := manager.GetPackageIdentity("git:git@github.com:user/repo")
		if httpsIdentity != "git:github.com/user/repo" || httpsIdentity != sshIdentity {
			t.Fatalf("identities = %q %q", httpsIdentity, sshIdentity)
		}
	})

	t.Run("dedupes SSH and HTTPS with refs", func(t *testing.T) {
		httpsIdentity := manager.GetPackageIdentity("https://github.com/user/repo@v1.0.0")
		sshIdentity := manager.GetPackageIdentity("git:git@github.com:user/repo@v1.0.0")
		if httpsIdentity != "git:github.com/user/repo" || httpsIdentity != sshIdentity {
			t.Fatalf("identities = %q %q", httpsIdentity, sshIdentity)
		}
	})

	t.Run("dedupes SSH URL with ssh protocol and git@ format", func(t *testing.T) {
		sshProtocolIdentity := manager.GetPackageIdentity("ssh://git@github.com/user/repo")
		gitAtIdentity := manager.GetPackageIdentity("git:git@github.com:user/repo")
		if sshProtocolIdentity != "git:github.com/user/repo" || sshProtocolIdentity != gitAtIdentity {
			t.Fatalf("identities = %q %q", sshProtocolIdentity, gitAtIdentity)
		}
	})

	t.Run("dedupes all supported URL formats for same repo", func(t *testing.T) {
		sources := []string{
			"https://github.com/user/repo",
			"https://github.com/user/repo.git",
			"ssh://git@github.com/user/repo",
			"git:https://github.com/user/repo",
			"git:github.com/user/repo",
			"git:git@github.com:user/repo",
			"git:git@github.com:user/repo.git",
		}
		for _, source := range sources {
			if identity := manager.GetPackageIdentity(source); identity != "git:github.com/user/repo" {
				t.Fatalf("identity for %q = %q", source, identity)
			}
		}
	})

	t.Run("keeps different repos separate", func(t *testing.T) {
		repo1 := manager.GetPackageIdentity("https://github.com/user/repo1")
		repo2 := manager.GetPackageIdentity("git:git@github.com:user/repo2")
		if repo1 != "git:github.com/user/repo1" || repo2 != "git:github.com/user/repo2" || repo1 == repo2 {
			t.Fatalf("identities = %q %q", repo1, repo2)
		}
	})
}

func assertGitSource(t *testing.T, source PackageSource, host, path, repo, ref string, pinned bool) {
	t.Helper()
	if source.Type != "git" || source.Host != host || source.Path != path || source.Repo != repo || source.Ref != ref || source.Pinned != pinned {
		t.Fatalf("source = %#v, want host=%q path=%q repo=%q ref=%q pinned=%v", source, host, path, repo, ref, pinned)
	}
}
