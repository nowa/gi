package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const RPCCommandClone = "clone"

type RPCCommand struct {
	ID   string `json:"id,omitempty"`
	Type string `json:"type"`
}

type RPCResponse struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Command string          `json:"command"`
	Success bool            `json:"success"`
	Error   string          `json:"error,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type RPCCloneResult struct {
	Cancelled bool `json:"cancelled"`
}

type RPCCommandSender interface {
	SendRPCCommand(context.Context, RPCCommand) (RPCResponse, error)
}

type RPCCommandSenderFunc func(context.Context, RPCCommand) (RPCResponse, error)

func (f RPCCommandSenderFunc) SendRPCCommand(ctx context.Context, command RPCCommand) (RPCResponse, error) {
	return f(ctx, command)
}

type RPCClient struct {
	sender RPCCommandSender
}

func NewRPCClient(sender RPCCommandSender) *RPCClient {
	return &RPCClient{sender: sender}
}

func (c *RPCClient) Clone(ctx context.Context) (RPCCloneResult, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandClone})
	if err != nil {
		return RPCCloneResult{}, err
	}
	return rpcResponseData[RPCCloneResult](response)
}

func (c *RPCClient) send(ctx context.Context, command RPCCommand) (RPCResponse, error) {
	if c.sender == nil {
		return RPCResponse{}, errors.New("RPC client not started")
	}
	return c.sender.SendRPCCommand(ctx, command)
}

func rpcResponseData[T any](response RPCResponse) (T, error) {
	var zero T
	if !response.Success {
		if response.Error != "" {
			return zero, errors.New(response.Error)
		}
		if response.Command != "" {
			return zero, fmt.Errorf("RPC command %s failed", response.Command)
		}
		return zero, errors.New("RPC command failed")
	}
	if len(response.Data) == 0 {
		return zero, nil
	}
	if err := json.Unmarshal(response.Data, &zero); err != nil {
		return zero, err
	}
	return zero, nil
}
