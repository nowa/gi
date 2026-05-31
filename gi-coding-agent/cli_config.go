package gicodingagent

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"

	gitui "github.com/nowa/gi/gi-tui"
)

type PackageResourceConfigOptions struct {
	CWD      string
	AgentDir string
	Terminal gitui.Terminal
}

type packageResourceConfigHost struct {
	cwd      string
	agentDir string
	terminal gitui.Terminal
}

func newDefaultCLIConfigHost(options PackageResourceConfigOptions) (CLIConfigRuntimeHost, error) {
	return &packageResourceConfigHost{
		cwd:      options.CWD,
		agentDir: options.AgentDir,
		terminal: options.Terminal,
	}, nil
}

func (h *packageResourceConfigHost) Run() error {
	if h == nil {
		return errors.New("config host is nil")
	}
	settings := NewSettingsManager(h.cwd, h.agentDir)
	manager := NewDefaultPackageManager(PackageManagerOptions{
		CWD:             h.cwd,
		AgentDir:        h.agentDir,
		SettingsManager: settings,
	})
	resources, err := manager.ListResourceToggles()
	if err != nil {
		return err
	}
	var mu sync.Mutex
	var runErr error
	done := make(chan struct{}, 1)
	component := newPackageResourceConfigComponent(manager, resources, done, func(err error) {
		mu.Lock()
		defer mu.Unlock()
		runErr = err
	})
	ui := gitui.NewTUI(h.terminal)
	ui.AddChild(component)
	ui.SetFocus(component)
	ui.Start()
	<-done
	ui.Stop()
	mu.Lock()
	defer mu.Unlock()
	if runErr != nil {
		return runErr
	}
	return nil
}

type packageResourceConfigComponent struct {
	manager    *DefaultPackageManager
	resources  []PackageResourceToggleItem
	entries    []packageResourceConfigEntry
	filtered   []int
	selected   int
	search     *gitui.Input
	done       chan<- struct{}
	onError    func(error)
	maxVisible int
}

type packageResourceConfigEntry struct {
	Kind         string
	Label        string
	GroupKey     string
	SubgroupKey  string
	ResourceType string
	ItemIndex    int
}

func newPackageResourceConfigComponent(manager *DefaultPackageManager, resources []PackageResourceToggleItem, done chan<- struct{}, onError func(error)) *packageResourceConfigComponent {
	c := &packageResourceConfigComponent{
		manager:    manager,
		resources:  append([]PackageResourceToggleItem(nil), resources...),
		search:     gitui.NewInput(),
		done:       done,
		onError:    onError,
		maxVisible: 15,
	}
	c.rebuildEntries()
	c.resetFilter()
	return c
}

func (c *packageResourceConfigComponent) Invalidate() {
	if c != nil && c.search != nil {
		c.search.Invalidate()
	}
}

func (c *packageResourceConfigComponent) Render(width int) []string {
	if c == nil {
		return nil
	}
	width = max(32, width)
	lines := []string{
		"",
		tuiThemeBorder(strings.Repeat("─", width)),
		"",
		c.renderHeader(width),
		tuiThemeMuted("Type to filter resources"),
		"",
	}
	if c.search != nil {
		lines = append(lines, c.search.Render(width)...)
		lines = append(lines, "")
	}
	lines = append(lines, c.renderResourceList(width)...)
	lines = append(lines, "", tuiThemeBorder(strings.Repeat("─", width)))
	return lines
}

func (c *packageResourceConfigComponent) HandleInput(data string) {
	if c == nil {
		return
	}
	kb := gitui.GetKeybindings()
	switch {
	case kb.Matches(data, "tui.select.up"):
		c.selected = c.findNextItem(c.selected, -1)
	case kb.Matches(data, "tui.select.down"):
		c.selected = c.findNextItem(c.selected, 1)
	case kb.Matches(data, "tui.select.pageUp"):
		c.pageSelection(-1)
	case kb.Matches(data, "tui.select.pageDown"):
		c.pageSelection(1)
	case kb.Matches(data, "tui.select.cancel"):
		c.finish()
	case gitui.MatchesKey(data, "ctrl+c"):
		c.finish()
	case data == " " || kb.Matches(data, "tui.select.confirm"):
		c.toggleSelected()
	default:
		if c.search == nil {
			return
		}
		c.search.HandleInput(data)
		c.applyFilter(c.search.GetValue())
	}
}

func (c *packageResourceConfigComponent) renderHeader(width int) string {
	title := tuiThemeBold("Resource Configuration")
	hint := tuiThemeMuted("space toggle · esc close")
	spacing := max(1, width-gitui.VisibleWidth(title)-gitui.VisibleWidth(hint))
	return gitui.TruncateToWidth(title+strings.Repeat(" ", spacing)+hint, width, "", true)
}

