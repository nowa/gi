package gicodingagent

import (
	"sync"

	llm "github.com/nowa/gi/gi-llm-provider"
)

// cliCustomMessageComponent owns the presentation state passed to an
// extension's custom-message renderer. The persisted message stays immutable;
// display settings can change independently and are sampled together on each
// render.
type cliCustomMessageComponent struct {
	mu sync.RWMutex

	message   llm.Message
	renderer  ProtocolMessageRenderer
	expanded  bool
	outputPad int
}

func newCLICustomMessageComponent(
	message llm.Message,
	renderer ProtocolMessageRenderer,
	outputPad int,
) *cliCustomMessageComponent {
	return &cliCustomMessageComponent{
		message:   cloneSessionMessage(message),
		renderer:  renderer,
		outputPad: normalizeOutputPad(outputPad),
	}
}

func (c *cliCustomMessageComponent) SetExpanded(expanded bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.expanded = expanded
	c.mu.Unlock()
}

func (c *cliCustomMessageComponent) SetOutputPad(outputPad int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.outputPad = normalizeOutputPad(outputPad)
	c.mu.Unlock()
}

func (c *cliCustomMessageComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	message := cloneSessionMessage(c.message)
	renderer := c.renderer
	expanded := c.expanded
	outputPad := c.outputPad
	c.mu.RUnlock()
	if renderer == nil {
		return nil
	}
	return normalizeRenderedLines(renderer(message, map[string]any{
		"width":     width,
		"expanded":  expanded,
		"outputPad": outputPad,
	}))
}

func (c *cliCustomMessageComponent) Invalidate() {}
