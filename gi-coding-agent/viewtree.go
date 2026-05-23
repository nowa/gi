package gicodingagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	gitui "github.com/nowa/gi/gi-tui"
)

type ViewTreeNode struct {
	Type     string                `json:"type"`
	ID       string                `json:"id,omitempty"`
	Text     string                `json:"text,omitempty"`
	Label    string                `json:"label,omitempty"`
	Value    *float64              `json:"value,omitempty"`
	Children []ViewTreeNode        `json:"children,omitempty"`
	Items    []ViewTreeItem        `json:"items,omitempty"`
	Columns  []ViewTreeTableColumn `json:"columns,omitempty"`
	Rows     []map[string]any      `json:"rows,omitempty"`
	Style    *ViewTreeStyle        `json:"style,omitempty"`
	Fallback *ViewTreeNode         `json:"fallback,omitempty"`
	Events   []string              `json:"events,omitempty"`
}

type ViewTreeItem struct {
	Node *ViewTreeNode
	Item *ViewTreeListItem
}

func (i *ViewTreeItem) UnmarshalJSON(data []byte) error {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	if probe.Type != "" {
		var node ViewTreeNode
		if err := json.Unmarshal(data, &node); err != nil {
			return err
		}
		i.Node = &node
		return nil
	}
	var item ViewTreeListItem
	if err := json.Unmarshal(data, &item); err != nil {
		return err
	}
	i.Item = &item
	return nil
}

func (i ViewTreeItem) MarshalJSON() ([]byte, error) {
	if i.Node != nil {
		return json.Marshal(i.Node)
	}
	if i.Item != nil {
		return json.Marshal(i.Item)
	}
	return []byte("null"), nil
}

type ViewTreeListItem struct {
	ID       string             `json:"id"`
	Text     string             `json:"text,omitempty"`
	Checked  *bool              `json:"checked,omitempty"`
	Disabled bool               `json:"disabled,omitempty"`
	Children []ViewTreeListItem `json:"children,omitempty"`
}

type ViewTreeTableColumn struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Align string `json:"align,omitempty"`
}

type ViewTreeStyle struct {
	FG        string `json:"fg,omitempty"`
	BG        string `json:"bg,omitempty"`
	Bold      bool   `json:"bold,omitempty"`
	Italic    bool   `json:"italic,omitempty"`
	Underline bool   `json:"underline,omitempty"`
	Dim       bool   `json:"dim,omitempty"`
}

type ViewTreePatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

type ViewTreeMount struct {
	MountID  string
	Slot     string
	View     ViewTreeNode
	Priority int
	Overlay  *ViewTreeOverlayOptions
}

type ViewTreeMountOptions struct {
	Priority int
	Overlay  *ViewTreeOverlayOptions
}

type ViewTreeOverlayOptions struct {
	Width        ViewTreeSizeValue     `json:"width,omitempty"`
	MinWidth     int                   `json:"minWidth,omitempty"`
	MaxHeight    ViewTreeSizeValue     `json:"maxHeight,omitempty"`
	Anchor       string                `json:"anchor,omitempty"`
	OffsetX      int                   `json:"offsetX,omitempty"`
	OffsetY      int                   `json:"offsetY,omitempty"`
	Row          ViewTreeSizeValue     `json:"row,omitempty"`
	Col          ViewTreeSizeValue     `json:"col,omitempty"`
	Margin       ViewTreeOverlayMargin `json:"margin,omitempty"`
	NonCapturing bool                  `json:"nonCapturing,omitempty"`
}

type ViewTreeSizeValue struct {
	Set          bool
	Value        int
	Percent      bool
	PercentValue float64
}

type ViewTreeOverlayMargin struct {
	Set    bool
	Top    int
	Right  int
	Bottom int
	Left   int
}

var (
	standardViewTreeNodeTypes = map[string]struct{}{
		"text": {}, "markdown": {}, "box": {}, "row": {}, "column": {}, "spacer": {},
		"list": {}, "table": {}, "tree": {}, "input": {}, "textarea": {}, "select": {},
		"button": {}, "progress": {}, "spinner": {}, "diff": {}, "image": {},
		"toolCall": {}, "message": {}, "keyHint": {}, "portal": {},
	}
	standardViewTreeEvents = map[string]struct{}{
		"mount": {}, "unmount": {}, "focus": {}, "blur": {}, "key": {},
		"textInput": {}, "submit": {}, "cancel": {}, "select": {}, "change": {},
		"resize": {}, "theme_change": {}, "visibility_change": {}, "tick": {},
	}
	standardViewTreeColors = map[string]struct{}{
		"default": {}, "muted": {}, "accent": {}, "success": {}, "warning": {},
		"error": {}, "dim": {}, "border": {}, "surface": {}, "surfaceAlt": {},
		"tool": {}, "customMessage": {},
	}
)

