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

func TestProtocolExtensionDiscoveryPathRules(t *testing.T) {
	t.Run("discovers direct Gi protocol descriptors in extensions", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		writeGiProtocolExtensionDescriptor(t, filepath.Join(env.extensionsDir, "foo.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(env.extensionsDir, "bar.gi.json"))
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Errors) != 0 {
			t.Fatalf("errors = %#v", result.Errors)
		}
		if got := baseNames(result.Extensions); !reflect.DeepEqual(got, []string{"bar.gi.json", "foo.gi.json"}) {
			t.Fatalf("extensions = %#v", got)
		}
	})

	t.Run("discovers subdirectory with index descriptor", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		subdir := filepath.Join(env.extensionsDir, "my-extension")
		writeGiProtocolExtensionDescriptor(t, filepath.Join(subdir, "index.gi.json"))
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 1 || !strings.Contains(result.Extensions[0].Path, "my-extension") || filepath.Base(result.Extensions[0].Path) != "index.gi.json" {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("discovers subdirectory with package.json gi field", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		subdir := filepath.Join(env.extensionsDir, "my-package")
		writeGiProtocolExtensionDescriptor(t, filepath.Join(subdir, "src", "main.gi.json"))
		writeGiProtocolPackageJSON(t, subdir, []string{"./src/main.gi.json"})
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 1 || !strings.Contains(result.Extensions[0].Path, filepath.Join("src", "main.gi.json")) {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("package.json can declare multiple extensions", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		subdir := filepath.Join(env.extensionsDir, "my-package")
		writeGiProtocolExtensionDescriptor(t, filepath.Join(subdir, "ext1.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(subdir, "ext2.gi.json"))
		writeGiProtocolPackageJSON(t, subdir, []string{"./ext1.gi.json", "./ext2.gi.json"})
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if got := baseNames(result.Extensions); !reflect.DeepEqual(got, []string{"ext1.gi.json", "ext2.gi.json"}) {
			t.Fatalf("extensions = %#v", got)
		}
	})

	t.Run("package.json with gi field takes precedence over index descriptor", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		subdir := filepath.Join(env.extensionsDir, "my-package")
		writeGiProtocolExtensionDescriptor(t, filepath.Join(subdir, "index.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(subdir, "custom.gi.json"))
		writeGiProtocolPackageJSON(t, subdir, []string{"./custom.gi.json"})
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 1 || filepath.Base(result.Extensions[0].Path) != "custom.gi.json" {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("ignores package.json without gi field and falls back to index descriptor", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		subdir := filepath.Join(env.extensionsDir, "my-package")
		writeGiProtocolExtensionDescriptor(t, filepath.Join(subdir, "index.gi.json"))
		writeJSON(t, filepath.Join(subdir, "package.json"), map[string]any{"name": "my-package", "version": "1.0.0"})
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 1 || filepath.Base(result.Extensions[0].Path) != "index.gi.json" {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("ignores subdirectory without index or package.json", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		subdir := filepath.Join(env.extensionsDir, "not-an-extension")
		writeGiProtocolExtensionDescriptor(t, filepath.Join(subdir, "helper.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(subdir, "utils.gi.json"))
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 0 {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("does not recurse beyond one level", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		writeGiProtocolExtensionDescriptor(t, filepath.Join(env.extensionsDir, "container", "nested", "index.gi.json"))
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 0 {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("handles mixed direct files and subdirectories", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		writeGiProtocolExtensionDescriptor(t, filepath.Join(env.extensionsDir, "direct.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(env.extensionsDir, "with-index", "index.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(env.extensionsDir, "with-manifest", "entry.gi.json"))
		writeGiProtocolPackageJSON(t, filepath.Join(env.extensionsDir, "with-manifest"), []string{"./entry.gi.json"})
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Errors) != 0 || len(result.Extensions) != 3 {
			t.Fatalf("extensions = %#v errors=%#v", result.Extensions, result.Errors)
		}
	})

	t.Run("skips non-existent paths declared in package.json", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		subdir := filepath.Join(env.extensionsDir, "my-package")
		writeGiProtocolExtensionDescriptor(t, filepath.Join(subdir, "exists.gi.json"))
		writeGiProtocolPackageJSON(t, subdir, []string{"./exists.gi.json", "./missing.gi.json"})
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 1 || filepath.Base(result.Extensions[0].Path) != "exists.gi.json" {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("handles explicitly configured paths", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		customPath := filepath.Join(env.tempDir, "custom-location", "my-ext.gi.json")
		writeGiProtocolExtensionDescriptor(t, customPath)
		result := DiscoverProtocolExtensions([]string{customPath}, env.cwd, env.agentDir)
		if len(result.Extensions) != 1 || filepath.Base(result.Extensions[0].Path) != "my-ext.gi.json" {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("loadExtensions only loads explicit paths without discovery", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		writeGiProtocolExtensionDescriptor(t, filepath.Join(env.extensionsDir, "discovered.gi.json"))
		explicitPath := filepath.Join(env.tempDir, "explicit.gi.json")
		writeGiProtocolExtensionDescriptor(t, explicitPath)
		result := LoadProtocolExtensionSources([]string{explicitPath}, env.cwd)
		if len(result.Extensions) != 1 || filepath.Base(result.Extensions[0].Path) != "explicit.gi.json" {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("loadExtensions with no paths loads nothing", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		writeGiProtocolExtensionDescriptor(t, filepath.Join(env.extensionsDir, "discovered.gi.json"))
		result := LoadProtocolExtensionSources(nil, env.cwd)
		if len(result.Extensions) != 0 || len(result.Errors) != 0 {
			t.Fatalf("result = %#v", result)
		}
	})
}

func TestProtocolPackageExtensionEntrypointDiscovery(t *testing.T) {
	t.Run("only loads protocol entrypoints from subdirectories, not helper descriptors", func(t *testing.T) {
		pkgDir := filepath.Join(t.TempDir(), "multifile-pkg")
		extensionsDir := filepath.Join(pkgDir, "extensions")
		writeGiProtocolExtensionDescriptor(t, filepath.Join(extensionsDir, "subagent", "index.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(extensionsDir, "subagent", "helpers.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(extensionsDir, "standalone.gi.json"))

		result := discoverProtocolExtensionsInDir(extensionsDir)
		if !protocolExtensionHasSuffix(result.Extensions, filepath.Join("subagent", "index.gi.json")) ||
			!protocolExtensionHasSuffix(result.Extensions, "standalone.gi.json") ||
			protocolExtensionHasSuffix(result.Extensions, "helpers.gi.json") {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("respects package.json gi.extensions manifest in subdirectories", func(t *testing.T) {
		pkgDir := filepath.Join(t.TempDir(), "manifest-subdir-pkg")
		customDir := filepath.Join(pkgDir, "extensions", "custom")
		writeGiProtocolExtensionDescriptor(t, filepath.Join(customDir, "main.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(customDir, "utils.gi.json"))
		writeGiProtocolPackageJSON(t, customDir, []string{"./main.gi.json"})

		result := discoverProtocolExtensionsInDir(filepath.Join(pkgDir, "extensions"))
		if !protocolExtensionHasSuffix(result.Extensions, filepath.Join("custom", "main.gi.json")) ||
			protocolExtensionHasSuffix(result.Extensions, "utils.gi.json") {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("handles mixed top-level files and subdirectories", func(t *testing.T) {
		pkgDir := filepath.Join(t.TempDir(), "mixed-pkg")
		extensionsDir := filepath.Join(pkgDir, "extensions")
		writeGiProtocolExtensionDescriptor(t, filepath.Join(extensionsDir, "simple.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(extensionsDir, "complex", "index.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(extensionsDir, "complex", "a.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(extensionsDir, "complex", "b.gi.json"))

		result := discoverProtocolExtensionsInDir(extensionsDir)
		if len(result.Extensions) != 2 ||
			!protocolExtensionHasSuffix(result.Extensions, "simple.gi.json") ||
			!protocolExtensionHasSuffix(result.Extensions, filepath.Join("complex", "index.gi.json")) ||
			protocolExtensionHasSuffix(result.Extensions, filepath.Join("complex", "a.gi.json")) ||
			protocolExtensionHasSuffix(result.Extensions, filepath.Join("complex", "b.gi.json")) {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("skips subdirectories without index or manifest", func(t *testing.T) {
		pkgDir := filepath.Join(t.TempDir(), "no-entry-pkg")
		extensionsDir := filepath.Join(pkgDir, "extensions")
		writeGiProtocolExtensionDescriptor(t, filepath.Join(extensionsDir, "broken", "helper.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(extensionsDir, "broken", "another.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(extensionsDir, "valid.gi.json"))

		result := discoverProtocolExtensionsInDir(extensionsDir)
		if len(result.Extensions) != 1 || !protocolExtensionHasSuffix(result.Extensions, "valid.gi.json") {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})
}

type protocolExtensionDiscoveryEnv struct {
	tempDir       string
	cwd           string
	agentDir      string
	extensionsDir string
}

func newProtocolExtensionDiscoveryEnv(t *testing.T) protocolExtensionDiscoveryEnv {
	t.Helper()
	tempDir := t.TempDir()
	return protocolExtensionDiscoveryEnv{
		tempDir:       tempDir,
		cwd:           tempDir,
		agentDir:      tempDir,
		extensionsDir: filepath.Join(tempDir, "extensions"),
	}
}

func writeGiProtocolPackageJSON(t *testing.T, dir string, extensions []string) {
	t.Helper()
	writeJSON(t, filepath.Join(dir, "package.json"), map[string]any{"gi": map[string]any{"extensions": extensions}})
}

func writeGiProtocolExtensionDescriptor(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte(`{"gi":{"extensionProtocol":"jsonl-rpc.v1"}}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func baseNames(sources []ProtocolExtensionSource) []string {
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, filepath.Base(source.Path))
	}
	sort.Strings(names)
	return names
}

func protocolExtensionHasSuffix(sources []ProtocolExtensionSource, suffix string) bool {
	cleanSuffix := filepath.Clean(suffix)
	for _, source := range sources {
		if strings.HasSuffix(filepath.Clean(source.Path), cleanSuffix) {
			return true
		}
	}
	return false
}
