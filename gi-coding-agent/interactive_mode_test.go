package gicodingagent

import (
	"errors"
	"reflect"
	"testing"
	"time"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
)

func TestInteractiveModeCloneCommandMatchesPi(t *testing.T) {
	t.Run("clones the current leaf into a new session", func(t *testing.T) {
		host := &fakeInteractiveRuntimeHost{}
		editor := &fakeInteractiveEditor{}
		ui := &fakeInteractiveUI{}
		var rendered int
		statuses := []string{}
		errorsShown := []string{}
		mode := &InteractiveMode{
			SessionManager:            fakeInteractiveLeafProvider{leaf: interactiveStringPtr("leaf-123")},
			RuntimeHost:               host,
			Editor:                    editor,
			UI:                        ui,
			RenderCurrentSessionState: func() { rendered++ },
			ShowStatus:                func(message string) { statuses = append(statuses, message) },
			ShowError:                 func(message string) { errorsShown = append(errorsShown, message) },
		}

		if err := mode.HandleCloneCommand(); err != nil {
			t.Fatal(err)
		}

		if host.forkEntryID != "leaf-123" || host.forkOptions.Position != "at" {
			t.Fatalf("fork = %q %#v, want leaf-123 position at", host.forkEntryID, host.forkOptions)
		}
		if rendered != 1 || editor.text != "" || !reflect.DeepEqual(statuses, []string{"Cloned to new session"}) {
			t.Fatalf("rendered=%d editor=%q statuses=%#v", rendered, editor.text, statuses)
		}
		if len(errorsShown) != 0 || len(ui.renderForces) != 0 {
			t.Fatalf("errors=%#v renders=%#v", errorsShown, ui.renderForces)
		}
	})

	t.Run("shows a status message when there is nothing to clone", func(t *testing.T) {
		host := &fakeInteractiveRuntimeHost{}
		statuses := []string{}
		errorsShown := []string{}
		mode := &InteractiveMode{
			SessionManager: fakeInteractiveLeafProvider{},
			RuntimeHost:    host,
			ShowStatus:     func(message string) { statuses = append(statuses, message) },
			ShowError:      func(message string) { errorsShown = append(errorsShown, message) },
		}

		if err := mode.HandleCloneCommand(); err != nil {
			t.Fatal(err)
		}

		if host.forkCalled {
			t.Fatal("fork should not be called")
		}
		if !reflect.DeepEqual(statuses, []string{"Nothing to clone yet"}) || len(errorsShown) != 0 {
			t.Fatalf("statuses=%#v errors=%#v", statuses, errorsShown)
		}
	})
}

func TestInteractiveModeImportPathParsingMatchesPi(t *testing.T) {
	mode := &InteractiveMode{}
	assertPathCommandArg := func(input, command string, want string, wantOK bool) {
		t.Helper()
		got, ok := mode.GetPathCommandArgument(input, command)
		if got != want || ok != wantOK {
			t.Fatalf("GetPathCommandArgument(%q, %q) = %q %v, want %q %v", input, command, got, ok, want, wantOK)
		}
	}

	assertPathCommandArg(`/import "path/to/session.jsonl"`, "/import", "path/to/session.jsonl", true)
	assertPathCommandArg(`/import "path with spaces/session.jsonl"`, "/import", "path with spaces/session.jsonl", true)
	assertPathCommandArg(`/import john's/session.jsonl`, "/import", "john's/session.jsonl", true)
	assertPathCommandArg(`/important /tmp/session.jsonl`, "/import", "", false)
	assertPathCommandArg(`/exporter out.html`, "/export", "", false)
	assertPathCommandArg(`/import /tmp/session.jsonl`, "/import", "/tmp/session.jsonl", true)
}

