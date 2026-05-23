package gicodingagent

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
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
	PendingTools                   map[string]*ToolExecutionComponent
	PendingToolChatIndexes         map[string]int

	RenderCurrentSessionState   func()
	RebuildChatFromMessages     func()
	AddMessageToChat            func(InteractiveMessage)
	GetRegisteredToolDefinition func(toolName string) ToolDefinition
	FlushCompactionQueue        func(InteractiveFlushCompactionOptions) error
	ShowStatus                  func(string)
	ShowError                   func(string)
	ShowExtensionConfirm        func(title, message string) (bool, error)
	PromptForMissingSessionCwd  func(error) (string, error)
	HandleFatalRuntimeError     func(prefix string, err error) error
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

type interactiveSessionCWDProvider interface {
	GetCWD() string
}

func (m *InteractiveMode) RenderSessionContext(context SessionContext) {
	if m == nil {
		return
	}
	if m.Chat != nil {
		m.Chat.Clear()
	}
	renderedPendingTools := map[string]*ToolExecutionComponent{}
	renderedPendingToolIndexes := map[string]int{}
	for _, rawMessage := range context.Messages {
		message, ok := sessionMessageToLLM(rawMessage)
		if !ok {
			continue
		}
		switch message.Role {
		case llm.RoleAssistant:
			m.renderAssistantSessionMessage(message, renderedPendingTools, renderedPendingToolIndexes)
		case llm.RoleToolResult:
			m.renderToolResultSessionMessage(message, renderedPendingTools, renderedPendingToolIndexes)
		default:
			m.addLLMMessageToChat(message)
		}
	}
	m.PendingTools = renderedPendingTools
	m.PendingToolChatIndexes = renderedPendingToolIndexes
	if m.Footer != nil {
		m.Footer.Invalidate()
	}
}

func (m *InteractiveMode) renderAssistantSessionMessage(message llm.Message, pending map[string]*ToolExecutionComponent, indexes map[string]int) {
	text := interactiveTextFromLLMMessage(message)
	if text != "" {
		m.addTextToChat(text)
	}
	for _, part := range message.Content {
		if part.Type != llm.ContentToolCall {
			continue
		}
		component := m.newToolExecutionComponent(part.Name, part.ID, part.Arguments)
		component.SetArgsComplete()
		pending[part.ID] = component
		if index := m.addToolExecutionToChat(component); index >= 0 {
			indexes[part.ID] = index
		}
	}
}

