package gicodingagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestCreateAgentSessionUsesAgentDirForDefaultPersistedSessionPath(t *testing.T) {
	tempDir := t.TempDir()
	cwd := filepath.Join(tempDir, "project")
	agentDir := filepath.Join(tempDir, "agent")
	mkdirAllForSDKTest(t, cwd, agentDir)

	session, err := CreateAgentSession(AgentSessionOptions{CWD: cwd, AgentDir: agentDir, Model: sdkTestModel()})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()

	expectedSessionDir := GetAgentDirSessionDir(cwd, agentDir)
	if got := session.SessionManager.GetSessionDir(); got != expectedSessionDir {
		t.Fatalf("session dir = %q, want %q", got, expectedSessionDir)
	}
	if sessionFile := session.SessionManager.GetSessionFile(); !hasPathPrefix(sessionFile, expectedSessionDir) {
		t.Fatalf("session file = %q, want under %q", sessionFile, expectedSessionDir)
	}
}

func TestCreateAgentSessionClampsDefaultThinkingLikePi(t *testing.T) {
	reasoningModel := sdkTestModel()
	reasoningModel.Reasoning = true
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:      t.TempDir(),
		Model:    reasoningModel,
		AgentDir: filepath.Join(t.TempDir(), "agent"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.Agent.State.ThinkingLevel != string(DefaultThinkingLevel) {
		t.Fatalf("reasoning model thinking = %q, want %q", session.Agent.State.ThinkingLevel, DefaultThinkingLevel)
	}

	plainModel := sdkTestModel()
	session, err = CreateAgentSession(AgentSessionOptions{
		CWD:           t.TempDir(),
		Model:         plainModel,
		ThinkingLevel: string(ThinkingHigh),
		AgentDir:      filepath.Join(t.TempDir(), "agent"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.Agent.State.ThinkingLevel != string(ThinkingOff) {
		t.Fatalf("plain model thinking = %q, want off", session.Agent.State.ThinkingLevel)
	}
}

func TestCreateAgentSessionKeepsExplicitSessionManagerOverride(t *testing.T) {
	tempDir := t.TempDir()
	cwd := filepath.Join(tempDir, "project")
	agentDir := filepath.Join(tempDir, "agent")
	mkdirAllForSDKTest(t, cwd, agentDir)
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
	defer session.Dispose()

	if session.SessionManager != sessionManager {
		t.Fatal("explicit session manager was not preserved")
	}
	if session.SessionManager.IsPersisted() {
		t.Fatal("explicit in-memory session manager should not be persisted")
	}
}

func TestCreateAgentSessionDerivesCwdFromExplicitSessionManagerWhenOmitted(t *testing.T) {
	tempDir := t.TempDir()
	sessionCwd := filepath.Join(tempDir, "session-project")
	agentDir := filepath.Join(tempDir, "agent")
	mkdirAllForSDKTest(t, sessionCwd, agentDir)
	sessionManager, err := InMemorySessionManager(sessionCwd)
	if err != nil {
		t.Fatal(err)
	}

	session, err := CreateAgentSession(AgentSessionOptions{
		AgentDir:       agentDir,
		Model:          sdkTestModel(),
		SessionManager: sessionManager,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()

	if session.SessionManager != sessionManager {
		t.Fatal("explicit session manager was not preserved")
	}
	if !strings.Contains(session.SystemPrompt, "Current working directory: "+sessionCwd) {
		t.Fatalf("system prompt missing cwd %q: %q", sessionCwd, session.SystemPrompt)
	}
	bashTool, ok := findSDKTool(session.Agent.State.Tools, "bash")
	if !ok {
		t.Fatal("bash tool not found")
	}
	result, err := bashTool.Execute("test", map[string]any{"command": "pwd"})
	if err != nil {
		t.Fatal(err)
	}
	output := sdkToolText(result)
	if filepath.Clean(output) != filepath.Clean(sessionCwd) {
		t.Fatalf("bash pwd output = %q, want %q", output, sessionCwd)
	}
}

func TestCreateAgentSessionSystemPromptConsumesResourceLoaderContext(t *testing.T) {
	tempDir := t.TempDir()
	cwd := filepath.Join(tempDir, "project")
	agentDir := filepath.Join(tempDir, "agent")
	mkdirAllForSDKTest(t, cwd, agentDir)
	writeResourceFile(t, filepath.Join(cwd, "AGENTS.md"), "Project rule")
	loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{
		CWD:                cwd,
		AgentDir:           agentDir,
		SystemPrompt:       "Custom base prompt",
		AppendSystemPrompt: []string{"Extra A", "Extra B"},
	})
	loader.Reload()

	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            cwd,
		AgentDir:       agentDir,
		Model:          sdkTestModel(),
		ResourceLoader: loader,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()

	for _, want := range []string{"Custom base prompt", "Extra A\n\nExtra B", "# Project Context", "Project rule", "Current working directory: " + cwd} {
		if !strings.Contains(session.SystemPrompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, session.SystemPrompt)
		}
	}
	if strings.Contains(session.SystemPrompt, "Available tools:") {
		t.Fatalf("custom prompt should replace default tool prompt:\n%s", session.SystemPrompt)
	}
}

func TestCreateAgentSessionDefaultFileToolsExecuteLikePi(t *testing.T) {
	tempDir := t.TempDir()
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:      tempDir,
		AgentDir: filepath.Join(t.TempDir(), "agent"),
		Model:    sdkTestModel(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()

	writeTool, ok := findSDKTool(session.Agent.State.Tools, "write")
	if !ok || writeTool.Execute == nil {
		t.Fatalf("write tool = %#v", writeTool)
	}
	writeResult, err := writeTool.Execute("write-1", map[string]any{
		"file_path": "notes.txt",
		"content":   "before\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sdkToolText(writeResult), "Successfully wrote") {
		t.Fatalf("write result = %#v", writeResult)
	}

	readTool, ok := findSDKTool(session.Agent.State.Tools, "read")
	if !ok || readTool.Execute == nil {
		t.Fatalf("read tool = %#v", readTool)
	}
	readResult, err := readTool.Execute("read-1", map[string]any{"file_path": "notes.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if sdkToolText(readResult) != "before" {
		t.Fatalf("read result = %#v", readResult)
	}

	editTool, ok := findSDKTool(session.Agent.State.Tools, "edit")
	if !ok || editTool.Execute == nil {
		t.Fatalf("edit tool = %#v", editTool)
	}
	editResult, err := editTool.Execute("edit-1", map[string]any{
		"path":    "notes.txt",
		"oldText": "before",
		"newText": "after",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sdkToolText(editResult), "Successfully replaced") {
		t.Fatalf("edit result = %#v", editResult)
	}
	details, ok := editResult.Details.(*FileToolDetails)
	if !ok || details.Diff == "" || details.FirstChangedLine != 1 {
		t.Fatalf("edit details = %#v", editResult.Details)
	}
	if content, err := os.ReadFile(filepath.Join(tempDir, "notes.txt")); err != nil || string(content) != "after\n" {
		t.Fatalf("content = %q err=%v", content, err)
	}
}

func TestAgentSessionActiveLLMToolsExposeSchemasForProviderCalls(t *testing.T) {
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:      t.TempDir(),
		AgentDir: filepath.Join(t.TempDir(), "agent"),
		Model:    sdkTestModel(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()

	tools := session.GetActiveLLMTools()
	read := findLLMTool(tools, "read")
	if read == nil || read.Parameters.Properties["path"].Type != "string" {
		t.Fatalf("read tool schema = %#v", read)
	}
	edit := findLLMTool(tools, "edit")
	if edit == nil {
		t.Fatalf("edit tool missing from %#v", tools)
	}
	if _, ok := edit.Parameters.Properties["oldText"]; ok {
		t.Fatalf("edit provider schema leaked legacy oldText: %#v", edit.Parameters)
	}
	if edit.Parameters.Properties["edits"].Type != "array" {
		t.Fatalf("edit edits schema = %#v", edit.Parameters.Properties["edits"])
	}
	grep := findLLMTool(tools, "grep")
	if grep == nil || grep.Parameters.Properties["pattern"].Type != "string" {
		t.Fatalf("grep tool schema = %#v", grep)
	}
}

func TestAgentSessionPrepareArgumentsRunsBeforeSDKToolExecution(t *testing.T) {
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:      t.TempDir(),
		AgentDir: filepath.Join(t.TempDir(), "agent"),
		Model:    sdkTestModel(),
		CustomTools: []SDKTool{{
			Name: "prepared_tool",
			PrepareArguments: func(input map[string]any) map[string]any {
				prepared := cloneSettingsMap(input)
				prepared["path"] = prepared["file_path"]
				delete(prepared, "file_path")
				return prepared
			},
			Execute: func(_ string, input map[string]any) (SDKToolResult, error) {
				return SDKToolResult{Details: input}, nil
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()

	tool := session.sdkTool("prepared_tool")
	if tool == nil {
		t.Fatal("prepared_tool missing")
	}
	result, err := session.executeSDKToolWithUpdates(tool, llm.ToolCall("call-1", "prepared_tool", map[string]any{"file_path": "README.md"}))
	if err != nil {
		t.Fatal(err)
	}
	details, ok := result.Details.(map[string]any)
	if !ok || details["path"] != "README.md" || details["file_path"] != nil {
		t.Fatalf("prepared details = %#v", result.Details)
	}
}

func TestCreateAgentSessionDiscoversSkillsByDefault(t *testing.T) {
	tempDir := t.TempDir()
	writeSDKTestSkill(t, filepath.Join(tempDir, "skills", "test-skill"))

	sessionManager, err := InMemorySessionManager(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            tempDir,
		AgentDir:       tempDir,
		SessionManager: sessionManager,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()

	skills := session.ResourceLoader.GetSkills().Skills
	if len(skills) == 0 || !hasSDKSkill(skills, "test-skill") {
		t.Fatalf("skills = %#v, want test-skill", skills)
	}
}

func TestCreateAgentSessionAllowsEmptySkillsFromResourceLoader(t *testing.T) {
	tempDir := t.TempDir()
	loader := fixedSDKResourceLoader{skills: nil, diagnostics: nil}
	sessionManager, err := InMemorySessionManager(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            tempDir,
		AgentDir:       tempDir,
		SessionManager: sessionManager,
		ResourceLoader: loader,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()

	result := session.ResourceLoader.GetSkills()
	if len(result.Skills) != 0 || len(result.Diagnostics) != 0 {
		t.Fatalf("skills result = %#v, want empty", result)
	}
}

func TestCreateAgentSessionUsesProvidedResourceLoaderSkills(t *testing.T) {
	tempDir := t.TempDir()
	customSkill := agentharness.Skill{
		Name:        "custom-skill",
		Description: "A custom skill",
		FilePath:    "/fake/path/SKILL.md",
		Content:     "Custom body.",
	}
	loader := fixedSDKResourceLoader{skills: []agentharness.Skill{customSkill}}
	sessionManager, err := InMemorySessionManager(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            tempDir,
		AgentDir:       tempDir,
		SessionManager: sessionManager,
		ResourceLoader: loader,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Dispose()

	result := session.ResourceLoader.GetSkills()
	if len(result.Skills) != 1 || result.Skills[0].Name != "custom-skill" || len(result.Diagnostics) != 0 {
		t.Fatalf("skills result = %#v, want custom-skill only", result)
	}
}

type fixedSDKResourceLoader struct {
	skills      []agentharness.Skill
	diagnostics []agentharness.SkillDiagnostic
}

func (l fixedSDKResourceLoader) GetSkills() AgentSessionSkillsResult {
	return AgentSessionSkillsResult{Skills: l.skills, Diagnostics: l.diagnostics}
}

func sdkTestModel() llm.Model {
	return llm.Model{ID: "test-model", Provider: "test", API: "openai-completions", BaseURL: "https://example.test", Input: []string{"text"}}
}

func mkdirAllForSDKTest(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func writeSDKTestSkill(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: test-skill
description: A test skill for SDK tests.
---

# Test Skill

This is a test skill.
`
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findSDKTool(tools []SDKTool, name string) (SDKTool, bool) {
	for _, tool := range tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return SDKTool{}, false
}

func findLLMTool(tools []llm.Tool, name string) *llm.Tool {
	for _, tool := range tools {
		if tool.Name == name {
			copy := tool
			return &copy
		}
	}
	return nil
}

func sdkToolText(result SDKToolResult) string {
	var text string
	for _, part := range result.Content {
		if part.Type == "text" {
			text += part.Text
		}
	}
	return strings.TrimSpace(text)
}

func hasSDKSkill(skills []agentharness.Skill, name string) bool {
	for _, skill := range skills {
		if skill.Name == name {
			return true
		}
	}
	return false
}

func hasPathPrefix(path, prefix string) bool {
	rel, err := filepath.Rel(prefix, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}
