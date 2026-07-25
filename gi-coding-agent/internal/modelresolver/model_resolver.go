package modelresolver

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	internalcli "github.com/nowa/gi/gi-coding-agent/internal/cli"
	llm "github.com/nowa/gi/gi-llm-provider"
)

type ThinkingLevel = internalcli.ThinkingLevel

const DefaultThinkingLevel ThinkingLevel = ThinkingMedium

const (
	ThinkingOff     ThinkingLevel = internalcli.ThinkingOff
	ThinkingMinimal ThinkingLevel = internalcli.ThinkingMinimal
	ThinkingLow     ThinkingLevel = internalcli.ThinkingLow
	ThinkingMedium  ThinkingLevel = internalcli.ThinkingMedium
	ThinkingHigh    ThinkingLevel = internalcli.ThinkingHigh
	ThinkingXHigh   ThinkingLevel = internalcli.ThinkingXHigh
)

var DefaultModelPerProvider = map[string]string{
	"amazon-bedrock":         "us.anthropic.claude-opus-4-6-v1",
	"anthropic":              "claude-opus-4-7",
	"openai":                 "gpt-5.4",
	"azure-openai-responses": "gpt-5.4",
	"openai-codex":           "gpt-5.5",
	"deepseek":               "deepseek-v4-pro",
	"google":                 "gemini-3.1-pro-preview",
	"google-vertex":          "gemini-3.1-pro-preview",
	"github-copilot":         "gpt-5.4",
	"openrouter":             "moonshotai/kimi-k2.6",
	"vercel-ai-gateway":      "zai/glm-5.1",
	"xai":                    "grok-4.20-0309-reasoning",
	"groq":                   "openai/gpt-oss-120b",
	"cerebras":               "zai-glm-4.7",
	"zai":                    "glm-5.1",
	"mistral":                "devstral-medium-latest",
	"minimax":                "MiniMax-M2.7",
	"minimax-cn":             "MiniMax-M2.7",
	"moonshotai":             "kimi-k2.6",
	"moonshotai-cn":          "kimi-k2.6",
	"huggingface":            "moonshotai/Kimi-K2.6",
	"fireworks":              "accounts/fireworks/models/kimi-k2p6",
	"together":               "moonshotai/Kimi-K2.6",
	"opencode":               "kimi-k2.6",
	"opencode-go":            "kimi-k2.6",
	"kimi-coding":            "kimi-for-coding",
	"cloudflare-workers-ai":  "@cf/moonshotai/kimi-k2.6",
	"cloudflare-ai-gateway":  "workers-ai/@cf/moonshotai/kimi-k2.6",
	"xiaomi":                 "mimo-v2.5-pro",
	"xiaomi-token-plan-cn":   "mimo-v2.5-pro",
	"xiaomi-token-plan-ams":  "mimo-v2.5-pro",
	"xiaomi-token-plan-sgp":  "mimo-v2.5-pro",
}

var defaultModelProviderOrder = []string{
	"amazon-bedrock",
	"anthropic",
	"openai",
	"azure-openai-responses",
	"openai-codex",
	"deepseek",
	"google",
	"google-vertex",
	"github-copilot",
	"openrouter",
	"vercel-ai-gateway",
	"xai",
	"groq",
	"cerebras",
	"zai",
	"mistral",
	"minimax",
	"minimax-cn",
	"moonshotai",
	"moonshotai-cn",
	"huggingface",
	"fireworks",
	"together",
	"opencode",
	"opencode-go",
	"kimi-coding",
	"cloudflare-workers-ai",
	"cloudflare-ai-gateway",
	"xiaomi",
	"xiaomi-token-plan-cn",
	"xiaomi-token-plan-ams",
	"xiaomi-token-plan-sgp",
}

type AllModelRegistry interface {
	GetAll() []llm.Model
}

type CodingModelRegistry interface {
	AllModelRegistry
	GetAvailable() []llm.Model
	Find(provider, modelID string) (llm.Model, bool)
}

type ParsedModelResult struct {
	Model         *llm.Model
	ThinkingLevel ThinkingLevel
	Warning       string
}

type ModelPatternOptions struct {
	StrictInvalidThinkingLevel bool
}

type ResolveCLIModelOptions struct {
	CLIProvider              string
	CLIModel                 string
	ModelRegistry            AllModelRegistry
	NoModelsAvailableMessage string
}