func (m *InteractiveMode) renderToolResultSessionMessage(message llm.Message, pending map[string]*ToolExecutionComponent, indexes map[string]int) {
	result := fileToolResultFromLLMMessage(message)
	if component := pending[message.ToolCallID]; component != nil {
		component.UpdateResult(result, message.IsError)
		m.updateOrAddToolExecution(message.ToolCallID, component, indexes)
		delete(pending, message.ToolCallID)
		delete(indexes, message.ToolCallID)
		return
	}
	m.addTextToChat(interactiveTextFromLLMMessage(message))
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

func resolveInteractiveSuspendOperations(ops InteractiveSuspendOperations) InteractiveSuspendOperations {
	if hasInteractiveSuspendOperations(ops) {
		return ops
	}
	return defaultInteractiveSuspendOperations()
}

func hasInteractiveSuspendOperations(ops InteractiveSuspendOperations) bool {
	return ops.Platform != "" ||
		ops.SetInterval != nil ||
		ops.ClearInterval != nil ||
		ops.OnSignal != nil ||
		ops.OnceSignal != nil ||
		ops.RemoveSignalListener != nil ||
		ops.KillProcessGroup != nil
}

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
	switch event.Type {
	case "tool_execution_start":
		m.handleToolExecutionStart(event)
		return nil
	case "tool_execution_end":
		m.handleToolExecutionEnd(event)
		return nil
	}
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

func (m *InteractiveMode) handleToolExecutionStart(event AgentSessionEvent) {
	if m == nil || event.ToolCallID == "" {
		return
	}
	m.ensurePendingTools()
	component := m.newToolExecutionComponent(event.ToolName, event.ToolCallID, event.Args)
	component.SetArgsComplete()
	m.PendingTools[event.ToolCallID] = component
	if index := m.addToolExecutionToChat(component); index >= 0 {
		m.PendingToolChatIndexes[event.ToolCallID] = index
	}
	if m.UI != nil {
		m.UI.RequestRender()
	}
}

func (m *InteractiveMode) handleToolExecutionEnd(event AgentSessionEvent) {
	if m == nil || event.ToolCallID == "" {
		return
	}
	m.ensurePendingTools()
	component := m.PendingTools[event.ToolCallID]
	if component == nil {
		component = m.newToolExecutionComponent(event.ToolName, event.ToolCallID, event.Args)
		component.SetArgsComplete()
	}
	result := FileToolResult{}
	isError := false
	if event.ToolResult != nil {
		result = fileToolResultFromLLMMessage(*event.ToolResult)
		isError = event.ToolResult.IsError
	}
	component.UpdateResult(result, isError)
	m.updateOrAddToolExecution(event.ToolCallID, component, m.PendingToolChatIndexes)
	delete(m.PendingTools, event.ToolCallID)
	delete(m.PendingToolChatIndexes, event.ToolCallID)
	if m.Footer != nil {
		m.Footer.Invalidate()
	}
	if m.UI != nil {
		m.UI.RequestRender()
	}
}

func (m *InteractiveMode) ensurePendingTools() {
	if m.PendingTools == nil {
		m.PendingTools = map[string]*ToolExecutionComponent{}
	}
	if m.PendingToolChatIndexes == nil {
		m.PendingToolChatIndexes = map[string]int{}
	}
}

func (m *InteractiveMode) newToolExecutionComponent(name, callID string, args any) *ToolExecutionComponent {
	definition := ToolDefinition{}
	if m != nil && m.GetRegisteredToolDefinition != nil {
		definition = m.GetRegisteredToolDefinition(name)
	}
	options := ToolExecutionOptions{}
	if m != nil {
		if settings, ok := m.Settings.(interface {
			GetShowImages() bool
			GetImageWidthCells() int
		}); ok {
			showImages := settings.GetShowImages()
			options.ShowImages = &showImages
			options.ImageWidthCells = settings.GetImageWidthCells()
		}
	}
	return NewToolExecutionComponent(name, callID, args, definition, m.interactiveCWD(), options)
}

func (m *InteractiveMode) interactiveCWD() string {
	if m != nil {
		if provider, ok := m.SessionManager.(interactiveSessionCWDProvider); ok {
			return provider.GetCWD()
		}
	}
	return ""
}

func (m *InteractiveMode) addToolExecutionToChat(component *ToolExecutionComponent) int {
	if component == nil {
		return -1
	}
	index := -1
	if indexed, ok := m.Chat.(InteractiveIndexedTextContainer); ok {
		index = indexed.Len()
	}
	m.addTextToChat(strings.Join(component.Render(120), "\n"))
	return index
}

func (m *InteractiveMode) updateOrAddToolExecution(callID string, component *ToolExecutionComponent, indexes map[string]int) {
	if m == nil || component == nil {
		return
	}
	if indexed, ok := m.Chat.(InteractiveIndexedTextContainer); ok {
		if index, exists := indexes[callID]; exists {
			indexed.SetTextAt(index, strings.Join(component.Render(120), "\n"))
			return
		}
	}
	m.addToolExecutionToChat(component)
}

func (m *InteractiveMode) addLLMMessageToChat(message llm.Message) {
	m.addTextToChat(interactiveTextFromLLMMessage(message))
}

func (m *InteractiveMode) addTextToChat(text string) {
	if m == nil || m.Chat == nil || text == "" {
		return
	}
	m.Chat.AddText(text)
}

func interactiveTextFromLLMMessage(message llm.Message) string {
	parts := make([]string, 0, len(message.Content))
	for _, part := range message.Content {
		if part.Type == llm.ContentText && part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "")
}

func fileToolResultFromLLMMessage(message llm.Message) FileToolResult {
	text := interactiveTextFromLLMMessage(message)
	return FileToolResult{
		Text:    text,
		Content: append([]llm.ContentPart(nil), message.Content...),
	}
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
	if ops.Platform == "win32" || ops.Platform == "windows" {
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
