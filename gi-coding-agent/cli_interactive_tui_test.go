package gicodingagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
	gitui "github.com/nowa/gi/gi-tui"
)

func TestCLIInteractiveTUIHostRendersInitialPromptAndViewTreeSlots(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	viewHost := NewViewTreeHost()
	if err := viewHost.Mount("package-widget", "aboveEditor", ViewTreeNode{Type: "text", Text: "Package widget"}); err != nil {
		t.Fatal(err)
	}
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:      runtimeHost,
		InitialMessage:   "hello",
		Terminal:         terminal,
		ViewTreeHost:     viewHost,
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	terminal.WaitForRender()
	viewport := strings.Join(terminal.GetViewport(), "\n")
	for _, expected := range []string{"Package widget", "hello", "Response to: hello"} {
		if !strings.Contains(viewport, expected) {
			t.Fatalf("viewport missing %q:\n%s", expected, viewport)
		}
	}
}

func TestCLIInteractiveTUIHostRendersLiveSessionEventsPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	terminal := gitui.NewVirtualTerminal(110, 28)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	user := llm.Message{Role: llm.RoleUser, Content: []llm.ContentPart{llm.Text("live user prompt")}}
	sessionHost.session.emit(AgentSessionEvent{Type: "message_start", Message: &user})
	waitForViewport(t, terminal, "live user prompt")

	assistantStart := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("final should not flash first")}}
	sessionHost.session.emit(AgentSessionEvent{Type: "message_start", Message: &assistantStart})
	partial := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("partial answer")}}
	sessionHost.session.emit(AgentSessionEvent{
		Type: "message_update",
		AssistantMessageEvent: &llm.AssistantMessageEvent{
			Type:    "text_delta",
			Partial: partial,
		},
	})
	waitForViewport(t, terminal, "partial answer")

	toolPartial := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{
		llm.Text("partial answer"),
		llm.ToolCall("live-tool", "demo_tool", map[string]any{"path": "notes.txt"}),
	}}
	sessionHost.session.emit(AgentSessionEvent{
		Type: "message_update",
		AssistantMessageEvent: &llm.AssistantMessageEvent{
			Type:    "toolcall_end",
			Partial: toolPartial,
			ToolCall: llm.ToolCall("live-tool", "demo_tool", map[string]any{
				"path": "notes.txt",
			}),
		},
	})
	waitForViewport(t, terminal, "demo_tool")

	result := llm.Message{
		Role:       llm.RoleToolResult,
		ToolCallID: "live-tool",
		ToolName:   "demo_tool",
		Content:    []llm.ContentPart{llm.Text("live tool result")},
	}
	sessionHost.session.emit(AgentSessionEvent{
		Type:       "tool_execution_end",
		ToolCallID: "live-tool",
		ToolName:   "demo_tool",
		Args:       map[string]any{"path": "notes.txt"},
		ToolResult: &result,
	})
	waitForViewport(t, terminal, "live tool result")

	final := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("final answer")}, StopReason: llm.StopReasonStop}
	sessionHost.session.emit(AgentSessionEvent{Type: "message_end", Message: &final})
	waitForViewport(t, terminal, "final answer")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRendersExtensionErrorsPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	mustLoadProtocolFactories(t, runtime, ProtocolExtensionFactory{Path: "throws.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.On("context", func(ProtocolSessionEvent) (ProtocolEventResult, error) {
			return ProtocolEventResult{}, errors.New("Handler error!")
		})
	}})
	sessionHost.session.ExtensionRuntime = runtime
	runtimeHostBridge, err := NewAgentSessionRuntimeHost(sessionHost.session, runtime)
	if err != nil {
		t.Fatal(err)
	}
	runtimeHostBridge.SetRebindSession(func(session *AgentSession) error {
		sessionHost.session = session
		return nil
	})
	sessionHost.sessionRuntimeHost = runtimeHostBridge

	terminal := gitui.NewVirtualTerminal(110, 28)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	if _, err := runtime.EmitSessionEvent(ProtocolSessionEvent{Type: "context"}); err == nil {
		t.Fatal("expected extension handler error")
	}
	waitForViewport(t, terminal, `Extension "throws.gi.json" error: Handler error!`)

	runtime.emitExtensionError(ProtocolExtensionError{
		ExtensionPath: "stack.gi.json",
		Error:         "Boom",
		Stack:         "Error: Boom\n    at run (/ext/index.js:1:2)\n    at main (/ext/index.js:3:4)",
	})
	waitForViewport(t, terminal, `Extension "stack.gi.json" error: Boom`)
	waitForViewport(t, terminal, "at run (/ext/index.js:1:2)")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostCoalescesSequentialStatusPiStyle(t *testing.T) {
	host := &CLIInteractiveTUIHost{chat: gitui.NewContainer()}
	first := host.addStatus("STATUS_ONE")
	second := host.addStatus("STATUS_TWO")
	if first == nil || first != second {
		t.Fatalf("status component was not reused: first=%p second=%p", first, second)
	}
	if children := host.chat.Children(); len(children) != 2 {
		t.Fatalf("children after coalesced statuses = %d, want 2", len(children))
	}
	rendered := strings.Join(host.chat.Render(80), "\n")
	if strings.Contains(rendered, "STATUS_ONE") || !strings.Contains(rendered, "STATUS_TWO") {
		t.Fatalf("rendered coalesced status =\n%s", rendered)
	}

	host.chat.AddChild(gitui.NewText("OTHER", 1, 0))
	host.addStatus("STATUS_THREE")
	if children := host.chat.Children(); len(children) != 5 {
		t.Fatalf("children after intervening message = %d, want 5", len(children))
	}
	rendered = strings.Join(host.chat.Render(80), "\n")
	for _, expected := range []string{"STATUS_TWO", "OTHER", "STATUS_THREE"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered status sequence missing %q:\n%s", expected, rendered)
		}
	}

	host.addStatus("Error: FIRST")
	host.addStatus("Error: SECOND")
	rendered = strings.Join(host.chat.Render(80), "\n")
	if !strings.Contains(rendered, "Error: FIRST") || !strings.Contains(rendered, "Error: SECOND") {
		t.Fatalf("diagnostic statuses should not coalesce:\n%s", rendered)
	}
}

