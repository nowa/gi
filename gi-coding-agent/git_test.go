package gicodingagent

import "testing"

func TestParseGitURLProtocolURLs(t *testing.T) {
	t.Run("should parse HTTPS URL", func(t *testing.T) {
		source, ok := ParseGitURL("https://github.com/user/repo")
		if !ok || source.Host != "github.com" || source.Path != "user/repo" || source.Repo != "https://github.com/user/repo" {
			t.Fatalf("source = %#v, %v", source, ok)
		}
	})

	t.Run("should parse ssh:// URL", func(t *testing.T) {
		source, ok := ParseGitURL("ssh://git@github.com/user/repo")
		if !ok || source.Host != "github.com" || source.Path != "user/repo" || source.Repo != "ssh://git@github.com/user/repo" {
			t.Fatalf("source = %#v, %v", source, ok)
		}
	})

	t.Run("should parse protocol URL with ref", func(t *testing.T) {
		source, ok := ParseGitURL("https://github.com/user/repo@v1.0.0")
		if !ok || source.Host != "github.com" || source.Path != "user/repo" || source.Ref != "v1.0.0" || source.Repo != "https://github.com/user/repo" {
			t.Fatalf("source = %#v, %v", source, ok)
		}
	})
}

func TestParseGitURLShorthandWithGitPrefix(t *testing.T) {
	t.Run("should parse git@host:path with git: prefix", func(t *testing.T) {
		source, ok := ParseGitURL("git:git@github.com:user/repo")
		if !ok || source.Host != "github.com" || source.Path != "user/repo" || source.Repo != "git@github.com:user/repo" {
			t.Fatalf("source = %#v, %v", source, ok)
		}
	})

	t.Run("should parse host/path shorthand with git: prefix", func(t *testing.T) {
		source, ok := ParseGitURL("git:github.com/user/repo")
		if !ok || source.Host != "github.com" || source.Path != "user/repo" || source.Repo != "https://github.com/user/repo" {
			t.Fatalf("source = %#v, %v", source, ok)
		}
	})

	t.Run("should parse shorthand with ref and git: prefix", func(t *testing.T) {
		source, ok := ParseGitURL("git:git@github.com:user/repo@v1.0.0")
		if !ok || source.Host != "github.com" || source.Path != "user/repo" || source.Ref != "v1.0.0" || source.Repo != "git@github.com:user/repo" {
			t.Fatalf("source = %#v, %v", source, ok)
		}
	})
}

func TestParseGitURLRejectsUnsupportedShorthandWithoutGitPrefix(t *testing.T) {
	for _, value := range []string{
		"git@github.com:user/repo",
		"github.com/user/repo",
		"user/repo",
	} {
		if source, ok := ParseGitURL(value); ok {
			t.Fatalf("ParseGitURL(%q) = %#v, true", value, source)
		}
	}
}
