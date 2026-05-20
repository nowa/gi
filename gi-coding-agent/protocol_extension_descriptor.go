package gicodingagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type protocolExtensionDescriptor struct {
	Gi *protocolExtensionDescriptorGI `json:"gi"`
}

type protocolExtensionDescriptorGI struct {
	ExtensionProtocol string                              `json:"extensionProtocol"`
	ID                string                              `json:"id"`
	InitError         string                              `json:"initError"`
	Commands          []protocolCommandDescriptor         `json:"commands"`
	Tools             []protocolToolDescriptor            `json:"tools"`
	MessageRenderers  []protocolMessageRendererDescriptor `json:"messageRenderers"`
	Events            []string                            `json:"events"`
	Shortcuts         []protocolShortcutDescriptor        `json:"shortcuts"`
	Flags             []protocolFlagDescriptor            `json:"flags"`
	Resources         protocolResourceDescriptor          `json:"resources"`
}

type protocolCommandDescriptor struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type protocolToolDescriptor struct {
	Name          string `json:"name"`
	Label         string `json:"label"`
	Description   string `json:"description"`
	PromptSnippet string `json:"promptSnippet"`
}

type protocolMessageRendererDescriptor struct {
	Type string `json:"type"`
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
}

type protocolExtensionDescriptorLoadResult struct {
	Errors    []ProtocolExtensionDiscoveryError
	Resources ResourceExtension
}

func LoadProtocolExtensionDescriptors(sources []ProtocolExtensionSource, runtime *ProtocolExtensionRuntime) protocolExtensionDescriptorLoadResult {
	var result protocolExtensionDescriptorLoadResult
	if runtime == nil {
		runtime = NewDefaultProtocolExtensionRuntime()
	}
	toolSources := map[string]string{}
	for _, source := range sources {
		descriptor, err := readProtocolExtensionDescriptor(source.Path)
		if err != nil {
			result.Errors = append(result.Errors, ProtocolExtensionDiscoveryError{Path: source.Path, Error: err.Error()})
			continue
		}
		metadata := protocolDescriptorSourceInfo(source, descriptor.Gi.ID)
		if descriptor.Gi.InitError != "" {
			result.Errors = append(result.Errors, ProtocolExtensionDiscoveryError{Path: source.Path, Error: descriptor.Gi.InitError})
			continue
		}
		context := &ProtocolExtensionContext{runtime: runtime, source: metadata}
		result.Errors = append(result.Errors, applyProtocolExtensionDescriptor(context, descriptor.Gi, toolSources)...)
		result.Resources.SkillPaths = append(result.Resources.SkillPaths, protocolDescriptorResourcePaths(source, descriptor.Gi.Resources.Skills, metadata)...)
		result.Resources.PromptPaths = append(result.Resources.PromptPaths, protocolDescriptorPromptPaths(source, descriptor.Gi.Resources.Prompts, metadata)...)
	}
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

func applyProtocolExtensionDescriptor(ctx *ProtocolExtensionContext, descriptor *protocolExtensionDescriptorGI, toolSources map[string]string) []ProtocolExtensionDiscoveryError {
	var errors []ProtocolExtensionDiscoveryError
	for _, command := range descriptor.Commands {
		if err := ctx.RegisterCommand(command.Name, ProtocolCommandDefinition{Description: command.Description}); err != nil {
			errors = append(errors, ProtocolExtensionDiscoveryError{Path: ctx.source.Path, Error: err.Error()})
		}
	}
	for _, tool := range descriptor.Tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		if previous := toolSources[name]; previous != "" {
			errors = append(errors, ProtocolExtensionDiscoveryError{Path: ctx.source.Path, Error: "tool " + name + " conflicts with " + previous})
			continue
		}
		toolSources[name] = ctx.source.Path
		if err := ctx.RegisterTool(ProtocolToolDefinition{Name: name, Label: tool.Label, Description: tool.Description, PromptSnippet: firstNonEmptyString(tool.PromptSnippet, tool.Description)}); err != nil {
			errors = append(errors, ProtocolExtensionDiscoveryError{Path: ctx.source.Path, Error: err.Error()})
		}
	}
	for _, renderer := range descriptor.MessageRenderers {
		rendererType := strings.TrimSpace(renderer.Type)
		if rendererType == "" {
			continue
		}
		if err := ctx.RegisterMessageRenderer(rendererType, func(any, any) []string { return nil }); err != nil {
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
		if err := ctx.RegisterFlag(flag.Name, ProtocolFlagDefinition{Description: flag.Description, Type: flag.Type, Default: flag.Default}); err != nil {
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

func NewDefaultProtocolExtensionRuntime() *ProtocolExtensionRuntime {
	return NewProtocolExtensionRuntime(
		CapabilityCommandsRegister,
		CapabilityLifecycleEvents,
		CapabilityProvidersRegister,
		CapabilityToolsRegister,
		CapabilityInputEvents,
		CapabilityShortcutsRegister,
		CapabilitySystemPromptModify,
	)
}
