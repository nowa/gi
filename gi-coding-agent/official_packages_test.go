package gicodingagent

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestOfficialPackagesProvideExecutableProtocolContributions(t *testing.T) {
	sources := make([]any, 0, len(OfficialPackageNames()))
	for _, name := range OfficialPackageNames() {
		sources = append(sources, "official:"+name)
	}
	loader, runtime, _ := newOfficialPackageTestRuntime(t, sources...)
	if runtime == nil {
		t.Fatal("runtime is nil")
	}

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
	if renderer := runtime.GetMessageRenderer("gi.todo.widget"); renderer == nil {
		t.Fatal("official todo renderer missing")
	} else if lines := renderer(llm.Message{Role: "custom", CustomType: "gi.todo.widget", Content: []llm.ContentPart{llm.Text(`{"command":"todo"}`)}}, nil); !strings.Contains(strings.Join(lines, "\n"), "Todo Widget") {
		t.Fatalf("official todo renderer lines = %#v", lines)
	}
	for _, name := range []string{"gi-subagents", "gi-mcp-adapter", "gi-git-guard", "gi-approval-gate", "gi-todo-widget", "gi-tools-ui"} {
		if toolsByPackage[name] == 0 {
			t.Fatalf("%s registered no executable tools", name)
		}
	}
}

func TestOfficialPackageCommandsMountViewTreeWidgets(t *testing.T) {
	sources := make([]any, 0, len(OfficialPackageNames()))
	for _, name := range OfficialPackageNames() {
		sources = append(sources, "official:"+name)
	}
	_, runtime, _ := newOfficialPackageTestRuntime(t, sources...)
	viewTreeHost := NewViewTreeHost()
	runtime.BindViewTreeHost(viewTreeHost)

	for _, scenario := range []struct {
		packageName string
		command     string
		slot        string
		want        string
	}{
		{packageName: "gi-plan-mode", command: "plan", slot: "aboveEditor", want: "Plan Mode"},
		{packageName: "gi-plan-mode", command: "plan-review", slot: "aboveEditor", want: "Plan Mode"},
		{packageName: "gi-subagents", command: "subagents", slot: "aboveEditor", want: "Subagents"},
		{packageName: "gi-mcp-adapter", command: "mcp", slot: "aboveEditor", want: "MCP Adapter"},
		{packageName: "gi-git-guard", command: "git-guard", slot: "aboveEditor", want: "Git Guard"},
		{packageName: "gi-approval-gate", command: "approvals", slot: "aboveEditor", want: "Approval Gate"},
		{packageName: "gi-todo-widget", command: "todo", slot: "aboveEditor", want: "Todo Widget"},
		{packageName: "gi-tools-ui", command: "tools", slot: "aboveEditor", want: "Tools"},
		{packageName: "gi-powerline-footer", command: "footer", slot: "footer", want: "Powerline Footer"},
	} {
		invokeOfficialCommand(t, runtime, scenario.packageName, scenario.command, "")
		mountID := "official." + scenario.packageName + "." + scenario.command
		if !viewTreeMountsContain(viewTreeHost.MountsBySlot(scenario.slot), mountID) {
			t.Fatalf("%s mount missing in %s: %#v", mountID, scenario.slot, viewTreeHost.MountsBySlot(scenario.slot))
		}
		rendered, err := viewTreeHost.RenderMount(mountID, 80)
		if err != nil {
			t.Fatal(err)
		}
		if joined := strings.Join(rendered, "\n"); !strings.Contains(joined, scenario.want) {
			t.Fatalf("%s render = %#v, want %q", mountID, rendered, scenario.want)
		}
	}
}

func viewTreeMountsContain(mounts []ViewTreeMount, mountID string) bool {
	for _, mount := range mounts {
		if mount.MountID == mountID {
			return true
		}
	}
	return false
}

func TestOfficialPackagePowerlineFooterRendersHostState(t *testing.T) {
	_, runtime, session := newOfficialPackageTestRuntime(t, "official:gi-powerline-footer")
	if _, err := exec.LookPath("git"); err == nil {
		runOfficialPackageGit(t, session.SessionManager.GetCWD(), "init")
		runOfficialPackageGit(t, session.SessionManager.GetCWD(), "checkout", "-b", "footer-test")
	}
	session.Agent.State.Model.ContextWindow = 1000
	session.Agent.State.ThinkingLevel = "medium"
	session.SessionManager.AppendMessage(statsUserMessage("hello", 1))
	session.SessionManager.AppendMessage(statsAssistantMessage("answer", 125, 2, session.Agent.State.Model))
	viewTreeHost := NewViewTreeHost()
	runtime.BindViewTreeHost(viewTreeHost)

	invokeOfficialCommand(t, runtime, "gi-powerline-footer", "footer", "")

	lines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.powerline.footer"), "\n")
	for _, expected := range []string{
		"Powerline Footer",
		"Model: test/test-model",
		"Thinking: medium",
		"Context: 125/1000",
		"Active tools:",
	} {
		if !strings.Contains(lines, expected) {
			t.Fatalf("footer renderer missing %q:\n%s", expected, lines)
		}
	}
	if _, err := exec.LookPath("git"); err == nil && !strings.Contains(lines, "Branch: footer-test") {
		t.Fatalf("footer renderer missing git branch:\n%s", lines)
	}
	rendered, err := viewTreeHost.RenderMount("official.gi-powerline-footer.footer", 120)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(rendered, "\n")
	for _, expected := range []string{"Powerline Footer", "Model: test/test-model", "Context: 125/1000", "Tools:"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("footer viewtree missing %q:\n%s", expected, joined)
		}
	}
}

