package gicodingagent

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func TestToolExecutionComponentPiRendererParity(t *testing.T) {
	t.Run("stacks custom call and result renderers like the old implementation", func(t *testing.T) {
		definition := ToolDefinition{
			Name:       "custom_tool",
			RenderCall: func(any, ToolRenderContext) []string { return []string{"custom call"} },
			RenderResult: func(FileToolResult, ToolRenderResultOptions, ToolRenderContext) []string {
				return []string{"custom result"}
			},
		}
		component := NewToolExecutionComponent("custom_tool", "tool-1", map[string]any{}, definition, t.TempDir())
		if rendered := toolExecutionRendered(component); !strings.Contains(rendered, "custom call") {
			t.Fatalf("call render = %q", rendered)
		}
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text("done")}}, false)
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "custom call") || !strings.Contains(rendered, "custom result") {
			t.Fatalf("result render = %q", rendered)
		}
	})

	t.Run("uses built-in rendering for built-in overrides without custom renderers", func(t *testing.T) {
		component := NewToolExecutionComponent(
			"edit",
			"tool-2",
			map[string]any{"path": "README.md", "oldText": "before", "newText": "after"},
			ToolDefinition{Name: "edit"},
			t.TempDir(),
		)
		component.UpdateResult(FileToolResult{Details: &FileToolDetails{Diff: "+ after"}}, false)
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "edit") || !strings.Contains(rendered, "README.md") {
			t.Fatalf("edit override render = %q", rendered)
		}
		if strings.Contains(rendered, ":1") {
			t.Fatalf("edit override should not render firstChangedLine marker: %q", rendered)
		}
	})

	t.Run("uses built-in rendering for search tools like Pi", func(t *testing.T) {
		var grepLines []string
		for i := 0; i < 16; i++ {
			grepLines = append(grepLines, "match")
		}
		grep := NewToolExecutionComponent("grep", "tool-search-grep", map[string]any{"pattern": "needle", "path": ".", "glob": "*.go", "limit": 5}, ToolDefinition{}, t.TempDir())
		grep.UpdateResult(FileToolResult{
			Content: []llm.ContentPart{llm.Text(strings.Join(grepLines, "\n"))},
			Details: &FileToolDetails{MatchLimitReached: 5},
		}, false)
		rendered := toolExecutionRendered(grep)
		if !strings.Contains(rendered, "grep /needle/ in . (*.go) limit 5") ||
			!strings.Contains(rendered, "... (1 more lines, Ctrl+O to expand)") ||
			!strings.Contains(rendered, "[Truncated: 5 matches limit]") ||
			strings.Contains(rendered, `"pattern"`) {
			t.Fatalf("grep render = %q", rendered)
		}

		find := NewToolExecutionComponent("find", "tool-search-find", map[string]any{"pattern": "**/*.go", "path": "src", "limit": 3}, ToolDefinition{}, t.TempDir())
		find.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text("a.go\nb.go")}}, false)
		if rendered := toolExecutionRendered(find); !strings.Contains(rendered, "find **/*.go in src (limit 3)") || strings.Contains(rendered, `"pattern"`) {
			t.Fatalf("find render = %q", rendered)
		}

		ls := NewToolExecutionComponent("ls", "tool-search-ls", map[string]any{"path": ".", "limit": 2}, ToolDefinition{}, t.TempDir())
		ls.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text("a\nb")}}, false)
		if rendered := toolExecutionRendered(ls); !strings.Contains(rendered, "ls . (limit 2)") || strings.Contains(rendered, `"path"`) {
			t.Fatalf("ls render = %q", rendered)
		}
	})

	t.Run("preserves legacy file_path rendering compatibility for built-in tools", func(t *testing.T) {
		component := NewToolExecutionComponent("read", "tool-3", map[string]any{"file_path": "README.md"}, ToolDefinition{}, t.TempDir())
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "read") || !strings.Contains(rendered, "README.md") {
			t.Fatalf("legacy file_path render = %q", rendered)
		}
	})

	t.Run("does not duplicate built-in headers when passed the active built-in definition", func(t *testing.T) {
		component := NewToolExecutionComponent("read", "tool-4", map[string]any{"path": "README.md"}, CreateReadToolDefinition(t.TempDir()), t.TempDir())
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text("hello")}}, false)
		rendered := toolExecutionRendered(component)
		if count := len(regexp.MustCompile(`\bread\b`).FindAllString(rendered, -1)); count != 1 {
			t.Fatalf("read header count = %d in %q", count, rendered)
		}
	})

	t.Run("inherits missing built-in result renderer slot from the built-in tool", func(t *testing.T) {
		definition := ToolDefinition{
			Name:       "read",
			RenderCall: func(any, ToolRenderContext) []string { return []string{"override call"} },
		}
		component := NewToolExecutionComponent("read", "tool-4b", map[string]any{"path": "notes.txt"}, definition, t.TempDir())
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text("hello")}}, false)
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "override call") || !strings.Contains(rendered, "hello") {
			t.Fatalf("missing result renderer inheritance = %q", rendered)
		}
	})

	t.Run("inherits missing built-in call renderer slot from the built-in tool", func(t *testing.T) {
		definition := ToolDefinition{
			Name: "read",
			RenderResult: func(FileToolResult, ToolRenderResultOptions, ToolRenderContext) []string {
				return []string{"override result"}
			},
		}
		component := NewToolExecutionComponent("read", "tool-4c", map[string]any{"path": "README.md"}, definition, t.TempDir())
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text("hello")}}, false)
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "read") || !strings.Contains(rendered, "README.md") || !strings.Contains(rendered, "override result") {
			t.Fatalf("missing call renderer inheritance = %q", rendered)
		}
	})

	t.Run("uses custom renderers for built-in overrides that reuse built-in definition parameters", func(t *testing.T) {
		definition := CreateReadToolDefinition(t.TempDir())
		definition.RenderCall = func(any, ToolRenderContext) []string { return []string{"override call"} }
		definition.RenderResult = func(FileToolResult, ToolRenderResultOptions, ToolRenderContext) []string {
			return []string{"override result"}
		}
		component := NewToolExecutionComponent("read", "tool-4d", map[string]any{"path": "README.md"}, definition, t.TempDir())
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text("hello")}}, false)
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "override call") || !strings.Contains(rendered, "override result") || strings.Contains(rendered, "read README.md") {
			t.Fatalf("custom read override render = %q", rendered)
		}
	})

	t.Run("uses custom renderers for built-in overrides that reuse wrapped built-in tool parameters", func(t *testing.T) {
		definition := ToolDefinition{
			Name:       "read",
			Parameters: CreateReadToolDefinition(t.TempDir()).Parameters,
			RenderCall: func(any, ToolRenderContext) []string { return []string{"wrapped override call"} },
			RenderResult: func(FileToolResult, ToolRenderResultOptions, ToolRenderContext) []string {
				return []string{"wrapped override result"}
			},
		}
		component := NewToolExecutionComponent("read", "tool-4e", map[string]any{"path": "README.md"}, definition, t.TempDir())
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text("hello")}}, false)
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "wrapped override call") || !strings.Contains(rendered, "wrapped override result") {
			t.Fatalf("wrapped read override render = %q", rendered)
		}
	})

	t.Run("renders image fallbacks when terminal images are unavailable like Pi", func(t *testing.T) {
		gitui.SetCapabilities(gitui.TerminalCapabilities{Images: false, Protocol: gitui.ImageProtocolNone})
		defer gitui.ResetCapabilitiesCache()

		imageData := testPNGBase64(t, 2, 2)
		component := NewToolExecutionComponent("read", "tool-image", map[string]any{"path": "image.png"}, ToolDefinition{Name: "read"}, t.TempDir())
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Image(imageData, "image/png")}}, false)

		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "[Image: [image/png] 2x2]") {
			t.Fatalf("image fallback render = %q", rendered)
		}
	})

	t.Run("uses terminal image protocol when enabled and show-images is true", func(t *testing.T) {
		gitui.SetCapabilities(gitui.TerminalCapabilities{Images: true, Protocol: gitui.ImageProtocolKitty})
		defer gitui.ResetCapabilitiesCache()

		showImages := true
		imageData := testPNGBase64(t, 2, 2)
		component := NewToolExecutionComponent(
			"custom_tool",
			"tool-image-protocol",
			map[string]any{},
			ToolDefinition{Name: "custom_tool"},
			t.TempDir(),
			ToolExecutionOptions{ShowImages: &showImages, ImageWidthCells: 24},
		)
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Image(imageData, "image/png")}}, false)

		rendered := toolExecutionRendered(component)
		if !gitui.IsImageLine(rendered) || strings.Contains(rendered, "[Image:") {
			t.Fatalf("image protocol render = %q", rendered)
		}
	})

	t.Run("keeps fallback when show-images is disabled even with image-capable terminal", func(t *testing.T) {
		gitui.SetCapabilities(gitui.TerminalCapabilities{Images: true, Protocol: gitui.ImageProtocolKitty})
		defer gitui.ResetCapabilitiesCache()

		showImages := false
		imageData := testPNGBase64(t, 2, 2)
		component := NewToolExecutionComponent(
			"read",
			"tool-image-hidden",
			map[string]any{"path": "image.png"},
			ToolDefinition{Name: "read"},
			t.TempDir(),
			ToolExecutionOptions{ShowImages: &showImages, ImageWidthCells: 24},
		)
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Image(imageData, "image/png")}}, false)

		rendered := toolExecutionRendered(component)
		if gitui.IsImageLine(rendered) || !strings.Contains(rendered, "[Image: [image/png] 2x2]") {
			t.Fatalf("disabled image render = %q", rendered)
		}
	})

	t.Run("shares renderer state across custom call and result slots", func(t *testing.T) {
		definition := ToolDefinition{
			Name: "custom_tool",
			RenderCall: func(_ any, context ToolRenderContext) []string {
				context.State["token"] = "shared-token"
				return []string{"custom call " + context.State["token"].(string)}
			},
			RenderResult: func(FileToolResult, ToolRenderResultOptions, ToolRenderContext) []string {
				return []string{"custom result shared-token"}
			},
		}
		component := NewToolExecutionComponent("custom_tool", "tool-5", map[string]any{}, definition, t.TempDir())
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text("done")}}, false)
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "custom call shared-token") || !strings.Contains(rendered, "custom result shared-token") {
			t.Fatalf("shared renderer state = %q", rendered)
		}
	})

	t.Run("exposes args in render result context", func(t *testing.T) {
		definition := ToolDefinition{
			Name:       "custom_tool",
			RenderCall: func(any, ToolRenderContext) []string { return []string{"call"} },
			RenderResult: func(_ FileToolResult, _ ToolRenderResultOptions, context ToolRenderContext) []string {
				args := context.Args.(map[string]any)
				return []string{"arg:" + args["foo"].(string)}
			},
		}
		component := NewToolExecutionComponent("custom_tool", "tool-5b", map[string]any{"foo": "bar"}, definition, t.TempDir())
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text("done")}}, false)
		if rendered := toolExecutionRendered(component); !strings.Contains(rendered, "arg:bar") {
			t.Fatalf("result context args = %q", rendered)
		}
	})

	t.Run("exposes showImages in renderer context like Pi", func(t *testing.T) {
		definition := ToolDefinition{
			Name: "custom_tool",
			RenderCall: func(_ any, context ToolRenderContext) []string {
				if context.ShowImages {
					return []string{"call images:on"}
				}
				return []string{"call images:off"}
			},
			RenderResult: func(_ FileToolResult, _ ToolRenderResultOptions, context ToolRenderContext) []string {
				if context.ShowImages {
					return []string{"result images:on"}
				}
				return []string{"result images:off"}
			},
		}
		showImages := false
		component := NewToolExecutionComponent(
			"custom_tool",
			"tool-5c",
			map[string]any{},
			definition,
			t.TempDir(),
			ToolExecutionOptions{ShowImages: &showImages},
		)
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text("done")}}, false)
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "call images:off") || !strings.Contains(rendered, "result images:off") {
			t.Fatalf("showImages context = %q", rendered)
		}
	})

	t.Run("falls back when custom renderers are absent", func(t *testing.T) {
		component := NewToolExecutionComponent("custom_tool", "tool-6", map[string]any{"foo": "bar"}, ToolDefinition{Name: "custom_tool"}, t.TempDir())
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text("done")}}, false)
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "custom_tool") || !strings.Contains(rendered, `"foo": "bar"`) || !strings.Contains(rendered, "done") {
			t.Fatalf("fallback render = %q", rendered)
		}
	})

	t.Run("falls back when custom call renderer panics like Pi", func(t *testing.T) {
		definition := ToolDefinition{
			Name: "custom_tool",
			RenderCall: func(any, ToolRenderContext) []string {
				panic("renderer failed")
			},
		}
		component := NewToolExecutionComponent("custom_tool", "tool-6b", map[string]any{"foo": "bar"}, definition, t.TempDir())
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "custom_tool") || !strings.Contains(rendered, `"foo": "bar"`) {
			t.Fatalf("panic call fallback render = %q", rendered)
		}
	})

	t.Run("falls back when custom result renderer panics like Pi", func(t *testing.T) {
		definition := ToolDefinition{
			Name:       "custom_tool",
			RenderCall: func(any, ToolRenderContext) []string { return []string{"custom call"} },
			RenderResult: func(FileToolResult, ToolRenderResultOptions, ToolRenderContext) []string {
				panic("renderer failed")
			},
		}
		component := NewToolExecutionComponent("custom_tool", "tool-6c", map[string]any{"foo": "bar"}, definition, t.TempDir())
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text("fallback result")}}, false)
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "custom call") || !strings.Contains(rendered, "fallback result") {
			t.Fatalf("panic result fallback render = %q", rendered)
		}
	})
}