type ResolveCLIModelResult struct {
	Model         *llm.Model
	ThinkingLevel ThinkingLevel
	Warning       string
	Error         string
}

type ScopedModel struct {
	Model         llm.Model
	ThinkingLevel ThinkingLevel
}

type ModelScopeDiagnosticType string

const ModelScopeDiagnosticWarning ModelScopeDiagnosticType = "warning"

type ModelScopeDiagnostic struct {
	Type    ModelScopeDiagnosticType
	Message string
	Pattern string
}

type ResolveModelScopeResult struct {
	ScopedModels []ScopedModel
	Diagnostics  []ModelScopeDiagnostic
}

type FindInitialModelOptions struct {
	CLIProvider              string
	CLIModel                 string
	ScopedModels             []ScopedModel
	IsContinuing             bool
	DefaultProvider          string
	DefaultModelID           string
	DefaultThinkingLevel     ThinkingLevel
	ModelRegistry            CodingModelRegistry
	NoModelsAvailableMessage string
}

type InitialModelResult struct {
	Model           *llm.Model
	ThinkingLevel   ThinkingLevel
	FallbackMessage string
	Error           string
}

var datedModelSuffix = regexp.MustCompile(`-\d{8}$`)

func IsAliasModelID(id string) bool {
	return strings.HasSuffix(id, "-latest") || !datedModelSuffix.MatchString(id)
}

func FindExactModelReferenceMatch(modelReference string, availableModels []llm.Model) *llm.Model {
	trimmed := strings.TrimSpace(modelReference)
	if trimmed == "" {
		return nil
	}
	normalized := strings.ToLower(trimmed)

	if model, ok := uniqueModelMatch(availableModels, func(model llm.Model) bool {
		return strings.ToLower(model.Provider+"/"+model.ID) == normalized
	}); ok {
		return modelPtr(model)
	}

	if provider, modelID, ok := strings.Cut(trimmed, "/"); ok {
		provider = strings.TrimSpace(provider)
		modelID = strings.TrimSpace(modelID)
		if provider != "" && modelID != "" {
			if model, ok := uniqueModelMatch(availableModels, func(model llm.Model) bool {
				return strings.EqualFold(model.Provider, provider) && strings.EqualFold(model.ID, modelID)
			}); ok {
				return modelPtr(model)
			}
		}
	}

	if model, ok := uniqueModelMatch(availableModels, func(model llm.Model) bool {
		return strings.ToLower(model.ID) == normalized
	}); ok {
		return modelPtr(model)
	}

	return nil
}

func ParseModelPattern(pattern string, availableModels []llm.Model, options ...ModelPatternOptions) ParsedModelResult {
	opts := ModelPatternOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	return parseModelPattern(pattern, availableModels, opts)
}

func ResolveModelScope(patterns []string, registry CodingModelRegistry) []ScopedModel {
	return ResolveModelScopeWithDiagnostics(patterns, registry).ScopedModels
}

func ResolveModelScopeWithDiagnostics(patterns []string, registry CodingModelRegistry) ResolveModelScopeResult {
	if registry == nil || len(patterns) == 0 {
		return ResolveModelScopeResult{}
	}
	availableModels := registry.GetAvailable()
	scopedModels := make([]ScopedModel, 0, len(patterns))
	diagnostics := make([]ModelScopeDiagnostic, 0)
	for _, rawPattern := range patterns {
		pattern := strings.TrimSpace(rawPattern)
		if pattern == "" {
			continue
		}
		if containsModelGlob(pattern) {
			globPattern, thinkingLevel := splitModelScopePattern(pattern)
			if exactMatch := FindExactModelReferenceMatch(globPattern, availableModels); exactMatch != nil {
				if !scopedModelExists(scopedModels, *exactMatch) {
					scopedModels = append(scopedModels, ScopedModel{
						Model:         *exactMatch,
						ThinkingLevel: thinkingLevel,
					})
				}
				continue
			}
			matched := false
			for _, model := range availableModels {
				if !modelScopeGlobMatches(globPattern, model) {
					continue
				}
				matched = true
				if !scopedModelExists(scopedModels, model) {
					scopedModels = append(scopedModels, ScopedModel{Model: model, ThinkingLevel: thinkingLevel})
				}
			}
			if !matched {
				diagnostics = append(diagnostics, noModelScopeMatchDiagnostic(rawPattern))
			}
			continue
		}
		parsed := ParseModelPattern(pattern, availableModels)
		if parsed.Warning != "" {
			diagnostics = append(diagnostics, ModelScopeDiagnostic{
				Type:    ModelScopeDiagnosticWarning,
				Message: parsed.Warning,
				Pattern: rawPattern,
			})
		}
		if parsed.Model == nil {
			diagnostics = append(diagnostics, noModelScopeMatchDiagnostic(rawPattern))
			continue
		}
		if scopedModelExists(scopedModels, *parsed.Model) {
			continue
		}
		scopedModels = append(scopedModels, ScopedModel{Model: *parsed.Model, ThinkingLevel: parsed.ThinkingLevel})
	}
	return ResolveModelScopeResult{
		ScopedModels: scopedModels,
		Diagnostics:  diagnostics,
	}
}

