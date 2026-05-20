package gicodingagent

import (
	"strings"
	"testing"
)

func TestOfficialPackagesProvideExecutableProtocolContributions(t *testing.T) {
	agentDir, cwd := createResourceLoaderDirs(t)
	sources := make([]any, 0, len(OfficialPackageNames()))
	for _, name := range OfficialPackageNames() {
		sources = append(sources, "official:"+name)
	}
	settings := NewInMemorySettingsManager(map[string]any{"packages": sources})
	loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings})
	loader.Reload()
	runtime := loader.GetExtensions().Runtime
	if runtime == nil {
		t.Fatal("runtime is nil")
	}
	sessionManager, err := InMemorySessionManager(cwd)
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            cwd,
		AgentDir:       agentDir,
		Model:          sdkTestModel(),
		SessionManager: sessionManager,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime.BindSession(session)

	commandsByPackage := map[string]int{}
	for _, command := range runtime.RegisteredCommands() {
		source := strings.TrimPrefix(command.SourceInfo.Source, "official:")
		if source == command.SourceInfo.Source {
			continue
		}
		if command.Handler == nil {
			t.Fatalf("official command %s has no handler", command.Name)
		}
		if err := command.Handler("test"); err != nil {
			t.Fatalf("command %s: %v", command.Name, err)
		}
		commandsByPackage[source]++
	}

	toolsByPackage := map[string]int{}
	for _, tool := range runtime.RegisteredTools() {
		source := strings.TrimPrefix(tool.SourceInfo.Source, "official:")
		if source == tool.SourceInfo.Source {
			continue
		}
		if tool.Execute == nil {
			t.Fatalf("official tool %s has no execute", tool.Name)
		}
		result, err := tool.Execute("test-"+tool.Name, officialToolTestInput(tool.Name))
		if err != nil {
			t.Fatalf("tool %s: %v", tool.Name, err)
		}
		if strings.TrimSpace(sdkToolText(result)) == "" {
			t.Fatalf("tool %s returned empty result", tool.Name)
		}
		toolsByPackage[source]++
	}

	for _, name := range OfficialPackageNames() {
		if commandsByPackage[name] == 0 {
			t.Fatalf("%s registered no executable commands", name)
		}
		if resourceFindSkill(loader.GetSkills().Skills, officialPackageSkillName(name)) == nil {
			t.Fatalf("%s skill missing", name)
		}
		if resourceFindPrompt(loader.GetPrompts().Prompts, officialPackagePromptName(name)) == nil {
			t.Fatalf("%s prompt missing", name)
		}
	}
	for _, name := range []string{"gi-subagents", "gi-mcp-adapter", "gi-git-guard", "gi-approval-gate", "gi-todo-widget", "gi-tools-ui"} {
		if toolsByPackage[name] == 0 {
			t.Fatalf("%s registered no executable tools", name)
		}
	}
}

func officialToolTestInput(name string) map[string]any {
	switch name {
	case "todo_write":
		return map[string]any{"todos": []any{"test todo"}}
	case "subagent_spawn":
		return map[string]any{"task": "test child task"}
	case "subagent_abort":
		return map[string]any{"id": "child-1"}
	case "approval_gate_request":
		return map[string]any{"action": "test approval"}
	case "mcp_call":
		return map[string]any{"tool": "test"}
	default:
		return map[string]any{"reason": "test"}
	}
}

func officialPackageSkillName(name string) string {
	switch name {
	case "gi-plan-mode":
		return "plan-mode"
	case "gi-mcp-adapter":
		return "mcp-adapter"
	case "gi-git-guard":
		return "git-guard"
	case "gi-approval-gate":
		return "approval-gate"
	case "gi-powerline-footer":
		return "powerline-footer"
	case "gi-todo-widget":
		return "todo-widget"
	case "gi-tools-ui":
		return "tools-ui"
	default:
		return strings.TrimPrefix(name, "gi-")
	}
}

func officialPackagePromptName(name string) string {
	switch name {
	case "gi-plan-mode":
		return "plan"
	case "gi-mcp-adapter":
		return "mcp-diagnose"
	case "gi-approval-gate":
		return "approval"
	case "gi-powerline-footer":
		return "footer-status"
	case "gi-subagents":
		return "subagent"
	case "gi-todo-widget":
		return "todo"
	case "gi-tools-ui":
		return "tools"
	default:
		return strings.TrimPrefix(name, "gi-")
	}
}
