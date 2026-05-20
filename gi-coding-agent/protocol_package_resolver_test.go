package gicodingagent

import (
	"path/filepath"
	"testing"
)

func TestProtocolPackageResolverLocalResources(t *testing.T) {
	t.Run("resolves local protocol extension paths", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		extensionPath := filepath.Join(manager.cwd, "ext.gi.json")
		writeGiProtocolExtensionDescriptor(t, extensionPath)

		result, err := manager.ResolveProtocolPackageResources([]string{extensionPath})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackageHasPath(result.Extensions, extensionPath) {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("handles directories with Gi manifest", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "my-package")
		extensionPath := filepath.Join(pkgDir, "src", "index.gi.json")
		skillPath := filepath.Join(pkgDir, "skills", "my-skill", "SKILL.md")
		writeGiProtocolExtensionDescriptor(t, extensionPath)
		writeResourceSkill(t, skillPath, "my-skill", "Test", "Content")
		writeProtocolPackageManifest(t, filepath.Join(pkgDir, "package.json"), map[string]any{
			"extensions": []any{"./src/index.gi.json"},
			"skills":     []any{"./skills"},
		})

		result, err := manager.ResolveProtocolPackageResources([]string{pkgDir})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackageHasPath(result.Extensions, extensionPath) || !protocolPackageHasPath(result.Skills, skillPath) {
			t.Fatalf("resources = %#v", result)
		}
	})

	t.Run("handles directories with auto-discovery layout", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "auto-pkg")
		extensionPath := filepath.Join(pkgDir, "extensions", "main.gi.json")
		themePath := filepath.Join(pkgDir, "themes", "dark.json")
		writeGiProtocolExtensionDescriptor(t, extensionPath)
		writeResourceFile(t, themePath, "{}")

		result, err := manager.ResolveProtocolPackageResources([]string{pkgDir})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackageHasPath(result.Extensions, extensionPath) || !protocolPackageHasPath(result.Themes, themePath) {
			t.Fatalf("resources = %#v", result)
		}
	})

	t.Run("stops recursing when a package skill directory contains SKILL.md", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "skill-root-pkg")
		rootSkill := filepath.Join(pkgDir, "skills", "root-skill", "SKILL.md")
		nestedSkill := filepath.Join(pkgDir, "skills", "root-skill", "nested-skill", "SKILL.md")
		writeResourceSkill(t, rootSkill, "root-skill", "Root", "Content")
		writeResourceSkill(t, nestedSkill, "nested-skill", "Nested", "Content")

		result, err := manager.ResolveProtocolPackageResources([]string{pkgDir})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackageHasPath(result.Skills, rootSkill) || protocolPackageHasPath(result.Skills, nestedSkill) {
			t.Fatalf("skills = %#v", result.Skills)
		}
	})
}

func writeProtocolPackageManifest(t *testing.T, path string, fields map[string]any) {
	t.Helper()
	gi := map[string]any{"manifestVersion": 1}
	for key, value := range fields {
		gi[key] = value
	}
	writeJSON(t, path, map[string]any{"gi": gi})
}

func protocolPackageHasPath(resources []ProtocolPackageResource, path string) bool {
	clean := filepath.Clean(path)
	for _, resource := range resources {
		if filepath.Clean(resource.Path) == clean && resource.Enabled {
			return true
		}
	}
	return false
}
