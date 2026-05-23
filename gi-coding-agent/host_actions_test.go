package gicodingagent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func TestRPCLineProcessorHandlesViewTreeHostActions(t *testing.T) {
	host := NewRPCSessionHost(nil)
	var lines []string
	processor := RPCLineProcessor{
		Host: host,
		WriteLine: func(line string) {
			lines = append(lines, strings.TrimSpace(line))
		},
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"mount_1","method":"host.tui.mount","params":{"mountId":"todo.current","slot":"widget.aboveEditor","view":{"type":"box","id":"root","children":[{"type":"text","text":"Todos"},{"type":"list","items":[{"id":"1","text":"Port approval gate","checked":false}]}]}}}`)
	assertHostActionResult(t, lines[len(lines)-1], "mount_1", "mounted", true)
	rendered, err := host.ViewTreeHost.RenderMount("todo.current", 80)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rendered, []string{"Todos", "[ ] Port approval gate"}) {
		t.Fatalf("rendered = %#v", rendered)
	}
	if mounts := host.ViewTreeHost.MountsBySlot("widget.aboveEditor"); len(mounts) != 1 || mounts[0].MountID != "todo.current" {
		t.Fatalf("widget.aboveEditor mounts = %#v", mounts)
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"overlay_mount","method":"host.tui.mount","params":{"mountId":"modal","slot":"overlay","priority":7,"overlayOptions":{"anchor":"top-left","width":"50%","minWidth":20,"margin":1,"nonCapturing":true},"view":{"type":"text","id":"modal-root","text":"Overlay"}}}`)
	assertHostActionResult(t, lines[len(lines)-1], "overlay_mount", "mounted", true)
	overlayMount, ok := host.ViewTreeHost.Mounted("modal")
	if !ok || overlayMount.Priority != 7 || overlayMount.Overlay == nil || overlayMount.Overlay.Anchor != "top-left" || !overlayMount.Overlay.Width.Percent || overlayMount.Overlay.MinWidth != 20 || !overlayMount.Overlay.NonCapturing || overlayMount.Overlay.Margin.Top != 1 {
		t.Fatalf("overlay mount = %#v ok=%v", overlayMount, ok)
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"patch_1","method":"host.tui.patch","params":{"mountId":"todo.current","ops":[{"op":"replace","path":"/children/1/items/0/checked","value":true}]}}`)
	assertHostActionResult(t, lines[len(lines)-1], "patch_1", "patched", true)
	rendered, err = host.ViewTreeHost.RenderMount("todo.current", 80)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rendered, []string{"Todos", "[x] Port approval gate"}) {
		t.Fatalf("patched rendered = %#v", rendered)
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"unmount_1","method":"host.tui.unmount","params":{"mountId":"todo.current"}}`)
	assertHostActionResult(t, lines[len(lines)-1], "unmount_1", "unmounted", true)
	if _, ok := host.ViewTreeHost.Mounted("todo.current"); ok {
		t.Fatal("mount should be removed")
	}

	statusHost := &recordingTUIStatusHost{}
	host.TUIStatus = statusHost
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"status_1","method":"host.tui.status","params":{"key":"plan-mode","text":"Plan mode","priority":50}}`)
	assertHostActionResult(t, lines[len(lines)-1], "status_1", "updated", true)
	if len(statusHost.statuses) != 1 || statusHost.statuses["plan-mode"] != "Plan mode" {
		t.Fatalf("statuses = %#v", statusHost.statuses)
	}
	statusMounts := host.ViewTreeHost.MountsBySlot("footer")
	if len(statusMounts) != 1 || statusMounts[0].MountID != "status:plan-mode" {
		t.Fatalf("status mounts = %#v", statusMounts)
	}
	rendered, err = host.ViewTreeHost.RenderMount("status:plan-mode", 80)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rendered, []string{"Plan mode"}) {
		t.Fatalf("status rendered = %#v", rendered)
	}

	titleHost := &recordingTUITitleHost{}
	host.TUITitle = titleHost
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"title_1","method":"host.tui.title","params":{"title":"Package title"}}`)
	var response HostActionResponse
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	result := assertHostActionResponseOK(t, response, "title_1")
	if result["title"] != "Package title" || len(titleHost.titles) != 1 || titleHost.titles[0] != "Package title" {
		t.Fatalf("title result = %#v titles=%#v", result, titleHost.titles)
	}

	workingHost := &recordingTUIWorkingHost{}
	host.TUIWorking = workingHost
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"working_1","method":"host.tui.working","params":{"message":"Package busy","visible":true,"indicator":{"frames":["."],"intervalMs":25}}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	result = assertHostActionResponseOK(t, response, "working_1")
	if result["updated"] != true || len(workingHost.updates) != 1 {
		t.Fatalf("working result = %#v updates=%#v", result, workingHost.updates)
	}
	update := workingHost.updates[0]
	if !update.MessageSet || update.Message != "Package busy" || !update.VisibleSet || !update.Visible ||
		!update.IndicatorSet || !reflect.DeepEqual(update.Indicator.Frames, []string{"."}) || update.Indicator.IntervalMs != 25 {
		t.Fatalf("working update = %#v", update)
	}

	thinkingHost := &recordingTUIThinkingLabelHost{}
	host.TUIThinkingLabel = thinkingHost
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"thinking_label_1","method":"host.tui.thinking_label","params":{"label":"Reasoning hidden"}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	result = assertHostActionResponseOK(t, response, "thinking_label_1")
	if result["label"] != "Reasoning hidden" || len(thinkingHost.labels) != 1 || thinkingHost.labels[0] != "Reasoning hidden" {
		t.Fatalf("thinking label result = %#v labels=%#v", result, thinkingHost.labels)
	}

	themeHost := &recordingTUIThemeHost{
		current: "dark",
		themes:  []TUIThemeInfo{{Name: "dark", Builtin: true}, {Name: "focus", Path: "/tmp/focus.json"}},
	}
	host.TUITheme = themeHost
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"theme_list","method":"host.tui.theme","params":{"action":"list"}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	result = assertHostActionResponseOK(t, response, "theme_list")
	if got, ok := result["themes"].([]any); !ok || len(got) != 2 {
		t.Fatalf("theme list result = %#v", result)
	}
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"theme_get","method":"host.tui.theme","params":{"action":"get","name":"focus"}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	result = assertHostActionResponseOK(t, response, "theme_get")
	if theme, ok := result["theme"].(map[string]any); !ok || theme["name"] != "focus" || theme["path"] != "/tmp/focus.json" {
		t.Fatalf("theme get result = %#v", result)
	}
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"theme_set","method":"host.tui.theme","params":{"action":"set","name":"focus"}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	result = assertHostActionResponseOK(t, response, "theme_set")
	if result["success"] != true || themeHost.current != "focus" || !reflect.DeepEqual(themeHost.setCalls, []string{"focus"}) {
		t.Fatalf("theme set result = %#v host=%#v", result, themeHost)
	}
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"theme_missing","method":"host.tui.theme","params":{"action":"set","name":"missing"}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	result = assertHostActionResponseOK(t, response, "theme_missing")
	if result["success"] != false || themeHost.current != "focus" {
		t.Fatalf("missing theme result = %#v host=%#v", result, themeHost)
	}

	toolExpansionHost := &recordingTUIToolExpansionHost{}
	host.TUIToolExpansion = toolExpansionHost
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"tools_expanded_set","method":"host.tui.tools_expanded","params":{"expanded":true}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	result = assertHostActionResponseOK(t, response, "tools_expanded_set")
	if result["expanded"] != true || !toolExpansionHost.expanded || !reflect.DeepEqual(toolExpansionHost.setCalls, []bool{true}) {
		t.Fatalf("tools expanded result = %#v host=%#v", result, toolExpansionHost)
	}
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"tools_expanded_get","method":"host.tui.tools_expanded","params":{}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	result = assertHostActionResponseOK(t, response, "tools_expanded_get")
	if result["expanded"] != true {
		t.Fatalf("tools expanded get result = %#v", result)
	}
}

