package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type officialPackageDefinition struct {
	Name             string
	Version          string
	Description      string
	Capabilities     []string
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

type officialMCPToolInfo struct {
	Name        string
	Description string
}

type officialToolRenderer struct {
	Name       string
	Definition ProtocolToolRendererDefinition
}

var officialPackages = map[string]officialPackageDefinition{
	"gi-plan-mode": {
		Name:        "gi-plan-mode",
		Version:     "0.1.0",
		Description: "Plan-mode commands, prompts, and session guidance.",
		Capabilities: []string{
			"commands.register",
			"tools.set_active",
			"session.write",
			"tui.status",
			"tui.widget",
			"tui.dialog",
			"tui.editor",
		},
		Commands: []protocolCommandDescriptor{
			{Name: "plan", Description: "Open or update the current implementation plan."},
			{Name: "plan-review", Description: "Review the current plan before execution."},
		},
		Tools: []protocolToolDescriptor{
			{Name: "plan_update", Label: "Update plan", Description: "Store the current implementation plan state."},
			{Name: "plan_read", Label: "Read plan", Description: "Read the current implementation plan state."},
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
		Capabilities: []string{
			"tools.register",
			"session.read",
			"agent.spawn",
			"agent.abort",
			"tui.status",
			"tui.widget",
		},
		Commands: []protocolCommandDescriptor{{Name: "subagents", Description: "Show subagent status and controls."}},
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
		Capabilities: []string{
			"tools.register",
			"process.stdio:<scope>",
			"network:<scope>",
			"tui.widget",
		},
		Commands: []protocolCommandDescriptor{{Name: "mcp", Description: "Inspect configured MCP servers and tools."}},
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
		Capabilities: []string{
			"session.read",
			"session.write",
			"process.exec:<scope>",
			"tui.dialog",
			"tui.widget",
		},
		Commands: []protocolCommandDescriptor{{Name: "git-guard", Description: "Show git guard status and policy."}},
		Tools:    []protocolToolDescriptor{{Name: "git_guard_check", Label: "Check git guard", Description: "Check current git state before a guarded action."}},
		MessageRenderers: []protocolMessageRendererDescriptor{
			{Type: "gi.git.guard"},
		},
		Events: []string{"before_session_action"},
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
		Capabilities: []string{
			"session.write",
			"tui.dialog",
			"tui.overlay",
			"tui.widget",
			"tui.tool_renderer",
		},
		Commands: []protocolCommandDescriptor{{Name: "approvals", Description: "Show pending approval gates."}},
		Tools: []protocolToolDescriptor{
			{Name: "approval_gate_request", Label: "Request approval", Description: "Request host approval with structured context."},
			{Name: "approval_gate_decide", Label: "Record approval decision", Description: "Record an approve, deny, or rewrite decision for a pending gate."},
		},
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
		Capabilities:     []string{"session.read", "tui.footer", "tui.status"},
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
		Capabilities: []string{
			"tools.register",
			"session.read",
			"session.write",
			"tui.widget",
			"tui.message_renderer",
			"tui.tool_renderer",
		},
		Commands: []protocolCommandDescriptor{{Name: "todo", Description: "Show or update the current todo list."}},
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
		Capabilities: []string{
			"commands.register",
			"tools.set_active",
			"session.write",
			"tui.dialog",
			"tui.overlay",
			"tui.widget",
		},
		Commands: []protocolCommandDescriptor{{Name: "tools", Description: "Show and toggle available tools."}},
		Tools: []protocolToolDescriptor{
			{Name: "tools_ui_list", Label: "List tools", Description: "List available host tools and their active state."},
			{Name: "tools_ui_set_active", Label: "Set active tools", Description: "Patch the active host tool set through the protocol boundary."},
		},
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
	root, err := resolveManagedPath(officialPackageStoreDir(m.agentDir, m.cwd), definition.Name)
	if err != nil {
		return "", err
	}
	files, err := definition.files()
	if err != nil {
		return "", err
	}
	for relPath, content := range files {
		path, err := resolveManagedPath(root, relPath)
		if err != nil {
			return "", err
		}
		if err := writeOfficialPackageFile(path, []byte(content)); err != nil {
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
			Capabilities:      p.Capabilities,
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
		if err := officialApplyCommandEffects(ctx, packageName, commandName, args); err != nil {
			return err
		}
		payload := officialCommandPayload(ctx, packageName, commandName, args)
		if _, err := ctx.runtime.AppendCustomEntry(packageName+".command", payload); err != nil {
			return err
		}
		customType := officialCommandRendererType(packageName, commandName)
		if customType == "" {
			return nil
		}
		if err := officialMountCommandView(ctx, packageName, commandName, customType, payload); err != nil {
			return err
		}
		return ctx.SendCustomMessage(ProtocolCustomMessage{
			CustomType: customType,
			Content:    payload,
			Display:    true,
		}, ProtocolSendCustomMessageOptions{})
	}
}

func officialApplyCommandEffects(ctx *ProtocolExtensionContext, packageName, commandName, args string) error {
	if ctx == nil || ctx.runtime == nil {
		return nil
	}
	switch packageName {
	case "gi-plan-mode":
		if commandName != "plan" && commandName != "plan-review" {
			return nil
		}
		readOnlyTools := officialKnownToolSubset(ctx, []string{"read", "grep", "find", "ls"})
		if len(readOnlyTools) == 0 {
			return nil
		}
		activeNames, err := officialSetActiveTools(ctx, "replace", readOnlyTools)
		if err != nil {
			return err
		}
		data := map[string]any{}
		if existing, ok := officialLatestCustomData(ctx.runtime, "plan_state"); ok {
			if object, ok := existing.(map[string]any); ok {
				for key, value := range object {
					data[key] = value
				}
			}
		}
		data["mode"] = "planning"
		data["status"] = "read-only"
		if commandName == "plan-review" {
			data["mode"] = "review"
			data["status"] = "reviewing"
		}
		data["activeToolNames"] = activeNames
		if trimmed := strings.TrimSpace(args); trimmed != "" {
			data["args"] = trimmed
		}
		_, err = ctx.runtime.AppendCustomEntry("plan_state", data)
		return err
	}
	return nil
}

func officialMountCommandView(ctx *ProtocolExtensionContext, packageName, commandName, rendererType string, payload map[string]any) error {
	if ctx == nil || ctx.runtime == nil || ctx.runtime.viewTreeHost == nil || rendererType == "" {
		return nil
	}
	slot := officialCommandViewSlot(packageName, commandName)
	if slot == "" {
		return nil
	}
	return ctx.MountViewTree(
		officialCommandMountID(packageName, commandName),
		slot,
		officialCommandViewTree(rendererType, packageName, payload, slot),
		ViewTreeMountOptions{Priority: officialCommandViewPriority(slot)},
	)
}

func officialCommandViewSlot(packageName, commandName string) string {
	switch packageName {
	case "gi-powerline-footer":
		return "footer"
	case "gi-plan-mode", "gi-subagents", "gi-mcp-adapter", "gi-git-guard", "gi-approval-gate", "gi-todo-widget", "gi-tools-ui":
		return "aboveEditor"
	default:
		return ""
	}
}

func officialCommandMountID(packageName, commandName string) string {
	id := "official." + strings.TrimSpace(packageName)
	if strings.TrimSpace(commandName) != "" {
		id += "." + strings.TrimSpace(commandName)
	}
	return id
}

func officialCommandViewPriority(slot string) int {
	if canonicalViewTreeSlot(slot) == "footer" {
		return 80
	}
	return 40
}

func officialCommandViewTree(rendererType, packageName string, payload map[string]any, slot string) ViewTreeNode {
	title := officialRendererTitle(rendererType, packageName)
	details := officialRendererDetails(payload)
	if len(details) == 0 {
		details = []string{"No status recorded."}
	}
	if canonicalViewTreeSlot(slot) == "footer" {
		footerText := details[0]
		for _, detail := range details {
			if !strings.HasPrefix(detail, "Command: ") {
				footerText = detail
				break
			}
		}
		return ViewTreeNode{
			Type: "text",
			ID:   "official-footer-" + strings.TrimSpace(packageName),
			Text: title + ": " + footerText,
		}
	}
	items := make([]ViewTreeItem, 0, len(details))
	for index, line := range details {
		items = append(items, ViewTreeItem{Item: &ViewTreeListItem{ID: fmt.Sprintf("line-%d", index+1), Text: line}})
	}
	return ViewTreeNode{
		Type: "box",
		ID:   "official-widget-" + strings.TrimSpace(packageName),
		Children: []ViewTreeNode{
			{Type: "text", ID: "title", Text: title},
			{Type: "list", ID: "details", Items: items},
		},
	}
}

func officialCommandRendererType(packageName, commandName string) string {
	switch packageName {
	case "gi-plan-mode":
		return "gi.plan.status"
	case "gi-subagents":
		return "gi.subagent.status"
	case "gi-mcp-adapter":
		return "gi.mcp.diagnostics"
	case "gi-git-guard":
		return "gi.git.guard"
	case "gi-approval-gate":
		return "gi.approval.request"
	case "gi-powerline-footer":
		return "gi.powerline.footer"
	case "gi-todo-widget":
		return "gi.todo.widget"
	case "gi-tools-ui":
		if commandName == "tools" {
			return "gi.tools.list"
		}
		return "gi.tools.status"
	default:
		return ""
	}
}

func officialMessageRenderer(ctx *ProtocolExtensionContext, rendererType string) ProtocolMessageRenderer {
	packageName := officialPackageNameFromSource(ctx)
	return func(message any, _ any) []string {
		payload := officialMessagePayload(message)
		title := officialRendererTitle(rendererType, packageName)
		details := officialRendererDetails(payload)
		if len(details) == 0 {
			return []string{title}
		}
		return append([]string{title}, details...)
	}
}

func officialRendererTitle(rendererType, packageName string) string {
	switch rendererType {
	case "gi.plan.status":
		return "Plan Mode"
	case "gi.subagent.status":
		return "Subagents"
	case "gi.mcp.diagnostics":
		return "MCP Adapter"
	case "gi.approval.request":
		return "Approval Gate"
	case "gi.approval.decision":
		return "Approval Decision"
	case "gi.git.guard":
		return "Git Guard"
	case "gi.powerline.footer":
		return "Powerline Footer"
	case "gi.todo.widget":
		return "Todo Widget"
	case "gi.tools.list":
		return "Tools"
	case "gi.tools.status":
		return "Tool Status"
	default:
		if packageName != "" {
			return packageName
		}
		return rendererType
	}
}

func officialRendererDetails(payload map[string]any) []string {
	var lines []string
	command := strings.TrimSpace(stringFromAny(payload["command"]))
	args := strings.TrimSpace(stringFromAny(payload["args"]))
	if command != "" && args != "" {
		lines = append(lines, "Command: /"+command+" "+args)
	} else if command != "" {
		lines = append(lines, "Command: /"+command)
	}
	if summary := strings.TrimSpace(stringFromAny(payload["summary"])); summary != "" {
		lines = append(lines, summary)
	}
	if model := strings.TrimSpace(stringFromAny(payload["model"])); model != "" {
		lines = append(lines, "Model: "+model)
	}
	if branch := strings.TrimSpace(stringFromAny(payload["branch"])); branch != "" {
		lines = append(lines, "Branch: "+branch)
	}
	if thinking := strings.TrimSpace(stringFromAny(payload["thinking"])); thinking != "" {
		lines = append(lines, "Thinking: "+thinking)
	}
	if context := strings.TrimSpace(stringFromAny(payload["context"])); context != "" {
		lines = append(lines, "Context: "+context)
	}
	if status := strings.TrimSpace(stringFromAny(payload["status"])); status != "" {
		lines = append(lines, "Status: "+status)
	}
	if decision := strings.TrimSpace(stringFromAny(payload["decision"])); decision != "" {
		lines = append(lines, "Decision: "+decision)
	}
	if risk := strings.TrimSpace(stringFromAny(payload["risk"])); risk != "" {
		lines = append(lines, "Risk: "+risk)
	}
	if reason := strings.TrimSpace(stringFromAny(payload["reason"])); reason != "" {
		lines = append(lines, "Reason: "+reason)
	}
	if replacement := strings.TrimSpace(stringFromAny(payload["replacement"])); replacement != "" {
		lines = append(lines, "Replacement: "+replacement)
	}
	if todos := officialStringListFromAny(payload["todos"]); len(todos) > 0 {
		lines = append(lines, "Todos: "+strings.Join(todos, ", "))
	}
	if tools := officialStringListFromAny(payload["tools"]); len(tools) > 0 {
		lines = append(lines, "Active tools: "+strings.Join(tools, ", "))
	}
	if tools := officialStringListFromAny(payload["mcpTools"]); len(tools) > 0 {
		lines = append(lines, "MCP tools: "+strings.Join(tools, ", "))
	}
	if progress := officialMCPProgressLines(payload["mcpProgress"]); len(progress) > 0 {
		lines = append(lines, "MCP progress: "+strings.Join(progress, ", "))
	}
	if boolFromAny(payload["mcpToolsListChanged"]) {
		lines = append(lines, "MCP tools changed")
	}
	if items := officialStringListFromAny(payload["items"]); len(items) > 0 {
		lines = append(lines, "Items: "+strings.Join(items, ", "))
	}
	if text := strings.TrimSpace(stringFromAny(payload["text"])); text != "" {
		lines = append(lines, text)
	}
	if diff := strings.TrimSpace(stringFromAny(payload["diff"])); diff != "" {
		lines = append(lines, "Diff:\n"+diff)
	}
	return lines
}

func officialCommandPayload(ctx *ProtocolExtensionContext, packageName, commandName, args string) map[string]any {
	payload := map[string]any{
		"package": packageName,
		"command": commandName,
		"args":    strings.TrimSpace(args),
	}
	var runtime *ProtocolExtensionRuntime
	if ctx != nil {
		runtime = ctx.runtime
	}
	switch packageName {
	case "gi-plan-mode":
		payload["summary"] = "Plan mode is available for tracked implementation steps."
		if data, ok := officialLatestCustomData(runtime, "plan_state"); ok {
			payload["plan"] = data
			if object, ok := data.(map[string]any); ok {
				if status := strings.TrimSpace(stringFromAny(object["status"])); status != "" {
					payload["status"] = status
				}
				if tools := officialStringListFromAny(object["activeToolNames"]); len(tools) > 0 {
					payload["tools"] = tools
					payload["summary"] = "Plan mode active with read-only tools."
					if strings.TrimSpace(stringFromAny(object["mode"])) == "review" {
						payload["summary"] = "Plan review active with read-only tools."
					}
				}
				items := officialStringListFromAny(object["items"])
				payload["items"] = items
				if len(items) > 0 {
					payload["summary"] = fmt.Sprintf("%d plan item(s) recorded.", len(items))
					if _, ok := payload["status"]; !ok {
						payload["status"] = "active"
					}
				}
			}
		}
	case "gi-subagents":
		payload["summary"] = "Subagent requests are routed through host-approved agent actions."
		if data, ok := officialLatestCustomData(runtime, "subagent_request"); ok {
			payload["latestRequest"] = data
			if object, ok := data.(map[string]any); ok {
				if status := strings.TrimSpace(stringFromAny(object["status"])); status != "" {
					payload["status"] = status
				} else {
					payload["status"] = "request recorded"
				}
				if object["result"] != nil {
					if strings.TrimSpace(stringFromAny(object["status"])) == "aborted" {
						payload["summary"] = "Last subagent run was aborted through host.agent.abort."
					} else {
						payload["summary"] = "Last subagent run completed through host.agent.spawn."
					}
				}
				if parentContext := officialSubagentContextLines(object["parentContext"]); len(parentContext) > 0 {
					payload["items"] = parentContext
				}
			} else {
				payload["status"] = "request recorded"
			}
		}
	case "gi-mcp-adapter":
		payload["summary"] = "MCP tools are exposed through the Gi protocol boundary."
		if data, ok := officialLatestCustomData(runtime, "mcp_tools"); ok {
			payload["latestTools"] = data
			tools := officialMCPToolsFromData(data)
			payload["mcpTools"] = tools
			payload["mcpProgress"] = officialMCPProgressFromData(data)
			payload["mcpToolsListChanged"] = officialMCPToolsListChangedFromData(data)
			if len(tools) > 0 {
				payload["summary"] = fmt.Sprintf("%d MCP tool(s) discovered.", len(tools))
				payload["status"] = "tools discovered"
			}
		}
		if data, ok := officialLatestCustomData(runtime, "mcp_call"); ok {
			payload["latestCall"] = data
			payload["mcpProgress"] = officialMCPProgressFromData(data)
			payload["mcpToolsListChanged"] = officialMCPToolsListChangedFromData(data)
			if object, ok := data.(map[string]any); ok {
				if status := strings.TrimSpace(stringFromAny(object["status"])); status != "" {
					payload["status"] = status
				}
				if tool := strings.TrimSpace(stringFromAny(object["tool"])); tool != "" {
					payload["summary"] = "Last MCP call: " + tool
				}
			}
		}
	case "gi-git-guard":
		payload["summary"] = "Review branch, dirty files, and remote state before guarded actions."
		if data, ok := officialLatestCustomData(runtime, "git_guard_check"); ok {
			payload["latestCheck"] = data
			if object, ok := data.(map[string]any); ok {
				if status := strings.TrimSpace(stringFromAny(object["status"])); status != "" {
					payload["status"] = status
				} else {
					payload["status"] = "check recorded"
				}
				if branch := strings.TrimSpace(stringFromAny(object["branch"])); branch != "" {
					payload["summary"] = "Review branch " + branch + " before guarded actions."
				}
				if dirtyFiles := officialStringListFromAny(object["dirtyFiles"]); len(dirtyFiles) > 0 {
					payload["items"] = dirtyFiles
				}
			} else {
				payload["status"] = "check recorded"
			}
		} else {
			payload["status"] = "ready"
		}
	case "gi-approval-gate":
		payload["summary"] = "Approval gates record sensitive actions with structured context."
		if data, ok := officialLatestCustomData(runtime, "approval_request"); ok {
			payload["latestRequest"] = data
			if object, ok := data.(map[string]any); ok {
				if status := strings.TrimSpace(stringFromAny(object["status"])); status != "" {
					payload["status"] = status
				} else {
					payload["status"] = "request recorded"
				}
				if request, ok := object["request"].(map[string]any); ok {
					officialApplyApprovalRequestPayload(payload, request)
				}
			} else {
				payload["status"] = "request recorded"
			}
		}
		if data, ok := officialLatestCustomData(runtime, "approval_decision"); ok {
			payload["latestDecision"] = data
			if object, ok := data.(map[string]any); ok {
				decision := strings.TrimSpace(stringFromAny(object["decision"]))
				if decision == "" {
					decision = strings.TrimSpace(stringFromAny(object["status"]))
				}
				if decision != "" {
					payload["decision"] = decision
					payload["status"] = decision
					payload["summary"] = "Approval decision: " + decision
				}
				if reason := strings.TrimSpace(stringFromAny(object["reason"])); reason != "" {
					payload["reason"] = reason
				}
				if replacement := strings.TrimSpace(stringFromAny(object["replacement"])); replacement != "" {
					payload["replacement"] = replacement
				}
			}
		}
	case "gi-powerline-footer":
		for key, value := range officialPowerlineFooterPayload(runtime) {
			payload[key] = value
		}
	case "gi-todo-widget":
		if data, ok := officialLatestCustomData(runtime, "todo_state"); ok {
			todos := officialTodosFromData(data)
			payload["todos"] = todos
			if len(todos) == 0 {
				payload["summary"] = "No todos recorded."
			} else {
				payload["summary"] = fmt.Sprintf("%d todo item(s) recorded.", len(todos))
			}
		} else {
			payload["todos"] = []string{}
			payload["summary"] = "No todos recorded."
		}
	case "gi-tools-ui":
		var tools []string
		if runtime != nil {
			tools = runtime.ActiveToolNames()
		}
		payload["tools"] = tools
		payload["summary"] = fmt.Sprintf("%d active tool(s) available.", len(tools))
		if data, ok := officialLatestCustomData(runtime, "tools_state"); ok {
			payload["latestToolsState"] = data
			if object, ok := data.(map[string]any); ok {
				if patched := officialStringListFromAny(firstPresentAny(object, "activeToolNames", "tools")); len(patched) > 0 {
					payload["tools"] = patched
					payload["summary"] = fmt.Sprintf("%d active tool(s) selected.", len(patched))
				}
				if status := strings.TrimSpace(stringFromAny(object["status"])); status != "" {
					payload["status"] = status
				}
			}
		}
	}
	return payload
}

func officialPowerlineFooterPayload(runtime *ProtocolExtensionRuntime) map[string]any {
	payload := map[string]any{
		"summary": "Footer segments require a bound host session.",
	}
	if runtime == nil || runtime.boundSession == nil {
		return payload
	}
	session := runtime.boundSession
	modelLabel := ""
	thinking := ""
	contextLabel := ""
	cwd := ""
	branch := ""
	var tools []string
	if session.Agent != nil {
		model := session.Agent.State.Model
		modelLabel = officialModelLabel(model)
		thinking = strings.TrimSpace(session.Agent.State.ThinkingLevel)
		if model.ContextWindow > 0 {
			contextLabel = fmt.Sprintf("unknown/%d", model.ContextWindow)
		}
	}
	if session.SessionManager != nil {
		cwd = session.SessionManager.GetCWD()
	}
	if cwd != "" {
		provider := NewFooterDataProvider(cwd, FooterDataProviderOptions{DisableWatchers: true})
		branch = strings.TrimSpace(provider.GetGitBranch())
		provider.Dispose()
	}
	if usage := session.GetContextUsage(); usage != nil {
		contextLabel = officialContextUsageLabel(usage)
	}
	tools = runtime.ActiveToolNames()
	payload["model"] = modelLabel
	payload["thinking"] = thinking
	payload["branch"] = branch
	payload["context"] = contextLabel
	payload["tools"] = tools
	payload["toolCount"] = len(tools)
	if summary := officialPowerlineSummary(modelLabel, branch, contextLabel, len(tools)); summary != "" {
		payload["summary"] = summary
	}
	return payload
}

func officialModelLabel(model llm.Model) string {
	provider := strings.TrimSpace(model.Provider)
	id := strings.TrimSpace(model.ID)
	switch {
	case provider != "" && id != "":
		return provider + "/" + id
	case id != "":
		return id
	case provider != "":
		return provider
	default:
		return ""
	}
}

func officialContextUsageLabel(usage *AgentContextUsage) string {
	if usage == nil {
		return ""
	}
	if usage.Tokens != nil && usage.ContextWindow > 0 {
		label := fmt.Sprintf("%d/%d", *usage.Tokens, usage.ContextWindow)
		if usage.Percent != nil {
			label += fmt.Sprintf(" (%.1f%%)", *usage.Percent)
		}
		return label
	}
	if usage.ContextWindow > 0 {
		return fmt.Sprintf("unknown/%d", usage.ContextWindow)
	}
	if usage.Tokens != nil {
		return fmt.Sprintf("%d", *usage.Tokens)
	}
	return ""
}

func officialPowerlineSummary(model, branch, context string, toolCount int) string {
	segments := []string{}
	if model != "" {
		segments = append(segments, "Model: "+model)
	}
	if branch != "" {
		segments = append(segments, "Branch: "+branch)
	}
	if context != "" {
		segments = append(segments, "Context: "+context)
	}
	segments = append(segments, fmt.Sprintf("Tools: %d", toolCount))
	return strings.Join(segments, " | ")
}

func officialApplyApprovalRequestPayload(payload map[string]any, request map[string]any) {
	if payload == nil || request == nil {
		return
	}
	if action := strings.TrimSpace(stringFromAny(firstPresentAny(request, "action", "command", "tool"))); action != "" {
		payload["summary"] = "Approval requested: " + action
	}
	if risk := strings.TrimSpace(stringFromAny(request["risk"])); risk != "" {
		payload["risk"] = risk
	}
	if diff := strings.TrimSpace(stringFromAny(request["diff"])); diff != "" {
		payload["diff"] = diff
	}
	if items := officialApprovalContextLines(request); len(items) > 0 {
		payload["items"] = items
	}
}

func officialApprovalContextLines(request map[string]any) []string {
	if request == nil {
		return nil
	}
	var lines []string
	for _, field := range []struct {
		key   string
		label string
	}{
		{"action", "Action"},
		{"tool", "Tool"},
		{"command", "Command"},
		{"path", "Path"},
		{"risk", "Risk"},
	} {
		if value := strings.TrimSpace(stringFromAny(request[field.key])); value != "" {
			lines = append(lines, field.label+": "+value)
		}
	}
	return lines
}

func officialPackageToolRenderers(ctx *ProtocolExtensionContext) []officialToolRenderer {
	switch officialPackageNameFromSource(ctx) {
	case "gi-plan-mode":
		return []officialToolRenderer{
			{
				Name: "plan_update",
				Definition: ProtocolToolRendererDefinition{
					RenderCall:   officialPlanToolRenderCall("update"),
					RenderResult: officialPlanToolRenderResult,
				},
			},
			{
				Name: "plan_read",
				Definition: ProtocolToolRendererDefinition{
					RenderCall:   officialPlanToolRenderCall("read"),
					RenderResult: officialPlanToolRenderResult,
				},
			},
		}
	case "gi-subagents":
		return []officialToolRenderer{
			{
				Name: "subagent_spawn",
				Definition: ProtocolToolRendererDefinition{
					RenderCall:   officialSubagentSpawnToolRenderCall,
					RenderResult: officialSubagentToolRenderResult,
				},
			},
			{
				Name: "subagent_abort",
				Definition: ProtocolToolRendererDefinition{
					RenderCall:   officialSubagentAbortToolRenderCall,
					RenderResult: officialSubagentToolRenderResult,
				},
			},
		}
	case "gi-approval-gate":
		return []officialToolRenderer{
			{
				Name: "approval_gate_request",
				Definition: ProtocolToolRendererDefinition{
					RenderCall:   officialApprovalRequestToolRenderCall,
					RenderResult: officialApprovalToolRenderResult,
				},
			},
			{
				Name: "approval_gate_decide",
				Definition: ProtocolToolRendererDefinition{
					RenderCall:   officialApprovalDecisionToolRenderCall,
					RenderResult: officialApprovalToolRenderResult,
				},
			},
		}
	case "gi-todo-widget":
		return []officialToolRenderer{
			{
				Name: "todo_write",
				Definition: ProtocolToolRendererDefinition{
					RenderCall:   officialTodoToolRenderCall("write"),
					RenderResult: officialTodoToolRenderResult,
				},
			},
			{
				Name: "todo_read",
				Definition: ProtocolToolRendererDefinition{
					RenderCall:   officialTodoToolRenderCall("read"),
					RenderResult: officialTodoToolRenderResult,
				},
			},
		}
	default:
		return nil
	}
}

func officialPlanToolRenderCall(action string) ToolCallRenderer {
	return func(args any, _ ToolRenderContext) []string {
		state := officialPlanStateFromInput(officialMapFromAny(args))
		return officialPlanRenderLines("Plan "+action, state)
	}
}

func officialPlanToolRenderResult(result FileToolResult, _ ToolRenderResultOptions, _ ToolRenderContext) []string {
	text := strings.TrimSpace(fileToolResultText(result))
	if text == "" {
		return []string{"Plan state recorded."}
	}
	var decoded any
	if err := json.Unmarshal([]byte(text), &decoded); err == nil {
		return officialPlanRenderLines("Plan state", officialMapFromAny(decoded))
	}
	return []string{text}
}

func officialPlanRenderLines(title string, state map[string]any) []string {
	items := officialPlanItemsFromAny(state["items"])
	if len(items) == 0 {
		items = officialPlanItemsFromAny(state["steps"])
	}
	if len(items) == 0 {
		if text := strings.TrimSpace(stringFromAny(state["text"])); text != "" {
			return []string{title + ": " + text}
		}
		return []string{title + ": no items"}
	}
	lines := []string{fmt.Sprintf("%s: %d item(s)", title, len(items))}
	return append(lines, officialPlanPreviewLines(items, 6)...)
}

func officialPlanPreviewLines(items []map[string]any, limit int) []string {
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	lines := make([]string, 0, limit+1)
	for index, item := range items[:limit] {
		step := strings.TrimSpace(stringFromAny(item["step"]))
		if step == "" {
			step = fmt.Sprintf("%d", index+1)
		}
		text := strings.TrimSpace(officialListItemString(item))
		if text == "" {
			continue
		}
		lines = append(lines, step+". "+text)
	}
	if len(items) > limit {
		lines = append(lines, fmt.Sprintf("... %d more", len(items)-limit))
	}
	return lines
}

func officialSubagentSpawnToolRenderCall(args any, _ ToolRenderContext) []string {
	input := officialMapFromAny(args)
	task := firstNonEmptyString(stringFromAny(input["task"]), stringFromAny(input["prompt"]), stringFromAny(input["message"]))
	lines := []string{"Subagent spawn"}
	if task != "" {
		lines = append(lines, "Task: "+task)
	}
	if name := strings.TrimSpace(stringFromAny(input["name"])); name != "" {
		lines = append(lines, "Name: "+name)
	}
	if tools := officialStringListFromAny(input["tools"]); len(tools) > 0 {
		lines = append(lines, "Tools: "+strings.Join(tools, ", "))
	}
	if limit := officialSubagentParentContextLimit(input); limit > 0 {
		lines = append(lines, fmt.Sprintf("Parent context: last %d entr%s", limit, pluralY(limit)))
	}
	return lines
}

func officialSubagentAbortToolRenderCall(args any, _ ToolRenderContext) []string {
	input := officialMapFromAny(args)
	target := firstNonEmptyString(stringFromAny(input["target"]), "children")
	return []string{"Subagent abort", "Target: " + target}
}

func officialSubagentToolRenderResult(result FileToolResult, _ ToolRenderResultOptions, _ ToolRenderContext) []string {
	text := strings.TrimSpace(fileToolResultText(result))
	if text == "" {
		return []string{"Subagent state recorded."}
	}
	var decoded any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return []string{text}
	}
	object := officialMapFromAny(decoded)
	lines := []string{}
	if boolFromAny(object["aborted"]) {
		lines = append(lines, "Subagent aborted")
	} else if status := strings.TrimSpace(stringFromAny(object["status"])); status != "" {
		lines = append(lines, "Subagent status: "+status)
	} else {
		lines = append(lines, "Subagent completed")
	}
	if sessionID := strings.TrimSpace(stringFromAny(object["sessionId"])); sessionID != "" {
		lines = append(lines, "Session: "+sessionID)
	}
	if parentContext := officialSubagentContextLines(object["parentContext"]); len(parentContext) > 0 {
		lines = append(lines, fmt.Sprintf("Parent context: %d entr%s", len(parentContext), pluralY(len(parentContext))))
		lines = append(lines, parentContext...)
	}
	if text := strings.TrimSpace(stringFromAny(object["lastAssistantText"])); text != "" {
		lines = append(lines, "Result: "+text)
	}
	return lines
}

func officialApprovalRequestToolRenderCall(args any, _ ToolRenderContext) []string {
	request := officialMapFromAny(args)
	lines := []string{"Approval request"}
	if context := officialApprovalContextLines(request); len(context) > 0 {
		lines = append(lines, context...)
	}
	if diff := strings.TrimSpace(stringFromAny(request["diff"])); diff != "" {
		lines = append(lines, "Diff:")
		lines = append(lines, officialPreviewMultiline(diff, 6)...)
	}
	return lines
}

func officialApprovalDecisionToolRenderCall(args any, _ ToolRenderContext) []string {
	decision := officialApprovalDecisionFromInput(officialMapFromAny(args))
	return officialApprovalDecisionLines(decision)
}

func officialApprovalToolRenderResult(result FileToolResult, _ ToolRenderResultOptions, _ ToolRenderContext) []string {
	text := strings.TrimSpace(fileToolResultText(result))
	if text == "" {
		return []string{"Approval state recorded."}
	}
	var decoded any
	if err := json.Unmarshal([]byte(text), &decoded); err == nil {
		object := officialMapFromAny(decoded)
		if request, ok := object["request"].(map[string]any); ok {
			return officialApprovalRequestToolRenderCall(request, ToolRenderContext{})
		}
		if lines := officialApprovalDecisionLines(object); len(lines) > 0 {
			return lines
		}
	}
	return []string{text}
}

func officialApprovalDecisionLines(data map[string]any) []string {
	decision := strings.TrimSpace(stringFromAny(firstPresentAny(data, "decision", "status")))
	if decision == "" {
		return nil
	}
	lines := []string{"Approval decision: " + decision}
	if reason := strings.TrimSpace(stringFromAny(data["reason"])); reason != "" {
		lines = append(lines, "Reason: "+reason)
	}
	if replacement := strings.TrimSpace(stringFromAny(data["replacement"])); replacement != "" {
		lines = append(lines, "Replacement: "+replacement)
	}
	return lines
}

func officialTodoToolRenderCall(action string) ToolCallRenderer {
	return func(args any, _ ToolRenderContext) []string {
		todos := officialTodosFromData(args)
		if len(todos) == 0 {
			return []string{"Todo: " + action + " current session todos"}
		}
		lines := []string{fmt.Sprintf("Todo: %s %d item(s)", action, len(todos))}
		return append(lines, officialTodoPreviewLines(todos, 5)...)
	}
}

func officialTodoToolRenderResult(result FileToolResult, _ ToolRenderResultOptions, _ ToolRenderContext) []string {
	text := strings.TrimSpace(fileToolResultText(result))
	if text == "" {
		return []string{"Todo state updated."}
	}
	var decoded any
	if err := json.Unmarshal([]byte(text), &decoded); err == nil {
		todos := officialTodosFromData(decoded)
		if len(todos) > 0 {
			lines := []string{fmt.Sprintf("Todo state: %d item(s)", len(todos))}
			return append(lines, officialTodoPreviewLines(todos, 5)...)
		}
	}
	return []string{text}
}

func officialTodoPreviewLines(todos []string, limit int) []string {
	if limit <= 0 || limit > len(todos) {
		limit = len(todos)
	}
	lines := make([]string, 0, limit+1)
	for _, todo := range todos[:limit] {
		lines = append(lines, "- "+todo)
	}
	if len(todos) > limit {
		lines = append(lines, fmt.Sprintf("... %d more", len(todos)-limit))
	}
	return lines
}

func officialTodosFromData(data any) []string {
	if object, ok := data.(map[string]any); ok {
		return officialStringListFromAny(object["todos"])
	}
	return officialStringListFromAny(data)
}

func officialStringListFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return compactOfficialStringList(typed)
	case []map[string]any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, officialListItemString(item))
		}
		return compactOfficialStringList(values)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, officialListItemString(item))
		}
		return compactOfficialStringList(values)
	case map[string]any:
		for _, key := range []string{"todos", "tools", "items", "values"} {
			if values := officialStringListFromAny(typed[key]); len(values) > 0 {
				return values
			}
		}
		if text := officialListItemString(typed); text != "" {
			return []string{text}
		}
	case nil:
		return nil
	default:
		if text := strings.TrimSpace(stringFromAny(typed)); text != "" {
			return []string{text}
		}
	}
	return nil
}

