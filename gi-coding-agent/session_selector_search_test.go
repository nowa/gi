package gicodingagent

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSessionSelectorSearchFiltersQuotedPhraseWithWhitespaceNormalization(t *testing.T) {
	sessions := []SessionInfo{
		makeSessionSelectorSearchSession("a", "2026-01-01T00:00:00Z", "node\n\n   cve was discussed", ""),
		makeSessionSelectorSearchSession("b", "2026-01-02T00:00:00Z", "node something else", ""),
	}

	result := FilterAndSortSessions(sessions, `"node cve"`, SessionSelectorSortRecent)

	if got, want := sessionSelectorIDs(result), []string{"a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %#v, want %#v", got, want)
	}
}

func TestSessionSelectorSearchFiltersCaseInsensitiveRegex(t *testing.T) {
	sessions := []SessionInfo{
		makeSessionSelectorSearchSession("a", "2026-01-02T00:00:00Z", "Brave is great", ""),
		makeSessionSelectorSearchSession("b", "2026-01-03T00:00:00Z", "bravery is not the same", ""),
	}

	result := FilterAndSortSessions(sessions, `re:\bbrave\b`, SessionSelectorSortRecent)

	if got, want := sessionSelectorIDs(result), []string{"a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %#v, want %#v", got, want)
	}
}

func TestSessionSelectorSearchRecentSortPreservesInputOrder(t *testing.T) {
	sessions := []SessionInfo{
		makeSessionSelectorSearchSession("newer", "2026-01-03T00:00:00Z", "brave", ""),
		makeSessionSelectorSearchSession("older", "2026-01-01T00:00:00Z", "brave", ""),
		makeSessionSelectorSearchSession("nomatch", "2026-01-04T00:00:00Z", "something else", ""),
	}

	result := FilterAndSortSessions(sessions, `"brave"`, SessionSelectorSortRecent)

	if got, want := sessionSelectorIDs(result), []string{"newer", "older"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %#v, want %#v", got, want)
	}
}

func TestSessionSelectorSearchRelevanceSortsByScoreThenModified(t *testing.T) {
	sessions := []SessionInfo{
		makeSessionSelectorSearchSession("late", "2026-01-03T00:00:00Z", "xxxx brave", ""),
		makeSessionSelectorSearchSession("early", "2026-01-01T00:00:00Z", "brave xxxx", ""),
	}

	result := FilterAndSortSessions(sessions, `"brave"`, SessionSelectorSortRelevance)
	if got, want := sessionSelectorIDs(result), []string{"early", "late"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %#v, want %#v", got, want)
	}

	tieSessions := []SessionInfo{
		makeSessionSelectorSearchSession("newer", "2026-01-03T00:00:00Z", "brave", ""),
		makeSessionSelectorSearchSession("older", "2026-01-01T00:00:00Z", "brave", ""),
	}

	result = FilterAndSortSessions(tieSessions, `"brave"`, SessionSelectorSortRelevance)
	if got, want := sessionSelectorIDs(result), []string{"newer", "older"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tie ids = %#v, want %#v", got, want)
	}
}

func TestSessionSelectorSearchInvalidRegexReturnsEmpty(t *testing.T) {
	sessions := []SessionInfo{
		makeSessionSelectorSearchSession("a", "2026-01-01T00:00:00Z", "brave", ""),
	}

	result := FilterAndSortSessions(sessions, "re:(", SessionSelectorSortRecent)

	if len(result) != 0 {
		t.Fatalf("result = %#v, want empty", result)
	}
}

func TestSessionSelectorSearchNameFilterAllReturnsAllSessions(t *testing.T) {
	sessions := makeSessionSelectorNamedFilterSessions()

	result := FilterAndSortSessions(sessions, "", SessionSelectorSortRecent, SessionSelectorNameAll)

	if got, want := sessionSelectorIDs(result), []string{"named1", "named2", "other1", "other2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %#v, want %#v", got, want)
	}
}

func TestSessionSelectorSearchNameFilterNamedReturnsOnlyNamedSessions(t *testing.T) {
	sessions := makeSessionSelectorNamedFilterSessions()

	result := FilterAndSortSessions(sessions, "", SessionSelectorSortRecent, SessionSelectorNameNamed)

	if got, want := sessionSelectorIDs(result), []string{"named1", "named2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %#v, want %#v", got, want)
	}
}

func TestSessionSelectorSearchNameFilterAppliesBeforeQuery(t *testing.T) {
	sessions := makeSessionSelectorNamedFilterSessions()

	result := FilterAndSortSessions(sessions, "blueberry", SessionSelectorSortRecent, SessionSelectorNameNamed)

	if got, want := sessionSelectorIDs(result), []string{"named1", "named2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %#v, want %#v", got, want)
	}
}

func TestSessionSelectorSearchNameFilterExcludesWhitespaceOnlyNames(t *testing.T) {
	sessions := []SessionInfo{
		makeSessionSelectorSearchSession("whitespace", "2026-01-01T00:00:00Z", "test", "   "),
		makeSessionSelectorSearchSession("empty", "2026-01-02T00:00:00Z", "test", ""),
		makeSessionSelectorSearchSession("named", "2026-01-03T00:00:00Z", "test", "Real Name"),
	}

	result := FilterAndSortSessions(sessions, "", SessionSelectorSortRecent, SessionSelectorNameNamed)

	if got, want := sessionSelectorIDs(result), []string{"named"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %#v, want %#v", got, want)
	}
}

