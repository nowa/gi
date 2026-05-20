package gicodingagent

import (
	"context"
	"errors"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type agentSessionPrintModeHost struct {
	session *AgentSession
}

func runCLIPrintMode(args Args, options CLIOptions) int {
	promptArgs := args
	initial, err := buildCLIPrintModeInitialMessage(&promptArgs)
	if err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}

	factory := options.PrintModeHostFactory
	if factory == nil {
		factory = func(args Args) (PrintModeRuntimeHost, error) {
			return newDefaultCLIPrintModeHost(args, options)
		}
	}
	host, err := factory(args)
	if err != nil {
		writeCLIError(options.Stderr, err.Error())
		return 1
	}
	if host == nil {
		writeCLIError(options.Stderr, "print mode host is required")
		return 1
	}

	mode := string(args.Mode)
	if mode == "" {
		mode = "text"
	}
	return RunPrintMode(host, PrintModeOptions{
		Mode:           mode,
		InitialMessage: initial.message,
		InitialImages:  initial.images,
		Messages:       promptArgs.Messages,
		Stdout:         options.Stdout,
		Stderr:         options.Stderr,
	})
}

type cliPrintModeInitialMessage struct {
	message string
	images  []llm.ContentPart
}

func buildCLIPrintModeInitialMessage(args *Args) (cliPrintModeInitialMessage, error) {
	var fileText string
	var fileImages []llm.ContentPart
	if args != nil && len(args.FileArgs) > 0 {
		processed, err := ProcessFileArguments(args.FileArgs)
		if err != nil {
			return cliPrintModeInitialMessage{}, err
		}
		fileText = processed.Text
		fileImages = processed.Images
	}
	fileImageValues := make([]any, 0, len(fileImages))
	for _, image := range fileImages {
		fileImageValues = append(fileImageValues, image)
	}
	initial := BuildInitialMessage(InitialMessageInput{
		Parsed:     args,
		FileText:   fileText,
		FileImages: fileImageValues,
	})
	return cliPrintModeInitialMessage{
		message: initial.InitialMessage,
		images:  contentPartsFromInitialImages(initial.InitialImages),
	}, nil
}

func contentPartsFromInitialImages(values []any) []llm.ContentPart {
	if len(values) == 0 {
		return nil
	}
	parts := make([]llm.ContentPart, 0, len(values))
	for _, value := range values {
		if part, ok := value.(llm.ContentPart); ok {
			parts = append(parts, part)
		}
	}
	return parts
}

func newDefaultCLIPrintModeHost(args Args, options CLIOptions) (PrintModeRuntimeHost, error) {
	writeAuth := !args.Offline || args.APIKey != ""
	modelRegistry, cwd, agentDir, err := newCLIModelRegistry(options, writeAuth)
	if err != nil {
		return nil, err
	}
	model, thinkingLevel, err := resolveCLIPrintModeModel(args, modelRegistry)
	if err != nil {
		return nil, err
	}
	if model == nil {
		return nil, errors.New("No model available. Configure provider auth or pass --model with --api-key.")
	}
	if args.APIKey != "" && modelRegistry.authStorage != nil {
		modelRegistry.authStorage.SetRuntimeAPIKey(model.Provider, args.APIKey)
	}

	var sessionManager *SessionManager
	if args.NoSession {
		var err error
		sessionManager, err = InMemorySessionManager(cwd)
		if err != nil {
			return nil, err
		}
	}

	host := &agentSessionPrintModeHost{}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            cwd,
		AgentDir:       agentDir,
		SessionManager: sessionManager,
		Model:          *model,
	})
	if err != nil {
		return nil, err
	}
	session.Agent.State.ThinkingLevel = string(thinkingLevel)
	if args.Offline {
		session.Responder = DefaultAgentSessionResponder
	} else {
		session.Responder = host.providerResponder(modelRegistry, args)
	}
	host.session = session
	return host, nil
}

func resolveCLIPrintModeModel(args Args, registry CodingModelRegistry) (*llm.Model, ThinkingLevel, error) {
	if registry == nil {
		return nil, DefaultThinkingLevel, errors.New("model registry is required")
	}
	if args.Model != "" {
		resolved := ResolveCLIModel(ResolveCLIModelOptions{
			CLIProvider:   args.Provider,
			CLIModel:      args.Model,
			ModelRegistry: registry,
		})
		if resolved.Error != "" {
			return nil, DefaultThinkingLevel, errors.New(resolved.Error)
		}
		level := firstThinkingLevel(resolved.ThinkingLevel, args.Thinking, DefaultThinkingLevel)
		return resolved.Model, level, nil
	}

	defaultModelID := ""
	if args.Provider != "" {
		defaultModelID = DefaultModelPerProvider[args.Provider]
	}
	resolved := FindInitialModel(FindInitialModelOptions{
		CLIProvider:          args.Provider,
		DefaultProvider:      args.Provider,
		DefaultModelID:       defaultModelID,
		DefaultThinkingLevel: firstThinkingLevel(args.Thinking, DefaultThinkingLevel),
		ModelRegistry:        registry,
	})
	if resolved.Error != "" {
		return nil, resolved.ThinkingLevel, errors.New(resolved.Error)
	}
	return resolved.Model, resolved.ThinkingLevel, nil
}

func firstThinkingLevel(levels ...ThinkingLevel) ThinkingLevel {
	for _, level := range levels {
		if level != "" {
			return level
		}
	}
	return ""
}

func (h *agentSessionPrintModeHost) PrintModeSession() PrintModeSession {
	return h
}

func (h *agentSessionPrintModeHost) Dispose() error {
	if h != nil && h.session != nil {
		h.session.Dispose()
	}
	return nil
}

func (h *agentSessionPrintModeHost) Prompt(message string, options PrintModePromptOptions) error {
	if h == nil || h.session == nil {
		return errors.New("session is required")
	}
	if len(options.Images) == 0 {
		return h.session.Prompt(message)
	}
	// AgentSession currently stores text user turns. Preserve the visible text
	// path until the session message model grows first-class image turns.
	return h.session.Prompt(message)
}

func (h *agentSessionPrintModeHost) WaitForIdle() error {
	return nil
}

func (h *agentSessionPrintModeHost) Messages() []llm.Message {
	if h == nil || h.session == nil {
		return nil
	}
	return h.session.Messages()
}

func (h *agentSessionPrintModeHost) providerResponder(registry *ModelRegistry, args Args) AgentSessionResponder {
	return func(_ string, messages []llm.Message, model llm.Model) (llm.Message, error) {
		auth := registry.GetAPIKeyAndHeaders(model)
		if !auth.OK {
			return llm.Message{}, errors.New(auth.Error)
		}
		options := llm.SimpleStreamOptions{
			APIKey:    firstNonEmptyString(args.APIKey, auth.APIKey),
			Headers:   auth.Headers,
			Reasoning: string(firstThinkingLevel(args.Thinking, ThinkingLevel(h.session.Agent.State.ThinkingLevel))),
		}
		return llm.CompleteSimple(context.Background(), model, llm.Context{
			SystemPrompt: h.session.SystemPrompt,
			Messages:     messages,
		}, options)
	}
}
