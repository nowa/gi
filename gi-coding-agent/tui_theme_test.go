package gicodingagent

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func TestTUIThemeForegroundTokensMatchPiDarkTheme(t *testing.T) {
	setTUIThemeForTest(t, "dark", nil)
	expected := map[string]string{
		"accent":             "\x1b[38;2;138;190;183m",
		"border":             "\x1b[38;2;95;135;255m",
		"borderAccent":       "\x1b[38;2;0;215;255m",
		"borderMuted":        "\x1b[38;2;80;80;80m",
		"success":            "\x1b[38;2;181;189;104m",
		"error":              "\x1b[38;2;204;102;102m",
		"warning":            "\x1b[38;2;255;255;0m",
		"muted":              "\x1b[38;2;128;128;128m",
		"dim":                "\x1b[38;2;102;102;102m",
		"text":               tuiResetFG,
		"thinkingText":       "\x1b[38;2;128;128;128m",
		"userMessageText":    tuiResetFG,
		"customMessageText":  tuiResetFG,
		"customMessageLabel": "\x1b[38;2;149;117;205m",
		"toolTitle":          tuiResetFG,
		"toolOutput":         "\x1b[38;2;128;128;128m",
		"mdHeading":          "\x1b[38;2;240;198;116m",
		"mdLink":             "\x1b[38;2;129;162;190m",
		"mdLinkUrl":          "\x1b[38;2;102;102;102m",
		"mdCode":             "\x1b[38;2;138;190;183m",
		"mdCodeBlock":        "\x1b[38;2;181;189;104m",
		"mdCodeBlockBorder":  "\x1b[38;2;128;128;128m",
		"mdQuote":            "\x1b[38;2;128;128;128m",
		"mdQuoteBorder":      "\x1b[38;2;128;128;128m",
		"mdHr":               "\x1b[38;2;128;128;128m",
		"mdListBullet":       "\x1b[38;2;138;190;183m",
		"toolDiffAdded":      "\x1b[38;2;181;189;104m",
		"toolDiffRemoved":    "\x1b[38;2;204;102;102m",
		"toolDiffContext":    "\x1b[38;2;128;128;128m",
		"syntaxComment":      "\x1b[38;2;106;153;85m",
		"syntaxKeyword":      "\x1b[38;2;86;156;214m",
		"syntaxFunction":     "\x1b[38;2;220;220;170m",
		"syntaxVariable":     "\x1b[38;2;156;220;254m",
		"syntaxString":       "\x1b[38;2;206;145;120m",
		"syntaxNumber":       "\x1b[38;2;181;206;168m",
		"syntaxType":         "\x1b[38;2;78;201;176m",
		"syntaxOperator":     "\x1b[38;2;212;212;212m",
		"syntaxPunctuation":  "\x1b[38;2;212;212;212m",
		"thinkingOff":        "\x1b[38;2;80;80;80m",
		"thinkingMinimal":    "\x1b[38;2;110;110;110m",
		"thinkingLow":        "\x1b[38;2;95;135;175m",
		"thinkingMedium":     "\x1b[38;2;129;162;190m",
		"thinkingHigh":       "\x1b[38;2;178;148;187m",
		"thinkingXhigh":      "\x1b[38;2;209;131;232m",
		"bashMode":           "\x1b[38;2;181;189;104m",
	}
	for color, prefix := range expected {
		t.Run(color, func(t *testing.T) {
			got := tuiThemeFG(color, "sample")
			want := prefix + "sample" + tuiResetFG
			if got != want {
				t.Fatalf("tuiThemeFG(%q) = %q, want %q", color, got, want)
			}
		})
	}
}

func TestTUIThemeBackgroundTokensMatchPiDarkTheme(t *testing.T) {
	setTUIThemeForTest(t, "dark", nil)
	expected := map[string]string{
		"selectedBg":      "\x1b[48;2;58;58;74m",
		"userMessageBg":   "\x1b[48;2;52;53;65m",
		"customMessageBg": "\x1b[48;2;45;40;56m",
		"toolPendingBg":   "\x1b[48;2;40;40;50m",
		"toolSuccessBg":   "\x1b[48;2;40;50;40m",
		"toolErrorBg":     "\x1b[48;2;60;40;40m",
	}
	for color, prefix := range expected {
		t.Run(color, func(t *testing.T) {
			got := tuiThemeBG(color, "sample")
			want := prefix + "sample" + tuiResetBG
			if got != want {
				t.Fatalf("tuiThemeBG(%q) = %q, want %q", color, got, want)
			}
		})
	}
}