func TestOfficialPackageTodoWidgetRehydratesAndRegistersToolRenderers(t *testing.T) {
	_, runtime, session := newOfficialPackageTestRuntime(t, "official:gi-todo-widget")
	executeOfficialTool(t, runtime, "gi-todo-widget", "todo_write", map[string]any{"todos": []any{"First session todo"}})
	invokeOfficialCommand(t, runtime, "gi-todo-widget", "todo", "")
	firstLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.todo.widget"), "\n")
	if !strings.Contains(firstLines, "First session todo") {
		t.Fatalf("todo renderer missing first session state:\n%s", firstLines)
	}

	secondManager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	secondSession, err := CreateAgentSession(AgentSessionOptions{
		CWD:            secondManager.GetCWD(),
		AgentDir:       t.TempDir(),
		Model:          sdkTestModel(),
		SessionManager: secondManager,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondManager.AppendCustomEntry("todo_state", map[string]any{"todos": []any{"Second session todo"}})
	runtime.BindSession(secondSession)

	invokeOfficialCommand(t, runtime, "gi-todo-widget", "todo", "")
	secondLines := strings.Join(lastCustomMessageLines(t, runtime, secondSession, "gi.todo.widget"), "\n")
	if !strings.Contains(secondLines, "Second session todo") || strings.Contains(secondLines, "First session todo") {
		t.Fatalf("todo renderer did not rehydrate from rebound session:\n%s", secondLines)
	}

	renderer := runtime.GetToolRenderer("todo_write")
	if renderer == nil || renderer.RenderCall == nil || renderer.RenderResult == nil {
		t.Fatalf("todo_write renderer = %#v", renderer)
	}
	callLines := strings.Join(renderer.RenderCall(map[string]any{"todos": []any{"Draft migration"}}, ToolRenderContext{}), "\n")
	if !strings.Contains(callLines, "Todo: write 1 item") || !strings.Contains(callLines, "Draft migration") {
		t.Fatalf("todo_write call render = %s", callLines)
	}
	resultLines := strings.Join(renderer.RenderResult(FileToolResult{Content: []llm.ContentPart{llm.Text(`{"todos":["Draft migration"]}`)}}, ToolRenderResultOptions{}, ToolRenderContext{}), "\n")
	if !strings.Contains(resultLines, "Todo state: 1 item") || !strings.Contains(resultLines, "Draft migration") {
		t.Fatalf("todo_write result render = %s", resultLines)
	}
}

func TestOfficialPackageCommandRenderersSurfaceRuntimeState(t *testing.T) {
	_, runtime, session := newOfficialPackageTestRuntime(t,
		"official:gi-approval-gate",
		"official:gi-plan-mode",
		"official:gi-subagents",
		"official:gi-todo-widget",
		"official:gi-tools-ui",
		"official:gi-git-guard",
	)
	if runtime == nil {
		t.Fatal("runtime is nil")
	}

	approvalResult := executeOfficialTool(t, runtime, "gi-approval-gate", "approval_gate_request", map[string]any{"action": "delete generated file"})
	if approvalText := sdkToolText(approvalResult); !strings.Contains(approvalText, `"pending"`) || !strings.Contains(approvalText, "delete generated file") {
		t.Fatalf("approval result = %s", approvalText)
	}
	invokeOfficialCommand(t, runtime, "gi-approval-gate", "approvals", "")
	approvalLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.approval.request"), "\n")
	for _, expected := range []string{"Approval Gate", "Status: pending", "delete generated file"} {
		if !strings.Contains(approvalLines, expected) {
			t.Fatalf("approval renderer missing %q:\n%s", expected, approvalLines)
		}
	}

	planResult := executeOfficialTool(t, runtime, "gi-plan-mode", "plan_update", map[string]any{"text": "Plan:\n1. Review protocol\n2. Run focused tests"})
	if planText := sdkToolText(planResult); !strings.Contains(planText, "Review protocol") || !strings.Contains(planText, "Run focused tests") {
		t.Fatalf("plan result = %s", planText)
	}
	invokeOfficialCommand(t, runtime, "gi-plan-mode", "plan", "")
	planLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.plan.status"), "\n")
	for _, expected := range []string{"Plan Mode", "Status: read-only", "Active tools: read, grep, find, ls", "Review protocol", "Focused tests"} {
		if !strings.Contains(planLines, expected) {
			t.Fatalf("plan renderer missing %q:\n%s", expected, planLines)
		}
	}

	executeOfficialTool(t, runtime, "gi-todo-widget", "todo_write", map[string]any{
		"todos": []any{
			map[string]any{"content": "Draft migration plan", "status": "pending"},
			"Run focused tests",
		},
	})
	invokeOfficialCommand(t, runtime, "gi-todo-widget", "todo", "")
	todoLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.todo.widget"), "\n")
	for _, expected := range []string{"Todo Widget", "Draft migration plan (pending)", "Run focused tests"} {
		if !strings.Contains(todoLines, expected) {
			t.Fatalf("todo renderer missing %q:\n%s", expected, todoLines)
		}
	}

	invokeOfficialCommand(t, runtime, "gi-tools-ui", "tools", "")
	toolsLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.tools.list"), "\n")
	if !strings.Contains(toolsLines, "Tools") || !strings.Contains(toolsLines, "Active tools:") || !strings.Contains(toolsLines, "read") {
		t.Fatalf("tools renderer lines = %s", toolsLines)
	}

	subagentResult := executeOfficialTool(t, runtime, "gi-subagents", "subagent_spawn", map[string]any{"task": "inspect protocol"})
	subagentText := sdkToolText(subagentResult)
	if !strings.Contains(subagentText, `"sessionId"`) || !strings.Contains(subagentText, "child response: inspect protocol") {
		t.Fatalf("subagent result = %s", subagentText)
	}
	invokeOfficialCommand(t, runtime, "gi-subagents", "subagents", "")
	subagentLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.subagent.status"), "\n")
	for _, expected := range []string{"Subagents", "Status: done", "Last subagent run completed"} {
		if !strings.Contains(subagentLines, expected) {
			t.Fatalf("subagent renderer missing %q:\n%s", expected, subagentLines)
		}
	}

	executeOfficialTool(t, runtime, "gi-git-guard", "git_guard_check", map[string]any{"action": "push"})
	invokeOfficialCommand(t, runtime, "gi-git-guard", "git-guard", "")
	guardLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.git.guard"), "\n")
	for _, expected := range []string{"Git Guard", "Status: check recorded", "Review branch"} {
		if !strings.Contains(guardLines, expected) {
			t.Fatalf("git guard renderer missing %q:\n%s", expected, guardLines)
		}
	}
}

func TestOfficialPackagePlanCommandEntersReadOnlyToolMode(t *testing.T) {
	_, runtime, session := newOfficialPackageTestRuntime(t, "official:gi-plan-mode")
	if runtime == nil || session == nil {
		t.Fatal("runtime/session is nil")
	}

	invokeOfficialCommand(t, runtime, "gi-plan-mode", "plan", "implement protocol host")
	if got := session.GetActiveToolNames(); !reflectStringSetEqual(got, []string{"read", "grep", "find", "ls"}) {
		t.Fatalf("active tools = %#v", got)
	}
	planLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.plan.status"), "\n")
	for _, expected := range []string{
		"Command: /plan implement protocol host",
		"Plan mode active with read-only tools.",
		"Status: read-only",
		"Active tools: read, grep, find, ls",
	} {
		if !strings.Contains(planLines, expected) {
			t.Fatalf("plan renderer missing %q:\n%s", expected, planLines)
		}
	}
}

func TestOfficialPackagePlanReviewAndToolRenderers(t *testing.T) {
	_, runtime, session := newOfficialPackageTestRuntime(t, "official:gi-plan-mode")
	planInput := map[string]any{"text": "Plan:\n1. Review protocol\n2. Run focused tests"}
	executeOfficialTool(t, runtime, "gi-plan-mode", "plan_update", planInput)

	updateRenderer := runtime.GetToolRenderer("plan_update")
	if updateRenderer == nil || updateRenderer.RenderCall == nil || updateRenderer.RenderResult == nil {
		t.Fatalf("plan_update renderer = %#v", updateRenderer)
	}
	callLines := strings.Join(updateRenderer.RenderCall(planInput, ToolRenderContext{}), "\n")
	for _, expected := range []string{"Plan update: 2 item", "1. Review protocol", "2. Focused tests"} {
		if !strings.Contains(callLines, expected) {
			t.Fatalf("plan update call renderer missing %q:\n%s", expected, callLines)
		}
	}
	readRenderer := runtime.GetToolRenderer("plan_read")
	if readRenderer == nil || readRenderer.RenderCall == nil || readRenderer.RenderResult == nil {
		t.Fatalf("plan_read renderer = %#v", readRenderer)
	}
	readResult := executeOfficialTool(t, runtime, "gi-plan-mode", "plan_read", nil)
	resultLines := strings.Join(readRenderer.RenderResult(FileToolResult{Content: []llm.ContentPart{llm.Text(sdkToolText(readResult))}}, ToolRenderResultOptions{}, ToolRenderContext{}), "\n")
	if !strings.Contains(resultLines, "Plan state: 2 item") || !strings.Contains(resultLines, "Review protocol") {
		t.Fatalf("plan read result renderer = %s", resultLines)
	}

	invokeOfficialCommand(t, runtime, "gi-plan-mode", "plan-review", "review protocol host")
	if got := session.GetActiveToolNames(); !reflectStringSetEqual(got, []string{"read", "grep", "find", "ls"}) {
		t.Fatalf("active tools = %#v", got)
	}
	planLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.plan.status"), "\n")
	for _, expected := range []string{
		"Command: /plan-review review protocol host",
		"Status: reviewing",
		"Active tools: read, grep, find, ls",
		"Review protocol",
		"Focused tests",
	} {
		if !strings.Contains(planLines, expected) {
			t.Fatalf("plan review renderer missing %q:\n%s", expected, planLines)
		}
	}
}

func TestOfficialPackageSubagentToolRenderersAndParentContext(t *testing.T) {
	_, runtime, session := newOfficialPackageTestRuntimeWithResponder(t, []any{"official:gi-subagents"}, func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("child response: " + prompt)}}, nil
	})
	session.SessionManager.AppendMessage(statsUserMessage("Parent question", 1))
	session.SessionManager.AppendMessage(statsAssistantMessage("Parent answer", 25, 2, session.Agent.State.Model))
	input := map[string]any{"task": "inspect protocol", "includeParentContext": true, "parentContextEntries": 2}

	spawnRenderer := runtime.GetToolRenderer("subagent_spawn")
	if spawnRenderer == nil || spawnRenderer.RenderCall == nil || spawnRenderer.RenderResult == nil {
		t.Fatalf("subagent_spawn renderer = %#v", spawnRenderer)
	}
	callLines := strings.Join(spawnRenderer.RenderCall(input, ToolRenderContext{}), "\n")
	for _, expected := range []string{"Subagent spawn", "Task: inspect protocol", "Parent context: last 2 entries"} {
		if !strings.Contains(callLines, expected) {
			t.Fatalf("subagent spawn call renderer missing %q:\n%s", expected, callLines)
		}
	}

	result := executeOfficialTool(t, runtime, "gi-subagents", "subagent_spawn", input)
	resultText := sdkToolText(result)
	for _, expected := range []string{"parentContextCount", "Parent question", "Parent answer", "child response: inspect protocol"} {
		if !strings.Contains(resultText, expected) {
			t.Fatalf("subagent result missing %q:\n%s", expected, resultText)
		}
	}
	resultLines := strings.Join(spawnRenderer.RenderResult(FileToolResult{Content: []llm.ContentPart{llm.Text(resultText)}}, ToolRenderResultOptions{}, ToolRenderContext{}), "\n")
	for _, expected := range []string{"Subagent completed", "Parent context: 2 entries", "user: Parent question", "assistant: Parent answer"} {
		if !strings.Contains(resultLines, expected) {
			t.Fatalf("subagent result renderer missing %q:\n%s", expected, resultLines)
		}
	}

	invokeOfficialCommand(t, runtime, "gi-subagents", "subagents", "")
	subagentLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.subagent.status"), "\n")
	for _, expected := range []string{"Subagents", "Status: done", "user: Parent question", "assistant: Parent answer"} {
		if !strings.Contains(subagentLines, expected) {
			t.Fatalf("subagent renderer missing %q:\n%s", expected, subagentLines)
		}
	}

	abortRenderer := runtime.GetToolRenderer("subagent_abort")
	if abortRenderer == nil || abortRenderer.RenderCall == nil || abortRenderer.RenderResult == nil {
		t.Fatalf("subagent_abort renderer = %#v", abortRenderer)
	}
	abortLines := strings.Join(abortRenderer.RenderCall(map[string]any{"target": "children"}, ToolRenderContext{}), "\n")
	if !strings.Contains(abortLines, "Subagent abort") || !strings.Contains(abortLines, "Target: children") {
		t.Fatalf("subagent abort renderer = %s", abortLines)
	}
}

