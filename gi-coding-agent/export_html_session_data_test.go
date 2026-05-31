package gicodingagent

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestRenderSessionManagerHTMLUsesPiStyleSessionDataBootstrap(t *testing.T) {
	sm, err := InMemorySessionManager(`/tmp/raw-export-xss-marker<script>`)
	if err != nil {
		t.Fatal(err)
	}
	sm.AppendMessage(llm.UserMessageText(`hello raw-export-xss-marker<script>alert(1)</script>`))

	rendered := RenderSessionManagerHTML(sm)
	for _, expected := range []string{
		`id="session-data"`,
		`id="tree-container"`,
		`id="header-container"`,
		`id="messages"`,
		`id="hamburger"`,
		`id="sidebar-overlay"`,
		`id="sidebar-resizer"`,
		`id="sidebar-close"`,
		`class="filter-btn active" data-filter="default"`,
		`class="filter-btn" data-filter="no-tools"`,
		`class="filter-btn" data-filter="user-only"`,
		`class="filter-btn" data-filter="labeled-only"`,
		`class="filter-btn" data-filter="all"`,
		`decodeSessionData`,
		`setupSidebarControls`,
		`searchInput.addEventListener('input'`,
		`button.addEventListener('click'`,
		`filterMode = button.dataset.filter`,
		`readURLParams`,
		`buildActivePathIds`,
		`findNewestLeaf`,
		`navigateTo(findNewestLeaf(entry.id), entry.id)`,
		`params.get('leafId')`,
		`params.get('targetId')`,
		`getPath(entriesById, currentLeafId)`,
		`computeStats`,
		`downloadSessionJson`,
		`window.downloadSessionJson`,
		`download.className = 'download-json-btn'`,
		`download.textContent = 'Download JSONL'`,
		`buildShareURL`,
		`meta[name="pi-share-base-url"]`,
		`copyToClipboard`,
		`renderCopyLinkButton`,
		`button.className = 'copy-link-btn'`,
		`Copy link to this message`,
		`findToolResult`,
		`renderToolCall`,
		`wrapper.className = 'tool-execution '`,
		`message.role === 'toolResult') continue`,
		`appendExpandableOutput`,
		`appendToolHeader`,
		`case 'bash':`,
		`case 'read':`,
		`case 'write':`,
		`case 'edit':`,
		`case 'ls':`,
		`result.details.diff`,
		`const renderedTools = data.renderedTools || {}`,
		`appendTrustedHTML`,
		`rendered.resultHtmlCollapsed`,
		`tool-output expandable ansi-rendered`,
		`appendMarkdownBlock`,
		`appendInlineMarkdown`,
		`isSafeMarkdownURL`,
		`appendHighlightedCode`,
		`appendHighlightedText`,
		`normalizeHighlightLanguage`,
		`hljs-keyword`,
		`getLanguageFromPath`,
		`markdown-content`,
		`startsWith('javascript:')`,
		`String.fromCharCode(96).repeat(3)`,
		`setupSidebarResize`,
		`gi-share:v1:sidebar-width`,
		`setPointerCapture`,
		`setupMobileSidebar`,
		`sidebar.classList.add('open')`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("export HTML missing %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, `raw-export-xss-marker<script>`) {
		t.Fatalf("raw session content should only appear inside base64 session data:\n%s", rendered)
	}

	match := regexp.MustCompile(`<script id="session-data" type="application/json">([^<]+)</script>`).FindStringSubmatch(rendered)
	if match == nil {
		t.Fatalf("session data script missing:\n%s", rendered)
	}
	payload, err := base64.StdEncoding.DecodeString(match[1])
	if err != nil {
		t.Fatal(err)
	}
	var data ExportHTMLSessionData
	if err := json.Unmarshal(payload, &data); err != nil {
		t.Fatal(err)
	}
	if data.Header == nil || data.Header.CWD != `/tmp/raw-export-xss-marker<script>` {
		t.Fatalf("decoded header = %#v", data.Header)
	}
	if len(data.Entries) != 1 || data.Entries[0].Type != "message" || data.LeafID == nil || *data.LeafID != data.Entries[0].ID {
		t.Fatalf("decoded data = %#v", data)
	}
}

func TestExportHTMLTemplateCSSUsesPiDarkThemeTokens(t *testing.T) {
	for _, expected := range []string{
		`--accent: #8abeb7;`,
		`--success: #b5bd68;`,
		`--error: #cc6666;`,
		`--warning: #ffff00;`,
		`--dim: #666666;`,
		`--selectedBg: #3a3a4a;`,
		`--customMessageLabel: #9575cd;`,
		`--syntaxKeyword: #569CD6;`,
		`--syntaxString: #CE9178;`,
		`--syntaxNumber: #B5CEA8;`,
		`.tree-role-assistant { color: var(--success); }`,
		`.tree-role-compaction { color: var(--borderAccent); }`,
		`.tree-role-branch-summary { color: var(--warning); }`,
		`.diff-added { color: var(--success); }`,
		`.diff-removed { color: var(--error); }`,
	} {
		if !strings.Contains(ExportHTMLTemplateCSS, expected) {
			t.Fatalf("export CSS missing Pi theme token %q:\n%s", expected, ExportHTMLTemplateCSS)
		}
	}
	for _, stale := range []string{
		`#8ec07c`,
		`#b8bb26`,
		`#fb4934`,
		`#d3869b`,
		`#fabd2f`,
	} {
		if strings.Contains(ExportHTMLTemplateCSS, stale) {
			t.Fatalf("export CSS still contains stale non-Pi theme color %q:\n%s", stale, ExportHTMLTemplateCSS)
		}
	}
}

func TestRenderSessionManagerHTMLAppliesPiLightThemeVariables(t *testing.T) {
	sm, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sm.AppendMessage(llm.UserMessageText("hello"))

	rendered := RenderSessionManagerHTMLWithOptions(sm, ExportHTMLOptions{ThemeName: "light"})
	for _, expected := range []string{
		`--accent: #5a8080;`,
		`--text: #000000;`,
		`--userMessageBg: #e8e8e8;`,
		`--syntaxKeyword: #0000ff;`,
		`--exportPageBg: #f8f8f8;`,
		`--exportCardBg: #ffffff;`,
		`--exportInfoBg: #fffae6;`,
		`--body-bg: #f8f8f8;`,
		`--container-bg: #ffffff;`,
		`--info-bg: #fffae6;`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("light export HTML missing theme variable %q:\n%s", expected, rendered)
		}
	}
}

