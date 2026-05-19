package gicodingagent

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const ConfigDirName = ".pi"

type SourceInfo struct {
	Path   string
	Source string
	Scope  string
	Origin string
}

type PromptTemplate struct {
	Name         string
	Description  string
	ArgumentHint string
	Content      string
	SourceInfo   SourceInfo
	FilePath     string
}

type LoadPromptTemplatesOptions struct {
	Cwd             string
	AgentDir        string
	PromptPaths     []string
	IncludeDefaults bool
}

func ParseCommandArgs(argsString string) []string {
	var args []string
	var current strings.Builder
	var quote rune

	for _, r := range argsString {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
		case unicode.IsSpace(r):
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

var promptTemplateSlicePattern = regexp.MustCompile(`\$\{@:(\d+)(?::(\d+))?\}`)
var promptTemplatePositionalPattern = regexp.MustCompile(`\$(\d+)`)

func SubstituteArgs(content string, args []string) string {
	result := promptTemplatePositionalPattern.ReplaceAllStringFunc(content, func(match string) string {
		num, err := strconv.Atoi(strings.TrimPrefix(match, "$"))
		if err != nil || num <= 0 || num > len(args) {
			return ""
		}
		return args[num-1]
	})

	result = promptTemplateSlicePattern.ReplaceAllStringFunc(result, func(match string) string {
		parts := promptTemplateSlicePattern.FindStringSubmatch(match)
		if len(parts) == 0 {
			return match
		}
		start, _ := strconv.Atoi(parts[1])
		start--
		if start < 0 {
			start = 0
		}
		if start >= len(args) {
			return ""
		}
		end := len(args)
		if parts[2] != "" {
			length, _ := strconv.Atoi(parts[2])
			end = start + length
			if end > len(args) {
				end = len(args)
			}
		}
		return strings.Join(args[start:end], " ")
	})

	allArgs := strings.Join(args, " ")
	result = strings.ReplaceAll(result, "$ARGUMENTS", allArgs)
	result = strings.ReplaceAll(result, "$@", allArgs)
	return result
}

func ExpandPromptTemplate(text string, templates []PromptTemplate) string {
	if !strings.HasPrefix(text, "/") {
		return text
	}
	withoutSlash := strings.TrimPrefix(text, "/")
	if withoutSlash == "" {
		return text
	}

	command := withoutSlash
	argsString := ""
	for i, r := range withoutSlash {
		if unicode.IsSpace(r) {
			command = withoutSlash[:i]
			argsString = withoutSlash[i+len(string(r)):]
			break
		}
	}
	if command == "" {
		return text
	}
	for _, template := range templates {
		if template.Name == command {
			return SubstituteArgs(template.Content, ParseCommandArgs(argsString))
		}
	}
	return text
}

func LoadPromptTemplates(options LoadPromptTemplatesOptions) []PromptTemplate {
	cwd := options.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	var templates []PromptTemplate
	globalPromptsDir := filepath.Join(options.AgentDir, "prompts")
	projectPromptsDir := filepath.Join(cwd, ConfigDirName, "prompts")

	if options.IncludeDefaults {
		templates = append(templates, loadPromptTemplatesFromDir(globalPromptsDir, sourceInfoForPromptPath(globalPromptsDir, globalPromptsDir, projectPromptsDir))...)
		templates = append(templates, loadPromptTemplatesFromDir(projectPromptsDir, sourceInfoForPromptPath(projectPromptsDir, globalPromptsDir, projectPromptsDir))...)
	}

	for _, rawPath := range options.PromptPaths {
		resolvedPath := resolvePromptTemplatePath(rawPath, cwd)
		info, err := os.Stat(resolvedPath)
		if err != nil {
			continue
		}
		getSourceInfo := sourceInfoForPromptPath(resolvedPath, globalPromptsDir, projectPromptsDir)
		if info.IsDir() {
			templates = append(templates, loadPromptTemplatesFromDir(resolvedPath, getSourceInfo)...)
			continue
		}
		if info.Mode().IsRegular() && strings.HasSuffix(resolvedPath, ".md") {
			if template, ok := loadPromptTemplateFromFile(resolvedPath, getSourceInfo(resolvedPath)); ok {
				templates = append(templates, template)
			}
		}
	}
	return templates
}

func loadPromptTemplatesFromDir(dir string, getSourceInfo func(string) SourceInfo) []PromptTemplate {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	templates := make([]PromptTemplate, 0, len(entries))
	for _, entry := range entries {
		fullPath := filepath.Join(dir, entry.Name())
		info, err := os.Stat(fullPath)
		if err != nil || !info.Mode().IsRegular() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		template, ok := loadPromptTemplateFromFile(fullPath, getSourceInfo(fullPath))
		if ok {
			templates = append(templates, template)
		}
	}
	return templates
}

func loadPromptTemplateFromFile(filePath string, sourceInfo SourceInfo) (PromptTemplate, bool) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return PromptTemplate{}, false
	}
	parsed, err := ParseFrontmatter(string(content))
	if err != nil {
		return PromptTemplate{}, false
	}

	name := strings.TrimSuffix(filepath.Base(filePath), ".md")
	description := parsed.Frontmatter["description"]
	if description == "" {
		for _, line := range strings.Split(parsed.Body, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if len(line) > 60 {
				description = line[:60] + "..."
			} else {
				description = line
			}
			break
		}
	}

	return PromptTemplate{
		Name:         name,
		Description:  description,
		ArgumentHint: parsed.Frontmatter["argument-hint"],
		Content:      parsed.Body,
		SourceInfo:   sourceInfo,
		FilePath:     filePath,
	}, true
}

func resolvePromptTemplatePath(path, cwd string) string {
	expanded := ExpandPath(strings.TrimSpace(path))
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded)
	}
	return filepath.Clean(filepath.Join(cwd, expanded))
}

func sourceInfoForPromptPath(basePath, globalPromptsDir, projectPromptsDir string) func(string) SourceInfo {
	return func(path string) SourceInfo {
		resolved := filepath.Clean(path)
		switch {
		case isUnderPath(resolved, globalPromptsDir):
			return SourceInfo{Path: resolved, Source: "local", Scope: "user", Origin: "top-level"}
		case isUnderPath(resolved, projectPromptsDir):
			return SourceInfo{Path: resolved, Source: "local", Scope: "project", Origin: "top-level"}
		default:
			return SourceInfo{Path: resolved, Source: "local", Scope: "temporary", Origin: "top-level"}
		}
	}
}

func isUnderPath(path, root string) bool {
	if root == "" {
		return false
	}
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}
