package gicodingagent

import (
	"reflect"
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
