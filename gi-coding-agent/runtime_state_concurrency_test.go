package gicodingagent

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestProtocolExtensionRuntimePublishesConcurrentRegistrySnapshots(t *testing.T) {
	runtime := NewDefaultProtocolExtensionRuntime()
	extension := &ProtocolExtensionContext{
		runtime: runtime,
		source:  ProtocolSourceInfo{Path: "concurrent.gi.json"},
	}

	const registrations = 64
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range registrations {
			name := fmt.Sprintf("command-%d", i)
			if err := extension.RegisterCommand(name, ProtocolCommandDefinition{}); err != nil {
				t.Errorf("RegisterCommand(%q): %v", name, err)
				return
			}
			if err := extension.RegisterShortcut(fmt.Sprintf("ctrl+%d", i), ProtocolShortcutDefinition{}); err != nil {
				t.Errorf("RegisterShortcut(%d): %v", i, err)
				return
			}
			if err := extension.RegisterAutocompleteProvider(name, ProtocolAutocompleteProviderDefinition{
				Priority: i,
				Handler: func(context.Context, ProtocolAutocompleteRequest) (ProtocolAutocompleteResult, error) {
					return ProtocolAutocompleteResult{}, nil
				},
			}); err != nil {
				t.Errorf("RegisterAutocompleteProvider(%q): %v", name, err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range registrations {
			_ = runtime.RegisteredCommands()
			_ = runtime.Shortcuts(DefaultProtocolKeybindings())
			_ = runtime.AutocompleteProviders()
		}
	}()
	wg.Wait()

	if got := len(runtime.RegisteredCommands()); got != registrations {
		t.Fatalf("registered commands = %d, want %d", got, registrations)
	}
	if got := len(runtime.Shortcuts(DefaultProtocolKeybindings()).Shortcuts); got != registrations {
		t.Fatalf("registered shortcuts = %d, want %d", got, registrations)
	}
	if got := len(runtime.AutocompleteProviders()); got != registrations {
		t.Fatalf("autocomplete providers = %d, want %d", got, registrations)
	}
}

func TestSettingsManagerPublishesConcurrentMergedSnapshots(t *testing.T) {
	settings := NewInMemorySettingsManager(nil)

	const iterations = 256
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range iterations {
			settings.SetEnableSkillCommands(i%2 == 0)
			settings.SetLastChangelogVersion(fmt.Sprintf("v%d", i))
		}
	}()
	go func() {
		defer wg.Done()
		for range iterations {
			_ = settings.GetEnableSkillCommands()
			_ = settings.GetLastChangelogVersion()
			_ = settings.GetGlobalSettings()
		}
	}()
	wg.Wait()

	if got := settings.GetLastChangelogVersion(); got != fmt.Sprintf("v%d", iterations-1) {
		t.Fatalf("last changelog version = %q", got)
	}
}

func TestModelRegistrySerializesConcurrentProviderRegistrationAndLookup(t *testing.T) {
	registry := NewInMemoryModelRegistry(nil)

	const registrations = 32
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range registrations {
			provider := fmt.Sprintf("provider-%d", i)
			err := registry.RegisterProvider(provider, ProviderConfigInput{
				BaseURL: "https://example.invalid",
				APIKey:  "test-key",
				API:     "openai-completions",
				Models: []ProviderModelDefinition{{
					ID:            "model",
					ContextWindow: 4096,
					MaxTokens:     1024,
				}},
			})
			if err != nil {
				t.Errorf("RegisterProvider(%q): %v", provider, err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := range registrations {
			_ = registry.GetAll()
			_, _ = registry.Find(fmt.Sprintf("provider-%d", i), "model")
		}
	}()
	wg.Wait()

	for i := range registrations {
		provider := fmt.Sprintf("provider-%d", i)
		model, ok := registry.Find(provider, "model")
		if !ok {
			t.Fatalf("model %s/model was not registered", provider)
		}
		if model.Provider != provider || model.ID != "model" {
			t.Fatalf("model = %#v", model)
		}
	}

	if got := len(registry.GetAll()); got < registrations {
		t.Fatalf("registered model count = %d, want at least %d", got, registrations)
	}
}
