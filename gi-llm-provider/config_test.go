package gillmprovider

import "testing"

func TestNormalizeAzureOpenAIBaseURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "normalizes Cognitive Services root endpoints to openai v1",
			input: "https://marc-quicktests-resource.cognitiveservices.azure.com",
			want:  "https://marc-quicktests-resource.cognitiveservices.azure.com/openai/v1",
		},
		{
			name:  "normalizes Azure OpenAI root endpoints to openai v1",
			input: "https://my-resource.openai.azure.com",
			want:  "https://my-resource.openai.azure.com/openai/v1",
		},
		{
			name:  "normalizes openai to openai v1",
			input: "https://my-resource.cognitiveservices.azure.com/openai",
			want:  "https://my-resource.cognitiveservices.azure.com/openai/v1",
		},
		{
			name:  "preserves openai v1 endpoints",
			input: "https://my-resource.cognitiveservices.azure.com/openai/v1",
			want:  "https://my-resource.cognitiveservices.azure.com/openai/v1",
		},
		{
			name:  "preserves explicit non-Azure proxy paths",
			input: "https://my-proxy.example.com/v1",
			want:  "https://my-proxy.example.com/v1",
		},
		{
			name:  "strips query params when normalizing Azure host URLs",
			input: "https://my-resource.openai.azure.com/openai?api-version=2024-12-01",
			want:  "https://my-resource.openai.azure.com/openai/v1",
		},
		{
			name:  "preserves query params on non-Azure proxy URLs",
			input: "https://my-proxy.example.com/v1?custom=true",
			want:  "https://my-proxy.example.com/v1?custom=true",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeAzureOpenAIBaseURL(tc.input)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("throws on invalid URLs", func(t *testing.T) {
		if _, err := NormalizeAzureOpenAIBaseURL("not-a-url"); err == nil {
			t.Fatal("expected invalid URL error")
		}
	})
}

func TestResolveAzureOpenAIConfigBuildsDefaultFromResourceName(t *testing.T) {
	t.Setenv("AZURE_OPENAI_RESOURCE_NAME", "my-resource")

	config, err := ResolveAzureOpenAIConfig(MustGetModel("azure-openai-responses", "gpt-4o-mini"), AzureOpenAIResponsesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if config.BaseURL != "https://my-resource.openai.azure.com/openai/v1" {
		t.Fatalf("base URL = %q", config.BaseURL)
	}
}

func TestResolveBedrockClientConfig(t *testing.T) {
	t.Run("assigns eu-central-1 runtime URLs to built-in EU inference profiles", func(t *testing.T) {
		model := MustGetModel("amazon-bedrock", "eu.anthropic.claude-sonnet-4-5-20250929-v1:0")
		if model.BaseURL != "https://bedrock-runtime.eu-central-1.amazonaws.com" {
			t.Fatalf("base URL = %q", model.BaseURL)
		}
	})
	t.Run("does not pin standard AWS endpoints when AWS_REGION is configured", func(t *testing.T) {
		t.Setenv("AWS_REGION", "us-east-2")
		config := ResolveBedrockClientConfig(MustGetModel("amazon-bedrock", "us.anthropic.claude-opus-4-7"), BedrockClientOptions{})
		if config.Region != "us-east-2" || config.Endpoint != "" {
			t.Fatalf("config = %#v", config)
		}
	})
	t.Run("derives region from a built-in EU endpoint when no region or profile is configured", func(t *testing.T) {
		config := ResolveBedrockClientConfig(MustGetModel("amazon-bedrock", "eu.anthropic.claude-sonnet-4-5-20250929-v1:0"), BedrockClientOptions{})
		if config.Region != "eu-central-1" || config.Endpoint != "https://bedrock-runtime.eu-central-1.amazonaws.com" {
			t.Fatalf("config = %#v", config)
		}
	})
	t.Run("passes custom endpoint through", func(t *testing.T) {
		t.Setenv("AWS_REGION", "us-west-2")
		model := MustGetModel("amazon-bedrock", "us.anthropic.claude-opus-4-7")
		model.BaseURL = "https://bedrock-vpc.example.com"
		config := ResolveBedrockClientConfig(model, BedrockClientOptions{})
		if config.Region != "us-west-2" || config.Endpoint != "https://bedrock-vpc.example.com" {
			t.Fatalf("config = %#v", config)
		}
	})
}

func TestResolveGoogleVertexClientConfig(t *testing.T) {
	model := MustGetModel("google-vertex", "gemini-3-flash-preview")
	tests := []struct {
		name       string
		options    GoogleVertexOptions
		envAPIKey  string
		wantAPIKey string
		wantADC    bool
	}{
		{name: "falls back to ADC when options.apiKey is a placeholder marker", options: GoogleVertexOptions{APIKey: "<authenticated>", Project: "test-project", Location: "us-central1"}, wantADC: true},
		{name: "falls back to ADC when options.apiKey is the gcp-vertex-credentials marker", options: GoogleVertexOptions{APIKey: GCPVertexCredentialsMarker, Project: "test-project", Location: "us-central1"}, wantADC: true},
		{name: "falls back to ADC when GOOGLE_CLOUD_API_KEY is a placeholder marker", envAPIKey: "<authenticated>", options: GoogleVertexOptions{Project: "test-project", Location: "us-central1"}, wantADC: true},
		{name: "still uses the API key client for real API keys", options: GoogleVertexOptions{APIKey: "AIzaSyExampleRealisticLookingApiKey123456"}, wantAPIKey: "AIzaSyExampleRealisticLookingApiKey123456"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envAPIKey != "" {
				t.Setenv("GOOGLE_CLOUD_API_KEY", tc.envAPIKey)
			}
			config := ResolveGoogleVertexClientConfig(model, tc.options)
			if tc.wantADC {
				if config.APIKey != "" || config.Project != "test-project" || config.Location != "us-central1" || config.APIVersion != "v1" {
					t.Fatalf("config = %#v", config)
				}
			} else if config.APIKey != tc.wantAPIKey || config.Project != "" || config.Location != "" {
				t.Fatalf("config = %#v", config)
			}
		})
	}
}

