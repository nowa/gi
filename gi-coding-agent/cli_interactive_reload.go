package gicodingagent

import (
	"context"
	"strings"

	gitui "github.com/nowa/gi/gi-tui"
)

type cliReloadBoxComponent struct {
	message string
}

func (c *cliReloadBoxComponent) Invalidate() {}

func (c *cliReloadBoxComponent) Render(width int) []string {
	width = max(20, width)
	border := tuiThemeBorder(strings.Repeat("─", width))
	message := tuiThemeMuted(" " + firstNonEmptyString(strings.TrimSpace(c.message), "Reloading..."))
	return []string{
		border,
		"",
		gitui.TruncateToWidth(message, width, "", true),
		"",
		border,
	}
}

func (h *CLIInteractiveTUIHost) showReloadingEditor() func() {
	if h == nil || h.editorContainer == nil || h.ui == nil {
		return func() {}
	}
	previousChildren := h.editorContainer.Children()
	previousFocus := h.ui.FocusedComponent()
	reloadBox := &cliReloadBoxComponent{message: "Reloading keybindings, extensions, skills, prompts, themes..."}
	h.editorContainer.SetChildren([]gitui.Component{reloadBox})
	h.ui.SetFocus(reloadBox)
	h.requestRender(true)
	return func() {
		if len(previousChildren) == 0 && h.editor != nil {
			previousChildren = []gitui.Component{h.editor}
		}
		h.editorContainer.SetChildren(previousChildren)
		if previousFocus != nil {
			h.ui.SetFocus(previousFocus)
		} else if h.editor != nil {
			h.ui.SetFocus(h.editor)
		}
		h.requestRender(false)
	}
}

func (h *CLIInteractiveTUIHost) handleReloadSlashCommand() error {
	session, err := h.currentAgentSession()
	if err != nil {
		return err
	}
	if session.IsStreaming() {
		h.addStatus("Warning: Wait for the current response to finish before reloading.")
		return nil
	}
	if session.IsCompacting() {
		h.addStatus("Warning: Wait for compaction to finish before reloading.")
		return nil
	}
	restoreReloadBox := h.showReloadingEditor()
	reloadCompleted := false
	defer func() {
		if !reloadCompleted {
			restoreReloadBox()
		}
	}()
	h.stopProtocolExtensionProcesses()
	var extensions ResourceExtensionsResult
	if loader, ok := session.ResourceLoader.(interface{ Reload() }); ok {
		loader.Reload()
	}
	if loader, ok := session.ResourceLoader.(agentSessionExtensionsResourceLoader); ok {
		extensions = loader.GetExtensions()
		if flagLoader, ok := session.ResourceLoader.(agentSessionExtensionFlagResourceLoader); ok {
			if provider, ok := h.runtimeHost.(interface{ ExtensionFlagValues() map[string]any }); ok {
				flagLoader.ApplyExtensionFlagValues(provider.ExtensionFlagValues(), len(extensions.ProcessExtensions) > 0)
				extensions = loader.GetExtensions()
			}
		}
		if extensions.Runtime != nil {
			if runtime := h.modelRuntime(); runtime != nil {
				extensions.Runtime.BindModelRuntime(runtime)
			} else if registry := h.modelRegistry(); registry != nil {
				extensions.Runtime.BindModelRegistry(registry)
			}
			h.bindProtocolRuntimeHosts(extensions.Runtime)
		}
		if host, ok := h.runtimeHost.(interface {
			SetProtocolExtensionProcessSpecs([]ProtocolPackageProcessExtension)
		}); ok {
			host.SetProtocolExtensionProcessSpecs(extensions.ProcessExtensions)
		}
	}
	if runtimeHost := h.agentSessionRuntimeHost(); runtimeHost != nil {
		if extensions.Runtime != nil {
			runtimeHost.SetExtensionRuntime(extensions.Runtime)
		}
		if err := runtimeHost.Reload(); err != nil {
			return err
		}
	} else if extensions.Runtime != nil {
		session.ExtensionRuntime = extensions.Runtime
		extensions.Runtime.BindSession(session)
		h.bindProtocolRuntimeHosts(extensions.Runtime)
		extensions.Runtime.ApplyToSession(session)
	} else if session.ExtensionRuntime != nil {
		session.ExtensionRuntime.BindSession(session)
		h.bindProtocolRuntimeHosts(session.ExtensionRuntime)
		session.ExtensionRuntime.ApplyToSession(session)
	}
	h.reloadKeybindings()
	h.applyReloadedInteractiveSettings()
	h.refreshEditorAutocompleteProvider()
	if err := h.startProtocolExtensionProcesses(context.Background(), "reload"); err != nil {
		return err
	}
	restoreReloadBox()
	reloadCompleted = true
	h.refreshViewTreeSlots()
	h.rerenderSessionMessages()
	h.showLoadedResourcesOnStartup()
	h.showModelRegistryErrorIfAny()
	h.addStatus("Reloaded keybindings, extensions, skills, prompts, themes")
	return nil
}

