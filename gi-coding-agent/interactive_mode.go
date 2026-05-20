package gicodingagent

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
)

type InteractiveMode struct {
	SessionManager InteractiveSessionLeafProvider
	RuntimeHost    InteractiveRuntimeHost
	UI             InteractiveUI
	Editor         InteractiveEditor
	Status         InteractiveContainer
	Chat           InteractiveContainer
	Footer         InteractiveFooter
	Settings       InteractiveSettings

	LoadingAnimation               InteractiveStopper
	AutoCompactionLoader           InteractiveStopper
	DefaultEditor                  *InteractiveDefaultEditor
	AutoCompactionEscapeHandler    func()
	Suspend                        InteractiveSuspendOperations
	lastStatusIndex                int
	lastStatusValid                bool
	ToolOutputExpanded             bool
	CustomHeader                   InteractiveExpandable
	BuiltInHeader                  InteractiveExpandable
	ChatExpandables                []InteractiveExpandable
	ThemeSettings                  InteractiveThemeSettings
	AutocompleteProviderWrappers   []AutocompleteProviderFactory
	CreateBaseAutocompleteProvider func() AutocompleteProvider
	DefaultAutocompleteEditor      AutocompleteEditor
	AutocompleteEditor             AutocompleteEditor
	LoadedResources                InteractiveLoadedResources

	RenderCurrentSessionState  func()
	RebuildChatFromMessages    func()
	AddMessageToChat           func(InteractiveMessage)
	FlushCompactionQueue       func(InteractiveFlushCompactionOptions) error
	ShowStatus                 func(string)
	ShowError                  func(string)
	ShowExtensionConfirm       func(title, message string) (bool, error)
	PromptForMissingSessionCwd func(error) (string, error)
	HandleFatalRuntimeError    func(prefix string, err error) error
}

type InteractiveSessionLeafProvider interface {
	GetLeafID() *string
}

type InteractiveRuntimeHost interface {
	Fork(entryID string, options InteractiveForkOptions) (InteractiveForkResult, error)
	ImportFromJsonl(inputPath string, cwdOverride ...string) (InteractiveImportResult, error)
}

type InteractiveForkOptions struct {
	Position string
}

type InteractiveForkResult struct {
	Cancelled bool
}

type InteractiveImportResult struct {
	Cancelled bool
}

type InteractiveUI interface {
	Start()
	Stop()
	RequestRender(force ...bool)
	Terminal() InteractiveTerminal
}

type InteractiveTerminal interface {
	SetProgress(active bool)
}

type InteractiveEditor interface {
	SetText(text string)
}

type InteractiveContainer interface {
	Clear()
	AddText(text string)
}

type InteractiveFooter interface {
	Invalidate()
}

type InteractiveSettings interface {
	GetShowTerminalProgress() bool
}

type InteractiveStopper interface {
	Stop()
}

type InteractiveDefaultEditor struct {
	OnEscape func()
}

type InteractiveMessage struct {
	Role         string
	Summary      string
	TokensBefore int
	Timestamp    string
}

type InteractiveFlushCompactionOptions struct {
	WillRetry bool
}

type SessionImportFileNotFoundError struct {
	Path string
}

func (e SessionImportFileNotFoundError) Error() string {
	return "File not found: " + e.Path
}

type InteractiveSuspendOperations struct {
	Platform             string
	SetInterval          func(func(), time.Duration) any
	ClearInterval        func(any)
	OnSignal             func(signal string, handler func()) any
	OnceSignal           func(signal string, handler func()) any
	RemoveSignalListener func(signal string, subscription any)
	KillProcessGroup     func(signal string) error
}

const interactiveSuspendKeepAliveInterval = time.Duration(1<<30) * time.Millisecond

