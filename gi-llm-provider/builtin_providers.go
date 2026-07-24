package gillmprovider

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type builtinProviderSpec struct {
	id        string
	name      string
	baseURL   string
	auth      func() ProviderAuth
	extraAPIs []string
	filter    func(models []Model, credential *Credential) []Model
}

// The declaration order matches Pi's providers/all.ts. Each call constructs
// fresh provider state; the generated catalog remains immutable shared input.
var builtinProviderSpecs = []builtinProviderSpec{
	{id: "amazon-bedrock", name: "Amazon Bedrock", auth: func() ProviderAuth {
		return ProviderAuth{APIKey: amazonBedrockAuth()}
	}},
	envBuiltin("ant-ling", "Ant Ling", "Ant Ling API key", "ANT_LING_API_KEY"),
	{id: "anthropic", name: "Anthropic", baseURL: "https://api.anthropic.com", auth: func() ProviderAuth {
		return ProviderAuth{
			APIKey: EnvAPIKeyAuth("Anthropic API key", "ANTHROPIC_OAUTH_TOKEN", "ANTHROPIC_API_KEY"),
			OAuth:  registeredOAuthAuth("anthropic", "Anthropic (Claude Pro/Max)", ""),
		}
	}},
	envBuiltin("azure-openai-responses", "Azure OpenAI", "Azure OpenAI API key", "AZURE_OPENAI_API_KEY"),
	envBuiltin("cerebras", "Cerebras", "Cerebras API key", "CEREBRAS_API_KEY"),
	{id: "cloudflare-ai-gateway", name: "Cloudflare AI Gateway", auth: func() ProviderAuth {
		return ProviderAuth{APIKey: cloudflareAIGatewayAuth()}
	}},
	{id: "cloudflare-workers-ai", name: "Cloudflare Workers AI", auth: func() ProviderAuth {
		return ProviderAuth{APIKey: cloudflareWorkersAIAuth()}
	}},
	envBuiltin("deepseek", "DeepSeek", "DeepSeek API key", "DEEPSEEK_API_KEY"),
	envBuiltin("fireworks", "Fireworks", "Fireworks API key", "FIREWORKS_API_KEY"),
	{
		id:      "github-copilot",
		name:    "GitHub Copilot",
		baseURL: "https://api.individual.githubcopilot.com",
		auth: func() ProviderAuth {
			return ProviderAuth{
				APIKey: EnvAPIKeyAuth("GitHub Copilot token", "COPILOT_GITHUB_TOKEN"),
				OAuth:  registeredOAuthAuth("github-copilot", "GitHub Copilot", ""),
			}
		},
		filter: filterGitHubCopilotModels,
	},
	envBuiltin("google", "Google", "Gemini API key", "GEMINI_API_KEY"),
	{id: "google-vertex", name: "Google Vertex AI", auth: func() ProviderAuth {
		return ProviderAuth{APIKey: googleVertexAuth()}
	}},
	envBuiltin("groq", "Groq", "Groq API key", "GROQ_API_KEY"),
	envBuiltin("huggingface", "Hugging Face", "Hugging Face token", "HF_TOKEN"),
	{id: "kimi-coding", name: "Kimi For Coding", auth: func() ProviderAuth {
		return ProviderAuth{
			APIKey: EnvAPIKeyAuth("Kimi API key", "KIMI_API_KEY"),
			OAuth: registeredOAuthAuth(
				"kimi-coding",
				"Kimi Code (subscription)",
				"Sign in with Kimi Code",
			),
		}
	}},
	envBuiltin("minimax", "MiniMax", "MiniMax API key", "MINIMAX_API_KEY"),
	envBuiltin("minimax-cn", "MiniMax CN", "MiniMax CN API key", "MINIMAX_CN_API_KEY"),
	envBuiltin("mistral", "Mistral", "Mistral API key", "MISTRAL_API_KEY"),
	envBuiltin("moonshotai", "Moonshot AI", "Moonshot AI API key", "MOONSHOT_API_KEY"),
	envBuiltin("moonshotai-cn", "Moonshot AI CN", "Moonshot AI API key", "MOONSHOT_API_KEY"),
	envBuiltin("nvidia", "NVIDIA", "NVIDIA API key", "NVIDIA_API_KEY"),
	envBuiltin("openai", "OpenAI", "OpenAI API key", "OPENAI_API_KEY"),
	{
		id:        "openai-codex",
		name:      "OpenAI Codex",
		baseURL:   "https://chatgpt.com/backend-api",
		extraAPIs: []string{"openai-codex-responses"},
		auth: func() ProviderAuth {
			return ProviderAuth{
				OAuth: registeredOAuthAuth(
					"openai-codex",
					"OpenAI (ChatGPT Plus/Pro)",
					"",
				),
			}
		},
	},
	envBuiltin("opencode", "OpenCode Zen", "OpenCode API key", "OPENCODE_API_KEY"),
	envBuiltin("opencode-go", "OpenCode Zen Go", "OpenCode API key", "OPENCODE_API_KEY"),
	{id: "openrouter", name: "OpenRouter", baseURL: "https://openrouter.ai/api/v1", auth: func() ProviderAuth {
		return ProviderAuth{
			APIKey: EnvAPIKeyAuth("OpenRouter API key", "OPENROUTER_API_KEY"),
			OAuth: registeredOAuthAuth(
				"openrouter",
				"OpenRouter OAuth",
				"Sign in with OpenRouter",
			),
		}
	}},
	envBuiltin("qwen-token-plan", "Qwen Token Plan", "Qwen Token Plan API key", "QWEN_TOKEN_PLAN_API_KEY"),
	envBuiltin("qwen-token-plan-cn", "Qwen Token Plan CN", "Qwen Token Plan CN API key", "QWEN_TOKEN_PLAN_CN_API_KEY"),
	{
		id:        "radius",
		name:      "Radius",
		baseURL:   "https://radius.pi.dev",
		extraAPIs: []string{"pi-messages"},
		auth: func() ProviderAuth {
			return ProviderAuth{
				APIKey: EnvAPIKeyAuth("Radius API key", "RADIUS_API_KEY"),
				OAuth:  registeredOAuthAuth("radius", "Radius", ""),
			}
		},
	},
	envBuiltin("together", "Together", "Together API key", "TOGETHER_API_KEY"),
	envBuiltin("vercel-ai-gateway", "Vercel AI Gateway", "Vercel AI Gateway API key", "AI_GATEWAY_API_KEY"),
	{id: "xai", name: "xAI", baseURL: "https://api.x.ai/v1", auth: func() ProviderAuth {
		return ProviderAuth{
			APIKey: EnvAPIKeyAuth("xAI API key", "XAI_API_KEY"),
			OAuth: registeredOAuthAuth(
				"xai",
				"xAI (Grok/X subscription)",
				"Sign in with SuperGrok or X Premium",
			),
		}
	}},
	envBuiltin("xiaomi", "Xiaomi", "Xiaomi API key", "XIAOMI_API_KEY"),
	envBuiltin(
		"xiaomi-token-plan-ams",
		"Xiaomi Token Plan AMS",
		"Xiaomi Token Plan AMS API key",
		"XIAOMI_TOKEN_PLAN_AMS_API_KEY",
	),
	envBuiltin(
		"xiaomi-token-plan-cn",
		"Xiaomi Token Plan CN",
		"Xiaomi Token Plan CN API key",
		"XIAOMI_TOKEN_PLAN_CN_API_KEY",
	),
	envBuiltin(
		"xiaomi-token-plan-sgp",
		"Xiaomi Token Plan SGP",
		"Xiaomi Token Plan SGP API key",
		"XIAOMI_TOKEN_PLAN_SGP_API_KEY",
	),
	envBuiltin("zai", "Z.AI", "Z.AI API key", "ZAI_API_KEY"),
	envBuiltin("zai-coding-cn", "Z.AI Coding CN", "Z.AI Coding CN API key", "ZAI_CODING_CN_API_KEY"),
}

