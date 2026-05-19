package gitui

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestFuzzyMatchPiSemantics(t *testing.T) {
	if match := FuzzyMatchText("", "anything"); !match.Matches || match.Score != 0 {
		t.Fatalf("empty query match = %#v", match)
	}
	if match := FuzzyMatchText("longquery", "short"); match.Matches {
		t.Fatalf("long query should not match")
	}
	exact := FuzzyMatchText("test", "test")
	if !exact.Matches || exact.Score >= 0 {
		t.Fatalf("exact match should have negative score: %#v", exact)
	}
	if !FuzzyMatchText("abc", "aXbXc").Matches || FuzzyMatchText("abc", "cba").Matches {
		t.Fatalf("order-sensitive matching failed")
	}
	consecutive := FuzzyMatchText("foo", "foobar")
	scattered := FuzzyMatchText("foo", "f_o_o_bar")
	if !(consecutive.Matches && scattered.Matches && consecutive.Score < scattered.Score) {
		t.Fatalf("consecutive score = %#v scattered = %#v", consecutive, scattered)
	}
	boundary := FuzzyMatchText("fb", "foo-bar")
	notBoundary := FuzzyMatchText("fb", "afbx")
	if !(boundary.Matches && notBoundary.Matches && boundary.Score < notBoundary.Score) {
		t.Fatalf("boundary score = %#v non = %#v", boundary, notBoundary)
	}
	if !FuzzyMatchText("codex52", "gpt-5.2-codex").Matches {
		t.Fatalf("expected swapped alpha numeric tokens to match")
	}
}

func TestFuzzyFilterPiSemantics(t *testing.T) {
	items := []string{"apple", "banana", "cherry"}
	if got := FuzzyFilter(items, "", func(s string) string { return s }); !reflect.DeepEqual(got, items) {
		t.Fatalf("empty query = %#v", got)
	}
	if got := FuzzyFilterStrings(items, "an"); !reflect.DeepEqual(got, []string{"banana"}) {
		t.Fatalf("filter = %#v", got)
	}
	if got := FuzzyFilterStrings([]string{"a_p_p", "app", "application"}, "app"); got[0] != "app" {
		t.Fatalf("best match first = %#v", got)
	}
	if got := FuzzyFilterStrings([]string{"clone", "cl"}, "cl"); !reflect.DeepEqual(got, []string{"cl", "clone"}) {
		t.Fatalf("exact before prefix = %#v", got)
	}
}

func TestCombinedAutocompleteProviderReturnsFirstNonEmptyProvider(t *testing.T) {
	first := AutocompleteProviderFunc(func(_ string, cursor int) AutocompleteSuggestions {
		return AutocompleteSuggestions{Start: cursor, End: cursor}
	})
	second := AutocompleteProviderFunc(func(_ string, cursor int) AutocompleteSuggestions {
		return AutocompleteSuggestions{Items: []AutocompleteItem{{Value: "x"}}, Start: cursor, End: cursor}
	})
	provider := NewCombinedAutocompleteProvider(first, second)
	got := provider.Suggestions("/", 1)
	if len(got.Items) != 1 || got.Items[0].Value != "x" {
		t.Fatalf("suggestions = %#v", got)
	}
}

func TestCombinedAutocompleteGetSuggestionsFallsBackToChildProviders(t *testing.T) {
	provider := NewCombinedAutocompleteProviderWithCommands(t.TempDir(), nil, AutocompleteProviderFunc(func(text string, cursor int) AutocompleteSuggestions {
		if text != "hello" || cursor != len([]rune("hello")) {
			t.Fatalf("child provider saw text=%q cursor=%d", text, cursor)
		}
		return AutocompleteSuggestions{
			Items:  []AutocompleteItem{{Value: "hello-world", Label: "hello-world"}},
			Prefix: "hello",
			Start:  0,
			End:    cursor,
		}
	}))

	result, err := provider.GetSuggestions([]string{"hello"}, 0, len([]rune("hello")), false)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) != 1 || result.Items[0].Value != "hello-world" || result.Prefix != "hello" {
		t.Fatalf("child provider fallback suggestions = %#v", result)
	}
}