func TestCLIInteractiveTUIHostAgentEndCleansStreamingStatePiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	terminal := gitui.NewVirtualTerminal(110, 28)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	assistantStart := llm.Message{Role: llm.RoleAssistant}
	sessionHost.session.emit(AgentSessionEvent{Type: "message_start", Message: &assistantStart})
	partial := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("orphan partial answer")}}
	sessionHost.session.emit(AgentSessionEvent{
		Type: "message_update",
		AssistantMessageEvent: &llm.AssistantMessageEvent{
			Type:    "text_delta",
			Partial: partial,
		},
	})
	waitForViewport(t, terminal, "orphan partial answer")
	result := llm.Message{
		Role:       llm.RoleToolResult,
		ToolCallID: "orphan-tool",
		ToolName:   "demo_tool",
		Content:    []llm.ContentPart{llm.Text("partial result")},
	}
	sessionHost.session.emit(AgentSessionEvent{
		Type:       "tool_execution_start",
		ToolCallID: "orphan-tool",
		ToolName:   "demo_tool",
		Args:       map[string]any{"path": "notes.txt"},
		ToolResult: &result,
	})
	waitUntil(t, func() bool { return host.streamingComponent != nil && len(host.pendingTools) == 1 })

	sessionHost.session.emit(AgentSessionEvent{Type: "agent_end"})
	waitUntil(t, func() bool {
		return host.streamingComponent == nil && host.streamingMessage == nil && len(host.pendingTools) == 0
	})
	terminal.WaitForRender()
	if viewport := strings.Join(terminal.GetViewport(), "\n"); strings.Contains(viewport, "orphan partial answer") {
		t.Fatalf("agent_end left orphan streaming assistant visible:\n%s", viewport)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostMirrorsPiTerminalProgressEvents(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.settingsManager.SetShowTerminalProgress(true)
	terminal := &recordingProgressTerminal{VirtualTerminal: gitui.NewVirtualTerminal(100, 20)}
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	sessionHost.session.emit(AgentSessionEvent{Type: "agent_start"})
	waitForProgressSequence(t, terminal, []bool{true})
	sessionHost.session.emit(AgentSessionEvent{Type: "agent_end"})
	waitForProgressSequence(t, terminal, []bool{true, false})
	sessionHost.session.emit(AgentSessionEvent{Type: "compaction_start"})
	waitForProgressSequence(t, terminal, []bool{true, false, true})
	sessionHost.session.emit(AgentSessionEvent{Type: "compaction_end"})
	waitForProgressSequence(t, terminal, []bool{true, false, true, false})

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRendersPartialToolExecutionUpdatesPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityTUIToolRenderer)
	runtime.BindSession(sessionHost.session)
	ctx := &ProtocolExtensionContext{runtime: runtime, source: ProtocolSourceInfo{Path: "partial-tool-renderer.gi.json", Source: "local:test", Scope: "temporary", Origin: "package"}}
	if err := ctx.RegisterToolRenderer("stream_tool", ProtocolToolRendererDefinition{
		RenderCall: func(args any, context ToolRenderContext) []string {
			return []string{"Stream tool"}
		},
		RenderResult: func(result FileToolResult, options ToolRenderResultOptions, context ToolRenderContext) []string {
			prefix := "final"
			if options.IsPartial && context.IsPartial {
				prefix = "partial"
			}
			return []string{prefix + ": " + fileToolResultText(result)}
		},
	}); err != nil {
		t.Fatal(err)
	}
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	sessionHost.session.emit(AgentSessionEvent{Type: "tool_execution_start", ToolCallID: "stream-call", ToolName: "stream_tool", Args: map[string]any{}})
	sessionHost.session.emit(AgentSessionEvent{
		Type:              ProtocolEventToolExecutionUpdate,
		ToolCallID:        "stream-call",
		ToolName:          "stream_tool",
		Args:              map[string]any{},
		PartialToolResult: &llm.Message{Role: llm.RoleToolResult, ToolCallID: "stream-call", ToolName: "stream_tool", Content: []llm.ContentPart{llm.Text("chunk 1")}},
	})
	waitForViewport(t, terminal, "partial: chunk 1")
	sessionHost.session.emit(AgentSessionEvent{
		Type:       "tool_execution_end",
		ToolCallID: "stream-call",
		ToolName:   "stream_tool",
		Args:       map[string]any{},
		ToolResult: &llm.Message{Role: llm.RoleToolResult, ToolCallID: "stream-call", ToolName: "stream_tool", Content: []llm.ContentPart{llm.Text("complete")}},
	})
	waitForViewport(t, terminal, "final: complete")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostMountsTrustedInProcessSlotComponents(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	registry := NewInProcessUIRegistry()
	contextOK := false
	disposed := false
	registry.SetHeader("trusted-header", func(ctx InProcessComponentContext) (gitui.Component, func(), error) {
		contextOK = ctx.Session != nil && ctx.ViewTree != nil && ctx.RuntimeHost != nil
		return gitui.NewText("Trusted Header", 1, 0), nil, nil
	})
	registry.SetWidget("trusted-widget", "aboveEditor", func(InProcessComponentContext) (gitui.Component, func(), error) {
		return gitui.NewText("Trusted Widget", 1, 0), func() { disposed = true }, nil
	})
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		InProcessUI: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForViewport(t, terminal, "Trusted Header")
	waitForViewport(t, terminal, "Trusted Widget")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
	if !disposed {
		t.Fatal("in-process component disposer was not called")
	}
	if !contextOK {
		t.Fatal("in-process component context did not include session, runtime host, and view tree")
	}
}

func TestCLIInteractiveTUIHostRefreshesTrustedInProcessSlotComponents(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	registry := NewInProcessUIRegistry()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		InProcessUI: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	disposed := 0
	registry.SetWidget("runtime-widget", "belowEditor", func(InProcessComponentContext) (gitui.Component, func(), error) {
		return gitui.NewText("Runtime Widget v1", 1, 0), func() { disposed++ }, nil
	})
	waitForViewport(t, terminal, "Runtime Widget v1")
	waitForSlotChildCount(t, host, "belowEditor", 1)

	registry.SetWidget("runtime-widget", "belowEditor", func(InProcessComponentContext) (gitui.Component, func(), error) {
		return gitui.NewText("Runtime Widget v2", 1, 0), func() { disposed++ }, nil
	})
	waitForViewport(t, terminal, "Runtime Widget v2")
	waitForSlotChildCount(t, host, "belowEditor", 1)
	if disposed != 1 {
		t.Fatalf("old in-process component dispose count = %d, want 1", disposed)
	}

	if !registry.Remove("runtime-widget") {
		t.Fatal("expected runtime widget removal to succeed")
	}
	waitForSlotChildCount(t, host, "belowEditor", 0)
	if disposed != 2 {
		t.Fatalf("removed in-process component dispose count = %d, want 2", disposed)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostPropagatesToolsExpandedToTrustedInProcessComponents(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	registry := NewInProcessUIRegistry()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		InProcessUI: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	header := &trustedExpandableComponent{label: "Trusted header"}
	registry.SetHeader("trusted-expandable-header", func(InProcessComponentContext) (gitui.Component, func(), error) {
		return header, nil, nil
	})
	waitForViewport(t, terminal, "Trusted header expanded: false")
	waitForCondition(t, func() bool { return header.lastExpanded() == false && len(header.expandedValues()) > 0 }, "trusted header initial collapsed state")

	if err := host.SetTUIToolsExpanded(true); err != nil {
		t.Fatal(err)
	}
	waitForViewport(t, terminal, "Trusted header expanded: true")
	waitForCondition(t, func() bool { return header.lastExpanded() }, "trusted header expanded state")

	footer := &trustedExpandableComponent{label: "Trusted footer"}
	registry.SetFooter("trusted-expandable-footer", func(InProcessComponentContext) (gitui.Component, func(), error) {
		return footer, nil, nil
	})
	waitForViewport(t, terminal, "Trusted footer expanded: true")
	waitForCondition(t, func() bool { return footer.lastExpanded() }, "trusted footer to inherit current expanded state")

	if err := host.SetTUIToolsExpanded(false); err != nil {
		t.Fatal(err)
	}
	waitForViewport(t, terminal, "Trusted header expanded: false")
	waitForViewport(t, terminal, "Trusted footer expanded: false")
	waitForCondition(t, func() bool { return !header.lastExpanded() && !footer.lastExpanded() }, "trusted components collapsed state")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostMountsTrustedInProcessEditorComponent(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	registry := NewInProcessUIRegistry()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		InProcessUI: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	disposed := false
	registry.SetEditor("trusted-editor", func(InProcessComponentContext) (gitui.Component, func(), error) {
		return gitui.NewText("Trusted Editor", 1, 0), func() { disposed = true }, nil
	})
	waitForViewport(t, terminal, "Trusted Editor")
	waitForEditorContainerChildCount(t, host, 1)
	if !host.customEditorActive || host.ui.FocusedComponent() == host.editor {
		t.Fatalf("trusted in-process editor should replace default editor, custom=%v focused=%T", host.customEditorActive, host.ui.FocusedComponent())
	}

	if !registry.Remove("trusted-editor") {
		t.Fatal("expected trusted editor removal to succeed")
	}
	waitForEditorContainerChildCount(t, host, 1)
	if host.customEditorActive || host.ui.FocusedComponent() != host.editor {
		t.Fatalf("default editor should be restored, custom=%v focused=%T", host.customEditorActive, host.ui.FocusedComponent())
	}
	if !disposed {
		t.Fatal("trusted in-process editor disposer was not called")
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostForwardsTrustedInProcessEditorInput(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	registry := NewInProcessUIRegistry()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		InProcessUI: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	editor := &trustedInProcessInputComponent{}
	registry.SetEditor("trusted-input-editor", func(InProcessComponentContext) (gitui.Component, func(), error) {
		return editor, nil, nil
	})
	waitForViewport(t, terminal, "Trusted input:")
	waitForCondition(t, func() bool { return editor.Focused() }, "trusted in-process editor to receive focus")

	terminal.SendInput("x")
	waitForViewport(t, terminal, "Trusted input: x")

	terminal.SendInput("y")
	waitForViewport(t, terminal, "Trusted input: xy")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRoutesEditorHostActionsToTrustedInProcessEditor(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	registry := NewInProcessUIRegistry()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		InProcessUI: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	customEditor := gitui.NewEditor(gitui.EditorTheme{Border: func(text string) string { return text }}, gitui.EditorOptions{Borderless: true})
	registry.SetEditor("trusted-rich-editor", func(InProcessComponentContext) (gitui.Component, func(), error) {
		return customEditor, nil, nil
	})
	waitForCondition(t, func() bool { return customEditor.Focused() }, "trusted editor to receive focus")

	host.SetEditorText("alpha")
	waitForViewport(t, terminal, "alpha")
	if got := host.ReadEditorText(); got != "alpha" {
		t.Fatalf("active editor text = %q, want alpha", got)
	}
	if got := host.editor.GetExpandedText(); got == "alpha" {
		t.Fatalf("default editor received custom editor text %q", got)
	}

	host.InsertEditorText(" beta")
	waitForViewport(t, terminal, "alpha beta")
	host.PasteEditorText("\nnext")
	want := "alpha beta\nnext"
	if got := host.ReadEditorText(); got != want {
		t.Fatalf("active editor text after paste = %q, want %q", got, want)
	}

	if !registry.Remove("trusted-rich-editor") {
		t.Fatal("expected trusted editor removal to succeed")
	}
	waitForCondition(t, func() bool { return host.ui.FocusedComponent() == host.editor }, "default editor to regain focus")
	if got := host.ReadEditorText(); got != want {
		t.Fatalf("default editor should preserve custom text after removal, got %q want %q", got, want)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostSubmitsTrustedInProcessEditorText(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	registry := NewInProcessUIRegistry()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		InProcessUI: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	customEditor := gitui.NewEditor(gitui.EditorTheme{Border: func(text string) string { return text }}, gitui.EditorOptions{Borderless: true})
	registry.SetEditor("trusted-submit-editor", func(InProcessComponentContext) (gitui.Component, func(), error) {
		return customEditor, nil, nil
	})
	waitForCondition(t, func() bool { return customEditor.Focused() }, "trusted editor to receive focus")

	host.SetEditorText("custom submit")
	if err := host.SubmitEditorText(); err != nil {
		t.Fatal(err)
	}
	waitForViewport(t, terminal, "Response to: custom submit")
	if got := host.ReadEditorText(); got != "" {
		t.Fatalf("custom editor text after submit = %q, want empty", got)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostForwardsTrustedInProcessKeyReleaseOptIn(t *testing.T) {
	gitui.SetKittyProtocolActive(true)
	t.Cleanup(func() { gitui.SetKittyProtocolActive(false) })

	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	registry := NewInProcessUIRegistry()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		InProcessUI: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	editor := &trustedInProcessInputComponent{wantsKeyRelease: true}
	registry.SetEditor("trusted-release-editor", func(InProcessComponentContext) (gitui.Component, func(), error) {
		return editor, nil, nil
	})
	waitForViewport(t, terminal, "Trusted input:")

	terminal.SendInput("\x1b[97;1:3u")
	waitForCondition(t, func() bool { return editor.received("\x1b[97;1:3u") }, "trusted in-process editor to receive key-release input")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostMountsTrustedInProcessOverlayComponent(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	registry := NewInProcessUIRegistry()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		InProcessUI: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	disposed := false
	registry.SetOverlay("trusted-overlay", func(InProcessComponentContext) (gitui.Component, func(), error) {
		return gitui.NewText("Trusted Overlay", 1, 0), func() { disposed = true }, nil
	}, gitui.OverlayOptions{Anchor: gitui.OverlayTopLeft, Width: &gitui.SizeValue{Value: 30}})
	waitForViewport(t, terminal, "Trusted Overlay")
	if !host.ui.HasOverlay() {
		t.Fatal("trusted in-process overlay should be mounted")
	}

	if !registry.Remove("trusted-overlay") {
		t.Fatal("expected trusted overlay removal to succeed")
	}
	waitForNoOverlay(t, host)
	if !disposed {
		t.Fatal("trusted in-process overlay disposer was not called")
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostTrustedInProcessNonCapturingOverlayPreservesEditorInput(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	registry := NewInProcessUIRegistry()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		InProcessUI: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	registry.SetOverlay("trusted-hint", func(InProcessComponentContext) (gitui.Component, func(), error) {
		return gitui.NewText("Trusted Hint", 1, 0), nil, nil
	}, gitui.OverlayOptions{Anchor: gitui.OverlayBottomCenter, NonCapturing: true})
	waitForViewport(t, terminal, "Trusted Hint")
	if host.ui.FocusedComponent() != host.editor {
		t.Fatalf("non-capturing trusted overlay should preserve editor focus, focused=%T", host.ui.FocusedComponent())
	}

	terminal.SendInput("z")
	waitForEditorText(t, host, "z")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShowsTrustedInProcessCustomEditorWorkflow(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	registry := NewInProcessUIRegistry()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		InProcessUI: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	host.editor.SetText("default draft")

	disposed := false
	custom := &trustedInProcessCustomComponent{}
	handle, err := registry.ShowCustom("trusted-custom", func(_ InProcessComponentContext, done InProcessCustomDone) (gitui.Component, func(), error) {
		custom.done = done
		return custom, func() { disposed = true }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForViewport(t, terminal, "Trusted custom:")
	waitForCondition(t, func() bool { return custom.Focused() }, "trusted custom component to receive focus")

	terminal.SendInput("x")
	waitForViewport(t, terminal, "Trusted custom: x")
	terminal.SendInput("\r")
	result := waitForInProcessCustomResult(t, handle.Done())
	if result.Err != nil || result.Cancelled || result.Value != "accepted:x" {
		t.Fatalf("custom result = %#v", result)
	}
	waitForCondition(t, func() bool { return host.ui.FocusedComponent() == host.editor }, "default editor focus to be restored")
	if got := host.editor.GetText(); got != "default draft" {
		t.Fatalf("default editor text = %q", got)
	}
	waitForCondition(t, func() bool { return disposed }, "trusted custom component disposer to run")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShowsTrustedInProcessCustomOverlayWorkflow(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	registry := NewInProcessUIRegistry()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		InProcessUI: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	host.editor.SetText("overlay draft")

	disposed := false
	custom := &trustedInProcessCustomComponent{}
	overlay := gitui.OverlayOptions{Anchor: gitui.OverlayTopCenter, Width: &gitui.SizeValue{Value: 32}}
	handle, err := registry.ShowCustom("trusted-custom-overlay", func(_ InProcessComponentContext, done InProcessCustomDone) (gitui.Component, func(), error) {
		custom.done = done
		return custom, func() { disposed = true }, nil
	}, InProcessCustomOptions{Overlay: &overlay})
	if err != nil {
		t.Fatal(err)
	}
	waitForViewport(t, terminal, "Trusted custom:")
	if !host.ui.HasOverlay() {
		t.Fatal("trusted custom overlay should be mounted")
	}
	waitForCondition(t, func() bool { return custom.Focused() }, "trusted custom overlay to receive focus")

	terminal.SendInput("o")
	waitForViewport(t, terminal, "Trusted custom: o")
	terminal.SendInput("\r")
	result := waitForInProcessCustomResult(t, handle.Done())
	if result.Err != nil || result.Cancelled || result.Value != "accepted:o" {
		t.Fatalf("custom overlay result = %#v", result)
	}
	waitForNoOverlay(t, host)
	waitForCondition(t, func() bool { return host.ui.FocusedComponent() == host.editor }, "default editor focus to be restored")
	if got := host.editor.GetText(); got != "overlay draft" {
		t.Fatalf("default editor text = %q", got)
	}
	waitForCondition(t, func() bool { return disposed }, "trusted custom overlay disposer to run")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRecoversTrustedInProcessRenderPanic(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	registry := NewInProcessUIRegistry()
	registry.SetWidget("panic-widget", "aboveEditor", func(InProcessComponentContext) (gitui.Component, func(), error) {
		return panicRenderComponent{}, nil, nil
	})
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		InProcessUI: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForViewport(t, terminal, "in-process component panic-widget render failed: boom")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostReportsTrustedInProcessInputPanic(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	registry := NewInProcessUIRegistry()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		InProcessUI: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	registry.SetEditor("panic-input", func(InProcessComponentContext) (gitui.Component, func(), error) {
		return panicInputComponent{}, nil, nil
	})
	waitForViewport(t, terminal, "Panic Input")
	terminal.SendInput("x")
	waitForViewport(t, terminal, "in-process component panic-input input failed: boom")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

type panicRenderComponent struct{}

func (panicRenderComponent) Render(int) []string {
	panic("boom")
}

func (panicRenderComponent) Invalidate() {}

type panicInputComponent struct{}

func (panicInputComponent) Render(int) []string { return []string{"Panic Input"} }

func (panicInputComponent) Invalidate() {}

func (panicInputComponent) HandleInput(string) {
	panic("boom")
}

type trustedInProcessInputComponent struct {
	gitui.FocusState
	mu              sync.Mutex
	text            string
	inputs          []string
	wantsKeyRelease bool
}

func (c *trustedInProcessInputComponent) Render(width int) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return []string{gitui.TruncateToWidth("Trusted input: "+c.text, width, "...")}
}

func (c *trustedInProcessInputComponent) Invalidate() {}

func (c *trustedInProcessInputComponent) HandleInput(data string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inputs = append(c.inputs, data)
	if !gitui.IsKeyRelease(data) {
		c.text += data
	}
}

func (c *trustedInProcessInputComponent) WantsKeyRelease() bool {
	return c.wantsKeyRelease
}

func (c *trustedInProcessInputComponent) received(data string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, input := range c.inputs {
		if input == data {
			return true
		}
	}
	return false
}

type trustedInProcessCustomComponent struct {
	gitui.FocusState
	mu   sync.Mutex
	text string
	done InProcessCustomDone
}

func (c *trustedInProcessCustomComponent) Render(width int) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return []string{gitui.TruncateToWidth("Trusted custom: "+c.text, width, "...")}
}

func (c *trustedInProcessCustomComponent) Invalidate() {}

func (c *trustedInProcessCustomComponent) HandleInput(data string) {
	c.mu.Lock()
	if data == "\r" {
		value := "accepted:" + c.text
		done := c.done
		c.mu.Unlock()
		if done != nil {
			done(value)
		}
		return
	}
	c.text += data
	c.mu.Unlock()
}

type trustedExpandableComponent struct {
	mu       sync.Mutex
	label    string
	expanded bool
	values   []bool
}

func (c *trustedExpandableComponent) Render(width int) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return []string{gitui.TruncateToWidth(fmt.Sprintf("%s expanded: %v", c.label, c.expanded), width, "...")}
}

func (c *trustedExpandableComponent) Invalidate() {}

func (c *trustedExpandableComponent) SetExpanded(expanded bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expanded = expanded
	c.values = append(c.values, expanded)
}

func (c *trustedExpandableComponent) lastExpanded() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.expanded
}

func (c *trustedExpandableComponent) expandedValues() []bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]bool(nil), c.values...)
}

func TestCLIInteractiveTUIHostUsesPiStyleFlowLayout(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(80, 12)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForViewport(t, terminal, "gi v0.0.0")

	viewport := terminal.GetViewport()
	viewportText := strings.Join(viewport, "\n")
	if !strings.Contains(viewportText, "gi v0.0.0") {
		t.Fatalf("viewport missing title:\n%s", strings.Join(viewport, "\n"))
	}
	if countExactViewportLine(viewport, "gi v0.0.0") != 1 {
		t.Fatalf("startup should render one Gi header, viewport:\n%s", viewportText)
	}
	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIStartupHelpUsesEffectiveKeybindingsPiStyle(t *testing.T) {
	previous := gitui.GetKeybindings()
	gitui.SetKeybindings(gitui.NewKeybindingsManager(gitui.KeybindingsConfig{
		"tui.input.submit":             []string{"ctrl+enter"},
		"tui.editor.deleteToLineEnd":   []string{"ctrl+e"},
		"tui.editor.deleteCharForward": []string{"delete"},
	}))
	t.Cleanup(func() { gitui.SetKeybindings(previous) })

	keybindings := mergeKeybindingsConfig(DefaultProtocolKeybindings(), KeybindingsConfig{
		"app.interrupt":    "ctrl+i",
		"app.clear":        "ctrl+x",
		"app.exit":         "ctrl+q",
		"app.tools.expand": "ctrl+y",
	})
	header := newCLIStartupHeaderComponent(DefaultCodingAgentVersion, false, keybindings)
	rendered := StripAnsi(strings.Join(header.Render(120), "\n"))
	for _, expected := range []string{"ctrl+i interrupt", "ctrl+x/ctrl+q clear/exit", "ctrl+y more"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("compact startup help missing %q:\n%s", expected, rendered)
		}
	}
	header.SetExpanded(true)
	rendered = StripAnsi(strings.Join(header.Render(120), "\n"))
	for _, expected := range []string{"ctrl+x to clear", "ctrl+x twice to exit", "ctrl+e to delete to end", "ctrl+y to expand tools"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expanded startup help missing %q:\n%s", expected, rendered)
		}
	}

	header.SetExpanded(false)
	rendered = StripAnsi(strings.Join(header.Render(120), "\n"))
	if !strings.Contains(rendered, "Press ctrl+y to show full startup help and loaded resources.") {
		t.Fatalf("compact startup help missing expanded-help hint:\n%s", rendered)
	}
}

func TestCLIInteractiveTUIHostRendersEditorAfterStartupContentPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(80, 18)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		ShowFooter:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("bottom check")
	waitForEditorText(t, host, "bottom check")
	terminal.WaitForRender()
	viewport := terminal.GetViewport()
	inputRow := -1
	for idx, line := range viewport {
		if strings.Contains(line, "bottom check") {
			inputRow = idx
			break
		}
	}
	headerRow := viewportLineIndex(viewport, "gi v0.0.0")
	footerRow := viewportLineIndex(viewport, "0.0%/")
	if headerRow < 0 || footerRow < 0 {
		t.Fatalf("viewport missing startup header or footer:\n%s", strings.Join(viewport, "\n"))
	}
	if inputRow <= headerRow || inputRow >= footerRow {
		t.Fatalf("editor input row = %d, want between startup header row %d and footer row %d:\n%s", inputRow, headerRow, footerRow, strings.Join(viewport, "\n"))
	}
	if inputRow >= footerRow-2 {
		t.Fatalf("editor should follow startup content instead of sitting against the footer, input row=%d footer row=%d:\n%s", inputRow, footerRow, strings.Join(viewport, "\n"))
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostAppliesPiInteractiveSettingsOnStartup(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cwd := t.TempDir()
	agentDir := filepath.Join(cwd, "agent")
	settings := NewSettingsManager(cwd, agentDir)
	settings.SetEditorPaddingX(2)
	settings.SetAutocompleteMaxVisible(10)
	settings.SetShowHardwareCursor(true)
	settings.SetClearOnShrink(true)
	settings.SetShowImages(false)
	settings.SetImageWidthCells(90)
	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		NoSession: true,
		Model:     "openai/gpt-4o-mini",
	}, CLIOptions{
		CWD:      cwd,
		AgentDir: agentDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtimeHost.Dispose() })
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    gitui.NewVirtualTerminal(80, 18),
	})
	if err != nil {
		t.Fatal(err)
	}

	host.buildUI()
	if host.editor.GetPaddingX() != 2 || host.editor.GetAutocompleteMaxVisible() != 10 ||
		!host.ui.ShowHardwareCursor() || !host.ui.ClearOnShrink() {
		t.Fatalf("startup settings not applied")
	}
	tool := host.newToolExecutionComponent("read", "tool-startup", map[string]any{"path": "image.png"})
	if tool.showImages || tool.imageWidthCells != 90 {
		t.Fatalf("startup tool image settings not applied: show=%v width=%d", tool.showImages, tool.imageWidthCells)
	}
}

func TestCLIInteractiveTUIHostWritesDebugLogPiStyle(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cwd := t.TempDir()
	agentDir := filepath.Join(cwd, "agent")
	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		NoSession: true,
		Model:     "openai/gpt-4o-mini",
	}, CLIOptions{CWD: cwd, AgentDir: agentDir})
	if err != nil {
		t.Fatal(err)
	}
	terminal := gitui.NewVirtualTerminal(90, 18)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		PackageName: DefaultCodingAgentPackageName,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForViewport(t, terminal, "Gi")

	terminal.SendInput("\x1b[68;6u")
	waitForViewport(t, terminal, "Debug log written")

	debugPath := filepath.Join(agentDir, "gi-debug.log")
	data, err := os.ReadFile(debugPath)
	if err != nil {
		t.Fatal(err)
	}
	debugLog := string(data)
	for _, expected := range []string{
		"Debug output at ",
		"Terminal: 90x18",
		"=== All rendered lines with visible widths ===",
		"=== Agent messages (JSONL) ===",
	} {
		if !strings.Contains(debugLog, expected) {
			t.Fatalf("debug log missing %q:\n%s", expected, debugLog)
		}
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostCanClearScreenBeforeStartup(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(80, 10)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:        runtimeHost,
		Terminal:           terminal,
		ClearScreenOnStart: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	terminal.WaitForRender()

	clearSequence := "\x1b[2J\x1b[H\x1b[3J"
	if output := terminal.Output(); !strings.Contains(output, clearSequence) {
		t.Fatalf("startup output should clear screen and scrollback before CLI TUI render, output=%q", output)
	}
	if output := terminal.Output(); strings.Count(output, clearSequence) < 2 {
		t.Fatalf("startup output should force a full redraw after terminal start, output=%q", output)
	}
	output := terminal.Output()
	firstFrame := strings.Index(output, "gi v0.0.0")
	forcedClear := strings.LastIndex(output, clearSequence)
	if firstFrame >= 0 && forcedClear >= 0 && firstFrame < forcedClear {
		t.Fatalf("startup should not draw a partial frame before the forced clear, output=%q", output)
	}
	if viewport := strings.Join(terminal.GetViewport(), "\n"); !strings.Contains(viewport, "Press ctrl+o") || !strings.Contains(viewport, strings.Repeat("─", 80)) {
		t.Fatalf("viewport missing CLI frame after startup clear:\n%s", viewport)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostKeepsStartupInputAndSlashAutocompleteInPiFlow(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(188, 56)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:        runtimeHost,
		Terminal:           terminal,
		ClearScreenOnStart: true,
		ShowFooter:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForViewport(t, terminal, "gi v0.0.0")

	input := "here is what I just input into."
	terminal.SendInput(input)
	waitForEditorText(t, host, input)
	waitForViewport(t, terminal, input)

	viewport := terminal.GetViewport()
	inputRow := viewportLineIndex(viewport, input)
	if inputRow < 0 {
		t.Fatalf("startup editor viewport missing input:\n%s", strings.Join(viewport, "\n"))
	}
	headerRow := viewportLineIndex(viewport, "gi v0.0.0")
	footerRow := viewportLineIndex(viewport, "0.0%/")
	if headerRow < 0 || footerRow < 0 {
		t.Fatalf("viewport missing startup header or footer:\n%s", strings.Join(viewport, "\n"))
	}
	if inputRow <= headerRow || inputRow >= footerRow {
		t.Fatalf("editor input row = %d, want between startup header row %d and footer row %d:\n%s", inputRow, headerRow, footerRow, strings.Join(viewport, "\n"))
	}

	host.SetEditorText("")
	waitForEditorText(t, host, "")
	terminal.SendInput("/")
	waitForViewport(t, terminal, "settings")
	waitForViewport(t, terminal, "model")
	if !host.editor.IsShowingAutocomplete() {
		t.Fatalf("slash should show builtin autocomplete")
	}
	viewport = terminal.GetViewport()
	slashRow := viewportTrimmedLineIndex(viewport, "/")
	settingsRow := viewportLineIndex(viewport, "settings")
	footerRow = viewportLineIndex(viewport, "0.0%/")
	if slashRow < 0 || settingsRow < 0 || footerRow < 0 {
		t.Fatalf("slash autocomplete viewport missing slash, settings, or footer:\n%s", strings.Join(viewport, "\n"))
	}
	if settingsRow <= slashRow || footerRow <= settingsRow {
		t.Fatalf("slash autocomplete should render below slash input and above footer: slash row=%d settings row=%d footer row=%d\n%s", slashRow, settingsRow, footerRow, strings.Join(viewport, "\n"))
	}
	if settingsRow-slashRow > 8 {
		t.Fatalf("slash autocomplete should follow the editor input without a large spacer: slash row=%d settings row=%d\n%s", slashRow, settingsRow, strings.Join(viewport, "\n"))
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

type cliInteractiveTestSignal string

func (s cliInteractiveTestSignal) String() string { return string(s) }
func (s cliInteractiveTestSignal) Signal()        {}

type failingWriteTerminal struct {
	*gitui.VirtualTerminal
	mu       sync.Mutex
	writeErr error
}

func (t *failingWriteTerminal) failWrites(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.writeErr = err
}

func (t *failingWriteTerminal) Write(data string) error {
	t.mu.Lock()
	err := t.writeErr
	t.mu.Unlock()
	if err != nil {
		return err
	}
	return t.VirtualTerminal.Write(data)
}

func (t *failingWriteTerminal) ClearScreen() error {
	return t.Write("\x1b[2J\x1b[H\x1b[3J")
}

func (t *failingWriteTerminal) SetTitle(title string) error {
	return t.Write("\x1b]0;" + title + "\x07")
}

func (t *failingWriteTerminal) SetProgress(active bool) error {
	if active {
		return t.Write("\x1b]9;4;3\x07")
	}
	return t.Write("\x1b]9;4;0;\x07")
}

func TestCLIInteractiveTUIHostStopsOnShutdownSignalPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(80, 10)
	signals := make(chan os.Signal, 1)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:     runtimeHost,
		Terminal:        terminal,
		ShutdownSignals: signals,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForViewport(t, terminal, "Gi")

	signals <- cliInteractiveTestSignal("SIGTERM")
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop after shutdown signal")
	}
	if output := terminal.Output(); !strings.Contains(output, "\x1b[?25h") {
		t.Fatalf("shutdown signal should restore the terminal cursor, output=%q", output)
	}
}

func TestCLIInteractiveTUIHostSIGHUPUsesDeadTerminalShutdownPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(80, 10)
	signals := make(chan os.Signal, 1)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:     runtimeHost,
		Terminal:        terminal,
		ShutdownSignals: signals,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForViewport(t, terminal, "Gi")

	signals <- cliInteractiveTestSignal("SIGHUP")
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop after SIGHUP")
	}
	if output := terminal.Output(); strings.Contains(output, "\x1b[?25h") {
		t.Fatalf("SIGHUP dead-terminal shutdown should not attempt cursor restore, output=%q", output)
	}
}

func TestCLIInteractiveTUIHostStopsOnDeadTerminalWritePiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := &failingWriteTerminal{VirtualTerminal: gitui.NewVirtualTerminal(80, 10)}
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForViewport(t, terminal.VirtualTerminal, "Gi")

	terminal.failWrites(io.ErrClosedPipe)
	host.requestRender(true)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop after dead terminal write")
	}
	if output := terminal.Output(); strings.Contains(output, "\x1b[?25h") {
		t.Fatalf("dead-terminal shutdown should not attempt cursor restore, output=%q", output)
	}
}

func TestCLIInteractiveTUIHostStopsOnDeadTerminalTitleWritePiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := &failingWriteTerminal{VirtualTerminal: gitui.NewVirtualTerminal(80, 10)}
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForViewport(t, terminal.VirtualTerminal, "Gi")

	terminal.failWrites(io.ErrClosedPipe)
	host.updateTerminalTitle()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop after dead terminal title write")
	}
}

type drainRecordingTerminal struct {
	*gitui.VirtualTerminal
	mu                sync.Mutex
	drained           bool
	drainedBeforeStop bool
	stopped           bool
}

func (t *drainRecordingTerminal) DrainInput(max, idle time.Duration) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.drained = true
	t.drainedBeforeStop = !t.stopped
	return nil
}

func (t *drainRecordingTerminal) Stop() {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
	t.VirtualTerminal.Stop()
}

func (t *drainRecordingTerminal) drainState() (drained, beforeStop bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.drained, t.drainedBeforeStop
}

func TestCLIInteractiveTUIHostDrainsInputBeforeShutdownStopPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := &drainRecordingTerminal{VirtualTerminal: gitui.NewVirtualTerminal(80, 10)}
	signals := make(chan os.Signal, 1)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:     runtimeHost,
		Terminal:        terminal,
		ShutdownSignals: signals,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForViewport(t, terminal.VirtualTerminal, "Gi")

	signals <- cliInteractiveTestSignal("SIGTERM")
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop after shutdown signal")
	}
	drained, beforeStop := terminal.drainState()
	if !drained || !beforeStop {
		t.Fatalf("shutdown signal should drain terminal input before terminal stop, drained=%v beforeStop=%v", drained, beforeStop)
	}
}

func TestCLIInteractiveTUIHostDrainsInputBeforeNormalStopPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := &drainRecordingTerminal{VirtualTerminal: gitui.NewVirtualTerminal(80, 10)}
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:      runtimeHost,
		Terminal:         terminal,
		Messages:         []string{"/quit"},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	drained, beforeStop := terminal.drainState()
	if !drained || !beforeStop {
		t.Fatalf("normal shutdown should drain terminal input before terminal stop, drained=%v beforeStop=%v", drained, beforeStop)
	}
}

func TestCLIInteractiveTUIHostShowsLoadedResourcesOnStartupPiStyle(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cwd := t.TempDir()
	agentDir := filepath.Join(cwd, "agent")
	writeResourceFile(t, filepath.Join(cwd, "AGENTS.md"), "Project context")
	writeResourceSkill(t, filepath.Join(cwd, ConfigDirName, "skills", "review", "SKILL.md"), "review", "Review code", "Review content")
	writeResourceFile(t, filepath.Join(cwd, ConfigDirName, "prompts", "plan.md"), "Plan prompt")
	writeGiProtocolExtensionDescriptor(t, filepath.Join(cwd, ConfigDirName, "extensions", "guard.gi.json"))
	writeJSON(t, filepath.Join(cwd, ConfigDirName, "themes", "focus.json"), map[string]any{"name": "focus"})
	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		NoSession: true,
		Model:     "openai/gpt-4o-mini",
	}, CLIOptions{
		CWD:      cwd,
		AgentDir: agentDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	terminal := gitui.NewVirtualTerminal(120, 40)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForViewport(t, terminal, "[Skills]")

	viewport := strings.Join(terminal.GetViewport(), "\n")
	for _, expected := range []string{"escape interrupt", "/ commands", "[Context]", "AGENTS.md", "[Skills]", "review", "[Prompts]", "/plan", "[Extensions]", "guard.gi.json", "[Themes]", "focus"} {
		if !strings.Contains(viewport, expected) {
			t.Fatalf("viewport missing %q:\n%s", expected, viewport)
		}
	}
	if strings.Contains(viewport, "skills/review/SKILL.md") {
		t.Fatalf("startup resources should be compact before Ctrl+O:\n%s", viewport)
	}

	terminal.SendInput("\x0f")
	waitForViewport(t, terminal, "skills/review/SKILL.md")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostStartupResourcesWithoutContextUsePiSpacing(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cwd := t.TempDir()
	agentDir := filepath.Join(cwd, "agent")
	writeResourceSkill(t, filepath.Join(cwd, ConfigDirName, "skills", "review", "SKILL.md"), "review", "Review code", "Review content")
	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		NoSession: true,
		Model:     "openai/gpt-4o-mini",
	}, CLIOptions{
		CWD:      cwd,
		AgentDir: agentDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForViewport(t, terminal, "[Skills]")

	viewport := terminal.GetViewport()
	onboardingIndex := viewportLineIndex(viewport, "Gi can explain its own features")
	skillsIndex := viewportLineIndex(viewport, "[Skills]")
	if onboardingIndex < 0 || skillsIndex < 0 {
		t.Fatalf("startup resources viewport missing expected lines:\n%s", strings.Join(viewport, "\n"))
	}
	blankLines := 0
	for _, line := range viewport[onboardingIndex+1 : skillsIndex] {
		if strings.TrimSpace(StripAnsi(line)) == "" {
			blankLines++
		}
	}
	if blankLines != 1 {
		t.Fatalf("startup resources without context should keep one Pi spacer before [Skills], got %d:\n%s", blankLines, strings.Join(viewport, "\n"))
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostQuietStartupSuppressesResourceListingButKeepsDiagnosticsPiStyle(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cwd := t.TempDir()
	agentDir := filepath.Join(cwd, "agent")
	settings := NewSettingsManager(cwd, agentDir)
	settings.SetQuietStartup(true)
	writeResourceSkill(t, filepath.Join(agentDir, "skills", "review", "SKILL.md"), "review", "User review", "User content")
	writeResourceSkill(t, filepath.Join(cwd, ConfigDirName, "skills", "review", "SKILL.md"), "review", "Project review", "Project content")
	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		NoSession: true,
		Model:     "openai/gpt-4o-mini",
	}, CLIOptions{
		CWD:      cwd,
		AgentDir: agentDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	terminal := gitui.NewVirtualTerminal(100, 18)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForViewport(t, terminal, "[Skill conflicts]")

	viewport := strings.Join(terminal.GetViewport(), "\n")
	if strings.Contains(viewport, "[Skills]") {
		t.Fatalf("quiet startup should suppress resource listing:\n%s", viewport)
	}
	if !strings.Contains(viewport, "[Skill conflicts]") || !strings.Contains(viewport, "overridden") {
		t.Fatalf("quiet startup should keep diagnostics:\n%s", viewport)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostReloadRebuildsResourcesAndCommandsPiStyle(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cwd := t.TempDir()
	agentDir := filepath.Join(cwd, "agent")
	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		NoSession: true,
		Model:     "openai/gpt-4o-mini",
	}, CLIOptions{
		CWD:      cwd,
		AgentDir: agentDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal := gitui.NewVirtualTerminal(110, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	writeResourceFile(t, filepath.Join(cwd, ConfigDirName, "prompts", "ship.md"), "Ship prompt")
	writeJSON(t, filepath.Join(cwd, ConfigDirName, "extensions", "deploy.gi.json"), map[string]any{"gi": map[string]any{
		"extensionProtocol": "descriptor.v1",
		"id":                "deploy",
		"commands":          []any{map[string]any{"name": "deploy", "description": "Deploy"}},
	}})

	terminal.SendInput("/reload")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Reloaded keybindings, extensions, skills, prompts, themes")
	waitForViewport(t, terminal, "[Prompts]")
	viewport := strings.Join(terminal.GetViewport(), "\n")
	if !strings.Contains(viewport, "/ship") || !strings.Contains(viewport, "deploy.gi.json") {
		t.Fatalf("reload did not render updated resources:\n%s", viewport)
	}
	if !slashCommandNamesContain(host.autocompleteSlashCommands(), "deploy") {
		t.Fatalf("reload did not rebuild extension slash commands: %#v", host.autocompleteSlashCommands())
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShowsReloadBoxWhileReloadingPiStyle(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T", runtimeHost)
	}
	loader := newBlockingReloadResourceLoader(sessionHost.session.ResourceLoader)
	sessionHost.session.ResourceLoader = loader
	t.Cleanup(loader.Release)
	terminal := gitui.NewVirtualTerminal(110, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("/reload")
	terminal.SendInput("\r")
	select {
	case <-loader.started:
	case <-time.After(time.Second):
		t.Fatal("reload did not reach blocking resource loader")
	}
	waitForViewport(t, terminal, "Reloading keybindings, extensions, skills, prompts, themes")
	if host.ui.FocusedComponent() == host.editor {
		t.Fatal("reload box should temporarily take focus from the editor")
	}

	loader.Release()
	waitForViewport(t, terminal, "Reloaded keybindings, extensions, skills, prompts, themes")
	waitForFocusedComponent(t, terminal, host, host.editor)

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShowsModelScopeOnStartupPiStyle(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "TEST")
	cwd := t.TempDir()
	agentDir := filepath.Join(cwd, "agent")
	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		NoSession: true,
		Model:     "openai/gpt-4o-mini",
		Models:    []string{"openai/gpt-4o-mini", "openai/gpt-5-mini:off"},
	}, CLIOptions{
		CWD:      cwd,
		AgentDir: agentDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal := gitui.NewVirtualTerminal(110, 18)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForViewport(t, terminal, "Model scope:")

	viewport := strings.Join(terminal.GetViewport(), "\n")
	if !strings.Contains(viewport, "gpt-4o-mini") || !strings.Contains(viewport, "gpt-5-mini:off") {
		t.Fatalf("model scope missing expected models:\n%s", viewport)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShowsModelsJSONErrorOnStartupPiStyle(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cwd := t.TempDir()
	agentDir := filepath.Join(cwd, "agent")
	writeResourceFile(t, filepath.Join(agentDir, "models.json"), "{")
	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		NoSession: true,
		Model:     "openai/gpt-4o-mini",
	}, CLIOptions{
		CWD:      cwd,
		AgentDir: agentDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal := gitui.NewVirtualTerminal(110, 18)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForViewport(t, terminal, "models.json error:")
	waitForViewport(t, terminal, "Failed to parse models.json")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShowsRuntimeStartupWarningsPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost := runtimeHost.(*agentSessionPrintModeHost)
	sessionHost.startupWarnings = []string{
		"Migrated credentials to auth.json: anthropic, openai",
		"Could not restore model anthropic/missing-model (not found). Using openai/gpt-4o.",
	}
	terminal := gitui.NewVirtualTerminal(120, 18)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForViewport(t, terminal, "Warning: Migrated credentials to auth.json: anthropic, openai")
	waitForViewport(t, terminal, "Warning: Could not restore model anthropic/missing-model")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShowsStartupChangelogPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost := runtimeHost.(*agentSessionPrintModeHost)
	cwd := sessionHost.session.SessionManager.GetCWD()
	writeResourceFile(t, filepath.Join(cwd, "CHANGELOG.md"), "## 0.2.0\n\n- Added startup notice\n\n## 0.1.0\n\n- Old entry")
	sessionHost.settingsManager.SetLastChangelogVersion("0.1.0")

	terminal := gitui.NewVirtualTerminal(100, 18)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Version:     "0.2.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForViewport(t, terminal, "What's New")
	waitForViewport(t, terminal, "Added startup notice")
	if got := sessionHost.settingsManager.GetLastChangelogVersion(); got != "0.2.0" {
		t.Fatalf("lastChangelogVersion = %q, want 0.2.0", got)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostCollapsesStartupChangelogPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost := runtimeHost.(*agentSessionPrintModeHost)
	cwd := sessionHost.session.SessionManager.GetCWD()
	writeResourceFile(t, filepath.Join(cwd, "CHANGELOG.md"), "## 0.3.0\n\n- Added compact notice\n\n## 0.2.0\n\n- Old entry")
	sessionHost.settingsManager.SetLastChangelogVersion("0.2.0")
	sessionHost.settingsManager.SetCollapseChangelog(true)

	terminal := gitui.NewVirtualTerminal(100, 18)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Version:     "0.3.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForViewport(t, terminal, "Updated to v0.3.0. Use /changelog to view full changelog.")
	if viewport := strings.Join(terminal.GetViewport(), "\n"); strings.Contains(viewport, "Added compact notice") {
		t.Fatalf("collapsed changelog should not show full entry:\n%s", viewport)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostSkipsFreshInstallStartupChangelogPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost := runtimeHost.(*agentSessionPrintModeHost)
	cwd := sessionHost.session.SessionManager.GetCWD()
	writeResourceFile(t, filepath.Join(cwd, "CHANGELOG.md"), "## 0.4.0\n\n- Fresh install entry")

	terminal := gitui.NewVirtualTerminal(100, 18)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Version:     "0.4.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	terminal.WaitForRender()
	viewport := strings.Join(terminal.GetViewport(), "\n")
	if strings.Contains(viewport, "Fresh install entry") || strings.Contains(viewport, "What's New") {
		t.Fatalf("fresh install should record changelog version without showing entries:\n%s", viewport)
	}
	if got := sessionHost.settingsManager.GetLastChangelogVersion(); got != "0.4.0" {
		t.Fatalf("lastChangelogVersion = %q, want 0.4.0", got)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShowsNewVersionNotificationPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	called := make(chan string, 1)
	terminal := gitui.NewVirtualTerminal(110, 18)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Version:     "0.4.0",
		VersionCheck: func(currentVersion string, _ VersionCheckOptions) (LatestGiRelease, bool) {
			called <- currentVersion
			return LatestGiRelease{Version: "0.5.0"}, true
		},
		InstallEnvironment: InstallEnvironment{Platform: "darwin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForViewport(t, terminal, "Update Available")
	waitForViewport(t, terminal, "New version 0.5.0 is available.")
	waitForViewport(t, terminal, "Changelog: https://github.com/nowa/gi/releases/latest")
	select {
	case got := <-called:
		if got != "0.4.0" {
			t.Fatalf("version check current version = %q", got)
		}
	default:
		t.Fatal("version checker was not called")
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShowsPackageUpdateNotificationPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(110, 18)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		PackageUpdateCheck: func() ([]string, error) {
			return []string{"github.com/test/extension"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForViewport(t, terminal, "Package Updates Available")
	waitForViewport(t, terminal, "Package updates are available. Run")
	waitForViewport(t, terminal, "github.com/test/extension")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShowsAnthropicSubscriptionWarningOnStartupPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost := runtimeHost.(*agentSessionPrintModeHost)
	sessionHost.session.Agent.State.Model = llm.Model{Provider: "anthropic", ID: "claude-sonnet-4-5", API: "anthropic"}
	sessionHost.modelRegistry.authStorage.Set("anthropic", AuthCredential{Type: "oauth", Access: "access-token"})

	terminal := gitui.NewVirtualTerminal(120, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForViewport(t, terminal, "Anthropic subscription auth is active.")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostUpdatesTerminalTitlePiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost := runtimeHost.(*agentSessionPrintModeHost)
	cwdBase := filepath.Base(sessionHost.session.SessionManager.GetCWD())
	terminal := gitui.NewVirtualTerminal(100, 14)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	if got, want := terminal.WindowTitle(), "Gi - "+cwdBase; got != want {
		t.Fatalf("startup title = %q, want %q", got, want)
	}

	if err := host.handleNameSlashCommand("Research"); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.WindowTitle(), "Gi - Research - "+cwdBase; got != want {
		t.Fatalf("named title = %q, want %q", got, want)
	}

	rpcHost := &RPCSessionHost{TUITitle: host}
	response := rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "title_1",
		Method:   "host.tui.title",
		Params:   []byte(`{"title":"Package title"}`),
	})
	result := assertHostActionResponseOK(t, response, "title_1")
	if result["title"] != "Package title" {
		t.Fatalf("title result = %#v", result)
	}
	if got := terminal.WindowTitle(); got != "Package title" {
		t.Fatalf("package title = %q", got)
	}
	if err := host.SetTUITitle(""); err != nil {
		t.Fatal(err)
	}
	if got, want := terminal.WindowTitle(), "Gi - Research - "+cwdBase; got != want {
		t.Fatalf("reset package title = %q, want %q", got, want)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostCanRenderDefaultFooter(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 12)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		ShowFooter:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForViewport(t, terminal, "gpt-4o-mini")
	waitForViewport(t, terminal, "(auto)")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShowsTmuxKeyboardWarningPiStyle(t *testing.T) {
	t.Setenv("TMUX", "tmux-session")
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		TmuxOptionReader: func(_ context.Context, option string) (string, bool) {
			if option == "extended-keys" {
				return "off", true
			}
			return "", false
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForViewport(t, terminal, "Warning: tmux extended-keys is off")
	viewport := terminal.GetViewport()
	foundWarningEnd := false
	for index, line := range viewport {
		if strings.Contains(line, "and restart tmux.") {
			foundWarningEnd = true
			if index+1 >= len(viewport) || strings.TrimSpace(viewport[index+1]) != "" {
				t.Fatalf("tmux warning should keep Pi spacer before editor border:\n%s", strings.Join(viewport, "\n"))
			}
			break
		}
	}
	if !foundWarningEnd {
		t.Fatalf("wrapped tmux warning end not found:\n%s", strings.Join(viewport, "\n"))
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostSubmitsEditorInputAndStopsOnCtrlD(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForHostEditor(t, host)
	host.editor.SetText("typed prompt")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Response to: typed prompt")

	terminal.SendInput("\x04")
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop on ctrl+d")
	}
}

func TestCLIInteractiveTUIHostUsesPiStyleCtrlCToClearThenExit(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("draft")
	terminal.SendInput("\x03")
	waitForEditorText(t, host, "")
	select {
	case err := <-errCh:
		t.Fatalf("host stopped after first ctrl+c: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	terminal.SendInput("\x03")
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop on second ctrl+c")
	}
}

func TestCLIInteractiveTUIHostCtrlZSuspendsAndRestoresPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	killSignals := make(chan string, 1)
	resumeHandlers := make(chan func(), 1)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Suspend: InteractiveSuspendOperations{
			Platform: "darwin",
			SetInterval: func(func(), time.Duration) any {
				return "interval"
			},
			ClearInterval: func(any) {},
			OnSignal: func(signal string, handler func()) any {
				return signal
			},
			OnceSignal: func(signal string, handler func()) any {
				if signal == "SIGCONT" {
					resumeHandlers <- handler
				}
				return signal
			},
			RemoveSignalListener: func(string, any) {},
			KillProcessGroup: func(signal string) error {
				killSignals <- signal
				return nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("\x1a")
	select {
	case signal := <-killSignals:
		if signal != "SIGTSTP" {
			t.Fatalf("signal = %q, want SIGTSTP", signal)
		}
	case <-time.After(time.Second):
		t.Fatal("ctrl+z did not suspend the process group")
	}
	select {
	case resume := <-resumeHandlers:
		resume()
	case <-time.After(time.Second):
		t.Fatal("ctrl+z did not register SIGCONT restore handler")
	}
	waitForViewport(t, terminal, "gi v0.0.0")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostQueuesStreamingEditorMessagesPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	prompts := make(chan string, 8)
	releaseFirst := make(chan struct{})
	sessionHost.session.Responder = func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		prompts <- prompt
		if prompt == "first" {
			<-releaseFirst
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done " + prompt)}}, nil
	}

	terminal := gitui.NewVirtualTerminal(120, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("first")
	terminal.SendInput("\r")
	waitForPrompt(t, prompts, "first")
	waitForViewport(t, terminal, "Thinking...")

	rpcHost := &RPCSessionHost{TUIWorking: host}
	response := rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "working_1",
		Method:   "host.tui.working",
		Params:   []byte(`{"message":"Package busy","indicator":{"frames":["."],"intervalMs":25}}`),
	})
	assertHostActionResponseOK(t, response, "working_1")
	waitForViewport(t, terminal, "Package busy")

	host.editor.SetText("steer now")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Steering: steer now")
	if got := sessionHost.session.GetSteeringMessages(); len(got) != 1 || got[0] != "steer now" {
		t.Fatalf("steering messages = %#v", got)
	}

	host.editor.SetText("follow later")
	terminal.SendInput("\x1b\r")
	waitForViewport(t, terminal, "Follow-up: follow later")
	if got := sessionHost.session.GetFollowUpMessages(); len(got) != 1 || got[0] != "follow later" {
		t.Fatalf("follow-up messages = %#v", got)
	}

	close(releaseFirst)
	waitForPrompt(t, prompts, "steer now")
	waitForPrompt(t, prompts, "follow later")
	waitForViewport(t, terminal, "done follow later")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRendersBashInPendingAreaWhileStreamingPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	prompts := make(chan string, 2)
	releaseFirst := make(chan struct{})
	sessionHost.session.Responder = func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		prompts <- prompt
		if prompt == "first" {
			<-releaseFirst
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done " + prompt)}}, nil
	}
	commands := make(chan string, 1)
	operations := BashOperations{
		Exec: func(command, _ string, options BashExecOptions) (BashOperationResult, error) {
			commands <- command
			if options.OnData != nil {
				options.OnData([]byte("pending-bash\n"))
			}
			return BashOperationResult{ExitCode: 0}, nil
		},
	}

	terminal := gitui.NewVirtualTerminal(120, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:    runtimeHost,
		Terminal:       terminal,
		BashOperations: operations,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("first")
	terminal.SendInput("\r")
	waitForPrompt(t, prompts, "first")
	waitForViewport(t, terminal, "Thinking...")

	host.editor.SetText("!printf pending-bash")
	terminal.SendInput("\r")
	waitForBashCommand(t, commands, "printf pending-bash")
	waitForViewport(t, terminal, "$ printf pending-bash")
	waitForViewport(t, terminal, "pending-bash")
	if host.pendingMessages.ChildCount() == 0 {
		t.Fatalf("bash component should render in pending area while streaming")
	}
	for _, child := range host.chat.Children() {
		if _, ok := child.(*BashExecutionComponent); ok {
			t.Fatalf("bash component should not render in chat while streaming")
		}
	}

	close(releaseFirst)
	waitForViewport(t, terminal, "done first")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostBlocksSecondBashAndEscapeCancelsPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	started := make(chan struct{})
	commands := make(chan string, 1)
	operations := BashOperations{
		Exec: func(command, _ string, options BashExecOptions) (BashOperationResult, error) {
			commands <- command
			close(started)
			<-options.Context.Done()
			return BashOperationResult{}, errors.New("aborted")
		},
	}

	terminal := gitui.NewVirtualTerminal(120, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:    runtimeHost,
		Terminal:       terminal,
		BashOperations: operations,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("!sleep forever")
	terminal.SendInput("\r")
	waitForBashCommand(t, commands, "sleep forever")
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("bash command did not start")
	}
	waitUntil(t, sessionHost.session.IsBashRunning)

	host.editor.SetText("!echo second")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "A bash command is already running. Press Esc to cancel it first.")
	waitForEditorText(t, host, "!echo second")

	terminal.SendInput("\x1b")
	waitUntil(t, func() bool { return !sessionHost.session.IsBashRunning() })
	waitForViewport(t, terminal, "Bash command cancelled")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostEscapeClearsBashDraftPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 18)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("!echo draft")
	terminal.SendInput("\x1b")
	waitForEditorText(t, host, "")

	host.editor.SetText("ordinary draft")
	terminal.SendInput("\x1b")
	time.Sleep(25 * time.Millisecond)
	if got := host.editor.GetText(); got != "ordinary draft" {
		t.Fatalf("non-bash draft after escape = %q", got)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostUsesUserBashExtensionResultPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	var got ProtocolSessionEvent
	fullOutputPath := filepath.Join(t.TempDir(), "full-bash-output.log")
	mustLoadProtocolFactories(t, sessionHost.session.ExtensionRuntime, ProtocolExtensionFactory{Path: "bash.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.On(ProtocolEventUserBash, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			got = event
			return ProtocolEventResult{
				BashResult:    &BashResult{Output: "from extension bash\n", ExitCode: 0, Truncated: true, FullOutputPath: fullOutputPath},
				BashResultSet: true,
			}, nil
		})
	}})
	commands := make(chan string, 1)
	operations := BashOperations{
		Exec: func(command, _ string, _ BashExecOptions) (BashOperationResult, error) {
			commands <- command
			return BashOperationResult{}, errors.New("host bash should not run")
		},
	}

	terminal := gitui.NewVirtualTerminal(120, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:    runtimeHost,
		Terminal:       terminal,
		BashOperations: operations,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("!!printf intercepted")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "$ printf intercepted")
	waitForViewport(t, terminal, "from extension bash")
	waitForViewport(t, terminal, "Output truncated. Full output:")
	waitForViewport(t, terminal, filepath.Base(fullOutputPath))
	if got.Command != "printf intercepted" || got.CWD == "" || !got.ExcludeFromContext {
		t.Fatalf("user_bash event = %#v", got)
	}
	if sessionHost.session.IsBashRunning() {
		t.Fatal("extension-handled bash should not leave host bash running")
	}
	select {
	case command := <-commands:
		t.Fatalf("host bash executed %q despite extension result", command)
	default:
	}
	if !sessionHasRole(sessionHost.session, "bashExecution") {
		t.Fatalf("expected extension bash result to be recorded")
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostQueuesInputDuringCompactionPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	mustPrompt(t, sessionHost.session, "one")
	mustPrompt(t, sessionHost.session, "two")

	compactionStarted := make(chan struct{})
	releaseCompaction := make(chan struct{})
	sessionHost.session.CompactionSummarizer = func(preparation agentharness.CompactionPreparation, _ string) (agentharness.CompactionResult, error) {
		close(compactionStarted)
		<-releaseCompaction
		return agentharness.CompactionResult{
			Summary:          "compacted context",
			FirstKeptEntryID: preparation.FirstKeptEntryID,
			TokensBefore:     preparation.TokensBefore,
		}, nil
	}
	prompts := make(chan string, 4)
	sessionHost.session.Responder = func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		prompts <- prompt
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done " + prompt)}}, nil
	}

	terminal := gitui.NewVirtualTerminal(120, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/compact focus")
	terminal.SendInput("\r")
	select {
	case <-compactionStarted:
	case <-time.After(time.Second):
		t.Fatal("compaction did not start")
	}
	if !sessionHost.session.IsCompacting() {
		t.Fatal("session should report compaction while manual compaction is running")
	}
	waitForViewport(t, terminal, "Compacting context")

	host.editor.SetText("after compact")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Steering: after compact")
	if got := len(host.compactionQueue); got != 1 {
		t.Fatalf("compaction queue length = %d, want 1", got)
	}

	close(releaseCompaction)
	waitForPrompt(t, prompts, "after compact")
	waitForViewport(t, terminal, "done after compact")
	if got := len(host.compactionQueue); got != 0 {
		t.Fatalf("compaction queue length after flush = %d, want 0", got)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostCancelsCompactionPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	mustPrompt(t, sessionHost.session, "one")
	mustPrompt(t, sessionHost.session, "two")

	compactionStarted := make(chan struct{})
	releaseCompaction := make(chan struct{})
	sessionHost.session.CompactionSummarizer = func(preparation agentharness.CompactionPreparation, _ string) (agentharness.CompactionResult, error) {
		close(compactionStarted)
		<-releaseCompaction
		return agentharness.CompactionResult{
			Summary:          "discarded summary",
			FirstKeptEntryID: preparation.FirstKeptEntryID,
			TokensBefore:     preparation.TokensBefore,
		}, nil
	}

	terminal := gitui.NewVirtualTerminal(120, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/compact")
	terminal.SendInput("\r")
	select {
	case <-compactionStarted:
	case <-time.After(time.Second):
		t.Fatal("compaction did not start")
	}
	waitForViewport(t, terminal, "Compacting context")
	terminal.SendInput("\x1b")
	close(releaseCompaction)
	waitUntil(t, func() bool { return !sessionHost.session.IsCompacting() })
	waitForViewport(t, terminal, "Compaction cancelled")
	output := strings.Join(terminal.GetViewport(), "\n")
	if strings.Contains(output, "Error: Compaction") {
		t.Fatalf("cancelled compaction rendered as submit error:\n%s", output)
	}
	if compactions := filterFileEntriesByType(sessionHost.session.SessionManager.GetEntries(), "compaction"); len(compactions) != 0 {
		t.Fatalf("compaction entries = %d, want 0 after cancellation", len(compactions))
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostDequeuesCompactionMessagesToEditorPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	mustPrompt(t, sessionHost.session, "one")
	mustPrompt(t, sessionHost.session, "two")

	compactionStarted := make(chan struct{})
	releaseCompaction := make(chan struct{})
	sessionHost.session.CompactionSummarizer = func(preparation agentharness.CompactionPreparation, _ string) (agentharness.CompactionResult, error) {
		close(compactionStarted)
		<-releaseCompaction
		return agentharness.CompactionResult{
			Summary:          "compacted context",
			FirstKeptEntryID: preparation.FirstKeptEntryID,
			TokensBefore:     preparation.TokensBefore,
		}, nil
	}

	terminal := gitui.NewVirtualTerminal(120, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/compact")
	terminal.SendInput("\r")
	select {
	case <-compactionStarted:
	case <-time.After(time.Second):
		t.Fatal("compaction did not start")
	}

	host.editor.SetText("edit me after compact")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Steering: edit me after compact")
	terminal.SendInput("\x1b[1;3A")
	waitForEditorText(t, host, "edit me after compact")
	if got := len(host.compactionQueue); got != 0 {
		t.Fatalf("compaction queue length after dequeue = %d, want 0", got)
	}
	waitForViewport(t, terminal, "Restored 1 queued message to editor")

	close(releaseCompaction)
	waitForViewport(t, terminal, "Compacted:")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostFlushesCompactionFollowUpQueuePiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	mustPrompt(t, sessionHost.session, "one")
	mustPrompt(t, sessionHost.session, "two")

	compactionStarted := make(chan struct{})
	releaseCompaction := make(chan struct{})
	sessionHost.session.CompactionSummarizer = func(preparation agentharness.CompactionPreparation, _ string) (agentharness.CompactionResult, error) {
		close(compactionStarted)
		<-releaseCompaction
		return agentharness.CompactionResult{
			Summary:          "compacted context",
			FirstKeptEntryID: preparation.FirstKeptEntryID,
			TokensBefore:     preparation.TokensBefore,
		}, nil
	}
	prompts := make(chan string, 4)
	releaseFirstPrompt := make(chan struct{})
	sessionHost.session.Responder = func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		prompts <- prompt
		if prompt == "first after compact" {
			<-releaseFirstPrompt
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done " + prompt)}}, nil
	}

	terminal := gitui.NewVirtualTerminal(120, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/compact")
	terminal.SendInput("\r")
	select {
	case <-compactionStarted:
	case <-time.After(time.Second):
		t.Fatal("compaction did not start")
	}

	host.editor.SetText("first after compact")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Steering: first after compact")
	host.editor.SetText("follow after compact")
	terminal.SendInput("\x1b\r")
	waitForViewport(t, terminal, "Follow-up: follow after compact")

	close(releaseCompaction)
	waitForPrompt(t, prompts, "first after compact")
	waitForViewport(t, terminal, "Follow-up: follow after compact")
	close(releaseFirstPrompt)
	waitForPrompt(t, prompts, "follow after compact")
	waitForViewport(t, terminal, "done follow after compact")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostDequeuesStreamingMessagesToEditorPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	prompts := make(chan string, 4)
	releaseFirst := make(chan struct{})
	sessionHost.session.Responder = func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		prompts <- prompt
		if prompt == "first" {
			<-releaseFirst
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done " + prompt)}}, nil
	}

	terminal := gitui.NewVirtualTerminal(120, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("first")
	terminal.SendInput("\r")
	waitForPrompt(t, prompts, "first")

	host.editor.SetText("steer now")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Steering: steer now")
	host.editor.SetText("follow later")
	terminal.SendInput("\x1b\r")
	waitForViewport(t, terminal, "Follow-up: follow later")

	terminal.SendInput("\x1b[1;3A")
	waitForEditorText(t, host, "steer now\n\nfollow later")
	if sessionHost.session.PendingMessageCount() != 0 {
		t.Fatalf("pending messages after dequeue = %d", sessionHost.session.PendingMessageCount())
	}
	waitForViewport(t, terminal, "Restored 2 queued messages to editor")

	close(releaseFirst)
	waitForViewport(t, terminal, "done first")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostEscapeAbortsStreamingAndRestoresQueuePiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	prompts := make(chan string, 4)
	releaseFirst := make(chan struct{})
	sessionHost.session.Responder = func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
		prompts <- prompt
		if prompt == "first" {
			<-releaseFirst
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("done " + prompt)}}, nil
	}

	terminal := gitui.NewVirtualTerminal(120, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("first")
	terminal.SendInput("\r")
	waitForPrompt(t, prompts, "first")

	host.editor.SetText("steer now")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Steering: steer now")
	host.editor.SetText("follow later")
	terminal.SendInput("\x1b\r")
	waitForViewport(t, terminal, "Follow-up: follow later")

	terminal.SendInput("\x1b")
	waitForEditorText(t, host, "steer now\n\nfollow later")
	if !sessionHost.session.abortRequested {
		t.Fatal("escape should abort active session")
	}
	if sessionHost.session.PendingMessageCount() != 0 {
		t.Fatalf("pending messages after escape = %d", sessionHost.session.PendingMessageCount())
	}

	close(releaseFirst)
	waitForViewport(t, terminal, "done first")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShowsAndCancelsAutoRetryPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	terminal := gitui.NewVirtualTerminal(120, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	cancelled := make(chan struct{}, 1)
	sessionHost.session.isRetrying = true
	sessionHost.session.retryAbort = func() {
		cancelled <- struct{}{}
		sessionHost.session.isRetrying = false
	}
	sessionHost.session.emit(AgentSessionEvent{Type: "auto_retry_start", Attempt: 1, MaxAttempts: 3, DelayMs: 2100})
	waitForViewport(t, terminal, "Retrying (1/3) in 3s... (Esc to cancel)")

	terminal.SendInput("\x1b")
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("escape did not abort retry")
	}
	waitForViewport(t, terminal, "Retry cancelled")

	sessionHost.session.emit(AgentSessionEvent{Type: "auto_retry_end", Attempt: 2, FinalError: "Network connection lost."})
	waitForViewport(t, terminal, "Retry failed after 2 attempts: Network connection lost.")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostAutoRetrySuccessClearsStatusPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	terminal := gitui.NewVirtualTerminal(120, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	sessionHost.session.emit(AgentSessionEvent{Type: "auto_retry_start", Attempt: 1, MaxAttempts: 3, DelayMs: 2100})
	waitForViewport(t, terminal, "Retrying (1/3) in 3s... (Esc to cancel)")
	sessionHost.session.emit(AgentSessionEvent{Type: "auto_retry_end", Success: true, Attempt: 1})
	waitUntil(t, func() bool {
		return !strings.Contains(strings.Join(terminal.GetViewport(), "\n"), "Retrying (1/3)")
	})

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostDoubleEscapeOpensTreePiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 48)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("tree seed")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Response to: tree seed")
	waitForCondition(t, func() bool { return host.streamingComponent == nil }, "tree seed response to finish")
	terminal.SendInput("\x1b")
	terminal.SendInput("\x1b")
	waitForViewport(t, terminal, "Session Tree")
	waitForViewport(t, terminal, "fold/branch")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostDoubleEscapeOpensForkWhenConfiguredPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.settingsManager.SetDoubleEscapeAction("fork")
	terminal := gitui.NewVirtualTerminal(120, 40)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("fork seed")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Response to: fork seed")
	waitForCondition(t, func() bool { return host.streamingComponent == nil }, "fork seed response to finish")
	terminal.SendInput("\x1b")
	terminal.SendInput("\x1b")
	waitForViewport(t, terminal, "Fork from Message")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostUsesSessionActionKeybindingsPiStyle(t *testing.T) {
	t.Run("new session", func(t *testing.T) {
		runtimeHost := newOfflineInteractiveRuntimeHost(t)
		terminal := gitui.NewVirtualTerminal(120, 28)
		host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
			RuntimeHost: runtimeHost,
			Terminal:    terminal,
		})
		if err != nil {
			t.Fatal(err)
		}
		errCh := make(chan error, 1)
		go func() {
			errCh <- host.RunContext(context.Background())
		}()
		t.Cleanup(func() { host.Stop() })
		waitForHostEditor(t, host)
		host.keybindings = mergeKeybindingsConfig(DefaultProtocolKeybindings(), KeybindingsConfig{
			"app.session.new": "ctrl+n",
		})

		terminal.SendInput("\x0e")
		waitForViewport(t, terminal, "New session started")

		host.Stop()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("RunContext returned %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("interactive TUI host did not stop")
		}
	})

	t.Run("tree", func(t *testing.T) {
		runtimeHost := newOfflineInteractiveRuntimeHost(t)
		terminal := gitui.NewVirtualTerminal(120, 32)
		host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
			RuntimeHost: runtimeHost,
			Terminal:    terminal,
		})
		if err != nil {
			t.Fatal(err)
		}
		errCh := make(chan error, 1)
		go func() {
			errCh <- host.RunContext(context.Background())
		}()
		t.Cleanup(func() { host.Stop() })
		waitForHostEditor(t, host)
		host.keybindings = mergeKeybindingsConfig(DefaultProtocolKeybindings(), KeybindingsConfig{
			"app.session.tree": "ctrl+y",
		})

		host.editor.SetText("tree keybinding seed")
		terminal.SendInput("\r")
		waitForViewport(t, terminal, "Response to: tree keybinding seed")
		terminal.SendInput("\x19")
		waitForViewport(t, terminal, "Session Tree")

		host.Stop()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("RunContext returned %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("interactive TUI host did not stop")
		}
	})

	t.Run("fork", func(t *testing.T) {
		runtimeHost := newOfflineInteractiveRuntimeHost(t)
		terminal := gitui.NewVirtualTerminal(120, 32)
		host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
			RuntimeHost: runtimeHost,
			Terminal:    terminal,
		})
		if err != nil {
			t.Fatal(err)
		}
		errCh := make(chan error, 1)
		go func() {
			errCh <- host.RunContext(context.Background())
		}()
		t.Cleanup(func() { host.Stop() })
		waitForHostEditor(t, host)
		host.keybindings = mergeKeybindingsConfig(DefaultProtocolKeybindings(), KeybindingsConfig{
			"app.session.fork": "ctrl+f",
		})

		host.editor.SetText("fork keybinding seed")
		terminal.SendInput("\r")
		waitForViewport(t, terminal, "Response to: fork keybinding seed")
		terminal.SendInput("\x06")
		waitForViewport(t, terminal, "Fork from Message")

		host.Stop()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("RunContext returned %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("interactive TUI host did not stop")
		}
	})

	t.Run("resume", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "")
		cwd := t.TempDir()
		agentDir := filepath.Join(t.TempDir(), "agent")
		sessionDir := filepath.Join(t.TempDir(), "sessions")
		targetManager, err := CreateSessionManager(cwd, sessionDir)
		if err != nil {
			t.Fatal(err)
		}
		targetManager.AppendMessage(llm.Message{Role: llm.RoleUser, Content: []llm.ContentPart{llm.Text("Target question")}})
		targetManager.AppendMessage(llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("Target answer")}})
		if err := targetManager.rewriteFile(); err != nil {
			t.Fatal(err)
		}

		runtimeHost, err := newDefaultCLIPrintModeHost(Args{
			Offline:    true,
			Model:      "openai/gpt-4o-mini",
			SessionDir: sessionDir,
		}, CLIOptions{CWD: cwd, AgentDir: agentDir})
		if err != nil {
			t.Fatal(err)
		}
		terminal := gitui.NewVirtualTerminal(120, 28)
		host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
			RuntimeHost: runtimeHost,
			Terminal:    terminal,
		})
		if err != nil {
			t.Fatal(err)
		}
		errCh := make(chan error, 1)
		go func() {
			errCh <- host.RunContext(context.Background())
		}()
		t.Cleanup(func() { host.Stop() })
		waitForHostEditor(t, host)
		host.keybindings = mergeKeybindingsConfig(DefaultProtocolKeybindings(), KeybindingsConfig{
			"app.session.resume": "ctrl+r",
		})

		terminal.SendInput("\x12")
		waitForViewport(t, terminal, "Resume Session")

		host.Stop()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("RunContext returned %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("interactive TUI host did not stop")
		}
	})
}

func TestCLIInteractiveTUIHostHandlesModelThinkingHotkeysPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 48)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("\x1b[Z")
	waitForViewport(t, terminal, "Thinking:")
	terminal.SendInput("\x10")
	waitForViewport(t, terminal, "Model:")
	terminal.SendInput("\x0c")
	waitForViewport(t, terminal, "Only showing models from configured providers")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostCtrlTTogglesThinkingBlocksPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.session.SessionManager.AppendMessage(sessionMessageValue(llm.Message{
		Role: llm.RoleAssistant,
		Content: []llm.ContentPart{
			llm.Thinking("secret plan"),
			llm.Text("final answer"),
		},
	}))
	terminal := gitui.NewVirtualTerminal(120, 40)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForViewport(t, terminal, "secret plan")

	terminal.SendInput("\x14")
	waitForViewport(t, terminal, "Thinking blocks: hidden")
	viewport := strings.Join(terminal.GetViewport(), "\n")
	if strings.Contains(viewport, "secret plan") {
		t.Fatalf("hidden thinking should not render raw thinking:\n%s", viewport)
	}
	if !strings.Contains(viewport, "Thinking...") || !strings.Contains(viewport, "final answer") {
		t.Fatalf("hidden thinking viewport missing label/text:\n%s", viewport)
	}
	if !sessionHost.settingsManager.GetHideThinkingBlock() {
		t.Fatal("hideThinkingBlock setting was not saved")
	}

	rpcHost := &RPCSessionHost{TUIThinkingLabel: host}
	response := rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "thinking_label_1",
		Method:   "host.tui.thinking_label",
		Params:   []byte(`{"label":"Reasoning hidden"}`),
	})
	assertHostActionResponseOK(t, response, "thinking_label_1")
	waitForViewport(t, terminal, "Reasoning hidden")
	viewport = strings.Join(terminal.GetViewport(), "\n")
	if strings.Contains(viewport, "secret plan") {
		t.Fatalf("custom hidden thinking label should not reveal thinking:\n%s", viewport)
	}

	terminal.SendInput("\x14")
	waitForViewport(t, terminal, "Thinking blocks: visible")
	waitForViewport(t, terminal, "secret plan")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostCtrlOTogglesToolOutputExpansionPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.session.SessionManager.AppendMessage(sessionMessageValue(llm.Message{
		Role:       llm.RoleAssistant,
		StopReason: "toolUse",
		Content: []llm.ContentPart{
			llm.ToolCall("read-call", "read", map[string]any{"path": "notes.txt"}),
		},
	}))
	sessionHost.session.SessionManager.AppendMessage(sessionMessageValue(llm.Message{
		Role:       llm.RoleToolResult,
		ToolCallID: "read-call",
		Content: []llm.ContentPart{
			llm.Text(strings.Join([]string{
				"line 01", "line 02", "line 03", "line 04", "line 05", "line 06",
				"line 07", "line 08", "line 09", "line 10", "line 11", "line 12",
			}, "\n")),
		},
	}))

	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForToolExpanded(t, host, false)

	terminal.SendInput("\x0f")
	waitForToolExpanded(t, host, true)
	terminal.SendInput("\x0f")
	waitForToolExpanded(t, host, false)

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRendersSummaryMessagesAsCollapsibleComponentsPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 48)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.addMessage(llm.Message{
		Role:    "branchSummary",
		Content: []llm.ContentPart{llm.Text("Branch **details**")},
	})
	host.addMessage(llm.Message{
		Role:    "compactionSummary",
		Content: []llm.ContentPart{llm.Text("Compaction **details**")},
		Details: map[string]any{"tokensBefore": float64(1234)},
	})
	host.requestRender(false)
	waitForViewport(t, terminal, "[branch] Branch summary")
	waitForViewport(t, terminal, "[compaction] Compacted from 1,234 tokens")
	if viewport := strings.Join(terminal.GetViewport(), "\n"); strings.Contains(viewport, "Branch details") || strings.Contains(viewport, "Compaction details") {
		t.Fatalf("collapsed summaries leaked details:\n%s", viewport)
	}

	terminal.SendInput("\x0f")
	waitForViewport(t, terminal, "Branch Summary")
	waitForViewport(t, terminal, "Compacted from 1,234 tokens")
	waitForViewport(t, terminal, "Branch details")
	waitForViewport(t, terminal, "Compaction details")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShowsStartupCompactionCountPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.session.SessionManager.AppendCompaction("first summary", "kept-1", 1000)
	sessionHost.session.SessionManager.AppendCompaction("second summary", "kept-2", 2000)

	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForViewport(t, terminal, "Session compacted 2 times")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRendersSkillInvocationAsCollapsibleComponentPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 40)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.addMessage(llm.UserMessageText("<skill name=\"review\" location=\"/skills/review/SKILL.md\">\nUse **care**.\n</skill>\n\nCheck this diff"))
	host.requestRender(false)
	waitForViewport(t, terminal, "[skill] review")
	waitForViewport(t, terminal, "Check this diff")
	if viewport := strings.Join(terminal.GetViewport(), "\n"); strings.Contains(viewport, "<skill") || strings.Contains(viewport, "Use care") {
		t.Fatalf("collapsed skill invocation leaked wrapper/details:\n%s", viewport)
	}

	terminal.SendInput("\x0f")
	waitForViewport(t, terminal, "Use care")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostCtrlGUsesExternalEditorPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	editorPath := filepath.Join(t.TempDir(), "editor.sh")
	if err := os.WriteFile(editorPath, []byte("#!/bin/sh\nprintf 'Edited externally\\n' > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", editorPath)
	host.editor.SetText("Draft")

	terminal.SendInput("\x07")
	waitForEditorText(t, host, "Edited externally")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostCtrlGWarnsWhenExternalEditorMissingPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	terminal.SendInput("\x07")
	waitForViewport(t, terminal, "No editor configured")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostCtrlVPastesClipboardImagePathPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 24)
	clipboardBytes := []byte("clipboard image bytes")
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		ClipboardImageRead: func() *ClipboardImage {
			return &ClipboardImage{Bytes: clipboardBytes, MIMEType: "image/png"}
		},
		ClipboardImageDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("\x16")
	path := waitForEditorTextSuffix(t, host, ".png")
	if !strings.Contains(filepath.Base(path), "gi-clipboard-") {
		t.Fatalf("clipboard path = %q, want gi-clipboard temp file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(clipboardBytes) {
		t.Fatalf("clipboard file = %q, want %q", string(data), string(clipboardBytes))
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostHandlesBuiltinSlashCommands(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	mustPrompt(t, sessionHost.session, "seed export session")
	var prompts int
	sessionHost.session.Responder = func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error) {
		prompts++
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("unexpected")}}, nil
	}
	exportPath := filepath.Join(t.TempDir(), "session.html")
	terminal := gitui.NewVirtualTerminal(120, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Messages: []string{
			"/model openai/gpt-4o-mini",
			"/session",
			"/export " + exportPath,
		},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{"Model: openai/gpt-4o-mini", "Session exported to:"} {
		waitForTerminalOutput(t, terminal, expected)
	}
	waitForTerminalOutput(t, terminal, "Session Info")
	if prompts != 0 {
		t.Fatalf("slash commands reached responder %d times", prompts)
	}
	if _, err := os.Stat(exportPath); err != nil {
		t.Fatalf("export path missing: %v", err)
	}
}

func TestCLIInteractiveTUIHostSubmitsSlashCommandOnLinefeed(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	var prompts int
	sessionHost.session.Responder = func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error) {
		prompts++
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("unexpected")}}, nil
	}
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	for _, r := range "/session" {
		terminal.SendInput(string(r))
	}
	terminal.SendInput("\n")
	waitForTerminalOutput(t, terminal, "Session Info")
	if prompts != 0 {
		t.Fatalf("linefeed slash command reached responder %d times", prompts)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRequiresExactNoArgSlashCommandsPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	var prompts []string
	sessionHost.session.Responder = func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error) {
		prompts = append(prompts, prompt)
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("agent saw: " + prompt)}}, nil
	}
	terminal := gitui.NewVirtualTerminal(120, 36)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Messages: []string{
			"/session extra",
			"/theme light",
			"/resources",
			"/thinking off",
			"/models",
			"/queue all",
			"/arminsayshi extra",
			"/dementedelves extra",
			"/quit later",
		},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(prompts, []string{"/session extra", "/theme light", "/resources", "/thinking off", "/models", "/queue all", "/arminsayshi extra", "/dementedelves extra", "/quit later"}) {
		t.Fatalf("prompts = %#v", prompts)
	}
	waitForViewport(t, terminal, "agent saw: /quit later")
	if strings.Contains(terminal.Output(), "Session Info") {
		t.Fatalf("argument-bearing /session should not run builtin command:\n%s", terminal.Output())
	}
	if sessionHost.settingsManager.GetTheme() == "light" {
		t.Fatalf("/theme should be submitted as a prompt like Pi, not handled as a Gi builtin")
	}
}

func TestCLIInteractiveTUIHostHandlesHiddenPiSlashCommands(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	var prompts int
	sessionHost.session.Responder = func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error) {
		prompts++
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("unexpected")}}, nil
	}
	terminal := gitui.NewVirtualTerminal(120, 40)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Messages: []string{
			"/arminsayshi",
			"/dementedelves",
		},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	waitForViewport(t, terminal, "ARMIN SAYS HI")
	waitForViewport(t, terminal, "gi has joined Earendil")
	waitForViewport(t, terminal, cliEarendilBlogURL)
	if prompts != 0 {
		t.Fatalf("hidden slash commands reached responder %d times", prompts)
	}
}

func TestCLIInteractiveTUIHostShowsBuiltinSlashAutocomplete(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:        runtimeHost,
		Terminal:           terminal,
		ClearScreenOnStart: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	changed := make(chan struct{}, 4)
	host.editor.OnAutocompleteChange = func() {
		host.requestRender(false)
		changed <- struct{}{}
	}
	terminal.SendInput("/")
	waitForHostAutocompleteChange(t, changed)
	if !host.editor.IsShowingAutocomplete() {
		t.Fatalf("slash should show builtin autocomplete")
	}
	terminal.WaitForRender()
	viewport := strings.Join(terminal.GetViewport(), "\n")
	if !strings.Contains(viewport, "settings") || !strings.Contains(viewport, "model") {
		t.Fatalf("viewport missing builtin slash commands:\n%s", viewport)
	}

	terminal.SendInput("login")
	waitForHostAutocompleteChange(t, changed)
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Select authentication method:")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostSlashAutocompleteAfterLongContentDoesNotLeaveStaleEditor(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 35)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		ShowFooter:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/hotkeys")
	terminal.SendInput("\r")
	waitForTerminalOutput(t, terminal, "Keyboard Shortcuts")
	waitForViewport(t, terminal, "Run bash command")

	terminal.SendInput("/")
	waitForViewport(t, terminal, "settings")
	waitForViewport(t, terminal, "model")
	viewport := terminal.GetViewport()
	slashRow := viewportTrimmedLineIndex(viewport, "/")
	settingsRow := viewportLineIndex(viewport, "settings")
	footerRow := viewportLineIndex(viewport, "0.0%/")
	if slashRow < 0 || settingsRow < 0 || footerRow < 0 {
		t.Fatalf("viewport missing slash autocomplete rows:\n%s", strings.Join(viewport, "\n"))
	}
	if settingsRow <= slashRow || footerRow <= settingsRow {
		t.Fatalf("slash autocomplete row order wrong: slash=%d settings=%d footer=%d\n%s", slashRow, settingsRow, footerRow, strings.Join(viewport, "\n"))
	}
	fullWidthBorder := strings.Repeat("─", 120)
	borderCount := 0
	for _, line := range viewport {
		if strings.TrimSpace(StripAnsi(line)) == fullWidthBorder {
			borderCount++
		}
	}
	if borderCount != 3 {
		t.Fatalf("slash autocomplete should leave exactly one editor frame after long content, full-width border count=%d\n%s", borderCount, strings.Join(viewport, "\n"))
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostDefaultSlashAutocompleteRender(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:        runtimeHost,
		Terminal:           terminal,
		ClearScreenOnStart: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("/")
	waitForViewport(t, terminal, "settings")
	waitForViewport(t, terminal, "scoped-models")
	waitForViewport(t, terminal, "model")
	if !host.editor.IsShowingAutocomplete() {
		t.Fatalf("slash should show builtin autocomplete")
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShowsSlashAutocompleteFromCSIuPrintable(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:        runtimeHost,
		Terminal:           terminal,
		ClearScreenOnStart: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("\x1b[47u")
	waitForViewport(t, terminal, "settings")
	waitForViewport(t, terminal, "model")
	if !host.editor.IsShowingAutocomplete() {
		t.Fatalf("CSI-u slash should show builtin autocomplete")
	}
	if got := host.editor.GetText(); got != "/" {
		t.Fatalf("editor text = %q, want slash", got)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRunsOfficialPackageSlashCommand(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHostWithPackages(t, "official:gi-todo-widget")
	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	changed := make(chan struct{}, 4)
	host.editor.OnAutocompleteChange = func() {
		host.requestRender(false)
		changed <- struct{}{}
	}
	terminal.SendInput("/to")
	waitForHostAutocompleteChange(t, changed)
	terminal.WaitForRender()
	if viewport := strings.Join(terminal.GetViewport(), "\n"); !strings.Contains(viewport, "todo") {
		t.Fatalf("official package slash command missing from autocomplete:\n%s", viewport)
	}
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Todo Widget")
	waitForViewport(t, terminal, "Command: /todo")
	waitForSlotChildCount(t, host, "aboveEditor", 1)
	if mounts := host.viewTreeHost.MountsBySlot("aboveEditor"); len(mounts) != 1 || mounts[0].MountID != "official.gi-todo-widget.todo" {
		t.Fatalf("official todo widget mounts = %#v", mounts)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRendersDescriptorViewTreeMounts(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cwd := t.TempDir()
	agentDir := filepath.Join(cwd, "agent")
	writeJSON(t, filepath.Join(agentDir, "extensions", "widget.gi.json"), map[string]any{"gi": map[string]any{
		"extensionProtocol": "descriptor.v1",
		"id":                "descriptor-widget",
		"viewTrees": []any{map[string]any{
			"mountId":  "descriptor.widget",
			"slot":     "aboveEditor",
			"priority": 30,
			"view":     map[string]any{"type": "text", "text": "Descriptor Widget"},
		}},
	}})
	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		NoSession: true,
		Model:     "openai/gpt-4o-mini",
	}, CLIOptions{CWD: cwd, AgentDir: agentDir})
	if err != nil {
		t.Fatal(err)
	}
	terminal := gitui.NewVirtualTerminal(100, 18)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	waitForViewport(t, terminal, "Descriptor Widget")
	waitForSlotChildCount(t, host, "aboveEditor", 1)

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRunsOfficialToolsPackageSlashCommand(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHostWithPackages(t, "official:gi-tools-ui")
	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("/tools")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Tools")
	waitForViewport(t, terminal, "Active tools:")
	waitForViewport(t, terminal, "read")
	waitForSlotChildCount(t, host, "aboveEditor", 1)
	if mounts := host.viewTreeHost.MountsBySlot("aboveEditor"); len(mounts) != 1 || mounts[0].MountID != "official.gi-tools-ui.tools" {
		t.Fatalf("official tools mounts = %#v", mounts)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRunsOfficialPlanPackageSlashCommand(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHostWithPackages(t, "official:gi-plan-mode")
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.settingsManager.SetTheme("dark")
	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("/plan ship it")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Plan Mode")
	waitForViewport(t, terminal, "Command: /plan ship it")
	waitForViewport(t, terminal, "Status: read-only")
	waitForViewport(t, terminal, "Active tools: read, grep, find, ls")
	if got := sessionHost.AgentSession().GetActiveToolNames(); !reflectStringSetEqual(got, []string{"read", "grep", "find", "ls"}) {
		t.Fatalf("active tools = %#v", got)
	}
	waitForSlotChildCount(t, host, "aboveEditor", 1)
	if mounts := host.viewTreeHost.MountsBySlot("aboveEditor"); len(mounts) != 1 || mounts[0].MountID != "official.gi-plan-mode.plan" {
		t.Fatalf("official plan mounts = %#v", mounts)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRunsOfficialApprovalPackageSlashCommand(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHostWithPackages(t, "official:gi-approval-gate")
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	runtime := sessionHost.ProtocolExtensionRuntime()
	executeOfficialTool(t, runtime, "gi-approval-gate", "approval_gate_request", map[string]any{
		"action": "push",
		"risk":   "high",
		"path":   "main",
	})
	executeOfficialTool(t, runtime, "gi-approval-gate", "approval_gate_decide", map[string]any{
		"decision": "approve",
		"reason":   "reviewed in live TTY",
	})

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("/approvals")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Approval Gate")
	waitForViewport(t, terminal, "Approval decision: approve")
	waitForViewport(t, terminal, "Decision: approve")
	waitForViewport(t, terminal, "Risk: high")
	waitForViewport(t, terminal, "Reason: reviewed in live TTY")
	waitForSlotChildCount(t, host, "aboveEditor", 1)
	if mounts := host.viewTreeHost.MountsBySlot("aboveEditor"); len(mounts) != 1 || mounts[0].MountID != "official.gi-approval-gate.approvals" {
		t.Fatalf("official approval mounts = %#v", mounts)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRunsOfficialSubagentsPackageSlashCommand(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHostWithPackages(t, "official:gi-subagents")
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	runtime := sessionHost.ProtocolExtensionRuntime()
	executeOfficialTool(t, runtime, "gi-subagents", "subagent_spawn", map[string]any{
		"task": "inspect protocol",
	})

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("/subagents")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Subagents")
	waitForViewport(t, terminal, "Status: done")
	waitForViewport(t, terminal, "Last subagent run completed")
	waitForSlotChildCount(t, host, "aboveEditor", 1)
	if mounts := host.viewTreeHost.MountsBySlot("aboveEditor"); len(mounts) != 1 || mounts[0].MountID != "official.gi-subagents.subagents" {
		t.Fatalf("official subagents mounts = %#v", mounts)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRunsOfficialMCPPackageSlashCommand(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHostWithPackages(t, "official:gi-mcp-adapter")
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	runtime := sessionHost.ProtocolExtensionRuntime()
	executeOfficialTool(t, runtime, "gi-mcp-adapter", "mcp_call", map[string]any{
		"tool": "inspect",
	})

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("/mcp")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "MCP Adapter")
	waitForViewport(t, terminal, "Command: /mcp")
	waitForViewport(t, terminal, "Last MCP call: inspect")
	waitForSlotChildCount(t, host, "aboveEditor", 1)
	if mounts := host.viewTreeHost.MountsBySlot("aboveEditor"); len(mounts) != 1 || mounts[0].MountID != "official.gi-mcp-adapter.mcp" {
		t.Fatalf("official mcp mounts = %#v", mounts)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRunsOfficialGitGuardPackageWithLiveConfirm(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	runtimeHost := newOfflineInteractiveRuntimeHostWithPackages(t, "official:gi-git-guard")
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	session := sessionHost.AgentSession()
	if session == nil || session.SessionManager == nil {
		t.Fatal("runtime host has no agent session")
	}
	cwd := session.SessionManager.GetCWD()
	runOfficialPackageGit(t, cwd, "init")
	if err := os.WriteFile(filepath.Join(cwd, "danger.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runtime := sessionHost.ProtocolExtensionRuntime()
	tool := findOfficialTool(t, runtime, "gi-git-guard", "git_guard_check")

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	type toolExecution struct {
		result SDKToolResult
		err    error
	}
	toolCh := make(chan toolExecution, 1)
	go func() {
		result, err := tool.Execute("test-git_guard_check", map[string]any{
			"action":              "push",
			"cwd":                 cwd,
			"requireConfirmation": true,
		})
		toolCh <- toolExecution{result: result, err: err}
	}()
	waitForViewport(t, terminal, "Git Guard")
	waitForViewport(t, terminal, "Action: push")
	waitForViewport(t, terminal, "Dirty files:")
	terminal.SendInput("\r")

	var execution toolExecution
	select {
	case execution = <-toolCh:
	case <-time.After(time.Second):
		t.Fatal("git guard tool did not finish after live confirm")
	}
	if execution.err != nil {
		t.Fatalf("git guard tool: %v", execution.err)
	}
	resultText := sdkToolText(execution.result)
	if !strings.Contains(resultText, `"confirmed"`) || !strings.Contains(resultText, "danger.txt") {
		t.Fatalf("git guard result = %s", resultText)
	}

	if err := host.FocusEditor(); err != nil {
		t.Fatal(err)
	}
	terminal.SendInput("/git-guard")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Git Guard")
	waitForViewport(t, terminal, "Status: confirmed")
	waitForViewport(t, terminal, "Review branch")
	waitForViewport(t, terminal, "danger.txt")
	waitForSlotChildCount(t, host, "aboveEditor", 1)
	if mounts := host.viewTreeHost.MountsBySlot("aboveEditor"); len(mounts) != 1 || mounts[0].MountID != "official.gi-git-guard.git-guard" {
		t.Fatalf("official git guard mounts = %#v", mounts)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRunsOfficialFooterPackageSlashCommand(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHostWithPackages(t, "official:gi-powerline-footer")
	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("/footer")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Powerline Footer")
	waitForViewport(t, terminal, "Command: /footer")
	waitForSlotChildCount(t, host, "footer", 1)
	if mounts := host.viewTreeHost.MountsBySlot("footer"); len(mounts) != 1 || mounts[0].MountID != "official.gi-powerline-footer.footer" {
		t.Fatalf("official footer mounts = %#v", mounts)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostHandlesSettingsCommand(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 44)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:      runtimeHost,
		Terminal:         terminal,
		Messages:         []string{"/settings"},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	waitForViewport(t, terminal, "Settings")
	waitForViewport(t, terminal, "Current model")
	waitForViewport(t, terminal, "openai/gpt-4o-mini")
	waitForViewport(t, terminal, "Install telemetry")
}

func TestCLIInteractiveTUIHostShowsSettingsSelectorAndTogglesTelemetry(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	terminal := gitui.NewVirtualTerminal(120, 48)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/settings")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Auto-compact")
	waitForViewport(t, terminal, "Auto-resize images")
	terminal.SendInput("installtelemetry")
	waitForViewport(t, terminal, "Install telemetry")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Send an anonymous version/update ping")
	if sessionHost.settingsManager.GetEnableInstallTelemetry() {
		t.Fatal("install telemetry should be disabled after settings selector toggle")
	}
	terminal.SendInput("\x1b")
	waitForCondition(t, func() bool { return host.ui.FocusedComponent() == host.editor }, "settings selector to restore default editor")
	if strings.Contains(strings.Join(terminal.GetViewport(), "\n"), "Settings updated") {
		t.Fatalf("settings selector should close silently like Pi:\n%s", strings.Join(terminal.GetViewport(), "\n"))
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostSettingsSelectorCtrlCCancelsLikePi(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 48)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/settings")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Auto-compact")
	terminal.SendInput("\x03")
	waitForCondition(t, func() bool { return host.ui.FocusedComponent() == host.editor }, "settings selector to cancel on ctrl+c")

	host.editor.SetText("/login")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Select authentication method:")
	if strings.Contains(strings.Join(terminal.GetViewport(), "\n"), "No matching settings") {
		t.Fatalf("ctrl+c should cancel settings selector before /login:\n%s", strings.Join(terminal.GetViewport(), "\n"))
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostAppliesThemeThroughHost(t *testing.T) {
	previousTheme := tuiActiveThemeSnapshot()
	t.Cleanup(func() { tuiSetActiveThemePalette(previousTheme) })
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    gitui.NewVirtualTerminal(100, 24),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.SetTUITheme("light"); err != nil {
		t.Fatal(err)
	}
	if got := sessionHost.settingsManager.GetTheme(); got != "light" {
		t.Fatalf("theme = %q, want light", got)
	}
	if got, want := tuiThemeAccent("x"), "\x1b[38;2;90;128;128mx"+tuiResetFG; got != want {
		t.Fatalf("active theme accent = %q, want %q", got, want)
	}
}

func TestCLIInteractiveTUIHostDispatchesThemeChangeToViewTreeSubscribers(t *testing.T) {
	previousTheme := tuiActiveThemeSnapshot()
	t.Cleanup(func() { tuiSetActiveThemePalette(previousTheme) })
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	viewHost := NewViewTreeHost()
	var events []ViewTreeEvent
	unsubscribe := viewHost.OnEvent(func(event ViewTreeEvent) {
		events = append(events, event)
	})
	defer unsubscribe()
	if err := viewHost.Mount("theme-panel", "footer", ViewTreeNode{
		Type: "box",
		ID:   "theme-root",
		Children: []ViewTreeNode{
			{Type: "text", ID: "theme-subscriber", Text: "Theme", Events: []string{"theme_change"}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:  runtimeHost,
		Terminal:     gitui.NewVirtualTerminal(100, 24),
		ViewTreeHost: viewHost,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.SetTUITheme("light"); err != nil {
		t.Fatal(err)
	}
	if got := sessionHost.settingsManager.GetTheme(); got != "light" {
		t.Fatalf("theme = %q, want light", got)
	}
	for _, event := range events {
		if event.MountID == "theme-panel" && event.NodeID == "theme-subscriber" && event.Event == "theme_change" {
			if event.Data["name"] != "light" || event.Data["preview"] != false {
				t.Fatalf("theme_change data = %#v", event.Data)
			}
			return
		}
	}
	t.Fatalf("theme_change was not dispatched: %#v", events)
}

func TestCLISettingsListComponentUsesPiBorderTheme(t *testing.T) {
	list := gitui.NewSettingsList([]gitui.SettingItem{{
		ID:           "autocompact",
		Label:        "Auto-compact",
		CurrentValue: "true",
		Values:       []string{"true", "false"},
	}}, 5, tuiThemeSettingsList())
	component := cliSettingsListComponent{list: list}

	lines := component.Render(40)
	if len(lines) < 2 {
		t.Fatalf("settings lines = %#v", lines)
	}
	wantBorder := "\x1b[38;2;95;135;255m" + strings.Repeat("─", 40)
	if !strings.HasPrefix(lines[0], wantBorder) || !strings.HasPrefix(lines[len(lines)-1], wantBorder) {
		t.Fatalf("settings borders not themed: %#v", lines)
	}
}

func TestCLIInteractiveTUISettingsListIncludesAndAppliesPiControls(t *testing.T) {
	gitui.SetCapabilities(gitui.TerminalCapabilities{Images: true, Protocol: gitui.ImageProtocolKitty})
	defer gitui.ResetCapabilitiesCache()

	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	rpcHost := NewRPCSessionHost(sessionHost.session)
	settings := sessionHost.settingsManager
	items := settingsListItems(rpcHost, rpcHost.GetState(), settings, []string{"dark", "light"})
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.ID] = true
	}
	for _, id := range []string{
		"autocompact",
		"show-images",
		"image-width-cells",
		"auto-resize-images",
		"block-images",
		"skill-commands",
		"transport",
		"hide-thinking",
		"double-escape-action",
		"tree-filter-mode",
		"warnings",
		"terminal-progress",
	} {
		if !seen[id] {
			t.Fatalf("settings item %q missing from %#v", id, items)
		}
	}

	host := &CLIInteractiveTUIHost{
		editor: gitui.NewEditor(gitui.EditorTheme{}, gitui.EditorOptions{}),
		chat:   gitui.NewContainer(),
		ui:     gitui.NewTUI(gitui.NewVirtualTerminal(40, 10)),
	}
	tool := NewToolExecutionComponent("read", "tool-settings", map[string]any{"path": "image.png"}, ToolDefinition{Name: "read"}, t.TempDir())
	host.chat.AddChild(tool)
	host.pendingTools = map[string]*ToolExecutionComponent{"tool-settings": tool}
	host.applySettingsListChange(rpcHost, settings, "autocompact", "false")
	host.applySettingsListChange(rpcHost, settings, "transport", "websocket-cached")
	host.applySettingsListChange(rpcHost, settings, "show-images", "false")
	host.applySettingsListChange(rpcHost, settings, "image-width-cells", "120")
	host.applySettingsListChange(rpcHost, settings, "auto-resize-images", "false")
	host.applySettingsListChange(rpcHost, settings, "skill-commands", "false")
	host.applySettingsListChange(rpcHost, settings, "show-hardware-cursor", "true")
	host.applySettingsListChange(rpcHost, settings, "editor-padding", "3")
	host.applySettingsListChange(rpcHost, settings, "autocomplete-max-visible", "20")
	host.applySettingsListChange(rpcHost, settings, "clear-on-shrink", "true")
	host.applySettingsListChange(rpcHost, settings, "hide-thinking", "true")
	host.applySettingsListChange(rpcHost, settings, "double-escape-action", "fork")
	host.applySettingsListChange(rpcHost, settings, "tree-filter-mode", "labeled-only")
	host.applySettingsListChange(rpcHost, settings, "warning-anthropic-extra-usage", "false")
	host.applySettingsListChange(rpcHost, settings, "terminal-progress", "true")

	if rpcHost.AutoCompactionEnabled ||
		settings.GetTransport() != "websocket-cached" ||
		settings.GetShowImages() ||
		settings.GetImageWidthCells() != 120 ||
		settings.GetImageAutoResize() ||
		settings.GetEnableSkillCommands() ||
		!settings.GetHideThinkingBlock() ||
		settings.GetDoubleEscapeAction() != "fork" ||
		settings.GetTreeFilterMode() != "labeled-only" ||
		settings.GetWarnings().AnthropicExtraUsage ||
		!settings.GetShowTerminalProgress() {
		t.Fatalf("settings not applied")
	}
	if host.editor.GetPaddingX() != 3 || host.editor.GetAutocompleteMaxVisible() != 20 ||
		!host.ui.ShowHardwareCursor() || !host.ui.ClearOnShrink() {
		t.Fatalf("live TUI settings not applied")
	}
	if tool.showImages || tool.imageWidthCells != 120 {
		t.Fatalf("tool image settings not applied: show=%v width=%d", tool.showImages, tool.imageWidthCells)
	}
}

func TestCLIInteractiveTUISettingsListHidesInlineImageControlsWhenUnsupportedPiStyle(t *testing.T) {
	gitui.SetCapabilities(gitui.TerminalCapabilities{Images: false, Protocol: gitui.ImageProtocolNone})
	defer gitui.ResetCapabilitiesCache()

	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	rpcHost := NewRPCSessionHost(sessionHost.session)
	items := settingsListItems(rpcHost, rpcHost.GetState(), sessionHost.settingsManager, []string{"dark", "light"})

	seen := map[string]bool{}
	for _, item := range items {
		seen[item.ID] = true
	}
	for _, id := range []string{"show-images", "image-width-cells"} {
		if seen[id] {
			t.Fatalf("inline image setting %q should be hidden when terminal images are unsupported: %#v", id, items)
		}
	}
	for _, id := range []string{"auto-resize-images", "block-images"} {
		if !seen[id] {
			t.Fatalf("image input setting %q should remain visible without terminal image support: %#v", id, items)
		}
	}
}

func TestCLIInteractiveTUIHostSkillCommandSettingRefreshesAutocompletePiStyle(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cwd := t.TempDir()
	agentDir := filepath.Join(cwd, "agent")
	writeResourceSkill(t, filepath.Join(cwd, ConfigDirName, "skills", "review", "SKILL.md"), "review", "Review", "Review content")
	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		NoSession: true,
		Model:     "openai/gpt-4o-mini",
	}, CLIOptions{
		CWD:      cwd,
		AgentDir: agentDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{RuntimeHost: runtimeHost})
	if err != nil {
		t.Fatal(err)
	}

	if !slashCommandNamesContain(host.autocompleteSlashCommands(), "skill:review") {
		t.Fatalf("skill command should be visible by default: %#v", host.autocompleteSlashCommands())
	}

	rpcHost := NewRPCSessionHost(sessionHost.session)
	host.applySettingsListChange(rpcHost, sessionHost.settingsManager, "skill-commands", "false")
	if slashCommandNamesContain(host.autocompleteSlashCommands(), "skill:review") {
		t.Fatalf("skill command should be hidden when setting is disabled: %#v", host.autocompleteSlashCommands())
	}
}

func TestCLIInteractiveTUIHostUsesEffectiveAppKeybindingsPiStyle(t *testing.T) {
	host := &CLIInteractiveTUIHost{
		editor: gitui.NewEditor(gitui.EditorTheme{}),
		chat:   gitui.NewContainer(),
	}
	keybindings := mergeKeybindingsConfig(DefaultProtocolKeybindings(), KeybindingsConfig{
		"app.tools.expand": "ctrl+x",
	})

	if !host.handleAppActionKey("\x18", keybindings) {
		t.Fatal("custom app.tools.expand key was not handled")
	}
	if !host.toolOutputExpanded {
		t.Fatal("custom app.tools.expand key did not toggle tool output")
	}
	if host.handleAppActionKey("\x0f", keybindings) {
		t.Fatal("default ctrl+o should not trigger after app.tools.expand is rebound")
	}
}

func TestCLIInteractiveTUIHostPrefixesResourceSlashDescriptionsPiStyle(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cwd := t.TempDir()
	agentDir := filepath.Join(cwd, "agent")
	writeResourceSkill(t, filepath.Join(cwd, ConfigDirName, "skills", "review", "SKILL.md"), "review", "Review skill", "Review content")
	writeResourceFile(t, filepath.Join(cwd, ConfigDirName, "prompts", "plan.md"), "---\ndescription: Plan work\nargument-hint: <scope>\n---\nPlan $1\n")
	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		NoSession: true,
		Model:     "openai/gpt-4o-mini",
	}, CLIOptions{
		CWD:      cwd,
		AgentDir: agentDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	extensionContext := &ProtocolExtensionContext{
		runtime: sessionHost.session.ExtensionRuntime,
		source:  ProtocolSourceInfo{Path: filepath.Join(cwd, ConfigDirName, "extensions", "deploy.gi.json"), Source: "local", Scope: "project"},
	}
	if err := extensionContext.RegisterCommand("deploy", ProtocolCommandDefinition{
		Description:  "Deploy work",
		ArgumentHint: "<env>",
	}); err != nil {
		t.Fatal(err)
	}
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{RuntimeHost: runtimeHost})
	if err != nil {
		t.Fatal(err)
	}
	commands := host.autocompleteSlashCommands()
	plan, ok := slashCommandByName(commands, "plan")
	if !ok || plan.Description != "[p] Plan work" || plan.ArgumentHint != "<scope>" {
		t.Fatalf("prompt command = %#v ok=%v", plan, ok)
	}
	skill, ok := slashCommandByName(commands, "skill:review")
	if !ok || skill.Description != "[p] Review skill" {
		t.Fatalf("skill command = %#v ok=%v", skill, ok)
	}
	deploy, ok := slashCommandByName(commands, "deploy")
	if !ok || deploy.Description != "[p] Deploy work" || deploy.ArgumentHint != "<env>" {
		t.Fatalf("extension command = %#v ok=%v", deploy, ok)
	}
	if got := autocompleteDescriptionWithSource("Ship", ProtocolSourceInfo{Source: "git:https://github.com/nowa/gi-pkg@v1.2.3", Scope: "project"}); got != "[p:git:github.com/nowa/gi-pkg@v1.2.3] Ship" {
		t.Fatalf("git source description = %q", got)
	}
	if got := autocompleteDescriptionWithSource("Tools", ProtocolSourceInfo{Source: "official:gi-tools-ui", Scope: "project"}); got != "[p:official:gi-tools-ui] Tools" {
		t.Fatalf("official source description = %q", got)
	}
}

func TestCLIInteractiveTUIHostModelSlashArgumentAutocompletePiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.session.SetScopedModels([]ScopedModel{{
		Model:         llm.MustGetModel("openai", "gpt-5-mini"),
		ThinkingLevel: ThinkingOff,
	}})
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{RuntimeHost: runtimeHost})
	if err != nil {
		t.Fatal(err)
	}
	modelCommand, ok := slashCommandByName(host.autocompleteSlashCommands(), "model")
	if !ok || modelCommand.GetArgumentCompletions == nil {
		t.Fatalf("/model command should expose argument completions: %#v ok=%v", modelCommand, ok)
	}

	items := modelCommand.GetArgumentCompletions("gpt 5")
	if len(items) != 1 {
		t.Fatalf("/model scoped completions = %#v, want one scoped item", items)
	}
	if items[0].Value != "openai/gpt-5-mini" || items[0].Label != "gpt-5-mini" ||
		items[0].Description != "openai" {
		t.Fatalf("/model completion = %#v", items[0])
	}

	for _, item := range modelCommand.GetArgumentCompletions("gpt") {
		if item.Value == "openai/gpt-4o-mini" {
			t.Fatalf("/model completions should respect scoped models, got %#v", item)
		}
	}
}

func TestCLIInteractiveTUIHostModelSlashExactMatchRespectsScopedModelsPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.session.SetScopedModels([]ScopedModel{{
		Model:         llm.MustGetModel("openai", "gpt-5-mini"),
		ThinkingLevel: ThinkingOff,
	}})
	host := &CLIInteractiveTUIHost{
		runtimeHost:      runtimeHost,
		exitAfterInitial: true,
	}

	if err := host.handleModelSlashCommand("openai/gpt-4o"); err == nil ||
		!strings.Contains(err.Error(), "Model not found") {
		t.Fatalf("outside scoped model error = %v, want Model not found", err)
	}
	if got := sessionHost.session.Agent.State.Model.ID; got != "gpt-4o-mini" {
		t.Fatalf("model after rejected scoped command = %q, want original gpt-4o-mini", got)
	}

	if err := host.handleModelSlashCommand("openai/gpt-5-mini"); err != nil {
		t.Fatalf("scoped model command returned %v", err)
	}
	if got := sessionHost.session.Agent.State.Model.ID; got != "gpt-5-mini" {
		t.Fatalf("model after scoped command = %q, want gpt-5-mini", got)
	}
}

func TestCLIInteractiveTUISettingsListUsesPiStyleSelectSubmenus(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	rpcHost := NewRPCSessionHost(sessionHost.session)
	items := settingsListItems(rpcHost, rpcHost.GetState(), sessionHost.settingsManager, []string{"dark", "light"})
	theme := settingItemForTest(t, items, "theme")
	if theme.Submenu == nil || len(theme.Values) != 0 {
		t.Fatalf("theme item should use submenu, got %#v", theme)
	}
	thinking := settingItemForTest(t, items, "thinking")
	if thinking.Submenu == nil || len(thinking.Values) != 0 {
		t.Fatalf("thinking item should use submenu, got %#v", thinking)
	}
	warnings := settingItemForTest(t, items, "warnings")
	if warnings.Submenu == nil || len(warnings.Values) != 0 {
		t.Fatalf("warnings item should use submenu, got %#v", warnings)
	}

	var selected string
	var changed bool
	component := theme.Submenu("dark", func(value string, didChange bool) {
		selected = value
		changed = didChange
	})
	handler, ok := component.(gitui.InputHandler)
	if !ok {
		t.Fatalf("submenu component = %T, want InputHandler", component)
	}
	handler.HandleInput("l")
	handler.HandleInput("i")
	rendered := strings.Join(component.Render(80), "\n")
	if !strings.Contains(rendered, "light") || strings.Contains(rendered, "dark (current)") {
		t.Fatalf("theme submenu should search by label: %q", rendered)
	}
	handler.HandleInput("\r")
	if selected != "light" || !changed {
		t.Fatalf("theme selection selected=%q changed=%v", selected, changed)
	}

	var previews []string
	items = settingsListItems(rpcHost, rpcHost.GetState(), sessionHost.settingsManager, []string{"dark", "light"}, settingsListItemsOptions{
		OnThemePreview: func(themeName string) {
			previews = append(previews, themeName)
		},
	})
	theme = settingItemForTest(t, items, "theme")
	component = theme.Submenu("dark", func(string, bool) {})
	handler, ok = component.(gitui.InputHandler)
	if !ok {
		t.Fatalf("preview submenu component = %T, want InputHandler", component)
	}
	handler.HandleInput("l")
	handler.HandleInput("i")
	handler.HandleInput("\x1b")
	if !reflect.DeepEqual(previews, []string{"light", "light", "dark"}) {
		t.Fatalf("theme previews = %#v", previews)
	}

	warningComponent := warnings.Submenu("configure", func(string, bool) {})
	warningHandler, ok := warningComponent.(gitui.InputHandler)
	if !ok {
		t.Fatalf("warning submenu component = %T, want InputHandler", warningComponent)
	}
	warningHandler.HandleInput("\r")
	if sessionHost.settingsManager.GetWarnings().AnthropicExtraUsage {
		t.Fatalf("warnings submenu did not toggle Anthropic extra usage warning")
	}
}

func TestCLIInteractiveTUIHostShowsModelSelectorForInteractiveModelCommand(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	terminal := gitui.NewVirtualTerminal(120, 28)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/model")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Only showing models from configured providers")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Model: openai/gpt-4o-mini")
	if sessionHost.settingsManager.GetDefaultProvider() != "openai" ||
		sessionHost.settingsManager.GetDefaultModel() != "gpt-4o-mini" {
		t.Fatalf("default model settings provider=%q model=%q",
			sessionHost.settingsManager.GetDefaultProvider(),
			sessionHost.settingsManager.GetDefaultModel())
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostModelSelectorTabSwitchesFromScopedToAllPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.session.Agent.State.Model = llm.MustGetModel("openai", "gpt-5-mini")
	sessionHost.session.SetScopedModels([]ScopedModel{{
		Model:         llm.MustGetModel("openai", "gpt-5-mini"),
		ThinkingLevel: ThinkingOff,
	}})
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/model gpt-4o-mini")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Scope: all | scoped")
	waitForViewport(t, terminal, "No matching models")
	terminal.SendInput("\t")
	waitForViewport(t, terminal, "gpt-4o-mini")
	terminal.SendInput("\r")
	waitForNoOverlay(t, host)
	if got := sessionHost.session.Agent.State.Model.ID; got != "gpt-4o-mini" {
		t.Fatalf("selected model = %q, want gpt-4o-mini", got)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostScopedModelsCommandUpdatesSessionAndSettings(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/scoped-models")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Model Configuration")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "(unsaved)")
	if len(sessionHost.session.ScopedModels) != 1 {
		t.Fatalf("session scoped models = %#v, want one selected model", sessionHost.session.ScopedModels)
	}
	terminal.SendInput("\x13")
	waitForViewport(t, terminal, "Model selection saved to settings")
	if len(sessionHost.settingsManager.GetEnabledModels()) != 1 {
		t.Fatalf("enabled model settings = %#v, want one selected model", sessionHost.settingsManager.GetEnabledModels())
	}
	terminal.SendInput("\x1b")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostLoginLogoutWithArgsSubmitAsPromptsPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.modelRegistry.authStorage.Set("openai", AuthCredential{Type: "api_key", Key: "stored-openai"})
	var prompts []string
	sessionHost.session.Responder = func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error) {
		prompts = append(prompts, prompt)
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("agent saw: " + prompt)}}, nil
	}
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Messages: []string{
			"/login openai",
			"/logout openai",
		},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(prompts, []string{"/login openai", "/logout openai"}) {
		t.Fatalf("prompts = %#v", prompts)
	}
	waitForViewport(t, terminal, "agent saw: /logout openai")
	if !sessionHost.modelRegistry.authStorage.Has("openai") {
		t.Fatal("/logout with args should not remove credentials")
	}
}

func TestCLIInteractiveTUIHostLoginSlashUsesProviderSelectorPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/login")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Select authentication method:")
	terminal.SendInput("\x1b[B")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Select provider to configure:")
	terminal.SendInput("openai openai")
	waitForViewport(t, terminal, "> openai openai")
	waitForViewport(t, terminal, "OpenAI")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Enter API key:")
	terminal.SendInput("test-openai-key")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Saved API key for OpenAI")
	waitForViewport(t, terminal, "~/.gi/agent/auth.json")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostLoginBedrockUsesPiInfoDialog(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/login")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Select authentication method:")
	terminal.SendInput("\x1b[B")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Select provider to configure:")
	terminal.SendInput("bedrock")
	waitForViewport(t, terminal, "Amazon Bedrock")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Amazon Bedrock setup")
	waitForViewport(t, terminal, "Amazon Bedrock uses AWS credentials instead of a single API key.")
	waitForViewport(t, terminal, "docs/providers.md")
	waitForViewport(t, terminal, "to close")
	if viewport := strings.Join(terminal.GetViewport(), "\n"); strings.Contains(viewport, "~/.gi/agent/models.json") {
		t.Fatalf("Bedrock info dialog should point to provider docs, not models.json:\n%s", viewport)
	}
	terminal.SendInput("\x1b")
	waitForHostEditor(t, host)
	if viewport := strings.Join(terminal.GetViewport(), "\n"); strings.Contains(viewport, "Resume cancelled") {
		t.Fatalf("/resume cancel should return silently like Pi:\n%s", viewport)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostLoginSubscriptionShowsOAuthDialogPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/login")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Select authentication method:")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Select provider to configure:")
	waitForViewport(t, terminal, "Anthropic (Claude Pro/Max)")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Login to Anthropic (Claude Pro/Max)")
	waitForViewport(t, terminal, "https://claude.ai/oauth/authorize")
	waitForViewport(t, terminal, oauthClickHint())
	waitForViewport(t, terminal, "Complete login in your browser.")
	waitForViewport(t, terminal, "Paste redirect URL below, or complete login in browser:")
	waitForViewport(t, terminal, "to cancel")
	if viewport := strings.Join(terminal.GetViewport(), "\n"); strings.Contains(viewport, "Subscription login is not implemented yet") {
		t.Fatalf("OAuth login should show the Pi-style login dialog, not the placeholder status:\n%s", viewport)
	}
	terminal.SendInput("\x1b")
	waitForHostEditor(t, host)

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostLogoutSlashUsesProviderSelectorPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.modelRegistry.authStorage.Set("openai", AuthCredential{Type: "api_key", Key: "stored-openai"})
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/logout")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Select provider to logout:")
	waitForViewport(t, terminal, "OpenAI")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Removed stored credential for openai")
	if sessionHost.modelRegistry.authStorage.Has("openai") {
		t.Fatalf("openai credential should be removed")
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostLogoutSlashWithoutCredentialsMatchesPiText(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.modelRegistry.authStorage = NewInMemoryAuthStorage(AuthStorageData{})
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/logout")
	terminal.SendInput("\r")
	expected := "No stored credentials to remove. /logout only removes credentials saved by /login; environment variables and models.json config are unchanged."
	waitForViewport(t, terminal, "No stored credentials to remove. /logout only removes credentials saved by /login")
	viewport := strings.Join(terminal.GetViewport(), "\n")
	normalizedViewport := strings.Join(strings.Fields(StripAnsi(viewport)), " ")
	if !strings.Contains(normalizedViewport, expected) {
		t.Fatalf("empty /logout text did not match Pi:\n%s", viewport)
	}
	if strings.Contains(viewport, "~/.gi/agent/auth.json") {
		t.Fatalf("empty /logout should not expose Gi-specific auth path:\n%s", viewport)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostHandlesHotkeysCommand(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 40)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:      runtimeHost,
		Terminal:         terminal,
		Messages:         []string{"/hotkeys"},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	output := strings.Join(terminal.GetScrollBuffer(), "\n")
	if !strings.Contains(output, "Keyboard Shortcuts") || !strings.Contains(output, "Run bash command (excluded from context)") {
		t.Fatalf("hotkeys output missing expected content:\n%s", output)
	}
	renderedMarkdown := strings.Join(newCLIMarkdownWithOptions(strings.TrimSpace(host.hotkeysMarkdown()), gitui.MarkdownOptions{PaddingX: 1, PaddingY: 1}).Render(120), "\n")
	if !strings.Contains(renderedMarkdown, tuiThemeAccent("Ctrl+C")) ||
		!strings.Contains(newCLIDynamicBorder().Render(120)[0], tuiThemeBorder(strings.Repeat("─", 120))) ||
		!strings.Contains(tuiThemeBoldAccent("Keyboard Shortcuts"), "\x1b[38;2;138;190;183mKeyboard Shortcuts") {
		t.Fatalf("hotkeys theme helpers did not match Pi styling:\n%q", renderedMarkdown)
	}
}

func TestCLIInteractiveTUIHostHotkeysUseEffectiveKeybindingsPiStyle(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cwd := t.TempDir()
	agentDir := filepath.Join(cwd, "agent")
	writeJSON(t, filepath.Join(agentDir, "keybindings.json"), map[string]any{
		"tui.input.submit":         "ctrl+enter",
		"app.model.select":         "ctrl+m",
		"app.clipboard.pasteImage": "ctrl+i",
	})
	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		NoSession: true,
		Model:     "openai/gpt-4o-mini",
	}, CLIOptions{CWD: cwd, AgentDir: agentDir})
	if err != nil {
		t.Fatal(err)
	}
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	mustLoadProtocolFactories(t, sessionHost.session.ExtensionRuntime, protocolShortcutFactory("shortcut.gi.json", "ctrl+y", "Run package shortcut"))

	terminal := gitui.NewVirtualTerminal(120, 80)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:      runtimeHost,
		Terminal:         terminal,
		Messages:         []string{"/hotkeys"},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	output := strings.Join(terminal.GetScrollBuffer(), "\n")
	for _, expected := range []string{
		"Ctrl+Enter",
		"Send message",
		"Ctrl+M",
		"Open model selector",
		"Ctrl+I",
		"Paste image from clipboard",
		"Ctrl+Y",
		"Run package shortcut",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("hotkeys output missing %q:\n%s", expected, output)
		}
	}
}

func TestCLIInteractiveTUIHostHandlesChangelogCommand(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	if err := os.WriteFile(filepath.Join(sessionHost.session.SessionManager.GetCWD(), "CHANGELOG.md"), []byte("## 0.1.0\n\n- Added TTY changelog"), 0o644); err != nil {
		t.Fatal(err)
	}
	terminal := gitui.NewVirtualTerminal(120, 28)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:      runtimeHost,
		Terminal:         terminal,
		Messages:         []string{"/changelog"},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	waitForViewport(t, terminal, "What's New")
	waitForViewport(t, terminal, "Added TTY changelog")
}

func TestCLIInteractiveTUIHostHandlesShareCommand(t *testing.T) {
	t.Setenv("GI_SHARE_VIEWER_URL", "https://share.gi.test/session/")
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 28)
	var exportedHTML string
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Messages: []string{
			"share this session",
			"/share",
		},
		ShareCreateGist: func(_ context.Context, path string) (string, error) {
			content, err := os.ReadFile(path)
			if err != nil {
				return "", err
			}
			exportedHTML = string(content)
			return "https://gist.github.com/nowa/abc123", nil
		},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	waitForViewport(t, terminal, "Share URL: https://share.gi.test/session/#abc123")
	waitForViewport(t, terminal, "Gist: https://gist.github.com/nowa/abc123")
	if !strings.Contains(exportedHTML, "share this session") || !strings.Contains(exportedHTML, "Response to: share this session") {
		t.Fatalf("exported HTML missing session content:\n%s", exportedHTML)
	}
}

func TestCLIInteractiveTUIHostShareEmptySessionShowsPiExportError(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 28)
	gistCalled := make(chan struct{}, 1)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		ShareCreateGist: func(_ context.Context, _ string) (string, error) {
			gistCalled <- struct{}{}
			return "https://gist.github.com/nowa/empty", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("/share")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Error: Failed to export session: Nothing to export yet - start a conversation first")
	select {
	case <-gistCalled:
		t.Fatal("empty /share should fail before creating a gist")
	default:
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostExportEmptySessionShowsPiExportError(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 28)
	exportPath := filepath.Join(t.TempDir(), "empty.html")
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("/export " + exportPath)
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Error: Failed to export session: Nothing to export yet - start a conversation first")
	if _, err := os.Stat(exportPath); !os.IsNotExist(err) {
		t.Fatalf("empty export file stat err = %v, want not exist", err)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShareCommandCanBeCancelled(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	mustPrompt(t, sessionHost.session, "share cancellable session")
	terminal := gitui.NewVirtualTerminal(120, 28)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		ShareCreateGist: func(ctx context.Context, _ string) (string, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return "", ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/share")
	terminal.SendInput("\r")
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("share gist creation did not start")
	}
	waitForViewport(t, terminal, "Creating gist")
	terminal.SendInput("\x1b")
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("share gist creation was not cancelled")
	}
	waitForViewport(t, terminal, "Share cancelled")
	if host.ui.FocusedComponent() != host.editor {
		t.Fatal("default editor focus was not restored after share cancellation")
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostHandlesPiLocalInteractiveCommands(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.session.CompactionSettings.KeepRecentTokens = 1
	var copied string
	exportJSONL := filepath.Join(t.TempDir(), "session.jsonl")
	terminal := gitui.NewVirtualTerminal(140, 40)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Messages: []string{
			"hello",
			"continue",
			"/compact focus previous turns",
			"/name Demo Session",
			"/session",
			"/copy",
			"/export " + exportJSONL,
			"!printf gi-bash",
			"!!printf hidden-bash",
		},
		ClipboardCopy: func(text string) error {
			copied = text
			return nil
		},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{
		"Session name set: Demo Session",
		"Session Info",
		"Name: Demo Session",
		"Messages",
		"Tokens",
		"$ printf gi-bash",
		"gi-bash",
		"$ printf hidden-bash",
		"hidden-bash",
		"Session exported to:",
	} {
		waitForViewport(t, terminal, expected)
	}
	waitForTerminalOutput(t, terminal, "[compaction] Compacted")
	if !strings.Contains(copied, "Response to: continue") {
		t.Fatalf("copied = %q, want last assistant text", copied)
	}
	if _, err := os.Stat(exportJSONL); err != nil {
		t.Fatalf("jsonl export path missing: %v", err)
	}
	if !sessionHasRole(sessionHost.session, "bashExecution") {
		t.Fatalf("expected bashExecution message in session entries")
	}
}

func TestRenderInteractiveSessionInfoUsesPiStyleSections(t *testing.T) {
	tokens := 90
	percent := 45.5
	info := renderInteractiveSessionInfo(
		RPCSessionState{
			Model:         &llm.Model{Provider: "openai", ID: "gpt-4o-mini"},
			ThinkingLevel: "off",
			SteeringMode:  "one-at-a-time",
			FollowUpMode:  "all",
			SessionID:     "state-id",
			SessionName:   "Demo",
		},
		RPCSessionStats{
			SessionID:         "stats-id",
			SessionFile:       "/tmp/session.jsonl",
			UserMessages:      2,
			AssistantMessages: 3,
			ToolCalls:         4,
			ToolResults:       5,
			TotalMessages:     14,
			Tokens: RPCSessionTokens{
				Input:      1000,
				Output:     2000,
				CacheRead:  3000,
				CacheWrite: 4000,
				Total:      10000,
			},
			Cost: 0.1234,
			ContextUsage: &AgentContextUsage{
				Tokens:        &tokens,
				ContextWindow: 200,
				Percent:       &percent,
			},
		},
	)

	for _, expected := range []string{
		tuiThemeBold("Session Info"),
		tuiThemeDim("Name:") + " Demo",
		tuiThemeDim("File:") + " /tmp/session.jsonl",
		tuiThemeDim("ID:") + " stats-id",
		tuiThemeBold("Messages"),
		tuiThemeDim("User:") + " 2",
		tuiThemeDim("Assistant:") + " 3",
		tuiThemeDim("Tool Calls:") + " 4",
		tuiThemeDim("Tool Results:") + " 5",
		tuiThemeDim("Total:") + " 14",
		tuiThemeBold("Tokens"),
		tuiThemeDim("Input:") + " 1,000",
		tuiThemeDim("Output:") + " 2,000",
		tuiThemeDim("Cache Read:") + " 3,000",
		tuiThemeDim("Cache Write:") + " 4,000",
		tuiThemeDim("Total:") + " 10,000",
		tuiThemeBold("Cost"),
		tuiThemeDim("Total:") + " 0.1234",
	} {
		if !strings.Contains(info, expected) {
			t.Fatalf("session info missing %q:\n%s", expected, info)
		}
	}
}

func TestCLIInteractiveTUIHostEmitsProviderLifecycleHooks(t *testing.T) {
	tempDir := t.TempDir()
	registry := NewModelRegistry(NewInMemoryAuthStorage(nil), "")
	beforeCalled := false
	afterCalled := false
	var providerSawPayload any

	if err := registry.RegisterProvider("interactive-hook-provider", ProviderConfigInput{
		BaseURL: "https://example.invalid/v1",
		APIKey:  "test-key",
		API:     "test-interactive-provider-lifecycle-hooks",
		StreamSimple: func(model llm.Model, _ llm.Context, options llm.SimpleStreamOptions) (*llm.AssistantMessageEventStream, error) {
			if options.OnPayload == nil || options.OnResponseStatus == nil {
				t.Fatal("provider lifecycle hooks were not installed")
			}
			next, replace, err := options.OnPayload(map[string]any{"prompt": "interactive original"}, model)
			if err != nil {
				t.Fatal(err)
			}
			if !replace {
				t.Fatal("OnPayload replace = false, want true")
			}
			providerSawPayload = next
			if err := options.OnResponseStatus(203, map[string]string{"X-Interactive": "ok"}, model); err != nil {
				t.Fatal(err)
			}
			return llm.CompletedAssistantStream(llm.Message{
				Role:       llm.RoleAssistant,
				Content:    []llm.ContentPart{llm.Text("interactive hooked")},
				API:        model.API,
				Provider:   model.Provider,
				Model:      model.ID,
				StopReason: llm.StopReasonStop,
			}), nil
		},
		Models: []ProviderModelDefinition{{
			ID:            "hook-model",
			Name:          "Hook Model",
			ContextWindow: 1000,
			MaxTokens:     100,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		registry.UnregisterProvider("interactive-hook-provider")
	})

	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		NoSession: true,
		Model:     "interactive-hook-provider/hook-model",
	}, CLIOptions{
		CWD:           tempDir,
		AgentDir:      filepath.Join(tempDir, "agent"),
		ModelRegistry: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityLifecycleEvents)
	if err := runtime.LoadFactories([]ProtocolExtensionFactory{{Path: "interactive-provider-hooks.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
		if err := ctx.On(ProtocolEventBeforeProviderRequest, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			beforeCalled = true
			payload, ok := event.Payload.(map[string]any)
			if !ok || payload["prompt"] != "interactive original" {
				t.Fatalf("before_provider_request payload = %#v", event.Payload)
			}
			return ProtocolEventResult{
				Payload:    map[string]any{"prompt": "interactive mutated"},
				PayloadSet: true,
			}, nil
		}); err != nil {
			return err
		}
		return ctx.On(ProtocolEventAfterProviderResponse, func(event ProtocolSessionEvent) (ProtocolEventResult, error) {
			afterCalled = true
			if event.Status != 203 || event.Headers["X-Interactive"] != "ok" {
				t.Fatalf("after_provider_response = %d %#v", event.Status, event.Headers)
			}
			return ProtocolEventResult{}, nil
		})
	}}}); err != nil {
		t.Fatal(err)
	}
	runtimeHostBridge, err := NewAgentSessionRuntimeHost(sessionHost.session, runtime)
	if err != nil {
		t.Fatal(err)
	}
	runtimeHostBridge.SetRebindSession(func(session *AgentSession) error {
		sessionHost.session = session
		return nil
	})
	sessionHost.sessionRuntimeHost = runtimeHostBridge

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:        runtimeHost,
		Terminal:           terminal,
		Messages:           []string{"hello"},
		ExitAfterInitial:   true,
		ClearScreenOnStart: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !beforeCalled || !afterCalled {
		t.Fatalf("hooks called before=%v after=%v", beforeCalled, afterCalled)
	}
	payload, ok := providerSawPayload.(map[string]any)
	if !ok || payload["prompt"] != "interactive mutated" {
		t.Fatalf("provider payload = %#v", providerSawPayload)
	}
	waitForViewport(t, terminal, "interactive hooked")
}

func TestCLIInteractiveTUIHostHandlesImportCommand(t *testing.T) {
	sourceCWD := t.TempDir()
	sourceSessionDir := filepath.Join(t.TempDir(), "sessions")
	sourceManager, err := CreateSessionManager(sourceCWD, sourceSessionDir)
	if err != nil {
		t.Fatal(err)
	}
	sourceManager.AppendMessage(llm.Message{Role: llm.RoleUser, Content: []llm.ContentPart{llm.Text("Imported question")}})
	sourceManager.AppendMessage(llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("Imported answer")}})
	if err := sourceManager.rewriteFile(); err != nil {
		t.Fatal(err)
	}

	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	terminal := gitui.NewVirtualTerminal(120, 28)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Messages: []string{
			"/import " + sourceManager.GetSessionFile(),
			"/session",
		},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	waitForTerminalOutput(t, terminal, "Session imported from:")
	waitForTerminalOutput(t, terminal, "Imported answer")
	waitForViewport(t, terminal, "Total: 2")
	if got := sessionHost.session.SessionManager.GetSessionFile(); got != sourceManager.GetSessionFile() {
		t.Fatalf("imported session file = %q, want %q", got, sourceManager.GetSessionFile())
	}
}

func TestCLIInteractiveTUIHostResumeWithPathSubmitsAsPromptPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	resumePath := "session.jsonl"
	var prompts []string
	sessionHost.session.Responder = func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error) {
		prompts = append(prompts, prompt)
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("agent saw: " + prompt)}}, nil
	}
	terminal := gitui.NewVirtualTerminal(120, 28)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Messages: []string{
			"/resume " + resumePath,
		},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	expected := "/resume " + resumePath
	if !reflect.DeepEqual(prompts, []string{expected}) {
		t.Fatalf("prompts = %#v", prompts)
	}
	waitForViewport(t, terminal, "agent saw: "+expected)
}

func TestCLIInteractiveTUIHostResumeCommandOpensSessionSelectorPiStyle(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cwd := t.TempDir()
	agentDir := filepath.Join(t.TempDir(), "agent")
	sessionDir := filepath.Join(t.TempDir(), "sessions")
	targetManager, err := CreateSessionManager(cwd, sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	targetManager.AppendMessage(llm.Message{Role: llm.RoleUser, Content: []llm.ContentPart{llm.Text("Target question")}})
	targetManager.AppendMessage(llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("Target answer")}})
	if err := targetManager.rewriteFile(); err != nil {
		t.Fatal(err)
	}

	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		Offline:    true,
		Model:      "openai/gpt-4o-mini",
		SessionDir: sessionDir,
	}, CLIOptions{CWD: cwd, AgentDir: agentDir})
	if err != nil {
		t.Fatal(err)
	}
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	terminal := gitui.NewVirtualTerminal(120, 28)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForViewport(t, terminal, "Gi")

	terminal.SendInput("/")
	terminal.SendInput("resume")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Resume Session")
	terminal.SendInput("Target")
	waitForViewport(t, terminal, "Target question")
	terminal.SendInput(sessionSelectorCtrlR)
	waitForViewport(t, terminal, "Rename Session")
	terminal.SendInput("Renamed target")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Renamed target")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Session resumed from:")
	waitForViewport(t, terminal, "Target answer")

	if got := sessionHost.session.SessionManager.GetSessionFile(); got != targetManager.GetSessionFile() {
		t.Fatalf("resumed session file = %q, want %q", got, targetManager.GetSessionFile())
	}
	if got := sessionHost.session.SessionManager.GetSessionName(); got != "Renamed target" {
		t.Fatalf("resumed session name = %q, want Renamed target", got)
	}
	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostResumeCommandShowsEmptySelectorPiStyle(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cwd := t.TempDir()
	agentDir := filepath.Join(t.TempDir(), "agent")
	sessionDir := filepath.Join(t.TempDir(), "sessions")
	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		Offline:    true,
		Model:      "openai/gpt-4o-mini",
		SessionDir: sessionDir,
	}, CLIOptions{CWD: cwd, AgentDir: agentDir})
	if err != nil {
		t.Fatal(err)
	}
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/resume")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Resume Session (Current Folder)")
	waitForViewport(t, terminal, "No sessions in current folder. Press Tab to view all.")
	if viewport := strings.Join(terminal.GetViewport(), "\n"); strings.Contains(viewport, "No sessions to resume") {
		t.Fatalf("/resume should render Pi-style empty selector instead of status:\n%s", viewport)
	}
	terminal.SendInput("\x1b")
	waitForHostEditor(t, host)

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostHandlesCloneCommand(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	terminal := gitui.NewVirtualTerminal(120, 28)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Messages: []string{
			"seed",
			"/clone",
			"/session",
		},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	waitForViewport(t, terminal, "Cloned to new session")
	waitForViewport(t, terminal, "Total: 2")
	if count := len(sessionHost.session.Messages()); count != 2 {
		t.Fatalf("messages after /clone = %d, want 2", count)
	}
}

func TestCLIInteractiveTUIHostCloneEmptySessionShowsPiStyleEntryError(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 28)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("/clone")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Error: Entry ")
	waitForViewport(t, terminal, "not found")
	viewport := strings.Join(terminal.GetViewport(), "\n")
	if strings.Contains(viewport, "Nothing to clone yet") {
		t.Fatalf("empty /clone should match installed Pi entry error, got:\n%s", viewport)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostForkWithArgSubmitsAsPromptPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	var prompts []string
	sessionHost.session.Responder = func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error) {
		prompts = append(prompts, prompt)
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("agent saw: " + prompt)}}, nil
	}
	terminal := gitui.NewVirtualTerminal(120, 28)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Messages: []string{
			"/fork entry-id",
		},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(prompts, []string{"/fork entry-id"}) {
		t.Fatalf("prompts = %#v", prompts)
	}
	waitForViewport(t, terminal, "agent saw: /fork entry-id")
}

func TestCLIInteractiveTUIHostForkSlashUsesUserMessageSelectorPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	mustPrompt(t, sessionHost.session, "fork first")
	mustPrompt(t, sessionHost.session, "fork second")
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/fork")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Fork from Message")
	waitForViewport(t, terminal, "Select a user message")
	waitForViewport(t, terminal, "Message 2 of 2")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Forked to new session")

	if count := len(sessionHost.session.Messages()); count != 2 {
		t.Fatalf("messages after selector fork = %d, want 2", count)
	}
	if text := host.editor.GetExpandedText(); text != "fork second" {
		t.Fatalf("editor text after selector fork = %q, want latest user text", text)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostTreeSlashUsesTreeSelectorPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	mustPrompt(t, sessionHost.session, "tree first")
	mustPrompt(t, sessionHost.session, "tree second")
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/tree")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Session Tree")
	waitForViewport(t, terminal, "tree second")
	waitForViewport(t, terminal, "fold/branch")
	terminal.SendInput("k")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Summarize branch?")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Navigated to selected point")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostTreeSlashEscapeReturnsWithoutStatusPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/tree")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Session Tree")
	waitForViewport(t, terminal, "No entries found")
	terminal.SendInput("\x1b")
	waitForFocusedComponent(t, terminal, host, host.editor)
	if output := terminal.Output(); strings.Contains(output, "Tree switch cancelled") {
		t.Fatalf("tree selector cancel should return silently like Pi, got output:\n%s", output)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostTreeWithArgSubmitsAsPromptPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	var prompts []string
	sessionHost.session.Responder = func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error) {
		prompts = append(prompts, prompt)
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("agent saw: " + prompt)}}, nil
	}
	terminal := gitui.NewVirtualTerminal(120, 28)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Messages: []string{
			"/tree entry-id",
		},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(prompts, []string{"/tree entry-id"}) {
		t.Fatalf("prompts = %#v", prompts)
	}
	waitForViewport(t, terminal, "agent saw: /tree entry-id")
}

