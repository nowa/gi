package gicodingagent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestProtocolExtensionProcessSupervisorRunsPackageViewTreeProcess(t *testing.T) {
	host := NewRPCSessionHost(nil)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "todo-widget",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_PROCESS_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	rendered := waitForProcessMountedView(t, host, "todo.current")
	if !reflect.DeepEqual(rendered, []string{"Process Widget", "[ ] Started from package process"}) {
		t.Fatalf("rendered = %#v", rendered)
	}
}

func TestProtocolExtensionProcessSupervisorUnmountsOwnedViewTreeMountsOnStop(t *testing.T) {
	host := NewRPCSessionHost(nil)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "todo-widget",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_PROCESS_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	if rendered := waitForProcessMountedView(t, host, "todo.current"); len(rendered) == 0 {
		t.Fatal("expected process widget to mount before shutdown")
	}
	if err := supervisor.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForProcessMountGone(t, host, "todo.current")
}

func TestProtocolExtensionProcessSupervisorClearsOwnedTUIStatusOnStop(t *testing.T) {
	host := NewRPCSessionHost(nil)
	host.ViewTreeHost = NewViewTreeHost()
	statusHost := &recordingTUIStatusHost{}
	host.TUIStatus = statusHost
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "status-widget",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.status"},
		Env:          map[string]string{"GI_EXTENSION_STATUS_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	rendered := waitForProcessMountedView(t, host, "status:process-status")
	if !reflect.DeepEqual(rendered, []string{"Process busy"}) {
		t.Fatalf("status mount = %#v", rendered)
	}
	waitForCondition(t, func() bool {
		return statusHost.statuses["process-status"] == "Process busy"
	}, "process status to reach TUI status host")

	if err := supervisor.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForProcessMountGone(t, host, "status:process-status")
	waitForCondition(t, func() bool {
		_, ok := statusHost.statuses["process-status"]
		return !ok
	}, "process status to be cleared from TUI status host")
}

func TestProtocolExtensionProcessUnmountsOwnedViewTreeMountsOnUnexpectedExit(t *testing.T) {
	host := NewRPCSessionHost(nil)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "ephemeral-widget",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_EXIT_AFTER_MOUNT_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	if rendered := waitForProcessMountedView(t, host, "ephemeral.widget"); !reflect.DeepEqual(rendered, []string{"Ephemeral Widget"}) {
		t.Fatalf("rendered = %#v", rendered)
	}
	waitForProcessMountGone(t, host, "ephemeral.widget")
}

func TestProtocolExtensionProcessRestoresOwnedTUIStateOnUnexpectedExit(t *testing.T) {
	host := NewRPCSessionHost(nil)
	titleHost := &protocolProcessTestTitleHost{}
	workingHost := &protocolProcessTestWorkingHost{}
	thinkingHost := &protocolProcessTestThinkingLabelHost{}
	host.TUITitle = titleHost
	host.TUIWorking = workingHost
	host.TUIThinkingLabel = thinkingHost
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "tui-state-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.title", "tui.working", "tui.thinking_label"},
		Env:          map[string]string{"GI_EXTENSION_TUI_STATE_EXIT_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	waitForProcessTitle(t, titleHost, "Package title")
	update := waitForProcessWorkingUpdate(t, workingHost)
	if !update.MessageSet || update.Message != "Package busy" || !update.VisibleSet || !update.Visible {
		t.Fatalf("working update = %#v", update)
	}
	waitForProcessThinkingLabel(t, thinkingHost, "Reasoning hidden")

	waitForProcessTitle(t, titleHost, "")
	reset := waitForProcessWorkingReset(t, workingHost)
	if !reset.ResetMessage || !reset.ResetIndicator || !reset.VisibleSet || !reset.Visible {
		t.Fatalf("working reset = %#v", reset)
	}
	waitForProcessThinkingLabelReset(t, thinkingHost)
}

func TestProtocolExtensionProcessIncludesStderrWhenProcessExitsBeforeHello(t *testing.T) {
	host := NewRPCSessionHost(nil)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "broken-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_FAIL_BEFORE_HELLO_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	err = supervisor.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "extension process exited before hello") || !strings.Contains(err.Error(), "boom before hello") {
		t.Fatalf("start error = %v", err)
	}
}

func TestProtocolExtensionProcessIncludesStderrWhenHelloTimesOut(t *testing.T) {
	host := NewRPCSessionHost(nil)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "hanging-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_HANG_BEFORE_HELLO_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = 50 * time.Millisecond
	err = supervisor.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "extension process hello timeout") || !strings.Contains(err.Error(), "warming before hello") {
		t.Fatalf("start error = %v", err)
	}
}

func TestProtocolExtensionProcessReportsShutdownTimeoutWithStderr(t *testing.T) {
	host := NewRPCSessionHost(nil)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "stuck-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_IGNORE_SHUTDOWN_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = 50 * time.Millisecond
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	err = supervisor.Stop(context.Background())
	if err == nil || !strings.Contains(err.Error(), "extension process shutdown timeout") || !strings.Contains(err.Error(), "ignoring shutdown") {
		t.Fatalf("shutdown error = %v", err)
	}
}

func TestProtocolExtensionProcessEmitsStderrDiagnostics(t *testing.T) {
	runtime := NewProtocolExtensionRuntime()
	session := &AgentSession{ExtensionRuntime: runtime}
	host := NewRPCSessionHost(session)
	events := make(chan ProtocolExtensionError, 2)
	unwatch := runtime.OnError(func(event ProtocolExtensionError) {
		events <- event
	})
	t.Cleanup(unwatch)

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:         "noisy-extension",
		PackageDir: t.TempDir(),
		Command:    []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:  "stdio-ndjson",
		Protocol:   "gi-ext-rpc@1",
		Metadata: ProtocolSourceInfo{
			Path:   "packages/noisy/extensions/index.gi.json",
			Source: "process:noisy-extension",
			Scope:  "project",
			Origin: "package",
		},
		Env: map[string]string{"GI_EXTENSION_STDERR_DIAGNOSTIC_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	var event ProtocolExtensionError
	select {
	case event = <-events:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stderr diagnostic")
	}
	if event.ExtensionPath != "packages/noisy/extensions/index.gi.json" || event.Event != "stderr" || event.Error != "diagnostic from stderr" {
		t.Fatalf("event = %#v", event)
	}
	processes := supervisor.Processes()
	if len(processes) != 1 || !strings.Contains(processes[0].stderrTailString(), "diagnostic from stderr") {
		t.Fatalf("stderr tail = %#v", processes)
	}
}

func TestProtocolExtensionProcessEmitsCrashDiagnostics(t *testing.T) {
	runtime := NewProtocolExtensionRuntime()
	session := &AgentSession{ExtensionRuntime: runtime}
	host := NewRPCSessionHost(session)
	events := make(chan ProtocolExtensionError, 4)
	unwatch := runtime.OnError(func(event ProtocolExtensionError) {
		events <- event
	})
	t.Cleanup(unwatch)

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:         "crashy-extension",
		PackageDir: t.TempDir(),
		Command:    []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:  "stdio-ndjson",
		Protocol:   "gi-ext-rpc@1",
		Metadata: ProtocolSourceInfo{
			Path:   "packages/crashy/extensions/index.gi.json",
			Source: "process:crashy-extension",
			Scope:  "project",
			Origin: "package",
		},
		Env: map[string]string{"GI_EXTENSION_CRASH_AFTER_EVENT_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	if err := supervisor.EmitEvent(ProtocolEventSessionStart, map[string]any{"cwd": t.TempDir()}); err != nil {
		t.Fatal(err)
	}

	var event ProtocolExtensionError
	deadline := time.After(time.Second)
	for event.Event != "process.exit" {
		select {
		case event = <-events:
		case <-deadline:
			t.Fatal("timed out waiting for process exit diagnostic")
		}
	}
	if event.ExtensionPath != "packages/crashy/extensions/index.gi.json" ||
		!strings.Contains(event.Error, "extension process exited") ||
		!strings.Contains(event.Error, "crashing after event") {
		t.Fatalf("event = %#v", event)
	}
}

func TestProtocolExtensionProcessEnvIncludesHostMetadata(t *testing.T) {
	packageDir := t.TempDir()
	env := protocolExtensionProcessEnv(ProtocolPackageProcessExtension{
		ID:         "metadata-extension",
		PackageDir: packageDir,
		Env: map[string]string{
			"CUSTOM_ENV":       "custom",
			"GI_EXTENSION_ID":  "spoofed",
			"GI_EXTENSION_BAD": "allowed",
			"BAD=KEY":          "ignored",
		},
		Metadata: ProtocolSourceInfo{
			Path:   "packages/metadata/gi.package.json#metadata-extension",
			Source: "git:https://example.test/metadata.git",
			Scope:  "project",
			Origin: "package",
		},
	})
	values := lastEnvValues(env)
	if values["CUSTOM_ENV"] != "custom" || values["GI_EXTENSION_BAD"] != "allowed" {
		t.Fatalf("custom env missing from %#v", values)
	}
	if _, ok := values["BAD"]; ok {
		t.Fatalf("invalid env key was included in %#v", values)
	}
	for key, want := range map[string]string{
		"GI_EXTENSION_ID":          "metadata-extension",
		"GI_EXTENSION_PATH":        "packages/metadata/gi.package.json#metadata-extension",
		"GI_EXTENSION_SOURCE":      "git:https://example.test/metadata.git",
		"GI_EXTENSION_SCOPE":       "project",
		"GI_EXTENSION_ORIGIN":      "package",
		"GI_EXTENSION_PACKAGE_DIR": packageDir,
	} {
		if values[key] != want {
			t.Fatalf("%s = %q, want %q in %#v", key, values[key], want, values)
		}
	}
}

func lastEnvValues(entries []string) map[string]string {
	values := map[string]string{}
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		values[key] = value
	}
	return values
}

