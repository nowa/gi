package gicodingagent

import (
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestAgentSessionDynamicProviderTopLevelOverride(t *testing.T) {
	host := createDynamicExtensionHost(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.RegisterProvider("anthropic", ProtocolProviderOverride{BaseURL: "http://localhost:8080/top-level"})
	})

	if got := host.Session.Agent.State.Model.BaseURL; got != "http://localhost:8080/top-level" {
		t.Fatalf("model baseURL = %q", got)
	}
	if got := capturePromptBaseURL(t, host.Session); got != "http://localhost:8080/top-level" {
		t.Fatalf("prompt baseURL = %q", got)
	}
}

func TestAgentSessionDynamicProviderSessionStartOverride(t *testing.T) {
	host := createDynamicExtensionHost(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.On(ProtocolEventSessionStart, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			return ProtocolEventResult{}, ctx.RegisterProvider("anthropic", ProtocolProviderOverride{BaseURL: "http://localhost:8080/session-start"})
		})
	})

	if got := host.Session.Agent.State.Model.BaseURL; got != "http://localhost:8080/session-start" {
		t.Fatalf("model baseURL = %q", got)
	}
	if got := capturePromptBaseURL(t, host.Session); got != "http://localhost:8080/session-start" {
		t.Fatalf("prompt baseURL = %q", got)
	}
}

func TestAgentSessionDynamicProviderCommandTimeOverride(t *testing.T) {
	host := createDynamicExtensionHost(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.RegisterCommand("use-proxy", ProtocolCommandDefinition{
			Description: "Use proxy",
			Handler: func(args string) error {
				return ctx.RegisterProvider("anthropic", ProtocolProviderOverride{BaseURL: "http://localhost:8080/command"})
			},
		})
	})

	if err := host.Session.Prompt("/use-proxy"); err != nil {
		t.Fatal(err)
	}
	if got := host.Session.Agent.State.Model.BaseURL; got != "http://localhost:8080/command" {
		t.Fatalf("model baseURL = %q", got)
	}
	if got := capturePromptBaseURL(t, host.Session); got != "http://localhost:8080/command" {
		t.Fatalf("prompt baseURL = %q", got)
	}
}

func TestAgentSessionDynamicToolRegistrationRefreshesSystemPrompt(t *testing.T) {
	host := createDynamicExtensionHost(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.On(ProtocolEventSessionStart, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			return ProtocolEventResult{}, ctx.RegisterTool(ProtocolToolDefinition{
				Name:             "dynamic_tool",
				Label:            "Dynamic Tool",
				Description:      "Tool registered from session_start",
				PromptSnippet:    "Run dynamic test behavior",
				PromptGuidelines: []string{"Use dynamic_tool when the user asks for dynamic behavior tests."},
				Execute: func(toolCallID string, input map[string]any) (SDKToolResult, error) {
					return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "ok"}}}, nil
				},
			})
		})
	})

	tools := host.Session.GetAllTools()
	dynamicTool := findDynamicSDKTool(tools, "dynamic_tool")
	readTool := findDynamicSDKTool(tools, "read")
	if dynamicTool == nil || readTool == nil {
		t.Fatalf("tools = %#v", tools)
	}
	if dynamicTool.SourceInfo.Path != "dynamic-extension" || dynamicTool.SourceInfo.Source != "inline" || dynamicTool.SourceInfo.Scope != "temporary" || dynamicTool.SourceInfo.Origin != "top-level" {
		t.Fatalf("dynamic tool source = %#v", dynamicTool.SourceInfo)
	}
	if readTool.SourceInfo.Path != "<builtin:read>" || readTool.SourceInfo.Source != "builtin" {
		t.Fatalf("read tool source = %#v", readTool.SourceInfo)
	}
	if !containsString(host.Session.GetActiveToolNames(), "dynamic_tool") {
		t.Fatalf("active tool names = %#v", host.Session.GetActiveToolNames())
	}
	if !strings.Contains(host.Session.SystemPrompt, "- dynamic_tool: Run dynamic test behavior") {
		t.Fatalf("system prompt missing dynamic tool snippet:\n%s", host.Session.SystemPrompt)
	}
	if !strings.Contains(host.Session.SystemPrompt, "- Use dynamic_tool when the user asks for dynamic behavior tests.") {
		t.Fatalf("system prompt missing dynamic guideline:\n%s", host.Session.SystemPrompt)
	}
}

