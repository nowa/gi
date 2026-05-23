package gicodingagent

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestRunCLIPrintModeUsesInjectedHost(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var factoryArgs Args
	host := newFakePrintModeHost(llm.Message{
		Role:       llm.RoleAssistant,
		Content:    []llm.ContentPart{llm.Text("done")},
		StopReason: llm.StopReasonStop,
	})

	code := RunCLI(CLIOptions{
		Args:   []string{"-p", "hello"},
		Stdout: &stdout,
		Stderr: &stderr,
		PrintModeHostFactory: func(args Args) (PrintModeRuntimeHost, error) {
			factoryArgs = args
			return host, nil
		},
	})

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != "done" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if !factoryArgs.Print || len(factoryArgs.Messages) != 1 || factoryArgs.Messages[0] != "hello" {
		t.Fatalf("factory args = %#v", factoryArgs)
	}
	if len(host.session.prompts) != 1 || host.session.prompts[0].message != "hello" {
		t.Fatalf("prompts = %#v", host.session.prompts)
	}
}

func TestRunCLIPrintsStartupTimingsWhenEnabled(t *testing.T) {
	t.Setenv("GI_TIMING", "1")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := RunCLI(CLIOptions{
		Args:   []string{"--help"},
		Stdout: &stdout,
		Stderr: &stderr,
		CWD:    t.TempDir(),
	})

	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "Usage: gi") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	output := stderr.String()
	for _, expected := range []string{"--- Startup Timings ---", "parse args:", "startup migrations:", "dispatch help:", "TOTAL:"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("stderr = %q, want %q", output, expected)
		}
	}
}

