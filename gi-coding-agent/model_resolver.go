package gicodingagent

import (
	modelresolver "github.com/nowa/gi/gi-coding-agent/internal/modelresolver"
	llm "github.com/nowa/gi/gi-llm-provider"
)

const DefaultThinkingLevel ThinkingLevel = modelresolver.DefaultThinkingLevel

var DefaultModelPerProvider = modelresolver.DefaultModelPerProvider

type AllModelRegistry = modelresolver.AllModelRegistry
type CodingModelRegistry = modelresolver.CodingModelRegistry
type ParsedModelResult = modelresolver.ParsedModelResult
type ModelPatternOptions = modelresolver.ModelPatternOptions
type ResolveCLIModelOptions = modelresolver.ResolveCLIModelOptions
type ResolveCLIModelResult = modelresolver.ResolveCLIModelResult
type ScopedModel = modelresolver.ScopedModel
type FindInitialModelOptions = modelresolver.FindInitialModelOptions
type InitialModelResult = modelresolver.InitialModelResult

func IsAliasModelID(id string) bool {
	return modelresolver.IsAliasModelID(id)
}

func FindExactModelReferenceMatch(modelReference string, availableModels []llm.Model) *llm.Model {
	return modelresolver.FindExactModelReferenceMatch(modelReference, availableModels)
}

func ParseModelPattern(pattern string, availableModels []llm.Model, options ...ModelPatternOptions) ParsedModelResult {
	return modelresolver.ParseModelPattern(pattern, availableModels, options...)
}

func ResolveModelScope(patterns []string, registry CodingModelRegistry) []ScopedModel {
	return modelresolver.ResolveModelScope(patterns, registry)
}

func ResolveCLIModel(options ResolveCLIModelOptions) ResolveCLIModelResult {
	syncModelResolverDefaults()
	if options.NoModelsAvailableMessage == "" {
		options.NoModelsAvailableMessage = formatNoModelsAvailableMessage()
	}
	return modelresolver.ResolveCLIModel(options)
}

func FindInitialModel(options FindInitialModelOptions) InitialModelResult {
	syncModelResolverDefaults()
	if options.NoModelsAvailableMessage == "" {
		options.NoModelsAvailableMessage = formatNoModelsAvailableMessage()
	}
	return modelresolver.FindInitialModel(options)
}

func syncModelResolverDefaults() {
	modelresolver.DefaultModelPerProvider = DefaultModelPerProvider
}

func modelPtr(model llm.Model) *llm.Model {
	copy := model
	return &copy
}