func (c *packageResourceConfigComponent) renderResourceList(width int) []string {
	if len(c.filtered) == 0 {
		return []string{tuiThemeMuted("  No resources found")}
	}
	start := max(0, min(c.selected-c.maxVisible/2, len(c.filtered)-c.maxVisible))
	end := min(start+c.maxVisible, len(c.filtered))
	var lines []string
	for row := start; row < end; row++ {
		entry := c.entries[c.filtered[row]]
		selected := row == c.selected
		switch entry.Kind {
		case "group":
			lines = append(lines, gitui.TruncateToWidth("  "+tuiThemeBoldAccent(entry.Label), width, "", true))
		case "subgroup":
			lines = append(lines, gitui.TruncateToWidth("    "+tuiThemeMuted(entry.Label), width, "", true))
		case "item":
			item := c.resources[entry.ItemIndex]
			cursor := "  "
			if selected {
				cursor = "> "
			}
			checkbox := tuiThemeDim("[ ]")
			if item.Enabled {
				checkbox = tuiThemeSuccess("[x]")
			}
			name := item.DisplayName
			if name == "" {
				name = item.Pattern
			}
			if selected {
				name = tuiThemeBold(name)
			}
			lines = append(lines, gitui.TruncateToWidth(cursor+"    "+checkbox+" "+name, width, "...", true))
		}
	}
	if start > 0 || end < len(c.filtered) {
		lines = append(lines, tuiThemeDim(gitui.TruncateToWidth(c.scrollLabel(), width, "", true)))
	}
	return lines
}

func (c *packageResourceConfigComponent) rebuildEntries() {
	type subgroup struct {
		resourceType string
		label        string
		items        []int
	}
	type group struct {
		key       string
		label     string
		subgroups map[string]*subgroup
		order     []string
	}
	groupsByKey := map[string]*group{}
	var groupOrder []string
	for index, resource := range c.resources {
		groupKey := packageResourceConfigGroupKey(resource)
		g := groupsByKey[groupKey]
		if g == nil {
			g = &group{
				key:       groupKey,
				label:     packageResourceConfigGroupLabel(resource),
				subgroups: map[string]*subgroup{},
			}
			groupsByKey[groupKey] = g
			groupOrder = append(groupOrder, groupKey)
		}
		resourceType := resource.ResourceType
		sg := g.subgroups[resourceType]
		if sg == nil {
			sg = &subgroup{resourceType: resourceType, label: packageResourceConfigSubgroupLabel(resourceType)}
			g.subgroups[resourceType] = sg
			g.order = append(g.order, resourceType)
		}
		sg.items = append(sg.items, index)
	}
	c.entries = nil
	for _, groupKey := range groupOrder {
		g := groupsByKey[groupKey]
		c.entries = append(c.entries, packageResourceConfigEntry{Kind: "group", Label: g.label, GroupKey: g.key})
		for _, resourceType := range packageResourceTypeOrder(g.order) {
			sg := g.subgroups[resourceType]
			subgroupKey := g.key + "\x1f" + resourceType
			c.entries = append(c.entries, packageResourceConfigEntry{Kind: "subgroup", Label: sg.label, GroupKey: g.key, SubgroupKey: subgroupKey, ResourceType: resourceType})
			for _, itemIndex := range sg.items {
				c.entries = append(c.entries, packageResourceConfigEntry{Kind: "item", GroupKey: g.key, SubgroupKey: subgroupKey, ResourceType: resourceType, ItemIndex: itemIndex})
			}
		}
	}
}

func (c *packageResourceConfigComponent) resetFilter() {
	c.filtered = make([]int, len(c.entries))
	for index := range c.entries {
		c.filtered[index] = index
	}
	c.selectFirstItem()
}

func (c *packageResourceConfigComponent) applyFilter(query string) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		c.resetFilter()
		return
	}
	matchingGroups := map[string]bool{}
	matchingSubgroups := map[string]bool{}
	matchingItems := map[int]bool{}
	for entryIndex, entry := range c.entries {
		if entry.Kind != "item" {
			continue
		}
		item := c.resources[entry.ItemIndex]
		searchText := strings.ToLower(strings.Join([]string{
			item.DisplayName,
			item.ResourceType,
			item.Pattern,
			item.Path,
		}, "\n"))
		if strings.Contains(searchText, query) {
			matchingGroups[entry.GroupKey] = true
			matchingSubgroups[entry.SubgroupKey] = true
			matchingItems[entryIndex] = true
		}
	}
	c.filtered = c.filtered[:0]
	for entryIndex, entry := range c.entries {
		switch entry.Kind {
		case "group":
			if matchingGroups[entry.GroupKey] {
				c.filtered = append(c.filtered, entryIndex)
			}
		case "subgroup":
			if matchingSubgroups[entry.SubgroupKey] {
				c.filtered = append(c.filtered, entryIndex)
			}
		case "item":
			if matchingItems[entryIndex] {
				c.filtered = append(c.filtered, entryIndex)
			}
		}
	}
	c.selectFirstItem()
}

