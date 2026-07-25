package gicodingagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultRPCProcessRequestTimeout  = 30 * time.Second
	defaultRPCProcessShutdownTimeout = time.Second
)

var (
	// ErrRPCProcessNotStarted reports a command sent without an active process.
	ErrRPCProcessNotStarted = errors.New("RPC process not started")
	// ErrRPCProcessStopped reports a command interrupted by explicit shutdown.
	ErrRPCProcessStopped = errors.New("RPC process stopped")
)

// RPCProcessOptions configures the subprocess transport used by RPCClient.
// A nil Args slice starts the current executable with "--mode rpc".
type RPCProcessOptions struct {
	Command         string
	Args            []string
	CWD             string
	Env             map[string]string
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
	Stderr          io.Writer
}

// RPCProcessExitError records the stable process state and stderr tail observed
// when an RPC subprocess exits unexpectedly.
type RPCProcessExitError struct {
	Code   *int
	Signal string
	Stderr string
	Cause  error
}

func (e *RPCProcessExitError) Error() string {
	if e == nil {
		return "Agent process exited"
	}
	code := "null"
	if e.Code != nil {
		code = fmt.Sprintf("%d", *e.Code)
	}
	signal := "null"
	if e.Signal != "" {
		signal = e.Signal
	}
	return fmt.Sprintf("Agent process exited (code=%s signal=%s). Stderr: %s", code, signal, e.Stderr)
}

func (e *RPCProcessExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type rpcProcessResult struct {
	response RPCResponse
	err      error
}

type rpcProcessEventListener struct {
	id       uint64
	listener func(AgentSessionEvent)
}

type rpcProcessRun struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     *rpcProcessLineWriter
	stderrTail *processOutputTail
	done       chan struct{}
	writes     chan string
	pending    map[string]chan rpcProcessResult
	exitErr    error
	closing    bool
}

type rpcProcessLineWriter struct {
	mu     sync.Mutex
	buffer []byte
	onLine func(string)
}

// RPCProcessTransport owns subprocess lifecycle and JSONL response routing.
// RPCClient remains a typed protocol facade and delegates transport state here.
type RPCProcessTransport struct {
	options RPCProcessOptions

	mu             sync.Mutex
	run            *rpcProcessRun
	requestID      uint64
	listeners      []rpcProcessEventListener
	nextListenerID uint64
	lastStderr     string
}

var _ RPCCommandSender = (*RPCProcessTransport)(nil)

// NewRPCProcessTransport creates a stopped subprocess transport.
func NewRPCProcessTransport(options RPCProcessOptions) *RPCProcessTransport {
	if options.Args != nil {
		options.Args = append([]string{}, options.Args...)
	}
	if options.Env != nil {
		env := make(map[string]string, len(options.Env))
		for key, value := range options.Env {
			env[key] = value
		}
		options.Env = env
	}
	return &RPCProcessTransport{options: options}
}

// NewRPCProcessClient creates an RPC client backed by a stopped subprocess.
func NewRPCProcessClient(options RPCProcessOptions) *RPCClient {
	return NewRPCClient(NewRPCProcessTransport(options))
}

func (t *RPCProcessTransport) Start(ctx context.Context) error {
	if t == nil {
		return ErrRPCProcessNotStarted
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	t.mu.Lock()
	if t.run != nil {
		select {
		case <-t.run.done:
			t.run = nil
		default:
			t.mu.Unlock()
			return errors.New("RPC process already started")
		}
	}

	run, err := t.startProcess()
	if err != nil {
		t.mu.Unlock()
		return err
	}
	t.run = run
	t.lastStderr = ""
	t.mu.Unlock()

	go t.writeProcessStdin(run)
	go t.waitForProcess(run)
	return nil
}

func (t *RPCProcessTransport) startProcess() (*rpcProcessRun, error) {
	command := strings.TrimSpace(t.options.Command)
	if command == "" {
		executable, err := os.Executable()
		if err != nil {
			return nil, err
		}
		command = executable
	}
	args := append([]string(nil), t.options.Args...)
	if t.options.Args == nil {
		args = []string{"--mode", "rpc"}
	}

	cmd := exec.Command(command, args...)
	configureHostProcessCommand(cmd)
	if t.options.CWD != "" {
		cmd.Dir = t.options.CWD
	}
	if len(t.options.Env) > 0 {
		cmd.Env = mergedRPCProcessEnvironment(t.options.Env)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	run := &rpcProcessRun{
		cmd:        cmd,
		stdin:      stdin,
		stderrTail: &processOutputTail{limit: 64 * 1024},
		done:       make(chan struct{}),
		writes:     make(chan string),
		pending:    make(map[string]chan rpcProcessResult),
	}
	run.stdout = &rpcProcessLineWriter{
		onLine: func(line string) {
			t.handleLine(run, line)
		},
	}
	cmd.Stdout = run.stdout
	cmd.Stderr = run.stderrTail
	if t.options.Stderr != nil {
		cmd.Stderr = io.MultiWriter(run.stderrTail, t.options.Stderr)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}
	return run, nil
}

func mergedRPCProcessEnvironment(overrides map[string]string) []string {
	environment := make(map[string]string, len(os.Environ())+len(overrides))
	for _, pair := range os.Environ() {
		key, value, ok := strings.Cut(pair, "=")
		if ok {
			environment[key] = value
		}
	}
	for key, value := range overrides {
		environment[key] = value
	}
	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+environment[key])
	}
	return result
}

