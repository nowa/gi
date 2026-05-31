package gitui

import (
	"fmt"
	"strings"
	"sync"
)

type SettingItem struct {
	ID           string
	Label        string
	Description  string
	Value        string
	CurrentValue string
	Values       []string
	Submenu      func(currentValue string, done func(selectedValue string, changed bool)) Component
}

type SettingsListTheme struct {
	Label        func(text string, selected bool) string
	CurrentValue func(text string, selected bool) string
	Description  func(string) string
	Hint         func(string) string
	Selected     func(string) string
	Value        func(string) string
	Cursor       string
}

type SettingsListOptions struct {
	EnableSearch bool
	OnChange     func(id string, newValue string)
	OnCancel     func()
}

type SettingsList struct {
	mu               sync.Mutex
	items            []SettingItem
	filteredIndices  []int
	selectedIndex    int
	maxVisible       int
	theme            SettingsListTheme
	options          SettingsListOptions
	searchInput      *Input
	submenu          Component
	submenuItemIndex int
}

func NewSettingsList(items []SettingItem, maxVisible int, theme SettingsListTheme, options ...SettingsListOptions) *SettingsList {
	opts := SettingsListOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	if maxVisible <= 0 {
		maxVisible = 5
	}
	s := &SettingsList{
		items:            append([]SettingItem(nil), items...),
		maxVisible:       maxVisible,
		theme:            theme,
		options:          opts,
		submenuItemIndex: -1,
	}
	s.resetFilter()
	if opts.EnableSearch {
		s.searchInput = NewInput()
	}
	return s
}

func (s *SettingsList) UpdateValue(id string, newValue string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for idx := range s.items {
		if s.settingID(s.items[idx]) == id {
			s.items[idx].CurrentValue = newValue
			s.items[idx].Value = newValue
			return
		}
	}
}

func (s *SettingsList) Invalidate() {
	s.mu.Lock()
	submenu := s.submenu
	searchInput := s.searchInput
	s.mu.Unlock()
	if submenu != nil {
		submenu.Invalidate()
	}
	if searchInput != nil {
		searchInput.Invalidate()
	}
}

func (s *SettingsList) Render(width int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.submenu != nil {
		return s.submenu.Render(width)
	}
	var lines []string
	if s.searchInput != nil {
		lines = append(lines, s.searchInput.Render(width)...)
		lines = append(lines, "")
	}
	if len(s.items) == 0 {
		lines = append(lines, style(s.theme.Hint, "  No settings available"))
		if s.searchInput != nil {
			return s.appendSettingsHint(lines, width)
		}
		return lines
	}
	if len(s.filteredIndices) == 0 {
		lines = append(lines, TruncateToWidth(style(s.theme.Hint, "  No matching settings"), width, ""))
		return s.appendSettingsHint(lines, width)
	}

	start := max(0, min(s.selectedIndex-s.maxVisible/2, len(s.filteredIndices)-s.maxVisible))
	end := min(start+s.maxVisible, len(s.filteredIndices))
	labelWidth := s.maxLabelWidth()
	for row := start; row < end; row++ {
		item := s.items[s.filteredIndices[row]]
		selected := row == s.selectedIndex
		prefix := "  "
		if selected {
			prefix = s.theme.Cursor
			if prefix == "" {
				prefix = "→ "
			}
		}
		label := item.Label
		label = label + strings.Repeat(" ", max(0, labelWidth-VisibleWidth(label)))
		label = s.styleSettingLabel(label, selected)
		valueWidth := max(1, width-VisibleWidth(prefix)-labelWidth-4)
		value := TruncateToWidth(s.settingValue(item), valueWidth, "")
		value = s.styleSettingValue(value, selected)
		lines = append(lines, TruncateToWidth(prefix+label+"  "+value, width, ""))
	}
	if start > 0 || end < len(s.filteredIndices) {
		lines = append(lines, style(s.theme.Hint, TruncateToWidth(fmt.Sprintf("  (%d/%d)", s.selectedIndex+1, len(s.filteredIndices)), width, "")))
	}
	item := s.items[s.filteredIndices[s.selectedIndex]]
	if item.Description != "" {
		lines = append(lines, "")
		for _, line := range WrapTextWithANSI(item.Description, max(1, width-4)) {
			lines = append(lines, style(s.theme.Description, "  "+line))
		}
	}
	return s.appendSettingsHint(lines, width)
}

