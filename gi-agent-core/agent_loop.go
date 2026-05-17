package giagentcore

import (
	"context"
	"fmt"
	"sync"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type AgentEventSink func(event AgentEvent) error

func AgentLoop(prompts []llm.Message, agentContext AgentContext, config AgentLoopConfig, ctx context.Context, streamFn StreamFn) *llm.EventStream[AgentEvent, []llm.Message] {
	stream := llm.NewEventStream(func(event AgentEvent) bool {
		return event.Type == "agent_end"
	}, func(event AgentEvent) []llm.Message {
		return event.Messages
	})
	go func() {
		messages, _ := RunAgentLoop(prompts, agentContext, config, func(event AgentEvent) error {
			stream.Push(event)
			return nil
		}, ctx, streamFn)
		if messages != nil {
			stream.End(messages)
		}
	}()
	return stream
}

func AgentLoopContinue(agentContext AgentContext, config AgentLoopConfig, ctx context.Context, streamFn StreamFn) (*llm.EventStream[AgentEvent, []llm.Message], error) {
	if len(agentContext.Messages) == 0 {
		return nil, fmt.Errorf("cannot continue: no messages in context")
	}
	if agentContext.Messages[len(agentContext.Messages)-1].Role == llm.RoleAssistant {
		return nil, fmt.Errorf("cannot continue from message role: assistant")
	}
	stream := llm.NewEventStream(func(event AgentEvent) bool {
		return event.Type == "agent_end"
	}, func(event AgentEvent) []llm.Message {
		return event.Messages
	})
	go func() {
		messages, _ := RunAgentLoopContinue(agentContext, config, func(event AgentEvent) error {
			stream.Push(event)
			return nil
		}, ctx, streamFn)
		if messages != nil {
			stream.End(messages)
		}
	}()
	return stream, nil
}

func RunAgentLoop(prompts []llm.Message, agentContext AgentContext, config AgentLoopConfig, emit AgentEventSink, ctx context.Context, streamFn StreamFn) ([]llm.Message, error) {
	newMessages := append([]llm.Message{}, prompts...)
	currentContext := AgentContext{
		SystemPrompt: agentContext.SystemPrompt,
		Messages:     append(append([]llm.Message{}, agentContext.Messages...), prompts...),
		Tools:        append([]AgentTool{}, agentContext.Tools...),
	}

	if err := emit(AgentEvent{Type: "agent_start"}); err != nil {
		return nil, err
	}
	if err := emit(AgentEvent{Type: "turn_start"}); err != nil {
		return nil, err
	}
	for _, prompt := range prompts {
		if err := emit(AgentEvent{Type: "message_start", Message: prompt}); err != nil {
			return nil, err
		}
		if err := emit(AgentEvent{Type: "message_end", Message: prompt}); err != nil {
			return nil, err
		}
	}
	return newMessages, runLoop(ctx, currentContext, &newMessages, config, emit, streamFn)
}

func RunAgentLoopContinue(agentContext AgentContext, config AgentLoopConfig, emit AgentEventSink, ctx context.Context, streamFn StreamFn) ([]llm.Message, error) {
	if len(agentContext.Messages) == 0 {
		return nil, fmt.Errorf("cannot continue: no messages in context")
	}
	if agentContext.Messages[len(agentContext.Messages)-1].Role == llm.RoleAssistant {
		return nil, fmt.Errorf("cannot continue from message role: assistant")
	}
	currentContext := AgentContext{
		SystemPrompt: agentContext.SystemPrompt,
		Messages:     append([]llm.Message{}, agentContext.Messages...),
		Tools:        append([]AgentTool{}, agentContext.Tools...),
	}
	newMessages := []llm.Message{}

	if err := emit(AgentEvent{Type: "agent_start"}); err != nil {
		return nil, err
	}
	if err := emit(AgentEvent{Type: "turn_start"}); err != nil {
		return nil, err
	}
	return newMessages, runLoop(ctx, currentContext, &newMessages, config, emit, streamFn)
}

func runLoop(ctx context.Context, initialContext AgentContext, newMessages *[]llm.Message, initialConfig AgentLoopConfig, emit AgentEventSink, streamFn StreamFn) error {
	currentContext := initialContext
	config := initialConfig
	firstTurn := true
	pendingMessages, err := drain(config.GetSteeringMessages)
	if err != nil {
		return err
	}

	for {
		hasMoreToolCalls := true
		for hasMoreToolCalls || len(pendingMessages) > 0 {
			if !firstTurn {
				if err := emit(AgentEvent{Type: "turn_start"}); err != nil {
					return err
				}
			} else {
				firstTurn = false
			}

			for _, message := range pendingMessages {
				if err := emit(AgentEvent{Type: "message_start", Message: message}); err != nil {
					return err
				}
				if err := emit(AgentEvent{Type: "message_end", Message: message}); err != nil {
					return err
				}
				currentContext.Messages = append(currentContext.Messages, message)
				*newMessages = append(*newMessages, message)
			}
			pendingMessages = nil

			message, err := streamAssistantResponse(ctx, &currentContext, config, emit, streamFn)
			if err != nil {
				return err
			}
			*newMessages = append(*newMessages, message)

			if message.StopReason == llm.StopReasonError || message.StopReason == llm.StopReasonAborted {
				if err := emit(AgentEvent{Type: "turn_end", Message: message}); err != nil {
					return err
				}
				return emit(AgentEvent{Type: "agent_end", Messages: append([]llm.Message{}, *newMessages...)})
			}

			toolCalls := toolCallsFromMessage(message)
			toolResults := []llm.Message{}
			hasMoreToolCalls = false
			if len(toolCalls) > 0 {
				batch, err := executeToolCalls(ctx, currentContext, message, toolCalls, config, emit)
				if err != nil {
					return err
				}
				toolResults = append(toolResults, batch.messages...)
				hasMoreToolCalls = !batch.terminate
				for _, result := range toolResults {
					currentContext.Messages = append(currentContext.Messages, result)
					*newMessages = append(*newMessages, result)
				}
			}

			if err := emit(AgentEvent{Type: "turn_end", Message: message, ToolResults: toolResults}); err != nil {
				return err
			}

			nextContext := PrepareNextTurnContext{Message: message, ToolResults: toolResults, Context: currentContext, NewMessages: *newMessages}
			if config.PrepareNextTurn != nil {
				update, ok, err := config.PrepareNextTurn(nextContext)
				if err != nil {
					return err
				}
				if ok {
					if update.Context != nil {
						currentContext = *update.Context
					}
					if update.Model != nil {
						config.Model = *update.Model
					}
					if update.ThinkingLevel != nil {
						if *update.ThinkingLevel == "off" {
							config.Reasoning = ""
						} else {
							config.Reasoning = *update.ThinkingLevel
						}
					}
				}
			}

			if config.ShouldStopAfterTurn != nil {
				stop, err := config.ShouldStopAfterTurn(ShouldStopAfterTurnContext{Message: message, ToolResults: toolResults, Context: currentContext, NewMessages: *newMessages})
				if err != nil {
					return err
				}
				if stop {
					return emit(AgentEvent{Type: "agent_end", Messages: append([]llm.Message{}, *newMessages...)})
				}
			}

			pendingMessages, err = drain(config.GetSteeringMessages)
			if err != nil {
				return err
			}
		}

		followUpMessages, err := drain(config.GetFollowUpMessages)
		if err != nil {
			return err
		}
		if len(followUpMessages) > 0 {
			pendingMessages = followUpMessages
			continue
		}
		break
	}

	return emit(AgentEvent{Type: "agent_end", Messages: append([]llm.Message{}, *newMessages...)})
}

func drain(fn func() ([]llm.Message, error)) ([]llm.Message, error) {
	if fn == nil {
		return nil, nil
	}
	messages, err := fn()
	if err != nil {
		return nil, err
	}
	return messages, nil
}

func streamAssistantResponse(ctx context.Context, agentContext *AgentContext, config AgentLoopConfig, emit AgentEventSink, streamFn StreamFn) (llm.Message, error) {
	messages := agentContext.Messages
	var err error
	if config.TransformContext != nil {
		messages, err = config.TransformContext(ctx, messages)
		if err != nil {
			return llm.Message{}, err
		}
	}

	convert := config.ConvertToLLM
	if convert == nil {
		convert = defaultConvertToLLM
	}
	llmMessages, err := convert(messages)
	if err != nil {
		return llm.Message{}, err
	}

	llmTools := make([]llm.Tool, 0, len(agentContext.Tools))
	for _, tool := range agentContext.Tools {
		llmTools = append(llmTools, tool.AsLLMTool())
	}
	llmContext := llm.Context{SystemPrompt: agentContext.SystemPrompt, Messages: llmMessages, Tools: llmTools}
	if config.GetAPIKey != nil && config.APIKey == "" {
		config.APIKey = config.GetAPIKey(config.Model.Provider)
	}
	config.SimpleStreamOptions.Context = ctx

	streamFunction := streamFn
	if streamFunction == nil {
		streamFunction = llm.StreamSimple
	}
	stream, err := streamFunction(config.Model, llmContext, config.SimpleStreamOptions)
	if err != nil {
		return llm.Message{}, err
	}

	var partial llm.Message
	addedPartial := false
	for event := range stream.Events() {
		switch event.Type {
		case "start":
			partial = event.Partial
			agentContext.Messages = append(agentContext.Messages, partial)
			addedPartial = true
			if err := emit(AgentEvent{Type: "message_start", Message: partial}); err != nil {
				return llm.Message{}, err
			}
		case "text_start", "text_delta", "text_end", "thinking_start", "thinking_delta", "thinking_end", "toolcall_start", "toolcall_delta", "toolcall_end":
			if addedPartial {
				partial = event.Partial
				agentContext.Messages[len(agentContext.Messages)-1] = partial
				if err := emit(AgentEvent{Type: "message_update", Message: partial, AssistantMessageEvent: event}); err != nil {
					return llm.Message{}, err
				}
			}
		case "done", "error":
			finalMessage, err := stream.Result(ctx)
			if err != nil {
				return llm.Message{}, err
			}
			if addedPartial {
				agentContext.Messages[len(agentContext.Messages)-1] = finalMessage
			} else {
				agentContext.Messages = append(agentContext.Messages, finalMessage)
				if err := emit(AgentEvent{Type: "message_start", Message: finalMessage}); err != nil {
					return llm.Message{}, err
				}
			}
			if err := emit(AgentEvent{Type: "message_end", Message: finalMessage}); err != nil {
				return llm.Message{}, err
			}
			return finalMessage, nil
		}
	}

	finalMessage, err := stream.Result(ctx)
	if err != nil {
		return llm.Message{}, err
	}
	if addedPartial {
		agentContext.Messages[len(agentContext.Messages)-1] = finalMessage
	} else {
		agentContext.Messages = append(agentContext.Messages, finalMessage)
		if err := emit(AgentEvent{Type: "message_start", Message: finalMessage}); err != nil {
			return llm.Message{}, err
		}
	}
	if err := emit(AgentEvent{Type: "message_end", Message: finalMessage}); err != nil {
		return llm.Message{}, err
	}
	return finalMessage, nil
}

func defaultConvertToLLM(messages []llm.Message) ([]llm.Message, error) {
	result := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role == llm.RoleUser || message.Role == llm.RoleAssistant || message.Role == llm.RoleToolResult {
			result = append(result, message)
		}
	}
	return result, nil
}