func envBuiltin(id, name, authName string, envVars ...string) builtinProviderSpec {
	keys := append([]string(nil), envVars...)
	return builtinProviderSpec{
		id:   id,
		name: name,
		auth: func() ProviderAuth {
			return ProviderAuth{APIKey: EnvAPIKeyAuth(authName, keys...)}
		},
	}
}

// GetBuiltinProviderIDs returns static catalog provider IDs in generated order.
func GetBuiltinProviderIDs() []string {
	return builtinProviderIDs()
}

// GetBuiltinModel returns a detached model from the compiled release catalog.
func GetBuiltinModel(providerID, modelID string) (Model, bool) {
	return builtinModel(providerID, modelID)
}

// GetBuiltinModels returns one provider's detached compiled catalog.
func GetBuiltinModels(providerID string) []Model {
	return builtinModels(providerID)
}

// GetBuiltinModelDataGeneratedAt returns the release catalog generation time.
func GetBuiltinModelDataGeneratedAt() time.Time {
	generatedAt, err := time.Parse(
		time.RFC3339Nano,
		piGeneratedModelDataGeneratedAt,
	)
	if err != nil {
		panic("invalid generated model data timestamp: " + err.Error())
	}
	return generatedAt
}

// NewBuiltinProvider constructs a fresh runtime provider backed by the
// immutable release catalog.
func NewBuiltinProvider(providerID string) (*Provider, error) {
	for _, spec := range builtinProviderSpecs {
		if spec.id != providerID {
			continue
		}
		models := GetBuiltinModels(spec.id)
		apiProviders := builtinAPIProviders(spec, models)
		return CreateProvider(CreateProviderOptions{
			ID:           spec.id,
			Name:         spec.name,
			BaseURL:      spec.baseURL,
			Auth:         spec.auth(),
			Models:       models,
			FilterModels: spec.filter,
			APIs:         apiProviders,
		})
	}
	return nil, fmt.Errorf("unknown built-in provider: %s", providerID)
}