func TestProtocolExtensionProcessSupervisorRunsPackageHeaderProcess(t *testing.T) {
	host := NewRPCSessionHost(nil)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "header-widget",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.header"},
		Env:          map[string]string{"GI_EXTENSION_HEADER_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	rendered := waitForProcessMountedView(t, host, "header.status")
	if !reflect.DeepEqual(rendered, []string{"Process Header"}) {
		t.Fatalf("rendered = %#v", rendered)
	}
}

func TestProtocolExtensionProcessSupervisorRunsPackageFooterProcess(t *testing.T) {
	host := NewRPCSessionHost(nil)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "footer-widget",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.footer"},
		Env:          map[string]string{"GI_EXTENSION_FOOTER_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	rendered := waitForProcessMountedView(t, host, "footer.status")
	if !reflect.DeepEqual(rendered, []string{"Process Footer"}) {
		t.Fatalf("rendered = %#v", rendered)
	}
}

func TestProtocolExtensionProcessSupervisorRunsPackageOverlayProcess(t *testing.T) {
	host := NewRPCSessionHost(nil)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "overlay-widget",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.overlay"},
		Env:          map[string]string{"GI_EXTENSION_OVERLAY_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	rendered := waitForProcessMountedView(t, host, "overlay.status")
	if !reflect.DeepEqual(rendered, []string{"Process Overlay"}) {
		t.Fatalf("rendered = %#v", rendered)
	}
}

func TestProtocolExtensionProcessSupervisorFansOutLifecycleEvents(t *testing.T) {
	host := NewRPCSessionHost(nil)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "lifecycle-widget",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_LIFECYCLE_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	if err := supervisor.EmitEvent(ProtocolEventSessionStart, map[string]any{"reason": "startup"}); err != nil {
		t.Fatal(err)
	}
	rendered := waitForProcessMountedView(t, host, "lifecycle.start")
	if !reflect.DeepEqual(rendered, []string{"Lifecycle: startup"}) {
		t.Fatalf("rendered = %#v", rendered)
	}
}

func TestProtocolExtensionProcessCanUseEditorHostAction(t *testing.T) {
	host := NewRPCSessionHost(nil)
	editor := &protocolProcessTestEditor{}
	host.TUIEditor = editor
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "editor-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.editor"},
		Env:          map[string]string{"GI_EXTENSION_EDITOR_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	waitForProcessEditorState(t, editor, "Package plan pasted", 1)
	if got := editor.pasteCount(); got != 1 {
		t.Fatalf("paste count = %d, want 1", got)
	}
}

func TestProtocolExtensionProcessCanUseTitleHostAction(t *testing.T) {
	host := NewRPCSessionHost(nil)
	titleHost := &protocolProcessTestTitleHost{}
	host.TUITitle = titleHost
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "title-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.title"},
		Env:          map[string]string{"GI_EXTENSION_TITLE_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			_ = supervisor.Stop(context.Background())
		}
	})

	waitForProcessTitle(t, titleHost, "Package title")
	if err := supervisor.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopped = true
	waitForProcessTitle(t, titleHost, "")
}

func TestProtocolExtensionProcessCanUseWorkingHostAction(t *testing.T) {
	host := NewRPCSessionHost(nil)
	workingHost := &protocolProcessTestWorkingHost{}
	host.TUIWorking = workingHost
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "working-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.working"},
		Env:          map[string]string{"GI_EXTENSION_WORKING_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			_ = supervisor.Stop(context.Background())
		}
	})

	update := waitForProcessWorkingUpdate(t, workingHost)
	if !update.MessageSet || update.Message != "Package busy" || !update.VisibleSet || !update.Visible ||
		!update.IndicatorSet || !reflect.DeepEqual(update.Indicator.Frames, []string{"."}) || update.Indicator.IntervalMs != 25 {
		t.Fatalf("working update = %#v", update)
	}
	if err := supervisor.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopped = true
	reset := waitForProcessWorkingReset(t, workingHost)
	if !reset.ResetMessage || !reset.ResetIndicator || !reset.VisibleSet || !reset.Visible {
		t.Fatalf("working reset = %#v", reset)
	}
}

func TestProtocolExtensionProcessCanUseThinkingLabelHostAction(t *testing.T) {
	host := NewRPCSessionHost(nil)
	thinkingHost := &protocolProcessTestThinkingLabelHost{}
	host.TUIThinkingLabel = thinkingHost
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "thinking-label-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.thinking_label"},
		Env:          map[string]string{"GI_EXTENSION_THINKING_LABEL_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			_ = supervisor.Stop(context.Background())
		}
	})

	waitForProcessThinkingLabel(t, thinkingHost, "Reasoning hidden")
	if err := supervisor.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopped = true
	waitForProcessThinkingLabelReset(t, thinkingHost)
}

func TestProtocolExtensionProcessCanUseThemeHostAction(t *testing.T) {
	host := NewRPCSessionHost(nil)
	themeHost := &protocolProcessTestThemeHost{
		current: "dark",
		themes:  []TUIThemeInfo{{Name: "dark", Builtin: true}, {Name: "focus", Path: "/tmp/focus.json"}},
	}
	host.TUITheme = themeHost
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "theme-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.theme"},
		Env:          map[string]string{"GI_EXTENSION_THEME_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	waitForProcessTheme(t, themeHost, "focus")
}

func TestProtocolExtensionProcessCanUseToolExpansionHostAction(t *testing.T) {
	host := NewRPCSessionHost(nil)
	toolExpansionHost := &protocolProcessTestToolExpansionHost{}
	host.TUIToolExpansion = toolExpansionHost
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "tools-expanded-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.tools_expanded"},
		Env:          map[string]string{"GI_EXTENSION_TOOLS_EXPANDED_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	waitForProcessToolsExpanded(t, toolExpansionHost, true)
}

func TestProtocolExtensionProcessSupervisorEmitsTerminalInputToCapableProcess(t *testing.T) {
	host := NewRPCSessionHost(nil)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "terminal-input-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.terminal_input", "tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_TERMINAL_INPUT_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	if err := supervisor.EmitTerminalInput("x"); err != nil {
		t.Fatal(err)
	}
	rendered := waitForProcessMountedView(t, host, "terminal.input")
	if !reflect.DeepEqual(rendered, []string{"Terminal input: x"}) {
		t.Fatalf("rendered = %#v", rendered)
	}
}

