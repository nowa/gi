package gicodingagent

import (
	"fmt"
	"strings"

	gitui "github.com/nowa/gi/gi-tui"
)

func (s *TreeSelectorComponent) Render(width int) []string {
	if s == nil {
		return nil
	}
	width = max(1, width)
	lines := []string{
		"",
		treeSelectorBorder(width),
		gitui.TruncateToWidth(" "+tuiThemeBold("  Session Tree"), width, "", true),
	}
	help := TreeSelectorHelp{keybindings: s.keybindings}
	lines = append(lines, help.Render(width)...)
	searchLine := "  " + tuiThemeMuted("Type to search:")
	if s.state.searchQuery != "" {
		searchLine += " " + tuiThemeAccent(s.state.searchQuery)
	}
	lines = append(lines, gitui.TruncateToWidth(searchLine, width, "", true))
	lines = append(lines, treeSelectorBorder(width), "")
	if s.labelEditor != nil {
		lines = append(lines, s.renderLabelEditor(width)...)
	} else {
		lines = append(lines, s.list.Render(width)...)
	}
	lines = append(lines, "", treeSelectorBorder(width))
	return lines
}

type treeSelectorHelpItem struct {
	actions    []string
	label      string
	labelFirst bool
}

// TreeSelectorHelp renders keybinding-aware semantic help rows.
type TreeSelectorHelp struct {
	keybindings KeybindingsConfig
}

func (h *TreeSelectorHelp) Invalidate() {}

func (h *TreeSelectorHelp) Render(width int) []string {
	if width <= 0 {
		width = 1
	}
	keybindings := h.keybindings
	if keybindings == nil {
		keybindings = DefaultProtocolKeybindings()
	}
	items := []treeSelectorHelpItem{
		{actions: []string{"tui.select.up", "tui.select.down"}, label: "move"},
		{actions: []string{"tui.editor.cursorLeft", "tui.editor.cursorRight"}, label: "page"},
		{actions: []string{"app.tree.foldOrUp", "app.tree.unfoldOrDown"}, label: "branch"},
		{actions: []string{"app.message.copy"}, label: "copy"},
		{actions: []string{"app.tree.editLabel"}, label: "label"},
		{actions: []string{"app.tree.toggleLabelTimestamp"}, label: "label time"},
		{
			actions: []string{
				"app.tree.filter.default",
				"app.tree.filter.noTools",
				"app.tree.filter.userOnly",
				"app.tree.filter.labeledOnly",
				"app.tree.filter.all",
			},
			label:      "filters",
			labelFirst: true,
		},
		{actions: []string{"app.tree.filter.cycleForward", "app.tree.filter.cycleBackward"}, label: "cycle", labelFirst: true},
	}
	const indent = "  "
	const separator = " · "
	var lines []string
	current := ""
	for _, item := range items {
		keys := formatTreeSelectorHelpKeys(keybindings, item.actions)
		text := item.label
		if keys != "" {
			if item.labelFirst {
				text += " " + keys
			} else {
				text = keys + " " + text
			}
		}
		candidate := text
		if current != "" {
			candidate = current + separator + text
		} else if gitui.VisibleWidth(indent+text) <= width {
			candidate = indent + text
		}
		if current == "" || gitui.VisibleWidth(candidate) <= width {
			current = candidate
			continue
		}
		for _, line := range gitui.WrapTextWithANSI(strings.TrimRight(current, " "), width) {
			lines = append(lines, tuiThemeMuted(line))
		}
		current = text
		if gitui.VisibleWidth(indent+text) <= width {
			current = indent + text
		}
	}
	if current != "" {
		for _, line := range gitui.WrapTextWithANSI(strings.TrimRight(current, " "), width) {
			lines = append(lines, tuiThemeMuted(line))
		}
	}
	return lines
}

func formatTreeSelectorHelpKeys(keybindings KeybindingsConfig, actions []string) string {
	keys := make([]string, 0, len(actions))
	for _, action := range actions {
		actionKeys := keybindingValueKeys(keybindings[action])
		if len(actionKeys) == 0 {
			actionKeys = gitui.GetKeybindings().GetKeys(action)
		}
		if len(actionKeys) > 0 {
			keys = append(keys, actionKeys[0])
		}
	}
	if len(keys) == 0 {
		return ""
	}
	return normalizeTreeSelectorHelpKeyText(formatHotkeyText(compactTreeSelectorRawKeys(keys), false))
}