func TestAgentSessionSDKCustomToolSourceMetadata(t *testing.T) {
	session := createDynamicSessionForTest(t, nil, []SDKTool{{
		Name:        "sdk_tool",
		Label:       "SDK Tool",
		Description: "Tool registered through createAgentSession",
		Execute: func(toolCallID string, input map[string]any) (SDKToolResult, error) {
			return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "ok"}}}, nil
		},
	}})

	sdkTool := findDynamicSDKTool(session.GetAllTools(), "sdk_tool")
	if sdkTool == nil {
		t.Fatalf("sdk_tool missing from %#v", session.GetAllTools())
	}
	if sdkTool.SourceInfo.Path != "<sdk:sdk_tool>" || sdkTool.SourceInfo.Source != "sdk" || sdkTool.SourceInfo.Scope != "temporary" || sdkTool.SourceInfo.Origin != "top-level" {
		t.Fatalf("sdk source = %#v", sdkTool.SourceInfo)
	}
	if !containsString(session.GetActiveToolNames(), "sdk_tool") {
		t.Fatalf("active tool names = %#v", session.GetActiveToolNames())
	}
}

func TestAgentSessionDynamicToolWithoutPromptSnippetStaysHiddenFromPrompt(t *testing.T) {
	host := createDynamicExtensionHost(t, func(ctx *ProtocolExtensionContext) error {
		return ctx.On(ProtocolEventSessionStart, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			return ProtocolEventResult{}, ctx.RegisterTool(ProtocolToolDefinition{
				Name:        "hidden_tool",
				Label:       "Hidden Tool",
				Description: "Description should not appear in available tools",
				Execute: func(toolCallID string, input map[string]any) (SDKToolResult, error) {
					return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "ok"}}}, nil
				},
			})
		})
	})

	if !containsString(host.Session.GetActiveToolNames(), "hidden_tool") {
		t.Fatalf("active tool names = %#v", host.Session.GetActiveToolNames())
	}
	if strings.Contains(host.Session.SystemPrompt, "hidden_tool") || strings.Contains(host.Session.SystemPrompt, "Description should not appear in available tools") {
		t.Fatalf("hidden tool leaked into system prompt:\n%s", host.Session.SystemPrompt)
	}
}

func TestAgentSessionToolAllowlistFiltersExtensionToolsPiRegression(t *testing.T) {
	host := createDynamicExtensionHostWithOptions(t, AgentSessionOptions{Tools: []string{"read", "dynamic_tool"}, ToolsSet: true}, registerDynamicToolOnSessionStart)
	names := host.Session.GetActiveToolNames()
	if !reflectStringSetEqual(names, []string{"dynamic_tool", "read"}) {
		t.Fatalf("active tool names = %#v", names)
	}
	allNames := toolNames(host.Session.GetAllTools())
	if !reflectStringSetEqual(allNames, []string{"dynamic_tool", "read"}) {
		t.Fatalf("all tool names = %#v", allNames)
	}
	if !strings.Contains(host.Session.SystemPrompt, "- read: Read file contents") ||
		!strings.Contains(host.Session.SystemPrompt, "- dynamic_tool: Run dynamic test behavior") ||
		strings.Contains(host.Session.SystemPrompt, "- bash:") ||
		strings.Contains(host.Session.SystemPrompt, "- edit:") {
		t.Fatalf("system prompt = %s", host.Session.SystemPrompt)
	}

	empty := createDynamicExtensionHostWithOptions(t, AgentSessionOptions{Tools: []string{}, ToolsSet: true}, registerDynamicToolOnSessionStart)
	if len(empty.Session.GetAllTools()) != 0 || len(empty.Session.GetActiveToolNames()) != 0 {
		t.Fatalf("empty allowlist tools = all %#v active %#v", empty.Session.GetAllTools(), empty.Session.GetActiveToolNames())
	}
	if !strings.Contains(empty.Session.SystemPrompt, "Available tools:\n(none)") || strings.Contains(empty.Session.SystemPrompt, "dynamic_tool") {
		t.Fatalf("empty allowlist system prompt = %s", empty.Session.SystemPrompt)
	}
}

