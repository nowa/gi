package gicodingagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

type AgentSessionOptions struct {
	CWD                  string
	AgentDir             string
	Model                llm.Model
	ThinkingLevel        string
	Preflight            AgentSessionPreflight
	SessionManager       *SessionManager
	ResourceLoader       AgentSessionResourceLoader
	CompactionSettings   *agentharness.CompactionSettings
	CompactionSummarizer AgentSessionCompactionSummarizer
	BranchSummarizer     AgentSessionBranchSummarizer
	RetrySettings        *AgentSessionRetrySettings
	AutoCompactionRunner AgentSessionAutoCompactionRunner
	AgentContinue        func() error
	Responder            AgentSessionResponder
	StreamResponder      AgentSessionStreamResponder
	ModelRuntime         *ModelRuntime
	CustomTools          []SDKTool
	ScopedModels         []ScopedModel
	Tools                []string
	ToolsSet             bool
	NoTools              string
}

type AgentSession struct {
	SessionManager       *SessionManager
	ResourceLoader       AgentSessionResourceLoader
	BaseSystemPrompt     string
	SystemPrompt         string
	Agent                *SDKAgent
	CompactionSettings   agentharness.CompactionSettings
	CompactionSummarizer AgentSessionCompactionSummarizer
	BranchSummarizer     AgentSessionBranchSummarizer
	RetrySettings        AgentSessionRetrySettings
	AutoCompactionRunner AgentSessionAutoCompactionRunner
	AgentContinue        func() error
	Responder            AgentSessionResponder
	StreamResponder      AgentSessionStreamResponder
	ModelRuntime         *ModelRuntime
	Preflight            AgentSessionPreflight
	ExtensionRuntime     *ProtocolExtensionRuntime
	DynamicTools         []SDKTool
	ScopedModels         []ScopedModel
	Tools                []string
	ToolsSet             bool
	NoTools              string
	SteeringMode         string
	FollowUpMode         string
	eventListeners       []AgentSessionEventListener
	lifecycle            agentSessionLifecycle
	overflowRecovered    bool
	agentQueuedMessages  []llm.Message
	steeringMessages     []string
	followUpMessages     []string
	steeringQueue        []QueuedUserMessage
	followUpQueue        []QueuedUserMessage
	pendingNextTurn      []QueuedCustomMessage
	pendingBashMessages  []map[string]any
}

type AgentSessionStats struct {
	Tokens       llm.Usage          `json:"tokens"`
	ContextUsage *AgentContextUsage `json:"contextUsage,omitempty"`
}

type AgentSessionPreflight func(model llm.Model) error

type AgentContextUsage struct {
	Tokens        *int     `json:"tokens"`
	ContextWindow int      `json:"contextWindow"`
	Percent       *float64 `json:"percent"`
}

type QueuedUserMessage struct {
	Text   string               `json:"text"`
	Images []llm.ContentPart    `json:"images,omitempty"`
	Custom *QueuedCustomMessage `json:"custom,omitempty"`
}

type QueuedCustomMessage struct {
	CustomType string `json:"customType"`
	Content    any    `json:"content,omitempty"`
	Display    bool   `json:"display,omitempty"`
	Details    any    `json:"details,omitempty"`
}

type SDKAgent struct {
	State SDKAgentState
}

type SDKAgentState struct {
	SystemPrompt  string
	Model         llm.Model
	ThinkingLevel string
	Tools         []SDKTool
}

type SDKTool struct {
	Name               string
	Label              string
	Description        string
	Parameters         llm.Schema
	PromptSnippet      string
	PromptGuidelines   []string
	ExecutionMode      string
	SourceInfo         ProtocolSourceInfo
	PrepareArguments   func(input map[string]any) map[string]any
	Execute            func(toolCallID string, input map[string]any) (SDKToolResult, error)
	ExecuteWithUpdates func(toolCallID string, input map[string]any, onUpdate func(SDKToolResult)) (SDKToolResult, error)
}

type SDKToolResult struct {
	Content []SDKContentPart
	Details any
}

type SDKContentPart struct {
	Type string
	Text string
}

type AgentSessionResourceLoader interface {
	GetSkills() AgentSessionSkillsResult
}

type AgentSessionPromptResourceLoader interface {
	GetPrompts() ResourcePromptsResult
}

