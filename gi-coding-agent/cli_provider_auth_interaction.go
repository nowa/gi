package gicodingagent

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	llm "github.com/nowa/gi/gi-llm-provider"
)

// cliProviderAuthInteraction is the application boundary for provider-owned
// authentication. Providers own protocol and token state; this adapter owns
// only terminal presentation and user cancellation.
type cliProviderAuthInteraction struct {
	host         *CLIInteractiveTUIHost
	providerName string
	cancel       context.CancelFunc
	dialog       *LoginDialogComponent

	openOnce   sync.Once
	lifecycle  cliEditorReplacementLifecycle
	cancelled  atomic.Bool
	mu         sync.Mutex
	prompt     chan string
	openedURLs map[string]struct{}
}

func newCLIProviderAuthInteraction(
	host *CLIInteractiveTUIHost,
	providerName string,
	cancel context.CancelFunc,
) *cliProviderAuthInteraction {
	interaction := &cliProviderAuthInteraction{
		host:         host,
		providerName: providerName,
		cancel:       cancel,
		openedURLs:   map[string]struct{}{},
	}
	dialog := NewLoginDialogComponent("Login to "+providerName, "")
	dialog.OnSubmit = interaction.submit
	dialog.OnCancel = interaction.cancelLogin
	interaction.dialog = dialog
	return interaction
}

func (i *cliProviderAuthInteraction) Prompt(
	ctx context.Context,
	prompt llm.AuthPrompt,
) (string, error) {
	if i == nil || i.host == nil || i.dialog == nil {
		return "", errors.New("interactive authentication is not ready")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", errors.New("Login cancelled")
	}
	i.ensureOpen()
	switch prompt.Type {
	case llm.AuthPromptSelect:
		return i.promptSelect(ctx, prompt)
	case llm.AuthPromptManualCode:
		return i.promptInput(ctx, func() {
			i.dialog.ShowManualInput(prompt.Message)
		})
	case llm.AuthPromptText, llm.AuthPromptSecret:
		return i.promptInput(ctx, func() {
			i.dialog.ShowPrompt(prompt.Message, prompt.Placeholder)
		})
	default:
		return "", errors.New(
			"unsupported authentication prompt: " + string(prompt.Type),
		)
	}
}

func (i *cliProviderAuthInteraction) Notify(event llm.AuthEvent) {
	if i == nil || i.host == nil || i.dialog == nil {
		return
	}
	i.ensureOpen()
	switch event.Type {
	case llm.AuthEventURL:
		i.dialog.ShowAuth(event.URL, event.Instructions, "")
		i.openBrowser(event.URL)
	case llm.AuthEventDeviceCode:
		i.dialog.ShowDeviceCode(
			event.VerificationURI,
			event.UserCode,
		)
		i.dialog.ShowWaiting("Waiting for authentication...")
		i.openBrowser(event.VerificationURI)
	case llm.AuthEventInfo:
		i.dialog.ShowInfo(formatProviderAuthInfo(event))
	default:
		i.dialog.ShowProgress(event.Message)
	}
	i.host.requestRender(false)
}

func (i *cliProviderAuthInteraction) Close() {
	if i == nil {
		return
	}
	i.lifecycle.close()
}

func (i *cliProviderAuthInteraction) ShowDetails(lines []string) {
	if i == nil || i.host == nil || i.dialog == nil {
		return
	}
	i.ensureOpen()
	i.dialog.ShowDetails(lines)
	i.host.requestRender(false)
}

func (i *cliProviderAuthInteraction) Cancelled() bool {
	return i != nil && i.cancelled.Load()
}

func (i *cliProviderAuthInteraction) ensureOpen() {
	if i == nil || i.host == nil || i.dialog == nil {
		return
	}
	i.openOnce.Do(func() {
		i.lifecycle.install(
			i.host.showEditorReplacement(i.dialog, i.dialog),
		)
	})
}

func (i *cliProviderAuthInteraction) submit(value string) {
	i.mu.Lock()
	result := i.prompt
	i.mu.Unlock()
	if result == nil {
		return
	}
	select {
	case result <- value:
	default:
	}
}

