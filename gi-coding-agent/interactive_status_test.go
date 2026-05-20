package gicodingagent

import (
	"reflect"
	"strings"
	"testing"
)

func TestInteractiveModeShowStatusMatchesPi(t *testing.T) {
	t.Run("coalesces immediately-sequential status messages", func(t *testing.T) {
		chat := &InteractiveStatusChat{}
		ui := &fakeInteractiveUI{}
		mode := &InteractiveMode{Chat: chat, UI: ui}

		mode.ShowStatusMessage("STATUS_ONE")
		if len(chat.Children) != 2 || !strings.Contains(chat.Children[len(chat.Children)-1], "STATUS_ONE") {
			t.Fatalf("children after first status = %#v", chat.Children)
		}
		mode.ShowStatusMessage("STATUS_TWO")
		if len(chat.Children) != 2 || !strings.Contains(chat.Children[len(chat.Children)-1], "STATUS_TWO") || strings.Contains(chat.Children[len(chat.Children)-1], "STATUS_ONE") {
			t.Fatalf("children after second status = %#v", chat.Children)
		}
	})

	t.Run("appends a new status line if something else was added in between", func(t *testing.T) {
		chat := &InteractiveStatusChat{}
		mode := &InteractiveMode{Chat: chat, UI: &fakeInteractiveUI{}}

		mode.ShowStatusMessage("STATUS_ONE")
		chat.AddText("OTHER")
		mode.ShowStatusMessage("STATUS_TWO")

		if len(chat.Children) != 5 || !strings.Contains(chat.Children[len(chat.Children)-1], "STATUS_TWO") {
			t.Fatalf("children = %#v", chat.Children)
		}
	})
}

func TestInteractiveModeSetToolsExpandedMatchesPi(t *testing.T) {
	header := &fakeExpandable{}
	child := &fakeExpandable{}
	ui := &fakeInteractiveUI{}
	mode := &InteractiveMode{BuiltInHeader: header, ChatExpandables: []InteractiveExpandable{child}, UI: ui}

	mode.SetToolsExpanded(true)

	if !mode.ToolOutputExpanded || !reflect.DeepEqual(header.values, []bool{true}) || !reflect.DeepEqual(child.values, []bool{true}) {
		t.Fatalf("expanded=%v header=%#v child=%#v", mode.ToolOutputExpanded, header.values, child.values)
	}
	if !reflect.DeepEqual(ui.renderForces, []bool{false}) {
		t.Fatalf("renders = %#v", ui.renderForces)
	}
}

func TestInteractiveModeExtensionUIContextThemeMatchesPi(t *testing.T) {
	t.Run("persists theme changes to settings manager", func(t *testing.T) {
		settings := &fakeThemeSettings{theme: "dark"}
		ui := &fakeInteractiveUI{}
		mode := &InteractiveMode{ThemeSettings: settings, UI: ui}

		result := mode.CreateExtensionUIContext().SetTheme("light")

		if !result.Success || settings.theme != "light" || !reflect.DeepEqual(settings.setCalls, []string{"light"}) {
			t.Fatalf("result=%#v settings=%#v", result, settings)
		}
		if !reflect.DeepEqual(ui.renderForces, []bool{false}) {
			t.Fatalf("renders = %#v", ui.renderForces)
		}
	})

	t.Run("does not persist invalid theme names", func(t *testing.T) {
		settings := &fakeThemeSettings{theme: "dark"}
		ui := &fakeInteractiveUI{}
		mode := &InteractiveMode{ThemeSettings: settings, UI: ui}

		result := mode.CreateExtensionUIContext().SetTheme("__missing_theme__")

		if result.Success || len(settings.setCalls) != 0 || len(ui.renderForces) != 0 {
			t.Fatalf("result=%#v settings=%#v renders=%#v", result, settings, ui.renderForces)
		}
	})
}

