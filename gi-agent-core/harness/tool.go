package harness

import (
	"context"
	"fmt"

	core "github.com/nowa/gi/gi-agent-core"
	llm "github.com/nowa/gi/gi-llm-provider"
)

// AgentHarnessTool is a tool whose execution receives the context snapshot
// resolved by AgentHarness for the current turn.
//
// The harness keeps context resolution outside the agent loop. This lets the
// loop operate on ordinary core.AgentTool values while tools still receive
// application-owned dependencies such as an ExecutionEnv.
type AgentHarnessTool struct {
	Name        string
	Label       string
	Description string
	Parameters  llm.Schema

	PrepareArguments func(args any) (map[string]any, error)
	Execute          func(ctx context.Context, toolCallID string, params map[string]any, onUpdate core.AgentToolUpdateCallback, toolContext any) (core.AgentToolResult, error)
	ExecutionMode    string
}

// AgentHarnessToolContextSource resolves the application-defined tool context
// once for each turn snapshot.
type AgentHarnessToolContextSource func(context.Context) (any, error)

// StaticToolContext adapts a stable value to an AgentHarnessToolContextSource.
func StaticToolContext(value any) AgentHarnessToolContextSource {
	return func(context.Context) (any, error) {
		return value, nil
	}
}

// ContextFreeAgentHarnessTool adapts an ordinary agent-loop tool for use by an
// AgentHarness. Its executor ignores the harness tool context.
func ContextFreeAgentHarnessTool(tool core.AgentTool) AgentHarnessTool {
	return AgentHarnessTool{
		Name:             tool.Name,
		Label:            tool.Label,
		Description:      tool.Description,
		Parameters:       tool.Parameters,
		PrepareArguments: tool.PrepareArguments,
		ExecutionMode:    tool.ExecutionMode,
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate core.AgentToolUpdateCallback, _ any) (core.AgentToolResult, error) {
			if tool.Execute == nil {
				return core.AgentToolResult{}, fmt.Errorf("tool %q has no executor", tool.Name)
			}
			return tool.Execute(ctx, toolCallID, params, onUpdate)
		},
	}
}

func (t AgentHarnessTool) bind(toolContext any) core.AgentTool {
	return core.AgentTool{
		Name:             t.Name,
		Label:            t.Label,
		Description:      t.Description,
		Parameters:       t.Parameters,
		PrepareArguments: t.PrepareArguments,
		ExecutionMode:    t.ExecutionMode,
		Execute: func(ctx context.Context, toolCallID string, params map[string]any, onUpdate core.AgentToolUpdateCallback) (core.AgentToolResult, error) {
			if t.Execute == nil {
				return core.AgentToolResult{}, fmt.Errorf("tool %q has no executor", t.Name)
			}
			return t.Execute(ctx, toolCallID, params, onUpdate, toolContext)
		},
	}
}