func officialListItemString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"content", "text", "title", "task", "name", "label"} {
			if text := strings.TrimSpace(stringFromAny(typed[key])); text != "" {
				if status := strings.TrimSpace(stringFromAny(typed["status"])); status != "" {
					return text + " (" + status + ")"
				}
				return text
			}
		}
		return ""
	default:
		return strings.TrimSpace(stringFromAny(typed))
	}
}

func compactOfficialStringList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func officialMessagePayload(message any) map[string]any {
	text := officialMessageText(message)
	var payload map[string]any
	if strings.TrimSpace(text) != "" && json.Unmarshal([]byte(text), &payload) == nil {
		return payload
	}
	return map[string]any{"text": text}
}

func officialMessageText(message any) string {
	switch typed := message.(type) {
	case llm.Message:
		return interactiveTextFromLLMMessage(typed)
	case *llm.Message:
		if typed == nil {
			return ""
		}
		return interactiveTextFromLLMMessage(*typed)
	case string:
		return typed
	default:
		return customMessageText(typed)
	}
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func officialSubagentRunParams(input map[string]any) hostAgentRunParams {
	return hostAgentRunParams{
		Prompt:        firstNonEmptyString(stringFromAny(input["prompt"]), stringFromAny(input["task"]), stringFromAny(input["message"])),
		Task:          stringFromAny(input["task"]),
		Message:       stringFromAny(input["message"]),
		Name:          stringFromAny(input["name"]),
		CWD:           stringFromAny(input["cwd"]),
		Tools:         officialStringListFromAny(input["tools"]),
		NoTools:       stringFromAny(input["noTools"]),
		ParentSession: stringFromAny(input["parentSession"]),
	}
}

func officialSubagentParentContext(ctx *ProtocolExtensionContext, input map[string]any) []map[string]any {
	limit := officialSubagentParentContextLimit(input)
	if limit <= 0 || ctx == nil || ctx.runtime == nil {
		return nil
	}
	entries := ctx.runtime.SessionEntries()
	context := make([]map[string]any, 0, limit)
	for index := len(entries) - 1; index >= 0 && len(context) < limit; index-- {
		entry := entries[index]
		if entry.Type != "message" {
			continue
		}
		role, text := officialSubagentEntryRoleText(entry)
		if role == "" || text == "" {
			continue
		}
		context = append(context, map[string]any{"role": role, "text": text})
	}
	for left, right := 0, len(context)-1; left < right; left, right = left+1, right-1 {
		context[left], context[right] = context[right], context[left]
	}
	return context
}

func officialSubagentEntryRoleText(entry FileEntry) (string, string) {
	switch message := entry.Message.(type) {
	case map[string]any:
		return strings.TrimSpace(stringFromAny(message["role"])), strings.TrimSpace(extractMessageText(message))
	case llm.Message:
		return strings.TrimSpace(string(message.Role)), strings.TrimSpace(interactiveTextFromLLMMessage(message))
	case *llm.Message:
		if message == nil {
			return "", ""
		}
		return strings.TrimSpace(string(message.Role)), strings.TrimSpace(interactiveTextFromLLMMessage(*message))
	default:
		return "", ""
	}
}

func officialSubagentParentContextLimit(input map[string]any) int {
	limit := officialIntFromAny(firstPresentAny(input, "parentContextEntries", "contextEntries", "parentContextLimit"))
	if limit <= 0 && boolFromAny(firstPresentAny(input, "includeParentContext", "parentContext")) {
		limit = 4
	}
	if limit > 20 {
		limit = 20
	}
	return limit
}

func officialSubagentContextLines(value any) []string {
	switch typed := value.(type) {
	case []map[string]any:
		lines := make([]string, 0, len(typed))
		for _, item := range typed {
			if line := officialSubagentContextLine(item); line != "" {
				lines = append(lines, line)
			}
		}
		return lines
	case []any:
		lines := make([]string, 0, len(typed))
		for _, item := range typed {
			if object, ok := item.(map[string]any); ok {
				if line := officialSubagentContextLine(object); line != "" {
					lines = append(lines, line)
				}
			}
		}
		return lines
	default:
		return nil
	}
}

func officialSubagentContextLine(item map[string]any) string {
	role := strings.TrimSpace(stringFromAny(item["role"]))
	text := strings.TrimSpace(stringFromAny(item["text"]))
	if role == "" || text == "" {
		return ""
	}
	return role + ": " + text
}

func officialSubagentAbortParams(input map[string]any) hostAgentAbortParams {
	return hostAgentAbortParams{
		Target:      firstNonEmptyString(stringFromAny(input["target"]), "children"),
		SessionID:   stringFromAny(input["sessionId"]),
		SessionFile: stringFromAny(input["sessionFile"]),
	}
}

func officialPlanStateFromInput(input map[string]any) map[string]any {
	text := firstNonEmptyString(stringFromAny(input["text"]), stringFromAny(input["plan"]), stringFromAny(input["message"]))
	items := officialPlanItemsFromAny(input["items"])
	if len(items) == 0 {
		items = officialPlanItemsFromAny(input["steps"])
	}
	if len(items) == 0 && text != "" {
		for _, item := range ExtractPlanTodoItems(text) {
			status := "pending"
			if item.Completed {
				status = "done"
			}
			items = append(items, map[string]any{"step": item.Step, "text": item.Text, "status": status})
		}
	}
	return map[string]any{
		"text":  text,
		"items": items,
		"raw":   input,
	}
}

func officialPlanItemsFromAny(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), typed...)
	case []any:
		items := make([]map[string]any, 0, len(typed))
		for index, item := range typed {
			switch value := item.(type) {
			case map[string]any:
				items = append(items, value)
			default:
				text := strings.TrimSpace(stringFromAny(value))
				if text != "" {
					items = append(items, map[string]any{"step": index + 1, "text": text, "status": "pending"})
				}
			}
		}
		return items
	default:
		return nil
	}
}