func TestInteractiveModeImportCommandMatchesPi(t *testing.T) {
	t.Run("passes unquoted path to runtimeHost.importFromJsonl", func(t *testing.T) {
		host := &fakeInteractiveRuntimeHost{}
		statuses := []string{}
		errorsShown := []string{}
		confirm := []string{}
		mode := &InteractiveMode{
			RuntimeHost: host,
			Status:      &fakeInteractiveContainer{},
			ShowStatus:  func(message string) { statuses = append(statuses, message) },
			ShowError:   func(message string) { errorsShown = append(errorsShown, message) },
			ShowExtensionConfirm: func(title, message string) (bool, error) {
				confirm = append(confirm, title, message)
				return true, nil
			},
			RenderCurrentSessionState: func() {},
			HandleFatalRuntimeError: func(string, error) error {
				t.Fatal("fatal runtime error should not be called")
				return nil
			},
		}

		if err := mode.HandleImportCommand(`/import "path/to/session.jsonl"`); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(confirm, []string{"Import session", "Replace current session with path/to/session.jsonl?"}) {
			t.Fatalf("confirm = %#v", confirm)
		}
		if !reflect.DeepEqual(host.importCalls, []fakeImportCall{{path: "path/to/session.jsonl"}}) {
			t.Fatalf("import calls = %#v", host.importCalls)
		}
		if len(errorsShown) != 0 || !reflect.DeepEqual(statuses, []string{"Session imported from: path/to/session.jsonl"}) {
			t.Fatalf("statuses=%#v errors=%#v", statuses, errorsShown)
		}
	})

	t.Run("passes unquoted apostrophe path to runtimeHost.importFromJsonl unchanged", func(t *testing.T) {
		host := &fakeInteractiveRuntimeHost{}
		statuses := []string{}
		errorsShown := []string{}
		mode := &InteractiveMode{
			RuntimeHost:               host,
			Status:                    &fakeInteractiveContainer{},
			ShowStatus:                func(message string) { statuses = append(statuses, message) },
			ShowError:                 func(message string) { errorsShown = append(errorsShown, message) },
			ShowExtensionConfirm:      func(string, string) (bool, error) { return true, nil },
			RenderCurrentSessionState: func() {},
			HandleFatalRuntimeError: func(string, error) error {
				t.Fatal("fatal runtime error should not be called")
				return nil
			},
		}

		if err := mode.HandleImportCommand(`/import john's/session.jsonl`); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(host.importCalls, []fakeImportCall{{path: "john's/session.jsonl"}}) {
			t.Fatalf("import calls = %#v", host.importCalls)
		}
		if len(errorsShown) != 0 || !reflect.DeepEqual(statuses, []string{"Session imported from: john's/session.jsonl"}) {
			t.Fatalf("statuses=%#v errors=%#v", statuses, errorsShown)
		}
	})

	t.Run("shows a non-fatal error when /import path does not exist", func(t *testing.T) {
		host := &fakeInteractiveRuntimeHost{importErr: SessionImportFileNotFoundError{Path: "/tmp/missing-session.jsonl"}}
		statuses := []string{}
		errorsShown := []string{}
		fatalCalled := false
		mode := &InteractiveMode{
			RuntimeHost:          host,
			Status:               &fakeInteractiveContainer{},
			ShowStatus:           func(message string) { statuses = append(statuses, message) },
			ShowError:            func(message string) { errorsShown = append(errorsShown, message) },
			ShowExtensionConfirm: func(string, string) (bool, error) { return true, nil },
			HandleFatalRuntimeError: func(string, error) error {
				fatalCalled = true
				return nil
			},
		}

		if err := mode.HandleImportCommand("/import /tmp/missing-session.jsonl"); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(errorsShown, []string{"Failed to import session: File not found: /tmp/missing-session.jsonl"}) {
			t.Fatalf("errors = %#v", errorsShown)
		}
		if len(statuses) != 0 || fatalCalled {
			t.Fatalf("statuses=%#v fatal=%v", statuses, fatalCalled)
		}
	})
}

