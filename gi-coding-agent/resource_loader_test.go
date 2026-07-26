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

	t.Run("clears the cache on resource loader reload", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		extensionPath := filepath.Join(
			agentDir,
			"extensions",
			"cached.gi.json",
		)
		writeCachedExtension := func(id string) {
			writeJSON(t, extensionPath, map[string]any{
				"gi": map[string]any{
					"extensionProtocol": "descriptor.v1",
					"id":                id,
				},
			})
		}
		writeCachedExtension("first")

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{
			CWD:      cwd,
			AgentDir: agentDir,
		})
		loader.Reload()
		first := loader.GetExtensions()
		if len(first.Extensions) != 1 ||
			first.Extensions[0].Metadata.Source != "extension:first" {
			t.Fatalf("first extensions = %#v", first.Extensions)
		}

		writeCachedExtension("second")
		loader.Reload()
		second := loader.GetExtensions()
		if len(second.Extensions) != 1 ||
			second.Extensions[0].Metadata.Source != "extension:second" {
			t.Fatalf("second extensions = %#v", second.Extensions)
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

	t.Run("reload applies updated top-level prompt settings", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		writeResourceFile(t, filepath.Join(agentDir, "prompts", "test.md"), "Echo test prompt")
		settingsPath := filepath.Join(agentDir, "settings.json")
		writeSettingsJSON(t, settingsPath, map[string]any{})
		settings := NewSettingsManager(cwd, agentDir)
		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings})
		loader.Reload()
		if !resourceHasPrompt(loader.GetPrompts().Prompts, "test") {
			t.Fatalf("initial prompts = %#v", loader.GetPrompts().Prompts)
		}

		writeSettingsJSON(t, settingsPath, map[string]any{"prompts": []any{"-prompts/test.md"}})
		loader.Reload()
		if resourceHasPrompt(loader.GetPrompts().Prompts, "test") {
			t.Fatalf("stale prompt settings after reload: %#v", loader.GetPrompts().Prompts)
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

	t.Run("discovers home .agents skills when CLI agent dir is overridden", func(t *testing.T) {
		home := t.TempDir()
		agentDir := filepath.Join(t.TempDir(), "custom-agent")
		cwd := filepath.Join(home, "work")
		t.Setenv("HOME", home)
		t.Setenv("GI_CODING_AGENT_DIR", agentDir)
		writeResourceSkill(t, filepath.Join(home, ".agents", "skills", "user-skill", "SKILL.md"), "user-skill", "User skill", "User content")
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatal(err)
		}

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		if !resourceHasSkill(loader.GetSkills().Skills, "user-skill") {
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

	t.Run("skill collision precedence project user package", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		pkgDir := filepath.Join(filepath.Dir(agentDir), "fake-package")
		packageSkill := filepath.Join(pkgDir, "skills", "web-fetch", "SKILL.md")
		writeResourceSkill(t, packageSkill, "web-fetch", "Package web fetch", "Package skill")
		writeJSON(t, filepath.Join(pkgDir, "gi.package.json"), map[string]any{
			"gi": map[string]any{"skills": []any{"skills/web-fetch"}},
		})
		writeSettingsJSON(t, filepath.Join(agentDir, "settings.json"), map[string]any{"packages": []any{pkgDir}})
		userSkill := filepath.Join(agentDir, "skills", "web-fetch", "SKILL.md")
		projectSkill := filepath.Join(cwd, ConfigDirName, "skills", "web-fetch", "SKILL.md")
		writeResourceSkill(t, userSkill, "web-fetch", "User web fetch", "User skill")
		writeResourceSkill(t, projectSkill, "web-fetch", "Project web fetch", "Project skill")

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		skills := loader.GetSkills()
		webFetch := resourceFindSkill(skills.Skills, "web-fetch")
		if webFetch == nil || webFetch.FilePath != projectSkill || webFetch.Description != "Project web fetch" {
			t.Fatalf("web-fetch = %#v", webFetch)
		}
		if !skillDiagnosticsMention(skills.Diagnostics, packageSkill) || !skillDiagnosticsMention(skills.Diagnostics, userSkill) {
			t.Fatalf("skill diagnostics = %#v", skills.Diagnostics)
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

	t.Run("keeps both extensions loaded when command names collide", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		projectExt := filepath.Join(cwd, ConfigDirName, "extensions", "project.gi.json")
		userExt := filepath.Join(agentDir, "extensions", "user.gi.json")
		writeJSON(t, projectExt, map[string]any{"gi": map[string]any{
			"extensionProtocol": "descriptor.v1",
			"commands": []any{
				map[string]any{"name": "deploy", "description": "project deploy"},
				map[string]any{"name": "project-only", "description": "project only"},
			},
		}})
		writeJSON(t, userExt, map[string]any{"gi": map[string]any{
			"extensionProtocol": "descriptor.v1",
			"commands": []any{
				map[string]any{"name": "deploy", "description": "user deploy"},
				map[string]any{"name": "user-only", "description": "user only"},
			},
		}})

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		extensions := loader.GetExtensions()
		if len(extensions.Extensions) != 2 || len(extensions.Errors) != 0 {
			t.Fatalf("extensions = %#v", extensions)
		}
		if got := extensions.Runtime.CommandInvocationNames(); !reflect.DeepEqual(got, []string{"deploy:1", "deploy:2", "project-only", "user-only"}) {
			t.Fatalf("commands = %#v", got)
		}
	})

	t.Run("applies cli extension flag values after loading descriptors", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		projectExt := filepath.Join(cwd, ConfigDirName, "extensions", "project.gi.json")
		writeJSON(t, projectExt, map[string]any{"gi": map[string]any{
			"extensionProtocol": "descriptor.v1",
			"flags": []any{
				map[string]any{"name": "review-mode", "description": "Review mode", "type": "boolean", "default": false},
				map[string]any{"name": "profile", "description": "Profile", "type": "string", "default": "default"},
			},
		}})

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()
		diagnostics := loader.ApplyExtensionFlagValues(map[string]any{
			"review-mode": true,
			"profile":     "fast",
		}, false)
		if len(diagnostics) != 0 {
			t.Fatalf("diagnostics = %#v", diagnostics)
		}

		runtime := loader.GetExtensions().Runtime
		if got := runtime.FlagValue("review-mode"); got != true {
			t.Fatalf("review-mode = %#v", got)
		}
		if got := runtime.FlagValue("profile"); got != "fast" {
			t.Fatalf("profile = %#v", got)
		}
	})

	t.Run("reports unknown cli extension flags when deferred registration is disabled", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		diagnostics := loader.ApplyExtensionFlagValues(map[string]any{"missing": true}, false)
		if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Error, "Unknown option: --missing") {
			t.Fatalf("diagnostics = %#v", diagnostics)
		}
		if !resourceExtensionErrorsContain(loader.GetExtensions().Errors, "Unknown option: --missing") {
			t.Fatalf("extension errors = %#v", loader.GetExtensions().Errors)
		}
	})

	t.Run("loads extended skills and prompts with extension metadata", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		skillDir := filepath.Join(t.TempDir(), "extra-skills", "extra-skill")
		skillPath := filepath.Join(skillDir, "SKILL.md")
		writeResourceSkill(t, skillPath, "extra-skill", "Extra skill", "Extra content")
		promptPath := filepath.Join(t.TempDir(), "extra-prompts", "extra.md")
		writeResourceFile(t, promptPath, "---\ndescription: Extra prompt\n---\nExtra prompt content")
		themePath := filepath.Join(t.TempDir(), "extra-themes", "extra.json")
		writeJSON(t, themePath, map[string]any{"name": "extra-theme"})

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()
		loader.ExtendResources(ResourceExtension{
			SkillPaths: []ResourceSkillPath{{
				Path:     skillDir,
				Metadata: ProtocolSourceInfo{Source: "extension:extra", Scope: "temporary", Origin: "top-level"},
			}},
			PromptPaths: []ResourcePromptPath{{
				Path:     promptPath,
				Metadata: ProtocolSourceInfo{Source: "extension:extra", Scope: "temporary", Origin: "top-level"},
			}},
			ThemePaths: []ResourceThemePath{{
				Path:     themePath,
				Metadata: ProtocolSourceInfo{Source: "extension:extra", Scope: "temporary", Origin: "top-level"},
			}},
		})

		skill := resourceFindSkill(loader.GetSkills().Skills, "extra-skill")
		if skill == nil {
			t.Fatalf("skills = %#v", loader.GetSkills().Skills)
		}
		sourceInfo, _ := skill.SourceInfo.(ProtocolSourceInfo)
		if sourceInfo.Source != "extension:extra" || sourceInfo.Path != skillPath {
			t.Fatalf("skill = %#v source = %#v", skill, sourceInfo)
		}
		prompt := resourceFindPrompt(loader.GetPrompts().Prompts, "extra")
		if prompt == nil || prompt.SourceInfo.Source != "extension:extra" || prompt.SourceInfo.Path != promptPath {
			t.Fatalf("prompt = %#v", prompt)
		}
		if theme := resourceFindTheme(loader.GetThemes().Themes, "extra-theme"); theme == nil || theme.SourcePath != themePath {
			t.Fatalf("theme = %#v themes=%#v", theme, loader.GetThemes().Themes)
		}
	})

	t.Run("loads dynamic resources discovered by extension event", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		skillDir := filepath.Join(t.TempDir(), "dynamic-skills", "dynamic-skill")
		skillPath := filepath.Join(skillDir, "SKILL.md")
		writeResourceSkill(t, skillPath, "dynamic-skill", "Dynamic skill", "Dynamic content")
		promptPath := filepath.Join(t.TempDir(), "dynamic-prompts", "dynamic.md")
		writeResourceFile(t, promptPath, "---\ndescription: Dynamic prompt\n---\nDynamic prompt content")
		themePath := filepath.Join(t.TempDir(), "dynamic-themes", "dynamic.json")
		writeJSON(t, themePath, map[string]any{"name": "dynamic-theme"})
		var reasons []string

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{
			CWD:      cwd,
			AgentDir: agentDir,
			ExtensionFactories: []ProtocolExtensionFactory{{
				Path: "dynamic-resources.gi.json",
				Factory: func(ctx *ProtocolExtensionContext) error {
					return ctx.On(ProtocolEventResourcesDiscover, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
						if event.CWD != cwd {
							t.Fatalf("event cwd = %q, want %q", event.CWD, cwd)
						}
						reasons = append(reasons, event.Reason)
						return ProtocolEventResult{
							ResourcesSet: true,
							Resources: ResourceExtension{
								SkillPaths:  []ResourceSkillPath{{Path: skillDir}},
								PromptPaths: []ResourcePromptPath{{Path: promptPath}},
								ThemePaths:  []ResourceThemePath{{Path: themePath}},
							},
						}, nil
					})
				},
			}},
		})

		loader.Reload()
		if !reflect.DeepEqual(reasons, []string{"startup"}) {
			t.Fatalf("reasons after first reload = %#v", reasons)
		}
		skill := resourceFindSkill(loader.GetSkills().Skills, "dynamic-skill")
		if skill == nil {
			t.Fatalf("skills = %#v", loader.GetSkills().Skills)
		}
		sourceInfo, _ := skill.SourceInfo.(ProtocolSourceInfo)
		if sourceInfo.Source != "inline" || sourceInfo.Scope != "temporary" || sourceInfo.Origin != "top-level" || sourceInfo.Path != skillPath {
			t.Fatalf("skill source = %#v", sourceInfo)
		}
		prompt := resourceFindPrompt(loader.GetPrompts().Prompts, "dynamic")
		if prompt == nil || prompt.SourceInfo.Source != "inline" || prompt.SourceInfo.Path != promptPath {
			t.Fatalf("prompt = %#v", prompt)
		}
		if theme := resourceFindTheme(loader.GetThemes().Themes, "dynamic-theme"); theme == nil || theme.SourcePath != themePath {
			t.Fatalf("theme = %#v themes=%#v", theme, loader.GetThemes().Themes)
		}

		loader.Reload()
		if !reflect.DeepEqual(reasons, []string{"startup", "reload"}) {
			t.Fatalf("reasons after second reload = %#v", reasons)
		}
		if got := resourceSkillCount(loader.GetSkills().Skills, "dynamic-skill"); got != 1 {
			t.Fatalf("dynamic skill count = %d skills=%#v", got, loader.GetSkills().Skills)
		}
		if got := resourcePromptCount(loader.GetPrompts().Prompts, "dynamic"); got != 1 {
			t.Fatalf("dynamic prompt count = %d prompts=%#v", got, loader.GetPrompts().Prompts)
		}
		if got := resourceThemeCount(loader.GetThemes().Themes, "dynamic-theme"); got != 1 {
			t.Fatalf("dynamic theme count = %d themes=%#v", got, loader.GetThemes().Themes)
		}
	})

	t.Run("loads configured official packages through protocol descriptors", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		settings := NewInMemorySettingsManager(map[string]any{
			"packages": []any{"official:gi-plan-mode", "official:gi-todo-widget"},
		})

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, SettingsManager: settings})
		loader.Reload()

		extensions := loader.GetExtensions()
		if len(extensions.Errors) != 0 {
			t.Fatalf("extension errors = %#v", extensions.Errors)
		}
		if got := extensions.Runtime.CommandInvocationNames(); !reflect.DeepEqual(got, []string{"plan", "plan-review", "todo"}) {
			t.Fatalf("commands = %#v", got)
		}
		todoTool := findDynamicSDKTool(extensions.Runtime.RegisteredTools(), "todo_read")
		if todoTool == nil {
			t.Fatalf("tools = %#v", extensions.Runtime.RegisteredTools())
		}
		sourceInfo := todoTool.SourceInfo
		if sourceInfo.Source != "official:gi-todo-widget" || sourceInfo.Origin != "package" {
			t.Fatalf("tool source = %#v", sourceInfo)
		}
		if resourceFindSkill(loader.GetSkills().Skills, "plan-mode") == nil ||
			resourceFindSkill(loader.GetSkills().Skills, "todo-widget") == nil {
			t.Fatalf("skills = %#v", loader.GetSkills().Skills)
		}
		if resourceFindPrompt(loader.GetPrompts().Prompts, "plan") == nil ||
			resourceFindPrompt(loader.GetPrompts().Prompts, "todo") == nil {
			t.Fatalf("prompts = %#v", loader.GetPrompts().Prompts)
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
		extensions.Runtime.BindSession(session)
		if err := session.Prompt("/plan"); err != nil {
			t.Fatal(err)
		}
		if !resourceHasCustomEntry(sessionManager.GetEntries(), "gi-plan-mode.command") {
			t.Fatalf("entries = %#v", sessionManager.GetEntries())
		}
		writeTool := findDynamicSDKTool(extensions.Runtime.RegisteredTools(), "todo_write")
		if writeTool == nil || writeTool.Execute == nil {
			t.Fatalf("tools = %#v", extensions.Runtime.RegisteredTools())
		}
		if _, err := writeTool.Execute("todo-write", map[string]any{"todos": []any{"Ship official packages"}}); err != nil {
			t.Fatal(err)
		}
		readTool := findDynamicSDKTool(extensions.Runtime.RegisteredTools(), "todo_read")
		if readTool == nil || readTool.Execute == nil {
			t.Fatalf("tools = %#v", extensions.Runtime.RegisteredTools())
		}
		result, err := readTool.Execute("todo-read", nil)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sdkToolText(result), "Ship official packages") {
			t.Fatalf("todo read result = %#v", result)
		}
	})

	t.Run("detects tool conflicts and keeps explicit extension first", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		globalExt := filepath.Join(agentDir, "extensions", "global.gi.json")
		explicitExt := filepath.Join(t.TempDir(), "explicit.gi.json")
		writeJSON(t, globalExt, map[string]any{"gi": map[string]any{
			"extensionProtocol": "descriptor.v1",
			"commands":          []any{map[string]any{"name": "deploy", "description": "global deploy"}},
			"tools":             []any{map[string]any{"name": "duplicate-tool", "description": "global tool"}},
			"flags":             []any{map[string]any{"name": "plan-mode", "description": "global plan", "type": "boolean", "default": false}},
		}})
		writeJSON(t, explicitExt, map[string]any{"gi": map[string]any{
			"extensionProtocol": "descriptor.v1",
			"commands":          []any{map[string]any{"name": "deploy", "description": "explicit deploy"}},
			"tools":             []any{map[string]any{"name": "duplicate-tool", "description": "explicit tool"}},
			"flags":             []any{map[string]any{"name": "plan-mode", "description": "explicit plan", "type": "boolean", "default": true}},
		}})

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, AdditionalExtensionPaths: []string{explicitExt}})
		loader.Reload()

		extensions := loader.GetExtensions()
		if !resourceExtensionErrorsContain(extensions.Errors, "duplicate-tool") || !resourceExtensionErrorsContain(extensions.Errors, "conflicts") {
			t.Fatalf("errors = %#v", extensions.Errors)
		}
		if !resourceExtensionErrorsContain(extensions.Errors, `Flag "--plan-mode" conflicts`) {
			t.Fatalf("errors = %#v", extensions.Errors)
		}
		tool := findDynamicSDKTool(extensions.Runtime.RegisteredTools(), "duplicate-tool")
		if tool == nil || tool.Description != "explicit tool" {
			t.Fatalf("tool = %#v", tool)
		}
		flags := extensions.Runtime.Flags()
		if len(flags) != 1 || flags[0].Description != "explicit plan" || extensions.Runtime.FlagValue("plan-mode") != true {
			t.Fatalf("flags = %#v value=%#v", flags, extensions.Runtime.FlagValue("plan-mode"))
		}
		if got := extensions.Runtime.CommandInvocationNames(); !reflect.DeepEqual(got, []string{"deploy:1", "deploy:2"}) {
			t.Fatalf("commands = %#v", got)
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

	t.Run("ignores context file candidates that are directories", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		if err := os.Mkdir(filepath.Join(cwd, "AGENTS.md"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeResourceFile(t, filepath.Join(cwd, "CLAUDE.md"), "fallback instructions")

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		files := loader.GetAgentsFiles().AgentsFiles
		if len(files) != 1 {
			t.Fatalf("context files = %#v, want one regular file", files)
		}
		if filepath.Base(files[0].Path) != "CLAUDE.md" || files[0].Content != "fallback instructions" {
			t.Fatalf("context file = %#v, want CLAUDE.md fallback", files[0])
		}
	})

	t.Run("discovers global and ancestor context files with content", func(t *testing.T) {
		root := t.TempDir()
		agentDir := filepath.Join(root, "agent")
		repo := filepath.Join(root, "repo")
		cwd := filepath.Join(repo, "sub", "leaf")
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatal(err)
		}
		writeResourceFile(t, filepath.Join(agentDir, "CLAUDE.MD"), "global instructions")
		writeResourceFile(t, filepath.Join(repo, "AGENTS.md"), "repo instructions")
		writeResourceFile(t, filepath.Join(cwd, "AGENTS.md"), "leaf instructions")

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})
		loader.Reload()

		files := loader.GetAgentsFiles().AgentsFiles
		if len(files) != 3 {
			t.Fatalf("context files = %#v", files)
		}
		if !strings.EqualFold(filepath.Base(files[0].Path), "CLAUDE.md") || files[0].Content != "global instructions" ||
			!strings.EqualFold(filepath.Base(files[1].Path), "AGENTS.md") || files[1].Content != "repo instructions" ||
			!strings.EqualFold(filepath.Base(files[2].Path), "AGENTS.md") || files[2].Content != "leaf instructions" {
			t.Fatalf("context file order/content = %#v", files)
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

	t.Run("resource disable flags still allow explicit CLI resources", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		writeGiProtocolExtensionDescriptor(t, filepath.Join(agentDir, "extensions", "default.gi.json"))
		writeResourceFile(t, filepath.Join(agentDir, "prompts", "default.md"), "Default prompt")
		writeJSON(t, filepath.Join(agentDir, "themes", "default.json"), map[string]any{"name": "default-theme"})

		explicitExt := filepath.Join(filepath.Dir(agentDir), "explicit", "explicit.gi.json")
		explicitPrompt := filepath.Join(filepath.Dir(agentDir), "explicit", "prompt.md")
		explicitTheme := filepath.Join(filepath.Dir(agentDir), "explicit", "theme.json")
		writeGiProtocolExtensionDescriptor(t, explicitExt)
		writeResourceFile(t, explicitPrompt, "Explicit prompt")
		writeJSON(t, explicitTheme, map[string]any{"name": "explicit-theme"})

		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{
			CWD:                      cwd,
			AgentDir:                 agentDir,
			NoExtensions:             true,
			NoPromptTemplates:        true,
			NoThemes:                 true,
			AdditionalExtensionPaths: []string{explicitExt},
			AdditionalPromptPaths:    []string{explicitPrompt},
			AdditionalThemePaths:     []string{explicitTheme},
		})
		loader.Reload()

		if !protocolExtensionHasSuffix(loader.GetExtensions().Extensions, "explicit.gi.json") ||
			protocolExtensionHasSuffix(loader.GetExtensions().Extensions, "default.gi.json") {
			t.Fatalf("extensions = %#v", loader.GetExtensions().Extensions)
		}
		if !resourceHasPrompt(loader.GetPrompts().Prompts, "prompt") || resourceHasPrompt(loader.GetPrompts().Prompts, "default") {
			t.Fatalf("prompts = %#v", loader.GetPrompts().Prompts)
		}
		if resourceFindTheme(loader.GetThemes().Themes, "explicit-theme") == nil || resourceFindTheme(loader.GetThemes().Themes, "default-theme") != nil {
			t.Fatalf("themes = %#v", loader.GetThemes().Themes)
		}
	})

	t.Run("missing explicit theme path reports Pi-style diagnostic", func(t *testing.T) {
		agentDir, cwd := createResourceLoaderDirs(t)
		missingTheme := filepath.Join(cwd, "light")
		loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{
			CWD:                  cwd,
			AgentDir:             agentDir,
			AdditionalThemePaths: []string{"light"},
		})
		loader.Reload()

		themes := loader.GetThemes()
		if len(themes.Diagnostics) != 1 ||
			themes.Diagnostics[0].Type != "warning" ||
			themes.Diagnostics[0].Message != "theme path does not exist" ||
			themes.Diagnostics[0].Path != missingTheme {
			t.Fatalf("theme diagnostics = %#v", themes.Diagnostics)
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

func TestDefaultResourceLoaderMapsPackageSkillDirectoriesPiStyle(t *testing.T) {
	agentDir, cwd := createResourceLoaderDirs(t)
	skillDir := filepath.Join(t.TempDir(), "package-skill")
	skillFile := filepath.Join(skillDir, "SKILL.md")
	writeResourceSkill(t, skillFile, "package-skill", "Package skill", "Package content")
	loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir})

	resource := ProtocolPackageResource{
		Path:    skillDir,
		Enabled: true,
		Metadata: ProtocolSourceInfo{
			Path:   skillDir,
			Source: "local:package",
			Scope:  "user",
			Origin: "package",
		},
	}
	mapped := loader.mapSkillPath(resource)
	if mapped.Path != skillFile || mapped.Metadata.Path != skillFile {
		t.Fatalf("mapped package skill = %#v", mapped)
	}

	loader.packageResources.Skills = []ProtocolPackageResource{resource}
	loaded := loader.loadSkills()
	skill := resourceFindSkill(loaded.Skills, "package-skill")
	if skill == nil || skill.FilePath != skillFile {
		t.Fatalf("loaded package skills = %#v", loaded.Skills)
	}
	source, _ := skill.SourceInfo.(ProtocolSourceInfo)
	if source.Path != skillFile || source.Source != "local:package" || source.Origin != "package" {
		t.Fatalf("loaded package skill source = %#v", source)
	}

	explicit := resource
	explicit.Metadata = ProtocolSourceInfo{Path: skillDir, Source: "extension:explicit", Scope: "temporary", Origin: "top-level"}
	if got := loader.mapSkillPath(explicit); got.Path != skillDir || got.Metadata.Path != skillDir {
		t.Fatalf("explicit skill path should remain declared = %#v", got)
	}
	missing := resource
	missing.Path = filepath.Join(t.TempDir(), "missing")
	missing.Metadata.Path = missing.Path
	if got := loader.mapSkillPath(missing); got.Path != missing.Path || got.Metadata.Path != missing.Path {
		t.Fatalf("missing skill path should remain declared = %#v", got)
	}
}

