package shellconfig

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestIsLegacyWSLBashPath(t *testing.T) {
	for _, path := range []string{
		`C:\Windows\System32\bash.exe`,
		`c:/WINDOWS/sysnative/BASH.EXE`,
	} {
		if !IsLegacyWSLBashPath(path) {
			t.Fatalf("%q should be a legacy WSL bash path", path)
		}
	}
	for _, path := range []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Windows\System32\wsl.exe`,
		`/bin/bash`,
	} {
		if IsLegacyWSLBashPath(path) {
			t.Fatalf("%q should not be a legacy WSL bash path", path)
		}
	}
}

func TestResolveCustomLegacyWSLBashUsesStdin(t *testing.T) {
	const shell = `C:\Windows\System32\bash.exe`
	config, err := Resolve(shell, ResolveOptions{
		GOOS:       "windows",
		FileExists: func(path string) bool { return path == shell },
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.Shell != shell ||
		!reflect.DeepEqual(config.Args, []string{"-s"}) ||
		config.Transport != CommandStdin {
		t.Fatalf("config = %#v", config)
	}

	invocation := config.Invocation(`echo "$HOME"`)
	if invocation.Command != shell ||
		!reflect.DeepEqual(invocation.Args, []string{"-s"}) ||
		!invocation.UsesStdin ||
		invocation.Stdin != `echo "$HOME"` {
		t.Fatalf("invocation = %#v", invocation)
	}
	invocation.Args[0] = "changed"
	if config.Args[0] != "-s" {
		t.Fatalf("invocation shares config args: %#v", config)
	}
}

func TestResolveUnixShellPriority(t *testing.T) {
	config, err := Resolve("", ResolveOptions{
		GOOS:       "linux",
		FileExists: func(path string) bool { return path == "/bin/bash" },
		LookPath: func(string) (string, error) {
			t.Fatal("LookPath should not run when /bin/bash exists")
			return "", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.Shell != "/bin/bash" {
		t.Fatalf("shell = %q", config.Shell)
	}

	config, err = Resolve("", ResolveOptions{
		GOOS:       "linux",
		FileExists: func(string) bool { return false },
		LookPath: func(name string) (string, error) {
			if name != "bash" {
				t.Fatalf("LookPath(%q)", name)
			}
			return "/opt/bin/bash", nil
		},
	})
	if err != nil || config.Shell != "/opt/bin/bash" {
		t.Fatalf("PATH config = %#v, %v", config, err)
	}

	config, err = Resolve("", ResolveOptions{
		GOOS:       "linux",
		FileExists: func(string) bool { return false },
		LookPath: func(string) (string, error) {
			return "", errors.New("not found")
		},
	})
	if err != nil ||
		config.Shell != "sh" ||
		!reflect.DeepEqual(config.Args, []string{"-c"}) {
		t.Fatalf("fallback config = %#v, %v", config, err)
	}
}

func TestResolveWindowsGitBashThenPath(t *testing.T) {
	const gitBash = `C:\Program Files\Git\bin\bash.exe`
	config, err := Resolve("", ResolveOptions{
		GOOS: "windows",
		Env:  map[string]string{"ProgramFiles": `C:\Program Files`},
		FileExists: func(path string) bool {
			return path == gitBash
		},
		LookPath: func(string) (string, error) {
			t.Fatal("LookPath should not run when Git Bash exists")
			return "", nil
		},
	})
	if err != nil || config.Shell != gitBash {
		t.Fatalf("Git Bash config = %#v, %v", config, err)
	}

	const pathBash = `C:\msys64\usr\bin\bash.exe`
	config, err = Resolve("", ResolveOptions{
		GOOS: "windows",
		FileExists: func(path string) bool {
			return path == pathBash
		},
		LookPath: func(name string) (string, error) {
			if name != "bash.exe" {
				t.Fatalf("LookPath(%q)", name)
			}
			return pathBash, nil
		},
	})
	if err != nil || config.Shell != pathBash {
		t.Fatalf("PATH config = %#v, %v", config, err)
	}
}

func TestResolveWindowsReportsSearchState(t *testing.T) {
	_, err := Resolve("", ResolveOptions{
		GOOS: "windows",
		Env:  map[string]string{"ProgramFiles": `C:\Programs`},
		FileExists: func(string) bool {
			return false
		},
		LookPath: func(string) (string, error) {
			return "", errors.New("not found")
		},
	})
	if err == nil {
		t.Fatal("expected missing bash error")
	}
	for _, expected := range []string{
		"No bash shell found",
		"Set shellPath in settings.json",
		`C:\Programs\Git\bin\bash.exe`,
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("error = %q, want %q", err, expected)
		}
	}
}
