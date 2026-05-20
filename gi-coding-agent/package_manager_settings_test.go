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