// BuiltinProviders constructs every built-in provider in Pi declaration order.
func BuiltinProviders() ([]*Provider, error) {
	providers := make([]*Provider, 0, len(builtinProviderSpecs))
	for _, spec := range builtinProviderSpecs {
		provider, err := NewBuiltinProvider(spec.id)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	return providers, nil
}

// BuiltinModels creates an isolated Models collection with every built-in
// provider registered.
func BuiltinModels(options ...ModelsOptions) (*Models, error) {
	models := NewModels(options...)
	providers, err := BuiltinProviders()
	if err != nil {
		return nil, err
	}
	for _, provider := range providers {
		if err := models.SetProvider(provider); err != nil {
			return nil, err
		}
	}
	return models, nil
}

func builtinAPIProviders(spec builtinProviderSpec, models []Model) map[string]APIProvider {
	apis := append([]string(nil), spec.extraAPIs...)
	for _, model := range models {
		if model.API != "" && !containsStringExact(apis, model.API) {
			apis = append(apis, model.API)
		}
	}
	providers := make(map[string]APIProvider, len(apis))
	for _, api := range apis {
		implementation := GetAPIProvider(api)
		if implementation != nil && IsCloudflareProvider(spec.id) {
			implementation = cloudflareAPIProvider{next: implementation}
		}
		// Keep unsupported APIs as explicit nil entries. CreateProvider will
		// produce a typed stream error while the catalog remains discoverable.
		providers[api] = implementation
	}
	return providers
}

func containsStringExact(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

type cloudflareAPIProvider struct {
	next APIProvider
}

func (p cloudflareAPIProvider) Stream(
	model Model,
	llmContext Context,
	options StreamOptions,
) (*AssistantMessageEventStream, error) {
	if p.next == nil {
		return nil, errors.New("Cloudflare API provider is not configured")
	}
	return p.next.Stream(ResolveCloudflareModel(model, options.Env), llmContext, options)
}

func (p cloudflareAPIProvider) StreamSimple(
	model Model,
	llmContext Context,
	options SimpleStreamOptions,
) (*AssistantMessageEventStream, error) {
	if p.next == nil {
		return nil, errors.New("Cloudflare API provider is not configured")
	}
	return p.next.StreamSimple(ResolveCloudflareModel(model, options.Env), llmContext, options)
}

func filterGitHubCopilotModels(models []Model, credential *Credential) []Model {
	if credential == nil || credential.Type != CredentialTypeOAuth {
		return models
	}
	raw, ok := credential.Metadata["availableModelIds"]
	if !ok {
		return models
	}
	available := map[string]struct{}{}
	switch ids := raw.(type) {
	case []string:
		for _, id := range ids {
			available[id] = struct{}{}
		}
	case []any:
		for _, value := range ids {
			id, ok := value.(string)
			if !ok {
				return models
			}
			available[id] = struct{}{}
		}
	default:
		return models
	}
	filtered := make([]Model, 0, len(models))
	for _, model := range models {
		if _, ok := available[model.ID]; ok {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

// NewAnthropicProvider returns the built-in Anthropic provider.
func NewAnthropicProvider() (*Provider, error) {
	return NewBuiltinProvider("anthropic")
}

// NewAmazonBedrockProvider returns the built-in Amazon Bedrock provider.
func NewAmazonBedrockProvider() (*Provider, error) {
	return NewBuiltinProvider("amazon-bedrock")
}

// NewCloudflareWorkersAIProvider returns the built-in Workers AI provider.
func NewCloudflareWorkersAIProvider() (*Provider, error) {
	return NewBuiltinProvider("cloudflare-workers-ai")
}

// NewCloudflareAIGatewayProvider returns the built-in AI Gateway provider.
func NewCloudflareAIGatewayProvider() (*Provider, error) {
	return NewBuiltinProvider("cloudflare-ai-gateway")
}

// NewGoogleVertexProvider returns the built-in Google Vertex provider.
func NewGoogleVertexProvider() (*Provider, error) {
	return NewBuiltinProvider("google-vertex")
}

func ambientAuthResult() *AuthResult {
	return &AuthResult{Auth: ModelAuth{}, Source: "ambient"}
}

func ambientAPIKeyAuth() *APIKeyAuth {
	return &APIKeyAuth{
		Name: "Ambient configuration",
		Resolve: func(context.Context, APIKeyResolveInput) (*AuthResult, error) {
			return ambientAuthResult(), nil
		},
	}
}