func TestInteractiveModeAutocompleteProviderMatchesPi(t *testing.T) {
	t.Run("stores wrapper factories and rebuilds autocomplete immediately", func(t *testing.T) {
		var setupCount int
		wrapper := func(current AutocompleteProvider) AutocompleteProvider { return current }
		mode := &InteractiveMode{
			CreateBaseAutocompleteProvider: func() AutocompleteProvider { return defaultAutocompleteProvider{} },
			DefaultAutocompleteEditor:      &fakeAutocompleteEditor{},
		}
		originalSetup := mode.SetupAutocompleteProvider
		_ = originalSetup
		mode.DefaultAutocompleteEditor = &fakeAutocompleteEditor{onSet: func(AutocompleteProvider) { setupCount++ }}

		mode.CreateExtensionUIContext().AddAutocompleteProvider(wrapper)

		if len(mode.AutocompleteProviderWrappers) != 1 || setupCount != 1 {
			t.Fatalf("wrappers=%d setup=%d", len(mode.AutocompleteProviderWrappers), setupCount)
		}
	})

	t.Run("stacks wrapper factories over a fresh base provider", func(t *testing.T) {
		defaultEditor := &fakeAutocompleteEditor{}
		customEditor := &fakeAutocompleteEditor{}
		calls := []string{}
		wrap1 := func(current AutocompleteProvider) AutocompleteProvider {
			return &fakeAutocompleteProvider{trigger: func(lines []string, cursorLine, cursorCol int) bool {
				calls = append(calls, "shouldTrigger:wrap1")
				return current.ShouldTriggerFileCompletion(lines, cursorLine, cursorCol)
			}}
		}
		wrap2 := func(current AutocompleteProvider) AutocompleteProvider {
			return &fakeAutocompleteProvider{trigger: func(lines []string, cursorLine, cursorCol int) bool {
				calls = append(calls, "shouldTrigger:wrap2")
				return current.ShouldTriggerFileCompletion(lines, cursorLine, cursorCol)
			}}
		}
		mode := &InteractiveMode{
			CreateBaseAutocompleteProvider: func() AutocompleteProvider { return defaultAutocompleteProvider{} },
			DefaultAutocompleteEditor:      defaultEditor,
			AutocompleteEditor:             customEditor,
			AutocompleteProviderWrappers:   []AutocompleteProviderFactory{wrap1, wrap2},
		}

		mode.SetupAutocompleteProvider()

		if defaultEditor.provider == nil || defaultEditor.provider != customEditor.provider {
			t.Fatalf("providers default=%#v custom=%#v", defaultEditor.provider, customEditor.provider)
		}
		if !defaultEditor.provider.ShouldTriggerFileCompletion([]string{"foo"}, 0, 3) {
			t.Fatal("provider should trigger")
		}
		if !reflect.DeepEqual(calls, []string{"shouldTrigger:wrap2", "shouldTrigger:wrap1"}) {
			t.Fatalf("calls = %#v", calls)
		}
	})
}

func TestInteractiveModeShowLoadedResourcesSkillsAndDiagnosticsMatchPi(t *testing.T) {
	t.Run("shows a compact resource listing by default", func(t *testing.T) {
		output := renderInteractiveLoadedResources(InteractiveLoadedResources{
			Skills: []InteractiveSkillResource{{FilePath: "/tmp/skill/SKILL.md", Name: "commit"}},
		}, InteractiveShowLoadedResourcesOptions{})
		if !strings.Contains(output, "[Skills]") || !strings.Contains(output, "commit") || strings.Contains(output, "resource-list") {
			t.Fatalf("output = %q", output)
		}
	})

	t.Run("shows full resource listing when expanded", func(t *testing.T) {
		output := renderInteractiveLoadedResources(InteractiveLoadedResources{
			ToolOutputExpanded: true,
			Skills:             []InteractiveSkillResource{{FilePath: "/tmp/skill/SKILL.md", Name: "commit"}},
		}, InteractiveShowLoadedResourcesOptions{})
		if !strings.Contains(output, "[Skills]") || !strings.Contains(output, "resource-list") || strings.Contains(output, "commit") {
			t.Fatalf("output = %q", output)
		}
	})

	t.Run("shows full resource listing on verbose startup even when tool output is collapsed", func(t *testing.T) {
		output := renderInteractiveLoadedResources(InteractiveLoadedResources{
			QuietStartup: true,
			Verbose:      true,
			Skills:       []InteractiveSkillResource{{FilePath: "/tmp/skill/SKILL.md", Name: "commit"}},
		}, InteractiveShowLoadedResourcesOptions{})
		if !strings.Contains(output, "[Skills]") || !strings.Contains(output, "resource-list") || strings.Contains(output, "commit") {
			t.Fatalf("output = %q", output)
		}
	})

	t.Run("does not show verbose listing on quiet startup during reload", func(t *testing.T) {
		output := renderInteractiveLoadedResources(InteractiveLoadedResources{
			QuietStartup: true,
			Skills:       []InteractiveSkillResource{{FilePath: "/tmp/skill/SKILL.md", Name: "commit"}},
			Extensions:   []InteractiveExtensionResource{{Path: "/tmp/ext/index.ts"}},
		}, InteractiveShowLoadedResourcesOptions{ShowDiagnosticsWhenQuiet: true})
		if output != "" {
			t.Fatalf("output = %q, want empty", output)
		}
	})

	t.Run("still shows diagnostics on quiet startup when requested", func(t *testing.T) {
		output := renderInteractiveLoadedResources(InteractiveLoadedResources{
			QuietStartup: true,
			Skills:       []InteractiveSkillResource{{FilePath: "/tmp/skill/SKILL.md", Name: "commit"}},
			SkillDiagnostics: []InteractiveResourceDiagnostic{{
				Type:    "warning",
				Message: "duplicate skill name",
			}},
		}, InteractiveShowLoadedResourcesOptions{ShowDiagnosticsWhenQuiet: true})
		if !strings.Contains(output, "[Skill conflicts]") || strings.Contains(output, "[Skills]") {
			t.Fatalf("output = %q", output)
		}
	})
}

