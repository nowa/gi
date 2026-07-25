package gicodingagent

import (
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestResolveConfigValueTemplatesAndEscapes(t *testing.T) {
	t.Setenv("TEST_CONFIG_LEFT", "left")
	t.Setenv("TEST_CONFIG_RIGHT", "right")

	tests := []struct {
		name   string
		config string
		want   string
		ok     bool
	}{
		{name: "literal", config: "literal-key", want: "literal-key", ok: true},
		{name: "unbraced environment", config: "$TEST_CONFIG_LEFT", want: "left", ok: true},
		{name: "mixed template", config: "${TEST_CONFIG_LEFT}_$TEST_CONFIG_RIGHT", want: "left_right", ok: true},
		{name: "escaped dollar", config: "$$TEST_CONFIG_LEFT", want: "$TEST_CONFIG_LEFT", ok: true},
		{name: "escaped command marker", config: "$!literal-$TEST_CONFIG_RIGHT", want: "!literal-right", ok: true},
		{name: "missing environment", config: "$TEST_CONFIG_MISSING", ok: false},
		{name: "invalid braced name remains literal", config: "${BAD-NAME}", want: "${BAD-NAME}", ok: true},
		{name: "unterminated brace remains literal", config: "${TEST_CONFIG_LEFT", want: "${TEST_CONFIG_LEFT", ok: true},
		{name: "invalid unbraced name remains literal", config: "$9", want: "$9", ok: true},
		{name: "trailing dollar remains literal", config: "key$", want: "key$", ok: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := ResolveConfigValue(test.config)
			if got != test.want || ok != test.ok {
				t.Fatalf("ResolveConfigValue(%q) = %q, %v; want %q, %v", test.config, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestResolveConfigValueUsesCredentialEnvironmentFirst(t *testing.T) {
	t.Setenv("TEST_CONFIG_SCOPED", "process")
	env := llm.ProviderEnv{"TEST_CONFIG_SCOPED": "credential"}

	if got, ok := ResolveConfigValueWithEnv("$TEST_CONFIG_SCOPED", env); !ok || got != "credential" {
		t.Fatalf("scoped resolution = %q, %v", got, ok)
	}
	env["TEST_CONFIG_SCOPED"] = ""
	if got, ok := ResolveConfigValueWithEnv("$TEST_CONFIG_SCOPED", env); !ok || got != "process" {
		t.Fatalf("empty scoped fallback = %q, %v", got, ok)
	}
}

func TestResolveConfigValueTreatsBareEnvironmentNamesAsLiterals(t *testing.T) {
	t.Setenv("TEST_CONFIG_LEGACY", "legacy-value")
	env := llm.ProviderEnv{"TEST_CONFIG_SCOPED_LEGACY": "scoped-value"}

	if got, ok := ResolveConfigValue("TEST_CONFIG_LEGACY"); !ok || got != "TEST_CONFIG_LEGACY" {
		t.Fatalf("process bare literal = %q, %v", got, ok)
	}
	if got, ok := ResolveConfigValueWithEnv("TEST_CONFIG_SCOPED_LEGACY", env); !ok || got != "TEST_CONFIG_SCOPED_LEGACY" {
		t.Fatalf("scoped bare literal = %q, %v", got, ok)
	}
}

func TestResolveConfigValueReferenceHelpers(t *testing.T) {
	if name, ok := ConfigValueEnvVarName("$ONLY"); !ok || name != "ONLY" {
		t.Fatalf("single name = %q, %v", name, ok)
	}
	if _, ok := ConfigValueEnvVarName("prefix-$ONLY"); ok {
		t.Fatal("mixed template reported as a single environment reference")
	}

	config := "$FIRST/${SECOND}/$FIRST/$THIRD"
	if got, want := ConfigValueEnvVarNames(config), []string{"FIRST", "SECOND", "THIRD"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("environment names = %#v, want %#v", got, want)
	}
	t.Setenv("SECOND", "process")
	env := llm.ProviderEnv{"FIRST": "scoped"}
	if got, want := MissingConfigValueEnvVarNames(config, env), []string{"THIRD"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing names = %#v, want %#v", got, want)
	}
	if IsConfigValueConfigured(config, env) {
		t.Fatal("template with a missing variable reported configured")
	}
	env["THIRD"] = "scoped"
	if !IsConfigValueConfigured(config, env) {
		t.Fatal("fully resolvable template reported unconfigured")
	}
	if !IsCommandConfigValue("!printf value") || IsCommandConfigValue("$COMMAND") {
		t.Fatal("command detection did not match the leading marker")
	}
	if !IsConfigValueConfigured("!exit 1", nil) {
		t.Fatal("configured check executed or rejected a command")
	}
}

func TestResolveConfigValueCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test commands use POSIX shell syntax")
	}
	ClearConfigValueCache()
	t.Cleanup(ClearConfigValueCache)

	if got, ok := ResolveConfigValue("!echo '  spaced-key  '"); !ok || got != "spaced-key" {
		t.Fatalf("trimmed command = %q, %v", got, ok)
	}
	if got, ok := ResolveConfigValue("!printf 'line1\\nline2'"); !ok || got != "line1\nline2" {
		t.Fatalf("multiline command = %q, %v", got, ok)
	}
	if got, ok := ResolveConfigValue("!echo 'hello world' | tr ' ' '-'"); !ok || got != "hello-world" {
		t.Fatalf("pipeline command = %q, %v", got, ok)
	}
	for _, command := range []string{"!exit 1", "!nonexistent-command-12345", "!printf ''"} {
		if got, ok := ResolveConfigValue(command); ok || got != "" {
			t.Fatalf("failed command %q = %q, %v", command, got, ok)
		}
	}
}

func TestResolveConfigValueCachesSuccessfulAndFailedCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test commands use POSIX shell syntax")
	}
	ClearConfigValueCache()
	t.Cleanup(ClearConfigValueCache)

	successCounter := writeCounterFile(t)
	success := incrementCounterCommand(successCounter, "value", false)
	for range 2 {
		if got, ok := ResolveConfigValue(success); !ok || got != "value" {
			t.Fatalf("successful command = %q, %v", got, ok)
		}
	}
	if got := readCounterFile(t, successCounter); got != 1 {
		t.Fatalf("successful command executions = %d, want 1", got)
	}

	failureCounter := writeCounterFile(t)
	failure := incrementCounterCommand(failureCounter, "", true)
	for range 2 {
		if got, ok := ResolveConfigValue(failure); ok || got != "" {
			t.Fatalf("failed command = %q, %v", got, ok)
		}
	}
	if got := readCounterFile(t, failureCounter); got != 1 {
		t.Fatalf("failed command executions = %d, want 1", got)
	}

	ClearConfigValueCache()
	if _, ok := ResolveConfigValue(failure); ok {
		t.Fatal("failed command resolved after cache clear")
	}
	if got := readCounterFile(t, failureCounter); got != 2 {
		t.Fatalf("failed command executions after clear = %d, want 2", got)
	}
}

func TestResolveConfigValueCommandCacheCoalescesConcurrentReads(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test commands use POSIX shell syntax")
	}
	ClearConfigValueCache()
	t.Cleanup(ClearConfigValueCache)
	counter := writeCounterFile(t)
	command := incrementCounterCommand(counter, "shared", false)

	var wait sync.WaitGroup
	results := make(chan string, 32)
	for range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			value, ok := ResolveConfigValue(command)
			if !ok {
				results <- "<unresolved>"
				return
			}
			results <- value
		}()
	}
	wait.Wait()
	close(results)
	for result := range results {
		if result != "shared" {
			t.Fatalf("concurrent result = %q", result)
		}
	}
	if got := readCounterFile(t, counter); got != 1 {
		t.Fatalf("concurrent command executions = %d, want 1", got)
	}
}