func noModelScopeMatchDiagnostic(pattern string) ModelScopeDiagnostic {
	return ModelScopeDiagnostic{
		Type:    ModelScopeDiagnosticWarning,
		Message: fmt.Sprintf("No models match pattern %q", pattern),
		Pattern: pattern,
	}
}

func containsModelGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

func splitModelScopePattern(pattern string) (string, ThinkingLevel) {
	colon := strings.LastIndex(pattern, ":")
	if colon == -1 {
		return pattern, ""
	}
	suffix := strings.TrimSpace(pattern[colon+1:])
	if !internalcli.IsValidThinkingLevel(suffix) {
		return pattern, ""
	}
	return strings.TrimSpace(pattern[:colon]), ThinkingLevel(suffix)
}

func modelScopeGlobMatches(pattern string, model llm.Model) bool {
	return modelScopeWildcardMatch(pattern, scopedModelFullID(model)) ||
		modelScopeWildcardMatch(pattern, model.ID)
}

func scopedModelFullID(model llm.Model) string {
	if model.Provider == "" {
		return model.ID
	}
	return model.Provider + "/" + model.ID
}

func modelScopeWildcardMatch(pattern, value string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	value = strings.ToLower(strings.TrimSpace(value))
	if pattern == "" {
		return false
	}
	matched, err := regexp.MatchString(modelScopeGlobRegexp(pattern), value)
	return err == nil && matched
}

func modelScopeGlobRegexp(pattern string) string {
	var builder strings.Builder
	builder.WriteString("^")
	for index := 0; index < len(pattern); index++ {
		switch pattern[index] {
		case '*':
			builder.WriteString(".*")
		case '?':
			builder.WriteByte('.')
		case '[':
			closing := strings.IndexByte(pattern[index+1:], ']')
			if closing >= 0 {
				end := index + 1 + closing
				builder.WriteString(pattern[index : end+1])
				index = end
			} else {
				builder.WriteString(regexp.QuoteMeta(pattern[index : index+1]))
			}
		default:
			builder.WriteString(regexp.QuoteMeta(pattern[index : index+1]))
		}
	}
	builder.WriteString("$")
	return builder.String()
}

func scopedModelExists(scopedModels []ScopedModel, model llm.Model) bool {
	for _, scoped := range scopedModels {
		if sameModel(scoped.Model, model) {
			return true
		}
	}
	return false
}

func sameModel(left, right llm.Model) bool {
	return strings.EqualFold(left.Provider, right.Provider) && strings.EqualFold(left.ID, right.ID)
}

func noModelsAvailableMessage(message string) string {
	if strings.TrimSpace(message) != "" {
		return message
	}
	return "No models available."
}