func TestCombinedAutocompleteSlashCommands(t *testing.T) {
	provider := NewCombinedAutocompleteProviderWithCommands(t.TempDir(), []SlashCommand{
		{Name: "model", Description: "switch model"},
		{Name: "merge", ArgumentHint: "<branch>"},
		{Name: "open", GetArgumentCompletions: func(prefix string) []AutocompleteItem {
			if prefix == "r" {
				return []AutocompleteItem{{Value: "README.md", Label: "README.md"}}
			}
			return nil
		}},
	})
	result, err := provider.GetSuggestions([]string{"/mo"}, 0, 3, false)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) != 1 || result.Items[0].Value != "model" || result.Prefix != "/mo" {
		t.Fatalf("slash suggestions = %#v", result)
	}
	applied := provider.ApplyCompletion([]string{"/mo"}, 0, 3, result.Items[0], result.Prefix)
	if applied.Lines[0] != "/model " || applied.CursorCol != len("/model ") {
		t.Fatalf("applied slash = %#v", applied)
	}
	result, err = provider.GetSuggestions([]string{"/open r"}, 0, len("/open r"), false)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Items[0].Value != "README.md" || result.Prefix != "r" {
		t.Fatalf("arg suggestions = %#v", result)
	}
	applied = provider.ApplyCompletion([]string{"/open re"}, 0, len("/open re"), result.Items[0], "e")
	if applied.Lines[0] != "/open README.md" || applied.CursorCol != len("/open README.md") {
		t.Fatalf("stale slash argument prefix should replace full current argument: %#v", applied)
	}

	modelProvider := NewCombinedAutocompleteProviderWithCommands(t.TempDir(), []SlashCommand{{
		Name: "model",
		GetArgumentCompletions: func(prefix string) []AutocompleteItem {
			all := []AutocompleteItem{{Value: "gpt-4o", Label: "gpt-4o"}, {Value: "gpt-4o-mini", Label: "gpt-4o-mini"}}
			var out []AutocompleteItem
			for _, item := range all {
				if strings.HasPrefix(item.Value, prefix) {
					out = append(out, item)
				}
			}
			return out
		},
	}})
	applied = modelProvider.ApplyCompletion([]string{"/model gpt-4o-mini"}, 0, len("/model gpt-4o-mini"), AutocompleteItem{Value: "gpt-4o", Label: "gpt-4o"}, "gpt-4o")
	if applied.Lines[0] != "/model gpt-4o-mini" || applied.CursorCol != len("/model gpt-4o-mini") {
		t.Fatalf("exact typed slash argument should win over stale shorter selection: %#v", applied)
	}
}

func TestCombinedAutocompleteCommandItems(t *testing.T) {
	provider := NewCombinedAutocompleteProviderWithCommandItems(t.TempDir(), []AutocompleteItem{
		{Value: "clear", Description: "clear context"},
		{Value: "model", Description: "switch model"},
	})
	result, err := provider.GetSuggestions([]string{"/cl"}, 0, len("/cl"), false)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) != 1 || result.Items[0].Value != "clear" || result.Items[0].Description != "clear context" {
		t.Fatalf("command item suggestions = %#v", result)
	}

	provider.SetCommandItems([]AutocompleteItem{{Label: "help", Description: "show help"}})
	result, err = provider.GetSuggestions([]string{"/he"}, 0, len("/he"), false)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Items) != 1 || result.Items[0].Value != "help" {
		t.Fatalf("label fallback command item suggestions = %#v", result)
	}
}

