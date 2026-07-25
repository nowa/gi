package gicodingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type RPCLineProcessor struct {
	Host                *RPCSessionHost
	Runtime             *ProtocolExtensionRuntime
	SourceInfo          ProtocolSourceInfo
	WriteLine           func(string)
	OnBeforeHostAction  func(HostActionRequest)
	OnHostAction        func(HostActionRequest, HostActionResponse)
	AllowedCapabilities []string
	EnforceCapabilities bool
	RequestTimeout      time.Duration
	eventMu             sync.Mutex
	eventSeq            int
	requestSeq          int
	ownedViewTreeMu     sync.Mutex
	ownedViewTreeMounts map[string]bool
	pendingMu           sync.Mutex
	pending             map[string]chan rpcExtensionResponse
}

func (p *RPCLineProcessor) HandleLine(ctx context.Context, line string) {
	var envelope struct {
		Type     string `json:"type"`
		Protocol string `json:"protocol,omitempty"`
	}
	if err := json.Unmarshal([]byte(line), &envelope); err == nil {
		switch {
		case envelope.Type == "hello":
			p.handleExtensionHello(line)
			return
		case envelope.Type == "response" && envelope.Protocol == "gi-ext-rpc@1":
			p.handleExtensionResponse(line)
			return
		case envelope.Type == "request" && envelope.Protocol == "gi-ext-rpc@1":
			var request HostActionRequest
			if err := json.Unmarshal([]byte(line), &request); err != nil {
				p.writeJSON(hostActionErrorResponse("parse", "gi-ext-rpc@1", "parse", "invalid_params", "Failed to parse host action: "+err.Error()))
				return
			}
			if isExtensionRegistrationMethod(request.Method) {
				p.writeJSON(p.handleExtensionRegistration(request))
				return
			}
			if authError := p.authorizeHostAction(request); authError != nil {
				p.writeJSON(hostActionErrorResponse(request.ID, request.Protocol, request.Method, authError.Code, authError.Message))
				return
			}
			if p.Host == nil {
				p.writeJSON(hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "no active RPC host"))
				return
			}
			p.notifyBeforeHostAction(request)
			response := p.Host.HandleHostActionContext(ctx, request)
			p.notifyHostAction(request, response)
			p.writeJSON(response)
			return
		case envelope.Type == "notification" && envelope.Protocol == "gi-ext-rpc@1":
			p.handleExtensionNotification(line)
			return
		case envelope.Type == "diagnostic" && envelope.Protocol == "gi-ext-rpc@1":
			p.handleExtensionDiagnostic(line)
			return
		}
	}
	var command RPCCommand
	if err := json.Unmarshal([]byte(line), &command); err != nil {
		p.writeResponse(rpcErrorResponse("parse", errors.New("Failed to parse command: "+err.Error())))
		return
	}
	if command.Type == "extension_ui_response" {
		return
	}
	response := p.handleCommand(ctx, command)
	response.ID = command.ID
	p.writeResponse(response)
}

func isExtensionRegistrationMethod(method string) bool {
	switch strings.TrimSpace(method) {
	case "register_command", "register_provider", "unregister_provider", "register_tool", "register_message_renderer", "register_tool_renderer", "register_autocomplete_provider", "register_shortcut", "register_flag", "get_flag":
		return true
	default:
		return false
	}
}

func (p *RPCLineProcessor) handleExtensionRegistration(request HostActionRequest) HostActionResponse {
	switch request.Method {
	case "register_command":
		return p.handleRegisterCommand(request)
	case "register_provider":
		return p.handleRegisterProvider(request)
	case "unregister_provider":
		return p.handleUnregisterProvider(request)
	case "register_tool":
		return p.handleRegisterTool(request)
	case "register_message_renderer":
		return p.handleRegisterMessageRenderer(request)
	case "register_tool_renderer":
		return p.handleRegisterToolRenderer(request)
	case "register_autocomplete_provider":
		return p.handleRegisterAutocompleteProvider(request)
	case "register_shortcut":
		return p.handleRegisterShortcut(request)
	case "register_flag":
		return p.handleRegisterFlag(request)
	case "get_flag":
		return p.handleGetFlag(request)
	default:
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "unsupported_method", "unsupported extension registration method")
	}
}

