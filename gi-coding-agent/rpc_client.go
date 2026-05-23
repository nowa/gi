package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

const RPCCommandClone = "clone"

type RPCCommand struct {
	ID                 string            `json:"id,omitempty"`
	Type               string            `json:"type"`
	Message            string            `json:"message,omitempty"`
	StreamingBehavior  string            `json:"streamingBehavior,omitempty"`
	Provider           string            `json:"provider,omitempty"`
	ModelID            string            `json:"modelId,omitempty"`
	Level              string            `json:"level,omitempty"`
	Mode               string            `json:"mode,omitempty"`
	CustomInstructions string            `json:"customInstructions,omitempty"`
	Enabled            *bool             `json:"enabled,omitempty"`
	Command            string            `json:"command,omitempty"`
	Images             []llm.ContentPart `json:"images,omitempty"`
	OutputPath         string            `json:"outputPath,omitempty"`
	SessionPath        string            `json:"sessionPath,omitempty"`
	EntryID            string            `json:"entryId,omitempty"`
	Name               string            `json:"name,omitempty"`
	ParentSession      string            `json:"parentSession,omitempty"`
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

type RPCThinkingLevelResult struct {
	Level string `json:"level"`
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

func (c *RPCClient) Prompt(ctx context.Context, message string) error {
	return c.sendNoData(ctx, RPCCommand{Type: RPCCommandPrompt, Message: message})
}

func (c *RPCClient) PromptWithImages(ctx context.Context, message string, images []llm.ContentPart) error {
	return c.sendNoData(ctx, RPCCommand{Type: RPCCommandPrompt, Message: message, Images: images})
}

func (c *RPCClient) Steer(ctx context.Context, message string) error {
	return c.sendNoData(ctx, RPCCommand{Type: RPCCommandSteer, Message: message})
}

func (c *RPCClient) SteerWithImages(ctx context.Context, message string, images []llm.ContentPart) error {
	return c.sendNoData(ctx, RPCCommand{Type: RPCCommandSteer, Message: message, Images: images})
}

func (c *RPCClient) FollowUp(ctx context.Context, message string) error {
	return c.sendNoData(ctx, RPCCommand{Type: RPCCommandFollowUp, Message: message})
}

func (c *RPCClient) FollowUpWithImages(ctx context.Context, message string, images []llm.ContentPart) error {
	return c.sendNoData(ctx, RPCCommand{Type: RPCCommandFollowUp, Message: message, Images: images})
}

func (c *RPCClient) Abort(ctx context.Context) error {
	return c.sendNoData(ctx, RPCCommand{Type: RPCCommandAbort})
}

func (c *RPCClient) NewSession(ctx context.Context, parentSession string) (RPCCloneResult, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandNewSession, ParentSession: parentSession})
	if err != nil {
		return RPCCloneResult{}, err
	}
	return rpcResponseData[RPCCloneResult](response)
}

func (c *RPCClient) GetState(ctx context.Context) (RPCSessionState, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandGetState})
	if err != nil {
		return RPCSessionState{}, err
	}
	return rpcResponseData[RPCSessionState](response)
}

func (c *RPCClient) SetModel(ctx context.Context, provider, modelID string) (llm.Model, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandSetModel, Provider: provider, ModelID: modelID})
	if err != nil {
		return llm.Model{}, err
	}
	return rpcResponseData[llm.Model](response)
}

func (c *RPCClient) CycleModel(ctx context.Context) (*RPCCycleModelResult, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandCycleModel})
	if err != nil {
		return nil, err
	}
	return rpcResponseData[*RPCCycleModelResult](response)
}

func (c *RPCClient) GetAvailableModels(ctx context.Context) ([]llm.Model, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandGetAvailableModels})
	if err != nil {
		return nil, err
	}
	result, err := rpcResponseData[RPCAvailableModelsResult](response)
	if err != nil {
		return nil, err
	}
	return result.Models, nil
}

func (c *RPCClient) SetThinkingLevel(ctx context.Context, level string) error {
	return c.sendNoData(ctx, RPCCommand{Type: RPCCommandSetThinkingLevel, Level: level})
}

func (c *RPCClient) CycleThinkingLevel(ctx context.Context) (*RPCThinkingLevelResult, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandCycleThinkingLevel})
	if err != nil {
		return nil, err
	}
	return rpcResponseData[*RPCThinkingLevelResult](response)
}

func (c *RPCClient) SetSteeringMode(ctx context.Context, mode string) error {
	return c.sendNoData(ctx, RPCCommand{Type: RPCCommandSetSteeringMode, Mode: mode})
}

func (c *RPCClient) SetFollowUpMode(ctx context.Context, mode string) error {
	return c.sendNoData(ctx, RPCCommand{Type: RPCCommandSetFollowUpMode, Mode: mode})
}

