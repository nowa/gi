package gitui

import (
	"fmt"
	"strings"
	"sync"
)

type SelectItem struct {
	Value       string
	Label       string
	Description string
}

type SelectListTheme struct {
	SelectedPrefix func(string) string
	SelectedText   func(string) string
	Description    func(string) string
	ScrollInfo     func(string) string
	NoMatch        func(string) string
}

type SelectListLayoutOptions struct {
	MinPrimaryColumnWidth int
	MaxPrimaryColumnWidth int
	TruncatePrimary       func(SelectListTruncatePrimaryContext) string
}

type SelectListTruncatePrimaryContext struct {
	Text        string
	MaxWidth    int
	ColumnWidth int
	Item        SelectItem
	IsSelected  bool
}

const (
	defaultSelectListPrimaryColumnWidth = 32
	selectListPrimaryColumnGap          = 2
	selectListMinDescriptionWidth       = 10
)

type SelectList struct {
	mu                sync.Mutex
	items             []SelectItem
	filtered          []SelectItem
	selectedIndex     int
	maxVisible        int
	theme             SelectListTheme
	layout            SelectListLayoutOptions
	OnSelect          func(SelectItem)
	OnCancel          func()
	OnSelectionChange func(SelectItem)
}

func NewSelectList(items []SelectItem, maxVisible int, theme SelectListTheme, layout ...SelectListLayoutOptions) *SelectList {
	opts := SelectListLayoutOptions{MinPrimaryColumnWidth: defaultSelectListPrimaryColumnWidth, MaxPrimaryColumnWidth: defaultSelectListPrimaryColumnWidth}
	if len(layout) > 0 {
		opts = layout[0]
	}
	if maxVisible <= 0 {
		maxVisible = 5
	}
	filtered := append([]SelectItem(nil), items...)
	return &SelectList{items: append([]SelectItem(nil), items...), filtered: filtered, maxVisible: maxVisible, theme: theme, layout: opts}
}

func (s *SelectList) SetFilter(filter string) {
	s.mu.Lock()
	var onSelectionChange func(SelectItem)
	var selectionItem SelectItem
	var hasSelectionItem bool
	if strings.TrimSpace(filter) == "" {
		s.filtered = append(s.filtered[:0], s.items...)
		s.selectedIndex = 0
		if len(s.filtered) > 0 {
			onSelectionChange = s.OnSelectionChange
			selectionItem = s.filtered[s.selectedIndex]
			hasSelectionItem = true
		}
		s.mu.Unlock()
		if hasSelectionItem && onSelectionChange != nil {
			onSelectionChange(selectionItem)
		}
		return
	}
	s.filtered = FuzzyFilter(s.items, filter, func(item SelectItem) string {
		return strings.Join([]string{item.Value, item.Label, item.Description}, " ")
	})
	s.selectedIndex = 0
	if len(s.filtered) > 0 {
		onSelectionChange = s.OnSelectionChange
		selectionItem = s.filtered[s.selectedIndex]
		hasSelectionItem = true
	}
	s.mu.Unlock()
	if hasSelectionItem && onSelectionChange != nil {
		onSelectionChange(selectionItem)
	}
}

func (s *SelectList) SetSelectedIndex(index int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selectedIndex = max(0, min(index, len(s.filtered)-1))
}

func (s *SelectList) SelectedItem() (SelectItem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.filtered) == 0 || s.selectedIndex < 0 || s.selectedIndex >= len(s.filtered) {
		return SelectItem{}, false
	}
	return s.filtered[s.selectedIndex], true
}

func (s *SelectList) GetSelectedItem() (SelectItem, bool) {
	return s.SelectedItem()
}

func (s *SelectList) Invalidate() {}
func (s *SelectList) Render(width int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.filtered) == 0 {
		return []string{style(s.theme.NoMatch, "  No matching commands")}
	}
	start := max(0, min(s.selectedIndex-s.maxVisible/2, len(s.filtered)-s.maxVisible))
	end := min(start+s.maxVisible, len(s.filtered))
	primaryWidth := s.primaryColumnWidth()
	var lines []string
	for idx := start; idx < end; idx++ {
		item := s.filtered[idx]
		lines = append(lines, s.renderItem(item, idx == s.selectedIndex, width, primaryWidth))
	}
	if start > 0 || end < len(s.filtered) {
		lines = append(lines, style(s.theme.ScrollInfo, TruncateToWidth(fmt.Sprintf("  (%d/%d)", s.selectedIndex+1, len(s.filtered)), width-2, "")))
	}
	return lines
}

