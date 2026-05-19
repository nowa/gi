package gitui

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type AutocompleteItem struct {
	Value       string
	Label       string
	Description string
}

type AutocompleteSuggestions struct {
	Items  []AutocompleteItem
	Start  int
	End    int
	Prefix string
}

type AutocompleteProvider interface {
	Suggestions(text string, cursor int) AutocompleteSuggestions
}

type AutocompleteProviderFunc func(text string, cursor int) AutocompleteSuggestions

func (f AutocompleteProviderFunc) Suggestions(text string, cursor int) AutocompleteSuggestions {
	return f(text, cursor)
}

type SlashCommand struct {
	Name                          string
	Description                   string
	ArgumentHint                  string
	GetArgumentCompletions        func(argumentPrefix string) []AutocompleteItem
	GetArgumentCompletionsContext func(ctx context.Context, argumentPrefix string) ([]AutocompleteItem, error)
}

type CombinedAutocompleteProvider struct {
	providers []AutocompleteProvider
	commands  []SlashCommand
	basePath  string
	maxFiles  int
}

func NewCombinedAutocompleteProvider(providers ...AutocompleteProvider) *CombinedAutocompleteProvider {
	base, _ := os.Getwd()
	return &CombinedAutocompleteProvider{providers: providers, basePath: base, maxFiles: 200}
}

func NewCombinedAutocompleteProviderWithCommands(basePath string, commands []SlashCommand, providers ...AutocompleteProvider) *CombinedAutocompleteProvider {
	if basePath == "" {
		basePath, _ = os.Getwd()
	}
	return &CombinedAutocompleteProvider{providers: providers, commands: commands, basePath: basePath, maxFiles: 200}
}

func NewCombinedAutocompleteProviderWithCommandItems(basePath string, commands []AutocompleteItem, providers ...AutocompleteProvider) *CombinedAutocompleteProvider {
	if basePath == "" {
		basePath, _ = os.Getwd()
	}
	return &CombinedAutocompleteProvider{providers: providers, commands: slashCommandsFromItems(commands), basePath: basePath, maxFiles: 200}
}

func (p *CombinedAutocompleteProvider) SetBasePath(basePath string) {
	if basePath != "" {
		p.basePath = basePath
	}
}

func (p *CombinedAutocompleteProvider) SetCommands(commands []SlashCommand) {
	p.commands = append([]SlashCommand(nil), commands...)
}

func (p *CombinedAutocompleteProvider) SetCommandItems(commands []AutocompleteItem) {
	p.commands = slashCommandsFromItems(commands)
}

func (p *CombinedAutocompleteProvider) Add(provider AutocompleteProvider) {
	if provider != nil {
		p.providers = append(p.providers, provider)
	}
}

func slashCommandsFromItems(items []AutocompleteItem) []SlashCommand {
	commands := make([]SlashCommand, 0, len(items))
	for _, item := range items {
		name := item.Value
		if name == "" {
			name = item.Label
		}
		if name == "" {
			continue
		}
		commands = append(commands, SlashCommand{Name: name, Description: item.Description})
	}
	return commands
}

func (p *CombinedAutocompleteProvider) Suggestions(text string, cursor int) AutocompleteSuggestions {
	for _, provider := range p.providers {
		suggestions := provider.Suggestions(text, cursor)
		if len(suggestions.Items) > 0 {
			return suggestions
		}
	}
	return AutocompleteSuggestions{Start: cursor, End: cursor}
}

func (p *CombinedAutocompleteProvider) GetSuggestions(lines []string, cursorLine, cursorCol int, force bool) (*AutocompleteSuggestions, error) {
	return p.GetSuggestionsContext(context.Background(), lines, cursorLine, cursorCol, force)
}

