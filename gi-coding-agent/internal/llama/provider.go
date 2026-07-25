package llama

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

const (
	LlamaProviderID       = "llama.cpp"
	DefaultLlamaServerURL = "http://127.0.0.1:8080"
)

type LlamaProviderOptions struct {
	HTTPClient   llm.HTTPDoer
	Timeout      time.Duration
	PollInterval time.Duration
}

// LlamaProviderController owns the atomic catalog projection shared by the
// provider refresh path and the interactive management flow.
type LlamaProviderController struct {
	mu sync.RWMutex

	provider      *llm.Provider
	catalog       []LlamaModelInfo
	serverURL     string
	clientOptions LlamaClientOptions
}

func CreateLlamaProvider(
	options ...LlamaProviderOptions,
) (*LlamaProviderController, error) {
	if len(options) > 1 {
		return nil, errors.New(
			"llama provider accepts at most one options value",
		)
	}
	var selected LlamaProviderOptions
	if len(options) == 1 {
		selected = options[0]
	}
	controller := &LlamaProviderController{
		clientOptions: LlamaClientOptions{
			HTTPClient:   selected.HTTPClient,
			Timeout:      selected.Timeout,
			PollInterval: selected.PollInterval,
		},
	}
	inferenceURL, err := LlamaInferenceURL(DefaultLlamaServerURL)
	if err != nil {
		return nil, err
	}
	api := llm.GetAPIProvider("openai-completions")
	if api == nil {
		return nil, errors.New(
			"openai-completions API provider is not registered",
		)
	}
	provider, err := llm.CreateProvider(llm.CreateProviderOptions{
		ID:      LlamaProviderID,
		Name:    "llama.cpp",
		BaseURL: inferenceURL,
		Auth: llm.ProviderAuth{
			APIKey: controller.apiKeyAuth(),
		},
		API: api,
	})
	if err != nil {
		return nil, err
	}
	controller.provider = provider
	provider.ModelSource = controller.modelSnapshot
	provider.RefreshModelsFunc = controller.refreshModels
	return controller, nil
}

func (c *LlamaProviderController) Provider() *llm.Provider {
	if c == nil {
		return nil
	}
	return c.provider
}

func (c *LlamaProviderController) SetCatalog(
	catalog []LlamaModelInfo,
	serverURL string,
) error {
	if c == nil {
		return errors.New("llama provider controller is required")
	}
	normalized, err := NormalizeLlamaServerURL(serverURL)
	if err != nil {
		return err
	}
	loaded := make([]LlamaModelInfo, 0, len(catalog))
	for _, entry := range catalog {
		if entry.Status.Value != LlamaModelLoaded {
			continue
		}
		if strings.TrimSpace(entry.ID) == "" {
			return errors.New("llama.cpp model ID is required")
		}
		if _, err := llamaProviderModel(entry, normalized); err != nil {
			return err
		}
		loaded = append(loaded, cloneLlamaModelInfo(entry))
	}
	c.mu.Lock()
	c.catalog = loaded
	c.serverURL = normalized
	c.mu.Unlock()
	return nil
}

func (c *LlamaProviderController) modelSnapshot() ([]llm.Model, error) {
	if c == nil {
		return nil, nil
	}
	c.mu.RLock()
	catalog := cloneLlamaModelCatalog(c.catalog)
	serverURL := c.serverURL
	c.mu.RUnlock()
	models := make([]llm.Model, 0, len(catalog))
	for _, entry := range catalog {
		model, err := llamaProviderModel(entry, serverURL)
		if err != nil {
			return nil, err
		}
		models = append(models, model)
	}
	return models, nil
}