type AgentSessionContextResourceLoader interface {
	GetAgentsFiles() ResourceAgentsFilesResult
}

type AgentSessionSystemPromptResourceLoader interface {
	GetSystemPrompt() string
	GetAppendSystemPrompt() string
}

type AgentSessionSkillsResult struct {
	Skills      []agentharness.Skill
	Diagnostics []agentharness.SkillDiagnostic
}

type DefaultAgentSessionResourceLoader struct {
	cwd      string
	agentDir string
}

func CreateAgentSession(options AgentSessionOptions) (*AgentSession, error) {
	sessionManager := options.SessionManager
	cwd := options.CWD
	if cwd == "" && sessionManager != nil {
		cwd = sessionManager.GetCwd()
	}
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	agentDir := options.AgentDir
	if agentDir == "" {
		agentDir = filepath.Join(cwd, ConfigDirName, "agent")
	}
	if sessionManager == nil {
		sessionDir := GetAgentDirSessionDir(cwd, agentDir)
		var err error
		sessionManager, err = CreateSessionManager(cwd, sessionDir)
		if err != nil {
			return nil, err
		}
	}
	resourceLoader := options.ResourceLoader
	if resourceLoader == nil {
		resourceLoader = NewDefaultAgentSessionResourceLoader(cwd, agentDir)
	}
	compactionSettings := agentharness.DefaultCompactionSettings
	if options.CompactionSettings != nil {
		compactionSettings = *options.CompactionSettings
	}
	compactionSummarizer := options.CompactionSummarizer
	if compactionSummarizer == nil {
		compactionSummarizer = DefaultAgentSessionCompactionSummarizer
	}
	branchSummarizer := options.BranchSummarizer
	if branchSummarizer == nil {
		branchSummarizer = DefaultAgentSessionBranchSummarizer
	}
	retrySettings := DefaultAgentSessionRetrySettings()
	if options.RetrySettings != nil {
		retrySettings = *options.RetrySettings
	}
	responder := options.Responder
	if responder == nil {
		responder = DefaultAgentSessionResponder
	}

	tools := defaultSDKTools(cwd)
	for _, tool := range options.CustomTools {
		if tool.SourceInfo.Path == "" {
			tool.SourceInfo = ProtocolSourceInfo{Path: "<sdk:" + tool.Name + ">", Source: "sdk", Scope: "temporary", Origin: "top-level"}
		}
		tools = append(tools, tool)
	}
	systemPrompt := buildAgentSessionSystemPrompt(cwd, resourceLoader, activeToolsForPolicy(tools, nil, options.Tools, options.ToolsSet, options.NoTools))
	thinkingLevel := options.ThinkingLevel
	if thinkingLevel == "" {
		thinkingLevel = string(DefaultThinkingLevel)
	}
	thinkingLevel = llm.ClampThinkingLevel(options.Model, thinkingLevel)
	agent := &SDKAgent{State: SDKAgentState{
		SystemPrompt:  systemPrompt,
		Model:         options.Model,
		ThinkingLevel: thinkingLevel,
		Tools:         tools,
	}}
	return &AgentSession{
		SessionManager:       sessionManager,
		ResourceLoader:       resourceLoader,
		BaseSystemPrompt:     systemPrompt,
		SystemPrompt:         systemPrompt,
		Agent:                agent,
		CompactionSettings:   compactionSettings,
		CompactionSummarizer: compactionSummarizer,
		BranchSummarizer:     branchSummarizer,
		RetrySettings:        retrySettings,
		AutoCompactionRunner: options.AutoCompactionRunner,
		AgentContinue:        options.AgentContinue,
		Responder:            responder,
		StreamResponder:      options.StreamResponder,
		ModelRuntime:         options.ModelRuntime,
		Preflight:            options.Preflight,
		ScopedModels:         append([]ScopedModel(nil), options.ScopedModels...),
		Tools:                append([]string(nil), options.Tools...),
		ToolsSet:             options.ToolsSet,
		NoTools:              options.NoTools,
		SteeringMode:         "one-at-a-time",
		FollowUpMode:         "one-at-a-time",
	}, nil
}

func (s *AgentSession) Dispose() {
	_ = s.Abort()
}

func (s *AgentSession) SetScopedModels(scopedModels []ScopedModel) {
	if s == nil {
		return
	}
	s.ScopedModels = append([]ScopedModel(nil), scopedModels...)
}