func (w *rpcProcessLineWriter) Write(p []byte) (int, error) {
	if w == nil {
		return len(p), nil
	}
	w.mu.Lock()
	w.buffer = append(w.buffer, p...)
	lines := w.takeLinesLocked(false)
	w.mu.Unlock()
	w.publish(lines)
	return len(p), nil
}

func (w *rpcProcessLineWriter) Flush() {
	if w == nil {
		return
	}
	w.mu.Lock()
	lines := w.takeLinesLocked(true)
	w.mu.Unlock()
	w.publish(lines)
}

func (w *rpcProcessLineWriter) takeLinesLocked(flush bool) []string {
	var lines []string
	for {
		index := bytes.IndexByte(w.buffer, '\n')
		if index < 0 {
			break
		}
		line := bytes.TrimSuffix(w.buffer[:index], []byte{'\r'})
		if len(line) > 0 {
			lines = append(lines, string(line))
		}
		w.buffer = w.buffer[index+1:]
	}
	if flush && len(w.buffer) > 0 {
		line := bytes.TrimSuffix(w.buffer, []byte{'\r'})
		if len(line) > 0 {
			lines = append(lines, string(line))
		}
		w.buffer = nil
	}
	return lines
}

func (w *rpcProcessLineWriter) publish(lines []string) {
	if w == nil || w.onLine == nil {
		return
	}
	for _, line := range lines {
		w.onLine(line)
	}
}

func (t *RPCProcessTransport) writeProcessStdin(run *rpcProcessRun) {
	for {
		select {
		case <-run.done:
			return
		case line := <-run.writes:
			if _, err := io.WriteString(run.stdin, line); err != nil {
				_ = terminateHostProcess(run.cmd.Process)
				return
			}
		}
	}
}

func (t *RPCProcessTransport) waitForProcess(run *rpcProcessRun) {
	err := run.cmd.Wait()
	run.stdout.Flush()

	exitErr := newRPCProcessExitError(run.cmd.ProcessState, run.stderrTail.String(), err)
	t.mu.Lock()
	run.exitErr = exitErr
	t.lastStderr = run.stderrTail.String()
	pendingErr := error(exitErr)
	if run.closing {
		pendingErr = ErrRPCProcessStopped
	}
	pending := make([]chan rpcProcessResult, 0, len(run.pending))
	for id, request := range run.pending {
		pending = append(pending, request)
		delete(run.pending, id)
	}
	close(run.done)
	t.mu.Unlock()

	for _, request := range pending {
		request <- rpcProcessResult{err: pendingErr}
	}
}

func newRPCProcessExitError(state *os.ProcessState, stderr string, cause error) *RPCProcessExitError {
	var code *int
	if state != nil {
		value := state.ExitCode()
		if value >= 0 {
			code = &value
		}
	}
	return &RPCProcessExitError{
		Code:   code,
		Signal: rpcProcessSignalName(state),
		Stderr: stderr,
		Cause:  cause,
	}
}