func ValidateViewTreeNode(node ViewTreeNode) error {
	return validateViewTreeNode(node, "$")
}

func validateViewTreeNode(node ViewTreeNode, path string) error {
	nodeType := strings.TrimSpace(node.Type)
	if nodeType == "" {
		return fmt.Errorf("viewtree node %s type is required", path)
	}
	_, registeredType := standardViewTreeNodeTypes[nodeType]
	if !registeredType && strings.HasPrefix(nodeType, "x-") && node.Fallback == nil {
		return fmt.Errorf("experimental viewtree node %s (%s) requires fallback", path, nodeType)
	}
	if err := validateViewTreeStyle(node.Style, path+".style"); err != nil {
		return err
	}
	for index, event := range node.Events {
		if _, ok := standardViewTreeEvents[event]; !ok {
			return fmt.Errorf("viewtree node %s events[%d] %q is not registered", path, index, event)
		}
	}
	for index, child := range node.Children {
		if err := validateViewTreeNode(child, fmt.Sprintf("%s.children[%d]", path, index)); err != nil {
			return err
		}
	}
	for index, item := range node.Items {
		if err := validateViewTreeItem(item, fmt.Sprintf("%s.items[%d]", path, index)); err != nil {
			return err
		}
	}
	for index, column := range node.Columns {
		if strings.TrimSpace(column.ID) == "" {
			return fmt.Errorf("viewtree node %s columns[%d] id is required", path, index)
		}
		if strings.TrimSpace(column.Title) == "" {
			return fmt.Errorf("viewtree node %s columns[%d] title is required", path, index)
		}
		if column.Align != "" && column.Align != "left" && column.Align != "center" && column.Align != "right" {
			return fmt.Errorf("viewtree node %s columns[%d] align %q is invalid", path, index, column.Align)
		}
	}
	if node.Fallback != nil {
		if err := validateViewTreeNode(*node.Fallback, path+".fallback"); err != nil {
			return err
		}
	}
	return nil
}

func validateViewTreeItem(item ViewTreeItem, path string) error {
	if item.Node != nil {
		return validateViewTreeNode(*item.Node, path)
	}
	if item.Item == nil {
		return nil
	}
	if strings.TrimSpace(item.Item.ID) == "" {
		return fmt.Errorf("viewtree list item %s id is required", path)
	}
	for index := range item.Item.Children {
		child := item.Item.Children[index]
		if err := validateViewTreeItem(ViewTreeItem{Item: &child}, fmt.Sprintf("%s.children[%d]", path, index)); err != nil {
			return err
		}
	}
	return nil
}

func validateViewTreeStyle(style *ViewTreeStyle, path string) error {
	if style == nil {
		return nil
	}
	for label, color := range map[string]string{"fg": style.FG, "bg": style.BG} {
		if color == "" {
			continue
		}
		if _, ok := standardViewTreeColors[color]; !ok {
			return fmt.Errorf("viewtree style %s.%s color %q is not registered", path, label, color)
		}
	}
	return nil
}

func (v *ViewTreeSizeValue) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		*v = ViewTreeSizeValue{}
		return nil
	}
	var number float64
	if err := json.Unmarshal(data, &number); err == nil {
		*v = ViewTreeSizeValue{Set: true, Value: int(number)}
		return nil
	}
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	raw = strings.TrimSpace(raw)
	if strings.HasSuffix(raw, "%") {
		value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(raw, "%")), 64)
		if err != nil {
			return err
		}
		*v = ViewTreeSizeValue{Set: true, Percent: true, Value: int(value), PercentValue: value}
		return nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return err
	}
	*v = ViewTreeSizeValue{Set: true, Value: value}
	return nil
}