func (s *SettingsList) HandleInput(data string) {
	var after func()
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		if after != nil {
			after()
		}
	}()
	if s.submenu != nil {
		if handler, ok := s.submenu.(InputHandler); ok {
			s.mu.Unlock()
			handler.HandleInput(data)
			s.mu.Lock()
		}
		return
	}
	kb := GetKeybindings()
	switch {
	case kb.Matches(data, "tui.select.up"):
		if len(s.filteredIndices) > 0 {
			if s.selectedIndex == 0 {
				s.selectedIndex = len(s.filteredIndices) - 1
			} else {
				s.selectedIndex--
			}
		}
	case kb.Matches(data, "tui.select.down"):
		if len(s.filteredIndices) > 0 {
			s.selectedIndex = (s.selectedIndex + 1) % len(s.filteredIndices)
		}
	case kb.Matches(data, "tui.select.confirm") || data == " ":
		after = s.activateSelectedSetting()
	case kb.Matches(data, "tui.select.cancel"):
		if s.options.OnCancel != nil {
			onCancel := s.options.OnCancel
			after = func() { onCancel() }
		}
	default:
		if s.searchInput == nil {
			return
		}
		sanitized := strings.ReplaceAll(data, " ", "")
		if sanitized == "" {
			return
		}
		s.searchInput.HandleInput(sanitized)
		s.applySettingsFilter(s.searchInput.GetValue())
	}
}

func (s *SettingsList) activateSelectedSetting() func() {
	if len(s.filteredIndices) == 0 {
		return nil
	}
	itemIndex := s.filteredIndices[s.selectedIndex]
	item := &s.items[itemIndex]
	if item.Submenu != nil {
		s.submenuItemIndex = s.selectedIndex
		s.submenu = item.Submenu(s.settingValue(*item), func(selectedValue string, changed bool) {
			var after func()
			s.mu.Lock()
			if changed {
				if itemIndex >= 0 && itemIndex < len(s.items) {
					s.items[itemIndex].CurrentValue = selectedValue
					s.items[itemIndex].Value = selectedValue
					id := s.settingID(s.items[itemIndex])
					if s.options.OnChange != nil {
						onChange := s.options.OnChange
						after = func() { onChange(id, selectedValue) }
					}
				}
			}
			s.submenu = nil
			if s.submenuItemIndex >= 0 && s.submenuItemIndex < len(s.filteredIndices) {
				s.selectedIndex = s.submenuItemIndex
			}
			s.submenuItemIndex = -1
			s.mu.Unlock()
			if after != nil {
				after()
			}
		})
		return nil
	}
	if len(item.Values) == 0 {
		return nil
	}
	current := s.settingValue(*item)
	nextIndex := 0
	for idx, value := range item.Values {
		if value == current {
			nextIndex = (idx + 1) % len(item.Values)
			break
		}
	}
	next := item.Values[nextIndex]
	item.CurrentValue = next
	item.Value = next
	if s.options.OnChange == nil {
		return nil
	}
	id := s.settingID(*item)
	onChange := s.options.OnChange
	return func() { onChange(id, next) }
}

func (s *SettingsList) notifySettingsChange(item SettingItem, value string) {
	if s.options.OnChange != nil {
		s.options.OnChange(s.settingID(item), value)
	}
}

func (s *SettingsList) applySettingsFilter(query string) {
	if strings.TrimSpace(query) == "" {
		s.resetFilter()
		return
	}
	type candidate struct {
		index int
		label string
	}
	candidates := make([]candidate, len(s.items))
	for idx, item := range s.items {
		candidates[idx] = candidate{index: idx, label: item.Label}
	}
	matches := FuzzyFilter(candidates, query, func(item candidate) string { return item.label })
	s.filteredIndices = s.filteredIndices[:0]
	for _, match := range matches {
		s.filteredIndices = append(s.filteredIndices, match.index)
	}
	s.selectedIndex = 0
}

func (s *SettingsList) resetFilter() {
	s.filteredIndices = make([]int, len(s.items))
	for idx := range s.items {
		s.filteredIndices[idx] = idx
	}
	s.selectedIndex = 0
}

func (s *SettingsList) maxLabelWidth() int {
	width := 0
	for _, item := range s.items {
		width = max(width, VisibleWidth(item.Label))
	}
	return min(30, width)
}

func (s *SettingsList) settingID(item SettingItem) string {
	if item.ID != "" {
		return item.ID
	}
	return item.Label
}

func (s *SettingsList) settingValue(item SettingItem) string {
	if item.CurrentValue != "" {
		return item.CurrentValue
	}
	if settingValuesContainEmpty(item.Values) {
		return ""
	}
	return item.Value
}

func settingValuesContainEmpty(values []string) bool {
	for _, value := range values {
		if value == "" {
			return true
		}
	}
	return false
}

func (s *SettingsList) styleSettingLabel(text string, selected bool) string {
	if s.theme.Label != nil {
		return s.theme.Label(text, selected)
	}
	if selected {
		return style(s.theme.Selected, text)
	}
	return text
}

func (s *SettingsList) styleSettingValue(text string, selected bool) string {
	if s.theme.CurrentValue != nil {
		return s.theme.CurrentValue(text, selected)
	}
	if s.theme.Value != nil {
		return s.theme.Value(text)
	}
	return text
}

func (s *SettingsList) appendSettingsHint(lines []string, width int) []string {
	lines = append(lines, "")
	hint := "  Enter/Space to change · Esc to cancel"
	if s.searchInput != nil {
		hint = "  Type to search · Enter/Space to change · Esc to cancel"
	}
	return append(lines, TruncateToWidth(style(s.theme.Hint, hint), width, ""))
}