func TestOfficialPackageSubagentAbortCancelsRunningChild(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	_, runtime, session := newOfficialPackageTestRuntimeWithResponder(t, []any{"official:gi-subagents"}, func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		startedOnce.Do(func() { close(started) })
		<-release
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("child response: " + prompt)}}, nil
	})
	spawnTool := findOfficialTool(t, runtime, "gi-subagents", "subagent_spawn")
	type toolExecution struct {
		result SDKToolResult
		err    error
	}
	spawnCh := make(chan toolExecution, 1)
	go func() {
		result, err := spawnTool.Execute("test-subagent_spawn", map[string]any{"task": "long child task"})
		spawnCh <- toolExecution{result: result, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("subagent did not start")
	}

	abortResult := executeOfficialTool(t, runtime, "gi-subagents", "subagent_abort", map[string]any{"target": "children"})
	if abortText := sdkToolText(abortResult); !strings.Contains(abortText, `"aborted": true`) {
		t.Fatalf("subagent abort result = %s", abortText)
	}
	close(release)
	var spawn toolExecution
	select {
	case spawn = <-spawnCh:
	case <-time.After(time.Second):
		t.Fatal("subagent spawn did not finish after abort")
	}
	if spawn.err != nil {
		t.Fatalf("subagent spawn: %v", spawn.err)
	}
	if spawnText := sdkToolText(spawn.result); !strings.Contains(spawnText, `"aborted": true`) {
		t.Fatalf("subagent spawn result = %s", spawnText)
	}

	invokeOfficialCommand(t, runtime, "gi-subagents", "subagents", "")
	subagentLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.subagent.status"), "\n")
	for _, expected := range []string{"Subagents", "Status: aborted", "Last subagent run was aborted"} {
		if !strings.Contains(subagentLines, expected) {
			t.Fatalf("subagent renderer missing %q:\n%s", expected, subagentLines)
		}
	}
}

