package gillmprovider

// OpenAIPromptCacheKeyMaxLength is OpenAI's cache-key limit in Unicode
// characters.
const OpenAIPromptCacheKeyMaxLength = 64

// ClampOpenAIPromptCacheKey applies OpenAI's 64-character cache-key limit.
// Runes are used instead of bytes so truncation never splits UTF-8 input.
func ClampOpenAIPromptCacheKey(key string) string {
	characters := []rune(key)
	if len(characters) <= OpenAIPromptCacheKeyMaxLength {
		return key
	}
	return string(characters[:OpenAIPromptCacheKeyMaxLength])
}
