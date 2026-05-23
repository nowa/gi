package gicodingagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type mcpStdioOptions struct {
	Command []string
	Env     map[string]string
	Timeout time.Duration
}

type mcpRPCEnvelope struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpRPCError    `json:"error,omitempty"`
}

type mcpRPCError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type mcpStderrBuffer struct {
	mu      sync.Mutex
	content strings.Builder
}

func (b *mcpStderrBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.content.Write(data)
}

func (b *mcpStderrBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.content.String()
}

func runMCPStdioListTools(options mcpStdioOptions) (map[string]any, error) {
	return runMCPStdioRequest(options, "tools/list", map[string]any{})
}

func runMCPStdioCallTool(options mcpStdioOptions, name string, arguments map[string]any) (map[string]any, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("mcp tool name is required")
	}
	return runMCPStdioRequest(options, "tools/call", map[string]any{"name": name, "arguments": arguments})
}

func runMCPStdioRequest(options mcpStdioOptions, method string, params map[string]any) (map[string]any, error) {
	if len(options.Command) == 0 || strings.TrimSpace(options.Command[0]) == "" {
		return nil, errors.New("mcp command is required")
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, options.Command[0], options.Command[1:]...)
	configureHostProcessCommand(cmd)
	cmd.Cancel = func() error {
		return killHostProcess(cmd.Process)
	}
	cmd.WaitDelay = hostProcessForceKillDelay
	if len(options.Env) > 0 {
		env := os.Environ()
		for key, value := range options.Env {
			if strings.TrimSpace(key) != "" {
				env = append(env, key+"="+value)
			}
		}
		cmd.Env = env
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	stderrBuffer := &mcpStderrBuffer{}
	go func() {
		_, _ = io.Copy(stderrBuffer, stderr)
	}()
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = killHostProcess(cmd.Process)
		}
		_ = cmd.Wait()
	}()

	reader := bufio.NewScanner(stdout)
	reader.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if err := mcpWriteRequest(stdin, 1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "gi", "version": DefaultCodingAgentVersion},
	}); err != nil {
		return nil, mcpAttachContextOrStderr(ctx, err, stderrBuffer)
	}
	if _, _, err := mcpReadResponse(reader, 1); err != nil {
		return nil, mcpAttachContextOrStderr(ctx, err, stderrBuffer)
	}
	if err := mcpWriteNotification(stdin, "notifications/initialized", map[string]any{}); err != nil {
		return nil, mcpAttachContextOrStderr(ctx, err, stderrBuffer)
	}
	if err := mcpWriteRequest(stdin, 2, method, params); err != nil {
		return nil, mcpAttachContextOrStderr(ctx, err, stderrBuffer)
	}
	result, notifications, err := mcpReadResponse(reader, 2)
	if err != nil {
		return nil, mcpAttachContextOrStderr(ctx, err, stderrBuffer)
	}
	mcpAttachNotifications(result, notifications)
	return result, nil
}

func mcpWriteRequest(writer io.Writer, id int, method string, params any) error {
	return json.NewEncoder(writer).Encode(mcpRPCEnvelope{JSONRPC: "2.0", ID: id, Method: method, Params: params})
}

func mcpWriteNotification(writer io.Writer, method string, params any) error {
	return json.NewEncoder(writer).Encode(mcpRPCEnvelope{JSONRPC: "2.0", Method: method, Params: params})
}

func mcpReadResponse(reader *bufio.Scanner, id int) (map[string]any, []mcpRPCEnvelope, error) {
	var notifications []mcpRPCEnvelope
	for reader.Scan() {
		var envelope mcpRPCEnvelope
		if err := json.Unmarshal(reader.Bytes(), &envelope); err != nil {
			continue
		}
		if envelope.ID == nil && strings.TrimSpace(envelope.Method) != "" {
			notifications = append(notifications, envelope)
			continue
		}
		if !mcpIDMatches(envelope.ID, id) {
			continue
		}
		if envelope.Error != nil {
			return nil, notifications, errors.New(envelope.Error.Message)
		}
		if len(envelope.Result) == 0 {
			return map[string]any{}, notifications, nil
		}
		var result map[string]any
		if err := json.Unmarshal(envelope.Result, &result); err != nil {
			return nil, notifications, err
		}
		return result, notifications, nil
	}
	if err := reader.Err(); err != nil {
		return nil, notifications, err
	}
	return nil, notifications, io.EOF
}

func mcpAttachNotifications(result map[string]any, notifications []mcpRPCEnvelope) {
	if result == nil || len(notifications) == 0 {
		return
	}
	items := make([]map[string]any, 0, len(notifications))
	var progress []map[string]any
	var toolsListChanged bool
	for _, notification := range notifications {
		method := strings.TrimSpace(notification.Method)
		if method == "" {
			continue
		}
		item := map[string]any{"method": method}
		if notification.Params != nil {
			item["params"] = notification.Params
		}
		items = append(items, item)
		switch method {
		case "notifications/progress", "$/progress":
			if object, ok := notification.Params.(map[string]any); ok {
				progress = append(progress, object)
			}
		case "notifications/tools/list_changed":
			toolsListChanged = true
		}
	}
	if len(items) > 0 {
		result["_notifications"] = items
	}
	if len(progress) > 0 {
		result["_progress"] = progress
	}
	if toolsListChanged {
		result["_toolsListChanged"] = true
	}
}

func mcpIDMatches(value any, want int) bool {
	switch typed := value.(type) {
	case float64:
		return int(typed) == want
	case int:
		return typed == want
	case string:
		return typed == fmt.Sprint(want)
	default:
		return false
	}
}

func mcpAttachStderr(err error, stderr *mcpStderrBuffer) error {
	if stderr == nil {
		return err
	}
	content := strings.TrimSpace(stderr.String())
	if content == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, content)
}

func mcpAttachContextOrStderr(ctx context.Context, err error, stderr *mcpStderrBuffer) error {
	if ctx != nil && ctx.Err() != nil {
		return mcpAttachStderr(ctx.Err(), stderr)
	}
	return mcpAttachStderr(err, stderr)
}
