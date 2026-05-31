package mcpstdio

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

type Options struct {
	Command []string
	Env     map[string]string
	Timeout time.Duration
	Hooks   ProcessHooks
}

type ProcessHooks struct {
	ConfigureCommand func(*exec.Cmd)
	KillProcess      func(*os.Process) error
	ForceKillDelay   time.Duration
	ClientVersion    string
}

type RPCEnvelope struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type StderrBuffer struct {
	mu      sync.Mutex
	content strings.Builder
}

func (b *StderrBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.content.Write(data)
}

func (b *StderrBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.content.String()
}

func RunListTools(options Options) (map[string]any, error) {
	return RunRequest(options, "tools/list", map[string]any{})
}

func RunCallTool(options Options, name string, arguments map[string]any) (map[string]any, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("mcp tool name is required")
	}
	return RunRequest(options, "tools/call", map[string]any{"name": name, "arguments": arguments})
}

func RunRequest(options Options, method string, params map[string]any) (map[string]any, error) {
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
	if options.Hooks.ConfigureCommand != nil {
		options.Hooks.ConfigureCommand(cmd)
	}
	if options.Hooks.KillProcess != nil {
		cmd.Cancel = func() error {
			return options.Hooks.KillProcess(cmd.Process)
		}
	}
	waitDelay := options.Hooks.ForceKillDelay
	if waitDelay <= 0 {
		waitDelay = 5 * time.Second
	}
	cmd.WaitDelay = waitDelay
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
	stderrBuffer := &StderrBuffer{}
	go func() {
		_, _ = io.Copy(stderrBuffer, stderr)
	}()
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil && options.Hooks.KillProcess != nil {
			_ = options.Hooks.KillProcess(cmd.Process)
		}
		_ = cmd.Wait()
	}()

	reader := bufio.NewScanner(stdout)
	reader.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	clientVersion := strings.TrimSpace(options.Hooks.ClientVersion)
	if clientVersion == "" {
		clientVersion = "dev"
	}
	if err := WriteRequest(stdin, 1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "gi", "version": clientVersion},
	}); err != nil {
		return nil, AttachContextOrStderr(ctx, err, stderrBuffer)
	}
	if _, _, err := ReadResponse(reader, 1); err != nil {
		return nil, AttachContextOrStderr(ctx, err, stderrBuffer)
	}
	if err := WriteNotification(stdin, "notifications/initialized", map[string]any{}); err != nil {
		return nil, AttachContextOrStderr(ctx, err, stderrBuffer)
	}
	if err := WriteRequest(stdin, 2, method, params); err != nil {
		return nil, AttachContextOrStderr(ctx, err, stderrBuffer)
	}
	result, notifications, err := ReadResponse(reader, 2)
	if err != nil {
		return nil, AttachContextOrStderr(ctx, err, stderrBuffer)
	}
	AttachNotifications(result, notifications)
	return result, nil
}

func WriteRequest(writer io.Writer, id int, method string, params any) error {
	return json.NewEncoder(writer).Encode(RPCEnvelope{JSONRPC: "2.0", ID: id, Method: method, Params: params})
}

func WriteNotification(writer io.Writer, method string, params any) error {
	return json.NewEncoder(writer).Encode(RPCEnvelope{JSONRPC: "2.0", Method: method, Params: params})
}

func ReadResponse(reader *bufio.Scanner, id int) (map[string]any, []RPCEnvelope, error) {
	var notifications []RPCEnvelope
	for reader.Scan() {
		var envelope RPCEnvelope
		if err := json.Unmarshal(reader.Bytes(), &envelope); err != nil {
			continue
		}
		if envelope.ID == nil && strings.TrimSpace(envelope.Method) != "" {
			notifications = append(notifications, envelope)
			continue
		}
		if !IDMatches(envelope.ID, id) {
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

func AttachNotifications(result map[string]any, notifications []RPCEnvelope) {
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

func IDMatches(value any, want int) bool {
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

func AttachStderr(err error, stderr *StderrBuffer) error {
	if stderr == nil {
		return err
	}
	content := strings.TrimSpace(stderr.String())
	if content == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, content)
}

func AttachContextOrStderr(ctx context.Context, err error, stderr *StderrBuffer) error {
	if ctx != nil && ctx.Err() != nil {
		return AttachStderr(ctx.Err(), stderr)
	}
	return AttachStderr(err, stderr)
}