func officialMCPRegisterDynamicTools(ctx *ProtocolExtensionContext, options mcpStdioOptions, data any) error {
	if ctx == nil || ctx.runtime == nil {
		return nil
	}
	tools := officialMCPToolInfosFromData(data)
	for _, tool := range tools {
		tool.Name = strings.TrimSpace(tool.Name)
		if tool.Name == "" {
			continue
		}
		registeredName := officialMCPDynamicToolName(tool.Name)
		if registeredName == "" || officialRuntimeHasTool(ctx.runtime, registeredName) {
			continue
		}
		capturedTool := tool
		capturedOptions := cloneMCPStdioOptions(options)
		description := firstNonEmptyString(strings.TrimSpace(capturedTool.Description), "Call MCP tool "+capturedTool.Name)
		if err := ctx.RegisterTool(ProtocolToolDefinition{
			Name:          registeredName,
			Label:         "MCP: " + capturedTool.Name,
			Description:   description,
			PromptSnippet: description,
			Execute: func(_ string, input map[string]any) (SDKToolResult, error) {
				result, err := runMCPStdioCallTool(capturedOptions, capturedTool.Name, input)
				if err != nil {
					_, _ = ctx.runtime.AppendCustomEntry("mcp_call", map[string]any{"status": "failed", "input": input, "tool": capturedTool.Name, "error": err.Error()})
					return SDKToolResult{}, err
				}
				callData := map[string]any{"status": "done", "tool": capturedTool.Name, "input": input, "result": result}
				if progress := officialMCPProgressFromData(result); len(progress) > 0 {
					callData["progress"] = progress
				}
				if officialMCPToolsListChangedFromData(result) {
					callData["toolsListChanged"] = true
				}
				if _, err := ctx.runtime.AppendCustomEntry("mcp_call", callData); err != nil {
					return SDKToolResult{}, err
				}
				return officialJSONToolResult(callData)
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func officialMCPToolInfosFromData(data any) []officialMCPToolInfo {
	switch typed := data.(type) {
	case []officialMCPToolInfo:
		return append([]officialMCPToolInfo(nil), typed...)
	case []map[string]any:
		result := make([]officialMCPToolInfo, 0, len(typed))
		for _, item := range typed {
			if info := officialMCPToolInfoFromMap(item); info.Name != "" {
				result = append(result, info)
			}
		}
		return result
	case []any:
		result := make([]officialMCPToolInfo, 0, len(typed))
		for _, item := range typed {
			result = append(result, officialMCPToolInfosFromData(item)...)
		}
		return result
	case map[string]any:
		if info := officialMCPToolInfoFromMap(typed); info.Name != "" {
			return []officialMCPToolInfo{info}
		}
		for _, key := range []string{"tools", "result", "latestTools"} {
			if tools := officialMCPToolInfosFromData(typed[key]); len(tools) > 0 {
				return tools
			}
		}
	}
	return nil
}

func officialMCPToolInfoFromMap(item map[string]any) officialMCPToolInfo {
	return officialMCPToolInfo{
		Name:        strings.TrimSpace(stringFromAny(item["name"])),
		Description: strings.TrimSpace(stringFromAny(item["description"])),
	}
}

func officialMCPDynamicToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("mcp")
	lastUnderscore := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	result := strings.Trim(b.String(), "_")
	if result == "mcp" {
		return ""
	}
	if !strings.HasPrefix(result, "mcp_") {
		result = "mcp_" + strings.TrimPrefix(result, "mcp")
	}
	return result
}

func officialRuntimeHasTool(runtime *ProtocolExtensionRuntime, name string) bool {
	if runtime == nil || strings.TrimSpace(name) == "" {
		return false
	}
	for _, tool := range runtime.RegisteredTools() {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func cloneMCPStdioOptions(options mcpStdioOptions) mcpStdioOptions {
	clone := mcpStdioOptions{
		Command: append([]string(nil), options.Command...),
		Env:     map[string]string{},
		Timeout: options.Timeout,
	}
	for key, value := range options.Env {
		clone.Env[key] = value
	}
	if len(clone.Env) == 0 {
		clone.Env = nil
	}
	return clone
}

func officialMCPOptionsFromInput(input map[string]any) (mcpStdioOptions, bool, error) {
	command := officialCommandListFromAny(firstPresentAny(input, "command", "cmd"))
	if len(command) == 0 {
		return mcpStdioOptions{}, false, nil
	}
	env, err := officialStringMapFromAny(input["env"])
	if err != nil {
		return mcpStdioOptions{}, true, err
	}
	options := mcpStdioOptions{Command: command, Env: env}
	if timeout := officialMillisecondsDurationFromAny(input["timeoutMillis"]); timeout > 0 {
		options.Timeout = timeout
	} else if timeout := officialDurationFromAny(firstPresentAny(input, "timeout", "timeoutSeconds")); timeout > 0 {
		options.Timeout = timeout
	}
	return options, true, nil
}

func officialCommandListFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return compactOfficialStringList(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, stringFromAny(item))
		}
		return compactOfficialStringList(values)
	case string:
		return compactOfficialStringList(strings.Fields(typed))
	default:
		text := strings.TrimSpace(stringFromAny(typed))
		if text == "" {
			return nil
		}
		return []string{text}
	}
}

func officialStringMapFromAny(value any) (map[string]string, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case map[string]string:
		result := map[string]string{}
		for key, value := range typed {
			if strings.TrimSpace(key) != "" {
				result[key] = value
			}
		}
		return result, nil
	case map[string]any:
		result := map[string]string{}
		for key, value := range typed {
			if strings.TrimSpace(key) != "" {
				result[key] = stringFromAny(value)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("env must be an object")
	}
}

func officialDurationFromAny(value any) time.Duration {
	switch typed := value.(type) {
	case int:
		return time.Duration(typed) * time.Second
	case int64:
		return time.Duration(typed) * time.Second
	case float64:
		return time.Duration(typed * float64(time.Second))
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return 0
		}
		if duration, err := time.ParseDuration(text); err == nil {
			return duration
		}
		var seconds float64
		if _, err := fmt.Sscan(text, &seconds); err == nil && seconds > 0 {
			return time.Duration(seconds * float64(time.Second))
		}
	}
	return 0
}

func officialMillisecondsDurationFromAny(value any) time.Duration {
	switch typed := value.(type) {
	case int:
		return time.Duration(typed) * time.Millisecond
	case int64:
		return time.Duration(typed) * time.Millisecond
	case float64:
		return time.Duration(typed * float64(time.Millisecond))
	case string:
		var milliseconds float64
		if _, err := fmt.Sscan(strings.TrimSpace(typed), &milliseconds); err == nil && milliseconds > 0 {
			return time.Duration(milliseconds * float64(time.Millisecond))
		}
	}
	return 0
}

func officialMCPArgumentsFromInput(input map[string]any) map[string]any {
	value := firstPresentAny(input, "arguments", "args")
	if value == nil {
		return map[string]any{}
	}
	if object, ok := value.(map[string]any); ok {
		return object
	}
	return map[string]any{"value": value}
}

func officialMCPToolsFromData(data any) []string {
	if object, ok := data.(map[string]any); ok {
		if tools := officialStringListFromAny(object["tools"]); len(tools) > 0 {
			return tools
		}
		if result, ok := object["result"]; ok {
			return officialMCPToolsFromData(result)
		}
		if latest, ok := object["latestTools"]; ok {
			return officialMCPToolsFromData(latest)
		}
	}
	return officialStringListFromAny(data)
}

func officialMCPProgressFromData(data any) []map[string]any {
	switch typed := data.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), typed...)
	case []any:
		progress := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if object, ok := item.(map[string]any); ok {
				progress = append(progress, object)
			}
		}
		return progress
	case map[string]any:
		for _, key := range []string{"progress", "_progress", "mcpProgress"} {
			if progress := officialMCPProgressFromData(typed[key]); len(progress) > 0 {
				return progress
			}
		}
		for _, key := range []string{"result", "latestCall", "latestTools"} {
			if progress := officialMCPProgressFromData(typed[key]); len(progress) > 0 {
				return progress
			}
		}
	}
	return nil
}