func TestDefaultResourceLoaderGatesProjectResourcesByTrust(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, "agent")
	cwd := filepath.Join(root, "repo", "subdir")
	projectDir := filepath.Join(cwd, ConfigDirName)
	projectExtension := filepath.Join(projectDir, "extensions", "project.gi.json")
	projectSkill := filepath.Join(projectDir, "skills", "project-skill", "SKILL.md")
	ancestorSkill := filepath.Join(root, "repo", ".agents", "skills", "ancestor-skill", "SKILL.md")
	projectPrompt := filepath.Join(projectDir, "prompts", "project.md")
	projectTheme := filepath.Join(projectDir, "themes", "project.json")
	explicitSkill := filepath.Join(cwd, "explicit", "SKILL.md")
	writeGiProtocolExtensionDescriptor(t, projectExtension)
	writeResourceSkill(t, projectSkill, "project-skill", "Project skill", "Project content")
	writeResourceSkill(t, ancestorSkill, "ancestor-skill", "Ancestor skill", "Ancestor content")
	writeResourceSkill(t, explicitSkill, "explicit-skill", "Explicit skill", "Explicit content")
	writeResourceFile(t, projectPrompt, "Project prompt")
	writeJSON(t, projectTheme, map[string]any{"name": "project-theme"})
	writeResourceFile(t, filepath.Join(projectDir, "SYSTEM.md"), "Project system prompt")
	writeResourceFile(t, filepath.Join(projectDir, "APPEND_SYSTEM.md"), "Project append prompt")
	writeResourceFile(t, filepath.Join(cwd, "AGENTS.md"), "Project context")
	writeResourceSkill(t, filepath.Join(agentDir, "skills", "user-skill", "SKILL.md"), "user-skill", "User skill", "User content")
	writeResourceFile(t, filepath.Join(agentDir, "prompts", "user.md"), "User prompt")
	writeJSON(t, filepath.Join(agentDir, "themes", "user.json"), map[string]any{"name": "user-theme"})
	writeResourceFile(t, filepath.Join(agentDir, "SYSTEM.md"), "User system prompt")
	writeResourceFile(t, filepath.Join(agentDir, "APPEND_SYSTEM.md"), "User append prompt")

	settings := NewSettingsManagerWithOptions(cwd, agentDir, SettingsManagerOptions{ProjectTrusted: false})
	loader := NewDefaultResourceLoader(DefaultResourceLoaderOptions{
		CWD:                  cwd,
		AgentDir:             agentDir,
		SettingsManager:      settings,
		AdditionalSkillPaths: []string{explicitSkill},
	})
	loader.Reload()

	if len(loader.GetExtensions().Extensions) != 0 || len(loader.GetExtensions().Errors) != 0 {
		t.Fatalf("untrusted extensions = %#v, errors = %#v", loader.GetExtensions().Extensions, loader.GetExtensions().Errors)
	}
	for _, name := range []string{"project-skill", "ancestor-skill"} {
		if resourceHasSkill(loader.GetSkills().Skills, name) {
			t.Fatalf("untrusted skill %q loaded: %#v", name, loader.GetSkills().Skills)
		}
	}
	for _, name := range []string{"user-skill", "explicit-skill"} {
		if !resourceHasSkill(loader.GetSkills().Skills, name) {
			t.Fatalf("trusted skill %q missing: %#v", name, loader.GetSkills().Skills)
		}
	}
	if resourceHasPrompt(loader.GetPrompts().Prompts, "project") || !resourceHasPrompt(loader.GetPrompts().Prompts, "user") {
		t.Fatalf("untrusted prompts = %#v", loader.GetPrompts().Prompts)
	}
	if resourceFindTheme(loader.GetThemes().Themes, "project-theme") != nil ||
		resourceFindTheme(loader.GetThemes().Themes, "user-theme") == nil {
		t.Fatalf("untrusted themes = %#v", loader.GetThemes().Themes)
	}
	if loader.GetSystemPrompt() != "User system prompt" || loader.GetAppendSystemPrompt() != "User append prompt" {
		t.Fatalf("untrusted prompts = system %q append %q", loader.GetSystemPrompt(), loader.GetAppendSystemPrompt())
	}
	if files := loader.GetAgentsFiles().AgentsFiles; !resourceContextFilesContain(files, filepath.Join(cwd, "AGENTS.md")) {
		t.Fatalf("project context should remain available: %#v", files)
	}

	settings.SetProjectTrusted(true)
	loader.Reload()
	if len(loader.GetExtensions().Extensions) != 1 || loader.GetExtensions().Extensions[0].Path != projectExtension {
		t.Fatalf("trusted extensions = %#v", loader.GetExtensions().Extensions)
	}
	for _, name := range []string{"project-skill", "ancestor-skill"} {
		if !resourceHasSkill(loader.GetSkills().Skills, name) {
			t.Fatalf("trusted skill %q missing: %#v", name, loader.GetSkills().Skills)
		}
	}
	if !resourceHasPrompt(loader.GetPrompts().Prompts, "project") ||
		resourceFindTheme(loader.GetThemes().Themes, "project-theme") == nil {
		t.Fatalf("trusted resources: prompts=%#v themes=%#v", loader.GetPrompts().Prompts, loader.GetThemes().Themes)
	}
	if loader.GetSystemPrompt() != "Project system prompt" || loader.GetAppendSystemPrompt() != "Project append prompt" {
		t.Fatalf("trusted prompts = system %q append %q", loader.GetSystemPrompt(), loader.GetAppendSystemPrompt())
	}
}

func resourceContextFilesContain(files []ResourceContextFile, path string) bool {
	for _, file := range files {
		if file.Path == path {
			return true
		}
	}
	return false
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

func skillDiagnosticsMention(diagnostics []agentharness.SkillDiagnostic, path string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Type == "collision" && (diagnostic.Path == path || strings.Contains(diagnostic.Message, path)) {
			return true
		}
	}
	return false
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

func resourceExtensionErrorsContain(errors []ProtocolExtensionDiscoveryError, text string) bool {
	for _, err := range errors {
		if strings.Contains(err.Error, text) {
			return true
		}
	}
	return false
}

func resourceHasCustomEntry(entries []FileEntry, customType string) bool {
	for _, entry := range entries {
		if entry.CustomType == customType {
			return true
		}
	}
	return false
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