func normalizeTreeSelectorHelpKeyText(text string) string {
	var result strings.Builder
	start := 0
	flush := func(end int) {
		part := text[start:end]
		switch strings.ToLower(part) {
		case "pageup":
			part = "pgup"
		case "pagedown":
			part = "pgdn"
		case "up":
			part = "↑"
		case "down":
			part = "↓"
		case "left":
			part = "←"
		case "right":
			part = "→"
		}
		result.WriteString(part)
	}
	for index, char := range text {
		if char != '+' && char != '/' {
			continue
		}
		flush(index)
		result.WriteRune(char)
		start = index + 1
	}
	flush(len(text))
	return result.String()
}

func compactTreeSelectorRawKeys(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	if len(keys) == 1 {
		return keys[0]
	}
	type keyParts struct {
		prefix string
		suffix string
	}
	parts := make([]keyParts, 0, len(keys))
	for _, key := range keys {
		index := strings.LastIndex(key, "+")
		if index < 0 {
			parts = append(parts, keyParts{suffix: key})
			continue
		}
		parts = append(parts, keyParts{prefix: key[:index+1], suffix: key[index+1:]})
	}
	prefix := parts[0].prefix
	if prefix != "" {
		suffixes := make([]string, 0, len(parts))
		for _, part := range parts {
			if part.prefix != prefix {
				return strings.Join(keys, "/")
			}
			suffixes = append(suffixes, part.suffix)
		}
		return prefix + strings.Join(suffixes, "/")
	}
	return strings.Join(keys, "/")
}