func officialMCPProgressLines(data any) []string {
	progress := officialMCPProgressFromData(data)
	lines := make([]string, 0, len(progress))
	for _, item := range progress {
		if line := officialMCPProgressLine(item); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func officialMCPProgressLine(item map[string]any) string {
	message := strings.TrimSpace(stringFromAny(firstPresentAny(item, "message", "text", "title")))
	value := strings.TrimSpace(stringFromAny(item["progress"]))
	total := strings.TrimSpace(stringFromAny(item["total"]))
	switch {
	case message != "" && value != "" && total != "":
		return message + " (" + value + "/" + total + ")"
	case message != "" && value != "":
		return message + " (" + value + ")"
	case message != "":
		return message
	case value != "" && total != "":
		return value + "/" + total
	default:
		return value
	}
}

func officialMCPToolsListChangedFromData(data any) bool {
	if object, ok := data.(map[string]any); ok {
		for _, key := range []string{"toolsListChanged", "_toolsListChanged", "mcpToolsListChanged"} {
			if boolFromAny(object[key]) {
				return true
			}
		}
		for _, key := range []string{"result", "latestCall", "latestTools"} {
			if officialMCPToolsListChangedFromData(object[key]) {
				return true
			}
		}
	}
	return false
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return false
	}
}

func firstPresentAny(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
}

func officialMapFromAny(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case string:
		var decoded map[string]any
		if json.Unmarshal([]byte(typed), &decoded) == nil {
			return decoded
		}
	}
	return map[string]any{}
}

func officialIntFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed); err == nil {
			return parsed
		}
	}
	return 0
}

func pluralY(count int) string {
	if count == 1 {
		return "y"
	}
	return "ies"
}

func officialPreviewMultiline(text string, limit int) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
		if limit > 0 && len(lines) == limit {
			break
		}
	}
	if limit > 0 && len(lines) == limit && len(strings.Split(strings.TrimSpace(text), "\n")) > limit {
		lines = append(lines, fmt.Sprintf("... %d more line(s)", len(strings.Split(strings.TrimSpace(text), "\n"))-limit))
	}
	return lines
}

func officialToolExecutor(ctx *ProtocolExtensionContext, toolName string) func(string, map[string]any) (SDKToolResult, error) {
	packageName := officialPackageNameFromSource(ctx)
	if packageName == "" {
		return nil
	}
	return func(_ string, input map[string]any) (SDKToolResult, error) {
		switch toolName {
		case "plan_update":
			data := officialPlanStateFromInput(input)
			if _, err := ctx.runtime.AppendCustomEntry("plan_state", data); err != nil {
				return SDKToolResult{}, err
			}
			return officialJSONToolResult(data)
		case "plan_read":
			if data, ok := officialLatestCustomData(ctx.runtime, "plan_state"); ok {
				return officialJSONToolResult(data)
			}
			return officialJSONToolResult(map[string]any{"items": []any{}})
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
			allTools := make([]string, 0)
			if ctx != nil && ctx.runtime != nil && ctx.runtime.boundSession != nil {
				for _, tool := range hostActionRegisteredTools(ctx.runtime.boundSession) {
					allTools = append(allTools, tool.Name)
				}
			}
			return officialJSONToolResult(map[string]any{"tools": allTools, "activeToolNames": ctx.runtime.ActiveToolNames()})
		case "tools_ui_set_active":
			mode := firstNonEmptyString(strings.TrimSpace(stringFromAny(input["mode"])), "replace")
			toolNames := officialStringListFromAny(firstPresentAny(input, "toolNames", "tools", "names"))
			activeNames, err := officialSetActiveTools(ctx, mode, toolNames)
			if err != nil {
				return SDKToolResult{}, err
			}
			data := map[string]any{"status": "updated", "mode": mode, "activeToolNames": activeNames}
			if _, err := ctx.runtime.AppendCustomEntry("tools_state", data); err != nil {
				return SDKToolResult{}, err
			}
			return officialJSONToolResult(data)
		case "approval_gate_request":
			data := map[string]any{"status": "pending", "request": input}
			if _, err := ctx.runtime.AppendCustomEntry("approval_request", data); err != nil {
				return SDKToolResult{}, err
			}
			return officialJSONToolResult(data)
		case "approval_gate_decide":
			data := officialApprovalDecisionFromInput(input)
			if _, err := ctx.runtime.AppendCustomEntry("approval_decision", data); err != nil {
				return SDKToolResult{}, err
			}
			return officialJSONToolResult(data)
		case "git_guard_check":
			data := officialGitGuardData(ctx, input)
			if err := officialGitGuardConfirmIfRequested(ctx, input, data); err != nil {
				return SDKToolResult{}, err
			}
			if _, err := ctx.runtime.AppendCustomEntry("git_guard_check", data); err != nil {
				return SDKToolResult{}, err
			}
			return officialJSONToolResult(data)
		case "subagent_spawn":
			params := officialSubagentRunParams(input)
			parentContext := officialSubagentParentContext(ctx, input)
			result, err := officialRunChildAgent(ctx, params)
			if err != nil {
				if _, appendErr := ctx.runtime.AppendCustomEntry("subagent_request", map[string]any{"status": "failed", "input": input, "error": err.Error()}); appendErr != nil {
					return SDKToolResult{}, appendErr
				}
				return SDKToolResult{}, err
			}
			status := "done"
			if boolFromAny(result["aborted"]) {
				status = "aborted"
			}
			data := map[string]any{"status": status, "input": input, "result": result}
			if len(parentContext) > 0 {
				result["parentContext"] = parentContext
				result["parentContextCount"] = len(parentContext)
				data["parentContext"] = parentContext
			}
			if _, err := ctx.runtime.AppendCustomEntry("subagent_request", data); err != nil {
				return SDKToolResult{}, err
			}
			return officialJSONToolResult(result)
		case "subagent_abort":
			params := officialSubagentAbortParams(input)
			aborted, err := officialAbortChildAgent(ctx, params)
			if err != nil {
				return SDKToolResult{}, err
			}
			status := "no matching child"
			if aborted {
				status = "aborted"
			}
			data := map[string]any{"status": status, "aborted": aborted, "input": input}
			if _, err := ctx.runtime.AppendCustomEntry("subagent_abort", data); err != nil {
				return SDKToolResult{}, err
			}
			return officialJSONToolResult(data)
		case "mcp_list_tools":
			options, hasCommand, err := officialMCPOptionsFromInput(input)
			if err != nil {
				return SDKToolResult{}, err
			}
			if !hasCommand {
				if data, ok := officialLatestCustomData(ctx.runtime, "mcp_tools"); ok {
					return officialJSONToolResult(data)
				}
				return officialJSONToolResult(map[string]any{"tools": []any{}})
			}
			result, err := runMCPStdioListTools(options)
			if err != nil {
				_, _ = ctx.runtime.AppendCustomEntry("mcp_tools", map[string]any{"status": "failed", "input": input, "error": err.Error()})
				return SDKToolResult{}, err
			}
			data := map[string]any{"status": "tools discovered", "result": result, "tools": result["tools"]}
			if progress := officialMCPProgressFromData(result); len(progress) > 0 {
				data["progress"] = progress
			}
			if officialMCPToolsListChangedFromData(result) {
				data["toolsListChanged"] = true
			}
			if _, err := ctx.runtime.AppendCustomEntry("mcp_tools", data); err != nil {
				return SDKToolResult{}, err
			}
			if err := officialMCPRegisterDynamicTools(ctx, options, result); err != nil {
				return SDKToolResult{}, err
			}
			return officialJSONToolResult(data)
		case "mcp_call":
			options, hasCommand, err := officialMCPOptionsFromInput(input)
			if err != nil {
				return SDKToolResult{}, err
			}
			tool := firstNonEmptyString(stringFromAny(input["tool"]), stringFromAny(input["name"]))
			if hasCommand {
				result, err := runMCPStdioCallTool(options, tool, officialMCPArgumentsFromInput(input))
				if err != nil {
					_, _ = ctx.runtime.AppendCustomEntry("mcp_call", map[string]any{"status": "failed", "input": input, "tool": tool, "error": err.Error()})
					return SDKToolResult{}, err
				}
				data := map[string]any{"status": "done", "tool": tool, "input": input, "result": result}
				if progress := officialMCPProgressFromData(result); len(progress) > 0 {
					data["progress"] = progress
				}
				if officialMCPToolsListChangedFromData(result) {
					data["toolsListChanged"] = true
				}
				if _, err := ctx.runtime.AppendCustomEntry("mcp_call", data); err != nil {
					return SDKToolResult{}, err
				}
				return officialJSONToolResult(data)
			}
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

func officialGitGuardData(ctx *ProtocolExtensionContext, input map[string]any) map[string]any {
	data := map[string]any{"status": "check recorded", "input": input}
	if action := strings.TrimSpace(stringFromAny(input["action"])); action != "" {
		data["action"] = action
	}
	cwd := officialGitGuardCWD(ctx, input)
	if cwd == "" {
		return data
	}
	data["cwd"] = cwd
	output, err := officialGitStatusShort(ctx, cwd)
	if err != nil {
		data["error"] = err.Error()
		return data
	}
	branch, dirtyFiles := officialParseGitStatus(output)
	data["status"] = "checked"
	data["statusShort"] = output
	if branch != "" {
		data["branch"] = branch
	}
	data["dirtyFiles"] = dirtyFiles
	if len(dirtyFiles) == 0 {
		data["clean"] = true
	}
	return data
}

func officialGitGuardConfirmIfRequested(ctx *ProtocolExtensionContext, input map[string]any, data map[string]any) error {
	if !boolFromAny(firstPresentAny(input, "confirm", "requireConfirmation", "dialog")) {
		return nil
	}
	request := TUIDialogRequest{
		Kind:    "confirm",
		Title:   "Git Guard",
		Message: officialGitGuardDialogMessage(data),
	}
	params, err := json.Marshal(request)
	if err != nil {
		return err
	}
	response := officialHostActionHost(ctx).HandleHostAction(HostActionRequest{
		ID:     "git_guard_confirm",
		Method: "host.tui.dialog",
		Params: params,
	})
	if response.Error != nil {
		data["status"] = "confirmation unavailable"
		data["dialogError"] = response.Error.Message
		return nil
	}
	result, ok := response.Result.(TUIDialogResult)
	if !ok {
		data["status"] = "confirmation unavailable"
		data["dialogError"] = "unexpected dialog result"
		return nil
	}
	data["dialog"] = result
	switch result.Action {
	case "confirmed", "selected", "submitted":
		data["status"] = "confirmed"
		data["decision"] = "confirmed"
	case "declined":
		data["status"] = "declined"
		data["decision"] = "declined"
	default:
		data["status"] = "cancelled"
		data["decision"] = "cancelled"
	}
	return nil
}

func officialGitGuardDialogMessage(data map[string]any) string {
	action := strings.TrimSpace(stringFromAny(data["action"]))
	branch := strings.TrimSpace(stringFromAny(data["branch"]))
	dirtyFiles := officialStringListFromAny(data["dirtyFiles"])
	parts := []string{}
	if action != "" {
		parts = append(parts, "Action: "+action)
	}
	if branch != "" {
		parts = append(parts, "Branch: "+branch)
	}
	if len(dirtyFiles) > 0 {
		parts = append(parts, fmt.Sprintf("Dirty files: %d", len(dirtyFiles)))
	} else if boolFromAny(data["clean"]) {
		parts = append(parts, "Workspace clean")
	}
	if len(parts) == 0 {
		return "Confirm guarded git action?"
	}
	return strings.Join(parts, "\n")
}

func officialApprovalDecisionFromInput(input map[string]any) map[string]any {
	decision := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
		stringFromAny(input["decision"]),
		stringFromAny(input["status"]),
		stringFromAny(input["action"]),
	)))
	switch decision {
	case "approve", "approved":
		decision = "approved"
	case "deny", "denied", "reject", "rejected", "block", "blocked":
		decision = "denied"
	case "rewrite", "rewritten":
		decision = "rewrite"
	case "":
		decision = "approved"
	}
	data := map[string]any{"status": decision, "decision": decision, "input": input}
	if reason := strings.TrimSpace(stringFromAny(input["reason"])); reason != "" {
		data["reason"] = reason
	}
	if replacement := strings.TrimSpace(stringFromAny(input["replacement"])); replacement != "" {
		data["replacement"] = replacement
	}
	return data
}

