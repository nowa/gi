package gicodingagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageManagerSettingsNormalizationPiParity(t *testing.T) {
	t.Run("stores global local packages relative to agent settings base", func(t *testing.T) {
		agentDir, projectDir := createPackageManagerSettingsDirs(t)
		pkgDir := filepath.Join(projectDir, "packages", "local-global-pkg")
		createPackageManagerSettingsPackage(t, pkgDir)
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: projectDir, AgentDir: agentDir})

		added, err := manager.addSourceToSettings("./packages/local-global-pkg", false)
		if err != nil {
			t.Fatal(err)
		}
		if !added {
			t.Fatal("added = false, want true")
		}

		packages := packageManagerSettingsPackages(t, filepath.Join(agentDir, "settings.json"))
		want := expectedPackageManagerSettingsPath(t, agentDir, pkgDir)
		if len(packages) != 1 || packages[0] != want {
			t.Fatalf("packages = %#v, want [%q]", packages, want)
		}
	})

	t.Run("stores project local packages relative to .gi settings base", func(t *testing.T) {
		agentDir, projectDir := createPackageManagerSettingsDirs(t)
		pkgDir := filepath.Join(projectDir, "project-local-pkg")
		createPackageManagerSettingsPackage(t, pkgDir)
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: projectDir, AgentDir: agentDir})

		added, err := manager.addSourceToSettings("./project-local-pkg", true)
		if err != nil {
			t.Fatal(err)
		}
		if !added {
			t.Fatal("added = false, want true")
		}

		packages := packageManagerSettingsPackages(t, filepath.Join(projectDir, ConfigDirName, "settings.json"))
		want := expectedPackageManagerSettingsPath(t, filepath.Join(projectDir, ConfigDirName), pkgDir)
		if len(packages) != 1 || packages[0] != want {
			t.Fatalf("packages = %#v, want [%q]", packages, want)
		}
	})

	t.Run("removes local package entries using equivalent path forms", func(t *testing.T) {
		agentDir, projectDir := createPackageManagerSettingsDirs(t)
		pkgDir := filepath.Join(projectDir, "remove-local-pkg")
		createPackageManagerSettingsPackage(t, pkgDir)
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: projectDir, AgentDir: agentDir})

		if _, err := manager.addSourceToSettings("./remove-local-pkg", false); err != nil {
			t.Fatal(err)
		}
		removed, err := manager.removeSourceFromSettings(pkgDir+string(filepath.Separator), false)
		if err != nil {
			t.Fatal(err)
		}
		if !removed {
			t.Fatal("removed = false, want true")
		}
		if packages := packageManagerSettingsPackages(t, filepath.Join(agentDir, "settings.json")); len(packages) != 0 {
			t.Fatalf("packages after remove = %#v", packages)
		}
	})
}

func TestPackageManagerUpdatePrefixSuggestionsPiParity(t *testing.T) {
	t.Run("suggests git source prefixes for update lookups", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{
			CWD:             t.TempDir(),
			AgentDir:        t.TempDir(),
			SettingsManager: NewInMemorySettingsManager(map[string]any{"packages": []any{"git:github.com/example/repo"}}),
		})

		err := manager.Update("github.com/example/repo")
		if err == nil {
			t.Fatal("Update returned nil error")
		}
		want := "No matching package found for github.com/example/repo. Did you mean git:github.com/example/repo?"
		if err.Error() != want {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("rejects npm package sources", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir()})

		if _, err := manager.addSourceToSettings("npm:example", true); err == nil || !strings.Contains(err.Error(), "npm packages are not supported") {
			t.Fatalf("error = %v", err)
		}
		if err := manager.Update("npm:example"); err == nil || !strings.Contains(err.Error(), "npm packages are not supported") {
			t.Fatalf("update error = %v", err)
		}
	})
}

func TestPackageManagerListPackageResourceToggles(t *testing.T) {
	agentDir, projectDir := createPackageManagerSettingsDirs(t)
	pkgDir := filepath.Join(projectDir, "toggle-pkg")
	extensionPath := filepath.Join(pkgDir, "extensions", "alpha.gi.json")
	skillPath := filepath.Join(pkgDir, "skills", "skill-a", "SKILL.md")
	promptPath := filepath.Join(pkgDir, "prompts", "review.md")
	themePath := filepath.Join(pkgDir, "themes", "dark.json")
	writeGiProtocolExtensionDescriptor(t, extensionPath)
	writeResourceSkill(t, skillPath, "skill-a", "Skill A", "Use skill A.")
	writeResourceFile(t, promptPath, "# Review")
	writeResourceFile(t, themePath, "{}")
	writeProtocolPackageManifest(t, filepath.Join(pkgDir, "gi.package.json"), map[string]any{
		"extensions": []any{
			"extensions/alpha.gi.json",
			map[string]any{
				"id": "daemon",
				"entry": map[string]any{
					"kind":    "process",
					"command": []any{"gi-daemon"},
				},
			},
		},
		"skills":  []any{"skills/skill-a"},
		"prompts": []any{"prompts/review.md"},
		"themes":  []any{"themes/dark.json"},
	})
	settings := NewInMemorySettingsManager(map[string]any{"packages": []any{map[string]any{
		"source":     pkgDir,
		"skills":     []any{"-skills/skill-a/SKILL.md"},
		"extensions": []any{"-daemon"},
	}}})
	manager := NewDefaultPackageManager(PackageManagerOptions{CWD: projectDir, AgentDir: agentDir, SettingsManager: settings})

	items, err := manager.ListPackageResourceToggles()
	if err != nil {
		t.Fatal(err)
	}
	if !packageResourceToggleItemState(items, "skills", "skills/skill-a/SKILL.md", false) ||
		!packageResourceToggleItemState(items, "extensions", "extensions/alpha.gi.json", true) ||
		!packageResourceToggleItemState(items, "extensions", "daemon", false) ||
		!packageResourceToggleItemState(items, "prompts", "prompts/review.md", true) ||
		!packageResourceToggleItemState(items, "themes", "themes/dark.json", true) {
		t.Fatalf("items = %#v", items)
	}
}