func TestTUIThemeForegroundTokensMatchPiLightTheme(t *testing.T) {
	setTUIThemeForTest(t, "light", nil)
	expected := map[string]string{
		"accent":             "\x1b[38;2;90;128;128m",
		"border":             "\x1b[38;2;84;125;167m",
		"borderAccent":       "\x1b[38;2;90;128;128m",
		"borderMuted":        "\x1b[38;2;176;176;176m",
		"success":            "\x1b[38;2;88;132;88m",
		"error":              "\x1b[38;2;170;85;85m",
		"warning":            "\x1b[38;2;154;115;38m",
		"muted":              "\x1b[38;2;108;108;108m",
		"dim":                "\x1b[38;2;118;118;118m",
		"text":               tuiResetFG,
		"thinkingText":       "\x1b[38;2;108;108;108m",
		"userMessageText":    tuiResetFG,
		"customMessageText":  tuiResetFG,
		"customMessageLabel": "\x1b[38;2;126;87;194m",
		"toolTitle":          tuiResetFG,
		"toolOutput":         "\x1b[38;2;108;108;108m",
		"mdHeading":          "\x1b[38;2;154;115;38m",
		"mdLink":             "\x1b[38;2;84;125;167m",
		"mdLinkUrl":          "\x1b[38;2;118;118;118m",
		"mdCode":             "\x1b[38;2;90;128;128m",
		"mdCodeBlock":        "\x1b[38;2;88;132;88m",
		"mdCodeBlockBorder":  "\x1b[38;2;108;108;108m",
		"mdQuote":            "\x1b[38;2;108;108;108m",
		"mdQuoteBorder":      "\x1b[38;2;108;108;108m",
		"mdHr":               "\x1b[38;2;108;108;108m",
		"mdListBullet":       "\x1b[38;2;88;132;88m",
		"toolDiffAdded":      "\x1b[38;2;88;132;88m",
		"toolDiffRemoved":    "\x1b[38;2;170;85;85m",
		"toolDiffContext":    "\x1b[38;2;108;108;108m",
		"syntaxComment":      "\x1b[38;2;0;128;0m",
		"syntaxKeyword":      "\x1b[38;2;0;0;255m",
		"syntaxFunction":     "\x1b[38;2;121;94;38m",
		"syntaxVariable":     "\x1b[38;2;0;16;128m",
		"syntaxString":       "\x1b[38;2;163;21;21m",
		"syntaxNumber":       "\x1b[38;2;9;134;88m",
		"syntaxType":         "\x1b[38;2;38;127;153m",
		"syntaxOperator":     "\x1b[38;2;0;0;0m",
		"syntaxPunctuation":  "\x1b[38;2;0;0;0m",
		"thinkingOff":        "\x1b[38;2;176;176;176m",
		"thinkingMinimal":    "\x1b[38;2;118;118;118m",
		"thinkingLow":        "\x1b[38;2;84;125;167m",
		"thinkingMedium":     "\x1b[38;2;90;128;128m",
		"thinkingHigh":       "\x1b[38;2;135;95;135m",
		"thinkingXhigh":      "\x1b[38;2;139;0;139m",
		"bashMode":           "\x1b[38;2;88;132;88m",
	}
	for color, prefix := range expected {
		t.Run(color, func(t *testing.T) {
			got := tuiThemeFG(color, "sample")
			want := prefix + "sample" + tuiResetFG
			if got != want {
				t.Fatalf("tuiThemeFG(%q) = %q, want %q", color, got, want)
			}
		})
	}
}

func TestTUIThemeBackgroundTokensMatchPiLightTheme(t *testing.T) {
	setTUIThemeForTest(t, "light", nil)
	expected := map[string]string{
		"selectedBg":      "\x1b[48;2;208;208;224m",
		"userMessageBg":   "\x1b[48;2;232;232;232m",
		"customMessageBg": "\x1b[48;2;237;231;246m",
		"toolPendingBg":   "\x1b[48;2;232;232;240m",
		"toolSuccessBg":   "\x1b[48;2;232;240;232m",
		"toolErrorBg":     "\x1b[48;2;240;232;232m",
	}
	for color, prefix := range expected {
		t.Run(color, func(t *testing.T) {
			got := tuiThemeBG(color, "sample")
			want := prefix + "sample" + tuiResetBG
			if got != want {
				t.Fatalf("tuiThemeBG(%q) = %q, want %q", color, got, want)
			}
		})
	}
}

