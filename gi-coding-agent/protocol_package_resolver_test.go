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

func TestProtocolPackageManifestPatternRules(t *testing.T) {
	t.Run("supports glob patterns in manifest extensions", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "manifest-pkg")
		local := filepath.Join(pkgDir, "extensions", "local.gi.json")
		remote := filepath.Join(pkgDir, "node_modules", "dep", "extensions", "remote.gi.json")
		skip := filepath.Join(pkgDir, "node_modules", "dep", "extensions", "skip.gi.json")
		writeGiProtocolExtensionDescriptor(t, local)
		writeGiProtocolExtensionDescriptor(t, remote)
		writeGiProtocolExtensionDescriptor(t, skip)
		writeProtocolPackageManifest(t, filepath.Join(pkgDir, "package.json"), map[string]any{
			"extensions": []any{"extensions", "node_modules/dep/extensions", "!**/skip.gi.json"},
		})

		result, err := manager.ResolveProtocolPackageResources([]string{pkgDir})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackageHasPath(result.Extensions, local) ||
			!protocolPackageHasPath(result.Extensions, remote) ||
			protocolPackageHasPath(result.Extensions, skip) {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("supports glob patterns in manifest skills", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "skill-manifest-pkg")
		good := filepath.Join(pkgDir, "skills", "good-skill", "SKILL.md")
		bad := filepath.Join(pkgDir, "skills", "bad-skill", "SKILL.md")
		writeResourceSkill(t, good, "good-skill", "Good", "Content")
		writeResourceSkill(t, bad, "bad-skill", "Bad", "Content")
		writeProtocolPackageManifest(t, filepath.Join(pkgDir, "package.json"), map[string]any{
			"skills": []any{"skills", "!**/bad-skill"},
		})

		result, err := manager.ResolveProtocolPackageResources([]string{pkgDir})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackageHasPath(result.Skills, good) || protocolPackageHasPath(result.Skills, bad) {
			t.Fatalf("skills = %#v", result.Skills)
		}
	})

	t.Run("expands positive glob entries before collecting skills", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "skill-manifest-glob-pkg")
		pdf := filepath.Join(pkgDir, "plugins", "pdf-to-markdown", "skills", "pdf-to-markdown", "SKILL.md")
		dws := filepath.Join(pkgDir, "plugins", "nutrient-dws", "skills", "document-processor-api", "SKILL.md")
		writeResourceSkill(t, pdf, "pdf-to-markdown", "PDF to Markdown", "Content")
		writeResourceSkill(t, dws, "document-processor-api", "DWS", "Content")
		writeProtocolPackageManifest(t, filepath.Join(pkgDir, "package.json"), map[string]any{
			"skills": []any{"./plugins/*/skills"},
		})

		result, err := manager.ResolveProtocolPackageResources([]string{pkgDir})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackageHasPath(result.Skills, pdf) || !protocolPackageHasPath(result.Skills, dws) {
			t.Fatalf("skills = %#v", result.Skills)
		}
	})

	t.Run("handles force-include in manifest patterns", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "manifest-force-pkg")
		one := filepath.Join(pkgDir, "extensions", "one.gi.json")
		two := filepath.Join(pkgDir, "extensions", "two.gi.json")
		three := filepath.Join(pkgDir, "extensions", "three.gi.json")
		writeGiProtocolExtensionDescriptor(t, one)
		writeGiProtocolExtensionDescriptor(t, two)
		writeGiProtocolExtensionDescriptor(t, three)
		writeProtocolPackageManifest(t, filepath.Join(pkgDir, "package.json"), map[string]any{
			"extensions": []any{"extensions", "!**/two.gi.json", "+extensions/two.gi.json"},
		})

		result, err := manager.ResolveProtocolPackageResources([]string{pkgDir})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackageHasPath(result.Extensions, one) ||
			!protocolPackageHasPath(result.Extensions, two) ||
			!protocolPackageHasPath(result.Extensions, three) {
			t.Fatalf("extensions = %#v", result.Extensions)
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