func (m *ViewTreeOverlayMargin) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		*m = ViewTreeOverlayMargin{}
		return nil
	}
	var number int
	if err := json.Unmarshal(data, &number); err == nil {
		*m = ViewTreeOverlayMargin{Set: true, Top: number, Right: number, Bottom: number, Left: number}
		return nil
	}
	var object struct {
		Top    int `json:"top,omitempty"`
		Right  int `json:"right,omitempty"`
		Bottom int `json:"bottom,omitempty"`
		Left   int `json:"left,omitempty"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	*m = ViewTreeOverlayMargin{Set: true, Top: object.Top, Right: object.Right, Bottom: object.Bottom, Left: object.Left}
	return nil
}

type ViewTreeFocus struct {
	MountID string `json:"mountId"`
	NodeID  string `json:"nodeId,omitempty"`
}

type ViewTreeEvent struct {
	MountID string         `json:"mountId"`
	NodeID  string         `json:"nodeId,omitempty"`
	Event   string         `json:"event"`
	Data    map[string]any `json:"data,omitempty"`
}

type ViewTreeChange struct {
	Type    string
	MountID string
	Slot    string
}

type viewTreeEventListener struct {
	id int
	fn func(ViewTreeEvent)
}

type viewTreeChangeListener struct {
	id int
	fn func(ViewTreeChange)
}

type ViewTreeHost struct {
	mu                   sync.RWMutex
	mounts               map[string]ViewTreeMount
	focus                ViewTreeFocus
	events               []ViewTreeEvent
	listeners            []viewTreeEventListener
	changeListeners      []viewTreeChangeListener
	nextListenerID       int
	nextChangeListenerID int
}

func NewViewTreeHost() *ViewTreeHost {
	return &ViewTreeHost{mounts: map[string]ViewTreeMount{}}
}

func (h *ViewTreeHost) Mount(mountID, slot string, view ViewTreeNode) error {
	return h.MountWithOptions(mountID, slot, view)
}

func (h *ViewTreeHost) MountWithOptions(mountID, slot string, view ViewTreeNode, options ...ViewTreeMountOptions) error {
	if h == nil {
		return errors.New("viewtree host is nil")
	}
	if strings.TrimSpace(mountID) == "" {
		return errors.New("viewtree mount id is required")
	}
	if strings.TrimSpace(slot) == "" {
		return errors.New("viewtree slot is required")
	}
	if err := ValidateViewTreeNode(view); err != nil {
		return err
	}
	canonicalSlot := canonicalViewTreeSlot(slot)
	priority := 0
	if len(options) > 0 {
		priority = options[0].Priority
	}
	h.mu.Lock()
	if h.mounts == nil {
		h.mounts = map[string]ViewTreeMount{}
	}
	var overlay *ViewTreeOverlayOptions
	if len(options) > 0 {
		overlay = options[0].Overlay
	}
	h.mounts[mountID] = ViewTreeMount{MountID: mountID, Slot: canonicalSlot, View: view, Priority: priority, Overlay: overlay}
	h.mu.Unlock()
	h.dispatchChange(ViewTreeChange{Type: "mount", MountID: mountID, Slot: canonicalSlot})
	_ = h.DispatchEvent(mountID, view.ID, "mount", nil)
	_ = h.DispatchVisibilityChange(mountID, true, "mount")
	return nil
}

func (h *ViewTreeHost) SetStatus(key, text string, priority int) error {
	if h == nil {
		return errors.New("viewtree host is nil")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("viewtree status key is required")
	}
	mountID := "status:" + key
	if strings.TrimSpace(text) == "" {
		h.Unmount(mountID)
		return nil
	}
	return h.MountWithOptions(mountID, "footer", ViewTreeNode{Type: "text", ID: mountID, Text: text}, ViewTreeMountOptions{Priority: priority})
}

func (h *ViewTreeHost) Unmount(mountID string) bool {
	if h == nil {
		return false
	}
	mount, ok := h.Mounted(mountID)
	if !ok {
		return false
	}
	if focus := h.Focused(); focus.MountID == mountID {
		_ = h.DispatchEvent(focus.MountID, focus.NodeID, "blur", nil)
	}
	_ = h.DispatchVisibilityChange(mountID, false, "unmount")
	_ = h.DispatchEvent(mountID, mount.View.ID, "unmount", nil)
	h.mu.Lock()
	if _, ok := h.mounts[mountID]; !ok {
		h.mu.Unlock()
		return false
	}
	if h.focus.MountID == mountID {
		h.focus = ViewTreeFocus{}
	}
	delete(h.mounts, mountID)
	h.mu.Unlock()
	h.dispatchChange(ViewTreeChange{Type: "unmount", MountID: mountID, Slot: mount.Slot})
	return true
}

func (h *ViewTreeHost) Mounted(mountID string) (ViewTreeMount, bool) {
	if h == nil {
		return ViewTreeMount{}, false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.mounts == nil {
		return ViewTreeMount{}, false
	}
	mount, ok := h.mounts[mountID]
	return mount, ok
}

func (h *ViewTreeHost) MountsBySlot(slot string) []ViewTreeMount {
	if h == nil {
		return nil
	}
	slot = canonicalViewTreeSlot(slot)
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.mounts == nil {
		return nil
	}
	var mounts []ViewTreeMount
	for _, mount := range h.mounts {
		if mount.Slot == slot {
			mounts = append(mounts, mount)
		}
	}
	sort.Slice(mounts, func(i, j int) bool {
		if mounts[i].Priority != mounts[j].Priority {
			return mounts[i].Priority > mounts[j].Priority
		}
		return mounts[i].MountID < mounts[j].MountID
	})
	return mounts
}

func (h *ViewTreeHost) RenderMount(mountID string, width int) ([]string, error) {
	mount, ok := h.Mounted(mountID)
	if !ok {
		return nil, fmt.Errorf("stale viewtree mount %q", mountID)
	}
	return RenderViewTree(mount.View, width), nil
}

func (h *ViewTreeHost) Patch(mountID string, ops []ViewTreePatchOperation) error {
	if h == nil {
		return errors.New("viewtree host is nil")
	}
	if len(ops) == 0 {
		return nil
	}
	h.mu.RLock()
	mount, ok := h.mounts[mountID]
	h.mu.RUnlock()
	if !ok {
		return fmt.Errorf("stale viewtree mount %q", mountID)
	}
	view, err := viewTreeNodeToAny(mount.View)
	if err != nil {
		return err
	}
	for _, op := range ops {
		if err := applyViewTreePatch(&view, op); err != nil {
			return err
		}
	}
	updated, err := anyToViewTreeNode(view)
	if err != nil {
		return err
	}
	if err := ValidateViewTreeNode(updated); err != nil {
		return err
	}
	h.mu.Lock()
	if _, ok := h.mounts[mountID]; !ok {
		h.mu.Unlock()
		return fmt.Errorf("stale viewtree mount %q", mountID)
	}
	mount.View = updated
	h.mounts[mountID] = mount
	h.mu.Unlock()
	h.dispatchChange(ViewTreeChange{Type: "patch", MountID: mountID, Slot: mount.Slot})
	return nil
}

func (h *ViewTreeHost) Focus(mountID, nodeID string) error {
	if h == nil {
		return errors.New("viewtree host is nil")
	}
	mount, ok := h.Mounted(mountID)
	if !ok {
		return fmt.Errorf("stale viewtree mount %q", mountID)
	}
	if nodeID != "" && !viewTreeNodeHasID(mount.View, nodeID) {
		return fmt.Errorf("viewtree node %q not found in mount %q", nodeID, mountID)
	}
	target := ViewTreeFocus{MountID: mountID, NodeID: nodeID}
	if h.Focused() == target {
		return nil
	}
	h.mu.Lock()
	previous := h.focus
	h.focus = target
	h.mu.Unlock()
	if previous.MountID != "" {
		_ = h.DispatchEvent(previous.MountID, previous.NodeID, "blur", nil)
	}
	return h.DispatchEvent(mountID, nodeID, "focus", nil)
}

func (h *ViewTreeHost) Blur(mountID, nodeID string) error {
	if h == nil {
		return errors.New("viewtree host is nil")
	}
	h.mu.Lock()
	if h.focus.MountID != mountID || h.focus.NodeID != nodeID {
		h.mu.Unlock()
		return nil
	}
	h.focus = ViewTreeFocus{}
	h.mu.Unlock()
	return h.DispatchEvent(mountID, nodeID, "blur", nil)
}

func (h *ViewTreeHost) Focused() ViewTreeFocus {
	if h == nil {
		return ViewTreeFocus{}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.focus
}

func (h *ViewTreeHost) DispatchEvent(mountID, nodeID, event string, data map[string]any) error {
	return h.dispatchEvent(mountID, nodeID, event, data, true)
}

func (h *ViewTreeHost) DispatchTick(frame int64) error {
	return h.dispatchSubscribedEvent("tick", map[string]any{"frame": frame}, false)
}

func (h *ViewTreeHost) DispatchResize(width, height int) error {
	return h.dispatchSubscribedEvent("resize", map[string]any{"width": width, "height": height}, false)
}

func (h *ViewTreeHost) DispatchThemeChange(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("theme name is required")
	}
	return h.dispatchSubscribedEvent("theme_change", map[string]any{"name": name, "preview": false}, false)
}

func (h *ViewTreeHost) DispatchVisibilityChange(mountID string, visible bool, reason string) error {
	if h == nil {
		return errors.New("viewtree host is nil")
	}
	mountID = strings.TrimSpace(mountID)
	if mountID == "" {
		return errors.New("viewtree mount id is required")
	}
	mount, ok := h.Mounted(mountID)
	if !ok {
		return fmt.Errorf("stale viewtree mount %q", mountID)
	}
	data := map[string]any{"visible": visible}
	if reason = strings.TrimSpace(reason); reason != "" {
		data["reason"] = reason
	}
	for _, nodeID := range viewTreeNodeEventTargets(mount.View, "visibility_change") {
		if err := h.dispatchEvent(mountID, nodeID, "visibility_change", data, false); err != nil {
			return err
		}
	}
	return nil
}

func (h *ViewTreeHost) dispatchSubscribedEvent(event string, data map[string]any, recordEvent bool) error {
	if h == nil {
		return errors.New("viewtree host is nil")
	}
	targets := h.eventTargets(event)
	for _, target := range targets {
		if err := h.dispatchEvent(target.MountID, target.NodeID, event, data, recordEvent); err != nil {
			return err
		}
	}
	return nil
}

func (h *ViewTreeHost) HasEventSubscription(event string) bool {
	if h == nil {
		return false
	}
	return len(h.eventTargets(event)) > 0
}

type viewTreeEventTarget struct {
	MountID string
	NodeID  string
}

func (h *ViewTreeHost) eventTargets(event string) []viewTreeEventTarget {
	event = strings.TrimSpace(event)
	if h == nil || event == "" {
		return nil
	}
	h.mu.RLock()
	mounts := make([]ViewTreeMount, 0, len(h.mounts))
	for _, mount := range h.mounts {
		mounts = append(mounts, mount)
	}
	h.mu.RUnlock()
	var targets []viewTreeEventTarget
	for _, mount := range mounts {
		for _, nodeID := range viewTreeNodeEventTargets(mount.View, event) {
			targets = append(targets, viewTreeEventTarget{MountID: mount.MountID, NodeID: nodeID})
		}
	}
	return targets
}

func viewTreeNodeEventTargets(node ViewTreeNode, event string) []string {
	var targets []string
	var visit func(ViewTreeNode)
	visit = func(current ViewTreeNode) {
		if containsString(current.Events, event) {
			targets = append(targets, current.ID)
		}
		for _, child := range current.Children {
			visit(child)
		}
		for _, item := range current.Items {
			if item.Node != nil {
				visit(*item.Node)
			}
		}
	}
	visit(node)
	return targets
}

func (h *ViewTreeHost) dispatchEvent(mountID, nodeID, event string, data map[string]any, recordEvent bool) error {
	if h == nil {
		return errors.New("viewtree host is nil")
	}
	mount, ok := h.Mounted(mountID)
	if !ok {
		return fmt.Errorf("stale viewtree mount %q", mountID)
	}
	if nodeID != "" && !viewTreeNodeHasID(mount.View, nodeID) {
		return fmt.Errorf("viewtree node %q not found in mount %q", nodeID, mountID)
	}
	if strings.TrimSpace(event) == "" {
		return errors.New("viewtree event is required")
	}
	record := ViewTreeEvent{MountID: mountID, NodeID: nodeID, Event: event, Data: cloneMapAny(data)}
	h.mu.Lock()
	if recordEvent {
		h.events = append(h.events, record)
	}
	listeners := append([]viewTreeEventListener(nil), h.listeners...)
	h.mu.Unlock()
	for _, listener := range listeners {
		if listener.fn != nil {
			listener.fn(record)
		}
	}
	return nil
}

func (h *ViewTreeHost) Events() []ViewTreeEvent {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]ViewTreeEvent(nil), h.events...)
}

func (h *ViewTreeHost) OnEvent(listener func(ViewTreeEvent)) func() {
	if h == nil || listener == nil {
		return func() {}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextListenerID++
	id := h.nextListenerID
	h.listeners = append(h.listeners, viewTreeEventListener{id: id, fn: listener})
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for index, candidate := range h.listeners {
			if candidate.id == id {
				h.listeners = append(h.listeners[:index], h.listeners[index+1:]...)
				return
			}
		}
	}
}

func (h *ViewTreeHost) OnChange(listener func(ViewTreeChange)) func() {
	if h == nil || listener == nil {
		return func() {}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextChangeListenerID++
	id := h.nextChangeListenerID
	h.changeListeners = append(h.changeListeners, viewTreeChangeListener{id: id, fn: listener})
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for index, candidate := range h.changeListeners {
			if candidate.id == id {
				h.changeListeners = append(h.changeListeners[:index], h.changeListeners[index+1:]...)
				return
			}
		}
	}
}

func (h *ViewTreeHost) dispatchChange(change ViewTreeChange) {
	if h == nil {
		return
	}
	h.mu.RLock()
	listeners := append([]viewTreeChangeListener(nil), h.changeListeners...)
	h.mu.RUnlock()
	for _, listener := range listeners {
		if listener.fn != nil {
			listener.fn(change)
		}
	}
}

type ViewTreeComponent struct {
	View    ViewTreeNode
	Host    *ViewTreeHost
	MountID string
	focused bool
}

func NewViewTreeComponent(view ViewTreeNode) *ViewTreeComponent {
	return &ViewTreeComponent{View: view}
}

func (c *ViewTreeComponent) Invalidate() {}

func (c *ViewTreeComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	if c.Host != nil && c.MountID != "" {
		lines, err := c.Host.RenderMount(c.MountID, width)
		if err != nil {
			return []string{err.Error()}
		}
		return lines
	}
	return RenderViewTree(c.View, width)
}

func NewMountedViewTreeComponent(host *ViewTreeHost, mountID string) *ViewTreeComponent {
	return &ViewTreeComponent{Host: host, MountID: mountID}
}

func (c *ViewTreeComponent) HandleInput(data string) {
	if c == nil || c.Host == nil || c.MountID == "" {
		return
	}
	nodeID := c.focusNodeID()
	_ = c.Host.DispatchEvent(c.MountID, nodeID, "key", map[string]any{"key": data})
	switch data {
	case "\r", "\n":
		_ = c.Host.DispatchEvent(c.MountID, nodeID, "submit", map[string]any{"key": "enter", "raw": data})
		if viewTreeNodeTypeSelectable(c.focusNodeType()) {
			_ = c.Host.DispatchEvent(c.MountID, nodeID, "select", map[string]any{"id": nodeID, "value": nodeID})
		}
	case "\x1b":
		_ = c.Host.DispatchEvent(c.MountID, nodeID, "cancel", map[string]any{"key": "escape", "raw": data})
	default:
		if text := viewTreeTextInputValue(data); text != "" {
			_ = c.Host.DispatchEvent(c.MountID, nodeID, "textInput", map[string]any{"text": text, "raw": data})
			if viewTreeNodeTypeTextual(c.focusNodeType()) {
				_ = c.Host.DispatchEvent(c.MountID, nodeID, "change", map[string]any{"text": text, "value": text, "raw": data})
			}
		}
	}
}

func (c *ViewTreeComponent) SetFocused(focused bool) {
	if c == nil {
		return
	}
	c.focused = focused
	if c.Host == nil || c.MountID == "" {
		return
	}
	if focused {
		_ = c.Host.Focus(c.MountID, c.focusNodeID())
	} else {
		_ = c.Host.Blur(c.MountID, c.focusNodeID())
	}
}

func (c *ViewTreeComponent) Focused() bool {
	if c == nil {
		return false
	}
	return c.focused
}

func (c *ViewTreeComponent) focusNodeID() string {
	if c == nil {
		return ""
	}
	if c.View.ID != "" {
		return c.View.ID
	}
	if c.Host != nil && c.MountID != "" {
		if mount, ok := c.Host.Mounted(c.MountID); ok {
			return mount.View.ID
		}
	}
	return ""
}

func (c *ViewTreeComponent) focusNodeType() string {
	if c == nil {
		return ""
	}
	nodeID := c.focusNodeID()
	if nodeID == "" {
		return ""
	}
	if c.View.ID == nodeID {
		return c.View.Type
	}
	if c.Host != nil && c.MountID != "" {
		if mount, ok := c.Host.Mounted(c.MountID); ok {
			if nodeType, ok := viewTreeNodeTypeByID(mount.View, nodeID); ok {
				return nodeType
			}
		}
	}
	return ""
}

func viewTreeNodeTypeByID(node ViewTreeNode, id string) (string, bool) {
	if id == "" {
		return "", false
	}
	if node.ID == id {
		return node.Type, true
	}
	for _, child := range node.Children {
		if nodeType, ok := viewTreeNodeTypeByID(child, id); ok {
			return nodeType, true
		}
	}
	for _, item := range node.Items {
		if item.Node != nil {
			if nodeType, ok := viewTreeNodeTypeByID(*item.Node, id); ok {
				return nodeType, true
			}
		}
		if item.Item != nil {
			if viewTreeListItemHasID(*item.Item, id) {
				return "listItem", true
			}
		}
	}
	return "", false
}

func viewTreeNodeTypeSelectable(nodeType string) bool {
	switch nodeType {
	case "button", "list", "select", "tree", "listItem":
		return true
	default:
		return false
	}
}

func viewTreeNodeTypeTextual(nodeType string) bool {
	return nodeType == "input" || nodeType == "textarea"
}

func viewTreeTextInputValue(data string) string {
	if data == "" {
		return ""
	}
	for _, r := range data {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return data
}

func RenderViewTree(view ViewTreeNode, width int) []string {
	return viewTreeRenderer{width: width}.renderNode(view)
}

type viewTreeRenderer struct {
	width int
}

func (r viewTreeRenderer) renderNode(node ViewTreeNode) []string {
	if r.width <= 0 {
		r.width = 1
	}
	switch node.Type {
	case "text", "markdown", "message":
		return r.wrap(firstNonEmptyString(node.Text, node.Label))
	case "keyHint":
		return r.wrap("<" + firstNonEmptyString(node.Text, node.Label) + ">")
	case "button":
		return r.wrap("[ " + firstNonEmptyString(node.Text, node.Label, "button") + " ]")
	case "spacer":
		return []string{""}
	case "box", "column", "portal":
		return r.renderChildren(node.Children)
	case "row":
		return r.renderRow(node.Children)
	case "list", "select", "tree":
		return r.renderItems(node.Items, 0)
	case "table":
		return r.renderTable(node)
	case "progress":
		return r.renderProgress(node)
	case "spinner":
		return r.wrap(firstNonEmptyString(node.Text, node.Label, "..."))
	case "input", "textarea":
		return r.wrap("> " + node.Text)
	case "diff", "image", "toolCall":
		return r.renderFallbackOrDiagnostic(node)
	default:
		return r.renderFallbackOrDiagnostic(node)
	}
}

func (r viewTreeRenderer) renderChildren(children []ViewTreeNode) []string {
	var lines []string
	for _, child := range children {
		lines = append(lines, r.renderNode(child)...)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func (r viewTreeRenderer) renderRow(children []ViewTreeNode) []string {
	var parts []string
	for _, child := range children {
		rendered := r.renderNode(child)
		if len(rendered) > 0 {
			parts = append(parts, strings.TrimSpace(rendered[0]))
		}
	}
	return []string{gitui.TruncateToWidth(strings.Join(parts, " "), r.width, "...")}
}

func (r viewTreeRenderer) renderItems(items []ViewTreeItem, depth int) []string {
	var lines []string
	for _, item := range items {
		if item.Node != nil {
			lines = append(lines, r.renderNode(*item.Node)...)
			continue
		}
		if item.Item == nil {
			continue
		}
		prefix := strings.Repeat("  ", depth) + "- "
		if item.Item.Checked != nil {
			if *item.Item.Checked {
				prefix = strings.Repeat("  ", depth) + "[x] "
			} else {
				prefix = strings.Repeat("  ", depth) + "[ ] "
			}
		}
		lines = append(lines, gitui.TruncateToWidth(prefix+item.Item.Text, r.width, "..."))
		childItems := make([]ViewTreeItem, 0, len(item.Item.Children))
		for index := range item.Item.Children {
			childItems = append(childItems, ViewTreeItem{Item: &item.Item.Children[index]})
		}
		lines = append(lines, r.renderItems(childItems, depth+1)...)
	}
	if len(lines) == 0 && depth == 0 {
		return []string{""}
	}
	return lines
}

func (r viewTreeRenderer) renderTable(node ViewTreeNode) []string {
	if len(node.Columns) == 0 {
		return r.renderFallbackOrDiagnostic(node)
	}
	headers := make([]string, 0, len(node.Columns))
	for _, column := range node.Columns {
		headers = append(headers, column.Title)
	}
	lines := []string{gitui.TruncateToWidth(strings.Join(headers, " | "), r.width, "...")}
	for _, row := range node.Rows {
		var cells []string
		for _, column := range node.Columns {
			cells = append(cells, fmt.Sprint(row[column.ID]))
		}
		lines = append(lines, gitui.TruncateToWidth(strings.Join(cells, " | "), r.width, "..."))
	}
	return lines
}

func (r viewTreeRenderer) renderProgress(node ViewTreeNode) []string {
	value := 0.0
	if node.Value != nil {
		value = *node.Value
	}
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	barWidth := max(4, min(20, r.width-8))
	filled := int(value * float64(barWidth))
	text := "[" + strings.Repeat("=", filled) + strings.Repeat(".", barWidth-filled) + "]"
	if label := firstNonEmptyString(node.Text, node.Label); label != "" {
		text = label + " " + text
	}
	return []string{gitui.TruncateToWidth(text, r.width, "...")}
}

func (r viewTreeRenderer) renderFallbackOrDiagnostic(node ViewTreeNode) []string {
	if node.Fallback != nil {
		return r.renderNode(*node.Fallback)
	}
	return r.wrap("[unsupported view node: " + firstNonEmptyString(node.Type, "unknown") + "]")
}

func (r viewTreeRenderer) wrap(text string) []string {
	lines := gitui.WrapTextWithANSI(text, r.width)
	for index, line := range lines {
		lines[index] = gitui.TruncateToWidth(line, r.width, "...")
	}
	return lines
}

func viewTreeNodeToAny(node ViewTreeNode) (any, error) {
	data, err := json.Marshal(node)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func anyToViewTreeNode(value any) (ViewTreeNode, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return ViewTreeNode{}, err
	}
	var node ViewTreeNode
	if err := json.Unmarshal(data, &node); err != nil {
		return ViewTreeNode{}, err
	}
	return node, nil
}

func viewTreeNodeHasID(node ViewTreeNode, id string) bool {
	if id == "" {
		return true
	}
	if node.ID == id {
		return true
	}
	for _, child := range node.Children {
		if viewTreeNodeHasID(child, id) {
			return true
		}
	}
	for _, item := range node.Items {
		if item.Node != nil && viewTreeNodeHasID(*item.Node, id) {
			return true
		}
		if item.Item != nil && viewTreeListItemHasID(*item.Item, id) {
			return true
		}
	}
	return false
}

func viewTreeListItemHasID(item ViewTreeListItem, id string) bool {
	if item.ID == id {
		return true
	}
	for _, child := range item.Children {
		if viewTreeListItemHasID(child, id) {
			return true
		}
	}
	return false
}

func cloneMapAny(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	copied := make(map[string]any, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

func canonicalViewTreeSlot(slot string) string {
	switch strings.TrimSpace(slot) {
	case "widget.aboveEditor":
		return "aboveEditor"
	case "widget.belowEditor":
		return "belowEditor"
	default:
		return strings.TrimSpace(slot)
	}
}

func applyViewTreePatch(root *any, op ViewTreePatchOperation) error {
	if root == nil {
		return errors.New("viewtree patch root is nil")
	}
	if op.Op != "replace" && op.Op != "add" && op.Op != "remove" {
		return fmt.Errorf("unsupported viewtree patch op %q", op.Op)
	}
	tokens, err := parseViewTreePatchPath(op.Path)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		if op.Op == "remove" {
			*root = nil
		} else {
			*root = op.Value
		}
		return nil
	}
	parent, key, err := viewTreePatchParent(*root, tokens)
	if err != nil {
		return err
	}
	switch target := parent.(type) {
	case map[string]any:
		if op.Op == "remove" {
			delete(target, key)
			return nil
		}
		if op.Op == "replace" {
			if _, ok := target[key]; !ok {
				return fmt.Errorf("viewtree patch path %q does not exist", op.Path)
			}
		}
		target[key] = op.Value
		return nil
	case []any:
		index, err := viewTreePatchArrayIndex(key, len(target), op.Op == "add")
		if err != nil {
			return err
		}
		if op.Op == "remove" {
			target = append(target[:index], target[index+1:]...)
		} else if op.Op == "add" {
			if key == "-" || index == len(target) {
				target = append(target, op.Value)
			} else {
				target = append(target[:index+1], target[index:]...)
				target[index] = op.Value
			}
		} else {
			target[index] = op.Value
		}
		return viewTreeSetParentChild(root, tokens[:len(tokens)-1], target)
	default:
		return fmt.Errorf("viewtree patch parent for %q is not addressable", op.Path)
	}
}

func parseViewTreePatchPath(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("viewtree patch path %q must start with /", path)
	}
	raw := strings.Split(path[1:], "/")
	tokens := make([]string, 0, len(raw))
	for _, token := range raw {
		token = strings.ReplaceAll(token, "~1", "/")
		token = strings.ReplaceAll(token, "~0", "~")
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func viewTreePatchParent(root any, tokens []string) (any, string, error) {
	current := root
	for _, token := range tokens[:len(tokens)-1] {
		next, err := viewTreePatchChild(current, token)
		if err != nil {
			return nil, "", err
		}
		current = next
	}
	return current, tokens[len(tokens)-1], nil
}

func viewTreePatchChild(value any, token string) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		child, ok := typed[token]
		if !ok {
			return nil, fmt.Errorf("viewtree patch path segment %q does not exist", token)
		}
		return child, nil
	case []any:
		index, err := viewTreePatchArrayIndex(token, len(typed), false)
		if err != nil {
			return nil, err
		}
		return typed[index], nil
	default:
		return nil, fmt.Errorf("viewtree patch segment %q is not addressable", token)
	}
}

func viewTreePatchArrayIndex(token string, length int, allowAppend bool) (int, error) {
	if token == "-" {
		if allowAppend {
			return length, nil
		}
		return 0, errors.New("viewtree patch '-' index is only valid for add")
	}
	index, err := strconv.Atoi(token)
	if err != nil {
		return 0, fmt.Errorf("viewtree patch array index %q is invalid", token)
	}
	if index < 0 || index >= length {
		if allowAppend && index == length {
			return index, nil
		}
		return 0, fmt.Errorf("viewtree patch array index %d out of range", index)
	}
	return index, nil
}

func viewTreeSetParentChild(root *any, parentTokens []string, value any) error {
	if len(parentTokens) == 0 {
		*root = value
		return nil
	}
	parent, key, err := viewTreePatchParent(*root, parentTokens)
	if err != nil {
		return err
	}
	switch typed := parent.(type) {
	case map[string]any:
		typed[key] = value
		return nil
	case []any:
		index, err := viewTreePatchArrayIndex(key, len(typed), false)
		if err != nil {
			return err
		}
		typed[index] = value
		return viewTreeSetParentChild(root, parentTokens[:len(parentTokens)-1], typed)
	default:
		return fmt.Errorf("viewtree patch parent for %q is not addressable", strings.Join(parentTokens, "/"))
	}
}
