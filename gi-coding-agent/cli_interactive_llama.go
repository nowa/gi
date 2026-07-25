package gicodingagent

import (
	"context"
	"errors"
	"strings"

	gitui "github.com/nowa/gi/gi-tui"
)

type llamaManagerProvider interface {
	LlamaManager() *builtinLlamaManager
}

func (h *CLIInteractiveTUIHost) llamaManager() *builtinLlamaManager {
	if h == nil || h.runtimeHost == nil {
		return nil
	}
	provider, ok := h.runtimeHost.(llamaManagerProvider)
	if !ok {
		return nil
	}
	return provider.LlamaManager()
}

func (h *CLIInteractiveTUIHost) handleLlamaSlashCommand() error {
	if h == nil || h.ui == nil {
		return errors.New("interactive TUI is not ready")
	}
	manager := h.llamaManager()
	if manager == nil {
		h.addStatus("llama.cpp manager is unavailable")
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopDoneWatch := make(chan struct{})
	go func() {
		select {
		case <-h.done:
			cancel()
		case <-stopDoneWatch:
		}
	}()
	defer close(stopDoneWatch)

	view := newLlamaManagerView(
		func() {
			h.requestRender(false)
		},
		func(message string, level string) {
			message = strings.TrimSpace(message)
			if message == "" {
				return
			}
			switch level {
			case "error":
				message = "Error: " + message
			case "warning":
				message = "Warning: " + message
			}
			h.addStatus(message)
		},
	)
	restore := h.showEditorReplacement(view, view)
	defer func() {
		view.Close()
		restore()
	}()
	err := manager.run(ctx, view)
	switch {
	case err == nil,
		errors.Is(err, context.Canceled):
		return nil
	case errors.Is(err, errBuiltinLlamaNotConfigured):
		h.addStatus(
			"Configure llama.cpp with /login llama.cpp",
		)
		return nil
	default:
		h.addStatus("Error: " + err.Error())
		return nil
	}
}

var _ gitui.Component = (*llamaManagerView)(nil)
var _ gitui.InputHandler = (*llamaManagerView)(nil)
var _ gitui.Focusable = (*llamaManagerView)(nil)