func TestTUIThemeLoadsCustomThemeTokens(t *testing.T) {
	themePath := filepath.Join(t.TempDir(), "focus.json")
	writeJSON(t, themePath, completeTUIThemeFixture("focus", map[string]any{
		"accent":        "#112233",
		"selectedBg":    "#445566",
		"toolOutput":    245,
		"userMessageBg": 238,
	}))
	setTUIThemeForTest(t, "focus", []TUIThemeInfo{{Name: "focus", Path: themePath}})
	if got, want := tuiThemeAccent("x"), "\x1b[38;2;17;34;51mx"+tuiResetFG; got != want {
		t.Fatalf("accent = %q, want %q", got, want)
	}
	if got, want := tuiThemeBG("selectedBg", "x"), "\x1b[48;2;68;85;102mx"+tuiResetBG; got != want {
		t.Fatalf("selectedBg = %q, want %q", got, want)
	}
	if got, want := tuiThemeToolOutput("x"), "\x1b[38;5;245mx"+tuiResetFG; got != want {
		t.Fatalf("toolOutput = %q, want %q", got, want)
	}
	if got, want := tuiThemeBG("userMessageBg", "x"), "\x1b[48;5;238mx"+tuiResetBG; got != want {
		t.Fatalf("userMessageBg = %q, want %q", got, want)
	}
}

func TestTUIThemeSchemaMatchesPiThemeContract(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(TUIThemeSchemaJSON(), &schema); err != nil {
		t.Fatal(err)
	}
	if got, want := schema["title"], "Gi Coding Agent Theme"; got != want {
		t.Fatalf("title = %v, want %q", got, want)
	}
	if got, want := schemaStringSlice(t, schema["required"], "required"), []string{"name", "colors"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required = %#v, want %#v", got, want)
	}
	if got := schema["additionalProperties"]; got != false {
		t.Fatalf("additionalProperties = %v, want false", got)
	}

	properties := schemaObject(t, schema["properties"], "properties")
	colors := schemaObject(t, properties["colors"], "properties.colors")
	if got := colors["additionalProperties"]; got != false {
		t.Fatalf("colors.additionalProperties = %v, want false", got)
	}
	gotTokens := schemaStringSlice(t, colors["required"], "properties.colors.required")
	sort.Strings(gotTokens)
	wantTokens := tuiThemeSchemaColorTokens()
	if !reflect.DeepEqual(gotTokens, wantTokens) {
		t.Fatalf("schema color tokens = %#v, want %#v", gotTokens, wantTokens)
	}
	colorProperties := schemaObject(t, colors["properties"], "properties.colors.properties")
	if len(colorProperties) != len(wantTokens) {
		t.Fatalf("colors.properties has %d tokens, want %d", len(colorProperties), len(wantTokens))
	}
	for _, token := range wantTokens {
		if _, ok := colorProperties[token]; !ok {
			t.Fatalf("colors.properties missing token %q", token)
		}
	}

	export := schemaObject(t, properties["export"], "properties.export")
	if got := export["additionalProperties"]; got != false {
		t.Fatalf("export.additionalProperties = %v, want false", got)
	}
	exportProperties := schemaObject(t, export["properties"], "properties.export.properties")
	if got, want := sortedThemeSchemaMapKeys(exportProperties), []string{"cardBg", "infoBg", "pageBg"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("export.properties = %#v, want %#v", got, want)
	}

	defs := schemaObject(t, schema["$defs"], "$defs")
	colorValue := schemaObject(t, defs["colorValue"], "$defs.colorValue")
	oneOf := schemaArray(t, colorValue["oneOf"], "$defs.colorValue.oneOf")
	if len(oneOf) != 2 {
		t.Fatalf("colorValue.oneOf has %d entries, want 2", len(oneOf))
	}
	stringColor := schemaObject(t, oneOf[0], "$defs.colorValue.oneOf[0]")
	if got, want := stringColor["type"], "string"; got != want {
		t.Fatalf("colorValue string type = %v, want %q", got, want)
	}
	indexColor := schemaObject(t, oneOf[1], "$defs.colorValue.oneOf[1]")
	if got, want := indexColor["type"], "integer"; got != want {
		t.Fatalf("colorValue integer type = %v, want %q", got, want)
	}
	if got, want := indexColor["minimum"], float64(0); got != want {
		t.Fatalf("colorValue minimum = %v, want %v", got, want)
	}
	if got, want := indexColor["maximum"], float64(255); got != want {
		t.Fatalf("colorValue maximum = %v, want %v", got, want)
	}
}