func TestInteractiveModeShowLoadedResourcesExtensionLabelsMatchPi(t *testing.T) {
	tests := []struct {
		name       string
		extensions []InteractiveExtensionResource
		expanded   bool
		want       string
	}{
		{
			name: "abbreviates extensions in compact listing",
			extensions: []InteractiveExtensionResource{
				{Path: "/tmp/extensions/answer.ts"},
				{Path: "/tmp/extensions/btw.ts"},
			},
			want: "[Extensions]\n  answer.ts, btw.ts",
		},
		{
			name:       "captures mixed extension layouts in compact output",
			extensions: createInteractiveExtensionFixtures(),
			want:       "[Extensions]\n  @scope/pi-scoped, answer.ts, cli-extension.ts, HazAT/pi-interactive-subagents, HazAT/pi-interactive-subagents:subagents, local-index, pi-markdown-preview, user-index",
		},
		{
			name: "adds more parent folders until local extension labels are unique",
			extensions: []InteractiveExtensionResource{
				{Path: "/tmp/alpha/one/index.ts", SourceInfo: sourceInfo("/tmp/alpha/one/index.ts", "cli", "temporary", "top-level", "/tmp/alpha")},
				{Path: "/tmp/beta/one/index.ts", SourceInfo: sourceInfo("/tmp/beta/one/index.ts", "cli", "temporary", "top-level", "/tmp/beta")},
				{Path: "/tmp/gamma/one/index.ts", SourceInfo: sourceInfo("/tmp/gamma/one/index.ts", "cli", "temporary", "top-level", "/tmp/gamma")},
			},
			want: "[Extensions]\n  alpha/one, beta/one, gamma/one",
		},
		{
			name: "strips index.ts from local extension label, showing parent dir",
			extensions: []InteractiveExtensionResource{
				{Path: "/tmp/extensions/plan-mode/index.ts", SourceInfo: sourceInfo("/tmp/extensions/plan-mode/index.ts", "local", "project", "top-level", "/tmp/extensions")},
			},
			want: "[Extensions]\n  plan-mode",
		},
		{
			name: "strips index.js from local extension label, showing parent dir",
			extensions: []InteractiveExtensionResource{
				{Path: "/tmp/extensions/plan-mode/index.js", SourceInfo: sourceInfo("/tmp/extensions/plan-mode/index.js", "local", "project", "top-level", "/tmp/extensions")},
			},
			want: "[Extensions]\n  plan-mode",
		},
		{
			name: "mixed single-file and subdirectory index.ts extensions strip index.ts",
			extensions: []InteractiveExtensionResource{
				{Path: "/tmp/extensions/webfetch.ts", SourceInfo: sourceInfo("/tmp/extensions/webfetch.ts", "local", "project", "top-level", "/tmp/extensions")},
				{Path: "/tmp/extensions/plan-mode/index.ts", SourceInfo: sourceInfo("/tmp/extensions/plan-mode/index.ts", "local", "project", "top-level", "/tmp/extensions")},
			},
			want: "[Extensions]\n  plan-mode, webfetch.ts",
		},
		{
			name: "multiple index.ts with unique parent dirs need no disambiguation",
			extensions: []InteractiveExtensionResource{
				{Path: "/tmp/extensions/foo/index.ts", SourceInfo: sourceInfo("/tmp/extensions/foo/index.ts", "local", "project", "top-level", "/tmp/extensions")},
				{Path: "/tmp/extensions/bar/index.ts", SourceInfo: sourceInfo("/tmp/extensions/bar/index.ts", "local", "project", "top-level", "/tmp/extensions")},
			},
			want: "[Extensions]\n  bar, foo",
		},
		{
			name: "multiple index.ts with same parent dir name disambiguated with grandparent",
			extensions: []InteractiveExtensionResource{
				{Path: "/tmp/alpha/tools/index.ts", SourceInfo: sourceInfo("/tmp/alpha/tools/index.ts", "cli", "temporary", "top-level", "/tmp/alpha")},
				{Path: "/tmp/beta/tools/index.ts", SourceInfo: sourceInfo("/tmp/beta/tools/index.ts", "cli", "temporary", "top-level", "/tmp/beta")},
			},
			want: "[Extensions]\n  alpha/tools, beta/tools",
		},
		{
			name: "non-index file in subdirectory stays as filename",
			extensions: []InteractiveExtensionResource{
				{Path: "/tmp/extensions/my-ext/main.ts", SourceInfo: sourceInfo("/tmp/extensions/my-ext/main.ts", "local", "project", "top-level", "/tmp/extensions")},
			},
			want: "[Extensions]\n  main.ts",
		},
		{
			name: "package extensions still strip index.ts correctly",
			extensions: []InteractiveExtensionResource{
				{Path: "/tmp/project/.pi/npm/node_modules/pi-markdown-preview/extensions/index.ts", SourceInfo: sourceInfo("/tmp/project/.pi/npm/node_modules/pi-markdown-preview/extensions/index.ts", "npm:pi-markdown-preview", "project", "package", "/tmp/project/.pi/npm/node_modules/pi-markdown-preview")},
			},
			want: "[Extensions]\n  pi-markdown-preview",
		},
		{
			name:       "captures mixed extension layouts in expanded output",
			extensions: createInteractiveExtensionFixtures(),
			expanded:   true,
			want: `[Extensions]
  project
    /tmp/project/.pi/extensions/answer.ts
    /tmp/project/.pi/extensions/local-index
    git:github.com/HazAT/pi-interactive-subagents
      extensions
      extensions/subagents
    npm:@scope/pi-scoped
      extensions
    npm:pi-markdown-preview
      extensions
  user
    /tmp/agent/extensions/user-index
  path
    /tmp/temp/cli-extension.ts`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output := renderInteractiveLoadedResources(InteractiveLoadedResources{
				ToolOutputExpanded: tc.expanded,
				Extensions:         tc.extensions,
			}, InteractiveShowLoadedResourcesOptions{})
			if output != tc.want {
				t.Fatalf("output:\n%s\nwant:\n%s", output, tc.want)
			}
		})
	}
}