func toolCallsFromMessage(message llm.Message) []AgentToolCall {
	var toolCalls []AgentToolCall
	for _, content := range message.Content {
		if content.Type == llm.ContentToolCall {
			toolCalls = append(toolCalls, content)
		}
	}
	return toolCalls
}

type executedToolCallBatch struct {
	messages  []llm.Message
	terminate bool
}

func executeToolCalls(ctx context.Context, agentContext AgentContext, assistantMessage llm.Message, toolCalls []AgentToolCall, config AgentLoopConfig, emit AgentEventSink) (executedToolCallBatch, error) {
	hasSequential := false
	for _, toolCall := range toolCalls {
		if tool, ok := findTool(agentContext.Tools, toolCall.Name); ok && tool.ExecutionMode == ToolExecutionSequential {
			hasSequential = true
			break
		}
	}
	if config.ToolExecution == ToolExecutionSequential || hasSequential {
		return executeToolCallsSequential(ctx, agentContext, assistantMessage, toolCalls, config, emit)
	}
	return executeToolCallsParallel(ctx, agentContext, assistantMessage, toolCalls, config, emit)
}

type finalizedToolCall struct {
	toolCall AgentToolCall
	result   AgentToolResult
	isError  bool
}

func executeToolCallsSequential(ctx context.Context, agentContext AgentContext, assistantMessage llm.Message, toolCalls []AgentToolCall, config AgentLoopConfig, emit AgentEventSink) (executedToolCallBatch, error) {
	finalizedCalls := []finalizedToolCall{}
	messages := []llm.Message{}
	for _, toolCall := range toolCalls {
		if err := emit(AgentEvent{Type: "tool_execution_start", ToolCallID: toolCall.ID, ToolName: toolCall.Name, Args: toolCall.Arguments}); err != nil {
			return executedToolCallBatch{}, err
		}
		finalized, err := prepareExecuteFinalizeToolCall(ctx, agentContext, assistantMessage, toolCall, config, emit)
		if err != nil {
			return executedToolCallBatch{}, err
		}
		if err := emitToolExecutionEnd(finalized, emit); err != nil {
			return executedToolCallBatch{}, err
		}
		message := createToolResultMessage(finalized)
		if err := emitToolResultMessage(message, emit); err != nil {
			return executedToolCallBatch{}, err
		}
		finalizedCalls = append(finalizedCalls, finalized)
		messages = append(messages, message)
	}
	return executedToolCallBatch{messages: messages, terminate: shouldTerminateToolBatch(finalizedCalls)}, nil
}

