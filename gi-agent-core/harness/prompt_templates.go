package harness

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type PromptTemplateDiagnostic struct {
	Type    string
	Code    string
	Message string
	Path    string
	Source  any
}

type PromptTemplateResult struct {
	PromptTemplates []PromptTemplate
	Diagnostics     []PromptTemplateDiagnostic
}

type SourcedPromptTemplate struct {
	PromptTemplate PromptTemplate
	Source         any
}

type SourcedPromptTemplateResult struct {
	PromptTemplates []SourcedPromptTemplate
	Diagnostics     []PromptTemplateDiagnostic
}

func LoadPromptTemplates(paths ...string) PromptTemplateResult {
	var result PromptTemplateResult
	for _, inputPath := range paths {
		absPath, _ := filepath.Abs(inputPath)
		info, err := os.Stat(absPath)
		if err != nil {
			if !os.IsNotExist(err) {
				result.Diagnostics = append(result.Diagnostics, PromptTemplateDiagnostic{Type: "warning", Code: "file_info_failed", Message: err.Error(), Path: absPath})
			}
			continue
		}
		if info.IsDir() {
			childResult := loadTemplatesFromDir(absPath)
			result.PromptTemplates = append(result.PromptTemplates, childResult.PromptTemplates...)
			result.Diagnostics = append(result.Diagnostics, childResult.Diagnostics...)
		} else if strings.EqualFold(filepath.Ext(absPath), ".md") {
			template, diagnostics := loadTemplateFromFile(absPath)
			if template != nil {
				result.PromptTemplates = append(result.PromptTemplates, *template)
			}
			result.Diagnostics = append(result.Diagnostics, diagnostics...)
		}
	}
	return result
}

func LoadSourcedPromptTemplates(inputs map[string]any) SourcedPromptTemplateResult {
	var result SourcedPromptTemplateResult
	paths := make([]string, 0, len(inputs))
	for path := range inputs {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		source := inputs[path]
		loaded := LoadPromptTemplates(path)
		for _, template := range loaded.PromptTemplates {
			result.PromptTemplates = append(result.PromptTemplates, SourcedPromptTemplate{PromptTemplate: template, Source: source})
		}
		for _, diagnostic := range loaded.Diagnostics {
			diagnostic.Source = source
			result.Diagnostics = append(result.Diagnostics, diagnostic)
		}
	}
	return result
}

func loadTemplatesFromDir(dir string) PromptTemplateResult {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return PromptTemplateResult{Diagnostics: []PromptTemplateDiagnostic{{Type: "warning", Code: "list_failed", Message: err.Error(), Path: dir}}}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var result PromptTemplateResult
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		fullPath := filepath.Join(dir, entry.Name())
		template, diagnostics := loadTemplateFromFile(fullPath)
		if template != nil {
			result.PromptTemplates = append(result.PromptTemplates, *template)
		}
		result.Diagnostics = append(result.Diagnostics, diagnostics...)
	}
	return result
}

func loadTemplateFromFile(path string) (*PromptTemplate, []PromptTemplateDiagnostic) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, []PromptTemplateDiagnostic{{Type: "warning", Code: "read_failed", Message: err.Error(), Path: path}}
	}
	frontmatter, body, err := parseFrontmatter(string(content))
	if err != nil {
		return nil, []PromptTemplateDiagnostic{{Type: "warning", Code: "parse_failed", Message: err.Error(), Path: path}}
	}
	description := frontmatter["description"]
	if description == "" {
		for _, line := range strings.Split(body, "\n") {
			if strings.TrimSpace(line) != "" {
				description = line
				if len([]rune(description)) > 60 {
					description = string([]rune(description)[:60]) + "..."
				}
				break
			}
		}
	}
	return &PromptTemplate{
		Name:        strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Description: description,
		Content:     body,
	}, nil
}