func TestRPCLineProcessorReplaysWidgetMountProtocolExample(t *testing.T) {
	host := NewRPCSessionHost(nil)
	var lines []string
	processor := RPCLineProcessor{
		Host: host,
		WriteLine: func(line string) {
			lines = append(lines, strings.TrimSpace(line))
		},
	}

	content, err := os.ReadFile(filepath.Join("..", "protocol", "spec", "examples", "widget-mount.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if !isExtensionToHostExampleLine(t, line) {
			continue
		}
		processor.HandleLine(context.Background(), line)
	}

	if len(lines) != 2 {
		t.Fatalf("host output lines = %#v", lines)
	}
	var helloResult rpcExtensionHelloResult
	if err := json.Unmarshal([]byte(lines[0]), &helloResult); err != nil {
		t.Fatal(err)
	}
	if helloResult.Type != "hello_result" ||
		helloResult.Protocols["gi-ext-rpc"] != "1.0.0" ||
		helloResult.Protocols["gi-viewtree"] != "1.0.0" ||
		!containsString(helloResult.GrantedCapabilities, "tui.widget") {
		t.Fatalf("hello result = %#v", helloResult)
	}
	assertHostActionResult(t, lines[1], "mount_1", "mounted", true)

	rendered, err := host.ViewTreeHost.RenderMount("todo.current", 80)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rendered, []string{"Todos", "[x] Port approval gate", "[ ] Run conformance replay"}) {
		t.Fatalf("rendered after replay = %#v", rendered)
	}
}

func TestRPCLineProcessorDeniesHostActionsOutsideGrantedCapabilities(t *testing.T) {
	host := NewRPCSessionHost(nil)
	var lines []string
	processor := RPCLineProcessor{
		Host:                host,
		AllowedCapabilities: []string{"session.read"},
		EnforceCapabilities: true,
		WriteLine: func(line string) {
			lines = append(lines, strings.TrimSpace(line))
		},
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"mount_1","method":"host.tui.mount","params":{"mountId":"todo.current","slot":"widget.aboveEditor","view":{"type":"text","text":"Denied"}}}`)

	var response HostActionResponse
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "mount_1" || response.Error == nil || response.Error.Code != "missing_capability" {
		t.Fatalf("response = %#v", response)
	}
	if _, ok := host.ViewTreeHost.Mounted("todo.current"); ok {
		t.Fatal("denied host action should not mount ViewTree")
	}

	titleHost := &recordingTUITitleHost{}
	host.TUITitle = titleHost
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"title_1","method":"host.tui.title","params":{"title":"Denied"}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "title_1" || response.Error == nil || response.Error.Code != "missing_capability" {
		t.Fatalf("title response = %#v", response)
	}
	if len(titleHost.titles) != 0 {
		t.Fatalf("denied title action should not mutate title: %#v", titleHost.titles)
	}

	workingHost := &recordingTUIWorkingHost{}
	host.TUIWorking = workingHost
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"working_1","method":"host.tui.working","params":{"message":"Denied"}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "working_1" || response.Error == nil || response.Error.Code != "missing_capability" {
		t.Fatalf("working response = %#v", response)
	}
	if len(workingHost.updates) != 0 {
		t.Fatalf("denied working action should not mutate loader: %#v", workingHost.updates)
	}

	thinkingHost := &recordingTUIThinkingLabelHost{}
	host.TUIThinkingLabel = thinkingHost
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"thinking_label_1","method":"host.tui.thinking_label","params":{"label":"Denied"}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "thinking_label_1" || response.Error == nil || response.Error.Code != "missing_capability" {
		t.Fatalf("thinking label response = %#v", response)
	}
	if len(thinkingHost.labels) != 0 {
		t.Fatalf("denied thinking label action should not mutate label: %#v", thinkingHost.labels)
	}

	themeHost := &recordingTUIThemeHost{current: "dark", themes: []TUIThemeInfo{{Name: "dark", Builtin: true}, {Name: "focus"}}}
	host.TUITheme = themeHost
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"theme_1","method":"host.tui.theme","params":{"action":"set","name":"focus"}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "theme_1" || response.Error == nil || response.Error.Code != "missing_capability" {
		t.Fatalf("theme response = %#v", response)
	}
	if len(themeHost.setCalls) != 0 {
		t.Fatalf("denied theme action should not mutate theme: %#v", themeHost.setCalls)
	}

	toolExpansionHost := &recordingTUIToolExpansionHost{}
	host.TUIToolExpansion = toolExpansionHost
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"tools_expanded_1","method":"host.tui.tools_expanded","params":{"expanded":true}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "tools_expanded_1" || response.Error == nil || response.Error.Code != "missing_capability" {
		t.Fatalf("tools expanded response = %#v", response)
	}
	if len(toolExpansionHost.setCalls) != 0 {
		t.Fatalf("denied tools expansion should not mutate state: %#v", toolExpansionHost.setCalls)
	}
}

func TestRPCLineProcessorUsesSlotSpecificTUICapabilities(t *testing.T) {
	host := NewRPCSessionHost(nil)
	var lines []string
	processor := RPCLineProcessor{
		Host:                host,
		AllowedCapabilities: []string{"tui.footer"},
		EnforceCapabilities: true,
		WriteLine: func(line string) {
			lines = append(lines, strings.TrimSpace(line))
		},
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"footer_mount","method":"host.tui.mount","params":{"mountId":"footer.segment","slot":"footer","view":{"type":"text","text":"Footer"}}}`)
	assertHostActionResult(t, lines[len(lines)-1], "footer_mount", "mounted", true)

	processor.AllowedCapabilities = []string{"tui.widget"}
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"overlay_denied","method":"host.tui.mount","params":{"mountId":"overlay.panel","slot":"overlay","view":{"type":"text","text":"Overlay"}}}`)
	var response HostActionResponse
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "overlay_denied" || response.Error == nil || response.Error.Code != "missing_capability" {
		t.Fatalf("overlay response = %#v", response)
	}

	processor.AllowedCapabilities = []string{"tui.overlay"}
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"overlay_mount","method":"host.tui.mount","params":{"mountId":"overlay.panel","slot":"overlay","view":{"type":"text","text":"Overlay"}}}`)
	assertHostActionResult(t, lines[len(lines)-1], "overlay_mount", "mounted", true)

	processor.AllowedCapabilities = []string{"tui.editor"}
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"editor_mount","method":"host.tui.mount","params":{"mountId":"editor.panel","slot":"editor","view":{"type":"textarea","text":"Editor"}}}`)
	assertHostActionResult(t, lines[len(lines)-1], "editor_mount", "mounted", true)
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"editor_patch","method":"host.tui.patch","params":{"mountId":"editor.panel","ops":[{"op":"replace","path":"/text","value":"Editor patched"}]}}`)
	assertHostActionResult(t, lines[len(lines)-1], "editor_patch", "patched", true)
}

