package gicodingagent

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
)

func TestDefaultResourceLoaderPiBasics(t *testing.T) {
	t.Run("initializes with empty results before reload", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})

		if len(loader.GetExtensions().Extensions) != 0 || len(loader.GetSkills().Skills) != 0 ||
			len(loader.GetPrompts().Prompts) != 0 || len(loader.GetThemes().Themes) != 0 {
			t.Fatalf("loader should start empty: extensions=%#v skills=%#v prompts=%#v themes=%#v",
				loader.GetExtensions(), loader.GetSkills(), loader.GetPrompts(), loader.GetThemes())
		}
	})

	t.Run("discovers skills and ignores extra markdown in skill dirs", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		writeResourceSkill(t, filepath.Join(agentDir, "skills", "test-skill.md"), "test-skill", "A test skill", "Skill content")
		writeResourceSkill(t, filepath.Join(agentDir, "skills", "pi-skills", "browser-tools", "SKILL.md"), "browser-tools", "Browser tools", "Browser content")
		writeResourceFile(t, filepath.Join(agentDir, "skills", "pi-skills", "browser-tools", "EFFICIENCY.md"), "No frontmatter")

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		skills := loader.GetSkills()
		if !resourceHasSkill(skills.Skills, "test-skill") || !resourceHasSkill(skills.Skills, "browser-tools") {
			t.Fatalf("skills = %#v", skills.Skills)
		}
		for _, diagnostic := range skills.Diagnostics {
			if strings.HasSuffix(diagnostic.Path, "EFFICIENCY.md") {
				t.Fatalf("unexpected diagnostic for extra markdown: %#v", skills.Diagnostics)
			}
		}
	})

	t.Run("discovers prompts from agent dir", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		writeResourceFile(t, filepath.Join(agentDir, "prompts", "test-prompt.md"), "---\ndescription: A test prompt\n---\nPrompt content.")

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		if !resourceHasPrompt(loader.GetPrompts().Prompts, "test-prompt") {
			t.Fatalf("prompts = %#v", loader.GetPrompts().Prompts)
		}
	})

	t.Run("prefers project resources over user on name collisions", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		userPrompt := filepath.Join(agentDir, "prompts", "commit.md")
		projectPrompt := filepath.Join(cwd, ConfigDirName, "prompts", "commit.md")
		writeResourceFile(t, userPrompt, "User prompt")
		writeResourceFile(t, projectPrompt, "Project prompt")
		userSkill := filepath.Join(agentDir, "skills", "collision-skill", "SKILL.md")
		projectSkill := filepath.Join(cwd, ConfigDirName, "skills", "collision-skill", "SKILL.md")
		writeResourceSkill(t, userSkill, "collision-skill", "user", "User skill")
		writeResourceSkill(t, projectSkill, "collision-skill", "project", "Project skill")
		userTheme := filepath.Join(agentDir, "themes", "collision.json")
		projectTheme := filepath.Join(cwd, ConfigDirName, "themes", "collision.json")
		writeJSON(t, userTheme, map[string]any{"name": "collision-theme", "accent": "#111111"})
		writeJSON(t, projectTheme, map[string]any{"name": "collision-theme", "accent": "#ff00ff"})

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		if prompt := resourceFindPrompt(loader.GetPrompts().Prompts, "commit"); prompt == nil || prompt.FilePath != projectPrompt {
			t.Fatalf("prompt = %#v", prompt)
		}
		if skill := resourceFindSkill(loader.GetSkills().Skills, "collision-skill"); skill == nil || skill.FilePath != projectSkill {
			t.Fatalf("skill = %#v", skill)
		}
		if theme := resourceFindTheme(loader.GetThemes().Themes, "collision-theme"); theme == nil || theme.SourcePath != projectTheme {
			t.Fatalf("theme = %#v", theme)
		}
	})

	t.Run("honors settings exclusions for auto-discovered resources", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		writeProtocolExtensionFile(t, filepath.Join(agentDir, "extensions", "disabled.ts"))
		writeResourceSkill(t, filepath.Join(agentDir, "skills", "skip-skill", "SKILL.md"), "skip-skill", "Skip me", "Content")
		writeResourceFile(t, filepath.Join(agentDir, "prompts", "skip.md"), "Skip prompt")
		writeJSON(t, filepath.Join(agentDir, "themes", "skip.json"), map[string]any{"name": "skip-theme"})
		settings := NewInMemorySettingsManager(map[string]any{
			"extensions": []any{"-extensions/disabled.ts"},
			"skills":     []any{"-skills/skip-skill"},
			"prompts":    []any{"-prompts/skip.md"},
			"themes":     []any{"-themes/skip.json"},
		})

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings})
		loader.Reload()

		if len(loader.GetExtensions().Extensions) != 0 || resourceHasSkill(loader.GetSkills().Skills, "skip-skill") ||
			resourceHasPrompt(loader.GetPrompts().Prompts, "skip") || resourceFindTheme(loader.GetThemes().Themes, "skip-theme") != nil {
			t.Fatalf("resources not filtered: extensions=%#v skills=%#v prompts=%#v themes=%#v",
				loader.GetExtensions(), loader.GetSkills(), loader.GetPrompts(), loader.GetThemes())
		}
	})

	t.Run("dedupes symlinked user and project extensions with project path winning", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		shared := filepath.Join(filepath.Dir(agentDir), "shared-extensions")
		writeProtocolExtensionFile(t, filepath.Join(shared, "shared.ts"))
		if err := os.Symlink(shared, filepath.Join(agentDir, "extensions")); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(cwd, ConfigDirName), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(shared, filepath.Join(cwd, ConfigDirName, "extensions")); err != nil {
			t.Fatal(err)
		}

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		extensions := loader.GetExtensions()
		if len(extensions.Extensions) != 1 || len(extensions.Errors) != 0 {
			t.Fatalf("extensions = %#v", extensions)
		}
		want := filepath.Join(cwd, ConfigDirName, "extensions", "shared.ts")
		if extensions.Extensions[0].Path != want {
			t.Fatalf("extension path = %q, want %q", extensions.Extensions[0].Path, want)
		}
	})

	t.Run("discovers context and system prompt files", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		writeResourceFile(t, filepath.Join(cwd, "AGENTS.md"), "# Project Guidelines")
		writeResourceFile(t, filepath.Join(cwd, ConfigDirName, "SYSTEM.md"), "You are helpful.")
		writeResourceFile(t, filepath.Join(cwd, ConfigDirName, "APPEND_SYSTEM.md"), "Additional instructions.")

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		if len(loader.GetAgentsFiles().AgentsFiles) != 1 || !strings.HasSuffix(loader.GetAgentsFiles().AgentsFiles[0].Path, "AGENTS.md") {
			t.Fatalf("agents files = %#v", loader.GetAgentsFiles())
		}
		if loader.GetSystemPrompt() != "You are helpful." || !strings.Contains(loader.GetAppendSystemPrompt(), "Additional instructions.") {
			t.Fatalf("system=%q append=%q", loader.GetSystemPrompt(), loader.GetAppendSystemPrompt())
		}

		noContext := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, NoContextFiles: true})
		noContext.Reload()
		if len(noContext.GetAgentsFiles().AgentsFiles) != 0 {
			t.Fatalf("noContext agents files = %#v", noContext.GetAgentsFiles())
		}
	})

	t.Run("respects noSkills while still loading additional skill paths", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		writeResourceSkill(t, filepath.Join(agentDir, "skills", "test-skill.md"), "test-skill", "A test skill", "Content")
		customDir := filepath.Join(filepath.Dir(agentDir), "custom-skills")
		writeResourceSkill(t, filepath.Join(customDir, "custom.md"), "custom", "Custom skill", "Content")

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, NoSkills: true, AdditionalSkillPaths: []string{customDir}})
		loader.Reload()

		skills := loader.GetSkills().Skills
		if resourceHasSkill(skills, "test-skill") || !resourceHasSkill(skills, "custom") {
			t.Fatalf("skills = %#v", skills)
		}
	})

	t.Run("applies skills and system prompt overrides", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		injected := agentharness.Skill{Name: "injected", Description: "Injected skill", FilePath: "/fake/path"}
		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{
			CWD:      cwd,
			AgentDir: agentDir,
			SkillsOverride: func() ResourceSkillsResult {
				return ResourceSkillsResult{Skills: []agentharness.Skill{injected}}
			},
			SystemPromptOverride: func() string { return "Custom system prompt" },
		})
		loader.Reload()

		if !reflect.DeepEqual(loader.GetSkills().Skills, []agentharness.Skill{injected}) || loader.GetSystemPrompt() != "Custom system prompt" {
			t.Fatalf("skills=%#v system=%q", loader.GetSkills().Skills, loader.GetSystemPrompt())
		}
	})
}

func createResourceLoaderDirs(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	agentDir := filepath.Join(root, "agent")
	cwd := filepath.Join(root, "project")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	return agentDir, cwd
}

func writeResourceSkill(t *testing.T, path, name, description, content string) {
	t.Helper()
	writeResourceFile(t, path, "---\nname: "+name+"\ndescription: "+description+"\n---\n"+content)
}

func writeResourceFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func resourceHasSkill(skills []agentharness.Skill, name string) bool {
	return resourceFindSkill(skills, name) != nil
}

func resourceFindSkill(skills []agentharness.Skill, name string) *agentharness.Skill {
	for i := range skills {
		if skills[i].Name == name {
			return &skills[i]
		}
	}
	return nil
}

func resourceHasPrompt(prompts []PromptTemplate, name string) bool {
	return resourceFindPrompt(prompts, name) != nil
}

func resourceFindPrompt(prompts []PromptTemplate, name string) *PromptTemplate {
	for i := range prompts {
		if prompts[i].Name == name {
			return &prompts[i]
		}
	}
	return nil
}

func resourceFindTheme(themes []ResourceTheme, name string) *ResourceTheme {
	for i := range themes {
		if themes[i].Name == name {
			return &themes[i]
		}
	}
	return nil
}
