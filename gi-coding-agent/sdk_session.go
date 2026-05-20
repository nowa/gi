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
	SessionManager       *SessionManager
	ResourceLoader       AgentSessionResourceLoader
	CompactionSettings   *agentharness.CompactionSettings
	CompactionSummarizer AgentSessionCompactionSummarizer
	BranchSummarizer     AgentSessionBranchSummarizer
	RetrySettings        *AgentSessionRetrySettings
	Responder            AgentSessionResponder
}

type AgentSession struct {
	SessionManager       *SessionManager
	ResourceLoader       AgentSessionResourceLoader
	SystemPrompt         string
	Agent                *SDKAgent
	CompactionSettings   agentharness.CompactionSettings
	CompactionSummarizer AgentSessionCompactionSummarizer
	BranchSummarizer     AgentSessionBranchSummarizer
	RetrySettings        AgentSessionRetrySettings
	Responder            AgentSessionResponder
	eventListeners       []AgentSessionEventListener
	branchSummaryAbort   chan struct{}
	isCompacting         bool
	isRetrying           bool
	isStreaming          bool
	steeringMessages     []string
	followUpMessages     []string
}

type AgentSessionStats struct {
	Tokens       llm.Usage
	ContextUsage *AgentContextUsage
}

type AgentContextUsage struct {
	Tokens        *int
	ContextWindow int
	Percent       *float64
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
	Name    string
	Execute func(toolCallID string, input map[string]any) (SDKToolResult, error)
}

type SDKToolResult struct {
	Content []SDKContentPart
}

type SDKContentPart struct {
	Type string
	Text string
}

type AgentSessionResourceLoader interface {
	GetSkills() AgentSessionSkillsResult
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
		agentDir = filepath.Join(cwd, ".pi", "agent")
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

	systemPrompt := BuildSystemPrompt(BuildSystemPromptOptions{
		CWD:           cwd,
		ContextFiles:  []SystemPromptContextFile{},
		Skills:        toSystemPromptSkills(resourceLoader.GetSkills().Skills),
		ToolSnippets:  defaultSDKToolSnippets(),
		SelectedTools: []string{"read", "bash", "edit", "write"},
	})
	agent := &SDKAgent{State: SDKAgentState{
		SystemPrompt:  systemPrompt,
		Model:         options.Model,
		ThinkingLevel: "off",
		Tools:         defaultSDKTools(cwd),
	}}
	return &AgentSession{
		SessionManager:       sessionManager,
		ResourceLoader:       resourceLoader,
		SystemPrompt:         systemPrompt,
		Agent:                agent,
		CompactionSettings:   compactionSettings,
		CompactionSummarizer: compactionSummarizer,
		BranchSummarizer:     branchSummarizer,
		RetrySettings:        retrySettings,
		Responder:            responder,
	}, nil
}

func (s *AgentSession) Dispose() {}

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
		filepath.Join(l.cwd, ".pi", "skills"),
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

func GetAgentDirSessionDir(cwd, agentDir string) string {
	safePath := strings.TrimLeft(cwd, `/\`)
	replacer := strings.NewReplacer("/", "-", `\`, "-", ":", "-")
	return filepath.Join(agentDir, "sessions", "--"+replacer.Replace(safePath)+"--")
}

func defaultSDKToolSnippets() map[string]string {
	return map[string]string{
		"read":  "Read file contents",
		"bash":  "Execute bash commands",
		"edit":  "Make surgical edits",
		"write": "Create or overwrite files",
	}
}

func defaultSDKTools(cwd string) []SDKTool {
	return []SDKTool{
		{Name: "read"},
		{
			Name: "bash",
			Execute: func(_ string, input map[string]any) (SDKToolResult, error) {
				command, _ := input["command"].(string)
				if strings.TrimSpace(command) == "" {
					return SDKToolResult{}, fmt.Errorf("bash command is required")
				}
				result, err := ExecuteBash(command, cwd)
				return SDKToolResult{Content: []SDKContentPart{{Type: "text", Text: result.Output}}}, err
			},
		},
		{Name: "edit"},
		{Name: "write"},
	}
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
		return result, usageTokenTotal(result) > 0
	default:
		return llm.Usage{}, false
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
