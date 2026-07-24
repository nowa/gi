package giagentcore

import (
	"context"
	"strings"
	"testing"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestDefaultStreamFunctionCompatibility(t *testing.T) {
	previous, _ := GetDefaultStreamFn()
	t.Cleanup(func() {
		SetDefaultStreamFn(previous)
	})

	calls := 0
	SetDefaultStreamFn(func(_ llm.Model, _ llm.Context, _ llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
		calls++
		return testStream(testAssistantMessage([]llm.ContentPart{llm.Text("fallback")}, llm.StopReasonStop))(
			llm.Model{},
			llm.Context{},
			llm.SimpleStreamOptions{},
		)
	})

	agent := New()
	if err := agent.PromptText(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("default stream calls = %d, want 1", calls)
	}
}

func TestDefaultStreamFunctionMustBeConfigured(t *testing.T) {
	previous, _ := GetDefaultStreamFn()
	t.Cleanup(func() {
		SetDefaultStreamFn(previous)
	})
	SetDefaultStreamFn(nil)

	_, err := GetDefaultStreamFn()
	if err == nil || !strings.Contains(err.Error(), "SetDefaultStreamFn") {
		t.Fatalf("GetDefaultStreamFn() error = %v", err)
	}
}