func (s *AgentSession) GetSessionStats() AgentSessionStats {
	branch := s.sessionBranch()
	return AgentSessionStats{
		Tokens:       aggregateSessionUsage(branch),
		ContextUsage: s.contextUsageFromBranch(branch),
	}
}

func (s *AgentSession) GetContextUsage() *AgentContextUsage {
	return s.contextUsageFromBranch(s.sessionBranch())
}

func (s *AgentSession) sessionBranch() []FileEntry {
	if s == nil || s.SessionManager == nil {
		return nil
	}
	return s.SessionManager.GetBranch()
}

func (s *AgentSession) contextUsageFromBranch(branch []FileEntry) *AgentContextUsage {
	if len(branch) == 0 {
		return nil
	}
	compactionIndex := lastSessionCompactionIndex(branch)
	startIndex := 0
	if compactionIndex >= 0 {
		startIndex = compactionIndex + 1
	}
	for index := len(branch) - 1; index >= startIndex; index-- {
		usage, ok := assistantEntryUsage(branch[index])
		if !ok {
			continue
		}
		tokens := usageTokenTotal(usage)
		return &AgentContextUsage{
			Tokens:        &tokens,
			ContextWindow: s.contextWindow(),
			Percent:       contextUsagePercent(tokens, s.contextWindow()),
		}
	}
	if compactionIndex >= 0 {
		return &AgentContextUsage{ContextWindow: s.contextWindow()}
	}
	return nil
}

func (s *AgentSession) contextWindow() int {
	if s == nil || s.Agent == nil {
		return 0
	}
	return s.Agent.State.Model.ContextWindow
}

func NewDefaultAgentSessionResourceLoader(cwd, agentDir string) *DefaultAgentSessionResourceLoader {
	return &DefaultAgentSessionResourceLoader{cwd: cwd, agentDir: agentDir}
}

func (l *DefaultAgentSessionResourceLoader) GetSkills() AgentSessionSkillsResult {
	paths := []string{
		filepath.Join(l.agentDir, "skills"),
		filepath.Join(l.cwd, ConfigDirName, "skills"),
	}
	seen := map[string]struct{}{}
	var result AgentSessionSkillsResult
	for _, path := range paths {
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		loaded := agentharness.LoadSkills(clean)
		result.Skills = append(result.Skills, loaded.Skills...)
		result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
	}
	return result
}

func (l *DefaultAgentSessionResourceLoader) GetPrompts() ResourcePromptsResult {
	agentPrompts := filepath.Join(l.agentDir, "prompts")
	projectPrompts := filepath.Join(l.cwd, ConfigDirName, "prompts")
	prompts := loadPromptTemplatesFromDir(agentPrompts, sourceInfoForPromptPath(agentPrompts, agentPrompts, projectPrompts))
	prompts = append(prompts, loadPromptTemplatesFromDir(projectPrompts, sourceInfoForPromptPath(projectPrompts, agentPrompts, projectPrompts))...)
	return ResourcePromptsResult{Prompts: dedupePromptsByName(prompts)}
}

func (l *DefaultAgentSessionResourceLoader) GetAgentsFiles() ResourceAgentsFilesResult {
	return ResourceAgentsFilesResult{AgentsFiles: loadProjectContextResourceFiles(l.cwd, l.agentDir)}
}

func (l *DefaultAgentSessionResourceLoader) GetSystemPrompt() string {
	return strings.TrimSpace(readOptionalFile(filepath.Join(l.cwd, ConfigDirName, "SYSTEM.md")))
}

func (l *DefaultAgentSessionResourceLoader) GetAppendSystemPrompt() string {
	return readOptionalFile(filepath.Join(l.cwd, ConfigDirName, "APPEND_SYSTEM.md"))
}