func executeToolCallsParallel(ctx context.Context, agentContext AgentContext, assistantMessage llm.Message, toolCalls []AgentToolCall, config AgentLoopConfig, emit AgentEventSink) (executedToolCallBatch, error) {
	type entry struct {
		index     int
		finalized finalizedToolCall
		err       error
	}
	finalizedCalls := make([]finalizedToolCall, len(toolCalls))
	ch := make(chan entry, len(toolCalls))
	var wg sync.WaitGroup
	var emitMu sync.Mutex

	for i, toolCall := range toolCalls {
		if err := emit(AgentEvent{Type: "tool_execution_start", ToolCallID: toolCall.ID, ToolName: toolCall.Name, Args: toolCall.Arguments}); err != nil {
			return executedToolCallBatch{}, err
		}
		preparation, immediate, err := prepareToolCall(ctx, agentContext, assistantMessage, toolCall, config)
		if err != nil {
			return executedToolCallBatch{}, err
		}
		if immediate != nil {
			finalizedCalls[i] = *immediate
			if err := emitToolExecutionEnd(*immediate, emit); err != nil {
				return executedToolCallBatch{}, err
			}
			continue
		}

		wg.Add(1)
		go func(index int, prepared preparedToolCall) {
			defer wg.Done()
			executed := executePreparedToolCall(ctx, prepared, emit)
			finalized, err := finalizeExecutedToolCall(ctx, agentContext, assistantMessage, prepared, executed, config)
			if err == nil {
				emitMu.Lock()
				err = emitToolExecutionEnd(finalized, emit)
				emitMu.Unlock()
			}
			ch <- entry{index: index, finalized: finalized, err: err}
		}(i, preparation)
	}

	wg.Wait()
	close(ch)
	for item := range ch {
		if item.err != nil {
			return executedToolCallBatch{}, item.err
		}
		finalizedCalls[item.index] = item.finalized
	}

	messages := make([]llm.Message, 0, len(finalizedCalls))
	for _, finalized := range finalizedCalls {
		message := createToolResultMessage(finalized)
		if err := emitToolResultMessage(message, emit); err != nil {
			return executedToolCallBatch{}, err
		}
		messages = append(messages, message)
	}
	return executedToolCallBatch{messages: messages, terminate: shouldTerminateToolBatch(finalizedCalls)}, nil
}

