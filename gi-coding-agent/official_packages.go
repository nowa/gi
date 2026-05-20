package gicodingagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type officialPackageDefinition struct {
	Name             string
	Version          string
	Description      string
	Commands         []protocolCommandDescriptor
	Tools            []protocolToolDescriptor
	MessageRenderers []protocolMessageRendererDescriptor
	Events           []string
	Shortcuts        []protocolShortcutDescriptor
	Flags            []protocolFlagDescriptor
	Skills           []officialPackageSkill
	Prompts          []officialPackagePrompt
}

type officialPackageSkill struct {
	Name        string
	Description string
	Body        string
}

type officialPackagePrompt struct {
	Name        string
	Description string
	Body        string
}

var officialPackages = map[string]officialPackageDefinition{
	"gi-plan-mode": {
		Name:        "gi-plan-mode",
		Version:     "0.1.0",
		Description: "Plan-mode commands, prompts, and session guidance.",
		Commands: []protocolCommandDescriptor{
			{Name: "plan", Description: "Open or update the current implementation plan."},
			{Name: "plan-review", Description: "Review the current plan before execution."},
		},
		MessageRenderers: []protocolMessageRendererDescriptor{{Type: "gi.plan.status"}},
		Events:           []string{"before_agent_start"},
		Shortcuts:        []protocolShortcutDescriptor{{Key: "ctrl+alt+p", Description: "Open plan mode."}},
		Flags:            []protocolFlagDescriptor{{Name: "plan-mode", Description: "Start the session in plan mode.", Type: "boolean", Default: false}},
		Skills: []officialPackageSkill{{
			Name:        "plan-mode",
			Description: "Create, review, and update implementation plans before editing code.",
			Body:        "Use this skill when a task needs a tracked implementation plan. Keep steps concrete, mark status as work progresses, and avoid treating planning as completion.",
		}},
		Prompts: []officialPackagePrompt{{
			Name:        "plan",
			Description: "Create a concise implementation plan.",
			Body:        "Create a concise implementation plan for: $ARGUMENTS\n\nList concrete steps, known risks, and the verification command.",
		}},
	},
	"gi-subagents": {
		Name:        "gi-subagents",
		Version:     "0.1.0",
		Description: "Subagent coordination tools and status rendering.",
		Commands:    []protocolCommandDescriptor{{Name: "subagents", Description: "Show subagent status and controls."}},
		Tools: []protocolToolDescriptor{
			{Name: "subagent_spawn", Label: "Spawn subagent", Description: "Start an isolated child agent through the host agent.spawn action."},
			{Name: "subagent_abort", Label: "Abort subagent", Description: "Cancel a running child agent through the host agent.abort action."},
		},
		MessageRenderers: []protocolMessageRendererDescriptor{{Type: "gi.subagent.status"}},
		Events:           []string{"agent_child_started", "agent_child_finished"},
		Skills: []officialPackageSkill{{
			Name:        "subagents",
			Description: "Coordinate bounded child-agent work without sharing private host APIs.",
			Body:        "Use subagents only for isolated, parallel work. Give each child a bounded task and integrate the result through normal host state.",
		}},
		Prompts: []officialPackagePrompt{{
			Name:        "subagent",
			Description: "Draft a bounded subagent task.",
			Body:        "Draft a self-contained subagent task for: $ARGUMENTS\n\nInclude scope, expected output, and files that must not be touched.",
		}},
	},
	"gi-mcp-adapter": {
		Name:        "gi-mcp-adapter",
		Version:     "0.1.0",
		Description: "MCP server supervision and tool registration adapter.",
		Commands:    []protocolCommandDescriptor{{Name: "mcp", Description: "Inspect configured MCP servers and tools."}},
		Tools: []protocolToolDescriptor{
			{Name: "mcp_list_tools", Label: "List MCP tools", Description: "List tools exposed by configured MCP servers."},
			{Name: "mcp_call", Label: "Call MCP tool", Description: "Call an MCP tool through a host-managed MCP server."},
		},
		MessageRenderers: []protocolMessageRendererDescriptor{{Type: "gi.mcp.diagnostics"}},
		Skills: []officialPackageSkill{{
			Name:        "mcp-adapter",
			Description: "Expose MCP server tools through the Gi protocol boundary.",
			Body:        "Use this adapter to route MCP server capabilities through host-approved process execution and tool registration.",
		}},
		Prompts: []officialPackagePrompt{{
			Name:        "mcp-diagnose",
			Description: "Diagnose MCP server configuration.",
			Body:        "Inspect MCP configuration for: $ARGUMENTS\n\nReport server command, granted capabilities, and available tools.",
		}},
	},
	"gi-git-guard": {
		Name:        "gi-git-guard",
		Version:     "0.1.0",
		Description: "Git workspace guardrails for sensitive actions.",
		Commands:    []protocolCommandDescriptor{{Name: "git-guard", Description: "Show git guard status and policy."}},
		Tools:       []protocolToolDescriptor{{Name: "git_guard_check", Label: "Check git guard", Description: "Check current git state before a guarded action."}},
		Events:      []string{"before_session_action"},
		Skills: []officialPackageSkill{{
			Name:        "git-guard",
			Description: "Check git state before high-risk workspace mutations.",
			Body:        "Before destructive or publish actions, inspect branch, status, and remote state. Record the decision as session state.",
		}},
		Prompts: []officialPackagePrompt{{
			Name:        "git-guard",
			Description: "Review git safety for an action.",
			Body:        "Review git safety for: $ARGUMENTS\n\nCheck branch, dirty files, remote divergence, and rollback path.",
		}},
	},
	"gi-approval-gate": {
		Name:        "gi-approval-gate",
		Version:     "0.1.0",
		Description: "Approval gate for sensitive tools and actions.",
		Commands:    []protocolCommandDescriptor{{Name: "approvals", Description: "Show pending approval gates."}},
		Tools:       []protocolToolDescriptor{{Name: "approval_gate_request", Label: "Request approval", Description: "Request host approval with structured context."}},
		MessageRenderers: []protocolMessageRendererDescriptor{
			{Type: "gi.approval.request"},
			{Type: "gi.approval.decision"},
		},
		Events: []string{"before_tool_call"},
		Skills: []officialPackageSkill{{
			Name:        "approval-gate",
			Description: "Request and record approval for sensitive tool calls.",
			Body:        "Use approval gates for operations that need explicit consent. Include command, target files, risk, and proposed decision.",
		}},
		Prompts: []officialPackagePrompt{{
			Name:        "approval",
			Description: "Prepare an approval request.",
			Body:        "Prepare an approval request for: $ARGUMENTS\n\nInclude action, reason, risk, and the exact command or diff.",
		}},
	},
	"gi-powerline-footer": {
		Name:             "gi-powerline-footer",
		Version:          "0.1.0",
		Description:      "Footer status component contributions.",
		Commands:         []protocolCommandDescriptor{{Name: "footer", Description: "Configure footer status segments."}},
		MessageRenderers: []protocolMessageRendererDescriptor{{Type: "gi.powerline.footer"}},
		Events:           []string{"status_refresh"},
		Skills: []officialPackageSkill{{
			Name:        "powerline-footer",
			Description: "Render compact model, branch, context, and tool status in the footer.",
			Body:        "Use semantic status fields and theme tokens. Keep footer state derived from host-provided session data.",
		}},
		Prompts: []officialPackagePrompt{{
			Name:        "footer-status",
			Description: "Summarize current footer status fields.",
			Body:        "Summarize footer status for: $ARGUMENTS\n\nInclude model, branch, context, tools, and pending work.",
		}},
	},
	"gi-todo-widget": {
		Name:        "gi-todo-widget",
		Version:     "0.1.0",
		Description: "Todo tools and widget rendering.",
		Commands:    []protocolCommandDescriptor{{Name: "todo", Description: "Show or update the current todo list."}},
		Tools: []protocolToolDescriptor{
			{Name: "todo_read", Label: "Read todos", Description: "Read the current session todo state."},
			{Name: "todo_write", Label: "Write todos", Description: "Update the current session todo state."},
		},
		MessageRenderers: []protocolMessageRendererDescriptor{{Type: "gi.todo.widget"}},
		Events:           []string{"session_loaded"},
		Skills: []officialPackageSkill{{
			Name:        "todo-widget",
			Description: "Maintain session todos and render current progress.",
			Body:        "Use todos for multi-step work. Keep each item actionable and update state as work completes.",
		}},
		Prompts: []officialPackagePrompt{{
			Name:        "todo",
			Description: "Create a focused todo list.",
			Body:        "Create a focused todo list for: $ARGUMENTS\n\nUse short actionable items and include one verification item.",
		}},
	},
	"gi-tools-ui": {
		Name:        "gi-tools-ui",
		Version:     "0.1.0",
		Description: "Tool selection and tool-status UI.",
		Commands:    []protocolCommandDescriptor{{Name: "tools", Description: "Show and toggle available tools."}},
		Tools:       []protocolToolDescriptor{{Name: "tools_ui_list", Label: "List tools", Description: "List available host tools and their active state."}},
		MessageRenderers: []protocolMessageRendererDescriptor{
			{Type: "gi.tools.list"},
			{Type: "gi.tools.status"},
		},
		Flags: []protocolFlagDescriptor{{Name: "tools-ui", Description: "Enable tools UI contributions.", Type: "boolean", Default: true}},
		Skills: []officialPackageSkill{{
			Name:        "tools-ui",
			Description: "Display and update the active tool set through host-approved actions.",
			Body:        "Use the tools UI to inspect available tools and request active-tool changes through the protocol host.",
		}},
		Prompts: []officialPackagePrompt{{
			Name:        "tools",
			Description: "Review tool selection for a task.",
			Body:        "Review the tool selection for: $ARGUMENTS\n\nList required tools, disabled tools, and any approval needed.",
		}},
	},
}