func (p *RPCLineProcessor) handleRegisterCommand(request HostActionRequest) HostActionResponse {
	if !p.capabilityGranted(CapabilityCommandsRegister) {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "missing_capability", "missing capability for "+request.Method)
	}
	runtime := p.extensionRuntime()
	if runtime == nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "extension runtime is unavailable")
	}
	var params struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		ArgumentHint string `json:"argumentHint"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "Failed to parse register_command: "+err.Error())
	}
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "command name is required")
	}
	source := p.SourceInfo
	if source.Path == "" {
		source = ProtocolSourceInfo{Path: "<rpc:" + name + ">", Source: "rpc", Scope: "temporary", Origin: "process"}
	}
	extensionContext := &ProtocolExtensionContext{runtime: runtime, source: source}
	err := extensionContext.RegisterCommand(name, ProtocolCommandDefinition{
		Description:  params.Description,
		ArgumentHint: params.ArgumentHint,
		Handler: func(args string) error {
			return p.writeExtensionEvent("command.invoke", map[string]any{
				"name": name,
				"args": args,
			})
		},
	})
	if err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, protocolErrorCode(err), err.Error())
	}
	return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"registered": true, "name": name})
}

func (p *RPCLineProcessor) handleRegisterProvider(request HostActionRequest) HostActionResponse {
	if !p.capabilityGranted(CapabilityProvidersRegister) {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "missing_capability", "missing capability for "+request.Method)
	}
	runtime := p.extensionRuntime()
	if runtime == nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "extension runtime is unavailable")
	}
	var params struct {
		Name       string                    `json:"name"`
		Provider   string                    `json:"provider"`
		Config     *ProtocolProviderOverride `json:"config"`
		BaseURL    string                    `json:"baseUrl"`
		APIKey     string                    `json:"apiKey"`
		API        string                    `json:"api"`
		Headers    map[string]string         `json:"headers"`
		AuthHeader *bool                     `json:"authHeader"`
		Compat     llm.ModelCompat           `json:"compat"`
		Models     []ProviderModelDefinition `json:"models"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "Failed to parse register_provider: "+err.Error())
	}
	name := strings.TrimSpace(firstNonEmptyString(params.Name, params.Provider))
	if name == "" {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "provider name is required")
	}
	override := ProtocolProviderOverride{}
	if params.Config != nil {
		override = *params.Config
	}
	if override.BaseURL == "" {
		override.BaseURL = params.BaseURL
	}
	if override.APIKey == "" {
		override.APIKey = params.APIKey
	}
	if override.API == "" {
		override.API = params.API
	}
	if override.Headers == nil && params.Headers != nil {
		override.Headers = cloneOptionalStringMap(params.Headers)
	}
	if override.AuthHeader == nil && params.AuthHeader != nil {
		authHeader := *params.AuthHeader
		override.AuthHeader = &authHeader
	}
	if override.Models == nil && params.Models != nil {
		override.Models = cloneProviderModelDefinitions(params.Models)
	}
	if !reflect.ValueOf(params.Compat).IsZero() && reflect.ValueOf(override.Compat).IsZero() {
		override.Compat = params.Compat
	}
	source := p.SourceInfo
	if source.Path == "" {
		source = ProtocolSourceInfo{Path: "<rpc:" + name + ">", Source: "rpc", Scope: "temporary", Origin: "process"}
	}
	extensionContext := &ProtocolExtensionContext{runtime: runtime, source: source}
	if err := extensionContext.RegisterProvider(name, override); err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, protocolErrorCode(err), err.Error())
	}
	return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"registered": true, "name": name})
}

func (p *RPCLineProcessor) handleUnregisterProvider(request HostActionRequest) HostActionResponse {
	if !p.capabilityGranted(CapabilityProvidersRegister) {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "missing_capability", "missing capability for "+request.Method)
	}
	runtime := p.extensionRuntime()
	if runtime == nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "extension runtime is unavailable")
	}
	var params struct {
		Name     string `json:"name"`
		Provider string `json:"provider"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "Failed to parse unregister_provider: "+err.Error())
	}
	name := strings.TrimSpace(firstNonEmptyString(params.Name, params.Provider))
	if name == "" {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "provider name is required")
	}
	source := p.SourceInfo
	if source.Path == "" {
		source = ProtocolSourceInfo{Path: "<rpc:" + name + ">", Source: "rpc", Scope: "temporary", Origin: "process"}
	}
	extensionContext := &ProtocolExtensionContext{runtime: runtime, source: source}
	if err := extensionContext.UnregisterProvider(name); err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, protocolErrorCode(err), err.Error())
	}
	return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"unregistered": true, "name": name})
}

func (p *RPCLineProcessor) handleRegisterAutocompleteProvider(request HostActionRequest) HostActionResponse {
	if !p.capabilityGranted(CapabilityTUIAutocomplete) {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "missing_capability", "missing capability for "+request.Method)
	}
	runtime := p.extensionRuntime()
	if runtime == nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "extension runtime is unavailable")
	}
	var params struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		Priority    int    `json:"priority"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "Failed to parse register_autocomplete_provider: "+err.Error())
	}
	id := strings.TrimSpace(params.ID)
	if id == "" {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "autocomplete provider id is required")
	}
	source := p.SourceInfo
	if source.Path == "" {
		source = ProtocolSourceInfo{Path: "<rpc:" + id + ">", Source: "rpc", Scope: "temporary", Origin: "process"}
	}
	extensionContext := &ProtocolExtensionContext{runtime: runtime, source: source}
	err := extensionContext.RegisterAutocompleteProvider(id, ProtocolAutocompleteProviderDefinition{
		Description: params.Description,
		Priority:    params.Priority,
		Handler: func(ctx context.Context, request ProtocolAutocompleteRequest) (ProtocolAutocompleteResult, error) {
			result, err := p.callExtension(ctx, "autocomplete.suggest", map[string]any{
				"text":          request.Text,
				"lines":         request.Lines,
				"cursorLine":    request.CursorLine,
				"cursorCol":     request.CursorCol,
				"force":         request.Force,
				"trigger":       request.Trigger,
				"slashCommand":  request.SlashCommand,
				"argumentIndex": request.ArgumentIndex,
			})
			if err != nil {
				return ProtocolAutocompleteResult{}, err
			}
			return decodeExtensionAutocompleteResult(result)
		},
	})
	if err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, protocolErrorCode(err), err.Error())
	}
	return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"registered": true, "id": id})
}

