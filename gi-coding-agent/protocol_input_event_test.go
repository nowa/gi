package gicodingagent

import (
	"errors"
	"reflect"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestProtocolInputEventPiParity(t *testing.T) {
	t.Run("returns continue when no handlers undefined return or explicit continue", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityInputEvents)
		if got := runtime.EmitInput("x", nil, "interactive"); got.Action != "continue" {
			t.Fatalf("no handlers result = %#v", got)
		}
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "undefined", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.OnInput(func(ProtocolInputEvent) (ProtocolInputResult, error) {
				return ProtocolInputResult{}, nil
			})
		}})
		if got := runtime.EmitInput("x", nil, "interactive"); got.Action != "continue" {
			t.Fatalf("undefined handler result = %#v", got)
		}

		runtime = NewProtocolExtensionRuntime(CapabilityInputEvents)
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "continue", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.OnInput(func(ProtocolInputEvent) (ProtocolInputResult, error) {
				return ProtocolInputContinue(), nil
			})
		}})
		if got := runtime.EmitInput("x", nil, "interactive"); got.Action != "continue" {
			t.Fatalf("explicit continue result = %#v", got)
		}
	})

	t.Run("transforms text and preserves images when omitted", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityInputEvents)
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "transform", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.OnInput(func(event ProtocolInputEvent) (ProtocolInputResult, error) {
				return ProtocolInputTransform("T:" + event.Text), nil
			})
		}})
		images := []llm.ContentPart{llm.Image("orig", "image/png")}
		got := runtime.EmitInput("hi", images, "interactive")
		want := ProtocolInputResult{Action: "transform", Text: "T:hi", Images: images, ImagesSet: true}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("transform result = %#v, want %#v", got, want)
		}
	})

	t.Run("transforms and replaces images when provided", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityInputEvents)
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "images", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.OnInput(func(ProtocolInputEvent) (ProtocolInputResult, error) {
				return ProtocolInputTransformWithImages("X", []llm.ContentPart{llm.Image("new", "image/jpeg")}), nil
			})
		}})
		got := runtime.EmitInput("hi", []llm.ContentPart{llm.Image("orig", "image/png")}, "interactive")
		want := ProtocolInputResult{Action: "transform", Text: "X", Images: []llm.ContentPart{llm.Image("new", "image/jpeg")}, ImagesSet: true}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("image transform result = %#v, want %#v", got, want)
		}
	})

	t.Run("chains transforms across multiple handlers", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityInputEvents)
		mustLoadProtocolFactories(t, runtime,
			ProtocolExtensionFactory{Path: "first", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.OnInput(func(event ProtocolInputEvent) (ProtocolInputResult, error) {
					return ProtocolInputTransform(event.Text + "[1]"), nil
				})
			}},
			ProtocolExtensionFactory{Path: "second", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.OnInput(func(event ProtocolInputEvent) (ProtocolInputResult, error) {
					return ProtocolInputTransform(event.Text + "[2]"), nil
				})
			}},
		)
		got := runtime.EmitInput("X", nil, "interactive")
		if got.Action != "transform" || got.Text != "X[1][2]" || got.Images != nil {
			t.Fatalf("chained transform result = %#v", got)
		}
	})

	t.Run("short-circuits on handled and skips subsequent handlers", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityInputEvents)
		called := false
		mustLoadProtocolFactories(t, runtime,
			ProtocolExtensionFactory{Path: "handled", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.OnInput(func(ProtocolInputEvent) (ProtocolInputResult, error) {
					return ProtocolInputHandled(), nil
				})
			}},
			ProtocolExtensionFactory{Path: "skipped", Factory: func(ctx *ProtocolExtensionContext) error {
				return ctx.OnInput(func(ProtocolInputEvent) (ProtocolInputResult, error) {
					called = true
					return ProtocolInputContinue(), nil
				})
			}},
		)
		if got := runtime.EmitInput("X", nil, "interactive"); got.Action != "handled" {
			t.Fatalf("handled result = %#v", got)
		}
		if called {
			t.Fatal("handler after handled should not run")
		}
	})

	t.Run("passes source correctly for all source types", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityInputEvents)
		var seen []string
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "source", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.OnInput(func(event ProtocolInputEvent) (ProtocolInputResult, error) {
				seen = append(seen, event.Source)
				return ProtocolInputContinue(), nil
			})
		}})
		for _, source := range []string{"interactive", "rpc", "extension"} {
			runtime.EmitInput("x", nil, source)
		}
		if !reflect.DeepEqual(seen, []string{"interactive", "rpc", "extension"}) {
			t.Fatalf("sources = %#v", seen)
		}
	})

	t.Run("catches handler errors and continues", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityInputEvents)
		var errorsSeen []ProtocolExtensionError
		runtime.OnError(func(event ProtocolExtensionError) {
			errorsSeen = append(errorsSeen, event)
		})
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "boom.ts", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.OnInput(func(ProtocolInputEvent) (ProtocolInputResult, error) {
				return ProtocolInputResult{}, errors.New("boom")
			})
		}})
		got := runtime.EmitInput("x", nil, "interactive")
		if got.Action != "continue" {
			t.Fatalf("error handler result = %#v", got)
		}
		if len(errorsSeen) != 1 || errorsSeen[0].Error != "boom" || errorsSeen[0].Event != "input" || errorsSeen[0].ExtensionPath != "boom.ts" {
			t.Fatalf("errors = %#v", errorsSeen)
		}
	})

	t.Run("hasHandlers returns correct value", func(t *testing.T) {
		runtime := NewProtocolExtensionRuntime(CapabilityInputEvents)
		if runtime.HasHandlers("input") {
			t.Fatal("empty runtime should not have input handlers")
		}
		mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "input", Factory: func(ctx *ProtocolExtensionContext) error {
			return ctx.OnInput(func(ProtocolInputEvent) (ProtocolInputResult, error) {
				return ProtocolInputContinue(), nil
			})
		}})
		if !runtime.HasHandlers("input") {
			t.Fatal("runtime should report input handlers")
		}
	})
}

func mustLoadProtocolFactories(t *testing.T, runtime *ProtocolExtensionRuntime, factories ...ProtocolExtensionFactory) {
	t.Helper()
	if err := runtime.LoadFactories(factories); err != nil {
		t.Fatal(err)
	}
}