func (s *SelectList) HandleInput(data string) {
	kb := GetKeybindings()
	var onSelect func(SelectItem)
	var selectItem SelectItem
	var hasSelectItem bool
	var onCancel func()
	var onSelectionChange func(SelectItem)
	var selectionItem SelectItem
	var hasSelectionItem bool
	s.mu.Lock()
	switch {
	case kb.Matches(data, "tui.select.up"):
		if len(s.filtered) > 0 {
			if s.selectedIndex == 0 {
				s.selectedIndex = len(s.filtered) - 1
			} else {
				s.selectedIndex--
			}
			onSelectionChange = s.OnSelectionChange
			selectionItem = s.filtered[s.selectedIndex]
			hasSelectionItem = true
		}
	case kb.Matches(data, "tui.select.down"):
		if len(s.filtered) > 0 {
			s.selectedIndex = (s.selectedIndex + 1) % len(s.filtered)
			onSelectionChange = s.OnSelectionChange
			selectionItem = s.filtered[s.selectedIndex]
			hasSelectionItem = true
		}
	case kb.Matches(data, "tui.select.pageUp"):
		if len(s.filtered) > 0 {
			s.selectedIndex = max(0, s.selectedIndex-max(1, s.maxVisible))
			onSelectionChange = s.OnSelectionChange
			selectionItem = s.filtered[s.selectedIndex]
			hasSelectionItem = true
		}
	case kb.Matches(data, "tui.select.pageDown"):
		if len(s.filtered) > 0 {
			s.selectedIndex = min(len(s.filtered)-1, s.selectedIndex+max(1, s.maxVisible))
			onSelectionChange = s.OnSelectionChange
			selectionItem = s.filtered[s.selectedIndex]
			hasSelectionItem = true
		}
	case kb.Matches(data, "tui.select.confirm"):
		if len(s.filtered) > 0 {
			onSelect = s.OnSelect
			selectItem = s.filtered[s.selectedIndex]
			hasSelectItem = true
		}
	case kb.Matches(data, "tui.select.cancel"):
		onCancel = s.OnCancel
	}
	s.mu.Unlock()
	if hasSelectionItem && onSelectionChange != nil {
		onSelectionChange(selectionItem)
	}
	if hasSelectItem && onSelect != nil {
		onSelect(selectItem)
	}
	if onCancel != nil {
		onCancel()
	}
}

func (s *SelectList) renderItem(item SelectItem, selected bool, width, primaryWidth int) string {
	label := item.Label
	if label == "" {
		label = item.Value
	}
	prefix := "  "
	if selected {
		prefix = "→ "
	}
	prefixWidth := VisibleWidth(prefix)
	if item.Description != "" && width > 40 {
		effectivePrimaryWidth := max(1, min(primaryWidth, width-prefixWidth-4))
		maxPrimary := max(1, effectivePrimaryWidth-selectListPrimaryColumnGap)
		truncatedLabel := s.truncatePrimary(item, label, selected, maxPrimary, effectivePrimaryWidth)
		spacing := strings.Repeat(" ", max(1, effectivePrimaryWidth-VisibleWidth(truncatedLabel)))
		descWidth := width - prefixWidth - VisibleWidth(truncatedLabel) - VisibleWidth(spacing) - 2
		if descWidth > selectListMinDescriptionWidth {
			desc := TruncateToWidth(normalizeSelectListDescription(item.Description), descWidth, "")
			if selected {
				return style(s.theme.SelectedText, prefix+truncatedLabel+spacing+desc)
			}
			return prefix + truncatedLabel + style(s.theme.Description, spacing+desc)
		}
	}
	maxPrimary := width - prefixWidth - 2
	label = s.truncatePrimary(item, label, selected, maxPrimary, maxPrimary)
	if selected {
		return style(s.theme.SelectedText, prefix+label)
	}
	return prefix + label
}

func (s *SelectList) primaryColumnWidth() int {
	minWidth, maxWidth := s.primaryColumnBounds()
	widest := 0
	for _, item := range s.filtered {
		label := item.Label
		if label == "" {
			label = item.Value
		}
		widest = max(widest, VisibleWidth(label)+selectListPrimaryColumnGap)
	}
	return max(minWidth, min(widest, maxWidth))
}

func (s *SelectList) primaryColumnBounds() (int, int) {
	rawMin := s.layout.MinPrimaryColumnWidth
	rawMax := s.layout.MaxPrimaryColumnWidth
	if rawMin == 0 {
		if rawMax == 0 {
			rawMin = defaultSelectListPrimaryColumnWidth
		} else {
			rawMin = rawMax
		}
	}
	if rawMax == 0 {
		rawMax = rawMin
	}
	return max(1, min(rawMin, rawMax)), max(1, max(rawMin, rawMax))
}

func (s *SelectList) truncatePrimary(item SelectItem, label string, selected bool, maxWidth, columnWidth int) string {
	maxWidth = max(1, maxWidth)
	if s.layout.TruncatePrimary != nil {
		label = s.layout.TruncatePrimary(SelectListTruncatePrimaryContext{Text: label, MaxWidth: maxWidth, ColumnWidth: columnWidth, Item: item, IsSelected: selected})
	}
	return TruncateToWidth(label, maxWidth, "")
}

func normalizeSelectListDescription(text string) string {
	var out strings.Builder
	lastWasLineBreak := false
	for _, r := range text {
		if r == '\r' || r == '\n' {
			if !lastWasLineBreak {
				out.WriteByte(' ')
				lastWasLineBreak = true
			}
			continue
		}
		out.WriteRune(r)
		lastWasLineBreak = false
	}
	return strings.TrimSpace(out.String())
}

func (s *SelectList) notifySelectionChange() {
	s.mu.Lock()
	var callback func(SelectItem)
	var item SelectItem
	ok := len(s.filtered) > 0
	if ok {
		callback = s.OnSelectionChange
		item = s.filtered[s.selectedIndex]
	}
	s.mu.Unlock()
	if ok && callback != nil {
		callback(item)
	}
}