func decodeExtensionAutocompleteResult(raw json.RawMessage) (ProtocolAutocompleteResult, error) {
	if len(raw) == 0 {
		return ProtocolAutocompleteResult{}, nil
	}
	var result ProtocolAutocompleteResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return ProtocolAutocompleteResult{}, err
	}
	return result, nil
}

func (p *RPCLineProcessor) handleRegisterShortcut(request HostActionRequest) HostActionResponse {
	if !p.capabilityGranted(CapabilityShortcutsRegister) {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "missing_capability", "missing capability for "+request.Method)
	}
	runtime := p.extensionRuntime()
	if runtime == nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "extension runtime is unavailable")
	}
	var params struct {
		Key         string `json:"key"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "Failed to parse register_shortcut: "+err.Error())
	}
	key := strings.TrimSpace(params.Key)
	if key == "" {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "shortcut key is required")
	}
	source := p.SourceInfo
	if source.Path == "" {
		source = ProtocolSourceInfo{Path: "<rpc:" + key + ">", Source: "rpc", Scope: "temporary", Origin: "process"}
	}
	extensionContext := &ProtocolExtensionContext{runtime: runtime, source: source}
	err := extensionContext.RegisterShortcut(key, ProtocolShortcutDefinition{
		Description: params.Description,
		Handler: func() error {
			return p.writeExtensionEvent("shortcut.invoke", map[string]any{
				"key":         key,
				"description": params.Description,
			})
		},
	})
	if err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, protocolErrorCode(err), err.Error())
	}
	return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"registered": true, "key": key})
}

func (p *RPCLineProcessor) handleRegisterFlag(request HostActionRequest) HostActionResponse {
	runtime := p.extensionRuntime()
	if runtime == nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "extension runtime is unavailable")
	}
	var params struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Default     any    `json:"default"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "Failed to parse register_flag: "+err.Error())
	}
	name := strings.TrimSpace(strings.TrimPrefix(params.Name, "--"))
	if name == "" {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "flag name is required")
	}
	source := p.SourceInfo
	if source.Path == "" {
		source = ProtocolSourceInfo{Path: "<rpc:" + name + ">", Source: "rpc", Scope: "temporary", Origin: "process"}
	}
	extensionContext := &ProtocolExtensionContext{runtime: runtime, source: source}
	if err := extensionContext.RegisterFlag(name, ProtocolFlagDefinition{Description: params.Description, Type: params.Type, Default: params.Default}); err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, protocolErrorCode(err), err.Error())
	}
	return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"registered": true, "name": name})
}

func (p *RPCLineProcessor) handleGetFlag(request HostActionRequest) HostActionResponse {
	runtime := p.extensionRuntime()
	if runtime == nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "extension runtime is unavailable")
	}
	var params struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "Failed to parse get_flag: "+err.Error())
	}
	name := normalizeProtocolFlagName(params.Name)
	if name == "" {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "flag name is required")
	}
	source := p.SourceInfo
	if source.Path == "" {
		source = ProtocolSourceInfo{Path: "<rpc:" + name + ">", Source: "rpc", Scope: "temporary", Origin: "process"}
	}
	extensionContext := &ProtocolExtensionContext{runtime: runtime, source: source}
	value := extensionContext.GetFlag(name)
	return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{
		"name":  name,
		"value": value,
		"set":   value != nil,
	})
}