func GetAgentDirSessionDir(cwd, agentDir string) string {
	safePath := strings.TrimLeft(cwd, `/\`)
	replacer := strings.NewReplacer("/", "-", `\`, "-", ":", "-")
	return filepath.Join(agentDir, "sessions", "--"+replacer.Replace(safePath)+"--")
}

func defaultSDKToolSnippets() map[string]string {
	return map[string]string{
		"read":  "Read file contents",
		"bash":  "Execute bash commands",
		"grep":  "Search file contents",
		"find":  "Find files by glob pattern",
		"ls":    "List directory entries",
		"edit":  "Make surgical edits",
		"write": "Create or overwrite files",
	}
}

func defaultSDKTools(cwd string) []SDKTool {
	return []SDKTool{
		sdkToolFromFileToolDefinition(CreateReadToolDefinition(cwd), "Read file contents", ProtocolSourceInfo{Path: "<builtin:read>", Source: "builtin", Scope: "temporary", Origin: "top-level"}),
		{
			Name:          "bash",
			Description:   "Execute bash commands",
			Parameters:    CreateBashToolDefinition(cwd).Parameters,
			PromptSnippet: "Execute bash commands",
			SourceInfo:    ProtocolSourceInfo{Path: "<builtin:bash>", Source: "builtin", Scope: "temporary", Origin: "top-level"},
			Execute: func(toolCallID string, input map[string]any) (SDKToolResult, error) {
				return executeDefaultSDKBashTool(toolCallID, input, cwd, nil)
			},
			ExecuteWithUpdates: func(toolCallID string, input map[string]any, onUpdate func(SDKToolResult)) (SDKToolResult, error) {
				return executeDefaultSDKBashTool(toolCallID, input, cwd, onUpdate)
			},
		},
		{
			Name:          "grep",
			Description:   "Search file contents",
			Parameters:    grepSDKToolParameters(),
			PromptSnippet: "Search file contents",
			SourceInfo:    ProtocolSourceInfo{Path: "<builtin:grep>", Source: "builtin", Scope: "temporary", Origin: "top-level"},
			Execute: func(_ string, input map[string]any) (SDKToolResult, error) {
				result, err := NewGrepTool(cwd).Execute("", GrepToolInput{
					Pattern:    stringFromToolInput(input, "pattern"),
					Path:       stringFromToolInput(input, "path"),
					Glob:       stringFromToolInput(input, "glob"),
					IgnoreCase: boolFromToolInput(input, "ignoreCase"),
					Literal:    boolFromToolInput(input, "literal"),
					Limit:      intFromToolInput(input, "limit"),
					Context:    intFromToolInput(input, "context"),
				})
				return sdkToolResultFromFileToolResult(result), err
			},
		},
		{
			Name:          "find",
			Description:   "Find files by glob pattern",
			Parameters:    findSDKToolParameters(),
			PromptSnippet: "Find files by glob pattern",
			SourceInfo:    ProtocolSourceInfo{Path: "<builtin:find>", Source: "builtin", Scope: "temporary", Origin: "top-level"},
			Execute: func(_ string, input map[string]any) (SDKToolResult, error) {
				result, err := NewFindTool(cwd).Execute("", FindToolInput{
					Pattern: stringFromToolInput(input, "pattern"),
					Path:    stringFromToolInput(input, "path"),
					Limit:   intFromToolInput(input, "limit"),
				})
				return sdkToolResultFromFileToolResult(result), err
			},
		},
		{
			Name:          "ls",
			Description:   "List directory entries",
			Parameters:    lsSDKToolParameters(),
			PromptSnippet: "List directory entries",
			SourceInfo:    ProtocolSourceInfo{Path: "<builtin:ls>", Source: "builtin", Scope: "temporary", Origin: "top-level"},
			Execute: func(_ string, input map[string]any) (SDKToolResult, error) {
				result, err := NewLsTool(cwd).Execute("", LsToolInput{Path: stringFromToolInput(input, "path"), Limit: intFromToolInput(input, "limit")})
				return sdkToolResultFromFileToolResult(result), err
			},
		},
		sdkToolFromFileToolDefinition(CreateEditToolDefinition(cwd), "Make surgical edits", ProtocolSourceInfo{Path: "<builtin:edit>", Source: "builtin", Scope: "temporary", Origin: "top-level"}),
		sdkToolFromFileToolDefinition(CreateWriteToolDefinition(cwd), "Create or overwrite files", ProtocolSourceInfo{Path: "<builtin:write>", Source: "builtin", Scope: "temporary", Origin: "top-level"}),
	}
}

func sdkToolFromFileToolDefinition(definition ToolDefinition, promptSnippet string, sourceInfo ProtocolSourceInfo) SDKTool {
	prepare := func(input map[string]any) map[string]any {
		if definition.PrepareArguments == nil {
			return input
		}
		return mapFromPreparedToolArguments(definition.PrepareArguments(input), input)
	}
	return SDKTool{
		Name:             definition.Name,
		Description:      definition.Description,
		Parameters:       definition.Parameters,
		PromptSnippet:    promptSnippet,
		SourceInfo:       sourceInfo,
		PrepareArguments: prepare,
		Execute: func(toolCallID string, input map[string]any) (SDKToolResult, error) {
			result, err := definition.Execute(toolCallID, prepare(input))
			return sdkToolResultFromFileToolResult(result), err
		},
	}
}

func mapFromPreparedToolArguments(prepared any, fallback map[string]any) map[string]any {
	values, ok := prepared.(map[string]any)
	if !ok {
		return fallback
	}
	return values
}

func grepSDKToolParameters() llm.Schema {
	return llm.Object(map[string]llm.Schema{
		"pattern":    llm.String(),
		"path":       llm.String(),
		"glob":       llm.String(),
		"ignoreCase": llm.Boolean(),
		"literal":    llm.Boolean(),
		"limit":      llm.Integer(),
		"context":    llm.Integer(),
	}, "pattern")
}

func findSDKToolParameters() llm.Schema {
	return llm.Object(map[string]llm.Schema{
		"pattern": llm.String(),
		"path":    llm.String(),
		"limit":   llm.Integer(),
	}, "pattern")
}

func lsSDKToolParameters() llm.Schema {
	return llm.Object(map[string]llm.Schema{
		"path":  llm.String(),
		"limit": llm.Integer(),
	}, "path")
}

func executeDefaultSDKBashTool(toolCallID string, input map[string]any, cwd string, onUpdate func(SDKToolResult)) (SDKToolResult, error) {
	command, _ := input["command"].(string)
	if strings.TrimSpace(command) == "" {
		return SDKToolResult{}, fmt.Errorf("bash command is required")
	}
	tool := NewBashTool(cwd)
	result, err := tool.ExecuteWithUpdates(toolCallID, BashToolInput{Command: command, Timeout: intFromToolInput(input, "timeout")}, func(partial FileToolResult) {
		if onUpdate != nil {
			onUpdate(sdkToolResultFromFileToolResult(partial))
		}
	})
	return sdkToolResultFromFileToolResult(result), err
}

func sdkToolResultFromFileToolResult(result FileToolResult) SDKToolResult {
	content := make([]SDKContentPart, 0, len(result.Content))
	for _, part := range result.Content {
		if part.Type == llm.ContentText {
			content = append(content, SDKContentPart{Type: "text", Text: part.Text})
		}
	}
	if len(content) == 0 && result.Text != "" {
		content = append(content, SDKContentPart{Type: "text", Text: result.Text})
	}
	return SDKToolResult{Content: content, Details: result.Details}
}

func (s *AgentSession) GetAllTools() []SDKTool {
	if s == nil || s.Agent == nil {
		return nil
	}
	tools := append([]SDKTool(nil), s.Agent.State.Tools...)
	tools = append(tools, s.DynamicTools...)
	return allToolsForPolicy(tools, s.Tools, s.ToolsSet, s.NoTools)
}

func (s *AgentSession) GetActiveToolNames() []string {
	tools := s.GetActiveTools()
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

func (s *AgentSession) GetActiveTools() []SDKTool {
	if s == nil || s.Agent == nil {
		return nil
	}
	tools := append([]SDKTool(nil), s.Agent.State.Tools...)
	tools = append(tools, s.DynamicTools...)
	return activeToolsForPolicy(tools, nil, s.Tools, s.ToolsSet, s.NoTools)
}

func (s *AgentSession) GetActiveLLMTools() []llm.Tool {
	if s == nil {
		return nil
	}
	return llmToolsFromSDKTools(s.GetActiveTools())
}

func (s *AgentSession) RefreshSystemPrompt() {
	if s == nil || s.Agent == nil || s.SessionManager == nil {
		return
	}
	prompt := buildAgentSessionSystemPrompt(s.SessionManager.GetCWD(), s.ResourceLoader, s.GetActiveTools())
	s.SystemPrompt = prompt
	s.Agent.State.SystemPrompt = prompt
}

func allToolsForPolicy(tools []SDKTool, allowlist []string, allowlistSet bool, noTools string) []SDKTool {
	if noTools == "all" {
		return nil
	}
	if allowlistSet {
		return filterToolsByAllowlist(tools, allowlist)
	}
	return append([]SDKTool(nil), tools...)
}

func activeToolsForPolicy(tools []SDKTool, dynamic []SDKTool, allowlist []string, allowlistSet bool, noTools string) []SDKTool {
	combined := append([]SDKTool(nil), tools...)
	combined = append(combined, dynamic...)
	filtered := allToolsForPolicy(combined, allowlist, allowlistSet, noTools)
	if noTools != "builtin" {
		return filtered
	}
	result := make([]SDKTool, 0, len(filtered))
	for _, tool := range filtered {
		if tool.SourceInfo.Source == "builtin" {
			continue
		}
		result = append(result, tool)
	}
	return result
}

func filterToolsByAllowlist(tools []SDKTool, allowlist []string) []SDKTool {
	allowed := map[string]bool{}
	for _, name := range allowlist {
		name = strings.TrimSpace(name)
		if name != "" {
			allowed[name] = true
		}
	}
	result := make([]SDKTool, 0, len(tools))
	for _, tool := range tools {
		if allowed[tool.Name] {
			result = append(result, tool)
		}
	}
	return result
}

func llmToolsFromSDKTools(tools []SDKTool) []llm.Tool {
	result := make([]llm.Tool, 0, len(tools))
	seen := map[string]bool{}
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, llm.Tool{
			Name:        name,
			Description: firstNonEmptyString(tool.Description, tool.PromptSnippet, tool.Label),
			Parameters:  sdkToolParametersOrEmptyObject(tool.Parameters),
		})
	}
	return result
}

func sdkToolParametersOrEmptyObject(schema llm.Schema) llm.Schema {
	if schema.Type != nil || len(schema.Properties) > 0 || len(schema.Required) > 0 || schema.Items != nil || len(schema.Enum) > 0 {
		return schema
	}
	return llm.Object(map[string]llm.Schema{})
}

func stringFromToolInput(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return value
}

func intFromToolInput(input map[string]any, key string) int {
	switch value := input[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
	}
}

func boolFromToolInput(input map[string]any, key string) bool {
	value, _ := input[key].(bool)
	return value
}

func buildAgentSessionSystemPrompt(cwd string, resourceLoader AgentSessionResourceLoader, tools []SDKTool) string {
	var guidelines []string
	selected := make([]string, 0, len(tools))
	snippets := map[string]string{}
	for _, tool := range tools {
		selected = append(selected, tool.Name)
		if strings.TrimSpace(tool.PromptSnippet) != "" {
			snippets[tool.Name] = tool.PromptSnippet
		}
		guidelines = append(guidelines, tool.PromptGuidelines...)
	}
	var skills []agentharness.Skill
	var contextFiles []SystemPromptContextFile
	customPrompt := ""
	appendSystemPrompt := ""
	if resourceLoader != nil {
		skills = resourceLoader.GetSkills().Skills
		if contextLoader, ok := resourceLoader.(AgentSessionContextResourceLoader); ok {
			for _, file := range contextLoader.GetAgentsFiles().AgentsFiles {
				content := file.Content
				if content == "" && strings.TrimSpace(file.Path) != "" {
					content = readOptionalFile(file.Path)
				}
				contextFiles = append(contextFiles, SystemPromptContextFile{Path: file.Path, Content: content})
			}
		}
		if promptLoader, ok := resourceLoader.(AgentSessionSystemPromptResourceLoader); ok {
			customPrompt = promptLoader.GetSystemPrompt()
			appendSystemPrompt = promptLoader.GetAppendSystemPrompt()
		}
	}
	return BuildSystemPrompt(BuildSystemPromptOptions{
		CustomPrompt:       customPrompt,
		CWD:                cwd,
		AppendSystemPrompt: appendSystemPrompt,
		ContextFiles:       contextFiles,
		Skills:             toSystemPromptSkills(skills),
		ToolSnippets:       snippets,
		SelectedTools:      selected,
		PromptGuidelines:   guidelines,
		DocumentationPaths: defaultGiDocumentationPaths(cwd),
	})
}

func toSystemPromptSkills(skills []agentharness.Skill) []SystemPromptSkill {
	converted := make([]SystemPromptSkill, 0, len(skills))
	for _, skill := range skills {
		if skill.DisableModelInvocation {
			continue
		}
		converted = append(converted, SystemPromptSkill{
			Name:        skill.Name,
			Description: skill.Description,
			Content:     skill.Content,
		})
	}
	return converted
}

func aggregateSessionUsage(branch []FileEntry) llm.Usage {
	total := llm.EmptyUsage()
	startIndex := 0
	if compactionIndex := lastSessionCompactionIndex(branch); compactionIndex >= 0 {
		tokensBefore := branch[compactionIndex].TokensBefore
		total.Input += tokensBefore
		total.TotalTokens += tokensBefore
		startIndex = compactionIndex + 1
	}
	for _, entry := range branch[startIndex:] {
		usage, ok := assistantEntryUsage(entry)
		if !ok {
			continue
		}
		addUsage(&total, usage)
	}
	return total
}

func lastSessionCompactionIndex(branch []FileEntry) int {
	for index := len(branch) - 1; index >= 0; index-- {
		if branch[index].Type == "compaction" {
			return index
		}
	}
	return -1
}

func assistantEntryUsage(entry FileEntry) (llm.Usage, bool) {
	if entry.Type != "message" {
		return llm.Usage{}, false
	}
	switch message := entry.Message.(type) {
	case llm.Message:
		if message.Role != llm.RoleAssistant || usageTokenTotal(message.Usage) == 0 {
			return llm.Usage{}, false
		}
		return message.Usage, true
	case map[string]any:
		role, _ := message["role"].(string)
		if role != llm.RoleAssistant {
			return llm.Usage{}, false
		}
		usage, ok := usageFromSessionMessageValue(message["usage"])
		if !ok || usageTokenTotal(usage) == 0 {
			return llm.Usage{}, false
		}
		return usage, true
	default:
		return llm.Usage{}, false
	}
}

func usageFromSessionMessageValue(value any) (llm.Usage, bool) {
	switch usage := value.(type) {
	case llm.Usage:
		return usage, true
	case map[string]any:
		result := llm.Usage{
			Input:       intFromSessionUsageValue(usage["input"]),
			Output:      intFromSessionUsageValue(usage["output"]),
			CacheRead:   intFromSessionUsageValue(usage["cacheRead"]),
			CacheWrite:  intFromSessionUsageValue(usage["cacheWrite"]),
			TotalTokens: intFromSessionUsageValue(usage["totalTokens"]),
		}
		if cost, ok := usage["cost"].(map[string]any); ok {
			result.Cost = llm.UsageCost{
				Input:      floatFromSessionUsageValue(cost["input"]),
				Output:     floatFromSessionUsageValue(cost["output"]),
				CacheRead:  floatFromSessionUsageValue(cost["cacheRead"]),
				CacheWrite: floatFromSessionUsageValue(cost["cacheWrite"]),
				Total:      floatFromSessionUsageValue(cost["total"]),
			}
		}
		return result, usageTokenTotal(result) > 0
	default:
		return llm.Usage{}, false
	}
}

func floatFromSessionUsageValue(value any) float64 {
	switch number := value.(type) {
	case int:
		return float64(number)
	case int64:
		return float64(number)
	case float64:
		return number
	case float32:
		return float64(number)
	default:
		return 0
	}
}

func intFromSessionUsageValue(value any) int {
	switch number := value.(type) {
	case int:
		return number
	case int64:
		return int(number)
	case float64:
		return int(number)
	case float32:
		return int(number)
	default:
		return 0
	}
}

func addUsage(total *llm.Usage, usage llm.Usage) {
	total.Input += usage.Input
	total.Output += usage.Output
	total.CacheRead += usage.CacheRead
	total.CacheWrite += usage.CacheWrite
	total.TotalTokens += usageTokenTotal(usage)
	total.Cost.Input += usage.Cost.Input
	total.Cost.Output += usage.Cost.Output
	total.Cost.CacheRead += usage.Cost.CacheRead
	total.Cost.CacheWrite += usage.Cost.CacheWrite
	total.Cost.Total += usage.Cost.Total
}

func usageTokenTotal(usage llm.Usage) int {
	if usage.TotalTokens != 0 {
		return usage.TotalTokens
	}
	return usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
}

func contextUsagePercent(tokens, contextWindow int) *float64 {
	if contextWindow <= 0 {
		return nil
	}
	percent := (float64(tokens) / float64(contextWindow)) * 100
	return &percent
}
