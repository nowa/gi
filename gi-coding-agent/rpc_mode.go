package gicodingagent

import (
	"context"
	"encoding/json"
)

type RPCLineProcessor struct {
	Host      *RPCSessionHost
	WriteLine func(string)
}

func (p *RPCLineProcessor) HandleLine(ctx context.Context, line string) {
	var command RPCCommand
	if err := json.Unmarshal([]byte(line), &command); err != nil {
		return
	}
	response := p.handleCommand(ctx, command)
	response.ID = command.ID
	p.writeResponse(response)
}

func (p *RPCLineProcessor) handleCommand(ctx context.Context, command RPCCommand) RPCResponse {
	if p == nil || p.Host == nil {
		return rpcErrorResponse(command.Type, nil)
	}
	if command.Type == RPCCommandPrompt {
		if err := p.Host.AcceptPrompt(command); err != nil {
			return rpcErrorResponse(command.Type, err)
		}
		return rpcSuccessResponse(command.Type, nil)
	}
	return p.Host.HandleCommand(ctx, command)
}

func (p *RPCLineProcessor) writeResponse(response RPCResponse) {
	if p == nil || p.WriteLine == nil {
		return
	}
	line, err := SerializeJSONLine(response)
	if err != nil {
		return
	}
	p.WriteLine(line)
}