func (t *RPCProcessTransport) Stop(ctx context.Context) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	run := t.run
	if run == nil {
		t.mu.Unlock()
		return nil
	}
	run.closing = true
	t.mu.Unlock()

	_ = run.stdin.Close()
	_ = terminateHostProcess(run.cmd.Process)
	timer := time.NewTimer(t.shutdownTimeout())
	defer timer.Stop()

	select {
	case <-run.done:
	case <-ctx.Done():
		_ = killHostProcess(run.cmd.Process)
		return ctx.Err()
	case <-timer.C:
		_ = killHostProcess(run.cmd.Process)
		select {
		case <-run.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	t.mu.Lock()
	if t.run == run {
		t.run = nil
	}
	t.mu.Unlock()
	return nil
}

func (t *RPCProcessTransport) SendRPCCommand(ctx context.Context, command RPCCommand) (RPCResponse, error) {
	if t == nil {
		return RPCResponse{}, ErrRPCProcessNotStarted
	}
	if err := ctx.Err(); err != nil {
		return RPCResponse{}, err
	}

	result := make(chan rpcProcessResult, 1)
	t.mu.Lock()
	run := t.run
	if run == nil {
		t.mu.Unlock()
		return RPCResponse{}, ErrRPCProcessNotStarted
	}
	if run.exitErr != nil {
		err := run.exitErr
		t.mu.Unlock()
		return RPCResponse{}, err
	}
	if run.closing {
		t.mu.Unlock()
		return RPCResponse{}, ErrRPCProcessStopped
	}
	t.requestID++
	command.ID = fmt.Sprintf("req_%d", t.requestID)
	run.pending[command.ID] = result
	t.mu.Unlock()

	line, err := SerializeJSONLine(command)
	if err != nil {
		t.removePending(run, command.ID)
		return RPCResponse{}, err
	}

	timer := time.NewTimer(t.requestTimeout())
	defer timer.Stop()
	select {
	case run.writes <- line:
	case <-ctx.Done():
		t.removePending(run, command.ID)
		return RPCResponse{}, ctx.Err()
	case <-timer.C:
		t.removePending(run, command.ID)
		return RPCResponse{}, fmt.Errorf(
			"timeout writing command %s. Stderr: %s",
			command.Type,
			run.stderrTail.String(),
		)
	case <-run.done:
		response := <-result
		return response.response, response.err
	}

	select {
	case response := <-result:
		return response.response, response.err
	case <-ctx.Done():
		t.removePending(run, command.ID)
		return RPCResponse{}, ctx.Err()
	case <-timer.C:
		if !t.removePending(run, command.ID) {
			response := <-result
			return response.response, response.err
		}
		return RPCResponse{}, fmt.Errorf(
			"timeout waiting for response to %s. Stderr: %s",
			command.Type,
			run.stderrTail.String(),
		)
	}
}

func (t *RPCProcessTransport) removePending(run *rpcProcessRun, id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if run == nil {
		return false
	}
	if _, ok := run.pending[id]; !ok {
		return false
	}
	delete(run.pending, id)
	return true
}

func (t *RPCProcessTransport) handleLine(run *rpcProcessRun, line string) {
	var envelope struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		return
	}
	if envelope.Type == "response" && envelope.ID != "" {
		var response RPCResponse
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			return
		}
		t.mu.Lock()
		pending := run.pending[envelope.ID]
		if pending != nil {
			delete(run.pending, envelope.ID)
		}
		t.mu.Unlock()
		if pending != nil {
			pending <- rpcProcessResult{response: response}
		}
		return
	}

	var event AgentSessionEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil || event.Type == "" {
		return
	}
	t.mu.Lock()
	listeners := make([]func(AgentSessionEvent), 0, len(t.listeners))
	for _, registration := range t.listeners {
		listeners = append(listeners, registration.listener)
	}
	t.mu.Unlock()
	for _, listener := range listeners {
		listener(event)
	}
}

func (t *RPCProcessTransport) OnEvent(listener func(AgentSessionEvent)) func() {
	if t == nil || listener == nil {
		return func() {}
	}
	t.mu.Lock()
	t.nextListenerID++
	id := t.nextListenerID
	t.listeners = append(t.listeners, rpcProcessEventListener{id: id, listener: listener})
	t.mu.Unlock()
	return func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		for index := range t.listeners {
			if t.listeners[index].id != id {
				continue
			}
			t.listeners = append(t.listeners[:index], t.listeners[index+1:]...)
			return
		}
	}
}

func (t *RPCProcessTransport) GetStderr() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	run := t.run
	lastStderr := t.lastStderr
	t.mu.Unlock()
	if run == nil {
		return lastStderr
	}
	return run.stderrTail.String()
}

func (t *RPCProcessTransport) requestTimeout() time.Duration {
	if t.options.RequestTimeout > 0 {
		return t.options.RequestTimeout
	}
	return defaultRPCProcessRequestTimeout
}

func (t *RPCProcessTransport) shutdownTimeout() time.Duration {
	if t.options.ShutdownTimeout > 0 {
		return t.options.ShutdownTimeout
	}
	return defaultRPCProcessShutdownTimeout
}