func TestPackageManagerListTopLevelResourceToggles(t *testing.T) {
	agentDir, projectDir := createPackageManagerSettingsDirs(t)
	userSkill := filepath.Join(agentDir, "skills", "user-skill", "SKILL.md")
	projectPrompt := filepath.Join(projectDir, ConfigDirName, "prompts", "review.md")
	writeResourceSkill(t, userSkill, "user-skill", "User Skill", "Use user skill.")
	writeResourceFile(t, projectPrompt, "# Review")
	settings := NewInMemorySettingsManager(map[string]any{
		"skills": []any{"-skills/user-skill/SKILL.md"},
	})
	manager := NewDefaultPackageManager(PackageManagerOptions{CWD: projectDir, AgentDir: agentDir, SettingsManager: settings})

	items, err := manager.ListResourceToggles()
	if err != nil {
		t.Fatal(err)
	}
	if !packageResourceToggleItemState(items, "skills", "skills/user-skill/SKILL.md", false) ||
		!packageResourceToggleItemState(items, "prompts", "prompts/review.md", true) {
		t.Fatalf("items = %#v", items)
	}
	changed, err := manager.SetTopLevelResourceEnabled(TopLevelResourceToggle{
		Scope:        "project",
		ResourceType: "prompts",
		Pattern:      "prompts/review.md",
		Enabled:      false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if got := settingsStringSlice(settings.project, "prompts"); len(got) != 1 || got[0] != "-prompts/review.md" {
		t.Fatalf("project prompt filters = %#v", got)
	}
}

func TestPackageManagerInstallGitPackageGiProtocolScope(t *testing.T) {
	agentDir, projectDir := createPackageManagerSettingsDirs(t)
	source := "git:github.com/example/protocol-package"
	var cloneTarget string
	manager := NewDefaultPackageManager(PackageManagerOptions{
		CWD:      projectDir,
		AgentDir: agentDir,
		Operations: PackageManagerOperations{
			RunCommand: func(command string, args []string, _ PackageCommandOptions) error {
				if command != "git" || len(args) != 3 || args[0] != "clone" {
					t.Fatalf("unexpected command: %s %#v", command, args)
				}
				cloneTarget = args[2]
				writeGiProtocolExtensionDescriptor(t, filepath.Join(cloneTarget, "extensions", "index.gi.json"))
				return nil
			},
		},
	})

	if err := manager.Install(source, true); err != nil {
		t.Fatal(err)
	}
	wantTarget := filepath.Join(projectDir, ConfigDirName, "git", "github.com", "example", "protocol-package")
	if cloneTarget != wantTarget {
		t.Fatalf("clone target = %q, want %q", cloneTarget, wantTarget)
	}
	if packages := packageManagerSettingsPackages(t, filepath.Join(projectDir, ConfigDirName, "settings.json")); len(packages) != 1 || packages[0] != source {
		t.Fatalf("project packages = %#v", packages)
	}
	resources, err := manager.ResolveConfiguredProtocolPackageResources()
	if err != nil {
		t.Fatal(err)
	}
	if !protocolPackageHasSuffix(resources.Extensions, filepath.Join("extensions", "index.gi.json")) {
		t.Fatalf("extensions = %#v", resources.Extensions)
	}

	removed, err := manager.RemoveAndPersist(source, true)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("removed = false, want true")
	}
	if _, err := os.Stat(wantTarget); !os.IsNotExist(err) {
		t.Fatalf("installed package still exists or stat failed unexpectedly: %v", err)
	}
}

func createPackageManagerSettingsDirs(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	agentDir := filepath.Join(root, "agent")
	projectDir := filepath.Join(root, "project")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, ConfigDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	return agentDir, projectDir
}

func packageResourceToggleItemState(items []PackageResourceToggleItem, resourceType, pattern string, enabled bool) bool {
	for _, item := range items {
		if item.ResourceType == resourceType && item.Pattern == pattern && item.Enabled == enabled {
			return true
		}
	}
	return false
}

func createPackageManagerSettingsPackage(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "extensions"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func expectedPackageManagerSettingsPath(t *testing.T, baseDir, targetDir string) string {
	t.Helper()
	relative, err := filepath.Rel(baseDir, targetDir)
	if err != nil {
		t.Fatal(err)
	}
	relative = filepath.Clean(relative)
	if relative != "." && !strings.HasPrefix(relative, ".") {
		relative = "." + string(filepath.Separator) + relative
	}
	return relative
}

func packageManagerSettingsPackages(t *testing.T, settingsPath string) []string {
	t.Helper()
	settings := readSettingsJSON(t, settingsPath)
	values, _ := settings["packages"].([]any)
	packages := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			packages = append(packages, text)
		}
	}
	return packages
}