func (c *RPCClient) Compact(ctx context.Context, customInstructions string) (agentharness.CompactionResult, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandCompact, CustomInstructions: customInstructions})
	if err != nil {
		return agentharness.CompactionResult{}, err
	}
	return rpcResponseData[agentharness.CompactionResult](response)
}

func (c *RPCClient) SetAutoCompaction(ctx context.Context, enabled bool) error {
	return c.sendNoData(ctx, RPCCommand{Type: RPCCommandSetAutoCompaction, Enabled: &enabled})
}

func (c *RPCClient) SetAutoRetry(ctx context.Context, enabled bool) error {
	return c.sendNoData(ctx, RPCCommand{Type: RPCCommandSetAutoRetry, Enabled: &enabled})
}

func (c *RPCClient) AbortRetry(ctx context.Context) error {
	return c.sendNoData(ctx, RPCCommand{Type: RPCCommandAbortRetry})
}

func (c *RPCClient) Bash(ctx context.Context, command string) (BashResult, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandBash, Command: command})
	if err != nil {
		return BashResult{}, err
	}
	return rpcResponseData[BashResult](response)
}

func (c *RPCClient) AbortBash(ctx context.Context) error {
	return c.sendNoData(ctx, RPCCommand{Type: RPCCommandAbortBash})
}

func (c *RPCClient) GetSessionStats(ctx context.Context) (RPCSessionStats, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandGetSessionStats})
	if err != nil {
		return RPCSessionStats{}, err
	}
	return rpcResponseData[RPCSessionStats](response)
}

func (c *RPCClient) ExportHTML(ctx context.Context, outputPath string) (RPCExportHTMLResult, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandExportHTML, OutputPath: outputPath})
	if err != nil {
		return RPCExportHTMLResult{}, err
	}
	return rpcResponseData[RPCExportHTMLResult](response)
}

func (c *RPCClient) SwitchSession(ctx context.Context, sessionPath string) (RPCCloneResult, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandSwitchSession, SessionPath: sessionPath})
	if err != nil {
		return RPCCloneResult{}, err
	}
	return rpcResponseData[RPCCloneResult](response)
}

func (c *RPCClient) Fork(ctx context.Context, entryID string) (RPCForkResult, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandFork, EntryID: entryID})
	if err != nil {
		return RPCForkResult{}, err
	}
	return rpcResponseData[RPCForkResult](response)
}

func (c *RPCClient) Clone(ctx context.Context) (RPCCloneResult, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandClone})
	if err != nil {
		return RPCCloneResult{}, err
	}
	return rpcResponseData[RPCCloneResult](response)
}

func (c *RPCClient) GetForkMessages(ctx context.Context) ([]AgentSessionForkMessage, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandGetForkMessages})
	if err != nil {
		return nil, err
	}
	result, err := rpcResponseData[RPCForkMessagesResult](response)
	if err != nil {
		return nil, err
	}
	return result.Messages, nil
}

func (c *RPCClient) GetLastAssistantText(ctx context.Context) (*string, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandGetLastAssistantText})
	if err != nil {
		return nil, err
	}
	result, err := rpcResponseData[RPCLastAssistantTextResult](response)
	if err != nil {
		return nil, err
	}
	return result.Text, nil
}

func (c *RPCClient) SetSessionName(ctx context.Context, name string) error {
	return c.sendNoData(ctx, RPCCommand{Type: RPCCommandSetSessionName, Name: name})
}

func (c *RPCClient) GetMessages(ctx context.Context) ([]llm.Message, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandGetMessages})
	if err != nil {
		return nil, err
	}
	result, err := rpcResponseData[RPCMessagesResult](response)
	if err != nil {
		return nil, err
	}
	return result.Messages, nil
}

func (c *RPCClient) GetCommands(ctx context.Context) ([]RPCSlashCommand, error) {
	response, err := c.send(ctx, RPCCommand{Type: RPCCommandGetCommands})
	if err != nil {
		return nil, err
	}
	result, err := rpcResponseData[RPCCommandsResult](response)
	if err != nil {
		return nil, err
	}
	return result.Commands, nil
}

func (c *RPCClient) send(ctx context.Context, command RPCCommand) (RPCResponse, error) {
	if c.sender == nil {
		return RPCResponse{}, errors.New("RPC client not started")
	}
	return c.sender.SendRPCCommand(ctx, command)
}

func (c *RPCClient) sendNoData(ctx context.Context, command RPCCommand) error {
	response, err := c.send(ctx, command)
	if err != nil {
		return err
	}
	_, err = rpcResponseData[struct{}](response)
	return err
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

func rpcSuccessResponse(command string, data any) RPCResponse {
	response := RPCResponse{Type: "response", Command: command, Success: true}
	if data == nil {
		return response
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		response.Success = false
		response.Error = err.Error()
		return response
	}
	response.Data = encoded
	return response
}

func rpcErrorResponse(command string, err error) RPCResponse {
	message := "RPC command failed"
	if err != nil {
		message = err.Error()
	}
	return RPCResponse{Type: "response", Command: command, Success: false, Error: message}
}
