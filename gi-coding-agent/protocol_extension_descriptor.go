package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	llm "github.com/nowa/gi/gi-llm-provider"
)

type protocolExtensionDescriptor struct {
	Gi *protocolExtensionDescriptorGI `json:"gi"`
}

type protocolExtensionDescriptorGI struct {
	ExtensionProtocol string                              `json:"extensionProtocol"`
	ID                string                              `json:"id"`
	InitError         string                              `json:"initError"`
	Capabilities      []string                            `json:"capabilities"`
	Commands          []protocolCommandDescriptor         `json:"commands"`
	Tools             []protocolToolDescriptor            `json:"tools"`
	MessageRenderers  []protocolMessageRendererDescriptor `json:"messageRenderers"`
	ViewTrees         []protocolViewTreeDescriptor        `json:"viewTrees"`
	Events            []string                            `json:"events"`
	Shortcuts         []protocolShortcutDescriptor        `json:"shortcuts"`
	Flags             []protocolFlagDescriptor            `json:"flags"`
	Resources         protocolResourceDescriptor          `json:"resources"`
}

type protocolCommandDescriptor struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	ArgumentHint string `json:"argumentHint"`
}

type protocolToolDescriptor struct {
	Name          string     `json:"name"`
	Label         string     `json:"label"`
	Description   string     `json:"description"`
	Parameters    llm.Schema `json:"parameters"`
	PromptSnippet string     `json:"promptSnippet"`
}

type protocolMessageRendererDescriptor struct {
	Type string `json:"type"`
}

type protocolViewTreeDescriptor struct {
	MountID  string       `json:"mountId"`
	Slot     string       `json:"slot"`
	Priority int          `json:"priority,omitempty"`
	View     ViewTreeNode `json:"view"`
}

type protocolShortcutDescriptor struct {
	Key         string `json:"key"`
	Description string `json:"description"`
}

type protocolFlagDescriptor struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Default     any    `json:"default"`
}

type protocolResourceDescriptor struct {
	Skills  []string `json:"skills"`
	Prompts []string `json:"prompts"`
	Themes  []string `json:"themes"`
}

type protocolExtensionDescriptorLoadResult struct {
	Errors    []ProtocolExtensionDiscoveryError
	Resources ResourceExtension
	Loaded    []ProtocolExtensionSource
}

func LoadProtocolExtensionDescriptors(sources []ProtocolExtensionSource, runtime *ProtocolExtensionRuntime) protocolExtensionDescriptorLoadResult {
	state := newProtocolExtensionLoadState(
		newProtocolExtensionLoader(),
		runtime,
		"",
		false,
	)
	result := state.loadExtensionsInternal(sources, "")
	result.Errors = append(
		result.Errors,
		state.conflictDiagnostics(sources, "")...,
	)
	return result
}

func readProtocolExtensionDescriptor(path string) (protocolExtensionDescriptor, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return protocolExtensionDescriptor{}, err
	}
	var descriptor protocolExtensionDescriptor
	if err := json.Unmarshal(content, &descriptor); err != nil {
		return protocolExtensionDescriptor{}, err
	}
	if descriptor.Gi == nil || strings.TrimSpace(descriptor.Gi.ExtensionProtocol) == "" {
		return protocolExtensionDescriptor{}, ProtocolRuntimeError{Code: "invalid_descriptor", Message: "descriptor does not declare gi.extensionProtocol"}
	}
	return descriptor, nil
}