func (p *CombinedAutocompleteProvider) GetSuggestionsContext(ctx context.Context, lines []string, cursorLine, cursorCol int, force bool) (*AutocompleteSuggestions, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fallbackProviderSuggestions := func() (*AutocompleteSuggestions, error) {
		if suggestions := p.providerSuggestions(lines, cursorLine, cursorCol); suggestions != nil {
			return suggestions, nil
		}
		return nil, nil
	}
	currentLine := ""
	if cursorLine >= 0 && cursorLine < len(lines) {
		currentLine = lines[cursorLine]
	}
	if cursorCol < 0 {
		cursorCol = 0
	}
	if lineRunes := utf8.RuneCountInString(currentLine); cursorCol > lineRunes {
		cursorCol = lineRunes
	}
	cursorByte := runeColToByteIndex(currentLine, cursorCol)
	before := currentLine[:cursorByte]
	if atPrefix := extractAtPrefix(before); atPrefix != "" {
		raw, isAt, quoted := parsePathPrefix(atPrefix)
		items, err := p.fuzzyFileSuggestions(raw, isAt, quoted)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return fallbackProviderSuggestions()
		}
		return &AutocompleteSuggestions{Items: items, Prefix: atPrefix, Start: cursorCol - runeLen(atPrefix), End: cursorCol}, nil
	}
	if !force && strings.HasPrefix(before, "/") {
		space := strings.Index(before, " ")
		if space == -1 {
			prefix := before[1:]
			items := make([]AutocompleteItem, 0, len(p.commands))
			for _, cmd := range p.commands {
				desc := cmd.Description
				if cmd.ArgumentHint != "" {
					if desc != "" {
						desc = cmd.ArgumentHint + " — " + desc
					} else {
						desc = cmd.ArgumentHint
					}
				}
				items = append(items, AutocompleteItem{Value: cmd.Name, Label: cmd.Name, Description: desc})
			}
			items = FuzzyFilter(items, prefix, func(item AutocompleteItem) string { return item.Value })
			if len(items) == 0 {
				return fallbackProviderSuggestions()
			}
			return &AutocompleteSuggestions{Items: items, Prefix: before, Start: 0, End: cursorCol}, nil
		}
		commandName := before[1:space]
		arg := before[space+1:]
		for _, cmd := range p.commands {
			if cmd.Name == commandName {
				var items []AutocompleteItem
				var err error
				if cmd.GetArgumentCompletionsContext != nil {
					items, err = cmd.GetArgumentCompletionsContext(ctx, arg)
				} else if cmd.GetArgumentCompletions != nil {
					items = cmd.GetArgumentCompletions(arg)
				}
				if err != nil {
					return nil, err
				}
				if len(items) == 0 {
					return fallbackProviderSuggestions()
				}
				return &AutocompleteSuggestions{Items: items, Prefix: arg, Start: cursorCol - runeLen(arg), End: cursorCol}, nil
			}
		}
		return fallbackProviderSuggestions()
	}
	pathPrefix := extractPathPrefix(before, force)
	if pathPrefix == "" && !force {
		return fallbackProviderSuggestions()
	}
	items, err := p.fileSuggestions(pathPrefix)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return fallbackProviderSuggestions()
	}
	return &AutocompleteSuggestions{Items: items, Prefix: pathPrefix, Start: cursorCol - runeLen(pathPrefix), End: cursorCol}, nil
}

func (p *CombinedAutocompleteProvider) providerSuggestions(lines []string, cursorLine, cursorCol int) *AutocompleteSuggestions {
	if len(p.providers) == 0 {
		return nil
	}
	text := strings.Join(lines, "\n")
	cursor := cursorFromLineCol(lines, cursorLine, cursorCol)
	for _, provider := range p.providers {
		suggestions := provider.Suggestions(text, cursor)
		if len(suggestions.Items) > 0 {
			return &suggestions
		}
	}
	return nil
}

type CompletionResult struct {
	Lines      []string
	CursorLine int
	CursorCol  int
}