type preparedToolCall struct {
	toolCall AgentToolCall
	tool     AgentTool
	args     map[string]any
}

type executedToolCall struct {
	result  AgentToolResult
	isError bool
}

func prepareToolCall(ctx context.Context, agentContext AgentContext, assistantMessage llm.Message, toolCall AgentToolCall, config AgentLoopConfig) (preparedToolCall, *finalizedToolCall, error) {
	tool, ok := findTool(agentContext.Tools, toolCall.Name)
	if !ok {
		finalized := finalizedToolCall{toolCall: toolCall, result: createErrorToolResult(fmt.Sprintf("Tool %s not found", toolCall.Name)), isError: true}
		return preparedToolCall{}, &finalized, nil
	}

	preparedToolCallContent := toolCall
	if tool.PrepareArguments != nil {
		args, err := tool.PrepareArguments(toolCall.Arguments)
		if err != nil {
			finalized := finalizedToolCall{toolCall: toolCall, result: createErrorToolResult(err.Error()), isError: true}
			return preparedToolCall{}, &finalized, nil
		}
		preparedToolCallContent.Arguments = args
	}
	args, err := llm.ValidateToolArguments(tool.AsLLMTool(), preparedToolCallContent)
	if err != nil {
		finalized := finalizedToolCall{toolCall: toolCall, result: createErrorToolResult(err.Error()), isError: true}
		return preparedToolCall{}, &finalized, nil
	}
	if config.BeforeToolCall != nil {
		before, err := config.BeforeToolCall(ctx, BeforeToolCallContext{AssistantMessage: assistantMessage, ToolCall: toolCall, Args: args, Context: agentContext})
		if err != nil {
			finalized := finalizedToolCall{toolCall: toolCall, result: createErrorToolResult(err.Error()), isError: true}
			return preparedToolCall{}, &finalized, nil
		}
		if before.Block {
			reason := before.Reason
			if reason == "" {
				reason = "Tool execution was blocked"
			}
			finalized := finalizedToolCall{toolCall: toolCall, result: createErrorToolResult(reason), isError: true}
			return preparedToolCall{}, &finalized, nil
		}
	}
	return preparedToolCall{toolCall: toolCall, tool: tool, args: args}, nil, nil
}

