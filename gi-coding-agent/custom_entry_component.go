package gicodingagent

import (
	"fmt"
	"strings"
	"sync"

	gitui "github.com/nowa/gi/gi-tui"
)

// CustomEntryComponent owns only presentation state for one persisted custom
// entry. The session manager remains the source of truth for entry data, while
// the extension runtime owns renderer registration and precedence.
type CustomEntryComponent struct {
	mu         sync.RWMutex
	entry      FileEntry
	renderer   ProtocolEntryRenderer
	expanded   bool
	generation uint64
	component  gitui.Component
}

func NewCustomEntryComponent(
	entry FileEntry,
	renderer ProtocolEntryRenderer,
) *CustomEntryComponent {
	component := &CustomEntryComponent{
		entry:      cloneFileEntry(entry),
		renderer:   renderer,
		generation: 1,
	}
	component.rebuild()
	return component
}

func (c *CustomEntryComponent) HasContent() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.component != nil
}

func (c *CustomEntryComponent) SetExpanded(expanded bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.expanded == expanded {
		c.mu.Unlock()
		return
	}
	c.expanded = expanded
	c.generation++
	c.mu.Unlock()
	c.rebuild()
}

func (c *CustomEntryComponent) Invalidate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.generation++
	previous := c.component
	c.mu.Unlock()
	if previous != nil {
		previous.Invalidate()
	}
	c.rebuild()
}

func (c *CustomEntryComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	component := c.component
	c.mu.RUnlock()
	if component == nil {
		return nil
	}
	lines := component.Render(width)
	return append([]string{""}, lines...)
}

func (c *CustomEntryComponent) rebuild() {
	if c == nil {
		return
	}
	c.mu.RLock()
	entry := cloneFileEntry(c.entry)
	renderer := c.renderer
	expanded := c.expanded
	generation := c.generation
	c.mu.RUnlock()

	component, err := invokeProtocolEntryRenderer(
		renderer,
		entry,
		ProtocolEntryRenderOptions{Expanded: expanded},
	)
	if err != nil {
		component = customEntryRendererErrorComponent(entry.CustomType, err)
	}

	c.mu.Lock()
	if c.generation == generation {
		c.component = component
	}
	c.mu.Unlock()
}

func invokeProtocolEntryRenderer(
	renderer ProtocolEntryRenderer,
	entry FileEntry,
	options ProtocolEntryRenderOptions,
) (component gitui.Component, err error) {
	if renderer == nil {
		return nil, nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
		}
	}()
	return renderer(entry, options), nil
}

func customEntryRendererErrorComponent(
	customType string,
	err error,
) gitui.Component {
	customType = strings.TrimSpace(customType)
	if customType == "" {
		customType = "custom"
	}
	box := gitui.NewBox(
		1,
		1,
		func(text string) string {
			return tuiThemeBG("customMessageBg", text)
		},
	)
	box.AddChild(gitui.NewText(
		tuiThemeError("["+customType+"] renderer failed: "+err.Error()),
		0,
		0,
	))
	return box
}
