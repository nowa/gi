package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestProtocolPackageResolverLocalResources(t *testing.T) {
	t.Run("returns no package-sourced paths when no sources configured", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})

		result, err := manager.ResolveConfiguredProtocolPackageResources()
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Extensions) != 0 || len(result.Skills) != 0 || len(result.Prompts) != 0 || len(result.Themes) != 0 {
			t.Fatalf("resources = %#v", result)
		}
	})

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
		writeProtocolPackageManifest(t, filepath.Join(pkgDir, "gi.package.json"), map[string]any{
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

func TestProtocolPackageResolverOfficialResources(t *testing.T) {
	t.Run("resolves official packages as materialized Gi package artifacts", func(t *testing.T) {
		agentDir := t.TempDir()
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: agentDir, SettingsManager: NewInMemorySettingsManager(nil)})

		source := manager.ParseSource("official:gi-plan-mode")
		if source.Type != "official" || source.Path != "gi-plan-mode" || manager.GetPackageIdentity("official:gi-plan-mode") != "official:gi-plan-mode" {
			t.Fatalf("source = %#v identity = %q", source, manager.GetPackageIdentity("official:gi-plan-mode"))
		}

		result, err := manager.ResolveProtocolPackageResources([]string{"official:gi-plan-mode"})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackageHasSuffix(result.Extensions, filepath.Join("official-packages", "gi-plan-mode", "extensions", "main.gi.json")) ||
			!protocolPackageHasSuffix(result.Skills, filepath.Join("official-packages", "gi-plan-mode", "skills", "plan-mode", "SKILL.md")) ||
			!protocolPackageHasSuffix(result.Prompts, filepath.Join("official-packages", "gi-plan-mode", "prompts", "plan.md")) {
			t.Fatalf("resources = %#v", result)
		}
		if result.Extensions[0].Metadata.Source != "official:gi-plan-mode" || result.Extensions[0].Metadata.Origin != "package" {
			t.Fatalf("metadata = %#v", result.Extensions[0].Metadata)
		}
	})

	t.Run("install stores official source and rejects unknown official packages", func(t *testing.T) {
		settings := NewInMemorySettingsManager(nil)
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: settings})

		if err := manager.Install("official:gi-tools-ui", false); err != nil {
			t.Fatal(err)
		}
		if packages := settingsPackagesToStrings(settings.GetPackages()); len(packages) != 1 || packages[0] != "official:gi-tools-ui" {
			t.Fatalf("packages = %#v", packages)
		}
		if err := manager.Install("official:not-real", false); err == nil {
			t.Fatal("expected unknown official package error")
		}
	})
}