func (c *LlamaProviderController) apiKeyAuth() *llm.APIKeyAuth {
	return &llm.APIKeyAuth{
		Name: "llama.cpp server",
		Login: func(
			ctx context.Context,
			interaction llm.AuthInteraction,
		) (llm.Credential, error) {
			if interaction == nil {
				return llm.Credential{}, errors.New(
					"llama.cpp login requires an interaction",
				)
			}
			fallbackURL := strings.TrimSpace(
				os.Getenv("LLAMA_BASE_URL"),
			)
			if fallbackURL == "" {
				fallbackURL = DefaultLlamaServerURL
			}
			enteredURL, err := interaction.Prompt(
				ctx,
				llm.AuthPrompt{
					Type:        llm.AuthPromptText,
					Message:     "llama.cpp server URL",
					Placeholder: fallbackURL,
				},
			)
			if err != nil {
				return llm.Credential{}, err
			}
			serverURL := strings.TrimSpace(enteredURL)
			if serverURL == "" {
				serverURL = fallbackURL
			}
			serverURL, err = NormalizeLlamaServerURL(serverURL)
			if err != nil {
				return llm.Credential{}, err
			}
			apiKey, err := interaction.Prompt(
				ctx,
				llm.AuthPrompt{
					Type:    llm.AuthPromptSecret,
					Message: "API key (optional)",
				},
			)
			if err != nil {
				return llm.Credential{}, err
			}
			apiKey = strings.TrimSpace(apiKey)
			client, err := NewLlamaClient(
				serverURL,
				apiKey,
				c.clientOptions,
			)
			if err != nil {
				return llm.Credential{}, err
			}
			if _, err := client.List(ctx, LlamaListOptions{}); err != nil {
				return llm.Credential{}, err
			}
			return llm.Credential{
				Type: llm.CredentialTypeAPIKey,
				Key:  apiKey,
				Env: llm.ProviderEnv{
					"LLAMA_BASE_URL": serverURL,
				},
			}, nil
		},
		Check: func(
			ctx context.Context,
			input llm.APIKeyCheckInput,
		) (*llm.AuthCheck, error) {
			serverURL, err := resolveLlamaServerURL(
				ctx,
				input.Context,
				input.Credential,
			)
			if err != nil || serverURL == "" {
				return nil, err
			}
			source := "LLAMA_BASE_URL"
			if input.Credential != nil {
				source = "stored credential"
			}
			return &llm.AuthCheck{
				Type:   llm.CredentialTypeAPIKey,
				Source: source,
			}, nil
		},
		Resolve: func(
			ctx context.Context,
			input llm.APIKeyResolveInput,
		) (*llm.AuthResult, error) {
			serverURL, err := resolveLlamaServerURL(
				ctx,
				input.Context,
				input.Credential,
			)
			if err != nil || serverURL == "" {
				return nil, err
			}
			apiKey := ""
			if input.Credential != nil {
				apiKey = strings.TrimSpace(input.Credential.Key)
			}
			if apiKey == "" {
				apiKey, err = resolveLlamaEnvironment(
					ctx,
					input.Context,
					"LLAMA_API_KEY",
				)
				if err != nil {
					return nil, err
				}
			}
			if apiKey == "" {
				apiKey = "local"
			}
			inferenceURL, err := LlamaInferenceURL(serverURL)
			if err != nil {
				return nil, err
			}
			env := llm.ProviderEnv{}
			if input.Credential != nil {
				for name, value := range input.Credential.Env {
					env[name] = value
				}
			}
			env["LLAMA_BASE_URL"] = serverURL
			source := "LLAMA_BASE_URL"
			if input.Credential != nil {
				source = "stored credential"
			}
			return &llm.AuthResult{
				Auth: llm.ModelAuth{
					APIKey:  apiKey,
					BaseURL: inferenceURL,
				},
				Env:    env,
				Source: source,
			}, nil
		},
	}
}

func (c *LlamaProviderController) refreshModels(
	ctx context.Context,
	input llm.RefreshModelsContext,
) error {
	ctx = llamaContext(ctx)
	if !input.AllowNetwork ||
		input.Credential == nil ||
		input.Credential.Type != llm.CredentialTypeAPIKey {
		return ctx.Err()
	}
	serverURL, err := llamaCredentialServerURL(input.Credential)
	if err != nil || serverURL == "" {
		return err
	}
	client, err := NewLlamaClient(
		serverURL,
		input.Credential.Key,
		c.clientOptions,
	)
	if err != nil {
		return err
	}
	catalog, err := client.List(ctx, LlamaListOptions{})
	if err != nil {
		return err
	}
	return c.SetCatalog(catalog, serverURL)
}