func TestCombinedAutocompleteFileSuggestions(t *testing.T) {
	base := t.TempDir()
	mustWrite(t, filepath.Join(base, "README.md"), "readme")
	mustWrite(t, filepath.Join(base, "src", "index.ts"), "export {}")
	mustWrite(t, filepath.Join(base, "src.txt"), "text")
	mustWrite(t, filepath.Join(base, ".pi", "config.json"), "{}")
	mustWrite(t, filepath.Join(base, ".git", "config"), "ignore")
	provider := NewCombinedAutocompleteProviderWithCommands(base, nil)

	result, err := provider.GetSuggestions([]string{"@"}, 0, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	values := suggestionValues(result)
	for _, want := range []string{"@README.md", "@src/", "@src.txt", "@.pi/"} {
		if !contains(values, want) {
			t.Fatalf("root @ suggestions missing %q in %#v", want, values)
		}
	}
	for _, value := range values {
		if value == "@.git/" || value == "@.git/config" {
			t.Fatalf(".git should be excluded: %#v", values)
		}
	}

	result, err = provider.GetSuggestions([]string{"@src"}, 0, len("@src"), false)
	if err != nil {
		t.Fatal(err)
	}
	values = suggestionValuesInOrder(result)
	if len(values) < 2 || values[0] != "@src/" || !contains(values, "@src.txt") {
		t.Fatalf("directory should rank before file: %#v", values)
	}

	result, err = provider.GetSuggestions([]string{"@index"}, 0, len("@index"), false)
	if err != nil {
		t.Fatal(err)
	}
	values = suggestionValues(result)
	if !contains(values, "@src/index.ts") {
		t.Fatalf("nested fuzzy @ suggestions = %#v", values)
	}
}

func TestCombinedAutocompleteQuotedAndDotSlashCompletion(t *testing.T) {
	base := t.TempDir()
	mustWrite(t, filepath.Join(base, "my folder", "test.txt"), "content")
	mustWrite(t, filepath.Join(base, "my folder", "other.txt"), "content")
	mustWrite(t, filepath.Join(base, "update.sh"), "#!/bin/sh")
	provider := NewCombinedAutocompleteProviderWithCommands(base, nil)

	result, err := provider.GetSuggestions([]string{"my"}, 0, len("my"), true)
	if err != nil {
		t.Fatal(err)
	}
	values := suggestionValues(result)
	if !contains(values, "\"my folder/\"") {
		t.Fatalf("quoted direct completion = %#v", values)
	}

	line := "\"my folder/te\""
	cursor := len(line) - 1
	result, err = provider.GetSuggestions([]string{line}, 0, cursor, true)
	if err != nil {
		t.Fatal(err)
	}
	item := findSuggestion(result, "\"my folder/test.txt\"")
	if item == nil {
		t.Fatalf("missing quoted file suggestion: %#v", suggestionValues(result))
	}
	applied := provider.ApplyCompletion([]string{line}, 0, cursor, *item, result.Prefix)
	if applied.Lines[0] != "\"my folder/test.txt\"" {
		t.Fatalf("quoted apply = %#v", applied)
	}

	result, err = provider.GetSuggestions([]string{"./up"}, 0, len("./up"), true)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(suggestionValues(result), "./update.sh") {
		t.Fatalf("dot slash suggestions = %#v", suggestionValues(result))
	}

	result, err = provider.GetSuggestions([]string{"@my"}, 0, len("@my"), false)
	if err != nil {
		t.Fatal(err)
	}
	values = suggestionValues(result)
	if !contains(values, "@\"my folder/\"") {
		t.Fatalf("quoted @ folder suggestion = %#v", values)
	}

	atLine := "@\"my folder/\""
	atCursor := len(atLine) - 1
	result, err = provider.GetSuggestions([]string{atLine}, 0, atCursor, false)
	if err != nil {
		t.Fatal(err)
	}
	values = suggestionValues(result)
	if !contains(values, "@\"my folder/test.txt\"") || !contains(values, "@\"my folder/other.txt\"") {
		t.Fatalf("quoted @ path continuation = %#v", values)
	}

	atLine = "@\"my folder/te\""
	atCursor = len(atLine) - 1
	result, err = provider.GetSuggestions([]string{atLine}, 0, atCursor, false)
	if err != nil {
		t.Fatal(err)
	}
	item = findSuggestion(result, "@\"my folder/test.txt\"")
	if item == nil {
		t.Fatalf("missing quoted @ file suggestion: %#v", suggestionValues(result))
	}
	applied = provider.ApplyCompletion([]string{atLine}, 0, atCursor, *item, result.Prefix)
	if applied.Lines[0] != "@\"my folder/test.txt\" " {
		t.Fatalf("quoted @ apply = %#v", applied)
	}
}

func TestCombinedAutocompleteForcedPathExtractionPiMatrix(t *testing.T) {
	base := t.TempDir()
	mustWrite(t, filepath.Join(base, "model"), "relative model file")
	mustWrite(t, filepath.Join(base, "src", "index.ts"), "export {}")
	provider := NewCombinedAutocompleteProviderWithCommands(base, []SlashCommand{{Name: "model"}})

	result, err := provider.GetSuggestions([]string{"hey /"}, 0, len("hey /"), true)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Prefix != "/" {
		t.Fatalf("forced root path extraction = %#v, want prefix /", result)
	}

	result, err = provider.GetSuggestions([]string{"/command /"}, 0, len("/command /"), true)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Prefix != "/" {
		t.Fatalf("forced command-argument root path extraction = %#v, want prefix /", result)
	}

	result, err = provider.GetSuggestions([]string{"/model"}, 0, len("/model"), true)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatalf("forced slash command name should not resolve relative model file, got %#v", result)
	}

	result, err = provider.GetSuggestions([]string{"./sr"}, 0, len("./sr"), true)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(suggestionValues(result), "./src/") {
		t.Fatalf("dot-slash directory suggestions = %#v", suggestionValues(result))
	}
}

