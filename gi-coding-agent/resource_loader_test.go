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

	t.Run("auto-discovers root markdown skills from project .gi skill dirs", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		writeResourceSkill(t, filepath.Join(cwd, ConfigDirName, "skills", "project-root.md"), "project-root", "Project root skill", "Project content")

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		if !resourceHasSkill(loader.GetSkills().Skills, "project-root") {
			t.Fatalf("skills = %#v", loader.GetSkills().Skills)
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

	t.Run("discovers .agents skills from user dir and project dirs up to git root", func(t *testing.T) {
		home, agentDir, repo, cwd := createResourceLoaderHomeDirs(t, true)
		writeResourceSkill(t, filepath.Join(home, ".agents", "skills", "user-skill", "SKILL.md"), "user-skill", "User skill", "User content")
		writeResourceSkill(t, filepath.Join(repo, ".agents", "skills", "repo-skill", "SKILL.md"), "repo-skill", "Repo skill", "Repo content")
		writeResourceSkill(t, filepath.Join(cwd, ".agents", "skills", "cwd-skill", "SKILL.md"), "cwd-skill", "CWD skill", "CWD content")
		writeResourceSkill(t, filepath.Join(home, "work", ".agents", "skills", "outside-skill", "SKILL.md"), "outside-skill", "Outside skill", "Outside content")
		writeResourceSkill(t, filepath.Join(cwd, ".agents", "skills", "root.md"), "root-md", "Root markdown should be ignored", "Ignored content")

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		skills := loader.GetSkills().Skills
		if !resourceHasSkill(skills, "user-skill") ||
			!resourceHasSkill(skills, "repo-skill") ||
			!resourceHasSkill(skills, "cwd-skill") ||
			resourceHasSkill(skills, "outside-skill") ||
			resourceHasSkill(skills, "root-md") {
			t.Fatalf("skills = %#v", skills)
		}
	})

	t.Run("keeps home .agents user scoped when cwd is under home without git", func(t *testing.T) {
		home, agentDir, _, cwd := createResourceLoaderHomeDirs(t, false)
		writeResourceSkill(t, filepath.Join(home, ".agents", "skills", "user-skill", "SKILL.md"), "user-skill", "User skill", "User content")
		writeResourceSkill(t, filepath.Join(home, "work", ".agents", "skills", "work-skill", "SKILL.md"), "work-skill", "Work skill", "Work content")

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		if got := resourceSkillCount(loader.GetSkills().Skills, "user-skill"); got != 1 {
			t.Fatalf("user-skill count = %d, skills = %#v", got, loader.GetSkills().Skills)
		}
		if !resourceHasSkill(loader.GetSkills().Skills, "work-skill") {
			t.Fatalf("skills = %#v", loader.GetSkills().Skills)
		}
	})

	t.Run("dedupes user skills when .gi agent skills symlink to .agents skills", func(t *testing.T) {
		home, agentDir, _, cwd := createResourceLoaderHomeDirs(t, false)
		userSkills := filepath.Join(home, ".agents", "skills")
		writeResourceSkill(t, filepath.Join(userSkills, "shared-skill", "SKILL.md"), "shared-skill", "Shared skill", "Shared content")
		if err := os.MkdirAll(agentDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(userSkills, filepath.Join(agentDir, "skills")); err != nil {
			t.Fatal(err)
		}

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		if got := resourceSkillCount(loader.GetSkills().Skills, "shared-skill"); got != 1 {
			t.Fatalf("shared-skill count = %d, skills = %#v", got, loader.GetSkills().Skills)
		}
	})

	t.Run("respects .agents skill gitignore without applying parent ignore to .gi", func(t *testing.T) {
		_, agentDir, repo, cwd := createResourceLoaderHomeDirs(t, true)
		writeResourceFile(t, filepath.Join(repo, ".agents", "skills", ".gitignore"), "ignored-skill/\n")
		writeResourceSkill(t, filepath.Join(repo, ".agents", "skills", "keep-skill", "SKILL.md"), "keep-skill", "Keep skill", "Keep content")
		writeResourceSkill(t, filepath.Join(repo, ".agents", "skills", "ignored-skill", "SKILL.md"), "ignored-skill", "Ignored skill", "Ignored content")
		writeResourceFile(t, filepath.Join(repo, ".gitignore"), ".gi/skills/project-skill/\n")
		writeResourceSkill(t, filepath.Join(cwd, ConfigDirName, "skills", "project-skill", "SKILL.md"), "project-skill", "Project skill", "Project content")

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		skills := loader.GetSkills().Skills
		if !resourceHasSkill(skills, "keep-skill") ||
			resourceHasSkill(skills, "ignored-skill") ||
			!resourceHasSkill(skills, "project-skill") {
			t.Fatalf("skills = %#v diagnostics = %#v", skills, loader.GetSkills().Diagnostics)
		}
	})

	t.Run("resolves local extension paths from settings", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		extensionPath := filepath.Join(agentDir, "extensions", "my-extension.gi.json")
		writeGiProtocolExtensionDescriptor(t, extensionPath)
		settings := NewInMemorySettingsManager(map[string]any{"extensions": []any{"extensions/my-extension.gi.json"}})

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings})
		loader.Reload()

		if len(loader.GetExtensions().Extensions) != 1 || loader.GetExtensions().Extensions[0].Path != extensionPath {
			t.Fatalf("extensions = %#v", loader.GetExtensions().Extensions)
		}
	})

	t.Run("resolves skill paths from settings", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		skillPath := filepath.Join(agentDir, "custom-skills", "my-skill", "SKILL.md")
		writeResourceSkill(t, skillPath, "my-skill", "A test skill", "Content")
		settings := NewInMemorySettingsManager(map[string]any{"skills": []any{"custom-skills"}})

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings})
		loader.Reload()

		if !resourceHasSkill(loader.GetSkills().Skills, "my-skill") {
			t.Fatalf("skills = %#v", loader.GetSkills().Skills)
		}
	})

	t.Run("resolves project extension paths relative to .gi", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		extensionPath := filepath.Join(cwd, ConfigDirName, "extensions", "project-ext.gi.json")
		writeGiProtocolExtensionDescriptor(t, extensionPath)
		settings := NewSettingsManager(cwd, agentDir)
		settings.SetProjectExtensionPaths([]string{"extensions/project-ext.gi.json"})

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings})
		loader.Reload()

		if len(loader.GetExtensions().Extensions) != 1 || loader.GetExtensions().Extensions[0].Path != extensionPath {
			t.Fatalf("extensions = %#v", loader.GetExtensions().Extensions)
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
		writeGiProtocolExtensionDescriptor(t, filepath.Join(agentDir, "extensions", "disabled.gi.json"))
		writeResourceSkill(t, filepath.Join(agentDir, "skills", "skip-skill", "SKILL.md"), "skip-skill", "Skip me", "Content")
		writeResourceFile(t, filepath.Join(agentDir, "prompts", "skip.md"), "Skip prompt")
		writeJSON(t, filepath.Join(agentDir, "themes", "skip.json"), map[string]any{"name": "skip-theme"})
		settings := NewInMemorySettingsManager(map[string]any{
			"extensions": []any{"-extensions/disabled.gi.json"},
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

	t.Run("applies bang filters for top-level settings resources", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		writeGiProtocolExtensionDescriptor(t, filepath.Join(agentDir, "extensions", "keep.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(agentDir, "extensions", "remove.gi.json"))
		writeResourceSkill(t, filepath.Join(agentDir, "skills", "good-skill", "SKILL.md"), "good-skill", "Good", "Content")
		writeResourceSkill(t, filepath.Join(agentDir, "skills", "bad-skill", "SKILL.md"), "bad-skill", "Bad", "Content")
		writeResourceFile(t, filepath.Join(agentDir, "prompts", "review.md"), "Review code")
		writeResourceFile(t, filepath.Join(agentDir, "prompts", "explain.md"), "Explain code")
		writeJSON(t, filepath.Join(agentDir, "themes", "dark.json"), map[string]any{"name": "dark"})
		writeJSON(t, filepath.Join(agentDir, "themes", "funky.json"), map[string]any{"name": "funky"})
		settings := NewInMemorySettingsManager(map[string]any{
			"extensions": []any{"!**/remove.gi.json"},
			"skills":     []any{"!**/bad-skill"},
			"prompts":    []any{"!explain.md"},
			"themes":     []any{"!funky.json"},
		})

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings})
		loader.Reload()

		if !protocolExtensionHasSuffix(loader.GetExtensions().Extensions, "keep.gi.json") ||
			protocolExtensionHasSuffix(loader.GetExtensions().Extensions, "remove.gi.json") ||
			!resourceHasSkill(loader.GetSkills().Skills, "good-skill") ||
			resourceHasSkill(loader.GetSkills().Skills, "bad-skill") ||
			!resourceHasPrompt(loader.GetPrompts().Prompts, "review") ||
			resourceHasPrompt(loader.GetPrompts().Prompts, "explain") ||
			resourceFindTheme(loader.GetThemes().Themes, "dark") == nil ||
			resourceFindTheme(loader.GetThemes().Themes, "funky") != nil {
			t.Fatalf("resources not filtered: extensions=%#v skills=%#v prompts=%#v themes=%#v",
				loader.GetExtensions(), loader.GetSkills(), loader.GetPrompts(), loader.GetThemes())
		}
	})

	t.Run("force-includes top-level extensions after broad exclusion", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		writeGiProtocolExtensionDescriptor(t, filepath.Join(agentDir, "extensions", "keep.gi.json"))
		writeGiProtocolExtensionDescriptor(t, filepath.Join(agentDir, "extensions", "remove.gi.json"))
		settings := NewInMemorySettingsManager(map[string]any{
			"extensions": []any{"!extensions/*.gi.json", "+extensions/keep.gi.json"},
		})

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings})
		loader.Reload()

		if !protocolExtensionHasSuffix(loader.GetExtensions().Extensions, "keep.gi.json") ||
			protocolExtensionHasSuffix(loader.GetExtensions().Extensions, "remove.gi.json") {
			t.Fatalf("extensions = %#v", loader.GetExtensions().Extensions)
		}
	})

	t.Run("force-excludes top-level resources after force-include", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		writeGiProtocolExtensionDescriptor(t, filepath.Join(agentDir, "extensions", "keep.gi.json"))
		settings := NewInMemorySettingsManager(map[string]any{
			"extensions": []any{"!extensions/*.gi.json", "+extensions/keep.gi.json", "-extensions/keep.gi.json"},
		})

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings})
		loader.Reload()

		if len(loader.GetExtensions().Extensions) != 0 {
			t.Fatalf("extensions = %#v", loader.GetExtensions().Extensions)
		}
	})

	t.Run("dedupes symlinked user and project resources with project path winning", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		shared := filepath.Join(filepath.Dir(agentDir), "shared-extensions")
		writeGiProtocolExtensionDescriptor(t, filepath.Join(shared, "shared.gi.json"))
		if err := os.Symlink(shared, filepath.Join(agentDir, "extensions")); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(cwd, ConfigDirName), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(shared, filepath.Join(cwd, ConfigDirName, "extensions")); err != nil {
			t.Fatal(err)
		}
		sharedSkills := filepath.Join(filepath.Dir(agentDir), "shared-skills")
		writeResourceSkill(t, filepath.Join(sharedSkills, "shared-skill", "SKILL.md"), "shared-skill", "Shared skill", "Shared content")
		if err := os.Symlink(sharedSkills, filepath.Join(agentDir, "skills")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(sharedSkills, filepath.Join(cwd, ConfigDirName, "skills")); err != nil {
			t.Fatal(err)
		}
		sharedPrompts := filepath.Join(filepath.Dir(agentDir), "shared-prompts")
		writeResourceFile(t, filepath.Join(sharedPrompts, "shared.md"), "Shared prompt")
		if err := os.Symlink(sharedPrompts, filepath.Join(agentDir, "prompts")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(sharedPrompts, filepath.Join(cwd, ConfigDirName, "prompts")); err != nil {
			t.Fatal(err)
		}
		sharedThemes := filepath.Join(filepath.Dir(agentDir), "shared-themes")
		writeJSON(t, filepath.Join(sharedThemes, "shared.json"), map[string]any{"name": "shared-theme"})
		if err := os.Symlink(sharedThemes, filepath.Join(agentDir, "themes")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(sharedThemes, filepath.Join(cwd, ConfigDirName, "themes")); err != nil {
			t.Fatal(err)
		}

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		extensions := loader.GetExtensions()
		if len(extensions.Extensions) != 1 || len(extensions.Errors) != 0 {
			t.Fatalf("extensions = %#v", extensions)
		}
		want := filepath.Join(cwd, ConfigDirName, "extensions", "shared.gi.json")
		if extensions.Extensions[0].Path != want {
			t.Fatalf("extension path = %q, want %q", extensions.Extensions[0].Path, want)
		}
		if got := resourceSkillCount(loader.GetSkills().Skills, "shared-skill"); got != 1 {
			t.Fatalf("shared-skill count = %d, skills = %#v", got, loader.GetSkills().Skills)
		}
		if skill := resourceFindSkill(loader.GetSkills().Skills, "shared-skill"); skill == nil ||
			skill.FilePath != filepath.Join(cwd, ConfigDirName, "skills", "shared-skill", "SKILL.md") {
			t.Fatalf("skill = %#v", skill)
		}
		if got := resourcePromptCount(loader.GetPrompts().Prompts, "shared"); got != 1 {
			t.Fatalf("shared prompt count = %d, prompts = %#v", got, loader.GetPrompts().Prompts)
		}
		if prompt := resourceFindPrompt(loader.GetPrompts().Prompts, "shared"); prompt == nil ||
			prompt.FilePath != filepath.Join(cwd, ConfigDirName, "prompts", "shared.md") {
			t.Fatalf("prompt = %#v", prompt)
		}
		if got := resourceThemeCount(loader.GetThemes().Themes, "shared-theme"); got != 1 {
			t.Fatalf("shared-theme count = %d, themes = %#v", got, loader.GetThemes().Themes)
		}
		if theme := resourceFindTheme(loader.GetThemes().Themes, "shared-theme"); theme == nil ||
			theme.SourcePath != filepath.Join(cwd, ConfigDirName, "themes", "shared.json") {
			t.Fatalf("theme = %#v", theme)
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

func createResourceLoaderHomeDirs(t *testing.T, withGitRoot bool) (string, string, string, string) {
	t.Helper()
	home := t.TempDir()
	agentDir := filepath.Join(home, ConfigDirName, "agent")
	repo := filepath.Join(home, "work", "repo")
	cwd := filepath.Join(repo, "nested")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	if withGitRoot {
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home, agentDir, repo, cwd
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

func resourceSkillCount(skills []agentharness.Skill, name string) int {
	var count int
	for _, skill := range skills {
		if skill.Name == name {
			count++
		}
	}
	return count
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

func resourcePromptCount(prompts []PromptTemplate, name string) int {
	var count int
	for _, prompt := range prompts {
		if prompt.Name == name {
			count++
		}
	}
	return count
}

func resourceFindTheme(themes []ResourceTheme, name string) *ResourceTheme {
	for i := range themes {
		if themes[i].Name == name {
			return &themes[i]
		}
	}
	return nil
}

func resourceThemeCount(themes []ResourceTheme, name string) int {
	var count int
	for _, theme := range themes {
		if theme.Name == name {
			count++
		}
	}
	return count
}
