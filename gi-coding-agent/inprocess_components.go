package gicodingagent

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	gitui "github.com/nowa/gi/gi-tui"
)

type InProcessComponentContext struct {
	Session     *AgentSession
	RuntimeHost PrintModeRuntimeHost
	ViewTree    *ViewTreeHost
}

type InProcessComponentFactory func(InProcessComponentContext) (gitui.Component, func(), error)
type InProcessCustomDone func(result any)
type InProcessCustomComponentFactory func(InProcessComponentContext, InProcessCustomDone) (gitui.Component, func(), error)

type InProcessCustomOptions struct {
	Slot    string
	Overlay *gitui.OverlayOptions
}

type InProcessCustomResult struct {
	Value     any
	Err       error
	Cancelled bool
}

type InProcessCustomHandle struct {
	registry *InProcessUIRegistry
	key      string
	done     chan InProcessCustomResult
	once     sync.Once
}

type InProcessSlotRegistration struct {
	Key     string
	Slot    string
	Factory InProcessComponentFactory
	Version int
	Overlay *gitui.OverlayOptions
}

type InProcessUIRegistry struct {
	mu            sync.Mutex
	slots         map[string]InProcessSlotRegistration
	version       int
	watchers      map[int]func()
	nextWatcherID int
}

func NewInProcessUIRegistry() *InProcessUIRegistry {
	return &InProcessUIRegistry{slots: map[string]InProcessSlotRegistration{}}
}

func (r *InProcessUIRegistry) SetWidget(key, slot string, factory InProcessComponentFactory) {
	r.SetSlot(key, slot, factory)
}

func (r *InProcessUIRegistry) SetHeader(key string, factory InProcessComponentFactory) {
	r.SetSlot(key, "header", factory)
}

func (r *InProcessUIRegistry) SetFooter(key string, factory InProcessComponentFactory) {
	r.SetSlot(key, "footer", factory)
}

func (r *InProcessUIRegistry) SetEditor(key string, factory InProcessComponentFactory) {
	r.SetSlot(key, "editor", factory)
}

func (r *InProcessUIRegistry) SetOverlay(key string, factory InProcessComponentFactory, options ...gitui.OverlayOptions) {
	var overlay *gitui.OverlayOptions
	if len(options) > 0 {
		value := options[0]
		overlay = &value
	}
	r.setSlot(key, "overlay", factory, overlay)
}

func (r *InProcessUIRegistry) SetSlot(key, slot string, factory InProcessComponentFactory) {
	r.setSlot(key, slot, factory, nil)
}

func (r *InProcessUIRegistry) ShowCustom(key string, factory InProcessCustomComponentFactory, options ...InProcessCustomOptions) (*InProcessCustomHandle, error) {
	if r == nil {
		return nil, fmt.Errorf("in-process UI registry is nil")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("in-process custom component key is required")
	}
	if factory == nil {
		return nil, fmt.Errorf("in-process custom component factory is required")
	}
	option := InProcessCustomOptions{Slot: "editor"}
	if len(options) > 0 {
		option = options[0]
	}
	handle := &InProcessCustomHandle{
		registry: r,
		key:      key,
		done:     make(chan InProcessCustomResult, 1),
	}
	done := func(result any) {
		if handle.complete(InProcessCustomResult{Value: result}) {
			r.Remove(key)
		}
	}
	wrapped := func(ctx InProcessComponentContext) (gitui.Component, func(), error) {
		component, dispose, err := factory(ctx, done)
		if err != nil {
			if handle.complete(InProcessCustomResult{Err: err}) {
				go r.Remove(key)
			}
			return nil, nil, err
		}
		wrappedDispose := func() {
			defer handle.complete(InProcessCustomResult{Cancelled: true})
			if dispose != nil {
				dispose()
			}
		}
		return component, wrappedDispose, nil
	}
	if option.Overlay != nil {
		r.SetOverlay(key, wrapped, *option.Overlay)
	} else {
		r.SetSlot(key, firstNonEmptyString(option.Slot, "editor"), wrapped)
	}
	return handle, nil
}

func (h *InProcessCustomHandle) Done() <-chan InProcessCustomResult {
	if h == nil {
		done := make(chan InProcessCustomResult)
		close(done)
		return done
	}
	return h.done
}

func (h *InProcessCustomHandle) Close() bool {
	if h == nil {
		return false
	}
	completed := h.complete(InProcessCustomResult{Cancelled: true})
	if h.registry != nil {
		h.registry.Remove(h.key)
	}
	return completed
}

func (h *InProcessCustomHandle) complete(result InProcessCustomResult) bool {
	if h == nil {
		return false
	}
	completed := false
	h.once.Do(func() {
		h.done <- result
		close(h.done)
		completed = true
	})
	return completed
}

func (r *InProcessUIRegistry) setSlot(key, slot string, factory InProcessComponentFactory, overlay *gitui.OverlayOptions) {
	if r == nil || strings.TrimSpace(key) == "" || factory == nil {
		return
	}
	key = strings.TrimSpace(key)
	r.mu.Lock()
	if r.slots == nil {
		r.slots = map[string]InProcessSlotRegistration{}
	}
	r.version++
	r.slots[key] = InProcessSlotRegistration{
		Key:     key,
		Slot:    normalizeInProcessSlot(slot),
		Factory: factory,
		Version: r.version,
		Overlay: overlay,
	}
	watchers := r.snapshotWatchersLocked()
	r.mu.Unlock()
	notifyInProcessWatchers(watchers)
}