func (s *TreeSelectorComponent) renderLabelEditor(width int) []string {
	if s == nil || s.labelEditor == nil {
		return nil
	}
	const indent = "  "
	available := max(1, width-gitui.VisibleWidth(indent))
	lines := []string{gitui.TruncateToWidth(indent+tuiThemeMuted("Label (empty to remove):"), width, "", true)}
	for _, line := range s.labelEditor.input.Render(available) {
		lines = append(lines, gitui.TruncateToWidth(indent+line, width, "", true))
	}
	confirm := firstNonEmptyString(formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.select.confirm"), false), "enter")
	cancel := firstNonEmptyString(formatHotkeyKeys(gitui.GetKeybindings().GetKeys("tui.select.cancel"), false), "escape")
	lines = append(lines, gitui.TruncateToWidth(
		indent+tuiThemeKeyHint(confirm, "save")+"  "+tuiThemeKeyHint(cancel, "cancel"),
		width,
		"",
		true,
	))
	return lines
}

func (l *TreeSelectorList) Render(width int) []string {
	if l == nil || l.selector == nil {
		return nil
	}
	width = max(1, width)
	if len(l.flat) == 0 {
		return []string{
			gitui.TruncateToWidth(tuiThemeMuted("  No entries found"), width, "", true),
			gitui.TruncateToWidth(tuiThemeMuted("  (0/0)"+l.statusLabels()), width, "", true),
		}
	}
	start := max(0, min(l.selected-l.selector.maxVisibleLines/2, len(l.flat)-l.selector.maxVisibleLines))
	end := min(start+l.selector.maxVisibleLines, len(l.flat))
	rows := make([]treeSelectorViewportRow, 0, end-start)
	for index := start; index < end; index++ {
		flat := l.flat[index]
		node := flat.node
		selected := index == l.selected
		cursor := "  "
		if selected {
			cursor = tuiThemeAccent("› ")
		}
		displayIndent := flat.indent
		if l.selector.multipleRoots {
			displayIndent = max(0, displayIndent-1)
		}
		connector := flat.showConnector && !flat.isVirtualRootChild
		connectorPosition := -1
		if connector {
			connectorPosition = displayIndent - 1
		}
		var prefix strings.Builder
		for column := 0; column < displayIndent*3; column++ {
			level := column / 3
			position := column % 3
			gutter, hasGutter := treeSelectorGutterAt(flat.gutters, level)
			switch {
			case hasGutter:
				if position == 0 && gutter.show {
					prefix.WriteString("│")
				} else {
					prefix.WriteString(" ")
				}
			case connector && level == connectorPosition:
				switch position {
				case 0:
					if flat.isLast {
						prefix.WriteString("└")
					} else {
						prefix.WriteString("├")
					}
				case 1:
					switch {
					case l.selector.state.folded[node.Entry.ID]:
						prefix.WriteString("⊞")
					case l.selector.isFoldable(node.Entry.ID):
						prefix.WriteString("⊟")
					default:
						prefix.WriteString("─")
					}
				default:
					prefix.WriteString(" ")
				}
			default:
				prefix.WriteString(" ")
			}
		}
		foldMarker := ""
		if l.selector.state.folded[node.Entry.ID] && !connector {
			foldMarker = tuiThemeAccent("⊞ ")
		}
		pathMarker := ""
		if l.selector.activePath[node.Entry.ID] {
			pathMarker = tuiThemeAccent("• ")
		}
		label := ""
		if node.Label != "" {
			label = tuiThemeWarning("[" + node.Label + "] ")
		}
		labelTimestamp := ""
		if l.selector.state.showLabelTimestamps && node.Label != "" && node.LabelTimestamp != "" {
			labelTimestamp = tuiThemeMuted(formatTreeSelectorLabelTimestamp(node.LabelTimestamp) + " ")
		}
		content := treeSelectorNodeText(node)
		if selected {
			content = tuiThemeBold(content)
		}
		prefixPart := tuiThemeDim(prefix.String()) + foldMarker + pathMarker
		body := prefixPart + label + labelTimestamp + content
		if selected {
			cursor = tuiThemeBG("selectedBg", cursor)
			body = tuiThemeBG("selectedBg", body)
		}
		rows = append(rows, treeSelectorViewportRow{
			gutter:     cursor,
			body:       body,
			anchorCol:  gitui.VisibleWidth(prefixPart),
			bodyWidth:  gitui.VisibleWidth(body),
			isSelected: selected,
		})
	}
	lines := renderTreeSelectorHorizontalViewport(rows, width)
	lines = append(lines, gitui.TruncateToWidth(
		tuiThemeMuted(fmt.Sprintf("  (%d/%d)%s", l.selected+1, len(l.flat), l.statusLabels())),
		width,
		"",
		true,
	))
	return lines
}

func (l *TreeSelectorList) statusLabels() string {
	if l == nil || l.selector == nil {
		return ""
	}
	var labels string
	switch l.selector.state.filter {
	case TreeSelectorNoToolsFilter:
		labels = " [no-tools]"
	case TreeSelectorUserFilter:
		labels = " [user]"
	case TreeSelectorLabelFilter:
		labels = " [labeled]"
	case TreeSelectorAllFilter:
		labels = " [all]"
	}
	if l.selector.state.showLabelTimestamps {
		labels += " [+label time]"
	}
	return labels
}

func treeSelectorGutterAt(gutters []treeSelectorGutter, position int) (treeSelectorGutter, bool) {
	for _, gutter := range gutters {
		if gutter.position == position {
			return gutter, true
		}
	}
	return treeSelectorGutter{}, false
}

func renderTreeSelectorHorizontalViewport(rows []treeSelectorViewportRow, width int) []string {
	const gutterWidth = 2
	const minAnchorContentWidth = 4
	const maxAnchorContentWidth = 20
	const minAnchorContextWidth = 2
	const maxAnchorContextWidth = 12

	viewportWidth := max(0, width-gutterWidth)
	maxBodyWidth := 0
	var selected *treeSelectorViewportRow
	for index := range rows {
		maxBodyWidth = max(maxBodyWidth, rows[index].bodyWidth)
		if rows[index].isSelected {
			selected = &rows[index]
		}
	}
	maxScroll := max(0, maxBodyWidth-viewportWidth)
	scroll := 0
	if selected != nil && maxScroll > 0 {
		minVisible := min(maxAnchorContentWidth, max(minAnchorContentWidth, viewportWidth/3))
		if selected.anchorCol > viewportWidth-minVisible {
			contextWidth := min(maxAnchorContextWidth, max(minAnchorContextWidth, viewportWidth/4))
			scroll = min(maxScroll, selected.anchorCol-contextWidth)
		}
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		line := row.gutter + row.body
		if scroll > 0 {
			line = row.gutter + gitui.SliceByColumn(row.body, scroll, viewportWidth, true) + "\x1b[0m"
		}
		lines = append(lines, gitui.TruncateToWidth(line, width, ""))
	}
	return lines
}
