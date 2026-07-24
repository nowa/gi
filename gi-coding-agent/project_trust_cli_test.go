package gicodingagent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

func TestParseArgsProjectTrustOverridesPiStyle(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want bool
	}{
		{name: "approve", args: []string{"--approve"}, want: true},
		{name: "approve short", args: []string{"-a"}, want: true},
		{name: "no approve", args: []string{"--no-approve"}, want: false},
		{name: "no approve short", args: []string{"-na"}, want: false},
		{name: "last wins", args: []string{"--approve", "--no-approve"}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := ParseArgs(test.args)
			if args.ProjectTrustOverride == nil || *args.ProjectTrustOverride != test.want {
				t.Fatalf("override = %#v", args.ProjectTrustOverride)
			}
		})
	}
	if got := ParseArgs(nil).ProjectTrustOverride; got != nil {
		t.Fatalf("default override = %#v", got)
	}
}

func TestCLIPackageListHonorsProjectTrust(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "project")
	agentDir := filepath.Join(root, "agent")
	writeSettingsJSON(t, filepath.Join(cwd, ConfigDirName, "settings.json"), map[string]any{
		"packages": []any{"npm:@project/pkg"},
	})

	run := func(args ...string) (int, string, string) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := RunCLI(CLIOptions{
			Args:     args,
			CWD:      cwd,
			AgentDir: agentDir,
			Stdin:    strings.NewReader(""),
			Stdout:   &stdout,
			Stderr:   &stderr,
		})
		return code, stdout.String(), stderr.String()
	}

	code, stdout, stderr := run("list")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "No packages installed.") ||
		strings.Contains(stdout, "@project/pkg") {
		t.Fatalf("untrusted list = code %d stdout %q stderr %q", code, stdout, stderr)
	}
	code, stdout, stderr = run("list", "--approve")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "Project packages:") ||
		!strings.Contains(stdout, "npm:@project/pkg") {
		t.Fatalf("approved list = code %d stdout %q stderr %q", code, stdout, stderr)
	}
	if err := NewProjectTrustStore(agentDir).Set(cwd, true); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = run("list")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "npm:@project/pkg") {
		t.Fatalf("remembered list = code %d stdout %q stderr %q", code, stdout, stderr)
	}
	code, stdout, stderr = run("list", "--no-approve")
	if code != 0 || stderr != "" || strings.Contains(stdout, "npm:@project/pkg") {
		t.Fatalf("overridden list = code %d stdout %q stderr %q", code, stdout, stderr)
	}
}

func TestCLIPackageListUsesInjectedProjectTrustPrompt(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "project")
	agentDir := filepath.Join(root, "agent")
	writeSettingsJSON(t, filepath.Join(cwd, ConfigDirName, "settings.json"), map[string]any{
		"packages": []any{"npm:@project/pkg"},
	})
	prompted := false
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunCLI(CLIOptions{
		Args:     []string{"list"},
		CWD:      cwd,
		AgentDir: agentDir,
		Stdin:    strings.NewReader(""),
		Stdout:   &stdout,
		Stderr:   &stderr,
		ProjectTrustPrompt: func(promptCWD string, options []ProjectTrustOption) (*ProjectTrustOption, error) {
			prompted = true
			if promptCWD != cwd || len(options) == 0 {
				t.Fatalf("prompt cwd = %q, options = %#v", promptCWD, options)
			}
			return &options[0], nil
		},
	})
	if code != 0 || !prompted || stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "npm:@project/pkg") {
		t.Fatalf("code = %d, prompted = %t, stdout = %q, stderr = %q", code, prompted, stdout.String(), stderr.String())
	}
	if decision, found, err := NewProjectTrustStore(agentDir).Get(cwd); err != nil || !found || !decision {
		t.Fatalf("saved decision = %t, found = %t, err = %v", decision, found, err)
	}
}

func TestCLICommandSettingsUpdateUsesSavedTrustOnly(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "project")
	agentDir := filepath.Join(root, "agent")
	writeSettingsJSON(t, filepath.Join(agentDir, "settings.json"), map[string]any{
		"defaultProjectTrust": "always",
	})
	writeSettingsJSON(t, filepath.Join(cwd, ConfigDirName, "settings.json"), map[string]any{
		"theme": "project",
	})
	prompted := false
	settings, trusted, err := newCLICommandSettingsManager(
		Args{},
		cwd,
		agentDir,
		func(string, []ProjectTrustOption) (*ProjectTrustOption, error) {
			prompted = true
			return nil, nil
		},
		true,
	)
	if err != nil || trusted || prompted || settings.IsProjectTrusted() {
		t.Fatalf("settings = %#v, trusted = %t, prompted = %t, err = %v", settings, trusted, prompted, err)
	}
	if err := NewProjectTrustStore(agentDir).Set(cwd, true); err != nil {
		t.Fatal(err)
	}
	settings, trusted, err = newCLICommandSettingsManager(Args{}, cwd, agentDir, nil, true)
	if err != nil || !trusted || !settings.IsProjectTrusted() || settings.GetTheme() != "project" {
		t.Fatalf("remembered settings = %#v, trusted = %t, err = %v", settings, trusted, err)
	}
}