func (m *InteractiveMode) HandleCloneCommand() error {
	leafID := ""
	if m != nil && m.SessionManager != nil {
		if leaf := m.SessionManager.GetLeafID(); leaf != nil {
			leafID = *leaf
		}
	}
	if leafID == "" {
		m.showStatus("Nothing to clone yet")
		return nil
	}
	if m.RuntimeHost == nil {
		return errors.New("runtime host is required")
	}
	result, err := m.RuntimeHost.Fork(leafID, InteractiveForkOptions{Position: "at"})
	if err != nil {
		m.showError(err.Error())
		return nil
	}
	if result.Cancelled {
		if m.UI != nil {
			m.UI.RequestRender()
		}
		return nil
	}
	if m.RenderCurrentSessionState != nil {
		m.RenderCurrentSessionState()
	}
	if m.Editor != nil {
		m.Editor.SetText("")
	}
	m.showStatus("Cloned to new session")
	return nil
}

func GetPathCommandArgument(text, command string) (string, bool) {
	if text == command {
		return "", false
	}
	if !strings.HasPrefix(text, command+" ") {
		return "", false
	}
	argsString := strings.TrimLeft(text[len(command)+1:], " \t\r\n")
	if argsString == "" {
		return "", false
	}
	firstChar := argsString[0]
	if firstChar == '"' || firstChar == '\'' {
		closingQuoteIndex := strings.IndexByte(argsString[1:], firstChar)
		if closingQuoteIndex < 0 {
			return "", false
		}
		return argsString[1 : closingQuoteIndex+1], true
	}
	firstWhitespaceIndex := strings.IndexFunc(argsString, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\r' || r == '\n'
	})
	if firstWhitespaceIndex < 0 {
		return argsString, true
	}
	return argsString[:firstWhitespaceIndex], true
}

func (m *InteractiveMode) GetPathCommandArgument(text, command string) (string, bool) {
	return GetPathCommandArgument(text, command)
}

func (m *InteractiveMode) HandleImportCommand(text string) error {
	inputPath, ok := m.GetPathCommandArgument(text, "/import")
	if !ok {
		m.showError("Usage: /import <path.jsonl>")
		return nil
	}
	if m.ShowExtensionConfirm != nil {
		confirmed, err := m.ShowExtensionConfirm("Import session", "Replace current session with "+inputPath+"?")
		if err != nil {
			return err
		}
		if !confirmed {
			m.showStatus("Import cancelled")
			return nil
		}
	}
	return m.importFromJsonl(inputPath)
}

func (m *InteractiveMode) importFromJsonl(inputPath string, cwdOverride ...string) error {
	if m.RuntimeHost == nil {
		return errors.New("runtime host is required")
	}
	if m.LoadingAnimation != nil {
		m.LoadingAnimation.Stop()
		m.LoadingAnimation = nil
	}
	if m.Status != nil {
		m.Status.Clear()
	}
	result, err := m.RuntimeHost.ImportFromJsonl(inputPath, cwdOverride...)
	if err == nil {
		if result.Cancelled {
			m.showStatus("Import cancelled")
			return nil
		}
		if m.RenderCurrentSessionState != nil {
			m.RenderCurrentSessionState()
		}
		m.showStatus("Session imported from: " + inputPath)
		return nil
	}
	var missingCwd MissingSessionCwdError
	if errors.As(err, &missingCwd) {
		if m.PromptForMissingSessionCwd == nil {
			return err
		}
		selectedCwd, promptErr := m.PromptForMissingSessionCwd(err)
		if promptErr != nil {
			return promptErr
		}
		if strings.TrimSpace(selectedCwd) == "" {
			m.showStatus("Import cancelled")
			return nil
		}
		return m.importFromJsonl(inputPath, selectedCwd)
	}
	var notFound SessionImportFileNotFoundError
	if errors.As(err, &notFound) {
		m.showError("Failed to import session: " + notFound.Error())
		return nil
	}
	if m.HandleFatalRuntimeError != nil {
		return m.HandleFatalRuntimeError("Failed to import session", err)
	}
	return fmt.Errorf("failed to import session: %w", err)
}

