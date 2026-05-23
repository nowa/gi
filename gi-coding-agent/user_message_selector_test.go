package gicodingagent

import (
	"strings"
	"testing"
)

func TestUserMessageSelectorComponentDefaultsToMostRecentPiStyle(t *testing.T) {
	selector := NewUserMessageSelectorComponent([]AgentSessionForkMessage{
		{EntryID: "first", Text: "first message"},
		{EntryID: "second", Text: "second\nmessage"},
	}, "")
	rendered := strings.Join(selector.Render(80), "\n")
	if !strings.Contains(rendered, "Fork from Message") ||
		!strings.Contains(rendered, "Select a user message") ||
		!strings.Contains(rendered, "> second message") ||
		!strings.Contains(rendered, "Message 2 of 2") {
		t.Fatalf("rendered selector missing Pi-style content:\n%s", rendered)
	}

	var selected string
	selector.OnSelect = func(entryID string) { selected = entryID }
	selector.HandleInput("\r")
	if selected != "second" {
		t.Fatalf("selected = %q, want most recent message", selected)
	}
}

func TestUserMessageSelectorComponentNavigatesAndCancelsPiStyle(t *testing.T) {
	selector := NewUserMessageSelectorComponent([]AgentSessionForkMessage{
		{EntryID: "first", Text: "first message"},
		{EntryID: "second", Text: "second message"},
	}, "")
	var selected string
	var cancelled bool
	selector.OnSelect = func(entryID string) { selected = entryID }
	selector.OnCancel = func() { cancelled = true }

	selector.HandleInput("\x1b[A")
	selector.HandleInput("\r")
	if selected != "first" {
		t.Fatalf("selected after up = %q, want first", selected)
	}
	selector.HandleInput("\x1b")
	if !cancelled {
		t.Fatal("escape should cancel selector")
	}
}
