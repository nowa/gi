// Package shellconfig resolves one immutable shell invocation policy for all
// coding-agent command execution paths.
package shellconfig

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// CommandTransport identifies where the shell receives command text.
type CommandTransport uint8

const (
	CommandArgument CommandTransport = iota
	CommandStdin
)

// Config is the resolved, reusable shell policy.
type Config struct {
	Shell     string
	Args      []string
	Transport CommandTransport
}

// Invocation is a detached command-specific projection of Config.
type Invocation struct {
	Command   string
	Args      []string
	Stdin     string
	UsesStdin bool
}

// ResolveOptions supplies target-platform state and injectable filesystem
// boundaries. Zero values use the current process.
type ResolveOptions struct {
	GOOS       string
	Env        map[string]string
	FileExists func(string) bool
	LookPath   func(string) (string, error)
}

var legacyWSLBashPathPattern = regexp.MustCompile(
	`^[a-z]:\\windows\\(system32|sysnative)\\bash\.exe$`,
)

// Resolve applies Pi's shell resolution order with Go-native path lookup.
func Resolve(
	customShellPath string,
	options ResolveOptions,
) (Config, error) {
	goos := options.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	fileExists := options.FileExists
	if fileExists == nil {
		fileExists = shellFileExists
	}
	lookPath := options.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	if customShellPath != "" {
		if !fileExists(customShellPath) {
			return Config{}, fmt.Errorf(
				"Custom shell path not found: %s",
				customShellPath,
			)
		}
		return ConfigForBash(customShellPath), nil
	}

	if goos == "windows" {
		candidates := windowsGitBashCandidates(options.Env)
		for _, candidate := range candidates {
			if fileExists(candidate) {
				return ConfigForBash(candidate), nil
			}
		}
		if candidate, err := lookPath("bash.exe"); err == nil &&
			candidate != "" &&
			fileExists(candidate) {
			return ConfigForBash(candidate), nil
		}
		return Config{}, errors.New(
			formatMissingWindowsBashError(candidates),
		)
	}

	if fileExists("/bin/bash") {
		return ConfigForBash("/bin/bash"), nil
	}
	if candidate, err := lookPath("bash"); err == nil &&
		candidate != "" {
		return ConfigForBash(candidate), nil
	}
	return Config{
		Shell: "sh",
		Args:  []string{"-c"},
	}, nil
}

// ConfigForBash selects stdin transport for the legacy Windows WSL launcher.
func ConfigForBash(shell string) Config {
	if IsLegacyWSLBashPath(shell) {
		return Config{
			Shell:     shell,
			Args:      []string{"-s"},
			Transport: CommandStdin,
		}
	}
	return Config{
		Shell: shell,
		Args:  []string{"-c"},
	}
}

// IsLegacyWSLBashPath identifies Windows' historical WSL bash launcher.
func IsLegacyWSLBashPath(path string) bool {
	normalized := strings.ToLower(
		strings.ReplaceAll(path, "/", `\`),
	)
	return legacyWSLBashPathPattern.MatchString(normalized)
}

// Invocation projects command text according to the resolved transport without
// sharing the Config Args backing array.
func (c Config) Invocation(command string) Invocation {
	args := append([]string(nil), c.Args...)
	invocation := Invocation{
		Command: c.Shell,
		Args:    args,
	}
	if c.Transport == CommandStdin {
		invocation.Stdin = command
		invocation.UsesStdin = true
		return invocation
	}
	invocation.Args = append(invocation.Args, command)
	return invocation
}

func shellFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func windowsGitBashCandidates(env map[string]string) []string {
	value := func(key string) string {
		if env != nil {
			return env[key]
		}
		return os.Getenv(key)
	}
	var candidates []string
	for _, root := range []string{
		value("ProgramFiles"),
		value("ProgramFiles(x86)"),
	} {
		if root == "" {
			continue
		}
		candidates = append(
			candidates,
			strings.TrimRight(root, `\/`)+
				`\Git\bin\bash.exe`,
		)
	}
	return candidates
}

func formatMissingWindowsBashError(candidates []string) string {
	var searched string
	if len(candidates) > 0 {
		searched = "\n\nSearched Git Bash in:\n"
		for _, candidate := range candidates {
			searched += "  " + candidate + "\n"
		}
		searched = strings.TrimSuffix(searched, "\n")
	}
	return "No bash shell found. Options:\n" +
		"  1. Install Git for Windows: https://git-scm.com/download/win\n" +
		"  2. Add your bash to PATH (Cygwin, MSYS2, etc.)\n" +
		"  3. Set shellPath in settings.json" +
		searched
}