func TestCLILocalPackageMutationRequiresProjectTrust(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "project")
	agentDir := filepath.Join(root, "agent")
	source := filepath.Join(root, "local-package")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(cwd, ConfigDirName, "settings.json")
	writeSettingsJSON(t, settingsPath, map[string]any{"packages": []any{"npm:existing"}})

	run := func(args ...string) (int, string) {
		var stderr bytes.Buffer
		code := RunCLI(CLIOptions{
			Args:     args,
			CWD:      cwd,
			AgentDir: agentDir,
			Stdin:    strings.NewReader(""),
			Stderr:   &stderr,
		})
		return code, stderr.String()
	}
	code, stderr := run("install", source, "--local")
	if code != 1 || !strings.Contains(stderr, "Project is not trusted") {
		t.Fatalf("untrusted install = code %d stderr %q", code, stderr)
	}
	if got := settingsStringSlice(readSettingsJSON(t, settingsPath), "packages"); len(got) != 1 || got[0] != "npm:existing" {
		t.Fatalf("packages after rejected install = %#v", got)
	}
	code, stderr = run("install", source, "--local", "--approve")
	if code != 0 || stderr != "" {
		t.Fatalf("approved install = code %d stderr %q", code, stderr)
	}
	if got := settingsStringSlice(readSettingsJSON(t, settingsPath), "packages"); len(got) != 2 {
		t.Fatalf("packages after approved install = %#v", got)
	}
}

func TestPackageManagerRejectsDirectUntrustedProjectStorageAccess(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "project")
	agentDir := filepath.Join(root, "agent")
	settings := NewSettingsManagerWithOptions(cwd, agentDir, SettingsManagerOptions{ProjectTrusted: false})
	manager := NewDefaultPackageManager(PackageManagerOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings})

	if _, err := manager.ResolveProtocolPackageSourceSpecs([]ProtocolPackageSourceSpec{{
		Source: filepath.Join(root, "pkg"),
		Scope:  "project",
	}}); err == nil || err.Error() != "Project is not trusted; refusing to access project package storage" {
		t.Fatalf("resolve error = %v", err)
	}
	if _, err := manager.SetTopLevelResourceEnabled(TopLevelResourceToggle{
		Scope:        "project",
		ResourceType: "skills",
		Pattern:      "skills/example/SKILL.md",
		Enabled:      false,
	}); err == nil || !strings.Contains(err.Error(), "Project is not trusted") {
		t.Fatalf("toggle error = %v", err)
	}
}

func TestCLIProjectTrustPromptSelectsAndCancels(t *testing.T) {
	cwd := t.TempDir()
	options := GetProjectTrustOptions(cwd, true)
	t.Run("select", func(t *testing.T) {
		terminal := gitui.NewVirtualTerminal(120, 24)
		resultCh := make(chan struct {
			result cliProjectTrustPromptResult
			err    error
		}, 1)
		go func() {
			result, err := runCLIProjectTrustPrompt(cwd, options, terminal)
			resultCh <- struct {
				result cliProjectTrustPromptResult
				err    error
			}{result: result, err: err}
		}()
		waitForViewport(t, terminal, "Trust project folder?")
		waitForViewport(t, terminal, ConfigDirName+" settings and resources")
		terminal.SendInput("\r")
		select {
		case out := <-resultCh:
			if out.err != nil || !out.result.Selected || out.result.Option == nil || !out.result.Option.Trusted {
				t.Fatalf("result = %#v, err = %v", out.result, out.err)
			}
		case <-time.After(time.Second):
			t.Fatal("project trust prompt did not finish")
		}
	})
	t.Run("cancel", func(t *testing.T) {
		terminal := gitui.NewVirtualTerminal(120, 24)
		resultCh := make(chan cliProjectTrustPromptResult, 1)
		go func() {
			result, _ := runCLIProjectTrustPrompt(cwd, options, terminal)
			resultCh <- result
		}()
		waitForViewport(t, terminal, "Trust project folder?")
		terminal.SendInput("\x1b")
		select {
		case result := <-resultCh:
			if result.Selected || result.Option != nil {
				t.Fatalf("result = %#v", result)
			}
		case <-time.After(time.Second):
			t.Fatal("project trust prompt cancel did not finish")
		}
	})
}

func TestCLIInteractiveTrustCommandPersistsDecision(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T", runtimeHost)
	}
	cwd := sessionHost.session.SessionManager.GetCWD()
	agentDir := sessionHost.settingsManager.agentDir
	terminal := gitui.NewVirtualTerminal(120, 28)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/trust")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Project trust")
	waitForViewport(t, terminal, "Saved decision: none")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Saved trust decision: trusted")
	if decision, found, err := NewProjectTrustStore(agentDir).Get(cwd); err != nil || !found || !decision {
		t.Fatalf("saved decision = %t, found %t, err %v", decision, found, err)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive trust host did not stop")
	}
}