func TestInteractiveModeShowLoadedResourcesContextPathsMatchPi(t *testing.T) {
	oldHome := osUserHomeDir
	osUserHomeDir = func() (string, error) { return "/Users/test", nil }
	defer func() { osUserHomeDir = oldHome }()

	cwd := "/Users/test/Development/pi-mono"
	files := []InteractiveContextFile{
		{Path: "/Users/test/.pi/agent/AGENTS.md"},
		{Path: "/Users/test/Development/pi-mono/AGENTS.md"},
	}
	compact := renderInteractiveLoadedResources(InteractiveLoadedResources{CWD: cwd, ContextFiles: files}, InteractiveShowLoadedResourcesOptions{})
	if compact != "[Context]\n  ~/.pi/agent/AGENTS.md, AGENTS.md" {
		t.Fatalf("compact = %q", compact)
	}
	expanded := renderInteractiveLoadedResources(InteractiveLoadedResources{CWD: cwd, ToolOutputExpanded: true, ContextFiles: files}, InteractiveShowLoadedResourcesOptions{})
	if expanded != "[Context]\n  ~/.pi/agent/AGENTS.md, ~/Development/pi-mono/AGENTS.md" {
		t.Fatalf("expanded = %q", expanded)
	}
}

func renderInteractiveLoadedResources(resources InteractiveLoadedResources, options InteractiveShowLoadedResourcesOptions) string {
	chat := &InteractiveStatusChat{}
	mode := &InteractiveMode{Chat: chat, LoadedResources: resources}
	mode.ShowLoadedResources(options)
	return chat.Render()
}