func TestCLIInteractiveTUIHostTreeSlashCurrentLeafNoopsPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	mustPrompt(t, sessionHost.session, "tree current")
	terminal := gitui.NewVirtualTerminal(120, 28)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/tree")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Session Tree")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Already at this point")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostTreeSlashPromptsForBranchSummaryPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	mustPrompt(t, sessionHost.session, "summary first")
	mustPrompt(t, sessionHost.session, "summary second")
	sessionHost.session.BranchSummarizer = func(entries []FileEntry, customInstructions string, abort <-chan struct{}) (string, error) {
		if customInstructions != "" {
			t.Fatalf("customInstructions = %q, want empty", customInstructions)
		}
		return "summary from tui", nil
	}
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/tree")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Session Tree")
	terminal.SendInput("k")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Summarize branch?")
	terminal.SendInput("\x1b[B")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Navigated to selected point")

	summaries := filterFileEntriesByType(sessionHost.session.SessionManager.GetEntries(), "branch_summary")
	if len(summaries) != 1 || summaries[0].Summary != "summary from tui" {
		t.Fatalf("branch summaries = %#v", summaries)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostTreeSlashUsesCustomBranchSummaryInstructionsPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	mustPrompt(t, sessionHost.session, "custom summary first")
	mustPrompt(t, sessionHost.session, "custom summary second")
	sessionHost.session.BranchSummarizer = func(entries []FileEntry, customInstructions string, abort <-chan struct{}) (string, error) {
		if customInstructions != "Focus risk" {
			t.Fatalf("customInstructions = %q, want Focus risk", customInstructions)
		}
		return "summary with custom prompt", nil
	}
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/tree")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Session Tree")
	terminal.SendInput("k")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Summarize branch?")
	terminal.SendInput("\x1b[B")
	terminal.SendInput("\x1b[B")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Custom summarization instructions")
	terminal.SendInput("Focus risk")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Navigated to selected point")

	summaries := filterFileEntriesByType(sessionHost.session.SessionManager.GetEntries(), "branch_summary")
	if len(summaries) != 1 || summaries[0].Summary != "summary with custom prompt" {
		t.Fatalf("branch summaries = %#v", summaries)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostTreeSlashEscAbortsBranchSummaryPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	mustPrompt(t, sessionHost.session, "abort summary first")
	mustPrompt(t, sessionHost.session, "abort summary second")
	started := make(chan struct{})
	var startedOnce sync.Once
	sessionHost.session.BranchSummarizer = func(entries []FileEntry, customInstructions string, abort <-chan struct{}) (string, error) {
		startedOnce.Do(func() { close(started) })
		<-abort
		return "", errAgentSessionBranchSummaryAborted
	}
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/tree")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Session Tree")
	terminal.SendInput("k")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Summarize branch?")
	terminal.SendInput("\x1b[B")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Summarizing branch... (Esc to cancel)")
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("branch summarizer did not start")
	}
	terminal.SendInput("\x1b")
	waitForViewport(t, terminal, "Branch summarization cancelled")

	if summaries := filterFileEntriesByType(sessionHost.session.SessionManager.GetEntries(), "branch_summary"); len(summaries) != 0 {
		t.Fatalf("branch summaries = %#v", summaries)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostTreeSlashRespectsSkipSummaryPromptSetting(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.settingsManager.SetBranchSummarySkipPrompt(true)
	mustPrompt(t, sessionHost.session, "skip first")
	mustPrompt(t, sessionHost.session, "skip second")
	sessionHost.session.BranchSummarizer = func(entries []FileEntry, customInstructions string, abort <-chan struct{}) (string, error) {
		t.Fatal("branch summarizer should not run when skipPrompt defaults to no summary")
		return "", nil
	}
	terminal := gitui.NewVirtualTerminal(120, 32)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	host.editor.SetText("/tree")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Session Tree")
	terminal.SendInput("k")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Navigated to selected point")

	if summaries := filterFileEntriesByType(sessionHost.session.SessionManager.GetEntries(), "branch_summary"); len(summaries) != 0 {
		t.Fatalf("branch summaries = %#v", summaries)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostHandlesNewAndQuitCommands(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	terminal := gitui.NewVirtualTerminal(120, 28)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
		Messages: []string{
			"seed",
			"/new",
			"/session",
			"/quit",
		},
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	waitForViewport(t, terminal, "New session started")
	waitForViewport(t, terminal, "Total: 0")
	if count := len(sessionHost.session.Messages()); count != 0 {
		t.Fatalf("messages after /new = %d, want 0", count)
	}
}

func TestCLIInteractiveTUIHostRefreshesDynamicViewTreeMounts(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	viewHost := NewViewTreeHost()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:  runtimeHost,
		Terminal:     terminal,
		ViewTreeHost: viewHost,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForHostEditor(t, host)
	if err := viewHost.Mount("runtime-widget", "widget.belowEditor", ViewTreeNode{Type: "text", Text: "Dynamic widget"}); err != nil {
		t.Fatal(err)
	}
	waitForViewport(t, terminal, "Dynamic widget")

	if err := viewHost.Patch("runtime-widget", []ViewTreePatchOperation{{
		Op:    "replace",
		Path:  "/text",
		Value: "Patched widget",
	}}); err != nil {
		t.Fatal(err)
	}
	waitForViewport(t, terminal, "Patched widget")

	if !viewHost.Unmount("runtime-widget") {
		t.Fatal("expected dynamic widget to unmount")
	}
	waitForSlotChildCount(t, host, "belowEditor", 0)

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostStartsPackageProcessViewTreeContributions(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "todo-widget",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_PROCESS_HELPER": "1"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForViewport(t, terminal, "Process Widget")
	waitForViewport(t, terminal, "Started from package process")
	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostLoadsPackageProcessDiscoveredResources(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	packageDir := t.TempDir()
	writeResourceSkill(t, filepath.Join(packageDir, "skills", "dynamic-skill", "SKILL.md"), "dynamic-skill", "Dynamic skill", "Use dynamic process resources.")
	writeResourceFile(t, filepath.Join(packageDir, "prompts", "dynamic.md"), "---\ndescription: Dynamic prompt\n---\nDynamic prompt content")
	writeJSON(t, filepath.Join(packageDir, "themes", "dynamic.json"), map[string]any{"name": "dynamic-theme"})
	cwd := sessionHost.session.SessionManager.GetCWD()
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "resources-extension",
		Path:         filepath.Join(packageDir, "gi.package.json") + "#resources-extension",
		PackageDir:   packageDir,
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{CapabilityResourcesDiscover},
		Env: map[string]string{
			"GI_EXTENSION_RESOURCES_DISCOVER_HELPER": "1",
			"GI_EXTENSION_RESOURCES_EXPECT_CWD":      cwd,
		},
		Metadata: ProtocolSourceInfo{Source: "local:" + packageDir, Scope: "project", Origin: "package"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForCondition(t, func() bool {
		return resourceFindSkill(sessionHost.session.ResourceLoader.GetSkills().Skills, "dynamic-skill") != nil
	}, "process-discovered skill to load")
	if resourceFindPrompt(sessionHost.session.ResourceLoader.(AgentSessionPromptResourceLoader).GetPrompts().Prompts, "dynamic") == nil {
		t.Fatalf("prompts = %#v", sessionHost.session.ResourceLoader.(AgentSessionPromptResourceLoader).GetPrompts().Prompts)
	}
	if resourceFindTheme(sessionHost.session.ResourceLoader.(interface{ GetThemes() ResourceThemesResult }).GetThemes().Themes, "dynamic-theme") == nil {
		t.Fatalf("themes = %#v", sessionHost.session.ResourceLoader.(interface{ GetThemes() ResourceThemesResult }).GetThemes().Themes)
	}
	if !strings.Contains(sessionHost.session.SystemPrompt, "dynamic-skill") {
		t.Fatalf("system prompt did not refresh with process skill:\n%s", sessionHost.session.SystemPrompt)
	}
	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostStartsPackageProcessResourcesWithReloadReason(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	packageDir := t.TempDir()
	writeResourceSkill(t, filepath.Join(packageDir, "skills", "dynamic-skill", "SKILL.md"), "dynamic-skill", "Dynamic skill", "Use dynamic process resources.")
	writeResourceFile(t, filepath.Join(packageDir, "prompts", "dynamic.md"), "---\ndescription: Dynamic prompt\n---\nDynamic prompt content")
	writeJSON(t, filepath.Join(packageDir, "themes", "dynamic.json"), map[string]any{"name": "dynamic-theme"})
	cwd := sessionHost.session.SessionManager.GetCWD()
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "resources-extension",
		Path:         filepath.Join(packageDir, "gi.package.json") + "#resources-extension",
		PackageDir:   packageDir,
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{CapabilityResourcesDiscover},
		Env: map[string]string{
			"GI_EXTENSION_RESOURCES_DISCOVER_HELPER": "1",
			"GI_EXTENSION_RESOURCES_EXPECT_CWD":      cwd,
			"GI_EXTENSION_RESOURCES_EXPECT_REASON":   "reload",
		},
		Metadata: ProtocolSourceInfo{Source: "local:" + packageDir, Scope: "project", Origin: "package"},
	}}

	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    gitui.NewVirtualTerminal(100, 20),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.startProtocolExtensionProcesses(context.Background(), "reload"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.stopProtocolExtensionProcesses)

	waitForCondition(t, func() bool {
		return resourceFindSkill(sessionHost.session.ResourceLoader.GetSkills().Skills, "dynamic-skill") != nil
	}, "reload process-discovered skill to load")
}

func TestCLIInteractiveTUIHostKeepsGoodPackageResourcesWhenOneProcessFails(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	packageDir := t.TempDir()
	badPackageDir := t.TempDir()
	writeResourceSkill(t, filepath.Join(packageDir, "skills", "dynamic-skill", "SKILL.md"), "dynamic-skill", "Dynamic skill", "Use dynamic process resources.")
	writeResourceFile(t, filepath.Join(packageDir, "prompts", "dynamic.md"), "---\ndescription: Dynamic prompt\n---\nDynamic prompt content")
	writeJSON(t, filepath.Join(packageDir, "themes", "dynamic.json"), map[string]any{"name": "dynamic-theme"})
	cwd := sessionHost.session.SessionManager.GetCWD()
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{
		{
			ID:           "resources-extension",
			Path:         filepath.Join(packageDir, "gi.package.json") + "#resources-extension",
			PackageDir:   packageDir,
			Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
			Transport:    "stdio-ndjson",
			Protocol:     "gi-ext-rpc@1",
			Capabilities: []string{CapabilityResourcesDiscover},
			Env: map[string]string{
				"GI_EXTENSION_RESOURCES_DISCOVER_HELPER": "1",
				"GI_EXTENSION_RESOURCES_EXPECT_CWD":      cwd,
			},
			Metadata: ProtocolSourceInfo{Source: "local:" + packageDir, Scope: "project", Origin: "package"},
		},
		{
			ID:           "bad-resources-extension",
			Path:         filepath.Join(badPackageDir, "gi.package.json") + "#bad-resources-extension",
			PackageDir:   badPackageDir,
			Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
			Transport:    "stdio-ndjson",
			Protocol:     "gi-ext-rpc@1",
			Capabilities: []string{CapabilityResourcesDiscover},
			Env:          map[string]string{"GI_EXTENSION_BAD_RESOURCES_DISCOVER_HELPER": "1"},
			Metadata:     ProtocolSourceInfo{Source: "local:" + badPackageDir, Scope: "project", Origin: "package"},
		},
	}

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.startProtocolExtensionProcesses(context.Background(), "startup"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.stopProtocolExtensionProcesses)

	waitForCondition(t, func() bool {
		return resourceFindSkill(sessionHost.session.ResourceLoader.GetSkills().Skills, "dynamic-skill") != nil
	}, "good process-discovered skill to load despite bad resources_discover response")
}

func TestCLIInteractiveTUIHostDiscoversPackageResourcesAfterSessionSwitch(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	packageDir := t.TempDir()
	for _, reason := range []string{"startup", "new"} {
		name := "dynamic-" + reason
		writeResourceSkill(t, filepath.Join(packageDir, "skills", name, "SKILL.md"), name, "Dynamic "+reason+" skill", "Use dynamic "+reason+" resources.")
		writeResourceFile(t, filepath.Join(packageDir, "prompts", name+".md"), "---\ndescription: Dynamic "+reason+" prompt\n---\nDynamic "+reason+" prompt content")
		writeJSON(t, filepath.Join(packageDir, "themes", name+".json"), map[string]any{"name": name + "-theme"})
	}
	cwd := sessionHost.session.SessionManager.GetCWD()
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "resources-extension",
		Path:         filepath.Join(packageDir, "gi.package.json") + "#resources-extension",
		PackageDir:   packageDir,
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{CapabilityResourcesDiscover},
		Env: map[string]string{
			"GI_EXTENSION_RESOURCES_DISCOVER_HELPER": "1",
			"GI_EXTENSION_RESOURCES_BY_REASON":       "1",
			"GI_EXTENSION_RESOURCES_EXPECT_CWD":      cwd,
			"GI_EXTENSION_RESOURCES_EXPECT_REASONS":  "startup,new",
		},
		Metadata: ProtocolSourceInfo{Source: "local:" + packageDir, Scope: "project", Origin: "package"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForCondition(t, func() bool {
		return resourceFindSkill(sessionHost.session.ResourceLoader.GetSkills().Skills, "dynamic-startup") != nil
	}, "startup process-discovered skill to load")

	host.editor.SetText("/new")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "New session started")
	waitForCondition(t, func() bool {
		return resourceFindSkill(sessionHost.session.ResourceLoader.GetSkills().Skills, "dynamic-new") != nil
	}, "session-switch process-discovered skill to load")
	if !strings.Contains(sessionHost.session.SystemPrompt, "dynamic-new") {
		t.Fatalf("system prompt did not refresh with session-switch process skill:\n%s", sessionHost.session.SystemPrompt)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostStopsUIBeforePackageShutdownEventsPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "shutdown.marker")
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "shutdown-notify",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.dialog"},
		Env: map[string]string{
			"GI_EXTENSION_SHUTDOWN_NOTIFY_HELPER": "1",
			"GI_EXTENSION_SHUTDOWN_MARKER":        marker,
		},
	}}

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForProcessSupervisor(t, host)

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("shutdown helper did not observe session_shutdown: %v", err)
	}
	if output := terminal.Output(); strings.Contains(output, "Shutdown cleanup notification") {
		t.Fatalf("package shutdown event repainted the stopped TUI:\n%s", output)
	}
}

func TestCLIInteractiveTUIHostStartsPackageProcessHeaderContribution(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "header-widget",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.header"},
		Env:          map[string]string{"GI_EXTENSION_HEADER_HELPER": "1"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForViewport(t, terminal, "Process Header")
	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostStartsPackageProcessFooterContribution(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "footer-widget",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.footer"},
		Env:          map[string]string{"GI_EXTENSION_FOOTER_HELPER": "1"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForViewport(t, terminal, "Process Footer")
	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostStartsPackageProcessOverlayContribution(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "overlay-widget",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.overlay"},
		Env:          map[string]string{"GI_EXTENSION_OVERLAY_HELPER": "1"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForViewport(t, terminal, "Process Overlay")
	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRoutesPackageProcessViewTreeInputEvents(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "event-editor",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.editor"},
		Env:          map[string]string{"GI_EXTENSION_VIEWTREE_EVENT_HELPER": "1"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForViewport(t, terminal, "Process event editor")
	terminal.SendInput("x")
	waitForViewport(t, terminal, "Process key: x")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostEmitsViewTreeTicksToPackageProcess(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "tick-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{CapabilityTUIWidget},
		Env:          map[string]string{"GI_EXTENSION_TICK_HELPER": "1"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForViewport(t, terminal, "Tick 0")
	waitForViewport(t, terminal, "Ticked")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostEmitsViewTreeResizeToPackageProcess(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "resize-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{CapabilityTUIWidget},
		Env:          map[string]string{"GI_EXTENSION_RESIZE_HELPER": "1"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForViewport(t, terminal, "Size")
	terminal.Resize(82, 18)
	waitForViewport(t, terminal, "Size 82x18")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostEmitsTerminalInputToPackageProcess(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "terminal-input-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.terminal_input", "tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_TERMINAL_INPUT_HELPER": "1"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForHostEditor(t, host)
	waitForProcessSupervisor(t, host)
	terminal.SendInput("z")
	waitForViewport(t, terminal, "Terminal input: z")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostUsesPackageProcessEditorHostAction(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "editor-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.editor"},
		Env:          map[string]string{"GI_EXTENSION_EDITOR_HELPER": "1"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForViewport(t, terminal, "Package plan pasted")
	waitForViewport(t, terminal, "Response to: Package plan pasted")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostUsesPackageProcessDialogHostAction(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "dialog-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.dialog", "tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_DIALOG_HELPER": "1"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForViewport(t, terminal, "Pick from process")
	waitForViewport(t, terminal, "Alpha")
	terminal.SendInput("\x1b[B")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Dialog selected: beta")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRefreshesPackageProcessSlashCommands(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "plan-mode",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"commands.register", "tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_COMMAND_HELPER": "1"},
		Metadata:     ProtocolSourceInfo{Path: "gi.package.json#plan-mode", Source: "local:test", Scope: "temporary", Origin: "package"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForProtocolCommand(t, sessionHost.session.ExtensionRuntime, "plan")

	changed := make(chan struct{}, 4)
	host.editor.OnAutocompleteChange = func() {
		host.requestRender(false)
		changed <- struct{}{}
	}
	terminal.SendInput("/pl")
	waitForHostAutocompleteChange(t, changed)
	terminal.WaitForRender()
	if viewport := strings.Join(terminal.GetViewport(), "\n"); !strings.Contains(viewport, "plan") || !strings.Contains(viewport, "Enter plan mode") {
		t.Fatalf("process slash command missing from autocomplete:\n%s", viewport)
	}

	host.editor.SetText("/plan ship it")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Command invoked: ship it")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostUsesPackageProcessAutocompleteProvider(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "issue-autocomplete",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.autocomplete"},
		Env:          map[string]string{"GI_EXTENSION_AUTOCOMPLETE_HELPER": "1"},
		Metadata:     ProtocolSourceInfo{Path: "gi.package.json#issue-autocomplete", Source: "local:test", Scope: "temporary", Origin: "package"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForAutocompleteProvider(t, sessionHost.session.ExtensionRuntime, "issues")

	host.editor.SetText("Fix #12")
	terminal.SendInput("\t")
	waitForEditorText(t, host, "Fix #123")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostUsesPackageProcessShortcut(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "shortcut-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{CapabilityShortcutsRegister, "tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_SHORTCUT_HELPER": "1"},
		Metadata:     ProtocolSourceInfo{Path: "gi.package.json#shortcut-extension", Source: "local:test", Scope: "temporary", Origin: "package"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForProtocolShortcut(t, sessionHost.session.ExtensionRuntime, "ctrl+y")

	terminal.SendInput("\x19")
	waitForViewport(t, terminal, "Shortcut invoked: ctrl+y")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostShowsShortcutHandlerErrorsPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	mustLoadProtocolFactories(t, sessionHost.session.ExtensionRuntime, ProtocolExtensionFactory{Path: "shortcut-error.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.RegisterShortcut("ctrl+y", ProtocolShortcutDefinition{
			Description: "Broken shortcut",
			Handler: func() error {
				return errors.New("boom")
			},
		})
	}})

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	terminal.SendInput("\x19")
	waitForViewport(t, terminal, "Error: Shortcut handler error: boom")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostFansOutPackageProcessSessionStart(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "lifecycle-widget",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_LIFECYCLE_HELPER": "1"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForViewport(t, terminal, "Lifecycle: startup")
	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRebindsPackageProcessesOnSessionSwitch(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "lifecycle-widget",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_LIFECYCLE_HELPER": "1"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForHostEditor(t, host)
	waitForProcessSupervisor(t, host)
	waitForViewport(t, terminal, "Lifecycle: startup")
	previousSession := sessionHost.session

	host.editor.SetText("/new")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Lifecycle switch: new")
	waitForViewport(t, terminal, "Lifecycle: new")
	if sessionHost.session == previousSession {
		t.Fatal("session host did not switch to a new session")
	}
	if host.processSupervisor == nil || host.processSupervisor.Host == nil || host.processSupervisor.Host.Session != sessionHost.session {
		t.Fatalf("process supervisor session = %#v, want %#v", host.processSupervisor, sessionHost.session)
	}

	host.editor.SetText("after switch")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "after switch")
	waitForViewport(t, terminal, "Response to: after switch")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostSessionSwitchContinuesAfterPackageProcessExit(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "tui-state-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.title", "tui.working", "tui.thinking_label"},
		Env:          map[string]string{"GI_EXTENSION_TUI_STATE_EXIT_HELPER": "1"},
	}}

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForHostEditor(t, host)
	waitForProcessSupervisor(t, host)
	waitForCondition(t, func() bool {
		host.mu.Lock()
		supervisor := host.processSupervisor
		host.mu.Unlock()
		if supervisor == nil || len(supervisor.processes) == 0 {
			return false
		}
		select {
		case <-supervisor.processes[0].done:
			return true
		default:
			return false
		}
	}, "package process to exit before session switch")
	previousSession := sessionHost.session

	host.editor.SetText("/new")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "New session started")
	if sessionHost.session == previousSession {
		t.Fatal("session host did not switch after package process exit")
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostUsesRegisteredMessageRenderers(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityTUIMessageRenderer)
	runtime.BindSession(sessionHost.session)
	ctx := &ProtocolExtensionContext{runtime: runtime, source: ProtocolSourceInfo{Path: "renderer.gi.json", Source: "local:test", Scope: "temporary", Origin: "package"}}
	if err := ctx.RegisterMessageRenderer("custom.status", func(message any, options any) []string {
		rendered, _ := message.(llm.Message)
		return []string{"Custom rendered: " + interactiveTextFromLLMMessage(rendered)}
	}); err != nil {
		t.Fatal(err)
	}
	sessionHost.session.SessionManager.AppendCustomMessageEntryWithContext("custom.status", "ready", true, nil, false)

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:      runtimeHost,
		Terminal:         terminal,
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	waitForViewport(t, terminal, "Custom rendered: ready")
}

func TestCLIInteractiveTUIHostPassesExpandedStateToCustomMessageRenderersPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityTUIMessageRenderer)
	runtime.BindSession(sessionHost.session)
	ctx := &ProtocolExtensionContext{runtime: runtime, source: ProtocolSourceInfo{Path: "renderer.gi.json", Source: "local:test", Scope: "temporary", Origin: "package"}}
	if err := ctx.RegisterMessageRenderer("custom.status", func(_ any, options any) []string {
		optionMap, _ := options.(map[string]any)
		return []string{fmt.Sprintf("Custom expanded: %v", optionMap["expanded"])}
	}); err != nil {
		t.Fatal(err)
	}
	sessionHost.session.SessionManager.AppendCustomMessageEntryWithContext("custom.status", "ready", true, nil, false)

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForViewport(t, terminal, "Custom expanded: false")

	if err := host.SetTUIToolsExpanded(true); err != nil {
		t.Fatal(err)
	}
	if rendered := strings.Join(host.chat.Render(100), "\n"); !strings.Contains(rendered, "Custom expanded: true") {
		t.Fatalf("custom renderer did not receive expanded=true:\n%s", rendered)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostUsesPackageProcessMessageRenderer(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "message-renderer",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.message_renderer"},
		Env:          map[string]string{"GI_EXTENSION_MESSAGE_RENDERER_HELPER": "1"},
		Metadata:     ProtocolSourceInfo{Path: "gi.package.json#message-renderer", Source: "local:test", Scope: "temporary", Origin: "package"},
	}}
	sessionHost.session.SessionManager.AppendCustomMessageEntryWithContext("rpc.message", "package says hi", true, nil, false)

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForViewport(t, terminal, "Message render: package says hi")
	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostCustomMessageFallbackAndDisplayFlag(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.session.SessionManager.AppendCustomMessageEntryWithContext("custom.hidden", "secret", false, nil, false)
	sessionHost.session.SessionManager.AppendCustomMessageEntryWithContext("custom.visible", "shown", true, nil, false)

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:      runtimeHost,
		Terminal:         terminal,
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	waitForViewport(t, terminal, "[custom.visible] shown")
	terminal.WaitForRender()
	viewport := strings.Join(terminal.GetViewport(), "\n")
	if strings.Contains(viewport, "secret") || strings.Contains(viewport, "custom.hidden") {
		t.Fatalf("hidden custom message rendered:\n%s", viewport)
	}
}

func TestCLIInteractiveTUIHostUsesRegisteredToolRenderers(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityToolsRegister, CapabilityTUIToolRenderer)
	runtime.BindSession(sessionHost.session)
	ctx := &ProtocolExtensionContext{runtime: runtime, source: ProtocolSourceInfo{Path: "tool-renderer.gi.json", Source: "local:test", Scope: "temporary", Origin: "package"}}
	if err := ctx.RegisterTool(ProtocolToolDefinition{
		Name:          "rendered_tool",
		Label:         "Rendered Tool",
		Description:   "Rendered test tool",
		PromptSnippet: "Rendered tool",
		Execute: func(toolCallID string, input map[string]any) (SDKToolResult, error) {
			return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: "rendered result"}}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ctx.RegisterToolRenderer("rendered_tool", ProtocolToolRendererDefinition{
		RenderCall: func(args any, context ToolRenderContext) []string {
			values, _ := args.(map[string]any)
			foo, _ := values["foo"].(string)
			return []string{"TUI call: " + foo}
		},
		RenderResult: func(result FileToolResult, options ToolRenderResultOptions, context ToolRenderContext) []string {
			return []string{"TUI result: " + fileToolResultText(result)}
		},
	}); err != nil {
		t.Fatal(err)
	}
	var calls int
	sessionHost.session.Responder = func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error) {
		calls++
		if calls == 1 {
			return llm.Message{
				Role:       llm.RoleAssistant,
				StopReason: "toolUse",
				Content:    []llm.ContentPart{llm.ToolCall("tool-call-1", "rendered_tool", map[string]any{"foo": "bar"})},
			}, nil
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("final")}}, nil
	}

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:      runtimeHost,
		InitialMessage:   "use tool",
		Terminal:         terminal,
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	waitForViewport(t, terminal, "TUI call: bar")
	waitForViewport(t, terminal, "TUI result: rendered result")
	waitForViewport(t, terminal, "final")
}

func TestCLIInteractiveTUIHostUsesPackageProcessToolRenderer(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:           "tool-renderer-e2e",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tools.register", "tui.tool_renderer"},
		Env:          map[string]string{"GI_EXTENSION_TOOL_RENDERER_E2E_HELPER": "1"},
		Metadata:     ProtocolSourceInfo{Path: "gi.package.json#tool-renderer-e2e", Source: "local:test", Scope: "temporary", Origin: "package"},
	}}
	var calls int
	sessionHost.session.Responder = func(prompt string, context []llm.Message, model llm.Model) (llm.Message, error) {
		calls++
		if calls == 1 {
			return llm.Message{
				Role:       llm.RoleAssistant,
				StopReason: "toolUse",
				Content:    []llm.ContentPart{llm.ToolCall("tool-call-1", "rendered_tool", map[string]any{"foo": "bar"})},
			}, nil
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("final")}}, nil
	}

	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)
	waitForProtocolTool(t, sessionHost.session.ExtensionRuntime, "rendered_tool")
	waitForToolRenderer(t, sessionHost.session.ExtensionRuntime, "rendered_tool")

	host.editor.SetText("use process tool")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Tool call render: bar")
	waitForViewport(t, terminal, "Tool result render: process rendered result: bar")
	waitForViewport(t, terminal, "final")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRunsIntegratedPackageProcessWorkflow(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sessionHost.processExtensions = []ProtocolPackageProcessExtension{{
		ID:         "integrated-package",
		PackageDir: t.TempDir(),
		Command:    []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:  "stdio-ndjson",
		Protocol:   "gi-ext-rpc@1",
		Capabilities: []string{
			CapabilityCommandsRegister,
			CapabilityShortcutsRegister,
			CapabilityToolsRegister,
			CapabilityTUIAutocomplete,
			CapabilityTUIMessageRenderer,
			CapabilityTUIToolRenderer,
			CapabilityTUIWidget,
			CapabilityTUIEditor,
			"tui.dialog",
		},
		Env:      map[string]string{"GI_EXTENSION_INTEGRATED_WORKFLOW_HELPER": "1"},
		Metadata: ProtocolSourceInfo{Path: "gi.package.json#integrated-package", Source: "local:test", Scope: "temporary", Origin: "package"},
	}}

	terminal := gitui.NewVirtualTerminal(120, 36)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		host.Stop()
	})
	waitForHostEditor(t, host)
	integratedCommand := waitForProtocolCommand(t, sessionHost.session.ExtensionRuntime, "integrate")
	if integratedCommand.Handler == nil {
		t.Fatal("integrated command handler was not registered")
	}
	waitForProtocolShortcut(t, sessionHost.session.ExtensionRuntime, "ctrl+y")
	waitForAutocompleteProvider(t, sessionHost.session.ExtensionRuntime, "integrated-issues")
	waitForMessageRenderer(t, sessionHost.session.ExtensionRuntime, "rpc.integrated")
	integratedTool := waitForProtocolTool(t, sessionHost.session.ExtensionRuntime, "integrated_tool")
	integratedToolRenderer := waitForToolRenderer(t, sessionHost.session.ExtensionRuntime, "integrated_tool")

	waitForViewport(t, terminal, "Integrated lifecycle: startup")
	waitForViewport(t, terminal, "Integrated choice")
	terminal.SendInput("\x1b[B")
	terminal.SendInput("\r")
	waitForViewport(t, terminal, "Integrated dialog: beta")
	waitForEditorText(t, host, "Integrated draft ready")

	sessionHost.session.SessionManager.AppendCustomMessageEntryWithContext("rpc.integrated", "package transcript", true, nil, false)
	host.rerenderSessionMessages()
	waitForViewport(t, terminal, "Integrated message: package transcript")

	host.editor.SetText("Fix #90")
	terminal.SendInput("\t")
	waitForEditorText(t, host, "Fix #900")

	toolResult, err := integratedTool.Execute("integrated-call-1", map[string]any{"foo": "bar"})
	if err != nil {
		t.Fatal(err)
	}
	if len(toolResult.Content) != 1 || toolResult.Content[0].Text != "integrated tool result: bar" {
		t.Fatalf("integrated tool result = %#v", toolResult)
	}
	if lines := integratedToolRenderer.RenderCall(map[string]any{"foo": "bar"}, ToolRenderContext{ToolCallID: "integrated-call-1"}); !containsString(lines, "Integrated tool call: bar") {
		t.Fatalf("integrated tool call render = %#v", lines)
	}
	if lines := integratedToolRenderer.RenderResult(FileToolResult{Text: "integrated tool result: bar"}, ToolRenderResultOptions{}, ToolRenderContext{ToolCallID: "integrated-call-1"}); !containsString(lines, "Integrated tool result render: integrated tool result: bar") {
		t.Fatalf("integrated tool result render = %#v", lines)
	}

	cancel()
	host.Stop()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("RunContext returned %v", err)
		}
	default:
	}
}

func TestCLIInteractiveTUIHostRendersAbortedAssistantToolCallsAsErrors(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.session.SessionManager.AppendMessage(sessionMessageValue(llm.Message{
		Role:       llm.RoleAssistant,
		StopReason: "aborted",
		Content:    []llm.ContentPart{llm.ToolCall("aborted-call", "rendered_tool", map[string]any{"foo": "bar"})},
	}))

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:      runtimeHost,
		Terminal:         terminal,
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	waitForViewport(t, terminal, "Operation aborted")
	if len(host.pendingTools) != 0 {
		t.Fatalf("pending tools = %#v", host.pendingTools)
	}
}

func TestCLIInteractiveTUIHostRendersAssistantStopErrorsWithoutToolCallsPiStyle(t *testing.T) {
	t.Run("aborted with default message", func(t *testing.T) {
		text := interactiveAssistantTextFromLLMMessage(llm.Message{
			Role:       llm.RoleAssistant,
			StopReason: llm.StopReasonAborted,
		}, false, "")
		if text != "Operation aborted" {
			t.Fatalf("aborted text = %q", text)
		}
	})

	t.Run("error appends after visible content", func(t *testing.T) {
		text := interactiveAssistantTextFromLLMMessage(llm.Message{
			Role:         llm.RoleAssistant,
			StopReason:   llm.StopReasonError,
			ErrorMessage: "provider failed",
			Content:      []llm.ContentPart{llm.Text("partial answer")},
		}, false, "")
		if text != "partial answer\n\nError: provider failed" {
			t.Fatalf("error text = %q", text)
		}
	})

	t.Run("tool call messages leave status to tool components", func(t *testing.T) {
		text := interactiveAssistantTextFromLLMMessage(llm.Message{
			Role:       llm.RoleAssistant,
			StopReason: llm.StopReasonError,
			Content:    []llm.ContentPart{llm.ToolCall("call-1", "read", nil)},
		}, false, "")
		if text != "" {
			t.Fatalf("tool-call error text = %q", text)
		}
	})
}

func TestCLIInteractiveTUIHostReplaysAssistantStopErrorsWithoutToolCallsPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	sessionHost.session.SessionManager.AppendMessage(sessionMessageValue(llm.Message{
		Role:         llm.RoleAssistant,
		StopReason:   llm.StopReasonError,
		ErrorMessage: "provider failed",
		Content:      []llm.ContentPart{llm.Text("partial answer")},
	}))

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:      runtimeHost,
		Terminal:         terminal,
		ExitAfterInitial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RunContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	waitForViewport(t, terminal, "partial answer")
	waitForViewport(t, terminal, "Error: provider failed")
}

func TestCLIInteractiveTUIHostReflectsViewTreeOverlayMounts(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	viewHost := NewViewTreeHost()
	var events []ViewTreeEvent
	unsubscribe := viewHost.OnEvent(func(event ViewTreeEvent) {
		events = append(events, event)
	})
	defer unsubscribe()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:  runtimeHost,
		Terminal:     terminal,
		ViewTreeHost: viewHost,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForHostEditor(t, host)
	if err := viewHost.Mount("overlay-panel", "overlay", ViewTreeNode{Type: "box", ID: "root", Children: []ViewTreeNode{{Type: "text", Text: "Package overlay"}}}); err != nil {
		t.Fatal(err)
	}
	waitForViewport(t, terminal, "Package overlay")
	terminal.SendInput("x")
	waitForViewTreeEvent(t, &events, "overlay-panel", "root", "key")

	if !viewHost.Unmount("overlay-panel") {
		t.Fatal("expected overlay to unmount")
	}
	waitForNoOverlay(t, host)

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostReflectsViewTreeOverlayOptions(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	viewHost := NewViewTreeHost()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:  runtimeHost,
		Terminal:     terminal,
		ViewTreeHost: viewHost,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForHostEditor(t, host)
	if err := viewHost.MountWithOptions("anchored-overlay", "overlay", ViewTreeNode{Type: "text", ID: "root", Text: "Anchored overlay"}, ViewTreeMountOptions{Overlay: &ViewTreeOverlayOptions{
		Width:        ViewTreeSizeValue{Set: true, Value: 30},
		MinWidth:     10,
		Anchor:       "top-left",
		NonCapturing: true,
	}}); err != nil {
		t.Fatal(err)
	}
	waitForViewport(t, terminal, "Anchored overlay")
	terminal.WaitForRender()
	if firstLine := terminal.GetViewport()[0]; !strings.Contains(firstLine, "Anchored overlay") {
		t.Fatalf("top-left overlay first line = %q", firstLine)
	}
	if host.ui.FocusedComponent() != host.editor {
		t.Fatalf("non-capturing overlay changed focus")
	}
	terminal.SendInput("z")
	if got := host.editor.GetText(); got != "z" {
		t.Fatalf("editor text after non-capturing overlay input = %q", got)
	}

	if !viewHost.Unmount("anchored-overlay") {
		t.Fatal("expected overlay to unmount")
	}
	waitForNoOverlay(t, host)

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostReflectsViewTreeEditorSlotMounts(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	viewHost := NewViewTreeHost()
	var events []ViewTreeEvent
	unsubscribe := viewHost.OnEvent(func(event ViewTreeEvent) {
		events = append(events, event)
	})
	defer unsubscribe()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:  runtimeHost,
		Terminal:     terminal,
		ViewTreeHost: viewHost,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })

	waitForHostEditor(t, host)
	host.editor.SetText("default draft")
	if err := viewHost.Mount("custom-editor", "editor", ViewTreeNode{Type: "textarea", ID: "editor-root", Text: "Package editor"}); err != nil {
		t.Fatal(err)
	}
	waitForViewport(t, terminal, "Package editor")
	waitForEditorContainerChildCount(t, host, 1)
	if !host.customEditorActive || host.ui.FocusedComponent() == host.editor {
		t.Fatalf("custom editor not active or default editor still focused")
	}
	terminal.SendInput("x")
	waitForViewTreeEvent(t, &events, "custom-editor", "editor-root", "key")
	host.ui.SetFocus(nil)
	rpcHost := &RPCSessionHost{TUIEditor: host}
	response := rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "editor_focus",
		Method:   "host.tui.editor",
		Params:   []byte(`{"action":"focus"}`),
	})
	result := assertHostActionResponseOK(t, response, "editor_focus")
	if result["focused"] != true || result["customEditorActive"] != true {
		t.Fatalf("focus result = %#v", result)
	}
	response = rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "editor_cursor_custom",
		Method:   "host.tui.editor",
		Params:   []byte(`{"action":"cursor"}`),
	})
	result = assertHostActionResponseOK(t, response, "editor_cursor_custom")
	if result["cursor"] != nil || result["customEditorActive"] != true {
		t.Fatalf("custom editor cursor result = %#v", result)
	}

	if !viewHost.Unmount("custom-editor") {
		t.Fatal("expected custom editor to unmount")
	}
	waitForEditorContainerChildCount(t, host, 1)
	if host.customEditorActive || host.ui.FocusedComponent() != host.editor {
		t.Fatalf("default editor was not restored")
	}
	if got := host.editor.GetText(); got != "default draft" {
		t.Fatalf("default editor text = %q", got)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostReflectsRPCStatusHostAction(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	viewHost := NewViewTreeHost()
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost:  runtimeHost,
		Terminal:     terminal,
		ViewTreeHost: viewHost,
		ShowFooter:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	rpcHost := &RPCSessionHost{ViewTreeHost: viewHost, TUIStatus: host}
	response := rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "status_1",
		Method:   "host.tui.status",
		Params:   []byte(`{"key":"plan-mode","text":"Plan mode","priority":50}`),
	})
	if response.Error != nil {
		t.Fatalf("host action error = %#v", response.Error)
	}
	waitForViewport(t, terminal, "Plan mode")
	if got := host.footerDataProvider.GetExtensionStatuses()["plan-mode"]; got != "Plan mode" {
		t.Fatalf("footer status = %q", got)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostReflectsRPCThemeHostAction(t *testing.T) {
	previousTheme := tuiActiveThemeSnapshot()
	t.Cleanup(func() { tuiSetActiveThemePalette(previousTheme) })
	cwd := t.TempDir()
	agentDir := filepath.Join(cwd, "agent")
	settings := NewSettingsManager(cwd, agentDir)
	settings.SetTheme("dark")
	themePath := filepath.Join(cwd, ConfigDirName, "themes", "focus.json")
	writeJSON(t, themePath, completeTUIThemeFixture("focus", map[string]any{
		"accent": "#112233",
	}))
	runtimeHost, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		NoSession: true,
		Model:     "openai/gpt-4o-mini",
	}, CLIOptions{CWD: cwd, AgentDir: agentDir})
	if err != nil {
		t.Fatal(err)
	}
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	rpcHost := &RPCSessionHost{TUITheme: host}
	response := rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "theme_list",
		Method:   "host.tui.theme",
		Params:   []byte(`{"action":"list"}`),
	})
	result := assertHostActionResponseOK(t, response, "theme_list")
	themes, ok := result["themes"].([]TUIThemeInfo)
	if !ok || !tuiThemeInfoHasName(themes, "dark") || !tuiThemeInfoHasName(themes, "light") || !tuiThemeInfoHasName(themes, "focus") {
		t.Fatalf("themes = %#v", result["themes"])
	}
	response = rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "theme_set",
		Method:   "host.tui.theme",
		Params:   []byte(`{"action":"set","name":"focus"}`),
	})
	result = assertHostActionResponseOK(t, response, "theme_set")
	settings.Reload()
	if result["success"] != true || settings.GetTheme() != "focus" || host.CurrentTUITheme() != "focus" {
		t.Fatalf("theme set result=%#v settings=%q current=%q", result, settings.GetTheme(), host.CurrentTUITheme())
	}
	response = rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "theme_missing",
		Method:   "host.tui.theme",
		Params:   []byte(`{"action":"set","name":"missing"}`),
	})
	result = assertHostActionResponseOK(t, response, "theme_missing")
	settings.Reload()
	if result["success"] != false || settings.GetTheme() != "focus" {
		t.Fatalf("missing theme result=%#v settings=%q", result, settings.GetTheme())
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostReflectsRPCToolExpansionHostAction(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	tool := NewToolExecutionComponent("read", "tool-1", map[string]any{"path": "README.md"}, ToolDefinition{Name: "read"}, t.TempDir())
	host.chat.AddChild(tool)
	rpcHost := &RPCSessionHost{TUIToolExpansion: host}
	response := rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "tools_expanded_set",
		Method:   "host.tui.tools_expanded",
		Params:   []byte(`{"expanded":true}`),
	})
	result := assertHostActionResponseOK(t, response, "tools_expanded_set")
	if result["expanded"] != true || !host.TUIToolsExpanded() || !tool.expanded {
		t.Fatalf("tools expanded result=%#v host=%v tool=%v", result, host.TUIToolsExpanded(), tool.expanded)
	}
	response = rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "tools_expanded_get",
		Method:   "host.tui.tools_expanded",
		Params:   []byte(`{}`),
	})
	result = assertHostActionResponseOK(t, response, "tools_expanded_get")
	if result["expanded"] != true {
		t.Fatalf("tools expanded get = %#v", result)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostReflectsRPCEditorHostAction(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	rpcHost := &RPCSessionHost{TUIEditor: host}
	response := rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "editor_set",
		Method:   "host.tui.editor",
		Params:   []byte(`{"action":"set","text":"Plan"}`),
	})
	assertHostActionResponseOK(t, response, "editor_set")
	response = rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "editor_insert",
		Method:   "host.tui.editor",
		Params:   []byte(`{"action":"insert","text":" mode"}`),
	})
	result := assertHostActionResponseOK(t, response, "editor_insert")
	if result["text"] != "Plan mode" {
		t.Fatalf("editor text = %#v", result["text"])
	}
	cursor, ok := result["cursor"].(map[string]int)
	if !ok || cursor["line"] != 0 || cursor["col"] != len("Plan mode") {
		t.Fatalf("editor cursor = %#v", result["cursor"])
	}
	if result["focused"] != true || result["customEditorActive"] != false {
		t.Fatalf("editor state = %#v", result)
	}
	response = rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "editor_cursor",
		Method:   "host.tui.editor",
		Params:   []byte(`{"action":"cursor"}`),
	})
	result = assertHostActionResponseOK(t, response, "editor_cursor")
	cursor, ok = result["cursor"].(map[string]int)
	if !ok || cursor["line"] != 0 || cursor["col"] != len("Plan mode") {
		t.Fatalf("editor cursor action result = %#v", result)
	}
	response = rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "editor_autocomplete_context",
		Method:   "host.tui.editor",
		Params:   []byte(`{"action":"autocomplete_context","force":true,"trigger":"/"}`),
	})
	result = assertHostActionResponseOK(t, response, "editor_autocomplete_context")
	lines, ok := result["lines"].([]string)
	if !ok || len(lines) != 1 || lines[0] != "Plan mode" || result["force"] != true || result["trigger"] != "/" {
		t.Fatalf("autocomplete context result = %#v", result)
	}
	response = rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "editor_clear",
		Method:   "host.tui.editor",
		Params:   []byte(`{"action":"set","text":""}`),
	})
	assertHostActionResponseOK(t, response, "editor_clear")
	largePaste := strings.Repeat("x", 1001)
	response = rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "editor_paste",
		Method:   "host.tui.editor",
		Params:   []byte(`{"action":"paste","text":"` + largePaste + `"}`),
	})
	result = assertHostActionResponseOK(t, response, "editor_paste")
	if result["text"] != largePaste || result["pasteSemantics"] != true {
		t.Fatalf("paste result = %#v", result)
	}
	if got := host.editor.GetText(); got != "[paste #1 1001 chars]" {
		t.Fatalf("paste marker = %q", got)
	}
	response = rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "editor_reset",
		Method:   "host.tui.editor",
		Params:   []byte(`{"action":"set","text":"Plan mode"}`),
	})
	assertHostActionResponseOK(t, response, "editor_reset")
	response = rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "editor_submit",
		Method:   "host.tui.editor",
		Params:   []byte(`{"action":"submit"}`),
	})
	assertHostActionResponseOK(t, response, "editor_submit")
	waitForViewport(t, terminal, "Response to: Plan mode")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostUsesProtocolAutocompleteProvider(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityTUIAutocomplete)
	requests := make(chan ProtocolAutocompleteRequest, 4)
	mustLoadProtocolFactories(t, runtime, protocolIssueAutocompleteFactory(requests))
	runtime.BindSession(sessionHost.session)

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	changed := make(chan struct{}, 4)
	host.editor.OnAutocompleteChange = func() { changed <- struct{}{} }
	host.editor.SetText("Fix #12")
	terminal.SendInput("\t")
	waitForHostAutocompleteChange(t, changed)
	request := waitForProtocolAutocompleteRequest(t, requests)
	if request.Text != "Fix #12" || request.CursorLine != 0 || request.CursorCol != len("Fix #12") || !request.Force {
		t.Fatalf("autocomplete request = %#v", request)
	}
	if !host.editor.IsShowingAutocomplete() {
		t.Fatalf("protocol autocomplete suggestions should be visible")
	}

	terminal.SendInput("\t")
	waitForEditorText(t, host, "Fix #123")
	if host.editor.IsShowingAutocomplete() {
		t.Fatalf("autocomplete should close after applying completion")
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostPassesSlashContextToProtocolAutocompleteProvider(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityTUIAutocomplete)
	requests := make(chan ProtocolAutocompleteRequest, 4)
	mustLoadProtocolFactories(t, runtime, protocolIssueAutocompleteFactory(requests))
	runtime.BindSession(sessionHost.session)

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	changed := make(chan struct{}, 4)
	host.editor.OnAutocompleteChange = func() { changed <- struct{}{} }
	terminal.SendInput("/deploy staging #")
	waitForHostAutocompleteChange(t, changed)
	request := waitForProtocolAutocompleteRequest(t, requests)
	if request.Text != "/deploy staging #" || request.SlashCommand != "deploy" || request.ArgumentIndex != 1 || request.Force {
		t.Fatalf("autocomplete request = %#v", request)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostRefreshesProtocolAutocompleteProvidersAfterStartup(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	sessionHost, ok := runtimeHost.(*agentSessionPrintModeHost)
	if !ok {
		t.Fatalf("runtime host = %T, want *agentSessionPrintModeHost", runtimeHost)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityTUIAutocomplete)
	runtime.BindSession(sessionHost.session)

	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	requests := make(chan ProtocolAutocompleteRequest, 4)
	mustLoadProtocolFactories(t, runtime, protocolIssueAutocompleteFactory(requests))
	changed := make(chan struct{}, 4)
	host.editor.OnAutocompleteChange = func() { changed <- struct{}{} }
	host.editor.SetText("Fix #12")
	terminal.SendInput("\t")
	waitForHostAutocompleteChange(t, changed)
	request := waitForProtocolAutocompleteRequest(t, requests)
	if request.Text != "Fix #12" || !request.Force {
		t.Fatalf("autocomplete request = %#v", request)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func protocolIssueAutocompleteFactory(requests chan<- ProtocolAutocompleteRequest) ProtocolExtensionFactory {
	return ProtocolExtensionFactory{Path: "autocomplete.gi.json", Factory: func(ctx *ProtocolExtensionContext) error {
		return ctx.RegisterAutocompleteProvider("issues", ProtocolAutocompleteProviderDefinition{
			Description: "Issue references",
			Priority:    50,
			Handler: func(_ context.Context, request ProtocolAutocompleteRequest) (ProtocolAutocompleteResult, error) {
				requests <- request
				if request.CursorLine < 0 || request.CursorLine >= len(request.Lines) {
					return ProtocolAutocompleteResult{}, nil
				}
				line := []rune(request.Lines[request.CursorLine])
				cursorCol := max(0, min(request.CursorCol, len(line)))
				prefixStart := cursorCol
				for prefixStart > 0 && line[prefixStart-1] != ' ' && line[prefixStart-1] != '\t' {
					prefixStart--
				}
				prefix := string(line[prefixStart:cursorCol])
				if !strings.HasPrefix(prefix, "#") {
					return ProtocolAutocompleteResult{}, nil
				}
				return ProtocolAutocompleteResult{
					Prefix: prefix,
					Start:  prefixStart,
					End:    cursorCol,
					Items: []ProtocolAutocompleteItem{
						{ID: "issue-123", Value: "#123", Label: "#123", Description: "Issue 123"},
						{ID: "issue-124", Value: "#124", Label: "#124", Description: "Issue 124"},
					},
				}, nil
			},
		})
	}}
}

func TestCLIInteractiveTUIHostReflectsRPCDialogNotifyHostAction(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	rpcHost := &RPCSessionHost{TUIDialog: host}
	response := rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "dialog_notify",
		Method:   "host.tui.dialog",
		Params:   []byte(`{"kind":"notify","message":"Plan saved"}`),
	})
	if response.ID != "dialog_notify" || response.Error != nil {
		t.Fatalf("dialog response = %#v", response)
	}
	if result, ok := response.Result.(TUIDialogResult); !ok || result.Action != "acknowledged" {
		t.Fatalf("dialog result = %#v", response.Result)
	}
	waitForViewport(t, terminal, "Plan saved")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostReflectsRPCDialogNotifySeverityPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	rpcHost := &RPCSessionHost{TUIDialog: host}
	response := rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "dialog_notify_warning",
		Method:   "host.tui.dialog",
		Params:   []byte(`{"kind":"notify","type":"warning","message":"Review required"}`),
	})
	if response.ID != "dialog_notify_warning" || response.Error != nil {
		t.Fatalf("dialog response = %#v", response)
	}
	waitForViewport(t, terminal, "Warning: Review required")

	response = rpcHost.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "dialog_notify_error",
		Method:   "host.tui.dialog",
		Params:   []byte(`{"kind":"notify","type":"error","message":"Blocked"}`),
	})
	if response.ID != "dialog_notify_error" || response.Error != nil {
		t.Fatalf("dialog response = %#v", response)
	}
	waitForViewport(t, terminal, "Error: Blocked")

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostReflectsRPCDialogConfirmHostAction(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	rpcHost := &RPCSessionHost{TUIDialog: host}
	responseCh := runHostActionAsync(rpcHost, HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "dialog_confirm",
		Method:   "host.tui.dialog",
		Params:   []byte(`{"kind":"confirm","title":"Danger","message":"Continue?","defaultValue":false}`),
	})
	waitForViewport(t, terminal, "Danger")
	waitForViewport(t, terminal, "Continue?")
	terminal.SendInput("\r")

	response := waitForHostActionResponse(t, responseCh)
	if response.ID != "dialog_confirm" || response.Error != nil {
		t.Fatalf("dialog response = %#v", response)
	}
	result, ok := response.Result.(TUIDialogResult)
	if !ok || result.Action != "declined" || result.OptionID != "no" || result.Value != false {
		t.Fatalf("dialog result = %#v", response.Result)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostReflectsRPCDialogSelectHostAction(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	rpcHost := &RPCSessionHost{TUIDialog: host}
	responseCh := runHostActionAsync(rpcHost, HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "dialog_select",
		Method:   "host.tui.dialog",
		Params: []byte(`{
			"kind":"select",
			"title":"Pick option",
			"options":[
				{"id":"a","label":"Alpha","value":"alpha"},
				{"id":"b","label":"Beta","value":"beta"}
			]
		}`),
	})
	waitForViewport(t, terminal, "Pick option")
	waitForViewport(t, terminal, "Alpha")
	terminal.SendInput("\x1b[B")
	terminal.SendInput("\r")

	response := waitForHostActionResponse(t, responseCh)
	if response.ID != "dialog_select" || response.Error != nil {
		t.Fatalf("dialog response = %#v", response)
	}
	result, ok := response.Result.(TUIDialogResult)
	if !ok || result.Action != "selected" || result.OptionID != "b" || result.Value != "beta" {
		t.Fatalf("dialog result = %#v", response.Result)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLISelectDialogSearchFiltersOptionLabels(t *testing.T) {
	var selected TUIDialogOption
	component := newCLISelectDialog(
		"Pick option",
		"",
		[]TUIDialogOption{
			{ID: "alpha", Label: "Alpha", Value: "alpha"},
			{ID: "beta", Label: "Beta", Value: "beta"},
		},
		-1,
		func(option TUIDialogOption) { selected = option },
		func() {},
	)

	component.HandleInput("b")
	component.HandleInput("e")
	rendered := strings.Join(component.Render(80), "\n")
	if !strings.Contains(rendered, "Beta") || strings.Contains(rendered, "Alpha") {
		t.Fatalf("filtered dialog = %q", rendered)
	}
	component.HandleInput("\r")
	if selected.ID != "beta" || selected.Value != "beta" {
		t.Fatalf("selected option = %#v", selected)
	}
}

func TestCLISelectDialogAllowsToolExpansionTogglePiStyle(t *testing.T) {
	var toggles int
	component := newCLISelectDialog(
		"Pick option",
		"",
		[]TUIDialogOption{{ID: "alpha", Label: "Alpha", Value: "alpha"}},
		0,
		func(TUIDialogOption) {},
		func() {},
	)
	component.onToggleToolsExpanded = func() {
		toggles++
	}

	component.HandleInput("\x0f")
	if toggles != 1 {
		t.Fatalf("toggles = %d, want 1", toggles)
	}
	rendered := strings.Join(component.Render(80), "\n")
	if !strings.Contains(rendered, "Alpha") {
		t.Fatalf("dialog changed unexpectedly: %q", rendered)
	}
}

func TestCLISelectDialogUsesEffectiveKeybindingsPiStyle(t *testing.T) {
	previous := gitui.GetKeybindings()
	gitui.SetKeybindings(gitui.NewKeybindingsManager(gitui.KeybindingsConfig{
		"tui.select.up":      []string{"p"},
		"tui.select.down":    []string{"n"},
		"tui.select.confirm": []string{"x"},
		"tui.select.cancel":  []string{"q"},
	}))
	t.Cleanup(func() { gitui.SetKeybindings(previous) })

	var selected TUIDialogOption
	var toggles int
	component := newCLISelectDialog(
		"Pick option",
		"",
		[]TUIDialogOption{
			{ID: "alpha", Label: "Alpha", Value: "alpha"},
			{ID: "beta", Label: "Beta", Value: "beta"},
		},
		0,
		func(option TUIDialogOption) { selected = option },
		func() {},
	)
	component.keybindings = mergeKeybindingsConfig(DefaultProtocolKeybindings(), KeybindingsConfig{"app.tools.expand": "z"})
	component.onToggleToolsExpanded = func() { toggles++ }

	rendered := strings.Join(component.Render(100), "\n")
	for _, expected := range []string{"P/N navigate", "X select", "Q cancel"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("render missing %q:\n%s", expected, rendered)
		}
	}
	component.HandleInput("z")
	if toggles != 1 {
		t.Fatalf("toggles = %d, want 1", toggles)
	}
	component.HandleInput("n")
	component.HandleInput("x")
	if selected.ID != "beta" || selected.Value != "beta" {
		t.Fatalf("selected option = %#v", selected)
	}
}

func TestCLIEditorDialogUsesEffectiveExternalEditorKeybindingPiStyle(t *testing.T) {
	editorPath := filepath.Join(t.TempDir(), "editor.sh")
	if err := os.WriteFile(editorPath, []byte("#!/bin/sh\nprintf 'Edited externally\\n' > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", editorPath)

	component := newCLIEditorDialog(nil, "External edit", "", "Draft", func(string) {}, func() {})
	component.keybindings = mergeKeybindingsConfig(DefaultProtocolKeybindings(), KeybindingsConfig{"app.editor.external": "e"})
	rendered := strings.Join(component.Render(100), "\n")
	if !strings.Contains(rendered, "E external editor") {
		t.Fatalf("render missing external editor hint:\n%s", rendered)
	}

	component.HandleInput("e")
	if got := component.editor.GetText(); got != "Edited externally" {
		t.Fatalf("editor text = %q, want external edit", got)
	}
}

func TestCLIInteractiveTUIHostReflectsRPCDialogInputHostAction(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	rpcHost := &RPCSessionHost{TUIDialog: host}
	responseCh := runHostActionAsync(rpcHost, HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "dialog_input",
		Method:   "host.tui.dialog",
		Params:   []byte(`{"kind":"input","title":"Enter label","placeholder":"label"}`),
	})
	waitForViewport(t, terminal, "Enter label")
	terminal.SendInput("Release")
	terminal.SendInput("\r")

	response := waitForHostActionResponse(t, responseCh)
	if response.ID != "dialog_input" || response.Error != nil {
		t.Fatalf("dialog response = %#v", response)
	}
	result, ok := response.Result.(TUIDialogResult)
	if !ok || result.Action != "submitted" || result.Value != "Release" {
		t.Fatalf("dialog result = %#v", response.Result)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostReflectsRPCDialogEditorHostAction(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	rpcHost := &RPCSessionHost{TUIDialog: host}
	responseCh := runHostActionAsync(rpcHost, HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "dialog_editor",
		Method:   "host.tui.dialog",
		Params:   []byte(`{"kind":"editor","title":"Refine plan","message":"Edit before submitting","defaultValue":"Draft"}`),
	})
	waitForViewport(t, terminal, "Refine plan")
	waitForViewport(t, terminal, "Draft")
	terminal.SendInput("\n")
	terminal.SendInput("Line 2")
	terminal.SendInput("\r")

	response := waitForHostActionResponse(t, responseCh)
	if response.ID != "dialog_editor" || response.Error != nil {
		t.Fatalf("dialog response = %#v", response)
	}
	result, ok := response.Result.(TUIDialogResult)
	if !ok || result.Action != "submitted" || result.Value != "Draft\nLine 2" {
		t.Fatalf("dialog result = %#v", response.Result)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostReflectsRPCDialogEditorExternalEditor(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 24)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	editorPath := filepath.Join(t.TempDir(), "editor.sh")
	if err := os.WriteFile(editorPath, []byte("#!/bin/sh\nprintf 'Edited externally\\n' > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", editorPath)

	rpcHost := &RPCSessionHost{TUIDialog: host}
	responseCh := runHostActionAsync(rpcHost, HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "dialog_editor_external",
		Method:   "host.tui.dialog",
		Params:   []byte(`{"kind":"editor","title":"External edit","defaultValue":"Draft"}`),
	})
	waitForViewport(t, terminal, "External edit")
	terminal.SendInput("\x07")
	waitForViewport(t, terminal, "Edited externally")
	terminal.SendInput("\r")

	response := waitForHostActionResponse(t, responseCh)
	if response.ID != "dialog_editor_external" || response.Error != nil {
		t.Fatalf("dialog response = %#v", response)
	}
	result, ok := response.Result.(TUIDialogResult)
	if !ok || result.Action != "submitted" || result.Value != "Edited externally" {
		t.Fatalf("dialog result = %#v", response.Result)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostReflectsRPCDialogCancelHostAction(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	rpcHost := &RPCSessionHost{TUIDialog: host}
	responseCh := runHostActionAsync(rpcHost, HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "dialog_cancel",
		Method:   "host.tui.dialog",
		Params:   []byte(`{"kind":"input","title":"Cancelable"}`),
	})
	waitForViewport(t, terminal, "Cancelable")
	terminal.SendInput("\x1b")

	response := waitForHostActionResponse(t, responseCh)
	if response.ID != "dialog_cancel" || response.Error != nil {
		t.Fatalf("dialog response = %#v", response)
	}
	result, ok := response.Result.(TUIDialogResult)
	if !ok || result.Action != "cancelled" {
		t.Fatalf("dialog result = %#v", response.Result)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func TestCLIInteractiveTUIHostReflectsRPCDialogTimeoutPiStyle(t *testing.T) {
	runtimeHost := newOfflineInteractiveRuntimeHost(t)
	terminal := gitui.NewVirtualTerminal(100, 20)
	host, err := NewCLIInteractiveTUIHost(CLIInteractiveTUIHostOptions{
		RuntimeHost: runtimeHost,
		Terminal:    terminal,
	})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- host.RunContext(context.Background())
	}()
	t.Cleanup(func() { host.Stop() })
	waitForHostEditor(t, host)

	rpcHost := &RPCSessionHost{TUIDialog: host}
	responseCh := runHostActionAsync(rpcHost, HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "dialog_timeout",
		Method:   "host.tui.dialog",
		Params:   []byte(`{"kind":"select","title":"Timed choice","timeout":30,"options":[{"id":"a","label":"Alpha","value":"alpha"}]}`),
	})
	waitForViewport(t, terminal, "Timed choice (1s)")

	response := waitForHostActionResponse(t, responseCh)
	if response.ID != "dialog_timeout" || response.Error != nil {
		t.Fatalf("dialog response = %#v", response)
	}
	result, ok := response.Result.(TUIDialogResult)
	if !ok || result.Action != "cancelled" {
		t.Fatalf("dialog result = %#v", response.Result)
	}

	host.Stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("RunContext returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive TUI host did not stop")
	}
}

func newOfflineInteractiveRuntimeHost(t *testing.T) PrintModeRuntimeHost {
	t.Helper()
	tempDir := t.TempDir()
	host, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		NoSession: true,
		Model:     "openai/gpt-4o-mini",
	}, CLIOptions{
		CWD:           tempDir,
		AgentDir:      filepath.Join(tempDir, "agent"),
		ModelRegistry: newTestOpenAIModelRegistry(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return host
}

func newOfflineInteractiveRuntimeHostWithPackages(t *testing.T, packages ...string) PrintModeRuntimeHost {
	t.Helper()
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "agent")
	settings := NewSettingsManager(tempDir, agentDir)
	values := make([]any, 0, len(packages))
	for _, pkg := range packages {
		values = append(values, pkg)
	}
	settings.SetPackages(values)
	host, err := newDefaultCLIPrintModeHost(Args{
		Offline:   true,
		NoSession: true,
		Model:     "openai/gpt-4o-mini",
	}, CLIOptions{
		CWD:           tempDir,
		AgentDir:      agentDir,
		ModelRegistry: newTestOpenAIModelRegistry(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return host
}

func newTestOpenAIModelRegistry() *ModelRegistry {
	return NewModelRegistry(NewInMemoryAuthStorage(AuthStorageData{
		"openai": {Type: "api_key", Key: "test-openai-key"},
	}), "")
}

type blockingReloadResourceLoader struct {
	base        AgentSessionResourceLoader
	started     chan struct{}
	unblock     chan struct{}
	startOnce   sync.Once
	releaseOnce sync.Once
}

func newBlockingReloadResourceLoader(base AgentSessionResourceLoader) *blockingReloadResourceLoader {
	return &blockingReloadResourceLoader{
		base:    base,
		started: make(chan struct{}),
		unblock: make(chan struct{}),
	}
}

func (l *blockingReloadResourceLoader) Reload() {
	l.startOnce.Do(func() {
		close(l.started)
	})
	<-l.unblock
	if reloader, ok := l.base.(interface{ Reload() }); ok {
		reloader.Reload()
	}
}

func (l *blockingReloadResourceLoader) Release() {
	l.releaseOnce.Do(func() {
		close(l.unblock)
	})
}

func (l *blockingReloadResourceLoader) GetSkills() AgentSessionSkillsResult {
	if l != nil && l.base != nil {
		return l.base.GetSkills()
	}
	return AgentSessionSkillsResult{}
}

func (l *blockingReloadResourceLoader) GetPrompts() ResourcePromptsResult {
	if l != nil {
		if loader, ok := l.base.(AgentSessionPromptResourceLoader); ok {
			return loader.GetPrompts()
		}
	}
	return ResourcePromptsResult{}
}

func (l *blockingReloadResourceLoader) GetExtensions() ResourceExtensionsResult {
	if l != nil {
		if loader, ok := l.base.(agentSessionExtensionsResourceLoader); ok {
			return loader.GetExtensions()
		}
	}
	return ResourceExtensionsResult{}
}

func (l *blockingReloadResourceLoader) ApplyExtensionFlagValues(values map[string]any, allowDeferred bool) []ProtocolExtensionDiscoveryError {
	if l != nil {
		if loader, ok := l.base.(agentSessionExtensionFlagResourceLoader); ok {
			return loader.ApplyExtensionFlagValues(values, allowDeferred)
		}
	}
	return nil
}

func (l *blockingReloadResourceLoader) GetThemes() ResourceThemesResult {
	if l != nil {
		if loader, ok := l.base.(agentSessionThemesResourceLoader); ok {
			return loader.GetThemes()
		}
	}
	return ResourceThemesResult{}
}

func (l *blockingReloadResourceLoader) GetAgentsFiles() ResourceAgentsFilesResult {
	if l != nil {
		if loader, ok := l.base.(agentSessionAgentsFilesResourceLoader); ok {
			return loader.GetAgentsFiles()
		}
	}
	return ResourceAgentsFilesResult{}
}

type recordingProgressTerminal struct {
	*gitui.VirtualTerminal
	mu       sync.Mutex
	progress []bool
}

func (t *recordingProgressTerminal) SetProgress(active bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.progress = append(t.progress, active)
	return nil
}

func (t *recordingProgressTerminal) progressSnapshot() []bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]bool(nil), t.progress...)
}

func waitForProgressSequence(t *testing.T, terminal *recordingProgressTerminal, want []bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got := terminal.progressSnapshot()
		if hasBoolPrefix(got, want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("terminal progress = %#v, want prefix %#v", terminal.progressSnapshot(), want)
}

func hasBoolPrefix(got, want []bool) bool {
	if len(got) < len(want) {
		return false
	}
	for index, value := range want {
		if got[index] != value {
			return false
		}
	}
	return true
}

func waitForViewport(t *testing.T, terminal *gitui.VirtualTerminal, expected string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		terminal.WaitForRender()
		viewport := strings.Join(terminal.GetViewport(), "\n")
		if strings.Contains(viewport, expected) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("viewport did not contain %q:\n%s", expected, strings.Join(terminal.GetViewport(), "\n"))
}

func waitForCondition(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(message)
}

func waitForTerminalOutput(t *testing.T, terminal *gitui.VirtualTerminal, expected string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		terminal.WaitForRender()
		output := terminal.Output()
		if strings.Contains(output, expected) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("terminal output did not contain %q:\n%s", expected, terminal.Output())
}

func viewportLineIndex(lines []string, expected string) int {
	for index, line := range lines {
		if strings.Contains(line, expected) {
			return index
		}
	}
	return -1
}

func viewportTrimmedLineIndex(lines []string, expected string) int {
	for index, line := range lines {
		if strings.TrimSpace(line) == expected {
			return index
		}
	}
	return -1
}

func waitForThemePreview(t *testing.T, host *CLIInteractiveTUIHost, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := host.CurrentTUITheme(); got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("theme preview = %q, want %q", host.CurrentTUITheme(), want)
}

func waitForFocusedComponent(t *testing.T, terminal *gitui.VirtualTerminal, host *CLIInteractiveTUIHost, component gitui.Component) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		terminal.WaitForRender()
		if host.ui.FocusedComponent() == component {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("focused component = %T, want %T", host.ui.FocusedComponent(), component)
}

func countExactViewportLine(viewport []string, text string) int {
	count := 0
	for _, line := range viewport {
		if strings.TrimSpace(line) == text {
			count++
		}
	}
	return count
}

func slashCommandNamesContain(commands []gitui.SlashCommand, name string) bool {
	for _, command := range commands {
		if command.Name == name {
			return true
		}
	}
	return false
}

func slashCommandByName(commands []gitui.SlashCommand, name string) (gitui.SlashCommand, bool) {
	for _, command := range commands {
		if command.Name == name {
			return command, true
		}
	}
	return gitui.SlashCommand{}, false
}

func tuiThemeInfoHasName(themes []TUIThemeInfo, name string) bool {
	for _, theme := range themes {
		if theme.Name == name {
			return true
		}
	}
	return false
}

func settingItemForTest(t *testing.T, items []gitui.SettingItem, id string) gitui.SettingItem {
	t.Helper()
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("setting item %q missing from %#v", id, items)
	return gitui.SettingItem{}
}

func waitForHostEditor(t *testing.T, host *CLIInteractiveTUIHost) {
	t.Helper()
	if host != nil && host.uiReady != nil {
		select {
		case <-host.uiReady:
		case <-time.After(time.Second):
			t.Fatal("interactive TUI host did not initialize UI")
		}
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if host.editor != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("interactive TUI host did not initialize editor")
}

func waitForProcessSupervisor(t *testing.T, host *CLIInteractiveTUIHost) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if host != nil && host.processSupervisor != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("interactive TUI host did not initialize process supervisor")
}

func sessionHasRole(session *AgentSession, role string) bool {
	if session == nil || session.SessionManager == nil {
		return false
	}
	for _, entry := range session.SessionManager.GetEntries() {
		message, ok := sessionMessageToLLM(entry.Message)
		if ok && message.Role == role {
			return true
		}
	}
	return false
}

func firstSessionEntryIDByRole(t *testing.T, session *AgentSession, role string) string {
	t.Helper()
	for _, entry := range session.SessionManager.GetEntries() {
		message, ok := sessionMessageToLLM(entry.Message)
		if ok && message.Role == role {
			return entry.ID
		}
	}
	t.Fatalf("no session entry for role %q", role)
	return ""
}

func waitForHostAutocompleteChange(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for autocomplete update")
	}
}

func waitForPrompt(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case got := <-ch:
			if got == want {
				return
			}
			t.Fatalf("prompt = %q, want %q", got, want)
		case <-deadline:
			t.Fatalf("timed out waiting for prompt %q", want)
		}
	}
}

func waitForBashCommand(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("bash command = %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for bash command %q", want)
	}
}