func (p *RPCLineProcessor) handleRegisterTool(request HostActionRequest) HostActionResponse {
	if !p.capabilityGranted(CapabilityToolsRegister) {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "missing_capability", "missing capability for "+request.Method)
	}
	runtime := p.extensionRuntime()
	if runtime == nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "extension runtime is unavailable")
	}
	var params struct {
		Name             string     `json:"name"`
		Label            string     `json:"label"`
		Description      string     `json:"description"`
		Parameters       llm.Schema `json:"parameters"`
		PromptSnippet    string     `json:"promptSnippet"`
		PromptGuidelines []string   `json:"promptGuidelines"`
		ExecutionMode    string     `json:"executionMode"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "Failed to parse register_tool: "+err.Error())
	}
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "tool name is required")
	}
	source := p.SourceInfo
	if source.Path == "" {
		source = ProtocolSourceInfo{Path: "<rpc:" + name + ">", Source: "rpc", Scope: "temporary", Origin: "process"}
	}
	extensionContext := &ProtocolExtensionContext{runtime: runtime, source: source}
	err := extensionContext.RegisterTool(ProtocolToolDefinition{
		Name:             name,
		Label:            params.Label,
		Description:      params.Description,
		Parameters:       params.Parameters,
		PromptSnippet:    params.PromptSnippet,
		PromptGuidelines: append([]string(nil), params.PromptGuidelines...),
		ExecutionMode:    params.ExecutionMode,
		Execute: func(toolCallID string, input map[string]any) (SDKToolResult, error) {
			result, err := p.callExtension(context.Background(), "tool.invoke", map[string]any{
				"toolCallId": toolCallID,
				"name":       name,
				"toolName":   name,
				"input":      input,
				"context":    p.toolInvocationContext(),
			})
			if err != nil {
				return SDKToolResult{}, err
			}
			return decodeExtensionToolResult(result)
		},
	})
	if err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, protocolErrorCode(err), err.Error())
	}
	return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"registered": true, "name": name})
}

func (p *RPCLineProcessor) toolInvocationContext() map[string]any {
	context := map[string]any{}
	if p == nil {
		return context
	}
	if p.SourceInfo.Path != "" || p.SourceInfo.Source != "" || p.SourceInfo.Scope != "" || p.SourceInfo.Origin != "" {
		context["source"] = p.SourceInfo
	}
	if p.Host == nil || p.Host.Session == nil || p.Host.Session.SessionManager == nil {
		return context
	}
	manager := p.Host.Session.SessionManager
	context["cwd"] = manager.GetCWD()
	if sessionID := manager.GetSessionID(); sessionID != "" {
		context["sessionId"] = sessionID
	}
	if sessionFile := manager.GetSessionFile(); sessionFile != "" {
		context["sessionFile"] = sessionFile
	}
	return context
}

func (p *RPCLineProcessor) handleRegisterMessageRenderer(request HostActionRequest) HostActionResponse {
	if !p.capabilityGranted(CapabilityTUIMessageRenderer) {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "missing_capability", "missing capability for "+request.Method)
	}
	runtime := p.extensionRuntime()
	if runtime == nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "extension runtime is unavailable")
	}
	var params struct {
		Type       string `json:"type"`
		CustomType string `json:"customType"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "Failed to parse register_message_renderer: "+err.Error())
	}
	customType := strings.TrimSpace(firstNonEmptyString(params.Type, params.CustomType))
	if customType == "" {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "message renderer type is required")
	}
	source := p.SourceInfo
	if source.Path == "" {
		source = ProtocolSourceInfo{Path: "<rpc:" + customType + ">", Source: "rpc", Scope: "temporary", Origin: "process"}
	}
	extensionContext := &ProtocolExtensionContext{runtime: runtime, source: source}
	err := extensionContext.RegisterMessageRenderer(customType, func(message any, options any) []string {
		result, err := p.callExtension(context.Background(), "message.render", map[string]any{
			"type":    customType,
			"message": message,
			"options": options,
		})
		if err != nil {
			return nil
		}
		return decodeExtensionRenderedLines(result, options)
	})
	if err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, protocolErrorCode(err), err.Error())
	}
	return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"registered": true, "type": customType})
}

func (p *RPCLineProcessor) handleRegisterToolRenderer(request HostActionRequest) HostActionResponse {
	if !p.capabilityGranted(CapabilityTUIToolRenderer) {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "missing_capability", "missing capability for "+request.Method)
	}
	runtime := p.extensionRuntime()
	if runtime == nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "stale_context", "extension runtime is unavailable")
	}
	var params struct {
		Name         string `json:"name"`
		ToolName     string `json:"toolName"`
		RenderCall   *bool  `json:"renderCall"`
		RenderResult *bool  `json:"renderResult"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "Failed to parse register_tool_renderer: "+err.Error())
	}
	name := strings.TrimSpace(firstNonEmptyString(params.Name, params.ToolName))
	if name == "" {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, "invalid_params", "tool renderer name is required")
	}
	renderCall := params.RenderCall == nil || *params.RenderCall
	renderResult := params.RenderResult == nil || *params.RenderResult
	source := p.SourceInfo
	if source.Path == "" {
		source = ProtocolSourceInfo{Path: "<rpc:" + name + ">", Source: "rpc", Scope: "temporary", Origin: "process"}
	}
	extensionContext := &ProtocolExtensionContext{runtime: runtime, source: source}
	definition := ProtocolToolRendererDefinition{}
	if renderCall {
		definition.RenderCall = func(args any, renderContext ToolRenderContext) []string {
			result, err := p.callExtension(context.Background(), "tool.render_call", map[string]any{
				"name":    name,
				"args":    args,
				"context": toolRenderContextParams(renderContext),
			})
			if err != nil {
				return nil
			}
			return decodeExtensionRenderedLines(result, renderContext)
		}
	}
	if renderResult {
		definition.RenderResult = func(result FileToolResult, options ToolRenderResultOptions, renderContext ToolRenderContext) []string {
			rendered, err := p.callExtension(context.Background(), "tool.render_result", map[string]any{
				"name":    name,
				"result":  result,
				"options": options,
				"context": toolRenderContextParams(renderContext),
			})
			if err != nil {
				return nil
			}
			return decodeExtensionRenderedLines(rendered, renderContext)
		}
	}
	if err := extensionContext.RegisterToolRenderer(name, definition); err != nil {
		return hostActionErrorResponse(request.ID, request.Protocol, request.Method, protocolErrorCode(err), err.Error())
	}
	return hostActionSuccessResponse(request.ID, request.Protocol, map[string]any{"registered": true, "name": name})
}

func decodeExtensionToolResult(raw json.RawMessage) (SDKToolResult, error) {
	var result struct {
		Content []SDKContentPart `json:"content"`
		Details any              `json:"details"`
	}
	if len(raw) == 0 {
		return SDKToolResult{}, nil
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return SDKToolResult{}, err
	}
	return SDKToolResult{Content: result.Content, Details: result.Details}, nil
}

func decodeExtensionRenderedLines(raw json.RawMessage, options any) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var lines []string
	if err := json.Unmarshal(raw, &lines); err == nil {
		return normalizeRenderedLines(lines)
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return normalizeRenderedLines(strings.Split(text, "\n"))
	}
	var result struct {
		Lines []string      `json:"lines"`
		Text  string        `json:"text"`
		View  *ViewTreeNode `json:"view"`
		Node  *ViewTreeNode `json:"node"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil
	}
	if len(result.Lines) > 0 {
		return normalizeRenderedLines(result.Lines)
	}
	if result.Text != "" {
		return normalizeRenderedLines(strings.Split(result.Text, "\n"))
	}
	view := result.View
	if view == nil {
		view = result.Node
	}
	if view != nil {
		return normalizeRenderedLines(RenderViewTree(*view, renderWidthFromOptions(options)))
	}
	return nil
}