func TestToolExecutionComponentPiBuiltinDisplayParity(t *testing.T) {
	t.Run("trims trailing blank display lines from write previews", func(t *testing.T) {
		component := NewToolExecutionComponent(
			"write",
			"tool-7",
			map[string]any{"path": "README.md", "content": "one\ntwo\n"},
			CreateWriteToolDefinition(t.TempDir()),
			t.TempDir(),
		)
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "one") || !strings.Contains(rendered, "two") || strings.Contains(rendered, "two\n\n") {
			t.Fatalf("write preview render = %q", rendered)
		}
	})

	t.Run("trims trailing blank display lines from read results", func(t *testing.T) {
		component := NewToolExecutionComponent("read", "tool-8", map[string]any{"path": "notes.txt"}, CreateReadToolDefinition(t.TempDir()), t.TempDir())
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text("one\ntwo\n")}}, false)
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "one") || !strings.Contains(rendered, "two") || strings.Contains(rendered, "two\n\n") {
			t.Fatalf("read result render = %q", rendered)
		}
	})

	t.Run("expands tabs in read results like Pi", func(t *testing.T) {
		component := NewToolExecutionComponent("read", "tool-read-tabs", map[string]any{"path": "notes.txt"}, CreateReadToolDefinition(t.TempDir()), t.TempDir())
		component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text("one\ttwo")}}, false)
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "one   two") || strings.Contains(rendered, "\t") {
			t.Fatalf("read tabs render = %q", rendered)
		}
	})

	t.Run("shows read truncation summary after collapsed output like Pi", func(t *testing.T) {
		var lines []string
		for i := 0; i < 12; i++ {
			lines = append(lines, "line")
		}
		component := NewToolExecutionComponent("read", "tool-read-truncated", map[string]any{"path": "notes.txt"}, CreateReadToolDefinition(t.TempDir()), t.TempDir())
		component.UpdateResult(FileToolResult{
			Content: []llm.ContentPart{llm.Text(strings.Join(lines, "\n"))},
			Details: &FileToolDetails{Truncation: &ReadToolTruncation{
				Truncated:   true,
				TruncatedBy: "lines",
				TotalLines:  2500,
				OutputLines: 2000,
			}},
		}, false)
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "... (2 more lines, Ctrl+O to expand)") ||
			!strings.Contains(rendered, "[Truncated: showing 2000 of 2500 lines (2000 line limit)]") {
			t.Fatalf("read truncation render = %q", rendered)
		}
	})

	t.Run("normalizes write preview CR and tabs like Pi", func(t *testing.T) {
		component := NewToolExecutionComponent(
			"write",
			"tool-write-tabs",
			map[string]any{"path": "notes.txt", "content": "one\ttwo\r\nthree\r"},
			CreateWriteToolDefinition(t.TempDir()),
			t.TempDir(),
		)
		rendered := toolExecutionRendered(component)
		if !strings.Contains(rendered, "one   two") || !strings.Contains(rendered, "three") || strings.Contains(rendered, "\t") || strings.Contains(rendered, "\r") {
			t.Fatalf("write preview render = %q", rendered)
		}
	})

	t.Run("syntax-highlights write previews with Pi-style incremental cache", func(t *testing.T) {
		component := NewToolExecutionComponent(
			"write",
			"tool-write-highlight",
			map[string]any{"path": "src/example.ts", "content": "const value = 1\n"},
			CreateWriteToolDefinition(t.TempDir()),
			t.TempDir(),
		)
		raw := strings.Join(component.Render(120), "\n")
		if !strings.Contains(raw, tuiThemeFG("syntaxKeyword", "const")) ||
			!strings.Contains(raw, tuiThemeFG("syntaxNumber", "1")) {
			t.Fatalf("write preview should include syntax highlighting:\n%s", raw)
		}
		firstCache := getWriteHighlightCache(component.rendererState)
		if firstCache == nil || firstCache.rawContent != "const value = 1\n" {
			t.Fatalf("initial write highlight cache = %#v", firstCache)
		}

		component.UpdateArgs(map[string]any{"path": "src/example.ts", "content": "const value = 1\nconst next = 2\n"})
		raw = strings.Join(component.Render(120), "\n")
		if getWriteHighlightCache(component.rendererState) != firstCache {
			t.Fatal("append-only partial write preview should reuse highlight cache")
		}
		if !strings.Contains(raw, tuiThemeFG("syntaxNumber", "2")) {
			t.Fatalf("incremental write preview missing highlighted appended line:\n%s", raw)
		}

		component.SetArgsComplete()
		raw = strings.Join(component.Render(120), "\n")
		if !strings.Contains(raw, tuiThemeFG("syntaxKeyword", "const")) ||
			!strings.Contains(raw, tuiThemeFG("syntaxNumber", "2")) {
			t.Fatalf("completed write preview should rebuild full highlighted content:\n%s", raw)
		}
	})

	for _, scenario := range []struct {
		title   string
		path    func(string) string
		content string
		compact string
		hidden  string
		absent  string
	}{
		{
			title:   "SKILL.md",
			path:    func(dir string) string { return filepath.Join(dir, "attio", "SKILL.md") },
			content: "---\nname: attio\ndescription: CRM helper\n---\n\n# Hidden skill instructions",
			compact: "[skill] attio",
			hidden:  "Hidden skill instructions",
			absent:  "read skill attio",
		},
		{
			title:   "AGENTS.md",
			path:    func(dir string) string { return filepath.Join(dir, ".gi", "AGENTS.md") },
			content: "Hidden resource instructions",
			compact: "read resource .gi/AGENTS.md",
			hidden:  "Hidden resource instructions",
		},
		{
			title:   "CLAUDE.md",
			path:    func(dir string) string { return filepath.Join(dir, "CLAUDE.md") },
			content: "Hidden Claude resource instructions",
			compact: "read resource CLAUDE.md",
			hidden:  "Hidden Claude resource instructions",
		},
		{
			title:   "outside AGENTS.md",
			path:    func(dir string) string { return filepath.Join(filepath.Dir(dir), "AGENTS.md") },
			content: "Hidden outside resource instructions",
			compact: "read resource ",
			hidden:  "Hidden outside resource instructions",
		},
		{
			title:   "Pi documentation",
			path:    func(dir string) string { return filepath.Join(dir, "README.md") },
			content: "Hidden docs content",
			compact: "read docs README.md",
			hidden:  "Hidden docs content",
		},
	} {
		scenario := scenario
		t.Run("renders "+scenario.title+" read results compactly until expanded", func(t *testing.T) {
			dir := t.TempDir()
			path := scenario.path(dir)
			component := NewToolExecutionComponent("read", "tool-compact-"+scenario.title, map[string]any{"path": path}, CreateReadToolDefinition(dir), dir)
			component.UpdateResult(FileToolResult{Content: []llm.ContentPart{llm.Text(scenario.content)}}, false)
			collapsed := toolExecutionRendered(component)
			if !strings.Contains(collapsed, scenario.compact) || strings.Contains(collapsed, scenario.hidden) {
				t.Fatalf("collapsed render = %q", collapsed)
			}
			if scenario.absent != "" && strings.Contains(collapsed, scenario.absent) {
				t.Fatalf("collapsed render should not contain %q: %q", scenario.absent, collapsed)
			}
			component.SetExpanded(true)
			if expanded := toolExecutionRendered(component); !strings.Contains(expanded, scenario.hidden) {
				t.Fatalf("expanded render = %q", expanded)
			}
		})
	}

	for _, scenario := range []struct {
		title   string
		path    func(string) string
		compact string
	}{
		{
			title:   "SKILL.md",
			path:    func(dir string) string { return filepath.Join(dir, "attio", "SKILL.md") },
			compact: "[skill] attio:120-329",
		},
		{
			title:   "Pi documentation",
			path:    func(dir string) string { return filepath.Join(dir, "README.md") },
			compact: "read docs README.md:120-329",
		},
	} {
		scenario := scenario
		t.Run("shows the read line range in compact "+scenario.title+" reads before the expand hint", func(t *testing.T) {
			dir := t.TempDir()
			component := NewToolExecutionComponent("read", "tool-compact-range-"+scenario.title, map[string]any{
				"path":   scenario.path(dir),
				"offset": 120,
				"limit":  210,
			}, CreateReadToolDefinition(dir), dir)
			collapsed := toolExecutionRendered(component)
			if !strings.Contains(collapsed, scenario.compact) {
				t.Fatalf("collapsed range render = %q", collapsed)
			}
			if strings.Index(collapsed, ":120-329") > strings.Index(collapsed, "to expand") {
				t.Fatalf("line range should come before expand hint: %q", collapsed)
			}
		})
	}
}