func TestRPCLineProcessorRequiresOwnedViewTreeMountForMountPatchAndUnmount(t *testing.T) {
	host := NewRPCSessionHost(nil)
	var ownerLines []string
	owner := RPCLineProcessor{
		Host:                host,
		AllowedCapabilities: []string{"tui.widget"},
		EnforceCapabilities: true,
		WriteLine: func(line string) {
			ownerLines = append(ownerLines, strings.TrimSpace(line))
		},
	}
	var intruderLines []string
	intruder := RPCLineProcessor{
		Host:                host,
		AllowedCapabilities: []string{"tui.widget"},
		EnforceCapabilities: true,
		WriteLine: func(line string) {
			intruderLines = append(intruderLines, strings.TrimSpace(line))
		},
	}

	owner.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"mount_owner","method":"host.tui.mount","params":{"mountId":"shared.widget","slot":"widget.aboveEditor","view":{"type":"text","text":"Owner"}}}`)
	assertHostActionResult(t, ownerLines[len(ownerLines)-1], "mount_owner", "mounted", true)

	intruder.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"mount_intruder","method":"host.tui.mount","params":{"mountId":"shared.widget","slot":"widget.aboveEditor","view":{"type":"text","text":"Intruder"}}}`)
	var response HostActionResponse
	if err := json.Unmarshal([]byte(intruderLines[len(intruderLines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "mount_intruder" || response.Error == nil || response.Error.Code != "policy_denied" {
		t.Fatalf("intruder mount response = %#v", response)
	}
	rendered, err := host.ViewTreeHost.RenderMount("shared.widget", 80)
	if err != nil || !reflect.DeepEqual(rendered, []string{"Owner"}) {
		t.Fatalf("rendered after denied mount = %#v err=%v", rendered, err)
	}

	owner.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"remount_owner","method":"host.tui.mount","params":{"mountId":"shared.widget","slot":"widget.aboveEditor","view":{"type":"text","text":"Owner remounted"}}}`)
	assertHostActionResult(t, ownerLines[len(ownerLines)-1], "remount_owner", "mounted", true)

	intruder.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"patch_intruder","method":"host.tui.patch","params":{"mountId":"shared.widget","ops":[{"op":"replace","path":"/text","value":"Intruder"}]}}`)
	if err := json.Unmarshal([]byte(intruderLines[len(intruderLines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "patch_intruder" || response.Error == nil || response.Error.Code != "policy_denied" {
		t.Fatalf("intruder patch response = %#v", response)
	}
	rendered, err = host.ViewTreeHost.RenderMount("shared.widget", 80)
	if err != nil || !reflect.DeepEqual(rendered, []string{"Owner remounted"}) {
		t.Fatalf("rendered after denied patch = %#v err=%v", rendered, err)
	}

	intruder.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"unmount_intruder","method":"host.tui.unmount","params":{"mountId":"shared.widget"}}`)
	if err := json.Unmarshal([]byte(intruderLines[len(intruderLines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "unmount_intruder" || response.Error == nil || response.Error.Code != "policy_denied" {
		t.Fatalf("intruder unmount response = %#v", response)
	}
	if _, ok := host.ViewTreeHost.Mounted("shared.widget"); !ok {
		t.Fatal("intruder unmounted a mount it does not own")
	}

	owner.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"patch_owner","method":"host.tui.patch","params":{"mountId":"shared.widget","ops":[{"op":"replace","path":"/text","value":"Owner patched"}]}}`)
	assertHostActionResult(t, ownerLines[len(ownerLines)-1], "patch_owner", "patched", true)
	owner.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"unmount_owner","method":"host.tui.unmount","params":{"mountId":"shared.widget"}}`)
	assertHostActionResult(t, ownerLines[len(ownerLines)-1], "unmount_owner", "unmounted", true)
	if _, ok := host.ViewTreeHost.Mounted("shared.widget"); ok {
		t.Fatal("owner unmount did not remove mount")
	}
}

func TestRPCLineProcessorReportsViewTreeHostActionErrors(t *testing.T) {
	host := NewRPCSessionHost(nil)
	var lines []string
	processor := RPCLineProcessor{Host: host, WriteLine: func(line string) {
		lines = append(lines, strings.TrimSpace(line))
	}}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"patch_missing","method":"host.tui.patch","params":{"mountId":"missing","ops":[{"op":"replace","path":"/text","value":"x"}]}}`)

	var response HostActionResponse
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "patch_missing" || response.Error == nil || response.Error.Code != "stale_context" {
		t.Fatalf("response = %#v", response)
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"editor_missing","method":"host.tui.editor","params":{"action":"read"}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "editor_missing" || response.Error == nil || response.Error.Code != "stale_context" {
		t.Fatalf("editor response = %#v", response)
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"dialog_missing","method":"host.tui.dialog","params":{"kind":"notify","message":"x"}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "dialog_missing" || response.Error == nil || response.Error.Code != "stale_context" {
		t.Fatalf("dialog response = %#v", response)
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"title_missing","method":"host.tui.title","params":{"title":"x"}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "title_missing" || response.Error == nil || response.Error.Code != "stale_context" {
		t.Fatalf("title response = %#v", response)
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"working_missing","method":"host.tui.working","params":{"message":"x"}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "working_missing" || response.Error == nil || response.Error.Code != "stale_context" {
		t.Fatalf("working response = %#v", response)
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"thinking_label_missing","method":"host.tui.thinking_label","params":{"label":"x"}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "thinking_label_missing" || response.Error == nil || response.Error.Code != "stale_context" {
		t.Fatalf("thinking label response = %#v", response)
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"theme_missing","method":"host.tui.theme","params":{"action":"current"}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "theme_missing" || response.Error == nil || response.Error.Code != "stale_context" {
		t.Fatalf("theme response = %#v", response)
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"tools_expanded_missing","method":"host.tui.tools_expanded","params":{}}`)
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "tools_expanded_missing" || response.Error == nil || response.Error.Code != "stale_context" {
		t.Fatalf("tools expanded response = %#v", response)
	}
}

func TestRPCSessionHostToolsAndCustomStateHostActions(t *testing.T) {
	host, session, manager := createRPCSessionHostForTest(t)

	list := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "tools_list",
		Method:   "host.tools.list",
		Params:   []byte(`{}`),
	})
	result := assertHostActionResponseOK(t, list, "tools_list")
	tools, ok := result["tools"].([]hostToolInfo)
	if !ok || len(tools) == 0 {
		t.Fatalf("tools = %#v", result["tools"])
	}
	if tools[0].Name != "read" || !tools[0].Active {
		t.Fatalf("first tool = %#v", tools[0])
	}

	setActive := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "tools_set",
		Method:   "host.tools.set_active",
		Params:   []byte(`{"mode":"replace","toolNames":["read","grep","find","ls"]}`),
	})
	result = assertHostActionResponseOK(t, setActive, "tools_set")
	if got := result["activeToolNames"]; !reflect.DeepEqual(got, []string{"read", "grep", "find", "ls"}) {
		t.Fatalf("activeToolNames = %#v", got)
	}
	if got := session.GetActiveToolNames(); !reflect.DeepEqual(got, []string{"read", "grep", "find", "ls"}) {
		t.Fatalf("session active tools = %#v", got)
	}
	if strings.Contains(session.SystemPrompt, "Create or overwrite files") {
		t.Fatalf("system prompt still contains disabled write tool:\n%s", session.SystemPrompt)
	}

	custom := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "append_custom",
		Method:   "host.session.append_custom",
		Params:   []byte(`{"owner":"gi-plan-mode","type":"plan_state","data":{"mode":"planning"}}`),
	})
	result = assertHostActionResponseOK(t, custom, "append_custom")
	if result["entryId"] == "" || result["type"] != "plan_state" {
		t.Fatalf("custom result = %#v", result)
	}
	entries := manager.GetEntries()
	entry := entries[len(entries)-1]
	if entry.CustomType != "plan_state" {
		t.Fatalf("custom entry = %#v", entry)
	}
	payload, ok := entry.Data.(map[string]any)
	if !ok || payload["owner"] != "gi-plan-mode" {
		t.Fatalf("custom payload = %#v", entry.Data)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok || data["mode"] != "planning" {
		t.Fatalf("custom payload data = %#v", payload["data"])
	}
}

func TestRPCSessionHostSessionModelAndThinkingHostActions(t *testing.T) {
	host, session, manager := createRPCSessionHostForTest(t)
	currentModel := session.Agent.State.Model
	nextModel := llm.MustGetModel("openai", "gpt-4o-mini")
	host.AvailableModels = []llm.Model{currentModel, nextModel}
	host.ProviderAuthStatus = func(providerID string) AuthStatus {
		if providerID == "openai" {
			return AuthStatus{Configured: true, Source: "environment", Label: "OPENAI_API_KEY"}
		}
		return AuthStatus{Configured: false}
	}
	customID := manager.AppendCustomEntry("checkpoint", map[string]any{"ok": true})

	entries := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "entries",
		Method:   "host.session.entries",
		Params:   []byte(`{"scope":"branch"}`),
	})
	result := assertHostActionResponseOK(t, entries, "entries")
	if got := result["entries"].([]FileEntry); len(got) == 0 {
		t.Fatalf("entries = %#v", got)
	}
	commands := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "commands",
		Method:   "host.commands.list",
		Params:   []byte(`{}`),
	})
	if commands.ID != "commands" || commands.Error != nil {
		t.Fatalf("commands response = %#v", commands)
	}
	if _, ok := commands.Result.(RPCCommandsResult); !ok {
		t.Fatalf("commands result = %#v", commands.Result)
	}

	label := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "label",
		Method:   "host.session.set_label",
		Params:   []byte(`{"entryId":"` + customID + `","label":"keep"}`),
	})
	result = assertHostActionResponseOK(t, label, "label")
	if result["label"] != "keep" {
		t.Fatalf("label result = %#v", result)
	}
	if got, ok := manager.GetLabel(customID); !ok || got != "keep" {
		t.Fatalf("label = %q ok=%v", got, ok)
	}

	name := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "name",
		Method:   "host.session.set_name",
		Params:   []byte(`{"name":"planning"}`),
	})
	assertHostActionResponseOK(t, name, "name")
	if manager.GetSessionName() != "planning" {
		t.Fatalf("session name = %q", manager.GetSessionName())
	}

	models := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "models",
		Method:   "host.model.list",
		Params:   []byte(`{}`),
	})
	result = assertHostActionResponseOK(t, models, "models")
	if got := result["models"].([]llm.Model); len(got) != 2 {
		t.Fatalf("models = %#v", got)
	}
	auth, ok := result["auth"].(map[string]AuthStatus)
	if !ok || !auth["openai"].Configured || auth["openai"].Source != "environment" || auth[currentModel.Provider].Configured {
		t.Fatalf("auth = %#v", result["auth"])
	}

	selected := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "model_select",
		Method:   "host.model.select",
		Params:   []byte(`{"provider":"openai","modelId":"gpt-4o-mini"}`),
	})
	result = assertHostActionResponseOK(t, selected, "model_select")
	if session.Agent.State.Model.Provider != "openai" || session.Agent.State.Model.ID != "gpt-4o-mini" {
		t.Fatalf("selected model = %#v result=%#v", session.Agent.State.Model, result)
	}

	thinking := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "thinking_set",
		Method:   "host.thinking.set",
		Params:   []byte(`{"level":"medium"}`),
	})
	result = assertHostActionResponseOK(t, thinking, "thinking_set")
	if result["thinkingLevel"] != "off" {
		t.Fatalf("thinkingLevel = %#v", result["thinkingLevel"])
	}
	currentThinking := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "thinking_get",
		Method:   "host.thinking.get",
		Params:   []byte(`{}`),
	})
	result = assertHostActionResponseOK(t, currentThinking, "thinking_get")
	if result["thinkingLevel"] != "off" {
		t.Fatalf("current thinkingLevel = %#v", result["thinkingLevel"])
	}
}

func TestRPCSessionHostAgentAndSessionActionHostActions(t *testing.T) {
	host, session, manager := createRPCSessionHostForTest(t)

	steer := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "send_steer",
		Method:   "host.agent.send_user_message",
		Params:   []byte(`{"text":"from extension","deliverAs":"steer"}`),
	})
	result := assertHostActionResponseOK(t, steer, "send_steer")
	if result["deliveredAs"] != "steer" || len(session.GetSteeringQueue()) != 1 {
		t.Fatalf("send steer result=%#v queue=%#v", result, session.GetSteeringQueue())
	}

	session.isStreaming = true
	abort := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "abort",
		Method:   "host.agent.abort",
		Params:   []byte(`{}`),
	})
	result = assertHostActionResponseOK(t, abort, "abort")
	if result["aborted"] != true || !session.abortRequested {
		t.Fatalf("abort result=%#v abortRequested=%v", result, session.abortRequested)
	}
	session.isStreaming = false

	if err := session.Prompt("hello before clear"); err != nil {
		t.Fatal(err)
	}
	if len(manager.GetEntries()) == 0 {
		t.Fatal("expected prompt entries before clear")
	}
	clear := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "clear",
		Method:   "host.session.action",
		Params:   []byte(`{"action":"clear"}`),
	})
	result = assertHostActionResponseOK(t, clear, "clear")
	if result["cancelled"] != false || session.PendingMessageCount() != 0 {
		t.Fatalf("clear result=%#v pending=%d", result, session.PendingMessageCount())
	}
	if len(manager.GetEntries()) != 0 {
		t.Fatalf("entries after clear = %#v", manager.GetEntries())
	}
}

func TestRPCSessionHostSessionActionNavigateTreeHostAction(t *testing.T) {
	host, _, manager := createRPCSessionHostForTest(t)
	mustRPCPrompt(t, host, "First message")
	mustRPCPrompt(t, host, "Second message")
	tree := manager.GetTree()
	if len(tree) != 1 {
		t.Fatalf("tree = %#v", tree)
	}

	response := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "navigate_tree",
		Method:   "host.session.action",
		Params:   []byte(`{"action":"navigate_tree","targetId":"` + tree[0].Entry.ID + `"}`),
	})
	result := assertHostActionResponseOK(t, response, "navigate_tree")
	if result["action"] != "navigate_tree" || result["cancelled"] != false || result["editorText"] != "First message" {
		t.Fatalf("navigate result = %#v", result)
	}
	if leaf := manager.GetLeafID(); leaf != nil {
		t.Fatalf("leaf = %v, want nil", *leaf)
	}
}

func TestRPCSessionHostSessionActionReloadHostAction(t *testing.T) {
	host, _, manager := createRPCSessionHostForTest(t)
	var reloads int
	host.ReloadSession = func() error {
		reloads++
		return nil
	}

	response := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "reload",
		Method:   "host.session.action",
		Params:   []byte(`{"action":"reload"}`),
	})
	result := assertHostActionResponseOK(t, response, "reload")
	if reloads != 1 || result["action"] != "reload" || result["cancelled"] != false || result["reloaded"] != true || result["sessionFile"] != manager.GetSessionFile() {
		t.Fatalf("reloads=%d result=%#v", reloads, result)
	}
}

func TestRPCSessionHostSessionActionReloadRequiresHostCallback(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)

	response := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "reload",
		Method:   "host.session.action",
		Params:   []byte(`{"action":"reload"}`),
	})
	if response.Error == nil || response.Error.Code != "invalid_params" || !strings.Contains(response.Error.Message, "reload is unavailable") {
		t.Fatalf("response = %#v", response)
	}
}

func TestAgentSessionPrintModeHostWiresReloadHostAction(t *testing.T) {
	_, session, manager := createRPCSessionHostForTest(t)
	runtimeHost, err := NewAgentSessionRuntimeHost(session, NewProtocolExtensionRuntime(CapabilityLifecycleEvents))
	if err != nil {
		t.Fatal(err)
	}
	printHost := &agentSessionPrintModeHost{session: session, sessionRuntimeHost: runtimeHost}
	host := printHost.NewProtocolExtensionRPCSessionHost(nil, nil, nil)
	if host == nil || host.ReloadSession == nil {
		t.Fatalf("reload callback was not wired: %#v", host)
	}

	response := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "reload",
		Method:   "host.session.action",
		Params:   []byte(`{"action":"reload"}`),
	})
	result := assertHostActionResponseOK(t, response, "reload")
	if result["action"] != "reload" || result["sessionFile"] != manager.GetSessionFile() {
		t.Fatalf("reload result = %#v", result)
	}
}

func TestRPCSessionHostAgentRunAndSpawnHostActionsUseIsolatedChildSessions(t *testing.T) {
	host, session, manager := createRPCSessionHostForTest(t, func(options *AgentSessionOptions) {
		options.Responder = func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
			return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("child: " + prompt)}}, nil
		}
	})
	parentEntryCount := len(manager.GetEntries())

	run := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "agent_run",
		Method:   "host.agent.run",
		Params:   []byte(`{"task":"summarize repo","tools":["read"]}`),
	})
	result := assertHostActionResponseOK(t, run, "agent_run")
	if result["persisted"] != false {
		t.Fatalf("run persisted = %#v", result)
	}
	if text, ok := result["lastAssistantText"].(*string); !ok || text == nil || *text != "child: summarize repo" {
		t.Fatalf("run last assistant text = %#v", result["lastAssistantText"])
	}
	if progress, ok := result["progressEvents"].([]map[string]any); !ok || len(progress) == 0 {
		t.Fatalf("run progress events = %#v", result["progressEvents"])
	}
	if !viewTreeStatusContains(t, host.ViewTreeHost, "agent done: summarize repo") {
		t.Fatalf("missing run status mounts = %#v", host.ViewTreeHost.MountsBySlot("footer"))
	}
	if len(manager.GetEntries()) != parentEntryCount {
		t.Fatalf("parent entries changed after run = %#v", manager.GetEntries())
	}

	spawn := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "agent_spawn",
		Method:   "host.agent.spawn",
		Params:   []byte(`{"prompt":"implement task","name":"worker"}`),
	})
	result = assertHostActionResponseOK(t, spawn, "agent_spawn")
	if result["persisted"] != true {
		t.Fatalf("spawn persisted = %#v", result)
	}
	sessionFile, ok := result["sessionFile"].(string)
	if !ok || sessionFile == "" || sessionFile == manager.GetSessionFile() {
		t.Fatalf("spawn sessionFile = %#v parent=%q", result["sessionFile"], manager.GetSessionFile())
	}
	if _, err := os.Stat(sessionFile); err != nil {
		t.Fatalf("spawn session file not persisted: %v", err)
	}
	entries, ok := result["entries"].([]FileEntry)
	if !ok || len(entries) < 2 {
		t.Fatalf("spawn entries = %#v", result["entries"])
	}
	persistedEntries := LoadEntriesFromFile(sessionFile)
	if len(persistedEntries) == 0 || persistedEntries[0].ParentSession != manager.GetSessionFile() {
		t.Fatalf("spawn persisted header = %#v", persistedEntries)
	}
	if text, ok := result["lastAssistantText"].(*string); !ok || text == nil || *text != "child: implement task" {
		t.Fatalf("spawn last assistant text = %#v", result["lastAssistantText"])
	}
	if !viewTreeStatusContains(t, host.ViewTreeHost, "agent done: worker") {
		t.Fatalf("missing spawn status mounts = %#v", host.ViewTreeHost.MountsBySlot("footer"))
	}
	if session.SessionManager.GetSessionFile() != manager.GetSessionFile() {
		t.Fatalf("parent session switched to %q", session.SessionManager.GetSessionFile())
	}

	outside := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "agent_outside",
		Method:   "host.agent.run",
		Params:   []byte(`{"prompt":"x","cwd":"../outside"}`),
	})
	if outside.Error == nil || outside.Error.Code != "policy_denied" {
		t.Fatalf("outside response = %#v", outside)
	}
}

func TestRPCSessionHostAgentSpawnDoesNotPersistWhenParentSessionIsInMemory(t *testing.T) {
	cwd := t.TempDir()
	manager, err := InMemorySessionManager(cwd)
	if err != nil {
		t.Fatal(err)
	}
	session, err := CreateAgentSession(AgentSessionOptions{
		CWD:            cwd,
		AgentDir:       filepath.Join(t.TempDir(), "agent"),
		Model:          llm.MustGetModel("anthropic", "claude-sonnet-4-5"),
		SessionManager: manager,
		Responder: func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
			return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("child: " + prompt)}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Dispose)
	host := NewRPCSessionHost(session)

	spawn := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "agent_spawn",
		Method:   "host.agent.spawn",
		Params:   []byte(`{"task":"inspect without parent persistence"}`),
	})
	result := assertHostActionResponseOK(t, spawn, "agent_spawn")
	if result["persisted"] != false || result["sessionFile"] != "" {
		t.Fatalf("spawn result should stay in-memory for in-memory parent = %#v", result)
	}
	if text, ok := result["lastAssistantText"].(*string); !ok || text == nil || *text != "child: inspect without parent persistence" {
		t.Fatalf("spawn last assistant text = %#v", result["lastAssistantText"])
	}
	matches, err := filepath.Glob(filepath.Join(cwd, "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("in-memory parent spawn wrote session files: %#v", matches)
	}
}

func TestRPCSessionHostAgentAbortCanCancelRunningChildAgent(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	host, _, _ := createRPCSessionHostForTest(t, func(options *AgentSessionOptions) {
		options.Responder = func(prompt string, _ []llm.Message, _ llm.Model) (llm.Message, error) {
			startedOnce.Do(func() { close(started) })
			<-release
			return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.Text("child result: " + prompt)}}, nil
		}
	})

	runCh := make(chan HostActionResponse, 1)
	go func() {
		runCh <- host.HandleHostAction(HostActionRequest{
			Type:     "request",
			Protocol: "gi-ext-rpc@1",
			ID:       "agent_run",
			Method:   "host.agent.run",
			Params:   []byte(`{"task":"long child task"}`),
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("child responder did not start")
	}

	abort := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "abort_children",
		Method:   "host.agent.abort",
		Params:   []byte(`{"target":"children"}`),
	})
	result := assertHostActionResponseOK(t, abort, "abort_children")
	if result["aborted"] != true || result["target"] != "children" {
		t.Fatalf("abort children result = %#v", result)
	}
	close(release)

	var run HostActionResponse
	select {
	case run = <-runCh:
	case <-time.After(time.Second):
		t.Fatal("child run did not finish after abort")
	}
	result = assertHostActionResponseOK(t, run, "agent_run")
	if result["aborted"] != true {
		t.Fatalf("child run result = %#v", result)
	}
	if !viewTreeStatusContains(t, host.ViewTreeHost, "agent aborted: long child task") {
		t.Fatalf("missing aborted child status mounts = %#v", host.ViewTreeHost.MountsBySlot("footer"))
	}
	host.childMu.Lock()
	runningChildren := len(host.childAgents)
	host.childMu.Unlock()
	if runningChildren != 0 {
		t.Fatalf("running child agents = %d", runningChildren)
	}
}

func TestRPCSessionHostFilesystemHostActionsAreScopedToSessionCWD(t *testing.T) {
	host, _, manager := createRPCSessionHostForTest(t)

	write := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "fs_write",
		Method:   "host.fs.write",
		Params:   []byte(`{"path":"state/plan.txt","content":"hello plan"}`),
	})
	result := assertHostActionResponseOK(t, write, "fs_write")
	if result["bytes"] != 10 {
		t.Fatalf("write result = %#v", result)
	}
	writtenPath := filepath.Join(manager.GetCWD(), "state", "plan.txt")
	content, err := os.ReadFile(writtenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello plan" {
		t.Fatalf("content = %q", string(content))
	}

	read := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "fs_read",
		Method:   "host.fs.read",
		Params:   []byte(`{"path":"state/plan.txt"}`),
	})
	result = assertHostActionResponseOK(t, read, "fs_read")
	if result["content"] != "hello plan" {
		t.Fatalf("read result = %#v", result)
	}

	outside := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "fs_outside",
		Method:   "host.fs.read",
		Params:   []byte(`{"path":"../outside.txt"}`),
	})
	if outside.Error == nil || outside.Error.Code != "policy_denied" {
		t.Fatalf("outside response = %#v", outside)
	}
}

func TestRPCSessionHostProcessExecHostActionRequiresExecutor(t *testing.T) {
	host, _, manager := createRPCSessionHostForTest(t)
	denied := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "process_denied",
		Method:   "host.process.exec",
		Params:   []byte(`{"command":["git","status","--short"]}`),
	})
	if denied.Error == nil || denied.Error.Code != "policy_denied" {
		t.Fatalf("denied response = %#v", denied)
	}

	executor := &fakeHostProcessExecutor{}
	host.ProcessExecutor = executor
	allowed := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "process_allowed",
		Method:   "host.process.exec",
		Params:   []byte(`{"command":["git","status","--short"]}`),
	})
	if allowed.ID != "process_allowed" || allowed.Error != nil {
		t.Fatalf("allowed response = %#v", allowed)
	}
	result, ok := allowed.Result.(HostProcessResult)
	if !ok || result.Stdout != "git status --short" {
		t.Fatalf("allowed result = %#v", allowed.Result)
	}
	if !reflect.DeepEqual(executor.command, []string{"git", "status", "--short"}) || executor.cwd != manager.GetCWD() {
		t.Fatalf("executor command=%#v cwd=%q", executor.command, executor.cwd)
	}
	if executor.options.Timeout != 0 {
		t.Fatalf("default timeout = %s", executor.options.Timeout)
	}

	subdir := filepath.Join(manager.GetCWD(), "packages", "demo")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	scoped := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "process_scoped",
		Method:   "host.process.exec",
		Params:   []byte(`{"command":["git","status","--short"],"cwd":"packages/demo"}`),
	})
	if scoped.ID != "process_scoped" || scoped.Error != nil {
		t.Fatalf("scoped response = %#v", scoped)
	}
	if executor.cwd != subdir {
		t.Fatalf("scoped cwd = %q, want %q", executor.cwd, subdir)
	}

	timed := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "process_timed",
		Method:   "host.process.exec",
		Params:   []byte(`{"command":["git","status","--short"],"timeoutMillis":1500}`),
	})
	if timed.ID != "process_timed" || timed.Error != nil {
		t.Fatalf("timed response = %#v", timed)
	}
	if executor.options.Timeout != 1500*time.Millisecond {
		t.Fatalf("timeout = %s, want 1500ms", executor.options.Timeout)
	}

	outside := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "process_outside",
		Method:   "host.process.exec",
		Params:   []byte(`{"command":["git","status","--short"],"cwd":"../outside"}`),
	})
	if outside.Error == nil || outside.Error.Code != "policy_denied" {
		t.Fatalf("outside process response = %#v", outside)
	}

	helperParams, err := json.Marshal(map[string]any{
		"command":       []string{os.Args[0], "-test.run=TestHostActionSleepHelper", "--", "--gi-sleep-helper"},
		"timeoutMillis": 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	host.ProcessExecutor = LocalHostProcessExecutor{}
	timeoutResponse := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "process_timeout",
		Method:   "host.process.exec",
		Params:   helperParams,
	})
	if timeoutResponse.ID != "process_timeout" || timeoutResponse.Error != nil {
		t.Fatalf("timeout response = %#v", timeoutResponse)
	}
	timeoutResult, ok := timeoutResponse.Result.(HostProcessResult)
	if !ok || !timeoutResult.Killed || timeoutResult.ExitCode != -1 {
		t.Fatalf("timeout result = %#v", timeoutResponse.Result)
	}
}

func TestHostActionSleepHelper(t *testing.T) {
	for _, arg := range os.Args {
		if arg == "--gi-sleep-helper" {
			time.Sleep(5 * time.Second)
			return
		}
		if arg == "--gi-sleep-helper-short" {
			time.Sleep(2 * time.Second)
			return
		}
		if arg == "--gi-spawn-stdout-child" {
			cmd := exec.Command(os.Args[0], "-test.run=TestHostActionSleepHelper", "--", "--gi-sleep-helper-short")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				t.Fatalf("start inherited-stdio child: %v", err)
			}
			return
		}
	}
}

func TestLocalHostProcessExecutorDoesNotWaitForInheritedStdio(t *testing.T) {
	start := time.Now()
	result, err := LocalHostProcessExecutor{}.ExecuteHostProcess(
		[]string{os.Args[0], "-test.run=TestHostActionSleepHelper", "--", "--gi-spawn-stdout-child"},
		t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || result.Killed {
		t.Fatalf("result = %#v", result)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("executor waited for inherited stdio handle: %s", elapsed)
	}
}

func TestRPCSessionHostProcessExecHostActionCancelsWithContext(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	host.ProcessExecutor = LocalHostProcessExecutor{}
	helperParams, err := json.Marshal(map[string]any{
		"command": []string{os.Args[0], "-test.run=TestHostActionSleepHelper", "--", "--gi-sleep-helper"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan HostActionResponse, 1)
	go func() {
		done <- host.HandleHostActionContext(ctx, HostActionRequest{
			Type:     "request",
			Protocol: "gi-ext-rpc@1",
			ID:       "process_cancel",
			Method:   "host.process.exec",
			Params:   helperParams,
		})
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case response := <-done:
		if response.ID != "process_cancel" || response.Error != nil {
			t.Fatalf("cancel response = %#v", response)
		}
		result, ok := response.Result.(HostProcessResult)
		if !ok || !result.Killed || result.ExitCode != -1 {
			t.Fatalf("cancel result = %#v", response.Result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("process cancellation did not return")
	}
}

func TestRPCSessionHostPolicyRequestDefaultsToDenied(t *testing.T) {
	host := NewRPCSessionHost(nil)
	response := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "policy_request",
		Method:   "host.policy.request",
		Params:   []byte(`{"capability":"process.exec:git","reason":"run git status"}`),
	})
	if response.ID != "policy_request" || response.Error != nil {
		t.Fatalf("response = %#v", response)
	}
	decision, ok := response.Result.(HostPolicyDecision)
	if !ok || decision.Granted || !reflect.DeepEqual(decision.DeniedCapabilities, []string{"process.exec:git"}) || decision.Reason == "" {
		t.Fatalf("decision = %#v", response.Result)
	}

	stdio := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "policy_stdio",
		Method:   "host.policy.request",
		Params:   []byte(`{"capability":"process.stdio:mcp","reason":"run MCP server"}`),
	})
	if stdio.ID != "policy_stdio" || stdio.Error != nil {
		t.Fatalf("stdio response = %#v", stdio)
	}
	stdioDecision, ok := stdio.Result.(HostPolicyDecision)
	if !ok || stdioDecision.Granted || !reflect.DeepEqual(stdioDecision.DeniedCapabilities, []string{"process.stdio:mcp"}) || stdioDecision.Reason == "" {
		t.Fatalf("stdio decision = %#v", stdio.Result)
	}

	invalid := host.HandleHostAction(HostActionRequest{
		Type:     "request",
		Protocol: "gi-ext-rpc@1",
		ID:       "policy_invalid",
		Method:   "host.policy.request",
		Params:   []byte(`{"capability":"unknown.capability"}`),
	})
	if invalid.Error == nil || invalid.Error.Code != "invalid_params" {
		t.Fatalf("invalid response = %#v", invalid)
	}
}

func TestHostActionRegistryPolicyRequestHasNoPriorCapability(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "protocol", "spec", "registries", "host-actions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Actions []struct {
			Name       string `json:"name"`
			Capability string `json:"capability"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(content, &registry); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, action := range registry.Actions {
		if action.Name != "host.policy.request" {
			continue
		}
		found = true
		if action.Capability != "none" {
			t.Fatalf("host.policy.request registry capability = %q, want none", action.Capability)
		}
	}
	if !found {
		t.Fatal("host.policy.request missing from host action registry")
	}
	if caps := hostActionRequiredCapabilities(HostActionRequest{Method: "host.policy.request"}); len(caps) != 0 {
		t.Fatalf("runtime required capabilities = %#v, want none", caps)
	}
}

func TestHostActionRuntimeCapabilitiesMatchRegistry(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "protocol", "spec", "registries", "host-actions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Actions []struct {
			Name       string `json:"name"`
			Capability string `json:"capability"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(content, &registry); err != nil {
		t.Fatal(err)
	}
	for _, action := range registry.Actions {
		request := HostActionRequest{Method: action.Name, Params: []byte(`{}`)}
		switch action.Name {
		case "host.tui.mount":
			request.Params = []byte(`{"slot":"footer","mountId":"test","view":{"type":"text","text":"x"}}`)
		case "host.process.exec":
			request.Params = []byte(`{"command":["git","status"]}`)
		}
		required := hostActionRequiredCapabilities(request)
		if !hostActionRegistryCapabilityMatchesRuntime(action.Capability, required) {
			t.Fatalf("%s registry capability %q does not match runtime %v", action.Name, action.Capability, required)
		}
	}
}

func hostActionRegistryCapabilityMatchesRuntime(registryCapability string, required []string) bool {
	switch registryCapability {
	case "none":
		return len(required) == 0
	case "slot-specific tui.*":
		return containsString(required, CapabilityTUIFooter)
	case "owned tui.* mount":
		for _, capability := range []string{CapabilityTUIWidget, CapabilityTUIHeader, CapabilityTUIFooter, CapabilityTUIOverlay, CapabilityTUIEditor} {
			if !containsString(required, capability) {
				return false
			}
		}
		return true
	default:
		if strings.Contains(registryCapability, "<scope>") {
			prefix := strings.TrimSuffix(registryCapability, "<scope>")
			for _, capability := range required {
				if capability == prefix || strings.HasPrefix(capability, prefix) {
					return true
				}
			}
			return false
		}
		return containsString(required, registryCapability)
	}
}

func TestRPCLineProcessorAppliesPolicyRequestCapabilityGrant(t *testing.T) {
	host, _, manager := createRPCSessionHostForTest(t)
	executor := &fakeHostProcessExecutor{}
	host.ProcessExecutor = executor
	host.PolicyRequester = HostPolicyRequesterFunc(func(request HostPolicyRequest) (HostPolicyDecision, error) {
		if !reflect.DeepEqual(request.Capabilities, []string{"process.exec:git"}) || request.Reason != "run git status" {
			t.Fatalf("policy request = %#v", request)
		}
		return HostPolicyDecision{
			Granted:             true,
			GrantedCapabilities: []string{"process.exec:"},
			Reason:              "approved for this session",
		}, nil
	})

	var lines []string
	processor := RPCLineProcessor{
		Host:                host,
		EnforceCapabilities: true,
		WriteLine: func(line string) {
			lines = append(lines, strings.TrimSpace(line))
		},
	}
	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"process_denied","method":"host.process.exec","params":{"command":["git","status","--short"]}}`)
	var response HostActionResponse
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "process_denied" || response.Error == nil || response.Error.Code != "missing_capability" {
		t.Fatalf("denied response = %#v", response)
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"policy_grant","method":"host.policy.request","params":{"capability":"process.exec:git","reason":"run git status"}}`)
	response = HostActionResponse{}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	result := assertHostActionResponseOK(t, response, "policy_grant")
	if result["granted"] != true {
		t.Fatalf("policy result = %#v", result)
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"process_allowed","method":"host.process.exec","params":{"command":["git","status","--short"]}}`)
	response = HostActionResponse{}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	result = assertHostActionResponseOK(t, response, "process_allowed")
	if result["stdout"] != "git status --short" || !reflect.DeepEqual(executor.command, []string{"git", "status", "--short"}) || executor.cwd != manager.GetCWD() {
		t.Fatalf("process result=%#v command=%#v cwd=%q", result, executor.command, executor.cwd)
	}
}

func TestRPCLineProcessorScopesProcessExecCapabilityToCommand(t *testing.T) {
	host, _, _ := createRPCSessionHostForTest(t)
	host.ProcessExecutor = &fakeHostProcessExecutor{}
	var lines []string
	processor := RPCLineProcessor{
		Host:                host,
		AllowedCapabilities: []string{"process.exec:git"},
		EnforceCapabilities: true,
		WriteLine: func(line string) {
			lines = append(lines, strings.TrimSpace(line))
		},
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"git_allowed","method":"host.process.exec","params":{"command":["/usr/bin/git","status"]}}`)
	var response HostActionResponse
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "git_allowed" || response.Error != nil {
		t.Fatalf("git response = %#v", response)
	}

	processor.HandleLine(context.Background(), `{"type":"request","protocol":"gi-ext-rpc@1","id":"python_denied","method":"host.process.exec","params":{"command":["python3","script.py"]}}`)
	response = HostActionResponse{}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "python_denied" || response.Error == nil || response.Error.Code != "missing_capability" {
		t.Fatalf("python response = %#v", response)
	}
}

func TestGrantExtensionCapabilitiesAllowsGenericScopedGrants(t *testing.T) {
	granted := grantExtensionCapabilities(
		[]string{"process.exec:git", "process.stdio:mcp", "network:https"},
		[]string{"process.exec:", "process.stdio:", "network:"},
		true,
	)
	for _, capability := range []string{"process.exec:git", "process.stdio:mcp", "network:https"} {
		if !containsString(granted, capability) {
			t.Fatalf("granted = %#v, missing %s", granted, capability)
		}
	}
}

type fakeHostProcessExecutor struct {
	command []string
	cwd     string
	options HostProcessOptions
}

func (f *fakeHostProcessExecutor) ExecuteHostProcess(command []string, cwd string) (HostProcessResult, error) {
	return f.ExecuteHostProcessWithOptions(command, cwd, HostProcessOptions{})
}

func (f *fakeHostProcessExecutor) ExecuteHostProcessWithOptions(command []string, cwd string, options HostProcessOptions) (HostProcessResult, error) {
	f.command = append([]string(nil), command...)
	f.cwd = cwd
	f.options = options
	return HostProcessResult{Stdout: strings.Join(command, " ")}, nil
}

func isExtensionToHostExampleLine(t *testing.T, line string) bool {
	t.Helper()
	var envelope struct {
		Type     string `json:"type"`
		Protocol string `json:"protocol,omitempty"`
		Method   string `json:"method,omitempty"`
	}
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Type == "hello" {
		return true
	}
	if (envelope.Type == "request" || envelope.Type == "notification") &&
		envelope.Protocol == "gi-ext-rpc@1" &&
		strings.HasPrefix(envelope.Method, "host.") {
		return true
	}
	return false
}

func viewTreeStatusContains(t *testing.T, host *ViewTreeHost, expected string) bool {
	t.Helper()
	if host == nil {
		return false
	}
	for _, mount := range host.MountsBySlot("footer") {
		lines, err := host.RenderMount(mount.MountID, 120)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.Join(lines, "\n"), expected) {
			return true
		}
	}
	return false
}

func assertHostActionResult(t *testing.T, line, id, field string, want bool) {
	t.Helper()
	var response struct {
		Type     string         `json:"type"`
		Protocol string         `json:"protocol"`
		ID       string         `json:"id"`
		Result   map[string]any `json:"result"`
		Error    any            `json:"error,omitempty"`
	}
	if err := json.Unmarshal([]byte(line), &response); err != nil {
		t.Fatalf("line = %q err=%v", line, err)
	}
	if response.Type != "response" || response.Protocol != "gi-ext-rpc@1" || response.ID != id || response.Error != nil {
		t.Fatalf("response = %#v", response)
	}
	if got, _ := response.Result[field].(bool); got != want {
		t.Fatalf("%s = %#v, want %v in %#v", field, response.Result[field], want, response.Result)
	}
}

type recordingTUITitleHost struct {
	titles []string
}

func (h *recordingTUITitleHost) SetTUITitle(title string) error {
	h.titles = append(h.titles, title)
	return nil
}

type recordingTUIWorkingHost struct {
	updates []TUIWorkingUpdate
}

func (h *recordingTUIWorkingHost) SetTUIWorking(update TUIWorkingUpdate) error {
	h.updates = append(h.updates, update)
	return nil
}

type recordingTUIThinkingLabelHost struct {
	labels []string
}

func (h *recordingTUIThinkingLabelHost) SetHiddenThinkingLabel(label string) error {
	h.labels = append(h.labels, label)
	return nil
}

type recordingTUIStatusHost struct {
	statuses map[string]string
}

func (h *recordingTUIStatusHost) SetTUIStatus(key, text string) error {
	if h.statuses == nil {
		h.statuses = map[string]string{}
	}
	if text == "" {
		delete(h.statuses, key)
	} else {
		h.statuses[key] = text
	}
	return nil
}

type recordingTUIThemeHost struct {
	current  string
	themes   []TUIThemeInfo
	setCalls []string
}

func (h *recordingTUIThemeHost) CurrentTUITheme() string {
	return h.current
}

func (h *recordingTUIThemeHost) AvailableTUIThemes() []TUIThemeInfo {
	return append([]TUIThemeInfo(nil), h.themes...)
}

func (h *recordingTUIThemeHost) SetTUITheme(name string) error {
	h.current = name
	h.setCalls = append(h.setCalls, name)
	return nil
}

type recordingTUIToolExpansionHost struct {
	expanded bool
	setCalls []bool
}

func (h *recordingTUIToolExpansionHost) TUIToolsExpanded() bool {
	return h.expanded
}

func (h *recordingTUIToolExpansionHost) SetTUIToolsExpanded(expanded bool) error {
	h.expanded = expanded
	h.setCalls = append(h.setCalls, expanded)
	return nil
}
