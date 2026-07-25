package gicodingagent

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func TestProtocolExtensionRuntimeEntryRendererRegistryPiStyle(t *testing.T) {
	runtime := NewProtocolExtensionRuntime(CapabilityTUIMessageRenderer)
	mustLoadProtocolFactories(
		t,
		runtime,
		ProtocolExtensionFactory{
			Path: "first.gi.json",
			Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.RegisterEntryRenderer(
					"checkpoint",
					func(
						FileEntry,
						ProtocolEntryRenderOptions,
					) gitui.Component {
						return gitui.NewText("first", 0, 0)
					},
				)
			},
		},
		ProtocolExtensionFactory{
			Path: "second.gi.json",
			Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.RegisterEntryRenderer(
					"checkpoint",
					func(
						FileEntry,
						ProtocolEntryRenderOptions,
					) gitui.Component {
						return gitui.NewText("second", 0, 0)
					},
				)
			},
		},
	)

	renderer := runtime.GetEntryRenderer("checkpoint")
	if renderer == nil {
		t.Fatalf("renderer = %#v", renderer)
	}
	if rendered := renderer(
		FileEntry{Type: "custom", CustomType: "checkpoint"},
		ProtocolEntryRenderOptions{},
	).Render(80); len(rendered) != 1 ||
		strings.TrimSpace(rendered[0]) != "first" {
		t.Fatalf("rendered = %#v", rendered)
	}
	if runtime.GetEntryRenderer("missing") != nil {
		t.Fatal("missing renderer should be nil")
	}

	runtime.RemoveSource(ProtocolSourceInfo{Path: "first.gi.json"})
	renderer = runtime.GetEntryRenderer("checkpoint")
	if renderer == nil {
		t.Fatalf("renderer after source removal = %#v", renderer)
	}
	if rendered := renderer(
		FileEntry{Type: "custom", CustomType: "checkpoint"},
		ProtocolEntryRenderOptions{},
	).Render(80); len(rendered) != 1 ||
		strings.TrimSpace(rendered[0]) != "second" {
		t.Fatalf("rendered after source removal = %#v", rendered)
	}
}

func TestProtocolExtensionRuntimeBeforeProviderHeadersPiStyle(t *testing.T) {
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	var errorsSeen []ProtocolExtensionError
	runtime.OnError(func(event ProtocolExtensionError) {
		errorsSeen = append(errorsSeen, event)
	})
	mustLoadProtocolFactories(
		t,
		runtime,
		ProtocolExtensionFactory{
			Path: "failing.gi.json",
			Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.On(
					ProtocolEventBeforeProviderHeaders,
					func(event ProtocolSessionEvent) (
						ProtocolEventResult,
						error,
					) {
						event.Headers["X-Before-Error"] = "kept"
						return ProtocolEventResult{}, errors.New("header handler boom")
					},
				)
			},
		},
		ProtocolExtensionFactory{
			Path: "panicking.gi.json",
			Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.On(
					ProtocolEventBeforeProviderHeaders,
					func(ProtocolSessionEvent) (
						ProtocolEventResult,
						error,
					) {
						panic("header panic")
					},
				)
			},
		},
		ProtocolExtensionFactory{
			Path: "working.gi.json",
			Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.On(
					ProtocolEventBeforeProviderHeaders,
					func(event ProtocolSessionEvent) (
						ProtocolEventResult,
						error,
					) {
						if event.Context == nil ||
							event.Model == nil ||
							event.Model.ID != "test-model" {
							t.Fatalf("event = %#v", event)
						}
						event.Headers["X-Good"] =
							event.Headers["X-Before-Error"] + ":yes"
						delete(event.Headers, "X-Remove")
						return ProtocolEventResult{}, nil
					},
				)
			},
		},
	)

	input := map[string]string{
		"User-Agent": "gi-test",
		"X-Remove":   "remove-me",
	}
	model := llm.Model{ID: "test-model"}
	headers := runtime.EmitBeforeProviderHeaders(
		context.Background(),
		input,
		&model,
	)
	if headers["User-Agent"] != "gi-test" ||
		headers["X-Before-Error"] != "kept" ||
		headers["X-Good"] != "kept:yes" {
		t.Fatalf("headers = %#v", headers)
	}
	if _, ok := headers["X-Remove"]; ok {
		t.Fatalf("removed header survived: %#v", headers)
	}
	if input["X-Good"] != "" || input["X-Remove"] != "remove-me" {
		t.Fatalf("caller headers mutated: %#v", input)
	}
	if len(errorsSeen) != 2 ||
		errorsSeen[0].ExtensionPath != "failing.gi.json" ||
		errorsSeen[0].Event != ProtocolEventBeforeProviderHeaders ||
		!strings.Contains(errorsSeen[0].Error, "header handler boom") ||
		errorsSeen[1].ExtensionPath != "panicking.gi.json" ||
		!strings.Contains(errorsSeen[1].Error, "header panic") {
		t.Fatalf("errors = %#v", errorsSeen)
	}
}

func TestProtocolExtensionRuntimeRunnerStateAccessors(t *testing.T) {
	manager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            manager.GetCWD(),
		AgentDir:       t.TempDir(),
		Model:          sdkTestModel(),
		SessionManager: manager,
		CustomTools: []SDKTool{{
			Name:        "entry_test",
			Description: "entry test",
		}},
		Tools:    []string{"entry_test"},
		ToolsSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewProtocolExtensionRuntime()
	runtime.BindSession(session)
	if got := runtime.GetActiveTools(); !reflect.DeepEqual(
		got,
		[]string{"entry_test"},
	) {
		t.Fatalf("active tools = %#v", got)
	}
	got := runtime.GetActiveTools()
	got[0] = "mutated"
	if next := runtime.GetActiveTools(); !reflect.DeepEqual(
		next,
		[]string{"entry_test"},
	) {
		t.Fatalf("active tools alias session state: %#v", next)
	}

	registry := NewModelRegistry(NewInMemoryAuthStorage(nil), "")
	runtime.BindModelRegistry(registry)
	if runtime.GetModelRegistry() != registry {
		t.Fatalf(
			"model registry = %p, want %p",
			runtime.GetModelRegistry(),
			registry,
		)
	}

	otherRegistry := NewModelRegistry(NewInMemoryAuthStorage(nil), "")
	var bindings sync.WaitGroup
	bindings.Add(2)
	go func() {
		defer bindings.Done()
		for range 100 {
			runtime.BindModelRegistry(registry)
			runtime.BindModelRegistry(otherRegistry)
		}
	}()
	go func() {
		defer bindings.Done()
		for range 200 {
			_ = runtime.GetModelRegistry()
		}
	}()
	bindings.Wait()
	runtime.BindModelRegistry(registry)
	if runtime.GetModelRegistry() != registry {
		t.Fatalf(
			"model registry after concurrent access = %p, want %p",
			runtime.GetModelRegistry(),
			registry,
		)
	}
}
