package gicodingagent

import (
	"reflect"
	"strings"
	"testing"
)

func TestViewTreeHostMountPatchAndRenderWidget(t *testing.T) {
	unchecked := false
	checked := true
	host := NewViewTreeHost()
	view := ViewTreeNode{
		Type: "box",
		ID:   "root",
		Children: []ViewTreeNode{
			{Type: "text", Text: "Todos"},
			{Type: "list", Items: []ViewTreeItem{
				{Item: &ViewTreeListItem{ID: "1", Text: "Port approval gate", Checked: &unchecked}},
				{Item: &ViewTreeListItem{ID: "2", Text: "Run conformance replay", Checked: &unchecked}},
			}},
		},
	}

	if err := host.Mount("todo.current", "widget.aboveEditor", view); err != nil {
		t.Fatal(err)
	}
	lines, err := host.RenderMount("todo.current", 80)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Todos", "[ ] Port approval gate", "[ ] Run conformance replay"}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("lines = %#v, want %#v", lines, want)
	}

	if err := host.Patch("todo.current", []ViewTreePatchOperation{{
		Op:    "replace",
		Path:  "/children/1/items/0/checked",
		Value: checked,
	}}); err != nil {
		t.Fatal(err)
	}
	lines, err = host.RenderMount("todo.current", 80)
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"Todos", "[x] Port approval gate", "[ ] Run conformance replay"}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("patched lines = %#v, want %#v", lines, want)
	}
}

func TestViewTreeRendererUnknownNodeFallbackAndDiagnostic(t *testing.T) {
	lines := RenderViewTree(ViewTreeNode{
		Type:     "x-vendor.sparkline",
		Fallback: &ViewTreeNode{Type: "text", Text: "fallback sparkline"},
	}, 80)
	if !reflect.DeepEqual(lines, []string{"fallback sparkline"}) {
		t.Fatalf("fallback lines = %#v", lines)
	}

	lines = RenderViewTree(ViewTreeNode{Type: "sparkline"}, 80)
	if len(lines) != 1 || !strings.Contains(lines[0], "unsupported view node: sparkline") {
		t.Fatalf("diagnostic lines = %#v", lines)
	}
}

func TestViewTreeHostValidatesMountedAndPatchedTrees(t *testing.T) {
	host := NewViewTreeHost()
	if err := host.Mount("empty-type", "aboveEditor", ViewTreeNode{}); err == nil || !strings.Contains(err.Error(), "type is required") {
		t.Fatalf("empty type mount error = %v", err)
	}
	if err := host.Mount("experimental", "aboveEditor", ViewTreeNode{Type: "x-demo.sparkline"}); err == nil || !strings.Contains(err.Error(), "requires fallback") {
		t.Fatalf("experimental mount error = %v", err)
	}
	if err := host.Mount("list", "aboveEditor", ViewTreeNode{
		Type:  "list",
		Items: []ViewTreeItem{{Item: &ViewTreeListItem{Text: "missing id"}}},
	}); err == nil || !strings.Contains(err.Error(), "id is required") {
		t.Fatalf("list item mount error = %v", err)
	}
	if err := host.Mount("table", "aboveEditor", ViewTreeNode{
		Type:    "table",
		Columns: []ViewTreeTableColumn{{ID: "name", Title: "Name", Align: "diagonal"}},
	}); err == nil || !strings.Contains(err.Error(), "align") {
		t.Fatalf("table column mount error = %v", err)
	}

	if err := host.Mount("widget", "aboveEditor", ViewTreeNode{
		Type: "x-demo.sparkline",
		Fallback: &ViewTreeNode{
			Type: "text",
			Text: "fallback sparkline",
		},
	}); err != nil {
		t.Fatal(err)
	}
	rendered, err := host.RenderMount("widget", 80)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(rendered, "\n") != "fallback sparkline" {
		t.Fatalf("rendered fallback = %#v", rendered)
	}
	if err := host.Patch("widget", []ViewTreePatchOperation{{Op: "replace", Path: "/fallback/type", Value: ""}}); err == nil || !strings.Contains(err.Error(), "type is required") {
		t.Fatalf("invalid patch error = %v", err)
	}
	rendered, err = host.RenderMount("widget", 80)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(rendered, "\n") != "fallback sparkline" {
		t.Fatalf("invalid patch changed mount: %#v", rendered)
	}
}

func TestViewTreeHostRejectsStaleMountPatch(t *testing.T) {
	host := NewViewTreeHost()
	if err := host.Patch("missing", []ViewTreePatchOperation{{Op: "replace", Path: "/text", Value: "x"}}); err == nil {
		t.Fatal("expected stale mount error")
	}
	if _, err := host.RenderMount("missing", 80); err == nil {
		t.Fatal("expected stale render error")
	}
}