func TestOfficialPackageCatalogMatchesProtocolRegistry(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "protocol", "spec", "registries", "official-packages.json"))
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Packages []struct {
			Name string `json:"name"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(content, &registry); err != nil {
		t.Fatal(err)
	}
	want := make([]string, 0, len(registry.Packages))
	for _, pkg := range registry.Packages {
		want = append(want, pkg.Name)
	}
	sort.Strings(want)
	if got := OfficialPackageNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("official packages = %#v, want %#v", got, want)
	}

	manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
	for _, name := range want {
		result, err := manager.ResolveProtocolPackageResources([]string{"official:" + name})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(result.Extensions) != 1 || len(result.Skills) == 0 || len(result.Prompts) == 0 {
			t.Fatalf("%s resources = %#v", name, result)
		}
	}
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
		writeProtocolPackageManifest(t, filepath.Join(pkgDir, "gi.package.json"), map[string]any{
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
		writeProtocolPackageManifest(t, filepath.Join(pkgDir, "gi.package.json"), map[string]any{
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
		writeProtocolPackageManifest(t, filepath.Join(pkgDir, "gi.package.json"), map[string]any{
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
		writeProtocolPackageManifest(t, filepath.Join(pkgDir, "gi.package.json"), map[string]any{
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

func TestProtocolPackageResourceFilterRules(t *testing.T) {
	t.Run("applies user filters on top of manifest filters", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "layered-pkg")
		foo := filepath.Join(pkgDir, "extensions", "foo.gi.json")
		bar := filepath.Join(pkgDir, "extensions", "bar.gi.json")
		baz := filepath.Join(pkgDir, "extensions", "baz.gi.json")
		writeGiProtocolExtensionDescriptor(t, foo)
		writeGiProtocolExtensionDescriptor(t, bar)
		writeGiProtocolExtensionDescriptor(t, baz)
		writeProtocolPackageManifest(t, filepath.Join(pkgDir, "gi.package.json"), map[string]any{
			"extensions": []any{"extensions", "!**/baz.gi.json"},
		})

		result, err := manager.ResolveProtocolPackageSourceSpecs([]ProtocolPackageSourceSpec{{
			Source: pkgDir,
			Filters: ProtocolPackageResourceFilters{
				Extensions: []string{"!**/bar.gi.json"},
			},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackagePathEnabled(result.Extensions, foo) ||
			protocolPackagePathEnabled(result.Extensions, bar) ||
			protocolPackageHasAnyPath(result.Extensions, baz) {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("excludes extensions from package with pattern", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "pattern-pkg")
		foo := filepath.Join(pkgDir, "extensions", "foo.gi.json")
		bar := filepath.Join(pkgDir, "extensions", "bar.gi.json")
		baz := filepath.Join(pkgDir, "extensions", "baz.gi.json")
		writeGiProtocolExtensionDescriptor(t, foo)
		writeGiProtocolExtensionDescriptor(t, bar)
		writeGiProtocolExtensionDescriptor(t, baz)

		result, err := manager.ResolveProtocolPackageSourceSpecs([]ProtocolPackageSourceSpec{{
			Source:  pkgDir,
			Filters: ProtocolPackageResourceFilters{Extensions: []string{"!**/baz.gi.json"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackagePathEnabled(result.Extensions, foo) ||
			!protocolPackagePathEnabled(result.Extensions, bar) ||
			!protocolPackagePathDisabled(result.Extensions, baz) {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("filters themes from package", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "theme-pkg")
		nice := filepath.Join(pkgDir, "themes", "nice.json")
		ugly := filepath.Join(pkgDir, "themes", "ugly.json")
		writeResourceFile(t, nice, "{}")
		writeResourceFile(t, ugly, "{}")

		result, err := manager.ResolveProtocolPackageSourceSpecs([]ProtocolPackageSourceSpec{{
			Source:  pkgDir,
			Filters: ProtocolPackageResourceFilters{Themes: []string{"!ugly.json"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackagePathEnabled(result.Themes, nice) || !protocolPackagePathDisabled(result.Themes, ugly) {
			t.Fatalf("themes = %#v", result.Themes)
		}
	})

	t.Run("combines include and exclude patterns", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "combo-pkg")
		alpha := filepath.Join(pkgDir, "extensions", "alpha.gi.json")
		beta := filepath.Join(pkgDir, "extensions", "beta.gi.json")
		gamma := filepath.Join(pkgDir, "extensions", "gamma.gi.json")
		writeGiProtocolExtensionDescriptor(t, alpha)
		writeGiProtocolExtensionDescriptor(t, beta)
		writeGiProtocolExtensionDescriptor(t, gamma)

		result, err := manager.ResolveProtocolPackageSourceSpecs([]ProtocolPackageSourceSpec{{
			Source: pkgDir,
			Filters: ProtocolPackageResourceFilters{
				Extensions: []string{"**/alpha.gi.json", "**/beta.gi.json", "!**/beta.gi.json"},
			},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackagePathEnabled(result.Extensions, alpha) ||
			!protocolPackagePathDisabled(result.Extensions, beta) ||
			!protocolPackagePathDisabled(result.Extensions, gamma) {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("works with direct paths", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "direct-pkg")
		one := filepath.Join(pkgDir, "extensions", "one.gi.json")
		two := filepath.Join(pkgDir, "extensions", "two.gi.json")
		writeGiProtocolExtensionDescriptor(t, one)
		writeGiProtocolExtensionDescriptor(t, two)

		result, err := manager.ResolveProtocolPackageSourceSpecs([]ProtocolPackageSourceSpec{{
			Source:  pkgDir,
			Filters: ProtocolPackageResourceFilters{Extensions: []string{"extensions/one.gi.json"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackagePathEnabled(result.Extensions, one) || !protocolPackagePathDisabled(result.Extensions, two) {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("force-include overrides exclude in package filters", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "force-pkg")
		alpha := filepath.Join(pkgDir, "extensions", "alpha.gi.json")
		beta := filepath.Join(pkgDir, "extensions", "beta.gi.json")
		gamma := filepath.Join(pkgDir, "extensions", "gamma.gi.json")
		writeGiProtocolExtensionDescriptor(t, alpha)
		writeGiProtocolExtensionDescriptor(t, beta)
		writeGiProtocolExtensionDescriptor(t, gamma)

		result, err := manager.ResolveProtocolPackageSourceSpecs([]ProtocolPackageSourceSpec{{
			Source:  pkgDir,
			Filters: ProtocolPackageResourceFilters{Extensions: []string{"!**/*.gi.json", "+extensions/beta.gi.json"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackagePathDisabled(result.Extensions, alpha) ||
			!protocolPackagePathEnabled(result.Extensions, beta) ||
			!protocolPackagePathDisabled(result.Extensions, gamma) {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("force-includes multiple resources", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "multi-force-pkg")
		skillA := filepath.Join(pkgDir, "skills", "skill-a", "SKILL.md")
		skillB := filepath.Join(pkgDir, "skills", "skill-b", "SKILL.md")
		skillC := filepath.Join(pkgDir, "skills", "skill-c", "SKILL.md")
		writeResourceSkill(t, skillA, "skill-a", "A", "Content")
		writeResourceSkill(t, skillB, "skill-b", "B", "Content")
		writeResourceSkill(t, skillC, "skill-c", "C", "Content")

		result, err := manager.ResolveProtocolPackageSourceSpecs([]ProtocolPackageSourceSpec{{
			Source:  pkgDir,
			Filters: ProtocolPackageResourceFilters{Skills: []string{"!**/*", "+skills/skill-a", "+skills/skill-c"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackagePathEnabled(result.Skills, skillA) ||
			!protocolPackagePathDisabled(result.Skills, skillB) ||
			!protocolPackagePathEnabled(result.Skills, skillC) {
			t.Fatalf("skills = %#v", result.Skills)
		}
	})

	t.Run("force-includes after a specific exclusion", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "specific-force-pkg")
		a := filepath.Join(pkgDir, "extensions", "a.gi.json")
		b := filepath.Join(pkgDir, "extensions", "b.gi.json")
		writeGiProtocolExtensionDescriptor(t, a)
		writeGiProtocolExtensionDescriptor(t, b)

		result, err := manager.ResolveProtocolPackageSourceSpecs([]ProtocolPackageSourceSpec{{
			Source:  pkgDir,
			Filters: ProtocolPackageResourceFilters{Extensions: []string{"!extensions/b.gi.json", "+extensions/b.gi.json"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackagePathEnabled(result.Extensions, a) || !protocolPackagePathEnabled(result.Extensions, b) {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("force-includes themes", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "theme-force-pkg")
		dark := filepath.Join(pkgDir, "themes", "dark.json")
		light := filepath.Join(pkgDir, "themes", "light.json")
		special := filepath.Join(pkgDir, "themes", "special.json")
		writeResourceFile(t, dark, "{}")
		writeResourceFile(t, light, "{}")
		writeResourceFile(t, special, "{}")

		result, err := manager.ResolveProtocolPackageSourceSpecs([]ProtocolPackageSourceSpec{{
			Source:  pkgDir,
			Filters: ProtocolPackageResourceFilters{Themes: []string{"!themes/*.json", "+themes/special.json"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackagePathDisabled(result.Themes, dark) ||
			!protocolPackagePathDisabled(result.Themes, light) ||
			!protocolPackagePathEnabled(result.Themes, special) {
			t.Fatalf("themes = %#v", result.Themes)
		}
	})

	t.Run("force-includes prompts", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "prompt-force-pkg")
		review := filepath.Join(pkgDir, "prompts", "review.md")
		explain := filepath.Join(pkgDir, "prompts", "explain.md")
		debug := filepath.Join(pkgDir, "prompts", "debug.md")
		writeResourceFile(t, review, "Review")
		writeResourceFile(t, explain, "Explain")
		writeResourceFile(t, debug, "Debug")

		result, err := manager.ResolveProtocolPackageSourceSpecs([]ProtocolPackageSourceSpec{{
			Source:  pkgDir,
			Filters: ProtocolPackageResourceFilters{Prompts: []string{"!prompts/*.md", "+prompts/debug.md"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackagePathDisabled(result.Prompts, review) ||
			!protocolPackagePathDisabled(result.Prompts, explain) ||
			!protocolPackagePathEnabled(result.Prompts, debug) {
			t.Fatalf("prompts = %#v", result.Prompts)
		}
	})

	t.Run("force-excludes in package filters", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "force-exclude-pkg")
		alpha := filepath.Join(pkgDir, "extensions", "alpha.gi.json")
		beta := filepath.Join(pkgDir, "extensions", "beta.gi.json")
		writeGiProtocolExtensionDescriptor(t, alpha)
		writeGiProtocolExtensionDescriptor(t, beta)

		result, err := manager.ResolveProtocolPackageSourceSpecs([]ProtocolPackageSourceSpec{{
			Source:  pkgDir,
			Filters: ProtocolPackageResourceFilters{Extensions: []string{"extensions/*.gi.json", "+extensions/alpha.gi.json", "-extensions/alpha.gi.json"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackagePathDisabled(result.Extensions, alpha) || !protocolPackagePathEnabled(result.Extensions, beta) {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})
}

func TestProtocolPackageConfiguredSourceDedupe(t *testing.T) {
	t.Run("dedupes same local package in global and project with project winning", func(t *testing.T) {
		agentDir, projectDir := createPackageManagerSettingsDirs(t)
		pkgDir := filepath.Join(projectDir, "shared-pkg")
		extensionPath := filepath.Join(pkgDir, "extensions", "shared.gi.json")
		writeGiProtocolExtensionDescriptor(t, extensionPath)
		settings := NewSettingsManager(projectDir, agentDir)
		settings.SetPackages([]any{pkgDir})
		settings.SetProjectPackages([]any{pkgDir})
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: projectDir, AgentDir: agentDir, SettingsManager: settings})

		result, err := manager.ResolveConfiguredProtocolPackageResources()
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Extensions) != 1 || result.Extensions[0].Path != extensionPath || result.Extensions[0].Metadata.Scope != "project" {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("keeps different packages", func(t *testing.T) {
		agentDir, projectDir := createPackageManagerSettingsDirs(t)
		pkg1 := filepath.Join(projectDir, "pkg1")
		pkg2 := filepath.Join(projectDir, "pkg2")
		ext1 := filepath.Join(pkg1, "extensions", "from-pkg1.gi.json")
		ext2 := filepath.Join(pkg2, "extensions", "from-pkg2.gi.json")
		writeGiProtocolExtensionDescriptor(t, ext1)
		writeGiProtocolExtensionDescriptor(t, ext2)
		settings := NewSettingsManager(projectDir, agentDir)
		settings.SetPackages([]any{pkg1})
		settings.SetProjectPackages([]any{pkg2})
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: projectDir, AgentDir: agentDir, SettingsManager: settings})

		result, err := manager.ResolveConfiguredProtocolPackageResources()
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackagePathEnabled(result.Extensions, ext1) || !protocolPackagePathEnabled(result.Extensions, ext2) {
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
	return protocolPackagePathEnabled(resources, path)
}

func protocolPackagePathEnabled(resources []ProtocolPackageResource, path string) bool {
	clean := filepath.Clean(path)
	for _, resource := range resources {
		if filepath.Clean(resource.Path) == clean && resource.Enabled {
			return true
		}
	}
	return false
}

func protocolPackagePathDisabled(resources []ProtocolPackageResource, path string) bool {
	clean := filepath.Clean(path)
	for _, resource := range resources {
		if filepath.Clean(resource.Path) == clean && !resource.Enabled {
			return true
		}
	}
	return false
}

func protocolPackageHasAnyPath(resources []ProtocolPackageResource, path string) bool {
	clean := filepath.Clean(path)
	for _, resource := range resources {
		if filepath.Clean(resource.Path) == clean {
			return true
		}
	}
	return false
}

func protocolPackageHasSuffix(resources []ProtocolPackageResource, suffix string) bool {
	suffix = filepath.Clean(suffix)
	for _, resource := range resources {
		if strings.HasSuffix(filepath.Clean(resource.Path), suffix) && resource.Enabled {
			return true
		}
	}
	return false
}