func ResolveCLIModel(options ResolveCLIModelOptions) ResolveCLIModelResult {
	if options.CLIModel == "" {
		return ResolveCLIModelResult{}
	}

	availableModels := options.ModelRegistry.GetAll()
	if len(availableModels) == 0 {
		return ResolveCLIModelResult{
			Error: noModelsAvailableMessage(options.NoModelsAvailableMessage),
		}
	}

	providerMap := map[string]string{}
	for _, model := range availableModels {
		providerMap[strings.ToLower(model.Provider)] = model.Provider
	}

	provider := ""
	if options.CLIProvider != "" {
		canonical, ok := providerMap[strings.ToLower(options.CLIProvider)]
		if !ok {
			return ResolveCLIModelResult{
				Error: `Unknown provider "` + options.CLIProvider + `". Use --list-models to see available providers/models.`,
			}
		}
		provider = canonical
	}

	pattern := options.CLIModel
	inferredProvider := false
	if provider == "" {
		if slash := strings.Index(options.CLIModel, "/"); slash != -1 {
			maybeProvider := options.CLIModel[:slash]
			if canonical, ok := providerMap[strings.ToLower(maybeProvider)]; ok {
				provider = canonical
				pattern = options.CLIModel[slash+1:]
				inferredProvider = true
			}
		}
	}

	if provider == "" {
		if exact := findFirstExactModel(options.CLIModel, availableModels); exact != nil {
			return ResolveCLIModelResult{Model: exact}
		}
	}

	if options.CLIProvider != "" && provider != "" {
		prefix := provider + "/"
		if strings.HasPrefix(strings.ToLower(options.CLIModel), strings.ToLower(prefix)) {
			pattern = options.CLIModel[len(prefix):]
		}
	}

	candidates := availableModels
	if provider != "" {
		candidates = filterModelsByProvider(availableModels, provider)
	}

	parsed := ParseModelPattern(pattern, candidates, ModelPatternOptions{StrictInvalidThinkingLevel: true})
	if parsed.Model != nil {
		return ResolveCLIModelResult{
			Model:         parsed.Model,
			ThinkingLevel: parsed.ThinkingLevel,
			Warning:       parsed.Warning,
		}
	}

	if inferredProvider {
		if exact := findFirstExactModel(options.CLIModel, availableModels); exact != nil {
			return ResolveCLIModelResult{Model: exact}
		}
		fallback := ParseModelPattern(options.CLIModel, availableModels, ModelPatternOptions{StrictInvalidThinkingLevel: true})
		if fallback.Model != nil {
			return ResolveCLIModelResult{
				Model:         fallback.Model,
				ThinkingLevel: fallback.ThinkingLevel,
				Warning:       fallback.Warning,
			}
		}
	}

	if provider != "" {
		if fallback := buildFallbackModel(provider, pattern, availableModels); fallback != nil {
			warning := `Model "` + pattern + `" not found for provider "` + provider + `". Using custom model id.`
			if parsed.Warning != "" {
				warning = parsed.Warning + " " + warning
			}
			return ResolveCLIModelResult{Model: fallback, Warning: warning}
		}
	}

	display := options.CLIModel
	if provider != "" {
		display = provider + "/" + pattern
	}
	return ResolveCLIModelResult{
		Warning: parsed.Warning,
		Error:   `Model "` + display + `" not found. Use --list-models to see available models.`,
	}
}

func FindInitialModel(options FindInitialModelOptions) InitialModelResult {
	thinkingLevel := DefaultThinkingLevel

	if options.CLIProvider != "" && options.CLIModel != "" {
		resolved := ResolveCLIModel(ResolveCLIModelOptions{
			CLIProvider:              options.CLIProvider,
			CLIModel:                 options.CLIModel,
			ModelRegistry:            options.ModelRegistry,
			NoModelsAvailableMessage: options.NoModelsAvailableMessage,
		})
		if resolved.Error != "" {
			return InitialModelResult{ThinkingLevel: DefaultThinkingLevel, Error: resolved.Error}
		}
		if resolved.Model != nil {
			return InitialModelResult{Model: resolved.Model, ThinkingLevel: DefaultThinkingLevel}
		}
	}

	if len(options.ScopedModels) > 0 && !options.IsContinuing {
		scoped := options.ScopedModels[0]
		level := scoped.ThinkingLevel
		if level == "" {
			level = options.DefaultThinkingLevel
		}
		if level == "" {
			level = DefaultThinkingLevel
		}
		return InitialModelResult{Model: modelPtr(scoped.Model), ThinkingLevel: level}
	}

	if options.DefaultProvider != "" && options.DefaultModelID != "" {
		if found, ok := options.ModelRegistry.Find(options.DefaultProvider, options.DefaultModelID); ok {
			if options.DefaultThinkingLevel != "" {
				thinkingLevel = options.DefaultThinkingLevel
			}
			return InitialModelResult{Model: modelPtr(found), ThinkingLevel: thinkingLevel}
		}
	}

	availableModels := options.ModelRegistry.GetAvailable()
	if len(availableModels) > 0 {
		for _, provider := range defaultModelProviderOrder {
			defaultID := DefaultModelPerProvider[provider]
			for _, model := range availableModels {
				if model.Provider == provider && model.ID == defaultID {
					return InitialModelResult{Model: modelPtr(model), ThinkingLevel: DefaultThinkingLevel}
				}
			}
		}
		return InitialModelResult{Model: modelPtr(availableModels[0]), ThinkingLevel: DefaultThinkingLevel}
	}

	return InitialModelResult{ThinkingLevel: DefaultThinkingLevel}
}