func TestViewTreeComponentRendersAsTUIComponent(t *testing.T) {
	component := NewViewTreeComponent(ViewTreeNode{
		Type: "row",
		Children: []ViewTreeNode{
			{Type: "text", Text: "Model"},
			{Type: "keyHint", Text: "gpt-4o"},
		},
	})
	lines := component.Render(80)
	if !reflect.DeepEqual(lines, []string{"Model <gpt-4o>"}) {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestViewTreeHostSlotsFocusAndInputEvents(t *testing.T) {
	host := NewViewTreeHost()
	var observed []ViewTreeEvent
	unsubscribe := host.OnEvent(func(event ViewTreeEvent) {
		observed = append(observed, event)
	})
	defer unsubscribe()

	view := ViewTreeNode{
		Type: "box",
		ID:   "root",
		Children: []ViewTreeNode{
			{Type: "button", ID: "save", Text: "Save"},
		},
	}
	if err := host.Mount("panel", "overlay", view); err != nil {
		t.Fatal(err)
	}
	if mounts := host.MountsBySlot("overlay"); len(mounts) != 1 || mounts[0].MountID != "panel" {
		t.Fatalf("mounts = %#v", mounts)
	}

	component := NewMountedViewTreeComponent(host, "panel")
	component.SetFocused(true)
	component.HandleInput("\r")
	component.HandleInput("a")
	component.HandleInput("\x1b")

	if focus := host.Focused(); focus.MountID != "panel" || focus.NodeID != "root" {
		t.Fatalf("focus = %#v", focus)
	}
	events := host.Events()
	if len(events) != 8 ||
		events[0].Event != "mount" ||
		events[1].Event != "focus" ||
		events[2].Event != "key" ||
		events[3].Event != "submit" ||
		events[4].Event != "key" ||
		events[5].Event != "textInput" ||
		events[6].Event != "key" ||
		events[7].Event != "cancel" {
		t.Fatalf("events = %#v", events)
	}
	if events[2].Data["key"] != "\r" {
		t.Fatalf("key event = %#v", events[2])
	}
	if events[3].Data["key"] != "enter" || events[3].Data["raw"] != "\r" {
		t.Fatalf("submit event = %#v", events[3])
	}
	if events[5].Data["text"] != "a" || events[5].Data["raw"] != "a" {
		t.Fatalf("textInput event = %#v", events[5])
	}
	if events[7].Data["key"] != "escape" || events[7].Data["raw"] != "\x1b" {
		t.Fatalf("cancel event = %#v", events[7])
	}
	if !reflect.DeepEqual(observed, events) {
		t.Fatalf("observed = %#v events = %#v", observed, events)
	}

	component.SetFocused(false)
	if focus := host.Focused(); focus.MountID != "" || focus.NodeID != "" {
		t.Fatalf("focus after blur = %#v", focus)
	}
	if events = host.Events(); events[len(events)-1].Event != "blur" {
		t.Fatalf("events after blur = %#v", events)
	}
}

func TestViewTreeComponentDispatchesSelectAndChangeEvents(t *testing.T) {
	t.Run("text controls emit change", func(t *testing.T) {
		host := NewViewTreeHost()
		var observed []ViewTreeEvent
		unsubscribe := host.OnEvent(func(event ViewTreeEvent) {
			observed = append(observed, event)
		})
		defer unsubscribe()

		if err := host.Mount("input-panel", "editor", ViewTreeNode{Type: "textarea", ID: "input", Text: ""}); err != nil {
			t.Fatal(err)
		}
		component := NewMountedViewTreeComponent(host, "input-panel")
		component.SetFocused(true)
		component.HandleInput("q")

		events := host.Events()
		if len(events) != 5 || events[2].Event != "key" || events[3].Event != "textInput" || events[4].Event != "change" {
			t.Fatalf("events = %#v", events)
		}
		if events[4].Data["text"] != "q" || events[4].Data["value"] != "q" || events[4].Data["raw"] != "q" {
			t.Fatalf("change event = %#v", events[4])
		}
		if !reflect.DeepEqual(observed, events) {
			t.Fatalf("observed = %#v events = %#v", observed, events)
		}
	})

	t.Run("selectable controls emit select", func(t *testing.T) {
		host := NewViewTreeHost()
		var observed []ViewTreeEvent
		unsubscribe := host.OnEvent(func(event ViewTreeEvent) {
			observed = append(observed, event)
		})
		defer unsubscribe()

		if err := host.Mount("button-panel", "overlay", ViewTreeNode{Type: "button", ID: "run", Text: "Run"}); err != nil {
			t.Fatal(err)
		}
		component := NewMountedViewTreeComponent(host, "button-panel")
		component.SetFocused(true)
		component.HandleInput("\r")

		events := host.Events()
		if len(events) != 5 || events[2].Event != "key" || events[3].Event != "submit" || events[4].Event != "select" {
			t.Fatalf("events = %#v", events)
		}
		if events[4].Data["id"] != "run" || events[4].Data["value"] != "run" {
			t.Fatalf("select event = %#v", events[4])
		}
		if !reflect.DeepEqual(observed, events) {
			t.Fatalf("observed = %#v events = %#v", observed, events)
		}
	})
}

func TestViewTreeHostDispatchesTickToSubscribedNodes(t *testing.T) {
	host := NewViewTreeHost()
	var observed []ViewTreeEvent
	unsubscribe := host.OnEvent(func(event ViewTreeEvent) {
		observed = append(observed, event)
	})
	defer unsubscribe()

	view := ViewTreeNode{
		Type:   "box",
		ID:     "root",
		Events: []string{"tick"},
		Children: []ViewTreeNode{
			{Type: "text", ID: "static", Text: "Static"},
			{Type: "text", ID: "animated", Text: "Animated", Events: []string{"tick"}},
		},
	}
	if err := host.Mount("animated-panel", "overlay", view); err != nil {
		t.Fatal(err)
	}
	if !host.HasEventSubscription("tick") {
		t.Fatal("expected tick subscription")
	}
	if err := host.DispatchTick(7); err != nil {
		t.Fatal(err)
	}

	var ticks []ViewTreeEvent
	for _, event := range observed {
		if event.Event == "tick" {
			ticks = append(ticks, event)
		}
	}
	if len(ticks) != 2 {
		t.Fatalf("ticks = %#v", ticks)
	}
	if ticks[0].MountID != "animated-panel" || ticks[0].NodeID != "root" || ticks[0].Data["frame"] != int64(7) {
		t.Fatalf("first tick = %#v", ticks[0])
	}
	if ticks[1].NodeID != "animated" {
		t.Fatalf("second tick = %#v", ticks[1])
	}
	for _, event := range host.Events() {
		if event.Event == "tick" {
			t.Fatalf("tick should not be retained in event history: %#v", host.Events())
		}
	}
}

func TestViewTreeHostDispatchesResizeToSubscribedNodes(t *testing.T) {
	host := NewViewTreeHost()
	var observed []ViewTreeEvent
	unsubscribe := host.OnEvent(func(event ViewTreeEvent) {
		observed = append(observed, event)
	})
	defer unsubscribe()

	view := ViewTreeNode{
		Type: "box",
		ID:   "root",
		Children: []ViewTreeNode{
			{Type: "text", ID: "size", Text: "Size", Events: []string{"resize"}},
		},
	}
	if err := host.Mount("size-panel", "footer", view); err != nil {
		t.Fatal(err)
	}
	if !host.HasEventSubscription("resize") {
		t.Fatal("expected resize subscription")
	}
	if err := host.DispatchResize(100, 24); err != nil {
		t.Fatal(err)
	}
	resize := observed[len(observed)-1]
	if resize.Event != "resize" || resize.NodeID != "size" || resize.Data["width"] != 100 || resize.Data["height"] != 24 {
		t.Fatalf("resize = %#v", resize)
	}
}

func TestViewTreeHostDispatchesThemeChangeToSubscribedNodes(t *testing.T) {
	host := NewViewTreeHost()
	var observed []ViewTreeEvent
	unsubscribe := host.OnEvent(func(event ViewTreeEvent) {
		observed = append(observed, event)
	})
	defer unsubscribe()

	view := ViewTreeNode{
		Type: "box",
		ID:   "root",
		Children: []ViewTreeNode{
			{Type: "text", ID: "theme", Text: "Theme", Events: []string{"theme_change"}},
		},
	}
	if err := host.Mount("theme-panel", "footer", view); err != nil {
		t.Fatal(err)
	}
	if !host.HasEventSubscription("theme_change") {
		t.Fatal("expected theme_change subscription")
	}
	if err := host.DispatchThemeChange("focus"); err != nil {
		t.Fatal(err)
	}
	theme := observed[len(observed)-1]
	if theme.Event != "theme_change" || theme.NodeID != "theme" || theme.Data["name"] != "focus" || theme.Data["preview"] != false {
		t.Fatalf("theme_change = %#v", theme)
	}
	for _, event := range host.Events() {
		if event.Event == "theme_change" {
			t.Fatalf("theme_change should not be retained in event history: %#v", host.Events())
		}
	}
}

func TestViewTreeHostDispatchesVisibilityChangeToSubscribedNodes(t *testing.T) {
	host := NewViewTreeHost()
	var observed []ViewTreeEvent
	unsubscribe := host.OnEvent(func(event ViewTreeEvent) {
		observed = append(observed, event)
	})
	defer unsubscribe()

	view := ViewTreeNode{
		Type: "box",
		ID:   "root",
		Children: []ViewTreeNode{
			{Type: "text", ID: "visibility", Text: "Visibility", Events: []string{"visibility_change"}},
		},
	}
	if err := host.Mount("visibility-panel", "footer", view); err != nil {
		t.Fatal(err)
	}
	if !host.Unmount("visibility-panel") {
		t.Fatal("expected mount to unmount")
	}

	var visibilityEvents []ViewTreeEvent
	for _, event := range observed {
		if event.Event == "visibility_change" {
			visibilityEvents = append(visibilityEvents, event)
		}
	}
	if len(visibilityEvents) != 2 {
		t.Fatalf("visibility events = %#v", visibilityEvents)
	}
	if visibilityEvents[0].NodeID != "visibility" || visibilityEvents[0].Data["visible"] != true || visibilityEvents[0].Data["reason"] != "mount" {
		t.Fatalf("mount visibility_change = %#v", visibilityEvents[0])
	}
	if visibilityEvents[1].NodeID != "visibility" || visibilityEvents[1].Data["visible"] != false || visibilityEvents[1].Data["reason"] != "unmount" {
		t.Fatalf("unmount visibility_change = %#v", visibilityEvents[1])
	}
	for _, event := range host.Events() {
		if event.Event == "visibility_change" {
			t.Fatalf("visibility_change should not be retained in event history: %#v", host.Events())
		}
	}
}

func TestViewTreeHostEmitsMountPatchUnmountChanges(t *testing.T) {
	host := NewViewTreeHost()
	var changes []ViewTreeChange
	unsubscribe := host.OnChange(func(change ViewTreeChange) {
		changes = append(changes, change)
	})
	defer unsubscribe()

	if err := host.Mount("panel", "footer", ViewTreeNode{Type: "text", Text: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := host.Patch("panel", []ViewTreePatchOperation{{Op: "replace", Path: "/text", Value: "two"}}); err != nil {
		t.Fatal(err)
	}
	if !host.Unmount("panel") {
		t.Fatal("expected panel to unmount")
	}

	want := []ViewTreeChange{
		{Type: "mount", MountID: "panel", Slot: "footer"},
		{Type: "patch", MountID: "panel", Slot: "footer"},
		{Type: "unmount", MountID: "panel", Slot: "footer"},
	}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("changes = %#v, want %#v", changes, want)
	}
}

func TestViewTreeHostSlotAliasesAndStatusOrdering(t *testing.T) {
	host := NewViewTreeHost()
	if err := host.Mount("widget", "widget.aboveEditor", ViewTreeNode{Type: "text", Text: "Widget"}); err != nil {
		t.Fatal(err)
	}
	if err := host.SetStatus("low", "Low priority", 10); err != nil {
		t.Fatal(err)
	}
	if err := host.SetStatus("high", "High priority", 50); err != nil {
		t.Fatal(err)
	}

	if mounts := host.MountsBySlot("aboveEditor"); len(mounts) != 1 || mounts[0].MountID != "widget" {
		t.Fatalf("aboveEditor mounts = %#v", mounts)
	}
	statusMounts := host.MountsBySlot("footer")
	if len(statusMounts) != 2 || statusMounts[0].MountID != "status:high" || statusMounts[1].MountID != "status:low" {
		t.Fatalf("status mounts = %#v", statusMounts)
	}
	if err := host.SetStatus("high", "", 0); err != nil {
		t.Fatal(err)
	}
	if statusMounts = host.MountsBySlot("footer"); len(statusMounts) != 1 || statusMounts[0].MountID != "status:low" {
		t.Fatalf("status mounts after clear = %#v", statusMounts)
	}
}

func TestViewTreeHostFocusRejectsUnknownNode(t *testing.T) {
	host := NewViewTreeHost()
	if err := host.Mount("panel", "overlay", ViewTreeNode{Type: "box", ID: "root"}); err != nil {
		t.Fatal(err)
	}
	if err := host.Focus("panel", "missing"); err == nil {
		t.Fatal("expected missing node focus error")
	}
}
