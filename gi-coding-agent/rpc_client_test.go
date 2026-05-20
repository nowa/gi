package gicodingagent

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestRPCClientCloneSendsCloneRPCCommand(t *testing.T) {
	var sent []RPCCommand
	client := NewRPCClient(RPCCommandSenderFunc(func(ctx context.Context, command RPCCommand) (RPCResponse, error) {
		sent = append(sent, command)
		return RPCResponse{
			Type:    "response",
			Command: RPCCommandClone,
			Success: true,
			Data:    mustJSONRawMessage(t, RPCCloneResult{Cancelled: false}),
		}, nil
	}))

	result, err := client.Clone(context.Background())
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}

	wantCommand := []RPCCommand{{Type: RPCCommandClone}}
	if !reflect.DeepEqual(sent, wantCommand) {
		t.Fatalf("sent commands = %#v, want %#v", sent, wantCommand)
	}
	wantResult := RPCCloneResult{Cancelled: false}
	if result != wantResult {
		t.Fatalf("Clone() = %#v, want %#v", result, wantResult)
	}
}

func mustJSONRawMessage(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%#v) error = %v", value, err)
	}
	return data
}