func llamaCredentialServerURL(
	credential *llm.Credential,
) (string, error) {
	if credential == nil {
		return "", nil
	}
	value := strings.TrimSpace(credential.Env["LLAMA_BASE_URL"])
	if value == "" {
		return "", nil
	}
	return NormalizeLlamaServerURL(value)
}

func resolveLlamaServerURL(
	ctx context.Context,
	authContext llm.AuthContext,
	credential *llm.Credential,
) (string, error) {
	serverURL, err := llamaCredentialServerURL(credential)
	if err != nil || serverURL != "" {
		return serverURL, err
	}
	value, err := resolveLlamaEnvironment(
		ctx,
		authContext,
		"LLAMA_BASE_URL",
	)
	if err != nil || value == "" {
		return "", err
	}
	return NormalizeLlamaServerURL(value)
}

func resolveLlamaEnvironment(
	ctx context.Context,
	authContext llm.AuthContext,
	name string,
) (string, error) {
	if authContext == nil {
		authContext = llm.DefaultProviderAuthContext()
	}
	value, ok, err := authContext.Env(llamaContext(ctx), name)
	if err != nil || !ok {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func llamaProviderModel(
	model LlamaModelInfo,
	serverURL string,
) (llm.Model, error) {
	inferenceURL, err := LlamaInferenceURL(serverURL)
	if err != nil {
		return llm.Model{}, err
	}
	contextWindow := 0
	if model.Meta.ContextWindow != nil {
		contextWindow = *model.Meta.ContextWindow
	} else if model.Meta.TrainingContext != nil {
		contextWindow = *model.Meta.TrainingContext
	}
	if contextWindow <= 0 {
		contextWindow = 128_000
	}
	input := []string{"text"}
	for _, modality := range model.Architecture.InputModalities {
		if modality == "image" {
			input = append(input, "image")
			break
		}
	}
	return llm.Model{
		ID:            model.ID,
		Name:          model.ID,
		API:           "openai-completions",
		Provider:      LlamaProviderID,
		BaseURL:       inferenceURL,
		Reasoning:     false,
		Input:         input,
		Cost:          llm.ModelCost{},
		ContextWindow: contextWindow,
		MaxTokens:     contextWindow,
		Compat: llm.ModelCompat{
			SupportsStore:            llamaBoolPointer(false),
			SupportsDeveloperRole:    llamaBoolPointer(false),
			SupportsReasoningEffort:  llamaBoolPointer(false),
			SupportsUsageInStreaming: llamaBoolPointer(false),
			SupportsStrictMode:       llamaBoolPointer(false),
			MaxTokensField:           "max_tokens",
		},
	}, nil
}

func llamaBoolPointer(value bool) *bool {
	return &value
}

func cloneLlamaModelCatalog(
	catalog []LlamaModelInfo,
) []LlamaModelInfo {
	if catalog == nil {
		return nil
	}
	cloned := make([]LlamaModelInfo, len(catalog))
	for index, model := range catalog {
		cloned[index] = cloneLlamaModelInfo(model)
	}
	return cloned
}

func cloneLlamaModelInfo(model LlamaModelInfo) LlamaModelInfo {
	model.Aliases = append([]string(nil), model.Aliases...)
	model.Status.Args = append([]string(nil), model.Status.Args...)
	if model.Status.ExitCode != nil {
		exitCode := *model.Status.ExitCode
		model.Status.ExitCode = &exitCode
	}
	model.Status.Progress = cloneLlamaTransferProgress(
		model.Status.Progress,
	)
	model.Architecture.InputModalities = append(
		[]string(nil),
		model.Architecture.InputModalities...,
	)
	model.Architecture.OutputModalities = append(
		[]string(nil),
		model.Architecture.OutputModalities...,
	)
	if model.Meta.ContextWindow != nil {
		contextWindow := *model.Meta.ContextWindow
		model.Meta.ContextWindow = &contextWindow
	}
	if model.Meta.TrainingContext != nil {
		trainingContext := *model.Meta.TrainingContext
		model.Meta.TrainingContext = &trainingContext
	}
	if model.Meta.Size != nil {
		size := *model.Meta.Size
		model.Meta.Size = &size
	}
	return model
}
