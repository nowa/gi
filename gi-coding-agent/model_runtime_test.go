package gicodingagent

import (
	"context"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestModelRuntimeOwnsProviderRequestAssemblyPiStyle(
	t *testing.T,
) {
	registry := NewInMemoryModelRegistry(
		NewInMemoryAuthStorage(nil),
	)
	var (
		capturedModel   llm.Model
		capturedOptions llm.SimpleStreamOptions
		transformCalls  atomic.Int32
	)
	if err := registry.RegisterProvider(
		"runtime-provider",
		ProviderConfigInput{
			Name:       "Runtime Provider",
			BaseURL:    "https://example.test/v1",
			APIKey:     "generated-key",
			API:        "runtime-provider-api",
			Headers:    map[string]string{"X-Provider": "provider"},
			AuthHeader: modelRuntimeBoolPointer(true),
			StreamSimple: func(
				model llm.Model,
				_ llm.Context,
				options llm.SimpleStreamOptions,
			) (*llm.AssistantMessageEventStream, error) {
				capturedModel = model
				capturedOptions = options
				return llm.CompletedAssistantStream(
					llm.AssistantMessage(
						[]llm.ContentPart{llm.Text("ok")},
						llm.StopReasonStop,
						model,
					),
				), nil
			},
			Models: []ProviderModelDefinition{{
				ID:            "runtime-model",
				Name:          "Runtime Model",
				ContextWindow: 1000,
				MaxTokens:     100,
				Headers:       map[string]string{"X-Model": "model"},
			}},
		},
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		registry.UnregisterProvider("runtime-provider")
	})

	runtime, err := NewModelRuntimeFromRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := runtime.GetModel(
		"runtime-provider",
		"runtime-model",
	)
	if !ok {
		t.Fatal("runtime model was not composed")
	}
	message, err := runtime.CompleteSimple(
		context.Background(),
		model,
		llm.Context{
			Messages: []llm.Message{llm.UserMessageText("hello")},
		},
		llm.ModelsStreamOptions{
			StreamOptions: llm.StreamOptions{
				Headers: map[string]string{
					"authorization": "Explicit token",
					"X-Request":     "request",
				},
			},
			TransformHeaders: func(
				_ context.Context,
				headers map[string]string,
			) (map[string]string, error) {
				transformCalls.Add(1)
				if headers["authorization"] != "Explicit token" ||
					headers["X-Provider"] != "provider" ||
					headers["X-Model"] != "model" ||
					headers["X-Request"] != "request" {
					t.Fatalf(
						"assembled headers = %#v",
						headers,
					)
				}
				headers["X-Transformed"] = "yes"
				return headers, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if message.Content[0].Text != "ok" {
		t.Fatalf("message = %#v", message)
	}
	if transformCalls.Load() != 1 {
		t.Fatalf(
			"transform calls = %d, want 1",
			transformCalls.Load(),
		)
	}
	if capturedModel.BaseURL != "https://example.test/v1" {
		t.Fatalf("request model = %#v", capturedModel)
	}
	wantHeaders := map[string]string{
		"authorization": "Explicit token",
		"X-Provider":    "provider",
		"X-Model":       "model",
		"X-Request":     "request",
		"X-Transformed": "yes",
	}
	if !reflect.DeepEqual(capturedOptions.Headers, wantHeaders) {
		t.Fatalf(
			"provider headers = %#v, want %#v",
			capturedOptions.Headers,
			wantHeaders,
		)
	}
	if capturedOptions.APIKey != "generated-key" {
		t.Fatalf(
			"provider API key = %q, want generated-key",
			capturedOptions.APIKey,
		)
	}
}

func TestModelRuntimeUsesProviderOwnedAuthProjectionPiStyle(
	t *testing.T,
) {
	previous := llm.GetAPIProvider("openai-completions")
	var (
		capturedModel   llm.Model
		capturedOptions llm.SimpleStreamOptions
	)
	llm.RegisterAPIProvider(
		"openai-completions",
		llm.APIProviderFuncs{
			StreamSimpleFunc: func(
				model llm.Model,
				_ llm.Context,
				options llm.SimpleStreamOptions,
			) (*llm.AssistantMessageEventStream, error) {
				capturedModel = model
				capturedOptions = options
				return llm.CompletedAssistantStream(
					llm.AssistantMessage(
						[]llm.ContentPart{llm.Text("ok")},
						llm.StopReasonStop,
						model,
					),
				), nil
			},
		},
	)
	t.Cleanup(func() {
		if previous != nil {
			llm.RegisterAPIProvider(
				"openai-completions",
				previous,
			)
			return
		}
		llm.UnregisterAPIProvider("openai-completions")
	})
	authStorage := NewInMemoryAuthStorage(
		AuthStorageData{
			"cloudflare-ai-gateway": {
				Type: llm.CredentialTypeAPIKey,
				Key:  "test-token",
				Env: llm.ProviderEnv{
					"CLOUDFLARE_ACCOUNT_ID": "test-account",
					"CLOUDFLARE_GATEWAY_ID": "test-gateway",
				},
			},
		},
	)
	registry := NewInMemoryModelRegistry(authStorage)
	if err := registry.RegisterProvider(
		"cloudflare-ai-gateway",
		ProviderConfigInput{
			Headers: map[string]string{
				"authorization": "Explicit token",
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewModelRuntimeFromRegistry(
		registry,
	)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := runtime.GetModel(
		"cloudflare-ai-gateway",
		"workers-ai/@cf/moonshotai/kimi-k2.5",
	)
	if !ok {
		t.Fatal("Cloudflare compatibility model not found")
	}
	if _, err := runtime.CompleteSimple(
		context.Background(),
		model,
		llm.Context{},
		llm.ModelsStreamOptions{},
	); err != nil {
		t.Fatal(err)
	}
	if capturedModel.BaseURL !=
		"https://gateway.ai.cloudflare.com/v1/test-account/test-gateway/compat" {
		t.Fatalf(
			"resolved base URL = %q",
			capturedModel.BaseURL,
		)
	}
	if capturedOptions.Headers["cf-aig-authorization"] !=
		"Bearer test-token" {
		t.Fatalf(
			"resolved headers = %#v",
			capturedOptions.Headers,
		)
	}
	if capturedOptions.Headers["authorization"] != "Explicit token" {
		t.Fatalf(
			"configured authorization = %#v",
			capturedOptions.Headers,
		)
	}
	for _, removal := range capturedOptions.HeaderRemovals {
		if strings.EqualFold(removal, "authorization") {
			t.Fatalf(
				"configured authorization remained removed: %#v",
				capturedOptions.HeaderRemovals,
			)
		}
	}
	if capturedOptions.Env["CLOUDFLARE_ACCOUNT_ID"] !=
		"test-account" {
		t.Fatalf("resolved env = %#v", capturedOptions.Env)
	}
}

func TestModelRuntimeNativeProviderLifecyclePiStyle(
	t *testing.T,
) {
	authStorage := NewInMemoryAuthStorage(nil)
	runtime, err := NewModelRuntimeFromRegistry(
		NewInMemoryModelRegistry(authStorage),
	)
	if err != nil {
		t.Fatal(err)
	}
	model := llm.Model{
		ID:            "native-model",
		Name:          "Native Model",
		API:           "native-api",
		Provider:      "native-provider",
		BaseURL:       "https://fallback.test/v1",
		Input:         []string{"text"},
		ContextWindow: 1000,
		MaxTokens:     100,
	}
	native, err := llm.CreateProvider(
		llm.CreateProviderOptions{
			ID:      "native-provider",
			Name:    "Native Provider",
			BaseURL: model.BaseURL,
			Models:  []llm.Model{model},
			Auth: llm.ProviderAuth{
				APIKey: &llm.APIKeyAuth{
					Name: "Native setup",
					Login: func(
						ctx context.Context,
						interaction llm.AuthInteraction,
					) (llm.Credential, error) {
						key, err := interaction.Prompt(
							ctx,
							llm.AuthPrompt{
								Type:    llm.AuthPromptSecret,
								Message: "Native API key",
							},
						)
						return llm.Credential{
							Type: llm.CredentialTypeAPIKey,
							Key:  key,
						}, err
					},
					Check: func(
						_ context.Context,
						input llm.APIKeyCheckInput,
					) (*llm.AuthCheck, error) {
						if input.Credential == nil ||
							input.Credential.Key == "" {
							return nil, nil
						}
						return &llm.AuthCheck{
							Type:   llm.CredentialTypeAPIKey,
							Source: "stored native key",
						}, nil
					},
					Resolve: func(
						_ context.Context,
						input llm.APIKeyResolveInput,
					) (*llm.AuthResult, error) {
						if input.Credential == nil ||
							input.Credential.Key == "" {
							return nil, nil
						}
						return &llm.AuthResult{
							Auth: llm.ModelAuth{
								APIKey:  input.Credential.Key,
								BaseURL: "https://resolved.test/v1",
							},
							Source: "stored native key",
						}, nil
					},
				},
			},
			API: llm.APIProviderFuncs{
				StreamSimpleFunc: func(
					model llm.Model,
					_ llm.Context,
					_ llm.SimpleStreamOptions,
				) (*llm.AssistantMessageEventStream, error) {
					return llm.CompletedAssistantStream(
						llm.AssistantMessage(
							[]llm.ContentPart{llm.Text("native")},
							llm.StopReasonStop,
							model,
						),
					), nil
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.RegisterNativeProvider(native); err != nil {
		t.Fatal(err)
	}
	if _, ok := runtime.GetRegisteredNativeProvider(
		"native-provider",
	); !ok {
		t.Fatal("native provider was not registered")
	}
	if _, ok := runtime.ModelRegistry().
		GetRegisteredNativeProvider("native-provider"); !ok {
		t.Fatal("registry did not project the native provider")
	}
	if _, ok := runtime.Find(
		"native-provider",
		"native-model",
	); !ok {
		t.Fatal("native model was not published")
	}

	credential, err := runtime.Login(
		context.Background(),
		"native-provider",
		llm.CredentialTypeAPIKey,
		staticModelRuntimeAuthInteraction{answer: "native-key"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Key != "native-key" {
		t.Fatalf("credential = %#v", credential)
	}
	resolved, err := runtime.GetAuth(
		context.Background(),
		model,
		llm.AuthResolutionOverrides{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if resolved == nil ||
		resolved.APIKey != "native-key" ||
		resolved.BaseURL != "https://resolved.test/v1" {
		t.Fatalf("native auth = %#v", resolved)
	}

	runtime.UnregisterProvider("native-provider")
	if _, ok := runtime.GetProvider("native-provider"); ok {
		t.Fatal("native provider survived unregister")
	}
}

func TestModelRuntimeExplicitEmptyAPIKeyIsRequestScopedPiStyle(
	t *testing.T,
) {
	registry := NewInMemoryModelRegistry(
		NewInMemoryAuthStorage(nil),
	)
	if err := registry.RegisterProvider(
		"override-provider",
		ProviderConfigInput{
			BaseURL: "https://example.test/v1",
			APIKey:  "configured-key",
			API:     "override-api",
			StreamSimple: func(
				model llm.Model,
				_ llm.Context,
				options llm.SimpleStreamOptions,
			) (*llm.AssistantMessageEventStream, error) {
				if options.APIKey != "" {
					t.Fatalf(
						"API key = %q, want explicit empty",
						options.APIKey,
					)
				}
				return llm.CompletedAssistantStream(
					llm.AssistantMessage(
						[]llm.ContentPart{llm.Text("ok")},
						llm.StopReasonStop,
						model,
					),
				), nil
			},
			Models: []ProviderModelDefinition{{
				ID:            "override-model",
				ContextWindow: 1000,
				MaxTokens:     100,
			}},
		},
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		registry.UnregisterProvider("override-provider")
	})
	runtime, err := NewModelRuntimeFromRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := runtime.GetModel(
		"override-provider",
		"override-model",
	)
	if !ok {
		t.Fatal("override model not found")
	}
	empty := ""
	if _, err := runtime.CompleteSimple(
		context.Background(),
		model,
		llm.Context{},
		llm.ModelsStreamOptions{APIKey: &empty},
	); err != nil {
		t.Fatal(err)
	}
	auth, err := runtime.GetAuth(
		context.Background(),
		model,
		llm.AuthResolutionOverrides{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if auth == nil || auth.APIKey != "configured-key" {
		t.Fatalf(
			"stored request auth changed after override: %#v",
			auth,
		)
	}
}

type staticModelRuntimeAuthInteraction struct {
	answer string
}

func (i staticModelRuntimeAuthInteraction) Prompt(
	context.Context,
	llm.AuthPrompt,
) (string, error) {
	return i.answer, nil
}

func (staticModelRuntimeAuthInteraction) Notify(llm.AuthEvent) {}

func TestModelRuntimeAvailabilityRefreshHonorsContext(
	t *testing.T,
) {
	runtime, err := NewModelRuntimeFromRegistry(
		NewInMemoryModelRegistry(NewInMemoryAuthStorage(nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	err = runtime.ForceRefreshAvailability(ctx)
	if err == nil {
		t.Fatal("cancelled availability refresh returned nil")
	}
	if time.Since(started) > time.Second {
		t.Fatal("cancelled availability refresh did not return promptly")
	}
}

func TestModelRuntimeForceAvailabilityRefreshRunsAfterInflightPiStyle(
	t *testing.T,
) {
	runtime, err := NewModelRuntimeFromRegistry(
		NewInMemoryModelRegistry(NewInMemoryAuthStorage(nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	var (
		checks     atomic.Int32
		block      atomic.Bool
		blocked    atomic.Bool
		entered    = make(chan struct{})
		release    = make(chan struct{})
		firstError = make(chan error, 1)
		forceError = make(chan error, 1)
	)
	native, err := llm.CreateProvider(
		llm.CreateProviderOptions{
			ID: "availability-native",
			Auth: llm.ProviderAuth{
				APIKey: &llm.APIKeyAuth{
					Name: "Availability key",
					Check: func(
						ctx context.Context,
						_ llm.APIKeyCheckInput,
					) (*llm.AuthCheck, error) {
						checks.Add(1)
						if block.Load() &&
							blocked.CompareAndSwap(false, true) {
							close(entered)
							select {
							case <-ctx.Done():
								return nil, ctx.Err()
							case <-release:
							}
						}
						return &llm.AuthCheck{
							Type:   llm.CredentialTypeAPIKey,
							Source: "test",
						}, nil
					},
					Resolve: func(
						context.Context,
						llm.APIKeyResolveInput,
					) (*llm.AuthResult, error) {
						return &llm.AuthResult{
							Auth:   llm.ModelAuth{APIKey: "test"},
							Source: "test",
						}, nil
					},
				},
			},
			Models: []llm.Model{{
				ID:            "availability-model",
				Name:          "Availability Model",
				API:           "availability-api",
				Provider:      "availability-native",
				Input:         []string{"text"},
				ContextWindow: 1000,
				MaxTokens:     100,
			}},
			API: llm.APIProviderFuncs{
				StreamSimpleFunc: func(
					model llm.Model,
					_ llm.Context,
					_ llm.SimpleStreamOptions,
				) (*llm.AssistantMessageEventStream, error) {
					return llm.CompletedAssistantStream(
						llm.AssistantMessage(
							nil,
							llm.StopReasonStop,
							model,
						),
					), nil
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.RegisterNativeProvider(native); err != nil {
		t.Fatal(err)
	}
	baseline := checks.Load()
	block.Store(true)
	go func() {
		firstError <- runtime.RefreshAvailability(
			context.Background(),
		)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("availability refresh did not enter auth check")
	}
	go func() {
		forceError <- runtime.ForceRefreshAvailability(
			context.Background(),
		)
	}()
	close(release)
	for name, result := range map[string]<-chan error{
		"first": firstError,
		"force": forceError,
	} {
		select {
		case err := <-result:
			if err != nil {
				t.Fatalf("%s refresh: %v", name, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s refresh did not finish", name)
		}
	}
	if got := checks.Load(); got < baseline+2 {
		t.Fatalf(
			"auth checks = %d, want at least %d",
			got,
			baseline+2,
		)
	}
}

func TestProtocolExtensionRuntimeBindsModelRuntimePiStyle(
	t *testing.T,
) {
	extensions := NewProtocolExtensionRuntime(
		CapabilityProvidersRegister,
	)
	if err := extensions.LoadFactories(
		[]ProtocolExtensionFactory{{
			Path: "runtime-provider.gi.json",
			Factory: func(
				ctx *ProtocolExtensionContext,
			) error {
				return ctx.RegisterProvider(
					"extension-runtime-provider",
					ProtocolProviderOverride{
						BaseURL: "https://example.test/v1",
						APIKey:  "extension-key",
						API:     "extension-runtime-api",
						StreamSimple: func(
							model llm.Model,
							_ llm.Context,
							_ llm.SimpleStreamOptions,
						) (*llm.AssistantMessageEventStream, error) {
							return llm.CompletedAssistantStream(
								llm.AssistantMessage(
									[]llm.ContentPart{
										llm.Text("extension"),
									},
									llm.StopReasonStop,
									model,
								),
							), nil
						},
						Models: []ProviderModelDefinition{{
							ID:            "extension-runtime-model",
							ContextWindow: 1000,
							MaxTokens:     100,
						}},
					},
				)
			},
		}},
	); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewModelRuntimeFromRegistry(
		NewInMemoryModelRegistry(NewInMemoryAuthStorage(nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	extensions.BindModelRuntime(runtime)
	model, ok := runtime.Find(
		"extension-runtime-provider",
		"extension-runtime-model",
	)
	if !ok {
		t.Fatal("pending extension provider was not composed")
	}
	message, err := runtime.CompleteSimple(
		context.Background(),
		model,
		llm.Context{},
		llm.ModelsStreamOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if message.Content[0].Text != "extension" {
		t.Fatalf("message = %#v", message)
	}
	if _, ok := runtime.ModelRegistry().Find(
		"extension-runtime-provider",
		"extension-runtime-model",
	); !ok {
		t.Fatal("compatibility registry was not synchronized")
	}
	config, ok := runtime.ModelRegistry().
		GetRegisteredProviderConfig("extension-runtime-provider")
	if !ok || config.APIKey != "extension-key" {
		t.Fatalf("registered provider config = %#v, found=%v", config, ok)
	}
	if !containsString(
		runtime.ModelRegistry().GetRegisteredProviderIDs(),
		"extension-runtime-provider",
	) {
		t.Fatal("registered provider IDs omitted extension provider")
	}
	runtime.UnregisterProvider("extension-runtime-provider")
}

func TestAgentSessionRetainsModelRuntimeAcrossReplacementPiStyle(
	t *testing.T,
) {
	runtime, err := NewModelRuntimeFromRegistry(
		NewInMemoryModelRegistry(NewInMemoryAuthStorage(nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	model, ok := runtime.GetModel("openai", "gpt-4o-mini")
	if !ok {
		t.Fatal("test model not found")
	}
	cwd := t.TempDir()
	manager, err := InMemorySessionManager(cwd)
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            cwd,
		SessionManager: manager,
		Model:          model,
		ModelRuntime:   runtime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.ModelRuntime != runtime {
		t.Fatal("session did not retain model runtime")
	}
	replacementManager, err := InMemorySessionManager(cwd)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := cloneAgentSessionWithManager(
		session,
		replacementManager,
	)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.ModelRuntime != runtime {
		t.Fatal("replacement session changed model runtime ownership")
	}
}