func renderWidthFromOptions(options any) int {
	switch typed := options.(type) {
	case map[string]any:
		if width := intFromAny(typed["width"]); width > 0 {
			return width
		}
	case map[string]int:
		if width := typed["width"]; width > 0 {
			return width
		}
	}
	return 80
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	default:
		return 0
	}
}

func toolRenderContextParams(context ToolRenderContext) map[string]any {
	return map[string]any{
		"args":             context.Args,
		"toolCallId":       context.ToolCallID,
		"state":            cloneMapAny(context.State),
		"cwd":              context.CWD,
		"argsComplete":     context.ArgsComplete,
		"isPartial":        context.IsPartial,
		"expanded":         context.Expanded,
		"showImages":       context.ShowImages,
		"isError":          context.IsError,
		"preflightDiff":    context.PreflightDiff,
		"executionStarted": context.ExecutionStarted,
	}
}

func (p *RPCLineProcessor) extensionRuntime() *ProtocolExtensionRuntime {
	if p == nil {
		return nil
	}
	if p.Runtime != nil {
		return p.Runtime
	}
	if p.Host != nil && p.Host.Session != nil {
		return p.Host.Session.ExtensionRuntime
	}
	return nil
}

func (p *RPCLineProcessor) callExtension(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if p == nil {
		return nil, ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension process is unavailable"}
	}
	id := p.nextExtensionRequestID(method)
	ch := make(chan rpcExtensionResponse, 1)
	p.pendingMu.Lock()
	if p.pending == nil {
		p.pending = map[string]chan rpcExtensionResponse{}
	}
	p.pending[id] = ch
	p.pendingMu.Unlock()
	defer func() {
		p.pendingMu.Lock()
		delete(p.pending, id)
		p.pendingMu.Unlock()
	}()
	p.writeExtensionEventWithID(id, method, params)
	timeout := p.RequestTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case response := <-ch:
		if response.Error != nil {
			return nil, errors.New(response.Error.Message)
		}
		return response.Result, nil
	case <-timer.C:
		return nil, errors.New("extension request timed out: " + method)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *RPCLineProcessor) nextExtensionRequestID(method string) string {
	p.eventMu.Lock()
	defer p.eventMu.Unlock()
	p.requestSeq++
	return fmt.Sprintf("%s_%d", strings.ReplaceAll(method, ".", "_"), p.requestSeq)
}

func (p *RPCLineProcessor) writeExtensionEvent(method string, params any) error {
	return p.writeExtensionEventWithID("", method, params)
}

func (p *RPCLineProcessor) writeExtensionEventWithID(id, method string, params any) error {
	if p == nil {
		return ProtocolRuntimeError{Code: "runtime_unavailable", Message: "extension process is unavailable"}
	}
	p.eventMu.Lock()
	p.eventSeq++
	seq := p.eventSeq
	p.eventMu.Unlock()
	event := map[string]any{
		"type":     "event",
		"protocol": "gi-ext-rpc@1",
		"eventSeq": seq,
		"method":   method,
		"params":   params,
	}
	if id != "" {
		event["id"] = id
	}
	p.writeJSON(event)
	return nil
}

type rpcExtensionResponse struct {
	Type     string             `json:"type"`
	Protocol string             `json:"protocol"`
	ID       any                `json:"id"`
	Result   json.RawMessage    `json:"result,omitempty"`
	Error    *rpcExtensionError `json:"error,omitempty"`
}

type rpcExtensionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type rpcExtensionDiagnosticMessage struct {
	Type     string `json:"type"`
	Protocol string `json:"protocol"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Stack    string `json:"stack,omitempty"`
}

func (p *RPCLineProcessor) handleExtensionResponse(line string) {
	var response rpcExtensionResponse
	if err := json.Unmarshal([]byte(line), &response); err != nil {
		return
	}
	key := fmt.Sprint(response.ID)
	p.pendingMu.Lock()
	ch := p.pending[key]
	p.pendingMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- response:
	default:
	}
}

func (p *RPCLineProcessor) handleExtensionDiagnostic(line string) {
	var diagnostic rpcExtensionDiagnosticMessage
	if err := json.Unmarshal([]byte(line), &diagnostic); err != nil {
		return
	}
	if p == nil || p.Runtime == nil || strings.TrimSpace(diagnostic.Severity) != "error" {
		return
	}
	message := strings.TrimSpace(diagnostic.Message)
	if message == "" {
		message = strings.TrimSpace(diagnostic.Code)
	}
	if message == "" {
		message = "Extension diagnostic"
	}
	p.Runtime.emitExtensionError(ProtocolExtensionError{
		ExtensionPath: p.SourceInfo.Path,
		Event:         strings.TrimSpace(diagnostic.Code),
		Error:         message,
		Stack:         diagnostic.Stack,
	})
}

func protocolErrorCode(err error) string {
	var runtimeErr ProtocolRuntimeError
	if errors.As(err, &runtimeErr) && runtimeErr.Code != "" {
		return runtimeErr.Code
	}
	return "extension_error"
}

type rpcExtensionHello struct {
	Type                  string               `json:"type"`
	Protocols             []string             `json:"protocols"`
	Extension             rpcExtensionIdentity `json:"extension"`
	RequestedCapabilities []string             `json:"requestedCapabilities"`
}

type rpcExtensionIdentity struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Version  string `json:"version"`
	Platform string `json:"platform,omitempty"`
}

type rpcExtensionHelloResult struct {
	Type                string               `json:"type"`
	SessionID           string               `json:"sessionId"`
	Protocols           map[string]string    `json:"protocols"`
	GrantedCapabilities []string             `json:"grantedCapabilities"`
	Host                rpcExtensionIdentity `json:"host"`
}

func (p *RPCLineProcessor) handleExtensionHello(line string) {
	var hello rpcExtensionHello
	if err := json.Unmarshal([]byte(line), &hello); err != nil {
		p.writeJSON(rpcExtensionDiagnostic("error", "invalid_params", "Failed to parse extension hello: "+err.Error()))
		return
	}
	protocols := supportedExtensionProtocolVersions(hello.Protocols)
	if len(protocols) == 0 {
		p.writeJSON(rpcExtensionDiagnostic("error", "unsupported_protocol", "Extension did not offer a compatible protocol"))
		return
	}
	p.writeJSON(rpcExtensionHelloResult{
		Type:                "hello_result",
		SessionID:           p.sessionID(),
		Protocols:           protocols,
		GrantedCapabilities: p.grantedExtensionCapabilities(hello.RequestedCapabilities),
		Host:                rpcExtensionIdentity{ID: "gi", Name: "gi", Version: "0.1.0"},
	})
}

func (p *RPCLineProcessor) handleExtensionNotification(line string) {
	var request HostActionRequest
	if err := json.Unmarshal([]byte(line), &request); err != nil {
		p.writeJSON(rpcExtensionDiagnostic("error", "invalid_params", "Failed to parse host action notification: "+err.Error()))
		return
	}
	if p.Host == nil {
		p.writeJSON(rpcExtensionDiagnostic("error", "stale_context", "No active RPC host"))
		return
	}
	request.Type = "request"
	if authError := p.authorizeHostAction(request); authError != nil {
		p.writeJSON(rpcExtensionDiagnostic("error", authError.Code, authError.Message))
		return
	}
	p.notifyBeforeHostAction(request)
	response := p.Host.HandleHostAction(request)
	p.notifyHostAction(request, response)
	if response.Error != nil {
		p.writeJSON(rpcExtensionDiagnostic("error", response.Error.Code, response.Error.Message))
	}
}

func (p *RPCLineProcessor) notifyBeforeHostAction(request HostActionRequest) {
	if p == nil || p.OnBeforeHostAction == nil {
		return
	}
	p.OnBeforeHostAction(request)
}

func (p *RPCLineProcessor) notifyHostAction(request HostActionRequest, response HostActionResponse) {
	if p == nil {
		return
	}
	if response.Error == nil {
		p.recordOwnedViewTreeHostAction(request)
		p.applyPolicyGrantHostAction(request, response)
	}
	if p.OnHostAction == nil {
		return
	}
	p.OnHostAction(request, response)
}

func (p *RPCLineProcessor) applyPolicyGrantHostAction(request HostActionRequest, response HostActionResponse) {
	if p == nil || strings.TrimSpace(request.Method) != "host.policy.request" {
		return
	}
	var decision HostPolicyDecision
	content, err := json.Marshal(response.Result)
	if err != nil {
		return
	}
	if err := json.Unmarshal(content, &decision); err != nil {
		return
	}
	if len(decision.GrantedCapabilities) == 0 {
		return
	}
	existing := stringSet(p.AllowedCapabilities)
	for _, capability := range cleanSupportedCapabilities(decision.GrantedCapabilities) {
		if existing[capability] {
			continue
		}
		existing[capability] = true
		p.AllowedCapabilities = append(p.AllowedCapabilities, capability)
	}
}

func (p *RPCLineProcessor) sessionID() string {
	if p != nil && p.Host != nil && p.Host.Session != nil && p.Host.Session.SessionManager != nil {
		return p.Host.Session.SessionManager.GetSessionID()
	}
	return "session"
}

func supportedExtensionProtocolVersions(protocols []string) map[string]string {
	result := map[string]string{}
	for _, protocol := range protocols {
		switch strings.TrimSpace(protocol) {
		case "gi-ext-rpc@1":
			result["gi-ext-rpc"] = "1.0.0"
		case "gi-viewtree@1":
			result["gi-viewtree"] = "1.0.0"
		}
	}
	return result
}

func grantedExtensionCapabilities(requested []string) []string {
	return grantExtensionCapabilities(requested, nil, false)
}

func (p *RPCLineProcessor) grantedExtensionCapabilities(requested []string) []string {
	if p == nil || !p.EnforceCapabilities {
		return grantExtensionCapabilities(requested, nil, false)
	}
	return grantExtensionCapabilities(requested, p.AllowedCapabilities, true)
}

func grantExtensionCapabilities(requested, allowed []string, enforce bool) []string {
	granted := make([]string, 0, len(requested))
	seen := map[string]bool{}
	for _, capability := range requested {
		capability = strings.TrimSpace(capability)
		if capability == "" || seen[capability] || !isSupportedExtensionCapability(capability) || !capabilityAllowed(capability, allowed, enforce) {
			continue
		}
		seen[capability] = true
		granted = append(granted, capability)
	}
	return granted
}

func (p *RPCLineProcessor) canCallHostAction(request HostActionRequest) bool {
	return p.authorizeHostAction(request) == nil
}

func (p *RPCLineProcessor) authorizeHostAction(request HostActionRequest) *HostActionError {
	if p == nil || !p.EnforceCapabilities {
		return nil
	}
	required := hostActionRequiredCapabilities(request)
	granted := len(required) == 0
	for _, capability := range required {
		if capabilityAllowed(capability, p.AllowedCapabilities, true) {
			granted = true
			break
		}
	}
	if !granted {
		return &HostActionError{Code: "missing_capability", Message: "missing capability for " + request.Method}
	}
	if strings.TrimSpace(request.Method) == "host.tui.mount" {
		mountID := hostActionMountParamsMountID(request)
		if mountID != "" && p.viewTreeMountExists(mountID) && !p.ownsViewTreeMount(mountID) {
			return &HostActionError{Code: "policy_denied", Message: "process does not own ViewTree mount " + mountID}
		}
	}
	if isOwnedViewTreeHostAction(request.Method) {
		mountID := hostActionViewTreeMountID(request)
		if mountID != "" && !p.ownsViewTreeMount(mountID) {
			return &HostActionError{Code: "policy_denied", Message: "process does not own ViewTree mount " + mountID}
		}
	}
	return nil
}

func (p *RPCLineProcessor) capabilityGranted(required string) bool {
	if p == nil || !p.EnforceCapabilities {
		return true
	}
	return capabilityAllowed(required, p.AllowedCapabilities, true)
}

func capabilityAllowed(required string, allowed []string, enforce bool) bool {
	if !enforce {
		return true
	}
	required = strings.TrimSpace(required)
	if required == "" {
		return true
	}
	for _, candidate := range allowed {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if candidate == required {
			return true
		}
		if strings.HasSuffix(candidate, ":") && strings.HasPrefix(required, candidate) {
			return true
		}
	}
	return false
}

func hostActionRequiredCapability(method string) string {
	switch strings.TrimSpace(method) {
	case "host.tools.list":
		return CapabilityToolsRegister
	case "host.tools.set_active":
		return "tools.set_active"
	case "host.commands.list":
		return CapabilityCommandsRegister
	case "host.session.entries", "host.thinking.get":
		return "session.read"
	case "host.session.append_custom", "host.session.set_label", "host.session.set_name", "host.session.action", "host.thinking.set":
		return "session.write"
	case "host.agent.send_user_message":
		return "agent.send_user_message"
	case "host.agent.run":
		return "agent.run"
	case "host.agent.spawn":
		return "agent.spawn"
	case "host.agent.abort":
		return "agent.abort"
	case "host.model.list", "host.model.select":
		return CapabilityProvidersRegister
	case "host.tui.dialog":
		return "tui.dialog"
	case "host.tui.editor":
		return "tui.editor"
	case "host.tui.status":
		return "tui.status"
	case "host.tui.title":
		return "tui.title"
	case "host.tui.working":
		return "tui.working"
	case "host.tui.thinking_label":
		return "tui.thinking_label"
	case "host.tui.theme":
		return CapabilityTUITheme
	case "host.tui.tools_expanded":
		return CapabilityTUIToolsExpanded
	case "host.process.exec":
		return "process.exec:"
	case "host.fs.read":
		return "fs.read:"
	case "host.fs.write":
		return "fs.write:"
	default:
		return ""
	}
}

func hostActionRequiredCapabilities(request HostActionRequest) []string {
	method := strings.TrimSpace(request.Method)
	switch method {
	case "host.tui.mount":
		var params hostTUIMountParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return []string{CapabilityTUIWidget}
		}
		return []string{viewTreeSlotCapability(params.Slot)}
	case "host.tui.patch", "host.tui.unmount":
		return []string{CapabilityTUIWidget, CapabilityTUIHeader, CapabilityTUIFooter, CapabilityTUIOverlay, CapabilityTUIEditor}
	case "host.process.exec":
		return []string{hostProcessExecCapability(request.Params)}
	default:
		required := hostActionRequiredCapability(method)
		if required == "" {
			return nil
		}
		return []string{required}
	}
}

func hostProcessExecCapability(params json.RawMessage) string {
	var parsed hostProcessExecParams
	if err := json.Unmarshal(params, &parsed); err != nil || len(parsed.Command) == 0 {
		return "process.exec:"
	}
	command := strings.TrimSpace(parsed.Command[0])
	if command == "" {
		return "process.exec:"
	}
	command = strings.TrimSuffix(filepath.Base(command), ".exe")
	if command == "" || command == "." || command == string(filepath.Separator) {
		return "process.exec:"
	}
	return "process.exec:" + command
}

func viewTreeSlotCapability(slot string) string {
	switch canonicalViewTreeSlot(slot) {
	case "header":
		return CapabilityTUIHeader
	case "footer":
		return CapabilityTUIFooter
	case "overlay":
		return CapabilityTUIOverlay
	case "editor":
		return CapabilityTUIEditor
	default:
		return CapabilityTUIWidget
	}
}

func hostActionMountParamsMountID(request HostActionRequest) string {
	var params hostTUIMountParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return ""
	}
	return strings.TrimSpace(params.MountID)
}

func isOwnedViewTreeHostAction(method string) bool {
	switch strings.TrimSpace(method) {
	case "host.tui.patch", "host.tui.unmount":
		return true
	default:
		return false
	}
}

func hostActionViewTreeMountID(request HostActionRequest) string {
	switch strings.TrimSpace(request.Method) {
	case "host.tui.patch":
		var params hostTUIPatchParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return ""
		}
		return strings.TrimSpace(params.MountID)
	case "host.tui.unmount":
		var params hostTUIUnmountParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return ""
		}
		return strings.TrimSpace(params.MountID)
	default:
		return ""
	}
}

func (p *RPCLineProcessor) recordOwnedViewTreeHostAction(request HostActionRequest) {
	if p == nil {
		return
	}
	switch strings.TrimSpace(request.Method) {
	case "host.tui.mount":
		var params hostTUIMountParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return
		}
		p.setOwnedViewTreeMount(params.MountID, true)
	case "host.tui.unmount":
		var params hostTUIUnmountParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return
		}
		p.setOwnedViewTreeMount(params.MountID, false)
	}
}

func (p *RPCLineProcessor) setOwnedViewTreeMount(mountID string, owned bool) {
	if p == nil {
		return
	}
	mountID = strings.TrimSpace(mountID)
	if mountID == "" {
		return
	}
	p.ownedViewTreeMu.Lock()
	defer p.ownedViewTreeMu.Unlock()
	if p.ownedViewTreeMounts == nil {
		p.ownedViewTreeMounts = map[string]bool{}
	}
	if owned {
		p.ownedViewTreeMounts[mountID] = true
		return
	}
	delete(p.ownedViewTreeMounts, mountID)
}

func (p *RPCLineProcessor) ownsViewTreeMount(mountID string) bool {
	if p == nil {
		return false
	}
	mountID = strings.TrimSpace(mountID)
	if mountID == "" {
		return false
	}
	p.ownedViewTreeMu.Lock()
	defer p.ownedViewTreeMu.Unlock()
	return p.ownedViewTreeMounts[mountID]
}

func (p *RPCLineProcessor) viewTreeMountExists(mountID string) bool {
	if p == nil || p.Host == nil || p.Host.ViewTreeHost == nil {
		return false
	}
	_, ok := p.Host.ViewTreeHost.Mounted(strings.TrimSpace(mountID))
	return ok
}

func isSupportedExtensionCapability(capability string) bool {
	switch capability {
	case CapabilityCommandsRegister,
		CapabilityLifecycleEvents,
		CapabilityProvidersRegister,
		CapabilityToolsRegister,
		CapabilityInputEvents,
		CapabilityBashIntercept,
		CapabilityShortcutsRegister,
		CapabilitySystemPromptModify,
		CapabilityResourcesDiscover,
		CapabilityTUIAutocomplete,
		CapabilityTUIWidget,
		CapabilityTUIHeader,
		CapabilityTUIFooter,
		CapabilityTUIOverlay,
		CapabilityTUIEditor,
		CapabilityTUITheme,
		CapabilityTUIToolsExpanded,
		CapabilityTUITerminalInput,
		"tools.set_active",
		"session.read",
		"session.write",
		"agent.send_user_message",
		"agent.run",
		"agent.spawn",
		"agent.abort",
		"compaction.custom",
		"tui.status",
		"tui.title",
		"tui.working",
		"tui.thinking_label",
		"tui.dialog",
		"tui.message_renderer",
		"tui.tool_renderer":
		return true
	default:
		return strings.HasPrefix(capability, "fs.read:") ||
			strings.HasPrefix(capability, "fs.write:") ||
			strings.HasPrefix(capability, "process.exec:") ||
			strings.HasPrefix(capability, "process.stdio:") ||
			strings.HasPrefix(capability, "network:")
	}
}

func rpcExtensionDiagnostic(severity, code, message string) map[string]any {
	return map[string]any{
		"type":     "diagnostic",
		"protocol": "gi-ext-rpc@1",
		"severity": severity,
		"code":     code,
		"message":  message,
	}
}

func (p *RPCLineProcessor) WriteEvent(event AgentSessionEvent) {
	if p == nil {
		return
	}
	p.writeJSON(event)
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
	p.writeJSON(response)
}

func (p *RPCLineProcessor) writeJSON(value any) {
	if p == nil || p.WriteLine == nil {
		return
	}
	line, err := SerializeJSONLine(value)
	if err != nil {
		return
	}
	p.WriteLine(line)
}