func (p *CombinedAutocompleteProvider) ApplyCompletion(lines []string, cursorLine, cursorCol int, item AutocompleteItem, prefix string) CompletionResult {
	newLines := append([]string(nil), lines...)
	if cursorLine < 0 || cursorLine >= len(newLines) {
		return CompletionResult{Lines: newLines, CursorLine: cursorLine, CursorCol: cursorCol}
	}
	line := newLines[cursorLine]
	cursorCol = max(0, min(cursorCol, runeLen(line)))
	cursorByte := runeColToByteIndex(line, cursorCol)
	if !strings.HasPrefix(prefix, "/") {
		if argStartCol, currentArg, cmd, ok := p.slashArgumentCompletion(line, cursorCol); ok {
			if currentArg != "" && cmd.GetArgumentCompletions != nil {
				for _, candidate := range cmd.GetArgumentCompletions(currentArg) {
					if candidate.Value == currentArg {
						item = candidate
						break
					}
				}
			}
			argStartByte := runeColToByteIndex(line, argStartCol)
			newLine := line[:argStartByte] + item.Value + line[cursorByte:]
			newLines[cursorLine] = newLine
			return CompletionResult{Lines: newLines, CursorLine: cursorLine, CursorCol: argStartCol + runeLen(item.Value)}
		}
	}
	prefixStartCol := max(0, cursorCol-runeLen(prefix))
	prefixStartByte := runeColToByteIndex(line, prefixStartCol)
	beforePrefix := line[:prefixStartByte]
	afterCursor := line[cursorByte:]
	quotedPrefix := strings.HasPrefix(prefix, "\"") || strings.HasPrefix(prefix, "@\"")
	if quotedPrefix && strings.HasSuffix(item.Value, "\"") && strings.HasPrefix(afterCursor, "\"") {
		afterCursor = afterCursor[1:]
	}
	isSlashCommand := strings.HasPrefix(prefix, "/") && strings.TrimSpace(beforePrefix) == "" && !strings.Contains(prefix[1:], "/")
	if isSlashCommand {
		newLine := beforePrefix + "/" + item.Value + " " + afterCursor
		newLines[cursorLine] = newLine
		return CompletionResult{Lines: newLines, CursorLine: cursorLine, CursorCol: prefixStartCol + runeLen(item.Value) + 2}
	}
	if strings.HasPrefix(prefix, "@") {
		suffix := " "
		if strings.HasSuffix(item.Label, "/") {
			suffix = ""
		}
		newLine := beforePrefix + item.Value + suffix + afterCursor
		newLines[cursorLine] = newLine
		offset := runeLen(item.Value)
		if strings.HasSuffix(item.Label, "/") && strings.HasSuffix(item.Value, "\"") {
			offset--
		}
		return CompletionResult{Lines: newLines, CursorLine: cursorLine, CursorCol: prefixStartCol + offset + runeLen(suffix)}
	}
	newLine := beforePrefix + item.Value + afterCursor
	newLines[cursorLine] = newLine
	offset := runeLen(item.Value)
	if strings.HasSuffix(item.Label, "/") && strings.HasSuffix(item.Value, "\"") {
		offset--
	}
	return CompletionResult{Lines: newLines, CursorLine: cursorLine, CursorCol: prefixStartCol + offset}
}

func (p *CombinedAutocompleteProvider) slashArgumentCompletion(line string, cursorCol int) (int, string, SlashCommand, bool) {
	cursorCol = max(0, min(cursorCol, runeLen(line)))
	cursorByte := runeColToByteIndex(line, cursorCol)
	beforeCursor := line[:cursorByte]
	if !strings.HasPrefix(beforeCursor, "/") {
		return 0, "", SlashCommand{}, false
	}
	space := strings.Index(beforeCursor, " ")
	if space <= 1 {
		return 0, "", SlashCommand{}, false
	}
	commandName := beforeCursor[1:space]
	for _, cmd := range p.commands {
		if cmd.Name == commandName && (cmd.GetArgumentCompletions != nil || cmd.GetArgumentCompletionsContext != nil) {
			return runeLen(beforeCursor[:space+1]), beforeCursor[space+1:], cmd, true
		}
	}
	return 0, "", SlashCommand{}, false
}

func (p *CombinedAutocompleteProvider) ShouldTriggerFileCompletion(lines []string, cursorLine, cursorCol int) bool {
	if cursorLine < 0 || cursorLine >= len(lines) {
		return false
	}
	line := lines[cursorLine]
	cursorCol = max(0, min(cursorCol, runeLen(line)))
	before := line[:runeColToByteIndex(line, cursorCol)]
	trimmed := strings.TrimSpace(before)
	return !(strings.HasPrefix(trimmed, "/") && !strings.Contains(trimmed, " "))
}

func runeLen(text string) int {
	return utf8.RuneCountInString(text)
}

func extractAtPrefix(text string) string {
	if quoted := extractQuotedPrefix(text); strings.HasPrefix(quoted, "@\"") {
		return quoted
	}
	start := findLastDelimiter(text) + 1
	if start >= 0 && start < len(text) && text[start] == '@' {
		return text[start:]
	}
	return ""
}