func TestRunCLIHelpListsProtocolExtensionFlagsPiStyle(t *testing.T) {
	cwd := t.TempDir()
	agentDir := filepath.Join(t.TempDir(), "agent")
	extensionPath := filepath.Join(t.TempDir(), "review.gi.json")
	writeJSON(t, extensionPath, map[string]any{"gi": map[string]any{
		"extensionProtocol": "descriptor.v1",
		"flags": []any{
			map[string]any{"name": "review-mode", "description": "Review mode", "type": "boolean"},
			map[string]any{"name": "profile", "description": "Profile name", "type": "string"},
		},
	}})
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := RunCLI(CLIOptions{
		Args:     []string{"--help", "-e", extensionPath},
		Stdout:   &stdout,
		Stderr:   &stderr,
		CWD:      cwd,
		AgentDir: agentDir,
	})

	if code != 0 {
		t.Fatalf("exit code = %d stderr=%q", code, stderr.String())
	}
	for _, want := range []string{"Extension CLI Flags:", "--review-mode", "Review mode", "--profile <value>", "Profile name"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestCLIPrintModeUsesSelectedSessionCwdForRuntimeResourcesPiStyle(t *testing.T) {
	startupCwd := t.TempDir()
	targetCwd := t.TempDir()
	agentDir := filepath.Join(t.TempDir(), "agent")
	sessionDir := filepath.Join(t.TempDir(), "sessions")
	writeResourceSkill(t, filepath.Join(startupCwd, ConfigDirName, "skills", "startup", "SKILL.md"), "startup", "Startup skill", "Use startup.")
	writeResourceSkill(t, filepath.Join(targetCwd, ConfigDirName, "skills", "target", "SKILL.md"), "target", "Target skill", "Use target.")
	manager, err := CreateSessionManager(targetCwd, sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	manager.AppendMessage(llm.Message{Role: llm.RoleUser, Content: []llm.ContentPart{llm.Text("from target")}})
	if err := manager.rewriteFile(); err != nil {
		t.Fatal(err)
	}

	host, err := newDefaultCLIPrintModeHost(Args{
		Offline:    true,
		Model:      "openai/gpt-4o-mini",
		Session:    manager.GetSessionFile(),
		SessionDir: sessionDir,
	}, CLIOptions{CWD: startupCwd, AgentDir: agentDir})
	if err != nil {
		t.Fatal(err)
	}
	session := host.(*agentSessionPrintModeHost).session
	if got := session.SessionManager.GetCWD(); got != targetCwd {
		t.Fatalf("session cwd = %q, want %q", got, targetCwd)
	}
	if !strings.Contains(session.SystemPrompt, "Target skill") {
		t.Fatalf("system prompt missing target skill: %q", session.SystemPrompt)
	}
	if strings.Contains(session.SystemPrompt, "Startup skill") {
		t.Fatalf("system prompt used startup resources: %q", session.SystemPrompt)
	}
}

func TestCLIPrintModePassesResourceFlagsToLoaderPiStyle(t *testing.T) {
	cwd := t.TempDir()
	agentDir := filepath.Join(t.TempDir(), "agent")
	writeResourceFile(t, filepath.Join(cwd, "AGENTS.md"), "Project context should be disabled")
	writeResourceSkill(t, filepath.Join(agentDir, "skills", "default.md"), "default", "Default skill", "Default skill body")
	customSkills := filepath.Join(t.TempDir(), "custom-skills")
	writeResourceSkill(t, filepath.Join(customSkills, "custom.md"), "custom", "Custom skill", "Custom skill body")

	host, err := newDefaultCLIPrintModeHost(Args{
		Offline:            true,
		Model:              "openai/gpt-4o-mini",
		NoContextFiles:     true,
		NoSkills:           true,
		Skills:             []string{customSkills},
		SystemPrompt:       "CLI system prompt",
		AppendSystemPrompt: []string{"CLI append"},
	}, CLIOptions{CWD: cwd, AgentDir: agentDir})
	if err != nil {
		t.Fatal(err)
	}
	session := host.(*agentSessionPrintModeHost).session
	for _, want := range []string{"CLI system prompt", "CLI append", "Custom skill body"} {
		if !strings.Contains(session.SystemPrompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, session.SystemPrompt)
		}
	}
	for _, unwanted := range []string{"Project context should be disabled", "Default skill body"} {
		if strings.Contains(session.SystemPrompt, unwanted) {
			t.Fatalf("system prompt contains disabled resource %q:\n%s", unwanted, session.SystemPrompt)
		}
	}
}

func TestCLIPrintModeSelectedSessionMissingCwdErrorsPiStyle(t *testing.T) {
	startupCwd := t.TempDir()
	missingCwd := filepath.Join(t.TempDir(), "missing-project")
	agentDir := filepath.Join(t.TempDir(), "agent")
	sessionDir := filepath.Join(t.TempDir(), "sessions")
	manager, err := CreateSessionManager(missingCwd, sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	manager.AppendMessage(llm.Message{Role: llm.RoleUser, Content: []llm.ContentPart{llm.Text("from missing")}})
	if err := manager.rewriteFile(); err != nil {
		t.Fatal(err)
	}

	_, err = newDefaultCLIPrintModeHost(Args{
		Offline:    true,
		Model:      "openai/gpt-4o-mini",
		Session:    manager.GetSessionFile(),
		SessionDir: sessionDir,
	}, CLIOptions{CWD: startupCwd, AgentDir: agentDir})
	var missingErr MissingSessionCwdError
	if !errors.As(err, &missingErr) {
		t.Fatalf("err = %T %v, want MissingSessionCwdError", err, err)
	}
	if missingErr.Issue.SessionCwd != missingCwd || missingErr.Issue.FallbackCwd != startupCwd {
		t.Fatalf("missing cwd issue = %#v", missingErr.Issue)
	}

	host, err := newDefaultCLIPrintModeHost(Args{
		Offline:            true,
		Model:              "openai/gpt-4o-mini",
		Session:            manager.GetSessionFile(),
		SessionDir:         sessionDir,
		SessionCwdOverride: startupCwd,
	}, CLIOptions{CWD: startupCwd, AgentDir: agentDir})
	if err != nil {
		t.Fatal(err)
	}
	if got := host.(*agentSessionPrintModeHost).session.SessionManager.GetCWD(); got != startupCwd {
		t.Fatalf("overridden session cwd = %q, want %q", got, startupCwd)
	}
}

func TestRunCLIReportsArgumentDiagnosticsPiStyle(t *testing.T) {
	t.Run("warnings are printed and execution continues", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		code := RunCLI(CLIOptions{
			Args:   []string{"--thinking", "random", "--version"},
			Stdout: &stdout,
			Stderr: &stderr,
		})

		if code != 0 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
		}
		if stdout.String() != DefaultCodingAgentVersion+"\n" {
			t.Fatalf("stdout = %q", stdout.String())
		}
		if !strings.Contains(stderr.String(), `Warning: Invalid thinking level "random"`) {
			t.Fatalf("stderr = %q", stderr.String())
		}
	})

	t.Run("version prints configured package version", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		code := RunCLI(CLIOptions{
			Args:    []string{"--version"},
			Stdout:  &stdout,
			Stderr:  &stderr,
			Version: "1.2.3",
		})

		if code != 0 || stdout.String() != "1.2.3\n" || stderr.String() != "" {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	})

	t.Run("errors are printed and execution stops", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		startupRan := false

		code := RunCLI(CLIOptions{
			Args:   []string{"-unknown", "--version"},
			Stdout: &stdout,
			Stderr: &stderr,
			Startup: func(io.Writer) error {
				startupRan = true
				return nil
			},
		})

		if code != 1 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
		}
		if startupRan {
			t.Fatal("startup should not run after argument errors")
		}
		if stdout.String() != "" {
			t.Fatalf("stdout = %q", stdout.String())
		}
		if !strings.Contains(stderr.String(), "Error: Unknown option: -unknown") {
			t.Fatalf("stderr = %q", stderr.String())
		}
	})
}