func TestAgentSessionNoBuiltinToolsKeepsExtensionToolsPiRegression(t *testing.T) {
	host := createDynamicExtensionHostWithOptions(t, AgentSessionOptions{NoTools: "builtin"}, registerDynamicToolOnSessionStart)
	allNames := toolNames(host.Session.GetAllTools())
	for _, name := range []string{"bash", "dynamic_tool", "edit", "find", "grep", "ls", "read", "write"} {
		if !containsString(allNames, name) {
			t.Fatalf("all tool names = %#v, missing %q", allNames, name)
		}
	}
	if got := host.Session.GetActiveToolNames(); !reflectStringSetEqual(got, []string{"dynamic_tool"}) {
		t.Fatalf("active tool names = %#v", got)
	}
	if !strings.Contains(host.Session.SystemPrompt, "- dynamic_tool: Run dynamic test behavior") ||
		strings.Contains(host.Session.SystemPrompt, "- read:") ||
		strings.Contains(host.Session.SystemPrompt, "- bash:") {
		t.Fatalf("system prompt = %s", host.Session.SystemPrompt)
	}

	allDisabled := createDynamicExtensionHostWithOptions(t, AgentSessionOptions{NoTools: "all"}, registerDynamicToolOnSessionStart)
	if len(allDisabled.Session.GetAllTools()) != 0 || len(allDisabled.Session.GetActiveToolNames()) != 0 {
		t.Fatalf("noTools all = all %#v active %#v", allDisabled.Session.GetAllTools(), allDisabled.Session.GetActiveToolNames())
	}
	if !strings.Contains(allDisabled.Session.SystemPrompt, "Available tools:\n(none)") {
		t.Fatalf("noTools all system prompt = %s", allDisabled.Session.SystemPrompt)
	}
}

func createDynamicExtensionHost(t *testing.T, factory func(*ProtocolExtensionContext) error) *AgentSessionRuntimeHost {
	t.Helper()
	return createDynamicExtensionHostWithOptions(t, AgentSessionOptions{}, factory)
}

func createDynamicExtensionHostWithOptions(t *testing.T, options AgentSessionOptions, factory func(*ProtocolExtensionContext) error) *AgentSessionRuntimeHost {
	t.Helper()
	session := createDynamicSessionForTestWithOptions(t, nil, nil, options)
	runtime := NewProtocolExtensionRuntime(
		CapabilityCommandsRegister,
		CapabilityLifecycleEvents,
		CapabilityProvidersRegister,
		CapabilityToolsRegister,
	)
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{Path: "dynamic-extension", Factory: factory}}); err != nil {
		t.Fatal(err)
	}
	host, err := NewAgentSessionRuntimeHost(session, runtime)
	if err != nil {
		t.Fatal(err)
	}
	return host
}

func createDynamicSessionForTest(t *testing.T, responder AgentSessionResponder, customTools []SDKTool) *AgentSession {
	t.Helper()
	return createDynamicSessionForTestWithOptions(t, responder, customTools, AgentSessionOptions{})
}

func createDynamicSessionForTestWithOptions(t *testing.T, responder AgentSessionResponder, customTools []SDKTool, options AgentSessionOptions) *AgentSession {
	t.Helper()
	manager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            manager.GetCWD(),
		AgentDir:       t.TempDir(),
		Model:          llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
		SessionManager: manager,
		Responder:      responder,
		CustomTools:    customTools,
		Tools:          options.Tools,
		ToolsSet:       options.ToolsSet,
		NoTools:        options.NoTools,
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func registerDynamicToolOnSessionStart(ctx *ProtocolExtensionContext) error {
	return ctx.On(ProtocolEventSessionStart, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
		return ProtocolEventResult{}, ctx.RegisterTool(ProtocolToolDefinition{
			Name:          "dynamic_tool",
			Label:         "Dynamic Tool",
			Description:   "Tool registered from session_start",
			PromptSnippet: "Run dynamic test behavior",
			Execute: func(toolCallID string, input map[string]any) (SDKToolResult, error) {
				return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "ok"}}}, nil
			},
		})
	})
}

func toolNames(tools []SDKTool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

func reflectStringSetEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for _, item := range want {
		if !containsString(got, item) {
			return false
		}
	}
	return true
}

func capturePromptBaseURL(t *testing.T, session *AgentSession) string {
	t.Helper()
	var baseURL string
	session.Responder = func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error) {
		baseURL = model.BaseURL
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done")}}, nil
	}
	if err := session.Prompt("hello"); err != nil {
		t.Fatal(err)
	}
	return baseURL
}

func findDynamicSDKTool(tools []SDKTool, name string) *SDKTool {
	for index := range tools {
		if tools[index].Name == name {
			return &tools[index]
		}
	}
	return nil
}