func (c *packageResourceConfigComponent) selectFirstItem() {
	c.selected = 0
	for index, entryIndex := range c.filtered {
		if c.entries[entryIndex].Kind == "item" {
			c.selected = index
			return
		}
	}
}

func (c *packageResourceConfigComponent) findNextItem(from, direction int) int {
	if len(c.filtered) == 0 {
		return 0
	}
	index := from + direction
	for index >= 0 && index < len(c.filtered) {
		if c.entries[c.filtered[index]].Kind == "item" {
			return index
		}
		index += direction
	}
	return from
}

func (c *packageResourceConfigComponent) pageSelection(direction int) {
	if len(c.filtered) == 0 {
		return
	}
	target := c.selected + direction*c.maxVisible
	target = max(0, min(target, len(c.filtered)-1))
	step := 1
	if direction < 0 {
		step = -1
	}
	for target >= 0 && target < len(c.filtered) {
		if c.entries[c.filtered[target]].Kind == "item" {
			c.selected = target
			return
		}
		target += step
	}
}

func (c *packageResourceConfigComponent) toggleSelected() {
	if len(c.filtered) == 0 || c.selected < 0 || c.selected >= len(c.filtered) {
		return
	}
	entry := c.entries[c.filtered[c.selected]]
	if entry.Kind != "item" {
		return
	}
	item := c.resources[entry.ItemIndex]
	enabled := !item.Enabled
	updated, err := applyResourceToggle(c.manager, packageResourceToggleSelection(item), enabled)
	if err != nil {
		c.reportError(err)
		return
	}
	if !updated {
		c.reportError(errors.New("resource not found"))
		return
	}
	c.resources[entry.ItemIndex].Enabled = enabled
}

func (c *packageResourceConfigComponent) reportError(err error) {
	if c != nil && c.onError != nil && err != nil {
		c.onError(err)
	}
}

func (c *packageResourceConfigComponent) finish() {
	select {
	case c.done <- struct{}{}:
	default:
	}
}

func (c *packageResourceConfigComponent) scrollLabel() string {
	itemCount := 0
	currentItem := 0
	for index, entryIndex := range c.filtered {
		if c.entries[entryIndex].Kind != "item" {
			continue
		}
		itemCount++
		if index <= c.selected {
			currentItem = itemCount
		}
	}
	return "  (" + strconv.Itoa(currentItem) + "/" + strconv.Itoa(itemCount) + ")"
}

func packageResourceConfigGroupKey(resource PackageResourceToggleItem) string {
	origin := resource.Metadata.Origin
	if origin == "" {
		origin = "package"
	}
	return strings.Join([]string{origin, resource.Scope, resource.Source}, "\x1f")
}

func packageResourceConfigGroupLabel(resource PackageResourceToggleItem) string {
	if resource.Metadata.Origin == "top-level" {
		if resource.Scope == "project" {
			return "Project (.gi/)"
		}
		return "User (~/.gi/agent/)"
	}
	return strings.TrimSpace(resource.Source + " (" + firstNonEmptyString(resource.Scope, "user") + ")")
}

func packageResourceConfigSubgroupLabel(resourceType string) string {
	switch resourceType {
	case "extensions":
		return "Extensions"
	case "skills":
		return "Skills"
	case "prompts":
		return "Prompts"
	case "themes":
		return "Themes"
	default:
		return strings.TrimSpace(resourceType)
	}
}

func packageResourceTypeOrder(values []string) []string {
	order := map[string]int{"extensions": 0, "skills": 1, "prompts": 2, "themes": 3}
	out := append([]string(nil), values...)
	sort.SliceStable(out, func(i, j int) bool {
		left, right := order[out[i]], order[out[j]]
		if left != right {
			return left < right
		}
		return out[i] < out[j]
	})
	return out
}

func packageResourceToggleSelection(resource PackageResourceToggleItem) resourceToggleSelection {
	if resource.Metadata.Origin == "top-level" {
		return resourceToggleSelection{TopLevel: TopLevelResourceToggle{
			Scope:        resource.Scope,
			ResourceType: resource.ResourceType,
			Pattern:      resource.Pattern,
			Enabled:      resource.Enabled,
		}}
	}
	return resourceToggleSelection{Package: PackageResourceToggle{
		Source:       resource.Source,
		Scope:        resource.Scope,
		ResourceType: resource.ResourceType,
		Pattern:      resource.Pattern,
		Enabled:      resource.Enabled,
	}}
}
