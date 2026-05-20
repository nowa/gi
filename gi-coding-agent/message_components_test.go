package gicodingagent

import (
	"strings"
	"testing"
)

func TestAssistantMessageComponentOSC133Markers(t *testing.T) {
	t.Run("adds OSC 133 zone markers to assistant messages without tool calls", func(t *testing.T) {
		component := NewAssistantMessageComponent([]AssistantContentBlock{{Type: "text", Text: "hello"}})
		lines := component.Render(40)

		if len(lines) == 0 {
			t.Fatalf("lines should not be empty")
		}
		if !strings.Contains(lines[0], OSC133ZoneStart) {
			t.Fatalf("first line = %q", lines[0])
		}
		if !strings.HasPrefix(lines[len(lines)-1], OSC133ZoneEnd+OSC133ZoneFinal) {
			t.Fatalf("last line = %q", lines[len(lines)-1])
		}
	})

	t.Run("does not add OSC 133 zone markers when assistant message contains tool calls", func(t *testing.T) {
		component := NewAssistantMessageComponent([]AssistantContentBlock{
			{Type: "text", Text: "calling tool"},
			{Type: "toolCall", ID: "tool-1", Name: "read", Arguments: map[string]any{"path": "file.txt"}},
		})
		rendered := strings.Join(component.Render(60), "\n")

		for _, marker := range []string{OSC133ZoneStart, OSC133ZoneEnd, OSC133ZoneFinal} {
			if strings.Contains(rendered, marker) {
				t.Fatalf("rendered should not contain %q: %q", marker, rendered)
			}
		}
	})
}

func TestUserMessageComponentOSC133Markers(t *testing.T) {
	component := NewUserMessageComponent("hello")
	lines := component.Render(20)

	if len(lines) != 3 {
		t.Fatalf("line count = %d, lines=%#v", len(lines), lines)
	}
	if !strings.Contains(lines[0], OSC133ZoneStart) || !strings.HasSuffix(lines[0], TerminalBGReset) {
		t.Fatalf("first line = %q", lines[0])
	}
	if strings.Contains(lines[0], OSC133ZoneEnd) {
		t.Fatalf("first line should not contain closing marker: %q", lines[0])
	}
	if !strings.Contains(lines[1], "hello") {
		t.Fatalf("content line = %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], OSC133ZoneEnd+OSC133ZoneFinal) || !strings.HasSuffix(lines[2], TerminalBGReset) {
		t.Fatalf("closing line = %q", lines[2])
	}
}