func TestResolveConfigValueUncachedExecutesEveryCall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test commands use POSIX shell syntax")
	}
	counter := writeCounterFile(t)
	command := incrementCounterCommand(counter, "value", false)

	for range 2 {
		if got, ok := ResolveConfigValueUncached(command); !ok || got != "value" {
			t.Fatalf("uncached command = %q, %v", got, ok)
		}
	}
	if got := readCounterFile(t, counter); got != 2 {
		t.Fatalf("uncached command executions = %d, want 2", got)
	}
}

func TestResolveConfigValueErrorsAndHeaders(t *testing.T) {
	_, err := ResolveConfigValueOrError(
		"$MISSING_ONE/$MISSING_TWO",
		"API key",
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "environment variables: MISSING_ONE, MISSING_TWO") {
		t.Fatalf("missing environment error = %v", err)
	}

	t.Setenv("HEADER_TOKEN", "token")
	headers := map[string]string{
		"Authorization": "Bearer $HEADER_TOKEN",
		"Missing":       "$MISSING_HEADER",
		"Empty":         "",
	}
	if got, want := ResolveConfigHeaders(headers, nil), map[string]string{"Authorization": "Bearer token"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved headers = %#v, want %#v", got, want)
	}
	if _, err := ResolveConfigHeadersOrError(headers, "provider", nil); err == nil ||
		!strings.Contains(err.Error(), `provider header "Missing"`) {
		t.Fatalf("strict header error = %v", err)
	}
}
