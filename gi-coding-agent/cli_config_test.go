package gicodingagent

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

func TestPackageResourceConfigHostTogglesPackageResource(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "project")
	agentDir := filepath.Join(root, "agent")
	pkgDir := filepath.Join(cwd, "toggle-pkg")
	writeResourceSkill(t, filepath.Join(pkgDir, "skills", "skill-a", "SKILL.md"), "skill-a", "Skill A", "Use skill A.")
	settings := NewSettingsManager(cwd, agentDir)
	settings.SetPackages([]any{pkgDir})
	terminal := gitui.NewVirtualTerminal(100, 24)
	host := &packageResourceConfigHost{cwd: cwd, agentDir: agentDir, terminal: terminal}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.Run()
	}()

	waitForConfigViewport(t, terminal, "Resource Configuration")
	waitForConfigViewport(t, terminal, "Skill skill-a")
	terminal.SendInput("\r")
	terminal.SendInput("\x1b")

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("config host did not exit")
	}
	reloaded := NewSettingsManager(cwd, agentDir)
	packages := settingsSlice(reloaded.global, "packages")
	if len(packages) != 1 {
		t.Fatalf("packages = %#v", packages)
	}
	object, ok := packages[0].(map[string]any)
	if !ok {
		t.Fatalf("package setting = %#v", packages[0])
	}
	if got := settingsStringSlice(object, "skills"); len(got) != 1 || got[0] != "-skills/skill-a/SKILL.md" {
		t.Fatalf("skill filters = %#v", got)
	}
}

func TestPackageResourceConfigHostTogglesTopLevelResource(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "project")
	agentDir := filepath.Join(root, "agent")
	writeResourceSkill(t, filepath.Join(cwd, ConfigDirName, "skills", "skill-a", "SKILL.md"), "skill-a", "Skill A", "Use skill A.")
	terminal := gitui.NewVirtualTerminal(100, 24)
	host := &packageResourceConfigHost{cwd: cwd, agentDir: agentDir, terminal: terminal}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.Run()
	}()

	waitForConfigViewport(t, terminal, "Resource Configuration")
	waitForConfigViewport(t, terminal, "Skill skill-a")
	terminal.SendInput("\r")
	terminal.SendInput("\x1b")

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("config host did not exit")
	}
	reloaded := NewSettingsManager(cwd, agentDir)
	if got := settingsStringSlice(reloaded.project, "skills"); len(got) != 1 || got[0] != "-skills/skill-a/SKILL.md" {
		t.Fatalf("project skill filters = %#v", got)
	}
}

func waitForConfigViewport(t *testing.T, terminal *gitui.VirtualTerminal, expected string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		terminal.WaitForRender()
		if viewport := strings.Join(terminal.GetViewport(), "\n"); strings.Contains(viewport, expected) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("viewport did not contain %q:\n%s", expected, strings.Join(terminal.GetViewport(), "\n"))
}