func officialGitGuardCWD(ctx *ProtocolExtensionContext, input map[string]any) string {
	if cwd := strings.TrimSpace(stringFromAny(input["cwd"])); cwd != "" {
		return cwd
	}
	if ctx != nil && ctx.runtime != nil && ctx.runtime.boundSession != nil && ctx.runtime.boundSession.SessionManager != nil {
		return ctx.runtime.boundSession.SessionManager.GetCWD()
	}
	return ""
}

func officialGitStatusShort(ctx *ProtocolExtensionContext, cwd string) (string, error) {
	if output, handled, err := officialGitStatusShortViaHostAction(ctx, cwd); handled {
		return output, err
	}
	runCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "git", "--no-optional-locks", "-C", cwd, "status", "--short", "--branch")
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text != "" {
			return "", fmt.Errorf("%w: %s", err, text)
		}
		return "", err
	}
	return text, nil
}

func officialGitStatusShortViaHostAction(ctx *ProtocolExtensionContext, cwd string) (string, bool, error) {
	host := officialHostActionHost(ctx)
	if host == nil || host.ProcessExecutor == nil {
		return "", false, nil
	}
	params, err := json.Marshal(hostProcessExecParams{
		Command:       []string{"git", "--no-optional-locks", "status", "--short", "--branch"},
		CWD:           cwd,
		TimeoutMillis: 2000,
	})
	if err != nil {
		return "", true, err
	}
	response := host.HandleHostAction(HostActionRequest{
		ID:     "git_guard_status",
		Method: "host.process.exec",
		Params: params,
	})
	if response.Error != nil {
		return "", true, errors.New(response.Error.Message)
	}
	result, ok := response.Result.(HostProcessResult)
	if !ok {
		return "", true, errors.New("unexpected host.process.exec result")
	}
	text := strings.TrimSpace(result.Stdout)
	if result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = text
		}
		if message != "" {
			return "", true, fmt.Errorf("git status exited with code %d: %s", result.ExitCode, message)
		}
		return "", true, fmt.Errorf("git status exited with code %d", result.ExitCode)
	}
	return text, true, nil
}