func TestCombinedAutocompleteShouldTriggerFileCompletionPiSemantics(t *testing.T) {
	provider := NewCombinedAutocompleteProviderWithCommands(t.TempDir(), []SlashCommand{{Name: "model"}})
	if provider.ShouldTriggerFileCompletion([]string{"/mo"}, 0, len("/mo")) {
		t.Fatalf("slash command name should not trigger file completion")
	}
	for _, line := range []string{"he", "open he", "/model he", "@he", "./he"} {
		if !provider.ShouldTriggerFileCompletion([]string{line}, 0, len(line)) {
			t.Fatalf("line %q should allow forced file completion", line)
		}
	}
}

func TestCombinedAutocompleteForceTreatsMissingCursorLineAsEmptyLikePi(t *testing.T) {
	base := t.TempDir()
	mustWrite(t, filepath.Join(base, "README.md"), "readme")
	provider := NewCombinedAutocompleteProviderWithCommands(base, nil)

	result, err := provider.GetSuggestions(nil, 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Prefix != "" || !contains(suggestionValues(result), "README.md") {
		t.Fatalf("forced completion on missing cursor line = %#v, want root file suggestion", result)
	}

	result, err = provider.GetSuggestions([]string{"ignored"}, 3, 12, true)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Prefix != "" || !contains(suggestionValues(result), "README.md") {
		t.Fatalf("forced completion on out-of-range cursor line = %#v, want empty-line root suggestion", result)
	}
}

func TestCombinedAutocompleteUnicodeCursorColumnsLikePi(t *testing.T) {
	base := t.TempDir()
	mustWrite(t, filepath.Join(base, "readme.md"), "readme")
	provider := NewCombinedAutocompleteProviderWithCommands(base, []SlashCommand{
		{
			Name: "model",
			GetArgumentCompletions: func(prefix string) []AutocompleteItem {
				if prefix == "α" {
					return []AutocompleteItem{{Value: "αβ-model", Label: "αβ-model"}}
				}
				return nil
			},
		},
	})

	line := "看 @read"
	cursor := len([]rune(line))
	result, err := provider.GetSuggestions([]string{line}, 0, cursor, false)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Prefix != "@read" || result.Start != len([]rune("看 ")) || result.End != cursor {
		t.Fatalf("unicode @ suggestions range = %#v", result)
	}
	item := findSuggestion(result, "@readme.md")
	if item == nil {
		t.Fatalf("unicode @ suggestions missing file: %#v", suggestionValues(result))
	}

	applyLine := "看 @rea end"
	applyCursor := len([]rune("看 @rea"))
	applied := provider.ApplyCompletion([]string{applyLine}, 0, applyCursor, *item, "@rea")
	if applied.Lines[0] != "看 @readme.md  end" || applied.CursorCol != len([]rune("看 @readme.md ")) {
		t.Fatalf("unicode @ apply = %#v", applied)
	}
	if !provider.ShouldTriggerFileCompletion([]string{line}, 0, cursor) {
		t.Fatalf("unicode @ line should allow forced file completion")
	}

	slashLine := "/model α"
	slashCursor := len([]rune(slashLine))
	result, err = provider.GetSuggestions([]string{slashLine}, 0, slashCursor, false)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Prefix != "α" || result.Start != len([]rune("/model ")) || result.End != slashCursor {
		t.Fatalf("unicode slash argument suggestions range = %#v", result)
	}
	applied = provider.ApplyCompletion([]string{slashLine}, 0, slashCursor, result.Items[0], result.Prefix)
	if applied.Lines[0] != "/model αβ-model" || applied.CursorCol != len([]rune("/model αβ-model")) {
		t.Fatalf("unicode slash argument apply = %#v", applied)
	}
}

func TestCombinedAutocompleteForcedAbsolutePathAtLineStart(t *testing.T) {
	tempRoot := filepath.VolumeName(os.TempDir())
	rest := strings.TrimPrefix(os.TempDir(), tempRoot)
	segments := strings.FieldsFunc(rest, func(r rune) bool { return os.IsPathSeparator(uint8(r)) })
	if len(segments) == 0 {
		t.Skipf("cannot derive absolute path segment from temp dir %q", os.TempDir())
	}
	prefix := tempRoot + string(os.PathSeparator) + segments[0]
	if _, err := os.Stat(prefix); err != nil {
		t.Skipf("absolute prefix %q is not readable: %v", prefix, err)
	}

	provider := NewCombinedAutocompleteProviderWithCommands(t.TempDir(), []SlashCommand{{Name: strings.TrimPrefix(prefix, string(os.PathSeparator))}})
	result, err := provider.GetSuggestions([]string{prefix}, 0, len(prefix), true)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Prefix != prefix {
		t.Fatalf("forced absolute path prefix result = %#v, want prefix %q", result, prefix)
	}
}

func TestCombinedAutocompleteFuzzyFullPathAndRelativeScope(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "cwd")
	outside := filepath.Join(root, "outside")
	mustWrite(t, filepath.Join(base, "packages", "tui", "src", "autocomplete.ts"), "export {};")
	mustWrite(t, filepath.Join(base, "packages", "ai", "src", "autocomplete.ts"), "export {};")
	mustWrite(t, filepath.Join(base, "src", "components", "Button.tsx"), "export {};")
	mustWrite(t, filepath.Join(base, "src", "utils", "helpers.ts"), "export {};")
	mustWrite(t, filepath.Join(outside, "nested", "alpha.ts"), "export {};")
	mustWrite(t, filepath.Join(outside, "nested", "deeper", "also-alpha.ts"), "export {};")
	mustWrite(t, filepath.Join(outside, "nested", "deeper", "zzz.ts"), "export {};")
	provider := NewCombinedAutocompleteProviderWithCommands(base, nil)

	result, err := provider.GetSuggestions([]string{"@tui/src/auto"}, 0, len("@tui/src/auto"), false)
	if err != nil {
		t.Fatal(err)
	}
	values := suggestionValues(result)
	if !contains(values, "@packages/tui/src/autocomplete.ts") {
		t.Fatalf("deep fuzzy path missing tui result: %#v", values)
	}
	if contains(values, "@packages/ai/src/autocomplete.ts") {
		t.Fatalf("deep fuzzy path should not include ai result: %#v", values)
	}

	result, err = provider.GetSuggestions([]string{"@components/"}, 0, len("@components/"), false)
	if err != nil {
		t.Fatal(err)
	}
	values = suggestionValues(result)
	if !contains(values, "@src/components/Button.tsx") || contains(values, "@src/utils/helpers.ts") {
		t.Fatalf("middle-directory fuzzy path values = %#v", values)
	}

	result, err = provider.GetSuggestions([]string{"@../outside/a"}, 0, len("@../outside/a"), false)
	if err != nil {
		t.Fatal(err)
	}
	values = suggestionValues(result)
	if !contains(values, "@../outside/nested/alpha.ts") ||
		!contains(values, "@../outside/nested/deeper/also-alpha.ts") ||
		contains(values, "@../outside/nested/deeper/zzz.ts") {
		t.Fatalf("relative scoped fuzzy values = %#v", values)
	}
}

