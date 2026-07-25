package gillmprovider

import (
	"context"
	"testing"
)

func TestPackageStreamInjectsScopedEnvironmentAPIKeyForCompatibility(t *testing.T) {
	const api = "compat-environment-api-key"
	var captured []string
	RegisterAPIProvider(api, APIProviderFuncs{
		StreamFunc: func(
			model Model,
			_ Context,
			options StreamOptions,
		) (*AssistantMessageEventStream, error) {
			captured = append(captured, options.APIKey)
			return CompletedAssistantStream(
				AssistantMessage([]ContentPart{Text("ok")}, StopReasonStop, model),
			), nil
		},
	})
	t.Cleanup(func() { UnregisterAPIProvider(api) })

	model := Model{
		ID:       "custom-openai",
		API:      api,
		Provider: "openai",
	}
	stream, err := Stream(model, Context{}, StreamOptions{
		Env: ProviderEnv{"OPENAI_API_KEY": "scoped-key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Result(context.Background()); err != nil {
		t.Fatal(err)
	}

	stream, err = StreamSimple(model, Context{}, SimpleStreamOptions{
		APIKey: "explicit-key",
		Env:    ProviderEnv{"OPENAI_API_KEY": "ignored-key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Result(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(captured) != 2 ||
		captured[0] != "scoped-key" ||
		captured[1] != "explicit-key" {
		t.Fatalf("captured API keys = %#v", captured)
	}
}

func TestPackageStreamDoesNotInjectAmbientCredentialMarker(t *testing.T) {
	const api = "compat-ambient-marker"
	t.Setenv("AWS_PROFILE", "work")
	captured := "not-called"
	RegisterAPIProvider(api, APIProviderFuncs{
		StreamFunc: func(
			model Model,
			_ Context,
			options StreamOptions,
		) (*AssistantMessageEventStream, error) {
			captured = options.APIKey
			return CompletedAssistantStream(
				AssistantMessage(nil, StopReasonStop, model),
			), nil
		},
	})
	t.Cleanup(func() { UnregisterAPIProvider(api) })

	stream, err := Stream(Model{
		ID:       "bedrock-model",
		API:      api,
		Provider: "amazon-bedrock",
	}, Context{}, StreamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Result(context.Background()); err != nil {
		t.Fatal(err)
	}
	if captured != "" {
		t.Fatalf("ambient auth marker leaked as API key: %q", captured)
	}
}

func TestHasResolvedCloudflareAuthIsCaseInsensitive(t *testing.T) {
	tests := []struct {
		name    string
		options StreamOptions
		want    bool
	}{
		{
			name:    "API key",
			options: StreamOptions{APIKey: "key"},
			want:    true,
		},
		{
			name: "gateway header",
			options: StreamOptions{Headers: map[string]string{
				"CF-AIG-Authorization": "Bearer gateway",
			}},
			want: true,
		},
		{
			name: "blank values",
			options: StreamOptions{
				APIKey: " ",
				Headers: map[string]string{
					"cf-aig-authorization": "\t",
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasResolvedCloudflareAuth(tc.options); got != tc.want {
				t.Fatalf("hasResolvedCloudflareAuth() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestRegisterBundledOAuthFlowLoadersInstallsAtomicBatch(t *testing.T) {
	const (
		first  = "bundled-oauth-first"
		second = "bundled-oauth-second"
	)
	t.Cleanup(func() {
		UnregisterOAuthAuthLoader(first)
		UnregisterOAuthAuthLoader(second)
	})

	firstAuth := &OAuthAuth{Name: "First"}
	secondAuth := &OAuthAuth{Name: "Second"}
	loaders := map[string]OAuthAuthLoader{
		first: func(context.Context) (*OAuthAuth, error) {
			return firstAuth, nil
		},
		second: func(context.Context) (*OAuthAuth, error) {
			return secondAuth, nil
		},
	}
	RegisterBundledOAuthFlowLoaders(loaders)
	delete(loaders, first)

	loadedFirst, err := getOAuthAuthLoader(first)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	loadedSecond, err := getOAuthAuthLoader(second)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if loadedFirst != firstAuth || loadedSecond != secondAuth {
		t.Fatalf("loaded auth: first=%p second=%p", loadedFirst, loadedSecond)
	}

	RegisterBundledOAuthFlowLoaders(map[string]OAuthAuthLoader{first: nil})
	if getOAuthAuthLoader(first) != nil || getOAuthAuthLoader(second) == nil {
		t.Fatal("batch removal changed the wrong OAuth loader")
	}
}