func officialParseGitStatus(output string) (string, []string) {
	var branch string
	var dirtyFiles []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			branch = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}
		dirtyFiles = append(dirtyFiles, line)
	}
	return branch, dirtyFiles
}

func officialSetActiveTools(ctx *ProtocolExtensionContext, mode string, toolNames []string) ([]string, error) {
	params, err := json.Marshal(hostToolsSetActiveParams{Mode: mode, ToolNames: toolNames})
	if err != nil {
		return nil, err
	}
	response := officialHostActionHost(ctx).HandleHostAction(HostActionRequest{
		ID:     "official_tools_set_active",
		Method: "host.tools.set_active",
		Params: params,
	})
	if response.Error != nil {
		return nil, errors.New(response.Error.Message)
	}
	result, ok := response.Result.(map[string]any)
	if !ok {
		return nil, errors.New("unexpected host.tools.set_active result")
	}
	return officialStringListFromAny(result["activeToolNames"]), nil
}

func officialRunChildAgent(ctx *ProtocolExtensionContext, params hostAgentRunParams) (map[string]any, error) {
	payload, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	response := officialHostActionHost(ctx).HandleHostAction(HostActionRequest{
		ID:     "official_subagent_spawn",
		Method: "host.agent.spawn",
		Params: payload,
	})
	if response.Error != nil {
		return nil, errors.New(response.Error.Message)
	}
	result, ok := response.Result.(map[string]any)
	if !ok {
		return nil, errors.New("unexpected host.agent.spawn result")
	}
	return result, nil
}