func TestCombinedAutocompleteFollowsSymlinksLikeFd(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "cwd")
	outside := filepath.Join(root, "outside")
	mustWrite(t, filepath.Join(base, "dir", "some_file.txt"), "real")
	mustWrite(t, filepath.Join(base, "original.txt"), "content")
	mustWrite(t, filepath.Join(outside, "nested", "some_file.txt"), "symlinked")
	if err := os.Symlink("../outside", filepath.Join(base, "symlinked_dir")); err != nil {
		t.Skipf("symlinked directories are unavailable: %v", err)
	}
	if err := os.Symlink("original.txt", filepath.Join(base, "link.txt")); err != nil {
		t.Skipf("symlinked files are unavailable: %v", err)
	}
	provider := NewCombinedAutocompleteProviderWithCommands(base, nil)

	result, err := provider.GetSuggestions([]string{"@some"}, 0, len("@some"), false)
	if err != nil {
		t.Fatal(err)
	}
	values := suggestionValues(result)
	if !contains(values, "@dir/some_file.txt") || !contains(values, "@symlinked_dir/nested/some_file.txt") {
		t.Fatalf("fuzzy symlink values = %#v", values)
	}

	result, err = provider.GetSuggestions([]string{"@symlinked"}, 0, len("@symlinked"), false)
	if err != nil {
		t.Fatal(err)
	}
	values = suggestionValues(result)
	if !contains(values, "@symlinked_dir/") {
		t.Fatalf("symlinked directory suggestion missing: %#v", values)
	}

	result, err = provider.GetSuggestions([]string{"@link"}, 0, len("@link"), false)
	if err != nil {
		t.Fatal(err)
	}
	values = suggestionValues(result)
	if !contains(values, "@link.txt") {
		t.Fatalf("symlinked file suggestion missing: %#v", values)
	}
}

