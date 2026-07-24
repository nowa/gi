package tools

import (
	"context"
	"fmt"

	core "github.com/nowa/gi/gi-agent-core"
	agentharness "github.com/nowa/gi/gi-agent-core/harness"
	llm "github.com/nowa/gi/gi-llm-provider"
)

func CreateWriteTool() agentharness.AgentHarnessTool {
	return agentharness.AgentHarnessTool{
		Name:        "write",
		Label:       "write",
		Description: "Write content to a file. Creates the file if it doesn't exist, overwrites if it does. Automatically creates parent directories.",
		Parameters: llm.Object(map[string]llm.Schema{
			"path": {
				Type:        "string",
				Description: "Path to the file to write (relative or absolute)",
			},
			"content": {
				Type:        "string",
				Description: "Content to write to the file",
			},
		}, "path", "content"),
		Execute: func(ctx context.Context, _ string, params map[string]any, _ core.AgentToolUpdateCallback, contextValue any) (core.AgentToolResult, error) {
			provider, mutations, err := executionContext(contextValue)
			if err != nil {
				return core.AgentToolResult{}, err
			}
			path, err := requiredString(params, "path")
			if err != nil {
				return core.AgentToolResult{}, err
			}
			content, err := requiredString(params, "content")
			if err != nil {
				return core.AgentToolResult{}, err
			}
			env := provider.ExecutionEnvironment()
			absolutePath := ResolveToolPath(env, path)
			var result core.AgentToolResult
			err = mutations.With(ctx, env, absolutePath, func() error {
				if err := contextError(ctx); err != nil {
					return err
				}
				if err := env.WriteFile(ctx, absolutePath, []byte(content)); err != nil {
					return err
				}
				if err := contextError(ctx); err != nil {
					return err
				}
				text := fmt.Sprintf("Successfully wrote %d bytes to %s", len([]byte(content)), path)
				result = core.AgentToolResult{Content: []llm.ContentPart{llm.Text(text)}}
				return nil
			})
			return result, err
		},
	}
}