func TestResolveGoogleVertexCustomBaseURL(t *testing.T) {
	t.Run("does not forward generated Vertex base URL placeholders", func(t *testing.T) {
		model := MustGetModel("google-vertex", "gemini-3-flash-preview")
		config := ResolveGoogleVertexClientConfig(model, GoogleVertexOptions{Project: "test-project", Location: "us-central1"})
		if config.HTTPOptions != nil {
			t.Fatalf("generated placeholder base URL should be omitted: %#v", config.HTTPOptions)
		}
	})

	t.Run("forwards custom baseUrl to the ADC client", func(t *testing.T) {
		model := MustGetModel("google-vertex", "gemini-3-flash-preview")
		model.BaseURL = "https://proxy.example.com"
		config := ResolveGoogleVertexClientConfig(model, GoogleVertexOptions{Project: "test-project", Location: "us-central1"})
		if config.HTTPOptions == nil || config.HTTPOptions.BaseURL != "https://proxy.example.com" || config.HTTPOptions.BaseURLResourceScope != "COLLECTION" {
			t.Fatalf("http options = %#v", config.HTTPOptions)
		}
	})

	t.Run("forwards custom baseUrl to the API key client", func(t *testing.T) {
		model := MustGetModel("google-vertex", "gemini-3-flash-preview")
		model.BaseURL = "https://proxy.example.com"
		config := ResolveGoogleVertexClientConfig(model, GoogleVertexOptions{APIKey: "AIzaSyExampleRealisticLookingApiKey123456"})
		if config.HTTPOptions == nil || config.HTTPOptions.BaseURL != "https://proxy.example.com" || config.HTTPOptions.BaseURLResourceScope != "COLLECTION" {
			t.Fatalf("http options = %#v", config.HTTPOptions)
		}
		if config.APIKey != "AIzaSyExampleRealisticLookingApiKey123456" || config.Project != "" || config.Location != "" {
			t.Fatalf("config = %#v", config)
		}
	})

	t.Run("does not append apiVersion when custom baseUrl already includes one", func(t *testing.T) {
		model := MustGetModel("google-vertex", "gemini-3-flash-preview")
		model.BaseURL = "https://proxy.example.com/v1/projects/test-project/locations/global"
		config := ResolveGoogleVertexClientConfig(model, GoogleVertexOptions{Project: "test-project", Location: "us-central1"})
		if config.HTTPOptions == nil || config.HTTPOptions.APIVersion != "" {
			t.Fatalf("http options = %#v", config.HTTPOptions)
		}
	})
}

func TestResolveHTTPProxyURLForTarget(t *testing.T) {
	clearProxyEnv(t)
	t.Setenv("HTTPS_PROXY", "http://proxy.example:8080")
	t.Setenv("NO_PROXY", "bedrock-runtime.us-east-1.amazonaws.com")
	proxy, err := ResolveHTTPProxyURLForTarget("https://bedrock-runtime.us-east-1.amazonaws.com")
	if err != nil {
		t.Fatal(err)
	}
	if proxy != nil {
		t.Fatalf("proxy should be nil, got %v", proxy)
	}

	t.Setenv("NO_PROXY", "")
	proxy, err = ResolveHTTPProxyURLForTarget("https://bedrock-runtime.us-east-1.amazonaws.com")
	if err != nil {
		t.Fatal(err)
	}
	if proxy == nil || proxy.String() != "http://proxy.example:8080" {
		t.Fatalf("proxy = %v", proxy)
	}

	t.Setenv("HTTPS_PROXY", "socks5://proxy.example:1080")
	if _, err = ResolveHTTPProxyURLForTarget("https://bedrock-runtime.us-east-1.amazonaws.com"); err == nil {
		t.Fatal("expected unsupported proxy protocol error")
	}
}

func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"NO_PROXY",
		"ALL_PROXY",
		"http_proxy",
		"https_proxy",
		"no_proxy",
		"all_proxy",
		"npm_config_http_proxy",
		"npm_config_https_proxy",
		"npm_config_proxy",
		"npm_config_no_proxy",
	} {
		t.Setenv(key, "")
	}
}