func parseModelPattern(pattern string, availableModels []llm.Model, options ModelPatternOptions) ParsedModelResult {
	if exact := tryMatchModel(pattern, availableModels); exact != nil {
		return ParsedModelResult{Model: exact}
	}

	lastColon := strings.LastIndex(pattern, ":")
	if lastColon == -1 {
		return ParsedModelResult{}
	}

	prefix := pattern[:lastColon]
	suffix := pattern[lastColon+1:]

	if internalcli.IsValidThinkingLevel(suffix) {
		result := parseModelPattern(prefix, availableModels, options)
		if result.Model != nil {
			if result.Warning != "" {
				return ParsedModelResult{Model: result.Model, Warning: result.Warning}
			}
			return ParsedModelResult{Model: result.Model, ThinkingLevel: ThinkingLevel(suffix)}
		}
		return result
	}

	if options.StrictInvalidThinkingLevel {
		return ParsedModelResult{}
	}

	result := parseModelPattern(prefix, availableModels, options)
	if result.Model != nil {
		return ParsedModelResult{
			Model:   result.Model,
			Warning: `Invalid thinking level "` + suffix + `" in pattern "` + pattern + `". Using default instead.`,
		}
	}
	return result
}

func tryMatchModel(modelPattern string, availableModels []llm.Model) *llm.Model {
	if exact := FindExactModelReferenceMatch(modelPattern, availableModels); exact != nil {
		return exact
	}

	needle := strings.ToLower(modelPattern)
	matches := make([]llm.Model, 0)
	for _, model := range availableModels {
		if strings.Contains(strings.ToLower(model.ID), needle) || strings.Contains(strings.ToLower(model.Name), needle) {
			matches = append(matches, model)
		}
	}
	if len(matches) == 0 {
		return nil
	}

	aliases := make([]llm.Model, 0)
	dated := make([]llm.Model, 0)
	for _, model := range matches {
		if IsAliasModelID(model.ID) {
			aliases = append(aliases, model)
		} else {
			dated = append(dated, model)
		}
	}
	if len(aliases) > 0 {
		sort.Slice(aliases, func(i, j int) bool { return aliases[i].ID > aliases[j].ID })
		return modelPtr(aliases[0])
	}
	sort.Slice(dated, func(i, j int) bool { return dated[i].ID > dated[j].ID })
	return modelPtr(dated[0])
}

func buildFallbackModel(provider, modelID string, availableModels []llm.Model) *llm.Model {
	providerModels := filterModelsByProvider(availableModels, provider)
	if len(providerModels) == 0 {
		return nil
	}

	base := providerModels[0]
	if defaultID := DefaultModelPerProvider[provider]; defaultID != "" {
		for _, model := range providerModels {
			if model.ID == defaultID {
				base = model
				break
			}
		}
	}
	base.ID = modelID
	base.Name = modelID
	return modelPtr(base)
}

func filterModelsByProvider(models []llm.Model, provider string) []llm.Model {
	filtered := make([]llm.Model, 0, len(models))
	for _, model := range models {
		if model.Provider == provider {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func findFirstExactModel(reference string, models []llm.Model) *llm.Model {
	normalized := strings.ToLower(reference)
	for _, model := range models {
		if strings.ToLower(model.ID) == normalized || strings.ToLower(model.Provider+"/"+model.ID) == normalized {
			return modelPtr(model)
		}
	}
	return nil
}

func uniqueModelMatch(models []llm.Model, match func(llm.Model) bool) (llm.Model, bool) {
	var found llm.Model
	count := 0
	for _, model := range models {
		if match(model) {
			found = model
			count++
		}
	}
	return found, count == 1
}

func modelPtr(model llm.Model) *llm.Model {
	copy := model
	return &copy
}
