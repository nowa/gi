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
