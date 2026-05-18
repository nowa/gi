# gi

`gi` is a Go rebuild of Pi's agent and LLM provider packages. It provides a small set of library packages for building agent loops, streaming model calls, tool execution, and test harnesses without pulling in provider SDK dependencies.

## Packages

| Package | Purpose |
| --- | --- |
| `github.com/nowa/gi/gi-llm-provider` | Provider registry, model catalog, message types, streaming event contracts, payload conversion, and HTTP/SSE transports. |
| `github.com/nowa/gi/gi-agent-core` | Stateful agent loop, tool execution, message queues, lifecycle events, and turn orchestration. |
| `github.com/nowa/gi/gi-agent-core/harness` | Session storage, prompt formatting, compaction, skills, local execution env, and harness utilities. |

## Supported Provider Transports

`gi-llm-provider` includes registered transports for:

- Anthropic Messages
- OpenAI Responses and Chat Completions
- OpenAI Codex Responses
- Azure OpenAI Responses
- Google Generative AI and Vertex AI
- Mistral Conversations
- Amazon Bedrock Converse Stream
- OpenRouter Images
- OpenAI-compatible providers including xAI/Grok, DeepSeek, Groq, Together, Z.ai, Fireworks, OpenCode Zen, and GitHub Copilot-compatible endpoints

Provider API keys are resolved from explicit stream options first, then environment variables such as `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `XAI_API_KEY`, `DEEPSEEK_API_KEY`, `GOOGLE_GENERATIVE_AI_API_KEY`, and provider-specific equivalents.

## Installation

```sh
go get github.com/nowa/gi
```

This module currently targets Go 1.26.

## Basic LLM Usage

```go
package main

import (
	"context"
	"fmt"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func main() {
	model := llm.MustGetModel("deepseek", "deepseek-v4-flash")
	msg, err := llm.CompleteSimple(context.Background(), model, llm.Context{
		SystemPrompt: "You are concise.",
		Messages:     []llm.Message{llm.UserMessageText("Say hello")},
	}, llm.SimpleStreamOptions{
		Reasoning: "high",
	})
	if err != nil {
		panic(err)
	}
	for _, part := range msg.Content {
		if part.Type == llm.ContentText {
			fmt.Print(part.Text)
		}
	}
}
```

To use Grok:

```go
model := llm.MustGetModel("xai", "grok-4.3")
```

Set `XAI_API_KEY` or pass `SimpleStreamOptions{APIKey: "..."}`.

## Basic Agent Usage

```go
agent := giagentcore.New(
	giagentcore.WithInitialState(giagentcore.AgentState{
		SystemPrompt:  "You are a helpful coding agent.",
		Model:         llm.MustGetModel("xai", "grok-4.3"),
		ThinkingLevel: "low",
	}),
)

err := agent.PromptText(context.Background(), "Summarize this repository")
```

Agent tools are defined with `giagentcore.AgentTool` and converted to LLM tools through the agent loop.

## Development

Run the full test suite:

```sh
go test -timeout 30s ./...
```

For a stable local cache in sandboxed environments:

```sh
GOCACHE=/private/tmp/gi-gocache go test -timeout 30s ./...
```

The compatibility coverage is tracked in [PI_COMPATIBILITY.md](PI_COMPATIBILITY.md).

## Design Notes

- Providers are explicit registry entries, so callers can replace or extend them with `RegisterAPIProvider`.
- Live HTTP transports are covered by local `httptest` SSE tests; credentialed live probes are intentionally outside the default test gate.
- The model catalog stores provider-specific compatibility details such as reasoning fields, cache controls, strict tool support, and token field naming.
