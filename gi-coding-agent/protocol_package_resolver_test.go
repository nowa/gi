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
		if len(result.Extensions) != 0 || len(result.ProcessExtensions) != 0 || len(result.Skills) != 0 || len(result.Prompts) != 0 || len(result.Themes) != 0 {
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

	t.Run("resolves process extension entries from Gi manifest", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		pkgDir := filepath.Join(manager.cwd, "process-package")
		extensionPath := filepath.Join(pkgDir, "extensions", "static.gi.json")
		writeGiProtocolExtensionDescriptor(t, extensionPath)
		writeProtocolPackageManifest(t, filepath.Join(pkgDir, "gi.package.json"), map[string]any{
			"extensions": []any{
				"./extensions/static.gi.json",
				map[string]any{
					"id": "todo-widget",
					"entry": map[string]any{
						"kind":      "process",
						"command":   []any{"./bin/todo-widget"},
						"transport": "stdio-ndjson",
						"protocol":  "gi-ext-rpc@1",
					},
					"capabilities": []any{"tui.widget", "session.read"},
					"env":          map[string]any{"GI_WIDGET_MODE": "test"},
				},
			},
		})

		result, err := manager.ResolveProtocolPackageResources([]string{pkgDir})
		if err != nil {
			t.Fatal(err)
		}
		if !protocolPackageHasPath(result.Extensions, extensionPath) {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
		if len(result.ProcessExtensions) != 1 {
			t.Fatalf("process extensions = %#v", result.ProcessExtensions)
		}
		process := result.ProcessExtensions[0]
		if process.ID != "todo-widget" ||
			process.PackageDir != filepath.Clean(pkgDir) ||
			!reflect.DeepEqual(process.Command, []string{"./bin/todo-widget"}) ||
			!reflect.DeepEqual(process.Capabilities, []string{"tui.widget", "session.read"}) ||
			process.Env["GI_WIDGET_MODE"] != "test" ||
			process.Metadata.Source != "local:"+filepath.Clean(pkgDir) ||
			process.Metadata.Origin != "package" {
			t.Fatalf("process extension = %#v", process)
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

func TestOfficialPackageDescriptorsDeclareRegistryCapabilities(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "protocol", "spec", "registries", "official-packages.json"))
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Packages []struct {
			Name                 string   `json:"name"`
			RequiredCapabilities []string `json:"requiredCapabilities"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(content, &registry); err != nil {
		t.Fatal(err)
	}
	requiredByPackage := map[string][]string{}
	for _, pkg := range registry.Packages {
		required := append([]string(nil), pkg.RequiredCapabilities...)
		sort.Strings(required)
		requiredByPackage[pkg.Name] = required
	}

	for _, name := range OfficialPackageNames() {
		definition := officialPackages[name]
		files, err := definition.files()
		if err != nil {
			t.Fatalf("%s files: %v", name, err)
		}
		raw := files[filepath.ToSlash(filepath.Join("extensions", "main.gi.json"))]
		if raw == "" {
			t.Fatalf("%s missing extension descriptor", name)
		}
		var descriptor protocolExtensionDescriptor
		if err := json.Unmarshal([]byte(raw), &descriptor); err != nil {
			t.Fatalf("%s descriptor: %v", name, err)
		}
		if descriptor.Gi == nil {
			t.Fatalf("%s descriptor missing gi section", name)
		}
		got := append([]string(nil), descriptor.Gi.Capabilities...)
		sort.Strings(got)
		want := requiredByPackage[name]
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s descriptor capabilities = %#v, registry = %#v", name, got, want)
		}
		for _, capability := range got {
			if !isSupportedExtensionCapability(capability) {
				t.Fatalf("%s descriptor declares runtime-unsupported capability %q", name, capability)
			}
		}
	}
}

func TestProtocolRegistryMCPAdapterUsesStdioProcessCapability(t *testing.T) {
	capabilityContent, err := os.ReadFile(filepath.Join("..", "protocol", "spec", "registries", "capabilities.json"))
	if err != nil {
		t.Fatal(err)
	}
	var capabilities struct {
		Capabilities []struct {
			Name string `json:"name"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(capabilityContent, &capabilities); err != nil {
		t.Fatal(err)
	}
	var capabilityNames []string
	for _, capability := range capabilities.Capabilities {
		capabilityNames = append(capabilityNames, capability.Name)
	}
	if !containsString(capabilityNames, "process.stdio:<scope>") {
		t.Fatalf("process.stdio capability missing from registry: %#v", capabilityNames)
	}

	packageContent, err := os.ReadFile(filepath.Join("..", "protocol", "spec", "registries", "official-packages.json"))
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Packages []struct {
			Name                 string   `json:"name"`
			RequiredCapabilities []string `json:"requiredCapabilities"`
			RequiredHostActions  []string `json:"requiredHostActions"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(packageContent, &registry); err != nil {
		t.Fatal(err)
	}
	for _, pkg := range registry.Packages {
		if pkg.Name != "gi-mcp-adapter" {
			continue
		}
		if !containsString(pkg.RequiredCapabilities, "process.stdio:<scope>") {
			t.Fatalf("gi-mcp-adapter capabilities = %#v", pkg.RequiredCapabilities)
		}
		if containsString(pkg.RequiredHostActions, "host.process.exec") {
			t.Fatalf("gi-mcp-adapter should not model interactive stdio as host.process.exec: %#v", pkg.RequiredHostActions)
		}
		return
	}
	t.Fatal("gi-mcp-adapter missing from official package registry")
}

func TestProtocolRegistriesReferenceKnownCapabilitiesAndHostActions(t *testing.T) {
	capabilityContent, err := os.ReadFile(filepath.Join("..", "protocol", "spec", "registries", "capabilities.json"))
	if err != nil {
		t.Fatal(err)
	}
	var capabilities struct {
		Capabilities []struct {
			Name string `json:"name"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(capabilityContent, &capabilities); err != nil {
		t.Fatal(err)
	}
	capabilityNames := map[string]bool{}
	for _, capability := range capabilities.Capabilities {
		capabilityNames[capability.Name] = true
	}

	hostActionContent, err := os.ReadFile(filepath.Join("..", "protocol", "spec", "registries", "host-actions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var hostActions struct {
		Actions []struct {
			Name       string `json:"name"`
			Capability string `json:"capability"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(hostActionContent, &hostActions); err != nil {
		t.Fatal(err)
	}
	hostActionNames := map[string]bool{}
	for _, action := range hostActions.Actions {
		hostActionNames[action.Name] = true
		switch {
		case action.Capability == "none",
			strings.HasPrefix(action.Capability, "slot-specific "),
			strings.HasPrefix(action.Capability, "owned "):
		case capabilityNames[action.Capability]:
		default:
			t.Fatalf("host action %s references unknown capability %q", action.Name, action.Capability)
		}
	}

	packageContent, err := os.ReadFile(filepath.Join("..", "protocol", "spec", "registries", "official-packages.json"))
	if err != nil {
		t.Fatal(err)
	}
	var official struct {
		Packages []struct {
			Name                 string   `json:"name"`
			RequiredCapabilities []string `json:"requiredCapabilities"`
			RequiredHostActions  []string `json:"requiredHostActions"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(packageContent, &official); err != nil {
		t.Fatal(err)
	}
	for _, pkg := range official.Packages {
		for _, capability := range pkg.RequiredCapabilities {
			if !capabilityNames[capability] {
				t.Fatalf("%s references unknown capability %q", pkg.Name, capability)
			}
		}
		for _, action := range pkg.RequiredHostActions {
			if !hostActionNames[action] {
				t.Fatalf("%s references unknown host action %q", pkg.Name, action)
			}
		}
	}
}

func TestProtocolCapabilityRegistryIsSupportedByRuntime(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "protocol", "spec", "registries", "capabilities.json"))
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Capabilities []struct {
			Name string `json:"name"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(content, &registry); err != nil {
		t.Fatal(err)
	}
	for _, capability := range registry.Capabilities {
		if !isSupportedExtensionCapability(capability.Name) {
			t.Fatalf("capability registry entry is not supported by runtime: %q", capability.Name)
		}
	}
}

func TestProtocolHostActionSchemaMatchesRegistry(t *testing.T) {
	registryContent, err := os.ReadFile(filepath.Join("..", "protocol", "spec", "registries", "host-actions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Actions []struct {
			Name string `json:"name"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(registryContent, &registry); err != nil {
		t.Fatal(err)
	}
	var registryNames []string
	for _, action := range registry.Actions {
		registryNames = append(registryNames, action.Name)
	}
	sort.Strings(registryNames)

	schemaContent, err := os.ReadFile(filepath.Join("..", "protocol", "spec", "schemas", "host-action.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaContent, &schema); err != nil {
		t.Fatal(err)
	}
	methods := hostActionSchemaMethodEnum(t, schema)
	sort.Strings(methods)
	if !reflect.DeepEqual(methods, registryNames) {
		t.Fatalf("host-action schema methods = %#v, registry = %#v", methods, registryNames)
	}
}

func hostActionSchemaMethodEnum(t *testing.T, schema map[string]any) []string {
	t.Helper()
	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatal("schema missing $defs")
	}
	request, ok := defs["request"].(map[string]any)
	if !ok {
		t.Fatal("schema missing request definition")
	}
	properties, ok := request["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema request missing properties")
	}
	method, ok := properties["method"].(map[string]any)
	if !ok {
		t.Fatal("schema request missing method")
	}
	values, ok := method["enum"].([]any)
	if !ok {
		t.Fatal("schema method missing enum")
	}
	methods := make([]string, 0, len(values))
	for _, value := range values {
		method, ok := value.(string)
		if !ok || method == "" {
			t.Fatalf("invalid method enum value: %#v", value)
		}
		methods = append(methods, method)
	}
	return methods
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

func TestProtocolPackageResourceToggleSettings(t *testing.T) {
	manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
	pkgDir := filepath.Join(manager.cwd, "toggle-pkg")
	alpha := filepath.Join(pkgDir, "extensions", "alpha.gi.json")
	beta := filepath.Join(pkgDir, "extensions", "beta.gi.json")
	writeGiProtocolExtensionDescriptor(t, alpha)
	writeGiProtocolExtensionDescriptor(t, beta)
	manager.settingsManager.SetPackages([]any{pkgDir})

	changed, err := manager.SetPackageResourceEnabled(PackageResourceToggle{
		Source:       pkgDir,
		Scope:        "user",
		ResourceType: "extensions",
		Pattern:      "extensions/beta.gi.json",
		Enabled:      false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}

	result, err := manager.ResolveConfiguredProtocolPackageResources()
	if err != nil {
		t.Fatal(err)
	}
	if !protocolPackagePathEnabled(result.Extensions, alpha) || !protocolPackagePathDisabled(result.Extensions, beta) {
		t.Fatalf("extensions = %#v", result.Extensions)
	}
	packages := manager.settingsManager.GetPackages()
	object, ok := packages[0].(map[string]any)
	if !ok {
		t.Fatalf("package setting = %#v, want object", packages[0])
	}
	if filters := settingsStringSlice(object, "extensions"); len(filters) != 1 || filters[0] != "-extensions/beta.gi.json" {
		t.Fatalf("filters = %#v", object["extensions"])
	}

	changed, err = manager.SetPackageResourceEnabled(PackageResourceToggle{
		Source:       protocolPackageSettingsIdentity(pkgDir, manager.agentDir),
		Scope:        "user",
		ResourceType: "extensions",
		Pattern:      "extensions/beta.gi.json",
		Enabled:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("changed = false, want true for identity source")
	}
	packages = manager.settingsManager.GetPackages()
	object, ok = packages[0].(map[string]any)
	if !ok {
		t.Fatalf("package setting = %#v, want object", packages[0])
	}
	if filters := settingsStringSlice(object, "extensions"); len(filters) != 1 || filters[0] != "+extensions/beta.gi.json" {
		t.Fatalf("filters = %#v", object["extensions"])
	}
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