func prepareExecuteFinalizeToolCall(ctx context.Context, agentContext AgentContext, assistantMessage llm.Message, toolCall AgentToolCall, config AgentLoopConfig, emit AgentEventSink) (finalizedToolCall, error) {
	prepared, immediate, err := prepareToolCall(ctx, agentContext, assistantMessage, toolCall, config)
	if err != nil {
		return finalizedToolCall{}, err
	}
	if immediate != nil {
		return *immediate, nil
	}
	executed := executePreparedToolCall(ctx, prepared, emit)
	return finalizeExecutedToolCall(ctx, agentContext, assistantMessage, prepared, executed, config)
}

func executePreparedToolCall(ctx context.Context, prepared preparedToolCall, emit AgentEventSink) executedToolCall {
	if prepared.tool.Execute == nil {
		return executedToolCall{result: createErrorToolResult("Tool has no execute function"), isError: true}
	}
	result, err := prepared.tool.Execute(ctx, prepared.toolCall.ID, prepared.args, func(partial AgentToolResult) {
		_ = emit(AgentEvent{Type: "tool_execution_update", ToolCallID: prepared.toolCall.ID, ToolName: prepared.toolCall.Name, Args: prepared.toolCall.Arguments, PartialResult: partial})
	})
	if err != nil {
		return executedToolCall{result: createErrorToolResult(err.Error()), isError: true}
	}
	return executedToolCall{result: result}
}

func finalizeExecutedToolCall(ctx context.Context, agentContext AgentContext, assistantMessage llm.Message, prepared preparedToolCall, executed executedToolCall, config AgentLoopConfig) (finalizedToolCall, error) {
	result := executed.result
	isError := executed.isError
	if config.AfterToolCall != nil {
		after, err := config.AfterToolCall(ctx, AfterToolCallContext{AssistantMessage: assistantMessage, ToolCall: prepared.toolCall, Args: prepared.args, Result: result, IsError: isError, Context: agentContext})
		if err != nil {
			return finalizedToolCall{toolCall: prepared.toolCall, result: createErrorToolResult(err.Error()), isError: true}, nil
		}
		if after.HasContent {
			result.Content = after.Content
		}
		if after.HasDetails {
			result.Details = after.Details
		}
		if after.HasTerminate {
			result.Terminate = after.Terminate
		}
		if after.HasIsError {
			isError = after.IsError
		}
	}
	return finalizedToolCall{toolCall: prepared.toolCall, result: result, isError: isError}, nil
}

func createErrorToolResult(message string) AgentToolResult {
	return AgentToolResult{Content: []llm.ContentPart{llm.Text(message)}, Details: map[string]any{}}
}

func emitToolExecutionEnd(finalized finalizedToolCall, emit AgentEventSink) error {
	return emit(AgentEvent{Type: "tool_execution_end", ToolCallID: finalized.toolCall.ID, ToolName: finalized.toolCall.Name, Result: finalized.result, IsError: finalized.isError})
}

func createToolResultMessage(finalized finalizedToolCall) llm.Message {
	return llm.Message{
		Role:       llm.RoleToolResult,
		ToolCallID: finalized.toolCall.ID,
		ToolName:   finalized.toolCall.Name,
		Content:    finalized.result.Content,
		Details:    finalized.result.Details,
		IsError:    finalized.isError,
		Timestamp:  llm.NowMillis(),
	}
}

func emitToolResultMessage(message llm.Message, emit AgentEventSink) error {
	if err := emit(AgentEvent{Type: "message_start", Message: message}); err != nil {
		return err
	}
	return emit(AgentEvent{Type: "message_end", Message: message})
}

func shouldTerminateToolBatch(finalizedCalls []finalizedToolCall) bool {
	if len(finalizedCalls) == 0 {
		return false
	}
	for _, finalized := range finalizedCalls {
		if !finalized.result.Terminate {
			return false
		}
	}
	return true
}

func findTool(tools []AgentTool, name string) (AgentTool, bool) {
	for _, tool := range tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return AgentTool{}, false
}
