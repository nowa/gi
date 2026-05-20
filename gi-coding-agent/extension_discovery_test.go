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

func TestProtocolExtensionDiscoveryPiPathParity(t *testing.T) {
	t.Run("discovers direct .ts files in extensions", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		writeProtocolExtensionFile(t, filepath.Join(env.extensionsDir, "foo.ts"))
		writeProtocolExtensionFile(t, filepath.Join(env.extensionsDir, "bar.ts"))
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Errors) != 0 {
			t.Fatalf("errors = %#v", result.Errors)
		}
		if got := baseNames(result.Extensions); !reflect.DeepEqual(got, []string{"bar.ts", "foo.ts"}) {
			t.Fatalf("extensions = %#v", got)
		}
	})

	t.Run("discovers direct .js files in extensions", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		writeProtocolExtensionFile(t, filepath.Join(env.extensionsDir, "foo.js"))
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 1 || filepath.Base(result.Extensions[0].Path) != "foo.js" {
			t.Fatalf("extensions = %#v errors=%#v", result.Extensions, result.Errors)
		}
	})

	t.Run("discovers subdirectory with index.ts", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		subdir := filepath.Join(env.extensionsDir, "my-extension")
		writeProtocolExtensionFile(t, filepath.Join(subdir, "index.ts"))
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 1 || !strings.Contains(result.Extensions[0].Path, "my-extension") || !strings.Contains(result.Extensions[0].Path, "index.ts") {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("discovers subdirectory with index.js", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		writeProtocolExtensionFile(t, filepath.Join(env.extensionsDir, "my-extension", "index.js"))
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 1 || !strings.Contains(result.Extensions[0].Path, "index.js") {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("prefers index.ts over index.js", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		subdir := filepath.Join(env.extensionsDir, "my-extension")
		writeProtocolExtensionFile(t, filepath.Join(subdir, "index.ts"))
		writeProtocolExtensionFile(t, filepath.Join(subdir, "index.js"))
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 1 || filepath.Base(result.Extensions[0].Path) != "index.ts" {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("discovers subdirectory with package.json pi field", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		subdir := filepath.Join(env.extensionsDir, "my-package")
		writeProtocolExtensionFile(t, filepath.Join(subdir, "src", "main.ts"))
		writeProtocolPackageJSON(t, subdir, []string{"./src/main.ts"})
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 1 || !strings.Contains(result.Extensions[0].Path, filepath.Join("src", "main.ts")) {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("package.json can declare multiple extensions", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		subdir := filepath.Join(env.extensionsDir, "my-package")
		writeProtocolExtensionFile(t, filepath.Join(subdir, "ext1.ts"))
		writeProtocolExtensionFile(t, filepath.Join(subdir, "ext2.ts"))
		writeProtocolPackageJSON(t, subdir, []string{"./ext1.ts", "./ext2.ts"})
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if got := baseNames(result.Extensions); !reflect.DeepEqual(got, []string{"ext1.ts", "ext2.ts"}) {
			t.Fatalf("extensions = %#v", got)
		}
	})

	t.Run("package.json with pi field takes precedence over index.ts", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		subdir := filepath.Join(env.extensionsDir, "my-package")
		writeProtocolExtensionFile(t, filepath.Join(subdir, "index.ts"))
		writeProtocolExtensionFile(t, filepath.Join(subdir, "custom.ts"))
		writeProtocolPackageJSON(t, subdir, []string{"./custom.ts"})
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 1 || filepath.Base(result.Extensions[0].Path) != "custom.ts" {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("ignores package.json without pi field and falls back to index.ts", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		subdir := filepath.Join(env.extensionsDir, "my-package")
		writeProtocolExtensionFile(t, filepath.Join(subdir, "index.ts"))
		writeJSON(t, filepath.Join(subdir, "package.json"), map[string]any{"name": "my-package", "version": "1.0.0"})
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 1 || filepath.Base(result.Extensions[0].Path) != "index.ts" {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("ignores subdirectory without index or package.json", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		subdir := filepath.Join(env.extensionsDir, "not-an-extension")
		writeProtocolExtensionFile(t, filepath.Join(subdir, "helper.ts"))
		writeProtocolExtensionFile(t, filepath.Join(subdir, "utils.ts"))
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 0 {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("does not recurse beyond one level", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		writeProtocolExtensionFile(t, filepath.Join(env.extensionsDir, "container", "nested", "index.ts"))
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 0 {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("handles mixed direct files and subdirectories", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		writeProtocolExtensionFile(t, filepath.Join(env.extensionsDir, "direct.ts"))
		writeProtocolExtensionFile(t, filepath.Join(env.extensionsDir, "with-index", "index.ts"))
		writeProtocolExtensionFile(t, filepath.Join(env.extensionsDir, "with-manifest", "entry.ts"))
		writeProtocolPackageJSON(t, filepath.Join(env.extensionsDir, "with-manifest"), []string{"./entry.ts"})
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Errors) != 0 || len(result.Extensions) != 3 {
			t.Fatalf("extensions = %#v errors=%#v", result.Extensions, result.Errors)
		}
	})

	t.Run("skips non-existent paths declared in package.json", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		subdir := filepath.Join(env.extensionsDir, "my-package")
		writeProtocolExtensionFile(t, filepath.Join(subdir, "exists.ts"))
		writeProtocolPackageJSON(t, subdir, []string{"./exists.ts", "./missing.ts"})
		result := DiscoverProtocolExtensions(nil, env.cwd, env.agentDir)
		if len(result.Extensions) != 1 || filepath.Base(result.Extensions[0].Path) != "exists.ts" {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("handles explicitly configured paths", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		customPath := filepath.Join(env.tempDir, "custom-location", "my-ext.ts")
		writeProtocolExtensionFile(t, customPath)
		result := DiscoverProtocolExtensions([]string{customPath}, env.cwd, env.agentDir)
		if len(result.Extensions) != 1 || filepath.Base(result.Extensions[0].Path) != "my-ext.ts" {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("loadExtensions only loads explicit paths without discovery", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		writeProtocolExtensionFile(t, filepath.Join(env.extensionsDir, "discovered.ts"))
		explicitPath := filepath.Join(env.tempDir, "explicit.ts")
		writeProtocolExtensionFile(t, explicitPath)
		result := LoadProtocolExtensionSources([]string{explicitPath}, env.cwd)
		if len(result.Extensions) != 1 || filepath.Base(result.Extensions[0].Path) != "explicit.ts" {
			t.Fatalf("extensions = %#v", result.Extensions)
		}
	})

	t.Run("loadExtensions with no paths loads nothing", func(t *testing.T) {
		env := newProtocolExtensionDiscoveryEnv(t)
		writeProtocolExtensionFile(t, filepath.Join(env.extensionsDir, "discovered.ts"))
		result := LoadProtocolExtensionSources(nil, env.cwd)
		if len(result.Extensions) != 0 || len(result.Errors) != 0 {
			t.Fatalf("result = %#v", result)
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

func writeProtocolExtensionFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("export default function(pi) {}"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeProtocolPackageJSON(t *testing.T, dir string, extensions []string) {
	t.Helper()
	writeJSON(t, filepath.Join(dir, "package.json"), map[string]any{"pi": map[string]any{"extensions": extensions}})
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
