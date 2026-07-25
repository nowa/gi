package gicodingagent

import (
	"errors"
	"reflect"
	"testing"
)

func TestInlineExtensionNamingPiRegression(t *testing.T) {
	noop := func(*ProtocolExtensionContext) error { return nil }

	t.Run("displays bare factories as <inline:N>", func(t *testing.T) {
		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{
			CWD:      t.TempDir(),
			AgentDir: t.TempDir(),
			ExtensionFactories: []ProtocolExtensionFactory{
				{Factory: noop},
				{Factory: noop},
			},
		})
		loader.Reload()

		extensions := loader.GetExtensions().Extensions
		if got := protocolExtensionSourcePaths(extensions); !reflect.DeepEqual(
			got,
			[]string{"<inline:1>", "<inline:2>"},
		) {
			t.Fatalf("extension paths = %#v", got)
		}
		for _, extension := range extensions {
			if extension.Metadata.Path != extension.Path ||
				extension.Metadata.Source != "inline" ||
				extension.Metadata.Scope != "temporary" ||
				extension.Metadata.Origin != "top-level" {
				t.Fatalf("extension = %#v", extension)
			}
		}
	})

	t.Run("displays named wrappers as <inline:name>", func(t *testing.T) {
		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{
			CWD:      t.TempDir(),
			AgentDir: t.TempDir(),
			ExtensionFactories: []ProtocolExtensionFactory{
				{Name: "my-provider", Factory: noop},
				{Name: "my-commands", Factory: noop},
			},
		})
		loader.Reload()

		if got := protocolExtensionSourcePaths(
			loader.GetExtensions().Extensions,
		); !reflect.DeepEqual(
			got,
			[]string{
				"<inline:my-provider>",
				"<inline:my-commands>",
			},
		) {
			t.Fatalf("extension paths = %#v", got)
		}
	})

	t.Run("preserves hidden state for named factories", func(t *testing.T) {
		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{
			CWD:      t.TempDir(),
			AgentDir: t.TempDir(),
			ExtensionFactories: []ProtocolExtensionFactory{{
				Name:    "built-in",
				Hidden:  true,
				Factory: noop,
			}},
		})
		loader.Reload()

		extensions := loader.GetExtensions().Extensions
		if len(extensions) != 1 ||
			extensions[0].Path != "<inline:built-in>" ||
			!extensions[0].Hidden {
			t.Fatalf("extensions = %#v", extensions)
		}
		if visible := interactiveExtensionsFromProtocolSources(
			extensions,
		); len(visible) != 0 {
			t.Fatalf("visible extensions = %#v", visible)
		}
	})

	t.Run("supports mixed bare and named factories", func(t *testing.T) {
		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{
			CWD:      t.TempDir(),
			AgentDir: t.TempDir(),
			ExtensionFactories: []ProtocolExtensionFactory{
				{Factory: noop},
				{Name: "named-ext", Factory: noop},
				{Factory: noop},
			},
		})
		loader.Reload()

		if got := protocolExtensionSourcePaths(
			loader.GetExtensions().Extensions,
		); !reflect.DeepEqual(
			got,
			[]string{
				"<inline:1>",
				"<inline:named-ext>",
				"<inline:3>",
			},
		) {
			t.Fatalf("extension paths = %#v", got)
		}
	})

	t.Run("reports generated names for factory errors", func(t *testing.T) {
		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{
			CWD:      t.TempDir(),
			AgentDir: t.TempDir(),
			ExtensionFactories: []ProtocolExtensionFactory{{
				Factory: func(*ProtocolExtensionContext) error {
					return errors.New("factory failed")
				},
			}},
		})
		loader.Reload()

		result := loader.GetExtensions()
		if len(result.Extensions) != 0 ||
			len(result.Errors) != 1 ||
			result.Errors[0].Path != "<inline:1>" ||
			result.Errors[0].Error != "factory failed" {
			t.Fatalf("result = %#v", result)
		}
	})
}

func protocolExtensionSourcePaths(
	sources []ProtocolExtensionSource,
) []string {
	paths := make([]string, 0, len(sources))
	for _, source := range sources {
		paths = append(paths, source.Path)
	}
	return paths
}
