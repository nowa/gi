# Repository Guidelines

## Project Structure & Module Organization

This is the Go module `github.com/nowa/gi`, targeting Go 1.26. Main packages:

- `gi-llm-provider/`: provider registry, model catalog, message types, payload conversion, streaming contracts, and HTTP/SSE transports.
- `gi-agent-core/`: agent loop, tool execution, lifecycle events, queues, and turn orchestration.
- `gi-agent-core/harness/`: session storage, prompt formatting, compaction, skills, and local execution helpers.
- `gi-tui/`: terminal UI components, key parsing, raw terminal support, image helpers, and headless rendering tests.

Tests live beside implementation as `*_test.go`. Root docs include `README.md` and `PI_COMPATIBILITY.md`.

## Build, Test, and Development Commands

- `go test -timeout 30s ./...`: run the full test suite.
- `GOCACHE=/private/tmp/gi-gocache go test -timeout 30s ./...`: run tests with a stable cache in sandboxed environments.
- `go test -run TestName ./gi-llm-provider`: run a focused provider test while iterating.
- `go test ./gi-agent-core/...`: test the agent core package and harness helpers.
- `go test ./gi-tui`: run the terminal UI compatibility tests.
- `gofmt -w path/to/file.go`: format edited Go files before committing.

There is no Makefile; `go test ./...` is the default validation gate.

## Coding Style & Naming Conventions

Follow standard Go style: tabs from `gofmt`, short package names, exported identifiers only for public API, and table-driven tests for multi-case behavior. Keep provider-specific code in files named after the provider or protocol, such as `anthropic_payload.go`. Prefer explicit conversion and validation helpers over implicit global behavior.

## Testing Guidelines

Use Go's built-in `testing` package. Name tests `TestFeatureOrContract` and keep fixtures local unless intentionally shared. Default tests must be deterministic and must not require live credentials or network calls. Add contract tests when changing stream events, payload shapes, model catalogs, or provider registration.

## Commit & Pull Request Guidelines

Recent commits use short imperative subjects, such as `Add project README`. Keep each commit focused on one logical change.

Pull requests should include a concise description, affected package paths, validation commands, and Pi compatibility or provider-behavior impact. Include sample payloads or stream traces for protocol changes. Link issues when available, and call out new environment variables such as `OPENAI_API_KEY` or provider-specific equivalents.

## Security & Configuration Tips

Do not commit API keys or live credential fixtures. Provider API keys should come from explicit stream options or environment variables. Keep credentialed live checks separate from default tests so contributors can validate the library without external accounts.