func (i *cliProviderAuthInteraction) cancelLogin() {
	if i == nil {
		return
	}
	i.cancelled.Store(true)
	if i.cancel != nil {
		i.cancel()
	}
}

func (i *cliProviderAuthInteraction) promptInput(
	ctx context.Context,
	show func(),
) (string, error) {
	result := make(chan string, 1)
	i.mu.Lock()
	if i.prompt != nil {
		i.mu.Unlock()
		return "", errors.New("authentication prompt is already active")
	}
	i.prompt = result
	i.mu.Unlock()
	if show != nil {
		show()
	}
	i.host.requestRender(false)
	defer func() {
		i.mu.Lock()
		if i.prompt == result {
			i.prompt = nil
		}
		i.mu.Unlock()
	}()

	select {
	case value := <-result:
		return value, nil
	case <-ctx.Done():
		return "", errors.New("Login cancelled")
	case <-i.host.done:
		i.cancelLogin()
		return "", errors.New("Login cancelled")
	}
}

func (i *cliProviderAuthInteraction) promptSelect(
	ctx context.Context,
	prompt llm.AuthPrompt,
) (string, error) {
	if len(prompt.Options) == 0 {
		return "", errors.New("authentication selection has no options")
	}
	labels := make([]string, len(prompt.Options))
	for index, option := range prompt.Options {
		labels[index] = firstNonEmptyString(option.Label, option.ID)
	}
	selector := NewExtensionSelectorComponent(prompt.Message, labels)
	completion := newCLIDialogCompletion()
	selector.OnSelect = func(string) {
		index := max(0, min(selector.selected, len(prompt.Options)-1))
		completion.finish(TUIDialogResult{
			Action:   "selected",
			OptionID: prompt.Options[index].ID,
			Value:    prompt.Options[index].ID,
		})
	}
	selector.OnCancel = func() {
		i.cancelLogin()
		completion.finish(TUIDialogResult{Action: "cancelled"})
	}
	completion.installRestore(
		i.host.showEditorReplacement(selector, selector),
	)

	var result TUIDialogResult
	select {
	case result = <-completion.result:
	case <-ctx.Done():
		completion.finish(TUIDialogResult{Action: "cancelled"})
		result = <-completion.result
	case <-i.host.done:
		i.cancelLogin()
		completion.finish(TUIDialogResult{Action: "cancelled"})
		result = <-completion.result
	}
	if result.Action != "selected" ||
		strings.TrimSpace(result.OptionID) == "" {
		return "", errors.New("Login cancelled")
	}
	return result.OptionID, nil
}

func (i *cliProviderAuthInteraction) openBrowser(rawURL string) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || !i.host.shouldAutoOpenOAuthBrowser() {
		return
	}
	i.mu.Lock()
	if _, opened := i.openedURLs[rawURL]; opened {
		i.mu.Unlock()
		return
	}
	i.openedURLs[rawURL] = struct{}{}
	i.mu.Unlock()
	_ = defaultOpenOAuthURL(rawURL)
}

func defaultOpenOAuthURL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", rawURL)
	case "windows":
		command = exec.Command("cmd", "/c", "start", "", rawURL)
	default:
		command = exec.Command("xdg-open", rawURL)
	}
	return command.Start()
}

func formatProviderAuthInfo(event llm.AuthEvent) []string {
	lines := make([]string, 0, 1+len(event.Links))
	if message := strings.TrimSpace(event.Message); message != "" {
		lines = append(lines, tuiThemeFG("text", message))
	}
	for _, link := range event.Links {
		rawURL := strings.TrimSpace(link.URL)
		if rawURL == "" {
			continue
		}
		label := strings.TrimSpace(link.Label)
		if label == "" {
			label = rawURL
		} else {
			label += ": " + rawURL
		}
		lines = append(
			lines,
			tuiThemeAccent(terminalHyperlink(rawURL, label)),
		)
	}
	return lines
}

var _ llm.AuthInteraction = (*cliProviderAuthInteraction)(nil)