func TestTUIThemeUsesPi256ColorFallbackForScreenTerm(t *testing.T) {
	t.Setenv("COLORTERM", "")
	t.Setenv("WT_SESSION", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("TERM", "screen-256color")
	gitui.ResetCapabilitiesCache()
	previous := tuiActiveThemeSnapshot()
	t.Cleanup(func() {
		tuiSetActiveThemePalette(previous)
		gitui.ResetCapabilitiesCache()
	})
	if err := tuiSetActiveTheme("dark", nil); err != nil {
		t.Fatal(err)
	}

	if got, want := tuiThemeAccent("x"), "\x1b[38;5;109mx"+tuiResetFG; got != want {
		t.Fatalf("accent = %q, want %q", got, want)
	}
	if got, want := tuiThemeMuted("x"), "\x1b[38;5;244mx"+tuiResetFG; got != want {
		t.Fatalf("muted = %q, want %q", got, want)
	}
	if got, want := tuiThemeBorder("x"), "\x1b[38;5;69mx"+tuiResetFG; got != want {
		t.Fatalf("border = %q, want %q", got, want)
	}
	if got, want := tuiThemeBG("userMessageBg", "x"), "\x1b[48;5;59mx"+tuiResetBG; got != want {
		t.Fatalf("userMessageBg = %q, want %q", got, want)
	}
}

func TestTUIThemeTruecolorOverridesScreenTermLikePi(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("WT_SESSION", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("TERM", "screen-256color")
	gitui.ResetCapabilitiesCache()
	previous := tuiActiveThemeSnapshot()
	t.Cleanup(func() {
		tuiSetActiveThemePalette(previous)
		gitui.ResetCapabilitiesCache()
	})
	if err := tuiSetActiveTheme("light", nil); err != nil {
		t.Fatal(err)
	}

	if got, want := tuiThemeAccent("x"), "\x1b[38;2;90;128;128mx"+tuiResetFG; got != want {
		t.Fatalf("accent = %q, want %q", got, want)
	}
}

func TestTUIThemeUserMessageUsesPiDefaultTextForeground(t *testing.T) {
	setTUIThemeForTest(t, "dark", nil)
	lines := newCLIUserMessageComponent("hello").Render(40)
	rendered := strings.Join(lines, "\n")
	if strings.Contains(rendered, "\x1b[38;2;212;212;212mhello") || !strings.Contains(rendered, tuiResetFG+"hello"+tuiResetFG) {
		t.Fatalf("user message should use Pi default foreground reset:\n%q", rendered)
	}
}

func TestTUIThemeUserMessageKeepsPiBackground(t *testing.T) {
	setTUIThemeForTest(t, "dark", nil)
	lines := newCLIUserMessageComponent("hello").Render(40)
	rendered := strings.Join(lines, "\n")
	if !strings.Contains(rendered, tuiDarkThemeBG["userMessageBg"]) {
		t.Fatalf("user message should keep Pi user message background:\n%q", rendered)
	}
}

func TestTUIThemeThinkingTextUsesPiThinkingColor(t *testing.T) {
	setTUIThemeForTest(t, "dark", nil)
	lines := renderCLIAssistantMessage(llm.Message{
		Role:    llm.RoleAssistant,
		Content: []llm.ContentPart{llm.Thinking("secret plan")},
	}, 40, true, "Thinking...")
	rendered := strings.Join(lines, "\n")
	if !strings.Contains(rendered, tuiThemeFG("thinkingText", "Thinking...")) {
		t.Fatalf("hidden thinking label should use Pi thinkingText color:\n%q", rendered)
	}
}

func setTUIThemeForTest(t *testing.T, name string, themes []TUIThemeInfo) {
	t.Helper()
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("WT_SESSION", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("TERM", "xterm-256color")
	gitui.ResetCapabilitiesCache()
	previous := tuiActiveThemeSnapshot()
	t.Cleanup(func() {
		tuiSetActiveThemePalette(previous)
		gitui.ResetCapabilitiesCache()
	})
	if err := tuiSetActiveTheme(name, themes); err != nil {
		t.Fatal(err)
	}
}

func completeTUIThemeFixture(name string, overrides map[string]any) map[string]any {
	colors := map[string]any{}
	for token := range tuiDarkThemeFG {
		colors[token] = "#010203"
	}
	for token := range tuiDarkThemeBG {
		colors[token] = "#040506"
	}
	colors["text"] = ""
	colors["userMessageText"] = ""
	colors["customMessageText"] = ""
	colors["toolTitle"] = ""
	for key, value := range overrides {
		colors[key] = value
	}
	return map[string]any{
		"name":   name,
		"colors": colors,
	}
}

func tuiThemeSchemaColorTokens() []string {
	tokens := make([]string, 0, len(tuiDarkThemeFG)+len(tuiDarkThemeBG))
	for token := range tuiDarkThemeFG {
		tokens = append(tokens, token)
	}
	for token := range tuiDarkThemeBG {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return tokens
}

func schemaObject(t *testing.T, value any, path string) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s is %T, want object", path, value)
	}
	return object
}

func schemaArray(t *testing.T, value any, path string) []any {
	t.Helper()
	array, ok := value.([]any)
	if !ok {
		t.Fatalf("%s is %T, want array", path, value)
	}
	return array
}

func schemaStringSlice(t *testing.T, value any, path string) []string {
	t.Helper()
	array := schemaArray(t, value, path)
	values := make([]string, 0, len(array))
	for index, item := range array {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("%s[%d] is %T, want string", path, index, item)
		}
		values = append(values, text)
	}
	return values
}

func sortedThemeSchemaMapKeys(input map[string]any) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