func TestBashToolPiInitialPartialUpdate(t *testing.T) {
	dir := t.TempDir()
	var updates []FileToolResult
	tool := NewBashTool(dir, BashToolOptions{Operations: BashOperations{
		Exec: func(string, string, BashExecOptions) (BashOperationResult, error) {
			time.Sleep(10 * time.Millisecond)
			return BashOperationResult{ExitCode: 0}, nil
		},
	}})
	if _, err := tool.ExecuteWithUpdates("tool-bash-1", BashToolInput{Command: "sleep 10"}, func(update FileToolResult) {
		updates = append(updates, update)
	}); err != nil {
		t.Fatal(err)
	}
	if len(updates) == 0 || fileToolResultText(updates[0]) != "" || len(updates[0].Content) != 0 || updates[0].Details != nil {
		t.Fatalf("initial update = %#v", updates)
	}
}

func TestToolRenderContextParamsIncludesPiShowImages(t *testing.T) {
	params := toolRenderContextParams(ToolRenderContext{
		Args:       map[string]any{"foo": "bar"},
		ToolCallID: "tool-context-1",
		ShowImages: true,
	})
	if params["showImages"] != true {
		t.Fatalf("showImages param = %#v in %#v", params["showImages"], params)
	}
}

func TestToolResultTextSanitizesPiStyle(t *testing.T) {
	result := FileToolResult{Content: []llm.ContentPart{
		llm.Text("\x1b[31mred\x1b[0m\r\nnul\x00ok\tkeep\ufff9drop"),
	}}
	if got := fileToolResultText(result); got != "red\nnulok\tkeepdrop" {
		t.Fatalf("sanitized text = %q", got)
	}
}

func TestShortenDisplayPathUsesHomeLikePi(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got := shortenDisplayPath(filepath.Join(home, "project", "README.md"))
	if got != "~/project/README.md" {
		t.Fatalf("short path = %q", got)
	}
}

func toolExecutionRendered(component *ToolExecutionComponent) string {
	lines := component.Render(120)
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		normalized = append(normalized, strings.TrimRight(StripAnsi(line), " \t"))
	}
	return strings.Join(normalized, "\n")
}

func toolExecutionContentLines(component *ToolExecutionComponent, width int) []string {
	rendered := component.Render(width)
	lines := make([]string, 0, len(rendered))
	for _, line := range rendered {
		text := strings.TrimSpace(StripAnsi(line))
		if text != "" {
			lines = append(lines, text)
		}
	}
	return lines
}
