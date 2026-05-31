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
	waitForConfigViewport(t, terminal, "Skills")
	waitForConfigViewport(t, terminal, "skill-a")
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
	waitForConfigViewport(t, terminal, "Project (.gi/)")
	waitForConfigViewport(t, terminal, "Skills")
	waitForConfigViewport(t, terminal, "skill-a")
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

func TestPackageResourceConfigComponentFiltersLikePiSelector(t *testing.T) {
	resources := []PackageResourceToggleItem{
		{
			Source:       "official:guard",
			Scope:        "user",
			ResourceType: "skills",
			Pattern:      "skills/review/SKILL.md",
			Path:         "/tmp/pkg/skills/review/SKILL.md",
			DisplayName:  "review",
			Enabled:      true,
			Metadata:     ProtocolSourceInfo{Origin: "package", Source: "official:guard", Scope: "user"},
		},
		{
			Source:       "auto",
			Scope:        "project",
			ResourceType: "prompts",
			Pattern:      "prompts/ship.md",
			Path:         "/tmp/project/.gi/prompts/ship.md",
			DisplayName:  "ship.md",
			Enabled:      true,
			Metadata:     ProtocolSourceInfo{Origin: "top-level", Source: "auto", Scope: "project"},
		},
	}
	component := newPackageResourceConfigComponent(nil, resources, make(chan struct{}, 1), nil)

	rendered := strings.Join(component.Render(100), "\n")
	for _, want := range []string{"Resource Configuration", "official:guard (user)", "Skills", "review", "Project (.gi/)", "Prompts", "ship.md", "Type to filter resources"} {
		if !strings.Contains(StripAnsi(rendered), want) {
			t.Fatalf("render missing %q:\n%s", want, StripAnsi(rendered))
		}
	}

	component.HandleInput("s")
	component.HandleInput("h")
	component.HandleInput("i")
	component.HandleInput("p")
	filtered := StripAnsi(strings.Join(component.Render(100), "\n"))
	if !strings.Contains(filtered, "Project (.gi/)") || !strings.Contains(filtered, "Prompts") || !strings.Contains(filtered, "ship.md") {
		t.Fatalf("filter should keep matching group/subgroup/item:\n%s", filtered)
	}
	if strings.Contains(filtered, "official:guard") || strings.Contains(filtered, "review") {
		t.Fatalf("filter should remove non-matching resources:\n%s", filtered)
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