func extractPathPrefix(text string, force bool) string {
	if quoted := extractQuotedPrefix(text); quoted != "" {
		return quoted
	}
	start := findLastDelimiter(text) + 1
	pathPrefix := text[start:]
	if force {
		return pathPrefix
	}
	if strings.Contains(pathPrefix, "/") || strings.HasPrefix(pathPrefix, ".") || strings.HasPrefix(pathPrefix, "~/") {
		return pathPrefix
	}
	if pathPrefix == "" && strings.HasSuffix(text, " ") {
		return pathPrefix
	}
	return ""
}

func extractQuotedPrefix(text string) string {
	quoteStart := -1
	inQuotes := false
	for i, r := range text {
		if r == '"' {
			inQuotes = !inQuotes
			if inQuotes {
				quoteStart = i
			}
		}
	}
	if !inQuotes || quoteStart < 0 {
		return ""
	}
	if quoteStart > 0 && text[quoteStart-1] == '@' {
		if quoteStart == 1 || isPathDelimiter(text[quoteStart-2]) {
			return text[quoteStart-1:]
		}
		return ""
	}
	if quoteStart == 0 || isPathDelimiter(text[quoteStart-1]) {
		return text[quoteStart:]
	}
	return ""
}

func findLastDelimiter(text string) int {
	for i := len(text) - 1; i >= 0; i-- {
		if isPathDelimiter(text[i]) {
			return i
		}
	}
	return -1
}

func isPathDelimiter(b byte) bool {
	return b == ' ' || b == '\t' || b == '"' || b == '\'' || b == '='
}

func parsePathPrefix(prefix string) (raw string, isAt bool, quoted bool) {
	switch {
	case strings.HasPrefix(prefix, "@\""):
		return prefix[2:], true, true
	case strings.HasPrefix(prefix, "\""):
		return prefix[1:], false, true
	case strings.HasPrefix(prefix, "@"):
		return prefix[1:], true, false
	default:
		return prefix, false, false
	}
}

func buildCompletionValue(path string, isDir, isAt, quoted bool) string {
	if isDir && !strings.HasSuffix(path, "/") {
		path += "/"
	}
	needsQuotes := quoted || strings.Contains(path, " ")
	prefix := ""
	if isAt {
		prefix = "@"
	}
	if !needsQuotes {
		return prefix + path
	}
	return prefix + "\"" + path + "\""
}

func (p *CombinedAutocompleteProvider) fileSuggestions(prefix string) ([]AutocompleteItem, error) {
	rawPrefix, isAt, quoted := parsePathPrefix(prefix)
	searchDir, searchPrefix, displayBase, err := p.resolveDirectSearch(rawPrefix)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return nil, nil
	}
	var items []AutocompleteItem
	for _, entry := range entries {
		if !strings.HasPrefix(strings.ToLower(entry.Name()), strings.ToLower(searchPrefix)) {
			continue
		}
		isDir := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(filepath.Join(searchDir, entry.Name())); err == nil {
				isDir = info.IsDir()
			}
		}
		displayPath := filepath.ToSlash(filepath.Join(displayBase, entry.Name()))
		if displayBase == "" || displayBase == "." {
			displayPath = entry.Name()
		}
		if strings.HasPrefix(rawPrefix, "./") && !strings.HasPrefix(displayPath, "./") {
			displayPath = "./" + displayPath
		}
		value := buildCompletionValue(displayPath, isDir, isAt, quoted)
		label := entry.Name()
		if isDir {
			label += "/"
		}
		items = append(items, AutocompleteItem{Value: value, Label: label})
	}
	sortAutocompleteItems(items)
	return items, nil
}

func (p *CombinedAutocompleteProvider) resolveDirectSearch(rawPrefix string) (searchDir, searchPrefix, displayBase string, err error) {
	expanded := expandHome(rawPrefix)
	rootPrefix := rawPrefix == "" || rawPrefix == "./" || rawPrefix == "../" || rawPrefix == "~" || rawPrefix == "~/" || rawPrefix == "/"
	if rootPrefix || strings.HasSuffix(rawPrefix, "/") {
		searchPrefix = ""
		displayBase = rawPrefix
		if rawPrefix == "" {
			searchDir = p.basePath
		} else if filepath.IsAbs(expanded) {
			searchDir = expanded
		} else {
			searchDir = filepath.Join(p.basePath, expanded)
		}
		return searchDir, searchPrefix, displayBase, nil
	}
	dir := filepath.Dir(expanded)
	file := filepath.Base(expanded)
	if dir == "." {
		dir = ""
	}
	if filepath.IsAbs(expanded) {
		searchDir = filepath.Dir(expanded)
		displayBase = filepath.Dir(rawPrefix)
		if displayBase == "." {
			displayBase = ""
		}
	} else {
		searchDir = filepath.Join(p.basePath, dir)
		displayBase = filepath.ToSlash(filepath.Dir(rawPrefix))
		if displayBase == "." {
			displayBase = ""
		}
	}
	return searchDir, file, displayBase, nil
}

