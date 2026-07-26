<!-- markdownlint-disable MD010 MD013 -->

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
| `github.com/nowa/gi/gi-coding-agent` | Coding-agent migration surface: CLI/config/settings, resources, session JSONL, tool helpers, protocol-backed packages/extensions, experimental first-time setup, and interactive runtime pieces as they are ported. |

Internal subpackages mirror Pi boundaries where the Go dependency graph allows
it without changing public APIs. Current examples include
`gi-llm-provider/internal/envkeys`, `gi-llm-provider/internal/oauthpage`,
`gi-llm-provider/internal/eventstream`, `gi-llm-provider/internal/httpproxy`,
`gi-agent-core/harness/env`, `gi-agent-core/harness/utils`,
`gi-agent-core/harness/sessionid`, `gi-tui/internal/history`,
`gi-coding-agent/internal/cli`, `gi-coding-agent/internal/planmode`,
`gi-coding-agent/internal/modelresolver`,
`gi-coding-agent/internal/rpcwire`, `gi-coding-agent/internal/imageresize`,
`gi-coding-agent/internal/syntaxhighlight`,
`gi-coding-agent/internal/fswatch`, `gi-coding-agent/internal/gitsource`,
`gi-coding-agent/internal/diffrender`, `gi-coding-agent/internal/toolqueue`,
`gi-coding-agent/internal/tooloutput`, `gi-coding-agent/internal/changelog`,
`gi-coding-agent/internal/versioncheck`, `gi-coding-agent/internal/frontmatter`,
`gi-coding-agent/internal/pathutil`, and `gi-coding-agent/internal/ansiutil`;
the parent packages keep
compatibility facades for existing callers and tests.

Pi's `pi-web-ui` package is not ported here. The declared `pi-coding-agent`
migration scope is closed against the immutable Pi `v0.82.1` release at
`b4f29368`; scope decisions, the zero-gap snapshot, and reproducible
verification commands are in
[docs/pi-parity/README.md](docs/pi-parity/README.md).

## Compatibility Status

| Pi package | Gi package | Status |
| --- | --- | --- |
| `@earendil-works/pi-ai` | `gi-llm-provider` | Pi-compatible for the audited provider contracts, model catalogs, message conversion, streaming, and registered transports; the v0.82.1 source/member/test mapping gate is closed under `docs/pi-parity/`. |
| `@earendil-works/pi-agent-core` | `gi-agent-core` | Pi-compatible for the audited agent loop, tools, stateful agent behavior, queues, lifecycle events, and proxy stream helper. |
| `pi-agent-core` harness/session | `gi-agent-core/harness` | Pi-compatible for audited sessions, prompt formatting, compaction, local env, skills, and storage helpers. |
| `@earendil-works/pi-tui` | `gi-tui` | Pi-compatible for the audited public TUI surface and case-level behavior; Markdown and headless terminal behavior remain fixture-level parity risks. |
| `@earendil-works/pi-coding-agent` | `gi-coding-agent` | The declared Pi v0.82.1 audit scope is closed. TypeScript in-process extensions and Node/npm lifecycle behavior remain explicit Go protocol/runtime replacements rather than direct API copies. |

Detailed coverage is tracked in [PI_COMPATIBILITY.md](PI_COMPATIBILITY.md).
Per-case provider/agent mapping is tracked in
[PI_AI_AGENT_TEST_CASE_PARITY.md](PI_AI_AGENT_TEST_CASE_PARITY.md), and per-case
TUI mapping is tracked in [PI_TUI_TEST_CASE_PARITY.md](PI_TUI_TEST_CASE_PARITY.md).
The Pi coding-agent migration scope is documented in
[PI_CODING_AGENT_TEST_CASE_PARITY.md](PI_CODING_AGENT_TEST_CASE_PARITY.md),
with source-level disposition documented in
[PI_CODING_AGENT_SOURCE_AUDIT.md](PI_CODING_AGENT_SOURCE_AUDIT.md).

The cross-language package, extension, and custom TUI component protocol used by
the coding-agent runtime is described in
[protocol/README.md](protocol/README.md), with machine-readable schemas,
registries, and replay examples under [protocol/spec/](protocol/spec/).
Gi uses `.gi` for project-local coding-agent settings, resources, sessions, and
package stores. Default auto-discovery does not read `.pi`; Pi case names that
mention `.pi` are preserved verbatim but treated as `.gi` equivalents in Gi's
parity table.

## Installation

This module targets Go 1.26.

```sh
go get github.com/nowa/gi/gi-llm-provider
go get github.com/nowa/gi/gi-agent-core
go get github.com/nowa/gi/gi-tui
```

## CLI Usage

Gi includes a Go CLI entrypoint for the coding-agent migration surface:

```sh
go run ./cmd/gi --help
go run ./cmd/gi --list-models
go run ./cmd/gi --model openai/gpt-4o-mini "Summarize this repository"
go run ./cmd/gi -p --model openai/gpt-4o-mini "Summarize this repository"
go run ./cmd/gi --offline --no-session --model openai/gpt-4o-mini "Smoke test"
go run ./cmd/gi install official:gi-plan-mode -l
go run ./cmd/gi config
```

`-p` runs non-interactive print mode. Live model calls use provider credentials
from `--api-key`, `~/.gi/agent/auth.json`, `~/.gi/agent/models.json`, or provider
environment variables. `--offline --no-session` uses the local deterministic
responder without writing session files and is intended for smoke tests.

The default path is split by terminal mode. In an interactive TTY, Gi starts a
Go-native TUI host with header/chat/editor/footer regions, initial prompts,
editor submit handling, scoped-model startup notice, `models.json` error
warnings, Anthropic subscription-auth warning, startup changelog,
release-update, and package-update notices, Pi-style terminal title updates
including protocol `host.tui.title`, resource sections for context
files, skills, prompts, extensions, themes, quiet-startup diagnostics, and
protocol ViewTree
slots/overlays, editor-slot replacement, keyed status/footer text,
`host.tui.editor` paste semantics, `host.tui.working` loader control, hidden
thinking-label control, and built-in
`/thinking`, `/theme`, `/model`, `/queue`,
`/models`, `/settings`, `/resources`, `/session`, `/hotkeys`, `/changelog`, `/export`, `/share`,
`/import`, `/resume <path>`, `/fork`, `/tree`, `/name`, `/new`, `/compact`, `/copy`,
`/clone`, `/reload`, and `/quit` commands, `!`/`!!` bash execution, and
notify/confirm/select/input/editor dialog overlays that can update through
runtime mount/patch/unmount/status/dialog host actions. Overlay mounts support
anchor, size, margin, priority, and non-capturing focus options. Package
extensions can also read, set, insert into, and submit the active editor through
the protocol host bridge; the default editor can toggle thinking block
visibility with `Ctrl+T`, open the configured editor, `$VISUAL`, `$EDITOR`, or
the platform default with `Ctrl+G`, and paste clipboard images to temp files
with `Ctrl+V`. External edits use a private temporary workspace and replace
editor text only after a successful exit. `Ctrl+Z` suspends the live TUI on
Unix-like systems and restores it on `SIGCONT`. In tmux, startup checks warn
when extended keyboard reporting is likely to break modified Enter keys. In
non-TTY contexts it falls back to a basic single-turn host for
`gi <message>` or piped stdin. The Pi-equivalent TUI workflow disposition is
documented in `PI_CODING_AGENT_SOURCE_AUDIT.md`.

Working, retry, compaction, and branch-summary activity share one typed
transient status slot. Each replacement disposes the previous loader and any
owned countdown; typed completion events cannot clear a newer status, and
shutdown drains the active status lifecycle before stopping the TUI.

For an official default installation with no settings file,
`GI_EXPERIMENTAL=1` (or legacy `PI_EXPERIMENTAL=1`) enables Pi-compatible
first-time theme and analytics setup before the interactive runtime is built.
Analytics defaults to disabled; opting in stores one stable local tracking ID
together with the theme and preference in a single settings transition.

### Project Trust

Gi gates project-local settings, packages, extensions, skills, prompts, themes,
and project system-prompt files behind one runtime trust decision. Decisions are
stored in `~/.gi/agent/trust.json` and inherit from parent directories.
Before consulting a saved decision, Gi loads only global/user and explicit
extensions; trusted in-process Go extensions may answer the `project_trust`
event. A decisive answer can optionally be remembered. The final trusted or
untrusted extension set is then completed on the same runtime, so bootstrap
extensions initialize once and auto-discovered project code never runs before
approval.
Interactive startup asks when trust-requiring resources exist; non-interactive
modes use the saved decision or global default, with `ask` safely resolving to
untrusted when no UI is available.

Use `--approve` / `-a` or `--no-approve` / `-na` for a session-only override.
Inside the TUI, `/trust` changes the saved decision for the next Gi process.
The global `defaultProjectTrust` setting accepts `ask`, `always`, or `never`.
Explicit resource paths supplied on the command line remain explicit user
choices and are not treated as auto-discovered project resources.

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

`gi-llm-provider` includes registered transports/provider boundaries for:

- Anthropic Messages
- OpenAI Responses and Chat Completions
- OpenAI Codex Responses
- Azure OpenAI Responses
- Google Generative AI and Vertex AI
- Mistral Conversations
- Amazon Bedrock Converse Stream event parsing with an injectable AWS transport boundary
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

See [CONTRIBUTING.md](CONTRIBUTING.md) for contributor guidance and
[AGENTS.md](AGENTS.md) for repository-specific coding-agent rules. Keep tests
deterministic and avoid requiring live provider credentials in the default suite.

## License

Gi is distributed under the MIT License. See [LICENSE](LICENSE), [NOTICE](NOTICE),
and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). Security reporting guidance
is in [SECURITY.md](SECURITY.md).