func TestCombinedAutocompleteFuzzyResultsIgnoreQueryInBasePath(t *testing.T) {
	root := t.TempDir()
	normalBase := filepath.Join(root, "cwd-normal")
	queryInPathBase := filepath.Join(root, "cwd-plan-repro")
	for _, base := range []string{normalBase, queryInPathBase} {
		mustWrite(t, filepath.Join(base, "packages", "coding-agent", "examples", "extensions", "plan-mode", "README.md"), "readme")
		mustWrite(t, filepath.Join(base, "packages", "web-ui", "docs", "plan.md"), "plan")
	}

	normalProvider := NewCombinedAutocompleteProviderWithCommands(normalBase, nil)
	queryInPathProvider := NewCombinedAutocompleteProviderWithCommands(queryInPathBase, nil)
	normalResult, err := normalProvider.GetSuggestions([]string{"@plan"}, 0, len("@plan"), false)
	if err != nil {
		t.Fatal(err)
	}
	queryInPathResult, err := queryInPathProvider.GetSuggestions([]string{"@plan"}, 0, len("@plan"), false)
	if err != nil {
		t.Fatal(err)
	}
	normal := suggestionLabelsAndDescriptions(normalResult)
	queryInPath := suggestionLabelsAndDescriptions(queryInPathResult)
	if !reflect.DeepEqual(queryInPath, normal) {
		t.Fatalf("base path query should not affect fuzzy suggestions\nnormal=%#v\nqueryInPath=%#v", normal, queryInPath)
	}
	if !contains(normal, "plan-mode/ :: packages/coding-agent/examples/extensions/plan-mode") ||
		!contains(normal, "plan.md :: packages/web-ui/docs") {
		t.Fatalf("missing expected plan suggestions: %#v", normal)
	}
}

func mustWrite(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func suggestionValues(result *AutocompleteSuggestions) []string {
	if result == nil {
		return nil
	}
	values := make([]string, len(result.Items))
	for i, item := range result.Items {
		values[i] = item.Value
	}
	sort.Strings(values)
	return values
}

func suggestionValuesInOrder(result *AutocompleteSuggestions) []string {
	if result == nil {
		return nil
	}
	values := make([]string, len(result.Items))
	for i, item := range result.Items {
		values[i] = item.Value
	}
	return values
}

func suggestionLabelsAndDescriptions(result *AutocompleteSuggestions) []string {
	if result == nil {
		return nil
	}
	values := make([]string, len(result.Items))
	for i, item := range result.Items {
		values[i] = item.Label + " :: " + item.Description
	}
	sort.Strings(values)
	return values
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func findSuggestion(result *AutocompleteSuggestions, value string) *AutocompleteItem {
	if result == nil {
		return nil
	}
	for i := range result.Items {
		if result.Items[i].Value == value {
			return &result.Items[i]
		}
	}
	return nil
}