func OfficialPackageNames() []string {
	names := make([]string, 0, len(officialPackages))
	for name := range officialPackages {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func isOfficialPackageName(name string) bool {
	_, ok := officialPackages[strings.TrimSpace(name)]
	return ok
}

func (m *DefaultPackageManager) materializeOfficialPackage(name string) (string, error) {
	definition, ok := officialPackages[strings.TrimSpace(name)]
	if !ok {
		return "", fmt.Errorf("unknown official package %q", name)
	}
	root := filepath.Join(officialPackageStoreDir(m.agentDir, m.cwd), definition.Name)
	files, err := definition.files()
	if err != nil {
		return "", err
	}
	for relPath, content := range files {
		if err := writeOfficialPackageFile(filepath.Join(root, relPath), []byte(content)); err != nil {
			return "", err
		}
	}
	return root, nil
}

func officialPackageStoreDir(agentDir, cwd string) string {
	if agentDir != "" {
		return filepath.Join(agentDir, "official-packages")
	}
	if cwd == "" {
		cwd = "."
	}
	return filepath.Join(cwd, ConfigDirName, "agent", "official-packages")
}

func (p officialPackageDefinition) files() (map[string]string, error) {
	skillEntries := make([]string, 0, len(p.Skills))
	promptEntries := make([]string, 0, len(p.Prompts))
	descriptorSkillEntries := make([]string, 0, len(p.Skills))
	descriptorPromptEntries := make([]string, 0, len(p.Prompts))
	files := map[string]string{}
	for _, skill := range p.Skills {
		path := filepath.ToSlash(filepath.Join("skills", skill.Name, "SKILL.md"))
		skillEntries = append(skillEntries, path)
		descriptorSkillEntries = append(descriptorSkillEntries, filepath.ToSlash(filepath.Join("..", path)))
		files[path] = renderOfficialPackageSkill(skill)
	}
	for _, prompt := range p.Prompts {
		path := filepath.ToSlash(filepath.Join("prompts", prompt.Name+".md"))
		promptEntries = append(promptEntries, path)
		descriptorPromptEntries = append(descriptorPromptEntries, filepath.ToSlash(filepath.Join("..", path)))
		files[path] = renderOfficialPackagePrompt(prompt)
	}
	manifest := map[string]any{
		"name":        p.Name,
		"version":     p.Version,
		"description": p.Description,
		"keywords":    []string{"gi", "gi-official-package"},
		"gi": map[string]any{
			"manifestVersion": 1,
			"extensions":      []string{"extensions/main.gi.json"},
			"skills":          skillEntries,
			"prompts":         promptEntries,
		},
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	descriptor := protocolExtensionDescriptor{
		Gi: &protocolExtensionDescriptorGI{
			ExtensionProtocol: "descriptor.v1",
			ID:                p.Name,
			Commands:          p.Commands,
			Tools:             p.Tools,
			MessageRenderers:  p.MessageRenderers,
			Events:            p.Events,
			Shortcuts:         p.Shortcuts,
			Flags:             p.Flags,
			Resources: protocolResourceDescriptor{
				Skills:  descriptorSkillEntries,
				Prompts: descriptorPromptEntries,
			},
		},
	}
	descriptorJSON, err := json.MarshalIndent(descriptor, "", "  ")
	if err != nil {
		return nil, err
	}
	files["gi.package.json"] = string(manifestJSON) + "\n"
	files[filepath.ToSlash(filepath.Join("extensions", "main.gi.json"))] = string(descriptorJSON) + "\n"
	return files, nil
}

func renderOfficialPackageSkill(skill officialPackageSkill) string {
	return "---\nname: " + skill.Name + "\ndescription: " + skill.Description + "\n---\n\n" + skill.Body + "\n"
}

func renderOfficialPackagePrompt(prompt officialPackagePrompt) string {
	return "---\ndescription: " + prompt.Description + "\n---\n\n" + prompt.Body + "\n"
}

func writeOfficialPackageFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil && string(existing) == string(content) {
		return nil
	}
	return os.WriteFile(path, content, 0o644)
}

func officialCommandHandler(ctx *ProtocolExtensionContext, commandName string) func(string) error {
	packageName := officialPackageNameFromSource(ctx)
	if packageName == "" {
		return nil
	}
	return func(args string) error {
		_, err := ctx.runtime.AppendCustomEntry(packageName+".command", map[string]any{
			"package": packageName,
			"command": commandName,
			"args":    strings.TrimSpace(args),
		})
		return err
	}
}

func officialToolExecutor(ctx *ProtocolExtensionContext, toolName string) func(string, map[string]any) (SDKToolResult, error) {
	packageName := officialPackageNameFromSource(ctx)
	if packageName == "" {
		return nil
	}
	return func(_ string, input map[string]any) (SDKToolResult, error) {
		switch toolName {
		case "todo_write":
			data := map[string]any{"todos": input["todos"]}
			if data["todos"] == nil {
				data = input
			}
			if _, err := ctx.runtime.AppendCustomEntry("todo_state", data); err != nil {
				return SDKToolResult{}, err
			}
			return officialTextToolResult("Stored todo state."), nil
		case "todo_read":
			if data, ok := officialLatestCustomData(ctx.runtime, "todo_state"); ok {
				return officialJSONToolResult(data)
			}
			return officialJSONToolResult(map[string]any{"todos": []any{}})
		case "tools_ui_list":
			return officialJSONToolResult(map[string]any{"tools": ctx.runtime.ActiveToolNames()})
		case "approval_gate_request":
			if _, err := ctx.runtime.AppendCustomEntry("approval_request", input); err != nil {
				return SDKToolResult{}, err
			}
			return officialTextToolResult("Recorded approval request."), nil
		case "git_guard_check":
			return officialJSONToolResult(map[string]any{"status": "recorded", "input": input})
		case "subagent_spawn":
			if _, err := ctx.runtime.AppendCustomEntry("subagent_request", input); err != nil {
				return SDKToolResult{}, err
			}
			return officialTextToolResult("Recorded subagent spawn request."), nil
		case "subagent_abort":
			if _, err := ctx.runtime.AppendCustomEntry("subagent_abort", input); err != nil {
				return SDKToolResult{}, err
			}
			return officialTextToolResult("Recorded subagent abort request."), nil
		case "mcp_list_tools":
			return officialJSONToolResult(map[string]any{"tools": []any{}})
		case "mcp_call":
			if _, err := ctx.runtime.AppendCustomEntry("mcp_call", input); err != nil {
				return SDKToolResult{}, err
			}
			return officialTextToolResult("Recorded MCP tool call request."), nil
		default:
			if _, err := ctx.runtime.AppendCustomEntry(packageName+".tool", map[string]any{"tool": toolName, "input": input}); err != nil {
				return SDKToolResult{}, err
			}
			return officialTextToolResult("Recorded " + toolName + " request."), nil
		}
	}
}

func officialPackageNameFromSource(ctx *ProtocolExtensionContext) string {
	if ctx == nil {
		return ""
	}
	source := strings.TrimSpace(ctx.source.Source)
	if !strings.HasPrefix(source, "official:") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(source, "official:"))
}

func officialLatestCustomData(runtime *ProtocolExtensionRuntime, customType string) (any, bool) {
	entries := runtime.SessionEntries()
	for index := len(entries) - 1; index >= 0; index-- {
		if entries[index].CustomType == customType {
			return entries[index].Data, true
		}
	}
	return nil, false
}

func officialTextToolResult(text string) SDKToolResult {
	return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: text}}}
}

func officialJSONToolResult(value any) (SDKToolResult, error) {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return SDKToolResult{}, err
	}
	return officialTextToolResult(string(content)), nil
}