func TestRunCLIDefaultInteractiveModeUsesInjectedHost(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var factoryArgs Args
	host := &fakeCLIInteractiveRuntimeHost{}

	code := RunCLI(CLIOptions{
		Args:   []string{"hello"},
		Stdout: &stdout,
		Stderr: &stderr,
		InteractiveHostFactory: func(args Args) (CLIInteractiveRuntimeHost, error) {
			factoryArgs = args
			return host, nil
		},
	})

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !host.ran {
		t.Fatal("interactive host was not run")
	}
	if len(factoryArgs.Messages) != 1 || factoryArgs.Messages[0] != "hello" {
		t.Fatalf("factory args = %#v", factoryArgs)
	}
	if stdout.String() != "" || stderr.String() != "" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunCLIDefaultInteractiveModeRunsBasicHostOffline(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	tempDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := RunCLI(CLIOptions{
		Args:     []string{"--offline", "--no-session", "--model", "openai/gpt-4o-mini", "hello"},
		Stdin:    strings.NewReader(""),
		Stdout:   &stdout,
		Stderr:   &stderr,
		CWD:      tempDir,
		AgentDir: filepath.Join(tempDir, "agent"),
	})

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != "Response to: hello" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunCLIDefaultInteractiveModeUsesPipedStdinAsPrompt(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	tempDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := RunCLI(CLIOptions{
		Args:     []string{"--offline", "--no-session", "--model", "openai/gpt-4o-mini"},
		Stdin:    strings.NewReader("hello from stdin\n"),
		Stdout:   &stdout,
		Stderr:   &stderr,
		CWD:      tempDir,
		AgentDir: filepath.Join(tempDir, "agent"),
	})

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != "Response to: hello from stdin" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunCLIDefaultInteractiveModeRequiresInputUntilTUIHost(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := RunCLI(CLIOptions{
		Args:   []string{},
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	})

	if code != 1 {
		t.Fatalf("exit code = %d, stdout = %q stderr = %q", code, stdout.String(), stderr.String())
	}
	if stdout.String() != "" ||
		!strings.Contains(stderr.String(), "Interactive input is required") ||
		!strings.Contains(stderr.String(), "full TUI host") {
		t.Fatalf("stdout = %q stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunCLIJSONModeUsesPrintMode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	host := newFakePrintModeHost(llm.Message{
		Role:       llm.RoleAssistant,
		Content:    []llm.ContentPart{llm.Text("done")},
		StopReason: llm.StopReasonStop,
	})

	code := RunCLI(CLIOptions{
		Args:   []string{"--mode", "json", "hello"},
		Stdout: &stdout,
		Stderr: &stderr,
		PrintModeHostFactory: func(Args) (PrintModeRuntimeHost, error) {
			return host, nil
		},
	})

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"role":"assistant"`) || !strings.Contains(stdout.String(), `"done"`) {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if len(host.session.prompts) != 1 || host.session.prompts[0].message != "hello" {
		t.Fatalf("prompts = %#v", host.session.prompts)
	}
}

type fakeCLIInteractiveRuntimeHost struct {
	ran bool
	err error
}

func (h *fakeCLIInteractiveRuntimeHost) Run() error {
	h.ran = true
	return h.err
}

func TestRunCLIPrintModeMergesPipedStdinLikePi(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	host := newFakePrintModeHost(llm.Message{
		Role:       llm.RoleAssistant,
		Content:    []llm.ContentPart{llm.Text("done")},
		StopReason: llm.StopReasonStop,
	})

	code := RunCLI(CLIOptions{
		Args:   []string{"-p", "Summarize"},
		Stdin:  strings.NewReader("README contents\n\n"),
		Stdout: &stdout,
		Stderr: &stderr,
		PrintModeHostFactory: func(Args) (PrintModeRuntimeHost, error) {
			return host, nil
		},
	})

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if len(host.session.prompts) != 1 || host.session.prompts[0].message != "README contentsSummarize" {
		t.Fatalf("prompts = %#v", host.session.prompts)
	}
}

func TestRunCLIPrintModeFactoryError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := RunCLI(CLIOptions{
		Args:   []string{"-p", "hello"},
		Stdout: &stdout,
		Stderr: &stderr,
		PrintModeHostFactory: func(Args) (PrintModeRuntimeHost, error) {
			return nil, errors.New("factory failed")
		},
	})

	if code != 1 {
		t.Fatalf("exit code = %d", code)
	}
	if stdout.String() != "" || stderr.String() != "factory failed\n" {
		t.Fatalf("stdout = %q stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunCLIDefaultOfflinePrintMode(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "agent")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := RunCLI(CLIOptions{
		Args:     []string{"--offline", "--no-session", "--model", "openai/gpt-4o-mini", "-p", "hello"},
		Stdout:   &stdout,
		Stderr:   &stderr,
		CWD:      tempDir,
		AgentDir: agentDir,
	})

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != "Response to: hello" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if _, err := os.Stat(agentDir); !os.IsNotExist(err) {
		t.Fatalf("agent dir stat err = %v, want not exist", err)
	}
}

func TestRunCLIPrintModeMissingAPIKeyUsesAuthGuidance(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	tempDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := RunCLI(CLIOptions{
		Args:     []string{"-p", "--model", "openai/gpt-4o-mini", "hello"},
		Stdout:   &stdout,
		Stderr:   &stderr,
		CWD:      tempDir,
		AgentDir: filepath.Join(tempDir, "agent"),
	})

	if code != 1 {
		t.Fatalf("exit code = %d, stdout = %q stderr = %q", code, stdout.String(), stderr.String())
	}
	output := stderr.String()
	for _, expected := range []string{"No API key found for openai.", "OPENAI_API_KEY", "--api-key", "~/.gi/agent/auth.json", "gi --list-models"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("stderr = %q, want %q", output, expected)
		}
	}
	if strings.Contains(output, "missing API key") {
		t.Fatalf("stderr leaked provider-level error: %q", output)
	}
}

func TestRunCLIPrintModeAppliesInstallTelemetryAttributionHeaders(t *testing.T) {
	clearInstallTelemetryEnv(t)
	t.Setenv("GI_TELEMETRY", "true")
	tempDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var captured llm.SimpleStreamOptions

	registry := NewModelRegistry(NewInMemoryAuthStorage(nil), "")
	if err := registry.RegisterProvider("custom-openrouter", ProviderConfigInput{
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  "test-key",
		API:     "test-openrouter-attribution",
		StreamSimple: func(_ llm.Model, _ llm.Context, options llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
			captured = options
			return llm.CompletedAssistantStream(llm.FauxAssistantText("ok")), nil
		},
		Models: []ProviderModelDefinition{{
			ID:            "test-model",
			Name:          "Test Model",
			ContextWindow: 1000,
			MaxTokens:     100,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		registry.UnregisterProvider("custom-openrouter")
	})

	code := RunCLI(CLIOptions{
		Args:          []string{"-p", "--model", "custom-openrouter/test-model", "hello"},
		Stdout:        &stdout,
		Stderr:        &stderr,
		CWD:           tempDir,
		AgentDir:      filepath.Join(tempDir, "agent"),
		ModelRegistry: registry,
	})

	if code != 0 {
		t.Fatalf("exit code = %d, stdout = %q stderr = %q", code, stdout.String(), stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "ok" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if captured.Headers["HTTP-Referer"] != "https://github.com/nowa/gi" ||
		captured.Headers["X-OpenRouter-Title"] != "gi" {
		t.Fatalf("headers = %#v", captured.Headers)
	}
}

func TestRunCLIPrintModeSettingsCanDisableInstallTelemetryAttribution(t *testing.T) {
	clearInstallTelemetryEnv(t)
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "agent")
	writeSettingsJSON(t, filepath.Join(agentDir, "settings.json"), map[string]any{"enableInstallTelemetry": false})
	var captured llm.SimpleStreamOptions

	registry := NewModelRegistry(NewInMemoryAuthStorage(nil), "")
	if err := registry.RegisterProvider("custom-openrouter-no-telemetry", ProviderConfigInput{
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  "test-key",
		API:     "test-openrouter-no-telemetry",
		StreamSimple: func(_ llm.Model, _ llm.Context, options llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
			captured = options
			return llm.CompletedAssistantStream(llm.FauxAssistantText("ok")), nil
		},
		Models: []ProviderModelDefinition{{
			ID:            "test-model",
			Name:          "Test Model",
			ContextWindow: 1000,
			MaxTokens:     100,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		registry.UnregisterProvider("custom-openrouter-no-telemetry")
	})

	code := RunCLI(CLIOptions{
		Args:          []string{"-p", "--model", "custom-openrouter-no-telemetry/test-model", "hello"},
		Stdout:        &bytes.Buffer{},
		Stderr:        &bytes.Buffer{},
		CWD:           tempDir,
		AgentDir:      agentDir,
		ModelRegistry: registry,
	})

	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if captured.Headers != nil {
		t.Fatalf("headers = %#v, want nil", captured.Headers)
	}
}

func TestAgentSessionPrintModeProviderResponderEmitsProviderLifecycleHooks(t *testing.T) {
	tempDir := t.TempDir()
	var providerSawPayload any
	var providerHeadersAfterHook map[string]string
	beforeCalled := false
	afterCalled := false

	registry := NewModelRegistry(NewInMemoryAuthStorage(nil), "")
	if err := registry.RegisterProvider("hook-provider", ProviderConfigInput{
		BaseURL: "https://example.invalid/v1",
		APIKey:  "test-key",
		API:     "test-provider-lifecycle-hooks",
		StreamSimple: func(model llm.Model, _ llm.Context, options llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
			if options.OnPayload == nil {
				t.Fatal("OnPayload hook is nil")
			}
			if options.OnResponseStatus == nil {
				t.Fatal("OnResponseStatus hook is nil")
			}
			next, replace, err := options.OnPayload(map[string]any{"prompt": "original"}, model)
			if err != nil {
				t.Fatal(err)
			}
			if !replace {
				t.Fatal("OnPayload replace = false, want true")
			}
			providerSawPayload = next

			headers := map[string]string{"X-Test": "ok"}
			if err := options.OnResponseStatus(202, headers, model); err != nil {
				t.Fatal(err)
			}
			providerHeadersAfterHook = headers

			return llm.CompletedAssistantStream(llm.Message{
				Role:       llm.RoleAssistant,
				Content:    []llm.ContentPart{llm.Text("hooked")},
				API:        model.API,
				Provider:   model.Provider,
				Model:      model.ID,
				StopReason: llm.StopReasonStop,
			}), nil
		},
		Models: []ProviderModelDefinition{{
			ID:            "hook-model",
			Name:          "Hook Model",
			ContextWindow: 1000,
			MaxTokens:     100,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		registry.UnregisterProvider("hook-provider")
	})
	model, ok := registry.Find("hook-provider", "hook-model")
	if !ok {
		t.Fatal("registered model not found")
	}

	manager, err := InMemorySessionManager(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            tempDir,
		AgentDir:       filepath.Join(tempDir, "agent"),
		Model:          model,
		SessionManager: manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{Path: "provider-hooks.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
		if err := ctx.On(ProtocolEventBeforeProviderRequest, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			beforeCalled = true
			if event.Context == nil {
				t.Fatal("before_provider_request context is nil")
			}
			if event.Model == nil || event.Model.Provider != "hook-provider" || event.Model.ID != "hook-model" {
				t.Fatalf("before_provider_request model = %#v", event.Model)
			}
			payload, ok := event.Payload.(map[string]any)
			if !ok || payload["prompt"] != "original" {
				t.Fatalf("before_provider_request payload = %#v", event.Payload)
			}
			return ProtocolEventResult{
				Payload:    map[string]any{"prompt": "mutated"},
				PayloadSet: true,
			}, nil
		}); err != nil {
			return err
		}
		return ctx.On(ProtocolEventAfterProviderResponse, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			afterCalled = true
			if event.Context == nil {
				t.Fatal("after_provider_response context is nil")
			}
			if event.Model == nil || event.Model.Provider != "hook-provider" || event.Model.ID != "hook-model" {
				t.Fatalf("after_provider_response model = %#v", event.Model)
			}
			if event.Status != 202 || event.Headers["X-Test"] != "ok" {
				t.Fatalf("after_provider_response status/headers = %d %#v", event.Status, event.Headers)
			}
			event.Headers["X-Test"] = "changed"
			return ProtocolEventResult{}, nil
		})
	}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAgentSessionRuntimeHost(session, runtime); err != nil {
		t.Fatal(err)
	}

	host := &agentSessionPrintModeHost{session: session, modelRegistry: registry}
	result, err := host.providerResponder(registry, Args{}, false)("hello", []llm.Message{llm.UserMessageText("hi")}, model)
	if err != nil {
		t.Fatal(err)
	}

	if !beforeCalled || !afterCalled {
		t.Fatalf("hooks called before=%v after=%v", beforeCalled, afterCalled)
	}
	payload, ok := providerSawPayload.(map[string]any)
	if !ok || payload["prompt"] != "mutated" {
		t.Fatalf("provider payload = %#v", providerSawPayload)
	}
	if providerHeadersAfterHook["X-Test"] != "ok" {
		t.Fatalf("provider headers mutated by event handler: %#v", providerHeadersAfterHook)
	}
	if result.Provider != "hook-provider" || result.Model != "hook-model" || result.Content[0].Text != "hooked" {
		t.Fatalf("result = %#v", result)
	}
}

func TestResolveCLIPrintModeModelClampsThinkingToModelCapabilities(t *testing.T) {
	registry := resolverTestRegistry{
		all:       resolverMockModels,
		available: resolverMockModels,
	}

	model, level, err := resolveCLIPrintModeModel(Args{Model: "openai/gpt-4o"}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if model == nil || model.Provider != "openai" || level != ThinkingOff {
		t.Fatalf("model=%#v level=%q, want openai off", model, level)
	}

	model, level, err = resolveCLIPrintModeModel(Args{Model: "openai/gpt-4o", Thinking: ThinkingXHigh}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if model == nil || model.Provider != "openai" || level != ThinkingOff {
		t.Fatalf("explicit xhigh model=%#v level=%q, want off", model, level)
	}

	model, level, err = resolveCLIPrintModeModel(Args{Model: "anthropic/claude-sonnet-4-5"}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if model == nil || model.Provider != "anthropic" || level != ThinkingMedium {
		t.Fatalf("model=%#v level=%q, want anthropic medium", model, level)
	}

	model, level, err = resolveCLIPrintModeModel(Args{Model: "anthropic/claude-sonnet-4-5:high", Thinking: ThinkingLow}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if model == nil || model.Provider != "anthropic" || level != ThinkingLow {
		t.Fatalf("explicit --thinking model=%#v level=%q, want anthropic low", model, level)
	}
}

func TestResolveCLIPrintModeModelRestoresSessionModelPiStyle(t *testing.T) {
	registry := resolverTestRegistry{
		all:       resolverMockModels,
		available: resolverMockModels,
	}
	settings := NewInMemorySettingsManager(map[string]any{
		"defaultProvider":      "openai",
		"defaultModel":         "gpt-4o",
		"defaultThinkingLevel": "medium",
	})
	manager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager.AppendModelChange("anthropic", "claude-sonnet-4-5")
	manager.AppendThinkingLevelChange("high")
	manager.AppendMessage(llm.UserMessageText("continue this"))

	resolved, err := resolveCLIPrintModeModelForSession(Args{Continue: true}, registry, settings, manager)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Model == nil || resolved.Model.Provider != "anthropic" || resolved.Model.ID != "claude-sonnet-4-5" {
		t.Fatalf("restored model = %#v", resolved.Model)
	}
	if resolved.ThinkingLevel != ThinkingHigh || resolved.Warning != "" {
		t.Fatalf("thinking/warning = %q / %q", resolved.ThinkingLevel, resolved.Warning)
	}
}

func TestResolveCLIPrintModeModelReportsSessionModelFallbackPiStyle(t *testing.T) {
	registry := resolverTestRegistry{
		all:       resolverMockModels,
		available: resolverMockModels,
	}
	settings := NewInMemorySettingsManager(map[string]any{
		"defaultProvider":      "openai",
		"defaultModel":         "gpt-4o",
		"defaultThinkingLevel": "medium",
	})
	manager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager.AppendModelChange("anthropic", "missing-model")
	manager.AppendMessage(llm.UserMessageText("continue this"))

	resolved, err := resolveCLIPrintModeModelForSession(Args{Continue: true}, registry, settings, manager)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Model == nil || resolved.Model.Provider != "openai" || resolved.Model.ID != "gpt-4o" {
		t.Fatalf("fallback model = %#v", resolved.Model)
	}
	for _, expected := range []string{"Could not restore model anthropic/missing-model", "not found", "Using openai/gpt-4o"} {
		if !strings.Contains(resolved.Warning, expected) {
			t.Fatalf("warning missing %q: %q", expected, resolved.Warning)
		}
	}
}

func TestRunCLIPrintModeOmitsReasoningForNonReasoningModel(t *testing.T) {
	tempDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var captured llm.SimpleStreamOptions

	registry := NewModelRegistry(NewInMemoryAuthStorage(nil), "")
	if err := registry.RegisterProvider("custom-openai", ProviderConfigInput{
		BaseURL: "https://example.invalid/v1",
		APIKey:  "test-key",
		API:     "test-non-reasoning",
		StreamSimple: func(_ llm.Model, _ llm.Context, options llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
			captured = options
			return llm.CompletedAssistantStream(llm.FauxAssistantText("ok")), nil
		},
		Models: []ProviderModelDefinition{{
			ID:            "gpt-4o-mini",
			Name:          "GPT-4o mini",
			ContextWindow: 128000,
			MaxTokens:     16384,
			Reasoning:     false,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	code := RunCLI(CLIOptions{
		Args:          []string{"-p", "--model", "custom-openai/gpt-4o-mini", "hello"},
		Stdout:        &stdout,
		Stderr:        &stderr,
		CWD:           tempDir,
		AgentDir:      filepath.Join(tempDir, "agent"),
		ModelRegistry: registry,
	})

	if code != 0 {
		t.Fatalf("exit code = %d, stdout = %q stderr = %q", code, stdout.String(), stderr.String())
	}
	if captured.Reasoning != "" {
		t.Fatalf("reasoning = %q, want empty for non-reasoning model", captured.Reasoning)
	}
	if stdout.String() != "ok" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestResolveCLIPrintModeModelUsesSettingsDefaults(t *testing.T) {
	registry := resolverTestRegistry{
		all:       resolverMockModels,
		available: resolverMockModels,
	}
	settings := NewInMemorySettingsManager(map[string]any{
		"defaultProvider":      "anthropic",
		"defaultModel":         "claude-sonnet-4-5",
		"defaultThinkingLevel": "high",
	})

	model, level, err := resolveCLIPrintModeModel(Args{}, registry, settings)
	if err != nil {
		t.Fatal(err)
	}
	if model == nil || model.Provider != "anthropic" || model.ID != "claude-sonnet-4-5" || level != ThinkingHigh {
		t.Fatalf("model=%#v level=%q, want settings default anthropic high", model, level)
	}
}

func TestResolveCLIPrintModeModelUsesModelsScope(t *testing.T) {
	registry := resolverTestRegistry{
		all:       resolverMockModels,
		available: resolverMockModels,
	}
	settings := NewInMemorySettingsManager(map[string]any{
		"defaultProvider": "anthropic",
		"defaultModel":    "claude-sonnet-4-5",
	})

	model, level, err := resolveCLIPrintModeModel(Args{
		Models: []string{"openai/gpt-4o", "anthropic/claude-sonnet-4-5:high"},
	}, registry, settings)
	if err != nil {
		t.Fatal(err)
	}
	if model == nil || model.Provider != "anthropic" || model.ID != "claude-sonnet-4-5" || level != ThinkingHigh {
		t.Fatalf("model=%#v level=%q, want scoped settings default anthropic high", model, level)
	}

	model, level, err = resolveCLIPrintModeModel(Args{
		Models: []string{"openai/gpt-4o"},
	}, registry, settings)
	if err != nil {
		t.Fatal(err)
	}
	if model == nil || model.Provider != "openai" || model.ID != "gpt-4o" || level != ThinkingOff {
		t.Fatalf("model=%#v level=%q, want first scoped openai off", model, level)
	}
}