func officialAbortChildAgent(ctx *ProtocolExtensionContext, params hostAgentAbortParams) (bool, error) {
	payload, err := json.Marshal(params)
	if err != nil {
		return false, err
	}
	response := officialHostActionHost(ctx).HandleHostAction(HostActionRequest{
		ID:     "official_subagent_abort",
		Method: "host.agent.abort",
		Params: payload,
	})
	if response.Error != nil {
		return false, errors.New(response.Error.Message)
	}
	result, ok := response.Result.(map[string]any)
	if !ok {
		return false, errors.New("unexpected host.agent.abort result")
	}
	return boolFromAny(result["aborted"]), nil
}

func officialHostActionHost(ctx *ProtocolExtensionContext) *RPCSessionHost {
	if ctx == nil || ctx.runtime == nil || ctx.runtime.boundSession == nil {
		return NewRPCSessionHost(nil)
	}
	runtime := ctx.runtime
	if runtime.hostActionHost == nil || runtime.hostActionSession != runtime.boundSession {
		runtime.hostActionHost = NewRPCSessionHost(runtime.boundSession)
		runtime.hostActionSession = runtime.boundSession
	}
	if runtime.viewTreeHost != nil {
		runtime.hostActionHost.ViewTreeHost = runtime.viewTreeHost
	}
	return runtime.hostActionHost
}

func officialKnownToolSubset(ctx *ProtocolExtensionContext, names []string) []string {
	if ctx == nil || ctx.runtime == nil || ctx.runtime.boundSession == nil {
		return nil
	}
	known := map[string]bool{}
	for _, tool := range hostActionRegisteredTools(ctx.runtime.boundSession) {
		known[tool.Name] = true
	}
	result := make([]string, 0, len(names))
	for _, name := range names {
		if known[name] {
			result = append(result, name)
		}
	}
	return result
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
	if runtime == nil {
		return nil, false
	}
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