func (h *CLIInteractiveTUIHost) bindProtocolViewTreeHost() {
	if h == nil || h.viewTreeHost == nil {
		return
	}
	if provider, ok := h.runtimeHost.(protocolExtensionRuntimeProvider); ok {
		if runtime := provider.ProtocolExtensionRuntime(); runtime != nil {
			h.bindProtocolRuntimeHosts(runtime)
		}
	}
	if runtimeHost := h.agentSessionRuntimeHost(); runtimeHost != nil && runtimeHost.ExtensionRuntime != nil {
		h.bindProtocolRuntimeHosts(runtimeHost.ExtensionRuntime)
	}
}

func (h *CLIInteractiveTUIHost) bindProtocolRuntimeHosts(runtime *ProtocolExtensionRuntime) {
	if h == nil || runtime == nil {
		return
	}
	runtime.BindViewTreeHost(h.viewTreeHost)
	h.watchProtocolRuntimeErrors(runtime)
	if provider, ok := h.runtimeHost.(protocolExtensionProcessProvider); ok {
		runtime.BindHostActionHost(h.configureProtocolExtensionRPCSessionHost(provider.NewProtocolExtensionRPCSessionHost(h.viewTreeHost, h, h)))
	}
}

func (h *CLIInteractiveTUIHost) watchProtocolRuntimeErrors(runtime *ProtocolExtensionRuntime) {
	if h == nil || runtime == nil {
		return
	}
	if h.unwatchProtocolErrors == nil {
		h.unwatchProtocolErrors = map[*ProtocolExtensionRuntime]func(){}
	}
	if h.unwatchProtocolErrors[runtime] != nil {
		return
	}
	h.unwatchProtocolErrors[runtime] = runtime.OnError(func(event ProtocolExtensionError) {
		h.showExtensionError(event)
	})
}

func (h *CLIInteractiveTUIHost) configureProtocolExtensionRPCSessionHost(rpcHost *RPCSessionHost) *RPCSessionHost {
	if h == nil || rpcHost == nil {
		return rpcHost
	}
	rpcHost.TUITitle = h
	rpcHost.TUIWorking = h
	rpcHost.TUIThinkingLabel = h
	rpcHost.TUIStatus = h
	rpcHost.TUITheme = h
	rpcHost.TUIToolExpansion = h
	if rpcHost.ProcessExecutor == nil {
		rpcHost.ProcessExecutor = LocalHostProcessExecutor{}
	}
	return rpcHost
}

func (h *CLIInteractiveTUIHost) applyReloadedInteractiveSettings() {
	if h == nil {
		return
	}
	settings := h.settingsManager()
	if settings == nil {
		return
	}
	if h.editor != nil {
		h.editor.SetPaddingX(settings.GetEditorPaddingX())
		h.editor.SetAutocompleteMaxVisible(settings.GetAutocompleteMaxVisible())
	}
	if active, ok := h.activeEditorComponent(); ok && active != h.editor {
		if appearance, ok := active.(gitui.EditorAppearanceComponent); ok {
			appearance.SetPaddingX(settings.GetEditorPaddingX())
			appearance.SetAutocompleteMaxVisible(settings.GetAutocompleteMaxVisible())
		}
	}
	if h.ui != nil {
		h.ui.SetShowHardwareCursor(settings.GetShowHardwareCursor())
		h.ui.SetClearOnShrink(settings.GetClearOnShrink())
	}
	h.applyCurrentTUITheme(context.Background())
}