func (p *CombinedAutocompleteProvider) fuzzyFileSuggestions(rawPrefix string, isAt, quoted bool) ([]AutocompleteItem, error) {
	scope := p.resolveFuzzyScope(rawPrefix)
	var candidates []fileCandidate
	err := walkFiles(scope.baseDir, p.maxFiles, &candidates)
	if err != nil {
		return nil, nil
	}
	filtered := FuzzyFilter(candidates, scope.query, func(c fileCandidate) string { return c.displayPath })
	items := make([]AutocompleteItem, 0, len(filtered))
	for _, candidate := range filtered {
		displayPath := scope.displayPath(candidate.relativePath)
		value := buildCompletionValue(displayPath, candidate.isDir, isAt, quoted)
		label := filepath.Base(strings.TrimSuffix(candidate.relativePath, "/"))
		if candidate.isDir {
			label += "/"
		}
		items = append(items, AutocompleteItem{Value: value, Label: label, Description: filepath.ToSlash(filepath.Dir(displayPath))})
	}
	sort.SliceStable(items, func(i, j int) bool {
		iDir := strings.HasSuffix(items[i].Label, "/")
		jDir := strings.HasSuffix(items[j].Label, "/")
		if iDir != jDir {
			return iDir
		}
		return items[i].Value < items[j].Value
	})
	return items, nil
}

type fuzzyScope struct {
	baseDir     string
	query       string
	displayBase string
}

func (s fuzzyScope) displayPath(relative string) string {
	relative = filepath.ToSlash(relative)
	if s.displayBase == "" {
		return relative
	}
	if s.displayBase == "/" {
		return "/" + relative
	}
	return filepath.ToSlash(filepath.Join(s.displayBase, relative))
}

func (p *CombinedAutocompleteProvider) resolveFuzzyScope(rawPrefix string) fuzzyScope {
	normalized := filepath.ToSlash(rawPrefix)
	slash := strings.LastIndex(normalized, "/")
	if slash < 0 {
		return fuzzyScope{baseDir: p.basePath, query: normalized}
	}
	displayBase := normalized[:slash+1]
	query := normalized[slash+1:]
	var baseDir string
	expanded := expandHome(displayBase)
	if filepath.IsAbs(expanded) {
		baseDir = expanded
	} else {
		baseDir = filepath.Join(p.basePath, expanded)
	}
	if info, err := os.Stat(baseDir); err != nil || !info.IsDir() {
		return fuzzyScope{baseDir: p.basePath, query: normalized}
	}
	return fuzzyScope{baseDir: baseDir, query: query, displayBase: displayBase}
}

type fileCandidate struct {
	relativePath string
	displayPath  string
	isDir        bool
}

