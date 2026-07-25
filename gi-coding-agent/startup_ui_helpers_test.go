package gicodingagent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gitui "github.com/nowa/gi/gi-tui"
)

func TestLoadStartupThemesUsesGlobalUntrustedSnapshotPiStyle(
	t *testing.T,
) {
	root := t.TempDir()
	agentDir := filepath.Join(root, "agent")
	cwd := filepath.Join(root, "project")
	globalThemePath := filepath.Join(
		agentDir,
		"themes",
		"focus.json",
	)
	writeJSON(
		t,
		globalThemePath,
		completeTUIThemeFixture("focus", nil),
	)
	writeJSON(
		t,
		filepath.Join(agentDir, "themes", "broken.json"),
		map[string]any{"name": "broken"},
	)
	writeJSON(
		t,
		filepath.Join(cwd, ConfigDirName, "themes", "project.json"),
		completeTUIThemeFixture("project-only", nil),
	)

	settings := NewSettingsManager(cwd, agentDir)
	themes := loadStartupThemes(settings)
	if theme, ok := findTUIThemeInfo(themes, "focus"); !ok ||
		theme.Path != globalThemePath {
		t.Fatalf("global startup theme = %#v, themes = %#v", theme, themes)
	}
	if _, ok := findTUIThemeInfo(themes, "broken"); ok {
		t.Fatalf("broken theme should be ignored: %#v", themes)
	}
	if _, ok := findTUIThemeInfo(themes, "project-only"); ok {
		t.Fatalf("project theme crossed startup trust boundary: %#v", themes)
	}
}

func TestShowStartupSelectorReturnsTypedSelectionPiStyle(
	t *testing.T,
) {
	terminal := gitui.NewVirtualTerminal(80, 20)
	settings := NewInMemorySettingsManager(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result := make(chan struct {
		value    int
		selected bool
		err      error
	}, 1)
	go func() {
		value, selected, err := showStartupSelector(
			ctx,
			settings,
			"Choose startup value",
			[]startupSelectorOption[int]{
				{label: "first", value: 10},
				{label: "second", value: 20},
			},
			startupTUIOptions{
				terminal: terminal,
				detectTheme: func(
					context.Context,
					*gitui.TUI,
					time.Duration,
					map[string]string,
				) TerminalTheme {
					return TerminalThemeDark
				},
			},
		)
		result <- struct {
			value    int
			selected bool
			err      error
		}{value: value, selected: selected, err: err}
	}()

	waitForStartupUIViewport(t, terminal, "Choose startup value")
	terminal.SendInput("j")
	terminal.SendInput("\r")
	select {
	case outcome := <-result:
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if !outcome.selected || outcome.value != 20 {
			t.Fatalf("selector outcome = %#v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("startup selector did not finish")
	}
}

func TestShowStartupInputReturnsSubmittedTextPiStyle(
	t *testing.T,
) {
	terminal := gitui.NewVirtualTerminal(80, 20)
	settings := NewInMemorySettingsManager(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result := make(chan struct {
		value     string
		submitted bool
		err       error
	}, 1)
	go func() {
		value, submitted, err := showStartupInput(
			ctx,
			settings,
			"Enter startup value",
			"placeholder text",
			startupTUIOptions{
				terminal: terminal,
				detectTheme: func(
					context.Context,
					*gitui.TUI,
					time.Duration,
					map[string]string,
				) TerminalTheme {
					return TerminalThemeDark
				},
			},
		)
		result <- struct {
			value     string
			submitted bool
			err       error
		}{value: value, submitted: submitted, err: err}
	}()

	waitForStartupUIViewport(t, terminal, "Enter startup value")
	waitForStartupUIViewport(t, terminal, "placeholder text")
	terminal.SendInput("hello")
	terminal.SendInput("\r")
	select {
	case outcome := <-result:
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if !outcome.submitted || outcome.value != "hello" {
			t.Fatalf("input outcome = %#v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("startup input did not finish")
	}
}

func waitForStartupUIViewport(
	t *testing.T,
	terminal *gitui.VirtualTerminal,
	expected string,
) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		terminal.WaitForRender()
		if strings.Contains(
			strings.Join(terminal.GetViewport(), "\n"),
			expected,
		) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf(
		"startup viewport did not contain %q:\n%s",
		expected,
		strings.Join(terminal.GetViewport(), "\n"),
	)
}
