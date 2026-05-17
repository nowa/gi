package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadSkillsLoadsSkillFiles(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, ".agents", "skills", "example", "SKILL.md")
	mustWrite(t, skillPath, `---
name: example
description: Example skill
disable-model-invocation: true
---
Use this skill.
`)

	result := LoadSkills(filepath.Join(root, ".agents", "skills"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	want := []Skill{{
		Name:                   "example",
		Description:            "Example skill",
		Content:                "Use this skill.",
		FilePath:               skillPath,
		DisableModelInvocation: true,
	}}
	if !reflect.DeepEqual(result.Skills, want) {
		t.Fatalf("skills = %#v, want %#v", result.Skills, want)
	}
}

func TestLoadSkillsThroughSymlinkedDirectories(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "actual", "example", "SKILL.md"), "---\nname: example\ndescription: Example skill\n---\nUse this skill.")
	if err := os.Symlink(filepath.Join(root, "actual"), filepath.Join(root, "skills-link")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	result := LoadSkills(filepath.Join(root, "skills-link"))
	if len(result.Skills) != 1 || result.Skills[0].Name != "example" {
		t.Fatalf("skills = %#v", result.Skills)
	}
	if result.Skills[0].FilePath != filepath.Join(root, "skills-link", "example", "SKILL.md") {
		t.Fatalf("filePath = %s", result.Skills[0].FilePath)
	}
}

func TestLoadSourcedSkillsPreservesSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "user")
	mustWrite(t, filepath.Join(path, "example", "SKILL.md"), "---\nname: example\ndescription: Example skill\n---\nUse this skill.")
	source := map[string]string{"type": "user"}

	result := LoadSourcedSkills(map[string]any{path: source})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if len(result.Skills) != 1 || !reflect.DeepEqual(result.Skills[0].Source, source) {
		t.Fatalf("sourced skills = %#v", result.Skills)
	}
	if result.Skills[0].Skill.Name != "example" || result.Skills[0].Skill.FilePath != filepath.Join(path, "example", "SKILL.md") {
		t.Fatalf("skill = %#v", result.Skills[0].Skill)
	}
}

func TestLoadSourcedSkillsAttachesSourceToDiagnostics(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "user")
	mustWrite(t, filepath.Join(path, "broken", "SKILL.md"), "---\nname: broken\n---\nMissing description.")
	source := map[string]string{"type": "user"}

	result := LoadSourcedSkills(map[string]any{path: source})
	if len(result.Skills) != 0 || len(result.Diagnostics) != 1 {
		t.Fatalf("result = %#v", result)
	}
	diagnostic := result.Diagnostics[0]
	if diagnostic.Code != "invalid_metadata" || diagnostic.Message != "description is required" || !reflect.DeepEqual(diagnostic.Source, source) {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestLoadSkillsLoadsDirectMarkdownChildrenOnlyFromRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "skills")
	mustWrite(t, filepath.Join(path, "root.md"), "---\ndescription: Root skill\n---\nRoot content")
	mustWrite(t, filepath.Join(path, "nested", "ignored.md"), "---\ndescription: Ignored\n---\nIgnored content")

	result := LoadSkills(path)
	if len(result.Skills) != 1 {
		t.Fatalf("skills = %#v", result.Skills)
	}
	if result.Skills[0].Name != "skills" || result.Skills[0].Content != "Root content" {
		t.Fatalf("skill = %#v", result.Skills[0])
	}
}
