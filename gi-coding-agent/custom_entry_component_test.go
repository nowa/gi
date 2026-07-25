package gicodingagent

import (
	"strings"
	"sync"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func TestCustomEntryComponentRebuildsFromTypedPresentationState(t *testing.T) {
	var mu sync.Mutex
	var expandedValues []bool
	var children []*customEntryTestComponent
	renderer := func(
		entry FileEntry,
		options ProtocolEntryRenderOptions,
	) gitui.Component {
		if entry.CustomType != "checkpoint" {
			t.Fatalf("entry = %#v", entry)
		}
		child := &customEntryTestComponent{
			text: map[bool]string{
				false: "collapsed",
				true:  "expanded",
			}[options.Expanded],
		}
		mu.Lock()
		expandedValues = append(expandedValues, options.Expanded)
		children = append(children, child)
		mu.Unlock()
		return child
	}

	component := NewCustomEntryComponent(
		FileEntry{
			Type:       "custom",
			CustomType: "checkpoint",
			Data:       map[string]any{"status": "ready"},
		},
		renderer,
	)
	if !component.HasContent() {
		t.Fatal("component should have content")
	}
	if got := component.Render(80); len(got) != 2 ||
		got[0] != "" || got[1] != "collapsed" {
		t.Fatalf("collapsed render = %#v", got)
	}

	component.SetExpanded(true)
	component.SetExpanded(true)
	if got := component.Render(80); len(got) != 2 ||
		got[1] != "expanded" {
		t.Fatalf("expanded render = %#v", got)
	}
	component.Invalidate()

	mu.Lock()
	defer mu.Unlock()
	if got := expandedValues; len(got) != 3 ||
		got[0] || !got[1] || !got[2] {
		t.Fatalf("expanded values = %#v", got)
	}
	if children[1].invalidations != 1 {
		t.Fatalf("invalidations = %d, want 1", children[1].invalidations)
	}
}

func TestCustomEntryComponentSuppressesEmptyAndRecoversRendererPanic(t *testing.T) {
	empty := NewCustomEntryComponent(
		FileEntry{Type: "custom", CustomType: "empty"},
		func(FileEntry, ProtocolEntryRenderOptions) gitui.Component {
			return nil
		},
	)
	if empty.HasContent() || len(empty.Render(80)) != 0 {
		t.Fatalf("empty component = content %t render %#v", empty.HasContent(), empty.Render(80))
	}

	failed := NewCustomEntryComponent(
		FileEntry{Type: "custom", CustomType: "broken"},
		func(FileEntry, ProtocolEntryRenderOptions) gitui.Component {
			panic("boom")
		},
	)
	rendered := strings.Join(failed.Render(80), "\n")
	if !failed.HasContent() ||
		!strings.Contains(StripAnsi(rendered), "[broken] renderer failed: boom") {
		t.Fatalf("failed render = %q", rendered)
	}
}

func TestCustomEntryComponentRejectsStaleConcurrentRebuild(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	renderer := func(
		_ FileEntry,
		options ProtocolEntryRenderOptions,
	) gitui.Component {
		if options.Expanded {
			close(started)
			<-release
			return gitui.NewText("stale-expanded", 0, 0)
		}
		return gitui.NewText("current-collapsed", 0, 0)
	}
	component := NewCustomEntryComponent(
		FileEntry{Type: "custom", CustomType: "concurrent"},
		renderer,
	)

	done := make(chan struct{})
	go func() {
		component.SetExpanded(true)
		close(done)
	}()
	<-started
	component.SetExpanded(false)
	close(release)
	<-done

	rendered := strings.Join(component.Render(80), "\n")
	if !strings.Contains(rendered, "current-collapsed") ||
		strings.Contains(rendered, "stale-expanded") {
		t.Fatalf("rendered = %q", rendered)
	}
}

func TestCLIInteractiveTUIHostRendersPersistedAndLiveCustomEntriesInOrder(t *testing.T) {
	manager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session := &AgentSession{SessionManager: manager}
	runtime := NewProtocolExtensionRuntime(CapabilityTUIMessageRenderer)
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{
		Path: "entries.gi.json",
		Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.RegisterEntryRenderer(
				"checkpoint",
				func(
					entry FileEntry,
					_ ProtocolEntryRenderOptions,
				) gitui.Component {
					data, _ := entry.Data.(map[string]any)
					label, _ := data["label"].(string)
					return gitui.NewText("entry:"+label, 0, 0)
				},
			)
		},
	}}); err != nil {
		t.Fatal(err)
	}
	runtime.BindSession(session)
	runtimeHost := &agentSessionPrintModeHost{session: session}

	manager.AppendCustomEntry(
		"checkpoint",
		map[string]any{"label": "before"},
	)
	manager.AppendMessage(sessionMessageValue(llm.UserMessageText("middle")))
	manager.AppendCustomEntry(
		"checkpoint",
		map[string]any{"label": "after"},
	)

	host := &CLIInteractiveTUIHost{
		runtimeHost: runtimeHost,
		chat:        gitui.NewContainer(),
	}
	host.renderSessionEntries(manager.BuildContextEntries(), false)
	rendered := StripAnsi(strings.Join(host.chat.Render(80), "\n"))
	before := strings.Index(rendered, "entry:before")
	middle := strings.Index(rendered, "middle")
	after := strings.Index(rendered, "entry:after")
	if before < 0 || middle <= before || after <= middle {
		t.Fatalf("history render order = %q", rendered)
	}

	liveHost := &CLIInteractiveTUIHost{
		runtimeHost: runtimeHost,
		chat:        gitui.NewContainer(),
	}
	streaming := newCLIAssistantMessageComponent(
		llm.Message{
			Role:    llm.RoleAssistant,
			Content: []llm.ContentPart{llm.Text("streaming")},
		},
		false,
		"",
		defaultOutputPad,
	)
	liveHost.liveState.setStreaming(
		llm.Message{Role: llm.RoleAssistant},
		streaming,
	)
	liveHost.chat.AddChild(streaming)
	liveHost.watchAgentSessionQueue()
	defer liveHost.removeSessionWatcher(true)

	if _, err := session.AppendCustomEntry(
		"checkpoint",
		map[string]any{"label": "live"},
	); err != nil {
		t.Fatal(err)
	}
	children := liveHost.chat.Children()
	if len(children) != 2 || children[1] != streaming {
		t.Fatalf("live children = %#v", children)
	}
	liveRendered := StripAnsi(strings.Join(liveHost.chat.Render(80), "\n"))
	if live := strings.Index(liveRendered, "entry:live"); live < 0 ||
		strings.Index(liveRendered, "streaming") <= live {
		t.Fatalf("live render order = %q", liveRendered)
	}
}

type customEntryTestComponent struct {
	text          string
	invalidations int
}

func (c *customEntryTestComponent) Render(int) []string {
	return []string{c.text}
}

func (c *customEntryTestComponent) Invalidate() {
	c.invalidations++
}