func waitForProtocolAutocompleteRequest(t *testing.T, ch <-chan ProtocolAutocompleteRequest) ProtocolAutocompleteRequest {
	t.Helper()
	select {
	case request := <-ch:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for protocol autocomplete request")
		return ProtocolAutocompleteRequest{}
	}
}

func waitForEditorText(t *testing.T, host *CLIInteractiveTUIHost, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if host.editor != nil && host.editor.GetText() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	got := ""
	if host.editor != nil {
		got = host.editor.GetText()
	}
	t.Fatalf("editor text = %q, want %q", got, want)
}

func waitForEditorTextSuffix(t *testing.T, host *CLIInteractiveTUIHost, suffix string) string {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if host.editor != nil {
			text := host.editor.GetText()
			if strings.HasSuffix(text, suffix) {
				return text
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	got := ""
	if host.editor != nil {
		got = host.editor.GetText()
	}
	t.Fatalf("editor text = %q, want suffix %q", got, suffix)
	return ""
}

func waitForToolExpanded(t *testing.T, host *CLIInteractiveTUIHost, want bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if host != nil && host.chat != nil {
			for _, child := range host.chat.Children() {
				if tool, ok := child.(*ToolExecutionComponent); ok && tool.expanded == want {
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no tool component with expanded=%v", want)
}

func waitForSlotChildCount(t *testing.T, host *CLIInteractiveTUIHost, slot string, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if host.slots != nil && host.slots[slot] != nil && host.slots[slot].ChildCount() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	got := -1
	if host.slots != nil && host.slots[slot] != nil {
		got = host.slots[slot].ChildCount()
	}
	t.Fatalf("slot %q child count = %d, want %d", slot, got, want)
}

func waitForEditorContainerChildCount(t *testing.T, host *CLIInteractiveTUIHost, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if host.editorContainer != nil && host.editorContainer.ChildCount() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	got := -1
	if host.editorContainer != nil {
		got = host.editorContainer.ChildCount()
	}
	t.Fatalf("editor container child count = %d, want %d", got, want)
}

func waitForNoOverlay(t *testing.T, host *CLIInteractiveTUIHost) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if host.ui != nil && !host.ui.HasOverlay() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("interactive TUI host still has an overlay")
}

func waitForInProcessCustomResult(t *testing.T, ch <-chan InProcessCustomResult) InProcessCustomResult {
	t.Helper()
	select {
	case result, ok := <-ch:
		if !ok {
			t.Fatal("in-process custom result channel closed without a result")
		}
		return result
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for in-process custom result")
	}
	return InProcessCustomResult{}
}

func waitForViewTreeEvent(t *testing.T, events *[]ViewTreeEvent, mountID, nodeID, eventName string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, event := range *events {
			if event.MountID == mountID && event.NodeID == nodeID && event.Event == eventName {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("viewtree event %s/%s/%s not observed: %#v", mountID, nodeID, eventName, *events)
}

func runHostActionAsync(host *RPCSessionHost, request HostActionRequest) <-chan HostActionResponse {
	responseCh := make(chan HostActionResponse, 1)
	go func() {
		responseCh <- host.HandleHostAction(request)
	}()
	return responseCh
}

func waitForHostActionResponse(t *testing.T, responseCh <-chan HostActionResponse) HostActionResponse {
	t.Helper()
	select {
	case response := <-responseCh:
		return response
	case <-time.After(time.Second):
		t.Fatal("host action did not return")
		return HostActionResponse{}
	}
}

func assertHostActionResponseOK(t *testing.T, response HostActionResponse, id string) map[string]any {
	t.Helper()
	if response.ID != id || response.Error != nil {
		t.Fatalf("response = %#v", response)
	}
	result, ok := response.Result.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v", response.Result)
	}
	return result
}
