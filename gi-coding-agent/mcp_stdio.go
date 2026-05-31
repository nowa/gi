package gicodingagent

import (
	"bufio"
	"context"
	"io"

	mcpstdio "github.com/nowa/gi/gi-coding-agent/internal/mcpstdio"
)

type mcpStdioOptions = mcpstdio.Options
type mcpRPCEnvelope = mcpstdio.RPCEnvelope
type mcpRPCError = mcpstdio.RPCError
type mcpStderrBuffer = mcpstdio.StderrBuffer

func withDefaultMCPStdioHooks(options mcpStdioOptions) mcpStdioOptions {
	if options.Hooks.ConfigureCommand == nil {
		options.Hooks.ConfigureCommand = configureHostProcessCommand
	}
	if options.Hooks.KillProcess == nil {
		options.Hooks.KillProcess = killHostProcess
	}
	if options.Hooks.ForceKillDelay <= 0 {
		options.Hooks.ForceKillDelay = hostProcessForceKillDelay
	}
	if options.Hooks.ClientVersion == "" {
		options.Hooks.ClientVersion = DefaultCodingAgentVersion
	}
	return options
}

func runMCPStdioListTools(options mcpStdioOptions) (map[string]any, error) {
	return mcpstdio.RunListTools(withDefaultMCPStdioHooks(options))
}

func runMCPStdioCallTool(options mcpStdioOptions, name string, arguments map[string]any) (map[string]any, error) {
	return mcpstdio.RunCallTool(withDefaultMCPStdioHooks(options), name, arguments)
}

func runMCPStdioRequest(options mcpStdioOptions, method string, params map[string]any) (map[string]any, error) {
	return mcpstdio.RunRequest(withDefaultMCPStdioHooks(options), method, params)
}

func mcpWriteRequest(writer io.Writer, id int, method string, params any) error {
	return mcpstdio.WriteRequest(writer, id, method, params)
}

func mcpWriteNotification(writer io.Writer, method string, params any) error {
	return mcpstdio.WriteNotification(writer, method, params)
}

func mcpReadResponse(reader *bufio.Scanner, id int) (map[string]any, []mcpRPCEnvelope, error) {
	return mcpstdio.ReadResponse(reader, id)
}

func mcpAttachNotifications(result map[string]any, notifications []mcpRPCEnvelope) {
	mcpstdio.AttachNotifications(result, notifications)
}

func mcpIDMatches(value any, want int) bool {
	return mcpstdio.IDMatches(value, want)
}

func mcpAttachStderr(err error, stderr *mcpStderrBuffer) error {
	return mcpstdio.AttachStderr(err, stderr)
}

func mcpAttachContextOrStderr(ctx context.Context, err error, stderr *mcpStderrBuffer) error {
	return mcpstdio.AttachContextOrStderr(ctx, err, stderr)
}
