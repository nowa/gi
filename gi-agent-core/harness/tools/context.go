// Package tools provides reusable filesystem and shell tools for AgentHarness.
package tools

import (
	"fmt"

	harnessenv "github.com/nowa/gi/gi-agent-core/harness/env"
)

// ExecutionEnvironmentProvider exposes the filesystem and shell environment
// required by the built-in tools. Applications can embed ExecutionToolContext
// in a larger turn context to add their own state.
type ExecutionEnvironmentProvider interface {
	ExecutionEnvironment() harnessenv.ExecutionEnv
}

type mutationQueueProvider interface {
	FileMutations() *FileMutationQueue
}

// ExecutionToolContext is the standard context for the built-in execution
// tools. Construct it with NewExecutionToolContext so unrelated harnesses do
// not share mutation scheduling state.
type ExecutionToolContext struct {
	Env       harnessenv.ExecutionEnv
	Mutations *FileMutationQueue
}

func NewExecutionToolContext(env harnessenv.ExecutionEnv) ExecutionToolContext {
	return ExecutionToolContext{
		Env:       env,
		Mutations: NewFileMutationQueue(),
	}
}

func (c ExecutionToolContext) ExecutionEnvironment() harnessenv.ExecutionEnv {
	return c.Env
}

func (c ExecutionToolContext) FileMutations() *FileMutationQueue {
	return c.Mutations
}

func executionContext(value any) (ExecutionEnvironmentProvider, *FileMutationQueue, error) {
	provider, ok := value.(ExecutionEnvironmentProvider)
	if !ok {
		return nil, nil, fmt.Errorf("execution tool context must provide an ExecutionEnv")
	}
	env := provider.ExecutionEnvironment()
	if env == nil {
		return nil, nil, fmt.Errorf("execution tool context has no ExecutionEnv")
	}
	if mutationProvider, ok := value.(mutationQueueProvider); ok {
		if mutations := mutationProvider.FileMutations(); mutations != nil {
			return provider, mutations, nil
		}
	}
	return provider, defaultFileMutationQueue, nil
}
