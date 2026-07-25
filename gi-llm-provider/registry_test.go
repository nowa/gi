package gillmprovider

import (
	"strings"
	"testing"
)

func TestAPIRegistrySourceLifecyclePiStyle(t *testing.T) {
	const (
		sourceID = "registry-source-test"
		apiOne   = "registry-source-test-one"
		apiTwo   = "registry-source-test-two"
		apiOther = "registry-source-test-other"
	)
	provider := APIProviderFuncs{StreamSimpleFunc: func(model Model, _ Context, _ SimpleStreamOptions) (*AssistantMessageEventStream, error) {
		return CompletedAssistantStream(AssistantMessage([]ContentPart{Text(model.ID)}, StopReasonStop, model)), nil
	}}
	RegisterAPIProviderWithSource(apiOne, provider, sourceID)
	RegisterAPIProviderWithSource(apiTwo, provider, sourceID)
	RegisterAPIProviderWithSource(apiOther, provider, "other-source")
	t.Cleanup(func() {
		UnregisterAPIProvider(apiOne)
		UnregisterAPIProvider(apiTwo)
		UnregisterAPIProvider(apiOther)
	})

	if !apiProvidersIncludeSource(GetAPIProviders(), apiOne, sourceID) || !apiProvidersIncludeSource(GetAPIProviders(), apiTwo, sourceID) {
		t.Fatalf("providers missing source registrations: %#v", GetAPIProviders())
	}

	UnregisterAPIProviders(sourceID)
	if GetAPIProvider(apiOne) != nil || GetAPIProvider(apiTwo) != nil {
		t.Fatalf("source providers were not removed: %#v", GetAPIProviders())
	}
	if GetAPIProvider(apiOther) == nil {
		t.Fatal("unregistering one source removed another source")
	}
}

func TestBuiltInAPIProviderLifecyclePiStyle(t *testing.T) {
	t.Cleanup(ResetAPIProviders)

	ClearAPIProviders()
	if len(GetAPIProviders()) != 0 {
		t.Fatalf("providers after clear = %#v", GetAPIProviders())
	}

	RegisterBuiltInAPIProviders()
	for _, api := range []string{
		"anthropic-messages",
		"openai-completions",
		"openai-responses",
		"openai-codex-responses",
		"azure-openai-responses",
		"google-generative-ai",
		"google-vertex",
		"mistral-conversations",
		"bedrock-converse-stream",
		"pi-messages",
	} {
		if !apiProvidersIncludeSource(GetAPIProviders(), api, BuiltInAPIProviderSourceID) {
			t.Fatalf("built-in provider %q missing after register: %#v", api, GetAPIProviders())
		}
	}

	RegisterAPIProvider("temporary-custom-api", APIProviderFuncs{})
	if GetAPIProvider("temporary-custom-api") == nil {
		t.Fatal("custom provider missing before reset")
	}
	ResetAPIProviders()
	if GetAPIProvider("temporary-custom-api") != nil {
		t.Fatalf("reset should remove non-built-in provider: %#v", GetAPIProviders())
	}
	if !apiProvidersIncludeSource(GetAPIProviders(), "openai-responses", BuiltInAPIProviderSourceID) {
		t.Fatalf("reset should restore built-ins: %#v", GetAPIProviders())
	}
}

func TestAPIRegistryProviderChecksModelAPIPiStyle(t *testing.T) {
	const api = "registry-mismatch-test"
	called := false
	RegisterAPIProvider(api, APIProviderFuncs{StreamSimpleFunc: func(Model, Context, SimpleStreamOptions) (*AssistantMessageEventStream, error) {
		called = true
		return ErrorAssistantStream(AssistantErrorMessage("should not be called", Model{API: api}, false)), nil
	}})
	t.Cleanup(func() { UnregisterAPIProvider(api) })

	provider := GetAPIProvider(api)
	if provider == nil {
		t.Fatal("registered provider missing")
	}
	_, err := provider.StreamSimple(Model{API: "different-api"}, Context{}, SimpleStreamOptions{})
	if err == nil || !strings.Contains(err.Error(), "mismatched api") {
		t.Fatalf("mismatch error = %v", err)
	}
	if called {
		t.Fatal("wrapped provider was called despite mismatched api")
	}
}

func apiProvidersIncludeSource(providers []APIProviderRegistration, api, sourceID string) bool {
	for _, provider := range providers {
		if provider.API == api && provider.SourceID == sourceID {
			return true
		}
	}
	return false
}
