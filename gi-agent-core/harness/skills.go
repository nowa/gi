package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type SkillDiagnostic struct {
	Type    string
	Code    string
	Message string
	Path    string
	Source  any
}

type SkillResult struct {
	Skills      []Skill
	Diagnostics []SkillDiagnostic
}

type SourcedSkill struct {
	Skill  Skill
	Source any
}

type SourcedSkillResult struct {
	Skills      []SourcedSkill
	Diagnostics []SkillDiagnostic
}

func LoadSkills(paths ...string) SkillResult {
	var result SkillResult
	for _, inputPath := range paths {
		absPath, _ := filepath.Abs(inputPath)
		info, err := os.Stat(absPath)
		if err != nil {
			if !os.IsNotExist(err) {
				result.Diagnostics = append(result.Diagnostics, SkillDiagnostic{Type: "warning", Code: "file_info_failed", Message: err.Error(), Path: absPath})
			}
			continue
		}
		if !info.IsDir() {
			continue
		}
		loaded := loadSkillsFromDir(absPath, true)
		result.Skills = append(result.Skills, loaded.Skills...)
		result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
	}
	return result
}

func LoadSourcedSkills(inputs map[string]any) SourcedSkillResult {
	var result SourcedSkillResult
	paths := make([]string, 0, len(inputs))
	for path := range inputs {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		source := inputs[path]
		loaded := LoadSkills(path)
		for _, skill := range loaded.Skills {
			result.Skills = append(result.Skills, SourcedSkill{Skill: skill, Source: source})
		}
		for _, diagnostic := range loaded.Diagnostics {
			diagnostic.Source = source
			result.Diagnostics = append(result.Diagnostics, diagnostic)
		}
	}
	return result
}

func loadSkillsFromDir(dir string, includeRootFiles bool) SkillResult {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return SkillResult{Diagnostics: []SkillDiagnostic{{Type: "warning", Code: "list_failed", Message: err.Error(), Path: dir}}}
	}
	for _, entry := range entries {
		if entry.Name() == "SKILL.md" {
			fullPath := filepath.Join(dir, entry.Name())
			skill, diagnostics := loadSkillFromFile(fullPath)
			result := SkillResult{Diagnostics: diagnostics}
			if skill != nil {
				result.Skills = append(result.Skills, *skill)
			}
			return result
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var result SkillResult
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}
		fullPath := filepath.Join(dir, name)
		info, err := os.Stat(fullPath)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, SkillDiagnostic{Type: "warning", Code: "file_info_failed", Message: err.Error(), Path: fullPath})
			continue
		}
		if info.IsDir() {
			loaded := loadSkillsFromDir(fullPath, false)
			result.Skills = append(result.Skills, loaded.Skills...)
			result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
			continue
		}
		if includeRootFiles && strings.EqualFold(filepath.Ext(name), ".md") {
			skill, diagnostics := loadSkillFromFile(fullPath)
			if skill != nil {
				result.Skills = append(result.Skills, *skill)
			}
			result.Diagnostics = append(result.Diagnostics, diagnostics...)
		}
	}
	return result
}

func loadSkillFromFile(path string) (*Skill, []SkillDiagnostic) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, []SkillDiagnostic{{Type: "warning", Code: "read_failed", Message: err.Error(), Path: path}}
	}
	frontmatter, body, err := parseFrontmatter(string(content))
	if err != nil {
		return nil, []SkillDiagnostic{{Type: "warning", Code: "parse_failed", Message: err.Error(), Path: path}}
	}
	dirName := filepath.Base(filepath.Dir(path))
	name := frontmatter["name"]
	if name == "" {
		name = dirName
	}
	description := frontmatter["description"]
	var diagnostics []SkillDiagnostic
	for _, message := range validateSkillDescription(description) {
		diagnostics = append(diagnostics, SkillDiagnostic{Type: "warning", Code: "invalid_metadata", Message: message, Path: path})
	}
	for _, message := range validateSkillName(name, dirName) {
		diagnostics = append(diagnostics, SkillDiagnostic{Type: "warning", Code: "invalid_metadata", Message: message, Path: path})
	}
	if strings.TrimSpace(description) == "" {
		return nil, diagnostics
	}
	return &Skill{
		Name:                   name,
		Description:            description,
		Content:                body,
		FilePath:               path,
		DisableModelInvocation: frontmatterBool(frontmatter, "disable-model-invocation"),
	}, diagnostics
}

func validateSkillDescription(description string) []string {
	if strings.TrimSpace(description) == "" {
		return []string{"description is required"}
	}
	if len([]rune(description)) > 1024 {
		return []string{fmt.Sprintf("description exceeds 1024 characters (%d)", len([]rune(description)))}
	}
	return nil
}

func validateSkillName(name, parentDir string) []string {
	var errors []string
	if name != parentDir {
		errors = append(errors, fmt.Sprintf("name %q does not match parent directory %q", name, parentDir))
	}
	if len([]rune(name)) > 64 {
		errors = append(errors, fmt.Sprintf("name exceeds 64 characters (%d)", len([]rune(name))))
	}
	validName := regexp.MustCompile(`^[a-z0-9-]+$`)
	if !validName.MatchString(name) {
		errors = append(errors, "name contains invalid characters (must be lowercase a-z, 0-9, hyphens only)")
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		errors = append(errors, "name must not start or end with a hyphen")
	}
	if strings.Contains(name, "--") {
		errors = append(errors, "name must not contain consecutive hyphens")
	}
	return errors
}