func TestOfficialPackageGitGuardChecksGitStatus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	repoDir := t.TempDir()
	runOfficialPackageGit(t, repoDir, "init")
	if err := os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, runtime, session := newOfficialPackageTestRuntime(t, "official:gi-git-guard")

	result := executeOfficialTool(t, runtime, "gi-git-guard", "git_guard_check", map[string]any{
		"action": "push",
		"cwd":    repoDir,
	})
	resultText := sdkToolText(result)
	if !strings.Contains(resultText, `"checked"`) || !strings.Contains(resultText, "file.txt") {
		t.Fatalf("git guard result = %s", resultText)
	}

	invokeOfficialCommand(t, runtime, "gi-git-guard", "git-guard", "")
	guardLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.git.guard"), "\n")
	for _, expected := range []string{"Git Guard", "Status: checked", "?? file.txt"} {
		if !strings.Contains(guardLines, expected) {
			t.Fatalf("git guard renderer missing %q:\n%s", expected, guardLines)
		}
	}
	if !strings.Contains(guardLines, "Review branch") {
		t.Fatalf("git guard renderer missing branch summary:\n%s", guardLines)
	}
}

func TestOfficialPackageGitGuardUsesHostProcessExecWhenAvailable(t *testing.T) {
	_, runtime, session := newOfficialPackageTestRuntime(t, "official:gi-git-guard")
	executor := &recordingOfficialHostProcessExecutor{
		result: HostProcessResult{Stdout: "## main\n M guarded.txt\n"},
	}
	host := NewRPCSessionHost(session)
	host.ProcessExecutor = executor
	runtime.BindHostActionHost(host)

	result := executeOfficialTool(t, runtime, "gi-git-guard", "git_guard_check", map[string]any{
		"action": "push",
	})
	resultText := sdkToolText(result)
	if !strings.Contains(resultText, `"checked"`) || !strings.Contains(resultText, "guarded.txt") {
		t.Fatalf("git guard result = %s", resultText)
	}
	if !reflect.DeepEqual(executor.command, []string{"git", "--no-optional-locks", "status", "--short", "--branch"}) {
		t.Fatalf("command = %#v", executor.command)
	}
	if executor.cwd != session.SessionManager.GetCWD() {
		t.Fatalf("cwd = %q, want %q", executor.cwd, session.SessionManager.GetCWD())
	}
	if executor.options.Timeout != 2*time.Second {
		t.Fatalf("timeout = %s, want 2s", executor.options.Timeout)
	}
}

