package gicodingagent

import (
	"errors"
	"regexp"
	"sort"
	"strings"
)

type SessionSelectorSortMode string

const (
	SessionSelectorSortThreaded  SessionSelectorSortMode = "threaded"
	SessionSelectorSortRecent    SessionSelectorSortMode = "recent"
	SessionSelectorSortRelevance SessionSelectorSortMode = "relevance"
)

type SessionSelectorNameFilter string

const (
	SessionSelectorNameAll   SessionSelectorNameFilter = "all"
	SessionSelectorNameNamed SessionSelectorNameFilter = "named"
)

type sessionSelectorSearchToken struct {
	kind  string
	value string
}

type parsedSessionSelectorSearchQuery struct {
	mode   string
	tokens []sessionSelectorSearchToken
	regex  *regexp.Regexp
	err    error
}

type sessionSelectorMatchResult struct {
	matches bool
	score   float64
}

func FilterAndSortSessions(sessions []SessionInfo, query string, sortMode SessionSelectorSortMode, filters ...SessionSelectorNameFilter) []SessionInfo {
	nameFilter := SessionSelectorNameAll
	if len(filters) > 0 && filters[0] != "" {
		nameFilter = filters[0]
	}
	filteredByName := make([]SessionInfo, 0, len(sessions))
	for _, session := range sessions {
		if sessionSelectorMatchesNameFilter(session, nameFilter) {
			filteredByName = append(filteredByName, session)
		}
	}

	if strings.TrimSpace(query) == "" {
		return filteredByName
	}

	parsed := parseSessionSelectorSearchQuery(query)
	if parsed.err != nil {
		return nil
	}

	if sortMode == SessionSelectorSortRecent {
		filtered := make([]SessionInfo, 0, len(filteredByName))
		for _, session := range filteredByName {
			if matchSessionSelectorSearch(session, parsed).matches {
				filtered = append(filtered, session)
			}
		}
		return filtered
	}

	scored := make([]struct {
		session SessionInfo
		score   float64
	}, 0, len(filteredByName))
	for _, session := range filteredByName {
		result := matchSessionSelectorSearch(session, parsed)
		if result.matches {
			scored = append(scored, struct {
				session SessionInfo
				score   float64
			}{session: session, score: result.score})
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score < scored[j].score
		}
		return scored[i].session.Modified.After(scored[j].session.Modified)
	})

	result := make([]SessionInfo, 0, len(scored))
	for _, item := range scored {
		result = append(result, item.session)
	}
	return result
}

func HasSessionName(session SessionInfo) bool {
	return strings.TrimSpace(session.Name) != ""
}

func sessionSelectorMatchesNameFilter(session SessionInfo, filter SessionSelectorNameFilter) bool {
	if filter == SessionSelectorNameNamed {
		return HasSessionName(session)
	}
	return true
}

func parseSessionSelectorSearchQuery(query string) parsedSessionSelectorSearchQuery {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return parsedSessionSelectorSearchQuery{mode: "tokens"}
	}
	if strings.HasPrefix(trimmed, "re:") {
		pattern := strings.TrimSpace(strings.TrimPrefix(trimmed, "re:"))
		if pattern == "" {
			return parsedSessionSelectorSearchQuery{mode: "regex", err: errors.New("empty regex")}
		}
		compiled, err := regexp.Compile("(?i:" + pattern + ")")
		if err != nil {
			return parsedSessionSelectorSearchQuery{mode: "regex", err: err}
		}
		return parsedSessionSelectorSearchQuery{mode: "regex", regex: compiled}
	}

	tokens := []sessionSelectorSearchToken{}
	buffer := strings.Builder{}
	inQuote := false
	hadUnclosedQuote := false
	flush := func(kind string) {
		value := strings.TrimSpace(buffer.String())
		buffer.Reset()
		if value != "" {
			tokens = append(tokens, sessionSelectorSearchToken{kind: kind, value: value})
		}
	}

	for _, ch := range trimmed {
		if ch == '"' {
			if inQuote {
				flush("phrase")
				inQuote = false
			} else {
				flush("fuzzy")
				inQuote = true
			}
			continue
		}
		if !inQuote && (ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r') {
			flush("fuzzy")
			continue
		}
		buffer.WriteRune(ch)
	}
	if inQuote {
		hadUnclosedQuote = true
	}
	if hadUnclosedQuote {
		tokens = tokens[:0]
		for _, token := range strings.Fields(trimmed) {
			tokens = append(tokens, sessionSelectorSearchToken{kind: "fuzzy", value: token})
		}
		return parsedSessionSelectorSearchQuery{mode: "tokens", tokens: tokens}
	}
	flush("fuzzy")
	return parsedSessionSelectorSearchQuery{mode: "tokens", tokens: tokens}
}

func matchSessionSelectorSearch(session SessionInfo, parsed parsedSessionSelectorSearchQuery) sessionSelectorMatchResult {
	text := getSessionSelectorSearchText(session)
	if parsed.mode == "regex" {
		if parsed.regex == nil {
			return sessionSelectorMatchResult{}
		}
		match := parsed.regex.FindStringIndex(text)
		if match == nil {
			return sessionSelectorMatchResult{}
		}
		return sessionSelectorMatchResult{matches: true, score: float64(match[0]) * 0.1}
	}
	if len(parsed.tokens) == 0 {
		return sessionSelectorMatchResult{matches: true}
	}

	totalScore := 0.0
	normalizedText := ""
	for _, token := range parsed.tokens {
		if token.kind == "phrase" {
			if normalizedText == "" {
				normalizedText = normalizeSessionSelectorWhitespaceLower(text)
			}
			phrase := normalizeSessionSelectorWhitespaceLower(token.value)
			if phrase == "" {
				continue
			}
			index := strings.Index(normalizedText, phrase)
			if index < 0 {
				return sessionSelectorMatchResult{}
			}
			totalScore += float64(index) * 0.1
			continue
		}
		matches, score := sessionSelectorFuzzyMatchScore(token.value, text)
		if !matches {
			return sessionSelectorMatchResult{}
		}
		totalScore += score
	}
	return sessionSelectorMatchResult{matches: true, score: totalScore}
}

func getSessionSelectorSearchText(session SessionInfo) string {
	return strings.Join([]string{
		session.ID,
		session.Name,
		session.AllMessagesText,
		session.CWD,
	}, " ")
}

func normalizeSessionSelectorWhitespaceLower(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(text)), " ")
}

func sessionSelectorFuzzyMatchScore(pattern, text string) (bool, float64) {
	patternRunes := []rune(strings.ToLower(pattern))
	if len(patternRunes) == 0 {
		return true, 0
	}
	textRunes := []rune(strings.ToLower(text))
	searchFrom := 0
	score := 0.0
	for _, target := range patternRunes {
		found := -1
		for index := searchFrom; index < len(textRunes); index++ {
			if textRunes[index] == target {
				found = index
				break
			}
		}
		if found < 0 {
			return false, 0
		}
		score += float64(found)
		searchFrom = found + 1
	}
	return true, score
}