func TestProtocolExtensionProcessSupervisorRequestsUserBashResultFromCapableProcess(t *testing.T) {
	host := NewRPCSessionHost(nil)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "bash-intercept-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{CapabilityBashIntercept},
		Env:          map[string]string{"GI_EXTENSION_BASH_INTERCEPT_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	result, handled, err := supervisor.EmitUserBash(context.Background(), map[string]any{
		"command":            "printf process",
		"cwd":                "/tmp/project",
		"excludeFromContext": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !handled || result == nil {
		t.Fatalf("handled=%v result=%#v", handled, result)
	}
	if result.Output != "process bash: printf process @ /tmp/project excluded=true\n" || result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestProtocolExtensionProcessSupervisorDiscoversResourcesFromCapableProcess(t *testing.T) {
	host := NewRPCSessionHost(nil)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	packageDir := t.TempDir()
	metadata := ProtocolSourceInfo{Source: "local:" + packageDir, Scope: "project", Origin: "package"}
	spec := ProtocolPackageProcessExtension{
		ID:           "resources-extension",
		Path:         packageDir + "/gi.package.json#resources-extension",
		PackageDir:   packageDir,
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{CapabilityResourcesDiscover},
		Env:          map[string]string{"GI_EXTENSION_RESOURCES_DISCOVER_HELPER": "1"},
		Metadata:     metadata,
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	resources, err := supervisor.DiscoverResources(context.Background(), "startup", "/tmp/project")
	if err != nil {
		t.Fatal(err)
	}
	wantSkill := packageDir + "/skills/dynamic-skill"
	wantPrompt := packageDir + "/prompts/dynamic.md"
	wantTheme := packageDir + "/themes/dynamic.json"
	if len(resources.SkillPaths) != 1 || resources.SkillPaths[0].Path != wantSkill ||
		len(resources.PromptPaths) != 1 || resources.PromptPaths[0].Path != wantPrompt ||
		len(resources.ThemePaths) != 1 || resources.ThemePaths[0].Path != wantTheme {
		t.Fatalf("resources = %#v", resources)
	}
	if resources.SkillPaths[0].Metadata.Source != metadata.Source ||
		resources.SkillPaths[0].Metadata.Scope != "project" ||
		resources.SkillPaths[0].Metadata.Origin != "package" ||
		resources.SkillPaths[0].Metadata.Path != wantSkill {
		t.Fatalf("skill metadata = %#v", resources.SkillPaths[0].Metadata)
	}
}

func TestProtocolExtensionProcessSupervisorDiscoversReloadResourcesFromCapableProcess(t *testing.T) {
	host := NewRPCSessionHost(nil)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	packageDir := t.TempDir()
	spec := ProtocolPackageProcessExtension{
		ID:           "resources-extension",
		Path:         packageDir + "/gi.package.json#resources-extension",
		PackageDir:   packageDir,
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{CapabilityResourcesDiscover},
		Env: map[string]string{
			"GI_EXTENSION_RESOURCES_DISCOVER_HELPER": "1",
			"GI_EXTENSION_RESOURCES_EXPECT_REASON":   "reload",
		},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	resources, err := supervisor.DiscoverResources(context.Background(), "reload", "/tmp/project")
	if err != nil {
		t.Fatal(err)
	}
	if len(resources.SkillPaths) != 1 || resources.SkillPaths[0].Path != packageDir+"/skills/dynamic-skill" {
		t.Fatalf("resources = %#v", resources)
	}
}

func TestProtocolExtensionProcessReceivesViewTreeTickAndPatchesMount(t *testing.T) {
	viewHost := NewViewTreeHost()
	host := NewRPCSessionHost(nil)
	host.ViewTreeHost = viewHost
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "tick-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{CapabilityTUIWidget},
		Env:          map[string]string{"GI_EXTENSION_TICK_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	if rendered := waitForProcessMountedView(t, host, "tick.widget"); !reflect.DeepEqual(rendered, []string{"Tick 0"}) {
		t.Fatalf("rendered = %#v", rendered)
	}
	if err := viewHost.DispatchTick(3); err != nil {
		t.Fatal(err)
	}
	rendered := waitForProcessMountedViewText(t, host, "tick.widget", "Ticked 3")
	if !reflect.DeepEqual(rendered, []string{"Ticked 3"}) {
		t.Fatalf("rendered after tick = %#v", rendered)
	}
}

func TestProtocolExtensionProcessReceivesViewTreeResizeAndPatchesMount(t *testing.T) {
	viewHost := NewViewTreeHost()
	host := NewRPCSessionHost(nil)
	host.ViewTreeHost = viewHost
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "resize-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{CapabilityTUIWidget},
		Env:          map[string]string{"GI_EXTENSION_RESIZE_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	if rendered := waitForProcessMountedView(t, host, "resize.widget"); !reflect.DeepEqual(rendered, []string{"Size 0x0"}) {
		t.Fatalf("rendered = %#v", rendered)
	}
	if err := viewHost.DispatchResize(100, 24); err != nil {
		t.Fatal(err)
	}
	rendered := waitForProcessMountedViewText(t, host, "resize.widget", "Size 100x24")
	if !reflect.DeepEqual(rendered, []string{"Size 100x24"}) {
		t.Fatalf("rendered after resize = %#v", rendered)
	}
}

func TestProtocolExtensionProcessReceivesViewTreeThemeChangeAndPatchesMount(t *testing.T) {
	viewHost := NewViewTreeHost()
	host := NewRPCSessionHost(nil)
	host.ViewTreeHost = viewHost
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "theme-event-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{CapabilityTUIWidget},
		Env:          map[string]string{"GI_EXTENSION_THEME_EVENT_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	if rendered := waitForProcessMountedView(t, host, "theme-event.widget"); !reflect.DeepEqual(rendered, []string{"Theme dark"}) {
		t.Fatalf("rendered = %#v", rendered)
	}
	if err := viewHost.DispatchThemeChange("focus"); err != nil {
		t.Fatal(err)
	}
	rendered := waitForProcessMountedViewText(t, host, "theme-event.widget", "Theme focus")
	if !reflect.DeepEqual(rendered, []string{"Theme focus"}) {
		t.Fatalf("rendered after theme_change = %#v", rendered)
	}
}

func TestProtocolExtensionProcessReceivesViewTreeVisibilityChangeOnMount(t *testing.T) {
	viewHost := NewViewTreeHost()
	host := NewRPCSessionHost(nil)
	host.ViewTreeHost = viewHost
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "visibility-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{CapabilityTUIWidget},
		Env:          map[string]string{"GI_EXTENSION_VISIBILITY_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	rendered := waitForProcessMountedViewText(t, host, "visibility.widget", "Visible true")
	if !reflect.DeepEqual(rendered, []string{"Visible true"}) {
		t.Fatalf("rendered after visibility_change = %#v", rendered)
	}
}

func TestProtocolExtensionProcessReceivesViewTreeSemanticInputEvents(t *testing.T) {
	viewHost := NewViewTreeHost()
	host := NewRPCSessionHost(nil)
	host.ViewTreeHost = viewHost
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "semantic-input-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{CapabilityTUIEditor},
		Env:          map[string]string{"GI_EXTENSION_SEMANTIC_INPUT_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	if rendered := waitForProcessMountedView(t, host, "semantic.editor"); !reflect.DeepEqual(rendered, []string{"> Semantic ready"}) {
		t.Fatalf("rendered = %#v", rendered)
	}
	component := NewMountedViewTreeComponent(viewHost, "semantic.editor")
	component.SetFocused(true)
	component.HandleInput("z")
	rendered := waitForProcessMountedViewText(t, host, "semantic.editor", "Text z")
	if !reflect.DeepEqual(rendered, []string{"> Text z"}) {
		t.Fatalf("rendered after textInput = %#v", rendered)
	}
	component.HandleInput("\r")
	rendered = waitForProcessMountedViewText(t, host, "semantic.editor", "Submitted")
	if !reflect.DeepEqual(rendered, []string{"> Submitted"}) {
		t.Fatalf("rendered after submit = %#v", rendered)
	}
	component.HandleInput("\x1b")
	rendered = waitForProcessMountedViewText(t, host, "semantic.editor", "Cancelled")
	if !reflect.DeepEqual(rendered, []string{"> Cancelled"}) {
		t.Fatalf("rendered after cancel = %#v", rendered)
	}
}

func TestProtocolExtensionProcessCanRegisterCommandAndReceiveInvocations(t *testing.T) {
	sessionManager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            sessionManager.GetCWD(),
		SessionManager: sessionManager,
		Model:          sdkTestModel(),
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityCommandsRegister)
	runtime.BindSession(session)
	host := NewRPCSessionHost(session)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "plan-mode",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"commands.register", "tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_COMMAND_HELPER": "1"},
		Metadata:     ProtocolSourceInfo{Path: "gi.package.json#plan-mode", Source: "local:test", Scope: "temporary", Origin: "package"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	command := waitForProtocolCommand(t, runtime, "plan")
	if command.Description != "Enter plan mode" || command.ArgumentHint != "<topic>" || command.SourceInfo.Source != "local:test" {
		t.Fatalf("command = %#v", command)
	}
	if err := command.Handler("ship it"); err != nil {
		t.Fatal(err)
	}
	rendered := waitForProcessMountedView(t, host, "plan.mode")
	if !reflect.DeepEqual(rendered, []string{"Command invoked: ship it"}) {
		t.Fatalf("rendered = %#v", rendered)
	}
	if err := supervisor.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForNoProtocolCommand(t, runtime, "plan")
	waitForProcessMountGone(t, host, "plan.mode")
}

func TestProtocolExtensionProcessCanRegisterToolAndReturnResults(t *testing.T) {
	sessionManager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            sessionManager.GetCWD(),
		SessionManager: sessionManager,
		Model:          sdkTestModel(),
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityToolsRegister)
	runtime.BindSession(session)
	host := NewRPCSessionHost(session)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "toolbox",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tools.register"},
		Env:          map[string]string{"GI_EXTENSION_TOOL_HELPER": "1"},
		Metadata:     ProtocolSourceInfo{Path: "gi.package.json#toolbox", Source: "local:test", Scope: "temporary", Origin: "package"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	tool := waitForProtocolTool(t, runtime, "echo_tool")
	if tool.Description != "Echo input through process" || tool.SourceInfo.Source != "local:test" {
		t.Fatalf("tool = %#v", tool)
	}
	result, err := tool.Execute("tool-call-1", map[string]any{"text": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "process tool: hello" {
		t.Fatalf("result = %#v", result)
	}
}

func TestProtocolExtensionProcessCanRegisterProvider(t *testing.T) {
	sessionManager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            sessionManager.GetCWD(),
		SessionManager: sessionManager,
		Model:          sdkTestModel(),
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewInMemoryModelRegistry(nil)
	runtime := NewProtocolExtensionRuntime(CapabilityProvidersRegister)
	runtime.BindModelRegistry(registry)
	runtime.BindSession(session)
	host := NewRPCSessionHost(session)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "provider-pack",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"providers.register"},
		Env:          map[string]string{"GI_EXTENSION_PROVIDER_HELPER": "1"},
		Metadata:     ProtocolSourceInfo{Path: "gi.package.json#provider-pack", Source: "local:test", Scope: "temporary", Origin: "package"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	model := waitForRegisteredModel(t, registry, "rpc-provider", "rpc-model")
	if model.BaseURL != "https://rpc-provider.test/v1" || model.API != "openai-completions" {
		t.Fatalf("model = %#v", model)
	}
}

func TestProtocolExtensionProcessCanRegisterMessageRenderer(t *testing.T) {
	sessionManager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            sessionManager.GetCWD(),
		SessionManager: sessionManager,
		Model:          sdkTestModel(),
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityTUIMessageRenderer)
	runtime.BindSession(session)
	host := NewRPCSessionHost(session)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "message-renderer",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.message_renderer"},
		Env:          map[string]string{"GI_EXTENSION_MESSAGE_RENDERER_HELPER": "1"},
		Metadata:     ProtocolSourceInfo{Path: "gi.package.json#message-renderer", Source: "local:test", Scope: "temporary", Origin: "package"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	renderer := waitForMessageRenderer(t, runtime, "rpc.message")
	rendered := renderer(map[string]any{"text": "hello"}, map[string]any{"width": 40})
	if !reflect.DeepEqual(rendered, []string{"Message render: hello"}) {
		t.Fatalf("rendered = %#v", rendered)
	}
}

func TestProtocolExtensionProcessCanRegisterToolRenderer(t *testing.T) {
	sessionManager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            sessionManager.GetCWD(),
		SessionManager: sessionManager,
		Model:          sdkTestModel(),
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityTUIToolRenderer)
	runtime.BindSession(session)
	host := NewRPCSessionHost(session)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "tool-renderer",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.tool_renderer"},
		Env:          map[string]string{"GI_EXTENSION_TOOL_RENDERER_HELPER": "1"},
		Metadata:     ProtocolSourceInfo{Path: "gi.package.json#tool-renderer", Source: "local:test", Scope: "temporary", Origin: "package"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	waitForToolRenderer(t, runtime, "rendered_tool")
	definition := runtime.GetRegisteredToolDefinition("rendered_tool")
	component := NewToolExecutionComponent("rendered_tool", "tool-call-1", map[string]any{"foo": "bar"}, definition, sessionManager.GetCWD())
	if rendered := component.Render(80); !reflect.DeepEqual(rendered, []string{"Tool call render: bar"}) {
		t.Fatalf("call rendered = %#v", rendered)
	}
	component.UpdateResult(FileToolResult{Text: "done"}, false)
	if rendered := component.Render(80); !reflect.DeepEqual(rendered, []string{"Tool call render: bar", "Tool result render: done"}) {
		t.Fatalf("result rendered = %#v", rendered)
	}
}

func TestProtocolExtensionProcessCanRegisterAutocompleteProvider(t *testing.T) {
	sessionManager, err := InMemorySessionManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            sessionManager.GetCWD(),
		SessionManager: sessionManager,
		Model:          sdkTestModel(),
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewProtocolExtensionRuntime(CapabilityTUIAutocomplete)
	runtime.BindSession(session)
	host := NewRPCSessionHost(session)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "issue-autocomplete",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{"tui.autocomplete"},
		Env:          map[string]string{"GI_EXTENSION_AUTOCOMPLETE_HELPER": "1"},
		Metadata:     ProtocolSourceInfo{Path: "gi.package.json#issue-autocomplete", Source: "local:test", Scope: "temporary", Origin: "package"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	waitForAutocompleteProvider(t, runtime, "issues")
	result, err := runtime.SuggestAutocomplete(context.Background(), ProtocolAutocompleteRequest{Text: "Fix #12", CursorCol: len("Fix #12"), Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Value != "#123" || result.Prefix != "#12" {
		t.Fatalf("autocomplete result = %#v", result)
	}
}

func TestProtocolExtensionProcessCanRegisterShortcutAndReceiveInvocations(t *testing.T) {
	runtime := NewProtocolExtensionRuntime(CapabilityShortcutsRegister)
	host := NewRPCSessionHost(nil)
	host.Session = &AgentSession{ExtensionRuntime: runtime}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:           "shortcut-extension",
		PackageDir:   t.TempDir(),
		Command:      []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:    "stdio-ndjson",
		Protocol:     "gi-ext-rpc@1",
		Capabilities: []string{CapabilityShortcutsRegister, "tui.widget"},
		Env:          map[string]string{"GI_EXTENSION_SHORTCUT_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	shortcut := waitForProtocolShortcut(t, runtime, "ctrl+y")
	if shortcut.Handler == nil {
		t.Fatal("shortcut handler is nil")
	}
	if err := shortcut.Handler(); err != nil {
		t.Fatal(err)
	}
	rendered := waitForProcessMountedView(t, host, "shortcut.status")
	if !reflect.DeepEqual(rendered, []string{"Shortcut invoked: ctrl+y"}) {
		t.Fatalf("rendered = %#v", rendered)
	}
}

func TestProtocolExtensionProcessCanRegisterFlag(t *testing.T) {
	runtime := NewProtocolExtensionRuntime()
	host := NewRPCSessionHost(nil)
	host.Session = &AgentSession{ExtensionRuntime: runtime}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:         "flag-extension",
		PackageDir: t.TempDir(),
		Command:    []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:  "stdio-ndjson",
		Protocol:   "gi-ext-rpc@1",
		Env:        map[string]string{"GI_EXTENSION_FLAG_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	flag := waitForProtocolFlag(t, runtime, "review-mode")
	if flag.Description != "Start review mode" || flag.Type != "boolean" || runtime.FlagValue("review-mode") != true {
		t.Fatalf("flag=%#v value=%#v", flag, runtime.FlagValue("review-mode"))
	}
}

func TestProtocolExtensionProcessAppliesPendingCLIFlagWhenRegistered(t *testing.T) {
	runtime := NewProtocolExtensionRuntime()
	runtime.SetCLIFlagValues(map[string]any{"profile": "fast"})
	host := NewRPCSessionHost(nil)
	host.Session = &AgentSession{ExtensionRuntime: runtime}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ProtocolPackageProcessExtension{
		ID:         "string-flag-extension",
		PackageDir: t.TempDir(),
		Command:    []string{executable, "-test.run=TestProtocolExtensionProcessHelper"},
		Transport:  "stdio-ndjson",
		Protocol:   "gi-ext-rpc@1",
		Env:        map[string]string{"GI_EXTENSION_STRING_FLAG_HELPER": "1"},
	}
	supervisor := NewProtocolExtensionProcessSupervisor(host, []ProtocolPackageProcessExtension{spec})
	supervisor.StartTimeout = time.Second
	supervisor.ShutdownTimeout = time.Second
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = supervisor.Stop(context.Background())
	})

	flag := waitForProtocolFlag(t, runtime, "profile")
	if flag.Type != "string" || runtime.FlagValue("profile") != "fast" {
		t.Fatalf("flag=%#v value=%#v", flag, runtime.FlagValue("profile"))
	}
}

func TestProtocolExtensionProcessHelper(t *testing.T) {
	switch {
	case os.Getenv("GI_EXTENSION_PROCESS_HELPER") == "1":
	case os.Getenv("GI_EXTENSION_EXIT_AFTER_MOUNT_HELPER") == "1":
		runProtocolExtensionExitAfterMountHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_STATUS_HELPER") == "1":
		runProtocolExtensionStatusHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_FAIL_BEFORE_HELLO_HELPER") == "1":
		runProtocolExtensionFailBeforeHelloHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_HANG_BEFORE_HELLO_HELPER") == "1":
		runProtocolExtensionHangBeforeHelloHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_IGNORE_SHUTDOWN_HELPER") == "1":
		runProtocolExtensionIgnoreShutdownHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_STDERR_DIAGNOSTIC_HELPER") == "1":
		runProtocolExtensionStderrDiagnosticHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_CRASH_AFTER_EVENT_HELPER") == "1":
		runProtocolExtensionCrashAfterEventHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_HEADER_HELPER") == "1":
		runProtocolExtensionHeaderHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_FOOTER_HELPER") == "1":
		runProtocolExtensionFooterHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_OVERLAY_HELPER") == "1":
		runProtocolExtensionOverlayHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_LIFECYCLE_HELPER") == "1":
		runProtocolExtensionLifecycleHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_COMMAND_HELPER") == "1":
		runProtocolExtensionCommandHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_TOOL_HELPER") == "1":
		runProtocolExtensionToolHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_PROVIDER_HELPER") == "1":
		runProtocolExtensionProviderHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_MESSAGE_RENDERER_HELPER") == "1":
		runProtocolExtensionMessageRendererHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_INTEGRATED_WORKFLOW_HELPER") == "1":
		runProtocolExtensionIntegratedWorkflowHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_TOOL_RENDERER_E2E_HELPER") == "1":
		runProtocolExtensionToolRendererE2EHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_TOOL_RENDERER_HELPER") == "1":
		runProtocolExtensionToolRendererHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_AUTOCOMPLETE_HELPER") == "1":
		runProtocolExtensionAutocompleteHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_EDITOR_HELPER") == "1":
		runProtocolExtensionEditorHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_TITLE_HELPER") == "1":
		runProtocolExtensionTitleHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_WORKING_HELPER") == "1":
		runProtocolExtensionWorkingHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_THINKING_LABEL_HELPER") == "1":
		runProtocolExtensionThinkingLabelHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_TUI_STATE_EXIT_HELPER") == "1":
		runProtocolExtensionTUIStateExitHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_THEME_HELPER") == "1":
		runProtocolExtensionThemeHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_TOOLS_EXPANDED_HELPER") == "1":
		runProtocolExtensionToolsExpandedHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_TERMINAL_INPUT_HELPER") == "1":
		runProtocolExtensionTerminalInputHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_BASH_INTERCEPT_HELPER") == "1":
		runProtocolExtensionBashInterceptHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_RESOURCES_DISCOVER_HELPER") == "1":
		runProtocolExtensionResourcesDiscoverHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_BAD_RESOURCES_DISCOVER_HELPER") == "1":
		runProtocolExtensionBadResourcesDiscoverHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_TICK_HELPER") == "1":
		runProtocolExtensionTickHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_RESIZE_HELPER") == "1":
		runProtocolExtensionResizeHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_THEME_EVENT_HELPER") == "1":
		runProtocolExtensionThemeEventHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_VISIBILITY_HELPER") == "1":
		runProtocolExtensionVisibilityHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_SEMANTIC_INPUT_HELPER") == "1":
		runProtocolExtensionSemanticInputHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_SHORTCUT_HELPER") == "1":
		runProtocolExtensionShortcutHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_FLAG_HELPER") == "1":
		runProtocolExtensionFlagHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_STRING_FLAG_HELPER") == "1":
		runProtocolExtensionStringFlagHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_DIALOG_HELPER") == "1":
		runProtocolExtensionDialogHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_SHUTDOWN_NOTIFY_HELPER") == "1":
		runProtocolExtensionShutdownNotifyHelper()
		os.Exit(0)
	case os.Getenv("GI_EXTENSION_VIEWTREE_EVENT_HELPER") == "1":
		runProtocolExtensionViewTreeEventHelper()
		os.Exit(0)
	default:
		return
	}
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"todo-widget","version":"1.0.0"},"requestedCapabilities":["tui.widget","session.read"]}`)

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.widget") || containsString(helloResult.GrantedCapabilities, "session.read") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"mount_1","method":"host.tui.mount","params":{"mountId":"todo.current","slot":"widget.aboveEditor","view":{"type":"box","children":[{"type":"text","text":"Process Widget"},{"type":"list","items":[{"id":"1","text":"Started from package process","checked":false}]}]}}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func runProtocolExtensionCrashAfterEventHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"crashy-extension","version":"1.0.0"},"requestedCapabilities":[]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionStart {
			fmt.Fprintln(os.Stderr, "crashing after event")
			os.Exit(7)
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func runProtocolExtensionStderrDiagnosticHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"noisy-extension","version":"1.0.0"},"requestedCapabilities":[]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "diagnostic from stderr")
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func runProtocolExtensionShutdownNotifyHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"shutdown-notify","version":"1.0.0"},"requestedCapabilities":["tui.dialog"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"shutdown_notify","method":"host.tui.dialog","params":{"kind":"notify","message":"Shutdown cleanup notification"}}`)
			if marker := os.Getenv("GI_EXTENSION_SHUTDOWN_MARKER"); marker != "" {
				_ = os.WriteFile(marker, []byte("shutdown"), 0o644)
			}
			time.Sleep(100 * time.Millisecond)
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func runProtocolExtensionExitAfterMountHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"ephemeral-widget","version":"1.0.0"},"requestedCapabilities":["tui.widget"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.widget") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"mount_ephemeral","method":"host.tui.mount","params":{"mountId":"ephemeral.widget","slot":"widget.aboveEditor","view":{"type":"text","text":"Ephemeral Widget"}}}`)
	time.Sleep(200 * time.Millisecond)
}

func runProtocolExtensionStatusHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"status-widget","version":"1.0.0"},"requestedCapabilities":["tui.status"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.status") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"status_set","method":"host.tui.status","params":{"key":"process-status","text":"Process busy","priority":80}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func runProtocolExtensionFailBeforeHelloHelper() {
	fmt.Fprintln(os.Stderr, "boom before hello")
	os.Exit(9)
}

func runProtocolExtensionHangBeforeHelloHelper() {
	fmt.Fprintln(os.Stderr, "warming before hello")
	time.Sleep(10 * time.Second)
}

func runProtocolExtensionIgnoreShutdownHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"stuck-extension","version":"1.0.0"},"requestedCapabilities":["tui.widget"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "ignoring shutdown")
	for {
		if !scanner.Scan() {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func runProtocolExtensionHeaderHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"header-widget","version":"1.0.0"},"requestedCapabilities":["tui.header"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.header") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"mount_header","method":"host.tui.mount","params":{"mountId":"header.status","slot":"header","view":{"type":"text","text":"Process Header"}}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func runProtocolExtensionFooterHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"footer-widget","version":"1.0.0"},"requestedCapabilities":["tui.footer"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.footer") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"mount_footer","method":"host.tui.mount","params":{"mountId":"footer.status","slot":"footer","view":{"type":"text","text":"Process Footer"}}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func runProtocolExtensionOverlayHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"overlay-widget","version":"1.0.0"},"requestedCapabilities":["tui.overlay"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.overlay") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"mount_overlay","method":"host.tui.mount","params":{"mountId":"overlay.status","slot":"overlay","view":{"type":"text","text":"Process Overlay"}}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func runProtocolExtensionViewTreeEventHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"event-editor","version":"1.0.0"},"requestedCapabilities":["tui.editor"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.editor") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"mount_event_editor","method":"host.tui.mount","params":{"mountId":"event.editor","slot":"editor","view":{"type":"textarea","id":"event-root","text":"Process event editor"}}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == "tui.event" {
			event, _ := envelope.Params["event"].(string)
			if event != "key" {
				continue
			}
			data, _ := envelope.Params["data"].(map[string]any)
			key, _ := data["key"].(string)
			response, _ := json.Marshal(map[string]any{
				"type":     "request",
				"protocol": "gi-ext-rpc@1",
				"id":       "patch_event_editor",
				"method":   "host.tui.patch",
				"params": map[string]any{
					"mountId": "event.editor",
					"ops": []map[string]any{{
						"op":    "replace",
						"path":  "/text",
						"value": "Process key: " + key,
					}},
				},
			})
			fmt.Println(string(response))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionEditorHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"editor-extension","version":"1.0.0"},"requestedCapabilities":["tui.editor"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.editor") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"editor_set","method":"host.tui.editor","params":{"action":"set","text":"Package"}}`)
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"editor_insert","method":"host.tui.editor","params":{"action":"insert","text":" plan"}}`)
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"editor_paste","method":"host.tui.editor","params":{"action":"paste","text":" pasted"}}`)
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"editor_submit","method":"host.tui.editor","params":{"action":"submit"}}`)
	for scanner.Scan() {
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(scanner.Text()), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionTitleHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"title-extension","version":"1.0.0"},"requestedCapabilities":["tui.title"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.title") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"title_set","method":"host.tui.title","params":{"title":"Package title"}}`)
	for scanner.Scan() {
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(scanner.Text()), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionWorkingHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"working-extension","version":"1.0.0"},"requestedCapabilities":["tui.working"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.working") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"working_set","method":"host.tui.working","params":{"message":"Package busy","visible":true,"indicator":{"frames":["."],"intervalMs":25}}}`)
	for scanner.Scan() {
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(scanner.Text()), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionThinkingLabelHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"thinking-label-extension","version":"1.0.0"},"requestedCapabilities":["tui.thinking_label"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.thinking_label") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"thinking_label_set","method":"host.tui.thinking_label","params":{"label":"Reasoning hidden"}}`)
	for scanner.Scan() {
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(scanner.Text()), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionTUIStateExitHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"tui-state-extension","version":"1.0.0"},"requestedCapabilities":["tui.title","tui.working","tui.thinking_label"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil ||
		helloResult.Type != "hello_result" ||
		!containsString(helloResult.GrantedCapabilities, "tui.title") ||
		!containsString(helloResult.GrantedCapabilities, "tui.working") ||
		!containsString(helloResult.GrantedCapabilities, "tui.thinking_label") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"title_set","method":"host.tui.title","params":{"title":"Package title"}}`)
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"working_set","method":"host.tui.working","params":{"message":"Package busy","visible":true,"indicator":{"frames":["."],"intervalMs":25}}}`)
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"thinking_label_set","method":"host.tui.thinking_label","params":{"label":"Reasoning hidden"}}`)
	seen := map[string]bool{}
	for scanner.Scan() {
		var envelope struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		_ = json.Unmarshal([]byte(scanner.Text()), &envelope)
		if envelope.Type == "response" {
			seen[envelope.ID] = true
			if seen["title_set"] && seen["working_set"] && seen["thinking_label_set"] {
				time.Sleep(100 * time.Millisecond)
				os.Exit(7)
			}
		}
	}
	os.Exit(7)
}

func runProtocolExtensionThemeHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"theme-extension","version":"1.0.0"},"requestedCapabilities":["tui.theme"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.theme") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"theme_current","method":"host.tui.theme","params":{"action":"current"}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Result map[string]any `json:"result"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "response" && envelope.ID == "theme_current" {
			themes, _ := envelope.Result["themes"].([]any)
			if len(themes) == 0 {
				os.Exit(4)
			}
			fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"theme_set","method":"host.tui.theme","params":{"action":"set","name":"focus"}}`)
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionToolsExpandedHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"tools-expanded-extension","version":"1.0.0"},"requestedCapabilities":["tui.tools_expanded"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.tools_expanded") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"tools_expanded_set","method":"host.tui.tools_expanded","params":{"expanded":true}}`)
	for scanner.Scan() {
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(scanner.Text()), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionTerminalInputHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"terminal-input-extension","version":"1.0.0"},"requestedCapabilities":["tui.terminal_input","tui.widget"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil ||
		helloResult.Type != "hello_result" ||
		!containsString(helloResult.GrantedCapabilities, "tui.terminal_input") ||
		!containsString(helloResult.GrantedCapabilities, "tui.widget") {
		os.Exit(3)
	}
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == "tui.terminal_input" {
			data, _ := envelope.Params["data"].(string)
			request, _ := json.Marshal(map[string]any{
				"type":     "request",
				"protocol": "gi-ext-rpc@1",
				"id":       "mount_terminal_input",
				"method":   "host.tui.mount",
				"params": map[string]any{
					"mountId": "terminal.input",
					"slot":    "widget.aboveEditor",
					"view": map[string]any{
						"type": "text",
						"text": "Terminal input: " + data,
					},
				},
			})
			fmt.Println(string(request))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionBashInterceptHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"bash-intercept-extension","version":"1.0.0"},"requestedCapabilities":["bash.intercept"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil ||
		helloResult.Type != "hello_result" ||
		!containsString(helloResult.GrantedCapabilities, CapabilityBashIntercept) {
		os.Exit(3)
	}
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventUserBash {
			command, _ := envelope.Params["command"].(string)
			cwd, _ := envelope.Params["cwd"].(string)
			excluded, _ := envelope.Params["excludeFromContext"].(bool)
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"bashResultSet": true,
					"bashResult": map[string]any{
						"output":   fmt.Sprintf("process bash: %s @ %s excluded=%t\n", command, cwd, excluded),
						"exitCode": 0,
					},
				},
			})
			fmt.Println(string(response))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionResourcesDiscoverHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"resources-extension","version":"1.0.0"},"requestedCapabilities":["resources.discover"]}`)
	expectedCWD := os.Getenv("GI_EXTENSION_RESOURCES_EXPECT_CWD")
	if expectedCWD == "" {
		expectedCWD = "/tmp/project"
	}
	expectedReasons := splitNonEmptyCSV(os.Getenv("GI_EXTENSION_RESOURCES_EXPECT_REASONS"))
	if len(expectedReasons) == 0 {
		expectedReason := os.Getenv("GI_EXTENSION_RESOURCES_EXPECT_REASON")
		if expectedReason == "" {
			expectedReason = "startup"
		}
		expectedReasons = []string{expectedReason}
	}
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil ||
		helloResult.Type != "hello_result" ||
		!containsString(helloResult.GrantedCapabilities, CapabilityResourcesDiscover) {
		os.Exit(3)
	}
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionStart {
			reason, _ := envelope.Params["reason"].(string)
			if !containsString(expectedReasons, reason) {
				os.Exit(5)
			}
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventResourcesDiscover {
			reason, _ := envelope.Params["reason"].(string)
			if !containsString(expectedReasons, reason) || envelope.Params["cwd"] != expectedCWD {
				os.Exit(4)
			}
			skillPath := "skills/dynamic-skill"
			promptPath := "prompts/dynamic.md"
			themePath := "themes/dynamic.json"
			if os.Getenv("GI_EXTENSION_RESOURCES_BY_REASON") == "1" {
				resourceName := protocolResourceNameForReason(reason)
				skillPath = "skills/" + resourceName
				promptPath = "prompts/" + resourceName + ".md"
				themePath = "themes/" + resourceName + ".json"
			}
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"resourcesSet": true,
					"resources": map[string]any{
						"skillPaths":  []map[string]any{{"path": skillPath}},
						"promptPaths": []map[string]any{{"path": promptPath}},
						"themePaths":  []map[string]any{{"path": themePath}},
					},
				},
			})
			fmt.Println(string(response))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func splitNonEmptyCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func protocolResourceNameForReason(reason string) string {
	if reason == "" {
		return "dynamic-resource"
	}
	var builder strings.Builder
	builder.WriteString("dynamic-")
	for _, r := range reason {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteRune('-')
	}
	return strings.ToLower(builder.String())
}

func runProtocolExtensionBadResourcesDiscoverHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"bad-resources-extension","version":"1.0.0"},"requestedCapabilities":["resources.discover"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil ||
		helloResult.Type != "hello_result" ||
		!containsString(helloResult.GrantedCapabilities, CapabilityResourcesDiscover) {
		os.Exit(3)
	}
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string `json:"type"`
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventResourcesDiscover {
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"resourcesSet": true,
					"resources":    "not-an-object",
				},
			})
			fmt.Println(string(response))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionTickHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"tick-extension","version":"1.0.0"},"requestedCapabilities":["tui.widget"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil ||
		helloResult.Type != "hello_result" ||
		!containsString(helloResult.GrantedCapabilities, CapabilityTUIWidget) {
		os.Exit(3)
	}
	mountRequest, _ := json.Marshal(map[string]any{
		"type":     "request",
		"protocol": "gi-ext-rpc@1",
		"id":       "mount_tick",
		"method":   "host.tui.mount",
		"params": map[string]any{
			"mountId": "tick.widget",
			"slot":    "widget.aboveEditor",
			"view": map[string]any{
				"type":   "text",
				"id":     "tick-root",
				"text":   "Tick 0",
				"events": []string{"tick"},
			},
		},
	})
	fmt.Println(string(mountRequest))
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == "tui.event" {
			event, _ := envelope.Params["event"].(string)
			if event != "tick" {
				continue
			}
			data, _ := envelope.Params["data"].(map[string]any)
			frame, _ := data["frame"].(float64)
			patchRequest, _ := json.Marshal(map[string]any{
				"type":     "request",
				"protocol": "gi-ext-rpc@1",
				"id":       fmt.Sprintf("patch_tick_%d", int(frame)),
				"method":   "host.tui.patch",
				"params": map[string]any{
					"mountId": "tick.widget",
					"ops": []map[string]any{{
						"op":    "replace",
						"path":  "/text",
						"value": fmt.Sprintf("Ticked %d", int(frame)),
					}},
				},
			})
			fmt.Println(string(patchRequest))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionResizeHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"resize-extension","version":"1.0.0"},"requestedCapabilities":["tui.widget"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil ||
		helloResult.Type != "hello_result" ||
		!containsString(helloResult.GrantedCapabilities, CapabilityTUIWidget) {
		os.Exit(3)
	}
	mountRequest, _ := json.Marshal(map[string]any{
		"type":     "request",
		"protocol": "gi-ext-rpc@1",
		"id":       "mount_resize",
		"method":   "host.tui.mount",
		"params": map[string]any{
			"mountId": "resize.widget",
			"slot":    "widget.aboveEditor",
			"view": map[string]any{
				"type":   "text",
				"id":     "resize-root",
				"text":   "Size 0x0",
				"events": []string{"resize"},
			},
		},
	})
	fmt.Println(string(mountRequest))
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == "tui.event" {
			event, _ := envelope.Params["event"].(string)
			if event != "resize" {
				continue
			}
			data, _ := envelope.Params["data"].(map[string]any)
			width, _ := data["width"].(float64)
			height, _ := data["height"].(float64)
			patchRequest, _ := json.Marshal(map[string]any{
				"type":     "request",
				"protocol": "gi-ext-rpc@1",
				"id":       fmt.Sprintf("patch_resize_%dx%d", int(width), int(height)),
				"method":   "host.tui.patch",
				"params": map[string]any{
					"mountId": "resize.widget",
					"ops": []map[string]any{{
						"op":    "replace",
						"path":  "/text",
						"value": fmt.Sprintf("Size %dx%d", int(width), int(height)),
					}},
				},
			})
			fmt.Println(string(patchRequest))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionThemeEventHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"theme-event-extension","version":"1.0.0"},"requestedCapabilities":["tui.widget"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil ||
		helloResult.Type != "hello_result" ||
		!containsString(helloResult.GrantedCapabilities, CapabilityTUIWidget) {
		os.Exit(3)
	}
	mountRequest, _ := json.Marshal(map[string]any{
		"type":     "request",
		"protocol": "gi-ext-rpc@1",
		"id":       "mount_theme_event",
		"method":   "host.tui.mount",
		"params": map[string]any{
			"mountId": "theme-event.widget",
			"slot":    "widget.aboveEditor",
			"view": map[string]any{
				"type":   "text",
				"id":     "theme-event-root",
				"text":   "Theme dark",
				"events": []string{"theme_change"},
			},
		},
	})
	fmt.Println(string(mountRequest))
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == "tui.event" {
			event, _ := envelope.Params["event"].(string)
			if event != "theme_change" {
				continue
			}
			data, _ := envelope.Params["data"].(map[string]any)
			name, _ := data["name"].(string)
			patchRequest, _ := json.Marshal(map[string]any{
				"type":     "request",
				"protocol": "gi-ext-rpc@1",
				"id":       "patch_theme_" + name,
				"method":   "host.tui.patch",
				"params": map[string]any{
					"mountId": "theme-event.widget",
					"ops": []map[string]any{{
						"op":    "replace",
						"path":  "/text",
						"value": "Theme " + name,
					}},
				},
			})
			fmt.Println(string(patchRequest))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionVisibilityHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"visibility-extension","version":"1.0.0"},"requestedCapabilities":["tui.widget"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil ||
		helloResult.Type != "hello_result" ||
		!containsString(helloResult.GrantedCapabilities, CapabilityTUIWidget) {
		os.Exit(3)
	}
	mountRequest, _ := json.Marshal(map[string]any{
		"type":     "request",
		"protocol": "gi-ext-rpc@1",
		"id":       "mount_visibility",
		"method":   "host.tui.mount",
		"params": map[string]any{
			"mountId": "visibility.widget",
			"slot":    "widget.aboveEditor",
			"view": map[string]any{
				"type":   "text",
				"id":     "visibility-root",
				"text":   "Visible unknown",
				"events": []string{"visibility_change"},
			},
		},
	})
	fmt.Println(string(mountRequest))
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == "tui.event" {
			event, _ := envelope.Params["event"].(string)
			if event != "visibility_change" {
				continue
			}
			data, _ := envelope.Params["data"].(map[string]any)
			visible, _ := data["visible"].(bool)
			patchRequest, _ := json.Marshal(map[string]any{
				"type":     "request",
				"protocol": "gi-ext-rpc@1",
				"id":       "patch_visibility",
				"method":   "host.tui.patch",
				"params": map[string]any{
					"mountId": "visibility.widget",
					"ops": []map[string]any{{
						"op":    "replace",
						"path":  "/text",
						"value": fmt.Sprintf("Visible %v", visible),
					}},
				},
			})
			fmt.Println(string(patchRequest))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionSemanticInputHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"semantic-input-extension","version":"1.0.0"},"requestedCapabilities":["tui.editor"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil ||
		helloResult.Type != "hello_result" ||
		!containsString(helloResult.GrantedCapabilities, CapabilityTUIEditor) {
		os.Exit(3)
	}
	mountRequest, _ := json.Marshal(map[string]any{
		"type":     "request",
		"protocol": "gi-ext-rpc@1",
		"id":       "mount_semantic_input",
		"method":   "host.tui.mount",
		"params": map[string]any{
			"mountId": "semantic.editor",
			"slot":    "editor",
			"view": map[string]any{
				"type": "textarea",
				"id":   "semantic-root",
				"text": "Semantic ready",
			},
		},
	})
	fmt.Println(string(mountRequest))
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == "tui.event" {
			event, _ := envelope.Params["event"].(string)
			data, _ := envelope.Params["data"].(map[string]any)
			text := ""
			switch event {
			case "textInput":
				value, _ := data["text"].(string)
				text = "Text " + value
			case "submit":
				text = "Submitted"
			case "cancel":
				text = "Cancelled"
			default:
				continue
			}
			patchRequest, _ := json.Marshal(map[string]any{
				"type":     "request",
				"protocol": "gi-ext-rpc@1",
				"id":       "patch_semantic_" + event,
				"method":   "host.tui.patch",
				"params": map[string]any{
					"mountId": "semantic.editor",
					"ops": []map[string]any{{
						"op":    "replace",
						"path":  "/text",
						"value": text,
					}},
				},
			})
			fmt.Println(string(patchRequest))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionDialogHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"dialog-extension","version":"1.0.0"},"requestedCapabilities":["tui.dialog","tui.widget"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil ||
		helloResult.Type != "hello_result" ||
		!containsString(helloResult.GrantedCapabilities, "tui.dialog") ||
		!containsString(helloResult.GrantedCapabilities, "tui.widget") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"dialog_select","method":"host.tui.dialog","params":{"kind":"select","title":"Pick from process","options":[{"id":"alpha","label":"Alpha","value":"alpha"},{"id":"beta","label":"Beta","value":"beta"}]}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Result map[string]any `json:"result"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "response" && envelope.ID == "dialog_select" {
			value, _ := envelope.Result["value"].(string)
			request, _ := json.Marshal(map[string]any{
				"type":     "request",
				"protocol": "gi-ext-rpc@1",
				"id":       "mount_dialog_result",
				"method":   "host.tui.mount",
				"params": map[string]any{
					"mountId": "dialog.result",
					"slot":    "widget.aboveEditor",
					"view": map[string]any{
						"type": "text",
						"text": "Dialog selected: " + value,
					},
				},
			})
			fmt.Println(string(request))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionLifecycleHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"lifecycle-widget","version":"1.0.0"},"requestedCapabilities":["tui.widget"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.widget") {
		os.Exit(3)
	}
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionStart {
			reason, _ := envelope.Params["reason"].(string)
			request, _ := json.Marshal(map[string]any{
				"type":     "request",
				"protocol": "gi-ext-rpc@1",
				"id":       "mount_lifecycle",
				"method":   "host.tui.mount",
				"params": map[string]any{
					"mountId": "lifecycle.start",
					"slot":    "widget.aboveEditor",
					"view": map[string]any{
						"type": "text",
						"text": "Lifecycle: " + reason,
					},
				},
			})
			fmt.Println(string(request))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionSwitch {
			reason, _ := envelope.Params["reason"].(string)
			request, _ := json.Marshal(map[string]any{
				"type":     "request",
				"protocol": "gi-ext-rpc@1",
				"id":       "mount_lifecycle_switch",
				"method":   "host.tui.mount",
				"params": map[string]any{
					"mountId": "lifecycle.switch",
					"slot":    "widget.aboveEditor",
					"view": map[string]any{
						"type": "text",
						"text": "Lifecycle switch: " + reason,
					},
				},
			})
			fmt.Println(string(request))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionProviderHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"provider-pack","version":"1.0.0"},"requestedCapabilities":["providers.register"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "providers.register") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_provider","method":"register_provider","params":{"name":"rpc-provider","baseUrl":"https://rpc-provider.test/v1","apiKey":"RPC_KEY","api":"openai-completions","models":[{"id":"rpc-model","name":"RPC Model","reasoning":false,"input":["text"],"contextWindow":1000,"maxTokens":100}]}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionMessageRendererHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"message-renderer","version":"1.0.0"},"requestedCapabilities":["tui.message_renderer"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.message_renderer") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_message_renderer","method":"register_message_renderer","params":{"type":"rpc.message"}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == "message.render" {
			message, _ := envelope.Params["message"].(map[string]any)
			text := rpcMessageRendererText(message)
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"lines": []string{"Message render: " + text},
				},
			})
			fmt.Println(string(response))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func rpcMessageRendererText(message map[string]any) string {
	if text, _ := message["text"].(string); text != "" {
		return text
	}
	content, _ := message["content"].([]any)
	for _, part := range content {
		block, _ := part.(map[string]any)
		if text, _ := block["text"].(string); text != "" {
			return text
		}
	}
	return ""
}

func runProtocolExtensionToolRendererHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"tool-renderer","version":"1.0.0"},"requestedCapabilities":["tui.tool_renderer"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.tool_renderer") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_tool_renderer","method":"register_tool_renderer","params":{"name":"rendered_tool"}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		switch {
		case envelope.Type == "event" && envelope.Method == "tool.render_call":
			args, _ := envelope.Params["args"].(map[string]any)
			foo, _ := args["foo"].(string)
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"lines": []string{"Tool call render: " + foo},
				},
			})
			fmt.Println(string(response))
		case envelope.Type == "event" && envelope.Method == "tool.render_result":
			result, _ := envelope.Params["result"].(map[string]any)
			text, _ := result["Text"].(string)
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"view": map[string]any{"type": "text", "text": "Tool result render: " + text},
				},
			})
			fmt.Println(string(response))
		case envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown:
			os.Exit(0)
		}
	}
}

func runProtocolExtensionToolRendererE2EHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"tool-renderer-e2e","version":"1.0.0"},"requestedCapabilities":["tools.register","tui.tool_renderer"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil ||
		helloResult.Type != "hello_result" ||
		!containsString(helloResult.GrantedCapabilities, "tools.register") ||
		!containsString(helloResult.GrantedCapabilities, "tui.tool_renderer") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_rendered_tool","method":"register_tool","params":{"name":"rendered_tool","label":"Rendered Tool","description":"Rendered process tool","promptSnippet":"Rendered process tool"}}`)
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_rendered_tool_renderer","method":"register_tool_renderer","params":{"name":"rendered_tool"}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		switch {
		case envelope.Type == "event" && envelope.Method == "tool.invoke":
			input, _ := envelope.Params["input"].(map[string]any)
			foo, _ := input["foo"].(string)
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "process rendered result: " + foo}},
				},
			})
			fmt.Println(string(response))
		case envelope.Type == "event" && envelope.Method == "tool.render_call":
			args, _ := envelope.Params["args"].(map[string]any)
			foo, _ := args["foo"].(string)
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"lines": []string{"Tool call render: " + foo},
				},
			})
			fmt.Println(string(response))
		case envelope.Type == "event" && envelope.Method == "tool.render_result":
			result, _ := envelope.Params["result"].(map[string]any)
			text, _ := result["Text"].(string)
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"view": map[string]any{"type": "text", "text": "Tool result render: " + text},
				},
			})
			fmt.Println(string(response))
		case envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown:
			os.Exit(0)
		}
	}
}

func runProtocolExtensionIntegratedWorkflowHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"integrated-package","version":"1.0.0"},"requestedCapabilities":["commands.register","shortcuts.register","tools.register","tui.autocomplete","tui.dialog","tui.editor","tui.message_renderer","tui.tool_renderer","tui.widget"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	required := []string{
		"commands.register",
		"shortcuts.register",
		"tools.register",
		"tui.autocomplete",
		"tui.dialog",
		"tui.editor",
		"tui.message_renderer",
		"tui.tool_renderer",
		"tui.widget",
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" {
		os.Exit(3)
	}
	for _, capability := range required {
		if !containsString(helloResult.GrantedCapabilities, capability) {
			os.Exit(3)
		}
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_integrated_command","method":"register_command","params":{"name":"integrate","description":"Run integrated package workflow"}}`)
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_integrated_shortcut","method":"register_shortcut","params":{"key":"ctrl+y","description":"Run integrated shortcut"}}`)
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_integrated_autocomplete","method":"register_autocomplete_provider","params":{"id":"integrated-issues","description":"Integrated issue autocomplete","priority":60}}`)
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_integrated_message_renderer","method":"register_message_renderer","params":{"type":"rpc.integrated"}}`)
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_integrated_tool","method":"register_tool","params":{"name":"integrated_tool","label":"Integrated Tool","description":"Integrated process tool","promptSnippet":"Integrated process tool"}}`)
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_integrated_tool_renderer","method":"register_tool_renderer","params":{"name":"integrated_tool"}}`)
	dialogOpened := false
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
			Result map[string]any `json:"result"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		switch {
		case envelope.Type == "event" && envelope.Method == ProtocolEventSessionStart:
			reason, _ := envelope.Params["reason"].(string)
			request, _ := json.Marshal(map[string]any{
				"type":     "request",
				"protocol": "gi-ext-rpc@1",
				"id":       "mount_integrated_lifecycle",
				"method":   "host.tui.mount",
				"params": map[string]any{
					"mountId": "integrated.lifecycle",
					"slot":    "widget.aboveEditor",
					"view": map[string]any{
						"type": "text",
						"text": "Integrated lifecycle: " + reason,
					},
				},
			})
			fmt.Println(string(request))
			if !dialogOpened {
				dialogOpened = true
				fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"integrated_dialog","method":"host.tui.dialog","params":{"kind":"select","title":"Integrated choice","options":[{"id":"alpha","label":"Alpha","value":"alpha"},{"id":"beta","label":"Beta","value":"beta"}]}}`)
			}
		case envelope.Type == "response" && envelope.ID == "integrated_dialog":
			value, _ := envelope.Result["value"].(string)
			fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"integrated_editor_set","method":"host.tui.editor","params":{"action":"set","text":"Integrated draft"}}`)
			fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"integrated_editor_insert","method":"host.tui.editor","params":{"action":"insert","text":" ready"}}`)
			request, _ := json.Marshal(map[string]any{
				"type":     "request",
				"protocol": "gi-ext-rpc@1",
				"id":       "mount_integrated_dialog",
				"method":   "host.tui.mount",
				"params": map[string]any{
					"mountId": "integrated.dialog",
					"slot":    "widget.aboveEditor",
					"view": map[string]any{
						"type": "text",
						"text": "Integrated dialog: " + value,
					},
				},
			})
			fmt.Println(string(request))
		case envelope.Type == "event" && envelope.Method == "command.invoke":
			args, _ := envelope.Params["args"].(string)
			request, _ := json.Marshal(map[string]any{
				"type":     "request",
				"protocol": "gi-ext-rpc@1",
				"id":       "mount_integrated_command",
				"method":   "host.tui.mount",
				"params": map[string]any{
					"mountId": "integrated.command",
					"slot":    "widget.aboveEditor",
					"view": map[string]any{
						"type": "text",
						"text": "Integrated command: " + args,
					},
				},
			})
			fmt.Println(string(request))
		case envelope.Type == "event" && envelope.Method == "shortcut.invoke":
			key, _ := envelope.Params["key"].(string)
			request, _ := json.Marshal(map[string]any{
				"type":     "request",
				"protocol": "gi-ext-rpc@1",
				"id":       "mount_integrated_shortcut",
				"method":   "host.tui.mount",
				"params": map[string]any{
					"mountId": "integrated.shortcut",
					"slot":    "widget.aboveEditor",
					"view": map[string]any{
						"type": "text",
						"text": "Integrated shortcut: " + key,
					},
				},
			})
			fmt.Println(string(request))
		case envelope.Type == "event" && envelope.Method == "autocomplete.suggest":
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"prefix": "#90",
					"start":  4,
					"end":    7,
					"items": []map[string]any{{
						"id":          "issue-900",
						"value":       "#900",
						"label":       "#900",
						"description": "Integrated suggestion",
					}},
				},
			})
			fmt.Println(string(response))
		case envelope.Type == "event" && envelope.Method == "message.render":
			message, _ := envelope.Params["message"].(map[string]any)
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"lines": []string{"Integrated message: " + rpcMessageRendererText(message)},
				},
			})
			fmt.Println(string(response))
		case envelope.Type == "event" && envelope.Method == "tool.invoke":
			input, _ := envelope.Params["input"].(map[string]any)
			foo, _ := input["foo"].(string)
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "integrated tool result: " + foo}},
				},
			})
			fmt.Println(string(response))
		case envelope.Type == "event" && envelope.Method == "tool.render_call":
			args, _ := envelope.Params["args"].(map[string]any)
			foo, _ := args["foo"].(string)
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"lines": []string{"Integrated tool call: " + foo},
				},
			})
			fmt.Println(string(response))
		case envelope.Type == "event" && envelope.Method == "tool.render_result":
			result, _ := envelope.Params["result"].(map[string]any)
			text, _ := result["Text"].(string)
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"view": map[string]any{"type": "text", "text": "Integrated tool result render: " + text},
				},
			})
			fmt.Println(string(response))
			os.Exit(0)
		case envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown:
			os.Exit(0)
		}
	}
}

func runProtocolExtensionAutocompleteHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"issue-autocomplete","version":"1.0.0"},"requestedCapabilities":["tui.autocomplete"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tui.autocomplete") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_issues","method":"register_autocomplete_provider","params":{"id":"issues","description":"Issue autocomplete","priority":50}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == "autocomplete.suggest" {
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"prefix": "#12",
					"start":  4,
					"end":    7,
					"items": []map[string]any{{
						"id":          "issue-123",
						"value":       "#123",
						"label":       "#123",
						"description": "Process suggestion",
					}},
				},
			})
			fmt.Println(string(response))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionToolHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"toolbox","version":"1.0.0"},"requestedCapabilities":["tools.register"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "tools.register") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_echo","method":"register_tool","params":{"name":"echo_tool","label":"Echo","description":"Echo input through process","promptSnippet":"Echo input"}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == "tool.invoke" {
			input, _ := envelope.Params["input"].(map[string]any)
			text, _ := input["text"].(string)
			response, _ := json.Marshal(map[string]any{
				"type":     "response",
				"protocol": "gi-ext-rpc@1",
				"id":       envelope.ID,
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "process tool: " + text}},
				},
			})
			fmt.Println(string(response))
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionCommandHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"plan-mode","version":"1.0.0"},"requestedCapabilities":["commands.register","tui.widget"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, "commands.register") || !containsString(helloResult.GrantedCapabilities, "tui.widget") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_plan","method":"register_command","params":{"name":"plan","description":"Enter plan mode","argumentHint":"<topic>"}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == "command.invoke" {
			args, _ := envelope.Params["args"].(string)
			fmt.Printf(`{"type":"request","protocol":"gi-ext-rpc@1","id":"mount_plan","method":"host.tui.mount","params":{"mountId":"plan.mode","slot":"widget.aboveEditor","view":{"type":"text","text":"Command invoked: %s"}}}`+"\n", args)
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionShortcutHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1","gi-viewtree@1"],"extension":{"id":"shortcut-extension","version":"1.0.0"},"requestedCapabilities":["shortcuts.register","tui.widget"]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type                string   `json:"type"`
		GrantedCapabilities []string `json:"grantedCapabilities"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" || !containsString(helloResult.GrantedCapabilities, CapabilityShortcutsRegister) || !containsString(helloResult.GrantedCapabilities, "tui.widget") {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_shortcut","method":"register_shortcut","params":{"key":"ctrl+y","description":"Open package shortcut"}}`)
	for scanner.Scan() {
		line := scanner.Text()
		var envelope struct {
			Type   string         `json:"type"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal([]byte(line), &envelope)
		if envelope.Type == "event" && envelope.Method == "shortcut.invoke" {
			key, _ := envelope.Params["key"].(string)
			fmt.Printf(`{"type":"request","protocol":"gi-ext-rpc@1","id":"mount_shortcut","method":"host.tui.mount","params":{"mountId":"shortcut.status","slot":"widget.aboveEditor","view":{"type":"text","text":"Shortcut invoked: %s"}}}`+"\n", key)
			continue
		}
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionFlagHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"flag-extension","version":"1.0.0"},"requestedCapabilities":[]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_flag","method":"register_flag","params":{"name":"--review-mode","description":"Start review mode","type":"boolean","default":true}}`)
	for scanner.Scan() {
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(scanner.Text()), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func runProtocolExtensionStringFlagHelper() {
	fmt.Println(`{"type":"hello","protocols":["gi-ext-rpc@1"],"extension":{"id":"string-flag-extension","version":"1.0.0"},"requestedCapabilities":[]}`)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var helloResult struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(scanner.Text()), &helloResult); err != nil || helloResult.Type != "hello_result" {
		os.Exit(3)
	}
	fmt.Println(`{"type":"request","protocol":"gi-ext-rpc@1","id":"register_flag","method":"register_flag","params":{"name":"--profile","description":"Profile","type":"string","default":"default"}}`)
	for scanner.Scan() {
		var envelope struct {
			Type   string `json:"type"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(scanner.Text()), &envelope)
		if envelope.Type == "event" && envelope.Method == ProtocolEventSessionShutdown {
			os.Exit(0)
		}
	}
}

func waitForProtocolCommand(t *testing.T, runtime *ProtocolExtensionRuntime, name string) *ProtocolCommandRegistration {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if command := runtime.GetCommand(name); command != nil {
			return command
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("command %q was not registered; commands=%#v", name, runtime.CommandInvocationNames())
	return nil
}

func waitForNoProtocolCommand(t *testing.T, runtime *ProtocolExtensionRuntime, name string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if runtime.GetCommand(name) == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("command %q still registered; commands=%#v", name, runtime.CommandInvocationNames())
}

func waitForProtocolShortcut(t *testing.T, runtime *ProtocolExtensionRuntime, key string) ProtocolShortcutRegistration {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if shortcut, ok := runtime.Shortcuts(DefaultProtocolKeybindings()).Shortcuts[key]; ok {
			return shortcut
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("shortcut %q was not registered", key)
	return ProtocolShortcutRegistration{}
}

func waitForProtocolFlag(t *testing.T, runtime *ProtocolExtensionRuntime, name string) ProtocolFlagRegistration {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, flag := range runtime.Flags() {
			if flag.Name == name {
				return flag
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("flag %q was not registered; flags=%#v", name, runtime.Flags())
	return ProtocolFlagRegistration{}
}

func waitForProtocolTool(t *testing.T, runtime *ProtocolExtensionRuntime, name string) *SDKTool {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, tool := range runtime.RegisteredTools() {
			if tool.Name == name {
				copy := tool
				return &copy
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("tool %q was not registered; tools=%#v", name, runtime.RegisteredTools())
	return nil
}

func waitForAutocompleteProvider(t *testing.T, runtime *ProtocolExtensionRuntime, id string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, provider := range runtime.AutocompleteProviders() {
			if provider.ID == id {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("autocomplete provider %q was not registered; providers=%#v", id, runtime.AutocompleteProviders())
}

func waitForRegisteredModel(t *testing.T, registry *ModelRegistry, provider, modelID string) llm.Model {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if model, ok := registry.Find(provider, modelID); ok {
			return model
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("model %s/%s was not registered", provider, modelID)
	return llm.Model{}
}

func waitForMessageRenderer(t *testing.T, runtime *ProtocolExtensionRuntime, customType string) ProtocolMessageRenderer {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if renderer := runtime.GetMessageRenderer(customType); renderer != nil {
			return renderer
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("message renderer %q was not registered", customType)
	return nil
}

func waitForToolRenderer(t *testing.T, runtime *ProtocolExtensionRuntime, toolName string) *ProtocolToolRendererRegistration {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if renderer := runtime.GetToolRenderer(toolName); renderer != nil {
			return renderer
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("tool renderer %q was not registered", toolName)
	return nil
}

func waitForProcessMountedView(t *testing.T, host *RPCSessionHost, mountID string) []string {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		rendered, err := host.ViewTreeHost.RenderMount(mountID, 80)
		if err == nil && len(rendered) > 0 && strings.TrimSpace(strings.Join(rendered, "\n")) != "" {
			return rendered
		}
		time.Sleep(10 * time.Millisecond)
	}
	rendered, _ := host.ViewTreeHost.RenderMount(mountID, 80)
	return rendered
}

func waitForProcessMountedViewText(t *testing.T, host *RPCSessionHost, mountID, text string) []string {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		rendered, err := host.ViewTreeHost.RenderMount(mountID, 80)
		if err == nil && strings.Contains(strings.Join(rendered, "\n"), text) {
			return rendered
		}
		time.Sleep(10 * time.Millisecond)
	}
	rendered, _ := host.ViewTreeHost.RenderMount(mountID, 80)
	return rendered
}

func waitForProcessMountGone(t *testing.T, host *RPCSessionHost, mountID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if host == nil || host.ViewTreeHost == nil {
			return
		}
		if _, ok := host.ViewTreeHost.Mounted(mountID); !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if host != nil && host.ViewTreeHost != nil {
		if mount, ok := host.ViewTreeHost.Mounted(mountID); ok {
			t.Fatalf("mount %q still present after process stop: %#v", mountID, mount)
		}
	}
}

type protocolProcessTestEditor struct {
	mu        sync.Mutex
	text      string
	submitted int
	pasted    int
}

func (e *protocolProcessTestEditor) ReadEditorText() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.text
}

func (e *protocolProcessTestEditor) SetEditorText(text string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.text = text
}

func (e *protocolProcessTestEditor) InsertEditorText(text string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.text += text
}

func (e *protocolProcessTestEditor) PasteEditorText(text string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.text += text
	e.pasted++
}

func (e *protocolProcessTestEditor) SubmitEditorText() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.submitted++
	return nil
}

func (e *protocolProcessTestEditor) snapshot() (string, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.text, e.submitted
}

func (e *protocolProcessTestEditor) pasteCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pasted
}

func waitForProcessEditorState(t *testing.T, editor *protocolProcessTestEditor, wantText string, wantSubmits int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		text, submitted := editor.snapshot()
		if text == wantText && submitted == wantSubmits {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	text, submitted := editor.snapshot()
	t.Fatalf("editor text=%q submitted=%d, want %q/%d", text, submitted, wantText, wantSubmits)
}

type protocolProcessTestTitleHost struct {
	mu     sync.Mutex
	title  string
	titles []string
}

func (h *protocolProcessTestTitleHost) SetTUITitle(title string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.title = title
	h.titles = append(h.titles, title)
	return nil
}

func (h *protocolProcessTestTitleHost) snapshot() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.title
}

func waitForProcessTitle(t *testing.T, host *protocolProcessTestTitleHost, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := host.snapshot(); got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("title = %q, want %q", host.snapshot(), want)
}

type protocolProcessTestWorkingHost struct {
	mu      sync.Mutex
	updates []TUIWorkingUpdate
}

func (h *protocolProcessTestWorkingHost) SetTUIWorking(update TUIWorkingUpdate) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.updates = append(h.updates, update)
	return nil
}

func (h *protocolProcessTestWorkingHost) snapshot() []TUIWorkingUpdate {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]TUIWorkingUpdate(nil), h.updates...)
}

func waitForProcessWorkingUpdate(t *testing.T, host *protocolProcessTestWorkingHost) TUIWorkingUpdate {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		updates := host.snapshot()
		if len(updates) > 0 {
			return updates[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("working update was not received")
	return TUIWorkingUpdate{}
}

func waitForProcessWorkingReset(t *testing.T, host *protocolProcessTestWorkingHost) TUIWorkingUpdate {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		updates := host.snapshot()
		for _, update := range updates {
			if update.ResetMessage && update.ResetIndicator && update.VisibleSet && update.Visible {
				return update
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("working reset was not received; updates=%#v", host.snapshot())
	return TUIWorkingUpdate{}
}

type protocolProcessTestThinkingLabelHost struct {
	mu     sync.Mutex
	labels []string
}

func (h *protocolProcessTestThinkingLabelHost) SetHiddenThinkingLabel(label string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.labels = append(h.labels, label)
	return nil
}

func (h *protocolProcessTestThinkingLabelHost) snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.labels...)
}

func waitForProcessThinkingLabel(t *testing.T, host *protocolProcessTestThinkingLabelHost, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		labels := host.snapshot()
		if len(labels) > 0 && labels[0] == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("thinking labels = %#v, want %q", host.snapshot(), want)
}

func waitForProcessThinkingLabelReset(t *testing.T, host *protocolProcessTestThinkingLabelHost) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		labels := host.snapshot()
		if len(labels) > 0 && labels[len(labels)-1] == "" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("thinking label reset was not received; labels=%#v", host.snapshot())
}

type protocolProcessTestThemeHost struct {
	mu      sync.Mutex
	current string
	themes  []TUIThemeInfo
}

func (h *protocolProcessTestThemeHost) CurrentTUITheme() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.current
}

func (h *protocolProcessTestThemeHost) AvailableTUIThemes() []TUIThemeInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]TUIThemeInfo(nil), h.themes...)
}

func (h *protocolProcessTestThemeHost) SetTUITheme(name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.current = name
	return nil
}

func waitForProcessTheme(t *testing.T, host *protocolProcessTestThemeHost, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := host.CurrentTUITheme(); got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("theme = %q, want %q", host.CurrentTUITheme(), want)
}

type protocolProcessTestToolExpansionHost struct {
	mu       sync.Mutex
	expanded bool
}

func (h *protocolProcessTestToolExpansionHost) TUIToolsExpanded() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.expanded
}

func (h *protocolProcessTestToolExpansionHost) SetTUIToolsExpanded(expanded bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.expanded = expanded
	return nil
}

func waitForProcessToolsExpanded(t *testing.T, host *protocolProcessTestToolExpansionHost, want bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := host.TUIToolsExpanded(); got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("tools expanded = %v, want %v", host.TUIToolsExpanded(), want)
}