func TestOfficialPackageGitGuardRecordsDialogDecision(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	repoDir := t.TempDir()
	runOfficialPackageGit(t, repoDir, "init")
	if err := os.WriteFile(filepath.Join(repoDir, "danger.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, runtime, session := newOfficialPackageTestRuntime(t, "official:gi-git-guard")
	dialog := &recordingTUIDialogHost{result: TUIDialogResult{Action: "confirmed", OptionID: "yes", Value: true}}
	host := NewRPCSessionHost(session)
	host.TUIDialog = dialog
	runtime.BindHostActionHost(host)

	result := executeOfficialTool(t, runtime, "gi-git-guard", "git_guard_check", map[string]any{
		"action":              "push",
		"cwd":                 repoDir,
		"requireConfirmation": true,
	})
	resultText := sdkToolText(result)
	if !strings.Contains(resultText, `"confirmed"`) || !strings.Contains(resultText, "danger.txt") {
		t.Fatalf("git guard confirm result = %s", resultText)
	}
	if len(dialog.requests) != 1 {
		t.Fatalf("dialog requests = %#v", dialog.requests)
	}
	if request := dialog.requests[0]; request.Kind != "confirm" || !strings.Contains(request.Message, "Action: push") || !strings.Contains(request.Message, "Dirty files: 1") {
		t.Fatalf("dialog request = %#v", request)
	}

	invokeOfficialCommand(t, runtime, "gi-git-guard", "git-guard", "")
	guardLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.git.guard"), "\n")
	for _, expected := range []string{"Git Guard", "Status: confirmed", "Review branch", "danger.txt"} {
		if !strings.Contains(guardLines, expected) {
			t.Fatalf("git guard renderer missing %q:\n%s", expected, guardLines)
		}
	}
}

func TestOfficialPackageApprovalGateRecordsDecision(t *testing.T) {
	_, runtime, session := newOfficialPackageTestRuntime(t, "official:gi-approval-gate")
	if runtime == nil || session == nil {
		t.Fatal("runtime/session is nil")
	}

	requestInput := map[string]any{
		"action":  "delete generated file",
		"command": "rm generated.txt",
		"diff":    "- old\n+ new",
		"risk":    "destructive",
	}
	executeOfficialTool(t, runtime, "gi-approval-gate", "approval_gate_request", requestInput)
	decision := executeOfficialTool(t, runtime, "gi-approval-gate", "approval_gate_decide", map[string]any{
		"decision":    "rewrite",
		"reason":      "workspace is dirty",
		"replacement": "rm -i generated.txt",
	})
	decisionText := sdkToolText(decision)
	if !strings.Contains(decisionText, `"rewrite"`) || !strings.Contains(decisionText, "rm -i generated.txt") {
		t.Fatalf("approval decision result = %s", decisionText)
	}

	invokeOfficialCommand(t, runtime, "gi-approval-gate", "approvals", "")
	approvalLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.approval.request"), "\n")
	for _, expected := range []string{
		"Approval Gate",
		"Approval decision: rewrite",
		"Status: rewrite",
		"Decision: rewrite",
		"Risk: destructive",
		"Reason: workspace is dirty",
		"Replacement: rm -i generated.txt",
		"Command: rm generated.txt",
		"Diff:",
	} {
		if !strings.Contains(approvalLines, expected) {
			t.Fatalf("approval renderer missing %q:\n%s", expected, approvalLines)
		}
	}

	decisionRenderer := runtime.GetMessageRenderer("gi.approval.decision")
	if decisionRenderer == nil {
		t.Fatal("approval decision renderer missing")
	}
	decisionLines := strings.Join(decisionRenderer(llm.Message{
		Role:       "custom",
		CustomType: "gi.approval.decision",
		Content:    []llm.ContentPart{llm.Text(`{"summary":"Approval decision recorded","decision":"rewrite","status":"rewrite","reason":"needs confirmation","replacement":"safe command"}`)},
	}, nil), "\n")
	for _, expected := range []string{"Approval Decision", "Approval decision recorded", "Status: rewrite", "Decision: rewrite", "Reason: needs confirmation", "Replacement: safe command"} {
		if !strings.Contains(decisionLines, expected) {
			t.Fatalf("approval decision renderer missing %q:\n%s", expected, decisionLines)
		}
	}

	requestToolRenderer := runtime.GetToolRenderer("approval_gate_request")
	if requestToolRenderer == nil || requestToolRenderer.RenderCall == nil || requestToolRenderer.RenderResult == nil {
		t.Fatalf("approval request tool renderer = %#v", requestToolRenderer)
	}
	requestCallLines := strings.Join(requestToolRenderer.RenderCall(requestInput, ToolRenderContext{}), "\n")
	for _, expected := range []string{"Approval request", "Action: delete generated file", "Command: rm generated.txt", "Risk: destructive", "Diff:", "+ new"} {
		if !strings.Contains(requestCallLines, expected) {
			t.Fatalf("approval request call renderer missing %q:\n%s", expected, requestCallLines)
		}
	}
	decisionToolRenderer := runtime.GetToolRenderer("approval_gate_decide")
	if decisionToolRenderer == nil || decisionToolRenderer.RenderCall == nil || decisionToolRenderer.RenderResult == nil {
		t.Fatalf("approval decision tool renderer = %#v", decisionToolRenderer)
	}
	decisionCallLines := strings.Join(decisionToolRenderer.RenderCall(map[string]any{"decision": "deny", "reason": "too risky"}, ToolRenderContext{}), "\n")
	if !strings.Contains(decisionCallLines, "Approval decision: denied") || !strings.Contains(decisionCallLines, "Reason: too risky") {
		t.Fatalf("approval decision call renderer = %s", decisionCallLines)
	}
}

func TestOfficialPackageToolsUIPatchesActiveTools(t *testing.T) {
	_, runtime, session := newOfficialPackageTestRuntime(t, "official:gi-tools-ui")
	if runtime == nil || session == nil {
		t.Fatal("runtime/session is nil")
	}

	result := executeOfficialTool(t, runtime, "gi-tools-ui", "tools_ui_set_active", map[string]any{
		"mode":      "replace",
		"toolNames": []any{"read", "grep"},
	})
	resultText := sdkToolText(result)
	if !strings.Contains(resultText, `"updated"`) || !strings.Contains(resultText, `"read"`) || !strings.Contains(resultText, `"grep"`) {
		t.Fatalf("tools_ui_set_active result = %s", resultText)
	}
	if got := session.GetActiveToolNames(); !reflectStringSetEqual(got, []string{"read", "grep"}) {
		t.Fatalf("active tools = %#v", got)
	}

	invokeOfficialCommand(t, runtime, "gi-tools-ui", "tools", "")
	toolsLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.tools.list"), "\n")
	for _, expected := range []string{"Tools", "Status: updated", "Active tools: read, grep"} {
		if !strings.Contains(toolsLines, expected) {
			t.Fatalf("tools renderer missing %q:\n%s", expected, toolsLines)
		}
	}
}

func TestOfficialPackageMCPStdioAdapter(t *testing.T) {
	_, runtime, session := newOfficialPackageTestRuntime(t, "official:gi-mcp-adapter")
	helper, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := []any{helper, "-test.run=TestOfficialPackageMCPHelper"}
	env := map[string]any{"GI_MCP_HELPER": "1"}

	listResult := executeOfficialTool(t, runtime, "gi-mcp-adapter", "mcp_list_tools", map[string]any{
		"command": command,
		"env":     env,
	})
	if listText := sdkToolText(listResult); !strings.Contains(listText, "echo") ||
		!strings.Contains(listText, "tools discovered") ||
		!strings.Contains(listText, "indexing") ||
		!strings.Contains(listText, "toolsListChanged") {
		t.Fatalf("mcp list result = %s", listText)
	}
	invokeOfficialCommand(t, runtime, "gi-mcp-adapter", "mcp", "")
	mcpLines := strings.Join(lastCustomMessageLines(t, runtime, session, "gi.mcp.diagnostics"), "\n")
	for _, expected := range []string{"MCP Adapter", "MCP tools: echo", "Status: tools discovered", "MCP progress: indexing", "MCP tools changed"} {
		if !strings.Contains(mcpLines, expected) {
			t.Fatalf("mcp renderer missing %q:\n%s", expected, mcpLines)
		}
	}
	dynamicTool := findDynamicSDKTool(runtime.RegisteredTools(), "mcp_echo")
	if dynamicTool == nil {
		t.Fatalf("dynamic MCP tool was not registered; tools=%#v", runtime.RegisteredTools())
	}
	dynamicResult, err := dynamicTool.Execute("test-mcp-echo", map[string]any{"text": "dynamic"})
	if err != nil {
		t.Fatalf("dynamic MCP tool: %v", err)
	}
	if dynamicText := sdkToolText(dynamicResult); !strings.Contains(dynamicText, "echo: dynamic") || !strings.Contains(dynamicText, "calling") {
		t.Fatalf("dynamic MCP tool result = %s", dynamicText)
	}

	callResult := executeOfficialTool(t, runtime, "gi-mcp-adapter", "mcp_call", map[string]any{
		"command":   command,
		"env":       env,
		"tool":      "echo",
		"arguments": map[string]any{"text": "hello"},
	})
	if callText := sdkToolText(callResult); !strings.Contains(callText, "echo: hello") ||
		!strings.Contains(callText, `"done"`) ||
		!strings.Contains(callText, "calling") {
		t.Fatalf("mcp call result = %s", callText)
	}
	invokeOfficialCommand(t, runtime, "gi-mcp-adapter", "mcp", "")
	mcpLines = strings.Join(lastCustomMessageLines(t, runtime, session, "gi.mcp.diagnostics"), "\n")
	for _, expected := range []string{"Last MCP call: echo", "Status: done"} {
		if !strings.Contains(mcpLines, expected) {
			t.Fatalf("mcp renderer after call missing %q:\n%s", expected, mcpLines)
		}
	}
}

func TestOfficialPackageMCPHelper(t *testing.T) {
	if os.Getenv("GI_MCP_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var envelope mcpRPCEnvelope
		if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
			continue
		}
		switch envelope.Method {
		case "initialize":
			writeMCPHelperResponse(t, encoder, envelope.ID, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "test-mcp", "version": "0.1.0"},
			})
		case "notifications/initialized":
		case "tools/list":
			writeMCPHelperNotification(t, encoder, "notifications/progress", map[string]any{
				"progressToken": "tools",
				"progress":      1,
				"total":         2,
				"message":       "indexing",
			})
			writeMCPHelperNotification(t, encoder, "notifications/tools/list_changed", map[string]any{})
			writeMCPHelperResponse(t, encoder, envelope.ID, map[string]any{
				"tools": []any{map[string]any{
					"name":        "echo",
					"description": "Echo input",
					"inputSchema": map[string]any{"type": "object"},
				}},
			})
		case "tools/call":
			params, _ := envelope.Params.(map[string]any)
			arguments, _ := params["arguments"].(map[string]any)
			writeMCPHelperNotification(t, encoder, "notifications/progress", map[string]any{
				"progressToken": "call",
				"progress":      1,
				"total":         1,
				"message":       "calling",
			})
			writeMCPHelperResponse(t, encoder, envelope.ID, map[string]any{
				"content": []any{map[string]any{"type": "text", "text": "echo: " + stringFromAny(arguments["text"])}},
				"isError": false,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func writeMCPHelperResponse(t *testing.T, encoder *json.Encoder, id any, result map[string]any) {
	t.Helper()
	if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result}); err != nil {
		t.Fatal(err)
	}
}

func writeMCPHelperNotification(t *testing.T, encoder *json.Encoder, method string, params map[string]any) {
	t.Helper()
	if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": method, "params": params}); err != nil {
		t.Fatal(err)
	}
}

func runOfficialPackageGit(t *testing.T, cwd string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}

func officialToolTestInput(name string) map[string]any {
	switch name {
	case "plan_update":
		return map[string]any{"text": "Plan:\n1. Review implementation\n2. Run tests"}
	case "todo_write":
		return map[string]any{"todos": []any{"test todo"}}
	case "subagent_spawn":
		return map[string]any{"task": "test child task"}
	case "subagent_abort":
		return map[string]any{"target": "children"}
	case "approval_gate_request":
		return map[string]any{"action": "test approval"}
	case "approval_gate_decide":
		return map[string]any{"decision": "approve", "reason": "test approval"}
	case "mcp_call":
		return map[string]any{"tool": "test"}
	case "tools_ui_set_active":
		return map[string]any{"toolNames": []any{"read", "grep"}}
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

func newOfficialPackageTestRuntime(t *testing.T, sources ...any) (*DefaultResourceLoader, *ProtocolExtensionRuntime, *AgentSession) {
	t.Helper()
	return newOfficialPackageTestRuntimeWithResponder(t, sources, func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("child response: " + prompt)}}, nil
	})
}

