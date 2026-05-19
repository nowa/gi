# Gi Agent Harness

`gi` is a Go rebuild of selected Pi agent harness packages. It provides library
packages for streaming LLM calls, agent loops, tool execution, session harnesses,
and terminal UI surfaces without depending on provider SDKs.

This repository follows Pi's package split where it makes sense for Go. The goal
is Pi-compatible behavior at the package boundary, not a direct TypeScript line
port.

## Packages

| Package | Description |
| --- | --- |
| `github.com/nowa/gi/gi-llm-provider` | Unified multi-provider LLM API, model catalog, payload conversion, streaming events, and HTTP/SSE transports. |
| `github.com/nowa/gi/gi-agent-core` | Agent runtime with state management, tool calling, lifecycle events, queues, and turn orchestration. |
| `github.com/nowa/gi/gi-agent-core/harness` | Session storage, prompt formatting, compaction, skills, local execution helpers, and test harness utilities. |
| `github.com/nowa/gi/gi-tui` | Terminal UI library with components, editor/input behavior, overlays, key parsing, image fallbacks, and differential rendering. |
| `github.com/nowa/gi/gi-coding-agent` | Partial coding-agent utility foundation only: args, prompt templates, frontmatter/path/ANSI helpers, and session JSONL management. It is not a full interactive coding-agent port. |

Pi's `pi-web-ui` package is not ported here. Pi's full interactive
`pi-coding-agent` runtime is also not ported yet.

## Compatibility Status

| Pi package | Gi package | Status |
| --- | --- | --- |
| `@earendil-works/pi-ai` | `gi-llm-provider` | Pi test-compatible for core provider contracts, model catalogs, message conversion, streaming, and registered transports. |
| `@earendil-works/pi-agent-core` | `gi-agent-core` | Pi test-compatible for the agent loop, tools, stateful agent behavior, queues, and lifecycle events. |
| `pi-agent-core` harness/session | `gi-agent-core/harness` | Pi test-compatible for sessions, prompt formatting, compaction, local env, skills, and storage helpers. |
| `@earendil-works/pi-tui` | `gi-tui` | TUI milestone complete. Pi TUI test files and case-level behavior are mapped and covered in Go. |
| `@earendil-works/pi-coding-agent` | `gi-coding-agent` | Partial utility-level foundation. Full interactive coding-agent runtime parity is not in the current scope. |

Detailed coverage is tracked in [PI_COMPATIBILITY.md](PI_COMPATIBILITY.md).
Per-case TUI mapping is tracked in
[PI_TUI_TEST_CASE_PARITY.md](PI_TUI_TEST_CASE_PARITY.md).

## Installation

This module targets Go 1.26.

```sh
go get github.com/nowa/gi/gi-llm-provider
go get github.com/nowa/gi/gi-agent-core
go get github.com/nowa/gi/gi-tui
```

## LLM Usage

```go
package main

import (
	"context"
	"fmt"

	llm "github.com/nowa/gi/gi-llm-provider"
)

func main() {
	model := llm.MustGetModel("xai", "grok-4.3")
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

Provider API keys can be passed in stream options or read from environment
variables such as `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `XAI_API_KEY`,
`DEEPSEEK_API_KEY`, and `GOOGLE_GENERATIVE_AI_API_KEY`.

## Agent Usage

```go
package main

import (
	"context"

	giagentcore "github.com/nowa/gi/gi-agent-core"
	llm "github.com/nowa/gi/gi-llm-provider"
)

func main() {
	agent := giagentcore.New(
		giagentcore.WithInitialState(giagentcore.AgentState{
			SystemPrompt:  "You are a helpful coding agent.",
			Model:         llm.MustGetModel("xai", "grok-4.3"),
			ThinkingLevel: "low",
		}),
	)

	if err := agent.PromptText(context.Background(), "Summarize this repository"); err != nil {
		panic(err)
	}
}
```

Tools are defined with `giagentcore.AgentTool` and converted to LLM tool calls by
the agent loop.

## TUI Usage

```go
package main

import (
	"fmt"
	"strings"

	gitui "github.com/nowa/gi/gi-tui"
)

func main() {
	terminal := gitui.NewVirtualTerminal(80, 12)
	ui := gitui.NewTUI(terminal)
	ui.AddChild(gitui.NewText("Hello from gi-tui", 1, 0))

	ui.Start()
	defer ui.Stop()
	ui.RequestRender(true)

	fmt.Println(strings.Join(terminal.GetViewport(), "\n"))
}
```

`gi-tui` includes a deterministic headless `VirtualTerminal` for tests and a
`ProcessTerminal` backed by `golang.org/x/term` for real terminals.

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

Credentialed live probes are intentionally outside the default test gate.

## Development

```sh
go test -timeout 30s ./...                                  # Run all tests
GOCACHE=/private/tmp/gi-gocache go test -timeout 30s ./...   # Stable cache for sandboxed runs
go test ./gi-tui                                             # TUI package tests
go test ./gi-agent-core/...                                  # Agent core and harness tests
go test ./gi-llm-provider                                    # Provider package tests
gofmt -w path/to/file.go                                     # Format edited Go files
```

There is no Makefile; `go test ./...` is the default validation gate.

## Contributing

See [AGENTS.md](AGENTS.md) for repository-specific contributor and agent rules.
Keep tests deterministic and avoid requiring live provider credentials in the
default suite.