func applyProtocolExtensionDescriptor(ctx *ProtocolExtensionContext, descriptor *protocolExtensionDescriptorGI) []ProtocolExtensionDiscoveryError {
	var errors []ProtocolExtensionDiscoveryError
	for _, command := range descriptor.Commands {
		if err := ctx.RegisterCommand(command.Name, ProtocolCommandDefinition{
			Description:  command.Description,
			ArgumentHint: command.ArgumentHint,
			Handler:      officialCommandHandler(ctx, command.Name),
		}); err != nil {
			errors = append(errors, ProtocolExtensionDiscoveryError{Path: ctx.source.Path, Error: err.Error()})
		}
	}
	for _, tool := range descriptor.Tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		if err := ctx.RegisterTool(ProtocolToolDefinition{
			Name:          name,
			Label:         tool.Label,
			Description:   tool.Description,
			Parameters:    tool.Parameters,
			PromptSnippet: firstNonEmptyString(tool.PromptSnippet, tool.Description),
			Execute:       officialToolExecutor(ctx, name),
		}); err != nil {
			errors = append(errors, ProtocolExtensionDiscoveryError{Path: ctx.source.Path, Error: err.Error()})
		}
	}
	for _, renderer := range descriptor.MessageRenderers {
		rendererType := strings.TrimSpace(renderer.Type)
		if rendererType == "" {
			continue
		}
		if err := ctx.RegisterMessageRenderer(rendererType, officialMessageRenderer(ctx, rendererType)); err != nil {
			errors = append(errors, ProtocolExtensionDiscoveryError{Path: ctx.source.Path, Error: err.Error()})
		}
	}
	for _, renderer := range officialPackageToolRenderers(ctx) {
		if err := ctx.RegisterToolRenderer(renderer.Name, renderer.Definition); err != nil {
			errors = append(errors, ProtocolExtensionDiscoveryError{Path: ctx.source.Path, Error: err.Error()})
		}
	}
	for _, mount := range descriptor.ViewTrees {
		if err := ctx.MountViewTree(mount.MountID, mount.Slot, mount.View, ViewTreeMountOptions{Priority: mount.Priority}); err != nil {
			errors = append(errors, ProtocolExtensionDiscoveryError{Path: ctx.source.Path, Error: err.Error()})
		}
	}
	for _, eventType := range descriptor.Events {
		eventType := strings.TrimSpace(eventType)
		if eventType == "" {
			continue
		}
		if err := ctx.On(eventType, func(ProtocolSessionEvent) (ProtocolEventResult, error) { return ProtocolEventResult{}, nil }); err != nil {
			errors = append(errors, ProtocolExtensionDiscoveryError{Path: ctx.source.Path, Error: err.Error()})
		}
	}
	for _, shortcut := range descriptor.Shortcuts {
		if err := ctx.RegisterShortcut(shortcut.Key, ProtocolShortcutDefinition{Description: shortcut.Description}); err != nil {
			errors = append(errors, ProtocolExtensionDiscoveryError{Path: ctx.source.Path, Error: err.Error()})
		}
	}
	for _, flag := range descriptor.Flags {
		name := normalizeProtocolFlagName(flag.Name)
		if name == "" {
			continue
		}
		if err := ctx.RegisterFlag(name, ProtocolFlagDefinition{Description: flag.Description, Type: flag.Type, Default: flag.Default}); err != nil {
			errors = append(errors, ProtocolExtensionDiscoveryError{Path: ctx.source.Path, Error: err.Error()})
		}
	}
	return errors
}

func protocolDescriptorSourceInfo(source ProtocolExtensionSource, id string) ProtocolSourceInfo {
	origin := strings.TrimSpace(id)
	if origin == "" {
		origin = strings.TrimSuffix(filepath.Base(source.Path), filepath.Ext(source.Path))
	}
	if source.Metadata.Source != "" || source.Metadata.Scope != "" || source.Metadata.Origin != "" {
		info := source.Metadata
		info.Path = source.Path
		if info.Source == "" {
			info.Source = "extension:" + origin
		}
		if info.Scope == "" {
			info.Scope = "temporary"
		}
		if info.Origin == "" {
			info.Origin = "top-level"
		}
		return info
	}
	return ProtocolSourceInfo{Path: source.Path, Source: "extension:" + origin, Scope: "temporary", Origin: "top-level"}
}

func protocolDescriptorResourcePaths(source ProtocolExtensionSource, paths []string, metadata ProtocolSourceInfo) []ResourceSkillPath {
	result := make([]ResourceSkillPath, 0, len(paths))
	for _, rawPath := range paths {
		path := ResolveToCwd(rawPath, source.BaseDir)
		info := metadata
		info.Path = path
		result = append(result, ResourceSkillPath{Path: path, Metadata: info})
	}
	return result
}

func protocolDescriptorPromptPaths(source ProtocolExtensionSource, paths []string, metadata ProtocolSourceInfo) []ResourcePromptPath {
	result := make([]ResourcePromptPath, 0, len(paths))
	for _, rawPath := range paths {
		path := ResolveToCwd(rawPath, source.BaseDir)
		info := metadata
		info.Path = path
		result = append(result, ResourcePromptPath{Path: path, Metadata: info})
	}
	return result
}

func protocolDescriptorThemePaths(source ProtocolExtensionSource, paths []string, metadata ProtocolSourceInfo) []ResourceThemePath {
	result := make([]ResourceThemePath, 0, len(paths))
	for _, rawPath := range paths {
		path := ResolveToCwd(rawPath, source.BaseDir)
		info := metadata
		info.Path = path
		result = append(result, ResourceThemePath{Path: path, Metadata: info})
	}
	return result
}

func NewDefaultProtocolExtensionRuntime() *ProtocolExtensionRuntime {
	return NewProtocolExtensionRuntime(
		CapabilityCommandsRegister,
		CapabilityLifecycleEvents,
		CapabilityProvidersRegister,
		CapabilityToolsRegister,
		CapabilityInputEvents,
		CapabilityBashIntercept,
		CapabilityShortcutsRegister,
		CapabilitySystemPromptModify,
		CapabilityTUIMessageRenderer,
		CapabilityTUIToolRenderer,
		CapabilityTUIAutocomplete,
		CapabilityTUIWidget,
		CapabilityTUIHeader,
		CapabilityTUIFooter,
		CapabilityTUIOverlay,
		CapabilityTUIEditor,
	)
}