func (m *InteractiveMode) HandleEvent(event AgentSessionEvent) error {
	if event.Type != "compaction_end" {
		return nil
	}
	if m.Settings != nil && m.Settings.GetShowTerminalProgress() && m.UI != nil && m.UI.Terminal() != nil {
		m.UI.Terminal().SetProgress(false)
	}
	if m.AutoCompactionEscapeHandler != nil && m.DefaultEditor != nil {
		m.DefaultEditor.OnEscape = m.AutoCompactionEscapeHandler
		m.AutoCompactionEscapeHandler = nil
	}
	if m.AutoCompactionLoader != nil {
		m.AutoCompactionLoader.Stop()
		m.AutoCompactionLoader = nil
		if m.Status != nil {
			m.Status.Clear()
		}
	}
	if event.Aborted {
		if event.Reason == "manual" {
			m.showError("Compaction cancelled")
		} else {
			m.showStatus("Auto-compaction cancelled")
		}
	} else if event.Result != nil {
		if m.Chat != nil {
			m.Chat.Clear()
		}
		if m.RebuildChatFromMessages != nil {
			m.RebuildChatFromMessages()
		}
		if m.AddMessageToChat != nil {
			m.AddMessageToChat(createInteractiveCompactionSummaryMessage(*event.Result))
		}
		if m.Footer != nil {
			m.Footer.Invalidate()
		}
	} else if event.ErrorMessage != "" {
		if event.Reason == "manual" {
			m.showError(event.ErrorMessage)
		} else if m.Chat != nil {
			m.Chat.AddText(event.ErrorMessage)
		}
	}
	if m.FlushCompactionQueue != nil {
		if err := m.FlushCompactionQueue(InteractiveFlushCompactionOptions{WillRetry: event.WillRetry}); err != nil {
			return err
		}
	}
	if m.UI != nil {
		m.UI.RequestRender()
	}
	return nil
}

func createInteractiveCompactionSummaryMessage(result agentharness.CompactionResult) InteractiveMessage {
	return InteractiveMessage{
		Role:         "compactionSummary",
		Summary:      result.Summary,
		TokensBefore: result.TokensBefore,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}
}

func (m *InteractiveMode) HandleCtrlZ() error {
	ops := m.Suspend.withDefaults()
	if ops.Platform == "win32" {
		m.showStatus("Suspend to background is not supported on Windows")
		return nil
	}
	suspendKeepAlive := ops.SetInterval(func() {}, interactiveSuspendKeepAliveInterval)
	ignoreSigint := func() {}
	sigintSubscription := ops.OnSignal("SIGINT", ignoreSigint)
	ops.OnceSignal("SIGCONT", func() {
		ops.ClearInterval(suspendKeepAlive)
		ops.RemoveSignalListener("SIGINT", sigintSubscription)
		if m.UI != nil {
			m.UI.Start()
			m.UI.RequestRender(true)
		}
	})
	if m.UI != nil {
		m.UI.Stop()
	}
	if err := ops.KillProcessGroup("SIGTSTP"); err != nil {
		ops.ClearInterval(suspendKeepAlive)
		ops.RemoveSignalListener("SIGINT", sigintSubscription)
		return err
	}
	return nil
}

func (ops InteractiveSuspendOperations) withDefaults() InteractiveSuspendOperations {
	if ops.Platform == "" {
		ops.Platform = runtime.GOOS
	}
	if ops.SetInterval == nil {
		ops.SetInterval = func(func(), time.Duration) any { return struct{}{} }
	}
	if ops.ClearInterval == nil {
		ops.ClearInterval = func(any) {}
	}
	if ops.OnSignal == nil {
		ops.OnSignal = func(signal string, handler func()) any { return handler }
	}
	if ops.OnceSignal == nil {
		ops.OnceSignal = func(signal string, handler func()) any { return handler }
	}
	if ops.RemoveSignalListener == nil {
		ops.RemoveSignalListener = func(string, any) {}
	}
	if ops.KillProcessGroup == nil {
		ops.KillProcessGroup = func(string) error { return nil }
	}
	return ops
}

func (m *InteractiveMode) showStatus(message string) {
	if m != nil && m.ShowStatus != nil {
		m.ShowStatus(message)
	}
}

func (m *InteractiveMode) showError(message string) {
	if m != nil && m.ShowError != nil {
		m.ShowError(message)
	}
}
