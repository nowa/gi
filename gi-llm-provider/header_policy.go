package gillmprovider

import "strings"

// applyHeaderRemovals applies case-insensitive removals after a transport has
// assembled its default, model, auth, and request headers.
func applyHeaderRemovals(headers map[string]string, removals []string) map[string]string {
	if len(removals) == 0 {
		return headers
	}
	for _, name := range removals {
		removeHeaderCaseInsensitive(headers, name)
	}
	return headers
}

func removeHeaderCaseInsensitive(headers map[string]string, name string) {
	for existing := range headers {
		if strings.EqualFold(existing, name) {
			delete(headers, existing)
		}
	}
}

func setHeaderCaseInsensitive(headers map[string]string, name, value string) {
	removeHeaderCaseInsensitive(headers, name)
	headers[name] = value
}

func appendUniqueHeaderRemovals(base, additional []string) []string {
	result := append([]string(nil), base...)
	for _, name := range additional {
		if strings.TrimSpace(name) == "" || containsHeaderName(result, name) {
			continue
		}
		result = append(result, name)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func clearOverriddenHeaderRemovals(removals []string, headers map[string]string) []string {
	if len(removals) == 0 || len(headers) == 0 {
		return removals
	}
	result := make([]string, 0, len(removals))
	for _, removal := range removals {
		if !hasHeaderCaseInsensitive(headers, removal) {
			result = append(result, removal)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func containsHeaderName(names []string, candidate string) bool {
	for _, name := range names {
		if strings.EqualFold(name, candidate) {
			return true
		}
	}
	return false
}

func hasHeaderCaseInsensitive(headers map[string]string, name string) bool {
	_, ok := headerValueCaseInsensitive(headers, name)
	return ok
}

func headerValueCaseInsensitive(headers map[string]string, name string) (string, bool) {
	for existing := range headers {
		if strings.EqualFold(existing, name) {
			return headers[existing], true
		}
	}
	return "", false
}
