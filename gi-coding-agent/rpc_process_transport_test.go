package gicodingagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const rpcProcessHelperEnvironment = "GI_RPC_PROCESS_HELPER"

func TestRPCProcessLineWriterPreservesJSONLFraming(t *testing.T) {
	var lines []string
	writer := &rpcProcessLineWriter{
		onLine: func(line string) {
			lines = append(lines, line)
		},
	}
	if _, err := writer.Write([]byte("\r\none\r\ntw")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("o\npartial\r")); err != nil {
		t.Fatal(err)
	}
	writer.Flush()
	if want := []string{"one", "two", "partial"}; !reflect.DeepEqual(lines, want) {
		t.Fatalf("lines = %#v, want %#v", lines, want)
	}
}

func TestRPCProcessTransportSupportsSerializedRestart(t *testing.T) {
	client := NewRPCProcessClient(rpcProcessTestOptions("serve"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Start(ctx); err == nil {
		t.Fatal("Start() accepted an already running process")
	}
	if err := client.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := client.GetState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.SessionID != "helper-session" {
		t.Fatalf("session ID = %q", state.SessionID)
	}
	if err := client.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRPCProcessTransportRoutesConcurrentResponsesAndEvents(t *testing.T) {
	client := NewRPCProcessClient(rpcProcessTestOptions("serve"))
	if err := client.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := client.Stop(ctx); err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	})

	events := make(chan AgentSessionEvent, 1)
	unsubscribe := client.OnEvent(func(event AgentSessionEvent) {
		select {
		case events <- event:
		default:
		}
	})
	defer unsubscribe()

	levels, err := client.GetAvailableThinkingLevels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(levels, []string{"off", "low", "high"}) {
		t.Fatalf("thinking levels = %#v", levels)
	}
	select {
	case event := <-events:
		if event.Type != "agent_settled" {
			t.Fatalf("event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for RPC event")
	}

	const requestCount = 16
	errs := make(chan error, requestCount)
	var wait sync.WaitGroup
	for range requestCount {
		wait.Add(1)
		go func() {
			defer wait.Done()
			state, err := client.GetState(context.Background())
			if err == nil && state.SessionID != "helper-session" {
				err = fmt.Errorf("session ID = %q", state.SessionID)
			}
			errs <- err
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	deadline := time.Now().Add(time.Second)
	for !strings.Contains(client.GetStderr(), "helper diagnostic") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if stderr := client.GetStderr(); !strings.Contains(stderr, "helper diagnostic") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRPCProcessTransportRejectsAllPendingRequestsOnExit(t *testing.T) {
	client := NewRPCProcessClient(rpcProcessTestOptions("exit"))
	if err := client.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	const pendingCount = 4
	errs := make(chan error, pendingCount)
	for range pendingCount {
		go func() {
			_, err := client.GetCommands(context.Background())
			errs <- err
		}()
	}
	for range pendingCount {
		err := <-errs
		var exitError *RPCProcessExitError
		if !errors.As(err, &exitError) {
			t.Fatalf("GetCommands() error = %T %v, want RPCProcessExitError", err, err)
		}
		if exitError.Code == nil || *exitError.Code != 43 || exitError.Signal != "" {
			t.Fatalf("exit state = %#v", exitError)
		}
		if !strings.Contains(exitError.Stderr, "helper boom") ||
			!strings.Contains(err.Error(), "code=43 signal=null") {
			t.Fatalf("exit error = %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if stderr := client.GetStderr(); !strings.Contains(stderr, "helper boom") {
		t.Fatalf("retained stderr = %q", stderr)
	}
}

func TestRPCProcessTransportHonorsRequestContext(t *testing.T) {
	client := NewRPCProcessClient(rpcProcessTestOptions("hang"))
	if err := client.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = client.Stop(ctx)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if err := client.Prompt(ctx, strings.Repeat("blocked", 1<<20)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Prompt() error = %v, want context deadline", err)
	}
}

func rpcProcessTestOptions(mode string) RPCProcessOptions {
	return RPCProcessOptions{
		Command: os.Args[0],
		Args:    []string{"-test.run=^TestRPCProcessTransportHelper$"},
		Env:     map[string]string{rpcProcessHelperEnvironment: mode},
	}
}

func TestRPCProcessTransportHelper(t *testing.T) {
	mode := os.Getenv(rpcProcessHelperEnvironment)
	if mode == "" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	switch mode {
	case "exit":
		for range 4 {
			if !scanner.Scan() {
				return
			}
		}
		_, _ = fmt.Fprintln(os.Stderr, "helper boom")
		os.Exit(43)
	case "hang":
		<-time.After(10 * time.Minute)
	case "serve":
		_, _ = fmt.Fprintln(os.Stderr, "helper diagnostic")
		emittedEvent := false
		for scanner.Scan() {
			var command RPCCommand
			if err := json.Unmarshal(scanner.Bytes(), &command); err != nil {
				continue
			}
			if !emittedEvent {
				writeRPCProcessHelperJSON(AgentSessionEvent{Type: "agent_settled"})
				emittedEvent = true
			}
			var response RPCResponse
			switch command.Type {
			case RPCCommandGetAvailableThinkingLevels:
				response = rpcSuccessResponse(command.Type, RPCThinkingLevelsResult{
					Levels: []string{"off", "low", "high"},
				})
			case RPCCommandGetState:
				response = rpcSuccessResponse(command.Type, RPCSessionState{SessionID: "helper-session"})
			default:
				response = rpcErrorResponse(command.Type, errors.New("unsupported helper command"))
			}
			response.ID = command.ID
			writeRPCProcessHelperJSON(response)
		}
	}
}

func writeRPCProcessHelperJSON(value any) {
	line, err := SerializeJSONLine(value)
	if err != nil {
		return
	}
	_, _ = fmt.Fprint(os.Stdout, line)
}