func (r *InProcessUIRegistry) Remove(key string) bool {
	if r == nil || strings.TrimSpace(key) == "" {
		return false
	}
	key = strings.TrimSpace(key)
	r.mu.Lock()
	if _, ok := r.slots[key]; !ok {
		r.mu.Unlock()
		return false
	}
	r.version++
	delete(r.slots, key)
	watchers := r.snapshotWatchersLocked()
	r.mu.Unlock()
	notifyInProcessWatchers(watchers)
	return true
}

func (r *InProcessUIRegistry) Slots() []InProcessSlotRegistration {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]InProcessSlotRegistration, 0, len(r.slots))
	for _, registration := range r.slots {
		result = append(result, registration)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Slot == result[j].Slot {
			return result[i].Key < result[j].Key
		}
		return result[i].Slot < result[j].Slot
	})
	return result
}

func (r *InProcessUIRegistry) OnChange(listener func()) func() {
	if r == nil || listener == nil {
		return func() {}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.watchers == nil {
		r.watchers = map[int]func(){}
	}
	r.nextWatcherID++
	id := r.nextWatcherID
	r.watchers[id] = listener
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.watchers, id)
	}
}

func (r *InProcessUIRegistry) snapshotWatchersLocked() []func() {
	if len(r.watchers) == 0 {
		return nil
	}
	watchers := make([]func(), 0, len(r.watchers))
	for _, watcher := range r.watchers {
		watchers = append(watchers, watcher)
	}
	return watchers
}

func notifyInProcessWatchers(watchers []func()) {
	for _, watcher := range watchers {
		if watcher != nil {
			watcher()
		}
	}
}

type safeInProcessComponent struct {
	key       string
	component gitui.Component
	onError   func(string)
}

func (c *safeInProcessComponent) Render(width int) (lines []string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			message := fmt.Sprintf("in-process component %s render failed: %v", c.key, recovered)
			lines = []string{message}
		}
	}()
	if c.component == nil {
		return nil
	}
	return c.component.Render(width)
}

func (c *safeInProcessComponent) RenderWithSize(width, height int) (lines []string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			message := fmt.Sprintf("in-process component %s render failed: %v", c.key, recovered)
			lines = []string{message}
		}
	}()
	if c.component == nil {
		return nil
	}
	if sized, ok := c.component.(gitui.SizeAwareComponent); ok {
		return sized.RenderWithSize(width, height)
	}
	return c.component.Render(width)
}

func (c *safeInProcessComponent) Invalidate() {
	defer func() {
		if recovered := recover(); recovered != nil {
			c.report(fmt.Sprintf("in-process component %s invalidate failed: %v", c.key, recovered))
		}
	}()
	if c.component != nil {
		c.component.Invalidate()
	}
}

func (c *safeInProcessComponent) HandleInput(data string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			c.report(fmt.Sprintf("in-process component %s input failed: %v", c.key, recovered))
		}
	}()
	handler, ok := c.component.(gitui.InputHandler)
	if !ok {
		return
	}
	handler.HandleInput(data)
}

func (c *safeInProcessComponent) SetFocused(focused bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			c.report(fmt.Sprintf("in-process component %s focus failed: %v", c.key, recovered))
		}
	}()
	focusable, ok := c.component.(gitui.Focusable)
	if !ok {
		return
	}
	focusable.SetFocused(focused)
}

func (c *safeInProcessComponent) Focused() (focused bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			c.report(fmt.Sprintf("in-process component %s focus failed: %v", c.key, recovered))
			focused = false
		}
	}()
	focusable, ok := c.component.(gitui.Focusable)
	if !ok {
		return false
	}
	return focusable.Focused()
}

func (c *safeInProcessComponent) WantsKeyRelease() (wants bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			c.report(fmt.Sprintf("in-process component %s key-release query failed: %v", c.key, recovered))
			wants = false
		}
	}()
	receiver, ok := c.component.(gitui.KeyReleaseReceiver)
	if !ok {
		return false
	}
	return receiver.WantsKeyRelease()
}

func (c *safeInProcessComponent) SetExpanded(expanded bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			c.report(fmt.Sprintf("in-process component %s expansion failed: %v", c.key, recovered))
		}
	}()
	expandable, ok := c.component.(interface{ SetExpanded(bool) })
	if !ok {
		return
	}
	expandable.SetExpanded(expanded)
}

func (c *safeInProcessComponent) report(message string) {
	if c != nil && c.onError != nil {
		c.onError(message)
	}
}

func normalizeInProcessSlot(slot string) string {
	switch strings.TrimSpace(slot) {
	case "header", "aboveEditor", "belowEditor", "footer", "editor", "overlay":
		return strings.TrimSpace(slot)
	case "widget", "above-editor", "above_editor", "":
		return "aboveEditor"
	case "below-editor", "below_editor":
		return "belowEditor"
	default:
		return "aboveEditor"
	}
}