func newOfficialPackageTestRuntimeWithResponder(t *testing.T, sources []any, responder func(string, []llm.Message, llm.Model) (llm.Message, error)) (*DefaultResourceLoader, *ProtocolExtensionRuntime, *AgentSession) {
	t.Helper()
	agentDir, cwd := createResourceLoaderDirs(t)
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
		Responder:      responder,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime.BindSession(session)
	return loader, runtime, session
}

type recordingTUIDialogHost struct {
	requests []TUIDialogRequest
	result   TUIDialogResult
	err      error
}

func (h *recordingTUIDialogHost) RunTUIDialog(request TUIDialogRequest) (TUIDialogResult, error) {
	h.requests = append(h.requests, request)
	if h.err != nil {
		return TUIDialogResult{}, h.err
	}
	return h.result, nil
}

type recordingOfficialHostProcessExecutor struct {
	command []string
	cwd     string
	options HostProcessOptions
	result  HostProcessResult
	err     error
}

func (e *recordingOfficialHostProcessExecutor) ExecuteHostProcess(command []string, cwd string) (HostProcessResult, error) {
	return e.ExecuteHostProcessWithOptions(command, cwd, HostProcessOptions{})
}

func (e *recordingOfficialHostProcessExecutor) ExecuteHostProcessWithOptions(command []string, cwd string, options HostProcessOptions) (HostProcessResult, error) {
	e.command = append([]string(nil), command...)
	e.cwd = cwd
	e.options = options
	return e.result, e.err
}

