package gicodingagent

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestPromptTemplateSubstituteArgsPiMatrix(t *testing.T) {
	longArgs := make([]string, 15)
	for i := range longArgs {
		longArgs[i] = "val" + strconv.Itoa(i)
	}

	tests := []struct {
		name     string
		template string
		args     []string
		want     string
	}{
		{"arguments", "Test: $ARGUMENTS", []string{"a", "b", "c"}, "Test: a b c"},
		{"at", "Test: $@", []string{"a", "b", "c"}, "Test: a b c"},
		{"no recursive substitution", "$ARGUMENTS", []string{"$1", "$ARGUMENTS"}, "$1 $ARGUMENTS"},
		{"mixed positional and arguments", "$1: $ARGUMENTS", []string{"prefix", "a", "b"}, "prefix: prefix a b"},
		{"empty positional", "Test: $1", nil, "Test: "},
		{"multiple occurrences", "$@ and $ARGUMENTS", []string{"a", "b"}, "a b and a b"},
		{"out of range", "$1 $2 $3 $4 $5", []string{"a", "b"}, "a b   "},
		{"unicode", "$ARGUMENTS", []string{"日本語", "🎉", "café"}, "日本語 🎉 café"},
		{"preserve newline and tab args", "$1 $2", []string{"line1\nline2", "tab\tthere"}, "line1\nline2 tab\tthere"},
		{"consecutive", "$1$2", []string{"a", "b"}, "ab"},
		{"zero index", "$0", []string{"a", "b"}, ""},
		{"decimal", "$1.5", []string{"a"}, "a.5"},
		{"arguments inside word", "pre$ARGUMENTS", []string{"a", "b"}, "prea b"},
		{"at inside word", "pre$@", []string{"a", "b"}, "prea b"},
		{"empty middle arg", "$ARGUMENTS", []string{"a", "", "c"}, "a  c"},
		{"literal non matches", "$A $$ $ $ARGS", []string{"a"}, "$A $$ $ $ARGS"},
		{"case sensitive", "$arguments $Arguments $ARGUMENTS", []string{"a", "b"}, "$arguments $Arguments a b"},
		{"multiple digits", "$10 $12 $15", longArgs, "val9 val11 val14"},
		{"escaped dollar is still literal backslash", `Price: \$100`, nil, `Price: \`},
		{"slice from index", "${@:2}", []string{"a", "b", "c", "d"}, "b c d"},
		{"slice length", "${@:2:2}", []string{"a", "b", "c", "d"}, "b c"},
		{"slice out of range", "${@:99}", []string{"a", "b"}, ""},
		{"zero length slice", "${@:2:0}", []string{"a", "b", "c"}, ""},
		{"slice before simple at", "${@:2} vs $@", []string{"a", "b", "c"}, "b c vs a b c"},
		{"slice no recursive substitution", "${@:1}", []string{"${@:2}", "test"}, "${@:2} test"},
		{"slice zero means all", "${@:0}", []string{"a", "b", "c"}, "a b c"},
		{"slice middle", "Process ${@:2} with $1", []string{"tool", "file1", "file2"}, "Process file1 file2 with tool"},
		{"multiple slices", "${@:1:2} vs ${@:3:2}", []string{"a", "b", "c", "d", "e"}, "a b vs c d"},
		{"slice no spacing", "prefix${@:2}suffix", []string{"a", "b", "c"}, "prefixb csuffix"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SubstituteArgs(tc.template, tc.args); got != tc.want {
				t.Fatalf("SubstituteArgs(%q, %#v) = %q, want %q", tc.template, tc.args, got, tc.want)
			}
		})
	}
}

func TestPromptTemplateParseCommandArgsPiMatrix(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"a b c", []string{"a", "b", "c"}},
		{`"first arg" second`, []string{"first arg", "second"}},
		{"'first arg' second", []string{"first arg", "second"}},
		{`"double" 'single' "double again"`, []string{"double", "single", "double again"}},
		{"", nil},
		{"a  b   c", []string{"a", "b", "c"}},
		{"a\tb\tc", []string{"a", "b", "c"}},
		{`"" " "`, []string{" "}},
		{"$100 @user #tag", []string{"$100", "@user", "#tag"}},
		{"日本語 🎉 café", []string{"日本語", "🎉", "café"}},
		{"\"line1\nline2\" second", []string{"line1\nline2", "second"}},
		{"label-2\n\nHere is some description #2.", []string{"label-2", "Here", "is", "some", "description", "#2."}},
		{"a\n\n\tb  c", []string{"a", "b", "c"}},
		{`"quoted \"text\""`, []string{`quoted \text\`}},
		{"   a b c   ", []string{"a", "b", "c"}},
	}

	for _, tc := range tests {
		t.Run(strings.ReplaceAll(tc.input, "\n", "\\n"), func(t *testing.T) {
			if got := ParseCommandArgs(tc.input); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ParseCommandArgs(%q) = %#v, want %#v", tc.input, got, tc.want)
			}
		})
	}
}

func TestPromptTemplateExpandPiNewlineArguments(t *testing.T) {
	templates := []PromptTemplate{{
		Name:        "arg-test",
		Description: "test",
		Content:     "- arg1: $1\n- rest: ${@:2}",
		SourceInfo:  SourceInfo{Path: "/tmp/arg-test.md", Source: "local", Scope: "temporary", Origin: "top-level"},
		FilePath:    "/tmp/arg-test.md",
	}}

	got := ExpandPromptTemplate("/arg-test label-2\n\nHere is some description #2.", templates)
	want := "- arg1: label-2\n- rest: Here is some description #2."
	if got != want {
		t.Fatalf("ExpandPromptTemplate newline args = %q, want %q", got, want)
	}

	got = ExpandPromptTemplate("/arg-test\nlabel-2", templates)
	if want := "- arg1: label-2\n- rest: "; got != want {
		t.Fatalf("ExpandPromptTemplate command newline = %q, want %q", got, want)
	}

	if got := ExpandPromptTemplate("plain text", templates); got != "plain text" {
		t.Fatalf("non-template text changed to %q", got)
	}
	if got := ExpandPromptTemplate("/missing value", templates); got != "/missing value" {
		t.Fatalf("missing template changed to %q", got)
	}
}

func TestPromptTemplateLoadArgumentHintsAndDefaults(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "project")
	agentDir := filepath.Join(root, "agent")
	explicitDir := filepath.Join(root, "explicit")
	projectDir := filepath.Join(cwd, ConfigDirName, "prompts")
	userDir := filepath.Join(agentDir, "prompts")
	for _, dir := range []string{explicitDir, projectDir, userDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	writeTemplate := func(dir, name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o600); err != nil {
			t.Fatalf("write template %s/%s: %v", dir, name, err)
		}
	}
	writeTemplate(explicitDir, "pr", `---
description: Review PRs from URLs with structured issue and code analysis
argument-hint: "<PR-URL>"
---
You are given one or more GitHub PR URLs: $@`)
	writeTemplate(explicitDir, "wr", `---
description: Finish the current task end-to-end with changelog, commit, and push
argument-hint: "[instructions]"
---
Wrap it. Additional instructions: $ARGUMENTS`)
	writeTemplate(explicitDir, "cl", `---
description: Audit changelog entries before release
---
Audit changelog entries for all commits since the last release.`)
	writeTemplate(explicitDir, "empty-hint", `---
description: A command with empty hint
argument-hint: ""
---
Do something`)
	writeTemplate(explicitDir, "fallback", "First non-empty line is used as description when frontmatter is missing.")
	writeTemplate(projectDir, "project", "---\ndescription: Project prompt\n---\nProject body")
	writeTemplate(userDir, "user", "---\ndescription: User prompt\n---\nUser body")
	if err := os.WriteFile(filepath.Join(explicitDir, "skip.txt"), []byte("nope"), 0o600); err != nil {
		t.Fatalf("write skip file: %v", err)
	}

	templates := LoadPromptTemplates(LoadPromptTemplatesOptions{
		Cwd:             cwd,
		AgentDir:        agentDir,
		PromptPaths:     []string{explicitDir, filepath.Join(explicitDir, "pr.md"), filepath.Join(explicitDir, "missing")},
		IncludeDefaults: true,
	})

	byName := map[string]PromptTemplate{}
	for _, template := range templates {
		byName[template.Name] = template
	}
	if got := byName["pr"].ArgumentHint; got != "<PR-URL>" {
		t.Fatalf("pr argument hint = %q", got)
	}
	if got := byName["wr"].ArgumentHint; got != "[instructions]" {
		t.Fatalf("wr argument hint = %q", got)
	}
	if got := byName["cl"].ArgumentHint; got != "" {
		t.Fatalf("cl argument hint = %q, want empty", got)
	}
	if got := byName["empty-hint"].ArgumentHint; got != "" {
		t.Fatalf("empty hint = %q, want empty", got)
	}
	if got := byName["fallback"].Description; got != "First non-empty line is used as description when frontmatter..." {
		t.Fatalf("fallback description = %q", got)
	}
	if got := byName["project"].SourceInfo.Scope; got != "project" {
		t.Fatalf("project scope = %q", got)
	}
	if got := byName["user"].SourceInfo.Scope; got != "user" {
		t.Fatalf("user scope = %q", got)
	}
	if _, ok := byName["skip"]; ok {
		t.Fatalf("non-markdown file loaded as template")
	}
}
