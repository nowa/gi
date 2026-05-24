package gicodingagent

import (
	"strings"
	"testing"
	"time"
)

func TestSessionSelectorPiRenameHint(t *testing.T) {
	sessions := []SessionInfo{makeSessionSelectorSession("a", "Old")}
	interactive := NewSessionSelectorComponent(sessions, SessionSelectorOptions{ShowRenameHint: true})
	output := StripAnsi(strings.Join(interactive.Render(120), "\n"))
	if !strings.Contains(output, "ctrl+r") || !strings.Contains(output, "rename") {
		t.Fatalf("interactive output = %q", output)
	}

	resumeFlag := NewSessionSelectorComponent(sessions, SessionSelectorOptions{ShowRenameHint: false})
	output = StripAnsi(strings.Join(resumeFlag.Render(120), "\n"))
	if strings.Contains(output, "ctrl+r") || strings.Contains(output, "rename") {
		t.Fatalf("resume flag output = %q", output)
	}
}

func TestSessionSelectorPiRenameModeSubmit(t *testing.T) {
	sessions := []SessionInfo{makeSessionSelectorSession("a", "Old")}
	var renamedPath string
	var renamedName string
	selector := NewSessionSelectorComponent(sessions, SessionSelectorOptions{
		ShowRenameHint: true,
		RenameSession: func(path, name string) error {
			renamedPath = path
			renamedName = name
			return nil
		},
	})

	selector.HandleInput(sessionSelectorCtrlR)
	output := strings.Join(selector.Render(120), "\n")
	if !strings.Contains(output, "Rename Session") || strings.Contains(output, "Resume Session") {
		t.Fatalf("rename output = %q", output)
	}

	selector.HandleInput("X")
	selector.HandleInput("\r")
	if renamedPath != sessions[0].Path || renamedName != "XOld" {
		t.Fatalf("renamed path/name = %q %q", renamedPath, renamedName)
	}
	output = strings.Join(selector.Render(120), "\n")
	if !strings.Contains(output, "XOld") {
		t.Fatalf("renamed session should update visible list: %q", output)
	}
}

func makeSessionSelectorSession(id, name string) SessionInfo {
	return SessionInfo{
		Path:            "/tmp/" + id + ".jsonl",
		ID:              id,
		Name:            name,
		Created:         time.Unix(0, 0),
		Modified:        time.Unix(0, 0),
		MessageCount:    1,
		FirstMessage:    "hello",
		AllMessagesText: "hello",
	}
}
