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
	CWD            string
	AgentDir       string
	Model          llm.Model
	SessionManager *SessionManager
	ResourceLoader AgentSessionResourceLoader
}

type AgentSession struct {
	SessionManager *SessionManager
	ResourceLoader AgentSessionResourceLoader
	SystemPrompt   string
	Agent          *SDKAgent
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
		SessionManager: sessionManager,
		ResourceLoader: resourceLoader,
		SystemPrompt:   systemPrompt,
		Agent:          agent,
	}, nil
}

func (s *AgentSession) Dispose() {}

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