func TestInteractiveModeCompactionEndRebuildsChatAndSummaryMatchesPi(t *testing.T) {
	chat := &fakeInteractiveContainer{}
	footer := &fakeInteractiveFooter{}
	ui := &fakeInteractiveUI{}
	var rebuilds int
	var messages []InteractiveMessage
	var flushes []InteractiveFlushCompactionOptions
	mode := &InteractiveMode{
		Chat:                    chat,
		Footer:                  footer,
		UI:                      ui,
		Settings:                fakeInteractiveSettings{},
		RebuildChatFromMessages: func() { rebuilds++ },
		AddMessageToChat:        func(message InteractiveMessage) { messages = append(messages, message) },
		FlushCompactionQueue: func(options InteractiveFlushCompactionOptions) error {
			flushes = append(flushes, options)
			return nil
		},
	}

	err := mode.HandleEvent(AgentSessionEvent{
		Type:   "compaction_end",
		Reason: "manual",
		Result: &agentharness.CompactionResult{
			TokensBefore: 123,
			Summary:      "summary",
		},
		Aborted:   false,
		WillRetry: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	if chat.clearCount != 1 || rebuilds != 1 || len(messages) != 1 {
		t.Fatalf("clear=%d rebuilds=%d messages=%#v", chat.clearCount, rebuilds, messages)
	}
	if messages[0].Role != "compactionSummary" || messages[0].TokensBefore != 123 || messages[0].Summary != "summary" {
		t.Fatalf("message = %#v", messages[0])
	}
	if footer.invalidateCount != 1 || !reflect.DeepEqual(flushes, []InteractiveFlushCompactionOptions{{WillRetry: false}}) {
		t.Fatalf("footer=%d flushes=%#v", footer.invalidateCount, flushes)
	}
	if !reflect.DeepEqual(ui.renderForces, []bool{false}) {
		t.Fatalf("renders = %#v", ui.renderForces)
	}
}

func TestInteractiveModeSuspendMatchesPi(t *testing.T) {
	t.Run("shows a status message and skips suspend on Windows", func(t *testing.T) {
		ui := &fakeInteractiveUI{}
		ops := &fakeSuspendOps{}
		statuses := []string{}
		mode := &InteractiveMode{
			UI:         ui,
			Suspend:    ops.operations("win32", nil),
			ShowStatus: func(message string) { statuses = append(statuses, message) },
		}

		if err := mode.HandleCtrlZ(); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(statuses, []string{"Suspend to background is not supported on Windows"}) {
			t.Fatalf("statuses = %#v", statuses)
		}
		if ui.stopCount != 0 || ops.setIntervalCount != 0 || len(ops.onSignals) != 0 || len(ops.onceSignals) != 0 || len(ops.kills) != 0 {
			t.Fatalf("ui=%#v ops=%#v", ui, ops)
		}
	})

	t.Run("keeps the process alive while suspended and restores the TUI on SIGCONT", func(t *testing.T) {
		ui := &fakeInteractiveUI{}
		ops := &fakeSuspendOps{intervalHandle: "keep-alive"}
		mode := &InteractiveMode{UI: ui, Suspend: ops.operations("darwin", nil)}

		if err := mode.HandleCtrlZ(); err != nil {
			t.Fatal(err)
		}

		if ops.setIntervalCount != 1 || ops.interval != interactiveSuspendKeepAliveInterval {
			t.Fatalf("setInterval count=%d interval=%s", ops.setIntervalCount, ops.interval)
		}
		if !reflect.DeepEqual(ops.onSignals, []string{"SIGINT"}) || !reflect.DeepEqual(ops.onceSignals, []string{"SIGCONT"}) {
			t.Fatalf("signals on=%#v once=%#v", ops.onSignals, ops.onceSignals)
		}
		if ui.stopCount != 1 || !reflect.DeepEqual(ops.kills, []string{"SIGTSTP"}) {
			t.Fatalf("stop=%d kills=%#v", ui.stopCount, ops.kills)
		}
		if ops.sigcontHandler == nil || ops.sigintToken == nil {
			t.Fatalf("sigcont=%v sigintToken=%v", ops.sigcontHandler != nil, ops.sigintToken)
		}

		ops.sigcontHandler()

		if !reflect.DeepEqual(ops.cleared, []any{"keep-alive"}) || !reflect.DeepEqual(ops.removed, []fakeSignalRemoval{{signal: "SIGINT", token: ops.sigintToken}}) {
			t.Fatalf("cleared=%#v removed=%#v", ops.cleared, ops.removed)
		}
		if ui.startCount != 1 || !reflect.DeepEqual(ui.renderForces, []bool{true}) {
			t.Fatalf("start=%d renders=%#v", ui.startCount, ui.renderForces)
		}
	})

	t.Run("cleans up the temporary handlers if suspension fails", func(t *testing.T) {
		ui := &fakeInteractiveUI{}
		suspendErr := errors.New("suspend failed")
		ops := &fakeSuspendOps{intervalHandle: "keep-alive"}
		mode := &InteractiveMode{UI: ui, Suspend: ops.operations("darwin", suspendErr)}

		err := mode.HandleCtrlZ()
		if !errors.Is(err, suspendErr) {
			t.Fatalf("err = %v, want suspend failed", err)
		}
		if ui.stopCount != 1 || ops.setIntervalCount != 1 {
			t.Fatalf("stop=%d setInterval=%d", ui.stopCount, ops.setIntervalCount)
		}
		if !reflect.DeepEqual(ops.cleared, []any{"keep-alive"}) || !reflect.DeepEqual(ops.removed, []fakeSignalRemoval{{signal: "SIGINT", token: ops.sigintToken}}) {
			t.Fatalf("cleared=%#v removed=%#v", ops.cleared, ops.removed)
		}
		if ui.startCount != 0 || len(ui.renderForces) != 0 {
			t.Fatalf("start=%d renders=%#v", ui.startCount, ui.renderForces)
		}
	})
}

type fakeInteractiveLeafProvider struct {
	leaf *string
}

func (p fakeInteractiveLeafProvider) GetLeafID() *string {
	return p.leaf
}

type fakeInteractiveRuntimeHost struct {
	forkCalled  bool
	forkEntryID string
	forkOptions InteractiveForkOptions
	forkResult  InteractiveForkResult
	forkErr     error
	importCalls []fakeImportCall
	importErr   error
}

func (h *fakeInteractiveRuntimeHost) Fork(entryID string, options InteractiveForkOptions) (InteractiveForkResult, error) {
	h.forkCalled = true
	h.forkEntryID = entryID
	h.forkOptions = options
	return h.forkResult, h.forkErr
}

func (h *fakeInteractiveRuntimeHost) ImportFromJsonl(inputPath string, cwdOverride ...string) (InteractiveImportResult, error) {
	h.importCalls = append(h.importCalls, fakeImportCall{path: inputPath, cwdOverride: append([]string(nil), cwdOverride...)})
	return InteractiveImportResult{}, h.importErr
}

type fakeImportCall struct {
	path        string
	cwdOverride []string
}

type fakeInteractiveEditor struct {
	text string
}

func (e *fakeInteractiveEditor) SetText(text string) {
	e.text = text
}

type fakeInteractiveUI struct {
	startCount   int
	stopCount    int
	renderForces []bool
	terminal     fakeInteractiveTerminal
}

func (u *fakeInteractiveUI) Start() {
	u.startCount++
}

func (u *fakeInteractiveUI) Stop() {
	u.stopCount++
}

func (u *fakeInteractiveUI) RequestRender(force ...bool) {
	value := false
	if len(force) > 0 {
		value = force[0]
	}
	u.renderForces = append(u.renderForces, value)
}

func (u *fakeInteractiveUI) Terminal() InteractiveTerminal {
	return &u.terminal
}

type fakeInteractiveTerminal struct {
	progress []bool
}

func (t *fakeInteractiveTerminal) SetProgress(active bool) {
	t.progress = append(t.progress, active)
}

type fakeInteractiveContainer struct {
	clearCount int
	text       []string
}

func (c *fakeInteractiveContainer) Clear() {
	c.clearCount++
}

func (c *fakeInteractiveContainer) AddText(text string) {
	c.text = append(c.text, text)
}

type fakeInteractiveFooter struct {
	invalidateCount int
}

func (f *fakeInteractiveFooter) Invalidate() {
	f.invalidateCount++
}

type fakeInteractiveSettings struct {
	showTerminalProgress bool
}

func (s fakeInteractiveSettings) GetShowTerminalProgress() bool {
	return s.showTerminalProgress
}

type fakeSuspendOps struct {
	intervalHandle   any
	setIntervalCount int
	interval         time.Duration
	onSignals        []string
	onceSignals      []string
	kills            []string
	cleared          []any
	removed          []fakeSignalRemoval
	sigintToken      any
	sigcontHandler   func()
}

type fakeSignalRemoval struct {
	signal string
	token  any
}

func (o *fakeSuspendOps) operations(platform string, killErr error) InteractiveSuspendOperations {
	if o.intervalHandle == nil {
		o.intervalHandle = "interval"
	}
	return InteractiveSuspendOperations{
		Platform: platform,
		SetInterval: func(_ func(), interval time.Duration) any {
			o.setIntervalCount++
			o.interval = interval
			return o.intervalHandle
		},
		ClearInterval: func(handle any) {
			o.cleared = append(o.cleared, handle)
		},
		OnSignal: func(signal string, handler func()) any {
			o.onSignals = append(o.onSignals, signal)
			token := &struct {
				signal  string
				handler func()
			}{signal: signal, handler: handler}
			if signal == "SIGINT" {
				o.sigintToken = token
			}
			return token
		},
		OnceSignal: func(signal string, handler func()) any {
			o.onceSignals = append(o.onceSignals, signal)
			if signal == "SIGCONT" {
				o.sigcontHandler = handler
			}
			return handler
		},
		RemoveSignalListener: func(signal string, subscription any) {
			o.removed = append(o.removed, fakeSignalRemoval{signal: signal, token: subscription})
		},
		KillProcessGroup: func(signal string) error {
			o.kills = append(o.kills, signal)
			return killErr
		},
	}
}

func interactiveStringPtr(value string) *string {
	return &value
}