func findOfficialTool(t *testing.T, runtime *ProtocolExtensionRuntime, packageName, toolName string) SDKTool {
	t.Helper()
	for _, tool := range runtime.RegisteredTools() {
		if tool.Name == toolName && tool.SourceInfo.Source == "official:"+packageName {
			if tool.Execute == nil {
				t.Fatalf("official tool %s has no execute", toolName)
			}
			return tool
		}
	}
	t.Fatalf("official tool %s from %s not found", toolName, packageName)
	return SDKTool{}
}

func executeOfficialTool(t *testing.T, runtime *ProtocolExtensionRuntime, packageName, toolName string, input map[string]any) SDKToolResult {
	t.Helper()
	tool := findOfficialTool(t, runtime, packageName, toolName)
	result, err := tool.Execute("test-"+toolName, input)
	if err != nil {
		t.Fatalf("tool %s: %v", toolName, err)
	}
	return result
}

func invokeOfficialCommand(t *testing.T, runtime *ProtocolExtensionRuntime, packageName, commandName, args string) {
	t.Helper()
	for _, command := range runtime.RegisteredCommands() {
		if command.Name == commandName && command.SourceInfo.Source == "official:"+packageName {
			if command.Handler == nil {
				t.Fatalf("official command %s has no handler", commandName)
			}
			if err := command.Handler(args); err != nil {
				t.Fatalf("command %s: %v", commandName, err)
			}
			return
		}
	}
	t.Fatalf("official command %s from %s not found", commandName, packageName)
}

func lastCustomMessageLines(t *testing.T, runtime *ProtocolExtensionRuntime, session *AgentSession, customType string) []string {
	t.Helper()
	renderer := runtime.GetMessageRenderer(customType)
	if renderer == nil {
		t.Fatalf("message renderer %s missing", customType)
	}
	entries := session.SessionManager.GetEntries()
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		if entry.Type != "custom_message" || entry.CustomType != customType {
			continue
		}
		message := llm.Message{
			Role:       "custom",
			CustomType: customType,
			Content:    []llm.ContentPart{llm.Text(customMessageText(entry.Content))},
		}
		return renderer(message, nil)
	}
	t.Fatalf("custom message %s not found", customType)
	return nil
}