func TestSessionSelectorCtrlNTogglesNamedFilterPiStyle(t *testing.T) {
	selector := NewSessionSelectorComponent(makeSessionSelectorNamedFilterSessions(), SessionSelectorOptions{})
	rendered := StripAnsi(strings.Join(selector.Render(120), "\n"))
	if !strings.Contains(rendered, "My Project") || !strings.Contains(rendered, "(no messages)") || !strings.Contains(rendered, "ctrl+n named") {
		t.Fatalf("initial render missing all sessions/hint:\n%s", rendered)
	}

	selector.HandleInput("\x0e")
	rendered = StripAnsi(strings.Join(selector.Render(120), "\n"))
	if !strings.Contains(rendered, "My Project") || strings.Contains(rendered, "(no messages)") || !strings.Contains(rendered, "Name: ") || !strings.Contains(rendered, "Named") || !strings.Contains(rendered, "ctrl+n named") {
		t.Fatalf("named-filter render mismatch:\n%s", rendered)
	}

	selector.HandleInput("\x0e")
	rendered = StripAnsi(strings.Join(selector.Render(120), "\n"))
	if !strings.Contains(rendered, "My Project") || !strings.Contains(rendered, "(no messages)") || !strings.Contains(rendered, "ctrl+n named") {
		t.Fatalf("restored all render mismatch:\n%s", rendered)
	}
}

func TestSessionSelectorUsesEffectiveKeybindingsPiStyle(t *testing.T) {
	keybindings := mergeKeybindingsConfig(DefaultProtocolKeybindings(), KeybindingsConfig{
		"app.session.toggleNamedFilter": "m",
		"app.session.toggleSort":        "s",
		"app.session.togglePath":        "p",
		"app.session.rename":            "r",
		"app.session.delete":            "d",
		"app.session.deleteNoninvasive": "x",
	})
	sessions := makeSessionSelectorNamedFilterSessions()
	var deletePath *string
	selector := NewSessionSelectorComponent(sessions, SessionSelectorOptions{
		ShowRenameHint: true,
		Keybindings:    keybindings,
		OnDeleteConfirmationChange: func(path *string) {
			if path == nil {
				deletePath = nil
				return
			}
			copyPath := *path
			deletePath = &copyPath
		},
	})

	rendered := StripAnsi(strings.Join(selector.Render(160), "\n"))
	for _, expected := range []string{"s sort", "m named", "p path (off)", "r rename"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("render missing %q:\n%s", expected, rendered)
		}
	}

	selector.HandleInput("m")
	rendered = StripAnsi(strings.Join(selector.Render(160), "\n"))
	if !strings.Contains(rendered, "Name: ") || !strings.Contains(rendered, "Named") || !strings.Contains(rendered, "m named") || strings.Contains(rendered, "(no messages)") {
		t.Fatalf("named keybinding render mismatch:\n%s", rendered)
	}

	selector.HandleInput("s")
	rendered = StripAnsi(strings.Join(selector.Render(160), "\n"))
	if !strings.Contains(rendered, "Sort: ") || !strings.Contains(rendered, "Recent") {
		t.Fatalf("sort keybinding render mismatch:\n%s", rendered)
	}

	selector.HandleInput("p")
	rendered = StripAnsi(strings.Join(selector.Render(160), "\n"))
	if !strings.Contains(rendered, "p path (on)") || !strings.Contains(rendered, sessions[0].Path) {
		t.Fatalf("path keybinding render mismatch:\n%s", rendered)
	}

	selector.HandleInput("r")
	rendered = StripAnsi(strings.Join(selector.Render(160), "\n"))
	if !strings.Contains(rendered, "Rename Session") {
		t.Fatalf("rename keybinding render mismatch:\n%s", rendered)
	}
	selector.HandleInput("\x1b")

	selector.HandleInput("d")
	if deletePath == nil || *deletePath != sessions[0].Path {
		t.Fatalf("delete path = %#v, want %q", deletePath, sessions[0].Path)
	}
	selector.HandleInput("\x1b")

	selector.HandleInput("a")
	selector.HandleInput("x")
	rendered = StripAnsi(strings.Join(selector.Render(160), "\n"))
	if !strings.Contains(rendered, "My Project") {
		t.Fatalf("noninvasive delete key should clear search instead of deleting:\n%s", rendered)
	}
}

func makeSessionSelectorNamedFilterSessions() []SessionInfo {
	return []SessionInfo{
		makeSessionSelectorSearchSession("named1", "2026-01-03T00:00:00Z", "blueberry", "My Project"),
		makeSessionSelectorSearchSession("named2", "2026-01-02T00:00:00Z", "blueberry", "Another Named"),
		makeSessionSelectorSearchSession("other1", "2026-01-04T00:00:00Z", "blueberry", ""),
		makeSessionSelectorSearchSession("other2", "2026-01-01T00:00:00Z", "blueberry", ""),
	}
}

func makeSessionSelectorSearchSession(id, modified, allMessagesText, name string) SessionInfo {
	parsedModified, err := time.Parse(time.RFC3339, modified)
	if err != nil {
		panic(err)
	}
	return SessionInfo{
		Path:            "/tmp/" + id + ".jsonl",
		ID:              id,
		Name:            name,
		Created:         time.Unix(0, 0),
		Modified:        parsedModified,
		MessageCount:    1,
		FirstMessage:    "(no messages)",
		AllMessagesText: allMessagesText,
	}
}

func sessionSelectorIDs(sessions []SessionInfo) []string {
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
	}
	return ids
}