func sourceInfo(path, source, scope, origin, baseDir string) *InteractiveSourceInfo {
	return &InteractiveSourceInfo{Path: path, Source: source, Scope: scope, Origin: origin, BaseDir: baseDir}
}

func createInteractiveExtensionFixtures() []InteractiveExtensionResource {
	return []InteractiveExtensionResource{
		{Path: "/tmp/project/.pi/extensions/answer.ts", SourceInfo: sourceInfo("/tmp/project/.pi/extensions/answer.ts", "local", "project", "top-level", "/tmp/project/.pi/extensions")},
		{Path: "/tmp/project/.pi/extensions/local-index/index.ts", SourceInfo: sourceInfo("/tmp/project/.pi/extensions/local-index/index.ts", "local", "project", "top-level", "/tmp/project/.pi/extensions")},
		{Path: "/tmp/agent/extensions/user-index/index.ts", SourceInfo: sourceInfo("/tmp/agent/extensions/user-index/index.ts", "local", "user", "top-level", "/tmp/agent/extensions")},
		{Path: "/tmp/project/.pi/npm/node_modules/pi-markdown-preview/extensions/index.ts", SourceInfo: sourceInfo("/tmp/project/.pi/npm/node_modules/pi-markdown-preview/extensions/index.ts", "npm:pi-markdown-preview", "project", "package", "/tmp/project/.pi/npm/node_modules/pi-markdown-preview")},
		{Path: "/tmp/project/.pi/npm/node_modules/@scope/pi-scoped/extensions/index.ts", SourceInfo: sourceInfo("/tmp/project/.pi/npm/node_modules/@scope/pi-scoped/extensions/index.ts", "npm:@scope/pi-scoped", "project", "package", "/tmp/project/.pi/npm/node_modules/@scope/pi-scoped")},
		{Path: "/tmp/project/.pi/git/github.com/HazAT/pi-interactive-subagents/extensions/index.ts", SourceInfo: sourceInfo("/tmp/project/.pi/git/github.com/HazAT/pi-interactive-subagents/extensions/index.ts", "git:github.com/HazAT/pi-interactive-subagents", "project", "package", "/tmp/project/.pi/git/github.com/HazAT/pi-interactive-subagents")},
		{Path: "/tmp/project/.pi/git/github.com/HazAT/pi-interactive-subagents/extensions/subagents/index.ts", SourceInfo: sourceInfo("/tmp/project/.pi/git/github.com/HazAT/pi-interactive-subagents/extensions/subagents/index.ts", "git:github.com/HazAT/pi-interactive-subagents", "project", "package", "/tmp/project/.pi/git/github.com/HazAT/pi-interactive-subagents")},
		{Path: "/tmp/temp/cli-extension.ts", SourceInfo: sourceInfo("/tmp/temp/cli-extension.ts", "cli", "temporary", "top-level", "/tmp/temp")},
	}
}

type fakeExpandable struct {
	values []bool
}

func (e *fakeExpandable) SetExpanded(expanded bool) {
	e.values = append(e.values, expanded)
}

type fakeThemeSettings struct {
	theme    string
	setCalls []string
}

func (s *fakeThemeSettings) GetTheme() string {
	return s.theme
}

func (s *fakeThemeSettings) SetTheme(theme string) {
	s.theme = theme
	s.setCalls = append(s.setCalls, theme)
}

type fakeAutocompleteProvider struct {
	trigger func([]string, int, int) bool
}

func (p fakeAutocompleteProvider) ShouldTriggerFileCompletion(lines []string, cursorLine, cursorCol int) bool {
	if p.trigger != nil {
		return p.trigger(lines, cursorLine, cursorCol)
	}
	return true
}

type fakeAutocompleteEditor struct {
	provider AutocompleteProvider
	onSet    func(AutocompleteProvider)
}

func (e *fakeAutocompleteEditor) SetAutocompleteProvider(provider AutocompleteProvider) {
	e.provider = provider
	if e.onSet != nil {
		e.onSet(provider)
	}
}
