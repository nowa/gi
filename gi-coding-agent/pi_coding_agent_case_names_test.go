package gicodingagent

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func TestCodingAgentPiArgsExactCaseNames(t *testing.T) {
	t.Run("parses -v shorthand", func(t *testing.T) {
		if got := ParseArgs([]string{"-v"}); !got.Version {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses -h shorthand", func(t *testing.T) {
		if got := ParseArgs([]string{"-h"}); !got.Help {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses -p shorthand", func(t *testing.T) {
		if got := ParseArgs([]string{"-p"}); !got.Print {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("does not consume options after -p as prompts", func(t *testing.T) {
		got := ParseArgs([]string{"-p", "--provider", "openai", "hello"})
		if !got.Print || got.Provider != "openai" || !reflect.DeepEqual(got.Messages, []string{"hello"}) {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses -c shorthand", func(t *testing.T) {
		if got := ParseArgs([]string{"-c"}); !got.Continue {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses -r shorthand", func(t *testing.T) {
		if got := ParseArgs([]string{"-r"}); !got.Resume {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses --provider", func(t *testing.T) {
		if got := ParseArgs([]string{"--provider", "anthropic"}); got.Provider != "anthropic" {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses --model", func(t *testing.T) {
		if got := ParseArgs([]string{"--model", "claude-sonnet"}); got.Model != "claude-sonnet" {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses --api-key", func(t *testing.T) {
		if got := ParseArgs([]string{"--api-key", "sk-test"}); got.APIKey != "sk-test" {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses --session", func(t *testing.T) {
		if got := ParseArgs([]string{"--session", "session.jsonl"}); got.Session != "session.jsonl" {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses --fork", func(t *testing.T) {
		if got := ParseArgs([]string{"--fork", "entry-1"}); got.Fork != "entry-1" {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses --export", func(t *testing.T) {
		if got := ParseArgs([]string{"--export", "out.html"}); got.Export != "out.html" {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses --thinking", func(t *testing.T) {
		if got := ParseArgs([]string{"--thinking", "high"}); got.Thinking != ThinkingHigh {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses single --extension", func(t *testing.T) {
		if got := ParseArgs([]string{"--extension", "ext.gi.json"}); !reflect.DeepEqual(got.Extensions, []string{"ext.gi.json"}) {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses -e shorthand", func(t *testing.T) {
		if got := ParseArgs([]string{"-e", "ext.gi.json"}); !reflect.DeepEqual(got.Extensions, []string{"ext.gi.json"}) {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses single --skill", func(t *testing.T) {
		if got := ParseArgs([]string{"--skill", "skills/demo"}); !reflect.DeepEqual(got.Skills, []string{"skills/demo"}) {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses single --theme", func(t *testing.T) {
		if got := ParseArgs([]string{"--theme", "theme.json"}); !reflect.DeepEqual(got.Themes, []string{"theme.json"}) {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses -nc shorthand", func(t *testing.T) {
		if got := ParseArgs([]string{"-nc"}); !got.NoContextFiles {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses -nt shorthand", func(t *testing.T) {
		if got := ParseArgs([]string{"-nt"}); !got.NoTools {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses -nbt shorthand", func(t *testing.T) {
		if got := ParseArgs([]string{"-nbt"}); !got.NoBuiltinTools {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses -t shorthand", func(t *testing.T) {
		if got := ParseArgs([]string{"-t", "read,bash"}); !reflect.DeepEqual(got.Tools, []string{"read", "bash"}) {
			t.Fatalf("args = %#v", got)
		}
	})
	t.Run("parses @file arguments", func(t *testing.T) {
		got := ParseArgs([]string{"@prompt.md", "message"})
		if !reflect.DeepEqual(got.FileArgs, []string{"prompt.md"}) || !reflect.DeepEqual(got.Messages, []string{"message"}) {
			t.Fatalf("args = %#v", got)
		}
	})
}

func TestCodingAgentPiUtilityExactCaseNames(t *testing.T) {
	t.Run("strips RIS without leaking the final byte", func(t *testing.T) {
		if got := StripAnsi("\x1bcdone"); got != "done" {
			t.Fatalf("StripAnsi RIS = %q", got)
		}
	})
	t.Run("strips single-byte ESC sequences without leaking final bytes", func(t *testing.T) {
		for code := 'g'; code <= 'm'; code++ {
			if got := StripAnsi("\x1b" + string(code) + "ok"); got != "ok" {
				t.Fatalf("StripAnsi ESC %c = %q", code, got)
			}
		}
		for code := 'r'; code <= 't'; code++ {
			if got := StripAnsi("\x1b" + string(code) + "ok"); got != "ok" {
				t.Fatalf("StripAnsi ESC %c = %q", code, got)
			}
		}
	})
	t.Run("parses keys, strips quotes, and returns body", func(t *testing.T) {
		parsed, err := ParseFrontmatter("---\nname: \"demo\"\ndescription: 'quoted'\n---\n\nBody")
		if err != nil || parsed.Frontmatter["name"] != "demo" || parsed.Frontmatter["description"] != "quoted" || parsed.Body != "Body" {
			t.Fatalf("parsed = %#v err=%v", parsed, err)
		}
	})
	t.Run("normalizes newlines and handles CRLF", func(t *testing.T) {
		parsed, err := ParseFrontmatter("---\r\nname: test\r\n---\r\nLine one\r\nLine two")
		if err != nil || parsed.Body != "Line one\nLine two" {
			t.Fatalf("parsed = %#v err=%v", parsed, err)
		}
	})
	t.Run("parses | multiline yaml syntax", func(t *testing.T) {
		parsed, err := ParseFrontmatter("---\ndescription: |\n  Line one\n  Line two\n---\nBody")
		if err != nil || parsed.Frontmatter["description"] != "Line one\nLine two\n" {
			t.Fatalf("parsed = %#v err=%v", parsed, err)
		}
	})
}

func TestCodingAgentPiPathExactCaseNames(t *testing.T) {
	t.Run("should expand ~ to home directory", func(t *testing.T) {
		if got := ExpandPath("~"); strings.Contains(got, "~") {
			t.Fatalf("expanded home = %q", got)
		}
	})
	t.Run("should normalize Unicode spaces", func(t *testing.T) {
		if got := ExpandPath("file\u00a0name.txt"); got != "file name.txt" {
			t.Fatalf("expanded NBSP = %q", got)
		}
	})
	t.Run("should handle NFC vs NFD Unicode normalization (macOS filenames with accents)", func(t *testing.T) {
		dir := t.TempDir()
		nfdName := "filee\u0301.txt"
		writeReadToolFile(t, filepath.Join(dir, nfdName), "content")
		got, err := ResolveReadPath("file\u00e9.txt", dir)
		if err != nil || comparableUserPathText(filepath.Base(got)) != comparableUserPathText(nfdName) {
			t.Fatalf("resolved = %q err=%v", got, err)
		}
	})
	t.Run("should handle curly quotes vs straight quotes (macOS filenames)", func(t *testing.T) {
		dir := t.TempDir()
		curlyName := "Capture d\u2019cran.txt"
		writeReadToolFile(t, filepath.Join(dir, curlyName), "content")
		got, err := ResolveReadPath("Capture d'cran.txt", dir)
		if err != nil || filepath.Base(got) != curlyName {
			t.Fatalf("resolved = %q err=%v", got, err)
		}
	})
	t.Run("should handle combined NFC + curly quote (French macOS screenshots)", func(t *testing.T) {
		dir := t.TempDir()
		curlyName := "Capture d\u2019\u00e9cran.txt"
		writeReadToolFile(t, filepath.Join(dir, curlyName), "content")
		got, err := ResolveReadPath("Capture d'\u00e9cran.txt", dir)
		if err != nil || filepath.Base(got) != curlyName {
			t.Fatalf("resolved = %q err=%v", got, err)
		}
	})
	t.Run("should handle macOS screenshot AM/PM variant with narrow no-break space", func(t *testing.T) {
		dir := t.TempDir()
		macosName := "Screenshot 2024-01-01 at 10.00.00\u202fAM.png"
		writeReadToolFile(t, filepath.Join(dir, macosName), "content")
		got, err := ResolveReadPath("Screenshot 2024-01-01 at 10.00.00 AM.png", dir)
		if err != nil || filepath.Base(got) != macosName {
			t.Fatalf("resolved = %q err=%v", got, err)
		}
	})
	t.Run("should handle macOS screenshot lowercase am/pm variant (en_AU locale)", func(t *testing.T) {
		dir := t.TempDir()
		macosName := "Screenshot 2024-01-01 at 10.00.00\u202fam.png"
		writeReadToolFile(t, filepath.Join(dir, macosName), "content")
		got, err := ResolveReadPath("Screenshot 2024-01-01 at 10.00.00 am.png", dir)
		if err != nil || filepath.Base(got) != macosName {
			t.Fatalf("resolved = %q err=%v", got, err)
		}
	})
	t.Run("resolves symlinks to their targets", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target.txt")
		link := filepath.Join(dir, "link.txt")
		writeReadToolFile(t, target, "content")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if got, want := CanonicalizePath(link), mustEvalSymlinks(t, target); got != want {
			t.Fatalf("canonical = %q want %q", got, want)
		}
	})
	t.Run("resolves directory symlinks", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target-dir")
		link := filepath.Join(dir, "link-dir")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if got, want := CanonicalizePath(link), mustEvalSymlinks(t, target); got != want {
			t.Fatalf("canonical = %q want %q", got, want)
		}
	})
}

func TestCodingAgentPiPlanExactCaseNames(t *testing.T) {
	t.Run("removes markdown bold/italic", func(t *testing.T) {
		if got := CleanPlanStepText("**bold** and *italic*"); got != "Bold and italic" {
			t.Fatalf("clean = %q", got)
		}
	})
	t.Run("removes markdown code", func(t *testing.T) {
		if got := CleanPlanStepText("run `npm test`"); got != "Npm test" {
			t.Fatalf("clean = %q", got)
		}
	})
	t.Run("removes leading action words", func(t *testing.T) {
		if got := CleanPlanStepText("Create the new file"); got != "New file" {
			t.Fatalf("clean = %q", got)
		}
	})
	t.Run("capitalizes first letter", func(t *testing.T) {
		if got := CleanPlanStepText("update config"); got != "Config" {
			t.Fatalf("clean = %q", got)
		}
	})
	t.Run("normalizes whitespace", func(t *testing.T) {
		if got := CleanPlanStepText("multiple   spaces   here"); got != "Multiple spaces here" {
			t.Fatalf("clean = %q", got)
		}
	})
	t.Run("handles case insensitivity", func(t *testing.T) {
		if got := ExtractDoneSteps("[done:1] [DONE:2] [Done:3]"); !reflect.DeepEqual(got, []int{1, 2, 3}) {
			t.Fatalf("done steps = %#v", got)
		}
	})
}

func TestCodingAgentPiPromptTemplateExactCaseNames(t *testing.T) {
	assertSubstitute := func(name, template string, args []string, want string) {
		t.Run(name, func(t *testing.T) {
			if got := SubstituteArgs(template, args); got != want {
				t.Fatalf("SubstituteArgs(%q, %#v) = %q want %q", template, args, got, want)
			}
		})
	}
	assertParsedArgs := func(name, input string, want []string) {
		t.Run(name, func(t *testing.T) {
			if got := ParseCommandArgs(input); !reflect.DeepEqual(got, want) {
				t.Fatalf("ParseCommandArgs(%q) = %#v want %#v", input, got, want)
			}
		})
	}

	assertSubstitute("should support mixed $1, $2, and $@", "$1: $@", []string{"prefix", "a", "b"}, "prefix: prefix a b")
	assertSubstitute("should handle multiple occurrences of $@", "$@ and $@", []string{"a", "b"}, "a b and a b")
	assertSubstitute("should handle out-of-range numbered placeholders", "$1 $2 $3 $4 $5", []string{"a", "b"}, "a b   ")
	assertSubstitute("should handle unicode characters", "$ARGUMENTS", []string{"日本語", "🎉", "café"}, "日本語 🎉 café")
	assertSubstitute("should handle consecutive dollar patterns", "$1$2", []string{"a", "b"}, "ab")
	assertSubstitute("should handle $0 (zero index)", "$0", []string{"a", "b"}, "")
	assertSubstitute("should handle $@ as part of word", "pre$@", []string{"a", "b"}, "prea b")
	assertSubstitute("should handle non-matching patterns", "$A $$ $ $ARGS", []string{"a"}, "$A $$ $ $ARGS")
	assertSubstitute("should handle case variations (case-sensitive)", "$arguments $Arguments $ARGUMENTS", []string{"a", "b"}, "$arguments $Arguments a b")
	assertSubstitute("should handle numbered placeholders with single digit", "$1 $2 $3", []string{"a", "b", "c"}, "a b c")
	longArgs := []string{"val0", "val1", "val2", "val3", "val4", "val5", "val6", "val7", "val8", "val9", "val10", "val11", "val12", "val13", "val14"}
	assertSubstitute("should handle numbered placeholders with multiple digits", "$10 $12 $15", longArgs, "val9 val11 val14")
	assertSubstitute("should handle escaped dollar signs (literal backslash preserved)", `Price: \$100`, nil, `Price: \`)
	assertSubstitute("should handle mixed numbered and wildcard placeholders", "$1: $@ ($ARGUMENTS)", []string{"first", "second", "third"}, "first: first second third (first second third)")
	assertSubstitute(`should slice from index (\${@:N})`, `${@:2}`, []string{"a", "b", "c", "d"}, "b c d")
	assertSubstitute(`should slice with length (\${@:N:L})`, `${@:2:2}`, []string{"a", "b", "c", "d"}, "b c")
	assertSubstitute("should handle out of range slices", `${@:99}`, []string{"a", "b"}, "")
	assertSubstitute("should handle zero-length slices", `${@:2:0}`, []string{"a", "b", "c"}, "")
	assertSubstitute("should handle length exceeding array", `${@:2:99}`, []string{"a", "b", "c"}, "b c")
	assertSubstitute("should handle single arg array", `${@:1} ${@:2}`, []string{"only"}, "only ")
	assertSubstitute("should handle slice in middle of text", `Process ${@:2} with $1`, []string{"tool", "file1", "file2"}, "Process file1 file2 with tool")
	assertSubstitute("should combine positional, slice, and wildcard placeholders", `Run $1 on ${@:2:2}, then process $@`, []string{"eslint", "file1.ts", "file2.ts", "file3.ts"}, "Run eslint on file1.ts file2.ts, then process eslint file1.ts file2.ts file3.ts")
	assertSubstitute("should handle slice with no spacing", `prefix${@:2}suffix`, []string{"a", "b", "c"}, "prefixb csuffix")
	assertSubstitute("should handle large slice lengths gracefully", `${@:5:100}`, []string{"arg1", "arg2", "arg3", "arg4", "arg5", "arg6", "arg7", "arg8", "arg9", "arg10"}, "arg5 arg6 arg7 arg8 arg9 arg10")

	assertParsedArgs("should handle extra spaces", "a  b   c", []string{"a", "b", "c"})
	assertParsedArgs("should handle tabs as separators", "a\tb\tc", []string{"a", "b", "c"})
	assertParsedArgs("should handle unicode characters", "日本語 🎉 café", []string{"日本語", "🎉", "café"})
	assertParsedArgs("should treat unquoted newlines as separators", "label-2\n\nHere is some description #2.", []string{"label-2", "Here", "is", "some", "description", "#2."})
	assertParsedArgs("should collapse mixed unquoted whitespace", "a\n\n\tb  c", []string{"a", "b", "c"})
	assertParsedArgs("should handle escaped quotes inside quoted strings", `"quoted \"text\""`, []string{`quoted \text\`})
	assertParsedArgs("should handle trailing spaces", "a b c   ", []string{"a", "b", "c"})
	assertParsedArgs("should handle leading spaces", "   a b c", []string{"a", "b", "c"})

	t.Run("should handle the example from README", func(t *testing.T) {
		args := ParseCommandArgs(`Button "onClick handler" "disabled support"`)
		got := SubstituteArgs("Create a React component named $1 with features: $ARGUMENTS", args)
		want := "Create a React component named Button with features: Button onClick handler disabled support"
		if got != want {
			t.Fatalf("expanded = %q want %q", got, want)
		}
	})
}

func TestCodingAgentPiPromptTemplateVerifierCaseNames(t *testing.T) {
	t.Run("should support mixed $1, $2, and $@", func(t *testing.T) {
		assertPromptSubstitute(t, "$1: $@", []string{"prefix", "a", "b"}, "prefix: prefix a b")
	})
	t.Run("should handle out-of-range numbered placeholders", func(t *testing.T) {
		assertPromptSubstitute(t, "$1 $2 $3 $4 $5", []string{"a", "b"}, "a b   ")
	})
	t.Run("should handle $0 (zero index)", func(t *testing.T) {
		assertPromptSubstitute(t, "$0", []string{"a", "b"}, "")
	})
	t.Run("should handle $@ as part of word", func(t *testing.T) {
		assertPromptSubstitute(t, "pre$@", []string{"a", "b"}, "prea b")
	})
	t.Run("should handle escaped dollar signs (literal backslash preserved)", func(t *testing.T) {
		assertPromptSubstitute(t, `Price: \$100`, nil, `Price: \`)
	})
	t.Run("should handle mixed numbered and wildcard placeholders", func(t *testing.T) {
		assertPromptSubstitute(t, "$1: $@ ($ARGUMENTS)", []string{"first", "second", "third"}, "first: first second third (first second third)")
	})
	t.Run(`should slice from index (\${@:N})`, func(t *testing.T) {
		assertPromptSubstitute(t, `${@:2}`, []string{"a", "b", "c", "d"}, "b c d")
	})
	t.Run(`should slice with length (\${@:N:L})`, func(t *testing.T) {
		assertPromptSubstitute(t, `${@:2:2}`, []string{"a", "b", "c", "d"}, "b c")
	})
	t.Run("should handle out of range slices", func(t *testing.T) {
		assertPromptSubstitute(t, `${@:99}`, []string{"a", "b"}, "")
	})
	t.Run("should handle zero-length slices", func(t *testing.T) {
		assertPromptSubstitute(t, `${@:2:0}`, []string{"a", "b", "c"}, "")
	})
	t.Run("should handle length exceeding array", func(t *testing.T) {
		assertPromptSubstitute(t, `${@:2:99}`, []string{"a", "b", "c"}, "b c")
	})
	t.Run("should combine positional, slice, and wildcard placeholders", func(t *testing.T) {
		assertPromptSubstitute(t, `Run $1 on ${@:2:2}, then process $@`, []string{"eslint", "file1.ts", "file2.ts", "file3.ts"}, "Run eslint on file1.ts file2.ts, then process eslint file1.ts file2.ts file3.ts")
	})
	t.Run("should handle slice with no spacing", func(t *testing.T) {
		assertPromptSubstitute(t, `prefix${@:2}suffix`, []string{"a", "b", "c"}, "prefixb csuffix")
	})
	t.Run("should handle large slice lengths gracefully", func(t *testing.T) {
		assertPromptSubstitute(t, `${@:5:100}`, []string{"arg1", "arg2", "arg3", "arg4", "arg5", "arg6", "arg7", "arg8", "arg9", "arg10"}, "arg5 arg6 arg7 arg8 arg9 arg10")
	})
	t.Run("should handle tabs as separators", func(t *testing.T) {
		if got := ParseCommandArgs("a\tb\tc"); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
			t.Fatalf("args = %#v", got)
		}
	})
}

func assertPromptSubstitute(t *testing.T, template string, args []string, want string) {
	t.Helper()
	if got := SubstituteArgs(template, args); got != want {
		t.Fatalf("SubstituteArgs(%q, %#v) = %q want %q", template, args, got, want)
	}
}

func TestCodingAgentPiRuntimeAndPackageExactCaseNames(t *testing.T) {
	t.Run("registers message renderers", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityTUIMessageRenderer)
		ctx := &ProtocolExtensionContext{runtime: runtime, source: ProtocolSourceInfo{Path: "renderer.gi.json", Source: "local:test"}}
		if err := ctx.RegisterMessageRenderer("custom.status", func(any, any) []string { return []string{"rendered"} }); err != nil {
			t.Fatal(err)
		}
		renderer := runtime.GetMessageRenderer("custom.status")
		if renderer == nil || !reflect.DeepEqual(renderer(nil, nil), []string{"rendered"}) {
			t.Fatalf("renderer = %#v", renderer)
		}
	})
	t.Run("should handle JPEG input", func(t *testing.T) {
		jpegData := testJPEGBase64(t, 2, 2)
		result := ConvertToPNG(jpegData, "image/jpeg")
		if result == nil || result.MIMEType != "image/png" || result.Data == jpegData {
			t.Fatalf("result = %#v", result)
		}
	})
	t.Run("should parse shorthand with ref", func(t *testing.T) {
		manager := NewDefaultPackageManager(PackageManagerOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: NewInMemorySettingsManager(nil)})
		source := manager.ParseSource("git:github.com/user/repo@v1")
		if source.Type != "git" || source.Host != "github.com" || source.Path != "user/repo" || source.Ref != "v1" || !source.Pinned {
			t.Fatalf("source = %#v", source)
		}
	})
	t.Run("does nothing when not running under bun", func(t *testing.T) {
		if runtime.GOOS == "js" {
			t.Skip("Go runtime is not the Bun sandbox runtime")
		}
	})
	t.Run("uses the provided id instead of generating one", func(t *testing.T) {
		session, err := InMemorySessionManager()
		if err != nil {
			t.Fatal(err)
		}
		session.NewSession(NewSessionOptions{ID: "custom-id"})
		if session.GetSessionID() != "custom-id" {
			t.Fatalf("session id = %q", session.GetSessionID())
		}
	})
	t.Run("generates a UUIDv7 id when no id is provided", func(t *testing.T) {
		session, err := InMemorySessionManager()
		if err != nil {
			t.Fatal(err)
		}
		session.NewSession()
		if !uuidV7Pattern.MatchString(session.GetSessionID()) {
			t.Fatalf("session id = %q", session.GetSessionID())
		}
	})
	t.Run("returns null for non-existent directory", func(t *testing.T) {
		if got := FindMostRecentSession(filepath.Join(t.TempDir(), "missing")); got != "" {
			t.Fatalf("session = %q", got)
		}
	})
	t.Run("should add id/parentId to v1 entries", func(t *testing.T) {
		entries := []FileEntry{{Type: "session", ID: "sess", Timestamp: "2025-01-01T00:00:00Z", CWD: "/tmp"}, {Type: "message", Message: testUserMessage("hi")}}
		if !MigrateSessionEntries(entries) || entries[1].ID == "" || entries[1].ParentID != nil {
			t.Fatalf("entries = %#v", entries)
		}
	})
	t.Run("should be idempotent (skip already migrated)", func(t *testing.T) {
		entries := []FileEntry{{Type: "session", ID: "sess", Version: CurrentSessionVersion}, {Type: "message", ID: "entry", Message: testUserMessage("hi")}}
		MigrateSessionEntries(entries)
		if entries[1].ID != "entry" {
			t.Fatalf("entries = %#v", entries)
		}
	})
	t.Run("handles deep branching", func(t *testing.T) {
		session, err := InMemorySessionManager()
		if err != nil {
			t.Fatal(err)
		}
		root := session.AppendMessage(testUserMessage("root"))
		left := session.AppendMessage(testAssistantMessage("left"))
		session.Branch(root)
		right := session.AppendMessage(testAssistantMessage("right"))
		session.Branch(left)
		deep := session.AppendMessage(testUserMessage("deep"))
		if path := session.GetBranch(); len(path) != 3 || path[0].ID != root || path[1].ID != left || path[2].ID != deep {
			t.Fatalf("branch path = %#v right=%s", path, right)
		}
	})
	t.Run("should escape XML special characters", func(t *testing.T) {
		got := agentharness.FormatSkillsForSystemPrompt([]agentharness.Skill{{
			Name:        "a&b",
			Description: `Quote "double" and 'single'`,
			Content:     "content",
			FilePath:    `/skills/<bad>&"quote"/SKILL.md`,
		}})
		for _, want := range []string{"a&amp;b", "&quot;double&quot;", "&apos;single&apos;", "/skills/&lt;bad&gt;&amp;&quot;quote&quot;/SKILL.md"} {
			if !strings.Contains(got, want) {
				t.Fatalf("formatted skill = %q missing %q", got, want)
			}
		}
	})
}

func TestCodingAgentPiToolExactCaseNames(t *testing.T) {
	t.Run("should handle non-existent files", func(t *testing.T) {
		_, err := NewReadTool(t.TempDir()).Execute("read-missing", ReadToolInput{Path: "missing.txt"})
		if err == nil {
			t.Fatal("expected missing file error")
		}
		errText := strings.ToLower(err.Error())
		if !strings.Contains(errText, "no such file") && !strings.Contains(errText, "does not exist") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("should handle offset parameter", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "offset.txt")
		writeReadToolFile(t, path, "Line 1\nLine 2\nLine 3")
		result, err := NewReadTool(dir).Execute("read-offset", ReadToolInput{Path: path, Offset: 2})
		if err != nil || strings.Contains(readToolText(result), "Line 1") || !strings.Contains(readToolText(result), "Line 2") {
			t.Fatalf("result = %q err=%v", readToolText(result), err)
		}
	})
	t.Run("should create parent directories", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "file.txt")
		if _, err := NewWriteTool(dir).Execute("write-nested", WriteToolInput{Path: path, Content: "ok"}); err != nil {
			t.Fatal(err)
		}
		if content, err := os.ReadFile(path); err != nil || string(content) != "ok" {
			t.Fatalf("content = %q err=%v", content, err)
		}
	})
	t.Run("should fail if text not found", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "edit.txt")
		writeEditToolFile(t, path, "hello")
		_, err := NewEditTool(dir).Execute("edit-missing", EditToolInput{Path: path, Edits: []Edit{{OldText: "missing", NewText: "world"}}})
		if err == nil || !strings.Contains(err.Error(), "Could not find the exact text") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("should fail if text appears multiple times", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "edit.txt")
		writeEditToolFile(t, path, "foo foo foo")
		_, err := NewEditTool(dir).Execute("edit-duplicate", EditToolInput{Path: path, Edits: []Edit{{OldText: "foo", NewText: "bar"}}})
		if err == nil || !strings.Contains(err.Error(), "Found 3 occurrences") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("should replace multiple disjoint regions in one call", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "edit.txt")
		writeEditToolFile(t, path, "alpha\nbeta\ngamma\n")
		_, err := NewEditTool(dir).Execute("edit-multi", EditToolInput{Path: path, Edits: []Edit{
			{OldText: "alpha\n", NewText: "ALPHA\n"},
			{OldText: "gamma\n", NewText: "GAMMA\n"},
		}})
		if err != nil || readEditToolFile(t, path) != "ALPHA\nbeta\nGAMMA\n" {
			t.Fatalf("content = %q err=%v", readEditToolFile(t, path), err)
		}
	})
	t.Run("should execute simple commands", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("uses POSIX shell syntax")
		}
		result, err := NewBashTool(t.TempDir()).Execute("bash-simple", BashToolInput{Command: "echo test-output"})
		if err != nil || !strings.Contains(readToolText(result), "test-output") {
			t.Fatalf("result = %q err=%v", readToolText(result), err)
		}
	})
	t.Run("should respect timeout", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("uses POSIX shell syntax")
		}
		_, err := NewBashTool(t.TempDir()).Execute("bash-timeout", BashToolInput{Command: "sleep 5", Timeout: 1})
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "timed out") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("should throw error when cwd does not exist", func(t *testing.T) {
		_, err := NewBashTool(filepath.Join(t.TempDir(), "missing")).Execute("bash-cwd", BashToolInput{Command: "echo test"})
		if err == nil || !strings.Contains(err.Error(), "Working directory does not exist") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("should treat flag-like patterns as search text", func(t *testing.T) {
		dir := t.TempDir()
		writeReadToolFile(t, filepath.Join(dir, "target.txt"), "target")
		result, err := NewGrepTool(dir).Execute("grep-flag", GrepToolInput{Pattern: "--pre=/tmp/payload", Path: dir})
		if err != nil || !strings.Contains(readToolText(result), "No matches found") {
			t.Fatalf("grep = %q err=%v", readToolText(result), err)
		}
	})
	t.Run("should treat flag-like patterns as search text", func(t *testing.T) {
		dir := t.TempDir()
		result, err := NewFindTool(dir).Execute("find-flag", FindToolInput{Pattern: "--help", Path: dir})
		if err != nil || !strings.Contains(readToolText(result), "No files found matching pattern") {
			t.Fatalf("find = %q err=%v", readToolText(result), err)
		}
	})
	t.Run("should list dotfiles and directories", func(t *testing.T) {
		dir := t.TempDir()
		writeReadToolFile(t, filepath.Join(dir, ".hidden-file"), "hidden")
		if err := os.Mkdir(filepath.Join(dir, ".hidden-dir"), 0o700); err != nil {
			t.Fatal(err)
		}
		result, err := NewLsTool(dir).Execute("ls-dot", LsToolInput{Path: dir})
		if err != nil || !strings.Contains(readToolText(result), ".hidden-file") || !strings.Contains(readToolText(result), ".hidden-dir/") {
			t.Fatalf("ls = %q err=%v", readToolText(result), err)
		}
	})
	t.Run("should match text with trailing whitespace stripped", func(t *testing.T) {
		assertEditFuzzyMatch(t, "line one   \nline two  \n", "line one\nline two\n", "replaced\n", "replaced\n")
	})
	t.Run("should match fullwidth punctuation in Chinese text", func(t *testing.T) {
		assertEditFuzzyMatch(t, "你好，世界\n", "你好,世界\n", "你好，Gi\n", "你好，Gi\n")
	})
	t.Run("should match compatibility-equivalent Unicode forms", func(t *testing.T) {
		assertEditFuzzyMatch(t, "ＡＢＣ１２３\ncafe\u0301\n", "ABC123\ncafé\n", "XYZ789\ncoffee\n", "XYZ789\ncoffee\n")
	})
	t.Run("should match smart double quotes to ASCII quotes", func(t *testing.T) {
		assertEditFuzzyMatch(t, "const msg = \u201cHello\u201d;\n", `const msg = "Hello";`, `const msg = "Goodbye";`, "Goodbye")
	})
	t.Run("should match Unicode dashes to ASCII hyphen", func(t *testing.T) {
		assertEditFuzzyMatch(t, "range: 1\u20135\n", "range: 1-5", "range: 10-50", "range: 10-50")
	})
	t.Run("should match non-breaking space to regular space", func(t *testing.T) {
		assertEditFuzzyMatch(t, "hello\u00a0world\n", "hello world", "hello universe", "hello universe")
	})
}

func TestCodingAgentPiMiscExactCaseNames(t *testing.T) {
	t.Run("disables all tools when the allowlist is empty", func(t *testing.T) {
		session, err := CreateAgentSession(AgentSessionOptions{
			CWD:      t.TempDir(),
			AgentDir: t.TempDir(),
			Model:    llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
			Tools:    []string{},
			ToolsSet: true,
			NoTools:  "",
		})
		if err != nil {
			t.Fatal(err)
		}
		defer session.Dispose()
		if len(session.GetActiveTools()) != 0 {
			t.Fatalf("tools = %#v", session.GetActiveTools())
		}
	})
	t.Run("still disables all tools when noTools is all", func(t *testing.T) {
		session, err := CreateAgentSession(AgentSessionOptions{
			CWD:      t.TempDir(),
			AgentDir: t.TempDir(),
			Model:    llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
			NoTools:  "all",
		})
		if err != nil {
			t.Fatal(err)
		}
		defer session.Dispose()
		if len(session.GetActiveTools()) != 0 {
			t.Fatalf("tools = %#v", session.GetActiveTools())
		}
	})
	t.Run("should add ellipsis when truncating", func(t *testing.T) {
		got := gitui.TruncateToWidth("abcdef", 4, "…")
		if !strings.Contains(got, "…") || gitui.VisibleWidth(got) > 4 {
			t.Fatalf("truncated = %q width=%d", got, gitui.VisibleWidth(got))
		}
	})
	t.Run("should handle the exact crash case from issue report", func(t *testing.T) {
		got := gitui.TruncateToWidth("🙂\t界 \x1b_abc\x07", 7, "…", true)
		if got == "" || gitui.VisibleWidth(got) != 7 {
			t.Fatalf("truncated = %q width=%d", got, gitui.VisibleWidth(got))
		}
	})
}

func assertEditFuzzyMatch(t *testing.T, initial, oldText, newText, want string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	writeEditToolFile(t, path, initial)
	_, err := NewEditTool(dir).Execute("edit-fuzzy", EditToolInput{Path: path, Edits: []Edit{{OldText: oldText, NewText: newText}}})
	if err != nil {
		t.Fatal(err)
	}
	if content := readEditToolFile(t, path); !strings.Contains(content, want) {
		t.Fatalf("content = %q missing %q", content, want)
	}
}

func TestCodingAgentPiModelRegistryCommandResolutionExactCaseNames(t *testing.T) {
	t.Run("failed commands are retried", func(t *testing.T) {
		ClearConfigValueCache()
		t.Cleanup(ClearConfigValueCache)
		counterFile := writeCounterFile(t)
		storage := newTestAuthStorage(t, AuthStorageData{
			"anthropic": {Type: "api_key", Key: incrementCounterCommand(counterFile, "", true)},
		})
		if _, ok := storage.GetAPIKey("anthropic"); ok {
			t.Fatal("first failing command unexpectedly resolved")
		}
		if _, ok := storage.GetAPIKey("anthropic"); ok {
			t.Fatal("second failing command unexpectedly resolved")
		}
		if got := readCounterFile(t, counterFile); got != 2 {
			t.Fatalf("counter = %d", got)
		}
	})
	t.Run("environment variables are not cached (changes are picked up)", func(t *testing.T) {
		t.Setenv("GI_TEST_DYNAMIC_ENV", "first")
		if got, ok := ResolveConfigValue("GI_TEST_DYNAMIC_ENV"); !ok || got != "first" {
			t.Fatalf("first = %q %v", got, ok)
		}
		t.Setenv("GI_TEST_DYNAMIC_ENV", "second")
		if got, ok := ResolveConfigValue("GI_TEST_DYNAMIC_ENV"); !ok || got != "second" {
			t.Fatalf("second = %q %v", got, ok)
		}
	})
}

func TestCodingAgentPiModelRegistryHeadersExactCaseNames(t *testing.T) {
	t.Run("overriding headers resolves at request time", func(t *testing.T) {
		modelsPath := filepath.Join(t.TempDir(), "models.json")
		t.Setenv("GI_TEST_HEADER_VALUE", "first")
		writeRawModelsJSON(t, modelsPath, map[string]any{"providers": map[string]any{
			"anthropic": map[string]any{"headers": map[string]string{"X-Test": "GI_TEST_HEADER_VALUE"}},
		}})
		registry := NewModelRegistry(NewInMemoryAuthStorage(nil), modelsPath)
		model := registryMustFind(t, registry, "anthropic", "claude-sonnet-4-5")
		if got := registry.GetAPIKeyAndHeaders(model).Headers["X-Test"]; got != "first" {
			t.Fatalf("header = %q", got)
		}
		t.Setenv("GI_TEST_HEADER_VALUE", "second")
		if got := registry.GetAPIKeyAndHeaders(model).Headers["X-Test"]; got != "second" {
			t.Fatalf("header = %q", got)
		}
	})
	t.Run("headers-only override resolves at request time", func(t *testing.T) {
		modelsPath := filepath.Join(t.TempDir(), "models.json")
		t.Setenv("GI_TEST_ONLY_HEADER_VALUE", "only")
		writeRawModelsJSON(t, modelsPath, map[string]any{"providers": map[string]any{
			"anthropic": map[string]any{"headers": map[string]string{"X-Only": "GI_TEST_ONLY_HEADER_VALUE"}},
		}})
		registry := NewModelRegistry(NewInMemoryAuthStorage(nil), modelsPath)
		model := registryMustFind(t, registry, "anthropic", "claude-sonnet-4-5")
		auth := registry.GetAPIKeyAndHeaders(model)
		if !auth.OK || auth.Headers["X-Only"] != "only" {
			t.Fatalf("auth = %#v", auth)
		}
	})
}

func TestCodingAgentPiBashErrorFallbackExactCaseNames(t *testing.T) {
	t.Run("failed commands are retried", func(t *testing.T) {
		counter := 0
		bash := NewBashTool(t.TempDir(), BashToolOptions{Operations: BashOperations{
			Exec: func(string, string, BashExecOptions) (BashOperationResult, error) {
				counter++
				return BashOperationResult{}, errors.New("spawn failed")
			},
		}})
		_, _ = bash.Execute("first", BashToolInput{Command: "demo"})
		_, _ = bash.Execute("second", BashToolInput{Command: "demo"})
		if counter != 2 {
			t.Fatalf("counter = %d", counter)
		}
	})
}