func walkFiles(baseDir string, maxResults int, out *[]fileCandidate) error {
	if maxResults <= 0 {
		maxResults = 200
	}
	visited := map[string]bool{}
	var walk func(string, string) error
	walk = func(dir, relBase string) error {
		realDir, _ := filepath.EvalSymlinks(dir)
		if realDir != "" {
			if visited[realDir] {
				return nil
			}
			visited[realDir] = true
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		for _, entry := range entries {
			if len(*out) >= maxResults {
				return nil
			}
			if entry.Name() == ".git" {
				continue
			}
			full := filepath.Join(dir, entry.Name())
			rel := filepath.ToSlash(filepath.Join(relBase, entry.Name()))
			isDir := entry.IsDir()
			if entry.Type()&os.ModeSymlink != 0 {
				if info, err := os.Stat(full); err == nil {
					isDir = info.IsDir()
				}
			}
			candidatePath := rel
			if isDir {
				candidatePath += "/"
			}
			*out = append(*out, fileCandidate{relativePath: candidatePath, displayPath: candidatePath, isDir: isDir})
			if isDir {
				_ = walk(full, rel)
			}
		}
		return nil
	}
	return walk(baseDir, "")
}

func sortAutocompleteItems(items []AutocompleteItem) {
	sort.SliceStable(items, func(i, j int) bool {
		iDir := strings.HasSuffix(items[i].Label, "/")
		jDir := strings.HasSuffix(items[j].Label, "/")
		if iDir != jDir {
			return iDir
		}
		return strings.ToLower(items[i].Label) < strings.ToLower(items[j].Label)
	})
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

type FuzzyMatch struct {
	Matches bool
	Score   float64
}

func FuzzyMatchText(query, text string) FuzzyMatch {
	queryLower := strings.ToLower(query)
	textLower := strings.ToLower(text)
	matchQuery := func(normalizedQuery string) FuzzyMatch {
		if normalizedQuery == "" {
			return FuzzyMatch{Matches: true, Score: 0}
		}
		if len([]rune(normalizedQuery)) > len([]rune(textLower)) {
			return FuzzyMatch{Matches: false}
		}
		queryRunes := []rune(normalizedQuery)
		textRunes := []rune(textLower)
		queryIndex := 0
		score := 0.0
		lastMatchIndex := -1
		consecutiveMatches := 0
		for i, r := range textRunes {
			if queryIndex >= len(queryRunes) {
				break
			}
			if r != queryRunes[queryIndex] {
				continue
			}
			isBoundary := i == 0 || isFuzzyBoundary(textRunes[i-1])
			if lastMatchIndex == i-1 {
				consecutiveMatches++
				score -= float64(consecutiveMatches * 5)
			} else {
				consecutiveMatches = 0
				if lastMatchIndex >= 0 {
					score += float64(i-lastMatchIndex-1) * 2
				}
			}
			if isBoundary {
				score -= 10
			}
			score += float64(i) * 0.1
			lastMatchIndex = i
			queryIndex++
		}
		if queryIndex < len(queryRunes) {
			return FuzzyMatch{Matches: false}
		}
		if normalizedQuery == textLower {
			score -= 100
		}
		return FuzzyMatch{Matches: true, Score: score}
	}
	primary := matchQuery(queryLower)
	if primary.Matches {
		return primary
	}
	if swapped := swappedAlphaNumeric(queryLower); swapped != "" {
		match := matchQuery(swapped)
		if match.Matches {
			match.Score += 5
			return match
		}
	}
	return primary
}

func isFuzzyBoundary(r rune) bool {
	return unicode.IsSpace(r) || r == '-' || r == '_' || r == '.' || r == '/' || r == ':'
}

func swappedAlphaNumeric(query string) string {
	if query == "" {
		return ""
	}
	runes := []rune(query)
	split := 0
	for split < len(runes) && unicode.IsLetter(runes[split]) {
		split++
	}
	if split > 0 && split < len(runes) {
		allDigits := true
		for _, r := range runes[split:] {
			if !unicode.IsDigit(r) {
				allDigits = false
				break
			}
		}
		if allDigits {
			return string(runes[split:]) + string(runes[:split])
		}
	}
	split = 0
	for split < len(runes) && unicode.IsDigit(runes[split]) {
		split++
	}
	if split > 0 && split < len(runes) {
		allLetters := true
		for _, r := range runes[split:] {
			if !unicode.IsLetter(r) {
				allLetters = false
				break
			}
		}
		if allLetters {
			return string(runes[split:]) + string(runes[:split])
		}
	}
	return ""
}

func FuzzyFilter[T any](items []T, query string, getText func(T) string) []T {
	if strings.TrimSpace(query) == "" {
		return items
	}
	tokens := strings.Fields(query)
	type result struct {
		item  T
		score float64
		index int
	}
	var results []result
	for idx, item := range items {
		total := 0.0
		allMatch := true
		for _, token := range tokens {
			match := FuzzyMatchText(token, getText(item))
			if !match.Matches {
				allMatch = false
				break
			}
			total += match.Score
		}
		if allMatch {
			results = append(results, result{item: item, score: total, index: idx})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score == results[j].score {
			return results[i].index < results[j].index
		}
		return results[i].score < results[j].score
	})
	out := make([]T, len(results))
	for i, result := range results {
		out[i] = result.item
	}
	return out
}

func FuzzyFilterStrings(items []string, query string) []string {
	return FuzzyFilter(items, query, func(item string) string { return item })
}
