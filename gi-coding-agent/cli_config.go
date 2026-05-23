package gicodingagent

import (
	"errors"
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
	items, toggles := packageResourceSettingItems(resources)
	var mu sync.Mutex
	var runErr error
	done := make(chan struct{}, 1)
	list := gitui.NewSettingsList(items, 16, gitui.SettingsListTheme{}, gitui.SettingsListOptions{
		EnableSearch: true,
		OnChange: func(id, newValue string) {
			toggle, ok := toggles[id]
			if !ok {
				mu.Lock()
				runErr = errors.New("resource not found")
				mu.Unlock()
				return
			}
			updated, err := applyResourceToggle(manager, toggle, newValue == "enabled")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				runErr = err
				return
			}
			if !updated {
				runErr = errors.New("resource not found")
				return
			}
		},
		OnCancel: func() {
			select {
			case done <- struct{}{}:
			default:
			}
		},
	})
	component := packageResourceConfigComponent{dialog: cliSettingsListDialog{title: "Resource Configuration", list: list}, done: done}
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
	dialog cliSettingsListDialog
	done   chan<- struct{}
}

func (c packageResourceConfigComponent) Invalidate() {
	c.dialog.Invalidate()
}

func (c packageResourceConfigComponent) Render(width int) []string {
	return c.dialog.Render(width)
}

func (c packageResourceConfigComponent) HandleInput(data string) {
	if gitui.MatchesKey(data, "ctrl+c") {
		select {
		case c.done <- struct{}{}:
		default:
		}
		return
	}
	c.dialog.HandleInput(data)
}