func TestRenderSessionManagerHTMLAppliesCustomThemeVariablesPiStyle(t *testing.T) {
	agentDir := t.TempDir()
	themesDir := filepath.Join(agentDir, "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	theme := `{
		"name": "focus",
		"vars": {
			"accentVar": "#112233",
			"userBg": "#445566",
			"ansiCard": 24
		},
		"colors": {
			"accent": "accentVar",
			"text": "",
			"userMessageBg": "userBg",
			"syntaxString": "ansiCard"
		},
		"export": {
			"pageBg": "#101010",
			"cardBg": "ansiCard",
			"infoBg": "userBg"
		}
	}`
	if err := os.WriteFile(filepath.Join(themesDir, "focus.json"), []byte(theme), 0o644); err != nil {
		t.Fatal(err)
	}
	sm, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sm.AppendMessage(llm.UserMessageText("hello"))

	rendered := RenderSessionManagerHTMLWithOptions(sm, ExportHTMLOptions{ThemeName: "focus", AgentDir: agentDir})
	for _, expected := range []string{
		`--accent: #112233;`,
		`--text: #e5e5e7;`,
		`--userMessageBg: #445566;`,
		`--syntaxString: #005f87;`,
		`--exportPageBg: #101010;`,
		`--exportCardBg: #005f87;`,
		`--exportInfoBg: #445566;`,
		`--body-bg: #101010;`,
		`--container-bg: #005f87;`,
		`--info-bg: #445566;`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("custom export HTML missing theme variable %q:\n%s", expected, rendered)
		}
	}
}

func TestBuildExportHTMLSessionDataPreRendersCustomToolsPiStyle(t *testing.T) {
	sm, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sm.AppendMessage(sessionMessageValue(llm.Message{
		Role: llm.RoleAssistant,
		Content: []llm.ContentPart{
			llm.ToolCall("custom-call-1", "custom_tool", map[string]any{"value": "alpha"}),
			llm.ToolCall("builtin-call-1", "bash", map[string]any{"command": "echo alpha"}),
		},
	}))
	sm.AppendMessage(sessionMessageValue(llm.Message{
		Role:       llm.RoleToolResult,
		ToolCallID: "custom-call-1",
		ToolName:   "custom_tool",
		Content:    []llm.ContentPart{llm.Text("custom result")},
	}))
	sm.AppendMessage(sessionMessageValue(llm.Message{
		Role:       llm.RoleToolResult,
		ToolCallID: "builtin-call-1",
		ToolName:   "bash",
		Content:    []llm.ContentPart{llm.Text("builtin result")},
	}))

	renderer := CreateToolHTMLRenderer(ExportHTMLToolRendererDeps{
		CWD:   "/tmp/export-tool-renderer",
		Width: 100,
		GetToolDefinition: func(name string) (ToolDefinition, bool) {
			if name != "custom_tool" {
				return ToolDefinition{}, false
			}
			return ToolDefinition{
				Name: "custom_tool",
				RenderCall: func(args any, context ToolRenderContext) []string {
					if context.CWD != "/tmp/export-tool-renderer" || !context.ArgsComplete || !context.ExecutionStarted {
						t.Fatalf("call render context = %#v", context)
					}
					values, _ := args.(map[string]any)
					return []string{"custom call " + values["value"].(string)}
				},
				RenderResult: func(result FileToolResult, options ToolRenderResultOptions, context ToolRenderContext) []string {
					if context.ToolCallID != "custom-call-1" || context.IsPartial || !context.ExecutionStarted {
						t.Fatalf("result render context = %#v", context)
					}
					if options.Expanded {
						return []string{"\x1b[31mexpanded " + result.Text + "\x1b[0m"}
					}
					return []string{"collapsed " + result.Text}
				},
			}, true
		},
	})

	data := BuildExportHTMLSessionDataWithOptions(sm, ExportHTMLOptions{ToolRenderer: renderer})
	if len(data.RenderedTools) != 1 {
		t.Fatalf("rendered tools = %#v", data.RenderedTools)
	}
	rendered := data.RenderedTools["custom-call-1"]
	if !strings.Contains(rendered.CallHTML, "custom call alpha") ||
		!strings.Contains(rendered.ResultHTMLCollapsed, "collapsed custom result") ||
		!strings.Contains(rendered.ResultHTMLExpanded, `color:#800000`) ||
		!strings.Contains(rendered.ResultHTMLExpanded, "expanded custom result") {
		t.Fatalf("custom rendered tool HTML = %#v", rendered)
	}
	if _, ok := data.RenderedTools["builtin-call-1"]; ok {
		t.Fatalf("template-rendered built-in tool should not be pre-rendered: %#v", data.RenderedTools)
	}
}
